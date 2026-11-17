# Implementation Log: AuthZEN Delegation Policy Information Point

Evidence recorded by implementation runs before 2026-11-08, moved unchanged from
`quickstart.md`. Section numbers are kept, so a reference such as "quickstart
§12" points to §12 here. New runs report their evidence in the phase result
on the pull request, not in this file.

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
- ADR 048 moved to `adrs/048-authzen-delegation-pip.md` (renumbered from 045 and then 047, which main assigned to agent-subject impersonation and unique agent external IDs) with
  `Accepted (approved by mjungsbluth on 2026-10-30: <url>)`. The draft covers every point T001
  lists.
- **Same-PR revision of API-2 (needs maintainer confirmation in review)**: T013 found that AuthZEN 1.0
  requires receivers to ignore unknown request fields. The user running this session chose
  "follow the standard: ignore unknown members and `properties`" and "keep the approved
  `ErrorResponse` body". ADR 048 §1, its revision history, the OpenAPI and the docs record this.
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
   5 s re-consent cooldown lapses; the exchange cache stays unchanged (recorded in ADR 048
   Consequences).
3. The approval record in ADR 048 (mjungsbluth, 2026-10-30, approval comment
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
ADR 048 already records that provenance. This run records no new approval and changes neither
`spec.md` nor `plan.md`.

The inherited API-2 revision and body-path cooldown are recorded as accepted in the historical
section above and in `adrs/048-authzen-delegation-pip.md:44-49,151-156,162-172`. However, the
supplied approval list still names a strict-member request contract, while the implementation,
OpenAPI and revised contract ignore unknown members. The data model §2.5 and research R10
also retain rejection language. No new slash-command direction or approval for the later
revision was supplied in this run. Maintainer approval coverage and the frozen-artifact
inconsistency need reconciliation; this run makes no further change to that request behavior
or to the existing exchange cooldown.

The 2026-11-05 planning run reconciled these artifacts at the maintainer's direction: spec
FR-003, research R10, data model §2.5 and the evaluation contract §2.1 now ignore unknown
request members, matching ADR 048 §1 and the OpenAPI. The revised API-2 and the body-path
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
`context.approval` and preserve ADR 048's existing provenance. All applicable non-container
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
conditional historical sequencing exception). ADR 048's Status and Revision history record
the supplied approval while preserving its original approval. No draft move is needed:
ADR 048 is already in `adrs/` and is not on the default branch.

D-6's original test-first/compile-only obligations were **not performed; covered by approved
D-6**, subject to its independent review and local/downstream validation conditions. Approval
alone does not complete T027, T029–T031, T036, T053, T065, T080, T094 or T097.
New corrections remain test-first. No existing validation or security guarantee is waived.

### Reanalysis findings I1 and I2

- **I1 — frozen plan status inconsistency**: `plan.md:650-654`'s blanket “No constitution
  violation needs justification” is obsolete. The historical VIII/XIII deviation needs the
  approved D-6 disposition and all alternative-validation conditions in
  `historical-validation-exception.md:49-78`. Superseding ADR 028 through ADR 048 independently
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
It covers all thirteen approval items, including D-6. ADR 048 already records this approval;
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
alternative validation. ADR 048's partial supersession of ADR 028 separately follows
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

## 15. Partial implementation (run 42243758976/1)

### Approval and completed work

The supplied approval by `mjungsbluth` at `2026-11-06T15:30:38Z` on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728>
covers exactly ADR-2, API-7, ADR-3, D-7 and D-8. T001 incorporates the permit-bound reuse
revision into ADR 048 §10 and preserves earlier approval history. Its binding behavior is
not yet implemented. ADR-3 remains unincorporated until its issue prerequisite is satisfied;
D-7 approval does not itself supply permission to publish the standard-file registry change.

T108/T109 review confirms the binding mismatch diagnostic is log-only Debug metadata
`delegation_binding=mismatch`, with no principal, token, raw scopes, grant contents, span
attribute or metric. Broker/resolver baseline identifiers and the exact D-7 rename/explicit
kinds/namespace requirements remain those in `contracts/observability.md`. No binding
mismatch diagnostic is emitted yet, and this design review does not close T103.

T123 isolates the independently approved final-forwarding expiry portion of T116. The
header-only request retains its original permit and checks its deadline after exchange,
before Authorization mutation. Body, batch and approval-authorized body forwarding check
the same original deadline after authorization, without re-resolution. Existing denial/error
precedence, disabled behavior and the expiry event are retained. Architecture, consumer docs
and ExtProc guidance describe this change; spec.md and plan.md remain unchanged.

### Test-first evidence

