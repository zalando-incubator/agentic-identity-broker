# Feature Specification: Request Principal Extraction

**Feature Branch**: `20251216-120000-session-management`
**Created**: 2025-12-16
**Status**: Draft
**Input**: User description: "Let's spec out the feature 'session management'. To support user context, http endpoints (sub-paths in the routing) need to support extracting a 'principal', an identifier for a user like a username, from an http header passed to http endpoints. We support a reverse proxy in front of the application to do the authentication of the users so we can just rely on the authenticated user inside the app. This app should support configuring an http header from which the principal is extracted in plain text, as a future enhancement, JWTs should be supported. Cookies should not be set, also storing the session state in a database is explicitly not required. Maintaining per request context is enough."

## Clarifications

### Session 2025-12-16

- Q: Should principal extraction apply to all HTTP endpoints or only specific ones? → A: Apply based on routing configuration - each route/path prefix in the application code specifies whether principal extraction is enabled (not user-configurable, defined at development time)
- Q: What should happen when a principal value contains only whitespace (becomes empty after trimming)? → A: Treat as missing principal (reject with 401 if extraction enabled for route)
- Q: Should strict/permissive mode be global or per-route? → A: Per-route implicit behavior - if principal extraction is enabled for a route, principals are required (strict); if not enabled, no extraction occurs
- Q: What should happen when a principal value exceeds the maximum length limit? → A: Reject with 400 Bad Request (maximum length: 200 characters)
- Q: Should the system create audit logs for principal extraction events? → A: No, audit logging is not required for this feature

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Configure Principal Header (Priority: P1)

A system administrator needs to configure the application to extract user principals from a specific HTTP header that their reverse proxy sets after authenticating users.

**Why this priority**: This is the foundational capability that enables all user context tracking. Without configuration, the system cannot extract principals from any requests. This is the minimal viable feature that must work first.

**Independent Test**: Can be fully tested by setting the configuration value and verifying the application reads it correctly at startup. Delivers the ability to control which header contains user identity information.

**Acceptance Scenarios**:

1. **Given** the configuration file exists, **When** administrator sets the principal header name to "X-Authenticated-User", **Then** the application loads this configuration at startup
2. **Given** no principal header is configured, **When** the application starts, **Then** the application uses a default header name (e.g., "X-Remote-User")
3. **Given** an empty or invalid header name is configured, **When** the application starts, **Then** the application logs an error and uses the default header name

---

### User Story 2 - Extract Principal from Requests (Priority: P2)

The application receives HTTP requests from a reverse proxy and needs to extract the authenticated user's principal (username) from the configured header to maintain user context throughout request processing.

**Why this priority**: This is the core functionality that enables per-request user context. It builds on P1 (configuration) and must work before any request-specific features can use the principal.

**Independent Test**: Can be fully tested by sending HTTP requests with the configured header and verifying the principal is extracted and available in request context. Delivers the ability to identify which user made each request.

**Acceptance Scenarios**:

1. **Given** a request to a route with principal extraction enabled contains the configured principal header with value "alice@example.com", **When** the application processes the request, **Then** the principal "alice@example.com" is extracted and available in request context
2. **Given** a request to a route without principal extraction enabled contains the principal header, **When** the application processes the request, **Then** principal extraction is skipped and the request proceeds without a principal
3. **Given** a request contains the configured principal header with multiple values, **When** the application processes the request, **Then** only the first value is used as the principal
4. **Given** a request contains the principal header with leading/trailing whitespace, **When** the application processes the request, **Then** the whitespace is trimmed and the cleaned principal is used
5. **Given** the principal header contains special characters (e.g., unicode, emojis), **When** the application processes the request, **Then** the principal is preserved exactly as received (UTF-8 encoding)

---

### User Story 3 - Handle Missing or Invalid Principals (Priority: P3)

The application needs to gracefully handle requests that don't contain a valid principal by rejecting them when principal extraction is enabled for a route.

**Why this priority**: This ensures system security and prevents unauthorized access. It builds on P1 and P2 and adds defensive handling for edge cases.

**Independent Test**: Can be fully tested by sending requests without the principal header or with empty values to routes with principal extraction enabled, and verifying rejection with 401. Delivers protection against unauthenticated requests.

**Acceptance Scenarios**:

1. **Given** a route has principal extraction enabled, **When** a request arrives without the principal header, **Then** the request is rejected with HTTP 401 Unauthorized
2. **Given** a route has principal extraction enabled, **When** a request contains the principal header with an empty value, **Then** the request is rejected with HTTP 401 Unauthorized
3. **Given** a route has principal extraction enabled, **When** a request contains the principal header with only whitespace (e.g., "   "), **Then** the application trims whitespace, treats the result as missing, and rejects with HTTP 401 Unauthorized
4. **Given** a route has principal extraction enabled, **When** a request contains a principal value exceeding 200 characters, **Then** the request is rejected with HTTP 400 Bad Request
5. **Given** a route does not have principal extraction enabled, **When** a request arrives with or without the principal header, **Then** the request proceeds normally without principal extraction or validation

