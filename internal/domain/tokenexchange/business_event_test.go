package tokenexchange

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/stretchr/testify/require"
)

func TestExchangeLedgerSpecificDenialHasPrecedenceOverGenericTokenFailure(t *testing.T) {
	for _, refusal := range []string{"authentication", "authorization"} {
		t.Run(refusal, func(t *testing.T) {
			key, keySet := generateTestRSAKeySet(t)
			agentID := id.NewAgentID()
			svc := newServiceForStep9TestWithAuthz(t, keySet, &trackingAgentRepository{}, "false")
			store := &ledgerfixture.Store{}
			svc.ledger = store.Recorder(t)
			now := time.Now()
			claims := map[string]interface{}{"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": "ledger-user", "azp": agentID.String(), "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()}
			subject := signServiceTestJWT(t, key, claims)
			assertion := signServiceTestJWT(t, key, claims)
			if refusal == "authentication" {
				assertion = "invalid-client-assertion-canary"
			}
			request := NewTokenExchangeRequest(TokenExchangeGrantType, subject, AccessTokenType, assertion, JWTBearerType, "https://api.example.com/resource", "")
			response, err := svc.Exchange(context.Background(), request)
			require.Error(t, err)
			require.Nil(t, response)
			require.Len(t, store.Events, 1, "one final specific refusal, without token-request-failed or token-issued duplicates")
			require.Equal(t, model.BusinessEventTypePrefix+"token-exchange-denied", store.Events[0].Type)
			require.Equal(t, model.BusinessEventDenied, store.Events[0].Outcome)
			if refusal == "authentication" {
				require.Nil(t, store.Events[0].Subject)
			}
		})
	}
}

func TestExchangeLedgerInternalEvaluationErrorIsNotAPermissionDenial(t *testing.T) {
	key, keySet := generateTestRSAKeySet(t)
	agentID := id.NewAgentID()
	svc := newServiceForStep9TestWithAuthz(t, keySet, &trackingAgentRepository{}, "client_assertion.missing == 'allowed'")
	store := &ledgerfixture.Store{}
	svc.ledger = store.Recorder(t)
	now := time.Now()
	claims := map[string]interface{}{"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": "ledger-user", "azp": agentID.String(), "exp": now.Add(time.Hour).Unix(), "iat": now.Unix()}
	subject := signServiceTestJWT(t, key, claims)
	assertion := signServiceTestJWT(t, key, claims)
	request := NewTokenExchangeRequest(TokenExchangeGrantType, subject, AccessTokenType, assertion, JWTBearerType, "https://api.example.com/resource", "")
	response, err := svc.Exchange(context.Background(), request)
	require.Nil(t, response)
	var exchangeErr *TokenExchangeError
	require.ErrorAs(t, err, &exchangeErr)
	require.Equal(t, "server_error", exchangeErr.Code())
	require.Len(t, store.Events, 1)
	require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", store.Events[0].Type)
	require.Equal(t, "internal_failure", store.Events[0].Data["reason_code"])
	require.Nil(t, store.Events[0].Actor.OnBehalfOf, "an evaluation error establishes no delegation")
}

func TestExchangeLedgerInsufficientSessionScopesAreAuthorizationDenial(t *testing.T) {
	key, keySet := generateTestRSAKeySet(t)
	agentID, serviceID, permissionSetID := id.NewAgentID(), id.NewServiceID(), id.NewPermissionSetID()
	const principal = "ledger-scope-user"
	const caller = "verified-scope-client"
	const resource = "https://api.example.com/resource"
	now := time.Now()
	subjectToken := signServiceTestJWT(t, key, map[string]interface{}{
		"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": principal,
		"azp": agentID.String(), "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	})
	assertion := signServiceTestJWT(t, key, map[string]interface{}{
		"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": caller,
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	})
	for _, scopes := range [][]string{nil, {"read"}, {"read", "write"}} {
		t.Run("scopes="+fmt.Sprint(scopes), func(t *testing.T) {
			store := &ledgerfixture.Store{}
			recorder := store.Recorder(t)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			agentRepo := &singleAgentRepo{agentID: agentID, agent: &storagedomain.Agent{
				ID: agentID, PermissionSets: []storagedomain.AgentPermissionSetEntry{{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeOptional}},
			}}
			grant := &storagedomain.UserGrant{
				ID: id.NewGrantID(), AgentID: agentID, Principal: id.Principal(principal),
				GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
			}
			grantRepo := &MockGrantRepository{grant: grant}
			provider := newTestProviderService(&MockServiceRepository{service: &model.ThirdpartyOAuth2ProviderEntity{
				ID: serviceID, DisplayName: "Provider", ProtectedResources: []string{resource},
			}})
			expires := now.Add(time.Hour)
			session := &storagedomain.UserSession{
				ID: id.NewSessionID(), Principal: id.Principal(principal), ServiceID: serviceID,
				EncryptedAccessToken: []byte("scope-access-token-canary"), AccessTokenExpiresAt: &expires,
				TokenType: "Bearer", Scope: scopes,
			}
			svc := newServiceForStep9TestWithAuthz(t, keySet, agentRepo, "true")
			svc.providerService = provider
			svc.consentService = consent.NewService(agentRepo, provider, grantRepo, nil, nil, logger, recorder)
			svc.permissionSetService = permissionset.NewPermissionSetService(&MockPermissionSetRepository{
				psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
					permissionSetID: {ID: permissionSetID, ServiceScopes: []storagedomain.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read", "write"}}}},
				},
			}, grantRepo, logger)
			svc.oauth2SessionService = oauth2session.NewOAuth2SessionService(provider, &MockSessionRepository{session: session}, nil, nil,
				&MockEncryption{}, http.DefaultClient, nil, oauth2session.Config{CallbackBaseURL: "https://broker.example"}, logger, recorder)
			svc.ledger = recorder
			response, err := svc.Exchange(context.Background(), NewTokenExchangeRequest(TokenExchangeGrantType, subjectToken, AccessTokenType, assertion, JWTBearerType, resource, ""))
			require.Len(t, store.Events, 1, "only one final exchange fact, without a generic failure companion")
			event := store.Events[0]
			if len(scopes) < 2 {
				require.Nil(t, response)
				var exchangeErr *TokenExchangeError
				require.ErrorAs(t, err, &exchangeErr)
				require.Equal(t, "invalid_grant", exchangeErr.Code())
				require.Equal(t, "https://broker.example/api/third-party/"+serviceID.String()+"/oauth2/authorize", exchangeErr.ErrorURI())
				require.Equal(t, model.BusinessEventTypePrefix+"token-exchange-denied", event.Type)
				require.Equal(t, model.BusinessEventDenied, event.Outcome)
				require.Equal(t, map[string]any{"reason_code": "authorization_failed"}, event.Data)
			} else {
				require.NoError(t, err)
				require.Equal(t, "scope-access-token-canary", response.AccessToken)
				require.Equal(t, model.BusinessEventTypePrefix+"token-exchanged", event.Type)
			}
			require.Equal(t, grant.Principal, *event.Subject)
			require.Equal(t, caller, *event.Actor.ID)
			require.Equal(t, grant.Principal, *event.Actor.OnBehalfOf)
			require.Equal(t, agentID, event.AgentID)
			require.Equal(t, grant.ID, event.GrantID)
			require.Equal(t, serviceID, event.ServiceID)
			require.Equal(t, session.ID, event.SessionID)
			require.Equal(t, []id.PermissionSetID{permissionSetID}, event.PermissionSetIDs)
			require.NotContains(t, fmt.Sprint(event.Wire()), "scope-access-token-canary")
		})
	}
}

type refreshExchangeProviderRepository struct{ *MockServiceRepository }

func (r refreshExchangeProviderRepository) Get(_ context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if r.service.ID != serviceID {
		return nil, ports.ErrNotFound
	}
	return r.service, nil
}

func TestExchangeLedgerDelegatedAutomaticRefreshActor(t *testing.T) {
	key, keySet := generateTestRSAKeySet(t)
	agentID := id.NewAgentID()
	const principal = "ledger-user"
	const gatewayClient = "verified-gateway-client"
	const resource = "https://api.example.com/resource"
	now := time.Now()
	subjectToken := signServiceTestJWT(t, key, map[string]interface{}{
		"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": principal,
		"azp": agentID.String(), "exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	})
	clientAssertion := signServiceTestJWT(t, key, map[string]interface{}{
		"iss": "https://auth.example.com", "aud": "agentic-identity-broker", "sub": gatewayClient,
		"exp": now.Add(time.Hour).Unix(), "iat": now.Unix(),
	})
	for _, failure := range []bool{false, true} {
		outcome := "success"
		if failure {
			outcome = "failure"
		}
		t.Run(outcome, func(t *testing.T) {
			var requests atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if failure {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = io.WriteString(w, `{"error":"invalid_grant"}`)
					return
				}
				_, _ = io.WriteString(w, `{"access_token":"exchange-refresh-canary","token_type":"Bearer","expires_in":3600,"scope":"read"}`)
			}))
			t.Cleanup(upstream.Close)

			serviceID, permissionSetID := id.NewServiceID(), id.NewPermissionSetID()
			entity := &model.ThirdpartyOAuth2ProviderEntity{
				ID: serviceID, ClientID: "public-client", DisplayName: "Provider",
				Secret: model.NewAbsentSecret(), TokenEndpointAuthMethod: model.TokenEndpointAuthMethodNone,
				Endpoints:          model.OAuth2Endpoints{TokenEndpoint: upstream.URL + "/token"},
				ProtectedResources: []string{resource},
			}
			provider := newTestProviderService(refreshExchangeProviderRepository{&MockServiceRepository{service: entity}})
			permissionSets := permissionset.NewPermissionSetService(&MockPermissionSetRepository{
				psMap: map[id.PermissionSetID]*storagedomain.PermissionSet{
					permissionSetID: {ID: permissionSetID, ServiceScopes: []storagedomain.ServiceScope{{ServiceID: serviceID, Scopes: []string{"read"}}}},
				},
			}, &MockGrantRepository{}, slog.Default())
			agentRepo := &singleAgentRepo{agentID: agentID, agent: &storagedomain.Agent{
				ID: agentID, PermissionSets: []storagedomain.AgentPermissionSetEntry{{PermissionSetID: permissionSetID, RequirementType: storagedomain.RequirementTypeOptional}},
			}}
			validUntil := time.Now().Add(time.Hour)
			grant := &storagedomain.UserGrant{
				ID: id.NewGrantID(), AgentID: agentID, Principal: id.Principal(principal), ValidUntil: &validUntil,
				GrantedPermissionSets: []storagedomain.GrantedPermissionSetEntry{{PermissionSetID: permissionSetID, IncludedServiceIDs: []id.ServiceID{serviceID}}},
			}
			grantRepo := &MockGrantRepository{grant: grant}
			store := &ledgerfixture.Store{}
			recorder := store.Recorder(t)
			expired := time.Now().Add(-time.Minute)
			session := &storagedomain.UserSession{
				ID: id.NewSessionID(), Principal: id.Principal(principal), ServiceID: serviceID,
				EncryptedAccessToken: []byte("expired-access-canary"), EncryptedRefreshToken: []byte("refresh-canary"),
				AccessTokenExpiresAt: &expired, TokenType: "Bearer", Scope: []string{"read"},
			}
			sessionRepo := &MockSessionRepository{session: session}
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			sessions := oauth2session.NewOAuth2SessionService(provider, sessionRepo, nil, nil, agentRepo, &MockEncryption{}, upstream.Client(), nil,
				oauth2session.Config{CallbackBaseURL: "https://broker.example"}, logger, recorder, store)
			svc := newServiceForStep9TestWithAuthz(t, keySet, agentRepo, `client_assertion.sub == "verified-gateway-client"`)
			svc.providerService = provider
			svc.oauth2SessionService = sessions
			svc.consentService = consent.NewService(agentRepo, provider, grantRepo, nil, nil, logger, recorder)
			svc.permissionSetService = permissionSets
			svc.ledger = recorder
			holder := security.NewCaptureHolder(security.TransportCapture{})
			ctx := security.WithCaptureHolder(context.Background(), holder)
			request := NewTokenExchangeRequest(TokenExchangeGrantType, subjectToken, AccessTokenType,
				clientAssertion, JWTBearerType, resource, "")
			response, err := svc.Exchange(ctx, request)
			if failure {
				require.Error(t, err)
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				require.Equal(t, principal, response.Principal)
				require.Equal(t, agentID.String(), response.AgentID)
			}
			require.EqualValues(t, 1, requests.Load(), "the upstream refresh endpoint must be used")
			resolved, finalized := holder.Finalized()
			require.True(t, finalized)
			require.Equal(t, principal, resolved.Actor)
			require.Equal(t, gatewayClient, resolved.CallingPeer)

			require.Len(t, store.Events, 2, "refresh and exchange must each store their own fact")
			refresh, exchange := store.Events[0], store.Events[1]
			if failure {
				require.Equal(t, model.BusinessEventTypePrefix+"session-refresh-failed", refresh.Type)
				require.Equal(t, map[string]any{"reason_code": "upstream_rejected"}, refresh.Data)
				require.Equal(t, model.BusinessEventTypePrefix+"token-request-failed", exchange.Type)
			} else {
				require.Equal(t, model.BusinessEventTypePrefix+"session-refreshed", refresh.Type)
				require.Equal(t, model.BusinessEventTypePrefix+"token-exchanged", exchange.Type)
			}
			require.Equal(t, session.Principal, *refresh.Subject)
			require.Equal(t, session.ID, refresh.SessionID)
			require.Equal(t, serviceID, refresh.ServiceID)
			require.Equal(t, "agent", refresh.Actor.Kind)
			require.Equal(t, gatewayClient, *refresh.Actor.ID)
			require.Equal(t, session.Principal, *refresh.Actor.OnBehalfOf)
			require.Equal(t, id.ClientID(gatewayClient), refresh.GatewayClientID)
			require.Equal(t, grant.ID, exchange.GrantID, "the exchange passed grant authorization before refresh")
			require.Equal(t, id.ClientID(gatewayClient), exchange.GatewayClientID)
		})
	}
}
