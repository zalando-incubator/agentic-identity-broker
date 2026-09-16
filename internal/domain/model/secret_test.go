package model

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPlaintextSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		plaintext string
	}{
		{name: "normal secret", plaintext: "mysecret"},
		{name: "long secret", plaintext: strings.Repeat("x", 1024)},
		{name: "special characters", plaintext: "s3cr3t!@#$%^&*()-_=+"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewPlaintextSecret(tt.plaintext)
			assert.True(t, s.IsPlaintext())
			assert.False(t, s.IsEncrypted())

			got, err := s.GetPlaintext()
			require.NoError(t, err)
			assert.Equal(t, tt.plaintext, got)
		})
	}
}

func TestNewEncryptedSecret(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ciphertext []byte
	}{
		{name: "normal ciphertext", ciphertext: []byte{0x01, 0x02, 0x03, 0xAB, 0xCD, 0xEF}},
		{name: "long ciphertext", ciphertext: make([]byte, 256)},
		{name: "single byte", ciphertext: []byte{0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := NewEncryptedSecret(tt.ciphertext)
			assert.True(t, s.IsEncrypted())
			assert.False(t, s.IsPlaintext())

			got, err := s.GetCiphertext()
			require.NoError(t, err)
			assert.Equal(t, tt.ciphertext, got)
		})
	}
}

func TestSecret_GetPlaintext_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		secret    Secret
		wantErrIn string
	}{
		{
			name:      "encrypted state returns error",
			secret:    NewEncryptedSecret([]byte{0x01, 0x02}),
			wantErrIn: "encrypted state",
		},
		{
			name:      "empty plaintext returns error",
			secret:    NewPlaintextSecret(""),
			wantErrIn: "plaintext is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.secret.GetPlaintext()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrIn)
			assert.Empty(t, got)
		})
	}
}

func TestSecret_GetCiphertext_Errors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		secret    Secret
		wantErrIn string
	}{
		{
			name:      "plaintext state returns error",
			secret:    NewPlaintextSecret("mysecret"),
			wantErrIn: "plaintext state",
		},
		{
			name:      "empty ciphertext returns error",
			secret:    NewEncryptedSecret([]byte{}),
			wantErrIn: "ciphertext is empty",
		},
		{
			name:      "nil ciphertext returns error",
			secret:    NewEncryptedSecret(nil),
			wantErrIn: "ciphertext is empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.secret.GetCiphertext()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErrIn)
			assert.Nil(t, got)
		})
	}
}

func TestSecret_Redacted(t *testing.T) {
	t.Parallel()
	t.Run("plaintext state", func(t *testing.T) {
		t.Parallel()
		s := NewPlaintextSecret("mysecret")
		assert.Equal(t, "REDACTED", s.Redacted())
	})

	t.Run("encrypted state", func(t *testing.T) {
		t.Parallel()
		s := NewEncryptedSecret([]byte{0x01, 0x02})
		assert.Equal(t, "REDACTED", s.Redacted())
	})

	t.Run("zero value", func(t *testing.T) {
		t.Parallel()
		var s Secret
		assert.Equal(t, "REDACTED", s.Redacted())
	})
}

func TestSecret_ImmutabilityOnCreate(t *testing.T) {
	t.Parallel()
	t.Run("NewEncryptedSecret does not share backing array", func(t *testing.T) {
		t.Parallel()
		original := []byte{0x01, 0x02, 0x03}
		s := NewEncryptedSecret(original)

		// Mutate the original slice
		original[0] = 0xFF

		got, err := s.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, byte(0x01), got[0], "Secret should not reflect mutation of original slice")
	})

	t.Run("GetCiphertext returns independent copy", func(t *testing.T) {
		t.Parallel()
		s := NewEncryptedSecret([]byte{0x01, 0x02, 0x03})

		ct1, err := s.GetCiphertext()
		require.NoError(t, err)
		ct1[0] = 0xFF

		ct2, err := s.GetCiphertext()
		require.NoError(t, err)
		assert.Equal(t, byte(0x01), ct2[0], "Subsequent GetCiphertext call should return independent copy")
	})
}

