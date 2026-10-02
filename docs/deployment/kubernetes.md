# Kubernetes Deployment Guide

This guide explains how to deploy the Agentic Identity Broker with Helm.

## Overview

The Helm chart supports these deployment patterns:

- **In-memory storage** for evaluation and testing
- **External PostgreSQL** for an existing production database
- **Zalando PostgreSQL Operator** for database provisioning
- **Dual Ingress** for separate end-user and admin API access
- **Automatic migrations** with separate database credentials
- **Static manifest generation** outside Helm

## Prerequisites

Before deployment, make sure that you have:

- A Kubernetes cluster version 1.25 or later
- `kubectl` configured for the cluster
- Helm 3.x
- The Zalando PostgreSQL Operator for managed databases, if you use it

## Quick Start

### 1. Deploy with in-memory storage for evaluation

Use this mode for evaluation and development:

```bash
# Install the chart
helm install broker ./charts/agentic-identity-broker

# Check deployment status
kubectl get pods -l app.kubernetes.io/name=agentic-identity-broker

# Access via port-forward
kubectl port-forward svc/broker-agentic-identity-broker 8000:8000

# Test health endpoint
curl http://localhost:8000/health
```

:::warning
In-memory storage loses data when a pod restarts. Do not use it in production.
:::

### 2. Deploy with External PostgreSQL (Production)

#### Prerequisites

- PostgreSQL 12+ instance accessible from Kubernetes
- Two database users:
  - **Migration user**: `CREATE`, `ALTER`, `DROP` permissions
  - **Broker user**: `SELECT`, `INSERT`, `UPDATE`, `DELETE` permissions

#### Step 1: Create Database Users

Connect to your PostgreSQL instance:

```sql
-- Create migration user (schema changes)
CREATE USER "broker-migration" WITH PASSWORD 'secure-migration-password';
GRANT ALL PRIVILEGES ON DATABASE broker TO "broker-migration";
GRANT ALL PRIVILEGES ON SCHEMA public TO "broker-migration";

-- Create broker user (data access only)
CREATE USER broker WITH PASSWORD 'secure-broker-password';
GRANT CONNECT ON DATABASE broker TO broker;
GRANT USAGE ON SCHEMA public TO broker;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO broker;
GRANT SELECT, UPDATE ON ALL SEQUENCES IN SCHEMA public TO broker;

-- Ensure future tables have correct permissions
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO broker;
ALTER DEFAULT PRIVILEGES IN SCHEMA public
  GRANT SELECT, UPDATE ON SEQUENCES TO broker;
```

#### Step 2: Create Kubernetes Secrets

```bash
# Migration user secret
kubectl create secret generic broker-db-migration \
  --from-literal=username=broker-migration \
  --from-literal=password=secure-migration-password

# Broker user secret
kubectl create secret generic broker-db \
  --from-literal=username=broker \
  --from-literal=password=secure-broker-password
```

#### Step 3: Deploy with Helm

```bash
helm install broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.external.enabled=true \
  --set postgresql.external.host=postgres.database.svc.cluster.local \
  --set postgresql.external.port=5432 \
  --set postgresql.external.database=broker \
  --set postgresql.external.migrationSecretName=broker-db-migration \
  --set postgresql.external.brokerSecretName=broker-db
```

#### Step 4: Examine deployment

```bash
# Check migration Job completed
kubectl get jobs
kubectl logs job/broker-migrate

# Check broker pod is running
kubectl get pods -l app.kubernetes.io/name=agentic-identity-broker
kubectl logs deployment/broker-agentic-identity-broker

# Run Helm tests
helm test broker
```

### 3. Deploy with Zalando PostgreSQL Operator

The Zalando PostgreSQL Operator automates database provisioning and user management.

#### Prerequisites

Install the Zalando PostgreSQL Operator:

```bash
# Add Helm repository
helm repo add postgres-operator-charts https://opensource.zalando.com/postgres-operator/charts/postgres-operator

# Install operator
helm install postgres-operator postgres-operator-charts/postgres-operator
```

#### Deploy Broker with Operator

```bash
helm install broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  --set postgresql.operator.teamId=my-team \
  --set postgresql.operator.numberOfInstances=2 \
  --set postgresql.operator.volume.size=20Gi
```

The operator does the following:
1. Creates a PostgreSQL cluster (`my-team-broker`).
2. Creates the migration user `broker-migration`.
3. Creates the broker user `broker`.
4. Generates Kubernetes Secrets.
5. Configures connection strings.

#### Examine operator status

```bash
# Check PostgreSQL cluster
kubectl get postgresql

# Check created secrets
kubectl get secrets | grep credentials.postgresql.acid.zalan.do

# Check PostgreSQL pods
kubectl get pods -l application=spilo
```

## Ingress Configuration

### Using Nginx Ingress Controller

Create a values file (`values-ingress.yaml`):

