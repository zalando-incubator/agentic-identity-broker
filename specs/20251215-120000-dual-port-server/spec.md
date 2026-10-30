# Feature Specification: Dual-Port HTTP Server

**Feature Branch**: `20251215-120000-dual-port-server`
**Created**: 2025-12-15
**Status**: Draft
**Input**: User description: "http server. this application should serve two distinct user groups: end-users and administrators. To separate the two and allow different security measures. The application should listen to two different ports: one for end-user backend and one for the admin backend. Ports should be configurable and default to 8000 for the end-user port and 14000 for the admin backend. Make suggestions for the configuration."

## Clarifications

### Session 2025-12-15

- Q: IPv4/IPv6 Support Strategy → A: Dual-stack support with automatic detection (bind to :: on IPv6-capable systems, falls back to 0.0.0.0 on IPv4-only systems)
- Q: Server Failure Isolation Behavior → A: Both servers must start successfully or neither starts (atomic startup)
- Q: Health Check Endpoint Strategy → A: Separate health endpoints on each server reporting only that server's status
- Q: TLS/HTTPS Support → A: Defer TLS configuration to future feature (focus on HTTP only, document TLS as future enhancement)
- Q: Request Timeout Configuration → A: Use Go's standard library defaults (no explicit timeout configuration in this feature)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Basic Dual-Port Server Startup (Priority: P1)

System administrators need to start the identity broker with separate, isolated HTTP servers for end-users and administrators. Each server operates independently on its own port, allowing different security policies, rate limiting, and monitoring for each audience.

**Why this priority**: This is the core foundation of the feature. Without dual servers running on separate ports, none of the security separation benefits can be realized. This provides immediate value by enabling basic port-based segmentation.

**Independent Test**: Can be fully tested by starting the server and verifying that both ports (8000 and 14000) accept HTTP connections, and delivers the foundational infrastructure for audience-specific API endpoints.

**Acceptance Scenarios**:

1. **Given** default configuration, **When** application starts, **Then** both servers start successfully with end-user server listening on port 8000 and admin server listening on port 14000
2. **Given** custom ports configured (e.g., 3000 and 5000), **When** application starts, **Then** both servers start successfully with end-user server listening on port 3000 and admin server listening on port 5000
3. **Given** one server fails to bind (port already in use), **When** application starts, **Then** neither server starts and application exits with error code
4. **Given** both servers are running, **When** one server encounters a runtime error, **Then** the other server continues operating independently

---

### User Story 2 - Port Configuration via Multiple Sources (Priority: P2)

System administrators need to configure server ports through various methods (YAML files, environment variables, CLI flags) to support different deployment environments (development, staging, production) with appropriate precedence rules.

**Why this priority**: Flexible configuration is critical for real-world deployments but builds upon the basic server startup. It enables environment-specific overrides without code modifications or file edits.

**Independent Test**: Can be tested independently by providing port configurations through different sources and verifying the correct precedence hierarchy is followed, delivering value for multi-environment deployments.

**Acceptance Scenarios**:

1. **Given** no configuration specified, **When** application starts, **Then** ports default to 8000 (end-user) and 14000 (admin)
2. **Given** ports in YAML config and CLI flags, **When** application starts, **Then** CLI flag values take precedence over YAML values
3. **Given** invalid port configuration (negative, out of range, already in use), **When** application starts, **Then** application fails to start with clear error message identifying the misconfiguration

---

### User Story 3 - Server Binding Configuration (Priority: P3)

System administrators need to control which network interfaces each server binds to (localhost only, all interfaces, specific IPs) for security isolation and network topology requirements.

**Why this priority**: Binding configuration is important for security but is less critical than basic dual-port functionality. Default secure binding (localhost) provides adequate security for most scenarios initially.

**Independent Test**: Can be tested by configuring different bind addresses and verifying network accessibility from different interfaces, delivering value for complex network topologies and security-hardened deployments.

**Acceptance Scenarios**:

1. **Given** no bind address configured and IPv6-capable system, **When** servers start, **Then** both bind to :: (all interfaces, dual-stack accepting both IPv4 and IPv6)
2. **Given** no bind address configured and IPv4-only system, **When** servers start, **Then** both bind to 0.0.0.0 (all IPv4 interfaces)
3. **Given** bind address configured as "127.0.0.1" or "::1", **When** servers start, **Then** servers only accept connections from localhost on the specified protocol
4. **Given** different bind addresses for each server (e.g., enduser on ::, admin on 127.0.0.1), **When** servers start, **Then** each server binds to its configured address independently

---

### User Story 4 - Server Lifecycle Management (Priority: P2)

Operations teams need graceful server shutdown that completes in-flight requests, refuses new connections, and provides configurable timeout behavior to ensure zero data loss during deployments and maintenance.

**Why this priority**: Graceful shutdown is essential for production reliability and zero-downtime deployments. While not needed for initial startup, it's required before production use.

**Independent Test**: Can be tested by sending shutdown signal during active requests and verifying existing requests complete while new requests are rejected, delivering value for safe production operations.

**Acceptance Scenarios**:

1. **Given** servers are running with active requests, **When** shutdown signal received (SIGTERM/SIGINT), **Then** servers stop accepting new connections and wait for active requests to complete
2. **Given** shutdown in progress, **When** configurable timeout expires, **Then** servers force-close remaining connections and exit
3. **Given** servers starting, **When** startup encounters port binding error, **Then** neither server starts and process exits with error code

---

### Edge Cases

