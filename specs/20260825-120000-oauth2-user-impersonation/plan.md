# Implementation Plan: OAuth2 User Impersonation

**Branch**: `20260825-120000-oauth2-user-impersonation` | **Date**: 2026-08-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/20260825-120000-oauth2-user-impersonation/spec.md`

## Summary

Add RFC 8693 user impersonation to `POST /oauth2/token`, active only in `local` mode when exactly
one request audience is `<oauth2_authorization_server.impersonation.audience_prefix>/<agent UUID
or canonical ID>`. The suffix resolves a registered target agent. The broker validates and authorizes a
privileged client assertion, actor, and subject through ordered rules, validates an optional requested
scope against the selected target agent's `allowed_scopes`, then mints through the normal local access-token
path with target-derived `agent_id`, target-backed local CEL `agent.*`, extracted `sub`, and validated
actor-token issuer/actor identity in `act.iss`/`act.sub`. The client assertion remains authorization/audit
identity only. Local
`token_claims_expression` owns emitted `aud`, including omission. Every decision is credential-free
audited.

Every rule-authorized request additionally verifies the impersonated subject's active `UserGrant` for the resolved target agent before minting. The subject identity is the lookup `id.Principal`; the check is mandatory for signed and unverified subjects and has no opt-out. Missing or expired delegations stop evaluation with generic `access_denied` plus the existing RFC 6749 §5.2 `error_uri` to `<end-user-public-url>/consent/agent/<target-agent-id>`; storage failures return `server_error`. The port boundary and error signal are governed by proposed ADR [032](../../adrs/20260825-120100-impersonation-requires-user-delegation.md).

The design reuses existing machinery: the JWKS adapter (`internal/adapters/jwks`), the ADR-20260116-120100 CEL
pattern (`internal/domain/tokenexchange/cel_evaluator.go`), the signed-JWT validation pattern
(`jwt_validator.go`), and the local issuer's signing key + `token_claims_expression` principal
context (`internal/domain/oauth2server`). A new bounded context `internal/domain/impersonation`
orchestrates the flow. See [research.md](research.md) for the grounded decisions and
[data-model.md](data-model.md) for entities.

**Governance**: The unsigned unverified subject path deviates from Constitution Principle I and
is governed by accepted ADR [031](../../adrs/20260825-120000-unverified-subject-unsigned-jwt.md), bounded to
the subject role with six compensating controls. The ADR and its paired Constitution Principle I
carve-out merge together with this feature under reviewing-maintainer approval.

## Technical Context

**Language/Version**: Go 1.25.6
**Primary Dependencies**: chi v5 (routing), `github.com/lestrrat-go/jwx/v3` (JWT/JWKS/sign),
`github.com/google/cel-go` v0.28.1 (authorization + extraction, ADR 20260116-120100), `github.com/lestrrat-go/httprc`
(JWKS refresh), Viper/mapstructure (config), fosite (local OAuth2 server), slog (audit)
**Storage**: No new storage. Each request looks up an existing `storage.Agent` through the existing
`ports.AgentRepository`; no migration or new identifier type is required.
**Testing**: Go `testing` + testify (unit/config), Ginkgo/Gomega (E2E), httptest JWKS servers
**Target Platform**: Linux server (Kubernetes)
**Project Type**: Web service (Go backend; no frontend/UI changes)
**Performance Goals**: SC-001 — ≥95% of valid requests <500ms at 100 concurrent (warmed)
**Constraints**: Fail closed (Principle I); no unsigned/`none` JWT for any signed role; local-mode
only; audit events carry no credential values; CEL authorization timeout 10ms–5s
**Scale/Scope**: Backend-only; ~1 new domain package, 1 config subtree, 1 new port + local-issuer
method, 1 handler activation branch, 1 E2E suite

## Constitution Check

*Constitution checks are revalidated after each design phase.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities/value objects identified in [data-model.md](data-model.md)
  (config value objects + in-flight value objects; new bounded context `internal/domain/impersonation`)
- [x] **Domain Concepts**: New terms (Impersonation Rule, Unverified Subject, Impersonated
  Broker Token, etc.) already in spec Glossary; will be added to `ARCHITECTURE.md` Glossary during implementation
- [x] **Entity IDs**: N/A — no new UUID-backed persistent entity; subject/actor/client are external
  strings mapping to existing `id.Principal` (ADR 20260313-120000 requires typed IDs only for UUID PKs)
- [x] **Configuration Design**: All config identified with YAML example
  ([contracts/impersonation-config.yaml](contracts/impersonation-config.yaml)); CR-001..CR-008
- [x] **Config Examples**: Will add `examples/config/impersonation.yaml` and update
  `examples/config/README.md`
- [x] **Helm Chart**: `charts/agentic-identity-broker/` (`values.yaml`, `values.schema.json`,
  `templates/configmap.yaml`, `README.md`) updated for the `impersonation` subtree
- [x] **API Design First**: OpenAPI contract fragment authored
  ([contracts/openapi-token-impersonation.yaml](contracts/openapi-token-impersonation.yaml));
  merged into `api/enduser/openapi.yaml` before implementation (FR-013)
- [x] **API Documentation**: `api/enduser/openapi.yaml` + rendered `docs/api/` updated
- [x] **API Changes**: Confirmed — the user's instruction to extend the existing token endpoint is
  the stakeholder confirmation (spec Assumptions); it is an additive extension of `/oauth2/token`
- [x] **Database Design**: N/A — no schema changes
- [x] **E2E Acceptance Tests**: `tests/e2e/impersonation_test.go` — 1:1 with spec scenarios, written
  first (red)
- [x] **E2E Test Mapping**: Each acceptance scenario → one `It()` (see Testing Strategy)
- [x] **E2E Red Phase**: Tests compile with realistic assertions and fail semantically before implementation
- [x] **Frontend Playwright E2E**: N/A — no React UI change
- [x] **Frontend Screenshots**: N/A — no UI change

**Implementation Considerations**:

- [x] **Security-First**: Enabled by design; all signed roles verify signatures; unsigned accepted
  ONLY on the bounded unverified subject role governed by ADR 20260825-120000 with startup-enforced
  compensating controls; fail closed on every error/timeout
- [x] **Architecture Docs**: `ARCHITECTURE.md` updated (new bounded context + impersonation flow)
- [x] **ADRs**: ADR 20260825-120000 and ADR 20260825-120100 are Accepted. ADR 20260825-120000 governs unsigned unverified subjects. ADR 20260825-120100 governs the mandatory user-delegation check, its port boundary, and the `error_uri` consent signal.
- [x] **Library-First Security**: jwx for verify/sign, cel-go for policy; no custom crypto
- [x] **Zalando Guidelines**: Additive OAuth2 params + RFC 8693-compliant response
- [x] **End-User Docs**: `docs/api/` + `docs/configuration.md` updated
- [x] **Migration Testing**: N/A — no migrations
- [x] **Hexagonal Architecture**: New port `ports.ImpersonationTokenIssuer`; domain
  `internal/domain/impersonation` depends on ports; handler → domain service → ports (no port bypass)
- [x] **Persistence Patterns**: N/A — stateless

**Result**: PASS. The single deviation (unsigned unverified subject) is justified and tracked via
accepted ADR 20260825-120000 and its paired Constitution Principle I carve-out; see Complexity Tracking.

## Project Structure

### Documentation (this feature)

```text
specs/20260825-120000-oauth2-user-impersonation/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/
│   ├── openapi-token-impersonation.yaml   # API contract fragment
│   └── impersonation-config.yaml          # Config contract + FR-014 example
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── impersonation/                 # NEW bounded context
│   │   ├── request.go                 # ImpersonationRequest parse/validate (FR-002a/003/003a/003b/003c)
│   │   ├── rule.go                     # Compiled rule: roles, issuers, verification, predicate
│   │   ├── validator.go               # Signed-credential validation + per-issuer algo allow-list (CR-007)
│   │   ├── unverified.go           # Unsigned alg:none subject parser (FR-003d, ADR 20260825-120000)
│   │   ├── cel.go                     # 5-var evaluator + startup subject_token binding check (FR-007/007a)
│   │   ├── extract.go                 # Identity + optional email extraction (FR-006/006a)
│   │   ├── service.go                 # Orchestration: activate → first-match → mint → audit
│   │   ├── errors.go                  # Error precedence mapping (FR-004a/FR-011)
│   │   └── *_test.go
│   └── oauth2server/
│       ├── provider.go                # + IssueImpersonationToken (implements new port)
│       └── strategies.go              # impersonation mint through normal local access-token path with protected act.iss/act.sub and granted scope (FR-006a/008)
├── ports/
│   ├── config.go                      # + ImpersonationConfig subtree under OAuth2AuthServerConfig
│   └── oauth2.go                      # + ImpersonationTokenIssuer, UserDelegationVerifier + statuses
├── config/
│   └── validator.go                   # + impersonation static validation (CR-001..CR-010)
├── app/
│   ├── impersonation_delegation.go     # Consent-service classifier implementing UserDelegationVerifier
│   └── builder.go                     # DI: rules, JWKS, CEL, delegation verifier, local issuer
└── adapters/http/enduser/
    └── oauth2_token.go                # activation branch before resource guard + audit event

