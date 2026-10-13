package config

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// validTestConfig returns a valid Config for testing with all required fields set.
func validTestConfig() *ports.Config {
	return &ports.Config{
		Log: ports.LogConfig{
			Level:  ports.LogLevelInfo,
			Format: ports.LogFormatText,
		},
		Server: ports.ServerConfig{
			EndUser: ports.ServerInstanceConfig{
				Port:      8000,
				Bind:      "::",
				PublicURL: "http://localhost:8000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
			Admin: ports.ServerInstanceConfig{
				Port:      14000,
				Bind:      "::",
				PublicURL: "http://localhost:14000",
				Authentication: ports.AuthenticationConfig{
					Preauth: ports.PreauthConfig{
						PrincipalHeaderName: "X-Remote-User",
					},
				},
			},
		},
		Storage: ports.StorageConfig{
			Backend: "memory",
			Timeouts: ports.StorageTimeouts{
				Read:  5 * time.Second,
				Write: 10 * time.Second,
			},
		},
		ThirdPartyOAuth2: ports.ThirdPartyOAuth2Config{
			JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
		},
		Encryption: ports.EncryptionConfig{
			Memory: &ports.MemoryConfig{
				RawKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
			},
		},
		OAuth2AuthServer: ports.OAuth2AuthServerConfig{
			Mode: "proxy",
			Proxy: ports.ProxyModeConfig{
				UpstreamIssuerURI:         "https://auth.example.com",
				UpstreamAuthorizeEndpoint: "https://auth.example.com/authorize",
				UpstreamTokenEndpoint:     "https://auth.example.com/token",
			},
		},
		RequestContext: ports.RequestContextConfig{
			TrustedProxy: ports.RequestContextTrustedProxyConfig{
				Enabled:         false,
				ForwardedHeader: "X-Forwarded-For",
			},
			Trace: ports.RequestContextTraceConfig{
				ResponseEnabled: true,
			},
		},
		Security: ports.SecurityConfig{},
	}
}

func TestValidateLogLevel(t *testing.T) {
	tests := []struct {
		name    string
		level   ports.LogLevel
		wantErr bool
	}{
		{"valid debug", ports.LogLevelDebug, false},
		{"valid info", ports.LogLevelInfo, false},
		{"valid warn", ports.LogLevelWarn, false},
		{"valid error", ports.LogLevelError, false},
		{"invalid verbose", ports.LogLevel("verbose"), true},
		{"invalid empty", ports.LogLevel(""), true},
		{"invalid trace", ports.LogLevel("trace"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogLevel(tt.level)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogLevel() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateLogFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  ports.LogFormat
		wantErr bool
	}{
		{"valid text", ports.LogFormatText, false},
		{"valid json", ports.LogFormatJSON, false},
		{"invalid xml", ports.LogFormat("xml"), true},
		{"invalid empty", ports.LogFormat(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLogFormat(tt.format)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateLogFormat() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ports.Config
		wantErr bool
	}{
		{
			name:    "valid config",
			cfg:     validTestConfig(),
			wantErr: false,
		},
		{
			name: "invalid log level",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Log.Level = ports.LogLevel("invalid")
				return cfg
			}(),
			wantErr: true,
		},
		{
			name: "invalid log format",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Log.Format = ports.LogFormat("invalid")
				return cfg
			}(),
			wantErr: true,
		},
		{
			name: "missing enduser principal header name without JWT fails",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName = ""
				return cfg
			}(),
			wantErr: true,
		},
		{
			name: "missing admin principal header name without JWT fails",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName = ""
				return cfg
			}(),
			wantErr: true,
		},
		{
			name: "missing enduser principal header name with JWT configured is valid",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName = ""
				cfg.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
					Verification: "none",
					ClaimExtraction: ports.JWTClaimExtractionConfig{
						PrincipalExpression: "claims.sub",
					},
				}
				return cfg
			}(),
			wantErr: false,
		},
		{
			name: "custom principal header names pass validation",
			cfg: func() *ports.Config {
				cfg := validTestConfig()
				cfg.Server.EndUser.Authentication.Preauth.PrincipalHeaderName = "X-Authenticated-User"
				cfg.Server.Admin.Authentication.Preauth.PrincipalHeaderName = "X-Admin-User"
				return cfg
			}(),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify error is ConfigError type when expected
			if err != nil {
				if _, ok := err.(*config.ConfigError); !ok {
					t.Errorf("Validate() error type = %T, want *config.ConfigError", err)
				}
			}
		})
	}
}

