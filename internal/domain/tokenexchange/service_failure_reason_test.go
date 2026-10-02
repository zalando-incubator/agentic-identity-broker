package tokenexchange

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type resolvingServiceRepository struct{ MockServiceRepository }

func (r *resolvingServiceRepository) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return r.service, r.err
}

func TestExchange_ClassifiesFailuresForResolvedService(t *testing.T) {
	privateKey, keySet := generateTestRSAKeySet(t)
	for _, tc := range []struct {
		name   string
		code   string
		reason FailureReason
		status int
		body   string
	}{
		{name: "no grant", code: "access_denied", reason: FailureReasonNoGrant},
		{name: "no session", code: "invalid_grant", reason: FailureReasonNoSession},
		{name: "access token expired, no refresh token", code: "invalid_grant", reason: FailureReasonAccessTokenExpired},
		{name: "stored refresh token expired", code: "invalid_grant", reason: FailureReasonRefreshTokenExpired},
		{name: "service invalid_grant", code: "server_error", reason: FailureReasonRefreshTokenExpired, status: 400, body: `{"error":"invalid_grant"}`},
		{name: "service rejects otherwise", code: "server_error", reason: FailureReason("service_rejected"), status: 502, body: "unavailable"},
		{name: "service unreachable", code: "server_error"},
		{name: "insufficient scope", code: "invalid_grant", reason: FailureReasonInsufficientScope},
		{name: "unresolved resource", code: "invalid_target"},
		{name: "success"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			agentID, svcID, psID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status == 0 {
					t.Errorf("unexpected refresh request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			if tc.name == "service unreachable" {
				upstream.Close()
			}
			repo := &resolvingServiceRepository{MockServiceRepository{service: &model.ThirdpartyOAuth2ProviderEntity{
				ID: svcID, DisplayName: "Example Service", ClientID: "service-client",
				TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone, Secret: model.NewAbsentSecret(),
				ProtectedResources: []string{"https://api.example.com/resource"},
				Endpoints:          model.OAuth2Endpoints{TokenEndpoint: upstream.URL},
			}}}
			if tc.name == "unresolved resource" {
				repo.service = nil
				repo.err = NewInvalidTargetError("no service configured for the requested resource")
			}
			agent := &storagedomain.Agent{ID: agentID, DisplayName: "Test Agent", PermissionSets: []storagedomain.AgentPermissionSetEntry{
				{PermissionSetID: psID, RequirementType: storagedomain.RequirementTypeOptional},
			}}
			agentRepo := &singleAgentRepo{agentID: agentID, agent: agent}
			future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
			grantRepo := &MockGrantRepository{grant: &storagedomain.UserGrant{
				ID: id.NewGrantID(), AgentID: agentID, Principal: "user@example.com", ValidUntil: &future,
				GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}}},
			}}
			if tc.name == "no grant" {
				grantRepo.grant = nil
			}
			psRepo := &MockPermissionSetRepository{psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{psID: {
				ID: psID, Name: "Test Permission Set", ServiceScopes: []storagedomain.ServiceScope{
					{ServiceID: svcID, Scopes: []string{"repo"}, RequirementType: storagedomain.RequirementTypeOptional},
				},
			}}}
			sessionRepo := &MockSessionRepository{session: &storagedomain.UserSession{
				ID: id.NewSessionID(), Principal: "user@example.com", ServiceID: svcID,
				EncryptedAccessToken: []byte("access-token"), TokenType: "Bearer", Scope: []string{"repo"}, AccessTokenExpiresAt: &future,
			}}
			switch tc.name {
			case "no session":
				sessionRepo.session = nil
			case "access token expired, no refresh token":
				sessionRepo.session.AccessTokenExpiresAt = &past
			case "stored refresh token expired", "service invalid_grant", "service rejects otherwise", "service unreachable":
				sessionRepo.session.AccessTokenExpiresAt = &past
				sessionRepo.session.EncryptedRefreshToken = []byte("refresh-token")
				if tc.name == "stored refresh token expired" {
					sessionRepo.session.RefreshTokenExpiresAt = &past
				}
			case "insufficient scope":
				sessionRepo.session.Scope = []string{}
			}
			thirdpartyService := newTestProviderService(repo)
			sessionService := oauth2session.NewOAuth2SessionService(thirdpartyService, sessionRepo, sessionRepo, nil, nil, &MockEncryption{}, &http.Client{Timeout: time.Second}, nil, oauth2session.DefaultConfig(), slog.Default())
			jwtValidator, err := NewJWTValidator(&MockJWKSProvider{keySet: keySet}, "https://auth.example.com", "agentic-identity-broker", 60)
			require.NoError(t, err)
			celEvaluator, err := NewCELEvaluator(CELEvaluatorConfig{
				PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp",
				AuthorizationExpression: "true", EvaluationTimeout: 100 * time.Millisecond,
			})
			require.NoError(t, err)
			svc := &TokenExchangeService{
				jwtValidator: jwtValidator, celEvaluator: celEvaluator, providerService: thirdpartyService,
				oauth2SessionService: sessionService, agentRepository: agentRepo,
				consentService:       consent.NewService(agentRepo, thirdpartyService, grantRepo, nil, nil, slog.Default()),
				permissionSetService: permissionset.NewPermissionSetService(psRepo, grantRepo, slog.Default()),
			}
			claims := map[string]interface{}{
				"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": "user@example.com",
				"azp": agentID.String(), "exp": future.Unix(), "iat": time.Now().Unix(),
			}
			req := NewTokenExchangeRequest(TokenExchangeGrantType, signServiceTestJWT(t, privateKey, claims), AccessTokenType,
				signServiceTestJWT(t, privateKey, claims), JWTBearerType, "https://api.example.com/resource", "")
			response, err := svc.Exchange(context.Background(), req)
			serviceRef := ServiceRef{ID: svcID, Name: "Example Service"}
			if tc.code == "" {
				require.NoError(t, err)
				assert.Equal(t, serviceRef, response.Service)
				return
			}
			require.Error(t, err)
			tokenErr, ok := err.(*TokenExchangeError)
			require.True(t, ok, "error must be *TokenExchangeError, got %T: %v", err, err)
			assert.Equal(t, tc.code, tokenErr.Code())
			assert.Equal(t, tc.reason, tokenErr.FailureReason())
			if tc.name == "unresolved resource" {
				serviceRef = ServiceRef{}
			}
			assert.Equal(t, serviceRef, tokenErr.Service())
		})
	}
}
