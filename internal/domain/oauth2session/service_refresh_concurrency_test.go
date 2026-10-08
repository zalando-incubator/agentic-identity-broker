package oauth2session_test

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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

func TestGetValidAccessToken_ConcurrentRotatingRefresh(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()

	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
	duplicateRequest := make(chan struct{}, 1)
	var refreshes atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if r.Form.Get("refresh_token") != "initial-refresh" {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		if refreshes.Add(1) != 1 {
			select {
			case duplicateRequest <- struct{}{}:
			default:
			}
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		close(firstRequest)
		<-releaseFirst
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"rotated-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
	}))
	defer provider.Close()

	upstream := createTestService(serviceID)
	upstream.Endpoints.TokenEndpoint = provider.URL
	require.NoError(t, providers.Create(ctx, upstream))

	enc := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	encryptedAccess, err := enc.Encrypt(ctx, []byte("expired-access"), encContext)
	require.NoError(t, err)
	encryptedRefresh, err := enc.Encrypt(ctx, []byte("initial-refresh"), encContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  encryptedAccess,
		EncryptedRefreshToken: encryptedRefresh,
		TokenType:             "Bearer",
		AccessTokenExpiresAt:  &expired,
		Scope:                 []string{"repo"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
	}
	require.NoError(t, sessions.Create(ctx, session))

	const callers = 8
	type result struct {
		token string
		err   error
	}
	results := make(chan result, callers)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
		results <- result{token, err}
	}()
	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("first refresh did not reach provider")
	}
	for range callers - 1 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
			results <- result{token, err}
		}()
	}
	select {
	case <-duplicateRequest:
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	workers.Wait()
	close(results)
	for result := range results {
		assert.NoError(t, result.err)
		assert.Equal(t, "rotated-access", result.token)
	}
	assert.EqualValues(t, 1, refreshes.Load(), "rotating provider must only receive one refresh")

	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	refreshToken, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "rotated-refresh", refreshToken)
}

func TestForceRefreshSession_ConcurrentRotatingRefresh(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("user@example.com")
	serviceID := id.NewServiceID()
	firstRequest := make(chan struct{})
	releaseFirst := make(chan struct{})
	duplicateRequest := make(chan struct{}, 1)
	var refreshes atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Form.Get("refresh_token") {
		case "initial-refresh":
			if refreshes.Add(1) != 1 {
				duplicateRequest <- struct{}{}
				http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
				return
			}
			close(firstRequest)
			<-releaseFirst
			_, _ = fmt.Fprint(w, `{"access_token":"first-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`)
		case "rotated-refresh":
			refreshes.Add(1)
			_, _ = fmt.Fprint(w, `{"access_token":"second-access","token_type":"Bearer","expires_in":3600,"refresh_token":"final-refresh"}`)
		default:
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		}
	}))
	defer provider.Close()
	upstream := createTestService(serviceID)
	upstream.Endpoints.TokenEndpoint = provider.URL
	require.NoError(t, providers.Create(ctx, upstream))

	enc := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	encryptedAccess, err := enc.Encrypt(ctx, []byte("initial-access"), encContext)
	require.NoError(t, err)
	encryptedRefresh, err := enc.Encrypt(ctx, []byte("initial-refresh"), encContext)
	require.NoError(t, err)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID:                    id.NewSessionID(),
		Principal:             principal,
		ServiceID:             serviceID,
		EncryptedAccessToken:  encryptedAccess,
		EncryptedRefreshToken: encryptedRefresh,
		TokenType:             "Bearer",
		Scope:                 []string{"repo"},
		EncryptionContext:     storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:           time.Now(),
		CreatedAt:             time.Now(),
	}))

	firstDone := make(chan error, 1)
	secondDone := make(chan error, 1)
	go func() {
		_, err := service.ForceRefreshSession(ctx, principal, serviceID)
		firstDone <- err
	}()
	select {
	case <-firstRequest:
	case <-time.After(5 * time.Second):
		t.Fatal("first force-refresh did not reach provider")
	}
	go func() {
		_, err := service.ForceRefreshSession(ctx, principal, serviceID)
		secondDone <- err
	}()
	select {
	case <-duplicateRequest:
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseFirst)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	assert.EqualValues(t, 2, refreshes.Load())

	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	refreshToken, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "final-refresh", refreshToken)
	_, accessToken, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "second-access", accessToken)
}

type failedCommitRefreshRepo struct {
	underlying ports.UserSessionRefreshRepository
}

func (r failedCommitRefreshRepo) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	return r.underlying.WithLockedSession(ctx, principal, serviceID, func(ctx context.Context, session *storage.UserSession) (bool, error) {
		updated, err := refresh(ctx, session)
		if err != nil || !updated {
			return updated, err
		}
		return false, errors.New("failed to commit refreshed session")
	})
}

func TestGetValidAccessToken_DoesNotAuditRefreshSuccessBeforePersistence(t *testing.T) {
	ctx := context.Background()
	_, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	principal := id.Principal("audit-user@example.com")
	serviceID := id.NewServiceID()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh"}`)
	}))
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	encryption := newTestEncryption(t)
	encryptionContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := encryption.Encrypt(ctx, []byte("old-access"), encryptionContext)
	require.NoError(t, err)
	refresh, err := encryption.Encrypt(ctx, []byte("old-refresh"), encryptionContext)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Hour)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, EncryptedRefreshToken: refresh, TokenType: "Bearer",
		AccessTokenExpiresAt: &expired, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt:       time.Now(), CreatedAt: time.Now(),
	}))
	var logs strings.Builder
	service := oauth2session.NewOAuth2SessionService(
		providers, sessions, failedCommitRefreshRepo{sessions.(ports.UserSessionRefreshRepository)},
		nil, nil, encryption, &http.Client{}, nil, oauth2session.DefaultConfig(), slog.New(slog.NewJSONHandler(&logs, nil)),
	)
	_, _, err = service.GetValidAccessToken(ctx, principal, serviceID)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailPersistenceFailed)
	assert.NotContains(t, logs.String(), "session.oauth2.token_refreshed")
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	currentRefresh, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "old-refresh", currentRefresh)
}
