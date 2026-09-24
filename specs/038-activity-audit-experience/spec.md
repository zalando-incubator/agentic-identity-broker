# Feature Specification: End-User Activity & Auditing Experience

**Feature Branch**: `038-activity-audit-experience`

**Created**: 2026-09-04

**Status**: Draft

**Input**: User description: "Extend the end-user consent and session experience so users can inspect, understand, and trust what the identity broker, connected services, and authorized agents are doing on their behalf. Users need an activity and auditing experience that explains what happened, why it happened, who or what was involved, what access or service was affected, whether the action succeeded, failed, or was blocked, and what they can do next."

## Clarifications

### Session 2026-09-04

- Q: How does a needs-attention item with no resolution path (a blocked/denied action, a completed revocation) leave the needs-attention area? → A: It never enters it — needs-attention is purely derived from live unresolved state (reconnect required, pending approval) and clears automatically; non-actionable notable events are informational history only, with no user dismissal (preserves the read-only assumption).
- Q: What was the initial retention proposal? → A: A seven-day rolling default was proposed on 2026-09-04 and superseded by the approval on 2026-09-23. The configured window always controls visible history.
- Q: Does the experience record every low-level, high-frequency success (each token exchange, each policy allow) or a curated set of significant events? → A: A curated set of business/security-significant events; routine high-frequency internals (individual token exchanges, per-request allow evaluations) are summarized into their significant milestones rather than surfaced individually.
- Q: What form does the time-window filter take (relative presets vs a custom date range)? → A: Relative presets only (for example last 24 hours, last 7 days, all activity within retention); an arbitrary custom start/end date range is not required for the MVP.

### Session 2026-09-09

- Q: Should unresolved needs-attention items remain visible regardless of history retention? → A: Keep unresolved needs-attention items live until resolved, regardless of history retention.

### Session 2026-09-22 (plan review)

- Q: Is a 7-day default retention sufficient for the trust goal? → A: No. The requester approved a **30-day default** and **arbitrary positive Go-duration values** on 2026-09-23. The boundary must be explained (FR-014). This approval does not cover the revised three-endpoint API contract; Principle X still requires written stakeholder sign-off before handlers.
- Q: How is "one canonical event per action" (FR-016b) achieved when the same action is reported repeatedly (replica caches, retries, lazy expiry detection)? → A: Each event carries a deterministic idempotency key; duplicate reports are absorbed, never shown.
- Q: How are routine high-frequency successes "summarized into milestones" (FR-001)? → A: One event per agent/service/time bucket counts successful broker access issuances ("Broker issued GitHub access for Research Agent · 41 times"). This is not a count of downstream uses.
- Q: Does an event belong to exactly one narrative thread? → A: No; an event may appear in several threads (a tool call belongs to its approval flow, the agent's journey, and the service's lifecycle).
- Q: Is on-load freshness sufficient for needs-attention items? → A: No; a pending approval expires within minutes, so the needs-attention area refreshes periodically while visible. Historical activity stays on-load/manual.

### Session 2026-09-21

- Q: What must happen if the system cannot record a security-significant activity event? → A: Complete the authorized action. The failed Activity recording adds an operational error; the underlying security action still emits its independent, safe structured audit record.
- Q: Which activity-event data may be displayed when a field has not been explicitly classified as safe for user display? → A: Display only pre-approved user-display fields; omit or redact all others.
- Q: How should a user see an activity record that must be corrected after it has appeared? → A: No correction flow is defined for this feature.
- Q: How should activity older than the configured retention window appear to the user? → A: Do not show older activity; history ends at the retention window.
- Q: When duplicate reports describe the same underlying action, should the user see more than one activity event? → A: Show one canonical event; duplicate reports are not shown.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Understand recent activity at a glance (Priority: P1)

