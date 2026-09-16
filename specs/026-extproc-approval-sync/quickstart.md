# Quickstart: Validating ExtProc Approval Cache Sync

**Feature**: `026-extproc-approval-sync` | **Plan**: [plan.md](plan.md)

Runnable scenarios that prove the feature works end to end. Contract details live in
[contracts/](contracts/); entity shapes live in [data-model.md](data-model.md). This document is a
validation guide, not an implementation guide.

---

## Prerequisites

| Requirement | Notes |
|---|---|
| Go 1.26.8 | `go version` |
| `just` | `just --list` for all recipes |
| `ginkgo` | `go install github.com/onsi/ginkgo/v2/ginkgo@latest` — required by every E2E recipe |
| Broker changes from this feature | FR-017 (`principal`/`agent_id`) and FR-018 (`approved_at`) — see [contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml). They are built as part of this feature and land first (plan Phase 2.5). Until they do, ExtProc fails closed on every `approval_required` and rejects every candidate as missing `approved_at` |
| Broker migrations ≥ 030 | `030_add_approval_patterns` supplies `tool_pattern` / `params_pattern` on the sync summary |
| Docker or Podman | Only for the agentgateway scenario (§6); every other scenario runs in-process |

---

## 1. Fast feedback loop

```bash
just check                # fmt → vet → lint
just extproc-test         # ExtProc unit tests with -race
just test-e2e-extproc     # ExtProc Ginkgo E2E suite
```

`just extproc-test` runs `go test -v -race ./internal/extproc/... ./cmd/extproc-token-exchange/...`.
Run it after every change in this feature — the approval cache is concurrent by construction and the
race detector is the primary guard.

Full gate before handing over:

```bash
just verify               # check + unit + web + cdk + mocks + integration + all E2E
```

---

## 2. Red-phase verification (TDD gate)

Before any implementation exists, the new tests must compile and fail **semantically**.

```bash
just test-e2e-extproc     # expect failures in tests/e2e/extproc/approval_sync_test.go
just extproc-test         # expect failures in internal/extproc/approval/*_test.go
```

Confirm the failures are semantic, not structural:

- Every failure names a concrete expectation (an HTTP status, a JSON-RPC error code, a broker call
  count), never a compile error and never `Expect(true).To(BeFalse())`.
- No `XIt`, `PIt`, `XDescribe`, `PDescribe`, `XContext`, `PContext`, or `Skip()` appears anywhere:

  ```bash
  grep -rnE '\b(XIt|PIt|XDescribe|PDescribe|XContext|PContext|Skip)\(' tests/e2e/extproc/
  ```

  This must print nothing.

Two **existing** unit tests assert behavior this feature supersedes and are expected to fail until
they are rewritten (see [contracts/opa-input-approval.md](contracts/opa-input-approval.md) §3):

- `internal/extproc/authorization/decision_test.go` — `TestParseDecision_ApprovalRequired_MappedToDeny`
- `internal/extproc/authorization/authorizer_test.go` — `TestOPAAuthorizer_ApprovalRequired_LogsRawActionAndReturnsDeny`

The `ciba_required` half of each must keep passing unchanged.

---

## 3. Configuration smoke test

Minimum configuration to enable approval gating:

```yaml
authorization:
  enabled: true
  policy:
    path: ./tests/e2e/extproc/fixtures/policies/approval_required.rego
tool_approvals:
  enabled: true
  url: "https://broker.example.com"
```

Full key reference, defaults, and every startup validation rule:
[contracts/extproc-approval-config.yaml](contracts/extproc-approval-config.yaml).

```bash
just extproc-build

# Happy path — starts and logs the bootstrap result
EXTPROC_CONFIG_PATH=./examples/config/extproc-tool-approvals.yaml ./bin/extproc-token-exchange

# Fail-fast checks — each must exit non-zero with an actionable message naming the offending key
EXTPROC_TOOL_APPROVALS_ENABLED=true ./bin/extproc-token-exchange
#   → tool_approvals.url is required when tool_approvals.enabled is true
#   → tool_approvals.enabled requires authorization.enabled

EXTPROC_TOOL_APPROVALS_ENABLED=true \
EXTPROC_TOOL_APPROVALS_URL=https://broker.example.com \
EXTPROC_TOOL_APPROVALS_LONG_POLL_TIMEOUT_SECONDS=300 \
EXTPROC_AUTHORIZATION_ENABLED=true ./bin/extproc-token-exchange
#   → tool_approvals.long_poll_timeout_seconds must be between 1 and 120
```

