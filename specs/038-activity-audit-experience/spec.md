# Feature Specification: End-User Activity & Auditing Experience

**Feature Branch**: `038-activity-audit-experience`

**Created**: 2026-09-04

**Status**: Draft

**Input**: User description: "Extend the end-user consent and session experience so users can inspect, understand, and trust what the identity broker, connected services, and authorized agents are doing on their behalf. Users need an activity and auditing experience that explains what happened, why it happened, who or what was involved, what access or service was affected, whether the action succeeded, failed, or was blocked, and what they can do next."

## Clarifications

### Session 2026-09-04

- Q: How does a needs-attention item with no resolution path (a blocked/denied action, a completed revocation) leave the needs-attention area? → A: It never enters it — needs-attention is purely derived from live unresolved state (reconnect required, pending approval) and clears automatically; non-actionable notable events are informational history only, with no user dismissal (preserves the read-only assumption).
- Q: What is the committed default retention window for surfaced activity? → A: A 7-day rolling window by default (configurable); activity older than the configured window is not shown.
- Q: Does the experience record every low-level, high-frequency success (each token exchange, each policy allow) or a curated set of significant events? → A: A curated set of business/security-significant events; routine high-frequency internals (individual token exchanges, per-request allow evaluations) are summarized into their significant milestones rather than surfaced individually.
- Q: What form does the time-window filter take (relative presets vs a custom date range)? → A: Relative presets only (for example last 24 hours, last 7 days, all activity within retention); an arbitrary custom start/end date range is not required for the MVP.

### Session 2026-09-09

- Q: Should unresolved needs-attention items remain visible regardless of history retention? → A: Keep unresolved needs-attention items live until resolved, regardless of history retention.

### Session 2026-09-22 (plan review — proposed, pending stakeholder confirmation)

- Q: Is a 7-day default retention sufficient for the trust goal? → A: Proposed default raised to **30 days** (still configurable). Seven days truncates the history of any user who checks in less than weekly and leaves the end of history unexplained; consumer security-activity pages retain 28–180 days. The experience MUST also state where history ends (see FR-014).
- Q: How is "one canonical event per action" (FR-016b) achieved when the same action is reported repeatedly (replica caches, retries, lazy expiry detection)? → A: Each event carries a deterministic idempotency key; duplicate reports are absorbed, never shown.
- Q: How are routine high-frequency successes "summarized into milestones" (FR-001)? → A: One event per actor/service/time-bucket carrying an occurrence count ("used GitHub · 41 times, 09:00–10:00").
- Q: Does an event belong to exactly one narrative thread? → A: No; an event may appear in several threads (a tool call belongs to its approval flow, the agent's journey, and the service's lifecycle).
- Q: Is on-load freshness sufficient for needs-attention items? → A: No; a pending approval expires within minutes, so the needs-attention area refreshes periodically while visible. Historical activity stays on-load/manual.

### Session 2026-09-21

- Q: What must happen if the system cannot record a security-significant activity event? → A: Complete the action and record only an operational error.
- Q: Which activity-event data may be displayed when a field has not been explicitly classified as safe for user display? → A: Display only pre-approved user-display fields; omit or redact all others.
- Q: How should a user see an activity record that must be corrected after it has appeared? → A: No correction flow is defined for this feature.
- Q: How should activity older than the configured retention window appear to the user? → A: Do not show older activity; history ends at the retention window.
- Q: When duplicate reports describe the same underlying action, should the user see more than one activity event? → A: Show one canonical event; duplicate reports are not shown.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Understand recent activity at a glance (Priority: P1)

A user opens the dedicated activity experience — a first-class destination in the end-user application, reachable directly from the primary navigation as a sibling to consent and session management rather than nested inside them — and immediately sees a scannable, plain-language summary of the significant things that recently happened on their behalf: an agent was delegated new access, a connected service was reconnected, an agent acted through a service, a tool call was approved or denied, an authorization was blocked by policy, a session expired. Without opening any detail, the user can tell, for each item, **what happened**, **when**, **who or what was involved**, **what was affected**, and **whether it succeeded, failed, or was blocked**.

