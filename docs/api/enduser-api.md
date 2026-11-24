# End-User API

The end-user API serves OAuth2 authorization, token, and metadata requests. The broker serves it on the end-user listener.

When token exchange is configured, it also serves the AuthZEN delegation PDP. See [AuthZEN Delegation Evaluation](authzen-evaluation.md).

## CIMD Client Authentication

In local or hybrid mode with CIMD enabled, a pre-registered CIMD client can authenticate at the token endpoint with `private_key_jwt`. The client signs a short-lived assertion with a key from its metadata document. The broker requires a fresh assertion for each authorization-code exchange and refresh request.

The assertion uses `RS256` or `ES256`, and its algorithm must match the client's metadata document. It must include `kid`, `jti`, `iss`, `sub`, `aud`, and `exp`. Both `iss` and `sub` must equal the CIMD URL. The `jti` is single-use per client.

For an authorization-code exchange, send the assertion fields with the existing grant fields:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=https%3A%2F%2Fchatgpt.com%2Foauth%2FEBkIgu5McFbd%2Fclient.json
&code=AUTHORIZATION_CODE
&redirect_uri=https%3A%2F%2Fchatgpt.com%2Fconnector%2Foauth%2FEBkIgu5McFbd
&code_verifier=PKCE_VERIFIER
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=SIGNED_CLIENT_ASSERTION
```

For refresh, use `grant_type=refresh_token`, `refresh_token`, `client_id`, `client_assertion_type`, and a new `client_assertion`. Do not send `client_secret` or an `Authorization` header with an assertion.

Malformed parameter combinations return `400 invalid_request`. Authentication failures return `401 invalid_client` with a stable description. A document or key-set dependency failure returns `server_error`. A failed request does not consume an authorization code or rotate a refresh token.

See the [token endpoint contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/051-private-key-jwt-auth/contracts/token-endpoint.md) for the complete assertion profile and response rules.

## Agent-Subject Token Exchange

In local mode, a privileged client can request a token for a registered agent acting for itself. The [canonical OpenAPI](../../api/enduser/openapi.yaml) defines the contract. Use the complete [agent-subject configuration](../../examples/config/impersonation-agent-subject.yaml) and [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance).

Classification is rule-local: the raw string `sub` must exactly equal the audience-selected target's canonical UUID, and the subject must be signed and verified or, for an unsigned (`verification: none`) subject, the rule's predicate must bind `subject_token.sub`. Everything else is human. There is no request category field. Aliases and alternate UUID spellings never select agent issuance, and a different registered agent's UUID is rejected without falling back to human delegation. The target must exist and existing scope limits still apply. Every credential validation, principal extraction and CEL authorization must succeed. The actor and client assertion always require signatures.

| `allowed_subject_categories` | Human | Signed exact-target subject |
| --- | --- | --- |
| Omitted or `[human]` | Active delegation required | Ineligible, even with same-text human delegation |
| `[agent]` | Ineligible | No human delegation required |
| `[human, agent]` (either order) | Active delegation required | No human delegation required |

Category-ineligible rules fall through. Missing/expired delegation for an authorized human is terminal, with the established consent `error_uri`; no automatic agent retry is allowed. Unsigned-subject rules may list `agent`, but unsigned subjects remain human and that entry has no effect.

### Request and signed response

This example uses registered target `research-agent` with UUID `550e8400-e29b-41d4-a716-446655440000`. Its rule explicitly allows `agent` and authorizes `gateway-prod`, `research-runner` and the target UUID in CEL. The three markers below represent externally signed ES256 credentials with issuer `https://idp.example.com`, audience `https://broker.example.com/impersonation` and valid lifetimes. Their subjects are respectively the gateway, runner and target UUID. Line breaks in the form body are for display only.

