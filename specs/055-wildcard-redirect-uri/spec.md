# Feature Specification: Wildcard Redirect URI Hosts

**Feature Branch**: `feat/wildcard-redirect`
**Created**: 2026-10-29
**Status**: Draft
**Input**: Support wildcard hostnames in registered HTTPS redirect URIs for preview callbacks, without allowing a wildcard to replace the registrable domain or public suffix.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Authorize preview callbacks (Priority: P1)

An operator registers one callback pattern for a test Agent. Preview deployments use separate hosts, and an authenticated client can return to each preview through the registered callback path.

**Why this priority**: Without this capability, every preview host needs a separate callback registration, blocking validation of clients in per-PR previews.

**Independent Test**: Register the callback pattern on a test Agent and complete authorization from two distinct preview hosts. Both flows must return to their requested callback with authorization results.

**Acceptance Scenarios**:

1. **Given** a test Agent has `https://preview*.app-*.example.com/api/oidc/callback` registered, **When** an authorized request uses `https://preview-pr-123.app-test.example.com/api/oidc/callback`, **Then** the broker redirects to that exact callback with the authorization result and state.
2. **Given** the same registration, **When** a second authorized request uses `https://preview-pr-456.app-test.example.com/api/oidc/callback`, **Then** the broker redirects to that second exact callback with its authorization result and state.
3. **Given** the same registration, **When** an authorized request uses `https://preview.app-test.example.com/api/oidc/callback`, **Then** the broker accepts the stable test host and redirects to that exact callback.
4. **Given** the same registration, **When** an authorized request uses `https://preview.app-sandbox.example.com/api/oidc/callback`, **Then** the broker accepts the sandbox host and redirects to that exact callback.
5. **Given** the same registration, **When** an authorized request uses `https://preview.app-staging.example.com/api/oidc/callback`, **Then** the broker accepts the staging host and redirects to that exact callback.

### User Story 2 - Reject callbacks outside the registered trust boundary (Priority: P1)

An operator can rely on a wildcard registration to accept only HTTPS callbacks in the intended host namespace and at the registered callback path. Existing exact callbacks and authentication controls continue to work.

**Why this priority**: A wildcard callback must not let an attacker redirect authorization results to an unrelated host or weaken existing callback protections.

**Independent Test**: Submit authorization requests with a host outside the pattern, a changed scheme, or a changed path; verify each is rejected without a redirect. Then verify an existing exact callback still succeeds.

**Acceptance Scenarios**:

1. **Given** the test Agent has the preview pattern, **When** an authorization request uses `https://preview-pr-123.app-test.example.net/api/oidc/callback`, **Then** the broker returns `invalid_redirect_uri` and does not redirect.
2. **Given** the same registration, **When** a request changes the callback path or uses HTTP, **Then** the broker returns `invalid_redirect_uri` and does not redirect.
3. **Given** a registration whose wildcard would replace the registrable domain or public suffix, or the entire hostname, **When** an operator attempts to register it, **Then** the registration is rejected.
4. **Given** an existing exact registered callback, **When** it is used for authorization, **Then** the broker continues to accept it under the existing rules.

### Edge Cases

- A wildcard matches characters inside one hostname label only; it never matches a dot or crosses into another label.
- A wildcard may appear in more than one subdomain label, as in `preview*.app-*.example.com`. The expected preview host shape is `preview-pr-<number>.app-test.example.com`.
- The wildcard portion of a complete hostname label must produce a valid, non-empty DNS label. A partial wildcard may match zero characters when the remaining literal characters form a valid label.
- The scheme, port, path, query, and other URI components remain fixed by the registered callback. For the preview callback, the scheme is HTTPS and the path is `/api/oidc/callback`. Wildcards in other URI components are rejected.
- The registrable domain and public suffix must remain literal. Patterns such as `https://preview*.app-*.example.*/api/oidc/callback` and `https://preview*.app-*.*.com/api/oidc/callback` are invalid.
- Private-use scheme callbacks remain exact-match only and retain their existing allowlisting rules.
- Existing exact-match callbacks, loopback HTTP port behavior, and error responses remain unchanged.
- Redirect URIs supplied by a CIMD document remain exact-match-only in this feature.

## Concrete Example Journeys *(mandatory)*

### Journey 1 - Complete authorization from preview deployments (User Story 1, Scenarios 1-5)

**Cast**: Alice (`alice@example.test`), test Agent `550e8400-e29b-41d4-a716-446655440000`, and preview hosts `preview-pr-123.app-test.example.com`, `preview-pr-456.app-test.example.com`, `preview.app-test.example.com`, `preview.app-sandbox.example.com`, and `preview.app-staging.example.com`. Alice already has an active grant for the Agent.

**Step 1 - Reconcile test-Agent configuration**: The test deployment applies the HTTPS callback pattern to existing and newly created test Agents. The configuration is reconciled before preview validation. The callback path is `/api/oidc/callback`.

**Step 2 - Authorize from preview 123**: Alice's preview starts an authorization request with its concrete callback URI.

```http
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&response_type=code&redirect_uri=https%3A%2F%2Fpreview-pr-123.app-test.example.com%2Fapi%2Foidc%2Fcallback&state=pr-123 HTTP/1.1
Host: broker.example.test
```

**Step 3 - Authorize from preview 456, stable test, sandbox, and staging**: The other deployments make equivalent requests with their own concrete `redirect_uri` and `state` values. Each callback keeps the same HTTPS scheme and `/api/oidc/callback` path.

