# Research: Automatic Third-Party Session Refresh

**Feature**: `053-session-auto-refresh` | **Date**: 2026-10-19 | **Spec**: [spec.md](spec.md)

Each entry records a decision, why it was chosen, and the alternatives considered. File references are to the repository at the start of planning.

## R1 — Where the request credentials live in the job

**Decision**: The script passes one curl configuration to `curl --disable --config -` through a quoted heredoc. Literal headers, the body, TLS paths, and timeouts are rendered as configuration lines. Each file header becomes `variable = "header_N@<path>"` plus `expand-header = "<Name>: <prefix>{{header_N:trim:json}}"`, so curl reads the file itself (curl 8.3 or later). Revised on 2026-10-19 after review; the first implementation had the shell read each file and pipe generated header lines to curl.

**Rationale**:
- A header passed with `--header` on the command line is visible in the process table to every process in the pod's PID namespace. With config variables, curl reads the file, so the value never reaches argv, the environment, or a pipe (SR-003). The shell only validates the file (R2).
- The quoted heredoc disables shell expansion, so every value needs only curl-config escaping. There is no shell-quoting layer.
- `trim` removes the file's trailing line ending. `json` escapes CR, LF, `"`, and `\`, so even a file that changes after validation cannot inject a header. It leaves token characters such as `.+/=_-~` unchanged (verified in `curlimages/curl:8.16.0`).
- `--disable` comes first so curl ignores any `.curlrc` in the image.

**Alternatives considered**:
- `--header "Authorization: Bearer $(cat file)"`: puts the credential in curl's argv.
- The first implementation, where the shell read the file into a variable and printed header lines to curl's stdin: it kept the value out of argv only while `printf` is a shell builtin, and it needed shell quoting on top of curl escaping.
- A rendered ConfigMap holding the curl configuration: splits one request across two resources and still needs the same variables for file values.
- An environment variable from a Secret: puts the credential in the environment, which SR-003 forbids.

## R2 — Escaping and validating values

**Decision**:
- The template escapes `\` and `"` in every value it writes into a curl configuration line.
- The values schema keeps CR and LF out of literal header values and `Host`, keeps braces out of file-header prefixes (they would start a curl expansion), and restricts header and TLS file paths to `^/[A-Za-z0-9._/-]+$`.
- curl reads each header file through a config variable and expands it with `trim` and `json`, so a value with a quote, backslash, CR, or LF cannot alter the request. A missing file fails curl before any request.

**Rationale**: Bearer tokens, JWTs, and API keys never contain these characters, so the escaping is a safety net rather than a validation step. The path pattern needs no shell quoting. A malformed or empty value reaches the target as a malformed credential and the target's rejection fails the job, which is diagnosable from the HTTP status in the log.

**Alternatives considered**: A shell pre-check that reads each file and rejects unsupported characters before calling curl. Dropped: it duplicated curl's escaping, put the credential in a shell variable, and needed its own run-time tests.

## R3 — Default target and `Host`

**Decision**: With `sessionSweep.url` unset, the target is `http://<fullname>.<namespace>.svc:<service.ports.admin>`. `Host` is `(urlParse .Values.broker.server.admin.publicUrl).host` when `publicUrl` is set. With `sessionSweep.url` set, the job sends `Host` only when `sessionSweep.host` is set.

**Rationale**: `RequireAdminHost` (`internal/adapters/http/middleware/admin_request.go:10-35`) compares the request `Host`, minus the scheme's default port, with the host of the admin public URL. Sprig's `urlParse` returns that host, including any non-default port. The `<name>.<namespace>.svc` form resolves through the standard pod DNS search path and does not assume the cluster domain. A proxy URL normally preserves its own host, so a derived override would be wrong there.

**Alternatives considered**:
- `svc.cluster.local`: breaks on clusters with a custom domain.
- Requiring `url`: removes the zero-configuration in-cluster path and the derived `Host`.
- Always sending the derived `Host`: breaks proxies whose external host differs from the broker's public URL.

## R4 — Render-time validation

**Decision**: Add the chart's first `fail` checks in a `session-sweep` helper:
1. `sessionSweep.enabled` with no `headers`, no `headersFromFile`, and no `tls.certFile` fails with the message in spec Journey 4.
2. `timeoutSeconds >= activeDeadlineSeconds` fails.
3. `tls.certFile` without `tls.keyFile`, or the reverse, fails.

Static shape and ranges (types, minimums, the ISO 8601 pattern, `additionalProperties: false`) go in `values.schema.json`.

