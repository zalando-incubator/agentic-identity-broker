package storage

import (
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// KeyDomain limits a signing key to one cryptographic trust boundary.
type KeyDomain string

const (
	// KeyDomainTokenSigning identifies keys that sign broker-issued access tokens.
	KeyDomainTokenSigning KeyDomain = "token_signing"
	// KeyDomainCIMDClientAuthentication identifies keys that sign outbound CIMD client assertions.
	KeyDomainCIMDClientAuthentication KeyDomain = "cimd_client_authentication"
)

// Validate reports whether the key domain is supported by the broker.
func (d KeyDomain) Validate() error {
	switch d {
	case KeyDomainTokenSigning, KeyDomainCIMDClientAuthentication:
		return nil
	default:
		return fmt.Errorf("unsupported key domain: %q", d)
	}
}

// SigningKey represents an asymmetric key pair used to sign locally-issued JWT access tokens.
// Exactly one key is marked is_current at any time. Private material is stored PEM-encoded
// and encrypted via EncryptionPort. Keys remain in JWKS until explicitly removed.
//
// ActivatesAt controls when the key becomes eligible for token signing. New keys are published
// to the JWKS endpoint immediately but must wait until ActivatesAt before they sign tokens,
// allowing JWKS caches to expire and learn about the new key first.
type SigningKey struct {
	ID                  id.SigningKeyID `json:"id" db:"id"`
	KID                 id.KeyID        `json:"kid" db:"kid"`
	KeyDomain           KeyDomain       `json:"-" db:"key_domain"`
	Algorithm           string          `json:"algorithm" db:"algorithm"`
	PrivateKeyEncrypted []byte          `json:"-" db:"private_key_encrypted"` // Never serialize
	IsCurrent           bool            `json:"is_current" db:"is_current"`
	ActivatesAt         time.Time       `json:"activates_at" db:"activates_at"`
	CreatedAt           time.Time       `json:"created_at" db:"created_at"`
	RemovedAt           *time.Time      `json:"removed_at,omitempty" db:"removed_at"`
}

// Validate validates the SigningKey fields.
func (k *SigningKey) Validate() error {
	if k.KID.IsZero() {
		return NewStorageError("SigningKey.Validate", ErrorKindValidation, nil, "kid is required")
	}
	if err := k.KeyDomain.Validate(); err != nil {
		return NewStorageError("SigningKey.Validate", ErrorKindValidation, err, "key_domain is invalid")
	}
	if k.Algorithm == "" {
		return NewStorageError("SigningKey.Validate", ErrorKindValidation, nil, "algorithm is required")
	}
	if len(k.PrivateKeyEncrypted) == 0 {
		return NewStorageError("SigningKey.Validate", ErrorKindValidation, nil, "private_key_encrypted is required")
	}
	return nil
}
