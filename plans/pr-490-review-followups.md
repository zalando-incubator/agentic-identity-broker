# PR #490 Review Follow-ups — Glob Approval Patterns

## Context

PR [#490](https://github.com/zalando-infosec/agentic-identity-broker/pull/490) (`approval-glob-patterns`)
adds glob-based approval coverage patterns. It must clear an **architectural review**
(comment `5568686853`: 7 findings, 5 recommendations, verdict REQUEST CHANGES) and **18 Copilot
review comments**. This plan resolves both sets.

Branch `026-extproc-approval-sync` (PR #492) is **stacked on top of this branch** and already
contains the enforcement consumer (`internal/extproc/approval/cache.go`), which changes how several
findings must be answered: the ExtProc consumer is real, not speculative — but the *precedence
engine* belongs on that side, not in the shared package. Because 026 is stacked, the symbol removals
in Step 1 are a normal rebase concern rather than a break of an independent branch; the exact edits
026 needs are made on that branch in Step 11.

End state: the pattern grammar lives in the approval bounded context, is validated on write in
exactly one place (Go), has no client-side mirror, and every exported symbol shipped by #490 has a
consumer in #490.

---

## Decisions taken (do not re-litigate)

| # | Decision | Rationale |
|---|---|---|
| D1 | **Do not** add broker-side enforcement here. Patterns are the write-side contract; matching lands in feature 026. | 026 already implements `internal/extproc/approval/{cache,gate,syncer}.go`. |
| D2 | Move `internal/toolpattern` → **`internal/domain/approval/toolpattern`**, and revise **ADR 035 in place** (Status stays `Accepted`). | Resolves finding #3 (domain ring imported a non-ring package). Owner = approval bounded context. ADR 035 is introduced by this branch (`[ADDED]` in the PR file list), so it is edited rather than superseded; the repository owner reviews it before merge. |
| D3 | **Remove the precedence engine** from the shared package; it moves to `internal/extproc/approval` on branch 026. | Evaluation-of-many belongs with the enforcer; validation/derivation is universal. Resolves finding #4. |
| D4 | **Delete the TypeScript matcher.** The SPA calls a new broker endpoint to validate + render patterns. | One source of truth; resolves finding #2 and the `String(1e21)` divergence permanently. |
| D5 | Migration 030 is edited **in place**; no compensating migration. | Tables are empty and 030 has not shipped in a release. See "Migration 030" for the operator note. |

### D3 in detail — who uses what

Verified across both branches (`git grep` on `approval-glob-patterns` and `origin/026-extproc-approval-sync`).

| Symbol | Kind | Broker use today | ExtProc use (026) | Disposition in #490 |
|---|---|---|---|---|
| `Canonical` | canonicalisation | internal only (`:117,177,304`) | — | keep exported (pinned by `canonical` vectors) |
| `EscapeLiteral` | canonicalisation | internal only (`:117`) | — | keep — **gains** callers (Step 2) |
| `ExactParams` | derivation | `service.go:279`, `tool_approval.go:132` | — | keep |
| `ValidateToolPattern` | validation | `service.go:281` | — | keep — **gains** creation caller (Step 3) |
| `ValidateParamsPattern` | validation | `service.go:284` | — | keep — **gains** creation caller (Step 3) |
| `Matches` | single-pattern coverage | `service.go:287` (self-coverage rule) | via `SelectBest` | keep |
| `Format` | rendering | **none** | — | keep — **gains** callers (Step 5) |
| `Match(glob, s)` | primitive | internal only (`:172,177`) | — | **unexport** → `match` |
| `Rank`, `Specificity`, `Compare` | precedence | none | via `SelectBest` | **delete → 026** |
| `Candidate` | precedence input | none | `cache.go:231` | **delete → 026** |
| `SelectBest` | precedence | none | `cache.go:240` | **delete → 026** |
| `MatchVectors()` | contract fixture | `toolpattern_test.go` | `cache_test.go:60` | keep |
| `CanonicalVectors()` | contract fixture | `toolpattern_test.go` | — | keep |
| `PrecedenceVectors()` | contract fixture | none | `cache_test.go:76` | **delete → 026** (with the `precedence` array) |
| `ErrInvalidToolPattern`, `ErrInvalidParamsPattern` | error contract | wrapped with `%v` | — | keep; rewrap with `%w` (Step 4) |

The split is: **broker derives, validates, self-checks coverage and renders; ExtProc selects among
many.** `Matches` is the only evaluation primitive genuinely needed on both sides, and on the broker
it is a *validation rule* (the submitted pattern must cover the reviewed call — anti-retargeting),
not an authorization decision.

---

## Approach

Steps are ordered so the tree builds and `just test` passes after each one.

### Step 1 — Move the package and drop the precedence engine

1. `git mv internal/toolpattern internal/domain/approval/toolpattern`. Package name stays
   `toolpattern`.
2. In `toolpattern.go` delete: `Rank` (`:203-207`), `Specificity` (`:210`), `Compare` (`:236`),
   `Candidate` (`:259-264`), `SelectBest` (`:267`). Rename exported `Match` (`:23`) to unexported
   `match`; update its two internal callers (`:172,177`).
3. In `vectors.go` delete `PrecedenceCandidate`, `PrecedenceVector`, `PrecedenceVectors()`
   (`:27-40,58`) and the `precedence` field of the decode struct (`:36`). In `vectors.json` delete
   the `precedence` array (`:28-33`).
4. In `toolpattern_test.go` delete the precedence-vector test and any `Match(` (exported) call.
5. Rewrite every import of
   `github.com/agentic-identity-broker/agentic-identity-broker/internal/toolpattern` to
   `.../internal/domain/approval/toolpattern`. Current sites:
   `internal/domain/approval/service.go:21`, `internal/domain/storage/tool_approval.go:11`
   (removed in Step 2), `tests/integration/migrations/migrations_test.go:11`,
   `tests/e2e/approval_api_test.go:23` (removed in Step 8).
   Use `lsp rename_file` if the Go server supports it; otherwise edit the five sites directly.

**No compatibility alias.** `internal/toolpattern` must not exist afterwards.

### Step 2 — Escape exact patterns; move exact-coverage derivation into the approval context

Resolves Copilot `r3948441910`, `r3948441863`, `r3948441941`, `r3948442057` and the layering
inversion that D2 would otherwise create (`domain/storage` → `domain/approval/*`).

1. Delete `func (a *ToolApproval) ApplyExactPatterns()` and the `toolpattern` import from
   `internal/domain/storage/tool_approval.go` (`:11`, `:128-133`). `storage.ToolApproval` keeps the
   `ToolPattern` / `ParamsPattern` fields and `ApprovalDecision`; it no longer knows the grammar.
2. Add to `internal/domain/approval/` (new file `exact_patterns.go`):

   ```go
   // ApplyExactPatterns sets a's pattern fields to the exact coverage of its own tool call and
   // validates the result against the pattern grammar.
   func ApplyExactPatterns(a *storage.ToolApproval) error {
       toolPattern := toolpattern.EscapeLiteral(a.ToolName)
       paramsPattern := toolpattern.ExactParams(a.Arguments)
       if err := toolpattern.ValidateToolPattern(toolPattern); err != nil {
           return fmt.Errorf("%w: %w", ErrApprovalInvalidPattern, err)
       }
       if err := toolpattern.ValidateParamsPattern(paramsPattern); err != nil {
           return fmt.Errorf("%w: %w", ErrApprovalInvalidPattern, err)
       }
       a.ToolPattern = toolPattern
       a.ParamsPattern = paramsPattern
       return nil
   }
   ```

3. `internal/domain/approval/service.go:445` becomes:
   ```go
   if err := ApplyExactPatterns(newApproval); err != nil {
       return nil, err
   }
   ```
   placed **before** `s.approvals.Create`. This makes creation reject tool names that violate the
   grammar (charset `[A-Za-z0-9_.:/-]` plus `*`, ≤255 bytes), >64 arguments, argument keys >255
   bytes, and canonical argument values >1024 bytes — instead of persisting a pending approval that
   can never be approved.
4. `internal/adapters/http/handlers/approval/create_handler.go`: map
   `domainapproval.ErrApprovalInvalidPattern` to `422` / `"invalid_pattern"`, matching
   `approve_handler.go:70-72`.
5. Update the 13 remaining `ApplyExactPatterns` call sites from the method form to
   `approval.ApplyExactPatterns(x)` (import alias `domainapproval` where the local variable is named
   `approval`, as `approve_handler_test.go` already does):
   `internal/adapters/http/handlers/approval/approve_handler_test.go:27`;
   `internal/adapters/storage/memory/tool_approval_repository_test.go:143`;
   `internal/adapters/storage/postgres/tool_approval_repository_test.go:430,435`;
   `internal/domain/approval/service_test.go:190`;
   `internal/domain/storage/tool_approval_test.go:373,381` (**move these two cases** into
   `internal/domain/approval/exact_patterns_test.go`; `domain/storage` must not import the grammar);
   `tests/e2e/approval_api_test.go:322`;
   `tests/e2e/frontend/approval_ui_test.go:49,413,440`;
   `tests/e2e/frontend/tool_authorizations_test.go:42`.
   All are single-expression statements; each gains an error check (`Expect(err).NotTo(HaveOccurred())`
   in Ginkgo, `require.NoError(t, err)` in Go tests).

Also escape the omitted-pattern default in `resolveApprovalDecision` — see Step 4.

### Step 3 — Preserve field presence in the approve DTO

Resolves Copilot `r3948441996` and the tri-state leak the review flagged.

`internal/adapters/http/handlers/approval/approve_handler.go:15-19`:

```go
type approveRequest struct {
	Persistence   string          `json:"persistence"`
	ToolPattern   *string         `json:"tool_pattern"`
	ParamsPattern json.RawMessage `json:"params_pattern"`
}
```

Mapping rules in the handler:
- `ToolPattern == nil` → omitted; pass `nil` through.
- `ToolPattern != nil` → pass the value verbatim, **including `""`**, so the service rejects it.
- `ParamsPattern` empty/absent → omitted; pass a nil map.
- `ParamsPattern` is the four bytes `null` → respond `422` / `"invalid_pattern"` with message
  `params_pattern must be an object`.
- otherwise `json.Unmarshal` into `map[string]string`; a decode failure is `400` / `"invalid_request"`.

`internal/domain/approval/service.go:52-56`:
```go
type ApproveRequest struct {
	Persistence   storage.ApprovalPersistence
	ToolPattern   *string
	ParamsPattern map[string]string
}
```
Nil `ToolPattern` = omitted; nil `ParamsPattern` = omitted; non-nil empty map = unconstrained.

### Step 4 — Fix ordering, escaping and observability in `ApproveApproval`

Resolves Copilot `r3948442028`, `r3948441863` and architecture finding #6.

1. Add to `internal/domain/storage/tool_approval.go`, extracting the existing checks at `:109-117`:
   ```go
   // EnsureApprovable reports whether this approval may still be approved by actingPrincipal.
   func (a *ToolApproval) EnsureApprovable(actingPrincipal id.Principal, now time.Time) error {
       if a.Principal != actingPrincipal {
           return ErrApprovalPrincipalMismatch
       }
       if a.Status != ApprovalStatusPending {
           return ErrApprovalNotPending
       }
       if a.IsExpired(now) {
           return ErrApprovalExpired
       }
       return nil
   }
   ```
   `Approve` calls `EnsureApprovable` first instead of inlining the three checks. No behaviour change.

2. Reorder `ApproveApproval` (`service.go:223-270`) to:
   1. `s.approvals.Get` + not-found mapping (unchanged, `:224-231`)
   2. principal check (unchanged, `:232-234`)
   3. `startLifecycleSpan(ctx, "approval.approve", …)` with attributes
      `approval.id`, `approval.principal`, `approval.tool_name` (**restored** — was deleted by this
      PR; `git diff origin/main` confirms it existed), `approval.persistence` (from `req`)
   4. `now := time.Now()`; `approval.EnsureApprovable(actingPrincipal, now)` → map
      `ErrApprovalExpired`→`ErrApprovalGone`, `ErrApprovalNotPending`→`ErrApprovalNotPending`
   5. `resolveApprovalDecision(approval, req)`; on error
      `s.logger.Warn("approval pattern rejected", "approval_id", approvalID, "principal",
      actingPrincipal, "agent_id", approval.AgentID, "tool_name", approval.ToolName, "action",
      "pattern_rejected", "error", err)` then return. Do **not** add `span.RecordError` — no file in
      `internal/domain/` uses it; the convention is span attributes plus a structured log.
   6. `span.SetAttributes(attribute.String("approval.tool_pattern", decision.ToolPattern))`
   7. `approval.Approve(actingPrincipal, decision, now)` and the rest, unchanged.

   Lifecycle errors now precede pattern errors, and every branch is inside the span.

3. `resolveApprovalDecision` (`service.go:272-291`) becomes:
   ```go
   toolPattern := toolpattern.EscapeLiteral(approval.ToolName)
   if req.ToolPattern != nil {
       toolPattern = *req.ToolPattern
   }
   paramsPattern := req.ParamsPattern
   if paramsPattern == nil {
       paramsPattern = toolpattern.ExactParams(approval.Arguments)
   }
   ```
   and both validation wraps change from `%v` to `%w` on the inner error so
   `errors.Is(err, toolpattern.ErrInvalidToolPattern)` works:
   `fmt.Errorf("%w: malformed tool glob: %w", ErrApprovalInvalidPattern, err)`.

### Step 5 — Server-rendered pattern preview and validation endpoint

Replaces the deleted TypeScript matcher (D4) and gives `Format` production callers.

**API (contract first — Constitution IV/X).** Add to `api/enduser/openapi.yaml` beside
`/api/approvals/{id}/approve` (`:1757`) and mirror into
`specs/024-approval-api-ui/contracts/approval-api.yaml`:

```
/api/approvals/{id}/scope-preview:
  post:
    operationId: previewApprovalScope
    summary: Validate and render an approval scope without changing state
```
- Security `PrincipalHeader`; path param `id` (uuid).
- Request body = the **same** schema as `ApproveRequest` minus `persistence`
  (`tool_pattern`, `params_pattern`, identical tri-state semantics).
- `200` → `ScopePreviewResponse`:
  `{ "data": { "tool_pattern": string, "params_pattern": object, "preview": string } }`
- `401`, `403`, `404`, `422` reuse `ApprovalError`, matching `approveApproval`.
  No `410`: preview is read-only and performs no lifecycle check.

Also in the OpenAPI:
- Add `pattern_preview` (string, required) to the approval detail schema returned by
  `getApproval`, `listPendingApprovals`, `listPermanentApprovals`.
- Add `422` / `invalid_pattern` to `createApproval` (Step 2.4).
- Document the escape rule on every `params_pattern` and `tool_pattern` description:
  *"`*` is the only metacharacter. A literal `*` or `\` is written `\*` / `\\`. Values are compared
  against the canonical rendering of the argument."*
- Document the empty-map overload: *"An empty `params_pattern` leaves every argument unconstrained.
  For a tool invoked with no arguments this coincides with exact coverage."*

**Backend.**
- `internal/domain/approval/service.go`: add
  ```go
  type ScopePreview struct {
      ToolPattern   string            `json:"tool_pattern"`
      ParamsPattern map[string]string `json:"params_pattern"`
      Preview       string            `json:"preview"`
  }

  func (s *Service) PreviewApprovalScope(ctx context.Context, approvalID id.ApprovalID,
      actingPrincipal id.Principal, req ApproveRequest) (*ScopePreview, error)
  ```
  Body: `s.approvals.Get` → not-found/forbidden mapping → `resolveApprovalDecision(approval, req)`
  → return `&ScopePreview{decision.ToolPattern, decision.ParamsPattern,
  toolpattern.Format(decision.ToolPattern, decision.ParamsPattern)}`. No span, no mutation, no sync
  bump — it is a read.
- Add `PatternPreview string \`json:"pattern_preview"\`` to `ApprovalDetail` (`service.go:680-681`
  block) and populate it in `enrichApproval` (`:710-711` block) with
  `toolpattern.Format(approval.ToolPattern, approval.ParamsPattern)`.
- New `internal/adapters/http/handlers/approval/scope_preview_handler.go` — `ScopePreviewHandler`
  modelled on `approve_handler.go`, sharing the Step 3 body-decoding helper (extract it into an
  unexported `decodePatternFields(json.RawMessage, *string) (*string, map[string]string, error)` in
  `approve_handler.go` and call it from both).
- Register in `internal/app/builder.go` (field `ApprovalScopePreview`, Principle XII) and in
  `internal/adapters/http/routing/enduser.go:97-102`:
  ```go
  r.With(requirePrincipal, browserMutationProtection.Handler).Post("/scope-preview", h.ApprovalScopePreview.ServeHTTP)
  ```
  Add the route to the comment block at `:60-66`.

### Step 6 — Delete the TypeScript grammar mirror

1. Delete `web/src/utils/toolPattern.ts` and `web/src/utils/toolPattern.test.ts`.
2. Move `humanizeParameterKey` (`toolPattern.ts:90`) and its test block
   (`toolPattern.test.ts:72-82`) to new `web/src/utils/humanize.ts` / `humanize.test.ts`. It is a
   label formatter with no grammar knowledge. Everything else goes:
   `formatToolPattern`, `escapeGlobLiteral`, `unescapeGlobLiteral`, `matchesGlob`,
   `canonicalArgumentValue`, `validateToolGlob`, `validateParamGlob`, `canonicalJson`.
3. `web/src/services/api/approvals.ts`: add
   ```ts
   async previewApprovalScope(approvalId: string, request: ScopePreviewRequest): Promise<ScopePreview>
   ```
   posting to `/approvals/${encodeURIComponent(approvalId)}/scope-preview`, unwrapping
   `response.data.data`.
4. `web/src/types/approval.ts`: add `pattern_preview: string;` to `ToolApprovalDetail` (`:19-30`);
   add `ScopePreviewRequest` (`tool_pattern?: string; params_pattern?: Record<string,string>`) and
   `ScopePreview` (`tool_pattern: string; params_pattern: Record<string,string>; preview: string`).
5. `web/src/components/approvals/ApprovalScopeEditor.tsx`:
   - Remove all imports from `@utils/toolPattern` except `humanizeParameterKey` (now `@utils/humanize`).
   - **Exact mode values come from the server, not from recomputation.** Replace
     `exactParamPattern` (`:75`, `escapeGlobLiteral(canonicalArgumentValue(...))`) with a lookup of
     `approval.params_pattern[key]`, and the exact tool value with `approval.tool_pattern`. Both are
     already exact coverage as created (Step 2).
   - Replace the local validation at `:93-94` and `:100-102` and the preview at `:399` with state
     driven by a **250 ms-debounced** `previewApprovalScope` call keyed on
     `(toolPattern, constraints)`. Track a monotonically increasing request id and ignore stale
     responses. Render: `preview` on success; the `422` response `message` as the inline error;
     on network failure show the existing error affordance and leave the previous preview visible.
   - Replace the literal-display expressions at `:281` and `:389`
     (`unescapeGlobLiteral(exactParamPattern(...))`) with the raw
     `approval.arguments?.[key]` rendered via `JSON.stringify` when it is not a string — display
     only, no grammar.
   - **a11y (Copilot `r3948442324`):** tool custom input (`:259-267`) gets
     ``label={`${toolLabel} custom match`}``; each parameter custom input (`:347-359`) gets
     ``label={`${label} custom match`}``, matching the existing per-parameter select convention at
     `:321-331`.
6. `web/src/components/approvals/ApprovalRequestSummary.tsx:4,104`: drop the `formatToolPattern`
   import and render `approval.pattern_preview`.
7. `web/src/components/approvals/ApprovalScopeEditor.test.tsx`: mock `approvalApi.previewApprovalScope`;
   add a case asserting the two custom inputs have **distinct** accessible names via
   `getByRole('textbox', { name: /repo custom match/i })`.
8. `tests/e2e/pages/approval_page.go`: update the `Custom match` selectors (`:295` region) to the new
   accessible names; `GetPatternPreview` (`:319`) now waits for the server-rendered preview.

### Step 7 — Migration 030

Tables are empty and 030 has not shipped in a release, so it is edited in place (D5).

Rewrite `migrations/030_add_approval_patterns.up.sql`:
- Header comment: state that the backfill sets exact coverage for pre-existing rows and that the
  row-level `trigger_tool_approvals_sync` fires per updated row, which is intentional — it is the
  single mechanism that bumps the sync version (ADR 014). Remove the stale claim currently at
  `:5-6`. (Resolves Copilot `r3948442127`.)
- **Delete** `ALTER TABLE tool_approvals DISABLE TRIGGER trigger_tool_approvals_sync` (`:49`),
  `ENABLE TRIGGER` (`:58`) and the hand-copied
  `UPDATE approval_sync_state …` / `SELECT pg_notify('approval_sync','')` (`:60-61`). This restores
  the ADR 014 invariant that only `notify_approval_sync()` bumps the version, removes the repo's
  only `DISABLE TRIGGER` (which needs table-owner privileges and an `ACCESS EXCLUSIVE` lock), and
  makes a down→up cycle stop double-bumping. It reverses the response to the earlier Copilot comment
  `r3934806301`; record the reason in the header comment so the tradeoff is explicit.
- Line 52: `SET tool_pattern = aib_glob_escape(tool_name)` — the backfill promised exact coverage
  but stored the raw name. (Resolves Copilot `r3948441941`.)
- Line 67: also drop the `params_pattern` default —
  `ALTER TABLE tool_approvals ALTER COLUMN params_pattern DROP DEFAULT;` — so neither column can be
  silently filled. An implicit `'{}'` means *unconstrained*, which is scope-widening.
- Keep the `aib_canonical_json` / `aib_canonical_value` / `aib_glob_escape` helpers and their
  `DROP FUNCTION`s (`:11-47`, `:63-65`): they exist only for the duration of the migration and are
  pinned by the integration test below.

`tests/integration/migrations/migrations_test.go:232-293`: keep the existing assertions; extend the
fixture set with a tool name containing `*` and one containing a trailing `\`, asserting the
backfilled `tool_pattern` equals `toolpattern.EscapeLiteral(tool_name)` and that
`toolpattern.ValidateToolPattern` accepts it. Add an assertion that `approval_sync_state.version`
increased after 030 ran.

**Operator note for the plan's executor:** any pre-release environment that already applied 030 will
not re-run it. Recreate that database, or manually apply the two `DROP DEFAULT`s and the escaped
backfill. CI and the integration suite start from empty containers and are unaffected.

### Step 8 — Storage adapter parity and port contract

- `internal/adapters/storage/memory/tool_approval_repository.go`: change `copyParamsPattern`
  (`:271-275`) to return `map[string]string{}` for a nil input, so `Approve` (`:80-97`) matches
  `Create` (`:39-45`) and Postgres (`marshalParamsPattern` `:1093-1098`,
  `unmarshalParamsPattern` `:1100-1108`). (Resolves Copilot `r3934806255`.)
- `internal/ports/storage.go` (`ToolApprovalRepository`, `:277-299`): document on the interface that
  implementations must return a non-nil `ParamsPattern`. Keep the defensive normalisation in
  `sync_handler.go:62-64` — it is the JSON-contract boundary.

### Step 9 — Specification traceability and E2E conventions

**`specs/024-approval-api-ui/spec.md`:**
- Add **User Story 7 — Scope an approval decision** after US6 (`:122-136`), with these acceptance
  scenarios (ids `US7-S1` … `US7-S5`), written in the existing Given/When/Then style of `:18-20`:
  - `US7-S1` exact coverage stored when the approve request omits both pattern fields.
  - `US7-S2` an edited `params_pattern` (`repo: acme/*`) is persisted and returned by sync.
  - `US7-S3` an explicit empty `params_pattern` stores unconstrained coverage.
  - `US7-S4` a pattern that does not cover the reviewed call is rejected with `422 invalid_pattern`.
  - `US7-S5` the scope editor shows the server-rendered preview and a server validation message for
    a malformed pattern.
- Fix the scenario count: `tasks.md:292,337` and `plan.md:399` say "all 27 spec scenarios"; the file
  contains 28 before this change. Update all three to the post-change total.
- Reference `US7-*` from FR-014/FR-015 (`:225-226`) and FR-022…FR-027 (`:233-238`).

**`tests/e2e/approval_api_test.go`** — replace the block at `:760-787`, which currently proves only
store-and-echo and asserts coverage by calling `toolpattern.Matches` in the test process
(finding #5). Remove the `internal/toolpattern` import at `:23`. New shape, per
`tests/e2e/AGENTS.md:64-88`:

```go
Describe("when a user scopes an approval decision", func() {
    BeforeEach(func() { /* create pending approval + seed, ≤15 lines */ })

    // US7-S1 from specs/024-approval-api-ui/spec.md
    It("should store exact coverage when the approve request omits pattern fields", …)
    // US7-S2 …
    It("should persist an edited parameter glob and expose it through sync", …)
    // US7-S3 …
    It("should store unconstrained coverage for an explicit empty params pattern", …)
    // US7-S4 …
    It("should reject a pattern that does not cover the reviewed tool call with 422", …)
})
```
Every assertion crosses the wire: read the stored patterns back from `GET /api/approvals` (sync) and
assert the literal strings; assert `422` + body `error == "invalid_pattern"` for the rejection.
This closes Copilot `r3948442218` and completes task T148 (edited / unconstrained / exact /
rejected), Copilot `r3948442097`.

**`tests/e2e/frontend/approval_ui_test.go`** — wrap `:409` and `:436` in
`Describe("when the approval scope editor is open", …)` with the 10 duplicated setup lines
(`:410-419` ≡ `:437-446`) hoisted into its `BeforeEach`; rename to
`It("should persist an edited permanent approval pattern", …)` and
`It("should store unconstrained coverage when a parameter is set to any value", …)`; add `// US7-S2`
and `// US7-S3` comments. Add a third `It` for `US7-S5` asserting the server-rendered preview text
and the inline validation message. (Copilot `r3948442252`, `r3948442281`.)

