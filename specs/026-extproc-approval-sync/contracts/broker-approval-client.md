# Broker client contract: ExtProc → identity broker approval endpoints

**Feature**: `026-extproc-approval-sync` | **Consumer**: `internal/extproc/approval/client.go`

ExtProc consumes three pre-existing feature-024 endpoints **as-is**. This document pins exactly what
ExtProc sends and how it interprets every response, so the client can be implemented and tested
without re-deriving the contract from broker source.

Base URL is `tool_approvals.url`; every path below is appended to it.

## HTTP safety and status errors

The approval HTTP client MUST set `CheckRedirect` to return `http.ErrUseLastResponse`. It MUST NOT follow a redirect for any approval request.

Every 3xx response is a broker error. The client returns a `StatusError` for it and makes no request to the redirect target. The syncer backs off after this error. A request-path operation returns a hard deny.

The client returns a `StatusError` for every HTTP response that the call site does not accept. The error exposes the response status so the gate can give a retry-suggesting denial for `429`.

`GET /api/approvals/{id}` is **not** part of this contract. It is browser-authenticated via
`requirePrincipal` (`internal/adapters/http/routing/enduser.go:98`) and is not callable with either
ExtProc credential. ExtProc MUST NOT call it (spec FR-007).

---

## Credentials

| Credential | Origin | Carried as |
|---|---|---|
| **Client assertion** | `TokenExchanger.ClientAssertion()` — the `id_token`/`access_token` from ExtProc's client-credentials grant, refreshed by the existing background ticker | `Authorization: Bearer …` on sync; `X-Client-Assertion: …` on create |
| **Subject token** | `requestState.bearerToken` — the caller's per-request bearer token | `Authorization: Bearer …` on create and consume |

`ErrAssertionExpired` from `ClientAssertion()` is a retryable condition: the syncer backs off; a
request-path call treats it as a broker-unavailable failure (fail closed).

---

## 1. `GET /api/approvals` — sync and targeted read

One endpoint serves three call sites: bootstrap (FR-003), the background long-poll (FR-004), and the
synchronous authoritative read on a match-miss (FR-005/FR-007).

**Auth**: client assertion only, in `Authorization: Bearer`. CEL-authorized by the broker with
`request.grant_type == "approval_sync"`.

**Query parameters**

| Parameter | Repeatable | Sent when |
|---|---|---|
| `principal` | no | Targeted read only. Never sent by bootstrap or long-poll. |
| `agent_session_id` | **yes** | Always, once per currently-active agent session. |

The broker reads `principal` via `Query().Get` and `agent_session_id` via `Query()["agent_session_id"]`
(`sync_handler.go:97-103`). No other query key is consumed; sending one is silently ignored.

Session-persistence approvals are returned **only** when their captured `agent_session_id` is among the
supplied values (`internal/domain/approval/service.go:526-536`). Permanent approvals are returned
unconditionally. Bootstrap therefore never receives session approvals — it precedes any live session
(FR-003).

**Request headers**

| Header | Bootstrap | Long-poll | Targeted read |
|---|---|---|---|
| `If-None-Match` | omitted | `"v{n}"` — last known ETag | omitted |
| `X-Long-Poll-Timeout` | omitted | `tool_approvals.long_poll_timeout_seconds` | omitted |

Omitting `If-None-Match` guarantees an immediate `200` with current state — that is what makes the
targeted read synchronous and the bootstrap non-blocking.

**Response handling**

| Status | ExtProc action |
|---|---|
| `200` | Body is a **full snapshot of the requested scope**, never a delta. Replace the state of every returned pair; store the `ETag`; reset backoff. |
| `304` | No body, no ETag. Re-poll immediately with the same ETag. Long-poll only. |
| `401` | `StatusError`. Log at warn and back off. Never retry without authentication. |
| `3xx` | `StatusError`. Do not follow the redirect. Back off for long-poll calls. Deny request-path calls. |
| any other status / transport error | A status response returns `StatusError`. The syncer backs off. The cache retains its last good state. |

