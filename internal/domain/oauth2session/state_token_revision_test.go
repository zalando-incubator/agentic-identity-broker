package oauth2session

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestStateTokenRevisionPresence(t *testing.T) {
	for _, claim := range []string{"", `,"upstream_client_revision":0`, `,"upstream_client_revision":9`, `,"upstream_client_revision":-1`} {
		t.Run(claim, func(t *testing.T) {
			original := OAuth2StateTokenClaims{Principal: "person@example.com", ServiceID: id.NewServiceID(), PKCEVerifier: "test-verifier", RedirectURI: "https://broker.example.com/sessions", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute)}
			encoded, err := json.Marshal(original)
			require.NoError(t, err)
			encoded = append(encoded[:len(encoded)-1], []byte(claim+"}")...)
			var claims OAuth2StateTokenClaims
			require.NoError(t, json.Unmarshal(encoded, &claims))
			if claim == `,"upstream_client_revision":-1` {
				assert.Error(t, claims.Validate())
				return
			}
			require.NoError(t, claims.Validate())
			roundtrip, err := json.Marshal(claims)
			require.NoError(t, err)
			var before, after map[string]any
			require.NoError(t, json.Unmarshal(encoded, &before))
			require.NoError(t, json.Unmarshal(roundtrip, &after))
			assert.Equal(t, before, after, "absence is different from an explicitly sealed revision 0")
		})
	}
}
