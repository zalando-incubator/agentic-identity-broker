# Feature Specification: Agent Permission Requirements

**Feature Branch**: `20260108-120000-agent-permission-requirements`  
**Created**: 2026-01-08  
**Status**: Draft  
**Input**: User description: "As the next feature I want to add a feature so that the system can capture which permissions an agent requests. For each agent I want to capture which thirdparty service it needs to operate or which it can use. That translates into a mandatory and an optional requirement of a service. For each service it should capture which scopes are mandatory. The authorize flow should check that all requirements of assigned services are met. The consent screen should highlight required services, display scopes as read-only with descriptions, remove edit mode, and support redirect_uri parameter for seamless flow continuation."

## Clarifications

### Session 2026-01-08

- Q: Should the same third-party service be allowed to appear multiple times in an agent's requirements array? → A: Reject duplicates; each service_id can appear at most once (validation error if duplicate)
- Q: After user connects to a mandatory third-party service from consent screen, where should they be redirected? → A: Return to consent screen preserving the original redirect_uri context
- Q: How should scope matching work if user's session has additional scopes beyond required? → A: Superset is valid; session scopes must include all required scopes (additional scopes acceptable)
- Q: Should authorization redirect to consent screen include details about which requirements are missing? → A: No; consent screen loads requirements independently and highlights missing services on its own
- Q: Should optional services have "Connect" buttons, and which services should the consent screen display? → A: Yes, allow connection for optional services; consent screen shows only agent-configured services (mandatory + optional), not all system services
- Q: How should the "Login" button on consent screen initiate third-party OAuth2 flow? → A: Frontend redirect to existing `/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<consent-screen-url-with-original-redirect>`

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Administrator Configures Agent Service Requirements (Priority: P1)

An administrator needs to define which third-party services an agent requires (mandatory) or can optionally use (optional) to function properly. For each service requirement, the administrator specifies which OAuth2 scopes are mandatory. This configuration determines what permissions users must grant for the agent to operate.

**Why this priority**: This is the foundational capability that enables all other features. Without service requirements defined for agents, the system cannot enforce permission checks or display accurate consent information.

**Independent Test**: Administrator creates or updates an agent via the Admin API, specifying required services with mandatory scopes. The system validates the configuration, stores it, and returns the complete agent configuration including all service requirements.

**Acceptance Scenarios**:

1. **Given** an authenticated administrator, **When** creating a new agent via POST `/api/agents`, **Then** the system accepts an optional `service_requirements` array containing service requirement definitions
2. **Given** a service requirement definition, **When** the request is processed, **Then** each service requirement MUST include: `service_id` (UUID of the third-party OAuth2 service), `requirement_type` (enum: "mandatory" or "optional"), and `required_scopes` (array of scope names that must be granted)
3. **Given** a service requirement with `required_scopes`, **When** the configuration is stored, **Then** the system validates that all specified scopes exist in the referenced third-party service's scope configuration
4. **Given** an agent with service requirements, **When** the administrator retrieves the agent via GET `/api/agents/{agent-id}`, **Then** the response includes the complete `service_requirements` array with service display names resolved
5. **Given** an existing agent, **When** the administrator updates it via PUT `/api/agents/{agent-id}` with modified service requirements, **Then** the system replaces the entire service requirements configuration with the new definition
6. **Given** a service requirement referencing a non-existent service_id, **When** the request is processed, **Then** the system returns HTTP 400 with error indicating the invalid service reference
7. **Given** a service requirement with scopes not defined in the referenced service, **When** the request is processed, **Then** the system returns HTTP 400 with error listing the invalid scope names

---

### User Story 2 - Authorization Endpoint Validates Service Requirements (Priority: P1)

When a client application initiates an OAuth2 authorization flow, the system must validate not only that the user has granted consent to the agent, but also that all mandatory service requirements are satisfied. This means the user must have active sessions with all required third-party services with the necessary scopes granted.

**Why this priority**: This is critical for the core authorization flow to work correctly. If mandatory service requirements are not met, the agent cannot function, so the authorization must be blocked until the user establishes the required sessions.

**Independent Test**: Initiate an OAuth2 authorization request for an agent that has mandatory service requirements. If the user lacks active sessions for all required services with proper scopes, the system redirects to the consent screen showing what's missing rather than proxying to the upstream OAuth2 server.

**Acceptance Scenarios**:

