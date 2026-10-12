package fixtures

import (
	"encoding/base64"
	"os"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// DefaultOAuth2Config returns a minimal valid OAuth2 authorization server configuration.
// Uses safe defaults suitable for E2E testing:
// - In-memory storage backend
// - Upstream issuer: E2E_UPSTREAM_BASE_URL or http://127.0.0.1:19000
// - Upstream authorize endpoint: <upstream>/authorize
// - Upstream token endpoint: <upstream>/token
// - Timeout: 30 seconds
// - Public URL: http://localhost:8000
// - X-Remote-User authentication header
// - ThirdPartyOAuth2: JWE signing key configured for OAuth2 session management
// - StateTokenTTL: 10 minutes
// - PKCE verifier length: 32 bytes
func DefaultOAuth2Config() *ports.Config {
	upstreamURL := os.Getenv("E2E_UPSTREAM_BASE_URL")
	if upstreamURL == "" {
		upstreamURL = "http://127.0.0.1:19000"
	}

	return &ports.Config{
		Log: ports.LogConfig{
			Level:  ports.LogLevelInfo,
			Format: ports.LogFormatText,
		},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port:      8000,
				Bind:      "127.0.0.1",
				PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port:      14000,
				Bind:      "127.0.0.1",
				PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
			Shutdown: ports.ShutdownConfig{
				Timeout: 5 * time.Second,
			},
		},
		Storage: ports.StorageConfig{
			Backend: "memory",
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 5 * time.Second,
			},
		},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         upstreamURL,
				UpstreamAuthorizeEndpoint: upstreamURL + "/authorize",
				UpstreamTokenEndpoint:     upstreamURL + "/token",
				UpstreamTimeout:           30 * time.Second,
			},
			SupportedResponseTypes: []string{"code"},
			SupportedGrantTypes:    []string{"authorization_code", "refresh_token"},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey:      base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")),
			StateTokenTTL:      10 * time.Minute,
			PKCEVerifierLength: 32,
		},
		Encryption: ports.EncryptionConfig{
			Memory: &ports.MemoryConfig{
				RawKey: TestKEKMaterialDeterministic(),
			},
		},
		Approvals: ports.ApprovalsConfig{
			PendingTTL:         10 * time.Minute,
			SyncCoalesceWindow: 1 * time.Second,
			RateLimit: ports.ApprovalRateLimitConfig{
				MaxPendingPerPair:    50,
				MaxRequestsPerMinute: 10,
			},
		},
		Security:       ports.SecurityConfig{},
		RequestContext: ports.DefaultRequestContextConfig(),
	}
}

// OAuth2ConfigWithUpstream returns a config with a custom upstream OAuth2 server URL.
// Useful for pointing to a mock upstream server in tests.
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithUpstream(upstreamURL string) *ports.Config {
	config := DefaultOAuth2Config()
	config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = upstreamURL
	config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = upstreamURL + "/oauth/authorize"
	config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = upstreamURL + "/oauth/token"
	config.OAuth2AuthServer.Proxy.UpstreamTimeout = 2 * time.Second
	return config
}

// OAuth2ConfigWithTokenExchange returns a config with a custom upstream OAuth2 server URL
// and token exchange fully configured (CEL authorization + claim extraction).
// Used for tests that exercise RFC 8693 token exchange flows where the upstream
// must serve a discovery endpoint with a jwks_uri.
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithTokenExchange(upstreamURL string) *ports.Config {
	config := OAuth2ConfigWithUpstream(upstreamURL)
	config.TokenExchange = ports.TokenExchangeConfig{
		ClaimExtraction: ports.ClaimExtractionConfig{
			PrincipalExpression: "subject_token.sub",
			AgentIDExpression:   "subject_token.azp",
		},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL: ports.CELAuthorizationConfig{
				Expression:        "true",
				EvaluationTimeout: 2 * time.Second,
			},
		},
	}
	return config
}

// OAuth2ConfigWithTelemetry returns an OAuth2 config with tracing enabled for HTTP E2E tests.
func OAuth2ConfigWithTelemetry(upstreamURL string) *ports.Config {
	return EnableTelemetryTracing(OAuth2ConfigWithUpstream(upstreamURL))
}

