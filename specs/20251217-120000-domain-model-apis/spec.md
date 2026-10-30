# Feature Specification: Domain Model and Consent APIs

**Feature Branch**: `20251217-120000-domain-model-apis`
**Created**: 2025-12-17
**Status**: Draft
**Input**: User description: "Let's define the core domain model of the identity broker with entities (Agent, Thirdparty OAuth2 Service, User Grant) and APIs for admin operations and user consent management."

## Clarifications

### Session 2025-12-17

- Q: Is User Story 3 describing a user interface or an API endpoint? → A: API endpoint that emits and accepts JSON (not a user interface)
- Q: How is the relationship between agents and third-party OAuth2 services configured? → A: Implicit - all configured third-party services are available to all agents (agent-to-service relationship deferred to future "content" domain feature)
- Q: How is the OAuth2 discovery metadata URL determined? → A: Standard well-known path from issuer_uri with optional override via metadata_url field
- Q: At what level is grant duration configured? → A: User-selectable at grant creation time; users can specify valid_until timestamp or omit it for indefinite grants; only validation is that specified date must be in the future
- Q: What happens to grants when a referenced third-party OAuth2 service is deleted? → A: Block deletion if grants reference the service; admin must clean up grants first; maintains referential integrity

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Administrator Manages Agent Registry (Priority: P1)

An administrator needs to register and configure AI agents in the identity broker system, providing essential metadata like display names, descriptions, OAuth2 client credentials, and governance URLs. This foundational capability enables all downstream consent workflows.

**Why this priority**: Without registered agents, there is nothing for users to grant permissions to. This is the foundational data that enables the consent flow.

**Independent Test**: Administrator can create a new agent with all required fields (ID, OAuth2 client_id, display_name, description) and optional fields (external_id, governance_url, user_documentation_url, agent_interface_url), retrieve it by ID, update any field, and delete it. Success is verified by confirming the agent appears in subsequent API calls and that deleted agents are no longer accessible.

**Acceptance Scenarios**:

1. **Given** administrator is authenticated, **When** they submit a POST request to `/api/agents` with valid agent data, **Then** system creates the agent with a unique ID and returns the complete agent record
2. **Given** an agent exists, **When** administrator requests GET `/api/agents/:agent-id`, **Then** system returns the complete agent configuration including all metadata
3. **Given** an agent exists, **When** administrator updates the agent via PUT `/api/agents/:agent-id`, **Then** system updates the specified fields and returns the updated record
4. **Given** an agent exists, **When** administrator deletes it via DELETE `/api/agents/:agent-id`, **Then** system removes the agent and returns confirmation
5. **Given** invalid agent data is submitted, **When** administrator attempts to create or update an agent, **Then** system returns validation errors identifying the specific issues

---

### User Story 2 - Administrator Configures Third-Party OAuth2 Services (Priority: P1)

An administrator needs to configure external OAuth2 providers (Google, GitHub, Databricks, etc.) that agents will access on behalf of users. Configuration includes credentials, endpoints, and available scopes with descriptions.

**Why this priority**: Third-party service configurations are required before users can delegate access. Like agent management, this is foundational data. Both P1 stories can be developed in parallel.

**Independent Test**: Administrator can create a third-party OAuth2 service with all required fields (display_name, client_id, client_secret, issuer_uri, endpoints) and optional fields (discovery settings, scopes), retrieve it by ID, update any field, and delete it. System automatically discovers OAuth2 endpoints when discovery is enabled.

**Acceptance Scenarios**:

1. **Given** administrator is authenticated, **When** they submit a POST request to `/api/third-party/oauth2/clients` with valid service configuration, **Then** system creates the service configuration with a unique ID
2. **Given** a service configuration has discovery enabled, **When** it is created or updated, **Then** system constructs well-known metadata URL from issuer_uri (or uses provided metadata_url override) and automatically discovers and populates OAuth2 endpoints
3. **Given** a service exists, **When** administrator requests GET `/api/third-party/oauth2/clients/:client-id`, **Then** system returns the complete service configuration (with client_secret redacted in responses)
4. **Given** a service exists, **When** administrator updates it via PUT `/api/third-party/oauth2/clients/:client-id`, **Then** system updates the specified fields and re-runs discovery if enabled
5. **Given** a service exists with no active grants referencing it, **When** administrator deletes it via DELETE `/api/third-party/oauth2/clients/:client-id`, **Then** system removes the service configuration
6. **Given** a service has active grants referencing it, **When** administrator attempts to delete it via DELETE `/api/third-party/oauth2/clients/:client-id`, **Then** system blocks the deletion and returns error indicating grants must be removed first
7. **Given** administrator configures scopes for a service, **When** the configuration is saved, **Then** system stores both scope values and human-readable descriptions

