package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// validConfig returns a minimal valid Config for use in test cases.
func validConfig() *config.Config {
	return &config.Config{
		GRPC: config.GRPCConfig{
			Bind:                 "0.0.0.0",
			Port:                 50051,
			MaxConcurrentStreams: 100,
		},
		OAuth2: config.OAuth2Config{
			TokenEndpoint:       "https://identity-broker.example.com/oauth2/token",
			Issuer:              "https://upstream-oauth2.example.com",
			ClientID:            "extproc-client",
			ClientSecret:        "supersecret",
			ClientAssertionType: "id_token",
			ExchangeTimeout:     5 * time.Second,
			TLS:                 config.TLSConfig{AllowHTTP: false},
		},
		Cache: config.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		CircuitBreaker: config.CircuitBreakerConfig{
			Enabled:      true,
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
		Sessions: config.SessionsConfig{
			Extraction: config.SessionExtractionConfig{HTTPHeader: "Mcp-Session-Id"},
		},
		Telemetry: config.TelemetryConfig{
			Exporter: config.OTLPExporterConfig{
				Timeout: 10 * time.Second,
			},
		},
	}
}

func extprocRepoPath(t *testing.T, relativePath string) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve caller path")

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	return filepath.Join(repoRoot, relativePath)
}

func loadStandaloneExtProcExampleConfig(t *testing.T, relativePath string, envVars map[string]string) *config.Config {
	t.Helper()

	t.Setenv("EXTPROC_CONFIG_PATH", extprocRepoPath(t, relativePath))
	for key, value := range envVars {
		t.Setenv(key, value)
	}

	cfg, err := config.Load()
	require.NoError(t, err, "expected standalone ExtProc example %s to load via internal/extproc/config.Load", relativePath)

	return cfg
}

// TestStandaloneExtProcExamplesLoadWithExtProcLoader keeps the standalone
// ExtProc examples explicit. Do not glob examples/config here: the directory
// also contains extproc-telemetry.yaml, which is an overlay rather than a
// standalone ExtProc config.
func TestStandaloneExtProcExamplesLoadWithExtProcLoader(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"deny\"}\n"), 0o600))

	tests := []struct {
		name         string
		relativePath string
		envVars      map[string]string
		assertConfig func(*testing.T, *config.Config)
	}{
		{
			name:         "extproc-token-exchange.yaml",
			relativePath: "examples/config/extproc-token-exchange.yaml",
			envVars: map[string]string{
				"EXTPROC_OAUTH2_CLIENT_SECRET": "extproc-example-secret",
			},
			assertConfig: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "extproc-gateway", cfg.OAuth2.ClientID)
				assert.Equal(t, "extproc-example-secret", cfg.OAuth2.ClientSecret)
				assert.Equal(t, "https://identity-broker.example.com/oauth2/token", cfg.OAuth2.TokenEndpoint)
				assert.Equal(t, "text", cfg.Log.Format)
			},
		},
		{
			name:         "extproc-opa-authorization.yaml",
			relativePath: "examples/config/extproc-opa-authorization.yaml",
			envVars: map[string]string{
				"EXTPROC_OAUTH2_CLIENT_ID":          "opa-extproc-client",
				"EXTPROC_OAUTH2_CLIENT_SECRET":      "opa-extproc-secret",
				"EXTPROC_AUTHORIZATION_POLICY_PATH": policyPath,
			},
			assertConfig: func(t *testing.T, cfg *config.Config) {
				t.Helper()
				assert.Equal(t, "opa-extproc-client", cfg.OAuth2.ClientID)
				assert.Equal(t, "opa-extproc-secret", cfg.OAuth2.ClientSecret)
				assert.True(t, cfg.Authorization.Enabled)
				assert.Equal(t, policyPath, cfg.Authorization.Policy.Path)
				assert.Empty(t, cfg.Authorization.Policy.ConfigFile)
				assert.Equal(t, "aib.extproc.authz", cfg.Authorization.Policy.Package)
				assert.Equal(t, "deny", cfg.Authorization.DefaultDecision)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := loadStandaloneExtProcExampleConfig(t, tt.relativePath, tt.envVars)
			tt.assertConfig(t, cfg)
		})
	}
}