**Why this priority**: This is the core of the feature. Delegation without visibility erodes trust; a comprehensible, at-a-glance account of activity is the minimum that delivers trust value. Every other story builds on this surface.

**Independent Test**: Seed a mix of significant events for a user (a new grant, a session reconnect, an agent action, an approved tool call, a policy denial), including duplicate reports for one underlying action. Open the activity experience and confirm that each underlying action is shown once with a plain-language summary answering what/when/who/what-affected/outcome, and that the surface is scannable without opening details.

**Acceptance Scenarios**:

1. **Given** a user has recent activity across grants, sessions, agent actions, and policy decisions, **When** they open the activity experience, **Then** they see a visually summarized, plain-language account of those events ordered by recency, each stating what happened, when, who/what was involved, what was affected, and the outcome.
2. **Given** an event describes a technical operation (for example a token exchange or a policy evaluation), **When** it is shown, **Then** it is expressed in human-readable language rather than internal system jargon or raw identifiers.
3. **Given** an event has a clear outcome, **When** it is displayed, **Then** success, failure, and blocked outcomes are each distinguishable without relying on color alone (for example by label, icon, and text).
4. **Given** the user has never had any activity, **When** they open the experience, **Then** they see a clear, reassuring empty state that explains what will appear here, with no error.
5. **Given** an event references an agent or service that has since been revoked or removed, **When** it is shown, **Then** the event remains readable and correctly attributes the historical actor without breaking.
6. **Given** the user is anywhere in the end-user application, **When** they use the primary navigation, **Then** the activity experience is reachable as its own top-level destination in a single action, not nested inside a specific agent, consent, or session view.
7. **Given** duplicate reports describe the same underlying action, **When** the user opens the activity experience, **Then** they see one canonical event for that action, not duplicate events.

---

### User Story 2 - See what needs attention and what to do next (Priority: P1)

A user needs to know, without hunting, which events require follow-up: a connected service that must be reconnected, a session that expired, or a tool call awaiting approval. The experience surfaces these "needs attention" items — those with a live resolution path — prominently and, for each, offers a clear next step that leads into the existing consent, session, or approval flow that resolves it. Notable events without a resolution path (an action that was blocked, access that was revoked) remain visible as informational history rather than occupying the needs-attention area.

**Why this priority**: Trust requires actionability. Explaining that something failed or is blocked is only useful if the user can understand the consequence and act. Attention triage plus guided next steps is essential to the feature's value and belongs in the MVP alongside Story 1.

**Independent Test**: Seed events that require follow-up (an expired session needing reconnect, a pending approval, a blocked action) alongside events that do not. Open the experience and confirm the needs-attention items are clearly separated/highlighted and that each exposes a next-step action that navigates to the correct existing flow.

**Acceptance Scenarios**:

1. **Given** the user has one or more events requiring follow-up, **When** they open the activity experience, **Then** those events are clearly distinguished as needing attention and are easy to find without scanning the full history.
2. **Given** a connected service requires reconnection, **When** the user views the corresponding needs-attention item, **Then** it explains the situation in plain language and offers a next step that leads to the reconnect flow.
3. **Given** a tool call is awaiting the user's approval, **When** the user views the needs-attention item, **Then** it offers a next step that leads to the approval review for that action.
4. **Given** an action was blocked by policy or an authorization was denied, **When** the user views that event, **Then** it explains what was blocked and why in plain language; if a resolution path exists it appears as a needs-attention item offering the appropriate next step, otherwise it is shown as informational history without occupying the needs-attention area.
5. **Given** the user has no items requiring attention, **When** they open the experience, **Then** the needs-attention area communicates a settled "nothing needs your attention" state rather than appearing broken or empty in an alarming way.
6. **Given** a needs-attention item has been resolved (for example the session was reconnected or the approval was decided), **When** the user next views the experience, **Then** the item no longer appears as needing attention and its resolution is reflected in the history.