api/enduser/openapi.yaml               # merge contract fragment
docs/api/, docs/configuration.md       # rendered docs
examples/config/impersonation.yaml     # working example (FR-014)
charts/agentic-identity-broker/        # values.yaml, values.schema.json, configmap.yaml, README.md (FR-015)
tests/e2e/impersonation_test.go        # E2E acceptance suite (Principle XIII)
```

**Structure Decision**: Web-service backend layout. Impersonation is a genuinely independent
bounded context (`internal/domain/impersonation`) — it orchestrates validation, authorization, and
local minting distinctly from third-party token retrieval (`internal/domain/tokenexchange`). It
reuses shared primitives (JWKS adapter, CEL pattern, JWT-validation pattern, local issuer) but does
not extend the third-party exchange service, which would violate single-responsibility. A new port
`ImpersonationTokenIssuer` keeps the domain insulated from the fosite/oauth2server concrete
(Principle VI, XII). No new adapters directory — reuses `internal/adapters/jwks` and
`internal/adapters/http/enduser`.

## Implementation Phase Overview

| Phase | Purpose | Included? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | **Skip** — no structural refactor needed; new package added alongside existing code |
| **Phase 1** | Setup — deps | **Skip** — all dependencies already present |
| **Phase 2** | Design Preconditions (domain model, config, API, E2E red tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | **Skip** — no new CRUD entity/repository |
| **Phase 2.5** | Foundational Infrastructure — config schema + validation, port, CEL ref-enumeration, algo allow-list, local-issuer mint method | Include |
| **Phase 3+** | User Stories P1→P2 (US1 mint, US2 rule config, US3 rejection, US4 audit) | Include |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **skip** — feature is additive; no behavior-preserving refactor precedes it
- [x] Phase 2.7 (entity boilerplate): **skip** — no persisted entity or CRUD handler

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/impersonation_test.go`
**Framework**: Ginkgo/Gomega ([tests/e2e/README.md](../../tests/e2e/README.md)), dual-server bootstrap.

