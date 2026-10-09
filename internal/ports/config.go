// Package ports defines interfaces for hexagonal architecture boundaries.
// Configuration loading is a driven adapter; domain logic depends on this interface.
package ports

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
)

// ConfigPort defines the interface for accessing application configuration.
// This is a hexagonal architecture port - domain logic depends on this interface,
// not concrete implementations.
//
// Implementation: internal/config/loader.go (adapter using Viper/Cobra/godotenv)
type ConfigPort interface {
	// GetConfig returns the fully loaded and validated configuration.
	// ctx allows timeout and cancellation control during loading.
	// Returns error if configuration is invalid or cannot be loaded.
	// Called once at application startup (FR-007).
	GetConfig(ctx context.Context) (*Config, error)

	// GetSources returns metadata about all configuration sources used.
	// Used for audit logging (SR-004) and startup summary (FR-010).
	// This is a pure getter, no I/O, so context not needed.
	GetSources() []ConfigSource

	// Reload reloads configuration from all sources.
	// ctx allows timeout and cancellation control during reload.
	// NOT IMPLEMENTED in initial scope (no hot-reloading).
	// Included for future extensibility.
	Reload(ctx context.Context) error
}

// Config represents the complete application configuration schema.
type Config struct {
	Log              LogConfig              `mapstructure:"log" validate:"required"`
	Server           ServerConfig           `mapstructure:"server" validate:"required"`
	Storage          StorageConfig          `mapstructure:"storage" validate:"required"`
	ThirdPartyOAuth2 ThirdPartyOAuth2Config `mapstructure:"third_party_oauth2"`
	OAuth2AuthServer OAuth2AuthServerConfig `mapstructure:"oauth2_authorization_server"`
	TokenExchange    TokenExchangeConfig    `mapstructure:"token_exchange"`
	Security         SecurityConfig         `mapstructure:"security"`
	Encryption       EncryptionConfig       `mapstructure:"encryption"`
	Telemetry        TelemetryConfig        `mapstructure:"telemetry"`
	RequestContext   RequestContextConfig   `mapstructure:"request_context"`
	Approvals        ApprovalsConfig        `mapstructure:"approvals"`
	TokenRefresh     TokenRefreshConfig     `mapstructure:"token_refresh"`
}

// TokenRefreshConfig declares the proactive refresh settings for production configuration.
type TokenRefreshConfig struct {
	LookaheadDuration time.Duration           `mapstructure:"lookahead_duration"`
	BackgroundWorkers int                     `mapstructure:"background_workers"`
	Sweep             TokenRefreshSweepConfig `mapstructure:"sweep"`
}

type TokenRefreshSweepConfig struct {
	DefaultPageSize int `mapstructure:"default_page_size"`
}

// ServerConfig contains configuration for both HTTP servers.
type ServerConfig struct {
	EndUser  ServerInstanceConfig `mapstructure:"enduser" validate:"required"`
	Admin    ServerInstanceConfig `mapstructure:"admin" validate:"required"`
	Shutdown ShutdownConfig       `mapstructure:"shutdown" validate:"required"`
}

// ServerInstanceConfig contains configuration for a single HTTP server instance.
type ServerInstanceConfig struct {
	Port           int                  `mapstructure:"port" validate:"required,min=1,max=65535"`
	Bind           string               `mapstructure:"bind" validate:"required"`
	PublicURL      string               `mapstructure:"public_url" validate:"required,http_url"`
	Authentication AuthenticationConfig `mapstructure:"authentication"`
	CORS           CORSConfig           `mapstructure:"cors"`
}

// CORSConfig contains CORS (Cross-Origin Resource Sharing) configuration.
// When AllowedOrigins is empty, CORS headers are not added (production default — secure by default).
// Use explicit localhost origins during development, for example ["http://localhost:5173"], instead of wildcard access.
type CORSConfig struct {
	// AllowedOrigins lists origins allowed for cross-origin requests.
	// Use explicit localhost origins during development instead of wildcard access.
	// Leave empty to disable CORS headers (production default).
	AllowedOrigins []string `mapstructure:"allowed_origins"`

	// AllowedMethods lists HTTP methods allowed for cross-origin requests.
	// Defaults to ["GET", "POST", "PUT", "DELETE", "OPTIONS"] when CORS is enabled.
	AllowedMethods []string `mapstructure:"allowed_methods"`

	// AllowedHeaders lists request headers allowed for cross-origin requests.
	// Defaults to ["Content-Type", "Authorization", "X-Custom-Principal"] when CORS is enabled.
	AllowedHeaders []string `mapstructure:"allowed_headers"`

	// MaxAge sets the cache duration for preflight responses in seconds.
	// Defaults to 86400 (24 hours) when CORS is enabled.
	MaxAge int `mapstructure:"max_age"`
}

type RequestContextConfig struct {
	TrustedProxy RequestContextTrustedProxyConfig `mapstructure:"trusted_proxy"`
	Trace        RequestContextTraceConfig        `mapstructure:"trace"`
}

type RequestContextTrustedProxyConfig struct {
	Enabled         bool   `mapstructure:"enabled"`
	ForwardedHeader string `mapstructure:"forwarded_header"`
}

type RequestContextTraceConfig struct {
	ResponseEnabled bool `mapstructure:"response_enabled"`
}

// AuthenticationConfig holds authentication configuration for a server.
type AuthenticationConfig struct {
	// Preauth holds configuration for pre-authentication (reverse proxy) mode
	Preauth PreauthConfig `mapstructure:"preauth"`

	// JWT holds optional signed JWT pre-authentication settings. The JWT signature
	// is verified against JWKS before claims can supply a principal or profile.
	// Nil leaves plain-header pre-authentication in place.
	JWT *JWTConfig `mapstructure:"jwt"`
}

// JWTConfig holds configuration for JWT-based pre-authentication.
// This is an additive, opt-in capability alongside the existing plain-header pre-auth.
// When present, JWTs from a configured HTTP header are parsed, validated, and claims
// extracted via CEL expressions to derive principal and optional profile attributes.
type JWTConfig struct {
	// HeaderName is the HTTP header containing the JWT.
	// When set to "Authorization" (default), the "Bearer " prefix is automatically stripped.
	// For any other header name, the raw header value is used as the JWT directly.
	// Default: "Authorization"
	HeaderName string `mapstructure:"header_name"`

	// Verification permits "jwks" only (also the default when omitted).
	// Every JWT signature is verified against the configured JWKS endpoint.
	Verification string `mapstructure:"verification"`

	// JWKSURI is required for JWT pre-authentication. It must use HTTPS unless
	// Security.SkipThirdpartyHTTPSValidation is true.
	JWKSURI string `mapstructure:"jwks_uri"`

	// ExpectedAudience is the expected value in the JWT aud claim.
	// If set, JWTs without this audience are rejected with 401.
	// Optional.
	ExpectedAudience string `mapstructure:"expected_audience"`

	// ExpectedIssuer is the expected value in the JWT iss claim.
	// If set, JWTs with a non-matching issuer are rejected with 401.
	// Optional.
	ExpectedIssuer string `mapstructure:"expected_issuer"`

	// ClaimExtraction holds CEL expressions for extracting principal and
	// optional profile attributes from JWT claims.
	ClaimExtraction JWTClaimExtractionConfig `mapstructure:"claim_extraction"`
}

