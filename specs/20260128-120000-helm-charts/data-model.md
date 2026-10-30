# Data Model: Helm Chart values.yaml Schema

This document defines the values.yaml schema for the Agentic Identity Broker Helm chart.

## Top-Level Structure

```yaml
# Image configuration
image:
  repository: string          # Container image repository
  tag: string                 # Image tag (default: chart appVersion)
  pullPolicy: string          # IfNotPresent, Always, Never

# Replica and scaling
replicaCount: integer         # Number of broker pods
autoscaling:
  enabled: boolean
  minReplicas: integer
  maxReplicas: integer
  targetCPUUtilizationPercentage: integer

# Service configuration
service:
  type: string                # ClusterIP, NodePort, LoadBalancer
  ports:
    enduser: integer          # End-user port (default: 8000)
    admin: integer            # Admin port (default: 14000)

# Ingress configuration
ingress:
  enduser: IngressConfig      # End-user Ingress
  admin: IngressConfig        # Admin Ingress

# Storage configuration
storage:
  type: string                # "memory" or "postgres"

# PostgreSQL configuration
postgresql:
  external: ExternalPostgresConfig
  operator: OperatorPostgresConfig

# Security and RBAC
serviceAccount:
  create: boolean
  name: string
  annotations: map

# Pod configuration
podAnnotations: map
podSecurityContext: PodSecurityContext
securityContext: SecurityContext
resources: ResourceRequirements
nodeSelector: map
tolerations: []Toleration
affinity: Affinity
```

## Detailed Schema

### IngressConfig

```yaml
ingress:
  enduser:
    enabled: boolean            # Enable end-user Ingress
    className: string           # Ingress class (nginx, skipper, etc.)
    annotations: map            # Ingress annotations
    hosts:
      - host: string            # Hostname
        paths:
          - path: string        # URL path
            pathType: string    # Prefix, Exact, ImplementationSpecific
    tls:
      - secretName: string      # TLS secret name
        hosts:
          - string              # Hostnames for TLS
  
  admin:
    enabled: boolean            # Enable admin Ingress
    className: string           # Ingress class
    annotations: map            # Ingress annotations
    hosts: []HostConfig
    tls: []TLSConfig
```

### ExternalPostgresConfig

For existing PostgreSQL instances with pre-created secrets.

```yaml
postgresql:
  external:
    enabled: boolean            # Use external PostgreSQL
    host: string                # PostgreSQL host
    port: integer               # PostgreSQL port (default: 5432)
    database: string            # Database name
    sslMode: string             # disable, require, verify-ca, verify-full
    
    # Migration user (schema permissions for migrations)
    migrationSecretName: string       # Existing secret name
    migrationSecretKeys:
      username: string          # Key for username (default: "username")
      password: string          # Key for password (default: "password")
    
    # Broker user (runtime database access)
    brokerSecretName: string       # Existing secret name
    brokerSecretKeys:
      username: string          # Key for username (default: "username")
      password: string          # Key for password (default: "password")
```

### OperatorPostgresConfig

For Zalando PostgreSQL Operator managed instances.

```yaml
postgresql:
  operator:
    enabled: boolean            # Create PostgreSQL CR
    teamId: string              # Required: Zalando team ID
    version: string             # PostgreSQL version (default: "15")
    numberOfInstances: integer  # Cluster size (default: 2)
    
    volume:
      size: string              # Storage size (default: "10Gi")
      storageClass: string      # Optional storage class
    
    resources:
      requests:
        cpu: string
        memory: string
      limits:
        cpu: string
        memory: string
    
    # User configuration (auto-created by operator)
    # Note: Uses PostgreSQL role attributes, not SQL permissions
    users:
      migration:
        name: string            # Migration user name (default: "broker-migration")
        permissions:
          - superuser           # Superuser role for schema changes
          - createdb            # Database creation permission
      broker:
        name: string            # Broker user name (default: "broker")
        permissions:
          - login               # Login permission (database ownership grants table access)
```