// ---------------------------------------------------------------------------
// Validate: All 10 validation rules (table-driven)
// ---------------------------------------------------------------------------

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(*config.Config)
		wantErr     bool
		errContains string
	}{
		{
			name:    "valid config passes all rules",
			mutate:  func(_ *config.Config) {},
			wantErr: false,
		},
		// Rule 1: grpc.port
		{
			name:        "rule1: port 0 is invalid",
			mutate:      func(c *config.Config) { c.GRPC.Port = 0 },
			wantErr:     true,
			errContains: "grpc.port",
		},
		{
			name:        "rule1: port 65536 is invalid",
			mutate:      func(c *config.Config) { c.GRPC.Port = 65536 },
			wantErr:     true,
			errContains: "grpc.port",
		},
		{
			name:    "rule1: port 1 is valid",
			mutate:  func(c *config.Config) { c.GRPC.Port = 1 },
			wantErr: false,
		},
		{
			name:    "rule1: port 65535 is valid",
			mutate:  func(c *config.Config) { c.GRPC.Port = 65535 },
			wantErr: false,
		},
		// Rule 1a: grpc.max_concurrent_streams
		{
			name:        "rule1a: negative max_concurrent_streams is invalid",
			mutate:      func(c *config.Config) { c.GRPC.MaxConcurrentStreams = -1 },
			wantErr:     true,
			errContains: "grpc.max_concurrent_streams",
		},
		{
			name: "rule1a: max_concurrent_streams above uint32 is invalid",
			mutate: func(c *config.Config) {
				c.GRPC.MaxConcurrentStreams = int(uint64(^uint32(0)) + 1)
			},
			wantErr:     true,
			errContains: "grpc.max_concurrent_streams",
		},
		// Rule 2: grpc.bind
		{
			name:        "rule2: empty bind is invalid",
			mutate:      func(c *config.Config) { c.GRPC.Bind = "" },
			wantErr:     true,
			errContains: "grpc.bind",
		},
		{
			name:        "rule2: whitespace-only bind is invalid",
			mutate:      func(c *config.Config) { c.GRPC.Bind = "   " },
			wantErr:     true,
			errContains: "grpc.bind",
		},
		// Rule 3: oauth2.token_endpoint
		{
			name:        "rule3: empty token_endpoint is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.TokenEndpoint = "" },
			wantErr:     true,
			errContains: "oauth2.token_endpoint",
		},
		{
			name:        "rule3: non-URL token_endpoint is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.TokenEndpoint = "not-a-url" },
			wantErr:     true,
			errContains: "oauth2.token_endpoint",
		},
		{
			name:        "rule3: ftp:// scheme is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.TokenEndpoint = "ftp://example.com/token" },
			wantErr:     true,
			errContains: "oauth2.token_endpoint",
		},
		// Rule 4: oauth2.issuer
		{
			name:        "rule4: empty issuer is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.Issuer = "" },
			wantErr:     true,
			errContains: "oauth2.issuer",
		},
		{
			name:        "rule4: non-URL issuer is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.Issuer = "not-a-url" },
			wantErr:     true,
			errContains: "oauth2.issuer",
		},
		// Rule 5: oauth2.client_id
		{
			name:        "rule5: empty client_id is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.ClientID = "" },
			wantErr:     true,
			errContains: "oauth2.client_id",
		},
		// Rule 6: oauth2.client_secret
		{
			name:        "rule6: empty client_secret is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.ClientSecret = "" },
			wantErr:     true,
			errContains: "oauth2.client_secret",
		},
		// Rule 7: cache.default_ttl
		{
			name:        "rule7: zero default_ttl is invalid",
			mutate:      func(c *config.Config) { c.Cache.DefaultTTL = 0 },
			wantErr:     true,
			errContains: "cache.default_ttl",
		},
		{
			name:        "rule7: negative default_ttl is invalid",
			mutate:      func(c *config.Config) { c.Cache.DefaultTTL = -1 * time.Second },
			wantErr:     true,
			errContains: "cache.default_ttl",
		},
		// Rule 8: TLS enforcement
		{
			name: "rule8: http token_endpoint rejected when allow_http is false",
			mutate: func(c *config.Config) {
				c.OAuth2.TokenEndpoint = "http://identity-broker.example.com/oauth2/token"
				c.OAuth2.TLS.AllowHTTP = false
			},
			wantErr:     true,
			errContains: "oauth2.token_endpoint",
		},
		{
			name: "rule8: http issuer rejected when allow_http is false",
			mutate: func(c *config.Config) {
				c.OAuth2.Issuer = "http://upstream-oauth2.example.com"
				c.OAuth2.TLS.AllowHTTP = false
			},
			wantErr:     true,
			errContains: "oauth2.issuer",
		},
		{
			name: "rule8: http endpoints allowed when allow_http is true",
			mutate: func(c *config.Config) {
				c.OAuth2.TokenEndpoint = "http://identity-broker.example.com/oauth2/token"
				c.OAuth2.Issuer = "http://upstream-oauth2.example.com"
				c.OAuth2.TLS.AllowHTTP = true
			},
			wantErr: false,
		},
		// Rule 9: cache.max_ttl
		{
			name:        "rule9: zero max_ttl is invalid",
			mutate:      func(c *config.Config) { c.Cache.MaxTTL = 0 },
			wantErr:     true,
			errContains: "cache.max_ttl",
		},
		// Rule 10: oauth2.exchange_timeout
		{
			name:        "rule10: zero exchange_timeout is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.ExchangeTimeout = 0 },
			wantErr:     true,
			errContains: "oauth2.exchange_timeout",
		},
		{
			name:        "rule10: negative exchange_timeout is invalid",
			mutate:      func(c *config.Config) { c.OAuth2.ExchangeTimeout = -1 * time.Second },
			wantErr:     true,
			errContains: "oauth2.exchange_timeout",
		},
		// Rule 11: log.level enum
		{
			name:        "rule11: invalid log level is rejected",
			mutate:      func(c *config.Config) { c.Log.Level = "verbose" },
			wantErr:     true,
			errContains: "log.level",
		},
		{
			name:    "rule11: valid log levels accepted",
			mutate:  func(c *config.Config) { c.Log.Level = "debug" },
			wantErr: false,
		},
		// Rule 12: log.format enum
		{
			name:        "rule12: invalid log format is rejected",
			mutate:      func(c *config.Config) { c.Log.Format = "logfmt" },
			wantErr:     true,
			errContains: "log.format",
		},
		{
			name:    "rule12: valid log formats accepted",
			mutate:  func(c *config.Config) { c.Log.Format = "json" },
			wantErr: false,
		},
		// Multiple errors collected
		{
			name: "multiple invalid fields returns combined error",
			mutate: func(c *config.Config) {
				c.OAuth2.TokenEndpoint = ""
				c.OAuth2.ClientID = ""
				c.OAuth2.ClientSecret = ""
			},
			wantErr:     true,
			errContains: "configuration validation failed",
		},
		// Rule 14: circuit_breaker.max_failures
		{
			name:        "rule14: zero max_failures is invalid",
			mutate:      func(c *config.Config) { c.CircuitBreaker.MaxFailures = 0 },
			wantErr:     true,
			errContains: "circuit_breaker.max_failures",
		},
		{
			name:        "rule14: negative max_failures is invalid",
			mutate:      func(c *config.Config) { c.CircuitBreaker.MaxFailures = -1 },
			wantErr:     true,
			errContains: "circuit_breaker.max_failures",
		},
		{
			name:    "rule14: max_failures 1 is valid",
			mutate:  func(c *config.Config) { c.CircuitBreaker.MaxFailures = 1 },
			wantErr: false,
		},
		// Rule 15: circuit_breaker.reset_timeout
		{
			name:        "rule15: zero reset_timeout is invalid",
			mutate:      func(c *config.Config) { c.CircuitBreaker.ResetTimeout = 0 },
			wantErr:     true,
			errContains: "circuit_breaker.reset_timeout",
		},
		{
			name:        "rule15: negative reset_timeout is invalid",
			mutate:      func(c *config.Config) { c.CircuitBreaker.ResetTimeout = -1 * time.Second },
			wantErr:     true,
			errContains: "circuit_breaker.reset_timeout",
		},
		{
			name: "rule19: zero exporter timeout is invalid",
			mutate: func(c *config.Config) {
				c.Telemetry.Enabled = true
				c.Telemetry.Exporter.Endpoint = "collector:4317"
				c.Telemetry.Exporter.Timeout = 0
			},
			wantErr:     true,
			errContains: "telemetry.exporter.timeout",
		},
		{
			name: "rule19: negative exporter timeout is invalid",
			mutate: func(c *config.Config) {
				c.Telemetry.Enabled = true
				c.Telemetry.Exporter.Endpoint = "collector:4317"
				c.Telemetry.Exporter.Timeout = -1 * time.Second
			},
			wantErr:     true,
			errContains: "telemetry.exporter.timeout",
		},
		// Disabled circuit breaker: rules 14-15 are skipped
		{
			name: "disabled circuit breaker: zero max_failures is valid",
			mutate: func(c *config.Config) {
				c.CircuitBreaker.Enabled = false
				c.CircuitBreaker.MaxFailures = 0
			},
			wantErr: false,
		},
		{
			name: "disabled circuit breaker: zero reset_timeout is valid",
			mutate: func(c *config.Config) {
				c.CircuitBreaker.Enabled = false
				c.CircuitBreaker.ResetTimeout = 0
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			tt.mutate(cfg)

			err := config.Validate(cfg)

			if tt.wantErr {
				require.Error(t, err, "expected validation error")
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains,
						"error message should reference the invalid field")
				}
			} else {
				assert.NoError(t, err, "expected no validation error")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// LoadFromViper: defaults, env vars, ${VAR} expansion
// ---------------------------------------------------------------------------

func TestLoadFromViper_Defaults(t *testing.T) {
	v := viper.New()

	// Set only required fields via Viper directly (simulate env vars)
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	// Verify defaults are applied
	assert.Equal(t, "0.0.0.0", cfg.GRPC.Bind)
	assert.Equal(t, 50051, cfg.GRPC.Port)
	assert.Equal(t, 100, cfg.GRPC.MaxConcurrentStreams)
	assert.Equal(t, 5*time.Second, cfg.OAuth2.ExchangeTimeout)
	assert.Equal(t, 5*time.Minute, cfg.Cache.DefaultTTL)
	assert.Equal(t, 1*time.Hour, cfg.Cache.MaxTTL)
	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "text", cfg.Log.Format)
	assert.False(t, cfg.OAuth2.TLS.InsecureSkipVerify)
	assert.False(t, cfg.OAuth2.TLS.AllowHTTP)
	assert.Equal(t, "", cfg.OAuth2.TLS.CaBundlePath)
	assert.Equal(t, 5, cfg.CircuitBreaker.MaxFailures)
	assert.Equal(t, 30*time.Second, cfg.CircuitBreaker.ResetTimeout)
	assert.True(t, cfg.CircuitBreaker.Enabled, "circuit breaker should be enabled by default")
}

func TestLoadFromViper_EnvVarExpansion(t *testing.T) {
	// Set an environment variable to be expanded
	t.Setenv("TEST_EXTPROC_SECRET", "expanded-secret-value")

	v := viper.New()
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "${TEST_EXTPROC_SECRET}")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.Equal(t, "expanded-secret-value", cfg.OAuth2.ClientSecret,
		"${VAR} notation should be expanded for client_secret")
}

