# Phase 0 Research: End-User Activity & Auditing Experience

**Feature**: 038-activity-audit-experience
**Date**: 2026-09-04 · **Revised**: 2026-09-22 (plan review — code re-verified, Decisions 11–13 added)
**Input**: [spec.md](./spec.md)

This document resolves every open question needed to design the feature. The dominant
concern is comprehension-first UX in the existing consent SPA, backed by a durable,
per-user activity record that the current codebase does not yet have.

---

## Codebase grounding (what already exists)

### Frontend SPA (`web/`)

- **Router** (re-verified 2026-09-22; the SPA is root-mounted per ADR 035-root-mounted-spa):
  explicit `react-router` `BrowserRouter`, **`basename="/"`**, lazy pages under `Suspense` in
  `web/src/App.tsx:52-73`. Routes: `/` → redirect to `/delegations`; `/delegations` →
  `ConsentOverviewPage`; `/agents/:agentId` → `AgentGrantDetailPage`; `/sessions` →
  `ThirdPartySessionsPage`; `/approvals/:id` → `ApprovalPage`; `/approvals` →
  `ToolAuthorizationsPage`; `*` → `ErrorPage`. (An earlier draft of this document cited
  `/consent`, `/agent/:agentId`, `/oauth2/sessions` — those are stale.)
- **Primary navigation**: `web/src/components/layout/AppLayout.tsx` owns `navLinks` for
  `/delegations`, `/approvals`, and `/sessions`. Its link container is `hidden md:flex`, and the
  wrapper passes no sidebar to the design-system layout. At small widths, no primary links are
  visible. Use those same links in a visible wrapping mobile row; adding `/activity` to the
  desktop list alone cannot meet US1 #6. The current exact pathname match must also allow
  `/activity/:eventId` to highlight Activity. `components/layout/Header.tsx` is an orphan and
  is not part of the fix. Use the design-system `EmptyState`, not the duplicate UI component.
- **Data hook shape** (canonical): `web/src/hooks/useSessions.ts:55-120` — `useState {data,
  loading, error}`, `fetchData` `useCallback` (sets loading/error, calls typed service), a
  `refetch` callback, `useEffect` on mount. Parallel loads use `Promise.all`
  (`useAgentGrants.ts:52-105`). All hooks expose manual `refetch`; **no polling anywhere**.
- **API client**: `web/src/services/api/client.ts` — axios `baseURL '/api'`, 30s timeout,
  response interceptor mapping 401/403/404/409/410/5xx/network → `ApiError`. **No auth header
  in the client**; the reverse proxy injects `X-Remote-User` (Vite dev proxy injects
  `dev@example.com`). Typed service methods: `apiClient.get<T>('/path')`
  (`services/api/consent.ts`, `sessions.ts`, `approvals.ts`). GET responses are manually
  cached via `services/api/cache.ts` (Map + TTL), invalidated on mutation.
- **Types**: hand-written interfaces in `web/src/types/*.ts`, **synced manually** with
  `api/enduser/openapi.yaml` (no generated client).
- **Filters/tabs/pagination in the app**: **none today**. No `useSearchParams`, no filter/URL
  state, no tabs usage, no pagination usage — although the design system ships `Tabs` and
  `Pagination` primitives that the app has never wired.

### Design system (`web/src/design-system/`)

| UX need (spec) | Existing primitive | Verdict |
|---|---|---|
| Tabs | `components/navigation/Tabs/Tabs.tsx` (`variant underline\|pill\|button`) | Reuse |
| Accordion / progressive disclosure | `components/advanced/Accordion/Accordion.tsx` (`exclusive`, `items`, `badge`) | Reuse |
| Empty state (nothing-yet + no-matches) | `components/feedback/EmptyState/EmptyState.tsx` (`primaryAction` = "Clear filters") | Reuse |
| Outcome status (not color-alone) | `components/data-display/StatusIndicator/StatusIndicator.tsx` (label **and** icon rendered, `role=img`) + `primitives/Badge` | Reuse; app supplies outcome→variant+icon+label mapping |
| Card / surface | `components/data-display/Card/Card.tsx` (`as="article"`, `header`, `divider`) | Reuse |
| Filter select / dropdown | `components/inputs/Select/Select.tsx`, `components/overlays/Dropdown/Dropdown.tsx` | Reuse |
| Relative time-window preset | *(none: no segmented-control/toggle-group)* | Compose from `Tabs` (`pill`) — presets only, no custom range needed |
| Loading skeleton | `components/feedback/Skeleton/Skeleton.tsx` (`role=status`) | Reuse |
| Incremental load | `components/navigation/Pagination/Pagination.tsx` + `Button` ("Load more") | Reuse `Button` for cursor "Load more" |
| Needs-attention callout | `components/feedback/Alert/Alert.tsx` (`variant warning\|error`, `action`, `role`/`aria-live`) | Reuse |
| Route navigation | *(no router-aware Link primitive)* | Use `react-router` `Link` / `Button` + `navigate` |
| **Timeline / narrative sequence** | **NONE** (only a docs composition pattern in `COMPONENT_PAIRING_GUIDE.md:524-583` and a `Divider` vertical story) | **New universal primitive required** (see Decision 3) |

