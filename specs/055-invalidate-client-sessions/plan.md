# Implementation Plan: Invalidate Sessions After Service Client-ID Changes

**Branch**: `speckit/issue-388` | **Date**: 2026-11-05 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/055-invalidate-client-sessions/spec.md`, plus maintainer direction: cover both client-ID change channels (filesystem credentials and the Admin API), treat client-secret rotation as an expected operation that never invalidates sessions, and support deployment without a stop-the-world maintenance window.

## Summary

When a third-party service's effective client ID changes, every session established under the previous identity must stop supplying tokens on its next use. Users reach consent automatically on the next agent authorization and reconnect under the new identity. Routine secret rotation must not invalidate anything.

Technical approach ([research.md](research.md), draft [ADR 047](adr-047-client-identity-revisions.md)):

- Each service persists its last **Effective Client Identity** and a monotonic **Client Identity Revision**. An accepted Admin API `PUT` that changes a stored `client_id` increments the revision atomically in the existing conditional `UPDATE`. A filesystem change is recorded by compare-and-set the first time any broker operation reads a different client-ID file value.
- New authorizations seal `{client ID, revision}` for every service variant. Sessions store both. A session is usable only when both match the service's current state. Legacy identity-less sessions keep ADR 041's exception only while the revision is still 0 (DEC-001). A rollback is a new revision, so it never revives a session.
- One domain rule is enforced at token acquisition, including still-valid tokens (narrowing ADR 038), and at refresh, sweep, callback, agent authorization, consent, and session listing.
- Recovery reuses existing flows: `invalid_grant` with ADR 044's `error_uri`, the consent redirect, and service sign-in. The End-User API extends its existing ADR 046 `requires_reauthentication` predicate with identity invalidation and retains the equal deprecated `is_expired` alias (proposed major 1.2.0 → 2.0.0 cutover). Observed-rejection handling remains binding. The Admin API takes a proposed 2.1.1 → 3.0.0 major cutover: sweep results classify identity-invalid candidates as failed rather than refreshed, including dry runs. A continuous rolling upgrade allows mixed-version traffic under fixed identities, then resumes identity mutations only after all old readers/writers and admitted work are drained; post-activation rollback is revision-aware only (DEC-005).

## Technical Context

**Language/Version**: Go 1.27.3 (backend); TypeScript, React 19, Vite 7, and Tailwind 4 (SPA)
**Primary Dependencies**: Existing only. chi v5, sqlx + pgx v5, `golang.org/x/oauth2`, jwx (JWE state), cel-go (unchanged), and the OpenTelemetry/slog plumbing. No new dependency.
**Storage**: PostgreSQL (production) and in-memory (development and tests). Migrations `047_add_client_identity_revision` and `048_validate_client_identity_revision` (DEC-002).
**Testing**: Go `testing` (domain and adapters), Ginkgo/Gomega E2E (`tests/e2e/`), Playwright via Ginkgo (`tests/e2e/frontend/`), Vitest (`web/`), and testcontainers PostgreSQL (`tests/integration/`).
**Target Platform**: Linux containers (Kubernetes/Helm). Mounted credential files per ADR 041.
**Project Type**: Web service with an embedded SPA (monorepo).
**Performance Goals**: Token exchange adds one indexed service read without secret decryption. For filesystem services it also adds one bounded `ReadClientID` call (≤ 64 KiB, same adapter as initiation). Invalidation itself is O(1) per change.
**Constraints**: Fail closed (Principle I, SR-001). No credential values, paths, or client IDs in logs, metrics, or public errors (SR-004). Administrative service-metadata operations never read files (ADR 041). Consent/session usability displays read the live client-ID file, never the secret file, with persisted fallback; Proposed ADR 047 partially supersedes ADR 041's no-file-read rule for those displays only. Observations atomically fence filesystem source, administrative version, canonical ID, and revision; lost preconditions require fresh service/binding/file reads. Continuous mixed-version serving during rollout with effective identity/source/canonical-ID/binding mutations frozen until every serving/writing process and admitted operation is revision-aware; no stop-the-world outage or enforcement opt-out (DEC-005). There is no internal scheduler or distributed lock (ADR 038).
**Scale/Scope**: All service variants: static, public, CIMD, DCR, Google, and filesystem. All principals of a changed service. Four browser states (spec wireframes A–D).

All Technical Context unknowns are resolved in [research.md](research.md).

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design. Result: PASS, subject to approval of the items below.*

**Design Preconditions (BLOCKING)**:

Checked entries below record design coverage, not implementation or executed test evidence. The unchecked tasks in [tasks.md](tasks.md) are the implementation checklist, including approval recording, acceptance-test red evidence, and validation.

- [x] **Domain Model**: Aggregates `ThirdpartyOAuth2Service` and `UserSession` are extended, with value objects Effective Client Identity and Client Identity Revision and the derived condition Reauthentication Required ([data-model.md](data-model.md)).
- [x] **Domain Concepts**: The two identity terms are planned for the `ARCHITECTURE.md` §12 Glossary and the AGENTS.md glossary. Extend existing **Requires Reauthentication** (ADR 046) and **Client-identity association** (ADR 041) entries rather than duplicating them.
- [x] **Entity IDs**: No new entity or UUID key. `id.ClientID` is reused. No `gen_ids.go` change.
- [x] **Configuration Design**: No new configuration. The existing `third_party_oauth2.credential_files` and `server.enduser.public_url` are used, as shown in the spec's Journey 1.
- [x] **Config Examples**: No new keys. `examples/config/third-party-oauth2.yaml` gains only a comment that points to the client-ID change procedure.
- [x] **Helm Deployment Contract**: No values change. The chart README's migration and rollback notes gain migrations 047/048, the guarded DOWN, and the continuous rolling upgrade, fixed-identity overlap, and verified old-work drain before identity mutations resume (DEC-005). No workflow or chart setting is added.
- [x] **API Design First**: The deltas are in [contracts/enduser-api.md](contracts/enduser-api.md) and [contracts/admin-api.md](contracts/admin-api.md). They are applied to `api/` only after approval (API-001, API-002).
- [x] **API Documentation**: `api/enduser/openapi.yaml` (proposed 2.0.0 from baseline 1.2.0) and `api/admin/openapi.yaml` (changed sweep classifications, proposed major 3.0.0 from baseline 2.1.1), plus `docs/api/` and `docs/reference/token-exchange.md`.
- [ ] **API Changes**: Pending maintainer sign-off on API-001 and API-002.
- [x] **Database Design**: Migrations `047`/`048` use go-migrate naming and the guarded online pattern of migrations 040/041 and 044/045.
- [x] **E2E Acceptance Tests**: All 16 named scenarios are mapped below and written before implementation.
- [x] **E2E Test Mapping**: Each AS maps to exactly one `It`.
- [x] **E2E Red Phase**: Realistic assertions on status, `error`, `error_uri`, `Location`, and `requires_reauthentication`, with no placeholders.
- [x] **Frontend Playwright E2E**: Consent and sessions-page journeys in `tests/e2e/frontend/`.
- [x] **Frontend Screenshots**: Four journey states plus the failure/retry state, named below.

**Implementation Considerations**:

- [x] **Security-First**: Enforcement is always on, with no opt-out. Unknown identity fails closed. A rollback cannot revive a session. No secrets are logged.
- [x] **Architecture Docs**: `ARCHITECTURE.md` updates the credential-source section (§ near "Transition evidence"), the session lifecycle, and the glossary.
- [x] **ADRs**: Draft [ADR 047](adr-047-client-identity-revisions.md) partially supersedes ADR 041 and ADR 038. It is Proposed (ADR-001).
- [x] **Library-First Security**: The existing JWE service and the ADR 041 credential-file reader are used. No crypto change.
- [x] **Zalando Guidelines**: No new property. The existing snake_case `requires_reauthentication` and its deprecated alias are reused. No closed enum is extended. Error codes are unchanged. Changed End-User behavior takes a major version (API-001); changed Admin sweep classifications take a major version too (API-002), even without a schema/status change.
- [x] **End-User Docs**: `docs/api/` and `docs/reference/token-exchange.md` are updated with the examples from the spec.
- [x] **Migration Testing**: Apply, rollback, reapply, refusal, and concurrent compare-and-set are tested in `tests/integration/migrations/`.
- [x] **Hexagonal Architecture**: Rules live in `oauth2session`. The other domains depend on the narrow port `ports.SessionUsabilityEvaluator`. Persistence goes through `ports.ThirdpartyOAuth2ClientIdentityRepository`. No handler logic is added beyond mapping.
- [x] **Persistence Patterns**: A focused ISP repository and minimal DTOs defined in `internal/ports/storage.go`, both adapters, explicit SQL columns, source/configuration-generation/revision observation fencing, and `StorageError` wrapping.

## Spec Alignment

| Requirement | Design element |
|---|---|
| FR-001, FR-002, SC-001 | Revision and identity check in `GetValidAccessToken` before any decrypt. Per-service evidence affects all principals. |
| FR-003, AS-1.1, AS-1.7 | Two recording channels (research R1). All variants are covered (R2). |
| FR-004, FR-005, AS-1.3–AS-1.5, SC-005 | Identity check under the session lock. Callback revision check. Existing callback-ID compare-and-set. A monotonic revision makes rollback a new revision. |
| FR-006 | Reasons `identity_mismatch`, `identity_superseded`, and `identity_missing` map to `credential_identity_changed` and `invalid_grant` with ADR 044's `error_uri`. Source failures give `server_error`. |
| FR-007, FR-014, AS-2.1 | `anyDelegatedSessionUnusable` gives reason `session_reauthentication_required`, which leads to the existing consent redirect. No push or navigation. |
| FR-008, AS-2.5 | `requires_reauthentication` and `connectionStatus: not_connected`. The existing Login and sign-in actions are reused. |
| FR-009, FR-010, AS-2.2–AS-2.4 | Existing service authorize, callback, and return-state flows. A new connection seals the current `{E,R}`. A failed callback changes nothing. |
| FR-011, AS-3.1, AS-3.3 | Evidence is per service. Grants are untouched. Only included services are checked. |
| FR-012, AS-3.2, AS-3.4, SC-004 | Increments happen only on a differing effective client ID. Secret, metadata, and same-identity source changes record nothing. |
| FR-013, AS-1.6 | Legacy rule `R == 0` (DEC-001). The column default is 0. No backfill. |
| FR-015, AS-3.1, SC-004 | ADR 046's existing `RequiresReauthentication` OR identity unusability; preserve rejected-session exclusion, refreshable expired-token behavior, `409 refresh_rejected`, and equality of the deprecated alias. |
| SR-001–SR-005 | R5 fail-closed. Existing state, PKCE, and return-URI protections. ADR 044 `PublicURL` link. Events in [contracts/observability.md](contracts/observability.md). No provider revocation. |

## Approval Items

These items need the maintainer's sign-off before implementation. Each is Proposed with its rationale and main alternative at the cited location.

| ID | Type | Decision | Details |
|---|---|---|---|
| API-001 | API | End-User API 1.2.0 → 2.0.0 major cutover on existing routes: add identity unusability to the existing `requires_reauthentication` predicate, preserving ADR 046's rejection and refreshability rule and the equal deprecated `is_expired` alias; `SessionStatus` inherits the summary fields. `connectionStatus` reports `not_connected` for such sessions. Documented behavior for `/oauth2/authorize` (consent redirect), `/oauth2/token` (`invalid_grant` despite an unexpired token, `server_error` when identity is unreadable), the third-party callback (stale or pre-upgrade state missing explicit identity/revision rejected before exchange), manual refresh (identity denial → `409 refresh_unavailable` instead of `500`, preserving `409 refresh_rejected`), and grant submission | specs/055-invalidate-client-sessions/contracts/enduser-api.md:1-80 |
| API-002 | API | Admin API 2.1.1 → 3.0.0 major cutover on existing routes (observable sweep result change, not description-only): an accepted `client_id` change invalidates all sessions of the service, secret rotation and same-identity changes do not, internal identity values are never bound as input or returned (request validation unchanged), and sweep `failed` includes superseded sessions, including in dry runs | specs/055-invalidate-client-sessions/contracts/admin-api.md:1-38 |
| ADR-001 | ADR | Draft ADR 047 "Client Identity Revisions Invalidate Third-Party Sessions", which partially supersedes ADR 041 (enforcement scope/timing, legacy exception, consent/session usability no-file-read boundary, and rollout/rollback procedure) and ADR 038 (immediate valid-token return now follows the identity check) | specs/055-invalidate-client-sessions/adr-047-client-identity-revisions.md:1-120 |
| DEC-001 | Decision | Identity-less legacy sessions stay usable only while the service's Client Identity Revision is 0. The first recorded change after rollout invalidates them for all principals. No identity is inferred | specs/055-invalidate-client-sessions/spec.md:439 |
| DEC-002 | Decision | Persisted schema change: `client_identity` and `client_identity_revision` on services, `upstream_client_revision` on sessions. Migrations 047 (add, `NOT VALID` checks, `NOWAIT`) and 048 (validate). DOWN 047 refuses while any revision > 0. Invalidated sessions are retained, not deleted | specs/055-invalidate-client-sessions/data-model.md:109-120 |
| DEC-003 | Decision | Security over availability: an unreadable client-ID file or unloadable identity state blocks the release of still-valid stored tokens (`500 server_error`). Display surfaces fall back to persisted evidence | specs/055-invalidate-client-sessions/research.md:75-81 |
| DEC-004 | Decision | Every differing filesystem observation whose source/configuration-generation/revision preconditions still hold, including a stale replica or reverted file, records a new revision. Lost preconditions require fresh reads. Reconnections during incomplete multi-replica publication can need repeating; operators publish coherently | specs/055-invalidate-client-sessions/research.md:83-89 |
| DEC-005 | Decision | Continuous rolling upgrade: allow old/new traffic with effective identities/sources/canonical IDs/bindings fixed; same-ID rotation continues, identity-mutating automation and new registrations wait. Apply additive migrations online; resume identity changes only after old processes/admitted work drain and old-image restart is prevented. New replicas enforce immediately and reject legacy callback state. A pre-activation prior-binary abort requires verified unchanged identities, zero increments, drained new work, and retained evidence; post-activation rollback is revision-aware only | specs/055-invalidate-client-sessions/adr-047-client-identity-revisions.md:81-93 |

## Project Structure

### Documentation (this feature)

```text
specs/055-invalidate-client-sessions/
├── spec.md
├── plan.md                                  # This file
├── research.md                              # Phase 0
├── data-model.md                            # Phase 1
├── quickstart.md                            # Phase 1
├── adr-047-client-identity-revisions.md     # Proposed ADR draft (moves to adrs/ after approval)
├── contracts/
│   ├── enduser-api.md                       # API-001 delta
│   ├── admin-api.md                         # API-002 delta
│   ├── ports.md                             # Internal ports and domain operations
│   └── observability.md                     # Log events
├── checklists/requirements.md
├── wireframes/                              # Spec States A–D
└── tasks.md                                 # Dependency-ordered implementation and validation checklist
```

### Source Code (repository root)

```text
migrations/
├── 047_add_client_identity_revision.{up,down}.sql
└── 048_validate_client_identity_revision.{up,down}.sql

