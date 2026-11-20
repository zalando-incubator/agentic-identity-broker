---
title: "Trust anchors"
description: Living specification of how a platform operator supplies the public keys that the broker trusts, from a URL, inline, or a file, and what happens when those keys change or are wrong. Every example on this page is checked against the acceptance suite.
---

# Trust anchors

The broker accepts a signed token only when it can verify the signature with a public key that
the operator told it to trust. A **trust anchor** is one such authority together with its keys.
This page describes where a trust anchor's keys come from, how they change while the broker runs,
and what the broker does when the key configuration is wrong. The flows that use the keys are
described on other pages.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

A platform operator runs the broker and an **agent gateway** in front of the agents. The gateway
authenticates to the broker with a ClientAssertion: a JWT that it signs with a private key that it
holds locally. Its current key has the identifier `agentgateway-2026-10`. No web server publishes
the matching public key.

The broker already knows one agent, the *Research Assistant*, and the provider *GitHub*. The user
`user@example.com` has granted the agent access to GitHub and has a GitHub session.

The broker has a trust anchor in three places:

| Trust anchor | Configuration path | Verifies |
|---|---|---|
| Token-exchange client assertion | `token_exchange.client_assertion` | The gateway's ClientAssertion in token exchange and on machine-facing approval requests |
| Impersonation trusted issuer | `oauth2_authorization_server.impersonation.rules[].trusted_issuers[]` | The signed credentials of an impersonation request |
| JWT pre-authentication | `server.enduser.authentication.jwt` and `server.admin.authentication.jwt` | The identity JWT that a proxy or service mesh puts on each request |

Each trust anchor has one **Key Set Source**: `remote` (a `jwks_uri`, or a location that the
broker discovers from the issuer), `inline` (`jwks`), or `file` (`jwks_file`).

## Keys written into the configuration

The operator has the gateway's public key and nothing else. The operator writes it into the
broker configuration as a JWK Set. This is the `inline` Key Set Source.

```yaml
# Configuration, not a checked exchange.
token_exchange:
  client_assertion:
    issuer_uri: "https://gateway.example.com"
    jwks: |
      {"keys":[{"kty":"EC","crv":"P-256","kid":"agentgateway-2026-10","alg":"ES256","use":"sig","x":"<base64url x>","y":"<base64url y>"}]}
```

The broker starts and verifies a ClientAssertion without sending any request to the issuer's
host. `example:trust-anchors.inline.no-outbound-request` The issuer does not have to publish
metadata or keys.

A token exchange whose ClientAssertion the gateway signed with that key succeeds: `example:trust-anchors.inline.accept`

```http example="trust-anchors.inline.accept"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 200 OK
Cache-Control: no-store

{
  "access_token": "github-token-xyz",
  "token_type": "Bearer",
  "principal": "user@example.com",
  "agent_id": "550e8400-e29b-41d4-a716-446655440000",
  "…": "…"
}
```

The same request with a ClientAssertion that another key signed gives no token. The other key
has the identifier `rogue-2026-10`, and the inline set does not contain it: `example:trust-anchors.inline.reject-unknown-key`

```http example="trust-anchors.inline.reject-unknown-key"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 401 Unauthorized
Cache-Control: no-store

{
  "error": "invalid_client",
  "error_description": "client authentication failed"
}
```

The same trust anchor authenticates the gateway when it creates a tool approval with the header
`X-Client-Assertion`. An assertion signed by the inline key is accepted there too. `example:trust-anchors.inline.approval-authentication`

### The inline keys serve this trust anchor only

In hybrid mode the operator can leave out `issuer_uri` and give only the inline key set. The
broker then expects the upstream issuer in the ClientAssertion, and it keeps two sets of keys
apart: `example:trust-anchors.inline.upstream-issuer`

- A ClientAssertion signed by the gateway's inline key is accepted.
- A ClientAssertion signed by the upstream issuer's own key is refused with `invalid_client`.
- A subject token signed by the gateway's key is refused. Subject tokens still verify against
  the upstream issuer's keys.

### The source does not change the decision

