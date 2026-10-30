# Data Model: Request Principal Extraction

## Overview

This document defines the domain model for request principal extraction. The system extracts authenticated user identifiers (principals) from HTTP headers set by a trusted reverse proxy and propagates them through request processing via Go context. Principals are request-scoped entities with no database persistence.

**Domain Scope**: Single HTTP request lifecycle. Principals exist only in memory as context values.

**Key Concepts**:
- **Principal**: Authenticated user identifier (username, email, user ID)
- **Request Context**: Go context.Context with embedded principal
- **Extraction**: Middleware process of reading and validating principals from HTTP headers
- **Propagation**: Passing principals through handler chain via context

## Entity: Principal

**Description**: A Principal represents an authenticated user identifier extracted from HTTP headers. It is a simple string value carrying user identity information set by a trusted reverse proxy after authentication. Principals are immutable once created and exist only for the duration of a single HTTP request.

**Structure**:
```go
// Principal is represented as a string in context
// The source header name is stored separately in configuration
type Principal string

// Internal representation (not exposed to domain)
type principalValue struct {
    Value  string  // The principal value (username, email, user ID)
    Source string  // The HTTP header name from which it was extracted
}
```

**Fields**:
- `Value` (string): The principal identifier extracted from the HTTP header
  - Examples: "alice@example.com", "user123", "alice.smith"
  - Constraints: Non-empty after trimming, ≤ 200 characters, valid UTF-8
  - Immutable: Cannot be changed after extraction

- `Source` (string, metadata): The HTTP header name from which the principal was extracted
  - Examples: "X-Remote-User", "X-Authenticated-User"
  - Purpose: Audit logging, debugging, security analysis
  - Set by configuration: `servers.api.principal_header_name`

**Validation Rules**:

1. **Trimming**: Value MUST be trimmed of leading/trailing whitespace before validation
2. **Non-empty**: Value MUST NOT be empty string after trimming
3. **Maximum Length**: Value MUST be ≤ 200 characters (prevents header injection attacks)
4. **UTF-8**: Value MUST be valid UTF-8 (Go's `string` type ensures this)
5. **No Newlines**: Value MUST NOT contain newline characters (enforced by HTTP header parsing)

**Validation Error Scenarios**:

| Scenario | Validation Rule Violated | Error Type | HTTP Status |
|----------|-------------------------|------------|-------------|
| Missing header | Non-empty | MissingPrincipalError | 401 Unauthorized |
| Empty value | Non-empty | MissingPrincipalError | 401 Unauthorized |
| Whitespace only (`"   "`) | Non-empty (after trim) | MissingPrincipalError | 401 Unauthorized |
| 201 characters | Maximum length | InvalidPrincipalError | 400 Bad Request |
| Multiple values | - | FirstValueUsed | N/A (first value used) |

**Lifecycle**:

1. **Creation**: Extracted by middleware from HTTP header during request processing
2. **Validation**: Checked against all validation rules before context storage
3. **Storage**: Stored in `context.Context` using unexported key (type-safe)
4. **Usage**: Accessed by downstream handlers via `FromContext(ctx)` or `MustFromContext(ctx)`
5. **Scope**: Single HTTP request (not persisted, not cached)
6. **Destruction**: Garbage collected when request completes

**Relationships**:
- **Embedded In**: Request Context (1:1 relationship)
- **Accessed By**: HTTP handlers, domain services (via context parameter)
- **Configured By**: `ServerInstanceConfig.PrincipalHeaderName` (1:1 relationship)

## Entity: Request Context

**Description**: A Request Context is a standard Go `context.Context` with a principal embedded as a context value. It follows Go's context propagation patterns and is passed through all layers of the application (HTTP adapters, domain services, storage adapters). The context is immutable—adding a principal creates a new context derived from the parent.

**Structure**:
```go
// Context is a standard Go context.Context
type Context = context.Context

// Context key for principal (unexported for type safety)
type principalContextKey struct{}

// Context values (conceptual—context.Context has no exposed fields)
// - Principal: string (user identifier)
// - Standard context fields: cancellation, deadlines, values
```

**Context Values**:

1. **Principal** (string):
   - Key: `principalContextKey{}` (unexported, prevents external packages from accessing)
   - Value: Validated principal string
   - Optional: Present only if middleware extracted valid principal
   - Retrieval: `principal.FromContext(ctx)` → `(string, bool)`

2. **Standard Go Context Fields**:
   - Cancellation: Request cancellation signal
   - Deadlines: Request timeout deadlines
   - Other values: Request ID, trace ID, etc. (from other middleware)

**Validation Rules**:

1. **Protected Routes**: Context MUST contain principal (enforced by `RequirePrincipalMiddleware`)
2. **Optional Routes**: Context MAY contain principal (not enforced)
3. **Type Safety**: Context MUST be accessed via `FromContext` (not direct `Value()` call)
4. **Immutability**: Context MUST NOT be modified after creation (Go enforces this)
5. **Propagation**: Context MUST be passed to all blocking operations (Go best practice)

**Lifecycle**:

1. **Creation**: Base context created by HTTP server for each request
2. **Augmentation**: Middleware creates derived context with principal using `WithPrincipal(ctx, principal)`
3. **Propagation**: Passed to all handlers via `r.Context()` and function parameters
4. **Cancellation**: Cancelled when request completes or client disconnects
5. **Destruction**: Garbage collected after request completes

**Relationships**:
- **Contains**: Principal entity (0..1 relationship—optional on public routes)
- **Passed To**: HTTP handlers, domain services, storage adapters (N relationships)
- **Created By**: HTTP middleware (`RequirePrincipalMiddleware`, `OptionalPrincipalMiddleware`)
- **Modified By**: Middleware (creates derived contexts, never mutates)

## Validation Error Types

### MissingPrincipalError

**Description**: Returned when a principal is required but missing or empty.

**Structure**:
```go
type MissingPrincipalError struct {
    HeaderName string
}

func (e *MissingPrincipalError) Error() string {
    return "missing or empty principal"
}
```

**Triggers**:
- HTTP header not present in request
- HTTP header present but value is empty string
- HTTP header present but value is whitespace only (after trimming)

**HTTP Response**:
- Status: 401 Unauthorized
- Body: `{"error": "missing or empty principal"}`
- Headers: `Content-Type: application/json`

**Recovery**: Not recoverable—request is rejected

### InvalidPrincipalError

**Description**: Returned when a principal is present but fails validation rules.

**Structure**:
```go
type InvalidPrincipalError struct {
    Reason string
    Value  string  // Truncated for logging safety
}

func (e *InvalidPrincipalError) Error() string {
    return e.Reason
}
```

**Triggers**:
- Principal length > 200 characters
- Principal contains invalid UTF-8 (rare—HTTP headers are typically ASCII)

**HTTP Response**:
- Status: 400 Bad Request
- Body: `{"error": "principal exceeds maximum length of 200 characters"}`
- Headers: `Content-Type: application/json`

**Recovery**: Not recoverable—request is rejected

## Context Operations

### Store Principal

**Function**: `WithPrincipal(ctx context.Context, principal string) context.Context`

**Description**: Creates a new context derived from the parent context with the principal embedded. The principal MUST be validated before calling this function.

**Parameters**:
- `ctx`: Parent context (typically from HTTP request)
- `principal`: Validated principal value (non-empty, ≤ 200 chars)

**Returns**: New context with principal embedded

**Preconditions**:
- `principal` MUST be non-empty
- `principal` MUST be trimmed
- `principal` MUST be ≤ 200 characters
- `principal` MUST be valid UTF-8

**Postconditions**:
- New context contains principal accessible via `FromContext`
- Parent context is unmodified (immutability)
- New context inherits cancellation, deadlines from parent

**Usage**:
```go
// In middleware
ctx := principal.WithPrincipal(r.Context(), "alice@example.com")
next.ServeHTTP(w, r.WithContext(ctx))
```

### Retrieve Principal (Safe)

**Function**: `FromContext(ctx context.Context) (string, bool)`

**Description**: Retrieves the principal from the context. Returns `(principal, true)` if present, `("", false)` if not present. This is the safe retrieval method for use in handlers that may or may not have principals.

**Parameters**:
- `ctx`: Context potentially containing principal

**Returns**:
- `string`: Principal value (empty string if not present)
- `bool`: True if principal present and valid, false otherwise

**Postconditions**:
- If `bool` is true, `string` is guaranteed non-empty and ≤ 200 characters
- If `bool` is false, `string` is empty string

**Usage**:
```go
// In handler (safe for optional routes)
principal, ok := principal.FromContext(r.Context())
if !ok {
    // No principal—handle accordingly
    http.Error(w, "unauthorized", http.StatusUnauthorized)
    return
}
// Use principal
log.Info("request from principal", "principal", principal)
```

### Retrieve Principal (Panic on Missing)

**Function**: `MustFromContext(ctx context.Context) string`

**Description**: Retrieves the principal from the context. Panics if principal is not present. This is intended for use on protected routes where middleware guarantees principal presence.

**Parameters**:
- `ctx`: Context MUST contain principal

**Returns**: Principal value (guaranteed non-empty)

**Preconditions**:
- Context MUST contain principal (enforced by middleware on protected routes)

**Error Handling**:
- Panics if principal not in context (programming error, not runtime error)
- Panic message: "principal not found in context"

**Usage**:
```go
// In handler (protected route only)
principal := principal.MustFromContext(r.Context())
// Safe to use—middleware guarantees presence
users := userService.GetUsersForPrincipal(principal)
```

**When to Use**:
- Protected routes with `RequirePrincipalMiddleware` applied
- Internal functions called only from protected routes
- Code paths guaranteed to have principal by design

**When NOT to Use**:
- Optional routes (use `FromContext` instead)
- Public routes (use `FromContext` instead)
- Code paths where principal may be missing

## Domain Invariants

These are properties that MUST always be true in the system:

1. **Valid Principal Invariant**: A principal stored in context MUST always be:
   - Non-empty string
   - Trimmed (no leading/trailing whitespace)
   - ≤ 200 characters
   - Valid UTF-8

2. **Protected Route Invariant**: If middleware rejects a request, handlers MUST NOT be invoked

3. **Context Safety Invariant**: Context keys MUST be unexported types to prevent key collisions

4. **Immutability Invariant**: Contexts MUST NOT be mutated—only derived contexts may be created

5. **Fail Closed Invariant**: Invalid principals MUST be rejected with 401/400 responses

6. **Single Source of Truth Invariant**: Principal value in context MUST match the HTTP header value (after trimming)

## Data Flow Diagram

```
HTTP Request
    │
    ├─ Header: X-Remote-User: "  alice@example.com  "
    │
    ▼
RequirePrincipalMiddleware
    │
    ├─ Extract: r.Header.Get("X-Remote-User")
    ├─ Trim: "alice@example.com"
    ├─ Validate: length=17, non-empty ✓
    │
    ▼
WithPrincipal(ctx, "alice@example.com")
    │
    ├─ Create: context with principalContextKey{} → "alice@example.com"
    │
    ▼
Request Handler
    │
    ├─ Retrieve: principal.FromContext(r.Context())
    ├─ Returns: ("alice@example.com", true)
    │
    ▼
Domain Service
    │
    ├─ Use: GetUsersForPrincipal("alice@example.com")
    │
    ▼
HTTP Response (200 OK)
```

## Performance Characteristics

| Operation | Time Complexity | Space Complexity | Typical Latency |
|-----------|----------------|------------------|-----------------|
| `WithPrincipal` | O(1) | O(n) where n=len(principal) | ~200ns |
| `FromContext` | O(1) | O(1) | ~50-100ns |
| `MustFromContext` | O(1) | O(1) | ~50-100ns |
| Header extraction | O(1) | O(n) where n=len(header) | ~100-500ns |
| Middleware (success) | O(1) | O(n) where n=len(principal) | <1μs |
| Middleware (failure) | O(1) | O(m) where m=len(error) | <10μs |

**Total Request Overhead**: <1ms at p95 (well within <200ms total request budget)

## Security Properties

1. **Authentication Delegated**: Application trusts reverse proxy for authentication
2. **Authorization Enabled**: Principals enable role-based access control (future feature)
3. **Audit Trail**: Principals logged for all authenticated actions
4. **Fail Closed**: Invalid requests rejected by default
5. **Type Safety**: Context keys prevent accidental collisions
6. **No Bypass**: Middleware must be explicitly applied to routes
7. **Length Limit**: Prevents header injection attacks via oversized values

## Storage Model

**Storage Type**: In-memory only (context.Context values)

**Persistence**: None. Principals are request-scoped and not persisted to database.

**Caching**: Not applicable. Context values are garbage collected after request.

**Distribution**: Not applicable. Each request creates its own context.

**Concurrency**: Safe. Context values are immutable and each request has isolated context.

## Future Extensions

While not part of this feature, the principal extraction model supports future enhancements:

1. **Role Extraction**: Extract roles from additional headers (e.g., "X-Remote-User-Roles")
2. **Claims Extraction**: Parse JWT claims from headers for richer user context
3. **Audit Logging**: Persist principal-based audit logs to database
4. **Multi-Tenant Support**: Extract tenant ID from headers alongside principal
5. **Rate Limiting**: Apply per-principal rate limits based on extracted identity

These extensions would follow the same patterns: extract from headers, validate, store in context, propagate to handlers.
