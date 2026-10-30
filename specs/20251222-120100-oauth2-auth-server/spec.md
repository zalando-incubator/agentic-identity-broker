# Feature Specification: OAuth2 Authorization Server Proxy

**Feature Branch**: `20251222-120100-oauth2-auth-server`
**Created**: 2025-12-22
**Status**: Draft
**Input**: User description: "New feature: OAuth2 Authorization Server

The identity broker should act like an OAuth2 Authorization Server.

To this end, it needs to
- provide an Authorization endpoint under the `/oauth2/authorize` path on the "enduser" server
- Provider a Token endpoint under the `/oauth2/token` path on the enduser server

Also we need to implement OAuth2 metadata discovery which results in supporting a `.well-known` endpoint and returns this application's OAuth2 endpoints.

We will not implement a full OAuth2 Authorization server but act like one. This means we'll proxy both endpoints to another OAuth2 server for token minting and validation.

This means, we need to configure the OAuth2 Authorization feature by providing the upstream servers issuer URI and the authorize and token endpoint.

The application does **NOT** for now: Issue, validate, or manage its own tokens. It acts merely as a proxy.

The configuration block should be called `oauth2_authorization_server`.

The authorize endpoint only interprets the `client_id` parameter and validates it against the registered Agents (use the Agent's client_id attribute) to exist. It also validates against the existing consent functionality that user consent has been given for the agent.  If consent has been given it forwards the authorize request to the target oAuth2 Authorization server. If consent has not been given, user is redirected to `/consent/agent/:agent-id?redirect_uri=<the full URL of the original /oauth2/authorize request>`"

## Clarifications

### Session 2025-12-22

- Q: When proxying the authorization request to the upstream OAuth2 server, how should the broker handle the user's authentication? → A: Redirect user's browser to upstream OAuth2 server (transparent proxy - upstream handles user authentication)
- Q: How should rate limiting be configured for the OAuth2 endpoints? → A: No rate limiting configuration (rely on upstream rate limiting only)
- Q: How should the broker proxy token endpoint requests to the upstream OAuth2 server? → A: Server-to-server HTTP request (broker acts as HTTP client, forwards request to upstream, returns response)
- Q: How should the broker validate the upstream OAuth2 server's TLS certificate? → A: Standard TLS validation (verify against system certificate authorities, reject self-signed or expired certificates)
- Q: What audit logging should be implemented for the token endpoint? → A: No token endpoint logging (rely on upstream OAuth2 server audit logs only)

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Client Application Initiates OAuth2 Authorization Flow (Priority: P1)

A client application (agent) needs to obtain authorization from a user to access resources on their behalf. The client redirects the user's browser to the identity broker's OAuth2 authorization endpoint with standard OAuth2 parameters (client_id, redirect_uri, scope, state, etc.). The broker validates the client_id and checks if user consent exists before proceeding.

**Why this priority**: This is the entry point for all OAuth2 authorization flows. Without this, no OAuth2 integration is possible. This delivers the core proxy capability.

**Independent Test**: Can be fully tested by initiating an OAuth2 authorization request with a valid registered agent client_id and verifying that the system correctly validates the client, checks consent status, and either redirects to the consent UI or proxies the request to the upstream OAuth2 server. Delivers immediate value by enabling OAuth2 flows through the identity broker.

**Acceptance Scenarios**:

1. **Given** a user is authenticated and a client application initiates authorization, **When** the authorization request arrives at `/oauth2/authorize` with a valid client_id parameter, **Then** the system looks up the Agent by client_id and validates that it exists
2. **Given** an authorization request with an invalid or missing client_id, **When** the request is processed, **Then** the system returns an OAuth2 error response indicating invalid_client
3. **Given** an authorization request with a valid client_id, **When** the system checks for existing user consent, **Then** it queries the grant store for an active grant matching the authenticated user's principal and the agent's ID
4. **Given** an authorization request for which the user has previously granted consent, **When** consent validation completes, **Then** the system issues an HTTP redirect (302/303) to the upstream OAuth2 authorization endpoint preserving all original query parameters
5. **Given** an authorization request for which no user consent exists, **When** consent validation fails, **Then** the system redirects the user to `/consent/agent/:agent-id?redirect_uri=<URL-encoded full original request URL>`
6. **Given** the user is redirected to the consent UI, **When** they complete the consent flow and approve, **Then** the consent UI redirects back to the original authorization request URL and the flow continues
7. **Given** all OAuth2 parameters from the original request, **When** proxying to the upstream server, **Then** the system forwards client_id, redirect_uri, scope, state, response_type, and any other parameters unchanged
8. **Given** the user's browser is redirected to the upstream OAuth2 server, **When** the upstream processes the authorization, **Then** the upstream redirects the user's browser directly to the client's redirect_uri with authorization code or error (broker is not in the response path)

---

### User Story 2 - Client Application Exchanges Authorization Code for Access Token (Priority: P2)

After receiving an authorization code from the authorization endpoint, a client application needs to exchange it for an access token by making a direct server-to-server request to the token endpoint. The identity broker proxies this request to the upstream OAuth2 server without additional validation.

**Why this priority**: This completes the OAuth2 authorization code flow. It depends on P1 (authorization must happen first) but is essential for delivering working OAuth2 integration. The token endpoint is simpler than authorization as it requires no consent checking.

**Independent Test**: Can be tested by making a POST request to `/oauth2/token` with a valid authorization code and client credentials, and verifying that the request is correctly proxied to the upstream OAuth2 server with the response returned unmodified to the client.

**Acceptance Scenarios**:

1. **Given** a client has received an authorization code, **When** it sends a POST request to `/oauth2/token` with grant_type=authorization_code and the code, **Then** the broker makes a server-to-server HTTP POST request to the upstream OAuth2 token endpoint
2. **Given** a token request is being proxied, **When** the system constructs the upstream request, **Then** it preserves all parameters including grant_type, code, redirect_uri, client_id, and client_secret
3. **Given** the upstream OAuth2 server responds to the token request, **When** the response is received, **Then** the system returns the response body and status code directly to the client without modification
4. **Given** the upstream server returns an access token, **When** the token response is proxied back, **Then** the client receives a standard OAuth2 token response with access_token, token_type, expires_in, and optional refresh_token
5. **Given** the upstream server returns an error response, **When** the error is proxied back, **Then** the client receives the OAuth2 error response with error code and description
6. **Given** a token request for grant_type=refresh_token, **When** the request is processed, **Then** the broker makes a server-to-server HTTP POST request to the upstream token endpoint following the same proxy pattern

---

### User Story 3 - OAuth2 Clients Discover Authorization Server Metadata (Priority: P3)

OAuth2 clients and libraries need to discover the identity broker's OAuth2 endpoints and capabilities to configure their OAuth2 flows automatically. The broker exposes a standard OAuth2 metadata discovery endpoint that returns information about its authorization and token endpoints.

**Why this priority**: While not strictly required for manual OAuth2 integration, metadata discovery is a standard OAuth2 feature that improves developer experience and enables automatic client configuration. It can be implemented independently after P1 and P2.

**Independent Test**: Can be tested by making a GET request to the well-known metadata endpoint and verifying that it returns a valid OAuth2 authorization server metadata document with the broker's endpoints and supported features.

**Acceptance Scenarios**:

1. **Given** an OAuth2 client needs to discover server metadata, **When** it requests GET `/.well-known/oauth-authorization-server`, **Then** the system returns a JSON metadata document
2. **Given** the metadata document is being constructed, **When** the response is built, **Then** it includes the issuer field set to the identity broker's public base URL
3. **Given** the metadata document is returned, **When** clients parse it, **Then** they find authorization_endpoint pointing to the broker's `/oauth2/authorize` endpoint
4. **Given** the metadata document is returned, **When** clients parse it, **Then** they find token_endpoint pointing to the broker's `/oauth2/token` endpoint
5. **Given** the metadata response includes capabilities, **When** clients review supported features, **Then** the document includes response_types_supported (e.g., ["code"]), grant_types_supported (e.g., ["authorization_code", "refresh_token"]), and token_endpoint_auth_methods_supported
6. **Given** the broker is configured with specific OAuth2 capabilities, **When** the metadata is generated, **Then** it accurately reflects the broker's configuration (not the upstream server's capabilities)