- Tokens: Tailwind v4 `@theme` semantic colors (`trust`, `success/error/warning/info`,
  `neutral`, text/bg/border) — extended palettes (`navy-*`, `gray-*`) forbidden
  (`docs/TOKEN_GUIDE.md`). Storybook: `web/.storybook/main.ts`, CSF3 with `tags:['autodocs']`,
  `addon-a11y` enabled (`Card.stories.tsx` is the reference).
- New-primitive gate (`docs/DESIGN_PRINCIPLES.md:353-376`): allowed only when the component has
  a distinct purpose, is reused across multiple places, needs specific interaction/semantics,
  and does not fit an existing primitive.

### Backend (`internal/`) and enduser API

- **Enduser OpenAPI** `api/enduser/openapi.yaml`: `{data: ...}` success envelope, `{error,
  message}` errors, RFC3339 timestamps, Zalando resource-oriented conventions, `X-Remote-User`
  principal. **No pagination pattern exists** (all list endpoints return bare arrays; approval
  sync uses ETag/long-poll).
- **Route registration**: `internal/adapters/http/routing/enduser.go` `SetupEnduserRoutes`
  (authenticated subrouter behind `RequirePrincipal`). Principal extracted in
  `internal/adapters/http/middleware/principal_middleware.go`.
- **Handler pattern**: `internal/adapters/http/handlers/consent/agents_handler.go:38-80`
  (`principal.FromContext`, service call, `{data}` / `ErrorResponse`).
- **Domain entities** in `internal/domain/storage/`: `UserSession` (encrypted tokens,
  `IsExpired` from refresh expiry), `UserGrant` (`valid_until`, `IsActive`), `Agent`,
  `ToolApproval` (`pending/approved/denied`, `Approve/Deny/Consume`, lazy expiry).
- **Storage**: ISP repos in `internal/ports/storage.go`; memory + postgres adapters
  (`internal/adapters/storage/{memory,postgres}`), sqlx, errors wrapped in `domain.StorageError`.
  Migration numbers `030` and `031` are allocated. Reserve `032` for activity events.
- **Typed IDs**: generated by `internal/domain/id/gen_ids.go` (`go:generate`) → add a
  `{"ActivityEventID", "activity event"}` row and regenerate `uuid_ids_gen.go` (ADR 013).
- **Config schema**: the `Config` struct with `mapstructure` tags lives in
  `internal/ports/config.go` (e.g. `StorageConfig` at `:271`); features add a section here and
  read it through the unified config port (Principle VII).
- **DI**: `internal/app/builder.go` `Build()` constructs services; enduser handlers assembled
  into `app.EnduserHandlers`. Adding a feature = new field in `app/handlers.go` + builder
  construction + registration in `enduser.go` (Principle XII).