func TestValidateAdminPublicURL(t *testing.T) {
	tests := []struct {
		name      string
		publicURL string
		wantValid bool
	}{
		{name: "blank URL", publicURL: ""},
		{name: "malformed URL", publicURL: "https://[::1"},
		{name: "missing host", publicURL: "https:///admin"},
		{name: "port without hostname", publicURL: "https://:14000"},
		{name: "empty explicit DNS port", publicURL: "https://admin.example.com:"},
		{name: "empty explicit IPv6 port with path", publicURL: "https://[2001:db8::1]:/admin"},
		{name: "userinfo", publicURL: "https://admin:secret@admin.example.com"},
		{name: "valid HTTPS DNS without port", publicURL: "https://admin.example.com", wantValid: true},
		{name: "valid HTTPS IPv6 without port", publicURL: "https://[2001:db8::1]", wantValid: true},
		{name: "valid HTTPS DNS", publicURL: "https://admin.example.com:14000", wantValid: true},
		{name: "valid HTTPS IPv6", publicURL: "https://[2001:db8::1]:14000", wantValid: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validTestConfig()
			cfg.Server.Admin.PublicURL = tt.publicURL
			err := Validate(cfg)
			if tt.wantValid {
				if err != nil {
					t.Fatalf("Validate() returned error for admin URL %q: %v", tt.publicURL, err)
				}
				return
			}

			var configErr *config.ConfigError
			if !errors.As(err, &configErr) {
				t.Fatalf("Validate() error = %v, want *config.ConfigError", err)
			}
			if configErr.Field != "server.admin.public_url" {
				t.Errorf("Validate() error field = %q, want server.admin.public_url", configErr.Field)
			}
		})
	}
}

