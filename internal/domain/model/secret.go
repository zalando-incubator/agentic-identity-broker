package model

import "errors"

// Secret is a value object representing an OAuth2 client secret in one of three mutually
// exclusive states: absent, plaintext, or encrypted. The state design enforces at the type
// level that plaintext secrets are never persisted and encrypted bytes are never exposed
// as strings.
//
// State is determined by the invariant: absent is explicit; otherwise, ciphertext == nil
// means plaintext state and ciphertext != nil means encrypted state.
// A plaintext Secret is created via NewPlaintextSecret and holds the raw secret string.
// An encrypted Secret is created via NewEncryptedSecret and holds opaque ciphertext bytes.
// State transitions produce new Secret instances — Secret is immutable.
//
// Zero value: var s Secret has ciphertext == nil, so IsPlaintext() returns true and
// IsEncrypted() returns false. However, GetPlaintext() rejects it because plaintext is "".
// This zero value is an "uninitialized" state that is distinct from an explicitly absent
// secret and a valid plaintext secret created with NewPlaintextSecret. IsPlaintext()
// returning true does NOT guarantee that GetPlaintext() will succeed; it only means the
// secret has not been encrypted. Always construct Secret values via NewAbsentSecret,
// NewPlaintextSecret, or NewEncryptedSecret.
type Secret struct {
	plaintext  string
	ciphertext []byte
	absent     bool
}

// NewAbsentSecret creates a Secret in its explicit absent state.
func NewAbsentSecret() Secret {
	return Secret{absent: true}
}

// NewPlaintextSecret creates a Secret in plaintext state.
// The plaintext value can be any string, including empty (validation happens at domain entity level).
func NewPlaintextSecret(plaintext string) Secret {
	return Secret{
		plaintext:  plaintext,
		ciphertext: nil,
	}
}

// NewEncryptedSecret creates a Secret in encrypted state.
// The ciphertext holds opaque encrypted bytes; use GetCiphertext() to retrieve them.
func NewEncryptedSecret(ciphertext []byte) Secret {
	ct := make([]byte, len(ciphertext))
	copy(ct, ciphertext)
	return Secret{
		plaintext:  "",
		ciphertext: ct,
	}
}

// GetPlaintext returns the plaintext value of the secret.
// It returns an error when the secret is absent, encrypted, or empty.
func (s Secret) GetPlaintext() (string, error) {
	if s.absent {
		return "", errors.New("secret is in absent state: absent secret has no plaintext")
	}
	if s.ciphertext != nil {
		return "", errors.New("secret is in encrypted state: ciphertext cannot be converted to plaintext")
	}
	if s.plaintext == "" {
		return "", errors.New("secret plaintext is empty")
	}
	return s.plaintext, nil
}

// GetCiphertext returns the ciphertext bytes of the secret.
// It returns an error when the secret is absent, plaintext, or empty.
func (s Secret) GetCiphertext() ([]byte, error) {
	if s.absent {
		return nil, errors.New("secret is in absent state: absent secret has no ciphertext")
	}
	if s.ciphertext == nil {
		return nil, errors.New("secret is in plaintext state: plaintext cannot be converted to ciphertext")
	}
	if len(s.ciphertext) == 0 {
		return nil, errors.New("secret ciphertext is empty")
	}
	ct := make([]byte, len(s.ciphertext))
	copy(ct, s.ciphertext)
	return ct, nil
}

// IsAbsent reports whether the secret is in its explicit absent state.
func (s Secret) IsAbsent() bool {
	return s.absent
}

// IsEncrypted reports whether the secret is in encrypted state.
func (s Secret) IsEncrypted() bool {
	return !s.absent && s.ciphertext != nil
}

// IsPlaintext reports whether the secret is in plaintext state.
func (s Secret) IsPlaintext() bool {
	return !s.absent && s.ciphertext == nil
}

// Redacted returns the string "REDACTED" regardless of the secret's state.
// Use this for API responses and log output to avoid exposing the secret value.
func (s Secret) Redacted() string {
	return "REDACTED"
}
