# Quickstart Validation: AuthZEN Delegation Policy Information Point

**Feature**: `specs/054-authzen-delegation-pip` | **Plan**: [plan.md](./plan.md)

**Latest outcome**: Document remediation reconciles all eight findings. Proposed ADR-2/API-7/D-8 define identity-safe binding reuse, terminal concurrency conflict and explicit provider/provenance limits; Proposed ADR-3 defines canonical-field guidance with present compatibility; D-7 records exact telemetry migration/registration. Current 122 tasks retain 104 historical checked and 18 unchecked markers. T103 publication permission and T104 follow-up issue remain external implementation prerequisites, not waived obligations. Original D-6 closure remains historical, not evidence for US3.11–US3.13.

This guide proves the feature end to end after implementation. It is a validation guide, not an
implementation description. The contracts are in [contracts/](./contracts/). The value objects
are in [data-model.md](./data-model.md).

## Status and prerequisites

Before any implementation, these preconditions must hold:

1. The maintainer has signed off the items in the plan's
   [Approval Items](./plan.md#approval-items) with a 👍 on the pipeline's approval comment.
   These cover [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md),
   [contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md), and the
   ADR draft [adr-048-authzen-delegation-pip.md](../../adrs/048-authzen-delegation-pip.md).
2. T001 records only supplied approval for current covered items, explicitly ADR-2 for `adr-048-revision-permit-bound-tokens.md`. ADR 048 already resides in `adrs/`; incorporate its covered revision with approval in Status/Revision history and remove the incorporated draft, without duplicating its historical move. Move only other covered ADR drafts. Historical direction or thirteen-item approval does not cover ADR-2.
3. The API-003 **Verify** checks against the final AuthZEN text have passed before the OpenAPI
   is frozen.
4. The spec revision for the OPA input shape (research R15) has landed. This was done on
   2026-10-28.
5. The Phase 0 extraction is isolated from feature behavior in its own commit with unchanged
   regression assertions. The current implementation workflow explicitly permits this in-PR
   refactoring commit; `b882d7b` is already in this branch (T002–T005).

Tooling is installed by the workflow (Go 1.27.3, Ginkgo, golangci-lint and browsers). The
PostgreSQL outage scenario US2.19 and container-backed ExtProc/agentgateway and integration
suites run in downstream container tests on the pushed commit. Do not start these suites or
try to enable a container runtime in the agent sandbox. Their absence is not a sandbox blocker;
report them as deferred, never validated. Run all non-container verification recipes locally.

Implementation evidence belongs in this guide: record approval provenance, the isolated refactor commit, scenario-to-test `file:line` mappings, semantic-red assertions, green commands/results and deferred-task reasons. Keep the approved `spec.md` and `plan.md` unchanged throughout implementation.

## 1. Static checks and fast tests

```bash
just check            # fmt → vet → lint
just test             # broker domain/adapters/app unit tests
just extproc-test     # ExtProc unit tests with -race, including the example-config loading test
```

**Expected**: All pass. The existing token-exchange unit tests pass unchanged after the Phase 0
resolver extraction, which proves FR-006 does not regress.

## 2. Broker acceptance journeys (US1, US2, US4.6)

```bash
ginkgo -v --label-filter="authzen-delegation && !docker" ./tests/e2e/
```

US2.19 MUST have both `authzen-delegation` and `Label("docker")`. The local command above
and `just test-e2e-backend` exclude it: 28 broker scenarios run locally. The remaining
broker scenario runs only downstream through `just test-e2e-docker` on the pushed commit;
never invoke that container recipe in the sandbox. Report it as deferred until CI evidence
exists, not locally validated or a sandbox blocker.

```bash
just test-e2e-backend     # 28 broker feature scenarios plus backend regressions
```

**Expected combined local/downstream coverage**: 29 broker scenarios:

- US1.1–US1.8 (8)
- US2.1–US2.20 (20)
- US4.6 (1)

Spot-check these observable outcomes against spec Journeys 1–3:

| Check | Expected |
|---|---|
| `GET /.well-known/authzen-configuration` | `policy_decision_point` equals the configured `server.enduser.public_url` |
| Permit with `X-Request-ID: delegation-001` | 200, `decision: true`, `Cache-Control: private, max-age=60`, request ID echoed, no `subject.id` JWT anywhere in the body |
| Grant `valid_until` in 15 s | `max-age` ≤ 15 |
| Revoked grant | 200, `decision: false`, `reason_admin.code = grant_missing`, `Cache-Control: no-store` |
| `subject.type: identity` | 400 |
| No `Authorization` | 401 with `WWW-Authenticate: Bearer` |
| CEL rejects the gateway | 403 |
| Renamed `user_grants` table | 500 |
| Token exchange disabled | Both routes return 404 |

## 3. Consumer journeys (US3, US4.5)

```bash
ginkgo -v --label-filter="delegation-context" ./tests/e2e/
```

**Expected after approved implementation**: 14 specs pass (US3.1–US3.13 and US4.5). They run the production ExtProc
composition against the real broker test server. US3.11 is pending implementation under
Proposed ADR-2; until then 11 specs exist. Confirm these outcomes:

- The body path forwards only after both exchange and resolution succeed.
- The header-only path never calls exchange when policy denies.
- US3.5 re-consults the broker immediately after renewed consent on both paths. Header-only
  forwarding permits immediately; body-path forwarding resumes once the unchanged ADR 012
  exchange re-consent cooldown (5 s) lapses. Assert request counts and use bounded
  `Eventually` polling rather than fixed sleeps or a cooldown bypass (Proposed D-5).
- After revocation, a repeat request past `max_staleness` is denied with 403 and `error_uri`
  set to the consent URL.
- A broker outage with a still-cached exchanged token returns 503, not a stale permit.
- The canonical-ID policy allows on both paths. The policy reads `input.context.agent`,
  `input.context.resource`, and `input.context.permission_sets`, and
  `granted_permission_sets_available` is `true`. Neither `input.context.delegation` nor
  `delegation_available` is present (contract §4).
- US3.11 (Journey 5, ADR-2): narrow Alice's grant from a two-scope to a one-scope permission
  set and replace her provider session with a one-scope token. After the permit deadline
  (`max_staleness: 2s`, bounded `Eventually`), the body and header-only paths forward the
  one-scope session's token, not the cached two-scope token; the broker exchange count rises by
  one per path; policy sees the narrowed `resource.scopes`. A re-exchange failure forwards
  nothing (contract §3.1).

**Complementary unit gates** (historical T049–T053 plus new T111–T113/T119, run by `just extproc-test`): synthetic broker replies
exercise missing, zero, malformed and duplicate `max-age`, invalid typed context, and unknown
members. Controlled-clock and synchronization tests cover a response received after its original
deadline and expiry during exchange, body buffering or batch processing. Assert 503 before the
expired decision reaches policy, with no forwarding or re-resolution. These checks complement
the real-broker journeys above; they are not extra acceptance scenarios or evidence that the
real broker emits malformed responses.

**ADR-2 unit gates** (`just extproc-test`, `-race`): binding equality by scope set and instant,
compare-and-delete under concurrency, one re-exchange per request, no stale-on-error fallback to
a removed token, unbound-token binding, and unchanged disabled mode and cooldown entries
(contract §3.1, data model §3.6).

## 4. ExtProc startup journeys (US4.1–US4.4)

```bash
ginkgo -v --label-filter="delegation-context-startup" ./tests/e2e/extproc/
```

**Expected**: The disabled default makes zero discovery or evaluation requests to the mock
broker. Each of these conditions stops startup with the error substring from
[contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md) §1:

- `authorization.enabled: false`
- discovery failure
- an identifier mismatch

The mismatch case makes **zero** evaluation requests.

## 5. Manual smoke test (optional, local compose)

1. Start the stack with `just compose-extproc-up`.
2. Add to `configs/config.extproc.docker.yaml`:
   `delegation_context: {enabled: true}` and `authorization.enabled: true`. Use a policy that
   keys on canonical IDs.
3. Configure canonical IDs `data-assistant`, `repo-reader`, and `repo-api` through the admin API.
   Grant consent in the SPA.
4. Send an MCP `tools/call` through agentgateway. **Expected**: The request is allowed.
5. Revoke consent in the SPA. Wait 60 s and repeat the request. **Expected**: 403
   `access_denied` with the consent URL.

## 6. Full gate

Run the non-container recipes of `just verify` individually, not its container-inclusive
aggregate. Tools are preinstalled; rerun installation only for tool-version changes.

```bash
just check
bash "$SECURITY_SCAN_SCRIPT" .  # workflow-prepared default-branch scan; not just security
just test
just extproc-test
just web-test
just cdk-test
just mock-sample-agent-test
just mock-upstream-oauth2-test
just test-integration          # self-contained only
just docs-build
just test-e2e-backend           # E2E is the final local guard layer
# Also run the three focused feature labels in §§2–4 and these non-container regressions:
ginkgo -v --focus="OPA Authorization|ExtProc Metadata Input" ./tests/e2e/extproc/
just test-e2e-frontend
```

Defer `just test-e2e-docker`, `just test-integration-infra` and all Agentgateway container
cases to downstream tests on the pushed commit. Do not invoke `just test-e2e`, unfiltered
`just test-e2e-extproc`, `just test-integration-all` or `just verify` in the sandbox. Container
absence does not block independent local tasks and is never a validation pass.

**Expected**: All required local checks pass and downstream evidence is recorded separately. The existing suites `token_exchange_test.go`,
`grant_diagnostics_test.go`, `tests/e2e/extproc/opa_authorization_test.go`, and
`extproc_approval_journey_test.go` pass unchanged, which proves SC-006 compatibility.

## 7. Implementation evidence (2026-10-30, interactive `/speckit-implement` session)

### Approval provenance (T001)

- Approval comment: <https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6504077937>
  for commit `a5042d5`, covering all eleven items (A1–A11 = API-1–API-6, ADR-1, D-1–D-4). It
  carries one 👍. The earlier pipeline implement run
  (<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6522572132>)
  received the approval as `context.approval` and named `mjungsbluth` as the approver. This session
  could not read reaction authors or times through its GitHub tools, so the recorded date is
  2026-10-30, the day of the `/speckit-implement` command
  (<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6520726517>).
- ADR 045 moved to `adrs/045-authzen-delegation-pip.md` (045 was unused) with
  `Accepted (approved by mjungsbluth on 2026-10-30: <url>)`. The draft covers every point T001
  lists.
- **Same-PR revision of API-2 (needs maintainer confirmation in review)**: T013 found that AuthZEN 1.0
  requires receivers to ignore unknown request fields. The user running this session chose
  "follow the standard: ignore unknown members and `properties`" and "keep the approved
  `ErrorResponse` body". ADR 045 §1, its revision history, the OpenAPI and the docs record this.
  It is user direction in this session, not the pipeline approval. The maintainer confirmed it on
  2026-10-31 (see "Maintainer acceptance" below).

### Phase 0 handoff (T002–T005)

The historical implementation record described a separate-PR requirement. The user
running that session kept the extraction in this PR as its own commit:
`b882d7b refactor(tokenexchange): extract shared DelegationResolver (exchange steps 2–9a)`.
This was recorded as a deviation at that time, not a maintainer-arranged handoff. The current
constitution explicitly requires an isolated no-behavior-change commit ahead of feature commits
in the same PR, not another PR. Retain the commit and its existing regression evidence.

- Baseline and regression: `go test ./internal/domain/tokenexchange/` passed after the extraction.
  The only test edits are receiver changes (`svc.resolveEffectiveScopes(...)` →
  `svc.DelegationResolver().resolveGrantCoverage(...)` with one extra ignored return); no
  assertion changed. The exchange E2E regression files (`token_exchange_test.go`,
  `grant_diagnostics_test.go`, `local_token_exchange_e2e_test.go`,
  `extproc_approval_journey_test.go`) ran 58/58 green after the extraction.

### API-003 verification (T013)

Source: the AuthZEN Authorization API 1.0 source text in the OpenID `authzen` repository
(`api/authorization-api-1_0.md`, fetched from `raw.githubusercontent.com` over verified TLS;
`openid.net` is blocked by this sandbox's network policy).

| Item | Standard | Contract |
|---|---|---|
| Metadata members | `policy_decision_point` (REQUIRED, https, no query/fragment), `access_evaluation_endpoint` (REQUIRED); absent optional endpoints mean "not supported" | Matches; batch/search omitted |
| Well-known path | Inserted between host and path: `https://pdp.example.com/.well-known/authzen-configuration/tenant1` | ExtProc `DiscoveryURL` implements this; the broker serves the root path, and path-prefixed deployments route it through the proxy (docs) |
| Identifier check | Advertised `policy_decision_point` MUST equal the identifier used; otherwise the data MUST NOT be used | Exact string comparison before any credential |
| Request identification | If the PEP sends `X-Request-ID`, the PDP MUST return the same value | Echoed on every path |
| Decision `context` | Implementation-defined; reason examples are non-normative | Denial form (a) kept |
| Unknown fields | "receivers MUST ignore unknown fields" | **Conflict with approved R10**; resolved by session direction (above) |
| Error bodies | Table says "An error message string" | Approved `ErrorResponse` object kept (session direction) |
| `Content-Type` | Requests MUST be `application/json` | Enforced (400 otherwise) |

### Red evidence (T027, T036, T044, T053, T065)

- Broker acceptance (28 in-memory scenarios): run against the pre-feature Phase 0 commit `b882d7b`
  in a separate worktree with only the new test files added. Result: `0 Passed | 28 Failed`.
  Every failure was a semantic assertion: 14 × missing `X-Request-ID` echo from the absent route,
  404/405 statuses, and the contract version `1.1.0` instead of `1.2.0` (US4.6). None was a
  compile or bootstrap failure.
- US4.6 was also red against the implemented code until the OpenAPI was updated (`1.1.0`).
- Domain, HTTP, ExtProc and consumer/startup E2E tests were written alongside their
  implementation in this session, not strictly before it. Their red phase was not separately
  recorded. One domain test did catch a real defect during development: evaluation did not check
  caller cancellation before resolving.

### Green results

| Command (run directly; `just` is not installed here) | Result |
|---|---|
| `gofmt -s -l` on tracked Go files | Only `internal/domain/oauth2/service_test.go`, which this feature does not touch (pre-existing) |
| `go vet ./...` | Clean |
| `golangci-lint run ./...` (v2.14.0, go1.27.3) | 0 issues |
| `go test -race <GO_FAST_TEST_PACKAGES>` | All pass except `TestBuildAWSConfig_DisableSSL`, `TestNewGuardedTransport_BlocksPrivateDialsAndBypassesProxy`, `TestFetcher_SSRF_BlocksLoopbackIPv6`; the same three also fail on the untouched base `a5042d5` (sandbox has no IPv6, and the AWS SDK rejects the proxy CA bundle) |
| `go test -race ./internal/extproc/... ./cmd/extproc-token-exchange/...` | Pass |
| `ginkgo --label-filter="!performance && !docker" ./tests/e2e/` (after `cd web && npm ci && npm run build`) | 950/950 passed |
| `ginkgo --procs=1 --label-filter="docker" ./tests/e2e/` | 8/8 passed, including US2.19 on real PostgreSQL |
| `ginkgo --label-filter="authzen-delegation" ./tests/e2e/` | 29 broker scenarios pass (28 in-memory + US2.19 on PostgreSQL) |
| `ginkgo --label-filter="delegation-context" ./tests/e2e/` | 11/11 passed |
| `ginkgo --label-filter="delegation-context-startup" ./tests/e2e/extproc/` | 4/4 passed |
| `ginkgo ./tests/e2e/extproc/` | 102 passed, 5 failed: all agentgateway container scenarios, which fail to pull `cr.agentgateway.dev/agentgateway:v1.5.0` (registry 403 from the sandbox network policy) |
| `cd assets/docusaurus && npm run build` | Pass |

Not run: the frontend Playwright suite and `just verify`; `just` itself is unavailable.

### Scenario-to-test mapping

US1.1 `tests/e2e/authzen_delegation_test.go:33`; US1.2 `:45`; US1.3 `:69`; US1.4 `:78`; US1.5 `:88`;
US1.6 `:99`; US1.7 `:135`; US1.8 `:114`.
US2.1 `tests/e2e/authzen_delegation_denial_test.go:23`; US2.2 `:30`; US2.3 `:38`; US2.4 `:47`;
US2.5 `:55`; US2.6 `:63`; US2.7 `:76`; US2.8 `:85`; US2.9 `:97`; US2.10 `:105`.
US2.11 `tests/e2e/authzen_delegation_errors_test.go:29`; US2.12 `:38`; US2.13 `:45`; US2.14 `:53`;
US2.15 `:63`; US2.16 `:72`; US2.17 `:98`; US2.18 `:83`; US2.19 `:144`; US2.20 `:90`.
US3.1 `tests/e2e/delegation_context_journey_test.go:105`; US3.2 `:114`; US3.3 `:122`; US3.4 `:132`;
US3.5 `:143`; US3.6 `:169`; US3.7 `:184`; US3.8 `:203`; US3.9 `:225`; US3.10 `:236`; US4.5 `:248`.
US4.1 `tests/e2e/extproc/delegation_context_startup_test.go:33`; US4.2 `:57`; US4.3 `:72`; US4.4 `:90`.
US4.6 `tests/e2e/authzen_delegation_compat_test.go:25`.

### Findings for the maintainer

- **F-1 (US3.5, body path)**: The delegation resolver never caches denials, and the header-only path
  permits immediately after consent. On the body path, however, the concurrent exchange during
  the denied request receives the broker's re-consent error. The existing ADR 012 exchanger then
  caches that error for its 5 s re-consent cooldown (`reAuthCooldownTTL`). A body request right
  after consent is therefore still answered from that exchange cooldown for up to 5 s. Changing
  exchange-cache semantics is outside the approved scope, so the US3.5 test asserts the immediate
  header-only permit, the immediate broker re-consultation on the body path, and body-path
  forwarding once the existing cooldown lapses. A maintainer decision is needed if the body path
  must also permit immediately (for example, skipping the re-consent cooldown in enabled mode).

### Design and scope verification (T008, T016, T017)

- The data model holds: no new entity, repository, migration or domain event; evaluation is
  read-only. The PostgreSQL outage journey (US2.19) ran on real PostgreSQL.
- No UI: the existing consent and revocation flows are reused. No design-system components,
  Wireloom sequences, Playwright tests or screenshots are added.

### Maintainer acceptance (2026-10-31)

The maintainer accepted the four items raised at the end of the implementation session:

1. The API-2 revision: unknown request members, AuthZEN `properties` and other `context`
   members are ignored per AuthZEN 1.0, and error bodies keep `ErrorResponse`.
2. Finding F-1: on the body path, consent after a denial takes effect once the existing ADR 012
   5 s re-consent cooldown lapses; the exchange cache stays unchanged (recorded in ADR 045
   Consequences).
3. The approval record in ADR 045 (mjungsbluth, 2026-10-30, approval comment
   `issuecomment-6504077937`).
4. The Phase 0 resolver extraction as commit `b882d7b` in this PR instead of a separate PR.

With item 1 confirmed, the four spec journeys were rechecked against the shipped OpenAPI. Journey
3's statuses, `X-Request-ID` echo and `no-store` match, and its 401 excerpt leaves the body to the
OpenAPI. The spec excludes `context.actor` support, which ignoring it satisfies. Journeys 1, 2 and 4
match the implemented permit, denial and `input.context` shapes (T028, T079).

### Historical task status at the end of the 2026-10-31 session

The continuation record below supersedes this table where tasks have since been completed.

| Task | Reason | Needed to close |
|---|---|---|
| T027, T036, T053, T065, T080, T094, T097 | Semantic-red evidence was recorded for the 28 in-memory broker scenarios only; unit tests and the consumer/startup E2E tests were written alongside the implementation | Historical proposal, superseded by §10: a retrospective rerun cannot establish test-first order. D-6 requires explicit exception sign-off plus alternative validation, not acceptance of evidence alone. |
| T029–T031 | No separate compile-only scaffolding step was done; the code was implemented directly | Historical proposal, superseded by §10: not automatically inapplicable or performed; retain noncompliance and use the explicit D-6 disposition conditions. |
| T044 | The deferred-holder matcher test and credential-free audit assertions exist; the full trace-ID equality across context, audit, access log and `traceresponse` for evaluation is not asserted end to end | Add the composed HTTP-harness assertion |
| T059 | ExtProc logs carry `operation=delegation_resolution`, `delegation.cache` hit/miss and bounded categories, but no test captures server log output | Add captured-log tests |
| T064 | US4.6 asserts the deprecation marker in E2E; no separate `authzen_compat_test.go` unit test | Add the unit test or accept the E2E coverage |
| T071, T099, T100 | The agentgateway container image cannot be pulled here (403), so 5 ExtProc E2E specs (including the agentgateway `metadata_input_test.go` scenario) fail; the frontend Playwright suite and `just verify` were not run; three unrelated unit tests fail on the base commit in this sandbox | Run `just verify` on a capable runner |
| T091 | Depends on T059 | As above |

Pre-extraction baseline (T003): on base `a5042d5`, `go test ./internal/domain/tokenexchange/` passed and the four exchange regression files ran 58/58 green. The results after the extraction were identical.

## 8. Implementation continuation (2026-11-04)

### Outcome

Closed the remaining observability, composed audit-correlation and unit schema-compatibility
work (T044, T059, T064, T091), and reran the named non-container regression journeys (T099).
The implementation phase is **blocked**, not complete: the static gate fails outside the
feature, the security scanner cannot compile, and the historical test-first/scaffolding tasks
remain unfulfilled. No test, check, approval or constitutional requirement was waived.

### Approval boundary

The supplied approval identifies `mjungsbluth`, `2026-10-30T10:40:47Z`, and
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6504077937>.
ADR 045 already records that provenance. This run records no new approval and changes neither
`spec.md` nor `plan.md`.

The inherited API-2 revision and body-path cooldown are recorded as accepted in the historical
section above and in `adrs/045-authzen-delegation-pip.md:44-49,151-156,162-172`. However, the
supplied approval list still names a strict-member request contract, while the implementation,
OpenAPI and revised contract ignore unknown members. The data model §2.5 and research R10
also retain rejection language. No new slash-command direction or approval for the later
revision was supplied in this run. Maintainer approval coverage and the frozen-artifact
inconsistency need reconciliation; this run makes no further change to that request behavior
or to the existing exchange cooldown.

The 2026-11-05 planning run reconciled these artifacts at the maintainer's direction: spec
FR-003, research R10, data model §2.5 and the evaluation contract §2.1 now ignore unknown
request members, matching ADR 047 §1 and the OpenAPI. The revised API-2 and the body-path
cooldown (D-5) are registered in the plan's Approval Items for sign-off.

### New red-to-green evidence

- `go test ./internal/extproc/delegation ./internal/extproc/server -run
  'TestResolutionTelemetry|TestDelegationLogs'` compiled and failed semantically before the
  observability changes: all five resolution cases emitted zero spans instead of two/three,
  and permit hit/miss emitted no JSON log record. Existing denial/error log cases passed.
- Added `extproc.delegation.resolve` and `extproc.delegation.evaluate` spans with bounded
  operation, outcome/detail, cache and error categories, plus Debug permit logs. The identical
  tests then passed with `-race`. Captured spans/logs exclude subject/assertion/provider
  credentials, context, requested resource URI and dependency error text.
- Composed production HTTP tests decorated the real evaluation service only to observe its
  capture holder. Five existing acceptance scenarios failed because the access-log filter
  excluded `/access/`: only the audit record, not the required audit/access pair, existed.
  Adding `/access/` to the shared perimeter filter made all 28 non-container broker scenarios
  pass. Assertions now bind the finalized context to both logs and `traceresponse`, preserve
  verified Actor/distinct CallingPeer on permit and caller-CEL denial, omit an equal peer,
  check anonymous fallback on malformed/missing/invalid credentials, and reject a second
  finalization attempt. No production test-only API was added.
- Added broker span credential-exclusion coverage and the unit OpenAPI compatibility test.
  These already-existing behaviors passed immediately; no artificial red result is claimed.
  The existing real-loader example table already includes the delegation-context example.

### Current validation

| Command | Result |
|---|---|
| `just test` | Pass, including new unit tests, with race coverage |
| `just extproc-test` | Pass, with race coverage |
| `just test-e2e-backend` | 950/950 selected scenarios pass, including unchanged exchange, diagnostics, local exchange and approval journeys |
| `ginkgo --label-filter='authzen-delegation && !docker' ./tests/e2e/` | 28/28 pass; US2.19 explicitly deferred |
| `ginkgo --no-color --label-filter="delegation-context" ./tests/e2e/` | 11/11 pass, including revocation/outage, both policy paths, canonical UUID parity and origin discovery |
| `ginkgo --no-color --label-filter="delegation-context-startup" ./tests/e2e/extproc/` | 4/4 pass |
| `ginkgo -v --procs=2 --skip="Agentgateway" ./tests/e2e/extproc/` | 102/102 selected scenarios pass, including unchanged OPA and metadata-input coverage; 17 Agentgateway scenarios excluded as container-backed |
| `just test-e2e-frontend` | 67/67 pass |
| `just docs-build` | Pass |
| `just web-test` | 262/262 pass |
| `just cdk-test` | Pass |
| `just mock-sample-agent-test mock-upstream-oauth2-test` | Pass |
| `just test-integration` | Pass, self-contained suites only |
| `just check` | Formatting and vet pass; lint fails at `cmd/extproc-token-exchange/root_lifecycle_test.go:180` (`errcheck`, unchecked subprocess result). The offending code predates the feature; `origin/main` already contains an assertion for that result. No unrelated code changed here. |
| `just security` | Scanner compilation terminated with `signal: killed` for the OpenAI and Anthropic SDK packages; retry with `GOMAXPROCS=2` failed the same way. No vulnerability-scan pass is claimed. |
| `just verify` | Stops at the same lint failure; all other non-container recipes were attempted independently. |

`just test-e2e` and the unfiltered `just test-e2e-extproc` include containers and were not
invoked. US2.19, `just test-e2e-docker`, `just test-integration-infra` and all Agentgateway
container scenarios are deferred to downstream container tests, not treated as sandbox
blockers and not reported as validated. The earlier section's container results are historical
records, not results from this continuation.

### Remaining unchecked tasks

| Tasks | Why they remain |
|---|---|
| T027, T036, T053, T065, T080, T094, T097 | The prior session explicitly says unit and consumer/startup tests were written alongside implementation. New red/green evidence above does not establish historical test-first compliance for that work. Proposed D-6 (§10) supplies a concrete exception/alternative-validation proposal; these remain unchecked until its sign-off and applicable conditions are met. |
| T029–T031 | The prior session implemented behavior directly instead of the planned compile-only scaffolding. Removing working behavior now would not establish the missing historical sequence. Retain that noncompliance; Proposed D-6 (§10) governs disposition, not retrospective performance of these tasks. |
| T071 | Startup/discovery/compatibility, ExtProc unit and non-container E2E, and docs checks pass, but its `just check` prerequisite still fails outside this feature. Container portions are deferred downstream. |
| T100 | Full validation remains incomplete because of the baseline lint failure and scanner compilation failure; container-only recipes remain downstream. |

No new frontend artifacts, dependencies, migrations, runtime configuration or API behavior were
introduced by this continuation. The existing isolated refactor commit remains unchanged.

## 9. Current task reconciliation

The task-generation update retains all 101 stable task IDs and historical evidence, without
marking any implementation work complete. Reopened tasks cover supplied approval verification
(T001), revised API-2 request tests/decoding (T043/T046), revised US3.5 coverage and validation
(T024/T060), and affected documentation/compliance gates. Missing historical test-first and
compile-only sequencing remains unfinished; a retrospective stub or later green run cannot
establish past compliance. No approval or exception is recorded by task generation.

Current requirements are in `spec.md`, `plan.md` and the contracts, not stale historical
strict-rejection descriptions or old separate-PR handoffs. API-003 verification above remains
valid. Revised API-2 and D-5 are Proposed for sign-off; implementation must use its supplied
`context.approval` and preserve ADR 047's existing provenance. All applicable non-container
work continues independently of downstream container evidence. This section supersedes only
obsolete directions in earlier entries, not their reported observations or failures.

## 10. Proposed historical sequencing disposition (D-6)

Analysis run `42039632078/1` identified the unresolved historical VIII/XIII deviation.
[historical-validation-exception.md](./historical-validation-exception.md) now records the
specific admitted sequencing failure, its scope, rationale, residual risk, main alternative
and conditional validation proposal under “When Constraints Cannot Be Met”. Plan approval
item D-6 requests a one-time exception for that history only; no approval is recorded here.

The recommended alternative validation is independent review of existing tests against the
requirements, a verified 44-scenario mapping, the unchanged required local gates and
commit-specific downstream container results, with genuine test-first coverage for every
new correction. Original sequencing remains recorded as not performed even if approved.
A later passing suite, replay against a pre-feature commit, or replacement with stubs cannot
make that history compliant. Declining D-6 leaves those obligations unresolved and prevents
a claim of feature completion.

T027, T029–T031, T036, T053, T065, T080, T094 and T097 remain unchecked. Only an implementation
session with supplied approval explicitly covering D-6 may record its provenance and the
applicable alternative-validation evidence before disposing of those tasks as an approved
exception, not as performed test-first development. D-6 waives no failing check or remaining
validation and does not apply to future corrections. Earlier section §7 suggestions to accept
existing evidence alone or mark scaffolding not applicable are superseded by these conditions.
The plan's constitutional gate now distinguishes design coverage from unresolved historical
execution. API-003's TLS-download failure is explicitly historical; §7 retains the completed
final-standard check and ignore-unknown-fields outcome.

## 11. Approved implementation continuation (2026-11-05)

### Supplied approval (T001)

`context.approval` supplies `mjungsbluth`, `2026-11-05T16:38:49Z`, and
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6661347255>.
It covers the thirteen items listed by reanalysis: API-1–API-6, ADR-1 and D-1–D-6,
matching every row of `plan.md`'s Approval Items. In particular it covers revised API-2
(ignore unknown fields), D-5 (unchanged 5 s body-path exchange cooldown), and D-6 (the
conditional historical sequencing exception). ADR 047's Status and Revision history record
the supplied approval while preserving its original approval. No draft move is needed:
ADR 047 is already in `adrs/` and is not on the default branch.

D-6's original test-first/compile-only obligations were **not performed; covered by approved
D-6**, subject to its independent review and local/downstream validation conditions. Approval
alone does not complete T027, T029–T031, T036, T053, T065, T080, T094 or T097.
New corrections remain test-first. No existing validation or security guarantee is waived.

### Reanalysis findings I1 and I2

- **I1 — frozen plan status inconsistency**: `plan.md:650-654`'s blanket “No constitution
  violation needs justification” is obsolete. The historical VIII/XIII deviation needs the
  approved D-6 disposition and all alternative-validation conditions in
  `historical-validation-exception.md:49-78`. Superseding ADR 028 through ADR 047 independently
  follows Principle II and is not a violation. This correction is recorded here rather than
  editing `plan.md`, which the implementation phase must keep unchanged. The source wording
  remains a documentation follow-up; this note does not claim that it was edited.
- **I2 — historical standards-check notes**: the checklist notes now distinguish the earlier
  TLS download failure from §7's completed final-standard verification and denial form (a).
  Its review markers are unchanged. The current revised-contract approval is recorded above,
  separately from research verification and historical approval.

### Independent validation review and corrections (D-6 conditions 1–2)

Reviewed the feature tests against FR-001–FR-019, SR-001–SR-006 and both contracts, rather
than using a green run as evidence of historical test-first development. The review retained
all existing exchange regression assertions. Concrete coverage and corrections:

| Requirement / invariant | Test evidence (repository-relative) | Review outcome |
|---|---|---|
| Projection, canonical omission, grant/service ordering, effective ceiling, batched metadata and expiry | `internal/domain/tokenexchange/evaluation_test.go:173-277` | Explicit expected context, lookup counts, schema minima and controlled subsecond/expired boundary; no session dependency introduced. |
| Token-derived agent anchor before grant reads, exact supported values, caller CEL reuse, denial/error classification | `internal/domain/tokenexchange/evaluation_test.go:279-459` | Matching/mismatched/non-UUID anchors, zero grant-read assertions, real signed credentials/CEL, allowlisted grant detail and wrong-stage rejection. |
| JSON shape, required types, Content-Type, bounded body and ignore-unknown invariance | `internal/adapters/http/enduser/authzen_evaluation_test.go:192-364` | Added null-context and every required member's missing/null/wrong-type cases; made overflow body otherwise valid, added exact-64-KiB acceptance, and compared domain requests/assertions/responses with and without ignored fields at every level. Existing handler already satisfies these; no artificial red claim or request-behavior change. |
| Response statuses, reasons, caching, request ID, credential-free logs/spans | `internal/adapters/http/enduser/authzen_evaluation_test.go:81-190,366-388` | Permit/deny/error assertions, challenge matrix and absent-ID behavior remain green. |
| Shared audit/perimeter identity and trace seam | `internal/adapters/http/middleware/security_context_test.go:496`, `tests/e2e/authzen_fixture_test.go:228-271`, `tests/e2e/authzen_delegation_test.go:46`, `tests/e2e/authzen_delegation_errors_test.go:54-108` | Deferred holder plus production composed actor/peer/trace assertions on permit, policy denial and early fallback retained; §8 holds the original correction's red/green evidence. |
| Producer/schema/consumer parity and no unknown fields reaching policy | `internal/extproc/delegation/client_test.go:90-303`, `internal/extproc/authorization/input_builder_test.go:429-460` | Expanded required-string and optional metadata matrix at every nesting level. Found and fixed null scope-element coercion; empty arrays and omitted optionals remain accepted. |
| Single request-start deadline, fail-closed freshness, independent permit cache | `internal/extproc/delegation/client_test.go:116-173`, `internal/extproc/delegation/resolver_test.go:57-213` | Clock-read count, bad/duplicate max-age, exact-deadline misses, late response, unchanged singleflight deadline, denial/error noncaching, eviction and shutdown assertions retained. |
| Body precedence, header-only ordering, expiry and immutable mapping | `internal/extproc/server/delegation_test.go:139-394` | Replaced the precedence sleep with completion-order channels; added deny over success/cancellation and resolution error/timeout/cancellation precedence in both orders. Added expiry while buffering, complementing exchange-wait, batch and header-only expiry tests; zero policy/re-resolution assertions. |
| Direct typed policy input and unchanged disabled mode | `internal/extproc/authorization/input_builder_test.go:429-460`, `internal/extproc/server/delegation_test.go:139-163,299-394` | Exact enabled JSON, byte-for-byte disabled JSON, deep-copy checks, permit-derived compatibility fields and session identity preserved. |
| Config defaults/precedence/validation and example loading | `internal/extproc/config/validate_delegation_test.go:27-119`, `internal/extproc/config/loader_test.go:85-156` | All four settings use the existing loader; enabled-only rules, origin/port and explicit path, example-file validation retained. |
| Startup discovery, exact identifier, credential exclusion and lifecycle | `internal/extproc/delegation/discovery_test.go:15-89`, `cmd/extproc-token-exchange/root_delegation_test.go:66-133`, `tests/e2e/extproc/delegation_context_startup_test.go:34-104` | Real startup errors precede listening; discovery has no bearer credential, mismatch has zero evaluation calls, advertised endpoint and shutdown are exercised. |
| Bounded observability and credential exclusion | `internal/extproc/delegation/telemetry_test.go:19-95`, `internal/extproc/server/delegation_telemetry_test.go:28-69` | Captured client/resolver spans and permit hit/miss/deny/error logs exclude tokens, context, principals, resource URIs and dependency causes. |
| Deprecation without response-field removal | `internal/adapters/http/enduser/authzen_compat_test.go:14-57`, `tests/e2e/authzen_delegation_compat_test.go:26-46` | `1.2.0`, deprecated permission map shape and unchanged principal/agent fields asserted through schema and actual exchange. |

**New red/green evidence (T102)**: before changing `context.go`,
`go test ./internal/extproc/delegation -run TestClient_TypedContextValidation -count=1`
compiled and failed exactly two assertions: `null_resource_scope_element` and
`null_service_scope_element` expected an error but got nil. Go's `encoding/json` silently
converted null array entries into empty strings in `[]string`. The wire decoder now uses
string pointers and validates each entry before constructing typed scope slices. The same
tests then passed with `-race`; the full race suites and E2E below pass. This enforces the
already-approved string-array contract; it adds no API, dependency, configuration or decision.

**US3.5 fixture correction (T024)**: the old single consumer's header-only permit had already
cached an exchanged token before its body-path revocation, so the body exchange did not
necessarily encounter non-consent and the test did not prove the cooldown. The revised single
`It()` uses a separate fresh production consumer for the body half. It observes denial,
renewed consent, immediate broker evaluation count increment, the existing 403
`WWW-Authenticate: Bearer error="insufficient_scope"` recovery challenge during the cooldown,
and eventual forwarding after that cooldown. Header-only forwarding and re-consultation
remain immediate. An initial assertion incorrectly looked for this existing exchange
challenge's URI in a flat JSON body; it was corrected to assert the documented challenge
header, not to change or weaken production behavior. No historical or new application TDD
claim is made for this test-only fixture correction.

### Verified 44-scenario mapping (D-6 condition 2; T096)

Each reference below is the `It()` line, checked against its adjacent scenario comment.
The six files contain 44 distinct scenario blocks, with no pending/focused/skip markers.
Fresh broker fixtures and production bootstrap are used; the consumer suite composes the
real broker with production ExtProc; startup tests use the shared production setup with a
counting metadata provider. No scenario or acceptance assertion was removed.

| File | Scenario → `It()` line |
|---|---|
| `tests/e2e/authzen_delegation_test.go` | US1.1 → 34; US1.2 → 46; US1.3 → 70; US1.4 → 79; US1.5 → 89; US1.6 → 100; US1.7 → 136; US1.8 → 115 |
| `tests/e2e/authzen_delegation_denial_test.go` | US2.1 → 24; US2.2 → 31; US2.3 → 39; US2.4 → 48; US2.5 → 56; US2.6 → 64; US2.7 → 77; US2.8 → 86; US2.9 → 98; US2.10 → 106 |
| `tests/e2e/authzen_delegation_errors_test.go` | US2.11 → 30; US2.12 → 39; US2.13 → 46; US2.14 → 54; US2.15 → 64; US2.16 → 73; US2.17 → 99; US2.18 → 84; US2.19 → 145 (downstream, `docker`); US2.20 → 91 |
| `tests/e2e/delegation_context_journey_test.go` | US3.1 → 106; US3.2 → 115; US3.3 → 123; US3.4 → 133; US3.5 → 144; US3.6 → 174; US3.7 → 189; US3.8 → 208; US3.9 → 230; US3.10 → 241; US4.5 → 253 |
| `tests/e2e/extproc/delegation_context_startup_test.go` | US4.1 → 34; US4.2 → 58; US4.3 → 73; US4.4 → 91 |
| `tests/e2e/authzen_delegation_compat_test.go` | US4.6 → 26 |

The complete assertions—not just identifiers—were reviewed: ten separate grant-denial
journeys, request/authentication/caller-policy failures, expiry and recovery, canonical-ID
policy across two UUID sets, fresh-cache isolation, discovery mismatch and retained exchange
fields. This establishes current acceptance coverage, not original test-first order.

### Local validation (D-6 condition 3; T060/T071/T098/T100)

Validated code revision: `02832f1e2a060060840c66041fdde479911b8e6d`, based on supplied
PR head `5f1c21b81ba241b9d64affddb7d681756416985c`. The later evidence commit only updates
feature documentation and task status. Commands below all passed in this session. E2E was
rerun as the final local guard after the decoder correction and strengthened assertions.

| Command | Observed result |
|---|---|
| `just check` | Formatting, vet and lint pass; 0 issues. The earlier baseline lint failure is not reproduced on this head. |
| `just test` | Pass with the recipe's race coverage, including domain, HTTP and builder/routing regressions. |
| `just extproc-test` | Pass with `-race`, including decoder, cache, precedence, buffering and startup tests. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Prepared default-branch scan passes: gosec, govulncheck and manifest OSV checks; repository exceptions unchanged. No scanner rebuild or weakened rule. |
| `just web-test` | 29 files, 261 tests pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `just test-integration` | Self-contained suites pass. |
| `just docs-build` | Pass. Existing build warnings remain non-fatal; no dependency-lock update retained. |
| `just test-e2e-backend` | 985 selected scenarios pass; 12 performance/container scenarios excluded by the existing recipe. Includes unchanged exchange/diagnostics/local-exchange/approval regressions. |
| `ginkgo -v --no-color --label-filter="authzen-delegation && !docker" ./tests/e2e/` | 28/28 pass; US2.19 remains downstream. |
| `ginkgo -v --no-color --label-filter="delegation-context" ./tests/e2e/` | 11/11 pass, including the strengthened US3.5 and both canonical UUID fixtures. |
| `ginkgo -v --no-color --label-filter="delegation-context-startup" ./tests/e2e/extproc/` | 4/4 pass. |
| `ginkgo -v --no-color --procs=2 --skip="Agentgateway" ./tests/e2e/extproc/` | 102/102 non-container scenarios pass; 17 Agentgateway scenarios excluded, not validated. Includes unchanged OPA and metadata-input coverage. |
| `just test-e2e-frontend` | 67/67 pass; no frontend code or screenshot changes retained. |

The earlier broad `--focus="OPA Authorization|ExtProc Metadata Input"` selection also matched
Agentgateway cases, which the existing harness skipped. It is not container validation. The
explicit `--skip="Agentgateway"` final run above supplies the non-container regression record.
No runtime or CI configuration was changed to obtain a pass.

### Remaining obligations and outcome

All independent local implementation and validation work is complete: 92 of 102 tasks are
checked. No approval is missing. The following ten historical tasks remain unchecked:
T027, T029–T031, T036, T053, T065, T080, T094 and T097. Each original sequencing obligation
was **not performed; covered by approved D-6**, with closure still conditional on D-6's
commit-specific downstream validation (condition 4). Local review, the 44-scenario mapping
and passing local gates do not retrospectively establish that history or satisfy downstream
checks. The isolated `b882d7b` refactoring and unchanged regression assertions are retained.

US2.19/`just test-e2e-docker`, `just test-integration-infra` and Agentgateway container cases
are deferred to downstream container tests on the pushed commit, never claimed as local
validation. Their absence is not a sandbox infrastructure blocker and did not defer any
independently runnable local task. No current-commit downstream result was supplied.

The phase reports **blocked**, not feature completion: the requested I1 source edit remains
in frozen `plan.md:652-654`, and the conditional historical-task closure remains outstanding.
A permitted design-phase update must replace the blanket Complexity Tracking statement with
D-6's deviation/validation disposition; the implementation phase cannot edit that plan.
I2's historical-standards notes are corrected without changing any checklist markers.

## 12. Conditional validation closure (run 42087219381/1)

**Outcome**: All 102 tasks are closed, including the ten historical obligations through the
approved D-6 alternative-validation disposition. No application code, API, configuration,
security guarantee, test assertion or approved scope changed in this session.

### Approval and independent review

The supplied approval is unchanged: `mjungsbluth`, `2026-11-05T16:38:49Z`, on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6661347255>.
It covers all thirteen approval items, including D-6. ADR 047 already records this approval;
its accepted design and revision history need no further change.

D-6 conditions 1–2 use the independent contract-to-test review and concrete coverage table
in §11, including the genuine null-scope red/green correction. The 44 separate scenario
comments and `It()` locations were rechecked on the supplied head; the mapping in §11 is
unchanged and none of those six feature files contains skipped, pending or focused blocks.
US2.19 remains under `Label("docker")` at
`tests/e2e/authzen_delegation_errors_test.go:112-152`. Its production-bootstrap journey uses
real PostgreSQL, renames `user_grants`, asserts 500/no consent recovery and the bounded
repository-failure audit detail, and disposes of the database fixture. All existing exchange
regression assertions remain unchanged.

### Current local gate (D-6 condition 3)

Code and tests validated: supplied head `277fec06b8b0ae735a6cfbd01cd7b8468c140d52`.
This session changes only feature evidence and task disposition. All commands below passed;
E2E followed the static, unit, integration, docs and security gates as the final local guard.

| Command | Result |
|---|---|
| `just check` | Formatting, vet and lint pass; 0 issues. |
| `just test` | Fast Go/package tests pass with the recipe's race coverage. |
| `just extproc-test` | ExtProc and startup package tests pass with `-race`. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Prepared default-branch security scan passes; existing exceptions unchanged. |
| `just web-test` | 29 files, 261 tests pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `just test-integration` | Self-contained integration suites pass. |
| `just docs-build` | Pass; existing non-fatal build warnings remain. |
| `just test-e2e-backend` | 985 selected scenarios pass; 12 performance/container scenarios excluded by the existing recipe. |
| `just test-e2e-frontend` | 67/67 pass. |
| `ginkgo -v --no-color --procs=2 --skip="Agentgateway" ./tests/e2e/extproc/` | 102/102 selected non-container scenarios pass; 17 Agentgateway cases excluded locally. |
| `ginkgo -v --no-color --label-filter="authzen-delegation && !docker" ./tests/e2e/` | 28/28 pass. |
| `ginkgo -v --no-color --label-filter="delegation-context" ./tests/e2e/` | 11/11 pass, including re-consent/cooldown and canonical-ID parity. |
| `ginkgo -v --no-color --label-filter="delegation-context-startup" ./tests/e2e/extproc/` | 4/4 pass. |

### Downstream results on the supplied head (D-6 condition 4)

[CI run 42073656945](https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42073656945)
completed successfully on the exact supplied head
`277fec06b8b0ae735a6cfbd01cd7b8468c140d52`. These are downstream observations, not local
container validation:

| Job | Downstream evidence |
|---|---|
| [Container tests / Docker E2E tests](https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42073656945/job/127408080178) | `just verify-e2e-docker-junit` passed; log reports 9/9 selected specs, 0 failed, 0 pending. The recipe selects all backend `docker` scenarios, including US2.19. The 988 excluded non-docker scenarios are not container passes. |
| [Container tests / Infra integration tests](https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42073656945/job/127408080289) | `just verify-integration-infra-junit` step and job succeeded; JUnit artifact upload succeeded. This is the downstream counterpart of `just test-integration-infra`. No per-test count is asserted from the job status. |
| [ExtProc E2E tests](https://github.com/zalando-incubator/agentic-identity-broker/actions/runs/42073656945/job/127408072691) | Full ExtProc suite reports 119/119 passed, 0 failed, 0 pending, 0 skipped, including all 17 Agentgateway container cases excluded locally. |

The artifact inventory also lists `e2e-docker-test-results`,
`integration-infra-test-results` and `e2e-extproc-test-results`. Archive downloads returned
HTTP 404, so this record relies on the accessible GitHub job/step results and suite logs,
not an inspection of the unavailable XML archives. The Docker and ExtProc log summaries
establish that every selected scenario passed. No container runtime was used locally.
Any new pushed evidence-only commit remains subject to the workflow's downstream validation;
these results apply only to the supplied head, not to a future commit.

### Historical task disposition (D-6 condition 5)

For every row, the original sequence was **not performed; covered by approved D-6**. The
checked task marker records approved alternative-validation closure, never historical TDD
compliance or a retrospective scaffold replay.

| Task | Original obligation and closure evidence |
|---|---|
| T027 | Original 44-scenario test-first sequence unavailable. Retain the limited historical 28-scenario broker red run (§7); current 44-scenario review/mapping (§11), local 43-scenario gate and downstream US2.19 result above satisfy D-6 instead. |
| T029 | Compile-only broker domain scaffolding not performed; existing typed-domain contract review (§11) and current package/acceptance gates provide alternative validation. |
| T030 | Nonfunctional HTTP/builder/routing scaffolding not performed; request/response, deferred identity and production-bootstrap review (§11), package and broker E2E gates provide alternative validation. |
| T031 | Nonfunctional ExtProc scaffolding not performed; typed input/config/cache/order/startup review (§11), race tests and full local/downstream ExtProc journeys provide alternative validation. |
| T036 | Original broker unit semantic-red sequence not performed; independent broker test review and unchanged exchange regression gates remain the alternative, not a claim that a later green run was red evidence. |
| T053 | Original consumer unit semantic-red sequence not performed; independent client/cache/input/concurrency review, genuine later null-scope correction evidence (§11), race and journey gates satisfy the approved alternative. |
| T065 | Original startup/config semantic-red sequence not performed; independent defaults/precedence/discovery/credential-exclusion review, package and startup/full ExtProc gates satisfy the approved alternative. |
| T080 | Original full acceptance red sequence not performed; retain authentic limited red evidence, the 44 separate-scenario mapping and combined local/downstream acceptance results. |
| T094 | Original unit test-first sequencing not performed; retain isolated `b882d7b` refactor and unchanged-regression record (§7), independent review and the authentic later-correction red/green evidence (§§8,11). |
| T097 | Original acceptance test-first sequencing not performed; reviewed full journeys and current local/downstream green evidence close only the approved alternative-validation obligation. |

D-6's scope, residual risk and test-first requirement for all new corrections are unchanged.
The earlier §11 statement that current-head downstream evidence was absent is superseded by
these results. No task or checklist item remains unchecked.

### Frozen-plan wording (reanalysis LOW I1)

At this historical checkpoint, the frozen plan still contained the obsolete blanket statement that no constitutional violation needed justification. The authoritative D-6 document, supplied approval and this
closure record explicitly preserve the historical VIII/XIII deviation and its completed
alternative validation. ADR 047's partial supersession of ADR 028 separately follows
Principle II. This implementation leaves the frozen plan unchanged as required; it does not
claim the source wording was repaired. The remaining LOW wording inconsistency is disclosed,
not used to gate independently completed implementation or to conceal the original deviation.
The current tasks-phase design edit reconciles that summary in `plan.md`'s Complexity Tracking without changing any approved requirement, runtime behavior, historical exception or validation obligation.

## 13. Approved review remediation (run 42177932182/1 — historical)

The 2026-11-06 proposal selection by `mjungsbluth` at `2026-11-06T06:59:39Z` covers
F2–F6: <https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6669627407>.
This run applies only F4/F5/F6. Publication rejected F2's required
`coding-standards/OPENTELEMETRY.md` registry change as outside allowed paths. Its tested
implementation was withdrawn before publication rather than publish an unregistered namespace
or bypass the allowlist. F3 remains unchanged because filing its required follow-up issue is
unavailable: the workflow has no issue-creation safe output. T103/T104 record the remaining
work; no immediate-removal alternative or unassigned F1 decision is implemented. This is a
partly resolved review, not a claim that all findings are closed.

### New test-first evidence

Before production changes, the focused Go test run compiled and failed semantically:
`TestAuthZENEvaluation_AuditRecords` and `TestAuthZENEvaluation_SpansExcludeCredentialsAndContext`
expected the new `tokenexchange` identifiers; `TestDelegationLogs_ContainOnlyBoundedMetadata`
and `TestDelegationExpiredLog` expected security events absent from existing records;
`TestContextInput_SerializesDelegationMembersOnlyWhenPresent` and
`TestAccessEvaluation_PermitProjectsCredentialFreeContext` expected `service_id` rather
than `id`. F4/F6's assertions now pass. F2's implementation and test edits were restored to
the supplied head after the push rejection; their initial red/green evidence is retained
here only as publication-attempt history, not as evidence of a published F2 resolution.

After the approved OpenAPI-first service-member revision, the new
`TestDelegationContractConformance` compiled and failed against both producer and wire
context declarations on `.resource.id` versus `.resource.service_id`. It now passes and
compares all nested context fields plus the exact domain and ExtProc denial allowlists
against OpenAPI. No test-only production API or cross-process production import was added.

### Passing checks

- `just check` — format, vet and lint, zero issues.
- `just test` — fast Go/package suites with race detection.
- `just extproc-test` — ExtProc and command suites with race detection.
- `just test-integration` — self-contained integration, including contract conformance.
- `just test-e2e-backend` — 985 passed, 12 excluded; includes 28 broker and 11 consumer
  feature journeys (US2.19 is container-only).
- `ginkgo -v --procs=2 --skip=Agentgateway ./tests/e2e/extproc/` — 102 passed, 17
  container scenarios excluded; includes all four feature startup journeys.

Thus all 43 non-container acceptance scenarios pass. US2.19, `just test-e2e-docker`,
`just test-integration-infra`, and Agentgateway-backed ExtProc scenarios are deferred to
container tests on the pushed commit, not validated locally. No frontend behavior changed.
Optional before/after implementation commit hooks were skipped; no mandatory hook applies.
The tracked legacy `.specify/feature.json` is restored before publication and not committed.

## 14. Current remediation validation design (run 42232121230/1)

The revised design covers 47 scenarios: 29 broker, 14 consumer (US3.1–US3.13 and US4.5), and four startup. The local feature gate is 46 scenarios; US2.19 remains downstream container evidence. Historical §13 passing runs cover the earlier 44-scenario scope only.

- T111/T112 must first fail identity-based deletion/claim/delivery tests with barriers: equal-token different-generation replacements, failed compare-and-delete with compatible/incompatible successors, immutable shared first claim, detached handles and non-cacheable shared successes. Retry conflicts produce API-7 503 without a third exchange/rebind/stale fallback; expired final forwarding uses the existing unavailable 503.
- T110/T121/T122 each add one acceptance `It()` for US3.11/US3.12/US3.13; T113 records compiling semantic-red evidence before T114–T117. The first narrows the provider session, the second intentionally does not (same broader token under a new handle), and the third uses controlled broker barriers for differing grant reads and shared-result conflict on both paths.
- T103 tests the exact observability §3 target mapping and explicit kinds before implementation. Namespace registration and breaking changelog/query migration must be published through a permitted path; the current standard remains unmodified here. Historical green telemetry tests are not compliance evidence.
- T104 requires supplied ADR-3 approval and the maintainer-created issue URL before recording canonical-field ADR/docs changes. Both compatibility inputs stay unchanged today; no OPA removal/deprecation is tested or implemented here.
- T119/T120 require fresh applicable local/race/regression/full non-container gates for all three new acceptance scenarios and the revised observability/compatibility design. Record actual commands and file:line mappings only after implementation; no new red/green or runtime completion is claimed by this document phase.