// OAuth2ConfigWithTrustedProxy returns an OAuth2 config with trusted-proxy handling enabled.
func OAuth2ConfigWithTrustedProxy(upstreamURL, forwardedHeader string) *ports.Config {
	return EnableTrustedProxy(OAuth2ConfigWithUpstream(upstreamURL), forwardedHeader)
}

// EnableTelemetryTracing toggles the production telemetry gate on an existing config.
func EnableTelemetryTracing(config *ports.Config) *ports.Config {
	if config == nil {
		return nil
	}

	config.Telemetry = TelemetryEnabledConfig().Telemetry
	return config
}

// EnableTrustedProxy enables request-context trusted proxy handling on an existing config.
func EnableTrustedProxy(config *ports.Config, forwardedHeader string) *ports.Config {
	if config == nil {
		return nil
	}

	config.RequestContext.TrustedProxy.Enabled = true
	config.RequestContext.TrustedProxy.ForwardedHeader = forwardedHeader
	return config
}

// SetTraceResponseEnabled toggles request_context.trace.response_enabled on an existing config.
func SetTraceResponseEnabled(config *ports.Config, enabled bool) *ports.Config {
	if config == nil {
		return nil
	}

	config.RequestContext.Trace.ResponseEnabled = enabled
	return config
}

// OAuth2ConfigWithTimeout returns a config with a custom upstream timeout.
// Useful for testing timeout behavior.
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithTimeout(timeout time.Duration) *ports.Config {
	config := DefaultOAuth2Config()
	config.OAuth2AuthServer.Proxy.UpstreamTimeout = timeout
	return config
}

// OAuth2ConfigWithLogLevel returns a config with a custom log level.
// Useful for debugging E2E test failures.
// LogLevel: specified by caller (e.g., "debug", "info", "warn", "error")
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithLogLevel(level string) *ports.Config {
	config := DefaultOAuth2Config()
	config.Log.Level = ports.LogLevel(level)
	return config
}

// OAuth2ConfigWithPublicURL returns a config with a custom public URL.
// Useful for testing metadata discovery with different base URLs.
// PublicURL: specified by caller
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithPublicURL(publicURL string) *ports.Config {
	config := DefaultOAuth2Config()
	config.Server.EndUser.PublicURL = publicURL
	return config
}

// OAuth2ConfigWithStorage returns a config with custom storage backend.
// Useful for testing with different storage backends (not typically used in E2E, but available).
// Backend: specified by caller (e.g., "memory" or "postgres")
// All other settings match DefaultOAuth2Config().
func OAuth2ConfigWithStorage(backend string) *ports.Config {
	config := DefaultOAuth2Config()
	config.Storage.Backend = backend
	return config
}

// TokenExchangeConfigWithCELExpression returns a config with a custom CEL authorization expression
// pointed at the given upstream OAuth2 server.
// Useful for testing different CEL authorization policies.
// Expression: specified by caller (e.g., "claims.iss == 'https://auth.example.com'")
func TokenExchangeConfigWithCELExpression(upstreamURL, expression string) *ports.Config {
	config := OAuth2ConfigWithTokenExchange(upstreamURL)
	config.TokenExchange.Authorization.CEL.Expression = expression
	return config
}

// TokenExchangeConfigWithInvalidCELSyntax returns a config with an invalid CEL expression.
// Used to test that invalid CEL syntax is caught at startup (per FR-017).
// InvalidExpression: A malformed CEL expression that should fail compilation.
// This is used in US4-S4 to test startup validation.
// Note: CEL compilation fails before discovery is attempted, so no real upstream is needed.
func TokenExchangeConfigWithInvalidCELSyntax(invalidExpression string) *ports.Config {
	config := DefaultOAuth2Config()
	config.TokenExchange = ports.TokenExchangeConfig{
		ClaimExtraction: ports.ClaimExtractionConfig{
			PrincipalExpression: "subject_token.sub",
			AgentIDExpression:   "subject_token.azp",
		},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL: ports.CELAuthorizationConfig{
				Expression:        invalidExpression,
				EvaluationTimeout: 2 * time.Second,
			},
		},
	}
	return config
}

