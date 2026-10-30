# Feature Specification: Consent Management Frontend

**Feature Branch**: `20251217-120100-consent-frontend`
**Created**: 2025-12-17
**Status**: Draft
**Input**: User description: "The consent frontend needs to be added now. It should be hosted by the existing enduser endpoint and be reachable as single page application under the path `/consent`. The page should show the currently logged in user, the list of agents to which the current user delegated their access. Using frontend routing, when the user navigates to `/consent/agent/:agent-id` the frontend should display the current grants for the agent specified by :agent-id. If no grant exist, the page should still render with the option to create a grant. From the user perspective creating and updating a grant should look the same. On this page, the agent information should be displayed. Afterwards the list of thirdparty services that the agent requested should be displayed. For each service the user should be able to toggle if the user's access in that service should be delegated to the agent. Each service should be expandable to display the scopes that the service offers. If the user does not expand a service, all scopes should be pre-selected. At the end of the page a button to Approve & Delegate should allow to create/update the user's grant to the agent."

## Clarifications

### Session 2025-12-17

- Q: What should users see while data is loading? → A: Progressive content loading with skeleton screens showing structure while fetching
- Q: Where should users navigate after successful grant submission? → A: Stay on agent grant page with success message and updated state
- Q: How should users retry failed operations? → A: Inline retry button within error message allowing immediate retry of failed operation
- Q: What accessibility requirements should be implemented? → A: No specific accessibility requirements (rely on browser/framework defaults)
- Q: How should the interface handle large numbers of third-party services? → A: Display all services without pagination (simple list, all services visible by scrolling)
- Q: How should grant validity/expiration be configured? → A: Default to indefinite (perpetual), with optional expiration date selection
- Q: What UI control should be used for grant expiration date selection? → A: Date picker (calendar control) with "No Expiration" checkbox to toggle indefinite grants

## User Scenarios & Testing

### User Story 1 - View Active Delegations (Priority: P1)

A user wants to see which agents they have granted access to and understand the current state of their delegations at a glance.

**Why this priority**: This is the entry point for all consent management activities. Users need visibility into their existing delegations before they can make informed decisions about modifying them. This is the minimum viable interface that provides transparency.

**Independent Test**: Can be fully tested by authenticating as a user, navigating to `/consent`, and verifying that the page displays the user's identity and a list of all agents to which they have delegated access. Delivers immediate value by providing transparency without requiring grant modification capabilities.

**Acceptance Scenarios**:

1. **Given** a user is authenticated, **When** they navigate to `/consent`, **Then** they immediately see skeleton screens showing the page structure while data loads
2. **Given** a user is authenticated, **When** they navigate to `/consent`, **Then** they see their own user identity (principal) displayed prominently
3. **Given** a user has delegated access to multiple agents, **When** they view the consent page, **Then** they see a list of all agents they have granted access to with each agent's display name
4. **Given** a user has not delegated access to any agents, **When** they view the consent page, **Then** they see an empty state message indicating no active delegations
5. **Given** a user views the agent list, **When** they click on an agent, **Then** they navigate to `/consent/agent/:agent-id` to view detailed grant information

---

### User Story 2 - Review Agent-Specific Grants (Priority: P2)

A user wants to review the specific services and permissions they have granted to a particular agent to understand what access that agent currently has.

**Why this priority**: Before modifying grants, users need to see current grant details. This provides transparency and informed decision-making capability. It builds on P1 (navigation from agent list) and enables P3 (grant modification).

**Independent Test**: Can be tested by navigating to `/consent/agent/{agent-id}` and verifying that the page displays the agent's information and all third-party services with their current delegation status. Works independently of the grant modification workflow - users can review without ability to change.

**Acceptance Scenarios**:

1. **Given** a user navigates to `/consent/agent/:agent-id`, **When** the page loads, **Then** they immediately see skeleton screens showing the page structure while agent and service data loads
2. **Given** a user navigates to `/consent/agent/:agent-id`, **When** the page loads, **Then** they see the agent's display name, description, and available governance/documentation links
3. **Given** a user is viewing an agent's grant page, **When** the page loads, **Then** they see a list of all third-party OAuth2 services configured in the system
4. **Given** a user has previously granted service access to an agent, **When** they view the grant page, **Then** they see which services are currently delegated with toggles showing the correct state (on/off)
5. **Given** a user has not yet granted access to an agent, **When** they navigate to `/consent/agent/:agent-id`, **Then** they see the agent information and service list with all toggles in the "off" state and the option to create a new grant
6. **Given** a service is delegated to the agent, **When** the user views it on the grant page, **Then** they can see which specific scopes are currently granted for that service

---