```http
POST /oauth2/token HTTP/1.1
Host: broker.example.com
Content-Type: application/x-www-form-urlencoded
Accept: application/json

grant_type=urn:ietf:params:oauth:grant-type:token-exchange
&audience=https%3A%2F%2Fbroker.example.com%2Fimpersonation%2Fresearch-agent
&client_assertion_type=urn:ietf:params:oauth:client-assertion-type:jwt-bearer
&client_assertion=SYNTHETIC_CLIENT_JWS
&actor_token_type=urn:ietf:params:oauth:token-type:jwt
&actor_token=SYNTHETIC_ACTOR_JWS
&subject_token_type=urn:ietf:params:oauth:token-type:jwt
&subject_token=SYNTHETIC_AGENT_JWS

HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: no-store
Pragma: no-cache

{"access_token":"SYNTHETIC_BROKER_AGENT_JWS","token_type":"Bearer","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","expires_in":3600}
```

Decoded access-token claims (authorization-relevant excerpt; normal `jti` may also be present):

```json
{
  "iss": "https://broker.example.com",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "aud": "https://policy.example.com",
  "scope": "",
  "iat": 1793005200,
  "exp": 1793008800,
  "act": {"iss": "https://idp.example.com", "sub": "research-runner"}
}
```

The broker creates no human grant/session and initiates no consent interaction. It retains `act`, even if actor and subject identities happen to be equal. Reserved `sub` and `agent_id` cannot be overridden by local policy. No category claim is added. Subject email extraction is skipped and no credential-derived email is supplied; output email remains policy-owned. Issuer, signing, lifetime, scope and audience policy keep their normal semantics.

### Denials

If no rule accepts the category (including signed exact-target equality on a human-only rule with or without a grant), the response is the generic existing envelope, without consent recovery:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"impersonation not permitted"}
```

For an otherwise valid third-party exchange using the issued token as `subject_token`, resource `https://api.thirdparty.example.com`, and a fresh valid third-party client assertion, default detection rejects before agent extraction, authorization or human grant/session/provider access. A matching compliant override has the same result:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"access_denied","error_description":"agent-subject tokens cannot be exchanged for third-party service tokens"}
```

There is no `error_uri`: human consent cannot recover this denial. Default detection is the guarded equality of `sub` and `agent_id`; it applies regardless of issuer or issuance date. Runtime detection errors, timeouts and dynamic non-boolean output instead return `500 server_error`, also without `error_uri` or human-state access. A configured detector fully replaces the default. Operators must retain broker agent-token matching; a non-matching override that references `sub` (a detector without `subject_token.sub`, such as `false`, fails startup) removes categorical rejection and same-text human-credential isolation. Existing checks may then consult human grants/sessions or return consent errors; this is not support for third-party agent identity or guaranteed exchange success.

See the [exchange concepts](../concepts/token-exchange.md#agent-subject-boundary), [diagnostic reference](../reference/token-exchange.md#agent-subject-boundary), and [detector example](../../examples/config/token-exchange.yaml).

### Approval endpoints

The same detector guards the approval endpoints that authenticate a subject token (ADR 045 decision 6a). An agent-subject token presented to `POST /api/approvals` or `POST /api/approvals/{id}/consume` is rejected before any approval is read or written:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json

{"error":"forbidden","message":"agent-subject tokens cannot authenticate approval requests"}
```

A detection failure returns `503 service_unavailable`. `GET /api/approvals` authenticates only the client assertion and is unaffected.

