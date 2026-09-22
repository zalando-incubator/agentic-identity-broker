package storage

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
)

func TestSigningKeyValidateKeyDomain(t *testing.T) {
	newKey := func(domain KeyDomain) *SigningKey {
		return &SigningKey{
			ID:                  id.NewSigningKeyID(),
			KID:                 id.NewKeyID("kid-1"),
			KeyDomain:           domain,
			Algorithm:           "ES256",
			PrivateKeyEncrypted: []byte("ciphertext"),
			ActivatesAt:         time.Now().UTC(),
			CreatedAt:           time.Now().UTC(),
		}
	}

	tests := []struct {
		name    string
		domain  KeyDomain
		wantErr bool
	}{
		{name: "token-signing domain", domain: KeyDomainTokenSigning},
		{name: "CIMD client-authentication domain", domain: KeyDomainCIMDClientAuthentication},
		{name: "missing domain", wantErr: true},
		{name: "unknown domain", domain: KeyDomain("other"), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := newKey(tt.domain).Validate()
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			assert.NoError(t, err)
		})
	}
}
