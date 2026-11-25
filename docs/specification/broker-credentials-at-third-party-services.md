---
title: "Broker credentials at a third-party service"
description: Living specification of how the broker proves its own identity to an external OAuth2 provider, with a stored secret, as a public client, with a hosted Client ID Metadata Document, or with credentials from mounted files. Every example on this page is checked against the acceptance suite.
---

# Broker credentials at a third-party service

When Alice connects her account at an external provider, the broker is the OAuth2 client of that
provider. The provider wants to know which client is asking. This page describes how an
administrator decides what the broker answers when it starts an authorization, exchanges a code,
or refreshes a token.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

An administrator, `admin@example.com`, registers each **ThirdpartyOAuth2Service** through the
admin API. Alice (`alice@example.com`) connects her account and later relies on the broker to
refresh her **UserSession**. A platform operator owns what the administrator cannot set through
the API: the key set that signs client assertions, and credential files mounted into the broker.

The exchanges below omit the header that carries the caller's identity. The provider in the
examples is a test double, so its host is `provider.example.test` or a local port.
What a provider is, its scopes, and its discovery belong to the page "Third-party service
registration". How agents authenticate to the broker belongs to "Client registration and
authentication".

## Three settings

A service carries three settings that together answer the provider's question.

| Setting | Values | What it decides |
|---|---|---|
| `client_registration` | `static`, `cimd` | Where the client ID comes from: the administrator, or a URL that the broker hosts |
| `token_endpoint_auth_method` | `null`, `none`, `private_key_jwt` | How the broker authenticates at the token endpoint. `null` means a client secret. |
| `credential_source` | `stored`, `filesystem` | Where a static client ID and secret live: in the broker's storage, or in mounted files |

An administrator who registers a service by hand can choose four pairs of the first two
settings. The sections below take them in turn.

| `client_registration` | `token_endpoint_auth_method` | The broker is |
|---|---|---|
| `static` | `null` | A confidential client with a client secret |
| `static` | `none` | A public client with PKCE and no secret |
| `cimd` | `private_key_jwt` | A confidential client that signs assertions and publishes a metadata document |
| `cimd` | `none` | A keyless public client that publishes a metadata document |

## A stored client secret

This is what the administrator gets by sending a client ID and a client secret and nothing else.
The broker never shows the secret again. The representation carries the word `REDACTED` in its
place: `example:broker-credentials-at-third-party-services.secret.create`

```http example="broker-credentials-at-third-party-services.secret.create"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "Public Client Test Service",
  "client_id": "public-client-id",
  "client_secret": "confidential-secret",
  "issuer_uri": "https://provider.example.test",
  "discovery": { "enable_discovery": false },
  "endpoints": {
    "authorize_endpoint": "https://provider.example.test/oauth/authorize",
    "token_endpoint": "https://provider.example.test/oauth/token"
  },
  "scopes": [{ "scope_value": "profile", "description": "Profile" }]
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "credential_source": "stored",
  "client_registration": "static",
  "client_id": "public-client-id",
  "client_secret": "REDACTED",
  "token_endpoint_auth_method": null,
  "…": "…"
}
```

The response names all three settings, although the request named none of them.

When Alice connects, the broker sends the client ID and secret to the token endpoint in an
`Authorization: Basic` header. When it refreshes her token, it sends them in the form body
instead. `example:broker-credentials-at-third-party-services.secret.connect-and-refresh`

## A public client: PKCE and no secret

Some providers register the broker as a public client and issue no secret. The administrator
declares this with `token_endpoint_auth_method: "none"`. The representation then has no
`client_secret` member at all: `example:broker-credentials-at-third-party-services.public.create`

```http example="broker-credentials-at-third-party-services.public.create"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "Public Client Test Service",
  "client_id": "public-client-id",
  "token_endpoint_auth_method": "none",
  "…": "…"
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440001",
  "canonical_id": null,
  "display_name": "Public Client Test Service",
  "credential_source": "stored",
  "client_registration": "static",
  "client_id": "public-client-id",
  "token_endpoint_auth_method": "none",
  "oauth2_flavor": "standard",
  "issuer_uri": "https://provider.example.test",
  "discovery": { "…": "…" },
  "endpoints": { "…": "…" },
  "scopes": [{ "scope_value": "profile", "description": "Profile" }],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

The members that the block shortens are the same as in the first example. This response is
otherwise complete, and it has no secret in it.

A public client that also sends a secret contradicts itself. The broker does not drop the secret
silently. It rejects the request and stores nothing: `example:broker-credentials-at-third-party-services.public.secret-rejected`

```http example="broker-credentials-at-third-party-services.public.secret-rejected"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "client_id": "public-client-id",
  "client_secret": "must-not-be-stored",
  "token_endpoint_auth_method": "none",
  "…": "…"
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "client_secret must not have a non-empty value when token_endpoint_auth_method is \"none\""
}
```

Without a secret, PKCE is what ties the code exchange to the authorization. When Alice starts to
connect, the broker sends her browser to the provider with an S256 code challenge:

```http example="broker-credentials-at-third-party-services.public.connect"
GET /api/third-party/550e8400-e29b-41d4-a716-446655440001/oauth2/authorize?redirect_uri=http%3A%2F%2Flocalhost%3A8000%2Fdone HTTP/1.1

