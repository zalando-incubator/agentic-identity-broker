# Contract: Helm values for the session sweep

**Feature**: `053-session-auto-refresh` | FR-001 to FR-016 | Research R3 to R7

Operators write these values, so after release they become a compatibility surface. `values.schema.json` gains a strict `sessionSweep` object (`additionalProperties: false` at every level), and `values.yaml` documents each key with a `# --` comment.

## `values.yaml` block

```yaml
# Scheduled session sweep (CronJob calling POST /api/sessions/sweep)
sessionSweep:
  # -- Render the CronJob and its ServiceAccount
  enabled: false
  # -- Cron schedule. Keep lookaheadDuration longer than one interval plus a missed run.
  schedule: "0 * * * *"
  # -- Optional IANA time zone for the schedule
  timeZone: ""
  # -- Seconds after the scheduled time within which a late run may still start
  startingDeadlineSeconds: 3600
  # -- Hard cap on one Job, in seconds. Must exceed timeoutSeconds.
  activeDeadlineSeconds: 2100
  # -- Request timeout in seconds, passed to curl --max-time
  timeoutSeconds: 1800
  # -- ISO 8601 lookahead sent as lookahead_duration (days, hours, minutes, seconds only)
  lookaheadDuration: PT2H
  # -- Send dry_run=true: classify candidates without refreshing
  dryRun: false
  # -- Optional page_size (1-1000); unset uses token_refresh.sweep.default_page_size
  pageSize: null
  # -- Target URL. Empty targets this release's admin Service: http://<fullname>.<namespace>.svc:<service.ports.admin>
  url: ""
  # -- Host header override. Empty derives it from broker.server.admin.publicUrl, only for the default target.
  host: ""
  # -- Literal request headers. They are rendered into the manifest; never put secrets here.
  headers: {}
  # -- Headers read from files at run time: {Header-Name: {path: /mounted/file, prefix: "Bearer "}}
  headersFromFile: {}
  tls:
    # -- Path of a PEM client certificate presented to the target
    certFile: ""
    # -- Path of the PEM private key for certFile
    keyFile: ""
    # -- Path of a PEM CA bundle used to verify the target
    caFile: ""
  # -- Volumes that provide header and TLS files
  volumes: []
  # -- Mounts for those volumes in the sweep container
  volumeMounts: []
  image:
    # -- Image with a POSIX shell and curl 8.3 or later
    repository: curlimages/curl
    # -- Image tag
    tag: "8.16.0"
    # -- Image pull policy
    pullPolicy: IfNotPresent
  # -- Resource requests and limits
  resources:
    requests:
      cpu: 50m
      memory: 64Mi
    limits:
      cpu: 50m
      memory: 64Mi
  # -- Extra pod labels; override commonLabels for shared keys
  podLabels: {}
  # -- Extra pod annotations
  podAnnotations: {}
```

## Schema rules

| Path | Type and constraint |
|---|---|
| `enabled`, `dryRun` | boolean |
| `schedule` | string, `minLength: 1` |
| `timeZone` | string |
| `startingDeadlineSeconds`, `activeDeadlineSeconds`, `timeoutSeconds` | integer, `minimum: 1` |
| `lookaheadDuration` | string, ISO 8601 pattern from research R5 |
| `pageSize` | `null` or integer `1..1000` |
| `url` | string; empty or `^https?://` without whitespace |
| `host` | string without CR or LF |
| `headers` | object; property names `^[!#$%&'*+.^_`|~0-9A-Za-z-]+$`, values without CR or LF |
| `headersFromFile` | object; same property names; each value `{path, prefix}` with `required: [path]`; `path` matches `^/[A-Za-z0-9._/-]+$`; `prefix` has no CR, LF, `{`, or `}` |
| `tls.certFile`, `tls.keyFile`, `tls.caFile` | empty or matching `^/[A-Za-z0-9._/-]+$` |
| `volumes`, `volumeMounts` | array of objects (Kubernetes shapes are not re-validated) |
| `image.repository`, `image.tag` | string, `minLength: 1` |
| `image.pullPolicy` | enum `Always`, `IfNotPresent`, `Never` |
| `resources` | `$ref: #/definitions/resourceRequirements` |
| `podLabels`, `podAnnotations` | object of strings |

## Render-time checks (`fail`)

Evaluated only when `sessionSweep.enabled` is true. Messages are exact.

| Condition | Message |
|---|---|
| `headers`, `headersFromFile`, and `tls.certFile` all empty | `sessionSweep.enabled requires an identity for the admin API: set sessionSweep.headers, sessionSweep.headersFromFile, or sessionSweep.tls.certFile.` |
| `timeoutSeconds >= activeDeadlineSeconds` | `sessionSweep.timeoutSeconds (<t>) must be shorter than sessionSweep.activeDeadlineSeconds (<d>).` |
| exactly one of `tls.certFile`, `tls.keyFile` set | `sessionSweep.tls.certFile and sessionSweep.tls.keyFile must be set together.` |

## Broker values (existing section)

```yaml
broker:
  tokenRefresh:
    healthMetrics:
      # -- Publish the oauth2_sessions gauge when telemetry metrics are enabled
      enabled: true
      # -- Timeout of the per-export health count query (Go duration)
      queryTimeout: 5s
```

The schema adds `healthMetrics` to `broker.tokenRefresh` with `enabled: boolean` and `queryTimeout` using the existing Go duration pattern (`values.schema.json:1180`). `templates/configmap.yaml` renders them as `token_refresh.health_metrics.enabled` and `query_timeout`.

## README

`charts/agentic-identity-broker/README.md` gains a "Scheduled session sweep" section:
- the values table
- two recipes, both through the authenticating proxy: a projected ServiceAccount token, and a TLS client certificate. `headers` is documented only as a mechanism for non-secret headers, sent as given and rendered into the manifest, with no direct-call recipe.
- the trust guidance from FR-025
- a `helm template --show-only templates/cronjob-session-sweep.yaml` example

It also replaces line 40 ("route sweep CronJobs through that proxy") and adds `broker.tokenRefresh.healthMetrics.*` to the parameter table (`README.md:179-181`).
