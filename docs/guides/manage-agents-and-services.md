---
title: "Manage agents and services"
description: Register third-party OAuth2 services and define permission sets. Register agents and issue broker client credentials through the admin API on port 14000.
---

# Manage agents and services

This guide explains how an administrator creates the objects for delegation with the admin
API on port 14000. First, register a third-party service. Next, group its scopes into
permission sets. Then register an agent that requests those sets. In local and hybrid
server modes, issue broker client credentials to the agent.

Create the objects in this order. A permission set references a service. An agent
references permission sets.

## What you need

- **Admin access through the proxy.** A trusted proxy in front of the admin port
  authenticates the caller, enforces administrator privilege, and sends the principal
  header. The default header is `X-Remote-User`. The broker accepts it only from a trusted
  source. Examples send the header directly to a local development stack. In production,
  the proxy sends the header.
- **The admin base URL.** Examples use `http://localhost:14000`. In production, use the
  internal admin endpoint behind the proxy. Do not use end-user port 8000.

Every request carries the principal header:

```bash
-H "X-Remote-User: admin@example.com"
```

## Register a third-party OAuth2 service

A third-party service is an external OAuth2 provider, such as GitHub, Google, or
an
internal API. Use the
[service creation reference](/api/admin#tag/Services/operation/createService)
for field definitions, validation rules, and complete request and response
examples.
The OpenAPI reference is authoritative; this guide describes the administration
workflow.

1. Register a client with the provider, unless you use
   [broker-hosted CIMD](#register-a-broker-hosted-outbound-cimd-confidential-service)
   or
   [protected-resource discovery](#discover-a-protected-resource-with-hosted-cimd-or-dcr)
   .
2. Choose the matching creation example: automatic endpoint discovery or manual
   endpoints. Replace its provider URLs, credentials, and scopes with your own
   values.
   For a provider without scopes, use the scope guidance in the request schema.
3. Submit the request to the authenticated admin endpoint, not the end-user
   port.
4. Record the returned service ID. Use it when you
   [define permission sets](#define-permission-sets) .
5. For later changes, use the
   [service update reference](/api/admin#tag/Services/operation/updateService) ,
   including its full-replacement and credential-rotation rules. Do not copy
   redacted
   secrets from a read response into a replacement request.

Before deleting a service, revoke or migrate dependent grants. See the
[service deletion contract](/api/admin#tag/Services/operation/deleteService) for
conflicts.

## Migrate administrative consumers to Admin API 2.0.0

Admin API `2.0.0` is an unreleased breaking change from the `1.0.0` contract in broker release `v0.1.81`.
The existing `/api/services` routes remain unchanged. There is no compatibility endpoint or shim.

Before deployment:

1. Update administrative consumers to read `credential_source` and accept filesystem responses without `client_id` or `client_secret`.
2. Preserve stored-mode handling: responses include the stored client ID and redact applicable secrets as `REDACTED`.
3. For filesystem registration, supply a canonical ID and `credential_source: "filesystem"`. Omit both inline credential fields, including null placeholders.
4. Apply database migrations 039 and 040 before new broker pods start.
5. Enable filesystem mode only after all old broker pods are replaced.

Filesystem authentication acquires current credentials for each exchange or refresh attempt. Missing bindings or unusable files fail closed without stored-credential fallback.
Client-ID changes can require a new connection. The [filesystem service instructions](#register-and-update-a-filesystem-service) describe configuration and source transitions.

Guarded database downgrade refuses filesystem services and prior source transitions. A compatible downgrade discards non-secret session client-ID associations.

## Register and update a filesystem service

1. Configure the credential-file bindings for the service's canonical ID using
   [credential file configuration](/docs/configuration#third-party-oauth2-credential-files)
   .
   Keep paths in operator configuration, not in Admin API requests.
2. Start from the filesystem example in the
   [service creation reference](/api/admin#tag/Services/operation/createService)
   .
   Submit the request and record the service ID.
3. Read the service through the
   [service read endpoint](/api/admin#tag/Services/operation/getService) .
   Registration and metadata reads do not open credential files, so they do not
   prove
   that a later token exchange can acquire credentials.
4. Connect an account and verify access with the provider. Missing or unusable
   files
   fail closed without a stored-credential fallback.
5. To change the credential source, use the source-transition examples in the
   [service update reference](/api/admin#tag/Services/operation/updateService).
   Prepare replacement credentials before switching back to stored mode; do not
   copy
   response redaction markers or assume that the broker imports credentials from
   files.

After a source transition, users whose established client identity no longer
matches
must reconnect. A source round trip does not reset that identity association.

## Register a broker-hosted outbound CIMD confidential service

Use outbound CIMD when the broker acts as a client of a provider that accepts a
Client ID Metadata Document. This is separate from inbound CIMD resolution for
agents authenticating to the broker.

1. Set a stable public HTTPS end-user URL in
   [server configuration](/docs/configuration#server-configuration).
2. Prepare a usable ES256 key using
   [Manage CIMD client-authentication keys](/docs/guides/operate-oauth2-server-modes#manage-cimd-client-authentication-keys)
   .
   Reuse an existing usable key rather than generating one for each service. If
   a
   later key awaits activation, inspect its activation time before registration.
3. Submit the CIMD confidential example from the
   [service creation reference](/api/admin#tag/Services/operation/createService)
   .
4. Record the service ID and inspect the hosted document at the returned client
   ID
   URL. Check that the provider can retrieve it and its public JWK Set.
5. Add the service to a permission set and connect an account through the normal
   user flow. These keys sign outbound client assertions, not broker-issued
   tokens
   or sealed OAuth state.

### Register a CIMD public service

For a provider that accepts CIMD without client authentication, use the CIMD
public
example in the
[service creation reference](/api/admin#tag/Services/operation/createService) .
Set the public HTTPS end-user URL first, but do not generate a CIMD key for this
service.

After creation, inspect the hosted document and connect an account through the
normal user flow. A public CIMD document does not publish keys; consult the
[end-user API reference](/api/enduser) for its metadata and JWK endpoint
responses.

Use the CIMD public replacement example in the
[service update reference](/api/admin#tag/Services/operation/updateService) for
later
changes. It documents the explicit registration choice required on every
replacement,
method switches, and transitions to an operator-provided client identity. Plan
for
users to reconnect if the provider rejects refresh tokens after an identity
change.

During a rolling upgrade after migration 044, old broker pods fail closed on two
writes: creating a confidential CIMD service and rewriting a CIMD service to an
operator client ID. Retry those writes after all old pods leave service.

Before downgrading through migration 044, convert or delete every CIMD public
service. The rollback refuses to reinterpret a hosted URL as a static public
client ID. If this refusal leaves a dirty migration state, run
`migrate force 44`
only after the conversion or deletion, then retry the rollback.

## Discover a protected resource with hosted CIMD or DCR

Protected-resource discovery starts at a public HTTPS resource URL. The broker
validates its metadata, selects an advertised authorization server, and chooses
hosted CIMD or dynamic client registration (DCR). This differs from direct
registration against an authorization-server metadata URL.

Before creation:

1. Set a stable public HTTPS end-user URL for the hosted identity or DCR
   callback.
2. For hosted CIMD, prepare a usable key as described in
   [outbound CIMD setup](#register-a-broker-hosted-outbound-cimd-confidential-service)
   .
3. For DCR, configure the broker-wide client name using
   [protected-resource discovery configuration](/docs/configuration#protected-resource-discovery-configuration)
   .
   This is the deployment's client name, not the individual service display
   name.

### Create a discovery-backed service

1. Open the protected-resource example in the
   [service creation reference](/api/admin#tag/Services/operation/createService)
   .
   Use your resource URL and follow its discovery-owned credential and endpoint
   rules.
2. Decide whether to use the verified resource URL as the token audience or to
   pin
   a provider-required audience. The creation contract defines the override and
   response strategy; the protected-resource ownership list serves token
   exchange
   routing instead of selecting that audience.
3. Submit the request. Record the returned service ID for permission sets and
   status reads.
   Inspect the selected issuer, client method, and effective audience in the
   response.
4. Connect an account and confirm that the issued token works with the protected
   resource. Successful metadata discovery alone does not establish token
   suitability.

### Select one issuer when the resource advertises several

If creation asks you to select an issuer, inspect the validated choices in its
error response. Retry with the exact issuer you intend to trust. Use the
[creation error examples](/api/admin#tag/Services/operation/createService) for
the
response shape and retry fields rather than choosing an unadvertised issuer.

A failed create stores no service. Correct the reported problem before retrying;
do not assume that there is a partial service to refresh or query for status.

### Read status and refresh without replacing a working client

1. Read the
   [stored discovery status](/api/admin#tag/Services/operation/getServiceDiscoveryStatus)
   with the service ID. This does not contact the provider. Its examples
   distinguish
   a successful setup, a failed refresh, and a manual service.
2. Read the [active service](/api/admin#tag/Services/operation/getService) and
   record
   its issuer, client identity, effective audience, and ETag before making
   changes.
3. Use the protected-resource refresh example in the
   [service update reference](/api/admin#tag/Services/operation/updateService).
   Follow its omission rules to retain the selected client and any pinned
   audience.
4. Read both active configuration and discovery status again. A failed refresh
   can
   record an unsuccessful attempt while leaving the working configuration
   intact.
   Consult the update contract for safe failure codes and conflict behavior.

To change an issuer or effective audience, terminate every user session for the
service first, including expired sessions. Then submit the explicit change and
read back the configuration to verify the intended issuer and audience. Follow
the
update contract's ETag requirements when also replacing owned protected
resources.

If discovery cannot support the provider, treat
[manual registration](#register-a-third-party-oauth2-service) as a separate
setup
decision, not an automatic fallback. The broker does not silently replace a
failed
discovery-backed client with manual credentials.

## Define permission sets

A permission set is a business-readable group of scopes for one or more services. Users
consent to the group, not raw provider scope strings. Create a permission set with
`POST /api/permission-sets`.

The fields:

- `name` — A unique, human-readable name with at most 255 characters.
- `description` — The capabilities in words that users understand.
- `service_scopes` — One or more `{service_id, scopes[], requirement_type}` values.
  `service_id` is a service UUID. Omit `scopes` or use an empty list when the service has no
  scopes. Otherwise, each scope must match the service configuration. `requirement_type` is
  `mandatory` or `optional`.

```bash
curl -X POST http://localhost:14000/api/permission-sets \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "GitHub Read Access",
    "description": "Read repository contents and user profile from GitHub",
    "service_scopes": [
      {
        "service_id": "550e8400-e29b-41d4-a716-446655440001",
        "scopes": [ "repo", "user:email" ],
        "requirement_type": "mandatory"
      }
    ]
  }'
```

The response contains the permission-set `id` (a UUID), `created_at`, and `updated_at`. An
agent references this `id`. A change to `service_scopes` applies to the next token exchange
for grants that reference the set.

## Register an agent

An agent is the AI agent that requests delegated access. Create one with `POST /api/agents`.

The fields:

- `display_name` and `description` — Required values for the consent interface.
- `permission_sets` — At least one `{permission_set_id, requirement_type}` value is
  required. Mandatory sets stop authorization until a user grants them. Optional sets let
  an agent continue without the access. See
  [delegation and consent](/docs/concepts/delegation-and-consent#mandatory-vs-optional-requirements).
- `client_id` — Optional. Use it only for an agent that uses an upstream OAuth2 client ID.
  Omit it for agents identified in another way.
- `external_id` — Optional external governance identifier, such as a URN. Several agents can share one external ID.
- `redirect_uris` — Optional. Register HTTPS, loopback HTTP (`localhost`, `127.0.0.1`, or `[::1]`), or native-app private-use callbacks such as `com.example.app:/oauth2redirect/example-provider` and `github-app://oauth/callback`. The broker stores accepted entries unchanged and in order. Private-use entries must be canonical, lowercase, unencoded, and free of fragments.
- `governance_url`, `user_documentation_url`, and `agent_interface_url` — Optional URLs for
  the consent interface.

```bash
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "Research Assistant",
    "external_id": "urn:example:ai-registry:agent:7f3a9c",
    "description": "AI assistant that reads repositories and datasets on your behalf",
    "governance_url": "https://example.com/agents/research-assistant/governance",
    "user_documentation_url": "https://example.com/docs/research-assistant",
    "permission_sets": [
      {
        "permission_set_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
        "requirement_type": "mandatory"
      }
    ]
  }'
```

The response contains an agent with a system-generated `id` (a UUID). This `id` is the
agent canonical identifier. Use it as `client_id` on the broker `/oauth2/authorize`
endpoint. Do not use the optional upstream `client_id` field. The broker does not generate
an upstream `client_id`.

### Find agents by external ID

Filter the admin agent list by an external governance ID:

```bash
curl 'http://localhost:14000/api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c' \
  -H 'X-Remote-User: admin@example.com'
```

The response is an array with zero, one, or several agents. External IDs are not unique.
The filter matches the complete value exactly and respects case. It does not search agent UUIDs or canonical IDs.
Percent-encode reserved characters such as `&`, `+`, and `;` in the value. An unencoded semicolon is invalid.
If `external_id` is empty, repeated, undecodable, or longer than 255 Unicode code points, the broker returns `400` with `error: invalid external_id`.
Omit the parameter to list every agent. Filtered responses also accept `Prefer: reference-id=canonical` for nested references.

### Allow a native-app callback for a CIMD agent

Register the exact callback on the Agent before the CIMD client uses it:

```bash
curl -X POST http://localhost:14000/api/agents \
  -H "X-Remote-User: admin@example.com" \
  -H "Content-Type: application/json" \
  -d '{
    "display_name": "GitHub Copilot",
    "description": "GitHub Copilot Desktop",
    "client_uris": ["https://github.com/copilot/desktop/client-metadata.json"],
    "redirect_uris": ["github-app://oauth/callback"],
    "permission_sets": [{
      "permission_set_id": "770e8400-e29b-41d4-a716-446655440001",
      "requirement_type": "mandatory"
    }]
  }'
```

The fetched CIMD document must also list `github-app://oauth/callback`. Without the Agent entry, the broker rejects the whole document, including its web callbacks. The broker matches private-use callbacks character for character in both lists. Register the lowercase, unencoded URI shown in the document. Web callbacks for a CIMD client come from the document, not the Agent list.

## Issue broker client credentials

In `local` or `hybrid` [server mode](/docs/guides/operate-oauth2-server-modes), the broker
issues agent credentials for the token endpoint. Generate one credential set for each agent
with `POST /api/agents/{agent-id}/client-credentials`. Use the agent UUID in the path.

```bash
curl -X POST http://localhost:14000/api/agents/550e8400-e29b-41d4-a716-446655440000/client-credentials \
  -H "Content-Type: application/json" \
  -H "X-Remote-User: admin@example.com"
```

The response returns:

```json
{
  "client_id": "550e8400-e29b-41d4-a716-446655440000",
  "client_secret": "brk_sec_a1b2c3d4e5f6g7h8i9j0k1l2m3n4o5p6",
  "created_at": "2025-12-19T10:30:00Z"
}
```

The response has this structure:

- `client_id` equals the agent UUID. Each agent has one credential set.
- `client_secret` starts with `brk_sec_`. The response shows it only once. Store it securely.
  You cannot read it again. `GET …/client-credentials` returns metadata only.
- A repeated `POST` rotates the credential. It returns `200`, a new secret, and
  `previous_invalidated_at`. Tokens from the previous secret remain valid until expiry.

## Provider-specific configuration hints

### Google

Google requires two `authorization_params` to support token refresh:

```json
"authorization_params": {
  "access_type": "offline",
  "prompt": "consent"
}
```

- **`access_type: offline`** — This parameter makes Google issue a refresh token with the
  access token. Without it, Google returns only a short-lived access token. The broker
  cannot refresh the session after that token expires.
- **`prompt: consent`** — This parameter shows the consent interface for every
  authorization. Google issues a refresh token only for the first consent grant for one
  user-client pair. If a user previously authorized the OAuth2 client, Google does not
  include a refresh token in later responses. `prompt: consent` requests a new refresh
  token.

If either parameter is absent, the broker stores no refresh token. After the access token expires, token exchange returns `invalid_grant`.
Its `error_uri` starts third-party OAuth for the resolved service, for example:
`https://broker.example.com/api/third-party/550e8400-e29b-41d4-a716-446655440000/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions`.
The broker uses its configured public URL and encodes the `/sessions` return URI. The provider callback returns the user to `/sessions`.

:::tip
After you add or change `authorization_params`, existing sessions do not change. A user
whose session has no refresh token must authenticate again to use the new parameters.
:::

## Related

- **[/api/admin](/api/admin)** — the full admin OpenAPI reference: every field, response
  schema, and error code.
- **[Delegation and consent](/docs/concepts/delegation-and-consent)** — how the objects you
  create here become grants and sessions.
- **[Operate OAuth2 server modes](/docs/guides/operate-oauth2-server-modes)** — when an agent
  needs broker client credentials, and how proxy, local, and hybrid modes differ.
