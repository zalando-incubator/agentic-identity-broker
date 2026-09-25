# Phase 0 Research: ExtProc Approval Cache Sync & OPA Integration

**Feature**: `026-extproc-approval-sync` | **Date**: 2026-09-04 | **Spec**: [spec.md](spec.md)

All findings below are grounded in the worktree source. Each decision records what was chosen, why,
and which alternatives were rejected.

---

## R-000: How ExtProc obtains a verified `(principal, agent_id)` for cache scoping

**Status**: Resolved — additive broker change, confirmed and in scope as spec **FR-017**.

### The gap

The spec assumes throughout that ExtProc knows the request's `principal` and `agent_id`:

- the approval cache is keyed by `(principal, agent_id)` (spec Key Entities, FR-005, FR-010);
- the authoritative broker read is `GET /api/approvals?principal={principal}` (FR-005, FR-007);
- SC-004 is stated per `(principal, agent, tool_pattern)`.

The spec never states how ExtProc derives those values, and **ExtProc cannot derive them today**. A
repository-wide search for `principal` across `internal/extproc/**` and `cmd/extproc-token-exchange/**`
returns zero matches in production code. The per-stream state carries only the raw bearer token:

```go
// internal/extproc/server/server.go:83-91
type requestState struct {
	bearerToken           string
	resourceURI           string
	headers               map[string]string
	protocol              string
	grantedPermissionSets map[string][]string
	requestContext        context.Context
	finishObservation     func(outcome, resourceURI, errorType string)
}
```

The broker derives principal and agent id itself, from the *verified* subject token, using
`token_exchange.claim_extraction.principal_expression` and `.agent_id_expression`
(`internal/ports/config.go:803-820`). ExtProc has no JWKS client, no JWT validator, and no claim
extraction configuration (confirmed: no `jwks` symbol exists anywhere under `internal/extproc/`).

### Decision

**Extend the RFC 8693 token-exchange response with broker-authoritative identity metadata**, exactly
mirroring the existing `granted_permission_sets` extension that feature 020 already added to the same
response. The broker emits the principal and agent id it *already computed from the verified subject
token* during the exchange it just authorized:

```go
// internal/extproc/server/exchanger.go — extended
type tokenExchangeResponse struct {
	AccessToken           string              `json:"access_token"`
	TokenType             string              `json:"token_type"`
	ExpiresIn             *int                `json:"expires_in"`
	GrantedPermissionSets map[string][]string `json:"granted_permission_sets,omitempty"`
	Principal             string              `json:"principal,omitempty"`  // NEW
	AgentID               string              `json:"agent_id,omitempty"`   // NEW
}
```

`server.ExchangeResult` gains the matching `Principal` and `AgentID` fields and carries them into
`requestState`, on the same cache window as the access token and permission-set snapshot.

**Fail-closed rule**: when the exchange response omits either field, ExtProc has no approval identity.
It MUST NOT infer one, MUST NOT consult the cache, MUST NOT issue a `?principal=` read, and MUST NOT
create an approval it cannot attribute. An `approval_required` decision in that state returns a deny
tool-result error with a distinct operator-facing reason (`approval identity unavailable`) and a
warning log. This is a deployment/version misconfiguration, and silently degrading to
"always elicit" would mask it while producing an unbreakable elicitation loop.

### Rationale

- The values are produced by the broker from a signature-verified subject token inside the request it
  authorized, and reach ExtProc over the same authenticated TLS channel as the access token. They are
  never caller-controlled.
- It reuses the exact extension precedent already shipped and consumed on this response
  (`granted_permission_sets`), so it introduces no new transport, no new endpoint, and no new
  round-trip on the hot path.
- ExtProc gains no JWT parsing, no JWKS fetching, and no claim-extraction configuration surface.
- The body-bearing ordering already guarantees the exchange precedes OPA, so the identity is always
  available before the first approval decision for the requests this feature gates.

### Alternatives rejected