### User Story 3 - Manage Service Delegation (Priority: P3)

A user wants to selectively grant or revoke access to specific third-party services for an agent, with fine-grained control over permissions (scopes).

**Why this priority**: This is the core delegation management capability. While viewing grants (P2) provides transparency, this story enables users to take action and modify their consent preferences. It depends on P1 and P2 for navigation and context.

**Independent Test**: Can be tested by toggling service delegation on/off, expanding services to select specific scopes, and saving with the "Approve & Delegate" button. Verifies that user choices are captured, validated, and persisted via the backend API.

**Acceptance Scenarios**:

1. **Given** a user is viewing an agent's grant page, **When** they toggle a service delegation on, **Then** the toggle reflects the new state immediately and the service is marked for inclusion in the grant
2. **Given** a user is viewing an agent's grant page, **When** they toggle a service delegation off, **Then** the toggle reflects the new state immediately and the service is marked for exclusion from the grant
3. **Given** a user has not expanded a service, **When** they click "Approve & Delegate", **Then** all scopes for that service are included in the grant by default
4. **Given** a user expands a service, **When** the expanded view displays, **Then** they see all available OAuth2 scopes for that service with checkboxes and human-readable descriptions
5. **Given** a user has expanded a service, **When** they select specific scopes and click "Approve & Delegate", **Then** only the selected scopes are included in the grant for that service
6. **Given** a user has deselected all scopes for an expanded service, **When** they click "Approve & Delegate", **Then** the service is excluded from the grant (same as toggling the service off)
7. **Given** a user clicks "Approve & Delegate", **When** the request is processed successfully, **Then** the system creates or updates the grant via POST `/api/consent/agent/:agent-id/grants`, remains on the agent grant page, displays a success message, and refreshes the page state to show updated toggle states and granted scopes
8. **Given** a user is viewing an agent's grant page, **When** they review the grant validity section, **Then** they see a "No Expiration" checkbox (checked by default) and a date picker control
9. **Given** the "No Expiration" checkbox is checked, **When** the user views the grant validity controls, **Then** the date picker is disabled indicating the grant will be indefinite
10. **Given** a user wants to limit grant duration, **When** they uncheck the "No Expiration" checkbox, **Then** the date picker becomes enabled and allows them to select a future date as the valid_until timestamp
11. **Given** a user submits a grant with "No Expiration" checked, **When** the grant is created, **Then** the grant is set as indefinite (valid_until = null) and remains active until explicitly revoked
12. **Given** a user has an existing grant with an expiration date, **When** they return to modify it, **Then** the interface displays the "No Expiration" checkbox unchecked and the date picker showing the current expiration date
13. **Given** a user has an existing indefinite grant, **When** they return to modify it, **Then** the interface displays the "No Expiration" checkbox checked and the date picker disabled
14. **Given** a user creates a new grant, **When** they later return to modify it, **Then** the interface displays the current grant state and the experience is identical to the initial creation
15. **Given** a grant submission fails, **When** the error response is received, **Then** the system remains on the agent grant page, displays an error message with details about the failure, and provides an inline retry button to attempt the operation again
16. **Given** a failed grant submission with retry button displayed, **When** the user clicks the retry button, **Then** the system re-attempts the grant submission with the same selections without requiring the user to reconfigure their choices

---

### Edge Cases

- What happens when a user navigates to `/consent/agent/:agent-id` for an agent that does not exist? Display an error message indicating the agent was not found and provide navigation back to the consent overview.
- How does the system handle when a third-party service is no longer available or has changed its scopes since the grant was created? Display the service as unavailable with a notice, or show new scopes with clear indication that the grant needs updating.
- What happens when a user has expanded some services and left others collapsed when they click "Approve & Delegate"? System processes collapsed services with all scopes selected and expanded services with user's explicit selections.
- How does the system behave when a user toggles all services off and clicks "Approve & Delegate"? System revokes the grant by submitting empty scopes array to the API, effectively deleting the grant record.
- What happens when a user navigates directly to `/consent/agent/:agent-id` without first visiting the `/consent` overview page? The page loads independently, fetching all necessary data for that agent. User can navigate back to overview using UI navigation.
- How does the system handle API failures when loading agent or service data? Display appropriate error messages with inline retry buttons, preserve user context (no navigation away), and ensure the application doesn't crash.
- What happens when concurrent users modify the same grant? Backend handles conflict resolution (last-write-wins or optimistic locking); frontend displays success/error based on API response.
- How does the interface handle agents with very long lists of third-party services? Display all services in a simple scrollable list without pagination; performance optimization (if needed) is deferred to future iterations.
- What happens if a user selects an expiration date in the past? The backend will reject it with a validation error; the frontend should display the error and allow the user to correct it via the inline retry mechanism.
- What happens to a grant when its expiration date is reached? The backend automatically treats the grant as inactive (excludes from active grant listings); the frontend will not display it in the active grants list on the overview page.
- How does the user convert an indefinite grant to one with an expiration date? By returning to the agent grant page and selecting an expiration date, then clicking "Approve & Delegate" to update the grant.
- What happens if the authenticated session expires while the user is on the consent page? Detect authentication failure and redirect to login, preserving intent to return to consent management after re-authentication.

