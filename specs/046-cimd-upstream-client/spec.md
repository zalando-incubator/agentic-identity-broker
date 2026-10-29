# Feature Specification: CIMD Client Authentication for Third-Party OAuth2 Services

**Feature Branch**: `046-cimd-upstream-client`

**Created**: 2026-09-17

**Status**: Draft
**Increment 2 (issue #264)**: 2026-10-22. This increment augments feature 046 in this directory.

**Input**: User description: "I would like to add CIMD support to the third-party OAuth2 services, i.e. act as OAuth2 Client. This means that the Broker needs to host a metadata document and the services should allow a configuration that represents a confidential client without specifying a client credential"

## Context

This feature makes the broker an OAuth2 client that identifies itself to the authorization server of a registered third-party OAuth2 service through a Client ID Metadata Document (CIMD). It is the outbound counterpart to [CIMD support](../028-cimd-support/spec.md), which lets the broker act as an authorization server that consumes a client's metadata document. It does not relax the existing inbound-CIMD prohibition in `proxy` mode: that prohibition remains unchanged for clients that authenticate to the broker.

It extends, rather than supersedes, [public third-party client support](../042-thirdparty-public-pkce/spec.md). A public service declares `none` and sends no client authentication. This feature adds the distinct `private_key_jwt` mode: a confidential service whose identity and public verification keys are published by the broker, while no shared client secret is configured or stored.

The `private_key_jwt` choice is protocol-driven: [CIMD draft §8.2](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-8.2) identifies it as a confidential-client method when public keys are published through `jwks_uri`; [§4.1](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-4.1) prohibits shared secrets and private key material in the document. This feature applies only when the selected third-party OAuth2 service supports that profile.

### Increment 2 (issue #264): CIMD public clients

CIMD draft-02 [§4.1](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-4.1) excludes shared-secret methods from the document, but permits public clients. [§8.2](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-client-id-metadata-document-02#section-8.2) recommends authentication for clients that can authenticate. The broker therefore keeps `private_key_jwt` as its recommended CIMD method. A service whose authorization server requires `none`, such as ServiceNow, can explicitly select CIMD public registration.

## Clarifications

### Session 2026-09-17

- Q: Which OAuth server modes must support outbound CIMD confidential services? → A: `proxy`, `local`, and `hybrid`; proxy mode uses broker-managed signing and public-key publication independent of upstream proxy behavior.
- Q: Which signing-key topology must CIMD confidential services use? → A: One mode-independent, broker-global signing-key set shared by all CIMD confidential services; its existing rotation lifecycle applies uniformly.
- Q: Should the specified public metadata and JWK Set URLs, plus the administrative service contract, be approved as written? → A: Yes. Approve `/.well-known/oauth-client/{service-id}`, `/.well-known/oauth-client/{service-id}/jwks.json`, and the documented create, update, read, and list semantics.

### Session 2026-09-18

- Q: Which key set signs CIMD client assertions? → A: A dedicated broker-global CIMD client-authentication key set, separate from the local token-signing keys. This refines the 2026-09-17 topology decision: "broker-global" means one set shared by all CIMD confidential services, not the key set that signs broker-issued access tokens. It reuses the existing generation, activation-grace, and retirement mechanisms.
- Q: What is the activation policy for CIMD key rotation? → A: Mirror the existing mechanism. A newly generated current key waits the activation grace window before it signs, while operator-initiated promotion of an already-created key takes effect immediately.
- Q: What is the rotation trigger and management surface for CIMD keys? → A: Operator-triggered only, through a dedicated CIMD key administrative route set wired in all three OAuth server modes. The broker performs no scheduled rotation, and the existing token-signing key routes are not reused.
- Q: Where should the CIMD client key administrative routes live? → A: A flat, unnamespaced `/api/cimd-client-keys` collection. The routes belong to neither the broker's authorization-server namespace nor an individual third-party service, so they are not nested under `/api/oauth2-server/` or `/api/services/`.
- Q: What are the request and response shapes for the CIMD key routes? → A: Mirror the existing signing-key representation — `kid`, `algorithm`, `is_current`, `activates_at`, `created_at` — with generation accepting an optional `algorithm` that defaults to `ES256` and rejects any other value, private key material never returned, and the existing administrative error representation reused.

### Session 2026-10-22 — Increment 2 (issue #264)

- Issue #264 approves independent `client_registration` values `static` and `cimd`, the four valid combinations, the two rejected combinations, omission resolution, read/list reporting, and classification of stored services (API-013).
- Stakeholder confirmation (API-014, 2026-10-22): D-A rejects explicit `null`, empty, and unknown registration values with `400`.
- Stakeholder confirmation (API-014, 2026-10-22): D-B rejects supplied registration during protected-resource discovery and reports `cimd` for hosted selection or `static` for DCR. Discovery never selects CIMD public.
- Stakeholder confirmation (API-014, 2026-10-22): D-C defines the five-member, keyless CIMD public document and the uniform JWK Set `404`.
- Stakeholder confirmation (API-014, 2026-10-22): D-D sets admin API version 2.1.0 and end-user API version 1.1.0.
- This increment augments feature 046 in place. The stakeholder explicitly accepted the decision now in ADR 044 (previously numbered 042, then 043), including D-E–D-J and the D-I rolling-upgrade limitation, on 2026-10-22.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Register a CIMD confidential service (Priority: P1)

An operator configures a third-party OAuth2 service whose authorization server accepts CIMD-based confidential clients. The operator does not provide a client identifier or shared client secret: the broker assigns the identifier and uses its dedicated, broker-global CIMD client-authentication key set, while making only the corresponding public verification keys available to the service's authorization server.

**Why this priority**: The service cannot use CIMD authentication until the operator can represent it safely and unambiguously.

**Independent Test**: Register a service with broker-managed confidential authentication and no client identifier or shared secret. Read the service back and confirm it identifies the service by a broker-hosted metadata URL, reports `private_key_jwt`, and reports no stored shared secret.

**Acceptance Scenarios**:

1. **Given** an administrator configures a compatible third-party OAuth2 service with `private_key_jwt`, **When** they omit both the client identifier and client secret, **Then** the broker creates the service with a stable, broker-hosted client identifier URL, authenticates with the dedicated broker-global CIMD client-authentication key set, and stores no shared client secret.
2. **Given** an administrator configures a CIMD confidential service, **When** the request includes a caller-supplied client identifier or shared client secret, **Then** the broker rejects the contradictory configuration before contacting the third-party OAuth2 service, because it authenticates only with broker-generated, internally retained signing material.
3. **Given** an administrator configures a service without an authentication method, **When** they provide a non-empty client secret, **Then** the broker keeps the existing static confidential-client behavior unchanged.
4. **Given** an administrator configures a static public service with `none`, **When** they omit a client secret, **Then** the broker keeps the existing static public-client behavior unchanged and does not create CIMD metadata for that service.
5. **Given** a provider variant whose client identity is derived from a credential document, **When** an administrator selects `private_key_jwt`, **Then** the broker rejects the incompatible configuration and identifies the conflicting variant.

---

### User Story 2 - Third-party service discovers broker metadata (Priority: P1)

The authorization server of a registered third-party OAuth2 service receives the broker's client identifier URL and retrieves a Client ID Metadata Document from that exact URL. The document enables that service to identify the broker as a confidential client and to obtain the public keys required to validate its token-endpoint authentication.

**Why this priority**: The document is the interoperable registration record that lets a third-party OAuth2 service authenticate the broker without a pre-shared secret.

**Independent Test**: Fetch a configured service's client identifier URL anonymously and verify that the returned document identifies that same URL, lists the broker callback URI, declares `private_key_jwt`, and links only to public verification keys.

**Acceptance Scenarios**:

1. **Given** a CIMD confidential service, **When** the authorization server of its registered third-party OAuth2 service retrieves the client identifier URL, **Then** it receives a successful JSON metadata document whose `client_id` exactly equals the requested URL.
2. **Given** a CIMD confidential service, **When** its metadata document is retrieved, **Then** it declares the broker's callback URI, `private_key_jwt` token-endpoint authentication, and a URL for the broker's public verification keys.
3. **Given** the authorization server of a registered third-party OAuth2 service retrieves the metadata document or its public keys, **When** it inspects the response, **Then** no client secret, private key material, authorization code, access token, refresh token, or user information is present.
4. **Given** a static public or static confidential service, **When** the authorization server of a registered third-party OAuth2 service requests a CIMD document for it, **Then** no broker-managed CIMD client identity is implied or exposed.
5. **Given** an existing CIMD confidential service with a usable published CIMD key, **When** the authorization server of its registered third-party OAuth2 service retrieves its metadata document, **Then** the response is `200 OK`, uses `application/json`, contains the required document fields, and is cacheable for no longer than five minutes.
6. **Given** an unknown, deleted, static public, static confidential, or CIMD confidential service without a usable published key, **When** a caller requests its CIMD metadata document or public-key document, **Then** the broker responds with `404 Not Found` and no client metadata or key material.
7. **Given** a broker with both CIMD client-authentication keys and local token-signing keys, **When** the two published JSON Web Key Sets are compared, **Then** no key identifier appears in both: the CIMD JWK Set exposes no token-signing key, and the broker's token JWK Set exposes no CIMD client-authentication key.

---

### User Story 3 - Connect with CIMD confidential authentication (Priority: P1)

A user connects an account at a third-party OAuth2 service whose authorization server accepts the broker's CIMD confidential client. The broker starts the existing authorization-code flow with its broker-hosted client identifier. When exchanging the authorization code or refreshing tokens, it proves its identity with a signed client assertion that the service's authorization server can validate from the advertised public keys.

**Why this priority**: This is the end-user value of the feature: the broker obtains and maintains delegated access to a third-party OAuth2 service without a manually managed shared credential.

**Independent Test**: Complete a connect journey against a third-party OAuth2 service whose authorization server obtains the broker metadata, requires a valid `private_key_jwt` assertion, and rejects a static client secret. Confirm that a usable encrypted user session is created and later refreshed.

**Acceptance Scenarios**:

1. **Given** a CIMD confidential service and a third-party OAuth2 service whose authorization server supports it, **When** a user starts the connect journey, **Then** the authorization request identifies the broker with the service's broker-hosted client identifier URL and preserves the existing PKCE protections.
2. **Given** a user returns with a valid authorization code for a CIMD confidential service, **When** the broker exchanges it, **Then** the request to the third-party OAuth2 service's token endpoint uses a valid signed client assertion bound to the service identity and token endpoint, and uses no client secret.
3. **Given** a CIMD confidential service with an expired access token, **When** the broker refreshes the session, **Then** it authenticates the refresh request with an equivalently valid signed client assertion and stores replacement encrypted tokens.
4. **Given** the broker cannot create a valid signed client assertion or the third-party OAuth2 service's authorization server rejects it, **When** token acquisition is attempted, **Then** the journey fails without retrying as a public client or with a static client secret.
5. **Given** a static public or static confidential service, **When** a user connects or refreshes it, **Then** its authorization and token behavior remains unchanged.
6. **Given** a CIMD confidential token request, **When** the authorization server of the third-party OAuth2 service examines it, **Then** the request contains exactly one `client_assertion_type` / `client_assertion` pair, with the JWT-bearer assertion type. The client assertion has issuer and subject equal to the exact client identifier URL, the configured token endpoint as its sole audience, expiry within five minutes, a fresh unique JWT ID, and a signature from an advertised ES256 signing key marked for signing.
7. **Given** the broker operates in `proxy` mode and a CIMD confidential service is registered, **When** a user connects or refreshes that service, **Then** the broker authenticates to the third-party OAuth2 service with its own CIMD signing material and public-key publication, not with the OAuth2 proxy's upstream credentials or keys.
8. **Given** a broker with both key domains provisioned, **When** it signs a CIMD client assertion and a broker-issued access token, **Then** the assertion is signed by a CIMD client-authentication key and the access token by a token-signing key, and neither operation succeeds using a key from the other domain.

---

### User Story 4 - Maintain continuity during key rotation (Priority: P2)

An operator rotates a key in the CIMD client-authentication key set. The authorization servers of registered third-party OAuth2 services can discover the newly usable public key before receiving assertions signed with it, so user sessions continue to connect and refresh without an avoidable authentication outage.

**Why this priority**: Confidential authentication depends on key continuity. Rotation must improve security without breaking active integrations.

**Independent Test**: Rotate a CIMD client-authentication key, retrieve the published public keys before the new key is used, and complete a CIMD confidential token request that the third-party authorization server validates successfully.

**Acceptance Scenarios**:

1. **Given** a CIMD confidential service, **When** an operator rotates a CIMD client-authentication key, **Then** the metadata's public-key reference remains stable and the newly usable verification key is published before any assertion uses it.
2. **Given** a provider has cached the broker's current public keys, **When** it receives an assertion signed with a newly active key, **Then** it can obtain the key through the advertised public-key reference and validate the assertion.
3. **Given** no usable CIMD client-authentication key is available, **When** a CIMD confidential token request is needed, **Then** the broker fails closed and does not send an unsigned, stale, or differently authenticated request.
4. **Given** an operator generates a new CIMD client-authentication key and marks it current, **When** the activation grace window has not yet elapsed, **Then** the key is already present in the published CIMD JSON Web Key Set but signs no assertion, and the previous key continues signing until the window elapses.
5. **Given** an existing CIMD client-authentication key, **When** an operator promotes it, **Then** the promotion takes effect immediately regardless of how long that key has been published, and subsequent assertions are signed with it.
6. **Given** a broker with no CIMD client-authentication key, **When** the broker provisions its first CIMD key, **Then** that key becomes active immediately without waiting for an activation grace window.
7. **Given** the broker operates in any of the three OAuth server modes, **When** an operator lists, generates, promotes, or retires CIMD client-authentication keys, **Then** the dedicated CIMD key routes serve those operations without exposing or modifying token-signing keys, and without making the token-signing key routes reachable in `proxy` mode.
8. **Given** an operator generates a CIMD client-authentication key, **When** the request supplies an `algorithm` other than `ES256`, **Then** the route rejects the request, creates no key, and never advertises or selects a non-`ES256` key for the CIMD domain.

---

### User Story 5 - Review authentication posture (Priority: P3)

An operator reviews service registrations and can distinguish a static public service, a static confidential service, and a CIMD confidential service. The representation explains the selected authentication method and the generated client identifier without exposing sensitive material.

**Why this priority**: Operators must be able to audit a service's authentication posture before trusting it with user-delegated access.

**Independent Test**: List a catalogue containing each original service type and confirm that every entry exposes its authentication method and only CIMD entries expose a broker-hosted client identifier URL; no entry reveals a secret.

**Acceptance Scenarios**:

1. **Given** a catalogue containing static public, static confidential, and CIMD confidential services, **When** an operator lists or reads them, **Then** each service's authentication mode is unambiguous.
2. **Given** a CIMD confidential service, **When** an operator reads it, **Then** the response reports `private_key_jwt`, exposes its stable broker-hosted client identifier URL, and omits any shared-secret value.
3. **Given** a service is changed from static confidential authentication to CIMD confidential authentication, **When** the update succeeds, **Then** the stored client secret is removed and future token requests use the CIMD confidential mode.

---

### Increment 2 (issue #264): CIMD public user stories

### User Story 6 - Register a CIMD public service (Priority: P1)

An operator registers a service whose authorization server accepts a hosted client identifier URL but requires `none` at the token endpoint. The broker assigns the URL without creating a CIMD client-authentication key.

**Why this priority**: ServiceNow-style authorization servers cannot use the existing CIMD confidential mode.

**Independent Test**: Register and replace a CIMD public service without a key. Fetch its document and confirm its assigned URL, method, and absence of a client secret.

**Acceptance Scenarios**:

1. **Given** a compatible service, **When** an operator creates it with `client_registration: cimd` and `token_endpoint_auth_method: none` but no client ID or secret, **Then** the broker returns `201`, assigns its client identifier URL, reports `cimd` and `none`, and omits `client_secret`.
2. **Given** an empty CIMD key set, **When** the operator creates or replaces a CIMD public service, **Then** both requests succeed without creating or requiring a key. A CIMD confidential request still requires a usable key.
3. **Given** a CIMD public create or replacement request, **When** it supplies a client ID or non-empty secret, **Then** the broker returns `400` before any provider traffic and leaves stored state unchanged.
4. **Given** a Google-flavor service or `credential_source: filesystem`, **When** an operator selects `client_registration: cimd`, **Then** the broker returns `400`. Explicit `static` remains valid.
5. **Given** a CIMD service, **When** the operator switches between `private_key_jwt` and `none` in a complete replacement, **Then** the client identifier URL stays the same. Switching to confidential requires a usable CIMD key.
6. **Given** a static service, **When** the operator switches to CIMD public in a complete replacement, **Then** the broker assigns its URL and removes a prior secret only after validation. Switching back requires an operator client ID and ends document serving.

---

### User Story 7 - Preserve existing client registrations (Priority: P1)

An operator can make the registration method explicit without changing existing authentication or discovery behavior. Stored services retain their client identities and sessions.

**Why this priority**: Registration is a new axis and must not silently reinterpret existing services.

**Independent Test**: Create and replace each existing mode with explicit or omitted registration. List migrated services and check discovery classification.

**Acceptance Scenarios**:

1. **Given** valid existing configurations, **When** an operator creates or replaces `static` plus an absent method, `static` plus `none`, or `cimd` plus `private_key_jwt`, **Then** each request succeeds and read/list responses report its registration.
2. **Given** `static` plus `private_key_jwt` or `cimd` plus an absent or `null` method, **When** an operator creates or replaces a service, **Then** the broker returns `400` before discovery or provider traffic and stores no change.
3. **Given** a create or complete replacement without `client_registration`, **When** the method is `private_key_jwt`, **Then** registration resolves to `cimd`. For every other method it resolves to `static`; CIMD public is never inferred.
4. **Given** explicit `client_registration: null`, an empty value, or an unknown value, **When** an operator creates or replaces a service, **Then** the broker returns `400` without contacting the provider.
5. **Given** services stored before this increment, **When** the broker migrates and reads them, **Then** `private_key_jwt` services report `cimd` and all others report `static`, without changing their client IDs, methods, or sessions.
6. **Given** a protected-resource discovery request, **When** the operator supplies `client_registration`, **Then** the broker rejects it before discovery traffic. Hosted-CIMD discovery reports `cimd`; DCR reports `static`. Discovery never selects CIMD public.

---

### User Story 8 - Connect through a CIMD public client (Priority: P1)

Alice connects and refreshes a third-party account when its authorization server requires `none` for a CIMD identity. The broker sends the client identifier URL and PKCE proof but no client authentication.

**Why this priority**: Keyless registration only creates value when a user can acquire and refresh tokens securely.

**Independent Test**: Complete a connect and refresh against an authorization server that rejects every client credential and requires `none` with PKCE S256.

**Acceptance Scenarios**:

1. **Given** a CIMD public service, **When** its authorization server fetches the client identifier URL, **Then** it receives the five-member `none` document with the existing `200` content type and 300-second cache header, without key members.
2. **Given** an empty CIMD key set, **When** the server fetches the CIMD public document and its `jwks.json` URL, **Then** the broker serves the document and returns the uniform JSON `404` for the JWK Set.
3. **Given** Alice starts a CIMD public connect, **When** the broker redirects her to authorize, **Then** the request carries its client identifier URL and a PKCE S256 challenge.
4. **Given** Alice returns with a valid code, **When** the broker exchanges it, **Then** it sends its URL as `client_id` and the `code_verifier` without a secret, assertion, or `Authorization` header. It stores the encrypted session.
5. **Given** the session needs refresh, **When** the broker requests new tokens, **Then** it sends its URL and refresh token without client authentication and stores encrypted replacements.
6. **Given** a provider rejection, **When** the broker exchanges or refreshes a token, **Then** it fails closed without retrying another authentication method or storing rejected tokens.
7. **Given** `proxy`, `local`, or `hybrid` OAuth server mode, **When** Alice connects and refreshes through a CIMD public service, **Then** each mode uses the hosted identity. Proxy mode never uses proxy upstream credentials for that service.
8. **Given** a broker whose only CIMD services are public, **When** it starts and Alice completes a connect and refresh, **Then** the journey succeeds with an empty CIMD key set. Startup creates no CIMD key.

---

### Edge Cases

- What happens when the broker's public base URL is not an HTTPS URL? The broker rejects configuration of any service with `client_registration: cimd` because its client identifier URL and metadata document must be publicly retrievable over HTTPS.
- What happens when the client identifier URL, its document's `client_id`, and the URL used by the third-party OAuth2 service's authorization server differ? The configuration or request is rejected; these values must match exactly.
- What happens when a CIMD confidential service is updated to static confidential authentication? The update requires a non-empty client secret in the same request; the broker does not retain or reuse a previously removed secret.
- What happens when a CIMD confidential service is updated to static public authentication? The broker removes its shared-secret state, ceases CIMD confidential token authentication, and uses the established public-client rules.
- What happens when a third-party OAuth2 service does not support CIMD or `private_key_jwt`? Its authorization server rejects the integration; the broker does not select, negotiate, or downgrade to another authentication method automatically.
- What happens when the authorization server of a third-party OAuth2 service cannot retrieve the metadata document or public keys? The authorization or token request fails closed; the broker does not substitute private key material or a static client secret.
- What happens when CIMD client-authentication keys are rotated? The advertised key location remains stable, only public verification keys are published, and previously active CIMD keys remain available for verification until they are explicitly retired.
- What happens when an operator immediately promotes a CIMD key that a third-party authorization server has not yet refreshed into its cache? The broker signs with the promoted key at once, so that server may reject assertions until it refetches the published key set. The broker cannot control a third party's refresh cadence, so operators should promote a key that has been published for at least one cache period, or use generation with its activation grace window instead.
- What happens when a service is deleted? Its broker-hosted metadata document must no longer identify it as a CIMD client.
- What happens when the broker cannot initialize or load a usable CIMD client-authentication key set while CIMD confidential services are configured? The broker refuses startup in that configuration. When no CIMD confidential service is configured, normal startup remains available. Any later CIMD confidential-service registration or use fails closed until a usable key and its public-key publication are available.

### Increment 2 (issue #264): additional edge cases

- What happens when a `PUT` omits `client_registration` for a CIMD public service? Full replacement resolves it to `static`. Without an operator client ID, the request returns `400` and leaves the service unchanged.
- What happens when no CIMD client-authentication key exists? The broker serves CIMD public metadata and starts with only CIMD public services. CIMD confidential registration still requires a usable key.
- What happens when an operator switches CIMD methods while a provider caches the document? The provider can retain the previous document for up to 300 seconds. The broker never changes the requested authentication method at runtime.
- What happens when protected-resource discovery supports hosted CIMD? Its selection remains `private_key_jwt`. Discovery never selects CIMD public and rejects a supplied `client_registration` before fetching metadata.


## Concrete Example Journeys *(mandatory)*

### Increment 2 (issue #264)

**Synthetic cast**: Morgan (`operator@example.com`) administers the broker. Alice (`alice@example.com`) connects a ServiceNow-style service. Its authorization server is `https://login.example.com` and requires `none` for service `0000`. The broker is `https://broker.example.com`. Service `550e8400-e29b-41d4-a716-446655440000` is CIMD public. Services ending in `0001`, `0002`, and `0003` are static confidential, static public, and CIMD confidential. All IDs, credentials, codes, JWE values, assertions, and tokens in these journeys are synthetic. Times are UTC on 2026-10-22. Journeys 3–6 start from this initial state independently. A switch in Journey 4 does not carry into the others.

### Journey 1 — Morgan registers and reviews four modes (US1, US5, US6-S1/S3, US7-S1/S3)

**Step 1 — Broker configuration (excerpt, other required proxy settings omitted)**

```yaml
server:
  enduser:
    public_url: "https://broker.example.com" # base for both CIMD client identifier URLs
oauth2_authorization_server:
  mode: "proxy"                       # outbound CIMD works independently of proxy credentials
```

**Step 2 — Morgan registers the ServiceNow-style service at 09:00**

```http
POST /api/services HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com
Content-Type: application/json

{"display_name":"ServiceNow-style public","client_registration":"cimd","token_endpoint_auth_method":"none","oauth2_flavor":"standard","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"}}

HTTP/1.1 201 Created
Content-Type: application/json

{"id":"550e8400-e29b-41d4-a716-446655440000","display_name":"ServiceNow-style public","credential_source":"stored","client_registration":"cimd","token_endpoint_auth_method":"none","oauth2_flavor":"standard","client_id":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440000","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"},"created_at":"2026-10-22T09:00:00Z","updated_at":"2026-10-22T09:00:00Z"}
```

The broker does not store a service secret. The response omits `client_secret`.

**Step 3 — Morgan lists all four modes**

```http
GET /api/services HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com

HTTP/1.1 200 OK
Content-Type: application/json
```

The response is a JSON array of Service objects. These fields show the four entries. Other required Service fields match step 2.

|Service ID suffix|`client_registration`|`token_endpoint_auth_method`|`client_id`|`client_secret`|
|---|---|---|---|---|
|`0000`|`cimd`|`none`|Broker URL ending in `0000`|Omitted|
|`0001`|`static`|`null`|`operator-confidential`|`REDACTED`|
|`0002`|`static`|`none`|`operator-public`|Omitted|
|`0003`|`cimd`|`private_key_jwt`|Broker URL ending in `0003`|Omitted|

**Observable result**: Morgan can distinguish registration from token-endpoint authentication. A CIMD public create with a supplied `client_id` or non-empty `client_secret` returns `400` before provider traffic.

### Journey 2 — Alice connects and refreshes without client authentication (US2, US8-S1–S6)

**Step 1 — The provider fetches the hosted document without a CIMD key**

```http
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: public, max-age=300

{"client_id":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440000","redirect_uris":["https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"none"}
```

```http
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440000/jwks.json HTTP/1.1
Host: broker.example.com

HTTP/1.1 404 Not Found
Content-Type: application/json

{"error":"not found","message":"CIMD service unavailable"}
```

**Step 2 — Alice starts authorization with PKCE S256 at 09:05**

```http
GET /api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com

HTTP/1.1 302 Found
Location: https://login.example.com/authorize?response_type=code&client_id=https%3A%2F%2Fbroker.example.com%2F.well-known%2Foauth-client%2F550e8400-e29b-41d4-a716-446655440000&redirect_uri=https%3A%2F%2Fbroker.example.com%2Fapi%2Fthird-party%2F550e8400-e29b-41d4-a716-446655440000%2Foauth2%2Fcallback&code_challenge=E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM&code_challenge_method=S256&state=synthetic-jwe-state
```

The challenge matches the synthetic verifier in step 3. The broker seals the verifier in the provider-facing JWE state.

**Step 3 — Alice returns with an authorization code and the broker exchanges it**

```http
GET /api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/callback?code=synthetic-code&state=synthetic-jwe-state HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com
```

The broker exchanges the code with the provider before replying to the browser.

```http
POST /token HTTP/1.1
Host: login.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=synthetic-code&redirect_uri=https%3A%2F%2Fbroker.example.com%2Fapi%2Fthird-party%2F550e8400-e29b-41d4-a716-446655440000%2Foauth2%2Fcallback&client_id=https%3A%2F%2Fbroker.example.com%2F.well-known%2Foauth-client%2F550e8400-e29b-41d4-a716-446655440000&code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk

HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"synthetic-access-1","refresh_token":"synthetic-refresh-1","token_type":"Bearer","expires_in":3600}
```

```http
HTTP/1.1 302 Found
Location: https://broker.example.com/sessions?success=true&service_id=550e8400-e29b-41d4-a716-446655440000
```

The token request contains no `client_secret`, `client_assertion`, `client_assertion_type`, or `Authorization` header.

**Step 4 — Alice refreshes at 10:05 and reads her encrypted session**

```http
POST /api/third-party/550e8400-e29b-41d4-a716-446655440000/session/refresh HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com

POST /token HTTP/1.1
Host: login.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&refresh_token=synthetic-refresh-1&client_id=https%3A%2F%2Fbroker.example.com%2F.well-known%2Foauth-client%2F550e8400-e29b-41d4-a716-446655440000

HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"synthetic-access-2","refresh_token":"synthetic-refresh-2","token_type":"Bearer","expires_in":3600}
```

The upstream refresh request has no client secret, assertion, or `Authorization` header. The broker refresh endpoint returns `200`.

```http
GET /api/third-party/550e8400-e29b-41d4-a716-446655440000/session HTTP/1.1
Host: broker.example.com
X-Remote-User: alice@example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"data":{"session":{"session_id":"660e8400-e29b-41d4-a716-446655440001","is_expired":false,"has_refresh_token":true,"tokens_encrypted":true},"dependent_agents":[]}}
```

The session response shows selected fields. The broker stores the replacement access and refresh tokens encrypted and never returns their plaintext in this response.

### Journey 3 — A CIMD confidential service signs its token request (US3-S1/S2/S6)

**Step 1 — Morgan registers the confidential mode after provisioning a usable CIMD key**

```http
POST /api/services HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com
Content-Type: application/json

{"display_name":"Signed client","client_registration":"cimd","token_endpoint_auth_method":"private_key_jwt","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"}}

HTTP/1.1 201 Created
Content-Type: application/json

{"id":"550e8400-e29b-41d4-a716-446655440003","client_registration":"cimd","token_endpoint_auth_method":"private_key_jwt","client_id":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003","display_name":"Signed client","credential_source":"stored","oauth2_flavor":"standard","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"},"created_at":"2026-10-22T09:10:00Z","updated_at":"2026-10-22T09:10:00Z"}
```

**Step 2 — The provider reads the confidential document and validates the assertion**

```http
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003 HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: public, max-age=300

{"client_id":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003","redirect_uris":["https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440003/oauth2/callback"],"grant_types":["authorization_code","refresh_token"],"response_types":["code"],"token_endpoint_auth_method":"private_key_jwt","token_endpoint_auth_signing_alg":"ES256","jwks_uri":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003/jwks.json"}
```

Alice completes the same PKCE authorization flow for service `0003`. The broker sends one signed assertion at the token endpoint.

```http
POST /token HTTP/1.1
Host: login.example.com
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code&code=synthetic-signed-code&code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk&redirect_uri=https%3A%2F%2Fbroker.example.com%2Fapi%2Fthird-party%2F550e8400-e29b-41d4-a716-446655440003%2Foauth2%2Fcallback&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer&client_assertion=synthetic.signed.jwt

HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"synthetic-signed-access","refresh_token":"synthetic-signed-refresh","token_type":"Bearer","expires_in":3600}
```

The synthetic marker `synthetic.signed.jwt` represents this decoded ES256 assertion. It is not a usable credential.

```json
{"header":{"alg":"ES256","kid":"synthetic-cimd-kid","typ":"JWT"},"claims":{"iss":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003","sub":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003","aud":"https://login.example.com/token","iat":1792660500,"exp":1792660800,"jti":"synthetic-unique-assertion-id"}}
```

The expiry is 300 seconds after issuance. No client secret accompanies this request.

### Journey 4 — Morgan switches modes and handles rejections (US6-S5/S6, US7-S2/S4/S6)

**Step 1 — Morgan replaces service `0000` with CIMD confidential authentication**

```http
PUT /api/services/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com
Content-Type: application/json
If-Match: "synthetic-service-etag"

{"display_name":"ServiceNow-style public","client_registration":"cimd","token_endpoint_auth_method":"private_key_jwt","oauth2_flavor":"standard","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"}}

HTTP/1.1 200 OK
Content-Type: application/json

{"id":"550e8400-e29b-41d4-a716-446655440000","client_registration":"cimd","token_endpoint_auth_method":"private_key_jwt","client_id":"https://broker.example.com/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440000","display_name":"ServiceNow-style public","credential_source":"stored","oauth2_flavor":"standard","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"},"created_at":"2026-10-22T09:00:00Z","updated_at":"2026-10-22T09:30:00Z"}
```

This replacement requires a usable CIMD key. Replacing the method with `none` and retaining `client_registration: cimd` keeps the same URL without requiring a key.

**Step 2 — Morgan tests a rejected replacement and a static identity switch**

```http
PUT /api/services/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com
Content-Type: application/json

{"display_name":"ServiceNow-style public","client_registration":"static","token_endpoint_auth_method":"private_key_jwt","issuer_uri":"https://login.example.com","discovery":{"enable_discovery":false},"endpoints":{"authorize_endpoint":"https://login.example.com/authorize","token_endpoint":"https://login.example.com/token"}}

HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed","message":"invalid client registration and token endpoint authentication method"}
```

Explicit `client_registration: null`, an empty value, and an unknown value also return `400`. Protected-resource discovery rejects the field before fetching metadata. A `PUT` without the field resolves to `static`. It cannot silently retain CIMD public registration.

Morgan next replaces service `0000` with `client_registration: static`, `token_endpoint_auth_method: none`, and `client_id: operator-public`. The successful response reports that client ID, and the old hosted document returns `404`. A switch from static confidential to CIMD public removes its old secret only after validation. Static confidential replacement requires a new non-empty secret.

**Step 3 — Alice reconnects after the client ID changes**

The old encrypted refresh token remains in Alice's session. A refresh under `operator-public` receives a generic provider rejection. The broker does not retry with the old URL or another method. Alice starts the authorization journey again and receives new tokens for `operator-public`.

### Journey 5 — Morgan rotates a CIMD confidential key (US4-S1–S8)

**Step 1 — Morgan generates a second CIMD key**

```http
POST /api/cimd-client-keys HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com
Content-Type: application/json

{"algorithm":"ES256"}

HTTP/1.1 201 Created
Content-Type: application/json

{"kid":"synthetic-next-cimd-kid","algorithm":"ES256","is_current":true,"activates_at":"2026-10-22T10:05:00Z","created_at":"2026-10-22T09:55:00Z"}
```

**Step 2 — The provider reads the JWK Set before activation**

```http
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003/jwks.json HTTP/1.1
Host: broker.example.com

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: public, max-age=300

```

The JSON `keys` array contains the old and newly published ES256 public keys with distinct `kid` and coordinate values. It contains no private material. The old key signs assertions until the generated key activates. Morgan can promote an already-published key through `PUT /api/cimd-client-keys/{kid}/current`. That change takes effect immediately. The CIMD public service `0000` never uses either key. The token-signing JWK Set remains separate.

### Journey 6 — A proxy broker starts with only CIMD public services (US8-S7/S8)

**Step 1 — Morgan starts the broker in `proxy` mode with service `0000` only**

The broker uses the proxy configuration excerpt in Journey 1. Its persisted CIMD client-authentication key list is empty.

```http
GET /api/cimd-client-keys HTTP/1.1
Host: broker.example.com:14000
X-Remote-User: operator@example.com

HTTP/1.1 200 OK
Content-Type: application/json

{"items":[]}
```

**Step 2 — Alice completes the Journey 2 connect and refresh**

The broker serves the hosted `none` document and returns `404` for its JWK Set. The provider accepts both secretless token requests. The broker stores Alice's encrypted session. The CIMD key list stays empty. The same journey works with `local` and `hybrid` modes, without using proxy upstream credentials.

## Requirements *(mandatory)*

### Functional Requirements

**Service identity and configuration**

- **FR-001**: A third-party OAuth2 service MUST support `private_key_jwt` as a token-endpoint authentication method in addition to the existing public (`none`) and static confidential (absent or `null`) modes.
- **FR-002**: A service configured with `private_key_jwt` MUST be treated as a confidential client and MUST use a broker-assigned, stable HTTPS client identifier URL. It MUST NOT depend on an operator-supplied static client identifier.
- **FR-003**: A service configured with `private_key_jwt` MUST be registrable and updatable without a client secret. The broker MUST use a private key from its internally generated, dedicated, broker-global CIMD client-authentication key set as the confidential-client credential and MUST NOT accept or store a shared client secret for that service.
- **FR-004**: A CIMD confidential-service request that supplies a client identifier or non-empty client secret MUST be rejected as contradictory before a request to the third-party OAuth2 service is made.
- **FR-005**: A provider variant whose identity depends on a credential document MUST reject `private_key_jwt` because it cannot supply that identity without the credential.
- **FR-006**: Static public (`none`) and static confidential (absent or `null`) service semantics MUST remain unchanged. The new authentication method MUST NOT be inferred from an omitted method or selected automatically from provider metadata. Explicit `client_registration: cimd` with `none` adds the CIMD public exception.
- **FR-007**: Updating a static confidential service to `private_key_jwt` MUST remove its stored client secret only after the complete replacement configuration validates. Updating back to static confidential authentication MUST require a new non-empty client secret in the same request.

**Hosted Client ID Metadata Document**

- **FR-008**: For every existing, non-deleted CIMD confidential service with a usable published CIMD key, the broker MUST anonymously serve a Client ID Metadata Document from its stable client identifier URL over HTTPS.
- **FR-009**: The metadata document's `client_id` MUST exactly equal its retrieval URL and MUST identify the broker as `private_key_jwt` client authentication.
- **FR-010**: The metadata document MUST publish the broker callback URI used for that service and a stable public-key URL that the authorization server of the third-party OAuth2 service can use to validate client assertions.
- **FR-011**: The metadata document and public-key response MUST contain public information only. They MUST NOT contain shared secrets, private key material, authorization codes, access tokens, refresh tokens, or user data.
- **FR-012**: A metadata or public-key response for a deleted, static public, static confidential, or CIMD confidential service without a usable published key MUST NOT identify the broker as an active CIMD confidential client. A CIMD public service remains eligible for its metadata document without a key but never serves a JWK Set.

**Authorization and token acquisition**

- **FR-013**: The existing authorization-code flow and PKCE protections MUST remain mandatory for CIMD confidential services.
- **FR-014**: Every authorization-code and refresh token request for a CIMD confidential service MUST use client authentication consistent with `private_key_jwt`, including a signed assertion bound to the broker's client identifier and the third-party OAuth2 service's token endpoint.
- **FR-015**: A CIMD confidential token request MUST NOT send a client secret by a request body, authorization header, query parameter, or any other credential channel.
- **FR-016**: If metadata serving, key availability, assertion creation, or third-party OAuth2 service assertion validation fails, the broker MUST fail the affected operation closed. It MUST NOT retry as a public client, with a static secret, with a different authentication method, or with an unsigned assertion.
- **FR-017**: A signed assertion used for CIMD confidential authentication MUST use only a currently usable private key from the dedicated, broker-global CIMD client-authentication key set retained inside the broker. Its public counterpart MUST be available from the metadata document's public-key URL before the assertion is used; the private key MUST never be accepted from an operator or published.

**Operations, compatibility, and auditability**

- **FR-018**: Service read and list representations MUST distinguish static public, static confidential, CIMD confidential, and CIMD public services through registration and authentication fields. Both CIMD modes MUST report the broker-hosted client identifier URL and omit any client-secret value.
- **FR-019**: The client identifier URL and public-key URL for a CIMD confidential service MUST remain stable across normal service updates and signing-key rotations.
- **FR-020**: Existing public and static confidential services, including their stored user sessions, MUST remain usable without operator changes after this feature is introduced.
- **FR-021**: Creating, updating, serving metadata for, authenticating, or rejecting a CIMD confidential service MUST emit structured audit records sufficient to identify the service and outcome without exposing credentials, private keys, signed assertions, authorization codes, or tokens.

**Protocol contract**

- **FR-022**: The client identifier URL for a CIMD confidential service MUST be `<public-base-url>/.well-known/oauth-client/<service-id>`, where `<public-base-url>` is the broker's configured public HTTPS URL and `<service-id>` is the service's immutable identifier. It MUST remain stable until the service is deleted.
- **FR-023**: An anonymous `GET` of that client identifier URL MUST return `200 OK`, `Content-Type: application/json`, and `Cache-Control: public, max-age=300` for an existing, non-deleted CIMD confidential service with a usable published CIMD key. Its JSON document MUST contain exactly the broker-assigned `client_id`, the one broker callback URL for that service in `redirect_uris`, `grant_types` of `authorization_code` and `refresh_token`, `response_types` of `code`, `token_endpoint_auth_method` of `private_key_jwt`, `token_endpoint_auth_signing_alg` of `ES256`, and a `jwks_uri` under that client identifier URL.
- **FR-024**: An anonymous `GET` of `<client-id>/jwks.json` MUST return `200 OK`, `Content-Type: application/json`, and `Cache-Control: public, max-age=300`. Its `keys` array MUST expose only public ES256 verification keys from the dedicated CIMD client-authentication key set that are valid for signing, including a newly published key during its activation overlap and any prior key retained for verification. Each key MUST identify `use` as `sig`, include its `kid`, and include no private material.
- **FR-025**: Requests to the metadata document or public-key URL for an unknown, deleted, static public, static confidential, or CIMD confidential service without a usable published key MUST return `404 Not Found` with the existing JSON error representation. The JWK Set URL for a CIMD public service MUST return the same `404`. An eligible CIMD public metadata request instead returns its keyless document. None of these unavailable requests can return a redirect, partial document, or key material.
- **FR-026**: Each CIMD confidential token request MUST contain exactly one `client_assertion_type` / `client_assertion` pair. The type value MUST be `urn:ietf:params:oauth:client-assertion-type:jwt-bearer`. The assertion MUST be an ES256-signed JWT whose `iss` and `sub` each exactly equal the service client identifier URL, whose sole `aud` is the configured token endpoint URL, whose `exp` is no more than five minutes after issuance, and whose `jti` is fresh and unique for that assertion. Its signing key MUST have `use: sig`, a matching advertised `kid`, and no symmetric or unsigned alternative.
- **FR-027**: CIMD confidential services MUST be supported in `proxy`, `local`, and `hybrid` OAuth server modes. The broker MUST provide a dedicated, broker-global CIMD client-authentication key lifecycle, metadata document, and public-key publication for this outbound third-party OAuth2 service feature, independently of local token issuance and independently of the OAuth2 proxy's upstream keys and credentials. This MUST NOT relax the existing prohibition on inbound CIMD authorization-server behavior in `proxy` mode.
- **FR-028**: Before accepting a CIMD confidential-service registration or traffic in any OAuth server mode, the broker MUST ensure a usable CIMD client-authentication key and its public verification-key publication are available. If CIMD confidential services are configured at startup and this prerequisite fails, the broker MUST fail startup closed. Otherwise, it MUST reject later CIMD confidential-service registration or use until the prerequisite is available; it MUST NOT affect services in other authentication modes.
- **FR-029**: The CIMD client-authentication key set MUST be a distinct key domain from the broker's local token-signing keys. A CIMD key MUST NOT sign broker-issued access tokens, a token-signing key MUST NOT sign a client assertion, and neither key set MUST appear in the other's published JSON Web Key Set. The CIMD key set MUST hold only ES256 keys, and a key of any other algorithm MUST NOT be selected for signing or advertised for verification.
- **FR-030**: CIMD key activation MUST follow the broker's existing three paths. The first CIMD key provisioned for an empty CIMD key domain MUST become active immediately. A later generated current key MUST appear in the published CIMD JSON Web Key Set from creation and MUST NOT sign any assertion until its activation grace window elapses, during which the previous key continues signing. Operator-initiated promotion MUST take effect immediately, regardless of how long the target key has been published. A retired CIMD key MUST remain published for verification until it is explicitly removed.
- **FR-031**: CIMD key rotation MUST be operator-triggered through a dedicated administrative surface that is available in `proxy`, `local`, and `hybrid` modes. The broker MUST NOT rotate CIMD keys on an automatic schedule. That surface MUST operate only on the CIMD key domain, and its availability MUST NOT make the token-signing key routes reachable in `proxy` mode.

**Increment 2 (issue #264): registration and public-client requirements**

- **FR-032**: Each service MUST have an independent `client_registration` setting with exactly `static` and `cimd` values.
- **FR-033**: `client_registration` MUST select the source of the client identity. `token_endpoint_auth_method` MUST independently select how the broker authenticates at the token endpoint.
- **FR-034**: Manual services MUST accept exactly `static` with absent or `null` method, `static` with `none`, `cimd` with `private_key_jwt`, and `cimd` with `none`.
- **FR-035**: The broker MUST reject `static` with `private_key_jwt` and `cimd` with absent or `null` method using `400` before discovery or provider traffic, without changing stored state.
- **FR-036**: Omitted registration on create or full replacement MUST resolve to `cimd` for `private_key_jwt` and `static` otherwise. Explicit `null`, empty, or unknown values MUST return `400`. The broker MUST NOT infer CIMD public.
- **FR-037**: A CIMD public service MUST use the broker-assigned HTTPS client identifier URL with `none` and MUST NOT store or send a client credential.
- **FR-038**: Creating, replacing, serving, and using a CIMD public service MUST NOT require, create, or block on a CIMD client-authentication key.
- **FR-039**: A CIMD public document MUST return the FR-023 status and headers. Its exact members MUST be `client_id`, one broker callback in `redirect_uris`, `grant_types` of `authorization_code` and `refresh_token`, `response_types` of `code`, and `token_endpoint_auth_method: none`.
- **FR-040**: A CIMD public document MUST omit `jwks_uri` and `token_endpoint_auth_signing_alg`. Its `jwks.json` URL MUST return the uniform JSON `404` from FR-025.
- **FR-041**: CIMD public authorization MUST carry its client identifier URL and PKCE S256 challenge. Code exchange and refresh MUST send that URL without a secret, assertion, or `Authorization` header. Code exchange MUST send the verifier. Successful tokens MUST use existing encrypted session storage.
- **FR-042**: A rejected CIMD public token request MUST fail closed without a retry under another authentication method or storage of rejected tokens. Existing OAuth2 session retry classification MUST remain unchanged.
- **FR-043**: The Google flavor and `credential_source: filesystem` MUST reject `client_registration: cimd`. Explicit `static` MUST remain valid for both.
- **FR-044**: Protected-resource discovery requests MUST omit `client_registration`. Hosted-CIMD selection MUST report `cimd`. DCR selection MUST report `static`. Existing selection rules MUST NOT select CIMD public.
- **FR-045**: Service create, read, update, and list responses MUST always include the resolved registration. Existing stored `private_key_jwt` services MUST become `cimd`. All other stored services MUST become `static`, without changing client IDs, methods, or sessions.
- **FR-046**: Full replacement MUST preserve the URL between CIMD confidential and CIMD public. Switching static to CIMD MUST assign the URL and remove an old secret only after validation. Switching CIMD to static MUST require an operator client ID, and static confidential also requires a new non-empty secret. The hosted document MUST then return `404`. Providers can cache the previous document for 300 seconds.
- **FR-047**: Startup MUST reject a stored CIMD service whose client ID differs from `<server.enduser.public_url>/.well-known/oauth-client/<service-id>`. The broker MUST bootstrap and require a usable CIMD key only when a CIMD confidential service exists.
- **FR-048**: CIMD public services MUST work in `proxy`, `local`, and `hybrid` modes. Proxy mode MUST use the hosted client identity, not proxy upstream credentials.
- **FR-049**: Lifecycle audit records for CIMD public creation, update, and rejection MUST say "CIMD public service" without credentials, codes, assertions, or tokens. Metadata serving MUST retain its single "CIMD metadata access" record. Token acquisition MUST use the existing `public_client: true` session audit records.


### API Requirements

- **API-001**: The administrative service create and full-replacement update contracts MUST accept `private_key_jwt` as the explicit CIMD confidential authentication method. For that method, the broker-assigned client identifier replaces an operator-supplied identifier and a shared client secret is forbidden.
- **API-002**: The administrative service read and list contracts MUST represent a CIMD confidential service with its `private_key_jwt` method and broker-hosted client identifier URL, while omitting the client-secret field.
- **API-003**: The public contract MUST document the anonymously retrievable metadata document and public-key response for existing, non-deleted CIMD confidential services with usable published keys, including response semantics and the fact that they disclose public information only.
- **API-004**: The stakeholder approved the administrative and public contract changes in API-001 through API-003 during the 2026-09-17 clarification session. Implementation MUST document the approved contract in the administrative and end-user OpenAPI specifications and rendered end-user API documentation, as required by the project constitution.
- **API-005**: The public metadata contract MUST use `GET /.well-known/oauth-client/{service-id}` as the stable client identifier URL. For an eligible CIMD confidential service it MUST return FR-023. For an eligible CIMD public service it MUST return FR-039. For other service states it MUST return FR-025.
- **API-006**: The public-key contract MUST use `GET /.well-known/oauth-client/{service-id}/jwks.json`. For an eligible CIMD confidential service it MUST return FR-024. CIMD public and other unavailable states MUST return FR-025.
- **API-007**: The administrative create and full-replacement update contracts MUST accept exactly `none`, `private_key_jwt`, `null`, or an omitted authentication method. With `private_key_jwt`, `client_id` and a non-empty `client_secret` are forbidden; the successful read and list representation exposes the generated client identifier URL and omits `client_secret`.
- **API-008**: The administrative contract MUST expose the CIMD key route set at a flat `/api/cimd-client-keys` collection: `POST /api/cimd-client-keys` to generate, `GET /api/cimd-client-keys` to list, `PUT /api/cimd-client-keys/{kid}/current` to promote, and `DELETE /api/cimd-client-keys/{kid}` to retire. These routes MUST be available in all three OAuth server modes, MUST operate only on the CIMD key domain, MUST reuse the existing administrative authentication and retirement guards, and MUST never return private key material. They MUST NOT be nested under `/api/oauth2-server/`, which scopes the broker's authorization-server role, or under `/api/services/`, which would imply per-service scope.
- **API-009**: The stakeholder approved the `/api/cimd-client-keys` paths, their four operations, and their request and response shapes during the 2026-09-18 clarification session. A key representation MUST carry `kid`, `algorithm`, `is_current`, `activates_at`, and `created_at`, and MUST NOT carry private key material. Generation MUST accept an optional `algorithm` that defaults to `ES256` and MUST reject any other value, so the CIMD domain never inherits the token-signing path's tolerance for other algorithms, per FR-029. Error responses MUST reuse the existing administrative error representation. The approved contract MUST be documented in the administrative OpenAPI specification before implementation, as required by the project constitution.

**Increment 2 (issue #264): API requirements**

- **API-010**: Admin create and full-replacement requests MUST accept optional, non-null `client_registration` with the FR-034 matrix. Admin create, read, update, and list responses MUST include its resolved, required value.
- **API-011**: The admin OpenAPI contract MUST document omission, explicit-value rejection, discovery selection, transitions, and `400` errors. Its version MUST become 2.1.0.
- **API-012**: The end-user OpenAPI contract MUST describe two closed CIMD metadata variants and the CIMD public JWK Set `404`. Its version MUST become 1.1.0.
- **API-013**: Issue #264 approves the independent registration field, two values, mode matrix, omission rule, response reporting, and stored-service classification.
- **API-014**: Before implementation, the stakeholder MUST confirm D-A (invalid explicit values), D-B (discovery exclusion and reporting), D-C (keyless document and JWK Set `404`), and D-D (API versions) in writing. The stakeholder confirmed all four on 2026-10-22.


### Persistence Requirements

- **DB-001**: The service record MUST durably preserve static public, static confidential, CIMD confidential, and CIMD public modes. It MUST retain the broker-assigned client identifier for both CIMD modes.
- **DB-002**: Persisted state MUST prevent either CIMD mode from retaining a shared client secret. Existing static public and static confidential records MUST retain their meaning.
- **DB-003**: Existing CIMD confidential rollback protections MUST remain. Migration 044 MUST refuse rollback when a CIMD public service exists, rather than reinterpret its broker-hosted URL as an operator client ID.
- **DB-004**: CIMD client-authentication keys MUST be persisted with their key domain distinguishable from token-signing keys, with private material encrypted at rest exactly as token-signing key material is. The storage layer MUST enforce at most one current CIMD key independently of the current token-signing key, and MUST preserve retired CIMD keys for verification until they are explicitly removed. Any migration introducing this domain MUST be reversible when no CIMD keys exist and MUST refuse rollback rather than discard CIMD key material when they do.

**Increment 2 (issue #264): persistence requirements**

- **DB-005**: The service record MUST persist a non-null registration. Migration 045 MUST classify existing `private_key_jwt` rows as `cimd` and all other rows as `static` without changing other service or session fields.
- **DB-006**: A separate persisted constraint MUST enforce the registration and method matrix, secret absence, stored credential source, discovery-method compatibility, and a CIMD client ID suffix. The domain and startup check MUST enforce the exact public URL.
- **DB-007**: Migration 044 MUST refuse rollback while a CIMD public row exists. Migration 045 rollback MUST retain registration values and leave its constraint unvalidated for replay.


### Key Entities *(include if feature involves data)*

- **CIMD Confidential Service**: A third-party OAuth2 service that the broker authenticates with `private_key_jwt` rather than a shared client secret. It has a broker-assigned client identifier URL and no stored shared secret.
- **Broker-hosted Client ID Metadata Document**: The publicly retrievable registration document for one CIMD service. It binds the stable client identifier URL, callback URI, and authentication method. Only the CIMD confidential variant includes a public-key reference.
- **Client Assertion**: A short-lived signed statement used by the broker to authenticate a CIMD confidential service to a third-party OAuth2 service's token endpoint. Its verification key is published through the service metadata.
- **Third-party OAuth2 Service**: The existing registered external OAuth2 service. Its client registration and token-endpoint authentication are independent. Static confidential, static public, CIMD confidential, and CIMD public are the four manual modes.

**Increment 2 (issue #264): key entities**

- **Client Registration**: The service setting that selects a static operator client identity or a broker-hosted CIMD client identifier URL. It is independent of token-endpoint authentication.
- **CIMD Public Service**: A service with `client_registration: cimd` and `token_endpoint_auth_method: none`. It uses the hosted URL and PKCE without a client-authentication key or secret.


## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An operator can register a CIMD confidential service in one administrative request without providing a client identifier or shared client credential.
- **SC-002**: 100% of metadata retrieval checks for an existing, non-deleted CIMD confidential service with a usable published key return a document whose identifier matches the retrieval URL, declares `private_key_jwt`, includes a callback URI and public-key reference, and exposes no private material.
- **SC-003**: A complete user connect and refresh journey succeeds against a conformant third-party OAuth2 service whose authorization server requires valid CIMD `private_key_jwt` authentication and rejects shared client secrets.
- **SC-004**: 100% of tested failures in metadata availability, key availability, assertion generation, or assertion validation terminate without an authentication downgrade or a token being stored.
- **SC-005**: After a signing-key rotation, a provider can validate newly issued client assertions without a service configuration change or user re-registration.
- **SC-006**: All existing public-client and static-confidential third-party-service acceptance scenarios continue to pass without changes to their operator configuration.
- **SC-007**: Across service representations, metadata responses, public-key responses, audit records, and error messages exercised by this feature, zero shared secrets, private keys, signed assertions, authorization codes, or tokens are exposed.
- **SC-008**: In the defined normal-load measurement, 100 successful anonymous requests to each public metadata and JWK route run with ten concurrent clients after one warm-up request per route. The p95 retrieval latency for each route MUST be less than one second.

**Increment 2 (issue #264): measurable outcomes**

- **SC-009**: An operator can register a CIMD public service in one request without a client ID, credential, or CIMD key.
- **SC-010**: A user can connect and refresh against an authorization server that requires `none` and rejects every client credential. The broker stores encrypted tokens after each success.
- **SC-011**: Omitted-registration requests and previously stored services retain their behavior. Every `private_key_jwt` service reports `cimd`. Every other existing service reports `static`.
- **SC-012**: 100% of invalid registration-method combinations fail before outbound traffic and leave stored service state unchanged.
- **SC-013**: A broker with only CIMD public services starts and completes connect and refresh with an empty CIMD key set.


## Assumptions

- The registered third-party OAuth2 service supports the current OAuth Client ID Metadata Document draft or a compatible profile and its authorization server accepts `private_key_jwt` for a pre-registered client identifier URL.
- The broker has a stable public HTTPS base URL. Operators remain responsible for registering the generated client identifier URL with the third-party OAuth2 service when its authorization server requires explicit allow-listing.
- The broker's CIMD client-authentication key lifecycle is mode-independent and broker-global: it generates and protects a dedicated internal private key set in `proxy`, `local`, and `hybrid` modes, separate from the keys that sign broker-issued access tokens. That key set is distinct from a shared `client_secret`, which is neither accepted nor stored for an individual service; only its public verification keys are published.
- The user request selects `private_key_jwt` as the one supported asymmetric CIMD authentication method. Other key-based methods remain out of scope; this selection is grounded in the CIMD draft's `private_key_jwt` example and in the broker's existing ES256 key lifecycle mechanisms.
- Administrative configuration is currently API-driven; no new administration user interface is required.
- The public metadata and JWK Set URLs, together with the documented administrative create, update, read, and list semantics, were approved by the stakeholder during the 2026-09-17 clarification session.

## Dependencies

- [Feature 028 — CIMD Support](../028-cimd-support/spec.md): terminology, exact client-identifier matching, and the existing client-metadata interoperability baseline. This feature is outbound and does not change Feature 028's inbound authorization-server behavior.
- [Feature 042 — Public Client Support](../042-thirdparty-public-pkce/spec.md): established `none` semantics, credential absence rules, PKCE invariant, and compatibility behavior. This feature adds a separate confidential mode and does not alter public-client semantics.
- The broker's existing key lifecycle mechanisms — generation, encryption at rest, activation grace, retirement guards, and JSON Web Key Set publication. The CIMD client-authentication key set reuses these mechanisms; it does not reuse the local token-signing key domain itself.
- A conformant third-party OAuth2 service test double: its authorization server must retrieve CIMD metadata and public keys, require a valid `private_key_jwt` assertion, and reject shared-secret authentication.

### Increment 2 (issue #264)

- [ADR 044 — Client Registration Independent of Authentication](../../adrs/044-client-registration-independent-of-authentication.md): Accepted by the stakeholder on 2026-10-22, including D-E–D-J and the D-I rolling-upgrade limitation.


## Out of Scope

- Changing the broker's existing inbound CIMD authorization-server support or the SSRF protections, cache rules, consent UI, and agent-registration rules defined by Feature 028.
- Dynamic client registration, automatic provider capability discovery, or automatic negotiation of token-endpoint authentication methods.
- Other client authentication methods, including `client_secret_basic`, `client_secret_post`, `client_secret_jwt`, mutual TLS, DPoP, or attestation-based authentication.
- Per-service private signing keys, regardless of whether they are operator-managed or broker-generated; per-service key selection or rotation; and importing private key material through service configuration.
- Changing how the broker encrypts, stores, exchanges, revokes, or otherwise manages third-party user tokens.
- Automatic or scheduled rotation of CIMD client-authentication keys; rotation is operator-triggered only.
- A new administration user interface for service registration or key management.

### Increment 2 (issue #264)

- Discovery-selected CIMD public services. Existing protected-resource discovery can select hosted CIMD confidential or DCR, but not CIMD public.
- Runtime fallback between `none` and `private_key_jwt` after provider rejection. An operator explicitly selects the method.