internal/
├── ports/
│   ├── storage.go                    # ThirdpartyOAuth2ClientIdentityRepository, ClientIdentityState/Observation
│   └── session_usability.go          # SessionUsabilityEvaluator, UsabilityMode
├── domain/
│   ├── model/
│   │   ├── thirdparty_oauth2_provider.go   # ClientIdentity, ClientIdentityRevision (+Copy)
│   │   └── credential_source_error.go      # identity_superseded reason
│   ├── storage/user_session.go             # UpstreamClientRevision; extend existing summary reauthentication predicate
│   ├── oauth2session/
│   │   ├── client_identity.go              # resolveClientIdentity, sessionIdentityStatus, evaluator
│   │   ├── credentials.go                  # identity sealing and checks for all variants
│   │   ├── state_token.go                  # upstream_client_revision claim
│   │   ├── service.go                      # GetValidAccessToken, refreshLockedSession, HandleCallback, listing
│   │   └── sweep.go                        # superseded candidates counted as failed
│   ├── thirdparty/service.go               # identity state read without decryption
│   ├── oauth2/service.go                   # anyDelegatedSessionUnusable via evaluator
│   ├── consent/service.go                  # IsConnected and mandatory validation via evaluator
│   └── tokenexchange/session_diagnostic.go # identity_superseded → credential_identity_changed
├── adapters/
│   ├── storage/postgres/{thirdparty_provider*.go,user_session.go}
│   ├── storage/memory/{thirdparty_provider*.go,user_session.go}
│   └── http/
│       ├── handlers/admin/services_handler.go   # identity values never bound or returned (test only)
│       └── oauth2_sessions/handler.go           # identity reasons → 409 refresh_unavailable
└── app/builder.go                                # wire the new port and evaluator

