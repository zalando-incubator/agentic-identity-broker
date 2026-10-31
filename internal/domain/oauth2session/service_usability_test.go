package oauth2session_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Usability for specs/055-incremental-service-authorization FR-008 and FR-018.
func TestCheckServiceUsability(t *testing.T) {
	hour := time.Hour
	for _, tc := range []struct {
		name            string
		noSession       bool
		accessExpiresIn *time.Duration
		hasRefresh      bool
		refreshExpired  bool
		providerStatus  int
		providerBody    string
		scopes          []string
		effective       []string
		want            servicescope.Usability
		wantCalls       int32
		wantMarker      bool
	}{
		{name: "valid access with sufficient scopes and absent refresh", accessExpiresIn: &hour, scopes: []string{"repo", "user:email"}, effective: []string{"repo", "user:email"}, want: servicescope.UsabilityUsable},
		{name: "valid access with expired refresh", accessExpiresIn: &hour, hasRefresh: true, refreshExpired: true, scopes: []string{"repo"}, effective: []string{"repo"}, want: servicescope.UsabilityUsable},
		{name: "valid access missing an effective scope despite empty configured scopes", accessExpiresIn: &hour, scopes: []string{"repo"}, effective: []string{"repo", "user:email"}, want: servicescope.UsabilityUnusableDefinitive},
		{name: "expired access refreshed successfully", hasRefresh: true, providerStatus: 200, providerBody: `{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`, scopes: []string{"repo"}, effective: []string{"repo"}, want: servicescope.UsabilityUsable, wantCalls: 1},
		{name: "refreshed access missing an effective scope", hasRefresh: true, providerStatus: 200, providerBody: `{"access_token":"fresh","token_type":"Bearer","expires_in":3600}`, scopes: []string{"repo"}, effective: []string{"repo", "user:email"}, want: servicescope.UsabilityUnusableDefinitive, wantCalls: 1},
		{name: "expired access without refresh", scopes: []string{"repo"}, want: servicescope.UsabilityUnusableDefinitive},
		{name: "expired access with expired refresh", hasRefresh: true, refreshExpired: true, scopes: []string{"repo"}, want: servicescope.UsabilityUnusableDefinitive},
		{name: "provider invalid_grant persists the rejection marker", hasRefresh: true, providerStatus: 400, providerBody: `{"error":"invalid_grant"}`, scopes: []string{"repo"}, want: servicescope.UsabilityUnusableDefinitive, wantCalls: 1, wantMarker: true},
		{name: "provider 503 is transient", hasRefresh: true, providerStatus: 503, providerBody: `{"error":"temporarily_unavailable"}`, scopes: []string{"repo"}, want: servicescope.UsabilityUnverifiableTransient, wantCalls: -1},
		{name: "provider 429 is transient", hasRefresh: true, providerStatus: 429, providerBody: `{"error":"slow_down"}`, scopes: []string{"repo"}, want: servicescope.UsabilityUnverifiableTransient, wantCalls: -1},
		{name: "missing session", noSession: true, want: servicescope.UsabilityMissing},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			service, _, sessions, _, _, services := setupServiceWithConfig(t, nil)
			principal := id.Principal("alice@example.com")
			serviceID := id.NewServiceID()
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.providerStatus)
				_, _ = fmt.Fprint(w, tc.providerBody)
			}))
			defer upstream.Close()
			provider := createTestService(serviceID)
			provider.Endpoints.TokenEndpoint = upstream.URL
			require.NoError(t, services.Create(ctx, provider))

			if !tc.noSession {
				enc := newTestEncryption(t)
				encContext := domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext()
				access, err := enc.Encrypt(ctx, []byte("stored-access"), encContext)
				require.NoError(t, err)
				accessExpiry := time.Now().Add(-time.Hour)
				if tc.accessExpiresIn != nil {
					accessExpiry = time.Now().Add(*tc.accessExpiresIn)
				}
				session := &storage.UserSession{
					ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID,
					EncryptedAccessToken: access, TokenType: "Bearer", AccessTokenExpiresAt: &accessExpiry,
					Scope: tc.scopes, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID},
					InitiatedAt: time.Now(), CreatedAt: time.Now(),
				}
				if tc.hasRefresh {
					session.EncryptedRefreshToken, err = enc.Encrypt(ctx, []byte("stored-refresh"), encContext)
					require.NoError(t, err)
					refreshExpiry := time.Now().Add(24 * time.Hour)
					if tc.refreshExpired {
						refreshExpiry = time.Now().Add(-time.Hour)
					}
					session.RefreshTokenExpiresAt = &refreshExpiry
				}
				require.NoError(t, sessions.Create(ctx, session))
			}

			assert.Equal(t, tc.want, service.CheckServiceUsability(ctx, principal, serviceID, tc.effective))
			if tc.wantCalls < 0 {
				assert.Positive(t, calls.Load(), "transient failures follow the existing retry policy")
			} else {
				assert.Equal(t, tc.wantCalls, calls.Load())
			}
			if tc.noSession {
				return
			}
			stored, err := sessions.FindByPrincipalAndService(ctx, principal, serviceID)
			require.NoError(t, err)
			require.NotNil(t, stored)
			if tc.wantMarker {
				require.NotNil(t, stored.RefreshTokenExpiresAt)
				assert.False(t, stored.CanRefresh(), "a definitive rejection is persisted")
			}
			if tc.want == servicescope.UsabilityUnverifiableTransient {
				assert.True(t, stored.CanRefresh(), "a transient failure never persists unusability")
			}
		})
	}
}

func TestServiceSessionSnapshotUsesOneVerifiedRead(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, _, _, services := setupServiceWithConfig(t, nil)
	principal := id.Principal("alice@example.com")
	serviceID := id.NewServiceID()
	require.NoError(t, services.Create(ctx, createTestService(serviceID)))
	enc := newTestEncryption(t)
	access, err := enc.Encrypt(ctx, []byte("stored-access"), domainencryption.NewServiceBranchKeySubject(serviceID).EncryptionContext())
	require.NoError(t, err)
	expiry := time.Now().Add(time.Hour)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, EncryptedAccessToken: access,
		TokenType: "Bearer", AccessTokenExpiresAt: &expiry, Scope: []string{"repo"},
		EncryptionContext: storage.EncryptionContext{ServiceID: serviceID}, InitiatedAt: time.Now(), CreatedAt: time.Now(),
	}))

	snapshot := service.ServiceSessionSnapshot(ctx, principal, serviceID)
	assert.Equal(t, servicescope.SessionSnapshot{Usability: servicescope.UsabilityUsable, Scopes: []string{"repo"}}, snapshot)
	assert.Equal(t, servicescope.UsabilityUnusableDefinitive, snapshot.For([]string{"repo", "user:email"}))

	canceled, cancel := context.WithCancel(ctx)
	cancel()
	assert.Equal(t, servicescope.UsabilityUnverifiableTransient, service.ServiceSessionSnapshot(canceled, principal, serviceID).Usability)
}
