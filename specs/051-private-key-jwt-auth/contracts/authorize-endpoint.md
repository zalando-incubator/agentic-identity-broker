# Contract: Authorisation Endpoint Delta

**Feature**: `051-private-key-jwt-auth` | **Date**: 2026-10-15
**Canonical source**: [`api/enduser/openapi.yaml`](../../../api/enduser/openapi.yaml), `GET /oauth2/authorize`
**Status**: **PROPOSED**. Requires stakeholder confirmation before implementation (API-003, Constitution Principle X).

This feature does not add parameters or endpoints here. It changes one error response of an existing endpoint, which is a visible API change, so it is documented and confirmed like the others.

## Summary of the delta

| Location | Change |
|---|---|
| `GET /oauth2/authorize`, error list in the description | Add: a URL-format (CIMD) `client_id` whose metadata document cannot be retrieved because of a timeout, connection failure or upstream server error returns `server_error` instead of `invalid_client`. |
| `500` response | Document the CIMD case alongside "Internal infrastructure failure", with an example. |
| `400` response | Unchanged. `invalid_client` remains for a CIMD document that is retrieved but fails validation (malformed, policy violation, blocked address, upstream 4xx) and for an unregistered client. |

## Behaviour

| Condition | Before | After |
|---|---|---|
| CIMD document retrieved but invalid, SSRF-blocked, or served with an upstream 4xx | 400 `invalid_client`, description "CIMD document validation failed" | unchanged |
| CIMD document cannot be fetched (timeout, connection failure, upstream 5xx) | 400 `invalid_client`, description "CIMD document validation failed" | **500 `server_error`**, JSON body, no redirect |
| CIMD document declares `private_key_jwt` with a valid key source | 400 `invalid_client` (rejected by document validation) | accepted; the authorisation flow continues as for any CIMD client |

Rules:
- The response is the endpoint's existing direct JSON error (the redirect URI is not yet verified, so no redirect is made, RFC 6749 §4.1.2.1). The only new case is the status and code for fetch failures.
- `error_description` for the new case does not include the target address or the upstream response.
- The same classification applies on `POST /oauth2/token` (see [token-endpoint.md](./token-endpoint.md)), because both endpoints resolve the client through the same step.

## Example (new case)

```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error":"server_error","error_description":"client metadata document unavailable"}
```

## Compatibility

Clients and tests that treated a CIMD fetch failure as `invalid_client` on this endpoint must treat it as a temporary server error. The change makes outages retryable and distinguishable from a rejected client. It is called out in the PR description (plan Risk R3).
