# Feature Specification: Protected Resource Discovery for Third-Party OAuth2 Services

**Feature Branch**: `050-oauth2-protected-resource-discovery`  
**Created**: 2026-10-01  
**Status**: Draft  
**Input**: [GitHub issue #21 — Discovery based on OAuth2 Protected Resource Metadata](https://github.com/zalando-incubator/agentic-identity-broker/issues/21)

## Clarifications

### Session 2026-10-01

- Q: Should every service added from a protected-resource URL request tokens limited to that resource, or only services the administrator marks as MCP? → A: For every discovery-backed service, store the discovered resource in `authorization_params.resource` unless the administrator supplies an explicit value. Return the effective value in service responses and send it on every authorization and token request.
- Q: Where should administrators put the protected-resource URL when they create or update a service? → A: Use `discovery.resource_url` with `discovery.enable_discovery: true`. Keep `discovery.metadata_url` for authorization-server metadata only.
- Q: If the authorization server offers both public and confidential dynamic registration, which client should the broker request? → A: Prefer compatible CIMD, then confidential DCR, then public DCR when confidential DCR is not offered. If a selected confidential registration fails, reject creation instead of retrying as public.
- Q: Should two dynamically registered services be allowed to share a client ID when their authorization servers have different issuers? → A: Yes. Dynamic client identity is the issuer and client ID together. Reject a duplicate pair under one issuer and leave existing manual-service uniqueness rules unchanged.
- Q: Should discovery status use the proposed read-only `GET /api/services/{service-id}/discovery-status` resource and its listed fields? → A: Yes. Use the authenticated status resource with `status`, `resource_url`, `issuer_uri`, `client_method`, `last_attempt_at`, `last_success_at`, and a safe `failure_reason`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Connect a service that supports CIMD (Priority: P1)

An administrator enters the URL of a protected third-party service. The broker discovers its authorization server and OAuth2 configuration. If the server supports compatible Client ID Metadata Documents (CIMD), the broker uses its hosted client identity. The existing proof key for code exchange (PKCE) protection remains in place. A user can connect without an administrator entering provider endpoints or client credentials.

**Why this priority**: This is the main path for a compatible MCP service and removes manual client setup.

**Independent Test**: Register a resource whose authorization server supports CIMD and dynamic client registration (DCR). Complete an account connection. Confirm that the broker chose CIMD without registering a second client.

**Acceptance Scenarios**:

1. **Given** one advertised issuer, **When** an administrator sets `discovery.resource_url` and `discovery.enable_discovery: true` without `issuer_uri`, **Then** the broker saves that issuer and its endpoints.
2. **Given** compatible CIMD and DCR, **When** an administrator registers the resource, **Then** the broker uses hosted CIMD and makes no DCR registration.
3. **Given** a discovered service without an explicit resource override, **When** a user connects, **Then** authorization and code exchange send `resource` equal to its configured URL. PKCE remains mandatory.
4. **Given** two advertised issuers, **When** an administrator selects one, **Then** the broker uses only that issuer's discovered endpoints.
5. **Given** a `401` Bearer or DPoP challenge with one `resource_metadata` value, **When** an administrator registers the resource, **Then** the broker uses that URL and accepts only matching metadata.
6. **Given** a path issuer with both OpenID locations but no OAuth2 metadata, **When** an administrator registers, **Then** path-inserted OpenID metadata wins.
7. **Given** a path issuer with two missing metadata locations, **When** an administrator registers, **Then** path-appended OpenID metadata supplies the endpoints.
8. **Given** a root issuer without OAuth2 metadata, **When** an administrator registers, **Then** root OpenID metadata supplies the endpoints.
9. **Given** a path-qualified resource without a challenge, **When** an administrator registers, **Then** path-specific Protected Resource Metadata takes priority over root metadata.
10. **Given** a discovered service without an override, **When** an administrator reads it, **Then** `authorization_params.resource` equals its verified resource URL.

---

### User Story 2 - Connect a service through dynamic registration (Priority: P1)

An administrator enters a protected resource URL whose authorization server does not support compatible CIMD. The server offers DCR instead. The broker uses its registered client identity for account connection. If the provider issues a refresh token and accepts that identity, the broker also uses it for renewal.

**Why this priority**: DCR makes discovery useful for services that do not accept the broker's hosted CIMD identity.

**Independent Test**: Register a DCR-only resource that issues a refresh token. Connect an account, renew its access after restart, and confirm that the original client identity remains in use.

**Acceptance Scenarios**:

1. **Given** only compatible DCR, **When** an administrator registers the resource, **Then** the service stores one working client identity from the server.
2. **Given** DCR returns a public client, **When** a user connects, **Then** the broker uses PKCE and sends no client secret.
3. **Given** DCR returns a supported confidential client and the provider issues a refresh token, **When** a user connects and renews after restart while its credential remains valid, **Then** the same client identity works without disclosing its secret.
4. **Given** a DCR-backed session without an override needs renewal, **When** the broker requests a token, **Then** `resource` equals the configured URL.
5. **Given** a discovery-backed service with an administrator-set `authorization_params.resource`, **When** a user connects and renews, **Then** each request sends exactly that value once.
6. **Given** an administrator supplied a different `authorization_params.resource`, **When** they read the service, **Then** that value is visible and the discovery URL remains separate.
7. **Given** an authorization server offers confidential and public DCR, **When** an administrator registers its resource, **Then** the broker selects confidential registration.
8. **Given** an authorization server offers only public DCR, **When** an administrator registers its resource, **Then** the broker selects a public client with PKCE.
9. **Given** two authorization servers issue the same DCR client ID, **When** an administrator registers both services, **Then** both remain usable under their distinct issuers.

---

### User Story 3 - Reject unsuitable or unsafe discovery (Priority: P1)

If the URL does not identify a valid protected resource or safe client identity, the broker reports an error. It does not create a partial service.

**Why this priority**: An invalid discovery source must never create an integration that sends users or credentials to the wrong server.

**Independent Test**: Attempt registrations with invalid metadata, an unsafe URL, and unsupported bootstrap. Confirm that no service is created. Then reject an MCP connection's `resource` parameter and confirm that the broker makes no broader request.

**Acceptance Scenarios**:

1. **Given** no Protected Resource Metadata, **When** an administrator registers the resource with manual endpoints, **Then** registration fails and no service exists.
2. **Given** a resource or issuer identity mismatch, **When** an administrator registers the resource, **Then** registration fails before accepting a client identity.
3. **Given** a discovered URL resolves to an internal address, **When** an administrator registers, **Then** no connection to that address occurs and registration fails.
4. **Given** no compatible CIMD or DCR, **When** an administrator registers, **Then** registration fails without a static client fallback.
5. **Given** a selected CIMD identity is unavailable, **When** an administrator registers, **Then** registration fails without trying DCR.
6. **Given** two advertised issuers without a selection, **When** an administrator registers, **Then** registration fails without choosing an issuer.
7. **Given** the first metadata document reports another issuer, **When** an administrator registers the resource, **Then** registration fails without trying lower-priority metadata locations.
8. **Given** an authorization server rejects a discovery-backed service's `resource`, **When** a user connects, **Then** the broker fails without a broader token request.
9. **Given** an invalid explicit `authorization_params.resource`, **When** an administrator creates a discovery-backed service, **Then** registration fails without storing a service.
10. **Given** both `discovery.resource_url` and `discovery.metadata_url`, **When** an administrator registers a service, **Then** the broker rejects conflicting discovery sources.
11. **Given** an advertised confidential DCR registration rejects the broker, **When** an administrator creates the service, **Then** creation fails without a public retry.
12. **Given** a DCR client ID already belongs to a service at the same issuer, **When** another service registers it, **Then** creation fails without changing either credential.

---

### User Story 4 - Inspect and refresh discovery (Priority: P2)

An administrator reads a service's discovery status through the Admin API. The status identifies the selected server, the chosen bootstrap method, and the last attempt. The administrator can request discovery again through a service update. A failed update leaves the active integration unchanged.

**Why this priority**: Operators need to distinguish a successful setup from a later discovery failure without exposing credentials.

**Independent Test**: Read the status after setup and after a failed update. Confirm that the status reports both outcomes while the previous client identity and endpoints remain usable.

**Acceptance Scenarios**:

1. **Given** successful discovery, **When** an administrator reads `GET /api/services/{service-id}/discovery-status`, **Then** it reports `ready`, resource, issuer, client method, and success time without secrets.
2. **Given** a previously ready service, **When** refresh fails, **Then** active settings stay unchanged and status separates the failure from the last success.
3. **Given** a manual service, **When** an administrator reads its status, **Then** it reports `not_applicable` and account connection behavior remains unchanged.
4. **Given** a ready service, **When** an administrator refreshes unchanged issuer and method, **Then** updated endpoints appear and the client identity stays unchanged.
5. **Given** a failed refresh with an earlier successful setup, **When** the broker restarts, **Then** status retains both outcomes and the active client still works.
6. **Given** a derived resource value, **When** an administrator changes the discovery URL for the same issuer, **Then** `authorization_params.resource` changes to the newly verified resource.
7. **Given** an explicit resource override, **When** an administrator refreshes discovery without changing the override, **Then** `authorization_params.resource` remains administrator-selected.
8. **Given** an explicit override, **When** an administrator updates `authorization_params` without `resource`, **Then** the returned value becomes the verified discovery URL.

### Edge Cases

- If resource metadata is missing, malformed, too large, or unavailable within the discovery time limit, the broker rejects new discovery-backed registrations.
- If resource metadata omits authorization servers, the broker rejects the registration. The URL alone cannot establish a trusted issuer.
- If metadata advertises multiple authorization servers and the administrator selects none, the broker rejects the request instead of choosing one arbitrarily.
- If all applicable OAuth2 and OpenID Connect metadata locations are absent, the broker rejects registration. A returned document with an invalid issuer or endpoints fails without fallback.
- If the authorization server does not accept RFC 8707 `resource`, account connection or renewal fails. Successful metadata discovery does not guarantee token issuance.
- If DCR requires an initial access token or returns an unsupported client authentication method, the broker rejects automatic registration.
- If a challenge has duplicate, malformed, or unsupported `resource_metadata`, the broker fails without a well-known fallback. A `resource_metadata` value on a response other than `401` also fails.
- If the provider issues no refresh token, a connection can succeed but later renewal fails without re-registration or authentication downgrade.
- If DCR returns a non-zero `client_secret_expires_at`, the broker rejects registration even when the expiry is in the future. It stores no local service or credential and does not retry with another client method.
- If a selected CIMD method lacks a usable broker-hosted identity or signing key, the broker rejects registration even when DCR is also advertised.
- If discovery fails after the provider created a remote DCR registration, the broker creates no usable local service. Removal of the remote registration depends on that provider's own management facilities.
- If a service is deleted, its discovery status is no longer available. Existing rules for service deletion still apply.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Administrators MUST select either existing direct authorization-server configuration or a protected resource URL in `discovery.resource_url` with `discovery.enable_discovery: true`. Existing `discovery.metadata_url` MUST continue to identify authorization-server metadata only.
- **FR-002**: A request with `discovery.resource_url` MUST reject `discovery.metadata_url`, manual OAuth2 endpoints, and caller-supplied client credentials as fallbacks. An optional `issuer_uri` MUST match one issuer advertised by that resource.
- **FR-003**: The broker MUST probe the protected resource without credentials. It MUST use `resource_metadata` only from a `401` Bearer or DPoP `WWW-Authenticate` challenge with exactly one occurrence. Duplicate values MUST fail, even when identical. Malformed or unsupported challenges with `resource_metadata` MUST fail. A value on any other response status MUST fail. None of these failures may trigger a well-known fallback. If no `resource_metadata` is present, a resource with a path MUST try its path-specific RFC 9728 location before the root location. For `https://mcp.example.com/mcp`, those locations are `https://mcp.example.com/.well-known/oauth-protected-resource/mcp` and `https://mcp.example.com/.well-known/oauth-protected-resource`, in that order. It MUST try the root only after a path-specific `404 Not Found`. A root resource uses the root location. Creation MUST fail if no valid Protected Resource Metadata exists.
- **FR-004**: The broker MUST require an exact match between the configured resource identifier and the identifier in its metadata. The metadata MUST name at least one authorization server. If it names one, the broker MUST select that issuer without requiring `issuer_uri`. If it names several, the administrator MUST select one of them.
- **FR-005**: For issuer `https://auth.example.com/tenant`, the broker MUST try `https://auth.example.com/.well-known/oauth-authorization-server/tenant`, then `https://auth.example.com/.well-known/openid-configuration/tenant`, then `https://auth.example.com/tenant/.well-known/openid-configuration`. For root issuer `https://auth.example.com`, it MUST try `https://auth.example.com/.well-known/oauth-authorization-server`, then `https://auth.example.com/.well-known/openid-configuration`. The broker MUST continue only after a candidate returns `404 Not Found`. A returned document MUST exactly match the selected issuer and provide usable authorization and token endpoints. Invalid documents and other errors MUST fail creation without trying another candidate.
- **FR-006**: A successful discovery MUST save the configured resource, selected issuer, discovered authorization and token endpoints, selected client identity, and chosen bootstrap method. These values MUST remain available after a restart.
- **FR-007**: The broker MUST prefer its existing hosted CIMD confidential client when the chosen authorization server advertises CIMD and supports the broker's authentication method. It MUST NOT invoke DCR when compatible CIMD succeeds.
- **FR-008**: If no compatible CIMD option exists, the broker MUST prefer DCR with a compatible confidential client method. It MUST use public DCR with PKCE only when confidential DCR is not offered or has no compatible method. If neither DCR option is usable, creation MUST fail. A failed selected confidential registration MUST NOT trigger a public retry.
- **FR-009**: DCR MUST register the broker's actual callback and obtain a client identity usable with its authorization-code flow. It MUST reject an empty client identifier, incompatible callback, a returned authentication mode that differs from the selected mode, or any non-zero `client_secret_expires_at`. A failed selected registration MUST NOT trigger another client method.
- **FR-010**: A DCR-created client MUST retain its returned identifier and required credential for subsequent connections. If the provider issues a refresh token and accepts the credential, renewal MUST use the same registered identity. If it issues no refresh token or rejects the credential, renewal MUST fail without re-registration, an authentication downgrade, or a broader retry. Ordinary account connections MUST NOT create a new registration.
- **FR-011**: Discovery-backed connections MUST keep authorization-code and PKCE safeguards. Every authorization request and token request, including code exchange and refresh, MUST contain exactly one RFC 8707 `resource` parameter. Its value MUST equal the persisted `authorization_params.resource`. If the authorization server rejects it, connection or renewal MUST fail. The broker MUST NOT retry without `resource` or store a replacement token from that failed request.
- **FR-012**: A failed create MUST leave no usable service or stored client credential. A failed update MUST preserve the previous active issuer, endpoints, client identity, and user sessions.
- **FR-013**: An explicit update of a discovery-backed service MUST repeat metadata validation. It MUST preserve both `discovery.client_method` and the exact `token_endpoint_auth_method`, including on an explicit issuer change. When the issuer stays the same, it MUST also preserve the client identity. An issuer change MUST require explicit selection of an advertised issuer and zero user sessions. If the new issuer cannot support the stored authentication method, the update MUST fail without registration fallback or changes to the active service. A confidential client MUST NOT become public.
- **FR-014**: Discovery failures MUST NOT cause an automatic fallback to static credentials, manual endpoints, a different issuer, or another authentication method after one has been selected.
- **FR-015**: Manually configured services MUST keep their current validation, update, connection, and token-renewal behavior without a discovery request or automatic registration.
- **FR-016**: Discovery status MUST distinguish the last attempt from the last successful configuration after a restart. A manual service MUST report `not_applicable` without an invented attempt.
- **FR-017**: On discovery-backed creation, the broker MUST materialize the verified resource in `authorization_params.resource` unless an administrator supplies a different value. An explicit value MUST be a non-empty absolute URI without a fragment. The broker MUST retain whether the stored value is derived or explicit. On a discovery URL change, it MUST update a derived value and preserve an explicit value. A replacement `authorization_params` object without `resource` MUST remove an explicit override and restore the verified resource. End users MUST NOT override the effective value.
- **FR-018**: A dynamically registered client's identity MUST be the selected issuer URI together with its client ID. Identical DCR client IDs from different issuers MUST be allowed. A duplicate pair under one issuer MUST reject creation without replacing existing credentials. Existing manual-service uniqueness rules MUST remain unchanged.

### Domain Model

- **Third-party OAuth2 Service** is the existing service aggregate. It owns one discovery source, one active issuer and client identity, and an effective resource value marked as derived or administrator-supplied.
- **Protected Resource Metadata** is a claim by a resource about its own identity and accepted authorization servers. Its resource identifier must match the configured URL.
- **Authorization Server Metadata** describes one advertised issuer and the capabilities required for OAuth2 account connection.
- **Client Registration** is the selected client identity for one service. It is either the broker's hosted CIMD identity or a DCR-created identity, not both. A DCR identity includes its issuer, client ID, and public or confidential mode.
- **Discovery Status** records the last attempt, last successful setup, chosen issuer, and chosen client method. A failed refresh does not replace the service's active configuration.
- **User Session** remains the existing connection between a user and a service. This feature does not change its ownership or retention rules.

### API Requirements

- **API-001**: Administrative service create and update requests MUST accept `discovery.resource_url` only with `discovery.enable_discovery: true`. It MUST be mutually exclusive with `discovery.metadata_url` and manual endpoints. The existing `authorization_params.resource` field remains the explicit resource override.
- **API-002**: Administrative service create, read, list, and update representations MUST expose `discovery.resource_url` and the effective `authorization_params.resource` for discovery-backed services. They MUST show the selected `issuer_uri`, `client_id`, bootstrap method, and public or confidential mode without disclosing credentials. For DCR, consumers MUST identify the client by issuer and client ID together.
- **API-003**: The Admin API MUST expose authenticated `GET /api/services/{service-id}/discovery-status` for existing services. Its response MUST include `status`, `resource_url`, `issuer_uri`, and `client_method`. It MUST also include `last_attempt_at`, `last_success_at`, and `failure_reason`. A missing value MUST be null. Status values are `ready`, `failed`, and `not_applicable`. Client methods are `cimd` and `dcr`. Failure reasons MUST omit sensitive data.
- **API-004**: This resource MUST return the latest recorded status without contacting the provider. A failed refresh MUST report `failed` while its issuer and method still describe the active configuration. Its `resource_url` identifies the discovery source. The effective token audience appears in the service's `authorization_params.resource`.
- **API-005**: Administrative error responses MUST identify why metadata discovery or client registration failed without returning remote response bodies, secrets, or other sensitive material.
- **API-006**: The stakeholder confirmed `discovery.resource_url`, the effective `authorization_params.resource`, and the authenticated status path and fields during the 2026-10-01 clarification session. The administrative OpenAPI contract MUST document their validation, success and error responses, and examples before implementation.
- **API-007**: Reading status for an existing service, including a manual service, MUST return success. Reading status for an unknown or deleted service MUST return not found.

### Database Requirements

- **DB-001**: A service's discovery source, selected issuer, endpoints, bootstrap method, public or confidential client mode, client identifier, effective resource, and whether that resource is derived or explicit MUST survive a restart. Existing manual services MUST remain unchanged.
- **DB-002**: Any DCR client secret MUST remain protected at rest and usable after a restart. The broker MUST discard registration-management credentials that the supported client lifecycle does not use.
- **DB-003**: Successful creation or refresh MUST make its active configuration and successful status visible together. No reader can observe a partially updated client identity.
- **DB-004**: Failed creation MUST leave no usable local service or stored credential. Failed refresh MUST keep the prior active configuration, last success, and user sessions while saving the attempt time and safe failure reason.
- **DB-005**: Existing service and session records MUST remain readable with their original authentication behavior. Deleting a service MUST remove its local discovery status and DCR credentials.
- **DB-006**: Dynamically registered services MUST preserve issuer-scoped client identities across restarts. A duplicate pair under one issuer MUST not replace a service or its credential.

### Security Requirements

- **SR-001**: All supplied and discovered remote locations MUST use public HTTPS origins in production. The broker MUST block internal, loopback, link-local, cloud-metadata, and redirected private destinations before making connections.
- **SR-002**: An unanswered discovery or registration attempt MUST finish within 15 seconds. Each remote document MUST be at most 256 KiB. Invalid documents, untrusted redirects, and identity mismatches MUST fail closed.
- **SR-003**: A DCR secret and any required registration-management credential MUST remain protected at rest. The broker MUST discard unused management credentials. Neither administrative responses nor status records MUST disclose them.
- **SR-004**: The broker MUST authenticate with only the selected client's supported method. It MUST NOT downgrade after CIMD or confidential DCR fails during creation, connection, or renewal.
- **SR-005**: Discovery, registration, rejection, and refresh attempts MUST produce audit records with service identity and outcome. These records MUST NOT contain client secrets, assertions, tokens, or raw remote error bodies.
- **SR-006**: A DCR client's credential MUST be selected by its service and verified issuer. The broker MUST NOT choose a credential using a client ID alone.

### Key Entities

- **Third-party OAuth2 Service**: Existing operator-managed service. Its discovery source, selected issuer, endpoints, and client identity remain linked throughout its lifetime.
- **Protected Resource**: The URL that an administrator supplies. The resource's published metadata identifies the resource and its authorization servers.
- **Authorization Server**: A resource-advertised issuer. Its metadata supplies OAuth2 endpoints and client-registration capabilities.
- **Client Registration**: The broker-hosted CIMD identity or a DCR-issued identity. A DCR identity is confidential with a protected secret or public without one.
- **Client Registration Identity**: For DCR, the selected issuer and returned client ID identify one client. An identical ID from another issuer identifies a different client.
- **Discovery Status**: A service-specific record of the latest attempt and last success. It contains no client credentials.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: At least 9 of 10 administrators in a guided setup trial can register a conforming single-issuer resource without entering OAuth2 endpoints or client credentials.
- **SC-002**: In 100% of acceptance cases where a server offers both compatible CIMD and DCR, service setup chooses CIMD and creates zero dynamic client registrations.
- **SC-003**: In 100% of acceptance cases where a DCR-only provider issues a refresh token and accepts the same registered credential after restart, a user can connect and renew with that client identity.
- **SC-004**: All tested URLs without valid Protected Resource Metadata fail registration and leave zero new services, even when manual endpoint values are supplied.
- **SC-005**: All tested failed refreshes preserve the previous active service configuration and expose zero secrets in administrative results and audit records.
- **SC-006**: In a trial of 20 independent registrations, at least 19 complete within five seconds when each external response takes at most one second.
- **SC-007**: In a guided status-review trial, at least 9 of 10 administrators can identify the chosen issuer and client method and distinguish a failed refresh from the last successful setup.
- **SC-008**: In 100% of tested restarts after failed refresh, status retains the failed attempt and last success, and the previously active client remains usable.
- **SC-009**: In 100% of tested discovery-backed connections and renewals, token access is requested for the configured resource or the administrator's explicit override. A rejection never triggers a broader retry.
- **SC-010**: In 100% of tested discovery-backed service responses, `authorization_params.resource` shows the value used for token requests, whether derived or administrator-supplied.
- **SC-011**: In 100% of tested setups with both public and confidential DCR, the broker registers a confidential client. Rejected confidential registration never causes a public retry.
- **SC-012**: Two tested DCR services with identical client IDs from different issuers can each connect with their own credentials. Same-issuer duplicates create no service.

## Assumptions

- The administrator supplies a protected resource identifier, such as an MCP service URL, not the authorization server's metadata URL. This feature follows the resource's published metadata to its authorization server.
- This feature extends [outbound CIMD client authentication](../046-cimd-upstream-client/spec.md). A compatible provider accepts the existing hosted confidential client method. This feature does not add a CIMD public-client mode.
- Automatic DCR applies to registration endpoints that accept a broker-initiated registration without a separate initial access token. A provider that needs prior approval remains eligible for the existing manual service setup.
- The broker requests a supported confidential client through DCR before it considers a public client. Public DCR requires PKCE and applies only when compatible confidential registration is not offered. New client authentication methods and automatic credential rotation are outside this feature.
- A DCR response with a non-zero secret expiry is not supported because this feature has no automatic credential rotation. A provider that revokes a non-expiring secret can still reject later connection or renewal; the broker does not re-register automatically.
- Client IDs issued through DCR are scoped to the selected issuer. Manual service registration keeps its existing client-ID uniqueness behavior.
- Discovery runs during create or an explicit service update. There is no scheduled re-discovery, automatic re-registration, new administration user interface, or automatic change to protected-resource ownership and permission sets.
- The resource URL can identify the target of an account connection. Administrators continue to manage separate protected-resource ownership under the existing service rules.
- An administrator-supplied `authorization_params.resource` can differ from the resource URL used for discovery. It sets the token audience, not the metadata source or protected-resource ownership.
- A rejected creation has no service and no status resource. A failed refresh of an existing service records its failure without replacing the active configuration.
- A resource's metadata can be valid even when its authorization server rejects the `resource` parameter later. The broker reports the connection failure and does not treat discovery status as proof of token compatibility.
- A server that silently ignores `resource` while returning an opaque token cannot be proven resource-bound from discovery alone. This feature assumes authorization servers for discovery-backed services honor RFC 8707. It does not claim independent verification of opaque token audiences.
- This specification draws its protocol terms from [RFC 9728](https://www.rfc-editor.org/rfc/rfc9728), [RFC 8414](https://www.rfc-editor.org/rfc/rfc8414), [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591), [RFC 8707](https://www.rfc-editor.org/rfc/rfc8707), and the [MCP authorization specification (2026-07-28)](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization). The current MCP rules for [authorization-server discovery](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/authorization-server-discovery) and [client registration](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization/client-registration) are in separate sections.
