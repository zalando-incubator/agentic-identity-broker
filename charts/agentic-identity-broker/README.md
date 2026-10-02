# Agentic Identity Broker Helm Chart

Official Helm chart for deploying the Agentic Identity Broker on Kubernetes.

## Features

- 🚀 **Simple deployment** with sensible defaults
- 🔐 **Flexible storage** options: in-memory, external PostgreSQL, or Zalando PostgreSQL Operator
- 🔄 **Automatic migrations** via Helm hooks with separate credentials
- 🌐 **Dual Ingress** support for end-user and admin APIs
- 📊 **Production-ready** with Pod Security Standards, resource limits, and health checks
- 📦 **Static manifests** generation via `helm template`
- ⚙️ **Highly configurable** with comprehensive values.yaml
- 🔭 **OpenTelemetry** support for distributed tracing and metrics via OTLP

## Prerequisites

- Kubernetes 1.25+
- Helm 3.x
- (Optional) Zalando PostgreSQL Operator for managed database provisioning

## Quick Start

### Install with In-Memory Storage

For evaluation and testing:

```bash
helm install broker ./charts/agentic-identity-broker
```

⚠️ **Warning**: In-memory storage is ephemeral. Data is lost on pod restart. Use PostgreSQL for production.

### Install with External PostgreSQL

For production deployments with an existing PostgreSQL database:

1. Create Kubernetes Secrets for database credentials:

```bash
# Migration user (schema permissions)
kubectl create secret generic broker-db-migration \
  --from-literal=username=broker-migration \
  --from-literal=password=<migration-password>

# Broker user (data access)
kubectl create secret generic broker-db \
  --from-literal=username=broker \
  --from-literal=password=<broker-password>
```

2. Install the chart:

```bash
helm install broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.external.enabled=true \
  --set postgresql.external.host=postgres.database.svc.cluster.local \
  --set postgresql.external.database=broker \
  --set postgresql.external.migrationSecretName=broker-db-migration \
  --set postgresql.external.brokerSecretName=broker-db
```

### Install with Zalando PostgreSQL Operator

For automatic PostgreSQL provisioning:

```bash
helm install broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  --set postgresql.operator.teamId=my-team
```

The operator will:
- Create a PostgreSQL cluster with 2 instances
- Create migration and broker users
- Generate Kubernetes Secrets automatically
- Configure the broker to use the operator-managed database

### Install with AWS IRSA (IAM Roles for Service Accounts)

For AWS EKS deployments using IAM roles for KMS encryption:

1. Deploy the CDK encryption infrastructure stack (see [Kubernetes IRSA Deployment Guide](/docs/deployment/kubernetes-irsa.md))

2. Extract the IAM role ARN from CloudFormation stack outputs:

```bash
aws cloudformation describe-stacks \
  --stack-name IdentityBrokerEncryption-prod \
  --query 'Stacks[0].Outputs[?OutputKey==`EncryptionRoleARN`].OutputValue' \
  --output text
```

3. Install the chart with IRSA enabled:

```bash
helm install broker ./charts/agentic-identity-broker \
  --set serviceAccount.irsa.enabled=true \
  --set serviceAccount.irsa.role="arn:aws:iam::123456789012:role/IdentityBrokerEncryptionRole-prod"
```

The ServiceAccount will be automatically annotated with `iam.amazonaws.com/role`, enabling the broker pods to assume the IAM role and access KMS/DynamoDB without credentials.

## Configuration

