---
title: "Third-party service registration"
description: Living specification of how an administrator describes an external OAuth2 provider, its scopes, and the resources it protects. Every example on this page is checked against the acceptance suite.
---

# Third-party service registration

An agent can act for a user at an external OAuth2 provider only after the broker knows that
provider. The broker stores each provider as a **ThirdpartyOAuth2Service**. This page describes
how an administrator registers a service and keeps it current, in the order an administrator
meets the behavior.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

A platform team operates the broker. An administrator of that team, `admin@example.com`, works
through the admin API. The team wants the *Research Assistant* agent to read repositories at
*GitHub* for users such as Alice.

Before that can happen, the broker needs four facts about GitHub:

- where GitHub's authorization and token endpoints are,
- which scopes GitHub offers, so that a permission set such as `github-read` can bundle them,
- which API addresses GitHub's tokens are good for, so that a token exchange for
  `https://api.github.com` finds this service,
- how the broker proves its own identity to GitHub.

The last fact has its own page, "Broker credentials at a third-party service". The examples here
use the simplest form, a client ID and a client secret that the broker stores.

Every request on this page carries the identity of an authenticated administrator. Most blocks
do not show that header.

## Registering a provider by hand

The administrator sends one description of GitHub: `example:third-party-service-registration.service.create`

```http example="third-party-service-registration.service.create"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "GitHub Test Service",
  "oauth2_flavor": "github",
  "client_id": "Iv1.1234567890abcdef",
  "client_secret": "…",
  "issuer_uri": "https://github.com",
  "discovery": {
    "enable_discovery": false
  },
  "endpoints": {
    "token_endpoint": "https://github.com/login/oauth/access_token",
    "authorize_endpoint": "https://github.com/login/oauth/authorize"
  },
  "scopes": [
    {
      "scope_value": "repo",
      "description": "Repository access"
    }
  ],
  "protected_resources": [
    "https://api.github.com"
  ]
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "660e8400-e29b-41d4-a716-446655440000",
  "display_name": "GitHub Test Service",
  "client_id": "Iv1.1234567890abcdef",
  "client_secret": "REDACTED",
  "oauth2_flavor": "github",
  "protected_resources": [
    "https://api.github.com"
  ],
  "…": "…"
}
```

Three details are worth noticing:

- The broker gives the service a UUID. Other records refer to the service by that UUID.
- The response never returns the secret. It shows the word `REDACTED` in its place.
- `discovery.enable_discovery` is `false`, so the broker uses the two endpoints exactly as given.

## The flavor says which dialect the provider speaks

Providers differ in details, for example in the form of the credential. The field `oauth2_flavor`
names the dialect. The broker accepts three values: `standard`, `google`, and `github`. Any
other value, such as `azure`, is refused with `400 Bad Request`. `example:third-party-service-registration.flavor.unknown`

The administrator can leave the field out. The broker then chooses:

- A service whose token endpoint is at `github.com` becomes `github`. `example:third-party-service-registration.flavor.github-detected`
- Any other service becomes `standard`. `example:third-party-service-registration.flavor.default-standard`

The `google` flavor changes what the credential is. The administrator sends a Google service
account key, a JSON document, in `client_secret` and sends no `client_id`. The broker takes the
client ID from that document, and it redacts the document in responses like any other
secret. `example:third-party-service-registration.flavor.google`

The credential must always fit the flavor. An update that changes a `standard` service to
`google` but carries a plain-text secret is refused with `400 Bad Request`. `example:third-party-service-registration.flavor.change-revalidates`

## Extra parameters that one provider needs

Some providers expect a request parameter that OAuth2 does not define. A commerce platform, for
example, wants a `business_partner_id` on every authorization request. The administrator stores
such parameters on the service as `authorization_params`. The broker returns them when the
service is created, read, and listed: `example:third-party-service-registration.authorization-params.returned`

