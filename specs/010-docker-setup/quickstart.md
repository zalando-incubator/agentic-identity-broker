# Quickstart: Docker Setup

**Branch**: `010-docker-setup` | **Date**: 2025-12-30

## Overview

This feature adds Docker containerization to the agentic identity broker, enabling packaged deployment of the Go backend and React frontend as a single production-ready artifact.

## Prerequisites

- Docker installed and running
- Go 1.24.0+ installed
- Node.js 18+ installed
- justfile available

## Quick Start

### 1. Build Release Artifacts

Create optimized backend binary and frontend assets:

```bash
just build-all
```

Produces:
- `./bin/agentic-identity-broker` - Optimized Go backend binary
- `./web/dist/` - Compiled React frontend

### 2. Build Docker Image

Package artifacts into Docker image:

```bash
just docker-build
```

Creates image: `agentic-identity-broker:latest`

### 3. Run Container

Start the application:

```bash
just docker-run
```

Access the application at `http://localhost:8000/`

## Image Details

- **Base**: `alpine-3:latest`
- **User**: Non-root (uid=1000, gid=1000)
- **Port**: 8000 (backend service), 14000 (admin)

## Configuration

Pass environment variables to customize behavior:

```bash
just docker-run APP_PORT=3000 LOG_LEVEL=debug
```

### Available Variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `APP_PORT` | 8000 | Backend HTTP service port |
| `LOG_LEVEL` | info | Application logging level |
| `DATABASE_URL` | N/A | PostgreSQL connection string (if applicable) |

## Multi-Architecture Support

Dockerfile supports future multi-architecture builds via `ARG TARGETARCH`. Current scope: amd64.

## Files Modified/Created

- `build/docker/Dockerfile` - Multi-stage build definition (NEW)
- `justfile` - Docker tasks added (MODIFIED)
- `.dockerignore` - Non-essential file exclusions (NEW)

## Related Documentation

- [Docker Image Specification](spec.md)
- [Research & Design Decisions](research.md)
- [Data Model](data-model.md)
- [API Contracts](contracts/README.md)
- [Development Workflow](../../AGENT.md#frontend-development-consent-ui)