HTTP/1.1 302 Found
Location: http://127.0.0.1:…/oauth/authorize?client_id=public-client-id&code_challenge=…&code_challenge_method=S256&redirect_uri=http%3A%2F%2Flocalhost%3A8000%2Fapi%2Fthird-party%2F550e8400-e29b-41d4-a716-446655440001%2Foauth2%2Fcallback&response_type=code&scope=read&state=…

<a href="…">Found</a>.
```

The token request that follows carries `client_id` and `code_verifier`. It carries no
`client_secret` and no `Authorization` header. `example:broker-credentials-at-third-party-services.public.connect`
A later refresh is equally free of credentials. `example:broker-credentials-at-third-party-services.secret.connect-and-refresh`

## A hosted Client ID Metadata Document

A provider that supports Client ID Metadata Documents (CIMD) needs no registration in advance. The
client ID is a URL, and the provider fetches a document from it. With `client_registration: "cimd"`
the broker hosts that document itself.

The confidential form authenticates with `private_key_jwt`. It needs a signing key first, and
the broker refuses the registration while none exists. `example:broker-credentials-at-third-party-services.cimd-public.no-key`
The platform operator therefore generates a key with `POST /api/cimd-client-keys`. The
administrator then registers the service without a client ID and without a secret. The broker assigns the client ID:
its own public URL, the path `/.well-known/oauth-client/`, and the UUID of the service: `example:broker-credentials-at-third-party-services.cimd.create`

```http example="broker-credentials-at-third-party-services.cimd.create"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "CIMD confidential service",
  "token_endpoint_auth_method": "private_key_jwt",
  "issuer_uri": "https://cimd-provider.example.test",
  "…": "…"
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440002",
  "credential_source": "stored",
  "client_registration": "cimd",
  "client_id": "https://broker.e2e.test/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440002",
  "token_endpoint_auth_method": "private_key_jwt",
  "…": "…"
}
```

The request did not name `client_registration`. A `private_key_jwt` method implies `cimd`, and
every other method implies `static`. `example:broker-credentials-at-third-party-services.combinations.omitted-registration`

Anyone can fetch the document, without authentication. This is the document of a service that
uses `private_key_jwt`:

```http example="broker-credentials-at-third-party-services.cimd-public.switch-method"
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003 HTTP/1.1

HTTP/1.1 200 OK
Cache-Control: public, max-age=300
Content-Type: application/json

{
  "client_id": "https://broker.e2e.test/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003",
  "redirect_uris": [
    "https://broker.e2e.test/api/third-party/550e8400-e29b-41d4-a716-446655440003/oauth2/callback"
  ],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "private_key_jwt",
  "token_endpoint_auth_signing_alg": "ES256",
  "jwks_uri": "https://broker.e2e.test/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003/jwks.json"
}
```

The document has exactly these seven members. `example:broker-credentials-at-third-party-services.cimd.document`
Its `client_id` equals the URL it was fetched from, which is how the provider knows the document
belongs to the client. The `jwks_uri` leads to the public halves of the signing keys.

At the token endpoint the broker sends its client ID URL and exactly one signed client assertion
of the JWT-bearer type, and no secret. `example:broker-credentials-at-third-party-services.cimd.assertion`
If the provider rejects the assertion, the connection fails and no session is stored. The broker
does not try again with another method. `example:broker-credentials-at-third-party-services.cimd.assertion-rejected`

### A key set of its own

The keys that sign client assertions are not the keys that sign the broker's own tokens. The
operator manages them under `/api/cimd-client-keys` in every server mode, and generating,
promoting, or deleting one of them leaves the token-signing keys untouched. `example:broker-credentials-at-third-party-services.cimd.key-domain`

Rotation is the operator's decision, and it has two steps so that a provider with a cached key
set keeps working:

1. Generate a new key. It appears in the published key set at once, while the previous key still
   signs. `example:broker-credentials-at-third-party-services.cimd.rotation.grace-key`
2. Promote it with `PUT /api/cimd-client-keys/` followed by the key ID and `/current`. The next
   assertion is signed with it. `example:broker-credentials-at-third-party-services.cimd.rotation.promote`

The `jwks_uri` in the document does not change during a rotation. `example:broker-credentials-at-third-party-services.cimd.rotation.stable-location`

A confidential CIMD service with no usable key is not a client at all. Its document and its key
set answer `404 Not Found`, and a refresh fails before any request reaches the provider. `example:broker-credentials-at-third-party-services.cimd.rotation.no-usable-key`

## A keyless CIMD public client

The fourth pair combines the hosted document with a public client. The administrator sends
`client_registration: "cimd"` and `token_endpoint_auth_method: "none"`, again without a client ID
or secret. The broker assigns the same kind of URL and shows no `client_secret`. `example:broker-credentials-at-third-party-services.cimd-public.create`
This pair must be asked for by name. A request that says only `none` creates a static public
client. `example:broker-credentials-at-third-party-services.combinations.omitted-registration`

No key is involved: the broker creates and updates such a service while its key set is empty. `example:broker-credentials-at-third-party-services.cimd-public.no-key`
The document says so by leaving out the two members about keys:

```http example="broker-credentials-at-third-party-services.cimd-public.switch-method"
GET /.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003 HTTP/1.1

