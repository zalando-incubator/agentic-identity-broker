// Package config implements the configuration loading adapter.
package config

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	mapstructure "github.com/go-viper/mapstructure/v2"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Loader implements the ConfigPort interface using Viper, Cobra, and godotenv.
// Each Loader instance has its own Viper instance for proper isolation.
type Loader struct {
	v       *viper.Viper
	sources []ports.ConfigSource
	cmd     *cobra.Command
}

// NewLoader creates a new configuration loader with instance-scoped Viper.
// This prevents test conflicts from global Viper state.
func NewLoader() *Loader {
	return &Loader{
		v:       viper.New(),
		sources: make([]ports.ConfigSource, 0),
	}
}

// SetCommand sets the Cobra command for CLI flag binding.
// Must be called before GetConfig if CLI flags should be used.
func (l *Loader) SetCommand(cmd *cobra.Command) {
	l.cmd = cmd
}

// GetConfig returns the fully loaded and validated configuration.
// Implements ports.ConfigPort interface.
func (l *Loader) GetConfig(ctx context.Context) (*ports.Config, error) {
	// Phase 1: Set defaults
	l.setDefaults()

	// Phase 2: Load .env files
	if err := l.loadEnvFiles(); err != nil {
		return nil, err
	}

	// Phase 3: Load YAML (User Story 2)
	if err := l.loadYAML(); err != nil {
		return nil, err
	}

	// Phase 4: Expand environment variables (User Story 2)
	if err := l.expandEnvVars(); err != nil {
		return nil, err
	}

	// Phase 5: Bind CLI flags (User Story 3)
	if err := l.bindFlags(); err != nil {
		return nil, err
	}

	// Unmarshal to Config struct
	var cfg ports.Config
	if err := l.v.Unmarshal(&cfg, func(c *mapstructure.DecoderConfig) {
		c.DecodeHook = mapstructure.ComposeDecodeHookFunc(rejectNumericDurationHook(), c.DecodeHook)
	}); err != nil {
		return nil, &config.ConfigError{
			Expected: "valid configuration structure",
			Err:      err,
		}
	}

	// Phase 6: Validate configuration (User Story 4)
	if err := Validate(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// GetSources returns metadata about all configuration sources used.
// Implements ports.ConfigPort interface.
func (l *Loader) GetSources() []ports.ConfigSource {
	return l.sources
}

// Reload reloads configuration from all sources.
// NOT IMPLEMENTED in initial scope (no hot-reloading).
func (l *Loader) Reload(ctx context.Context) error {
	return fmt.Errorf("reload not implemented in initial scope")
}

// setDefaults sets default configuration values.
// Records source metadata for audit logging.
func (l *Loader) setDefaults() {
	// Log configuration defaults
	l.v.SetDefault("log.level", string(config.LogLevelInfo))
	l.v.SetDefault("log.format", string(config.LogFormatText))

	// Server configuration defaults
	serverDefaults := ports.DefaultServerConfig()
	l.v.SetDefault("server.enduser.port", serverDefaults.EndUser.Port)
	l.v.SetDefault("server.enduser.bind", serverDefaults.EndUser.Bind)
	l.v.SetDefault("server.enduser.public_url", serverDefaults.EndUser.PublicURL)
	// No default for principal_header_name — must be explicitly configured to avoid trust boundary exposure
	l.v.SetDefault("server.admin.port", serverDefaults.Admin.Port)
	l.v.SetDefault("server.admin.bind", serverDefaults.Admin.Bind)
	l.v.SetDefault("server.admin.public_url", serverDefaults.Admin.PublicURL)
	// No default for principal_header_name — must be explicitly configured to avoid trust boundary exposure
	l.v.SetDefault("server.shutdown.timeout", serverDefaults.Shutdown.Timeout)

	// Storage configuration defaults
	l.v.SetDefault("storage.backend", "memory")
	l.v.SetDefault("storage.timeouts.read", "5s")
	l.v.SetDefault("storage.timeouts.write", "10s")

	requestContextDefaults := ports.DefaultRequestContextConfig()
	l.v.SetDefault("request_context.trusted_proxy.enabled", requestContextDefaults.TrustedProxy.Enabled)
	l.v.SetDefault("request_context.trusted_proxy.forwarded_header", requestContextDefaults.TrustedProxy.ForwardedHeader)
	l.v.SetDefault("request_context.trace.response_enabled", requestContextDefaults.Trace.ResponseEnabled)

	// Bind environment variables explicitly
	// This ensures env vars override YAML config (proper precedence)
	// Note: BindEnv errors are not critical - viper will continue with defaults
	_ = l.v.BindEnv("log.level", "IDENTITY_BROKER_LOG_LEVEL")
	_ = l.v.BindEnv("log.format", "IDENTITY_BROKER_LOG_FORMAT")
	_ = l.v.BindEnv("server.enduser.port", "IDENTITY_BROKER_SERVER_ENDUSER_PORT")
	_ = l.v.BindEnv("server.enduser.bind", "IDENTITY_BROKER_SERVER_ENDUSER_BIND")
	_ = l.v.BindEnv("server.enduser.public_url", "IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL")
	_ = l.v.BindEnv("server.enduser.authentication.preauth.principal_header_name", "IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME")
	_ = l.v.BindEnv("server.admin.port", "IDENTITY_BROKER_SERVER_ADMIN_PORT")
	_ = l.v.BindEnv("server.admin.bind", "IDENTITY_BROKER_SERVER_ADMIN_BIND")
	_ = l.v.BindEnv("server.admin.public_url", "IDENTITY_BROKER_SERVER_ADMIN_PUBLIC_URL")
	_ = l.v.BindEnv("server.admin.authentication.preauth.principal_header_name", "IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME")
	_ = l.v.BindEnv("server.shutdown.timeout", "IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT")
	// CORS configuration (list values are best set via YAML; these env vars cover single-origin scenarios)
	_ = l.v.BindEnv("server.enduser.cors.max_age", "IDENTITY_BROKER_SERVER_ENDUSER_CORS_MAX_AGE")
	_ = l.v.BindEnv("server.admin.cors.max_age", "IDENTITY_BROKER_SERVER_ADMIN_CORS_MAX_AGE")
	_ = l.v.BindEnv("storage.backend", "IDENTITY_BROKER_STORAGE_BACKEND")
	_ = l.v.BindEnv("storage.postgres.connection_url", "IDENTITY_BROKER_STORAGE_POSTGRES_URL")
	_ = l.v.BindEnv("request_context.trusted_proxy.enabled", "IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_ENABLED")
	_ = l.v.BindEnv("request_context.trusted_proxy.forwarded_header", "IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_FORWARDED_HEADER")
	_ = l.v.BindEnv("request_context.trace.response_enabled", "IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED")
	_ = l.v.BindEnv("third_party_oauth2.jwe_signing_key", "IDENTITY_BROKER_JWE_SIGNING_KEY")
	_ = l.v.BindEnv("third_party_oauth2.state_token_ttl", "IDENTITY_BROKER_STATE_TOKEN_TTL")
	_ = l.v.BindEnv("third_party_oauth2.pkce_verifier_length", "IDENTITY_BROKER_PKCE_VERIFIER_LENGTH")
	// Bind encryption configuration to environment variables
	// AWS KMS backend - KMS key and DynamoDB cache configuration
	_ = l.v.BindEnv("encryption.aws_kms.key_arn", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN")
	_ = l.v.BindEnv("encryption.aws_kms.dynamodb_table_name", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME")
	_ = l.v.BindEnv("encryption.aws_kms.branch_key_ttl", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL")
	_ = l.v.BindEnv("encryption.aws_kms.dynamodb_region", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_REGION")
	_ = l.v.BindEnv("encryption.aws_kms.dynamodb_timeout", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT")
	// AWS SDK configuration (region, endpoints, credentials, role assumption)
	_ = l.v.BindEnv("encryption.aws_kms.region", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_REGION")
	_ = l.v.BindEnv("encryption.aws_kms.kms_endpoint", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ENDPOINT")
	_ = l.v.BindEnv("encryption.aws_kms.dynamodb_endpoint", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_ENDPOINT")
	_ = l.v.BindEnv("encryption.aws_kms.profile", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_PROFILE")
	_ = l.v.BindEnv("encryption.aws_kms.access_key_id", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ACCESS_KEY_ID")
	_ = l.v.BindEnv("encryption.aws_kms.secret_access_key", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_SECRET_ACCESS_KEY")
	_ = l.v.BindEnv("encryption.aws_kms.assume_role_arn", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ASSUME_ROLE_ARN")
	_ = l.v.BindEnv("encryption.aws_kms.disable_ssl", "IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL")
	// Memory backend
	_ = l.v.BindEnv("encryption.memory.raw_key", "IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY")

	// Set OAuth2 configuration defaults
	l.v.SetDefault("third_party_oauth2.state_token_ttl", "10m")
	l.v.SetDefault("third_party_oauth2.pkce_verifier_length", 32)

	// Note: No encryption configuration defaults set here to avoid creating
	// both backend structs. Defaults are handled in the adapter factory functions.

	// Set security configuration defaults
	l.v.SetDefault("security.skip_thirdparty_https_validation", false)

	// Approval configuration defaults
	l.v.SetDefault("approvals.pending_ttl", "10m")
	l.v.SetDefault("approvals.sync_coalesce_window", "1s")
	l.v.SetDefault("approvals.rate_limit.max_pending_per_pair", 50)
	l.v.SetDefault("approvals.rate_limit.max_requests_per_minute", 10)
	_ = l.v.BindEnv("approvals.pending_ttl", "APPROVAL_PENDING_TTL")
	_ = l.v.BindEnv("approvals.sync_coalesce_window", "APPROVAL_SYNC_COALESCE_WINDOW")
	_ = l.v.BindEnv("approvals.rate_limit.max_pending_per_pair", "APPROVAL_RATE_LIMIT_MAX_PENDING")
	_ = l.v.BindEnv("approvals.rate_limit.max_requests_per_minute", "APPROVAL_RATE_LIMIT_REQUESTS_PER_MINUTE")

	// Telemetry configuration defaults
	telDefaults := ports.DefaultTelemetryConfig()
	l.v.SetDefault("telemetry.enabled", telDefaults.Enabled)
	l.v.SetDefault("telemetry.service_name", telDefaults.ServiceName)
	l.v.SetDefault("telemetry.traces.enabled", telDefaults.Traces.Enabled)
	l.v.SetDefault("telemetry.traces.sampling_rate", telDefaults.Traces.SamplingRate)
	l.v.SetDefault("telemetry.traces.propagators", telDefaults.Traces.Propagators)
	l.v.SetDefault("telemetry.metrics.enabled", telDefaults.Metrics.Enabled)
	l.v.SetDefault("telemetry.metrics.export_interval", telDefaults.Metrics.ExportInterval)
	l.v.SetDefault("telemetry.logs.enabled", telDefaults.Logs.Enabled)
	l.v.SetDefault("telemetry.exporter.protocol", telDefaults.Exporter.Protocol)
	l.v.SetDefault("telemetry.exporter.timeout", telDefaults.Exporter.Timeout)
	l.v.SetDefault("telemetry.exporter.insecure", telDefaults.Exporter.Insecure)
	l.v.SetDefault("telemetry.exporter.compression", telDefaults.Exporter.Compression)

	// Bind telemetry env vars
	_ = l.v.BindEnv("telemetry.enabled", "IDENTITY_BROKER_TELEMETRY_ENABLED")
	_ = l.v.BindEnv("telemetry.service_name", "IDENTITY_BROKER_TELEMETRY_SERVICE_NAME")
	_ = l.v.BindEnv("telemetry.traces.enabled", "IDENTITY_BROKER_TELEMETRY_TRACES_ENABLED")
	_ = l.v.BindEnv("telemetry.traces.sampling_rate", "IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE")
	_ = l.v.BindEnv("telemetry.metrics.enabled", "IDENTITY_BROKER_TELEMETRY_METRICS_ENABLED")
	_ = l.v.BindEnv("telemetry.metrics.export_interval", "IDENTITY_BROKER_TELEMETRY_METRICS_EXPORT_INTERVAL")
	_ = l.v.BindEnv("telemetry.logs.enabled", "IDENTITY_BROKER_TELEMETRY_LOGS_ENABLED")
	_ = l.v.BindEnv("telemetry.exporter.protocol", "IDENTITY_BROKER_TELEMETRY_EXPORTER_PROTOCOL")
	_ = l.v.BindEnv("telemetry.exporter.endpoint", "IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT")
	_ = l.v.BindEnv("telemetry.exporter.timeout", "IDENTITY_BROKER_TELEMETRY_EXPORTER_TIMEOUT")
	_ = l.v.BindEnv("telemetry.exporter.insecure", "IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE")
	_ = l.v.BindEnv("telemetry.exporter.compression", "IDENTITY_BROKER_TELEMETRY_EXPORTER_COMPRESSION")

	// Bind OAuth2 authorization server env vars
	_ = l.v.BindEnv("oauth2_authorization_server.mode", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "IDENTITY_BROKER_OAUTH2_MODE")
	_ = l.v.BindEnv("oauth2_authorization_server.proxy.upstream_issuer_uri", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_ISSUER_URI")
	_ = l.v.BindEnv("oauth2_authorization_server.proxy.upstream_authorize_endpoint", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_AUTHORIZE_ENDPOINT")
	_ = l.v.BindEnv("oauth2_authorization_server.proxy.upstream_token_endpoint", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TOKEN_ENDPOINT")
	_ = l.v.BindEnv("oauth2_authorization_server.proxy.upstream_timeout", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TIMEOUT")
	_ = l.v.BindEnv("oauth2_authorization_server.supported_scopes", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_SUPPORTED_SCOPES")
	_ = l.v.BindEnv("oauth2_authorization_server.local.refresh_token_ttl", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL")
	_ = l.v.BindEnv("oauth2_authorization_server.local.token_ttl", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_TTL")
	_ = l.v.BindEnv("oauth2_authorization_server.local.token_claims_expression", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_CLAIMS_EXPRESSION")
	_ = l.v.BindEnv("oauth2_authorization_server.local.signing_keys.bootstrap_timeout", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_SIGNING_KEYS_BOOTSTRAP_TIMEOUT")

	// Set CIMD configuration defaults
	cimdDefaults := ports.DefaultCIMDConfig()
	l.v.SetDefault("oauth2_authorization_server.cimd.enabled", cimdDefaults.Enabled)
	l.v.SetDefault("oauth2_authorization_server.cimd.fetch_timeout", cimdDefaults.FetchTimeout)
	l.v.SetDefault("oauth2_authorization_server.cimd.max_response_bytes", cimdDefaults.MaxResponseBytes)
	l.v.SetDefault("oauth2_authorization_server.cimd.cache.max_ttl", cimdDefaults.Cache.MaxTTL)
	l.v.SetDefault("oauth2_authorization_server.cimd.cache.min_ttl", cimdDefaults.Cache.MinTTL)
	l.v.SetDefault("oauth2_authorization_server.cimd.cache.max_entries", cimdDefaults.Cache.MaxEntries)

	// Bind CIMD env vars
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.enabled", "IDENTITY_BROKER_CIMD_ENABLED")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.fetch_timeout", "IDENTITY_BROKER_CIMD_FETCH_TIMEOUT")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.max_response_bytes", "IDENTITY_BROKER_CIMD_MAX_RESPONSE_BYTES")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.cache.max_ttl", "IDENTITY_BROKER_CIMD_CACHE_MAX_TTL")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.cache.min_ttl", "IDENTITY_BROKER_CIMD_CACHE_MIN_TTL")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.cache.max_entries", "IDENTITY_BROKER_CIMD_CACHE_MAX_ENTRIES")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.ssrf.extra_blocked_cidrs", "IDENTITY_BROKER_CIMD_SSRF_EXTRA_BLOCKED_CIDRS")
	_ = l.v.BindEnv("oauth2_authorization_server.cimd.client_name_blocklist", "IDENTITY_BROKER_CIMD_CLIENT_NAME_BLOCKLIST")

	// Set token exchange configuration defaults
	// expected_audience defaults to the well-known "token-exchange-broker" value.
	// Operators can override it to match whatever audience their JWTs carry.
	l.v.SetDefault("token_exchange.expected_audience", tokenexchange.DefaultBrokerAudience)
	_ = l.v.BindEnv("token_exchange.expected_audience", "IDENTITY_BROKER_TOKEN_EXCHANGE_EXPECTED_AUDIENCE")
	_ = l.v.BindEnv("token_exchange.client_assertion.issuer_uri", "IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_ISSUER_URI")
	_ = l.v.BindEnv("token_exchange.client_assertion.jwks_uri", "IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_URI")
	_ = l.v.BindEnv("token_exchange.client_assertion.jwks_min_refresh", "IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_MIN_REFRESH")
	_ = l.v.BindEnv("token_exchange.client_assertion.jwks_max_refresh", "IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_MAX_REFRESH")

	// Record defaults source
	l.sources = append(l.sources, ports.ConfigSource{
		Type:       ports.SourceTypeDefault,
		Path:       "defaults",
		Precedence: 0,
		LoadedAt:   time.Now(),
		Keys: []string{
			"log.level", "log.format",
			"server.enduser.port", "server.enduser.bind", "server.enduser.public_url",
			"server.admin.port", "server.admin.bind", "server.admin.public_url",
			"server.shutdown.timeout",
			"storage.backend", "storage.timeouts.read", "storage.timeouts.write",
			"request_context.trusted_proxy.enabled",
			"request_context.trusted_proxy.forwarded_header",
			"request_context.trace.response_enabled",
			"third_party_oauth2.state_token_ttl", "third_party_oauth2.pkce_verifier_length",
			"security.skip_thirdparty_https_validation",
			"telemetry.enabled", "telemetry.service_name",
			"telemetry.traces.enabled", "telemetry.traces.sampling_rate", "telemetry.traces.propagators",
			"telemetry.metrics.enabled", "telemetry.metrics.export_interval",
			"telemetry.exporter.protocol", "telemetry.exporter.timeout", "telemetry.exporter.insecure",
			"telemetry.logs.enabled",
			"telemetry.exporter.compression",
			"token_exchange.expected_audience",
		},
	})
}

// loadEnvFiles loads .env files in correct precedence order.
// Order: .env → .env.local → .env.{environment} → .env.{environment}.local
// Records source metadata for each loaded file.
func (l *Loader) loadEnvFiles() error {
	// Determine environment
	env := os.Getenv("GO_ENV")
	if env == "" {
		env = "development"
	}

	// Define .env file loading order
	envFiles := []string{
		".env",
		".env.local",
		fmt.Sprintf(".env.%s", env),
		fmt.Sprintf(".env.%s.local", env),
	}

	// Load each .env file if it exists
	for _, filename := range envFiles {
		if err := l.loadEnvFile(filename); err != nil {
			// Continue if file doesn't exist
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
	}

	return nil
}

// loadEnvFile loads a single .env file and records source metadata.
func (l *Loader) loadEnvFile(filename string) error {
	// Get absolute path
	absPath, err := filepath.Abs(filename)
	if err != nil {
		return &config.ConfigError{
			Field:    "env_file",
			Value:    filename,
			Expected: "valid file path",
			Err:      err,
		}
	}

	// Check if file exists
	if _, err := os.Stat(absPath); err != nil { // #nosec G703 -- env file names are local operator configuration paths.
		return err
	}

	// Load .env file
	envMap, err := godotenv.Read(absPath)
	if err != nil {
		return &config.ConfigError{
			Field:    "env_file",
			Value:    absPath,
			Expected: "valid .env file format",
			Err:      err,
		}
	}

	// Set environment variables in both Viper and OS environment
	// OS environment is needed for os.Expand() when expanding ${VAR} references
	keys := make([]string, 0, len(envMap))
	for key, value := range envMap {
		// Set in OS environment (needed for os.Expand() in expandEnvVars phase)
		_ = os.Setenv(key, value)

		// Strip IDENTITY_BROKER_ prefix and convert to Viper format
		viperKey := key
		if after, ok := strings.CutPrefix(strings.ToUpper(key), "IDENTITY_BROKER_"); ok {
			viperKey = after
		}
		// Convert to lowercase with dots
		viperKey = strings.ToLower(strings.ReplaceAll(viperKey, "_", "."))
		l.v.Set(viperKey, value)
		keys = append(keys, viperKey)
	}

	// Record source metadata
	l.sources = append(l.sources, ports.ConfigSource{
		Type:       ports.SourceTypeEnvFile,
		Path:       absPath,
		Precedence: 1,
		LoadedAt:   time.Now(),
		Keys:       keys,
	})

	return nil
}

// loadYAML loads configuration from a YAML file.
// File path is determined by --config flag or IDENTITY_BROKER_CONFIG_PATH env var.
// If neither is set, looks for config.yaml in the current directory.
// Records source metadata for audit logging.
func (l *Loader) loadYAML() error {
	// Determine config file path
	configPath := os.Getenv("IDENTITY_BROKER_CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	// Check if file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) { // #nosec G703 -- config path is local operator configuration, not an HTTP input.
		// Config file is optional
		return nil
	}

	// Get absolute path
	absPath, err := filepath.Abs(configPath)
	if err != nil {
		return &config.ConfigError{
			Field:    "config_file",
			Value:    configPath,
			Expected: "valid file path",
			Err:      err,
		}
	}

	// Set config file in Viper
	l.v.SetConfigFile(absPath)

	// Read config file
	if err := l.v.ReadInConfig(); err != nil {
		// Check for specific error types
		if os.IsPermission(err) {
			return &config.ConfigError{
				Field:    "config_file",
				Value:    absPath,
				Expected: "readable file with proper permissions",
				Source:   "yaml",
				Err:      err,
			}
		}
		return &config.ConfigError{
			Field:    "config_file",
			Value:    absPath,
			Expected: "valid YAML syntax",
			Source:   "yaml",
			Err:      err,
		}
	}

	// Get all keys from the config file
	keys := l.v.AllKeys()

	// Record source metadata
	l.sources = append(l.sources, ports.ConfigSource{
		Type:       ports.SourceTypeYAML,
		Path:       absPath,
		Precedence: 2,
		LoadedAt:   time.Now(),
		Keys:       keys,
	})

	return nil
}

// expandEnvVars expands environment variable references in configuration values.
// Supports ${VAR_NAME} syntax with circular reference detection.
// Validates against command injection patterns.
func (l *Loader) expandEnvVars() error {
	// Get all configuration keys
	allKeys := l.v.AllKeys()

	for _, key := range allKeys {
		value := l.v.GetString(key)
		if value == "" {
			continue
		}

		// Check if value contains environment variable reference
		if !strings.Contains(value, "${") {
			continue
		}

		// Validate against command injection patterns
		if err := l.validateSecurePattern(value); err != nil {
			return &config.ConfigError{
				Field:    key,
				Value:    value,
				Expected: "environment variable reference without command injection patterns",
				Source:   "yaml",
				Err:      err,
			}
		}

		// Expand environment variables with circular detection
		expanded, err := l.expandWithCircularCheck(value, make(map[string]bool), 0)
		if err != nil {
			return &config.ConfigError{
				Field:    key,
				Value:    value,
				Expected: "resolvable environment variable reference",
				Source:   "yaml",
				Err:      err,
			}
		}

		// Update value in Viper
		l.v.Set(key, expanded)
	}

	return nil
}

// expandWithCircularCheck expands environment variables with circular reference detection.
// maxDepth limits recursion to prevent infinite loops.
// visited tracks variables seen in the current expansion chain.
func (l *Loader) expandWithCircularCheck(value string, visited map[string]bool, depth int) (string, error) {
	const maxDepth = 10

	// Check recursion depth
	if depth >= maxDepth {
		return "", fmt.Errorf("environment variable expansion exceeded maximum depth of %d", maxDepth)
	}

	// Track expansion errors
	var expansionErrors []string

	// Expand ${VAR} patterns
	result := os.Expand(value, func(varName string) string {
		// Parse variable name and default value
		// Supports ${VAR:default} syntax
		actualVarName := varName
		defaultValue := ""
		if before, after, found := strings.Cut(varName, ":"); found {
			actualVarName = before
			defaultValue = after
		}

		// Check for circular reference
		if visited[actualVarName] {
			// Build circular chain for error message
			chain := []string{}
			for v := range visited {
				chain = append(chain, v)
			}
			chain = append(chain, actualVarName)
			expansionErrors = append(expansionErrors, fmt.Sprintf("CIRCULAR:%s→%s", strings.Join(chain, "→"), actualVarName))
			return ""
		}

		// Get environment variable value
		varValue, exists := os.LookupEnv(actualVarName)
		if !exists {
			// Use default value if provided
			if defaultValue != "" {
				return defaultValue
			}
			expansionErrors = append(expansionErrors, fmt.Sprintf("UNDEFINED:%s", actualVarName))
			return ""
		}

		// Check if variable value contains more references
		if strings.Contains(varValue, "${") {
			// Mark variable as visited
			newVisited := make(map[string]bool, len(visited))
			maps.Copy(newVisited, visited)
			newVisited[actualVarName] = true

			// Recursively expand
			expanded, err := l.expandWithCircularCheck(varValue, newVisited, depth+1)
			if err != nil {
				expansionErrors = append(expansionErrors, fmt.Sprintf("NESTED:%v", err))
				return ""
			}
			return expanded
		}

		return varValue
	})

	// Check for expansion errors
	if len(expansionErrors) > 0 {
		for _, errMsg := range expansionErrors {
			if chain, ok := strings.CutPrefix(errMsg, "CIRCULAR:"); ok {
				return "", fmt.Errorf("circular reference detected: %s", chain)
			}
			if varName, ok := strings.CutPrefix(errMsg, "UNDEFINED:"); ok {
				return "", fmt.Errorf("environment variable '%s' is not set", varName)
			}
			if nested, ok := strings.CutPrefix(errMsg, "NESTED:"); ok {
				return "", fmt.Errorf("nested expansion error: %s", nested)
			}
		}
	}

	return result, nil
}

// validateSecurePattern validates that configuration values don't contain
// command injection patterns like $(command) or backticks.
func (l *Loader) validateSecurePattern(value string) error {
	// Reject command substitution patterns
	if strings.Contains(value, "$(") || strings.Contains(value, "`") {
		return fmt.Errorf("command substitution patterns $(command) and backticks are not allowed")
	}

	// Check for shell metacharacters that could indicate injection attempts
	dangerousChars := []string{";", "|", "&", ">", "<", "\n", "\r"}
	for _, char := range dangerousChars {
		if strings.Contains(value, char) {
			return fmt.Errorf("potentially dangerous shell metacharacter '%s' detected", char)
		}
	}

	return nil
}

// bindFlags binds Cobra command flags to Viper configuration.
// CLI flags have the highest precedence and override all other sources.
func rejectNumericDurationHook() mapstructure.DecodeHookFuncType {
	durationType := reflect.TypeOf(time.Duration(0))

	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if from == nil || to != durationType {
			return data, nil
		}
		if from == durationType || from.Kind() == reflect.String {
			return data, nil
		}

		switch from.Kind() {
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
			reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
			reflect.Float32, reflect.Float64:
			return nil, fmt.Errorf("duration values must use Go duration strings like '30s' or '500ms', not bare numbers")
		default:
			return data, nil
		}
	}
}

func (l *Loader) bindFlags() error {
	if l.cmd == nil {
		// No command set, skip flag binding
		return nil
	}

	// Track which keys came from CLI flags
	cliKeys := make([]string, 0)

	// Bind config file path flag
	if l.cmd.Flags().Changed("config") {
		configPath, _ := l.cmd.Flags().GetString("config")
		_ = os.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)
	}

	// Bind log.level flag
	if l.cmd.Flags().Changed("log-level") {
		logLevel, _ := l.cmd.Flags().GetString("log-level")
		l.v.Set("log.level", logLevel)
		cliKeys = append(cliKeys, "log.level")
	}

	// Bind log.format flag
	if l.cmd.Flags().Changed("log-format") {
		logFormat, _ := l.cmd.Flags().GetString("log-format")
		l.v.Set("log.format", logFormat)
		cliKeys = append(cliKeys, "log.format")
	}

	// Bind server.enduser.port flag
	if l.cmd.Flags().Changed("server.enduser.port") {
		port, _ := l.cmd.Flags().GetInt("server.enduser.port")
		l.v.Set("server.enduser.port", port)
		cliKeys = append(cliKeys, "server.enduser.port")
	}

	// Bind server.enduser.bind flag
	if l.cmd.Flags().Changed("server.enduser.bind") {
		bind, _ := l.cmd.Flags().GetString("server.enduser.bind")
		l.v.Set("server.enduser.bind", bind)
		cliKeys = append(cliKeys, "server.enduser.bind")
	}

	// Bind server.admin.port flag
	if l.cmd.Flags().Changed("server.admin.port") {
		port, _ := l.cmd.Flags().GetInt("server.admin.port")
		l.v.Set("server.admin.port", port)
		cliKeys = append(cliKeys, "server.admin.port")
	}

	// Bind server.admin.bind flag
	if l.cmd.Flags().Changed("server.admin.bind") {
		bind, _ := l.cmd.Flags().GetString("server.admin.bind")
		l.v.Set("server.admin.bind", bind)
		cliKeys = append(cliKeys, "server.admin.bind")
	}

	// Bind server.shutdown.timeout flag
	if l.cmd.Flags().Changed("server.shutdown.timeout") {
		timeout, _ := l.cmd.Flags().GetDuration("server.shutdown.timeout")
		l.v.Set("server.shutdown.timeout", timeout)
		cliKeys = append(cliKeys, "server.shutdown.timeout")
	}

	// Bind request_context.trusted_proxy.enabled flag
	if l.cmd.Flags().Changed("request_context.trusted_proxy.enabled") {
		enabled, _ := l.cmd.Flags().GetBool("request_context.trusted_proxy.enabled")
		l.v.Set("request_context.trusted_proxy.enabled", enabled)
		cliKeys = append(cliKeys, "request_context.trusted_proxy.enabled")
	}

	// Bind request_context.trusted_proxy.forwarded_header flag
	if l.cmd.Flags().Changed("request_context.trusted_proxy.forwarded_header") {
		header, _ := l.cmd.Flags().GetString("request_context.trusted_proxy.forwarded_header")
		l.v.Set("request_context.trusted_proxy.forwarded_header", header)
		cliKeys = append(cliKeys, "request_context.trusted_proxy.forwarded_header")
	}

	// Bind request_context.trace.response_enabled flag
	if l.cmd.Flags().Changed("request_context.trace.response_enabled") {
		enabled, _ := l.cmd.Flags().GetBool("request_context.trace.response_enabled")
		l.v.Set("request_context.trace.response_enabled", enabled)
		cliKeys = append(cliKeys, "request_context.trace.response_enabled")
	}

	// Record CLI source if any flags were set
	if len(cliKeys) > 0 {
		l.sources = append(l.sources, ports.ConfigSource{
			Type:       ports.SourceTypeCLI,
			Path:       "cli",
			Precedence: 3,
			LoadedAt:   time.Now(),
			Keys:       cliKeys,
		})
	}

	return nil
}