| Alternative | Rejected because |
|---|---|
| **Parse the caller-supplied `subject_token` JWT without verifying its signature** and read configured principal/agent claims | Directly violates Constitution Principle I ("Signature validation is NEVER optional"). A forged or malformed claim would select another user's cached approval and forward a token past the approval gate. Even granting that the preceding exchange would reject an unverifiable token, this makes an authorization-relevant cache key depend on unverified caller input — an unacceptable dependency for a privilege-escalation boundary. |
| **Parse the exchanged (broker-issued) access token** and read `sub` / `agent_id` | RFC 8693 permits an opaque access token, and `ExchangeResult` carries no `issued_token_type`; nothing in the current contract guarantees a JWT. Worse, the broker's local token claims are CEL-policy owned (`token_claims_expression`), so `sub` is not contractually the principal. This binds ExtProc to a policy-mutable claim layout that no test or ADR pins. |
| **Key the cache by the subject token itself** (mirroring `tokenCacheKey`) and always issue unfiltered `GET /api/approvals` | Contradicts FR-005/FR-007, which mandate the `?principal=` filter. An unfiltered read returns every active approval for every principal to every replica on every match-miss — a data-exposure and scaling regression. It also cannot satisfy FR-010's per-pair idle eviction. |
| **A new broker identity-introspection endpoint** called by ExtProc per request | Adds a second synchronous broker round-trip on the hot path (breaks SC-001/SC-004), a new endpoint, and a new auth boundary — all to return data the exchange already computed. |

### Consequence for the spec

Spec Assumptions previously stated that broker endpoints are "consumed as-is" and that "no
broker-side work is required". That was **false for this identity binding**, and the spec has been
corrected. This decision is now binding requirement **FR-017**, with acceptance scenarios under
User Story 5: two optional response fields on `POST /oauth2/token` for the token-exchange grant, plus
the matching `api/enduser/openapi.yaml` schema update. R-014 contributes the second broker change
(**FR-018**, `approved_at` on the approval sync summary). Both are specified in
[contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml); Principle X confirmation is
recorded in the spec's 2026-09-04 Clarifications session.

---

## R-001: Where the approval logic lives

**Decision**: A new ExtProc-local package `internal/extproc/approval/` with four files —
`client.go` (broker HTTP client), `cache.go` (in-memory store + matching + CAS), `syncer.go`
(long-poll goroutine), `gate.go` (per-request orchestration). `internal/extproc/server` gains one
optional field and one decision branch.

**Rationale**: `internal/extproc/` currently has exactly three packages — `config`, `authorization`,
`server` (`internal/extproc/AGENTS.md:11-31`). Approval sync is a distinct concern with its own
lifecycle (a background goroutine), its own outbound protocol (three broker endpoints), and its own
state (a cache). Folding it into `server` would make an already 1240-line file the owner of an HTTP
client and a cache; folding it into `authorization` would couple OPA evaluation to broker I/O, which
the spec explicitly forbids (FR-001: approval records MUST NOT enter the OPA input).

**Alternatives rejected**: extending `server/exchanger.go` (conflates RFC 8693 exchange with approval
REST); a generic "broker client" package (premature — the only other broker call is the exchange,
which has entirely different caching, circuit-breaking, and singleflight semantics).

---

## R-002: Reuse of `internal/toolpattern` from ExtProc

**Decision**: `internal/extproc/approval/cache.go` imports `internal/toolpattern` directly and uses
`toolpattern.Matches` for per-record matching and `toolpattern.SelectBest` for precedence. Cross-service
agreement is pinned by the embedded vectors via `toolpattern.MatchVectors()`,
`toolpattern.CanonicalVectors()`, and `toolpattern.PrecedenceVectors()`.

**Rationale**: ADR 035 is accepted and names this exact import as the single sanctioned exception to
ExtProc's broker-package isolation:

> `internal/toolpattern` is a neutral, dependency-free leaf package that both `internal/domain/*` and
> `internal/extproc/*` may import. It owns the approval-pattern grammar, canonicalization, matching,
> formatting, precedence, and embedded language-neutral test vectors.
> — `adrs/035-shared-tool-pattern-matching.md:13-17`

`internal/extproc/AGENTS.md:118` already records the exception. The relevant surface is:

```go
func Matches(toolPattern string, paramsPattern map[string]string, toolName string, args map[string]any) bool
func SelectBest(candidates []Candidate, toolName string, args map[string]any) int
type Candidate struct { ID string; ToolPattern string; ParamsPattern map[string]string; DecidedAt time.Time }
```

`SelectBest` returns the index of the winning candidate or `-1`, ranking exact-tool over wildcard,
then more constrained params, then fewer wildcards, with ties broken by later `DecidedAt` then
lexicographically smaller `ID` (`internal/toolpattern/toolpattern.go:202-283`). That final ID
tie-breaker makes cache selection deterministic across replicas.

**Alternatives rejected**: reimplementing glob matching inside ExtProc (ADR 035 exists precisely to
prevent broker/ExtProc divergence); passing patterns into OPA (FR-001 forbids approval records in the
OPA input).

---

## R-003: OPA decision contract — activating `approval_required`

**Decision**: Introduce exported action constants in `internal/extproc/authorization` and stop
collapsing `approval_required`:

```go
const (
	ActionAllow            = "allow"
	ActionDeny             = "deny"
	ActionApprovalRequired = "approval_required"
	ActionCIBARequired     = "ciba_required"
)
```

`ParseDecision` returns `ActionApprovalRequired` verbatim. `ActionCIBARequired` retains today's
behavior: normalized to deny, logged with the raw action and `result_code=unsupported_action`.
Undefined, malformed, and unknown actions remain fail-closed deny.

**Rationale**: `internal/extproc/authorization/decision.go:51-55` currently maps both future actions to
deny with reason `"approval_required is not yet supported"` — the shipped feature-020 FR-016 behavior.
Feature 026 supersedes FR-016 for exactly one action (spec FR-002, Clarification Q33). Making the
action a named constant removes the string literals now scattered across `decision.go`,
`authorizer.go`, and `server.go`.

**Consequence**: `internal/extproc/authorization/decision_test.go:28-32`
(`TestParseDecision_ApprovalRequired_MappedToDeny`) and
`TestOPAAuthorizer_ApprovalRequired_LogsRawActionAndReturnsDeny`
(`authorizer_test.go:455-472`) assert the superseded behavior and must be rewritten as part of this
feature — the `ciba_required` half of each is retained.

---

## R-004: Approval-cache design

**Decision**: `map[pairKey]*pairEntry` behind a `sync.RWMutex`, following ADR 012's precedent rather
than `sync.Map`.

```go
type pairKey struct {  // comparable struct key — no separator injection
	principal string
	agentID   string
}
```

Each `pairEntry` holds the pair's approval records, a `lastSeen time.Time` for FR-010 idle eviction,
and per-record reservation state for FR-008's compare-and-swap. Eviction runs on a single background
ticker owned by the syncer goroutine; there is no second worker.

**Rationale**: ADR 012 chose `map` + `sync.RWMutex` + a periodic eviction ticker and explicitly
rejected `sync.Map` because iteration and deletion make periodic eviction less auditable
(`adrs/012-extproc-in-memory-token-cache.md:23-44, 78-82`). The existing `TokenExchanger` cache is the
in-tree reference (`internal/extproc/server/exchanger.go:136-159, 585-599`). Matching this pattern
keeps ExtProc's two caches structurally identical for reviewers.

**One-time reservation (FR-008)**: reservation is a per-record boolean flipped under the write lock —
a genuine compare-and-swap within one instance. Confirmed on a `200 OK` consume; released on any
consume failure so a later retry can re-attempt. Cross-replica at-most-once is explicitly not
guaranteed (spec FR-008 item 3).

