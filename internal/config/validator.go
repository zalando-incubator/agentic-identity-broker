// Package config implements the configuration loading adapter.
package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/servermode"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Validate validates the complete configuration structure.
// Returns a ConfigError with detailed information on validation failures.
func Validate(cfg *ports.Config) error {
	// Validate log level
	if err := validateLogLevel(cfg.Log.Level); err != nil {
		return formatValidationError("log.level", string(cfg.Log.Level), "debug, info, warn, or error", err)
	}

	// Validate log format
	if err := validateLogFormat(cfg.Log.Format); err != nil {
		return formatValidationError("log.format", string(cfg.Log.Format), "text or json", err)
	}

	// Validate server configuration (pass security config for HTTPS validation)
	if err := validateServerConfig(&cfg.Server, &cfg.Security); err != nil {
		return err
	}

	if err := validateRequestContextConfig(&cfg.RequestContext); err != nil {
		return err
	}

	// Validate storage configuration
	if err := validateStorageConfig(&cfg.Storage); err != nil {
		return err
	}

	// Validate third-party OAuth2 configuration
	if err := validateThirdPartyOAuth2Config(&cfg.ThirdPartyOAuth2); err != nil {
		return err
	}

	// Validate OAuth2 Authorization Server configuration (mandatory)
	if err := validateOAuth2AuthServerConfig(&cfg.OAuth2AuthServer); err != nil {
		return err
	}

	// Validate optional impersonation configuration (local mode only, CR-001..CR-008).
	if cfg.OAuth2AuthServer.Impersonation != nil {
		if err := validateImpersonationConfig(cfg.OAuth2AuthServer.Impersonation, cfg.OAuth2AuthServer.Mode, &cfg.Security); err != nil {
			return err
		}
	}

	if err := validateTokenExchangeConfig(&cfg.TokenExchange, &cfg.Security); err != nil {
		return err
	}

	// Validate encryption configuration
	if err := validateEncryptionConfig(&cfg.Encryption); err != nil {
		return err
	}

	// Validate telemetry configuration
	if err := validateTelemetryConfig(&cfg.Telemetry); err != nil {
		return err
	}

	return nil
}

// validateServerConfig validates the server configuration for all instances.
// Ensures required authentication settings are properly configured.
func validateServerConfig(sc *ports.ServerConfig, security *ports.SecurityConfig) error {
	// Validate EndUser server
	if err := validateServerInstance(&sc.EndUser, "server.enduser", security); err != nil {
		return err
	}

	// Validate Admin server
	if err := validateServerInstance(&sc.Admin, "server.admin", security); err != nil {
		return err
	}

	return nil
}

func validateRequestContextConfig(cfg *ports.RequestContextConfig) error {
	if cfg == nil {
		return nil
	}

	if !cfg.TrustedProxy.Enabled {
		return nil
	}

	if strings.TrimSpace(cfg.TrustedProxy.ForwardedHeader) == "" {
		return formatValidationError(
			"request_context.trusted_proxy.forwarded_header",
			cfg.TrustedProxy.ForwardedHeader,
			"non-empty header name when trusted proxy is enabled",
			nil,
		)
	}

	return nil
}

// validateServerInstanceAuth validates authentication configuration for a server instance.
// At least one authentication method must be configured: either JWT or preauth (principal header).
// When JWT is configured, it is the sole authentication mechanism (fail-closed, no fallback).
// When JWT is NOT configured, PrincipalHeaderName is required for plain-header preauth.
func validateServerInstanceAuth(sic *ports.ServerInstanceConfig, prefix string, security *ports.SecurityConfig) error {
	hasJWT := sic.Authentication.JWT != nil
	hasPreauth := sic.Authentication.Preauth.PrincipalHeaderName != ""

	// At least one authentication method must be configured
	if !hasJWT && !hasPreauth {
		return formatValidationError(
			prefix+".authentication",
			"",
			"at least one authentication method (jwt or preauth.principal_header_name)",
			nil,
		)
	}

	// Validate JWT configuration if present
	if hasJWT {
		if err := validateJWTConfig(sic.Authentication.JWT, prefix+".authentication.jwt", security); err != nil {
			return err
		}
	}

	return nil
}

