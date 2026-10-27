# End-User API

The end-user API serves OAuth2 authorization, token, and metadata requests. The broker serves it on the end-user listener.

## End-User API 2.0.0: reconnect a third-party session

End-User API `2.0.0` changes the RFC 8693 recovery link. In `1.0.0`, a recoverable session error pointed to `/sessions`. Clients that open `error_uri` now start authorization for the resolved service. Other token exchange responses and the `/oauth2/token` path do not change.

For a broker with public URL `https://broker.example.com` and service UUID `550e8400-e29b-41d4-a716-446655440000`, a missing session returns:

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid_grant","error_description":"User session is unavailable. Please re-authenticate.","error_uri":"https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions"}
```

Open this URI in the authenticated user's browser. The GET starts third-party authorization with PKCE. The provider callback returns to `/sessions`. The `redirect_uri` value is the percent-encoded, fixed same-origin return URI. The broker uses its configured public URL and the resolved service UUID, not a client-supplied origin or service ID. Do not replace the link with broker `/oauth2/authorize` or a raw provider URL.

Missing, expired, scope-deficient, provider-rejected, and client-identity-mismatched sessions use this link. Grant errors keep their `/agents/{agent-id}` link. Other failures have no `error_uri`. The broker token endpoint sends no `WWW-Authenticate` for these errors. ExtProc forwards the broker URI unchanged in its HTTP 403 challenge and URL elicitation. See [the token exchange reference](../reference/token-exchange.md#400-invalid_grant) and [ADR 044](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/044-service-specific-session-recovery-url.md).

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