func TestLoadFromViper_EnvVarExpansion_LogFields(t *testing.T) {
	t.Setenv("TEST_LOG_LEVEL", "debug")
	t.Setenv("TEST_LOG_FORMAT", "json")

	v := viper.New()
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")
	v.Set("log.level", "${TEST_LOG_LEVEL}")
	v.Set("log.format", "${TEST_LOG_FORMAT}")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.Equal(t, "debug", cfg.Log.Level, "${VAR} notation should be expanded for log.level")
	assert.Equal(t, "json", cfg.Log.Format, "${VAR} notation should be expanded for log.format")
}

func TestLoadFromViper_EnvVarExpansion_AuthorizationDefaultDecisionRejectsAllow(t *testing.T) {
	t.Setenv("TEST_DEFAULT_DECISION", "allow")

	v := viper.New()
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")
	v.Set("authorization.enabled", true)
	v.Set("authorization.policy.package", "aib.extproc.authz")
	v.Set("authorization.policy.decision", "result")
	v.Set("authorization.policy.path", t.TempDir())
	v.Set("authorization.evaluation_timeout", "100ms")
	v.Set("authorization.max_body_size", 1048576)
	v.Set("authorization.default_decision", "${TEST_DEFAULT_DECISION}")

	_, err := config.LoadFromViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authorization.default_decision must be deny")
}