Only the origin of the public keys differs between the sources. The acceptance suite sends the
same ClientAssertion to three brokers: one with a `jwks_uri`, one with `jwks`, and one with
`jwks_file`. A valid assertion gets `200 OK` from all three, and a wrongly signed one gets the
same `401 Unauthorized` with `invalid_client` from all three. `example:trust-anchors.source.parity`

## Keys in a file

An inline key set is fixed until the broker restarts. An operator who rotates the gateway's key
keeps the JWK Set in a file instead, for example a file that the cluster mounts from a Secret.
This is the `file` Key Set Source. The broker reads key-set files from one place only, the
**Trust Directory**.

```yaml
# Configuration, not a checked exchange.
security:
  trust_material_dir: "/etc/agentic-identity-broker/trust"

token_exchange:
  client_assertion:
    issuer_uri: "https://gateway.example.com"
    jwks_file: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"
    jwks_min_refresh: "15m"
    jwks_max_refresh: "1h"
```

The broker starts, accepts a ClientAssertion signed by a key in the file, and refuses one signed
by a key that the file does not contain. `example:trust-anchors.file.verify` A Kubernetes volume
does not hold the file itself. It holds a relative symbolic link through `..data` to a directory
with a timestamp in its name. The broker follows that chain of links. `example:trust-anchors.file.kubernetes-layout`

### A replaced file takes effect without a restart

The gateway moves to a new key, `agentgateway-2027-01`. The operator updates the Secret, and the
cluster swaps the link to a new directory. The broker re-reads the file on its own. Soon after,
an assertion signed by the new key is accepted, and an assertion signed by the old key gets
`401 Unauthorized`, because the new file no longer lists the old key. The broker logs
`trust anchor key set changed`. `example:trust-anchors.file.rotation`

`jwks_min_refresh` sets how often the broker re-reads the file. The scenario uses 200
milliseconds so that the change is visible within three seconds.

### A bad file does not replace a good one

Suppose a deployment writes a truncated document into the file. The next re-read fails. The
broker keeps the last valid key set, so the gateway's token exchange still succeeds. It reports
the problem in the `components` of `GET /health` on the end-user server: `example:trust-anchors.file.failed-re-read`

```http example="trust-anchors.file.failed-re-read"
GET /health HTTP/1.1

HTTP/1.1 200 OK

{
  "status": "healthy",
  "components": {
    "trust_anchor_key_files": "degraded",
    "upstream_jwks": "healthy"
  },
  "…": "…"
}
```

The HTTP status and the top-level `status` do not change. An operator who wants an alert must
watch `components`.

The broker also logs `trust anchor key set re-read failed`. The record names the trust anchor
(`token_exchange.client_assertion`), the source (`file`), the kind of error (`not_a_jwk_set`),
the time of the last successful read, and the time at which verification will stop. It does not
contain the file's contents.

The broker does not trust an old key set for ever. `jwks_max_refresh` is the longest time that
it uses the last valid set after the last successful read. The scenario sets two seconds. After
that, the trust anchor fails closed. The broker can no longer verify the gateway, so it answers
with a server error, not with a token:

```http example="trust-anchors.file.failed-re-read"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fapi.github.com

HTTP/1.1 500 Internal Server Error
Cache-Control: no-store

{
  "error": "server_error",
  "error_description": "internal server error during token exchange"
}
```

The broker logs `trust anchor key set expired` once, and health stays `degraded`. When the
operator puts a valid file back, the next re-read applies it. The broker logs
`trust anchor key set recovered`, the gateway's exchange succeeds again, and
`trust_anchor_key_files` returns to `healthy`.

### The Trust Directory is a hard boundary

A file path in the configuration could otherwise make the broker read any file that its process
can read, such as a credential. The broker therefore refuses to start in each of these cases:

- `jwks_file` is set and `security.trust_material_dir` is not. The error is
  `token_exchange.client_assertion: jwks_file requires security.trust_material_dir to be set`. `example:trust-anchors.trust-directory.required`
