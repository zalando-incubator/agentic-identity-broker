# Quickstart: Dual-Port HTTP Server

**Feature**: 20251215-120000-dual-port-server | **Date**: 2025-12-15

## Overview

This guide helps developers quickly set up and test the dual-port HTTP server feature. By the end, you'll have two independent HTTP servers running (end-user on port 8000, admin on port 14000) with health check endpoints.

## Prerequisites

- Go 1.23.0 or later
- Network access to ports 8000 and 14000
- IPv6-capable system (recommended, falls back to IPv4 automatically)

## Installation

### 1. Install Dependencies

```bash
# From repository root
go get github.com/go-chi/chi/v5
go get golang.org/x/sync/errgroup
go mod tidy
```

### 2. Verify Installation

```bash
# Check Go version
go version  # Should be 1.23.0+

# Verify dependencies
go list -m github.com/go-chi/chi/v5
go list -m golang.org/x/sync/errgroup
```

## Configuration

### Default Configuration

Create or update `config.yaml` in the repository root:

```yaml
log:
  level: info
  format: text

server:
  enduser:
    port: 8000
    bind: "::"  # Dual-stack (IPv6 + IPv4)
  admin:
    port: 14000
    bind: "::"
  shutdown:
    timeout: 30s
```

### Environment Variable Override

```bash
# Override specific settings
export IDENTITY_BROKER_SERVER_ENDUSER_PORT=3000
export IDENTITY_BROKER_SERVER_ADMIN_PORT=3001
export IDENTITY_BROKER_LOG_LEVEL=debug
```

### CLI Flag Override

```bash
# Override at runtime
./bin/agentic-identity-broker \
  --server.enduser.port 9000 \
  --server.admin.port 9001 \
  --log.level debug
```

## Running the Server

### Development Mode

```bash
# Hot-reload with Air
just dev

# Or build and run
just run
```

### Manual Build and Run

```bash
# Build binary
just build

# Run with default config
./bin/agentic-identity-broker

# Run with custom config
./bin/agentic-identity-broker --config ./examples/config/config.development.yaml

# Run with CLI overrides
./bin/agentic-identity-broker \
  --server.enduser.port 8080 \
  --server.admin.port 14001 \
  --log.level debug
```

## Testing

### Verify Server Startup

#### Check Logs

```bash
# Should see:
# [INFO] Starting enduser server on [::]:8000
# [INFO] Starting admin server on [::]:14000
# [INFO] Both servers started successfully
```

#### Test Health Endpoints

**End-User Server**:
```bash
curl http://localhost:8000/health

# Expected response (HTTP 200):
{
  "status": "healthy",
  "server": "enduser",
  "timestamp": "2025-12-15T10:30:00Z",
  "uptime_seconds": 60
}
```

**Admin Server**:
```bash
curl http://localhost:14000/health

# Expected response (HTTP 200):
{
  "status": "healthy",
  "server": "admin",
  "timestamp": "2025-12-15T10:30:00Z",
  "uptime_seconds": 60
}
```

### Test IPv4 Connectivity

```bash
# Explicitly connect via IPv4
curl http://127.0.0.1:8000/health
curl http://127.0.0.1:14000/health
```

### Test IPv6 Connectivity

```bash
# Explicitly connect via IPv6
curl http://[::1]:8000/health
curl http://[::1]:14000/health
```

### Test Graceful Shutdown

```bash
# Terminal 1: Start server
./bin/agentic-identity-broker

# Terminal 2: Send SIGTERM
kill -TERM $(pgrep agentic-identity-broker)

# Check logs:
# [INFO] Received shutdown signal
# [INFO] Gracefully shutting down servers...
# [INFO] EndUser server shutdown complete
# [INFO] Admin server shutdown complete
# [INFO] Application stopped
```

### Test Atomic Startup Failure

**Scenario**: Admin port already in use

```bash
# Terminal 1: Occupy port 14000
nc -l 14000

# Terminal 2: Try to start server (should fail)
./bin/agentic-identity-broker

# Expected logs:
# [ERROR] Failed to start admin server: bind: address already in use
# [ERROR] Shutting down enduser server due to startup failure
# [FATAL] Server startup failed: admin server bind error
# Exit code: 1
```

## Common Tasks

### Change Ports

**YAML Configuration**:
```yaml
server:
  enduser:
    port: 3000  # Changed from 8000
  admin:
    port: 5000  # Changed from 14000
```

**Environment Variables**:
```bash
export IDENTITY_BROKER_SERVER_ENDUSER_PORT=3000
export IDENTITY_BROKER_SERVER_ADMIN_PORT=5000
```

**CLI Flags**:
```bash
./bin/agentic-identity-broker \
  --server.enduser.port 3000 \
  --server.admin.port 5000
```

### Bind to Specific Interface

