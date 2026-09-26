# Configuration Examples

This directory contains example configuration files for the Agentic Identity Broker. Each file demonstrates different configuration approaches for various deployment scenarios.

## Quick Start

The simplest way to run the application:

```bash
# Use all defaults (log level: info, format: text)
./agentic-identity-broker

# Specify a configuration file
./agentic-identity-broker --config ./examples/config/config.development.yaml

# Override settings via CLI flags
./agentic-identity-broker --config config.yaml --log-level debug --log-format json

# Use environment-specific configuration
GO_ENV=production ./agentic-identity-broker --config ./examples/config/config.production.yaml
```

## Configuration Files

### `extproc-tool-approvals.yaml`

This file configures OPA approval gating for the standalone ExtProc service. It contains the required approval API URL and session-header setting.

Use it as a full ExtProc configuration file:

```bash
EXTPROC_CONFIG_PATH=./examples/config/extproc-tool-approvals.yaml ./extproc-token-exchange
```

### `config.minimal.yaml`

The most minimal valid configuration. Demonstrates:
- Only required fields (log section with level and format)
- Uses explicit default values
- No environment variable substitution
- Suitable for testing or simple deployments

**Usage:**
```bash
./agentic-identity-broker --config ./examples/config/config.minimal.yaml
```

### `config.development.yaml`

Development environment configuration. Demonstrates:
- Debug logging level for detailed troubleshooting
- Text format for human-readable logs
- Commented-out sections showing future expansion points
- Quick iteration and local testing

**Usage:**
```bash
./agentic-identity-broker --config ./examples/config/config.development.yaml
```

Or combine with environment variables:
```bash
GO_ENV=development ./agentic-identity-broker --config ./examples/config/config.development.yaml
```

### `config.staging.yaml`

Staging/pre-production environment configuration. Demonstrates:
- Environment variable substitution with defaults
- Info logging level for normal operation visibility
- JSON format for structured logging and monitoring
- TLS configuration setup (commented)
- Higher connection pool limits
- Production-like settings but with logging visibility

**Usage:**
```bash
IDENTITY_BROKER_LOG_LEVEL=info IDENTITY_BROKER_LOG_FORMAT=json \
  ./agentic-identity-broker --config ./examples/config/config.staging.yaml
```

### `config.production.yaml`

Production environment configuration. Demonstrates:
- All sensitive values from environment variables (never hardcoded)
- Info logging with JSON format
- TLS mandatory with environment-driven paths
- High connection pool limits
- Monitoring and metrics configuration
- Security-first approach
- Detailed comments on production best practices

**Usage:**
```bash
# Set required environment variables
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(openssl rand -base64 32)"
export IDENTITY_BROKER_STORAGE_POSTGRES_URL="postgresql://prod-user:password@prod-db:5432/identity_broker"
export IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN="arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012"
export IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_ISSUER_URI="https://idp.example.com"
export IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_AUTHORIZE_ENDPOINT="https://idp.example.com/oauth2/authorize"
export IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TOKEN_ENDPOINT="https://idp.example.com/oauth2/token"

./agentic-identity-broker --config ./examples/config/config.production.yaml
```

### `config.yaml.example`

The original template file. Demonstrates:
- Basic YAML structure
- All logging configuration keys
- Environment variable syntax (${VAR_NAME})

**Usage:**
```bash
# Copy as template and customize
cp examples/config/config.yaml.example config.yaml
./agentic-identity-broker --config config.yaml
```

### `third-party-oauth2.yaml`

Third-party OAuth2 session management configuration. Demonstrates:
- JWE signing key configuration for state tokens
- State token TTL configuration (max 15 minutes)
- PKCE code verifier length configuration (32-128 bytes)
- Security-focused comments explaining each setting
- Environment variable substitution for sensitive keys

This configuration is required when enabling OAuth2 session management with third-party
services (GitHub, Google, Microsoft, etc.). It controls how the broker orchestrates
OAuth2 authorization flows and stores encrypted tokens.

**Usage:**
```bash
# Generate JWE signing key
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(openssl rand -base64 32)"

# Run with third-party OAuth2 configuration
./agentic-identity-broker --config ./examples/config/third-party-oauth2.yaml
```

