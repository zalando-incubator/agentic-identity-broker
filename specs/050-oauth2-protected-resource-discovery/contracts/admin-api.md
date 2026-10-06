# Contract: Administrative API Delta

**Feature**: `050-oauth2-protected-resource-discovery` | **Date**: 2026-10-06
**Canonical source**: [`api/admin/openapi.yaml`](../../../api/admin/openapi.yaml)
**Status**: Confirmed core fields and route; representation details require review in the canonical OpenAPI file before implementation.

This document is the delta for `api/admin/openapi.yaml`. The root OpenAPI file remains the administrative contract. The `PreAuthProxy` security scheme, `ServiceId` parameter, ETag behavior, and service resource names remain unchanged.

The 2026-10-01 clarification confirms these items:

- `discovery.resource_url` with `discovery.enable_discovery: true`.
- The effective `authorization_params.resource` value.
- `GET /api/services/{service-id}/discovery-status`.
- Status fields `status`, `resource_url`, `issuer_uri`, `client_method`, `last_attempt_at`, `last_success_at`, and `failure_reason`.

The new `token_endpoint_auth_method` response values, the `discovery.client_method` response field, and failure codes follow from API-002, API-003, and API-005. Record stakeholder review of these details in the PR before implementation.
The 2026-10-06 stakeholder feedback confirms returning validated `authorization_servers` when issuer selection is required.

## Summary of the delta

| Location | Change |
|---|---|
| `DiscoveryConfigRequest` | Add `resource_url`. It requires `enable_discovery: true` and excludes `metadata_url`. |
| `ServiceCreateRequest` | Accept protected-resource discovery without client or endpoint fields. Make `issuer_uri` optional for this mode. |
| `ServiceUpdateRequest` | Re-run discovery with full-replacement rules. Preserve unchanged issuer and client identity. |
| `Service` | Return the discovery source, broker-selected client method, effective resource, and explicit DCR authentication method. Do not return a DCR secret. |
| `/api/services/{service-id}/discovery-status` | Add an authenticated, read-only status resource. |
| `ErrorResponse` | Keep `error` and `message`. Add optional `authorization_servers` only for `issuer_selection_required`. Add safe failure-code messages. |

## 1. Request representation

### 1.1 `DiscoveryConfigRequest`

Add this optional property:

```yaml
resource_url:
  type: string
  format: uri
  nullable: true
  description: >-
    Public HTTPS protected-resource identifier. Requires enable_discovery true.
    Excludes metadata_url, manual endpoints, and client credentials.
  example: https://mcp.example.com/mcp
```

A protected-resource request must satisfy these rules before network access:

| Field | Rule |
|---|---|
| `discovery.enable_discovery` | Must be `true`. |
| `discovery.resource_url` | Required for this mode. Must be absolute HTTPS with a host and no fragment or user information. |
| `discovery.metadata_url` | Must be absent or `null`. |
| `endpoints` | Must be absent or `null`. |
| `client_id`, `client_secret` | Must be absent, `null`, or empty. |
| `token_endpoint_auth_method` | Must be absent or `null`. The broker selects the method. |
| `oauth2_flavor` | Must be absent or `standard`. |
| `issuer_uri` | Optional. If present, it must exactly equal one advertised issuer. It is required when metadata advertises more than one issuer. |
| `authorization_params.resource` | Optional explicit override. If present, it must be a non-empty absolute URI without a fragment. |
| `scopes`, `protected_resources`, `canonical_id`, `display_name` | Existing rules apply. |

An omitted `token_endpoint_auth_method` does not mean static confidential mode when `resource_url` is present. Existing manual and direct-metadata requests keep their present meaning. Their accepted request method values remain `none`, `private_key_jwt`, omitted, or `null`.

### 1.2 Create example

```json
{
  "display_name": "Example MCP",
  "issuer_uri": "https://auth.example.com/tenant",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp"
  },
  "scopes": [
    { "scope_value": "files:read", "description": "Read files" }
  ]
}
```

### 1.3 Update semantics

`PUT /api/services/{service-id}` keeps full-replacement semantics.

| Request | Result |
|---|---|
| Same verified issuer and same client and token authentication methods | Update metadata and endpoints. Keep the client ID, DCR secret, and user sessions. |
| `issuer_uri` omitted for an existing discovery-backed service | Use the active issuer only if the resource still advertises it. Do not select another advertised issuer. |
| Explicit different advertised `issuer_uri` | Fail with `409` when the service has user sessions. Otherwise, keep the exact `discovery.client_method` and `token_endpoint_auth_method`. Register a new DCR client only with that saved authentication method. If the new issuer or registration cannot support it, reject the update without public or alternate-method fallback. |
| Different client bootstrap or token authentication method required by metadata | Reject the update, including after explicit issuer selection. This API has no method selector. Keep the active client unchanged. |
| `authorization_params` omitted | Keep the current parameter map and explicit/derived source. |
| `authorization_params` present with `resource` | Store that value as an explicit override. |
| `authorization_params` present without `resource` | Clear the explicit override and use the verified resource URL. |
| Manual configuration without `resource_url` | Apply the existing manual replacement contract. Remove discovery source, DCR credential, and discovery timestamps after success. |

