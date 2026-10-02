# Implementation Plan: Consent-Bound Refresh Sessions for Local Token Minting

**Branch**: `049-fix-refresh-consent` | **Date**: 2026-09-30 | **Spec**: [spec.md](spec.md)

**Input**: `specs/049-fix-refresh-consent/spec.md`

**Scope**: Local token minting only: `local` mode and the local minting path of `hybrid` mode. No upstream refresh-policy changes.

## Summary

This plan changes only broker-issued user refresh sessions on the local minting path. It applies in `local` mode and the local path of `hybrid` mode. It does not apply to `proxy` mode or the upstream path of `hybrid` mode. Vaulted third-party provider tokens remain outside scope.

Bind every local refresh to active user delegation and a durable refresh-session root. Revoke roots with their grant, agent, or credential lifecycle. Add a bounded, idempotent retry of the last refresh without branching or lifetime extension.

Keep Fosite for fresh OAuth protocol handling. An agent-scoped coordinator owns one transaction and shared decision time across authorization and refresh state. Eligible retries return the stored pair instead of rotating again. The existing EncryptionPort protects persisted results with a typed refresh-session subject.

The default retry interval is 30 seconds. Absolute lifetime defaults to unlimited. Inactivity remains 30 days through `local.refresh_token_ttl`. Existing JWT expiry, JWKS validation, proxy behavior, and third-party sessions remain unchanged.

The specified runtime is implemented, and all 47 primary acceptance scenarios passed. The Validation Record contains initial classifications, final outcomes, and release blockers. Full verification remains blocked by the owner-retained gRPC dependency vulnerability. On 2026-10-01, the owner confirmed all five Open Decisions and approved the API/lifecycle contract with major-release handling for released-contract breaks. CLI support and encrypted backups remain approved. Constitution v2.2.0 requires semantic-red evidence for changed behavior and permits baseline-green evidence for unchanged behavior.

### Implementation mode boundary

