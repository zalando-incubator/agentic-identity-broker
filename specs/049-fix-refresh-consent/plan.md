# Implementation Plan: Consent-Bound Refresh Sessions

**Branch**: `049-fix-refresh-consent` | **Date**: 2026-09-30 | **Spec**: [spec.md](spec.md)

**Input**: `specs/049-fix-refresh-consent/spec.md`

## Summary

Bind every local refresh to active user delegation and a durable refresh-session root. Revoke roots with their grant, agent, or credential lifecycle. Add a bounded, idempotent retry of the last refresh without branching or lifetime extension.

Keep Fosite for fresh OAuth protocol handling. An agent-scoped coordinator owns one transaction and shared decision time across authorization and refresh state. Eligible retries return the stored pair instead of rotating again. The existing EncryptionPort protects persisted results with a typed refresh-session subject.

The default retry interval is 30 seconds. Absolute lifetime defaults to unlimited. Inactivity remains 30 days through `local.refresh_token_ttl`. Existing JWT expiry, JWKS validation, proxy behavior, and third-party sessions remain unchanged.

This plan and tasks.md describe implementation work, not an implemented feature. Analysis remediation maps the current 47 scenarios. It preserves the owner's CLI and encrypted-backup decisions and leaves the constitution unchanged.

### Bounded, idempotent retry of the last refresh

This handles a successful refresh whose response the client did not receive. It does not permit another rotation using a consumed token.

1. The client sends RT₀. AIB consumes it and issues access token AT₁ and replacement refresh token RT₁.
2. If the response is lost, the client can retry RT₀ before the 30-second interval ends.
3. An eligible retry returns the same AT₁ and RT₁. It does not mint AT₂ or RT₂.

**Bounded**: The interval starts at RT₀'s first successful consumption and never restarts on retry. Consumption at 14:00:00 permits retries before 14:00:30, not at or after that deadline.

**Idempotent**: The returned token strings stay identical. `expires_in` reports AT₁'s remaining validity and can decrease. The retry advances no session lifetime or activity clock.

Only the immediately previous token qualifies while its replacement remains current and unused. If RT₁ already rotates to RT₂, RT₀ is too old. Its reuse is rejected and revokes the session.

Every retry still requires client authentication and binding, active consent, valid session lifetimes, and an unexpired AT₁. The requested scope must match the original request. The specification permits at most three stored-result returns per consumed token.

Revocation and expiry override the interval. `refresh_token_reuse_interval: 0s` disables these retries and restores strict single use.

An indeterminate commit returns token-free `server_error` without claiming rollback. A later request resolves durable state under the agent guard. Confirmed rollback permits ordinary rotation. Committed consumption follows existing retry rules, including zero reuse and the three-authorization limit. Unresolved state fails closed. No recovery path creates another successor, refunds a committed retry count, or extends a retry window.

### Refresh failure precedence

FR-029 checks authentication/binding, revocation, consent, lifetimes, refresh capability, token classification, then candidate response scopes. The first failure determines the response and audit reason. Capability removal rejects without mutation before replay classification. With capability allowed, prohibited reuse revokes before response-scope checks. Fresh and otherwise eligible retry results must pass current scope permissions before any token return. An eligible retry checks its exact original response scope and cannot substitute a narrower result.

## Technical Context

**Language/Version**: Go 1.27.1. Existing React/Playwright infrastructure is used for one browser regression journey.

**Primary Dependencies**: Fosite v0.49.0, lestrrat-go/jwx v4.5.0, chi v5, sqlx with pgx v5, Viper/Cobra, go-viper/mapstructure v2.5.0. No new runtime dependency.

**Storage**: PostgreSQL for durable restart/replica behavior. Memory implements in-process parity through staged writes and remains ephemeral.

**Testing**: Go testing/testify, Ginkgo/Gomega production-bootstrap E2E, Playwright, existing testcontainers PostgreSQL/bootstrap and migration harnesses.

**Target Platform**: Existing broker server deployment and Helm chart. This does not change the standalone ExtProc binary.

**Project Type**: Backend security feature in the existing web monorepo. No new frontend component or bounded context.