func TestLoadFromViper_AuthorizationPolicyPathFromEnv(t *testing.T) {
	policyDir := t.TempDir()

	// EXTPROC_AUTHORIZATION_POLICY_PATH env var must populate Policy.Path when
	// authorization is explicitly enabled and the path exists.
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyDir)
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "secret")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.True(t, cfg.Authorization.Enabled)
	assert.Equal(t, policyDir, cfg.Authorization.Policy.Path,
		"EXTPROC_AUTHORIZATION_POLICY_PATH env var must populate Authorization.Policy.Path")
}

func TestLoadFromViper_AuthorizationPolicyConfigFileFromEnv(t *testing.T) {
	cfgFile, err := os.CreateTemp(t.TempDir(), "opa-config-*.yaml")
	require.NoError(t, err)
	_, _ = cfgFile.WriteString("services: {}\n")
	require.NoError(t, cfgFile.Close())

	// EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE env var must populate Policy.ConfigFile when
	// authorization is explicitly enabled and the file exists.
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE", cfgFile.Name())
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "secret")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	cfg, loadErr := config.LoadFromViper(v)
	require.NoError(t, loadErr)

	assert.True(t, cfg.Authorization.Enabled)
	assert.Equal(t, cfgFile.Name(), cfg.Authorization.Policy.ConfigFile,
		"EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE env var must populate Authorization.Policy.ConfigFile")
}

func TestLoadFromViper_AuthorizationPolicySourceRequiresEnabled(t *testing.T) {
	policyDir := t.TempDir()

	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyDir)
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "secret")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	_, err := config.LoadFromViper(v)
	require.Error(t, err)
	assert.Contains(t, err.Error(),
		"authorization.policy.path or authorization.policy.config_file is set but authorization.enabled is false")
}

func TestLoadWithCommand_AuthorizationPolicyFlagsOverrideEnv(t *testing.T) {
	envPath := t.TempDir()
	cliPath := t.TempDir()

	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	// Set env vars for authorization policy fields — CLI flags must win over these.
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", envPath)
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PACKAGE", "env.authz")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_DECISION", "env_result")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("authorization.policy.path", cliPath))
	require.NoError(t, cmd.Flags().Set("authorization.policy.package", "my.authz"))
	require.NoError(t, cmd.Flags().Set("authorization.policy.decision", "allow"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	assert.Equal(t, cliPath, cfg.Authorization.Policy.Path,
		"--authorization.policy.path CLI flag must override EXTPROC_AUTHORIZATION_POLICY_PATH env var")
	assert.Equal(t, "my.authz", cfg.Authorization.Policy.Package,
		"--authorization.policy.package CLI flag must override EXTPROC_AUTHORIZATION_POLICY_PACKAGE env var")
	assert.Equal(t, "allow", cfg.Authorization.Policy.Decision,
		"--authorization.policy.decision CLI flag must override EXTPROC_AUTHORIZATION_POLICY_DECISION env var")
}

func TestLoadFromViper_EnvVarOverridesDefault(t *testing.T) {
	// Set EXTPROC_GRPC_PORT env var — t.Setenv auto-restores after test
	t.Setenv("EXTPROC_GRPC_PORT", "9090")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.GRPC.Port,
		"EXTPROC_GRPC_PORT env var should override the default port")
}