**Alternatives rejected**: `sync.Map` (ADR 012 rejection stands); an LRU with a size cap (the spec
specifies idle-TTL eviction, and a size cap would silently drop permanent approvals, degrading SC-004);
sharing the token cache (different key, different lifetime, different invalidation source).

---

## R-005: Long-poll client protocol

**Decision**: One background goroutine. Each iteration issues `GET {tool_approvals.url}/api/approvals`
with `Authorization: Bearer {client_assertion}`, `If-None-Match: {lastETag}`,
`X-Long-Poll-Timeout: {long_poll_timeout_seconds}`, and one repeated `agent_session_id` query parameter
per currently-active session. HTTP client timeout is `long_poll_timeout_seconds + a fixed margin`, so a
server-side hold never trips the client timeout.

- `200 OK` → replace the state of every returned `(principal, agent_id)` pair atomically, store the new
  `ETag`, reset backoff, re-poll immediately.
- `304 Not Modified` → re-poll immediately with the same ETag.
- any error / non-2xx → exponential backoff 1s → 60s, then re-poll.

**Rationale**: ADR 014 fixes the server contract — the ETag is the global `approval_sync_state.version`
rendered as `"v{version}"`, and a matching ETag blocks while a differing or absent one returns current
state immediately (`adrs/014-long-poll-listen-notify.md:24-32`). The handler clamps
`X-Long-Poll-Timeout` to 120s and defaults invalid values to 30s
(`internal/adapters/http/handlers/approval/sync_handler.go:19-22, 120-128`).

**Critical finding — there are no deltas and no retention window**. Spec US4 scenario 3 says the broker
"returns all changes since that ETag (or the full state if the ETag is stale beyond a server-side
retention window)". That retention window **does not exist**. The handler parses `If-None-Match` as an
optionally quoted, optionally `v`-prefixed `int64`; *any* value that is not exactly equal to the current
version — older, newer, or unparseable — yields an immediate `200` with the complete current state
(`sync_handler.go:105-118, 130-195`). `Service.GetSyncState` always returns full active state grouped by
`(principal, agent_id)` (`internal/domain/approval/service.go:527-573`).

Therefore ExtProc MUST treat every `200` as a **full-state snapshot for the requested filter scope**, not
an incremental patch, and MUST replace rather than merge the returned pairs. US4 scenarios 3 and 4
collapse to the same observable behavior, which simplifies the implementation and removes the
"stale ETag" special case entirely.

**Backoff attribution**: ADR 014's `1s → 30s` backoff governs the *broker's* PostgreSQL
`ApprovalSyncSubscriber` reconnect (`adrs/014-long-poll-listen-notify.md:93-96`), not this HTTP client.
Feature 026's `1s → 60s` client backoff is independent and does not contradict it.

---

## R-006: Client assertion for the sync and create calls

**Decision**: Add one exported accessor to `TokenExchanger` and consume it through a narrow interface
declared in the `approval` package:

```go
// internal/extproc/server
func (te *TokenExchanger) ClientAssertion() (string, error)  // ErrAssertionExpired when nil/expired

// internal/extproc/approval
type AssertionProvider interface {
	ClientAssertion() (string, error)
}
```

**Rationale**: ExtProc does not sign its own assertion. `refreshClientAssertion` performs a
`clientcredentials` grant and stores the resulting `id_token` or `access_token` in
`atomic.Pointer[assertionState]`, refreshed by an existing background ticker
(`internal/extproc/server/exchanger.go:510-559, 562-583`). Duplicating that flow in the approval client
would double the credential traffic and create two independently-expiring assertions. One accessor over
the existing lock-free atomic read is the minimal change, and the interface is declared by the consumer,
preserving hexagonal direction (`server` imports `approval`; `approval` imports nothing from `server`).

`ErrAssertionExpired` already exists as the sentinel for this condition
(`internal/extproc/AGENTS.md:107`). The syncer treats it as a retryable error and backs off.

---

## R-007: MCP `URLElicitationRequiredError` emission