1. **Given** an authorization request at `/oauth2/authorize` with a valid client_id, **When** the agent has mandatory service requirements, **Then** the system checks if the user has active third-party OAuth2 sessions for all mandatory services
2. **Given** a mandatory service requirement with required scopes, **When** checking session validity, **Then** the system verifies that the user's session for that service includes all required scopes (session scopes must be a superset of required scopes)
3. **Given** the user has active sessions for all mandatory services with all required scopes, **When** the system has also verified user consent exists, **Then** the authorization request is proxied to the upstream OAuth2 server
4. **Given** the user is missing a session for a mandatory service, **When** the authorization endpoint processes the request, **Then** the system redirects to `/consent/agent/:agent-id?redirect_uri=<URL-encoded original request>` (consent screen independently evaluates and highlights missing requirements)
5. **Given** the user has a session for a mandatory service but is missing required scopes, **When** the authorization endpoint processes the request, **Then** the system treats this as an unmet requirement and redirects to the consent screen
6. **Given** optional service requirements, **When** the authorization endpoint processes the request, **Then** optional services do not block the authorization flow (only mandatory services are enforced)
7. **Given** an agent with no service requirements defined, **When** the authorization endpoint processes the request, **Then** the system only checks for user consent (existing behavior preserved)

---

### User Story 3 - Consent Screen Displays Required Services (Priority: P2)

When a user is directed to the consent screen for an agent, the UI must clearly indicate which third-party services the agent requires and which are optional. Required services are highlighted to help users understand what permissions are essential for the agent to function.

**Why this priority**: Users need to understand what they're granting access to. Clear visual distinction between required and optional services improves user trust and decision-making. This depends on P1 (requirements must be configured first).

**Independent Test**: Navigate to the consent screen for an agent with both mandatory and optional service requirements. The UI displays services grouped by requirement type, with mandatory services visually highlighted and marked as required.

**Acceptance Scenarios**:

1. **Given** a user navigates to `/consent/agent/:agent-id`, **When** the agent has service requirements, **Then** the consent screen displays all required and optional third-party services and not extra services that might be configured in the system
2. **Given** a service is marked as mandatory, **When** the consent screen renders, **Then** the service is visually highlighted (with a distinct status indicator) and labeled as "Required"
3. **Given** a service is marked as optional, **When** the consent screen renders, **Then** the service is displayed with standard styling and labeled with a status indicator "Optional"
4. **Given** an agent has mandatory services, **When** displaying service requirements, **Then** mandatory services are listed before optional services in the UI
5. **Given** a mandatory service where the user lacks an active session, **When** the consent screen renders, **Then** the service shows a clear call-to-action to establish the required session (login button)
6. **Given** a mandatory service where the user has an active session with all required scopes, **When** the consent screen renders, **Then** the service shows a "Active Session" status indicator

---

### User Story 4 - Display-Only Scopes with Descriptions (Priority: P2)

The consent screen no longer allows users to select or deselect individual scopes. Instead, required scopes for each service are displayed as read-only information with helpful descriptions explaining what each scope allows. This simplifies the consent process and ensures agents receive consistent permissions.

**Why this priority**: Simplifies the user experience by removing unnecessary complexity. When scopes are predefined by administrators, users don't need to make granular scope decisions. They simply approve or decline the entire permission set.

**Independent Test**: View the consent screen for an agent with service requirements. Each service shows its required scopes as non-editable text with descriptions. No checkboxes or toggles are present for scope selection.

**Acceptance Scenarios**:

1. **Given** a user views the consent screen for an agent with service requirements, **When** the UI renders scopes, **Then** scopes are displayed as read-only text (no checkboxes, no toggles)
2. **Given** a scope has a description defined in the third-party service configuration, **When** displaying the scope, **Then** the description is shown as a tooltip or helper text
3. **Given** a service requirement has multiple required scopes, **When** the consent screen renders, **Then** all required scopes are listed under that service with their descriptions
4. **Given** a scope without a description, **When** displaying the scope, **Then** only the scope name is displayed without helper text
5. **Given** the user is on the consent screen, **When** reviewing scope information, **Then** there is no mechanism to add or remove individual scopes from what the agent requires

---

### User Story 5 - Simplified Consent Screen Without Edit Mode (Priority: P2)

The consent screen no longer has a separate "edit mode". The screen always displays the agent's requirements in an editable/actionable state. Users can connect to services and approve permissions directly without toggling between view and edit modes.

**Why this priority**: Removes UI complexity and reduces user confusion. A single-mode interface is clearer and faster for users to complete the consent flow.

