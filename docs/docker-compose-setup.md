# Docker Compose Development Setup

This guide explains how to start the complete Agentic Identity Broker stack with Docker
Compose. It includes hot reload for the backend and frontend.

## Quick Start (5 minutes)

### Prerequisites

- Docker installed and running, with Docker Compose v2 (`docker compose`)
- `justfile` (included in repo)

### Step 1: Start All Services

```bash
just compose-up
```

This command does the following:

1. Creates `.env.compose` from `.env.compose.example` when required.
2. Builds Docker images for all services.
3. Starts all services with hot reload.
4. Runs seed-data scripts.

The command shows service startup messages and container logs. Seed-data scripts finish after
the broker health check.

### Step 2: Access Services

Open in your browser:

- **Frontend and backend API**: http://localhost:8000
- **Admin API**: http://localhost:14000
- **Upstream OAuth2**: http://localhost:9001
- **Third-Party OAuth2**: http://localhost:9000
- **Sample Agent**: http://localhost:9002

### Examine service status

```bash
# Check all services are healthy
just compose-health

# View logs from any service
just compose-logs-backend
just compose-logs-frontend

# Make a code change (modify a Go file)
# → Air rebuilds in ~1-2 seconds
# → Check logs: just compose-logs-backend
```

### Step 4: Stop All Services

Press `Ctrl+C` in the terminal, or:

```bash
just compose-down
```

---

## Services Overview

| Service | Port | Purpose | Notes |
|---------|------|---------|-------|
| `identity-broker` | Internal 8000, published 14000 | Backend APIs | Hot reload with Air |
| `frontend` | Published 8000 → container 3000 | React UI and backend proxy | HMR enabled |
| `upstream-oauth2` | 9001 | Mock upstream OAuth2 provider | Fixed build |
| `third-party-oauth2` | 9000 | Mock third-party service | Fixed build |
| `sample-agent` | 9002 | Sample OAuth2 client | Fixed build |

Compose exposes one browser origin: `http://localhost:8000`. Vite serves the current `web/` source and proxies `/api`, `/oauth2`, `/.well-known`, and `/health` to the broker.
The backend image contains no copied frontend build. Port 3000 remains internal to the frontend container.
OAuth2 callbacks and HMR use the published port 8000. The Vite proxy preserves the browser host and adds the development principal.

---

## Configuration Files

The project uses different configuration files for Docker Compose and native development.

### `config.yaml` (Native development)

Use this configuration file when the broker runs directly on your host with `just run` or
`just dev`. It uses `localhost` and `127.0.0.1` addresses:

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: http://127.0.0.1:9001
  upstream_authorize_endpoint: http://127.0.0.1:9001/oauth/authorize
  upstream_token_endpoint: http://127.0.0.1:9001/oauth/token
```

Use this mode for native debugging or profiling. You can use it when you do not require the
complete stack.

### `configs/config.docker.yaml` (Docker Compose)

Docker Compose uses this file for container-to-container communication. It uses internal
container DNS names:

```yaml
oauth2_authorization_server:
  upstream_issuer_uri: http://upstream-oauth2:9001
  upstream_authorize_endpoint: http://upstream-oauth2:9001/oauth/authorize
  upstream_token_endpoint: http://upstream-oauth2:9001/oauth/token