---

### Edge Cases

- What happens when the upstream OAuth2 server is unreachable during authorization or token exchange? The system returns an OAuth2 error response (temporarily_unavailable) to the client with appropriate error details.
- What happens when the upstream OAuth2 server's TLS certificate is invalid (self-signed, expired, hostname mismatch)? The system rejects the connection and returns an OAuth2 error response (server_error) to the client without completing the proxy operation.
- How does the system handle malformed OAuth2 requests missing required parameters? Returns standard OAuth2 error responses (invalid_request) following RFC 6749 specification.
- What happens when the redirect_uri in the token request doesn't match the one used in the authorization request? The upstream OAuth2 server validates this and returns an error; the broker proxies this error back to the client.
- How does the system behave if the user's authentication session expires during the authorization flow? Redirects to login with return path preserving the original authorization request parameters.
- What happens when a user completes consent but the original authorization request parameters are lost? The consent UI must preserve and pass back the full redirect_uri containing all original parameters.
- How does the system handle OAuth2 implicit flow (response_type=token) or other response types? Proxies them to the upstream server; the upstream server handles flow-specific logic and validation.
- What happens when the upstream OAuth2 server redirects back to the broker instead of the client's redirect_uri? This indicates a configuration error; the system logs the issue and returns an error to prevent redirect loops.
- How does the system handle PKCE (Proof Key for Code Exchange) parameters in the authorization and token requests? Proxies them transparently; the upstream OAuth2 server validates PKCE if configured.
- What happens when multiple agents use the same OAuth2 client_id? This is a configuration error that violates uniqueness constraints; agent registration should prevent this, but if it occurs, the system returns an internal_error.
- How does the system handle authorization requests for agents that are registered but have no associated user grants? The system treats this as "no consent" and redirects to the consent UI for the user to create a grant.
- What happens when a user's grant expires (valid_until timestamp passed) during an authorization request? The system treats expired grants as non-existent and redirects to the consent UI to obtain fresh consent.
- How does the system determine the public base URL for constructing OAuth2 metadata and redirect URIs? Uses configuration (hostname, port, protocol) from the application's enduser server settings.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST expose an OAuth2 authorization endpoint at `/oauth2/authorize` on the enduser server
- **FR-002**: System MUST expose an OAuth2 token endpoint at `/oauth2/token` on the enduser server
- **FR-003**: System MUST expose OAuth2 authorization server metadata at `/.well-known/oauth-authorization-server`
- **FR-004**: Authorization endpoint MUST extract the client_id parameter from the authorization request
- **FR-005**: Authorization endpoint MUST validate that the client_id corresponds to a registered Agent in the agent registry
- **FR-006**: Authorization endpoint MUST return OAuth2 error response with error=invalid_client if client_id is missing or does not match a registered agent
- **FR-007**: Authorization endpoint MUST extract the authenticated user's principal from the request context (set by RequirePrincipalMiddleware which reads the configured principal header)
- **FR-008**: Authorization endpoint MUST query the grant store to check if an active grant exists matching the user's principal and the agent's ID
- **FR-009**: Authorization endpoint MUST treat grants with valid_until timestamp in the past as non-existent (expired)
- **FR-010**: Authorization endpoint MUST redirect to `/consent/agent/:agent-id?redirect_uri=<URL-encoded original request>` when no active grant exists
- **FR-011**: Authorization endpoint MUST construct the consent redirect URL with the full original authorization request URL (including all query parameters) as the redirect_uri parameter
- **FR-012**: Authorization endpoint MUST proxy the authorization request to the configured upstream OAuth2 authorization endpoint when an active grant exists
- **FR-013**: System MUST forward all original query parameters (client_id, redirect_uri, scope, state, response_type, etc.) when proxying the authorization request
- **FR-014**: System MUST redirect the user's browser to the upstream OAuth2 authorization endpoint (HTTP 302/303) allowing the upstream server to handle user authentication
- **FR-015**: After redirecting to upstream, the upstream OAuth2 server handles the authorization response by redirecting the user's browser directly to the client's redirect_uri (broker not involved in response path)
- **FR-016**: Token endpoint MUST accept POST requests with standard OAuth2 token request parameters
- **FR-017**: Token endpoint MUST proxy all token requests by making server-to-server HTTP POST requests to the configured upstream OAuth2 token endpoint
- **FR-018**: Token endpoint MUST forward all parameters in the HTTP request body to upstream including grant_type, code, redirect_uri, client_id, client_secret, and refresh_token
- **FR-019**: Token endpoint MUST read the HTTP response from upstream and return the response body and headers to the requesting client without modification
- **FR-020**: Token endpoint MUST preserve HTTP status codes from the upstream OAuth2 server response
- **FR-021**: Token endpoint MUST support both application/x-www-form-urlencoded and application/json content types for token requests
- **FR-022**: Metadata endpoint MUST return a JSON document conforming to RFC 8414 (OAuth 2.0 Authorization Server Metadata)
- **FR-023**: Metadata endpoint MUST include the issuer field set to the identity broker's public base URL
- **FR-024**: Metadata endpoint MUST include authorization_endpoint pointing to the broker's `/oauth2/authorize` URL
- **FR-025**: Metadata endpoint MUST include token_endpoint pointing to the broker's `/oauth2/token` URL
- **FR-026**: Metadata endpoint MUST include response_types_supported, grant_types_supported, and token_endpoint_auth_methods_supported arrays
- **FR-027**: System MUST validate that required configuration parameters (upstream OAuth2 server details) are present at startup
- **FR-028**: System MUST log all OAuth2 authorization requests including client_id, principal, and consent check results
- **FR-029**: System MUST handle HTTP redirects (3xx responses) from the upstream OAuth2 server appropriately
- **FR-030**: System MUST include proper error handling for upstream server unavailability (connection timeout, DNS failure, etc.)

