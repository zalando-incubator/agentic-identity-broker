# Data Model: JWT Pre-Authentication & Principal Profile Enrichment

**Feature**: 016-jwt-preauth  
**Date**: 2026-02-27

---

## Overview

This feature introduces no new persistent entities. All changes are to **value objects** (request-scoped, immutable) and **configuration types**. The domain model enriches the existing Principal concept with optional profile attributes extracted from JWT claims.

---

## Value Objects

### PrincipalProfile

**Purpose**: Enriched user identity extracted from the pre-authentication source (JWT or plain header). Immutable once constructed. Carried through request context and surfaced via `/api/me`.

**Location**: `internal/domain/principal/profile.go`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `Principal` | `string` | Yes | — | Unique user identifier (e.g., email, username, sub claim). Max 200 chars. |
| `DisplayName` | `string` | Yes | Value of `Principal` | Human-readable display name. Falls back to principal if not extracted. |
| `Email` | `*string` | No | `nil` | User's email address. Nil if not extracted from JWT or not configured. |
| `PictureURL` | `*string` | No | `nil` | URL to user's profile picture. Nil if not extracted. Treated as untrusted input. |

**Construction**:
```go
// From JWT with profile extraction
profile := principal.NewProfile(principalValue).
    WithDisplayName(displayName).
    WithEmail(email).
    WithPictureURL(pictureURL)

// From plain header (backward-compatible)
profile := principal.NewProfile(principalValue)
// DisplayName defaults to principalValue, Email and PictureURL are nil
```

**Invariants**:
- `Principal` must be non-empty and ≤ 200 characters
- `DisplayName` is never empty — defaults to `Principal` if not explicitly set
- `Email` and `PictureURL` are truly optional (nil when absent, not empty string)
- Immutable after construction (builder pattern returns new instances)

**Context Storage**:
- Stored via `principal.WithProfile(ctx, profile) context.Context`
- Retrieved via `principal.ProfileFromContext(ctx) (PrincipalProfile, bool)`
- **Parallel** to existing `principal.WithPrincipal(ctx, string)` — both are set; backward compatibility preserved

---

### AuthResult

**Purpose**: Result of JWT parsing, validation, and claim extraction. Produced by the `JWTAuthenticator` adapter, consumed by the principal middleware.

