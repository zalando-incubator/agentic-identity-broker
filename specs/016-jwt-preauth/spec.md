# Feature Specification: JWT Pre-Authentication & Principal Profile Enrichment

**Feature Branch**: `016-jwt-preauth`  
**Created**: 2026-02-27  
**Status**: Draft  
**Input**: User description: "Extend pre-auth to accept signed and unsigned JWTs with CEL-based extraction of principal display name, email, and profile picture URL, reflected in the consent UI"

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Accept Signed JWTs for Pre-Authentication (Priority: P1)

An operator deploys the identity broker behind a reverse proxy or API gateway that issues signed JWTs (e.g., oauth2-proxy with `--pass-access-token`, Envoy ext_authz, or custom gateways). Instead of relying solely on a plain-text header for the principal identifier, the broker can now read a JWT from a configurable HTTP header, validate its signature against a JWKS endpoint, and extract the principal using a CEL expression.

**Why this priority**: Signed JWTs are the most common and security-critical authentication mechanism in production environments. Without signature validation, pre-auth relies entirely on network-level trust in the reverse proxy header. JWT support enables defense-in-depth by cryptographically verifying the user's identity.

**Independent Test**: Can be fully tested by configuring `authentication.jwt` with a JWKS endpoint and a signed JWT header, making an API request, and verifying the principal is correctly extracted and the request succeeds. A request with an invalid or expired JWT must be rejected.

**Acceptance Scenarios**:

1. **Given** a server configured with `authentication.jwt` specifying a `jwks_uri`, a JWT `header_name`, and a principal CEL expression, **When** a request arrives with a valid, non-expired, signature-verified JWT in the configured header, **Then** the principal is extracted using the CEL expression and set in the request context.
2. **Given** a server configured with `authentication.jwt`, **When** a request arrives with a JWT whose signature does not match any key in the JWKS, **Then** the request is rejected with 401 Unauthorized.
3. **Given** a server configured with `authentication.jwt`, **When** a request arrives with an expired JWT (the `exp` claim is in the past), **Then** the request is rejected with 401 Unauthorized.
4. **Given** a server configured with `authentication.jwt`, **When** a request arrives with no JWT in the configured header, **Then** the request is rejected with 401 Unauthorized (on protected routes) or proceeds without a principal (on optional routes).
5. **Given** a server configured with `authentication.jwt` with an `expected_audience` value, **When** a request arrives with a JWT whose `aud` claim does not contain the expected audience, **Then** the request is rejected with 401 Unauthorized.
6. **Given** a server configured with `authentication.jwt` with an `expected_issuer` value, **When** a request arrives with a JWT whose `iss` claim does not match the expected issuer, **Then** the request is rejected with 401 Unauthorized.

---

### User Story 2 - Accept Unsigned (Pre-Authenticated) JWTs (Priority: P2)

An operator deploys the broker behind a trusted reverse proxy that issues unsigned JWTs (alg: "none") containing user claims. This is common in service mesh environments (e.g., Istio, Linkerd) where the mesh has already authenticated the user and passes claims as an unsigned JWT. The broker must accept these JWTs when explicitly configured to do so, while rejecting unsigned JWTs by default for security.

**Why this priority**: Supports a valid deployment pattern (service mesh environments) but is lower priority than signed JWTs because unsigned JWTs carry inherent security risk and require careful deployment. This is still critical for completeness of the pre-auth model.

**Independent Test**: Can be tested by configuring `authentication.jwt` with `verification: none` (and no `jwks_uri`) and sending a request with an unsigned JWT containing the principal claim. Must verify that unsigned JWTs are rejected when `verification` is `jwks` or omitted. Must also verify that startup fails if both `verification: none` and `jwks_uri` are specified simultaneously.

**Acceptance Scenarios**:

1. **Given** a server configured with `authentication.jwt` with `verification: none` (and no `jwks_uri`), **When** a request arrives with an unsigned JWT (alg: "none") containing valid claims, **Then** the principal is extracted using the CEL expression and set in the request context.
2. **Given** a server configured with `authentication.jwt` with `verification: jwks` (the default when omitted), **When** a request arrives with an unsigned JWT (alg: "none"), **Then** the request is rejected with 401 Unauthorized.
3. **Given** a server configured with `authentication.jwt` with `verification: none`, **When** a request arrives with a signed JWT, **Then** the JWT is still accepted (signature is not checked), and the principal is extracted from claims.
4. **Given** a server configured with `authentication.jwt` with `verification: none`, **When** a request arrives with a JWT whose `exp` claim is in the past, **Then** the request is rejected with 401 Unauthorized (expiry is always enforced regardless of verification mode).
5. **Given** a server configured with `authentication.jwt` with both `verification: none` and a `jwks_uri` present, **When** the server starts up, **Then** startup fails with a clear configuration error explaining that `verification: none` and `jwks_uri` are mutually exclusive.

---

### User Story 3 - Extract Profile Attributes from JWT Using CEL (Priority: P2)

An operator configures CEL expressions to extract optional user profile attributes—display name, email address, and profile picture URL—from the JWT claims. These attributes enrich the principal context and are surfaced in the `/api/me` endpoint response, enabling the consent UI to display personalized user information.

**Why this priority**: Directly improves user experience by showing human-readable identity instead of raw principal identifiers. Same priority as User Story 2 because it builds on the JWT infrastructure and is the key user-visible improvement.

**Independent Test**: Can be tested by configuring CEL expressions for display_name, email, and picture_url, sending a JWT with matching claims, and verifying the `/api/me` endpoint returns the extracted values.

**Acceptance Scenarios**:

1. **Given** a server configured with JWT pre-auth and CEL expressions for `display_name_expression`, `email_expression`, and `picture_url_expression`, **When** a request arrives with a JWT containing matching claims, **Then** the `/api/me` endpoint returns the extracted display name, email, and picture URL alongside the principal.
2. **Given** a server configured with JWT pre-auth and CEL expressions for profile attributes, **When** a request arrives with a JWT that is missing some of the claims referenced by the CEL expressions, **Then** the missing attributes are omitted from the `/api/me` response (returned as null/absent), but the request still succeeds with the principal extracted.
3. **Given** a server configured with JWT pre-auth but no CEL expressions for profile attributes (only the principal expression is configured), **When** a request arrives with a valid JWT, **Then** the `/api/me` endpoint returns only the principal and derives the display name from the principal (current behavior preserved).
4. **Given** a server configured with plain header-based pre-auth (no JWT), **When** a request arrives with the principal header, **Then** the `/api/me` endpoint returns the principal with the display name derived from the principal and no email or picture URL (backward compatible).

---

### User Story 4 - Display User Profile in Consent UI (Priority: P3)

When profile attributes (display name, email, profile picture) are available from the `/api/me` endpoint, the consent UI header displays them. The display name replaces the raw principal identifier, the email is shown as a secondary label, and the profile picture is rendered as an avatar. If attributes are unavailable, the UI falls back gracefully to the current behavior (showing the principal identifier).

**Why this priority**: This is a UI enhancement that depends on the backend profile extraction (User Stories 1-3). It is independently testable but provides value only when the backend enrichment is in place.

**Independent Test**: Can be tested by mocking the `/api/me` response to include display name, email, and picture URL, then verifying the header renders each attribute. A second test mocks a response without these attributes and verifies the fallback behavior.

**Acceptance Scenarios**:

1. **Given** the `/api/me` endpoint returns a response with display name, email, and picture URL, **When** the consent UI loads, **Then** the header displays the profile picture as an avatar, the display name as the primary label, and the email as the secondary label.
2. **Given** the `/api/me` endpoint returns a response with only the principal (no display name, email, or picture URL), **When** the consent UI loads, **Then** the header displays the principal as the primary label, no avatar image (shows initials fallback), and no secondary label.
3. **Given** the `/api/me` endpoint returns a response with display name and email but no picture URL, **When** the consent UI loads, **Then** the header displays an initials avatar derived from the display name, the display name as the primary label, and the email as the secondary label.

---

### User Story 5 - Backward-Compatible Configuration (Priority: P1)

Operators currently using the plain-header pre-auth mode (principal extracted from `X-Remote-User` or another configured header) must not be affected by this change. The existing configuration must remain valid and functional without any modification. JWT pre-auth is an additive, opt-in capability.

**Why this priority**: Same as P1 because breaking existing deployments is unacceptable. This is a core constraint, not a separate feature.

