package storage_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestSessionAdminSummaryEvaluate(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	at := func(d time.Duration) *time.Time { v := now.Add(d); return &v }

	for name, tc := range map[string]struct {
		access, refresh         *time.Time
		wantExpired, wantAccess bool
	}{
		"no expiries":              {nil, nil, false, false},
		"both in the future":       {at(time.Minute), at(time.Hour), false, false},
		"access expired":           {at(-time.Minute), at(time.Hour), false, true},
		"access expires exactly":   {at(0), nil, false, true},
		"refresh expired":          {nil, at(-time.Minute), true, false},
		"refresh expires exactly":  {nil, at(0), false, false},
		"both expired":             {at(-time.Hour), at(-time.Minute), true, true},
		"refresh just before then": {nil, at(-time.Nanosecond), true, false},
	} {
		t.Run(name, func(t *testing.T) {
			summary := &storage.SessionAdminSummary{AccessTokenExpiresAt: tc.access, RefreshTokenExpiresAt: tc.refresh}
			summary.Evaluate(now)
			assert.Equal(t, tc.wantExpired, summary.IsExpired)
			assert.Equal(t, tc.wantAccess, summary.AccessTokenExpired)
		})
	}
}

func TestSessionAdminSummaryEvaluateNormalizesScope(t *testing.T) {
	summary := &storage.SessionAdminSummary{}
	summary.Evaluate(time.Now())
	assert.NotNil(t, summary.Scope)
	assert.Empty(t, summary.Scope)

	summary = &storage.SessionAdminSummary{Scope: []string{"repo"}}
	summary.Evaluate(time.Now())
	assert.Equal(t, []string{"repo"}, summary.Scope)
}

func TestNewSessionAdminSummaryCarriesNoTokenMaterial(t *testing.T) {
	session := &storage.UserSession{
		EncryptedAccessToken:  []byte("secret-access"),
		EncryptedRefreshToken: []byte("secret-refresh"),
	}
	summary := storage.NewSessionAdminSummary(session, time.Now())
	assert.True(t, summary.HasRefreshToken)

	raw, err := json.Marshal(summary)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret")
	assert.NotContains(t, string(raw), "c2VjcmV0")
}
