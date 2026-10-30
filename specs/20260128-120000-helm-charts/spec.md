# Feature Specification: Helm Charts for Kubernetes Deployment

**Feature Branch**: `20260128-120000-helm-charts`  
**Created**: 2026-01-28  
**Status**: Draft  
**Input**: User description: "I want to add consumable helm charts s.t. the broker can be deployed easily. For targets where Helm cannot be used, Kubernetes manifests can be generated. The Helm Chart should deploy the broker, adequate config maps, Ingress and Service resources. PostgresSQL support should be optionally configured, additionally the Zalando PostgreSQL operator can utilized by adding postgresql resources. For automatic database migrations, a Kubernetes Job should be run separate from the main broker but with the same Docker image. The Job should have permissions to run DDL, the broker should only have DML permissions."

## Clarifications

### Session 2026-01-28

- Q: Who creates the two PostgreSQL users (migration/broker) when using Zalando operator? → A: Zalando operator creates both users automatically via `postgresql.users` configuration
- Q: How are migration/broker users provided for external PostgreSQL? → A: Two separate existing secrets (one for migration user, one for broker user)
- Q: How should migration Job be triggered relative to broker Deployment? → A: Helm pre-install/pre-upgrade hook (Job runs before Deployment created/updated)
- Q: How should Skipper ingress controller be supported? → A: Document common Skipper annotations in values.yaml examples (operators copy/customize)
- Q: How should the dual-port broker (8000 end-user, 14000 admin) be exposed? → A: Single Service exposing both ports with different names, plus two separate Ingress resources
- Q: How should PostgreSQL credentials be passed to broker/migration Job? → A: Via environment variables; when using Zalando operator, automatically reference the operator-created secrets (e.g., `<team>.<username>.credentials.postgresql.acid.zalan.do`)

### Session 2026-01-29 - Docker Image Architecture Decision

**Design Change**: The original user requirement specified "same Docker image" for broker and migration Job. After security analysis, **ADR-20260129-120000** was created recommending separate images for defense-in-depth security:
- `agentic-identity-broker`: Runtime broker image (no migration tools)
- `agentic-identity-broker-migrate`: Migration-only image (golang-migrate + migrations)

**Rationale**: Removing migration tooling from runtime containers reduces attack surface if the broker is compromised, following the principle of least privilege. See [adrs/20260129-120000-separate-migration-docker-image.md](../../adrs/20260129-120000-separate-migration-docker-image.md) for full analysis.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Deploy Broker with Helm (Priority: P1)

As a platform operator, I want to deploy the Agentic Identity Broker to my Kubernetes cluster using Helm, so that I can quickly get a working broker instance with minimal configuration effort.

**Why this priority**: This is the core value proposition—enabling easy deployment of the broker. Without this, operators must manually create all Kubernetes resources, which is error-prone and time-consuming.

**Independent Test**: Can be fully tested by running `helm install` with default values and verifying the broker pod is running and accessible via the created Service.

**Acceptance Scenarios**:

1. **Given** a Kubernetes cluster with Helm installed, **When** the operator runs `helm install broker ./charts/agentic-identity-broker`, **Then** the broker Deployment, Service, and ConfigMap are created and the broker pod becomes ready within 2 minutes.
2. **Given** a successful Helm installation with defaults, **When** the operator queries the Service spec, **Then** the Service exposes port 8000 (named "http") and port 14000 (named "admin").
3. **Given** a successful Helm installation with defaults, **When** the operator accesses the broker Service endpoint, **Then** the broker responds with a healthy status.
4. **Given** a Helm installation, **When** the operator runs `helm upgrade` with modified values, **Then** the broker is updated with the new configuration without manual intervention.

---

### User Story 2 - Generate Static Kubernetes Manifests (Priority: P2)

As a platform operator in an environment where Helm cannot be used (e.g., GitOps with raw manifests, policy restrictions), I want to generate static Kubernetes manifests from the Helm chart, so that I can deploy the broker using kubectl or a GitOps tool.

**Why this priority**: Some environments prohibit Helm or prefer declarative manifests. This extends deployment reach without maintaining separate manifest files.

**Independent Test**: Can be fully tested by running `helm template` and applying the output with `kubectl apply`.

