// Package config — loader.go provides Viper-based configuration loading for the
// ExtProc Token Exchange Service. It uses the EXTPROC_ env prefix and maps
// dotted YAML keys to underscored env vars (e.g. oauth2.token_endpoint →
// EXTPROC_OAUTH2_TOKEN_ENDPOINT). All string values support ${VAR} notation.
package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Load reads configuration from env vars (EXTPROC_ prefix) and an optional
// YAML config file. The config file path is read from the EXTPROC_CONFIG_PATH
// env var if set. Validation is applied after loading.
//
// Precedence (highest to lowest):
//  1. Environment variables (EXTPROC_*)
//  2. YAML config file (if provided)
//  3. Defaults
func Load() (*Config, error) {
	v := viper.New()
	return LoadFromViper(v)
}

// LoadWithCommand reads configuration with CLI flags applied at the highest
// precedence, above environment variables. This is the preferred entrypoint
// when the service is started via a Cobra command.
//
// Precedence (highest to lowest):
//  1. CLI flags (explicitly passed on the command line)
//  2. Environment variables (EXTPROC_*)
//  3. YAML config file (if provided)
//  4. Defaults
func LoadWithCommand(cmd *cobra.Command) (*Config, error) {
	v := viper.New()
	return loadFromViperWithCommand(v, cmd)
}

// LoadFromViper loads configuration from the provided Viper instance.
// Useful for testing with a pre-configured Viper.
func LoadFromViper(v *viper.Viper) (*Config, error) {
	return loadFromViperWithCommand(v, nil)
}

// RegisterFlags registers all ExtProc configuration flags on the given Cobra
// command. Call this from both the production cmd init() and test helpers so
// the flag set is always in sync with bindFlags.
func RegisterFlags(cmd *cobra.Command) {
	// gRPC server flags
	cmd.Flags().String("grpc.bind", "", "gRPC bind address (default: 0.0.0.0)")
	cmd.Flags().Int("grpc.port", 0, "gRPC listen port (default: 50051)")
	cmd.Flags().Int("grpc.max_concurrent_streams", 0, "maximum concurrent gRPC streams (default: 100)")
	// OAuth2 / token exchange flags
	cmd.Flags().String("oauth2.token_endpoint", "", "OAuth2 token endpoint URL (required)")
	cmd.Flags().String("oauth2.issuer", "", "OAuth2 authorization server issuer URL (required)")
	cmd.Flags().String("oauth2.client_id", "", "client ID for obtaining client assertion (required)")
	cmd.Flags().String("oauth2.client_secret", "", "client secret for obtaining client assertion (required)")
	cmd.Flags().String("oauth2.client_credentials_endpoint", "", "explicit client_credentials endpoint (optional)")
	cmd.Flags().StringSlice("oauth2.client_credentials_scopes", nil, "OAuth2 scopes for client_credentials grant (optional, defaults to [openid])")
	cmd.Flags().Duration("oauth2.exchange_timeout", 0, "timeout for token exchange HTTP calls (default: 5s)")
	// Cache flags
	cmd.Flags().Duration("cache.default_ttl", 0, "fallback TTL when token lacks expiry (default: 5m)")
	cmd.Flags().Duration("cache.max_ttl", 0, "maximum cache TTL cap (default: 1h)")
	// Logging flags
	cmd.Flags().String("log.level", "", "log level: debug, info, warn, error (default: info)")
	cmd.Flags().String("log.format", "", "log format: text, json (default: text)")
	// Circuit breaker flags
	cmd.Flags().Bool("circuit_breaker.enabled", true, "enable circuit breaker for token exchange calls (default: true)")
	cmd.Flags().Int("circuit_breaker.max_failures", 0, "consecutive failures before opening circuit (default: 5)")
	cmd.Flags().Duration("circuit_breaker.reset_timeout", 0, "duration in open state before probing recovery (default: 30s)")
	// Authorization flags
	cmd.Flags().Bool("authorization.enabled", false, "enable OPA-based request authorization (default: false)")
	cmd.Flags().String("authorization.policy.path", "", "filesystem path to local Rego policy file or directory")
	cmd.Flags().String("authorization.policy.config_file", "", "filesystem path to OPA configuration file (mutually exclusive with policy.path)")
	cmd.Flags().String("authorization.policy.package", "", "OPA package name (default: aib.extproc.authz)")
	cmd.Flags().String("authorization.policy.decision", "", "OPA decision document name (default: result)")
	cmd.Flags().String("authorization.default_decision", "", "decision when the policy result is undefined; must remain deny (default: deny)")
	cmd.Flags().Duration("authorization.evaluation_timeout", 0, "OPA evaluation timeout (default: 100ms)")
	cmd.Flags().Int("authorization.max_body_size", 0, "maximum request body size in bytes for OPA evaluation (default: 1048576)")
	// Approval gating flags
	cmd.Flags().Bool("tool_approvals.enabled", false, "enable broker approval gating (default: false)")
	cmd.Flags().String("tool_approvals.url", "", "identity broker approval API base URL")
	cmd.Flags().Int("tool_approvals.long_poll_timeout_seconds", 0, "approval long-poll timeout in seconds (default: 30)")
	cmd.Flags().Duration("tool_approvals.approval_cache_idle_ttl", 0, "approval cache idle TTL (default: 5m)")
	cmd.Flags().Duration("tool_approvals.request_timeout", 0, "approval request timeout (default: 5s)")
	cmd.Flags().String("sessions.extraction.http_header", "", "agent session HTTP header (default: Mcp-Session-Id)")
	// Telemetry flags
	cmd.Flags().Bool("telemetry.enabled", false, "enable OpenTelemetry (default: false)")
	cmd.Flags().String("telemetry.service_name", "", "service name in telemetry data (default: extproc-token-exchange)")
	cmd.Flags().String("telemetry.traces.sampling_rate", "", "trace sampling rate 0.0-1.0 (default: 1.0)")
	cmd.Flags().Bool("telemetry.exporter.insecure", false, "disable TLS for exporter (default: false, DEV ONLY)")
	cmd.Flags().String("telemetry.exporter.protocol", "", "exporter protocol: grpc, http, https (default: grpc)")
	cmd.Flags().String("telemetry.exporter.endpoint", "", "exporter endpoint (required when enabled)")
}