**Security Note:** The JWE signing key MUST be kept secret. It protects OAuth2 state
tokens during the authorization flow. Compromise allows state token forgery and CSRF attacks.

### `jwt-preauth.yaml`

JWT Pre-Authentication configuration. Demonstrates:
- Signed JWT validation with JWKS endpoint (production recommended)
- Unsigned JWT support for service mesh environments (`verification: none`)
- CEL expressions for principal and profile attribute extraction (display name, email, picture URL)
- Fail-closed behavior when `authentication.jwt` is configured (no plain-header fallback)
- Mutual exclusivity enforcement (`verification: none` + `jwks_uri` → startup error)

**Usage:**
```bash
# Include jwt section in your main configuration file under server.enduser.authentication
# See jwt-preauth.yaml for complete examples of signed, unsigned, and plain-header-only configurations
server:
  enduser:
    authentication:
      jwt:
        jwks_uri: https://auth.example.com/.well-known/jwks.json
        claim_extraction:
          principal_expression: "claims.sub"
          email_expression: "claims.email"
```

**Key Features:**
- Cryptographic JWT signature verification via JWKS (default mode)
- Optional unsigned JWT support for trusted environments (explicit opt-in)
- CEL-based claim extraction for principal, display name, email, and picture URL
- Enriched `/api/me` response with profile attributes
- Fail-closed security: invalid JWTs always rejected, no silent fallback
- Bearer token auto-detection for Authorization header

### `token-exchange.yaml`

RFC 8693 OAuth 2.0 Token Exchange configuration. Demonstrates:
- CEL expressions for claim extraction from JWT tokens (principal and agent ID)
- Gateway authorization rules using CEL expressions
- Automatic token refresh configuration
- Security-first design with mandatory JWT validation
- Support for both minimal and complex authorization scenarios
- Both `multi_agent_client` modes for `agent_id_expression`

**Usage:**
```bash
# Include token_exchange section in your main configuration file
./agentic-identity-broker --config ./examples/config/config.yaml

# Where config.yaml includes token_exchange section:
token_exchange:
  claim_extraction:
    principal_expression: "subject_token.sub"
    # multi_agent_client disabled (default): resolve upstream client_id → agent.id UUID
    agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"
    # multi_agent_client enabled: use the agent ID claim name from multi_agent_client.agent_id_claim_name
    # agent_id_expression: "subject_token.x_agent_id"
  authorization:
    type: "cel"
    cel:
      expression: 'true'  # or more complex authorization rules
  refresh:
    enabled: true
```

**Key Features:**
- RFC 8693 compliant token exchange endpoint
- JWT validation against upstream OAuth2 JWKS (mandatory, no bypass)
- CEL-based gateway authorization policies
- Resource-based service discovery via protected_resources
- Automatic token refresh with configurable behavior
- Support for custom claim extraction expressions
- Complete audit logging of token exchange events
- `resolveAgentIdByClientId(clientId)` CEL helper when `multi_agent_client` is disabled to map upstream `client_id` → `agent.id` UUID

### `request-context.yaml`

Request security-context configuration. Demonstrates:
- Secure-by-default trusted-proxy handling (`trusted_proxy.enabled: false`)
- Configurable forwarded-header name for deployments behind a trusted ingress or reverse proxy
- Additive W3C `traceresponse` response-header emission (`trace.response_enabled: true`)
- The fact that request capture and log correlation stay enabled even if the response header is suppressed

**Usage:**
```bash
# Copy the request_context block into your main configuration file, or merge it as an overlay.
./agentic-identity-broker --config ./examples/config/config.development.yaml
```

**Key Features:**
- Capture is always enabled; there is no feature-level off switch
- Forwarding headers are ignored unless trusted proxy mode is explicitly enabled
- The broker treats the right-most forwarded entry as authoritative when proxy trust is enabled
- The `traceresponse` header is additive and does not change request or response body schemas

See [`request-context.yaml`](request-context.yaml) for the documented example block.

### `extproc-telemetry.yaml`