---

### Edge Cases

- What happens when the configured header name contains special characters or invalid characters?
- Principal value exceeding maximum length (200 characters): Rejected with HTTP 400 Bad Request as malformed input
- What happens when multiple headers with the same name are present in the request?
- How does the system handle case-sensitivity of header names (HTTP headers are case-insensitive)?
- What happens when the reverse proxy is misconfigured and doesn't set the expected header on routes requiring principals?
- Principal containing only whitespace: Trimmed to empty string and treated as missing principal (rejected with 401 if route has extraction enabled)
- How does routing determine whether principal extraction applies to a given request path?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support configuring the HTTP header name from which principals are extracted
- **FR-002**: System MUST extract the principal value from the configured HTTP header as plain text
- **FR-003**: System MUST make the extracted principal available in per-request context for all request handlers
- **FR-004**: System MUST trim leading and trailing whitespace from extracted principal values; if trimming results in an empty string, treat as a missing principal
- **FR-005**: System MUST handle HTTP header name matching in a case-insensitive manner (per HTTP specification)
- **FR-006**: System MUST reject requests with HTTP 401 Unauthorized when principal extraction is enabled for a route and the principal is missing or empty
- **FR-007**: System MUST use only the first value when multiple headers with the same name are present
- **FR-008**: System MUST preserve UTF-8 encoded characters in principal values
- **FR-009**: System MUST provide a default principal header name when none is configured
- **FR-010**: System MUST allow routing configuration to specify which routes/paths require principal extraction
- **FR-011**: System MUST apply principal extraction only to routes where it is explicitly enabled in the routing configuration
- **FR-012**: System MUST support route-level control (per-route or per-route-group basis) for enabling/disabling principal extraction
- **FR-013**: System MUST skip principal extraction and validation for routes where it is not enabled
- **FR-014**: System MUST reject requests with HTTP 400 Bad Request when the principal value exceeds 200 characters

### Security Requirements *(mandatory for security-critical features)*

<!--
  Per Constitution Principle I: Security-First Development
  Security features must be enabled by default and fail closed.
-->

- **SR-001**: Routes with principal extraction enabled MUST reject requests with missing or empty principals (fail closed)
- **SR-002**: System MUST NOT accept principals from headers outside the configured header name
- **SR-003**: System MUST enforce a maximum length limit of 200 characters on principal values to prevent memory exhaustion attacks; reject with HTTP 400 Bad Request when exceeded
- **SR-004**: System MUST validate that the configured header name is a valid HTTP header name according to RFC 7230
- **SR-005**: Routes requiring user context SHOULD enable principal extraction by default during route registration (secure by default)

### Key Entities *(include if feature involves data)*

- **Principal**: An identifier for an authenticated user (e.g., username, email address, user ID). Extracted from HTTP headers and available throughout request processing. Not persisted; exists only in request context.

- **Request Context**: The runtime context associated with a single HTTP request, containing the extracted principal and other request-specific metadata. Lifecycle is scoped to the request.

*Domain concepts should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Requests with valid principals to routes with extraction enabled are processed successfully with the principal available in request context
- **SC-002**: Requests without principals to routes with extraction enabled are rejected with HTTP 401 with 100% consistency
- **SC-003**: Principal extraction adds less than 1ms latency to request processing (measured at 95th percentile)
- **SC-004**: System correctly extracts principals from 99.9% of valid requests (excluding malformed requests)

## Assumptions *(optional - include if making assumptions)*

- The reverse proxy is correctly configured and trusted to authenticate users before forwarding requests
- The reverse proxy sets the principal header reliably for all authenticated requests to routes requiring principals
- Principal values are unique identifiers sufficient for user identification (no collision concerns)
- The application does not need to verify or validate the authenticity of principals (trusted reverse proxy pattern)
- Principal values will typically be email addresses or usernames (human-readable identifiers)
- Routes are configured at development time to enable/disable principal extraction based on their need for user context

## Out of Scope *(optional - explicitly state what's NOT included)*

- JWT token parsing and validation (future enhancement)
- Cookie-based session management
- Database-backed session storage
- Principal authentication or verification (delegated to reverse proxy)
- User profile or metadata retrieval based on principals
- Session expiration or timeout handling
- Multi-factor authentication integration
- Principal-to-role mapping or authorization policies

## Dependencies *(optional - external requirements)*

- Reverse proxy must be configured to set the principal header for authenticated requests
- Reverse proxy must perform user authentication before forwarding requests
- Configuration system must support string values for header name configuration
- Routing system must support route-level metadata or middleware attachment for enabling principal extraction

## Future Enhancements *(optional - potential follow-up work)*

- Support for JWT token extraction and validation from Authorization header
- Support for multiple principal sources with fallback logic (e.g., try JWT first, then header)
- Support for principal transformation rules (e.g., extract username from email address)
- Support for principal caching to avoid repeated extractions within the same request
- Integration with authorization system for role-based access control