**Acceptance Scenarios**:

1. **Given** the Helm chart, **When** the operator runs `helm template broker ./charts/agentic-identity-broker > manifests.yaml`, **Then** valid Kubernetes manifests are generated that can be applied with `kubectl apply -f manifests.yaml`.
2. **Given** generated manifests with custom values, **When** the operator applies them to a cluster, **Then** the broker deploys identically to a Helm-based installation with the same values.

---

### User Story 3 - Configure Ingress for External Access (Priority: P2)

As a platform operator, I want to optionally configure separate Ingress resources for end-user and admin APIs through Helm values, so that I can expose each API with appropriate routing, access controls, and TLS termination.

**Why this priority**: Production deployments require external access with proper ingress configuration. Separating end-user and admin APIs allows different security postures (e.g., admin restricted to internal networks).

**Independent Test**: Can be tested by enabling ingress for each API and verifying the Ingress resources are created with correct annotations, hosts, and port routing.

**Acceptance Scenarios**:

1. **Given** Helm values with `ingress.enduser.enabled: true` and host configuration, **When** the chart is installed, **Then** an Ingress resource is created routing to the end-user API port (8000) with the specified host, paths, and TLS settings.
2. **Given** Helm values with `ingress.admin.enabled: true` and host configuration, **When** the chart is installed, **Then** a separate Ingress resource is created routing to the admin API port (14000).
3. **Given** Helm values with both ingresses disabled (default), **When** the chart is installed, **Then** no Ingress resources are created.
4. **Given** Helm values with custom ingress annotations (including Skipper-specific annotations), **When** the chart is installed, **Then** the Ingress resources include all specified annotations.
5. **Given** Helm values with `ingress.enduser.className: skipper`, **When** the chart is installed, **Then** the Ingress resource uses the Skipper ingress class.

---

### User Story 4 - Deploy with In-Memory Storage (Priority: P2)

As a platform operator evaluating the broker, I want to deploy without PostgreSQL dependencies, so that I can quickly test the broker functionality without setting up a database.

**Why this priority**: Enables quick evaluation and development use cases. Reduces barrier to entry for new users.

**Independent Test**: Can be tested by installing with storage type set to in-memory and verifying the broker starts without database connection errors.

**Acceptance Scenarios**:

1. **Given** Helm values with `storage.type: memory`, **When** the chart is installed, **Then** the broker starts successfully without any PostgreSQL resources or connection attempts.
2. **Given** an in-memory deployment, **When** the broker pod restarts, **Then** all data is cleared (expected behavior documented in notes).

---

### User Story 5 - Deploy with External PostgreSQL (Priority: P3)

As a platform operator with an existing PostgreSQL instance, I want to configure the broker to use my external database with separate migration and broker users, so that I can integrate with my existing database infrastructure while maintaining proper privilege separation.

**Why this priority**: Common enterprise scenario where databases are managed separately. Important for production but not required for initial deployment testing.

**Independent Test**: Can be tested by providing external PostgreSQL connection details with two separate secrets and verifying both migration Job and broker connect successfully with appropriate credentials.

**Acceptance Scenarios**:

1. **Given** Helm values with `storage.type: postgres` and external database connection details, **When** the chart is installed, **Then** the broker connects to the external PostgreSQL instance.
2. **Given** Helm values with `postgresql.external.migrationSecretName` specified, **When** the chart is installed, **Then** the migration Job uses credentials from that secret for schema operations.
3. **Given** Helm values with `postgresql.external.brokerSecretName` specified, **When** the chart is installed, **Then** the broker uses credentials from that secret for runtime operations.
4. **Given** Helm values specifying existing secret names for both migration and broker credentials, **When** the chart is installed, **Then** no new secrets are created and the existing secrets are mounted to the respective workloads.

---

### User Story 6 - Deploy with Zalando PostgreSQL Operator (Priority: P3)

As a platform operator using the Zalando PostgreSQL Operator, I want the Helm chart to create a `postgresql` custom resource with both migration and broker users configured, so that the operator provisions and manages the database with proper privilege separation.

**Why this priority**: Provides a complete, self-contained deployment option for teams using the Zalando operator. Reduces operational burden for database and user management.

