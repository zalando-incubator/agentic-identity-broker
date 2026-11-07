# Quickstart: Validate Incremental Service Authorization

**Feature**: [spec.md](./spec.md) | **Contracts**: [contracts/](./contracts/) | **Data model**: [data-model.md](./data-model.md)

This guide proves the feature end to end after implementation. It references contracts instead of
repeating them. All values are synthetic and follow the spec's shared cast.

## Remediation update — 2026-11-07

All five latest analysis findings have document remedies:

- Journey 2 now uses existing `AffectedAgent.agent_id`, retaining `service.id`. T011 validates the
  concrete spec example against that schema, not merely a separate contract example.
- API-CONSENT specifies exact authorize continuation failures: validated callback `302 access_denied`
  with `authorization session is invalid or expired`, or binding-mismatch `302 invalid_scope` with
  `requested service scope is not permitted`; request state is echoed or omitted if absent. T011/T012
  reconcile OpenAPI; T115 adds/reviews full domain/HTTP regressions without claiming already-correct
  behavior is new test-first implementation.
- The observability inventory covers all six feature-owned security-record labels and the
  continuation redirect-build diagnostic. T024/T106 cover every classification, level, bounded
  reason/error kind and credential-safe error/redirect path, without unrelated telemetry cleanup.
- Each new behavior depends on its own semantic-red tests. The historical T022 review branches to
  final T098/T109/T114 compliance; it no longer blocks independent approved foundations/story work.
- [validation-evidence.md](./validation-evidence.md) audits available browser history and proposes
  DEC-006's one-time missing-historical-execution exception, with retrospective sensitivity and
  current green validation required. No historical browser-red output was found, no exception is
  granted, and T022/T098/T109 remain unchecked until all applicable evidence conditions are met.

There are **15 Proposed sign-off items** and **115 tasks (85 historically checked, 30 unchecked)**.
The current API-CONSENT wording includes the explicit continuation errors; DEC-006 is the one new
consequential proposal. The checkpoints below retain historical outcomes, not fresh validation.

## Task update — 2026-11-07

The reconciled backlog has **115 tasks: 85 historically recorded complete and 30 unchecked**.
Task references throughout this guide use the regenerated IDs; checkpoint counts and test outcomes
below remain historical, not validation by this document phase. The original combined I2 task is
now T068–T069 (red) and T075–T077 (implementation/documentation/green); U4 documentation is T093.

- T001 first verifies all 15 current sign-off items and records only supplied approvals, including
  moving and applying the approved ADR 049 Revision 1 draft. The previous approval does not cover
  reconciled wording automatically.
- T011–T012 provide test-first current OpenAPI reconciliation to version 1.3.0 before behavior.
- T032–T033 validate merged ADR 046: no refresh-token expiry prediction; a recorded rejection
  disqualifies even valid access, while valid access without refresh/no rejection remains usable.
  Regression coverage of already implemented behavior is not fabricated semantic-red evidence.
- T023–T024 and T106–T107 cover the observability contract, credential/principal safety tests,
  canonical event attributes and bounded classifications. No new metric/span namespace is added.
- Historical browser-red evidence remains open (T022/T098/T109), even though browser journeys
  are recorded green. DEC-006 proposes the conditional alternative in `validation-evidence.md`;
  it is not approved. Do not weaken tests, claim retroactive red or silently grant an exception.
- The current active-session Wireloom source/SVG follows ADR 046 and shows no predicted refresh
  expiry. T097/T112 still verify examples and runtime accessibility in both themes.
- T113 runs every non-container recipe of the full gate and the prepared default-branch security
  scan. Real PostgreSQL and Agentgateway/container evidence remains deferred to the pushed-commit
  container tests, never locally validated or a missing-Docker blocker.

The earlier UUID-fallback proposals below are retained only as superseded historical discussion.
The authoritative current decision is DEC-005: document the defect and existing issue #446, with
no fallback, validator, migration or Admin API work in this feature.

## Planning update — 2026-11-07 (reanalysis I2 and U4)

The maintainer directed how to resolve the two open contract boundaries. Both are now in the
specification and plan as Proposed items that the next approval comment covers; nothing below is
implemented by this planning run.