// loadFromViperWithCommand is the internal loading pipeline shared by
// Load, LoadWithCommand, and LoadFromViper. When cmd is non-nil its
// explicitly-changed flags are applied at the highest precedence (v.Set)
// before the config struct is unmarshalled.
func loadFromViperWithCommand(v *viper.Viper, cmd *cobra.Command) (*Config, error) {
	// Configure env var prefix and key replacer
	v.SetEnvPrefix("EXTPROC")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	// Apply defaults. Empty-string defaults for required fields are intentional:
	// they register the keys with Viper so that AutomaticEnv can supply values
	// from the environment during Unmarshal (keys unknown to Viper are skipped).
	applyDefaults(v)

	// Load YAML config file if path is provided
	configPath := v.GetString("config_path")
	if configPath == "" {
		configPath = os.Getenv("EXTPROC_CONFIG_PATH")
	}
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file %q: %w", configPath, err)
		}
	}

	// Apply CLI flag overrides at the highest precedence.
	// Only flags that were explicitly set on the command line are applied, so
	// defaults and unset flags do not shadow lower-precedence sources.
	if cmd != nil {
		bindFlags(v, cmd)
	}

	// Unmarshal into Config struct
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("failed to unmarshal configuration: %w", err)
	}

	// Expand ${VAR} notation in all string fields
	expandEnvVars(cfg)

	// Validate
	if err := Validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// bindFlags applies explicitly-set Cobra flag values to the Viper instance