**Location**: `internal/domain/jwtauth/auth_result.go`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Principal` | `string` | Yes | Extracted principal (from CEL expression, default: `claims.sub`) |
| `DisplayName` | `*string` | No | Extracted display name (from CEL expression, if configured) |
| `Email` | `*string` | No | Extracted email (from CEL expression, if configured) |
| `PictureURL` | `*string` | No | Extracted picture URL (from CEL expression, if configured) |
| `Claims` | `map[string]interface{}` | Yes | Full JWT claims map (for audit logging) |

**Invariants**:
- `Principal` must be non-empty (CEL extraction must produce a non-empty string)
- Optional fields are `nil` when the CEL expression is not configured or evaluates to non-string
- `Claims` is never nil (even empty JWT has at least header claims)

---

## Configuration Types

### JWTConfig

**Purpose**: Configuration for JWT-based pre-authentication. Sibling of existing `PreauthConfig` within `AuthenticationConfig`.

**Location**: `internal/ports/config.go`

| Field | Type | Required | Default | Validation |
|-------|------|----------|---------|------------|
| `HeaderName` | `string` | No | `"Authorization"` | Min 1 char |
| `Verification` | `string` | No | `"jwks"` | Must be `"jwks"` or `"none"` |
| `JWKSURI` | `string` | Conditional | — | Required when `Verification` is `"jwks"`. Must be valid URL. Forbidden when `Verification` is `"none"`. |
| `ExpectedAudience` | `string` | No | — | If set, JWT `aud` must contain this value |
| `ExpectedIssuer` | `string` | No | — | If set, JWT `iss` must match |
| `ClaimExtraction` | `JWTClaimExtractionConfig` | No | See below | CEL expressions for claim extraction |

**Mutual Exclusivity**: `Verification: "none"` + `JWKSURI` present → startup error (FR-003a).

```go
type JWTConfig struct {
    HeaderName       string                   `mapstructure:"header_name"`
    Verification     string                   `mapstructure:"verification"`
    JWKSURI          string                   `mapstructure:"jwks_uri"`
    ExpectedAudience string                   `mapstructure:"expected_audience"`
    ExpectedIssuer   string                   `mapstructure:"expected_issuer"`
    ClaimExtraction  JWTClaimExtractionConfig  `mapstructure:"claim_extraction"`
}
```

### JWTClaimExtractionConfig

**Purpose**: CEL expressions for extracting principal and optional profile attributes from JWT claims.

**Location**: `internal/ports/config.go`

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `PrincipalExpression` | `string` | No | `"claims.sub"` | CEL expression for principal extraction |
| `DisplayNameExpression` | `string` | No | — | CEL expression for display name (e.g., `"claims.name"`) |
| `EmailExpression` | `string` | No | — | CEL expression for email (e.g., `"claims.email"`) |
| `PictureURLExpression` | `string` | No | — | CEL expression for picture URL (e.g., `"claims.picture"`) |

```go
type JWTClaimExtractionConfig struct {
    PrincipalExpression     string `mapstructure:"principal_expression"`
    DisplayNameExpression   string `mapstructure:"display_name_expression"`
    EmailExpression         string `mapstructure:"email_expression"`
    PictureURLExpression    string `mapstructure:"picture_url_expression"`
}
```

**CEL Environment**: All expressions receive a single variable `claims` of type `map(string, any)` — the JWT claims map. This follows the pattern from ADR 009 (CEL for Authorization Policies) where token exchange uses `subject_token` and `client_assertion` variables.

---

### AuthenticationConfig (Modified)

**Purpose**: Extends existing `AuthenticationConfig` to include optional JWT configuration.

**Location**: `internal/ports/config.go`

```go
type AuthenticationConfig struct {
    Preauth PreauthConfig `mapstructure:"preauth"`
    JWT     *JWTConfig    `mapstructure:"jwt"`  // NEW — pointer for optional presence detection
}
```

The `JWT` field is a **pointer** (`*JWTConfig`) so that `nil` indicates "not configured" (backward-compatible with existing deployments that have no `authentication.jwt` block).

---

## Domain Types (Modified)

### UserInfo (Modified)

**Purpose**: User information returned by `/api/me`. Extended with `Email` field.

**Location**: `internal/domain/consent/user_info.go`

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `Principal` | `string` | Yes | Unique user identifier |
| `DisplayName` | `string` | Yes | Human-readable name (falls back to principal) |
| `Email` | `*string` | No | **NEW** — User's email address |
| `PictureURL` | `*string` | No | Profile picture URL |

```go
type UserInfo struct {
    Principal   string  `json:"principal"`
    DisplayName string  `json:"displayName"`
    Email       *string `json:"email,omitempty"`      // NEW
    PictureURL  *string `json:"pictureUrl,omitempty"`
}
```

---

## Port Interface

### JWTAuthenticator

**Purpose**: Domain port for JWT authentication. Abstracts JWT parsing, signature verification, and claim extraction from the middleware layer.

**Location**: `internal/domain/jwtauth/authenticator.go`

```go
type JWTAuthenticator interface {
    // Authenticate parses, validates, and extracts claims from a raw JWT string.
    // Returns AuthResult on success or an error describing the validation failure.
    // Errors: ErrInvalidSignature, ErrTokenExpired, ErrAudienceMismatch,
    //         ErrIssuerMismatch, ErrClaimExtraction, ErrMalformedToken
    Authenticate(ctx context.Context, rawJWT string) (*AuthResult, error)
}
```

**Adapter**: `internal/adapters/jwtauth/jwx_authenticator.go` — implements `JWTAuthenticator` using `lestrrat-go/jwx/v3`.

---

## Domain Events

### JWTValidationFailed

**Purpose**: Audit event emitted when JWT validation fails. Logged as structured JSON for security monitoring.

| Field | Type | Description |
|-------|------|-------------|
| `Reason` | `string` | Failure category (e.g., "invalid_signature", "token_expired", "audience_mismatch") |
| `HeaderName` | `string` | HTTP header the JWT was extracted from |
| `RemoteAddr` | `string` | Client IP address |
| `Error` | `string` | Detailed error message |

Not persisted — emitted as structured log entries per FR-020/SR-005.

---

## Entity Relationship Diagram

```
┌─────────────────────────────────┐
│      AuthenticationConfig       │
│  ┌───────────────────────────┐  │
│  │     PreauthConfig         │  │
│  │  principal_header_name    │  │
│  └───────────────────────────┘  │
│  ┌───────────────────────────┐  │
│  │     JWTConfig (NEW)       │  │
│  │  header_name              │  │
│  │  verification             │  │
│  │  jwks_uri                 │  │
│  │  expected_audience        │  │
│  │  expected_issuer          │  │
│  │  ┌─────────────────────┐  │  │
│  │  │ ClaimExtractionCfg  │  │  │
│  │  │ principal_expression│  │  │
│  │  │ display_name_expr   │  │  │
│  │  │ email_expression    │  │  │
│  │  │ picture_url_expr    │  │  │
│  │  └─────────────────────┘  │  │
│  └───────────────────────────┘  │
└─────────────────────────────────┘
           │ configures
           ▼