A user opens the dedicated activity experience — a first-class destination in the end-user
application, reachable from primary navigation alongside consent and session management. The
user sees a scannable summary of recent significant events: an agent received new access, a
service was reconnected, the broker issued delegated access for an agent, a tool call was approved
or denied, an authorization was blocked, or a session expired. Without opening a detail, the
user can tell **what happened**, **when**, **who or what was involved**, **what was affected**,
and **whether it succeeded, failed, or was blocked**.

**Why this priority**: This is the core of the feature. Delegation without visibility erodes trust; a comprehensible, at-a-glance account of activity is the minimum that delivers trust value. Every other story builds on this surface.

**Independent Test**: Seed significant events for a user (a grant, a reconnect, a broker access
issuance, an approval, a policy denial), including duplicate reports for one transition. Open
the activity experience. Confirm that each transition appears once with a plain-language
summary, a time, an actor, a subject, and an outcome.

**Acceptance Scenarios**:

1. **Given** a user has recent activity across grants, sessions, broker access issuances, and policy decisions, **When** they open the activity experience, **Then** they see those events ordered by recency, each with what happened, when, the actor, the subject, and the outcome.
2. **Given** an event describes a technical operation (for example a token exchange or a policy evaluation), **When** it is shown, **Then** it is expressed in human-readable language rather than internal system jargon or raw identifiers.
3. **Given** an event has a clear outcome, **When** it is displayed, **Then** success, failure, and blocked outcomes are each distinguishable without relying on color alone (for example by label, icon, and text).
4. **Given** the user has no activity within the configured retention window, **When** they open the experience, **Then** they see neutral empty copy explaining that new activity will appear here and the exact local history boundary, with no error.
5. **Given** an event references an agent or service that has since been revoked or removed, **When** it is shown, **Then** the event remains readable and correctly attributes the historical actor without breaking.
6. **Given** the user is anywhere in the end-user application, **When** they use the primary navigation, **Then** the activity experience is reachable as its own top-level destination in a single action, not nested inside a specific agent, consent, or session view.
7. **Given** duplicate reports describe the same underlying action, **When** the user opens the activity experience, **Then** they see one canonical event for that action, not duplicate events.

---

### User Story 2 - See what needs attention and what to do next (Priority: P1)

A user sees a separate live band for unresolved sessions requiring reconnection and pending approvals. These Needs-Attention Items come from current state, not historical events; they survive missing or pruned history. Each offers a next step in an existing session or approval flow. The exact retained transition can also appear under the feed's `needs_attention=true` filter. Expired or settled approvals leave the live band; a recorded expiry remains historical. Blocked actions and completed revocations without a live next step appear only in history.

**Why this priority**: Trust requires actionability. Explaining that something failed or is blocked is only useful if the user can understand the consequence and act. Attention triage plus guided next steps is essential to the feature's value and belongs in the MVP alongside Story 1.

**Independent Test**: Seed a reconnect-required session and a pending approval with and without retained transitions. Confirm both live items and their next steps appear. Resolve them, and confirm the band clears while recorded resolutions remain in history. A blocked action stays informational history.

**Acceptance Scenarios**:

1. **Given** the user has unresolved sessions or approvals, **When** they open Activity, **Then** a distinct live attention band shows the next steps even when the historical events are missing or pruned.
2. **Given** a connected service requires reconnection, **When** the user views the corresponding needs-attention item, **Then** it explains the situation in plain language and offers a next step that leads to the reconnect flow.
3. **Given** a tool call is awaiting the user's approval, **When** the user views the needs-attention item, **Then** it offers a next step that leads to the approval review for that action.
4. **Given** a policy block or denied authorization, **When** the user views its history, **Then** it explains the cause; only a separate live unresolved session or approval can enter the attention band.
5. **Given** the user has no items requiring attention, **When** they open the experience, **Then** the needs-attention area communicates a settled "nothing needs your attention" state rather than appearing broken or empty in an alarming way.
6. **Given** an approval is settled or expired or a session is reconnected, **When** the band refreshes, **Then** the item clears; any recorded resolution or expiry remains historical.

---

### User Story 3 - Explore activity by agent, service, grant, time, and outcome (Priority: P2)