web/src/
├── services/api/sessions.ts                 # requires_reauthentication
└── components/sessions/SessionCard.tsx      # "Sign-in required" state, Login action

api/enduser/openapi.yaml, api/admin/openapi.yaml
ARCHITECTURE.md, AGENTS.md (glossary, ADR index), docs/api/, docs/reference/token-exchange.md,
docs/guides/manage-agents-and-services.md, docs/configuration.md, charts/agentic-identity-broker/README.md

tests/
├── e2e/client_identity_change_test.go
├── e2e/frontend/client_identity_reconnect_test.go
├── e2e/pages/{consent_page.go,sessions_page.go}   # page-object additions
└── integration/migrations/client_identity_revision_migration_test.go
```

**Structure Decision**: Existing monorepo layout. The work extends the `oauth2session` bounded context and the storage adapters. No new domain package is added. Consumers in `oauth2` and `consent` depend on a port, not on `oauth2session` internals.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | Skipped |
| **Phase 1** | Setup | Minimal: move the approved ADR draft to `adrs/` and record the approval |
| **Phase 2** | Design preconditions: OpenAPI deltas, ADR, glossary, migrations, red E2E | **MANDATORY** |
| **Phase 2.7** | Entity boilerplate | Skipped |
| **Phase 2.5** | Foundational: ports, both adapters, revision recording, usability rule | Yes |
| **Phase 3+** | US1 (enforcement), US2 (consent and recovery UI), US3 (isolation and rotation) | Yes |
| **Phase N** | Constitution compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): skip. The change extends existing functions. No structural move is needed beyond what the feature requires.
- [x] Phase 2.7 (entity boilerplate): skip. No new entity or CRUD surface.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/client_identity_change_test.go` (in-memory store). Restart-survival variants carry the existing `docker` label and use PostgreSQL.