// validateServerInstanceConfig validates a server instance configuration.
func validateServerInstance(sic *ports.ServerInstanceConfig, prefix string, security *ports.SecurityConfig) error {
	// Validate port
	if sic.Port < 1 || sic.Port > 65535 {
		return formatValidationError(
			prefix+".port",
			strconv.Itoa(sic.Port),
			"port number between 1 and 65535",
			nil,
		)
	}

	// Validate bind address
	if sic.Bind == "" {
		return formatValidationError(
			prefix+".bind",
			"",
			"non-empty bind address",
			nil,
		)
	}

	// Validate public URL (required for enduser server for OAuth2 callbacks)
	if prefix == "server.enduser" && sic.PublicURL == "" {
		return formatValidationError(
			prefix+".public_url",
			"",
			"non-empty public URL (required for OAuth2 callbacks)",
			nil,
		)
	}

	// Validate public URL format if provided
	if sic.PublicURL != "" {
		if !isValidURL(sic.PublicURL) {
			return formatValidationError(
				prefix+".public_url",
				sic.PublicURL,
				"valid HTTP/HTTPS URL",
				nil,
			)
		}
	}

	// Validate authentication
	if err := validateServerInstanceAuth(sic, prefix, security); err != nil {
		return err
	}

	// Validate CORS configuration (only when AllowedOrigins is non-empty)
	if err := validateCORSConfig(&sic.CORS, prefix+".cors"); err != nil {
		return err
	}

	return nil
}

// validateCORSConfig validates CORS configuration for a server instance.
// When AllowedOrigins is empty, CORS is disabled and no further validation is performed
// (this is the production-safe default). Validation only applies when CORS is explicitly enabled.
func validateCORSConfig(cfg *ports.CORSConfig, prefix string) error {
	// Empty AllowedOrigins = CORS disabled (secure default). Nothing to validate.
	if len(cfg.AllowedOrigins) == 0 {
		return nil
	}

	// When CORS is enabled, validate that no entry is an empty string.
	for i, origin := range cfg.AllowedOrigins {
		if origin == "" {
			return formatValidationError(
				fmt.Sprintf("%s.allowed_origins[%d]", prefix, i),
				"",
				"non-empty origin value",
				nil,
			)
		}
	}

	for i, method := range cfg.AllowedMethods {
		if method == "" {
			return formatValidationError(
				fmt.Sprintf("%s.allowed_methods[%d]", prefix, i),
				"",
				"non-empty HTTP method value",
				nil,
			)
		}
	}

	for i, header := range cfg.AllowedHeaders {
		if header == "" {
			return formatValidationError(
				fmt.Sprintf("%s.allowed_headers[%d]", prefix, i),
				"",
				"non-empty header name value",
				nil,
			)
		}
	}

	// MaxAge must be non-negative (0 means no explicit max-age / use browser default).
	if cfg.MaxAge < 0 {
		return formatValidationError(
			prefix+".max_age",
			fmt.Sprintf("%d", cfg.MaxAge),
			"non-negative integer (seconds)",
			nil,
		)
	}

	return nil
}

// validateStorageConfig validates the storage configuration.
// Ensures backend is valid and backend-specific parameters are present.
func validateStorageConfig(sc *ports.StorageConfig) error {
	// Validate backend value
	if sc.Backend != "memory" && sc.Backend != "postgres" {
		return formatValidationError("storage.backend", sc.Backend, "memory or postgres", nil)
	}

	// Validate storage timeouts
	if err := validateStorageTimeouts(&sc.Timeouts); err != nil {
		return err
	}

	// Validate backend-specific parameters
	if sc.Backend == "postgres" {
		if err := validatePostgresConfig(&sc.Postgres); err != nil {
			return err
		}
	}

	return nil
}

// validateStorageTimeouts validates storage timeout configuration.
func validateStorageTimeouts(st *ports.StorageTimeouts) error {
	if st.Read <= 0 {
		return formatValidationError("storage.timeouts.read", st.Read.String(), "positive duration", nil)
	}
	if st.Write <= 0 {
		return formatValidationError("storage.timeouts.write", st.Write.String(), "positive duration", nil)
	}
	return nil
}

// validatePostgresConfig validates PostgreSQL-specific configuration.
func validatePostgresConfig(pc *ports.PostgresConfig) error {
	if pc.ConnectionURL == "" {
		return formatValidationError("storage.postgres.connection_url", "", "non-empty PostgreSQL connection URL", nil)
	}

	// Validate connection URL format
	if !isValidPostgresURL(pc.ConnectionURL) {
		return formatValidationError("storage.postgres.connection_url", pc.ConnectionURL, "valid postgresql:// URL", nil)
	}

	return nil
}

// isValidPostgresURL checks if a string is a valid PostgreSQL URL format.
func isValidPostgresURL(s string) bool {
	if s == "" {
		return false
	}
	return (len(s) > 13 && (s[:13] == "postgresql://" || s[:11] == "postgres://"))
}

// validateLogLevel validates the log level value.
// Fast, zero-allocation validation using domain type.
func validateLogLevel(level ports.LogLevel) error {
	domainLevel := domconfig.LogLevel(level)
	return domainLevel.Validate()
}

