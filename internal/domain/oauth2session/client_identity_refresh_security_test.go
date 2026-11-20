package oauth2session

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientIdentityRefreshDenialsHaveOneSafeFailureAcrossTriggers(t *testing.T) {
	for _, trigger := range []string{"manual", "on-demand", "background"} {
		for _, reason := range []model.CredentialSourceReason{model.CredentialSourceReasonIdentityMissing, model.CredentialSourceReasonIdentityMismatch, model.CredentialSourceReasonIdentitySuperseded, model.CredentialSourceReasonReadFailed} {
			t.Run(trigger+"/"+string(reason), func(t *testing.T) {
				reader := backgroundMetrics(t)
				service, _, session, store, logs := newIdentityEnforcementFixture(t, "filesystem")
				switch reason {
				case model.CredentialSourceReasonIdentityMissing:
					session.UpstreamClientID = nil
				case model.CredentialSourceReasonIdentityMismatch:
					session.UpstreamClientID = identityPointer("identity-previous-sentinel")
				case model.CredentialSourceReasonIdentitySuperseded:
					session.UpstreamClientRevision = 2
				default:
					service.credentialFileReader = enforcementCredentialReader{err: model.NewCredentialSourceError(reason)}
				}
				before, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				var calls, decrypts atomic.Int32
				service.encryption = operationEncryption{decrypt: func(_ context.Context, data []byte, _ map[string]string) ([]byte, error) {
					decrypts.Add(1)
					return data, nil
				}}
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				if trigger == "background" {
					submitBackground(t, service.background, context.Background(), session)
					require.NoError(t, service.Close(context.Background()))
				} else {
					if trigger == "manual" {
						_, err = service.ForceRefreshSession(context.Background(), session.Principal, session.ServiceID)
					} else {
						_, _, err = service.GetValidAccessToken(context.Background(), session.Principal, session.ServiceID)
					}
					var source *model.CredentialSourceError
					require.ErrorAs(t, err, &source)
					assert.Equal(t, reason, source.Reason)
				}
				assert.Zero(t, calls.Load())
				assert.Zero(t, decrypts.Load())
				after, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
				operation := "refresh"
				if trigger == "on-demand" {
					operation = "token_acquisition"
				}
				assertIdentityFailureEvent(t, logs, session.ServiceID, operation, reason)
				if trigger == "background" {
					assertBackgroundMetrics(t, reader, map[string]int64{"proactive_refresh_triggered_total": 1, "proactive_refresh_failed_total": 1, "proactive_refresh_dropped_total": 0})
					assert.Equal(t, int64(1), backgroundFailures(t, reader)[session.ServiceID.String()+"/credential_source_unavailable"])
				}
			})
		}
	}
}

func TestClientIdentityRefreshUsesReconnectEvidenceFoundUnderLock(t *testing.T) {
	service, provider, session, store, _ := newIdentityEnforcementFixture(t, "public")
	provider.ClientIdentityRevision = 5
	session.UpstreamClientRevision = 4
	service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, update func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
		return store.withLocked(ctx, principal, serviceID, func(ctx context.Context, current *storage.UserSession) (bool, error) {
			current.UpstreamClientRevision = 5
			current.CallbackID = "winning-reconnect-generation"
			current.EncryptedRefreshToken = []byte("winning-reconnect-refresh")
			return update(ctx, current)
		})
	})
	calls := 0
	service.httpClient.Transport = operationTransport(func(request *http.Request) (*http.Response, error) {
		calls++
		require.NoError(t, request.ParseForm())
		assert.Equal(t, "winning-reconnect-refresh", request.Form.Get("refresh_token"))
		return operationTokenResponse(200, `{"access_token":"current-access","refresh_token":"current-refresh","expires_in":3600}`), nil
	})
	current, refreshed, _, err := service.refreshDueSession(context.Background(), session.Principal, session.ServiceID, time.Now, RefreshTriggerBackground)
	require.NoError(t, err)
	require.NotNil(t, current)
	assert.True(t, refreshed)
	assert.Equal(t, 1, calls)
	assert.Equal(t, int64(5), current.UpstreamClientRevision)
	assert.Equal(t, "winning-reconnect-generation", current.CallbackID)
	assert.Equal(t, identityPointer("identity-current-sentinel"), current.UpstreamClientID)
}

func TestClientIdentityRefreshDenialNeverMutatesAnyServiceVariant(t *testing.T) {
	for _, variant := range identityVariants {
		for _, manual := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/manual=%t", variant, manual), func(t *testing.T) {
				service, _, session, store, _ := newIdentityEnforcementFixture(t, variant)
				session.UpstreamClientRevision = 2
				before, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				calls := 0
				service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return operationTokenResponse(200, `{"access_token":"unexpected"}`), nil
				})
				if manual {
					_, err = service.ForceRefreshSession(context.Background(), session.Principal, session.ServiceID)
				} else {
					_, _, _, err = service.refreshDueSession(context.Background(), session.Principal, session.ServiceID, time.Now, RefreshTriggerBackground)
				}
				var source *model.CredentialSourceError
				require.ErrorAs(t, err, &source)
				assert.Equal(t, model.CredentialSourceReasonIdentitySuperseded, source.Reason)
				assert.Zero(t, calls)
				after, err := store.find(context.Background(), session.Principal, session.ServiceID)
				require.NoError(t, err)
				assert.Equal(t, before, after)
			})
		}
	}
}