See [values.yaml](values.yaml) for the complete list of configuration options.

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of broker pods | `1` |
| `image.repository` | Broker container image repository | `agentic-identity-broker` |
| `image.tag` | Broker image tag | Chart appVersion |
| `broker.extraEnv` | Additional broker environment variables (`value` or `valueFrom`) | `[]` |
| `migration.image.repository` | Migration container image repository | `agentic-identity-broker-migrate` |
| `migration.image.tag` | Migration image tag | Chart appVersion |
| `storage.type` | Storage backend (`memory` or `postgres`) | `memory` |
| `serviceAccount.irsa.enabled` | Enable AWS IRSA annotation | `false` |
| `serviceAccount.irsa.role` | IAM role ARN for IRSA | `""` |
| `ingress.enduser.enabled` | Enable Ingress for end-user API | `false` |
| `ingress.admin.enabled` | Enable Ingress for admin API | `false` |
| `resources.requests.cpu` | CPU request | `100m` |
| `resources.requests.memory` | Memory request | `128Mi` |
| `commonLabels` | Labels applied to all chart resources | `{}` |
| `namespace.create` | Create Namespace resource | `false` |
| `namespace.name` | Namespace name override (applied to all resources) | `""` |
| `postgresql.labels` | Labels applied only to the PostgreSQL resource (overrides `commonLabels`) | `{}` |
| `postgresql.operator.initContainerResources` | Resources for wait-for-postgres init container | `{requests: {cpu: 50m, memory: 64Mi}, limits: {cpu: 50m, memory: 64Mi}}` |
| `postgresql.operator.secretSuffix` | Operator credentials secret suffix | `postgresql.acid.zalan.do` |
| `broker.thirdPartyOauth2.jweSigningKeySecret` | Secret ref for JWE signing key | `{name: "", key: signing-key}` |
| `broker.thirdPartyOauth2.jweSigningKeyBase64` | Base64-encoded JWE signing key (highest precedence) | `""` |
| `broker.encryption.memory.rawKey` | Base64-encoded memory encryption key | `""` |
| `broker.encryption.awsKms.keyArn` | AWS KMS key ARN | `""` |
| `broker.encryption.awsKms.dynamodbTableName` | DynamoDB table for branch keys | `IdentityBrokerEncryptionBranchKeys` |
| `broker.encryption.awsKms.branchKeyTtl` | Branch key TTL | `1h` |
| `broker.telemetry.enabled` | Enable OpenTelemetry tracing and metrics | `false` |
| `broker.oauth2AuthorizationServer.mode` | Operation mode: `proxy`, `local`, or `hybrid`. Required. | `"proxy"` |
| `broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri` | Upstream OAuth2 issuer URI (required in proxy/hybrid mode) | `""` |
| `broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint` | Upstream authorize endpoint | `""` |
| `broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint` | Upstream token endpoint | `""` |
| `broker.oauth2AuthorizationServer.proxy.upstreamTimeout` | Upstream request timeout (empty = use application default of 30s; accepts Go duration syntax: 30s, 500ms) | `""` |
| `broker.oauth2AuthorizationServer.proxy.upstreamJwksMinRefresh` | Minimum interval between upstream JWKS refresh attempts (empty = use application default of 15m) | `""` |
| `broker.oauth2AuthorizationServer.proxy.upstreamJwksMaxRefresh` | Maximum interval between upstream JWKS refresh attempts (empty = use application default of 1h) | `""` |
| `broker.oauth2AuthorizationServer.local.tokenTtl` | Access token validity period (required in local/hybrid mode) | `""` |
| `broker.oauth2AuthorizationServer.local.tokenClaimsExpression` | CEL expression for custom JWT claims | `""` |
| `broker.oauth2AuthorizationServer.local.signingKeys.bootstrapTimeout` | Startup budget for signing-key bootstrap coordination | `""` |
| `broker.oauth2AuthorizationServer.impersonation` | RFC 8693 user impersonation (signed or unverified subject). Only valid in `local` mode. Omitted when unset. Subtree keys use the broker config's snake_case names (`audience_prefix`, `rules[].{name,roles,trusted_issuers,authorization}`). `audience_prefix` is routing-only: a request appends one target agent's canonical lower-case UUID or `canonical_id`, both resolving to the same registered target; its UUID supplies minted `agent_id` and local-policy `agent.*`; local `tokenClaimsExpression` alone controls emitted `aud`. | _unset_ |
| `broker.tokenExchange.clientAssertion.issuerUri` | External IdP issuer for privileged-gateway client assertions. Required for configured token exchange in local mode; empty defaults to the proxy upstream issuer in proxy/hybrid mode. | `""` |
| `broker.tokenExchange.clientAssertion.jwksUri` | Explicit client-assertion JWKS endpoint; empty discovers it from the issuer. | `""` |
| `broker.tokenExchange.clientAssertion.jwksMinRefresh` | Minimum client-assertion JWKS refresh interval (Go duration); empty defaults to `15m`. | `""` |
| `broker.tokenExchange.clientAssertion.jwksMaxRefresh` | Maximum client-assertion JWKS refresh interval (Go duration); empty defaults to `max(jwksMinRefresh, 1h)`. | `""` |
| `broker.telemetry.serviceName` | Service name reported in every telemetry signal | `agentic-identity-broker` |
| `broker.telemetry.resourceAttributes` | Additional OTel resource attributes (map) | `{}` |
| `broker.telemetry.traces.enabled` | Enable trace export via OTLP | `true` |
| `broker.telemetry.traces.samplingRate` | Fractional sampling rate (0.0–1.0) | `1.0` |
| `broker.telemetry.traces.propagators` | Propagators to register globally (ottrace, b3multi, b3, tracecontext, baggage) | `[ottrace, b3multi, baggage]` |
| `broker.telemetry.metrics.enabled` | Enable metrics export via OTLP | `true` |
| `broker.telemetry.metrics.exportInterval` | Metrics push interval | `30s` |
| `broker.telemetry.logs.enabled` | Enable OTLP log export (set false if collector lacks LogsService) | `true` |
| `broker.telemetry.exporter.protocol` | OTLP transport protocol (`grpc`, `http`, or `https`) | `grpc` |
| `broker.telemetry.exporter.endpoint` | OTLP collector endpoint (host:port for gRPC, URL for HTTP/HTTPS) | `""` |
| `broker.telemetry.exporter.headers` | Additional headers sent with every export request (map) | `{}` |
| `broker.telemetry.exporter.timeout` | Export request timeout | `10s` |
| `broker.telemetry.exporter.compression` | Payload compression (`none` or `gzip`) | `none` |
| `broker.telemetry.exporter.insecure` | Disable TLS for the OTLP connection (never use in production) | `false` |
| `broker.requestContext.trustedProxy.enabled` | Trust forwarded headers from an upstream proxy for caller-IP derivation | `false` |
| `broker.requestContext.trustedProxy.forwardedHeader` | Forwarded header to inspect when trusted proxy mode is enabled | `X-Forwarded-For` |
| `broker.requestContext.trace.responseEnabled` | Emit the additive W3C `traceresponse` response header | `true` |