- **No persisted audit/activity/event store or domain event bus exists.** Only structured
  slog/OTel logs. Event-like seams already present but not persisted — **re-verified
  2026-09-22, with corrections**:
  - oauth2 `TokenIssued` / `TokenRequestFailed` are slog lines in
    `internal/adapters/http/enduser/token_grant_strategy.go` for local client_credentials /
    authorization_code / refresh_token grants. The public `/oauth2/token` route has no
    authenticated user principal; these failures remain operational audit events rather than
    per-user activity. **The RFC 8693 exchange** (`TokenExchangeService.Exchange`,
    `internal/domain/tokenexchange/service.go:170`) does not emit `TokenIssued`;
    the activity recorder must hook `Exchange` directly.
  - Token-exchange failures are `access_denied` / `invalid_grant` **with free-text detail only**;
    there is no structured reason. Revocation **deletes** the grant
    (`internal/domain/consent/service.go` ~`:675`), so "revoked" and "never granted" are
    indistinguishable at exchange time. → Phase 0 adds a typed `DenialReason`.
  - Impersonation `AuditRecord` exists and is logged synchronously (credential-free).
  - `ToolApproval` create/approve/deny/consume/revoke are synchronous service methods. **Create
    is idempotent** and returns the existing pending approval with `IsNew:false`
    (`internal/domain/approval/service.go:360-378`). **Expiry is lazy** — detected only on read
    (`:190-197`) or when a mutation is refused. No job marks approvals expired.
  - `UserSession` expiry is **also lazy**: `GetValidAccessToken` returns `ErrSessionExpired`
    only when an exchange reads a session with no valid access token and no usable refresh
    token (`internal/domain/oauth2session/service.go` ~`:1155`). There is no `session.expired`
    detection otherwise; it can fire late, never, or repeatedly.
  - **No scheduler, cron, leader election, or distributed lock exists** in the app. The only
    tickers are cache eviction goroutines (permission-set cache, extproc cache) and the SPA's
    `apiCache.cleanup`. The approval sync broadcaster is in-process only.
  - **Frequency of exchanges**: extproc caches exchanged tokens per instance keyed by
    `{subjectToken, resourceURI}` with singleflight, a 5 s negative cache, and stale-on-error
    (`internal/extproc/server/exchanger.go`). The broker therefore sees one exchange per
    (subject, resource) per TTL **per extproc replica**, not per MCP request — the count scales
    with replicas and evictions, and the same logical action reaches the broker repeatedly.
  - **Correlation**: OTel is wired (`otelchi`, spans in the token handler and approval
    lifecycle); `trace_id` is available in domain services from the span context;
    `X-Request-ID` is available only on the oauth2 routes; `ToolApproval` stores `traceparent`.
  - `principal` is `id.Principal` (string) taken from a **configurable** header
    (`PrincipalHeaderName`, default `X-Remote-User`) — not necessarily an email; there is no
    tenant concept.

### E2E / Playwright (`tests/e2e/`)

- **Backend E2E**: Ginkgo/Gomega, dual-server bootstrap (`tests/e2e/bootstrap/`), fixtures,
  matchers, helpers; enduser calls via `enduserServer.AuthenticatedGET('/api/me', principal)`
  (`bootstrap/test_server.go:586-620` injects `X-Remote-User`). One `It()` per spec scenario
  with a `// Scenario X.Y` comment.
- **Frontend E2E**: **Go Ginkgo + `playwright-go`** (not a Node runner).
  `tests/e2e/frontend/frontend_suite_test.go` launches Chromium in `BeforeSuite`; `BeforeEach`
  builds fresh storage + mock upstream + app + enduser server + browser context/page with
  `X-Remote-User` via `ExtraHTTPHeaders`. Page objects in `tests/e2e/pages/` (base
  `page.go:63-95`, `TakeScreenshot` `:268-315` gated by `E2E_CAPTURE_SCREENSHOTS`, full-page
  PNG with animations disabled, saved to `coverage/screenshots` then synced to
  `tests/e2e/screenshots/`). Seeding is via direct `GetTestStorage()` repositories.
- **Accessibility testing today: none** (no axe / keyboard / zoom / reduced-motion checks in
  `tests/e2e`). `web/package.json` has Storybook `addon-a11y`; `axe-core` is a transitive dep;
  `jsx-a11y` lint runs on the SPA. → Accessibility E2E must be **added** within the existing
  `playwright-go` harness.

---

## Decisions

### Decision 1 — Location: a dedicated first-class `/activity` route

**Decision**: Add a single top-level destination `Activity` at `/activity`, a sibling to Agent Delegations
(`/delegations`), Tool Authorizations (`/approvals`), and Third-Party Sessions (`/sessions`).
Event drill-down is a deep-linkable child route `/activity/:eventId` rendered as a detail panel
**inside** the activity shell (the surrounding feed/thread stays in view). Needs-attention "next
steps" and back-links link **out** to the existing `/sessions`, `/approvals`, `/approvals/:id`,
and `/agents/:agentId` flows, generated only from a server-side route allowlist (data-model
§3.3). The agent and sessions pages gain a one-line "View activity" inbound link.

**Rationale**: FR-016 and SC-010 require a first-class, single-action destination that is not
nested inside agent/consent/session views. Add the lazy route and a shared `navLinks` entry,
then render primary links directly on both desktop and mobile. The current `hidden md:flex`
container leaves small and zoomed viewports without any link. A menu drawer needs an extra action
and does not meet the single-action acceptance scenario. A child detail route keeps US4 within
the activity shell and allows deep links.

**Alternatives considered**:
- *Embed activity inside existing session/agent pages* — rejected: violates FR-016 (would make
  it subordinate) and fragments the "at a glance" overview (US1).
- *Modal/drawer-only detail with no route* — rejected: not deep-linkable; harder to test and to
  return to; loses browser back semantics.
- *Both a global route and embedded per-entity strips* — deferred: per-entity filtered views
  are reachable with `/activity?agent_id=…` or `?service_id=…` and are linked from the existing
  agent/session pages. A separate embedded surface is extra scope for the MVP.

