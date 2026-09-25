# Implementation Plan: ExtProc Approval Cache Sync & OPA Integration

**Branch**: `026-extproc-approval-sync` | **Date**: 2026-09-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `/specs/026-extproc-approval-sync/spec.md`

## Summary

Close the Tier-2 authorization loop between ExtProc's OPA engine (feature 020) and the broker's tool
approval service (feature 024). ExtProc stops collapsing OPA's `approval_required` action to deny and
instead: matches the invocation against a locally-synced approval cache using the shared
`internal/toolpattern` matcher (ADR 035); on a match-miss performs an authoritative
`GET /api/approvals?principal=…` read; on a confirmed miss creates a pending approval via
`POST /api/approvals` and returns an MCP `URLElicitationRequiredError` (`-32042`) carrying the broker's
`approval_url`; consumes `once` approvals synchronously before forwarding; and keeps the cache current
through one background ETag long-poll goroutine. `ciba_required` remains deny (deferred). A `deny`
never reaches the creation path, so no user is ever prompted to approve a tool they cannot invoke.

The design reuses existing machinery throughout: the ADR 012 cache pattern
(`internal/extproc/server/exchanger.go`), the shipped MCP elicitation builder
(`server.go:1123-1156`), the shared glob matcher (`internal/toolpattern`), the ADR 014 long-poll
contract, and the existing `tests/e2e/extproc/` Ginkgo harness. One new ExtProc package,
`internal/extproc/approval/`, owns the cache, the broker client, the syncer, and the per-request gate.
See [research.md](research.md) for the grounded decisions and [data-model.md](data-model.md) for
entities.

**Two broker changes are in scope.** Both are additive, optional-valued fields on responses ExtProc
already consumes, specified as spec FR-017 / FR-018 and
[contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml). Each projects a value the
broker has already computed, so neither adds extraction logic, configuration, storage, or a migration.

1. **Token-exchange response identity** (`principal`, `agent_id`). ExtProc cannot key an approval
   cache by `(principal, agent_id)` because it has no verified way to learn either value: it holds
   only the opaque caller bearer token, has no JWKS client, and has no claim-extraction configuration.
   Parsing the caller's subject token without verifying its signature would let a forged claim select
   another user's cached approval — a direct Constitution Principle I violation. The broker returns the
   identity it **already derived from the verified subject token**: `principal` from
   `celEvaluator.ExtractPrincipal` (`internal/domain/tokenexchange/service.go:188`) and the resolved
   canonical `agent.ID` (`service.go:245-256`), both in scope at response assembly
   (`service.go:391-411`), mirroring feature 020's `granted_permission_sets` extension.
   ([research.md](research.md) § R-000)
2. **Approval sync summary decision time** (`approved_at`). FR-015 requires equal-specificity ties to
   be broken by later decision time, and `toolpattern.SelectBest` implements exactly that — but the
   sync summary carries no timestamp at all, so every candidate would rank with a zero `DecidedAt` and
   ties would silently fall through to the UUID tie-breaker, picking the wrong winner with no error.
   `ToolApproval.ApprovedAt` already exists (column present since migration 024); this projects it
   through `toSummary`. ([research.md](research.md) § R-014)

**Principle X confirmation**: recorded in the spec's 2026-09-04 Clarifications session — the
stakeholder directed that both broker changes be implemented with this feature.

## Technical Context