**`specs/024-approval-api-ui/tasks.md:465-481`** — tick T145–T154 once their acceptance holds and
append T155–T160 for this round: package move + precedence removal; ADR 035 revision; exact-pattern
escaping and creation validation; presence-preserving DTO; scope-preview endpoint and TS mirror
removal; spec scenarios US7-* and E2E renaming; the paired branch-026 changes of Step 11.

**`specs/024-approval-api-ui/glob-approval-matching.md:5`** — change
`**Status**: Proposed — requires API sign-off (Constitution IV/X)` to
`**Status**: Accepted — API contract reviewed in PR #490 (api/enduser/openapi.yaml)`.
This artifact is introduced by this branch, so the PR review *is* the sign-off gate that Copilot
`r3948442159` asked for; record the approving reviewer in the header at approval time.

### Step 10 — ADR 035, AGENTS and ARCHITECTURE

**`adrs/035-shared-tool-pattern-matching.md`** — this ADR is **introduced by this branch**
(`[ADDED] (+42 -0)` in the PR file list), so it is revised in place and keeps
`**Status**: Accepted` — the repository owner reviews it before merge, which is the sign-off that
the ADR index's "same-PR ADR is only a proposal" guidance exists to require. No superseding ADR.
Rewrite Context, Decision, Rationale and Implementation Notes so the record matches what ships:
- **Decision**: the package is `internal/domain/approval/toolpattern` — a standard-library-only leaf
  owned by the approval bounded context, importable by `internal/domain/*` and, by one explicit
  exception, by `internal/extproc/*`. It must not import `internal/ports`, `internal/adapters` or
  `internal/extproc`.
