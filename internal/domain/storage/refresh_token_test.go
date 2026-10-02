package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/require"
)

func refreshModelToken() *RefreshToken {
	issued := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	return &RefreshToken{Signature: strings.Repeat("a", 64), SessionID: id.NewRefreshSessionID(), IssuedAt: issued, ExpiresAt: issued.Add(720 * time.Hour)}
}

func TestRefreshTokenRejectsInvalidIssuance(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RefreshToken)
	}{
		{"raw credential", func(token *RefreshToken) { token.Signature = "opaque-refresh-credential" }},
		{"uppercase signature", func(token *RefreshToken) { token.Signature = strings.Repeat("A", 64) }},
		{"zero root", func(token *RefreshToken) { token.SessionID = id.RefreshSessionID{} }},
		{"zero issued time", func(token *RefreshToken) { token.IssuedAt = time.Time{} }},
		{"zero expiry", func(token *RefreshToken) { token.ExpiresAt = time.Time{} }},
		{"expiry equals issuance", func(token *RefreshToken) { token.ExpiresAt = token.IssuedAt }},
		{"expiry precedes issuance", func(token *RefreshToken) { token.ExpiresAt = token.IssuedAt.Add(-time.Second) }},
		{"used before issuance", func(token *RefreshToken) { token.UsedAt = ptr.To(token.IssuedAt.Add(-time.Second)) }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			token := refreshModelToken()
			test.change(token)
			require.Error(t, token.Validate())
		})
	}
}

func TestRefreshTokenRejectsLineageAndExpiryRewrites(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RefreshToken)
	}{
		{"signature", func(token *RefreshToken) { token.Signature = strings.Repeat("b", 64) }},
		{"root", func(token *RefreshToken) { token.SessionID = id.NewRefreshSessionID() }},
		{"issued time", func(token *RefreshToken) { token.IssuedAt = token.IssuedAt.Add(time.Second) }},
		{"extended issued expiry", func(token *RefreshToken) { token.ExpiresAt = token.ExpiresAt.Add(time.Hour) }},
		{"replaced first consumption", func(token *RefreshToken) { token.UsedAt = ptr.To(token.UsedAt.Add(time.Second)) }},
		{"cleared consumption", func(token *RefreshToken) { token.UsedAt = nil }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			previous := refreshModelToken()
			previous.UsedAt = ptr.To(previous.IssuedAt.Add(time.Hour))
			next := *previous
			test.change(&next)
			require.Error(t, next.ValidateTransition(previous))
		})
	}
}
