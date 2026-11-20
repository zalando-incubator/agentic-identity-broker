# Tasks: Agent Tags

**Input**: [spec.md](spec.md), [plan.md](plan.md), [contract proposal](contracts/agent-tags.md)
**Status**: Draft design only. Runtime implementation has not started; approval and semantic-red gates remain open.

🤖 Prepared by Repo Assist, an automated AI assistant.

## 🔒 Feature Scope [MANDATORY]

Only literal Agent tags maintained through existing administrative API operations and displayed in the existing end-user overview/detail views are in scope. All filtering, admin catalogue/editing UI, policy/CEL/token changes, and unrelated cleanup are excluded. The maintainer's [scope clarification](https://github.com/zalando-incubator/agentic-identity-broker/issues/427#issuecomment-6878251111) does not approve the detailed API proposal. No task below authorizes a deviation from a merged Accepted ADR or an unrelated change.

## Phase 1: Setup (Shared Infrastructure)

- [x] T001 Read #427 and the maintainer's two scope decisions; check for an existing open tagging/#427 PR before drafting. Record confirmed scope separately from proposed semantics in `spec.md` (Principles IV/X).
- [x] T002 Review current Agent validation/copy, reconciliation policy/equality, storage read paths, admin/consent projections, frontend transforms, card comparator, and contracts; document source-led change map in `plan.md` (Principles II/VI).
- [x] T003 Author the draft spec, concrete journeys, rendered editable Wireloom artifacts, API proposal, plan and tasks under `specs/056-agent-tags/`, without modifying runtime code or unrelated feature selection (Principles II/XIV).
- [ ] T004 Before implementation, explicitly select `SPECIFY_FEATURE_DIRECTORY=specs/056-agent-tags`; read routed AGENTS.md files, tests/e2e guides, storage guidance, both coding standards and applicable accepted ADRs. Recheck manifests/current branches and record detailed contract approval link here (Principles II/X/XV).

## 🔒 Phase 2: Design Preconditions (Blocking Prerequisites) [MANDATORY]

**Purpose**: Complete domain, configuration, API, database, concrete spec-journey, and E2E design before implementation.

**Gate**: No code implementation can begin until this entire phase is complete. Checked drafting tasks do not constitute acceptance of the proposed design.

### Phase 2a: Domain Model & Glossary [MANDATORY]

- [ ] T005 Confirm the Agent-owned literal set model, exact equality, proposed bounds and omission/replacement policy in `spec.md`/`contracts/agent-tags.md`; add Agent Tag to `ARCHITECTURE.md` glossary and document its non-authorizing meaning, performance limits and copy/consistency rules (FR-001–FR-005; Principles II/V).

### Phase 2b: Configuration Design [MANDATORY]

- [ ] T006 Verify no new config/env/CLI/Helm or standalone deployment settings are needed. Record no-change disposition instead of adding empty examples to `examples/config/` or irrelevant chart values; preserve existing cache/auth settings (Principle VII).

### Phase 2c: API Design [MANDATORY]

- [ ] T007 Obtain written approval of limits, literal equality, duplicate/null rejection, omission-as-empty, response presence/sorting, errors and additive version proposal; link approval and apply it to `api/admin/openapi.yaml` and `api/enduser/openapi.yaml` before production changes. Keep existing wire names/envelopes and ADR 047 field/race/timestamp behavior (FR-002–FR-004; Principles IV/X).
- [ ] T008 Prepare `docs/api/` end-user examples and an administrative management-guide warning that legacy full-definition writes omitting tags clear them; verify no filtering/editor capability is promised (FR-006; Principles IV/X/XIV).

### Phase 2d: Database Design [MANDATORY]

- [ ] T009 Finalize `agents.tags` array column/default/shape/cardinality/null-element constraints, select the next free migration number and author atomic `NNN_agent_tags.up.sql`/`.down.sql`. Document rollout order and intentional tag loss on DOWN without loss of pre-feature data (DB-001–DB-004; Principle IX).

### Phase 2e: Frontend/Design System Review [MANDATORY IF FRONTEND]

