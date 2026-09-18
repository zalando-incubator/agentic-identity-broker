// Package config — validate.go provides startup validation for ExtProc configuration.
// Validation rules from specs/015-extproc-token-exchange/contracts/configuration.md
// are enforced here. Validation runs after loading and before the service starts.
package config

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Validate checks the Config for required fields and constraint violations.
// Returns a descriptive error if any rules are violated (all errors collected).
// Implements fail-fast startup validation per FR-016 and US3 Scenario 2.
//
// Validation rules (per contracts/configuration.md):
//  1. grpc.port must be 1-65535
//  2. grpc.bind must not be empty
//  3. oauth2.token_endpoint must be a valid URL starting with http:// or https://
//  4. oauth2.issuer must be a valid URL starting with http:// or https://
//  5. oauth2.client_id must not be empty
//  6. oauth2.client_secret must not be empty after env var expansion
//  7. cache.default_ttl must be a positive duration
//  8. token_endpoint and issuer must use https:// unless oauth2.tls.allow_http is true
//  9. cache.max_ttl must be a positive duration
//  10. oauth2.exchange_timeout must be a positive duration
//  11. log.level must be one of debug, info, warn, error
//  12. log.format must be one of text, json
//  13. oauth2.client_assertion_type must be one of id_token, access_token
//  14. circuit_breaker.max_failures must be >= 1 (only when enabled)
//  15. circuit_breaker.reset_timeout must be a positive duration (only when enabled)
//  16. telemetry.exporter.endpoint must not be empty if telemetry.enabled is true
//  17. telemetry.exporter.protocol must be one of: grpc, http, https
//  18. telemetry.traces.sampling_rate must be in range [0.0, 1.0]
//  19. telemetry.exporter.timeout must be a positive duration
func Validate(cfg *Config) error {
	var errs []string
	const maxUint32 = ^uint32(0)

	// Rule 1: grpc.port must be 1-65535
	if cfg.GRPC.Port < 1 || cfg.GRPC.Port > 65535 {
		errs = append(errs, fmt.Sprintf("grpc.port must be between 1 and 65535, got %d", cfg.GRPC.Port))
	}

	// Rule 1a: grpc.max_concurrent_streams must fit grpc.MaxConcurrentStreams.
	if cfg.GRPC.MaxConcurrentStreams < 1 || uint64(cfg.GRPC.MaxConcurrentStreams) > uint64(maxUint32) {
		errs = append(errs, fmt.Sprintf("grpc.max_concurrent_streams must be between 1 and %d, got %d", maxUint32, cfg.GRPC.MaxConcurrentStreams))
	}

	// Rule 2: grpc.bind must not be empty
	if strings.TrimSpace(cfg.GRPC.Bind) == "" {
		errs = append(errs, "grpc.bind must not be empty")
	}

	// Rule 3: oauth2.token_endpoint must be a valid URL with http/https scheme
	if err := validateURL("oauth2.token_endpoint", cfg.OAuth2.TokenEndpoint, true); err != nil {
		errs = append(errs, err.Error())
	}

	// Rule 4: oauth2.issuer must be a valid URL with http or https scheme
	if err := validateURL("oauth2.issuer", cfg.OAuth2.Issuer, true); err != nil {
		errs = append(errs, err.Error())
	}

	// Rule 5: oauth2.client_id must not be empty
	if strings.TrimSpace(cfg.OAuth2.ClientID) == "" {
		errs = append(errs, "oauth2.client_id must not be empty")
	}

	// Rule 6: oauth2.client_secret must not be empty
	if strings.TrimSpace(cfg.OAuth2.ClientSecret) == "" {
		errs = append(errs, "oauth2.client_secret must not be empty")
	}

	// Rule 7: cache.default_ttl must be positive
	if cfg.Cache.DefaultTTL <= 0 {
		errs = append(errs, "cache.default_ttl must be a positive duration")
	}

	// Rule 8: TLS enforcement — token_endpoint and issuer must use https:// unless allow_http is set
	if !cfg.OAuth2.TLS.AllowHTTP {
		if cfg.OAuth2.TokenEndpoint != "" && strings.HasPrefix(cfg.OAuth2.TokenEndpoint, "http://") {
			errs = append(errs, "oauth2.token_endpoint must use https:// scheme (set oauth2.tls.allow_http: true to disable — DEV ONLY)")
		}
		if cfg.OAuth2.Issuer != "" && strings.HasPrefix(cfg.OAuth2.Issuer, "http://") {
			errs = append(errs, "oauth2.issuer must use https:// scheme (set oauth2.tls.allow_http: true to disable — DEV ONLY)")
		}
	}

	// Rule 9: cache.max_ttl must be positive
	if cfg.Cache.MaxTTL <= 0 {
		errs = append(errs, "cache.max_ttl must be a positive duration")
	}

	// Rule 10: oauth2.exchange_timeout must be positive
	if cfg.OAuth2.ExchangeTimeout <= 0 {
		errs = append(errs, "oauth2.exchange_timeout must be a positive duration")
	}

	// Rule 11: log.level must be a valid level if non-empty
	switch cfg.Log.Level {
	case "debug", "info", "warn", "error", "":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("log.level must be one of debug, info, warn, error; got %q", cfg.Log.Level))
	}

	// Rule 12: log.format must be a valid format if non-empty
	switch cfg.Log.Format {
	case "text", "json", "":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("log.format must be one of text, json; got %q", cfg.Log.Format))
	}

	// Rule 13: oauth2.client_assertion_type must be either "id_token" or "access_token"
	switch cfg.OAuth2.ClientAssertionType {
	case "id_token", "access_token":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("oauth2.client_assertion_type must be one of id_token, access_token; got %q", cfg.OAuth2.ClientAssertionType))
	}

	// Rule 14: circuit_breaker.max_failures must fit gobreaker.Settings (only when enabled).
	if cfg.CircuitBreaker.Enabled && (cfg.CircuitBreaker.MaxFailures < 1 || uint64(cfg.CircuitBreaker.MaxFailures) > uint64(maxUint32)) {
		errs = append(errs, fmt.Sprintf("circuit_breaker.max_failures must be between 1 and %d, got %d", maxUint32, cfg.CircuitBreaker.MaxFailures))
	}

	// Rule 15: circuit_breaker.reset_timeout must be positive (only when enabled)
	if cfg.CircuitBreaker.Enabled && cfg.CircuitBreaker.ResetTimeout <= 0 {
		errs = append(errs, "circuit_breaker.reset_timeout must be a positive duration")
	}

	policyPath := strings.TrimSpace(cfg.Authorization.Policy.Path)
	policyConfigFile := strings.TrimSpace(cfg.Authorization.Policy.ConfigFile)

	if !cfg.Authorization.Enabled && (policyPath != "" || policyConfigFile != "") {
		errs = append(errs, "authorization.policy.path or authorization.policy.config_file is set but authorization.enabled is false")
	} else if cfg.Authorization.Enabled {
		// Authorization rules — only enforced when authorization.enabled = true.
		// Rule A1: at least one policy source must be specified
		if policyPath == "" && policyConfigFile == "" {
			errs = append(errs, "authorization.policy.path or authorization.policy.config_file must be set when authorization is enabled")
		}

		// Rule A2: policy.path and policy.config_file are mutually exclusive
		if policyPath != "" && policyConfigFile != "" {
			errs = append(errs, "authorization.policy.path and authorization.policy.config_file are mutually exclusive")
		}

		// Rule A3: path traversal in policy.path — files and directories are both valid.
		if policyPath != "" {
			if err := validatePolicyPath("authorization.policy.path", cfg.Authorization.Policy.Path, false); err != nil {
				errs = append(errs, err.Error())
			}
		}

		// Rule A4: path traversal in policy.config_file — must be a file (not a directory).
		if policyConfigFile != "" {
			if err := validatePolicyPath("authorization.policy.config_file", cfg.Authorization.Policy.ConfigFile, true); err != nil {
				errs = append(errs, err.Error())
			}
		}

		// Rule A5: default_decision must remain fail-closed.
		if cfg.Authorization.DefaultDecision == "" {
			errs = append(errs, "authorization.default_decision must be deny")
		} else if cfg.Authorization.DefaultDecision != "deny" {
			errs = append(errs, fmt.Sprintf("authorization.default_decision must be deny; got %q", cfg.Authorization.DefaultDecision))
		}

		// Rule A6: evaluation_timeout must be positive
		if cfg.Authorization.EvaluationTimeout <= 0 {
			errs = append(errs, "authorization.evaluation_timeout must be a positive duration")
		}

		// Rule A7: max_body_size must be positive
		if cfg.Authorization.MaxBodySize <= 0 {
			errs = append(errs, "authorization.max_body_size must be a positive value")
		}

		// Rule A8: policy.package must not be empty
		if strings.TrimSpace(cfg.Authorization.Policy.Package) == "" {
			errs = append(errs, "authorization.policy.package must not be empty when authorization is enabled")
		}

		// Rule A9: policy.decision must not be empty
		if strings.TrimSpace(cfg.Authorization.Policy.Decision) == "" {
			errs = append(errs, "authorization.policy.decision must not be empty when authorization is enabled")
		}
	}

	// Telemetry validation only runs when telemetry.enabled is true
	if cfg.Telemetry.Enabled {
		// Rule 16: endpoint must not be empty
		if strings.TrimSpace(cfg.Telemetry.Exporter.Endpoint) == "" {
			errs = append(errs, "telemetry.exporter.endpoint must not be empty when telemetry.enabled is true")
		} else {
			// Validate endpoint format based on protocol
			protocol := cfg.Telemetry.Exporter.Protocol
			if protocol == "" {
				protocol = "grpc" // default protocol
			}
			if err := validateTelemetryEndpoint(cfg.Telemetry.Exporter.Endpoint, protocol); err != nil {
				errs = append(errs, err.Error())
			}
		}

		// Rule 17: if protocol is set, it must be one of: grpc, http, https
		if cfg.Telemetry.Exporter.Protocol != "" {
			switch cfg.Telemetry.Exporter.Protocol {
			case "grpc", "http", "https":
				// valid
			default:
				errs = append(errs, fmt.Sprintf("telemetry.exporter.protocol must be one of grpc, http, https; got %q", cfg.Telemetry.Exporter.Protocol))
			}
		}

		// Rule 18: sampling_rate must be in range [0.0, 1.0]
		if cfg.Telemetry.Traces.SamplingRate < 0.0 || cfg.Telemetry.Traces.SamplingRate > 1.0 {
			errs = append(errs, fmt.Sprintf("telemetry.traces.sampling_rate must be between 0.0 and 1.0; got %f", cfg.Telemetry.Traces.SamplingRate))
		}

		// Rule 19: exporter.timeout must be a positive duration
		if cfg.Telemetry.Exporter.Timeout <= 0 {
			errs = append(errs, "telemetry.exporter.timeout must be a positive duration")
		}
	}

	// Note: unrecognized propagators are not validated here. At runtime,
	// registerPropagators() in internal/adapters/telemetry logs unrecognized
	// propagators as warnings and skips them gracefully.
	_ = cfg.Telemetry.Traces.Propagators // Use propagators to avoid unused var warning

	if len(errs) > 0 {
		return fmt.Errorf("configuration validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

// validateURL checks that s is a non-empty valid URL. When requireHTTPScheme is true,
// the URL must have an http or https scheme. The host must be non-empty.
func validateURL(field, s string, requireHTTPScheme bool) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("%s must not be empty", field)
	}
	u, err := url.ParseRequestURI(s)
	if err != nil {
		return fmt.Errorf("%s must be a valid URL: %w", field, err)
	}
	if requireHTTPScheme && u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%s must be a valid URL starting with http:// or https://", field)
	}
	if u.Host == "" {
		return fmt.Errorf("%s must be a valid URL with a host", field)
	}
	return nil
}