---

### User Story 3 - User Reviews Agent Information Before Granting Access (Priority: P2)

A client application needs to retrieve detailed information about an agent to present to the user before they grant access. The API returns JSON containing the agent's display name, description, documentation links, and the specific third-party services and scopes the agent requests. This data enables the client to build an informed consent interface.

**Why this priority**: Users must understand what they're granting before consenting. This depends on P1 (agents and services must exist) but is independent of the actual grant creation (P3).

**Independent Test**: Client application makes a GET request to `/api/consent/agent/:agent-id` and receives a JSON response containing all relevant information including agent metadata, available third-party services, and detailed scope descriptions. The response structure allows the client to present this information to users for decision-making.

**Acceptance Scenarios**:

1. **Given** client application is authenticated and an agent exists, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** system returns JSON response with agent display_name, description, governance_url, user_documentation_url, and agent_interface_url
2. **Given** third-party OAuth2 services are configured in the system, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** JSON response includes a list of all configured third-party services available for delegation with their display names
3. **Given** third-party services are configured, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** JSON response includes all available OAuth2 scopes for each service with both scope_value and description fields
4. **Given** agent or services don't exist, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** system returns JSON error response with appropriate HTTP status (404 for not found)
5. **Given** agent has governance_url configured, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** JSON response includes governance_url field
6. **Given** agent has user_documentation_url configured, **When** GET `/api/consent/agent/:agent-id` is requested, **Then** JSON response includes user_documentation_url field

---

### User Story 4 - User Grants and Manages Permissions to Agents (Priority: P3)

A user needs to grant an agent permission to access specific third-party services and scopes on their behalf. The user should be able to create new grants, view existing grants, modify granted scopes, and revoke access entirely.

**Why this priority**: This is the core consent functionality. It depends on both P1 (agents and services must exist) and P2 (users should review agent info first), but delivers the primary business value of the feature.

**Independent Test**: User creates a new grant for an agent specifying which third-party services and scopes to delegate, views their active grants, modifies the scopes of an existing grant, and revokes a grant. System correctly enforces that only the grant owner can view or modify their grants.

**Acceptance Scenarios**:

1. **Given** user is authenticated and reviewing an agent, **When** user submits POST `/api/consent/agent/:agent-id/grants` with selected services and scopes, **Then** system creates a new grant record with the user's principal derived from session
2. **Given** user has active grants, **When** user requests GET `/api/consent/agent/:agent-id/grants`, **Then** system returns all grants for this agent where the user is the principal, including granted services, scopes, and expiration timestamps (or null for indefinite grants)
3. **Given** user has an existing grant, **When** user submits POST `/api/consent/agent/:agent-id/grants` with modified scopes, **Then** system updates the existing grant (one grant per user-agent pair)
4. **Given** user submits a grant with invalid scopes, **When** system validates the request, **Then** system rejects the grant and returns validation errors identifying non-existent scopes
5. **Given** user creates a grant with valid_until timestamp, **When** grant is saved, **Then** system stores the user-provided valid_until value if it's in the future, or stores null if valid_until is omitted (indefinite grant)
6. **Given** user provides valid_until in the past, **When** grant is submitted, **Then** system rejects the grant with validation error indicating timestamp must be in the future
7. **Given** user wants to revoke access, **When** user submits POST `/api/consent/agent/:agent-id/grants` with empty scopes array, **Then** system deletes the grant record
8. **Given** grant has expired (current time > valid_until), **When** system checks grant validity, **Then** system treats the grant as inactive and excludes it from active grants list
9. **Given** user A creates a grant, **When** user B attempts to view or modify it, **Then** system denies access and returns authorization error

---

### Edge Cases