### Broker Configuration

The broker's YAML configuration is managed via ConfigMap.

```yaml
broker:
  # Logging
  log:
    level: string               # debug, info, warn, error
    format: string              # json, text
  
  # Server configuration
  server:
    enduser:
      port: integer             # Must match service.ports.enduser
    admin:
      port: integer             # Must match service.ports.admin
  
  # OAuth2 configuration
  oauth2:
    issuer: string
    clients: []ClientConfig
  
  # Additional configuration (passed to ConfigMap)
  extraConfig: map              # Arbitrary additional config
```

### Migration Job

```yaml
migration:
  enabled: boolean              # Run migrations (default: true when postgres enabled)
  
  # Helm hook configuration
  hookWeight: integer           # Hook weight (default: -5)
  
  # Job configuration
  backoffLimit: integer         # Retry limit (default: 3)
  ttlSecondsAfterFinished: integer  # Cleanup delay (default: 300)
  
  # Resource limits
  resources:
    requests:
      cpu: string
      memory: string
    limits:
      cpu: string
      memory: string
```

### Security Context

Follows Kubernetes Pod Security Standards (restricted).

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 65534              # nobody
  runAsGroup: 65534
  fsGroup: 65534
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

## Default Values Summary

| Path | Default | Description |
|------|---------|-------------|
| `image.repository` | `ghcr.io/your-org/agentic-identity-broker` | Image repository |
| `image.tag` | Chart appVersion | Image tag |
| `image.pullPolicy` | `IfNotPresent` | Image pull policy |
| `replicaCount` | `1` | Number of replicas |
| `service.type` | `ClusterIP` | Service type |
| `service.ports.enduser` | `8000` | End-user port |
| `service.ports.admin` | `14000` | Admin port |
| `ingress.enduser.enabled` | `false` | End-user Ingress |
| `ingress.admin.enabled` | `false` | Admin Ingress |
| `storage.type` | `memory` | Storage type |
| `postgresql.external.enabled` | `false` | External PostgreSQL |
| `postgresql.operator.enabled` | `false` | Zalando operator |
| `migration.enabled` | auto | Enabled when PostgreSQL used |
| `autoscaling.enabled` | `false` | HPA disabled |

## Validation Rules

1. **Mutual Exclusion**: `postgresql.external.enabled` and `postgresql.operator.enabled` cannot both be `true`
2. **Required Fields**:
   - When `postgresql.external.enabled`: `host`, `database`, `migrationSecretName`, `brokerSecretName` required
   - When `postgresql.operator.enabled`: `teamId` required
3. **Port Consistency**: `broker.server.*.port` should match `service.ports.*`
4. **Ingress Requirements**: `ingress.*.hosts` required when `ingress.*.enabled`

## Secret Naming Convention

### External PostgreSQL
User-provided secrets with configurable key names.

### Zalando Operator
Auto-generated secrets follow operator naming convention:
```
<username>.<teamId>-<database>.credentials.postgresql.acid.zalan.do
```

Example for team `my-team`, database `broker`, user `broker`:
```
broker.my-team-broker.credentials.postgresql.acid.zalan.do
```

## Environment Variable Binding

The Helm chart templates generate environment variable references to secrets:

```yaml
# For broker Deployment
env:
  - name: DATABASE_USER
    valueFrom:
      secretKeyRef:
        name: {{ include "broker.brokerSecretName" . }}
        key: username
  - name: DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: {{ include "broker.brokerSecretName" . }}
        key: password

# For migration Job
env:
  - name: DATABASE_USER
    valueFrom:
      secretKeyRef:
        name: {{ include "broker.migrationSecretName" . }}
        key: username
  - name: DATABASE_PASSWORD
    valueFrom:
      secretKeyRef:
        name: {{ include "broker.migrationSecretName" . }}
        key: password
```
