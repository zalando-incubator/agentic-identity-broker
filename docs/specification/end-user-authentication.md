---
title: "End-user authentication"
description: Living specification of how the broker learns who the user is, from a header or a JWT that a trusted proxy sets on each request. Every example on this page is checked against the acceptance suite.
---

# End-user authentication

The broker does not log users in. A trusted proxy in front of the broker authenticates the user
and tells the broker who the user is on every request. The broker calls that identity the
**Principal**. This page describes the two ways a proxy can pass the Principal, what the broker
checks, and what the user sees.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

A platform operator runs the broker behind a proxy. The proxy authenticates each person and
forwards the request with the person's identity. Alice (`alice@example.com`) and a second user,
`user@example.com`, open the broker's pages to manage what their agents can do.
`admin@example.com` uses the admin server.

The operator chooses one of two methods for each server:

- **Plain header.** The proxy writes the Principal into a header that the operator names.
- **JWT.** The proxy puts a JWT into a header. The broker checks the JWT and computes the
  Principal from its claims.

The broker has no default for either method. A configuration that names neither a principal
header nor a JWT block does not load. `example:end-user-authentication.configuration.method-required`

## The Principal from a plain header

The operator names the header:

```yaml
# Configuration, not a checked exchange.
server:
  enduser:
    authentication:
      preauth:
        principal_header_name: X-Remote-User
```

The proxy authenticates Alice and sends `X-Remote-User: alice@example.com`. The broker takes the
value as the Principal of the request. `example:end-user-authentication.header.principal`

The user's profile endpoint shows the result. With a plain header, the broker knows only the
Principal, so the display name is the Principal and there is no email or picture: `example:end-user-authentication.profile.plain-header`

```http example="end-user-authentication.profile.plain-header"
GET /api/me HTTP/1.1
X-Remote-User: user@example.com

HTTP/1.1 200 OK
Content-Type: application/json

{
  "data": {
    "principal": "user@example.com",
    "displayName": "user@example.com"
  }
}
```

The broker applies a few rules to the header value:

- It removes leading and trailing spaces and tabs. Spaces inside the value stay. `example:end-user-authentication.header.trimmed`
- A protected route answers `401 Unauthorized` when the header is missing, empty, or only
  white space. `example:end-user-authentication.header.rejected`
- A protected route answers `400 Bad Request` when the value is longer than 200 characters. `example:end-user-authentication.header.rejected`
- Only the configured header counts. When the operator names `X-Custom-User`, a request that
  carries `X-Remote-User` or `Authorization` has no Principal. `example:end-user-authentication.header.configured-name`

A route that does not require a user answers with or without a Principal. `example:end-user-authentication.header.principal`

The broker trusts the header completely. The deployment must make sure that only the proxy can
reach the broker and that the proxy removes any copy of the header that a client sends.

## The Principal from a JWT

A header that anyone on the network path could write is weak evidence. The operator can instead
have the proxy send a signed JWT and give the broker the proxy's public keys:

```yaml
# Configuration, not a checked exchange.
server:
  enduser:
    authentication:
      jwt:
        header_name: Authorization
        verification: jwks
        jwks_uri: https://auth.example.com/.well-known/jwks.json
        expected_issuer: https://auth.example.com
        expected_audience: agentic-identity-broker
        claim_extraction:
          principal_expression: claims.sub
          display_name_expression: claims.name
          email_expression: claims.email
          picture_url_expression: claims.picture
```

The four expressions are **CEL expressions** over the claims of the JWT. The first one yields the
Principal. The other three are optional and yield the display name, the email address, and the
picture URL.

The proxy sends `Authorization: Bearer …` with a JWT that it signed. The JWT has the subject
`user@example.com`, the name `Jane Doe`, the email `jane@example.com`, and a picture. The broker
verifies the signature and returns all four values: `example:end-user-authentication.profile.from-claims`

```http example="end-user-authentication.profile.from-claims"
GET /api/me HTTP/1.1

HTTP/1.1 200 OK
Content-Type: application/json

{
  "data": {
    "principal": "user@example.com",
    "displayName": "Jane Doe",
    "email": "jane@example.com",
    "pictureUrl": "https://example.com/jane.jpg"
  }
}
```

The evidence does not record the `Authorization` header, so the exchanges in this section do not
show it.

When the JWT has no name, email, or picture claim, the request still succeeds. The display name
falls back to the Principal, and the response leaves out `email` and `pictureUrl`. `example:end-user-authentication.profile.missing-claims`

The keys can also come from an inline key set or a key-set file instead of a URL. `example:end-user-authentication.jwt.local-key-set`
The page "Trust anchors" describes the key sources.

### What the broker checks

The broker rejects a JWT with `401 Unauthorized` and a short reason when a check fails. The first
check is the signature. A JWT signed with a key that is not in the key set is refused: `example:end-user-authentication.jwt.bad-signature`

```http example="end-user-authentication.jwt.bad-signature"
GET /api/me HTTP/1.1

HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": "invalid JWT signature"
}
```

A JWT with no signature at all gets the same answer. `example:end-user-authentication.jwt.unsigned-rejected`

The other checks give these answers:

| The JWT | Answer | Proof |
|---|---|---|
| has an `exp` in the past | `401`, `JWT token expired` | `example:end-user-authentication.jwt.expired` |
| has an `aud` without the expected audience | `401`, `JWT audience mismatch` | `example:end-user-authentication.jwt.wrong-audience` |
| has an `iss` other than the expected issuer | `401`, `JWT issuer mismatch` | `example:end-user-authentication.jwt.wrong-issuer` |
| is missing from the request | `401`, `missing or empty principal` | `example:end-user-authentication.jwt.missing` |

A JWT that passes every check authenticates the request as the Principal that the expression
yields. `example:end-user-authentication.jwt.signed`

### No fallback to the plain header

A server can have both a JWT block and a principal header name. The JWT then decides alone. When
a request carries a valid JWT for `jwt-user@example.com` and also `X-Remote-User:
plain-header-user@example.com`, the Principal is `jwt-user@example.com`. `example:end-user-authentication.jwt.precedence`

When the JWT is absent, the broker does not read the plain header. It refuses the request: `example:end-user-authentication.jwt.no-fallback-absent`

```http example="end-user-authentication.jwt.no-fallback-absent"
GET /api/me HTTP/1.1
X-Remote-User: plain-user@example.com

HTTP/1.1 401 Unauthorized
Content-Type: application/json

{
  "error": "missing or empty principal"
}
```

The same holds when the JWT is present but not valid. A request with an expired JWT and a plain
header that names a user is refused with `401` and `JWT token expired`. `example:end-user-authentication.jwt.no-fallback-invalid`

The reason is that the broker must fail closed. If a weaker method took over whenever the
stronger one failed, an attacker would only have to drop or break the JWT.

### A JWT that the broker does not verify

Some service meshes authenticate the user and pass the claims as a JWT without a signature. The
operator can accept this, but only by saying so:

```yaml
# Configuration, not a checked exchange.
server:
  enduser:
    authentication:
      jwt:
        header_name: X-JWT-Claims
        verification: none
        claim_extraction:
          principal_expression: claims.sub
          display_name_expression: claims.preferred_username
          email_expression: claims.email
```

The mesh sends an unsigned JWT with the subject `user@example.com`, the preferred user name
`testuser`, and an email claim. The broker reads the claims without a signature check: `example:end-user-authentication.jwt.unverified`

```http example="end-user-authentication.jwt.unverified"
GET /api/me HTTP/1.1
X-JWT-Claims: …

HTTP/1.1 200 OK
Content-Type: application/json

{
  "data": {
    "principal": "user@example.com",
    "displayName": "testuser",
    "email": "user@example.com"
  }
}
```

Expiry still counts. An unsigned JWT with an `exp` in the past is refused with `401` and
`JWT token expired`. `example:end-user-authentication.jwt.unverified-expired`

`verification: none` and a key source contradict each other. The broker does not start when a
JWT block has both `verification: none` and a `jwks_uri`. `example:end-user-authentication.jwt.verification-none-with-jwks-uri`

With `verification: none`, the broker trusts the header as completely as it trusts a plain
header. The same deployment duty applies.

## Two servers, two configurations

The end-user server and the admin server each have their own `authentication` block. They can
name different principal headers, for example `X-User-Header` for users and `X-Admin-Header` for
administrators. `example:end-user-authentication.configuration.per-server`

The same is true for JWTs. When the admin server trusts the keys of an operator proxy and the
end-user server trusts the keys of the mesh, the admin server accepts a JWT that the operator
proxy signed and does not accept one that the mesh signed. `example:end-user-authentication.servers.separate-key-sets`

## The actor of a request

The Principal is also the **Actor** that the broker records for the request. The access log entry
of an authenticated `GET /api/me` carries the user's Principal as `actor`. `example:end-user-authentication.actor.recorded`
The page "Observability and audit" describes the record.

## Not yet on this page

- The header name is matched without regard to case, and the first value counts when the header
  occurs more than once. Both are specified. No acceptance or integration test proves them.
- The header of the web application shows the picture as an avatar, the display name, and the
  email address, and falls back to initials and the Principal. The browser suite has no scenario
  for it.
- Startup fails when a CEL expression is not valid or when the key-set URL cannot be reached.
  The acceptance suite does not cover these cases.
- A JWT without `exp` is refused with `verification: jwks` and accepted with `verification:
  none`. A signed JWT is accepted unchecked with `verification: none`. The tests cover the
  accepted cases; this page leaves them out.
- A profile expression that yields a value that is not text, and the behavior when the key-set
  URL stops answering while the broker runs.

## Sources

The behavior on this page was decided in `specs/005-session-management/spec.md` and
`specs/016-jwt-preauth/spec.md`, with key sources from
`specs/052-inline-jwks-trust-anchors/spec.md` and the actor from
`specs/033-request-security-context/spec.md`. The contract of `GET /api/me` is
`api/enduser/openapi.yaml`. The scenarios are in `tests/e2e/jwt_preauth_test.go`,
`tests/e2e/jwt_preauth_local_jwks_test.go`, `tests/e2e/request_security_context_test.go`,
`tests/integration/principal_middleware_test.go`, and `internal/config/loader_test.go`.