### Business Event Ledger Configuration

The chart exposes the accepted [ledger configuration contract](../../specs/048-business-event-ledger/contracts/configuration.md).
The broker ConfigMap includes both `business_events` keys and preserves an explicit `false`.
Ledger recording, retention, erasure, recoverable telemetry, partition provisioning, scheduling, and role grants are implemented.
Performance release acceptance remains open. See the [validation gates](../../specs/048-business-event-ledger/quickstart.md).

| Parameter | Type | Default | Contract |
|-----------|------|---------|----------|
| `broker.businessEvents.retention` | string | `2160h` | Recorded-time retention as a positive Go duration (90 days). |
| `broker.businessEvents.telemetryCopyEnabled` | bool | `true` | Additional ledger telemetry copies. Also requires `broker.telemetry.enabled` and `broker.telemetry.logs.enabled`. |
| `migration.grants.businessEvents.readerRole` | string | `""` | Existing operational role for `SELECT` on ledger parent tables. Empty grants nothing. |
| `migration.grants.businessEvents.erasureRole` | string | `""` | Existing operational role for `EXECUTE` on `public.business_event_erase_subject(text)`. Empty grants nothing. |

The retention contract accepts positive Go durations, such as `2160h` or `720h`, but not `90d`.
It rejects malformed, zero, negative, and overflowing durations.
Positive fractions round upward to microseconds.
No ledger-persistence disable switch exists.
Disabling telemetry copies does not disable recording or retention.
Existing telemetry destinations and ordinary logs remain unchanged.

The broker environment variables are `IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION` and `IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED`.
Their CLI flags are `--business_events.retention` and `--business_events.telemetry_copy_enabled`.
Precedence is explicit CLI flag, environment variable, configuration file, then default.

Example chart values for shorter retention without additional telemetry copies:

```yaml
broker:
  businessEvents:
    retention: 720h
    telemetryCopyEnabled: false
```