**Performance Goals**: Fresh/retry lookup remains indexed and constant-work with respect to token history. One agent-scoped critical section owns each decision. Lifecycle bulk revocation uses ownership indexes. No new throughput SLO is invented.

**Constraints**: Mandatory consent, fail-closed errors, identical-token recovery, no raw credentials at rest/logs, no nested owner commits, no upstream/metadata fetch or transaction-unaware signing lookup under the guard. Preserve exact deadline behavior and permanent invalidation.

**Scale/Scope**: Six stories and 47 acceptance scenarios. Local issuance in local/hybrid modes, lifecycle services, both storage adapters, shared time, at-rest encryption, configuration delivery, and documentation. Excludes immediate access-token revocation and a new public revocation endpoint.

## Constitution Check

The initial check allowed research because the specification defines scope, entities, configuration, external behavior, and acceptance criteria. Existing accepted ADRs remain the design constraints.

The post-design check passes for planning. Approval/release items below are explicit implementation gates, not claims of completed implementation.

### Design Preconditions (BLOCKING)

| Precondition | Design result | Implementation obligation |
|--------------|---------------|---------------------------|
| Domain model | PASS: root, token history, retry claims, and policy in data-model.md | Add domain models and invariants before persistence consumers. |
| Domain concepts | PASS: names and ownership defined | Update ARCHITECTURE.md glossary and refresh flow. |
| Entity IDs | PASS: new RefreshSessionID is planned under ADR 013 | Extend generator/catalogue, regenerate, update ID context documentation. |
| Configuration design | PASS: three durations, explicit-zero rules, cross-setting validation, YAML/env/CLI/Helm contract | Implement every source and preserve proxy-mode isolation. |
| Config examples | PASS: existing local/hybrid examples selected | Update examples and their README. |
| Helm contract | PASS for design: values, JSON schema, ConfigMap, README named | T004 updates the chart in Phase 2b before runtime work. T062 later proves rendered configuration through the production loader. |
| API design first | PASS for design: proposed refresh behavior is specified in feature Markdown; canonical APIs remain unchanged | T005 updates approved canonical documentation during implementation, before runtime edits. Do not publish pending behavior or add a separate refresh operation. |
| API documentation | PASS for design: canonical contracts and rendered guide identified | T005 adds docs/api/oauth2-refresh-sessions.md with denial, retry, rollback, and indeterminate-commit examples before runtime work. T070 verifies the examples. |
| API changes | PASS for design: user explicitly requested remediation | Record final contract review and major-release/version compatibility handling required by the constitution. |
| Database design | PASS: paired migration 036 and indexes specified | Recheck number; test apply/down/reapply without restoring credential usability. |
| E2E acceptance tests | PASS for design: all 47 scenarios mapped below | Author every primary journey before runtime changes. |
| E2E mapping | PASS: each scenario has one primary It location | Use stable scenario identifiers, not fabricated source line numbers. |
| E2E red phase | PASS for design: acceptance-linked feature assertions required | T015 records a semantic failure for every primary scenario. Standalone baseline regressions do not count toward these 47 entries. |
| Frontend E2E | PASS: existing UI revoke journey has one browser mapping | Use the existing page objects and local issuance bootstrap. |
| Screenshots | PASS: browser artifact named | Capture maintained revoke-before/after states in serial screenshot mode. |

### Implementation Considerations

| Principle / constraint | Post-design result |
|------------------------|--------------------|
| Security-first | PASS: consent has no opt-out; every successful retry reauthorizes. |
| Architecture documentation | PASS: existing architecture sections and glossary will change with implementation. |
| Binding ADRs | PASS for design: 004/013/014/032 retained. Proposed ADR 038 supersedes ADR 008's closed subject list; accept it before implementation. |
| Library-first security | PASS: existing EncryptionPort raw-key/KMS adapters, JWX signing, and random-token strategy. No custom crypto. |
| API guidelines | PASS: existing routes/status shapes; no new endpoint. |
| End-user docs | PASS: configuration, API behavior, residual JWT expiry, and migration effects are included in scope. |
| Migration testing | PASS: real PostgreSQL harness, rollback invalidation, and unrelated-record preservation. |
| Hexagonal architecture | PASS: issuer owns policy, ports expose native types, adapters own persistence and coordination. |
| Persistence patterns | PASS: sqlx, storage errors, both adapters, small ISP facets and factory accessors. |
| DI via Builder | PASS: all dependencies are required and wired in internal/app/builder.go, not routing. |
| Design-system rules | PASS: no styled UI change; existing consent surface/page objects are reused. |