- **I2**: proxy refresh validates broker-prefixed scopes before forwarding
  ([End-User API §4](./contracts/enduser-api.md#4-post-oauth2token-proxy-refresh_token-dec-002-api-proxy-refresh-validation),
  approval items API-PROXY-REFRESH-VALIDATION and ADR-SCOPES-REV1). T068–T069 provide semantic-red tests; T075–T077 implement, document and validate it.
- **U4**: the canonical IDs `.` and `..` are a known defect for this feature (DEC-005). No UUID
  fallback and no validation change; [issue #446](https://github.com/zalando-incubator/agentic-identity-broker/issues/446)
  restricts them. T093 documents the limitation only.

The "Remaining reanalysis contract boundaries" section of the 2026-10-29 checkpoint below keeps
its historical proposals; this update supersedes its open questions.

## Implementation checkpoint — 2026-11-07

**85 of 101 tasks complete; all 19 feature acceptance scenarios pass locally.** The supplied
approval still does not cover the I2/U4 revisions. This checkpoint supersedes earlier status and
missing-tool reports below; those sections retain their historical evidence.

### Completed work

- **T039/T041:** migration apply/down/reapply and PostgreSQL binding/succession tests are written;
  both integration-tagged suites compile via `go test -c` without starting containers. Real-database
  execution is deferred to the container tests; compilation is not red/green persistence evidence.
- **T067:** hybrid authorization regression coverage now exercises proxy, local and CIMD agents
  with a requested service: all enforce usability, only proxy forwards upstream, and forwarded
  scopes contain only ordinary values. Public (`none`) and confidential (`private_key_jwt`) CIMD
  code/refresh tests verify exact virtual scopes, authentication failure without consumption,
  service failure without rotation, successful retry and immutable successor bindings. Existing
  PKCE, code replay, client-credentials rejection and refresh-chain regressions pass unchanged.
  These are additional regression tests of already implemented behavior, not new feature logic
  or evidence of a historical semantic-red run.
- **T064/T086:** both complete browser journeys pass: focused consent preserves selections and end
  date through login/Save and succeeds on tool retry; service management refreshes and terminates
  back to the no-session state. All 17 backend scenarios also pass.
- **T111:** the capture-enabled serial browser run produced the four plan-named images under
  `tests/e2e/frontend/coverage/screenshots/`. They were reviewed against consent C/D and session
  A/B; the existing maintained images under `tests/e2e/screenshots/` remain intact. The active
  session state follows merged ADR 046 and no longer displays predicted refresh expiry.

### Passed validation

| Command | Outcome |
|---|---|
| `just check` | Pass after the added regressions; formatting, vet and lint pass. |
| `just test` | Pass with race detection after the added regressions. |
| `just extproc-test` | Pass with race detection. |
| `just web-test` | 34 files / 279 tests pass. |
| `just test-integration` | Self-contained suites pass, including Feature 055 contract/config checks. |
| `just test-e2e-backend` | 977 pass; 18 excluded by existing suite selection. All 17 feature backend scenarios pass. |
| `E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend` | 69 pass, none pending/skipped; both feature browser scenarios pass and all four captures are produced. |
| `just cdk-test mock-sample-agent-test mock-upstream-oauth2-test` | All three recipes pass. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Prepared default-branch security scan passes. |

The non-container recipes of the required full gate have passed, except that the ExtProc E2E
selection attempts did not restrict execution to non-container cases: 99 cases passed, but the
Agentgateway setup was attempted and failed for unavailable containers. This is **not** a passing
ExtProc suite. No test, skip marker or workflow was changed. The whole Agentgateway/container
portion remains deferred to the container tests, alongside `just test-integration-infra` and
`just test-e2e-docker`; no real PostgreSQL or container-backed result is claimed here.

### Remaining tasks

| Tasks | Remaining work / evidence |
|---|---|
| T004, T068–T069, T075–T077, T093 | Current proxy-refresh pre-forward validation and dot-segment known-limitation documentation remain unapproved. The 2026-11-07 planning update supersedes the earlier UUID-fallback alternative: no fallback or canonical-ID validation change is implemented here. T001 verifies all 15 current approval items before dependent work. |
| T022, T098, T109 | Backend semantic-red evidence is recorded historically. Historical compiling browser semantic-red evidence before feature logic is not established; green browser runs now cannot retroactively prove it. Unit/contract/Vitest red evidence and the complete traceability audit also need final review. No exception or waiver is approved; DEC-006 is now Proposed with the available-history audit and required alternative validation in `validation-evidence.md`. |
| T040, T042, T103 | SQL, PostgreSQL mappings and compiled migration/repository tests are present; container tests must establish their real-database apply/down/reapply and successor-inheritance evidence. Test-writing tasks T039/T041 do not wait for that evidence. Deferred validation, not a missing-Docker blocker. |
| T094, T096, T097 | Final approval/contract/spec-journey reconciliation is incomplete because I2/U4 remain unresolved. Immutable examples also predate merged ADR 046's observed-rejection API and removal of predicted refresh expiry. Supporting data model/accepted ADRs already follow that binding default-branch decision. |
| T110, T113 | All 19 feature journeys are green; final full-gate evidence still includes the deferred PostgreSQL/Agentgateway/container suites. No container suite is reported as validated. |
| T112 | Runtime keyboard/focus/screen-reader and measured text/UI contrast evidence in both themes remains incomplete; light-theme captures alone do not establish WCAG 2.1 AA. |
| T114 | Final checklist/evidence sign-off remains incomplete while the tasks above are open. The reviewer-owned checklist markers are unchanged. |

Current maintainer decisions are the 15 Proposed items in `plan.md` Approval Items, including the
three added for I2/U4 and DEC-006's narrow historical browser-execution exception. Historical
proposals below are not current approval text. Routine evidence audits remain implementation work;
only the explicit DEC-006 exception requires new sign-off, not the fact that an audit is open.

## Implementation checkpoint — 2026-10-31

**Feature behavior implemented; 79 of 101 tasks complete. All 17 backend acceptance scenarios
pass. Remaining tasks need infrastructure this sandbox lacks or maintainer approval.** This
supersedes the earlier checkpoints below for status.

### Completed since the previous checkpoint

- **US1 (T047–T054, T063):** authorization resolves virtual scopes after redirect validation,
  exempts them from `allowed_scopes`, evaluates every requested service for grant coverage and
  verified usability (`temporarily_unavailable` when unverifiable), seals requested IDs and bindings
  in the consent JWE and revalidates the same JWE on protected re-entry before code issuance or an
  upstream redirect. Consent returns `requested_service_ids`/`requested_service_coverage`, rejects
  unsatisfied submissions with `400 requested service not satisfied` before persistence and returns
  the continuation with the same `session_token`. One resolver and evaluator are wired for
  authorization, consent and local issuance in `internal/app/builder.go`.
- **US3 (T065, T066, T070–T074):** local refresh rechecks every immutable binding, including
  bindings narrowed out of `Scope`, before rotation (`invalid_scope` definitive, `server_error`
  transient, never consuming the token); `client_credentials` rejects service scopes; proxy
  authorization and token requests drop virtual scopes and omit an empty `scope`.
- **US2 (T078–T084):** `GET /api/third-party/sessions/{service}` and the `/sessions/:service` SPA
  page with Login (returning to the same page), Refresh and confirmed termination.
- **US4/docs (T087–T092, T101):** discovery never advertises virtual scopes; the operator guide
  `docs/guides/incremental-authorization.md`; updated gateway guide, token-exchange reference,
  end-user API overview, configuration reference, examples, `ARCHITECTURE.md` and agent context.
- **Consent UI (T051, T060–T062):** requested-service banner, candidate markers that never select or
  lock optional sets, and live Save gating with an `aria-describedby` reason.
- **Compliance verified (T095, T099, T100, T102, T104–T108):** no new broker or ExtProc config key and
  no chart change; `servicescope` imports only `canonical`, `id`, `model`, `storage` and `ports`;
  ExtProc imports no broker package; new adapters do not import each other; structured, credential-
  free events `incremental_authorization_required`, `requested_service_submission_rejected`,
  `refresh_service_recheck_failed` and `challenge_metadata_invalid` plus presence-only challenge
  booleans.

Two decisions taken within the approved contracts:

- For submissions carrying requested services, the requested-service check runs after structural
  validation but before the existing session-presence (FR-020) check, so the response names the
  unsatisfied requested service rather than the generic `unconnected services` error.
- ExtProc's existing 5-second re-authentication cooldown (ADR 012) is unchanged; the gateway
  acceptance test retries within that bound, consistent with client-owned bounded retries.

### Validation in this run

| Command | Actual outcome |
|---|---|
| `go test -p 2 ./internal/... ./cmd/...` | All pass except the same three sandbox-network tests in `internal/adapters/encryption/aws` and `internal/adapters/outboundhttp` that fail identically on base commit `28baed0`. |
| `go test ./tests/integration/...` (self-contained) | Pass, including the new overlay/route example contract test. |
| `ginkgo --procs=2 --label-filter='!performance && !docker' ./tests/e2e/` | After updating one existing consent-continuation assertion: all specs pass (17/17 Feature 055 scenarios). |
| `ginkgo --label-filter='!docker' ./tests/e2e/extproc/` | 98 passed; 1 failed starting the Agentgateway container (no Docker in this sandbox). |
| `npx vitest run` (web) | 34 files, 280 tests pass. `npm run build` succeeds; `npx eslint src` reports 0 errors (2 pre-existing warnings). |
| PostgreSQL migration/repository tests, browser E2E, `golangci-lint`, `just verify` | **Not run** — no Docker, no playwright-go driver, golangci-lint built with Go 1.25, `just` not installed. |

### Remaining tasks and required actions

| Tasks | Status / required action |
|---|---|
| T004, T068–T093 | Maintainer approval for the U4/I2 contract revisions (unchanged). |
| T022, T064, T086, T110–T111 | Browser journeys (US1 S4, US2 S2) and screenshots: install the playwright-go driver v1.62.1 and run `just test-e2e-frontend`. The Agentgateway ExtProc journey needs Docker. |
| T039–T042, T103, T113 | Run `just test-integration-infra` and `just verify` on a Docker-capable runner; the SQL, PostgreSQL mapping and tests are written. |
| T067 | Hybrid/CIMD continuity variants beyond the covered `client_credentials` rejection. |
| T094, T096–T098, T109, T112, T114 | Final review gates: spec journey re-check, OpenAPI/migration sign-off, red-evidence audit, contrast measurement in both themes and checklist sign-off. |

## Implementation checkpoint — 2026-10-30 (second run)

**In progress; 39 of 101 tasks complete. Acceptance tests, shared foundations, local issuance,
exchange recovery and the ExtProc challenge are implemented. Authorization/consent integration,
the session detail page, SPA work, local refresh recheck, proxy stripping, docs and final gates
remain.** This run supersedes the earlier 2026-10-30 checkpoint below for status; that section
keeps its original validation evidence.

### Completed in this run

- **T015–T018, T020–T021** acceptance tests: the shared cast fixtures, mock-provider refresh and
  scope controls, nine local journeys (`incremental_authorization_test.go`), two broker-plus-ExtProc
  gateway journeys, two proxy journeys, the operator-guide/discovery check and two browser journeys
  with page-object locators. All 17 backend scenarios compiled and failed on missing behavior before
  any feature code (red evidence: 17 failures, e.g. no `requested_service_ids`, 200 instead of
  `invalid_scope`, unstripped upstream scope, missing guide).
- **T025–T029** `internal/domain/servicescope/`: configured-origin grammar/emission, requested-set
  resolution with eligibility and binding pinning, the shared coverage rule and requested-service
  evaluation. Token exchange now uses the shared rule; all existing exchange tests pass unchanged,
  including the final-entry-wins duplicate-grant case.
- **T030–T034** `ServiceSessionSnapshot`/`CheckServiceUsability` on top of `GetValidAccessToken`.
- **T035–T038** binding validation and both memory stores (deep copies, exact successor inheritance,
  no scope widening).
- **T044** sealed `requested_service_ids`/`service_scope_bindings` and exposed original expiry.
- **T045–T046, T055–T056** local issuance: allow-list exemption only with a binding, exact scope
  forms in response and JWT, code-to-refresh bindings, code-exchange rename/reuse rejection,
  exact (case-sensitive) non-consuming widening refusal and durable narrowing.
- **T050, T057–T059** exchange `scope` member on recoverable errors and service-page recovery
  (ADR 048); ExtProc decodes `scope`, validates `aib.tokenexchange.resource_metadata_url` and
  appends both attributes in fixed order with value-free diagnostics.
- **T085** existing acceptance assertions updated for the approved recovery target and `scope`.

### Validation in this run

| Command | Actual outcome |
|---|---|
| `gofmt -l` on changed files; `go vet` on changed packages | Clean. `internal/domain/oauth2/service_test.go` was already unformatted before this branch and is untouched. |
| `golangci-lint run` | **Not run**: the installed binary is built with Go 1.25 and refuses the Go 1.27.3 module. |
| `go test -p 2 ./internal/... ./cmd/...` | All packages pass except `internal/adapters/encryption/aws` (`TestBuildAWSConfig_DisableSSL`) and `internal/adapters/outboundhttp` (two SSRF/proxy tests). Both fail identically on the branch's starting commit `28baed0` in this sandbox and are not touched here. |
| `go test -race ./internal/extproc/...` | Pass. |
| `ginkgo --procs=2 --label-filter='!performance && !docker' ./tests/e2e/` | Before T085: 896 passed, 32 failed. After T085 the 17 affected existing scenarios pass (33/33 in their focused suites). Remaining failures are the 15 not-yet-implemented Feature 055 scenarios. US1 S1 and US4 S1 (gateway) now pass. |
| `ginkgo ./tests/e2e/frontend/` | **Blocked**: the playwright-go driver v1.62.1 is not installed and downloading it was not permitted. Both browser scenarios compile; red was not observed. |
| PostgreSQL migration/repository tests (T039–T042) | **Blocked**: no Docker in this sandbox. Migration SQL, PostgreSQL JSONB mapping and integration tests are written and vet-clean; a self-contained codec unit test passes. |
| `just …` recipes | `just` is not on PATH in this container; the equivalent underlying commands above were run. |

### Remaining work

| Tasks | Status / next step |
|---|---|
| T004, T043, T068–T093 | Unchanged blockers (U4/I2 approvals); T043's authorize-re-entry assertions belong with T047/T087. |
| T022 | Backend red evidence recorded; browser red needs the playwright-go driver. |
| T039–T042 | Code written; needs a Docker-capable runner to execute and check. |
| T047–T049, T051–T054, T060–T064 | Authorization-endpoint requested-service evaluation, consent arrays/submission, protected re-entry, builder wiring (`WithServiceScopes`, bindings through the proceed strategy) and SPA. |
| T065–T074 | Full binding recheck on local refresh with usability, proxy scope stripping, `client_credentials` rejection. |
| T078–T084, T086 | Session detail domain read, route and SPA page. |
| T087–T092 | Boundary regressions, operator guide and reference docs. |
| T094–T114 | Compliance and the full `just verify` gate. |

## Implementation checkpoint — 2026-10-30

**Blocked; approved contract and compiling acceptance tests are added, but no feature behavior or migration is implemented.**

### Progress

- T009: `api/enduser/openapi.yaml` is now version **1.2.0** and carries the approved authorization,
  protected continuation, local/proxy scope, exchange-recovery, consent and session-detail deltas.
  The legacy UUID session route and its schemas remain unchanged. The new dependent-agent list
  references the approved existing `AffectedAgent` schema (`agent_id`, `display_name`).
- T010: `tests/integration/incremental_authorization_contract_test.go` validates nullable session
  detail, required fields, all four public baseline/candidate usability values, required requested
  arrays and response examples. The first run compiled and failed because `allOf` with nullable
  did not admit `session: null`, and the token error enum omitted `invalid_scope`. An explicit
  null-only alternative and the approved error enum made these tests green without weakening the
  non-null `SessionStatus` requirements or changing legacy schemas.
- T019: `tests/e2e/service_session_page_test.go` now owns exactly the three US2 backend scenarios
  S1/S3/S4. It uses fresh production bootstrap/storage and existing fixtures per scenario, checks
  both identifier forms, principal-owned dependent agents and absence of read-triggered refresh,
  and includes provider sign-in/return and recovery/exchange retry without grant mutation.
  These are executable acceptance tests, not passing feature behavior. The full backend run
  proves all three compile and fail on missing functionality: known-service detail is 404 instead
  of 200, unknown-service error is plain text instead of JSON, and recovery lacks `scope`.
- `ARCHITECTURE.md` now identifies the 1.2.0 approved contract separately from incomplete runtime
  implementation. `spec.md`, `plan.md` and the supplied approval are unchanged.

### Validation in this run

| Command | Actual outcome |
|---|---|
| `just check` | Exit 0 after formatting; vet and installed golangci-lint pass with 0 issues. An earlier formatting failure was corrected. |
| `just test` | Exit 0. The first attempt timed out after 180 seconds; the subsequent complete run passed. Baseline package coverage does not implement Feature 055. |
| `just test-integration` | Exit 0, including the new contract tests. Initial dependency setup timed out; the first completed run exposed the two schema failures described above, then the corrected contract passed. |
| `go test ./tests/integration -run '^TestIncrementalAuthorization' -count=1 -v` | Exit 0 after the observed semantic-red schema failures. |
| `ginkgo --label-filter='!performance' --focus='Incremental Authorization Service Session Page' ./tests/e2e/` | Exit 1; all 3 new scenarios compile and fail for missing behavior. |
| `just test-e2e-backend` | Exit 1 after successful frontend build: 911 passed, the 3 new Feature 055 scenarios failed, 0 pending, 10 excluded by the existing suite labels. This is semantic-red evidence, not a passing acceptance gate. |

Ginkgo and golangci-lint are now exposed by the workflow's existing setup. The earlier missing-tool
blocker no longer applies to them. No tool versions or installation steps were changed here.
Real PostgreSQL/container validation and the complete frontend/ExtProc journeys and full
`just verify` gate are still outstanding; no new result is claimed for those gates. Docker-in-Docker
is unsupported in this sandbox, so the real database gate needs a supported execution environment.

### Remaining work and required maintainer actions

The supplied approval still covers only its original 11 items. The two contract revisions below
remain unapproved; this run neither edits their accepted ADR text nor implements them.

1. **T068 / I2**: approve the proposed pre-forward grammar/service/agent-eligibility validation and
   exact proxy-refresh errors, or an explicit constitution-compatible alternative. Authoritative
   proposal and alternative remain in the previous checkpoint's **Remaining reanalysis contract
   boundaries** section. Do not add a usability check or post-upstream-response veto.
2. **T093 / U4**: approve UUID-only emission/recovery for complete canonical IDs `.` and `..`, or
   a compatible alternative. Reconcile the spec/plan in an authorized pre-implementation run;
   implementation cannot silently amend them or widen the existing approval.
3. Provide a supported real-PostgreSQL execution environment for migration/repository validation
   and the final full gate; do not waive those tasks.

| Deferred tasks | Remaining work / reason |
|---|---|
| T004, T068–T093 | The emission model and the two API/ADR boundary revisions need authorized reconciliation and sign-off. T009 contains only the original approved deltas. |
| T015–T018, T020–T022 | Finish shared fixtures and the other 16 owning scenarios, including both browser journeys; run all 19 compiling semantic-red tests before foundations. The three T019 scenarios are written but not green. |
| T025–T046 | Shared scope/coverage/usability, bindings in both adapters, real migration tests/SQL, JWE continuity and local issuance remain behind the incomplete T022 checkpoint. |
| T047–T092 | All user-story behavior, domain/handler/frontend tests, integrated operator/API documentation and green acceptance journeys remain. The three new tests intentionally expose the absent session-detail and recovery behavior. |
| T094–T114 | All final compliance gates, real persistence validation, all acceptance journeys, four screenshots and `just verify` remain. No checklist marker is changed or gate waived. |

## Implementation checkpoint — 2026-10-29

This historical checkpoint is superseded by the 2026-10-30 progress and validation above.

**Blocked; setup/design-precondition progress only. No feature behavior, migration or new acceptance test is implemented.**

### Supplied approval

All 11 items in the immutable plan match the supplied approval by **mjungsbluth** on
**2026-10-29**, [approval comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-6509178189).
ADRs [048](../../adrs/048-service-session-recovery-return.md) and
[049](../../adrs/049-virtual-service-scopes.md) now record that approval in `adrs/`.
Their feature-local paths remain reference-only pointers. Approval is not evidence that the
feature is implemented. The original spec and plan are intentionally unchanged.

### Tooling and validation blocker

The installed executables report Go **1.27.3**, Node **24.22.1**, npm **11.19.1** and
just **1.58.0**. Migration number **048** is available after existing migration 045.
`ginkgo`, `golangci-lint` and `wireloom` are not on PATH. Tool versions have not changed in
this feature; no tool installation or permission workaround was attempted.

| Command | Actual outcome |
|---|---|
| `just check` | Exit 0. Formatting/vet pass; the recipe reports missing `golangci-lint` and falls back to vet. This is not a full lint run. |
| `just test` | Exit 0 on the existing application; not evidence of Feature 055 coverage. |
| `just test-integration` | Exit 0 on the existing self-contained suites; does not validate the new examples through dedicated Feature 055 assertions or replace real PostgreSQL tests. |
| `just test-e2e-backend` | Exit 1 after successful frontend build: `ginkgo is not installed`. No acceptance spec executed. |
| `just test-e2e-frontend` | Exit 127 after successful frontend build: `ginkgo: command not found`. No browser journey or capture executed. |
| `just test-integration-infra` | The attempt timed out after 30 seconds; no completed migration/repository validation. Container-backed validation is not available in this sandbox (Docker-in-Docker is unsupported). |
| `just verify` | Timed out after 90 seconds during security-tool dependency setup; the full gate did not complete. |

**Required maintainer action**: make the workflow's existing tool setup expose its pinned
Ginkgo, lint and Wireloom executables to the agent, and provide a supported runner for the
required real PostgreSQL/container suites. No failed/missing gate is waived. Once the harness
is available, finish the acceptance fixtures/tests and observe all 19 semantic-red scenarios
(T015–T022) before implementing foundations or user stories. Existing baseline tests passing
cannot substitute for that checkpoint.

### Remaining reanalysis contract boundaries

The supplied approval covers the 11 published items, not an unlisted exception or contract
revision that resolves the remaining findings. No unapproved behavior is implemented.

- **I2 (proxy-refresh validation)**: FR-002 requires rejecting malformed, unknown and ineligible
  broker scopes, but the approved proxy-refresh text only specifies stripping and preserving
  upstream outcomes. Recommended **Proposed API clarification**: validate syntax, resolve the
  request's agent through the existing client-resolution path, and check service existence and
  eligibility before forwarding; use `400 invalid_scope` with `requested service scope is not
  permitted` for the three invalid-scope categories and `500 server_error` for lookup failure.
  Ordinary-scope-only requests retain their existing path. Do not check provider usability or
  veto any upstream response. Rationale: satisfy FR-002 without losing rotated credentials.
  Main alternative: explicitly approve a proxy-refresh exception to FR-002. The newly specified
  pre-forward response contract requires explicit approval before code; T068 tracks it.
- **U4 (dot-segment IDs)**: existing canonical validation permits the complete identifiers `.`
  and `..`, while the approved emission/recovery contract always prefers an existing canonical
  ID. Browser path normalization prevents those URLs from reliably naming one service.
  Recommended **Proposed API/ADR revision**: emit UUID scope and recovery/page URLs only for
  those two complete identifiers, retain both existing canonical records and raw-scope parsing,
  and preserve exact granted strings/bindings and UUID rename durability. Do not tighten the
  global canonical-ID validator. Rationale: browser-safe recovery without invalidating existing
  records. Main alternative: explicitly approve a canonical-ID migration/restriction, which is
  broader and backward-incompatible. This fallback is not covered by the approved always-prefer-
  canonical text. T093 requires approval and regression coverage before implementation.

**Required maintainer action**: approve the proposed pre-forward API clarification and the
UUID-only emission/recovery exception for `.` and `..` (or supply a compatible approved
alternative). The immutable spec/plan must be reconciled in an authorized pre-implementation
run before those revisions are implemented. The existing approved ADR text is not silently
changed to imply that either revision was approved.

### Deferred work and resumption

| Tasks | Why unfinished / required next work |
|---|---|
| T004, T009, T068–T093 | Reconcile the two contract boundaries above under explicit approval, then finish the model review and complete OpenAPI 1.2.0 before behavior changes. |
| T015–T022 | Fixtures, 19 acceptance tests and semantic-red evidence remain unwritten/unexecuted. Expose Ginkgo, finish these tests and run the actual backend/browser suites. |
| T025–T046 | Foundations depend on T022. Keep migration SQL behind failing real-PostgreSQL tests; complete scope, usability, identity binding, both adapters and issuance in test-first pairs. |
| T047–T092 | All user-story tests, implementation and integrated documentation remain; they depend on the acceptance checkpoint and foundations. Continue the approved TDD sequence, including both reanalysis regression sets. |
| T094–T114 | Final compliance, complete lint, real database suites, all E2E journeys, four screenshots and `just verify` remain. Supply the supported tooling/runtime and rerun every required gate after implementation; do not treat baseline passes as feature completion. |

## Prerequisites

- `just install-tools` has completed, including Playwright browsers.
- Docker or Podman is available for infrastructure-backed integration tests and the full
  verification gate's container-backed suites. This feature's broker-plus-ExtProc challenge tests
  use `bootstrap.ExtProcEnvironment` and assert gRPC immediate responses without a gateway container.
- The maintainer has approved the items in [plan.md › Approval Items](./plan.md#approval-items).
  ADRs 048 and 049 have been moved to `adrs/` with an Accepted status.

## 1. Static checks and fast tests

```bash
just check
just test
```

Expected:

- The new `internal/domain/servicescope` table tests pass for the grammar in
  [contracts/virtual-service-scope.md](./contracts/virtual-service-scope.md).
- The `oauth2server` refresh tests show that `invalid_scope` leaves the refresh session unused.
- The ExtProc challenge tests cover `scope` and `resource_metadata` inclusion and omission.

## 2. Migration and repositories

```bash
just test-integration-infra
```

Expected: migration `048_add_service_scope_bindings` applies, rolls back, and reapplies. Bindings
survive the round trip on `authorization_codes` and `refresh_token_sessions`, and `NULL` rows stay
readable. Both adapters enforce exact successor binding inheritance and scope subset. Exercise code
issuance with full scopes → successful narrowed refresh → scope-less successor refresh: successor
`Scope` and response/JWT stay narrowed, every original binding survives and is rechecked, and neither
a scope-less refresh nor a retained binding can restore a removed scope. Include third-step service
failure and re-requested removed-scope refusal, both without token consumption.

## 3. Backend acceptance journeys (spec Journeys 1, 3, and 4)

```bash
just test-e2e
```

Run the suites below, or the whole E2E set on a container-capable runner. In the agent sandbox use
only the non-container recipes listed in the task update; do not invoke the container-backed
aggregate `just test-e2e`.

| Suite | Proves |
|---|---|
| `tests/e2e/incremental_authorization_test.go` | Story 1, Scenarios 2, 3, 5, 6 and 7; Story 3, Scenarios 1 and 4; Story 4, Scenarios 2 and 3. Complete authorization/consent/code/refresh/exchange journeys cover non-consuming widening refusal, server-derived coverage, submission enforcement, exact visible scopes, local refresh rechecks, and rename/reuse identity continuity. |
| `tests/e2e/incremental_authorization_proxy_test.go` | Story 3, Scenarios 2 and 3. A mock upstream IdP records the received `scope`: virtual scopes never appear, and the rotated upstream refresh token is returned unchanged. |
| `tests/e2e/service_session_page_test.go` | Story 2, Scenarios 1, 3 and 4: both detail identifiers, `session: null`, `401` and `404`, principal isolation, provider login/callback to the same service page and successful exchange after direct recovery. |
| `tests/e2e/incremental_authorization_gateway_test.go` | Story 1, Scenario 1 and Story 4, Scenario 1. Each owns one complete broker-plus-ExtProc scenario, including persisted provider rejection, scope/action propagation and unchanged elicitation, with metadata-present/absent variants. |
| `tests/e2e/incremental_authorization_docs_test.go` | Story 4, Scenario 4: live discovery excludes virtual scopes and the operator guide's route/config example agrees with the contract. |
| `tests/e2e/frontend/incremental_authorization_test.go` | Story 1, Scenario 4 and Story 2, Scenario 2. Consent States C/D and service-page States A/B are exercised through the existing page objects, including login, Save/tool retry and refresh/termination. |

The [task mapping](./tasks.md#acceptance-scenario--task-mapping) assigns exactly one owning `It`
to each of the 19 scenarios; supporting unit/integration assertions do not duplicate acceptance
ownership. Run `just test-e2e-backend` for these broker-plus-ExtProc and proxy journeys, and
`just test-e2e-frontend` for the two browser journeys.

### Required regression variants

Keep these variants inside their existing owning scenarios, with semantic-red assertions first:

- US1 S4/S7: all-scopes and no-explicit-requirements agents, empty configured required scopes,
  selected Read Access needing `repo user:email`, but valid access with only `repo`. Selecting the
  failing candidate disables Save, deselecting it can enable Save; direct submission and refresh
  cannot claim usability until the effective union is present. Complete login/Save/issuance/exchange.
- US4 S2: Save successfully, deterministically reassign the canonical ID, then follow the returned
  authorize URL with its original JWE. Assert `invalid_scope`, no authorization code and no proxy
  redirect, even when the replacement service has a usable session and grant coverage.
- US3 S3: proxy refresh with a malformed (`/sessions/github/`), unknown (`/sessions/unknown`) or
  agent-ineligible broker scope returns `400 invalid_scope` with
  `requested service scope is not permitted`; the mock upstream IdP records zero calls and the
  upstream refresh token still works afterwards. Supporting unit cases cover a lookup failure,
  which returns `500 server_error` with zero upstream calls. An ordinary-only refresh and a lookalike-origin scope keep the existing
  forwarding path.
- US3 S1: full → narrowed → scope-less refresh retains narrowed scopes and every assurance binding;
  test subsequent unusability and attempts to restore removed scopes without consuming the token.
- Supporting T030/T032 unit cases: valid scope-sufficient, identity-matching access without refresh
  and without recorded rejection is usable without provider calls. Recorded `RefreshRejectedAt`
  disqualifies even valid access; expired access without refresh is unusable. No test predicts
  refresh-token expiry. Existing passing behavior is regression evidence, not retroactive red.

### Maintained UI captures

Normal verification does not capture screenshots. For maintained artifacts, use the existing
capture-enabled serial frontend harness:

```bash
E2E_CAPTURE_SCREENSHOTS=true GINKGO_FRONTEND_PROCS=1 just test-e2e-frontend
```

The harness stages captures under `coverage/screenshots/` relative to the frontend suite working
directory. Retain the four plan-named images under `tests/e2e/screenshots/` using existing screenshot
handling; do not modify workflows. Verify consent C/D and session A/B against the spec's Wireloom
states. The design composition review below records T014; capture evidence is historical in the
latest implementation checkpoint, while runtime accessibility validation remains unfinished.

### Design-system composition review (T014)

The review uses the design-system index, common-mistake guidance, decision trees and component
pairing guide, together with the existing `PermissionSetCard`, `SessionCard` and
`TerminationDialog` source. Both new compositions are application-specific, not universal
components; no design-system primitive or visual-direction change is needed.

| Wireloom state | Existing composition and behavior |
|---|---|
| Consent C — requested service needs login | Keep agent header, End Date and the existing permission-set/service switches. Use one application `RequestedServiceBanner` per service built from `Alert`/`Card`, text and `Badge` markers for every covering candidate. Use warning semantics plus explicit text, not color alone. Explain partial required-scope coverage and the selected candidate's usability separately; keep optional toggles editable and never auto-select them. |
| Consent D — ready to save | Change the banner to success with a textual ready explanation only when the baseline and every selected contribution are usable and the selected union covers required scopes. Unselected unusable candidates do not block Save. Save remains the single primary action for submission; separate the existing destructive revoke action visually. Preserve end date, unrelated toggles and protected request context through login. |
| Session A — missing/unusable | Use `Card` with service identity, scope URL, text-labelled warning state, the principal's dependent agents and a primary Login action. Distinguish an existing unusable row from no row. A loading skeleton, authenticated not-found state and retryable `Alert` are separate states, not fabricated empty sessions. |
| Session B — active | Reuse `SessionCard` content and UUID-based Refresh/Terminate actions, plus `TerminationDialog` confirmation. Show scopes, expiry, encryption and the principal-owned dependent agents. Successful refresh refetches detail; termination returns to State A. Login/management never mutates an agent grant. |

Use the existing design-system semantic tokens and variants (surface, trust, success, warning,
error and secondary text), not raw palette utilities, custom CSS, inline color values or external
assets. Use `Card padding="default"`, existing layout spacing and the existing switch keyboard
behavior/labels. Retain visible focus indicators. Requested-service markers must have readable
text as well as status color. Keep disabled-Save reasons outside the button, visible to keyboard
and screen-reader users, with stable IDs linked via `aria-describedby`; announce changing
satisfaction/retry status without moving focus on each toggle.

The existing light/dark tokens are the source of truth. Implementation must test both themes for
4.5:1 text and 3:1 UI contrast, keyboard focus/order, switch names/state and confirmation focus
restoration. Reusing an existing component does not prove accessibility or matching usability
semantics; T080/T084/T112 must cover any minimum adaptation needed by this feature without
unrelated restyling. Verify the four maintained browser captures against the current spec and
complete the outstanding runtime checks; previous captures alone do not establish both-theme
accessibility.

## 4. Manual spot check (optional)

1. Start the broker with an in-memory store and `oauth2_authorization_server.mode: "local"`, using
   the configuration from spec Journey 1, Step 1.
2. Reproduce spec Journey 1, Step 2 with the token-exchange request. The JSON error contains
   `"scope":"https://broker.example.com/sessions/github"`
   ([contracts/enduser-api.md §1](./contracts/enduser-api.md#1-post-oauth2token-rfc-8693-error-scope-member-api-exchange-challenge)).
3. Repeat spec Journey 1, Step 3 with the refresh request. The response is
   `400 invalid_scope`. Repeating the same refresh without `scope` then succeeds, which shows the
   token was not consumed.
4. Open `https://broker.example.com/sessions/github` while signed in. State A renders. Login
   returns to the same page in State B.
5. Rename the service's canonical ID to `github-production` through the admin API.
   - `/sessions/github` now shows not found.
   - A local refresh bound to the old scope returns `invalid_scope`.
   - A new exchange error emits `/sessions/github-production`.

## 5. Gateway metadata variant

Add `resource_metadata_url` to the agentgateway route's `metadataContext`, as in
[contracts/extproc-challenge.md §1](./contracts/extproc-challenge.md#1-optional-metadata-field-aibtokenexchangeresource_metadata_url-api-metadata-config).
Trigger an exchange error.

- With the field, the `WWW-Authenticate` header contains `resource_metadata="…"`.
- Without it, the header omits that attribute. The `scope` and `error_uri` are identical in both
  variants.

## 6. Rollback note

Rolling back requires reverting the code first, then migration 048. Rolling back the code alone is
not enough: the pre-feature broker treats a granted virtual scope as an ordinary scope string.
After the rollback, affected clients may need to authorize again. No user session or grant data is
lost. The discarded service-scope bindings cannot be recovered by reapplying migration 048;
affected clients need fresh interactive authorization to reestablish identity pinning.

## 7. Full completion gate

```bash
just verify
```

All tasks, including migration/repository validation and the complete acceptance journeys, remain
required. In this automation sandbox, run every non-container recipe of `just verify`, using the
prepared default-branch security scan in place of `just security`. Defer container-backed suites
to the container tests on the pushed commit; their absence is not a sandbox blocker and never
counts as passing evidence. Distinguish successful commands from deferred validation and leave
unfinished tasks unchecked.
