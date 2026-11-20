package oauth2session

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newIdentityCallbackFixture(t *testing.T, variant string) (*OAuth2SessionService, *model.ThirdpartyOAuth2ProviderEntity, *storage.UserSession, *backgroundStore, *backgroundLog) {
	t.Helper()
	service, provider, session, store, logs := newIdentityEnforcementFixture(t, variant)
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "identity-test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))
	service.jweTokenService = domjwe.New(key)
	service.config.CallbackBaseURL = "https://broker.example.com"
	provider.Endpoints.AuthorizeEndpoint = "https://provider.example/authorize"
	if provider.Discovery.ResourceURL != nil {
		session.ExpectedIssuerURI = provider.IssuerURI
		session.ExpectedResource = provider.AuthorizationParams["resource"]
	}
	return service, provider, session, store, logs
}

func identityCallbackState(t *testing.T, service *OAuth2SessionService, provider *model.ThirdpartyOAuth2ProviderEntity, session *storage.UserSession, identity *id.ClientID, revision *int64) string {
	t.Helper()
	now := time.Now()
	claims := &OAuth2StateTokenClaims{
		Principal: session.Principal, ServiceID: provider.ID, PKCEVerifier: "identity-pkce-sentinel",
		RedirectURI: "https://broker.example.com/sessions", IssuedAt: now, ExpiresAt: now.Add(time.Minute),
		UpstreamClientID: identity, UpstreamClientRevision: revision,
		SessionID: session.ID, SessionCallbackID: session.CallbackID,
	}
	if provider.Discovery.ResourceURL != nil {
		claims.IssuerURI, claims.Resource = provider.IssuerURI, provider.AuthorizationParams["resource"]
	}
	state, err := service.jweTokenService.Encrypt(claims)
	require.NoError(t, err)
	return state
}

func TestClientIdentityEveryAuthorizationVariantSealsExplicitRevision(t *testing.T) {
	for _, variant := range identityVariants {
		for _, revision := range []int64{0, 4} {
			t.Run(fmt.Sprintf("%s/revision=%d", variant, revision), func(t *testing.T) {
				service, provider, session, store, _ := newIdentityCallbackFixture(t, variant)
				provider.ClientIdentityRevision = revision
				flow, err := service.InitiateOAuth2Flow(context.Background(), session.Principal, provider.ID, "https://broker.example.com/sessions")
				require.NoError(t, err)
				require.NotNil(t, flow)
				claims, err := service.ValidateStateToken(flow.StateToken, session.Principal, provider.ID)
				require.NoError(t, err)
				require.NotNil(t, claims.UpstreamClientID, "every variant must seal the initiating effective identity")
				assert.Equal(t, id.ClientID("identity-current-sentinel"), *claims.UpstreamClientID)
				require.NotNil(t, claims.UpstreamClientRevision, "missing is not an explicitly sealed revision 0")
				assert.Equal(t, revision, *claims.UpstreamClientRevision)
				var raw map[string]any
				require.NoError(t, service.jweTokenService.Decrypt(flow.StateToken, &raw))
				assert.Contains(t, raw, "upstream_client_id")
				assert.Equal(t, float64(revision), raw["upstream_client_revision"])
				rejectedAt := time.Now()
				session.RefreshRejectedAt = &rejectedAt
				result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: flow.StateToken})
				require.NoError(t, err)
				require.NotNil(t, result)
				assert.Equal(t, claims.UpstreamClientID, result.Session.UpstreamClientID)
				assert.Equal(t, revision, result.Session.UpstreamClientRevision)
				assert.Nil(t, result.Session.RefreshRejectedAt)
				persisted, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, claims.UpstreamClientID, persisted.UpstreamClientID)
				assert.Equal(t, revision, persisted.UpstreamClientRevision)
				assert.Nil(t, persisted.RefreshRejectedAt)
			})
		}
	}
}

func TestClientIdentityLegacyCallbackClaimsAreRejectedForEveryVariant(t *testing.T) {
	for _, variant := range identityVariants {
		for _, missing := range []string{"identity", "revision", "both"} {
			t.Run(variant+"/missing="+missing, func(t *testing.T) {
				service, provider, session, store, logs := newIdentityCallbackFixture(t, variant)
				provider.ClientIdentityRevision = 0
				identity, revision := identityPointer("identity-current-sentinel"), new(int64)
				if missing != "revision" {
					identity = nil
				}
				if missing != "identity" {
					revision = nil
				}
				state := identityCallbackState(t, service, provider, session, identity, revision)
				before, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				calls := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state})
				assert.Nil(t, result)
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, model.CredentialSourceReasonIdentityMissing, source.Reason)
				assert.Zero(t, calls, "the legacy stored-session exception never permits a legacy callback")
				after, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				assertIdentityFailureEvent(t, logs, provider.ID, "code_exchange", model.CredentialSourceReasonIdentityMissing)
				for _, secret := range []string{state, "identity-code-sentinel", "identity-pkce-sentinel"} {
					assert.NotContains(t, logs.text(), secret)
				}
			})
		}
	}
}