**Framework**: Ginkgo/Gomega, following [tests/e2e/README.md](../../tests/e2e/README.md) and the dual-server pattern. Admin `PUT` goes to the admin server; token exchange and browser flows go to the end-user server.

**Test Organization**: `Describe("Client identity change")` → `Context` per user story → one `It` per scenario.

**Scenario Mapping**:

| Spec Scenario | E2E Test Location | Test Description |
|---------------|-------------------|------------------|
| AS-1.1 | `tests/e2e/client_identity_change_test.go` | `It("denies an unexpired token after the client-ID file changes and returns the service recovery URI")` |
| AS-1.2 | same | `It("requires each affected user to reconnect independently, including sessions without expiry")` |
| AS-1.3 | same | `It("never refreshes a superseded session on demand, in the background, or in the admin sweep")` |
| AS-1.4 | same | `It("rejects a callback that began under the previous client identity and allows a fresh connection")` |
| AS-1.5 | same | `It("keeps an invalidated session unusable after the previous client ID is restored")` |
| AS-1.6 | same (+ `docker` actual-prior-release mixed-version/schema and restart variants) | `It("keeps legacy sessions through continuous rolling upgrade and invalidates them on the first later change")`; includes traffic continuity under fixed identities, old-work drain, and failed-upgrade/pre-activation abort. |
| AS-1.7 | same | `It("denies tokens for every user after an accepted Admin API client_id change")` |
| AS-2.1 | same | `It("shows agent consent on the next authorization despite an existing grant")` |
| AS-2.2 | same | `It("reconnects from consent under the current client ID and completes the original authorization")` |
| AS-2.3 | same | `It("recovers through the token error_uri and returns to sessions")` |
| AS-2.4 | same | `It("keeps the session unusable after a declined or failed reconnect and allows retry")` |
| AS-2.5 | same | `It("reports requires_reauthentication and not_connected for the superseded session")` |
| AS-3.1 | same | `It("leaves other services, broker login, and grants unchanged")`; matrix retains ADR 046 rejection, refreshability, and alias outcomes for unchanged services. |
| AS-3.2 | same | `It("keeps sessions usable through a filesystem secret rotation with the same client ID")` |
| AS-3.3 | same | `It("does not force consent for a changed service excluded from the grant")` |
| AS-3.4 | same | `It("keeps sessions usable through Admin API secret rotation, metadata updates, and same-identity source transitions")` |

