# Feature Specification: Revoke Agent Consent

**Feature Branch**: `20260320-120100-revoke-agent-consent`
**Created**: 2026-03-20
**Status**: Draft
**Input**: User description: "I want to add the capability to revoke consent given to agents. At the momement if an end user goes to the broker page and selects an agent, there is no way to revoke the entire agent's permissions. Revocation of individual services is possible but cannot be saved due to them being mandatory."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Revoke All Agent Permissions From Detail Page (Priority: P1)

As an end user who has previously granted an agent access to one or more services, I want to revoke all permissions for that agent in a single action so that the agent can no longer act on my behalf for any service.

A user navigates to the agent detail page, sees a "Revoke All Access" button prominently alongside the existing "Approve & Delegate" action, clicks it, confirms the action in a dialog, and the agent's grant is fully removed. The user is returned to the consent overview page, and the agent no longer appears with an active grant.

**Why this priority**: This is the core capability requested. Without it, users have no way to fully revoke an agent's permissions — the current UI allows toggling individual services off but cannot save that state because mandatory services cannot be deselected.

**Independent Test**: Can be fully tested by creating a grant for an agent, navigating to its detail page, clicking "Revoke All Access", confirming, and verifying the grant no longer exists and the agent appears as "no active grant" on the overview page.

**Acceptance Scenarios**:

1. **Given** a user has an active grant for an agent with at least one mandatory service, **When** the user visits the agent detail page, **Then** a "Revoke All Access" button is visible alongside the approval controls.
2. **Given** the user clicks "Revoke All Access", **When** the confirmation dialog appears, **Then** the dialog clearly states the agent name, that all permissions will be removed, and that any connected OAuth2 services (e.g., GitHub, Google) will remain active and are not affected by this action.
3. **Given** the user confirms revocation, **When** the action completes, **Then** the grant is permanently deleted and the user is redirected to the consent overview page with a success notification.
4. **Given** the user confirms revocation, **When** the action completes, **Then** the agent no longer appears with an active grant on the consent overview page.
5. **Given** the user cancels the confirmation dialog, **When** the dialog is dismissed, **Then** no changes are made to the grant and the detail page remains active.

---

### User Story 2 - Revoke Agent Permissions From Consent Overview (Priority: P2)

As an end user reviewing my active agent consents on the overview page, I want to revoke an agent's permissions directly from the list without navigating into the detail page, so I can efficiently manage multiple agents.

A user sees their list of agents with active grants. Each agent card has a "Revoke" action. Clicking it shows a confirmation dialog, and upon confirmation the grant is removed and the card disappears from the list.

**Why this priority**: Improves efficiency for users managing consent across multiple agents. The detail-page flow (P1) is sufficient for the base case, but inline revocation from the overview significantly reduces friction.

**Independent Test**: Can be fully tested by creating grants for multiple agents, opening the overview page, revoking one agent inline, and confirming only that agent's card is removed from the list while others remain unchanged.

**Acceptance Scenarios**:

1. **Given** a user has active grants for one or more agents, **When** the user views the consent overview page, **Then** each agent card includes a visible "Revoke" action.
2. **Given** the user clicks "Revoke" on an agent card, **When** the confirmation dialog appears, **Then** the dialog names the agent, asks for explicit confirmation, and informs the user that connected OAuth2 services remain active.
3. **Given** the user confirms revocation from the overview, **When** the action completes, **Then** the agent card is removed from the list and a success notification is shown.
4. **Given** the user cancels the confirmation, **When** the dialog is dismissed, **Then** the agent card remains unchanged in the list.

---

### User Story 3 - Revocation is Enforced at Authorization Time (Priority: P3)

As a system operator, I need revoked grants to be rejected at agent authorization time so that revoked agents cannot continue to exchange tokens on behalf of users.

After a user revokes consent, any attempt by the revoked agent to perform a token exchange for that user must be denied with an authorization error.

**Why this priority**: This is a correctness and security requirement ensuring revocation has actual effect beyond the UI. It depends on P1 and addresses the existing `// TODO: Implement revocation check` in the domain authorization service.

**Independent Test**: Can be fully tested by granting an agent access, revoking the grant, then performing a token exchange request for that agent and verifying it is rejected.

**Acceptance Scenarios**:

1. **Given** a user has revoked consent for an agent, **When** that agent attempts a token exchange for the user, **Then** the authorization check fails and no token is returned.
2. **Given** a user has an active grant, **When** the grant is revoked and the agent immediately re-attempts authorization, **Then** the second attempt is denied.