// validateLogFormat validates the log format value.
// Fast, zero-allocation validation using domain type.
func validateLogFormat(format ports.LogFormat) error {
	domainFormat := domconfig.LogFormat(format)
	return domainFormat.Validate()
}

// validateThirdPartyOAuth2Config validates the third-party OAuth2 configuration.
func validateThirdPartyOAuth2Config(cfg *ports.ThirdPartyOAuth2Config) error {

	// JWESigningKey is mandatory - must be provided
	if cfg.JWESigningKey == "" {
		return formatValidationError(
			"third_party_oauth2.jwe_signing_key",
			"",
			"base64-encoded key (32 bytes)",
			nil,
		)
	}

	// Validate base64 encoding
	keyBytes, err := base64.StdEncoding.DecodeString(cfg.JWESigningKey)
	if err != nil {
		// Don't log full key value for security
		truncated := cfg.JWESigningKey
		if len(truncated) > 10 {
			truncated = truncated[:10] + "..."
		}
		return formatValidationError(
			"third_party_oauth2.jwe_signing_key",
			truncated,
			"valid base64-encoded string",
			err,
		)
	}

	// Validate key length (must be exactly 32 bytes for A256GCMKW)
	if len(keyBytes) != 32 {
		return formatValidationError(
			"third_party_oauth2.jwe_signing_key",
			fmt.Sprintf("%d bytes", len(keyBytes)),
			"exactly 32 bytes when decoded",
			nil,
		)
	}

	// Validate StateTokenTTL if provided (should be positive)
	if cfg.StateTokenTTL > 0 {
		// Validate it's within the maximum of 15 minutes per SR-008
		if cfg.StateTokenTTL > 15*60*1e9 { // 15 minutes in nanoseconds
			return formatValidationError(
				"third_party_oauth2.state_token_ttl",
				cfg.StateTokenTTL.String(),
				"positive duration (max 15 minutes per SR-008)",
				nil,
			)
		}
	}

	// Validate PKCEVerifierLength if provided (should be within RFC 7636 limits)
	if cfg.PKCEVerifierLength > 0 {
		if cfg.PKCEVerifierLength < 32 || cfg.PKCEVerifierLength > 128 {
			return formatValidationError(
				"third_party_oauth2.pkce_verifier_length",
				strconv.Itoa(cfg.PKCEVerifierLength),
				"32-128 bytes per RFC 7636",
				nil,
			)
		}
	}

	return nil
}

// validateOAuth2AuthServerConfig validates the OAuth2 Authorization Server configuration.
// OAuth2AuthServer is optional; Validate() handles the unconfigured case internally.
func validateOAuth2AuthServerConfig(cfg *ports.OAuth2AuthServerConfig) error {
	if err := cfg.Validate(); err != nil {
		return formatValidationError(
			"oauth2_authorization_server",
			"",
			"valid OAuth2 authorization server configuration",
			err,
		)
	}
	return nil
}

// formatValidationError converts validation errors to ConfigError with context.
func formatValidationError(field string, value string, expected string, err error) error {
	return &domconfig.ConfigError{
		Field:    field,
		Value:    value,
		Expected: expected,
		Source:   "", // Will be filled in by caller if known
		Err:      err,
	}
}

// validateEncryptionConfig validates the new backend-explicit encryption configuration.
// Ensures exactly one backend (AWS KMS or Memory) is configured.
func validateEncryptionConfig(cfg *ports.EncryptionConfig) error {
	// Count configured backends
	backendCount := 0
	if cfg.AWSKMS != nil {
		backendCount++
	}
	if cfg.Memory != nil {
		backendCount++
	}

	// Ensure exactly one backend is configured
	if backendCount == 0 {
		return formatValidationError(
			"encryption",
			"neither backend configured",
			"exactly one backend (aws_kms or memory) must be configured",
			nil,
		)
	}
	if backendCount > 1 {
		return formatValidationError(
			"encryption",
			"multiple backends configured",
			"exactly one backend (aws_kms or memory) must be configured, not both",
			nil,
		)
	}

	// Validate backend-specific configuration
	if cfg.AWSKMS != nil {
		if err := validateAWSKMSConfig(cfg.AWSKMS); err != nil {
			return err
		}
	}
	if cfg.Memory != nil {
		if err := validateMemoryConfig(cfg.Memory); err != nil {
			return err
		}
	}

	return nil
}