OpenTelemetry configuration example for the **ExtProc Token Exchange service**. Demonstrates:
- Configuration schema identical to the identity broker's `telemetry.yaml` (feature parity)
- Service-specific defaults: `service_name: extproc-token-exchange`, propagators: `["tracecontext", "ottrace", "b3multi", "baggage"]`
- Environment variable prefix: `EXTPROC_TELEMETRY_*` (for ExtProc gRPC service)
- Four deployment scenarios: development (full tracing), production (5% sampling), unreachable collector (graceful degradation), propagation-only (context forwarding without local spans)
- TLS certificate delegation to OTel SDK standard environment variables
- Distributed tracing through the token exchange chain: agentgateway → ExtProc → Identity Broker

**Usage:**

```bash
# Enable ExtProc telemetry with local collector
# Note: extproc-telemetry.yaml is a telemetry-focused overlay; it must be
# combined with a complete base config that includes all required fields
# (grpc, oauth2, cache, circuit_breaker). Use config merging or copy the
# telemetry block into your full config file.
EXTPROC_CONFIG_PATH=./path/to/full-config-with-telemetry.yaml \
  ./extproc-token-exchange

# Override endpoint via environment variable (overrides YAML)
EXTPROC_CONFIG_PATH=./path/to/full-config-with-telemetry.yaml \
EXTPROC_TELEMETRY_EXPORTER_ENDPOINT=collector.monitoring.svc:4317 \
  ./extproc-token-exchange
```

**Key Features:**
- Reuses the same telemetry configuration schema as the identity broker for operational parity
- Adds W3C Trace Context (`tracecontext`) to default propagators to handle agentgateway's traceparent headers
- End-to-end trace visibility: each token exchange request creates a child span linked to the upstream trace
- Graceful degradation when collector is unavailable (FR-008 — service continues operating normally)

See [`extproc-telemetry.yaml`](extproc-telemetry.yaml) for inline documentation covering all telemetry options.

### `telemetry.yaml`

OpenTelemetry observability configuration for the **Identity Broker** service. Demonstrates:
- Enabling distributed tracing, metrics, and log correlation
- Service name and resource attribute configuration
- Trace sampling rate and propagator selection
- Metrics export interval configuration
- OTLP exporter protocol (gRPC or HTTP), endpoint, headers with `${ENV_VAR}` substitution
- TLS configuration (enabled by default; `insecure: false` is the secure default)

Telemetry is **disabled by default** — no OTel SDK code runs when `enabled: false`, incurring zero overhead. Enable only when an OTLP-compatible collector is available.

**Usage:**
```bash
# Set the OTLP exporter auth token (if required by your collector)
export OTEL_EXPORTER_AUTH_TOKEN="your-collector-token"

# Enable telemetry by including this section in your configuration
./agentic-identity-broker --config ./examples/config/telemetry.yaml
```

**Key Features:**
- Zero overhead when disabled (no-op provider, no SDK initialization)
- Configurable sampling rate (default: 1.0 for development, 0.1 recommended for production)
- gRPC (default) or HTTP OTLP exporter protocol
- TLS enabled by default (`insecure: false`) — explicit opt-in required to disable
- Environment variable substitution for sensitive headers (e.g., auth tokens)
- Compatible with any OTLP-capable collector (Grafana Agent, OpenTelemetry Collector, Datadog, etc.)

**Security Note:** Never set `insecure: true` in production. This disables TLS for telemetry export, exposing trace data and metric data in transit. The broker will emit a WARN log if insecure mode is enabled at startup.

See [docs/configuration.md](../../docs/configuration.md) for the full list of configuration parameters and environment variable mappings.

### `oauth2-authorization-server.yaml`

OAuth2 Authorization Server proxy configuration. Demonstrates:
- Upstream OAuth2 server URLs (authorization, token endpoints)
- Supported OAuth2 grant types and response types
- Upstream request timeout configuration
- Upstream JWKS refresh floor/ceiling configuration
- TLS certificate validation (enforced by default)
- `multi_agent_client` block — enabled and disabled examples
- **Note**: The broker's public URL used for RFC 8414 metadata/issuer is configured via `server.enduser.public_url` (not inside the `oauth2_authorization_server` block)