func TestLoadFromViper_MissingRequiredField_ReturnsError(t *testing.T) {
	v := viper.New()
	// Do not set token_endpoint — should fail validation

	_, err := config.LoadFromViper(v)
	require.Error(t, err, "missing required field should return error")
	assert.Contains(t, err.Error(), "oauth2.token_endpoint",
		"error should mention the missing field")
}

func TestLoadFromViper_InvalidConfigFile_ReturnsError(t *testing.T) {
	v := viper.New()
	v.Set("config_path", "/nonexistent/path/config.yaml")

	_, err := config.LoadFromViper(v)
	require.Error(t, err, "nonexistent config file should return error")
}

// ---------------------------------------------------------------------------
// Validate: additional edge cases
// ---------------------------------------------------------------------------

func TestValidate_AllRequiredFieldsMissing(t *testing.T) {
	cfg := &config.Config{}
	err := config.Validate(cfg)
	require.Error(t, err)
	// Error should mention token_endpoint specifically (used by E2E test)
	assert.Contains(t, err.Error(), "token_endpoint")
}

func TestValidate_OptionalClientCredentialsEndpointNotRequired(t *testing.T) {
	cfg := validConfig()
	cfg.OAuth2.ClientCredentialsEndpoint = "" // optional field
	err := config.Validate(cfg)
	assert.NoError(t, err, "client_credentials_endpoint is optional")
}

// ---------------------------------------------------------------------------
// LoadWithCommand: CLI flag precedence tests
// ---------------------------------------------------------------------------

// newTestCommand creates a minimal Cobra command registered with the same flags
// as the extproc-token-exchange binary via RegisterFlags, ensuring tests always
// stay in sync with the production flag set.
func newTestCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	config.RegisterFlags(cmd)
	return cmd
}

