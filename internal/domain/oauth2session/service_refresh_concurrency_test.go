package oauth2session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
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
