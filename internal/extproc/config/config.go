// Package config defines the configuration schema for the ExtProc Token Exchange Service.
// It uses its own schema entirely separate from the identity broker configuration.
// All configuration is loaded via Viper with the EXTPROC_ env var prefix.
package config

import "time"

// Config is the root configuration for the ExtProc Token Exchange Service.
type Config struct {
	GRPC           GRPCConfig           `mapstructure:"grpc"`
	OAuth2         OAuth2Config         `mapstructure:"oauth2"`
	Cache          CacheConfig          `mapstructure:"cache"`
	Log            LogConfig            `mapstructure:"log"`
	CircuitBreaker CircuitBreakerConfig `mapstructure:"circuit_breaker"`
	Authorization  AuthorizationConfig  `mapstructure:"authorization"`
	ToolApprovals  ToolApprovalsConfig  `mapstructure:"tool_approvals"`
	Sessions       SessionsConfig       `mapstructure:"sessions"`
	Telemetry      TelemetryConfig      `mapstructure:"telemetry"`
}

// GRPCConfig holds gRPC server settings.
type GRPCConfig struct {
	Bind                 string `mapstructure:"bind"`
	Port                 int    `mapstructure:"port"`
	MaxConcurrentStreams int    `mapstructure:"max_concurrent_streams"`
}

// OAuth2Config holds OAuth2 and token exchange settings.
type OAuth2Config struct {
	TokenEndpoint             string        `mapstructure:"token_endpoint"`
	Issuer                    string        `mapstructure:"issuer"`
	ClientID                  string        `mapstructure:"client_id"`
	ClientSecret              string        `mapstructure:"client_secret"`
	ClientCredentialsEndpoint string        `mapstructure:"client_credentials_endpoint"`
	ClientCredentialsScopes   []string      `mapstructure:"client_credentials_scopes"`
	ClientAssertionType       string        `mapstructure:"client_assertion_type"`
	ExchangeTimeout           time.Duration `mapstructure:"exchange_timeout"`
	TLS                       TLSConfig     `mapstructure:"tls"`
}

// TLSConfig holds TLS settings for outbound HTTP connections.
type TLSConfig struct {
	InsecureSkipVerify bool   `mapstructure:"insecure_skip_verify"`
	CaBundlePath       string `mapstructure:"ca_bundle_path"`
	AllowHTTP          bool   `mapstructure:"allow_http"`
}

// CacheConfig holds token cache settings.
type CacheConfig struct {
	DefaultTTL time.Duration `mapstructure:"default_ttl"`
	MaxTTL     time.Duration `mapstructure:"max_ttl"`
}

// LogConfig holds logging settings.
type LogConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// CircuitBreakerConfig holds circuit breaker settings for the token exchange
// HTTP calls to the identity broker. The circuit breaker prevents a thundering
// herd when the identity broker recovers after an outage.
type CircuitBreakerConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	MaxFailures  int           `mapstructure:"max_failures"`
	ResetTimeout time.Duration `mapstructure:"reset_timeout"`
}

// AuthorizationConfig holds ExtProc-side OPA request authorization settings.
// This is distinct from the broker token-exchange authorization config in internal/ports/config.go.
type AuthorizationConfig struct {
	Enabled           bool          `mapstructure:"enabled"`
	Policy            PolicyConfig  `mapstructure:"policy"`
	DefaultDecision   string        `mapstructure:"default_decision"`
	EvaluationTimeout time.Duration `mapstructure:"evaluation_timeout"`
	MaxBodySize       int           `mapstructure:"max_body_size"`
}

// PolicyConfig holds OPA policy source settings.
// Exactly one of Path or ConfigFile must be set when authorization is enabled.
type PolicyConfig struct {
	Path       string `mapstructure:"path"`        // Filesystem path to local Rego file or directory
	ConfigFile string `mapstructure:"config_file"` // Filesystem path to OPA configuration file
	Package    string `mapstructure:"package"`     // OPA package name (default: aib.extproc.authz)
	Decision   string `mapstructure:"decision"`    // Decision document name (default: result)
}

type ToolApprovalsConfig struct {
	Enabled                bool          `mapstructure:"enabled"`
	URL                    string        `mapstructure:"url"`
	LongPollTimeoutSeconds int           `mapstructure:"long_poll_timeout_seconds"`
	ApprovalCacheIdleTTL   time.Duration `mapstructure:"approval_cache_idle_ttl"`
	RequestTimeout         time.Duration `mapstructure:"request_timeout"`
}

type SessionsConfig struct {
	Extraction SessionExtractionConfig `mapstructure:"extraction"`
}

type SessionExtractionConfig struct {
	HTTPHeader string `mapstructure:"http_header"`
}

// TelemetryConfig contains OpenTelemetry observability configuration.
// Mirror of ports.TelemetryConfig — structurally identical, defined locally
// to avoid importing internal/ports in the ExtProc service.
type TelemetryConfig struct {
	Enabled            bool               `mapstructure:"enabled"`
	ServiceName        string             `mapstructure:"service_name"`
	ResourceAttributes map[string]string  `mapstructure:"resource_attributes"`
	Traces             TracesConfig       `mapstructure:"traces"`
	Metrics            MetricsConfig      `mapstructure:"metrics"`
	Logs               LogsConfig         `mapstructure:"logs"`
	Exporter           OTLPExporterConfig `mapstructure:"exporter"`
}

// TracesConfig contains distributed tracing configuration.
type TracesConfig struct {
	Enabled      bool     `mapstructure:"enabled"`
	SamplingRate float64  `mapstructure:"sampling_rate"`
	Propagators  []string `mapstructure:"propagators"`
}

// MetricsConfig contains metrics collection and export configuration.
type MetricsConfig struct {
	Enabled        bool          `mapstructure:"enabled"`
	ExportInterval time.Duration `mapstructure:"export_interval"`
}

// LogsConfig contains OTLP log export configuration.
type LogsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// OTLPExporterConfig contains OTLP exporter connection parameters.
type OTLPExporterConfig struct {
	Protocol    string            `mapstructure:"protocol"`
	Endpoint    string            `mapstructure:"endpoint"`
	Headers     map[string]string `mapstructure:"headers"`
	Timeout     time.Duration     `mapstructure:"timeout"`
	Insecure    bool              `mapstructure:"insecure"`
	Compression string            `mapstructure:"compression"`
}
