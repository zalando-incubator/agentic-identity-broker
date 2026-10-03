# Docker Compose Integration Contract

**Purpose**: Defines the three new containers and their integration with the existing docker-compose setup.

## New Services

### 1. agentgateway

| Property | Value |
|----------|-------|
| Image | `cr.agentgateway.dev/agentgateway` |
| Container Name | `aib-agentgateway` |
| Ports | `4000:4000` (MCP HTTP), `15000:15000` (Admin UI) |
| Config Mount | `./mocks/agentgateway/config.yaml:/config.yaml:ro` |
| Command | `agentgateway -f /config.yaml` |
| Network | `aib-network` |
| Depends On | `extproc-token-exchange`, `mcp-server-mock` |

### 2. extproc-token-exchange

The config file is at `configs/config.extproc.docker.yaml` in the project `configs/` directory.

| Property | Value |
|----------|-------|
| Build | `Dockerfile.mock` with `SERVICE_PATH=cmd/extproc-token-exchange` |
| Container Name | `aib-extproc` |
| Ports | `50051` (gRPC, container-internal only — not exposed to Docker host) |
| Config Mount | `./configs/config.extproc.docker.yaml:/app/config.yaml:ro` |
| Environment | `EXTPROC_CLIENT_SECRET`, `EXTPROC_LOG_LEVEL=debug` |
| Network | `aib-network` |
| Depends On | `identity-broker` (healthy), `upstream-oauth2` |

The ExtProc gRPC interface is intentionally not exposed to the Docker host. Only agentgateway on `aib-network` should reach it.

### 3. mcp-server-mock

| Property | Value |
|----------|-------|
| Build | `Dockerfile.mock` with `SERVICE_PATH=mocks/mcp-server/cmd/mcp-server` |
| Container Name | `aib-mcp-server` |
| Ports | `9003:9003` |
| Config Mount | `./mocks/mcp-server/config.yaml:/app/config.yaml:ro` |
| Network | `aib-network` |

## Agentgateway Configuration

```yaml
# WARNING: Development configuration only.
# HTTP endpoints (identity-broker, upstream-oauth2) are used for local Docker Compose.
# In production: use HTTPS and set oauth2.tls.allow_http: false in ExtProc config.

# mocks/agentgateway/config.yaml
# yaml-language-server: $schema=https://agentgateway.dev/schema/config
binds:
- port: 4000
  listeners:
  - routes:
    - policies:
        cors:
          allowOrigins:
            - "*"
            # WARNING: Wildcard origin is for local development only.
            # Production deployments MUST restrict to specific origins.
          allowHeaders:
            - "*"
          exposeHeaders:
            - "Mcp-Session-Id"
        extProc:
          host: "extproc-token-exchange:50051"
          failureMode: failClosed
      backends:
      - mcp:
          targets:
          - name: tools
            mcp:
              host: http://mcp-server-mock:9003/mcp
```

## Network Topology

```
┌──────────────────┐      HTTP (9002)      ┌─────────────────┐
│  Sample Agent    │ ─────────────────────► │  agentgateway   │
│  (aib-sample-    │   MCP client call     │  (aib-agent-    │
│   agent:9002)    │                        │   gateway:4000) │
└──────────────────┘                        └────────┬────────┘
                                                     │
                                            ┌────────┴────────┐
                                            │                 │
                                   gRPC     ▼        HTTP     ▼
                            ┌──────────────────┐  ┌──────────────────┐
                            │ ExtProc Token    │  │ MCP Server Mock  │
                            │ Exchange         │  │ (aib-mcp-        │
                            │ (aib-extproc:    │  │  server:9003)    │
                            │  50051)          │  └──────────────────┘
                            └────────┬─────────┘
                                     │
                          ┌──────────┴──────────┐
                          │                     │
                HTTP      ▼             HTTP    ▼
         ┌──────────────────┐  ┌──────────────────┐
         │ Identity Broker  │  │ Upstream OAuth2  │
         │ (aib-broker:     │  │ (aib-upstream-   │
         │  8000)           │  │  oauth2:9001)    │
         └──────────────────┘  └──────────────────┘
```

## Port Allocation

| Service | Port | Protocol | Purpose |
|---------|------|----------|---------|
| identity-broker | 8000 | HTTP | End-user API (includes `/oauth2/token`) |
| identity-broker | 14000 | HTTP | Admin API |
| upstream-oauth2 | 9001 | HTTP | Mock upstream OAuth2 server |
| third-party-oauth2 | 9000 | HTTP | Mock third-party OAuth2 service |
| sample-agent | 9002 | HTTP | Sample agent application |
| mcp-server-mock | 9003 | HTTP | MCP server mock (Streamable HTTP) |
| frontend | 3000 | HTTP | Consent UI (Vite dev server) |
| agentgateway | 4000 | HTTP | MCP gateway (Streamable HTTP) |
| agentgateway | 15000 | HTTP | Agentgateway admin UI |
| extproc-token-exchange | 50051 | gRPC | ExtProc token exchange service |
