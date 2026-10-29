# Contract Delta: End-User API (`api/enduser/openapi.yaml`)

**Status**: Proposed. Nothing in this file is approved. Each section names its approval item in
[plan.md](../plan.md#approval-items). The implementation phase copies the approved sections into
`api/enduser/openapi.yaml` before it writes code (Principles IV and X). It also renders them in
`docs/api/enduser-api.md` and `docs/reference/token-exchange.md`.

**Version**: `info.version` changes from `1.1.0` to `1.2.0`. All changes are additive, or they
change the target of the existing optional `error_uri` action link within its documented recovery
purpose (ARCHITECTURE.md › API versioning; research R13).

Synthetic values follow the spec's shared cast. `public_url` is `https://broker.example.com`.

---

## 1. `POST /oauth2/token`: RFC 8693 error `scope` member (API-EXCHANGE-CHALLENGE)

`TokenExchangeError` gains an optional property:

```yaml
scope:
  type: string
  format: uri
  description: |
    Virtual Service Scope of the resolved third-party service, in its current canonical form
    (UUID form when the service has no canonical ID). Present only on errors that also carry
    `error_uri` and occur after the requested resource resolved to a service: missing, expired,
    or empty grant; service not covered by the grant; missing or unusable provider session;
    insufficient session scopes. Gateways copy it into their RFC 6750 `insufficient_scope`
    challenge. Never present on non-recoverable errors.
  example: "https://broker.example.com/sessions/github"
```

| Failure (diagnostic) | Status | `error` | `error_uri` | `scope` |
|---|---|---|---|---|
| Session missing, unusable, or refresh rejected (`provider_session`) | 400 | `invalid_grant` | `/api/third-party/<uuid>/oauth2/authorize?redirect_uri=<public_url>/sessions/<identifier>` (§9) | yes |
| Session scopes insufficient | 400 | `invalid_grant` | same as above | yes |
| Grant missing, expired, or empty; service not covered | 403 | `access_denied` | `<public_url>/agents/<agent-id>` (unchanged) | yes |
| Resource unknown, client or subject invalid, policy denied, server error | unchanged | unchanged | absent | **absent** |

Example: Journey 1, Step 2 of the spec.

```json
{"error":"invalid_grant","error_description":"User session is unavailable. Please re-authenticate.","error_uri":"https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub","scope":"https://broker.example.com/sessions/github"}
```

The broker still sends no `WWW-Authenticate` header on `/oauth2/token` (API-003).

## 2. `GET /oauth2/authorize`: virtual scopes (API-CONSENT)

The `scope` parameter description gains this text:

> Space-delimited scopes can include Virtual Service Scopes (see
> `contracts/virtual-service-scope.md`). A virtual scope makes the named service required for this
> request only. Virtual scopes are exempt from the agent's `allowed_scopes` list. They are never
> forwarded upstream.

Outcomes once `redirect_uri` is validated. All are redirects to the client's `redirect_uri` with
`error` and `state` unless noted otherwise:

| Condition | Result |
|---|---|
| Malformed virtual scope, unknown service, or service not eligible for the agent | `error=invalid_scope`, `error_description=requested service scope is not permitted` |
| Any requested service uncovered by the active grant, lacking required-scope coverage, missing, or definitively unusable | `302` to `<public_url>/agents/<agent-id>?session_token=<JWE>` (existing consent redirect). The JWE also seals `requested_service_ids` and bindings. |
| A requested service's usability cannot be verified (transient provider or storage failure) | `error=temporarily_unavailable` |
| All requested services satisfied, local or CIMD agent | Existing code issuance. The code records the granted virtual scopes and their bindings. |
| All requested services satisfied, proxy agent | Existing upstream `302`. The upstream `scope` has every virtual scope removed and is omitted when empty. |
| Continuation `session_token` is valid but a virtual scope no longer resolves to its sealed UUID | `error=invalid_scope`; no code or upstream redirect. |

**Consent continuation parameter (Proposed, API-CONSENT)**: `session_token` is an optional opaque
JWE query parameter on `/oauth2/authorize`, used only for the server-built post-Save continuation.
The server authenticates expiry, principal and agent again, compares the request (excluding only
`session_token`) against the sealed `original_url`, and uses the sealed request and bindings.
Invalid, expired or request/user/agent-mismatched context is rejected by existing invalid-session
handling; it never falls back to fresh unbound resolution. Re-resolve every scope and compare UUIDs
before code issuance or upstream redirect, even when grant submission just succeeded. Re-entering
consent preserves the original bindings/expiry. Consume this parameter locally; never forward it
upstream or to the client callback. Initial requests without it retain initial resolution behavior.

## 3. `POST /oauth2/token`: local `authorization_code` and `refresh_token` (API-LOCAL-SCOPES)

`OAuth2TokenResponse.scope` and the local access-token JWT `scope` claim both equal the granted
scope set. That set includes granted virtual scopes in their requested form. On refresh it is the
requested subset of the predecessor's current `Scope`, or that current `Scope` when omitted.
Successful narrowing persists the same narrowed successor `Scope`; all original service bindings
remain inherited as an assurance-only set, not permission to re-grant removed scopes. Every refresh
checks all bindings against current grant-derived effective provider scopes, even bindings whose
scope was dropped. Later scope-less refresh cannot restore removed scopes.

Three-step example: (1) code exchange returns `openid offline_access
https://broker.example.com/sessions/github`; (2) refresh with `scope=openid offline_access`
returns `200` with `scope: "openid offline_access"`, a JWT with that same scope and a successor
refresh token whose `Scope` is narrowed but whose GitHub binding is retained; (3) scope-less refresh
of that successor returns only `openid offline_access` after rechecking GitHub. If GitHub is now
unusable, step 3 returns non-consuming `invalid_scope`. Explicitly requesting the removed GitHub
scope is widening and also returns non-consuming `invalid_scope`.

Spec Journey 1, Step 8:

```json
{"access_token":"SYNTHETIC_LOCAL_JWT","token_type":"Bearer","expires_in":3600,"refresh_token":"SYNTHETIC_REFRESH_2","scope":"openid offline_access https://broker.example.com/sessions/github"}
```

Decoded payload (synthetic): `{"iss":"https://broker.example.com","sub":"alice@example.com","agent_id":"550e8400-e29b-41d4-a716-446655440000","scope":"openid offline_access https://broker.example.com/sessions/github","iat":1793093447,"exp":1793097047,"jti":"b2c3d4e5-f6a7-4b8c-9d0e-1f2a3b4c5d6e"}`.

New `refresh_token` outcomes (local issuance). In every case below, **the refresh token is neither
rotated nor revoked**:

| Condition | Status | Body |
|---|---|---|
| Request `scope` contains a virtual scope absent from the predecessor's current `Scope`, including a previously removed scope | 400 | `{"error":"invalid_scope","error_description":"scope <scope> requires interactive authorization"}` |
| An originally bound virtual scope no longer resolves, resolves to another service, is no longer eligible, is uncovered, or is definitively unusable for the grant-derived effective provider scopes. This holds whether request `scope` is omitted or narrowed, including assurance-only bindings retained after narrowing. | 400 | `{"error":"invalid_scope","error_description":"scope <scope> is no longer satisfied; interactive authorization required"}` |
| Usability of a granted virtual scope cannot be verified | 500 | `{"error":"server_error"}` |

Existing revoked-grant handling (`invalid_grant`) and replay and rotation semantics are unchanged.

`client_credentials` with any virtual scope returns `400 invalid_scope`, with the description
"service scopes require a user authorization".

## 4. `POST /oauth2/token`: proxy `refresh_token` (DEC-002)

The documentation states that the broker removes virtual scopes from the forwarded `scope` and
omits `scope` when it becomes empty. The upstream response is returned unchanged, including a
rotated `refresh_token`. A successful proxy refresh does not attest that any broker-managed service
is usable.

## 5. `GET /api/consent/agents/{agent-id}`: requested services (API-CONSENT)

`GetAgentDetailResponse.data` gains two properties:

```yaml
requested_service_ids:
  type: array
  items: {type: string, format: uuid}
  description: |
    Services named by Virtual Service Scopes in the authorization request sealed in
    `session_token`. Derived by the server; empty without `session_token`.
requested_service_coverage:
  type: array
  items:
    type: object
    required: [service_id, required_scopes, usability, covering_permission_sets]
    properties:
      service_id: {type: string, format: uuid}
      required_scopes:
        type: array
        items: {type: string}
        description: The agent's required provider scopes for this service.
      usability:
        type: string
        enum: [usable, missing, unusable, unavailable]
        description: |
          Baseline session usability for configured required scopes (empty baseline for
          require_all_scopes or agents without explicit requirements). Not proof of a live
          selection; selected candidate contributions must also be usable.
          unavailable means could not be verified now; retry.
      covering_permission_sets:
        type: array
        items:
          type: object
          required: [permission_set_id, supplied_required_scopes, usability]
          properties:
            permission_set_id: {type: string, format: uuid}
            supplied_required_scopes:
              type: array
              items: {type: string}
              description: Subset of `required_scopes` this set supplies for the service.
            usability:
              type: string
              enum: [usable, missing, unusable, unavailable]
              description: |
                Usability for this candidate's effective provider-scope contribution, against
                the same verified session snapshot as the baseline. Intersect with the fixed
                required-scope ceiling only for an explicit fixed-scope requirement; otherwise
                use the full candidate contribution. Does not expose raw permission-set scopes.
```

Example fragment for spec Journey 1, Step 5:

```json
{"requested_service_ids":["880e8400-e29b-41d4-a716-446655440010"],
 "requested_service_coverage":[{"service_id":"880e8400-e29b-41d4-a716-446655440010","required_scopes":["repo","user:email"],"usability":"unusable",
   "covering_permission_sets":[
     {"permission_set_id":"770e8400-e29b-41d4-a716-446655440001","supplied_required_scopes":["repo","user:email"],"usability":"unusable"},
     {"permission_set_id":"770e8400-e29b-41d4-a716-446655440003","supplied_required_scopes":["repo"],"usability":"unusable"}]}]}
```

For live selections, Save requires selection/definition and required-scope coverage, baseline
`usability: usable`, and `usability: usable` on every selected candidate contribution. Unselected
unusable candidates never block Save. The union of these contributions equals exchange's effective
scopes, including `require_all_scopes` and agents without explicit requirements. Switching toggles
recomputes the gate locally; login/refresh invalidates and refetches the verified snapshot. The
submission rechecks fresh server-side state rather than trusting these flags.

`services[].connectionStatus` describes usability for the saved selection's effective scopes, not
an arbitrary unsaved selection or all candidates. The enum does not change.

## 6. `POST /api/consent/agents/{agent-id}/grants`: requested-service enforcement (API-CONSENT)

A new `400` case applies only when `session_token` seals requested services:

```json
{"error":"requested service not satisfied","message":"requested services must be selected, cover required scopes and have a usable session: 880e8400-e29b-41d4-a716-446655440010"}
```

The grant is not created or replaced, and `redirect_url` is not returned. Existing errors keep
their precedence. The new check runs after the existing structural validation and before
persistence. Compute usability for the submitted selection's effective provider scopes (the same
union/ceiling as exchange), never merely empty configured required scopes.

