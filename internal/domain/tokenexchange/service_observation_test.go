package tokenexchange

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type observingGrantRepository struct {
	MockGrantRepository
	context context.Context
}

func (r *observingGrantRepository) FindByPrincipalAndAgent(ctx context.Context, principal id.Principal, agent id.AgentID) (*storage.UserGrant, error) {
	r.context = ctx
	return r.MockGrantRepository.FindByPrincipalAndAgent(ctx, principal, agent)
}

func observationFixture(t *testing.T) (*TokenExchangeService, *TokenExchangeRequest, *observingGrantRepository, *MockSessionRepository) {
	t.Helper()
	key, keyset := generateTestRSAKeySet(t)
	agentID, serviceID, setID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
	future := time.Now().Add(time.Hour)
	agent := &storage.Agent{ID: agentID, DisplayName: "Observation agent", PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: setID, RequirementType: storage.RequirementTypeOptional}}}
	agentRepo := &singleAgentRepo{agentID: agentID, agent: agent}
	grants := &observingGrantRepository{MockGrantRepository: MockGrantRepository{grant: &storage.UserGrant{
		ID: id.NewGrantID(), Principal: "human", AgentID: agentID, ValidUntil: &future,
		GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: setID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
	}}}
	sets := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storage.PermissionSet{setID: {
		ID: setID, Name: "read", ServiceScopes: []storage.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional}},
	}}}
	ps := permissionset.NewPermissionSetService(sets, grants, slog.Default())
	t.Cleanup(ps.Close)
	sessions := &MockSessionRepository{session: &storage.UserSession{
		ID: id.NewSessionID(), Principal: "human", ServiceID: serviceID, EncryptedAccessToken: []byte("synthetic-access"),
		TokenType: "Bearer", AccessTokenExpiresAt: &future, Scope: []string{"read"},
	}}
	validator, err := NewJWTValidator(&MockJWKSProvider{keySet: keyset}, "https://issuer.example.com", "broker", 0)
	require.NoError(t, err)
	evaluator, err := NewCELEvaluator(CELEvaluatorConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.agent_id", AuthorizationExpression: "true"})
	require.NoError(t, err)
	providers := newTestProviderService(&MockServiceRepository{service: &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "Service", ClientID: "provider-client", Secret: model.NewEncryptedSecret([]byte("synthetic-secret")), ProtectedResources: []string{"https://api.example.com"},
	}})
	svc := &TokenExchangeService{
		jwtValidator: validator, celEvaluator: evaluator, providerService: providers, agentRepository: agentRepo,
		consentService:       consent.NewService(agentRepo, providers, grants, nil, nil, nil, slog.Default()),
		permissionSetService: ps,
		oauth2SessionService: oauth2session.NewOAuth2SessionService(nil, sessions, nil, nil, nil, &MockEncryption{}, nil, nil, oauth2session.Config{CallbackBaseURL: "https://broker.example.com"}, slog.Default()),
		config:               &ports.TokenExchangeConfig{},
	}
	claims := map[string]interface{}{"iss": "https://issuer.example.com", "aud": "broker", "sub": "human", "agent_id": agentID.String(), "exp": future.Unix(), "iat": time.Now().Unix()}
	token := signServiceTestJWT(t, key, claims)
	req := NewTokenExchangeRequest(TokenExchangeGrantType, token, AccessTokenType, token, JWTBearerType, "https://api.example.com", "")
	return svc, req, grants, sessions
}

func TestExchangeObservationNoopAndCancellation(t *testing.T) {
	svc, req, grants, sessions := observationFixture(t)
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(noop.NewTracerProvider())
	t.Cleanup(func() { otel.SetTracerProvider(previous) })
	response, err := svc.Exchange(context.Background(), req)
	require.NoError(t, err)
	require.Equal(t, "synthetic-access", response.AccessToken)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	grants.findByPrincipalAndAgentCalls = 0
	sessions.findByPrincipalAndServiceCalls = 0
	response, err = svc.Exchange(ctx, req)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, response)
	require.Zero(t, grants.findByPrincipalAndAgentCalls)
	require.Zero(t, sessions.findByPrincipalAndServiceCalls)
}

func TestExchangeHumanStateObservation(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		grantMissing, grantError, sessionError bool
		wantSession                            bool
	}{
		{name: "success", wantSession: true}, {name: "missing grant", grantMissing: true},
		{name: "grant repository error", grantError: true}, {name: "session repository error", sessionError: true, wantSession: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, req, grants, sessions := observationFixture(t)
			recorder := tracetest.NewSpanRecorder()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
			previous := otel.GetTracerProvider()
			otel.SetTracerProvider(provider)
			t.Cleanup(func() { otel.SetTracerProvider(previous); require.NoError(t, provider.Shutdown(context.Background())) })
			ctx, parent := provider.Tracer("test").Start(context.Background(), "parent")
			defer parent.End()
			if tc.grantMissing {
				grants.grant = nil
			}
			if tc.grantError {
				grants.err = errors.New("synthetic grant failure")
			}
			if tc.sessionError {
				sessions.err = errors.New("synthetic session failure")
			}
			var sessionContext context.Context
			sessions.findByPrincipalAndServiceFunc = func(ctx context.Context, _ id.Principal, _ id.ServiceID) (*storage.UserSession, error) {
				sessionContext = ctx
				return sessions.session, sessions.err
			}
			response, err := svc.Exchange(ctx, req)
			if tc.grantMissing || tc.grantError || tc.sessionError {
				require.Error(t, err)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.Equal(t, "synthetic-access", response.AccessToken)
			}
			seen := map[string]sdktrace.ReadOnlySpan{}
			for _, span := range recorder.Ended() {
				seen[span.Name()] = span
			}
			grantSpan := seen["tokenexchange.verify_agent_access"]
			require.NotNil(t, grantSpan)
			require.Empty(t, grantSpan.Attributes())
			require.Empty(t, grantSpan.Events())
			require.Equal(t, grantSpan.SpanContext(), trace.SpanContextFromContext(grants.context))
			require.Equal(t, parent.SpanContext().SpanID(), grantSpan.Parent().SpanID())
			if tc.wantSession {
				sessionSpan := seen["tokenexchange.get_valid_access_token"]
				require.NotNil(t, sessionSpan)
				require.Equal(t, sessionSpan.SpanContext(), trace.SpanContextFromContext(sessionContext))
				require.Empty(t, sessionSpan.Attributes())
				require.Empty(t, sessionSpan.Events())
			} else {
				require.NotContains(t, seen, "tokenexchange.get_valid_access_token")
			}
		})
	}
}