**Usage:**
```bash
# Include in your main configuration file
./identity-broker --config ./examples/config/config.yaml

# Where config.yaml references oauth2-authorization-server section:
# oauth2_authorization_server:
#   mode: "proxy"
#   proxy:
#     upstream_issuer_uri: "https://auth.example.com"
#     upstream_authorize_endpoint: "https://auth.example.com/authorize"
#     upstream_token_endpoint: "https://auth.example.com/token"
#     upstream_jwks_min_refresh: "15m"
#     upstream_jwks_max_refresh: "1h"
#
# The broker's public URL (used as the OAuth2 issuer) is set separately:
# server:
#   enduser:
#     public_url: "https://identity-broker.example.com"
```

**Key Features:**
- RFC 6749 compliant OAuth2 Authorization Code flow
- RFC 8414 OAuth2 metadata discovery (/.well-known/oauth-authorization-server)
- Integration with broker's consent UI (Feature 007)
- Agent registry validation (Feature 006)
- TLS certificate validation enforced (no self-signed certificates)
- Audit logging for authorization requests
- Multi-agent client sharing: optional `multi_agent_client` block allows multiple agents to share one upstream OAuth2 application

**Multi-agent client configuration** (`multi_agent_client` block):

```yaml
# multi_agent_client disabled (default): each agent must have a unique client_id
multi_agent_client:
  enabled: false

# multi_agent_client enabled: multiple agents may share one upstream client_id
multi_agent_client:
  enabled: true
  agent_id_param_name: "x_agent_id"   # query param injected into upstream authorize redirect
  agent_id_claim_name: "x_agent_id"   # JWT claim verified in upstream token response
```

> **Token exchange configuration**: Set `token_exchange.claim_extraction.agent_id_expression` to `resolveAgentIdByClientId(subject_token.azp)` when `multi_agent_client.enabled = false`. Set it to `subject_token.<claim_name>` when `multi_agent_client.enabled = true`.

### `cimd.yaml` — Client ID Metadata Document (CIMD) Configuration

Annotated configuration example for the CIMD feature (Feature 028). Demonstrates the `cimd` block nested inside `oauth2_authorization_server`:
- `enabled` — enable URL-based `client_id` support (default: `false`)
- `fetch_timeout` / `max_response_bytes` — fetch safety limits
- `cache.max_ttl` / `cache.min_ttl` — operator TTL bounds for cached documents
- `ssrf.extra_blocked_cidrs` — additional CIDR ranges blocked beyond RFC 6890 defaults
- `client_name_blocklist` — case-insensitive keyword blocklist for CIMD `client_name` values

**Usage:**
```yaml
# Merge into your existing oauth2_authorization_server configuration:
oauth2_authorization_server:
  mode: "local"  # required: CIMD is only supported in local/hybrid mode
  # ... token_ttl, signing_keys.bootstrap_timeout, etc. (under the local: section) ...
  cimd:
    enabled: true
    fetch_timeout: 1s
    max_response_bytes: 5120
    cache:
      max_ttl: 1h
      min_ttl: 60s
```

See [cimd.yaml](cimd.yaml) for the full annotated example with all available options.

### OAuth2 Server Mode (`oauth2-server-mode.yaml`)

Configures the broker as a standalone OAuth2 authorization server using `local` mode. The broker mints its own JWT access tokens signed with managed ES256 keys, supports `client_credentials` and `authorization_code` (with PKCE) grant types, and exposes RFC 8414 discovery and JWKS endpoints.

Key settings:
- `mode: "local"` — switches from proxy mode to local token minting
- `local.token_ttl` — access token validity period (default: 1h)
- `local.token_claims_expression` — optional CEL expression for custom JWT claims
- `local.signing_keys.bootstrap_timeout` — startup budget for signing-key bootstrap coordination

**Usage:**
```bash
./agentic-identity-broker --config ./examples/config/oauth2-server-mode.yaml
```

### `impersonation.yaml` — RFC 8693 User Impersonation

