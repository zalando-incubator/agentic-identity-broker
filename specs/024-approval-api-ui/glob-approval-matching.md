# Broker Extension: Glob Pattern Approval Matching

**Type**: Prerequisite change spec for the identity broker (feature `024-approval-api-ui` follow-up)
**Created**: 2026-09-04
**Status**: Proposed — requires API sign-off (Constitution IV/X)
**Blocks**: `specs/026-extproc-approval-sync/spec.md` FR-015 (glob approval matching)
**Design source of truth**: `specs/design-permission-sets-tool-authorization.md` §4.7 (Persistence Modes), §4.8 (Tool Matching: Pattern Semantics), §5.1 (sync payload)

---

## 1. Context & Problem

The tool-authorization **design** specifies glob pattern matching for approvals as a deliberate,
user-facing capability (design §4.8):

1. Users see **readable** tool names/params in the Approval UI (not opaque hashes).
2. A single approval can cover a **family** of calls — e.g. "always allow `create_pull_request`
   on any repo in `acme/*`", or `issues.*`, or `*` (within the permission-set boundary).

Persistence breadth is tied to the pattern (design §4.7 ExtProc check order):

| Mode | Stored key | Coverage |
|---|---|---|
| `once` | `(user, agent, tool_name, params)` exact | one specific invocation, consumed on use |
| `session` | `(user, agent, session_id, tool_pattern)` | any params in the session for the tool |
| `permanent` | `(user, agent, tool_pattern)` | possibly wildcarded (`repo=acme/*`, `issues.*`) |

**Feature 024 as shipped did not implement the pattern model.** It persists exact
`tool_name` + `arguments` + `arguments_hash` (`internal/adapters/storage/.../tool_approval.go`),
the Approval UI offers `once`/`session`/`permanent` with **no** pattern-breadth control, and the
machine sync DTO (`toolApprovalSummary` in
`internal/adapters/http/handlers/approval/sync_handler.go`) exposes only
`id`, `tool_name`, `arguments_hash`, `status`, `persistence`, `consumed`, `agent_session_id`
— **no** `tool_pattern`/`params_pattern` and **no** raw `arguments`.

Consequently ExtProc (feature 026 FR-015) cannot perform wildcard matching: from an
`arguments_hash` alone it can only do exact equality, and even a `session` approval is currently
bound to the exact original arguments rather than "any params in the session".

This spec defines the broker/024 changes required to realize the designed glob capability so that
feature 026 FR-015 becomes buildable.

## 2. Goal

Extend feature 024 so that:

- Approvals carry a **glob `tool_pattern` and `params_pattern`** in addition to the exact
  `tool_name`/`arguments`/`arguments_hash` used for creation-time deduplication.
- The Approval UI lets the user choose the **breadth** of a `session`/`permanent` approval and
  previews the pattern that will be stored.
- The `GET /api/approvals` sync channel (and browser read endpoints) expose the patterns so
  ExtProc and the UI can match/display them.
- The pattern grammar, precedence, and canonicalization are defined once, with cross-service
  **test vectors**, so the broker and ExtProc agree on matching without a duplicated, undocumented
  implementation.

## 3. Non-Goals

- Risk-based auto-approval (design §4.6) — out of scope.
- CIBA / Tier 3 — out of scope.
- Changing the creation-time dedup key (stays exact — see FR-B4).
- Cross-replica at-most-once consume / `409 Conflict` — separate concern, not part of this spec.

## 4. Functional Requirements

### FR-B1: Pattern grammar (mirrors design §4.8)

```
tool_pattern  := tool_name [ "(" param_pattern { "," param_pattern } ")" ]
param_pattern := param_name "=" ( literal_value | "*" )
tool_name     := identifier | identifier ".*"   # ".*" = wildcard suffix; "*" = any tool
```

Examples (normative, become test vectors in FR-B7):

| Pattern | Matches |
|---|---|
| `create_pull_request` | any `create_pull_request` call, any params |
| `create_pull_request(repo=acme/app)` | only `repo == "acme/app"`, other params any |
| `create_pull_request(repo=acme/*)` | `repo` glob-matches `acme/*`, other params any |
| `issues.*` | `issues.create`, `issues.update`, … |
| `*` | any tool (still bounded by the OPA permission-set tier) |

The broker MUST persist `tool_pattern` and a structured `params_pattern` (map of
`param_name -> literal_or_glob`). Params absent from `params_pattern` are implicitly `*`.

### FR-B2: Matching semantics & precedence

A concrete invocation `(tool_name, arguments)` matches a stored approval when the tool name
satisfies `tool_pattern` AND every constrained key in `params_pattern` glob-matches the
corresponding argument value (unconstrained keys are `*`). When multiple approvals match, the
**most specific** wins, ranked by (in order): exact tool name > wildcard tool name; then greater
number of constrained (non-`*`) params; then fewer glob metacharacters. Ties MUST be broken
deterministically (e.g. most recently approved). This ranking MUST be identical in the broker and
in ExtProc and is fixed by the FR-B7 test vectors.

### FR-B3: Approval-time breadth selection

`POST /api/approvals/{id}/approve` MUST accept, alongside `persistence`, an optional breadth
selector that yields the stored pattern:

- `this_call` (default for `once`) → `tool_pattern = tool_name`, `params_pattern` = exact
  arguments.