A user wants to answer targeted questions: "When did the broker issue GitHub access for this agent?", "What happened with my GitHub connection?", "What was blocked this week?", "What still needs attention?". The experience lets the user narrow activity by agent, connected service, grant, time window, outcome, and needs-attention state, and combine those dimensions.

**Why this priority**: Once the overview exists, exploration makes it genuinely useful for investigation and for building confidence about a specific agent or service. It is high value but depends on Story 1 being in place.

**Independent Test**: Seed two grants and an agent and service absent from the first feed page. From a card's approved grant reference or the separate grant link on `/agents/{agent_id}`, enter `/activity?grant_id={grants.id}`. Combine it with an outcome filter, remove the grant chip, and clear all. Also follow agent/service links from their existing pages and confirm retained off-page matches appear.

**Acceptance Scenarios**:

1. **Given** activity exists for several agents, **When** the user narrows to a single agent, **Then** only that agent's activity is shown and the active narrowing is clearly indicated.
2. **Given** activity exists for several connected services, **When** the user narrows to a single service, **Then** only activity involving that service is shown.
3. **Given** activity spans a range of dates, **When** the user selects a time window, **Then** only events within that window are shown.
4. **Given** events have different outcomes, **When** the user narrows to "blocked" (or "failed", or "succeeded"), **Then** only events with that outcome are shown.
5. **Given** the user selects `needs_attention=true`, **When** the feed filter applies, **Then** only retained events matching a current unresolved approval or session transition appear; live attention remains independent.
6. **Given** a card has an approved `related_refs.grant_id` or an existing `UserGrant` appears at `/agents/{agent_id}`, **When** the user selects “View this grant's activity” or the grant activity link and adds a second filter, **Then** the URL contains the exact `grant_id`, both filters intersect, a removable grant chip appears, and clear-all restores the full feed. A card without a grant reference offers no grant action.
7. **Given** a filter combination matches no events, **When** it is applied, **Then** a clear "no activity matches these filters" state is shown with an easy way to reset.

---

### User Story 4 - Inspect a single event in depth (Priority: P2)

An event detail shows the full context and related broker-observed transitions. For example, it
shows that a tool approval was requested, approved, then consumed. It does not claim that the
downstream tool finished. Links lead back to consent, session, or agent context. Sensitive
details remain redacted or abstracted.

**Why this priority**: Deeper inspection turns a summary into a trustworthy audit trail. It relies on the overview and adds the "why" and the connective tissue, but is not required for the initial scannable value.

**Independent Test**: Seed an approval flow (requested, approved, consumed) and standalone events
with sensitive or unapproved fields. Open the event details and confirm the context, ordered
sequence, related links, and absence of unapproved values.

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

**Independent Test**: Seed a service that was connected, refreshed, expired, and reconnected.
Seed broker access milestones and approvals for one agent. Confirm the sequences remain readable
as the number of events increases.

**Acceptance Scenarios**:

1. **Given** a connected service with a lifecycle of connect, refresh, expire, and reconnect events, **When** the user views its activity, **Then** those events are presented as a single coherent timeline rather than scattered independent rows.
2. **Given** an agent has many broker-observed milestones, **When** the user views its activity, **Then** the milestones form a readable narrative without implying that downstream actions completed.
3. **Given** a large volume of events, **When** the user opens the experience, **Then** grouping and summarization keep it scannable and a specific recent event remains findable without overwhelming the user.
4. **Given** a grouped timeline, **When** the user drills into any point, **Then** progressive disclosure reveals that point's detail without losing the surrounding narrative.

---

### Edge Cases

- **No retained activity**: The experience shows “No activity in the period shown. New activity will appear here.” with the exact local retention boundary. It does not claim the user never had activity.
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

- **FR-001**: The system MUST provide an end-user activity experience for business-significant
  and security-significant events for the current user. It MUST cover sessions, grants,
  delegations, broker-observed agent access, policy/authorization decisions, failures,
  revocations, and reconnect flows. Routine token exchanges and policy allows MUST appear as
  summarized milestones, not individual events. A successful broker token exchange MUST NOT be
  described or counted as a completed downstream action.
