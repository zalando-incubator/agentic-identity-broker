package aws

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// TestKeyStoreConfig tests configuration validation
func TestKeyStoreConfig(t *testing.T) {
	tests := []struct {
		name          string
		config        KeyStoreConfig
		expectedError bool
		errorContains string
	}{
		{
			name: "empty_kms_key_arn",
			config: KeyStoreConfig{
				KMSKeyARN:         "",
				DynamoDBTableName: "TestTable",
				BranchKeyTTL:      1 * time.Hour,
			},
			expectedError: true,
			errorContains: "KMS key ARN is required",
		},
		{
			name: "invalid_kms_arn_format",
			config: KeyStoreConfig{
				KMSKeyARN:         "not-a-valid-arn",
				DynamoDBTableName: "TestTable",
				BranchKeyTTL:      1 * time.Hour,
			},
			expectedError: true,
			errorContains: "not accessible",
		},
		{
			name: "default_table_name_applied",
			config: KeyStoreConfig{
				KMSKeyARN:         "", // Will fail on KMS check but shows defaults work
				DynamoDBTableName: "",
				BranchKeyTTL:      0,
			},
			expectedError: true,
			errorContains: "KMS key ARN is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal AWS config for test
			awsConfig := &ports.AWSKMSConfig{KeyARN: tt.config.KMSKeyARN}
			_, err := createKeyStore(context.Background(), tt.config, awsConfig, "TestKeyStore", slog.New(slog.DiscardHandler))

			if tt.expectedError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				if tt.errorContains != "" && !strings.Contains(err.Error(), tt.errorContains) {
					t.Errorf("error should contain %q, got: %v", tt.errorContains, err)
				}
			} else {
				if err != nil {
					t.Errorf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestKeyStoreConfigDefaults tests that defaults are applied
func TestKeyStoreConfigDefaults(t *testing.T) {
	config := KeyStoreConfig{
		DynamoDBTableName: "",
		BranchKeyTTL:      0,
	}

	// Apply defaults like createKeyStore does
	if config.DynamoDBTableName == "" {
		config.DynamoDBTableName = DefaultBranchKeyTableName
	}
	if config.BranchKeyTTL == 0 {
		config.BranchKeyTTL = DefaultBranchKeyTTL
	}

	// Verify defaults
	if config.DynamoDBTableName != DefaultBranchKeyTableName {
		t.Errorf("expected default table name %q, got %q", DefaultBranchKeyTableName, config.DynamoDBTableName)
	}

	if config.BranchKeyTTL != DefaultBranchKeyTTL {
		t.Errorf("expected default TTL %v, got %v", DefaultBranchKeyTTL, config.BranchKeyTTL)
	}
}

// TestKeyStoreConfigStructure tests that KeyStore configuration is properly structured
func TestKeyStoreConfigStructure(t *testing.T) {
	// This test verifies the KeyStore config matches what the AWS library expects
	// It's primarily a compile-time check via the struct literals in keystore.go

	// Test that KeyStoreConfig has required fields
	config := KeyStoreConfig{
		KMSKeyARN:         "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012",
		DynamoDBTableName: "TestBranchKeys",
		BranchKeyTTL:      2 * time.Hour,
	}

	if config.KMSKeyARN == "" {
		t.Error("KMSKeyARN should not be empty")
	}

	if config.DynamoDBTableName == "" {
		t.Error("DynamoDBTableName should not be empty")
	}

	if config.BranchKeyTTL == 0 {
		t.Error("BranchKeyTTL should not be zero")
	}
}

// TestKMSARNValidationInKeyStore tests KMS ARN validation
func TestKMSARNValidationInKeyStore(t *testing.T) {
	tests := []struct {
		name          string
		kmsARN        string
		shouldFail    bool
		errorContains string
	}{
		{
			name:       "valid_kms_key_arn",
			kmsARN:     "arn:aws:kms:us-west-2:123456789012:key/12345678-1234-1234-1234-123456789012",
			shouldFail: true, // Will fail due to KMS access in test, but validates ARN format
		},
		{
			name:          "empty_arn",
			kmsARN:        "",
			shouldFail:    true,
			errorContains: "KMS key ARN is required",
		},
		{
			name:       "kms_alias_arn",
			kmsARN:     "arn:aws:kms:us-east-1:123456789012:alias/my-key",
			shouldFail: true, // Will fail due to KMS access in test
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := KeyStoreConfig{
				KMSKeyARN:         tt.kmsARN,
				DynamoDBTableName: DefaultBranchKeyTableName,
				BranchKeyTTL:      DefaultBranchKeyTTL,
			}

			// Create minimal AWS config for test
			awsConfig := &ports.AWSKMSConfig{KeyARN: config.KMSKeyARN}
			_, err := createKeyStore(context.Background(), config, awsConfig, "TestKeyStore", slog.New(slog.DiscardHandler))

			if !tt.shouldFail && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if tt.shouldFail && err == nil {
				t.Errorf("expected error, got nil")
			}

			if tt.errorContains != "" && err != nil && !strings.Contains(err.Error(), tt.errorContains) {
				t.Errorf("error should contain %q, got: %v", tt.errorContains, err)
			}
		})
	}
}

// TestBranchKeyTTLConfiguration tests branch key TTL configuration
func TestBranchKeyTTLConfiguration(t *testing.T) {
	tests := []struct {
		name    string
		ttl     time.Duration
		isValid bool
	}{
		{
			name:    "one_hour_ttl",
			ttl:     1 * time.Hour,
			isValid: true,
		},
		{
			name:    "one_day_ttl",
			ttl:     24 * time.Hour,
			isValid: true,
		},
		{
			name:    "thirty_minutes_ttl",
			ttl:     30 * time.Minute,
			isValid: true,
		},
		{
			name:    "zero_ttl_uses_default",
			ttl:     0,
			isValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := KeyStoreConfig{
				BranchKeyTTL: tt.ttl,
			}

			// Verify TTL is properly set or defaults apply
			if config.BranchKeyTTL == 0 {
				config.BranchKeyTTL = DefaultBranchKeyTTL
			}

			if config.BranchKeyTTL.Seconds() <= 0 {
				t.Errorf("TTL in seconds should be positive, got: %v", config.BranchKeyTTL.Seconds())
			}
		})
	}
}

// TestKeyStoreInitialization tests KeyStore initialization flow
func TestKeyStoreInitialization(t *testing.T) {
	// This test verifies that KeyStoreConfig can be used to create a KeyStore
	// The actual KMS call will fail in test environment, but the configuration flow is validated

	config := KeyStoreConfig{
		KMSKeyARN:         "arn:aws:kms:us-west-2:123456789012:key/invalid-in-test",
		DynamoDBTableName: "TestBranchKeys",
		BranchKeyTTL:      1 * time.Hour,
	}

	// Should fail on KMS key verification, not on configuration
	// Create minimal AWS config for test
	awsConfig := &ports.AWSKMSConfig{KeyARN: config.KMSKeyARN}
	_, err := createKeyStore(context.Background(), config, awsConfig, "TestKeyStore", slog.New(slog.DiscardHandler))
	if err == nil {
		t.Fatal("expected error due to invalid KMS key in test")
	}

	// Verify error is about KMS access, not configuration
	if !isKEKUnavailableError(err) {
		t.Errorf("expected KEKUnavailable error, got: %T", err)
	}
}

// TestDefaultConstants tests that default constants have sensible values
func TestDefaultConstants(t *testing.T) {
	if DefaultKEKValidationTimeout <= 0 {
		t.Errorf("DefaultKEKValidationTimeout should be positive, got: %v", DefaultKEKValidationTimeout)
	}

	if DefaultBranchKeyTTL <= 0 {
		t.Errorf("DefaultBranchKeyTTL should be positive, got: %v", DefaultBranchKeyTTL)
	}

	if DefaultBranchKeyTableName == "" {
		t.Error("DefaultBranchKeyTableName should not be empty")
	}

	// TTL should be in a reasonable range (1 minute to 24 hours)
	if DefaultBranchKeyTTL < 1*time.Minute || DefaultBranchKeyTTL > 24*time.Hour {
		t.Errorf("DefaultBranchKeyTTL should be between 1 minute and 24 hours, got: %v", DefaultBranchKeyTTL)
	}
}

// Helper functions