**Implementation approval gate**: Accept ADR 038, including its ADR 008 supersession, and final API/release handling. Confirm the specification's recommended Open Decisions before runtime work. The feature owner explicitly selected constitution-compliant CLI support and allowed encrypted backup copies during task generation.

## Project Structure

### Documentation (this feature)

```text
specs/049-fix-refresh-consent/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── checklists/requirements.md
└── contracts/
    ├── lifecycle.md
    ├── configuration.md
    └── storage.md
adrs/038-consent-bound-refresh-sessions.md   # proposed
```

The executable task breakdown is in [tasks.md](tasks.md). Its design/test authoring pass precedes foundational runtime work.

### Source Code (repository root)

Existing files are extended unless a proposed file is explicitly marked new:

```text
internal/domain/id/{gen_ids.go,uuid_ids_gen.go}
internal/domain/storage/
  refresh_session.go                    # new aggregate and policy value
  refresh_token.go                      # new token-history model
internal/domain/oauth2server/
  provider.go
  fosite_storage.go
  strategies.go
  credential_service.go
  session_cleanup.go
  refresh_policy.go                     # new pure policy evaluation
  refresh_retry.go                      # new encrypted payload and bounded retry
internal/domain/consent/service.go
internal/domain/agents/service.go
internal/domain/encryption/subject.go
internal/adapters/encryption/branchkey/id.go
internal/adapters/encryption/aws/{branchkeysupplier.go,branchkeymanager.go}
internal/domain/impersonation/service.go
internal/domain/tokenexchange/service.go
internal/ports/{storage.go,oauth2.go,config.go,oauth2_mode_config.go}
internal/app/{builder.go,impersonation_delegation.go}
internal/adapters/storage/
  factory.go
  memory/                               # shared scoped staging + root/token stores
  postgres/                             # ambient transactions + root/token repositories
internal/config/{loader.go,validator.go}
cmd/agentic-identity-broker/root.go
api/{enduser,admin}/openapi.yaml
migrations/036_consent_bound_refresh_sessions.{up,down}.sql
charts/agentic-identity-broker/{values.yaml,values.schema.json,README.md}
charts/agentic-identity-broker/templates/configmap.yaml
examples/config/{oauth2-server-mode.yaml,oauth2-hybrid-mode.yaml,README.md}
docs/{configuration.md,changelog.md}
docs/api/oauth2-refresh-sessions.md        # new rendered end-user guide and examples
tests/e2e/{bootstrap,fixtures,helpers}/
tests/e2e/oauth2_refresh_{consent,lifecycle,retry,absolute,inactivity,policy}_e2e_test.go
tests/e2e/oauth2_refresh_{consent,lifecycle,retry,absolute,policy}_postgres_e2e_test.go
tests/e2e/frontend/refresh_consent_revocation_test.go
tests/integration/{migrations,storage,bootstrap}/
```

**Structure Decision**: Extend the existing issuer, consent/lifecycle services, storage adapters, and builder. Do not create a second OAuth service or encryption subsystem.

Remove the obsolete token-only RefreshTokenSession model/repository/store after every caller migrates. Existing tests and factory/cleanup callsites must migrate in the same cutover. No aliases or disabled fallback paths remain.

## Implementation Phase Overview

| Phase | Purpose | Required? |
|-------|---------|-----------|
| Contract approval | Accept ADR 038 and review canonical API/release changes | Mandatory before implementation |
| Design preconditions / red coverage | Domain/config/schema contracts, updated Helm contract, API guide, and 47 semantic-red acceptance journeys | Mandatory |
| Storage and coordination | Typed root/token state, transaction participation, memory write-set, migration | Mandatory |
| Consent and lifecycle | Continuing verifier, both grant-delete paths, agent delete, credential revoke/replace | P1 |
| Rotation and recovery | Borrowed Fosite scope, identical sealed response, client binding and replay outcomes | P2 |
| Lifetimes and deployment | Original clocks, startup reconciliation, CLI delivery, and runtime source/chart parity | P2 |
| Verification and docs | Gates, consumer smoke, migration/race parity, architecture and operations | Mandatory |