**Independent Test**: Can be tested by enabling Zalando PostgreSQL and verifying the `postgresql` CR is created with both users in the `users` specification, and that workloads automatically reference the operator-created secrets.

**Acceptance Scenarios**:

1. **Given** Helm values with `postgresql.operator.enabled: true`, **When** the chart is installed, **Then** a `postgresql` custom resource is created for the Zalando operator.
2. **Given** the `postgresql` CR, **When** it is created, **Then** it includes both migration and broker users in the `users` configuration (e.g., `broker-migration` and `broker`).
3. **Given** a deployed Zalando-managed PostgreSQL, **When** the PostgreSQL operator provisions the database, **Then** it creates secrets following the naming convention `<username>.<team>-<db>.credentials.postgresql.acid.zalan.do`.
4. **Given** the operator-created secrets, **When** the migration Job starts, **Then** it mounts the migration user secret and injects credentials via environment variables (`DB_USERNAME`, `DB_PASSWORD`).
5. **Given** the operator-created secrets, **When** the broker Deployment starts, **Then** it mounts the broker user secret and injects credentials via environment variables.
6. **Given** Helm values with custom PostgreSQL sizing (storage, replicas), **When** the chart is installed, **Then** the `postgresql` CR reflects the specified sizing parameters.
7. **Given** custom user names via `postgresql.operator.users.migration` and `postgresql.operator.users.broker`, **When** the chart is installed, **Then** the PostgreSQL CR and workload configurations use the specified user names and derive the correct secret names.

---

### User Story 7 - Run Database Migrations Separately (Priority: P1)

As a platform operator following security best practices, I want database migrations to run as a separate Kubernetes Job with elevated permissions via Helm hooks, while the broker itself runs with minimal database permissions.

**Why this priority**: Critical for security—separating schema changes (migrations) from data operations (runtime) follows the principle of least privilege. Using Helm hooks ensures migrations complete before broker deployment.

**Independent Test**: Can be tested by verifying the migration Job runs as a pre-install/pre-upgrade hook, uses migration credentials, completes before broker pods start, and the broker pod uses only broker credentials.

**Acceptance Scenarios**:

1. **Given** a Helm installation with PostgreSQL configured, **When** the chart is installed, **Then** a Kubernetes Job with Helm `pre-install` hook runs golang-migrate before the broker Deployment is created.
2. **Given** the migration Job, **When** it executes, **Then** it uses the migration user credentials from the configured secret (with CREATE, ALTER, DROP permissions).
3. **Given** the broker Deployment, **When** it starts, **Then** it uses the broker user credentials from the configured secret (with only SELECT, INSERT, UPDATE, DELETE permissions).
4. **Given** a Helm upgrade, **When** the upgrade runs, **Then** the migration Job executes as a `pre-upgrade` hook, applying any new migrations before updating the broker pods.
5. **Given** the migration Job fails, **When** the Helm installation/upgrade is attempted, **Then** Helm aborts the release, the broker Deployment is not created/updated, and the failed Job pod remains for debugging (no automatic deletion).
6. **Given** the Zalando PostgreSQL Operator is used, **When** the chart is installed, **Then** both migration and broker users are created via `postgresql.users` configuration in the PostgreSQL CR.

---

### User Story 8 - Customize Resource Limits and Requests (Priority: P3)

As a platform operator, I want to configure CPU and memory resource limits for the broker, so that I can right-size the deployment for my workload and enforce resource quotas.

**Why this priority**: Important for production resource management but not required for basic deployment functionality.

**Independent Test**: Can be tested by setting custom resource values and verifying the Deployment spec reflects them.

**Acceptance Scenarios**:

1. **Given** Helm values with custom `resources.limits` and `resources.requests`, **When** the chart is installed, **Then** the broker pod has the specified resource constraints.
2. **Given** default Helm values, **When** the chart is installed, **Then** sensible default resource requests are applied.

---

### Edge Cases

- What happens when the Zalando PostgreSQL operator is not installed but `postgresql.operator.enabled: true`?
  - The `postgresql` CR will be created but remain pending; documentation should note the operator prerequisite.
- What happens when the migration Job cannot connect to the database?
  - The Job fails, retries based on `backoffLimit`, and if exhausted, the Helm installation fails. The broker Deployment should not start.
