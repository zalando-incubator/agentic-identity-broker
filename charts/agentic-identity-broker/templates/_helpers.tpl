{{/*
Expand the name of the chart.
*/}}
{{- define "agentic-identity-broker.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "agentic-identity-broker.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "agentic-identity-broker.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Base labels (without commonLabels)
*/}}
{{- define "agentic-identity-broker.baseLabels" -}}
helm.sh/chart: {{ include "agentic-identity-broker.chart" . }}
{{ include "agentic-identity-broker.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "agentic-identity-broker.labels" -}}
{{- $base := fromYaml (include "agentic-identity-broker.baseLabels" .) -}}
{{- $common := .Values.commonLabels | default dict -}}
{{- toYaml (merge $base $common) -}}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "agentic-identity-broker.selectorLabels" -}}
app.kubernetes.io/name: {{ include "agentic-identity-broker.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Pod template labels (selector labels merged with commonLabels)
*/}}
{{- define "agentic-identity-broker.podLabels" -}}
{{- $selector := fromYaml (include "agentic-identity-broker.selectorLabels" .) -}}
{{- $common := .Values.commonLabels | default dict -}}
{{- toYaml (merge $selector $common) -}}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "agentic-identity-broker.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "agentic-identity-broker.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Return the appropriate apiVersion for ingress
*/}}
{{- define "agentic-identity-broker.ingress.apiVersion" -}}
{{- if .Capabilities.APIVersions.Has "networking.k8s.io/v1" -}}
networking.k8s.io/v1
{{- else if .Capabilities.APIVersions.Has "networking.k8s.io/v1beta1" -}}
networking.k8s.io/v1beta1
{{- else -}}
extensions/v1beta1
{{- end -}}
{{- end -}}

{{/*
Return the namespace for resources.
Uses namespace.name if set, otherwise falls back to the release namespace.
*/}}
{{- define "agentic-identity-broker.namespace" -}}
{{- .Values.namespace.name | default .Release.Namespace }}
{{- end }}

{{/*
PostgreSQL broker secret name helper
Returns the name of the secret containing broker database credentials
*/}}
{{- define "agentic-identity-broker.brokerSecretName" -}}
{{- if and (eq .Values.storage.type "postgres") .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.brokerSecretName }}
{{- else if and (eq .Values.storage.type "postgres") .Values.postgresql.operator.enabled }}
{{- $hasSuffix := hasKey .Values.postgresql.operator "secretSuffix" }}
{{- $suffix := "postgresql.acid.zalan.do" }}
{{- if $hasSuffix }}
{{- $suffix = .Values.postgresql.operator.secretSuffix }}
{{- end }}
{{- if $suffix }}
{{- printf "%s.%s-%s.credentials.%s" .Values.postgresql.operator.users.broker.name .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) $suffix }}
{{- else }}
{{- printf "%s.%s-%s.credentials" .Values.postgresql.operator.users.broker.name .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) }}
{{- end }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}

{{/*
PostgreSQL migration secret name helper
Returns the name of the secret containing migration database credentials
*/}}
{{- define "agentic-identity-broker.migrationSecretName" -}}
{{- if and (eq .Values.storage.type "postgres") .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.migrationSecretName }}
{{- else if and (eq .Values.storage.type "postgres") .Values.postgresql.operator.enabled }}
{{- $hasSuffix := hasKey .Values.postgresql.operator "secretSuffix" }}
{{- $suffix := "postgresql.acid.zalan.do" }}
{{- if $hasSuffix }}
{{- $suffix = .Values.postgresql.operator.secretSuffix }}
{{- end }}
{{- if $suffix }}
{{- printf "%s.%s-%s.credentials.%s" .Values.postgresql.operator.users.migration.name .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) $suffix }}
{{- else }}
{{- printf "%s.%s-%s.credentials" .Values.postgresql.operator.users.migration.name .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) }}
{{- end }}
{{- else }}
{{- "" }}
{{- end }}
{{- end }}

{{/*
PostgreSQL connection URL for broker
*/}}
{{- define "agentic-identity-broker.postgresHost" -}}
{{- if .Values.postgresql.external.enabled }}
{{- .Values.postgresql.external.host }}
{{- else if .Values.postgresql.operator.enabled }}
{{- printf "%s-%s" .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) }}
{{- end }}
{{- end }}

{{- define "agentic-identity-broker.postgresURL" -}}
{{- if .Values.postgresql.external.enabled }}
{{- printf "postgres://$(DB_USERNAME):$(DB_PASSWORD)@%s:%d/%s?sslmode=%s" .Values.postgresql.external.host (.Values.postgresql.external.port | int) .Values.postgresql.external.database .Values.postgresql.external.sslMode }}
{{- else if .Values.postgresql.operator.enabled }}
{{- printf "postgres://$(DB_USERNAME):$(DB_PASSWORD)@%s-%s:%d/%s?sslmode=require" .Values.postgresql.operator.teamId (include "agentic-identity-broker.fullname" .) 5432 .Values.postgresql.operator.database }}
{{- end }}
{{- end }}

{{/* Forward-only migration stages. Fresh installs retain the direct UP entrypoint. */}}
{{- define "agentic-identity-broker.migrationCommand" -}}
{{- if and .Release.IsUpgrade (gt (int .Values.migration.targetVersion) 0) }}
command: ["/bin/sh", "-ec"]
args:
  - |
    current="$(/usr/local/bin/migrate "$@" version 2>&1)" || {
      printf '%s\n' "$current" >&2
      exit 1
    }
    case "$current" in
      *"(dirty)"*)
        printf '%s\n' 'Dirty migration metadata: inspect schema before manual repair' >&2
        exit 1
        ;;
      ''|*[!0-9]*)
        printf 'Unexpected migration version: %s\n' "$current" >&2
        exit 1
        ;;
    esac
    target={{ .Values.migration.targetVersion }}
    if [ "$current" -ge "$target" ]; then
      printf 'Migration stage %s already satisfied at version %s: no schema changes\n' "$target" "$current"
      exit 0
    fi
    exec /usr/local/bin/migrate "$@" goto "$target"
  - "session-migration-stage"
  - "-path"
  - "/app/migrations"
  - "-database"
  - {{ include "agentic-identity-broker.postgresURL" . | quote }}
{{- else }}
args:
  - "-path"
  - "/app/migrations"
  - "-database"
  - {{ include "agentic-identity-broker.postgresURL" . | quote }}
  - "up"
{{- end }}
{{- end }}
