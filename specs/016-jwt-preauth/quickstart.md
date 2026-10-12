# Quickstart: JWT Pre-Authentication & Principal Profile Enrichment

**Feature**: 016-jwt-preauth  
**Date**: 2026-02-27

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
- `Verification` defaults to `"jwks"` — set in `defaultConfig()` function in `schema.go`
- `PrincipalExpression` defaults to `"claims.sub"`

**Validation** (in `internal/config/schema.go`):
- `verification: none` + `jwks_uri` present → startup error (FR-003a)
- `verification: jwks` + `jwks_uri` empty → startup error
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

**What**: Implement `JWTAuthenticator` using `lestrrat-go/jwx/v3`.

**Pattern to follow**: 
- JWKS caching: `internal/adapters/jwks/adapter.go` (same library, same cache pattern)
- JWT parsing: `lestrrat-go/jwx/v3/jwt` and `lestrrat-go/jwx/v3/jwk`

**Key decisions**:
- Signed mode (`jwks`): `jwt.Parse(rawToken, jwt.WithKeySet(keyset, jws.WithInferAlgorithmFromKey(true)))` + `jwt.WithValidate(true)`
- Unsigned mode (`none`): `jwt.Parse(rawToken, jwt.WithVerify(false))` + `jwt.WithValidate(true)`
- Expiry always validated via `jwt.WithValidate(true)` (handles `exp` check)
- Audience/issuer validated via parsed token's claims after parsing
- Claims extracted as `map[string]interface{}` → passed to CEL evaluator

### Step 5: Middleware Extension (`internal/adapters/http/middleware/principal_middleware.go`)

**What**: Extend `RequirePrincipalMiddleware` and `OptionalPrincipalMiddleware` with JWT authentication path.

**Pattern to follow**: Existing middleware in the same file. The JWT authenticator is injected as an additional parameter (or via the extended `AuthenticationConfig`).

**Key decisions**:
- JWT authenticator is optional (nil when no JWT config) — injected via builder
- Flow: JWT header present? → Yes: authenticate JWT → Success: set principal + profile → Failure: 401
- JWT header absent? → Fallback to plain header (existing logic)
- JWT present but invalid? → 401, NO fallback to plain header (FR-013)
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
- If `config.Server.Enduser.Authentication.JWT != nil`:
  1. Create CEL evaluator (fail-fast on invalid expressions)
  2. Create JWKS adapter if verification is `jwks` (fail-fast if JWKS unreachable)
  3. Create jwx authenticator with CEL evaluator + JWKS adapter
  4. Pass authenticator to enduser route config
- If JWT config is nil: no authenticator created (plain-header only)

### Step 8: Routing Update (`internal/adapters/http/routing/enduser.go`)

**What**: Pass JWT authenticator to middleware via `EnduserRouteConfig`.

**Pattern to follow**: Existing `EnduserRouteConfig` struct.

**Key decisions**:
- Add `JWTAuthenticator jwtauth.JWTAuthenticator` field to `EnduserRouteConfig` (optional, nil when not configured)
- Pass to `RequirePrincipalMiddleware` and `OptionalPrincipalMiddleware`

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
| Config validation | `internal/config/schema.go` | Mutual exclusivity rules |
| E2E test with mock server | `tests/e2e/helpers/mock_upstream.go` | Mock JWKS server |

---

## Testing Checklist

- [ ] Unit tests for CEL evaluator (compile errors, string extraction, non-string handling, timeout)
- [ ] Unit tests for jwx authenticator (signed/unsigned, expired, bad audience/issuer, bad signature)
- [ ] Unit tests for PrincipalProfile (construction, defaults, context storage/retrieval)
- [ ] Unit tests for config validation (mutual exclusivity, required fields, defaults)
- [ ] Unit tests for middleware (JWT path, fallback path, invalid JWT no-fallback)
- [ ] Unit tests for UserInfoHandler (enriched profile, plain profile, backward-compatible)
- [ ] E2E tests for all 22 acceptance scenarios (Ginkgo/Gomega + Go Playwright for US4 frontend)
- [ ] Frontend tests for header display (with email, without email, with picture, initials fallback)