**Independent Test**: Can be tested by deploying with the current configuration (only `authentication.preauth.principal_header_name` set) and verifying all existing endpoints work identically.

**Acceptance Scenarios**:

1. **Given** a server configured with only `authentication.preauth.principal_header_name` (no `authentication.jwt` section), **When** a request arrives with the principal header, **Then** the principal is extracted from the header and the request succeeds (identical to current behavior).
2. **Given** a server configured with both `authentication.preauth.principal_header_name` and `authentication.jwt`, **When** a request arrives with the JWT header present, **Then** the JWT is used for authentication (JWT takes precedence over the plain header).
3. **Given** a server configured with both `authentication.preauth.principal_header_name` and `authentication.jwt`, **When** a request arrives without the JWT header but with the plain principal header, **Then** the request is rejected with 401 Unauthorized (fail-closed — no fallback to the plain header when JWT is configured).

---

### Edge Cases

- What happens when the JWKS endpoint is unreachable at startup? The system fails to start with a clear error message (fail-closed per Constitution Principle I).
- What happens when the JWKS endpoint becomes unreachable after startup? The system uses cached JWKS keys and logs warnings. If the cache expires and no keys are available, requests requiring JWT validation are rejected with 503 Service Unavailable.
- What happens when a CEL expression for a profile attribute evaluates to a non-string value? The attribute is ignored (treated as absent) and a warning is logged.
- What happens when the JWT header contains a value that is not a valid JWT? The request is rejected with 401 Unauthorized.
- What happens when a JWT has a `kid` not in the cached JWKS? The system relies on `lestrrat-go/jwx`'s built-in refresh behavior. If the key is still not found after any library-initiated refresh, the request is rejected with 401 Unauthorized.
- What happens when the JWT contains no `exp` claim? When `verification` is `jwks` (or unset), the JWT is rejected with 401 Unauthorized (expiry is mandatory). When `verification` is `none`, a missing `exp` is accepted (unsigned JWTs from trusted upstreams may omit it); if `exp` is present, it is still enforced.
- What happens when both JWT and plain header are configured but the JWT is invalid? The request is rejected with 401 Unauthorized. The system does not silently fall back to the plain header when a JWT is present but invalid (fail-closed).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support a new `authentication.jwt` configuration block (sibling of `authentication.preauth`) that enables JWT-based authentication alongside the existing plain-header pre-auth mode.
- **FR-002**: System MUST accept signed JWTs (RS256, RS384, RS512, ES256, ES384, ES512, PS256, PS384, PS512) when `verification` is set to `jwks` (the default when omitted) and a `jwks_uri` is provided.
- **FR-003**: System MUST accept unsigned JWTs (alg: "none") when `verification` is explicitly set to `none`. Unsigned JWTs MUST be rejected when `verification` is `jwks` or not specified.
- **FR-003a**: System MUST reject startup (fail closed) if `verification: none` and `jwks_uri` are both present in `authentication.jwt`, as these options are mutually exclusive and represent contradictory intent. The error message MUST state which keys conflict.
- **FR-004**: System MUST extract the principal from the JWT using a configurable CEL expression (`principal_expression`), defaulting to `claims.sub`.
- **FR-005**: System MUST validate the JWT `exp` claim and reject expired JWTs with 401 Unauthorized. When `verification` is `jwks` (the default), `exp` MUST be present; absent `exp` is rejected with 401. When `verification` is `none`, `exp` is optional — if present, it MUST be valid (not expired); if absent, it is accepted. No clock skew tolerance is applied in either mode; operators must ensure clock synchronization via NTP.
- **FR-006**: System MUST optionally validate the JWT `aud` claim against a configured `expected_audience` value. If configured and the JWT `aud` does not contain the expected value, the request MUST be rejected with 401 Unauthorized.
- **FR-007**: System MUST optionally validate the JWT `iss` claim against a configured `expected_issuer` value. If configured and the JWT `iss` does not match, the request MUST be rejected with 401 Unauthorized.
- **FR-008**: System MUST support optional CEL expressions for extracting profile attributes from JWT claims: `display_name_expression`, `email_expression`, and `picture_url_expression`.
- **FR-009**: When profile attribute CEL expressions are configured and the JWT contains matching claims, the `/api/me` endpoint MUST return the extracted `displayName`, `email`, and `pictureUrl` values.
- **FR-010**: When profile attribute CEL expressions are not configured or the JWT claims do not match, the corresponding fields MUST be absent (null/omitted) in the `/api/me` response, except `displayName` which falls back to the principal value (preserving current behavior).
- **FR-011**: The existing plain-header pre-auth configuration (`authentication.preauth.principal_header_name`) MUST continue to work without modification when no `authentication.jwt` block is present.
- **FR-012**: When both `authentication.jwt` and `authentication.preauth.principal_header_name` are configured, the JWT takes precedence. If the JWT header is present in the request, it is used (and validated). If the JWT header is absent, the request is rejected with 401 Unauthorized (fail-closed — no fallback to the plain header).
- **FR-013**: When both `authentication.jwt` and `authentication.preauth.principal_header_name` are configured and a JWT is present but invalid (bad signature, expired, etc.), the request MUST be rejected with 401 Unauthorized. The system MUST NOT silently fall back to the plain header.
- **FR-014**: All CEL expressions (principal and profile attributes) MUST be validated at startup. Invalid CEL expressions MUST cause startup failure with a descriptive error message.
- **FR-015**: The JWKS endpoint MUST be fetched and validated at startup when `verification` is `jwks`. Unreachable JWKS endpoints MUST cause startup failure.
- **FR-016**: JWKS key fetching and caching MUST use the `lestrrat-go/jwx/v3` library's built-in JWKS cache (`jwk.Cache`), which handles background refresh, HTTP cache header honoring, and stale key serving during refresh. No custom JWKS caching logic is required.
- **FR-017**: The `email` field MUST be added to the `UserInfo` domain type and the `/api/me` API response as an optional field.
- **FR-018**: The consent UI header MUST display the user's email as a secondary label when available, replacing the principal display that currently appears below the display name.
- **FR-019**: The consent UI MUST continue to function correctly when profile attributes are absent (graceful degradation to current behavior).
- **FR-020**: Security-critical operations (JWT validation failures, JWKS fetch errors, unsigned JWT rejection) MUST be logged with structured audit logging.