**Decision**: Reuse the existing machinery. Extract the URL/message construction out of
`urlElicitationResponse` into a parameterized helper and call it from both the existing broker re-auth
path and the new approval path, passing the **real** JSON-RPC request id.

**Rationale**: The elicitation response already exists and is exercised end-to-end
(`internal/extproc/server/server.go:1123-1156`); it builds `mcp.URLElicitationRequiredError` with
`mcp.ElicitationModeURL`, a UUID elicitation id, and returns HTTP 200 carrying the JSON-RPC error
(code `-32042`). No new type or dependency is needed.

Current signature and its limitation:

```go
func urlElicitationResponse(brokerErr *BrokerExchangeError, rawID json.RawMessage) *extprocv3.ProcessingResponse
```

Every existing caller passes `rawID = nil` because token exchange happens before the body is read, so
today's re-auth elicitation always emits `id: null` (`server.go:789, 1146-1148`). The approval path is
different: it fires in the **body** phase, where `authorization.ParseMCPMessage` has already parsed the
request id. Passing the real raw id is required for a conformant JSON-RPC response and costs nothing —
the plumbing to preserve the id token verbatim already exists.

**Alternatives rejected**: defining an ExtProc-local elicitation type (duplicates `mcp-go`); returning
403 with an `approval_url` body (agents would not surface the URL; the spec mandates `-32042`).

---

## R-008: Ordering — exchange before OPA for body-bearing requests

**Decision**: No change. Keep the shipped ordering; document it as the ordering this feature relies on.

**Rationale**: `Server.Process` already dispatches body-bearing OPA requests to
`processRequestHeadersOPA`, which calls `Exchanger.Exchange` in the headers phase and requests a
BUFFERED body, then evaluates OPA in `processRequestBody`
(`internal/extproc/server/server.go:300-306, 557-571, 579-627`). Header-only `mcp_headers_only`
requests keep OPA-before-exchange with `granted_permission_sets_available: false`
(`server.go:633-678`; `authorization/input_builder.go:92-118`). ADR 028 defines this pipeline
(`adrs/028-opa-extproc-authorization.md:67-79`), and the spec's 2026-09-03 clarification adopts it.

This ordering is also what makes R-000 workable: the identity and permission-set snapshot are both
present before the first approval decision. On deny or elicitation the exchanged token is simply
discarded and never forwarded.

---

## R-009: Batch requests

**Decision**: Extend the existing deny-wins aggregation. `processRequestBodyBatch` already evaluates
every message independently and accumulates `denied` plus `denyReasons`
(`internal/extproc/server/server.go:681-741`). Add one branch: a message returning
`ActionApprovalRequired` sets `denied` and appends a fixed reason instructing the agent to re-issue that
specific tool call as a standalone, non-batch request. No approval is created and no elicitation is
emitted inside a batch.

**Rationale**: Spec FR-016 and feature 020 FR-023. Elicitation is a single-tool-call interaction; a
JSON-RPC batch has no place to carry one elicitation per element.

---

## R-010: Configuration surface

**Decision**: Two new top-level sections on the ExtProc `Config` root
(`internal/extproc/config/config.go:9-17`):

```yaml
tool_approvals:
  url: ""                        # required when enabled — identity broker base URL
  enabled: false                 # explicit opt-in; false keeps today's behavior exactly
  long_poll_timeout_seconds: 30
  approval_cache_idle_ttl: 5m
  request_timeout: 5s
sessions:
  extraction:
    http_header: "Mcp-Session-Id"
```

Validation (fail-fast at startup, joining `Validate()`'s existing numbered rules in
`internal/extproc/config/validate.go`):

1. `tool_approvals.enabled` requires `authorization.enabled` — approval gating is meaningless without
   an OPA decision to gate on.
2. `tool_approvals.url` is required when enabled and MUST be an absolute HTTP(S) URI with a host and no
   query or fragment; HTTP is permitted only under the existing `oauth2.tls.allow_http` development flag.
