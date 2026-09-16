# Implementation Plan: Public Client Support for Third-Party OAuth2 Services

**Branch**: `042-thirdparty-public-pkce` | **Date**: 2026-09-10 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/042-thirdparty-public-pkce/spec.md`

**Phase artifacts**: [research.md](./research.md) · [data-model.md](./data-model.md) ·
[contracts/admin-api.md](./contracts/admin-api.md) ·
[contracts/upstream-token-requests.md](./contracts/upstream-token-requests.md) ·
[quickstart.md](./quickstart.md)

## Summary

A third-party OAuth2 service cannot exist without a client secret today: registration rejects an
empty secret, the `client_secret_encrypted` column is `NOT NULL`, and both upstream token requests
transmit a credential. Providers that issue public clients only cannot be onboarded.

This feature adds one optional attribute, `token_endpoint_auth_method`, whose only accepted value is
`none`. Declaring it makes the service a public client: no credential is stored and none is sent
upstream by any channel. Omitting it — or sending an explicit `null`, which means the same thing —
keeps the service confidential with upstream requests byte-identical to today's. PKCE with `S256` —
already sent on every third-party flow — becomes a hard invariant.

The technical approach, established in [research.md](./research.md):

1. **Domain** — `Secret` gains an explicitly constructed absent state; a new
   `TokenEndpointAuthMethod` value object carries `none` or absence; the entity gains one predicate,
   `IsPublicClient()`, that every consumer branches on.
2. **Upstream** — the public code exchange reuses `golang.org/x/oauth2` with an empty secret and
   `Endpoint.AuthStyle` pinned to `AuthStyleInParams`. Verified against the module source: that
   style omits `client_secret` when empty, never calls `SetBasicAuth`, and disables the
   auto-detect probe that would otherwise send an `Authorization` header with an empty password.
   The confidential path keeps `AuthStyleAutoDetect` and is not touched. The hand-rolled refresh
   omits one form value.
3. **Persistence** — migration `031` makes the ciphertext column nullable, adds a nullable method
   column with no default and no backfill, and enforces the XOR invariant plus the closed value set
   in a single `CHECK`. The down migration refuses to run while any public service exists.
4. **API** — `client_secret` becomes conditionally required on create and update, where `null` and
   omission are equivalent spellings of "confidential" so a read response round-trips as input; the read
   representation omits it for public services and reports the method for every service.

## Technical Context

**Language/Version**: Go 1.26.8
**Primary Dependencies**: `golang.org/x/oauth2` v0.36.0 (upstream token requests), `chi` v5 (ADR
003), `sqlx` + `pgx` v5 (ADR 004), `golang-migrate/migrate` v4.19.1, AWS Encryption SDK via the
existing branch-key manager (ADRs 008/009)
**Storage**: PostgreSQL (production) and in-memory (dev/test); table `thirdparty_oauth2_services`
**Testing**: Ginkgo/Gomega v2 for E2E (ADR 007), Go `testing` + table-driven tests for unit and
integration, testcontainers-backed PostgreSQL for the infra integration layer
**Target Platform**: Linux server (container), dual HTTP servers on `:8000` (end-user) and `:14000`
(admin) per ADR 004
**Project Type**: Web application — Go backend with hexagonal layering; **no frontend change** (no
administrative UI exists for third-party service registration)
**Performance Goals**: No change. This feature removes work from the public path (one fewer
encryption call per create/update, one fewer auto-detect probe per exchange) and adds none to the
confidential path.
**Constraints**: Confidential upstream requests must remain byte-identical (FR-017, SC-004); no
credential material may reach any log or error message (FR-027, SR-008); public token requests must
carry zero credential material in body, header, or query (SR-003)
**Scale/Scope**: ~14 production files, 1 migration pair, 2 OpenAPI schema groups, 1 new ADR, 22
acceptance scenarios. No new entity, no new port, no new configuration parameter.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Verified against [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.9.1.

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities, aggregates, and value objects identified and documented in
      [data-model.md](./data-model.md). One extended entity, one extended value object
      (`Secret` gains an absent state), one new value object (`TokenEndpointAuthMethod`).
- [x] **Domain Concepts**: `TokenEndpointAuthMethod`, *public client*, and *confidential client* will
      be added to the ARCHITECTURE.md Glossary; the existing **Secret** entry is amended to name the
      third state ([data-model.md §8](./data-model.md#8-glossary-additions-architecturemd)).
- [x] **Entity IDs**: N/A — no new entity. `id.ServiceID` already exists
      (`internal/domain/id/uuid_ids_gen.go:66-74`); no `gen_ids.go` change (ADR 013).
- [x] **Configuration Design**: N/A — third-party services are registered through the administrative
      API, not startup configuration. No new setting is read through `internal/ports/config.go`.
- [x] **Config Examples**: N/A — no configuration parameter added, so `examples/config/` is unchanged.
- [x] **Helm Chart**: N/A — no configuration parameter added, changed, or removed, so
      `charts/agentic-identity-broker/` is unchanged (Principle VII trigger not met).
- [x] **API Design First**: The complete delta is specified in
      [contracts/admin-api.md](./contracts/admin-api.md) before any implementation.
- [x] **API Documentation**: Changes land in `api/admin/openapi.yaml`; `api/enduser/openapi.yaml`
      receives description-only amendments (API-008).
- [x] **API Changes**: **CONFIRMED 2026-09-10.** API-010 satisfied — the stakeholder approved the
      two substantive decisions in the delta: `client_secret` omitted rather than `"REDACTED"` for
      public services, and explicit `null` accepted as equivalent to omission on write. The
      remaining items in
      [contracts/admin-api.md §8](./contracts/admin-api.md#8-confirmation-checklist-api-010) are
      mechanical consequences of those two plus the already-clarified full-replacement semantics.
      Decisions recorded in [spec.md Clarifications](./spec.md#clarifications) and
      [checklists/requirements.md finding 11](./checklists/requirements.md).
- [x] **Database Design**: Migration pair `031_add_token_endpoint_auth_method.{up,down}.sql` in
      `/migrations/`, go-migrate naming, documented in
      [data-model.md §6](./data-model.md#6-persistence-schema).
- [x] **E2E Acceptance Tests**: All 22 acceptance scenarios map 1:1 to `It()` blocks; see
      [Testing Strategy](#testing-strategy).
- [x] **E2E Test Mapping**: One `It()` per scenario, table below.
- [x] **E2E Red Phase**: Assertions target concrete HTTP status codes, response bodies, captured
      upstream request headers and form values, and database column state.
- [x] **Frontend Playwright E2E**: N/A — no React UI change. No administrative UI exists for
      third-party service registration, and the end-user flow is unchanged from the user's
      perspective.
- [x] **Frontend Screenshots**: N/A — same reason.

**Implementation Considerations**:

- [x] **Security-First**: Absence is constructed, never inherited, so a forgotten `Secret` still
      fails validation instead of becoming a public client. Contradictory configuration fails closed
      at registration, before any upstream request (SR-004). The pinned `AuthStyleInParams`
      structurally disables the credentialed probe and fallback (FR-015, SR-005).
- [x] **Architecture Docs**: ARCHITECTURE.md gains the glossary entries and a note on the two
      upstream client-authentication modes.
- [x] **ADRs**: `adrs/036-public-client-token-endpoint-auth.md` is **required**, not discretionary.
      ADR 036 supersedes the two-state `Secret` decision in
      `adrs/012-encryption-layer-separation.md:32-43` and records the amendment in ADR 012.
      Principle II requires this superseding decision. ADR 036 also records absence-as-confidential
      with no default, `none` as the only accepted value, the pinned auth style, the ADR 017
      representation precedent, and the deliberate divergence from ADR 017's update semantics
      ([research.md §D11](./research.md#d11--architectural-record)). `036` is the next free number.
- [x] **Library-First Security**: No new cryptographic primitive. PKCE generation and verification
      stay with `golang.org/x/oauth2`; encryption stays with the existing branch-key manager
      (Principle III, SR-009).
- [x] **Zalando Guidelines**: The new property is snake_case, optional, with a closed enum and no
      default; errors reuse the existing admin `ErrorResponse` shape.
- [x] **End-User Docs**: `docs/api/` gains no new guide (none exists for service CRUD); the read
      schema `required` relaxation is recorded in `docs/changelog.md`.
- [x] **Migration Testing**: Apply, constraint enforcement in both directions, rollback refusal with
      a public service present, and successful rollback without one (DB-007). The refusal leaves
      `schema_migrations.dirty = true`, so `tests/integration/migrations/framework.go` gains a
      `Force(t, version)` helper; without it the second rollback cannot run
      ([research.md §D6](./research.md#d6--schema-and-rollback)).
- [x] **Hexagonal Architecture**: The invariant lives in the domain entity; the handler maps JSON
      presence to domain state only; adapters stay opaque to credential semantics. No new port.
- [x] **Persistence Patterns**: Both adapters implement the change; ISP interfaces in
      `internal/ports/thirdparty_provider.go` keep their exact method set — only the contract
      documentation changes from "encrypted" to "encrypted or absent".

*All BLOCKING preconditions are satisfied. API-010 was cleared on 2026-09-10; the null-write
decision it produced amended API-003 in the specification, and every dependent artifact was migrated
to match. Implementation may begin.*

## Project Structure

### Documentation (this feature)

```text
specs/042-thirdparty-public-pkce/
├── plan.md                              # This file
├── spec.md                              # Feature specification
├── research.md                          # Phase 0 output — 11 decisions, all unknowns resolved
├── data-model.md                        # Phase 1 output — entity, value objects, schema, invariant
├── quickstart.md                        # Phase 1 output — runnable validation ladder
├── contracts/
│   ├── admin-api.md                     # Phase 1 output — exact OpenAPI delta + confirmation gate
│   └── upstream-token-requests.md       # Phase 1 output — outbound wire contract, both modes
├── checklists/
│   └── requirements.md                  # Specification quality checklist (complete)
└── tasks.md                             # Phase 2 output — created by /speckit-tasks
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── model/
│   │   ├── secret.go                              # + absent state, NewAbsentSecret, IsAbsent
│   │   ├── token_endpoint_auth_method.go          # NEW value object
│   │   └── thirdparty_oauth2_provider.go          # + field, IsPublicClient, validation rules 1-5
│   ├── thirdparty/service.go                      # skip encrypt/decrypt when public; keep branch key
│   └── oauth2session/service.go                   # buildOAuth2Config branch; refresh omits secret
├── ports/
│   └── thirdparty_provider.go                     # contract doc: "encrypted or absent"
├── adapters/
│   ├── http/handlers/admin/services_handler.go    # presence mapping, DTO fields, response shaping
│   └── storage/
│       ├── postgres/
│       │   ├── thirdparty_provider.go             # + column in SELECT/INSERT/UPDATE
│       │   └── thirdparty_provider_record.go      # nullable ciphertext, method field
│       └── memory/thirdparty_provider_record.go   # accept absent secret

