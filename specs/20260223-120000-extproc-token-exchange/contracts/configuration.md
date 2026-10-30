# ExtProc Token Exchange Configuration Contract

**Format**: YAML  
**Env Prefix**: `EXTPROC_`

## Configuration Schema

```yaml
# ExtProc Token Exchange Service Configuration
# File: extproc-token-exchange.yaml

# gRPC server settings
grpc:
  # Bind address for the gRPC server
  # Type: string
  # Default: "0.0.0.0"
  bind: "0.0.0.0"

  # Port for the gRPC server
  # Type: integer (1-65535)
  # Default: 50051
  port: 50051

  # Maximum number of concurrent gRPC streams
  # Type: integer
  # Default: 100
  max_concurrent_streams: 100

# OAuth2 and token exchange settings
oauth2:
  # Identity broker's token exchange endpoint
  # The ExtProc service sends RFC 8693 token exchange requests here
  # Type: string (URL, required)
  # Example: "http://identity-broker:8000/oauth2/token"
  token_endpoint: "http://identity-broker:8000/oauth2/token"

  # OAuth2 authorization server issuer
  # Used to obtain client assertion (ID token) via client_credentials grant
  # The token endpoint is derived as: {issuer}/oauth/token
  # Type: string (URL, required)
  issuer: "http://upstream-oauth2:9001"

  # Client ID for authenticating this ExtProc service
  # Used in client_credentials grant to obtain client assertion
  # Type: string (required)
  client_id: "extproc-gateway"

  # Client secret for authenticating this ExtProc service
  # Supports ${ENV_VAR} notation for environment variable injection
  # Type: string (required, sensitive)
  client_secret: "${EXTPROC_CLIENT_SECRET}"

  # Timeout for outbound HTTP calls (token exchange and client credentials)
  # Type: duration string (e.g., "5s", "10s")
  # Default: "5s"
  exchange_timeout: "5s"

  # Explicit token endpoint for the client_credentials grant
  # Used to obtain the client assertion (id_token).
  # If unset, defaults to {issuer}/oauth/token (Auth0-compatible providers).
  # Other providers (Keycloak, Hydra, etc.) MUST set this explicitly.
  # Type: string (URL, optional)
  # client_credentials_endpoint: "https://upstream-oauth2.example.com/realms/master/protocol/openid-connect/token"

  # TLS configuration for outbound HTTP calls
  tls:
    # Skip TLS certificate verification — NEVER use in production
    # Type: boolean
    # Default: false
    insecure_skip_verify: false

    # Path to custom CA bundle for self-signed certificates
    # Type: string (file path, optional)
    ca_bundle_path: ""

    # Allow HTTP endpoints — DEV ONLY. Disables SR-002 TLS enforcement.
    # Type: boolean
    # Default: false
    allow_http: false

# Token cache settings
cache:
  # Default TTL for cached exchanged tokens when the token exchange
  # response does not include an expires_in field
  # Type: duration string (e.g., "5m", "1h", "300s")
  # Default: "5m"
  default_ttl: "5m"

  # Maximum cache TTL regardless of expires_in value.
  # Caps very large expires_in values to limit stale token lifetime.
  # Type: duration string
  # Default: "1h"
  max_ttl: "1h"

# Logging configuration
log:
  # Log level
  # Type: string (debug|info|warn|error)
  # Default: "info"
  level: "info"

  # Log format
  # Type: string (text|json)
  # Default: "text"
  format: "text"

# Circuit breaker settings for token exchange HTTP calls
# Only 5xx server errors trip the circuit; 4xx client errors are excluded.
circuit_breaker:
  # Enable or disable the circuit breaker
  # Type: boolean
  # Default: true
  enabled: true

  # Consecutive 5xx failures before the circuit opens
  # Type: integer (>= 1, validated only when enabled)
  # Default: 5
  max_failures: 5

  # Duration the circuit stays open before allowing a probe
  # Type: duration string (e.g., "30s")
  # Default: "30s"
  reset_timeout: "30s"
```

## Environment Variable Mapping

