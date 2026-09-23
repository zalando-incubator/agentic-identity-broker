# Implementation Plan: End-User Activity & Auditing Experience

**Branch**: `038-activity-audit-experience` | **Date**: 2026-09-04 · **Revised**: 2026-09-22 (plan review) | **Spec**: [spec.md](./spec.md)

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

**Language/Version**: Go 1.26.8 (backend); TypeScript 5 + React 19 (SPA)
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
**Constraints**: read-only experience (no new mutation surface); 30-day rolling retention default
for historical activity (configurable; spec clarification 2026-09-22); unresolved needs-attention
items remain visible until the underlying state resolves; exactly one visible event per underlying
action via `dedup_key` (FR-016b); no sensitive value in any response or view (FR-011, SC-007) —
enforced by server-side approved-field response projection and the `SafeFields` registry;
raw resource URIs never enter activity events or browser DTOs; WCAG 2.1 AA (SC-006); per-principal isolation (FR-012);
recording never blocks or fails the primary security operation, and a dropped recording is
observable as an operational error (FR-016a)
**Scale/Scope**: per-user scope only (no admin/tenant view); ~8 event categories; one new SPA
route + detail child route; one new design-system primitive; ~5 new frontend hooks/components
groups; new backend bounded context + 3 endpoints + 1 migration; **Phase 0** touches
`tokenexchange` (denial reasons + success hook) and `approval` (`IsNew` guard)

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Verified against [.specify/memory/constitution.md](../../.specify/memory/constitution.md) v1.9.1.

**Planning Preconditions**:

- [x] **Domain Model**: `ActivityEvent` aggregate, `ActivityThread`/`NeedsAttentionItem` (derived),
  and value objects (`ActorRef`, `SubjectRef`, `ActivityDetail`, `RelatedRefs`) documented in
  [data-model.md](./data-model.md).
- [ ] **Domain Concepts**: Add Activity Event, Activity Category, Actor, Affected Subject,
  Activity Thread, Needs-Attention Item, and Exploration Context to the `ARCHITECTURE.md` glossary
  in the implementation PR. The names are defined in the spec and data model.
- [ ] **Entity IDs**: Add `ActivityEventID` to `internal/domain/id/gen_ids.go`. Regenerate
  `uuid_ids_gen.go` and update `internal/domain/id/AGENTS.md` as ADR 013 requires.
- [x] **Configuration Design**: `activity` config section (`retention_window` default `720h`,
  `page_size`, `rollup_bucket`, `queue_size`, `prune_interval`, `prune_batch`,
  `attention_poll_interval_seconds`) in `internal/ports/config.go` with YAML examples (research
  Decision 8).
- [ ] **Config Artifacts**: Add `examples/config/activity.yaml`. Update
  `docs/configuration.md`, the configuration example index, and the Helm values, ConfigMap, and
  README for `activity.*` settings.
- [x] **API Design First**: The OpenAPI fragment is authored in
  [contracts/enduser-activity.openapi.yaml](./contracts/enduser-activity.openapi.yaml). Merge it
  into `api/enduser/openapi.yaml` only during implementation.
- [ ] **API Documentation**: Add end-user API examples under `docs/api/` during implementation.
- [ ] **API Changes**: the three new endpoints require stakeholder confirmation in PR review
  (Principle X) — **pending sign-off**; contract is the confirmation artifact.
- [ ] **Database Implementation**: Create PostgreSQL migration
  `migrations/032_create_activity_events.{up,down}.sql` with the §10 indexes, the
  `UNIQUE (principal, dedup_key)` constraint, and `CREATE EXTENSION IF NOT EXISTS pg_trgm`
  (verify the managed-PostgreSQL offering allows it; fall back to `ILIKE` on `summary` if not).
  Add `user_sessions.token_revision` and `user_sessions.reconnect_required_at` in the same
  migration. Do not add a DynamoDB application table.
- [ ] **E2E Acceptance Tests**: Add one `It()` per spec scenario in
  `tests/e2e/activity_test.go`, using the mapping in Testing Strategy.
- [x] **E2E Test Mapping**: 1:1 scenario→`It()` mapping table populated in Testing Strategy.
- [ ] **E2E Red Phase**: Add tests that compile and fail semantically against the contract before
  implementation.
- [ ] **Frontend Playwright E2E**: Add `tests/e2e/frontend/activity_test.go` and the
  `tests/e2e/pages/activity_page.go` page object.
