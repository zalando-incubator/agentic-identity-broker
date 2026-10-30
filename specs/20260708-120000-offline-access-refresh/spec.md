# Feature Specification: Offline Access Refresh Tokens

**Feature Branch**: `20260708-120000-offline-access-refresh`
**Created**: 2026-07-08
**Status**: Draft
**Input**: User description: "retroactively create a spec for the executed plan and the implementation that was done on this branch."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Obtain offline access without re-authenticating (Priority: P1)

As an OAuth2 client using broker-issued tokens, I want to request offline access during authorization so that I can continue acting on behalf of the user after the original access token expires, without forcing the user through another authorization flow.

**Why this priority**: This is the core outcome of the feature. Without it, the broker cannot support long-lived delegated access for clients that need background or delayed work.

**Independent Test**: Can be fully tested by completing an authorization-code flow with offline access requested, confirming a refresh token is returned, then redeeming that refresh token for a replacement access token.

**Acceptance Scenarios**:

1. **Given** the broker is operating in a mode that issues its own tokens, **and** a client is allowed to use refresh tokens, **when** the client requests `offline_access` during authorization and successfully exchanges the authorization code, **then** the token response includes both an access token and a refresh token.
2. **Given** a client has received a valid refresh token, **when** the client redeems it before it expires, **then** the broker issues a replacement access token and a replacement refresh token without requiring user interaction.
3. **Given** a client has already used a refresh token successfully, **when** it attempts to reuse that same refresh token, **then** the broker rejects the request as an invalid grant.

---

### User Story 2 - Prevent refresh token issuance for ineligible requests (Priority: P2)

As an operator or relying party, I want refresh tokens to be issued only for eligible clients and eligible authorization requests so that offline access is explicit, least-privilege, and predictable.

**Why this priority**: Offline access is a privileged capability. The broker must not mint refresh tokens implicitly or for clients that have not declared support for them.

**Independent Test**: Can be fully tested by running otherwise identical authorization flows where either the offline-access scope is omitted or the client does not declare refresh-token support, and confirming no refresh token is issued.

**Acceptance Scenarios**:

1. **Given** a client completes a successful authorization-code flow without requesting `offline_access`, **when** the broker returns the token response, **then** no refresh token is included.
2. **Given** a client requests `offline_access` but does not declare refresh-token support, **when** the broker completes the token exchange, **then** no refresh token is included.
3. **Given** a client attempts to use the refresh-token grant without possessing a valid refresh token, **when** the broker evaluates the request, **then** the broker rejects it as an invalid request or invalid grant.

---

### User Story 3 - Discover offline-access capability by deployment mode (Priority: P3)

As an integrator or operator, I want the broker’s metadata to accurately describe whether offline access and refresh tokens are supported in the current deployment mode so that clients can configure themselves correctly and avoid unsupported behavior.

**Why this priority**: Capability discovery prevents misconfiguration, especially in mixed deployments where some modes issue local tokens and others proxy upstream behavior.

**Independent Test**: Can be fully tested by requesting the broker’s authorization-server metadata in each deployment mode and verifying the advertised scope and grant-type capabilities match the mode’s behavior.

**Acceptance Scenarios**:

1. **Given** the broker is operating in a locally issuing mode, **when** an integrator fetches the authorization-server metadata, **then** the metadata advertises support for the refresh-token grant and the `offline_access` scope.
2. **Given** the broker is operating in proxy mode, **when** an integrator fetches the authorization-server metadata with default configuration, **then** the metadata does not claim offline-access support unless that support has been explicitly configured.
3. **Given** the broker is operating in proxy or hybrid mode, **when** a client sends an upstream-compatible refresh-token request for a proxied client, **then** the broker forwards that request without changing the grant semantics.

---

### Edge Cases