func TestLoadWithCommand_CLIFlagOverridesEnvVar(t *testing.T) {
	// Set an env var that would normally take effect.
	t.Setenv("EXTPROC_GRPC_PORT", "9090")
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")

	cmd := newTestCommand()
	// Simulate the user passing --grpc.port on the CLI.
	require.NoError(t, cmd.Flags().Set("grpc.port", "7777"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	// CLI flag wins over env var.
	assert.Equal(t, 7777, cfg.GRPC.Port,
		"CLI --grpc.port should override EXTPROC_GRPC_PORT env var")
	// Other env vars are unaffected.
	assert.Equal(t, "https://env.example.com/token", cfg.OAuth2.TokenEndpoint)
}

func TestLoadWithCommand_UnsetFlagsDoNotShadowEnvVars(t *testing.T) {
	// Set all required fields via env vars.
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_GRPC_PORT", "8888")

	// Create a command but do NOT set any flags explicitly.
	cmd := newTestCommand()

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	// Env var should still win over the Cobra default (0).
	assert.Equal(t, 8888, cfg.GRPC.Port,
		"unset CLI flag must not shadow EXTPROC_GRPC_PORT env var")
}

func TestLoadWithCommand_CLIFlagOverridesDuration(t *testing.T) {
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("cache.default_ttl", "30m"))
	require.NoError(t, cmd.Flags().Set("cache.max_ttl", "2h"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	assert.Equal(t, 30*time.Minute, cfg.Cache.DefaultTTL,
		"CLI --cache.default_ttl should override the default")
	assert.Equal(t, 2*time.Hour, cfg.Cache.MaxTTL,
		"CLI --cache.max_ttl should override the default")
}

func TestLoadWithCommand_LogLevelAndFormatFromCLI(t *testing.T) {
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("log.level", "debug"))
	require.NoError(t, cmd.Flags().Set("log.format", "json"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	assert.Equal(t, "debug", cfg.Log.Level, "CLI --log.level should take effect")
	assert.Equal(t, "json", cfg.Log.Format, "CLI --log.format should take effect")
}

// TestLoadWithCommand_AuthorizationNumericAndBoolFlagsOverrideEnv verifies CLI-vs-env
// precedence for authorization.enabled (bool), authorization.evaluation_timeout (duration),
// and authorization.max_body_size (int). Authorization is kept disabled so that path
// validation rules (A1-A4) do not run.
func TestLoadWithCommand_AuthorizationNumericAndBoolFlagsOverrideEnv(t *testing.T) {
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")

	// Set env vars for authorization bindings — CLI flags must win over these.
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "false")
	t.Setenv("EXTPROC_AUTHORIZATION_EVALUATION_TIMEOUT", "50ms")
	t.Setenv("EXTPROC_AUTHORIZATION_MAX_BODY_SIZE", "512")

	cmd := newTestCommand()
	// Do not enable authorization so path validation is not triggered.
	require.NoError(t, cmd.Flags().Set("authorization.evaluation_timeout", "250ms"))
	require.NoError(t, cmd.Flags().Set("authorization.max_body_size", "8192"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)

	assert.Equal(t, 250*time.Millisecond, cfg.Authorization.EvaluationTimeout,
		"--authorization.evaluation_timeout CLI flag must override EXTPROC_AUTHORIZATION_EVALUATION_TIMEOUT env var")
	assert.Equal(t, 8192, cfg.Authorization.MaxBodySize,
		"--authorization.max_body_size CLI flag must override EXTPROC_AUTHORIZATION_MAX_BODY_SIZE env var")
}

// TestLoadWithCommand_AuthorizationEnabledFlagOverridesEnv verifies that
// --authorization.enabled CLI flag overrides the EXTPROC_AUTHORIZATION_ENABLED env var.
// A policy.path pointing to an existing file is required because path validation runs
// when authorization.enabled is true.
func TestLoadWithCommand_AuthorizationEnabledFlagOverridesEnv(t *testing.T) {
	// Create a temporary Rego policy file to satisfy path validation.
	policyFile, err := os.CreateTemp(t.TempDir(), "policy-*.rego")
	require.NoError(t, err)
	_, _ = policyFile.WriteString(`package aib.extproc.authz
default result = {"action": "deny"}`)
	require.NoError(t, policyFile.Close())

	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "false")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("authorization.enabled", "true"))
	require.NoError(t, cmd.Flags().Set("authorization.policy.path", policyFile.Name()))

	cfg, loadErr := config.LoadWithCommand(cmd)
	require.NoError(t, loadErr)

	assert.True(t, cfg.Authorization.Enabled,
		"--authorization.enabled CLI flag must override EXTPROC_AUTHORIZATION_ENABLED env var")
}

// TestLoadWithCommand_AuthorizationConfigFileFlagOverridesEnv verifies that
// --authorization.policy.config_file CLI flag overrides the env var.
func TestLoadWithCommand_AuthorizationConfigFileFlagOverridesEnv(t *testing.T) {
	// Create a minimal OPA config YAML file.
	cfgFile, err := os.CreateTemp(t.TempDir(), "opa-config-*.yaml")
	require.NoError(t, err)
	_, _ = cfgFile.WriteString("services: {}\n")
	require.NoError(t, cfgFile.Close())

	envFile, err := os.CreateTemp(t.TempDir(), "env-opa-config-*.yaml")
	require.NoError(t, err)
	_, _ = envFile.WriteString("services: {}\n")
	require.NoError(t, envFile.Close())

	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE", envFile.Name())

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("authorization.policy.config_file", cfgFile.Name()))

	cfg, loadErr := config.LoadWithCommand(cmd)
	require.NoError(t, loadErr)

	assert.Equal(t, cfgFile.Name(), cfg.Authorization.Policy.ConfigFile,
		"--authorization.policy.config_file CLI flag must override EXTPROC_AUTHORIZATION_POLICY_CONFIG_FILE env var")
}

// TestLoadWithCommand_AuthorizationDefaultDecisionFlagOverridesEnv verifies that
// --authorization.default_decision CLI flag can override an invalid env var with the only valid value.
func TestLoadWithCommand_AuthorizationDefaultDecisionFlagOverridesEnv(t *testing.T) {
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	// Env sets an invalid fail-open value; CLI sets the only supported fail-closed value.
	t.Setenv("EXTPROC_AUTHORIZATION_DEFAULT_DECISION", "allow")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("authorization.default_decision", "deny"))

	cfg, loadErr := config.LoadWithCommand(cmd)
	require.NoError(t, loadErr)

	assert.Equal(t, "deny", cfg.Authorization.DefaultDecision,
		"--authorization.default_decision CLI flag must override EXTPROC_AUTHORIZATION_DEFAULT_DECISION env var")
}

// replaceDotsWithUnderscores returns a string replacer for Viper key mapping.
// Used in tests that set up their own Viper instance with env prefix.
func replaceDotsWithUnderscores() *strings.Replacer {
	return strings.NewReplacer(".", "_")
}

// ---------------------------------------------------------------------------
// TelemetryConfig: Loading with defaults, env vars, validation
// ---------------------------------------------------------------------------

func TestLoadFromViper_TelemetryDefaults(t *testing.T) {
	v := viper.New()
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	// Verify telemetry defaults
	assert.False(t, cfg.Telemetry.Enabled, "telemetry should be disabled by default")
	assert.Equal(t, "extproc-token-exchange", cfg.Telemetry.ServiceName)
	assert.Empty(t, cfg.Telemetry.ResourceAttributes)
	assert.True(t, cfg.Telemetry.Traces.Enabled)
	assert.Equal(t, 1.0, cfg.Telemetry.Traces.SamplingRate)
	assert.Equal(t, []string{"tracecontext", "ottrace", "b3multi", "baggage"}, cfg.Telemetry.Traces.Propagators)
	assert.True(t, cfg.Telemetry.Metrics.Enabled)
	assert.Equal(t, 30*time.Second, cfg.Telemetry.Metrics.ExportInterval)
	assert.True(t, cfg.Telemetry.Logs.Enabled)
	assert.Equal(t, "grpc", cfg.Telemetry.Exporter.Protocol)
	assert.Equal(t, "", cfg.Telemetry.Exporter.Endpoint)
	assert.Equal(t, 10*time.Second, cfg.Telemetry.Exporter.Timeout)
	assert.False(t, cfg.Telemetry.Exporter.Insecure)
	assert.Equal(t, "none", cfg.Telemetry.Exporter.Compression)
}

