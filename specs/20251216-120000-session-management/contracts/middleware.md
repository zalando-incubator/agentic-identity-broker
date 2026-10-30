# API Contract: Principal Extraction Middleware

## Overview

This document defines the behavioral contracts for principal extraction middleware. The middleware extracts authenticated user principals from HTTP headers, validates them, and propagates them through request processing via Go context. Two middleware variants are provided: `RequirePrincipalMiddleware` for protected routes (rejects invalid requests) and `OptionalPrincipalMiddleware` for public routes (extracts if present, continues if missing).

**Contract Type**: HTTP Middleware (chi router compatible)

**Security Model**: Fail closed. Invalid requests are rejected by default on protected routes.

**Performance Target**: <1ms added latency at p95

## Contract 1: RequirePrincipalMiddleware

### Purpose

HTTP middleware that extracts and validates principals from HTTP headers. Requests without valid principals are rejected with 401 Unauthorized (missing) or 400 Bad Request (malformed). Successfully extracted principals are added to the request context for use by downstream handlers.

### Signature

```go
func RequirePrincipalMiddleware(headerName string) func(http.Handler) http.Handler
```

**Factory Pattern**: Returns a middleware function that can be applied to chi route groups.

### Input Contract

#### Parameters

- **`headerName`** (string): Name of HTTP header containing principal
  - Examples: `"X-Remote-User"`, `"X-Authenticated-User"`, `"Remote-User"`
  - Case-insensitive: Middleware uses `http.Header.Get()` for case-insensitive lookup
  - Preconditions:
    - MUST be a valid HTTP header name (no newlines, no colons)
    - MUST NOT be empty string
    - SHOULD be configured per server instance (not hardcoded)

#### HTTP Request Requirements

- **Header Presence**: Header `headerName` MUST be present in request
- **Header Value**: Header value MUST be non-empty after trimming whitespace
- **Header Length**: Header value MUST be ≤ 200 characters
- **Header Encoding**: Header value MUST be valid UTF-8 (HTTP spec requires ASCII, but UTF-8 is accepted)

#### Preconditions

1. `headerName` MUST be a valid HTTP header name
2. `headerName` MUST NOT be empty
3. Middleware MUST be applied to chi router before request processing
4. Reverse proxy MUST set header for authenticated requests (deployment requirement)

### Output Contract

#### Success Case: Valid Principal

**Conditions**: Header present, value non-empty after trimming, length ≤ 200 characters

**HTTP Response**:
- Status: None written (continues to next handler)
- Body: None written
- Headers: None written

**Context Modification**:
- New context created with principal embedded
- Original context unmodified (immutability)
- Principal accessible via `principal.FromContext(ctx)`

**Side Effects**:
- Debug log entry: `"principal extracted"` with principal value and header name
- Next handler invoked with modified context

**Guarantees**:
- Principal in context is guaranteed non-empty
- Principal in context is guaranteed ≤ 200 characters
- Principal in context is guaranteed trimmed

#### Failure Case: Missing or Empty Principal

**Conditions**:
- Header not present in request, OR
- Header present but value is empty string, OR
- Header present but value is whitespace only (after trimming)

**HTTP Response**:
- Status: **401 Unauthorized**
- Body: `{"error": "missing or empty principal"}`
- Headers: `Content-Type: application/json`

**Context Modification**: None (original context unmodified)

**Side Effects**:
- Warning log entry: `"principal missing or empty"` with header name and request path
- Request processing stops (next handler NOT invoked)
- HTTP response written to client

**Guarantees**:
- Handler is NOT invoked
- Response is written exactly once
- No panic or error propagation

#### Failure Case: Principal Too Long

**Conditions**: Header value length > 200 characters

**HTTP Response**:
- Status: **400 Bad Request**
- Body: `{"error": "principal exceeds maximum length of 200 characters"}`
- Headers: `Content-Type: application/json`

**Context Modification**: None (original context unmodified)

**Side Effects**:
- Warning log entry: `"principal too long"` with header name, actual length, and max length
- Request processing stops (next handler NOT invoked)
- HTTP response written to client