### Configuration Requirements *(if applicable - document before implementation)*

**Configuration Parameters**:
- **upstream_issuer_uri**: String, the issuer URI of the upstream OAuth2 authorization server (e.g., "https://oauth2.example.com"), required, no default
- **upstream_authorize_endpoint**: String, the full URL of the upstream OAuth2 authorization endpoint (e.g., "https://oauth2.example.com/oauth2/authorize"), required, no default
- **upstream_token_endpoint**: String, the full URL of the upstream OAuth2 token endpoint (e.g., "https://oauth2.example.com/oauth2/token"), required, no default
- **server.enduser.public_url**: String (configured under `server.enduser`), the public base URL of the identity broker used for OAuth2 metadata (issuer) and redirects (e.g., "https://identity-broker.example.com"), required, no default
- **supported_response_types**: Array of strings, OAuth2 response types supported by the broker (e.g., ["code"]), optional, default: ["code"]
- **supported_grant_types**: Array of strings, OAuth2 grant types supported by the broker (e.g., ["authorization_code", "refresh_token"]), optional, default: ["authorization_code", "refresh_token"]
- **upstream_timeout**: Duration string, timeout for HTTP requests to the upstream OAuth2 server (e.g., "30s"), optional, default: 30s
- **mode**: Enum with only one value: "delegate_upstream"; No default for this value, so it must be configured. 