3. `long_poll_timeout_seconds` in `[1, 120]` — the broker clamps above 120 (`sync_handler.go:19-22`),
   so a larger value is silently ineffective and must be rejected rather than accepted.
4. `approval_cache_idle_ttl` > 0.
5. `request_timeout` > 0 and strictly less than `long_poll_timeout_seconds` is **not** required — the
   long-poll uses its own derived deadline (R-005); `request_timeout` governs create/consume/targeted-read.
6. `sessions.extraction.http_header` non-empty and a valid HTTP token.

**Rationale**: Principle VII requires the unified config port — for ExtProc that port is its own
standalone schema, explicitly separate from `internal/ports/config.go`
(`internal/extproc/config/config.go:1-5`; ADR 011). Loading precedence, `EXTPROC_` env mapping,
`${VAR}` expansion, and Cobra flags are handled by the existing `LoadWithCommand` pipeline
(`internal/extproc/config/loader.go:29-40, 93-149`); the new sections only add defaults, flags, and
validation rules.

An explicit `enabled` flag is added beyond the spec's Assumptions list: the spec derives activation
from `tool_approvals.url` being set, but every other optional ExtProc subsystem (`authorization`,
`telemetry`, `circuit_breaker`) uses an explicit boolean. Matching that convention keeps
`Validate()` uniform and makes "approval gating is off" auditable in one line of config.

**Helm chart**: `charts/agentic-identity-broker/` contains **no** ExtProc references (verified: zero
matches for `extproc` across the chart). ExtProc is deployed from `Dockerfile.extproc` and configured
via `EXTPROC_*` env / `EXTPROC_CONFIG_PATH`, not by this chart. Principle VII's Helm requirement is
therefore not applicable to these keys; `examples/config/` and `docs/` remain in scope.

---

## R-011: OPA input additions

**Decision**: Add one field to `ContextInput` and thread the extracted agent session id through
`BuildOPAInput`:

```go
// internal/extproc/authorization/input.go
type ContextInput struct {
	GrantedPermissionSetsAvailable bool                `json:"granted_permission_sets_available"`
	GrantedPermissionSets          map[string][]string `json:"granted_permission_sets,omitempty"`
	AgentSessionID                 string              `json:"agent_session_id,omitempty"`  // NEW
}
```

`BuildOPAInput`'s trailing `grantedPermissionSets map[string][]string` parameter is replaced by a
`ContextInput` value, so future context fields do not grow the signature again. All call sites
(`server.go:607`, tests) are updated in the same change.

`input.mcp.session_id` is **unchanged**: it stays the MCP protocol session read from the fixed
`Mcp-Session-Id` header by `extractSessionID` (`authorization/input_builder.go:113-114, 128-135,
162-170`). The new `input.context.agent_session_id` is read from the configurable
`sessions.extraction.http_header` and is the value used for approval scoping and for the
`agent_session_id` create metadata.

**Rationale**: Spec FR-013 and the 2026-09-03 clarification. The two fields coincide by default and
diverge only when an operator maps a different header. Folding them was explicitly rejected in the
spec because `input.mcp.session_id` is already asserted by shipped code and tests
(`internal/extproc/authorization/input_builder_test.go`) and by feature 020's contract.

---

## R-012: Testing approach

**Decision**: Extend the existing ExtProc Ginkgo suite at `tests/e2e/extproc/`; add a composed
broker+ExtProc bootstrap for the sync scenarios.

**Rationale**: An ExtProc E2E harness already exists — `tests/e2e/extproc/extproc_suite_test.go`
(`RunSpecs(t, "ExtProc Token Exchange E2E Suite")`), with `bootstrap/`, `fixtures/`, and `helpers/`
subpackages, driven by `just test-e2e-extproc`. A new suite would fragment it.

However, the default `TestEnvironment` deliberately stands up only two `httptest` mocks — a
client-credentials `MockOAuth2Server` and an RFC 8693 `MockTokenExchangeServer`
(`tests/e2e/extproc/bootstrap/bootstrap.go:92-100`) — so it cannot exercise real ETag long-poll
semantics, real `304` behavior, or the broker's dedup index.