**Guarantees**:
- Handler is NOT invoked
- Response is written exactly once
- No panic or error propagation

### Behavior Specification

**Algorithm**:

1. Extract header value using case-insensitive lookup: `value = r.Header.Get(headerName)`
2. **Check presence**: If value is empty string, write 401 response and return
3. **Trim whitespace**: `value = strings.TrimSpace(value)`
4. **Check non-empty**: If trimmed value is empty string, write 401 response and return
5. **Check length**: If `len(value) > 200`, write 400 response and return
6. **Create context**: `ctx = principal.WithPrincipal(r.Context(), value)`
7. **Log debug**: Log successful extraction with principal value and header name
8. **Invoke next**: Call `next.ServeHTTP(w, r.WithContext(ctx))`

**Edge Cases**:

| Case | Behavior |
|------|----------|
| Multiple header values | Use first value (per `http.Header.Get()` semantics) |
| Header with leading/trailing whitespace | Trim and use |
| Header with internal whitespace | Preserve internal whitespace |
| Non-ASCII characters | Accept if valid UTF-8 |
| Empty header name | Programming error (panic or validation at startup) |
| Nil http.Handler | Programming error (panic per chi middleware contract) |

**Concurrency Safety**: Safe for concurrent requests. Each request has isolated context.

### Usage Example

```go
package main

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

func main() {
    r := chi.NewRouter()

    // Protected API routes - require principal
    r.Group(func(r chi.Router) {
        r.Use(http.RequirePrincipalMiddleware("X-Remote-User"))
        r.Get("/api/users", handleGetUsers)      // Requires principal
        r.Post("/api/data", handleCreateData)    // Requires principal
        r.Delete("/api/data/{id}", handleDeleteData)  // Requires principal
    })

    // Public routes - no principal required
    r.Get("/health", handleHealth)
    r.Get("/metrics", handleMetrics)

    http.ListenAndServe(":8080", r)
}

func handleGetUsers(w http.ResponseWriter, r *http.Request) {
    // Principal guaranteed to be present (middleware ensures this)
    principalValue := principal.MustFromContext(r.Context())

    // Use principal for business logic
    users := getUsersForPrincipal(principalValue)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(users)
}
```

### Test Scenarios

#### Scenario 1: Valid Principal

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User: alice@example.com
```

**Expected Output**:
- HTTP Status: Continues to handler (no status written by middleware)
- Context: Principal = `"alice@example.com"`
- Logs: `DEBUG principal extracted principal=alice@example.com header=X-Remote-User`

#### Scenario 2: Missing Header

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
```

**Expected Output**:
- HTTP Status: `401 Unauthorized`
- Response Body: `{"error": "missing or empty principal"}`
- Response Headers: `Content-Type: application/json`
- Context: Unmodified (handler not invoked)
- Logs: `WARN principal missing or empty header=X-Remote-User path=/api/users`

#### Scenario 3: Empty Header Value

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User:
```

**Expected Output**:
- HTTP Status: `401 Unauthorized`
- Response Body: `{"error": "missing or empty principal"}`
- Response Headers: `Content-Type: application/json`
- Context: Unmodified (handler not invoked)
- Logs: `WARN principal missing or empty header=X-Remote-User path=/api/users`

#### Scenario 4: Whitespace-Only Header Value

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User:
```

**Expected Output**:
- HTTP Status: `401 Unauthorized`
- Response Body: `{"error": "missing or empty principal"}`
- Response Headers: `Content-Type: application/json`
- Context: Unmodified (handler not invoked)
- Logs: `WARN principal missing or empty header=X-Remote-User path=/api/users`

#### Scenario 5: Principal Too Long (201 Characters)

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User: aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa
```

**Expected Output**:
- HTTP Status: `400 Bad Request`
- Response Body: `{"error": "principal exceeds maximum length of 200 characters"}`
- Response Headers: `Content-Type: application/json`
- Context: Unmodified (handler not invoked)
- Logs: `WARN principal too long header=X-Remote-User length=201 max=200`

#### Scenario 6: Leading/Trailing Whitespace

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User:   alice@example.com
```

