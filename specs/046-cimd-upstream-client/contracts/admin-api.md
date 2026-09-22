# Contract: Administrative API Delta

**Feature**: `046-cimd-upstream-client` | **Date**: 2026-09-21
**Canonical source**: [`api/admin/openapi.yaml`](../../../api/admin/openapi.yaml)
**Status**: **CONFIRMED** by the 2026-09-17 and 2026-09-18 specification clarifications.

This document is the approved delta for `api/admin/openapi.yaml`.
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
| `none` | Public | Operator | Omitted |
| `private_key_jwt` | CIMD confidential | Broker | Omitted |

For `private_key_jwt`, the `client_id` is the stable HTTPS Client ID Metadata Document URL.
The API must describe the URL as broker-generated and immutable for the service lifetime.

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

A private-to-static update requires a new non-empty secret in that request.
A private-to-public update removes any secret state and stops client assertion use.

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