// JWTClaimExtractionConfig holds CEL expressions for extracting principal and
// optional profile attributes from JWT claims. All expressions receive a single
// variable "claims" of type map(string, any) — the JWT claims map.
// This follows the pattern from ADR 009 (CEL for Authorization Policies).
type JWTClaimExtractionConfig struct {
	// PrincipalExpression is a CEL expression to extract the principal from JWT claims.
	// Must evaluate to a non-empty string. Authentication fails if extraction fails.
	// Default: "claims.sub"
	PrincipalExpression string `mapstructure:"principal_expression"`

	// DisplayNameExpression is an optional CEL expression to extract the user's display name.
	// If not configured or evaluates to non-string, display name falls back to principal.
	// Example: "claims.name"
	DisplayNameExpression string `mapstructure:"display_name_expression"`

	// EmailExpression is an optional CEL expression to extract the user's email address.
	// If not configured or evaluates to non-string, email is nil in the profile.
	// Example: "claims.email"
	EmailExpression string `mapstructure:"email_expression"`

	// PictureURLExpression is an optional CEL expression to extract the user's profile picture URL.
	// If not configured or evaluates to non-string, picture URL is nil in the profile.
	// Example: "claims.picture"
	PictureURLExpression string `mapstructure:"picture_url_expression"`
}

// PreauthConfig holds configuration for reverse proxy pre-authentication.
type PreauthConfig struct {
	// PrincipalHeaderName is the HTTP header from which principals are extracted.
	// This header is set by a trusted reverse proxy after authentication.
	// Example values: "X-Remote-User", "X-Authenticated-User", "Remote-User"
	// REQUIRED: Must be explicitly configured. No default is provided to prevent
	// accidental exposure of a trust boundary.
	PrincipalHeaderName string `mapstructure:"principal_header_name" validate:"required,min=1"`
}

// ShutdownConfig contains graceful shutdown settings.
type ShutdownConfig struct {
	Timeout time.Duration `mapstructure:"timeout" validate:"required"`
}

// DefaultServerConfig returns default server configuration values.
func DefaultServerConfig() ServerConfig {
	return ServerConfig{
		EndUser: ServerInstanceConfig{
			Port:      8000,
			Bind:      "::",                    // Dual-stack (IPv6 with IPv4 fallback)
			PublicURL: "http://localhost:8000", // Default for local development
		},
		Admin: ServerInstanceConfig{
			Port:      14000,
			Bind:      "::",
			PublicURL: "http://localhost:14000", // Default for local development
		},
		Shutdown: ShutdownConfig{
			Timeout: 30 * time.Second,
		},
	}
}

func DefaultRequestContextConfig() RequestContextConfig {
	return RequestContextConfig{
		TrustedProxy: RequestContextTrustedProxyConfig{
			Enabled:         false,
			ForwardedHeader: "X-Forwarded-For",
		},
		Trace: RequestContextTraceConfig{
			ResponseEnabled: true,
		},
	}
}

// LogConfig contains logging-related configuration.
type LogConfig struct {
	Level  LogLevel  `mapstructure:"level" validate:"required"`
	Format LogFormat `mapstructure:"format" validate:"required"`
}

// LogLevel is an enumeration of valid log levels.
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat is an enumeration of valid log output formats.
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// ConfigSource represents metadata about a configuration source.
type ConfigSource struct {
	Type       SourceType // Type of configuration source
	Path       string     // File path (for file-based sources) or "env" / "cli"
	Precedence int        // Precedence level (higher = higher priority)
	LoadedAt   time.Time  // Timestamp when source was loaded
	Keys       []string   // Configuration keys provided by this source
}

// SourceType enumerates the types of configuration sources.
type SourceType string

const (
	SourceTypeDefault SourceType = "default"  // Built-in defaults (Precedence: 0)
	SourceTypeEnvFile SourceType = "env_file" // .env files (Precedence: 1)
	SourceTypeYAML    SourceType = "yaml"     // YAML file (Precedence: 2)
	SourceTypeCLI     SourceType = "cli"      // Command-line flags (Precedence: 3)
)

// StorageConfig contains configuration for the storage layer.
// Specifies which backend (memory or postgres) to use and its parameters.
type StorageConfig struct {
	Backend  string          `mapstructure:"backend" validate:"required,oneof=memory postgres"`
	Postgres PostgresConfig  `mapstructure:"postgres"`
	Timeouts StorageTimeouts `mapstructure:"timeouts" validate:"required"`
}

// PostgresConfig contains PostgreSQL-specific connection parameters.
// Only used when StorageConfig.Backend is "postgres".
type PostgresConfig struct {
	ConnectionURL string `mapstructure:"connection_url" validate:"required_if=Backend postgres"`
}

// StorageTimeouts defines timeout durations for steady-state storage operations.
// Applied to all backends to prevent indefinite hangs.
type StorageTimeouts struct {
	Read  time.Duration `mapstructure:"read" validate:"required"`
	Write time.Duration `mapstructure:"write" validate:"required"`
}

// ThirdPartyOAuth2Config contains configuration for OAuth2 session management with third-party services.
// This configuration is optional - if not provided, OAuth2 sessions routes will not be registered.
// When provided, enables users to authenticate with external OAuth2 providers
// (GitHub, Google, Microsoft, etc.) through the identity broker.
type ThirdPartyOAuth2Config struct {
	// JWESigningKey is the base64-encoded key for signing JWE state tokens (REQUIRED, 32 bytes).
	// This key MUST be kept secret. It protects OAuth2 state tokens during authorization flows.
	// REQUIRED - application will fail to start if not provided.
	// Generate with: openssl rand -base64 32
	// Store in environment variable: IDENTITY_BROKER_JWE_SIGNING_KEY
	JWESigningKey string `mapstructure:"jwe_signing_key"`

	// StateTokenTTL is the time-to-live for OAuth2 state tokens.
	// Maximum allowed: 15 minutes per security requirements (SR-008).
	// Shorter TTL reduces exposure window for state token leakage.
	// Default: 10 minutes
	StateTokenTTL time.Duration `mapstructure:"state_token_ttl"`

	// PKCEVerifierLength is the length of PKCE code verifier in bytes.
	// Must be 32-128 bytes per RFC 7636.
	// Default: 32 bytes (256 bits of entropy)
	PKCEVerifierLength int `mapstructure:"pkce_verifier_length"`
}

