// Package fixtures provides test configuration objects for ExtProc E2E tests.
package fixtures

import (
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// DefaultConfig returns a valid ExtProc configuration suitable for E2E tests.
// OAuth2 endpoints are placeholders — bootstrap.TestEnvironment.Start() will
// override them with the mock server URLs.
func DefaultConfig() *extprocconfig.Config {
	return &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{
			Bind:                 "127.0.0.1",
			Port:                 50051,
			MaxConcurrentStreams: 100,
		},
		OAuth2: extprocconfig.OAuth2Config{ // #nosec G101 -- test-only placeholders and fake OAuth client credentials.
			TokenEndpoint:             "http://placeholder/oauth2/token",
			Issuer:                    "http://placeholder",
			ClientID:                  "test-extproc-client",
			ClientSecret:              "test-client-secret",
			ClientCredentialsEndpoint: "http://placeholder/oauth/token",
			ClientAssertionType:       "access_token",
			ExchangeTimeout:           5 * time.Second,
			TLS: extprocconfig.TLSConfig{
				InsecureSkipVerify: false,
				CaBundlePath:       "",
				AllowHTTP:          false, // Start() sets this to true
			},
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		Log: extprocconfig.LogConfig{
			Level:  "debug",
			Format: "text",
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
		Telemetry: extprocconfig.TelemetryConfig{
			Exporter: extprocconfig.OTLPExporterConfig{
				Timeout: 10 * time.Second,
			},
		},
	}
}

// ShortCacheTTLConfig returns a config with very short cache TTL for expiry testing.
// The DefaultTTL is set to 150ms to allow tests to observe cache expiry within test timeouts.
func ShortCacheTTLConfig() *extprocconfig.Config {
	cfg := DefaultConfig()
	cfg.Cache.DefaultTTL = ShortTTL
	cfg.Cache.MaxTTL = 5 * ShortTTL
	return cfg
}

// InvalidConfig returns a config missing required fields (used for startup validation tests).
// oauth2.token_endpoint is empty, which should trigger a validation error on startup.
func InvalidConfig() *extprocconfig.Config {
	return &extprocconfig.Config{
		GRPC: extprocconfig.GRPCConfig{
			Bind: "127.0.0.1",
			Port: 50051,
		},
		OAuth2: extprocconfig.OAuth2Config{
			TokenEndpoint: "", // Missing required field — should fail validation
			ClientID:      "", // Missing required field
			ClientSecret:  "", // Missing required field
		},
		Cache: extprocconfig.CacheConfig{
			DefaultTTL: 5 * time.Minute,
			MaxTTL:     1 * time.Hour,
		},
		CircuitBreaker: extprocconfig.CircuitBreakerConfig{
			MaxFailures:  5,
			ResetTimeout: 30 * time.Second,
		},
	}
}

// ConfigWithInvalidPort returns a config with an invalid gRPC port (0).
func ConfigWithInvalidPort() *extprocconfig.Config {
	cfg := DefaultConfig()
	cfg.GRPC.Port = 0 // Invalid port — should fail validation
	return cfg
}

// ConfigWithClientCredentialsScopes returns a config with custom OAuth2 scopes
// for the client_credentials grant used to obtain the client assertion.
func ConfigWithClientCredentialsScopes(scopes []string) *extprocconfig.Config {
	cfg := DefaultConfig()
	cfg.OAuth2.ClientCredentialsScopes = scopes
	return cfg
}