```

Use it with `just compose-up`. `docker-compose.yml` sets
`IDENTITY_BROKER_CONFIG_PATH=configs/config.docker.yaml`.

### Why the files differ

- **Native development:** `just run` uses `localhost` or `127.0.0.1`. Services are not in a
  Docker network.
- **Docker Compose:** Services use container DNS names such as `upstream-oauth2`. These names
  resolve only in the Docker network.
- **No manual switch:** The environment selects the configuration based on the start command.

---

## Hot Reload Development

### Backend (Go with Air)

When you change a `.go` file in `./cmd` or `./internal`, Air responds:

1. Air detects the change.
2. Air compiles `tmp/agentic-identity-broker`.
3. Air restarts the server.
4. The server provides the new behavior after restart.

**View logs:**
```bash
just compose-logs-backend
```

**Manual restart:**
```bash
just compose-restart-backend
```

### Frontend (React with Vite HMR)

When you change a file in `./web/src`, Vite responds:

1. Vite detects the change.
2. The browser receives an HMR update.
3. Vite replaces the component without a full page reload.
4. The change is visible in about 500ms.

**View logs:**
```bash
just compose-logs-frontend
```

**Manual restart:**
```bash
just compose-restart-frontend
```

### Configuration Changes

If you change `build/air/.air.docker.toml` or `vite.config.ts`, restart the affected service:

```bash
just compose-restart-backend    # For Air config changes
just compose-restart-frontend   # For Vite config changes
```

---

## Seed Data

The broker startup wrapper runs `scripts/setup-dev-data.sh` after the admin API becomes ready.
Air runs the wrapper again after backend rebuilds. There are no separate seed-service containers.

The script creates Weather Assistant, Task Manager Pro, Sample Agent, Local Research Agent, and CIMD Demo Agent.
It also creates Mock OAuth2, Weather, Calendar, and Email services with their permission sets.

List the agents through the admin API:

```bash
curl -fsS -H 'X-Remote-User: dev@example.com' http://localhost:14000/api/agents
```

If automatic seeding fails, run the script inside your broker container:

```bash
docker compose --env-file .env.compose exec \
  -e MOCK_SERVER_TOKEN_URL=http://third-party-oauth2:9000 \
  identity-broker bash scripts/setup-dev-data.sh
```

---

## Common Workflows

### View All Logs in Real-Time

```bash
just compose-logs
```

### View Logs from One Service

```bash
just compose-logs-backend
just compose-logs-frontend
just compose-logs-service upstream-oauth2
```

### Restart a service after code changes

```bash
just compose-restart-backend
just compose-restart-frontend
```

### Examine service health

```bash
just compose-health
```

This command shows:
- Running containers and their state
- Health state for each service
- Connectivity test results

### Start services in the background

```bash
just compose-up-detached

# Later, stop them:
just compose-down
```

### Clean containers, volumes, and temporary files

```bash
just compose-clean
```

---

## Troubleshooting

### "Address already in use" Error

One or more published ports (8000, 14000, 9000, 9001, 9002, or 9004) are already in use.

Find the service that owns the port:

```bash
lsof -nP -iTCP:8000 -sTCP:LISTEN
docker ps
```

Do not stop another person's service. Use a separate Compose project and override its ports and container names.
When you change the browser port, change the broker public URL and HMR client port to match:

```yaml
services:
  frontend:
    ports: !override
      - "8001:3000"
    environment:
      - VITE_HMR_CLIENT_PORT=8001
  identity-broker:
    ports: !override
      - "14001:14000"
    environment:
      - IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL=http://localhost:8001
```

Also set `broker.base_url` and `broker.authorize_endpoint` in the Sample Agent configuration to the matching browser origin.

The `!override` tag requires Docker Compose 2.24.4 or later. Also give the project unique container names and nonconflicting mock ports.

### Frontend cannot access the backend

The backend API proxy returned a connection error.

**Examine:**
```bash
# 1. Backend is running
curl http://localhost:8000/health

# 2. Inside frontend container, can reach backend
just compose-logs-frontend  # Look for proxy errors

