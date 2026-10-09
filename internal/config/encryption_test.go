package config

import (
	"context"
	"testing"
	"time"

	aws "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// Backend Configuration Tests
// ============================================================================

// TestEncryptionConfigBackend_AWSKMSBackend verifies AWS KMS backend configuration parses correctly
func TestEncryptionConfigBackend_AWSKMSBackend(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: &ports.AWSKMSConfig{
				KeyARN:            "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				DynamoDBTableName: "IdentityBrokerEncryptionBranchKeys",
				BranchKeyTTL:      "1h",
				DynamoDBRegion:    "us-east-1",
				DynamoDBTimeout:   "5s",
			},
			Memory: nil, // Only AWS KMS backend should be set
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for AWS KMS backend, got error: %v", err)
	}

	// Verify that exactly one backend is configured
	if cfg.Encryption.AWSKMS == nil {
		t.Error("Expected AWSKMS backend to be configured")
	}
	if cfg.Encryption.Memory != nil {
		t.Error("Expected Memory backend to be nil when AWS KMS is configured")
	}
}

// TestEncryptionConfigBackend_MemoryBackend verifies Memory backend configuration parses correctly
func TestEncryptionConfigBackend_MemoryBackend(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil, // Only Memory backend should be set
			Memory: &ports.MemoryConfig{
				RawKey: generateBase64EncodedString(t, 32),
			},
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for Memory backend, got error: %v", err)
	}

	// Verify that exactly one backend is configured
	if cfg.Encryption.Memory == nil {
		t.Error("Expected Memory backend to be configured")
	}
	if cfg.Encryption.AWSKMS != nil {
		t.Error("Expected AWSKMS backend to be nil when Memory is configured")
	}
}

// TestEncryptionConfigBackend_BothBackends verifies validation fails when both backends are specified
func TestEncryptionConfigBackend_BothBackends(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: &ports.AWSKMSConfig{
				KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
			},
			Memory: &ports.MemoryConfig{
				RawKey: generateBase64EncodedString(t, 32),
			},
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error when both AWS KMS and Memory backends are specified, got nil")
	}

	// Verify error contains appropriate message about exactly one backend
	if cerr, ok := err.(*config.ConfigError); ok {
		if cerr.Field != "encryption" {
			t.Errorf("Expected error field to be 'encryption', got: %s", cerr.Field)
		}
	} else {
		t.Errorf("Expected ConfigError, got %T", err)
	}
}

// TestEncryptionConfigBackend_NoBackends verifies validation fails when neither backend is specified
func TestEncryptionConfigBackend_NoBackends(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil,
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error when neither AWS KMS nor Memory backend is specified, got nil")
	}

	// Verify error contains appropriate message about exactly one backend
	if cerr, ok := err.(*config.ConfigError); ok {
		if cerr.Field != "encryption" {
			t.Errorf("Expected error field to be 'encryption', got: %s", cerr.Field)
		}
	} else {
		t.Errorf("Expected ConfigError, got %T", err)
	}
}

// ============================================================================
// Backend-Specific Validation Tests
// ============================================================================

// TestEncryptionConfigValidation_AWSKMSValidation verifies AWS KMS specific validation
func TestEncryptionConfigValidation_AWSKMSValidation(t *testing.T) {
	tests := []struct {
		name          string
		awsKMSConfig  *ports.AWSKMSConfig
		shouldFail    bool
		expectedField string
	}{
		{
			name: "valid_complete_config",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN:            "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				DynamoDBTableName: "IdentityBrokerEncryptionBranchKeys",
				BranchKeyTTL:      "1h",
				DynamoDBRegion:    "us-east-1",
				DynamoDBTimeout:   "5s",
			},
			shouldFail: false,
		},
		{
			name: "missing_key_arn",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN: "", // Missing required field
			},
			shouldFail:    true,
			expectedField: "encryption.aws_kms.key_arn",
		},
		{
			name: "invalid_key_arn_format",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN: "invalid-arn-format", // Invalid ARN format
			},
			shouldFail:    true,
			expectedField: "encryption.aws_kms.key_arn",
		},
		{
			name: "minimal_valid_config",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				// Other fields optional with defaults
			},
			shouldFail: false,
		},
		{
			name: "negative_dynamodb_timeout",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN:          "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				DynamoDBTimeout: "-5s",
			},
			shouldFail:    true,
			expectedField: "encryption.aws_kms.dynamodb_timeout",
		},
		{
			name: "zero_dynamodb_timeout",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN:          "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				DynamoDBTimeout: "0s",
			},
			shouldFail:    true,
			expectedField: "encryption.aws_kms.dynamodb_timeout",
		},
		{
			name: "invalid_dynamodb_timeout",
			awsKMSConfig: &ports.AWSKMSConfig{
				KeyARN:          "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				DynamoDBTimeout: "not-a-duration",
			},
			shouldFail:    true,
			expectedField: "encryption.aws_kms.dynamodb_timeout",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ports.Config{
				Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
				Server:   createValidServerConfig(),
				Storage:  createValidStorageConfig(),
				Security: createValidSecurityConfig(),
				ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
					JWESigningKey: generateBase64EncodedString(t, 32),
				},
				Encryption: ports.EncryptionConfig{
					AWSKMS: tt.awsKMSConfig,
					Memory: nil,
				},
				OAuth2AuthServer: createValidOAuth2Config(),
				TokenRefresh:     validTokenRefreshConfig(),
			}

			err := Validate(cfg)
			if tt.shouldFail {
				if err == nil {
					t.Errorf("Test %s: Expected validation error, got nil", tt.name)
				}
				if tt.expectedField != "" {
					if cerr, ok := err.(*config.ConfigError); ok {
						if cerr.Field != tt.expectedField {
							t.Errorf("Test %s: Expected error field '%s', got: %s", tt.name, tt.expectedField, cerr.Field)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Test %s: Expected validation to pass, got error: %v", tt.name, err)
				}
			}
		})
	}
}