- What happens when an agent is deleted but has active grants? System should cascade delete all associated grants and log the cleanup operation for audit purposes.
- What happens when a third-party OAuth2 service has active grants referencing it? System blocks deletion and returns error message indicating that grants must be revoked or modified first. Admin must clean up grants before removing service to maintain referential integrity.
- How does system handle expired grants? System filters expired grants (valid_until < current_time) from active grant listings. Indefinite grants (valid_until = null) never expire and remain active until explicitly revoked.
- What happens when third-party OAuth2 service configuration is invalid or endpoints are unreachable? System returns appropriate error during admin configuration but does not block grant creation (services may be temporarily unavailable).
- How does system handle concurrent grant modifications by the same user? System uses optimistic locking or last-write-wins semantics with proper conflict detection.
- What happens when user tries to grant scopes that don't exist in the service configuration? System validates all scopes against registered service configurations and rejects the grant with detailed error messages.
- What happens when OAuth2 endpoint discovery fails? System logs the failure and either falls back to manually configured endpoints (if provided) or returns an error requiring manual endpoint configuration. Discovery attempts standard well-known path first; if metadata_url override is provided, uses that instead.
- How does system handle client secret rotation for third-party services? Administrator updates the service configuration with new client_secret; active grants continue to work as tokens are obtained using current credentials.
- What happens when a user has no active grants for an agent? System returns an empty grants array with 200 OK status (not an error condition).

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST store agent configurations including unique ID, OAuth2 client_id, external_id, display_name, description, governance_url, user_documentation_url, and agent_interface_url
- **FR-002**: System MUST generate unique IDs for agents and grants upon creation
- **FR-003**: System MUST store third-party OAuth2 service configurations including unique ID, display_name, client_id, client_secret, issuer_uri, discovery settings, OAuth2 endpoints, and available scopes
- **FR-004**: System MUST support OAuth2 endpoint discovery when discovery is enabled by constructing standard well-known metadata URL from issuer_uri (e.g., `{issuer}/.well-known/oauth-authorization-server`) or using explicit metadata_url override if provided
- **FR-005**: System MUST fall back to manually configured endpoints when discovery is disabled or fails
- **FR-006**: Admins MUST be able to create, read, update, and delete agent configurations via `/api/agents/:agent-id`
- **FR-007**: Admins MUST be able to create, read, update, and delete third-party OAuth2 service configurations via `/api/third-party/oauth2/clients/:client-id`
- **FR-008**: System MUST redact client_secret values in all API responses (only accept them in create/update operations)
- **FR-009**: Users MUST be able to view agent information including display name, description, governance URL, documentation URL, and available third-party services via GET `/api/consent/agent/:agent-id`
- **FR-010**: GET `/api/consent/agent/:agent-id` endpoint MUST return JSON response containing agent metadata, list of third-party services, and their available OAuth2 scopes with descriptions
- **FR-011**: Users MUST be able to create grants delegating specific OAuth2 scopes to agents via POST `/api/consent/agent/:agent-id/grants`
- **FR-012**: Users MUST be able to view their existing grants for an agent via GET `/api/consent/agent/:agent-id/grants`
- **FR-013**: Users MUST be able to modify existing grants by submitting a new grant request (upsert semantics)
- **FR-014**: Users MUST be able to revoke grants by submitting an empty scopes array
- **FR-015**: System MUST enforce one active grant per user-agent pair (subsequent grants update the existing record)
- **FR-016**: System MUST accept optional user-provided valid_until timestamp when creating grants; if omitted, grant has no expiration (stored as null); if provided, system MUST validate that timestamp is in the future
- **FR-017**: System MUST derive the principal from the authenticated session for all user operations
- **FR-018**: System MUST validate that all requested scopes exist in the corresponding third-party service configuration before accepting a grant
- **FR-019**: System MUST filter expired grants (valid_until < current_time) from active grant listings; indefinite grants (valid_until = null) MUST always be included in active grant listings
- **FR-020**: System MUST validate agent_id and thirdparty_oauth2_service_id references when creating grants
- **FR-021**: System MUST cascade delete grants when an associated agent is deleted
- **FR-022**: System MUST block deletion of third-party OAuth2 services that have active grants referencing them; deletion request MUST return error with list of affected grants or count
- **FR-023**: System MUST provide detailed validation error messages identifying specific field issues
- **FR-024**: All API endpoints MUST accept JSON request bodies (for POST/PUT operations) and return JSON responses with appropriate Content-Type headers
- **FR-025**: GET `/api/consent/agent/:agent-id` endpoint MUST return all configured third-party OAuth2 services and their scopes regardless of which agent is being queried

### Security Requirements