- **Why not top-level `internal/`**: that placement sat outside every hexagonal ring, and
  `internal/domain/AGENTS.md:5-7,38-39,47` permits only the standard library, `internal/ports/`,
  other `internal/domain/` packages and declared domain libraries. Two domain files imported it
  without the allowlist ever being amended.
- **Scope**: the shared package owns grammar, canonicalisation, validation, single-pattern coverage
  (`Matches`) and rendering (`Format`), plus the `matches`/`canonical` vectors. Precedence
  (`Rank`/`Specificity`/`Compare`/`Candidate`/`SelectBest`) is enforcement-side and lives with the
  matcher in `internal/extproc/approval` together with its `precedence` vectors — it has exactly one
  consumer and no broker caller.
- **Consumer**: name `internal/extproc/approval/cache.go` on the stacked branch
  `026-extproc-approval-sync` (Step 11). Do not claim ExtProc consumes it on this branch.
- **Relation to ADR 011/027**: narrowed, not reversed — exactly one importable path, standard
  library only, no broker I/O, and no other `internal/domain` package may be imported from
  `internal/extproc`.
- Drop the Implementation Notes sentence listing `Specificity`, `Compare` and `SelectBest` as
  exposed by this package; they no longer live here.

**`AGENTS.md`** — update the existing 035 row in the ADR Decision Index (`:147`) to
`| 035 | \`adrs/035-shared-tool-pattern-matching.md\` | Approval-pattern package owned by the approval domain |`.
No new row.