# 3. Check Vite proxy config
# Edit: web/vite.config.ts
# The proxy target should be http://localhost:8000 (host) or http://identity-broker:8000 (container)
```

**Examine the proxy:**
```bash
curl -fsS http://localhost:8000/health  # Vite proxies this request to the broker
```

### Seed data did not start

The startup wrapper reports seed errors in the backend logs.

```bash
just compose-logs-backend
```

If the broker is healthy, use the manual command in [Seed Data](#seed-data).

### File changes are not detected

Air uses polling for changes in bind-mounted files.

**Expected behavior:**
- Go changes are detected in about 1–2 seconds.
- React changes are detected in about 500ms.

**Make sure that polling is enabled:**
```bash
grep "poll = " build/air/.air.docker.toml
# Should show: poll = true
```

**Manual restart:**
```bash
just compose-restart-backend
```

### Slow container startup

The first startup builds Docker images and takes about 1–2 minutes. Later starts are faster.

To reduce startup time:
- Use an SSD for Docker storage.
- Allocate more Docker Desktop resources.
- Build images with `docker compose build` before you start the stack.

### "health check: too many retries"

Slow startup can cause health-check failures.

**Read logs:**
```bash
just compose-logs

# Increase startup delay in docker-compose.yml:
# identity-broker:
#   healthcheck:
#     start_period: 20s  # Increase from 10s
```

---

## Native Development

Native development requires Go 1.27.1, Node 24 or later, npm, OpenSSL, and just. Hot reload also requires Air.

```bash
# Build both artifacts and serve the current UI on port 8000
just run

# Or rebuild the backend and frontend after source changes
just dev
```

The broker serves `web/dist` from the repository root. `just run` builds that directory before startup.
Native Air rebuilds it before each backend restart and watches frontend source files. Docker Air does not build the frontend because Vite owns it.
Protected APIs require a trusted reverse proxy that supplies `X-Remote-User`. A direct native browser request does not supply that principal.

For optional native Vite HMR, use two terminals:

```bash
# Terminal 1: callback URLs point to the Vite browser origin
IDENTITY_BROKER_SERVER_ENDUSER_PUBLIC_URL=http://localhost:3000 just dev

# Terminal 2: Vite proxies API requests to the native broker on port 8000
just web-dev
```

In this mode, open `http://localhost:3000/`. Vite supplies the development identity.
Keep the browser on that origin throughout OAuth2. Selection drafts use tab-local storage and cannot cross origins.
Do not start native Vite alongside Compose. Compose already provides the frontend and its proxy on port 8000.

---

## Environment Variables

### Backend (.env.compose)

```bash
IDENTITY_BROKER_LOG_LEVEL=debug              # Log verbosity
IDENTITY_BROKER_LOG_FORMAT=text               # text or json
IDENTITY_BROKER_JWE_SIGNING_KEY=...          # Encryption key
IDENTITY_BROKER_STATE_TOKEN_TTL=10m          # Token lifetime
IDENTITY_BROKER_STORAGE_BACKEND=memory       # memory or postgres
```

### Frontend (docker-compose.yml)

```bash
VITE_API_URL=http://identity-broker:8000     # Server-side proxy target
VITE_USE_POLLING=true                       # Bind-mounted source changes
VITE_HMR_HOST=localhost                     # Browser websocket host
VITE_HMR_CLIENT_PORT=8000                   # Published browser websocket port
```

Compose sets these values on the frontend service. They are not production API configuration.
Native Vite defaults its proxy target to `http://localhost:8000`.

### Seed Scripts (.env.compose)

```bash
ADMIN_API=http://identity-broker:14000/api   # Admin endpoint (container DNS)
BROKER_HEALTH_URL=http://identity-broker:14000/health  # Health check endpoint
MOCK_SERVER_URL=http://third-party-oauth2:9000        # Mock server DNS name
```

---

## Production Docker Image

For production deployment (not for development):

### Build Production Image

```bash
# Build release binaries and frontend assets before Docker packaging
just build-linux-amd64 build-linux-arm64 web-build

# Build and load the native image (use linux/amd64 on an x86_64 host)
docker buildx build --load --platform linux/arm64 \
  -f build/docker/Dockerfile -t agentic-identity-broker:local .
```

