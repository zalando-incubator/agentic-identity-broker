# End-User API

The end-user API serves OAuth2 authorization, token, and metadata requests. The broker serves it on the end-user listener.

## CIMD Client Authentication

In local or hybrid mode with CIMD enabled, a pre-registered CIMD client can authenticate at the token endpoint with `private_key_jwt`. The client signs a short-lived assertion with a key from its metadata document. The broker requires a fresh assertion for each authorization-code exchange and refresh request.

The assertion uses `RS256` or `ES256`, and its algorithm must match the client's metadata document. It must include `kid`, `jti`, `iss`, `sub`, `aud`, and `exp`. Both `iss` and `sub` must equal the CIMD URL. The `jti` is single-use per client.

For an authorization-code exchange, send the assertion fields with the existing grant fields:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=authorization_code
&client_id=https%3A%2F%2Fchatgpt.com%2Foauth%2FEBkIgu5McFbd%2Fclient.json
&code=AUTHORIZATION_CODE
&redirect_uri=https%3A%2F%2Fchatgpt.com%2Fconnector%2Foauth%2FEBkIgu5McFbd
&code_verifier=PKCE_VERIFIER
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=SIGNED_CLIENT_ASSERTION
```

For refresh, use `grant_type=refresh_token`, `refresh_token`, `client_id`, `client_assertion_type`, and a new `client_assertion`. Do not send `client_secret` or an `Authorization` header with an assertion.

Malformed parameter combinations return `400 invalid_request`. Authentication failures return `401 invalid_client` with a stable description. A document or key-set dependency failure returns `server_error`. A failed request does not consume an authorization code or rotate a refresh token.

See the [token endpoint contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/051-private-key-jwt-auth/contracts/token-endpoint.md) for the complete assertion profile and response rules.

## Incremental Service Authorization

API version 1.2.0 adds Virtual Service Scopes, `<public URL>/sessions/<canonical-id-or-UUID>`. The [incremental authorization guide](../guides/incremental-authorization.md) describes the flows, and [`api/enduser/openapi.yaml`](../../api/enduser/openapi.yaml) is the authoritative contract.

- `GET /oauth2/authorize` accepts virtual scopes. They are exempt from the agent's `allowed_scopes` and make each named service required for that request. Malformed, unknown or ineligible service scopes redirect with `invalid_scope`. An unsatisfied service redirects to consent; unverifiable usability redirects with `temporarily_unavailable`. After Save, the server-built continuation carries the same `session_token`, which the endpoint revalidates before issuing a code or redirecting upstream.
- `GET /api/consent/agents/{agent-id}` always returns `requested_service_ids` and `requested_service_coverage`. Coverage lists the service's required scopes, baseline usability and every covering permission set with its supplied required scopes and usability (`usable`, `missing`, `unusable` or `unavailable`).
- `POST /api/consent/agents/{agent-id}/grants` returns `400 requested service not satisfied` without writing the grant when a requested service is not selected, lacks required-scope coverage or has no usable session.
- `POST /oauth2/token`: recoverable token-exchange errors include `scope`. Local responses and JWT `scope` claims include granted virtual scopes. Refresh never widens: adding a scope returns `invalid_scope`, a bound service that is no longer satisfied returns `invalid_scope`, and unverifiable usability returns `server_error`. None of these rotate the refresh token. Proxy requests have virtual scopes removed.
- `GET /api/third-party/sessions/{service}` returns the service, the caller's session (or `null`) and the caller's dependent agents for a canonical ID or UUID, and `404` for an unknown service.