**Regression check** — an ExtProc with no `tool_approvals` section must behave exactly as before,
including collapsing `approval_required` to deny. Confirm the existing suites stay green:

```bash
just test-e2e-extproc
```

---

## 4. Scenario validation

Each row is one spec acceptance scenario and the observable outcome that proves it. All run under
`just test-e2e-extproc`.

### US1 — agent retries after approval

| # | Setup | Observable outcome |
|---|---|---|
| 1 | Policy returns `approval_required`; cache empty | Broker receives one `POST /api/approvals` with `Authorization: Bearer <subject>` **and** `X-Client-Assertion`; ExtProc returns HTTP 200 with JSON-RPC error `-32042` whose `data.elicitations[0].url` equals the broker's `approval_url` and whose `id` equals the request's JSON-RPC id |
| 2 | Approval created and approved; long-poll has not delivered it | ExtProc issues `GET /api/approvals?principal=…&agent_session_id=…`, matches, and forwards — **no** second `POST /api/approvals` |
| 3 | `session` approval synced for the active session | Request forwarded; **zero** broker approval calls |
| 4 | `permanent` approval in cache | Request forwarded in a *different* session; zero broker approval calls |
| 5 | `once` approval in cache | Exactly one `POST /api/approvals/{id}/consume` with the subject token and an empty body; forwarded only after `200`. The next identical call elicits again |
| 6 | Session ends (id absent for the idle window) | Next call in a new session elicits again; the permanent approval from row 4 still matches |

### US2 — no approval on deny

| # | Setup | Observable outcome |
|---|---|---|
| 1 | Policy returns `deny` | HTTP 403 `access_denied`; broker approval-call count is **0** (SC-003) |
| 2 | Policy returns `allow` | Forwarded with the exchanged token; zero approval calls |
| 3 | Policy returns `approval_required` for a granted set | Approval path entered (proves Tier 1 passed before Tier 2) |

### US3 — bootstrap

| # | Setup | Observable outcome |
|---|---|---|
| 1 | Broker pre-seeded with a permanent approval; then ExtProc starts | First matching call forwards with zero approval creation |
| 2 | Broker unreachable at startup | Process starts; a startup warning is logged; the next `approval_required` first attempts the targeted read, and only after that fails does it attempt create |
| 3 | Bootstrap returned an ETag | The first long-poll carries `If-None-Match` with that exact ETag |
| 4 | Unknown `(principal, agent)` pair arrives | A targeted `GET /api/approvals?principal=…` precedes any create |

### US4 — long-poll currency

| # | Setup | Observable outcome |
|---|---|---|
| 1 | No changes within the timeout | Broker returns `304`; ExtProc re-polls immediately with the same ETag |
| 2 | User approves during the poll | Broker returns `200` + new ETag within 2s; the affected pair is updated (SC-002) |
| 3 | Connection drops | Reconnect within 5s replaying the last ETag (SC-007) |
| 4 | ETag unknown to the broker | Broker returns full current state; ExtProc **replaces** rather than merges — a revoked approval must not survive |

Rows 3 and 4 are observably identical by design: the broker has no delta protocol and no stale-ETag
retention window (research R-005). Assert the replace semantics explicitly — seed an approval, sync
it, revoke it broker-side, force a resync, and confirm the record is gone from the cache.

### Edge cases

| Case | Observable outcome |
|---|---|
| Batched `tools/call` containing an `approval_required` message | Single 403 for the whole batch; the reason instructs a standalone re-issue; **no** approval created and **no** `-32042` (FR-016) |
| `ciba_required` | 403 `access_denied`, logged with the raw action; no approval created |
| Undefined OPA `decision` | 403, logged as a policy-misconfiguration warning distinguishable from an intentional deny (FR-012) |
| `POST /api/approvals` fails | 403 whose message states approval could not be initiated — **never** a fabricated `-32042` without a real URL |
| Consume returns non-2xx | 403; the reservation is released so a later retry can re-attempt (FR-008 item 4) |
| Two concurrent same-instance matches on one `once` approval | Exactly one consume call; the other falls through to creation |
| Exchange response omits `principal`/`agent_id` | 403 with reason "approval identity unavailable"; **no** cache lookup and **no** create (research R-000) |
| Sync delivers an `approved` record with no `approved_at` | Record treated as **unmatchable** and logged as a broker-contract warning; it is never ranked with a zero decision time (research R-014) |

