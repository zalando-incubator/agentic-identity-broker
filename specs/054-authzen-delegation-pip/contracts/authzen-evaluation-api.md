# Contract (Accepted): AuthZEN Discovery and Access Evaluation

**Status**: **Accepted** under approval items API-1–API-3 (👍 on
<https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6504077937>) and
applied to `api/enduser/openapi.yaml` (`1.2.0`). The API-003 verification against the AuthZEN 1.0
source text is recorded in [quickstart.md](../quickstart.md) §7. It found one conflict: AuthZEN
requires receivers to ignore unknown request fields. Following the user's direction in the
implementation session, unknown members, `properties` and other `context` members are ignored
instead of rejected (ADR 045 §1). Error bodies keep `ErrorResponse`. The maintainer confirmed that revision
of API-2 on 2026-10-31. The schemas below keep the original proposal for
traceability; the OpenAPI is authoritative.

**Server**: End-user server (port 8000). **Availability**: only when token exchange is
configured (the builder's `tokenExchangeEnabled`). No admin API change is made.

**OpenAPI version (Proposed)**: `info.version` changes from the current `1.1.0` to `1.2.0`. The change is additive; see §3.

---

## 1. `GET /.well-known/authzen-configuration`

| Aspect | Value |
|---|---|
| Authentication | None (public metadata) |
| Response | `200 application/json`, `Cache-Control: max-age=3600` |
| Disabled deployment | `404` (the `/.well-known` group has no matching route) |

```yaml
AuthZENConfiguration:
  type: object
  required: [policy_decision_point, access_evaluation_endpoint]
  additionalProperties: false
  properties:
    policy_decision_point:
      type: string
      format: uri
      description: server.enduser.public_url without trailing slash. Consumers MUST compare it exactly
        with their expected identifier before sending credentials.
      example: https://broker.example.com
    access_evaluation_endpoint:
      type: string
      format: uri
      example: https://broker.example.com/access/v1/evaluation
```

**Verify (API-003)**:

- Confirm the member names.
- Confirm whether other AuthZEN endpoint members must be absent or listed. Batch
  (`access_evaluations_endpoint`) and search endpoints are out of scope and are omitted.
- Confirm the well-known path insertion rule for identifiers that contain a path.

## 2. `POST /access/v1/evaluation`

### 2.1 Request

| Aspect | Value |
|---|---|
| Authentication | `Authorization: Bearer <client assertion JWT>`, validated by the existing token-exchange client-assertion trust anchor (ADR 029) and authorized by the unchanged `token_exchange.authorization.cel.expression` with `request.grant_type = urn:ietf:params:oauth:grant-type:token-exchange`, `request.resource = resource.id`, and an empty `request.scope` |
| Headers | `Content-Type: application/json` (required). `X-Request-ID` is optional and echoed. |
| Body limit | 64 KiB. Larger bodies return 400. |
| Security scheme | Reuse `ClientAssertion` (`http`/`bearer`), with its description extended to the evaluation endpoint |

```yaml
AccessEvaluationRequest:
  type: object
  required: [subject, action, resource]
  additionalProperties: false            # proposed strictness (research R10); review item
  properties:
    subject:
      type: object
      required: [type, id]
      additionalProperties: false
      properties:
        type: { type: string, enum: ["urn:ietf:params:oauth:token-type:jwt"] }
        id:   { type: string, minLength: 1, description: Signed subject JWT. Never echoed. }
    action:
      type: object
      required: [name]
      additionalProperties: false
      properties:
        name: { type: string, enum: ["urn:ietf:params:oauth:grant-type:token-exchange"] }
    resource:
      type: object
      required: [type, id]
      additionalProperties: false
      properties:
        type: { type: string, enum: ["protected_resource"] }
        id:   { type: string, format: uri, description: Protected-resource URI, normalized as in token exchange. }
    context:
      type: object
      additionalProperties: false
      properties:
        agent:
          type: string
          format: uuid
          description: Optional consistency anchor. Must equal the agent resolved from the verified subject
            token; a mismatch returns 400. Never replaces token-derived resolution.
```

**Review item**: The anchor representation is a plain Agent UUID string. Accepting a canonical
ID as an anchor is **not** proposed. With strict members (R10), AuthZEN `properties` objects
and `context.actor` are rejected. The stakeholder may relax this before the contract is frozen.

### 2.2 Responses

All responses echo a supplied `X-Request-ID` and never contain the subject token, assertion,
provider tokens, secrets, raw claims, or internal error text.

| Status | When | `Cache-Control` | Body |
|---|---|---|---|
| 200 | Permit | `private, max-age=<0..60>` | `AccessEvaluationPermit` |
| 200 | Grant denial (US2.1–US2.10) | `no-store` | `AccessEvaluationDenial` |
| 400 | Malformed or oversized body; unsupported or missing member; invalid or expired subject; anchor mismatch; unregistered or invalid resource | `no-store` | `ErrorResponse` (`error: invalid_request`) |
| 401 | Missing, invalid, or expired client assertion. Adds `WWW-Authenticate: Bearer` and, for an invalid credential, `error="invalid_token"`. | `no-store` | `ErrorResponse` (`error: unauthorized`) |
| 403 | Caller CEL policy denies | `no-store` | `ErrorResponse` (`error: forbidden`) |
| 500 | Configuration or dependency failure (agent missing or invalid, resource ambiguous, JWKS, CEL, repositories, canonical-ID lookup) | `no-store` | `ErrorResponse` (`error: server_error`) |

`ErrorResponse` is the existing end-user schema (`{error, message?}`). Its messages are fixed
strings per status.

```yaml
AccessEvaluationPermit:
  type: object
  required: [decision, context]
  properties:
    decision: { type: boolean, enum: [true] }
    context:  { $ref: "#/components/schemas/DelegationContext" }

DelegationContext:
  type: object
  required: [subject, agent, resource, grant, permission_sets]
  properties:
    subject:
      type: object
      required: [id]
      properties: { id: { type: string, minLength: 1, description: Verified principal } }
    agent:    { $ref: "#/components/schemas/DelegationEntityRef" }
    resource:
      type: object
      required: [id, resource_uri, scopes]
      properties:
        id:           { type: string, format: uuid, description: Resolved service UUID }
        canonical_id: { type: string, minLength: 1, description: Omitted when unset }
        resource_uri: { type: string, format: uri, description: Normalized URI }
        scopes:       { type: array, items: { type: string }, description: Effective scopes after the agent ceiling }
    grant:
      type: object
      required: [id, updated_at]
      properties:
        id:          { type: string, format: uuid }
        updated_at:  { type: string, format: date-time }
        valid_until: { type: string, format: date-time, description: Omitted when the grant does not expire }
    permission_sets:
      type: array
      minItems: 1
      items:
        type: object
        required: [id, services]
        properties:
          id:           { type: string, format: uuid }
          canonical_id: { type: string, minLength: 1, description: Omitted when unset }
          services:
            type: array
            description: Grant-included services only. Scopes are permission-set contributions, not the effective ceiling.
            items:
              type: object
              required: [id, scopes]
              properties:
                id:           { type: string, format: uuid }
                canonical_id: { type: string, minLength: 1, description: Omitted when unset }
                scopes:       { type: array, items: { type: string } }

DelegationEntityRef:
  type: object
  required: [id]
  properties:
    id:           { type: string, format: uuid }
    canonical_id: { type: string, minLength: 1, description: Omitted when unset; never null }

AccessEvaluationDenial:            # proposed form (a); see Verify below
  type: object
  required: [decision, context]
  properties:
    decision: { type: boolean, enum: [false] }
    context:
      type: object
      required: [reason_admin, reason_user]
      properties:
        reason_admin:
          type: object
          required: [code]
          properties:
            code:
              type: string
              enum: [grant_missing, grant_expired, grant_empty, grant_permission_set_undeclared,
                     grant_permission_set_missing, grant_service_omitted, grant_service_definition_missing,
                     grant_service_requirement_excluded, grant_scope_intersection_empty]
        reason_user:
          type: object
          required: [consent_url]
          properties:
            consent_url: { type: string, format: uri, example: "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000" }
```

**Spec reconciliation**: `grant.valid_until` is omitted when the grant does not expire. This
reconciles the "nullable grant expiry" item in API-002 with the FR-008 omission convention.
Spec Journey 1 shows `valid_until` because that grant expires.

**Verify (API-003), denial envelope**: AuthZEN's informative reason examples are
language-tagged objects. Before the contract is frozen, choose one form:

- **(a)** The machine-readable form above. This is the spec's proposal and the recommendation,
  because a policy engine consumes the code and URL directly.
- **(b)** Language-tagged strings plus extension members, for example:

```json
{"decision": false, "context": {
  "reason_admin": {"en": "grant_missing"},
  "reason_user": {"en": "Consent required"},
  "consent_url": "https://broker.example.com/agents/550e8400-e29b-41d4-a716-446655440000"}}
```

The ExtProc client (see [extproc-delegation-context.md](./extproc-delegation-context.md))
parses whichever form is confirmed.

### 2.3 Examples

Requests and responses follow spec Journeys 1–3 exactly. Each scenario becomes an OpenAPI
`examples` entry:

- `permit` (Journey 1 step 3)
- `permitWithoutCanonicalIds` (step 3a)
- `permitExpiringSoon` (`max-age=15`)
- `denyGrantMissing` (Journey 2 step 3)
- `unauthorizedMissingAssertion` (Journey 3 step 1)
- `badRequestIdentitySubject`
- `badRequestAgentMismatch`
- `forbiddenCallerPolicy`
- `serverError`

## 3. `/oauth2/token` response deprecation (additive documentation)

**Proposed release version**: end-user OpenAPI `info.version` increases from the existing
`1.1.0` (`api/enduser/openapi.yaml:92`) to `1.2.0` for the additive endpoints and deprecation
marker. This corrects the stale `1.0.0` baseline used in the earlier design review; it does not
claim maintainer approval of the corrected version. Alternative: intentionally reuse `1.1.0`,
which would not identify this additive contract revision separately. Sign-off remains API-3.

The `TokenExchangeResponse.granted_permission_sets` property gains `deprecated: true` and this
description suffix: "Deprecated: superseded by the AuthZEN Delegation Context from
`POST /access/v1/evaluation`. Removal is scheduled for one release after the ExtProc delegation
resolver ships and requires end-user API 2.0.0." The response behavior is unchanged.
`principal` and `agent_id` are unchanged and not deprecated.

## 4. Rendered documentation

- Add `docs/api/authzen-evaluation.md`. It covers discovery, the request and response, error
  mapping, freshness, the denial envelope, caller-policy reuse, and the journeys.
- Link the new page from `docs/api/enduser-api.md`. Rebuild the docs site with `just docs-build`.