**Expected Output**:
- HTTP Status: Continues to handler
- Context: Principal = `"alice@example.com"` (whitespace trimmed)
- Logs: `DEBUG principal extracted principal=alice@example.com header=X-Remote-User`

#### Scenario 7: Multiple Header Values (HTTP/1.1)

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User: alice@example.com
X-Remote-User: bob@example.com
```

**Expected Output**:
- HTTP Status: Continues to handler
- Context: Principal = `"alice@example.com"` (first value per `http.Header.Get()` semantics)
- Logs: `DEBUG principal extracted principal=alice@example.com header=X-Remote-User`

#### Scenario 8: Non-ASCII Characters (UTF-8)

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
X-Remote-User: alice_münchen@example.com
```

**Expected Output**:
- HTTP Status: Continues to handler
- Context: Principal = `"alice_münchen@example.com"`
- Logs: `DEBUG principal extracted principal=alice_münchen@example.com header=X-Remote-User`

#### Scenario 9: Case-Insensitive Header Lookup

**Input**:
```http
GET /api/users HTTP/1.1
Host: example.com
x-remote-user: alice@example.com
```

**Expected Output** (middleware configured with `"X-Remote-User"`):
- HTTP Status: Continues to handler
- Context: Principal = `"alice@example.com"` (header matched case-insensitively)
- Logs: `DEBUG principal extracted principal=alice@example.com header=X-Remote-User`

## Contract 2: OptionalPrincipalMiddleware

### Purpose

HTTP middleware that extracts principals if present but allows requests without principals to continue. Useful for routes that benefit from principal context (e.g., logging, metrics) but don't require authentication. Malformed principals are logged as warnings but do not block the request.

### Signature

```go
func OptionalPrincipalMiddleware(headerName string) func(http.Handler) http.Handler
```

**Factory Pattern**: Returns a middleware function that can be applied to chi route groups.

### Input Contract

#### Parameters

- **`headerName`** (string): Name of HTTP header containing principal
  - Examples: `"X-Remote-User"`, `"X-Authenticated-User"`
  - Case-insensitive: Middleware uses `http.Header.Get()` for case-insensitive lookup
  - Preconditions:
    - MUST be a valid HTTP header name
    - MUST NOT be empty string

#### HTTP Request Requirements

- **Header Presence**: Header `headerName` MAY be present (not required)
- **Header Value**: If present, header value SHOULD be non-empty after trimming
- **Header Length**: If present, header value SHOULD be ≤ 200 characters
- **Header Encoding**: If present, header value SHOULD be valid UTF-8

#### Preconditions

1. `headerName` MUST be a valid HTTP header name
2. `headerName` MUST NOT be empty
3. Middleware MUST be applied to chi router before request processing

### Output Contract

#### Success Case: Valid Principal

**Conditions**: Header present, value non-empty after trimming, length ≤ 200 characters

**HTTP Response**:
- Status: None written (continues to next handler)
- Body: None written
- Headers: None written

**Context Modification**:
- New context created with principal embedded
- Original context unmodified (immutability)
- Principal accessible via `principal.FromContext(ctx)`

**Side Effects**:
- Debug log entry: `"principal extracted (optional)"` with principal value and header name
- Next handler invoked with modified context

#### Success Case: No Principal

**Conditions**: Header not present in request

**HTTP Response**:
- Status: None written (continues to next handler)
- Body: None written
- Headers: None written

**Context Modification**: None (original context unmodified, no principal in context)

**Side Effects**:
- Debug log entry: `"principal not provided (optional)"` with header name and path
- Next handler invoked with original context

**Guarantees**:
- Handler is invoked regardless of principal presence
- No HTTP error response written
- No panic or error propagation

#### Warning Case: Empty Principal

**Conditions**: Header present but value is empty or whitespace only (after trimming)

