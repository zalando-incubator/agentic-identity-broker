package oauth2session

import (
	"context"
	"errors"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestReconnectWithoutRefreshTokenReplacesConcurrentRotationUnderLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, provider, stored := newOperationTestService(t, io.Discard)
		ctx := context.Background()
		refreshStarted := make(chan struct{})
		releaseRefresh := make(chan struct{})
		service.httpClient.Transport = operationTransport(func(*http.Request) (*http.Response, error) {
			close(refreshStarted)
			<-releaseRefresh
			return operationTokenResponse(http.StatusOK, `{"access_token":"rotated-access","refresh_token":"rotated-refresh","expires_in":3600}`), nil
		})
		var lock sync.Mutex
		var lockCalls atomic.Int32
		reconnectWaiting := make(chan struct{})
		repository := service.refreshRepo.(operationRefreshRepository)
		repository.lock = func(ctx context.Context, principal id.Principal, serviceID id.ServiceID, update func(context.Context, *storage.UserSession) error) (*storage.UserSession, error) {
			if lockCalls.Add(1) == 2 {
				close(reconnectWaiting)
			}
			lock.Lock()
			defer lock.Unlock()
			return repository.ledgerSessionRecords.WithLockedSession(ctx, principal, serviceID, update)
		}
		service.refreshRepo = repository
		refreshed := make(chan error, 1)
		go func() {
			_, _, _, err := service.refreshDueSession(ctx, stored.Principal, provider.ID, time.Now, RefreshTriggerOnDemand)
			refreshed <- err
		}()
		<-refreshStarted
		reconnected := make(chan error, 1)
		go func() {
			_, err := service.createSession(ctx, stored.Principal, provider.ID, &oauth2.Token{AccessToken: "other-account-access", Expiry: time.Now().Add(time.Hour)}, []string{"other-scope"}, &OAuth2StateTokenClaims{SessionID: stored.ID, SessionCallbackID: stored.CallbackID})
			reconnected <- err
		}()
		<-reconnectWaiting
		select {
		case err := <-reconnected:
			t.Fatalf("reconnect completed while refresh held the session lock: %v", err)
		default:
		}
		close(releaseRefresh)
		require.NoError(t, <-refreshed)
		require.NoError(t, <-reconnected)
		stored = repository.session
		assert.Equal(t, "other-account-access", string(stored.EncryptedAccessToken))
		assert.Empty(t, stored.EncryptedRefreshToken)
		assert.Nil(t, stored.RefreshRejectedAt)
		require.NotNil(t, stored.AccessTokenExpiresAt)
		assert.Equal(t, time.Now().Add(time.Hour), *stored.AccessTokenExpiresAt)
		assert.Equal(t, []string{"other-scope"}, stored.Scope)
	})
}

func TestReconnectLockedFailurePreservesCredentialsWithoutUpsert(t *testing.T) {
	for _, tc := range []struct {
		name        string
		stageUpdate bool
	}{
		{"lock acquisition", false},
		{"locked write", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, provider, stored := newOperationTestService(t, io.Discard)
			before := *stored
			cause := errors.New("sentinel-locked-reconnect-failure")
			repository := service.refreshRepo.(operationRefreshRepository)
			if tc.stageUpdate {
				repository.update = func(context.Context, *storage.UserSession, *storage.UserSession) error { return cause }
			} else {
				repository.lock = func(context.Context, id.Principal, id.ServiceID, func(context.Context, *storage.UserSession) error) (*storage.UserSession, error) {
					return nil, cause
				}
			}
			service.refreshRepo = repository
			createCalls := 0
			service.sessionRepo = operationSessionRepository{create: func(_ context.Context, session *storage.UserSession) error {
				createCalls++
				*stored = *session
				return nil
			}}

			session, err := service.createSession(context.Background(), stored.Principal, provider.ID, &oauth2.Token{AccessToken: "other-account-access", Expiry: time.Now().Add(time.Hour)}, []string{"other-scope"}, &OAuth2StateTokenClaims{SessionID: stored.ID, SessionCallbackID: stored.CallbackID})

			assert.Nil(t, session)
			assert.Equal(t, before, *stored, "failed reconnect must preserve the stored session")
			assert.Zero(t, createCalls, "failed locked reconnect must not fall back to an upsert")
			metadata := requireOperationError(t, err, OperationCodeExchange, DetailPersistenceFailed)
			assert.Equal(t, KindInfrastructure, metadata.Kind())
			assert.Equal(t, DependencySessionRepository, metadata.Dependency())
			assert.ErrorIs(t, err, cause)
		})
	}
}

func TestSetSessionTokensClearsRejectionOnlyForReplacementRefreshToken(t *testing.T) {
	for _, replacement := range []string{"", "rotated-refresh"} {
		t.Run("refresh_token="+replacement, func(t *testing.T) {
			service, provider, session := newOperationTestService(t, io.Discard)
			rejectedAt := time.Now().Add(-time.Minute)
			session.RefreshRejectedAt = &rejectedAt
			previousRefresh := append([]byte(nil), session.EncryptedRefreshToken...)

			err := service.setSessionTokens(context.Background(), session, &oauth2.Token{
				AccessToken: "renewed-access", RefreshToken: replacement,
			}, provider.Flavor)

			require.NoError(t, err)
			if replacement == "" {
				assert.Equal(t, &rejectedAt, session.RefreshRejectedAt)
				assert.Equal(t, previousRefresh, session.EncryptedRefreshToken)
				assert.True(t, session.RequiresReauthentication())
			} else {
				assert.Nil(t, session.RefreshRejectedAt)
				assert.NotEqual(t, previousRefresh, session.EncryptedRefreshToken)
				assert.True(t, session.CanRefresh())
			}
		})
	}
}