**Observable result**: Both pull-request hosts, the stable test host, the sandbox host, and the staging host complete authorization. Each response targets the exact concrete URI from that request and preserves its corresponding state. The wildcard pattern is never used as the redirect destination.

### Journey 2 - Reject a callback outside the registration (User Story 2, Scenarios 1-3)

**Cast**: Alice and the same test Agent. The test Agent has the preview callback pattern and Alice has an active grant.

**Step 1 - Request an unregistered callback**: A client requests authorization using an unrelated hostname or changes the registered scheme or path.

```http
GET /oauth2/authorize?client_id=550e8400-e29b-41d4-a716-446655440000&response_type=code&redirect_uri=https%3A%2F%2Fpreview-pr-123.app-test.example.net%2Fapi%2Foidc%2Fcallback&state=outside-scope HTTP/1.1
Host: broker.example.test
```

**Step 2 - Reject the request**: The broker does not redirect to the supplied callback.

**Observable result**: The broker returns the existing `invalid_redirect_uri` error response. No authorization code or error response is sent to the unregistered callback.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The system MUST accept wildcard callback patterns in an Agent's registered HTTPS `redirect_uris` values.
- **FR-002**: The system MUST support the preview pattern `https://preview*.app-*.example.com/api/oidc/callback` for pull-request previews, the stable test host, and sandbox and staging hosts represented by that pattern.
- **FR-003**: A `*` MUST match characters only within one hostname label and MUST NOT match a dot or span multiple labels. Multiple subdomain labels may contain wildcards.
- **FR-004**: The system MUST require the registrable domain and public suffix to be literal in every wildcard pattern. It MUST reject a wildcard that replaces either portion or the entire hostname.
- **FR-005**: The scheme, port, path, query, and other non-host URI components MUST match the registered value under the existing redirect URI rules. The system MUST reject wildcards outside the hostname.
- **FR-006**: When a wildcard pattern matches, the system MUST redirect only to the concrete `redirect_uri` supplied in the authorization request, never to the registered pattern.
- **FR-007**: A callback that does not match a registered exact URI or valid wildcard pattern MUST be rejected with the existing redirect URI error behavior and MUST NOT receive an authorization code or redirect.
- **FR-008**: Existing exact callback registrations MUST continue to work without requiring changes.
- **FR-009**: Private-use scheme callbacks MUST remain exact-match only and retain their existing allowlisting requirements.
- **FR-010**: CIMD document `redirect_uris` MUST retain exact-match behavior in this feature.
- **FR-011**: The feature MUST preserve existing shared callbacks and agent-to-agent callbacks, production callback configuration, and other authentication controls.

### Domain Model

- **Redirect URI pattern**: An Agent-configured HTTPS callback URI in which `*` matches characters inside one or more subdomain labels. The registrable domain, public suffix, and non-host URI components remain literal.

### API Requirements

- **API-001**: The existing Agent `redirect_uris` contract MUST document the supported wildcard syntax, the fixed-component matching rules, the registrable-domain restriction, and invalid-pattern behavior.
- **API-002**: The feature MUST NOT add a new endpoint or request field.

### Security Requirements

- **SR-001**: The system MUST fail closed when a wildcard pattern is malformed, cannot be safely matched, or does not preserve a literal registrable domain and public suffix.
- **SR-002**: A wildcard MUST NOT match across hostname labels or alter the scheme, port, path, query, or fragment of a callback.
- **SR-003**: The system MUST validate the concrete requested callback under the existing redirect URI security rules before using it as a redirect destination.
- **SR-004**: The system MUST preserve existing authentication, consent, state, and authorization-code protections.

### Key Entities

- **Agent**: The configured identity whose registered callback URIs determine which authorization return locations are allowed.
- **Redirect URI pattern**: A registration rule that matches a bounded set of concrete HTTPS callback URIs.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: After test-Agent configuration is reconciled, authorization requests from two distinct pull-request previews complete and return to each concrete callback with its own state.
- **SC-002**: Pull-request preview hosts, the stable test host, and sandbox and staging hosts matching the example pattern are accepted with the exact `/api/oidc/callback` path.
- **SC-003**: Every tested callback outside the registered host, scheme, or path scope is rejected without redirecting to that callback.
- **SC-004**: Wildcards that replace the registrable domain, public suffix, or entire hostname are rejected at registration.
- **SC-005**: Existing exact callback and private-use callback scenarios retain their current results.
- **SC-006**: No production callback configuration or existing authentication control changes as part of this feature.

## Assumptions

- This feature applies to Agent-configured `redirect_uris`; it does not extend wildcard behavior to CIMD document registrations.
- The pattern is an explicit operator registration for a test Agent. It does not permit arbitrary client-provided patterns.
- `*` is the only wildcard character; regular-expression syntax and character classes are not supported.
- The preview deployment provides pull-request identifiers in the `preview-pr-<number>` label; wildcard matching does not impose a numeric-only constraint on the characters matched by `*`.
- The required security outcome is that the registrable domain and public suffix are literal. Selecting the mechanism used to determine those domain boundaries is an implementation-planning decision.
- Deployment-specific configuration of callback patterns is outside this specification. The test deployment must apply the registration to both existing and newly created test Agents and reconcile that configuration before end-to-end preview validation.

## References

- [Existing wildcard CIMD client URI specification](../028c-cimd-wildcard-client-uris/spec.md)