`internal/extproc/server/delegation_test.go:367` adds
`TestDelegation_RejectsExpiryBeforeFinalForwarding` with three subtests. Before implementation,
`go test ./internal/extproc/server -run '^TestDelegation_RejectsExpiryBeforeFinalForwarding$' -count=1`
compiled and failed semantically in all three: body and final batch policy evaluation returned
a forwarding RequestBody response, and header-only exchange returned an Authorization-bearing
RequestHeaders response after the clock reached the original deadline. After implementation
the unchanged status/body assertions pass. Buffer-log assertions verify the existing
`extproc.delegation_context.permit_expired` event and exclude subject/provider credentials,
raw scopes and the grant-update timestamp. `just extproc-test` passes with the race detector.
This is new evidence only for final-forwarding expiry, not for binding or US3.11–US3.13.

### Current passing validation

- `just check`
- `just test`
- `just extproc-test` (race)
- `just web-test`
- `just cdk-test`
- `just mock-sample-agent-test`
- `just mock-upstream-oauth2-test`
- `just test-integration` (self-contained, including delegation contract conformance)
- `just docs-build`
- `just test-e2e-backend`: 999 passed, 0 failed
- `just test-e2e-frontend`: 67 passed, 0 failed
- `ginkgo -v --label-filter="authzen-delegation && !docker" ./tests/e2e/`: 28 passed
- `ginkgo -v --label-filter="delegation-context" ./tests/e2e/`: 11 passed
- `ginkgo -v --label-filter="delegation-context-startup" ./tests/e2e/extproc/`: 4 passed
- `ginkgo -v --focus="OPA Authorization|ExtProc Metadata Input" --label-filter="!docker" ./tests/e2e/extproc/`: 30 passed

The focused feature total is still **43 local scenarios**, not the target 46: the three new
binding scenarios are not authored or validated. All existing local regression journeys pass.

### Blockers and remaining work

The prepared `bash "$SECURITY_SCAN_SCRIPT" .` scan did not pass. Its initial concurrent run
terminated with exit 137. A standalone retry completed gosec (0 issues), then govulncheck
exited 3 with **GO-2026-6865**, "Panic on two-block frames", in the existing
`github.com/klauspost/compress v1.20.1` dependency (`go.mod:151`), fixed in `v1.20.2`.
The reported call paths originate in HTTP gzip middleware, not this ExtProc change. No
reproduction/exploitation, dependency change, suppression or check weakening was performed.
The maintainer must arrange the dependency remediation; the full security gate remains failed.

| Unchecked work | Why it remains |
|---|---|
| T103 | Namespace registration needs the permitted publication path specified in `contracts/observability.md:71`; the earlier rejection remains recorded in §13. No permission update or registered rows are supplied by this context. |
| T104 | No maintainer-created follow-up issue URL is supplied; the approved ADR-3 requires it before incorporation (`adr-048-revision-canonical-permission-sets.md:25-27`). |
| T110, T111, T112, T113, T121, T122 | New narrowing/provider-limit/concurrent-binding acceptance tests, cache/server binding tests and their genuine semantic-red gate are not completed in this run. |
| T114, T115, T116, T117 | Binding-aware port, shared immutable first-claim handles, identity-safe deletion/retry, terminal conflict and mismatch diagnostic remain unimplemented; T116's independent forwarding-expiry portion alone is completed through T123. |
| T118 | Full binding architecture/glossary/operator guidance awaits implemented binding behavior; current edits describe only final-forwarding expiry. |
| T119, T120 | Binding acceptance coverage and full compliance remain incomplete; the security scan also fails. Current passing commands do not close these tasks. |

US2.19, `just test-e2e-docker`, `just test-integration-infra` and Agentgateway container cases
are deferred to downstream container tests on the pushed commit, never claimed validated here.
The unchecked checklists remain unchanged (the requirements checklist has no unchecked items).
Reanalysis LOW I2 is resolved by a serial T110 → T121 → T122 E2E lane. LOW I1's research
approval references are labeled historical; its spec.md wording is left immutable, with the
approval provenance above resolving the current status. This run is **blocked**, not a completed
feature or an approval to merge.

## 16. Partial implementation (run 42297225880/1)

### Completed D-7 telemetry migration (T103)

The supplied approval remains the five-item approval by `mjungsbluth` at
`2026-11-06T15:30:38Z` on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728>.
The current implementation workflow permits the required standard-file change. Both exact
namespace rows (`access_evaluation` and `delegation`) are added to
`coding-standards/OPENTELEMETRY.md` alongside the code/test migration. No workflow or
permission change is part of this implementation.

The broker now emits `tokenexchange.access_evaluation` and
`tokenexchange.access_evaluation.permitted|denied|failed`, without dual-emitting old names.
Broker and resolver spans explicitly select Internal; the outbound evaluation span retains
explicit Client. Scope, bounded outcome/decision/cache attributes, levels and credential
handling are unchanged. The operator reference has an old-to-new table and rolling-deployment
query guidance; no metric changes. ADR 048 records this implementation under its existing
approval provenance. `spec.md` and `plan.md` remain unchanged.

### Genuine test-first evidence

