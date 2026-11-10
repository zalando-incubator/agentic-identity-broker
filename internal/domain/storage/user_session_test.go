package storage

import (
	"encoding/json"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestUserSession_ClientRevision(t *testing.T) {
	for _, revision := range []int64{0, 7, -1} {
		session := &UserSession{ID: id.NewSessionID(), Principal: "alice@example.com", ServiceID: id.NewServiceID(), TokenType: "Bearer", EncryptedAccessToken: []byte("ciphertext"), UpstreamClientRevision: revision}
		if revision < 0 {
			require.Error(t, session.Validate())
		} else {
			require.NoError(t, session.Validate())
		}
		body, err := json.Marshal(session)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "upstream_client_revision")
		assert.NotContains(t, string(body), "UpstreamClientRevision")
		session.UpstreamClientID = new(id.ClientID("reports"))
		summary := NewUserSessionSummary(session, "Reports", 0)
		assert.Equal(t, revision, summary.UpstreamClientRevision)
		require.Equal(t, session.UpstreamClientID, summary.UpstreamClientID)
		*summary.UpstreamClientID = "other"
		assert.Equal(t, id.ClientID("reports"), *session.UpstreamClientID)
		body, err = json.Marshal(summary)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "upstream_client")
		assert.NotContains(t, string(body), "UpstreamClient")
	}
}

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
