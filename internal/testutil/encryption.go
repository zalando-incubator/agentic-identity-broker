package testutil

import (
	"log/slog"
	"testing"

	awsencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

// TestKEKBase64 is the single deterministic test key used across all test suites.
// It is a 256-bit random value encoded as base64 and is safe for use in tests only.
// Never use this key in production or commit secrets derived from it.
const TestKEKBase64 = "ASNFZ4mrze/+3LqYdlQyEAEjRWeJq83v/ty6mHZUMhA="

// NewTestEncryptionAdapter returns a real EncryptionPort backed by in-memory AES-256-GCM
// using the deterministic TestKEKBase64 key. It registers a test cleanup and fails the
// test immediately if initialization fails.
//
// Use this in adapter-layer and integration tests that require real roundtrip encryption.
// Domain-layer tests should use hand-rolled or testify mocks instead.
func NewTestEncryptionAdapter(t testing.TB) ports.EncryptionPort {
	t.Helper()
	adapter, _, err := awsencryption.NewAWSEncryption(TestKEKBase64, "", 0, slog.New(slog.DiscardHandler))
	require.NoError(t, err, "testutil: failed to initialize test encryption adapter")
	return adapter
}

// NewPanicTestEncryptionAdapter returns a real EncryptionPort for use in test helpers
// that cannot accept a testing.TB (e.g. package-level setup functions).
// Panics on initialization failure — acceptable because the key is a compile-time constant.
func NewPanicTestEncryptionAdapter() ports.EncryptionPort {
	adapter, _, err := awsencryption.NewAWSEncryption(TestKEKBase64, "", 0, slog.New(slog.DiscardHandler))
	if err != nil {
		panic("testutil: failed to initialize test encryption adapter: " + err.Error())
	}
	return adapter
}
