package oauth2session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression suite for recurring third-party "logout" reports.
// Scenario references: specs/008-thirdparty-oauth2-sessions/spec.md (US2-S10, US2-S11)
// and specs/013-token-exchange/spec.md (US1-S7, US1-S8, US5-S6).

func seedExpiringSession(t *testing.T, sessions sessionStore, principal id.Principal, serviceID id.ServiceID, accessExpiresAt time.Time, refreshToken string, scope []string) {
	t.Helper()
	ctx := context.Background()
	enc := newTestEncryption(t)
	encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
	access, err := enc.Encrypt(ctx, []byte("stored-access"), encContext)
	require.NoError(t, err)
	session := &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
		EncryptedAccessToken: access, TokenType: "Bearer", AccessTokenExpiresAt: &accessExpiresAt,
		Scope: scope, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
		InitiatedAt: time.Now(), CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if refreshToken != "" {
		session.EncryptedRefreshToken, err = enc.Encrypt(ctx, []byte(refreshToken), encContext)
		require.NoError(t, err)
	}
	require.NoError(t, sessions.Create(ctx, session))
}

type sessionStore interface {
	Create(ctx context.Context, session *storage.UserSession) error
	FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error)
}

func jsonTokenEndpoint(handler func(form map[string]string, calls int32) (int, string)) (*httptest.Server, *atomic.Int32) {
	return jsonTokenEndpointWithHeaders(func(form map[string]string, calls int32) (int, http.Header, string) {
		status, body := handler(form, calls)
		return status, nil, body
	})
}

func jsonTokenEndpointWithHeaders(handler func(form map[string]string, calls int32) (int, http.Header, string)) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		form := make(map[string]string, len(r.Form))
		for key := range r.Form {
			form[key] = r.Form.Get(key)
		}
		status, header, body := handler(form, calls.Add(1))
		for name, values := range header {
			w.Header()[name] = values
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	return server, &calls
}

