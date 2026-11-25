package aws

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestDecrypt_FailureLogCarriesRequestContext(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(telemetry.NewContextHandler(slog.NewJSONHandler(&logs, nil)))
	adapter, _, err := NewEncryptionAdapter(&ports.EncryptionConfig{
		Memory: &ports.MemoryConfig{RawKey: "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="},
	}, logger)
	require.NoError(t, err)
	ctx := security.WithSecurityContext(t.Context(), security.SecurityContext{
		TraceID:     "0123456789abcdef0123456789abcdef",
		Actor:       id.NewPrincipal("operator@example.com"),
		CallingPeer: "test-peer",
	})

	_, err = adapter.Decrypt(ctx, []byte("not a ciphertext"), map[string]string{"service_id": "svc-1"})

	require.Error(t, err)
	var entry map[string]any
	require.NoError(t, json.Unmarshal(logs.Bytes(), &entry))
	assert.Equal(t, "encryption.token.decrypt_failed", entry["event"])
	assert.Equal(t, "0123456789abcdef0123456789abcdef", entry["trace_id"])
	assert.Equal(t, "operator@example.com", entry["actor"])
	assert.Equal(t, "test-peer", entry["calling_peer"])
}

func TestNewEncryptionAdapterRequiresLogger(t *testing.T) {
	adapter, manager, err := NewEncryptionAdapter(&ports.EncryptionConfig{
		Memory: &ports.MemoryConfig{RawKey: "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="},
	}, nil)

	require.EqualError(t, err, "aws.NewEncryptionAdapter: logger must not be nil")
	assert.Nil(t, adapter)
	assert.Nil(t, manager)
}

func TestNewAWSEncryptionRequiresLogger(t *testing.T) {
	adapter, manager, err := NewAWSEncryption("AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA=", "", 0, nil)

	require.EqualError(t, err, "aws.NewAWSEncryption: logger must not be nil")
	assert.Nil(t, adapter)
	assert.Nil(t, manager)
}

func TestUninitializedAdapterReturnsKEKUnavailableWithoutLogger(t *testing.T) {
	adapter := new(AWSAdapter)

	_, encryptErr := adapter.Encrypt(t.Context(), []byte("plaintext"), map[string]string{"service_id": "svc-1"})
	_, decryptErr := adapter.Decrypt(t.Context(), []byte("ciphertext"), map[string]string{"service_id": "svc-1"})

	assert.True(t, isKEKUnavailableError(encryptErr), "encrypt: %v", encryptErr)
	assert.True(t, isKEKUnavailableError(decryptErr), "decrypt: %v", decryptErr)
}
