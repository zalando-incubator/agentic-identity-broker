package consent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	httpconsent "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/handlers/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	principalcontext "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type gateEvaluatorFunc func(context.Context, []*storage.UserSession, ports.UsabilityMode) (map[id.ServiceID]bool, error)

func (f gateEvaluatorFunc) RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	return f(ctx, sessions, mode)
}

func usableTestSessions(_ context.Context, sessions []*storage.UserSession, _ ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	result := make(map[id.ServiceID]bool, len(sessions))
	for _, session := range sessions {
		result[session.ServiceID] = session.RequiresReauthentication()
	}
	return result, nil
}

func TestUsabilityGateBoundariesLogOneSafeContextualFailure(t *testing.T) {
	key, err := jwk.Import[jwk.Key]([]byte("0123456789abcdef0123456789abcdef"))
	require.NoError(t, err)
	tokenService := sessiontoken.NewService(domjwe.New(key))
	reader := metric.NewManualReader()
	meterProvider := metric.NewMeterProvider(metric.WithReader(reader))
	recorder := tracetest.NewSpanRecorder()
	tracerProvider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	oldTracer, oldMeter := otel.GetTracerProvider(), otel.GetMeterProvider()
	otel.SetTracerProvider(tracerProvider)
	otel.SetMeterProvider(meterProvider)
	t.Cleanup(func() {
		otel.SetTracerProvider(oldTracer)
		otel.SetMeterProvider(oldMeter)
		require.NoError(t, tracerProvider.Shutdown(context.Background()))
		require.NoError(t, meterProvider.Shutdown(context.Background()))
	})
	for _, boundary := range []string{"authorization", "grant"} {
		for _, reason := range []model.CredentialSourceReason{
			model.CredentialSourceReasonIdentityMissing, model.CredentialSourceReasonIdentityMismatch, model.CredentialSourceReasonIdentitySuperseded,
			model.CredentialSourceReasonMissingBinding, model.CredentialSourceReasonGenerationChanged, model.CredentialSourceReasonNotFound,
			model.CredentialSourceReasonPermissionDenied, model.CredentialSourceReasonNotRegular, model.CredentialSourceReasonTooLarge,
			model.CredentialSourceReasonEmpty, model.CredentialSourceReasonReadFailed, "untrusted-reason-sentinel",
		} {
			for _, outcome := range []string{"failure", "canceled", "telemetry_disabled"} {
				t.Run(fmt.Sprintf("%s/%s/%s", boundary, reason, outcome), func(t *testing.T) {
					var logs bytes.Buffer
					logger := slog.New(telemetry.NewContextHandler(telemetry.NewPrincipalPolicyHandler(slog.NewJSONHandler(&logs, nil))))
					if outcome == "telemetry_disabled" {
						logger = slog.New(slog.DiscardHandler)
					}
					principal := id.Principal("alice@example.com")
					ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{TraceID: "gate-trace", Actor: id.Principal("operator@example.com"), CallingPeer: "agent-peer"})
					agentID, psID := id.NewAgentID(), id.NewPermissionSetID()
					firstID, failedID := id.NewServiceID(), id.NewServiceID()
					agents, grants := memory.NewAgentRepository(), memory.NewUserGrantRepository()
					sessions := memory.NewInMemoryUserSessionRepository()
					psRepo := memory.NewPermissionSetRepository()
					agent := &storage.Agent{ID: agentID, DisplayName: "Reader", Description: "Gate test", RedirectURIs: []string{"https://agent.example.com/callback"}, PermissionSets: []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory}}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: firstID, RequirementType: storage.RequirementTypeMandatory}, {ServiceID: failedID, RequirementType: storage.RequirementTypeOptional}}}
					require.NoError(t, agents.Create(ctx, agent))
					require.NoError(t, psRepo.Create(ctx, &storage.PermissionSet{ID: psID, Name: "Read", Description: "Gate test", ServiceScopes: []storage.ServiceScope{{ServiceID: firstID, RequirementType: storage.RequirementTypeMandatory}, {ServiceID: failedID, RequirementType: storage.RequirementTypeOptional}}}))
					entries := []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{firstID, failedID}}}
					expires := time.Now().Add(time.Hour)
					require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), AgentID: agentID, Principal: principal, GrantedPermissionSets: entries, ValidUntil: &expires}))
					for _, serviceID := range []id.ServiceID{firstID, failedID} {
						require.NoError(t, sessions.Create(ctx, &storage.UserSession{ID: id.NewSessionID(), ServiceID: serviceID, Principal: principal, TokenType: "Bearer", EncryptedAccessToken: []byte("ciphertext-sentinel")}))
					}
					cause := errors.Join(&model.CredentialSourceError{Reason: reason}, errors.New("/private/credential-path-sentinel/client-id-basename-sentinel old-client-sentinel new-client-sentinel access-token-sentinel refresh-token-sentinel secret-sentinel file-content-sentinel"))
					if outcome == "canceled" {
						cause = errors.Join(cause, context.Canceled)
					}
					failure := &ports.SessionUsabilityError{ServiceID: failedID, Err: cause}
					calls := 0
					evaluator := gateEvaluatorFunc(func(gotCtx context.Context, batch []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
						assert.Equal(t, ctx, gotCtx)
						assert.Equal(t, ports.UsabilityGate, mode)
						require.Len(t, batch, 2, "one batch preserves exact failure attribution independently of map order")
						calls++
						return nil, failure
					})
					if boundary == "authorization" {
						svc := oauth2.NewAuthorizationService(grants, sessions, oauth2.NewAgentClientResolver(agents, logger), &oauth2.OAuth2Config{PublicURL: "https://broker.example.com", ModeStrategy: oauth2.NewLocalModeStrategy()}, logger, tokenService).WithSessionUsabilityEvaluator(evaluator)
						decision, err := svc.HandleAuthorization(ctx, &ports.AuthorizationRequest{ClientID: id.ClientID(agentID.String()), RedirectURI: "https://agent.example.com/callback", ResponseType: "code", State: "sealed-state-sentinel", CodeChallenge: "pkce-sentinel", CodeChallengeMethod: "S256"}, principal)
						require.NoError(t, err)
						assert.Equal(t, "error", decision.Action)
						assert.Equal(t, "server_error", decision.ErrorCode)
					} else {
						ps := permissionset.NewPermissionSetService(psRepo, grants, logger)
						t.Cleanup(ps.Close)
						svc := consent.NewService(agents, nil, grants, sessions, nil, ps, logger).WithSessionUsabilityEvaluator(evaluator)
						grant, err := svc.GrantConsent(ctx, &consent.GrantRequest{AgentID: agentID, Principal: principal, GrantedPermissionSets: entries, ValidUntil: &expires})
						assert.Nil(t, grant)
						require.ErrorIs(t, err, failure)
						assert.NotErrorIs(t, err, consent.ErrUnconnectedServices)
						assert.Empty(t, logs.String(), "a domain service that returns the failure must not log it")
						body := httpconsent.GrantRequest{GrantedPermissionSets: map[string][]string{psID.String(): {firstID.String(), failedID.String()}}, ValidUntil: &expires}
						encoded, err := json.Marshal(body)
						require.NoError(t, err)
						route := chi.NewRouteContext()
						route.URLParams.Add("agent-id", agentID.String())
						ctx = context.WithValue(principalcontext.WithPrincipal(ctx, principal), chi.RouteCtxKey, route)
						request := httptest.NewRequest(http.MethodPost, "/api/consent/agents/"+agentID.String()+"/grants", bytes.NewReader(encoded)).WithContext(ctx)
						response := httptest.NewRecorder()
						httpconsent.NewGrantsHandler(svc, logger, tokenService).CreateGrant(response, request)
						assert.Equal(t, http.StatusInternalServerError, response.Code)
						assert.NotContains(t, response.Body.String(), "sentinel")
					}
					expectedCalls := 1
					if boundary == "grant" {
						expectedCalls = 2
					}
					assert.Equal(t, expectedCalls, calls)
					if outcome == "canceled" || outcome == "telemetry_disabled" {
						assert.Empty(t, logs.String(), "cancellation and disabled telemetry emit no failure record")
						return
					}
					require.Equal(t, 1, strings.Count(logs.String(), "\n"), "one boundary record, no helper or generic duplicate")
					var record map[string]any
					require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
					level := "ERROR"
					if reason == model.CredentialSourceReasonIdentityMissing || reason == model.CredentialSourceReasonIdentityMismatch || reason == model.CredentialSourceReasonIdentitySuperseded {
						level = "WARN"
					}
					assert.Equal(t, level, record["level"])
					assert.Equal(t, "session.oauth2.credential_source_failed", record["event"])
					assert.Equal(t, failedID.String(), record["service_id"])
					assert.Equal(t, "usability_gate", record["operation"])
					assert.Equal(t, "failed", record["outcome"])
					assert.Equal(t, string(model.NewCredentialSourceError(reason).Reason), record["reason"])
					assert.Equal(t, principal.String(), record["principal"], "the real principal policy renders the typed session owner")
					assert.Equal(t, "operator@example.com", record["actor"])
					assert.Equal(t, "gate-trace", record["trace_id"])
					assert.Equal(t, "agent-peer", record["calling_peer"])
					assert.Equal(t, map[string]any{"operation": "session_lookup", "failure_detail": "credential_source_unavailable", "error_kind": "infrastructure", "dependency": "credential_source", "http_status_code": float64(0), "oauth_error_code": ""}, record["oauth2_session"])
					for _, sentinel := range []string{"credential-path-sentinel", "client-id-basename-sentinel", "old-client-sentinel", "new-client-sentinel", "access-token-sentinel", "refresh-token-sentinel", "secret-sentinel", "ciphertext-sentinel", "file-content-sentinel", "sealed-state-sentinel", "pkce-sentinel", "untrusted-reason-sentinel"} {
						assert.NotContains(t, logs.String(), sentinel)
					}
				})
			}
		}
	}
	assert.Empty(t, recorder.Ended(), "usability gates add no spans, events, status, or principal attributes")
	var metrics metricdata.ResourceMetrics
	require.NoError(t, reader.Collect(context.Background(), &metrics))
	assert.Empty(t, metrics.ScopeMetrics, "usability gates add no instruments or metric dimensions")
}