**Language/Version**: Go 1.26.8
**Primary Dependencies**: `envoyproxy/go-control-plane` (ExtProc gRPC), `open-policy-agent/opa/v1`
(embedded, ADR 028), `mark3labs/mcp-go` (elicitation error), `internal/toolpattern` (ADR 035),
Viper/Cobra (ExtProc config), `otelhttp` (outbound tracing), stdlib `net/http`
**Storage**: None. No new persistent entity, no migration. The broker's `tool_approvals` table and
migrations 024–026 + 030 already exist and are unchanged.
**Testing**: stdlib `testing` + testify with `-race` for `internal/extproc/**`; Ginkgo/Gomega for
`tests/e2e/extproc/`; `httptest` broker fakes plus a composed production broker for sync semantics
**Target Platform**: Linux container (`Dockerfile.extproc`), deployed alongside agentgateway
**Project Type**: Standalone Go gRPC service (ExtProc); no frontend, no broker UI change
**Performance Goals**: SC-001 gate processing <500 ms measured independently of a controllable broker-client duration; SC-002 approval propagation <2 s; SC-004 zero broker round-trips on a cached permanent match; SC-005 non-blocking bootstrap request deadline ≤5 s; SC-007 long-poll reconnect <5 s
**Constraints**: Fail closed on every authorization error, timeout, malformed broker response, and undefined decision; a failed authoritative read is never a confirmed miss and MUST NOT create an approval; a `once` approval forwards only after a confirmed `200` consume; no approval records in the OPA input; no unverified identity may key the cache; `internal/extproc/*` may import no broker package except `internal/toolpattern`
**Scale/Scope**: ExtProc-only except two additive broker response field sets (`principal`/`agent_id`
on the token-exchange response; `approved_at` on the approval sync summary); 1 new ExtProc package,
2 new config sections, 1 new OPA context field, 1 changed decision action, 1 new E2E test file plus
one new bootstrap helper and one new Rego fixture

## Constitution Check

*Constitution checks are revalidated after each design phase. Re-validated post-Phase 1 — result
unchanged.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities identified in [data-model.md](data-model.md) — `ApprovalCache`,
  `pairKey`, `ApprovalRecord`, `SessionRegistry`, `ApprovalIdentity`, `Invocation`, `GateOutcome`.
  All in-memory; no persistent aggregate is introduced.
- [x] **Domain Concepts**: New terms (Approval Cache, Long-Poll Syncer, Approval Identity, Approval
  Gate) will be added to the `ARCHITECTURE.md` Glossary during implementation. **ToolApproval** is
  already present.
- [x] **Entity IDs**: N/A — no new UUID-backed persistent entity. ExtProc holds broker approval ids as
  opaque strings and cannot import `internal/domain/id` (ADR 035 permits only `internal/toolpattern`).
  ADR 013 applies to broker domain entities with UUID primary keys.
- [x] **Configuration Design**: Both new sections specified with a working YAML example, defaults, env
  mappings, and seven startup validation rules —
  [contracts/extproc-approval-config.yaml](contracts/extproc-approval-config.yaml).
- [x] **Config Examples and Documentation**: `examples/config/extproc-tool-approvals.yaml` will be added and referenced from `examples/config/README.md`; `docs/configuration.md` will document both new sections and their source precedence.
- [x] **Helm Chart**: **N/A — documented standalone boundary.** `charts/agentic-identity-broker/` deploys only the broker and mounts its ConfigMap as the broker's `/app/config.yaml`; it MUST NOT carry ExtProc configuration. Constitution Principle VII (v2.0.0) scopes Helm changes to workloads the chart deploys. ADR 011 and `ARCHITECTURE.md` record that ExtProc receives `EXTPROC_*` configuration through its independently owned deployment artifact.
- [x] **API Design First**: Both API changes are specified before implementation —
  [contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml). The three approval endpoints
  are otherwise consumed as-is; ExtProc's exact usage is pinned in
  [contracts/broker-approval-client.md](contracts/broker-approval-client.md).
- [x] **API Documentation**: `api/enduser/openapi.yaml` gains two optional properties on the token-exchange response and one on `ToolApprovalSummary`; `docs/api/approval-endpoints.md` documents the additions with representative response examples, and `docs/guides/token-exchange-gateway.md` documents the approval flow.
- [x] **API Changes**: **CONFIRMED.** Both changes (`principal`/`agent_id` on the token-exchange
  response; `approved_at` on the approval sync summary) are additive and optional. Principle X
  confirmation is documented in the spec's 2026-09-04 Clarifications session, where the stakeholder
  directed that the broker changes be implemented with this feature; they are now binding spec
  requirements FR-017 and FR-018 with acceptance scenarios under User Story 5. The spec Assumption
  that no broker-side work is required has been corrected accordingly.
