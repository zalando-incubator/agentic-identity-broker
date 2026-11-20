package tokenexchange

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestExchange_PreservesKnownSubsecondExpiry(t *testing.T) {
	for _, known := range []bool{false, true} {
		name := "unknown expiry"
		if known {
			name = "known subsecond expiry"
		}
		t.Run(name, func(t *testing.T) {
			session := &storage.UserSession{EncryptedAccessToken: []byte("access"), TokenType: "Bearer"}
			repo := &MockSessionRepository{session: session}
			fixture := newExchangeFixture(t, exchangeFixtureConfig{sessionRepo: repo, encryption: &MockEncryption{}})
			session.ServiceID = fixture.serviceID
			if known {
				expiry := time.Now().Add(900 * time.Millisecond)
				session.AccessTokenExpiresAt = &expiry
			}
			response, err := fixture.svc.Exchange(context.Background(), fixture.req)
			require.NoError(t, err)
			body, err := json.Marshal(response)
			require.NoError(t, err)
			var wire map[string]any
			require.NoError(t, json.Unmarshal(body, &wire))
			if known {
				assert.Contains(t, wire, "expires_in")
				assert.Equal(t, float64(0), wire["expires_in"])
			} else {
				assert.NotContains(t, wire, "expires_in")
			}
		})
	}
}
