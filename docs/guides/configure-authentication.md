---
title: "Configure authentication"
description: Configure proxy pre-authentication with an explicit principal header or a signed JWT from a JWKS endpoint.
---

# Configure authentication

The broker accepts an identity from a trusted reverse proxy. The proxy authenticates users
with your identity provider. It sends either a principal header or a signed JWT to the
broker. The broker accepts this identity only from a source that you control.

This page explains the proxy trust boundary for both server ports. It also describes JWT
pre-authentication and optional profile extraction. See
[architecture](/docs/concepts/architecture) for the system context.

## What you need

- A reverse proxy or gateway that authenticates users and can add or remove request headers.
  Continue to use Keycloak, Okta, Auth0, or your internal OIDC provider for human login.
- Network access to put the proxy in front of the broker end-user port 8000 and admin port
  14000.
- Access to the broker YAML configuration. See the
  [configuration reference](/docs/configuration).
- For JWT pre-authentication, a JWKS endpoint for the signed tokens that your proxy sends.

## Reverse-proxy pre-authentication

Without JWT configuration, the proxy sets a header that contains a principal identifier,
such as an email, username, or opaque ID. The broker uses this value as the authenticated
user for the request.

### Set the principal header

Set `authentication.preauth.principal_header_name` explicitly for each server that uses
plain-header pre-authentication. There is no global default principal header.

```yaml
server:
  enduser:
    port: 8000
    bind: "0.0.0.0"
    public_url: "https://broker.example.com"
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"
  admin:
    port: 14000
    bind: "0.0.0.0"
    public_url: "https://broker.example.com:14000"
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"
```

If your proxy sends `X-Forwarded-User` or `X-Auth-User`, rename the header at the proxy.
You can instead set `principal_header_name` to the proxy header name. The names must match.

### Establish the trust boundary

The principal header asserts a user identity. It is safe only when a request cannot reach
the broker with a header that the proxy did not set. These rules establish that boundary:

- **Only the proxy can access the broker ports.** Bind the broker to an internal network.
  Route all traffic through the proxy. A direct client can assert any principal.
- **The proxy removes client-supplied headers.** Before the proxy adds its trusted value,
  remove inbound `X-Remote-User` or the configured header name. Otherwise, a caller can
  set the header and impersonate another user.

:::warning
Accept the principal header only from the proxy. Remove every external copy before the
proxy authenticates the request. Do not expose broker ports directly. A direct port or an
unremoved header defeats the delegation model.
:::

### Enforce admin privilege at the proxy

The broker does not identify administrators. The proxy enforces administrator privilege
before requests reach the admin port. This rule applies to both plain-header and JWT
pre-authentication. Restrict the admin route with proxy access controls, such as group
membership or an allowlist. Keep the admin port internal and separate from the public
end-user port. See the [API reference](/docs/reference/api) for the admin surface.

## JWT pre-authentication (optional)

If your proxy or mesh sends a signed JWT, enable JWT pre-authentication for that server.
The broker validates its signature against the configured JWKS endpoint. It requires an
`exp` claim. Then it extracts the principal and optional profile values with CEL.
The consent interface can show the profile instead of a bare ID.

Configure `server.enduser.authentication.jwt` for the end-user server. Configure
`server.admin.authentication.jwt` only if the admin proxy sends signed JWTs. Otherwise,
configure the admin server's explicit `preauth.principal_header_name` for plain-header mode.
When a server has JWT configuration, it ignores the plain principal header. Protected end-user routes
return `401 Unauthorized` for a missing or invalid JWT. The admin sweep returns
`500 server_misconfiguration` without an operator principal. The admin proxy must reject
unauthorized requests before they reach any admin route.

### Use a JWKS

Set `verification: jwks` and a `jwks_uri` for a signed JWT. The JWT must contain an `exp`
claim. Set the expected audience and issuer when the proxy provides these claims. The
broker rejects a token when a configured audience or issuer does not match.

```yaml
server:
  enduser:
    port: 8000
    authentication:
      jwt:
        header_name: "Authorization"             # strips the "Bearer " prefix
        verification: "jwks"
        jwks_uri: "https://auth.example.com/.well-known/jwks.json"
        expected_audience: "agentic-identity-broker"
        expected_issuer: "https://auth.example.com"
        claim_extraction:
          principal_expression: "claims.sub"
          display_name_expression: "claims.name"
          email_expression: "claims.email"
          picture_url_expression: "claims.picture"
```

The broker rejects `verification: none` at startup for either server. A trusted mesh does
not make unsigned JWT pre-authentication valid. If the proxy cannot send a signed JWT,
configure an explicit principal header instead. The proxy must remove client-supplied
copies before it sets that header. The unsigned impersonation subject exception in ADR 031
does not apply to pre-authentication.

### Extract the principal and profile with CEL

Each `claim_extraction` value is a CEL expression for the token `claims` object.
`principal_expression` defaults to `claims.sub`. It must return a non-empty principal
string. The optional profile expressions populate values for the consent interface:

| Expression | Populates | Typical claim |
|---|---|---|
| `principal_expression` | The principal identifier (required) | `claims.sub` |
| `display_name_expression` | The user's display name | `claims.name` or `claims.preferred_username` |
| `email_expression` | The user's email | `claims.email` |
| `picture_url_expression` | The user's avatar URL | `claims.picture` |

Adjust each expression to match the claim names your identity provider emits.

## Troubleshooting

- **Protected end-user requests return 401.** In plain-header mode, make sure that the proxy sends the
  configured principal header with a non-empty value. In JWT mode, make sure that the proxy
  sends a signed token in `header_name`. Make sure that it has an `exp` claim and that
  `principal_expression` returns a non-empty value. A plain header cannot replace a missing
  or invalid JWT.
- **The admin sweep returns `500 server_misconfiguration`.** The broker did not receive an operator
  principal. Confirm the proxy's configured admin header or signed JWT and its authorization policy.
  The broker does not run the sweep without an operator identity.
- **The wrong user is authenticated, or outside clients authenticate.** The broker accepts
  the header from an untrusted source. Make sure that only the proxy can reach broker ports.
  Make sure that the proxy removes each client-supplied copy before it adds its own value.
- **The broker does not start with a JWT error.** `verification: none` is not supported for
  pre-authentication on either server. Set `verification: jwks` and a `jwks_uri` for signed
  tokens. If the proxy cannot sign JWTs, remove the JWT block and configure an explicit
  `preauth.principal_header_name` instead.
- **The consent interface shows a bare ID.** A profile expression did not return a claim.
  Make sure that the JWT contains the claim. Make sure that
  `display_name_expression`, `email_expression`, and `picture_url_expression` use the
  correct claim names.

## Related

- [Architecture](/docs/concepts/architecture) — where the proxy sits and how a request flows.
- [Configuration reference](/docs/configuration) — every configuration key in detail.
- [API reference](/docs/reference/api) — the endpoints protected by this authentication.