// MultiAgentClientConfig holds configuration for multi-agent OAuth2 client sharing.
// When Enabled is true, multiple agents may share the same upstream OAuth2 client ID.
// All-or-nothing: applies to all agents when enabled.
type MultiAgentClientConfig struct {
	// Enabled enables multi-agent client sharing. Default: false.
	// When false, uniqueness of agent.ClientID is enforced at the application layer.
	Enabled bool `mapstructure:"enabled"`

	// AgentIDParamName is the query parameter name appended to the upstream
	// authorization redirect URL carrying the agent's internal ID.
	// Required when Enabled is true. Example: "x_agent_id".
	AgentIDParamName string `mapstructure:"agent_id_param_name"`

	// AgentIDClaimName is the JWT claim name expected in tokens returned by the
	// upstream server that contains the agent's internal ID.
	// Required when Enabled is true. Example: "x_agent_id".
	AgentIDClaimName string `mapstructure:"agent_id_claim_name"`
}

// CIMDConfig holds Client ID Metadata Document fetch and validation settings.
// CIMD is disabled by default (Enabled: false); SSRF protection is always active.
type CIMDConfig struct {
	Enabled             bool            `mapstructure:"enabled"`
	FetchTimeout        time.Duration   `mapstructure:"fetch_timeout"`
	MaxResponseBytes    int             `mapstructure:"max_response_bytes"`
	Cache               CIMDCacheConfig `mapstructure:"cache"`
	SSRF                CIMDSSRFConfig  `mapstructure:"ssrf"`
	ClientNameBlocklist []string        `mapstructure:"client_name_blocklist"`
}

// CIMDCacheConfig holds operator TTL bounds and entry cap for CIMD response caching.
type CIMDCacheConfig struct {
	MaxTTL     time.Duration `mapstructure:"max_ttl"`
	MinTTL     time.Duration `mapstructure:"min_ttl"`
	MaxEntries int           `mapstructure:"max_entries"`
}

// CIMDSSRFConfig holds SSRF protection settings for CIMD fetches.
type CIMDSSRFConfig struct {
	ExtraBlockedCIDRs []string `mapstructure:"extra_blocked_cidrs"`
}

// DefaultCIMDConfig returns safe defaults for CIMD configuration.
func DefaultCIMDConfig() CIMDConfig {
	return CIMDConfig{
		Enabled:          false,
		FetchTimeout:     1 * time.Second,
		MaxResponseBytes: 5120,
		Cache: CIMDCacheConfig{
			MaxTTL:     1 * time.Hour,
			MinTTL:     60 * time.Second,
			MaxEntries: 1000,
		},
	}
}

// ProxyModeConfig holds upstream OAuth2 server configuration for proxy mode.
type ProxyModeConfig struct {
	UpstreamIssuerURI         string        `mapstructure:"upstream_issuer_uri"`
	UpstreamAuthorizeEndpoint string        `mapstructure:"upstream_authorize_endpoint"`
	UpstreamTokenEndpoint     string        `mapstructure:"upstream_token_endpoint"`
	UpstreamTimeout           time.Duration `mapstructure:"upstream_timeout"`
	UpstreamJWKSMinRefresh    time.Duration `mapstructure:"upstream_jwks_min_refresh"`
	UpstreamJWKSMaxRefresh    time.Duration `mapstructure:"upstream_jwks_max_refresh"`
}

const DefaultSigningKeyBootstrapTimeout = 90 * time.Second

// LocalModeConfig holds local token issuance configuration for local/hybrid mode.
type LocalModeConfig struct {
	// IssuerURI is the JWT iss claim for locally-minted tokens. Optional — defaults to
	// server.enduser.public_url when empty, allowing independent control behind CDNs or proxies.
	IssuerURI             string                 `mapstructure:"issuer_uri"`
	TokenTTL              time.Duration          `mapstructure:"token_ttl"`
	RefreshTokenTTL       time.Duration          `mapstructure:"refresh_token_ttl"`
	TokenClaimsExpression string                 `mapstructure:"token_claims_expression"`
	SigningKeys           LocalSigningKeysConfig `mapstructure:"signing_keys"`
}

// LocalSigningKeysConfig holds local signing-key startup settings.
type LocalSigningKeysConfig struct {
	BootstrapTimeout time.Duration `mapstructure:"bootstrap_timeout"`
}

// OAuth2AuthServerConfig represents configuration for OAuth2 authorization server functionality.
type OAuth2AuthServerConfig struct {
	Mode  servermode.Mode `mapstructure:"mode"`
	Proxy ProxyModeConfig `mapstructure:"proxy"`
	Local LocalModeConfig `mapstructure:"local"`

	// SupportedResponseTypes, SupportedGrantTypes, and SupportedScopes are shared; defaults differ by mode.
	SupportedResponseTypes []string `mapstructure:"supported_response_types"`
	SupportedGrantTypes    []string `mapstructure:"supported_grant_types"`
	SupportedScopes        []string `mapstructure:"supported_scopes"`

	// MultiAgentClient holds optional multi-agent client sharing configuration.
	MultiAgentClient MultiAgentClientConfig `mapstructure:"multi_agent_client"`

	// CIMD holds Client ID Metadata Document configuration.
	CIMD CIMDConfig `mapstructure:"cimd"`

	// Impersonation holds optional RFC 8693 user-impersonation configuration.
	// Present only in local mode (CR-006); nil preserves existing behavior.
	Impersonation *ImpersonationConfig `mapstructure:"impersonation"`
}

// Validate validates the OAuth2AuthServerConfig structure.
// Sets defaults for empty fields and returns an error for missing required fields.
// oauth2_authorization_server is mandatory — an absent or zero-value block fails validation.
func (c *OAuth2AuthServerConfig) Validate() error {
	if c.Mode == "issue_token" {
		return c.newValidationError("oauth2_authorization_server.mode 'issue_token' has been renamed to 'local' — please update your configuration")
	}

	if c.Mode == "" {
		return c.newValidationError("oauth2_authorization_server.mode is required (use 'proxy', 'local', or 'hybrid')")
	}

	switch c.Mode {
	case servermode.Local:
		return c.validateLocalMode()
	case servermode.Proxy:
		return c.validateProxyMode()
	case servermode.Hybrid:
		return c.validateHybridMode()
	default:
		return c.newValidationError("oauth2_authorization_server.mode must be 'proxy', 'local', or 'hybrid'")
	}
}

