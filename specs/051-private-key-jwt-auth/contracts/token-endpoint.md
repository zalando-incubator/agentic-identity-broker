# Contract: Token Endpoint Delta

**Feature**: `051-private-key-jwt-auth` | **Date**: 2026-10-15
**Canonical source**: [`api/enduser/openapi.yaml`](../../../api/enduser/openapi.yaml), `POST /oauth2/token` and `TokenExchangeRequest`
**Status**: **PROPOSED**. Requires stakeholder confirmation before implementation (API-003, Constitution Principle X).

This is the delta to apply to the end-user OpenAPI file. Nothing else in the file changes for this endpoint. The RFC 8693 token-exchange grant and its existing `client_assertion` handling are untouched.

## Summary of the delta

| Location | Change |
|---|---|
| `POST /oauth2/token` description | Add a "CIMD client authentication (`private_key_jwt`)" subsection under the authorisation-code and refresh-token grants. |
| Request schema | Document `client_assertion_type` and `client_assertion` as usable by `authorization_code` and `refresh_token` grants. Add `refresh_token` to the `grant_type` enum (implemented today, not listed). Keep their existing meaning for token exchange. |
| Responses | Document `invalid_request`, `invalid_client` (401) and `server_error` (5xx) for these cases. |

## 1. Request parameters

For a CIMD client whose metadata document declares `token_endpoint_auth_method: private_key_jwt`:

| Parameter | Required | Value |
|---|---|---|
| `grant_type` | yes | `authorization_code` or `refresh_token` |
| `client_id` | yes | The client's CIMD URL; must equal the assertion's `iss` and `sub` |
| `client_assertion_type` | yes | `urn:ietf:params:oauth:client-assertion-type:jwt-bearer` |
| `client_assertion` | yes | Compact-serialised signed JWT (below) |
| `code`, `redirect_uri`, `code_verifier` | per grant | Unchanged; PKCE S256 stays mandatory |
| `refresh_token` | per grant | Unchanged |

`client_secret` and an `Authorization` header MUST NOT accompany an assertion.

### Assertion format

Header:

| Field | Rule |
|---|---|
| `alg` | `RS256` or `ES256`; must equal the algorithm in the client's document (`RS256` if omitted there) |
| `kid` | Required; identifies a key in the client's published key set |

Claims:

| Claim | Rule |
|---|---|
| `iss`, `sub` | Both equal `client_id` |
| `aud` | The broker issuer identifier or its token endpoint URL (`<issuer>/oauth2/token`); an array is accepted if at least one entry is approved; other entries are ignored |
| `exp` | Required; at most 5 minutes in the future, plus 30 s skew |
| `iat`, `nbf` | Optional; if present, at most 30 s in the future; `exp − iat` ≤ 5 minutes |
| `jti` | Required, non-empty, single-use per client until `exp` + 30 s |

Example (illustrative values):

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=https%3A%2F%2Fchatgpt.com%2Foauth%2FEBkIgu5McFbd%2Fclient.json
&code=SplxlOBeZQQYbYS6WxSbIA
&redirect_uri=https%3A%2F%2Fchatgpt.com%2Fconnector%2Foauth%2FEBkIgu5McFbd
&code_verifier=dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=eyJhbGciOiJSUzI1NiIsImtpZCI6ImsxIn0...
```

## 2. Responses

Successful responses are unchanged: `200` with the existing token response.

| Condition | HTTP | `error` |
|---|---|---|
| Missing assertion from a `private_key_jwt` client | 401 | `invalid_client` |
| Bad signature, claims, audience, expiry, lifetime, unknown `kid`, missing `jti`, unusable key, or replay | 401 | `invalid_client` |
| Assertion sent by a client that declares `none` or omits the method | 401 | `invalid_client` |
| Only one of `client_assertion` / `client_assertion_type` present; assertion combined with `client_secret` or `Authorization` | 400 | `invalid_request` |
| Client document or key set cannot be retrieved (timeout, network error, upstream failure) | 5xx | `server_error` |

Rules:
- `error_description` never names the specific failing check (FR-023). The reason is recorded only in server-side audit events.
- On any failure the authorisation code stays unconsumed and the refresh token stays unrotated (FR-011).
- `401` responses follow the existing `invalid_client` convention of this endpoint.

## 3. Compatibility

- The parameter and error rules above apply only to requests whose `client_id` resolves to a CIMD client. Proxy-mode requests are forwarded upstream unchanged, and locally registered confidential agents are unaffected.

- Clients declaring `none`, or omitting the method, are unaffected, except that an assertion sent by them is now rejected.
- Confidential locally registered agents (`client_secret`) are unaffected.
- No new endpoint, header or status code is introduced.