Before the production rename, the updated target-name assertions in
`internal/adapters/http/enduser/authzen_evaluation_test.go` compiled and failed semantically:
`TestAuthZENEvaluation_AuditRecords` (all six outcomes),
`TestAuthZENEvaluation_EarlyFailureAudit`, and
`TestAuthZENEvaluation_SpansExcludeCredentialsAndContext` observed old `authzen.*` identifiers
where the approved `tokenexchange.*` identifiers were required. Command:

```bash
go test ./internal/adapters/http/enduser -run 'TestAuthZENEvaluation_(AuditRecords|EarlyFailureAudit|Spans)' -count=1
```

After the rename the unchanged target-name assertions pass. The broker span assertions also
pin Internal kind, `tokenexchange` scope, `access_evaluation.decision=permit|deny|error`,
permit-only `max_age=15`, and credential/context exclusion. ExtProc telemetry tests pin
`extproc` scope, Internal resolver/Client evaluation kinds, cache hit/miss, bounded error
metadata and exclusion of subject/assertion/resource/context/dependency markers. These test
results cover T103 only, not the unimplemented binding diagnostic or acceptance scenarios.

### Current validation

Passed on this run's changes:

- `just check`
- `just test`
- `just extproc-test` (race)
- `just test-integration` (self-contained, including delegation contract conformance)
- `just web-test` (261 tests)
- `just cdk-test`
- `just mock-sample-agent-test`
- `just mock-upstream-oauth2-test`
- `just docs-build`
- `just test-e2e-backend` (999 passed)
- `just test-e2e-frontend` (67 passed)
- `ginkgo -v --label-filter="authzen-delegation && !docker" ./tests/e2e/` (28 passed)
- `ginkgo -v --label-filter="delegation-context" ./tests/e2e/` (11 passed)
- `ginkgo -v --label-filter="delegation-context-startup" ./tests/e2e/extproc/` (4 passed)
- `ginkgo -v --focus="OPA Authorization|ExtProc Metadata Input" --label-filter="!docker" ./tests/e2e/extproc/` (30 non-container scenarios passed; Agentgateway cases did not execute)

The focused feature coverage remains 43 local scenarios, not the target 46. US3.11–US3.13
are not authored or validated. Container-backed US2.19, `just test-e2e-docker`,
`just test-integration-infra` and Agentgateway suites are deferred to downstream container
tests on a pushed completed implementation; no local validation is claimed for them.

### Failed security gate and unfinished work

`bash "$SECURITY_SCAN_SCRIPT" .` exited 3 at sample-agent govulncheck with
**GO-2026-6911** in `github.com/mark3labs/mcp-go v1.4.0`
(`mocks/sample-agent/go.mod:6`). The advisory reports no fixed version. The reported call
paths originate in the sample-agent MCP client (`mocks/sample-agent/internal/handlers/handlers.go:473-506`),
not this telemetry change. The earlier GO-2026-6865 compression finding is resolved by the
already merged base update. No unrelated dependency/code change, exploit verification,
security suppression or test weakening was made. A maintainer must arrange resolution of
the sample-agent finding before the required full security gate can pass.

| Unchecked work | Current disposition |
|---|---|
| T104 | The configured safe output accepted creation of `Follow-ups for #292` for the approved future OPA migration. It returned no concrete issue number/URL before downstream publication. The canonical-field ADR/docs change remains unapplied pending that published URL; the current two compatibility fields are unchanged. |
| T110, T111, T112, T113, T121, T122 | Binding acceptance and cache/server tests plus their genuine semantic-red gate remain unfinished. No binding implementation is introduced ahead of that gate. |
| T114, T115, T116, T117 | Binding-aware port/handles, atomic first claim, identity-safe delete/retry, terminal conflict and log-only mismatch diagnostic remain unimplemented; T123's already completed forwarding-expiry subset is preserved. |
| T118 | Full binding architecture/glossary/operator guidance remains unfinished; this telemetry-only increment makes no binding runtime claim. |
| T119, T120 | Full binding acceptance/compliance remains incomplete, independently of the failed external security gate. Passing existing suites does not close these tasks. |

No new approval is requested for the telemetry migration. The follow-up issue tracks future
compatibility removal only; creating it grants no removal/deprecation approval. Checklists
remain read-only and unchanged. This phase is **blocked**, not feature completion.

## 17. Test-first continuation (run 42350955442/1)

### Outcome and supplied approval

Partial implementation: T104, T110 and T121 are complete; T111 is partially authored.
There are 112 checked tasks and 11 unchecked tasks. Token-binding runtime implementation
has not started; its complete test-first prerequisite remains unfinished. This is a blocked
phase, not feature completion, and the new failing tests remain enabled.

The supplied approval by `mjungsbluth` at `2026-11-06T15:30:38Z` on
https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728
covers ADR-2, API-7, ADR-3, D-7 and D-8. The maintainer supplied
https://github.com/zalando-incubator/agentic-identity-broker/issues/441 for T104. ADR 048 §5
and Revision history now record ADR-3 incorporation and that issue. New enabled policies
use canonical `input.context.permission_sets` and the effective ceiling in `resource.scopes`.
Both compatibility inputs retain their current values and disabled serialization; no OPA
field is removed or deprecated, and no future removal is approved. Policy guidance is in
`docs/api/authzen-evaluation.md`, `docs/reference/token-exchange.md` and
`internal/extproc/AGENTS.md`. The incorporated revision draft was removed. `spec.md` and
`plan.md` are unchanged.