### Domain Model

**Entities** (things with unique identity):
- No new entities are introduced. This feature enriches the existing **Principal** concept.

**Value Objects** (things without identity):
- **PrincipalProfile**: Enriched user profile extracted from pre-auth context. Contains: `principal` (string, required), `displayName` (string, optional — falls back to principal), `email` (string, optional), `pictureUrl` (string, optional). Immutable once extracted from the authentication source.
- **JWTValidationResult**: Result of JWT parsing and validation. Contains: validated claims map, extracted principal, extracted profile attributes, or validation error. Immutable.

**Domain Events** (state changes of business significance):
- **JWTValidationFailed**: When a JWT fails signature verification, expiry check, audience/issuer validation, or CEL extraction. Includes: failure reason, header name, remote address (for audit logging).

*All domain terms should be added to ARCHITECTURE.md Glossary section*

### Configuration Requirements

**Configuration Parameters**:
- **authentication.jwt.header_name**: (string) HTTP header containing the JWT. Default: "Authorization". When set to "Authorization", the system automatically strips the "Bearer " prefix before parsing the JWT. For any other header name, the raw header value is used as the JWT directly.
- **authentication.jwt.verification**: (string) Verification mode. Default: `jwks` (signature-verified, requires `jwks_uri`). Set to `none` to accept unsigned JWTs (alg: "none") from a trusted upstream without signature verification. **Mutually exclusive with `jwks_uri`**: specifying both `verification: none` and a `jwks_uri` is a configuration error and causes startup failure.
- **authentication.jwt.jwks_uri**: (string) URL of the JWKS endpoint for signature verification. Required when `verification` is `jwks` (or omitted). MUST NOT be set when `verification` is `none`.
- **authentication.jwt.expected_audience**: (string, optional) Expected value in the JWT `aud` claim. If set, JWTs without this audience are rejected.
- **authentication.jwt.expected_issuer**: (string, optional) Expected value in the JWT `iss` claim. If set, JWTs without this issuer are rejected.
- **authentication.jwt.claim_extraction.principal_expression**: (string) CEL expression to extract the principal from JWT claims. Default: "claims.sub".
- **authentication.jwt.claim_extraction.display_name_expression**: (string, optional) CEL expression to extract the user's display name. Example: "claims.name".
- **authentication.jwt.claim_extraction.email_expression**: (string, optional) CEL expression to extract the user's email. Example: "claims.email".
- **authentication.jwt.claim_extraction.picture_url_expression**: (string, optional) CEL expression to extract the user's profile picture URL. Example: "claims.picture".