---

## 5. Glob matching parity

Broker and ExtProc use vectors to keep one pattern dialect. The shared match and canonicalization
vectors are in `internal/domain/approval/toolpattern/vectors.json`. ExtProc-owned precedence vectors
are in `internal/extproc/approval/precedence_vectors.json`.

```bash
go test -run 'TestMatchesVectors|TestCanonicalVectors' ./internal/domain/approval/toolpattern/
go test -race -run 'Vectors' ./internal/extproc/approval/
```

The ExtProc cache tests exercise `toolpattern.Matches` through the cache-matching entry point.
`selectBest` in `internal/extproc/approval/precedence.go` evaluates the ExtProc precedence vectors.
A divergence in matching or precedence fails the relevant vector test.

Manual spot check of the precedence rule (exact tool > wildcard tool; then more constrained params;
then fewer wildcards; ties by later decision time, then smaller id):

| Cached patterns | Call | Winner |
|---|---|---|
| `create_*` (no params) and `create_pull_request` (`{"repo":"acme/*"}`) | `create_pull_request{repo:"acme/app"}` | the exact-tool record |
| `create_pull_request` (`{}`) and `create_pull_request` (`{"repo":"acme/*"}`) | `create_pull_request{repo:"acme/app"}` | the constrained record |
| Two records with identical patterns, `approved_at` one hour apart | `create_pull_request{repo:"acme/app"}` | the **later-approved** record, regardless of which id sorts first |

The last row is the FR-015 tie-break and the reason broker change 2 exists. Assert it with ids chosen
so that the lexicographically smaller id belongs to the *older* approval — otherwise a zero-timestamp
regression would pass by coincidence.

---

## 6. Full-stack agentgateway validation (optional)

Proves a real MCP client receives a conformant `URLElicitationRequiredError` through agentgateway.

```bash
just test-e2e-extproc        # includes the Docker-gated agentgateway specs
```

Requires a container runtime. Assert that the MCP client surfaces error code `-32042`, that
`data.elicitations[0].mode` is `"url"`, and that the JSON-RPC `id` matches the request — the approval
path emits the real request id, unlike the pre-existing re-auth elicitation which emits `null`.

---

## 7. Manual end-to-end walkthrough

```bash
just compose-up
```

1. Open `http://localhost:9002` and sign in as the Proxy Client.
2. Complete the broker delegation and upstream OAuth2 approvals.
3. Click **Call MCP Tool: create_issue (approval required)**.
4. Select **Review and approve tool call**, choose **Always allow**, then expand **Approval scope**.
   Change `repository` from **This value** to **Custom match** and enter `acme/*`, or select **Any
   value** to remove that constraint.
5. Wait for the broker to validate the scope and approve the request. Click the `create_issue`
   button again. The mock MCP server reports `Created demo issue #1`. Use **Always allow** in this
   demo because each button click opens a new MCP session.
6. Stop the stack with `just compose-down`.

---

## 8. Done checklist

- [ ] `just check` clean
- [ ] `just extproc-test` green under `-race`
- [ ] `just test-e2e-extproc` green, every spec scenario mapped 1:1
- [ ] `just test-e2e-backend` green — the token-exchange identity extension broke nothing
- [ ] `just verify` green
- [ ] Superseded `approval_required` unit tests rewritten; `ciba_required` coverage retained
- [ ] `examples/config/extproc-tool-approvals.yaml` present and referenced from `examples/config/README.md`
- [ ] `docs/guides/token-exchange-gateway.md` documents the approval flow and both new config sections
- [ ] `api/enduser/openapi.yaml` carries the token-exchange identity extension, confirmed by stakeholders
- [ ] `ARCHITECTURE.md` glossary updated (Approval Cache, Long-Poll Syncer, Approval Identity)
