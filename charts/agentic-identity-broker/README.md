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

Set `broker.server.admin.publicUrl` (string, default `""`) to the external admin
URL, for example `https://broker-admin.example.com`. An empty chart value uses
the broker's default `http://localhost:14000`. Admin `/api` requests must match
this URL's Host authority; ingress and other reverse proxies must preserve it.
Forwarded Host headers are not trusted. `/health` probes remain unrestricted.

The admin port must only be reachable through an authenticated proxy. Restrict access at deployment level and route sweep CronJobs through that proxy.

`broker.server.enduser.authentication.jwt` and `broker.server.admin.authentication.jwt`
are optional, independent JWT pre-authentication blocks (default: unset). Each supports
`header_name`, `verification` (`jwks` or `none`), `jwks_uri`, audience/issuer constraints,
and CEL `claim_extraction` keys in broker snake_case. The chart passes both blocks to
their respective servers. If admin uses `verification: none` with `X-Userinfo`, the
gateway must authenticate and authorize the caller, strip client-supplied `X-Userinfo`,
set its own header with an operator `principal` claim, and prevent direct access to
port 14000. An `Authorization` bearer token alone does not supply the operator principal.

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

Keep AWS TLS certificate verification enabled in production. If
`broker.extraEnv` sets `IDENTITY_BROKER_ENCRYPTION_AWS_KMS_DISABLE_SSL=true`,
startup requires explicit `GO_ENV=development`. Production, staging, unset,
and unknown environments reject that development-only bypass.

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of broker pods | `1` |
| `strategy` | Deployment strategy. Online upgrades require `RollingUpdate`, no unavailable pods, and ready surge capacity. Overrides remain operator-controlled. | `{type: RollingUpdate, rollingUpdate: {maxUnavailable: 0, maxSurge: 1}}` |
| `terminationGracePeriodSeconds` | Pod termination grace. Must exceed broker shutdown plus traffic-drain time. | `60` |
| `image.repository` | Broker container image repository | `agentic-identity-broker` |
| `image.tag` | Broker image tag | Chart appVersion |
| `broker.extraEnv` | Additional broker environment variables (`value` or `valueFrom`) | `[]` |
| `migration.image.repository` | Migration container image repository | `agentic-identity-broker-migrate` |
| `migration.image.tag` | Migration image tag | Chart appVersion |
| `migration.targetVersion` | Integer from 0 through 40. Fresh installs run all available UP migrations through 40. Upgrades use a positive forward-only target. Explicit 0 is refused. Equal/newer clean schemas are no-ops. Dirty metadata aborts. | `40` |
| `storage.type` | Storage backend (`memory` or `postgres`) | `memory` |
| `serviceAccount.irsa.enabled` | Enable AWS IRSA annotation | `false` |
| `serviceAccount.irsa.role` | IAM role ARN for IRSA | `""` |
| `ingress.enduser.enabled` | Enable Ingress for end-user API | `false` |
| `ingress.admin.enabled` | Enable Ingress for admin API | `false` |
| `broker.server.enduser.authentication.jwt` | End-user JWT pre-auth configuration, independent of admin | unset |
| `broker.server.admin.authentication.jwt` | Admin JWT pre-auth configuration, independent of end-user | unset |
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
| `broker.thirdPartyOauth2.sessionAPICompatibility` | Default API v2 omits `is_expired`. Explicit true adds only `is_expired=requires_reauthentication` for legacy consumers, without refresh dates. | `false` |
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
| `broker.tokenRefresh.lookaheadDuration` | String (Go duration): refresh before expiry; set below each provider access-token lifetime. | `"5m"` |
| `broker.tokenRefresh.backgroundWorkers` | Integer (1–20): concurrent refreshes per replica. The 20-worker cap reserves 5 of 25 pooled database connections. | `10` |
| `broker.tokenRefresh.sweep.defaultPageSize` | Integer (1–1000): default sessions per page for the externally triggered admin sweep. | `100` |
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
5. Migration and grants complete before the new Deployment starts or rolls. Existing pods continue serving during a pre-upgrade hook.

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

### Online Session Validity Upgrade

This PR3 cleanup release includes all three migrations: 038, 039, and 040. It retains the prepared broker, API, and SPA from PR1 #224 and durable rejection from PR2 #225. A normal Helm upgrade applies 040 before the application update and removes only the unused legacy column. The default and maximum `migration.targetVersion` are 40. Fresh installations use direct UP-all through 40, including with the default target. Explicit target 0 remains available for fresh installs but fails PostgreSQL upgrade rendering.

| Release | Default target | Serving replicas and schema |
|---|---|---|
| PR1 #224 preparation (preceding release) | `38` | Preserve the original table shape. Add the explicit read view and stable write functions. Roll prepared brokers across every replica. |
| PR2 #225 activation (preceding release) | `39` | Add the real rejection flag and replace the view/functions atomically. Already-running PR1 replicas activate durable rejection without restart. |
| PR3 cleanup (this release) | `40` | Remove only the unused legacy column after schema and API consumer gates pass. Default API compatibility becomes false. |