**Example YAML Configuration**:
```yaml
# Signed JWT validation with profile enrichment (production)
# verification defaults to "jwks" when omitted
server:
  enduser:
    port: 8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User   # fallback when JWT header absent
      jwt:
        header_name: Authorization
        # verification: jwks  # default — omit or set explicitly
        jwks_uri: https://auth.example.com/.well-known/jwks.json
        expected_audience: agentic-identity-broker
        expected_issuer: https://auth.example.com
        claim_extraction:
          principal_expression: "claims.sub"
          display_name_expression: "claims.name"
          email_expression: "claims.email"
          picture_url_expression: "claims.picture"
```

```yaml
# Unsigned JWT (service mesh / trusted sidecar injects claims)
# jwks_uri MUST NOT be set when verification is "none" — startup will fail if both are present
server:
  enduser:
    port: 8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User
      jwt:
        header_name: X-JWT-Claims
        verification: none
        claim_extraction:
          principal_expression: "claims.sub"
          display_name_expression: "claims.preferred_username"
          email_expression: "claims.email"
```

```yaml
# INVALID — startup fails: verification: none and jwks_uri are mutually exclusive
server:
  enduser:
    port: 8000
    authentication:
      jwt:
        verification: none
        jwks_uri: https://auth.example.com/.well-known/jwks.json  # ERROR
        claim_extraction:
          principal_expression: "claims.sub"
```

```yaml
# Existing config (unchanged, fully backward compatible)
server:
  enduser:
    port: 8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User
```

**Configuration Location**: Will be added to `examples/config/jwt-preauth.yaml` and referenced in `examples/config/README.md`

### API Requirements

- **API-001**: The `/api/me` endpoint response schema MUST be extended to include an optional `email` field (type: string, nullable).
- **API-002**: The existing `displayName` field behavior MUST be preserved: when no display name is extractable, it defaults to the principal value.
- **API-003**: The existing `pictureUrl` field behavior MUST be preserved: it remains optional/nullable.
- **API-004**: The OpenAPI specification at `/api/enduser/openapi.yaml` MUST be updated to document the new `email` field on the `UserInfo` schema.
- **API-005**: No new endpoints are required. Changes are limited to the response schema of the existing `GET /api/me` endpoint.
- **API-006**: All API changes MUST be confirmed by user/stakeholder before implementation begins.

### Security Requirements