- [ ] **Frontend Screenshots**: Capture the listed states to `tests/e2e/screenshots/` after the
  frontend E2E suite is implemented.

**Implementation Considerations**:

- [x] **Security Design**: The plan is read-only and per-principal. It excludes raw resource
  URIs from activity events and browser DTOs, and projects only approved related identifiers.
  The approved-field registry shapes display text and context. Every `target_route` uses a
  server-side allowlist, and recording does not change fail-closed security operations.
  `correlation_id` stays server-side. Public local token failures without a verified user
  remain in operational audit logs, not the per-user feed.
- [ ] **Architecture Documentation**: Add the Activity glossary terms and bounded-context description to
  `ARCHITECTURE.md` in the implementation PR. Document the recorder seam. Link to ADR 037 when that ADR is accepted.
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

**Post-design Result**: PASS (re-checked 2026-09-22). The design artifacts, OpenAPI fragment, and
proposed ADR 037 meet the planning gate. The unchecked items are implementation work, not
completed evidence. PostgreSQL follows ADR 004. DynamoDB requires a superseding ADR and new
application infrastructure. API stakeholder sign-off remains a review-time gate before
implementation and must be requested against the **revised** contract.

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
- **Filters in URL**: exploration state uses
  `?agent_id=&service_id=&grant_id=&window=&before=&outcome=&category=&q=&needs_attention=` via
  `useSearchParams`. The names match the OpenAPI contract and make per-agent/service views
  linkable from other pages (US3). `cursor` is never in the URL.
- **Outbound links only for actions**: needs-attention next steps and back-links navigate to the
  existing `/sessions`, `/approvals`, `/approvals/:id`, `/agents/:agentId` routes via the server
  allowlist (data-model §3.3). The session page must add a "Reconnect" action for the matching
  `/api/activity/attention` service. This action starts its existing OAuth authorize endpoint;
  the activity experience adds no mutation of its own.
- **Inbound links**: `AgentGrantDetailPage` and `ThirdPartySessionsPage` get a "View activity"
  link to `/activity?agent_id=…` / `/activity?service_id=…` (one line each; makes SC-002's
  two-interaction path real from where users already are).

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
   needs-attention toggle (only the unresolved approval/session transitions are shown in the
   retained feed), and a clear-all control. Agent/service/grant are **facets** rendered as
   removable chips from the current page's `related_refs` labels plus **click-to-filter** on any
   card's actor/subject label (adds `agent_id=`/`service_id=`). Active filters render as removable
   chips above the feed with an `aria-live="polite"` result count ("12 events match"). All synced
   to the URL (US3, FR-006).
3. **Activity feed** — reverse-chronological, grouped by day in the browser timezone (Today /
   Yesterday / date) as `<section aria-labelledby>` with an `<h2>` date heading. Each item is a
   narrative **ActivityCard** (`Card as="article"` + `OutcomeStatus` + humanized summary answering
   what/when/who/what-affected/outcome). Roll-up cards show broker access issuances and the
   local time span ("Broker issued GitHub access for Research Agent · 41 times, 09:00–10:00").
   Cards for `agent_access` / `policy_decision` carry a secondary link "Don't recognise this? Review this agent's access" →
   `/agents/:id` (where revoke already lives; zero new mutation surface). Not a table (FR-007).
   Incremental "Load more" via `next_cursor` (SC-005, FR-013). A footer line explains the cliff:
   "Activity older than {retention_days} days isn't kept."
4. **Threads** — related events (service lifecycle, agent journey, tool-call flow) render with the
   new `Timeline` primitive, **collapsed by default** behind an `Accordion` whose summary carries
   the count ("GitHub connection · 6 events"); `truncated` threads show "N earlier events → open
   thread" which navigates to `/activity?service_id=…` (US5).