### Decision 2 — Backend: durable curated per-principal store on PostgreSQL

**Decision**: Keep the new `internal/domain/activity` bounded context and its append-only
`ActivityEvent` aggregate, `ActivityEventRepository` port (`internal/ports/storage.go`), memory
adapter for development/testing, PostgreSQL adapter for production, and migration
`032_create_activity_events`. Domain services emit curated events after successful transition seams
through the narrow `ActivityRecorder` port, each with a stable `dedup_key` (Decision 11). The
read-side `ActivityService` windows every historical query by retention, derives needs-attention
from current `UserSession`/`UserGrant`/`ToolApproval` state, assembles threads on read from
`related_refs`, and shapes DTOs through the `SafeFields` registry (Decision 13).

PostgreSQL remains the production store. DynamoDB is not introduced as an application-data
backend, and the existing DynamoDB branch-key cache is not reused. Proposed
[ADR 037](../../adrs/037-activity-event-storage.md) records this selection under ADR 004. The
current factory, configuration, adapters, migrations, Helm migration job, and SQLx/pgx
operational model all implement the accepted storage decision.

**Rationale**: Aggregate-on-read cannot reconstruct historical state after updates, expiry,
revocation, or deletion, so an append-only curated record remains necessary for FR-001, FR-013,
FR-015, and US5. Routine token exchanges appear only as bucketed broker-access milestones,
not as completed downstream actions or individual activity cards. PostgreSQL fits the existing
hexagonal repository pattern and can express principal-scoped, keyset-ordered feed queries plus
the feature's arbitrary combinations of agent, service, grant, outcome, category, and time
filters without new infrastructure.

**Exploratory storage benchmark — not a product requirement or capacity SLA**:

- Exercise a stress population of 2,000 active principals with a **benchmark maximum** of 10,000
  retained curated events per principal. This aligns the per-user read path with SC-005 without
  imposing an application limit: 20,000,000 events across the seven-day window.
- Drive a representative 3 KiB serialized event (labels plus redacted `detail` and
  `related_refs`). The raw payload estimate is about 55.9 GiB at 20 million events; PostgreSQL
  capacity must be measured with `pg_total_relation_size`, including indexes, TOAST, WAL, and
  headroom. The repository's current 10 GiB Helm default is not a sizing commitment for this
  exploratory load.
- The evenly distributed stress dataset implies 33.1 significant-event writes/s over seven days.
  Run a 10x, 330 writes/s burst across principals. This tests recorder and connection-pool
  behavior; it is not an expected MCP/request rate or a product write SLO.
- Seed one principal with 10,000 events for SC-005 and issue keyset feed pages plus every single
  filter and representative combined filters. Capture p50/p95/p99 and execution plans on a warm
  isolated PostgreSQL database. The benchmark records storage behaviour; SC-005 itself remains the
  user finding a recent event in under 30 seconds, not an API latency promise.

**PostgreSQL comparison**: The base B-tree `(principal, occurred_at DESC, id DESC)` serves the
feed and cursor. Partial expression B-tree indexes for the optional `agent_id`, `service_id`, and
`grant_id` keys in `related_refs`, plus a principal/thread/time index, keep selective filters and
threads bounded. The planner can combine those indexes and apply low-cardinality `outcome` and
`category` predicates within a principal's retained window; the benchmark verifies this rather
than adding speculative indexes. Seven-day application filtering remains authoritative, while a
bounded periodic prune controls physical growth. This reuses the existing SQLx adapter,
connection pool, migration, backup, and multi-instance PostgreSQL operations.

**DynamoDB comparison**: A dedicated table with principal/time keys can serve an unfiltered feed.
`GetByID`, thread lookup, and the combined agent/service/grant/outcome/category filters require
additional GSIs or duplicate index items. Filter expressions scan candidates and can underfill
cursor pages. Live-derived needs-attention is not a persisted DynamoDB query key. These access
paths multiply write units and retained-index footprint at the benchmark load. DynamoDB TTL is
asynchronous, so the read-time retention predicate remains required. DynamoDB also needs a new
production adapter, configuration, CDK table, IAM policy, observability, and operational tests.
The branch-key table has an AWS Encryption SDK schema and least-privilege policy that must remain
separate. That cost has no requirement-driven benefit over PostgreSQL and violates ADR 004 unless
a superseding ADR is accepted.

**Alternatives considered**:
- *Aggregate-on-read from existing repositories only* — rejected: cannot recover historical,
  expired, or deleted state; fails FR-015 and US5.
- *Parse structured logs / OTel spans into a feed* — rejected: logs are not a queryable,
  per-principal retention-scoped contract and are brittle to test.
- *DynamoDB activity store* — rejected for this feature: it adds an unapproved production-storage
  backend and index/write amplification for the required query model; do not reuse the encryption
  key-store table.
