# Implementation Plan: Invalidate Sessions After Third-Party Credential Changes

**Branch**: `speckit/issue-388` | **Date**: 2026-11-03 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/055-invalidate-credential-sessions/spec.md`

## Summary

When an operator changes a third-party service's client ID or shared secret, the broker must stop supplying or refreshing tokens from sessions established under the old credentials. It must then guide users back through login. Research ([research.md](research.md)) selects the following approach:

1. **Per-service credential generation** in a new internal table, `thirdparty_credential_states` (R2). Every covered change increments the generation in the same transaction that makes the change effective. That transaction also **invalidates** every session of the service by expiring the retained row: the access token expires now, the refresh token is discarded, and the private callback generation is rotated (R1).
2. **Stored changes** are detected in the administrative `PUT` by exact comparison with the persisted, decrypted values (R3).
3. **Filesystem changes** are detected on demand. Before third-party initiation, code exchange, every token-supply or refresh decision, and broker `/oauth2/authorize` admission, the broker reads the pair. It checks the pair against a process-local HMAC cache and then against a durable salted Argon2id fingerprint. It commits a recognition before the operation proceeds (R3, R4).
4. **In-flight fencing**. The authorization state seals the generation, first-connection inserts compare it under the existing service-row share lock, reconnections are fenced by the rotated callback generation, and refreshes are serialized by the existing session row lock (R5).
5. **Recovery reuses existing flows**. Broker `/oauth2/authorize` already sends a user with an active grant to consent when an included or mandatory connection is unusable. RFC 8693 keeps ADR 044's recoverable `invalid_grant` and `error_uri`. The sessions page keeps its "Log in to reconnect" action. No React change and no new endpoint or response shape are planned (R6).

Because C1 and C3 contradict Accepted ADR 041, a narrow superseding [ADR draft](adr-draft-credential-change-session-invalidation.md) is proposed. Admin API `2.2.0` and End-User API `1.2.0` document the behavior changes ([contracts/](contracts/)). Every consequential item is listed under [Approval Items](#approval-items). None is approved.

## Approval Items

None of these items is approved. Each one is Proposed and waits for the maintainer's 👍 on the pipeline's approval comment. Implementation records the approval. Details point to the authoritative text. Spec proposals C1, C2, and C3 correspond to D-SCOPE, D-CONSENT, and D-CURRENCY.

| ID | Type | Decision | Details |
|---|---|---|---|
| API-ADMIN | API | Admin API `2.1.0` → `2.2.0`. `PUT /api/services/{service-id}` documents that a change of effective client ID or stored secret atomically invalidates every user session for the service. Unchanged values, metadata edits, rejected updates, and unchanged-pair source transitions do not invalidate. Restored values never revive sessions. File rotation has the same effect, without changing the ETag. | specs/055-invalidate-credential-sessions/contracts/admin-api.md:1-65 |
| API-ENDUSER | API | End-User API `1.1.0` → `1.2.0`, documenting the following existing response sets. `/oauth2/authorize` redirects to consent for an unusable included or mandatory connection despite an active grant, and returns `server_error` on filesystem source failure. Third-party initiation reads both credential files. A callback begun before a change returns `callback_failed`. Invalidated sessions list as `is_expired: true` and `has_refresh_token: false`, and refresh returns `409`. RFC 8693 returns the recoverable `invalid_grant` with the ADR 044 `error_uri`. Filesystem token use returns `500 server_error` when the files are unavailable, even for an unexpired token. | specs/055-invalidate-credential-sessions/contracts/enduser-api.md:1-116 |
| ADR-045 | ADR | Draft ADR 045 (number provisional): credential-change session invalidation. It narrowly supersedes five ADR 041 rules and amends Feature 051 scenarios US2-S1, US2-S5, and US5-S3. | specs/055-invalidate-credential-sessions/adr-draft-credential-change-session-invalidation.md:1-138 |
| D-SCOPE | Decision | C1: invalidate on any actual change of the operator-managed effective client ID or shared secret, including secret-only rotation, from the stored and filesystem sources and across transitions. CIMD-derived identifiers, CIMD key rotation, discovery and DCR lifecycles, and same-value saves are excluded. | specs/055-invalidate-credential-sessions/adr-draft-credential-change-session-invalidation.md:21-34 |
| D-CONSENT | Decision | C2: consent appears on the next agent browser authorization through the existing unusable-connection gate, in every broker mode, using the existing consent and sessions UI. Protocol recovery keeps ADR 044's direct provider login and `/sessions` return. | specs/055-invalidate-credential-sessions/research.md:129-138 |
| D-CURRENCY | Decision | C3: for filesystem services, the broker establishes currency on demand before initiation, exchange, every token-supply or refresh decision (including unexpired tokens), and `/oauth2/authorize` admission. This trades availability: unreadable files now fail token use. No polling is added, and a publish-and-restore between observations is not detected. | specs/055-invalidate-credential-sessions/adr-draft-credential-change-session-invalidation.md:49-73 |
| D-INVALIDATE | Decision | Invalidation retains and expires session rows: the access token expires at the change instant, the refresh-token ciphertext is discarded, and `callback_id` is rotated. Rows are not deleted. Grants are unchanged. | specs/055-invalidate-credential-sessions/research.md:7-32 |
| D-FINGERPRINT | Decision | Persist a salted Argon2id fingerprint of the last recognized filesystem pair, using the existing hasher parameters and at most 2 concurrent computations. A per-process random-key HMAC-SHA256 cache avoids repeated computations. Stored comparisons decrypt in memory, and decryption failure counts as a change. No plaintext or ciphertext of a last-known credential is persisted. | specs/055-invalidate-credential-sessions/research.md:53-74 |
| D-SCHEMA | Decision | Add the atomic migration `046_create_thirdparty_credential_states` (number provisional). It creates a lazily populated table keyed by service with a cascading foreign key, generation, fingerprint, and timestamps, under `NOWAIT` with a one-second timeout and no backfill. DOWN drops the table after a 10-minute drain of authorization flows. | specs/055-invalidate-credential-sessions/data-model.md:97-118 |

## Technical Context

**Language/Version**: Go 1.27.3 (backend). The React 19 SPA is unchanged.

**Primary Dependencies**:

- Existing modules: chi v5, sqlx + pgx v5, `golang.org/x/oauth2`, the `golang.org/x/crypto/argon2` hasher through `oauth2server.Argon2Hasher`, Ginkgo/Gomega, and Playwright.
- Standard library: `crypto/hmac`, `crypto/sha256`, `crypto/rand`, and `crypto/subtle`.
- **No new module.**

**Storage**:

- PostgreSQL and in-memory (both required by ADR 004).
- New table `thirdparty_credential_states`.
- `user_sessions` columns are rewritten on invalidation, and its schema is unchanged.
- `thirdparty_oauth2_services` is unchanged, and so is its ETag.

**Testing**:

- Unit tests: stdlib `testing` + testify.
- PostgreSQL integration tests: testcontainers, `-tags=integration`, container-backed.
- Backend E2E: Ginkgo, in-memory store. Two brokers share one storage instance to simulate restart and replicas.
- Frontend E2E: Playwright with screenshots.

**Target Platform**: Linux containers on Kubernetes, with credential files from projected volumes (ADR 041).

**Project Type**: Web service (Go) with an embedded SPA. The feature is backend and documentation only.

**Performance Goals**:

- Stored services: no added work on the token path.
- Filesystem services: each token decision adds one primary-key read, two file opens, and one HMAC computation. The first observation of a new pair costs one Argon2id computation (64 MiB) per process, with at most 2 at a time.
- An administrative update with a credential change runs one `UPDATE user_sessions ... WHERE service_id` through the existing `idx_user_sessions_service_id` index.

**Constraints**:

- Fail closed (SR-001, SR-005).
- No credential, digest, fingerprint, or path in logs, errors, spans, or metrics (SR-002, SR-003).
- No change to the administrative ETag from file rotation (ADR 041).
- Lock order: service row, then session rows. Refresh never locks the service row.

**Scale/Scope**:

- 1 migration, 1 port with 2 adapters, 2 small ports (fingerprinter, currency checker), and edits to 3 domain services.
- 2 new E2E files, 3 amended Feature 051 scenarios, and documentation.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: [data-model.md](data-model.md) defines CredentialState, CredentialChange, CredentialChangeResult, the generation state machine, and the UserSession transitions.
- [x] **Domain Concepts**: The `ARCHITECTURE.md` Glossary and the AGENTS.md glossary gain **Credential Change**, **Credential Generation**, **Credential Fingerprint**, **Credential Currency**, and **Session Invalidation** (Phase 2 task).
- [x] **Entity IDs**: No new UUID-keyed entity. CredentialState is keyed by the existing `id.ServiceID` (ADR 013 is not triggered).
- [x] **Configuration Design**: No new key (R10). The existing `third_party_oauth2.credential_files` YAML is shown in the spec.
- [x] **Config Examples**: No new options. `examples/config/third-party-oauth2.yaml` comments and `examples/config/README.md` gain the rotation behavior note.
- [x] **Helm Deployment Contract**: No chart-managed parameter changes. The chart README's rollback section gains the migration `046` drain note.
- [x] **API Design First**: The deltas are in [contracts/admin-api.md](contracts/admin-api.md) and [contracts/enduser-api.md](contracts/enduser-api.md). They are copied into `api/` only after approval.
- [x] **API Documentation**: `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`, and `docs/api/enduser-api.md`.
- [ ] **API Changes**: Pending the maintainer's 👍 on API-ADMIN and API-ENDUSER. This is the expected open state for planning.
- [x] **Database Design**: Migration `046` (provisional) with go-migrate naming ([data-model.md](data-model.md#migration-number-provisional-next-free-currently-046)).
- [x] **E2E Acceptance Tests**: All 11 scenarios are mapped below and written before implementation.
- [x] **E2E Test Mapping**: Each scenario maps to one `It`. Scenario 1.1's credential-change matrix uses one `DescribeTable` entry per variant, and every entry maps to Scenario 1.1.
- [x] **E2E Red Phase**: The tests assert concrete status codes, `error_uri`, provider request counts, session fields, and UI elements.
- [x] **Frontend Playwright E2E**: The UI is not changed, but Story 2 browser journeys get Playwright tests in `tests/e2e/frontend/` as the spec requires.
- [x] **Frontend Screenshots**: Three states are captured (see [quickstart.md](quickstart.md#4-browser-journeys-user-story-2)).

**Implementation Considerations**:

- [x] **Security-First**: Invalidation is unconditional and has no opt-out. Every failure path fails closed. No fallback credentials are used.
- [x] **Architecture Docs**: `ARCHITECTURE.md` gets updates to storage (key tables), the credential-source section, API versioning, and the glossary.
- [x] **ADRs**: The superseding draft is in this directory. It moves to `adrs/` after approval.
- [x] **Library-First Security**: Argon2id through the existing hasher. HMAC, SHA-256, random, and constant-time compare from Go `crypto/*`.
- [x] **Zalando Guidelines**: No new resources or fields. Minor version bumps for documented compatible behavior.
- [x] **End-User Docs**: `docs/api/enduser-api.md` and `docs/reference/token-exchange.md`.
- [x] **Migration Testing**: Apply, rollback, and reapply, plus CHECK and cascade tests, in PostgreSQL integration tests (container-backed in CI).
- [x] **Hexagonal Architecture**: The domain depends on new ports. The adapters implement them. The builder wires them. No handler reaches a port directly.
- [x] **Persistence Patterns**: The ISP repository is in `internal/ports`, with memory and PostgreSQL adapters, `StorageError` wrapping, and configured timeouts.

**Post-design re-check (Phase 1)**: All preconditions hold. The one open item, API confirmation, is tracked in [Approval Items](#approval-items). No violation needs justification. The new availability coupling (D-CURRENCY) and the persisted fingerprint (D-FINGERPRINT) are trade-offs proposed for approval, not constitutional exceptions.

## Spec Alignment

These planning-phase reconciliations are recorded in the spec's Clarifications section (planning reconciliation session):

- Invalidated sessions are retained and expired (D-INVALIDATE).
- C2 is met by the existing consent gate. The State A/B/C wireframes now depict the existing consent and sessions UI (**Login** and **Save**; expired card with **Login**). The wireframes were re-rendered with `just render-wireframes`.
- The spec's persistence assumption is refined to allow only a non-recoverable fingerprint (D-FINGERPRINT).
- The API and database requirement sections reference the proposed contracts and migration.

No acceptance scenario or success criterion changed.

## Project Structure

### Documentation (this feature)

```text
specs/055-invalidate-credential-sessions/
├── spec.md
├── plan.md                                             # This file
├── research.md                                         # Phase 0
├── data-model.md                                       # Phase 1
├── quickstart.md                                       # Phase 1
├── adr-draft-credential-change-session-invalidation.md # Proposed superseding ADR (moves to adrs/ on approval)
├── contracts/
│   ├── admin-api.md                                    # API-ADMIN
│   ├── enduser-api.md                                  # API-ENDUSER
│   └── ports.md                                        # Internal ports, error mapping, audit events
├── checklists/requirements.md
├── wireframes/journey-2-{a-reconnect,b-connected,c-cancelled}.svg
└── tasks.md                                            # Phase 2 (/speckit-tasks)
```

### Source Code (repository root)

```text
migrations/
├── 046_create_thirdparty_credential_states.up.sql      # number provisional
└── 046_create_thirdparty_credential_states.down.sql

internal/
├── ports/
│   ├── thirdparty_provider.go        # + ThirdpartyCredentialStateRepository; Update contract note
│   ├── credential_file.go            # + CredentialFingerprinter
│   └── oauth2.go                     # + CredentialCurrencyChecker
├── domain/
│   ├── storage/credential_state.go   # CredentialState, CredentialChange, CredentialChangeResult (+ tests)
│   ├── storage/user_session.go       # + ExpectedCredentialGeneration (db:"-")
│   ├── model/thirdparty_oauth2_provider.go  # + transient CredentialChange
│   ├── thirdparty/service.go         # change detection in Update; audit
│   ├── oauth2session/
│   │   ├── credential_currency.go    # establishCurrency, digest cache, EnsureCredentialCurrency
│   │   ├── credentials.go            # selectCredentials uses the validated pair
│   │   ├── state_token.go            # + CredentialGeneration claim
│   │   └── service.go                # initiation, callback fence, GetValidAccessToken, refreshLockedSession
│   └── oauth2/service.go             # currency check before steps 4–5 of HandleAuthorization
├── adapters/
│   ├── credentialfile/fingerprint.go # Argon2id fingerprinter over oauth2server.Argon2Hasher
│   ├── storage/postgres/thirdparty_credential_state.go  # new repository
│   ├── storage/postgres/thirdparty_provider.go          # Update applies CredentialChange in-tx
│   ├── storage/postgres/user_session.go                 # generation fence in create()
│   ├── storage/memory/thirdparty_credential_state.go    # new repository
│   ├── storage/memory/thirdparty_provider.go            # Update applies CredentialChange under the gate and session locks
│   └── storage/memory/user_session.go                   # generation fence in create()
└── app/builder.go                    # wiring

tests/
├── e2e/credential_change_invalidation_test.go           # US1 1.1–1.7, US2 2.4
├── e2e/frontend/credential_change_reconnect_test.go     # US2 2.1–2.3 + screenshots
├── e2e/oauth2_secret_file_rotation_test.go              # amend US2-S1, US2-S5
└── e2e/oauth2_secret_file_boundaries_test.go            # amend US5-S3

api/admin/openapi.yaml, api/enduser/openapi.yaml, docs/api/enduser-api.md,
docs/reference/token-exchange.md, docs/configuration.md, docs/operations/deployment-checklist.md,
examples/config/third-party-oauth2.yaml, examples/config/README.md,
charts/agentic-identity-broker/README.md (rollback note), ARCHITECTURE.md, AGENTS.md,
internal/ports/AGENTS.md, internal/adapters/AGENTS.md
```

**Structure Decision**: Use the existing hexagonal layout. The feature extends the existing `oauth2session`, `thirdparty`, and `oauth2` domain services and their ports. It adds no new bounded context.

## Implementation Phase Overview

*`tasks.md` (generated by `/speckit-tasks`) has the detailed task breakdown.*

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 1** | Record the maintainer approval in the ADR draft, move it into `adrs/` with the next free number, and renumber the migration if needed | **First task** |
| **Phase 2** | Design preconditions: copy the approved OpenAPI deltas, glossary, migration files, red E2E tests (backend and frontend), amended Feature 051 scenarios | **MANDATORY** |
| **Phase 2.7** | Port and stub adapters for `ThirdpartyCredentialStateRepository`, `CredentialFingerprinter`, and `CredentialCurrencyChecker` so the red tests compile | Included (new persisted record) |
| **Phase 3 (US1)** | Stored change detection and atomic invalidation, filesystem currency, fencing, both stores | P1 |
| **Phase 4 (US2)** | `/oauth2/authorize` currency admission, callback-fence redirect, browser journeys green | P1 |
| **Phase N** | Documentation, `just check`, `just test`, matching E2E suites, `just verify` (non-container recipes), constitution compliance | **MANDATORY** |

- [x] Phase 0 (refactoring): skipped. The `selectCredentials` signature change is feature work. No structural refactor is needed beforehand.
- [x] Phase 2.7 (entity boilerplate): included. It keeps the new repository's scaffolding reviewable apart from the invalidation logic.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/credential_change_invalidation_test.go`, labeled `credential-change-invalidation`

**Framework**: Ginkgo/Gomega ([tests/e2e/README.md](../../tests/e2e/README.md)). It uses the production bootstrap through `tests/e2e/bootstrap/` and the mock OAuth2 provider. Filesystem variants use `helpers.PublishCredentialPair`.

**Test Organization**: `Describe("Credential-change session invalidation")` → `Context` per precondition (stored or filesystem service, two users and two services) → one `It` per scenario. Every `It` carries a `// Scenario X.Y from specs/055-invalidate-credential-sessions/spec.md` comment.

#### Scenario mapping

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| US1 1.1 | `tests/e2e/credential_change_invalidation_test.go` | `DescribeTable("1.1 invalidates every user's session for the changed service only", ...)`. One `Entry` each for stored client ID, stored secret-only, filesystem client ID, and filesystem secret-only. Asserts: the RFC 8693 response for Alice is `400 invalid_grant` with the ADR 044 `error_uri`, although her access token has not expired. Bob's refresh does not reach the provider. Alice's Calendar token is still supplied. |
| US1 1.2 | same | `It("1.2 keeps sessions usable after an unchanged-credential or display-name update")`: tokens still supplied, no `credentials_changed` event |
| US1 1.3 | same | `It("1.3 keeps sessions usable when a credential update is rejected")`: `409` stale version, session unchanged |
| US1 1.4 | same | `It("1.4 refuses an authorization callback and a refresh begun before the change")`: paused callback leads to `callback_failed` with no provider token request; a gated in-flight refresh commits and is then invalidated |
| US1 1.5 | same | `It("1.5 keeps invalidated sessions unusable after restart, on another replica, and after restoring former values")`: second broker sharing the storage |
| US1 1.6 | same | `It("1.6 recognizes a filesystem pair change on the next unexpired-token request and on another replica")`: zero provider calls, Bob rejected via the second broker, service ETag unchanged |
| US1 1.7 | same | `It("1.7 fails closed without inferring a change when the filesystem pair is unavailable")`: `500 server_error` without `error_uri`, no provider call; after restoring unchanged files the session still works |
| US2 2.4 | same | `It("2.4 returns the recoverable invalid_grant whose error_uri starts direct provider login")`: follows `error_uri`, asserts the `302` to the provider with the current client ID, completes the callback, lands on `/sessions` |

**Red Phase Requirements**: The tests compile against the Phase 2.7 stubs and fail on real assertions, for example `Expect(resp.StatusCode).To(Equal(400))` or `Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(n))`. They never use `XIt`, `PIt`, `Skip()`, or red-phase comments.

**Test Data Strategy**: Use the existing fixtures: principals (`fixtures.AnotherPrincipal`), credential fixtures (`CredentialClientID`, `AlternateCredentialClientID`, `RotatedCredentialSecret`), the agent with permission sets spanning two services, and the projected-volume binding helpers. New fixture constants are limited to the synthetic `report-client-v1` and `report-client-v2` IDs and their secrets.

**Bootstrap Strategy**: A fresh broker and storage per test. Scenarios 1.5 and 1.6 start a second broker on the same storage adapter, following `proactive_token_refresh_test.go` (two replicas). The second broker has its own process-local digest cache.

**Helper Utilities**: Reuse `helpers.PublishCredentialPair` and `helpers.CredentialTokenRequest`, and the mock provider's request counters and authorization gates. Add a small `tokenExchange(principal, resource)` helper in the test file if one does not already exist.

**Amended Feature 051 scenarios** (ADR-045): US2-S1, US2-S5, and US5-S3, as listed in [research R9](research.md#r9-feature-051-acceptance-scenarios-affected). Each amended `It` keeps its 051 scenario ID and adds a `// Amended by specs/055-invalidate-credential-sessions (ADR 045)` comment.

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/credential_change_reconnect_test.go`

**Framework**: Playwright through the existing harness and page objects in `tests/e2e/pages/` (consent page, sessions page).

**Screenshot Location**: `tests/e2e/screenshots/`

| UI Scenario | Playwright Test Location | Screenshot Filename |
|-------------|--------------------------|---------------------|
| US2 2.1: an active grant with an invalidated required connection shows consent automatically, listing GitHub Production under "Connect Your Accounts" with **Login**; **Save** disabled | `tests/e2e/frontend/credential_change_reconnect_test.go` | `credential_change_consent_reconnect_required.png` |
| US2 2.2: login with the current credentials restores the connection; **Save** resumes the original authorization with the grant unchanged | same | `credential_change_consent_reconnected.png` |
| US2 2.3: cancelling at the provider returns to `/sessions` with the access-denied alert and the expired card's **Login**; no new delegation | same | `credential_change_sessions_cancelled.png` |

### Unit & Integration Tests

**Unit Tests** (TDD, written first):

- `internal/domain/storage/credential_state_test.go`: validation (table-driven).
- `internal/domain/thirdparty/credential_change_test.go`: change detection matrix covering unchanged values, metadata, client ID, secret, removal or addition of a secret, transitions, and decryption failure.
- `internal/domain/oauth2session/credential_currency_test.go`: cache hit, baseline, recognition, lost compare-and-set, source error with no write, persistence failure.
- `internal/domain/oauth2session/credential_fence_test.go`: stale sealed generation, stale reconnection, legacy zero claim.
- `internal/domain/oauth2/service_test.go`: currency error leads to `server_error`; recognition leads to a consent redirect.
- `internal/adapters/credentialfile/fingerprint_test.go`.
- Memory repository tests, including atomicity and lock ordering with a concurrent `WithLockedSession`.

**Integration Tests** (PostgreSQL, testcontainers, `-tags=integration`, container-backed; validated by CI):

- Migration `046`: apply, rollback, and reapply; CHECKs; cascade.
- `thirdparty_credential_state` repository: compare-and-set.
- `Update` with a `CredentialChange`: atomic or rolled back on conflict.
- The `create()` generation fence under concurrency.
- Invalidation waits for a locked refresh.

**Test Coverage Goals**:

- Unit: every branch of change detection, currency, and fencing.
- Integration: both new and changed PostgreSQL repository methods.
- E2E: 100% of the 11 acceptance scenarios.
- Frontend E2E: all 3 browser scenarios.

## Complexity Tracking

No constitutional violations. Nothing to justify.