- **FR-002**: For every surfaced event, the experience MUST let the user determine **what happened**, **when it happened**, **who or what was involved**, **what was affected**, and **the outcome** (succeeded, failed, blocked, or pending).
- **FR-003**: A Needs-Attention Item MUST represent live unresolved session or approval state with a next step. It MUST survive missing or pruned Activity history and clear when its source resolves; users cannot dismiss it. The `needs_attention=true` feed filter MUST show only retained events matching the current unresolved `approval_id` or `session_id` plus `token_revision`. Neither view needs a stored per-event attention flag. Expired or settled approvals leave the live band while any recorded expiry stays in history. Non-actionable blocked or denied events remain informational history.
- **FR-004**: Each live Needs-Attention Item MUST link to the existing consent, session, or approval flow that resolves it.
- **FR-005**: The experience MUST support fast scanning of events, plain-language explanation of each event, and deeper inspection of an event's details through progressive disclosure.
- **FR-006**: The experience MUST let the user explore activity by agent, by connected service, by permission/grant context, by time window, by outcome, and by needs-attention state, and MUST allow these dimensions to be combined and cleared. The time-window control MUST offer relative presets (for example last 24 hours, last 7 days, and all activity within retention); an arbitrary custom start/end date range is not required for the MVP.
- **FR-007**: The primary presentation MUST NOT rely on simple tables; it MUST emphasize comprehensible visual summaries, narrative groupings, sequences/timelines, and progressive disclosure.
- **FR-008**: The experience MUST link relevant activity back to the related consent, session, or agent context where appropriate, reachable in a single action from the event.
- **FR-009**: Event detail MUST present the full context of an event, including a plain-language explanation of why it happened and, where the event is part of a larger flow, the related sequence of events in order.
- **FR-010**: The default presentation of every event MUST use human-readable language rather than internal jargon or raw system identifiers.
- **FR-011**: Sensitive information (including secrets, raw tokens, and parameters marked sensitive) MUST be redacted or abstracted wherever it would otherwise appear, in both summary and detail views. Only fields explicitly approved as safe for user display MAY be shown; all other event fields MUST be omitted or redacted.
- **FR-012**: The experience MUST scope all activity to the current authenticated user and MUST NOT expose any other user's activity.
- **FR-013**: The experience MUST remain useful and scannable under low, moderate, and high event volume, using grouping, summarization, and incremental loading so that a specific recent event remains findable.
- **FR-014**: The experience MUST present neutral, non-error empty copy for no retained activity and a reset action for no filter matches. It MUST show the exact configured history cutoff in the browser's local time whenever the historical feed has loaded, including empty, paginating, filtered, and detail-child views. The cutoff MUST NOT limit live attention items or imply that no earlier activity occurred.
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

- **Activity Event**: One significant occurrence within the current user's scope. Duplicate
  reports of one transition produce no more than one visible event; distinct transitions retain
  distinct identities. Attributes include category, type, time, actor, subject, outcome, summary,
  redaction state, and related references. Events include grant changes, session lifecycle,
  broker access issuances for agents, policy decisions, revocations, and failures.