Skip an unrelated refactoring phase. Skip a separately shipped entity-boilerplate phase: no 501 handlers, empty repositories, or scaffold release is needed.

### Concrete integration sequence

1. Publish reviewed canonical contracts, the rendered API guide, and the approved release decision. Approve the ADR and update the Helm contract before runtime work.
2. Write all 47 acceptance journeys and focused boundary tests. Record acceptance-linked semantic failure for every primary journey before runtime implementation.
3. Introduce RefreshSessionID, aggregate/token models, policy evaluation, and repository facets. Add both backends, migration 036, and the complete core YAML/environment policy in T025.
4. Add the coordinator and make every participating repository read/write use its context. Credential replacement must join the owner transaction. Memory must stage only touched rows.
5. Make FositeStorage borrow the owner transaction. Preserve required standalone behavior. Propagate commit errors without retrying COMMIT or claiming an indeterminate outcome rolled back.
6. Inject the extended delegation verifier, coordinator, shared clock, EncryptionPort, branch-key manager, refresh facets, and policy through the builder. Migrate every provider and verifier caller; no compatibility bypass remains.
7. Guard initial refresh issuance and all refresh decisions. Prepare signing material/CIMD resolution outside the guard; re-read local authorization and credential identity inside it.
8. Revoke on both grant-deletion paths, agent deletion, explicit credential deletion, expired-grant renewal, and code replay. Credential replacement and active-grant edits preserve sessions. Publish lifecycle success and secrets only after commit; record revocation durably before agent cascade removal.
9. Implement 30-second idempotent retries with matching normalized requested scope and a maximum of three stored returns. Narrow access-token scope only, retain the root ceiling, and compare request-context fingerprints for audit. Use EncryptionPort and the approved refresh-session subject.
10. Evaluate grant/session/reuse deadlines from the shared clock. Classify consumed signatures before considering individual token expiry. Preserve expired ancestor replay evidence. Startup and maintenance own expiry and ciphertext erasure.
11. Verify the core policy from T025 without reimplementing it. Add CLI delivery and prove parity with YAML, environment, and the Phase 2b chart. Proxy defaults remain valid.
12. Remove obsolete token-only code and update existing strict-reuse tests to explicitly select zero interval. Add default recovery coverage rather than weakening replay assertions.
13. Run the verification sequence in quickstart.md and capture the direct-JWKS, browser revoke, restart, and two-replica outcomes.

### Cutover and rollback

Migration 036 is additive. Keep refresh_token_sessions readable/writable by old binaries and add nullable root/ancestry anchors. New code issues only anchored records. Validate FR-025 evidence independently. Before fresh consumption, lock and re-read the anchored legacy current row and retain its lock through rotation. Old-writer consumption without matching new history makes the original token and unsupported descendant invalid_grant without mutation or another successor. If the new rotation wins first, the old writer's conditional MarkUsed must fail. Unsupported lineage requires reauthorization, not a guessed start time.

Keep the old table through mixed-version operation. Revocation and terminal lifetime expiry invalidate matching legacy current/descendant rows in the same transaction as the root transition. Expiry also commits ExpiredAt/reason and ciphertext erasure. A failed mirror update rolls back the whole transition and blocks startup readiness. Before binary-only rollback, quiesce token traffic and old writers, then run expiry reconciliation under the outgoing policy. Admit old binaries only after reconciliation succeeds. No down migration is necessary for this protection. Revoked or terminally expired authority never returns. Backup restoration clears cached results and requires reauthorization where current history cannot be proved.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

Use Ginkgo/Gomega with real production app bootstrap, dual end-user/admin servers, fixtures, and full authorization-code/PKCE journeys. Use the existing Playwright harness for the actual UI revoke action.

**Primary scenario mapping**: All locations below are planned. No source line numbers or passed-test claims are invented. Each scenario maps to one It block bearing its identifier.