- [x] **Database Design**: N/A — no schema changes. Migrations 024, 025, 026, and 030 already supply
  everything the broker side needs.
- [x] **E2E Acceptance Tests**: `tests/e2e/extproc/approval_sync_test.go` — 1:1 with spec scenarios,
  written first (red).
- [x] **E2E Test Mapping**: Each acceptance scenario and edge case maps to one `It()` — see Testing
  Strategy.
- [x] **E2E Red Phase**: Tests compile with concrete assertions (HTTP status, JSON-RPC `-32042`,
  elicitation URL, broker call counts, cache contents) and fail semantically before implementation.
- [x] **Frontend Playwright E2E**: N/A — no React UI change. Approval UI is feature 024 territory and
  is explicitly out of scope.
- [x] **Frontend Screenshots**: N/A — no UI change.

**Implementation Considerations**:

- [x] **Security-First**: Approval gating is off by default (`tool_approvals.enabled: false`) and
  enabling it is explicit configuration — the control being enabled is the *restriction*, so
  default-off preserves today's fail-closed deny for `approval_required`. Every failure path denies:
  evaluation errors, undefined decisions, `ciba_required`, failed create, failed consume, and missing
  approval identity. **No unverified input ever keys the cache** (research R-000) — the identity comes
  from the broker's verified exchange. A `once` approval forwards only after a confirmed `200` consume.
- [x] **Architecture Docs**: `ARCHITECTURE.md` updated with the new ExtProc package, the approval flow,
  and the glossary terms.
- [x] **Configuration Deployment Governance**: Constitution Principle VII (v2.0.0), ADR 011, and `ARCHITECTURE.md` explicitly scope the broker Helm chart to broker workloads and prohibit inserting ExtProc settings into its ConfigMap or Deployment. ExtProc remains independently deployed with its documented `EXTPROC_` configuration path.
- [x] **Library-First Security**: No cryptography is added. ExtProc performs no JWT parsing and no
  signature verification — deliberately, because it has no trust anchor for either token.
- [x] **Zalando Guidelines**: Both API changes are additive, optional, snake_case, and consistent with
  the properties they sit beside (`granted_permission_sets`; the rest of `ToolApprovalSummary`).
- [x] **End-User Docs**: `docs/guides/token-exchange-gateway.md` documents the ExtProc approval flow and configuration; `docs/api/approval-endpoints.md` documents the additive API fields with response examples.
- [x] **Migration Testing**: N/A — no migrations.
- [x] **Hexagonal Architecture**: `internal/extproc/approval` declares the interfaces it consumes
  (`AssertionProvider`, `BrokerClient`); `server` imports `approval`, never the reverse. ExtProc's
  isolation invariant holds: the only broker import is `internal/toolpattern` under ADR 035.
  Composition stays in `cmd/extproc-token-exchange/root.go`.
- [x] **Persistence Patterns**: N/A — stateless with respect to durable storage.

**Result**: **PASS.** Every design precondition is satisfied, including the standalone configuration-delivery boundary: ExtProc settings are documented and independently deployed under ADR 011, while the broker Helm chart remains limited to the broker workload it actually deploys. The two broker extensions are stakeholder-confirmed, spec-bound (FR-017, FR-018), and contract-specified ([contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml)).

## Project Structure

### Documentation (this feature)

