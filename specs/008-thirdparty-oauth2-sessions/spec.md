# Feature Specification: Third-Party OAuth2 Session Management

**Feature Branch**: `008-thirdparty-oauth2-sessions`  
**Created**: 2025-12-22  
**Status**: Draft  
**Input**: User description: "Users need to be able to login to third-party services and the identity broker will manage the users' sessions and store their respective OAuth2 tokens."

## Clarifications

### Session 2025-12-22

- Q: What happens when user initiates multiple OAuth2 flows for the same service simultaneously (race condition)? → A: Use database unique constraint (principal, service_id); first successful callback wins, subsequent callbacks see existing session and skip token storage
- Q: How does system handle third-party authorization endpoint returning an error instead of authorization code? → A: Parse OAuth2 error response (error, error_description); redirect to sessions page with user-friendly error message displayed; allow immediate retry
- Q: How does system handle network failures during token exchange? → A: Retry token exchange up to 3 times with exponential backoff (1s, 2s, 4s); if all retries fail, display error message allowing user to retry OAuth2 flow
- Q: What happens when access token expires but refresh token is still valid? → A: Show session as expired only if refresh token is expired. Expired access tokens will be refreshed in a future iteration transparently to the user
- Q: What happens if a third-party service is deleted while user sessions exist? → A: Block service deletion with error message listing active session count; admin must manually terminate all user sessions before deleting service

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View Available Third-Party Sessions (Priority: P1)