---

### User Story 3 - Explore activity by agent, service, grant, time, and outcome (Priority: P2)

A user wants to answer targeted questions: "What has this agent done?", "What happened with my GitHub connection?", "What was blocked this week?", "What still needs attention?". The experience lets the user narrow the activity by agent, by connected service, by permission/grant context, by time window, by outcome (succeeded/failed/blocked), and by needs-attention state, and combine those dimensions.

**Why this priority**: Once the overview exists, exploration makes it genuinely useful for investigation and for building confidence about a specific agent or service. It is high value but depends on Story 1 being in place.

**Independent Test**: Seed activity spanning multiple agents, services, time windows, and outcomes. Apply each filter dimension individually and in combination, and confirm the visible activity matches the selected criteria and that clearing filters restores the full view.

**Acceptance Scenarios**:

1. **Given** activity exists for several agents, **When** the user narrows to a single agent, **Then** only that agent's activity is shown and the active narrowing is clearly indicated.
2. **Given** activity exists for several connected services, **When** the user narrows to a single service, **Then** only activity involving that service is shown.
3. **Given** activity spans a range of dates, **When** the user selects a time window, **Then** only events within that window are shown.
4. **Given** events have different outcomes, **When** the user narrows to "blocked" (or "failed", or "succeeded"), **Then** only events with that outcome are shown.
5. **Given** the user narrows to "needs attention", **When** the filter is applied, **Then** only follow-up items are shown.
6. **Given** the user has combined multiple filters, **When** they clear the filters, **Then** the full activity view is restored.
7. **Given** a filter combination matches no events, **When** it is applied, **Then** a clear "no activity matches these filters" state is shown with an easy way to reset.

---

### User Story 4 - Inspect a single event in depth (Priority: P2)

For an event that matters, the user opens it to see the full picture through progressive disclosure: the complete context of what happened, the related sequence of events around it (for example a tool call that was requested, approved, then executed), and links back to the related consent, session, or agent context. Sensitive details are redacted or abstracted so inspection never exposes secrets.

**Why this priority**: Deeper inspection turns a summary into a trustworthy audit trail. It relies on the overview and adds the "why" and the connective tissue, but is not required for the initial scannable value.

**Independent Test**: Seed a multi-step flow (tool call requested, approved, executed) plus standalone events carrying sensitive parameters and fields not approved as safe for user display. Open each event's detail and confirm the full context, the related sequence, and working links back to the related agent/session/grant context, and confirm unapproved values are not exposed.

**Acceptance Scenarios**:

1. **Given** an event in the overview, **When** the user opens its detail, **Then** they see the full context (actor, affected subject, timing, outcome, and a plain-language explanation of why it happened) without leaving the activity experience.
2. **Given** an event is part of a larger flow, **When** the user opens its detail, **Then** the related sequence of events is shown in order so the user can follow the progression.
3. **Given** an event relates to a specific agent, session, or grant, **When** the user opens its detail, **Then** a link leads to that related consent/session/agent context in a single action.
4. **Given** an event carries sensitive information or fields not explicitly approved as safe for user display, **When** its detail is shown, **Then** only approved fields are shown; every other value is redacted or abstracted and never displayed in the clear.
5. **Given** an action is still in progress with no final outcome, **When** the user opens its detail, **Then** the pending state is shown honestly rather than implying success or failure.

---

### User Story 5 - Follow activity as narratives and timelines (Priority: P3)

Rather than reading rows one by one, the user can follow activity as coherent stories: an agent's journey over a period, a connected service's lifecycle from connection through refreshes to reconnection, or a session's timeline. The experience groups related events into narrative sequences and timelines so understanding scales even when there are many events.

**Why this priority**: Narrative grouping and timelines are what make the experience comprehensible at moderate and high volume and differentiate it from a plain log. It is valuable polish that depends on the earlier stories and the underlying event structure.