- `any_params` (typical for `session`) → `tool_pattern = tool_name`, `params_pattern = {}`.
- `custom` → an explicit `tool_pattern` (+ optional `params_pattern`) validated against FR-B1.

The broker MUST validate the requested pattern is within the requesting user's authority (it may
only broaden coverage of the same tool/permission-set boundary, never escalate to tools the user
could not approve). Invalid or over-broad patterns MUST be rejected with `422`.

### FR-B4: Creation stays exact (dedup unchanged)

`POST /api/approvals` continues to persist the exact `tool_name` + `arguments` + `arguments_hash`
and continues to dedup pending records by `(principal, agent_id, tool_name, arguments_hash)`
(design §5.1). Pattern fields are populated only at **approve** time (FR-B3); a freshly created
`pending` record has `tool_pattern = tool_name`, `params_pattern` = exact arguments.

### FR-B5: Sync DTO & read endpoints expose patterns

- `GET /api/approvals` (`toolApprovalSummary`): add `tool_pattern` (string) and `params_pattern`
  (object) to each approval summary.
- `GET /api/approvals/{id}`, `GET /api/approvals/pending`, `GET /api/approvals/permanent`: include
  the same pattern fields for UI display.
- The `arguments_hash` field is retained for exact once/dedup semantics and back-compat.

### FR-B6: Approval UI pattern control

The Approval page MUST let a user selecting `session` or `permanent` choose the breadth (FR-B3)
and MUST preview the human-readable pattern (design §4.8), e.g.
"Always allow this agent to call `create_pull_request` on any repo in `acme/*`". `once` retains the
current exact display.

### FR-B7: Canonicalization & cross-service test vectors

This spec MUST ship a language-neutral fixture file of matching test vectors
(`(tool_pattern, params_pattern, concrete tool_name, arguments) -> match: bool` plus precedence
rankings) consumed by **both** the broker unit tests and the ExtProc (feature 026) matcher tests,
so the two implementations are provably consistent. Argument-value canonicalization for comparison
MUST be defined here (scalar rendering, string/number/bool coercion, nested object/array handling)
and pinned by the vectors. ExtProc MUST NOT rely on an undocumented duplicate of the broker's
hashing/matching.

### FR-B8: Backward compatibility

Existing rows (exact, no pattern) MUST be backfilled so that `tool_pattern = tool_name` and
`params_pattern` = exact arguments, preserving current exact behavior. ExtProc's FR-015 matcher
MUST treat a fully-constrained `params_pattern` as exact matching, so pre-extension approvals keep
matching exactly as today.

## 5. Database Requirements

- **DB-B1**: Migration `NNN_add_approval_patterns.{up,down}.sql` adds
  `tool_pattern VARCHAR NOT NULL` and `params_pattern JSONB NOT NULL DEFAULT '{}'` to
  `tool_approvals`, backfilling per FR-B8. The down migration drops both columns.
- **DB-B2**: If pattern-based lookup is needed server-side, add a supporting index
  (e.g. GIN on `params_pattern`); otherwise matching stays client-side in ExtProc and the columns
  are delivery-only.
- **DB-B3**: The pending dedup partial unique index (024 DB-003) is unchanged — it keys on
  `(principal, agent_id, tool_name, arguments_hash)`.

## 6. API Requirements

- **API-B1**: Update `/api/enduser/openapi.yaml`: `ApprovalSyncResponse` approval summary and the
  single/pending/permanent approval schemas gain `tool_pattern` (string) and `params_pattern`
  (object).
- **API-B2**: `POST /api/approvals/{id}/approve` request schema gains the optional breadth selector
  / `tool_pattern` + `params_pattern` (FR-B3).
- **API-B3**: All API changes MUST be confirmed with stakeholders before implementation
  (Constitution IV/X). This is an additive, backward-compatible change (new optional fields).

## 7. Testing

- Unit tests for pattern parsing, matching, and precedence (broker) driven by the FR-B7 vectors.
- PostgreSQL integration tests for the migration (apply + rollback + backfill correctness).
- Handler tests: approve with each breadth; sync/read responses include pattern fields.
- Frontend tests: breadth control + pattern preview.
- The FR-B7 vector fixture is shared with feature 026's ExtProc matcher tests.

## 8. Acceptance Criteria

1. A user approving `create_pull_request` with `permanent` + `custom` pattern `repo=acme/*` results
   in a stored approval whose `GET /api/approvals` summary carries
   `tool_pattern="create_pull_request(repo=acme/*)"` (or equivalent structured form).
2. A subsequent concrete call `create_pull_request(repo=acme/app, title=…)` matches that approval
   per the shared FR-B7 vectors, in both the broker and ExtProc implementations.
3. A `session` + `any_params` approval matches any params for that tool within the session and no
   others.
4. Pre-extension exact approvals continue to match exactly after the migration (FR-B8).
5. The migration applies and rolls back cleanly against real PostgreSQL with no data loss.

## 9. Ownership & Sequencing

- **Owner**: identity broker (feature 024) track.
- **Consumer**: feature 026 (`specs/026-extproc-approval-sync/spec.md` FR-015) — its glob matching
  is inert until FR-B5 ships the pattern fields on the sync channel. Until then, 026 degrades to
  exact `tool_name` + `arguments_hash` matching (a fully-constrained `params_pattern`).
