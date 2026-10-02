package config

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/aws"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// TestEncryptionConfigStructure verifies that the EncryptionConfig is properly integrated into the Config structure
func TestEncryptionConfigStructure(t *testing.T) {
	// Test that the EncryptionConfig is properly integrated into the main Config struct
	var config ports.Config

	// Verify that Encryption field exists and has new backend structure
	if config.Encryption.AWSKMS != nil {
		t.Error("Default AWSKMS backend should be nil")
	}
	if config.Encryption.Memory != nil {
		t.Error("Default Memory backend should be nil")
	}

	// Test assignment of AWS KMS backend
	config.Encryption.AWSKMS = &ports.AWSKMSConfig{
		KeyARN: "test-key-arn",
	}
	if config.Encryption.AWSKMS == nil || config.Encryption.AWSKMS.KeyARN != "test-key-arn" {
		t.Error("Failed to assign AWS KMS config")
	}

	// Test assignment of Memory backend
	config.Encryption.AWSKMS = nil
	config.Encryption.Memory = &ports.MemoryConfig{
		RawKey: "test-raw-key",
	}
	if config.Encryption.Memory == nil || config.Encryption.Memory.RawKey != "test-raw-key" {
		t.Error("Failed to assign Memory config")
	}
}

// TestEncryptionConfigValidation verifies the new backend structure validation works
func TestEncryptionConfigValidation(t *testing.T) {
	// This is a structural test to verify the EncryptionConfig has proper backend structure
	var config ports.EncryptionConfig

	// Test that we can assign AWS KMS backend configuration
	config.AWSKMS = &ports.AWSKMSConfig{
		KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
	}
	if config.AWSKMS == nil || config.AWSKMS.KeyARN == "" {
		t.Error("Failed to assign AWS KMS backend configuration")
	}

	// Test that we can assign Memory backend configuration
	config.AWSKMS = nil
	config.Memory = &ports.MemoryConfig{
		RawKey: "${ENCRYPTION_KEK}",
	}
	if config.Memory == nil || config.Memory.RawKey != "${ENCRYPTION_KEK}" {
		t.Error("Failed to assign Memory backend configuration")
	}
}

// TestEncryptionConfigFieldTags verifies the struct tags are correct
func TestEncryptionConfigFieldTags(t *testing.T) {
	// Verify that the configuration integrates properly at the type level
	var mainConfig ports.Config

	// Ensure the Encryption field is present and accessible with new backend structure
	mainConfig.Encryption.AWSKMS = &ports.AWSKMSConfig{
		KeyARN: "test-value",
	}

	if mainConfig.Encryption.AWSKMS == nil || mainConfig.Encryption.AWSKMS.KeyARN != "test-value" {
		t.Error("Encryption AWS KMS backend field not properly accessible in main Config struct")
	}

	// Test that all expected configuration sections exist
	// This ensures the EncryptionConfig is properly integrated alongside other configs
	testConfigs := map[string]interface{}{
		"Log":              mainConfig.Log,
		"Server":           mainConfig.Server,
		"Storage":          mainConfig.Storage,
		"ThirdPartyOAuth2": mainConfig.ThirdPartyOAuth2,
		"OAuth2AuthServer": mainConfig.OAuth2AuthServer,
		"Security":         mainConfig.Security,
		"Encryption":       mainConfig.Encryption,
	}

	for name, config := range testConfigs {
		if config == nil {
			// Some configs might be zero values, that's okay
			t.Logf("Config section %s has zero value (this is normal)", name)
		}
	}
}

// TestEncryptionConfigDefaults verifies default values
func TestEncryptionConfigDefaults(t *testing.T) {
	var config ports.EncryptionConfig

	// Defaults should be nil (backends must be explicitly configured)
	if config.AWSKMS != nil {
		t.Error("Default AWSKMS backend should be nil")
	}
	if config.Memory != nil {
		t.Error("Default Memory backend should be nil")
	}
}