**Example YAML Configuration**:
```yaml
# OAuth2 Authorization Server Proxy Configuration
oauth2_authorization_server:
  mode: "delegate_upstream"

  # Upstream OAuth2 server configuration
  upstream_issuer_uri: "https://oauth2.example.com"
  upstream_authorize_endpoint: "https://oauth2.example.com/oauth2/authorize"
  upstream_token_endpoint: "https://oauth2.example.com/oauth2/token"

  # Note: the broker's public URL (issuer for RFC 8414 metadata) is configured
  # under server.enduser.public_url, not in this section.

  # Supported OAuth2 flows (optional, defaults shown)
  supported_response_types:
    - "code"
  supported_grant_types:
    - "authorization_code"
    - "refresh_token"

  # Upstream request timeout (optional)
  upstream_timeout: 30s
```

**Configuration Location**: Will be added to `examples/config/oauth2-authorization-server.yaml` and referenced in `examples/config/README.md`

### API Requirements *(if applicable - design before database)*

- **API-001**: All OAuth2 endpoints MUST be documented in `/api/enduser/openapi.yaml` (OpenAPI 3.0+ format)
- **API-002**: API documentation for `/oauth2/authorize` MUST include all standard OAuth2 authorization request parameters (client_id, redirect_uri, scope, state, response_type, etc.) with descriptions
- **API-003**: API documentation for `/oauth2/token` MUST include all standard OAuth2 token request parameters (grant_type, code, redirect_uri, client_id, client_secret, refresh_token) with descriptions
- **API-004**: API documentation for `/.well-known/oauth-authorization-server` MUST include the JSON schema for the metadata response following RFC 8414
- **API-005**: API documentation MUST describe OAuth2 error responses following RFC 6749 standard error format (error, error_description, error_uri)
- **API-006**: APIs MUST follow Zalando RESTful API and Event Guidelines (https://opensource.zalando.com/restful-api-guidelines/)
- **API-007**: OAuth2 endpoint paths and parameter names MUST conform to RFC 6749 and RFC 8414 specifications
- **API-008**: API documentation MUST include example authorization flows showing consent check, redirect to consent UI, and successful proxy to upstream server

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Authorization endpoint MUST verify user authentication before processing any authorization request
- **SR-002**: Authorization endpoint MUST validate that the client_id in the authorization request matches a registered agent to prevent unauthorized OAuth2 clients
- **SR-003**: System MUST enforce that users can only authorize agents for which they have active, non-expired grants
- **SR-004**: System MUST validate all URLs (redirect_uri, upstream endpoints) to prevent open redirect vulnerabilities
- **SR-005**: System MUST use HTTPS for all communication with the upstream OAuth2 server (reject HTTP URLs in configuration) and validate TLS certificates against system certificate authorities (reject self-signed or expired certificates)
- **SR-006**: System MUST NOT log or expose sensitive data including authorization codes, access tokens, refresh tokens, or client secrets
- **SR-007**: System MUST set secure HTTP headers when proxying OAuth2 responses (prevent caching of sensitive redirects)
- **SR-008**: Token endpoint MUST validate Content-Type and reject requests with unexpected content types to prevent CSRF attacks
- **SR-009**: System delegates rate limiting to the upstream OAuth2 server; broker does not implement additional rate limiting on OAuth2 endpoints
- **SR-010**: System MUST sanitize and validate all query parameters before constructing redirect URLs to prevent injection attacks
- **SR-011**: System MUST treat expired grants (valid_until < current time) as non-existent to enforce time-based consent expiration
- **SR-012**: System MUST fail closed if it cannot reach the upstream OAuth2 server (return error, do not silently fail or use stale data)
- **SR-013**: System MUST validate that upstream OAuth2 server responses conform to expected OAuth2 format before returning them to clients
- **SR-014**: System MUST include request IDs in all OAuth2 flows for audit trail and debugging purposes
- **SR-015**: System MUST emit structured audit logs for all authorization requests including client_id, principal, consent status, and authorization outcome; token endpoint operations are not logged by the broker (audit logs maintained by upstream OAuth2 server)

### Key Entities *(include if feature involves data)*

- **Agent**: Existing entity with `client_id` attribute used for OAuth2 client identification. The authorization endpoint validates incoming client_id parameters against the Agent registry.

- **User Grant**: Existing entity representing user consent to delegate access to an agent. The authorization endpoint checks for active grants (non-expired) before proxying to the upstream OAuth2 server.

- **OAuth2 Authorization Request**: Transient request object containing standard OAuth2 parameters (client_id, redirect_uri, scope, state, response_type, etc.) that flows through the authorization endpoint.

- **OAuth2 Token Request**: Transient request object containing token exchange parameters (grant_type, code, redirect_uri, client_id, client_secret, refresh_token) that flows through the token endpoint.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: OAuth2 clients can successfully complete an authorization code flow through the identity broker in under 5 seconds (excluding user consent interaction time)
- **SC-002**: Authorization endpoint correctly validates client_id against registered agents with 100% accuracy
- **SC-003**: Authorization endpoint correctly identifies missing consent and redirects to consent UI with 100% accuracy
- **SC-004**: Authorization requests with valid consent are proxied to the upstream OAuth2 server within 500 milliseconds
- **SC-005**: Token endpoint proxies token exchange requests to the upstream OAuth2 server within 500 milliseconds
- **SC-006**: OAuth2 metadata discovery endpoint responds with valid RFC 8414 compliant metadata within 100 milliseconds
- **SC-007**: System maintains 99.9% uptime for OAuth2 endpoints when the upstream OAuth2 server is available
- **SC-008**: System correctly handles upstream OAuth2 server failures without exposing internal errors to clients
- **SC-009**: All OAuth2 error responses conform to RFC 6749 error format with appropriate error codes
- **SC-010**: Consent flow correctly preserves all original OAuth2 request parameters through user interaction and back to authorization endpoint
- **SC-011**: System supports at least 100 concurrent OAuth2 authorization flows without performance degradation

## Scope

### In Scope

- OAuth2 authorization endpoint with client validation and consent checking
- OAuth2 token endpoint with transparent proxying to upstream server
- OAuth2 metadata discovery endpoint (well-known)
- Configuration for upstream OAuth2 server (issuer, endpoints)
- Integration with existing agent registry for client_id validation
- Integration with existing grant store for consent verification
- Redirect to consent UI when consent is missing
- Preservation of OAuth2 request parameters through consent flow
- Error handling and OAuth2-compliant error responses
- Structured logging and audit trail for OAuth2 operations
- Support for authorization code grant type and refresh token grant type

### Out of Scope

- Token issuance, validation, or management (all handled by upstream OAuth2 server)
- OAuth2 client registration (uses existing agent registry)
- OAuth2 scope validation beyond what the upstream server provides
- Custom OAuth2 extensions or non-standard flows
- OAuth2 device flow, client credentials flow, or resource owner password credentials flow
- JWT token inspection or validation endpoints
- Token revocation endpoint
- Dynamic client registration (RFC 7591)
- User authentication (relies on existing session management)
- Frontend UI for OAuth2 consent (uses existing consent UI from feature 20251217-120100)
- OAuth2 server administration UI
- Integration testing with actual third-party OAuth2 providers
- Performance optimization for high-volume OAuth2 traffic (baseline implementation)

## Dependencies

- **Agent Registry** (from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)): Must have agents registered with unique client_id values for OAuth2 client validation
- **User Grant Store** (from [20251217-120000-domain-model-apis](../20251217-120000-domain-model-apis/spec.md)): Must be operational to check for active user consent grants
- **Consent Frontend** (from [20251217-120100-consent-frontend](../20251217-120100-consent-frontend/spec.md)): Must be available to handle consent collection when users haven't granted access
- **User Authentication**: Must provide session management via upstream authentication proxy, with principal extraction handled by existing RequirePrincipalMiddleware
- **Enduser HTTP Server** (from [20251215-120000-dual-port-server](../20251215-120000-dual-port-server/spec.md)): Must be operational to host the OAuth2 endpoints
- **Upstream OAuth2 Authorization Server**: Must be configured, operational, and reachable from the identity broker for proxying authorization and token requests