// TelemetryEnabledConfig returns a config with telemetry tracing enabled.
// Intended for E2E tests that use BuildAppWithTracerProvider() — the OTLP exporter
// endpoint is irrelevant because WithTracerProvider() bypasses NewProvider().
// Having Telemetry.Enabled=true and Traces.Enabled=true ensures the otelchi
// middleware gate in SetupEnduserRoutes/SetupAdminRoutes is satisfied.
func TelemetryEnabledConfig() *ports.Config {
	cfg := DefaultOAuth2Config()
	cfg.Telemetry = ports.TelemetryConfig{
		Enabled:            true,
		ServiceName:        "test-broker",
		ResourceAttributes: map[string]string{},
		Traces: ports.TracesConfig{
			Enabled:      true,
			SamplingRate: 1.0,
			Propagators:  []string{"ottrace", "b3multi", "baggage"},
		},
		Metrics: ports.MetricsConfig{
			Enabled:        false, // no runtime metrics in tests
			ExportInterval: 30 * time.Second,
		},
		Logs: ports.LogsConfig{
			Enabled: false, // no OTLP log export in tests
		},
		Exporter: ports.OTLPExporterConfig{
			Protocol: "grpc",
			Endpoint: "localhost:4317", // not used when TP is injected via WithTracerProvider
			Headers:  map[string]string{},
			Timeout:  5 * time.Second,
			Insecure: true,
		},
	}
	return cfg
}

// TelemetryGRPCConfig returns a config with telemetry enabled using the gRPC OTLP protocol.
// The endpoint (localhost:4317) is intentionally unreachable; the OTel SDK buffers telemetry
// and does not fail at startup. Suitable for smoke-testing the production NewProvider() path.
// Uses a short exporter timeout to keep test execution fast.
func TelemetryGRPCConfig() *ports.Config {
	cfg := DefaultOAuth2Config()
	cfg.Telemetry = ports.TelemetryConfig{
		Enabled:            true,
		ServiceName:        "test-broker",
		ResourceAttributes: map[string]string{},
		Traces: ports.TracesConfig{
			Enabled:      true,
			SamplingRate: 1.0,
			Propagators:  []string{"ottrace", "b3multi", "baggage"},
		},
		Metrics: ports.MetricsConfig{
			Enabled:        false,
			ExportInterval: 30 * time.Second,
		},
		Logs: ports.LogsConfig{
			Enabled: false,
		},
		Exporter: ports.OTLPExporterConfig{
			Protocol: "grpc",
			Endpoint: "localhost:4317",
			Headers:  map[string]string{},
			Timeout:  500 * time.Millisecond,
			Insecure: true,
		},
	}
	return cfg
}

// TelemetryHTTPConfig returns a config with telemetry enabled using the HTTP OTLP protocol.
// The endpoint (http://localhost:4318) is intentionally unreachable; the OTel SDK buffers
// telemetry and does not fail at startup. Suitable for smoke-testing the production
// NewProvider() path. Uses a short exporter timeout to keep test execution fast.
//
// The endpoint is a full http:// URL as required by the HTTP OTLP exporter (WithEndpointURL).
func TelemetryHTTPConfig() *ports.Config {
	cfg := DefaultOAuth2Config()
	cfg.Telemetry = ports.TelemetryConfig{
		Enabled:            true,
		ServiceName:        "test-broker",
		ResourceAttributes: map[string]string{},
		Traces: ports.TracesConfig{
			Enabled:      true,
			SamplingRate: 1.0,
			Propagators:  []string{"ottrace", "b3multi", "baggage"},
		},
		Metrics: ports.MetricsConfig{
			Enabled:        false,
			ExportInterval: 30 * time.Second,
		},
		Logs: ports.LogsConfig{
			Enabled: false,
		},
		Exporter: ports.OTLPExporterConfig{
			Protocol: "http",
			Endpoint: "http://localhost:4318", // full URL required for HTTP OTLP exporter
			Headers:  map[string]string{},
			Timeout:  500 * time.Millisecond,
			Insecure: true,
		},
	}
	return cfg
}

