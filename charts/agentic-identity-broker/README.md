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

Use `broker.extraVolumes` and `broker.extraVolumeMounts` to append standard Kubernetes volumes and container mounts. Both default to empty lists. Mount names must reference a pod volume; use names other than the chart's built-in `config` and `tmp`. Create any referenced Secrets or ConfigMaps in the broker's namespace before deploying it.

For example, mount an existing ConfigMap read-only:

```yaml
broker:
  extraVolumes:
    - name: supplemental-config
      configMap:
        name: broker-settings
  extraVolumeMounts:
    - name: supplemental-config
      mountPath: /etc/broker/settings
      readOnly: true
```

### Filesystem OAuth2 credentials

`broker.thirdPartyOauth2.credentialFiles` maps exact service canonical IDs to two paths. The default `{}` renders no broker binding. Keys preserve case and dots. Both `client_id_file` and `client_secret_file` are required strings. Broker startup validates absolute, non-empty paths and the full canonical-ID grammar without opening files.

The chart does not register services or choose their credential source. Register an eligible service through Admin API 2.0.0 with `credential_source: filesystem` and no inline `client_id` or `client_secret`, including null or empty fields. Stored services ignore bindings. A filesystem service without its exact binding fails closed at use. Use a feature-capable broker image with this contract.

```yaml
broker:
  thirdPartyOauth2:
    credentialFiles:
      zalando-platform:
        client_id_file: /meta/credentials/employee-client-id
        client_secret_file: /meta/credentials/employee-client-secret
  extraVolumes:
    - name: credentials
      secret:
        secretName: agentic-platform-credentials
  extraVolumeMounts:
    - name: credentials
      mountPath: /meta/credentials
      readOnly: true
```

Use the existing volume inputs for a read-only directory mount. Do not use credential-file `subPath`. Preserve the built-in `config` and `tmp` mounts. The non-root pod identity must read both files and traverse every parent directory. Read-only mounting does not grant those permissions.

Publish each replacement pair together through fresh immutable targets. Never modify published targets or republish retired targets during acquisition. Code exchange and refresh acquire both values and revalidate their targets. A detected change produces `generation_changed` before authentication without acquisition retries. Publication after validation can leave an in-flight operation using its validated pair. Later operations acquire current targets.

Authorization initiation reads only the client-ID file. Metadata reads resolve neither file. Same-client secret rotation requires no restart or registration update. A changed client ID requires new connections for old codes or sessions. Provider validity overlap remains an operator responsibility, not an uninterrupted-rotation guarantee.

### Key Configuration Parameters

