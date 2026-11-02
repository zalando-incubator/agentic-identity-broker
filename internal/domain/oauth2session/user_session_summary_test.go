package oauth2session_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestNewUserSessionSummary(t *testing.T) {
	now := time.Now()

	session := &storage.UserSession{
		ID:          id.NewSessionID(),
		ServiceID:   id.NewServiceID(),
		TokenType:   "Bearer",
		Scope:       []string{"repo", "user"},
		InitiatedAt: now.Add(-1 * time.Hour),
	}

	summary := storage.NewUserSessionSummary(session, "GitHub", 3)

	assert.Equal(t, session.ID, summary.ID)
	assert.Equal(t, session.ServiceID, summary.ServiceID)
	assert.Equal(t, "GitHub", summary.ServiceDisplayName)
	assert.Equal(t, "Bearer", summary.TokenType)
	assert.Equal(t, []string{"repo", "user"}, summary.Scope)
	assert.Equal(t, 3, summary.DependentAgentCount)
	assert.True(t, summary.IsEncrypted)
	assert.False(t, summary.RequiresReauthentication)
}

func TestUserSessionSummary_WithRejectedRefreshToken(t *testing.T) {
	session := &storage.UserSession{
		ID: id.NewSessionID(), ServiceID: id.NewServiceID(),
		AccessTokenExpiresAt:  ptrTime(time.Now().Add(time.Hour)),
		EncryptedRefreshToken: []byte("ciphertext"), RefreshTokenRejected: true,
	}
	summary := storage.NewUserSessionSummary(session, "GitHub", 0)
	assert.True(t, summary.RequiresReauthentication)
	assert.True(t, summary.HasRefreshToken, "rejection does not erase the stored credential")
	assert.False(t, summary.AccessTokenExpired)
}

func TestUserSessionSummary_WithExpiredAccessToken(t *testing.T) {
	now := time.Now()
	accessExp := now.Add(-1 * time.Hour) // Expired access token

	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		ServiceID:             id.NewServiceID(),
		AccessTokenExpiresAt:  &accessExp,
		EncryptedRefreshToken: []byte("ciphertext"),
	}

	summary := storage.NewUserSessionSummary(session, "GitHub", 2)

	assert.True(t, summary.AccessTokenExpired)
	assert.False(t, summary.RequiresReauthentication)
}

func TestUserSessionSummary_ExpiredAccessWithoutRefreshRequiresReauthentication(t *testing.T) {
	session := &storage.UserSession{AccessTokenExpiresAt: ptrTime(time.Now().Add(-time.Hour))}
	summary := storage.NewUserSessionSummary(session, "GitHub", 0)
	assert.True(t, summary.AccessTokenExpired)
	assert.True(t, summary.RequiresReauthentication)
	assert.False(t, summary.HasRefreshToken)
}

func TestUserSessionSummary_HasRefreshToken(t *testing.T) {
	t.Run("has_refresh_token is true when EncryptedRefreshToken is set", func(t *testing.T) {
		session := &storage.UserSession{
			ID:                    id.NewSessionID(),
			ServiceID:             id.NewServiceID(),
			EncryptedRefreshToken: []byte("encrypted-refresh-token"),
		}
		summary := storage.NewUserSessionSummary(session, "GitHub", 0)
		assert.True(t, summary.HasRefreshToken)
	})

	t.Run("has_refresh_token is false when EncryptedRefreshToken is empty", func(t *testing.T) {
		session := &storage.UserSession{
			ID:                    id.NewSessionID(),
			ServiceID:             id.NewServiceID(),
			EncryptedRefreshToken: nil,
		}
		summary := storage.NewUserSessionSummary(session, "GitHub", 0)
		assert.False(t, summary.HasRefreshToken)
	})
}
