# Implementation Plan: End-User Activity & Auditing Experience

**Branch**: `038-activity-audit-experience` | **Date**: 2026-09-04 · **Revised**: 2026-09-24 (artifact remediation) | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `/specs/038-activity-audit-experience/spec.md`

**Design docs**: [research.md](./research.md) · [data-model.md](./data-model.md) ·
[contracts/](./contracts/) · [quickstart.md](./quickstart.md)

## Summary

Deliver a first-class, top-level **Activity** destination in the existing end-user consent SPA
that lets a user inspect, understand, and trust what the broker, connected services, and
authorized agents did on their behalf. The surface is comprehension-first: a needs-attention band
with guided next steps, a scannable narrative feed grouped by day, combinable filters, event
drill-down with related sequences, and connected-service/agent timelines — never a plain table.

Because the current backend exposes only present state (grants, sessions, approvals) and keeps no
queryable per-user history, the feature adds a **durable, curated activity-event store** (new
`internal/domain/activity` bounded context, port, memory adapter for development/testing,
PostgreSQL adapter for production, and migration `032`) written at existing domain transition
seams. Research Decision 2 compares PostgreSQL with DynamoDB and retains PostgreSQL under binding
ADR 004; the existing DynamoDB branch-key cache remains separate. A read-side service assembles
the feed, derives needs-attention live, threads related events **on read**, and redacts sensitive
values through an approved-field registry. Three new read-only enduser endpoints
(`GET /api/activity`, `/api/activity/{event-id}`, `/api/activity/attention`) back the SPA. One new
universal design-system primitive (`Timeline`) is introduced with Storybook coverage; all other UI
composes from existing primitives.

**Revision 2026-09-22.** A review against the current code found that several assumed emission
seams do not exist (no expiry detection job, no denial reason codes, `TokenIssued` not covering
RFC 8693, idempotent approval creation), that FR-016b (one event per action) and FR-001
("summarize into milestones") had no mechanism, that no scheduler exists for the prune, and that
the SPA routes referenced were stale. The plan now: adds a **Phase 0 seam-preparation** step;
introduces `dedup_key` + roll-up counters; records events best-effort after successful writes
without sharing mutation transactions; coordinates prune with an advisory lock; computes threads
at read time; drops per-event `needs_attention`; structures the "why"; adds an approved-field
registry; adds `q=`/`before=` so SC-005 is reachable; raises default retention to 30 days; polls
the attention endpoint; and corrects all routes to the root-mounted SPA (`/delegations`,
`/agents/:agentId`, `/sessions`, `/approvals[/:id]`). See research Decisions 11–13 and
[data-model.md](./data-model.md).

## Technical Context

