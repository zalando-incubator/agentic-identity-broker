package config

import (
	"testing"
	"time"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageConfigValidation_Memory(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	err := validateStorageConfig(config)
	assert.NoError(t, err)
}

func TestStorageConfigValidation_Postgres(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "postgresql://user:pass@localhost:5432/testdb",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	err := validateStorageConfig(config)
	assert.NoError(t, err)
}

func TestStorageConfigValidation_InvalidBackend(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "invalid",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	err := validateStorageConfig(config)
	assert.Error(t, err)
}

func TestStorageConfigValidation_InvalidReadTimeout(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  0, // Invalid: zero duration
			Write: 10 * time.Second,
		},
	}

	err := validateStorageConfig(config)
	assert.Error(t, err)
}

func TestStorageConfigValidation_InvalidWriteTimeout(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "memory",
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 0, // Invalid: zero duration
		},
	}

	err := validateStorageConfig(config)
	assert.Error(t, err)
}

func TestStorageConfigValidation_PostgresNoURL(t *testing.T) {
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: "",
		},
		Timeouts: ports.StorageTimeouts{
			Read:  5 * time.Second,
			Write: 10 * time.Second,
		},
	}

	err := validateStorageConfig(config)
	assert.Error(t, err)
}

func TestValidate_PostgresConnectionURLCredentialsRedacted(t *testing.T) {
	tests := []struct {
		name string
		url  string
	}{
		{"invalid scheme", "mysql://user:credential-secret@localhost/testdb"},
		{"missing scheme", "user:credential-secret@localhost/testdb"},
		{"malformed URL with query password", "postgresql:/localhost/testdb?password=credential-secret%zz"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Storage.Backend = "postgres"
			cfg.Storage.Postgres.ConnectionURL = tt.url

			err := Validate(cfg)
			require.Error(t, err)
			var configErr *domconfig.ConfigError
			require.ErrorAs(t, err, &configErr)
			assert.Equal(t, "storage.postgres.connection_url", configErr.Field)
			assert.Equal(t, RedactedValue, configErr.Value)
			assert.Contains(t, err.Error(), "valid postgresql:// URL")
			assert.NotContains(t, err.Error(), tt.url)
			assert.NotContains(t, err.Error(), "credential-secret")
		})
	}
}

func TestStorageConfigValidation_PostgresInvalidURL(t *testing.T) {
	tests := []struct {
		name  string
		url   string
		valid bool
	}{
		{
			name:  "valid postgresql:// URL",
			url:   "postgresql://user:pass@localhost:5432/testdb",
			valid: true,
		},
		{
			name:  "valid postgres:// URL",
			url:   "postgres://user:pass@localhost:5432/testdb",
			valid: true,
		},
		{
			name:  "invalid scheme",
			url:   "mysql://user:pass@localhost:3306/testdb",
			valid: false,
		},
		{
			name:  "missing scheme",
			url:   "user:pass@localhost:5432/testdb",
			valid: false,
		},
		{
			name:  "empty URL",
			url:   "",
			valid: false,
		},
		{
			name:  "with sslmode parameter",
			url:   "postgresql://user:pass@localhost:5432/testdb?sslmode=require",
			valid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isValidPostgresURL(tt.url)
			assert.Equal(t, tt.valid, result, "URL validation for %s", tt.url)
		})
	}
}

func TestStorageTimeoutsValidation(t *testing.T) {
	tests := []struct {
		name    string
		read    time.Duration
		write   time.Duration
		wantErr bool
	}{
		{
			name:    "valid timeouts",
			read:    5 * time.Second,
			write:   10 * time.Second,
			wantErr: false,
		},
		{
			name:    "zero read timeout",
			read:    0,
			write:   10 * time.Second,
			wantErr: true,
		},
		{
			name:    "zero write timeout",
			read:    5 * time.Second,
			write:   0,
			wantErr: true,
		},
		{
			name:    "negative read timeout",
			read:    -1 * time.Second,
			write:   10 * time.Second,
			wantErr: true,
		},
		{
			name:    "very short timeouts",
			read:    1 * time.Millisecond,
			write:   1 * time.Millisecond,
			wantErr: false,
		},
		{
			name:    "very long timeouts",
			read:    60 * time.Second,
			write:   120 * time.Second,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			timeouts := &ports.StorageTimeouts{
				Read:  tt.read,
				Write: tt.write,
			}

			err := validateStorageTimeouts(timeouts)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