| YAML Path | Environment Variable | Description |
|-----------|---------------------|-------------|
| `grpc.bind` | `EXTPROC_GRPC_BIND` | gRPC bind address |
| `grpc.port` | `EXTPROC_GRPC_PORT` | gRPC port |
| `grpc.max_concurrent_streams` | `EXTPROC_GRPC_MAX_CONCURRENT_STREAMS` | Maximum concurrent gRPC streams |
| `oauth2.token_endpoint` | `EXTPROC_OAUTH2_TOKEN_ENDPOINT` | Token exchange endpoint |
| `oauth2.issuer` | `EXTPROC_OAUTH2_ISSUER` | OAuth2 issuer |
| `oauth2.client_id` | `EXTPROC_OAUTH2_CLIENT_ID` | Client ID |
| `oauth2.client_secret` | `EXTPROC_OAUTH2_CLIENT_SECRET` | Client secret |
| `oauth2.client_credentials_endpoint` | `EXTPROC_OAUTH2_CLIENT_CREDENTIALS_ENDPOINT` | Explicit client credentials token endpoint |
| `oauth2.exchange_timeout` | `EXTPROC_OAUTH2_EXCHANGE_TIMEOUT` | Timeout for outbound token exchange HTTP calls |
| `oauth2.tls.insecure_skip_verify` | `EXTPROC_OAUTH2_TLS_INSECURE_SKIP_VERIFY` | Skip TLS certificate verification |
| `oauth2.tls.ca_bundle_path` | `EXTPROC_OAUTH2_TLS_CA_BUNDLE_PATH` | Path to custom CA bundle |
| `oauth2.tls.allow_http` | `EXTPROC_OAUTH2_TLS_ALLOW_HTTP` | Allow HTTP endpoints |
| `cache.default_ttl` | `EXTPROC_CACHE_DEFAULT_TTL` | Default cache TTL |
| `cache.max_ttl` | `EXTPROC_CACHE_MAX_TTL` | Maximum cache TTL cap |
| `log.level` | `EXTPROC_LOG_LEVEL` | Log level |
| `log.format` | `EXTPROC_LOG_FORMAT` | Log format |
| `circuit_breaker.enabled` | `EXTPROC_CIRCUIT_BREAKER_ENABLED` | Enable/disable circuit breaker |
| `circuit_breaker.max_failures` | `EXTPROC_CIRCUIT_BREAKER_MAX_FAILURES` | Consecutive 5xx failures before opening circuit |
| `circuit_breaker.reset_timeout` | `EXTPROC_CIRCUIT_BREAKER_RESET_TIMEOUT` | Duration in open state before probing |

## Validation Rules

### Startup Validation (fail-fast)

1. `grpc.port` must be 1-65535
2. `grpc.bind` must not be empty
3. `oauth2.token_endpoint` must be a valid URL starting with `http://` or `https://`
4. `oauth2.issuer` must be a valid URL
5. `oauth2.client_id` must not be empty
6. `oauth2.client_secret` must not be empty after env var expansion
7. `cache.default_ttl` must be a positive duration
8. `oauth2.token_endpoint` and `oauth2.issuer` must use `https://` scheme unless `oauth2.tls.allow_http: true` is set
9. `cache.max_ttl` must be a positive duration
10. `oauth2.exchange_timeout` must be a positive duration
11. `log.level` must be one of `debug`, `info`, `warn`, `error`
12. `log.format` must be one of `text`, `json`
13. `oauth2.client_assertion_type` must be one of `id_token`, `access_token`
14. `circuit_breaker.max_failures` must be >= 1 (only validated when `circuit_breaker.enabled` is `true`)
15. `circuit_breaker.reset_timeout` must be a positive duration (only validated when `circuit_breaker.enabled` is `true`)

### Security

- `client_secret` is treated as sensitive and MUST be redacted in logs
- All config values logged at startup summary level will redact sensitive fields

## Implementation Notes

### Viper Environment Variable Resolution

The loader uses `viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))` so that nested YAML keys map to underscored env vars: `oauth2.token_endpoint` → `EXTPROC_OAUTH2_TOKEN_ENDPOINT`. Without this replacer, Viper's `AutomaticEnv` would not match nested keys.

All string config values are processed through `os.ExpandEnv` to support `${VAR}` notation, not just `client_secret`.