| Spec scenario | Planned E2E location | Journey assertion |
|---------------|----------------------|-------------------|
| US1-S1 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: rotation retains original issuance evidence, ownership, and scope ceiling |
| US1-S2 | `tests/e2e/frontend/refresh_consent_revocation_test.go` | Playwright: UI revocation prevents renewal |
| US1-S3 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: expired grant prevents renewal |
| US1-S4 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: consent overrides retry grace |
| US1-S5 | `tests/e2e/oauth2_refresh_consent_postgres_e2e_test.go` | PostgreSQL HTTP: unavailable consent preserves token state |
| US1-S6 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: another client cannot mutate the session |
| US1-S7 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: JWKS token keeps expiry but cannot renew |
| US1-S8 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: expired-grant renewal invalidates old sessions |
| US1-S9 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: active-grant changes preserve session clocks |
| US2-S1 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: principal-scoped revocation |
| US2-S2 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: both grant-delete paths preserve other sessions |
| US2-S3 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: agent-wide deletion |
| US2-S4 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: credential revoke blocks public fallback |
| US2-S5 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: regrant cannot restore revoked tokens |
| US2-S6 | `tests/e2e/oauth2_refresh_lifecycle_postgres_e2e_test.go` | PostgreSQL HTTP: lifecycle commit defeats racing descendants |
| US2-S7 | `tests/e2e/oauth2_refresh_lifecycle_postgres_e2e_test.go` | PostgreSQL HTTP: lifecycle failure rolls back |
| US2-S8 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: replacement credentials retain original retry results, clocks, and counts |
| US2-S9 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: idempotent missing-grant deletion revokes leftovers |
| US2-S10 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: identical predecessor recovery precedes code replay, which rejects both tokens and preserves an unrelated session |
| US3-S1 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: lost response returns identical pair |
| US3-S2 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: concurrent refresh has one successor |
| US3-S3 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: fixed deadline rejects and revokes |
| US3-S4 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: expired older predecessor still revokes its live family |
| US3-S5 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: zero interval enforces strict use |
| US3-S6 | `tests/e2e/oauth2_refresh_retry_postgres_e2e_test.go` | PostgreSQL HTTP: replica/restart returns identical pair |
| US3-S7 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: session expiry overrides grace |
| US3-S8 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: expired cached access preserves valid current token |
| US3-S9 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: retry scope mismatch is prohibited reuse |
| US3-S10 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: fourth stored-result request is prohibited reuse |
| US3-S11 | `tests/e2e/oauth2_refresh_retry_postgres_e2e_test.go` | PostgreSQL HTTP: indeterminate commit resolves without branching, count refunds, or zero-reuse recovery |
| US4-S1 | `tests/e2e/oauth2_refresh_absolute_e2e_test.go` | HTTP/CLI: absolute deadline does not slide |
| US4-S2 | `tests/e2e/oauth2_refresh_absolute_e2e_test.go` | HTTP/CLI: activity cannot pass absolute deadline |
| US4-S3 | `tests/e2e/oauth2_refresh_absolute_e2e_test.go` | HTTP/CLI: original start persists beyond 30 days with no absolute deadline |
| US4-S4 | `tests/e2e/oauth2_refresh_absolute_postgres_e2e_test.go` | PostgreSQL HTTP: restart and shared time preserve deadline |
| US5-S1 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: default idle expiry is 30 days |
| US5-S2 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: fresh rotation renews inactivity |
| US5-S3 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: resource access does not renew inactivity |
| US5-S4 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: retry advances no clocks |
| US5-S5 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: custom inactivity controls expiry |
| US6-S1 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: file/env/CLI/chart govern real behavior |
| US6-S2 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: existing inactivity key governs rotation, not duplicate retry activity |
| US6-S3 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: invalid combinations fail startup |
| US6-S4 | `tests/e2e/oauth2_refresh_policy_postgres_e2e_test.go` | PostgreSQL HTTP: unsupported legacy session requires reauthorization |
| US6-S5 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: hybrid local enforcement coexists with upstream independence and proxy parity |
| US6-S6 | `tests/e2e/oauth2_refresh_policy_postgres_e2e_test.go` | PostgreSQL HTTP: shorter deadlines persist; inactivity increases affect the next rotation only; absolute/shortened inactivity expiry blocks legacy renewal after binary-only rollback |
| US6-S7 | `tests/e2e/oauth2_refresh_policy_postgres_e2e_test.go` | PostgreSQL HTTP: old-first consumption rejects the anchored original and unsupported descendant; new-first locking defeats old consumption; neither order branches |
| US6-S8 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: bounded memory recovery succeeds before restart rejects current and previous tokens |

