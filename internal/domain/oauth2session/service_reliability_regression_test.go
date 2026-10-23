package oauth2session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Regression suite for recurring third-party "logout" reports.
// Scenario references: specs/008-thirdparty-oauth2-sessions/spec.md (US2-S10)
// and specs/013-token-exchange/spec.md (US1-S7, US1-S8, US5-S6).

func seedExpiringSession(t *testing.T, sessions ports.UserSessionRepository, principal id.Principal, serviceID id.ServiceID, accessExpiresAt time.Time, refreshToken string, scope []string) {
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

func jsonTokenEndpoint(handler func(form url.Values, calls int32) (int, string)) (*httptest.Server, *atomic.Int32) {
	return jsonTokenEndpointWithHeaders(func(form url.Values, calls int32) (int, http.Header, string) {
		status, body := handler(form, calls)
		return status, nil, body
	})
}

func jsonTokenEndpointWithHeaders(handler func(form url.Values, calls int32) (int, http.Header, string)) (*httptest.Server, *atomic.Int32) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status, header, body := handler(r.Form, calls.Add(1))
		for name, values := range header {
			w.Header()[name] = values
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = fmt.Fprint(w, body)
	}))
	return server, &calls
}

func TestHandleCallback_ReconnectWithoutRefreshTokenClearsStoredRefreshToken(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")
	upstream, _ := jsonTokenEndpoint(func(_ url.Values, call int32) (int, string) {
		if call == 1 {
			return http.StatusOK, `{"access_token":"account-a-access","token_type":"Bearer","expires_in":3600,"refresh_token":"account-a-refresh","scope":"repo user"}`
		}
		return http.StatusOK, `{"access_token":"account-b-access","token_type":"Bearer","expires_in":3600,"scope":"repo"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))
	connect := func(code string) (*oauth2session.HandleCallbackResult, error) {
		flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
		require.NoError(t, err)
		return service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: code, State: flow.StateToken})
	}
	first, err := connect("account-a-code")
	require.NoError(t, err)
	refreshExpiry := time.Now().Add(2 * time.Hour)
	first.Session.RefreshTokenExpiresAt = &refreshExpiry
	require.NoError(t, sessions.Create(ctx, first.Session))
	before, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, first.Session.RefreshTokenExpiresAt, before.RefreshTokenExpiresAt)

	result, err := connect("account-b-code")
	require.NoError(t, err)
	assert.Equal(t, before.ID, result.Session.ID)
	assert.Equal(t, before.CreatedAt, result.Session.CreatedAt)
	assert.Equal(t, before.InitiatedAt, result.Session.InitiatedAt)
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Empty(t, persisted.EncryptedRefreshToken)
	assert.Nil(t, persisted.RefreshTokenExpiresAt)
	access, err := service.DecryptAccessToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "account-b-access", access)
	current, access, err := service.GetValidAccessToken(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.Equal(t, "account-b-access", access)
	assert.False(t, current.CanRefresh())
	assert.False(t, current.IsExpired())
	expired := time.Now().Add(-time.Minute)
	persisted.AccessTokenExpiresAt = &expired
	require.NoError(t, sessions.Create(ctx, persisted))
	_, _, err = service.GetValidAccessToken(ctx, principal, serviceID)
	require.ErrorIs(t, err, oauth2session.ErrRefreshNotAvailable)
}

func TestHandleCallback_FirstCompletionWinsUntilANewReconnect(t *testing.T) {
	for _, reconnect := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing_session=%t", reconnect), func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
			serviceID := id.NewServiceID()
			principal := id.Principal("user@example.com")
			upstream, _ := jsonTokenEndpoint(func(form url.Values, _ int32) (int, string) {
				return http.StatusOK, fmt.Sprintf(`{"access_token":%q,"refresh_token":%q,"token_type":"Bearer","expires_in":3600}`, form.Get("code"), form.Get("code")+"-refresh")
			})
			defer upstream.Close()
			provider := createTestService(serviceID)
			provider.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, providers.Create(ctx, provider))
			start := func() string {
				flow, err := service.InitiateOAuth2Flow(ctx, principal, serviceID, "https://example.com/sessions")
				require.NoError(t, err)
				return flow.StateToken
			}
			complete := func(state, code string) {
				_, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: serviceID, Code: code, State: state})
				require.NoError(t, err)
			}
			if reconnect {
				complete(start(), "original-account")
			}
			first, second := start(), start()
			complete(second, "winning-account")
			complete(first, "losing-account")
			stored, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			access, err := service.DecryptAccessToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "winning-account", access)
			refresh, err := service.DecryptRefreshToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "winning-account-refresh", refresh)
			complete(start(), "deliberate-reconnect")
			stored, err = sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			access, err = service.DecryptAccessToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "deliberate-reconnect", access)
			refresh, err = service.DecryptRefreshToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "deliberate-reconnect-refresh", refresh)
		})
	}
}

func TestListUserSessions_ExcludesSpentAccessOnlyConnectionsFromActiveServices(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")
	require.NoError(t, providers.Create(ctx, createTestService(serviceID)))
	seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Minute), "", []string{"repo"})
	summaries, err := service.ListUserSessions(ctx, principal)
	require.NoError(t, err)
	require.Len(t, summaries, 1)
	assert.True(t, summaries[0].IsExpired, "a spent access-only connection must require reauthentication")
	active, err := sessions.ListActiveByPrincipal(ctx, principal)
	require.NoError(t, err)
	assert.Empty(t, active)
	activeIDs, err := sessions.ListActiveServiceIDsByPrincipal(ctx, principal)
	require.NoError(t, err)
	assert.Empty(t, activeIDs)
}

// 013 US1-S7 from specs/013-token-exchange/spec.md.
func TestGetValidAccessToken_RefreshesNearExpiryAccessTokenInBackground(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
	serviceID := id.NewServiceID()
	principal := id.Principal("user@example.com")

	upstream, refreshes := jsonTokenEndpoint(func(form url.Values, _ int32) (int, string) {
		if form.Get("grant_type") != "refresh_token" {
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
	assert.Equal(t, "stored-access", token)
	require.NotNil(t, session.AccessTokenExpiresAt)
	assert.True(t, session.AccessTokenExpiresAt.Before(time.Now().Add(30*time.Second)))
	closeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	require.NoError(t, service.Close(closeCtx))
	assert.EqualValues(t, 1, refreshes.Load())
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	refreshedToken, err := service.DecryptAccessToken(ctx, persisted)
	require.NoError(t, err)
	assert.Equal(t, "refreshed-access", refreshedToken)
	require.NotNil(t, persisted.AccessTokenExpiresAt)
	assert.True(t, persisted.AccessTokenExpiresAt.After(time.Now().Add(30*time.Minute)))
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

	upstream, attempts := jsonTokenEndpoint(func(_ url.Values, call int32) (int, string) {
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

	upstream, _ := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
		return http.StatusOK, `{"access_token":"string-expiry-access","token_type":"Bearer","expires_in":"3600","refresh_token":"next-refresh"}`
	})
	defer upstream.Close()
	provider := createTestService(serviceID)
	provider.Endpoints.TokenEndpoint = upstream.URL

	token, err := service.RefreshAccessToken(ctx, provider, "stored-refresh", nil)
	require.NoError(t, err, "expires_in encoded as a string must be accepted")
	assert.Equal(t, "string-expiry-access", token.AccessToken)
	assert.WithinDuration(t, time.Now().Add(time.Hour), token.Expiry, time.Minute)
}

func TestRefreshAccessToken_RejectsUnusableExpiresIn(t *testing.T) {
	for _, tc := range []struct {
		name     string
		lifetime string
	}{
		{name: "malformed string", lifetime: `"soon"`},
		{name: "object", lifetime: `{"seconds":3600}`},
		{name: "boolean", lifetime: `true`},
		{name: "array", lifetime: `[3600]`},
		{name: "empty string", lifetime: `""`},
		{name: "positive fraction", lifetime: `0.5`},
		{name: "positive fractional string", lifetime: `"0.5"`},
		{name: "positive underflow", lifetime: `1e-1000`},
		{name: "duration overflow", lifetime: `9223372037`},
		{name: "integer overflow", lifetime: `9223372036854775808`},
		{name: "negative integer overflow", lifetime: `-9223372036854775809`},
		{name: "float overflow", lifetime: `1e1000`},
		{name: "not a number", lifetime: `"NaN"`},
		{name: "infinity", lifetime: `"Inf"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service, _, _, _, _, _ := setupServiceWithConfig(t, nil)
			upstream, calls := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
				return http.StatusOK, fmt.Sprintf(`{"access_token":"new-access","token_type":"Bearer","expires_in":%s}`, tc.lifetime)
			})
			defer upstream.Close()
			provider := createTestService(id.NewServiceID())
			provider.Endpoints.TokenEndpoint = upstream.URL

			token, err := service.RefreshAccessToken(context.Background(), provider, "stored-refresh", nil)
			metadata := assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailProviderResponseInvalid)
			assert.Equal(t, http.StatusOK, metadata.StatusCode())
			assert.Nil(t, token, "unusable lifetime must not produce a non-expiring token")
			assert.EqualValues(t, 1, calls.Load(), "invalid lifetime metadata must not be retried")
		})
	}
}

