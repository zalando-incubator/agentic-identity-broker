# Contract: End-User API Delta

**Feature**: `20260917-120000-cimd-upstream-client` | **Date**: 2026-09-21
**Canonical source**: [`api/enduser/openapi.yaml`](../../../api/enduser/openapi.yaml)
**Status**: Increment 1 **CONFIRMED** by the 2026-09-17 clarification. Increment 2 **CONFIRMED** by issue #264 and the 2026-10-22 API-014 stakeholder decision.

This document records both confirmed increments for `api/enduser/openapi.yaml`.
The root OpenAPI file remains the end-user contract.
The canonical end-user `ErrorResponse` remains unchanged.

## Summary of the delta

| Method | Path | Authentication | Success |
|---|---|---|---|
| `GET` | `/.well-known/oauth-client/{service-id}` | None | Client ID Metadata Document |
| `GET` | `/.well-known/oauth-client/{service-id}/jwks.json` | None | CIMD client-authentication JWK Set |

Both operations are on the end-user server.
They are in the existing `/.well-known` route block before the SPA fallback.
They must set `security: []` in the root OpenAPI document.

## 1. Client ID Metadata Document

### Path

```text
GET /.well-known/oauth-client/{service-id}
```

`service-id` is the immutable service UUID.
The retrieval URL must exactly equal the returned `client_id`.

### Successful response

Return `200 OK` with:

```text
Content-Type: application/json
Cache-Control: public, max-age=300
```

The JSON object contains exactly these public fields:

```json
{
  "client_id": "https://broker.example.com/.well-known/oauth-client/<service-id>",
  "redirect_uris": [
    "https://broker.example.com/api/third-party/<service-id>/oauth2/callback"
  ],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "private_key_jwt",
  "token_endpoint_auth_signing_alg": "ES256",
  "jwks_uri": "https://broker.example.com/.well-known/oauth-client/<service-id>/jwks.json"
}
```

Use `server.enduser.public_url` as the URL base.
Do not use the admin URL, request host header, or a second CIMD base URL.

## 2. Public JWK Set for CIMD confidential services

### Path

```text
GET /.well-known/oauth-client/{service-id}/jwks.json
```

Return `200 OK` with:

```text
Content-Type: application/json
Cache-Control: public, max-age=300
```

The response is an RFC 7517 JWK Set.
Each key is from the `cimd_client_authentication` domain and has:

```json
{
  "kty": "EC",
  "crv": "P-256",
  "kid": "<globally-unique-kid>",
  "use": "sig",
  "alg": "ES256",
  "x": "<base64url-coordinate>",
  "y": "<base64url-coordinate>"
}
```

The JWK Set includes a generated key during its activation grace period.
It retains non-current keys until an operator explicitly removes them.
It never includes token-signing keys, upstream keys, private JWK members, encrypted key material, or secrets.

## 3. Unavailable document response

Return `404 Not Found` for an unknown, deleted, static public, static confidential, or unavailable CIMD service.
Use the canonical end-user JSON `ErrorResponse`.
`error` remains required. `message` remains optional.

The endpoint must not redirect or return partial metadata or key material.
All unavailable states use the same public response shape.

## 4. Existing public key surface

`GET /oauth2/jwks.json` remains unchanged.
It continues to publish the token-signing and mode-appropriate upstream verification surfaces.
It does not publish CIMD client-authentication keys.

## 5. Documentation and confirmation record

Update `api/enduser/openapi.yaml` before implementation.
Build the Redocusaurus documentation site from that root file.
Update `docs/reference/api.md` and `docs/guides/operate-oauth2-server-modes.md` with the separation between the two JWK surfaces.

This delta does not define a parallel end-user API surface.

## 6. Increment 2 (issue #264): CIMD public document

The canonical end-user OpenAPI `info.version` changes from `1.0.0` to `1.1.0` (D-D). The existing public routes and security requirements remain unchanged.

### 6.1 Closed metadata variants

`CIMDClientMetadataResponse` has a closed `oneOf` of two variants. Each variant rejects additional properties.
The CIMD confidential variant retains exactly the seven members shown in section 1.
It requires `token_endpoint_auth_method: private_key_jwt`, `token_endpoint_auth_signing_alg: ES256`, and `jwks_uri`.
The CIMD public variant has exactly five required members: `client_id`, `redirect_uris`, `grant_types`, `response_types`, and `token_endpoint_auth_method`.
Its method is `none`. It has no `jwks_uri` or `token_endpoint_auth_signing_alg`.

For a CIMD public service, `GET /.well-known/oauth-client/{service-id}` returns `200 OK` with the unchanged headers:

```http
HTTP/1.1 200 OK
Content-Type: application/json
Cache-Control: public, max-age=300

{
  "client_id": "https://broker.example.com/.well-known/oauth-client/770e8400-e29b-41d4-a716-446655440002",
  "redirect_uris": ["https://broker.example.com/api/third-party/770e8400-e29b-41d4-a716-446655440002/oauth2/callback"],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

The retrieval URL exactly matches `client_id`. The broker serves this document without a CIMD client-authentication key.
The redirect URI uses the same `server.enduser.public_url` and service ID as the client identifier URL.

### 6.2 JWK Set and unavailable states

For a CIMD public service, `GET /.well-known/oauth-client/{service-id}/jwks.json` returns the uniform section 3 JSON `404`.
It does not return a JWK Set or expose key availability. A CIMD confidential service retains the section 2 JWK Set.
The metadata endpoint returns the same uniform `404` for unknown, deleted, static public, static confidential, or unavailable CIMD services.
The existing `GET /oauth2/jwks.json` behavior remains unchanged.

### 6.3 Confirmation record

Issue #264 approves independent client registration and the new `cimd` plus `none` mode.
The stakeholder confirmed D-C on 2026-10-22: the five-member document, keyless availability, JWK Set `404`, and closed variant shape.
The stakeholder confirmed end-user version `1.1.0` under D-D on the same date. The stakeholder also accepted the decision now in ADR 20261022-120100, including the rolling-upgrade limitation.