func TestValidateTokenExchangeConfig(t *testing.T) {
	tests := []struct {
		name     string
		config   ports.ClientAssertionTrustConfig
		security ports.SecurityConfig
		errMsg   string
	}{
		{
			name:   "rejects HTTP issuer when HTTPS validation is enabled",
			config: ports.ClientAssertionTrustConfig{IssuerURI: "http://idp.example"},
			errMsg: "HTTPS URL",
		},
		{
			name:     "accepts HTTP issuer when HTTPS validation is disabled",
			config:   ports.ClientAssertionTrustConfig{IssuerURI: "http://idp.example"},
			security: ports.SecurityConfig{SkipThirdpartyHTTPSValidation: true},
		},
		{
			name:   "rejects negative JWKS minimum refresh",
			config: ports.ClientAssertionTrustConfig{JWKSMinRefresh: -time.Minute},
			errMsg: "non-negative duration",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTokenExchangeConfig(&ports.TokenExchangeConfig{ClientAssertion: tt.config}, &tt.security)
			if tt.errMsg == "" {
				if err != nil {
					t.Fatalf("validateTokenExchangeConfig() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.errMsg) {
				t.Fatalf("validateTokenExchangeConfig() error = %v, want %q", err, tt.errMsg)
			}
		})
	}
}

// TestValidateJWTConfig tests JWT pre-authentication configuration validation.
// Covers mutual exclusivity checks, required fields, defaults application,
// and CEL expression validation per FR-003a and SR-004.
func TestValidateJWTConfig(t *testing.T) {
	tests := []struct {
		name     string
		jwt      *ports.JWTConfig
		security *ports.SecurityConfig
		wantErr  bool
		errMsg   string
	}{
		{
			name: "valid signed JWT config with JWKS URI",
			jwt: &ports.JWTConfig{
				HeaderName:   "Authorization",
				Verification: "jwks",
				JWKSURI:      "https://auth.example.com/.well-known/jwks.json",
				ClaimExtraction: ports.JWTClaimExtractionConfig{
					PrincipalExpression: "claims.sub",
				},
			},
			security: nil,
			wantErr:  false,
		},
		{
			name: "valid unsigned JWT config",
			jwt: &ports.JWTConfig{
				HeaderName:   "X-JWT-Claims",
				Verification: "none",
				ClaimExtraction: ports.JWTClaimExtractionConfig{
					PrincipalExpression: "claims.sub",
				},
			},
			security: nil,
			wantErr:  false,
		},
		{
			name: "defaults applied when fields are empty",
			jwt: &ports.JWTConfig{
				JWKSURI: "https://auth.example.com/.well-known/jwks.json",
			},
			security: nil,
			wantErr:  false,
		},
		{
			name: "mutual exclusivity: verification none + jwks_uri",
			jwt: &ports.JWTConfig{
				Verification: "none",
				JWKSURI:      "https://auth.example.com/.well-known/jwks.json",
			},
			security: nil,
			wantErr:  true,
			errMsg:   "mutually exclusive",
		},
		{
			name: "missing jwks_uri when verification is jwks",
			jwt: &ports.JWTConfig{
				Verification: "jwks",
				JWKSURI:      "",
			},
			security: nil,
			wantErr:  true,
			errMsg:   "jwks_uri",
		},
		{
			name: "invalid verification mode",
			jwt: &ports.JWTConfig{
				Verification: "custom",
			},
			security: nil,
			wantErr:  true,
			errMsg:   "'jwks' or 'none'",
		},
		{
			name: "HTTPS required for JWKS URI by default",
			jwt: &ports.JWTConfig{
				Verification: "jwks",
				JWKSURI:      "http://auth.example.com/.well-known/jwks.json",
			},
			security: nil,
			wantErr:  true,
			errMsg:   "HTTPS",
		},
		{
			name: "HTTP allowed when skip_thirdparty_https_validation is true",
			jwt: &ports.JWTConfig{
				Verification: "jwks",
				JWKSURI:      "http://auth.example.com/.well-known/jwks.json",
			},
			security: &ports.SecurityConfig{
				SkipThirdpartyHTTPSValidation: true,
			},
			wantErr: false,
		},
		{
			name: "invalid JWKS URI scheme (ftp)",
			jwt: &ports.JWTConfig{
				Verification: "jwks",
				JWKSURI:      "ftp://auth.example.com/jwks.json",
			},
			security: nil,
			wantErr:  true,
			errMsg:   "HTTP or HTTPS URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateJWTConfig(tt.jwt, "server.enduser.authentication.jwt", tt.security)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateJWTConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.errMsg != "" {
				errStr := err.Error()
				if !strings.Contains(errStr, tt.errMsg) {
					t.Errorf("validateJWTConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			}
		})
	}
}

// TestValidateJWTConfig_DefaultsApplied verifies that defaults are correctly
// set when JWT config fields are empty.
func TestValidateJWTConfig_DefaultsApplied(t *testing.T) {
	jwt := &ports.JWTConfig{
		JWKSURI: "https://auth.example.com/.well-known/jwks.json",
	}

	err := validateJWTConfig(jwt, "server.enduser.authentication.jwt", nil)
	if err != nil {
		t.Fatalf("validateJWTConfig() unexpected error: %v", err)
	}

	if jwt.HeaderName != "Authorization" {
		t.Errorf("expected HeaderName default 'Authorization', got %q", jwt.HeaderName)
	}
	if jwt.Verification != "jwks" {
		t.Errorf("expected Verification default 'jwks', got %q", jwt.Verification)
	}
	if jwt.ClaimExtraction.PrincipalExpression != "claims.sub" {
		t.Errorf("expected PrincipalExpression default 'claims.sub', got %q",
			jwt.ClaimExtraction.PrincipalExpression)
	}
}

// TestValidate_WithJWTConfig tests that full config validation includes JWT
// validation when JWT config is present.
func TestValidate_WithJWTConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.Server.EndUser.Authentication.JWT = &ports.JWTConfig{
		Verification: "none",
		JWKSURI:      "https://should-not-be-set.example.com",
	}

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for mutual exclusivity, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("expected 'mutually exclusive' in error, got: %v", err)
	}
}

