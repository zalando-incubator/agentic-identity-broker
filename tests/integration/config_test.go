package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testJWESigningKey is a valid test JWE key (32 bytes base64 encoded)
var testJWESigningKey = base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))

// testEncryptionKey is a valid test encryption key (32 bytes base64 encoded)
var testEncryptionKey = base64.StdEncoding.EncodeToString([]byte("abcdef0123456789abcdef0123456789"))

func repoPath(t *testing.T, relativePath string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve caller path")

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	return filepath.Join(repoRoot, relativePath)
}

func loadBrokerExampleConfig(t *testing.T, relativePath string, envVars map[string]string) (*ports.Config, []ports.ConfigSource) {
	t.Helper()

	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", repoPath(t, relativePath))
	for key, value := range envVars {
		t.Setenv(key, value)
	}

	loader := config.NewLoader()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := loader.GetConfig(ctx)
	require.NoError(t, err, "expected standalone broker example %s to load via internal/config.NewLoader", relativePath)

	return cfg, loader.GetSources()
}

func requireYAMLSource(t *testing.T, relativePath string, sources []ports.ConfigSource) {
	t.Helper()

	require.NotEmpty(t, sources, "expected %s to record configuration sources", relativePath)

	hasYAML := false
	for _, src := range sources {
		if src.Type == ports.SourceTypeYAML {
			hasYAML = true
			break
		}
	}

	assert.True(t, hasYAML, "expected %s to record a YAML source", relativePath)
}

// TestConfigurationPrecedence tests that configuration sources are applied in correct precedence order.
// Order: CLI flags > Environment variables > YAML > Defaults
func TestConfigurationPrecedence(t *testing.T) {
	tests := []struct {
		name          string
		yamlContent   string
		envVars       map[string]string
		cliFlags      map[string]interface{}
		expectedPort  int
		expectedBind  string
		expectedLevel string
	}{
		{
			name: "defaults only",
			yamlContent: fmt.Sprintf(`
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
encryption:
  key: %s
`, testEncryptionKey),
			expectedPort:  8000,
			expectedBind:  "::",
			expectedLevel: "info",
		},
		{
			name: "yaml overrides defaults",
			yamlContent: fmt.Sprintf(`
log:
  level: debug
  format: text
server:
  enduser:
    port: 9000
    bind: "127.0.0.1"
    public_url: http://localhost:9000
  admin:
    port: 14000
    bind: "::"
    public_url: http://localhost:14000
  shutdown:
    timeout: 30s
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
encryption:
  key: %s
`, testEncryptionKey),
			expectedPort:  9000,
			expectedBind:  "127.0.0.1",
			expectedLevel: "debug",
		},
		{
			name: "env overrides yaml",
			yamlContent: fmt.Sprintf(`
log:
  level: debug
  format: text
server:
  enduser:
    port: 9000
    bind: "127.0.0.1"
    public_url: http://localhost:9000
  admin:
    port: 14000
    bind: "::"
    public_url: http://localhost:14000
  shutdown:
    timeout: 30s
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
encryption:
  key: %s
`, testEncryptionKey),
			envVars: map[string]string{
				"IDENTITY_BROKER_SERVER_ENDUSER_PORT": "9500",
				"IDENTITY_BROKER_LOG_LEVEL":           "warn",
			},
			expectedPort:  9500,
			expectedBind:  "127.0.0.1", // From YAML
			expectedLevel: "warn",
		},
		{
			name: "cli overrides all",
			yamlContent: fmt.Sprintf(`
log:
  level: debug
  format: text
server:
  enduser:
    port: 9000
    bind: "127.0.0.1"
    public_url: http://localhost:9000
  admin:
    port: 14000
    bind: "::"
    public_url: http://localhost:14000
  shutdown:
    timeout: 30s
storage:
  backend: memory
  timeouts:
    read: 5s
    write: 10s
encryption:
  key: %s
`, testEncryptionKey),
			envVars: map[string]string{
				"IDENTITY_BROKER_SERVER_ENDUSER_PORT": "9500",
				"IDENTITY_BROKER_LOG_LEVEL":           "warn",
			},
			cliFlags: map[string]interface{}{
				"server.enduser.port": 10000,
				"log-level":           "error",
			},
			expectedPort:  10000,
			expectedBind:  "127.0.0.1", // From YAML
			expectedLevel: "error",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporary directory for test files
			tmpDir := t.TempDir()

			// Clear any existing environment variables to ensure clean test state
			// This prevents inherited env vars from interfering with test
			t.Setenv("IDENTITY_BROKER_LOG_LEVEL", "")
			t.Setenv("IDENTITY_BROKER_LOG_FORMAT", "")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_PORT", "")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_BIND", "")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_PORT", "")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_BIND", "")
			t.Setenv("IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT", "")

			// Set mandatory JWESigningKey, encryption key, principal headers, and oauth2 mode for all tests
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", testJWESigningKey)
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", testEncryptionKey)
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")

			// Create YAML config file if content is provided
			var configPath string
			if tt.yamlContent != "" {
				configPath = filepath.Join(tmpDir, "config.yaml")
				if err := os.WriteFile(configPath, []byte(tt.yamlContent), 0644); err != nil {
					t.Fatalf("Failed to write config file: %v", err)
				}
			}

			// Set environment variables (after clearing)
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// If we have a config file, set it in environment
			if configPath != "" {
				t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)
			}

			// Create loader
			loader := config.NewLoader()

			// Create cobra command if CLI flags are provided
			if tt.cliFlags != nil {
				cmd := &cobra.Command{
					Use: "test",
				}

				// Add flags
				cmd.Flags().Int("server.enduser.port", 0, "")
				cmd.Flags().String("server.enduser.bind", "", "")
				cmd.Flags().String("log-level", "", "")

				// Set flag values (need to parse flags to mark them as changed)
				for key, value := range tt.cliFlags {
					var err error
					switch key {
					case "server.enduser.port":
						err = cmd.Flags().Set(key, fmt.Sprintf("%d", value.(int)))
					case "log-level":
						err = cmd.Flags().Set(key, value.(string))
					}
					if err != nil {
						t.Fatalf("Failed to set flag %s: %v", key, err)
					}
				}

				loader.SetCommand(cmd)
			}

			// Load configuration
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			cfg, err := loader.GetConfig(ctx)
			if err != nil {
				t.Fatalf("Failed to load config: %v", err)
			}

			// Verify precedence
			if cfg.Server.EndUser.Port != tt.expectedPort {
				t.Errorf("Expected port %d, got %d", tt.expectedPort, cfg.Server.EndUser.Port)
			}

			if cfg.Server.EndUser.Bind != tt.expectedBind {
				t.Errorf("Expected bind %q, got %q", tt.expectedBind, cfg.Server.EndUser.Bind)
			}

			if string(cfg.Log.Level) != tt.expectedLevel {
				t.Errorf("Expected log level %q, got %q", tt.expectedLevel, cfg.Log.Level)
			}
		})
	}
}