Before merging or deploying PR2 #225, complete the PR1 #224 rollout across every replica. Drain every unprepared replica and in-flight operation first. Before merging or deploying this PR3 release, complete PR2 across every replica. Upgrade the embedded SPA and every external session-API consumer. End all legacy database dependencies before cleanup. Normal cleanup uses the default target 40 and compatibility false, without a manual stage selector.

At schema 38, ordinary acquisition, refresh, and authorization work normally. The preparation view reports rejection=false, and true writes fail as `persistence_failed`. At schemas 39 and 40, the same SQL boundary persists rejection and immediately stops subsequent broker issuance. Unprepared binaries do not enforce the new policy and cannot coexist after activation. The legacy expiry column at 38 and 39 supplies storage compatibility only, never provider-validity evidence. Migration 040 removes it without changing the stable SQL boundary or token metadata.

Session API compatibility defaulted to true in preparation and activation. This cleanup release defaults it to false and omits the legacy wire field. Explicit `broker.thirdPartyOauth2.sessionAPICompatibility=true` remains a deliberate override for legacy consumers. It adds only `is_expired=requires_reauthentication`, without refresh dates. This override does not waive PR3's consumer-upgrade gate.

Live rollout requires at least two healthy replicas, including `autoscaling.minReplicas >= 2` when autoscaling is enabled. Keep `strategy.type=RollingUpdate`, `maxUnavailable=0`, and `maxSurge >= 1`. Reserve resources for a ready replacement before an old pod terminates. Enable a disruption budget that protects the required capacity during voluntary evictions. A disruption budget does not control Deployment rolling updates. Set termination grace longer than broker shutdown plus load-balancer drain time. Verify readiness and request draining in the actual deployment.

The existing asset-hosting arrangement must retain immutable hashed SPA assets from both running and replacement releases. Assets must remain available during rolling updates and for already-open clients. A shared asset origin, retained bundles, or verified routing can provide this continuity. SQL safety and rolling limits alone do not prove browser continuity. If this prerequisite is absent, do not begin the rollout. Keep the existing release serving. No new deployment stack is required.

The chart retains custom strategy and grace values. `Recreate`, positive `maxUnavailable`, or insufficient surge capacity do not satisfy online-rollout prerequisites. Default one-replica installations and disabled disruption budgets also do not meet them. The default 60-second grace assumes the 30-second broker shutdown plus sufficient drain time.

The migration Job keeps its separate image, credentials, grants image, and migration-before-grants ordering in both grants modes. On upgrades, positive targets inspect the current version and dirty state with the same migration tool. Dirty, unreadable, or unexpected metadata aborts without `Force`. A clean current version equal to or greater than the target is a non-destructive no-op. Only a lower clean version runs `goto`. The stage wrapper requires the existing Alpine image's `/bin/sh` and `/usr/local/bin/migrate`, or those paths in a custom image. An upgrade stage requires an initialized schema. Fresh installs use direct UP-all instead. Serialize all migration runners for the database to prevent a version-check/execution race. Helm stages never request DOWN. Use the separate runner's explicit rollback procedure instead. For live phase changes, set `migration.backoffLimit=0` so operators inspect failures before retry. A failed pre-upgrade hook leaves existing compatible replicas in place.

Schema steps use atomic statements and fail-fast locks. Migrations 039 and 040 lock the view, then the table, with `NOWAIT`. Busy locks abort without schema or row changes. Short metadata locks can affect latency. There is no absolute latency guarantee. Inspect the actual schema and dirty migration metadata before deliberate repair or retry. Never automate `Force`.

This release supplies DOWN 040, which restores only a nullable legacy column and retains rejection. Prepared replicas can continue to serve. Guarded DOWN 039 to schema 38 refuses while rejected rows remain. Resolve those rows through successful reauthentication or explicit user termination, not flag clearing or automatic deletion. At schema 38, matching old binaries can replace prepared replicas through a safe rolling deployment. Drain every prepared/new replica before DOWN 038 removes their SQL boundary. Helm image rollback alone does not establish schema compatibility. Do not automatically roll back to an unprepared image after phase 39.

See [the online rollout runbook](../../docs/operations/session-sweep.md#online-session-validity-rollout-migrations-038040) for cleanup values, failure handling, and rollback order.


### Migration Credentials

**External PostgreSQL**:
- Set `postgresql.external.migrationSecretName` to your migration user secret
- Set `postgresql.external.brokerSecretName` to your broker user secret
- Migration user needs: `CREATE`, `ALTER`, `DROP` permissions for schema changes
- Broker user needs: `SELECT`, `INSERT`, `UPDATE`, `DELETE` permissions for data access

**Zalando PostgreSQL Operator**:
- Secrets are created automatically by the operator
- Secret names follow the pattern: `{username}.{teamId}-{instanceName}.credentials.postgresql.acid.zalan.do`
- Migration user gets: `superuser` and `createdb` role attributes
- Broker user gets: `login` attribute + database ownership (grants full table access)

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