**Independent Test**: Navigate to the consent screen for an agent. The UI displays service requirements with action buttons (Connect/Disconnect) directly visible without needing to enter any edit mode.

**Acceptance Scenarios**:

1. **Given** a user navigates to the consent screen, **When** the page loads, **Then** service connection actions are immediately available (no "Edit" button to enable editing)
2. **Given** a user is viewing a service they're not connected to, **When** the consent screen renders, **Then** a "Login" button is visible for that service
3. **Given** a user is viewing a service they're already connected to, **When** the consent screen renders, **Then** a "Disconnect" action is available if they want to terminate the session
4. **Given** the previous consent screen had an edit mode toggle, **When** the updated consent screen is implemented, **Then** the edit mode toggle is completely removed from the UI

---

### User Story 6 - Redirect URL for Seamless Flow Continuation (Priority: P1)

When the user completes the consent flow (by clicking "Approve" or equivalent button), if the original request included a `redirect_uri` parameter, the backend issues a redirect to that URL. This allows the OAuth2 authorization flow to continue seamlessly without requiring manual navigation.

**Why this priority**: Essential for a smooth user experience in the OAuth2 flow. Without automatic redirect, users would be stuck on the consent screen after approval, breaking the authorization flow.

**Independent Test**: Initiate an OAuth2 authorization flow that redirects to the consent screen with a `redirect_uri` parameter. After user approval, the browser is automatically redirected back to the original authorization URL, allowing the flow to continue.

**Acceptance Scenarios**:

1. **Given** a user is on the consent screen with a `redirect_uri` query parameter, **When** the user clicks the "Approve" button, **Then** the backend issues an HTTP redirect (302/303) to the URL specified in `redirect_uri`
2. **Given** the redirect is issued, **When** the browser follows the redirect, **Then** the original OAuth2 authorization flow resumes from where it left off
3. **Given** a user is on the consent screen without a `redirect_uri` parameter, **When** the user clicks the "Approve" button, **Then** the system displays a success confirmation on the consent page (no redirect)
4. **Given** a `redirect_uri` parameter is provided, **When** the backend processes approval, **Then** the system validates the redirect_uri matches the current request's origin or is a relative URL before redirecting (prevent open redirect)
5. **Given** a malicious `redirect_uri` to an external domain, **When** the backend processes approval, **Then** the system rejects the redirect and displays an error message

---

### Edge Cases

- **Agent references deleted third-party service**: When a third-party service is deleted, agents referencing it in their requirements become invalid. The authorization endpoint detects missing services and treats them as unmet requirements, redirecting to consent screen with an error indicating the service is no longer available.
- **User's session expires during consent flow**: If the user's third-party session expires while on the consent screen, the session status is re-validated on approval. If a mandatory session has expired, approval fails with an error prompting the user to re-authenticate with the required service.
- **Circular service dependencies**: Not applicable as service requirements are a flat list without dependency relationships between services.
- **Scope removed from third-party service configuration**: If a scope that agents require is removed from the service configuration, the agent's requirement validation fails at authorization time. Administrators receive an error when updating the service, warning about agents that depend on the scope being removed.
- **Empty required_scopes array**: When a service requirement has an empty required_scopes array, only the session existence is checked, not specific scopes.
- **User declines consent**: When the user clicks a "Decline" button (if present) or closes the consent screen, no redirect is issued and no grant is created. The OAuth2 flow cannot proceed until the user grants consent.
- **redirect_uri with fragment or query parameters**: The system preserves the complete redirect_uri including any query parameters when redirecting after approval, allowing the OAuth2 flow to resume with all original parameters intact.

## Requirements *(mandatory)*

### Functional Requirements

#### Agent Service Requirements Configuration

- **FR-001**: System MUST extend the Agent entity to include an optional `service_requirements` array
- **FR-002**: Each service requirement MUST contain: `service_id` (UUID), `requirement_type` (enum: "mandatory" | "optional"), and `required_scopes` (string array)
- **FR-003**: System MUST validate that `service_id` references an existing ThirdPartyOAuth2Service at creation/update time
- **FR-003a**: System MUST reject service requirements where the same service_id appears more than once (duplicate service validation error)
- **FR-004**: System MUST validate that all `required_scopes` exist in the referenced service's scope configuration
- **FR-005**: Admin API endpoint `POST /api/admin/agents` MUST accept service_requirements in the request body
- **FR-006**: Admin API endpoint `PUT /api/admin/agents/{agent-id}` MUST accept service_requirements in the request body
- **FR-007**: Admin API endpoint `GET /api/admin/agents/{agent-id}` MUST return service_requirements with resolved service display names
- **FR-008**: System MUST return HTTP 400 with descriptive error when service_id or scope validation fails