type identityGateProviderRepo struct {
	ports.ThirdpartyOAuth2ProviderRepository
	get func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error)
}

func (r identityGateProviderRepo) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return r.get(ctx, serviceID)
}

type unusedGateEncryption struct{ ports.EncryptionPort }
type unusedGateBranchKeys struct{ ports.BranchKeyManager }

func TestUsabilityGateProducerAttributesExactFailingService(t *testing.T) {
	for _, failLookup := range []bool{false, true} {
		t.Run(fmt.Sprintf("lookup_failure=%t", failLookup), func(t *testing.T) {
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, nil))
			firstID, failedID := id.NewServiceID(), id.NewServiceID()
			lookupErr := errors.New("repository-path-sentinel")
			reads := 0
			repo := identityGateProviderRepo{get: func(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				reads++
				if serviceID == firstID {
					return &model.ThirdpartyOAuth2ProviderEntity{ID: firstID, ClientID: "first-client-sentinel", CredentialSource: model.CredentialSourceStored}, nil
				}
				if failLookup {
					return nil, lookupErr
				}
				return &model.ThirdpartyOAuth2ProviderEntity{ID: failedID, CredentialSource: model.CredentialSourceFilesystem}, nil
			}}
			providers := thirdparty.NewThirdpartyOAuth2ProviderService(repo, unusedGateEncryption{}, unusedGateBranchKeys{}, nil, false, logger)
			evaluator := oauth2session.NewOAuth2SessionService(providers, nil, nil, nil, nil, nil, nil, nil, oauth2session.DefaultConfig(), logger)
			t.Cleanup(func() { require.NoError(t, evaluator.Close(context.Background())) })
			result, err := evaluator.RequiresReauthentication(context.Background(), []*storage.UserSession{{ServiceID: firstID}, {ServiceID: failedID}}, ports.UsabilityGate)
			assert.Nil(t, result)
			var failure *ports.SessionUsabilityError
			require.ErrorAs(t, err, &failure)
			assert.Equal(t, failedID, failure.ServiceID)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			var source *model.CredentialSourceError
			require.ErrorAs(t, err, &source)
			want := model.CredentialSourceReasonMissingBinding
			if failLookup {
				want = model.CredentialSourceReasonReadFailed
			}
			assert.Equal(t, want, source.Reason)
			assert.Equal(t, 2, reads, "attribution must not reread the service")
			assert.Empty(t, logs.String(), "the returning evaluator never logs a Gate failure")
			assert.NotContains(t, err.Error(), "sentinel")
		})
	}
}

func TestSessionUsabilityErrorPreservesCauseWithoutLoggingIt(t *testing.T) {
	cause := errors.New("/private/credential-file-sentinel client-id-sentinel token-sentinel")
	failure := &ports.SessionUsabilityError{ServiceID: id.NewServiceID(), Err: cause}
	assert.ErrorIs(t, failure, cause)
	assert.Same(t, cause, errors.Unwrap(failure))
	assert.NotContains(t, fmt.Sprint(failure), "sentinel")
	var logs bytes.Buffer
	slog.New(slog.NewJSONHandler(&logs, nil)).Error("usability failed", "oauth2_session", failure)
	assert.NotContains(t, logs.String(), "sentinel")
	var record map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &record))
	assert.Equal(t, map[string]any{
		"operation": "session_lookup", "failure_detail": "credential_source_unavailable", "error_kind": "infrastructure",
		"dependency": "credential_source", "http_status_code": float64(0), "oauth_error_code": "",
	}, record["oauth2_session"])
}