```http example="third-party-service-registration.authorization-params.returned"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "Zalando Platform",
  "authorization_params": {
    "business_partner_id": "12345"
  },
  "…": "…"
}

HTTP/1.1 201 Created

{
  "authorization_params": {
    "business_partner_id": "12345"
  },
  "…": "…"
}
```

When Alice later connects her account, the broker adds `business_partner_id=12345` to the
redirect to the provider, to the code exchange, and to each token
refresh. `example:third-party-service-registration.authorization-params.sent-to-provider`
The value is administrator configuration, not user input. If Alice's browser sends
`business_partner_id=untrusted` to the broker, the provider still receives
`12345`. `example:third-party-service-registration.authorization-params.browser-ignored`

A later update of the service has two ways to treat the parameters:

- An update that does not mention `authorization_params` keeps them. `example:third-party-service-registration.authorization-params.update-omits`
- An update that sends an empty object removes them. `example:third-party-service-registration.authorization-params.clear`

The broker builds some parameters itself, such as `state` and `client_id`. An administrator
cannot configure those names. A service that carries `"state": "unsafe"` in its
`authorization_params` is refused with `400 Bad Request`. `example:third-party-service-registration.authorization-params.broker-owned`

## The resources that a service protects

A gateway that holds Alice's token asks the broker for a token for an address, for example
`https://api.github.com`. The broker finds the service through its **Protected Resources**: a
set of normalized URIs that the service owns. A URI belongs to at most one service, because the
address alone must decide which provider's token the gateway receives.

The GitHub service in the examples below already exists and owns `https://api.github.com`.

### One resource at a time

The team puts a new API behind GitHub tokens. The administrator adds its address. The request
needs no client secret and no other field of the service: `example:third-party-service-registration.protected-resources.add`

```http example="third-party-service-registration.protected-resources.add"
POST /api/services/660e8400-e29b-41d4-a716-446655440000/protected-resources HTTP/1.1
Content-Type: application/json

{
  "resource_uri": "https://resources.example.test/post/"
}

HTTP/1.1 201 Created
ETag: "2"

{
  "resource": "https://resources.example.test/post",
  "protected_resources": [
    "https://api.github.com",
    "https://resources.example.test/post"
  ]
}
```

The broker stored the URI without its trailing slash. The answer carries the stored URI, the
complete set, and an `ETag` that changes with every change of the service.

The same step has more forms:

- The administrator can name the resource in the path instead, with
  `PUT /api/services/…/protected-resources/…` and the percent-encoded URI as the last segment.
  The other fields of the service stay as they were. `example:third-party-service-registration.protected-resources.add-by-uri`
- Adding a URI that the service already owns succeeds with `200 OK` and changes
  nothing. `example:third-party-service-registration.protected-resources.add-again`
- `PATCH` on a resource with a body such as `{"to": "https://resources.example.test/target/"}`
  replaces that URI with the new, normalized one. `example:third-party-service-registration.protected-resources.rename`
- `GET /api/services/…/protected-resources` returns the set and the current
  `ETag`. `example:third-party-service-registration.protected-resources.list`

A second service, *Google*, owns `https://shared.example.test/resource`. GitHub cannot claim the
same address. The request is refused with `409 Conflict`, and neither set
changes. `example:third-party-service-registration.protected-resources.owned-elsewhere`

### Removing a resource takes effect at once

GitHub owns `https://resources.example.test/resolution`, and a gateway exchanges tokens for it
successfully. The administrator removes the resource with `DELETE` on its path. The answer is
`200 OK` with the remaining set. The next exchange for that address fails, although Alice's
grant and session are unchanged: `example:third-party-service-registration.protected-resources.remove`

```http example="third-party-service-registration.protected-resources.remove"
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=urn%3Aietf%3Aparams%3Aoauth%3Agrant-type%3Atoken-exchange
&subject_token=…
&subject_token_type=urn%3Aietf%3Aparams%3Aoauth%3Atoken-type%3Aaccess_token
&client_assertion_type=urn%3Aietf%3Aparams%3Aoauth%3Aclient-assertion-type%3Ajwt-bearer
&client_assertion=…
&resource=https%3A%2F%2Fresources.example.test%2Fresolution

HTTP/1.1 400 Bad Request

{
  "error": "invalid_target",
  "error_description": "token exchange target is invalid or unavailable"
}
```