- [ ] T010 Review current design-system INDEX, DECISION_TREES, COMPONENT_PAIRING_GUIDE, COMMON_MISTAKES and DESIGN_PRINCIPLES with actual Badge/Stack/Card APIs. Confirm an application-specific labelled noninteractive tag list, semantic neutral styling, long-text wrapping and empty omission; no new universal primitive/visual direction (AT-2.1/2.2/2.4; Principle XI).

### Phase 2f: E2E Acceptance Test Design [MANDATORY]

- [ ] T011 Write `tests/e2e/agent_tags_test.go` with exactly one referenced Ginkgo It for each of the ten `AT-*` scenarios in `spec.md`; use production dual-server bootstrap, fixtures, isolated state, hierarchy and complete write/read/delegation journeys. Instrument existing test seams for replay writes/audits, never add test-only production APIs (SC-001–SC-005; Principles VIII/XIII).
- [ ] T012 Add Playwright journeys in `tests/e2e/frontend/agent_tags_test.go`, using existing harness/page objects, proving overview → detail, empty states, tag-only reload, escaped/maximum strings, light/dark/narrow layouts and existing keyboard/consent actions; configure descriptive screenshots under `tests/e2e/screenshots/` (AT-2.1–AT-2.4; Principles XI/XIII).
- [ ] T013 Compile and run locally runnable acceptance suites to record meaningful semantic-red failures before production changes; container-backed tests may obtain evidence from CI on the implementation commit. No pending/Skip/always-fail assertions, no red-phase annotations in test files, and no counting unavailable infrastructure as semantic red (Principles VIII/XIII).
- [ ] T014 Review both concrete journeys, all HTTP shapes and Wireloom source/SVGs against the approved contract and scenario map. Confirm config/JWT examples are inapplicable and the UI has no tag controls; retain one-to-one scenario mapping while browser tests supplement rendering assertions (Principles XIII/XIV).

### Phase 2g: Observability Design [MANDATORY IF TELEMETRY]

No new log event, span, attribute, or metric is proposed.

- [ ] T015 Review `coding-standards/LOGGING.md` and `OPENTELEMETRY.md` against the actual affected paths. Confirm tags are not emitted in telemetry and existing successful Agent mutation audits/no-op silence remain sufficient. If implementation requires an instrument change, document the contract, required tests/registry entries, and any approval before introducing it (FR-003; SR-001; Principle XV).

## Phase 3: User Story 1 — Maintain Tags Through the API (Priority: P1)

### Tests for User Story 1 [MANDATORY - Principle VIII]

- [ ] T016 Add domain table tests for counts 0/32/33, Unicode lengths 1/255/256 including non-BMP, empty, duplicates, NUL/UTF-8 and literal distinctions; copy-isolation and effective-definition cases, including the existing Agent field-coverage catalogue in `internal/domain/agents/reconcile_test.go`. Record semantic red (AT-1.1–AT-1.6; Principle VIII).
- [ ] T017 Add admin request/response tests for omission/empty/null/types on every write path and both collection-PUT branches, no writes on rejection, all response routes, sorted copies, and unchanged envelopes/auth/headers. Add memory alias/atomicity tests and consent projection tests. Record semantic red (AT-1.1–AT-2.4; Principles I/VIII).
- [ ] T018 Add PostgreSQL tests using production migrations: legacy rows, CRUD, every lookup/batch/list read, array constraints, atomic invalid writes, UP/DOWN/reapply, tag-loss rollback and unchanged UUID/grant/client-URI/timestamp data. Record real database red/green evidence via available infrastructure/CI, not Docker-in-Docker in the sandbox (DB-001–DB-004; Principles VIII/IX).

### Implementation for User Story 1

