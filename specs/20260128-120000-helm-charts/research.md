# Research: Helm Charts for Kubernetes Deployment

**Feature**: 20260128-120000-helm-charts  
**Date**: 2026-01-28

## R1: Helm Chart Best Practices

### Decision: Follow Helm 3.x Conventions

**Rationale**: Helm 3.x is the current stable version with widespread adoption. Following community conventions ensures compatibility with Helm tooling and familiarity for operators.

### Key Patterns

#### Chart Structure
```text
charts/agentic-identity-broker/
├── Chart.yaml           # Required: chart metadata
├── values.yaml          # Required: default configuration
├── .helmignore          # Optional: exclude files from packaging
├── README.md            # Required: chart documentation
├── templates/           # Required: Kubernetes manifest templates
│   ├── _helpers.tpl     # Partial templates for reusable logic
│   ├── NOTES.txt        # Post-install instructions
│   └── *.yaml           # Resource templates
└── tests/               # Optional: helm test resources
```

#### Helper Templates (_helpers.tpl)

Standard helpers to include:
```yaml
{{- define "agentic-identity-broker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "agentic-identity-broker.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}

{{- define "agentic-identity-broker.labels" -}}
helm.sh/chart: {{ include "agentic-identity-broker.chart" . }}
app.kubernetes.io/name: {{ include "agentic-identity-broker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "agentic-identity-broker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentic-identity-broker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
```

#### Helm Hooks for Migrations

Migration Job uses Helm hooks to run before main resources:
```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: {{ include "agentic-identity-broker.fullname" . }}-migrate
  annotations:
    "helm.sh/hook": pre-install,pre-upgrade
    "helm.sh/hook-weight": "-5"
    "helm.sh/hook-delete-policy": before-hook-creation,hook-succeeded
```

Hook annotations:
- `helm.sh/hook: pre-install,pre-upgrade` - Run before install and upgrade
- `helm.sh/hook-weight: "-5"` - Lower weight runs first (useful if multiple hooks)
- `helm.sh/hook-delete-policy: before-hook-creation,hook-succeeded` - Clean up previous Job and successful Jobs

**Alternatives Considered**:
- Init containers: Rejected because they run on every pod restart, not just deployments
- Separate Helm release: Rejected because it adds operational complexity

---

## R2: Zalando PostgreSQL Operator Integration

### Decision: Use Operator-Managed Users via `postgresql.users`

**Rationale**: The Zalando PostgreSQL Operator automatically creates Kubernetes Secrets for defined users, eliminating manual secret management.

### PostgreSQL CR Schema

```yaml
apiVersion: "acid.zalan.do/v1"
kind: postgresql
metadata:
  name: {{ .Values.postgresql.operator.teamId }}-broker
  namespace: {{ .Release.Namespace }}
spec:
  teamId: {{ .Values.postgresql.operator.teamId }}
  volume:
    size: {{ .Values.postgresql.operator.volume.size }}
    storageClass: {{ .Values.postgresql.operator.volume.storageClass }}
  numberOfInstances: {{ .Values.postgresql.operator.numberOfInstances }}
  
  # User definitions (PostgreSQL role attributes)
  users:
    {{ .Values.postgresql.operator.users.migration }}:
      - superuser   # Schema modification permissions for migrations
      - createdb    # Database creation permission
    {{ .Values.postgresql.operator.users.broker }}:
      - login       # Login permission (ownership grants table access)
  
  databases:
    broker: {{ .Values.postgresql.operator.users.broker }}  # Owner is broker user
  
  postgresql:
    version: "15"
```

### Secret Naming Convention

The Zalando operator creates secrets with this pattern:
```
<username>.<teamId>-<cluster-name>.credentials.postgresql.acid.zalan.do
```

Example for our configuration:
- Migration user secret: `broker-migration.broker-broker.credentials.postgresql.acid.zalan.do`
- Broker user secret: `broker.broker-broker.credentials.postgresql.acid.zalan.do`

Secret contains:
```yaml
data:
  username: <base64-encoded username>
  password: <base64-encoded password>
```

### Helm Template for Secret Reference

```yaml
{{- if .Values.postgresql.operator.enabled }}
env:
  - name: POSTGRES_USER
    valueFrom:
      secretKeyRef:
        name: {{ .Values.postgresql.operator.users.broker }}.{{ .Values.postgresql.operator.teamId }}-broker.credentials.postgresql.acid.zalan.do
        key: username
  - name: POSTGRES_PASSWORD
    valueFrom:
      secretKeyRef:
        name: {{ .Values.postgresql.operator.users.broker }}.{{ .Values.postgresql.operator.teamId }}-broker.credentials.postgresql.acid.zalan.do
        key: password
{{- end }}
```