**Rationale**: The schema cannot express "at least one of three properties is non-empty" with a readable message, and it cannot compare two values. No existing template uses `fail` or `required`, so these messages set the convention: name the value path, the constraint, and the fix.

**Alternatives considered**: Schema `anyOf` for the identity check: Helm reports it as a generic schema error that does not name the fix.

## R5 — ISO 8601 pattern in the values schema

**Decision**: `lookaheadDuration` uses `allOf` with three checks:
- `pattern: "^P([0-9]+D)?(T([0-9]+H)?([0-9]+M)?([0-9]+(\\.[0-9]+)?S)?)?$"`
- `not: {pattern: "T$"}`
- `pattern: "[1-9]"`

**Rationale**: This matches the grammar accepted by the sweep endpoint: days, hours, minutes, and seconds, with no years, months, or weeks (ADR 038). The nonzero-digit check rejects `P`, `P0D`, and `PT0S`, which the endpoint also rejects, so a zero lookahead fails at render time instead of on every scheduled run. It avoids lookahead, which a Go-regexp-based schema validator would reject. The broker still validates the value. The schema gives an earlier error.

**Alternatives considered**: One pattern with lookahead: not portable across Helm's schema validators.

## R6 — Pod labels must not join the broker Service

**Decision**: The job pod carries `app.kubernetes.io/instance`, `app.kubernetes.io/part-of: <chart name>`, `app.kubernetes.io/component: session-sweep`, and `app.kubernetes.io/managed-by`. It does not carry `app.kubernetes.io/name`. The labels merge in this order: base sweep labels, then `commonLabels`, then `sessionSweep.podLabels`, then the fixed `component` label.

**Rationale**: The broker Service and PodDisruptionBudget select `app.kubernetes.io/name` and `app.kubernetes.io/instance` (`templates/_helpers.tpl:54-57`, `service.yaml:19-20`, `pdb.yaml:16-18`). A running job pod with both labels would become a Service endpoint without a listener. `part-of` keeps the relationship visible. The fixed `component` label lets network policies select the job reliably (FR-016).

**Alternatives considered**: Reusing `agentic-identity-broker.podLabels`, as the migration Job does: it includes the selector labels. That existing migration behavior is outside this feature's scope and is deferred.

## R7 — Job image and filesystem

**Decision**: The default image is `curlimages/curl:8.16.0`, already used in the runbook. The command is `["/bin/sh", "-ec", <script>]`. It mounts an `emptyDir` at `/tmp`, as the migration Job does.

**Rationale**: The image has BusyBox `sh` and curl 8.16, which is all FR-014 requires. The `/tmp` mount covers shells that write heredocs or other scratch data to `/tmp` on a read-only root filesystem. `-e` stops on the first failed check. The script never enables `-x` (SR-005).

**Alternatives considered**: The broker image: it has no shell or curl. A purpose-built Go client: a new binary and image for one HTTP call.

**Verification of the first implementation (T032, 2026-10-19)**: In `curlimages/curl:8.16.0`, `type printf` reports `printf is a shell builtin` and `curl --version` reports `curl 8.16.0`. The rendered script, run in that image with a read-only root filesystem as UID 1000, did three things: sent a CRLF-terminated file token as `Bearer tok-abc123`, sent a literal header containing `'` and `"` unchanged, and rejected a file value containing `"` before sending.

## R8 — Job outcome and log

**Decision**: The script runs curl with these options:
- `--silent --show-error`
- `--connect-timeout 10 --max-time <timeoutSeconds>`
- `--write-out '\n%{http_code}'`

It captures the output and splits off the last line as the status. It prints the body. It exits `0` only for a `2xx` status. Otherwise it writes `session sweep failed: HTTP <status>` to stderr and exits `1`. A transport error or timeout makes curl exit non-zero, and `sh -e` stops the script with curl's error text.

**Rationale**: `--fail-with-body` fails only on status 400 and above. A `3xx`, such as a proxy redirecting to a login page, would exit `0` and violate FR-012. Checking the status class in the script covers every non-`2xx` response. It still prints the body (FR-013, spec Journey 3). curl never echoes request headers without `--verbose`.

**Alternatives considered**:
- `--fail-with-body` alone: passes `3xx` responses.
- Parsing the summary to fail on `failed > 0`: the spec makes those runs successful, and the shell would need a JSON parser.

## R9 — Health query shape and ownership of classification

**Decision**: Add one ISP port, `UserSessionHealthRepository.CountSessionsByExpiry(ctx, threshold, now)`. It returns rows of `(service_id, access_expiry, refresh_usable, count)`, where `access_expiry` is `unknown`, `after_threshold`, `due`, or `expired`. The domain maps each group to an `(expiry, refreshable)` key and zero-fills all eight keys per service.

