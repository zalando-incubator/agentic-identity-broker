# Quickstart Validation: AuthZEN Delegation Policy Information Point

**Feature**: `specs/054-authzen-delegation-pip` | **Plan**: [plan.md](./plan.md)

This guide proves the feature end to end after implementation. It is a validation guide, not an
implementation description. The contracts are in [contracts/](./contracts/). The value objects
are in [data-model.md](./data-model.md).

## Status and prerequisites

Before any implementation, these preconditions must hold:

1. The maintainer has signed off the items in the plan's
   [Approval Items](./plan.md#approval-items) with a 👍 on the pipeline's approval comment.
   These cover [contracts/authzen-evaluation-api.md](./contracts/authzen-evaluation-api.md),
   [contracts/extproc-delegation-context.md](./contracts/extproc-delegation-context.md), and the
   ADR draft [adr-045-authzen-delegation-pip.md](../../adrs/045-authzen-delegation-pip.md).
2. T001 has recorded only the supplied approval in each covered ADR's Status line and moved the approved draft to `adrs/`. Review direction alone is not sign-off.
3. The API-003 **Verify** checks against the final AuthZEN text have passed before the OpenAPI
   is frozen.
4. The spec revision for the OPA input shape (research R15) has landed. This was done on
   2026-10-28.
5. The Phase 0 extraction has its own maintainer-arranged PR and is reviewed and integrated
   before dependent feature code starts (T002–T005). This feature's worker does not create
   that separate PR. Record its URL and integrated revision here; an unavailable handoff
   blocks dependent work, not independent approved documentation or test authoring.

Tooling: `just install-tools` (Go 1.27.3, Ginkgo, golangci-lint). The focused broker run needs Docker or Podman for the real PostgreSQL outage scenario US2.19; the full gate also includes existing container-backed ExtProc/agentgateway and integration suites. Docker-in-Docker is unavailable in this sandbox. Run infrastructure gates on a capable runner; report any unfinished gate as a blocker, never as passed or waived.

Implementation evidence belongs in this guide: record approval provenance, the separate refactor PR and integrated revision, scenario-to-test `file:line` mappings, semantic-red assertions, green commands/results and deferred-task reasons. Keep the approved `spec.md` and `plan.md` unchanged throughout implementation.

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
ginkgo -v --label-filter="authzen-delegation" ./tests/e2e/
```

US2.19 MUST have both the shared `authzen-delegation` label and `Label("docker")`. The command above includes it and requires real PostgreSQL infrastructure. The ordinary `just test-e2e-backend` recipe excludes `docker`; it therefore covers 28 of these broker scenarios, not all 29.

```bash
just test-e2e-backend     # excludes US2.19; not the complete broker acceptance gate
just test-e2e-docker      # includes US2.19; requires a capable container runner
```

**Expected with infrastructure available**: 29 focused specs pass:

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

**Expected**: 11 specs pass (US3.1–US3.10 and US4.5). They run the production ExtProc
composition against the real broker test server. Confirm these outcomes:

- The body path forwards only after both exchange and resolution succeed.
- The header-only path never calls exchange when policy denies.
- After revocation, a repeat request past `max_staleness` is denied with 403 and `error_uri`
  set to the consent URL.
- A broker outage with a still-cached exchanged token returns 503, not a stale permit.
- The canonical-ID policy allows on both paths. The policy reads `input.context.agent`,
  `input.context.resource`, and `input.context.permission_sets`, and
  `granted_permission_sets_available` is `true`. Neither `input.context.delegation` nor
  `delegation_available` is present (contract §4).

**Complementary unit gates** (T049–T053, run by `just extproc-test`): synthetic broker replies
exercise missing, zero, malformed and duplicate `max-age`, invalid typed context, and unknown
members. Controlled-clock and synchronization tests cover a response received after its original
deadline and expiry during exchange, body buffering or batch processing. Assert 503 before the
expired decision reaches policy, with no forwarding or re-resolution. These checks complement
the 11 real-broker journeys above; they are not extra acceptance scenarios or evidence that the
real broker emits malformed responses.

## 4. ExtProc startup journeys (US4.1–US4.4)

```bash
just test-e2e-extproc          # or: ginkgo -v --label-filter="delegation-context-startup" ./tests/e2e/extproc/
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

```bash
just test-e2e           # backend + docker + ExtProc + frontend, including US2.19
just docs-build         # docs/api/authzen-evaluation.md renders
just verify             # full gate, E2E last
```

**Expected**: All suites pass. The existing suites `token_exchange_test.go`,
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

The constitution asks for the resolver extraction in a separate PR. The user running this
session chose to keep it in this PR as its own commit:
`b882d7b refactor(tokenexchange): extract shared DelegationResolver (exchange steps 2–9a)`.
This is a recorded deviation from the separate-PR requirement, not a maintainer-arranged handoff.

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

US1.1 `tests/e2e/authzen_delegation_test.go:33`; US1.2 `:45`; US1.3 `:64`; US1.4 `:73`; US1.5 `:83`;
US1.6 `:94`; US1.7 `:130`; US1.8 `:109`.
US2.1 `tests/e2e/authzen_delegation_denial_test.go:23`; US2.2 `:30`; US2.3 `:38`; US2.4 `:47`;
US2.5 `:55`; US2.6 `:63`; US2.7 `:76`; US2.8 `:85`; US2.9 `:97`; US2.10 `:105`.
US2.11 `tests/e2e/authzen_delegation_errors_test.go:29`; US2.12 `:38`; US2.13 `:45`; US2.14 `:53`;
US2.15 `:61`; US2.16 `:69`; US2.17 `:94`; US2.18 `:79`; US2.19 `:139`; US2.20 `:86`.
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

### Tasks left unchecked and why

| Task | Reason | Needed to close |
|---|---|---|
| T027, T036, T053, T065, T080, T094, T097 | Semantic-red evidence was recorded for the 28 in-memory broker scenarios only; unit tests and the consumer/startup E2E tests were written alongside the implementation | Maintainer accepts the recorded evidence, or a reviewer reruns those tests against the pre-feature commit (they need the new packages to compile, so this requires compile-only scaffolding) |
| T029–T031 | No separate compile-only scaffolding step was done; the code was implemented directly | Superseded by the implementation; close as not applicable if the maintainer agrees |
| T044 | The deferred-holder matcher test and credential-free audit assertions exist; the full trace-ID equality across context, audit, access log and `traceresponse` for evaluation is not asserted end to end | Add the composed HTTP-harness assertion |
| T059 | ExtProc logs carry `operation=delegation_resolution`, `delegation.cache` hit/miss and bounded categories, but no test captures server log output | Add captured-log tests |
| T064 | US4.6 asserts the deprecation marker in E2E; no separate `authzen_compat_test.go` unit test | Add the unit test or accept the E2E coverage |
| T071, T099, T100 | The agentgateway container image cannot be pulled here (403), so 5 ExtProc E2E specs (including the agentgateway `metadata_input_test.go` scenario) fail; the frontend Playwright suite and `just verify` were not run; three unrelated unit tests fail on the base commit in this sandbox | Run `just verify` on a capable runner |
| T091 | Depends on T059 | As above |

Pre-extraction baseline (T003): on base `a5042d5`, `go test ./internal/domain/tokenexchange/` passed and the four exchange regression files ran 58/58 green. The results after the extraction were identical.
