# Tasks: Helm Charts for Kubernetes Deployment

**Input**: Design documents from `/specs/20260128-120000-helm-charts/`
**Prerequisites**: plan.md ✅, spec.md ✅, research.md ✅, data-model.md ✅, quickstart.md ✅

**Tests**: This is an infrastructure feature. Validation uses Helm linting, template rendering, and Kind cluster deployment rather than Go E2E tests.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story?] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

---

## Phase 1: Setup (Chart Foundation)

**Purpose**: Create Helm chart structure and foundational files

- [X] T001 Create chart directory structure at charts/agentic-identity-broker/
- [X] T002 [P] Create Chart.yaml with metadata (name, version, appVersion, description) in charts/agentic-identity-broker/Chart.yaml
- [X] T003 [P] Create .helmignore file in charts/agentic-identity-broker/.helmignore
- [X] T004 Create _helpers.tpl with common template helpers in charts/agentic-identity-broker/templates/_helpers.tpl
- [X] T005 [P] Create NOTES.txt with post-install instructions in charts/agentic-identity-broker/templates/NOTES.txt

---

## Phase 2: Design Preconditions (Infrastructure Feature Adaptations)

**Purpose**: Verify infrastructure-specific design requirements are met

**⚠️ Note**: This is an infrastructure feature. Standard Phase 2 sections are adapted accordingly.

**E2E Acceptance Testing**: Per Principle XIII, infrastructure features require infrastructure-equivalent E2E testing. Kind cluster validation script (scripts/validate-helm-chart.sh) serves as E2E acceptance testing, validating complete deployment integration against all user story scenarios.

### Phase 2a: Configuration Design ✅

**Status**: Complete - values.yaml schema documented in [data-model.md](data-model.md)

- [x] T006 Configuration schema documented in data-model.md
- [x] T007 Example YAML snippets provided in spec.md

### Phase 2b: No API/Domain Changes

**Status**: N/A - Infrastructure feature, no new APIs or domain entities

### Phase 2b.1: Architecture Decision Record

**Required**: Document Docker image changes per Principle II

- [X] T007a Create ADR documenting Docker image architecture (adrs/20260129-120000-separate-migration-docker-image.md) covering: separate images for security, golang-migrate installation method, version selection, migration vs broker image patterns, security implications

### Phase 2c: Docker Image Design

**Required**: Docker image must include golang-migrate for migration Job

- [X] T008 Update Dockerfile to install golang-migrate tool in /usr/local/bin/migrate
- [X] T009 [P] Ensure migrations from /migrations/ are copied to Docker image at /app/migrations/
- [X] T010 [P] Verify Docker image can run as both broker (default entrypoint) and migrate (command override)

### Phase 2d: Validation Strategy Design

**Required**: Kind cluster validation instead of E2E tests

- [X] T011 Create scripts/validate-helm-chart.sh with Kind cluster validation logic
- [X] T012 [P] Add justfile targets for helm-lint and helm-validate

**Checkpoint**: Foundation ready for user story implementation

---

## Phase 2.5: Foundational Templates (Core Resources)

**Purpose**: Create templates shared by all user stories

**⚠️ CRITICAL**: These templates must be complete before user story work begins

- [X] T013 Create values.yaml with all configuration parameters in charts/agentic-identity-broker/values.yaml
- [X] T014 Create serviceaccount.yaml template in charts/agentic-identity-broker/templates/serviceaccount.yaml
- [X] T015 [P] Create configmap.yaml template for broker configuration in charts/agentic-identity-broker/templates/configmap.yaml
- [X] T016 Create deployment.yaml template for broker in charts/agentic-identity-broker/templates/deployment.yaml
- [X] T017 [P] Create service.yaml template with dual ports (8000, 14000) in charts/agentic-identity-broker/templates/service.yaml

**Checkpoint**: Foundation templates ready - user story implementation can begin

---

## Phase 3: User Story 1 - Deploy Broker with Helm (Priority: P1) 🎯 MVP

**Goal**: Enable basic Helm deployment with in-memory storage

**Independent Test**: `helm install broker ./charts/agentic-identity-broker` creates running broker pod

### Validation for User Story 1