The Dockerfile copies the pre-built architecture-specific binary and `web/dist` into the image. It does not install Node dependencies or build the frontend.
The `just docker-build-broker` recipe builds both binaries and the frontend before image packaging. It validates both image architectures without loading a local image.

The `just` broker and ExtProc build recipes use `-trimpath` and strip debug
symbols by default. To keep DWARF, run `LDFLAGS="" just build` (or set the
same variable for an architecture-specific or ExtProc recipe). The CIMD,
mock-service, and ExtProc development Docker builds support
`--build-arg LDFLAGS=""`. The release workflow passes the tagged commit's
`SOURCE_DATE_EPOCH` to Buildx and checks each image's amd64 and arm64 OCI config
creation timestamp against that commit.

### Run Production Image

For a local image smoke run, use the native configuration and fresh development keys:

```bash
export IDENTITY_BROKER_JWE_SIGNING_KEY="$(./scripts/generate-jwe-key.sh)"
export IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY="$(./scripts/generate-jwe-key.sh)"
docker run --rm --name aib-production-review \
  -p 8000:8000 -p 14000:14000 \
  -v "$PWD/config.yaml:/app/config.yaml:ro" \
  -e IDENTITY_BROKER_JWE_SIGNING_KEY -e IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY \
  agentic-identity-broker:local
```

Open `http://localhost:8000/`. Do not mount host frontend assets over the image.
For deployment, provide your production configuration, secrets, public HTTPS URL, and trusted authentication proxy.

### Key Differences from Dev

- Pre-built binaries (no hot reload)
- Production-optimized (minified, no source maps)
- Alpine Linux base (10x smaller)
- Single container (no compose orchestration)

---

## Tips & Tricks

### Quick Local Testing

```bash
# Build the current frontend and backend before a native smoke run
just run
```

### Monitor All Logs

```bash
# Terminal 1: Stream all logs
just compose-logs

# Terminal 2: Work on code
# Services auto-rebuild and logs appear in Terminal 1
```

### Debug a Specific Service

```bash
# Get into running container
docker compose --env-file .env.compose exec identity-broker /bin/sh

# Then run commands directly
curl http://localhost:8000/health
ps aux | grep air
```

### Validate Configuration

```bash
# Check docker-compose.yml syntax
just compose-validate

# Check all environment variables are set
cat .env.compose | grep -v "^#"
```

### Network Isolation Testing

Application services share the project-scoped bridge network `<project>_aib-network`. Docker allocates a free subnet automatically to prevent address-range collisions between checkouts.

Compose also assigns project-scoped container names. Run commands through Compose service names, not fixed container names.
Published host ports remain fixed. Simultaneous stacks need distinct port mappings and matching application URLs.

If startup failed with `Pool overlaps with other one on this address space`, retry `just compose-up-detached`. Existing networks from other projects do not need removal.

```bash
# List project-scoped application networks
docker network ls --filter label=com.docker.compose.network=aib-network

# Test service DNS resolution
docker compose --env-file .env.compose exec identity-broker getent hosts upstream-oauth2
```

---

## Architecture Diagram

```text
Browser: http://localhost:8000
  │
  ▼
frontend:3000 (Vite, bind-mounted web source, HMR)
  │
  ├── SPA routes and current source assets
  │
  └── /api, /oauth2, /.well-known, /health
        │
        ▼
      identity-broker:8000 (Go API, Air hot reload)
        │
        ├── upstream-oauth2:9001
        └── third-party-oauth2:9000

Admin: http://localhost:14000 → identity-broker:14000
Sample client: http://localhost:9002 → frontend on port 8000
```

---


## Next steps

- **Edit code:** Edit a `.go` or `.tsx` file. The relevant service rebuilds.
- **View logs:** Use `just compose-logs` to debug.
- **Run tests:** Use `just verify` on the host.
- **Build for production:** Use `just docker-build-broker` or the native Buildx command in this guide.