- *A generic domain event bus* — rejected as over-engineering: no second consumer exists; direct
  recorder calls at established seams are simpler and testable. Revisit only when a second
  consumer appears.
### Decision 3 — One new design-system primitive: `Timeline`

**Decision**: Add a universal `Timeline` primitive at
`web/src/design-system/components/data-display/Timeline/` (`Timeline.tsx`, `index.ts`,
`Timeline.stories.tsx`) with CSF3 `autodocs` + `addon-a11y` stories. Everything else composes
from existing primitives (`Tabs`, `Accordion`, `Card`, `EmptyState`, `StatusIndicator`/`Badge`,
`Alert`, `Select`, `Skeleton`, `Button`).

**Rationale**: No timeline/narrative-sequence primitive exists, yet FR-007/US5 make ordered
timelines a core, repeated element: the grouped overview, the thread detail sequence (US4), the
connected-service lifecycle, and the agent journey are all the same vertical, ordered,
marker-and-connector pattern. Per `DESIGN_PRINCIPLES.md:353-376` this qualifies as universal
(distinct purpose, reused in ≥4 places, needs standardized semantics: `<ol>` ordering for
screen readers, a status-marker slot, and reduced-motion handling). Constitution XI requires
universal patterns to be contributed to the design system with Storybook coverage. The `Timeline`
takes an ordered `items[]` (marker slot, title, description, timestamp, status) and renders an
accessible ordered list; reduced-motion is honored by suppressing entrance/connector animation.

**Alternatives considered**:
- *Inline the docs composition pattern per page* — rejected: duplicated across ≥4 usages,
  inconsistent a11y semantics, violates XI ("universal patterns MUST be contributed").
- *A new app-specific-only timeline under `web/src/components/activity/`* — rejected: it is not
  activity-specific; a reusable, generic timeline belongs in the design system.
- *Build several new primitives (segmented control, chip group, relative-range picker)* —
  rejected: the spec needs relative presets only, which `Tabs`/`Select` already express; no
  additional primitives are justified.

### Decision 4 — Filter state synced to URL query params

**Decision**: Drive the filter state through `react-router` `useSearchParams`. The exact
contract names are `agent_id`, `service_id`, `grant_id`, `window`, `outcome`, `category`, and
`needs_attention`. Pass these names unchanged to `GET /api/activity`. This is the app's first
URL-synced filter pattern.

**Rationale**: US3 requires combined, clearable, shareable filters and one-action deep links.
For example, `/activity?agent_id=…` answers "what has this agent done?". URL state makes each
filter state independently testable and screenshot-able. `useSearchParams` is part of
react-router and adds no dependency.

**Alternatives considered**:
- *Local `useState` filters* — rejected: no shareable deep link, browser-back support, or
  isolated filter-state test.

### Decision 5 — Cursor pagination + "Load more"

**Decision**: `GET /api/activity` returns `{data:{events, threads, next_cursor, retention_days}}`;
the client requests additional pages with `?cursor=` behind a design-system `Button` "Load
more". Default page size configurable; ordering strictly reverse-chronological with a stable
tiebreak (`occurred_at`, then `id`). The cursor also encodes a hash of the filter set; a cursor
presented with different filters is rejected (`400 invalid_cursor`) so a stale cursor can never
silently return pages from another query. `before=` (RFC3339) and a trigram `q=` keyword filter
give the user a way to *jump* rather than page through 10,000 events (SC-005).

**Rationale**: SC-005 requires a specific recent event to remain findable under 10,000 events
with a scannable, responsive surface (FR-013). Cursor pagination is the Zalando-preferred,
stable-under-insert approach and avoids offset drift as new events arrive while viewing (edge
case: "new events arriving while viewing"). This is the first enduser pagination pattern, so the
contract documents it explicitly for reuse.

**Alternatives considered**:
- *Offset pagination* — rejected: drifts as new events arrive; worse for an append-heavy feed.
- *Return everything* — rejected: fails SC-005 at scale.

### Decision 6 — Needs-attention as a live-derived, separate read

**Decision**: `GET /api/activity/attention` derives `reconnect_required` from session refresh
expiry or `UserSession.reconnect_required_at`, with an active grant. It derives `approval_pending`
from non-expired, pending `ToolApproval`s. Each item carries a summary, `group_key`, and
allowlisted `next_step`. An upstream refresh rejection or no usable refresh token after access
expiry sets `reconnect_required_at` on the live session before the historical event is enqueued.
A successful refresh or OAuth callback clears the marker with the token write. A conditional
session update compares the revision read before failure to prevent a late failure from
overwriting a later success. This endpoint never queries activity history. Unresolved items
remain visible beyond history retention.