// at the highest precedence (v.Set). Only flags that the user actually passed
// on the command line (cmd.Flags().Changed) are applied, so defaults and
// unset flags do not shadow lower-precedence sources.
//
// Flag getter errors (_, err := cmd.Flags().GetString(...)) are intentionally
// ignored. The getter only fails if the flag does not exist or has the wrong
// type; since we guard each call with Changed(), which itself requires the
// flag to be registered and set, neither failure mode can occur here.
func bindFlags(v *viper.Viper, cmd *cobra.Command) {
	type flagBinding struct {
		flagName string
		viperKey string
		get      func() interface{}
	}

	bindings := []flagBinding{
		{
			"grpc.bind", "grpc.bind",
			func() interface{} { s, _ := cmd.Flags().GetString("grpc.bind"); return s },
		},
		{
			"grpc.port", "grpc.port",
			func() interface{} { i, _ := cmd.Flags().GetInt("grpc.port"); return i },
		},
		{
			"grpc.max_concurrent_streams", "grpc.max_concurrent_streams",
			func() interface{} { i, _ := cmd.Flags().GetInt("grpc.max_concurrent_streams"); return i },
		},
		{
			"oauth2.token_endpoint", "oauth2.token_endpoint",
			func() interface{} { s, _ := cmd.Flags().GetString("oauth2.token_endpoint"); return s },
		},
		{
			"oauth2.issuer", "oauth2.issuer",
			func() interface{} { s, _ := cmd.Flags().GetString("oauth2.issuer"); return s },
		},
		{
			"oauth2.client_id", "oauth2.client_id",
			func() interface{} { s, _ := cmd.Flags().GetString("oauth2.client_id"); return s },
		},
		{
			"oauth2.client_secret", "oauth2.client_secret",
			func() interface{} { s, _ := cmd.Flags().GetString("oauth2.client_secret"); return s },
		},
		{
			"oauth2.client_credentials_endpoint", "oauth2.client_credentials_endpoint",
			func() interface{} { s, _ := cmd.Flags().GetString("oauth2.client_credentials_endpoint"); return s },
		},
		{
			"oauth2.client_credentials_scopes", "oauth2.client_credentials_scopes",
			func() interface{} { ss, _ := cmd.Flags().GetStringSlice("oauth2.client_credentials_scopes"); return ss },
		},
		{
			"oauth2.exchange_timeout", "oauth2.exchange_timeout",
			func() interface{} { d, _ := cmd.Flags().GetDuration("oauth2.exchange_timeout"); return d },
		},
		{
			"cache.default_ttl", "cache.default_ttl",
			func() interface{} { d, _ := cmd.Flags().GetDuration("cache.default_ttl"); return d },
		},
		{
			"cache.max_ttl", "cache.max_ttl",
			func() interface{} { d, _ := cmd.Flags().GetDuration("cache.max_ttl"); return d },
		},
		{
			"log.level", "log.level",
			func() interface{} { s, _ := cmd.Flags().GetString("log.level"); return s },
		},
		{
			"log.format", "log.format",
			func() interface{} { s, _ := cmd.Flags().GetString("log.format"); return s },
		},
		{
			"circuit_breaker.enabled", "circuit_breaker.enabled",
			func() interface{} { b, _ := cmd.Flags().GetBool("circuit_breaker.enabled"); return b },
		},
		{
			"circuit_breaker.max_failures", "circuit_breaker.max_failures",
			func() interface{} { i, _ := cmd.Flags().GetInt("circuit_breaker.max_failures"); return i },
		},
		{
			"circuit_breaker.reset_timeout", "circuit_breaker.reset_timeout",
			func() interface{} { d, _ := cmd.Flags().GetDuration("circuit_breaker.reset_timeout"); return d },
		},
		{
			"authorization.enabled", "authorization.enabled",
			func() interface{} { b, _ := cmd.Flags().GetBool("authorization.enabled"); return b },
		},
		{
			"authorization.policy.path", "authorization.policy.path",
			func() interface{} { s, _ := cmd.Flags().GetString("authorization.policy.path"); return s },
		},
		{
			"authorization.policy.config_file", "authorization.policy.config_file",
			func() interface{} { s, _ := cmd.Flags().GetString("authorization.policy.config_file"); return s },
		},
		{
			"authorization.policy.package", "authorization.policy.package",
			func() interface{} { s, _ := cmd.Flags().GetString("authorization.policy.package"); return s },
		},
		{
			"authorization.policy.decision", "authorization.policy.decision",
			func() interface{} { s, _ := cmd.Flags().GetString("authorization.policy.decision"); return s },
		},
		{
			"authorization.default_decision", "authorization.default_decision",
			func() interface{} { s, _ := cmd.Flags().GetString("authorization.default_decision"); return s },
		},
		{
			"authorization.evaluation_timeout", "authorization.evaluation_timeout",
			func() interface{} { d, _ := cmd.Flags().GetDuration("authorization.evaluation_timeout"); return d },
		},
		{
			"authorization.max_body_size", "authorization.max_body_size",
			func() interface{} { i, _ := cmd.Flags().GetInt("authorization.max_body_size"); return i },
		},
		{
			"tool_approvals.enabled", "tool_approvals.enabled",
			func() interface{} { b, _ := cmd.Flags().GetBool("tool_approvals.enabled"); return b },
		},
		{
			"tool_approvals.url", "tool_approvals.url",
			func() interface{} { s, _ := cmd.Flags().GetString("tool_approvals.url"); return s },
		},
		{
			"tool_approvals.long_poll_timeout_seconds", "tool_approvals.long_poll_timeout_seconds",
			func() interface{} { i, _ := cmd.Flags().GetInt("tool_approvals.long_poll_timeout_seconds"); return i },
		},
		{
			"tool_approvals.approval_cache_idle_ttl", "tool_approvals.approval_cache_idle_ttl",
			func() interface{} {
				d, _ := cmd.Flags().GetDuration("tool_approvals.approval_cache_idle_ttl")
				return d
			},
		},
		{
			"tool_approvals.request_timeout", "tool_approvals.request_timeout",
			func() interface{} { d, _ := cmd.Flags().GetDuration("tool_approvals.request_timeout"); return d },
		},
		{
			"sessions.extraction.http_header", "sessions.extraction.http_header",
			func() interface{} { s, _ := cmd.Flags().GetString("sessions.extraction.http_header"); return s },
		},
		{
			"telemetry.enabled", "telemetry.enabled",
			func() interface{} { b, _ := cmd.Flags().GetBool("telemetry.enabled"); return b },
		},
		{
			"telemetry.service_name", "telemetry.service_name",
			func() interface{} { s, _ := cmd.Flags().GetString("telemetry.service_name"); return s },
		},
		{
			"telemetry.traces.sampling_rate", "telemetry.traces.sampling_rate",
			func() interface{} { s, _ := cmd.Flags().GetString("telemetry.traces.sampling_rate"); return s },
		},
		{
			"telemetry.exporter.insecure", "telemetry.exporter.insecure",
			func() interface{} { b, _ := cmd.Flags().GetBool("telemetry.exporter.insecure"); return b },
		},
		{
			"telemetry.exporter.protocol", "telemetry.exporter.protocol",
			func() interface{} { s, _ := cmd.Flags().GetString("telemetry.exporter.protocol"); return s },
		},
		{
			"telemetry.exporter.endpoint", "telemetry.exporter.endpoint",
			func() interface{} { s, _ := cmd.Flags().GetString("telemetry.exporter.endpoint"); return s },
		},
	}

	for _, b := range bindings {
		if cmd.Flags().Changed(b.flagName) {
			v.Set(b.viperKey, b.get())
		}
	}
}

