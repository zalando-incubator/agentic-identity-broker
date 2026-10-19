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

1. For each `headersFromFile` entry, in sorted header-name order, it reads the file:
   - If the file is missing or empty, it prints `session sweep: header <Name> file <path> is missing or empty` to stderr and exits `1`.
   - It strips trailing CR and LF.
   - If the value contains `"`, `\`, CR, or LF, it prints `session sweep: header <Name> file <path> contains an unsupported character` and exits `1`.
2. It writes curl configuration lines to stdout, which is piped into `curl --config -`:
   - `header = "Content-Type: application/json"`
   - `header = "Host: <host>"`, only when a `Host` override applies (research R3)
   - one `header = "<Name>: <value>"` per literal header, escaped at render time (research R2)
   - one `header = "<Name>: <prefix><file value>"` per file header
3. It runs `curl --config - --silent --show-error --connect-timeout 10 --max-time <timeoutSeconds> --request POST --data '<body>' --write-out '\n%{http_code}' [--cert <certFile> --key <keyFile>] [--cacert <caFile>] '<url>/api/sessions/sweep'`.
4. It prints the response body.
5. For a `2xx` status, it exits `0`. For any other status, it prints `session sweep failed: HTTP <status>` to stderr and exits `1`. If curl fails without a status (transport error or timeout), the script exits with curl's code after curl prints its error.

The script never uses heredocs, `set -x`, `--verbose`, or `--trace`, and never echoes configuration lines.

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
read_header_file() { # name path
  [ -s "$2" ] || { printf 'session sweep: header %s file %s is missing or empty\n' "$1" "$2" >&2; exit 1; }
  v=$(tr -d '\r\n' < "$2")
  case $v in *[\"\\]*) printf 'session sweep: header %s file %s contains an unsupported character\n' "$1" "$2" >&2; exit 1;; esac
  printf '%s' "$v"
}
auth=$(read_header_file Authorization /var/run/secrets/session-sweep/token)
out=$(
  { printf '%s\n' 'header = "Content-Type: application/json"' \
                  'header = "Host: broker-admin.example.test"' \
                  'header = "X-Request-Source: session-sweep"'
    printf 'header = "Authorization: Bearer %s"\n' "$auth"
  } | curl --config - --silent --show-error --connect-timeout 10 --max-time 1800 \
      --request POST --data '{"lookahead_duration":"PT2H","dry_run":false}' \
      --write-out '\n%{http_code}' \
      'http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep'
)
status=${out##*
}
printf '%s\n' "${out%
*}"
case $status in 2??) exit 0 ;; esac
printf 'session sweep failed: HTTP %s\n' "$status" >&2
exit 1
```

This example fixes the observable behavior, not the exact text. Implementation may restructure it as long as the behavior above holds and the E2E tests pass. The file value is held in a shell variable inside the container and never reaches argv or the environment of another process. `printf` is a shell builtin in BusyBox `ash` and `dash`.