**Critical**: there is no delta protocol and no stale-ETag retention window. The broker parses
`If-None-Match` as an optionally quoted, optionally `v`-prefixed `int64`; any value that is not exactly
the current version — older, newer, or unparseable — returns an immediate `200` with complete current
state (`sync_handler.go:105-118, 130-195`). Merging responses instead of replacing them would resurrect
revoked approvals.

**Targeted-read ETag ordering**: A targeted `200` response must include an ETag. ExtProc compares its version with the cache's known ETag. If the targeted version is older, ExtProc rejects the response, does not replace cache state, and returns a hard deny. It must not create an approval from that response (FR-019).

**Response body** — mirrors `syncResponse` (`sync_handler.go:24-50`):

```json
{
  "data": {
    "pairs": [
      {
        "principal": "alice@example.com",
        "agent_id": "3f2a9c1e-…",
        "approvals": [
          {
            "id": "9b1d…",
            "tool_name": "create_pull_request",
            "arguments_hash": "…",
            "tool_pattern": "create_pull_request",
            "params_pattern": {"repo": "acme/*"},
            "status": "approved",
            "persistence": "session",
            "consumed": false,
            "agent_session_id": "sess-abc",
            "approved_at": "2026-09-04T10:11:12Z"
          }
        ],
        "granted_permission_sets": {}
      }
    ]
  }
}
```

`granted_permission_sets` is always `{}` — a broker-side placeholder (`sync_handler.go:180-187`).
ExtProc **ignores** it; permission sets come from the token-exchange snapshot (spec FR-001).

`persistence` and `agent_session_id` are `omitempty` pointers and are absent while an approval is
pending. `params_pattern` is normalized to `{}` rather than `null` by `toSummary`
(`sync_handler.go:52-72`).