A request-shape error happens before discovery and does not change status. A failure after remote discovery starts updates only `last_attempt_at` and `failure_reason`. It does not change active configuration, `last_success_at`, the service ETag, or user sessions.

## 2. `Service` representation

Add this read-only property under `discovery`:

```yaml
client_method:
  type: string
  nullable: true
  enum: [cimd, dcr, null]
  readOnly: true
```

Extend the read-only response enum for `token_endpoint_auth_method`:

```yaml
enum: [none, private_key_jwt, client_secret_basic, client_secret_post, null]
```

The new values appear only for discovery-backed DCR clients.

| Service | `discovery.client_method` | `token_endpoint_auth_method` | `client_secret` |
|---|---|---|---|
| Manual static confidential | `null` | `null` | `REDACTED` |
| Manual public | `null` | `none` | Omitted |
| Manual CIMD | `null` | `private_key_jwt` | Omitted |
| Discovery CIMD | `cimd` | `private_key_jwt` | Omitted |
| Discovery confidential DCR | `dcr` | `client_secret_basic` or `client_secret_post` | Omitted |
| Discovery public DCR | `dcr` | `none` | Omitted |

`none` identifies a public client. Every other non-null method identifies a confidential client. A consumer identifies a DCR client by `issuer_uri` and `client_id` together.

Discovery-backed read, list, create, and update responses always contain `authorization_params.resource`. Its value is the effective value for authorization and token requests.

### 2.1 Example response

```json
{
  "id": "6c84fb90-12c4-11e1-840d-7b25c5ee775a",
  "canonical_id": null,
  "display_name": "Example MCP",
  "client_id": "dcr-client-123",
  "token_endpoint_auth_method": "client_secret_basic",
  "oauth2_flavor": "standard",
  "issuer_uri": "https://auth.example.com/tenant",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://mcp.example.com/mcp",
    "client_method": "dcr"
  },
  "endpoints": {
    "token_endpoint": "https://auth.example.com/tenant/token",
    "authorize_endpoint": "https://auth.example.com/tenant/authorize"
  },
  "scopes": [
    { "scope_value": "files:read", "description": "Read files" }
  ],
  "authorization_params": {
    "resource": "https://mcp.example.com/mcp"
  },
  "created_at": "2026-10-06T10:00:00Z",
  "updated_at": "2026-10-06T10:00:00Z"
}
```

## 3. Discovery status resource

Add this route under the root `PreAuthProxy` security requirement:

| Method | Path | Operation | Success |
|---|---|---|---|
| `GET` | `/api/services/{service-id}/discovery-status` | `getServiceDiscoveryStatus` | `200` and `ServiceDiscoveryStatus` |

The route accepts the existing UUID or canonical service ID forms. It reads persisted state only and never contacts a provider.

```yaml
ServiceDiscoveryStatus:
  type: object
  required:
    - status
    - resource_url
    - issuer_uri
    - client_method
    - last_attempt_at
    - last_success_at
    - failure_reason
  properties:
    status:
      type: string
      enum: [ready, failed, not_applicable]
    resource_url:
      type: string
      format: uri
      nullable: true
    issuer_uri:
      type: string
      format: uri
      nullable: true
    client_method:
      type: string
      enum: [cimd, dcr]
      nullable: true
    last_attempt_at:
      type: string
      format: date-time
      nullable: true
    last_success_at:
      type: string
      format: date-time
      nullable: true
    failure_reason:
      type: string
      nullable: true
      description: Safe failure code from section 4.1, or null after success and for manual services.
```

The response examples are:

```json
{
  "status": "failed",
  "resource_url": "https://mcp.example.com/mcp",
  "issuer_uri": "https://auth.example.com/tenant",
  "client_method": "dcr",
  "last_attempt_at": "2026-10-06T11:00:00Z",
  "last_success_at": "2026-10-06T10:00:00Z",
  "failure_reason": "authorization_server_metadata_invalid"
}
```

```json
{
  "status": "not_applicable",
  "resource_url": null,
  "issuer_uri": null,
  "client_method": null,
  "last_attempt_at": null,
  "last_success_at": null,
  "failure_reason": null
}
```

