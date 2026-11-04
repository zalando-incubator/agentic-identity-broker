package storage

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserSession_RequiresReauthentication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		now := time.Now()
		past, future := now.Add(-time.Second), now.Add(time.Second)
		for _, tc := range []struct {
			name         string
			accessExpiry *time.Time
			rejectedAt   *time.Time
			refresh      []byte
			wantRequired bool
		}{
			{name: "access-only valid", accessExpiry: &future},
			{name: "access-only spent", accessExpiry: &past, wantRequired: true},
			{name: "access expires now", accessExpiry: &now, wantRequired: true},
			{name: "unknown access expiry"},
			{name: "refreshable with spent access", accessExpiry: &past, refresh: []byte("refresh")},
			{name: "refreshable with unknown access expiry", refresh: []byte("refresh")},
			{name: "rejected with spent access", accessExpiry: &past, rejectedAt: &past, refresh: []byte("refresh"), wantRequired: true},
			{name: "rejected with valid access", accessExpiry: &future, rejectedAt: &past, refresh: []byte("refresh"), wantRequired: true},
			{name: "rejected with unknown access expiry", rejectedAt: &past, refresh: []byte("refresh"), wantRequired: true},
		} {
			session := &UserSession{AccessTokenExpiresAt: tc.accessExpiry, RefreshRejectedAt: tc.rejectedAt, EncryptedRefreshToken: tc.refresh}
			assert.Equal(t, tc.wantRequired, session.RequiresReauthentication(), tc.name)
			assert.Equal(t, tc.rejectedAt == nil && len(tc.refresh) > 0, session.CanRefresh(), tc.name)
			summary := NewUserSessionSummary(session, "provider", 0)
			assert.Equal(t, tc.wantRequired, summary.RequiresReauthentication, tc.name)
			assert.Equal(t, tc.wantRequired, summary.IsExpired, tc.name)
		}
	})
}
