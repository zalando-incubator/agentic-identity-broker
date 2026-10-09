# Research: JWT Pre-Authentication & Principal Profile Enrichment

**Security amendment (2026-10-09):** Constitution Principle I supersedes the original unsigned pre-authentication research. Both servers require JWKS signature verification. ADR 031 permits unsigned JWTs only as local-mode OAuth2 impersonation subjects.

**Feature**: 016-jwt-preauth  
**Date**: 2026-02-27  
**Status**: Complete

---

## Research Topic 1: Principal Context Enrichment Strategy

**Question**: The current principal context stores a bare `string` via `principal.WithPrincipal(ctx, string)` / `principal.FromContext(ctx) (string, bool)`. How should profile attributes (display name, email, picture URL) be stored alongside the principal without breaking 12+ existing callers?

### Decision: Parallel Context Key for PrincipalProfile

Add a **separate context key** for `PrincipalProfile` alongside the existing `string` principal. The existing `principal.WithPrincipal(ctx, principalString)` and `principal.FromContext(ctx) (string, bool)` remain unchanged — all 12+ callers continue to work without modification.

A new pair of functions is added:
- `principal.WithProfile(ctx, PrincipalProfile) context.Context`
- `principal.ProfileFromContext(ctx) (PrincipalProfile, bool)`

The middleware sets **both**: the string principal (backward-compatible) and the enriched profile (new). The `UserInfoHandler` reads the profile if available, falls back to the string principal if not (backward-compatible with plain-header mode).

### Rationale

- **Zero breaking changes**: All existing callers use `FromContext(ctx) (string, bool)` — this signature and behavior is preserved.
- **Additive**: New callers (only `UserInfoHandler`) use the new `ProfileFromContext` function.
- **Clean separation**: Profile enrichment is optional — plain-header mode sets only the string principal (no profile), JWT mode sets both.
- **Follows Go context patterns**: Multiple context keys for related but distinct concerns is idiomatic (e.g., `trace.SpanFromContext` alongside custom keys).

### Alternatives Considered

1. **Change `WithPrincipal` to store a struct**: Rejected because it requires updating 12+ callers and their `(string, bool)` return type. High risk, low value — most callers only need the identifier string.
2. **Store `interface{}` in existing key**: Rejected because runtime type assertions are fragile and type-unsafe. Would require sentinel checks at every call site.
3. **Embed profile in HTTP request header**: Rejected because it conflates transport concerns with domain context and is not idiomatic Go.

---

## Research Topic 2: JWT Authentication Middleware Integration

**Question**: Should JWT authentication be a new middleware or an extension of the existing `RequirePrincipalMiddleware`?

### Decision: Extend Existing Middleware with JWT Authentication Path

Modify `RequirePrincipalMiddleware` and `OptionalPrincipalMiddleware` to accept `AuthenticationConfig` (which now includes `JWT *JWTConfig`). When JWT config is present:

1. If the JWT header is present, verify the signature against JWKS, extract claims, and set the principal and profile in context.
2. If the JWT is invalid or absent while JWT authentication is configured, reject the protected request without a plain-header fallback (FR-012/FR-013).
3. If no JWT config is present, use the plain principal header (FR-011).

### Rationale

- **Single responsibility**: Authentication is one concern — splitting JWT and header into separate middlewares creates ordering dependencies and confusion about which "wins".
- **Fail-closed semantics**: The middleware can enforce FR-013 (no silent fallback from invalid JWT to plain header) because it controls both paths.
- **Existing callers unchanged**: The middleware still calls `principal.WithPrincipal(ctx, string)` — downstream handlers don't know whether the principal came from a header or JWT.

### Alternatives Considered

