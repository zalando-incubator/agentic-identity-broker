---
title: "Configure authentication"
description: Configure the trusted reverse proxy that authenticates users and sends the principal header. You can also validate a signed JWT and extract a user profile.
---

# Configure authentication

The broker does not authenticate users. A trusted reverse proxy does this work. The proxy
can be oauth2-proxy, an nginx `auth_request`, an API gateway, or a service mesh. The proxy
authenticates each request with your identity provider. It sends the user identity in a
request header. The broker uses that value as the **principal**. It accepts the value only
from a source that you control.

This page explains the proxy trust boundary for both server ports. It also describes an
optional JWT mode that validates a signed token and extracts a user profile. See
[architecture](../concepts/architecture.md) for the system context.

## What you need

- A reverse proxy or gateway that authenticates users and can add or remove request headers.
  Continue to use Keycloak, Okta, Auth0, or your internal OIDC provider for human login.
- Network access to put the proxy in front of the broker end-user port 8000 and admin port
  14000.
- Access to the broker YAML configuration. See the
  [configuration reference](../configuration.md).
- For JWT pre-authentication, a JWKS endpoint for proxy-signed tokens. A trusted mesh can
  instead inject unsigned claims.

## Reverse-proxy pre-authentication

This is the baseline mode for every deployment. The proxy sets a header that contains a
principal identifier, such as an email, username, or opaque ID. The broker uses this value
as the authenticated user for the request.

### Set the principal header

Configure `authentication.preauth.principal_header_name` for each server port. Set it in
both the `enduser` and `admin` blocks. The default value is `X-Remote-User`.

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

The admin API on port 14000 accepts the same principal header. The broker does not identify
administrators. The proxy enforces administrator privilege before requests reach the admin
port. Restrict the admin route with your proxy access controls. You can use group
membership, an allowlist, a separate authentication policy, or a dedicated ingress. Keep
the admin port internal. Do not expose it with the public end-user port. See the
[API reference](../reference/api.md) for the admin surface.

## JWT pre-authentication (optional)

If your proxy or mesh sends a JWT, you can enable JWT pre-authentication. The broker reads
the token from a header. It can validate the signature. It then extracts the principal and
a profile with CEL. The profile can contain a display name, email address, and picture. The
consent interface can show this profile instead of a bare ID.

Configure this mode in `server.enduser.authentication.jwt`. Configure
`server.admin.authentication.jwt` when the admin proxy also sends JWTs. Keep the `preauth`
block. The server still requires `principal_header_name`. With `jwt` enabled, a missing
`header_name` returns `401 Unauthorized`. The broker does not use the plain header instead.

### Use a JWKS

Use this mode when the proxy sends signed JWTs. Set `verification: jwks`. This is the
default when `jwks_uri` is present. Set the expected audience and issuer. The broker then
rejects tokens that were issued for other services.

```yaml
server:
  enduser:
    port: 8000
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"   # still configured, but not used as a runtime fallback
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

### Use a trusted mesh without signature validation (`verification: none`)

Use this mode only in a network where you control JWT injection. For example, use a
mutual-TLS service mesh such as Istio or Linkerd. You can also use a trusted sidecar that
already authenticated the user. With `verification: none`, the broker reads claims without
validating a signature.

```yaml
server:
  enduser:
    port: 8000
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"
      jwt:
        header_name: "X-JWT-Claims"
        verification: "none"
        claim_extraction:
          principal_expression: "claims.sub"
          display_name_expression: "claims.preferred_username"
          email_expression: "claims.email"
```

Do not set `jwks_uri` with `verification: none`. These settings are mutually exclusive. The
broker does not start when both settings are present.

### Extract the principal and profile with CEL

Each `claim_extraction` value is a CEL expression for the token `claims` object.
`principal_expression` is required. It must return the principal string. The optional profile
expressions populate values for the consent interface:

| Expression | Populates | Typical claim |
|---|---|---|
| `principal_expression` | The principal identifier (required) | `claims.sub` |
| `display_name_expression` | The user's display name | `claims.name` or `claims.preferred_username` |
| `email_expression` | The user's email | `claims.email` |
| `picture_url_expression` | The user's avatar URL | `claims.picture` |

Adjust each expression to match the claim names your identity provider emits.

## Troubleshooting

- **Every request returns 401.** The principal is missing or empty. Make sure that the proxy
  sends the configured header to the broker. Make sure that the proxy header name exactly
  matches `principal_header_name`. With JWT pre-authentication, make sure that the token is
  in `header_name` and `principal_expression` returns a non-empty value.
- **The wrong user is authenticated, or outside clients authenticate.** The broker accepts
  the header from an untrusted source. Make sure that only the proxy can reach broker ports.
  Make sure that the proxy removes each client-supplied copy before it adds its own value.
- **The broker does not start with a JWT error.** `verification: none` and `jwks_uri` are
  both set. Remove `jwks_uri` for an unsigned mesh. Or set `verification: jwks` to validate
  the JWT.
- **The consent interface shows a bare ID.** A profile expression did not return a claim.
  Make sure that the JWT contains the claim. Make sure that
  `display_name_expression`, `email_expression`, and `picture_url_expression` use the
  correct claim names.

## Related

- [Architecture](../concepts/architecture.md) — where the proxy sits and how a request flows.
- [Configuration reference](../configuration.md) — every configuration key in detail.
- [API reference](../reference/api.md) — the endpoints protected by this authentication.