On success, `redirect_url` is the server-built sealed `original_url` plus
`&session_token=<same JWE>` (using `?` if needed). The browser follows it unchanged. The authorization
endpoint independently revalidates the protected bindings after this round trip (§2); validating
only at Save is insufficient. No browser-submitted service UUIDs replace the sealed map.

## 7. `GET /api/third-party/sessions/{service}`: service session detail (API-SESSION-DETAIL)

```yaml
/api/third-party/sessions/{service}:
  get:
    operationId: getServiceSessionDetail
    tags: [Third-Party Sessions]
    security: [preAuth: []]
    parameters:
      - name: service
        in: path
        required: true
        description: Current canonical ID or UUID of the third-party service.
        schema: {type: string, maxLength: 128}
    responses:
      "200":
        content:
          application/json:
            schema:
              type: object
              required: [data]
              properties:
                data:
                  type: object
                  required: [service, session, dependent_agents]
                  properties:
                    service:
                      type: object
                      required: [id, display_name, scope_url]
                      properties:
                        id: {type: string, format: uuid}
                        canonical_id: {type: string, nullable: true}
                        display_name: {type: string}
                        description: {type: string}
                        scope_url: {type: string, format: uri, description: Current Virtual Service Scope / page URL.}
                    session:
                      allOf: [{$ref: "#/components/schemas/SessionStatus"}]
                      nullable: true
                    dependent_agents:
                      type: array
                      items: {$ref: "#/components/schemas/AffectedAgent"}
      "401": {$ref: "#/components/responses/Unauthorized"}
      "404": {description: Unknown service, content: {application/json: {example: {error: not_found, message: service not found}}}}
      "500": {$ref: "#/components/responses/InternalError"}
```

