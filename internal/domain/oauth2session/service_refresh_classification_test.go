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

func TestGetValidAccessToken_ClassifiesUnusableSessions(t *testing.T) {
	for _, tc := range []struct {
		name           string
		status         int
		contentType    string
		body           string
		hasRefresh     bool
		storedExpired  bool
		refreshExpired bool
		oauthError     string
		detail         oauth2session.ErrorDetail
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
		{name: "stored refresh token expired", hasRefresh: true, storedExpired: true, refreshExpired: true, detail: oauth2session.DetailRefreshTokenExpired},
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
			if tc.storedExpired {
				session.RefreshTokenExpiresAt = &expired
			}
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
				assert.ErrorIs(t, err, oauth2session.ErrSessionExpired)
			}
			if tc.refreshExpired {
				assert.ErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
			} else {
				assert.NotErrorIs(t, err, oauth2session.ErrRefreshTokenExpired)
			}
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
