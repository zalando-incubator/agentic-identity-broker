# Redirect URI Contract

This feature extends existing callback behavior. It adds no endpoint, request field, or response field.

## Agent Registration

The existing `POST /api/agents` and `PUT /api/agents/{agent-id}` operations accept exact callback URIs and valid HTTPS host-wildcard callback URIs in `redirect_uris`. The `GET /api/agents/{agent-id}` response returns the registered strings unchanged.

A wildcard registration uses `*` within one or more hostname labels. It does not match a dot. The scheme, port, path, query, and other URI components remain fixed. The registrable domain and public suffix must be literal. Invalid patterns produce the existing `400 validation failed` response.

The preview example in this feature is:

```text
Registered: https://preview*.app-*.example.com/api/oidc/callback
Matches:    https://preview-pr-123.app-test.example.com/api/oidc/callback
Matches:    https://preview.app-sandbox.example.com/api/oidc/callback
Does not match: https://preview-pr-123.app-test.example.net/api/oidc/callback
```

## OAuth2 Authorization

The existing `GET /oauth2/authorize` operation accepts a concrete `redirect_uri` when it matches an exact or wildcard registration on the opaque Agent. On success, the broker redirects to the concrete URI from the request and preserves its `state`; it never redirects to the registered pattern.

A non-matching callback returns the existing `invalid_redirect_uri` error without a `Location` redirect. Exact callback matching remains unchanged.

## Unchanged Callback Sources

- CIMD document `redirect_uris` remain exact-match-only.
- Private-use scheme callbacks remain byte-for-byte exact and continue to require Agent allowlisting where currently required.
- Loopback HTTP port matching remains unchanged.
- No production or deployment-specific callback registrations are changed by this broker contract.

The normative OpenAPI descriptions remain in `api/admin/openapi.yaml` and `api/enduser/openapi.yaml`.
