# Refresh third-party sessions

## How refresh works

If the access token is still valid, the broker returns it without waiting for a provider response. If that token is close to expiry, the broker attempts a background refresh without delaying the exchange.

If the access token has expired, the exchange waits for an on-demand refresh before responding. If the refresh succeeds, the broker updates the existing session and returns the new token. An absent or expired refresh token requires the user to reconnect. A provider response with 4xx `invalid_grant` (other than `429`) also requires reconnection. Other provider errors, including `429` and `5xx`, return `server_error` without requiring user reconnection. The broker does not return the expired access token as a fallback.

A sweep selects all sessions due within the lookahead window, including sessions with active exchanges. It also reaches inactive sessions that do not trigger background refresh. Sessions without a recorded access-token expiry are not included. The broker does not schedule sweeps. Administrators can run them on demand or schedule them outside the broker.

## Before you run a sweep

CAUTION: Send requests through your authenticated admin proxy. A sweep can refresh sessions for all users. Do not expose the admin server directly or send identity headers from a client. The proxy must authenticate and authorize the operator.

Use the proxy URL that matches the broker's configured admin public URL. The proxy must preserve its Host header. Send `Content-Type: application/json` with every request, including an empty JSON object.

Set the caller and proxy timeouts to allow for provider responses and the number of sessions due. A sweep waits for each refresh before it proceeds to the next session. If a request times out, earlier refreshes remain saved. Examine the audit event and session state before you start another sweep.

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

Choose a lookahead shorter than the shortest access-token lifetime that your providers issue. Otherwise, token exchanges can submit another refresh soon after one succeeds. Choose a schedule that reaches inactive sessions before their tokens expire. For example, use a five-minute schedule and ten-minute lookahead only if all relevant providers issue longer-lived access tokens. Allow for provider latency and missed runs when you set timeouts.

The broker Helm chart does not create a sweep CronJob. The following example is for an administrator-managed Kubernetes deployment. The CronJob omits `dry_run`, so each Job performs a real sweep. The example assumes that the admin proxy accepts service-account tokens with audience `broker-admin-proxy` and authorizes the service account as a sweep operator. If your proxy uses another authentication method, adapt the Job before you deploy it.

```yaml
apiVersion: batch/v1
kind: CronJob
metadata:
  name: broker-session-sweep
  namespace: identity-broker
spec:
  schedule: "*/5 * * * *"
  concurrencyPolicy: Forbid
  jobTemplate:
    spec:
      backoffLimit: 0
      activeDeadlineSeconds: 960
      template:
        spec:
          serviceAccountName: broker-session-sweeper
          automountServiceAccountToken: false
          restartPolicy: Never
          containers:
            - name: sweep
              image: curlimages/curl:8.16.0
              command: ["/bin/sh", "-eu", "-c"]
              args:
                - |
                  { printf 'header = "Authorization: Bearer '; tr -d '\n' </var/run/secrets/admin-proxy/token; printf '"\n'; } |
                    curl --config - --fail-with-body --silent --show-error \
                      --connect-timeout 10 --max-time 900 \
                      -H 'Content-Type: application/json' \
                      -d '{"lookahead_duration":"PT10M"}' \
                      'https://broker-admin.internal.example.com/api/sessions/sweep'
              volumeMounts:
                - name: admin-proxy-identity
                  mountPath: /var/run/secrets/admin-proxy
                  readOnly: true
          volumes:
            - name: admin-proxy-identity
              projected:
                sources:
                  - serviceAccountToken:
                      path: token
                      audience: broker-admin-proxy
                      expirationSeconds: 3600
```

Replace the namespace, service account, proxy URL, token audience, and schedule with your deployment values. Configure the proxy to validate the token issuer, signature, audience, expiry, and service-account identity. Restrict network access to the proxy and admin server. Do not put a reusable credential in the Job manifest or logs.

`Forbid` prevents overlap within this CronJob but not with manual sweeps or other schedules. `backoffLimit: 0` prevents automatic retries after a Job failure. Adjust the 900-second curl timeout and 960-second Job deadline to your workload. If the response is `200`, the example Job succeeds even with `failed` greater than zero. Monitor the response counts and missed or skipped Jobs.

## Monitor and resolve failures

If metrics are enabled, use the exported OpenTelemetry counters. The broker does not expose them on `/metrics` or `/health`.

| Counter | Meaning |
|---|---|
| `session_sweep_refreshed_total` | Sessions refreshed by real sweeps. |
| `session_sweep_failed_total` | Candidates that failed during real sweeps. |
| `proactive_refresh_triggered_total` | Background refresh attempts accepted during token exchange. |
| `proactive_refresh_dropped_total` | Background refresh attempts dropped under load. |
| `proactive_refresh_failed_total` | Accepted background refresh attempts that failed. |

Dry runs do not increment the sweep counters. The sweep audit event records the operator, counts, duration, and outcome without token values. The response also contains no tokens or user identities.

If `failed` persists across sweeps, examine the refresh failure logs and provider health. An absent or expired refresh token cannot recover through another sweep. A 4xx provider `invalid_grant` response (other than `429`) also requires user reconnection. Provider rate limits and outages can cause failures without requiring reconnection. Resolve those provider errors before you ask users to reconnect.

Users can reconnect through `GET /api/third-party/{serviceId}/oauth2/authorize`. They can end their own session through `DELETE /api/third-party/{serviceId}/session`. Ending a session removes the stored tokens and can interrupt agents. It does not revoke the tokens at the provider.

The sweep response contains counts, not the identities of affected users. Use your approved support process to contact users. Do not delete another user's session to clear a failed count.
