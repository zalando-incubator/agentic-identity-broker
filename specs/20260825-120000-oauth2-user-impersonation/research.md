# Phase 0 Research: OAuth2 User Impersonation

**Feature**: 20260825-120000-oauth2-user-impersonation
**Date**: 2026-08-25
**Spec**: [spec.md](spec.md)

This document resolves every NEEDS CLARIFICATION from Technical Context by grounding the design
in the existing codebase. Each decision cites the concrete files/symbols the implementation will
touch or reuse.

---

## Governance basis

- **Decision**: The unsigned (`alg:none`) unverified subject exception is governed by accepted
  ADR [20260825-120000-unverified-subject-unsigned-jwt.md](../../adrs/20260825-120000-unverified-subject-unsigned-jwt.md).
- **Rationale**: Constitution Principle I ("signature validation never optional") is binding;
  Principle II requires a superseding ADR/amendment to deviate. ADR 20260825-120000 and its paired Constitution
  Principle I carve-out record the bounded exception (subject role only), the trust rationale (no
  issuer signature conveys trust on a caller-asserted subject), and six startup-enforced compensating
  controls. They merge with this feature under reviewing-maintainer approval.
- **Alternatives considered**: (a) Drop FR-003d entirely (rejected — removes the chat-bridge use
  case the feature exists to serve); (b) require a signed subject always (rejected — no issuer
  exists for a chat user).

---

## R1: Where impersonation activates in the token endpoint

- **Decision**: `(*enduser.OAuth2TokenHandler).handleTokenExchange` resolves target routing after
  form parsing and before the legacy mandatory-resource guard. Activation is exactly one
  `<impersonation.audience_prefix>/<agent UUID or canonical ID>` audience. A bare suffix or one
  matching neither identifier form returns `invalid_request`; an unknown target of either form
  returns `invalid_target`; absent/different/multiple audiences retain third-party routing.
- **Rationale**: Target resolution must precede generic guards while preserving unselected exchange
  behavior. The target agent supplies minted `agent_id` and local CEL `agent.*`; the assertion is
  privileged-client authorization/audit identity only.
- **Alternatives considered**: A new endpoint (rejected — FR-002 mandates the existing endpoint);
  routing middleware (rejected — activation depends on parsed form values).

## R2: Signed-credential JWT validation + per-issuer algorithm allow-list

- **Decision**: Introduce an impersonation-scoped signed-JWT validator that reuses the JWKS
  adapter (`internal/adapters/jwks/adapter.go`, httprc-backed) and the `parseWithPolicy` verify
  path pattern from `internal/domain/tokenexchange/jwt_validator.go:402-434`
  (`jwt.WithVerify(true)` + `jwt.WithKeySet` + issuer/audience/exp/nbf), but adds an **explicit
  per-issuer asymmetric algorithm allow-list** enforced against the JWS protected header `alg`.
- **Rationale**: CR-007/clarification require each issuer to list permitted algorithms; `none` and
  `HS*` always rejected; approved set is `RS256/384/512, PS256/384/512, ES256/384/512, EdDSA`. The
  current `JWTValidationPolicy` has **no** algorithm field and no header `alg` check — the adapter
  is transport/cache only (`adapter_test.go` builds HS256 JWKs successfully), so this must be added,
  not reused. Validation is scoped to impersonation issuers so existing token-exchange behavior is
  unchanged (clarification).
- **Alternatives considered**: Rely on `jwt.WithKeySet` matching the JWK's own `alg` (rejected — an
  issuer JWKS could advertise `HS*`; not a real allow-list). A global allow-list (rejected — spec
  requires per-issuer, per-rule scoping).

## R3: Unsigned unverified subject parsing (FR-003d)

- **Decision**: Parse the unverified `subject_token` with an **isolated** parser that uses
  `jwt.ParseInsecure` (JWX 3.1.1, `jwt/jwt.go:146-170`) AND additionally asserts protected-header
  `alg == "none"` with an empty compact signature segment; read claims without verification; never
  resolve against a trusted issuer. This parser is reachable only when a matching rule declares
  `verification: none`.