## Requirements

### Functional Requirements

- **FR-001**: System MUST display the authenticated user's identity (principal) on the consent management interface
- **FR-002**: System MUST display a list of all agents to which the user has active grants (derived from User Grant entities where principal matches authenticated user)
- **FR-003**: System MUST provide client-side routing to navigate to agent-specific grant details via the path pattern `/consent/agent/:agent-id`
- **FR-004**: System MUST fetch and display agent metadata (display_name, description, governance_url, user_documentation_url) via GET `/api/consent/agent/:agent-id`
- **FR-005**: System MUST fetch and display all configured third-party OAuth2 services and their available scopes via GET `/api/consent/agent/:agent-id`
- **FR-006**: System MUST display all third-party services in a simple scrollable list without pagination, showing all services simultaneously
- **FR-007**: System MUST fetch and display existing grants for the current user and agent via GET `/api/consent/agent/:agent-id/grants`
- **FR-008**: System MUST render the grant management interface even when no existing grant exists for the agent (empty grant state)
- **FR-009**: System MUST provide toggle controls for each third-party service to enable or disable delegation
- **FR-010**: System MUST support expandable service entries that reveal available OAuth2 scopes with checkboxes when expanded
- **FR-011**: System MUST pre-select all scopes for a service when the service toggle is enabled and the service is not expanded by the user
- **FR-012**: System MUST allow users to selectively choose individual scopes via checkboxes when a service is expanded
- **FR-013**: System MUST display human-readable descriptions for each OAuth2 scope (from OAuth Scope entities)
- **FR-014**: System MUST provide an "Approve & Delegate" button that submits the grant to POST `/api/consent/agent/:agent-id/grants`
- **FR-015**: System MUST construct the grant payload with delegated_oauth2_tokens containing thirdparty_oauth2_service_id and selected scopes for each enabled service
- **FR-016**: System MUST provide grant validity controls consisting of a "No Expiration" checkbox and a date picker (calendar control) for selecting expiration dates
- **FR-017**: System MUST default the "No Expiration" checkbox to checked state, indicating indefinite (perpetual) grant by default
- **FR-018**: System MUST disable the date picker when the "No Expiration" checkbox is checked, and enable it when unchecked
- **FR-019**: System MUST allow users to select a future date via the date picker as the valid_until timestamp when "No Expiration" is unchecked
- **FR-020**: System MUST include the valid_until field in the grant payload when an expiration date is selected, or omit it (resulting in null) when "No Expiration" is checked
- **FR-021**: System MUST display existing grants with expiration dates by unchecking "No Expiration" and showing the date in the date picker
- **FR-022**: System MUST display existing indefinite grants by checking "No Expiration" and disabling the date picker
- **FR-023**: System MUST handle both grant creation (no existing grant) and grant update (existing grant) with the same interface and workflow
- **FR-024**: System MUST submit an empty scopes array when all services are toggled off, effectively revoking the grant
- **FR-025**: System MUST remain on the agent grant page after successful grant submission, display a success message, and refresh the page state to reflect updated grant details
- **FR-026**: System MUST remain on the agent grant page after failed grant submission and display an error message with failure details
- **FR-027**: System MUST provide inline retry buttons within error messages for all failed operations (data loading, grant submission) to allow immediate retry without losing user context or selections
- **FR-028**: System MUST be hosted as a single page application under the `/consent` path on the existing enduser endpoint
- **FR-029**: System MUST handle client-side routing without full page reloads when navigating between `/consent` and `/consent/agent/:agent-id`
- **FR-030**: System MUST display clickable links to agent governance_url and user_documentation_url when these fields are populated
- **FR-031**: System MUST display skeleton screens showing page structure immediately while data is loading, before actual content is available

### Security Requirements