**`internal/extproc/AGENTS.md:118`** — repoint the exception to
`internal/domain/approval/toolpattern` and state that no other `internal/domain/` package may be
imported.

**`internal/domain/AGENTS.md`** — no rule change needed; the package is now inside the ring. Add one
line to the import guidance (`:47` area) noting that `domain/storage` must not import
`domain/approval/*` (the grammar dependency was removed in Step 2).

**`ARCHITECTURE.md`** — beside the `ToolPattern`/`ParamsPattern` glossary entries (`:1498-1500`) add:
- **Authority:** `arguments_hash` is the identity used for pending-approval de-duplication
  (`ON CONFLICT (principal, agent_id, tool_name, arguments_hash)`);
  `tool_pattern`/`params_pattern` are the coverage grammar consumed by the ExtProc matcher in
  feature 026. They answer different questions and are both authoritative in their own domain.
- **Canonicalisation:** `storage.ComputeArgumentsHash` (`json.Marshal`, HTML-escaping) produces a
  de-duplication *identity*; `toolpattern.Canonical` produces a *coverage rendering*. They are
  deliberately different functions and must not be unified.
- The empty-`params_pattern` overload, as worded in Step 5.

### Step 11 — Apply the paired changes on branch `026-extproc-approval-sync`

Step 1 deletes symbols the stacked branch uses, so the receiving edits are **made on that branch as
part of this work**, not left as a note. Use a worktree rather than switching branches in place:
`git worktree add .worktrees/026-extproc-approval-sync 026-extproc-approval-sync`.