```text
specs/026-extproc-approval-sync/
├── plan.md                  # This file
├── research.md              # Phase 0 output
├── data-model.md            # Phase 1 output
├── quickstart.md            # Phase 1 output
├── contracts/
│   ├── extproc-approval-config.yaml     # Config schema, defaults, validation rules
│   ├── broker-approval-client.md        # Exact ExtProc → broker HTTP contract
│   ├── opa-input-approval.md            # OPA input additions + decision action contract
│   └── broker-api-changes.yaml          # BROKER API CHANGES — identity + approved_at
├── checklists/
│   └── requirements.md      # Pre-existing
└── tasks.md                 # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/extproc/
├── approval/                       # NEW package
│   ├── cache.go                    # pairKey map + RWMutex, toolpattern matching, CAS reservation, idle/session eviction (FR-008/009/010/015)
│   ├── client.go                   # Broker HTTP client: sync, targeted read, create, consume (FR-003..008, FR-011)
│   ├── syncer.go                   # Long-poll goroutine: ETag, 304 loop, 1s→60s backoff, active-session query params (FR-004)
│   ├── gate.go                     # Per-request orchestration: match → authoritative read → create → elicit; once-consume (FR-002/005/006/007)
│   ├── errors.go                   # Deny reasons and error taxonomy
│   └── *_test.go
├── authorization/
│   ├── decision.go                 # Action constants; approval_required no longer collapsed to deny (FR-002)
│   ├── input.go                    # ContextInput.AgentSessionID (FR-013)
│   ├── input_builder.go            # BuildOPAInput takes ContextInput; agent-session header extraction
│   └── authorizer.go               # unsupported_action logging narrowed to ciba_required
├── config/
│   ├── config.go                   # ToolApprovalsConfig + SessionsConfig on the root
│   ├── loader.go                   # Defaults, EXTPROC_ env keys, Cobra flags
│   └── validate.go                 # Validation rules V1–V7
└── server/
    ├── server.go                   # approval_required branch; batch deny-wins reason; parameterized elicitation with the real JSON-RPC id
    └── exchanger.go                # tokenExchangeResponse/ExchangeResult identity fields; ClientAssertion() accessor

cmd/extproc-token-exchange/root.go  # Wire cache + client + syncer + gate; bootstrap; ordered shutdown

internal/domain/tokenexchange/                        # BROKER change 1: surface principal + agent_id
internal/adapters/http/handlers/approval/sync_handler.go  # BROKER change 2: project ApprovedAt into the summary
api/enduser/openapi.yaml                              # BROKER: token-exchange response + ToolApprovalSummary schemas
tests/e2e/helpers/approval_types.go                   # BROKER: ApprovedAt on the shared ApprovalSummary wire type

examples/config/extproc-tool-approvals.yaml   # Working example (Principle VII)
examples/config/README.md                     # Reference the new example
docs/guides/token-exchange-gateway.md         # Approval flow + config documentation
ARCHITECTURE.md                               # Package, flow, glossary

tests/e2e/extproc/
├── approval_sync_test.go                     # E2E acceptance suite (Principle XIII)
├── bootstrap/broker.go                       # NEW: composed production broker + broker-shaped httptest fake
└── fixtures/policies/approval_required.rego  # NEW: policy fixture emitting approval_required
```

**Structure Decision**: Standalone-service layout. Approval sync becomes its own ExtProc package rather
than an extension of `server` or `authorization`. It has a distinct lifecycle (a background goroutine),
a distinct outbound protocol (three REST endpoints with three different auth combinations), and
distinct state (a cache) — none of which belongs in `server.go`, already 1240 lines and owner of the
Envoy stream. Putting it in `authorization` would couple OPA evaluation to broker I/O, which spec
FR-001 explicitly forbids. The `server` package gains exactly one optional field and one decision
branch; `nil` gate means the feature is off and behavior is byte-for-byte unchanged.

## Implementation Phase Overview