// validateAWSKMSConfig validates AWS KMS backend configuration.
func validateAWSKMSConfig(cfg *ports.AWSKMSConfig) error {
	// KeyARN is required
	if cfg.KeyARN == "" {
		return formatValidationError(
			"encryption.aws_kms.key_arn",
			"",
			"AWS KMS ARN (arn:aws:kms:region:account:key/key-id)",
			nil,
		)
	}

	// Validate KeyARN format
	if !strings.HasPrefix(cfg.KeyARN, "arn:aws:kms:") {
		return formatValidationError(
			"encryption.aws_kms.key_arn",
			maskSensitiveValue(cfg.KeyARN),
			"valid AWS KMS ARN format (arn:aws:kms:region:account:key/key-id)",
			nil,
		)
	}

	// Basic ARN structure validation
	parts := strings.Split(cfg.KeyARN, ":")
	if len(parts) < 6 {
		return formatValidationError(
			"encryption.aws_kms.key_arn",
			maskSensitiveValue(cfg.KeyARN),
			"valid AWS KMS ARN with at least 6 colon-separated parts",
			nil,
		)
	}

	// Validate DynamoDBTimeout when set
	if cfg.DynamoDBTimeout != "" {
		d, err := time.ParseDuration(cfg.DynamoDBTimeout)
		if err != nil {
			return formatValidationError(
				"encryption.aws_kms.dynamodb_timeout",
				cfg.DynamoDBTimeout,
				"valid positive duration (e.g. 5s, 30s)",
				err,
			)
		}
		if d <= 0 {
			return formatValidationError(
				"encryption.aws_kms.dynamodb_timeout",
				cfg.DynamoDBTimeout,
				"positive duration (must be > 0)",
				nil,
			)
		}
	}

	return nil
}

// validateMemoryConfig validates Memory backend configuration.
func validateMemoryConfig(cfg *ports.MemoryConfig) error {
	// RawKey is required
	if cfg.RawKey == "" {
		return formatValidationError(
			"encryption.memory.raw_key",
			"",
			"base64-encoded 32-byte AES-256 key",
			nil,
		)
	}

	// Validate base64 encoding
	keyBytes, err := base64.StdEncoding.DecodeString(cfg.RawKey)
	if err != nil {
		return formatValidationError(
			"encryption.memory.raw_key",
			maskSensitiveValue(cfg.RawKey),
			"valid base64-encoded string",
			err,
		)
	}

	// Validate key length (must be exactly 32 bytes for AES-256)
	if len(keyBytes) != 32 {
		return formatValidationError(
			"encryption.memory.raw_key",
			fmt.Sprintf("%d bytes", len(keyBytes)),
			"exactly 32 bytes when decoded (AES-256)",
			nil,
		)
	}

	return nil
}

// validateJWTConfig validates JWT pre-authentication configuration.
// Applies defaults for HeaderName, Verification, and PrincipalExpression when not set.
// Enforces mutual exclusivity between verification: none and jwks_uri.
// Enforces HTTPS for JWKS URI per SR-004.
func validateJWTConfig(jwt *ports.JWTConfig, prefix string, security *ports.SecurityConfig) error {
	// Apply defaults for fields that have default values
	if jwt.HeaderName == "" {
		jwt.HeaderName = "Authorization"
	}
	if jwt.Verification == "" {
		jwt.Verification = "jwks"
	}
	if jwt.ClaimExtraction.PrincipalExpression == "" {
		jwt.ClaimExtraction.PrincipalExpression = "claims.sub"
	}

	// Validate verification mode
	if jwt.Verification != "jwks" && jwt.Verification != "none" {
		return formatValidationError(
			prefix+".verification",
			jwt.Verification,
			"'jwks' or 'none'",
			nil,
		)
	}

	// Mutual exclusivity: verification: none + jwks_uri → startup error (FR-003a)
	if jwt.Verification == "none" && jwt.JWKSURI != "" {
		return formatValidationError(
			prefix,
			"verification: none with jwks_uri: "+jwt.JWKSURI,
			"verification 'none' and jwks_uri are mutually exclusive",
			nil,
		)
	}

	// Required jwks_uri when verification is jwks
	if jwt.Verification == "jwks" && jwt.JWKSURI == "" {
		return formatValidationError(
			prefix+".jwks_uri",
			"",
			"non-empty JWKS URI (required when verification is 'jwks')",
			nil,
		)
	}

	// HTTPS enforcement for JWKS URI (SR-004)
	if jwt.JWKSURI != "" {
		u, err := url.Parse(jwt.JWKSURI)
		if err != nil {
			return formatValidationError(
				prefix+".jwks_uri",
				jwt.JWKSURI,
				"valid URL",
				err,
			)
		}
		skipHTTPS := security != nil && security.SkipThirdpartyHTTPSValidation
		if u.Scheme == "http" && !skipHTTPS {
			return formatValidationError(
				prefix+".jwks_uri",
				jwt.JWKSURI,
				"HTTPS URL (set security.skip_thirdparty_https_validation to allow HTTP in development)",
				nil,
			)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return formatValidationError(
				prefix+".jwks_uri",
				jwt.JWKSURI,
				"HTTP or HTTPS URL",
				nil,
			)
		}
	}

	// Validate CEL claim extraction expressions by attempting compilation.
	// This catches syntax/type errors at startup rather than at request time.
	_, err := jwtauth.NewCELEvaluator(jwtauth.CELEvaluatorConfig{
		PrincipalExpression:   jwt.ClaimExtraction.PrincipalExpression,
		DisplayNameExpression: jwt.ClaimExtraction.DisplayNameExpression,
		EmailExpression:       jwt.ClaimExtraction.EmailExpression,
		PictureURLExpression:  jwt.ClaimExtraction.PictureURLExpression,
	}, nil)
	if err != nil {
		return formatValidationError(
			prefix+".claim_extraction",
			err.Error(),
			"valid CEL expressions for principal_expression, display_name_expression, email_expression, picture_url_expression",
			err,
		)
	}

	return nil
}

