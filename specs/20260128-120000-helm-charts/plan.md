# Implementation Plan: Helm Charts

**Branch**: `20260128-120000-helm-charts` | **Date**: 2025-01-27 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20260128-120000-helm-charts/spec.md`

## Summary

Create consumable Helm charts for deploying the Agentic Identity Broker to Kubernetes. The chart supports in-memory storage (default), external PostgreSQL, and Zalando PostgreSQL Operator provisioning. Database migrations run as a Helm hook Job with schema permissions, separate from the broker's data-only runtime. Static manifests can be generated via `helm template` for non-Helm environments.

## Technical Context

**Language/Version**: Helm 3.x (Go templates), Dockerfile modifications  
**Primary Dependencies**: Kubernetes 1.25+, golang-migrate, Zalando PostgreSQL Operator (optional)  
**Storage**: PostgreSQL 12+ (optional, in-memory fallback)  
**Testing**: helm lint, helm template, Kind cluster validation  
**Target Platform**: Kubernetes 1.25+ clusters  
**Project Type**: Infrastructure/Deployment (not application code)  
**Performance Goals**: N/A (infrastructure deployment)  
**Constraints**: Static manifests must be fully functional without Helm runtime  
**Scale/Scope**: Single Helm chart, 15-20 template files

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: N/A - Infrastructure feature, no new domain entities
- [x] **Domain Concepts**: N/A - No new domain concepts
- [x] **Configuration Design**: Helm values.yaml schema documented in spec.md
- [x] **Config Examples**: values.yaml serves as the example configuration
- [x] **API Design First**: N/A - No API changes, only deployment infrastructure
- [x] **API Documentation**: N/A - No new APIs
- [x] **API Changes**: N/A - No API changes
- [x] **Database Design**: N/A - Using existing migrations from /migrations/
- [x] **E2E Acceptance Tests**: Infrastructure E2E testing via Kind cluster validation (scripts/validate-helm-chart.sh) per Principle XIII - validates complete deployment integration
- [x] **E2E Test Mapping**: Kind validation script maps to all user story acceptance scenarios (deployment, ingress, PostgreSQL modes)
- [x] **E2E Red Phase**: Validation script fails before implementation (no chart exists yet)

**Implementation Considerations**:

- [x] **Security-First**: Pod Security Standards (restricted) by default, no privilege escalation
- [x] **Architecture Docs**: ARCHITECTURE.md update not required (deployment, not architecture)
- [x] **ADRs**: ADR for Helm chart design decisions considered (may add if complex patterns emerge)
- [x] **Library-First Security**: Using standard Kubernetes security patterns
- [x] **Zalando Guidelines**: N/A - No REST APIs in this feature
- [x] **End-User Docs**: docs/deployment/kubernetes.md will document Helm usage
- [x] **Migration Testing**: Migrations tested via existing integration tests
- [x] **Hexagonal Architecture**: N/A - Infrastructure, not domain code
- [x] **Persistence Patterns**: N/A - Using existing persistence layer

*All checks passed. This is an infrastructure feature that does not modify domain code, APIs, or database schema.*

## Project Structure

### Documentation (this feature)

```text
specs/20260128-120000-helm-charts/
├── plan.md              # This file (implementation plan)
├── research.md          # Phase 0 output (research findings)
├── data-model.md        # Phase 1 output (values.yaml schema)
├── quickstart.md        # Phase 1 output (deployment guide)
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code (repository root)

```text
charts/
└── agentic-identity-broker/
    ├── Chart.yaml                    # Chart metadata
    ├── values.yaml                   # Default configuration
    ├── .helmignore                   # Files to exclude from chart
    ├── README.md                     # Chart-specific documentation
    ├── templates/
    │   ├── _helpers.tpl              # Shared template helpers
    │   ├── NOTES.txt                 # Post-install notes
    │   ├── deployment.yaml           # Broker Deployment
    │   ├── service.yaml              # Dual-port Service (8000, 14000)
    │   ├── configmap.yaml            # Broker configuration
    │   ├── secret.yaml               # Optional secrets management
    │   ├── ingress-enduser.yaml      # End-user Ingress (port 8000)
    │   ├── ingress-admin.yaml        # Admin Ingress (port 14000)
    │   ├── job-migrate.yaml          # Migration Job (Helm hook)
    │   ├── postgresql.yaml           # Zalando PostgreSQL CR (optional)
    │   ├── serviceaccount.yaml       # ServiceAccount
    │   ├── hpa.yaml                  # HorizontalPodAutoscaler (optional)
    │   └── pdb.yaml                  # PodDisruptionBudget (optional)
    └── tests/
        └── test-connection.yaml      # Helm test for connectivity

scripts/
└── validate-helm-chart.sh            # Kind cluster validation script

docs/
└── deployment/
    └── kubernetes.md                 # End-user Kubernetes deployment guide
```