```yaml
ingress:
  enduser:
    enabled: true
    className: nginx
    annotations:
      cert-manager.io/cluster-issuer: letsencrypt-prod
      nginx.ingress.kubernetes.io/ssl-redirect: "true"
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: broker-tls
        hosts:
          - broker.example.com

  admin:
    enabled: true
    className: nginx
    annotations:
      # Restrict admin API to internal network
      nginx.ingress.kubernetes.io/whitelist-source-range: "10.0.0.0/8,172.16.0.0/12"
    hosts:
      - host: broker-admin.internal.example.com
        paths:
          - path: /
            pathType: Prefix
```

Deploy with Ingress:

```bash
helm install broker ./charts/agentic-identity-broker -f values-ingress.yaml
```

### Using Skipper Ingress Controller

For Zalando Kubernetes clusters:

```yaml
ingress:
  enduser:
    enabled: true
    className: skipper
    annotations:
      zalando.org/skipper-filter: |
        ratelimit(50, "1m")
        enableAccessLog(4,5)
      zalando.org/skipper-predicate: 'Path("/")'
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: ImplementationSpecific
```

## Production Configuration

### Recommended Production Values

Create `values-production.yaml`:

```yaml
# Replica count
replicaCount: 3

# Storage
storage:
  type: postgres

postgresql:
  operator:
    enabled: true
    teamId: production
    numberOfInstances: 3
    volume:
      size: 50Gi

# Resource limits
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1000m
    memory: 512Mi

# Autoscaling
autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70

# Pod disruption budget
podDisruptionBudget:
  enabled: true
  minAvailable: 2

# Ingress
ingress:
  enduser:
    enabled: true
    className: nginx
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
    tls:
      - secretName: broker-tls
        hosts:
          - broker.example.com

  admin:
    enabled: true
    className: nginx
    annotations:
      nginx.ingress.kubernetes.io/whitelist-source-range: "10.0.0.0/8"
    hosts:
      - host: broker-admin.internal.example.com
        paths:
          - path: /
            pathType: Prefix
```

Deploy:

```bash
helm install broker ./charts/agentic-identity-broker -f values-production.yaml
```

## Static Manifest Generation

For environments without Helm:

```bash
# Generate manifests
helm template broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  --set postgresql.operator.teamId=production \
  > manifests.yaml

# Review manifests
less manifests.yaml

# Apply to cluster
kubectl apply -f manifests.yaml
```

## Upgrading

### Upgrade Chart Version

```bash
# Update to new chart version
helm upgrade broker ./charts/agentic-identity-broker

# Upgrade with new values
helm upgrade broker ./charts/agentic-identity-broker -f values-production.yaml
```

### Upgrade Process

1. The migration Job starts as a Helm pre-upgrade hook.
2. The migration user updates the database schema.
3. Broker pods update one at a time.
4. Health checks keep the service available during the upgrade.

### Rollback

If upgrade fails:

```bash
# Rollback to previous release
helm rollback broker

# View release history
helm history broker
```

## Monitoring and Observability

### Health Checks

The broker exposes health endpoints:

- **Liveness**: `GET /health` on port 8000
- **Readiness**: `GET /health` on port 8000

### Logs

```bash
# Broker logs
kubectl logs -l app.kubernetes.io/name=agentic-identity-broker -f

# Migration logs
kubectl logs job/broker-migrate
```

### Metrics

Metrics endpoint (if enabled):

```bash
kubectl port-forward svc/broker-agentic-identity-broker 14000:14000
curl http://localhost:14000/metrics
```

## Security Best Practices

### 1. Separate Migration and Runtime Images

The Helm chart uses two separate Docker images for enhanced security:

| Image | Purpose | Contains | Security Posture |
|-------|---------|----------|------------------|
| `agentic-identity-broker` | Runtime broker | Application binary, frontend assets | Minimal - no SQL migration capabilities |
| `agentic-identity-broker-migrate` | Database migrations | golang-migrate tool, migration files | Schema permissions - isolated to Job |

**Security Benefits**:

- **Defense in Depth**: Runtime containers cannot execute arbitrary SQL migrations, even if compromised
- **Least Privilege**: Migration tooling only present in short-lived Job pods with schema permissions
- **Attack Surface Reduction**: Production broker image contains only what's needed to run the application
- **Clear Boundaries**: Explicit separation between runtime and schema modification capabilities

**Configuration**:

By default, both images use the same version tag (`appVersion` from the chart):

```yaml
# Default configuration (automatically set)
image:
  repository: agentic-identity-broker
  tag: ""  # Uses chart appVersion

migration:
  image:
    repository: agentic-identity-broker-migrate
    tag: ""  # Uses chart appVersion
```

To use custom image repositories:

```yaml
image:
  repository: ghcr.io/your-org/agentic-identity-broker
  tag: "v1.2.3"

migration:
  image:
    repository: ghcr.io/your-org/agentic-identity-broker-migrate
    tag: "v1.2.3"
```

### 2. Network Policies

Restrict network access:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: broker-netpol
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentic-identity-broker
  policyTypes:
    - Ingress
    - Egress
  ingress:
    - from:
        - namespaceSelector:
            matchLabels:
              name: ingress-nginx
  egress:
    - to:
        - podSelector:
            matchLabels:
              application: spilo  # PostgreSQL pods
      ports:
        - protocol: TCP
          port: 5432
