# Incremental service authorization

Use this guide to let an MCP client recover a tool call that failed because one broker-managed
third-party service is not usable, without a silent reauthorization loop. The design is recorded in
[ADR 045](../../adrs/045-service-session-recovery-return.md) and
[ADR 046](../../adrs/046-virtual-service-scopes.md).

Every configured service has a broker-only **Virtual Service Scope**:

```text
<server.enduser.public_url>/sessions/<canonical-id-or-UUID>
```

The broker emits the service's current canonical ID, or its lowercase UUID when it has none. The
same URL is the human-readable service page. The prefix comes only from configuration, never from a
request `Host`. Values with the prefix but an invalid identifier are rejected; URLs on another origin
are ordinary scopes. The broker never advertises these scopes in
`/.well-known/oauth-authorization-server` and never sends them to an upstream identity provider or
a third-party provider.

## Broker configuration

No new broker key is needed. The existing public URL defines the scope prefix:

```yaml
server:
  enduser:
    public_url: "https://broker.example.com"
oauth2_authorization_server:
  mode: "local"
```

The complete overlay is [`examples/config/incremental-authorization.yaml`](../../examples/config/incremental-authorization.yaml).

## Initial challenge

When a tool call's token exchange fails for a resolved service and the user can repair it, the
broker returns a recoverable RFC 8693 error with `error_uri` and the service `scope`. The ExtProc
sidecar turns it into a 403 challenge and keeps the existing JSON-RPC `-32042` URL elicitation:

```http
HTTP/1.1 403 Forbidden
Content-Type: application/json
WWW-Authenticate: Bearer error="insufficient_scope", error_description="User session is unavailable. Please re-authenticate.", error_uri="https://broker.example.com/api/third-party/880e8400-e29b-41d4-a716-446655440010/oauth2/authorize?redirect_uri=https%3A%2F%2Fbroker.example.com%2Fsessions%2Fgithub", scope="https://broker.example.com/sessions/github", resource_metadata="https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"
```

- A missing, expired or provider-rejected session (`invalid_grant`) links to the service's provider
  sign-in, which returns to `/sessions/<identifier>`.
- A grant that does not cover the service (`access_denied`) links to the agent's consent page.
- Errors before service resolution, and 5xx or transient errors, carry no `scope`.

## Protected-resource metadata

Each Agentgateway route can name its RFC 9728 protected-resource metadata URL with the optional
`aib.tokenexchange.resource_metadata_url` field. There is no ExtProc YAML key, environment variable
or CLI flag. Keep strict JWT authentication, `preserveToken: false` and `failureMode: failClosed`:

```yaml
extProc:
  host: "extproc-token-exchange:50051"
  failureMode: failClosed
  metadataContext:
    aib.tokenexchange:
      subject_token: "jwt.rawToken.unredacted()"
      resource_uri: "'https://agentgateway.example.com/mcp'"
      resource_metadata_url: "'https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp'"
```

Omit the field for the default variant: the challenge keeps `scope` and `error_uri` and leaves out
`resource_metadata`. The value must be an absolute `https` URL with a host, no userinfo or fragment,
printable ASCII without `"` or `\`, and at most 2048 bytes. An invalid value is omitted and logged as
`challenge_metadata_invalid` without the value; the request outcome does not change. Publishing the
metadata document itself is the deployment's responsibility. The full route is
[`examples/config/incremental-authorization-agentgateway.yaml`](../../examples/config/incremental-authorization-agentgateway.yaml).

## Recovery paths

1. **Focused consent (step-up).** A client that understands `scope` starts an interactive
   authorization request that adds the challenged scope, for example
   `scope=openid offline_access https://broker.example.com/sessions/github`. Every requested service
   is required for that request only. When the grant does not cover it, required provider scopes are
   missing, or its session is missing or unusable, the user sees consent with the service
   highlighted and its covering permission sets. Save stays disabled until the selection covers the
   service and its session is usable; the server enforces the same rule. After Save the browser
   returns to the original request, which is revalidated before a code or upstream redirect.
   Unverifiable usability returns `temporarily_unavailable`; malformed, unknown or ineligible service
   scopes return `invalid_scope`.
2. **Direct repair.** Any client can show the `error_uri`. The user signs in to the provider and
   returns to the service page, which shows the session, its scopes and dependent agents and offers
   refresh and termination. This repairs the session but never changes the agent's grant; a grant
   denial still needs consent.

## Client retries

Retries are owned by the client. Retry the original tool call once after the user finishes either
recovery path, and stop after a small fixed number of attempts. The ExtProc sidecar briefly caches a
re-authentication outcome per subject and resource, so allow a few seconds before the first retry.
Never loop on silent refresh: a local refresh that adds a service scope returns `invalid_scope` and
leaves the refresh token valid, and only interactive authorization can add it.

## OAuth2 mode limitations

- **Local and CIMD clients.** The token response and the JWT `scope` claim contain granted virtual
  scopes in their requested form. Every refresh rechecks every originally bound service, even after
  narrowing; a refusal never rotates or revokes the refresh token. `invalid_scope` means interactive
  authorization is required, and `server_error` means usability could not be verified now.
  `client_credentials` rejects virtual scopes.
- **Proxy clients.** The broker checks requested services before redirecting upstream and removes
  virtual scopes from upstream authorization and refresh requests, omitting an empty `scope`. A
  successful proxy refresh, including a rotated refresh token, does not attest that any
  broker-managed service is usable; the next exchange still challenges.
- **Hybrid.** Each agent follows its own local or proxy path.
- **Renames.** UUID scopes survive canonical-ID renames. A canonical scope bound to an old canonical
  ID fails closed: refresh returns `invalid_scope` without consuming the token, and the client must
  authorize again with the scope from a fresh challenge.
- **Rollback.** Rolling back migration `046` discards stored service bindings and requires rolling
  back the broker version; affected clients authorize again.