A user needs to see which third-party services have active sessions with the identity broker and understand their current session status for each service (whether they're logged in, when the session was initiated, and how many agents depend on it).

**Why this priority**: This is the foundational capability that provides users visibility into available services and their authentication status. Without this view, users cannot discover which third-party services have been delegated to agents.

**Independent Test**: User navigates to the "Third-party Sessions" page and sees a list of all registered third-party OAuth2 services that have sessions with status indicators showing when the session was initiated, how many agents use it, and that tokens are stored encrypted and if the session has expired.

**Acceptance Scenarios**:

1. **Given** user is authenticated and third-party services are configured, **When** user navigates to the third-party sessions page, **Then** system displays a list of all configured third-party OAuth2 services with their display names and descriptions if they have an active session
2. **Given** user has established a session with a service, **When** viewing the service card, **Then** system displays session initiation timestamp, number of agents using this session, expiry date of the refresh token and an encryption status indicator
4. **Given** user views an established session, **When** reviewing the service card, **Then** system shows a "Terminate Session" button
5. **Given** multiple agents depend on a session, **When** user views the session card, **Then** system displays the count of dependent agents
6. **Given** user has an established session where refresh token has expired, **When** viewing the service card, **Then** system displays session with "Expired" status indicator and user must terminate and re-authenticate to establish new session. To re-authenticate show a login button instead of the Terminate button.

---

### User Story 2 - Establish OAuth2 Session via Authorization Code Flow (Priority: P2)

A user needs to authenticate with a third-party service by initiating an OAuth2 authorization code flow with PKCE. The system securely manages the OAuth2 ceremony, redirects the user to the third-party's authorization endpoint, and handles the callback to obtain access and refresh tokens.

**Why this priority**: This enables users to actually create sessions with third-party services. It depends on P1 (users must see available services) and delivers the core authentication capability.

**Independent Test**: Navigating to `/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<the url of the session page>` gets redirected to third-party authorization page, approves access, returns to the broker, and sees their session established with tokens stored securely.

**Acceptance Scenarios**:

1. **Given** user navigates to the `/api/third-party/{serviceId}/oauth2/authorize?redirect_uri=<the url of the session page>` endpoint, **When** system initiates OAuth2 flow, **Then** system generates PKCE code verifier and challenge, creates signed state token (JWE), and redirects user to the third-party's authorization endpoint with appropriate parameters
2. **Given** system initiates OAuth2 flow, **When** building authorization URL, **Then** system includes client_id (from service configuration), redirect_uri (callback endpoint on broker), response_type=code, code_challenge (PKCE), code_challenge_method=S256, scope (configured scopes), and state (JWE token)
3. **Given** user approves access at third-party authorization page, **When** third-party redirects back to broker callback endpoint, **Then** system validates the state token, extracts PKCE verifier, and exchanges authorization code for access and refresh tokens
4. **Given** callback receives authorization code, **When** system exchanges code for tokens, **Then** system validates that current principal matches principal in state token, validates PKCE verifier, and validates service ID matches
5. **Given** tokens are successfully obtained, **When** system stores them, **Then** system encrypts tokens using EncryptionPort, associates them with the user's principal and service ID, records session initiation timestamp, and stores them in the token vault
6. **Given** OAuth2 flow completes successfully, **When** user returns to the sessions page, **Then** system displays the newly established session with status indicators
7. **Given** state token validation fails at callback, **When** system detects mismatch, **Then** system rejects the callback with error and does not store any tokens
8. **Given** third-party returns OAuth2 error in callback (e.g., access_denied, invalid_scope), **When** system processes callback, **Then** system parses error and error_description parameters, redirects user to sessions page with user-friendly error message, and Login button remains available for retry
9. **Given** token exchange request fails due to network error, **When** system attempts token exchange, **Then** system retries up to 3 times with exponential backoff (1s, 2s, 4s), and if all retries fail, displays error message allowing user to restart OAuth2 flow

---

### User Story 3 - Terminate Third-Party Session with Warnings (Priority: P3)

A user needs to revoke their session with a third-party service, which removes stored tokens and breaks the connection. Before terminating, the system warns the user about which agents will lose access to this service.

**Why this priority**: This provides users control over their sessions and security. It depends on P2 (sessions must exist to be terminated) and adds safety through warnings about affected agents.

**Independent Test**: User clicks "Terminate Session" button, sees warning dialog listing affected agents, confirms termination, and sees session removed with tokens deleted from storage.

**Acceptance Scenarios**:

1. **Given** user has an established session, **When** user clicks "Terminate Session" button, **Then** system displays a warning dialog listing all agents that currently use this session
2. **Given** warning dialog shows affected agents, **When** user confirms termination, **Then** system deletes stored tokens (access token, refresh token) from token vault and removes session record
3. **Given** session is terminated, **When** user returns to sessions page, **Then** service card shows "Login" button again with no session status indicators
4. **Given** agents previously used a terminated session, **When** those agents attempt to use the session, **Then** agents detect missing session and must request user to re-authenticate
5. **Given** user cancels termination in warning dialog, **When** dialog is dismissed, **Then** session remains active and no changes are made

---

### User Story 4 - Secure State Token Management (Priority: P2)

The system needs to securely manage OAuth2 state parameters during the authorization flow by creating and validating JWE tokens that bind the flow to the current user and protect against CSRF attacks.

**Why this priority**: This is a critical security mechanism that runs alongside P2 (session establishment). It's not independently testable from a user perspective but is essential for secure OAuth2 flow implementation.

**Independent Test**: System creates JWE state tokens containing principal, PKCE verifier, service ID, and redirect_uri; validates all parameters on callback; and rejects mismatched or tampered tokens.

**Acceptance Scenarios**:

1. **Given** system initiates OAuth2 flow, **When** creating state token, **Then** system generates JWE with claims: principal (current user), pkce_verifier (PKCE code verifier), service_id (third-party service identifier), redirect_uri (original redirect_uri parameter from authorize endpoint)
2. **Given** state token is created, **When** signing JWE, **Then** system uses authenticated encryption to prevent tampering and ensure confidentiality
3. **Given** callback receives state token, **When** validating token, **Then** system verifies JWE signature, decrypts claims, validates principal matches current authenticated user, validates service_id matches the callback endpoint parameter, and extracts PKCE verifier
4. **Given** state token validation detects principal mismatch, **When** system processes callback, **Then** system rejects callback with 403 Forbidden and logs security event
5. **Given** state token validation detects service_id mismatch, **When** system processes callback, **Then** system rejects callback with 400 Bad Request
6. **Given** state token is expired or tampered, **When** system attempts to decrypt, **Then** decryption fails and system rejects callback with 400 Bad Request

---

### User Story 5 - Preserve Consent Selections Across Third-Party OAuth2 Redirects (Priority: P2)

The consent page encodes active permission set selections into the `redirect_uri` before initiating a third-party OAuth2 login, so that selections survive the redirect round-trip and are restored when the user returns.

**Why this priority**: Without this, any third-party OAuth2 login from the consent page discards the user's optional permission set toggle state, requiring the user to re-select their choices after every service login.

**Independent Test**: User toggles optional permission sets on the consent page, clicks Login for a service, and after the OAuth2 redirect round-trip the consent page re-renders with the same permission set selections active.

**Acceptance Scenarios**:

1. **Given** the user has active permission set selections on the consent page, **When** the user initiates a third-party OAuth2 login for a service, **Then** the consent page encodes the current selections as `consent_state` (base64url JSON) in the `redirect_uri` so they survive the OAuth2 redirect round-trip.
2. **Given** the OAuth2 callback redirects back to the consent page with a `consent_state` URL parameter, **When** the consent page renders, **Then** the page restores the permission set selection state from `consent_state`, re-selecting optional permission sets that were active before the redirect.
3. **Given** a user completes a full OAuth2 redirect round-trip (consent → third-party OAuth2 → callback → consent), **When** the consent page re-renders after the callback, **Then** permission set selections from before the redirect are fully restored and the corresponding service connections remain visible.

### User Story 5 Amendment - Compact Consent Selection References (2026-09-22)

This amendment supersedes only User Story 5's URL-encoded `consent_state` transport. Feature 008 remains authoritative for third-party OAuth session initiation, JWE and PKCE validation, token exchange, token storage, callback errors, and all unrelated session behavior.

The consent page stores the selection map in current-tab `sessionStorage`. When selections exist, it submits the bare UUID `consent_state_id` and the return URL in a same-origin form POST to the service-login endpoint. The broker seals `consent_state_id` into the third-party state JWE. `consent_state_id` is not a URL or provider authorization parameter. The existing `redirect_uri` JWE claim contains the protected return URL, which contains no selection state.

**Acceptance Scenarios**:

1. **Scenario 1: Compact reference creation. Given** a consent page has a large selection map, **When** the user starts a service login, **Then** the page stores the map under a UUID reference, sends it as `consent_state_id` in a same-origin form POST, and the provider-facing `state` JWE contains that claim. The nested return URL contains no `consent_state_id`, `consent_state`, or selection JSON, and provider-facing `state` is less than 6,000 bytes.
2. **Scenario 2: Same-tab restoration. Given** a callback returns after a JWE with a valid `consent_state_id` claim, **When** the consent page renders in the originating tab, **Then** it restores the validated pending selection map from current-tab storage, including selected optional permission sets and their available service cards.
3. **Scenario 3: Complete provider callback. Given** the user starts a service login with optional selections, **When** the mock provider completes the callback in the same tab, **Then** the broker returns callback parameters without a selection-state URL parameter, and the page restores the selected optional permission set from current-tab storage.

**Edge Cases**:

- A missing, malformed, expired, or unreadable reference uses existing grant or default selections. The callback continues.
- If browser storage cannot save a non-empty selection map, the page does not navigate. It shows `Unable to preserve selections. Please try again.`.
- If no selections exist, the page clears its pending selection reference and starts the existing flow.
- A valid record stays until it expires. A page reload in the originating tab can restore it.
- A callback in another browser tab uses existing grant or default selections.

---

### Edge Cases

- **Multiple simultaneous OAuth2 flows for same service**: Database unique constraint on (principal, service_id) ensures first successful callback wins. Subsequent callbacks detect existing session and skip token storage, preventing race conditions and token corruption.
- **Third-party authorization errors**: When third-party returns OAuth2 error (e.g., access_denied, invalid_scope, server_error), system parses error and error_description parameters from callback URL, redirects user to sessions page with user-friendly error message, and allows immediate retry via Login button.
- **Network failures during token exchange**: System retries token exchange request up to 3 times with exponential backoff (1s, 2s, 4s delays). If all retries fail, displays error message to user allowing them to retry the entire OAuth2 flow from the beginning.
- **Expired access token with valid refresh token**: Session remains active and is not marked as expired. Future iteration will implement automatic transparent token refresh for agents. Only when refresh token itself expires should session be marked as expired in UI.
- **Third-party service deletion with active sessions**: System blocks service deletion and returns error message showing count of active user sessions. Admin must manually terminate all user sessions before service can be deleted (enforced by ON DELETE RESTRICT foreign key constraint).
- **Malformed or missing callback parameters**: When third-party returns malformed callback (missing code or state), system redirects to sessions page with error message "Invalid callback parameters" and allows retry via Login button. Missing code treated same as access_denied error.
- **Redirect URI domain mismatch**: When redirect_uri parameter in authorize request does not match the Host header of the incoming request, system returns 400 Bad Request with error "invalid_redirect_uri" and message "redirect_uri must match the host of the request". Flow does not proceed.
- **PKCE validation failures**: When PKCE code_verifier from state token fails validation at third-party token endpoint (invalid_grant error), system logs security event, redirects to sessions page with error "Authorization failed - please try again", and allows retry via Login button.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a user interface listing all configured third-party OAuth2 services with active sessions
- **FR-002**: System MUST display session status for each service including: session existence, initiation timestamp, dependent agent count, encryption status, and expiration status (only marked expired when refresh token expires, not when access token expires)
- **FR-003**: System MUST provide a "Login" button for services with expired sessions (replaces Terminate button - user must re-authenticate; cannot terminate expired session)
- **FR-004**: System MUST provide a "Terminate Session" button for services with established sessions
- **FR-005**: System MUST expose `GET /api/third-party/{serviceId}/oauth2/authorize` accepting a `redirect_uri` query parameter and a same-origin `POST /api/third-party/{serviceId}/oauth2/authorize` accepting `redirect_uri` and `consent_state_id` form fields to initiate OAuth2 authorization code flow with PKCE
- **FR-006**: System MUST validate that `redirect_uri` parameter in authorize endpoint matches the host of the incoming request (same-origin validation)
- **FR-007**: System MUST generate PKCE code verifier (32-128 bytes per RFC 7636, using crypto/rand.Reader for entropy) and code challenge (SHA256 hash of verifier, base64url encoded) for each OAuth2 flow
- **FR-008**: System MUST create JWE state token containing: principal (current user), pkce_verifier, service_id, redirect_uri, and an optional consent_state_id claim for a consent-selection login
- **FR-009**: System MUST redirect user to third-party authorization endpoint with parameters: client_id, redirect_uri (callback endpoint), response_type=code, code_challenge, code_challenge_method=S256, scope, and state (JWE token)
- **FR-010**: System MUST expose endpoint `/api/third-party/{serviceId}/oauth2/callback` to receive OAuth2 callbacks from third-party services
- **FR-011**: System MUST validate state token at callback endpoint by: verifying JWE signature, decrypting claims, validating principal matches current authenticated user, validating service_id matches callback endpoint parameter, and validating consent_state_id when present
- **FR-012**: System MUST extract PKCE verifier from state token and use it to exchange authorization code for tokens
- **FR-013**: System MUST exchange authorization code for access token and refresh token using third-party's token endpoint
- **FR-014**: System MUST store obtained tokens encrypted in the token vault associated with user's principal and service ID
- **FR-015**: System MUST record session initiation timestamp when tokens are stored
- **FR-016**: System MUST count and display number of agents depending on each user session by querying user_grants.delegated_oauth2_tokens JSONB field matching thirdparty_oauth2_service_id
- **FR-017**: System MUST display warning dialog before session termination showing affected agents
- **FR-018**: System MUST delete stored tokens (access and refresh) when user terminates a session
- **FR-019**: Both authorize and callback endpoints MUST require authenticated principal (user must be logged in)
- **FR-020**: System MUST handle OAuth2 error responses from third-party services gracefully by parsing error and error_description parameters from callback URL, displaying user-friendly error messages on sessions page, and allowing immediate retry
- **FR-021**: System MUST retry failed token exchange requests up to 3 times with exponential backoff delays (1 second, 2 seconds, 4 seconds) before displaying error message to user

- **FR-022**: The frontend MUST store a non-empty `Record<string, string[]>` selection map as `{ expiresAt, selections }` in current-tab `sessionStorage` under the internal key `agentic-identity-broker:consent-state:<uuid>`. It MUST generate `consent_state_id` as the bare UUID returned by `crypto.randomUUID()`. The UUID MUST resist collisions among live records in the originating tab during its 15-minute lifetime. `consent_state_id` MUST NOT include a URI, URN, or the internal storage-key prefix.
- **FR-023**: Consent-selection records MUST expire after 15 minutes. Saving MUST prune expired records under the consent-state prefix.
- **FR-024**: A service login with selections MUST submit `redirect_uri` and `consent_state_id` in a same-origin form POST. The broker MUST store `consent_state_id` only in `OAuth2StateTokenClaims`; the service-login URL and return URL MUST NOT contain `consent_state_id`, `consent_state`, or selection JSON. Before login, the frontend MUST remove a legacy `consent_state` parameter from the copied URL.
- **FR-025**: The frontend MUST load only UUID references and validated maps with string-array values. It MUST return `undefined` without throwing for unknown, malformed, expired, or unavailable browser-storage records.
- **FR-026**: If a selection reference cannot load, the frontend MUST use existing grant or default selections. It MUST preserve `session_token`, `success`, `service_id`, same-origin validation, and callback processing.
- **FR-027**: If saving a non-empty selection map fails, the frontend MUST not navigate. It MUST show `Unable to preserve selections. Please try again.`.
- **FR-028**: This amendment adds only the same-origin form POST selection initiation and the optional `consent_state_id` JWE claim. It MUST NOT change the PKCE verifier, authenticated principal binding, service binding, expiration, token exchange, token storage, or the broker authorization-server state path. The provider-facing `state` parameter MUST stay below 6,000 bytes for a large selection map.

### Domain Model

**Entities**:

- **UserSession**: Represents an authenticated session between a user (principal) and a third-party OAuth2 service. Contains encrypted access token, refresh token, service ID, principal identifier, initiation timestamp, and token expiration metadata.

- **OAuth2StateToken**: JWE token containing the OAuth2 flow state parameters. Short-lived, contains principal, PKCE verifier, service_id, and redirect_uri. Used only during OAuth2 authorization flow to bind callback to the initiating request.

**Value Objects**:

- **PKCE**: Use existing types from the Golang oauth library
- **EncryptedToken**: Encrypted representation of OAuth2 access or refresh token using EncryptionPort

**Domain Events**:

- **SessionEstablished**: Fired when user successfully completes OAuth2 flow and tokens are stored
- **SessionTerminated**: Fired when user explicitly terminates a session and tokens are deleted

*All domain terms should be added to ARCHITECTURE.md Glossary section*

### Configuration Requirements

**Configuration Parameters**:

- **third_party_oauth2.jwe_signing_key**: Symmetric key for signing and encrypting JWE state tokens (type: string, required, environment variable: IDENTITY_BROKER_JWE_SIGNING_KEY)
- **third_party_oauth2.state_token_ttl**: Time-to-live for state tokens in seconds (type: int, default: 600, example: 600)
- **third_party_oauth2.pkce_verifier_length**: Length of PKCE verifier in bytes (type: int, default: 32, range: 32-128)

**Example YAML Configuration**:
```yaml
# Third-party OAuth2 session configuration
third_party_oauth2:
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
  state_token_ttl: 600  # 10 minutes
  pkce_verifier_length: 32  # 32 bytes = 256 bits
```

**Configuration Location**: Will be added to `examples/config/third-party-oauth2.yaml` and referenced in `examples/config/README.md`

### API Requirements

- **API-001**: All OAuth2 session endpoints MUST be documented in `/api/enduser/openapi.yaml`
- **API-002**: API documentation MUST include endpoint paths, HTTP methods, parameters, request/response bodies, error codes, examples, and authentication requirements
- **API-003**: Authorize endpoint: `GET /api/third-party/{serviceId}/oauth2/authorize?redirect_uri={uri}` - initiates OAuth2 flow, returns 302 redirect to third-party authorization endpoint
- **API-004**: Callback endpoint: `GET /api/third-party/{serviceId}/oauth2/callback?code={code}&state={state}` - processes OAuth2 callback, returns redirect to sessions page with success/error status
- **API-005**: Sessions list endpoint: `GET /api/third-party/sessions` - returns list of all services with user's session status for each
- **API-006**: Session termination endpoint: `DELETE /api/third-party/{serviceId}/session` - terminates user session and deletes stored tokens
- **API-007**: All endpoints MUST require authenticated principal (X-Remote-User header)
- **API-008**: APIs MUST follow Zalando RESTful API and Event Guidelines
- **API-009**: Error responses MUST follow standard format: `{"error": "code", "message": "description"}`

### Database Requirements

- **DB-001**: Create migration `004_create_user_sessions.up.sql` and `004_create_user_sessions.down.sql` for user_sessions table
- **DB-002**: user_sessions table schema MUST include: id (UUID primary key), principal (string, indexed), service_id (UUID, foreign key to thirdparty_oauth2_services), encrypted_access_token (bytea), encrypted_refresh_token (bytea, nullable), token_type (string), access_token_expires_at (timestamp, nullable), refresh_token_expires_at (timestamp, nullable), initiated_at (timestamp), scope (string array), encryption_context (jsonb)
- **DB-003**: Add unique constraint on (principal, service_id) - one session per user per service. This constraint prevents race conditions when multiple OAuth2 flows are initiated simultaneously; first callback to complete successfully wins, subsequent callbacks will detect existing session via constraint violation.
- **DB-004**: Add index on principal for fast session lookups by user
- **DB-005**: Add foreign key constraint from service_id to third_party_services.id with ON DELETE RESTRICT (prevent deleting services with active sessions). Admin interface must catch constraint violation and display error message with active session count, requiring admin to terminate all user sessions before service deletion.
- **DB-006**: Migration MUST be tested for both up and down operations without data loss
- **DB-007**: Repository implementation MUST follow `specs/004-persistence-layer/quickstart.md` patterns

### Security Requirements

- **SR-001**: State tokens MUST use authenticated encryption (JWE) to prevent tampering
- **SR-002**: PKCE MUST be used for all OAuth2 authorization code flows (code_challenge_method=S256)
- **SR-003**: redirect_uri MUST be validated to match the host of the incoming request (prevent open redirect)
- **SR-004**: State token validation MUST verify principal matches current authenticated user (prevent CSRF)
- **SR-005**: OAuth2 tokens (access and refresh) MUST be encrypted before storage using EncryptionPort
- **SR-006**: System MUST fail closed if state token validation fails (reject callback)
- **SR-007**: System MUST fail closed if PKCE validation fails (reject token exchange)
- **SR-008**: JWE signing key MUST be loaded from environment variable, not committed to config files
- **SR-009**: System MUST emit structured audit logs for: session establishment, session termination, failed state validation, failed PKCE validation
- **SR-010**: Token encryption MUST use encryption context including: principal, service_id, and session_id
- **SR-011**: State tokens MUST have short TTL (recommended default: 10 minutes, MUST NOT exceed 15 minutes) to limit exposure window

### Key Entities

- **UserSession**: Represents authenticated OAuth2 session between a user and third-party service, storing encrypted tokens and session metadata
- **OAuth2StateToken**: Short-lived JWE token binding OAuth2 flow to user and request context
- **ThirdPartyService**: Configuration for external OAuth2 provider (already exists from spec 006)
- **Principal**: User identifier extracted from authentication headers (already exists from spec 005)

*Domain concepts should be added to ARCHITECTURE.md Glossary (per Constitution Principle V)*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Users can successfully complete OAuth2 authorization flow and establish sessions with third-party services in under 60 seconds
- **SC-002**: 100% of OAuth2 tokens are stored encrypted in the token vault (verified through code review and security scanning)
- **SC-003**: State token validation rejects 100% of tampered or mismatched tokens (verified through security testing)
- **SC-004**: Users can view their third-party session status and see accurate counts of dependent agents within 2 seconds of page load
- **SC-005**: Users can terminate sessions and see confirmation within 2 seconds, with tokens removed from storage immediately
- **SC-006**: PKCE validation prevents authorization code interception attacks with 100% effectiveness (verified through security testing)
- **SC-007**: Redirect URI validation blocks open redirect vulnerabilities in 100% of test cases

## Assumptions *(optional - include if making assumptions)*

- Third-party OAuth2 services are already configured by administrators with client credentials and endpoints (from spec 006)
- Users are authenticated via reverse proxy and have valid principals in session (from spec 005)
- EncryptionPort implementation is available for encrypting/decrypting tokens
- Token vault storage implementation follows PostgreSQL patterns from spec 004
- Frontend framework (React) can display service cards and handle navigation to third-party authorization endpoints
- Third-party OAuth2 services support authorization code flow with PKCE (RFC 7636)
- Third-party services accept redirect_uri pointing to the broker's callback endpoint
- Agent-to-session dependency tracking will be implemented separately (counts displayed but relationship management is future work)
- Token refresh logic (using refresh tokens to obtain new access tokens) is deferred to future enhancement

## Out of Scope *(optional - explicitly state what's NOT included)*

- Automatic token refresh using refresh tokens when access tokens expire (future enhancement - will be implemented transparently to users)
- Token revocation at third-party services when session is terminated
- Multi-tab synchronization of session status (users must refresh to see updates)
- Session timeout or automatic expiration based on inactivity
- OAuth2 device flow or implicit flow (only authorization code flow with PKCE)
- Agent interface for consuming stored tokens (future enhancement)
- Migration of existing tokens if encryption key changes
- Admin UI for viewing or revoking user sessions
- Session analytics or usage statistics
- Support for OAuth2 clients that don't support PKCE