- What happens when only one port is configured (missing the other)?
- How does the system handle when one port is configured but the other defaults?
- What happens if both servers try to bind to the same port?
- How does system handle when configured port is below 1024 (requires elevated privileges on Unix)?
- What happens if one server fails to start but the other succeeds?
- How does system handle when network interface specified in bind address doesn't exist?
- What happens during shutdown if requests exceed the graceful timeout period?
- What happens when IPv6 is configured but the system does not support IPv6?
- How does the system handle binding to :: on dual-stack systems (should it accept IPv4-mapped IPv6 addresses)?
- What happens when one server is configured for IPv4 and another for IPv6?
- How does system behave when IPv6 is disabled at OS level but :: is configured?

## Requirements *(mandatory)*

### Scope

**In Scope:**
- HTTP server functionality on two independent ports
- IPv4 and IPv6 dual-stack support
- Configuration via YAML, environment variables, and CLI flags
- Graceful shutdown and lifecycle management
- Health check endpoints
- Comprehensive logging and observability

**Out of Scope (Future Enhancements):**
- TLS/HTTPS support (to be addressed in separate feature)
- Request timeout configuration (read/write/idle timeouts - using Go standard library defaults)
- Authentication and authorization mechanisms
- Rate limiting and throttling
- Request routing and middleware
- API endpoint implementation (focus is server infrastructure only)

### Functional Requirements

- **FR-001**: System MUST run two independent HTTP servers simultaneously: one for end-user traffic and one for administrative traffic
- **FR-002**: End-user server MUST default to port 8000 if not explicitly configured
- **FR-003**: Admin server MUST default to port 14000 if not explicitly configured
- **FR-004**: System MUST support port configuration via YAML configuration files
- **FR-005**: System MUST support port configuration via environment variables (IDENTITY_BROKER_ENDUSER_PORT, IDENTITY_BROKER_ADMIN_PORT)
- **FR-006**: System MUST support port configuration via command-line flags (--enduser-port, --admin-port)
- **FR-008**: System MUST validate port numbers are within valid range (1-65535) before attempting to bind
- **FR-009**: System MUST fail to start with descriptive error if port is invalid, privileged, or already in use
- **FR-010**: System MUST support configurable bind addresses for each server (which network interfaces to listen on)
- **FR-011**: System MUST support both IPv4 and IPv6 bind addresses, including dual-stack operation
- **FR-012**: System MUST automatically detect IPv6 capability and default to "::" (dual-stack) on IPv6-capable systems, falling back to "0.0.0.0" (IPv4-only) on IPv4-only systems
- **FR-013**: System MUST accept both IPv4 addresses (e.g., "0.0.0.0", "127.0.0.1", "192.168.1.1") and IPv6 addresses (e.g., "::", "::1", "2001:db8::1") as valid bind address configurations
- **FR-014**: System MUST use atomic startup: both servers must bind and start successfully, or the entire application fails to start (no partial service state)
- **FR-015**: Each server MUST operate independently at runtime such that a runtime failure of one server does not crash the other (independence applies after successful startup)
- **FR-016**: System MUST support graceful shutdown: completing in-flight requests while rejecting new connections
- **FR-017**: Graceful shutdown MUST have a configurable timeout (default: 30 seconds) after which connections are force-closed
- **FR-018**: System MUST log server startup events including bound addresses and ports for each server
- **FR-019**: Each server MUST expose a health check endpoint that reports only that server's operational status
- **FR-020**: Health check endpoints MUST respond with HTTP 200 when the server is healthy and able to accept requests
- **FR-021**: Health check endpoints MUST respond with HTTP 503 when the server is unhealthy or shutting down

### Security Requirements

- **SR-001**: Each server MUST operate in isolation such that a security vulnerability in one server does not automatically compromise the other
- **SR-002**: System MUST log all server lifecycle events (startup, shutdown, binding failures) for audit purposes
- **SR-003**: System MUST NOT expose admin server port in end-user server responses or error messages
- **SR-004**: System MUST validate bind addresses (both IPv4 and IPv6 format) to prevent binding to unintended interfaces
- **SR-005**: Configuration validation MUST fail closed: reject startup if configuration is ambiguous or invalid
- **SR-006**: System MUST emit structured logs for security-critical events (port binding, configuration loading, server failures)

### Key Entities

- **EndUserServer**: HTTP server instance dedicated to serving end-user traffic, operates on configurable port (default 8000), independent lifecycle from AdminServer
- **AdminServer**: HTTP server instance dedicated to serving administrative traffic, operates on configurable port (default 14000), independent lifecycle from EndUserServer
- **ServerConfig**: Configuration structure containing port numbers, bind addresses, timeouts, and lifecycle settings for both servers
- **BindAddress**: Network interface specification (IPv4 address, IPv6 address, or hostname) that determines which network interfaces a server accepts connections from; supports dual-stack operation when bound to :: on IPv6-capable systems

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Application successfully starts both servers on their configured ports within 5 seconds under normal conditions, or fails completely if either server cannot start
- **SC-002**: System administrators can change server ports without code modifications by updating configuration files or environment variables
- **SC-003**: After successful startup, when one server encounters a runtime failure, the other server continues serving requests with 100% uptime
- **SC-004**: During graceful shutdown, 99% of in-flight requests complete successfully before server termination
- **SC-005**: Configuration errors are detected before server binding, with clear error messages that allow administrators to resolve issues in under 2 minutes
- **SC-006**: Each server handles requests independently such that load on one server does not impact response time on the other server
- **SC-007**: All server lifecycle events (startup, configuration loading, shutdown) are logged with sufficient detail for troubleshooting in production environments