Two layers are therefore needed:

1. **Broker-shaped `httptest` fake** in `tests/e2e/extproc/bootstrap/` for deterministic control of
   error paths, backoff, consume failures, and auth assertions. Pattern copied from
   `internal/extproc/server/exchanger_test.go:42-160` (`mockServers` + `configForMocks`).
2. **Composed real broker** for US3/US4 and SC-002: a new bootstrap helper that builds the production
   broker end-user server via `tests/e2e/bootstrap` (`NewStorageFactory` → `BuildApp` →
   `NewEndUserTestServer`) and points ExtProc's `tool_approvals.url` at it. Test packages are not
   subject to ExtProc's production import isolation, so this composition is legal. It exercises the
   real `SyncHandler`, real `ApprovalSyncBroadcaster`, and real dedup index against memory storage.

Signed machine credentials are already available and reusable:
`helpers.NewApprovalRequestAuthFixture`, `helpers.ApprovalCreateHeaders`, `helpers.ApprovalSyncHeaders`,
`helpers.ApprovalSubjectTokenHeaders` (`tests/e2e/helpers/approval_auth.go:11-103`), together with the
wire types in `tests/e2e/helpers/approval_types.go:5-143` — whose `ApprovalSummary` already carries
`ToolPattern`, `ParamsPattern`, `Persistence`, `Consumed`, and `AgentSessionID`.

**Policy fixture gap**: `tests/e2e/extproc/fixtures/policies/` contains `allow_readonly.rego`,
`allow_all.rego`, `deny_all.rego`, and `assert_us2_input_shapes.rego` — none emits
`approval_required`. A new `approval_required.rego` fixture is required, following the existing
`policyPath(name)` convention (`tests/e2e/extproc/opa_authorization_test.go:42-66`).

---

## R-013: No new ADR required

**Decision**: Feature 026 needs no new ADR. Every architectural choice it makes is already governed by
an accepted decision.

| Concern | Governing ADR |
|---|---|
| ETag long-poll, version semantics, multi-instance wake-up | ADR 014 |
| Per-endpoint create/sync/consume authentication split | ADR 018 |
| In-process cache concurrency and eviction | ADR 012 |
| Exchange-before-OPA body-bearing pipeline | ADR 028 |
| ExtProc importing `internal/toolpattern` | ADR 035 |
| ExtProc as a standalone binary with its own config schema | ADR 011 |

The highest existing ADR number is **035**; the next free number is 036 if one becomes necessary.