### Replacing the whole set needs the current version

The full update of a service can also carry `protected_resources`. It then replaces the complete
set, so it could silently undo a resource that a colleague added a moment ago. The broker
therefore asks the administrator to prove which version was read. The administrator reads the
`ETag` of the set and sends it back in `If-Match`: `example:third-party-service-registration.protected-resources.replace-set`

```http example="third-party-service-registration.protected-resources.replace-set"
PUT /api/services/660e8400-e29b-41d4-a716-446655440000 HTTP/1.1
Content-Type: application/json
If-Match: "1"

{
  "display_name": "GitHub OAuth2 Service Updated",
  "protected_resources": [
    "https://api.example.com/"
  ],
  "…": "…"
}

HTTP/1.1 200 OK
ETag: "2"

{
  "protected_resources": [
    "https://api.example.com"
  ],
  "…": "…"
}
```

Without `If-Match`, the same update is refused with `428 Precondition Required`. With an
outdated value it is refused with `412 Precondition Failed`. In both cases the service stays as
it was. `example:third-party-service-registration.protected-resources.replace-needs-if-match`

## Letting the broker find the provider from a resource

For some services the administrator knows only the address of the API, for example an MCP server
at `https://cimd-only.example.test/mcp`. Such a resource can say which authorization server
protects it. The administrator gives the broker the address and nothing else: `example:third-party-service-registration.discovery.create`

```http example="third-party-service-registration.discovery.create"
POST /api/services HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "display_name": "Example MCP",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://cimd-only.example.test/mcp"
  }
}

HTTP/1.1 201 Created

{
  "id": "660e8400-e29b-41d4-a716-446655440002",
  "display_name": "Example MCP",
  "client_id": "https://broker.e2e.test/.well-known/oauth-client/660e8400-e29b-41d4-a716-446655440002",
  "issuer_uri": "https://cimd-issuer.example.test/tenant",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://cimd-only.example.test/mcp",
    "client_method": "cimd",
    "authorization_param_resource_strategy": "derived"
  },
  "endpoints": {
    "token_endpoint": "https://cimd-issuer.example.test/tenant/token",
    "authorize_endpoint": "https://cimd-issuer.example.test/tenant/authorize"
  },
  "authorization_params": {
    "resource": "https://cimd-only.example.test/mcp"
  },
  "…": "…"
}
```

To build this answer the broker made three requests: one to the resource itself, one for the
resource's metadata document, and one for the metadata of the issuer that the document names.
The scenario checks exactly this sequence. The result:

- `issuer_uri` and both endpoints come from the issuer's metadata.
- `authorization_params.resource` holds the verified address. The broker sends it to the provider
  as the audience of the tokens it requests.
- The broker also chose how to identify itself to this provider. Here it uses a client document
  that it hosts, and it registers no other client. `example:third-party-service-registration.discovery.client-selection`
  The page "Broker credentials at a third-party service" describes that choice.

### When the resource names two issuers

The resource `https://shared.example.test/mcp` accepts tokens from two authorization servers. The
broker does not guess. It creates nothing and returns the choices: `example:third-party-service-registration.discovery.issuer-selection`

```http example="third-party-service-registration.discovery.issuer-selection"
POST /api/services HTTP/1.1
Content-Type: application/json

{
  "display_name": "Protected MCP service",
  "discovery": {
    "enable_discovery": true,
    "resource_url": "https://shared.example.test/mcp"
  }
}

HTTP/1.1 400 Bad Request

{
  "error": "discovery failed",
  "message": "issuer_selection_required",
  "authorization_servers": [
    "https://login-a.example.test",
    "https://login-b.example.test"
  ]
}
```

The administrator repeats the request with `"issuer_uri": "https://login-b.example.test"`. The
broker then creates the service and contacts only that issuer.