migrations/
├── 031_add_token_endpoint_auth_method.up.sql      # NEW
└── 031_add_token_endpoint_auth_method.down.sql    # NEW — guarded rollback

api/
├── admin/openapi.yaml                             # 3 schemas + 4 path descriptions
└── enduser/openapi.yaml                           # descriptions only

adrs/036-public-client-token-endpoint-auth.md      # NEW
ARCHITECTURE.md                                    # glossary + upstream auth modes
docs/changelog.md                                  # read-schema required relaxation

tests/
├── e2e/
│   ├── thirdparty_public_client_test.go           # NEW — 22 It() blocks
│   ├── helpers/mock_upstream.go                   # + opt-in strict public-client mode
│   └── fixtures/services.go                       # + PublicClientService fixture
└── integration/
    ├── migrations/framework.go                    # + Force(t, version) — clears the dirty flag
    ├── migrations/migrations_test.go              # + TestMigration031 apply/constraint/rollback
    └── storage/infra/thirdparty_service_test.go   # + migration 031, absent-credential persistence
```

**Structure Decision**: Existing hexagonal Go backend, no new package or module. The change fans
across four layers — domain model, domain service, adapters (HTTP and both storage), and
migrations — but introduces no new architectural element. `internal/app/builder.go` and
`internal/adapters/http/routing/admin.go` are untouched: the handler and service already exist and
are already wired (Principle XII).

## Implementation Phase Overview

*Detailed task breakdown is in `tasks.md` (generated by `/speckit-tasks`).*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | **Skipped** |
| **Phase 1** | Setup | **Skipped** — no new dependency, module, or tool |
| **Phase 2** | Design Preconditions (domain model, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | **Skipped** |
| **Phase 2.5** | Foundational Infrastructure — `Secret` absent state, `TokenEndpointAuthMethod`, migration 031, upstream double strict mode | Included |
| **Phase 3-6** | User Stories 1-4 by priority | Included |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **skip** — no rename, restructure, or interface extraction is needed.
      Every change is additive to existing types, and no existing test requires modification. The
      one naming divergence found (`ThirdpartyOAuth2ProviderEntity` versus the specification's
      `ThirdpartyOAuth2Service`) predates this feature and renaming it would inflate the diff across
      dozens of unrelated files.
- [x] Phase 2.7 (entity boilerplate): **skip** — no new domain entity, no new port, no new storage
      adapter, and no new HTTP handler. Scaffolding would be empty.

**Phase 2.5 rationale**: The absent `Secret` state, the `TokenEndpointAuthMethod` value object,
migration `031`, and the upstream double's strict mode are prerequisites shared by all four user
stories. Landing them first keeps each story's diff to its own behaviour.

**Story sequencing**: Story 1 (registration) is a viable standalone increment. Story 2 (authorize
and exchange) depends only on Story 1. Story 3 (refresh) depends on Story 2. Story 4 (mode changes)
depends on Story 1 and can land in parallel with Story 3.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/thirdparty_public_client_test.go`