**Language/Version**: Go 1.27.1 (from `go.mod`, backend); TypeScript 5 + React 19 (SPA)
**Primary Dependencies**: chi v5, sqlx + pgx v5, Viper/Cobra, Ginkgo/Gomega, `playwright-go`
(backend/E2E); Vite 7, react-router, axios, Tailwind 4 + CVA, Storybook, `axe-core` (frontend)
**Storage**: PostgreSQL (production) + in-memory (development/test) via ISP repositories; new
`activity_events` table (migration `032`). DynamoDB is rejected for activity storage (Decision 2).
**Testing**: `go test` (unit), testcontainers PostgreSQL (integration), Ginkgo/Gomega (backend
E2E), `playwright-go` + screenshots + `axe-core` (frontend E2E/a11y), Vitest + Storybook (SPA
unit/visual)
**Target Platform**: Linux server (dual-server :8000 enduser / :14000 admin); modern browsers for
the SPA (root-mounted, `basename="/"` — ADR 035-root-mounted-spa)
**Project Type**: web (Go hexagonal backend + React SPA in one monorepo)
**Performance Goals**: SC-005 is a product UX criterion: a specific recent event remains
findable in under 30 seconds with ≥10,000 events. The separate storage experiment uses a
10,000-event-per-principal stress fixture, 2,000 active principals, and a 330 significant-event-writes/s
burst. It captures p50/p95/p99 and execution plans on a warm isolated PostgreSQL database; it is
exploratory, not a product load, quota, or latency commitment. Cursor pagination uses `(principal, occurred_at
DESC, id DESC)` with the cursor bound to its filter set. Roll-ups (data-model §1.2) group
successful broker access issuances; their counts do not represent downstream actions.
**Constraints**: read-only experience (no new mutation surface); approved `720h` default
for historical activity (2026-09-23) with arbitrary positive representable Go durations;
live unresolved Needs-Attention Items outlast history; exactly one visible event per underlying
action via `dedup_key` (FR-016b); approved-only display fields and no sensitive values in any
response (FR-011, SC-007); raw resource URIs never enter Activity or browser DTOs;
WCAG 2.1 AA (SC-006) and principal isolation (FR-012). Activity recording never changes an
authorized primary result; every security-critical action keeps its independent structured audit.
**Scale/Scope**: per-user scope only (no admin/tenant view); ~8 event categories; one new SPA
route + detail child route; one new design-system primitive; ~5 new frontend hooks/components
groups; new backend bounded context + 3 endpoints + 1 migration; **Phase 0** touches
`tokenexchange` (denial reasons + success hook) and `approval` (`IsNew` guard)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Compared with [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.9.2 (path-correction amendment proposed; acceptance pending). Principle VII is not yet marked compliant.

**Planning Preconditions**:

- [x] **Domain Model**: `ActivityEvent` aggregate, `ActivityThread`/`NeedsAttentionItem` (derived),
  and value objects (`ActorRef`, `SubjectRef`, `ActivityDetail`, `RelatedRefs`) documented in
  [data-model.md](./data-model.md).
- [ ] **Domain Concepts**: Add Activity Event, Activity Category, Actor, Affected Subject,
  Activity Thread, Needs-Attention Item, and Exploration Context to the `ARCHITECTURE.md` glossary
  in the implementation PR. The names are defined in the spec and data model.
- [ ] **Entity IDs**: Add `ActivityEventID` to `internal/domain/id/gen_ids.go`. Regenerate
  `uuid_ids_gen.go` and update `internal/domain/id/AGENTS.md` as ADR 013 requires.
- [x] **Configuration Design**: The planned `activity` section has `retention_window`
  default `720h`, `page_size`, `rollup_bucket`, `queue_size`, `prune_interval`, and
  `prune_batch`. The design uses `internal/ports/config.go`; it does not claim the section
  or YAML example already exists. A 30-second attention poll belongs to the SPA, not the backend.
- [ ] **Config Artifacts**: Add `examples/config/activity.yaml`. Update
  `docs/configuration.md`, the configuration example index, and the Helm values, ConfigMap, and
  README for `activity.*` settings.
- [x] **API Design First**: The OpenAPI fragment is authored in
  [contracts/enduser-activity.openapi.yaml](./contracts/enduser-activity.openapi.yaml). Merge it
  into `api/enduser/openapi.yaml` only during implementation.
- [ ] **API Documentation**: Add end-user API examples under `docs/api/` during implementation.
- [ ] **API Changes**: T013 merges the proposed three-endpoint contract for review; T014
  requires explicit written stakeholder approval before handler implementation (Principle X).
- [ ] **Database Implementation**: Create PostgreSQL migration
  `migrations/032_create_activity_events.{up,down}.sql` with the §10 indexes, the
  `UNIQUE (principal, dedup_key)` constraint, and `CREATE EXTENSION IF NOT EXISTS pg_trgm`
  (verify the managed-PostgreSQL offering allows it; fall back to `ILIKE` on `summary` if not).
  Add `user_sessions.token_revision` and `user_sessions.reconnect_required_at` in the same
  migration. Do not add a DynamoDB application table.
- [ ] **E2E Acceptance Tests**: Add one Ginkgo `It()` per numbered spec scenario across
  `tests/e2e/activity_test.go` (7 backend) and `tests/e2e/frontend/activity_test.go` (22 frontend).
- [x] **E2E Test Mapping**: The table in Testing Strategy assigns each of the 29 keys once;
  this does not mark the red-phase tests written or passing.
- [ ] **E2E Red Phase**: Add tests that compile and fail semantically against the contract before
  implementation.
- [ ] **Frontend Playwright E2E**: Add `tests/e2e/frontend/activity_test.go` and the
  `tests/e2e/pages/activity_page.go` page object.
- [ ] **Frontend Screenshots**: Capture the listed states to `tests/e2e/screenshots/` after the
  frontend E2E suite is implemented.
- [ ] **Design-System Review**: T017 reviews all eight named design guides and records
  component, semantic-token, keyboard-focus, reduced-motion, and responsive evidence before UI
  work. The component plan below is not evidence that this gate has passed.

**Implementation Considerations**:

- [x] **Security Design**: Durable, per-principal Activity is a user-facing historical view;
  existing structured `slog` action audits remain independent. `consent/service.go` records
  successful principal/grant revocation; `token_grant_strategy.go` records local
  `TokenIssued`/`TokenRequestFailed`; `oauth2_audit.go` records principal, method, path, status.
  These examples do not cover all new emission seams, and a generic request log does not prove
  action coverage. T006's matrix checks grant, session, approval, RFC 8693 issuance, and
  verified-principal denial boundaries; T046–T049 retain action logs and add only missing safe
  structured records. An Activity write failure produces an ERROR drop log and metric while the
  authorized mutation still succeeds with its independent action audit. Do not parse logs into
  Activity events. Never log tokens, approval arguments, raw resource URIs, or unapproved labels.
  Server projections use the approved-field registry, principal isolation, and route allowlist;
  fail-closed authorization is unchanged. Unverified public token failures remain operational
  only; no user principal is inferred. Keep `correlation_id` server-side.
- [ ] **Architecture Documentation**: In the same feature architecture PR, add seven glossary
  terms and an Activity subsection to `ARCHITECTURE.md`. Record the under-30-second user task
  among 10,000 events (not a DB p95 SLO), principal isolation, approved-only display fields,
  read-time retention, independent structured audit, non-blocking recorder, bounded queue,
  keyset reads/prune, and multi-replica advisory lock. Do not describe unbuilt components as
  deployed. Link ADR 037 only after acceptance; T105 reviews this evidence before II/V.
- [x] **ADR**: Proposed [ADR 037: Activity Event Storage](../../adrs/037-activity-event-storage.md)
  (renumbered from 035, which collided with `035-root-mounted-spa.md`) records the durable-store
  choice, the dedup/roll-up model, recorder durability modes, and the advisory-lock prune. It
  follows accepted ADR 004 and documents the new repository boundary.
- [x] **Library-First Security**: The design adds no cryptography. Redaction is data shaping.
- [x] **Zalando Contract Design**: The OpenAPI fragment uses resource-oriented paths, `{data}`
  envelopes, `{error,message}` errors, RFC3339, and cursor pagination.
- [ ] **End-User Docs**: Add `docs/api/` examples for the three endpoints during implementation.
- [ ] **Migration and Benchmark Evidence**: Run PostgreSQL migration apply/rollback and the
  manual storage experiment after the repository exists.
- [x] **Hexagonal Architecture**: domain `activity` service depends on the
  `ActivityEventRepository` port and read-side ports for sessions/grants/approvals; adapters
  implement ports; handler is thin.
- [x] **Persistence Patterns**: ADR 004 requires the ISP repository, memory + PostgreSQL
  adapters, sqlx, `StorageError` wrapping, and adapter accessor. DynamoDB's existing encryption
  key-store table, IAM policy, and schema are not application persistence.

**Post-design Result**: OPEN. The storage choice follows accepted ADR 004; ADR 037 remains
proposed. Principle VII awaits acceptance of the separate v1.9.2 path-correction amendment and
implementation of unified Activity bindings. The proposed three-endpoint API awaits T013 merge
and T014 written stakeholder sign-off. T017 design review and T018–T022 semantic-red evidence
also remain open. None of these gates passes solely because this plan describes them.

## Route & Navigation Recommendation

- **New top-level destination**: `Activity` at path `/activity` (the SPA is root-mounted,
  `basename="/"`), a sibling of Agent Delegations (`/delegations`), Tool Authorizations
  (`/approvals`), and Third-Party Sessions (`/sessions`) (FR-016, SC-010).
- **Wiring**: add a lazy `ActivityPage` route in `web/src/App.tsx`. Add `Activity` to the
  shared `navLinks` in `web/src/components/layout/AppLayout.tsx`. Today those links are inside
  `hidden md:flex` and the wrapper provides no sidebar. Render the same primary links in a
  visible, wrapping mobile row below the header. At small widths and 200% zoom, Activity must
  remain visible and reachable in one link activation, with no menu opener or horizontal scroll.
  Mark a link active on exact path or a child path starting with `${href}/`, not an arbitrary
  string prefix. This keeps Activity active at `/activity/:eventId` without matching a peer
  route. Do not touch the orphan `web/src/components/layout/Header.tsx`.
- **Detail**: deep-linkable child route `/activity/:eventId`, rendered as a detail panel **within**
  the activity shell so the surrounding feed/thread stays in view (US4/US5).
- **Filters in URL**: `useSearchParams` owns
  `?agent_id=&service_id=&grant_id=&window=&before=&outcome=&category=&q=&needs_attention=`.
  `cursor` stays outside the URL. Removable chips preserve other filters when one clears;
  clear-all resets every filter. The server applies each filter across all retained pages.
- **Outbound context links**: Needs-attention next steps and `ActivityEvent.related_links` use
  current principal-owned targets and the allowlist (`/sessions`, `/approvals`,
  `/approvals/:id`, `/agents/:agentId`). The session page adds a "Reconnect" action for its
  live attention item through the existing OAuth authorize flow; Activity adds no mutation.
- **Inbound links**: `AgentGrantDetailPage.tsx` keeps "View activity" for the agent using
  `agent_id`. When `useAgentGrants` returns a grant, add a separate grant activity link using
  `grant_id=grants.id` from `web/src/types/consent.ts`. Never substitute the agent ID or show
  that grant link without a grant. `ThirdPartySessionsPage.tsx` links a service to
  `/activity?service_id=…`.

For SC-002, seed an agent and a connected service with activity absent from page one. Start
the agent task at `/agents/{agent_id}` and the service task at `/sessions`; one "View activity"
link applies the exact `agent_id` or `service_id` and finds all retained matches, including
later pages. Start window/outcome tasks at `/activity`; use existing controls within two
activations. Keyword search alone cannot identify every off-page entity event.

## UX Structure & Event Grouping Model

**Summary view (`/activity`)**, top to bottom:
1. **Needs-attention band** — `role="region" aria-label="Needs your attention"` containing
   design-system `Alert` (`warning`) cards from `GET /api/activity/attention`, ordered oldest
   unresolved first. **At most 3 visible**; the rest collapse behind "+N more". Items sharing a
   `group_key` collapse into one card ("4 tool calls waiting for your approval → Review all" →
   `/approvals`). Polled every 30 s while the tab is visible and revalidated on focus; a
   "Updated 2 min ago · Refresh" line sits in the page header. Empty → settled `EmptyState`
   "nothing needs your attention" (US2, FR-003/004). Not `aria-live`: it does not change under
   the user's hands except on poll, and a polite live region would re-announce every 30 s.
2. **Filter bar** — `<form role="search">`: `Select` for category/outcome, `Tabs` (`pill`) for
   the relative time-window presets `24h | 7d | 30d | all`, a keyword `Input` (`q`), a
   needs-attention toggle (only exact retained unresolved approval/session transitions), and a
   clear-all control. Agent/service/grant are facets from approved `related_refs`; actor/subject
   labels on a card set only an existing `agent_id` or `service_id`. A card with an approved
   `related_refs.grant_id` offers “View this grant's activity” and sets that exact `grant_id`
   in the URL; without the reference it offers no grant action. An active `grant_id` renders
   as a removable chip, survives additional filters, and clears with clear-all. Active filters
   show an `aria-live="polite"` result count. All filter state lives in the URL.
3. **Activity feed** — reverse-chronological, grouped by day in the browser timezone (Today /
   Yesterday / date) as `<section aria-labelledby>` with an `<h2>` date heading. Each item is a
   narrative **ActivityCard** (`Card as="article"` + `OutcomeStatus` + humanized summary answering
   what/when/who/what-affected/outcome). Roll-up cards show broker access issuances and the
   local time span ("Broker issued GitHub access for Research Agent · 41 times, 09:00–10:00").
   Cards for `agent_access` / `policy_decision` carry a secondary link "Don't recognise this? Review this agent's access" →
   `/agents/:id` (where revoke already lives; zero new mutation surface). Not a table (FR-007).
   Incremental "Load more" via `next_cursor` (SC-005, FR-013). T054 adds `RetentionFooter`
   below the historical feed after a valid response loads: "Activity before {local date and
   time} is not shown." Format `retention_cutoff_at` via browser-locale `Intl.DateTimeFormat`,
   as `SessionCard.tsx` does. Show it for populated, paginating, unfiltered empty, filtered
   no-match, and detail-child-route views. Never show a history cutoff on live attention items.