*Line numbers are recorded during Phase 2 when the tests are written.*

**Red Phase Requirements**: As in the template. Tests compile, assert real status codes, bodies, headers, and redirects, fail semantically before implementation, and use no `XIt`, `PIt`, or `Skip()`.

**Test Data Strategy**:

- Existing fixtures: agents, permission sets, grants, principals, and the mock OAuth2 provider.
- New fixtures: a filesystem-bound "Reports" service with a temp-dir credential pair and helpers to publish a new pair, a stored "Reports" service with `client_id` `reports-client-v1`, and a "Calendar" control service. The mock provider accepts both `reports-client-v1` and `reports-client-v2`, so provider rejection never masks broker behavior.
- Legacy fixture (AS-1.6): a session inserted through the repository with nil `UpstreamClientID` and revision 0.
- Prior-release fixture (AS-1.6 `docker` variant): the pinned published prior-release image, overridable through `CLIENT_IDENTITY_PRIOR_RELEASE_IMAGE`, serving next to the new broker on the same expanded PostgreSQL schema ([research.md](research.md) R12).

**Bootstrap Strategy**: Production bootstrap via `tests/e2e/bootstrap/`. The credential-file directory is configured through `third_party_oauth2.credential_files`. The `docker` restart variant restarts the broker against the same PostgreSQL database. The `docker` mixed-version variant also starts the prior-release container through testcontainers with the same configuration document, credential directory, and database, reaching host services through the existing host tunnel (R12).