**Test Organization**:
- Top-level `Describe`: "OAuth2 User Impersonation"
- Nested `Context` per user story / precondition (local vs proxy/hybrid; signed vs unverified)
- One `It()` per spec acceptance scenario + edge case

**Scenario Mapping** (populate exact line numbers during Phase 2 test authoring):

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1 S1 | `impersonation_test.go` | `It("mints a token with sub=subject and act.iss/act.sub identifying the actor", ...)` with an active subject delegation |
| US1 S2 | `impersonation_test.go` | `It("rejects absent/malformed/unsupported token types with invalid_request", ...)` |
| US1 S3 | `impersonation_test.go` | `It("issues local-policy base claims, policy aud, and granted scope without credential leakage", ...)` |
| US1 S4 | `impersonation_test.go` | `It("does not activate for a different/absent audience", ...)` |
| US1 S5 | `impersonation_test.go` | `It("rejects impersonation in proxy/hybrid mode without forwarding", ...)` |
| US1 S6 | `impersonation_test.go` | `It("mints from an unsigned unverified subject with email", ...)` with an active subject delegation |
| US2 S1–S5 | `impersonation_test.go` | rule validation, extraction, role/issuer mismatch, startup failure, first-match fall-through |
| US3 S1–S5 | `impersonation_test.go` | unsigned signed-role rejection, credential-failure rejection, access_denied, extraction failure, unverified guards |
| US4 S1–S3 | `impersonation_test.go` | success audit, failure audit, no-credential audit |
| US5 S1–S5 | `impersonation_test.go` | active delegation mint; missing/expired delegation returns 403 `access_denied` plus consent `error_uri`; unverified subject remains gated; lookup failure is 500 |
| Edge cases | `impersonation_test.go` | target-allowed, target-rejected, unrestricted-empty-list, and absent scope; resource present, requested_token_type mismatch, actor==subject, JWKS unavailable; terminal delegation decision |