func TestLoadFromViper_TelemetryEnabledEnvVar(t *testing.T) {
	t.Setenv("EXTPROC_TELEMETRY_ENABLED", "true")
	t.Setenv("EXTPROC_TELEMETRY_EXPORTER_ENDPOINT", "collector:4317")
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "test-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "test-secret")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.True(t, cfg.Telemetry.Enabled)
	assert.Equal(t, "collector:4317", cfg.Telemetry.Exporter.Endpoint)
}

func TestLoadFromViper_TelemetryExporterEndpointEnvVar(t *testing.T) {
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://idp.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://idp.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "test-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "test-secret")
	t.Setenv("EXTPROC_TELEMETRY_EXPORTER_ENDPOINT", "otel-collector.monitoring.svc:4317")

	v := viper.New()
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(replaceDotsWithUnderscores())
	v.AutomaticEnv()

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	assert.Equal(t, "otel-collector.monitoring.svc:4317", cfg.Telemetry.Exporter.Endpoint)
}

func TestValidate_TelemetryEnabledWithoutEndpoint_Fails(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Endpoint = ""

	err := config.Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telemetry.exporter.endpoint")
}

func TestValidate_TelemetryEnabledWithEndpoint_Passes(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Endpoint = "collector:4317"

	err := config.Validate(cfg)
	assert.NoError(t, err)
}

func TestValidate_InvalidExporterProtocol_Fails(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Endpoint = "collector:4317"
	cfg.Telemetry.Exporter.Protocol = "ftp"

	err := config.Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telemetry.exporter.protocol")
}

func TestValidate_ValidExporterProtocols_Pass(t *testing.T) {
	testCases := []struct {
		protocol string
		endpoint string
	}{
		{"grpc", "collector:4317"},
		{"http", "http://localhost:4318"},
		{"https", "https://localhost:4318"},
		{"https", "collector.example.com:4318"}, // bare host:port for https
	}
	for _, tc := range testCases {
		t.Run(tc.protocol+":"+tc.endpoint, func(t *testing.T) {
			cfg := validConfig()
			cfg.Telemetry.Enabled = true
			cfg.Telemetry.Exporter.Protocol = tc.protocol
			cfg.Telemetry.Exporter.Endpoint = tc.endpoint

			err := config.Validate(cfg)
			assert.NoError(t, err, "protocol %s with endpoint %s should be valid", tc.protocol, tc.endpoint)
		})
	}
}

func TestValidate_SamplingRateOutOfRange_Fails(t *testing.T) {
	tests := []struct {
		name string
		rate float64
	}{
		{"negative", -0.1},
		{"above 1.0", 1.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Telemetry.Enabled = true
			cfg.Telemetry.Exporter.Endpoint = "collector:4317"
			cfg.Telemetry.Traces.SamplingRate = tt.rate

			err := config.Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "sampling_rate")
		})
	}
}

func TestValidate_SamplingRateInRange_Passes(t *testing.T) {
	validRates := []float64{0.0, 0.5, 1.0}
	for _, rate := range validRates {
		t.Run(fmt.Sprintf("%.1f", rate), func(t *testing.T) {
			cfg := validConfig()
			cfg.Telemetry.Traces.SamplingRate = rate

			err := config.Validate(cfg)
			assert.NoError(t, err)
		})
	}
}

func TestValidate_TelemetryDisabledIgnoresInvalidSettings_Passes(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = false
	cfg.Telemetry.Exporter.Protocol = "invalid" // Should be ignored
	cfg.Telemetry.Traces.SamplingRate = 1.5     // Should be ignored
	cfg.Telemetry.Exporter.Endpoint = ""        // Should be ignored

	err := config.Validate(cfg)
	assert.NoError(t, err, "when telemetry is disabled, other telemetry settings should not cause validation failure")
}

func TestValidate_MalformedHTTPEndpoint_Fails(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Protocol = "http"
	cfg.Telemetry.Exporter.Endpoint = "not-a-url"

	err := config.Validate(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "telemetry.exporter.endpoint")
}

func TestValidate_ValidHTTPEndpoint_Passes(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Protocol = "http"
	cfg.Telemetry.Exporter.Endpoint = "http://localhost:4318"

	err := config.Validate(cfg)
	assert.NoError(t, err)
}