func TestRefreshSession_RFC6749RefreshTokenRetentionAndRotation(t *testing.T) {
	for _, tc := range []struct {
		name         string
		refreshField string
		wantRefresh  string
	}{
		{"omitted refresh token", "", "stored-refresh"},
		{"rotated refresh token", `,"refresh_token":"rotated-refresh"`, "rotated-refresh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
			serviceID := id.NewServiceID()
			principal := id.Principal("user@example.com")
			upstream, calls := jsonTokenEndpoint(func(form url.Values, call int32) (int, string) {
				wantRefresh := "stored-refresh"
				if call > 1 {
					wantRefresh = tc.wantRefresh
				}
				if form.Get("grant_type") != "refresh_token" || form.Get("refresh_token") != wantRefresh {
					return http.StatusBadRequest, `{"error":"invalid_grant"}`
				}
				if call == 1 {
					return http.StatusOK, `{"access_token":"first-access","token_type":"Bearer","expires_in":3600` + tc.refreshField + `}`
				}
				return http.StatusOK, `{"access_token":"second-access","token_type":"Bearer","expires_in":3600}`
			})
			defer upstream.Close()
			provider := createTestService(serviceID)
			provider.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, providers.Create(ctx, provider))
			seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Hour), "stored-refresh", []string{"repo"})
			stored, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			refreshExpiry := time.Now().Add(2 * time.Hour)
			stored.RefreshTokenExpiresAt = &refreshExpiry
			require.NoError(t, sessions.Create(ctx, stored))

			_, access, err := service.GetValidAccessToken(ctx, principal, serviceID)
			require.NoError(t, err)
			assert.Equal(t, "first-access", access)
			_, err = service.ForceRefreshSession(ctx, principal, serviceID)
			require.NoError(t, err)

			persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			access, err = service.DecryptAccessToken(ctx, persisted)
			require.NoError(t, err)
			assert.Equal(t, "second-access", access)
			refresh, err := service.DecryptRefreshToken(ctx, persisted)
			require.NoError(t, err)
			assert.Equal(t, tc.wantRefresh, refresh)
			if tc.refreshField == "" {
				assert.Equal(t, &refreshExpiry, persisted.RefreshTokenExpiresAt)
			} else {
				assert.Nil(t, persisted.RefreshTokenExpiresAt, "the old expiry does not apply to a replacement refresh token")
			}
			assert.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestRefreshSession_AppliesExplicitProviderAuthority(t *testing.T) {
	for _, tc := range []struct {
		name      string
		metadata  string
		wantScope []string
		wantType  string
		flavor    model.OAuth2Flavor
	}{
		{name: "narrowed scope and changed type", metadata: `,"scope":"repo","token_type":"DPoP"`, wantScope: []string{"repo"}, wantType: "DPoP"},
		{name: "explicit empty scope", metadata: `,"scope":""`, wantScope: []string{}, wantType: "Bearer"},
		{name: "omitted metadata", wantScope: []string{"repo", "user"}, wantType: "Bearer"},
		{name: "GitHub scope separator", metadata: `,"scope":" repo , read:user "`, flavor: model.OAuth2FlavorGitHub, wantScope: []string{"repo", "read:user"}, wantType: "Bearer"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, providers := setupServiceWithConfig(t, nil)
			serviceID := id.NewServiceID()
			principal := id.Principal("user@example.com")
			upstream, _ := jsonTokenEndpoint(func(_ url.Values, _ int32) (int, string) {
				return http.StatusOK, `{"access_token":"new-access","expires_in":3600,"refresh_token":"rotated"` + tc.metadata + `}`
			})
			defer upstream.Close()
			provider := createTestService(serviceID)
			provider.Endpoints.TokenEndpoint = upstream.URL
			if tc.flavor != "" {
				provider.Flavor = tc.flavor
			}
			require.NoError(t, providers.Create(ctx, provider))
			seedExpiringSession(t, sessions, principal, serviceID, time.Now().Add(-time.Hour), "stored-refresh", []string{"repo", "user"})
			_, _, err := service.GetValidAccessToken(ctx, principal, serviceID)
			require.NoError(t, err)
			stored, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantScope, stored.Scope)
			assert.Equal(t, tc.wantType, stored.TokenType)
			access, err := service.DecryptAccessToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "new-access", access)
			refresh, err := service.DecryptRefreshToken(ctx, stored)
			require.NoError(t, err)
			assert.Equal(t, "rotated", refresh)
		})
	}
}