```

### 3. Pod Security Standards

The chart enforces restricted Pod Security Standards by default:

- Non-root user (UID 1000)
- Read-only root filesystem
- No privilege escalation
- All capabilities dropped
- Seccomp profile applied

### 4. Secret Management

Use external secret management:

```yaml
# Example: External Secrets Operator
apiVersion: external-secrets.io/v1beta1
kind: ExternalSecret
metadata:
  name: broker-db-migration
spec:
  secretStoreRef:
    name: vault-backend
  target:
    name: broker-db-migration
  data:
    - secretKey: username
      remoteRef:
        key: /broker/db/migration
        property: username
    - secretKey: password
      remoteRef:
        key: /broker/db/migration
        property: password
```

## Local Refresh Sessions: Rollout and Restore

These rules apply to broker-issued refresh sessions in local mode and the local path of hybrid mode. Upstream sessions remain unchanged.

### Policy and Readiness

The Helm values are `broker.oauth2AuthorizationServer.local.refreshTokenReuseInterval`, `absoluteSessionLifetime`, and `refreshTokenTtl`. Their defaults are `30s`, `0s`, and `720h`.

Only fresh successful rotation renews inactivity. A policy increase affects the next fresh successor, not an issued token or its current interval.

Startup persists shorter effective deadlines and clears overdue retry results before readiness. Reuse `0s` clears every cached retry result before admission.

With reachable storage, maintenance erases expired live retry ciphertext within one second. Unavailable storage or missed cleanup blocks readiness until recovery.

Encrypted backups can retain ciphertext after live erasure. Existing access JWTs also retain their issued expiry, up to `token_ttl`.

The memory backend supports one instance and loses refresh history on restart. Durable replicas require shared PostgreSQL, encryption keys, and policy.

### Upgrade and Binary-Only Rollback

Every pre-feature unanchored session requires fresh authorization. Only complete anchored new-issuer families can continue under current consent.

1. Retain migrations 036 and 037 and the legacy `refresh_token_sessions` table during mixed-version rollout.
2. Before binary-only rollback, stop token traffic and all old writers.
3. Start the outgoing broker image with its outgoing policy and no token traffic.
4. Wait for successful startup reconciliation and ready health.
5. Stop the outgoing instance before you admit the old binary.

If reconciliation fails, keep token traffic stopped. Neither schema-preserving rollback nor later grants can restore terminal sessions.

Migration 037 verifies complete ancestry once and maintains the indexed lineage proof for later refreshes.
Its down migration invalidates anchored legacy authority before removing that proof. Reapplication does not restore those sessions.

### Restore Admission

1. Stop token traffic and all writers before restoration.
2. Restore the database or refresh-state snapshot with the supported schema.
3. Run the offline command with the deployment configuration and restored database access:

```bash
agentic-identity-broker --config <broker-config.yaml> refresh-sessions invalidate-restored
```

4. If the command fails or its commit is indeterminate, keep brokers offline.
5. Rerun the idempotent command until it exits successfully.
6. Admit token traffic only after acknowledged invalidation and the final empty-authority scan.

The command starts no HTTP server. It invalidates active roots, retry ciphertext, and unused legacy rows, including rootless unanchored rows.

Existing terminal reasons, grants, agents, credentials, signing keys, and third-party sessions remain unchanged. Fresh authorization can create a new local session.

See [the refresh policy reference](../configuration.md#local-refresh-session-policy) for duration relations and detailed backup limits.

## Troubleshooting

### Migration Job Failed

**Symptoms**: Migration Job shows status `Failed` or `Error`

**Diagnosis**:
```bash
kubectl logs job/broker-migrate
kubectl describe job/broker-migrate
```

**Common Causes**:
- Database not reachable (network policy, DNS)
- Invalid credentials in migration secret
- Insufficient permissions for migration user

### Broker Pod CrashLoopBackOff

**Symptoms**: Broker pod repeatedly restarts

**Diagnosis**:
```bash
kubectl logs deployment/broker-agentic-identity-broker
kubectl describe pod <pod-name>
```

**Common causes:**
- The broker cannot connect to the database. Examine the broker Secret.
- A ConfigMap has a configuration error.
- A required environment variable is missing.

### Zalando Operator Issues

**Symptoms**: PostgreSQL CR created but no pods appear

**Diagnosis**:
```bash
kubectl describe postgresql <teamId>-broker
kubectl logs -n postgres-operator deployment/postgres-operator
```

**Common Causes**:
- Operator not installed or not running
- Insufficient storage quota
- Invalid team ID or configuration

## Uninstalling

```bash
# Uninstall broker
helm uninstall broker

# (Optional) Delete PostgreSQL cluster
kubectl delete postgresql <teamId>-broker

# (Optional) Delete PVCs
kubectl delete pvc -l application=spilo
```

## Next Steps

- Configure OAuth2 settings
- Set up monitoring and alerts
- Prepare a production deployment checklist
- Define backup and restore procedures
