package oauth2session_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type pausedCommitRefreshRepo struct {
	underlying ports.UserSessionRefreshRepository
	refreshed  chan struct{}
	release    chan struct{}
}

func (r pausedCommitRefreshRepo) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	return r.underlying.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
		updated, err := refresh(ctx, session)
		if err == nil && updated {
			close(r.refreshed)
			<-r.release
		}
		return updated, err
	})
}

func TestGetValidAccessToken_LeaderCancellationDoesNotLoseRotatedToken(t *testing.T) {
	ctx := context.Background()
	_, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("canceled-leader@example.com")
	serviceID := id.NewServiceID()
	var refreshes atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if refreshes.Add(1) != 1 || r.Form.Get("refresh_token") != "initial-refresh" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
	}))
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	encryption := newTestEncryption(t)
	encryptionContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("expired-access"), encryptionContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("initial-refresh"), encryptionContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       time.Now(), CreatedAt: time.Now(),
	}))
	paused := pausedCommitRefreshRepo{underlying: sessions.(ports.UserSessionRefreshRepository), refreshed: make(chan struct{}), release: make(chan struct{})}
	defer func() {
		select {
		case <-paused.release:
		default:
			close(paused.release)
		}
	}()
	service := oauth2session.NewOAuth2SessionService(
		providers, sessions, paused, nil, nil, encryption, &http.Client{Timeout: time.Second}, nil, oauth2session.DefaultConfig(), slog.Default(),
	)
	leaderCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	leaderDone := make(chan error, 1)
	go func() {
		_, _, err := service.GetValidAccessToken(leaderCtx, principal, serviceID)
		leaderDone <- err
	}()
	select {
	case <-paused.refreshed:
	case <-time.After(3 * time.Second):
		t.Fatal("provider did not rotate token")
	}
	type result struct {
		token string
		err   error
	}
	waiterDone := make(chan result, 1)
	go func() {
		_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
		waiterDone <- result{token, err}
	}()
	cancel()
	select {
	case err := <-leaderDone:
		assert.ErrorIs(t, err, context.Canceled)
		assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailCallerCanceled)
	case <-time.After(500 * time.Millisecond):
		t.Error("canceled request remained blocked on the shared refresh")
	}
	close(paused.release)
	select {
	case result := <-waiterDone:
		require.NoError(t, result.err)
		assert.Equal(t, "rotated-access", result.token)
	case <-time.After(3 * time.Second):
		t.Fatal("surviving request did not finish")
	}
	assert.EqualValues(t, 1, refreshes.Load(), "rotating provider must see one refresh")
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	rotated, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "rotated-refresh", rotated)
}