| Phase | Purpose | Included? |
|-------|---------|-----------|
| **Phase 0** | Pre-implementation refactoring | **Include (small)** — two behavior-preserving refactors land first: parameterize `urlElicitationResponse` to take a URL/message/id instead of `*BrokerExchangeError`, and change `BuildOPAInput`'s trailing argument to `ContextInput`. Both touch existing call sites and tests with no behavior change; isolating them keeps the feature diff readable. |
| **Phase 1** | Setup — dependencies | **Skip** — every dependency is already present (`toolpattern`, `mcp-go`, `otelhttp`, stdlib `net/http`). |
| **Phase 2** | Design Preconditions (domain model, config, API, E2E red tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate | **Skip** — no persisted entity, no repository, no CRUD handler. |
| **Phase 2.5** | Foundational Infrastructure — **broker changes first** (FR-017 identity, FR-018 `approved_at`, OpenAPI, shared wire type), then ExtProc config schema + validation, identity plumbing, action constants, broker client, cache, syncer | Include |
| **Phase 3+** | User Stories P1→P2 (US5 broker fields, US1 retry loop, US2 deny suppression, US3 bootstrap, US4 long-poll) | Include |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **include** — two isolated, behavior-preserving signature changes
- [x] Phase 2.7 (entity boilerplate): **skip** — no persisted entity or CRUD handler

**Sequencing constraint**: the two broker changes lead Phase 2.5 and are the feature's critical path —
the identity-dependent paths (cache lookup, targeted read) and the FR-015 decision-time tie-break
cannot be built or meaningfully tested until they land. They are also the smallest and least risky
work in the feature: two field additions, two assignments, two schema updates, no migration and no new
configuration. Land them first, then build outward. Phases 0 and 2 are independent of them and may
proceed in parallel.

**Broker work breakdown** (Phase 2.5, ~6 edit sites):

| Change | Site | Edit |
|---|---|---|
| FR-017 | `internal/domain/tokenexchange/response.go:63` | Add `Principal` + `AgentID` to `TokenExchangeResponse`, both `omitempty` |
| FR-017 | `internal/domain/tokenexchange/service.go:391-411` | Assign `principal` (from `:188`) and `agent.ID.String()` (resolved at `:245-256`) — **not** the raw CEL-extracted `agentID` string, so the value matches the canonical UUID the sync response carries |
| FR-017 | `api/enduser/openapi.yaml` | Token-exchange response schema |
| FR-018 | `internal/adapters/http/handlers/approval/sync_handler.go:40-73` | Add `ApprovedAt *time.Time` to `toolApprovalSummary`; project `a.ApprovedAt` in `toSummary` |
| FR-018 | `api/enduser/openapi.yaml:3733-3756` | `ToolApprovalSummary` schema |
| Both | `tests/e2e/helpers/approval_types.go` | Phase 2 task T017 adds `ApprovedAt` to the shared `ApprovalSummary` wire type before the broker changes are implemented |

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/extproc/approval_sync_test.go` for US1–US4 and the edge cases;
`tests/e2e/token_exchange_test.go` and `tests/e2e/approval_api_test.go` (existing broker suites) for
US5 S1–S4, since those assert broker responses and belong with the endpoints they cover. US5 S5 is an
ExtProc precedence assertion and lives with the ExtProc suite.

**Framework**: Ginkgo/Gomega in the existing ExtProc suite
(`tests/e2e/extproc/extproc_suite_test.go`), run by `just test-e2e-extproc`. A new suite is **not**
created — one already exists and fragmenting it would split ExtProc coverage across two runners.

**Test Organization**:
- Top-level `Describe`: "ExtProc Approval Cache Sync"
- Nested `Context` per user story and precondition (cache hit vs miss; signed broker vs unreachable
  broker; standalone vs batch; per persistence mode)
- One `It()` per spec acceptance scenario and per edge case

**Scenario Mapping** (populate exact line numbers during Phase 2 test authoring):

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1 S1 | `approval_sync_test.go` | `It("creates a pending approval with dual auth and returns -32042 with the broker approval_url", …)` |
| US1 S2 | `approval_sync_test.go` | `It("resolves a freshly approved record via the targeted principal read without a second create", …)` |
| US1 S3 | `approval_sync_test.go` | `It("forwards from a synced session approval with no broker approval call", …)` |
| US1 S4 | `approval_sync_test.go` | `It("forwards from a permanent approval in a different session", …)` |
| US1 S5 | `approval_sync_test.go` | `It("consumes a once approval synchronously and elicits again on the next call", …)` |
| US1 S6 | `approval_sync_test.go` | `It("evicts session approvals when the agent session goes idle", …)` |
| US2 S1–S3 | `approval_sync_test.go` | deny creates nothing; allow forwards; approval_required reaches Tier 2 |
| US3 S1–S4 | `approval_sync_test.go` | pre-seeded permanent served from bootstrap; unreachable-broker startup + warning; bootstrap ETag reused on first poll; unknown pair augmented before create |
| US4 S1–S4 | `approval_sync_test.go` | 304 re-poll; approval delivered <2s; reconnect <5s replaying the ETag; unknown ETag returns full state and the cache **replaces** (revoked record disappears) |
| Edge cases | `approval_sync_test.go` | broker-read failure denies without create; stale-cache duplicate creation reuses the existing URL; create failure never fabricates a URL; invalid subject-token create is denied; idle re-augmentation; concurrent same-instance once CAS; multi-replica idempotent consume; batch deny-wins; `ciba_required` deny; undefined decision warning; missing approval identity denies |
| US5 S1 | `tests/e2e/token_exchange_test.go` | `It("returns principal and the resolved canonical agent_id on a successful exchange", …)` |
| US5 S2 | `tests/e2e/token_exchange_test.go` | `It("omits principal/agent_id rather than emitting empty values when unresolved", …)` |
| US5 S3 | `tests/e2e/approval_api_test.go` | `It("returns approved_at on an approved record in the sync summary", …)` |
| US5 S4 | `tests/e2e/approval_api_test.go` | `It("omits approved_at for pending and denied records", …)` |
| US5 S5 | `approval_sync_test.go` | `It("selects the later-approved record when two equally specific approvals tie", …)` |

**Red Phase Requirements**: tests compile with concrete assertions — HTTP status, decoded JSON-RPC
error code `-32042`, `data.elicitations[0].url` and `mode`, the JSON-RPC `id`, broker call counts per
endpoint, request headers (`Authorization`, `X-Client-Assertion`, `If-None-Match`,
`X-Long-Poll-Timeout`), query parameters, and post-sync cache contents — and fail semantically. No
`XIt`/`PIt`/`Skip()`; no red-phase comments.

All mapped E2E tests, including the broker response scenarios and every edge case, are written in Phase 2 before production implementation begins. User-story phases implement against those semantically failing assertions and do not add acceptance-test behavior.

**Test Data Strategy**: Two layers, per [research.md](research.md) § R-012.

1. **Broker-shaped `httptest` fake** in `tests/e2e/extproc/bootstrap/broker.go` for deterministic
   control of error paths, backoff, consume failures, ETag sequences, and per-endpoint call counting.
   Pattern copied from `internal/extproc/server/exchanger_test.go:42-160` (`mockServers` +
   `configForMocks`), with synchronized access because the syncer polls concurrently.
2. **Composed production broker** for US3/US4 and SC-002: a bootstrap helper that builds the real
   end-user server via `tests/e2e/bootstrap` (`NewStorageFactory` → `BuildApp` →
   `NewEndUserTestServer`) with memory storage, and points `tool_approvals.url` at it. This exercises
   the real `SyncHandler`, real `ApprovalSyncBroadcaster`, and the real pending-dedup index. Test
   packages are not bound by ExtProc's production import isolation, so this composition is legal.

Signed machine credentials reuse `helpers.NewApprovalRequestAuthFixture`,
`helpers.ApprovalCreateHeaders`, `helpers.ApprovalSyncHeaders`, and
`helpers.ApprovalSubjectTokenHeaders`; wire types reuse `tests/e2e/helpers/approval_types.go`, whose
`ApprovalSummary` already carries `ToolPattern`, `ParamsPattern`, `Persistence`, `Consumed`, and
`AgentSessionID`. The Phase 2 red test task extends it with `ApprovedAt`, so broker E2E and ExtProc
tests continue to share one wire type and a projection regression fails both suites.

**New fixture**: `tests/e2e/extproc/fixtures/policies/approval_required.rego`. The existing policy
fixtures (`allow_readonly`, `allow_all`, `deny_all`, `assert_us2_input_shapes`) contain none that emits
`approval_required`. It follows the established `policyPath(name)` convention.

**Bootstrap Strategy**: `NewTestEnvironment` / `StartWithAuthorizer` for the in-process ExtProc gRPC
server; `NewRequestHeaders(...).BuildWithMetadata()` + `BuildRequestBody` + `SendHeadersAndBody` to
drive a `tools/call` over a single stream. One agentgateway Docker spec validates that a real MCP
client receives a conformant `-32042` with the correct JSON-RPC id.

### Unit & Integration Tests

**Unit** (`internal/extproc/approval/*_test.go`, `package approval_test`, run with `-race`):
cache matching and precedence driven through `toolpattern.MatchVectors()` /
`PrecedenceVectors()` so broker/ExtProc semantics cannot drift; the FR-015 decision-time tie-break
asserted with ids chosen so the lexicographically smaller id belongs to the *older* approval (a
zero-timestamp regression must fail, not pass by coincidence); fail-closed rejection of an `approved`
record delivered without `approved_at`; persistence-scope filtering
(`permanent` / `session` / `once`); CAS reservation under concurrency including the
two-goroutines-one-record case; reservation release on consume failure; idle and session eviction;
full-snapshot replace semantics (a revoked record must disappear); syncer ETag/304/backoff state
machine against an `httptest` fake; broker client auth-header placement per endpoint, query-parameter
construction, and status-code taxonomy; gate orchestration including the fail-closed missing-identity
path.

**Unit (broker side)**: `internal/domain/tokenexchange/*_test.go` asserts that a successful exchange
response carries the same `principal` the service extracted and the **resolved canonical**
`agent.ID.String()` — not the raw CEL-extracted `agentID` string — and that each field is omitted
rather than emitted empty when unresolved. `internal/adapters/http/handlers/approval/*_test.go`
asserts `toSummary` projects `ApprovedAt` for approved records and omits it for pending and denied
ones. Both are small table-driven additions to existing test files; no new broker test file is needed.

**Unit** (`internal/extproc/authorization/*_test.go`): `ParseDecision` returns
`ActionApprovalRequired` verbatim while `ciba_required`, undefined, and unknown actions stay deny;
`ContextInput.AgentSessionID` emission and its independence from `input.mcp.session_id`.
The two superseded tests (`TestParseDecision_ApprovalRequired_MappedToDeny`,
`TestOPAAuthorizer_ApprovalRequired_LogsRawActionAndReturnsDeny`) are rewritten, retaining their
`ciba_required` coverage.

**Unit** (`internal/extproc/server/*_test.go`): the `approval_required` branch via the existing
`mockExchanger` / `mockAuthorizer` seams (`server_test.go:84-95, 616-628`); batch deny-wins with the
standalone-retry reason; elicitation emits the real JSON-RPC id.

**Config** (`internal/extproc/config/*_test.go`): table-driven coverage of validation rules V1–V7 with
field-path assertions; env/flag precedence for both new sections; the disabled-by-default regression
that an absent `tool_approvals` section changes nothing.

**Integration**: none added. This feature introduces no repository, no migration, and no PostgreSQL
interaction. Broker-side approval storage is already covered by
`tests/integration/migrations/migrations_test.go:232-291` (migration 030) and
`tests/e2e/approval_api_test.go`.

**Test Coverage Goals**: critical paths (matching, reservation/consume, sync state machine, fail-closed
branches) fully covered under `-race`; E2E covers 100% of acceptance scenarios (Principle XIII).

## Complexity Tracking

*No entries.* This feature introduces no constitutional deviation. The two broker extensions — the
token-exchange identity fields and the approval-sync `approved_at` projection — are stakeholder-
confirmed additive API fields (Principle X), not principle violations. The first exists precisely to
**avoid** the Principle I violation that parsing an unverified token would constitute; the second
exists to make FR-015's precedence rule implementable at all rather than silently wrong. Neither adds
a migration, configuration, storage, or new domain logic.