- **Rationale**: FR-003d/FR-005 require reading claims without signature verification, bounded to
  the unverified subject role. `jwt.ParseInsecure` accepts *any* JWS unverified, so the extra
  `alg==none` + empty-signature assertion prevents a signed token from sneaking through this route.
  The existing pre-auth `verification:"none"` path (`internal/adapters/jwtauth/jwx_authenticator.go`)
  MUST NOT be reused for any signed role.
- **Alternatives considered**: Plaintext identifier (rejected by clarification — must carry multiple
  attributes: principal id + email); accept any unverified JWS (rejected — would let an attacker
  submit a real signed token on the unverified route and bypass issuer scoping).

## R4: CEL authorization + startup subject-binding check (FR-007/FR-007a)

- **Decision**: Reuse the ADR-20260116-120100 CEL machinery pattern from
  `internal/domain/tokenexchange/cel_evaluator.go`, but build an impersonation-scoped evaluator per
  rule whose environment declares exactly five variables: `client_assertion`, `actor_token`,
  `subject_token`, `subject_unverified` (bool), `request` (map with `resource`/`grant_type`/
  `scope`). Retain the checked AST (`env.Check` result) and, for rules that accept the
  unverified subject mode, verify at startup that the compiled predicate references
  `subject_token` via `checked.NativeRep().ReferenceMap()` (cel-go 0.28.1,
  `common/ast.ReferenceInfo.Name`); fail startup otherwise.
- **Rationale**: FR-007a/CR-005a require the subject binding to be verified from the compiled
  expression at startup. The current evaluator discards the checked AST and exposes no
  reference-enumeration API — this capability must be added. Claims are dynamic
  `map[string]interface{}` (`jwtToClaims`, `service.go:516-553`); the unverified subject exposes
  its claims through the same `subject_token` variable.
- **Alternatives considered**: A separate unverified-subject predicate (rejected by clarification — one
  predicate governs both client and unverified subject); runtime-only binding check (rejected —
  CR-005a mandates startup failure).

## R5: Identity extraction

- **Decision**: Each rule declares per-role CEL extraction expressions producing a
  non-empty string identity (client-assertion identity, actor identity, subject identity); the
  subject role additionally supports an optional email extraction. The unverified subject's
  extraction expressions live on the same subject role (verification `none`). Extraction reuses the
  `ExtractPrincipal`-style pattern (`cel_evaluator.go:247-299`) that binds one claims map and
  returns a string; empty/failed extraction rejects the request (FR-006).
- **Rationale**: FR-006 requires non-empty identities from each credential via the selected rule's
  per-role extraction rules; conflicting/empty single-identity resolution rejects. Actor==subject is
  permitted (no distinctness check).

## R6: Local broker-token minting with `sub`, `act.iss`/`act.sub`, `aud`, and granted scope (FR-006a/FR-008/FR-009)

- **Decision**: The local issuer consumes an `ImpersonationMintInput` containing subject, validated actor-token
  issuer, actor, optional email, validated scopes, and the registered target agent. It builds an ephemeral fosite
  requester whose client is that target, grant type is token-exchange, and requested/granted scopes are the
  validated values. The normal local mint path writes target `agent_id`, scope, and local-policy claims;
  impersonation adds protected `act = {"iss": actorIssuer, "sub": actor}`. `token_claims_expression` emits
  `aud` or omits it.
- **Rationale**: Reusing the normal requester context gives the policy actual target `agent.*` and
  token-exchange request context without turning the privileged client into a broker agent. Target
  `AllowedScopes` retains the established empty-list-is-unrestricted behavior, and normal policy already
  owns issuer, TTL, signing, and allowed custom claims.
- **Alternatives considered**: Emit `aud`/`act`/subject via policy CEL (rejected —
  `strategies_test.go:234-254` proves CEL list `aud` fails JWX construction, and CEL has no actor
  variable); a second token issuer (rejected — duplicates normal local issuance).

## R7: Configuration placement and shape

- **Decision**: Add optional `Impersonation *ImpersonationConfig` to `OAuth2AuthServerConfig`.
  `ImpersonationConfig` holds routing-only `AudiencePrefix string` plus ordered rules. Each request
  resolves a canonical audience suffix through the existing `ports.AgentRepository`; it introduces
  no persisted entity or new ID type. Static validation runs in `config.Validate`; startup DI
  creates rule validators and the target-aware service in `internal/app/builder.go`. Errors retain
  indexed `domain/config.ConfigError` paths.