**Alternatives Considered**:
- Manual secret creation: Rejected because it adds operational steps
- ExternalSecrets: Out of scope; can be added later

---

## R3: Kubernetes Security Best Practices

### Decision: Pod Security Standards (Restricted Profile)

**Rationale**: The restricted profile provides the highest security while remaining compatible with most workloads.

### SecurityContext Configuration

```yaml
# Pod-level security context
securityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

# Container-level security context
containers:
  - name: broker
    securityContext:
      allowPrivilegeEscalation: false
      readOnlyRootFilesystem: true
      capabilities:
        drop:
          - ALL
```

### Values.yaml Defaults

```yaml
podSecurityContext:
  runAsNonRoot: true
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000
  seccompProfile:
    type: RuntimeDefault

securityContext:
  allowPrivilegeEscalation: false
  readOnlyRootFilesystem: true
  capabilities:
    drop:
      - ALL
```

### ServiceAccount

Create a dedicated ServiceAccount with minimal permissions:
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: {{ include "agentic-identity-broker.serviceAccountName" . }}
  labels:
    {{- include "agentic-identity-broker.labels" . | nindent 4 }}
automountServiceAccountToken: false  # Disable unless needed
```

**Alternatives Considered**:
- Default ServiceAccount: Rejected for security isolation
- Privileged containers: Rejected; not needed

---

## R4: golang-migrate Docker Integration

### Decision: Separate Migration Docker Image (ADR-20260129-120000)

**Rationale**: Security best practice requires minimizing attack surface in production runtime. A compromised broker container should not have SQL schema modification capabilities. Using separate images (`agentic-identity-broker` for runtime, `agentic-identity-broker-migrate` for migrations) follows defense-in-depth and least-privilege principles while maintaining version consistency through shared appVersion.

### Migration Image Dockerfile

```dockerfile
# Dockerfile.migrate - Separate migration image
FROM alpine:3.19

ARG MIGRATE_VERSION=v4.17.0
ARG TARGETARCH

# Install golang-migrate
RUN apk add --no-cache curl && \
    curl -L https://github.com/golang-migrate/migrate/releases/download/${MIGRATE_VERSION}/migrate.linux-${TARGETARCH}.tar.gz | tar xvz && \
    mv migrate /usr/local/bin/migrate && \
    chmod +x /usr/local/bin/migrate && \
    apk del curl

# Copy migration files only
COPY ./migrations /migrations

# Run as non-root user
USER 1000:1000

ENTRYPOINT ["/usr/local/bin/migrate"]
```

### Migration Job Command

```yaml
containers:
  - name: migrate
    image: "{{ .Values.migration.image.repository }}:{{ .Values.migration.image.tag | default .Chart.AppVersion }}"
    args:
      - "-path"
      - "/migrations"
      - "-database"
      - "postgres://$(DB_USERNAME):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=disable"
      - "up"
```

### Environment Variables

Standard database environment variables (DB_* prefix per ADR-20260129-120000):
- `DB_HOST` - Database host
- `DB_PORT` - Database port (default: 5432)
- `DB_NAME` - Database name
- `DB_USERNAME` - Migration user with schema privileges (from secret)
- `DB_PASSWORD` - Migration password (from secret)

**Alternatives Considered**:
- Single image with dual-purpose: Rejected per ADR-20260129-120000; violates least-privilege principle, increases runtime attack surface
- Flyway: Rejected; golang-migrate is lighter and Go-native
- Embedded migrations in Go binary: Requires Go code changes; Helm should work with existing binary

---

## Summary

| Research Area | Decision | Key Pattern |
|--------------|----------|-------------|
| Chart Structure | Helm 3.x conventions | _helpers.tpl, NOTES.txt, standard labels |
| Migration Hooks | pre-install/pre-upgrade Job | `helm.sh/hook` annotations |
| Zalando Operator | Operator-managed users | `postgresql.users` in CR spec |
| Secret Naming | Auto-derived from CR | `<user>.<team>-<db>.credentials...` |
| Security | Restricted Pod Security | runAsNonRoot, drop ALL capabilities |
| golang-migrate | Separate migration image (ADR-20260129-120000) | Two images: broker (runtime) + migrate (migrations only) |
