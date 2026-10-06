# Contract: End-User API Delta

**Feature**: `046-cimd-upstream-client` | **Date**: 2026-09-21
**Canonical source**: [`api/enduser/openapi.yaml`](../../../api/enduser/openapi.yaml)
**Status**: **CONFIRMED** by the 2026-09-17 specification clarification.

This document is the approved delta for `api/enduser/openapi.yaml`.
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

## 2. CIMD public JWK Set

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

Return `404 Not Found` for an unknown, deleted, public, static confidential, or unavailable CIMD service.
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