**Caveat**: the two broker changes (R-000 identity → FR-017, R-014 `approved_at` → FR-018) are *API*
changes, not architectural ones — they are governed by Principle X (confirmation recorded in the
spec's 2026-09-04 Clarifications session) and recorded in `api/enduser/openapi.yaml`, not by a new
ADR. Had the identity extension been rejected in favor of a different mechanism, that replacement
decision would have warranted ADR 036.

---

## R-014: FR-015's decision-time tie-break has no data source

**Status**: Resolved — additive broker change, confirmed and in scope as spec **FR-018**.

### The gap

Spec FR-015 requires that when several cached records match one invocation, "the most specific wins
(exact tool > wildcard tool; then more constrained params; then fewer wildcards; **ties broken by
later decision time**) via `toolpattern.SelectBest`". The shared matcher implements exactly that:
equal-rank candidates are ordered by later `Candidate.DecidedAt`, then by lexicographically smaller
`Candidate.ID` (`internal/toolpattern/toolpattern.go:266-283`).

**The sync response carries no timestamp of any kind.** `toolApprovalSummary` is exactly
`id`, `tool_name`, `arguments_hash`, `tool_pattern`, `params_pattern`, `status`, `persistence`,
`consumed`, `agent_session_id` (`internal/adapters/http/handlers/approval/sync_handler.go:40-50`), and
`toSummary` drops `CreatedAt`, `ApprovedAt`, `DeniedAt`, and `ConsumedAt` (`sync_handler.go:52-73`).
The OpenAPI `ToolApprovalSummary` schema matches.

Without a source, every ExtProc candidate would carry a zero `DecidedAt`, so every tie would fall
through to the ID tie-breaker. That is deterministic but **arbitrary**: a lexicographically smaller
approval id would beat a more recent user decision. It fails FR-015 silently — no error, no log, just
the wrong winner — and it diverges from how the broker reads the same records.

### Decision

**Project `ApprovedAt` into the sync summary as `approved_at`** (RFC 3339, `omitempty`).
`ApprovalRecord.DecidedAt` is populated from it and becomes `toolpattern.Candidate.DecidedAt`.

**Fail-closed rule**: a record with `status == "approved"` but no `approved_at` is treated as
**unmatchable** and logged as a broker-contract warning. It is never ranked with a zero timestamp.

### Rationale

- The value already exists on the aggregate as `ToolApproval.ApprovedAt`
  (`internal/domain/storage/tool_approval.go:70-94`), with the column present since migration 024.
  This is a projection, not new state: no migration, no query change, no new domain logic.
- `approved_at` rather than a generic `decided_at`: only `approved` records are matchable
  (data-model.md matchability rule #1), so every ranking candidate has an approval timestamp. A
  generic field would have to encode denial time too, which no consumer needs.
- Additive and `omitempty`, so it is invisible to existing consumers — the same convention as
  change 1 and as feature 020's `granted_permission_sets`.

### Alternatives rejected

| Alternative | Rejected because |
|---|---|
| Leave `DecidedAt` zero and rely on the ID tie-breaker | Violates FR-015 silently and non-obviously; picks by UUID ordering instead of user intent. The failure is invisible in production and only surfaces as an inexplicably wrong approval winning a tie. |
| Have ExtProc stamp `DecidedAt` at the moment a record first appears in a sync response | First-observation time is not decision time. It varies per replica, resets on restart and on idle eviction, and inverts ordering whenever a bootstrap delivers old and new approvals in one snapshot. |
| Drop the tie-break and rank ties by ID only | Requires amending FR-015 and the shared matcher's documented precedence, which the broker also relies on. Changing `toolpattern` semantics to work around a missing field would break the ADR 035 parity guarantee in the opposite direction. |
| Fetch full approval detail per record | `GET /api/approvals/{id}` is browser-authenticated and not machine-callable (spec FR-007); and a per-record round trip on the hot path breaks SC-001/SC-004. |

### Consequence

Feature 026 carries **two** broker changes, not one. Both are additive, optional-valued, confirmed
under Principle X, and bound as spec FR-017 / FR-018 with acceptance scenarios under User Story 5.
Schemas: [contracts/broker-api-changes.yaml](contracts/broker-api-changes.yaml).

---


## Resolved unknowns summary

| Unknown | Resolution |
|---|---|
| How ExtProc obtains `(principal, agent_id)` | R-000 — additive broker exchange-response fields; fail closed when absent |
| Whether sync `200` responses are deltas | R-005 — **no**; always a full snapshot of the requested scope; replace, never merge |
| Whether a stale-ETag retention window exists | R-005 — **no**; any non-matching ETag returns current state immediately |
| Where the client assertion comes from | R-006 — new `TokenExchanger.ClientAssertion()` accessor behind an `AssertionProvider` interface |
| Whether elicitation machinery exists | R-007 — **yes**; reuse and parameterize it, and pass the real JSON-RPC id |
| Whether an ExtProc E2E harness exists | R-012 — **yes**; extend `tests/e2e/extproc/`, add a composed-broker bootstrap |
| Whether a new ADR is needed | R-013 — **no** |
| Source of FR-015's decision-time tie-break | R-014 — **none today**; additive `approved_at` on the sync summary; unmatchable when absent |
| Helm chart impact | R-010 — none; the chart does not deploy or configure ExtProc |
