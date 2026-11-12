package trustanchor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsApprovedAlgorithm(t *testing.T) {
	approved := []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"}
	for _, name := range approved {
		t.Run(name, func(t *testing.T) {
			assert.True(t, IsApprovedAlgorithm(name))
		})
	}

	rejected := []string{"none", "HS256", "HS384", "HS512", "", "es256", "rs256", "eddsa", "Ed25519", "RSA-OAEP"}
	for _, name := range rejected {
		t.Run("rejects "+name, func(t *testing.T) {
			assert.False(t, IsApprovedAlgorithm(name))
		})
	}
}
