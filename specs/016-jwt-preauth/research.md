# Research: JWT Pre-Authentication & Principal Profile Enrichment

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

1. Check if the JWT header is present in the request
2. If present: parse and validate the JWT, extract principal + profile via CEL, set both in context
3. If present but invalid: reject with 401 (fail-closed, no fallback to plain header — per FR-013)
4. If absent: fall back to plain-header extraction (per FR-012)
5. If no JWT config at all: plain-header only (backward-compatible — per FR-011)

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
- Uses `lestrrat-go/jwx/v3` for JWT parsing and signature verification
- Uses `jwk.Cache` for JWKS fetching/caching (reuses existing JWKS adapter pattern from `internal/adapters/jwks/adapter.go`)
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
- **Library-first**: `lestrrat-go/jwx/v3` (already v3.0.13 in go.mod) handles all crypto — no custom JWT parsing or signature verification (Constitution Principle III).

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
- **Simple**: No shared state, no coordination logic, no risk of cache pollution. The `jwk.Cache` from `lestrrat-go/jwx/v3` is lightweight.

### Alternatives Considered

1. **Shared JWKS adapter with multiple URIs**: Rejected — overcomplicates the adapter; `jwk.Cache` already supports single URI registration per cache instance.
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

## Research Topic 6: Configuration Validation & Mutual Exclusivity

**Question**: How should the `verification: none` + `jwks_uri` mutual exclusivity be enforced?

### Decision: Startup-Time Validation in Config Schema

Add a custom validation function in `internal/config/schema.go` that runs during config loading:

1. If `authentication.jwt` is present and `verification` is `"none"` and `jwks_uri` is non-empty → startup error with descriptive message: `"authentication.jwt: verification 'none' and jwks_uri are mutually exclusive"`
2. If `authentication.jwt` is present and `verification` is `"jwks"` (or empty) and `jwks_uri` is empty → startup error: `"authentication.jwt: jwks_uri is required when verification is 'jwks'"`
3. CEL expressions validated at startup via CEL compile (fail-fast per ADR 009 pattern)
4. `principal_expression` required when JWT config present; profile expressions optional

### Rationale

- **Fail-fast**: Constitution Principle I (security-first, fail-closed). Misconfiguration detected immediately, not at first request.
- **Clear error messages**: Operators get actionable error messages at startup, not cryptic runtime failures.
- **Consistent with existing patterns**: Config validation in `schema.go` follows the established approach.

---

## Research Topic 7: Frontend Profile Display Strategy

> **Historical visual scope**: The token names below record this feature's original design choice, not a current constitutional aesthetic mandate; in particular, `slate-600` is not a current semantic-token recommendation. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

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

## Research Topic 8: Unsigned JWT (alg: "none") Handling with lestrrat-go/jwx

**Question**: How does `lestrrat-go/jwx/v3` handle unsigned JWTs (alg: "none")?

### Decision: Use `jwt.WithVerify(false)` for Unsigned Mode

When `verification` is `"none"`:
- Parse with `jwt.Parse([]byte(rawToken), jwt.WithVerify(false))` — disables signature verification
- Still validate `exp` claim via `jwt.WithValidate(true)` — expiry is always enforced regardless of verification mode (per FR-005)
- Still extract claims normally — JWT structure is preserved even without a signature

When `verification` is `"jwks"`:
- Parse with `jwt.Parse([]byte(rawToken), jwt.WithKeySet(keyset))` — signature verification against JWKS

### Rationale

- **Library-first**: `lestrrat-go/jwx/v3` natively supports both modes via parse options. No custom JWT parsing logic.
- **Expiry always enforced**: `jwt.WithValidate(true)` validates temporal claims (`exp`, `nbf`) regardless of signature verification mode. This satisfies FR-005 without additional code.
- **Secure by default**: `WithVerify(false)` is only used when explicitly configured with `verification: none`. The default path always validates signatures.

---

## Research Topic 9: E2E Test Infrastructure for JWT Pre-Auth

**Question**: How should E2E tests generate and serve JWTs for testing?

### Decision: In-Process JWKS Server + JWT Builder Helper

**Test infrastructure**:
1. **JWT builder** (`tests/e2e/helpers/jwt_helpers.go`):
   - Generate RSA and EC key pairs at test setup
   - Build signed JWTs with configurable claims (`sub`, `name`, `email`, `picture`, `aud`, `iss`, `exp`)
   - Build unsigned JWTs (alg: "none") for unsigned mode tests
   - Build expired JWTs, wrong-audience JWTs, wrong-issuer JWTs

2. **Mock JWKS server** (`tests/e2e/helpers/mock_jwks_server.go`):
   - `httptest.NewServer` serving the test JWK set (public key)
   - Returns the server URL to configure `jwks_uri` in test config
   - Can be shut down to simulate JWKS endpoint unavailability

3. **Test fixtures** (`tests/e2e/fixtures/jwt_config.go`):
   - Pre-configured `JWTConfig` structs for common test scenarios
   - Functions: `SignedJWTConfig(jwksURL)`, `UnsignedJWTConfig()`, `NoJWTConfig()` (backward-compatible)

### Rationale

- **Self-contained**: No external JWKS servers needed. Tests control key material and JWT content.
- **Follows existing patterns**: `mock_upstream.go` in `tests/e2e/helpers/` already uses `httptest.NewServer` for mock OAuth2 server — same pattern for mock JWKS.
- **Flexible**: Can generate any JWT variation (expired, wrong signature, missing claims) without external tooling.