// validateProxyMode validates configuration for proxy mode (upstream OAuth2 server).
func (c *OAuth2AuthServerConfig) validateProxyMode() error {
	if c.CIMD.Enabled {
		return c.newValidationError("oauth2_authorization_server.cimd.enabled requires mode 'local' or 'hybrid'; CIMD is incompatible with proxy mode")
	}

	if c.Local.TokenTTL != 0 || c.Local.RefreshTokenTTL != 0 || c.Local.TokenClaimsExpression != "" || c.Local.IssuerURI != "" || c.Local.SigningKeys.BootstrapTimeout != 0 {
		return c.newValidationError("oauth2_authorization_server.local must be empty in proxy mode")
	}

	if err := c.validateProxyFields(""); err != nil {
		return err
	}
	c.applySharedDefaults([]string{"authorization_code"})
	return c.validateMultiAgentClient()
}

// validateMultiAgentClient validates multi-agent client config if enabled.
func (c *OAuth2AuthServerConfig) validateMultiAgentClient() error {
	if !c.MultiAgentClient.Enabled {
		return nil
	}
	if c.MultiAgentClient.AgentIDParamName == "" {
		return c.newValidationError("oauth2_authorization_server.multi_agent_client.agent_id_param_name is required")
	}
	if c.MultiAgentClient.AgentIDClaimName == "" {
		return c.newValidationError("oauth2_authorization_server.multi_agent_client.agent_id_claim_name is required")
	}
	return nil
}

// validateLocalMode validates configuration for local mode (local token minting).
func (c *OAuth2AuthServerConfig) validateLocalMode() error {
	if c.Proxy.UpstreamIssuerURI != "" || c.Proxy.UpstreamAuthorizeEndpoint != "" ||
		c.Proxy.UpstreamTokenEndpoint != "" || c.Proxy.UpstreamTimeout != 0 ||
		c.Proxy.UpstreamJWKSMinRefresh != 0 || c.Proxy.UpstreamJWKSMaxRefresh != 0 {
		return c.newValidationError("oauth2_authorization_server.proxy must be empty in local mode")
	}

	c.applyLocalDefaults()
	if err := c.validateRefreshTokenTTL(); err != nil {
		return err
	}
	c.applySharedDefaults([]string{"authorization_code", "client_credentials", "refresh_token"})
	if err := c.validateLocalSigningKeys(); err != nil {
		return err
	}

	if err := c.validateCIMDCache(); err != nil {
		return err
	}
	if len(c.SupportedScopes) == 0 {
		c.SupportedScopes = []string{"offline_access"}
	}

	if c.MultiAgentClient.Enabled {
		return c.newValidationError("oauth2_authorization_server.multi_agent_client is not supported in local mode")
	}

	return nil
}

// validateHybridMode validates configuration for hybrid mode (proxy + local token minting).
func (c *OAuth2AuthServerConfig) validateHybridMode() error {
	if err := c.validateProxyFields(" is required in hybrid mode"); err != nil {
		return err
	}
	c.applyLocalDefaults()
	if err := c.validateRefreshTokenTTL(); err != nil {
		return err
	}
	c.applySharedDefaults([]string{"authorization_code", "client_credentials", "refresh_token"})
	if err := c.validateLocalSigningKeys(); err != nil {
		return err
	}
	if err := c.validateCIMDCache(); err != nil {
		return err
	}
	if len(c.SupportedScopes) == 0 {
		c.SupportedScopes = []string{"offline_access"}
	}
	return c.validateMultiAgentClient()
}

// validateProxyFields checks that the proxy section has all required upstream endpoints.
func (c *OAuth2AuthServerConfig) validateProxyFields(suffix string) error {
	if c.Proxy.UpstreamIssuerURI == "" {
		return c.newValidationError("oauth2_authorization_server.proxy.upstream_issuer_uri" + suffix)
	}
	if c.Proxy.UpstreamAuthorizeEndpoint == "" {
		return c.newValidationError("oauth2_authorization_server.proxy.upstream_authorize_endpoint" + suffix)
	}
	if c.Proxy.UpstreamTokenEndpoint == "" {
		return c.newValidationError("oauth2_authorization_server.proxy.upstream_token_endpoint" + suffix)
	}
	if c.Proxy.UpstreamTimeout < 0 {
		return c.newValidationError("oauth2_authorization_server.proxy.upstream_timeout must be a positive duration (e.g. 30s, 500ms)")
	}
	if c.Proxy.UpstreamTimeout == 0 {
		c.Proxy.UpstreamTimeout = 30 * time.Second
	}
	return nil
}

// applyLocalDefaults sets local-mode defaults.
func (c *OAuth2AuthServerConfig) applyLocalDefaults() {
	if c.Local.TokenTTL == 0 {
		c.Local.TokenTTL = time.Hour
	}
	if c.Local.RefreshTokenTTL == 0 {
		c.Local.RefreshTokenTTL = 30 * 24 * time.Hour
	}
	if c.Local.SigningKeys.BootstrapTimeout == 0 {
		c.Local.SigningKeys.BootstrapTimeout = DefaultSigningKeyBootstrapTimeout
	}
}

func (c *OAuth2AuthServerConfig) validateRefreshTokenTTL() error {
	if c.Local.RefreshTokenTTL < 0 {
		return c.newValidationError("oauth2_authorization_server.local.refresh_token_ttl must be a positive duration")
	}
	return nil
}

func (c *OAuth2AuthServerConfig) validateLocalSigningKeys() error {
	if c.Local.SigningKeys.BootstrapTimeout <= 0 {
		return c.newValidationError("oauth2_authorization_server.local.signing_keys.bootstrap_timeout must be a positive duration")
	}
	return nil
}

// applySharedDefaults sets shared defaults for response types and grant types.
func (c *OAuth2AuthServerConfig) applySharedDefaults(defaultGrantTypes []string) {
	if len(c.SupportedResponseTypes) == 0 {
		c.SupportedResponseTypes = []string{"code"}
	}
	if len(c.SupportedGrantTypes) == 0 {
		c.SupportedGrantTypes = defaultGrantTypes
	}
}