1. **New `internal/extproc/approval/precedence.go`** (`package approval`). Move the deleted code
   verbatim from the old `internal/toolpattern/toolpattern.go:202-283` and unexport it, because
   `cache.go` sits in the same package: `rank` (was `Rank`; fields `exactTool`, `constrainedParams`,
   `wildcards`), `specificity` (was `Specificity`, `:210-219`), `countWildcards` (`:221-233`,
   already unexported and used only by `specificity`), `compareRank` (was `Compare`, `:236-256`),
   `candidate` (was `Candidate`, `:259-264`) and `selectBest` (was `SelectBest`, `:267-283`).
   `selectBest` keeps calling the shared `toolpattern.Matches`, so import
   `internal/domain/approval/toolpattern`. Preserve the tie-break exactly: rank comparison, then
   later `DecidedAt`, then lower `ID`.
2. **New `internal/extproc/approval/precedence_vectors.json`** — the four objects currently at
   `internal/toolpattern/vectors.json:29-32`, as a top-level JSON array (drop the enclosing
   `{"precedence": …}` wrapper). Embed it from a new `vectors.go` in the same package, mirroring
   `internal/toolpattern/vectors.go:42-58`: unexported `precedenceCandidate` / `precedenceVector`
   types with the same JSON tags, a `sync.OnceValue` parser, and
   `func precedenceVectors() []precedenceVector`.
