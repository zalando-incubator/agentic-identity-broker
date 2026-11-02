package storage

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserSession_Usability(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		past, future := now.Add(-time.Second), now.Add(time.Second)
		for _, tc := range []struct {
			name         string
			accessExpiry *time.Time
			rejected     bool
			refresh      []byte
			wantExpired  bool
		}{
			{name: "access-only valid", accessExpiry: &future},
			{name: "access-only spent", accessExpiry: &past, wantExpired: true},
			{name: "access expires now", accessExpiry: &now, wantExpired: true},
			{name: "unknown access expiry"},
			{name: "refreshable", accessExpiry: &past, refresh: []byte("refresh")},
			{name: "rejected refresh", accessExpiry: &past, rejected: true, refresh: []byte("refresh"), wantExpired: true},
			{name: "rejected despite valid access", accessExpiry: &future, rejected: true, refresh: []byte("refresh"), wantExpired: true},
		} {
			session := &UserSession{AccessTokenExpiresAt: tc.accessExpiry, RefreshTokenRejected: tc.rejected, EncryptedRefreshToken: tc.refresh}
			assert.Equal(t, tc.wantExpired, session.RequiresReauthentication(), tc.name)
			assert.Equal(t, tc.wantExpired, NewUserSessionSummary(session, "provider", 0).RequiresReauthentication, tc.name)
		}
	})
}