- **Activity Category**: A user-comprehensible event kind, such as delegation, session, broker-issued agent access, policy decision, approval, revocation, completed reconnection, or failure.
- **Actor**: The party responsible for an event — an authorized agent, a connected service, the user (principal), or the broker/system itself.
- **Affected Subject**: The thing an event acted on or changed — a grant, a permission set, a connected service, a session, a tool, or a protected resource.
- **Activity Thread**: An ordered grouping of related events that forms a narrative or timeline (for example a tool-call flow, a connected-service lifecycle, or an agent's journey over a period).
- **Needs-Attention Item**: A live unresolved session or approval state with a next step in an existing flow. It is not an Activity Event or thread, is never persisted separately, and survives missing or pruned history.
- **Exploration Context**: The set of dimensions used to narrow the view — agent, connected service, permission/grant context, time window, outcome, and needs-attention state.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For any surfaced event, a user can correctly state what happened, when, who/what was involved, what was affected, and the outcome within 10 seconds and without opening the event's detail.
- **SC-002**: A user can locate all retained activity for a specific agent, service, time window, or outcome in no more than two interactions.
- **SC-003**: In usability testing, at least 90% of participants correctly describe, in their own words, what a given event means — with no reliance on internal jargon.
- **SC-004**: Every live Needs-Attention Item offers an existing next-step flow that advances or resolves it, reachable in no more than two interactions.
- **SC-005**: With at least 10,000 events present, a user can still find a specific recent event in under 30 seconds, and the experience remains scannable and responsive.
- **SC-006**: The experience passes WCAG 2.1 AA: it is 100% keyboard operable, conveys all information non-visually, distinguishes status without color alone, remains usable at 200% zoom, and honors reduced-motion — verified by audit.
- **SC-007**: No sensitive value (secret, raw token, or parameter marked sensitive) or event field not explicitly approved as safe for user display appears anywhere in the experience — verified across summary and detail views for a corpus of events known to carry sensitive and unapproved data.
- **SC-008**: From a relevant event with a current accessible related target, a user can reach the related consent, session, or agent context directly from its feed card in one action; missing targets keep historical text without a broken link.
- **SC-009**: When there is no activity, or when a filter combination matches nothing, 100% of these cases present a clear, non-error state rather than a blank or broken view.
- **SC-010**: A first-time user can locate and open the activity experience from the application's primary navigation in a single action, without first entering a specific agent, consent, or session view.

## Assumptions

- **Reuse existing surface**: The experience is delivered as a new, top-level section of the existing end-user application (the same SPA that today serves consent, sessions, and approvals), composing the existing design-system components and patterns rather than introducing a separate application or a new app shell. Reusing the shell and components does **not** make it subordinate to the consent flow — it is its own destination reached from the primary navigation, and it links into consent, session, and approval contexts rather than living inside them.
- **Per-user scope**: Activity is scoped to the current authenticated principal, consistent with the established end-user identity model; there is no cross-user, tenant-wide, or operator/admin activity in this feature.
- **Event source is in scope as a dependency**: Presenting these events requires the system to record or derive them from existing domain state changes and security decisions (session lifecycle, grant/delegation changes, approvals, policy/authorization decisions, token exchanges, revocations, reconnect prompts, failures). Producing and exposing that user-facing activity record is a dependency of this feature; the exact mechanism is deferred to planning.
- **Read-mostly**: The activity experience itself is read-only, except that needs-attention items link into the existing consent, session, and approval flows; it introduces no new mutation surface of its own. Needs-attention state is derived from current unresolved state, not user-dismissible, so no acknowledgment or read-state store is introduced.
- **Retention window**: The requester approved a **30-day default** (`720h`) and arbitrary positive, representable Go-duration values on 2026-09-23. This supersedes the proposed seven-day default. Historical activity before the configured read-time cutoff MUST NOT appear. Show the exact cutoff to users; do not round durations to days. Unresolved Needs-Attention Items remain live outside this history window. The revised API still requires separate written sign-off.
- **Freshness**: On-load and manual refresh freshness is sufficient for historical activity; live streaming/real-time push of new events is not required. The needs-attention area refreshes periodically while the experience is visible so its next steps are not stale.
- **Time display**: Timestamps are presented in the user's local context with relative labels (for example "2 hours ago") plus an absolute time available on inspection.
- **Non-goals**: Bulk export/download of an audit log, operator/admin-facing audit dashboards, long-term compliance archival, machine-consumable audit APIs, and activity-history correction, reversal, or deletion workflows are out of scope for this feature.
- **Dependency on existing capabilities**: The feature depends on the existing consent, session, approval, revocation, and reconnect flows to fulfill the "what can I do next" next steps.