3. **`internal/extproc/approval/cache.go`** — change the import at `:9` to
   `internal/domain/approval/toolpattern`; at `:224-240` build `[]candidate` and call
   `selectBest(candidates, tool, arguments)`. No behavioural change.
4. **`internal/extproc/approval/cache_test.go`** — import path at `:12`; `:60` keeps
   `toolpattern.MatchVectors()`; `:76` switches from `toolpattern.PrecedenceVectors()` to the local
   `precedenceVectors()`, with `Candidates`/`ToolPattern`/`ParamsPattern`/`DecidedAt`/`WinnerID`
   renamed to the unexported fields from step 2.
5. `ApplyExactPatterns` needs no extra work here: `git grep` shows 026 has the same 15 call sites as
   this branch and adds none, so Step 2 covers them once rebased.

**Acceptance:** rebase 026 onto this branch and run
`GOEXPERIMENT=jsonv2 go build ./... && go test ./internal/extproc/...` inside the worktree.
`TestCacheMatchesSharedVectors` and `TestCacheUsesSharedPrecedenceVectors` must both pass with
unchanged behaviour, and `grep -rn "internal/toolpattern" .` must return nothing.

---

## Critical files & anchors

| File | Region | Why |
|---|---|---|
| `internal/domain/approval/service.go` | `:223-291` `ApproveApproval` / `resolveApprovalDecision`; `:445` creation; `:680-711` `ApprovalDetail` | Ordering, escaping, observability, preview field — Steps 2, 4, 5 |
| `internal/domain/storage/tool_approval.go` | `:11`, `:109-117`, `:128-133` | Grammar import removed, `EnsureApprovable` extracted — Steps 2, 4 |
| `internal/adapters/http/handlers/approval/approve_handler.go` | `:15-19`, `:61-72` | Presence-preserving DTO + shared decoder — Steps 3, 5 |
| `migrations/030_add_approval_patterns.up.sql` | `:1-6`, `:49-61`, `:52`, `:67` | Comment, trigger dance, escaping, defaults — Step 7 |
| `web/src/components/approvals/ApprovalScopeEditor.tsx` | `:75`, `:93-102`, `:259-267`, `:281`, `:347-359`, `:389`, `:399` | Server-driven validation/preview + a11y labels — Step 6 |