---

### Edge Cases

- What happens when the user attempts to revoke a grant that does not exist? The system must return a clear, user-friendly "not found" message and remain stable.
- What happens when the revocation request fails due to a transient error? The user must see an error message and the grant must remain intact (no partial state).
- What if an agent has no active grant when the user arrives at the detail page? The "Revoke All Access" button must not be shown.
- What if multiple browser tabs are open and the user revokes from one tab while viewing the same agent in another? The second tab must reflect the revoked state upon any subsequent page interaction.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Users MUST be able to revoke all permissions for a specific agent from the agent detail page in a single action.
- **FR-002**: The revocation action MUST require explicit confirmation before any change is persisted.
- **FR-003**: Upon confirmed revocation, the system MUST permanently delete the user's grant for the specified agent.
- **FR-004**: After successful revocation, the user MUST be redirected to the consent overview page with a success notification.
- **FR-005**: Users MUST be able to revoke an agent's permissions directly from the consent overview page without navigating to the detail page.
- **FR-006**: The system MUST reject a revocation request for a non-existent grant with a clear "not found" error.
- **FR-007**: Revocation MUST be scoped to the authenticated user — a user MUST NOT be able to revoke another user's grant.
- **FR-008**: The system MUST enforce that a revoked (deleted) grant is rejected during agent authorization checks, completing the existing revocation TODO in the domain service.
- **FR-009**: The "Revoke All Access" button on the agent detail page MUST only be visible when the user has an active grant for that agent.
- **FR-010**: The confirmation dialog MUST clearly state the agent name, the consequence of revocation, and that any connected OAuth2 services (e.g., GitHub, Google) remain active and are NOT terminated by this action.

### Domain Model

**Entities** (things with unique identity):

- **UserGrant**: Existing entity representing a user's delegation of OAuth2 scopes to an agent. No structural changes — hard deletion remains the revocation mechanism. The domain authorization service's `VerifyAgentAccess` must treat a missing grant as a denial (fail closed).

**Domain Events** (state changes of business significance):

- **GrantRevoked**: Occurs when a user explicitly deletes their grant for an agent. Carries `principal`, `agent_id`, `grant_id`, and `revoked_at` timestamp. Used for audit logging.

*All domain terms should be added to ARCHITECTURE.md Glossary section*

### API Requirements

