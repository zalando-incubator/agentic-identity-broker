# Quickstart: JWT Pre-Authentication & Principal Profile Enrichment

**Feature**: 016-jwt-preauth  
**Date**: 2026-02-27

**Security amendment (2026-10-09):** Admin and end-user JWT pre-authentication require signed JWKS verification. The original unsigned pre-authentication steps are superseded by Constitution Principle I. ADR 031 permits unsigned JWTs only as local-mode OAuth2 impersonation subjects.

---

## Overview

This guide describes how to implement the JWT pre-authentication feature following the patterns established in this codebase. It covers the implementation order, key patterns to follow, and references to existing code that should be used as templates.

---

## Implementation Order

The feature should be implemented in this sequence, with each step building on the previous:

### Step 1: Configuration Types (`internal/ports/config.go`)

**What**: Add `JWTConfig` and `JWTClaimExtractionConfig` types. Extend `AuthenticationConfig` with `JWT *JWTConfig`.

**Pattern to follow**: Existing `PreauthConfig` struct in the same file. Use `mapstructure` tags for YAML binding and `validate` tags for required fields.

**Key decisions**:
- `JWT` is a `*JWTConfig` (pointer) — `nil` means "not configured" (backward-compatible)
- `Verification` defaults to `"jwks"` in the validator. No unsigned mode is available.
- `PrincipalExpression` defaults to `"claims.sub"`

**Validation** (in `internal/config/validator.go`):
- `verification: none` on either server → startup error, even if `jwks_uri` is present
- Missing `jwks_uri` with JWT pre-authentication → startup error
- CEL expressions validated at startup (delegate to CEL evaluator constructor)

### Step 2: Domain Value Objects (`internal/domain/principal/profile.go`)

**What**: Create `PrincipalProfile` value object and context functions.

**Pattern to follow**: Existing `principal.WithPrincipal(ctx, string)` / `principal.FromContext(ctx)` in `internal/domain/principal/context.go`.

**Key decisions**:
- Add new context key `profileContextKey struct{}` (separate from existing `principalContextKey`)
- Both keys are set by middleware — backward compatibility preserved
- Builder pattern: `NewProfile(principal).WithDisplayName(name).WithEmail(email).WithPictureURL(url)`

### Step 3: Domain Port & CEL Evaluator (`internal/domain/jwtauth/`)

**What**: Create `JWTAuthenticator` port interface, `AuthResult` type, CEL evaluator, and domain errors.

**Pattern to follow**: 
- Port interface: `internal/ports/encryption.go` (EncryptionPort pattern)
- CEL evaluator: `internal/domain/tokenexchange/cel_evaluator.go` (ADR 009 pattern)
  - Same environment setup pattern (`cel.NewEnv` with variable declarations)
  - Same compile-at-startup, evaluate-at-runtime pattern
  - Same timeout enforcement (100ms default via goroutine + channel)
  - Variable name: `claims` instead of `subject_token` (JWT claims map)

