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
