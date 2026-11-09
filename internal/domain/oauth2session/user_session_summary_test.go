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
		ID:                    id.NewSessionID(),
		ServiceID:             id.NewServiceID(),
		TokenType:             "Bearer",
		Scope:                 []string{"repo", "user"},
		InitiatedAt:           now.Add(-1 * time.Hour),
		EncryptedRefreshToken: []byte("refresh"),
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
	assert.False(t, summary.IsExpired)
}

func TestUserSessionSummary_WithRejectedRefreshToken(t *testing.T) {
	now := time.Now()
	rejectedAt := now.Add(-1 * time.Hour)
	validUntil := now.Add(1 * time.Hour)

	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		ServiceID:             id.NewServiceID(),
		EncryptedRefreshToken: []byte("refresh"),
		RefreshRejectedAt:     &rejectedAt,
		AccessTokenExpiresAt:  &validUntil,
	}

	summary := storage.NewUserSessionSummary(session, "GitHub", 0)

	assert.True(t, summary.RequiresReauthentication, "a recorded rejection requires login even while the access token is valid")
	assert.True(t, summary.IsExpired)
	assert.False(t, summary.AccessTokenExpired)
	assert.True(t, summary.HasRefreshToken)
}

func TestUserSessionSummary_WithExpiredAccessToken(t *testing.T) {
	now := time.Now()
	accessExp := now.Add(-1 * time.Hour)

	session := &storage.UserSession{
		ID:                    id.NewSessionID(),
		ServiceID:             id.NewServiceID(),
		AccessTokenExpiresAt:  &accessExp,
		EncryptedRefreshToken: []byte("refresh"),
	}

	summary := storage.NewUserSessionSummary(session, "GitHub", 2)

	assert.True(t, summary.AccessTokenExpired)
	assert.False(t, summary.RequiresReauthentication, "a stored refresh token without a recorded rejection permits a refresh attempt")
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

func TestNewUserSessionDetails(t *testing.T) {
	now := time.Now()
	accessExp := now.Add(time.Hour)
	session := &storage.UserSession{
		ID:                   id.NewSessionID(),
		Principal:            id.Principal("user@example.com"),
		ServiceID:            id.NewServiceID(),
		TokenType:            "Bearer",
		AccessTokenExpiresAt: &accessExp,
		EncryptionContext:    storage.EncryptionContext{ServiceID: id.NewServiceID()},
		CreatedAt:            now.Add(-time.Hour),
		UpdatedAt:            now,
	}

	details := storage.NewUserSessionDetails(session, "GitHub", 1)

	assert.Equal(t, session.ID, details.ID)
	assert.Equal(t, "GitHub", details.ServiceDisplayName)
	assert.Equal(t, 1, details.DependentAgentCount)
	assert.Equal(t, session.Principal, details.Principal)
	assert.Equal(t, &accessExp, details.AccessTokenExpiresAt)
	assert.Equal(t, session.EncryptionContext, details.EncryptionContext)
	assert.Equal(t, session.CreatedAt, details.CreatedAt)
	assert.Equal(t, session.UpdatedAt, details.UpdatedAt)
	assert.False(t, details.RequiresReauthentication)
}