// validatePolicyPath checks that a policy path does not contain path traversal
// segments and that the path exists. When fileOnly is true the path must be a
// regular file, not a directory (used for config_file). When fileOnly is false
// the path may be either a file or a directory (used for policy.path, which
// supports a single Rego file or a directory of Rego files).
func validatePolicyPath(field, path string, fileOnly bool) error {
	// Detect path traversal by checking for ".." path segments in the original path.
	// Using filepath.Clean and comparing avoids false positives on valid paths
	// containing ".." as part of a filename (e.g. "policy..v2.rego").
	for _, segment := range strings.Split(filepath.ToSlash(path), "/") {
		if segment == ".." {
			return fmt.Errorf("%s must not contain path traversal (\"..\")", field)
		}
	}

	// Verify the path exists and is readable at startup time.
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s: path not found or not readable: %w", field, err)
	}
	if fileOnly && info.IsDir() {
		return fmt.Errorf("%s: expected a file but got a directory", field)
	}

	return nil
}

// validateTelemetryEndpoint checks that the telemetry endpoint is valid for the given protocol.
// For HTTP, requires http:// URL. For HTTPS, requires https:// URL or bare host:port.
// For gRPC, requires a host:port format or unix: socket.
func validateTelemetryEndpoint(endpoint, protocol string) error {
	if strings.TrimSpace(endpoint) == "" {
		return fmt.Errorf("telemetry.exporter.endpoint must not be empty")
	}

	switch protocol {
	case "http":
		// HTTP requires http:// URL (not https://)
		u, err := url.ParseRequestURI(endpoint)
		if err != nil {
			return fmt.Errorf("telemetry.exporter.endpoint must be a valid URL for protocol %s: %w", protocol, err)
		}
		if u.Host == "" {
			return fmt.Errorf("telemetry.exporter.endpoint must be a valid URL with a host for protocol %s", protocol)
		}
		if u.Scheme != "http" {
			return fmt.Errorf("telemetry.exporter.endpoint for protocol http must use http:// scheme, not %s://", u.Scheme)
		}
	case "https":
		// HTTPS requires https:// URL or bare host:port (which auto-upgrades to https://).
		// Reject any non-https URI scheme (case-insensitive per RFC 3986 §3.1).
		lowered := strings.ToLower(endpoint)
		if strings.Contains(lowered, "://") {
			if !strings.HasPrefix(lowered, "https://") {
				return fmt.Errorf("telemetry.exporter.endpoint for protocol https must use https:// scheme or bare host:port, not %s",
					endpoint[:strings.Index(endpoint, "://")+3])
			}
			// Valid https:// URL — parse and validate host
			u, err := url.ParseRequestURI(endpoint)
			if err != nil {
				return fmt.Errorf("telemetry.exporter.endpoint must be a valid URL for protocol %s: %w", protocol, err)
			}
			if u.Host == "" {
				return fmt.Errorf("telemetry.exporter.endpoint must be a valid URL with a host for protocol %s", protocol)
			}
		} else if strings.HasPrefix(endpoint, "/") {
			// Absolute path — not a valid host:port
			return fmt.Errorf("telemetry.exporter.endpoint for protocol https must be https://... or host:port")
		} else {
			// Bare host:port fallback (no scheme). Use net.SplitHostPort for
			// strict parsing, then validate host is non-empty and port is numeric.
			host, port, err := net.SplitHostPort(endpoint)
			if err != nil || host == "" || port == "" {
				return fmt.Errorf("telemetry.exporter.endpoint for protocol https must be https://... or host:port")
			}
			if portNum, err := strconv.Atoi(port); err != nil || portNum < 1 || portNum > 65535 {
				return fmt.Errorf("telemetry.exporter.endpoint for protocol https must be https://... or host:port")
			}
		}
	case "grpc":
		// gRPC endpoints should be host:port or unix:path socket — not HTTP URLs.
		// Reject any URI-scheme prefix (case-insensitive) to catch http://, HTTP://, etc.
		// Only bare unix:path is supported; unix://path (double-slash) is rejected because
		// gRPC-Go interprets it differently from unix:path.
		// Note: gRPC bare host:port validation is intentionally looser than HTTPS
		// (no net.SplitHostPort or port-range check). gRPC-Go validates the address
		// at dial time, and we only reject clearly wrong formats here (HTTP URLs,
		// unix:// double-slash, mixed-case unix:).
		lowered := strings.ToLower(endpoint)
		if strings.HasPrefix(lowered, "unix://") {
			return fmt.Errorf("telemetry.exporter.endpoint for gRPC must use unix:path format, not unix://path")
		}
		// Reject mixed-case unix: (e.g. UNIX:/tmp/sock) — the gRPC resolver expects lowercase.
		if strings.HasPrefix(lowered, "unix:") && !strings.HasPrefix(endpoint, "unix:") {
			return fmt.Errorf("telemetry.exporter.endpoint for gRPC unix: prefix must be lowercase")
		}
		if strings.Contains(endpoint, "://") && !strings.HasPrefix(endpoint, "unix:") {
			return fmt.Errorf("telemetry.exporter.endpoint for gRPC must be host:port, not an HTTP URL")
		}
		if !strings.Contains(endpoint, ":") && !strings.HasPrefix(endpoint, "unix:") {
			return fmt.Errorf("telemetry.exporter.endpoint for gRPC should be in host:port format")
		}
	}

	return nil
}