┌─────────────────────────────────┐
│     Principal Middleware        │
│  (RequirePrincipalMiddleware)   │
│                                 │
│  1. Check JWT header present?   │
│     ├── Yes → JWTAuthenticator  │──────────┐
│     │         .Authenticate()   │          │
│     │         → AuthResult      │          ▼
│     │         → WithPrincipal() │  ┌──────────────────┐
│     │         → WithProfile()   │  │ JWTAuthenticator  │
│     ├── No → Fallback to       │  │  (Port Interface) │
│     │         plain header      │  └──────────────────┘
│     │         → WithPrincipal() │          │
│     │         (no profile)      │          │ implemented by
│     └── Invalid → 401           │          ▼
└─────────────────────────────────┘  ┌──────────────────┐
           │ sets context          │ jwxAuthenticator  │
           ▼                       │  (Adapter)        │
┌─────────────────────────────────┐  │  lestrrat-go/jwx │
│     PrincipalProfile            │  │  + CEL Evaluator │
│  principal: string (required)   │  └──────────────────┘
│  displayName: string (=principal│
│  email: *string (optional)      │
│  pictureURL: *string (optional) │
└─────────────────────────────────┘
           │ read by
           ▼
┌─────────────────────────────────┐
│     UserInfoHandler             │
│  GET /api/me                    │
│  → ProfileFromContext(ctx)      │
│  → UserInfo { principal,        │
│       displayName, email,       │
│       pictureUrl }              │
└─────────────────────────────────┘
           │ consumed by
           ▼
┌─────────────────────────────────┐
│     Consent UI Header           │
│  Avatar (pictureUrl / initials) │
│  Display Name (primary)         │
│  Email (secondary label)        │
└─────────────────────────────────┘
```

---

## Glossary Additions (for ARCHITECTURE.md)

| Term | Definition |
|------|-----------|
| **PrincipalProfile** | Enriched user identity value object containing principal identifier, display name, email, and picture URL. Extracted from pre-authentication source (JWT or plain header). Request-scoped, immutable. Stored in request context via `principal.WithProfile()`. |
| **JWTAuthConfig** | Configuration value object defining JWT-based pre-authentication behavior: HTTP header name, verification mode (`jwks` or `none`), JWKS endpoint, audience/issuer constraints, and CEL claim extraction expressions. Validated at startup with mutual exclusivity rules. |
| **JWTAuthenticator** | Port interface for JWT authentication in the pre-auth layer. Abstracts JWT parsing, signature verification (JWKS or none), temporal validation, and CEL-based claim extraction. Implemented by jwx adapter. |
| **JWTVerificationMode** | String enum (`"jwks"` or `"none"`) controlling JWT signature verification behavior. `"jwks"` (default) requires JWKS URI and validates signatures. `"none"` accepts unsigned JWTs for trusted upstream environments (service mesh). |
| **JWTValidationFailed** | Domain event emitted when JWT pre-authentication fails. Contains failure reason, header name, and remote address. Logged as structured audit data for security monitoring (SR-005). |