4. **Threads** — related events (service lifecycle, agent journey, tool-call flow) render with the
   new `Timeline` primitive, **collapsed by default** behind an `Accordion` whose summary carries
   the count ("GitHub connection · 6 events"); `truncated` threads show "N earlier events → open
   thread" which navigates to `/activity?service_id=…` (US5).
5. **Empty/no-match states** — an unfiltered empty feed says "No activity in the period shown.
   New activity will appear here." This is also true when all rows aged out. Filtered no
   matches show a reset action and the same exact retention boundary (FR-014, SC-009).
   An empty retained `needs_attention=true` feed never hides live attention or its next steps.

**Detail view (`/activity/:eventId`)** — full context (actor, subject, timing, outcome), the
structured **why** rendered as three lines (*trigger* → *basis* → *consequence*; the basis is the
FR-008 back-link), approved context fields (e.g. scope delta for `grant.updated`, `argument_count`
for tool calls), the bounded related **sequence** as a `Timeline` with "N more" when
`sequence_truncated`, redacted values shown as shape ("3 arguments, redacted"), honest pending
state, and single-action links back to related consent/session/agent context (US4,
FR-005/008/009/011). An event older than retention returns 404, even before prune removes it;
its related sequence also excludes events beyond retention.

The feed card and detail both read the required response-only `ActivityEvent.related_links`
array. T042 selects a safe, per-type route (§3.3); T050 omits missing, cross-principal, or deleted
targets, so historical text remains visible without a broken link. Link selection uses existing
agent/session/approval read ports and reuses each unique target lookup per page (at most 100).
Lookup errors omit the link and produce an operational log; no raw route or unapproved label
is emitted. Both views guard `target_route` with `web/src/utils/inAppRoute.ts`.