// TestEncryptionConfigValidation_MemoryValidation verifies Memory specific validation
func TestEncryptionConfigValidation_MemoryValidation(t *testing.T) {
	tests := []struct {
		name          string
		memoryConfig  *ports.MemoryConfig
		shouldFail    bool
		expectedField string
	}{
		{
			name: "valid_base64_key",
			memoryConfig: &ports.MemoryConfig{
				RawKey: generateBase64EncodedString(t, 32),
			},
			shouldFail: false,
		},
		{
			name: "missing_raw_key",
			memoryConfig: &ports.MemoryConfig{
				RawKey: "", // Missing required field
			},
			shouldFail:    true,
			expectedField: "encryption.memory.raw_key",
		},
		{
			name: "invalid_base64",
			memoryConfig: &ports.MemoryConfig{
				RawKey: "not-valid-base64-!!!@#$",
			},
			shouldFail:    true,
			expectedField: "encryption.memory.raw_key",
		},
		{
			name: "wrong_key_length",
			memoryConfig: &ports.MemoryConfig{
				RawKey: generateBase64EncodedString(t, 16), // Wrong length (16 bytes, not 32)
			},
			shouldFail:    true,
			expectedField: "encryption.memory.raw_key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &ports.Config{
				Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
				Server:   createValidServerConfig(),
				Storage:  createValidStorageConfig(),
				Security: createValidSecurityConfig(),
				ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
					JWESigningKey: generateBase64EncodedString(t, 32),
				},
				Encryption: ports.EncryptionConfig{
					AWSKMS: nil,
					Memory: tt.memoryConfig,
				},
				OAuth2AuthServer: createValidOAuth2Config(),
				TokenRefresh:     validTokenRefreshConfig(),
			}

			err := Validate(cfg)
			if tt.shouldFail {
				if err == nil {
					t.Errorf("Test %s: Expected validation error, got nil", tt.name)
				}
				if tt.expectedField != "" {
					if cerr, ok := err.(*config.ConfigError); ok {
						if cerr.Field != tt.expectedField {
							t.Errorf("Test %s: Expected error field '%s', got: %s", tt.name, tt.expectedField, cerr.Field)
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Test %s: Expected validation to pass, got error: %v", tt.name, err)
				}
			}
		})
	}
}