**Key decisions**:
- Four optional CEL programs: principal (required), display name, email, picture URL
- Non-string CEL result for optional fields → treat as absent (log warning, don't fail)
- Non-string CEL result for principal → authentication failure (fail-closed)

### Step 4: JWT Adapter (`internal/adapters/jwtauth/jwx_authenticator.go`)

**What**: Implement `JWTAuthenticator` using `lestrrat-go/jwx/v4`.

**Pattern to follow**: 
- JWKS caching: `internal/adapters/jwks/adapter.go` (same library, same cache pattern)
- JWT parsing: `lestrrat-go/jwx/v4/jwt` and `lestrrat-go/jwx/v4/jwk`

**Key decisions**:
- Parse and verify with `jwt.Parse(rawToken, jwt.WithKeySet(keyset, jws.WithInferAlgorithmFromKey(true)), jwt.WithValidate(false))`.
- After signature verification, reject missing or expired `exp` and check configured audience/issuer claims.
- Claims extracted as `map[string]interface{}` → passed to CEL evaluator

### Step 5: Middleware Extension (`internal/adapters/http/middleware/principal_middleware.go`)

**What**: Extend `RequirePrincipalMiddleware` and `OptionalPrincipalMiddleware` with JWT authentication path.

**Pattern to follow**: Existing middleware in the same file. The JWT authenticator is injected as an additional parameter (or via the extended `AuthenticationConfig`).

**Key decisions**:
- JWT authenticator is optional (nil when no JWT config) — injected via builder
- Flow: JWT header present? → Yes: authenticate JWT → Success: set principal + profile → Failure: 401
- JWT header absent while JWT pre-authentication is configured? → No fallback to plain-header pre-authentication; reject on protected routes.
- JWT present but invalid? → Reject with 401; do not use the plain-header value.
- Both `WithPrincipal(ctx, string)` and `WithProfile(ctx, profile)` set on success

### Step 6: Handler Update (`internal/adapters/http/handlers/consent/user_info_handler.go`)

**What**: Read `PrincipalProfile` from context, construct enriched `UserInfo`.

**Pattern to follow**: Existing handler in the same file.

**Key decisions**:
- Try `principal.ProfileFromContext(ctx)` first → if present, use enriched profile
- Fallback to `principal.FromContext(ctx)` → construct minimal `UserInfo` (backward-compatible)
- `UserInfo` now includes `Email *string` field

### Step 7: Builder Wiring (`internal/app/builder.go`)

**What**: Conditionally create JWT authenticator when JWT config is present. Pass to middleware via routing config.

**Pattern to follow**: Conditional service creation at `builder.go` line ~275 (token exchange service pattern).

**Key decisions**:
- If JWT pre-authentication is configured on either server:
  1. Create the CEL evaluator (fail fast on invalid expressions).
  2. Require JWKS verification and an explicit JWKS URI.
  3. Create the JWX authenticator with a ready JWKS cache.
  4. Pass the authenticator to the matching server's route config.
- If JWT config is nil on a server, that server uses plain-header pre-authentication only.

### Step 8: Routing Update (`internal/adapters/http/routing/`)

**What**: Pass each configured server's JWT authenticator to its authentication middleware.

**Pattern to follow**: Existing `EnduserRouteConfig` struct.

**Key decisions**:
- Add a `JWTAuthenticator` field to each server's route configuration (optional when JWT pre-authentication is not configured).
- Pass the authenticator to that server's principal middleware.

### Step 9: OpenAPI Update (`api/enduser/openapi.yaml`)

**What**: Add `email` field to `UserInfo` schema.

**Pattern to follow**: Existing `pictureUrl` field in the same schema.

### Step 10: Frontend Update (`web/src/`)

**What**: Add `email` to TypeScript types, update header display.

**Pattern to follow**: Existing `pictureUrl` handling in `AppLayout.tsx`.

**Key decisions**:
- `email?: string` added to `UserInfo` interface in `web/src/types/consent.ts`
- Header secondary label: show `email` if available, otherwise show `principal` (current behavior)
- No new components — existing Avatar and typography handle all cases

### Step 11: Configuration Examples (`examples/config/`)

**What**: Create `jwt-preauth.yaml` with example configurations. Update `README.md`.

**Pattern to follow**: Existing files in `examples/config/`.

---

## Key Patterns Reference

| Pattern | Example Location | Usage in This Feature |
|---------|-----------------|----------------------|
| CEL compile + evaluate | `internal/domain/tokenexchange/cel_evaluator.go` | Claim extraction from JWT |
| JWKS cache adapter | `internal/adapters/jwks/adapter.go` | JWT signature verification |
| Principal context | `internal/domain/principal/context.go` | Profile context storage |
| Port interface | `internal/ports/encryption.go` | JWTAuthenticator interface |
| Conditional builder wiring | `internal/app/builder.go:275+` | JWT authenticator creation |
| Config validation | `internal/config/validator.go` | Require signed JWKS verification and a JWKS URI |
| E2E test with mock server | `tests/e2e/helpers/mock_upstream.go` | Mock JWKS server |

---

## Testing Checklist

- [ ] Unit tests for CEL evaluator (compile errors, string extraction, non-string handling, timeout)
- [ ] Unit tests for the JWX authenticator (signed, expired, missing `exp`, wrong audience/issuer, unsigned rejection)
- [ ] Unit tests for PrincipalProfile (construction, defaults, context storage/retrieval)
- [ ] Unit tests for config validation (JWKS-only verification, required URI, defaults)
- [ ] Unit tests for middleware (signed JWT, plain-header-only, missing JWT without fallback, invalid JWT rejection)
- [ ] Unit tests for UserInfoHandler (enriched profile, plain profile, backward-compatible)
- [ ] E2E tests for active signed, unsigned-rejection, profile, plain-header, and UI acceptance scenarios
- [ ] Frontend tests for header display (with email, without email, with picture, initials fallback)