// validateCIMDCache validates CIMD cache TTL invariants when CIMD is enabled.
func (c *OAuth2AuthServerConfig) validateCIMDCache() error {
	if c.CIMD.Enabled && c.CIMD.Cache.MinTTL > c.CIMD.Cache.MaxTTL {
		return c.newValidationError("oauth2_authorization_server.cimd.cache.min_ttl must not exceed max_ttl")
	}
	return nil
}

// Resolve validates the OAuth2AuthServerConfig and produces a concrete, mode-specific
// OAuth2ModeConfig. The returned type is one of *ProxyOAuth2Config, *LocalOAuth2Config,
// or *HybridOAuth2Config. Downstream code type-switches on the result — no scattered
// mode checks needed.
func (c *OAuth2AuthServerConfig) Resolve() (OAuth2ModeConfig, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}
	switch c.Mode {
	case servermode.Proxy:
		return &ProxyOAuth2Config{
			UpstreamIssuerURI:         c.Proxy.UpstreamIssuerURI,
			UpstreamAuthorizeEndpoint: c.Proxy.UpstreamAuthorizeEndpoint,
			UpstreamTokenEndpoint:     c.Proxy.UpstreamTokenEndpoint,
			UpstreamTimeout:           c.Proxy.UpstreamTimeout,
			UpstreamJWKSMinRefresh:    c.Proxy.UpstreamJWKSMinRefresh,
			UpstreamJWKSMaxRefresh:    c.Proxy.UpstreamJWKSMaxRefresh,
			SupportedResponseTypes:    c.SupportedResponseTypes,
			SupportedGrantTypes:       c.SupportedGrantTypes,
			SupportedScopes:           c.SupportedScopes,
			MultiAgentClient:          c.MultiAgentClient,
		}, nil
	case servermode.Local:
		return &LocalOAuth2Config{
			IssuerURI:              c.Local.IssuerURI,
			TokenTTL:               c.Local.TokenTTL,
			RefreshTokenTTL:        c.Local.RefreshTokenTTL,
			TokenClaimsExpression:  c.Local.TokenClaimsExpression,
			SigningKeys:            c.Local.SigningKeys,
			SupportedResponseTypes: c.SupportedResponseTypes,
			SupportedGrantTypes:    c.SupportedGrantTypes,
			SupportedScopes:        c.SupportedScopes,
			CIMD:                   c.CIMD,
		}, nil
	case servermode.Hybrid:
		return &HybridOAuth2Config{
			Proxy: ProxyOAuth2Config{
				UpstreamIssuerURI:         c.Proxy.UpstreamIssuerURI,
				UpstreamAuthorizeEndpoint: c.Proxy.UpstreamAuthorizeEndpoint,
				UpstreamTokenEndpoint:     c.Proxy.UpstreamTokenEndpoint,
				UpstreamTimeout:           c.Proxy.UpstreamTimeout,
				UpstreamJWKSMinRefresh:    c.Proxy.UpstreamJWKSMinRefresh,
				UpstreamJWKSMaxRefresh:    c.Proxy.UpstreamJWKSMaxRefresh,
				SupportedResponseTypes:    c.SupportedResponseTypes,
				SupportedGrantTypes:       c.SupportedGrantTypes,
				SupportedScopes:           c.SupportedScopes,
				MultiAgentClient:          c.MultiAgentClient,
			},
			Local: LocalOAuth2Config{
				IssuerURI:              c.Local.IssuerURI,
				TokenTTL:               c.Local.TokenTTL,
				RefreshTokenTTL:        c.Local.RefreshTokenTTL,
				TokenClaimsExpression:  c.Local.TokenClaimsExpression,
				SigningKeys:            c.Local.SigningKeys,
				SupportedResponseTypes: c.SupportedResponseTypes,
				SupportedGrantTypes:    c.SupportedGrantTypes,
				SupportedScopes:        c.SupportedScopes,
				CIMD:                   c.CIMD,
			},
		}, nil
	default:
		return nil, c.newValidationError("oauth2_authorization_server.mode must be 'proxy', 'local', or 'hybrid'")
	}
}

// newValidationError creates a validation error for the given field.
// This is a helper to create errors compatible with domain/config.ConfigError.
func (c *OAuth2AuthServerConfig) newValidationError(field string) error {
	// Return a generic error that has a Field() method for test compatibility
	// The actual ConfigError type is created in the config adapter layer
	return &oauth2ValidationError{field: field}
}

// oauth2ValidationError is a simple internal error type to avoid circular imports
type oauth2ValidationError struct {
	field string
}

// Error implements the error interface.
// Returns the full field path so callers using ContainSubstring on err.Error() can match field names.
func (e *oauth2ValidationError) Error() string {
	return e.field
}

// Field returns the error field for test compatibility
func (e *oauth2ValidationError) Field() string {
	return e.field
}

// CredentialRole identifies an impersonation credential role. It keys the rule's roles map,
// scopes a trusted issuer's signs_roles, and labels audit records (data-model §1).
type CredentialRole string

// Impersonation credential roles.
const (
	CredentialRoleClientAssertion CredentialRole = "client_assertion"
	CredentialRoleActor           CredentialRole = "actor"
	CredentialRoleSubject         CredentialRole = "subject"
)

// Subject verification modes (ImpersonationRoleConfig.Verification, subject role only).
const (
	// SubjectVerificationJWKS validates a signed subject against a trusted issuer (default).
	SubjectVerificationJWKS = "jwks"
	// SubjectVerificationNone accepts an unsigned unverified subject JWT (FR-003d, ADR 031).
	SubjectVerificationNone = "none"
)

// Impersonation audience-requirement policy values (ImpersonationRoleConfig.AudienceRequirement).
const (
	// ImpersonationAudienceRequirementAbsent accepts a signed credential only when it carries
	// no aud claim; any credential presenting an aud is rejected (CR-009).
	ImpersonationAudienceRequirementAbsent = "absent"
)

// ImpersonationConfig is the optional oauth2_authorization_server.impersonation subtree.
// It activates an RFC 8693 user-impersonation flow in local mode (data-model §1).
type ImpersonationConfig struct {
	// AudiencePrefix is a routing URI prefix, not an issued-token audience (CR-001).
	AudiencePrefix string `mapstructure:"audience_prefix"`
	// Rules is a non-empty, ordered list evaluated first-match (CR-002, FR-004a).
	Rules []ImpersonationRuleConfig `mapstructure:"rules"`
}

