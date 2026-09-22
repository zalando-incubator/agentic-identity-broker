# Activity API contract (Phase 1)

**Feature**: 038-activity-audit-experience

Files in this directory define the interface contracts the feature exposes to the SPA:

- [`enduser-activity.openapi.yaml`](./enduser-activity.openapi.yaml) — OpenAPI 3.0.3 fragment
  for the three new end-user endpoints. These paths/schemas are to be **merged into**
  `api/enduser/openapi.yaml` during implementation (Principle IV/X). They follow the existing
  enduser conventions: `{data: ...}` success envelope, `{error, message}` errors, RFC3339
  timestamps, `X-Remote-User` principal, Zalando resource-oriented design.
- [`frontend-types.md`](./frontend-types.md) — the hand-written TypeScript contract
  (`web/src/types/activity.ts`) the SPA hooks/services consume, kept in sync with the OpenAPI
  schemas (there is no generated client).

**Stakeholder confirmation gate**: per Constitution Principle X, the endpoints below MUST be
confirmed in PR review before implementation begins. This contract is the confirmation artifact.

## Endpoint summary

| Method | Path | Purpose | Spec refs |
|---|---|---|---|
| `GET` | `/api/activity` | Paginated (cursor bound to filters), filterable (`agent_id`, `service_id`, `grant_id`, `window`, `before`, `outcome`, `category`, `q`, `needs_attention`) curated feed with roll-up counts + read-derived threads + `retention_days` | US1, US3, US5; FR-001/002/006/007/013/016b |
| `GET` | `/api/activity/attention` | Live-derived needs-attention items (grouped, oldest first) with allowlisted next-step routes; polled by the SPA | US2; FR-003/004 |
| `GET` | `/api/activity/{event-id}` | Single event detail: structured explanation, approved context, bounded related sequence, related-context links, redacted | US4; FR-005/008/009/011 |

Revised 2026-09-22 after the plan review; see `plan.md` "Revision 2026-09-22" for the change list.

All endpoints are per-principal scoped (FR-012) and read-only (no mutation surface).