**Framework**: Ginkgo/Gomega BDD. `tests/e2e/AGENTS.md` is the binding guide for this directory
(ADR 007); `tests/e2e/README.md` is historical and is used only for broker HTTP patterns.

**Test Organization**:

- **Top-level Describe**: `"Public Client Support for Third-Party OAuth2 Services"`
- **Nested Describe/Context**: one `Context` per user story, then per precondition
  (e.g. `"when the provider rejects client credentials"`)
- **It blocks**: one per acceptance scenario, 22 total

**Spec traceability (mandatory, `tests/e2e/AGENTS.md` rule 2)**: the mapping table below is not
sufficient on its own. Every `It()` MUST carry a nearby comment naming the actual scenario
identifier, in the repository's established form:

```go
// US2-S3 from specs/042-thirdparty-public-pkce/spec.md
It("sends no authorization header, including none with an empty password", func() {
```

A shared `Describe`/`Context` comment may cover a block only when it clearly maps that whole block
to one scenario. An `It()` with no nearby reference is listed as an anti-pattern in
`tests/e2e/AGENTS.md:98` and must be flagged in review.

**Anti-patterns to avoid** (`tests/e2e/AGENTS.md:96-102`): more than 15 setup lines inside an
`It()` — use `BeforeEach`; repeated setup outside a `Context`; tests constructing services directly
instead of using `bootstrap/`; mutable state crossing `It()` blocks. Use `Eventually` or the
polling helpers, never `time.Sleep`.