- [X] T018 [US1] Validate chart with `helm lint charts/agentic-identity-broker`
- [X] T019 [US1] Validate template rendering with `helm template broker ./charts/agentic-identity-broker`
- [X] T020 [US1] Deploy to Kind cluster and verify broker pod is running

### Implementation for User Story 1

- [X] T021 [P] [US1] Add standard Kubernetes labels to _helpers.tpl (app.kubernetes.io/*)
- [X] T022 [P] [US1] Add resource limits/requests to deployment.yaml
- [X] T023 [P] [US1] Add Pod Security Context (runAsNonRoot, seccompProfile) to deployment.yaml
- [X] T024 [P] [US1] Add container Security Context (readOnlyRootFilesystem, capabilities drop) to deployment.yaml
- [X] T025 [US1] Add health check probes (liveness, readiness) to deployment.yaml
- [X] T026 [US1] Create test-connection.yaml Helm test in charts/agentic-identity-broker/tests/test-connection.yaml

**Checkpoint**: User Story 1 complete - basic Helm deployment works

---

## Phase 4: User Story 7 - Run Database Migrations Separately (Priority: P1)

**Goal**: Migration Job runs as Helm hook with schema permissions before broker starts

**Independent Test**: With PostgreSQL configured, migration Job completes successfully before broker Deployment

### Validation for User Story 7

- [X] T027 [US7] Validate job-migrate.yaml template renders correctly with PostgreSQL enabled
- [X] T028 [US7] Verify Helm hook annotations (pre-install, pre-upgrade) are present

### Implementation for User Story 7

- [X] T029 [P] [US7] Create job-migrate.yaml template with Helm hook annotations in charts/agentic-identity-broker/templates/job-migrate.yaml
- [X] T030 [P] [US7] Add migration user secret reference to job-migrate.yaml environment variables
- [X] T031 [US7] Add conditional rendering (only when storage.type=postgres) to job-migrate.yaml
- [X] T032 [US7] Add hook-delete-policy annotation (before-hook-creation) to job-migrate.yaml
- [X] T033 [P] [US7] Configure Job backoffLimit and ttlSecondsAfterFinished from values

**Checkpoint**: User Story 7 complete - migrations run before broker deployment

---

## Phase 5: User Story 2 - Generate Static Kubernetes Manifests (Priority: P2)

**Goal**: `helm template` produces valid, deployable manifests

**Independent Test**: `helm template broker ./charts/agentic-identity-broker | kubectl apply --dry-run=server -f -`

### Validation for User Story 2

- [X] T034 [US2] Generate manifests with helm template and validate with kubectl dry-run
- [X] T035 [US2] Verify all resources have consistent labels and selectors

### Implementation for User Story 2

- [X] T036 [P] [US2] Ensure all templates use consistent naming via _helpers.tpl
- [X] T037 [P] [US2] Add release name prefix to all resource names for uniqueness
- [X] T038 [US2] Document static manifest generation in charts/agentic-identity-broker/README.md

**Checkpoint**: User Story 2 complete - static manifests work with kubectl apply

---

## Phase 6: User Story 3 - Configure Ingress for External Access (Priority: P2)

**Goal**: Separate Ingress resources for end-user and admin APIs

**Independent Test**: With ingress enabled, two Ingress resources are created with correct port routing

### Validation for User Story 3

- [X] T039 [US3] Validate ingress templates render with various className values (nginx, skipper)
- [X] T040 [US3] Verify TLS configuration renders correctly

### Implementation for User Story 3

- [X] T041 [P] [US3] Create ingress-enduser.yaml template in charts/agentic-identity-broker/templates/ingress-enduser.yaml
- [X] T042 [P] [US3] Create ingress-admin.yaml template in charts/agentic-identity-broker/templates/ingress-admin.yaml
- [X] T043 [US3] Add conditional rendering based on ingress.enduser.enabled and ingress.admin.enabled
- [X] T044 [P] [US3] Add className support to both Ingress templates
- [X] T045 [P] [US3] Add custom annotations support to both Ingress templates
- [X] T046 [US3] Add TLS configuration support to both Ingress templates
- [X] T047 [P] [US3] Document Skipper ingress examples in values.yaml comments

**Checkpoint**: User Story 3 complete - Ingress resources created for external access

---

## Phase 7: User Story 4 - Deploy with In-Memory Storage (Priority: P2)

**Goal**: Default deployment works without PostgreSQL dependencies

**Independent Test**: With default values (storage.type=memory), broker starts without DB errors

### Validation for User Story 4

- [X] T048 [US4] Verify no PostgreSQL resources created when storage.type=memory
- [X] T049 [US4] Verify migration Job is NOT created when storage.type=memory

### Implementation for User Story 4

- [X] T050 [US4] Add storage.type condition to deployment.yaml for database environment variables
- [X] T051 [US4] Ensure values.yaml defaults to storage.type=memory

**Checkpoint**: User Story 4 complete - in-memory deployment works out of the box

---

## Phase 8: User Story 5 - Deploy with External PostgreSQL (Priority: P3)

**Goal**: Connect to existing PostgreSQL with separate migration/broker secrets

**Independent Test**: With external PostgreSQL config, broker uses brokerSecretName and migration uses migrationSecretName

### Validation for User Story 5

- [X] T052 [US5] Validate environment variables reference correct secret names
- [X] T053 [US5] Verify no PostgreSQL CR is created when external.enabled=true

### Implementation for User Story 5

- [X] T054 [P] [US5] Add external PostgreSQL host/port/database environment variables to deployment.yaml
- [X] T055 [P] [US5] Add migrationSecretName environment variable reference to job-migrate.yaml
- [X] T056 [P] [US5] Add brokerSecretName environment variable reference to deployment.yaml
- [X] T057 [US5] Add helper function for secret name resolution in _helpers.tpl

**Checkpoint**: User Story 5 complete - external PostgreSQL connection works

---

## Phase 9: User Story 6 - Deploy with Zalando PostgreSQL Operator (Priority: P3)

**Goal**: Create PostgreSQL CR with migration and broker users

**Independent Test**: With operator.enabled=true, postgresql CR is created with both users in users configuration

### Validation for User Story 6

- [X] T058 [US6] Validate postgresql.yaml template renders correct CR structure
- [X] T059 [US6] Verify operator secret naming convention in environment variable references

### Implementation for User Story 6

- [X] T060 [P] [US6] Create postgresql.yaml template for Zalando operator CR in charts/agentic-identity-broker/templates/postgresql.yaml
- [X] T061 [US6] Add users configuration (migration and broker) to postgresql.yaml
- [X] T062 [P] [US6] Add volume sizing and numberOfInstances to postgresql.yaml
- [X] T063 [US6] Add helper function for operator secret name derivation in _helpers.tpl
- [X] T064 [US6] Update deployment.yaml to reference operator-created secrets when operator.enabled=true
- [X] T065 [US6] Update job-migrate.yaml to reference operator-created secrets when operator.enabled=true

**Checkpoint**: User Story 6 complete - Zalando operator integration works

---

## Phase 10: User Story 8 - Customize Resource Limits and Requests (Priority: P3)

**Goal**: Configurable CPU/memory resource constraints

**Independent Test**: Custom resources.requests/limits values appear in Deployment spec

### Validation for User Story 8

- [X] T066 [US8] Validate custom resource values render in deployment.yaml
- [X] T067 [US8] Verify default resource values are sensible

### Implementation for User Story 8

- [X] T068 [P] [US8] Ensure resources block in deployment.yaml uses values.yaml configuration
- [X] T069 [P] [US8] Add HPA template in charts/agentic-identity-broker/templates/hpa.yaml (optional, disabled by default)
- [X] T070 [P] [US8] Add PDB template in charts/agentic-identity-broker/templates/pdb.yaml (optional, disabled by default)

**Checkpoint**: User Story 8 complete - resource customization works

---

## Phase 11: Documentation & Polish

**Purpose**: Complete documentation and final validation

### Documentation

- [X] T071 [P] Create comprehensive README.md for Helm chart in charts/agentic-identity-broker/README.md
- [X] T072 [P] Create docs/deployment/kubernetes.md with deployment guide
- [X] T073 Update NOTES.txt with all configuration options and next steps

### Final Validation

- [X] T074 Run full Kind cluster validation with scripts/validate-helm-chart.sh
- [X] T075 [P] Verify helm lint passes with no warnings
- [X] T076 [P] Verify all template combinations render successfully
- [X] T077 Run helm template with all major configuration combinations

### Extended PostgreSQL Validation

- [X] T082 Add Zalando PostgreSQL Operator installation to validation script
- [X] T083 Add PostgreSQL cluster deployment test with operator
- [X] T084 Add migration Job execution verification with real database
- [X] T085 Add broker deployment verification with PostgreSQL backend
- [X] T086 Add health endpoint test with PostgreSQL backend

### Constitution Compliance (Infrastructure Adaptations)

- [X] T078 Verify Pod Security Standards (restricted) compliance in all templates
- [X] T079 [P] Verify security context blocks are present in all pod specs
- [X] T080 [P] Verify no hardcoded secrets in any template
- [X] T081 Verify configuration documentation in values.yaml is comprehensive

---

## Dependencies & Execution Order

### Phase Dependencies

```
Phase 1 (Setup)
    ↓
Phase 2 (Design/Docker)
    ↓
Phase 2.5 (Foundational Templates) ──────────────────────┐
    ↓                                                     │
Phase 3 (US1: Basic Deploy) ← MVP                        │
    ↓                                                     │
Phase 4 (US7: Migrations) ← Required for PostgreSQL      │
    ↓                                                     │
┌───┴───────────────────────────────────┐                │
│                                       │                │
Phase 5 (US2: Static)   Phase 6 (US3: Ingress)          │
        │                       │                        │
        └───────────┬───────────┘                        │
                    ↓                                     │
            Phase 7 (US4: In-Memory)                     │
                    ↓                                     │
┌───────────────────┴───────────────────┐                │
│                                       │                │
Phase 8 (US5: External PG)  Phase 9 (US6: Zalando)      │
        │                       │                        │
        └───────────┬───────────┘                        │
                    ↓                                     │
            Phase 10 (US8: Resources) ←──────────────────┘
                    ↓
            Phase 11 (Docs & Polish)
```

### Parallel Opportunities

**Phase 1**: T002, T003, T005 can run in parallel
**Phase 2**: T008, T009, T010 can run in parallel after T008
**Phase 2.5**: T014, T015, T017 can run in parallel after T013
**Each User Story**: Test templates marked [P] can run in parallel

---

## Implementation Strategy

### MVP First (User Story 1 + 7)

1. Complete Phase 1: Setup
2. Complete Phase 2: Docker modifications
3. Complete Phase 2.5: Foundational templates
4. Complete Phase 3: User Story 1 (Basic Deploy)
5. Complete Phase 4: User Story 7 (Migrations)
6. **STOP and VALIDATE**: Deploy to Kind cluster with in-memory storage

### Incremental Delivery

1. **MVP**: Basic deploy + migrations → Kind validation
2. **+Static Manifests**: helm template works → kubectl apply validation
3. **+Ingress**: External access → routing validation
4. **+PostgreSQL Options**: External + Zalando → database validation
5. **+Polish**: Resource limits, docs → final validation

---

## Summary

| Phase | Tasks | Priority | Description |
|-------|-------|----------|-------------|
| 1 | T001-T005 | Setup | Chart structure and foundation |
| 2 | T006-T012 | Design | Docker modifications, validation script |
| 2.5 | T013-T017 | Foundation | Core templates (values, deployment, service) |
| 3 | T018-T026 | P1/MVP | US1: Basic Helm deployment |
| 4 | T027-T033 | P1 | US7: Migration Job with Helm hooks |
| 5 | T034-T038 | P2 | US2: Static manifest generation |
| 6 | T039-T047 | P2 | US3: Ingress configuration |
| 7 | T048-T051 | P2 | US4: In-memory storage |
| 8 | T052-T057 | P3 | US5: External PostgreSQL |
| 9 | T058-T065 | P3 | US6: Zalando operator integration |
| 10 | T066-T070 | P3 | US8: Resource customization |
| 11 | T071-T081 | Polish | Documentation and final validation |

**Total Tasks**: 81
**P1 (MVP)**: 26 tasks (Phases 1-4)
**P2**: 22 tasks (Phases 5-7)
**P3**: 22 tasks (Phases 8-10)
**Polish**: 11 tasks (Phase 11)