func TestValidateTelemetryConfig(t *testing.T) {
	// valid base config used as the starting point for each test case;
	// only the field under test is changed so that exactly one rule fires at a time.
	validEnabled := func() ports.TelemetryConfig {
		cfg := ports.DefaultTelemetryConfig()
		cfg.Enabled = true
		cfg.Exporter.Endpoint = "otel-collector:4317"
		cfg.Exporter.Protocol = "grpc"
		cfg.Exporter.Timeout = 10 * time.Second
		cfg.Traces.SamplingRate = 1.0
		cfg.Metrics.Enabled = true
		cfg.Metrics.ExportInterval = 30 * time.Second
		return cfg
	}

	tests := []struct {
		name            string
		cfg             ports.TelemetryConfig
		wantErr         bool
		wantField       string
		wantCompression string // non-empty: assert cfg.Exporter.Compression after call
	}{
		{
			// Rule 1: skip validation when disabled — even an empty endpoint must not error
			name: "skip-when-disabled",
			cfg: func() ports.TelemetryConfig {
				cfg := ports.DefaultTelemetryConfig()
				cfg.Enabled = false
				cfg.Exporter.Endpoint = ""
				return cfg
			}(),
			wantErr: false,
		},
		{
			// Rule 2: endpoint is required when enabled
			name: "endpoint required when enabled",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Endpoint = ""
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.exporter.endpoint",
		},
		{
			// Rule 3: endpoint must be a valid format (no percent-encoded garbage)
			name: "endpoint invalid format",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Endpoint = "not-valid-%%url"
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.exporter.endpoint",
		},
		{
			// Rule 4: protocol must be grpc or http
			name: "protocol invalid",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Protocol = "jaeger"
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.exporter.protocol",
		},
		{
			// Rule 5: sampling rate must be 0.0-1.0
			name: "sampling rate out of range high",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Traces.SamplingRate = 1.5
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.traces.sampling_rate",
		},
		{
			// Rule 6: export interval must be positive when metrics enabled
			name: "export interval zero when metrics enabled",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Metrics.Enabled = true
				cfg.Metrics.ExportInterval = 0
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.metrics.export_interval",
		},
		{
			// Rule 7: exporter timeout must be positive
			name: "exporter timeout zero",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Timeout = 0
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.exporter.timeout",
		},
		{
			// Rule 8: service_name must be non-empty
			name: "empty service_name",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.ServiceName = ""
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.service_name",
		},
		{
			// Rule 8 (whitespace variant): whitespace-only service_name treated as empty
			name: "whitespace-only service_name",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.ServiceName = "   "
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.service_name",
		},
		{
			// Happy path: fully valid enabled config
			name:    "valid enabled config",
			cfg:     validEnabled(),
			wantErr: false,
		},
		{
			// Happy path: http protocol with a proper URL endpoint
			name: "valid http protocol with URL endpoint",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Protocol = "http"
				cfg.Exporter.Endpoint = "http://otel-collector:4318"
				return cfg
			}(),
			wantErr: false,
		},
		{
			// Normalization: empty string compression is normalized to "none" — no error
			name: "empty compression normalized to none",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Compression = ""
				return cfg
			}(),
			wantErr:         false,
			wantCompression: "none",
		},
		{
			// Compression value outside the allowed set is rejected
			name: "invalid compression value",
			cfg: func() ports.TelemetryConfig {
				cfg := validEnabled()
				cfg.Exporter.Compression = "br"
				return cfg
			}(),
			wantErr:   true,
			wantField: "telemetry.exporter.compression",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTelemetryConfig(&tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateTelemetryConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil && tt.wantField != "" {
				configErr, ok := err.(*config.ConfigError)
				if !ok {
					t.Errorf("validateTelemetryConfig() error type = %T, want *config.ConfigError", err)
					return
				}
				if configErr.Field != tt.wantField {
					t.Errorf("validateTelemetryConfig() error field = %q, want %q", configErr.Field, tt.wantField)
				}
			}

			if tt.wantCompression != "" && tt.cfg.Exporter.Compression != tt.wantCompression {
				t.Errorf("validateTelemetryConfig() compression = %q, want %q", tt.cfg.Exporter.Compression, tt.wantCompression)
			}
		})
	}
}