// applyDefaults sets configuration defaults.
// Empty-string defaults for required fields are intentional: they register the
// key with Viper so that AutomaticEnv (EXTPROC_* env vars) can supply values
// during Unmarshal. Keys unknown to Viper are not traversed on Unmarshal.
func applyDefaults(v *viper.Viper) {
	v.SetDefault("grpc.bind", "0.0.0.0")
	v.SetDefault("grpc.port", 50051)
	v.SetDefault("grpc.max_concurrent_streams", 100)
	// Required fields: empty defaults register the keys so env vars are read.
	v.SetDefault("oauth2.token_endpoint", "")
	v.SetDefault("oauth2.issuer", "")
	v.SetDefault("oauth2.client_id", "")
	v.SetDefault("oauth2.client_secret", "")
	v.SetDefault("oauth2.client_credentials_endpoint", "")
	v.SetDefault("oauth2.client_credentials_scopes", []string{})
	// Optional fields with non-empty defaults.
	v.SetDefault("oauth2.exchange_timeout", "5s")
	v.SetDefault("oauth2.client_assertion_type", "id_token")
	v.SetDefault("oauth2.tls.insecure_skip_verify", false)
	v.SetDefault("oauth2.tls.ca_bundle_path", "")
	v.SetDefault("oauth2.tls.allow_http", false)
	v.SetDefault("cache.default_ttl", "5m")
	v.SetDefault("cache.max_ttl", "1h")
	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "text")
	v.SetDefault("circuit_breaker.enabled", true)
	v.SetDefault("circuit_breaker.max_failures", 5)
	v.SetDefault("circuit_breaker.reset_timeout", "30s")
	// Authorization defaults — disabled by default (fail-closed).
	v.SetDefault("authorization.enabled", false)
	// Empty defaults for policy source fields register the Viper keys so that
	// AutomaticEnv (e.g. EXTPROC_AUTHORIZATION_POLICY_PATH) can supply values.
	v.SetDefault("authorization.policy.path", "")
	v.SetDefault("authorization.policy.config_file", "")
	v.SetDefault("authorization.policy.package", "aib.extproc.authz")
	v.SetDefault("authorization.policy.decision", "result")
	v.SetDefault("authorization.default_decision", "deny")
	v.SetDefault("authorization.evaluation_timeout", "100ms")
	v.SetDefault("authorization.max_body_size", 1048576)
	v.SetDefault("tool_approvals.enabled", false)
	v.SetDefault("tool_approvals.url", "")
	v.SetDefault("tool_approvals.long_poll_timeout_seconds", 30)
	v.SetDefault("tool_approvals.approval_cache_idle_ttl", "5m")
	v.SetDefault("tool_approvals.request_timeout", "5s")
	v.SetDefault("sessions.extraction.http_header", "Mcp-Session-Id")
	applyTelemetryDefaults(v)
}