The SPA polls this endpoint every 30 s while the tab is visible and revalidates on focus
(`Cache-Control: no-store`). The history feed stays on-load/manual. The band shows at most three
items and groups same-kind items.

**Rationale**: FR-003 requires attention from current unresolved state, with no dismissal. A
refresh-failure event can be dropped by the async recorder or pruned while the session remains
unusable. A session with no refresh expiry also reports `IsExpired() == false`, even when it
cannot refresh. Persisting the marker with the session keeps the alert live without a separate
attention/read-state table.

**Alternatives considered**:
- *A `needs_attention=true` filter on the main feed* — kept as a convenience view of
  retained events for current unresolved transitions only. Match `approval.requested` by
  pending `approval_id`, or `session.expired` / `session.refresh_failed` by `session_id` and
  current `token_revision`. Never match by tool name or service ID alone. Apply other feed
  filters and retention; the live band remains authoritative if no matching event is retained.
- *No polling (on-load only)* — rejected: the next step for a pending approval can expire.
- *Derive reconnect state from historical refresh-failure events* — rejected: async loss and
  retention can hide an unresolved failure.
- *Persist a separate needs-attention/read-state table* — rejected: the session already owns its
  reconnect state; attention items remain derived and cannot be dismissed.

### Decision 7 — Server-side redaction and human-readable rendering

