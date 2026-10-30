# Quickstart: Deploying with Helm

This guide covers deploying the Agentic Identity Broker to Kubernetes using Helm.

## Prerequisites

- Kubernetes 1.25+
- Helm 3.x
- kubectl configured for your cluster

## Quick Start (In-Memory Storage)

Deploy with default settings for evaluation:

```bash
# Add the chart (if published to a repository)
# helm repo add agentic-identity-broker https://...

# Install from local chart
helm install broker ./charts/agentic-identity-broker

# Verify deployment
kubectl get pods -l app.kubernetes.io/name=agentic-identity-broker
```

The broker starts with in-memory storage (data lost on restart). This is suitable for evaluation only.

## Production Deployment

### Option 1: External PostgreSQL

Use an existing PostgreSQL database with separate migration and broker users.

**Prerequisites**:
1. PostgreSQL 12+ instance accessible from Kubernetes
2. Two database users created:
   - Migration user with `CREATE, ALTER, DROP` permissions (for schema migrations)
   - Broker user with `SELECT, INSERT, UPDATE, DELETE` permissions (for runtime)
3. Kubernetes Secrets containing credentials

**Create secrets**:
```bash
# Migration user secret (for schema migrations)
kubectl create secret generic broker-db-migration \
  --from-literal=username=broker-migration \
  --from-literal=password=<migration-password>

# Broker user secret (for runtime)
kubectl create secret generic broker-db \
  --from-literal=username=broker \
  --from-literal=password=<broker-password>
```

**Deploy**:
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

### Option 2: Zalando PostgreSQL Operator

Let the Zalando PostgreSQL Operator provision the database and users automatically.

**Prerequisites**:
- Zalando PostgreSQL Operator installed in cluster

**Deploy**:
```bash
helm install broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  --set postgresql.operator.teamId=my-team \
  --set postgresql.operator.volume.size=20Gi \
  --set postgresql.operator.numberOfInstances=2
```

The operator will:
1. Create a PostgreSQL cluster
2. Create migration and broker users
3. Create Kubernetes Secrets for each user
4. The Helm chart automatically references these secrets

## Ingress Configuration

### Nginx Ingress Controller

```yaml
# values-production.yaml
ingress:
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

```bash
helm install broker ./charts/agentic-identity-broker -f values-production.yaml
```

### Skipper Ingress Controller

```yaml
# values-skipper.yaml
ingress:
  enduser:
    enabled: true
    className: skipper
    annotations:
      zalando.org/skipper-filter: ratelimit(50, "1m")
      zalando.org/skipper-predicate: 'Path("/")'
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: ImplementationSpecific
```

## Generate Static Manifests

For environments where Helm cannot be used:

```bash
# Generate manifests with custom values
helm template broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  > manifests.yaml

# Apply to cluster
kubectl apply -f manifests.yaml
```

## Upgrading

```bash
# Upgrade with new values or chart version
helm upgrade broker ./charts/agentic-identity-broker \
  --set image.tag=v1.2.0

# The migration Job runs automatically before broker pods are updated
```

## Uninstalling

```bash
helm uninstall broker

# Note: If using Zalando PostgreSQL Operator, the PostgreSQL CR is NOT deleted
# by default (to prevent data loss). Delete manually if desired:
kubectl delete postgresql my-team-broker
```

## Troubleshooting

### Migration Job Failed

Check migration logs:
```bash
kubectl logs job/broker-migrate
```

Common issues:
- Database not reachable (check network policies, service DNS)
- Invalid credentials (check secret values)
- Migration files missing (verify Docker image contains /app/migrations/)

### Broker Pod CrashLoopBackOff

Check broker logs:
```bash
kubectl logs deployment/broker
```

Common issues:
- Database connection failed (verify broker secret, database host)
- Configuration error (check ConfigMap values)

### Zalando Operator Secrets Not Created

Check PostgreSQL CR status:
```bash
kubectl describe postgresql my-team-broker
```

The operator creates secrets after the PostgreSQL cluster is ready (may take 1-2 minutes).