- **API-001**: All end-user APIs MUST be documented in `/api/enduser/openapi.yaml` (OpenAPI 3.0+ format)
- **API-002**: APIs MUST follow Zalando RESTful API and Event Guidelines
- **API-003**: A dedicated `DELETE /api/consent/agent/{agent-id}/grants` endpoint MUST be added as the canonical, RESTful revocation path.
- **API-004**: The `DELETE` endpoint MUST return `204 No Content` on success and `404 Not Found` if no active grant exists for the authenticated user and given agent.
- **API-005**: The `DELETE` endpoint MUST be authenticated — the principal is derived from the existing auth middleware and only the grant owner may delete it; cross-user deletion returns `404 Not Found` (principal-scoped query returns not-found for another user's grant, leaking no existence information — see SR-001 and `research.md §4`).
- **API-006**: All API changes MUST be confirmed by user/stakeholder before implementation begins.

### Database Requirements

- **DB-001**: No schema changes are required — grant deletion uses the existing or newly added `DeleteUserGrant` storage port method.
- **DB-002**: The `UserGrantRepository` port MUST expose a `DeleteByPrincipalAndAgentID` method. If it does not exist, it MUST be added and implemented for both in-memory and PostgreSQL adapters.
- **DB-003**: The PostgreSQL adapter MUST verify that deleting a non-existent grant returns a domain-level "not found" error (not a raw database error).
- **DB-004**: PostgreSQL-backed repository MUST have integration tests verifying delete behavior (success, not-found, wrong-principal isolation).

### Security Requirements

- **SR-001**: Revocation MUST be authorized — only the principal who owns the grant may delete it. Cross-user deletion returns `404 Not Found` by design: the principal-scoped `DELETE WHERE principal = $1 AND agent_id = $2` query naturally returns not-found for another user's grant, leaking no existence information. `403 Forbidden` is explicitly rejected because it would confirm a grant exists for a given agent ID. See `research.md §4` for full rationale.
- **SR-002**: Revocation operations MUST emit structured audit log entries via the existing application logger (JSON to stdout) including: `principal`, `agent_id`, `grant_id`, `action=grant_revoked`, `timestamp`.
- **SR-003**: System MUST fail closed — if the grant lookup during revocation fails with a transient error, the grant MUST NOT be deleted and the error MUST be surfaced to the caller.
- **SR-004**: The `VerifyAgentAccess` domain function MUST treat a missing grant as an authorization denial (fail closed).

### Frontend/Design System Requirements

**Design System Compliance**:
- All frontend components MUST use design system at `web/src/design-system/`
- Before implementation, review `DECISION_TREES.md`, `COMPONENT_PAIRING_GUIDE.md`, and `COMMON_MISTAKES.md`

**Component Classification**:
- **Application-Specific Components** (`web/src/components/consent/`):
  - `RevokeGrantButton` — destructive action button, uses design system `Button` primitive
  - `RevokeGrantDialog` — full modal confirmation dialog, shared by both the agent detail page (P1) and consent overview page (P2). Displays agent name, revocation consequence, and a note that connected OAuth2 services remain active. Uses design system `Dialog`/`Modal` primitive.

**Design Tokens Usage**:
- Revoke/destructive actions MUST use `error-primary` semantic token for button styling
- Dialog background and body text MUST use `neutral-*` tokens
- DO NOT use extended palettes (`red-600`, `gray-*`) or custom CSS bypasses

**Accessibility Requirements**:
- WCAG 2.1 AA compliance mandatory (4.5:1 text contrast, 3:1 UI component contrast)
- Confirmation dialog MUST trap focus until dismissed
- Destructive button MUST have an `aria-label` that includes the agent name
- Keyboard navigation: dialog dismissal via `Escape`, confirmation via `Enter`

### Key Entities

- **UserGrant**: Represents a user's delegation of OAuth2 scopes to an agent. Deletion of this entity is the revocation mechanism.
- **Agent**: The AI agent whose permissions are being revoked. Used for display name in confirmation dialogs and audit logs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A user with an active agent grant can complete full revocation from the detail page in 3 clicks or fewer (navigate → click Revoke → confirm).
- **SC-002**: A user with an active agent grant can revoke from the overview page in 2 clicks (click Revoke on card → confirm).
- **SC-003**: After revocation, the agent no longer appears with an active grant on the consent overview within one page refresh.
- **SC-004**: 100% of revocation attempts by a principal targeting another principal's grant are rejected with an authorization error.
- **SC-005**: All revocation events are captured in structured audit logs at the time of the action.
- **SC-006**: 100% of new token exchange attempts by a revoked agent on behalf of the affected user are denied after revocation. Already-issued tokens remain valid until their natural expiry (no immediate invalidation).

## Clarifications

### Session 2026-03-20

- Q: Does revoking an agent's grant also terminate the underlying OAuth2 sessions for that agent's required services? → A: No cascade. Sessions remain active. The confirmation dialog MUST explicitly inform the user that connected OAuth2 services are not affected.
- Q: Are already-issued tokens (from prior token exchanges) immediately invalidated when a grant is revoked? → A: No. Already-issued tokens remain valid until their natural expiry. Only new token exchange attempts are blocked at the point of exchange.
- Q: Is a bulk "Revoke All Agents" action (removing all grants for the user in one step) in scope? → A: No. Out of scope for this feature. Explicitly excluded in Assumptions.
- Q: Does the confirmation dialog on the overview page (P2) use the same full modal as the detail page (P1), or an inline card element? → A: Full modal dialog — same `RevokeGrantDialog` component used for both P1 and P2.
- Q: Where are revocation audit log entries written? → A: Existing structured application logger (JSON to stdout), consistent with other security events in the system.

## Assumptions

- Hard deletion of the `UserGrant` row is the revocation mechanism — no soft-delete or `revoked` status flag is introduced. If audit retention requires soft-delete, that is a separate feature.
- Revoking an agent's grant does NOT automatically terminate the underlying `UserSession` for the associated services. Users may manage sessions separately from the OAuth2 Sessions page. This follows the existing system design.
- The `DeleteUserGrant`-equivalent method is trivially addable to the `UserGrantRepository` port with no database schema migration required.
- The existing `POST /api/consent/agent/{agent-id}/grants` empty-array revocation workaround may be retained for backward compatibility but is not the primary user-facing revocation path.
- Bulk revocation ("Revoke All Agents" — removing all grants for a user in a single action) is explicitly out of scope. This can be addressed as a follow-on feature.