## Verification

Run from the repository root.

1. **Build + static checks:** `just check && just build-all`.
2. **Package boundary is real:**
   `! grep -rn "internal/toolpattern" --include='*.go' .` returns nothing, and
   `! grep -rn "toolpattern" internal/domain/storage/` returns nothing.
3. **No TS grammar remains:**
   `! test -f web/src/utils/toolPattern.ts` and
   `grep -rn "matchesGlob\|canonicalArgumentValue\|validateToolGlob\|validateParamGlob\|formatToolPattern\|escapeGlobLiteral" web/src` returns nothing.
4. **Go unit + adapter tests:** `just test`.
5. **New-behaviour proof — creation validation (Step 2).**
   `go test ./internal/domain/approval/ -run TestApplyExactPatterns` with a case
   `ToolName: "read*file"` → stored `tool_pattern` is `read\*file`, and
   `ToolName: "create pull"` → error wrapping `ErrApprovalInvalidPattern`.
6. **New-behaviour proof — lifecycle precedes pattern (Step 4).**
   `go test ./internal/domain/approval/` with a case: an **expired** pending approval approved with
   a malformed `tool_pattern` returns `ErrApprovalGone`, not `ErrApprovalInvalidPattern`.
7. **New-behaviour proof — tri-state (Step 3).**
   `go test ./internal/adapters/http/handlers/approval/`:
   body `{"persistence":"permanent"}` → exact coverage stored;
   `{"persistence":"permanent","tool_pattern":""}` → `422 invalid_pattern`;
   `{"persistence":"permanent","params_pattern":null}` → `422 invalid_pattern`;
   `{"persistence":"permanent","params_pattern":{}}` → unconstrained stored.