PostgreSQL computes the groups in one query:

```sql
SELECT service_id,
       CASE WHEN access_token_expires_at IS NULL THEN 'unknown'
            WHEN access_token_expires_at > $1 THEN 'after_threshold'
            WHEN access_token_expires_at > $2 THEN 'due'
            ELSE 'expired' END AS access_expiry,
       (COALESCE(octet_length(encrypted_refresh_token), 0) > 0
        AND (refresh_token_expires_at IS NULL OR refresh_token_expires_at > $2)) AS refresh_usable,
       COUNT(*) AS session_count
FROM user_sessions
GROUP BY 1, 2, 3
```

The memory adapter applies the same predicates to its snapshots with the supplied `threshold` and `now`.

**Rationale**:
- The predicates mirror `UserSession.AccessTokenExpiresBy` (inclusive) and `CanRefresh` (`internal/domain/storage/user_session.go:125-133`), and the existing SQL refresh-presence test (`postgres/user_session.go:29-31`).
- The mapping to metric attributes (FR-018) stays in the domain service, not in SQL. That avoids an anemic domain.
- Passing `now` and `threshold` makes both adapters deterministic for parity tests. Only metadata columns are read (DB-003).
- No existing operation fits. `ListExpiringSessions` excludes NULL expiries, and `CountByService` has no breakdown (`ports/storage.go:268, 280-291`).

**Alternatives considered**:
- Returning the metric attributes from SQL: moves domain classification into two adapters.
- Paging `ListExpiringSessions`: misses sessions without a recorded expiry and loads token blobs.
- Extending `UserSessionRepository`: widens an existing ISP interface used by many callers.

## R10 — Query cost

**Decision**: Accept a sequential scan of `user_sessions` once per export cycle per replica. No index is added.

**Rationale**: The query reads every row by definition, so an index does not change the plan. The table has one row per user and service, and the default 30-second export interval keeps the cost low. Token blobs are not read, apart from `octet_length`, which reads the stored length. The spec's Assumptions let operators disable the gauge or lengthen the interval. `query_timeout` (5s default) bounds it through the caller's context; the adapter read timeout does not apply.

**Alternatives considered**: A cached snapshot on a separate ticker. The user dropped it on 2026-10-19 for the simpler design.

## R11 — Gauge registration and lifecycle

**Decision**: A new domain type `SessionHealthMetrics` in `internal/domain/oauth2session` registers the following on `otel.Meter("oauth2session")`:
- An `Int64ObservableGauge` named `oauth2_sessions`, unit `{session}`.
- One callback that runs the query under `context.WithTimeout(ctx, query_timeout)` and observes every `(service_id, expiry, refreshable)` combination.

On error, the callback observes nothing, logs `session health query failed` at WARN with the storage error kind, and returns `nil`. `builder.go` constructs it only when `token_refresh.health_metrics.enabled` is true, and calls `Registration.Unregister` in the shutdown chain before telemetry shuts down.

**Rationale**:
- Existing broker instruments are created unconditionally on the global meter. With metrics disabled, the global provider is the SDK no-op (`internal/adapters/telemetry/provider.go:61-62, 125-127`), and the no-op meter never invokes callbacks, so no query runs (FR-021).
- E2E suites install a `ManualReader` as the global provider and leave telemetry disabled (`tests/e2e/approval_api_test.go:136-143`). A builder gate on `telemetry.enabled` would make the gauge untestable there.
- Returning `nil` avoids a second, unstructured error from the OTel global handler for the same failure.
- The only existing observable gauge (`internal/extproc/approval/syncer.go:26-37`) never unregisters. This feature unregisters, so a stopped service cannot be queried.

**Alternatives considered**:
- Gating registration on `telemetry.enabled && telemetry.metrics.enabled` in the builder: duplicates the provider's gate and breaks the E2E pattern.
- Returning the error from the callback: logs twice.

## R12 — Sweep duration histogram placement

**Decision**: Record `session_sweep_duration_seconds` in the existing deferred audit block of `SessionSweepHandler.Sweep` (`internal/adapters/http/handlers/admin/session_sweep_handler.go:53-72`). Attribute it with the same `outcome` value and `dry_run`. Use `otel.Meter("oauth2session")`, unit `s`, and explicit bucket boundaries of `1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600` seconds.

