# Consent SPA Configuration

The consent console has one frontend source: `web/`. Production builds and development servers use that source.

## Backend Configuration

The broker serves the root-mounted SPA from `web/dist`, relative to its working directory.
It has no SPA-specific environment variables, YAML keys, or CLI flags.
The end-user server defaults to port 8000. The admin API uses port 14000.

Browser routes include `/agents`, `/agents/:id`, `/connections`, `/approvals`, `/approvals/remembered`, and `/settings/appearance`.
The root route opens Agents. API, OAuth2, discovery, and health namespaces take precedence over SPA fallback.

Configure the public browser origin and the trusted principal header through the existing server configuration:

```yaml
server:
  enduser:
    port: 8000
    bind: "::"
    public_url: https://broker.example.com
    authentication:
      preauth:
        principal_header_name: X-Remote-User
```

The corresponding environment variables are:

- `IDENTITY_BROKER_SERVER_ENDUSER_PORT`
- `IDENTITY_BROKER_SERVER_ENDUSER_BIND`
- `IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL`
- `IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME`.

The public URL determines OAuth2 callback URLs and locally issued token metadata.
Use the same origin for the UI, API proxy, and OAuth2 callback. Consent selection drafts use tab-local storage.
A callback on a different origin cannot restore that draft.

## Frontend Configuration

`web/vite.config.ts` sets `base: '/'` and builds to `web/dist`.
The build produces the asset manifest, CSP script hash, and compressed assets that the Go SPA handler serves.

The Axios client uses the relative base URL `/api`. There is no `VITE_API_BASE_URL` override.
There is no `VITE_DEV_SERVER_PORT` configuration. Native Vite listens on port 3000 and fails if that port is occupied.

Vite proxies `/api`, `/oauth2`, `/.well-known`, and `/health` to the backend.
It preserves the browser host and supplies the development principal `dev@example.com`.
These development server variables control that proxy and Docker HMR:

| Variable | Native default | Compose value | Purpose |
| --- | --- | --- | --- |
| `VITE_API_URL` | `http://localhost:8000` | `http://identity-broker:8000` | Server-side proxy target |
| `VITE_USE_POLLING` | Disabled | `true` | Watch bind-mounted source files |
| `VITE_HMR_HOST` | Used only in polling mode | `localhost` | Browser websocket host |
| `VITE_HMR_CLIENT_PORT` | `3000` in polling mode | `8000` | Browser websocket port |

These values do not change the production API origin.

## Native Runtime

Native development requires Go 1.27.1, Node 24 or later, npm, OpenSSL, and just.

```bash
# Build the backend and current frontend, then start the broker
just run
```

Run this command from the repository root. Open `http://localhost:8000/`.
The root `config.yaml` uses that public origin.
Protected APIs require a trusted authentication proxy or an explicit principal header in direct API requests.

For source changes, run Air:

```bash
just dev
```

This command requires Air. It installs missing frontend dependencies before startup.
Air watches backend and frontend source, rebuilds both artifacts, and restarts the broker.
Generated frontend output does not trigger another rebuild.

## Optional Native Vite HMR

Start the native backend with callback URLs that match the Vite browser origin:

```bash
# Terminal 1
IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL=http://localhost:3000 just dev

# Terminal 2
just web-dev
```

Open `http://localhost:3000/` for this workflow. Keep OAuth2 requests and callbacks on that origin.
Both servers use the current frontend source, but only Vite supplies the development principal.
Do not run this native Vite server alongside Docker Compose.

## Docker Compose

```bash
just compose-up
```

Compose publishes the frontend container's Vite port 3000 as `http://localhost:8000/`.
That is its only browser origin. Port 3000 is not published on the host.
Vite serves the bind-mounted `web/` source and proxies backend namespaces to `identity-broker:8000`.
The backend's end-user port remains internal. The admin port remains available at `http://localhost:14000`.

`configs/config.docker.yaml` sets the public URL to `http://localhost:8000`.
HMR and Sample Agent browser redirects use that origin too.
The backend development image contains no copied frontend build, so it cannot serve a stale second UI.

See [Docker Compose Development Setup](../docker-compose-setup.md) for mocks, seeding, and isolated-project instructions.

## Production Deployment

The production Dockerfile copies the pre-built frontend from `web/dist` into `/app/web/dist`.
It does not install Node dependencies or build the frontend.
The runtime image contains the pre-built architecture-specific Go binary and frontend assets from the same checkout.

```bash
# Build both backend architectures, frontend assets, and production images
just docker-build-broker
```

This recipe validates both image architectures without loading them into the local Docker daemon.
For a native image smoke run, use the Buildx command in the [Compose guide](../docker-compose-setup.md#production-docker-image).

For a filesystem deployment, build both artifacts:

```bash
just build-all
```

Deploy `bin/agentic-identity-broker` and `web/dist` from the same checkout.
Keep `web/dist` relative to the broker working directory. Do not mount an older frontend build over a production image.

The browser UI and API share the public HTTPS origin. A trusted authentication proxy supplies the configured principal header.
Keep the admin server on a separate trusted network. See [Kubernetes Deployment](../deployment/kubernetes.md).

## Authentication and CORS

The broker does not accept a client-provided identity as proof of authentication in production.
The deployment must protect the principal header at its trusted proxy boundary.
Vite's fixed development identity is for local development only.

Same-origin production and proxied development requests do not require cross-origin API access.
If a deployment needs CORS, configure it under `server.enduser.cors` with explicit permitted origins:

```yaml
server:
  enduser:
    cors:
      allowed_origins:
        - https://trusted-client.example.com
```

Consent writes and OAuth2 initiation retain their existing cross-origin checks.
Do not bypass those checks to compensate for a proxy that changes the browser host.

## Troubleshooting

### The broker shows an old UI

For native runtime, restart with `just run` or `just dev` to rebuild the current frontend.
For Docker, rebuild the production image from the current source rather than mounting host frontend output.
For Compose, open the published Vite origin on port 8000. Recreate only your own frontend container after deployment wiring changes.

### The SPA returns 404

For a filesystem deployment, make sure that `web/dist/index.html` exists relative to the broker working directory.
For Compose, examine frontend logs with `just compose-logs-frontend`.
The internal backend development container intentionally contains no SPA assets.

### Protected API requests return 401

Make sure that the trusted proxy supplies the configured principal header.
For a direct development API request, send the header explicitly:

```bash
curl -fsS -H 'X-Remote-User: dev@example.com' http://localhost:8000/api/me
```

### OAuth2 returns to the wrong origin

Make sure that `server.enduser.public_url` matches the browser URL, including its port.
For an isolated Compose port override, also update the HMR client port and Sample Agent browser URLs.
Keep the same browser tab and origin throughout the provider flow.

## Related Documentation

- [Configuration Guide](../configuration.md)
- [API Reference](../reference/api.md)
- [Root-Mounted SPA Decision](../../adrs/035-root-mounted-spa.md)
- [Docker Compose Development Setup](../docker-compose-setup.md)
