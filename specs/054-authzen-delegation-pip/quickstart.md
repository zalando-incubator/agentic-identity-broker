# Quickstart Validation: AuthZEN Delegation Policy Information Point

**Feature**: `specs/054-authzen-delegation-pip` | **Plan**: [plan.md](./plan.md)

**Latest outcome**: Implementation complete (run 42527822363/1). All 124 tasks are checked; permit-bound reuse, terminal conflict and US3.13 are implemented with genuine red/green evidence. All required non-container gates pass, including 46 local feature scenarios. US2.19, infra integration and Agentgateway container suites are deferred to downstream container tests on the pushed commit. Current evidence is in [implementation-log.md §20](implementation-log.md#20-binding-implementation-continuation--run-425278223631); historical D-6 does not cover new work.

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
2. T001 records only supplied approval for current covered items. The ADR-2 revision has been incorporated into [ADR 048 §10](../../adrs/048-authzen-delegation-pip.md#10-the-permit-governs-enabled-mode-token-reuse) with the current five-item approval in Status/Revision history; its incorporated draft was removed. Historical direction or thirteen-item approval does not cover the continuation. ADR-3 is incorporated with T104's recorded issue #441; it approves canonical guidance, not removal.
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

Implementation evidence belongs in `implementation-log.md`: record approval provenance, isolated refactor commits, scenario-to-test `file:line` mappings, semantic-red assertions, green commands/results and deferred-task reasons. Keep the approved `spec.md` and `plan.md` unchanged throughout implementation.

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

**Expected**: 14 specs pass (US3.1–US3.13 and US4.5). They run the production ExtProc composition against the real broker test server, including the approved ADR-2 binding journeys. Confirm these outcomes:

- The body path forwards only after both exchange and resolution succeed. US3.1 also proves
  that different exchange principal/agent UUID returns terminal API-7 503 before buffering,
  Authorization mutation or approval creation. The gate retains exchange-derived identity
  under ADR 036 after permit/exchange equality is checked.
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

**ADR-2 unit gates** (`just extproc-test`, `-race`): binding equality by principal/agent UUID,
scope set and instant; terminal 503 for each different or absent exchange identity on both paths,
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

## 7–20. Implementation evidence

Sections 7 to 19 retain evidence from earlier implementation runs; section 20 records the completed binding continuation and current validation. The evidence lives in
[`implementation-log.md`](implementation-log.md). References such as
"quickstart §12" resolve to that file.