func validateTokenExchangeConfig(cfg *ports.TokenExchangeConfig, security *ports.SecurityConfig) error {
	clientAssertion := &cfg.ClientAssertion
	if err := validateOptionalHTTPSURL(clientAssertion.IssuerURI, "token_exchange.client_assertion.issuer_uri", security); err != nil {
		return err
	}
	if err := validateOptionalHTTPSURL(clientAssertion.JWKSURI, "token_exchange.client_assertion.jwks_uri", security); err != nil {
		return err
	}
	if clientAssertion.JWKSMinRefresh < 0 {
		return formatValidationError("token_exchange.client_assertion.jwks_min_refresh", clientAssertion.JWKSMinRefresh.String(), "non-negative duration", nil)
	}
	if clientAssertion.JWKSMaxRefresh < 0 {
		return formatValidationError("token_exchange.client_assertion.jwks_max_refresh", clientAssertion.JWKSMaxRefresh.String(), "non-negative duration", nil)
	}
	return nil
}

func validateOptionalHTTPSURL(value, field string, security *ports.SecurityConfig) error {
	if value == "" {
		return nil
	}
	u, err := url.Parse(value)
	if err != nil {
		return formatValidationError(field, value, "valid URL", err)
	}
	skipHTTPS := security != nil && security.SkipThirdpartyHTTPSValidation
	if u.Scheme == "http" && !skipHTTPS {
		return formatValidationError(field, value, "HTTPS URL (set security.skip_thirdparty_https_validation to allow HTTP in development)", nil)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return formatValidationError(field, value, "HTTP or HTTPS URL", nil)
	}
	return nil
}

// maskSensitiveValue masks sensitive values for display in error messages.
// Shows only the first 10 and last 5 characters for readability while maintaining some specificity.
func maskSensitiveValue(value string) string {
	if len(value) <= 15 {
		return "***" // Too short to safely display any part
	}
	return value[:10] + "..." + value[len(value)-5:]
}

// validateTelemetryConfig validates the OpenTelemetry configuration.
// Validation is skipped when telemetry is disabled (cfg.Enabled=false).
func validateTelemetryConfig(cfg *ports.TelemetryConfig) error {
	if !cfg.Enabled {
		return nil
	}

	// ServiceName must be non-empty (OTel semantic convention: service.name)
	if strings.TrimSpace(cfg.ServiceName) == "" {
		return formatValidationError(
			"telemetry.service_name", cfg.ServiceName,
			"non-empty service name (OTel service.name resource attribute)", nil,
		)
	}

	// Endpoint is required when enabled
	if cfg.Exporter.Endpoint == "" {
		return formatValidationError(
			"telemetry.exporter.endpoint", "",
			"non-empty OTLP endpoint", nil,
		)
	}

	// Protocol must be grpc, http, or https
	if cfg.Exporter.Protocol != "grpc" && cfg.Exporter.Protocol != "http" && cfg.Exporter.Protocol != "https" {
		return formatValidationError(
			"telemetry.exporter.protocol", cfg.Exporter.Protocol,
			"one of: grpc, http, https", nil,
		)
	}

	// Normalize empty compression to "none" (canonical default).
	if cfg.Exporter.Compression == "" {
		cfg.Exporter.Compression = "none"
	}

	// Compression must be none or gzip
	if cfg.Exporter.Compression != "none" && cfg.Exporter.Compression != "gzip" {
		return formatValidationError(
			"telemetry.exporter.compression", cfg.Exporter.Compression,
			"one of: none, gzip", nil,
		)
	}

	// Validate endpoint format (must be valid host:port or URL)
	if err := validateOTLPEndpoint(cfg.Exporter.Endpoint, cfg.Exporter.Protocol); err != nil {
		return formatValidationError(
			"telemetry.exporter.endpoint", cfg.Exporter.Endpoint,
			"valid gRPC host:port or HTTP URL", err,
		)
	}

	// Sampling rate 0.0–1.0
	if cfg.Traces.SamplingRate < 0.0 || cfg.Traces.SamplingRate > 1.0 {
		return formatValidationError(
			"telemetry.traces.sampling_rate",
			fmt.Sprintf("%f", cfg.Traces.SamplingRate),
			"value between 0.0 and 1.0", nil,
		)
	}

	// Export interval must be positive when metrics enabled
	if cfg.Metrics.Enabled && cfg.Metrics.ExportInterval <= 0 {
		return formatValidationError(
			"telemetry.metrics.export_interval", cfg.Metrics.ExportInterval.String(),
			"positive duration", nil,
		)
	}

	// Exporter timeout must be positive
	if cfg.Exporter.Timeout <= 0 {
		return formatValidationError(
			"telemetry.exporter.timeout", cfg.Exporter.Timeout.String(),
			"positive duration", nil,
		)
	}

	return nil
}