Complete, bootable `local`-mode configuration demonstrating RFC 8693 user impersonation. A privileged client presents client-assertion, actor, and subject JWT credentials; the broker authorizes that client, resolves the registered target agent from the suffixed routing audience, verifies the subject's active user delegation to that agent, and mints a token whose `sub` represents the impersonated subject and whose `act` records the actor. A request activates only when its single `audience` is `<audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>`.

Key settings (`oauth2_authorization_server.impersonation`):
- `audience_prefix` — routing-only HTTP(S) URI prefix; requests append one target agent's canonical lower-case UUID or `canonical_id`. It is not an issued-token audience.
- `rules[]` — ordered, first-match list of impersonation rules (CR-002)
- `rules[].roles.{client_assertion,actor,subject}` — per-role `expected_audience` and `principal_expression` (subject also `email_expression`) (CR-003)
- `rules[].trusted_issuers[]` — trust anchors: `issuer_uri`, JWKS refresh bounds, `allowed_algorithms` (asymmetric only), and `signs_roles` (CR-007/CR-008)
- `rules[].authorization` — CEL predicate, reusing the token-exchange schema (`type: cel`, `cel.expression`, `cel.evaluation_timeout`) (CR-005)

The audience-selected target supplies minted `agent_id` and local policy `agent.*`; the asserted privileged-client identity is retained only for authorization and audit. `token_claims_expression` alone controls issued `aud`.

An active `UserGrant` for `(extracted subject, target agent)` is mandatory for every rule, including unverified subjects. A missing or expired grant returns generic `403 access_denied` and an `error_uri` to the existing target-agent consent page; no configuration option disables this check.

**Usage:**
```bash
./agentic-identity-broker --config ./examples/config/impersonation.yaml
```

### Hybrid Mode (`oauth2-hybrid-mode.yaml`)

Configures the broker to serve both proxy agents (forwarded to an upstream OAuth2 server) and local agents (tokens issued locally) in a single deployment. Agent classification is property-based: agents with `ClientID` set are ProxyClients; agents with `ClientURIs` set are CIMDClients; agents with neither are LocalClients.

Both the `proxy` and `local` sections are required in hybrid mode.

Key settings:
- `mode: "hybrid"` — enables all three client modes
- `proxy.*` — upstream OAuth2 server configuration (required)
- `proxy.upstream_jwks_min_refresh` / `proxy.upstream_jwks_max_refresh` — optional JWKS refresh bounds for the upstream cache
- `local.*` — local token issuance configuration (required)
- `local.signing_keys.bootstrap_timeout` — startup budget for signing-key bootstrap coordination
- `cimd.enabled` — optional CIMD support for URL-addressed agents

**Usage:**
```bash
./agentic-identity-broker --config ./examples/config/oauth2-hybrid-mode.yaml
```

### `extproc-opa-authorization.yaml`

ExtProc OPA-based authorization configuration. Demonstrates:
- OPA authorization enabled with a local Rego policy file
- OPA policy package and decision document configuration
- Default deny (fail-closed) security posture
- Evaluation timeout and max body size limits
- Alternative mode using OPA config file for bundle server integration

**Usage:**
```bash
# Run ExtProc with OPA authorization via local Rego file
EXTPROC_OAUTH2_CLIENT_ID="my-client" \
EXTPROC_OAUTH2_CLIENT_SECRET="my-secret" \
EXTPROC_CONFIG_PATH=./examples/config/extproc-opa-authorization.yaml \
  ./extproc-token-exchange
```

**Key Features:**
- Optional OPA authorization (disabled by default, zero overhead when off)
- Local Rego file support for quick setup and development
- OPA config file support for production bundle server integration
- Protocol-aware MCP tool call evaluation (`input.mcp.tool_name`, `input.type`)
- Set-based allow/deny policy pattern (deny always wins)
- Configurable evaluation timeout and body size limits
- Fail-closed security: denied by default when no rule matches

## Configuration Sources and Precedence

The application loads configuration from multiple sources with this precedence (highest to lowest):

1. **CLI Flags** (highest precedence)
   ```bash
   ./agentic-identity-broker --log-level debug --log-format json
   ```

