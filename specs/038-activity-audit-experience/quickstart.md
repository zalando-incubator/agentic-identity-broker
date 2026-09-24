# Quickstart: End-User Activity & Auditing Experience

**Feature**: 038-activity-audit-experience
**Purpose**: runnable validation scenarios proving the feature works end to end. Details of the
contract and data live in [contracts/](./contracts/) and [data-model.md](./data-model.md); this is
a run/verify guide, not an implementation guide.

## Prerequisites

- Go 1.27.1 toolchain (as in `go.mod`), Node/pnpm for the SPA, `just` runner.
- Docker/Podman for PostgreSQL-backed integration tests (memory backend needs nothing).
- Playwright browser deps for the `playwright-go` frontend suite (installed by the frontend E2E
  BeforeSuite; screenshots require `E2E_CAPTURE_SCREENSHOTS=true`).

## Build & run locally

```bash
just build-all                              # backend binary + SPA bundle
./bin/agentic-identity-broker --config ./examples/config/activity.yaml
# SPA is root-mounted; open the new destination:
#   http://localhost:8000/activity
```

The reverse proxy (or Vite dev proxy) injects `X-Remote-User`; in dev it is `dev@example.com`.

## Static checks & fast tests

```bash
just check          # fmt -> vet -> lint (Go) + jsx-a11y lint (SPA)
just test           # fast Go/package tests (activity domain + adapters unit tests)
```

## Backend acceptance (Ginkgo E2E) — seven assigned story keys plus supplemental API tests

```bash
ginkgo -v ./tests/e2e/ --focus "Activity & Auditing"
```

The 1:1 mapping spans both focused suites: seven backend and 22 frontend numbered `It()`
cases, as assigned in the plan. Backend API tests for filter intersection, invalid cursor,
principal isolation, approved fields, read-time retention, FR-014, independent action audits,
and unaffiliated-token handling are supplemental; they do not add story keys.

Validates, per the enduser API contract, with a seeded principal:
- **US1**: `GET /api/activity` returns events with `summary`, `occurred_at`, `actor`, `subject`,
  `outcome`, `occurrence_count`, and `related_links`; jargon-free summaries; an empty feed
  returns `{data:{events:[],threads:[],next_cursor:null,retention_cutoff_at:"2026-08-05T12:00:00Z"}}`
  at request time `2026-09-04T12:00:00Z` with `720h`. A reported grant update
  recorded twice yields one event, while two updates within one second yield two events. Two
  OAuth callbacks on one session produce distinct connection/reconnection events. Forty-one
  successful broker token exchanges in one bucket yield one `agent.access_issued` event with
  `occurrence_count: 41`. They do not prove 41 downstream service uses.
- **US2**: `GET /api/activity/attention` returns live items with an allowlisted
  `next_step.target_route` (`/sessions`, `/approvals/{id}`). A rejected refresh or absent usable
  refresh token sets the session marker even without refresh expiry. The alert persists if its
  activity event is dropped or pruned. It clears after a successful refresh or reconnection;
  approval attention clears after the decision.
- **US3**: `agent_id`, `service_id`, `grant_id`, `window`, `before`, `outcome`, `category`,
  `q`, and `needs_attention` intersect on the server. A pending approval matches only its own
  `approval.requested` event; a reconnect matches its current `session_id` and token revision,
  not old service history. Missing or pruned transitions do not hide live attention.
  From an Activity card with approved `related_refs.grant_id`, choose “View this grant's activity”
  to set the exact `grant_id`; omit this action without that reference. From
  `/agents/{agent_id}`, a separate grant link appears only when `useAgentGrants` returns a
  `UserGrant`; it uses `grants.id`, not `agent_id`. Add a second filter, remove the grant chip,
  and clear all. A cursor with changed filters returns `400 invalid_cursor`.
- **US4**: `GET /api/activity/{event-id}` returns `detail.explanation.{trigger,basis,consequence}`,
  a bounded `sequence` with `sequence_truncated`, and top-level `data.related_links` inherited
  from `ActivityEvent`. A deleted or cross-principal target yields `related_links: []` and
  keeps historical text visible. Derive pending state from `outcome=pending`, not `detail.pending`.
  An owned event outside retention returns 404 even before pruning; related sequences exclude
  old events. Seed a protected-resource URI with credentials and a `?access_token=SECRET` query.
  Neither the feed,
  detail, nor related sequence response contains the raw URI or `SECRET`. A seeded sensitive
  event returns `redacted:true` without the clear-text value. Unregistered `context` keys and
  sensitive arguments are absent from all browser response fields (SC-007).
- **US5**: a service seeded with connect→refresh→expire→reconnect returns a single
  `service_lifecycle` thread. Let its session expire without a token exchange, then reconnect
  before the next periodic sweep; the history still includes expiry before reconnection.
  Let another session and a pending approval expire while idle; the periodic worker records both
  without token exchange or approval-detail access. Repeated scans create one expiry event per
  session revision or approval ID. Agent broker-access milestones appear in `agent_journey`;
  an approval event appears in both `tool_flow` and `agent_journey`; a thread spanning two pages
  is `truncated: true` with the correct `total_events`.