PostgreSQL journeys carry the `integration` build tag and a stable `Consent-Bound Refresh Sessions` focus. Extend the volatile bootstrap with a thin Ginkgo suite wrapper over the existing shared-container/template-database helper. Pass the enclosing per-process testing.T to that helper and clean each cloned database in AfterEach. Do not invent another container framework.

For the unavailable-grant journey, temporarily make the relevant grant relation unavailable in an isolated real database, then restore it. For lifecycle rollback, use a test-owned database constraint/trigger that rejects revocation writes. These are infrastructure faults, not mock repositories in production app wiring.

For US3-S11, use a test-owned PostgreSQL protocol fault around COMMIT. Suppress the transaction acknowledgement without replacing production repositories. Observe durable state independently after the connection closes. Exercise acknowledged rollback, committed rotation, committed retry count, unavailable resolution, and zero reuse. The response must contain no tokens or rollback guarantee.

Configuration journeys exercise the actual binary and production loader with rendered chart YAML. US6-S5 combines local consent denial with a stateful upstream control in hybrid mode, plus proxy parity. US6-S8 recovers an original pair before memory restart rejection. Verify the configuration reference separately, not through source-text assertions.

### Red phase and test data

- Every journey first obtains authority through real authorization and PKCE before exercising the changed transition.
- Assert concrete status/error codes, original token identity, successor usability, and absence of token members on denial.
- Race tests control database-lock interleavings and observe final consumer outcomes. They do not use arbitrary sleeps or source assertions.
- Every primary It compiles and fails on an acceptance-linked feature assertion before runtime implementation. US2-S10 first fails on its required identical-result recovery control, not on existing replay rejection. Existing baseline regressions stay separate. No skipped cases, placeholder failures, or unrelated assertions are permitted.
- Positive local journeys verify immutable origin, ownership, scope ceiling, and exact activity/deadline transitions through real storage and consumer outcomes. Do not substitute row-existence assertions for behavior.
- US2-S8 and US2-S10 require US3's positive retry controls. T050 completes those controls and US1-S4. T072 verifies their full acceptance results.
- Use fresh agents, principals, grants, local config, valid encryption/JWE keys, and independent storage per journey.
- Use short configured durations for HTTP timing. For default day-scale limits, use aged production-model fixture state after a complete authorization journey.
- Unit-test exact equality/before/after boundaries through pure policy evaluation with explicit timestamps.
- Do not poll successful refresh while waiting for inactivity expiry, because it renews activity. Poll time/state, then issue the deciding request.
- Use PostgreSQL for restart and replica identity recovery. Memory restart legitimately loses state and rejects prior tokens.

### Frontend Playwright E2E Tests

One primary browser case maps US1-S2. Reuse the existing ConsentPage revoke methods and a local-mode production bootstrap. Obtain the refresh token before browser revocation, confirm revoke through the UI, then attempt refresh and observe invalid_grant.

Capture maintained `refresh_consent_before_revoke.png` and `refresh_consent_after_revoke.png` states using the existing serial screenshot workflow. There is no new visual design or component.

### Unit & Integration Tests

**Issuer/domain**: Fixed failure precedence, expired consumed-ancestor replay, and unchanged state after authorization denial or confirmed rollback. Include indeterminate-commit resolution, committed retry counts, zero reuse, shared-time boundaries, and encrypted payload binding. T031 writes and runs fresh capability-removal, response-scope withdrawal, narrow-then-full-ceiling, and public-session promotion tests before T032–T034. T043 writes and runs retry capability/scope/promotion and overlapping-failure tests before T044–T047. Record genuine semantic-red evidence before each implementation wave.

**Storage parity**: Ambient transaction participation, bounded write-set rollback, one successor, scoped root/legacy revocation, immutable issuance evidence, live tombstone retention, terminal retention through every token expiry, and unrelated-record preservation.

