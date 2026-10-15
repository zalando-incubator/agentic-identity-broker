package oauth2session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestForceRefreshSession_RejectionStopsValidAccessTokenIssuance(t *testing.T) {
	ctx := context.Background()
	requests := 0
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":"invalid_grant"}`)
	}))
	defer provider.Close()
	expires := time.Now().Add(time.Hour)
	fixture := newLoggedRefreshFixture(t, provider.URL, expires, nil)
	service, sessions := fixture.service, fixture.sessions
	principal, serviceID, session := fixture.principal, fixture.serviceID, fixture.session
	access, refresh := session.EncryptedAccessToken, session.EncryptedRefreshToken
	_, err := service.ForceRefreshSession(ctx, principal, serviceID)
	require.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
	assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
	_, token, err := service.GetValidAccessToken(ctx, principal, serviceID)
	assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
	assert.Empty(t, token)
	persisted, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
	require.NoError(t, err)
	assert.True(t, persisted.RefreshTokenRejected)
	assert.True(t, persisted.RequiresReauthentication())
	assert.Equal(t, session.Scope, persisted.Scope)
	assert.Equal(t, session.TokenType, persisted.TokenType)
	assert.Equal(t, session.InitiatedAt, persisted.InitiatedAt)
	assert.False(t, persisted.UpdatedAt.IsZero())
	assert.Equal(t, access, persisted.EncryptedAccessToken)
	assert.Equal(t, refresh, persisted.EncryptedRefreshToken)
	assert.Equal(t, expires, *persisted.AccessTokenExpiresAt)
	_, err = service.ForceRefreshSession(ctx, principal, serviceID)
	metadata := assertOperationMetadata(t, err, oauth2session.OperationRefresh, oauth2session.DetailRefreshRejected)
	assert.Zero(t, metadata.StatusCode(), "recorded rejection has no new provider response")
	assert.Empty(t, metadata.OAuthCode())
	assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
	assert.Equal(t, 1, requests)
}

func TestGetValidAccessToken_ClassifiesUnusableSessions(t *testing.T) {
	for _, tc := range []struct {
		name        string
		status      int
		contentType string
		body        string
		hasRefresh  bool
		rejected    bool
		oauthError  string
		detail      oauth2session.ErrorDetail
	}{
		{name: "service answers invalid_grant", status: 400, contentType: "application/json", body: `{"error":"invalid_grant"}`, hasRefresh: true, oauthError: "invalid_grant", detail: oauth2session.DetailRefreshRejected},
		{name: "service answers invalid_client", status: 401, contentType: "application/json", body: `{"error":"invalid_client"}`, hasRefresh: true, oauthError: "invalid_client", detail: oauth2session.DetailProviderClientRejected},
		{name: "service answers unauthorized_client", status: 400, contentType: "application/json", body: `{"error":"unauthorized_client"}`, hasRefresh: true, oauthError: "unauthorized_client", detail: oauth2session.DetailProviderClientRejected},
		{name: "service answers invalid_scope", status: 400, contentType: "application/json", body: `{"error":"invalid_scope"}`, hasRefresh: true, oauthError: "invalid_scope", detail: oauth2session.DetailProviderRejected},
		{name: "service answers non-JSON 503", status: 503, contentType: "text/html", body: "<html>unavailable</html>", hasRefresh: true, detail: oauth2session.DetailProviderUnavailable},
		{name: "service answers 429", status: 429, contentType: "application/json", body: `{"error":"slow_down"}`, hasRefresh: true, detail: oauth2session.DetailProviderUnavailable},
		{name: "service answers 500 with invalid_grant", status: 500, contentType: "application/json", body: `{"error":"invalid_grant"}`, hasRefresh: true, oauthError: "invalid_grant", detail: oauth2session.DetailProviderUnavailable},
		{name: "service answers 503 with invalid_client", status: 503, contentType: "application/json", body: `{"error":"invalid_client"}`, hasRefresh: true, oauthError: "invalid_client", detail: oauth2session.DetailProviderUnavailable},
		{name: "service answers 429 with invalid_grant", status: 429, contentType: "application/json", body: `{"error":"invalid_grant"}`, hasRefresh: true, oauthError: "invalid_grant", detail: oauth2session.DetailProviderUnavailable},
		{name: "recorded provider rejection", hasRefresh: true, rejected: true, detail: oauth2session.DetailRefreshRejected},
		{name: "no refresh token stored", detail: oauth2session.DetailAccessTokenExpired},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, services := setupServiceWithConfig(t, nil)
			principal := id.Principal("user@example.com")
			serviceID := id.NewServiceID()
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status == 0 {
					t.Errorf("unexpected refresh request")
					w.WriteHeader(http.StatusInternalServerError)
					return
				}
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			thirdpartyService := createTestService(serviceID)
			thirdpartyService.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, services.Create(ctx, thirdpartyService))

			enc := newTestEncryption(t)
			encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
			access, err := enc.Encrypt(ctx, []byte("expired-access"), encContext)
			require.NoError(t, err)
			expired := time.Now().Add(-time.Hour)
			session := &storage.UserSession{
				ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
				EncryptedAccessToken: access, TokenType: "Bearer", AccessTokenExpiresAt: &expired,
				Scope: []string{"repo"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
				InitiatedAt: time.Now(), CreatedAt: time.Now(),
			}
			if tc.hasRefresh {
				session.EncryptedRefreshToken, err = enc.Encrypt(ctx, []byte("refresh-token"), encContext)
				require.NoError(t, err)
			}
			session.RefreshTokenRejected = tc.rejected
			require.NoError(t, sessions.Create(ctx, session))

			_, _, err = service.GetValidAccessToken(ctx, principal, serviceID)
			metadata := assertOperationMetadata(t, err, oauth2session.OperationRefresh, tc.detail)
			assert.Equal(t, tc.status, metadata.StatusCode())
			wantMetadataCode := tc.oauthError
			if tc.status == 429 && tc.oauthError == "" {
				wantMetadataCode = "unknown"
			}
			assert.Equal(t, wantMetadataCode, metadata.OAuthCode())
			if tc.detail == oauth2session.DetailRefreshRejected {
				assert.ErrorIs(t, err, oauth2session.ErrRefreshRejected)
			} else {
				assert.NotErrorIs(t, err, oauth2session.ErrRefreshRejected)
			}
			if tc.status != 0 {
				assert.ErrorIs(t, err, oauth2session.ErrRefreshFailed)
				var rejected *oauth2session.RefreshRejectedError
				if assert.ErrorAs(t, err, &rejected) {
					assert.Equal(t, tc.status, rejected.StatusCode)
					assert.Equal(t, tc.oauthError, rejected.OAuthError)
				}
			} else {
				assert.ErrorIs(t, err, oauth2session.ErrReauthenticationRequired)
			}
			persisted, findErr := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, findErr)
			assert.Equal(t, tc.detail == oauth2session.DetailRefreshRejected, persisted.RefreshTokenRejected)
			assert.Equal(t, session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
			assert.Equal(t, session.AccessTokenExpiresAt, persisted.AccessTokenExpiresAt)
		})
	}
}