// validateOTLPEndpoint validates an OTLP endpoint string.
// For gRPC: must be a valid host:port (no scheme); port must be a number in [1, 65535].
// For HTTP: must be a valid http:// or https:// URL.
// For HTTPS: must be a valid https:// URL or a bare host:port (auto-prefixed by the provider).
func validateOTLPEndpoint(endpoint, protocol string) error {
	if endpoint == "" {
		return fmt.Errorf("endpoint is empty")
	}
	if protocol == "http" {
		// HTTP endpoint must be a full http:// or https:// URL
		if !isValidURL(endpoint) {
			return fmt.Errorf("not a valid http(s) URL")
		}
		return nil
	}
	if protocol == "https" {
		// HTTPS accepts either a bare host:port (auto-prefixed) or a full https:// URL.
		// An http:// URL contradicts the https protocol choice.
		if strings.HasPrefix(endpoint, "http://") {
			return fmt.Errorf("protocol is https but endpoint uses http:// scheme — use https:// or bare host:port")
		}
		if strings.Contains(endpoint, "://") {
			// Full URL provided — must be https://
			if !strings.HasPrefix(endpoint, "https://") {
				return fmt.Errorf("protocol is https but endpoint has non-https scheme")
			}
			if !isValidURL(endpoint) {
				return fmt.Errorf("not a valid https URL")
			}
			return nil
		}
		// Bare host:port — validate format (will be auto-prefixed with https:// by the provider)
		host, portStr, err := net.SplitHostPort(endpoint)
		if err != nil {
			return fmt.Errorf("HTTPS endpoint must be host:port or https:// URL: %w", err)
		}
		if strings.TrimSpace(host) == "" {
			return fmt.Errorf("HTTPS endpoint host must not be empty: got %q", endpoint)
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			return fmt.Errorf("HTTPS endpoint port is not a number: %w", err)
		}
		if port < 1 || port > 65535 {
			return fmt.Errorf("HTTPS endpoint port %d is out of range [1, 65535]", port)
		}
		return nil
	}
	// gRPC: must be host:port without a URL scheme
	if strings.Contains(endpoint, "://") {
		return fmt.Errorf("gRPC endpoint should be host:port without scheme (e.g. collector:4317)")
	}
	host, portStr, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fmt.Errorf("gRPC endpoint must be host:port: %w", err)
	}
	_ = host // SplitHostPort guarantees non-empty host when err == nil
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("gRPC endpoint port is not a number: %w", err)
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("gRPC endpoint port %d is out of range [1, 65535]", port)
	}
	return nil
}

// isValidURL checks if a string is a valid HTTP or HTTPS URL.
func isValidURL(urlStr string) bool {
	if urlStr == "" {
		return false
	}

	// Parse URL
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}

	// Check scheme is http or https
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}

	// Check host is present
	if u.Host == "" {
		return false
	}

	return true
}