### Discovery has no fallback

A service uses either discovery from a resource or values that the administrator supplies, never
a mix of the two. The reason is safety: if discovery fails, the broker must not quietly continue
with values that nobody verified.

- The resource publishes no metadata. The request fails with `resource_metadata_not_found`.
  Repeating it with manual endpoints added is a validation error, and no service
  exists afterwards. `example:third-party-service-registration.discovery.no-fallback`
- A request that names both `discovery.resource_url` and `discovery.metadata_url` is rejected
  before the broker contacts anything. `example:third-party-service-registration.discovery.conflicting-sources`
- A resource address that leads to a loopback address is rejected with `unsafe_destination`, and
  the broker does not connect to it. `example:third-party-service-registration.discovery.unsafe-destination`

### Looking at the state of discovery

Each service has a discovery status that the administrator can read. Reading it needs
authentication and causes no request to the provider. `example:third-party-service-registration.discovery.status-ready`
A service that was registered by hand reports `not_applicable`, and all other fields are
`null`. `example:third-party-service-registration.discovery.status-manual`

A full update of a discovered service repeats discovery. Suppose the service *Files MCP* works,
Alice is connected, and the issuer then publishes a broken metadata document. The update fails
with `400 Bad Request`, and the status records the attempt: `example:third-party-service-registration.discovery.failed-refresh`

```http example="third-party-service-registration.discovery.failed-refresh"
GET /api/services/660e8400-e29b-41d4-a716-446655440004/discovery-status HTTP/1.1

HTTP/1.1 200 OK

{
  "status": "failed",
  "resource_url": "https://files.example.test/mcp",
  "issuer_uri": "https://login.example.test",
  "client_method": "dcr",
  "last_attempt_at": "2026-10-31T14:00:00.000000000Z",
  "last_success_at": "2026-10-30T09:00:00.000000000Z",
  "failure_reason": "authorization_server_metadata_invalid"
}
```

The failed update changed nothing else. The service, its `ETag`, and Alice's session are the same
as before, and Alice's token can still be refreshed.

## Not yet on this page

- How the broker proves its identity to the provider: stored secret, public client, hosted client
  document, dynamic registration, and mounted files. See "Broker credentials at a third-party
  service".
- Canonical IDs of services. See "Resource identifiers".
- Reading, listing, and deleting a service, and the rule that a service in use cannot be deleted.
- The remaining validation rules of the `google` flavor, such as required fields and the size
  limit of the service account key.
- Discovery of endpoints from a `discovery.metadata_url` or from the issuer's well-known address
  for a service that is registered by hand.
- The order in which the broker tries metadata locations, the handling of an explicit token
  audience, and what happens to connected sessions when the audience changes.
- The audit events for changes to protected resources, and the guarantee that parallel changes
  to one service do not overwrite each other. No acceptance scenario on this page proves them.

## Sources

The behavior on this page was decided in `specs/006-domain-model-apis/spec.md`,
`specs/013-token-exchange/spec.md`, `specs/018-oauth2-provider-flavors/spec.md`,
`specs/034-provider-auth-params/spec.md`, `specs/035-protected-resource-subresources/spec.md`,
`specs/050-oauth2-protected-resource-discovery/spec.md`,
`adrs/030-normalize-protected-resources.md`, and
`adrs/040-protected-resource-discovery-and-dcr.md`. The contract is `api/admin/openapi.yaml`.
The scenarios are in `tests/e2e/oauth2_provider_flavor_test.go`,
`tests/e2e/oauth2_provider_github_flavor_test.go`,
`tests/e2e/provider_authorization_params_test.go`,
`tests/e2e/service_protected_resources_test.go`, `tests/e2e/token_exchange_test.go`,
`tests/e2e/thirdparty_protected_resource_discovery_test.go`,
`tests/e2e/thirdparty_protected_resource_rejection_test.go`,
`tests/e2e/thirdparty_protected_resource_status_test.go`, and
`tests/integration/service_credential_source_api_test.go`.