// ImpersonationRuleConfig is a self-contained, atomically reviewable impersonation rule (CR-002/CR-003).
type ImpersonationRuleConfig struct {
	// Name is a unique, operator-facing rule name; appears in audit (CR-003).
	Name string `mapstructure:"name"`
	// Roles declares per-role semantics keyed by role name (client_assertion|actor|subject) (CR-003).
	Roles map[string]ImpersonationRoleConfig `mapstructure:"roles"`
	// TrustedIssuers are the rule's trust anchors, each declaring which roles it may sign (CR-008).
	TrustedIssuers []TrustedTokenIssuerConfig `mapstructure:"trusted_issuers"`
	// Authorization is the rule's single CEL predicate, reusing the token-exchange schema (CR-005).
	Authorization AuthorizationConfig `mapstructure:"authorization"`
}

// ImpersonationRoleConfig declares role semantics once per role within a rule (data-model §1).
type ImpersonationRoleConfig struct {
	// Verification applies to the subject role only: "jwks" (default, signed) or "none" (unverified).
	Verification string `mapstructure:"verification"`
	// ExpectedAudience is the aud each signed credential must carry; required for signed roles,
	// forbidden when Verification is "none" (CR-003).
	ExpectedAudience string `mapstructure:"expected_audience"`
	// AudienceRequirement is an exceptional audience policy for a signed role. The only value is
	// "absent" (ImpersonationAudienceRequirementAbsent): the credential MUST carry no aud claim
	// and any credential presenting an aud is rejected. Mutually exclusive with ExpectedAudience;
	// forbidden on an unverified subject (verification: none) (CR-009).
	AudienceRequirement string `mapstructure:"audience_requirement"`
	// PrincipalExpression extracts the non-empty role identity from claims (required, FR-006).
	PrincipalExpression string `mapstructure:"principal_expression"`
	// EmailExpression optionally extracts a subject email; subject role only (FR-006a).
	EmailExpression string `mapstructure:"email_expression"`
}

// TrustedTokenIssuerConfig is an impersonation trust anchor scoped within a rule (data-model §1).
type TrustedTokenIssuerConfig struct {
	// IssuerURI is matched against a credential's iss claim (CR-008).
	IssuerURI string `mapstructure:"issuer_uri"`
	// JWKSURI is the explicit key-set location; discovered from issuer metadata when empty.
	JWKSURI string `mapstructure:"jwks_uri"`
	// JWKSMinRefresh bounds the minimum JWKS refresh cadence (default 15m).
	JWKSMinRefresh time.Duration `mapstructure:"jwks_min_refresh"`
	// JWKSMaxRefresh bounds the maximum JWKS refresh cadence (default max(min, 1h)).
	JWKSMaxRefresh time.Duration `mapstructure:"jwks_max_refresh"`
	// AllowedAlgorithms is a non-empty subset of the broker-approved asymmetric set (CR-007).
	AllowedAlgorithms []string `mapstructure:"allowed_algorithms"`
	// SignsRoles lists the signed roles this issuer may sign (CR-008).
	SignsRoles []string `mapstructure:"signs_roles"`
}

// ClientAssertionTrustConfig configures the trust anchor used to validate the privileged gateway's
// client_assertion JWT in RFC 8693 token exchange and machine-facing approval authentication.
// It is independent of whether the broker mints tokens locally or proxies them.
// When IssuerURI is empty, it defaults to oauth2_authorization_server.proxy.upstream_issuer_uri
// in proxy and hybrid modes. In local mode it must be set explicitly and must identify an external
// identity provider; broker-minted tokens must never be accepted as client assertions.
type ClientAssertionTrustConfig struct {
	IssuerURI      string        `mapstructure:"issuer_uri"`
	JWKSURI        string        `mapstructure:"jwks_uri"`
	JWKSMinRefresh time.Duration `mapstructure:"jwks_min_refresh"`
	JWKSMaxRefresh time.Duration `mapstructure:"jwks_max_refresh"`
}

// TokenExchangeConfig contains configuration for RFC 8693 Token Exchange.
// This allows gateways to exchange tokens issued by the upstream OAuth2 server
// for third-party OAuth2 tokens stored in the token vault.
type TokenExchangeConfig struct {
	// ExpectedAudience is the broker's own identifier that must appear in the audience (aud) claim
	// of both subject_token and client_assertion JWTs submitted for token exchange.
	// If empty, defaults to "token-exchange-broker".
	// Configurable to support deployments where the broker is known under a different identifier.
	// Environment variable: IDENTITY_BROKER_TOKEN_EXCHANGE_EXPECTED_AUDIENCE
	ExpectedAudience string `mapstructure:"expected_audience"`

	// ClientAssertion configures the external identity provider used to validate privileged
	// gateway client assertions.
	ClientAssertion ClientAssertionTrustConfig `mapstructure:"client_assertion"`

	// ClaimExtraction defines how to extract user principal and agent identifier from subject_token JWT.
	// Both are configurable via CEL expressions for flexibility in token structure mapping.
	ClaimExtraction ClaimExtractionConfig `mapstructure:"claim_extraction"`

	// Authorization defines authorization policies for token exchange requests.
	// Controls whether a gateway (identified by client_assertion) is authorized to perform token exchange.
	Authorization AuthorizationConfig `mapstructure:"authorization"`
}

// ClaimExtractionConfig defines CEL expressions for extracting claims from subject_token JWT.
// These expressions are evaluated to extract the user principal and agent identifier.
type ClaimExtractionConfig struct {
	// PrincipalExpression is a CEL expression that extracts the user principal from subject_token.
	// The expression receives the validated subject_token JWT as input.
	// Default: "subject_token.sub"
	// Examples: "subject_token.sub", "subject_token['preferred_username']"
	// This value is REQUIRED and MUST be a valid CEL expression.
	// It is validated at startup (FR-017) and will cause startup failure if invalid.
	PrincipalExpression string `mapstructure:"principal_expression" validate:"required"`

	// AgentIDExpression is a CEL expression that extracts the agent identifier from subject_token.
	// The expression receives the validated subject_token JWT as input.
	// Default: "subject_token.azp"
	// Examples: "resolveAgentIdByClientId(subject_token.azp)" (feature disabled),
	//           "subject_token.x_agent_id" (feature enabled, use configured agent_id_claim_name)
	// This value is REQUIRED and MUST be a valid CEL expression.
	// It is validated at startup (FR-017) and will cause startup failure if invalid.
	AgentIDExpression string `mapstructure:"agent_id_expression" validate:"required"`
}

