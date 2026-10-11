package storage

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserSession_HasAccessTokenValidFor(t *testing.T) {
	inFiveSeconds := time.Now().Add(5 * time.Second)
	inOneHour := time.Now().Add(time.Hour)
	expired := time.Now().Add(-time.Second)

	for _, tc := range []struct {
		name      string
		expiresAt *time.Time
		window    time.Duration
		want      bool
	}{
		{name: "no recorded expiry never expires", window: time.Hour, want: true},
		{name: "valid beyond window", expiresAt: &inOneHour, window: 30 * time.Second, want: true},
		{name: "expires inside window", expiresAt: &inFiveSeconds, window: 30 * time.Second, want: false},
		{name: "still valid without window", expiresAt: &inFiveSeconds, want: true},
		{name: "already expired", expiresAt: &expired, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			session := &UserSession{AccessTokenExpiresAt: tc.expiresAt}
			assert.Equal(t, tc.want, session.HasAccessTokenValidFor(tc.window))
		})
	}
}