### New test-first evidence

The test-harness extraction is a separate behavior-preserving commit. Before adding binding
unit tests, `go test ./internal/extproc/server -run '^TestDelegation_' -count=1` passed with
all existing assertions unchanged. The helper can now use the production TokenExchanger.
No production binding signatures, stubs or behavior were added.

| Scenario / test | Location | Observed semantic red |
|---|---|---|
| US3.11 (T110) | `tests/e2e/delegation_context_journey_test.go:329` | The first target request after permit expiry leaves broker exchange count at 1, expected 2. Broad warmup, real consent narrowing and provider re-authorization complete successfully; the stored session has only `read:repo`. |
| US3.12 (T121) | `tests/e2e/delegation_context_journey_test.go:365` | Without re-authorization, the count again stays 1 instead of 2. The expected identical broad token is explicitly retained under the provider-capability limitation. |
| Scope-set / grant-instant reuse (partial T111) | `internal/extproc/server/exchanger_binding_test.go:63` | Changed scopes alone, changed grant instant alone and empty scopes forward `SYNTHETIC_OLD_RESULT` instead of `SYNTHETIC_NEW_RESULT`, and make 1 broker call instead of 2, on both paths. Equal/reordered/duplicate scope sets and equal timestamp instants remain passing controls. |
| Failed replacement (partial T111/T112 coverage) | `internal/extproc/server/exchanger_binding_test.go:113` | Both paths return Authorization-bearing request headers instead of an immediate failure after the permit changes and the replacement broker response would fail. |
| Disabled Exchange control | `internal/extproc/server/exchanger_binding_test.go:137` | Passes: 1 broker call with unchanged token, identities and permission-set extensions. |

US3.11 and US3.12 each have exactly one `It()` and independent body/header consumer
fixtures. US3.11 includes both successful and failed replacement variants; US3.12 checks
an unchanged broad provider session. Their policy fixture checks exact permit scopes and
the new grant instant. Expiry polling issues no requests, so the count measures one target
request rather than retries. The current semantic failures occur in the first body variant;
later variants remain authored but not green-validated.

Commands and current results:

- `ginkgo -v --label-filter='delegation-context' --focus='forwards the reauthorized reader|re-exchanges after narrowing' ./tests/e2e/`: compiled, 2 semantic failures, no pending specs.
- `go test ./internal/extproc/server -run '^TestTokenBinding_' -count=1`: compiled, 8 failing subtests across 2 new test functions; equality and disabled controls pass.
- `just extproc-test`: expected semantic red in those same new tests, all other ExtProc packages pass; no race-detector warning.
- `just test`: expected semantic red in those same new tests; no other test failures reported.
- `ginkgo --no-color --succinct --label-filter='delegation-context' ./tests/e2e/`: 13 scenarios executed, 11 existing scenarios pass and 2 new scenarios fail, no pending specs. US3.13 is not authored yet; this is not the required 14-scenario green gate.

### Passing checks and remaining work

- `just check` passed after the test changes.
- `just web-build`, `just test-integration` (including delegation-contract conformance) and
  `just docs-build` passed. The site build reports its existing CSS-minimizer warning.
- `bash "$SECURITY_SCAN_SCRIPT" .` passed on this run before the test additions. §16's
  sample-agent finding no longer blocks that command: the workflow-prepared default-branch
  scan applies its existing scoped advisory exception. This phase made no dependency,
  scanner, exception or security-control change and did not verify an exploit.
- The requirements checklist has 16 checked items and no unchecked items; it is unchanged.

| Remaining task | Why it remains unchecked |
|---|---|
| T111 | The initial black-box tests are written and semantically red. Identity-safe compare/delete, competing first claims, same-token replacements, singleflight and non-cacheable retry delivery, TTL/cooldown, timeout/cancellation/breaker and terminal-conflict permutations still need complete test coverage. |
| T112 | Complete binding-aware server stubs and precedence, retry/expiry and API-7 terminal-response assertions are not authored yet. The shared harness supports real-exchanger tests but does not complete this task. |
| T122 | The barrier-controlled US3.13 complete journey for both paths is not authored yet. |
| T113 | The full unit/concurrency/E2E semantic-red prerequisite is incomplete; the evidence above closes no missing assertion. |
| T114–T117 | Binding types/port, result identities, atomic first claim, compare/delete/retry, server integration, API-7 mapping and safe mismatch diagnostic are not implemented. They remain behind T113; existing forwarding-expiry behavior is preserved. |
| T118 | Complete binding architecture/glossary/operator guidance is not yet updated; T104 changes only canonical policy guidance. |
| T119–T120 | New binding tests remain red; full acceptance, compliance and final non-container verification are not complete. Current checks are not substituted for these gates. |