**Independent Test**: Seed a connected service that was linked, refreshed several times, expired, and reconnected, plus an agent with a day of activity. Confirm the experience presents these as coherent grouped timelines/sequences that remain readable, and that the grouping still holds under a high event count.

**Acceptance Scenarios**:

1. **Given** a connected service with a lifecycle of connect, refresh, expire, and reconnect events, **When** the user views its activity, **Then** those events are presented as a single coherent timeline rather than scattered independent rows.
2. **Given** an agent with many related actions, **When** the user views its activity, **Then** the actions are grouped into a readable narrative sequence.
3. **Given** a large volume of events, **When** the user opens the experience, **Then** grouping and summarization keep it scannable and a specific recent event remains findable without overwhelming the user.
4. **Given** a grouped timeline, **When** the user drills into any point, **Then** progressive disclosure reveals that point's detail without losing the surrounding narrative.

---

### Edge Cases

- **No activity yet**: The experience shows a clear, non-alarming empty state explaining what will appear, not an error.
- **Very high volume**: With thousands of events, grouping, summarization, and incremental loading keep the experience scannable and performant; the user can still locate a specific recent event.
- **Deleted or revoked references**: Events that reference an agent, service, grant, or session that has since been removed or revoked still render correctly and attribute the historical actor.
- **Sensitive or unapproved payloads**: Events whose underlying data includes secrets, raw tokens, parameters marked sensitive, or fields not explicitly approved as safe for user display show only approved fields; every other value is redacted or abstracted everywhere it could appear (overview and detail).
- **In-flight/unknown outcome**: Events without a final outcome are shown as pending, never implied as success or failure.
- **New events arriving while viewing**: The user is not disrupted; new significant events become visible on the next load or refresh without losing the user's place.
- **Assistive technology traversal**: A screen-reader user can perceive, navigate, and understand visual summaries, groupings, and timelines through non-visual equivalents.
- **Reduced motion**: Any animated transitions in timelines or summaries are suppressed or simplified when the user prefers reduced motion, without losing information.
- **High zoom / small viewport**: The experience remains readable and usable at high zoom and on constrained widths without loss of content or function.
- **Cross-user isolation**: A user only ever sees activity within their own scope; another user's activity is never exposed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST provide an end-user activity experience that surfaces business-significant and security-significant events for the current user, spanning at minimum: sessions, grants, delegations, agent activity, policy/authorization decisions, failures, revocations, and reconnect flows. The experience surfaces a curated set of significant events; routine, high-frequency internal operations (for example individual token exchanges or per-request policy allow evaluations) MUST be summarized into their significant milestones rather than surfaced as individual events.
- **FR-002**: For every surfaced event, the experience MUST let the user determine **what happened**, **when it happened**, **who or what was involved**, **what was affected**, and **the outcome** (succeeded, failed, blocked, or pending).
- **FR-003**: Each event MUST indicate whether follow-up action is needed. The needs-attention set MUST be derived purely from current unresolved state: only events with a live resolution path (for example a required reconnect or a pending approval) appear as needing attention, and they MUST remain visible until the underlying flow completes, regardless of the activity-retention window — with no user dismissal or acknowledgment. Notable but non-actionable events (for example a blocked or denied action, or a completed revocation) remain visible as informational history and MUST NOT occupy the needs-attention area. Events needing attention MUST be distinguishable and locatable without scanning the entire history.
- **FR-004**: For each event that needs attention and has a resolution path, the experience MUST offer a clear next step that leads into the existing consent, session, or approval flow that resolves it.
- **FR-005**: The experience MUST support fast scanning of events, plain-language explanation of each event, and deeper inspection of an event's details through progressive disclosure.
- **FR-006**: The experience MUST let the user explore activity by agent, by connected service, by permission/grant context, by time window, by outcome, and by needs-attention state, and MUST allow these dimensions to be combined and cleared. The time-window control MUST offer relative presets (for example last 24 hours, last 7 days, and all activity within retention); an arbitrary custom start/end date range is not required for the MVP.
- **FR-007**: The primary presentation MUST NOT rely on simple tables; it MUST emphasize comprehensible visual summaries, narrative groupings, sequences/timelines, and progressive disclosure.
- **FR-008**: The experience MUST link relevant activity back to the related consent, session, or agent context where appropriate, reachable in a single action from the event.
- **FR-009**: Event detail MUST present the full context of an event, including a plain-language explanation of why it happened and, where the event is part of a larger flow, the related sequence of events in order.
- **FR-010**: The default presentation of every event MUST use human-readable language rather than internal jargon or raw system identifiers.
- **FR-011**: Sensitive information (including secrets, raw tokens, and parameters marked sensitive) MUST be redacted or abstracted wherever it would otherwise appear, in both summary and detail views. Only fields explicitly approved as safe for user display MAY be shown; all other event fields MUST be omitted or redacted.
- **FR-012**: The experience MUST scope all activity to the current authenticated user and MUST NOT expose any other user's activity.
- **FR-013**: The experience MUST remain useful and scannable under low, moderate, and high event volume, using grouping, summarization, and incremental loading so that a specific recent event remains findable.
- **FR-014**: The experience MUST present a clear, non-error empty state when the user has no activity and a clear "no matches" state when a filter combination yields no events. Where history ends because of the retention window, the experience MUST say so rather than presenting the boundary as "nothing happened".
- **FR-015**: The experience MUST correctly render and attribute events whose referenced agent, service, grant, or session has since been revoked or removed.
- **FR-016**: The activity experience MUST be a dedicated, first-class destination within the end-user application, reachable directly from the primary navigation as a peer to consent and session management, and MUST NOT be nested as a subordinate view within a specific agent, consent, or session detail.
- **FR-016a**: If the system cannot record a security-significant activity event, it MUST complete the underlying action and record an operational error.
- **FR-016b**: The experience MUST show no more than one activity event for each underlying action; duplicate reports MUST NOT create additional visible events.