- **SR-001**: Admin APIs (`/api/agents/*`, `/api/third-party/oauth2/clients/*`) MUST be protected and accessible only to authenticated administrators with appropriate permissions
- **SR-002**: Client secrets for third-party OAuth2 services MUST be encrypted at rest
- **SR-003**: Client secrets MUST be redacted from all API responses (return masked values or omit entirely)
- **SR-004**: User grant APIs MUST validate that the authenticated principal matches the grant owner for all view and modify operations
- **SR-005**: System MUST prevent privilege escalation by ensuring users can only grant scopes from configured services
- **SR-006**: System MUST fail closed if session principal extraction fails (reject request rather than proceeding with undefined principal)
- **SR-007**: System MUST emit structured audit logs for all admin operations (agent creation, modification, deletion, service configuration changes)
- **SR-008**: System MUST emit structured audit logs for all grant operations (grant creation, modification, revocation)
- **SR-009**: OAuth2 endpoint discovery MUST validate SSL certificates and reject self-signed certificates in production
- **SR-010**: System MUST prevent cross-user grant access by always filtering grants by session principal
- **SR-011**: System MUST sanitize and validate all URLs (governance_url, user_documentation_url, agent_interface_url, metadata_url) to prevent injection attacks
- **SR-012**: System MUST implement rate limiting on admin and user APIs to prevent abuse

### Key Entities

- **Agent**: Represents an AI agent registered in the identity broker. Contains OAuth2 client configuration (client_id), optional external governance system identifier (external_id), display metadata (display_name, description), and optional URLs (governance_url, user_documentation_url, agent_interface_url) for user information and transparency.

- **Thirdparty OAuth2 Service**: External OAuth2 provider (e.g., Google, GitHub, Databricks) that agents will access on behalf of users. Contains display_name, OAuth2 credentials (client_id, client_secret), issuer_uri, discovery configuration (enable_discovery, metadata_url), OAuth2 endpoints (token_endpoint, authorize_endpoint), and available OAuth2 scopes with descriptions.

- **OAuth Scope**: Permission scope within a third-party OAuth2 service. Contains scope_value (used in OAuth2 protocol) and human-readable description explaining what the scope allows.

- **User Grant**: Record of a user (principal) delegating specific permissions to an agent for one or more third-party OAuth2 services. Contains unique ID, principal identifier (derived from session), agent_id reference, optional valid_until timestamp (null for indefinite grants), and set of delegated_oauth2_tokens (each containing thirdparty_oauth2_service_id and granted scopes).

*Domain concepts should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Administrators can register a new agent with all required fields in under 2 minutes
- **SC-002**: Administrators can configure a third-party OAuth2 service with automatic endpoint discovery in under 3 minutes
- **SC-003**: API returns complete agent information with all third-party services and scopes in under 2 seconds
- **SC-004**: Users can grant permissions to an agent in under 2 minutes from first seeing the consent screen
- **SC-005**: System correctly filters expired grants (valid_until < current_time) from active listings within 1 minute of expiration; indefinite grants remain active indefinitely
- **SC-006**: 95% of OAuth2 endpoint discovery attempts succeed on first try when metadata URL is valid
- **SC-007**: Users successfully complete the grant creation flow on first attempt 90% of the time
- **SC-008**: Grant modification (adding/removing scopes) completes in under 30 seconds
- **SC-009**: System handles 100 concurrent grant creation requests without errors or significant latency degradation

## Assumptions *(optional)*

- **Session Management**: Existing session management middleware provides authenticated principal extraction for user APIs
- **Admin Authentication**: Existing authentication and authorization infrastructure protects admin endpoints
- **Persistence Layer**: Database or persistence layer is available and supports CRUD operations with transactional integrity
- **UUID Generation**: Platform provides UUID generation capabilities
- **Grant Cardinality**: One active grant per user-agent pair; subsequent grants update/replace existing grants rather than creating duplicates
- **Cascade Deletion**: When an agent is deleted, all associated grants are automatically deleted
- **Client Secret Storage**: Infrastructure provides secure encryption at rest for sensitive credentials
- **OAuth2 Standard Compliance**: Third-party services follow standard OAuth2 well-known metadata conventions (e.g., `{issuer}/.well-known/oauth-authorization-server`); non-standard providers can specify explicit metadata_url override
- **Network Access**: System has network access to reach third-party OAuth2 metadata URLs for discovery
- **Error Handling**: Standard HTTP status codes and error response format are established
- **API Versioning**: APIs follow existing versioning and routing conventions in the identity broker
- **Agent-Service Relationship**: All configured third-party OAuth2 services are available to all agents (no per-agent service restrictions in this phase; future "content" domain feature will add granular agent-to-service associations)
