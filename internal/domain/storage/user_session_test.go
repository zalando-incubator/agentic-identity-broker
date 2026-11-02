package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserSession_AccessTokenExpiresBy(t *testing.T) {
	expiry := time.Date(2026, time.October, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name   string
		expiry *time.Time
		by     time.Time
		want   bool
	}{
		{name: "no recorded expiry is never due", expiry: nil, by: expiry.Add(time.Hour), want: false},
		{name: "before expiry", expiry: &expiry, by: expiry.Add(-time.Second), want: false},
		{name: "at expiry", expiry: &expiry, by: expiry, want: true},
		{name: "after expiry", expiry: &expiry, by: expiry.Add(time.Second), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &UserSession{AccessTokenExpiresAt: tt.expiry}
			assert.Equal(t, tt.want, session.AccessTokenExpiresBy(tt.by))
		})
	}
}

func TestUserSession_AccessTokenExpiresByMatchesAccessTokenValidity(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	tests := []struct {
		name   string
		expiry *time.Time
	}{
		{name: "no recorded expiry", expiry: nil},
		{name: "expired token", expiry: &past},
		{name: "valid token", expiry: &future},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &UserSession{AccessTokenExpiresAt: tt.expiry}
			assert.Equal(t, !session.HasValidAccessToken(), session.AccessTokenExpiresBy(now))
		})
	}
}

func TestUserSession_RequiresReauthentication(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	future := time.Now().Add(time.Hour)
	tests := []struct {
		name                     string
		expiry                   *time.Time
		hasRefreshToken          bool
		rejected                 bool
		canRefresh               bool
		requiresReauthentication bool
	}{
		{name: "unknown access expiry without refresh", canRefresh: false},
		{name: "valid access without refresh", expiry: &future},
		{name: "expired access without refresh", expiry: &past, requiresReauthentication: true},
		{name: "expired access with refresh", expiry: &past, hasRefreshToken: true, canRefresh: true},
		{name: "valid access with refresh", expiry: &future, hasRefreshToken: true, canRefresh: true},
		{name: "rejected refresh with valid access", expiry: &future, hasRefreshToken: true, rejected: true, requiresReauthentication: true},
		{name: "rejected refresh with unknown access expiry", hasRefreshToken: true, rejected: true, requiresReauthentication: true},
		{name: "rejected without refresh ciphertext", expiry: &future, rejected: true, requiresReauthentication: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &UserSession{AccessTokenExpiresAt: tt.expiry, RefreshTokenRejected: tt.rejected}
			if tt.hasRefreshToken {
				session.EncryptedRefreshToken = []byte("encrypted-refresh")
			}
			assert.Equal(t, tt.canRefresh, session.CanRefresh())
			assert.Equal(t, tt.requiresReauthentication, session.RequiresReauthentication())
			summary := NewUserSessionSummary(session, "Test Service", 0)
			assert.Equal(t, tt.requiresReauthentication, summary.RequiresReauthentication)
			assert.Equal(t, tt.hasRefreshToken, summary.HasRefreshToken)
			assert.Equal(t, !session.HasValidAccessToken(), summary.AccessTokenExpired)
		})
	}
}