// validateImpersonationConfig validates the optional impersonation subtree (CR-001..CR-008).
// Local-issuer exclusion (CR-004) is enforced in the builder where the resolved local issuer
// URI is known.
func validateImpersonationConfig(cfg *ports.ImpersonationConfig, mode servermode.Mode, security *ports.SecurityConfig) error {
	const base = "oauth2_authorization_server.impersonation"

	// CR-006: impersonation is local mode only.
	if mode != servermode.Local {
		return formatValidationError(base, string(mode), "mode 'local' (impersonation is not supported in proxy or hybrid mode)", nil)
	}
	// CR-001: routing-only audience prefix.
	if err := impersonation.ValidateAudiencePrefix(cfg.AudiencePrefix); err != nil {
		return formatValidationError(base+".audience_prefix", cfg.AudiencePrefix, "absolute HTTP(S) URI with host and no userinfo, query, fragment, or trailing slash", err)
	}
	// CR-002: non-empty, ordered rules.
	if len(cfg.Rules) == 0 {
		return formatValidationError(base+".rules", "", "at least one impersonation rule", nil)
	}

	seenNames := make(map[string]bool)
	for i := range cfg.Rules {
		rule := &cfg.Rules[i]
		rbase := fmt.Sprintf("%s.rules[%d]", base, i)
		if rule.Name == "" {
			return formatValidationError(rbase+".name", "", "non-empty, unique rule name", nil)
		}
		if seenNames[rule.Name] {
			return formatValidationError(rbase+".name", rule.Name, "unique rule name across rules", nil)
		}
		seenNames[rule.Name] = true
		if err := validateImpersonationRule(rule, rbase, security); err != nil {
			return err
		}
	}
	return nil
}

var impersonationRoleKeys = []string{
	string(ports.CredentialRoleClientAssertion),
	string(ports.CredentialRoleActor),
	string(ports.CredentialRoleSubject),
}

// validateImpersonationRule validates a single rule's roles, authorization predicate, and
// trusted issuers (CR-003, CR-005, CR-007, CR-008).
func validateImpersonationRule(rule *ports.ImpersonationRuleConfig, rbase string, security *ports.SecurityConfig) error {
	validRole := map[string]bool{
		string(ports.CredentialRoleClientAssertion): true,
		string(ports.CredentialRoleActor):           true,
		string(ports.CredentialRoleSubject):         true,
	}
	// CR-003: role keys subset of the credential roles; all three defined.
	for key := range rule.Roles {
		if !validRole[key] {
			return formatValidationError(rbase+".roles", key, "role keys in {client_assertion, actor, subject}", nil)
		}
	}
	for _, roleName := range impersonationRoleKeys {
		role, ok := rule.Roles[roleName]
		if !ok {
			return formatValidationError(rbase+".roles."+roleName, "", "the role to be defined", nil)
		}
		pbase := rbase + ".roles." + roleName
		if role.PrincipalExpression == "" {
			return formatValidationError(pbase+".principal_expression", "", "non-empty CEL principal_expression", nil)
		}
		if roleName != string(ports.CredentialRoleSubject) {
			if role.EmailExpression != "" {
				return formatValidationError(pbase+".email_expression", role.EmailExpression, "email_expression only on the subject role", nil)
			}
			if role.Verification != "" {
				return formatValidationError(pbase+".verification", role.Verification, "verification only on the subject role", nil)
			}
		}
	}

	subject := rule.Roles[string(ports.CredentialRoleSubject)]
	subjectUnverified := subject.Verification == ports.SubjectVerificationNone
	// FR-003d/ADR 031: the subject role may be signed ("jwks", default) or unverified ("none").
	if subject.Verification != "" &&
		subject.Verification != ports.SubjectVerificationJWKS &&
		subject.Verification != ports.SubjectVerificationNone {
		return formatValidationError(rbase+".roles.subject.verification", subject.Verification, "'jwks' or 'none'", nil)
	}
	if subjectUnverified {
		// The unverified subject carries no signature or audience: expected_audience is forbidden.
		if subject.ExpectedAudience != "" {
			return formatValidationError(rbase+".roles.subject.expected_audience", subject.ExpectedAudience,
				"unset for the unverified subject mode (verification: none)", nil)
		}
	}
	// CR-003 / CR-009: signed roles carry either expected_audience or audience_requirement: absent.
	for _, roleName := range impersonationRoleKeys {
		role := rule.Roles[roleName]
		pbase := rbase + ".roles." + roleName
		unverifiedSubjectRole := roleName == string(ports.CredentialRoleSubject) && subjectUnverified
		if role.AudienceRequirement != "" {
			if unverifiedSubjectRole {
				return formatValidationError(pbase+".audience_requirement", role.AudienceRequirement, "unset for the unverified subject mode", nil)
			}
			if role.AudienceRequirement != ports.ImpersonationAudienceRequirementAbsent {
				return formatValidationError(pbase+".audience_requirement", role.AudienceRequirement, "'absent' or unset", nil)
			}
			if role.ExpectedAudience != "" {
				return formatValidationError(pbase+".expected_audience", role.ExpectedAudience, "unset when audience_requirement is 'absent'", nil)
			}
			continue
		}
		if unverifiedSubjectRole {
			continue
		}
		if role.ExpectedAudience == "" {
			return formatValidationError(pbase+".expected_audience", "", "non-empty expected_audience for a signed role", nil)
		}
	}

	// CR-005: exactly one CEL authorization predicate.
	if rule.Authorization.Type != "cel" {
		return formatValidationError(rbase+".authorization.type", rule.Authorization.Type, "'cel'", nil)
	}
	if rule.Authorization.CEL.Expression == "" {
		return formatValidationError(rbase+".authorization.cel.expression", "", "non-empty CEL authorization expression", nil)
	}
	if t := rule.Authorization.CEL.EvaluationTimeout; t != 0 && (t < 10*time.Millisecond || t > 5*time.Second) {
		return formatValidationError(rbase+".authorization.cel.evaluation_timeout", t.String(), "a duration between 10ms and 5s", nil)
	}

	return validateImpersonationTrustedIssuers(rule, rbase, security)
}