- **SR-001**: Unsigned JWTs (alg: "none") MUST be rejected by default. Accepting unsigned JWTs MUST require explicit opt-in via `verification: none`.
- **SR-002**: JWT signature verification MUST use `lestrrat-go/jwx/v3` (already a project dependency) per Constitution Principle III. This library provides JWT parsing, signature verification, JWKS fetching with caching, and algorithm allowlisting.
- **SR-003**: The system MUST fail closed on JWT validation failures: invalid signature, expired token, audience mismatch, issuer mismatch, or CEL extraction failure MUST all result in 401 Unauthorized.
- **SR-004**: JWKS fetching MUST use HTTPS in production. Allowing HTTP for JWKS URIs MUST require the existing `skip_thirdparty_https_validation` configuration flag to be set to `true`. When `skip_thirdparty_https_validation` is `false` (the default), `jwks_uri` values with an `http://` scheme MUST cause startup failure with a descriptive error message.
- **SR-005**: JWT validation failures MUST emit structured audit logs including: failure reason, JWT header name, remote address, and timestamp.
- **SR-006**: CEL expressions for profile extraction MUST NOT be able to modify state or perform I/O operations (CEL's sandboxed evaluation model inherently enforces this).
- **SR-007**: Profile picture URLs extracted from JWTs MUST be treated as untrusted input. The frontend MUST render them as `<img>` src attributes only (no script injection via URL).

### Frontend/Design System Requirements

> **Historical visual scope**: The token names below record this feature's original design choice, not a current constitutional aesthetic mandate; in particular, `slate-600` is not a current semantic-token recommendation. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**Design System Compliance**:
- All frontend changes MUST use the existing design system components.
- The `Avatar` component from `@design-system/components/primitives/Avatar` is already in use and supports profile pictures.

**Component Classification**:
- **Application-Specific Components**: Updates to the `Header` component in `web/src/components/layout/AppLayout.tsx`
  - Must use existing `Avatar` design system primitive
- **No new universal components required**: All needed primitives (Avatar, typography) already exist

**Design Tokens Usage**:
- Use existing semantic tokens: `trust-deep` for primary text, `slate-600` for secondary text (email label)

**Accessibility Requirements**:
- Profile picture `alt` text MUST be set to the display name
- Email text MUST be readable by screen readers
- Layout changes MUST maintain keyboard navigation support

### Key Entities

- **PrincipalProfile**: Enriched user identity containing principal identifier (string, required), display name (string, defaults to principal), email (string, optional), picture URL (string, optional). Extracted from pre-auth source (JWT or plain header). Used by `/api/me` endpoint and rendered in consent UI header.
- **JWTAuthConfig**: Configuration value object defining JWT authentication behavior: header name, verification mode (`jwks` or `none`), JWKS URI (present only when verification is `jwks`), audience/issuer constraints, and CEL extraction expressions. Validation at startup enforces mutual exclusivity of `verification: none` and `jwks_uri`.

*Domain concepts should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can configure JWT-based pre-authentication using a declarative YAML configuration block, and the broker successfully authenticates users using signed JWTs from any standard OAuth2 provider.
- **SC-002**: Operators using the existing plain-header pre-auth configuration experience zero changes in behavior after upgrading (full backward compatibility).
- **SC-003**: Users see their display name, email, and profile picture in the consent UI header when the operator has configured JWT pre-auth with profile extraction CEL expressions.
- **SC-004**: Invalid JWTs (expired, wrong signature, wrong audience/issuer) are rejected within 50ms, and the rejection reason is logged in structured audit format.
- **SC-005**: The consent UI gracefully degrades when profile attributes are unavailable, showing the principal identifier as the display name and an initials avatar (matching current behavior).
- **SC-006**: All CEL configuration expressions are validated at startup, with clear error messages for invalid expressions, preventing misconfiguration from reaching production.

## Clarifications

### Session 2026-02-27

- Q: What JWKS cache refresh strategy should be used? → A: Use the existing JWKS caching mechanism provided by lestrrat-go/jwx/v3 (already a project dependency). No custom cache logic needed.
- Q: Should JWT expiry validation include a clock skew tolerance? → A: Zero tolerance (library default). Clocks must be synchronized via NTP; no configurable skew allowance.
- Q: How should the 'Bearer ' prefix be handled when extracting the JWT from the HTTP header? → A: Auto-detect by header name. When header_name is 'Authorization', strip 'Bearer ' prefix automatically. For any other header name, expect raw JWT.
- Q: What should happen when a JWT has a 'kid' not found in the cached JWKS? → A: Rely on lestrrat-go/jwx's built-in refresh behavior (library default). No custom on-demand refresh logic.
- Q: What configuration structure should be used for JWT authentication, and how should signed vs unsigned JWT modes be expressed? → A: Promote `authentication.jwt` as a sibling of `authentication.preauth` (Proposal 2). Single `jwt` block with a `verification` field (`jwks` | `none`; default `jwks`). `verification: none` and `jwks_uri` are mutually exclusive — specifying both is a startup-time configuration error with a descriptive message. `authentication.preauth` is reduced to plain-header trust only.

## Assumptions

- The reverse proxy or API gateway is trusted to set the JWT header correctly. JWT signature verification provides defense-in-depth but does not replace network-level trust.
- JWKS endpoints follow the standard RFC 7517 format and are reachable over HTTPS in production.
- CEL expressions for profile extraction follow the same pattern as the existing `token_exchange.claim_extraction` CEL expressions (ADR 009).
- The `claims` variable in CEL expressions is a map of JWT claims (same pattern as `subject_token` in token exchange configuration).
- JWT `exp` claim is always required and always validated (no configuration to skip expiry validation, per security-first principle).
- The `email` field addition to `/api/me` is a non-breaking API change (additive, optional field).
- Profile picture URLs in JWTs are standard HTTPS URLs. No image proxying or caching is required on the backend.