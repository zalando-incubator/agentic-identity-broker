# Frontend type contract — `web/src/types/activity.ts`

**Feature**: 038-activity-audit-experience · **Revised**: 2026-09-22 (plan review)

The SPA has no generated client; response types are hand-written in `web/src/types/*.ts` and kept
in sync with `api/enduser/openapi.yaml` (see [enduser-activity.openapi.yaml](./enduser-activity.openapi.yaml)).
This file is the source-of-truth shape the activity hooks/services consume. It mirrors the
OpenAPI schemas 1:1.

```ts
// web/src/types/activity.ts

export type Outcome = 'succeeded' | 'failed' | 'blocked' | 'pending';

export type ActivityCategory =
  | 'delegation'
  | 'session'
  | 'agent_access'
  | 'policy_decision'
  | 'approval'
  | 'revocation'
  | 'reconnect'
  | 'failure';


export type ActorKind = 'agent' | 'service' | 'user' | 'broker';
export type SubjectKind = 'grant' | 'permission_set' | 'service' | 'session' | 'tool' | 'resource';

export interface ActorRef {
  kind: ActorKind;
  id?: string | null;
  /** Historical human name captured at event time; renders even if the entity was removed (FR-015). */
  display_label: string;
}

export interface SubjectRef {
  kind: SubjectKind;
  id?: string | null;
  display_label: string;
}

export interface RelatedRefs {
  agent_id?: string | null;
  service_id?: string | null;
  grant_id?: string | null;
  session_id?: string | null;
  approval_id?: string | null;
  client_id?: string | null;
  resource_uri?: string | null;
}

/** Relative in-app route from the server allowlist: /agents/:id, /sessions, /approvals, /approvals/:id, /activity/:id */
export type InAppRoute = `/${string}`;

export interface RelatedLink {
  label: string;
  target_route: InAppRoute;
}

export type BasisKind = 'grant' | 'permission_set' | 'policy' | 'approval' | 'provider' | 'user' | 'broker';

export interface Basis {
  kind: BasisKind;
  id?: string | null;
  label: string;
  target_route?: InAppRoute | null;
}

/** Structured "why" rendered as three lines (FR-009). */
export interface Explanation {
  trigger: string;
  basis: Basis;
  consequence: string;
}

export interface ActivityDetail {
  explanation: Explanation;
  /** Approved key/value context only; never secrets/tokens/sensitive params (FR-011). */
  context?: Record<string, string>;
  related_links?: RelatedLink[];
  pending: boolean;
}

export interface ActivityEvent {
  id: string;
  category: ActivityCategory;
  type: string;
  actor: ActorRef;
  subject: SubjectRef;
  outcome: Outcome;
  summary: string;
  related_refs: RelatedRefs;
  /** >1 for roll-ups; agent.access_issued counts broker token exchanges, not downstream actions. */
  occurrence_count: number;
  first_occurred_at: string; // RFC3339
  occurred_at: string; // RFC3339 (latest occurrence; ordering key)
  redacted: boolean;
}

/** GET /api/activity/:id adds detail + bounded related sequence. */
export interface ActivityEventDetail extends ActivityEvent {
  detail: ActivityDetail;
  sequence: ActivityEvent[];
  sequence_truncated: boolean;
}

export type ThreadKind = 'service_lifecycle' | 'agent_journey' | 'tool_flow';

export interface ActivityThreadResponse {
  thread_key: string;
  kind: ThreadKind;
  title: string;
  latest_outcome: Outcome;
  total_events: number;
  truncated: boolean;
  /** Ordered ascending by occurred_at. The client resolves these IDs with data.events. */
  event_ids: string[];
}

export type AttentionKind = 'reconnect_required' | 'approval_pending';

export interface NextStep {
  label: string;
  target_route: InAppRoute;
}

export interface NeedsAttentionItem {
  kind: AttentionKind;
  title: string;
  summary: string;
  subject: SubjectRef;
  next_step: NextStep;
  since: string; // RFC3339
  group_key: string;
}

// --- Query + response envelopes (mirror {data: ...}) ---

export type TimeWindow = '24h' | '7d' | '30d' | 'all';

export interface ActivityFilters {
  agent_id?: string;
  service_id?: string;
  grant_id?: string;
  window?: TimeWindow;
  before?: string; // RFC3339
  outcome?: Outcome;
  category?: ActivityCategory;
  q?: string;
  needs_attention?: boolean;
  cursor?: string;
  limit?: number;
}

export interface ListActivityResponse {
  data: {
    events: ActivityEvent[];
    threads: ActivityThreadResponse[];
    next_cursor: string | null;
    retention_days: number;
  };
}

export interface GetActivityEventResponse {
  data: ActivityEventDetail;
}

export interface ListAttentionResponse {
  data: { items: NeedsAttentionItem[] };
}
```

## Client wiring (follows existing patterns)

- **Service** `web/src/services/api/activity.ts`: `apiClient.get<ListActivityResponse>('/activity', { params })`,
  `get<GetActivityEventResponse>('/activity/:id', { params: { sequence_limit } })`,
  `get<ListAttentionResponse>('/activity/attention')`.
  Cache the feed and detail GETs via `services/api/cache.ts` with a short TTL; **do not cache**
  `/activity/attention` (it is polled).
- **Hooks** (canonical `useSessions` shape): `useActivity(filters)` (feed + `loadMore` via
  `next_cursor`; a filter change discards the cursor), `useActivityEvent(id)`,
  `useNeedsAttention({ pollMs })` — polls while `document.visibilityState === 'visible'` and
  revalidates on `focus`; the first poll interval in the SPA (research Decision 6).
- **Filter state**: `ActivityFilters` is derived from `useSearchParams()` so the URL is the single
  source of truth (research Decision 4). `cursor` is never written to the URL.
- **Route guard**: a `toInAppRoute(value: string): InAppRoute | null` helper accepts only values
  matching `^/(agents/[^/]+|sessions|approvals(/[^/]+)?|activity/[^/]+)$`; anything else renders
  as plain text, never as a `<Link>`.
- **Day grouping** and relative labels use the browser timezone; roll-up cards render
  `first_occurred_at`–`occurred_at` as a local span ("09:00–10:00").

## Outcome → presentation mapping (app-level, not a new primitive)

`web/src/components/activity/OutcomeStatus.tsx` maps `Outcome` to a design-system
`StatusIndicator` (label + icon + text, never color alone — FR-019):

| Outcome | StatusIndicator variant | icon | label |
|---|---|---|---|
| `succeeded` | `success` | check | "Succeeded" |
| `failed` | `error` | alert-triangle | "Failed" |
| `blocked` | `warning` | shield-x | "Blocked" |
| `pending` | `info` | clock | "Pending" |