1. **Separate `JWTMiddleware` before `PrincipalMiddleware`**: Rejected because it complicates the precedence logic (JWT present → JWT wins → don't check header; JWT invalid → reject; JWT absent → check header). Two separate middlewares can't easily coordinate this.
2. **Middleware chain with shared context**: Rejected because it adds coupling between middlewares via context state, which is harder to test and reason about.

---

## Research Topic 3: JWT Authenticator Architecture

**Question**: How should the JWT parsing, validation, and claim extraction be organized?

### Decision: Domain Port Interface + jwx Adapter

**Port** (`internal/domain/jwtauth/authenticator.go`):
```go
type JWTAuthenticator interface {
    Authenticate(ctx context.Context, rawJWT string) (*AuthResult, error)
}

type AuthResult struct {
    Principal       string
    DisplayName     *string  // nil if not extracted
    Email           *string  // nil if not extracted
    PictureURL      *string  // nil if not extracted
    Claims          map[string]interface{}
}
```

**Adapter** (`internal/adapters/jwtauth/jwx_authenticator.go`):
- Uses `lestrrat-go/jwx/v4` to verify JWT signatures against a JWKS key set
- Uses `jwkfetch.Cache` for independent JWKS fetching and refresh
- Delegates claim extraction to the CEL evaluator in the domain layer

**CEL Evaluator** (`internal/domain/jwtauth/cel_evaluator.go`):
- Follows exact pattern from `internal/domain/tokenexchange/cel_evaluator.go` (ADR 009)
- CEL environment variable: `claims` — `map(string, any)` (the JWT claims map)
- Four compiled programs: `principalProgram`, `displayNameProgram` (optional), `emailProgram` (optional), `pictureURLProgram` (optional)
- Fail-fast: expressions compiled at startup; invalid = startup failure
- Fail-closed: evaluation errors → authentication failure
- 100ms timeout for evaluation

### Rationale

- **Hexagonal architecture**: Domain defines the contract (`JWTAuthenticator` interface), adapter provides the implementation. Follows Constitution Principle VI.
- **Reuses proven patterns**: JWKS adapter pattern from token exchange (ADR 008), CEL evaluator pattern from token exchange (ADR 009).
- **Testable**: Port interface enables mock implementations for unit testing middleware.
- **Library-first**: `lestrrat-go/jwx/v4` handles JWT signature verification. No custom JWT parsing or signature checks are needed.

### Alternatives Considered

1. **Inline JWT validation in middleware**: Rejected — mixes transport concerns with crypto validation, violates hexagonal architecture.
2. **Reuse token exchange's `JWTValidator`**: Rejected — token exchange JWT validator is coupled to token exchange semantics (client_assertion validation, specific claim extraction). JWT pre-auth has different validation requirements (user JWT, audience/issuer, profile extraction). A new adapter with shared JWKS infrastructure is cleaner.

---

## Research Topic 4: JWKS Cache Reuse vs. Separate Instance

**Question**: Should the JWT pre-auth JWKS cache share the existing JWKS adapter instance (used by token exchange) or create a new one?

### Decision: Separate JWKS Cache Instance

JWT pre-auth creates its **own** JWKS adapter instance pointing to the `authentication.jwt.jwks_uri`. The token exchange's JWKS adapter points to the upstream OAuth2 server's JWKS endpoint (e.g., `oauth2_auth_server.upstream_issuer_uri + /.well-known/jwks.json`). These are typically **different endpoints** serving different key sets.

### Rationale

- **Different key sources**: Token exchange validates client assertion JWTs from the upstream OAuth2 server. JWT pre-auth validates user JWTs from a reverse proxy/gateway — potentially a completely different issuer with different keys.
- **Independent lifecycle**: Each JWKS cache has its own refresh interval and failure handling. A problem with token exchange JWKS should not affect pre-auth JWKS.
- **Simple**: Each pre-authentication JWKS cache is independent; the adapter uses `jwkfetch.Cache`.

### Alternatives Considered

1. **Shared JWKS adapter with multiple URIs**: Rejected — it adds shared state between distinct trusted issuers; each server's JWKS cache is independent.
2. **Registry pattern**: Rejected — over-engineered for two instances.

---

## Research Topic 5: Bearer Token Header Extraction

**Question**: How should the JWT be extracted from the HTTP header, particularly handling the `Bearer ` prefix?

### Decision: Auto-detect Based on Header Name

Per the spec: when `header_name` is `"Authorization"` (default), strip the `"Bearer "` prefix automatically. For any other header name, use the raw header value as the JWT.

Implementation:
```go
rawToken := r.Header.Get(headerName)
if strings.EqualFold(headerName, "Authorization") {
    rawToken = strings.TrimPrefix(rawToken, "Bearer ")
    rawToken = strings.TrimPrefix(rawToken, "bearer ")
}
rawToken = strings.TrimSpace(rawToken)
```

### Rationale

- **Convention-aware**: Standard OAuth2 Authorization header uses `Bearer <token>` format (RFC 6750). Auto-stripping follows principle of least surprise.
- **Flexible**: Non-standard headers (e.g., `X-JWT-Claims` in service mesh) pass raw JWTs without prefix.
- **Explicitly documented**: Spec clarification section confirms this behavior.

---

## Research Topic 6: JWT Pre-Authentication Configuration Validation

**Decision (amended 2026-10-09):** `internal/config/validator.go` requires signed JWKS verification on both servers:

1. If `authentication.jwt.verification` is not `jwks` or empty, startup fails with the verification field in the error.
2. If JWT pre-authentication has no `jwks_uri`, startup fails.
3. The broker compiles CEL expressions at startup and rejects invalid expressions.

The original mutual-exclusivity rule for `verification: none` and `jwks_uri` is superseded. `none` is invalid with or without a URI. This avoids an unsigned pre-authentication path while retaining ADR 031's separate impersonation-subject exception.

---

## Research Topic 7: Frontend Profile Display Strategy

**Question**: How should the consent UI header adapt to display email and enriched profile?

### Decision: Progressive Enhancement in AppLayout Header

1. Add `email?: string` to the `UserInfo` TypeScript interface
2. In the `Header` component (inside `AppLayout.tsx`):
   - Primary label: `displayName` (unchanged — already falls back to principal)
   - Secondary label: `email` if available, otherwise `principal` (current behavior shows principal as secondary)
   - Avatar: `pictureUrl` if available, otherwise initials from `displayName` (existing behavior)
3. No new components needed — existing Avatar and typography primitives handle all cases

### Rationale

- **Minimal frontend changes**: One type addition, one conditional in the header component.
- **Graceful degradation**: When email is absent, the header shows exactly what it shows today (principal as secondary label).
- **Design system compliant**: Uses existing `Avatar` primitive and semantic tokens (`trust-deep`, `slate-600`).

---

## Research Topic 8: Superseded Unsigned JWT Pre-Authentication Decision

The original research proposed parsing JWT pre-authentication credentials without signature verification when `verification: none` was set. Constitution Principle I supersedes that proposal. The JWX pre-authentication adapter always verifies signatures against JWKS before it reads claims. It rejects missing or expired `exp` claims after signature verification.

Accepted ADR 031 is a distinct, local-mode OAuth2 impersonation subject rule. It does not authorize unsigned JWTs on the admin or end-user pre-authentication path.

---

## Research Topic 9: E2E Test Infrastructure for JWT Pre-Auth

**Question**: How should E2E tests generate and serve JWTs for testing?

### Decision: In-Process JWKS Server + JWT Builder Helper

**Test infrastructure**:
1. **JWT builder** (`tests/e2e/helpers/jwt_helpers.go`):
   - Generate RSA and EC key pairs at test setup
   - Build signed JWTs with configurable claims (`sub`, `name`, `email`, `picture`, `aud`, `iss`, `exp`)
   - Build unsigned JWTs only to prove JWKS verification rejects them
   - Build expired JWTs, wrong-audience JWTs, wrong-issuer JWTs

2. **Mock JWKS server** (`tests/e2e/helpers/mock_jwks_server.go`):
   - `httptest.NewServer` serving the test JWK set (public key)
   - Returns the server URL to configure `jwks_uri` in test config
   - Can be shut down to simulate JWKS endpoint unavailability

3. **Test fixtures** (`tests/e2e/fixtures/jwt_config.go`):
   - Pre-configured `JWTConfig` structs for common test scenarios
   - Functions: `SignedJWTConfig(jwksURL)` and `NoJWTConfig()`; invalid unsigned settings are built in the rejection test

### Rationale

- **Self-contained**: No external JWKS servers needed. Tests control key material and JWT content.
- **Follows existing patterns**: `mock_upstream.go` in `tests/e2e/helpers/` already uses `httptest.NewServer` for mock OAuth2 server — same pattern for mock JWKS.
- **Flexible**: Can generate any JWT variation (expired, wrong signature, missing claims) without external tooling.