`approved_at` is **added by this feature** — broker change 2, see
[broker-api-changes.yaml](broker-api-changes.yaml). Today's summary carries no timestamp of any kind
(`sync_handler.go:40-73), leaving FR-015's decision-time tie-break with no data source. ExtProc uses
it for ExtProc-local candidate selection in `Cache.Match`. A record with `status: "approved"` and no
`approved_at` is treated as **unmatchable** and logged as a broker-contract warning. ExtProc never
ranks it with a zero timestamp because an older approval could win a tie against a newer one.

---

## 2. `POST /api/approvals` — create

**Auth**: dual. Subject token in `Authorization: Bearer`, client assertion in `X-Client-Assertion`
(`internal/adapters/http/middleware/approval_auth.go:65-90, 162-177`). The broker derives principal and
agent id from the verified subject token; ExtProc sends neither.

**Request body** — mirrors `createRequest` (`create_handler.go:19-41`):

```json
{
  "metadata": {
    "mcp_session_id": "…",
    "agent_session_id": "…",
    "tool_invocation_id": "…",
    "description": "Approval required for create_pull_request"
  },
  "tool_name": "create_pull_request",
  "arguments": {"repo": "acme/app", "title": "Add retry"},
  "risk_level": "medium"
}
```

Broker-enforced requirements (`create_handler.go:75-91`): `metadata` present,
`metadata.description` present, `tool_name` non-empty, `arguments` non-nil. ExtProc supplies a
deterministic description fallback (`"Approval required for <tool_name>"`) when OPA's optional
`approval_context` omits one — otherwise the broker rejects the request with `400`.

`risk_level` is optional; an unknown non-empty value is coerced to `critical` by the broker
(`internal/domain/approval/service.go:400-410`). ExtProc forwards OPA's `approval_context.risk_level`
when present and omits the field otherwise.

**Response handling**

| Status | ExtProc action |
|---|---|
| `201` | New pending approval. Use `data.approval_url` in the elicitation. |
| `200` | An equivalent pending approval already existed (broker dedup). **Identical handling** — use the returned `approval_url`. |
| `400` | `StatusError`. Log at error and deny. |
| `401` | `StatusError`. Log at warn and deny. |
| `429` | `StatusError`. Return a hard deny that tells the agent to retry later. |
| any other status / transport error | A status response returns `StatusError`. Deny (spec Edge Cases). |

**Never fabricate an elicitation.** Without a broker-supplied `approval_url` there is no URL to give
the agent, so a failed create returns a deny tool-result error stating that approval could not be
initiated (spec Edge Cases, "Broker unavailable during approval creation").

**Idempotency**: the broker dedups pending records on
`(principal, agent_id, tool_name, arguments_hash)` via a partial unique index
(`migrations/024_create_tool_approvals.up.sql:28-31`) with `ON CONFLICT … DO NOTHING` plus a re-select
(`postgres/tool_approval_repository.go:115-252`). This is the final backstop for the residual race
between the FR-005 read and this call.

---

## 3. `POST /api/approvals/{id}/consume` — one-time consumption

**Auth**: subject token only, in `Authorization: Bearer`. The client assertion is **not** sent
(`approval_auth.go:117-139`). The broker verifies the token's principal owns the approval.

**Request body**: none. Sending one is a contract violation.

**Response handling**

| Status | Meaning | ExtProc action |
|---|---|---|
| `200` | Consumed, or already consumed (idempotent) | Confirm the reservation → mark `Consumed`; **only now** proceed |
| `403` | Consuming principal is not the owner | `StatusError`. Release reservation and deny |
| `404` | Unknown approval — cache is stale | `StatusError`. Release reservation and deny |
| `422` | Not a consumable `once` approval | `StatusError`. Release reservation, deny, and log a matching bug |
| any other status / transport error | Unknown | A status response returns `StatusError`. Release reservation and deny |

**Fail closed (FR-008 item 4)**: the request proceeds **only** after a confirmed `200`. A broker that
never records the consume would keep the approval `approved` and re-deliver it to every replica
indefinitely, turning a one-time grant into unbounded fleet-wide reuse — a Principle I violation.
Releasing the reservation on failure preserves retryability once the broker recovers.

**Response body** (informational only; ExtProc keys on the status code):

```json
{"data": {"id": "9b1d…", "consumed": true, "consumed_at": "2026-09-04T10:11:12Z"}}
```

---

## 4. Call-site matrix

| Call site | Endpoint | Auth | `principal` | `agent_session_id` | `If-None-Match` | Blocking |
|---|---|---|---|---|---|---|
| Bootstrap (FR-003) | `GET /api/approvals` | assertion | no | yes (none active yet) | no | no |
| Long-poll (FR-004) | `GET /api/approvals` | assertion | no | yes | yes | yes |
| Authoritative read (FR-005/FR-007) | `GET /api/approvals` | assertion | **yes** | yes | no | no |
| Create (FR-006) | `POST /api/approvals` | subject + assertion | — | in body metadata | — | no |
| Consume (FR-008) | `POST /api/approvals/{id}/consume` | subject | — | — | — | no |

---

## 5. Error taxonomy surfaced to the agent

| Condition | Agent-visible result |
|---|---|
| OPA `deny` / `ciba_required` / undefined | 403 `access_denied` — no approval created (FR-002/FR-012/FR-014) |
| `approval_required`, no match, create succeeded | 200 JSON-RPC `-32042` with `approval_url` |
| `approval_required`, create failed | 403 `access_denied` — "approval could not be initiated" |
| `approval_required`, stale or never-synced cache, authoritative read failed, malformed, or returned an older ETag | 403 `access_denied` — no approval created (FR-019) |
| `approval_required`, create returned `429` | 403 `access_denied` — retry later |
| `approval_required` inside a batch | 403 `access_denied` for the whole batch, with a reason instructing a standalone re-issue (FR-016) |
| `once` match, consume failed | 403 `access_denied` (FR-008 item 4) |
| Approval identity unavailable from the exchange | 403 `access_denied` — "approval identity unavailable" (research R-000) |