HTTP/1.1 200 OK
Cache-Control: public, max-age=300
Content-Type: application/json

{
  "client_id": "https://broker.e2e.test/.well-known/oauth-client/550e8400-e29b-41d4-a716-446655440003",
  "redirect_uris": [
    "https://broker.e2e.test/api/third-party/550e8400-e29b-41d4-a716-446655440003/oauth2/callback"
  ],
  "grant_types": ["authorization_code", "refresh_token"],
  "response_types": ["code"],
  "token_endpoint_auth_method": "none"
}
```

Both documents on this page come from one service. An administrator can move a CIMD service
between `none` and `private_key_jwt`, and its client ID URL stays the same. The move to
`private_key_jwt` is refused while no usable key exists. `example:broker-credentials-at-third-party-services.cimd-public.switch-method`

## Combinations the broker rejects

The two remaining pairs make no sense. A static client has no hosted key set to verify an
assertion against, and a hosted document must name a method. The broker rejects them on create
and on update, before it contacts the provider: `example:broker-credentials-at-third-party-services.combinations.rejected`

```http example="broker-credentials-at-third-party-services.combinations.rejected"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "client_registration": "static",
  "token_endpoint_auth_method": "private_key_jwt",
  "…": "…"
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "token_endpoint_auth_method private_key_jwt requires cimd client_registration"
}
```

The opposite mistake, `cimd` with no method, receives the message
`client_registration cimd requires private_key_jwt or none`. `example:broker-credentials-at-third-party-services.combinations.rejected`

Two kinds of service cannot use a hosted document. A service of the `google` flavor takes its
client ID from a credential document, and a service that reads its credentials from files has a
client ID that the broker does not control. Both reject `cimd` and accept an explicit `static`. `example:broker-credentials-at-third-party-services.combinations.google-and-files`

## Switching between modes

An update replaces the whole service, so a switch is a `PUT` that describes the new mode
completely. Three rules follow from that.

**The secret goes when it is no longer used.** Updating a confidential service to `none` removes
the stored secret. `example:broker-credentials-at-third-party-services.switch.secret-removed`
The way back needs a secret in the same request. Without one the broker answers
`client_secret is required` and the service stays public. `example:broker-credentials-at-third-party-services.switch.secret-required`

**Alice's session survives.** A switch does not touch stored sessions. Her encrypted tokens are
the same before and after. The next refresh uses the mode in force at that moment: no secret
after the switch to public, the new secret after the switch back. `example:broker-credentials-at-third-party-services.switch.sessions-kept`

**The client ID changes only when the new mode is valid.** Moving a static confidential service to
CIMD replaces the administrator's client ID with the hosted URL and removes the secret. A request
that still carries the old secret is refused, and the service keeps its client ID and its secret.
Moving back to static needs a client ID from the administrator. After that the hosted document
answers `404 Not Found`. `example:broker-credentials-at-third-party-services.switch.registration`

## Credentials from mounted files

A stored secret changes only when an administrator sends a new one. Where a secret manager
rotates the client secret on a schedule, the operator instead mounts the client ID and the secret
as two files and binds them to the canonical ID of the service in the broker configuration: `example:broker-credentials-at-third-party-services.files.binding`

```yaml
third_party_oauth2:
  credential_files:
    filesystem-pair:
      client_id_file: /meta/credentials/client-id
      client_secret_file: /meta/credentials/client-secret