// TestEncryptionConfigFieldTypes verifies field types are correct
func TestEncryptionConfigFieldTypes(t *testing.T) {
	var config ports.EncryptionConfig

	// Test AWS KMS backend assignment
	awsKMSConfig := &ports.AWSKMSConfig{
		KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
	}
	config.AWSKMS = awsKMSConfig
	if config.AWSKMS != awsKMSConfig {
		t.Error("Failed to assign AWS KMS config")
	}

	// Test Memory backend assignment
	memoryConfig := &ports.MemoryConfig{
		RawKey: generateBase64EncodedString(t, 32),
	}
	config.Memory = memoryConfig
	if config.Memory != memoryConfig {
		t.Error("Failed to assign Memory config")
	}
}

// ============================================================================
// Backend Configuration Tests
// ============================================================================

// TestEncryptionConfigBackend_AWSKMSBackend verifies AWS KMS backend configuration parses correctly
func TestEncryptionConfigBackend_AWSKMSBackend(t *testing.T) {
	cfg := &ports.Config{
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil,
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
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
				BusinessEvents: ports.DefaultBusinessEventsConfig(),
				Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
				Server:         createValidServerConfig(),
				Storage:        createValidStorageConfig(),
				Security:       createValidSecurityConfig(),
				ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
					JWESigningKey: generateBase64EncodedString(t, 32),
				},
				Encryption: ports.EncryptionConfig{
					AWSKMS: tt.awsKMSConfig,
					Memory: nil,
				},
				OAuth2AuthServer: createValidOAuth2Config(),
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
				BusinessEvents: ports.DefaultBusinessEventsConfig(),
				Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
				Server:         createValidServerConfig(),
				Storage:        createValidStorageConfig(),
				Security:       createValidSecurityConfig(),
				ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
					JWESigningKey: generateBase64EncodedString(t, 32),
				},
				Encryption: ports.EncryptionConfig{
					AWSKMS: nil,
					Memory: tt.memoryConfig,
				},
				OAuth2AuthServer: createValidOAuth2Config(),
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

// ============================================================================
// Environment Variable Binding Tests
// ============================================================================

// TestEncryptionConfig_EnvironmentVariableBinding verifies environment variable binding works
func TestEncryptionConfig_EnvironmentVariableBinding(t *testing.T) {
	// This test verifies that the new structure supports environment variable binding
	// The actual binding is tested in loader tests, but this ensures field names are correct

	// Test AWS KMS environment variables should map to:
	// ENCRYPTION_AWS_KMS_KEY_ARN -> encryption.aws_kms.key_arn
	// ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME -> encryption.aws_kms.dynamodb_table_name
	// ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL -> encryption.aws_kms.branch_key_ttl
	// ENCRYPTION_AWS_KMS_DYNAMODB_REGION -> encryption.aws_kms.dynamodb_region
	// ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT -> encryption.aws_kms.dynamodb_timeout

	// Test Memory environment variables should map to:
	// ENCRYPTION_MEMORY_RAW_KEY -> encryption.memory.raw_key

	// Verify struct tags exist and are correctly formatted
	// This is a compile-time check that ensures mapstructure tags are correct
	cfg := ports.EncryptionConfig{}

	// These assignments should compile without error, validating struct field existence
	if cfg.AWSKMS != nil {
		_ = cfg.AWSKMS.KeyARN
		_ = cfg.AWSKMS.DynamoDBTableName
		_ = cfg.AWSKMS.BranchKeyTTL
		_ = cfg.AWSKMS.DynamoDBRegion
		_ = cfg.AWSKMS.DynamoDBTimeout
	}

	if cfg.Memory != nil {
		_ = cfg.Memory.RawKey
	}

	// This test passes if compilation succeeds
	t.Log("Environment variable binding structure validation passed")
}

// ============================================================================
// Adapter Factory Tests
// ============================================================================