- **Rationale**: The authoritative schema is `internal/ports/config.go`. Nesting under the auth
  server preserves local-only enforcement and distinguishes absent configuration from fail-closed
  invalid configuration.
- **Alternatives considered**: Top-level sibling of `token_exchange` (rejected — spec fixes the path
  under the auth-server block and ties activation to local mode); a global issuer map (rejected by
  clarification — per-rule atomicity, first-match order).

## R8: Local-mode-only enforcement

- **Decision**: Reject any impersonation configuration outside local mode. In local mode, a valid
  suffixed audience resolves an existing target before impersonation processing; proxy and hybrid
  cannot forward it upstream.
- **Rationale**: FR-001/CR-006 keep the local minting boundary strict. `OAuth2AuthServerConfig`
  already resolves the server mode.

## R9: Structured audit (FR-012)

- **Decision**: Emit one credential-free `slog` event with a fixed message
  `impersonation_decision` at the token-exchange boundary
  (`internal/adapters/http/enduser/oauth2_token.go`), following the existing flat-attribute
  convention (`internal/domain/oauth2session/service.go` audit events). Whitelist fields: `outcome`,
  failure category, rule evaluation outcome, `audience`, selected `rule` (nullable),
  trusted issuer identifiers + roles, available `privileged_client_identity`/`actor_identity`/
  `subject_identity`, OAuth error code, and correlation (`request_id`). To obtain `request_id`, wrap
  `POST /oauth2/token` with the existing `OAuth2AuditMiddleware`
  (`internal/adapters/http/middleware/oauth2_audit.go`) and export a safe request-ID accessor
  (currently the context key is private and only `GET /oauth2/authorize` is wrapped).
- **Rationale**: No `AuditPort` exists; the codebase standard is injected `*slog.Logger` with an
  `event` attribute. FR-012/SC-005 require no credential values in events.
- **Alternatives considered**: A new audit port/adapter (rejected — inconsistent with existing
  convention; out of scope).

## R10: Persistence / migrations / typed IDs

- **Decision**: **None required.** No new persistent entity, migration, repository port, or ADR-20260313-120000
  typed ID. Impersonation is stateless; it reuses already-persisted broker signing keys only.
  Subject/actor/client identities are external stable strings mapping to existing `id.Principal`.
- **Rationale**: Spec Key Entities are request/config/decision values; the request never persists
  raw credential values (spec Key Entities). Confirmed by TokenMintAndAudit scout evidence.

## R11: Error taxonomy and no-match precedence (FR-004a/FR-011)

- **Decision**: Reuse `tokenexchange.TokenExchangeError` envelope constructors
  (`invalid_request`→400, `invalid_client`→401, `access_denied`→403; add `server_error` mapping for
  fail-closed internal errors). No-match precedence (order-independent): (1) any rule validated
  all credentials but its predicate returned false → `access_denied`; (2) else invalid client
  assertion → `invalid_client`; (3) else invalid actor/subject/malformed → `invalid_request`.
  Errors never reveal token values, key material, or trust internals.
- **Rationale**: FR-011 fixes error codes per failure class; FR-004a fixes the deterministic
  precedence on exhaustion.

## R12: RFC 8693 request/response shape for impersonation

- **Decision**: Reject `resource` (FR-002a) with `invalid_request`. Accept optional `scope`; split it on
  literal spaces and validate it against the resolved target agent's `AllowedScopes`, where an empty
  allow-list is unrestricted and reserved refresh-token scopes retain normal handling. A denied value
  returns credential-free `invalid_scope`. Accept optional `requested_token_type`; if present it must
  equal the access-token type else `invalid_request`. Success body includes `access_token`,
  `issued_token_type` = access-token type, `token_type` = `Bearer`, and non-empty granted `scope`.
- **Rationale**: This preserves normal local-agent scope semantics while keeping target routing,
  `resource` rejection, and policy-owned `aud` unchanged.

---

## Open questions

None. All Technical Context unknowns resolved above.