**HTTP Response**:
- Status: None written (continues to next handler)
- Body: None written
- Headers: None written

**Context Modification**: None (original context unmodified, no principal in context)

**Side Effects**:
- Debug log entry: `"principal not provided (optional)"` with header name and path
- Next handler invoked with original context

#### Warning Case: Principal Too Long

**Conditions**: Header value length > 200 characters

**HTTP Response**:
- Status: None written (continues to next handler)
- Body: None written
- Headers: None written

**Context Modification**: None (original context unmodified, no principal in context)

**Side Effects**:
- Warning log entry: `"principal too long, ignoring"` with header name and length
- Next handler invoked with original context

### Behavior Specification

**Algorithm**:

1. Extract header value using case-insensitive lookup: `value = r.Header.Get(headerName)`
2. **Check presence**: If value is empty string, log debug and continue with original context
3. **Trim whitespace**: `value = strings.TrimSpace(value)`
4. **Check non-empty**: If trimmed value is empty string, log debug and continue with original context
5. **Check length**: If `len(value) > 200`, log warning and continue with original context
6. **Create context**: `ctx = principal.WithPrincipal(r.Context(), value)`
7. **Log debug**: Log successful extraction with principal value and header name
8. **Invoke next**: Call `next.ServeHTTP(w, r.WithContext(ctx))`

**Key Difference from RequirePrincipalMiddleware**: All validation failures result in continuing with original context (no principal) instead of rejecting the request.

### Usage Example

```go
package main

import (
    "net/http"

    "github.com/go-chi/chi/v5"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

func main() {
    r := chi.NewRouter()

    // Public routes - principal optional but useful for logging
    r.Group(func(r chi.Router) {
        r.Use(http.OptionalPrincipalMiddleware("X-Remote-User"))
        r.Get("/health", handleHealth)        // Works with or without principal
        r.Get("/metrics", handleMetrics)      // Works with or without principal
        r.Get("/docs", handleDocs)            // Works with or without principal
    })

    http.ListenAndServe(":8080", r)
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
    // Check if principal is present (for audit logging)
    if principalValue, ok := principal.FromContext(r.Context()); ok {
        log.Info("health check", "principal", principalValue)
    } else {
        log.Info("health check", "principal", "anonymous")
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}
```

### Test Scenarios

#### Scenario 1: Valid Principal

**Input**:
```http
GET /health HTTP/1.1
Host: example.com
X-Remote-User: alice@example.com
```

**Expected Output**:
- HTTP Status: Continues to handler (handler determines status)
- Context: Principal = `"alice@example.com"`
- Logs: `DEBUG principal extracted (optional) principal=alice@example.com header=X-Remote-User`

#### Scenario 2: Missing Header

**Input**:
```http
GET /health HTTP/1.1
Host: example.com
```

**Expected Output**:
- HTTP Status: Continues to handler (handler determines status)
- Context: No principal (original context)
- Logs: `DEBUG principal not provided (optional) header=X-Remote-User path=/health`

#### Scenario 3: Empty Header Value

**Input**:
```http
GET /health HTTP/1.1
Host: example.com
X-Remote-User:
```

**Expected Output**:
- HTTP Status: Continues to handler (handler determines status)
- Context: No principal (original context)
- Logs: `DEBUG principal not provided (optional) header=X-Remote-User path=/health`

#### Scenario 4: Principal Too Long

**Input**:
```http
GET /health HTTP/1.1
Host: example.com
X-Remote-User: [201 characters]
```

**Expected Output**:
- HTTP Status: Continues to handler (handler determines status)
- Context: No principal (original context)
- Logs: `WARN principal too long, ignoring header=X-Remote-User length=201`

## Contract 3: Context Propagation

### Purpose

Type-safe functions for storing and retrieving principals from Go context. These functions provide the API for middleware to store principals and for handlers to retrieve them.

### Function: WithPrincipal

#### Signature

```go
func WithPrincipal(ctx context.Context, principal string) context.Context
```

#### Purpose

Creates a new context derived from the parent context with the principal embedded as a context value.