## Assumptions

- The upstream OAuth2 authorization server is configured, operational, and accessible from the identity broker
- The upstream OAuth2 server has a valid TLS certificate signed by a recognized certificate authority
- The upstream OAuth2 server uses standard RFC 6749 OAuth2 endpoints and response formats
- Each agent in the agent registry has a unique client_id that matches the OAuth2 client_id registered with the upstream OAuth2 server
- The identity broker's agent client credentials (client_id, client_secret) are already registered with the upstream OAuth2 server
- User authentication is handled by upstream proxy (oauth2-proxy, nginx, etc.) which sets a principal header (configurable, default X-Remote-User) that is extracted by RequirePrincipalMiddleware
- The consent UI (feature 20251217-120100) correctly preserves and passes back the full redirect_uri parameter containing the original OAuth2 authorization request
- The `server.enduser.public_url` configuration accurately reflects how external clients access the identity broker
- Network connectivity between the identity broker and upstream OAuth2 server is reliable with reasonable latency
- OAuth2 clients are configured with the identity broker's OAuth2 endpoints (not the upstream server directly)
- The upstream OAuth2 server validates redirect_uri and other security-critical parameters
- Grant expiration (valid_until) is enforced by treating expired grants as non-existent
- The system handles standard OAuth2 flows (authorization code, refresh token) - other flows are out of scope for this feature
- Error responses from the upstream OAuth2 server are already in RFC 6749 compliant format
- The identity broker does not need to inspect or modify OAuth2 tokens - they are treated as opaque values during proxying
- The upstream OAuth2 server implements rate limiting to protect against brute force and abuse; the broker relies on upstream rate limiting rather than implementing its own
- The upstream OAuth2 server maintains comprehensive audit logs for token endpoint operations (token exchanges, refresh operations); the broker does not duplicate this logging
