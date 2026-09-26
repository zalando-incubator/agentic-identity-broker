# Implementation Plan: Docker Setup

**Branch**: `010-docker-setup` | **Date**: 2025-12-30 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/010-docker-setup/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Create a production-ready Docker image that packages both the Go backend and React frontend (built static assets) into a single containerized artifact. The image uses multi-stage builds to minimize size, supports pre-built binaries as input, and integrates with the justfile task automation system. This enables consistent, reproducible deployments across environments while maintaining the existing development workflow.

## Technical Context

**Language/Version**: Go 1.24.0 (backend), Node.js 18+ (frontend build) | Multi-stage Dockerfile
**Primary Dependencies**:
- Go backend: chi/v5 (HTTP router), viper (configuration), slog (logging)
- Frontend: Node.js 18+, Vite build tool, React 18+
- Docker build infrastructure
**Storage**: N/A (stateless artifact, external config via environment)
**Testing**: Go test suite (backend), Vitest/Jest (frontend) - docker image acceptance testing via integration tests
**Target Platform**: Linux-based container (amd64 architecture initially, structured for future multi-arch)
**Project Type**: Web application (backend + frontend consolidated into single container)
**Performance Goals**: Image build completes in <5 minutes, container starts and is ready for requests in <30s
**Constraints**: Final image size optimized (multi-stage, minimal base image), minimal runtime dependencies, supports configuration via environment variables
**Scale/Scope**: Single production-ready container image, supports future multi-architecture builds

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: N/A - Docker setup is infrastructure, not domain modeling
- [x] **Domain Concepts**: N/A - No new domain concepts introduced
- [x] **Configuration Design**: N/A - Uses existing unified config system (viper), no new config options
- [x] **Config Examples**: N/A - Configuration passed via environment variables to running container
- [x] **API Design First**: N/A - No new APIs; artifact is deployment mechanism for existing services
- [x] **API Documentation**: N/A - No API changes
- [x] **API Changes**: N/A - No API changes
- [x] **Database Design**: N/A - No database schema changes

**Implementation Considerations**:

- [x] **Security-First**: Container runs as non-root user, uses minimal base image to reduce attack surface
- [x] **Architecture Docs**: N/A - Docker is deployment concern, doesn't change domain architecture
- [x] **ADRs**: N/A - Docker strategy is operational, not architectural decision requiring ADR
- [x] **Library-First Security**: N/A - No new security features; artifact uses existing application security model
- [x] **Zalando Guidelines**: N/A - No APIs being designed
- [x] **End-User Docs**: Will add deployment documentation with Docker usage examples
- [x] **Migration Testing**: N/A - No database migrations
- [x] **Hexagonal Architecture**: N/A - Artifact is deployment mechanism, doesn't change domain architecture
- [x] **Persistence Patterns**: N/A - No new persistence entities

*If any BLOCKING check fails, stop and clarify requirements. Implementation cannot begin until all preconditions complete.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

The Docker setup is primarily configuration-driven with deployment artifacts generated from existing source:

```text
build/docker/Dockerfile             # Multi-stage build definition (NEW)
justfile                            # Updated with docker build/deploy tasks (MODIFIED)

cmd/agentic-identity-broker/        # Existing Go backend entry point
web/                                # Existing React frontend
├── src/
│   ├── components/
│   ├── pages/
│   └── ...
└── dist/                           # Build output (frontend assets)

bin/
└── agentic-identity-broker         # Pre-built Go binary (input to docker build)
```

**Structure Decision**:
- **Dockerfile**: Multi-stage build strategy with multi-architecture support
  - Build args: `TARGETARCH` (amd64, arm64, etc.) for future multi-platform builds
  - Stage 1 (Builder): Node.js + Go compiler for building frontend and backend
  - Stage 2 (Runtime): Minimal base image (distroless or alpine) with:
    - Pre-built Go backend binary (copied from ./bin/)
    - Compiled frontend static assets
    - Non-root user for execution
    - Port exposure for backend service
    - Environment variable support for configuration
- **justfile**: Add docker-specific tasks
  - `just docker-build` - Build Docker image
  - `just docker-run` - Run image locally
  - `just docker-deploy` - Deployment task (placeholder for future CI/CD integration)

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