func TestValidate_GRPCEndpointRejectsHTTPURL(t *testing.T) {
	for _, endpoint := range []string{
		"http://localhost:4317",
		"https://localhost:4317",
		"HTTP://localhost:4317",
		"Https://collector:4317",
	} {
		cfg := validConfig()
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Exporter.Protocol = "grpc"
		cfg.Telemetry.Exporter.Endpoint = endpoint

		err := config.Validate(cfg)
		assert.Error(t, err, "gRPC endpoint %q should be rejected", endpoint)
		assert.Contains(t, err.Error(), "not an HTTP URL")
	}
}

func TestValidate_GRPCEndpointAcceptsUnixPath(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Protocol = "grpc"
	cfg.Telemetry.Exporter.Endpoint = "unix:/var/run/otel.sock"

	err := config.Validate(cfg)
	assert.NoError(t, err, "gRPC endpoint unix:path should be accepted")
}

func TestValidate_GRPCEndpointRejectsUnixDoubleSlash(t *testing.T) {
	cfg := validConfig()
	cfg.Telemetry.Enabled = true
	cfg.Telemetry.Exporter.Protocol = "grpc"
	cfg.Telemetry.Exporter.Endpoint = "unix:///var/run/otel.sock"

	err := config.Validate(cfg)
	assert.Error(t, err, "gRPC endpoint unix://path should be rejected")
	assert.Contains(t, err.Error(), "unix:path format")
}

func TestValidate_GRPCEndpointRejectsMixedCaseUnix(t *testing.T) {
	for _, endpoint := range []string{
		"UNIX:/var/run/otel.sock",
		"Unix:/var/run/otel.sock",
		"uNiX:/var/run/otel.sock",
	} {
		cfg := validConfig()
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Exporter.Protocol = "grpc"
		cfg.Telemetry.Exporter.Endpoint = endpoint

		err := config.Validate(cfg)
		assert.Error(t, err, "gRPC endpoint %q with mixed-case unix: should be rejected", endpoint)
		assert.Contains(t, err.Error(), "lowercase")
	}
}

func TestValidate_HTTPSEndpointRejectsMixedCaseHTTP(t *testing.T) {
	for _, endpoint := range []string{
		"http://collector:4318",
		"HTTP://collector:4318",
		"Http://collector:4318",
		"ftp://collector:4318",
		"grpc://collector:4318",
	} {
		cfg := validConfig()
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Exporter.Protocol = "https"
		cfg.Telemetry.Exporter.Endpoint = endpoint

		err := config.Validate(cfg)
		assert.Error(t, err, "HTTPS endpoint %q with non-https scheme should be rejected", endpoint)
	}
}

func TestValidate_HTTPSEndpointAcceptsMixedCaseHTTPS(t *testing.T) {
	for _, endpoint := range []string{
		"https://collector:4318",
		"HTTPS://collector:4318",
		"Https://collector:4318",
	} {
		cfg := validConfig()
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Exporter.Protocol = "https"
		cfg.Telemetry.Exporter.Endpoint = endpoint

		err := config.Validate(cfg)
		assert.NoError(t, err, "HTTPS endpoint %q should be accepted", endpoint)
	}
}

func TestValidate_HTTPSEndpointRejectsEmptyPort(t *testing.T) {
	for _, endpoint := range []string{
		"collector:",          // trailing colon, no port
		":4318",               // no host
		":",                   // neither host nor port
		"collector",           // no colon at all
		"/var/run.sock",       // path, not host:port
		"collector:4318/path", // path suffix after port
		"collector:abc",       // non-numeric port
		"[::1]",               // bracketed IPv6 without port
		"collector:0",         // port below valid range
		"collector:65536",     // port above valid range
		"collector:-1",        // negative port
	} {
		cfg := validConfig()
		cfg.Telemetry.Enabled = true
		cfg.Telemetry.Exporter.Protocol = "https"
		cfg.Telemetry.Exporter.Endpoint = endpoint

		err := config.Validate(cfg)
		assert.Error(t, err, "HTTPS bare endpoint %q should be rejected", endpoint)
	}
}

func TestLoadFromViper_TelemetryExporterHeadersStructureSupported(t *testing.T) {
	// This test verifies that the config structure supports headers and env var expansion
	// is properly applied via the expandEnvVars() function (SR-002).
	t.Setenv("TELEMETRY_SECRET", "secret-value-123")

	v := viper.New()
	v.Set("oauth2.token_endpoint", "https://idp.example.com/oauth2/token")
	v.Set("oauth2.issuer", "https://idp.example.com")
	v.Set("oauth2.client_id", "test-client")
	v.Set("oauth2.client_secret", "test-secret")
	v.Set("telemetry.enabled", true)
	v.Set("telemetry.exporter.endpoint", "collector.example.com:4317")
	v.Set("telemetry.exporter.protocol", "grpc")
	v.Set("telemetry.exporter.headers.authorization", "${TELEMETRY_SECRET}")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)

	// Verify headers structure exists and env var was expanded
	assert.NotNil(t, cfg.Telemetry.Exporter.Headers)
	assert.Equal(t, "secret-value-123", cfg.Telemetry.Exporter.Headers["authorization"],
		"telemetry header should have expanded the env var")
}