### Custom Values File

Create a `values-production.yaml` file:

```yaml
replicaCount: 3

namespace:
  create: true
  name: agentic-identity-broker

commonLabels:
  custom-label: my-value

storage:
  type: postgres

postgresql:
  # Labels here override commonLabels for the PostgreSQL resource.
  # Precedence (highest to lowest): postgresql.labels > commonLabels > base Helm labels.
  # Note: app.kubernetes.io/component is always "database" and cannot be overridden.
  labels:
    custom-label: a-deviating-value
  operator:
    enabled: true
    teamId: production
    numberOfInstances: 3
    volume:
      size: 50Gi

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

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10
  targetCPUUtilizationPercentage: 70
```

> **Label precedence** (highest to lowest):
> 1. PostgreSQL-specific labels (`postgresql.labels`)
> 2. Common labels (`commonLabels`)
> 3. Base Helm labels (chart, version, etc.)
>
> Note: The `app.kubernetes.io/component` label is always set to `database` for the PostgreSQL resource and cannot be overridden.

Install with custom values:

```bash
helm install broker ./charts/agentic-identity-broker -f values-production.yaml
```

## Ingress Configuration

### Nginx Ingress Controller

```yaml
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
```

### Skipper Ingress Controller

```yaml
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

## Database Migration

### How Migrations Work

1. Migrations run as a Kubernetes Job with Helm hook (`pre-install`, `pre-upgrade`)
2. Migration Job uses a **separate Docker image** (`agentic-identity-broker-migrate`) containing only migration tooling
3. Migration Job uses a separate ServiceAccount and user credentials (least privilege)
4. Broker Deployment uses the main image and a different user with appropriate runtime permissions
5. Migration Job completes before broker pods start

### Security: Separate Migration Image

For security hardening, the migration Job uses a dedicated Docker image:

| Image | Contains | Used By | Security Posture |
|-------|----------|---------|------------------|
| `agentic-identity-broker` | Broker binary, frontend assets | Broker Deployment | Minimal - no SQL migration capabilities |
| `agentic-identity-broker-migrate` | golang-migrate tool, migration files | Migration Job only | Schema permissions - isolated to Job |

**Benefits**:
- **Defense in Depth**: Runtime broker containers cannot execute arbitrary SQL migrations even if compromised
- **Least Privilege**: Migration tooling only present in short-lived Job pods
- **Clear Security Boundaries**: Explicit separation between runtime and schema modification capabilities

Both images share the same version tag to ensure consistency between migrations and application code.

### Migration Credentials

**External PostgreSQL**:
- Set `postgresql.external.migrationSecretName` to your migration user secret
- Set `postgresql.external.brokerSecretName` to your broker user secret
- Migration user needs: `CREATE`, `ALTER`, `DROP` permissions for schema changes
- Broker user needs business-table DML and the restricted ledger permissions in the [operations guide](../../docs/operations/business-event-ledger.md).

**Zalando PostgreSQL Operator**:
- Secrets are created automatically by the operator
- Secret names follow the pattern: `{username}.{teamId}-{instanceName}.credentials.postgresql.acid.zalan.do`
- The migration user owns the database. No special role attributes are required by the default configuration.
- The broker user has no special role attributes or database ownership. Post-migration grants provide runtime data access.

### Business Event Ledger Deployment Contract

With PostgreSQL and `migration.enabled: true`, the chart installs migration-owned maintenance every five minutes, independently of telemetry.
It uses a CronJob with schedule `*/5 * * * *` and `concurrencyPolicy: Forbid`.
The Job runs `SELECT public.business_event_maintain_partitions();` through `psql -v ON_ERROR_STOP=1`.
It reuses `migration.grants.image`, the migration ServiceAccount, migration credentials, and the trusted database endpoint and SSL configuration.
It uses non-root execution, a read-only filesystem, bounded resources, a 120-second deadline, and at most two retries.
The SQL connection sets a five-second lock timeout and a 30-second statement timeout.

The maintenance function reads the stored `business_event_policy`.
Broker startup writes the validated retention policy.
The Job has no separate retention parser or telemetry switch.
The initial migration seeds a 90-day policy.
The broker remains DML-only and never receives the migration Secret.
Memory uses an in-process retention worker with startup and five-minute sweeps, not a CronJob.

Migrations and pre-install/pre-upgrade hooks only call `public.business_event_provision_partitions()` to provision current and future partitions.
Provisioning never drops partitions.
Only scheduled maintenance drops expired partition pairs.
The migration Job remains required even when `migration.grants.enabled` is `false`.
Missing maintenance must trigger an alert before the 24-hour removal limit.
A missing current partition fails readiness until maintenance provisions it.
Disabling `migration.enabled` removes provisioning and maintenance Jobs. Install equivalent external migration and scheduler procedures in that case.
The [operations guide](../../docs/operations/business-event-ledger.md) provides bare-process installation, SQL grants, erasure, health alerts, and rollback precautions.

#### Coordinated Retention Changes

Retention is startup-scoped, and all replicas that share a ledger must use the same value.
The last successful validated startup policy write is authoritative.
Policy writes serialize with maintenance.
An old, shorter policy can delete history that an increased retention value must preserve.

For each retention change:

1. Suspend the maintenance CronJob.
2. Upgrade the release so every broker replica starts with the same new retention value.
3. With migration credentials, verify that `business_event_policy.retention_microseconds` contains the new normalized duration.
4. Resume the maintenance CronJob.

The maintenance template omits `spec.suspend`, so `helm upgrade` preserves the operator's suspension.
The pre-upgrade provisioning hook never runs a destructive retention sweep.

#### Operational Roles and Custom Grants

The role contract uses existing database roles only.
The chart does not create roles.
Empty role values grant nothing, so investigation remains outside the broker and erasure remains with the migration owner.
Neither operational capability belongs to the broker user.

A custom `migration.grants.sql` replaces the complete default grants script.
For a ledger deployment, the custom script must apply ledger restrictions and operational role grants itself.
It must revoke event `UPDATE` and `DELETE` after broad DML grants.
It must deny broker execution of maintenance, provisioning, and erasure functions.
It must preserve migration ownership and immutable-update protection for parent tables and future partitions.
Reader and erasure role values do not supplement a custom script.
The [storage contract](../../specs/048-business-event-ledger/contracts/storage.md) defines the required privilege boundaries.
The default script removes PUBLIC/runtime execution on future migration-owned functions. Give non-ledger functions explicit execution grants when necessary.
Runtime must not own ledger objects or inherit forbidden privileges from another role.

CAUTION: Back up ledger and business data before migration rollback. Obtain explicit acknowledgement of history loss and deploy the matching old binary.
Migration 036 rollback removes ledger history, pending references, policy, and expiry markers. It does not restore erased or expired history.

## Static Manifest Generation

Generate Kubernetes manifests without installing:

```bash
# Generate with default values
helm template broker ./charts/agentic-identity-broker > manifests.yaml