5. **Empty/no-match states** — `EmptyState` "no activity yet" with a concrete pointer ("Delegate
   access to an agent to see its activity here → Agent Delegations") and "no matches for these
   filters" with a reset action (FR-014, SC-009).
   With `needs_attention=true`, an empty retained feed does not hide live attention items;
   explain that their history is unavailable while keeping their next steps visible above.

**Detail view (`/activity/:eventId`)** — full context (actor, subject, timing, outcome), the
structured **why** rendered as three lines (*trigger* → *basis* → *consequence*; the basis is the
FR-008 back-link), approved context fields (e.g. scope delta for `grant.updated`, `argument_count`
for tool calls), the bounded related **sequence** as a `Timeline` with "N more" when
`sequence_truncated`, redacted values shown as shape ("3 arguments, redacted"), honest pending
state, and single-action links back to related consent/session/agent context (US4,
FR-005/008/009/011). An event older than retention returns 404, even before prune removes it;
its related sequence also excludes events beyond retention.

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
| `GET` | `/api/activity` | Cursor-paginated, filterable curated feed (`agent_id`, `service_id`, `grant_id`, `window`, `before`, `outcome`, `category`, `q`, `needs_attention`) + read-derived threads + `retention_days` |
| `GET` | `/api/activity/attention` | Live-derived needs-attention items (`group_key`, oldest first), unaffected by historical retention, `Cache-Control: no-store`, polled |
| `GET` | `/api/activity/{event-id}` | Retained single-event detail: structured explanation, approved context, bounded retained sequence (`sequence_limit`), redacted, back-links; 404 beyond retention |

Conventions match the existing API (`{data}` envelope, `{error,message}`, RFC3339,
`X-Remote-User`). This introduces the **first cursor-pagination pattern** in the enduser API
(documented for reuse); the cursor is bound to the filter set and a mismatch is `400
invalid_cursor`. `/api/activity/attention` is registered **before** `/api/activity/{event-id}`
and a routing test asserts precedence. All responses are per-principal scoped and pre-redacted
server-side. The browser DTO omits raw resource URIs, even in `related_refs` and related
sequences; `detail.pending` is not returned because `outcome=pending` supplies the state.
The change **requires stakeholder confirmation** (Principle X) before implementation.

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
- **Backend**: new bounded context
  `internal/domain/activity/{event.go,service.go,recorder.go,safefields.go,templates.go,threads.go,prune.go}`;
  `ActivityEventRepository` in `internal/ports/storage.go`; memory and PostgreSQL adapters in
  `internal/adapters/storage/{memory,postgres}/activity_events.go`; principal/keyset, unique
  dedup, selective `related_refs` expression, trigram, and prune indexes in migration `032`;
  the same migration adds `UserSession.token_revision`, `reconnect_required_at`, and due-time
  indexes for sessions and pending approvals. An expiry-read port supplies bounded due queries
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
| **Phase 0** | Pre-implementation refactoring — **seam preparation** | **Include** — typed `DenialReason` on token-exchange errors; success hook on `TokenExchangeService.Exchange`; pin `IsNew` semantics of approval create with a test; nav active-link prefix match |
| **Phase 1** | Setup — typed ID, config section, dependencies | Yes |
| **Phase 2** | Design Preconditions (domain, config, API, DB, E2E tests) | **MANDATORY** |
| **Phase 2.7** | Entity Boilerplate — empty repo/handler (501) for `ActivityEvent` | **Include** — one new entity + port + adapters + handler; isolates skeleton PR |
| **Phase 2.5** | Foundational Infra — `ActivityRecorder` seam + `Timeline` primitive + SPA scaffolding | Yes |
| **Phase 3+** | User Stories US1–US5 by priority (P1: US1,US2; P2: US3,US4; P3: US5) | Yes |
| **Phase N** | Constitution Compliance verification | **MANDATORY** |

- [x] Phase 0 (refactoring): **include** — the review found the assumed seams for denial reasons
  and RFC 8693 success do not exist; they are small, behaviour-preserving changes to
  `tokenexchange` that must land (with tests) before any recorder call can be wired.
- [x] Phase 2.7 (entity boilerplate): **include** — introduces the `ActivityEvent` entity with new
  port, memory + postgres adapters, and HTTP handler; a scaffold-only PR (repository stubs,
  handler returning 501, migration) keeps the business-logic PRs reviewable.

## Testing Strategy

### Backend E2E (Ginkgo/Gomega) — 1:1 spec mapping

**Location**: `tests/e2e/activity_test.go` · **Bootstrap**: `tests/e2e/bootstrap/` dual-server,
`enduserServer.AuthenticatedGET('/api/activity', principal)` · **Fixtures**: extend
`tests/e2e/fixtures/` with seeded activity events (grants, sessions, approvals, denials, a
sensitive-payload event, a service lifecycle, a multi-agent day, a since-deleted reference, and a
second principal for isolation).

| Spec scenario | `It()` (in `tests/e2e/activity_test.go`) |
|---|---|
| US1 #1 recency + fields | events ordered by recency, each with what/when/who/subject/outcome |
| US1 #2 human-readable | technical op rendered jargon-free |
| US1 #3 outcome not color-alone | outcome exposed as label+icon+text in payload |
| US1 #4 empty state | no events → `{events:[],threads:[],next_cursor:null,retention_days:N}` |
| US1 #5 revoked reference | since-deleted actor renders with historical label |
| US1 #6 top-level reachability | (frontend) Activity link visible and single-action on desktop, narrow mobile, and 200% zoom — Playwright |
| US1 #7 one canonical event | same transition reported twice → one event; two grant updates within one second and two reconnections on one session → distinct events; roll-ups count broker issuances, not downstream uses |
| US2 #1–#6 needs-attention | attention list present/settled; clears after resolve; blocked stays history-only; rejected refresh or no usable refresh token yields `reconnect_required` even if its history event is dropped or pruned; `/sessions` offers OAuth reconnection despite `is_expired=false` |
| US3 #1–#7 exploration | each filter (incl. `q`, `before`, `30d`) + combination + clear + no-match; pending approval with a shared tool name selects only its own `approval.requested`, and a reconnect selects only the current session revision's expiry/failure, not older service history; cursor with changed filters → `400 invalid_cursor` |
| US4 #1–#5 detail | structured explanation, bounded sequence + `sequence_truncated`, back-links, redaction via SafeFields (unknown context key is stripped), pending derived from `outcome`; feed/detail/related sequence never serialize a raw resource URI with embedded credentials or query secrets; 404 for an owned event past retention while pruning is pending; retained sequence excludes old events |
| US5 #1–#4 threads | service lifecycle + agent journey threads derived on read; idle expiry before reconnect without exchange between worker ticks; pending approval expiry without detail access; repeated observations yield one event per session revision/approval ID; approval event in tool_flow **and** agent_journey; `truncated` across pages; high volume scannable; drill-down |
| FR-016a operational error | recorder with a failing repository: primary op succeeds, `activity_events_dropped_total` increments, ERROR log emitted |
| FR-012 unaffiliated token failure | client-credentials and invalid-code failures without a verified principal remain operational logs and produce no user activity event |

**Red phase**: assertions target concrete contract values (status codes, `data.events[…].summary`,
`redacted:true`, `next_step.target_route`), compile, and fail semantically before implementation.
No `XIt`/`Skip`, no red-phase comments.

### Frontend Playwright E2E + Screenshots

**Location**: `tests/e2e/frontend/activity_test.go`, page object `tests/e2e/pages/activity_page.go`
· seeding via direct `GetTestStorage()` repositories · auth via `X-Remote-User` in the browser
context. Screenshots gated by `E2E_CAPTURE_SCREENSHOTS`, saved to `tests/e2e/screenshots/`.

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

### Unit & Integration

- **Unit** (TDD, red first): `internal/domain/activity/*_test.go` — feed assembly, read-time
  threading, live needs-attention derivation, SafeFields stripping (every `type` has a registry
  entry; no deny-listed key), dedup-key derivation per type, roll-up bucketing, cursor encoding +
  filter binding, wording templates, route allowlist. Table-driven. `tokenexchange` Phase 0:
  every failure path yields the expected `DenialReason`.
- **Integration** (testcontainers PostgreSQL): `internal/adapters/storage/postgres/activity_events_test.go`
  — record (idempotent upsert incl. concurrent roll-up increments), list, related, prune under
  `pg_try_advisory_lock` with two competing connections, `032` migration apply + rollback +
  repeat, principal/keyset ordering, selective `related_refs` filters, trigram `q`, and
  cross-principal isolation. Also prove that an unpruned expired event returns 404 by ID and
  cannot enter the related sequence. A session refresh rejection must persist its marker even
  when the history insert fails, and a later token write must clear it without a stale retry.
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
- **`pg_trgm` extension** — the only way to make SC-005 ("find a specific recent event in 30 s
  among 10,000") honest without a full search stack; an `ILIKE` fallback is specified.
- **Async recorder goroutine + advisory-lock prune** — the app has no scheduler; this is the
  smallest coordination that is multi-replica safe and needs no new infrastructure.
- **Phase 0 in `tokenexchange`** — unavoidable: the seams the feature needs do not exist.
- **Polling `/attention`** — the SPA has never polled, but a pending approval expires in minutes
  and an on-load-only band would routinely offer a dead next step.
