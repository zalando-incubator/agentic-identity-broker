# Feature Specification: Third-Party OAuth2 Session Management

**Feature Branch**: `008-thirdparty-oauth2-sessions`  
**Created**: 2025-12-22  
**Status**: Draft  
**Input**: User description: "Users need to be able to login to third-party services and the identity broker will manage the users' sessions and store their respective OAuth2 tokens."

## Clarifications

### Session 2025-12-22

- Q: What happens when user initiates multiple OAuth2 flows for the same service simultaneously? → A: The unique constraint permits one row. Each successful callback uses the existing upsert to replace credentials and clear recorded refresh rejection.
- Q: How does system handle third-party authorization endpoint returning an error instead of authorization code? → A: Parse OAuth2 error response (error, error_description); redirect to sessions page with user-friendly error message displayed; allow immediate retry
- Q: How does system handle network failures during token exchange? → A: Retry token exchange up to 3 times with exponential backoff (1s, 2s, 4s); if all retries fail, display error message allowing user to retry OAuth2 flow
- Q: What happens when the access token expires but a refresh token is stored? → A: Refresh remains available unless the provider rejected it. Access expiry alone does not require reauthentication in this case. The broker does not predict third-party refresh-token expiry.
- Q: What happens if a service has session rows when an administrator deletes it? → A: All retained rows block deletion, including rejected sessions. The administrator must explicitly terminate these rows first.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - View Available Third-Party Sessions (Priority: P1)

A user needs to see retained third-party sessions and their current status. This includes initiation time, dependent agents, access-token expiry, and required reauthentication.

**Why this priority**: This is the foundational capability that provides users visibility into available services and their authentication status. Without this view, users cannot discover which third-party services have been delegated to agents.

**Independent Test**: The sessions page lists retained sessions, initiation time, dependent agents, and encryption status. It distinguishes access-token expiry from required reauthentication.

**Acceptance Scenarios**:

1. **Given** an authenticated user with configured services, **When** user opens the sessions page, **Then** it lists retained sessions. Each entry includes the service name and description. Rejected sessions remain visible.
2. **Given** user has a session, **When** user views its card, **Then** the card shows session metadata. This includes initiation time, dependent-agent count, encryption status, and current status. It shows no refresh-expiry date or permanent-validity claim.
4. **Given** user views an established session, **When** reviewing the service card, **Then** system shows a "Terminate Session" button
5. **Given** multiple agents depend on a session, **When** user views the session card, **Then** system displays the count of dependent agents
6. **Given** a session has an unexpired access token, **When** user clicks Refresh, **Then** provider HTTP 400 `invalid_grant` makes the broker record rejection. The broker retains the row and token metadata. The card immediately shows "Reauthentication required" without a reload. Login is available, Refresh is hidden, and Terminate remains an optional action. Later token acquisition returns no token. **When** user clicks Login and completes the real authorize/callback flow, **Then** the callback clears rejection through upsert. The card returns to "Active", and Refresh is available again.

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
5. **Given** user cancels termination in the warning dialog, **When** the dialog closes, **Then** the session retains its previous status and stored state.

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

The consent page stores its selections and original return URL in current-tab `sessionStorage` under a bare UUID `consent_state_id`. With selections or return-page query/fragment data, it submits that ID and a same-origin return path in a form POST. The broker seals the ID in its third-party state JWE; it does not seal the original `session_token` or other page parameters. A successful callback returns the verified ID so the originating tab can recover the right record and replace its URL without the ID.

**Acceptance Scenarios**:

1. **Scenario 1: Compact reference creation. Given** a consent page has a large selection map, **When** the user starts a service login, **Then** the page stores that map and its return URL under a UUID reference, sends the ID as `consent_state_id` in a same-origin form POST, and the provider-facing state JWE contains only the ID and a clean same-origin return path. Neither selection JSON nor the session token appears inside the return path, and the provider-facing `state` is less than 6,000 bytes.
2. **Scenario 2: Same-tab restoration. Given** a callback returns with a valid `consent_state_id` claim, **When** the consent page renders in the originating tab, **Then** it restores only that ID's selections for the matching service and page, including selected optional permission sets and their available service cards. After the user changes a selection and reloads the page, the previous selections do not return.
3. **Scenario 3: Complete provider callback. Given** the user starts a service login with optional selections and a `session_token`, **When** the mock provider completes the callback in the same tab, **Then** the broker returns the validated ID and callback status, the page restores the original `session_token` and selected permission sets, and the actual provider-facing `state` stays below 6,000 bytes.

**Edge Cases**:

- A missing, malformed, expired, wrong-service, wrong-page, or unreadable reference uses existing grant or default selections. The callback continues.
- If browser storage cannot save non-empty selections or return data, the page does not navigate and reports the preservation failure.
- If no selections or return data exist, the page uses the existing GET flow.
- A valid record remains until it expires; after restoration, the callback ID is removed from the browser URL, so reloads cannot replay selections.
- A callback in another browser tab cannot recover the originating tab's return data.

---