- **FR-016a**: with the event repository failing after a successful grant write, the grant
  remains committed and `activity_events_dropped_total` increments with an `ERROR` log.
- **FR-015**: an event whose agent/service was deleted still renders with its historical
  `display_label`.
- **FR-012**: a second principal's events are never returned to the first.
- **Principal boundary**: local `client_credentials` and invalid-code token failures without
  a verified user remain operational audit logs; they create no user activity event.

## Frontend acceptance (Playwright via `playwright-go`) + screenshots

```bash
E2E_CAPTURE_SCREENSHOTS=true ginkgo -v ./tests/e2e/frontend/ --focus "Activity"
# screenshots -> coverage/screenshots -> synced to tests/e2e/screenshots/
```

Exercises the `/activity` UI (page object `tests/e2e/pages/activity_page.go`) and captures the
state list in the plan. Verifies a visible, single-action Activity link on desktop, narrow
mobile, and at 200% zoom (SC-010); Activity remains active on `/activity/:eventId`.
The needs-attention band shows at most three visible items, groups related items, polls for
changes, and links to `/sessions` or `/approvals/:id`. A marked session on `/sessions` offers
"Reconnect" despite `is_expired=false`. A card with an approved grant reference offers the
exact grant activity link. The agent detail page keeps its agent activity link and adds a
separate `grant_id=grants.id` link when a grant exists. Filters preserve the removable grant
chip while another filter is added; clear-all restores the unfiltered feed. The broker-access
card shows successful exchange count and a local time span. Detail shows a three-line
explanation, bounded sequence, and
redacted data. A safe `ActivityEvent.related_links` route appears directly on a card and in its
detail; a missing target leaves historical text without a link. Collapsed threads expose
"N earlier events". After a valid feed loads, show "Activity before {local date and time} is not
shown." below history on populated, paginating, empty, filtered no-match, and detail-child views.
An unfiltered empty feed says "No activity in the period shown. New activity will appear here."
Never put the history boundary on the live attention band.

## Accessibility validation (WCAG 2.1 AA — SC-006)

Run inside the same frontend suite (`--focus "Activity accessibility"`):
- Keyboard-only traversal of nav, filters, tabs, accordion, timeline, load-more, detail links.
- `axe-core` injected on the main states asserts zero critical/serious violations.
- Outcome conveyed by icon + text (not color alone); reduced-motion suppresses animation.
- At narrow widths and 200% zoom, the Activity link and content remain visible and keyboard
  operable without a menu opener, horizontal page scroll, or clipping.

## Interaction proof (SC-002/004/008/010)

Count a click, Enter/Space activation, menu opening, or completed query submission as one
interaction. Passive rendering does not count. Record the starting URL, the action sequence,
the resulting exact filter/target, and the count with the Playwright page object.

- **SC-002**: Seed an agent and a connected service whose activity is absent from page one.
  Start at `/agents/{agent_id}` for the agent and `/sessions` for the service. Follow the
  existing-page "View activity" link to set exact `agent_id` or `service_id`; check all retained
  results, including later pages. Start relative-window and outcome tasks at `/activity`.
  Reach each result within two activations. Keyword search cannot prove off-page completeness.
- **SC-004**: Start from a live attention item. Reach the existing approval or reconnect
  resolving flow within two activations.
- **SC-008**: Start at a feed card with a server-approved `ActivityEvent.related_links` target.
  Follow the first safe route directly in one activation. Do not count an extra click to open
  detail as success. Check US4 #3's detail link separately. A deleted target shows historical
  text without a context link; the client guards both views with `inAppRoute.ts`.
- **SC-010**: Start from another end-user route. Open the primary Activity link in one
  activation on desktop, mobile, and 200% zoom without first opening a menu.

## Exploratory storage benchmark (not a feature acceptance SLA)

Run this only against an isolated PostgreSQL environment with capacity beyond the raw stress
payload; it is intentionally outside normal CI and does not change production sizing. After the
planned repository benchmark is added, run:

```bash
go test -run '^$' -bench '^BenchmarkActivityEventRepository$' -benchmem \
  ./internal/adapters/storage/postgres/
```

Use [research.md](./research.md) Decision 2 and [data-model.md](./data-model.md) §10 for the
workload and storage design.

- Seed a single principal with 10,000 *distinct* curated events (distinct `dedup_key`s) to
  exercise the SC-005 experience, and separately drive 330 writes/s at a handful of roll-up keys
  to measure upsert contention.
- Separately measure 2,000 principals × 10,000 retained 3-KiB events (20 million total), with a
  330 significant-event-writes/s burst. This is an exploratory stress profile, not a product
  load, quota, or MCP request-rate assumption.
