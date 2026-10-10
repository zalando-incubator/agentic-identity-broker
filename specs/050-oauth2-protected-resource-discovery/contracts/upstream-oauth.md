# Contract: Upstream Discovery, Registration, and Token Requests

**Feature**: `050-oauth2-protected-resource-discovery` | **Date**: 2026-10-06
**Consumers**: Protected resources, authorization servers, DCR endpoints, and E2E provider doubles.

This document specifies the broker's outbound HTTP behavior. It adds no End-user API route. Existing hosted CIMD routes from feature 046 remain unchanged.

## 1. Common outbound rules

Every request in sections 2–4 obeys these rules:

- The URL is absolute HTTPS with a host. It has no user information or fragment.
- In production, the dialer rejects private, loopback, link-local, cloud-metadata, transition, and other blocked addresses after DNS resolution and before TCP connect.
- The client accepts no redirect.
- The response body limit is 256 KiB. A larger body fails with `response_too_large`.
- One discovery or registration attempt has one 15-second deadline across all requests.
- The broker sends no service credential, user token, cookie, or browser header to a metadata URL.
- Audit and error data contain only identifiers, request class, outcome, and the failure codes in [admin-api.md](./admin-api.md#41-failure-codes).

Metadata requests send `Accept: application/json`. A successful metadata response is `200` with a JSON content type. A `404` permits only the next location defined in this contract. Every other response is final for that attempt.

## 2. Protected resource metadata

### 2.1 Challenge request

The broker sends one unauthenticated `GET` to `discovery.resource_url`.

If a `401` response has exactly one `resource_metadata` occurrence in a Bearer or DPoP `WWW-Authenticate` challenge, the broker fetches that URL. It never falls back to well-known locations after that fetch fails.

If the challenge has duplicate `resource_metadata` occurrences, even identical ones, the broker fails with `resource_metadata_invalid`. Malformed or unsupported challenges with `resource_metadata` also fail. A `resource_metadata` value on a response other than `401` fails with the same code; it never triggers a fallback.

If no `resource_metadata` value exists, the broker uses section 2.2.

### 2.2 Well-known locations

| Resource URL | Ordered metadata URLs |
|---|---|
| `https://mcp.example.com/mcp` | `https://mcp.example.com/.well-known/oauth-protected-resource/mcp`, then `https://mcp.example.com/.well-known/oauth-protected-resource` |
| `https://mcp.example.com` | `https://mcp.example.com/.well-known/oauth-protected-resource` |

The broker requests the root URL only after the path-specific URL returns `404`.

### 2.3 Validation

```json
{
  "resource": "https://mcp.example.com/mcp",
  "authorization_servers": ["https://auth.example.com/tenant"]
}
```

- `resource` must be identical to the configured `resource_url`.
- `authorization_servers` must be a non-empty array of distinct public HTTPS issuer identifiers without user information, query, or fragment.
- A single issuer is selected automatically.
- More than one issuer requires an identical administrator-supplied `issuer_uri`.
- If the administrator supplies no issuer for a multi-issuer resource, the Admin API error returns the validated `authorization_servers` values. It does not fetch metadata from either authorization server.
- On retry, the broker re-fetches resource metadata. A selected `issuer_uri` must still be advertised before the broker contacts that issuer.

## 3. Authorization-server metadata

### 3.1 Probe order

| Selected issuer | Ordered metadata URLs |
|---|---|
| `https://auth.example.com/tenant` | `https://auth.example.com/.well-known/oauth-authorization-server/tenant`, `https://auth.example.com/.well-known/openid-configuration/tenant`, `https://auth.example.com/tenant/.well-known/openid-configuration` |
| `https://auth.example.com` | `https://auth.example.com/.well-known/oauth-authorization-server`, `https://auth.example.com/.well-known/openid-configuration` |

The broker requests the next location only after `404`.

### 3.2 Required fields and capability selection

| Field | Rule |
|---|---|
| `issuer` | Identical to the selected issuer. |
| `authorization_endpoint` | Required public HTTPS URL. |
| `token_endpoint` | Required public HTTPS URL. It cannot include a `resource` query parameter. |
| `jwks_uri` | Optional. If present, it must be public HTTPS. |
| `client_id_metadata_document_supported` | `true` is required for hosted CIMD. |
| `token_endpoint_auth_methods_supported` | Must contain `private_key_jwt` for CIMD. If omitted, the RFC 8414 default is only `client_secret_basic`. |
| `token_endpoint_auth_signing_alg_values_supported` | Must contain `ES256` for CIMD. |
| `registration_endpoint` | Required for DCR. It must be public HTTPS. |
| `code_challenge_methods_supported` | Must contain `S256` for public DCR. |

The selection order is:

1. Hosted CIMD with `private_key_jwt`.
2. DCR with `client_secret_basic`.
3. DCR with `client_secret_post`.
4. DCR with `none`.

If the selected hosted CIMD identity or key is unavailable, the attempt fails. It does not try DCR. If selected confidential DCR fails, the attempt fails. It does not try public DCR.

The preference order applies only to creation. On update, keep the exact active `client_method` and `token_endpoint_auth_method`. If a new advertised issuer cannot support that authentication method, fail without another registration method. A confidential client never changes to public during an update.

## 4. Dynamic client registration

### 4.1 Request

The broker sends one `POST` with `Content-Type: application/json` to `registration_endpoint`. It sends no initial access token.

```json
{
  "redirect_uris": [
    "https://broker.example.com/api/third-party/6c84fb90-12c4-11e1-840d-7b25c5ee775a/oauth2/callback"
  ],
  "client_name": "Example MCP",
  "application_type": "web",
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "client_secret_basic"
}
```

`token_endpoint_auth_method` is the one selected method from section 3.2.
`client_name` is the administrator-supplied service `display_name`, not a name from the protected resource or authorization server.

### 4.2 Response validation

A successful response is `201` with JSON.

| Response member | Rule |
|---|---|
| `client_id` | Required and non-empty. |
| `token_endpoint_auth_method` | If present, identical to the requested method. |
| `client_secret` | Required for `client_secret_basic` and `client_secret_post`. Must be absent or empty for `none`. |
| `redirect_uris` | If present, must contain the exact broker callback. |
| `grant_types` | If present, must contain `authorization_code`. A missing `refresh_token` grant is permitted, but renewal requires a provider-issued refresh token. |
| `client_secret_expires_at` | Absent or zero is permitted. Reject any non-zero value, including a future expiry, with `client_registration_invalid`. Do not retry another client method. |
| `registration_access_token`, `registration_client_uri` | Discarded. They are not stored or logged. |

A `4xx` registration error fails with `client_registration_rejected`. This includes a request that needs an initial access token. The broker does not parse or return the provider error body.

A registration without the refresh grant can support account connection but does not promise renewal. A response with a non-zero secret expiry fails registration and leaves no local service or credential. The broker never re-registers a client or changes methods after a failed registration or missing refresh token.

## 5. Authorization and token requests

Every discovery-backed authorization request contains:

- The stored `client_id`.
- `response_type=code`.
- The existing service-specific callback.
- PKCE `code_challenge_method=S256`.
- Exactly one `resource` parameter that equals the persisted `authorization_params.resource`.

Every code exchange and refresh request contains exactly one `resource` form parameter with the same value. The token endpoint URL contains no `resource` query parameter. A provider error returns a failed connection or refresh. The broker does not retry without `resource`, try a broader resource, or store a token from that failed request.

If the provider issues no refresh token, renewal fails. If it rejects the stored credential, connection or renewal fails. The broker does not re-register, downgrade authentication, or retry without `resource`.

| Client method | Code exchange authentication | Refresh authentication |
|---|---|---|
| Hosted CIMD | `client_id` plus one fresh ES256 `client_assertion` pair. No secret or `Authorization` header. | Same as code exchange. |
| DCR `client_secret_basic` | HTTP Basic only. No body `client_secret`. | HTTP Basic only. No body `client_secret`. |
| DCR `client_secret_post` | Body `client_id` and `client_secret`. No `Authorization` header. | Body `client_id` and `client_secret`. No `Authorization` header. |
| DCR `none` | Body `client_id` and PKCE verifier. No secret or `Authorization` header. | Body `client_id`. No secret or `Authorization` header. |

The broker uses the service ID to load the stored client. It confirms that the stored client method and issuer match the service before it sends a token request.

## 6. Status and audit effects

| Outcome | Persisted effect |
|---|---|
| Successful create | Service, active discovery state, credential, `last_attempt_at`, and `last_success_at` commit together. |
| Failed create | No local service or credential remains. |
| Successful update | Active state and both timestamps commit together. |
| Failed update after discovery begins | Only `last_attempt_at` and `failure_reason` change. |
| Connection or renewal error | Existing session failure behavior applies. Discovery status does not change. |

Each discovery, registration, rejection, and refresh outcome produces one structured audit record. It has the service ID, operation, outcome, issuer when known, client method when selected, and a safe failure code when present.