**Rationale**:
- The handler is the only place that knows every outcome. `rejected` includes request-body parse failures that never reach the domain.
- Recording next to the audit event keeps the metric and the log consistent.
- A missing operator principal returns before `started` and before the deferred block, so it records nothing, matching FR-023 and spec Journey 3.
- The default SDK buckets are tuned for milliseconds. Sweeps take seconds to tens of minutes.
- Using the `oauth2session` scope keeps every sweep instrument in one scope.

**Alternatives considered**:
- Recording in `SessionSweepService.Sweep`: misses handler-level rejections.
- A separate runs counter: dropped. The histogram's count series counts runs (2026-10-19 clarification).

## R13 — Skipped counter

**Decision**: Add `session_sweep_skipped_total` (`Int64Counter`, unit `{refresh}`, attribute `triggered_by=sweep`) beside the existing counters in `NewSessionSweepService` (`sweep.go:55-65`). Increment it wherever a real sweep increments `result.Skipped`: a deleted candidate and a candidate no longer due. Dry runs do not increment it, matching the existing counters.

**Rationale**: It keeps the existing attribute set and naming. `refreshed + skipped + failed` then accounts for every evaluated real candidate.

**Alternatives considered**: None. The spec fixes the name and attribute.

## R14 — Testing the chart

**Decision**: Test the chart at render time only, in `tests/integration/helm_chart_test.go`, next to the existing chart render checks.
- `TestHelmTemplate_SessionSweep` runs `helm template` with per-case values and asserts on the CronJob defaults, the rendered curl configuration, the pod labels, and the three render-time failures.
- It skips when `helm` is not on `PATH`, like the existing chart tests, so no CI job needs a new dependency.
- The run-time behavior of the rendered script (file headers, TLS, exit codes) is verified manually against the admin server (quickstart §5) and by the author's run in `curlimages/curl:8.16.0`.

**Rationale**:
- A first version ran the rendered script from the backend E2E suite against the production-bootstrapped admin server. It needed `helm` and curl 8.3+ on every E2E runner, about 1,400 lines of harness (chart render, recording proxy, script runner, test CA), and the Zalando CDP pipeline had neither binary. That was out of proportion to a dozen lines of shell.
- Render checks cover what the chart itself decides: defaults, URL and `Host` derivation, the curl configuration lines, label disjointness, and the `fail` paths. curl's own behavior for `variable@file` and `expand-header` is not the chart's to test.
- Chart changes already count as `code` in the CI change filter (`ci.yml:76-84`), so the backend E2E job runs for them.

**Alternatives considered**:
- `helm-unittest`: a new plugin and YAML test format outside Ginkgo. It cannot run the script.
- The Helm Go SDK as a test dependency: a large new module for what one binary call does.
- Kind-based tests: slow, and not available in the backend E2E job.

## R15 — Configuration surface

**Decision**: Add `TokenRefreshConfig.HealthMetrics TokenRefreshHealthMetricsConfig` with `Enabled bool "enabled"` and `QueryTimeout time.Duration "query_timeout"`.
- Defaults are set with `SetDefault` at `internal/config/loader.go:210-212`, with explicit `BindEnv` calls and entries in the defaults key list (`:213-215`, `:304`).
- CLI flags are registered at `cmd/agentic-identity-broker/root.go:453-455` and in `bindFlags` (`loader.go:739-752`).
- Validation in `validateTokenRefreshConfig` (`internal/config/validator.go:688-699`) requires `query_timeout > 0` and uses `formatValidationError`.
- Helm adds `broker.tokenRefresh.healthMetrics.{enabled, queryTimeout}` to `values.yaml`, the strict schema (`values.schema.json:1173`, Go duration pattern at `:1180`), `templates/configmap.yaml:81-86`, and the README table (`README.md:179-181`).

**Rationale**: It follows the three existing `token_refresh` keys exactly. The loader never uses `AutomaticEnv`, so each key needs its own bindings.

**Alternatives considered**: Putting the switch under `telemetry.metrics`: the gauge belongs to session refresh and uses the refresh lookahead.

## R16 — Out of scope, recorded for follow-up

- The migration Job's pod labels match the broker Service selector (`templates/job-migrate.yaml:22-23`). This is unrelated to this feature. Constitution v4.1.0 defers it.
- Broker-side authentication and authorization of service callers on the admin API (spec Assumptions).

**R1 revision verification (2026-10-19)**: The revised script ran in `curlimages/curl:8.16.0` and `amazonlinux:2023` (curl 8.21) with a read-only root filesystem as UID 1000. A CRLF-terminated token was sent as `Bearer <token>`, and a literal header containing `'` and `"` arrived unchanged. Files containing `"`, `\`, or an embedded line break, empty files, and missing files each failed before any request. The echo server saw only the valid request.