**Scenario Mapping**:

Each row's identifier is the exact string that MUST appear in the `It()`'s traceability comment, as
`// <id> from specs/042-thirdparty-public-pkce/spec.md`. All 22 live in
`tests/e2e/thirdparty_public_client_test.go`.

| Scenario ID | Spec scenario | Test description |
|---|---|---|
| `US1-S1` | register with `none`, no secret | `It("creates the service and reports method none with no credential", …)` |
| `US1-S2` | `none` with a secret | `It("rejects the contradiction and creates no service", …)` |
| `US1-S3` | method omitted | `It("creates a confidential service with unchanged credential requirements", …)` |
| `US1-S4` | confidential, empty secret | `It("rejects a confidential service with no client secret exactly as today", …)` |
| `US1-S5` | `none` on the google flavor | `It("rejects none for the google variant, naming it", …)` |
| `US1-S6` | mixed catalogue listing | `It("reports the method for every entry and omits the credential for public ones", …)` |
| `US2-S1` | authorization redirect | `It("sends code_challenge and code_challenge_method S256 upstream", …)` |
| `US2-S2` | exchange body | `It("sends client_id and code_verifier with no client_secret in the body", …)` |
| `US2-S3` | exchange headers | `It("sends no authorization header, including none with an empty password", …)` |
| `US2-S4` | session stored | `It("stores an encrypted session exactly as for a confidential service", …)` |
| `US2-S5` | upstream rejection | `It("fails closed without any retry adding a credential", …)` |
| `US2-S6` | confidential regression | `It("leaves the confidential connect flow unchanged", …)` |
| `US3-S1` | refresh body | `It("refreshes with client_id and refresh_token and no client_secret", …)` |
| `US3-S2` | refresh persistence | `It("encrypts and persists the refreshed tokens, replacing the previous ones", …)` |
| `US3-S3` | confidential refresh | `It("leaves the confidential refresh request unchanged", …)` |
| `US3-S4` | refresh rejection | `It("surfaces the failure without retrying with a credential or downgrading", …)` |
| `US4-S1` | confidential → public | `It("removes the stored credential when the service becomes public", …)` |
| `US4-S2` | omit method, no secret | `It("rejects the update and leaves the public service intact", …)` |
| `US4-S3` | omit method, with secret | `It("makes the service confidential and sends that credential upstream", …)` |
| `US4-S4` | `none`, rename only | `It("keeps the service public with no credential", …)` |
| `US4-S5` | sessions survive | `It("keeps stored sessions valid and uses the new setting on the next refresh", …)` |
| `US4-S6` | read-modify-write round trip | `It("accepts the null token_endpoint_auth_method a read returned and keeps the service confidential", …)` |