// validateImpersonationTrustedIssuers validates the rule's trust anchors (CR-007, CR-008).
func validateImpersonationTrustedIssuers(rule *ports.ImpersonationRuleConfig, rbase string, security *ports.SecurityConfig) error {
	if len(rule.TrustedIssuers) == 0 {
		return formatValidationError(rbase+".trusted_issuers", "", "at least one trusted issuer", nil)
	}
	subjectUnverified := rule.Roles[string(ports.CredentialRoleSubject)].Verification == ports.SubjectVerificationNone
	signedRole := map[string]bool{
		string(ports.CredentialRoleClientAssertion): true,
		string(ports.CredentialRoleActor):           true,
		string(ports.CredentialRoleSubject):         true,
	}
	seenIssuer := make(map[string]bool)
	covered := make(map[string]bool)
	for j := range rule.TrustedIssuers {
		issuer := &rule.TrustedIssuers[j]
		ibase := fmt.Sprintf("%s.trusted_issuers[%d]", rbase, j)
		if issuer.IssuerURI == "" {
			return formatValidationError(ibase+".issuer_uri", "", "non-empty issuer_uri", nil)
		}
		// CR-008: issuer_uri unique within the rule.
		if seenIssuer[issuer.IssuerURI] {
			return formatValidationError(ibase+".issuer_uri", issuer.IssuerURI, "unique issuer_uri within the rule", nil)
		}
		seenIssuer[issuer.IssuerURI] = true
		if err := validateOptionalHTTPSURL(issuer.IssuerURI, ibase+".issuer_uri", security); err != nil {
			return err
		}
		if err := validateOptionalHTTPSURL(issuer.JWKSURI, ibase+".jwks_uri", security); err != nil {
			return err
		}
		if issuer.JWKSMinRefresh < 0 {
			return formatValidationError(ibase+".jwks_min_refresh", issuer.JWKSMinRefresh.String(), "non-negative duration", nil)
		}
		if issuer.JWKSMaxRefresh < 0 {
			return formatValidationError(ibase+".jwks_max_refresh", issuer.JWKSMaxRefresh.String(), "non-negative duration", nil)
		}
		// CR-007: non-empty, approved asymmetric algorithms only.
		if len(issuer.AllowedAlgorithms) == 0 {
			return formatValidationError(ibase+".allowed_algorithms", "", "a non-empty list of approved asymmetric algorithms", nil)
		}
		for _, alg := range issuer.AllowedAlgorithms {
			if !impersonation.IsApprovedAlgorithm(alg) {
				return formatValidationError(ibase+".allowed_algorithms", alg,
					"an approved asymmetric algorithm (RS/PS/ES 256/384/512 or EdDSA); none and HS* are rejected", nil)
			}
		}
		// CR-008: signs_roles non-empty, subset of the rule's signed roles.
		if len(issuer.SignsRoles) == 0 {
			return formatValidationError(ibase+".signs_roles", "", "a non-empty list of signed roles", nil)
		}
		for _, r := range issuer.SignsRoles {
			if !signedRole[r] {
				return formatValidationError(ibase+".signs_roles", r, "roles in {client_assertion, actor, subject}", nil)
			}
			if r == string(ports.CredentialRoleSubject) && subjectUnverified {
				return formatValidationError(ibase+".signs_roles", r,
					"the unverified subject role (verification: none) to be absent from signs_roles", nil)
			}
			covered[r] = true
		}
	}
	// CR-008: each signed role covered by at least one trusted issuer; the unverified subject is exempt.
	for _, r := range impersonationRoleKeys {
		if r == string(ports.CredentialRoleSubject) && subjectUnverified {
			continue
		}
		if !covered[r] {
			return formatValidationError(rbase+".trusted_issuers", r, "at least one trusted issuer whose signs_roles includes "+r, nil)
		}
	}
	return nil
}