# Generate with custom values
helm template broker ./charts/agentic-identity-broker \
  --set storage.type=postgres \
  --set postgresql.operator.enabled=true \
  --set postgresql.operator.teamId=my-team \
  > manifests.yaml

# Apply to cluster
kubectl apply -f manifests.yaml
```

## Upgrading

```bash
# Upgrade to new chart version or values
helm upgrade broker ./charts/agentic-identity-broker -f values-production.yaml

# Migrations run automatically before broker pods are updated
```

## Uninstalling

```bash
helm uninstall broker
```

⚠️ **Note**: If using Zalando PostgreSQL Operator, the PostgreSQL CR is NOT deleted automatically to prevent data loss. Delete manually if desired:

```bash
kubectl delete postgresql <teamId>-broker
```

## Troubleshooting

### Migration Job Failed

Check migration logs:

```bash
kubectl logs job/broker-migrate
```

Common issues:
- **Database not reachable**: Check network policies, service DNS resolution
- **Invalid credentials**: Verify secret values and key names
- **Migration files missing**: Ensure Docker image includes `/app/migrations/`

### Broker Pod CrashLoopBackOff

Check broker logs:

```bash
kubectl logs deployment/broker
```

Common issues:
- **Database connection failed**: Verify broker secret and database host
- **Configuration error**: Check ConfigMap values
- **Missing JWE signing key**: Ensure secret `broker-jwe-key` exists

### Zalando Operator Secrets Not Created

Check PostgreSQL CR status:

```bash
kubectl describe postgresql <teamId>-broker
```

The operator creates secrets after the PostgreSQL cluster is ready (1-2 minutes).

## Security

### Pod Security Standards

The chart enforces Kubernetes Pod Security Standards (restricted):

- `runAsNonRoot: true` - Runs as non-root user (UID 1000)
- `readOnlyRootFilesystem: true` - Filesystem is read-only
- `allowPrivilegeEscalation: false` - No privilege escalation
- `capabilities.drop: [ALL]` - All Linux capabilities dropped
- `seccompProfile: RuntimeDefault` - Seccomp profile applied

### Database Credential Separation

- **Migration Job**: Uses dedicated ServiceAccount with separate database credentials
- **Migration user**: Schema modification permissions (varies by PostgreSQL setup)
- **Broker user**: Runtime data access permissions (varies by PostgreSQL setup)
- **Separate Secrets**: Different Kubernetes Secrets for each user

## Examples

### Minimal Production Setup

```yaml
storage:
  type: postgres

