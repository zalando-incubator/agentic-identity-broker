# Refresh third-party sessions

## How refresh works

If the access token is still valid, the broker returns it without waiting for a provider response. If that token is close to expiry, the broker attempts a background refresh without delaying the exchange.

If the access token has expired, the exchange waits for an on-demand refresh before responding. If the refresh succeeds, the broker updates the existing session and returns the new token. An absent or expired refresh token requires the user to reconnect. A provider response with 4xx `invalid_grant` (other than `429`) also requires reconnection. Other provider errors, including `429` and `5xx`, return `server_error` without requiring user reconnection. The broker does not return the expired access token as a fallback.

A sweep selects all sessions due within the lookahead window, including sessions with active exchanges. It also reaches inactive sessions that do not trigger background refresh. Sessions without a recorded access-token expiry are not included. The broker itself does not schedule sweeps. Run them on demand, enable the [chart-managed schedule](#use-the-chart-managed-schedule), or schedule them with another external job.

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

The broker Helm chart renders an optional CronJob that calls the sweep. Set `sessionSweep.enabled: true`; the [chart README](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/charts/agentic-identity-broker/README.md#scheduled-session-sweep) lists every value. The defaults run hourly with a `PT2H` lookahead, a 1800-second request timeout, a 2100-second Job deadline, and a one-hour late-start window. The CronJob never overlaps a running sweep and never retries a failed Job.

A run succeeds on any `2xx` response, even when `failed` is greater than zero. A failed candidate needs either the user to reconnect or the provider to recover, and an immediate retry would fail the same way. The next run tries it again, and the health metrics below show how many sessions are affected. Any other status, a timeout, or an unreachable target fails the Job.

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
| `session_sweep_refreshed_total` | Counter | Sessions refreshed by real sweeps. |
| `session_sweep_skipped_total` | Counter | Candidates a real sweep skipped because another refresh renewed them or they were deleted. |
| `session_sweep_failed_total` | Counter | Candidates that failed during real sweeps. |
| `proactive_refresh_triggered_total` | Counter | Background refresh attempts accepted during token exchange. |
| `proactive_refresh_dropped_total` | Counter | Background refresh attempts dropped under load. |
| `proactive_refresh_failed_total` | Counter | Accepted background refresh attempts that failed. |

`expiry` is `valid` when the access token expires after now plus `token_refresh.lookahead_duration`, `expiring` when it expires within that window, `expired` when it has already expired, and `unknown` when no expiry is recorded. `refreshable` is `true` when a refresh token is present and not expired. The series to watch is `expiry="expired",refreshable="true"`: these sessions the sweep could renew but has not, and inactive users accumulate there when no sweep runs. `expiry="expired",refreshable="false"` counts users who must reconnect. The sweep never reaches `unknown` sessions.

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
```

If your cluster runs kube-state-metrics, the CronJob status gives a second signal that does not depend on the broker:

```promql
time() - kube_cronjob_status_last_successful_time{cronjob=~".*-session-sweep"} > 3 * 3600
kube_job_status_failed{job_name=~".*-session-sweep-.*"} > 0
```

If `failed` persists across sweeps, examine the refresh failure logs and provider health. An absent or expired refresh token cannot recover through another sweep. A 4xx provider `invalid_grant` response (other than `429`) also requires user reconnection. Provider rate limits and outages can cause failures without requiring reconnection. Resolve those provider errors before you ask users to reconnect.

Users can reconnect through `GET /api/third-party/{serviceId}/oauth2/authorize`. They can end their own session through `DELETE /api/third-party/{serviceId}/session`. Ending a session removes the stored tokens and can interrupt agents. It does not revoke the tokens at the provider.

The sweep response contains counts, not the identities of affected users. Use your approved support process to contact users. Do not delete another user's session to clear a failed count.