**Decision**: The recorder produces already-redacted, already-humanized events through the
`SafeFields` registry (Decision 13). Sensitive fields (secrets, raw tokens,
`ToolApproval.Arguments`, parameters flagged sensitive) are not serialized. Values that existed
can appear as an abstracted marker (`redacted: true`, `•••`) or a shape ("3 arguments,
redacted"). Summaries and structured explanations use safe historical labels, not raw IDs.
Resolve protected-resource URIs to a service before recording activity. Do not put the raw URI
in `related_refs`, the actor/subject, summaries, or detail. The response projects approved IDs
instead of serializing stored JSONB; the browser receives no raw URI even in a related sequence.

**Rationale**: SC-007/FR-011 require that no sensitive value appears anywhere, and FR-010
requires human-readable defaults. Doing redaction and humanization server-side means the browser
never receives sensitive data (defense in depth) and the contract is testable directly against
the API response body (E2E can assert absence of known-sensitive substrings).

**Alternatives considered**:
- *Client-side redaction* — rejected: sensitive data would still traverse the wire and sit in
  browser memory/devtools; fails the "never displayed in the clear" intent.

### Decision 8 — Retention via unified config, default 30 days; advisory-lock prune

**Decision**: Add an `activity` config section to `internal/ports/config.go`
(`retention_window` default `720h`, `page_size` 25, `rollup_bucket` `1h`, `queue_size` 1024,
`prune_interval` `10m`, `prune_batch` 5000, `attention_poll_interval_seconds` 30; durations are
Go duration strings). Every historical activity-event query (feed, detail, related sequence,
and thread count) applies `occurred_at >= now - retention` with one cutoff per request. Detail
returns 404 before that cutoff, even while a row awaits pruning. Live needs-attention has no
retention cutoff. The feed response includes `retention_days` so the UI can explain the boundary.
Add `examples/config/activity.yaml`, document settings in `docs/configuration.md`, and update
the Helm chart (`values.yaml`, ConfigMap template, README) per Principle VII.

**Periodic worker**: the app has no general scheduler. The activity recorder's drainer
attempts `pg_try_advisory_lock(hashtext('activity_prune'))` every `prune_interval`; the holder
deletes one `prune_batch` of rows older than the window. The same periodic tick also observes
bounded batches of due session and pending-approval expiries with their current identities and
dedup keys. Before a callback overwrites an expired session, it observes the previous token
revision so a quick reconnection does not erase its expiry. The memory adapter applies the same
observation and dedup semantics.

**Rationale**: Seven days (the earlier clarification) undermines the feature's purpose — a user
who checks in every other week sees a truncated history with no explanation, and consumer
precedents keep far longer (Google security activity 28 days, GitHub audit 180 days). With
roll-ups (Decision 11) curated events are low-volume, so 30 days costs little. Applying the
window on reads makes the user-visible retention exact even if a prune is delayed; bounded,
lock-coordinated prune batches keep the table from growing without long deletion work or
duplicate work across replicas. The 20-million-row, 3-KiB footprint in Decision 2 is an
exploratory capacity measurement, not a retention quota or product provisioning request. The
spec's Assumptions and Clarifications are updated accordingly (2026-09-22).

**Alternatives considered**:
- *Hard-coded 7 days* — rejected: violates Principle VII (configuration must use the unified
  system).
- *Read-window only, never prune* — rejected: correct responses but unbounded physical growth.
- *Keep 7 days* — rejected: see rationale; remains available via configuration.
- *A generic job scheduler* — rejected: the existing activity worker handles the bounded
  prune and expiry-observation ticks without separate scheduling infrastructure.
- *DynamoDB TTL as the retention mechanism* — rejected: expiry is asynchronous and does not
  replace the application read window or PostgreSQL production decision.
### Decision 9 — Accessibility verification extends the `playwright-go` harness

**Decision**: Add a new page object `tests/e2e/pages/activity_page.go` and a frontend test file
`tests/e2e/frontend/activity_test.go`. Cover WCAG 2.1 AA (SC-006) by: keyboard traversal via
`page.Keyboard` (Tab/Enter/Space) with focus-order assertions; ARIA/non-visual assertions via
`GetByRole`/`GetByLabel` (feed as list, `StatusIndicator role=img` labels, `Alert`
`aria-live`, heading hierarchy); status-without-color by asserting the outcome **text/icon** is
present; 200% zoom via a scaled viewport / CSS zoom with key controls still operable;
reduced-motion via `context.EmulateMedia(prefers-reduced-motion: reduce)` asserting animation is
suppressed. Inject `axe-core` (already a transitive dep, loaded via `page.AddScriptTag`) and
assert zero critical/serious violations on the main states.

**Rationale**: No a11y E2E exists today, but SC-006 mandates an audited WCAG 2.1 AA pass. The
existing `playwright-go` harness already disables animations and injects `X-Remote-User`, so a11y
checks slot in without new tooling; `axe-core` gives an automated baseline while the explicit
keyboard/zoom/reduced-motion assertions cover what axe cannot.

**Alternatives considered**:
- *Manual audit only* — rejected: not repeatable, not a regression guard, not CI-enforceable.
- *Add a separate Node/Playwright a11y runner* — rejected: duplicates the harness and CI wiring
  for no benefit; the Go harness already drives the browser.

### Decision 10 — Event category & outcome model

**Decision**: Curated categories: `delegation`, `session`, `agent_access`, `policy_decision`,
`approval`, `revocation`, `reconnect`, `failure`. `agent_access` means a successful broker token
exchange for an agent, not a downstream action. Outcome enum: `succeeded | failed | blocked |
pending`. Each event carries `category`, `type` (finer sub-kind, e.g. `session.refreshed`),
`actor` (agent | service | user | broker), `subject` (grant | permission_set | service |
session | tool | resource), `occurred_at`, `first_occurred_at`, `occurrence_count`, `outcome`,
`summary`, `redacted`, and `related_refs` (agent/service/grant/session/approval/client IDs used
for filtering, read-time threading, and outbound links). Threads are derived on read from
`related_refs` (service lifecycle by service, agent journey by agent within the window, tool flow
by approval); an event can belong to several threads.

**Alternatives considered (threads)**:
- *Persisted single `thread_key` assigned at emit time* — rejected: an approval event belongs
  to the tool flow, the agent's journey, and the service lifecycle at once; and
  `agent:{id}:{yyyy-mm-dd}` bakes a UTC day boundary into storage while the spec shows times
  in the user's local context. The `related_refs` expression indexes already serve read-time
  assembly.

**Rationale**: Maps 1:1 to the spec's Activity Category, Actor, Affected Subject, and Activity
Thread entities and to the real domain seams identified by the backend scout, keeping the
taxonomy user-comprehensible while covering FR-001's required span (sessions, grants,
delegations, agent activity, policy decisions, failures, revocations, reconnect). See
[data-model.md](./data-model.md).

**Alternatives considered**:
- *One flat "log entry" type* — rejected: FR-007 forbids a plain-log presentation and US5 needs
  category-aware threading.
- *Surface every low-level token exchange* — rejected by clarification: routine high-frequency
  internals are summarized into milestones, not surfaced individually.

---

### Decision 11 — Deduplication keys and roll-ups (FR-016b, FR-001)

**Decision**: Every event carries a stable `dedup_key` derived per `type` (data-model §1.1).
The store enforces `UNIQUE (principal, dedup_key)`. Creation uses the new grant/session ID;
grant updates use a UUID minted once per successful update, and reconnect events use the
committed session token revision. Thus two updates in one second and two OAuth callbacks on one
session remain distinct. Repeated reports of one transition reuse its key.

Routine broker access issuances (`agent.access_issued`), session refreshes, and bucketed
failures/denials roll up into one row per principal/key/bucket. `occurrence_count` and
`occurred_at` update on a new occurrence. A card can say "Broker issued GitHub access for
Research Agent · 41 times". It counts successful broker token exchanges, not completed GitHub
actions. Expiry observations (worker, lazy request, and pre-reconnection check) reuse
`session.expired:{session_id}:{token_revision}` or `approval.expired:{approval_id}`. The
store's unique key collapses repeated reports, even across replicas.