No additional maintainer decision is needed: continuation uses the existing supplied approval.
Resume from the unfinished test-authoring tasks before runtime implementation. Container-backed
US2.19, `just test-e2e-docker`, `just test-integration-infra` and Agentgateway cases remain
**deferred to downstream container tests**, not locally validated and not a blocker here.

## 18. Binding test continuation (run 42378270217/1)

### Outcome

T111 and T112 have additional genuine test-first coverage, but both remain partially
complete. No runtime binding implementation, new approval, requirement change, production
API, configuration change or dependency change is recorded. The existing five-item approval
and ADR 048 acceptance remain unchanged. There are still 112 checked tasks and 11 unchecked
tasks; this session is **blocked**, not feature completion.

### New assertions and observed evidence

| Test | Location | Evidence |
|---|---|---|
| `TestTokenBinding_PreservesIdenticalTokenReplacementAndDetachedDelivery` | `internal/extproc/server/exchanger_binding_test.go:161` | Three barrier-controlled body-path cases capture an initial exchange result before a successor request, then release resolution: compatible replacement reuse, incompatible replacement requiring one extra exchange, and a matching detached result that remains usable. All provider token bytes are identical. The test also checks that a subsequent successor request can reuse its binding unless legitimately replaced. All three cases compile and fail semantically because the changed successor permit still makes only one broker call, not two; subsequent expected totals also fail. |
| `TestTokenBinding_NonCacheableSuccessPreservesZeroLifetimeAndExchangeIdentity` | `internal/extproc/server/exchanger_binding_test.go:225` | Both request paths pass the existing-behavior control: `expires_in: 0` remains non-cacheable, every request exchanges, and principal, agent and permission-set extensions are preserved. This is not evidence of atomic first-claim or shared non-cacheable delivery. |
| `TestDelegation_RejectsExpiryDuringBindingReplacement` | `internal/extproc/server/delegation_test.go:152` | Both request paths compile and fail semantically: they return Authorization-bearing RequestHeaders rather than the required delegation-unavailable 503. The broker fixture advances the test clock to the original permit deadline during the replacement response; assertions require no buffering/mutation, no re-resolution, and no body policy evaluation after expiry. The current implementation never requests that replacement, so these tests are red for missing binding integration, not proof of an existing final-forwarding-check regression. |

The replacement fixture synchronizes both resolver entry and completion of the production
exchanger's initial call before running the successor request. It uses test-only wrappers,
channels and clocks; there is no scheduler sleep, production test API, token-byte identity
comparison, skipped test or deliberately broken production behavior. The new focused tests
were run with `-race`, compiled, reached their semantic assertions, and emitted no race warning.

### Validation

| Command | Result |
|---|---|
| `just check` | Pass after both test-file changes; format, vet and lint, 0 issues. |
| `go test -race ./internal/extproc/server -run '^TestTokenBinding_(PreservesIdenticalTokenReplacementAndDetachedDelivery\|NonCacheableSuccessPreservesZeroLifetimeAndExchangeIdentity)$' -count=1` | Three new replacement/detached cases are semantically red; both non-cacheable controls pass. |
| `go test -race ./internal/extproc/server -run '^TestDelegation_RejectsExpiryDuringBindingReplacement$' -count=1` | Both new expiry cases are semantically red. |
| `just extproc-test` | Fails only in the unfinished binding tests: the two existing binding test functions and the two new semantically red functions. No race warning. |
| `just test` | Same unfinished binding-test failures; no additional test failure reported. |
| `just test-integration` | Pass, including delegation contract conformance. |
| `just web-test` | Pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `just docs-build` | Pass; generated dependency-lock changes were not retained. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Pass before the test additions, with the workflow-prepared existing scoped exceptions unchanged; no production code or dependency changed afterward. |
| `just test-e2e-backend` | 1001 selected scenarios execute: 999 pass, US3.11 and US3.12 fail semantically on missing replacement exchange. The recipe excludes 19 scenarios; none is claimed as validated. |
| `just test-e2e-frontend` | 67/67 pass. |
| `ginkgo -v --no-color --procs=2 --skip="Agentgateway" ./tests/e2e/extproc/` | 103 selected non-container scenarios pass; 17 Agentgateway scenarios are excluded, not validated. |

### Remaining work

- T111: add atomic competing first-claim, exact same-flight delivery, non-cacheable conflict,
  all retry-result outcomes, timeout/cancellation/breaker failure and TTL/cooldown assertions.
  The new same-token replacement/detached tests do not substitute for these cases.
- T112: add binding-aware port stubs, API-7 terminal conflict, retry-error precedence and
  remaining buffering/batch/concurrency coverage. The new replacement-expiry tests cover
  only that portion of the task.
- T122: US3.13's barrier-controlled, complete concurrent-live-permit journey is not authored.
- T113: the complete required unit and E2E red gate remains unfinished.
- T114–T117: token-binding types and port, opaque shared result identity, atomic first claim,
  identity-safe removal/replacement/retry, server integration, terminal 503 mapping and
  credential-safe mismatch diagnostic remain unimplemented behind T113.