#### Input Contract

**Parameters**:
- `ctx` (context.Context): Parent context (typically from HTTP request)
- `principal` (string): Principal value (MUST be validated by caller)

**Preconditions**:
- `principal` MUST be non-empty
- `principal` MUST be trimmed (no leading/trailing whitespace)
- `principal` MUST be ≤ 200 characters
- `principal` MUST be valid UTF-8

#### Output Contract

**Returns**: New context with principal embedded

**Postconditions**:
- New context contains principal accessible via `FromContext`
- Parent context is unmodified (immutability)
- New context inherits cancellation and deadlines from parent
- New context inherits other context values from parent

#### Behavior Specification

**Algorithm**:
1. Create new context using `context.WithValue(ctx, principalContextKey{}, principal)`
2. Return new context

**Performance**: ~200ns, 1 allocation

**Concurrency Safety**: Safe. Context values are immutable.

#### Usage Example

```go
// In middleware
func RequirePrincipalMiddleware(headerName string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            principalValue := extractAndValidate(r, headerName)
            if principalValue == "" {
                http.Error(w, "unauthorized", http.StatusUnauthorized)
                return
            }

            // Add principal to context
            ctx := principal.WithPrincipal(r.Context(), principalValue)

            // Continue with modified context
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

### Function: FromContext

#### Signature

```go
func FromContext(ctx context.Context) (string, bool)
```

#### Purpose

Retrieves the principal from the context. Safe for use in any handler (protected or public). Returns a tuple indicating whether the principal is present.

#### Input Contract

**Parameters**:
- `ctx` (context.Context): Context potentially containing principal

**Preconditions**: None

#### Output Contract

**Returns**:
- `string`: Principal value (empty string if not present)
- `bool`: True if principal is present, false otherwise

**Postconditions**:
- If `bool` is true, `string` is guaranteed non-empty and ≤ 200 characters
- If `bool` is false, `string` is empty string `""`

#### Behavior Specification

**Algorithm**:
1. Retrieve value from context: `value = ctx.Value(principalContextKey{})`
2. Type assert to string: `principal, ok = value.(string)`
3. Return `(principal, ok)`

**Performance**: ~50-100ns, 0 allocations

**Concurrency Safety**: Safe. Context values are immutable.

#### Usage Example

```go
// In handler (safe for optional routes)
func handleMetrics(w http.ResponseWriter, r *http.Request) {
    // Check if principal is present
    principal, ok := principal.FromContext(r.Context())
    if !ok {
        // No principal—handle accordingly
        log.Info("metrics request", "principal", "anonymous")
    } else {
        // Principal present—use for logging
        log.Info("metrics request", "principal", principal)
    }

    // Continue processing regardless of principal presence
    metrics := collectMetrics()
    json.NewEncoder(w).Encode(metrics)
}
```

### Function: MustFromContext

#### Signature

```go
func MustFromContext(ctx context.Context) string
```

#### Purpose

Retrieves the principal from the context. Panics if principal is not present. Intended for use on protected routes where middleware guarantees principal presence.

#### Input Contract

**Parameters**:
- `ctx` (context.Context): Context MUST contain principal

**Preconditions**:
- Context MUST contain principal (enforced by middleware on protected routes)

#### Output Contract

**Returns**: Principal value (guaranteed non-empty and ≤ 200 characters)

**Postconditions**:
- Returned principal is guaranteed non-empty
- Returned principal is guaranteed ≤ 200 characters

#### Error Handling

**Panic Conditions**:
- Principal not present in context

**Panic Message**: `"principal not found in context"`

**When to Panic**: This indicates a programming error (middleware not applied to route) rather than a runtime error. Panicking is appropriate because:
1. It fails fast during development/testing
2. It's caught by chi's recovery middleware in production
3. It indicates a configuration error, not a user error

#### Behavior Specification

**Algorithm**:
1. Call `FromContext(ctx)`
2. If `ok` is false, panic with message `"principal not found in context"`
3. Return principal value

**Performance**: ~50-100ns, 0 allocations (no panic case)

**Concurrency Safety**: Safe. Context values are immutable.

#### Usage Example

```go
// In handler (protected route only)
func handleGetUsers(w http.ResponseWriter, r *http.Request) {
    // Principal guaranteed to be present (middleware ensures this)
    principalValue := principal.MustFromContext(r.Context())

    // Safe to use—no need to check for presence
    users := userService.GetUsersForPrincipal(principalValue)

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(users)
}
```

#### When to Use

**Use `MustFromContext` when**:
- Route has `RequirePrincipalMiddleware` applied
- Code path is guaranteed to have principal by design
- You want to fail fast on programming errors

**Use `FromContext` when**:
- Route has `OptionalPrincipalMiddleware` applied
- Route is public (no principal middleware)
- You want to handle missing principal gracefully

## Error Response Format

All error responses use consistent JSON structure for API consistency and client parsing.

### Structure

```json
{
  "error": "human-readable error message"
}
```

### Content-Type

All error responses MUST have `Content-Type: application/json` header.

### Examples

**Missing Principal (401)**:
```json
{
  "error": "missing or empty principal"
}
```

**Principal Too Long (400)**:
```json
{
  "error": "principal exceeds maximum length of 200 characters"
}
```

### Rationale

- **Consistency**: All API errors use same format
- **Parseable**: Clients can reliably extract error messages
- **Human-readable**: Suitable for logging and debugging
- **Simple**: No nested structures, easy to construct

## Performance Characteristics

| Operation | Time Complexity | Typical Latency | Allocations |
|-----------|----------------|-----------------|-------------|
| Context lookup (`FromContext`) | O(1) | ~50-100ns | 0 |
| Context creation (`WithPrincipal`) | O(1) | ~200ns | 1 (context) |
| Header extraction (`http.Header.Get`) | O(n)† | ~100-500ns | 0 |
| String trimming (`strings.TrimSpace`) | O(n) | ~50-200ns | 0-1 (if trim needed) |
| Middleware (success path) | O(1) | <1μs | 1 (context) |
| Middleware (failure path) | O(1) | <10μs | 2-3 (error response) |
| JSON error encoding | O(1) | ~1-5μs | 2-3 (buffer, JSON) |

† O(n) where n = number of headers (typically <50), but effectively constant time

**Total Request Overhead**: <1ms at p95 (well within <200ms total request budget)

**Benchmark Targets**:
- `BenchmarkFromContext`: <100ns/op
- `BenchmarkWithPrincipal`: <250ns/op
- `BenchmarkMiddleware_Success`: <1000ns/op
- `BenchmarkMiddleware_Failure`: <10000ns/op

## Security Guarantees

### 1. Fail Closed

Invalid requests are rejected by default on protected routes. No requests bypass validation.

**Implementation**: `RequirePrincipalMiddleware` returns 401/400 before invoking handler.

### 2. No Bypass

Middleware must be explicitly applied to routes. No automatic principal extraction.

**Implementation**: Middleware applied via `router.Use()` or `router.With()`. Protected routes explicitly configured.

### 3. Case-Insensitive Header Matching

Header names matched per HTTP/1.1 spec (RFC 7230, case-insensitive).

**Implementation**: `http.Header.Get()` performs case-insensitive lookup.

### 4. Type Safety

Context keys are unexported to prevent collisions with other packages.

**Implementation**: `type principalContextKey struct{}` is unexported. External packages cannot create this type.

### 5. Input Validation

All principals validated before storage in context.

**Implementation**: Middleware performs trimming, empty check, length check before calling `WithPrincipal`.

### 6. Length Limit

Prevents header injection attacks via oversized values.

**Implementation**: 200 character limit enforced, requests rejected with 400 status.

### 7. Immutability

Context values cannot be modified after creation.

**Implementation**: Go's `context.Context` is immutable by design. All modifications create new derived contexts.

### 8. Authentication Delegation

Application trusts reverse proxy for authentication. No password verification.

**Security Model**: Reverse proxy MUST be configured to:
- Authenticate users before setting principal header
- Strip principal headers from untrusted sources (client requests)
- Use TLS for communication with application

**Deployment Requirement**: Documented in deployment guide.

## Integration with Chi Router

### Applying Middleware to Route Groups

```go
r := chi.NewRouter()