// applyTelemetryDefaults sets OpenTelemetry configuration defaults.
// Telemetry is disabled by default; other values are production-safe defaults.
func applyTelemetryDefaults(v *viper.Viper) {
	v.SetDefault("telemetry.enabled", false)
	v.SetDefault("telemetry.service_name", "extproc-token-exchange")
	v.SetDefault("telemetry.resource_attributes", map[string]string{})
	v.SetDefault("telemetry.traces.enabled", true)
	v.SetDefault("telemetry.traces.sampling_rate", 1.0)
	v.SetDefault("telemetry.traces.propagators", []string{"tracecontext", "ottrace", "b3multi", "baggage"})
	v.SetDefault("telemetry.metrics.enabled", true)
	v.SetDefault("telemetry.metrics.export_interval", "30s")
	v.SetDefault("telemetry.logs.enabled", true)
	v.SetDefault("telemetry.exporter.protocol", "grpc")
	v.SetDefault("telemetry.exporter.endpoint", "")
	v.SetDefault("telemetry.exporter.headers", map[string]string{})
	v.SetDefault("telemetry.exporter.timeout", "10s")
	v.SetDefault("telemetry.exporter.insecure", false)
	v.SetDefault("telemetry.exporter.compression", "none")
}

// expandEnvVars processes ${VAR} notation in string config fields.
// This allows secrets to be injected via env vars in YAML files.
func expandEnvVars(cfg *Config) {
	cfg.OAuth2.TokenEndpoint = os.ExpandEnv(cfg.OAuth2.TokenEndpoint)
	cfg.OAuth2.Issuer = os.ExpandEnv(cfg.OAuth2.Issuer)
	cfg.OAuth2.ClientID = os.ExpandEnv(cfg.OAuth2.ClientID)
	cfg.OAuth2.ClientSecret = os.ExpandEnv(cfg.OAuth2.ClientSecret)
	cfg.OAuth2.ClientCredentialsEndpoint = os.ExpandEnv(cfg.OAuth2.ClientCredentialsEndpoint)
	cfg.OAuth2.TLS.CaBundlePath = os.ExpandEnv(cfg.OAuth2.TLS.CaBundlePath)
	cfg.GRPC.Bind = os.ExpandEnv(cfg.GRPC.Bind)
	cfg.Log.Level = os.ExpandEnv(cfg.Log.Level)
	cfg.Log.Format = os.ExpandEnv(cfg.Log.Format)
	cfg.Authorization.Policy.Path = os.ExpandEnv(cfg.Authorization.Policy.Path)
	cfg.Authorization.Policy.ConfigFile = os.ExpandEnv(cfg.Authorization.Policy.ConfigFile)
	cfg.Authorization.Policy.Package = os.ExpandEnv(cfg.Authorization.Policy.Package)
	cfg.Authorization.Policy.Decision = os.ExpandEnv(cfg.Authorization.Policy.Decision)
	cfg.Authorization.DefaultDecision = os.ExpandEnv(cfg.Authorization.DefaultDecision)
	cfg.ToolApprovals.URL = os.ExpandEnv(cfg.ToolApprovals.URL)
	cfg.Sessions.Extraction.HTTPHeader = os.ExpandEnv(cfg.Sessions.Extraction.HTTPHeader)
	// Expand telemetry fields
	cfg.Telemetry.ServiceName = os.ExpandEnv(cfg.Telemetry.ServiceName)
	cfg.Telemetry.Exporter.Endpoint = os.ExpandEnv(cfg.Telemetry.Exporter.Endpoint)
	cfg.Telemetry.Exporter.Protocol = os.ExpandEnv(cfg.Telemetry.Exporter.Protocol)
	cfg.Telemetry.Exporter.Compression = os.ExpandEnv(cfg.Telemetry.Exporter.Compression)
	// Expand header values in exporter headers map (SR-002: secret injection via ${VAR})
	for k, v := range cfg.Telemetry.Exporter.Headers {
		cfg.Telemetry.Exporter.Headers[k] = os.ExpandEnv(v)
	}
}