The [configuration reference](../configuration.md#subject-categories-and-self-agent-issuance), [concepts](../concepts/token-exchange.md#agent-subject-boundary), [field reference](../reference/token-exchange.md#agent-subject-boundary) and [example configurations](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/examples/config/README.md#impersonation-agent-subjectyaml--signed-self-agent-issuance) describe the same contract.
## Incremental Service Authorization

API version 1.4.0 adds Virtual Service Scopes, `<public URL>/sessions/<canonical-id-or-UUID>`. The [incremental authorization guide](../guides/incremental-authorization.md) describes the flows, and [`api/enduser/openapi.yaml`](../../api/enduser/openapi.yaml) is the authoritative contract.

- `GET /oauth2/authorize` accepts virtual scopes. They are exempt from the agent's `allowed_scopes` and make each named service required for that request. Malformed, unknown or ineligible service scopes redirect with `invalid_scope`. An unsatisfied service redirects to consent; unverifiable usability of a selected service redirects with `temporarily_unavailable`. A service the grant does not select redirects to consent even when its usability is unverifiable. After Save, the server-built continuation carries the same `session_token`, which the endpoint revalidates before issuing a code or redirecting upstream. Empty or repeated `session_token` values, or forged, malformed, expired or request/principal/agent-mismatched context, return `302 access_denied` with `authorization session is invalid or expired`; a valid JWE with reassigned binding returns `302 invalid_scope` with `requested service scope is not permitted`. Redirect validation runs first; errors use only the registered callback and request state (omitted if absent), never forwarding JWE or PKCE material.
- Consent agent-detail (`GET /api/consent/agents/{agent-id}`) and grant submission (`POST /api/consent/agents/{agent-id}/grants`) reject empty or repeated `session_token` query values with `400 invalid_token`, before grant mutation. Only an absent parameter denotes standalone consent management; malformed, expired and mismatched tokens retain their existing errors.
- `GET /api/consent/agents/{agent-id}` always returns `requested_service_ids` and `requested_service_coverage`. Coverage lists the service's required scopes, baseline usability and every covering permission set with its supplied required scopes and usability (`usable`, `missing`, `unusable` or `unavailable`).
- `POST /api/consent/agents/{agent-id}/grants` returns `400 requested service not satisfied` without writing the grant when a requested service is not selected, lacks required-scope coverage or has no usable session.
- `POST /oauth2/token`: recoverable token-exchange errors include `scope`. Local responses and JWT `scope` claims include granted virtual scopes. Refresh never widens: adding a scope returns `invalid_scope`, a bound service that is no longer satisfied returns `invalid_scope`, and unverifiable usability of a selected service returns `server_error`. A bound service the grant no longer selects returns `invalid_scope` even when its usability is unverifiable. None of these rotate the refresh token. Proxy refresh validates broker-prefixed syntax, current service existence and agent eligibility before forwarding: `400 invalid_scope` (`requested service scope is not permitted`) or lookup-failure `500 server_error` makes no upstream call. Valid virtual scopes are removed; ordinary-only requests add no lookup, and upstream response/rotation remain unchanged. No session-usability or grant-coverage check runs.
- `POST /oauth2/token` impersonation (local mode): virtual scopes are exempt from the target's `allowed_scopes`. After delegation, a human subject's virtual scope must name an existing, eligible service covered by the subject's grant with a usable session; otherwise `400 invalid_scope`, or `500 server_error` when lookup or usability cannot be verified. An agent subject with a virtual scope returns `400 invalid_scope` (`service scopes require a user authorization`).
- `GET /api/third-party/sessions/{service}` returns the service, the caller's session (or `null`) and the caller's dependent agents for a canonical ID or UUID, and `404` for an unknown service.

## Service Icons

The approved icon contract extends existing service, consent, and session JSON responses.
The [Admin OpenAPI](../../api/admin/openapi.yaml) and [end-user OpenAPI](../../api/enduser/openapi.yaml) are canonical.
No new runtime configuration, image deployment, or image endpoint is required.

### Fields and access boundaries

| API representation | Optional field | Source |
| --- | --- | --- |
| Admin service create/update input | `icon_svg` | Operator-supplied SVG document |
| Admin service create/update/detail/list response | `icon_svg` | Exact accepted source |
| Consent `data.services[]` (`ServiceRequirementForUser`) | `iconSvg` | Current service source |
| Session summary (`UserSessionSummary`) | `service_icon_svg` | Current service source, not a session snapshot |
| Session details (`SessionStatus`) | Inherited `service_icon_svg` | Same current service projection |

Admin requests retain the existing trusted-proxy boundary.
The proxy must authenticate the caller and enforce administrator privilege before it forwards a request.
The broker does not identify administrators. The icon feature adds no broker-side Admin authentication requirement.
The examples use the configured proxy principal header, `X-Remote-User`.

Consent and session routes retain principal authentication, ownership checks, and existing error envelopes.
Unauthenticated requests return the route's existing `401` response without SVG bytes.
Existing JSON caching rules remain unchanged. The feature adds no public or shared cache.

Consent icons follow the existing filtering of `data.services`.
Permission-set service rows reuse that metadata, rather than additional SVG copies on scope records.
The feature adds no service catalog. Service icons do not use a remote URL fallback.

### Create, replace, preserve, and clear

| `icon_svg` input | Create | Otherwise valid full-service PUT |
| --- | --- | --- |
| Omitted | No icon | Preserve the current icon |
| `null` | No icon | Clear the current icon |
| Permitted string | Store exact source | Replace with exact source |

An empty or whitespace-only string is invalid, not a clear operation.
A number, object, array, URL, path, or data URI is also invalid.
The server omits absent source from responses. It returns configured source exactly after JSON decoding, without normalization or sanitization.

This example creates Atlas Files with original SVG artwork:

```http
POST /api/services HTTP/1.1
Host: admin.broker.example.com
X-Remote-User: olivia@example.com
Content-Type: application/json

{
  "display_name": "Atlas Files",
  "client_id": "atlas-broker-public",
  "token_endpoint_auth_method": "none",
  "issuer_uri": "https://atlas.example.com",
  "discovery": {"enable_discovery": false},
  "endpoints": {
    "authorize_endpoint": "https://atlas.example.com/authorize",
    "token_endpoint": "https://atlas.example.com/token"
  },
  "scopes": [{"scope_value": "files.read", "description": "Read files"}],
  "icon_svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M4 4h16v16H4z\"/></svg>"
}
```

Creation returns `201 Created` and the direct service object, without a `data` envelope.
`GET /api/services/{service-id}` returns the same `icon_svg` value.
`GET /api/services` returns a direct array whose configured service entries include that value.
The following response excerpt omits unrelated service properties:

```json
{
  "id": "880e8400-e29b-41d4-a716-446655440010",
  "display_name": "Atlas Files",
  "icon_svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><path fill=\"currentColor\" d=\"M4 4h16v16H4z\"/></svg>"
}
```

An icon update still uses a complete, valid full-service replacement, not a partial PATCH.
Send `PUT /api/services/880e8400-e29b-41d4-a716-446655440010` with the headers and body fields from the POST example.
Replace `icon_svg` with the following value:

```xml
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle fill="currentColor" cx="12" cy="12" r="8"/></svg>
```

A successful update returns `200 OK`, the current strong service `ETag`, and the exact replacement source.
For preservation, send the same full body without `icon_svg`.
For clearing, send the same full body with `"icon_svg": null`.
The clear response omits `icon_svg`.

Existing service row-version checks still apply. A concurrent modification returns `409 Conflict` without partial changes.
Changing only the icon does not require `If-Match` when `protected_resources` is omitted or null.
An explicit `protected_resources` array still replaces that set and requires the current strong service ETag in `If-Match`.
That operation retains `428` for a missing precondition and `412` for a stale precondition.
Existing not-found and authorization behavior remains unchanged.

### Validation and image-only rendering

Non-null source must be valid UTF-8 and contain 1–65,536 bytes after JSON decoding.
The limit measures bytes, not Unicode characters or JSON escape length.
OpenAPI character-length keywords cannot enforce this byte limit or the XML rules.

The upload checks require these properties:

- One root `svg` in `http://www.w3.org/2000/svg`, properly nested elements, and no trailing non-whitespace content
- Strict, well-formed XML without duplicate attributes or unknown entity references
- At most one pre-root SVG DOCTYPE, optionally with an external PUBLIC or SYSTEM identifier, without an internal subset
- No entity declarations, other XML directives, or processing instructions except one initial UTF-8 XML declaration
- No `script`, `a`, or `foreignObject` element local names, ASCII case-insensitively in every namespace
- No attribute local names that start with `on`, ASCII case-insensitively, including namespaced forms
- No decoded attribute value with a leading `javascript:` or `vbscript:` scheme after ASCII control/space removal and ASCII-insensitive comparison

Ordinary comments, harmless metadata, namespace aliases, and visual features remain permitted.
Styles, text, effects, local reuse, embedded media, and declarative visual animation do not require manual simplification.
Missing references, duplicate IDs, unusual geometry, and browser-ignored values are rendering concerns, not automatic upload errors.
These named checks do not certify all CSS, references, animated values, or arbitrary document safety.
The byte limit is not a browser CPU, memory, filter-cost, or animation-lifetime budget.

Invalid input returns the existing Admin `400` validation envelope without echoing the document, for example:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed","message":"icon_svg contains prohibited content"}
```

Malformed and oversized documents use the same validation envelope.
Validation fails before discovery, credential changes, or persistence. No service properties change, even if the request also changes credentials or metadata.
The broker never resolves a DTD, installs an external resolver or custom entity table, or fetches icon references.

**Render source only as an image.** URI-encode the exact source into `data:image/svg+xml;charset=utf-8,...` and use an `<img>` with empty alt text.
Do not insert SVG markup into the page DOM or use object, embed, iframe, or document delivery.
External references can remain in accepted source. Browser SVG image mode prevents their loads.
Authenticated JSON controls receipt of bytes, not arbitrary reuse by an authorized client who copies them.

### Consent and session integration

`GET /api/consent/agents/{agent-id}` returns the configured source beside the existing service ID and name.
This response excerpt omits unrelated agent properties:

```json
{
  "data": {
    "services": [{
      "serviceId": "880e8400-e29b-41d4-a716-446655440010",
      "serviceName": "Atlas Files",
      "iconSvg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><circle fill=\"currentColor\" cx=\"12\" cy=\"12\" r=\"8\"/></svg>",
      "requirementType": "mandatory",
      "requiredScopes": [{"name": "files.read", "description": "Read files"}],
      "connectionStatus": "connected"
    }]
  }
}
```

Session list, detail, and refresh responses use the same current service metadata:

| Operation | Field location |
| --- | --- |
| `GET /api/third-party/sessions` | `data.sessions[].service_icon_svg` |
| `GET /api/third-party/{serviceId}/session` | `data.session.service_icon_svg`, inherited through `SessionStatus` |
| `POST /api/third-party/{serviceId}/session/refresh` | `data.service_icon_svg` in the returned summary |

This session list excerpt shows the source after the replacement:

```json
{
  "data": {
    "sessions": [{
      "id": "990e8400-e29b-41d4-a716-446655440020",
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
      "service_display_name": "Atlas Files",
      "service_icon_svg": "<svg xmlns=\"http://www.w3.org/2000/svg\" viewBox=\"0 0 24 24\"><circle fill=\"currentColor\" cx=\"12\" cy=\"12\" r=\"8\"/></svg>"
    }]
  }
}
```

After clearing, fresh responses omit `iconSvg` and `service_icon_svg` from those same objects.
Clients accept omission or null and replace their previous icon metadata, rather than merge stale source back into the view.
Existing refresh and cache lifetimes remain unchanged. Fresh source replaces the prior image, and changed source retries a failed image without a full page reload.
Absent or broken images retain each view's original fallback, visible service name, and working consent, connection, refresh, and termination controls.
The icon adds no keyboard stop or duplicate service announcement.