- What happens when a client requests `offline_access` together with client-specific scopes that remain allowed, even though `offline_access` itself is not part of the client’s normal scope allowlist?
- How does the broker behave when a rotated refresh token is replayed after a newer refresh token has already been issued?
- What happens when a public client discovered via metadata omits refresh-token support but still requests `offline_access`?
- How does metadata behave when the broker can forward upstream refresh-token behavior but cannot safely advertise that capability by default?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The broker MUST issue a refresh token for a successful authorization-code exchange only when the granted scopes include `offline_access` or `offline` and the client declares support for the refresh-token grant.
- **FR-002**: The broker MUST allow eligible clients to redeem a valid refresh token for a replacement access token without requiring the user to repeat the authorization flow.
- **FR-003**: The broker MUST rotate refresh tokens on successful use so that the previously used refresh token becomes invalid.
- **FR-004**: The broker MUST reject reuse of a rotated or revoked refresh token as an invalid grant.
- **FR-005**: The broker MUST NOT issue a refresh token when `offline_access` is not requested, even if the rest of the authorization flow succeeds.
- **FR-006**: The broker MUST NOT issue a refresh token to a client that does not declare support for the refresh-token grant.
- **FR-007**: The broker MUST treat `offline_access` and `offline` as broker-reserved authorization scopes that are permitted during authorization even when they are absent from a client’s normal allowlisted scopes.
- **FR-008**: The broker MUST persist enough refresh-token state to enforce expiration, single use, and request-chain revocation across token refreshes.
- **FR-009**: The broker MUST support the refresh-token grant for locally issued tokens in local mode and for locally issued clients in hybrid mode.
- **FR-010**: The broker MUST preserve transparent refresh-token passthrough behavior for proxied clients in proxy mode and in the proxy portion of hybrid mode.
- **FR-011**: The broker MUST advertise `refresh_token` in authorization-server metadata for deployment modes that issue local refresh tokens.
- **FR-012**: The broker MUST advertise `offline_access` in authorization-server metadata for deployment modes where offline access is supported by default.
- **FR-013**: The broker MUST allow operators to configure which scopes are advertised in authorization-server metadata.
- **FR-014**: The broker MUST allow operators to configure the lifetime of locally issued refresh tokens.
- **FR-015**: The broker MUST default local and hybrid deployments to advertising `offline_access` and supporting the refresh-token grant.
- **FR-016**: The broker MUST default proxy deployments to not advertising offline-access support unless explicitly configured.
- **FR-017**: The broker MUST continue to differentiate client capability by client type, including public clients whose declared capabilities come from their metadata document.

### Configuration Requirements *(if applicable - document before implementation)*

**Configuration Parameters**:
- **supported_scopes**: List of scopes advertised in authorization-server metadata; defaults to `offline_access` for local and hybrid deployments and to an empty list for proxy deployments.
- **local.refresh_token_ttl**: Duration for how long locally issued refresh tokens remain redeemable before expiring.

**Example YAML Configuration**:
```yaml
oauth2_authorization_server:
  mode: local
  supported_scopes:
    - offline_access
  local:
    token_ttl: 1h
    refresh_token_ttl: 720h
```

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: Offline access MUST remain explicit; refresh tokens MUST only be issued when the authorization request clearly asks for offline access.
- **SR-002**: Refresh tokens MUST be treated as single-use credentials after successful redemption.
- **SR-003**: The broker MUST fail closed when refresh-token validation, lookup, or rotation cannot be completed.
- **SR-004**: Replay or reuse of a refresh token MUST be auditable through the broker’s normal security-relevant event logging.
- **SR-005**: The broker MUST preserve client binding so that a refresh token can only be redeemed by the client to which it was originally issued.

### Key Entities *(include if feature involves data)*

- **Refresh Token Session**: A persisted representation of an issued refresh token, including the subject, client, granted scopes, expiry, and whether the token has already been used or revoked.
- **Client Capability Declaration**: The set of grant types and related capabilities a client presents, used to determine whether refresh tokens may be issued.
- **Authorization Server Metadata**: The broker-published capability document that tells integrators which grant types and scopes are supported in the current deployment mode.


### Assumptions

- The broker continues to operate in three authorization-server modes: proxy, local, and hybrid.
- Clients needing background or delayed access are expected to request `offline_access` during the authorization flow.
- Public clients may derive their declared grant-type capabilities from a metadata document rather than from pre-provisioned credentials.
- Proxy mode may forward upstream refresh-token behavior without claiming that the upstream supports offline access unless an operator explicitly chooses to advertise it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: In locally issuing modes, 100% of successful authorization-code exchanges that request `offline_access` for eligible clients return a refresh token.
- **SC-002**: 100% of successful authorization-code exchanges that do not request `offline_access`, or that involve clients without refresh-token capability, return no refresh token.
- **SC-003**: 100% of attempts to reuse a refresh token after a successful redemption are rejected as invalid grants.
- **SC-004**: Integrators can determine offline-access and refresh-token capability for a deployment from a single metadata fetch, with local and hybrid deployments advertising both capabilities by default and proxy deployments not advertising offline access by default.
