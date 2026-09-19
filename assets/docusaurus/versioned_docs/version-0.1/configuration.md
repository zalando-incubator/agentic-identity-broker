---
title: Configuration
description: Configuration sources, precedence, and the full reference of settings for the Agentic Identity Broker.
---

# Configuration

This guide explains how to configure the Agentic Identity Broker for different deployment environments.

## Table of Contents

- [Overview](#overview)
- [Configuration Sources](#configuration-sources)
- [Precedence Rules](#precedence-rules)
- [Environment-Specific Configuration](#environment-specific-configuration)
- [YAML Configuration](#yaml-configuration)
- [Command-Line Flags](#command-line-flags)
- [Configuration Reference](#configuration-reference)
- [Available Settings](#available-settings)
- [Security Best Practices](#security-best-practices)
- [Troubleshooting](#troubleshooting)

## Overview

The Identity Broker supports multiple configuration sources with clear precedence rules. You can combine .env files, YAML configuration, and command-line flags to achieve flexible, environment-specific configuration without code changes.

### Key Features

- **Multiple Sources**: .env files, YAML, CLI flags
- **Environment Variable Substitution**: Use `${VAR_NAME}` in YAML files
- **Environment-Specific**: Automatic loading of `.env.{environment}` files
- **Secure by Default**: Sensitive values automatically redacted in logs
- **Clear Validation**: Helpful error messages with fix instructions
- **Audit Logging**: JSON audit log of all configuration sources

## Configuration Sources

The Identity Broker loads configuration from four sources (in order of precedence):

### 1. Built-in Defaults

Default values applied if no other source provides a value:

- `log.level`: `info`
- `log.format`: `text`

### 2. .env Files

Environment-specific files loaded automatically based on the `GO_ENV` environment variable:

1. `.env` - Base configuration (committed to git as .env.example)
2. `.env.local` - Local overrides (gitignored)
3. `.env.{GO_ENV}` - Environment-specific (e.g., .env.production)
4. `.env.{GO_ENV}.local` - Environment-specific local overrides (gitignored)

**Example** `.env`:

```bash
# Base configuration
IDENTITY_BROKER_LOG_LEVEL=info
IDENTITY_BROKER_LOG_FORMAT=text
```

**Example** `.env.production`:

```bash
# Production overrides
IDENTITY_BROKER_LOG_LEVEL=warn
IDENTITY_BROKER_LOG_FORMAT=json
```

### 3. YAML Configuration File

Optional YAML file for structured configuration. Supports environment variable substitution using `${VAR_NAME}` syntax.

**File Location** (in order of precedence):

1. Path from `--config` CLI flag
2. Path from `IDENTITY_BROKER_CONFIG_PATH` environment variable
3. `config.yaml` in current directory

**Example** `config.yaml`:

```yaml
log:
  level: ${IDENTITY_BROKER_LOG_LEVEL}
  format: ${IDENTITY_BROKER_LOG_FORMAT}
```

### 4. Command-Line Flags

Highest precedence - overrides all other sources.

```bash
agentic-identity-broker --log-level debug --log-format json
```

## Precedence Rules

When the same configuration key is provided by multiple sources, the value from the highest-precedence source wins:

```
CLI Flags > YAML > .env Files > Defaults
   (3)       (2)      (1)        (0)
```

**Example**:

- Defaults set `log.level=info`
- `.env` sets `IDENTITY_BROKER_LOG_LEVEL=warn`
- `config.yaml` sets `log.level=${IDENTITY_BROKER_LOG_LEVEL}` (expands to `warn`)
- CLI flag `--log-level=debug` is provided

**Result**: `log.level=debug` (from CLI flag)

## Environment-Specific Configuration

Use the `GO_ENV` environment variable to control which .env files are loaded:

### Development (default)

```bash
# GO_ENV defaults to "development" if not set
agentic-identity-broker

# Loads: .env → .env.local → .env.development → .env.development.local
```

### Production

```bash
GO_ENV=production agentic-identity-broker

# Loads: .env → .env.local → .env.production → .env.production.local
```

### Staging

```bash
GO_ENV=staging agentic-identity-broker

# Loads: .env → .env.local → .env.staging → .env.staging.local
```

## YAML Configuration

### Basic Structure

```yaml
log:
  level: info    # debug, info, warn, error
  format: text   # text, json
```

### Environment Variable Substitution

Reference environment variables using `${VAR_NAME}` syntax:

```yaml
log:
  level: ${IDENTITY_BROKER_LOG_LEVEL}
  format: ${IDENTITY_BROKER_LOG_FORMAT}
```

### Nested Variable References

Environment variables can reference other environment variables:

```bash
# In .env
BASE_LEVEL=info
IDENTITY_BROKER_LOG_LEVEL=${BASE_LEVEL}
```

```yaml
# In config.yaml
log:
  level: ${IDENTITY_BROKER_LOG_LEVEL}  # Expands to "info"
```

**Note**: Circular references are detected and will cause an error. Maximum expansion depth is 10 levels.

### Security Validation

The following patterns are **rejected** for security:

- Command substitution: `$(command)` or backticks
- Shell metacharacters: `;`, `|`, `&`, `>`, `<`
- These protections prevent command injection attacks

## Command-Line Flags

### Available Flags

```bash
agentic-identity-broker [flags]

Flags:
  -c, --config string       config file path (overrides IDENTITY_BROKER_CONFIG_PATH)
      --log-level string    log level: debug, info, warn, error
      --log-format string   log format: text, json
  -h, --help               help for agentic-identity-broker
```

### Examples

```bash
# Override log level
agentic-identity-broker --log-level debug

# Use custom config file
agentic-identity-broker --config /etc/agentic-identity-broker/config.yaml

# Multiple flags
agentic-identity-broker --log-level debug --log-format json

# Short form for config
agentic-identity-broker -c config.production.yaml --log-level warn
```

## Configuration Reference

This section lists all configuration options. See [Available Settings](#available-settings) for detailed descriptions and examples.

### Current Configuration Options

#### Logging Configuration

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------------|--------------|-----------|----------------------|----------|-------------|
| `log.level` | enum | `info` | `debug`, `info`, `warn`, `error` | No | `IDENTITY_BROKER_LOG_LEVEL` | `--log-level` | Sets logging verbosity level. Use `debug` for troubleshooting, `info` for normal operation, `warn` for production. |
| `log.format` | enum | `text` | `text`, `json` | No | `IDENTITY_BROKER_LOG_FORMAT` | `--log-format` | Sets log output format. Use `json` for production and log aggregation systems. |

#### Encryption Configuration

The broker uses a backend-explicit encryption contract.
Configure exactly one of `encryption.aws_kms` or `encryption.memory`.

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
| -------- | ------ | --------------- | -------------- | ----------- | ---------------------- | ---------- | ------------- |
| `encryption.aws_kms.key_arn` | string | - | AWS KMS key ARN or alias ARN | Yes when `encryption.aws_kms` is set | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN` | N/A | Customer-managed KMS key for the AWS KMS backend. |
| `encryption.aws_kms.dynamodb_table_name` | string | `IdentityBrokerEncryptionBranchKeys` | DynamoDB table name | No | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME` | N/A | DynamoDB table used to cache hierarchical branch keys. |
| `encryption.aws_kms.branch_key_ttl` | duration string | `1h` | Go duration string | No | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL` | N/A | Lifetime of cached branch keys in DynamoDB. |
| `encryption.aws_kms.dynamodb_region` | string | AWS SDK default region | AWS region | No | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_REGION` | N/A | Optional region override for DynamoDB branch-key storage. |
| `encryption.aws_kms.dynamodb_timeout` | duration string | - | Positive Go duration string | No | `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT` | N/A | Optional timeout applied to AWS-backed encryption operations. |
| `encryption.memory.raw_key` | string | - | Base64-encoded 32-byte AES-256 key | Yes when `encryption.memory` is set | `IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY` | N/A | Raw AES key for the in-memory backend. Sensitive - redacted in logs. |

**Encryption Configuration Notes:**

- Exactly one of `encryption.aws_kms` or `encryption.memory` must be configured.
- `encryption.aws_kms.key_arn` accepts KMS key ARNs and alias ARNs.
- `encryption.memory.raw_key` must decode to exactly 32 bytes.
- Startup fails closed if encryption configuration is missing or invalid.
- Tokens encrypted with old KEK material remain decryptable as long as the corresponding backend material remains available.

#### Server Configuration

The Identity Broker runs two independent HTTP servers on separate ports:

- **End-User Server**: Public-facing API for authentication and identity operations (default port 8000)
- **Admin Server**: Internal management API for monitoring and administration (default port 14000)

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------------|--------------|-----------|----------------------|----------|-------------|
| `server.enduser.port` | integer | `8000` | 1-65535 | No | `IDENTITY_BROKER_SERVER_ENDUSER_PORT` | `--server.enduser.port` | Port for end-user server. Must differ from admin port. |
| `server.enduser.bind` | string | `::` | IPv4/IPv6 address or hostname | No | `IDENTITY_BROKER_SERVER_ENDUSER_BIND` | `--server.enduser.bind` | Bind address for end-user server. Use `::` for dual-stack (IPv6+IPv4), `0.0.0.0` for IPv4 only, or `127.0.0.1` for localhost only. |
| `server.admin.port` | integer | `14000` | 1-65535 | No | `IDENTITY_BROKER_SERVER_ADMIN_PORT` | `--server.admin.port` | Port for admin server. Must differ from end-user port. |
| `server.admin.bind` | string | `::` | IPv4/IPv6 address or hostname | No | `IDENTITY_BROKER_SERVER_ADMIN_BIND` | `--server.admin.bind` | Bind address for admin server. In production, restrict to private network (e.g., `10.0.1.0`) or use firewall rules. |
| `server.shutdown.timeout` | duration | `30s` | 1s-5m | No | `IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT` | `--server.shutdown.timeout` | Maximum time to wait for in-flight requests to complete during graceful shutdown. Use longer timeouts (60s) in production. |

**Server configuration notes:**

- Both servers start together. If either server cannot bind, both stop.
- After startup, each server runs independently. A failure in one server does not affect the
  other server.
- Both servers provide `/health`.
- Graceful shutdown waits for active requests until the configured timeout.

#### Storage Configuration

Storage configuration controls the persistence backend and steady-state timeout budgets used by storage-backed workflows.

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------------|--------------|-----------|----------------------|----------|-------------|
| `storage.backend` | enum | `memory` | `memory`, `postgres` | No | `IDENTITY_BROKER_STORAGE_BACKEND` | N/A | Selects the storage backend. Use `postgres` for persistent or multi-replica deployments. |
| `storage.postgres.connection_url` | string | - | PostgreSQL connection URL | Yes when `storage.backend=postgres` | `IDENTITY_BROKER_STORAGE_POSTGRES_URL` | N/A | PostgreSQL connection string for the persistent storage backend. |
| `storage.timeouts.read` | duration | `5s` | Positive duration | No | N/A | N/A | Timeout for steady-state storage read operations. |
| `storage.timeouts.write` | duration | `10s` | Positive duration | No | N/A | N/A | Timeout for steady-state storage write operations. |

**Storage configuration notes:**

- `storage.timeouts.read` and `storage.timeouts.write` apply to steady-state repository
  operations.
- `oauth2_authorization_server.local.signing_keys.bootstrap_timeout` defines the
  signing-key startup time limit. It applies to local and hybrid token issuance.

**Example YAML:**

```yaml
storage:
  backend: postgres
  postgres:
    connection_url: ${IDENTITY_BROKER_STORAGE_POSTGRES_URL}
  timeouts:
    read: 5s
    write: 10s
```

#### IPv4/IPv6 dual-stack support

The Identity Broker supports these network bindings:

- **Dual-stack (default):** `::` accepts IPv6 and IPv4 connections on dual-stack systems.
- **IPv6 only:** Use `::1` for localhost or a specific IPv6 address.
- **IPv4 only:** Use `0.0.0.0` for all interfaces or `127.0.0.1` for localhost.
- **Fallback:** If IPv6 binding fails, the broker uses IPv4 and writes a warning log.

Examples are in `examples/config/`:

- `config.ipv6-only.yaml` uses dual-stack with IPv6 preference.
- `config.ipv4-only.yaml` uses IPv4 only.

#### Graceful Shutdown

The broker completes active requests during graceful shutdown:

1. On `SIGTERM` or `SIGINT`, health status becomes `shutting_down`.
2. New requests return HTTP 503 Service Unavailable.
3. Active requests can complete until `server.shutdown.timeout`.
4. After the timeout, the broker closes remaining connections.
5. The process exits.

**Example:**

```bash
# Start broker
agentic-identity-broker &

# Graceful shutdown (waits for requests)
kill -TERM $(pgrep agentic-identity-broker)

# Force shutdown (immediate)
kill -KILL $(pgrep agentic-identity-broker)
```

#### Authentication Configuration

Pre-authentication mode allows the Identity Broker to trust authenticated principals from a reverse proxy. The reverse proxy handles user authentication and passes the authenticated user identifier via an HTTP header.

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | Description |
|--------|------|---------------|--------------|-----------|----------------------|-------------|
| `server.enduser.authentication.preauth.principal_header_name` | string | `X-Remote-User` | Any HTTP header name | No | `IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME` | HTTP header containing the authenticated user principal for end-user server |
| `server.admin.authentication.preauth.principal_header_name` | string | `X-Remote-User` | Any HTTP header name | No | `IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME` | HTTP header containing the authenticated user principal for admin server |

**Pre-Authentication Configuration Notes:**

- The principal header must be set by a **trusted reverse proxy only** (nginx, Traefik, HAProxy, etc.)
- Never expose the Identity Broker to untrusted networks - always use a reverse proxy for authentication
- The principal value is trimmed of leading/trailing whitespace
- Maximum principal length: 200 characters (longer principals are rejected with 400 Bad Request)
- Empty/missing headers result in 401 Unauthorized on protected routes
- Optional authentication routes allow missing principals

**Example YAML configurations:**

Development (custom header):

```yaml
server:
  enduser:
    port: 8000
    bind: "::"
    authentication:
      preauth:
        principal_header_name: X-Authenticated-User
  admin:
    port: 14000
    bind: "::"
    authentication:
      preauth:
        principal_header_name: X-Remote-User
```

Production (environment variable):

```yaml
server:
  enduser:
    port: 8000
    bind: "::"
    authentication:
      preauth:
        principal_header_name: ${IDENTITY_BROKER_PRINCIPAL_HEADER}
  admin:
    port: 14000
    bind: "10.0.1.0"  # Restrict to private network
    authentication:
      preauth:
        principal_header_name: ${IDENTITY_BROKER_PRINCIPAL_HEADER}
```

Then set the environment variable:

```bash
export IDENTITY_BROKER_PRINCIPAL_HEADER="X-Authenticated-User"
```

**Nginx Reverse Proxy Example:**

```nginx
upstream identity_broker {
    server localhost:8000;
}

server {
    listen 80;
    server_name api.example.com;

    location / {
        auth_request /auth;
        proxy_pass http://identity_broker;
        proxy_set_header X-Remote-User $remote_user;
    }

    location = /auth {
        internal;
        auth_basic "Restricted";
        auth_basic_user_file /etc/nginx/.htpasswd;
        return 200;
    }
}
```

#### Request Context Configuration

The broker captures a request security context for every inbound HTTP request. This capture is
always enabled. This section configures trusted-proxy caller IP derivation and the additive
W3C `traceresponse` response header.

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------------|--------------|-----------|----------------------|----------|-------------|
| `request_context.trusted_proxy.enabled` | boolean | `false` | `true`, `false` | No | `IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_ENABLED` | `--request_context.trusted_proxy.enabled` | Use the configured forwarded header to derive the caller IP. When `false`, the broker ignores client forwarding headers and uses the direct connection address. |
| `request_context.trusted_proxy.forwarded_header` | string | `X-Forwarded-For` | Any HTTP header name | No | `IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_FORWARDED_HEADER` | `--request_context.trusted_proxy.forwarded_header` | Header read in trusted-proxy mode. The right-most entry is authoritative. |
| `request_context.trace.response_enabled` | boolean | `true` | `true`, `false` | No | `IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED` | `--request_context.trace.response_enabled` | Add the W3C `traceresponse` response header. Disabling this option does not disable request capture or log correlation. |

**Request context notes:**

- Security-context capture is always enabled. It has no feature-level disable switch.
- When `request_context.trusted_proxy.enabled=false`, the broker ignores forwarding headers
  to prevent IP spoofing.
- In trusted-proxy mode, the broker uses the right-most configured-header entry as the caller
  IP.
- `request_context.trace.response_enabled=false` removes only the response header.
  Structured logs still include `trace_id`.

**Example YAML:**

```yaml
request_context:
  trusted_proxy:
    enabled: false
    forwarded_header: X-Forwarded-For
  trace:
    response_enabled: true
```

**Notes:**

- All configuration options have built-in defaults and are optional unless marked "Required"
- Environment variables follow the pattern: `IDENTITY_BROKER_{SECTION}_{KEY}` (uppercased)
- CLI flags follow the pattern: `--{section}-{key}` (lowercase with hyphens)
- See [Precedence Rules](#precedence-rules) for how values from different sources are resolved
- For YAML configuration syntax, see [YAML Configuration](#yaml-configuration)

### Future Configuration Options

The following configuration sections are planned for future releases. This table documents the intended structure for extensibility:

| Option | Type | Default Value | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------------|--------------|-----------|----------------------|----------|-------------|
| `server.host` | string | `0.0.0.0` | Any valid hostname/IP | No | `IDENTITY_BROKER_SERVER_HOST` | `--server-host` | Server bind address. Use `127.0.0.1` for local-only access. |
| `server.port` | integer | `8080` | 1-65535 | No | `IDENTITY_BROKER_SERVER_PORT` | `--server-port` | Server listen port for HTTP requests. |
| `server.tls.enabled` | boolean | `false` | `true`, `false` | No | `IDENTITY_BROKER_SERVER_TLS_ENABLED` | `--server-tls-enabled` | Enable TLS/HTTPS for secure connections. |
| `server.tls.cert_file` | string | - | Valid file path | Yes (if TLS enabled) | `IDENTITY_BROKER_SERVER_TLS_CERT_FILE` | `--server-tls-cert-file` | Path to TLS certificate file (PEM format). |
| `server.tls.key_file` | string | - | Valid file path | Yes (if TLS enabled) | `IDENTITY_BROKER_SERVER_TLS_KEY_FILE` | `--server-tls-key-file` | Path to TLS private key file (PEM format). |
| `database.type` | enum | `postgres` | `postgres`, `mysql`, `sqlite` | No | `IDENTITY_BROKER_DATABASE_TYPE` | `--database-type` | Database backend type for persistent storage. |
| `database.host` | string | `localhost` | Valid hostname/IP | Yes | `IDENTITY_BROKER_DATABASE_HOST` | `--database-host` | Database server hostname or IP address. |
| `database.port` | integer | `5432` | 1-65535 | No | `IDENTITY_BROKER_DATABASE_PORT` | `--database-port` | Database server port (defaults: PostgreSQL 5432, MySQL 3306). |
| `database.name` | string | `identity_broker` | Valid database name | Yes | `IDENTITY_BROKER_DATABASE_NAME` | `--database-name` | Database name to use for broker data. |
| `database.username` | string | - | Valid username | Yes | `IDENTITY_BROKER_DATABASE_USERNAME` | `--database-username` | Database authentication username. |
| `database.password` | string | - | Any string | Yes | `IDENTITY_BROKER_DATABASE_PASSWORD` | N/A | Database authentication password. Sensitive - redacted in logs. |
| `database.ssl_mode` | enum | `prefer` | `disable`, `allow`, `prefer`, `require`, `verify-ca`, `verify-full` | No | `IDENTITY_BROKER_DATABASE_SSL_MODE` | `--database-ssl-mode` | SSL/TLS mode for database connections. |
| `database.max_connections` | integer | `25` | 1-1000 | No | `IDENTITY_BROKER_DATABASE_MAX_CONNECTIONS` | `--database-max-connections` | Maximum number of open database connections in pool. |
| `auth.jwt.secret` | string | - | Base64 string (min 32 bytes) | Yes | `IDENTITY_BROKER_AUTH_JWT_SECRET` | N/A | JWT signing secret key. Sensitive - redacted in logs. |
| `auth.jwt.expiry` | duration | `1h` | Valid duration (e.g., `30m`, `2h`) | No | `IDENTITY_BROKER_AUTH_JWT_EXPIRY` | `--auth-jwt-expiry` | JWT token expiration duration. |
| `auth.session.timeout` | duration | `24h` | Valid duration | No | `IDENTITY_BROKER_AUTH_SESSION_TIMEOUT` | `--auth-session-timeout` | User session inactivity timeout. |
| `auth.providers` | list | `[]` | Array of provider configs | Yes | N/A | N/A | List of configured identity providers (OAuth, SAML, etc.). |

**Future options notes:**

- These options are for planning and design.
- The current release does not implement them.
- Requirements and feedback can change their structure.
- Sensitive values, such as passwords, secrets, and keys, never have CLI flags.
- Duration values use formats such as `30s`, `5m`, `1h`, and `24h`.

### Naming Conventions

The configuration system follows consistent naming patterns across all sources:

1. **YAML Paths**: Use dot notation with lowercase keys (e.g., `log.level`, `server.tls.enabled`)
2. **Environment Variables**: Prefix + uppercase + underscores (e.g., `IDENTITY_BROKER_LOG_LEVEL`, `IDENTITY_BROKER_SERVER_TLS_ENABLED`)
3. **CLI Flags**: Lowercase with hyphens (e.g., `--log-level`, `--server-tls-enabled`)
4. **Nested Config**: Each level adds a separator (`.` in YAML, `_` in env vars, `-` in flags)

### Configuration by Use Case

**Development Environment:**

```yaml
log:
  level: debug
  format: text
```

**Production Environment:**

```yaml
log:
  level: warn
  format: json
```

**Troubleshooting:**

```bash
# Temporarily override to debug level
agentic-identity-broker --log-level debug
```

For complete configuration examples and detailed explanations, continue to the [Available Settings](#available-settings) section.

## Available Settings

### Logging Configuration

#### log.level

**Description**: Sets the logging verbosity level.

**Valid Values**: `debug`, `info`, `warn`, `error`

**Default**: `info`

**Environment Variable**: `IDENTITY_BROKER_LOG_LEVEL`

**CLI Flag**: `--log-level`

**Examples**:

```bash
# .env file
IDENTITY_BROKER_LOG_LEVEL=debug

# YAML file
log:
  level: warn

# CLI flag
--log-level error
```

#### log.format

**Description**: Sets the log output format.

**Valid Values**: `text`, `json`

**Default**: `text`

**Environment Variable**: `IDENTITY_BROKER_LOG_FORMAT`

**CLI Flag**: `--log-format`

**Examples**:

```bash
# .env file
IDENTITY_BROKER_LOG_FORMAT=json

# YAML file
log:
  format: json

# CLI flag
--log-format json
```

**Recommendation**: Use `json` format in production for structured logging and log aggregation.

### Encryption Backend Configuration

The broker requires encryption in every environment.
Configure exactly one of `encryption.aws_kms` or `encryption.memory`.

- `encryption.aws_kms` for AWS KMS with hierarchical branch-key caching
- `encryption.memory` for development/testing with a base64 AES-256 key

There are no CLI flags for encryption configuration.

#### encryption.aws_kms

**Description**: Configures the AWS KMS backend for envelope encryption.

**Required field**: `encryption.aws_kms.key_arn`

**Core environment variables**:

- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_REGION`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TIMEOUT`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_REGION`
- `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_ASSUME_ROLE_ARN`

**Example**:

```yaml
# config.yaml - Production deployment
encryption:
  aws_kms:
    key_arn: ${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN}
    dynamodb_table_name: ${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME:IdentityBrokerEncryptionBranchKeys}
    branch_key_ttl: ${IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL:1h}
```

```bash
# .env.production
IDENTITY_BROKER_ENCRYPTION_AWS_KMS_KEY_ARN=arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012
IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DYNAMODB_TABLE_NAME=IdentityBrokerEncryptionBranchKeys
IDENTITY_BROKER_ENCRYPTION_AWS_KMS_BRANCH_KEY_TTL=1h
```

`key_arn` accepts both KMS key ARNs and alias ARNs.

#### encryption.memory.raw_key

**Description**: Configures the in-memory backend with a base64-encoded 32-byte AES-256 key.

**Environment Variable**: `IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY`

**Example**:

`.env.local` values are literal strings; paste a generated base64 key instead of shell command substitution.

```dotenv
# .env.local
IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY=base64-encoded-32-byte-key
```

```bash
# shell
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(openssl rand -base64 32)"
```

```yaml
# config.yaml - Development deployment
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
```

**AWS KMS Setup Instructions**:

1. Create a customer-managed key in AWS KMS:

```bash
aws kms create-key \
  --description "Identity Broker OAuth Token Encryption Key" \
  --region eu-central-1
```

1. Create an alias for easier reference:

```bash
aws kms create-alias \
  --alias-name "alias/identity-broker-encryption" \
  --target-key-id "arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012"
```

1. Grant IAM permissions to the Identity Broker service role:

```bash
aws kms create-grant \
  --key-id "arn:aws:kms:eu-central-1:123456789012:key/12345678-1234-1234-1234-123456789012" \
  --grantee-principal "arn:aws:iam::123456789012:role/IdentityBrokerRole" \
  --operations "Encrypt" "Decrypt" "GenerateDataKey" "DescribeKey"
```

1. Reference the key in configuration:

```yaml
encryption:
  aws_kms:
    key_arn: "arn:aws:kms:eu-central-1:123456789012:alias/identity-broker-encryption"
```

**Startup validation:**

- **AWS KMS backend:** At startup, the broker validates KMS key access and service
  permissions. It does not start when validation fails.
- **Memory backend:** At startup, the broker validates
  `IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY` or its YAML value. The value must decode to a
  32-byte key. The broker does not start when validation fails.

**Security considerations:**

- **AWS KMS:** The KEK remains in AWS KMS. The application receives only encrypted data
  encryption keys. KMS performs cryptographic operations.
- **Memory backend:** The broker loads KEK material into application memory at startup.
- **Rotation:** KMS keys can rotate without an application restart. Tokens encrypted with
  prior key material remain decryptable.
- **Permissions:** Grant the service role `kms:Decrypt` and `kms:GenerateDataKey`.
  Limit permissions to the required resources.

**Backend by environment:**

| Environment | Approach | Configuration |
| --- | --- | --- |
| Production | AWS KMS | `encryption.aws_kms.key_arn` with a customer-managed KMS key |
| Staging | AWS KMS | A separate AWS KMS key for each environment |
| Development | Memory backend | `encryption.memory.raw_key` from `IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY` |
| CI/CD | Memory backend | `encryption.memory.raw_key` from CI/CD secrets |
| Testing | Memory backend | A new random key for each test |


### Request Context Configuration

The request-context configuration defines how the broker derives caller IP metadata and whether it returns the
W3C `traceresponse` response header. The security-context capture itself is always enabled.

#### request_context.trusted_proxy.enabled

**Description**: Enables trusted-proxy caller-IP derivation.

**Valid Values**: `true`, `false`

**Default**: `false`

**Environment Variable**: `IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_ENABLED`

**CLI Flag**: None

**Behavior**:

- `false`: use the direct connection address and ignore client-supplied forwarding headers.
- `true`: inspect `request_context.trusted_proxy.forwarded_header` and use the right-most forwarded entry as the trusted caller IP.

#### request_context.trusted_proxy.forwarded_header

**Description**: Names the HTTP header that carries the forwarded caller-IP chain when trusted proxy mode is enabled.

**Default**: `X-Forwarded-For`

**Environment Variable**: `IDENTITY_BROKER_REQUEST_CONTEXT_TRUSTED_PROXY_FORWARDED_HEADER`

**CLI Flag**: None

**Notes**:

- This value applies only when `request_context.trusted_proxy.enabled=true`.
- The broker uses the right-most entry to prevent trust in attacker-controlled left-most
  values.

#### request_context.trace.response_enabled

**Description:** Controls the W3C `traceresponse` response header.

**Valid values:** `true`, `false`

**Default:** `true`

**Environment variable:** `IDENTITY_BROKER_REQUEST_CONTEXT_TRACE_RESPONSE_ENABLED`

**CLI flag:** None

**Notes:**

- When enabled, each HTTP response can contain
  `traceresponse: 00-<trace-id>-<child-id>-<flags>`.
- `<trace-id>` matches the `trace_id` in structured logs.
- Disabling this option removes only the response header. Trace capture and log correlation
  remain active.

**Example:**

```yaml
request_context:
  trusted_proxy:
    enabled: true
    forwarded_header: X-Forwarded-For
  trace:
    response_enabled: true
```


See [`examples/config/request-context.yaml`](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/examples/config/request-context.yaml) for a documented overlay example.

## Security Best Practices

### Sensitive Values

Any configuration key starting with `IDENTITY_BROKER_` or containing these keywords is considered sensitive and will be redacted in logs:

- `PASSWORD`
- `SECRET`
- `TOKEN`
- `KEY`
- `CREDENTIAL`
- `AUTH`

**Example**:

```bash
IDENTITY_BROKER_API_KEY=secret123
DB_PASSWORD=mypassword
```

The startup summary and audit logs show both values as `***REDACTED***`.

### Do not commit secrets

1. Use `.env.local` and `.env.{environment}.local` for local secrets. Git ignores these
   files.
2. Commit `.env.example` and `.env.production.example` as templates.
3. Use environment variables or secret-management systems in production.

### File permissions

Do not make configuration files world-readable:

```bash
# Recommended permissions
chmod 600 .env
chmod 600 config.yaml
```

### Environment Variable Injection

The system validates against command injection attempts. The following will be rejected:

```yaml
# REJECTED: Command substitution
log:
  level: $(malicious_command)

# REJECTED: Shell metacharacters
database:
  host: localhost; rm -rf /
```

## Troubleshooting

### Configuration Not Loading

**Symptom:** The application uses default values instead of your configuration.

**Solutions:**

1. Make sure that `.env` files are in the current directory. Or, use an absolute path.
2. Make sure that environment variable names start with `IDENTITY_BROKER_`.
3. Validate YAML syntax with `yamllint` or an online validator.
4. Use `--help` to make sure that flag names are correct.

### Undefined environment variable error

**Symptom:** The error message is "environment variable 'VAR_NAME' is not set".

**Cause:** The YAML file references `${VAR_NAME}`, but the environment has no variable with
that name.

**Solutions:**

1. Set the environment variable with `export VAR_NAME=value`.
2. Add `VAR_NAME=value` to `.env`.
3. Remove the `${}` YAML reference and use a literal value.

### Invalid configuration value

**Symptom:** The error message is "invalid value 'X' for field 'Y'".

**Cause:** The configuration value does not match its expected format or enum.

**Solutions:**

1. Read the error message for expected values. For example: "expected: debug, info, warn,
   or error".
2. Make sure that spelling and case are correct. Values are case-sensitive.
3. Remove extra whitespace or quotes from the value.

### Circular Reference Detected

**Symptom**: Error message: "circular reference detected: VAR1 → VAR2 → VAR1"

**Cause**: Environment variables reference each other in a loop.

**Solution**: Break the circular dependency:

```bash
# WRONG
VAR1=${VAR2}
VAR2=${VAR1}

# CORRECT
VAR1=value1
VAR2=${VAR1}
```

### Viewing Effective Configuration

To see configuration values and their sources:

```bash
agentic-identity-broker

# Output shows:
# === Configuration Summary ===
#   log.level: debug [source: cli]
#   log.format: json [source: yaml (/path/to/config.yaml)]
#
# === Configuration Sources ===
#   [0] default: defaults (loaded at ...)
#   [1] env_file: /path/to/.env (loaded at ...)
#   [2] yaml: /path/to/config.yaml (loaded at ...)
#   [3] cli: cli (loaded at ...)
```

### Audit Log

For compliance and troubleshooting, read the JSON audit log. It is the first startup output:

```json
{
  "timestamp": "2025-12-15T09:00:00Z",
  "event": "configuration_loaded",
  "sources": [...],
  "keys": ["log.level", "log.format"],
  "redacted_keys": []
}
```

### OAuth2 Authorization Server Configuration

#### oauth2_authorization_server.multi_agent_client

**Description:** Controls whether multiple agents share one upstream OAuth2 `client_id`.
The broker stores this value as `agent.client_id`. When disabled, the broker requires each
agent upstream `client_id` to be unique. It always resolves an incoming OAuth2 request with
the internal UUID in `agent.id`. When enabled, agents can share one upstream OAuth2
application. The broker validates the agent identity with a custom claim in the upstream
authorization redirect and token response.

**Configuration block:** Nested under `oauth2_authorization_server`.

| Option | Type | Default | Valid Values | Required | Description |
|--------|------|---------|--------------|----------|-------------|
| `multi_agent_client.enabled` | boolean | `false` | `true`, `false` | No | Allow multiple agents to share one upstream OAuth2 `client_id`. When `false`, duplicate `client_id` on agent creation or update returns `409 Conflict`. |
| `multi_agent_client.agent_id_param_name` | string | — | Any URL-safe string | Yes if `enabled=true` | Query parameter on the upstream authorization redirect that contains the agent internal UUID. It must match the upstream OAuth2 claim name. |
| `multi_agent_client.agent_id_claim_name` | string | — | Any string | Yes if `enabled=true` | JWT claim in the upstream token response that contains the agent internal UUID. The broker validates it on each token proxy response. A missing claim is rejected. |

**Startup validation:** If `enabled = true` and `agent_id_param_name` or
`agent_id_claim_name` is empty, the broker does not start.

**Feature disabled (default)**:

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: "https://auth.example.com"
  upstream_token_endpoint: "https://auth.example.com/token"
  public_base_url: "https://broker.example.com"

  multi_agent_client:
    enabled: false   # default — each agent must have a unique client_id
```

**Feature enabled**:

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: "https://auth.example.com"
  upstream_token_endpoint: "https://auth.example.com/token"
  public_base_url: "https://broker.example.com"

  multi_agent_client:
    enabled: true
    agent_id_param_name: "x_agent_id"   # injected into authorize redirect as ?x_agent_id=<agent.id>
    agent_id_claim_name: "x_agent_id"   # expected in upstream token response JWT
```

**Token exchange CEL expression**: Set `token_exchange.claim_extraction.agent_id_expression` according to `multi_agent_client.enabled`:

| `multi_agent_client.enabled` | CEL expression | Purpose |
|---|---|---|
| `false` (default) | `resolveAgentIdByClientId(subject_token.azp)` | Maps the upstream `client_id` to `agent.id`. |
| `true` | `subject_token.x_agent_id` (use your `agent_id_claim_name`) | Reads the agent UUID from the token claim. |

See `examples/config/oauth2-authorization-server.yaml` for a complete configuration example with both modes commented.

### Token Exchange Client Assertion Configuration

#### token_exchange.client_assertion

**Description**: Configures the external identity-provider trust anchor used to validate privileged-gateway `client_assertion` JWTs for RFC 8693 token exchange and machine-facing approval authentication. Broker-minted tokens are never valid client assertions.

| Option | Type | Default | Required? | Environment Variable | Rules |
|--------|------|---------|-----------|----------------------|-------|
| `token_exchange.client_assertion.issuer_uri` | string | Proxy upstream issuer | Yes for configured token exchange in `local` mode | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_ISSUER_URI` | External IdP issuer URI. In `proxy` and `hybrid` modes, an empty value defaults to `oauth2_authorization_server.proxy.upstream_issuer_uri`. In `local` mode it must be explicit. The broker's own issuer is rejected at startup. |
| `token_exchange.client_assertion.jwks_uri` | string | Discovered from `issuer_uri` | No | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_URI` | Explicit external IdP JWKS endpoint. Set this when the IdP does not support OAuth/OIDC discovery. |
| `token_exchange.client_assertion.jwks_min_refresh` | duration | `15m` | No | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_MIN_REFRESH` | Minimum interval between client-assertion JWKS refresh attempts. Must be a non-negative Go duration. |
| `token_exchange.client_assertion.jwks_max_refresh` | duration | `max(jwks_min_refresh, 1h)` | No | `IDENTITY_BROKER_TOKEN_EXCHANGE_CLIENT_ASSERTION_JWKS_MAX_REFRESH` | Maximum client-assertion JWKS refresh interval. Must be a non-negative Go duration; values below the effective minimum are raised to it. |

**Example**:

```yaml
token_exchange:
  client_assertion:
    issuer_uri: "https://privileged-gateway-idp.example.com"
    # Optional when discovery is unavailable:
    # jwks_uri: "https://privileged-gateway-idp.example.com/keys"
    # jwks_min_refresh: "15m"
    # jwks_max_refresh: "1h"
```

In `local` mode, set `issuer_uri` whenever token exchange or approval authentication is configured. In `proxy` and `hybrid` modes it defaults to the configured proxy upstream issuer when omitted. The configured issuer must be an external IdP; a broker-self issuer is rejected at startup to prevent broker-minted tokens from becoming privileged client assertions.

### OAuth2 Server Mode Configuration

#### oauth2_authorization_server

**Description**: Controls the broker's OAuth2 operating mode. Three symmetric modes are supported:

- **`proxy`**: OAuth2 requests are forwarded to an upstream authorization server. The broker acts as a transparent proxy — it handles consent and delegation, then routes the final authorization to the upstream. No local token issuance; discovery and `jwks_uri` remain broker-hosted using upstream verification keys.
- **`local`**: The broker acts as a standalone OAuth2 authorization server, minting its own JWT access tokens signed with managed asymmetric keys. Supports `client_credentials` and `authorization_code` (with PKCE) grant types, and exposes RFC 8414 discovery and JWKS endpoints.
- **`hybrid`**: Both proxy and local paths coexist. Agents are classified by their properties: agents with an upstream `ClientID` are routed to the proxy path; local agents (no `ClientID`, no `client_uris`) and CIMD agents (`client_uris` set) are issued local tokens. Requires both `proxy` and `local` configuration sections.

**Configuration block** (nested under `oauth2_authorization_server`):

| Option | Type | Default | Valid Values | Required? | Environment Variable | CLI Flag | Description |
|--------|------|---------|--------------|-----------|----------------------|----------|-------------|
| `oauth2_authorization_server.mode` | enum | — | `proxy`, `local`, `hybrid` | Yes (if any auth server field is set) | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE` | — | Operating mode. `proxy` forwards to upstream; `local` mints tokens locally; `hybrid` supports both. |
| `oauth2_authorization_server.proxy.upstream_issuer_uri` | string | — | Valid HTTPS URI | Yes (if `proxy` or `hybrid`) | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_ISSUER_URI` | — | Upstream OAuth2 issuer URI. Used for proxy path routing. |
| `oauth2_authorization_server.proxy.upstream_authorize_endpoint` | string | — | Valid HTTPS URI | Yes (if `proxy` or `hybrid`) | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_AUTHORIZE_ENDPOINT` | — | Upstream authorization endpoint. |
| `oauth2_authorization_server.proxy.upstream_token_endpoint` | string | — | Valid HTTPS URI | Yes (if `proxy` or `hybrid`) | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_TOKEN_ENDPOINT` | — | Upstream token endpoint. |
| `oauth2_authorization_server.proxy.upstream_jwks_min_refresh` | duration | `15m` | Go duration (e.g. `30s`, `5m`, `1h`) | No | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_JWKS_MIN_REFRESH` | — | Minimum interval between upstream JWKS refresh attempts. Applies a floor to the cache cadence derived from upstream cache headers. |
| `oauth2_authorization_server.proxy.upstream_jwks_max_refresh` | duration | `1h` | Go duration (e.g. `5m`, `30m`, `2h`) | No | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_PROXY_UPSTREAM_JWKS_MAX_REFRESH` | — | Maximum interval between upstream JWKS refresh attempts. Caps how stale the broker will allow upstream JWKS cache entries to become. |
| `oauth2_authorization_server.local.token_ttl` | duration | `1h` | Go duration (e.g. `30m`, `2h`) | No | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_TTL` | — | Validity period for locally issued JWT access tokens. |
| `oauth2_authorization_server.local.token_claims_expression` | string | `""` | CEL expression | No | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_CLAIMS_EXPRESSION` | — | CEL expression to inject custom claims into issued JWTs. |
| `oauth2_authorization_server.local.signing_keys.bootstrap_timeout` | duration | `90s` | Positive duration | No | `IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_SIGNING_KEYS_BOOTSTRAP_TIMEOUT` | — | Startup budget for signing-key bootstrap in local/hybrid mode. Covers advisory locking, key generation, encryption, and persistence. |

**Required**: `oauth2_authorization_server.mode` is mandatory — the broker rejects startup when the block is absent or `mode` is empty or invalid. Set `mode` to `proxy`, `local`, or `hybrid` before deploying.

**Startup validation**: The broker validates configuration at startup and rejects incompatible combinations — proxy-only fields in local mode, local-only fields in proxy mode, or missing sections in hybrid mode.

**Proxy mode**:

```yaml
oauth2_authorization_server:
  mode: "proxy"
  proxy:
    upstream_issuer_uri: "https://auth.example.com"
    upstream_authorize_endpoint: "https://auth.example.com/oauth/authorize"
    upstream_token_endpoint: "https://auth.example.com/oauth/token"
    upstream_jwks_min_refresh: "15m"
    upstream_jwks_max_refresh: "1h"
```

**Local mode**:

```yaml
oauth2_authorization_server:
  mode: "local"
  local:
    token_ttl: "1h"
    token_claims_expression: '{"team": agent.display_name}'
    signing_keys:
      bootstrap_timeout: 90s
```

**Hybrid mode** (proxy and local agents coexist):

```yaml
oauth2_authorization_server:
  mode: "hybrid"
  proxy:
    upstream_issuer_uri: "https://auth.example.com"
    upstream_authorize_endpoint: "https://auth.example.com/oauth/authorize"
    upstream_token_endpoint: "https://auth.example.com/oauth/token"
    upstream_jwks_min_refresh: "15m"
    upstream_jwks_max_refresh: "1h"
  local:
    token_ttl: "1h"
    signing_keys:
      bootstrap_timeout: 90s
```

**Security notes**:

- PKCE is always enforced (S256 only) for authorization code grants. No plaintext challenge method.
- Client secrets are hashed with Argon2id and never stored in plaintext.
- Signing key private material is encrypted at rest via `EncryptionPort`.
- Authorization codes are single-use with 60-second TTL, stored as SHA-256 hashes.
- Signing key decryption failure prevents token issuance (fail-closed).
- In `proxy` mode, locally registered agents (no `ClientID`) are rejected. In `local` mode, proxy agents (with `ClientID`) are rejected. Mode boundaries are strict.

See [OAuth2 server modes](./concepts/oauth2-server-modes.md) for how the broker issues or proxies tokens.

### OAuth2 User Impersonation Configuration

#### oauth2_authorization_server.impersonation

**Description**: Configures RFC 8693 user impersonation on `POST /oauth2/token`. In `local` mode only, exactly one request `audience` of `<impersonation.audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>` selects a registered target agent before the third-party `resource` guard. Both forms resolve the same target and retain its UUID as minted `agent_id`; the target supplies local-token CEL `agent.*`, while the validated client assertion remains privileged-client authorization and audit material. Every rule-authorized request must also have an active `UserGrant` for the extracted subject identity and selected target agent. The extracted subject is used unchanged as the grant principal; this is mandatory for signed and unverified subjects, with no configuration opt-out. A missing or expired grant returns generic `403 access_denied` plus `error_uri=<server.enduser.public_url>/agents/<target-agent-id>` so the privileged client can direct the authenticated user to the existing consent-management page. A grant lookup failure returns `500 server_error` without `error_uri`. The routing prefix is never copied to an issued `aud`: `token_claims_expression` controls `aud`, including omission, exactly as normal local issuance. The subject role supports signed (`verification: jwks`) and broker-profile unverified (`verification: none`, ADR 031) JWTs.

**Configuration block** (nested under `oauth2_authorization_server`, `local` mode only):

| Option | Type | Default | Valid Values | Required | Description |
|--------|------|---------|--------------|----------|-------------|
| `impersonation.audience_prefix` | URI | — | Absolute HTTP(S) URI; host required; no userinfo/query/fragment/trailing slash | Yes (CR-001) | Routing-only prefix. Exactly one `<audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>` selects a registered target agent; it never sets the issued token `aud`. |
| `impersonation.rules` | list | — | Non-empty, ordered | Yes (CR-002) | Ordered rules evaluated first-match. |
| `impersonation.rules[].name` | string | — | Unique across rules | Yes (CR-003) | Operator-facing rule identifier. |
| `impersonation.rules[].roles` | map | — | Keys `client_assertion`, `actor`, `subject` | Yes (CR-003) | Role semantics keyed by role name; all three roles MUST be defined. |
| `impersonation.rules[].roles.<role>.expected_audience` | string | — | Non-empty string | Yes for signed roles without `audience_requirement: absent` | The `aud` each signed credential for this role must carry. Omit it for an unverified subject (`verification: none`) or when `audience_requirement` is `absent`. |
| `impersonation.rules[].roles.<role>.audience_requirement` | enum | — | `absent` | No | Signed roles only. `absent` accepts only a credential with no `aud` claim and rejects a credential with an `aud`. It is mutually exclusive with `expected_audience`, forbidden on an unverified subject, and requires the authorization predicate to reference this role (CR-009). |
| `impersonation.rules[].roles.<role>.principal_expression` | string | — | CEL expression | Yes | CEL extracting a non-empty identity from the role's token. |
| `impersonation.rules[].roles.subject.verification` | enum | `jwks` | `jwks`, `none` | No | `subject` role only. `jwks` = signed subject validated against a trusted issuer; `none` = unverified unsigned `alg:none` subject JWT under `subject_token_type=urn:ietf:params:oauth:token-type:jwt` (broker profile extension, ADR 031). |
| `impersonation.rules[].roles.subject.email_expression` | string | `""` | CEL expression | No | CEL extracting the subject's email; `subject` role only. For an unverified subject the email is minted only when the authorization predicate binds `subject_token.email` (FR-006a). Omitted from the issued token when it evaluates empty. |
| `impersonation.rules[].trusted_issuers` | list | — | Non-empty | Yes (CR-008) | Trust anchors permitted to sign this rule's credentials. |
| `impersonation.rules[].trusted_issuers[].issuer_uri` | string | — | Valid URI | Yes | Matched against a credential's `iss`. |
| `impersonation.rules[].trusted_issuers[].jwks_uri` | string | Discovered from `issuer_uri` | Valid URI | No | Explicit JWKS endpoint; discovered from issuer metadata when empty. |
| `impersonation.rules[].trusted_issuers[].jwks_min_refresh` | duration | `15m` | Non-negative Go duration | No | Minimum interval between JWKS refresh attempts. |
| `impersonation.rules[].trusted_issuers[].jwks_max_refresh` | duration | `max(jwks_min_refresh, 1h)` | Non-negative Go duration | No | Maximum JWKS refresh interval; values below the effective minimum are raised to it. |
| `impersonation.rules[].trusted_issuers[].allowed_algorithms` | list | — | Subset of `RS256,RS384,RS512,PS256,PS384,PS512,ES256,ES384,ES512,EdDSA` | Yes (CR-007) | Accepted signing algorithms; `none` and `HS*` are rejected. |
| `impersonation.rules[].trusted_issuers[].signs_roles` | list | — | Subset of defined signed roles | Yes (CR-008) | Which signed roles this issuer may sign. |
| `impersonation.rules[].authorization.type` | enum | — | `cel` | Yes (CR-005) | Authorization predicate type; reuses the token-exchange schema. |
| `impersonation.rules[].authorization.cel.expression` | string | — | CEL expression | Yes (CR-005) | Predicate deciding the request; compiled at startup. |
| `impersonation.rules[].authorization.cel.evaluation_timeout` | duration | `100ms` | `10ms`–`5s` | No (CR-005) | Per-evaluation CEL timeout. |

**Startup validation:** Each failure returns a `ConfigError` with an indexed field path. For
example: `oauth2_authorization_server.impersonation.rules[1].trusted_issuers[0].allowed_algorithms`.

- **CR-001:** `audience_prefix` is an absolute HTTP(S) routing URI. It has a host and no
  userinfo, query, fragment, or trailing slash. One exact
  `<audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>` enables
  impersonation. A bare or invalid suffix is `invalid_request`. An unknown valid target is
  `invalid_target`.
- **CR-002:** `rules` is non-empty and ordered. The broker evaluates the first matching rule.
- **CR-003:** Each rule has a unique `name`. Its `roles` keys are a subset of
  `{client_assertion, actor, subject}` and define all three roles. Each role requires
  `principal_expression`. Signed roles require `expected_audience`. An unverified subject
  must not use it. Only `subject` can use `verification` and `email_expression`.
- **CR-004:** The local `issuer_uri` must not be a trusted issuer for `client_assertion`.
- **CR-005:** `authorization.type` is `cel`. Its expression is required and compiles at
  startup. `authorization.cel.evaluation_timeout` must be 10ms through 5s. The default is
  100ms.
- **CR-005a:** Unverified subject mode is disabled unless a rule declares
  `verification: none`. Its authorization predicate must reference `subject_token`. The
  broker checks this at startup. It can issue a caller-provided email only when the predicate
  binds `subject_token.email`.
- **CR-006:** The broker rejects `impersonation` outside local mode.
- **CR-007:** `allowed_algorithms` is a non-empty subset of
  `{RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512, EdDSA}`. The broker
  rejects `none` and `HS*`. This rule governs signed credentials only. It does not validate
  the unsigned unverified subject.
- **CR-008:** Within a rule, `issuer_uri` is unique across `trusted_issuers`. Every
  `signs_roles` entry is a defined signed role. At least one trusted issuer covers every
  signed role: `client_assertion`, `actor`, and a signed `subject`. An unverified subject
  must not appear in `signs_roles`. The same `issuer_uri` can appear in different rules.
- **CR-010:** Impersonation requires the user-delegation verifier and
  `server.enduser.public_url`. The broker does not start without both. It has no bypass
  mode.

**Example** (`local` mode; the same issuer signs the credentials of two signed rules):

```yaml
oauth2_authorization_server:
  mode: "local"
  local:
    issuer_uri: "https://broker.example.com"
    token_ttl: "1h"
    # Routing never sets aud. The existing local token policy may emit it.
    token_claims_expression: '{"aud": "https://policy.example.com"}'
  impersonation:
    audience_prefix: "https://broker.example.com/impersonation"
    rules:
      - name: "internal-gateway-signed-subject"
        roles:
          client_assertion:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "client_assertion.sub"
          actor:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "actor_token.sub"
          subject:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "subject_token.sub"
            email_expression: "has(subject_token.email) ? subject_token.email : ''"
        trusted_issuers:
          - issuer_uri: "https://idp.internal.example.com"
            jwks_min_refresh: "15m"
            jwks_max_refresh: "1h"
            allowed_algorithms: ["ES256", "RS256"]
            signs_roles: ["client_assertion", "actor", "subject"]
        authorization:
          type: cel
          cel:
            expression: |
              client_assertion.iss == "https://idp.internal.example.com" &&
              client_assertion.sub in ["gateway-prod", "gateway-staging"]
            evaluation_timeout: "100ms"
      - name: "partner-gateway-signed-subject"
        roles:
          client_assertion:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "client_assertion.sub"
          actor:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "actor_token.sub"
          subject:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "subject_token.sub"
        trusted_issuers:
          - issuer_uri: "https://idp.internal.example.com"
            jwks_min_refresh: "15m"
            jwks_max_refresh: "1h"
            allowed_algorithms: ["ES256", "RS256"]
            signs_roles: ["client_assertion", "actor", "subject"]
        authorization:
          type: cel
          cel:
            expression: |
              client_assertion.sub == "partner-gateway-prod"
            evaluation_timeout: "100ms"
```

**Request contract** (`POST /oauth2/token`): Send
`grant_type=urn:ietf:params:oauth:grant-type:token-exchange` and one
`audience=<impersonation.audience_prefix>/<canonical lower-case AgentID UUID or canonical_id>`.
Send client assertion, actor, and subject credentials. The suffix must identify a registered
target agent. `requested_token_type` is optional. When present, it must be the access-token
type. `resource` must be absent. `scope` is optional and uses literal-space-separated values.
Each non-reserved scope must be in the resolved target `allowed_scopes`. An empty allow list
permits all non-reserved scopes. `offline` and `offline_access` are reserved and permitted.
The response and minted token contain a non-empty granted scope. The target UUID and agent
always supply `agent_id` and local CEL `agent.*`, regardless of identifier form. Local policy
controls `aud`.

**Unverified subject rule** (broker profile extension, ADR 031): The subject role uses
`verification: none` and has no `expected_audience`. It does not occur in any issuer
`signs_roles`. The authorization predicate must reference `subject_token`. It must bind
`subject_token.email` before the broker mints a caller-provided email:

```yaml
      - name: "chat-bridge-unverified-subject"
        roles:
          client_assertion:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "client_assertion.sub"
          actor:
            expected_audience: "https://broker.example.com/impersonation"
            principal_expression: "actor_token.sub"
          subject:
            verification: "none"
            principal_expression: "subject_token.sub"
            email_expression: "has(subject_token.email) ? subject_token.email : ''"
        trusted_issuers:
          - issuer_uri: "https://idp.internal.example.com"
            allowed_algorithms: ["ES256", "RS256"]
            signs_roles: ["client_assertion", "actor"]
        authorization:
          type: cel
          cel:
            expression: |
              client_assertion.sub == "chat-bridge" &&
              subject_token.sub != "" &&
              has(subject_token.email) && subject_token.email.endsWith("@example.com")
            evaluation_timeout: "100ms"
```

For the unverified subject, send `subject_token_type=urn:ietf:params:oauth:token-type:jwt` and `subject_token=<unsigned alg:none JWT carrying sub and optional email>`; all other request parameters are identical to the signed contract above.

## Observability / OpenTelemetry

The Identity Broker supports configurable OpenTelemetry (OTel) tracing, metrics, and logging export via OTLP (gRPC or HTTP). When disabled (default), the OTel SDK is never initialized and there is zero overhead.

### Quickstart

See `examples/config/telemetry.yaml` for a complete production-ready example. The minimal configuration to enable tracing:

```yaml
telemetry:
  enabled: true
  service_name: agentic-identity-broker
  exporter:
    protocol: grpc
    endpoint: otel-collector:4317
    insecure: true   # only for non-production environments
```

### OTel Configuration Reference

| Parameter | Type | Default | Environment Variable | Description |
|---|---|---|---|---|
| `telemetry.enabled` | bool | `false` | `IDENTITY_BROKER_TELEMETRY_ENABLED` | Master switch. When false, no OTel SDK is initialized and overhead is zero. |
| `telemetry.service_name` | string | `"agentic-identity-broker"` | `IDENTITY_BROKER_TELEMETRY_SERVICE_NAME` | Value for the `service.name` OTel resource attribute. Appears on all spans, metrics, and logs. |
| `telemetry.resource_attributes` | map[string]string | `{}` | N/A (config file only) | Additional OTel resource attributes added to every telemetry signal (e.g., `deployment.environment: production`). |
| `telemetry.traces.enabled` | bool | `true` (when telemetry.enabled) | `IDENTITY_BROKER_TELEMETRY_TRACES_ENABLED` | Enable trace export. Disabling traces suppresses otelchi HTTP spans and storage child spans. |
| `telemetry.traces.sampling_rate` | float64 | `1.0` | `IDENTITY_BROKER_TELEMETRY_TRACES_SAMPLING_RATE` | Fractional sampling rate for traces (0.0–1.0). `1.0` = 100% sampled. Uses `ParentBased(TraceIDRatioBased(rate))`. |
| `telemetry.traces.propagators` | []string | `["ottrace","b3multi","baggage"]` | N/A (config file only) | Propagators to register globally. Supported values: `ottrace` (OpenTracing interop), `b3multi` (Zipkin B3 multiple headers), `b3` (B3 single header), `tracecontext` (W3C), `baggage` (W3C). Propagators are registered unconditionally when telemetry is enabled, even if `traces.enabled` is false. |
| `telemetry.metrics.enabled` | bool | `true` (when telemetry.enabled) | `IDENTITY_BROKER_TELEMETRY_METRICS_ENABLED` | Enable metrics export. When enabled, process runtime metrics (goroutines, memory, GC) and HTTP request metrics (via otelchi) are exported automatically. |
| `telemetry.metrics.export_interval` | duration | `30s` | `IDENTITY_BROKER_TELEMETRY_METRICS_EXPORT_INTERVAL` | How often metrics are pushed to the collector. Must be positive. Example: `30s`, `1m`. |
| `telemetry.logs.enabled` | bool | `true` (when telemetry.enabled) | `IDENTITY_BROKER_TELEMETRY_LOGS_ENABLED` | Enable OTLP log export via the `slog` bridge. Set to `false` if the collector does not support `opentelemetry.proto.collector.logs.v1.LogsService`. |
| `telemetry.exporter.protocol` | string | `"grpc"` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_PROTOCOL` | OTLP transport protocol. Accepted values: `grpc`, `http`, `https`. The `https` protocol uses the HTTP OTLP exporter with TLS; bare `host:port` endpoints are auto-prefixed with `https://`. |
| `telemetry.exporter.endpoint` | string | `""` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT` | OTLP collector endpoint. **Required when `telemetry.enabled=true`**. Format: `host:port` for gRPC, `http(s)://host:port` for HTTP/HTTPS. |
| `telemetry.exporter.headers` | map[string]string | `{}` | N/A (config file only) | Additional HTTP/gRPC headers sent with every export request (e.g., authentication tokens). Use `${ENV_VAR}` substitution to avoid committing secrets. |
| `telemetry.exporter.timeout` | duration | `10s` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_TIMEOUT` | Per-export request timeout. Must be positive. Example: `5s`, `30s`. |
| `telemetry.exporter.compression` | string | `"none"` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_COMPRESSION` | Payload compression for all OTLP exporters. Accepted values: `none`, `gzip`. |
| `telemetry.exporter.insecure` | bool | `false` | `IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE` | Disable TLS for the OTLP exporter. **Do not use in production** — a startup warning is emitted when this is true. Only meaningful for gRPC; for HTTP the URL scheme controls TLS. |

### Environment Variable Mapping

All OTel settings can be overridden via environment variables using the `IDENTITY_BROKER_TELEMETRY_` prefix, following the same Viper binding rules as other settings. For example:

```bash
IDENTITY_BROKER_TELEMETRY_ENABLED=true
IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT=otel-collector:4317
IDENTITY_BROKER_TELEMETRY_EXPORTER_INSECURE=true
IDENTITY_BROKER_TELEMETRY_SERVICE_NAME=my-broker-instance
```

### What is Instrumented

When tracing is enabled, the following operations emit child spans:

| Span Name | Operation | Key Attributes |
|---|---|---|
| HTTP span (per route) | All inbound HTTP requests (via otelchi) | `http.method`, `http.route`, `http.status_code` |
| `storage.get.agent` | PostgreSQL `AgentRepository.Get` | `db.system=postgresql`, `db.operation=GetAgent` |
| `storage.list.userGrants` | PostgreSQL `UserGrantRepository.ListByPrincipal` | `db.system=postgresql` |
| `storage.upsert.userGrant` | PostgreSQL `UserGrantRepository.Create` (upsert) | `db.system=postgresql` |
| `storage.get.thirdPartyService` | PostgreSQL `ThirdpartyServiceRepository.Get` | `db.system=postgresql` |
| `encryption.encrypt` | AWS KMS envelope encryption | `encryption.key_type=aws_kms` |
| `encryption.decrypt` | AWS KMS envelope decryption | `encryption.key_type=aws_kms` |
| `jwks.fetch` | JWKS key set fetch/cache lookup | `url.full` |
| `oauth2.token_exchange` | Upstream OAuth2 token proxy | `http.method=POST`, `http.status_code` |

### Security notes

- `telemetry.exporter.insecure` defaults to `false`. TLS is required by default. The broker
  records a startup warning when this value is true.
- Span attributes do not contain tokens, encryption keys, PII, credentials, or request body
  content.
- Use `${VAR_NAME}` substitution for `telemetry.exporter.headers` values such as API keys.
  This prevents secrets from entering configuration files.

## Getting help

- Read error messages. They include correction instructions.
- Read the startup summary to identify loaded sources.
- Read `ARCHITECTURE.md` for the configuration subsystem architecture.
- Make sure that each configuration path is absolute or relative to the current working directory.
- Make sure that `GO_ENV` has the expected environment value.
- Read `adrs/002-configuration-libraries.md` for the configuration library decision.