- The path leaves the Trust Directory through `..` segments, or through a relative symbolic link
  that points outside. The error names the path and the Trust Directory, for example
  `jwks_file "…" is outside security.trust_material_dir "…"`. `example:trust-anchors.trust-directory.escape`
- The path is an absolute symbolic link, even when its target lies inside the Trust Directory.
  The broker follows relative links only. `example:trust-anchors.trust-directory.escape`
- The path does not exist (`jwks_file "…" does not exist`), or it is a directory
  (`jwks_file "…" is not a regular file`). `example:trust-anchors.file.missing-path`

## A wrong key configuration stops startup

A key set is a trust decision. The broker does not start with a trust anchor that is ambiguous
or only partly valid. Every error starts with the configuration path of the trust anchor, so the
operator knows which setting to correct.

**One source for each trust anchor.** There is no order of precedence between the sources. A
trust anchor that sets `jwks_uri` and `jwks` together stops startup with
`token_exchange.client_assertion: only one of jwks_uri, jwks, jwks_file may be set (found: jwks_uri, jwks)`. `example:trust-anchors.startup.one-source`

**A valid Local Key Set.** A **Local Key Set** is the JWK Set from an inline or file source. The
broker checks it before it serves any request. Each of these documents stops startup:

| The document | Error after `token_exchange.client_assertion.jwks: ` | Proof |
|---|---|---|
| Is one JWK, not a JWK Set | `key set is not a valid JWK Set document` | `example:trust-anchors.startup.malformed-key-set` |
| Has no keys | `key set contains no keys` | `example:trust-anchors.startup.malformed-key-set` |
| Is larger than 1 MiB | `key set exceeds 1048576 bytes` | `example:trust-anchors.startup.malformed-key-set` |
| Has a key with no `kid` or an empty `kid` | `key at index 0 requires a non-empty kid` | `example:trust-anchors.startup.malformed-key-set` |
| Uses one `kid` for two keys | `key identifier "dup-1" appears more than once` | `example:trust-anchors.startup.duplicate-kid` |
| Contains a private key | `key set contains private key material; only public keys are allowed` | `example:trust-anchors.startup.private-key` |
| Contains a symmetric key | `key set contains a symmetric key; only public asymmetric keys are allowed` | `example:trust-anchors.startup.symmetric-key` |
| Declares `HS256` as a key's `alg` | `key "agentgateway-2026-10" declares an algorithm outside the approved asymmetric set` | `example:trust-anchors.startup.symmetric-key` |

The same rules apply to a file. Its errors start with the path of the `jwks_file` setting.

An operator who pastes a private key by mistake must not find that key in an error message or
a log. Neither the error nor the logs contain any coordinate, modulus, or private value of the
rejected key. `example:trust-anchors.startup.private-key`

**No refresh bounds on an inline key set.** `jwks_min_refresh` or `jwks_max_refresh` next to
`jwks` stops startup, because an inline key set never refreshes. `example:trust-anchors.startup.inline-refresh-bounds`

**Never the broker's own issuer.** A client-assertion `issuer_uri` that equals the broker's own
issuer stops startup with `must be an external identity provider, not the broker's own issuer`.
An inline or file key set does not lift that rule. `example:trust-anchors.startup.own-issuer`

When the key set is valid, the broker logs `trust anchor key set loaded` once for the trust
anchor, with the source (`inline`), the number of keys, and the key identifiers. The log does not
contain the JWK Set or any key material. `example:trust-anchors.startup.loaded-log`

## The same sources for the other trust anchors

### Impersonation trusted issuers

An impersonation rule names the issuers that it trusts. Each trusted issuer is a trust anchor of
its own and takes `jwks` or `jwks_file` in the same way:

```yaml
# Configuration, not a checked exchange.
oauth2_authorization_server:
  impersonation:
    rules:
      - name: "agentgateway"
        trusted_issuers:
          - issuer_uri: "https://gateway.example.com"
            jwks_file: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"
            allowed_algorithms: ["ES256"]
            signs_roles: ["client_assertion", "actor"]
```

