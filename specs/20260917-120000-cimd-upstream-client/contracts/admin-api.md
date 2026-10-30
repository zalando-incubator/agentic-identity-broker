# Contract: Administrative API Delta

**Feature**: `20260917-120000-cimd-upstream-client` | **Date**: 2026-09-21
**Canonical source**: [`api/admin/openapi.yaml`](../../../api/admin/openapi.yaml)
**Status**: Increment 1 **CONFIRMED** by the 2026-09-17 and 2026-09-18 clarifications. Increment 2 **CONFIRMED** by issue #264 and the 2026-10-22 API-014 stakeholder decision.

This document records both confirmed increments for `api/admin/openapi.yaml`.
The root OpenAPI file remains the administrative contract.
The `PreAuthProxy` security scheme and the canonical `ErrorResponse` remain unchanged.

## Summary of the delta

| Location | Change |
|---|---|
| `Service` | Add `private_key_jwt` as a third authentication value. A CIMD service has a generated HTTPS `client_id` and omits `client_secret`. |
| `ServiceCreateRequest` | Permit `private_key_jwt`. Forbid a caller `client_id` and a non-empty `client_secret`. |
| `ServiceUpdateRequest` | Apply the same private mode rules with existing full-replacement semantics. |
| `/api/cimd-client-keys` | Add generate and list operations. |
| `/api/cimd-client-keys/{kid}/current` | Add immediate promotion. |
| `/api/cimd-client-keys/{kid}` | Add guarded removal. |
| `ErrorResponse` | Unchanged. `error` is required. `message` remains optional. |

## 1. Service authentication representation

### 1.1 `Service` read and list schema

Extend `token_endpoint_auth_method` to this nullable enum:

```yaml
enum: [none, private_key_jwt, null]
```

Keep the property required and nullable on read responses.
Its meanings are:

| Value | Service posture | `client_id` source | `client_secret` response |
|---|---|---|---|
| `null` | Static confidential | Operator | `REDACTED` |
| `none` | Static public, or CIMD public with `client_registration: cimd` | Operator or broker | Omitted |
| `private_key_jwt` | CIMD confidential | Broker | Omitted |

For a CIMD service, the `client_id` is the stable HTTPS Client ID Metadata Document URL.
The URL is immutable while `client_registration` is `cimd`. A change to `static` replaces this identity.

The canonical `Service` response must omit `client_secret` for both secretless modes.
It must not use a redacted placeholder for a CIMD service.

### 1.2 Create request

Extend `ServiceCreateRequest.token_endpoint_auth_method` with `private_key_jwt`.
Retain omitted and `null` as static confidential behavior.

When the method is `private_key_jwt`:

- `client_id` must be omitted or null.
- `client_secret` must be omitted, null, or empty.
- A non-empty `client_id` or `client_secret` returns the existing 400 error representation.
- `oauth2_flavor: google` returns the existing 400 error representation.
- The broker creates the service ID first, then assigns its HTTPS client ID URL.

The handler must reject contradictions before endpoint discovery or provider traffic.

### 1.3 Update request

`ServiceUpdateRequest` retains the existing full-replacement semantics.
When the method is `private_key_jwt`, the broker ignores no stored identity or secret.
It validates the complete replacement, then removes a prior static secret and assigns the stable client ID URL.

A change from CIMD confidential to static confidential requires a new non-empty secret and an operator `client_id` in that request.
A change from CIMD confidential to either public mode removes secret state and stops client assertion use.

## 2. CIMD key routes

Add these authenticated routes under the existing root `PreAuthProxy` security requirement:

| Method | Path | Operation | Success |
|---|---|---|---|
| `POST` | `/api/cimd-client-keys` | Generate an ES256 CIMD key | `201` and key metadata |
| `GET` | `/api/cimd-client-keys` | List active CIMD keys | `200` and `{ "items": [...] }` |
| `PUT` | `/api/cimd-client-keys/{kid}/current` | Promote a CIMD key immediately | `200` and key metadata |
| `DELETE` | `/api/cimd-client-keys/{kid}` | Remove a non-signing CIMD key | `204` |

The routes are flat. They are not under `/api/oauth2-server/` or `/api/services/`.
They are present in `proxy`, `local`, and `hybrid` modes.
The token-signing key routes remain absent in proxy mode.

### 2.1 Key representation

Use one representation for create, list, and promotion:

```yaml
kid: string
algorithm: ES256
is_current: boolean
activates_at: date-time
created_at: date-time
```

The generation request body is optional:

```yaml
algorithm: ES256 # default and only accepted value
```

The representation never includes private key bytes, PEM, ciphertext, branch-key identifiers, or assertion data.

The first generated CIMD key becomes usable immediately.
A later generated current key enters the existing grace period and appears in the CIMD JWK Set first. A promoted existing key becomes usable immediately.

### 2.2 Errors

Reuse the canonical administrative `ErrorResponse` without a new schema.
Use the existing status mapping:

| Condition | Status |
|---|---|
| Unsupported algorithm or invalid request | `400` |
| Unknown key | `404` |
| Last, current, or effective-current key removal | `409` |
| Internal failure | `500` |

`error` remains required. `message` remains optional.
No error value can contain private key material, credentials, assertions, codes, or tokens.

## 3. Documentation and confirmation record

Update `api/admin/openapi.yaml` before implementation.
Update `docs/reference/api.md`, `docs/guides/manage-agents-and-services.md`, and `docs/guides/operate-oauth2-server-modes.md` from that root contract.