**Helper Utilities**: Reuse the existing token-exchange and authorize helpers (`authorization_flow_helpers_test.go`) and the matchers for `invalid_grant` and `error_uri`. Add a small helper to write credential files atomically through a new target and symlink swap (ADR 041 publication model).

### Frontend Playwright E2E Tests

**Test Location**: `tests/e2e/frontend/client_identity_reconnect_test.go`

**Framework**: The existing Playwright harness, with page objects in `tests/e2e/pages/`.

**Screenshot Location**: Maintained artifacts in `tests/e2e/screenshots/`. Capture is opt-in (`E2E_CAPTURE_SCREENSHOTS=true`, serial `GINKGO_FRONTEND_PROCS=1`); the harness first writes suite-relative `coverage/screenshots/`, then the existing maintained-image flow supplies the review artifacts. See [quickstart.md](quickstart.md).

| UI Scenario | Playwright Test Location | Screenshot Filename |
|-------------|--------------------------|---------------------|
| AS-2.1 State A: consent shows Reports needing sign-in | `tests/e2e/frontend/client_identity_reconnect_test.go` | `client_identity_consent_sign_in_required.png` |
| AS-2.2 State B: returned consent with Reports connected and selections preserved | same | `client_identity_consent_reconnected.png` |
| AS-2.5 State C: sessions page shows Reports "Sign-in required" | same | `client_identity_sessions_sign_in_required.png` |
| AS-2.3 State D: sessions page after direct recovery | same | `client_identity_sessions_reconnected.png` |
| AS-2.4: callback failure with retry action | same | `client_identity_reconnect_failed.png` |