- T118: Token Binding glossary and complete architecture/operator documentation remain open.
- T119–T120: green acceptance, fresh compliance and final successful non-container validation
  remain open. The failed commands above are not passing checks or completion evidence.

No additional sign-off or permission is needed to continue. Resume implementation with the
remaining test-authoring and semantic-red gate, then runtime binding and green verification.
US2.19, `just test-e2e-docker`, `just test-integration-infra` and Agentgateway-backed tests
remain deferred to downstream container tests on the pushed commit, never validated locally.

## 19. Retry-failure and competing-first-use tests (run 42381635025/1)

### Outcome and approval boundary

Partial implementation: T111/T112 have 14 additional semantic-red cases; neither task is
complete. No production behavior, signature, approval, configuration, dependency or contract
changes. T001's supplied approval by `mjungsbluth` at `2026-11-06T15:30:38Z` on
https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728
continues to cover ADR-2/API-7/ADR-3/D-7/D-8. T103/T104 remain completed. `spec.md`,
`plan.md` and the requirements checklist are unchanged. No new sign-off is needed.

The fixture extraction is isolated in preceding commit `a089e228`: existing successful
non-cacheable/disabled controls and the direct-context/body-precedence tests pass unchanged
before and after it. It adds request-aware callbacks and per-fixture configuration without
changing existing fixture responses. This is a test-helper refactor, not runtime binding.
The pre-existing replacement-expiry test remains red and is not claimed as a passing baseline.

### Test-first assertions and observed failures

New file: `internal/extproc/server/exchanger_binding_failure_test.go`.

| Test | Location | New coverage and observed semantic red |
|---|---|---|
| `TestTokenBinding_ReplacementFailuresRetainExchangeErrorHandling` | `:22` | Ten cases, body/header-only for broker 503, timeout, malformed success, circuit-open and reauthentication. A changed permit must not emit the prior Authorization result. Non-recovery failures retain 500 handling, open circuit retains 503, and reauthentication retains the 403 insufficient-scope challenge/JSON-RPC recovery body and tokenless five-second cooldown. Current code returns RequestHeaders instead of ImmediateResponse before any replacement is attempted. |
| `TestTokenBinding_CompetingFirstUseOfSharedResults` | `:121` | Cacheable and non-cacheable successful-result copies are delivered to two body requests before a shared resolver barrier releases different live permits. Exactly one may forward the initial result; the other must exchange once more. Current code forwards the initial token twice and makes one broker call instead of two. The wrapper delivers the same production result to both callers; this is not evidence of actual same-flight retry delivery. |
| `TestTokenBinding_CanceledReplacementCannotForwardPriorResult` | `:170` | Body/header-only requests must enter the required replacement exchange before cancellation, then emit no response token and finish with gRPC Canceled. The broker barrier is released during cleanup. Current code instead returns Authorization-bearing RequestHeaders before the replacement begins. Cancellation after retry entry is authored, not yet reached or green-validated. |

Command: `go test -race ./internal/extproc/server -run '^TestTokenBinding_(ReplacementFailuresRetainExchangeErrorHandling|CompetingFirstUseOfSharedResults|CanceledReplacementCannotForwardPriorResult)$' -count=1`.
All 14 cases compile and fail semantically, with no race warning or pending tests. Existing
passing assertions were not weakened. No scheduler sleep, custom production clock, token-byte
identity logic or test-only production API is added. Tokens in assertions are synthetic.

The new failure matrix does not certify the as-yet-unreached timeout, cancellation, breaker
or cooldown branches as correct. Their current failure is the missing replacement operation.
Shared first-use coverage does not close the required real singleflight and terminal-conflict
permutations in T111, binding-aware port stubs in T112, or the complete US3.13 journey.

### Current validation

| Command | Result |
|---|---|
| `just check` | Pass, format/vet/lint with 0 issues. |
| `just test` | Fails only in seven unfinished binding test functions, including the three new functions above. |
| `just extproc-test` | Same seven semantically red functions; no race warning. All other ExtProc packages pass. |
| `just test-integration` | Pass, including delegation producer/consumer contract conformance. |
| `just web-test` | Pass, 261 tests. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Pass using the prepared default-branch scan and its existing scoped exceptions. No scanner, dependency or exception changes. |
| `just docs-build` | Pass. |
| `just test-e2e-backend` | 999 pass, 2 fail: US3.11/US3.12 still do not perform the required replacement exchange. The recipe selects 1001 scenarios; its 19 excluded scenarios are not validated. |
| `just test-e2e-frontend` | Pass, 67 scenarios. |
| `ginkgo -v --no-color --procs=2 --skip='Agentgateway' ./tests/e2e/extproc/` | 103 non-container scenarios pass; 17 Agentgateway scenarios are excluded and not validated. |
| `ginkgo -v --no-color --label-filter='authzen-delegation && !docker' ./tests/e2e/` | 28 pass. |
| `ginkgo -v --no-color --label-filter='delegation-context' ./tests/e2e/` | 11 pass, US3.11/US3.12 fail. US3.13 is absent, not skipped or validated. |
| `ginkgo -v --no-color --label-filter='delegation-context-startup' ./tests/e2e/extproc/` | 4 pass. |