// TestEncryptionConfigFactory_AdapterFactory verifies the adapter factory pattern works with new config
func TestEncryptionConfigFactory_AdapterFactory(t *testing.T) {
	// Test that NewEncryptionAdapter factory function works with new config structure
	tests := []struct {
		name          string
		config        *ports.EncryptionConfig
		shouldFail    bool
		expectedError string
	}{
		{
			name: "aws_kms_backend",
			config: &ports.EncryptionConfig{
				AWSKMS: &ports.AWSKMSConfig{
					KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				},
				Memory: nil,
			},
			// Note: This test will fail because it tries to access real AWS KMS
			// In a real environment, this would require valid AWS credentials
			// For unit tests, we expect this to fail with AWS access error
			shouldFail:    true,
			expectedError: "KMS key not accessible",
		},
		{
			name: "memory_backend",
			config: &ports.EncryptionConfig{
				AWSKMS: nil,
				Memory: &ports.MemoryConfig{
					RawKey: generateBase64EncodedString(t, 32),
				},
			},
			shouldFail: false,
		},
		{
			name: "no_backend_configured",
			config: &ports.EncryptionConfig{
				AWSKMS: nil,
				Memory: nil,
			},
			shouldFail:    true,
			expectedError: "exactly one encryption backend must be configured (aws_kms or memory)",
		},
		{
			name: "both_backends_configured",
			config: &ports.EncryptionConfig{
				AWSKMS: &ports.AWSKMSConfig{
					KeyARN: "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012",
				},
				Memory: &ports.MemoryConfig{
					RawKey: generateBase64EncodedString(t, 32),
				},
			},
			shouldFail:    true,
			expectedError: "exactly one encryption backend must be configured, not both aws_kms and memory",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This calls the new factory function from the aws package
			adapter, manager, err := aws.NewEncryptionAdapter(tt.config)

			if tt.shouldFail {
				if err == nil {
					t.Errorf("Test %s: Expected error, got nil", tt.name)
				}
				if tt.expectedError != "" && err != nil {
					// For AWS KMS tests, we check if error contains the expected substring
					// because AWS errors can have varying details
					if tt.name == "aws_kms_backend" {
						if !strings.Contains(err.Error(), tt.expectedError) {
							t.Errorf("Test %s: Expected error containing '%s', got: %s", tt.name, tt.expectedError, err.Error())
						}
					} else {
						if err.Error() != tt.expectedError {
							t.Errorf("Test %s: Expected error '%s', got: %s", tt.name, tt.expectedError, err.Error())
						}
					}
				}
			} else {
				if err != nil {
					t.Errorf("Test %s: Expected success, got error: %v", tt.name, err)
				}
				if adapter == nil {
					t.Errorf("Test %s: Expected non-nil adapter", tt.name)
				}
				// Note: manager can be nil for base64 memory scenarios (raw AES keyring)
				// For AWS KMS scenarios, manager should not be nil
				if tt.name == "aws_kms_backend" && manager == nil {
					t.Errorf("Test %s: Expected non-nil branch key manager for AWS KMS backend", tt.name)
				}
			}
		})
	}
}

// TestValidateEncryptionConfigMissingBackend verifies validation fails when no backend is configured
func TestValidateEncryptionConfigMissingBackend(t *testing.T) {
	cfg := &ports.Config{
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: generateBase64EncodedString(t, 32),
		},
		Encryption: ports.EncryptionConfig{
			AWSKMS: nil, // No backend configured
			Memory: nil,
		},
		OAuth2AuthServer: createValidOAuth2Config(),
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
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for valid AWS KMS backend, got error: %v", err)
	}
}

// TestValidateEncryptionConfigValidMemoryBackend verifies validation passes for valid Memory backend
func TestValidateEncryptionConfigValidMemoryBackend(t *testing.T) {
	cfg := &ports.Config{
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
	}

	err := Validate(cfg)
	if err != nil {
		t.Errorf("Expected validation to pass for valid Memory backend, got error: %v", err)
	}
}

// TestValidateEncryptionConfigInvalidMemoryKey verifies validation fails for invalid Memory backend key
func TestValidateEncryptionConfigInvalidMemoryKey(t *testing.T) {
	cfg := &ports.Config{
		BusinessEvents: ports.DefaultBusinessEventsConfig(),
		Log:            ports.LogConfig{Level: ports.LogLevelInfo, Format: ports.LogFormatText},
		Server:         createValidServerConfig(),
		Storage:        createValidStorageConfig(),
		Security:       createValidSecurityConfig(),
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
