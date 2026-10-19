# Contract: Rendered sweep resources and job behavior

**Feature**: `052-session-auto-refresh` | FR-002 to FR-016, SR-002 to SR-005 | Research R1 to R8

## Resources

With `sessionSweep.enabled: true`, the chart renders two resources. With it false, it renders nothing new (FR-001).

| Template | Kind | Name |
|---|---|---|
| `templates/serviceaccount-session-sweep.yaml` | `ServiceAccount` | `<fullname>-session-sweep` |
| `templates/cronjob-session-sweep.yaml` | `CronJob` (`batch/v1`) | `<fullname>-session-sweep` |

Neither resource is a Helm hook.

### ServiceAccount

- `automountServiceAccountToken: false`.
- Labels: `agentic-identity-broker.labels` plus `app.kubernetes.io/component: session-sweep`.

### CronJob (fields fixed by the chart)

```yaml
spec:
  schedule: <schedule>
  timeZone: <timeZone>                 # only when set
  concurrencyPolicy: Forbid
  startingDeadlineSeconds: <startingDeadlineSeconds>
  jobTemplate:
    spec:
      backoffLimit: 0
      activeDeadlineSeconds: <activeDeadlineSeconds>
      template:
        metadata:
          labels:                      # research R6; never app.kubernetes.io/name
            app.kubernetes.io/instance: <release>
            app.kubernetes.io/part-of: <chart name>
            app.kubernetes.io/managed-by: Helm
            # commonLabels, then sessionSweep.podLabels
            app.kubernetes.io/component: session-sweep
          annotations: <podAnnotations>
        spec:
          restartPolicy: Never
          serviceAccountName: <fullname>-session-sweep
          automountServiceAccountToken: false
          imagePullSecrets: <imagePullSecrets>
          securityContext: <podSecurityContext>
          containers:
            - name: session-sweep
              image: <repository>:<tag>
              imagePullPolicy: <pullPolicy>
              securityContext: <securityContext>   # readOnlyRootFilesystem, drop ALL, no escalation
              command: ["/bin/sh", "-ec", <script>]
              resources: <resources>
              volumeMounts: [tmp at /tmp] + <volumeMounts>
          volumes: [tmp emptyDir] + <volumes>
```

The CronJob metadata labels are `agentic-identity-broker.labels` plus `app.kubernetes.io/component: session-sweep`.

## Script contract

The script is rendered per release. It contains no secret: only the target, the `Host`, literal headers, file paths, and the JSON body. Its behavior:

1. For each `headersFromFile` entry, in sorted header-name order, it validates the file without passing its value anywhere:
   - If the file is missing or empty, it prints `session sweep: header file <path> is missing or empty` to stderr and exits `1`.
   - It removes one trailing LF run and one trailing CR. If the value is then empty or contains `"`, `\`, CR, or LF, it prints `session sweep: header file <path> contains an unsupported character` and exits `1`.
2. It runs `curl --disable --config -` and passes one configuration through a quoted heredoc, so the shell expands nothing:
   - `url`, `request = "POST"`, `header = "Content-Type: application/json"`
   - `header = "Host: <host>"`, only when a `Host` override applies (research R3)
   - one `header = "<Name>: <value>"` per literal header, escaped at render time (research R2)
   - per file header: `variable = "header_N@<path>"` and `expand-header = "<Name>: <prefix>{{header_N:trim:json}}"`, so curl reads the file itself
   - `data`, optional `cert`, `key`, and `cacert`, `connect-timeout = 10`, `max-time = <timeoutSeconds>`, `silent`, `show-error`, and `write-out = "\n%{http_code}"`
3. It prints the response body.
4. For a `2xx` status, it exits `0`. For any other status, it prints `session sweep failed: HTTP <status>` to stderr and exits `1`. If curl fails without a status (transport error or timeout), the script exits with curl's code after curl prints its error.

The script never uses `set -x`, `--verbose`, or `--trace`, and never echoes configuration lines. `--disable` comes first so curl ignores any `.curlrc` in the image. The image needs curl 8.3 or later for config variables.

### Request body

Rendered at template time, with keys in this order:

```json
{"lookahead_duration":"<lookaheadDuration>","dry_run":<dryRun>}
```

When `pageSize` is set, `,"page_size":<pageSize>` is added before the closing brace.

### Target

| `url` | `host` | Request URL | `Host` override |
|---|---|---|---|
| empty | empty | `http://<fullname>.<namespace>.svc:<service.ports.admin>` | `(urlParse broker.server.admin.publicUrl).host` if `publicUrl` is set, else none |
| empty | set | same | `host` |
| set | empty | `url` (trailing `/` trimmed) | none |
| set | set | `url` | `host` |

## Example: rendered script (in-cluster target, one literal and one file header)

```sh
nl='
'
cr=$(printf '\r')
check_header_file() {
  [ -s "$1" ] || { printf 'session sweep: header file %s is missing or empty\n' "$1" >&2; exit 1; }
  value=$(cat -- "$1")
  value=${value%"$cr"}
  case $value in
    ""|*"$nl"*|*"$cr"*|*\"*|*\\*) printf 'session sweep: header file %s contains an unsupported character\n' "$1" >&2; exit 1 ;;
  esac
}
check_header_file /var/run/secrets/session-sweep/token
out=$(curl --disable --config - <<'SESSION_SWEEP_CURL_CONFIG'
url = "http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep"
request = "POST"
header = "Content-Type: application/json"
header = "Host: broker-admin.example.test"
header = "X-Request-Source: session-sweep"
variable = "header_0@/var/run/secrets/session-sweep/token"
expand-header = "Authorization: Bearer {{header_0:trim:json}}"
data = "{\"lookahead_duration\":\"PT2H\",\"dry_run\":false}"
connect-timeout = 10
max-time = 1800
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
```

This example fixes the observable behavior, not the exact text. Implementation may restructure it as long as the behavior above holds and the E2E tests pass.