2. **YAML File** (from --config flag or IDENTITY_BROKER_CONFIG_PATH)
   ```bash
   ./agentic-identity-broker --config ./examples/config/config.production.yaml
   ```

3. **.env Files** (environment-specific loading)
   - `.env` (always loaded)
   - `.env.local` (local overrides, not in version control)
   - `.env.{GO_ENV}` (environment-specific, e.g., `.env.production`)
   - `.env.{GO_ENV}.local` (local overrides for environment-specific, not in version control)

   Example with GO_ENV=production:
   ```bash
   GO_ENV=production ./agentic-identity-broker
   ```

4. **Defaults** (lowest precedence)
   - log.level: `info`
   - log.format: `text`

## Environment Variables

### Setting Environment Variables

**Via command line:**
```bash
export IDENTITY_BROKER_LOG_LEVEL=debug
./agentic-identity-broker
```

**Via .env file:**
Create `.env` in the application directory:
```
IDENTITY_BROKER_LOG_LEVEL=debug
IDENTITY_BROKER_LOG_FORMAT=json
IDENTITY_BROKER_JWE_SIGNING_KEY=base64-encoded-32-byte-jwe-key
```

**Via YAML substitution:**
```yaml
log:
  level: ${IDENTITY_BROKER_LOG_LEVEL}
  format: ${IDENTITY_BROKER_LOG_FORMAT}
```

### Sensitive Values

Values with keys containing `IDENTITY_BROKER_`, `password`, `secret`, `token`, `key`, or `credential` are redacted in logs:

```
Configuration Summary:
  log.level: info [source: CLI]
  log.format: json [source: YAML]
  IDENTITY_BROKER_JWE_SIGNING_KEY: ***REDACTED*** [source: Environment]
```

## Examples by Deployment Type

### Local Development

```bash
# Option 1: Use defaults only
./agentic-identity-broker

# Option 2: Use development config with debug logging
./agentic-identity-broker --config ./examples/config/config.development.yaml

# Option 3: Mix config file and CLI flags
./agentic-identity-broker --config ./examples/config/config.development.yaml --log-level debug
```

### Docker Container

```dockerfile
FROM golang:1.27.1-alpine as builder
WORKDIR /app
COPY . .
RUN go build -o agentic-identity-broker ./cmd/agentic-identity-broker

FROM alpine:latest
COPY --from=builder /app/agentic-identity-broker /usr/local/bin/
COPY --from=builder /app/examples/config/config.production.yaml /etc/agentic-identity-broker/config.yaml

ENV IDENTITY_BROKER_LOG_LEVEL=info
ENV IDENTITY_BROKER_LOG_FORMAT=json

CMD ["agentic-identity-broker", "--config", "/etc/agentic-identity-broker/config.yaml"]
```

### Kubernetes Deployment

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: agentic-identity-broker-config
data:
  config.yaml: |
    log:
      level: ${IDENTITY_BROKER_LOG_LEVEL:info}
      format: ${IDENTITY_BROKER_LOG_FORMAT:json}

---
apiVersion: v1
kind: Secret
metadata:
  name: agentic-identity-broker-secrets
type: Opaque
stringData:
  IDENTITY_BROKER_JWE_SIGNING_KEY: "base64-encoded-32-byte-jwe-key"
  IDENTITY_BROKER_STORAGE_POSTGRES_URL: "postgresql://..."
  IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN: "arn:aws:kms:eu-central-1:123456789012:key/..."

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: agentic-identity-broker
spec:
  template:
    spec:
      containers:
      - name: agentic-identity-broker
        image: agentic-identity-broker:latest
        args:
        - --config
        - /etc/agentic-identity-broker/config.yaml
        env:
        - name: IDENTITY_BROKER_LOG_LEVEL
          value: "info"
        - name: IDENTITY_BROKER_LOG_FORMAT
          value: "json"
        - name: IDENTITY_BROKER_JWE_SIGNING_KEY
          valueFrom:
            secretKeyRef:
              name: agentic-identity-broker-secrets
              key: IDENTITY_BROKER_JWE_SIGNING_KEY
        - name: IDENTITY_BROKER_STORAGE_POSTGRES_URL
          valueFrom:
            secretKeyRef:
              name: agentic-identity-broker-secrets
              key: IDENTITY_BROKER_STORAGE_POSTGRES_URL
        - name: IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN
          valueFrom:
            secretKeyRef:
              name: agentic-identity-broker-secrets
              key: IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN
        volumeMounts:
        - name: config
          mountPath: /etc/agentic-identity-broker
      volumes:
      - name: config
        configMap:
          name: agentic-identity-broker-config