func TestEncryptionConfigFactory_AdapterFactory(t *testing.T) {
	t.Run("memory backend encrypts and binds its context", func(t *testing.T) {
		adapter, manager, err := aws.NewEncryptionAdapter(&ports.EncryptionConfig{
			Memory: &ports.MemoryConfig{RawKey: generateBase64EncodedString(t, 32)},
		})
		require.NoError(t, err)
		require.NotNil(t, adapter)
		require.Nil(t, manager)
		binding := map[string]string{"service_id": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}
		plain := []byte("private-session-token")
		ciphertext, err := adapter.Encrypt(context.Background(), plain, binding)
		require.NoError(t, err)
		require.NotEqual(t, plain, ciphertext)
		decrypted, err := adapter.Decrypt(context.Background(), ciphertext, binding)
		require.NoError(t, err)
		require.Equal(t, plain, decrypted)
		_, err = adapter.Decrypt(context.Background(), ciphertext, map[string]string{"service_id": "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"})
		require.Error(t, err)
	})

	for _, tc := range []struct {
		name string
		cfg  ports.EncryptionConfig
	}{
		{name: "missing backend"},
		{name: "conflicting backends", cfg: ports.EncryptionConfig{
			Memory: &ports.MemoryConfig{RawKey: generateBase64EncodedString(t, 32)},
			AWSKMS: &ports.AWSKMSConfig{KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"},
		}},
		{name: "invalid AWS TTL fails before KMS access", cfg: ports.EncryptionConfig{
			AWSKMS: &ports.AWSKMSConfig{KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012", BranchKeyTTL: "not-a-duration"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			adapter, manager, err := aws.NewEncryptionAdapter(&tc.cfg)
			require.Error(t, err)
			require.Nil(t, adapter)
			require.Nil(t, manager)
		})
	}
}

// TestValidateEncryptionConfigMissingBackend verifies validation fails when no backend is configured
func TestValidateEncryptionConfigMissingBackend(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil, // No backend configured
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error for missing encryption backend, got nil")
	}

	// Verify error mentions the field
	if _, ok := err.(*config.ConfigError); !ok {
		t.Errorf("Expected ConfigError, got %T", err)
	}
}

// TestValidateEncryptionConfigValidAWSKMSARN verifies validation passes for valid AWS KMS backend
func TestValidateEncryptionConfigValidAWSKMSARN(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: &ports.AWSKMSConfig{
				KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
			},
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for valid AWS KMS backend, got error: %v", err)
	}
}

// TestValidateEncryptionConfigValidMemoryBackend verifies validation passes for valid Memory backend
func TestValidateEncryptionConfigValidMemoryBackend(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil,
			Memory: &ports.MemoryConfig{
				RawKey: generateBase64EncodedString(t, 32),
			},
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for valid Memory backend, got error: %v", err)
	}
}

// TestValidateEncryptionConfigInvalidMemoryKey verifies validation fails for invalid Memory backend key
func TestValidateEncryptionConfigInvalidMemoryKey(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil,
			Memory: &ports.MemoryConfig{
				RawKey: "not-valid-base64-!!!@#$", // Invalid base64
			},
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error for invalid Memory backend key, got nil")
	}
}

// TestValidateEncryptionConfigWrongMemoryKeyLength verifies validation fails for non-32-byte Memory key
func TestValidateEncryptionConfigWrongMemoryKeyLength(t *testing.T) {
	// Create a 16-byte key (not 32-byte)
	invalidKey := generateBase64EncodedString(t, 16)

	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil,
			Memory: &ports.MemoryConfig{
				RawKey: invalidKey,
			},
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error for 16-byte Memory key (not 32-byte), got nil")
	}
}

// TestValidateEncryptionConfigInvalidAWSKMSARN verifies validation fails for malformed AWS KMS ARN
func TestValidateEncryptionConfigInvalidAWSKMSARN(t *testing.T) {
	cfg := &ports.Config{
		Log:      ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:   createValidServerConfig(),
		Storage:  createValidStorageConfig(),
		Security: createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: &ports.AWSKMSConfig{
				KeyARN: "arn:aws:kms:incomplete", // Incomplete ARN
			},
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
		TokenRefresh:     validTokenRefreshConfig(),
	}

	err := Validate(cfg)
	if err == nil {
		t.Error("Expected validation error for malformed AWS KMS ARN, got nil")
	}
}

// Helper function to create valid ServerConfig
func createValidServerConfig() ports.ServerConfig {
	return ports.ServerConfig{
		EndUser: ports.ServerInstanceConfig{
			Port:      8000,
			Bind:      "::",
			PublicURL: "http://localhost:8000",
			Authentication: ports.AuthenticationConfig{
				Preauth: ports.PreauthConfig{
					PrincipalHeaderName: "X-Remote-User",
				},
			},
		},
		Admin: ports.ServerInstanceConfig{
			Port:      14000,
			Bind:      "::",
			PublicURL: "http://localhost:14000",
			Authentication: ports.AuthenticationConfig{
				Preauth: ports.PreauthConfig{
					PrincipalHeaderName: "X-Remote-User",
				},
			},
		},
		Shutdown: ports.ShutdownConfig{
			Timeout: 30 * time.Duration(1e9),
		},
	}
}

// Helper function to create valid StorageConfig
func createValidStorageConfig() ports.StorageConfig {
	return ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Duration(1e9),
			Write: 10 * time.Duration(1e9),
		},
	}
}

func createValidSecurityConfig() ports.SecurityConfig {
	return ports.SecurityConfig{}
}

func createValidOAuth2Config() ports.OAuth2AuthServerConfig {
	return ports.OAuth2AuthServerConfig{
		Mode: "proxy",
		Proxy: ports.ProxyModeConfig{
			UpstreamIssuerURI:         "https://auth.example.com",
			UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
			UpstreamTokenEndpoint:     "https://auth.example.com/token",
		},
	}
}