| Parameter | Description | Default |
|-----------|-------------|---------|
| `replicaCount` | Number of broker pods | `1` |
| `image.repository` | Broker container image repository | `agentic-identity-broker` |
| `image.tag` | Broker image tag | Chart appVersion |
| `broker.extraEnv` | Additional broker environment variables (`value` or `valueFrom`) | `[]` |
| `broker.extraVolumes` | Additional Kubernetes volumes for the broker pod | `[]` |
| `broker.extraVolumeMounts` | Additional Kubernetes volume mounts for the broker container | `[]` |
| `migration.image.repository` | Migration container image repository | `agentic-identity-broker-migrate` |
| `migration.image.tag` | Migration image tag | Chart appVersion |
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
| `broker.thirdPartyOauth2.credentialFiles` | Exact canonical-ID object with required `client_id_file` and `client_secret_file` string paths in each binding | `{}` |
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
| `broker.tokenExchange.clientAssertion.issuerUri` | External IdP issuer for privileged-gateway client assertions. Required in local mode when token exchange, approval authentication, or any client-assertion key source is configured; empty defaults to the proxy upstream issuer in proxy/hybrid mode. | `""` |
| `broker.tokenExchange.clientAssertion.jwksUri` | Explicit client-assertion JWKS endpoint; empty discovers it from the issuer. | `""` |
| `broker.tokenExchange.clientAssertion.jwksMinRefresh` | Minimum client-assertion JWKS refresh interval (Go duration); empty defaults to `15m`. | `""` |
| `broker.tokenExchange.clientAssertion.jwksMaxRefresh` | Maximum client-assertion JWKS refresh interval (Go duration). For `jwksFile`, empty or below-minimum values default to `max(3 × jwksMinRefresh, 1h)`; remote sources default to `max(jwksMinRefresh, 1h)`. | `""` |
| `broker.tokenExchange.clientAssertion.jwks` | Inline JWK Set JSON (public keys only) for client assertions. Mutually exclusive with `jwksUri`, `jwksFile`, `jwksMinRefresh`, and `jwksMaxRefresh`. Skips discovery and key fetch. | `""` |
| `broker.tokenExchange.clientAssertion.jwksFile` | Absolute key-set file path inside `broker.security.trustMaterial.directory`. Mutually exclusive with `jwksUri` and `jwks`. | `""` |
| `broker.security.trustMaterial.directory` | Absolute path of the trust directory (`security.trust_material_dir`), the only place `jwks_file` key sets are read. Required when `sources` is set. | `""` |
| `broker.security.trustMaterial.sources` | Existing Secrets or ConfigMaps projected read-only into the trust directory. Entries are `{secret: {name, items}}` or `{configMap: {name, items}}`; items are `{key, path}`. | `[]` |
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
| `extraObjects` | Additional manifests deployed with the release, rendered with `tpl` (see [Extra Objects](#extra-objects)) | `[]` |

### Protected-resource discovery

| Parameter | Type | Default | Description |
|-----------|------|---------|-------------|
| `broker.thirdPartyOauth2.clientName` | `string` | `""` | Optional broker or platform name. The chart maps it to `third_party_oauth2.client_name`. DCR requires a non-blank value. Startup, manual services, and hosted CIMD do not. |
| `broker.server.enduser.publicUrl` | `string` | `""` | The chart maps it to `server.enduser.public_url`. Hosted CIMD and DCR require a stable, public HTTPS URL. DCR uses it for the OAuth2 callback. |

The chart does not configure a service's `discovery.resource_url`. Send that public HTTPS URL in the service's Admin API create or update request.

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

## Extra Objects

`extraObjects` deploys additional manifests with the release, such as routing resources for an
ingress controller the chart does not template. Each entry is a map or a multi-line string, and
the chart renders it with Helm's `tpl`, so entries can reference values and chart helpers.

Use the string form when a templated field must keep its YAML type. In the map form, a quoted
template such as `'{{ .Values.service.ports.enduser }}'` renders as a string, while the string
form below keeps `port` an integer:

```yaml
extraObjects:
  - |
    apiVersion: gateway.networking.k8s.io/v1
    kind: HTTPRoute
    metadata:
      name: {{ include "agentic-identity-broker.fullname" . }}-enduser
      namespace: {{ include "agentic-identity-broker.namespace" . }}
      labels:
        {{- include "agentic-identity-broker.labels" . | nindent 4 }}
    spec:
      parentRefs:
        - name: public-gateway
      hostnames:
        - broker.example.com
      rules:
        - backendRefs:
            - name: {{ include "agentic-identity-broker.fullname" . }}
              port: {{ .Values.service.ports.enduser }}
```

Text in the form `{{ ... }}` is a Helm template expression. When a later tool templates the
rendered manifests, such as a deployment pipeline that substitutes `{{{BUILD_VERSION}}}`, write the
placeholder as a string constant so that Helm emits it literally:

```yaml
extraObjects:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: broker-build-info
    data:
      build: '{{ "{{{BUILD_VERSION}}}" }}'   # renders as '{{{BUILD_VERSION}}}'
```

Mustache-based pipelines HTML-escape a `{{VAR}}` value and insert a `{{{VAR}}}` value unchanged.
Use the triple-brace form for values that can contain characters such as `&`, `/`, or `=`.

Quote a value that starts with `{{` in single quotes. Unquoted, YAML parses it as a flow mapping.

## Database Migration

### How Migrations Work

1. Migrations run as a Kubernetes Job with Helm hook (`pre-install`, `pre-upgrade`)
2. Migration Job uses a **separate Docker image** (`agentic-identity-broker-migrate`) containing only migration tooling
3. Migration Job uses a separate ServiceAccount and user credentials (least privilege)
4. Broker Deployment uses the main image and a different user with appropriate runtime permissions
5. Migration Job completes before new broker pods start; on upgrade, existing pods continue serving until the hook succeeds

### Credential-source migrations 040 and 041

Migration 040 performs metadata-only schema expansion. It adds CHECK constraints as `NOT VALID`: new writes are checked immediately, but existing rows are not scanned under the exclusive schema locks. Both table locks use `NOWAIT`, and the schema statement has a one-second timeout. A busy table causes the hook to fail immediately rather than queue an exclusive lock that blocks live traffic.

Migration 041 validates existing rows in a separate transaction. Its `SHARE UPDATE EXCLUSIVE` locks permit ordinary reads and writes. It also uses `NOWAIT` to avoid waiting on other schema maintenance. Each migration is atomic; keep `x-multi-statement` disabled when using golang-migrate.

Before migration 040, deploy a compatibility release with explicit session-query columns to every pod. Wildcard scans in older binaries reject the new `upstream_client_id` column, even for stored services. Changing queries in the new release cannot repair those running binaries.

Complete both migrations before rolling out the new broker. Keep services in `stored` mode until all old broker pods are replaced. Older binaries cannot scan filesystem services with NULL `client_id`, including during startup validation. The expansion still needs a brief exclusive lock, so it cannot promise zero request latency. Validation can also consume database I/O. Allow sufficient Helm timeout and database headroom.

On failure, leave the old Deployment running and inspect migration logs and actual schema before clearing dirty metadata:

| Failed step | Schema to verify | Recovery after verification |
|---|---|---|
| 040 UP: contention or timeout | Intact 039 columns, constraints, and data | `migrate force 39`, then retry the upgrade separately |
| 041 UP: contention or invalid legacy data | Intact 040 columns and all three CHECKs still `NOT VALID`; data unchanged | Resolve contention or repair invalid data explicitly, `migrate force 40`, then rerun validation |

The migration Job defaults to `backoffLimit: 0`. A failed migration can leave dirty metadata, so automatic Job retries cannot recover it. If inspection finds a different schema, stop and reconcile it before forcing any version. Never force 41 after failed validation.

#### Application rollback with the schema retained

1. Select a rollback release that includes explicit session-query columns. Do not roll back to a binary with wildcard session scans.
2. Before starting older pods, list services through the current Admin API.
3. PUT every filesystem service back to `credential_source: stored` with explicit, non-empty `client_id` and `client_secret`. Supply the complete replacement metadata. Use inline credentials from the operator's secure source, not values imported by the broker. Omit `protected_resources` to preserve them, or supply the current `If-Match` when replacing them.
4. Verify that every service reports stored mode before running `helm rollback`.
5. Retain schema 040 and the transition evidence. Returning to stored does not permit a database downgrade: migration 040 DOWN still rejects transitioned rows.

An earlier compatibility release can instead exclude NULL-client-ID rows from every provider lookup and listing query. Those services must return not-found and fail closed. Converting NULL to an empty string is not sufficient. This branch does not change already-released binaries.

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

### Trust material for local key sets

A trust anchor can take its public verification keys from an inline JWK Set (`jwks`) or a key-set file (`jwks_file`) instead of a remote `jwks_uri`. The broker reads key-set files only from `broker.security.trustMaterial.directory`. The chart mounts existing Secrets or ConfigMaps there, so no custom image is needed.

The chart exposes these settings for the token-exchange client-assertion trust anchor (`broker.tokenExchange.clientAssertion.jwks` and `jwksFile`). Impersonation trusted issuers (`broker.oauth2AuthorizationServer.impersonation.rules[].trusted_issuers[]`) and JWT pre-authentication on either server (`broker.server.enduser.authentication.jwt`, `broker.server.admin.authentication.jwt`) accept `jwks` and `jwks_file` in their pass-through blocks, with the broker's snake_case key names.

```bash
kubectl create secret generic agentgateway-jwks --from-file=jwks.json=./agentgateway-jwks.json
```

```yaml
broker:
  security:
    trustMaterial:
      directory: "/etc/agentic-identity-broker/trust"
      sources:
        - secret:
            name: agentgateway-jwks
            items:
              - key: jwks.json
                path: agentgateway-jwks.json
        - configMap:
            name: mesh-jwks
            items:
              - key: jwks.json
                path: mesh-jwks.json
  tokenExchange:
    clientAssertion:
      issuerUri: "https://agentgateway.internal.example.com"
      jwksFile: "/etc/agentic-identity-broker/trust/agentgateway-jwks.json"
      jwksMinRefresh: "5m"
      jwksMaxRefresh: "1h"
```

- **Projected volume**: each entry becomes one source of a read-only `projected` volume named `trust-material`, mounted at `directory`. Setting `sources` without `directory` fails `helm template` with `broker.security.trustMaterial.directory is required when sources are set`.
- **No `subPath`**: the chart never mounts the volume with `subPath`. Kubernetes does not update `subPath` mounts, so a `subPath` mount would never deliver a rotated key set. The projected volume keeps the `..data` relative-link layout that the broker accepts. Absolute symbolic links inside the trust directory are rejected.
- **Rotation**: when the Secret or ConfigMap changes, the kubelet updates the volume without restarting the pod. The broker re-reads each file every `jwksMinRefresh` (default `15m`; JWT pre-authentication uses a fixed `5m`) and applies a valid set without a restart. A failed re-read keeps the last valid set and reports `components.trust_anchor_key_files` as `degraded` on `GET /health`. If `jwksMaxRefresh` is unset or below `jwksMinRefresh`, a file uses `max(3 × jwksMinRefresh, 1h)`; remote sources use `max(jwksMinRefresh, 1h)`. An explicit maximum at least as large as the minimum applies as configured. JWT pre-authentication uses a fixed `15m` maximum. After the applicable maximum without a successful read, the trust anchor fails closed.
- **Inline alternative**: `jwks` is rendered as a YAML block scalar and is never refreshed, so `jwksMinRefresh` and `jwksMaxRefresh` are rejected next to it.

A whole-value environment reference keeps the key set in a Secret:

```yaml
broker:
  extraEnv:
    - name: GATEWAY_JWKS
      valueFrom:
        secretKeyRef:
          name: agentgateway-jwks
          key: jwks.json
  tokenExchange:
    clientAssertion:
      issuerUri: "https://agentgateway.internal.example.com"
      jwks: "${GATEWAY_JWKS}"
```

- **One source per trust anchor**: set at most one of `jwksUri`, `jwks`, and `jwksFile`.

See `examples/config/trust-anchor-local-jwks.yaml` for the rendered configuration.

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