func TestSecret_SecurityNoLeakage(t *testing.T) {
	t.Parallel()
	t.Run("error message from GetPlaintext on encrypted secret does not leak ciphertext", func(t *testing.T) {
		t.Parallel()
		sensitiveBytes := []byte("this-should-never-appear-in-error")
		s := NewEncryptedSecret(sensitiveBytes)

		_, err := s.GetPlaintext()
		require.Error(t, err)
		assert.NotContains(t, err.Error(), string(sensitiveBytes))
	})

	t.Run("error message from GetCiphertext on plaintext secret does not leak plaintext", func(t *testing.T) {
		t.Parallel()
		sensitive := "super-secret-password-123"
		s := NewPlaintextSecret(sensitive)

		_, err := s.GetCiphertext()
		require.Error(t, err)
		assert.NotContains(t, err.Error(), sensitive)
	})

	t.Run("Redacted never exposes plaintext value", func(t *testing.T) {
		t.Parallel()
		sensitive := "super-secret-password-123"
		s := NewPlaintextSecret(sensitive)
		assert.NotContains(t, s.Redacted(), sensitive)
	})

	t.Run("Redacted never exposes ciphertext value", func(t *testing.T) {
		t.Parallel()
		s := NewEncryptedSecret([]byte{0xDE, 0xAD, 0xBE, 0xEF})
		// Redacted() must return fixed string, not hex-encoded or base64-encoded ciphertext
		assert.Equal(t, "REDACTED", s.Redacted())
	})
}

func TestSecret_ZeroValue(t *testing.T) {
	t.Parallel()
	// A zero-value Secret is "uninitialized": ciphertext is nil so it looks like plaintext
	// state, but plaintext is "" so GetPlaintext() rejects it. IsPlaintext() returning true
	// does NOT mean the secret is usable — always construct via NewPlaintextSecret or
	// NewEncryptedSecret.
	var s Secret

	t.Run("IsPlaintext returns true for uninitialized secret", func(t *testing.T) {
		t.Parallel()
		assert.True(t, s.IsPlaintext())
		assert.False(t, s.IsEncrypted())
	})

	t.Run("uninitialized secret is not a valid plaintext: GetPlaintext returns error", func(t *testing.T) {
		t.Parallel()
		// IsPlaintext() is true, yet GetPlaintext() fails — this is the "uninitialized"
		// state. Code of the form "if s.IsPlaintext() { use(s.GetPlaintext()) }" is
		// unsound when s was never constructed via NewPlaintextSecret.
		_, err := s.GetPlaintext()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	})
}

func TestSecret_States(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		secret        Secret
		wantAbsent    bool
		wantPlaintext bool
		wantEncrypted bool
	}{
		{
			name:          "explicit absent",
			secret:        NewAbsentSecret(),
			wantAbsent:    true,
			wantPlaintext: false,
			wantEncrypted: false,
		},
		{
			name:          "plaintext",
			secret:        NewPlaintextSecret("mysecret"),
			wantAbsent:    false,
			wantPlaintext: true,
			wantEncrypted: false,
		},
		{
			name:          "encrypted",
			secret:        NewEncryptedSecret([]byte{0x01, 0x02}),
			wantAbsent:    false,
			wantPlaintext: false,
			wantEncrypted: true,
		},
		{
			name:          "zero value",
			secret:        Secret{},
			wantAbsent:    false,
			wantPlaintext: true,
			wantEncrypted: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.wantAbsent, tt.secret.IsAbsent())
			assert.Equal(t, tt.wantPlaintext, tt.secret.IsPlaintext())
			assert.Equal(t, tt.wantEncrypted, tt.secret.IsEncrypted())
		})
	}
}

func TestSecret_AbsentAccessors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		access func(Secret) error
	}{
		{
			name: "GetPlaintext",
			access: func(secret Secret) error {
				_, err := secret.GetPlaintext()
				return err
			},
		},
		{
			name: "GetCiphertext",
			access: func(secret Secret) error {
				_, err := secret.GetCiphertext()
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.access(NewAbsentSecret())
			require.Error(t, err)
			assert.Contains(t, err.Error(), "absent state")
		})
	}
}