- **SR-001**: System MUST verify user authentication before displaying any consent management interface
- **SR-002**: System MUST ensure users can only view and modify their own grants (API enforces principal matching)
- **SR-003**: System MUST handle authentication failures gracefully by redirecting to login with return path
- **SR-004**: System MUST validate agent_id from URL path before making API requests
- **SR-005**: System MUST sanitize and validate all URLs (governance_url, user_documentation_url) before rendering as clickable links to prevent XSS
- **SR-006**: System MUST use HTTPS for all API communication (enforced by backend, frontend assumes secure transport)
- **SR-007**: System MUST handle API authorization errors (403/401) by displaying appropriate error messages
- **SR-008**: System MUST not store sensitive grant data in browser local storage or cookies (fetch fresh on each session)
- **SR-009**: System MUST protect against CSRF attacks when submitting grants (use existing CSRF protection mechanisms)

### Key Entities

*References domain model from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)*

- **Agent**: AI agent registered in the identity broker. Frontend displays agent's display_name, description, governance_url, user_documentation_url, and agent_interface_url.

- **Thirdparty OAuth2 Service**: External OAuth2 provider configuration. Frontend displays display_name and available scopes for each service.

- **OAuth Scope**: Permission scope within a third-party OAuth2 service. Frontend displays scope_value and human-readable description to help users make informed decisions.

- **User Grant**: Record of a user (principal) delegating specific permissions to an agent. Frontend creates grants with principal (from session), agent_id (from URL), and delegated_oauth2_tokens array (from user selections). Each delegated token contains thirdparty_oauth2_service_id and array of granted scope values.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Users can view all their active agent delegations in under 3 seconds after navigating to `/consent`
- **SC-002**: Users can navigate to agent-specific grant details and see all available services in under 2 seconds
- **SC-003**: Users can create or update a grant with multiple service selections in under 30 seconds
- **SC-004**: 95% of users successfully complete grant approval on their first attempt without validation errors
- **SC-005**: Users understand the difference between collapsed (all scopes) and expanded (selective scopes) service delegation without additional help documentation
- **SC-006**: Grant modifications are reflected in the interface within 1 second of successful API response
- **SC-007**: The interface correctly handles agents with no existing grants, displaying the creation flow without errors or user confusion
- **SC-008**: Application loads and renders initial view within 2 seconds on standard broadband connection
- **SC-009**: Users can manage grants for agents with 20+ third-party services without performance degradation
- **SC-010**: 90% of users correctly identify which services they have delegated by looking at the consent overview page

## Scope

### In Scope

- Single page application interface for consent management
- Consent overview page showing user identity and list of delegated agents
- Agent-specific grant detail page with service and scope selection
- Service-level delegation toggles
- Scope-level selection when services are expanded
- Grant validity/expiration configuration (optional expiration date or indefinite/perpetual grants)
- Create and update grant workflows with unified interface
- Display of agent information and governance links
- Error handling and user feedback for API operations
- Client-side routing between consent pages

### Out of Scope

- Agent registration or management interfaces (admin-only, separate feature)
- Third-party service configuration interfaces (admin-only, see [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md))
- Third-party service connection or OAuth2 authentication flows (handled by agents, not end users)
- Grant revocation history or audit trail visualization
- Bulk grant management across multiple agents
- Email or notification systems for grant changes
- Mobile-specific responsive design optimizations (assumes standard responsive web design)
- Multi-language internationalization (assumes English-only in this phase)
- Real-time updates when grants are modified (requires manual page refresh)
- Agent search or filtering on the consent overview page

## Dependencies

- **Backend APIs** (from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)):
  - GET `/api/consent/agent/:agent-id` - Fetch agent info and available services/scopes
  - GET `/api/consent/agent/:agent-id/grants` - Fetch existing grants for the user
  - POST `/api/consent/agent/:agent-id/grants` - Create or update grant
- **Existing enduser endpoint** must be operational and capable of hosting a single page application
- **User authentication system** must provide session management and principal extraction
- **Agent registry** (from 006) must have agents configured for users to grant access to
- **Third-party OAuth2 service configurations** (from 006) must exist for users to delegate services

## Assumptions

- Users are already authenticated before accessing the consent interface
- The system already has agents and third-party services configured by administrators (via 006 APIs)
- Service and scope information is current and accurate (backend responsibility)
- Users have a basic understanding of what "agents" are in the context of this system
- The enduser endpoint supports client-side routing for single page applications (e.g., URL rewriting, history API support)
- Default behavior (all scopes when service not expanded) aligns with user expectations and security requirements
- Visual design system and component library exist for building the UI
- Users access the application via modern web browsers (last 2 major versions of Chrome, Firefox, Safari, Edge)
- API responses conform to standard JSON format with consistent error structures
- Backend enforces all security policies (frontend is untrusted client)
- Grant updates via POST with upsert semantics (backend handles create vs. update logic)
- Accessibility relies on browser and framework defaults without specific WCAG compliance requirements in this phase