// TokenExchangeConfigWithClaimExtraction returns a config with custom claim extraction expressions.
// Useful for testing different claim mapping strategies.
// PrincipalExpr: CEL expression to extract principal (e.g., "subject_token.preferred_username")
// AgentExpr: CEL expression to extract agent ID (e.g., "subject_token.client_id")
func TokenExchangeConfigWithClaimExtraction(upstreamURL, principalExpr, agentExpr string) *ports.Config {
	config := OAuth2ConfigWithTokenExchange(upstreamURL)
	config.TokenExchange.ClaimExtraction.PrincipalExpression = principalExpr
	config.TokenExchange.ClaimExtraction.AgentIDExpression = agentExpr
	return config
}

// ============ JWT Pre-Authentication Config Fixtures ============

// SignedJWTConfig returns a config with signed JWT pre-authentication enabled.
// The jwksURL must point to a JWKS endpoint serving the public key matching
// the key used to sign test JWTs (typically from MockJWKSServer.JWKSURL()).
//
// Configuration:
//   - JWT verification: jwks (signed, requires JWKS URI)
//   - JWT header: Authorization (Bearer prefix auto-stripped)
//   - Principal CEL expression: claims.sub
//   - Display name CEL expression: claims.name
//   - Email CEL expression: claims.email
//   - Picture URL CEL expression: claims.picture
//   - Expected audience: agentic-identity-broker
//   - Expected issuer: https://auth.example.com
//   - Plain header fallback: X-Remote-User
//
// All other settings match DefaultOAuth2Config().
func SignedJWTConfig(jwksURL string) *ports.Config {
	config := DefaultOAuth2Config()
	// Allow HTTP JWKS URIs in tests (mock servers use HTTP)
	config.Security.SkipThirdpartyHTTPSValidation = true
	config.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		HeaderName:       "Authorization",
		Verification:     "jwks",
		JWKSURI:          jwksURL,
		ExpectedAudience: "agentic-identity-broker",
		ExpectedIssuer:   "https://auth.example.com",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression:   "claims.sub",
			DisplayNameExpression: "claims.name",
			EmailExpression:       "claims.email",
			PictureURLExpression:  "claims.picture",
		},
	}
	return config
}

// SignedJWTConfigMinimal returns a config with signed JWT pre-auth using only principal extraction.
// No display name, email, or picture URL expressions configured.
// Useful for testing fallback behavior when optional profile expressions are absent.
func SignedJWTConfigMinimal(jwksURL string) *ports.Config {
	config := DefaultOAuth2Config()
	// Allow HTTP JWKS URIs in tests (mock servers use HTTP)
	config.Security.SkipThirdpartyHTTPSValidation = true
	config.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		HeaderName:       "Authorization",
		Verification:     "jwks",
		JWKSURI:          jwksURL,
		ExpectedAudience: "agentic-identity-broker",
		ExpectedIssuer:   "https://auth.example.com",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	}
	return config
}

// UnsignedJWTConfig returns a config with unsigned JWT pre-authentication enabled.
// Used for testing service mesh environments where the upstream injects unsigned JWTs.
//
// Configuration:
//   - JWT verification: none (unsigned, no JWKS URI)
//   - JWT header: X-JWT-Claims (custom header, raw JWT value)
//   - Principal CEL expression: claims.sub
//   - Display name CEL expression: claims.preferred_username
//   - Email CEL expression: claims.email
//   - No audience/issuer validation (trusted upstream)
//   - Plain header fallback: X-Remote-User
//
// All other settings match DefaultOAuth2Config().
func UnsignedJWTConfig() *ports.Config {
	config := DefaultOAuth2Config()
	config.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		HeaderName:   "X-JWT-Claims",
		Verification: "none",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression:   "claims.sub",
			DisplayNameExpression: "claims.preferred_username",
			EmailExpression:       "claims.email",
		},
	}
	return config
}

// NoJWTConfig returns a config with no JWT pre-authentication (plain header only).
// This is the default/backward-compatible configuration where only the X-Remote-User
// header is used for principal extraction.
//
// All other settings match DefaultOAuth2Config().
func NoJWTConfig() *ports.Config {
	// DefaultOAuth2Config already has no JWT config (JWT field is nil)
	return DefaultOAuth2Config()
}

