# Configuration Suggestions: Dual-Port HTTP Server

**Feature**: 20251215-120000-dual-port-server
**Created**: 2025-12-15

This document provides concrete configuration examples for the dual-port HTTP server feature, following the existing configuration patterns established in 20251214-120100-flexible-configuration.

## Configuration Schema

### YAML Configuration

Add the following structure to `config.yaml`:

```yaml
server:
  enduser:
    port: 8000              # Default: 8000
    bind: "0.0.0.0"         # Default: "0.0.0.0" (all interfaces)
  admin:
    port: 14000             # Default: 14000
    bind: "0.0.0.0"         # Default: "0.0.0.0" (all interfaces)
  shutdown:
    timeout: 30s            # Default: 30s (graceful shutdown timeout)
```

### Environment Variables

Following the existing `IDENTITY_BROKER_` prefix pattern:

```bash
# End-user server configuration
IDENTITY_BROKER_SERVER_ENDUSER_PORT=8000
IDENTITY_BROKER_SERVER_ENDUSER_BIND="0.0.0.0"

# Admin server configuration
IDENTITY_BROKER_SERVER_ADMIN_PORT=14000
IDENTITY_BROKER_SERVER_ADMIN_BIND="0.0.0.0"

# Shutdown configuration
IDENTITY_BROKER_SERVER_SHUTDOWN_TIMEOUT=30s
```

### Command-Line Flags

Following Go flag conventions:

```bash
agentic-identity-broker \
  --server.enduser.port 8000 \
  --server.enduser.bind "0.0.0.0" \
  --server.admin.port 14000 \
  --server.admin.bind "0.0.0.0" \
  --server.shutdown.timeout 30s
```

## Example Configurations

### Development Environment

**File**: `examples/config/config.development.yaml`

```yaml
log:
  level: debug
  format: text

server:
  enduser:
    port: 3000           # Non-privileged port for local development
    bind: "127.0.0.1"    # Localhost only for security
  admin:
    port: 3001           # Adjacent port for easy development
    bind: "127.0.0.1"    # Localhost only for security
  shutdown:
    timeout: 5s          # Short timeout for faster development cycles
```

### Staging Environment

**File**: `examples/config/config.staging.yaml`

```yaml
log:
  level: info
  format: json

server:
  enduser:
    port: 8000
    bind: "0.0.0.0"      # All interfaces for external access
  admin:
    port: 14000
    bind: "10.0.1.0"     # Internal network only
  shutdown:
    timeout: 30s
```

### Production Environment

**File**: `examples/config/config.production.yaml`

```yaml
log:
  level: warn
  format: json

server:
  enduser:
    port: 8000
    bind: "0.0.0.0"      # All interfaces (behind load balancer)
  admin:
    port: 14000
    bind: "10.0.0.0"     # Private subnet only for admin access
  shutdown:
    timeout: 60s         # Longer timeout for production safety
```

### Environment Variable Override Example

Using environment variables to override YAML configuration:

```bash
# Base configuration from config.yaml
# Override only the admin port for a specific deployment

export IDENTITY_BROKER_SERVER_ADMIN_PORT=15000
agentic-identity-broker --config /etc/agentic-identity-broker/config.yaml
```

### CLI Flag Override Example

Using command-line flags for temporary overrides:

```bash
# Start with custom ports for testing
agentic-identity-broker \
  --config /etc/agentic-identity-broker/config.yaml \
  --server.enduser.port 9000 \
  --server.admin.port 9001
```

## Security Recommendations

### Bind Address Patterns

| Environment | End-User Bind | Admin Bind | Rationale |
|-------------|---------------|------------|-----------|
| Development | 127.0.0.1 | 127.0.0.1 | Maximum isolation, local testing only |
| Staging | 0.0.0.0 | [private-ip] | End-user accessible, admin restricted to internal network |
| Production | 0.0.0.0 | [private-ip] | End-user behind load balancer, admin on private subnet |

### Port Selection Guidelines

| Port Range | Usage | Notes |
|------------|-------|-------|
| 1-1023 | Privileged ports | Requires root/admin privileges on Unix systems |
| 1024-49151 | Registered ports | Recommended for services, less likely to conflict |
| 49152-65535 | Dynamic/private ports | May conflict with ephemeral ports |

**Recommended Defaults**:
- End-user: **8000** (common for HTTP services, non-privileged)
- Admin: **14000** (clearly separated, non-privileged, unlikely to conflict)

### Firewall Configuration

Ensure firewall rules match your bind configuration:

```bash
# Example: iptables rules for production
# Allow end-user traffic from anywhere
iptables -A INPUT -p tcp --dport 8000 -j ACCEPT

# Allow admin traffic only from internal network (10.0.0.0/8)
iptables -A INPUT -p tcp --dport 14000 -s 10.0.0.0/8 -j ACCEPT
iptables -A INPUT -p tcp --dport 14000 -j DROP
```

## Configuration Validation

The system will validate:

1. **Port Range**: Ports must be between 1-65535
2. **Port Uniqueness**: End-user and admin ports must differ
3. **Bind Address**: Must be a valid IP address or hostname
4. **Timeout Format**: Must be a valid duration (e.g., "30s", "1m", "90s")

Invalid configurations will fail at startup with descriptive error messages before attempting to bind to network interfaces.

## Precedence Examples

Given this scenario:

**config.yaml**:
```yaml
server:
  enduser:
    port: 8000
```

**Environment**:
```bash
export IDENTITY_BROKER_SERVER_ENDUSER_PORT=9000
```

**CLI**:
```bash
agentic-identity-broker --server.enduser.port 10000
```

**Result**: End-user server binds to port **10000** (CLI takes highest precedence)

**Effective Configuration**: CLI (10000) > ENV (9000) > YAML (8000) > Default (8000)
