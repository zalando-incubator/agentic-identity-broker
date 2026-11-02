# Refresh third-party sessions

## How refresh works

If the session is not known rejected and access is still valid, the broker returns its access token without waiting for a provider response. Near expiry, it attempts background refresh without delaying exchange. Transient background failure and dropped work preserve otherwise valid access.

If access has expired and refresh credentials are available, exchange waits for on-demand refresh. Success updates the existing session and returns its new access token. Absent refresh credentials require reauthentication after access expiry.

HTTP 4xx `invalid_grant`, except 429, commits `refresh_token_rejected=true` before returning the recovery error. The row, ciphertext, and access expiry remain intact. Future broker issuance stops immediately, even while access remains unexpired. A failed rejection-state write reports a persistence error, not durable rejection. Other provider errors do not record rejection. On-demand provider 429/5xx failures remain `server_error`; expired access is never returned as a fallback.

A sweep selects due access tokens within its lookahead, including sessions with active exchanges and inactive sessions. Known-rejected records and unknown access expiry are excluded. Unknown expiry is not permanent validity. A stale candidate rejected after listing is skipped. Newly observed provider rejection counts as failed once and persists the flag; later sweeps exclude it. Missing-refresh candidates retain the existing failed classification.

The broker itself does not schedule sweeps. Run them on demand, enable the [chart-managed schedule](#use-the-chart-managed-schedule), or schedule them with another external job.

## Before you run a sweep

CAUTION: Send manual requests through your authenticated admin proxy. A sweep can refresh sessions for all users. Do not expose the admin server directly or send identity headers from a client. The proxy must authenticate and authorize the operator. For the chart-managed schedule, see the [trust requirements](#use-the-chart-managed-schedule) of each target.

Use the proxy URL that matches the broker's configured admin public URL. The proxy must preserve its Host header. Send `Content-Type: application/json` with every request, including an empty JSON object.

Set the caller and proxy timeouts to allow for provider responses and the number of sessions due. A sweep waits for each refresh before it proceeds to the next session. If a request times out, earlier refreshes remain saved. Examine the audit event and session state before you start another sweep.

Provider outages consume database capacity as well as HTTP time. Each refresh keeps its row lock and connection across up to three attempts and two `Retry-After` waits of up to ten seconds. PostgreSQL admits at most 20 locked-session operations across background refreshes, on-demand refreshes, sweeps, and reconnects. Excess operations wait before acquiring a connection and can fail within `storage.timeouts.write` (default `10s`). Five of the 25 pool connections remain outside locked-session work. A sequential sweep does not prevent other sweeps or on-demand sessions from competing for this capacity.

## Run a sweep

Run a dry run before you schedule the first sweep. The examples use an operator certificate and key for authentication at the proxy. If your proxy uses a different method, use its approved authentication method instead. Keep credentials in restricted files from your credential manager. Set `OPERATOR_CERT_FILE` and `OPERATOR_KEY_FILE` to those file paths.

CAUTION: If you omit `dry_run`, the broker starts a real sweep. Set `"dry_run":true` to preview without changing sessions.

```sh
ADMIN_PROXY_URL=https://broker-admin.internal.example.com
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error --max-time 900 \
  -H 'Content-Type: application/json' \
  -d '{"lookahead_duration":"PT10M","dry_run":true,"page_size":100}' \
  "$ADMIN_PROXY_URL/api/sessions/sweep"
```

If the dry-run counts match the sessions you expect, run a real sweep:

```sh
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error --max-time 900 \
  -H 'Content-Type: application/json' \
  -d '{"lookahead_duration":"PT10M","dry_run":false,"page_size":100}' \
  "$ADMIN_PROXY_URL/api/sessions/sweep"
```

CAUTION: Sending `{}` starts a real sweep with the configured lookahead and page size. Omitted `dry_run` defaults to `false`. `lookahead_duration` accepts a positive ISO 8601 duration, such as `PT10M` or `P1D`. It does not accept `10m`, weeks, months, or years. `page_size` accepts `1` through `1000`. The broker rejects invalid or unknown request fields with `400`.

A completed sweep returns a count-only response:

```json
{"refreshed":42,"skipped":3,"failed":1,"total_evaluated":46,"dry_run":false}
```

- `refreshed`: Sessions with a saved refresh. In a dry run, sessions eligible for refresh, with no provider call or saved change.
- `skipped`: Candidates that no longer require a refresh at evaluation time.
- `failed`: Candidates with no usable refresh token, or a provider error during a real sweep. A dry run cannot detect provider errors.
- `total_evaluated`: Sum of `refreshed`, `skipped`, and `failed`. Sessions outside the window do not contribute.

If every refresh fails but the sweep finishes, the endpoint still returns `200`. Alert on `failed`, not only on HTTP status. If the sweep stops with a server error or a timeout, earlier successful refreshes remain saved. Do not retry without reviewing the audit event and session state.

If you get `403`, check the proxy URL, Host header, and browser origin. If you get `415`, send a JSON content type. If you get `500 server_misconfiguration`, ask the proxy administrator to check operator identity delivery. The broker does not start a sweep without an operator identity.

## Schedule sweeps

Choose the schedule and lookahead together:

- The lookahead must cover the schedule interval plus a margin for one missed or late run. Otherwise, inactive sessions can expire between runs and wait for an on-demand refresh.
- The lookahead should be shorter than the shortest access-token lifetime that your providers issue. A token refreshed by one run expires within the next run's window when the lookahead is longer, so every run refreshes every session of that provider. This adds provider calls and locked-session load but does not break sessions.

For example, an hourly schedule with a two-hour lookahead (`PT2H`) suits providers that issue access tokens valid for more than two hours. Allow for provider latency and the number of sessions due when you set timeouts, and keep the Job deadline above the request timeout.

### Use the chart-managed schedule

The broker Helm chart renders an optional CronJob that calls the sweep. Set `sessionSweep.enabled: true`; the [chart README](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/charts/agentic-identity-broker/README.md#scheduled-session-sweep) lists every value. The defaults run hourly with a `PT2H` lookahead, a 1800-second request timeout, a 2100-second Job deadline, and a one-hour late-start window. The CronJob never overlaps a running sweep and, by default, never retries a failed Job.

A run succeeds on any `2xx` response, even when `failed` is greater than zero. A failed candidate needs either the user to reconnect or the provider to recover, and an immediate retry would fail the same way. The next run tries it again, and the health metrics below show how many sessions are affected. Any other status, a timeout, or an unreachable target fails the Job.

A rolling update of the broker can stop the replica that serves a sweep. The Job then fails and that run is lost. Sessions it already refreshed stay refreshed. To retry such a run, set `sessionSweep.backoffLimit: 1`. The retry evaluates only the sessions that are still due. `activeDeadlineSeconds` covers all attempts.

The chart has no identity model. Supply the credential that your admin path accepts as literal headers, headers read from mounted files, or a TLS client certificate; rendering fails without one. `curl` reads each header file itself at run time, so its value never appears in the manifest, a process argument list, the environment, or the log. A missing header file fails the run before any request is sent. `curl` trims the value and escapes any quote, backslash, or line break in it, so a malformed file cannot alter the request and fails with the target's status. Literal headers are rendered into the manifest, so never put secrets in them. Give the job a dedicated operator principal, so the sweep audit event distinguishes scheduled sweeps from manual ones.

Send the request through your authenticating proxy. With a projected ServiceAccount token:

```yaml
sessionSweep:
  enabled: true
  url: https://broker-admin.internal.example.com
  headersFromFile:
    Authorization:
      path: /var/run/secrets/session-sweep/token
      prefix: "Bearer "
  volumes:
    - name: proxy-token
      projected:
        sources:
          - serviceAccountToken:
              path: token
              audience: broker-admin-proxy
              expirationSeconds: 3600
  volumeMounts:
    - name: proxy-token
      mountPath: /var/run/secrets/session-sweep
      readOnly: true
```

Configure the proxy to validate the token issuer, signature, audience, expiry, and the `<fullname>-session-sweep` ServiceAccount, then forward the operator principal. With a TLS client certificate, set `sessionSweep.tls.certFile`, `keyFile`, and `caFile` to files from a mounted Secret instead.

The admin server authenticates the identity it receives but does not decide who is an operator; the proxy normally makes that decision. When `sessionSweep.url` is empty, the job calls this release's admin Service directly, which is trusted only as far as the admin server's own authentication goes. If you use that target, allow only the proxy and the sweep job to reach the admin port. For example:

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: broker-admin-port
  namespace: identity-broker
spec:
  podSelector:
    matchLabels:
      app.kubernetes.io/name: agentic-identity-broker
      app.kubernetes.io/instance: broker
  policyTypes: ["Ingress"]
  ingress:
    - ports:
        - port: 8000                              # end-user API
    - from:
        - podSelector:
            matchLabels:
              app.kubernetes.io/component: session-sweep
              app.kubernetes.io/instance: broker
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: admin-proxy   # your authenticating proxy
          podSelector:
            matchLabels:
              app.kubernetes.io/name: admin-proxy        # your proxy's pod labels
      ports:
        - port: 14000                             # admin API
```

Replace the namespace, release, and proxy selectors with your values. Keep `namespaceSelector` and `podSelector` in the same peer so that only the proxy pods in that namespace are admitted, not every pod there. A policy that selects the broker pods denies every ingress it does not list, so keep the end-user rule.

### Render the CronJob as plain manifests

Deployments that apply plain manifests can render the same resources with `helm template`:

```bash
helm template broker ./charts/agentic-identity-broker -n identity-broker -f values.yaml \
  --show-only templates/cronjob-session-sweep.yaml \
  --show-only templates/serviceaccount-session-sweep.yaml
```

Any other scheduler can send the same request. Apply the same lookahead, timeout, overlap, and credential guidance.

## Monitor and resolve failures

If metrics are enabled, use the exported OpenTelemetry instruments. The broker does not expose them on `/metrics` or `/health`.

| Instrument | Kind | Meaning |
|---|---|---|
| `oauth2_sessions{service_id, expiry, refreshable}` | Gauge | Stored sessions per service, access-token expiry class (`valid`, `expiring`, `expired`, or `unknown`), and whether a usable refresh token exists (`true` or `false`). Every combination is reported for a service with sessions, with zero when empty. |
| `session_sweep_duration_seconds{outcome, dry_run}` | Histogram | Duration of each sweep request with an operator principal. `outcome` is `completed`, `rejected`, `aborted`, or `canceled`. Its count series counts runs. |
| `session_sweep_refreshed_total{service_id}` | Counter | Sessions refreshed by real sweeps. |
| `session_sweep_skipped_total` | Counter | Candidates a real sweep skipped because another refresh renewed them or they were deleted. |
| `session_sweep_failed_total{service_id, failure_detail}` | Counter | Candidates that failed during real sweeps. `failure_detail` is the detail code that `session.oauth2.refresh_failed` logs, for example `refresh_rejected` or `access_token_expired`. |
| `proactive_refresh_triggered_total{service_id}` | Counter | Background refresh attempts accepted during token exchange. |
| `proactive_refresh_dropped_total` | Counter | Background refresh attempts dropped under load. |
| `proactive_refresh_failed_total{service_id, failure_detail}` | Counter | Accepted background refresh attempts that failed, with the same `failure_detail` codes. |

`service_id` is the service's canonical ID when one is set, otherwise its UUID. Canonical IDs are unique and never look like UUIDs, so each value names one service. Earlier releases reported the UUID for every service. Setting or changing a canonical ID starts new series for that service, so a query whose window spans the change sees both values. The counters look the canonical ID up when they record, and the gauge once per export; both use the UUID if that read fails. Logs keep the UUID in `service_id`.

`expiry` is `valid` when access expires after now plus `token_refresh.lookahead_duration`, `expiring` within that window, `expired` after access expiry, and `unknown` without recorded expiry. `refreshable` is `true` when refresh-token ciphertext is present and not rejected. The series `expiry="expired",refreshable="true"` identifies sessions that a sweep can attempt to renew. Inactive users accumulate there without sweeps. `expiry="expired",refreshable="false"` counts users who require reauthentication. The sweep excludes rejected and unknown-expiry sessions. Rejection can require reauthentication even while access remains unexpired.

Each replica runs one grouped count query for the gauge per metric export cycle, bounded by `token_refresh.health_metrics.query_timeout` alone; `storage.timeouts.read` does not apply to it. If the query fails or times out, that cycle exports no `oauth2_sessions` points and the broker logs `session health query failed` at WARN with `error_kind`. Set `token_refresh.health_metrics.enabled: false` to stop the query.

Every replica reports the same `oauth2_sessions` totals, so aggregate the gauge with `max`. Only the replica that served a sweep records its counters and duration, so aggregate those with `sum`. Backends that translate OTLP metric names, for example to Prometheus naming, may change suffixes; look up the translated names in your backend.

Dry runs do not increment the refreshed, skipped, or failed counters. The duration histogram records them with `dry_run="true"`, so alert on completed runs with `dry_run="false"`. The sweep audit event records the operator, counts, duration, and outcome without token values. The response also contains no tokens or user identities.

Example queries, in PromQL:

```promql
# Sessions per service, expiry class, and refreshability
max by (service_id, expiry, refreshable) (oauth2_sessions)

# Expired sessions the sweep could refresh but has not. Alert when this stays above
# zero for longer than two schedule periods: the sweep is not running or not keeping up.
max by (service_id) (oauth2_sessions{expiry="expired",refreshable="true"}) > 0

# Users who must reconnect, rising over a day
max by (service_id) (oauth2_sessions{expiry="expired",refreshable="false"})
  - max by (service_id) (oauth2_sessions{expiry="expired",refreshable="false"} offset 1d) > 0

# No completed real sweep in three hours (hourly schedule)
sum(increase(session_sweep_duration_seconds_count{outcome="completed",dry_run="false"}[3h])) == 0

# Health gauge missing for fifteen minutes although it reported sessions before
absent_over_time(oauth2_sessions[15m])
  and on() (max(max_over_time(oauth2_sessions[1h] offset 15m)) > 0)

# Sweep failures over a day
sum(increase(session_sweep_failed_total[1d])) > 0

# Refresh failures per service and reason over the last hour
sum by (service_id, failure_detail) (
  increase(session_sweep_failed_total[1h]) or increase(proactive_refresh_failed_total[1h])
)
```

A replica creates each counter series, and the histogram's count series, on its first event for those attributes. The first exported value already includes that event. Backends that ignore the OTLP start time use it as the baseline, so `increase()` and `rate()` miss it. A service that starts failing can then show no failures until a sweep runs on the same replica again. In such a backend, subtract the value from an hour ago and count a series that did not exist then from zero:

```promql
# Sweep failures per service and reason over the last hour, including new series
sum by (service_id, failure_detail) (
  max_over_time(session_sweep_failed_total[1h])
    - (session_sweep_failed_total offset 1h or max_over_time(session_sweep_failed_total[1h]) * 0)
)
```

If your cluster runs kube-state-metrics, the CronJob status gives a second signal that does not depend on the broker:

```promql
time() - kube_cronjob_status_last_successful_time{cronjob=~".*-session-sweep"} > 3 * 3600
kube_job_status_failed{job_name=~".*-session-sweep-.*"} > 0
```

If failures persist across sweeps, examine refresh logs and provider health. Missing credentials cannot recover through another sweep. Recorded provider rejection requires successful reauthentication or explicit termination, not another refresh attempt. Provider outages and rate limits do not require reconnection by themselves.

Users can select Login on `/sessions` to start `GET /api/third-party/{serviceId}/oauth2/authorize`. A successful OAuth2 callback replaces tokens and clears rejection. Manual terminal refresh rejection returns 409 `refresh_rejected`; absent credentials return 409 `refresh_unavailable`; non-terminal upstream failures return 502 `refresh_failed`. Terminate remains available for retained records through `DELETE /api/third-party/{serviceId}/session`. Termination deletes stored tokens but does not revoke tokens already issued by the provider.

The sweep response contains counts, not the identities of affected users. Use your approved support process to contact users. Do not delete another user's session to clear a failed count.

## Online session validity rollout (migrations 046–048)

The stakeholder approved staged compatibility on 2026-10-14. This procedure replaces the earlier offline cutover in unmerged ADR 045. Keep the existing separate migration runner. Do not run deployment operations from an implementation or test session.

This PR ships migration 046 and the prepared broker, API, and SPA only. Migrations 047 and 048 are not present. This release defaults to target 46, with maximum 46. A normal Helm upgrade prepares the schema before rolling prepared brokers. Fresh installs run direct UP-all through 46. At 46, ordinary acquisition, refresh, and authorization work normally. True rejection writes fail as `persistence_failed`, not durable rejection. The memory adapter already supports the complete rejection lifecycle.

### Availability prerequisites

1. Keep at least two healthy broker replicas throughout the rollout.
2. If autoscaling is enabled, set its minimum to at least two replicas.
3. Use `RollingUpdate` with `maxUnavailable=0` and `maxSurge` of at least one.
4. Reserve capacity for a ready replacement before an old pod terminates.
5. Enable a disruption budget that protects the required healthy capacity during voluntary evictions.
6. Set termination grace longer than broker shutdown plus the required load-balancer drain interval.
7. Verify readiness and traffic draining under the deployment's actual request load.
8. Retain immutable hashed SPA assets from running and replacement releases for rolling updates and already-open clients.
9. Serialize migration runners for this database across all deployment workflows.

The chart defaults to safe rolling limits but keeps operator strategy overrides. `Recreate`, positive `maxUnavailable`, or unavailable surge capacity cannot satisfy this procedure. A disruption budget does not control Deployment rolling updates. Routing must stop new requests to a terminating pod and let existing requests finish. Chart defaults alone do not prove this behavior.

The existing asset-hosting arrangement must provide continuity through a shared origin, retained bundles, or verified routing. SQL migration proof and `maxUnavailable=0` alone do not prove browser continuity. If assets cannot remain available, do not begin the rollout. Keep the existing release serving. No new deployment stack is required.

Use these chart values for preparation, alongside the deployment's existing credentials, images, ingress, and resource configuration:

```yaml
replicaCount: 2
autoscaling:
  minReplicas: 2
strategy:
  type: RollingUpdate
  rollingUpdate:
    maxUnavailable: 0
    maxSurge: 1
podDisruptionBudget:
  enabled: true
  minAvailable: 2
terminationGracePeriodSeconds: 60
migration:
  targetVersion: 46
  backoffLimit: 0
broker:
  thirdPartyOauth2:
    sessionAPICompatibility: true
```

The 60-second example assumes the default 30-second broker shutdown and sufficient remaining time for traffic draining. Increase it for longer shutdown or drain intervals. Keep only one disruption-budget policy: `minAvailable` or `maxUnavailable`.

### Prepare, activate, and contract

1. With existing replicas serving, deploy PR1 through the normal Helm upgrade at its default target 46.
2. Complete the prepared broker and embedded SPA rollout across every replica.
3. Keep the default session API compatibility until all external consumers and the SPA support API v2.
4. Before merging or deploying PR2, verify that every unprepared replica and in-flight operation drained.
5. With prepared replicas serving, deploy PR2 at its default target 47 to activate durable rejection.
6. Before merging or deploying PR3, upgrade the SPA and every external session-API consumer.
7. After legacy schema dependencies end, deploy PR3 at its default target 48 and compatibility false.

| Release and target | Effect |
|---|---|
| PR1, 46 prepare (this release) | Preserve the original table shape for strict legacy `SELECT *` and old writes. Add the explicit `user_session_state` view and stable write functions. The view reports rejection=false. True rejection writes fail clearly rather than claiming persistence. |
| PR2, 47 activate (following PR) | Add the false-default rejection flag and replace the view/functions atomically. Keep the legacy column. Prepared replicas activate durable rejection without restart. Unprepared binaries must not coexist. |
| PR3, 48 cleanup (following PR) | Remove only the unused legacy column. Prepared and newer replicas keep the same SQL boundary. Default API compatibility becomes false after its consumer gate. |

Do not select targets 47 or 48 with this release. Existing unprepared binaries do not enforce recorded rejection. Schema 46 prepares PostgreSQL but does not activate durable rejection. The temporary legacy column supplies storage compatibility only. Historical dates never indicate provider validity or rejection. All phases preserve rows, ciphertext, real access expiry, and other token metadata. Migration 037 and its access-expiry index remain unchanged.

Helm `migration.targetVersion` accepts integers from 0 through 46 in this release. The default is 46. Fresh installations use direct `up` for all available migrations, including with the default target. PostgreSQL upgrades with migrations enabled refuse targets below 46 during rendering. Accepted upgrade targets inspect version and dirty state in both grants modes. Dirty, unreadable, or unexpected metadata aborts without `Force`. An equal/newer clean schema is a non-destructive no-op. Only a lower clean version runs `goto`. The wrapper requires an initialized schema and the existing image's `/bin/sh` and `/usr/local/bin/migrate` paths. Keep version-matched images, grants ordering, separate credentials, and serialized runners. Forward Helm stage controls never request DOWN. Use the separate runner's explicit rollback operation instead.

API `2.0.0` requires `requires_reauthentication` and removes refresh-expiry dates. Preparation and activation default compatibility to true. Compatibility adds only `is_expired`, equal to the new boolean. Modern consumers can explicitly set compatibility false. Cleanup defaults it to false after its consumer gate passes. Compatibility fabricates no refresh dates and does not change the rejection policy.

The SPA also supports old brokers during preparation rollout. It normalizes legacy responses before caching them, using the former Login rule only when `requires_reauthentication` is absent. Successful authorization replacement clears the legacy refresh-expiry marker; refresh clears it only when replacing the stored refresh credential. Retaining the same credential preserves its legacy marker. Prepared brokers never use that timestamp as validity evidence.

### Handle lock contention or migration failure

Each schema step is one atomic procedural statement. Expansion and contraction acquire `NOWAIT` locks on the view, then the underlying table. Lock contention aborts promptly without schema or row changes. Existing compatible replicas continue to serve, and a failed Helm hook leaves the existing Deployment in place. Short metadata locks can affect latency. There is no absolute latency guarantee.

1. If a migration fails, retain the compatible serving replicas.
2. Examine the migration error, actual schema phase, and `schema_migrations.version` and `dirty` values.
3. Resolve lock contention or the reported error before another migration attempt.
4. If metadata is dirty, select its repair version from the inspected schema.
5. Apply deliberate metadata repair through the separate runner, then retry the selected phase.

Never automate `Force` or infer the actual phase from the dirty version alone. `Force` changes metadata only. It cannot resolve rejection or change the schema. Disable automatic rollback to an unprepared image after phase 47.

### Roll back by compatible phase

This release supplies DOWN 046 only. The 047 and 048 rollback steps apply after their following PRs ship.

1. If schema 48 must return to 47, apply DOWN 048 with preparation or newer replicas still serving.
2. Before DOWN 047, resolve every rejected row through successful reauthentication or explicit user termination.
3. Apply DOWN 047 to schema 46 while preparation replicas continue to serve.
4. If an older broker is required, roll it across every replica only after schema 46 is established.
5. Before DOWN 046, drain every preparation or newer replica that uses the stable SQL boundary.
6. Apply DOWN 046 to schema 45 while matching old replicas continue to serve.

DOWN 048 restores a nullable legacy column only and retains rejection state. It fabricates no dates. DOWN 047 atomically restores preparation view/functions and removes the flag only after its rejection guard passes. A refused DOWN preserves schema and rows but can dirty metadata. Inspect both before manual repair. Never clear rejection or automatically delete sessions to bypass the guard.

Rollback locks also fail fast. On failure, keep the compatible replicas serving and use the same inspection procedure. Helm image rollback alone does not establish database compatibility. Do not deploy an unprepared binary at schema 47 or 48.

See [ADR 045](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/adrs/045-third-party-session-validity.md) for the approved contract and lifecycle decision.