Redocusaurus reads the root OpenAPI file. This delta does not define a parallel API surface.

The specification records stakeholder approval for:

- The `private_key_jwt` service semantics.
- The two public document URLs.
- The four flat CIMD key operations.
- The ES256-only key representation.
- Reuse of the existing administrative error representation.

## 4. Increment 2 (issue #264): client registration

The canonical Admin OpenAPI `info.version` changes from `2.0.0` to `2.1.0`. This increment augments the existing `/api/services` contract.

### 4.1 Service field and valid combinations

`Service.client_registration` is required, non-null, and always present in create, get, update, and list responses. Its enum is `static` or `cimd`.
`ServiceCreateRequest.client_registration` and `ServiceUpdateRequest.client_registration` are optional but non-null when supplied. They use the same enum.
Client registration selects who assigns the client identifier. `token_endpoint_auth_method` independently selects token-endpoint authentication.

| `client_registration` | `token_endpoint_auth_method` | Mode | `client_id` | `client_secret` response |
|---|---|---|---|---|
| `static` | Absent or `null` | Static confidential | Operator-supplied, except existing Google and filesystem behavior | `REDACTED` for stored services; omitted for filesystem |
| `static` | `none` | Static public | Operator-supplied | Omitted |
| `cimd` | `private_key_jwt` | CIMD confidential | Broker-hosted client identifier URL | Omitted |
| `cimd` | `none` | CIMD public | Broker-hosted client identifier URL | Omitted |

Reject `static` with `private_key_jwt` and `cimd` with absent or `null` authentication. Return `400` before discovery or provider traffic, without storing the request.
The existing discovery-backed DCR authentication methods remain valid only when discovery selects them. They report `client_registration: static`.

When a create or full-replacement request omits `client_registration`, resolve it to `cimd` for `private_key_jwt` and `static` otherwise.
The broker never infers CIMD public from `none`. An explicit `null`, empty string, or unknown registration returns `400`.
The stakeholder confirmed this non-null rule on 2026-10-22 (D-A, API-014).

For both CIMD modes, the broker assigns `<server.enduser.public_url>/.well-known/oauth-client/<service-id>` after creating the service ID.
The caller must omit `client_id` or send `null`, and must omit `client_secret` or send `null` or an empty string.
A non-empty caller `client_id` or `client_secret` returns `400` before provider traffic. CIMD public requires no CIMD client-authentication key.
Google flavor and `credential_source: filesystem` reject `cimd` with `400`. Explicit `static` remains valid for eligible services.

### 4.2 Discovery and full replacement

If a request supplies `discovery.resource_url`, it must omit `client_registration`. Presence returns `400` before discovery traffic.
When discovery selects hosted CIMD, the response reports `cimd` and `discovery.client_method: cimd`.
When discovery selects DCR, the response reports `static` and `discovery.client_method: dcr`.
`discovery.client_method` remains the authoritative discovery axis. Existing selection rules never select CIMD public.
The stakeholder confirmed the complete discovery rule on 2026-10-22 (D-B, API-014).

`PUT` remains full replacement. Switching CIMD confidential to CIMD public or back keeps the client identifier URL.
The switch to CIMD confidential requires a usable CIMD client-authentication key.
Switching from `static` to `cimd` replaces the operator client ID with the broker URL.
The broker removes a stored secret only after the complete replacement validates.
Switching from `cimd` to `static` requires an operator `client_id`. Static confidential also requires a non-empty `client_secret`.
After a switch to `static`, the old hosted document returns `404`. Providers can cache the former document for 300 seconds.
The broker never retries token acquisition with another authentication method after a provider rejection.
If a CIMD public `PUT` omits `client_registration`, it resolves to `static` and fails without a caller `client_id`.

### 4.3 Complete CIMD public Service response

This example includes every required `Service` property and omits the secret. The response has no discovery-selected client method.

```json
{
  "id": "770e8400-e29b-41d4-a716-446655440002",
  "display_name": "ServiceNow",
  "credential_source": "stored",
  "client_registration": "cimd",
  "client_id": "https://broker.example.com/.well-known/oauth-client/770e8400-e29b-41d4-a716-446655440002",
  "token_endpoint_auth_method": "none",
  "oauth2_flavor": "standard",
  "issuer_uri": "https://auth.example.com",
  "discovery": {
    "enable_discovery": false,
    "client_method": null,
    "authorization_param_resource_strategy": null
  },
  "endpoints": {
    "token_endpoint": "https://auth.example.com/oauth/token",
    "authorize_endpoint": "https://auth.example.com/oauth/authorize"
  },
  "created_at": "2026-10-22T10:00:00Z",
  "updated_at": "2026-10-22T10:00:00Z"
}
```

### 4.4 Error representation and confirmation

Reuse the canonical Admin `ErrorResponse`. The following examples show the response shape, not prescribed validation-message text.

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed","message":"invalid client_registration"}
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"validation failed","message":"incompatible client registration and authentication method"}
```

Issue #264 approves the registration field, its values, the valid matrix, omission behavior, read/list reporting, and legacy classification.
Existing `private_key_jwt` services report `cimd`. All other stored services report `static` without changes to their identity, authentication, or sessions.
The stakeholder confirmed API-014 decisions D-A, D-B, D-C, and D-D in writing on 2026-10-22. The admin version change to `2.1.0` is D-D. The end-user document shape is D-C.
The stakeholder accepted the decision now in ADR 20261022-120100, including the rolling-upgrade limitation, on the same date.