On failure, `resource_url`, `issuer_uri`, and `client_method` describe the active configuration. The status does not return a failed replacement URL. The effective token audience appears only in the service's `authorization_params.resource`.

| Condition | Status |
|---|---|
| Existing manual or discovery-backed service | `200` |
| Malformed service identifier | `400` |
| Unknown or deleted service | `404` |
| Internal failure | `500` |

## 4. Errors

Extend `ErrorResponse` with optional `authorization_servers`. `error` remains required and `message` remains optional. Its error categories remain unchanged.

| Condition | Status | `error` | `message` |
|---|---|---|---|
| Invalid or conflicting request fields | `400` | `validation failed` | Existing field-level text. |
| Metadata, issuer, or destination failure | `400` | `discovery failed` | One failure code from section 4.1. |
| CIMD or DCR selection or registration failure | `400` | `client registration failed` | One failure code from section 4.1. |
| Discovery or registration deadline exceeded | `504` | `discovery failed` | `timeout` |
| Same-issuer DCR identity already exists | `409` | `conflict` | `duplicate_client_identity` |
| Explicit issuer change while user sessions exist | `409` | `conflict` | `issuer_change_requires_no_sessions` |
| Existing ETag or version precondition failure | `409`, `412`, or `428` | Existing value | Existing text. |
| Local storage or encryption failure | `500` | `internal server error` | Omitted. |

On `issuer_selection_required`, `message` remains the safe failure code. The authenticated error includes every validated `authorization_servers` URI from matching Protected Resource Metadata, in published order. Other errors omit the property. The broker never returns raw metadata, issuer URLs with queries, secrets, or provider response bodies.

```yaml
authorization_servers:
  type: array
  minItems: 2
  uniqueItems: true
  items:
    type: string
    format: uri
  description: Validated issuer candidates from the matching protected resource, returned only with issuer_selection_required.
```

```json
{"error":"discovery failed","message":"issuer_selection_required","authorization_servers":["https://login-a.example.com","https://login-b.example.com"]}
```

These are candidates, not proof that an authorization server accepts the broker's client method. On retry, the broker fetches fresh resource metadata and rejects a selected issuer that is no longer advertised. It contacts only the selected authorization server after that check.

### 4.1 Failure codes

Discovery error messages, `failure_reason` values, and audit records use only these failure codes:

| Code | Meaning |
|---|---|
| `resource_metadata_not_found` | All applicable protected-resource locations returned `404`. |
| `resource_metadata_unavailable` | A resource metadata request failed or returned an unexpected status. |
| `resource_metadata_invalid` | A resource challenge or metadata document is malformed or unsupported. |
| `resource_mismatch` | The metadata resource is not identical to `resource_url`. |
| `authorization_server_missing` | Resource metadata lists no authorization server. |
| `issuer_selection_required` | Resource metadata lists more than one valid issuer and no `issuer_uri` was supplied. The error includes the candidates in `authorization_servers`. |
| `issuer_not_advertised` | The supplied or active issuer is not in resource metadata. |
| `authorization_server_metadata_not_found` | All applicable issuer metadata locations returned `404`. |
| `authorization_server_metadata_unavailable` | Issuer metadata failed or returned an unexpected status. |
| `authorization_server_metadata_invalid` | Issuer metadata is malformed or lacks a required endpoint. |
| `issuer_mismatch` | Issuer metadata is not identical to the selected issuer. |
| `unsafe_destination` | A URL is not public HTTPS, resolves to a blocked address, or redirects. |
| `response_too_large` | A remote response exceeds 256 KiB. |
| `timeout` | The 15-second attempt deadline expired. |
| `no_compatible_client_method` | No compatible CIMD or DCR method exists. |
| `cimd_unavailable` | The selected hosted CIMD identity or signing key is not usable. |
| `client_registration_rejected` | The selected DCR registration returned an error. |
| `client_registration_invalid` | DCR returned an empty ID, incompatible callback, missing secret, another method, or a non-zero `client_secret_expires_at`. |
| `client_method_changed` | An update would change `discovery.client_method` or the exact `token_endpoint_auth_method`, including confidential/public mode. |
| `duplicate_client_identity` | The DCR client ID already exists for the same issuer. |

No response includes a provider response body, URL query, secret, assertion, code, or token.

## 5. Documentation and confirmation record

Update `api/admin/openapi.yaml` before implementation. Then update `docs/reference/api.md` and `docs/guides/manage-agents-and-services.md` from that root contract. Correct the stale reference that says `ErrorResponse.message` is always required.

The stakeholder confirmation in [spec.md](../spec.md) covers the request field, effective resource, status route and fields, and the `authorization_servers` error field confirmed on 2026-10-06. The PR must record review of the remaining new response values in sections 2 and 4.