### Deployment Safety

Proposed DEC-005's authoritative procedure is [ADR 047](adr-047-client-identity-revisions.md#guarded-rollout-and-rollback-proposed-dec-005). Keep existing traffic flowing across both ingress planes while old/new replicas overlap under fixed effective identities, sources, canonical IDs, and bindings. Same-ID secret rotation and metadata-only writes continue; pause identity-mutating automation/direct writers and defer identity-assigning registrations. Apply additive migrations online before new replicas start. Verify prior-release explicit-column reads, preservation of unknown columns on updates, and revision-0 inserts against the expanded schema using the actual supported prior release in the AS-1.6 PostgreSQL variant; do not simulate compatibility by disabling enforcement. New replicas enforce from startup. Drain only old processes/admitted work while healthy new replicas serve, then verify the complete serving/writing image inventory and prevent old-image restart before resuming identity changes. Failed upgrades retain healthy traffic and the freeze. Prior-binary abort before activation requires unchanged identities, zero increments, drained new work, and retained evidence; after activation rollback is revision-aware only. Document this in the chart/operator runbook without changing workflows, hooks, Helm settings, or adding a runtime bypass. AS-1.6 covers continuity, compatibility, incremental drain, abort, and legacy sessions; AS-1.4 covers callback claim-presence rejection and cross-version sign-in retry; AS-1.5 covers safe revision-aware rollback. Automated tests cannot substitute for operator freeze/inventory verification. Unrestricted mixed-version identity mutation is not supported.

### Unit & Integration Tests

**Unit Tests** (TDD, red first):

- `internal/domain/oauth2session/client_identity_test.go`: a usability rule table covering match, mismatch, superseded, legacy, rollback, each variant, and the cross-product with ADR 046's recorded rejection and refreshability states; source/version/canonical-ID/revision observation fencing and fresh service/binding/file reads on lost preconditions; fail-closed on source errors.
- `internal/domain/oauth2session/*_test.go`: the token-acquisition fast path, refresh under lock, callback revision check, sweep classification, and sealing for public, CIMD, DCR, and Google services.
- `internal/domain/oauth2/service_test.go` and `internal/domain/consent/service_test.go`: evaluator gate and display modes.
- `internal/domain/tokenexchange/session_diagnostic_test.go`: `identity_superseded` mapping.
- `internal/adapters/storage/memory/*_test.go`: recording rules and compare-and-set parity, with paused-observer filesystem→stored transitions, source round trips, canonical-ID/binding changes, and later same-ID updates that never spuriously invalidate sessions.
- `internal/adapters/http/handlers/admin/services_handler_test.go`: identity values supplied in a request are not bound, record nothing, and never appear in responses.
- `internal/adapters/http/oauth2_sessions/handler_test.go`: identity `409 refresh_unavailable`, unchanged `409 refresh_rejected`, and alias equality.
- `web/src/components/sessions/SessionCard.test.tsx`: `requires_reauthentication` gives "Sign-in required" and the Login action.

**Integration Tests**:

- `tests/integration/migrations/client_identity_revision_migration_test.go`: 047/048 apply, roll back, reapply, DOWN refusal, and lock-contention abort.
- Postgres repository tests in `internal/adapters/storage/postgres/`: the update recording rule within version and discovery preconditions, observation compare-and-set under concurrency (exactly one increment, atomic source/version/canonical-ID/revision fencing, obsolete values discarded after source/binding changes), and explicit-column reads.

**Test Coverage Goals**: All new domain branches. Both adapters at parity. E2E coverage of 100% of the 16 acceptance scenarios. Frontend E2E for all UI states.

## Complexity Tracking

No constitution violations. ADR 041 and ADR 038 are partially superseded through draft ADR 047 under Principle II. This is not an exception.
