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

{{/*
Session sweep CronJob and ServiceAccount name. CronJob names are capped at 52 characters, so the
fullname is truncated to 38 to keep the 14-character "-session-sweep" suffix.
*/}}
{{- define "agentic-identity-broker.sessionSweep.name" -}}
{{- printf "%s-session-sweep" (include "agentic-identity-broker.fullname" . | trunc 38 | trimSuffix "-") }}
{{- end }}

{{/*
Render-time checks for the session sweep.
*/}}
{{- define "agentic-identity-broker.sessionSweep.validate" -}}
{{- $sweep := .Values.sessionSweep }}
{{- if not (or $sweep.headers $sweep.headersFromFile $sweep.tls.certFile) }}
{{- fail "sessionSweep.enabled requires an identity for the admin API: set sessionSweep.headers, sessionSweep.headersFromFile, or sessionSweep.tls.certFile." }}
{{- end }}
{{- if ge (int $sweep.timeoutSeconds) (int $sweep.activeDeadlineSeconds) }}
{{- fail (printf "sessionSweep.timeoutSeconds (%d) must be shorter than sessionSweep.activeDeadlineSeconds (%d)." (int $sweep.timeoutSeconds) (int $sweep.activeDeadlineSeconds)) }}
{{- end }}
{{- if ne (empty $sweep.tls.certFile) (empty $sweep.tls.keyFile) }}
{{- fail "sessionSweep.tls.certFile and sessionSweep.tls.keyFile must be set together." }}
{{- end }}
{{- end }}

{{/*
Session sweep pod labels. They never include app.kubernetes.io/name, so the job pod cannot
match the broker Service or PodDisruptionBudget selector.
*/}}
{{- define "agentic-identity-broker.sessionSweep.podLabels" -}}
{{- $base := dict "app.kubernetes.io/instance" .Release.Name "app.kubernetes.io/part-of" (include "agentic-identity-broker.name" .) "app.kubernetes.io/managed-by" .Release.Service }}
{{- $labels := merge (dict) (deepCopy (.Values.sessionSweep.podLabels | default dict)) (deepCopy (.Values.commonLabels | default dict)) $base }}
{{- $_ := unset $labels "app.kubernetes.io/name" }}
{{- $_ = set $labels "app.kubernetes.io/component" "session-sweep" }}
{{- toYaml $labels }}
{{- end }}

{{/*
Session sweep target URL: sessionSweep.url, or this release's admin Service.
*/}}
{{- define "agentic-identity-broker.sessionSweep.url" -}}
{{- if .Values.sessionSweep.url }}
{{- trimSuffix "/" .Values.sessionSweep.url }}
{{- else }}
{{- printf "http://%s.%s.svc:%v" (include "agentic-identity-broker.fullname" .) (include "agentic-identity-broker.namespace" .) .Values.service.ports.admin }}
{{- end }}
{{- end }}

{{/*
Session sweep Host override: sessionSweep.host, or the admin public URL host for the default target.
*/}}
{{- define "agentic-identity-broker.sessionSweep.host" -}}
{{- if .Values.sessionSweep.host }}
{{- .Values.sessionSweep.host }}
{{- else if and (not .Values.sessionSweep.url) .Values.broker.server.admin.publicUrl }}
{{- (urlParse .Values.broker.server.admin.publicUrl).host }}
{{- end }}
{{- end }}

{{/*
Session sweep request body.
*/}}
{{- define "agentic-identity-broker.sessionSweep.body" -}}
{{- $sweep := .Values.sessionSweep }}
{{- printf "{\"lookahead_duration\":%s,\"dry_run\":%t" (toJson $sweep.lookaheadDuration) $sweep.dryRun }}
{{- if $sweep.pageSize }}{{ printf ",\"page_size\":%d" (int $sweep.pageSize) }}{{ end }}
{{- "}" }}
{{- end }}

{{/*
Escape a string for a double-quoted curl configuration value.
*/}}
{{- define "agentic-identity-broker.curlConfigEscape" -}}
{{- replace "\"" "\\\"" (replace "\\" "\\\\" .) }}
{{- end }}

{{/*
Session sweep job script. curl reads each header file itself through a config variable, so a
credential never reaches a process argument list, the environment, or the shell's output. A
missing file fails the run before any request is sent. The values schema restricts file paths
to characters that need no quoting and keeps CR, LF, and braces out of configuration lines.
*/}}
{{- define "agentic-identity-broker.sessionSweep.script" -}}
{{- $sweep := .Values.sessionSweep -}}
{{- $fileHeaders := $sweep.headersFromFile | default dict -}}
nl='
'
out=$(curl --disable --config - <<'SESSION_SWEEP_CURL_CONFIG'
url = "{{ include "agentic-identity-broker.curlConfigEscape" (printf "%s/api/sessions/sweep" (include "agentic-identity-broker.sessionSweep.url" .)) }}"
request = "POST"
header = "Content-Type: application/json"
{{- with include "agentic-identity-broker.sessionSweep.host" . }}
header = "Host: {{ include "agentic-identity-broker.curlConfigEscape" . }}"
{{- end }}
{{- $literalHeaders := $sweep.headers | default dict }}
{{- range $name := keys $literalHeaders | sortAlpha }}
header = "{{ include "agentic-identity-broker.curlConfigEscape" $name }}: {{ include "agentic-identity-broker.curlConfigEscape" (toString (get $literalHeaders $name)) }}"
{{- end }}
{{- range $index, $name := keys $fileHeaders | sortAlpha }}
{{- $header := get $fileHeaders $name }}
variable = "header_{{ $index }}@{{ $header.path }}"
expand-header = "{{ include "agentic-identity-broker.curlConfigEscape" $name }}: {{ include "agentic-identity-broker.curlConfigEscape" ($header.prefix | default "") }}{{ printf "{{header_%d:trim:json}}" $index }}"
{{- end }}
data = "{{ include "agentic-identity-broker.curlConfigEscape" (include "agentic-identity-broker.sessionSweep.body" .) }}"
{{- with $sweep.tls.certFile }}
cert = "{{ . }}"
{{- end }}
{{- with $sweep.tls.keyFile }}
key = "{{ . }}"
{{- end }}
{{- with $sweep.tls.caFile }}
cacert = "{{ . }}"
{{- end }}
connect-timeout = 10
max-time = {{ int $sweep.timeoutSeconds }}
silent
show-error
write-out = "\n%{http_code}"
SESSION_SWEEP_CURL_CONFIG
)
status=${out##*"$nl"}
printf '%s\n' "${out%"$nl"*}"
case $status in
  2??) exit 0 ;;
esac
printf 'session sweep failed: HTTP %s\n' "$status" >&2
exit 1
{{- end }}
