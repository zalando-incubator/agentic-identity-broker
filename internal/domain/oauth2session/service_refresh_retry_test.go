package oauth2session_test

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
)

func TestRefreshAccessToken_RetryPolicy(t *testing.T) {
	for _, tc := range []struct {
		name         string
		status       int
		retryAfter   string
		body         string
		wantAttempts int32
		wantDetail   oauth2session.ErrorDetail
	}{
		{name: "invalid_grant is not retried", status: http.StatusBadRequest, body: `{"error":"invalid_grant"}`, wantAttempts: 1, wantDetail: oauth2session.DetailRefreshRejected},
		{name: "invalid_client is not retried", status: http.StatusUnauthorized, body: `{"error":"invalid_client"}`, wantAttempts: 1, wantDetail: oauth2session.DetailProviderClientRejected},
		{name: "persistent 503 is retried up to the attempt budget", status: http.StatusServiceUnavailable, body: `{"error":"temporarily_unavailable"}`, wantAttempts: 3, wantDetail: oauth2session.DetailProviderUnavailable},
		{name: "429 is retried within the attempt budget", status: http.StatusTooManyRequests, body: `{"error":"slow_down"}`, wantAttempts: 3, wantDetail: oauth2session.DetailProviderUnavailable},
		{name: "429 with a long Retry-After is not waited for", status: http.StatusTooManyRequests, retryAfter: "120", body: `{"error":"slow_down"}`, wantAttempts: 1, wantDetail: oauth2session.DetailProviderUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, _, _, _, _ := setupServiceWithConfig(t, nil)
			upstream, attempts := jsonTokenEndpointWithHeaders(func(_ url.Values, _ int32) (int, http.Header, string) {
				header := http.Header{}
				if tc.retryAfter != "" {
					header.Set("Retry-After", tc.retryAfter)
				}
				return tc.status, header, tc.body
			})
			defer upstream.Close()
			provider := createTestService(id.NewServiceID())
			provider.Endpoints.TokenEndpoint = upstream.URL

			_, err := service.RefreshAccessToken(ctx, provider, "stored-refresh", nil)
			assertOperationMetadata(t, err, oauth2session.OperationRefresh, tc.wantDetail)
			assert.Equal(t, tc.wantAttempts, attempts.Load())
		})
	}
}

func TestRefreshAccessToken_PreSendDialFailureIsRetried(t *testing.T) {
	ctx := context.Background()
	var attempts atomic.Int32
	upstream, _ := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
		return http.StatusOK, `{"access_token":"after-reset","token_type":"Bearer","expires_in":3600}`
	})
	defer upstream.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if attempts.Add(1) == 1 {
			return nil, &net.OpError{Op: "dial", Net: network, Err: errors.New("connection refused")}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	service, _, _, _, _, _ := setupServiceWithHTTPClient(t, nil, &http.Client{Transport: transport})
	provider := createTestService(id.NewServiceID())
	provider.Endpoints.TokenEndpoint = upstream.URL

	token, err := service.RefreshAccessToken(ctx, provider, "stored-refresh", nil)
	require.NoError(t, err)
	assert.Equal(t, "after-reset", token.AccessToken)
	assert.EqualValues(t, 2, attempts.Load())
}

func TestRefreshAccessToken_PostSendConnectionFailureIsNotRetried(t *testing.T) {
	var attempts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
			return
		}
		if r.Form.Get("refresh_token") != "stored-refresh" {
			t.Error("unexpected refresh token")
		}
		attempts.Add(1)
		conn, _, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = conn.Close()
	}))
	defer upstream.Close()
	service, _, _, _, _, _ := setupServiceWithConfig(t, nil)
	provider := createTestService(id.NewServiceID())
	provider.Endpoints.TokenEndpoint = upstream.URL

	_, err := service.RefreshAccessToken(context.Background(), provider, "stored-refresh", nil)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailProviderUnavailable)
	assert.EqualValues(t, 1, attempts.Load(), "a processed refresh must not be replayed after its response is lost")
}

func TestRefreshAccessToken_RedirectedDialFailureIsNotRetried(t *testing.T) {
	var posts atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		http.Redirect(w, r, "http://redirect.invalid/token", http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == "redirect.invalid:80" {
			return nil, &net.OpError{Op: "dial", Net: network, Err: errors.New("connection refused")}
		}
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	defer transport.CloseIdleConnections()
	service, _, _, _, _, _ := setupServiceWithHTTPClient(t, nil, &http.Client{Transport: transport})
	provider := createTestService(id.NewServiceID())
	provider.Endpoints.TokenEndpoint = upstream.URL
	_, err := service.RefreshAccessToken(context.Background(), provider, "stored-refresh", nil)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailProviderUnavailable)
	assert.EqualValues(t, 1, posts.Load(), "a later dial failure must not replay an already-sent POST")
}

func TestRefreshAccessToken_TimeoutIsNotRetried(t *testing.T) {
	ctx := context.Background()
	release := make(chan struct{})
	upstream, attempts := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
		<-release
		return http.StatusOK, `{"access_token":"late"}`
	})
	defer upstream.Close()
	defer close(release)
	service, _, _, _, _, _ := setupServiceWithHTTPClient(t, nil, &http.Client{Timeout: 50 * time.Millisecond})
	provider := createTestService(id.NewServiceID())
	provider.Endpoints.TokenEndpoint = upstream.URL

	_, err := service.RefreshAccessToken(ctx, provider, "stored-refresh", nil)
	assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailProviderUnavailable)
	assert.EqualValues(t, 1, attempts.Load())
}

func TestGetValidAccessToken_ServesUnexpiredTokenWithoutRefreshToken(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")
	provider := createTestService(serviceID)
	require.NoError(t, providers.Create(ctx, provider))
	seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(10*time.Second), "", []string{"repo"})

	_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "stored-access", token)
}

// 008 US2-S10: a re-connect whose refresh token has already expired must not keep it.
func TestHandleCallback_ReconnectDropsExpiredStoredRefreshToken(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")
	upstream, _ := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
		return http.StatusOK, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"scope":"repo"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))
	seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Hour), "dead-refresh", []string{"repo"})
	stored, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	expired := time.Now().Add(-time.Minute)
	stored.RefreshTokenExpiresAt = &expired
	require.NoError(t, sessions.Create(ctx, stored))

	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: "code", State: flow.StateToken})
	require.NoError(t, err)
	assert.Empty(t, result.Session.EncryptedRefreshToken)
	assert.Nil(t, result.Session.RefreshTokenExpiresAt)
	assert.Equal(t, stored.ID, result.Session.ID, "re-connect keeps the session identity")
}