The [specification's mode table](spec.md#scope-and-mode-boundary) defines the scope for every implementation phase:

- The local issuer owns consent checks, refresh rotation, bounded retries, lifetimes, and local session audit outcomes.
- Shared consent, agent, and credential services coordinate local session revocation. They introduce no upstream session revocation or third-party refresh policy.
- Session migration, rollout reauthorization, cleanup, and restore invalidation affect only locally issued refresh authority.
- Configuration remains under `oauth2_authorization_server.local`. Hybrid deployments use it only for their local minting path. Proxy deployments receive no local policy defaults.
- Upstream proxy routing, provider-controlled refresh, and vaulted third-party sessions retain their existing behavior. Client-credentials grants without user refresh sessions remain outside scope.

**Local minting only does not mean `mode: local` only.** US6-S5 must prove local enforcement and upstream independence in `hybrid` mode, plus unchanged `proxy` behavior.

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

**Scale/Scope**: Six stories and 47 acceptance scenarios. Broker-issued user refresh sessions in `local` mode and the local minting path of `hybrid` mode only. Includes lifecycle services, both storage adapters, shared time, at-rest encryption, configuration delivery, and documentation for those sessions. Excludes upstream proxy refresh, vaulted third-party refresh, immediate access-token revocation, and a new public revocation endpoint.

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
| E2E initial evidence | Acceptance-linked assertions required for all 47 journeys | T015 records each journey's observed initial outcome: semantic-red for changed behavior or baseline-green for unchanged behavior. Standalone regressions remain separate. |
| Frontend E2E | PASS: existing UI revoke journey has one browser mapping | Use the existing page objects and local issuance bootstrap. |
| Screenshots | PASS: browser artifact named | Capture maintained revoke-before/after states in serial screenshot mode. |

### Implementation Considerations

| Principle / constraint | Post-design result |
|------------------------|--------------------|
| Security-first | PASS: consent has no opt-out; every successful retry reauthorizes. |
| Architecture documentation | PASS: existing architecture sections and glossary will change with implementation. |
| Binding ADRs | PASS: 004/013/014/032 retained. Accepted ADR 038 extends ADR 008's subject list and establishes agent-scoped transaction ownership. |
| Library-first security | PASS: existing EncryptionPort raw-key/KMS adapters, JWX signing, and random-token strategy. No custom crypto. |
| API guidelines | PASS: existing routes/status shapes; no new endpoint. |
| End-user docs | PASS: configuration, API behavior, residual JWT expiry, and migration effects are included in scope. |
| Migration testing | PASS: real PostgreSQL harness, rollback invalidation, and unrelated-record preservation. |
| Hexagonal architecture | PASS: issuer owns policy, ports expose native types, adapters own persistence and coordination. |
| Persistence patterns | PASS: sqlx, storage errors, both adapters, small ISP facets and factory accessors. |
| DI via Builder | PASS: all dependencies are required and wired in internal/app/builder.go, not routing. |
| Design-system rules | PASS: no styled UI change; existing consent surface/page objects are reused. |

**Implementation approval gate**: Complete on 2026-10-01. ADR 038 was already accepted, including its narrow supersession of ADR 008's approved subject list. In this implementation session, the owner selected “Confirm all five” for the specification's Open Decisions and “Approve contract and release handling” for the API/lifecycle contract in spec.md and contracts/lifecycle.md. Keep existing endpoints and wire shapes; handle released-contract breaks through a major release. CLI support and encrypted backup copies remain approved. T005 publishes the approved canonical API documentation before runtime edits. These approvals do not establish runtime implementation or release readiness.

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
adrs/038-consent-bound-refresh-sessions.md   # accepted; implementation pending
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
cmd/agentic-identity-broker/refresh_sessions.go  # new one-shot restore invalidation command
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
| Contract approval | Review final API/release changes and Open Decisions; ADR 038 is already accepted | Mandatory before implementation |
| Design preconditions / initial coverage | Domain/config/schema contracts, updated Helm contract, API guide, and 47 acceptance journeys with classified initial evidence | Mandatory |
| Storage and coordination | Typed root/token state, transaction participation, memory write-set, migration | Mandatory |
| Consent and lifecycle | Continuing verifier, both grant-delete paths, agent delete, credential revoke/replace | P1 |
| Rotation and recovery | Borrowed Fosite scope, identical sealed response, client binding and replay outcomes | P2 |
| Lifetimes and deployment | Original clocks, startup reconciliation, CLI delivery, and runtime source/chart parity | P2 |
| Verification and docs | Gates, consumer smoke, migration/race parity, architecture and operations | Mandatory |

Skip an unrelated refactoring phase. Skip a separately shipped entity-boilerplate phase: no 501 handlers, empty repositories, or scaffold release is needed.

### Concrete integration sequence

1. Publish reviewed canonical contracts, the rendered API guide, and the approved release decision during implementation. ADR 038 is already accepted. Update the Helm contract before runtime work.
2. Write all 47 acceptance journeys and focused boundary tests. Before runtime implementation, record acceptance-linked semantic-red evidence for changed behavior and observed baseline-green evidence for unchanged behavior.
3. Introduce RefreshSessionID, aggregate/token models, policy evaluation, and repository facets. Add both backends, migration 036, and the complete core YAML/environment policy in T025.
4. Add the coordinator and make every participating repository read/write use its context. Credential replacement must join the owner transaction. Memory must stage only touched rows.
5. Make FositeStorage borrow the owner transaction. Preserve required standalone behavior. Propagate commit errors without retrying COMMIT or claiming an indeterminate outcome rolled back.
6. Inject the extended delegation verifier, coordinator, shared clock, EncryptionPort, branch-key manager, refresh facets, and policy through the builder. Migrate every provider and verifier caller; no compatibility bypass remains.
7. Guard initial refresh issuance and all refresh decisions. Prepare signing material/CIMD resolution outside the guard; re-read local authorization and credential identity inside it.
8. Revoke on both grant-deletion paths, agent deletion, explicit credential deletion, expired-grant renewal, and code replay. FR-040 preserves code replay by a different client authenticated as itself. Refresh-token binding protections do not suppress this exception. Credential replacement and active-grant edits preserve sessions. Publish lifecycle success and secrets only after commit. Record revocation durably before agent cascade removal.
9. Implement 30-second idempotent retries with matching normalized requested scope and a maximum of three stored returns. Narrow access-token scope only, retain the root ceiling, and compare request-context fingerprints for audit. Use EncryptionPort and the approved refresh-session subject.
10. Evaluate grant/session/reuse deadlines from the shared clock. Classify consumed signatures before individual token expiry. Preserve expired ancestor replay evidence. Startup and maintenance own expiry and ciphertext erasure. Persist only shorter effective retry deadlines while keeping sealed original expiry fixed. Deny at equality and meet DB-006's 1-second cleanup bound. Shortened or zero reuse must reconcile before readiness.
11. Verify the core policy from T025 without reimplementing it. Add CLI delivery and prove parity with YAML, environment, and the Phase 2b chart. Proxy defaults remain valid.
12. Remove obsolete token-only code and update existing strict-reuse tests to explicitly select zero interval. Add default recovery coverage rather than weakening replay assertions.
13. Run the verification sequence in quickstart.md and capture the direct-JWKS, browser revoke, restart, and two-replica outcomes.

### Cutover and rollback

Migration 036 is additive. Keep refresh_token_sessions readable/writable by old binaries and add nullable root/ancestry anchors. New code issues only anchored records. FR-025 evidence comes from new-issuer root/token records, including the original active grant, first issuance, complete ancestry, and revocation history. Every pre-feature unanchored session requires fresh authorization. Before fresh consumption, lock and re-read the anchored legacy current row through rotation. Old-writer consumption without matching new history makes the original and unsupported descendant invalid_grant without mutation or another successor. If new rotation wins first, the old writer's conditional MarkUsed must fail.

Keep the old table through mixed-version operation. Revocation and terminal expiry invalidate matching legacy current/descendant rows with the root transition. Expiry also commits ExpiredAt/reason and ciphertext erasure. A failed mirror update rolls back that transition and blocks readiness. Before binary-only rollback, quiesce token traffic and old writers, then reconcile expiry under the outgoing policy. Admit old binaries only after reconciliation succeeds. No down migration is necessary for this protection. Revoked or terminally expired authority never returns.

After a database or refresh-state restore, operators keep all brokers and token writers stopped. Run `agentic-identity-broker --config <file> refresh-sessions invalidate-restored` before admission. T059 authors failing command integrations. T063 implements the command and its coordinated maintenance path. T064 documents the procedure, and T065/T074 exercise it. The command invalidates every restored local root and unused legacy row, including unanchored rows. Grants, agents, credentials, signing keys, and third-party sessions remain unchanged. Failed or indeterminate completion keeps traffic stopped until an acknowledged successful idempotent rerun. The [restore procedure](data-model.md#restore-invalidation) defines batching, receipts, terminal-state preservation, and final completeness checks. No snapshot-local evidence or automatic restore detector proves current revocation history.


## Testing Strategy

### End-to-End (E2E) Acceptance Tests

Use Ginkgo/Gomega with real production app bootstrap, dual end-user/admin servers, fixtures, and full authorization-code/PKCE journeys. Use the existing Playwright harness for the actual UI revoke action.

Changed-behavior journeys exercise broker-issued local refresh sessions. US6-S5 is the mode-isolation journey, not an extension of this policy to upstream sessions. It proves local enforcement in `hybrid` mode and unchanged upstream behavior in both `hybrid` and `proxy` modes.

**Primary scenario mapping**: All locations below are planned. No source line numbers or passed-test claims are invented. Each scenario maps to one It block bearing its identifier.

| Spec scenario | Planned E2E location | Journey assertion |
|---------------|----------------------|-------------------|
| US1-S1 | `tests/e2e/oauth2_refresh_consent_postgres_e2e_test.go` | PostgreSQL HTTP: rotation retains original issuance evidence, ownership, and scope ceiling |
| US1-S2 | `tests/e2e/frontend/refresh_consent_revocation_test.go` | Playwright: UI revocation prevents renewal |
| US1-S3 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: expired grant prevents renewal |
| US1-S4 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: consent overrides retry grace |
| US1-S5 | `tests/e2e/oauth2_refresh_consent_postgres_e2e_test.go` | PostgreSQL HTTP: unavailable consent preserves token state |
| US1-S6 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: another client cannot mutate the session |
| US1-S7 | `tests/e2e/oauth2_refresh_consent_e2e_test.go` | HTTP/CLI: JWKS token keeps expiry but cannot renew |
| US1-S8 | `tests/e2e/oauth2_refresh_consent_postgres_e2e_test.go` | PostgreSQL HTTP: expired-grant renewal invalidates old sessions |
| US1-S9 | `tests/e2e/oauth2_refresh_consent_postgres_e2e_test.go` | PostgreSQL HTTP: active-grant changes preserve session clocks |
| US2-S1 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: principal-scoped revocation |
| US2-S2 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: both grant-delete paths preserve other sessions |
| US2-S3 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: agent-wide deletion |
| US2-S4 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: credential revoke blocks public fallback |
| US2-S5 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: regrant cannot restore revoked tokens |
| US2-S6 | `tests/e2e/oauth2_refresh_lifecycle_postgres_e2e_test.go` | PostgreSQL HTTP: lifecycle commit defeats racing descendants |
| US2-S7 | `tests/e2e/oauth2_refresh_lifecycle_postgres_e2e_test.go` | PostgreSQL HTTP: lifecycle failure rolls back |
| US2-S8 | `tests/e2e/oauth2_refresh_lifecycle_postgres_e2e_test.go` | PostgreSQL HTTP: replacement credentials retain original retry results, clocks, and counts |
| US2-S9 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: idempotent missing-grant deletion revokes leftovers |
| US2-S10 | `tests/e2e/oauth2_refresh_lifecycle_e2e_test.go` | HTTP/CLI: identical predecessor recovery precedes code replay by another authenticated client, rejecting both tokens and preserving an unrelated session |
| US3-S1 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: lost response returns identical pair |
| US3-S2 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: concurrent refresh has one successor |
| US3-S3 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: fixed deadline rejects and revokes |
| US3-S4 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: expired older predecessor still revokes its live family |
| US3-S5 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: zero interval enforces strict use |
| US3-S6 | `tests/e2e/oauth2_refresh_retry_postgres_e2e_test.go` | PostgreSQL HTTP: replica/restart returns identical pair |
| US3-S7 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: session expiry overrides grace |
| US3-S8 | `tests/e2e/oauth2_refresh_retry_postgres_e2e_test.go` | PostgreSQL HTTP: expired cached access preserves valid current token |
| US3-S9 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: retry scope mismatch is prohibited reuse |
| US3-S10 | `tests/e2e/oauth2_refresh_retry_e2e_test.go` | HTTP/CLI: fourth stored-result request is prohibited reuse |
| US3-S11 | `tests/e2e/oauth2_refresh_retry_postgres_e2e_test.go` | PostgreSQL HTTP: indeterminate commit resolves without branching, count refunds, or zero-reuse recovery |
| US4-S1 | `tests/e2e/oauth2_refresh_absolute_e2e_test.go` | HTTP/CLI: absolute deadline does not slide |
| US4-S2 | `tests/e2e/oauth2_refresh_absolute_e2e_test.go` | HTTP/CLI: activity cannot pass absolute deadline |
| US4-S3 | `tests/e2e/oauth2_refresh_absolute_postgres_e2e_test.go` | PostgreSQL HTTP: original start persists beyond 30 days with no absolute deadline |
| US4-S4 | `tests/e2e/oauth2_refresh_absolute_postgres_e2e_test.go` | PostgreSQL HTTP: restart and shared time preserve deadline |
| US5-S1 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: default idle expiry is 30 days |
| US5-S2 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: fresh rotation renews inactivity |
| US5-S3 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: resource access does not renew inactivity |
| US5-S4 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: retry advances no clocks |
| US5-S5 | `tests/e2e/oauth2_refresh_inactivity_e2e_test.go` | HTTP/CLI: custom inactivity controls expiry |
| US6-S1 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: file/env/CLI/chart govern real behavior |
| US6-S2 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: existing inactivity key governs rotation, not duplicate retry activity |
| US6-S3 | `tests/e2e/oauth2_refresh_policy_e2e_test.go` | HTTP/CLI: invalid combinations fail startup |
| US6-S4 | `tests/e2e/oauth2_refresh_policy_postgres_e2e_test.go` | PostgreSQL HTTP: every pre-feature unanchored session reauthorizes; a fully evidenced anchored new-issuer family continues |
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
- Every primary It compiles and has an observed initial acceptance result before runtime implementation. Changed behavior must fail semantically; unchanged behavior can pass as an evidenced baseline. US2-S10 first fails on its required identical-result recovery control, not on existing replay rejection. Standalone regressions stay separate. No skipped cases, placeholder failures, or unrelated assertions are permitted.
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

**Issuer/domain**: Fixed failure precedence, expired consumed-ancestor replay, and unchanged state after refresh authorization denial or confirmed rollback. Include indeterminate-commit resolution, committed counts, zero reuse, shared-time boundaries, and encrypted payload binding. T031 runs fresh capability/scope/promotion tests red before T032–T034. T043 runs retry, overlapping-failure, and cleanup tests red before T044–T049. Cleanup tests cover the 1-second bound, readiness recovery, and shorter effective deadlines without changing sealed original expiry. US2-S10 uses another authenticated client for FR-040's separate code-replay exception.

**Storage parity**: Ambient transaction participation, bounded write-set rollback, one successor, scoped root/legacy revocation, immutable issuance evidence, live tombstone retention, terminal retention through every token expiry, and unrelated-record preservation.

**PostgreSQL**: Additive apply/down/reapply, mixed-version writers, one-connection safety, shared time, race/restart, startup reconciliation, confirmed rollback, and lost commit acknowledgement. Test old-first and new-first lock interleavings on the same anchored current token. A new instance rejects old-only consumption without another successor or request mutation. After absolute or shortened inactivity expiry, retain the schema and exercise legacy redemption with the old binary. It must reject current and descendant mirrors. A failed expiry mirror write preserves the prior transaction state and blocks startup readiness. Unrelated eligible sessions remain usable.

**Configuration**: Omission/zero, precedence, malformed/numeric/negative durations, relation validation, startup warnings, local/hybrid resolution, and chart-to-loader behavior. Policy-change tests persist shorter deadlines before expiry, then restart with larger values. Increased inactivity never extends an issued token or its current interval. A fresh rotation applies the new duration to its successor. Absolute changes retain StartedAt and cannot restore elapsed stored deadlines. Shorter reuse clamps effective RetryExpiresAt without changing ReuseUntil or sealed payload expiry. Zero reuse erases every cached result before readiness. Tests prove an eligible retry still works after a nonzero shortening.

**Existing regression protection**: Keep strict replay rejection under explicit zero interval. Preserve offline-access eligibility, PKCE, profile claims, current signing-key handling, metadata, upstream flows, and third-party token refresh.

**Coverage goals**: All 47 acceptance scenarios have one primary E2E case, an observed classified initial result, and a post-implementation passing result. Changed behavior requires semantic-red evidence; unchanged behavior permits baseline-green evidence. Test encryption and backup restoration without plaintext logs or resurrection. No arbitrary line-coverage percentage is added.

**Restore integration**: T059's focused integrations are separate from the 47 primary acceptance journeys. Restore an encrypted snapshot, stop all token writers, and run the real one-shot maintenance command. Test root/legacy invalidation, unanchored rows without roots, preserved terminal reasons, and receipt/mirror/commit faults. A failed command blocks admission. An acknowledged idempotent rerun rejects restored current and predecessor tokens. Fresh authorization succeeds without changing third-party sessions or credentials.


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
- The approved follow-up addresses C1, I1, A1, A2, U1, and U2 across the specification, tasks, model, contracts, and ADR 038. All 75 implementation tasks remain pending.
- Document validation rendered 11 Markdown artifacts and checked 17 tables, relative links, unique OpenAPI keys, local references, and unchanged endpoint/response shapes. Model field quotations still match their implementation tasks.
- Traceability validation found 89 mapped requirements and 47 matching primary scenario locations. Every task dependency exists and precedes its consumer. The revised test-first gates precede their implementation tasks. The prerequisite script still resolves feature 049. No runtime tests, migrations, or token journeys ran during this design-only remediation.
- Contract cleanup keeps proposed refresh behavior in contracts/lifecycle.md and removes the duplicate API definition. Canonical API contents match the pre-cleanup baseline. T005 owns approved canonical documentation updates during implementation, before runtime edits.
- Cleanup validation rendered 11 Markdown artifacts and resolved 21 relative links. All 89 requirement mappings, 75 pending tasks, and 47 primary scenarios remain intact. No retired contract references remain. Scoped quality-delta reports zero regressions and zero gating findings. No production code or published API behavior changed.
- ADR 038 records only the encryption-subject extension and agent-scoped transaction ownership, with rationale, alternatives, and consequences. Feature policy and field definitions remain in the specification and design contracts.
- On 2026-10-01, the user explicitly approved the narrowed ADR: "I accept ADR 038". Its status is Accepted. ADR 008 and agent routing reflect the narrow subject-list supersession. Open Decisions, API/release approval, and all 75 implementation tasks remain pending.
- The user approved remediation of I1, U1, A1, I2, and I3 from the current analysis. C1 is intentionally unchanged. The design preserves the code-replay exception, assigns full-restore invalidation, bounds live-ciphertext erasure, aligns anchored evidence, and makes Open Decisions provisional. All 75 runtime tasks remain pending.
- Current remediation validation rendered 10 Markdown artifacts and 17 tables, parsed two YAML examples, and found no broken relative links or heading anchors. All 89 requirements map to 75 pending tasks and 47 matching primary scenarios. All 36 model-field constraints match their task quotations. The dependency graph and C1 approval order remain unchanged. Canonical APIs, ADR 038, and the constitution retain their pre-remediation contents. Scoped quality-delta reports zero regressions and zero gating findings. No runtime tests, migrations, restore commands, or token journeys ran.
- T001 approval recorded on 2026-10-01: the owner confirmed all five recommended Open Decisions and approved the feature's API/lifecycle contract and major-release handling for released-contract breaks. No choice changed; requirement/task/scenario mappings remain applicable. ADR 038 acceptance, CLI support, and encrypted-backup approval were preserved without requesting approval again.
- T001 traceability validation found 75 ordered tasks, 89 mapped requirements, and 47 unique primary scenarios. Every referenced dependency precedes its consumer. No approved option changed.
- T002 completed with LSP references and implementations. research.md records exact delegation, grant, provider, factory, and encryption consumers, current module/migration versions, and existing ignore-file coverage. No runtime dependency or ignore-file change was necessary.
- T003 documents the approved refresh aggregate, coordinator/shared clock, encrypted retry payload, independent receipts, lifecycle distinctions, deadline erasure, lineage retention, and offline restore invalidation in ARCHITECTURE.md. This design update does not claim implemented runtime support.
- T007 reviewed the existing browser revoke journey and ConsentPage methods. US1-S2 reuses NavigateToAgent, ClickRevokeButton, WaitForRevokeDialog, ConfirmRevoke, and WaitForRevokeDialogDismissed. No React component or style changes are required. Maintain refresh_consent_before_revoke.png and refresh_consent_after_revoke.png through the serial screenshot workflow under tests/e2e/screenshots/. Screenshot capture and the new token-renewal browser journey have not run.
- T004 completed. Helm rendered local/hybrid quoted string defaults 30s/0s/720h and proxy configuration without a local policy block. Explicit reuse 0s remained a string. Chart schema rejected numeric reuse, negative absolute lifetime, and unsupported inactivity unit 1d. Both example YAML files parsed. Broker-loader/runtime parity remains T062.
- T005 completed. Approved canonical refresh/lifecycle documentation and docs/api/oauth2-refresh-sessions.md retain existing routes and token response members. Both OpenAPI documents parsed, and POST /oauth2/token retains operationId tokenExchange. The changelog records major-release handling without inventing a release version. Runtime examples remain unverified until the feature journeys execute.
- The pre-feature binary built successfully from source revision b4bdd1cf4dce2c23fb65223a0b31a46d29d40008. It is preserved outside tracked source at /tmp/aib-refresh-prefeature.0uNu8j/agentic-identity-broker, SHA-256 a174ef70679f170cc8e93d285d101d47a2da8a3c6aa773aeab94bb93f2c9a70b. Its CLI does not provide --version; invoking that flag returned its help with an unknown-flag error.
- T006 completed. `go test -tags=integration -count=1 -run '^TestMigration036' ./tests/integration/migrations` compiled and ran against cloned PostgreSQL databases. All six top-level migration tests failed because migration 036 does not exist, including all four deletion-fault subtests. This is migration-schema red evidence, not evidence for any of the 47 primary journeys.
- T008 is in progress. New production-bootstrap dual-server, native client/configuration, and HTTP/PKCE helpers compiled and completed a throwaway smoke against the unchanged runtime. Authorization issued a pair; fresh rotation returned 200; predecessor replay returned 400 invalid_grant; the successor then returned 400 invalid_grant. The smoke printed no credentials and its temporary source was removed. The pre-feature binary remains available at the recorded external path.
- The smoke demonstrates that US3-S5's strict-use behavior already passes before feature implementation. T015 nevertheless requires an acceptance-linked semantic failure for every primary scenario, and prohibits unrelated/artificial failures. Baseline classification or acceptance criteria must be reconciled before dependent implementation. No primary journey is marked red or green from this smoke; the constitution and requirements remain unchanged pending the owner's decision.
- `just check` initially identified formatting in two new fixture files. After scoped gofmt, it passed formatting, go vet, and lint with zero issues. The scoped quality-delta reports six gating findings, including test-helper clone patterns and expanded token-endpoint documentation. It does not establish a clean final quality gate; runtime implementation and final convergence remain pending.
- The owner selected “Allow unchanged baseline passes” on 2026-10-01. Constitution v2.2.0 adds the approved narrow clarification to VIII/XIII. The mandatory initialization hook ran and skipped because this worktree already belongs to a Git repository. The active constitution scaffold resolved successfully; version/report/dates and all 13 principles validated. Feature initial-evidence gates now classify each of the same 47 journeys as semantic-red for changed behavior or observed baseline-green for unchanged behavior. No acceptance assertion, scenario, or functional requirement was removed.
- Reconciliation validation preserved all 75 ordered task dependencies, all 89 requirement mappings, and all 47 unique primary scenario mappings. Historical checklist notes retain the earlier policy; checklist files and markers were not changed.
- T008 completed. Production-bootstrap memory and cloned-PostgreSQL helpers provide dual servers, HTTP authorization-code/PKCE issuance, credential lifecycle, restartable applications, native encrypted configuration, shared database time, locked legacy-token consumption, root observation, and initial-session aging. The E2E suite supplies its enclosing testing.T and owns shared-container teardown. The captured pre-feature binary remains outside source with the recorded revision/checksum. PostgreSQL smoke observed successful PKCE issuance and rejected renewal after a committed shared-time old-writer consumption, then closed the cloned database and shared container. The temporary smoke test was removed. New root-observation/aging queries compile but cannot execute against the pre-feature schema; their behavior is verified after migration 036. No primary journey result is claimed from fixture smoke.


### T015 observed initial acceptance evidence

All47 primary journeys ran against the unchanged runtime before implementation. The final classified evidence is39 semantic-red and8 baseline-green. The browser exercised the real revoke action and captured both maintained states. All failed primary reports identify an It assertion, not a BeforeEach failure. Unrelated focused-out specs are not primary skips.

Initial reports: `/tmp/aib-refresh-initial-backend.json` and `/tmp/aib-refresh-initial-browser.json`. Corrected fixture outcomes replace affected entries from `/tmp/aib-refresh-initial-corrected.json` and `/tmp/aib-refresh-initial-policy-source-complete.json`. These are local execution artifacts, not runtime support claims.

| Scenario | Initial classification | Observed acceptance assertion/control |
|----------|------------------------|---------------------------------------|
| US1-S1 | semantic-red | Persist first grant/token/start/ownership and immutable scope ceiling |
| US1-S2 | semantic-red | 400 invalid_grant after actual UI revoke; observed200 |
| US1-S3 | semantic-red | 400 invalid_grant after grant deadline; observed200 |
| US1-S4 | semantic-red | Eligible identical recovery before consent override; observed400 instead of200 |
| US1-S5 | semantic-red | Unavailable grant relation returns server_error; observed200 |
| US1-S6 | baseline-green | Another authenticated client cannot mutate the owner session; owner remains usable |
| US1-S7 | semantic-red | JWKS-valid original JWT keeps expiry; revoke prevents renewal; observed200 renewal |
| US1-S8 | semantic-red | Confirmed expired-grant renewal keeps old tokens unusable; observed200 renewal |
| US1-S9 | semantic-red | Successful active-grant edits preserve original native clocks and origin evidence |
| US2-S1 | semantic-red | Revoked principal descendants fail while unrelated principal survives; observed200 |
| US2-S2 | semantic-red | Both grant deletion paths end only selected authority; observed200 |
| US2-S3 | baseline-green | Agent deletion rejects its sessions and preserves unrelated agents |
| US2-S4 | semantic-red | Credential revocation/public fallback cannot renew; observed200 |
| US2-S5 | semantic-red | Regrant does not restore earlier refresh authority; observed200 |
| US2-S6 | semantic-red | Committed lifecycle action defeats racing current descendants; observed200 |
| US2-S7 | semantic-red | Real revocation-write fault rolls back lifecycle; observed204 instead of500 |
| US2-S8 | semantic-red | Replacement credential recovers original pair without clock/count reset; observed400 |
| US2-S9 | semantic-red | Absent-grant idempotent empty POST returns204 and revokes leftovers; observed400 |
| US2-S10 | semantic-red | Identical recovery succeeds before authenticated code replay; observed400 |
| US3-S1 | semantic-red | Recover original pair after lost response; observed400 instead of200 |
| US3-S2 | semantic-red | Concurrent current uses converge on one successor; one request observed400 |
| US3-S3 | semantic-red | Positive recovery precedes fixed-window at-or-after denial; observed400 |
| US3-S4 | baseline-green | Expired consumed ancestor still revokes live family |
| US3-S5 | baseline-green | Explicit-zero second use rejects and ends remaining authority |
| US3-S6 | semantic-red | Replica/restart returns original pair; observed400 |
| US3-S7 | semantic-red | Positive recovery precedes session-lifetime override; observed400 |
| US3-S8 | semantic-red | Valid post-mint delay permits original pair before access expiry; observed400 |
| US3-S9 | semantic-red | Positive recovery precedes changed-scope prohibited reuse; observed400 |
| US3-S10 | semantic-red | Three original-result returns succeed before fourth revokes; observed400 |
| US3-S11 | semantic-red | Real lost COMMIT acknowledgement recovers only existing durable successor; observed400 |
| US4-S1 | semantic-red | Rotation cannot reset original absolute deadline; observed200 after deadline |
| US4-S2 | semantic-red | Continuous activity cannot exceed original absolute deadline; observed200 |
| US4-S3 | baseline-green | Verified35-day native history remains active with unlimited absolute lifetime |
| US4-S4 | semantic-red | Replica/restart preserves original shared deadline; observed200 after deadline |
| US5-S1 | baseline-green | Default719h active control succeeds and721h inactive token is denied |
| US5-S2 | baseline-green | Fresh successor remains usable after original interval and uses configured lifetime |
| US5-S3 | baseline-green | Signed/JWKS-verified resource traffic does not renew inactivity |
| US5-S4 | semantic-red | Eligible duplicate returns original pair without advancing clocks; observed400 |
| US5-S5 | semantic-red | Configured inactivity deadline denies renewal under shared time; observed200 |
| US6-S1 | semantic-red | Real YAML binary honors identical predecessor recovery; observed400 instead of200 |
| US6-S2 | semantic-red | Existing inactivity key allows recovery without activity extension; observed400 |
| US6-S3 | semantic-red | Malformed reuse must prevent startup; binary continued serving until diagnostic timeout |
| US6-S4 | semantic-red | Pre-feature unanchored authority requires reauthorization; observed200 renewal |
| US6-S5 | semantic-red | Hybrid local consent revocation denies renewal independently of upstream; observed200 |
| US6-S6 | semantic-red | Shorter restart lifetime ends native session from original clocks; observed200 |
| US6-S7 | semantic-red | Unsupported old-writer successor cannot renew on new issuer; observed200 |
| US6-S8 | semantic-red | Positive memory recovery precedes new-store restart rejection; observed400 |

The full original assertions remain unchanged. Baseline-green journeys are required regression protection under constitution v2.2.0, including the already working expired-ancestor and strict-use cases. Fixture corrections used real provider authorization/PKCE and server-side UTC grant expiry, not fabricated failures. The existing loader selects YAML before its config flag binding; the binary fixture explicitly supplies the documented config-path environment variable. Restore/CLI delivery must select the requested file before YAML loading in T025/T061. Exact policy equality and deliberately skewed-instance validation remain their later explicit verification tasks, not claimed by this initial run.
- T016 generated RefreshSessionID through the existing UUID generator, updated its ID catalogue, and passed the ID package tests. New model compilation declarations were isolated from runtime wiring.
- T017 authored origin/owner/ceiling, predecessor-binding, retry-count, retention, terminal monotonicity, deadline equality, finite-issued-expiry, and token-identity/consumption transition tests before validation logic. The targeted model test command compiled and failed semantically with invalid authority/rewrites incorrectly accepted. After implementing real validators, the same targeted command passed. No provisional validation body remains in the new models.
- T018–T020 implemented real native root/token validators, the non-credential receipt projection, and focused coordinator/clock/repository facets. The model smoke exercised issuance, rotation, retry, and irreversible restore revocation; scoped static checks passed after formatting generated IDs and the receipt projection.
- T024 moved before repository implementation because the PostgreSQL contract harness requires the additive schema. Its dependencies are T020/T006; T023 additionally requires T024. Migration 036 now passes `go test -tags=integration ./tests/integration/migrations -run '^TestMigration036' -count=1`, including apply/down/reapply, lineage constraints, legacy compatibility, independent receipts, and deletion-fault rollback. Fixture UUID casts were corrected without weakening assertions. Down migration explicitly drops the added legacy ownership index so reapplication succeeds.
- T021 authored memory/PostgreSQL storage contracts and core configuration tests before implementations. Configuration tests compiled and failed on missing defaults, relations, and source-presence behavior. Storage suites compiled using isolated `refresh_red` declarations and failed because native coordinated issuance was absent; those runs did not reach later rollback/race assertions. The invalid memory agent fixture was corrected using existing optional permission-set conventions. Both temporary compilation files were removed before backend implementation. Full storage behavior remains subject to the integrated green gate.
- T025 core policy configuration passes `go test ./internal/config ./internal/ports -run '^(TestRefreshPolicy|TestOAuth2AuthServerConfig_)' -count=1`. A disposable `go run` consumer exercised the production loader: requested `--config` path selection, 30s/0s/720h local defaults, YAML zero, environment-over-YAML zero, malformed/numeric/negative/invalid-relation rejection, and proxy isolation. The smoke source was removed. Backend wiring, original-issuance deadlines, and startup warning observation remain outstanding for T025.
- T023's PostgreSQL contracts pass `go test -tags=integration ./internal/adapters/storage/postgres -run '^TestPGRefresh' -count=1`: confirmed rollback, one successor, receipts/cascades, one-connection reads, database time after the agent guard, old-first/new-first mirror fencing, retention, and actual committed/rolled-back rotation/retry acknowledgement faults. An actual SQL hydration smoke first reproduced driver-returned CEST/+7200 timestamps rejecting native model validation. Adapter-boundary UTC normalization preserves instants and deadlines; the deterministic hydration regression also passes. The temporary SQL smoke was removed. The memory contention helper now binds both requests to the same submitted predecessor; its original one-successor assertions remain unchanged.
- T022/T025's integrated memory contracts and race run pass, including a new red-first missing-agent resurrection regression. The memory gate now rejects a missing agent before sampling time or invoking its callback, matching PostgreSQL. A disposable native API consumer ran on both memory and actual PostgreSQL and observed issuance, authorization/token rollback, one rotation with unchanged origin, committed revocation, retention-boundary purge, and independently retained PostgreSQL receipts. The memory consumer also proved a denied scope cannot recreate a missing agent.
- The original-deadline smoke passed for a fixed shared issuance time, finite first-token inactivity expiry, unlimited absolute zero, and a finite absolute deadline. A real builder startup consumer started both unlimited and shorter-finite configurations; only the latter emitted the required warning. All three temporary smoke sources were removed. The expanded migration suite, including the red-first SQL NULL terminal-reason regression, passes against cloned PostgreSQL.
- T022–T025 are complete. `just check` passed formatting, vet, and lint with zero issues; full memory adapter, storage factory, config, and ports package tests passed. Original-issuance deadline initialization moves from T025 to the atomic Fosite storage cutover in T029: the isolated helper's smoke passed, but adding it before its production consumer caused the unused-code lint gate to fail. The helper was removed and will be introduced with that consumer. Requirements and all 47 scenarios remain unchanged; FR-017/FR-019 traceability includes T029.
- T026's typed subject/namespace and real SDK tests compiled and failed before routing changes. The raw-key SDK exposed its subset AAD check: decrypting a refresh-bound result with an empty expected context succeeded. T027 now requires explicit matching authenticated refresh-session subject presence before returning plaintext, rejects malformed/mixed refresh AAD, and preserves SDK signing metadata and existing service/kid behavior. Raw-mode branch registration rejects zero IDs and returns the deterministic namespace.
- All domain subject, branch-key, AWS adapter, and raw manager package tests pass, and `just check` reports zero issues. A disposable `go run` consumer exercised real raw-key and KMS hierarchical SDK operations against the existing AWS emulator: deterministic/idempotent root-key provisioning, same-root recovery, missing/cross/mixed-root rejection, and tamper rejection. The smoke source and emulator were removed; no credentials or ciphertext were printed. KMS supplier and manager reuse their existing typed routing, so no second provisioning mechanism was introduced.
- T028 migrated all verifier/service/model/query callers to explicit shared decision time and required clock injection. Active decisions preserve the actual nonzero grant ID and validity; missing/expired decisions carry no usable grant. `just check` passed, all 13 matching domain/app/HTTP/memory/integration/fixture packages passed, and PostgreSQL active-query/native refresh tests passed. A disposable native consent consumer proved historical shared time overrides the node clock, original evidence is preserved, and equality expires both service and repository decisions. The smoke source was removed. Existing non-authorization permission-set deletion counts retain their prior count-time behavior; the unfiltered principal/agent query remains unfiltered.
- T029 cut over Fosite storage/provider construction to required native facets, owner scopes, original code-UUID roots, active grant evidence, typed encryption, and shared time. Signing/CIMD/key provisioning occur before the gate; the actual signed JWT expiry is recorded at NumericDate second precision without decoding. Fosite transaction hooks borrow the owner. Initial PKCE/code persistence required an explicit sanitation whitelist; the default library whitelist dropped the challenge before storage. Existing issuer/PKCE/signing regressions now pass.
- Actual memory and PostgreSQL HTTP consumers observed native first issuance, a narrowed fresh rotation, and committed strict-zero family revocation. PostgreSQL exposed a scoped readback bug: after this transaction consumed its predecessor, the legacy-current fence rejected rereading that same row. A red-first regression now proves owned readback/rollback. The scope records only successfully consumed native/mirror signatures; other reads retain old-writer fencing. The original old-first/new-first and lost-acknowledgement tests still pass. Actual submitted requested scope is preserved in the owner context before Fosite sanitizes its storage request.
- T030 passed `just check`, all 12 matching model/crypto/storage/issuer/config/app/integration packages, and PostgreSQL refresh/grant/migration suites. The original PostgreSQL US1-S1 primary journey passed with unchanged assertions, proving original grant/start/ceiling and narrow-then-full renewal. The installed Ginkgo CLI version differed from the repository pin; the passing run used the pinned module CLI via `go run`. Focused-out specs are not claimed as executed acceptance scenarios. All temporary owner/HTTP diagnostic sources and containers were removed; no full-feature completion is claimed at this foundation checkpoint.
- T031 added provider/client and consent-renewal behavioral regressions before story implementation. The client authority transition cases passed. `go test ./internal/domain/oauth2server -run '^TestProviderRefresh_' -count=1` reproduced two semantic failures: a permitted narrower response after scope withdrawal still returned `invalid_scope`; a credential replacement committed between preflight and the owner scope returned consent `server_error` instead of earlier `invalid_client`. Other original-grant, deadline, client, revocation, capability, scope-ceiling, and public-promotion controls passed.
- The focused consent run reproduced missing expired/equality renewal revocation, wall-clock rejection of historical shared-time-valid renewals/active edits, acceptance of a deadline equal to shared time, and ignored revocation/commit/clock faults. Tests use stateful domain port doubles for scoped root/grant snapshots; real-backend acceptance remains mandatory. Only two dependency fields were added to compile these red tests; their constructor wiring and behavior are still pending T033. No production behavior changed before this evidence.
- T032 moved current credential/registration reauthentication before revocation/consent/lifetimes inside the owner and repeated that order before commit. Its targeted provider regressions passed, including the previously red credential-race case. Real HTTP acceptance US1-S3/S5/S6/S7 passed (4 scenarios), including PostgreSQL consent relation failure/recovery with unchanged token state and direct published-JWKS verification. A disposable `go run` broker consumer observed missing-consent denial without consumption, replaced-secret denial, and successful current-credential rotation preserving original grant/start/ceiling. The smoke source was removed. Required issuer dependencies were already wired during T029; no second wiring mechanism was added.
- T033 now owns grant creation/update in the agent coordinator. Expired/equality renewal revokes older principal/agent roots before retaining the original grant ID; active edits retain roots and clocks. A grant result escapes only after acknowledged commit. Grant validation and timestamps use supplied decision time, and all constructor/model callers were migrated without aliases. Six matching consent/model/app/HTTP/token-exchange/fixture packages passed; PostgreSQL US1-S8/S9 passed, proving expired renewal and active editing through production HTTP.
- A one-connection regression on the actual PostgreSQL adapter pool reproduced owner-connection starvation in permission-set and active upstream-session validation. Those read paths now use the existing ambient executor (including permission-scope batch loading). The regression and targeted PostgreSQL refresh/grant/permission/session tests passed, and `just check` passed with zero issues. The separate inspection connection is not used as evidence for broker-pool safety.
- A disposable real broker HTTP consumer renewed an expired grant with the same grant ID, observed committed `expired_grant_renewal` on the old root and denied its token, then completed fresh PKCE issuance and rotation. The source was removed. Native renewal/fault tests use stateful port doubles; older isolated unit fixtures use explicit-clock mocks, while HTTP integration fixtures join real memory stores.
- T034 separated Fosite's original-grant protocol scope check from current candidate-response permission checks. A request-local client view exposes only the immutable ceiling to Fosite; current permissions are checked after classification/narrowing before minting and again before commit. No root scope or client registration is rewritten. The previously red withdrawn-scope/narrow-candidate test now passes, along with the full issuer package's capability, client-promotion, ceiling, and error-order tests. `just check` passed with zero issues.
- A disposable actual broker HTTP consumer observed omitted-scope denial after withdrawal without consumption, successful permitted `read` narrowing, unchanged root ceiling/start, and full-ceiling renewal after current permission restoration. Its source was removed. Retry-specific scope/classification behavior remains gated by T043 and is not claimed complete.
- T035 verified and retained the existing domain/transport error mapper: Fosite hints/debug/wrapped causes do not enter the public description, and the JSON response exposes no `error_uri` or token fields on failure. Consent/session/lifetime/reuse use generic `invalid_grant`; infrastructure uses 5xx `server_error`; capability and candidate scope keep their existing distinct contracts. The full issuer and end-user adapter packages passed. The seven ready US1 backend journeys (S1/S3/S5/S6/S7/S8/S9) all passed through production HTTP, including actual unavailable-consent recovery. No redundant mapper or compatibility shim was added; retry-specific acceptance still waits for US3.
- T036's ready core acceptance is green: backend US1-S1/S3/S5/S6/S7/S8/S9 (7 observed journeys) plus serial built-mode Playwright US1-S2 (1 observed journey). The browser actually clicked the existing revoke action, confirmed the dialog, observed disappearance of that action, and received token-free `invalid_grant` for the previously usable refresh token. Maintained captures are `tests/e2e/screenshots/refresh_consent_before_revoke.png` and `refresh_consent_after_revoke.png`; visual inspection showed Revoke All Access before and Approve & Delegate after. No React component/style change was needed. US1-S4's confirmed positive predecessor recovery remains explicitly pending T050/T072, not skipped-as-passing or counted as complete acceptance.
- T037 authored domain lifecycle regressions before implementation. The integrated red run compiled and observed uncoordinated agent deletion, missing all-principal credential revocation/public-fallback protection, ignored credential-read/coordinator faults, credential writes escaping confirmed failures, and missing grant-path/absent-grant revocation. The empty submission still failed validation rather than revoking. Tests assert unchanged authority on rollback, empty secret results on failure, retained replacement origins, independent receipt evidence, and unrelated authority. Agent receipt cases use stateful domain doubles; native credential tests use actual memory scoped stores with injected stage/commit conflicts. Real PostgreSQL receipt-write faults remain covered by the backend and acceptance gates.
- T038 routes both grant revocation entry points and empty grant submission through one owner transaction, revoking matching roots/legacy authority even when the grant is absent. DELETE retains its post-commit 404; empty submission returns idempotent 204. Infrastructure failures are not mistaken for absence. Nonempty grant deadline validation now belongs solely to the shared-clock domain service. The obsolete nil-service HTTP deadline test was replaced by real-service API rejection and historical/equality shared-time coverage, retaining the consumer-visible rejection contract.
- Consent/domain and HTTP adapter packages passed, `just check` passed with zero issues, and correctly matched US2-S1/S2/S9 HTTP journeys passed (3 executed). An initial ID-only focus matched zero journeys and is not evidence; US2 scenario IDs are comments beside descriptively named It blocks. A disposable actual HTTP consumer observed absent-grant leftover revocation, two 204 empty submissions, and rejection of the original token after regrant. Its source was removed. Agent/credential lifecycle remains next; full US2 acceptance is not claimed.
- T039/T040 use required coordinator/revocation ports and one acknowledged boundary for agent deletion and credential actions. Agent deletion records independent receipts before cascades, including direct repository safety paths; scoped deletes require receipt evidence. Credential preparation stays outside the gate, while current agent/credential reads and metadata timestamps are shared-time scoped. Replacement/first generation keep roots; explicit revoke ends them. Every failure returns no replacement secret, including lost acknowledgement. Domain/native fault tests passed with preserved state on confirmed rollback.
- Shared revocation coverage found and reproduced a rootless-legacy gap on both memory and PostgreSQL for grant deletion, expired renewal, and credential revoke. Scoped and agent-wide facets now invalidate every matching unused legacy row without fabricating native origin; unrelated ownership survives. Single-root code replay remains narrow. Seven matching domain/adapter/app/integration packages passed, actual PostgreSQL refresh/agent/credential and migration-036 suites passed, and `just check` reported zero issues.
- Production HTTP US2-S3/S4/S5 passed (3 executed). A disposable actual admin consumer proved replacement keeps the existing session under current credentials, explicit revoke defeats public fallback and later credential creation, and deleted-agent renewal fails while an unrelated agent rotates. Its source was removed. Retry-dependent US2-S8/S10 remain pending US3; no full-story or full-feature completion is claimed.
- T041 preserved the independently owned code-replay path already introduced in T029: the consumed code's original UUID selects the root, another client authenticates as itself, and the replay commits only that root's revocation while returning no new authority. Targeted issuer/storage regressions passed. A disposable actual HTTP consumer proved a foreign consumed-refresh presentation leaves the owner's successor usable, then foreign code replay ends only the original root while another same-agent session still rotates. Its source was removed. No refresh binding or consent gate was added to suppress FR-040; positive predecessor recovery before replay remains the US3-dependent US2-S10 control.
- T042's eight ready US2 journeys (S1–S7 and S9) all passed through production HTTP, including real PostgreSQL two-instance overlapping rotation/revocation and database-write-fault rollback. Race-enabled agent/consent/issuer lifecycle and memory storage-contract tests passed; the memory adapter package had no tests matching that focused race regex and is not counted as race coverage from that run. Independent PostgreSQL receipts/cascades and the standalone authenticated code-replay regressions passed in their targeted gates. S8/S10's identical-result recovery controls remain explicitly pending T050/T072; their exclusion is not a passed full-story result.
- T043's complete bounded red run compiled and reproduced missing identical-pair/canonical-scope recovery, absent authenticated retry persistence, no committed retry-count recovery after lost acknowledgement, incorrect family revocation when only cached access expired, and missing native erasure/reconciliation/readiness/cancellation work. Authenticated payload/AAD mutation cases fail because no cipher is persisted yet, rather than from fake placeholders. Existing strict-zero and older-ancestor replay controls remained protected.
- The first cleanup attempt was fixture-invalid: model validation correctly prohibited attaching ciphertext after a committed rotation. Cleanup-only opaque authenticated results now enter the first fresh-transition Save under a test-owned wrapper; the positive shortened-retry case requires real issuer persistence. Issuance/rotation clocks are controlled rather than slept. An unbounded startup cancellation wait was bounded and the interrupted run is not claimed complete. The final `go test ./internal/domain/oauth2server -run '^TestProviderRefreshRetry_|^TestSessionCleanup_' -count=1 -timeout=60s` completed with acceptance-linked semantic failures. Test denial assertions omit raw token results/ciphertext from failure diagnostics. Only native cleanup fields and a real readiness-state getter were added to compile red tests; no native cleanup behavior was added before this evidence.
- T044–T047's integrated retry wave implements exact purpose/version and immutable origin/ownership/lineage/request/deadline bindings under refresh-session AAD. Fresh Fosite generation records exact token values plus the actual signed NumericDate expiry in owner-local state; the first successor/root Save seals the response atomically, never restores it after commit. Zero reuse persists no ciphertext. Consumed presentations now classify explicitly before Fosite: eligible immediate predecessors recover the original pair, only the committed return count advances, and bound prohibited reuse commits root/legacy revocation. Cached-access expiry alone denies without revocation or stateful cleanup. Current authentication/consent/lifetimes/capability and exact response scopes are rechecked before commit.
- The complete focused issuer retry/cipher/AAD/binding/lost-ack/rollback plus earlier fresh/client/native-storage regression run passed. A disposable actual broker HTTP consumer observed encrypted retry persistence, three canonical-equivalent predecessor returns with identical token strings and unchanged lineage/activity/reuse clocks, then fourth-presentation revocation with atomic ciphertext erasure. Its source was removed. Native deadline cleanup, readiness, auditing, and full cross-story acceptance remain subsequent gates; the existing legacy worker has not yet been replaced.
- T048 emits post-acknowledgement RefreshRotated/RefreshRetryAccepted and safe RefreshRejected events with bound principal/agent/client/non-credential root ID, first-failing-stage reason, trusted request context, and comparison to the original redacted fingerprint. Rejections expose no token/ciphertext/payload or wrapped infrastructure cause. Existing global OTel wiring supplies accepted-retry and rejection counters; only bounded reason labels are used, never identity labels.
- A disposable actual broker consumer and OTel ManualReader observed one committed stored-result authorization and one prohibited-replay rejection, exact counter values, and no identity metric attributes. Verbose production logs showed the three expected events and safe context/root metadata without token strings. Matching audit-context/capability/consent/credential tests passed; `just check` passed with zero issues after the event dispatch was aligned with its staticcheck suggestion. The smoke source was removed.
- T049 replaces legacy token-only deletion with required native maintenance dependencies, synchronous bounded startup reconciliation, shared-clock deadline-driven erasure, and failure/recovery readiness. It clamps effective retry deadlines without altering sealed original expiry, clears zero-reuse results before admission, preserves hash/classification history, and commits each erasure in its agent scope. Local/hybrid /health reports native maintenance health and returns 503 on loss; degraded upstream alone remains 200. Build failure releases already-constructed resources; worker shutdown waits for in-flight writes. Healthy periodic scans do not deliberately flicker readiness.
- All T043 cleanup state/deadline/fault/shortened-zero/cancellation tests passed, as did application, HTTP health-transition, memory, integration, and actual PostgreSQL refresh suites. `just check` passed with zero issues. The old app legacy-deletion call-count test was removed rather than repinned to prohibited history deletion; real native state/readiness tests cover the lifecycle. The formatter gate now ignores absent tracked Go files so this clean cutover can remove obsolete source before the final commit.
- A disposable actual running broker consumer observed encrypted retry-result erasure by fixed deadline plus one second without successful-refresh polling, unchanged lineage/activity metadata, ready /health and native component, and successful current-successor rotation afterward. The source was removed. Canonical end-user health documentation now describes required native cleanup admission and recovery. General lifetime expiry/reconciliation remains T053/T057 except the atomic expiry transition needed when elapsed stored lifetime prevents a required ciphertext-erasure Save.
- T050's ten ready US3 primary journeys (all except lifetime-dependent S7) and US1-S4/US2-S8/US2-S10 passed through production HTTP. The grouped run executed 12 cases; the separately focused authenticated code-replay control executed one. Delayed refresh entropy now occurs after real JWT minting and before fresh consumption, with valid 5s reuse and 6s access TTL. The clock is sampled immediately before consumption so signed access can genuinely expire before grace; the process-global entropy fixture is serial. PostgreSQL's fault relay tracks cached Parse/Bind/Execute writes as well as simple queries, so rotation and count acknowledgements are actually interrupted.
- US3-S11 also passed its expanded confirmed rotation/count rollback, committed acknowledgement loss, unavailable durable resolution, and strict-zero controls. Zero reuse neither recovers a committed pair nor authorizes a retry count; its confirmed rollback leaves the original token usable. A disposable actual broker consumer observed one shared consumption/activity instant, identical stored-pair recovery, and unchanged retry clocks; its source was removed. The native storage/retry package gate passed. Scoped quality analysis reported existing feature-level Fosite complexity and interface-reachability/clone findings, not a clean quality gate; these remain subject to final review rather than being blanket acknowledged.
- T051 authored deterministic runtime and startup regressions before production edits. The corrected runtime run reproduced successful authority after original stored inactivity or issued-current-token expiry during staging; other absolute-boundary/unlimited/policy cases were baseline green. Initial future-clock fixtures were corrected with virtual shared time; their server_error failures were fixture errors, not feature-red evidence. Startup tests reproduced missing no-predecessor reconciliation, stored-expiry terminalization, unexpired absolute recomputation, and shortened-lifetime erasure. The actual PostgreSQL mirror-fault test reproduced admission despite the failed legacy update. T052/T053 implementation follows this evidence.
- T052 preserves the original interval/current-token authority during the final shared-clock recheck, rather than trusting only the staged successor's renewed interval. The new absolute tests and existing error-order/deadline tests pass. The existing staged-expiry fixture now supplies the consumption instant and then the commit instant, retaining all denial/rollback assertions. A disposable actual HTTP broker consumer passed before-deadline rotation and token-free denial after the original absolute deadline, with unchanged origin/deadline; its source was removed.
- T053 reconciles every active root before readiness, including unrotated roots without cached results. It persists shortened inactivity, recomputes still-active absolute policy from the origin, preserves elapsed stored deadlines, and expires through the atomic native/legacy revocation facet. `just check` passed with zero issues, and full issuer, app, and self-contained integration packages passed. The actual PostgreSQL mirror-failure/recovery regression passed after normalizing newly discovered legacy retention timestamps to UTC at the adapter boundary; its timestamp assertion compares exact UTC instants rather than driver locations. A disposable actual broker restart consumer observed no-predecessor expiry before ready health, old-token denial, and unrelated-session renewal. A separate disposable PostgreSQL smoke hosted real HTTP replicas with observed node clocks one minute behind/ahead; both renewed before and denied after the shared original deadline. This setup is being integrated into US4-S4, not counted as another primary scenario.
- T054 executed all four US4 primary journeys successfully. US4-S4 now observes real production HTTP brokers with node clocks one minute behind/ahead while PostgreSQL retains its own real clock; rotation and original deadline persistence survive restart and both skews, and the ahead node and ordinary node reject renewal after the shared deadline. Clock bubbles run sequentially and shut down before ordinary servers because httptest closes shared idle transports; the initial concurrent-bubble teardown failed and is not counted as a passing run. The final four-case run passed. Scoped quality review retained the explicit journey and existing small Close helper rather than extracting unrelated clone matches.
- T055/T056 runtime inactivity coverage was baseline green: 720h/custom issuance, fresh-only renewal, authenticated duplicate recovery, resource access and failures, shorter-policy equality, policy increases, terminal non-resurrection, and retained issued-token history. No redundant runtime rewrite was needed. A disposable actual HTTP restart consumer proved a 4s-to-8s increase preserves old issued expiry/current interval, applies only to a valid fresh successor, still denies an unrotated token at its old deadline, and lets the fresh successor continue beyond it; its source was removed.
- T055 maintenance regressions were semantic red: empty-cipher unrotated roots did not expire in the live worker; retention failures did not block readiness. Actual PostgreSQL tests reproduced admission despite live mirror-write failure and failure to purge NULL-anchor request-ID descendants at retention equality. Initial unit compile/nil-assertion errors were corrected before classifying behavior. T057 follows this red evidence.
- T057 schedules absolute/inactivity expiry even for unrotated empty-cipher roots, commits expiry and legacy invalidation through the existing scoped facet, and purges terminal history in bounded shared-time batches. Retention failure blocks readiness. PostgreSQL purge now atomically deletes old-only NULL-anchor request-ID descendants along with root/native/anchored history; independent receipts and unrelated active families survive. After standard time.Until lint corrections and removal of smoke scaffolding, just check passed with zero issues, full issuer/app/default integration packages passed, and all real PostgreSQL rollout expiry/retention/fault/recovery tests passed. A disposable actual live broker consumer observed empty-cipher expiry by deadline plus one second, legacy invalidation, token denial, and ready health; its source was removed. Scoped quality review retained bounded explicit maintenance over unrelated tiny clone matches, without a blanket quality acknowledgement.
- T058 executed all five US5 primary PostgreSQL journeys successfully: aged 719h/721h default intervals, fresh successor after original interval, direct-JWKS signed resource traffic without renewal, identical duplicate result without activity/reuse-clock changes, and custom inactivity expiry. No successful-refresh polling was used while waiting for idle expiry.
- T059 authored CLI/chart/environment and encrypted restore-command integrations before delivery implementation. Missing dotted CLI flags were semantic red. Complete native lineage tests reproduced admission with missing native intermediate, mismatched intermediate owner, or broken predecessor linkage; rejected lineage must remain non-mutating. After fixing fixture-only ciphertext replacement and preservation-asset construction errors, the real offline restore command was semantic red because it entered normal startup and attempted HTTP binds on reserved ports.
- T060 passed core config/ports and corrected chart/environment real-refresh integrations, including local/proxy/hybrid defaults, explicit zero, duration relations, and warning behavior. Initial Helm fixture errors came from an older helper forcing proxy/upstream and missing the existing local trust-anchor prerequisite; they were corrected using direct real rendering and an in-process mock trust anchor, not security bypasses or policy assertions weakened to pass. T061 adds CLI delivery to this existing working policy.
- T061 registers the three exact dotted string-valued duration flags and binds only explicit CLI changes through the existing loader, preserving zero and validation precedence. Expanded CLI duration/relations/real-refresh tests passed. The actual command completed file, environment-over-YAML, and CLI-over-environment/YAML rows with live issuance, identical retry, idle expiry, and original absolute expiry; no copied-value-only evidence was used. A repeated binary-client fixture permission-set name was made unique after its second creation correctly returned conflict.
- T062 passed actual ConfigMap rendering, production loading, local/proxy/hybrid resolution and real refresh behavior, explicit zero/finite policy, invalid durations/relations, and observed warnings. No chart parameters were added again. Full US6-S1 remains pending its older E2E rendered-chart fixture trust-anchor prerequisite; the three completed binary rows are not reported as a passed primary journey. Offline restore implementation now follows these completed delivery gates.
- T063 implements the one-shot offline restore command through the production loader and maintenance-only builder, with no HTTP servers, issuers, discovery or normal workers. Bounded agent scopes revoke active roots and all legacy authority, preserve terminal reasons and independent receipts, and require acknowledged commits plus the new five-method maintenance facet's authoritative final emptiness check. PostgreSQL continuation now validates every anchored ancestry link, matching native history, ownership and consumption/issuance times; cyclic/broken or incomplete ancestry is rejected without mutation.
- Full issuer/app/memory/ports/CLI packages and PostgreSQL native contracts passed. Restore-command receipt/mirror/real lost-COMMIT faults and idempotent rerun plus complete-lineage regressions passed three successive runs. The lost-ack assertion checks committed owner invalidation independent of whether the first UUID-sorted owner has a root or only legacy rows; a duplicate snapshot current-token fixture entry was corrected. A disposable real PostgreSQL/HTTP consumer ran the actual offline command twice, rejected restored predecessor/current tokens, and completed fresh authorization under preserved credentials; its source was removed.
- The corrected US6-S1 actual binary journey now passed all file/env/CLI/rendered-Helm rows. Its Helm fixture supplies the existing local client-assertion trust anchor through a real in-process mock, matching the already-passing chart integrations. No consumer security validation was disabled. Scoped quality analysis retains explicit bounded operations and reports existing clone/complexity findings rather than claiming a clean global quality gate.
- T064 aligns configuration, Kubernetes deployment, changelog, example README and both local/hybrid YAML comments with implemented behavior. Deployment procedures cover one-second live erasure versus backups, shortened/zero readiness reconciliation, fresh-only inactivity increases, residual access-JWT exposure, mandatory unanchored reauthorization, stopped-writer rollback reconciliation, and offline restore failure/rerun admission. Both example YAML files parsed successfully; the added deployment prose and existing reference target were validated. No release number or shipped-release claim was invented.
- T065 observed passing outcomes for all eight US6 primary journeys: actual delivery sources, fixed duplicate/inactivity clocks, invalid startup combinations, unanchored reauthorization/new-issuer restart, local/upstream hybrid/proxy independence, shortened lifetime/increase/schema-preserving old-binary rollback, old-first/new-first fencing, and volatile memory restart. Seven passed in the integrated run, and corrected S8 passed separately. The test now preserves the existing 401 invalid_client authentication-first contract when the process loses registration; its registered-client control still requires invalid_grant for lost history. An initial relative binary locator was corrected to an absolute path and is not a passed run.
- The original US6-S6 assertions exposed lost effective deadline metadata on immediate startup expiry. Maintenance now rereads the terminalized root and persists shortened session/retry deadlines within the same owner transaction, preserving receipts and atomic legacy invalidation. Native cleanup/absolute/inactivity regressions passed after the fix, and schema-preserving rollback controls passed through the captured checksum-verified old binary. Restore snapshot/fault/rerun integrations remain separate from the 47 primary scenarios and passed before this gate.
- T066 removes the obsolete domain token-only model, port, factory accessor, PostgreSQL repository and memory exported store/wrapper. The native memory coordinator owns a private bridge map and five-argument constructor; all external callers migrated. Private memory and actual PostgreSQL SQL fixtures preserve rootless lifecycle, rollback, restore, retention and receipt coverage without test-only production APIs. Only Fosite's required protocol method names retain RefreshTokenSession terminology. The additive legacy SQL table remains.
- Read-only security review found three reachable defects, all reproduced before fixes: old-first mirror consumption blocked terminal cleanup, a held agent gate preserved overdue ready health, and overlapping unused preflights emitted two rotations instead of one stored-result event. Pure native reads now remain available to maintenance; explicit locked CheckCurrentLineage follows earlier authorization checks and still fences consumption/returns. Known overdue work clears readiness before gate waits, and live operations have a one-second timeout. Audit kind follows the actual owner-scoped branch. The dead consumed branch and obsolete red-phase cipher seeding wrapper/allocation bookkeeping were removed.
- just check passed with zero issues; full issuer/memory/agents/app/default integration packages and actual PostgreSQL native/rollout/restore regressions passed after the cutover/review fixes. A disposable real running PostgreSQL broker consumer observed old-first lineage expiry, token-free denial and restored ready health; its source was removed. Storage/architecture documentation now distinguishes evidence reads from issuance eligibility and records the bounded maintenance proof. Remaining broad refactoring suggestions were not applied merely to reduce metrics; required concrete transaction/facet behavior remains explicit.
- T067 confirms the recorded ADR 038 acceptance, separate T001 API/release/Open Decisions approval, early Helm update, and exactly 47 unique initial classifications. Both canonical OpenAPI documents parsed. Historical design-only validation records remain historical, not claims of current incompleteness.
- T068 verifies fail-closed authentication/consent/lifetime/replay/indeterminate-commit paths, safe auditing, typed exactly-one-subject AAD, offline restore and cleanup readiness. All domain encryption, real SDK/raw AWS adapter, branch-key and complete issuer package tests passed. The three evidence-backed security-review findings are fixed and regression protected; no custom crypto, recovery bypass, plaintext token logs or identity metric labels were added. Related root/domain/ports retrieval context now describes implemented refresh subjects and routes to the canonical API guide.
- T069 verifies typed RefreshSessionID generation/catalogue, domain-native models, focused 3/4/3/5-method repository facets, explicit issuance versus maintenance boundaries, builder-only service construction, and architecture/glossary parity. The obsolete token-only domain/port/accessor/store APIs are absent; required Fosite protocol method names remain.
- T070 validates unchanged tokenExchange operation ID, canonical end-user/admin OpenAPI syntax, approved major-release policy without assigning a release number, implemented API-guide status, and local/hybrid/chart YAML parity. Earlier guide/quickstart design-only pending status was removed. Executed zero-reuse and lost-ack outcomes match the documented examples.
- T071 passed actual PostgreSQL migration-036 apply/down/reapply, native repository/one-connection/race/rollback/lost-ack, complete-ancestry/old-first maintenance, deadline/retention/independent receipt and restored-authority invalidation tests. Port facets remain within seven methods. No missing database infrastructure was substituted with mock/source-text evidence.
- T072 passed all 47 primary scenarios after the final source/read-fence/readiness/audit cutover: the pinned tagged backend run executed 46/46 with zero failures or pending feature cases, and built-mode serial Playwright executed US1-S2 successfully. Unrelated focused-out cases are not counted as executed. This includes US3-S7 and all cross-story controls previously held for lifetime integration, real old/current binaries, one-connection and node-skew controls, native/legacy races, lost acknowledgements, retries and independent receipts. All 47 initial classifications remain recorded separately.
- The actual browser clicked and confirmed Revoke All Access, then rejected the previously usable refresh token without token fields. Maintained before/after screenshots were refreshed and visually inspected: Revoke All Access and Save become Approve & Delegate. No frontend component or styling change was needed. The frontend TypeScript/Vite production build passed. A local ignored pinned Ginkgo 2.33.0 runner avoids the previously observed system CLI/module mismatch for full verification.
- T073's full just verify run first stopped at gosec findings in new feature code. Audit fingerprint length prefixes now use non-narrowing uint64 lengths. Two exact nosec annotations identify a public reason enum and the payload serialized only for immediate encryption. The rerun reported zero gosec issues, with no global rule disabled.
- Full verification remains blocked by govulncheck GO-2026-6443 in the existing google.golang.org/grpc v1.84.0 ExtProc dependency (server panic from missing authority/Host). The latest released module is also v1.84.0; the advisory names a v1.85.0 development revision as fixed. The owner explicitly selected “Keep the existing dependency pin” rather than expanding this feature to a prerelease dependency upgrade. No vulnerability bypass or passing-full-gate claim is permitted. Remaining verification recipes run separately so this blocker does not hide reachable test results.
- T073 passed `just check`, the complete race-enabled fast Go suite, CDK tests, both mock-service suites, and self-contained plus infrastructure-backed integration suites. PostgreSQL adapters and migration apply/down/reapply ran against real containers. The tagged root integration package also passed, including the supported restore command, encrypted snapshots, receipt/mirror faults, lost acknowledgements, and idempotent reruns.
- Frontend tests initially ran no cases because the local install lacked the lockfile's `@testing-library/dom` peer. `npm ci` restored the exact dependency set without manifest changes. All 29 frontend unit-test files then passed, with 256 passing tests. The TypeScript/Vite production build also passed.
- The complete backend E2E gate initially found two obsolete assertions. The strict-use fixture now explicitly selects `0s` and retains its replay-denial assertion. The obsolete empty-grant rejection test was removed; approved complete lifecycle journeys cover empty submission as revocation. The rerun passed 666 cases with zero failures or pending cases. Two performance-labelled cases were excluded by the existing functional gate and are not claimed as executed.
- ExtProc E2E passed all 112 cases. The first full browser run passed 62 of 64 cases and failed two existing navigation waits. Other builds and tests ran concurrently. This record does not establish the cause of those timeouts. The isolated full serial run then passed all 64 cases with zero failures, pending cases, or skips. No browser assertion changed and no automatic retry was added.
- Final scoped `ripwire --quality-delta` reported 147 gating findings; it is not a clean quality-gate result. LSP references show that flagged transaction and cleanup methods remain reachable. Fosite protocol methods remain required interface implementations. Reviewed production growth covers shared-clock validation, transaction ownership, receipt-before-cascade checks, and required lifecycle dependencies. Cross-context clone matches include unrelated constructors and context accessors. These findings were not suppressed or used to justify speculative abstractions. No error-masking finding appeared. The earlier read-only reviews and regression-backed security fixes remain the substantive quality evidence.
- T073 execution and outcome accounting are complete. `just verify` remains unsuccessful because of the owner-retained gRPC vulnerability. The initial parallel browser failures and non-clean quality-delta result remain recorded separately. No passing release gate is claimed.
- T074's disposable actual broker consumer passed lost-result recovery with identical token strings, published-JWKS signature/claim validation, credential replacement, and explicit credential revocation with public-fallback denial. The source and empty smoke directory were removed. The complete browser journey clicked and confirmed Revoke All Access and rejected renewal. Both maintained screenshot files show the expected before/after controls and contain no tokens. The supported restore command passed actual encrypted-snapshot invalidation, failed receipt/mirror admission, lost-COMMIT acknowledgement, and acknowledged idempotent reruns in the tagged integration package. Fresh authorization and unaffected credentials, grants, signing keys, and upstream sessions remained covered.
- T075 finalized implemented status, canonical API/guide alignment, and release evidence without assigning a version. Both OpenAPI documents parsed with unique operation IDs and valid local references; `tokenExchange` remains unchanged. Five delivery documents rendered with ten tables and resolved links/heading anchors. All 89 requirements map to the 75 tasks. All 47 primary scenario files match their 39 semantic-red and eight baseline-green initial records. The task dependency graph has no cycles. The final small contract/document edit quality check had zero gating findings and one minor two-line growth finding; it does not replace the broader non-clean assessment. All specified feature work is implemented. Release blockers remain explicit, and no temporary runtime scaffold remains.