func TestClientIdentityCallbackRejectsSupersededAndRollbackBeforeExchange(t *testing.T) {
	for _, variant := range identityVariants {
		for _, change := range []string{"different identity", "rollback"} {
			t.Run(variant+"/"+change, func(t *testing.T) {
				service, provider, session, store, logs := newIdentityCallbackFixture(t, variant)
				revision := int64(2)
				identity := identityPointer("identity-current-sentinel")
				want := model.CredentialSourceReasonIdentitySuperseded
				if change == "different identity" {
					identity = identityPointer("identity-previous-sentinel")
					want = model.CredentialSourceReasonIdentityMismatch
				}
				state := identityCallbackState(t, service, provider, session, identity, &revision)
				before, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				calls := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state})
				assert.Nil(t, result)
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, want, source.Reason)
				assert.Zero(t, calls)
				after, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				assertIdentityFailureEvent(t, logs, provider.ID, "code_exchange", want)
			})
		}
	}
}

func TestClientIdentityDelayedCallbackCannotReplaceNewReconnectGeneration(t *testing.T) {
	service, provider, session, store, _ := newIdentityCallbackFixture(t, "public")
	revision := provider.ClientIdentityRevision
	oldState := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
			return operationTokenResponse(200, `{"access_token":"delayed-old-access","refresh_token":"delayed-old-refresh","expires_in":3600}`), nil
		}
		return operationTokenResponse(200, `{"access_token":"winning-reconnect-access","refresh_token":"winning-reconnect-refresh","expires_in":3600}`), nil
	})
	finished := make(chan error, 1)
	go func() {
		_, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "delayed-code", State: oldState})
		finished <- err
	}()
	receiveBackground(t, entered)
	releaseOld := sync.OnceFunc(func() { close(release) })
	defer releaseOld()
	provider.ClientIdentityRevision++
	revision = provider.ClientIdentityRevision
	newState := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
	result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "winning-code", State: newState})
	require.NoError(t, err)
	require.NotNil(t, result)
	winner, err := store.find(context.Background(), session.Principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, revision, winner.UpstreamClientRevision)
	releaseOld()
	_ = receiveBackground(t, finished)
	after, err := store.find(context.Background(), session.Principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, winner, after, "neither a delayed response nor old evidence may replace the winning callback generation")
	assert.Equal(t, []byte("winning-reconnect-access"), after.EncryptedAccessToken)
}

func TestClientIdentityStateRevisionRoundtripRetainsExplicitZero(t *testing.T) {
	service, provider, session, _, _ := newIdentityCallbackFixture(t, "public")
	for _, revision := range []int64{0, 8} {
		state := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
		claims, err := service.ValidateStateToken(state, session.Principal, provider.ID)
		require.NoError(t, err)
		encoded, err := json.Marshal(claims)
		require.NoError(t, err)
		var raw map[string]any
		require.NoError(t, json.Unmarshal(encoded, &raw))
		assert.Equal(t, float64(revision), raw["upstream_client_revision"])
	}
}

type identityCreationRepository struct {
	ports.UserSessionRepository
	created   *storage.UserSession
	createErr error
}

func (r *identityCreationRepository) FindByPrincipalAndService(context.Context, id.Principal, id.ServiceID) (*storage.UserSession, error) {
	return r.created, nil
}

func (r *identityCreationRepository) CreateIfAbsent(_ context.Context, session *storage.UserSession) (*storage.UserSession, error) {
	if r.createErr != nil {
		return nil, r.createErr
	}
	if r.created == nil {
		r.created = session
	}
	return r.created, nil
}

func TestClientIdentityFirstCallbackCopiesSealedRevisionForEveryVariant(t *testing.T) {
	for _, variant := range identityVariants {
		t.Run(variant, func(t *testing.T) {
			service, provider, session, _, _ := newIdentityCallbackFixture(t, variant)
			repo := new(identityCreationRepository)
			service.sessionRepo = repo
			service.refreshRepo = operationRefreshRepository(func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				return nil, nil
			})
			initial := *session
			initial.ID = id.SessionID{}
			revision := provider.ClientIdentityRevision
			state := identityCallbackState(t, service, provider, &initial, session.UpstreamClientID, &revision)
			result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.NotNil(t, repo.created)
			assert.Equal(t, revision, repo.created.UpstreamClientRevision)
			assert.Equal(t, session.UpstreamClientID, repo.created.UpstreamClientID)
			assert.False(t, repo.created.ID.IsZero())
		})
	}
}