**Red Phase Requirements**: tests compile with realistic assertions (status codes, decoded JWT
`sub`/`aud`/`act.iss`/`act.sub`/`scope`, audit JSON fields, and response scope omission when none is granted) and
fail semantically; no `XIt`/`Skip`; no red-phase comments.

**Test Data Strategy**: httptest JWKS servers (pattern from `internal/adapters/jwks/adapter_test.go`)
signing ES256/RS256 JWTs; new fixtures for impersonation config + issuer key sets. Reuse
`tests/e2e/matchers/oauth2_matchers.go` and add an impersonation success matcher asserting `act.iss`/`act.sub`.

**Bootstrap Strategy**: production bootstrap via `tests/e2e/bootstrap/`; fresh server + config per
test; separate contexts for local vs proxy/hybrid mode to prove FR-001/US1-S5.

### Unit & Integration Tests

**Unit** (`internal/domain/impersonation/*_test.go`): request validation (types/singleton/scope/
resource/requested_token_type), target scope allow-list validation, per-issuer algorithm allow-list
(approved accept, `none`/`HS*` reject), signed-role `alg:none` rejection, unverified unsigned parse
(alg==none + empty sig required), CEL 5-var authorization + startup `subject_token` binding enumeration,
identity/email extraction, first-match precedence + error taxonomy, and mandatory user-delegation
verification. Service tests prove active delegation mints; missing and expired delegation return the
same generic `access_denied` plus `error_uri`; a verifier failure is `server_error`; predicate denial
does not query delegation; delegation rejection does not fall through. App tests cover the consent
sentinel-to-port-status classifier.

**Config** (`internal/config/validator_test.go`, `internal/ports/config_test.go`): table-driven
CR-001..CR-010 with indexed `ConfigError` field-path assertions; proxy/hybrid rejection; local
issuer excluded from client-assertion role; unverified predicate binding; delegation dependencies
available when impersonation is configured.

**Test Coverage Goals**: critical paths (validation, authorization, minting, audit) fully covered;
E2E 100% of acceptance scenarios (Principle XIII).

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Unsigned (`alg:none`) JWT accepted on the unverified subject role (deviates from Principle I) | Privileged clients bridging non-OIDC subjects (chat users) hold no signed subject token but must convey multi-attribute subjects (principal id + email) | Plaintext identifier can't carry multiple attributes (clarification); requiring a signed subject is impossible (no issuer). Bounded to the subject role, protected by a verified client assertion and subject-binding predicate, and governed by accepted ADR 20260825-120000 plus its paired Constitution Principle I carve-out. |
| `ports.UserDelegationVerifier` + app-boundary consent classifier | Impersonation requires active user delegation without importing the consent bounded context | Calling `consent.Service` from impersonation leaks a sibling domain's concrete API and sentinels; reading `UserGrantRepository` directly duplicates consent's active-grant logic. |