#### Authorization Endpoint Enhancement

- **FR-009**: Authorization endpoint MUST check mandatory service requirements after validating client_id and consent
- **FR-010**: For each mandatory service requirement, system MUST verify user has an active (non-expired) third-party OAuth2 session
- **FR-011**: For each mandatory service requirement, system MUST verify user's session scopes include all required_scopes (superset check)
- **FR-012**: Authorization endpoint MUST redirect to consent screen if any mandatory requirement is not satisfied
- **FR-013**: Authorization endpoint MUST proxy to upstream OAuth2 server only when all mandatory requirements are satisfied AND user consent exists
- **FR-014**: Optional service requirements MUST NOT block the authorization flow
- **FR-015**: Agents without service_requirements MUST be processed using existing consent-only logic (backward compatible)

#### Consent Screen Enhancements

- **FR-016**: Consent screen MUST display only the agent's configured service requirements (mandatory and optional), not all third-party services available in the system
- **FR-016a**: Both mandatory and optional services MUST show "Connect" buttons when user lacks an active session
- **FR-017**: Mandatory services MUST be visually distinguished from optional services (highlighted styling, "Required" label)
- **FR-018**: Mandatory services MUST be displayed before optional services in the list
- **FR-019**: For each service, system MUST show current connection status (Connected/Not Connected)
- **FR-020**: For services without active sessions, consent screen MUST show a "Login" action button that redirects to `/api/third-party/{serviceId}/oauth2/authorize`
- **FR-020a**: The "Login" button redirect_uri parameter MUST be set to the current consent screen URL including the original redirect_uri as a preserved query parameter (e.g., `/consent/agent/{agentId}?redirect_uri=<original-oauth2-authorize-url>`)
- **FR-021**: Scopes MUST be displayed as read-only text (no user selection mechanism)
- **FR-022**: Scope descriptions MUST be displayed alongside scope names when available
- **FR-023**: Consent screen MUST NOT have an edit mode toggle - actions are always available
- **FR-024**: The "Approve" button MUST be disabled until all mandatory service requirements are satisfied

#### Redirect After Approval

- **FR-025**: When user approves consent and `redirect_uri` query parameter is present, backend MUST issue HTTP 302/303 redirect to that URL
- **FR-026**: System MUST validate `redirect_uri` is same-origin or a relative URL before redirecting (prevent open redirect)
- **FR-027**: System MUST reject redirect_uri to external domains with HTTP 400 error
- **FR-028**: When `redirect_uri` is not present, approval MUST display success confirmation on the consent page
- **FR-029**: System MUST preserve all query parameters in redirect_uri when redirecting

### Domain Model

**Entities**:

- **Agent** (extended): Now includes `service_requirements` array defining which third-party services the agent needs access to and with what scopes.

**Value Objects**:

- **ServiceRequirement**: Represents a single service requirement for an agent. Contains `service_id` (UUID reference to ThirdPartyOAuth2Service), `requirement_type` (enum: "mandatory" | "optional"), and `required_scopes` (array of scope names). Immutable once set.