### Unfinished work

- T111: real singleflight initial/retry delivery, typed terminal-conflict outcomes, and all
  remaining ownership/delete/delivery permutations need assertions and genuine red evidence.
- T112: binding-aware port stubs, precedence before binding, terminal API-7 mapping and the
  remaining retry/body/batch integration cases are incomplete. Existing expiry checks remain.
- T122: the barrier-controlled US3.13 complete journey on both paths is not authored.
- T113: the complete test-first prerequisite remains open; passing old behavior and these
  additional red cases do not replace its missing unit/E2E coverage.
- T114–T117: binding types/port, shared identities/cells, cache deletion/retry, integration,
  terminal conflict and safe mismatch diagnostic remain unimplemented behind T113.
- T118: complete binding architecture/glossary/operator guidance remains unfinished.
- T119/T120: green acceptance and current full compliance remain unfinished. The passing
  independent checks above do not make the failed unit/consumer gates successful.

All 11 boxes remain unchecked. This session is **blocked**, not feature completion. Resume
with the remaining concurrency/terminal-conflict test authoring and US3.13, then binding
runtime and green verification under the supplied approval. Container-backed US2.19,
`just test-e2e-docker`, `just test-integration-infra` and Agentgateway suites are deferred to
downstream tests on the pushed commit, never claimed validated locally or a sandbox blocker.

## 20. Binding implementation continuation — run 42527822363/1

The supplied approval is unchanged: mjungsbluth, 2026-11-06T15:30:38Z, https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728. ADR-2/API-7/ADR-3/D-7/D-8 are covered; T001/T103/T104 remain completed.

### Test-first checkpoint (before runtime binding)

Minimal compiling TokenBinding/result-handle/ExchangeBound signatures retain the old behavior only. New port tests cover atomic claiming, detached/matching/replaced identity, compatible and incompatible replacements, joined retry delivery (including zero-lifetime result), terminal conflict, exchange failure, and credential-safe log-only diagnostics. Server port stubs require binding after precedence and before buffering, and exact terminal API-7 mapping on both paths.

- `go test ./internal/extproc/server -run 'TestBindingPort|TestTokenBinding_|TestDelegation.*Expir' -count=1`: semantic failures for missing claims/replacement exchanges; existing equality/disabled controls pass.
- `go test ./internal/extproc/server -run 'TestBindingPort|TestDelegation_BindingPort' -count=1`: semantic failures for missing claim, missing retry-flight participation, missing mismatch diagnostic and absent binding-port invocation; tests compile.
- `ginkgo --label-filter='delegation-context' --focus='shared retry|reauthorized reader token|without claiming provider' ./tests/e2e/`: all three scenarios compile and fail semantically. US3.11/US3.12 exchange counts remain 1 rather than 2; new US3.13 fails to reach the required binding-validation barrier because the server never calls ExchangeBound. The new journey uses independent production resolver instances against the real broker, differing live grant snapshots, a shared production exchanger and a response barrier around the real broker's retry. It checks winner forwarding, loser terminal 503 and same-token replacement survival.

Runtime implementation and green verification follow this checkpoint; no historical D-6 evidence is substituted for this red run.

### Implementation and corrections

- `de74ea2a` isolates the behavior-preserving exchanger extraction: shared flights return the original broker outcome and each caller applies the existing stale fallback. On a clean checkout with only this refactor applied, `go test -race ./internal/extproc/server -skip 'TestTokenBinding_|TestDelegation_RejectsExpiryDuringBinding' -count=1` passed unchanged pre-existing regression assertions. The exclusions are the already-red binding continuation, not validation of those cases. Enabled retries can now forbid stale fallback per caller without changing the singleflight key or breaker accounting.
- Every successful result now has an opaque shared handle, including zero-lifetime successes. `ExchangeBound` normalizes copied scope sets, atomically claims once, compares grant instants, deletes only its observed identity, preserves compatible replacements, and retries at most once without incompatible success-cache reuse or stale fallback. Retry conflict returns the exact API-7 503 with no rebind/deletion/forwarding. Body and header-only paths use the validated permit and retain the original deadline through final forwarding.
- The real-broker journeys exposed truncation of grant update instants. `TestAccessEvaluation_GrantBindingPreservesUpdateInstant` failed semantically before the producer correction: fractional instants became the same whole-second string, losing same-scope version changes. RFC3339Nano preserves the approved grant-instant binding within the existing date-time contract; it introduces no field, token claim, key/default, schema or dependency. Whole-second instants serialize unchanged. The projection expectation was updated to retain, not round, that instant. The test and both narrowing/concurrency journeys now pass.
- The timeout fixture now drains its request body before awaiting HTTP cancellation; otherwise httptest cleanup waits indefinitely because the server has not consumed the request. All timeout/error assertions remain intact. The pre-policy expiry test now asserts the stronger approved API-7 boundary: an already-expired permit gets 503 before Authorization mutation or buffering, rather than waiting for the body phase. Buffering/batch/approval expiry coverage remains enabled.
- `TestTokenBindingFormattingDoesNotExposeGrantOrScopes` failed before adding redacted String/GoString/LogValue implementations. The Debug mismatch field is log-only; buffer/telemetry tests reject credential, scope, principal and resource markers, and assert binding adds no span or instrument.
- Architecture, glossary, ExtProc guidance and operator references document ownership, same-token/new-generation reuse, bounded retry, unchanged cooldown/disabled behavior, and the explicitly approved provider/provenance limitations. No stronger downscoping or exchange-version guarantee is claimed.