```

## Environment-Specific .env Files

The application supports environment-specific .env files. Create them in the application root:

### `.env`
Loaded by all environments. Contains defaults:
```
IDENTITY_BROKER_LOG_LEVEL=info
IDENTITY_BROKER_LOG_FORMAT=text
```

### `.env.local`
Local overrides for development. Not in version control:
```
IDENTITY_BROKER_LOG_LEVEL=debug
IDENTITY_BROKER_LOG_FORMAT=json
```

### `.env.production`
Production-specific settings. Not in version control:
```
IDENTITY_BROKER_LOG_LEVEL=info
IDENTITY_BROKER_LOG_FORMAT=json
```

### `.env.production.local`
Local production overrides. Not in version control:
```
# Only for local testing of production config
IDENTITY_BROKER_STORAGE_POSTGRES_URL=postgresql://localhost:5432/test_db
```

## Configuration Validation

The application validates configuration at startup. Invalid values cause a clear error:

```
Error: Configuration error: invalid value "invalid" for field "log.level"
Expected: one of [debug, info, warn, error]
Source: CLI flag --log-level

Please fix the configuration and try again.
```

## Security Best Practices

1. **Never commit sensitive values** to version control
   ```bash
   # ❌ Don't do this
   echo "IDENTITY_BROKER_JWE_SIGNING_KEY=base64-encoded-32-byte-jwe-key" >> config.yaml
   git add config.yaml

   # ✅ Do this instead
   echo "IDENTITY_BROKER_JWE_SIGNING_KEY=${IDENTITY_BROKER_JWE_SIGNING_KEY}" >> .env.production.local
   ```

2. **Use .env.*.local files** for local overrides
   ```bash
   # These files are in .gitignore and won't be committed
   cat .env.production.local  # Safe to add secrets here
   ```

3. **Use environment variables in production**
   ```bash
   # Always prefer environment variables in containerized environments
   docker run -e IDENTITY_BROKER_JWE_SIGNING_KEY=base64-encoded-32-byte-jwe-key agentic-identity-broker
   ```

4. **Check file permissions** on configuration files
   ```bash
   # Restrict access to configuration files with secrets
   chmod 600 config.yaml
   chmod 600 .env.production.local
   ```

5. **Audit logs track all sources**
   ```json
   {
     "timestamp": "2025-12-15T10:30:00Z",
     "level": "info",
     "message": "configuration_loaded",
     "sources": [".env", "config.yaml", "cli_flags"],
     "config_keys": ["log.level", "log.format"],
     "redacted_keys": ["IDENTITY_BROKER_JWE_SIGNING_KEY"]
   }
   ```

## Troubleshooting

### "Required field 'log.level' is missing"

The log level must be set via one of these methods:
```bash
# Via CLI flag
./agentic-identity-broker --log-level info

# Via YAML file
echo "log:\n  level: info" > config.yaml
./agentic-identity-broker --config config.yaml

# Via .env file
echo "IDENTITY_BROKER_LOG_LEVEL=info" > .env
./agentic-identity-broker
```

### "Environment variable 'IDENTITY_BROKER_JWE_SIGNING_KEY' not set"

Set the environment variable before running:
```bash
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(openssl rand -base64 32)"
./agentic-identity-broker --config config.yaml
```

### "Permission denied reading 'config.yaml'"

Fix file permissions:
```bash
chmod +r config.yaml
./agentic-identity-broker --config config.yaml
```

### Wrong configuration loaded

Check the startup summary to see which configuration source was used:
```
Configuration Summary:
  log.level: debug [source: CLI]
  log.format: json [source: YAML]