- **RequirementType**: Enum value object with two possible values: "mandatory" (agent cannot function without this service) and "optional" (agent can use this service if available but doesn't require it).

**Domain Events**:

- **AgentRequirementsUpdated**: Fired when an agent's service requirements are created or modified through the Admin API. Contains agent_id and the new requirements list.

*All domain terms should be added to ARCHITECTURE.md Glossary section*

### API Requirements

- **API-001**: Admin API endpoint `POST /api/admin/agents` MUST be updated to accept `service_requirements` array in request body
- **API-002**: Admin API endpoint `PUT /api/admin/agents/{agent-id}` MUST be updated to accept `service_requirements` array in request body
- **API-003**: Admin API endpoint `GET /api/admin/agents/{agent-id}` MUST be updated to return `service_requirements` with enriched service information
- **API-004**: End-user API endpoint `GET /api/consent/agent/{agent-id}` MUST return service requirements with connection status for current user
- **API-005**: End-user API endpoint `POST /api/consent/agent/{agent-id}/approve` MUST support `redirect_uri` parameter and issue redirect on success
- **API-006**: All API changes MUST be documented in respective OpenAPI specs (`/api/admin/openapi.yaml` and `/api/enduser/openapi.yaml`)
- **API-007**: Error responses for validation failures MUST include specific error codes and messages identifying which service or scope failed validation
- **API-008**: APIs MUST follow Zalando RESTful API and Event Guidelines

**Service Requirement Schema** (for OpenAPI):
```yaml
ServiceRequirement:
  type: object
  required:
    - service_id
    - requirement_type
    - required_scopes
  properties:
    service_id:
      type: string
      format: uuid
      description: UUID of the third-party OAuth2 service
    requirement_type:
      type: string
      enum: [mandatory, optional]
      description: Whether this service is required or optional for the agent
    required_scopes:
      type: array
      items:
        type: string
      description: OAuth2 scopes that must be granted for this service
```

### Database Requirements

- **DB-001**: Create migration `005_add_agent_service_requirements.up.sql` and `005_add_agent_service_requirements.down.sql`
- **DB-002**: Add `service_requirements` JSONB column to `agents` table (nullable, default NULL for backward compatibility)
- **DB-003**: The JSONB structure MUST store an array of service requirement objects with service_id, requirement_type, and required_scopes
- **DB-004**: Add index on agents.service_requirements for JSONB queries if performance testing indicates need
- **DB-005**: Migration MUST be tested for both up and down operations without data loss
- **DB-006**: Existing agents without service_requirements MUST continue to function (NULL treated as empty array)

### Security Requirements

- **SR-001**: redirect_uri validation MUST use strict same-origin checking to prevent open redirect vulnerabilities
- **SR-002**: redirect_uri MUST NOT redirect to external domains (only same-origin absolute URLs or relative URLs allowed)
- **SR-003**: Service requirement validation MUST fail closed - if any validation error occurs, the request is rejected
- **SR-004**: Authorization endpoint MUST log when mandatory requirements are not met (for security audit)
- **SR-005**: Admin API endpoints for managing service requirements MUST require admin authentication
- **SR-006**: Scope validation MUST be case-sensitive to prevent scope confusion attacks

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrators can configure agent service requirements through the Admin API within 30 seconds
- **SC-002**: Authorization endpoint validates mandatory service requirements with 100% accuracy (never proxies when requirements unmet)
- **SC-003**: Consent screen displays all service requirements with clear mandatory/optional distinction
- **SC-003**: Scope descriptions are displayed for all scopes that have descriptions configured
- **SC-004**: Invalid redirect_uri attempts are blocked with 100% effectiveness (no open redirect vulnerabilities)

## Assumptions

- Third-party OAuth2 services are already configured with scopes defined (from spec 20251217-120000)
- User session management for third-party services is operational (from spec 20251222-120000)
- OAuth2 authorization server proxy is operational (from spec 20251222-120100)
- The consent frontend (React) is operational and can be extended to support new UI requirements (from spec 20251217-120100)
- Administrators have access to the Admin API for configuring agents
- The frontend can query for user's third-party session status via existing APIs
- Scope descriptions are stored in the ThirdPartyOAuth2Service entity (from spec 20251217-120000)
- The existing consent screen component can be modified to support the new requirements display

## Out of Scope

- Dynamic scope negotiation (agents specify scopes at runtime rather than configuration time)
- Scope approval at the individual scope level (scopes are pre-configured, not user-selectable)
- Dependency relationships between services (e.g., "service A requires service B")
- Version-specific service requirements (different requirements for different agent versions)
- Service requirement templates (reusable requirement sets across agents)
- User-initiated service connection outside of consent flow (covered by existing third-party session management)
- Refresh of expired sessions during authorization (user must manually re-authenticate)
- Partial approval (user cannot approve some requirements but not others - it's all or nothing)
- Service requirement inheritance (child agents inheriting parent service requirements)

## Dependencies

- **Agent Registry** (from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)): Must be operational with Agent CRUD capabilities
- **Third-Party OAuth2 Services** (from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)): Must have services and scopes configured
- **Third-Party OAuth2 Sessions** (from [20251222-120000-thirdparty-oauth2-sessions](../20251222-120000-thirdparty-oauth2-sessions/spec.md)): Must be able to check user session status for services
- **OAuth2 Authorization Server** (from [20251222-120100-oauth2-auth-server](../20251222-120100-oauth2-auth-server/spec.md)): Authorization endpoint to be extended with requirement validation
- **Consent Frontend** (from [20251217-120100-consent-frontend](../20251217-120100-consent-frontend/spec.md)): React-based consent UI to be enhanced with new display requirements