**PostgreSQL**: Additive apply/down/reapply, mixed-version writers, one-connection safety, shared time, race/restart, startup reconciliation, confirmed rollback, and lost commit acknowledgement. Test old-first and new-first lock interleavings on the same anchored current token. A new instance rejects old-only consumption without another successor or request mutation. After absolute or shortened inactivity expiry, retain the schema and exercise legacy redemption with the old binary. It must reject current and descendant mirrors. A failed expiry mirror write preserves the prior transaction state and blocks startup readiness. Unrelated eligible sessions remain usable.

**Configuration**: Omission/zero, source precedence, malformed/numeric/negative durations, cross-setting validation, startup warnings, local/hybrid resolution, and chart-to-loader behavior. Policy-change tests persist shorter deadlines before expiry, then restart with larger values after that deadline. Increased inactivity never extends an issued token or its current interval. A valid fresh rotation applies the new duration to its successor. Absolute changes retain StartedAt and never restore elapsed stored deadlines.

**Existing regression protection**: Keep strict replay rejection under explicit zero interval. Preserve offline-access eligibility, PKCE, profile claims, current signing-key handling, metadata, upstream flows, and third-party token refresh.

**Coverage goals**: All 47 acceptance scenarios have one primary E2E case and an acceptance-linked semantic-red record. Test encryption and backup restoration without plaintext logs or resurrection. No arbitrary line-coverage percentage is added.

## Complexity Tracking

No constitution exception is requested. The new coordinator, root aggregate, and repository facets are required for the specified atomicity, identity recovery, and lifetime behavior.

Known costs are deliberate: per-agent serialization, durable encrypted recovery, and live-family signature retention. Research records simpler rejected alternatives and why they fail the acceptance contract.

## Validation Record

- Planning setup resolved branch and feature directory to 049.
- Two read-only research slices inspected lifecycle/transactions and encryption/configuration integration.
- LSP references identified the production provider construction and four test construction callsites.
- A disposable program demonstrated presence-aware duration decoding with pinned Viper/mapstructure dependencies. It was removed.
- Original planning validation covered 37 scenarios. Task-generation reconciliation covered 46. Analysis remediation maps 47, including US3-S11 for indeterminate commits. The retry default remains 30 seconds.
- The downstream prerequisite command discovered research.md, data-model.md, contracts/, and quickstart.md under feature 049. The documented just recipes were present in just --list.
- Runtime implementation, new migration execution, and the planned E2E journeys have not run in this planning command.
- Analysis-remediation validation checked 89 requirement mappings, 75 sequential tasks, and 47 matching primary scenario locations. All task predecessors exist. Helm, API-guide, and semantic-red gates precede runtime work.
- Earlier document validation checked OpenAPI/YAML syntax and Markdown tables. Its scoped quality check reported zero gating regressions and four non-gating verbosity findings.
- This remediation changed design artifacts only. It did not execute the planned runtime journeys or mark implementation tasks complete.
- The approved follow-up addresses C1, I1, A1, A2, U1, and U2 across the specification, tasks, model, contracts, and Proposed ADR. All 75 implementation tasks remain pending.
- Document validation rendered 11 Markdown artifacts and checked 17 tables, relative links, unique OpenAPI keys, local references, and unchanged endpoint/response shapes. Model field quotations still match their implementation tasks.
- Traceability validation found 89 mapped requirements and 47 matching primary scenario locations. Every task dependency exists and precedes its consumer. The revised test-first gates precede their implementation tasks. The prerequisite script still resolves feature 049. No runtime tests, migrations, or token journeys ran during this design-only remediation.
- Contract cleanup keeps proposed refresh behavior in contracts/lifecycle.md and removes the duplicate API definition. Canonical API contents match the pre-cleanup baseline. T005 owns approved canonical documentation updates during implementation, before runtime edits.
- Cleanup validation rendered 11 Markdown artifacts and resolved 21 relative links. All 89 requirement mappings, 75 pending tasks, and 47 primary scenarios remain intact. No retired contract references remain. Scoped quality-delta reports zero regressions and zero gating findings. No production code or published API behavior changed.