// TestStandaloneBrokerExamplesLoadWithBrokerLoader keeps the standalone broker
// examples explicit. Do not replace this with a glob over examples/config:
// that directory also contains partial fragments and overlays that are not
// valid top-level broker configs.
func TestStandaloneBrokerExamplesLoadWithBrokerLoader(t *testing.T) {
	tests := []struct {
		name         string
		relativePath string
		envVars      map[string]string
		assertConfig func(*testing.T, *ports.Config)
	}{
		{
			name:         "config.development.yaml",
			relativePath: "examples/config/config.development.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY": testJWESigningKey,
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, 3000, cfg.Server.EndUser.Port)
				assert.Equal(t, "127.0.0.1", cfg.Server.EndUser.Bind)
				require.NotNil(t, cfg.Encryption.Memory)
				assert.Equal(t, "local", string(cfg.OAuth2AuthServer.Mode))
			},
		},
		{
			name:         "config.staging.yaml",
			relativePath: "examples/config/config.staging.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":           testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": testEncryptionKey,
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "::", cfg.Server.EndUser.Bind)
				assert.Equal(t, ports.LogFormatJSON, cfg.Log.Format)
				require.NotNil(t, cfg.Encryption.Memory)
			},
		},
		{
			name:         "config.production.yaml",
			relativePath: "examples/config/config.production.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":                                      testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN":                           "arn:aws:kms:us-east-1:123456789012:key/test-key-id",
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_ISSUER_URI":         "https://idp.example.com",
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_AUTHORIZE_ENDPOINT": "https://idp.example.com/oauth2/authorize",
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TOKEN_ENDPOINT":     "https://idp.example.com/oauth2/token",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "postgres", cfg.Storage.Backend)
				require.NotNil(t, cfg.Encryption.AWSKMS)
				assert.Equal(t, 30*time.Second, cfg.OAuth2AuthServer.Proxy.UpstreamTimeout)
				assert.Equal(t, "proxy", string(cfg.OAuth2AuthServer.Mode))
			},
		},
		{
			name:         "config.minimal.yaml",
			relativePath: "examples/config/config.minimal.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":                                             testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY":                                   testEncryptionKey,
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE":                                     "local",
				"IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME": "X-Remote-User",
				"IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME":   "X-Admin-User",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "X-Remote-User", cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName)
				assert.Equal(t, "X-Admin-User", cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName)
				assert.Equal(t, "local", string(cfg.OAuth2AuthServer.Mode))
				require.NotNil(t, cfg.Encryption.Memory)
			},
		},
		{
			name:         "config.ipv4-only.yaml",
			relativePath: "examples/config/config.ipv4-only.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":           testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": testEncryptionKey,
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE":   "local",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "0.0.0.0", cfg.Server.EndUser.Bind)
				assert.Equal(t, "127.0.0.1", cfg.Server.Admin.Bind)
			},
		},
		{
			name:         "config.ipv6-only.yaml",
			relativePath: "examples/config/config.ipv6-only.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":           testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": testEncryptionKey,
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE":   "local",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "::", cfg.Server.EndUser.Bind)
				assert.Equal(t, "::1", cfg.Server.Admin.Bind)
			},
		},
		{
			name:         "config.aws-emulator.yaml",
			relativePath: "examples/config/config.aws-emulator.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":         testJWESigningKey,
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE": "local",
				"AWS_ACCESS_KEY_ID":                       "test-access-key",
				"AWS_SECRET_ACCESS_KEY":                   "test-secret-key",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				require.NotNil(t, cfg.Encryption.AWSKMS)
				assert.Equal(t, "http://localhost:4566", cfg.Encryption.AWSKMS.KMSEndpoint)
				assert.Equal(t, "test-access-key", cfg.Encryption.AWSKMS.AccessKeyID)
				assert.Equal(t, "test-secret-key", cfg.Encryption.AWSKMS.SecretAccessKey)
				assert.Equal(t, "local", string(cfg.OAuth2AuthServer.Mode))
			},
		},
		{
			name:         "config.aws-production-advanced.yaml",
			relativePath: "examples/config/config.aws-production-advanced.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":                    testJWESigningKey,
				"IDENTITY_BROKER_STORAGE_POSTGRES_URL":               "postgresql://broker:secret@db.example.com:5432/identity_broker",
				"IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN":         "arn:aws:kms:eu-central-1:123456789012:key/advanced-test-key",
				"IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ASSUME_ROLE_ARN": "arn:aws:iam::123456789012:role/EncryptionRole",
				"IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE":            "local",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "postgres", cfg.Storage.Backend)
				assert.Equal(t, "postgresql://broker:secret@db.example.com:5432/identity_broker", cfg.Storage.Postgres.ConnectionURL)
				require.NotNil(t, cfg.Encryption.AWSKMS)
				assert.Equal(t, "arn:aws:iam::123456789012:role/EncryptionRole", cfg.Encryption.AWSKMS.AssumeRoleARN)
				assert.Equal(t, "5s", cfg.Encryption.AWSKMS.DynamoDBTimeout)
			},
		},
		{
			name:         "config.cimd-local.yaml",
			relativePath: "examples/config/config.cimd-local.yaml",
			envVars:      map[string]string{},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "local", string(cfg.OAuth2AuthServer.Mode))
				assert.True(t, cfg.OAuth2AuthServer.CIMD.Enabled)
				assert.True(t, cfg.Security.SkipThirdpartyHTTPSValidation)
			},
		},
		{
			name:         "oauth2-server-mode.yaml",
			relativePath: "examples/config/oauth2-server-mode.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":                                           testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY":                                 testEncryptionKey,
				"IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME": "X-Admin-User",
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "local", string(cfg.OAuth2AuthServer.Mode))
				assert.Equal(t, "X-Admin-User", cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName)
				assert.Equal(t, 90*time.Second, cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout)
				require.NotNil(t, cfg.Encryption.Memory)
			},
		},
		{
			name:         "oauth2-hybrid-mode.yaml",
			relativePath: "examples/config/oauth2-hybrid-mode.yaml",
			envVars: map[string]string{
				"IDENTITY_BROKER_JWE_SIGNING_KEY":           testJWESigningKey,
				"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": testEncryptionKey,
			},
			assertConfig: func(t *testing.T, cfg *ports.Config) {
				t.Helper()
				assert.Equal(t, "hybrid", string(cfg.OAuth2AuthServer.Mode))
				assert.Equal(t, []string{"authorization_code", "client_credentials"}, cfg.OAuth2AuthServer.SupportedGrantTypes)
				assert.Equal(t, 90*time.Second, cfg.OAuth2AuthServer.Local.SigningKeys.BootstrapTimeout)
				require.NotNil(t, cfg.Encryption.Memory)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, sources := loadBrokerExampleConfig(t, tt.relativePath, tt.envVars)
			tt.assertConfig(t, cfg)
			requireYAMLSource(t, tt.relativePath, sources)
		})
	}
}

func TestConfigurationParsesProxyUpstreamTimeoutFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	err := os.WriteFile(configPath, []byte(`oauth2_authorization_server:
  mode: proxy
  proxy:
    upstream_issuer_uri: https://idp.example.com
    upstream_authorize_endpoint: https://idp.example.com/oauth2/authorize
    upstream_token_endpoint: https://idp.example.com/oauth2/token
    upstream_timeout: 500ms
`), 0644)
	if err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", testJWESigningKey)
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", testEncryptionKey)
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")

	loader := config.NewLoader()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cfg, err := loader.GetConfig(ctx)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.OAuth2AuthServer.Proxy.UpstreamTimeout != 500*time.Millisecond {
		t.Fatalf("Expected proxy upstream timeout %v, got %v", 500*time.Millisecond, cfg.OAuth2AuthServer.Proxy.UpstreamTimeout)
	}
}