**Localhost Only (IPv4)**:
```yaml
server:
  enduser:
    bind: "127.0.0.1"
  admin:
    bind: "127.0.0.1"
```

**Localhost Only (IPv6)**:
```yaml
server:
  enduser:
    bind: "::1"
  admin:
    bind: "::1"
```

**Specific Network Interface**:
```yaml
server:
  enduser:
    bind: "192.168.1.100"  # Private network
  admin:
    bind: "10.0.0.50"      # Management network
```

### Adjust Shutdown Timeout

```yaml
server:
  shutdown:
    timeout: 60s  # Wait up to 60 seconds for in-flight requests
```

### Enable Debug Logging

```yaml
log:
  level: debug
  format: json  # Structured JSON logs
```

## Troubleshooting

### Port Already in Use

**Symptom**: `bind: address already in use`

**Solution**:
1. Find process using the port:
   ```bash
   # macOS/Linux
   lsof -i :8000
   lsof -i :14000

   # Or
   netstat -tuln | grep 8000
   ```

2. Kill the process or change the port:
   ```bash
   ./bin/agentic-identity-broker --server.enduser.port 8001 --server.admin.port 14001
   ```

### Permission Denied (Privileged Ports)

**Symptom**: `bind: permission denied` when using ports <1024

**Solution**:
1. Use ports ≥1024 (recommended):
   ```yaml
   server:
     enduser:
       port: 8000  # Non-privileged
   ```

2. Or run with elevated privileges (not recommended):
   ```bash
   sudo ./bin/agentic-identity-broker
   ```

### IPv6 Not Available

**Symptom**: `cannot assign requested address` when binding to `::`

**Solution**: System automatically falls back to `0.0.0.0` (IPv4 only). Check logs:
```
[WARN] IPv6 not available, falling back to IPv4: [::]:8000 → 0.0.0.0:8000
```

### Health Check Returns 503

**Symptom**: `curl http://localhost:8000/health` returns HTTP 503

**Possible Causes**:
1. Server is still starting (status: "starting")
2. Server is shutting down (status: "shutting_down")
3. Server is unhealthy (status: "unhealthy")

**Solution**: Check response body for status details:
```bash
curl -i http://localhost:8000/health
```

### Configuration Not Loading

**Symptom**: Servers use default ports despite custom configuration

**Debug Steps**:
1. Verify configuration file exists and is valid YAML
2. Check configuration precedence (CLI > env > YAML > defaults)
3. Enable debug logging to see which values are loaded:
   ```bash
   ./bin/agentic-identity-broker --log.level debug
   ```

## Development Workflow

### 1. Make Changes

Edit code in `internal/adapters/http/`, `internal/domain/server/`, etc.

### 2. Run Tests

```bash
# Fast Go/package tests
just test

# Integration suites
just test-integration

# Full verification gate
just verify

# Coverage report
just test-coverage
```

### 3. Run Code Quality Checks

```bash
# Format, vet, lint
just check
```

### 4. Manual Verification

```bash
# Start server with hot-reload
just dev

# Test endpoints
curl http://localhost:8000/health
curl http://localhost:14000/health
```

### 5. Commit Changes

```bash
# Ensure all checks pass
just check

# Stage and commit
git add .
git commit -m "feat: implement dual-port HTTP server"
```

## Next Steps

- **Add Routes**: Extend chi routers in `internal/adapters/http/server.go`
- **Add Middleware**: Implement custom middleware in `internal/adapters/http/middleware.go`
- **Monitor**: Integrate with Prometheus/Grafana for metrics
- **Load Balancing**: Configure load balancer to use `/health` for readiness probes
- **TLS Support**: Wait for future TLS feature (currently out of scope)

## Reference

- **Spec**: [spec.md](spec.md)
- **Architecture**: [data-model.md](data-model.md)
- **API Contract**: [contracts/health-api.yaml](contracts/health-api.yaml)
- **Configuration Guide**: `docs/configuration.md` (to be created)
- **API Documentation**: `docs/api.md` (to be created)

## Quick Reference Card

```bash
# Build
just build

# Run
./bin/agentic-identity-broker

# Test health
curl http://localhost:8000/health    # End-user
curl http://localhost:14000/health   # Admin

# Stop (graceful)
kill -TERM $(pgrep agentic-identity-broker)

# Check logs
tail -f logs/agentic-identity-broker.log

# Override port
./bin/agentic-identity-broker --server.enduser.port 9000
```

## Configuration Precedence

```
CLI Flags           (Highest priority)
     ↓
Environment Variables
     ↓
YAML Config File
     ↓
Code Defaults      (Lowest priority)
```

**Example**:
- Default: 8000 (code)
- YAML: 8080 (config.yaml)
- Env: 8888 (IDENTITY_BROKER_SERVER_ENDUSER_PORT)
- CLI: 9000 (--server.enduser.port)
- **Result**: Server listens on port **9000**