func TestForceRefreshSession_NonterminalProviderFailuresRetainValidToken(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		detail oauth2session.ErrorDetail
	}{
		{"rate limited invalid grant", 429, `{"error":"invalid_grant"}`, oauth2session.DetailProviderUnavailable},
		{"server invalid grant", 503, `{"error":"invalid_grant"}`, oauth2session.DetailProviderUnavailable},
		{"invalid client", 401, `{"error":"invalid_client"}`, oauth2session.DetailProviderClientRejected},
		{"unauthorized client", 400, `{"error":"unauthorized_client"}`, oauth2session.DetailProviderClientRejected},
		{"invalid scope", 400, `{"error":"invalid_scope"}`, oauth2session.DetailProviderRejected},
		{"malformed rejection", 400, `not-json`, oauth2session.DetailProviderRejected},
		{"malformed success", 200, `not-json`, oauth2session.DetailProviderResponseInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			requests := 0
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests++
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			}))
			defer upstream.Close()
			fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(time.Hour), nil)
			_, err := fixture.service.ForceRefreshSession(context.Background(), fixture.principal, fixture.serviceID)
			assertOperationMetadata(t, err, oauth2session.OperationRefresh, tc.detail)
			assert.NotErrorIs(t, err, oauth2session.ErrRefreshRejected)
			persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
			require.NoError(t, err)
			assert.False(t, persisted.RefreshTokenRejected)
			assert.Equal(t, fixture.session.EncryptedAccessToken, persisted.EncryptedAccessToken)
			assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
			assert.Equal(t, fixture.session.AccessTokenExpiresAt, persisted.AccessTokenExpiresAt)
			_, token, err := fixture.service.GetValidAccessToken(context.Background(), fixture.principal, fixture.serviceID)
			require.NoError(t, err)
			assert.Equal(t, "old-access", token)
			assert.Equal(t, 1, requests)
		})
	}
}

func assertOperationMetadata(t *testing.T, err error, operation oauth2session.Operation, detail oauth2session.ErrorDetail) oauth2session.ErrorMetadata {
	t.Helper()
	var failure *oauth2session.OperationError
	require.ErrorAs(t, err, &failure)
	assert.Equal(t, operation, failure.Metadata().Operation())
	assert.Equal(t, detail, failure.Metadata().Detail())
	return failure.Metadata()
}

func TestGetValidAccessToken_RefreshRetainsCredentialWithoutReplacement(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","token_type":"Bearer","expires_in":3600}`)
	}))
	defer upstream.Close()
	fixture := newLoggedRefreshFixture(t, upstream.URL, time.Now().Add(-time.Hour), nil)
	before := time.Now()
	current, token, err := fixture.service.GetValidAccessToken(context.Background(), fixture.principal, fixture.serviceID)
	after := time.Now()
	require.NoError(t, err)
	assert.Equal(t, "new-access", token)
	require.NotNil(t, current.AccessTokenExpiresAt)
	assert.False(t, current.AccessTokenExpiresAt.Before(before.Add(time.Hour)))
	assert.False(t, current.AccessTokenExpiresAt.After(after.Add(time.Hour)))
	persisted, err := fixture.sessions.FindByPrincipalAndService(context.Background(), fixture.principal, fixture.serviceID)
	require.NoError(t, err)
	assert.Equal(t, fixture.session.EncryptedRefreshToken, persisted.EncryptedRefreshToken)
	assert.False(t, persisted.RefreshTokenRejected)
	assert.True(t, persisted.CanRefresh())
	assert.False(t, persisted.RequiresReauthentication())
}
