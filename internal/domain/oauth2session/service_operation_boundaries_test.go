package oauth2session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type operationReadFunc func([]byte) (int, error)

func (f operationReadFunc) Read(p []byte) (int, error) { return f(p) }

func TestRefreshBodyFailureClassifiesDeadlineAndCallerCancellation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		shared bool
	}{
		{"shared dependency deadline", true},
		{"caller canceled while reading", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			shared := tc.shared
			service, provider, session := newOperationTestService(t, io.Discard)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			cause := context.DeadlineExceeded
			detail, kind := DetailProviderUnavailable, KindInfrastructure
			if !shared {
				cause, detail, kind = context.Canceled, DetailCallerCanceled, KindCanceled
			}
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				body := operationReadFunc(func([]byte) (int, error) {
					if !shared {
						cancel()
					}
					return 0, cause
				})
				return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body)}, nil
			})
			var err error
			if shared {
				_, _, err = service.GetValidAccessToken(ctx, session.Principal, provider.ID)
			} else {
				_, err = service.RefreshAccessToken(ctx, provider, "refresh")
			}
			metadata := requireOperationError(t, err, OperationRefresh, detail)
			assert.Equal(t, kind, metadata.Kind())
			assert.Equal(t, http.StatusOK, metadata.StatusCode())
			assert.ErrorIs(t, err, cause)
			assert.False(t, session.RefreshTokenRejected)
		})
	}
}

func TestRefreshProviderLookupKeepsConfigurationCauses(t *testing.T) {
	for _, tc := range []struct {
		name    string
		missing bool
	}{
		{"provider deleted", true},
		{"provider identity mismatch", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			missing := tc.missing
			service, provider, session := newOperationTestService(t, io.Discard)
			repository := operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				if missing {
					return nil, ports.ErrNotFound
				}
				wrong := *provider
				wrong.ID = id.NewServiceID()
				return &wrong, nil
			}}
			service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(repository, operationEncryption{}, operationBranchKeyManager{}, nil, false, service.logger)
			_, _, err := service.GetValidAccessToken(context.Background(), session.Principal, provider.ID)
			metadata := requireOperationError(t, err, OperationRefresh, DetailConfiguration)
			assert.Equal(t, KindConfiguration, metadata.Kind())
			assert.Equal(t, DependencyProviderRepository, metadata.Dependency())
			assert.False(t, session.RefreshTokenRejected)
			if missing {
				assert.ErrorIs(t, err, ports.ErrNotFound)
				assert.ErrorIs(t, err, ErrServiceNotFound)
			} else {
				assert.ErrorIs(t, err, thirdparty.ErrProviderConfiguration)
			}
		})
	}
}

func TestKnownRefreshRejection_PrecedesProviderMetadataAndDueTime(t *testing.T) {
	for _, trigger := range []RefreshTrigger{RefreshTriggerOnDemand, RefreshTriggerBackground, RefreshTriggerSweep} {
		t.Run(string(trigger), func(t *testing.T) {
			session := backgroundSession(id.NewServiceID(), "known-rejected")
			session.RefreshTokenRejected = true
			service, store, logs := newBackgroundFixture(t, session)
			metadataFailure := errors.New("private-provider-metadata-failure")
			lookups := 0
			service.providerService = thirdparty.NewThirdpartyOAuth2ProviderService(operationProviderRepository{get: func(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
				lookups++
				return nil, metadataFailure
			}}, operationEncryption{}, operationBranchKeyManager{}, nil, false, service.logger)
			service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
				t.Error("known-rejected credentials must not reach the provider")
				return nil, metadataFailure
			})
			locks := 0
			service.refreshRepo = operationRefreshRepository(func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, callback func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
				locks++
				return store.withLocked(ctx, principal, serviceID, callback)
			})
			current, refreshed, _, err := service.refreshLockedSession(context.Background(), session.Principal, session.ServiceID, func() time.Time {
				t.Error("known rejection must precede due-time decisions")
				return time.Now().Add(-time.Hour)
			}, trigger)
			assert.False(t, refreshed)
			if trigger == RefreshTriggerOnDemand {
				metadata := requireOperationError(t, err, OperationRefresh, DetailRefreshRejected)
				assert.Zero(t, metadata.StatusCode())
				assert.Empty(t, metadata.OAuthCode())
				assert.ErrorIs(t, err, ErrRefreshRejected)
				assert.ErrorIs(t, err, ErrReauthenticationRequired)
				assert.NotErrorIs(t, err, metadataFailure)
			} else {
				require.NoError(t, err)
				require.NotNil(t, current)
				assert.True(t, current.RefreshTokenRejected)
			}
			assert.Equal(t, 1, lookups, "metadata stays outside the lock")
			assert.Equal(t, 1, locks)
			stored, err := store.find(context.Background(), session.Principal, session.ServiceID)
			require.NoError(t, err)
			assert.Equal(t, session, stored, "known rejection is an unchanged state")
			pool := newBackgroundRefresher(service, 1)
			continued, err := pool.continueSharedRefresh(context.Background(), session.Principal, session.ServiceID, 5*time.Minute, refreshFlightResult{session: stored, trigger: RefreshTriggerOnDemand})
			require.NoError(t, err)
			assert.False(t, continued.refreshed)
			assert.Equal(t, 1, lookups, "a shared rejected row must not reacquire metadata")
			assert.Equal(t, 1, locks, "a shared rejected row must not reacquire the lock")
			require.NoError(t, pool.Close(context.Background()))
			assert.Empty(t, logs.events(t, "session.oauth2.token_refreshed"))
		})
	}
}