8. **New-behaviour proof — scope preview over the wire (Step 5).**
   Start the broker (`just run` or the E2E harness) and, with a pending approval id `$ID` for tool
   `create_pull_request` with `{"repo":"acme/app","title":"Fix bug"}`:
   ```
   curl -s -X POST localhost:8000/api/approvals/$ID/scope-preview \
     -H 'X-Remote-User: <principal>' -H 'Content-Type: application/json' \
     -d '{"params_pattern":{"repo":"acme/*"}}'
   ```
   → `200` with `"preview": "create_pull_request(repo=acme/*)"`.
   With `-d '{"tool_pattern":"delete_*"}'` → `422` and body `"error":"invalid_pattern"`.
9. **Migration (Step 7):** `just test-integration-infra` (or
   `go test -tags=integration ./tests/integration/migrations/ -run TestMigration030ApprovalPatterns`)
   — requires Docker/Podman.
10. **Frontend unit:** `just web-test`.
11. **Browser acceptance (Steps 5, 6, 9):** `just test-e2e-frontend`. Then drive the review page in a
    browser: select **Always allow**, expand **Approval scope**, set *Repo* to *Custom match* and
    type `acme/*` → the preview text updates from the server and no client-side validation fires;
    type `acme\` → the server's `422` message appears inline. Confirm the two custom inputs expose
    distinct accessible names in the accessibility tree.
12. **Full gate:** `GOEXPERIMENT=jsonv2 just verify`.

## Assumptions & contingencies

- **`tool_approvals` is empty in every environment, and migration 030's schema is already applied
  in the pre-release environment.** Both were stated by the repository owner during planning; they
  are the reason Step 7 edits 030 in place and does not add an expand/contract compatibility path
  for the `DROP DEFAULT`s. If a deployment turns out to hold rows, the exact-coverage backfill still
  runs and is correct; only the removed `DISABLE TRIGGER` optimisation is lost, costing one
  sync-version bump per row. If old replicas can still serve after the pre-upgrade migration Job in
  a real rollout, do **not** drop the defaults in 030: keep them and drop them in a follow-up
  migration one release later.
- **Migration 030 has not shipped in a release.** If it turns out to have shipped, do **not** edit it
  in place: add `migrations/031_fix_approval_pattern_defaults.{up,down}.sql` performing the two
  `DROP DEFAULT`s and re-running the escaped backfill with `WHERE tool_pattern <> aib_glob_escape(tool_name)`.
- **`lsp rename_file` may not move a Go package directory.** If it fails, do the `git mv` and rewrite
  the five import sites listed in Step 1 by hand, then `just check`.
- **Debounced preview may be judged too chatty in review.** If so, keep the debounce but only call
  the endpoint on input blur and on persistence change; the server remains the sole validator either
  way.