- [ ] T019 Extend `storage.Agent` validation/copy and reconcile membership comparison; retain effective shared candidate validation, identity rules, timestamps and audits, and no new ports or bounded context (FR-001–FR-005; ADR 047; Principles V/VI).
- [ ] T020 Persist tags through memory and PostgreSQL create/update and every explicit-column scan/read path listed in `plan.md`, using existing array transport, timeout and error patterns. Reject unsupported array shape/null elements and preserve atomicity/deep-copy isolation (FR-005; DB-004; Principle IX).
- [ ] T021 Extend admin parsing/conversion/presentation and both consent read projections. Keep stored spellings/order intact, sort only copies, omit empty tags and do not enumerate extra Agents or alter grant stats. Preserve batch loading and current principal/session context (FR-002–FR-005/FR-007; SR-002).
- [ ] T022 Run matching domain/adapter/HTTP tests and backend acceptance cases, proving all new semantic-red assertions turn green with minimal fixture-only adjustments; verify ten reversed-order replays are true no-ops (SC-001–SC-003; Principles VIII/XIII).

## Phase 4: User Story 2 — Display Categories to End Users (Priority: P1)

### Tests for User Story 2 [MANDATORY - Principle VIII]

- [ ] T023 Add Vitest cases for both raw API transformations and view types, tagged/untagged rendering, markup-looking text, long strings and a memoized DelegationCard rerender with unchanged grant stats; record semantic red before UI changes (AT-2.1–AT-2.4; Principle VIII).

### Implementation for User Story 2

- [ ] T024 Extend `web/src/types/consent.ts` and `web/src/services/api/consent.ts` to carry optional tags safely; use the approved application-specific Badge/Stack tag list in DelegationCard and AgentGrantDetailPage, and update memo equality to observe displayed tags without changing grant timestamps/cache TTLs (FR-006/FR-007; Principles VI/XI).
- [ ] T025 Run Vitest and Playwright acceptance journeys, capture nonempty overview/detail and empty-state screenshots plus long/markup/theme evidence, and verify accessible reading/navigation/wrapping without adding filters/editor controls (SC-004; Principles XI/XIII).

## 🔒 Phase N: Constitution Compliance Verification [MANDATORY COMPLIANCE SECTION]

**Purpose**: Verify constitution requirements and feature-scope compliance. Missing approval, tests or evidence remains unchecked, not silently waived.

### 🔒 Constitution Compliance Verification [MANDATORY]

#### Design Phase Verification [MANDATORY]

- [ ] T026 Verify tasks trace only to approved requirements/scenarios or binding constraints; glossary/model/config no-change disposition, authoritative OpenAPI approval, schema/rollout design and design-system review are complete (Feature Scope; Principles II/IV/V/VII/IX/X/XI).
- [ ] T027 Verify concrete journeys and editable/rendered wireframes match approved contracts, all ten scenarios have one referenced Ginkgo It each, semantic-red evidence exists per suite, and browser tests/screenshots are present without pending/Skip/placeholders. Verify observability disposition and required standard contracts if any instruments changed (Principles VIII/XIII/XIV/XV).

#### Implementation Phase Verification [MANDATORY]

**Feature Scope** (Development Requirements):

- [ ] T028 Verify no filtering, admin catalogue/editing UI, unrelated cleanup, new dependencies or unapproved policy/token behavior entered the diff. Handle unrelated findings only through the user's affirmative issue-filing decision (Feature Scope & Deferred Findings).

**API & Documentation** (Principles IV, X):

- [ ] T029 Verify all write/read/response/error/null/omission/order/version behavior matches the approved authoritative OpenAPI contracts; finish rendered `docs/api/` examples and management-guide full-definition/rollback warnings (Principles IV/X/XIV).

**Architecture & Documentation** (Principle II):

- [ ] T030 Verify `ARCHITECTURE.md` glossary/design/security/performance notes match actual implementation and accepted ADRs. If a merged Accepted ADR must be deviated from, obtain an Accepted superseding ADR first; do not amend it in place (Principles II/V).

**Configuration** (Principle VII):

- [ ] T031 Verify no ad-hoc config/default/Helm changes were introduced and existing deployment/config/auth/cache boundaries remain unchanged; no empty example configuration is necessary (Principle VII).

**Database & Persistence** (Principle IX):

- [ ] T032 Verify sequential atomic migration numbering, production-runner UP/DOWN/reapply evidence, documented tag loss on downgrade, old-data identity/relationship preservation, explicit SQL projections, both adapter invariants/copy isolation, every read path, and no per-Agent query regression (Principle IX; SC-005).

**Security** (Principles I, III):