**Red Phase Requirements**:

- Tests must compile and fail semantically. Assertions target concrete values: HTTP status codes,
  `has("client_secret")` on the JSON body, `mockUpstream.GetLastBody()` form contents,
  `mockUpstream.GetLastRequest().Header.Get("Authorization")`, and stored session state.
- `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, and `Skip()` are forbidden.
- No red-phase comments.

**Test Data Strategy**:

- Existing fixtures: `fixtures.GitHubService`, `fixtures.GoogleService`, `fixtures.ServiceWithID`
  (`tests/e2e/fixtures/services.go`), `fixtures.DefaultOAuth2Config`
- New fixture: `fixtures.PublicClientService` — a service with `TokenEndpointAuthMethod: "none"` and
  `Secret: model.NewAbsentSecret()`, for tests that seed storage directly rather than going through
  the admin API.
- During Phase 2f, the fixture is compile-only. It has no domain-validation requirement.
  T039 and T042 establish the public-client invariant before an E2E scenario depends on validation.

**Test Execution Flow**:

1. Phase 2f: write all 22 `It()` blocks with detailed expectations
2. Verify red: `ginkgo -v --label-filter="!performance" --focus="Public Client" ./tests/e2e/` —
   all fail semantically. Full backend suite: `just test-e2e-backend`.
3. Implement Phase 2.5, then stories in priority order
4. Verify green as each story lands
5. Only fixture adjustments during implementation, never test logic

**Bootstrap Strategy**:

- `bootstrap.NewTestStorage` + `ServerFactory` + `NewAdminTestServer` / `NewEndUserTestServer`,
  fresh per test via `BeforeEach`, closed in `AfterEach` (existing pattern)
- Feature-specific: tests for Stories 2 and 3 construct their **own** `MockUpstreamOAuth2Server` in
  strict public-client mode instead of the suite-wide instance from `e2e_suite_test.go:21-38`.
  That shared double is treated as read-only across parallel backend workers
  (`tests/e2e/AGENTS.md:49`), so enabling strict mode on it would leak rejections into unrelated
  suites and break under `GINKGO_PROCS`. A per-test double also keeps the recorded PKCE challenge
  scoped to the test that produced it.
- Both server types are needed: `NewAdminTestServer` for Stories 1 and 4 (service CRUD) and
  `NewEndUserTestServer` for Stories 2 and 3 (authorize, callback, refresh), per the dual-server
  pattern in `tests/e2e/AGENTS.md:104-111`.

**Helper Utilities**:

- New matchers: none. `HaveStatusCode` and direct assertions on the captured form and headers are
  sufficient and keep the failure output readable.
- HTTP helpers: existing `createService` / `updateService` / `getService` / `listServices` patterns
  from `tests/e2e/oauth2_provider_flavor_test.go:49-79`
- Mock services: `MockUpstreamOAuth2Server` extended with opt-in strict mode — records the PKCE
  challenge at `/authorize`, and at `/token` returns `401 invalid_client` on any credential and
  `400 invalid_grant` on a verifier that does not match. Default behaviour is unchanged, so no
  existing test is affected. See
  [contracts/upstream-token-requests.md §5](./contracts/upstream-token-requests.md#5-test-double-conformance).

### Frontend Playwright E2E Tests

**Not applicable.** This feature changes no React UI. No administrative interface exists for
third-party service registration, and the end-user session pages are unaffected because the flow
they trigger is unchanged from the user's perspective. No screenshots are required.

### Unit & Integration Tests

**Unit Tests** (TDD, written first). Domain conventions from `internal/domain/AGENTS.md:85-90`
apply: `_test.go` beside the package in the same package (white-box); table-driven tests for
validation; **hand-rolled mocks — structs with function fields — never `testify/mock` in the
domain**; and security tests in `_security_test.go` files.

The SR-003 assertions belong in
`internal/domain/oauth2session/service_security_test.go`: that no public-client token or refresh
request carries credential material in the body, an `Authorization` header, or the query — the
"zero credential material by any channel" claim of SC-003, tested at the unit boundary rather than
only through E2E.

The FR-027 and SC-008 assertions also belong in
`internal/domain/oauth2session/service_security_test.go`. Simulate exchange and refresh errors that
contain sentinel credential, verifier, authorization-code, access-token, and refresh-token values.
Assert that `exchangeCodeWithRetry`, `HandleCallback`, and `refreshSessionTokens` log and return only
safe reasons.

| Location | Coverage |
|---|---|
| `internal/domain/model/secret_test.go` | Absent state: constructor, all three predicates, `GetPlaintext`/`GetCiphertext` errors, zero value still reports plaintext |
| `internal/domain/model/token_endpoint_auth_method_test.go` | Table-driven `Validate`: `""` accepted, `none` accepted, every other value rejected naming `none` |
| `internal/domain/model/thirdparty_oauth2_provider_test.go` | Table-driven create/update validation across the six rules in [data-model.md §3.1](./data-model.md#31-validation-rules), including google rejection ordering |
| `internal/domain/thirdparty/service_test.go` | Branch key still provisioned for public create and update; encryption skipped; `Get`/`List` return an absent secret with no error or warning |
| `internal/domain/oauth2session/service_test.go` | `buildOAuth2Config` pins `AuthStyleInParams` with an empty secret when public and leaves `AuthStyle` zero when confidential; refresh body omits `client_secret` when public |
| `internal/domain/oauth2session/service_security_test.go` | SR-003: no public-client exchange or refresh request carries credential material in the body, an `Authorization` header, or the query. FR-027 and SC-008: sentinel values in upstream errors do not reach logs, callback errors, or refresh errors. |
| `internal/adapters/http/handlers/admin/services_handler_test.go` | Mapping table from [research.md §D5](./research.md#d5--wire-format-presence-semantics) on both create and update: absent and `null` both yield a confidential entity, `"none"` yields an absent secret, any other value is rejected, and `none` with a non-empty secret is a contradiction |

**Integration Tests**:

| Location | Coverage |
|---|---|
| `tests/integration/migrations/migrations_test.go` | `031` applies on a `030` database; no backfill; `CHECK` rejects both contradictory directions. Rollback refusal is a six-step sequence: apply `031` → insert a public service → `Down(30)` returns an error naming the blocking service → `Version()` reports `dirty == true` and `token_endpoint_auth_method` still exists (the guard aborted the whole step, it did not tear down half the schema) → `Force(31)` clears the flag → delete the service and `Down(30)` now succeeds with `dirty == false` and the column gone |
| `tests/integration/storage/infra/thirdparty_service_test.go` | Round-trip a public service through the PostgreSQL repository: method persisted, ciphertext `NULL`, absent secret returned; confidential→public update nulls the column |
| `internal/adapters/storage/memory/thirdparty_provider_test.go` | Same round-trip against the in-memory adapter (DB-008) |

**Test Coverage Goals**:

- Unit: every branch of the public/confidential decision, both directions of each validation rule
- Integration: both adapters and the migration in both directions
- E2E: all 22 acceptance scenarios (mandatory, Principle XIII)
- Frontend E2E: not applicable

## Complexity Tracking

No constitution violations require justification, and no gate remains open.

Two simplifications are worth recording because they *avoid* complexity a reader might expect:

| Considered | Rejected because |
|---|---|
| A separate credential-free token-request implementation | `golang.org/x/oauth2` already emits exactly the required request under `AuthStyleInParams` with an empty secret, verified in the module source. A second implementation would duplicate response parsing and error handling for no behavioural gain. |
| A new structured field-error response shape for validation failures | The admin API has one `ErrorResponse` shape used by every handler. API-007 is satisfied by naming the field in `message`, as every current validation failure already does. A second shape for one feature would fragment the contract. |