// 008 US2-S10: Re-connecting a service whose provider does not re-issue a refresh token
// (Google without prompt=consent, Microsoft, Okta) must keep the stored refresh token.
// Today createSession overwrites it with nil and the session silently loses auto-refresh.
func TestHandleCallback_ReconnectWithoutRefreshTokenPreservesStoredRefreshToken(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, func(c *oauth2session.Config) { c.RetryBaseDelay = 10 * time.Millisecond })
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")

	upstream, _ := jsonTokenEndpoint(func(_ map[string]string, call int32) (int, string) {
		if call == 1 {
			return http.StatusOK, `{"access_token":"first-access","token_type":"Bearer","expires_in":3600,"refresh_token":"long-lived-refresh","scope":"repo user"}`
		}
		return http.StatusOK, `{"access_token":"second-access","token_type":"Bearer","expires_in":3600,"scope":"repo user"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	connect := func(code string) *storage.UserSession {
		flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
		require.NoError(t, err)
		result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: code, State: flow.StateToken})
		require.NoError(t, err)
		return result.Session
	}

	first := connect("code-1")
	firstRefresh, err := service.DecryptRefreshToken(ctx, first)
	require.NoError(t, err)
	require.Equal(t, "long-lived-refresh", firstRefresh)

	connect("code-2")

	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	require.NotNil(t, persisted)
	accessToken, err := service.DecryptAccessToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "second-access", accessToken, "the new access token must replace the old one")
	refreshToken, err := service.DecryptRefreshToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "long-lived-refresh", refreshToken, "re-connect without refresh_token in the provider response must keep the stored refresh token")
	assert.True(t, persisted.CanRefresh(), "session must remain refreshable after re-connect")
}

// 013 US1-S7: An access token that expires within the refresh leeway must be refreshed before use,
// otherwise the exchanged token expires in flight and the agent sees a provider 401.
func TestGetValidAccessToken_RefreshesAccessTokenInsideExpiryLeeway(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")

	upstream, refreshes := jsonTokenEndpoint(func(form map[string]string, _ int32) (int, string) {
		if form["grant_type"] != "refresh_token" {
			return http.StatusBadRequest, `{"error":"unsupported_grant_type"}`
		}
		return http.StatusOK, `{"access_token":"refreshed-access","token_type":"Bearer","expires_in":3600,"refresh_token":"rotated-refresh"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(10*time.Second), "stored-refresh", []string{"repo"})

	session, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-access", token, "a token expiring within the leeway window must be refreshed proactively")
	assert.EqualValues(t, 1, refreshes.Load(), "exactly one refresh must reach the provider")
	require.NotNil(t, session.AccessTokenExpiresAt)
	assert.True(t, session.AccessTokenExpiresAt.After(time.Now().Add(30*time.Minute)), "returned session must carry the refreshed expiry")
}

// 013 US5-S6: A transient provider failure (5xx, 429, network reset) during refresh must be retried
// with backoff like the authorization-code exchange already is. Today a single 503 surfaces as
// server_error to the agent and, through the UI refresh button, as a failed session.
func TestGetValidAccessToken_RetriesTransientProviderFailureDuringRefresh(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, func(c *oauth2session.Config) {
		c.RetryBaseDelay = 10 * time.Millisecond
	})
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")

	upstream, attempts := jsonTokenEndpoint(func(_ map[string]string, call int32) (int, string) {
		if call == 1 {
			return http.StatusServiceUnavailable, `{"error":"temporarily_unavailable"}`
		}
		return http.StatusOK, `{"access_token":"recovered-access","token_type":"Bearer","expires_in":3600,"refresh_token":"stored-refresh"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Hour), "stored-refresh", []string{"repo"})

	_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err, "a single transient provider failure must not fail the refresh")
	assert.Equal(t, "recovered-access", token)
	assert.EqualValues(t, 2, attempts.Load(), "the refresh must be retried after the transient failure")
}

// 013 US1-S8: Providers that serialize expires_in as a JSON string (several enterprise IdPs and
// GitHub Apps proxies) must not break refresh. Today the strict int64 decode fails and the
// session is reported as provider_response_invalid.
func TestRefreshAccessToken_AcceptsStringExpiresIn(t *testing.T) {
	ctx := context.Background()
	service, _, _, _, _, _ := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()

	upstream, _ := jsonTokenEndpoint(func(_ map[string]string, _ int32) (int, string) {
		return http.StatusOK, `{"access_token":"string-expiry-access","token_type":"Bearer","expires_in":"3600","refresh_token":"next-refresh"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL

	token, err := service.RefreshAccessToken(ctx, provider, "stored-refresh")
	require.NoError(t, err, "expires_in encoded as a string must be accepted")
	assert.Equal(t, "string-expiry-access", token.AccessToken)
	assert.WithinDuration(t, time.Now().Add(time.Hour), token.Expiry, time.Minute)
}

// 008 US2-S11: providers advertise refresh-token lifetime via refresh_expires_in
// (Keycloak) or refresh_token_expires_in (GitHub Apps, Auth0). The session must record it so
// the sessions page and consent gate can warn before the refresh token dies.
func TestGetValidAccessToken_RecordsRefreshTokenExpiryFromProviderResponse(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
	}{
		{name: "refresh_expires_in", field: "refresh_expires_in"},
		{name: "refresh_token_expires_in", field: "refresh_token_expires_in"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
			serviceID := id.NewServiceID()
			principal := id.Principal("user@example.com")

			upstream, _ := jsonTokenEndpoint(func(_ map[string]string, _ int32) (int, string) {
				return http.StatusOK, fmt.Sprintf(`{"access_token":"new-access","token_type":"Bearer","expires_in":3600,"refresh_token":"new-refresh","%s":1209600}`, tc.field)
			})
			defer upstream.Close()
			provider := createTestService(serviceID)
			provider.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, providers.Create(ctx, provider))

			seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Hour), "stored-refresh", []string{"repo"})

			session, _, err := service.GetValidAccessToken(ctx, principal, serviceID)
			require.NoError(t, err)
			require.NotNil(t, session.RefreshTokenExpiresAt, "refresh token expiry advertised by the provider must be recorded")
			assert.WithinDuration(t, time.Now().Add(14*24*time.Hour), *session.RefreshTokenExpiresAt, time.Minute)

			persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			require.NotNil(t, persisted.RefreshTokenExpiresAt, "refresh token expiry must be persisted")
		})
	}
}

func TestHandleCallback_RecordsRefreshTokenExpiryFromProviderResponse(t *testing.T) {
	ctx := context.Background()
	service, _, _, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")

	upstream, _ := jsonTokenEndpoint(func(_ map[string]string, _ int32) (int, string) {
		return http.StatusOK, `{"access_token":"access","token_type":"Bearer","expires_in":3600,"refresh_token":"refresh","refresh_expires_in":2592000,"scope":"repo user"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))

	flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
	require.NoError(t, err)
	result, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: "code", State: flow.StateToken})
	require.NoError(t, err)
	require.NotNil(t, result.Session.RefreshTokenExpiresAt, "callback must record the advertised refresh token lifetime")
	assert.WithinDuration(t, time.Now().Add(30*24*time.Hour), *result.Session.RefreshTokenExpiresAt, time.Minute)
}