- What happens when external PostgreSQL credentials are invalid?
  - The broker pod fails health checks and enters CrashLoopBackOff. Clear error messages in logs should indicate credential issues.
- What happens when upgrading from in-memory to PostgreSQL storage?
  - Data from in-memory storage is lost; documentation should warn about this transition.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST provide a Helm chart that deploys the Agentic Identity Broker to Kubernetes.
- **FR-002**: Helm chart MUST create a Deployment resource for the broker application.
- **FR-003**: Helm chart MUST create a single Service resource exposing both ports (8000 for end-user API, 14000 for admin API) with named ports (`http`, `admin`).
- **FR-004**: Helm chart MUST create a ConfigMap for broker configuration.
- **FR-005**: Helm chart MUST support creating two optional Ingress resources (one for end-user API, one for admin API) with independent host/path/TLS configuration.
- **FR-006**: Helm chart MUST support in-memory storage mode (no PostgreSQL required).
- **FR-007**: Helm chart MUST support external PostgreSQL connection configuration with two separate secrets (migration user for migrations, broker user for runtime).
- **FR-008**: Helm chart MUST support creating a Zalando PostgreSQL Operator `postgresql` custom resource with two users defined via `postgresql.users` configuration.
- **FR-009**: Helm chart MUST create a Kubernetes Job for database migrations using golang-migrate when PostgreSQL is enabled.
- **FR-010**: Migration Job MUST run as a Helm pre-install/pre-upgrade hook, completing before broker Deployment is created or updated.
- **FR-011**: Migration Job MUST use database credentials with schema permissions (CREATE, ALTER, DROP, etc.).
- **FR-012**: Broker Deployment MUST use database credentials with only data permissions (SELECT, INSERT, UPDATE, DELETE).
- **FR-013**: System MUST use separate Docker images for security: `agentic-identity-broker` (runtime, no migration tools) and `agentic-identity-broker-migrate` (migration image with golang-migrate tool and migrations from `/migrations/` directory) per ADR-20260129-120000.
- **FR-014**: Helm chart MUST support generating static Kubernetes manifests via `helm template`.
- **FR-015**: Helm chart MUST follow Helm best practices (proper labels, annotations, helper templates).
- **FR-016**: Helm chart MUST include a `values.yaml` with sensible defaults and comprehensive documentation.
- **FR-017**: Helm chart MUST support specifying existing secrets for sensitive values (separate migration and broker database credentials).
- **FR-018**: Helm chart MUST support custom annotations and labels on all created resources.
- **FR-019**: Migration Job MUST use the `agentic-identity-broker-migrate` image while broker Deployment uses the `agentic-identity-broker` image to maintain separation of runtime and migration capabilities per ADR-20260129-120000.
- **FR-020**: Ingress configuration MUST support className and custom annotations, with documented examples for Skipper ingress controller.
- **FR-021**: Database credentials MUST be injected via environment variables from Kubernetes Secrets: `DB_USERNAME` and `DB_PASSWORD` (connection URL is constructed in Helm templates using values from `postgresql.external.*` or `postgresql.operator.*` configuration).
- **FR-022**: When using Zalando PostgreSQL Operator, Helm chart MUST automatically derive and reference the operator-created secret names following the convention `<username>.<teamId>-<database>.credentials.postgresql.acid.zalan.do` (example: `broker.broker-broker.credentials.postgresql.acid.zalan.do` for user "broker" with teamId "broker" and database "broker").
- **FR-023**: For external PostgreSQL deployments, operators MUST manually create both migration and broker database users before Helm installation and provide credentials via two separate Kubernetes Secrets (referenced by `postgresql.external.migrationSecretName` and `postgresql.external.brokerSecretName`).
- **FR-024**: When migration Job fails, Helm MUST abort the release without creating or updating the broker Deployment; failed Job pods MUST remain available for debugging (helm hook-delete-policy MUST NOT delete on failure).

### Configuration Requirements

**Configuration Parameters**:

| Parameter | Type | Purpose | Default |
|-----------|------|---------|---------|
| `image.repository` | string | Docker image repository | `agentic-identity-broker` |
| `image.tag` | string | Docker image tag | Chart appVersion |
| `image.pullPolicy` | string | Image pull policy | `IfNotPresent` |
| `migration.image.repository` | string | Migration Docker image repository | `agentic-identity-broker-migrate` |
| `migration.image.tag` | string | Migration image tag | Chart appVersion |
| `migration.image.pullPolicy` | string | Migration image pull policy | `IfNotPresent` |
| `replicaCount` | integer | Number of broker replicas | `1` |
| `service.type` | string | Kubernetes Service type | `ClusterIP` |
| `service.ports.http` | integer | End-user API port | `8000` |
| `service.ports.admin` | integer | Admin API port | `14000` |
| `ingress.enduser.enabled` | boolean | Create end-user Ingress resource | `false` |
| `ingress.enduser.className` | string | End-user Ingress class name | `""` |
| `ingress.enduser.annotations` | map | End-user Ingress annotations | `{}` |
| `ingress.enduser.hosts` | list | End-user Ingress host configurations | `[]` |
| `ingress.enduser.tls` | list | End-user Ingress TLS configurations | `[]` |
| `ingress.admin.enabled` | boolean | Create admin Ingress resource | `false` |
| `ingress.admin.className` | string | Admin Ingress class name | `""` |
| `ingress.admin.annotations` | map | Admin Ingress annotations | `{}` |
| `ingress.admin.hosts` | list | Admin Ingress host configurations | `[]` |
| `ingress.admin.tls` | list | Admin Ingress TLS configurations | `[]` |
| `storage.type` | string | Storage backend (memory/postgres) | `memory` |
| `postgresql.external.enabled` | boolean | Use external PostgreSQL | `false` |
| `postgresql.external.host` | string | External PostgreSQL host | `""` |
| `postgresql.external.port` | integer | External PostgreSQL port | `5432` |
| `postgresql.external.database` | string | Database name | `broker` |
| `postgresql.external.migrationSecretName` | string | Existing secret for migration user credentials | `""` |
| `postgresql.external.brokerSecretName` | string | Existing secret for broker user credentials | `""` |
| `postgresql.operator.enabled` | boolean | Create Zalando PostgreSQL CR | `false` |
| `postgresql.operator.teamId` | string | Zalando operator team ID | `broker` |
| `postgresql.operator.numberOfInstances` | integer | PostgreSQL replicas | `1` |
| `postgresql.operator.volume.size` | string | PostgreSQL storage size | `10Gi` |
| `postgresql.operator.users.migration` | string | Migration user name for schema changes | `broker-migration` |
| `postgresql.operator.users.broker` | string | Broker user name for runtime | `broker` |
| `migrations.enabled` | boolean | Run database migrations | `true` (when storage.type=postgres) |
| `migrations.backoffLimit` | integer | Job retry limit | `3` |
| `resources.requests.cpu` | string | CPU request | `100m` |
| `resources.requests.memory` | string | Memory request | `128Mi` |
| `resources.limits.cpu` | string | CPU limit | `500m` |
| `resources.limits.memory` | string | Memory limit | `256Mi` |

**Example values.yaml Configuration**:

```yaml
# Broker deployment configuration
replicaCount: 2

image:
  repository: ghcr.io/zalando-incubator/agentic-identity-broker
  tag: ""  # Defaults to chart appVersion
  pullPolicy: IfNotPresent

# Migration Job configuration (separate image per ADR-20260129-120000)
migration:
  enabled: true  # Auto-enabled when storage.type=postgres
  image:
    repository: ghcr.io/zalando-incubator/agentic-identity-broker-migrate
    tag: ""  # Defaults to chart appVersion
    pullPolicy: IfNotPresent
  backoffLimit: 10
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 200m
      memory: 128Mi

# Service configuration (single Service, dual ports)
service:
  type: ClusterIP
  ports:
    http: 8000   # End-user API
    admin: 14000 # Admin API
  annotations: {}

# Ingress configuration (separate Ingress per API)
ingress:
  # End-user API Ingress
  enduser:
    enabled: true
    className: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: broker-tls
        hosts:
          - broker.example.com
  
  # Admin API Ingress (typically internal-only or different host)
  admin:
    enabled: true
    className: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
      # Restrict to internal network
      nginx.ingress.kubernetes.io/whitelist-source-range: "10.0.0.0/8"
    hosts:
      - host: broker-admin.internal.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: broker-admin-tls
        hosts:
          - broker-admin.internal.example.com

# Example: Skipper Ingress Controller configuration
# ingress:
#   enduser:
#     enabled: true
#     className: skipper
#     annotations:
#       zalando.org/skipper-filter: ratelimit(50, "1m")
#       zalando.org/skipper-predicate: 'Path("/")'
#     hosts:
#       - host: broker.example.com
#         paths:
#           - path: /
#             pathType: ImplementationSpecific

# Storage configuration
storage:
  type: postgres  # memory | postgres

# PostgreSQL configuration (when storage.type=postgres)
postgresql:
  # Option 1: External PostgreSQL (requires pre-provisioned users)
  external:
    enabled: true
    host: postgres.database.svc.cluster.local
    port: 5432
    database: broker
    # Separate secrets for migration and broker users
    migrationSecretName: broker-db-migration-credentials  # Must contain 'username' and 'password' keys
    brokerSecretName: broker-db-credentials  # Must contain 'username' and 'password' keys
  
  # Option 2: Zalando PostgreSQL Operator (auto-provisions users)
  operator:
    enabled: false
    teamId: broker
    numberOfInstances: 2
    volume:
      size: 20Gi
      storageClass: standard
    users:
      migration: broker-migration  # User with schema permissions for migrations
      broker: broker  # User with data permissions for runtime

# Database migrations (using golang-migrate, runs as Helm pre-install/pre-upgrade hook)
migrations:
  enabled: true
  backoffLimit: 3

# Resource limits
resources:
  requests:
    cpu: 100m
    memory: 128Mi
  limits:
    cpu: 500m
    memory: 256Mi
```

**Configuration Location**: `charts/agentic-identity-broker/values.yaml`

### Security Requirements

- **SR-001**: Database credentials MUST be stored in Kubernetes Secrets, never in ConfigMaps or plain values.
- **SR-002**: Migration Job credentials and broker credentials MUST be separate database users.
- **SR-003**: Helm chart MUST support referencing existing secrets to avoid storing credentials in values files.
- **SR-004**: Default Security Context MUST run containers as non-root user.
- **SR-005**: Helm chart MUST support Pod Security Standards (restricted profile compatibility).
- **SR-006**: Ingress TLS configuration MUST be supported for encrypted external traffic.

### Key Entities

- **Helm Chart**: The packaged Kubernetes deployment templates with configurable values.
- **Migration Job**: A Kubernetes Job (Helm pre-install/pre-upgrade hook) that runs golang-migrate with schema permissions.
- **Broker Deployment**: The main application Deployment with data-only database permissions.
- **PostgreSQL CR**: A Zalando PostgreSQL Operator custom resource for managed database provisioning with dual users.
- **Migration User**: Database user with schema modification permissions (CREATE, ALTER, DROP) used exclusively by migration Job.
- **Broker User**: Database user with data manipulation permissions only (SELECT, INSERT, UPDATE, DELETE) used by broker runtime.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can deploy a functional broker to Kubernetes in under 5 minutes using Helm with default values.
- **SC-002**: Generated static manifests produce identical deployments to Helm installations with the same values.
- **SC-003**: Database migrations complete before broker pods accept traffic, ensuring schema consistency.
- **SC-004**: Broker operates with minimum database permissions (data operations only) in PostgreSQL deployments.
- **SC-005**: Chart passes `helm lint` validation with no errors or warnings.
- **SC-006**: All chart templates render successfully with `helm template` for all documented configuration combinations.
- **SC-007**: Documentation enables first-time users to deploy the broker without prior Helm expertise.

## Assumptions

- Kubernetes cluster version 1.25+ is used (for current API versions).
- Helm 3.x is available for Helm-based deployments.
- For Zalando PostgreSQL Operator integration, the operator is already installed in the cluster.
- A separate migration Docker image (`agentic-identity-broker-migrate`) will be created containing golang-migrate tool and migrations from `/migrations/` directory per ADR-20260129-120000; the runtime image remains minimal.
- PostgreSQL 12+ is used for database deployments.
- Standard ingress controllers (nginx, skipper, traefik, etc.) are supported via className and custom annotations.