**Event grouping model** (categories → domain seams; full table in
[data-model.md](./data-model.md) §4): `delegation`, `session`, `agent_access`, `policy_decision`,
`approval`, `revocation`, `reconnect`, `failure`. **Outcome**: `succeeded | failed | blocked |
pending`. **Roll-ups** (data-model §1.2): `agent.access_issued` counts successful broker token
exchanges; `session.refreshed` groups routine refreshes. A broker exchange does not prove that a
downstream action occurred. **Needs-attention** derives from live session reconnect state and
pending approvals. It outlives activity retention and clears when the source state resolves.

**Wording templates**: one template per event `type` (data-model §3.1/§3.2) is written in the
Phase 2 design PR — before any code — so SC-003 wording can be reviewed with stakeholders.

## API Contract Changes

New read-only enduser endpoints (full schema:
[contracts/enduser-activity.openapi.yaml](./contracts/enduser-activity.openapi.yaml)), merged into
`api/enduser/openapi.yaml`:

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/activity` | Filterable feed, read-derived threads, exact RFC3339 UTC `retention_cutoff_at`, and response-only `ActivityEvent.related_links` |
| `GET` | `/api/activity/attention` | Live-derived Needs-Attention Items (`group_key`, oldest first), unaffected by historical retention, `Cache-Control: no-store`, SPA polled |
| `GET` | `/api/activity/{event-id}` | Retained event with inherited `ActivityEvent.related_links`, explanation, approved context, bounded sequence; 404 beyond retention |

Conventions match the existing API (`{data}` envelope, `{error,message}`, RFC3339,
`X-Remote-User`). This introduces the **first cursor-pagination pattern** in the enduser API
(documented for reuse); the cursor is bound to the filter set and a mismatch is `400
invalid_cursor`. `/api/activity/attention` is registered **before** `/api/activity/{event-id}`
and a routing test asserts precedence. All responses are per-principal scoped and pre-redacted
server-side. The browser DTO omits raw resource URIs, even in `related_refs` and related
sequences; `detail.pending` is not returned because `outcome=pending` supplies the state.
The revised three-endpoint contract, including `retention_cutoff_at` and shared
`ActivityEvent.related_links`, **requires separate written stakeholder approval** (Principle X)
after T013 merges it and before any handler implementation.

**Thread response mapping**: threads are derived on read from `related_refs`. The API exposes an
`ActivityThreadResponse` with ordered `event_ids` (present on this page), `total_events` and
`truncated`. The client resolves the IDs before it renders the Timeline and shows "N earlier
events" when truncated. `ActivityEvent` no longer carries `thread_key` or `needs_attention`.

## Frontend Architecture Touchpoints

- **Routing/nav**: `web/src/App.tsx` (+2 routes), `web/src/components/layout/AppLayout.tsx`
  (shared primary links visible as a wrapping row on mobile and in the desktop header).
- **Pages**: `web/src/pages/activity/ActivityPage.tsx` (summary), `ActivityDetailPanel` (child
  route).
- **App components** `web/src/components/activity/`: `NeedsAttentionPanel`, `ActivityFilterBar`,
  `ActivityFeed`, `ActivityCard`, `ActivityThread`, `ActivityDetail`, `OutcomeStatus` (maps
  outcome→`StatusIndicator` label+icon+text).
- **Hooks** (canonical `useSessions` shape): `web/src/hooks/useActivity.ts` (feed + `loadMore`),
  `useActivityEvent.ts`, `useNeedsAttention.ts`.
- **Service + types**: `web/src/services/api/activity.ts` (typed `apiClient.get`, short-TTL cache
  via `services/api/cache.ts`) and `web/src/types/activity.ts` (hand-written, synced with OpenAPI —
  see [contracts/frontend-types.md](./contracts/frontend-types.md)).
- **Design system (new universal primitive)**:
  `web/src/design-system/components/data-display/Timeline/{Timeline.tsx,index.ts,Timeline.stories.tsx}`
  — ordered `items[]` (marker slot, title, description, timestamp, status), rendered as an
  accessible `<ol>`, reduced-motion aware; CSF3 `autodocs` + `addon-a11y` stories covering
  variants/empty/loading/reduced-motion. Barrel export updated. Semantic tokens only.
- **App components (additional)**: `ActiveFilterChips`, `RollupCard` variant of `ActivityCard`,
  `RetentionFooter`; `web/src/utils/inAppRoute.ts` (route allowlist guard, unit-tested).
- **Existing pages**: `AgentGrantDetailPage` and `ThirdPartySessionsPage` gain "View activity"
  links into the filtered feed. `ThirdPartySessionsPage` also uses `useNeedsAttention` to show a
  "Reconnect" control for a marked service, even when its existing `is_expired` field is false.
  That control starts the existing third-party OAuth authorization route.
- **T017 design-system review gate (unchecked)**: Before frontend work, review
  `INDEX.md`, `DECISION_TREES.md`, `COMMON_MISTAKES.md`, `COMPONENT_PAIRING_GUIDE.md`,
  `DESIGN_PRINCIPLES.md`, `TOKEN_GUIDE.md`, `ACCESSIBILITY_GUIDE.md`, and `MOTION_GUIDE.md`
  in `web/src/design-system/docs/`. Record actual reuse and focus/zoom evidence; T106/T117
  check it. Only `Timeline` qualifies as a new universal primitive.
- **Planned component rules**: `Timeline` uses a semantic `<ol>` with `StatusIndicator` text
  and icons and `Accordion` for disclosure. Feed cards use `Card as="article"` with `Badge`,
  `StatusIndicator`, and a ghost `Button` or descriptive router link for context. Grant facets
  compose existing `Select`/`Tabs` and `Button`; `ActiveFilterChips` stays app-specific.
  `RetentionFooter` uses `text-secondary` and a local timestamp, not a new primitive.
  Use `text-trust-deep` headings, `text-neutral-700` body, semantic status colors, and no
  `gray-*` or extended palettes. Controls need labels, visible `border-focus` focus rings,
  keyboard activation, 4.5:1 text/3:1 UI contrast, and 44×44px mobile targets. At 200% zoom
  and mobile widths, the primary links wrap and cards, chips, and Timeline reflow without
  clipping or horizontal scrolling. Use design-system timing tokens (150ms hover/focus,
  200ms state, 300ms card/accordion, 500ms page); suppress nonessential motion with
  `prefers-reduced-motion` while retaining content and focus.
- **Backend**: new bounded context
  `internal/domain/activity/{event.go,service.go,recorder.go,safefields.go,templates.go,threads.go,prune.go}`;
  `ActivityEventRepository` in `internal/ports/storage.go`; memory and PostgreSQL adapters in
  `internal/adapters/storage/{memory,postgres}/activity_events.go`; principal/keyset, unique
  dedup, selective `related_refs` expression, optional `pg_trgm` keyword, and prune indexes
  in migration `032`. That migration adds `UserSession.token_revision`,
  `reconnect_required_at`, and due-time indexes for sessions and pending approvals. An
  expiry-read port supplies bounded due queries
  through both storage adapters. The session repository fills the session with its committed ID
  and revision on each write. A small status port conditionally
  marks refresh failure against that revision; refresh success/reconnection clears the marker
  with its token write. Wire the activity worker to observe due expiries in bounded batches on
  its existing periodic tick, and observe an expired session's prior revision before reconnection
  overwrites it. Add handler `internal/adapters/http/handlers/activity/handler.go`; wire the
  activity service/repo/handler and async recorder in `internal/app/handlers.go` +
  `internal/app/builder.go` (drainer tied to app lifecycle); routes in
  `internal/adapters/http/routing/enduser.go`; recorder calls at [data-model.md](./data-model.md)
  §11. Public local token-grant failures without a verified principal stay in structured
  operational audit logs. OTel counter `activity_events_dropped_total`. No DynamoDB
  application adapter or resource is added.
- **Phase 0 touchpoints (existing code)**: `internal/domain/tokenexchange/service.go` — typed
  `DenialReason` on exchange errors and a success hook after `Exchange` returns; the approval
  service's `CreateApprovalResult.IsNew` is respected by the seam (no change needed in
  `approval`, but a test pins the behaviour).

## Project Structure

### Documentation (this feature)

```text
specs/038-activity-audit-experience/
├── plan.md              # This file
├── research.md          # Phase 0 decisions
├── data-model.md        # Phase 1 entities, storage, emission seams
├── quickstart.md        # Phase 1 validation guide
├── contracts/
│   ├── README.md
│   ├── enduser-activity.openapi.yaml   # 3 endpoints, merged into api/enduser/openapi.yaml
│   └── frontend-types.md               # web/src/types/activity.ts contract
└── tasks.md             # Phase 2 output (/speckit-tasks — NOT created here)
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── activity/                       # NEW bounded context
│   │   ├── event.go                    # ActivityEvent aggregate + value objects + dedup keys
│   │   ├── service.go                  # read-side: feed assembly, live needs-attention, cursor binding
│   │   ├── threads.go                  # read-time thread derivation from related_refs
│   │   ├── safefields.go               # approved-field registry per event type (SC-007 oracle)
│   │   ├── templates.go                # summary + structured-explanation wording templates
│   │   ├── recorder.go                 # ActivityRecorder: post-write bounded queue + drop metric
│   │   └── prune.go                    # advisory-lock coordinated bounded prune
│   ├── storage/user_session.go           # live reconnect marker + token revision
│   ├── tokenexchange/service.go        # Phase 0: DenialReason + success hook
│   └── id/
│       ├── gen_ids.go                  # + ActivityEventID row
│       └── uuid_ids_gen.go             # regenerated
├── ports/
│   ├── storage.go                      # + ActivityEventRepository and session status port
│   └── config.go                       # + Activity config section
├── adapters/
│   ├── storage/{memory,postgres}/activity_events.go   # NEW adapters
│   └── http/
│       ├── handlers/activity/handler.go               # NEW enduser handler
│       └── routing/enduser.go                         # register /api/activity*
└── app/
    ├── handlers.go                     # + Activity handler field
    └── builder.go                      # wire activity service/repo/handler + post-write recorder