// Protected API routes
r.Group(func(r chi.Router) {
    r.Use(RequirePrincipalMiddleware("X-Remote-User"))
    r.Get("/api/users", handleGetUsers)
    r.Post("/api/data", handleCreateData)
})

// Public routes
r.Get("/health", handleHealth)
r.Get("/metrics", handleMetrics)
```

### Per-Endpoint Middleware

```go
r := chi.NewRouter()

// Most routes are public
r.Get("/docs", handleDocs)
r.Get("/health", handleHealth)

// Single protected endpoint
r.With(RequirePrincipalMiddleware("X-Remote-User")).
    Get("/api/private", handlePrivate)
```

### Multiple Route Groups with Different Headers

```go
r := chi.NewRouter()

// Admin routes - X-Admin-User header
r.Group(func(r chi.Router) {
    r.Use(RequirePrincipalMiddleware("X-Admin-User"))
    r.Get("/admin/stats", handleAdminStats)
})

// API routes - X-Remote-User header
r.Group(func(r chi.Router) {
    r.Use(RequirePrincipalMiddleware("X-Remote-User"))
    r.Get("/api/users", handleGetUsers)
})
```

## Compatibility

### HTTP Versions

- **HTTP/1.0**: Supported (single header value)
- **HTTP/1.1**: Supported (multiple header values—uses first value)
- **HTTP/2**: Supported (lowercase headers automatically handled by Go's HTTP/2 implementation)

### Header Encoding

- **ASCII**: Fully supported (standard HTTP header encoding)
- **UTF-8**: Supported (Go's `string` type is UTF-8)
- **Latin-1/ISO-8859-1**: Supported (subset of UTF-8)
- **Invalid UTF-8**: Rejected implicitly by Go's string handling

### Reverse Proxy Compatibility

Tested with:
- **nginx**: `proxy_set_header X-Remote-User $remote_user;`
- **Apache**: `RequestHeader set X-Remote-User %{REMOTE_USER}e`
- **Traefik**: `headers.customrequestheaders.X-Remote-User`
- **Envoy**: `request_headers_to_add`
- **Caddy**: `header_up X-Remote-User {http.auth.user.id}`

## Observability

### Log Levels

- **DEBUG**: Successful principal extraction (every request on protected routes)
- **WARN**: Principal validation failures (missing, empty, too long)
- **ERROR**: None (middleware does not produce errors, only rejects requests)

### Log Fields

All log entries include:

| Field | Description | Example |
|-------|-------------|---------|
| `level` | Log level | `DEBUG`, `WARN` |
| `msg` | Human-readable message | `"principal extracted"` |
| `principal` | Extracted principal value | `"alice@example.com"` |
| `header` | Header name (from config) | `"X-Remote-User"` |
| `path` | Request path | `"/api/users"` |
| `length` | Principal length (for errors) | `201` |
| `max` | Maximum allowed length | `200` |

### Metrics (Future)

Recommended metrics for monitoring:

- `principal_extraction_total{status="success"}`: Counter of successful extractions
- `principal_extraction_total{status="missing"}`: Counter of missing principals (401)
- `principal_extraction_total{status="too_long"}`: Counter of oversized principals (400)
- `principal_extraction_duration_seconds`: Histogram of middleware latency

## Future Enhancements

This contract may be extended in the future to support:

1. **Role Extraction**: Additional headers for role-based access control
2. **JWT Claims**: Parse JWT tokens from headers for richer user context
3. **Multi-Tenant Support**: Extract tenant ID alongside principal
4. **Rate Limiting**: Per-principal rate limits
5. **Audit Logging**: Persistent audit trail of principal-based actions

All enhancements will maintain backward compatibility with this contract.