With an inline key set, the broker starts without contacting the issuer and issues an
impersonation token for credentials that the gateway's key signed. `example:trust-anchors.impersonation.inline`
With a key-set file, the result is the same. `example:trust-anchors.impersonation.file`

Two rules can trust the same issuer identifier with different inline keys. Each rule then
verifies only against its own keys. A credential signed with the key of rule B is accepted when
rule B authorizes the request. It is refused when only rule A would authorize the request,
because rule A does not hold that key. `example:trust-anchors.impersonation.per-rule-keys`

### JWT pre-authentication

A service mesh in front of the broker puts a signed identity JWT on each request. The operator
gives the mesh's public keys to the end-user server:

```yaml
# Configuration, not a checked exchange.
server:
  enduser:
    authentication:
      jwt:
        verification: jwks
        jwks_file: "/etc/agentic-identity-broker/trust/mesh-jwks.json"
```

A request whose JWT the mesh signed for the subject `user@example.com` is authenticated as that
Principal. This holds for an inline key set and for a key-set file. `example:trust-anchors.jwt.local-key-set`

```http example="trust-anchors.jwt.local-key-set"
GET /api/me HTTP/1.1

HTTP/1.1 200 OK

{
  "data": {
    "principal": "user@example.com",
    "displayName": "user@example.com"
  }
}
```

The JWT travels in the `Authorization` header, which the evidence does not record. A JWT that an
unknown key signed is refused:

```http example="trust-anchors.jwt.local-key-set"
GET /api/me HTTP/1.1

HTTP/1.1 401 Unauthorized

{
  "error": "invalid JWT signature"
}
```

The admin server is a separate trust anchor with its own key set. When the admin server trusts
the key of an operator proxy and the end-user server trusts the mesh, an admin request with a JWT
from the operator proxy succeeds, and an admin request with a JWT from the mesh does not. `example:trust-anchors.jwt.per-server`

`verification: none` means that the broker does not check signatures. A key set next to it would
give a false sense of safety, so `jwks` or `jwks_file` with `verification: none` stops startup:
`server.enduser.authentication.jwt: verification 'none' and jwks are mutually exclusive`. `example:trust-anchors.jwt.verification-none`

## Not yet on this page

- The `remote` source appears only in the comparison of the three sources. Discovery of the key
  location from issuer metadata, reuse of the upstream issuer's keys when a trust anchor declares
  no source, and the refresh of remote key sets are not described here.
- The default refresh bounds of a key-set file, and the fixed bounds of JWT pre-authentication,
  are documented in `docs/configuration.md`. No acceptance scenario on this page proves them.
- A symbolic link that is retargeted to a file outside the Trust Directory after startup, an
  unreadable file, a relative `jwks_file` path, and a Trust Directory that is `/` are covered by
  tests below the acceptance level only.
- The inline key set from an environment variable, the allowed-algorithm list of a trusted
  issuer, and JWT pre-authentication with `verification: jwks` and no key source are specified
  and tested, but not described here.
- The Helm chart can mount a Secret or ConfigMap into the Trust Directory. That is a deployment
  topic.
- Token exchange, impersonation, and end-user authentication themselves are described on their
  own pages.

## Sources

The behavior on this page was decided in `specs/052-inline-jwks-trust-anchors/spec.md` and
`adrs/040-trust-anchor-local-key-sets.md`. The places that use a trust anchor come from
`specs/013-token-exchange/spec.md`, `specs/016-jwt-preauth/spec.md`,
`specs/037-oauth2-user-impersonation/spec.md`, and
`adrs/029-token-exchange-client-assertion-trust-anchor.md`, which has the status Proposed. The
settings are documented in `docs/configuration.md` and
`examples/config/trust-anchor-local-jwks.yaml`. The health component is part of
`api/enduser/openapi.yaml`. The scenarios are in `tests/e2e/trust_anchor_inline_jwks_test.go`,
`tests/e2e/trust_anchor_key_files_test.go`, `tests/e2e/trust_anchor_key_source_parity_test.go`,
`tests/e2e/impersonation_local_jwks_test.go`, and `tests/e2e/jwt_preauth_local_jwks_test.go`.