```

The `[source: ...]` annotation shows where each value came from.

## Observability

The broker supports configurable OpenTelemetry (OTel) distributed tracing, metrics, and log correlation. Telemetry is **disabled by default** with zero overhead.

### Quick Start (Development)

```bash
# Start a local OpenTelemetry Collector (e.g., via Docker)
docker run --rm -p 4317:4317 otel/opentelemetry-collector-contrib

# Enable telemetry with gRPC exporter and insecure connection for local dev
./agentic-identity-broker --config ./examples/config/telemetry.yaml
```

### Configuration Reference

See [`telemetry.yaml`](telemetry.yaml) for a full production-ready example covering:
- `telemetry.enabled` — master switch (default: `false`)
- `telemetry.service_name` — service name on all spans and metrics
- `telemetry.resource_attributes` — custom key-value pairs added to all telemetry
- `telemetry.traces.sampling_rate` — fraction of traces to export (0.0–1.0)
- `telemetry.traces.propagators` — propagation formats (`ottrace`, `b3multi`, `b3`, `tracecontext`, `baggage`)
- `telemetry.metrics.export_interval` — how often to push metrics to the collector
- `telemetry.logs.enabled` — toggle OTLP log export (set `false` if collector lacks LogsService)
- `telemetry.exporter.protocol` — `grpc` (default), `http`, or `https`
- `telemetry.exporter.endpoint` — OTLP collector address (`host:port` for gRPC, `http(s)://host:port` for HTTP)
- `telemetry.exporter.headers` — additional headers (e.g., auth tokens via `${ENV_VAR}`)
- `telemetry.exporter.timeout` — per-export timeout
- `telemetry.exporter.compression` — `none` (default) or `gzip`
- `telemetry.exporter.insecure` — disable TLS (default: `false`, **never use in production**)

### Environment Variable Mapping

| YAML Key | Environment Variable |
|---|---|
| `telemetry.enabled` | `IDENTITY_BROKER_TELEMETRY_ENABLED` |
| `telemetry.service_name` | `IDENTITY_BROKER_TELEMETRY_SERVICE_NAME` |
| `telemetry.exporter.endpoint` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT` |
| `telemetry.exporter.protocol` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_PROTOCOL` |
| `telemetry.exporter.insecure` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE` |

See [docs/configuration.md](../../docs/configuration.md) for the complete list.

### ADR Reference

[ADR 011](../../adrs/011-opentelemetry-provider-pattern.md) documents the app-layer OTel provider pattern, `otelchi` library choice, and context-based span propagation approach.

## See Also

- [Configuration Guide](../../docs/configuration.md) - Comprehensive documentation
- [ADR 002](../../adrs/002-configuration-libraries.md) - Library selection rationale
- [ADR 011](../../adrs/011-opentelemetry-provider-pattern.md) - OpenTelemetry provider pattern
- [Architecture](../../docs/ARCHITECTURE.md) - Configuration subsystem architecture

### `approvals.yaml`

Tool approval configuration. Demonstrates:
- Pending TTL for approval expiry
- Sync coalesce window for long-poll batching
- Rate limiting per (principal, agent) pair

**Key settings:**
| Setting | Default | Description |
|---|---|---|
| `approvals.pending_ttl` | `10m` | Time before a pending approval expires |
| `approvals.sync_coalesce_window` | `1s` | Batch window for long-poll notifications |
| `approvals.rate_limit.max_pending_per_pair` | `50` | Max pending approvals per principal-agent pair |
| `approvals.rate_limit.max_requests_per_minute` | `10` | Max creation rate per principal-agent pair |

**Environment Variables:**
| Config Key | Environment Variable |
|---|---|
| `approvals.pending_ttl` | `APPROVAL_PENDING_TTL` |
| `approvals.sync_coalesce_window` | `APPROVAL_SYNC_COALESCE_WINDOW` |
| `approvals.rate_limit.max_pending_per_pair` | `APPROVAL_RATE_LIMIT_MAX_PENDING` |
| `approvals.rate_limit.max_requests_per_minute` | `APPROVAL_RATE_LIMIT_REQUESTS_PER_MINUTE` |
