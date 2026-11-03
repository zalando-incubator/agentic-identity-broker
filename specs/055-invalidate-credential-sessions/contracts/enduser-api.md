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
          If the revision/pair publication is missing, unreadable, incoherent, malformed, stale,
          or reuses a revision with different values, the response is the `server_error`
          redirect. If a coherent higher-revision pair differs from the last observed credentials, the broker invalidates
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
        For mounted credentials, initiation reads one coherent client ID, secret and publication
        revision (credential_revision_file) and establishes currency before redirecting. Invalid,
        unavailable or stale material returns the existing `500` without a provider redirect or
        recovery URI. A changed higher-revision pair first invalidates all service connections.
        First valid observation of unknown legacy connections also invalidates them conservatively;
        no existing connection is grandfathered without comparison evidence.
```

## `GET /api/third-party/{serviceId}/oauth2/callback`

Replace the **competing callbacks** sentence and add a bullet:

```yaml
          The first completed callback from a shared starting state wins; competing callbacks return
          success without replacing its credentials while the winner's connection is usable.
        - A stale generation detected before provider exchange returns
          `/sessions?error=callback_failed` without contacting the provider. If a change is recognized
          during an already admitted exchange, that provider call cannot be recalled; the broker
          rejects stale session persistence and returns the same `callback_failed` redirect.
          This applies to first connections and reconnections. The user starts a fresh login.
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
          never newly authorizes old-token release after the change-effective commit, even for an
          unexpired token. Its final generation/session-guarded decision orders release with that
          commit. A snapshot authorized before commit may still be delivered afterward
        - For a service whose credentials come from mounted files, every token request reads the
          coherent revision/pair files before token decisions. A changed higher-revision pair
          invalidates connections first; unknown legacy sessions expire on initialization. Missing,
          unreadable, incoherent, invalid or stale publications return the existing non-recoverable
          `500 server_error` without `error_uri`, even if the stored access token has not expired
```

The new required binding key and upgrade procedure are Proposed in [configuration.md](configuration.md). Lower-revision replicas cannot invalidate a fresh session. CAS conflicts abort the operation with the existing non-recoverable failure; retries require fresh acquisition, not relabeling old material.

## Unchanged

- ExtProc forwarding (ADR 044 and Feature 023)
- Approval endpoints
- The consent API schemas (`/api/consent/agents/{agent-id}` keeps reporting services without active sessions as it does today)
- `/.well-known/*`
- JWKS