- [ ] T033 Verify fail-closed administrative writes/end-user scoping and unchanged middleware, escaped non-authorizing display, no custom crypto, no new JWT/CIMD/CEL/ExtProc propagation, and mutation/no-op audit behavior (Principles I/III; SR-001–SR-003).

**Coding Standards** (Principle XV):

- [ ] T034 Verify every changed log/span/metric satisfies its applicable standard/checklist with required tests/contracts/registry entries; tags/credentials are absent from telemetry. Do not disable checks, add suppressions, or silently expand telemetry scope (Principle XV).

**Architecture Patterns** (Principle VI):

- [ ] T035 Verify domain-owned validation/equality and port-based persistence, no handler→repository bypass/cross-adapter coupling, and unchanged Builder/routing ownership. Keep helpers test-only where applicable (Principles VI/XII).

**Testing** (Principle VIII - Unit & Integration Tests):

- [ ] T036 Verify new tests were written first, compiled and failed semantically, changed minimally, and now pass; include meaningful unit/integration coverage and no Bash correctness tests. Run `just check`, focused Go tests, `just test`, frontend lint/format/build/test and full verification from CONTRIBUTING; record actual failures and unavailable infrastructure without weakening gates (Principle VIII).

**E2E Acceptance Testing** (Principle XIII):

- [ ] T037 Verify ten-scenario/It mapping, hierarchical fixture-based production-bootstrap journeys, references, meaningful semantic-red→green evidence, isolated data, ten replay requests, all HTTP write/read paths, and no mutation audit/write on replay. Run backend/ExtProc/frontend E2E and obtain real PostgreSQL/container evidence in CI where required (Principles VIII/XIII).

**Frontend** (Principle XI - if applicable):

- [ ] T038 Verify Badge/Stack reuse, semantic neutral tokens and labelled noninteractive lists; no new universal component is needed. Verify light/dark WCAG 2.1 AA contrast, narrow-viewport long-word wrapping, escaped text, memoized tag-only reload and missing-field compatibility with screenshots (Principles XI/XIII).
- [ ] T039 Review the final intended diff and completed gates, preserve explicit outstanding work, and leave the PR a draft until a maintainer decides otherwise. Do not close #427 based on design documents alone (Principles II/VIII/XIII).

## Dependencies & Execution Order

T001–T003 establish this draft. T004 selects the implementation feature. Phase 2 (T005–T015) blocks production implementation. T016–T018 and T023 must precede the production code their suites test. T019–T021 provide data for T024; T022/T025 prove green. Phase N completes release evidence. Local tests proceed independently of container-backed CI; unrun suites are never marked passed. Phase 0/refactoring and new-entity boilerplate are unnecessary.

## Design PR Validation (2026-11-14)

- Passed: `npm ci --prefix tools/wireloom --ignore-scripts --no-audit --no-fund` using the existing lockfile, no manifest or lockfile change.
- Passed: `just --tempdir /tmp/gh-aw/agent render-wireframes specs/056-agent-tags/spec.md` (three SVGs).
- Passed: `node --test tools/wireloom/render.test.mjs` (3/3 tests).
- Passed: temporary Node documentation validation (four Markdown documents, local links, ten unique scenario mappings, 39 unique tasks with only three drafting tasks checked, three wireframe links, five HTTP JSON bodies, and no-NUL schema pattern); all three SVGs parsed as XML; staged `git diff --check`.
- Passed: `just --tempdir /tmp/gh-aw/agent check` (format check, go vet, golangci-lint; zero issues).
- Passed: `just --tempdir /tmp/gh-aw/agent build`.
- Not passed: `AWS_EC2_METADATA_DISABLED=true just --tempdir /tmp/gh-aw/agent verify` stopped at the existing security gate: govulncheck reports public advisory GO-2026-6859 against the unchanged Go 1.27.3 toolchain (fixed in 1.27.4). Later aggregate test/integration/E2E stages were not reached. No security exception, dependency/toolchain change, or bypass was added; this documentation-only PR does not resolve that separate finding.
- Runtime feature tests have not been authored/run; the new behavior is proposed, not implemented. Checkboxes T004–T039 deliberately remain open.
