# Contract: End-User API 2.0.0 — invalidated connections and filesystem credential currency

**Status**: Proposed (approval item API-ENDUSER). Not approved. Implementation copies these deltas into `api/enduser/openapi.yaml` and the rendered `docs/api/` pages only after approval.

**Scope**: Documented behavior only. Routes, parameters, schemas, status codes, error codes, and the RFC 8693 `error_uri` construction (ADR 044) stay the same. The response sets below already exist. This change documents when they now occur.

## `info.version`

```yaml
info:
  version: 2.0.0   # was 1.1.0
```

## `GET /oauth2/authorize` (`oauth2Authorize`)

Replace the **Local/Hybrid Mode** bullets and add a **Connection checks** block:

```yaml
        **Local/Hybrid Mode**:
        When the broker acts as its own OAuth2 authorization server:
        - If the user has an active grant and every connection described under Connection checks
          is usable, redirects to `redirect_uri` with `code` and `state`
        - Otherwise redirects to the consent UI for user approval or reconnection
        - Supports PKCE (RFC 7636) with `code_challenge` and `code_challenge_method`

        **Connection checks** (all modes, after client, redirect_uri, and scope validation):
        - Each third-party service that the active grant includes, and each mandatory service of
          the agent, must have a usable connection for the user. A connection is unusable when it
          is missing or expired, including after an administrator or deployment changed that
          service's client credentials
        - An unusable connection redirects to the consent UI (`/agents/{agent-id}?session_token=...`)
          even when the grant is active. The grant itself is not changed. Optional services that
          the grant does not include never trigger this redirect
        - For a service whose credentials come from mounted files, the broker reads the files first.
          If they are missing, unreadable, or incoherent, the response is the `server_error`
          redirect. If they differ from the last observed credentials, the broker invalidates
          every connection for that service first, and the request redirects to consent
```

Under **Proxy Mode**, add one bullet:

```yaml
        - The same connection checks apply before the upstream redirect
```

In the `Location` header description of the `"302"` response, change the consent bullet:

```yaml
                - Consent UI URL if the user has no active grant or an included or mandatory connection is unusable (all modes)
```

## `GET` and `POST /api/third-party/{serviceId}/oauth2/authorize`

Append to both descriptions:

```yaml
        For a service whose credentials come from mounted files, initiation reads both the client
        ID and the secret file and establishes that they are current before it redirects. If either
        file is missing, unreadable, or incoherent, the existing `500` response is returned and no
        provider redirect occurs. A changed pair first invalidates every connection for the service.
```

## `GET /api/third-party/{serviceId}/oauth2/callback`

Replace the **competing callbacks** sentence and add a bullet:

```yaml
          The first completed callback from a shared starting state wins; competing callbacks return
          success without replacing its credentials while the winner's connection is usable.
        - A callback for an authorization that began before the service's client credentials changed
          redirects to `/sessions?error=callback_failed` without completing the code exchange, and
          the change remains in effect. This includes a reconnect whose connection was invalidated
          in the meantime. The user starts a new login.
```

## `GET /api/third-party/sessions` and `GET /api/third-party/{serviceId}/session`

Append:

```yaml
        A connection invalidated by a change of the service's client credentials stays listed with
        `is_expired: true` and `has_refresh_token: false` until the user logs in again. Listing never
        reads credential files.
```

## `POST /api/third-party/{serviceId}/session/refresh`

Append to the `"409"` description:

```yaml
          This includes a connection invalidated by a change of the service's client credentials.
```

## `POST /oauth2/token` (RFC 8693 token exchange)

Append to the token exchange error documentation, next to the existing recoverable session list:

```yaml
        - A connection invalidated by a change of the service's client credentials returns the
          existing recoverable `400 invalid_grant` with the service-specific `error_uri` (ADR 044).
          Following it starts the provider login directly and returns to `/sessions`. The broker
          never returns an old access token after the change takes effect, even if the token has
          not expired
        - For a service whose credentials come from mounted files, every token request reads the
          files before it returns or refreshes a token. A changed pair invalidates the connections
          first. Missing, unreadable, or incoherent files return the existing non-recoverable
          `500 server_error` without `error_uri`, even if the stored access token has not expired
```

## Unchanged

- ExtProc forwarding (ADR 044 and Feature 023)
- Approval endpoints
- The consent API schemas (`/api/consent/agents/{agent-id}` keeps reporting services without active sessions as it does today)
- `/.well-known/*`
- JWKS