**Rationale**: FR-016b needs stable identity for repeated reports of the same transition, while
FR-001 needs roll-ups for routine broker observations. The key cannot use a Unix-second
timestamp or the session ID alone for a reconnect. Grouping broker exchanges by bucket limits
feed volume without claiming that the broker observed downstream actions.

**Alternatives considered**:
- *Separate `activity_rollups` counter table* — viable, but a second table is not needed for a
  bucketed broker milestone.
- *Emit only the first exchange per session* — loses the number of broker issuances.
- *Client-side collapsing* — rejected: the client only sees one page.

### Decision 12 — Recorder durability: best-effort after successful writes

**Decision**: All transition seams enqueue into a bounded in-process queue (`queue_size`,
default 1024). Grant upsert/revoke, approval transitions, and session store/terminate enqueue
only after their repository writes succeed. Their repositories commit their own transactions;
the session repository writes its own statement. The same worker observes due expiries on its
periodic tick. A callback captures the expired old session before writing tokens. After a
successful token write, it enqueues expiry ahead of reconnection. The recorder drains the queue
using an application-lifecycle context. A full queue or insert error increments
`activity_events_dropped_total{type}` and logs at `ERROR`. Recording never changes the primary
operation's result. Existing structured security audit logs remain independent.

**Rationale**: FR-016a requires the action to complete even when an event insert fails. A failed
PostgreSQL insert inside a shared transaction would abort that transaction unless isolated by a
savepoint. Current repository methods do not expose such a transaction. Post-write enqueueing
preserves the primary action and keeps the exchange hot path free of synchronous inserts, but
the event and its source mutation cannot commit atomically.

**Alternatives considered**:
- *Shared transaction with no savepoint* — rejected: an event insert failure would abort the
  primary mutation, contrary to FR-016a.
- *Transaction-aware repository boundary with an event savepoint* — possible, but it changes the
  current storage contracts and still permits event loss when the insert fails.
- *Unobserved fire-and-forget recording* — rejected: FR-016a requires an operational error.

### Decision 13 — `SafeFields` registry and structured explanation

**Decision**: `internal/domain/activity/safefields.go` declares, per event `type`, the allowed
`detail.context` keys with formatters, the summary wording template, and the structured
explanation template (`trigger`, `basis {kind,id,label}`, `consequence`). The recorder strips
unapproved context keys. It never copies a raw resource URI into activity events, including
`related_refs` and the actor/subject. The browser DTO projects approved identifiers rather than
serializing stored event JSONB. A unit test checks every `type` has an entry and that no context
key matches `token`, `secret`, `arguments`, `authorization`, `assertion`, `code`, `password`, or
`refresh`. Server-side route templates allow only in-app `target_route` values, and the client
validates routes before rendering a `<Link>`. A `correlation_id` stays server-side.

**Rationale**: The 2026-09-21 clarification ("display only pre-approved user-display fields")
had no plan artifact; a free-form `map[string]string` pushed the discipline onto each future
seam author. The registry gives SC-007 a concrete oracle. The three-part explanation follows
what security-activity pages and agent-governance research converge on — the trigger, the
decision basis, and the consequence — and the basis doubles as the FR-008 single-action link.
The route allowlist closes an open-redirect surface that server-supplied routes would otherwise
create.

**Alternatives considered**:
- *Free-form explanation string per event* — rejected: untestable for SC-003 and inconsistent
  across seams.
- *Client-side route validation only* — rejected: the server should never emit a route it
  would not accept.

---

## Open questions requiring stakeholder confirmation (not blocking Phase 1 design)

- **API contract confirmation (Principle IV/X)**: the new `/api/activity*` endpoints in
  [contracts/enduser-activity.openapi.yaml](./contracts/enduser-activity.openapi.yaml) must be
  confirmed by the stakeholder in PR review before implementation. This is a workflow gate, not
  a design unknown.

- **Retention default 30 days** (Decision 8) supersedes the 2026-09-04 clarification's 7-day
  default; recorded in spec.md Clarifications 2026-09-22 and flagged for stakeholder
  confirmation alongside the contract.
- **`pg_trgm` availability** on the managed PostgreSQL offering; an `ILIKE` fallback is
  specified if the extension cannot be enabled.

All other spec-level unknowns are resolved; the **Clarifications** in spec.md (curated-vs-raw
events, needs-attention lifecycle, live-attention retention behavior, relative-preset filtering,
fail-open recording, approved-field display, one-canonical-event) are reflected in the decisions
above.
