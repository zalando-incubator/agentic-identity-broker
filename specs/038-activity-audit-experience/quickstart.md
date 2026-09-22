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
  `{data:{events:[],threads:[],next_cursor:null,retention_days:30}}`; the same action recorded
  twice (replayed exchange, repeated lazy expiry, idempotent approval create) yields one event
  (US1 #7); 41 exchanges within one bucket yield one `agent.acted_via_service` event with
  `occurrence_count: 41`.
- **US2**: `GET /api/activity/attention` returns live items with an allowlisted
  `next_step.target_route` (`/sessions`, `/approvals/{id}`); a refresh failure (not only refresh
  expiry) yields `reconnect_required`; an unresolved item remains visible even if its historical
  event has aged out of retention, then clears after the underlying session is reconnected or
  approval decided (re-query returns empty).
- **US3**: filter params (`agent_id`, `service_id`, `grant_id`, `window` incl. `30d`, `before`,
  `outcome`, `category`, `q`, `needs_attention`) narrow results; combined filters intersect; a
  no-match combination returns an empty page; clearing filters restores the full feed; a cursor
  reused with different filters returns `400 invalid_cursor`.
- **US4**: `GET /api/activity/{event-id}` returns `detail.explanation.{trigger,basis,consequence}`,
  a bounded `sequence` with `sequence_truncated`, and `detail.related_links`; a seeded event
  carrying sensitive parameters returns `redacted:true` with no clear-text secret anywhere in the
  body, and a seeded event with an unregistered `context` key returns without that key (SC-007).
- **US5**: a service seeded with connect→refresh→expire→reconnect returns a single
  `service_lifecycle` thread; an agent with many actions returns an `agent_journey` thread; an
  approval event appears in both `tool_flow` and `agent_journey`; a thread spanning two pages is
  `truncated: true` with the correct `total_events`.
- **FR-016a**: with the repository failing, the primary operation still succeeds and
  `activity_events_dropped_total` increments.
- **FR-015**: an event whose agent/service was deleted still renders with its historical
  `display_label`.
- **FR-012**: a second principal's events are never returned to the first.

## Frontend acceptance (Playwright via `playwright-go`) + screenshots

```bash
E2E_CAPTURE_SCREENSHOTS=true ginkgo -v ./tests/e2e/frontend/ --focus "Activity"
# screenshots -> coverage/screenshots -> synced to tests/e2e/screenshots/
```

Exercises the `/activity` UI (page object `tests/e2e/pages/activity_page.go`) and captures the
state list in the plan. Verifies: first-class nav destination reachable in one action (SC-010)
and the nav item stays active on `/activity/:eventId`; needs-attention band (max three visible,
grouped, polled) with working next-step links into `/sessions` and `/approvals/:id`; filter
narrowing via controls, active-filter chips, and click-to-filter on a card label, plus clear;
roll-up card with count and local time span; detail panel with three-line explanation, bounded
sequence, and redaction; collapsed timeline threads with "N earlier events"; retention footer;
empty and no-match states.

## Accessibility validation (WCAG 2.1 AA — SC-006)

Run inside the same frontend suite (`--focus "Activity accessibility"`):
- Keyboard-only traversal of nav, filters, tabs, accordion, timeline, load-more, detail links.
- `axe-core` injected on the main states asserts zero critical/serious violations.
- Outcome conveyed by icon + text (not color alone); reduced-motion suppresses animation;
  content and function preserved at 200% zoom.

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
  representative combined filters, and threads. The comparison targets are repository p95 ≤100
  ms and endpoint p95 ≤500 ms on a warm database; SC-005 remains the under-30-second user task.
- Record `pg_total_relation_size` and index/WAL headroom. Do not treat the 55.9-GiB raw payload
  estimate as a deployment requirement; it excludes those physical costs.

## Manual smoke walkthrough

1. Open `http://localhost:8000/activity` from the header nav (single click).
2. Confirm the needs-attention band shows any expired session / pending approval with a next step;
   with none, it shows the settled "nothing needs your attention" state. Approve a pending
   request in another tab and confirm the band clears within ~30 s without a reload.
3. Scan the grouped feed: each card states what/when/who/what-affected/outcome without opening it;
   a busy agent shows one card with "· N times, HH:MM–HH:MM", not N cards.
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