func TestValidateThirdPartyOAuth2Config(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *ports.ThirdPartyOAuth2Config
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid key",
			cfg: &ports.ThirdPartyOAuth2Config{
				JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef")),
			},
			wantErr: false,
		},
		{
			name: "empty key",
			cfg: &ports.ThirdPartyOAuth2Config{
				JWESigningKey: "",
			},
			wantErr: true,
			errMsg:  "third_party_oauth2.jwe_signing_key",
		},
		{
			name: "invalid base64",
			cfg: &ports.ThirdPartyOAuth2Config{
				JWESigningKey: "not-valid-base64!@#$%",
			},
			wantErr: true,
			errMsg:  "valid base64-encoded string",
		},
		{
			name: "key too short - 16 bytes",
			cfg: &ports.ThirdPartyOAuth2Config{
				JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef")),
			},
			wantErr: true,
			errMsg:  "exactly 32 bytes when decoded",
		},
		{
			name: "key too long - 64 bytes",
			cfg: &ports.ThirdPartyOAuth2Config{
				JWESigningKey: base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")),
			},
			wantErr: true,
			errMsg:  "exactly 32 bytes when decoded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateThirdPartyOAuth2Config(tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateThirdPartyOAuth2Config() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Check error message contains expected text
			if err != nil && tt.errMsg != "" {
				errStr := err.Error()
				if !strings.Contains(errStr, tt.errMsg) {
					t.Errorf("validateThirdPartyOAuth2Config() error = %v, want error containing %q", err, tt.errMsg)
				}
			}

			// Verify error is ConfigError type when expected
			if err != nil {
				if _, ok := err.(*config.ConfigError); !ok {
					t.Errorf("validateThirdPartyOAuth2Config() error type = %T, want *config.ConfigError", err)
				}
			}
		})
	}
}

func TestValidateCORSConfig(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ports.CORSConfig
		prefix  string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "empty config (secure default) passes",
			cfg:     ports.CORSConfig{},
			prefix:  "server.enduser.cors",
			wantErr: false,
		},
		{
			name:    "localhost origin passes",
			cfg:     ports.CORSConfig{AllowedOrigins: []string{"http://localhost:3000"}},
			prefix:  "server.enduser.cors",
			wantErr: false,
		},
		{
			name: "specific origin with full options passes",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://app.example.com"},
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{"Content-Type"},
				MaxAge:         3600,
			},
			prefix:  "server.enduser.cors",
			wantErr: false,
		},
		{
			name:    "wildcard origin fails",
			cfg:     ports.CORSConfig{AllowedOrigins: []string{"*"}},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "server.enduser.cors.allowed_origins[0]",
		},
		{
			name:    "HTTPS wildcard origin fails",
			cfg:     ports.CORSConfig{AllowedOrigins: []string{"https://*"}},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "server.enduser.cors.allowed_origins[0]",
		},
		{
			name:    "subdomain wildcard mixed with specific origin fails for admin server",
			cfg:     ports.CORSConfig{AllowedOrigins: []string{"https://app.example.com", "https://*.example.com"}},
			prefix:  "server.admin.cors",
			wantErr: true,
			errMsg:  "server.admin.cors.allowed_origins[1]",
		},
		{
			name:    "wildcard mixed with specific origin fails for admin server",
			cfg:     ports.CORSConfig{AllowedOrigins: []string{"https://app.example.com", "*"}},
			prefix:  "server.admin.cors",
			wantErr: true,
			errMsg:  "server.admin.cors.allowed_origins[1]",
		},
		{
			name: "empty string in allowed_origins fails",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://valid.com", ""},
			},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "allowed_origins",
		},
		{
			name: "empty string in allowed_methods fails",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://valid.com"},
				AllowedMethods: []string{"GET", ""},
			},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "allowed_methods",
		},
		{
			name: "empty string in allowed_headers fails",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://valid.com"},
				AllowedHeaders: []string{"", "Content-Type"},
			},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "allowed_headers",
		},
		{
			name: "negative max_age fails",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://valid.com"},
				MaxAge:         -1,
			},
			prefix:  "server.enduser.cors",
			wantErr: true,
			errMsg:  "max_age",
		},
		{
			name: "zero max_age passes (browser default)",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{"https://valid.com"},
				MaxAge:         0,
			},
			prefix:  "server.enduser.cors",
			wantErr: false,
		},
		{
			name: "error message contains server-specific prefix (admin server)",
			cfg: ports.CORSConfig{
				AllowedOrigins: []string{""},
			},
			prefix:  "server.admin.cors",
			wantErr: true,
			errMsg:  "server.admin.cors",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCORSConfig(&tt.cfg, tt.prefix)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateCORSConfig() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("validateCORSConfig() error = %v, want error containing %q", err, tt.errMsg)
				}
			}

			if err != nil {
				if _, ok := err.(*config.ConfigError); !ok {
					t.Errorf("validateCORSConfig() error type = %T, want *config.ConfigError", err)
				}
			}
		})
	}
}

func TestValidateRequestContextConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ports.RequestContextConfig
		wantErr   bool
		wantField string
	}{
		{
			name: "trusted proxy disabled allows empty forwarded header",
			cfg: ports.RequestContextConfig{
				TrustedProxy: ports.RequestContextTrustedProxyConfig{
					Enabled: false,
				},
			},
			wantErr: false,
		},
		{
			name: "trusted proxy enabled with forwarded header passes",
			cfg: ports.RequestContextConfig{
				TrustedProxy: ports.RequestContextTrustedProxyConfig{
					Enabled:         true,
					ForwardedHeader: "X-Forwarded-For",
				},
				Trace: ports.RequestContextTraceConfig{
					ResponseEnabled: false,
				},
			},
			wantErr: false,
		},
		{
			name: "trusted proxy enabled requires a forwarded header",
			cfg: ports.RequestContextConfig{
				TrustedProxy: ports.RequestContextTrustedProxyConfig{
					Enabled: true,
				},
			},
			wantErr:   true,
			wantField: "request_context.trusted_proxy.forwarded_header",
		},
		{
			name: "trusted proxy enabled rejects whitespace-only forwarded header",
			cfg: ports.RequestContextConfig{
				TrustedProxy: ports.RequestContextTrustedProxyConfig{
					Enabled:         true,
					ForwardedHeader: "   ",
				},
			},
			wantErr:   true,
			wantField: "request_context.trusted_proxy.forwarded_header",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateRequestContextConfig(&tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateRequestContextConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if err != nil {
				configErr, ok := err.(*config.ConfigError)
				if !ok {
					t.Errorf("validateRequestContextConfig() error type = %T, want *config.ConfigError", err)
					return
				}
				if configErr.Field != tt.wantField {
					t.Errorf("validateRequestContextConfig() error field = %q, want %q", configErr.Field, tt.wantField)
				}
			}
		})
	}
}

func TestValidate_WithRequestContextConfig(t *testing.T) {
	cfg := validTestConfig()
	cfg.RequestContext.TrustedProxy.Enabled = true
	cfg.RequestContext.TrustedProxy.ForwardedHeader = ""

	err := Validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for missing trusted proxy forwarded header, got nil")
	}
	if !strings.Contains(err.Error(), "request_context.trusted_proxy.forwarded_header") {
		t.Errorf("expected request_context.trusted_proxy.forwarded_header in error, got: %v", err)
	}
	if configErr, ok := err.(*config.ConfigError); !ok {
		t.Errorf("Validate() error type = %T, want *config.ConfigError", err)
	} else if configErr.Field != "request_context.trusted_proxy.forwarded_header" {
		t.Errorf("Validate() error field = %q, want %q", configErr.Field, "request_context.trusted_proxy.forwarded_header")
	}
}