// AuthorizationConfig defines broker-side authorization policies for RFC 8693 token exchange.
// It does not control ExtProc request/tool authorization in internal/extproc, which is a
// separate service boundary with its own configuration schema.
type AuthorizationConfig struct {
	// Type specifies the broker-side authorization method: "cel" (implemented) or "opa"
	// (reserved for a future broker-side design).
	// Default: "cel"
	Type string `mapstructure:"type" validate:"required,oneof=cel opa"`

	// CEL contains CEL-based authorization configuration.
	// This is used when Type is "cel".
	CEL CELAuthorizationConfig `mapstructure:"cel"`

	// OPA contains broker-side OPA authorization configuration.
	// Reserved for future broker token-exchange authorization. Currently ignored.
	// This is distinct from the ExtProc OPA authorizer used by cmd/extproc-token-exchange.
	OPA OPAAuthorizationConfig `mapstructure:"opa"`
}

// CELAuthorizationConfig defines CEL-based authorization for token exchange.
// CEL expressions are used to authorize gateways (identified by client_assertion) to perform token exchange.
type CELAuthorizationConfig struct {
	// Expression is a CEL expression that determines whether a gateway is authorized for token exchange.
	// The expression receives a context object with:
	//   - claims: The validated client_assertion JWT claims (sub, aud, iss, exp, iat, custom claims)
	//   - request: The token exchange request context (resource, grant_type, scope, etc.)
	//
	// The expression MUST return a boolean:
	//   - true: Gateway is authorized, proceed with token exchange
	//   - false: Gateway is not authorized, return 403 Forbidden with error=access_denied
	//
	// Default: "true" (allow all valid gateways after basic validation)
	// Examples: "claims.iss == 'https://auth.example.com'",
	//           "claims.sub in ['gateway-1', 'gateway-2']"
	// This value is REQUIRED and MUST be a valid CEL expression.
	// It is validated at startup (FR-017) and will cause startup failure if invalid.
	Expression string `mapstructure:"expression" validate:"required"`

	// EvaluationTimeout is the maximum time allowed for CEL expression evaluation.
	// If evaluation exceeds this timeout, the request returns 500 Internal Server Error.
	// Prevents runaway CEL expressions from blocking token exchange.
	// Default: 100ms
	// Must be between 10ms and 5000ms.
	EvaluationTimeout time.Duration `mapstructure:"evaluation_timeout" validate:"min=10ms,max=5s"`
}

// OPAAuthorizationConfig is reserved for future broker-side OPA support.
// It does not refer to ExtProc's standalone OPA request authorizer.
type OPAAuthorizationConfig struct {
	// PolicyURL is the broker-side OPA server endpoint (e.g., "http://opa:8181").
	// Not implemented in the current broker token-exchange flow.
	PolicyURL string `mapstructure:"policy_url"`

	// PolicyPath is the broker-side OPA policy path for token exchange evaluation.
	// Not implemented in the current broker token-exchange flow.
	PolicyPath string `mapstructure:"policy_path"`
}

// SecurityConfig contains security-related configuration.
type SecurityConfig struct {
	// SkipThirdpartyHTTPSValidation skips HTTPS certificate validation for third-party OAuth2 services.
	// WARNING: This is ONLY for development/test environments!
	// Allows HTTP connections and invalid HTTPS certificates.
	// NEVER enable this in production.
	SkipThirdpartyHTTPSValidation bool `mapstructure:"skip_thirdparty_https_validation"`

	// SkipCIMDSSRFValidation disables the SSRF IP blocklist and TLS certificate verification
	// for CIMD document fetches.
	// WARNING: This is ONLY for development/test environments with a local mock CIMD server!
	// NEVER enable this in production.
	SkipCIMDSSRFValidation bool `mapstructure:"skip_cimd_ssrf_validation"`
}

// EncryptionConfig contains configuration for encryption operations.
// Uses backend-explicit design to enforce exactly one encryption backend.
// This makes illegal states unrepresentable at the type level.
type EncryptionConfig struct {
	// AWSKMS contains AWS KMS backend configuration.
	// When set, the system uses AWS KMS with hierarchical keyring for envelope encryption.
	// Exactly one of AWSKMS or Memory must be non-nil.
	AWSKMS *AWSKMSConfig `mapstructure:"aws_kms"`

	// Memory contains in-memory backend configuration.
	// When set, the system uses raw AES keyring with environment variable KEK.
	// Exactly one of AWSKMS or Memory must be non-nil.
	Memory *MemoryConfig `mapstructure:"memory"`
}

// AWSKMSConfig contains AWS KMS specific configuration for envelope encryption.
// Used when EncryptionConfig.AWSKMS is non-nil.
type AWSKMSConfig struct {
	// KeyARN specifies the AWS KMS Customer-Managed Key (CMK) ARN.
	// Format: "arn:aws:kms:region:account-id:key/key-id" or "arn:aws:kms:region:account-id:alias/alias-name"
	// REQUIRED when AWS KMS backend is selected.
	//
	// Examples:
	//   - "arn:aws:kms:us-east-1:123456789012:key/12345678-1234-1234-1234-123456789012"
	//   - "arn:aws:kms:us-east-1:123456789012:alias/my-encryption-key"
	KeyARN string `mapstructure:"key_arn" validate:"required_if_backend"`

	// DynamoDBTableName specifies the DynamoDB table for caching branch keys.
	// The hierarchical keyring uses this table to cache branch keys, reducing KMS API calls.
	// Defaults to "IdentityBrokerEncryptionBranchKeys" if not specified.
	//
	// Required table schema:
	// - Partition key: "BranchKeyId" (String)
	// - Sort key: "TimeToLive" (Number, for TTL-based auto-deletion)
	DynamoDBTableName string `mapstructure:"dynamodb_table_name"`

	// BranchKeyTTL specifies the Time-To-Live for cached branch keys in DynamoDB.
	// Valid range: 1 minute to 24 hours. Defaults to "1h" if not specified.
	// Format: duration string (e.g., "1h", "30m", "3600s")
	BranchKeyTTL string `mapstructure:"branch_key_ttl"`

	// DynamoDBRegion specifies the AWS region for DynamoDB operations.
	// If not specified, uses default AWS SDK region resolution.
	// Examples: "us-east-1", "eu-west-1", "ap-southeast-1"
	DynamoDBRegion string `mapstructure:"dynamodb_region"`

	// DynamoDBTimeout specifies the context timeout applied to each top-level
	// keystore/encrypt/decrypt operation that performs DynamoDB access.
	// The timeout is enforced via context.WithTimeout at the call site so that the
	// standard Go context cancellation chain is respected end-to-end.
	// This deadline is shared by all downstream calls made during that operation,
	// including DynamoDB and any other AWS service calls involved.
	// Format: duration string (e.g., "5s", "10s", "1m"). Defaults to no timeout when empty.
	DynamoDBTimeout string `mapstructure:"dynamodb_timeout"`

	// AWS SDK Configuration (optional - empty/falsy values use AWS SDK defaults)

	// Region specifies the AWS region for KMS operations.
	// If not specified, uses default AWS SDK region resolution.
	// Examples: "us-east-1", "eu-west-1", "ap-southeast-1"
	Region string `mapstructure:"region"`

	// KMSEndpoint specifies a custom KMS endpoint URL.
	// Used for testing with a LocalStack-compatible AWS emulator or custom KMS implementations.
	// Example: "http://localhost:4566" (LocalStack-compatible AWS emulator)
	KMSEndpoint string `mapstructure:"kms_endpoint"`

	// DynamoDBEndpoint specifies a custom DynamoDB endpoint URL.
	// Used for testing with a LocalStack-compatible AWS emulator or DynamoDB Local.
	// Example: "http://localhost:4566" (LocalStack-compatible AWS emulator)
	DynamoDBEndpoint string `mapstructure:"dynamodb_endpoint"`

	// Profile specifies the AWS profile to use for credentials.
	// Uses credentials from ~/.aws/credentials or ~/.aws/config.
	// Examples: "default", "production", "development"
	Profile string `mapstructure:"profile"`

	// AccessKeyID specifies static AWS access key ID.
	// Used for testing or environments without IAM role access.
	// SECURITY: This field contains sensitive data and will be redacted in logs.
	AccessKeyID string `mapstructure:"access_key_id"`

	// SecretAccessKey specifies static AWS secret access key.
	// Used with AccessKeyID for static credential authentication.
	// SECURITY: This field contains sensitive data and will be redacted in logs.
	SecretAccessKey string `mapstructure:"secret_access_key"`

	// AssumeRoleARN specifies an IAM role ARN to assume for operations.
	// Used in production environments for role-based access.
	// Format: "arn:aws:iam::account-id:role/role-name"
	// Example: "arn:aws:iam::123456789012:role/EncryptionRole"
	AssumeRoleARN string `mapstructure:"assume_role_arn"`

	// DisableSSL disables SSL verification for AWS API calls.
	// WARNING: Only use for development/testing with a LocalStack-compatible AWS emulator.
	// NEVER enable this in production environments.
	DisableSSL bool `mapstructure:"disable_ssl"`
}