// MutuallyExclusiveJWTConfig returns a config with both verification: none and jwks_uri set.
// This is an INVALID configuration that should cause startup failure (FR-003a).
// Used to test mutual exclusivity validation at startup.
func MutuallyExclusiveJWTConfig(jwksURL string) *ports.Config {
	config := DefaultOAuth2Config()
	config.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		HeaderName:   "Authorization",
		Verification: "none",
		JWKSURI:      jwksURL,
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	}
	return config
}

// SignedJWTConfigWithAudience returns a signed JWT config with a custom expected audience.
// Useful for testing audience validation scenarios.
func SignedJWTConfigWithAudience(jwksURL, audience string) *ports.Config {
	config := SignedJWTConfig(jwksURL)
	config.Server.EndUser.Authentication.JWT.ExpectedAudience = audience
	return config
}

// SignedJWTConfigWithIssuer returns a signed JWT config with a custom expected issuer.
// Useful for testing issuer validation scenarios.
func SignedJWTConfigWithIssuer(jwksURL, issuer string) *ports.Config {
	config := SignedJWTConfig(jwksURL)
	config.Server.EndUser.Authentication.JWT.ExpectedIssuer = issuer
	return config
}

// HybridConfig returns a config for hybrid mode E2E testing.
// Both proxy and local sections are required by hybrid mode validation.
func HybridConfig(upstreamURL string) *ports.Config {
	config := DefaultOAuth2Config()
	config.OAuth2AuthServer.Mode = "hybrid"
	config.OAuth2AuthServer.Proxy.UpstreamIssuerURI = upstreamURL
	config.OAuth2AuthServer.Proxy.UpstreamAuthorizeEndpoint = upstreamURL + "/oauth/authorize"
	config.OAuth2AuthServer.Proxy.UpstreamTokenEndpoint = upstreamURL + "/oauth/token"
	config.OAuth2AuthServer.Proxy.UpstreamTimeout = 2 * time.Second
	config.OAuth2AuthServer.Local.TokenTTL = time.Hour
	return config
}

// HybridConfigWithCIMD returns a hybrid mode config with CIMD support enabled.
func HybridConfigWithCIMD(upstreamURL string) *ports.Config {
	config := HybridConfig(upstreamURL)
	config.OAuth2AuthServer.CIMD = ports.CIMDConfig{
		Enabled:          true,
		FetchTimeout:     5 * time.Second,
		MaxResponseBytes: 5120,
		Cache: ports.CIMDCacheConfig{
			MinTTL:     60 * time.Second,
			MaxTTL:     1 * time.Hour,
			MaxEntries: 1000,
		},
	}
	return config
}

// LocalConfig returns a config for local mode E2E testing.
// Uses in-memory storage and encryption, with a test issuer URI.
// Upstream OAuth2 fields are cleared (not needed in local mode).
func LocalConfig() *ports.Config {
	config := DefaultOAuth2Config()
	config.OAuth2AuthServer.Mode = "local"
	config.OAuth2AuthServer.Local.TokenTTL = time.Hour
	config.OAuth2AuthServer.Local.TokenClaimsExpression = ""
	config.OAuth2AuthServer.Proxy = ports.ProxyModeConfig{}
	return config
}

// LocalConfigWithCEL returns a config for local mode with custom JWT claims.
func LocalConfigWithCEL(celExpr string) *ports.Config {
	config := LocalConfig()
	config.OAuth2AuthServer.Local.TokenClaimsExpression = celExpr
	return config
}

// OAuth2ConfigWithCIMD returns a config for local mode with CIMD support enabled.
// The CIMD fetcher is wired in the builder; the config only enables the feature gate.
// The upstreamURL parameter is accepted for compatibility but not used (CIMD is local mode only).
func OAuth2ConfigWithCIMD(_ string) *ports.Config {
	config := LocalConfig()
	config.OAuth2AuthServer.CIMD = ports.CIMDConfig{
		Enabled:          true,
		FetchTimeout:     5 * time.Second,
		MaxResponseBytes: 5120,
		Cache: ports.CIMDCacheConfig{
			MinTTL:     60 * time.Second,
			MaxTTL:     1 * time.Hour,
			MaxEntries: 1000,
		},
	}
	return config
}