**Structure Decision**: Standard Helm chart structure in `/charts/` directory following Helm 3 best practices. Single chart supports all deployment modes via values.yaml configuration.

## Testing Strategy

### Infrastructure Validation

This is an infrastructure feature. Testing uses Helm tooling and Kind cluster validation rather than Go E2E tests.

**Validation Methods**:

| Method | Purpose | Command |
|--------|---------|---------|
| Helm Lint | Validate chart syntax and structure | `helm lint charts/agentic-identity-broker` |
| Template Rendering | Verify all values produce valid YAML | `helm template broker charts/agentic-identity-broker` |
| Kind Cluster Deploy | Validate templates work in real cluster | `kind create cluster && helm install broker ./charts/agentic-identity-broker` |
| Helm Test | Test connectivity after deployment | `helm test broker` |

**Test Scenarios**:

| Scenario | Values Override | Validates |
|----------|-----------------|-----------|
| Default (in-memory) | None | Deployment, Service, ConfigMap |
| External PostgreSQL | `storage.type=postgres, postgresql.external.enabled=true` | Migration Job, Secret references |
| Zalando Operator | `storage.type=postgres, postgresql.operator.enabled=true` | PostgreSQL CR, auto secret binding |
| Ingress enabled | `ingress.enduser.enabled=true, ingress.admin.enabled=true` | Both Ingress resources |
| Static manifests | `helm template` output | Complete manifest bundle |

**Test Location**: `charts/agentic-identity-broker/tests/test-connection.yaml`

### Kind Cluster Validation

Automated validation script runs against a local Kind cluster:

```bash
# scripts/validate-helm-chart.sh
#!/bin/bash
set -e

# Create Kind cluster
kind create cluster --name helm-test --wait 5m

# Install chart with default values (in-memory)
helm install broker ./charts/agentic-identity-broker --wait --timeout 2m

# Verify broker is running
kubectl wait --for=condition=available deployment/broker --timeout=60s

# Run Helm tests
helm test broker

# Verify health endpoint
kubectl port-forward svc/broker 8000:8000 &
sleep 2
curl -f http://localhost:8000/health

# Cleanup
kind delete cluster --name helm-test
```

**Justfile Integration**:
```makefile
# Validate Helm chart in Kind cluster
helm-validate:
    ./scripts/validate-helm-chart.sh

# Quick lint only
helm-lint:
    helm lint charts/agentic-identity-broker
```

### No Go E2E Tests Required

Per Constitution Principle XIII, E2E tests are required for feature behavior. This feature:
- Does not add new domain logic
- Does not modify APIs
- Is validated through infrastructure tooling (helm lint, Kind cluster deployment)
- Automated Kind validation provides equivalent coverage to manual E2E testing

## Phase 0: Research Tasks

Research completed and documented in [research.md](research.md).

| ID | Topic | Status | Key Decision |
|----|-------|--------|--------------|
| R1 | Helm chart best practices | ✅ Complete | Standard _helpers.tpl, named templates, semantic versioning |
| R2 | Zalando PostgreSQL Operator | ✅ Complete | Operator creates users/secrets, env var binding pattern |
| R3 | Kubernetes security standards | ✅ Complete | Pod Security Standards (restricted), non-root, read-only rootfs |
| R4 | golang-migrate integration | ✅ Complete | Install in production image, same image for broker and Job |

## Phase 1: Design Artifacts

| Artifact | Status | Description |
|----------|--------|-------------|
| [research.md](research.md) | ✅ Complete | Phase 0 research findings |
| [data-model.md](data-model.md) | ✅ Complete | values.yaml schema documentation |
| [quickstart.md](quickstart.md) | ✅ Complete | Deployment guide |

## Complexity Tracking

No Constitution violations identified. This is a straightforward infrastructure feature.