### Edge Cases

- **Multiple simultaneous OAuth2 flows for the same service**: The unique constraint permits one row per principal and service. Successful callbacks replace credentials through upsert. A successful reauthorization clears recorded rejection without prior termination.
- **Third-party authorization errors**: When third-party returns OAuth2 error (e.g., access_denied, invalid_scope, server_error), system parses error and error_description parameters from callback URL, redirects user to sessions page with user-friendly error message, and allows immediate retry via Login button.
- **Network failures during token exchange**: System retries token exchange request up to 3 times with exponential backoff (1s, 2s, 4s delays). If all retries fail, displays error message to user allowing them to retry the entire OAuth2 flow from the beginning.
- **Expired access token with stored refresh credentials**: The card shows "Access Token Expired" unless the provider rejected refresh. The broker can attempt refresh. Credential presence does not prove provider validity.
- **Provider refresh rejection**: HTTP 4xx `invalid_grant`, excluding 429, records rejection and immediately stops future broker token issuance. The broker retains ciphertext, access expiry, and other metadata. The card shows "Reauthentication required" even for unexpired access tokens.
- **Transient refresh failure**: Other provider errors, transport errors, and malformed responses do not record rejection. An otherwise valid access token remains usable.
- **Access-only sessions and unknown expiry**: Access expiry requires reauthentication when no refresh credential exists. Missing access expiry means unknown expiry, not permanent validity.
- **Service deletion with retained sessions**: The foreign key blocks deletion while session rows reference a service, including rejected rows. The administrator must explicitly terminate those sessions first.
- **Malformed or missing callback parameters**: When third-party returns malformed callback (missing code or state), system redirects to sessions page with error message "Invalid callback parameters" and allows retry via Login button. Missing code treated same as access_denied error.
- **Redirect URI domain mismatch**: When redirect_uri parameter in authorize request does not match the Host header of the incoming request, system returns 400 Bad Request with error "invalid_redirect_uri" and message "redirect_uri must match the host of the request". Flow does not proceed.
- **PKCE validation failures**: When PKCE code_verifier from state token fails validation at third-party token endpoint (invalid_grant error), system logs security event, redirects to sessions page with error "Authorization failed - please try again", and allows retry via Login button.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST list retained third-party sessions, including sessions that require reauthentication.
- **FR-002**: System MUST show initiation time, dependent-agent count, encryption status, and status without predicted refresh-expiry dates. Status precedence is "Reauthentication required", "Access Token Expired", then "Active".
- **FR-003**: System MUST show Login for sessions with `requires_reauthentication=true`. Refresh MUST appear only with stored refresh credentials and no required reauthentication.
- **FR-004**: System MUST keep Terminate available for every retained session. Reauthentication does not require prior termination.
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

- **FR-022**: The frontend MUST store `{ expiresAt, selections, serviceID, returnURL }` in current-tab `sessionStorage` under `agentic-identity-broker:consent-state:<uuid>` when it must preserve selections or return-page query/fragment data. `selections` MUST be a `Record<string, string[]>` (possibly empty when preserving only return data). `consent_state_id` MUST be the bare UUID from `crypto.randomUUID()`, not a URI, URN, or prefixed storage key.
- **FR-023**: Records MUST expire after 15 minutes, covering the maximum configurable third-party state lifetime; the default state TTL is 10 minutes. Saving MUST prune expired records under the consent-state prefix.
- **FR-024**: A service login with selections or return data MUST send `redirect_uri` (same-origin page path, without query or fragment) and `consent_state_id` in a same-origin form POST. The broker MUST seal the ID in `OAuth2StateTokenClaims` and add it to the successful callback URL only after the state is validated and the session is saved. The service-login URL and JWE return path MUST NOT contain the ID, selection JSON, or the original return-page query/fragment. Before login, the frontend MUST remove any legacy `consent_state` parameter from its saved page URL.
- **FR-025**: The frontend MUST load only UUID references and validated maps with string-array values, matching the callback service ID, origin, and path. It MUST return `undefined` without throwing for unknown, malformed, expired, mismatched, or unavailable browser-storage records.
- **FR-026**: If a selection reference cannot load, the frontend MUST use existing grant or default selections. For a valid same-tab record it MUST restore the original `session_token` and callback `success`/`service_id`, then remove the callback ID from the browser URL so reloads do not replay old selections. The broker's same-origin validation and callback processing remain unchanged.
- **FR-027**: If saving a non-empty selection map fails, the frontend MUST not navigate. It MUST show `Unable to preserve selections. Please try again.`. If saving only return data fails, it MUST also block login and report the failure.
- **FR-028**: These changes MUST NOT change the PKCE verifier, authenticated principal binding, service binding, expiration, token exchange, token storage, or broker authorization-server state path. The provider-facing `state` MUST remain below 6,000 bytes even when the return page has a realistic `session_token` JWE; arbitrary oversized `redirect_uri` inputs MUST fail before redirecting to a third-party provider.

### Domain Model

**Entities**:

- **UserSession**: Stores encrypted third-party tokens, scope, principal, service ID, initiation time, access-token expiry, and observed refresh rejection. A false rejection flag means no observed rejection, not proven validity.

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
- **API-010**: The session contract MUST use version `2.0.0` without endpoint changes. It MUST expose `requires_reauthentication` instead of an invented session-expiry predicate. No refresh-expiry field exists.
- **API-011**: Manual refresh MUST return 409 `refresh_rejected` for recorded or newly observed rejection. Its message is `refresh token rejected by provider; reauthentication required`. Absent refresh credentials return 409 `refresh_unavailable`. Transient upstream errors return 502 `refresh_failed`.
- **API-012**: `has_refresh_token` MUST report ciphertext presence, including rejected credentials. `access_token_expired` MUST remain an independent access-token fact.

### Database Requirements

**Release scope**: This PR2 activation release includes migration 038 and adds migration 039 to prepared application code. Migration 040 belongs to the following cleanup PR and is not present. Durable PostgreSQL rejection requirements apply at 39 on already-running PR1 brokers. PostgreSQL at 38 supports ordinary traffic but rejects true writes as `persistence_failed`. The memory adapter already supports rejection. This release defaults and limits Helm targets to 39. Fresh installs run direct UP-all through 39, while normal upgrades use the guarded wrapper. Explicit upgrade target 0 remains refused. Before merging or deploying activation, complete PR1 across every replica. Drain every unprepared replica and in-flight operation first. Compatibility defaults true in preparation and activation, with explicit false supported for modern consumers. Cleanup defaults false after its consumer gate passes. The alias supplies only `is_expired=requires_reauthentication`, without refresh dates.

- **DB-001**: Preserve original migration `004_create_user_sessions` and migration 037 unchanged. Migration 038 MUST prepare a stable view and write functions without changing the legacy table shape. Before merging or deploying PR2, complete the PR1 rollout and drain every unprepared replica and in-flight operation. PR2 adds migration 039 and activates durable rejection without restart. Before merging or deploying PR3, upgrade the embedded SPA and every external session-API consumer. PR3 adds migration 040 and removes only the unused legacy column after schema dependencies end. The separate runner MUST support explicit phase targets. Temporary storage compatibility MUST NOT imply provider validity.
- **DB-002**: From phase 39, the schema MUST include id, principal, service_id, encrypted tokens, token_type, access_token_expires_at, refresh_token_rejected, initiated_at, scope, and encryption_context. `refresh_token_rejected` is BOOLEAN NOT NULL DEFAULT FALSE. Historical expiry values MUST NOT imply rejection. Preparation/new brokers MUST use `user_session_state` and stable invoker write functions at phases 38, 39, and 40. The preparation view reports false and rejects requested rejection writes rather than claiming persistence.
- **DB-003**: Keep the unique constraint on (principal, service_id). Successful authorization callbacks MUST replace credentials and clear rejection through the existing upsert. Normal refresh MUST NOT clear rejection.
- **DB-004**: Add index on principal for fast session lookups by user
- **DB-005**: Keep the service_id foreign key with ON DELETE RESTRICT. Retained sessions MUST block service deletion, including rejected rows. The administrator MUST explicitly terminate these rows before deletion.
- **DB-006**: Migration tests MUST retain legacy reads/writes at 38, data preservation, true-write refusal, and preparation rollback/reapply. This PR2 release adds phase-39 rejection, guarded rollback, and contention coverage. PR3 adds the final phase-40 lifecycle and nullable-column restoration coverage. Tests MUST NOT call an absent migration. Rendered Helm tests MUST cover default fresh-install UP-all, default guarded upgrade 39, explicit targets 38 and 39, unavailable-target rejection, explicit target-0 upgrade refusal, both grants modes, compatibility true with explicit false, and rolling availability. Real image execution MUST abort on dirty metadata and leave equal/newer clean schemas unchanged. Serialize migration runners. Rollback MUST use compatible binaries and an explicit separate-runner DOWN, never forward Helm stage controls. Operators MUST inspect actual schema and dirty metadata before deliberate repair, never automatic `Force` or row deletion. See the [online rollout runbook](../../docs/operations/session-sweep.md#online-session-validity-rollout-migrations-038040) for availability and SPA-asset prerequisites.
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
- On-demand and manual refresh use stored credentials unless rejection is recorded. Feature 029 specifies proactive refresh and sweep operations.

## Out of Scope *(optional - explicitly state what's NOT included)*

- Automatic refresh scheduling and sweep operations belong to feature 029. They exclude recorded provider rejection.
- Token revocation at third-party services when session is terminated
- Multi-tab synchronization of session status (users must refresh to see updates)
- Session timeout or automatic expiration based on inactivity
- OAuth2 device flow or implicit flow (only authorization code flow with PKCE)
- Agent interface for consuming stored tokens (future enhancement)
- Migration of existing tokens if encryption key changes
- Admin UI for viewing or revoking user sessions
- Session analytics or usage statistics
- Support for OAuth2 clients that don't support PKCE