migrations/
├── 032_create_activity_events.up.sql    # activity table + user_sessions reconnect fields
└── 032_create_activity_events.down.sql
adrs/
└── 037-activity-event-storage.md       # Proposed activity storage decision (renumbered from 035)

api/enduser/openapi.yaml                # + Activity paths/schemas
examples/config/activity.yaml           # NEW
charts/agentic-identity-broker/         # values.yaml + ConfigMap + README (activity.*)

web/src/
├── App.tsx                             # + /activity, /activity/:eventId
├── components/layout/AppLayout.tsx     # + Activity navLink; prefix-based active match
├── pages/activity/ActivityPage.tsx     # NEW
├── pages/{AgentGrantDetailPage,ThirdPartySessionsPage}.tsx   # + activity links; sessions reconnect control
├── components/activity/*               # NEW app components (incl. ActiveFilterChips, RollupCard, RetentionFooter)
├── hooks/{useActivity,useActivityEvent,useNeedsAttention}.ts   # NEW (useNeedsAttention polls while visible)
├── services/api/activity.ts            # NEW
├── types/activity.ts                   # NEW
├── utils/inAppRoute.ts                 # NEW route allowlist guard
└── design-system/components/data-display/Timeline/*            # NEW universal primitive

tests/e2e/
├── activity_test.go                    # backend Ginkgo (1:1 scenarios)
├── frontend/activity_test.go           # Playwright + a11y + screenshots
├── pages/activity_page.go              # NEW page object
└── screenshots/                        # captured states
```

**Structure Decision**: Web application (Go hexagonal backend + React SPA). The feature adds one
new backend bounded context (`internal/domain/activity`) following the established
domain→ports→adapters→app flow, three read-only enduser endpoints, one migration, and a new SPA
route plus one universal design-system primitive. No existing structure is reorganized.

## Implementation Phase Overview

| Phase | Purpose | Applies? |
|-------|---------|----------|
| **Phase 0 · T001–T004** | Behavior-preserving token-exchange denial/success seams and approval `IsNew` test; no navigation | Separate seam review unit |
| **Phase 1 · T005** | Setup: check migration number, dependencies, Go 1.27.1, and paths | Required |
| **Phase 2 · T006–T022** | Design, stakeholder gates, and semantic-red backend/frontend acceptance | **MANDATORY** |
| **Phase N design · T105–T107** | Verify architecture, approved API/design review, and 29 semantic-red scenarios after T022 and **before T023** | **MANDATORY gate** (section remains at end of tasks.md) |
| **Phase 2.7 · T023–T030** | Entity, ports, adapters, migration, read-only handler scaffold | Intermediate review unit only |
| **Phase 2.5 · T031–T035** | Unified Activity config, session revision/marker, Timeline primitive | Required after scaffold |
| **US1 · T036–T055** | Recorder T045, navigation T053, feed and UI; then US2–US5 (T056–T104) | Story implementation |
| **Phase N implementation · T108–T120** | Principle-grouped evidence after all stories | **MANDATORY** |

- [x] Phase 0 (refactoring): **include** — the review found the assumed seams for denial reasons
  and RFC 8693 success do not exist; they are small, behaviour-preserving changes to
  `tokenexchange` that must land (with tests) before any recorder call can be wired.
- [x] Phase 2.7 (entity boilerplate): **include** — introduces the `ActivityEvent` entity with new
  port, memory + postgres adapters, and HTTP handler; a scaffold-only PR (repository stubs,
  handler returning 501, migration) keeps the business-logic PRs reviewable.

## Testing Strategy

### Backend + frontend E2E (Ginkgo/Gomega) — 29 unique scenario keys

**Backend**: `tests/e2e/activity_test.go`, dual-server bootstrap and `AuthenticatedGET`.
**Frontend**: `tests/e2e/frontend/activity_test.go`, Playwright page objects and browser context.
T018 seeds isolated grants, sessions, approvals, denials, sensitive data, deleted references,
aged-out history, a second principal, and high-volume events. T019 writes only backend rows;
T021 writes only frontend rows. Each key names exactly one Ginkgo `It()` across both suites.

| Scenario | Suite | Unique `It()` title and direct observation |
|---|---|---|
| US1 #1 | Backend | recent events ordered with what/when/actor/subject/outcome |
| US1 #2 | Backend | technical transition has a plain-language summary without raw IDs |
| US1 #3 | Frontend | succeeded, failed, blocked show icon plus text without color dependence |
| US1 #4 | Frontend | unfiltered empty feed explains new activity and shows retention boundary |
| US1 #5 | Backend | deleted actor still appears with its historical display label |
| US1 #6 | Frontend | primary Activity link opens in one activation at desktop/mobile/200% zoom |
| US1 #7 | Backend | repeated transition has one row, distinct transitions remain separate, and roll-up count means broker issuances |
| US2 #1 | Frontend | unresolved items appear in a distinct, labelled attention region |
| US2 #2 | Frontend | reconnect item explains state and reaches existing reconnect flow |
| US2 #3 | Frontend | pending tool request reaches its approval review |
| US2 #4 | Backend | blocked decision explains cause and stays out of attention without next step |
| US2 #5 | Frontend | empty attention region states that nothing needs attention |
| US2 #6 | Frontend | resolved item clears on refresh while history records resolution |
| US3 #1 | Frontend | agent filter narrows feed and shows active filter |
| US3 #2 | Frontend | service filter excludes other services |
| US3 #3 | Frontend | selected relative window excludes earlier events |
| US3 #4 | Frontend | outcome filter shows only selected outcome |
| US3 #5 | Frontend | needs-attention filter shows only current unresolved transitions |
| US3 #6 | Frontend | grant facet plus second filter combine, then clear restores full feed |
| US3 #7 | Frontend | unmatched filters show reset action and retention boundary |
| US4 #1 | Frontend | event detail remains inside Activity with full context and why |
| US4 #2 | Backend | related transitions appear in deterministic progression order |
| US4 #3 | Frontend | related context link opens in one activation |
| US4 #4 | Frontend | sensitive and unapproved fields never appear in rendered detail |
| US4 #5 | Frontend | pending event does not imply success or failure |
| US5 #1 | Backend | connected-service lifecycle forms one derived thread |
| US5 #2 | Frontend | agent timeline shows broker milestones without downstream-use claims |
| US5 #3 | Frontend | 10,000-event view stays scannable and exposes target via search |
| US5 #4 | Frontend | expanded timeline opens detail without losing surrounding narrative |

Keep supplemental backend API tests for filter intersection, invalid cursor, principal
isolation, approved fields, read-time retention, FR-014 boundary, independent operational
audit/drop behavior (FR-016a), and unaffiliated-token denial. Keep a separate frontend `It()`
for SC-008's direct-card link. US4 #3 alone owns the numbered detail-context link. Supplemental
tests do not acquire story keys or change the count of 29.

**Red phase**: assertions target concrete contract values (status codes, `data.events[…].summary`,
`redacted:true`, `next_step.target_route`), compile, and fail semantically before implementation.
No `XIt`/`Skip`, no red-phase comments.

### Frontend Playwright E2E + Screenshots

**Location**: `tests/e2e/frontend/activity_test.go`, page object `tests/e2e/pages/activity_page.go`
· seeding via direct `GetTestStorage()` repositories · auth via `X-Remote-User` in the browser
context. Screenshots gated by `E2E_CAPTURE_SCREENSHOTS`, saved to `tests/e2e/screenshots/`.

### Interaction proof (SC-002/004/008/010)

Use `tests/e2e/pages/activity_page.go` to count each click, Enter/Space activation, menu
opening, or completed query submission as one interaction. Passive rendering is not an
interaction. Record starting URL, action sequence, filter/target result, and count:

| Outcome | Starting state and observable target | Maximum |
|---|---|---|
| SC-002 agent/service | Seed an agent and connected service absent from page one. From `/agents/{agent_id}` or `/sessions`, follow one existing-page "View activity" link for the exact ID; all retained matches, including later pages, appear. | 2 activations |
| SC-002 window/outcome | Start at `/activity`, use the existing relative-window or outcome controls, and check matching rows. A submitted keyword alone cannot prove all off-page entity results. | 2 activations |
| SC-004 | Start from a visible live attention item and enter the existing reconnect or approval resolving flow. | 2 activations |
| SC-008 | Start from a relevant Activity **feed card** with an approved context link; activate the first server-approved `ActivityEvent.related_links` route directly. The detail link is checked separately by US4 #3. Do not count an extra detail-opening click as success. Missing/deleted targets retain text without a link. | 1 activation |
| SC-010 | Start from a non-Activity app route; open the top-level Activity link on desktop, mobile, and at 200% zoom, without a menu opener. | 1 activation |

T020/T021/T055/T069/T081/T092/T116 record these counts. Guard direct-card and detail routes
with `web/src/utils/inAppRoute.ts`. T042 selects `/approvals/{approval_id}` for approvals,
`/sessions` for session/reconnect, and `/agents/{agent_id}` for agent/grant/policy when the
principal still owns the target. The server omits absent, deleted, or cross-principal links.

### Accessibility Test Plan (WCAG 2.1 AA — SC-006)

Run in the frontend suite (`--focus "Activity accessibility"`), extending the `playwright-go`
harness (no a11y E2E exists today):
- **Keyboard operable (FR-017)**: Tab/Shift-Tab/Enter/Space traversal of nav, filter bar, tabs,
  accordion, timeline, "Load more", and detail links; assert logical focus order and activation.
- **Non-visual equivalents (FR-018)**: assert ARIA — feed as list, `Timeline` as `<ol>`,
  `StatusIndicator role=img` aria-label, `Alert` `aria-live`, heading hierarchy, labelled filter
  controls; inject `axe-core` (transitive dep) via `page.AddScriptTag` and assert zero
  critical/serious violations on the main states.
- **Status without color (FR-019)**: assert each outcome shows icon + text, not color alone.
- **Reduced motion (FR-020)**: `context.EmulateMedia(prefers-reduced-motion: reduce)`; assert
  timeline/summary animation suppressed, no information lost.
- **High zoom / small viewport (FR-021)**: at 200% zoom and narrow mobile width, assert the
  primary Activity link stays visible, keyboard-operable, and opens `/activity` with one
  activation, without opening a menu or scrolling horizontally. Key content and controls
  remain usable without overlap or clipping.

### Playwright Screenshot State List (`tests/e2e/screenshots/`)

1. `activity_overview_populated.png` — grouped feed + needs-attention band
2. `activity_needs_attention_reconnect.png` — reconnect item with next step
3. `activity_needs_attention_settled.png` — "nothing needs your attention"
4. `activity_empty_no_activity.png` — first-time empty state
5. `activity_filters_applied.png` — narrowed by agent + `blocked`, active-filter indicator
6. `activity_filters_no_matches.png` — no-match state with reset
7. `activity_event_detail.png` — detail with related sequence + back-links
8. `activity_event_detail_redacted.png` — redacted/abstracted sensitive values
9. `activity_thread_timeline.png` — service-lifecycle timeline (Timeline primitive)
10. `activity_pending_event.png` — in-flight pending outcome shown honestly
11. `activity_high_volume_scannable.png` — high event count, grouped + incremental
12. `activity_reduced_motion.png` — reduced-motion variant
13. `activity_zoom_200.png` — 200% zoom readable/usable
14. `activity_keyboard_focus.png` — visible keyboard focus on a control
15. `activity_rollup_card.png` — rolled-up routine milestone with count and local time span
16. `activity_attention_grouped.png` — attention band with a grouped "+N more" item
17. `activity_retention_footer.png` — end-of-history explanation

### Opt-in human study (SC-001/003/005; not an acceptance `It()`)

T018 seeds exactly 10,000 **distinct retained** `ActivityEvent`s for `user@example.com`,
including every emitted type in T006's reviewed registry (the current model lists 18).
Include a uniquely named recent target at rank 250. Store the approved five-field
what/when/who/affected/outcome answer rubric for each type with the fixture. Use timestamps
relative to startup so all rows remain within `720h`. The frontend Ginkgo suite closes storage
after each `It()`, so T119 adds an opt-in `tests/e2e/study/activity/main.go` driver outside both
acceptance suites. Do not treat an automated text check as comprehension proof.

The driver starts an isolated PostgreSQL 15 database with
`bootstrap.NewPostgresFixture(ctx)` (production migrations). Set `ports.StorageConfig` backend
to `postgres`, connection URL to `PostgresFixture.ConnectionURL`, read timeout 5s, write timeout
10s. Construct the production storage adapter via `storageadapter.NewAdapter` in
`internal/adapters/storage/factory.go` and seed through its planned Activity repository. Reuse
`helpers.NewMockUpstreamOAuth2Server`, `fixtures.OAuth2ConfigWithUpstream`, and set
`ports.Config.Storage` to the same PostgreSQL config **before**
`bootstrap.NewServerFactory(...).BuildApp(storage)` and
`bootstrap.NewEndUserTestServer`. Use built SPA assets after `just web-build`.

Launch headed Chromium with a fresh browser context, viewport 1280×800, and
`X-Remote-User: user@example.com`. Print `Activity study ready: <URL>/activity` only after a
fixture card renders. Exit nonzero on any setup, seed, or browser error. Handle SIGINT,
SIGTERM, and every setup error with one cleanup path: close browser/context, server, mock
upstream, adapter, and `PostgresFixture.Close`. Ryuk is disabled by the fixture; never rely on
process exit to terminate its container. Each new launch creates a newly migrated database and
fresh browser context. If T016 chooses `ILIKE`, the study uses its migrated fallback schema;
T120 captures the 10,000-row fallback query plan separately. Never substitute a faster
`pg_trgm` run for a failed `ILIKE` user task.

Recruit ten distinct participants using the same 1280×800 viewport and Chromium build.
Start each stopwatch after a fixture card confirms feed readiness. Assign each participant at
least two **different** emitted types, round-robin until every type has a timed, no-detail
comprehension trial. Score SC-001 **per trial**: all five rubric answers correct within 10s,
without detail, for every emitted type. For SC-003, give each participant one preselected
paraphrase prompt and require at least 9/10 correct without jargon; score **per participant**.
For SC-005, run ten distinct target-finding trials at rank 250 among 10,000 events; each must
finish in under 30s. Record anonymized answers, types, elapsed times, activations, browser
build, viewport, fixture count, and failures in this plan's verification evidence. T120 storage
timings are diagnostic and never substitute for these tasks.

**Verification evidence (not yet collected):** SC-001 trial rubric/times and type coverage:
unverified. SC-003 participant paraphrases (target ≥9/10): unverified. SC-005 ten target
trials (target each <30s): unverified. If participants or a container runtime are unavailable,
leave these outcome claims unverified; do not invent scores.

### Unit & Integration

- **Unit** (TDD, red first): `internal/domain/activity/*_test.go` — feed assembly, read-time
  threading, live needs-attention derivation, SafeFields stripping (every `type` has a registry
  entry; no deny-listed key), dedup-key derivation per type, roll-up bucketing, cursor encoding +
  filter binding, wording templates, route allowlist. Table-driven. `tokenexchange` Phase 0:
  every failure path yields the expected `DenialReason`.
- **Integration** (testcontainers PostgreSQL): `internal/adapters/storage/postgres/activity_events_test.go`
  — record (idempotent upsert incl. concurrent roll-up increments), list, related, prune under
  `pg_try_advisory_lock` with two competing connections, `032` migration apply + rollback +
  repeat, principal/keyset ordering, selective `related_refs` filters, selected `pg_trgm` or
  `ILIKE` keyword path, and cross-principal isolation. An unpruned expired event returns
  404 by ID and cannot enter the related sequence. A rejected session refresh must
  persist its marker even when the history insert fails; a later successful token write
  must clear it without a stale retry.
- **Exploratory storage benchmark** (manual, isolated PostgreSQL; not normal CI or a product SLO):
  `BenchmarkActivityEventRepository` seeds the SC-005 10,000-event principal and measures all
  filters/threads; it separately exercises the 2,000-principal, 20-million-event, 3-KiB stress
  dataset and 330-writes/s burst. Capture p50/p95/p99, `EXPLAIN (ANALYZE, BUFFERS)`, and physical
  table/index/WAL size; Decision 2 defines the workload and measurements.
- **SPA**: Vitest for hooks/components (filter→query mapping, cursor reset on filter change,
  outcome mapping, thread IDs resolve against page events + truncated affordance, attention band
  cap/grouping, polling pauses when hidden, `inAppRoute` guard rejects absolute/unknown paths,
  empty/no-match);
  `Timeline` Storybook stories (variants, empty, loading, reduced-motion) with `addon-a11y`.
- **Coverage goals**: critical paths in the activity domain + all new postgres repository methods
  in integration; 100% of spec scenarios in E2E; all listed UI states in frontend E2E.

## Complexity Tracking

No constitution violations require justification. The one added universal primitive (`Timeline`)
is justified under Principle XI / `DESIGN_PRINCIPLES.md` (distinct, reused in ≥4 places, needs
standardized a11y semantics) — see research Decision 3. The durable PostgreSQL event store is the
minimal design that satisfies FR-015/US5 and accepted ADR 004; research Decision 2 rejects
DynamoDB's additional application-data infrastructure and query-index amplification, and rejects
a generic event bus as over-engineering.

Added complexity accepted in the 2026-09-22 revision, and why:
- **`pg_trgm` extension** — optional search index when the managed PostgreSQL offering permits
  it. The documented `ILIKE` fallback must pass the same SC-005 user study; neither query
  timing nor a faster alternate run proves the user task.
- **Async recorder goroutine + advisory-lock prune** — the app has no scheduler; this is the
  smallest coordination that is multi-replica safe and needs no new infrastructure.
- **Phase 0 in `tokenexchange`** — unavoidable: the seams the feature needs do not exist.
- **Polling `/attention`** — the SPA has never polled, but a pending approval expires in minutes
  and an on-load-only band would routinely offer a dead next step.