postgresql:
  operator:
    enabled: true
    teamId: production

ingress:
  enduser:
    enabled: true
    hosts:
      - host: broker.example.com
        paths:
          - path: /
            pathType: Prefix
```

### High Availability Setup

```yaml
replicaCount: 3

storage:
  type: postgres

postgresql:
  operator:
    enabled: true
    teamId: production
    numberOfInstances: 3

autoscaling:
  enabled: true
  minReplicas: 3
  maxReplicas: 10

podDisruptionBudget:
  enabled: true
  minAvailable: 2
```

### OpenTelemetry with gRPC Exporter

Enable distributed tracing and metrics using a gRPC OTLP collector (e.g. OpenTelemetry Collector, Grafana Alloy):

```yaml
broker:
  telemetry:
    enabled: true
    serviceName: agentic-identity-broker
    resourceAttributes:
      deployment.environment: production
      cloud.region: eu-west-1
    traces:
      enabled: true
      samplingRate: 0.1  # 10% in production
    metrics:
      enabled: true
      exportInterval: 30s
    exporter:
      protocol: grpc
      endpoint: otel-collector.monitoring.svc:4317
      insecure: false  # TLS enabled (production default)
```

### OpenTelemetry with HTTP Exporter and Authentication

For OTLP/HTTP collectors that require an authorization header (e.g. Grafana Cloud):

```yaml
broker:
  telemetry:
    enabled: true
    exporter:
      protocol: http
      endpoint: https://otlp-gateway.grafana.net
      headers:
        Authorization: "Bearer ${OTEL_EXPORTER_AUTH_TOKEN}"
      timeout: 30s
```

> **Tip**: Store the authentication token in a Kubernetes Secret and inject it into the pod environment so that `${OTEL_EXPORTER_AUTH_TOKEN}` is resolved at runtime by the broker's configuration loader, which expands `${VAR}` references from the process environment. The env var `IDENTITY_BROKER_TELEMETRY_EXPORTER_ENDPOINT` can also be used to override the endpoint without modifying the ConfigMap (useful in multi-environment Helm releases).

### Request Security Context Behind a Trusted Proxy

Use this when your ingress or reverse proxy appends `X-Forwarded-For` and you want the broker to return the additive W3C `traceresponse` header to callers:

```yaml
broker:
  requestContext:
    trustedProxy:
      enabled: true
      forwardedHeader: X-Forwarded-For
    trace:
      responseEnabled: true
```

> **Note**: Request-security-context capture is always enabled. Setting `responseEnabled: false` suppresses only the response header; request-scoped `trace_id` logging remains active.

## License

[License information]

## Maintainers

- Agentic Identity Broker Team

## Links

- [Project Repository](https://github.com/zalando-incubator/agentic-identity-broker)
- [Documentation](https://github.com/zalando-incubator/agentic-identity-broker/tree/main/docs)
- [Issue Tracker](https://github.com/zalando-incubator/agentic-identity-broker/issues)
