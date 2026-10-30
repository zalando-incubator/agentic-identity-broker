# Research: Docker Setup

**Branch**: `20251230-120000-docker-setup` | **Date**: 2025-12-30 | **Status**: Complete

## Summary

Research resolves all technical decisions for containerizing the agentic identity broker application. The feature requires packaging pre-built Go backend and React frontend artifacts into a single production-ready container with multi-stage builds and multi-architecture support.

---

## Decision: Multi-Stage Dockerfile Strategy

### Decision
Use multi-stage Dockerfile with optimization stages for minimal runtime artifact.

### Rationale
- **Minimized final image size**: Non-essential files excluded from runtime image
- **Security**: Reduces attack surface by excluding build tools and dependencies
- **Performance**: Smaller images pull faster, faster container starts
- **Multi-architecture**: ARG TARGETARCH enables future cross-compilation

### Implementation
Multi-stage approach with optimization layers before final runtime stage that receives pre-built artifacts.

---

## Decision: Base Image Selection

### Decision
Use `alpine:latest` for runtime base image.

### Rationale
- **Minimal footprint**: ~7-10MB base image
- **Shell access**: Includes busybox shell for debugging in production
- **Mature ecosystem**: Widely used, well-supported for Go applications
- **Package management**: apk available if runtime dependencies needed

---

## Decision: Pre-Built Artifacts Approach

### Decision
Docker image uses pre-built binary and frontend assets as inputs.

### Rationale
- **Performance**: Avoids recompilation during Docker build
- **Flexibility**: Decouples build orchestration from Docker image creation
- **Spec compliance**: FR-004 requires supporting pre-built binaries; FR-005 requires compiled frontend assets

### Implementation
- Backend: Pre-built Go binary from `./bin/agentic-identity-broker` (release-quality, built via `just build-all`)
- Frontend: Pre-built static assets from `./web/dist/` (optimized, built via `just build-all`)
- Both copied into Alpine runtime image by `docker build`

---

## Decision: Frontend Asset Serving

### Decision
Backend serves compiled frontend static assets from `/web/dist/` directory.

### Rationale
- **Unified endpoint**: Single container means one HTTP server
- **Existing pattern**: Project already uses chi/v5 to serve frontend
- **Production standard**: Common full-stack application pattern

### Implementation
- Frontend pre-built to `web/dist/`
- Docker copies `web/dist/` into image
- Go backend serves static assets via chi router

---

## Decision: Environment-Based Configuration

### Decision
All runtime configuration passed via environment variables.

### Rationale
- **12-factor compliance**: Standard practice for containerized applications
- **Existing infrastructure**: Project uses Viper which supports env var sourcing
- **No code changes needed**: Leverages existing 20251214-120100-flexible-configuration system

### Configuration Variables
- `APP_PORT`: HTTP service port (default: 8000)
- `LOG_LEVEL`: Logging level (default: info)
- `DATABASE_URL`: PostgreSQL connection string (if applicable)
- Standard Go env vars as needed

---

## Decision: Non-Root User Execution

### Decision
Container process runs as non-root user (uid=1000).

### Rationale
- **Security best practice**: Principle I (Security-First)
- **Industry standard**: Standard container security practice
- **No application impact**: Go binaries don't require root privileges

---

## Decision: Multi-Architecture Support

### Decision
Dockerfile uses `ARG TARGETARCH` for future multi-platform build support.

### Rationale
- **Future-proof**: FR-003 requires supporting multi-architecture deployments
- **Zero overhead**: Declaration costs nothing; only used when cross-compiling
- **Standard pattern**: Docker buildx pattern for multi-arch images

### Current Scope
Build for amd64 locally. Future builds can use cross-compilation via buildx.

---

## Decision: Task Automation Integration

### Decision
Separate concerns: `just build-all` produces release artifacts; `just docker-build` packages them.

### Rationale
- **Consistency**: Unified task system (following existing patterns)
- **Clarity**: Clear distinction between artifact creation and image creation
- **Flexibility**: Developers can build artifacts and images separately if needed

### Tasks
- `just build-all` - Build release-quality backend binary and frontend assets
- `just docker-build` - Create Docker image from pre-built artifacts
- `just docker-run` - Run built image locally

### Usage Example
```bash
just build-all       # Builds backend to ./bin/ and frontend to ./web/dist/
just docker-build    # Creates Docker image from these artifacts
just docker-run      # Run container
```

---

## Assumptions Validated

All spec assumptions verified against codebase:

✅ Build environment has containerization (Docker available)
✅ Compiler toolchain available (Go 1.24.0)
✅ Node.js available (18+)
✅ Backend accessible on configurable port (chi/v5)
✅ Frontend compiled to static assets (Vite)
✅ External services via environment config (Viper)
✅ Pre-built artifacts supported (file copy)

---

## Next Steps (Phase 1)

- [x] Research complete
- [ ] Generate quickstart guide
- [ ] Create Dockerfile implementation
- [ ] Update justfile with docker tasks