// MemoryConfig contains in-memory backend configuration for envelope encryption.
// Used when EncryptionConfig.Memory is non-nil.
type MemoryConfig struct {
	// RawKey specifies the base64-encoded AES-256 key for envelope encryption.
	// Must be exactly 32 bytes (256 bits) when decoded.
	// REQUIRED when Memory backend is selected.
	//
	// Generate with: openssl rand -base64 32
	// Environment variable injection supported: "${ENCRYPTION_KEK}"
	//
	// SECURITY: This field contains sensitive key material and will be redacted in logs.
	RawKey string `mapstructure:"raw_key" validate:"required_if_backend"`
}

// TelemetryConfig contains OpenTelemetry observability configuration.
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
// Not all OTEL collectors support the LogsService gRPC service; when the collector
// does not, set Enabled=false to suppress connection errors.
type LogsConfig struct {
	Enabled bool `mapstructure:"enabled"`
}

// OTLPProtocol identifies the transport protocol for the OTLP exporter.
type OTLPProtocol = string

// OTLPCompression identifies the payload compression algorithm for the OTLP exporter.
type OTLPCompression = string

const (
	// OTLPProtocolGRPC uses gRPC transport for OTLP export.
	// Endpoint format: host:port (e.g. "collector:4317").
	// TLS is controlled by the Insecure flag.
	OTLPProtocolGRPC OTLPProtocol = "grpc"
	// OTLPProtocolHTTP uses HTTP/protobuf transport for OTLP export.
	// Endpoint format: full URL including scheme (e.g. "http://collector:4318" or "https://collector:4318").
	// The URL scheme determines whether TLS is used; the Insecure flag is ignored.
	OTLPProtocolHTTP OTLPProtocol = "http"
	// OTLPProtocolHTTPS uses HTTP/protobuf transport over TLS for OTLP export.
	// Endpoint format: host:port (e.g. "collector:4318") — https:// is added automatically.
	// A full https:// URL is also accepted. Using http:// is rejected at validation time.
	OTLPProtocolHTTPS OTLPProtocol = "https"
)

const (
	// OTLPCompressionNone sends payloads uncompressed (default).
	OTLPCompressionNone OTLPCompression = "none"
	// OTLPCompressionGzip compresses payloads with gzip before sending.
	OTLPCompressionGzip OTLPCompression = "gzip"
)

// OTLPExporterConfig contains OTLP exporter connection parameters.
type OTLPExporterConfig struct {
	Protocol    OTLPProtocol      `mapstructure:"protocol"`
	Endpoint    string            `mapstructure:"endpoint"`
	Headers     map[string]string `mapstructure:"headers"`
	Timeout     time.Duration     `mapstructure:"timeout"`
	Insecure    bool              `mapstructure:"insecure"`
	Compression OTLPCompression   `mapstructure:"compression"`
}

// DefaultTelemetryConfig returns default telemetry configuration.
// Telemetry is DISABLED by default; all other values are production-safe defaults.
func DefaultTelemetryConfig() TelemetryConfig {
	return TelemetryConfig{
		Enabled:            false,
		ServiceName:        "agentic-identity-broker",
		ResourceAttributes: map[string]string{},
		Traces: TracesConfig{
			Enabled:      true,
			SamplingRate: 1.0,
			Propagators:  []string{"tracecontext", "ottrace", "b3multi", "baggage"},
		},
		Metrics: MetricsConfig{
			Enabled:        true,
			ExportInterval: 30 * time.Second,
		},
		Logs: LogsConfig{
			Enabled: true,
		},
		Exporter: OTLPExporterConfig{
			Protocol:    OTLPProtocolGRPC,
			Endpoint:    "",
			Headers:     map[string]string{},
			Timeout:     10 * time.Second,
			Insecure:    false,
			Compression: OTLPCompressionNone,
		},
	}
}

// ApprovalsConfig contains configuration for the tool approval system.
type ApprovalsConfig struct {
	PendingTTL         time.Duration           `mapstructure:"pending_ttl"`
	SyncCoalesceWindow time.Duration           `mapstructure:"sync_coalesce_window"`
	RateLimit          ApprovalRateLimitConfig `mapstructure:"rate_limit"`
}

// ApprovalRateLimitConfig contains rate limiting settings for approval creation.
type ApprovalRateLimitConfig struct {
	MaxPendingPerPair    int `mapstructure:"max_pending_per_pair"`
	MaxRequestsPerMinute int `mapstructure:"max_requests_per_minute"`
}
