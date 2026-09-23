# Quickstart: End-User Activity & Auditing Experience

**Feature**: 038-activity-audit-experience
**Purpose**: runnable validation scenarios proving the feature works end to end. Details of the
contract and data live in [contracts/](./contracts/) and [data-model.md](./data-model.md); this is
a run/verify guide, not an implementation guide.

## Prerequisites

- Go 1.26.8 toolchain, Node/pnpm for the SPA, `just` runner.
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

## Backend acceptance (Ginkgo E2E) — maps 1:1 to spec scenarios

```bash
ginkgo -v ./tests/e2e/ --focus "Activity & Auditing"
```

Validates, per the enduser API contract, with a seeded principal:
- **US1**: `GET /api/activity` returns events with `summary`, `occurred_at`, `actor`, `subject`,
  `outcome`, `occurrence_count`; jargon-free summaries; empty state returns
  `{data:{events:[],threads:[],next_cursor:null,retention_days:30}}`. A reported grant update
  recorded twice yields one event, while two updates within one second yield two events. Two
  OAuth callbacks on one session produce distinct connection/reconnection events. Forty-one
  successful broker token exchanges in one bucket yield one `agent.access_issued` event with
  `occurrence_count: 41`. They do not prove 41 downstream service uses.
- **US2**: `GET /api/activity/attention` returns live items with an allowlisted
  `next_step.target_route` (`/sessions`, `/approvals/{id}`). A rejected refresh or absent usable
  refresh token sets the session marker even without refresh expiry. The alert persists if its
  activity event is dropped or pruned. It clears after a successful refresh or reconnection;
  approval attention clears after the decision.
- **US3**: filter params (`agent_id`, `service_id`, `grant_id`, `window` incl. `30d`, `before`,
  `outcome`, `category`, `q`, `needs_attention`) narrow results; combined filters intersect.
  A pending approval matches only its own `approval.requested` event, even when another approval
  has the same tool name. A reconnect matches expiry/failure for its current session revision,
  not earlier events for the same service. If history was pruned, the live attention item remains
  on `/api/activity/attention` without adding unrelated events to the filtered feed. Clearing
  filters restores the full feed; a cursor reused with different filters returns `400 invalid_cursor`.
- **US4**: `GET /api/activity/{event-id}` returns `detail.explanation.{trigger,basis,consequence}`,
  a bounded `sequence` with `sequence_truncated`, and `detail.related_links`. Derive pending
  state from `outcome=pending`, not `detail.pending`. An owned event outside retention returns
  404 even if its row awaits pruning; a retained event's sequence excludes old events. Seed a
  protected resource URI with credentials and a `?access_token=SECRET` query. Neither the feed,
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
"Reconnect" despite `is_expired=false`. Filters narrow the feed through controls or a card label,
show active chips, and clear correctly. The broker-access card shows a count of successful
exchanges and a local time span. Detail shows a three-line explanation, bounded sequence, and
redacted data. Collapsed threads expose "N earlier events". The retention footer and the empty
and no-match states remain visible when needed.

## Accessibility validation (WCAG 2.1 AA — SC-006)

Run inside the same frontend suite (`--focus "Activity accessibility"`):
- Keyboard-only traversal of nav, filters, tabs, accordion, timeline, load-more, detail links.
- `axe-core` injected on the main states asserts zero critical/serious violations.
- Outcome conveyed by icon + text (not color alone); reduced-motion suppresses animation.
- At narrow widths and 200% zoom, the Activity link and content remain visible and keyboard
  operable without a menu opener, horizontal page scroll, or clipping.

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
4. Click an agent's name on a card; confirm the feed narrows, a removable chip appears, and the
   URL carries `agent_id=`. Add `blocked`; type a keyword; clear all.
5. Open an event with sensitive data; confirm no secret/token is shown (arguments appear as
   "N arguments, redacted"), the explanation reads trigger → basis → consequence, and the basis
   link lands on the right agent/approval/session page.
6. Open a connected service with a lifecycle; confirm the collapsed thread expands into one
   coherent timeline and "N earlier events" opens the filtered feed.
7. Scroll to the end of the feed; confirm the footer explains where history ends.

## Done when

- `just check` and `just test` pass.
- `ginkgo ./tests/e2e/ --focus "Activity & Auditing"` is green (all spec scenarios).
- `ginkgo ./tests/e2e/frontend/ --focus "Activity"` is green and screenshots are written.
- Accessibility checks pass; no sensitive value appears in any API response or rendered view.