func TestClientIdentityFailedCurrentCallbackPreservesPreviousSessionEvidence(t *testing.T) {
	for _, variant := range identityVariants {
		for _, declined := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/declined=%t", variant, declined), func(t *testing.T) {
				service, provider, session, store, logs := newIdentityCallbackFixture(t, variant)
				rejectedAt := time.Now()
				session.RefreshRejectedAt = &rejectedAt
				before, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				revision := provider.ClientIdentityRevision
				state := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
				calls := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(400, `{"error":"invalid_grant"}`), nil
				})
				request := &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state}
				if declined {
					request.Error = "access_denied"
				}
				result, err := service.HandleCallback(context.Background(), session.Principal, request)
				assert.Nil(t, result)
				require.Error(t, err)
				if declined {
					assert.Zero(t, calls)
				} else {
					assert.Positive(t, calls, "this case must reach the provider rather than fail identity validation")
				}
				after, err := store.find(context.Background(), session.Principal, provider.ID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				assert.Empty(t, logs.events(t, "session.oauth2.credential_source_failed"))
			})
		}
	}
}

func TestClientIdentityChangeDuringCodeExchangeCannotEstablishCurrentConnection(t *testing.T) {
	for _, rollback := range []bool{false, true} {
		t.Run(fmt.Sprintf("rollback=%t", rollback), func(t *testing.T) {
			service, provider, session, store, logs := newIdentityCallbackFixture(t, "public")
			revision := provider.ClientIdentityRevision
			state := identityCallbackState(t, service, provider, session, session.UpstreamClientID, &revision)
			before, err := store.find(context.Background(), session.Principal, provider.ID)
			require.NoError(t, err)
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				provider.ClientID = "identity-new-during-exchange-sentinel"
				provider.ClientIdentityRevision++
				if rollback {
					provider.ClientID = *session.UpstreamClientID
					provider.ClientIdentityRevision++
				}
				provider.ClientIdentity = identityPointer(provider.ClientID.String())
				return operationTokenResponse(200, `{"access_token":"old-exchange-result","refresh_token":"old-exchange-refresh","expires_in":3600}`), nil
			})
			result, callbackErr := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "old-code", State: state})
			assert.Nil(t, result)
			assert.ErrorIs(t, callbackErr, model.ErrCredentialSourceUnavailable)
			var source *model.CredentialSourceError
			require.ErrorAs(t, callbackErr, &source)
			want := model.CredentialSourceReasonIdentityMismatch
			if rollback {
				want = model.CredentialSourceReasonIdentitySuperseded
			}
			assert.Equal(t, want, source.Reason)
			beforeReconnect, err := store.find(context.Background(), session.Principal, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, before, beforeReconnect)
			assertIdentityFailureEvent(t, logs, provider.ID, "code_exchange", want)
			got, token, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
			assert.Nil(t, got)
			assert.Empty(t, token)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			currentRevision := provider.ClientIdentityRevision
			currentIdentity := identityPointer(provider.ClientID.String())
			fresh := identityCallbackState(t, service, provider, beforeReconnect, currentIdentity, &currentRevision)
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				return operationTokenResponse(200, `{"access_token":"fresh-current-access","refresh_token":"fresh-current-refresh","expires_in":3600}`), nil
			})
			result, err = service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "fresh-code", State: fresh})
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, currentRevision, result.Session.UpstreamClientRevision)
			assert.Equal(t, currentIdentity, result.Session.UpstreamClientID)
			_, token, err = service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
			require.NoError(t, err)
			assert.Equal(t, "fresh-current-access", token)
		})
	}
}

func TestClientIdentityPersistenceDenialHasOneSafeFailure(t *testing.T) {
	for _, initial := range []bool{false, true} {
		t.Run(fmt.Sprintf("initial=%t", initial), func(t *testing.T) {
			service, provider, session, _, logs := newIdentityCallbackFixture(t, "public")
			conflict := storage.NewStorageError("callback", storage.ErrorKindConflict, model.NewCredentialSourceError(model.CredentialSourceReasonIdentitySuperseded), "client identity revision changed")
			service.refreshRepo = operationRefreshRepository(func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				if initial {
					return nil, nil
				}
				return nil, conflict
			})
			baseline := *session
			if initial {
				baseline.ID = id.SessionID{}
				service.sessionRepo = &identityCreationRepository{createErr: conflict}
			}
			state := identityCallbackState(t, service, provider, &baseline, session.UpstreamClientID, &provider.ClientIdentityRevision)
			result, err := service.HandleCallback(context.Background(), session.Principal, &HandleCallbackRequest{ServiceID: provider.ID, Code: "identity-code-sentinel", State: state})
			assert.Nil(t, result)
			assert.ErrorIs(t, err, model.ErrCredentialSourceUnavailable)
			assert.Equal(t, DetailCredentialSourceUnavailable, sessionFailureMetadata(err).Detail())
			assertIdentityFailureEvent(t, logs, provider.ID, "code_exchange", model.CredentialSourceReasonIdentitySuperseded)
		})
	}
}