Spec Journey 2, Step 1:

```json
{"data":{"service":{"id":"880e8400-e29b-41d4-a716-446655440010","canonical_id":"github","display_name":"GitHub Production","description":"Repository access for delegated agents","scope_url":"https://broker.example.com/sessions/github"},"session":null,"dependent_agents":[{"id":"550e8400-e29b-41d4-a716-446655440000","display_name":"Data Analysis Assistant"}]}}
```

- `dependent_agents` lists only agents whose grants the authenticated principal owns and that
  include the service.
- A definitively unusable session appears with `is_expired: true`.
- The read never attempts a provider refresh.
- Refresh and terminate continue to use the UUID routes `POST /api/third-party/{serviceId}/session/refresh`
  and `DELETE /api/third-party/{serviceId}/session`.
- `GET /api/third-party/{serviceId}/session` is unchanged. It accepts a UUID only and returns `404`
  when no session exists.

## 8. Discovery

`/.well-known/oauth-authorization-server` does not change. `scopes_supported` never lists virtual
scopes (FR-014).

## 9. Recovery `error_uri` target (ADR-RECOVERY)

The `TokenExchangeError.error_uri` description, its examples, and the `/oauth2/token` 400
narrative change the encoded return target from `<public_url>/sessions` to
`<public_url>/sessions/<identifier>`. The identifier is the canonical ID, or the UUID when there is
none. `GET /api/third-party/{serviceId}/oauth2/authorize` does not change, because its same-origin
`redirect_uri` validation already accepts this path. After the callback, the browser is redirected
to the service page with the existing `service_id` and `success` query parameters.