- Capture `EXPLAIN (ANALYZE, BUFFERS)` plus p50/p95/p99 for the unfiltered feed, each filter,
  representative combined filters, and threads on a warm isolated PostgreSQL database. SC-005
  remains the under-30-second user task; the benchmark does not establish an API latency SLO.
- Record `pg_total_relation_size` and index/WAL headroom. Do not treat the 55.9-GiB raw payload
  estimate as a deployment requirement; it excludes those physical costs.

## Opt-in human outcome study (SC-001/003/005)

Run this only after T018 fixtures, T119's standalone driver, the Activity app, and the built
SPA exist. Have Docker or Podman available. For each of ten distinct participants, from the
repository root:

```bash
just web-build
go run ./tests/e2e/study/activity/
# Wait for: Activity study ready: <URL>/activity
# After the participant's trials, press Ctrl-C to stop the driver.
```

The driver opens headed Chromium at 1280×800 with a fresh context for `user@example.com`.
It seeds **exactly 10,000 distinct retained events** in a new PostgreSQL 15 database using
production migrations, with every reviewed emitted type and a uniquely named rank-250 target.
It prints readiness only after a fixture card appears. A new launch gives the next participant
a new migrated database and browser context. Use the same browser build and viewport for all
ten. Agents must launch this long-running driver with `hub` `op:"start"`, wait for the
`Activity study ready:` log, and end each run with `hub` `op:"stop"`. The driver cleans up on
SIGINT and SIGTERM, including `PostgresFixture.Close` (Ryuk disabled). Any setup, seed, or
browser error exits nonzero.

Start the stopwatch after the fixture card shows. Give each participant at least two **different**
event types, round-robin until every emitted type has a no-detail, timed trial. For SC-001,
every trial must answer the fixture's approved what/when/who/affected/outcome rubric within
10 seconds. For SC-003, give each participant one preselected paraphrase prompt; at least
9/10 must describe it correctly without jargon. For SC-005, give each participant one separate
rank-250 target task; all ten must find it among the 10,000 events in under 30 seconds.
Record anonymous answers, elapsed time, activations, emitted-type coverage, browser build,
fixture count, and failures in `plan.md` verification evidence. These three denominators
are trials, participants, and ten target tasks respectively. If participants or a container
runtime are unavailable, leave outcomes unverified. T120's database benchmark cannot replace
a human result. If T016 selected `ILIKE`, use its migrated production fallback schema; record
its 10,000-row query plan separately in T120. Do not use a faster `pg_trgm` run to claim a
failed `ILIKE` user task.

## Manual smoke walkthrough

1. Open `http://localhost:8000/activity` from the primary Activity link in one activation,
   including on a narrow viewport and at 200% zoom.
2. Confirm the band shows expired or reconnect-required sessions and pending approvals with next steps.
   With none, it shows "nothing needs your attention". Approve a pending request in another tab.
   Confirm the band clears within ~30 s without reload.
   Follow a reconnect item to `/sessions` and start its OAuth authorization flow. After the
   callback, confirm the attention item clears without waiting for history to be pruned.
3. Scan the grouped feed: each card states what/when/who/what-affected/outcome without opening it;
   broker-issued agent access shows one card with "· N times, HH:MM–HH:MM", not N use claims.
4. Seed two grants and select a card with approved `related_refs.grant_id`. Choose “View this
   grant's activity”; confirm `grant_id=` uses that grant's ID. Add `blocked`, remove only the
   grant chip, then clear all. Confirm a card without a grant reference has no grant action.
   At `/agents/{agent_id}`, compare the agent activity link with the separate grant activity
   link (`grants.id`). Confirm no grant link appears when `useAgentGrants` returns no grant.
5. Open an event with sensitive data; confirm no secret/token is shown (arguments appear as
   "N arguments, redacted"), the explanation reads trigger → basis → consequence, and the basis
   link lands on the right agent/approval/session page.
6. Open a connected service with a lifecycle; confirm the collapsed thread expands into one
   coherent timeline and "N earlier events" opens the filtered feed.
7. Scroll to the end of the feed; confirm the footer explains where history ends.
8. For SC-002, seed an agent and a connected service whose retained events are absent from page
   one. Start at `/agents/{agent_id}` for the agent and `/sessions` for the service. Use each
   existing-page "View activity" link to set the exact `agent_id` or `service_id`; confirm all
   matches across later pages. Start the window and outcome tasks at `/activity` and reach the
   result with existing controls in at most two activations. Do not rely on keyword search.

## Done when

- `just check` and `just test` pass.
- Both `ginkgo ./tests/e2e/ --focus "Activity & Auditing"` and
  `ginkgo ./tests/e2e/frontend/ --focus "Activity"` are green with **29 unique numbered
  scenario keys** (7 backend, 22 frontend). Screenshots are written. Supplemental
  contract/security checks and the direct-card SC-008 `It()` do not count toward 29.
- Accessibility checks pass; no sensitive value appears in any API response or rendered view.