```

The binding alone changes nothing. The administrator selects the source on the service, and sends
neither a client ID nor a secret: `example:broker-credentials-at-third-party-services.files.create`

```http example="broker-credentials-at-third-party-services.files.create"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "canonical_id": "filesystem-pair",
  "credential_source": "filesystem",
  "display_name": "Credential source service",
  "…": "…"
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440005",
  "canonical_id": "filesystem-pair",
  "display_name": "Credential source service",
  "credential_source": "filesystem",
  "client_registration": "static",
  "token_endpoint_auth_method": null,
  "…": "…"
}
```

The representation shows the source and neither credential, not even the client ID. The broker
reads the files when it needs them. When Alice starts to connect, the client ID in the redirect
to the provider is the content of the client ID file at that moment. `example:broker-credentials-at-third-party-services.files.create`

### Rotation without a restart

The code exchange and every refresh read both files again. When the secret manager publishes a
new secret for the same client ID, the next connection and the next refresh of Alice's existing
session use it. Nobody restarts the broker or updates the service. `example:broker-credentials-at-third-party-services.files.rotation.secret`

A new client ID is a different matter, because Alice's tokens were issued to the old client. Each
session remembers the client ID it was established with. After the files change to another client
ID, the broker refuses to refresh the old session and to exchange a code that was requested under
the old client ID. It refuses before it sends anything to the provider. Alice connects again and
receives a session under the new client ID. `example:broker-credentials-at-third-party-services.files.rotation.client-id`

### No file, no fallback

A service that selects `filesystem` and has no binding cannot authenticate. The broker does not
fall back to anything. Starting a connection and refreshing a session both fail with
`500 Internal Server Error` and the error `internal_error`, and no request reaches the provider. `example:broker-credentials-at-third-party-services.files.unbound`

### What the source excludes

A `filesystem` service that also sends `client_id` or `client_secret`, even as `null` or an empty
string, is rejected with `inline credentials must be absent for filesystem credential_source`. `example:broker-credentials-at-third-party-services.files.inline-rejected`
Files hold a client ID and a secret, so only a static confidential service can select them. A
public client, a `private_key_jwt` service, and a `google` service reject `filesystem`. `example:broker-credentials-at-third-party-services.files.excluded-modes`

### Changing the source

Switching a stored service to `filesystem` removes the stored client ID and secret in the same
update. `example:broker-credentials-at-third-party-services.files.switch-to-files`
The way back never copies the file values into storage. The administrator must send the
credentials to store: `example:broker-credentials-at-third-party-services.files.switch-to-stored`

```http example="broker-credentials-at-third-party-services.files.switch-to-stored"
PUT /api/services/550e8400-e29b-41d4-a716-446655440007 HTTP/1.1
Content-Type: application/json

{
  "canonical_id": "metadata-source",
  "credential_source": "stored",
  "…": "…"
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "explicit client_id and client_secret are required when returning to stored credential_source"
}
```

In both directions, existing sessions follow the client ID rule from the rotation section: a
session continues only if the newly selected client ID equals the one it was established with. `example:broker-credentials-at-third-party-services.files.switch-to-files` `example:broker-credentials-at-third-party-services.files.switch-to-stored`

## When discovery chooses

An administrator who registers a service from a protected-resource URL does not choose any of
this. The broker reads what the provider supports and picks the hosted `private_key_jwt` document
first and dynamic client registration otherwise. A request that tries to set
`client_registration` as well is rejected. The response reports the outcome: `cimd` for the
hosted document, `static` for a dynamically registered client. A dynamically registered client
also reports the method that the provider registered, for example `client_secret_basic`. `example:broker-credentials-at-third-party-services.discovery.selection`

## Not yet on this page

- The content of the published key set and the claims inside a client assertion.
- The order between confidential and public dynamic client registration.
- Keyless CIMD public clients on the wire, and CIMD in `proxy` and `hybrid` mode.
- Unusable credential files (missing, empty, oversized, not a regular file), whitespace trimming,
  projected-volume symlink swaps, and the events that separate a file problem from a provider
  rejection.
- Configuration of bindings through environment variables and the command line, the Helm chart,
  and the administration screens.
- Deleting a key, the rejection of key algorithms other than ES256, and broker startup with CIMD
  services but no key.

## Sources

The behavior on this page was decided in `specs/042-thirdparty-public-pkce/spec.md`,
`specs/046-cimd-upstream-client/spec.md`, `specs/051-oauth2-secret-file-overrides/spec.md`, and
`specs/050-oauth2-protected-resource-discovery/spec.md`, and in
`adrs/036-public-client-token-endpoint-auth.md`,
`adrs/037-cimd-client-authentication-key-domain.md`,
`adrs/041-oauth2-client-secret-file-overrides.md`, and
`adrs/044-client-registration-independent-of-authentication.md`. The contracts are
`api/admin/openapi.yaml` and `api/enduser/openapi.yaml`. The scenarios are in
`tests/e2e/thirdparty_public_client_test.go`, `tests/e2e/thirdparty_client_registration_test.go`,
the `tests/e2e/thirdparty_cimd_*_test.go` files, and the `tests/e2e/oauth2_secret_file_*_test.go`
files.