### Accessibility Requirements

- **FR-017**: The entire experience MUST be fully operable by keyboard, including navigation of visual summaries, groupings, timelines, filters, and progressive-disclosure controls.
- **FR-018**: All information conveyed visually (including summaries, groupings, timelines, and outcome status) MUST be comprehensible to screen-reader users through non-visual equivalents.
- **FR-019**: Outcome and status distinctions (succeeded, failed, blocked, needs attention) MUST be understandable without relying on color alone.
- **FR-020**: The experience MUST honor a reduced-motion preference, suppressing or simplifying animation without losing information.
- **FR-021**: The experience MUST remain readable and usable at high zoom levels and on constrained viewport widths without loss of content or function.

### Key Entities *(include if feature involves data)*

- **Activity Event**: A single business-significant or security-significant occurrence within the current user's scope. Each underlying action corresponds to no more than one visible Activity Event, even when duplicate reports arrive. Key attributes: category/type, timestamp, actor, affected subject, outcome (succeeded/failed/blocked/pending), significance, needs-attention state, a human-readable summary, redaction state, and references to related context. Corresponds to occurrences such as grant/delegation changes, session lifecycle changes, agent actions, policy/authorization decisions, approval decisions, revocations, reconnect prompts, and failures.
- **Activity Category**: The taxonomy that groups events into user-comprehensible kinds (for example: delegation & grants, connected-service sessions, agent activity, policy decisions, approvals, revocations, reconnect, failures).
- **Actor**: The party responsible for an event — an authorized agent, a connected service, the user (principal), or the broker/system itself.
- **Affected Subject**: The thing an event acted on or changed — a grant, a permission set, a connected service, a session, a tool, or a protected resource.
- **Activity Thread**: An ordered grouping of related events that forms a narrative or timeline (for example a tool-call flow, a connected-service lifecycle, or an agent's journey over a period).
- **Needs-Attention Item**: An event or thread that requires user follow-up, paired with a recommended next step that leads into the existing consent, session, or approval flow.
- **Exploration Context**: The set of dimensions used to narrow the view — agent, connected service, permission/grant context, time window, outcome, and needs-attention state.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any surfaced event, a user can correctly state what happened, when, who/what was involved, what was affected, and the outcome within 10 seconds and without opening the event's detail.
- **SC-002**: A user can locate all activity for a specific agent, service, time window, or outcome in no more than two interactions.
- **SC-003**: In usability testing, at least 90% of participants correctly describe, in their own words, what a given event means — with no reliance on internal jargon.
- **SC-004**: Every event flagged as needing attention exposes a next-step action that advances or resolves it, reachable in no more than two interactions.
- **SC-005**: With at least 10,000 events present, a user can still find a specific recent event in under 30 seconds, and the experience remains scannable and responsive.
- **SC-006**: The experience passes WCAG 2.1 AA: it is 100% keyboard operable, conveys all information non-visually, distinguishes status without color alone, remains usable at 200% zoom, and honors reduced-motion — verified by audit.
- **SC-007**: No sensitive value (secret, raw token, or parameter marked sensitive) or event field not explicitly approved as safe for user display appears anywhere in the experience — verified across summary and detail views for a corpus of events known to carry sensitive and unapproved data.
- **SC-008**: From any event, a user can reach the related consent, session, or agent context in a single action.
- **SC-009**: When there is no activity, or when a filter combination matches nothing, 100% of these cases present a clear, non-error state rather than a blank or broken view.
- **SC-010**: A first-time user can locate and open the activity experience from the application's primary navigation in a single action, without first entering a specific agent, consent, or session view.

## Assumptions

- **Reuse existing surface**: The experience is delivered as a new, top-level section of the existing end-user application (the same SPA that today serves consent, sessions, and approvals), composing the existing design-system components and patterns rather than introducing a separate application or a new app shell. Reusing the shell and components does **not** make it subordinate to the consent flow — it is its own destination reached from the primary navigation, and it links into consent, session, and approval contexts rather than living inside them.
- **Per-user scope**: Activity is scoped to the current authenticated principal, consistent with the established end-user identity model; there is no cross-user, tenant-wide, or operator/admin activity in this feature.
- **Event source is in scope as a dependency**: Presenting these events requires the system to record or derive them from existing domain state changes and security decisions (session lifecycle, grant/delegation changes, approvals, policy/authorization decisions, token exchanges, revocations, reconnect prompts, failures). Producing and exposing that user-facing activity record is a dependency of this feature; the exact mechanism is deferred to planning.
- **Read-mostly**: The activity experience itself is read-only, except that needs-attention items link into the existing consent, session, and approval flows; it introduces no new mutation surface of its own. Needs-attention state is derived from current unresolved state, not user-dismissible, so no acknowledgment or read-state store is introduced.
- **Retention window**: A rolling retention window defaulting to **30 days** (proposed 2026-09-22, superseding the earlier 7-day default; pending stakeholder confirmation) governs how far back historical activity is surfaced. Activity older than the configured window MUST NOT be shown; history ends at that boundary with no older-event summaries, and the boundary is explained to the user. The window does not limit unresolved needs-attention items, which remain visible until resolved. The window is tunable via configuration.
- **Freshness**: On-load and manual refresh freshness is sufficient for historical activity; live streaming/real-time push of new events is not required. The needs-attention area refreshes periodically while the experience is visible so its next steps are not stale.
- **Time display**: Timestamps are presented in the user's local context with relative labels (for example "2 hours ago") plus an absolute time available on inspection.
- **Non-goals**: Bulk export/download of an audit log, operator/admin-facing audit dashboards, long-term compliance archival, machine-consumable audit APIs, and activity-history correction, reversal, or deletion workflows are out of scope for this feature.
- **Dependency on existing capabilities**: The feature depends on the existing consent, session, approval, revocation, and reconnect flows to fulfill the "what can I do next" next steps.