### Current scenario mapping and green evidence

| Scenario / gate | Location / result |
|---|---|
| US3.11 | `tests/e2e/delegation_context_journey_test.go:330-365`: real consent narrowing and provider re-authorization; success and failed replacement on both paths; red then green. |
| US3.12 | `tests/e2e/delegation_context_journey_test.go:366-390`: consent narrowing with unchanged broader provider session; new result identity and identical token bytes on both paths; red then green. |
| US3.13 | `tests/e2e/delegation_context_journey_test.go:391-467`: independent production resolver/server instances share one production exchanger; controlled real-broker grant reads yield different live permits; both join one held retry; exactly one winner forwards, loser returns terminal 503, winner's identical-token replacement survives. Initial missing binding-port invocation was semantically red; complete journey now green on both paths. |
| Port outcomes / diagnostics | `internal/extproc/server/token_binding_test.go`: claim/replacement/detached outcomes, shared retry compatibility/conflict/failure including actual zero-lifetime results, redacted formatting and log-only credential safety. |
| Server integration | `internal/extproc/server/delegation_test.go`: binding-aware stubs pin precedence, evaluated permit, terminal API-7 and pre-buffer/final-forward expiry; prior cache/timeout/cancellation/claim tests remain active. |
| `go test -race ./internal/extproc/server ./internal/domain/tokenexchange -count=1 -timeout=90s` | Pass. |
| `just check` | Pass, zero lint issues; final formatting/vet/lint checks pass. |
| `just test` | Pass, complete fast Go/package race suite. |
| `just extproc-test` | Pass, complete ExtProc/command race suites. |
| `just test-integration` | Pass, self-contained suite including producer/consumer contracts. |
| `just web-test` | Pass. |
| `just cdk-test` | Pass. |
| `just mock-sample-agent-test` | Pass. |
| `just mock-upstream-oauth2-test` | Pass. |
| `bash "$SECURITY_SCAN_SCRIPT" .` | Pass with the workflow-prepared default-branch scanners and existing advisory exclusions unchanged. |
| `just docs-build` | Pass. The newly added ADR reference uses a repository URL so the site does not resolve it as a nonexistent local documentation route; no broken-link suppression. |
| `just test-e2e-backend` | Pass, 1002 non-container/non-performance scenarios including all 14 consumer journeys. |
| `just test-e2e-frontend` | Pass, all 67 scenarios. |
| `ginkgo -v --no-color --procs=2 --skip='Agentgateway' ./tests/e2e/extproc/` | Pass, 103 non-container scenarios; 17 container Agentgateway scenarios excluded/deferred, not validated. |
| `ginkgo --no-color --label-filter='authzen-delegation && !docker' ./tests/e2e/` | Pass, 28 local broker feature scenarios. |
| `ginkgo --no-color --label-filter='delegation-context' ./tests/e2e/` | Pass, 14 consumer scenarios. |
| `ginkgo --no-color --label-filter='delegation-context-startup' ./tests/e2e/extproc/` | Pass, 4 startup scenarios. |
| `ginkgo --no-color --label-filter='delegation-context' --focus='shared retry' --repeat=4 ./tests/e2e/` | Pass, five consecutive executions, both body/header-only paths in each. |

### Completion and downstream boundary

All 124 tasks are checked; all 16 specification-quality checklist markers remain reviewer-owned and already checked. The approved spec and plan are unchanged. Existing telemetry registration/canonical-guidance approvals remain implemented, and binding code now agrees with ADR 048 §10. No new approval item is introduced. The stale parallel marker on T110 is removed and the E2E-authoring lane is explicitly serialized.

There are 47 scenario mappings: 28 broker + 14 consumer + 4 startup = 46 locally validated; US2.19 remains the real-PostgreSQL downstream scenario. `just test-e2e-docker`, `just test-integration-infra` and the 17 Agentgateway container cases are deferred to container tests on the pushed commit, never claimed locally validated. Their absence does not leave an independent local task unfinished. Original D-6 dispositions remain historical exceptions, not retrospective TDD evidence.
