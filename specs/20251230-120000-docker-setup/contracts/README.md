# API Contracts: Docker Setup

**Branch**: `20251230-120000-docker-setup` | **Status**: N/A - No New APIs

## Summary

Docker setup introduces **no new APIs**. This feature is infrastructure-focused and packages existing application services into a containerized artifact.

## Existing APIs

All existing APIs (backend services, frontend UI) remain unchanged:

- **End-user APIs**: Documented in `/api/enduser/openapi.yaml` (unchanged)
- **Admin APIs**: Documented in `/api/admin/openapi.yaml` (unchanged)
- **Frontend**: React UI in `web/` (unchanged)

## Docker Artifact Interface

The Docker image itself has no API contract. It is a deployment artifact that:
- Accepts configuration via environment variables (documented in quickstart.md)
- Exposes service port `8000` for HTTP requests
- Serves existing backend and frontend services as-is

## Reference

- Principle IV - API Documentation & OpenAPI Transparency (constitution.md)
- Principle X - API-First Development (constitution.md)
