# Refresh third-party sessions

## How refresh works

If the session is not known rejected and access is still valid, the broker returns its access token without waiting for a provider response. Near expiry, it attempts background refresh without delaying exchange. Transient background failure and dropped work preserve otherwise valid access.

If access has expired and refresh credentials are available, exchange waits for on-demand refresh. Success updates the existing session and returns its new access token. Absent refresh credentials require reauthentication after access expiry.

HTTP 4xx `invalid_grant`, except 429, commits `refresh_token_rejected=true` before returning the recovery error. The row, ciphertext, and access expiry remain intact. Future broker issuance stops immediately, even while access remains unexpired. A failed rejection-state write reports a persistence error, not durable rejection. Other provider errors do not record rejection. On-demand provider 429/5xx failures remain `server_error`; expired access is never returned as a fallback.

A sweep selects due access tokens within its lookahead, including inactive sessions. Known-rejected records and unknown access expiry are excluded. Unknown expiry is not permanent validity. A stale candidate rejected after listing is skipped. Newly observed provider rejection counts as failed once and persists the flag; later sweeps exclude it. Missing-refresh candidates retain the existing failed classification. The broker has no sweep scheduler.

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

If failures persist across sweeps, examine refresh logs and provider health. Missing credentials cannot recover through another sweep. Recorded provider rejection requires successful reauthentication or explicit termination, not another refresh attempt. Provider outages and rate limits do not require reconnection by themselves.

Users can select Login on `/sessions` to start `GET /api/third-party/{serviceId}/oauth2/authorize`. A successful OAuth2 callback replaces tokens and clears rejection. Manual terminal refresh rejection returns 409 `refresh_rejected`; absent credentials return 409 `refresh_unavailable`; non-terminal upstream failures return 502 `refresh_failed`. Terminate remains available for retained records through `DELETE /api/third-party/{serviceId}/session`. Termination deletes stored tokens but does not revoke tokens already issued by the provider.

The sweep response contains counts, not the identities of affected users. Use your approved support process to contact users. Do not delete another user's session to clear a failed count.

## Online session validity rollout (migrations 038–040)

The stakeholder approved staged compatibility on 2026-10-14. This procedure replaces the earlier offline cutover in unmerged ADR 040. Keep the existing separate migration runner. Do not run deployment operations from an implementation or test session.

This PR ships migration 038 and the prepared broker, API, and SPA only. Migrations 039 and 040 are not present. This release defaults to target 38, with maximum 38. A normal Helm upgrade prepares the schema before rolling prepared brokers. Fresh installs run direct UP-all through 38. At 38, ordinary acquisition, refresh, and authorization work normally. True rejection writes fail as `persistence_failed`, not durable rejection. The memory adapter already supports the complete rejection lifecycle.

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
  targetVersion: 38
  backoffLimit: 0
broker:
  thirdPartyOauth2:
    sessionAPICompatibility: true
```

The 60-second example assumes the default 30-second broker shutdown and sufficient remaining time for traffic draining. Increase it for longer shutdown or drain intervals. Keep only one disruption-budget policy: `minAvailable` or `maxUnavailable`.

### Prepare, activate, and contract

1. With existing replicas serving, deploy PR1 through the normal Helm upgrade at its default target 38.
2. Complete the prepared broker and embedded SPA rollout across every replica.
3. Keep the default session API compatibility until all external consumers and the SPA support API v2.
4. Before merging or deploying PR2, verify that every unprepared replica and in-flight operation drained.
5. With prepared replicas serving, deploy PR2 at its default target 39 to activate durable rejection.
6. Before merging or deploying PR3, upgrade the SPA and every external session-API consumer.
7. After legacy schema dependencies end, deploy PR3 at its default target 40 and compatibility false.

| Release and target | Effect |
|---|---|
| PR1, 38 prepare (this release) | Preserve the original table shape for strict legacy `SELECT *` and old writes. Add the explicit `user_session_state` view and stable write functions. The view reports rejection=false. True rejection writes fail clearly rather than claiming persistence. |
| PR2, 39 activate (following PR) | Add the false-default rejection flag and replace the view/functions atomically. Keep the legacy column. Prepared replicas activate durable rejection without restart. Unprepared binaries must not coexist. |
| PR3, 40 cleanup (following PR) | Remove only the unused legacy column. Prepared and newer replicas keep the same SQL boundary. Default API compatibility becomes false after its consumer gate. |

Do not select targets 39 or 40 with this release. Existing unprepared binaries do not enforce recorded rejection. Schema 38 prepares PostgreSQL but does not activate durable rejection. The temporary legacy column supplies storage compatibility only. Historical dates never indicate provider validity or rejection. All phases preserve rows, ciphertext, real access expiry, and other token metadata. Migration 037 and its access-expiry index remain unchanged.

Helm `migration.targetVersion` accepts integers from 0 through 38 in this release. The default is 38. Fresh installations use direct `up` for all available migrations, including with the default target. PostgreSQL upgrades with migrations enabled refuse explicit target 0 during rendering. Positive upgrade targets inspect version and dirty state in both grants modes. Dirty, unreadable, or unexpected metadata aborts without `Force`. An equal/newer clean schema is a non-destructive no-op. Only a lower clean version runs `goto`. The wrapper requires an initialized schema and the existing image's `/bin/sh` and `/usr/local/bin/migrate` paths. Keep version-matched images, grants ordering, separate credentials, and serialized runners. Forward Helm stage controls never request DOWN. Use the separate runner's explicit rollback operation instead.

API `2.0.0` requires `requires_reauthentication` and removes refresh-expiry dates. Preparation and activation default compatibility to true. Compatibility adds only `is_expired`, equal to the new boolean. Modern consumers can explicitly set compatibility false. Cleanup defaults it to false after its consumer gate passes. Compatibility fabricates no refresh dates and does not change the rejection policy.

### Handle lock contention or migration failure

Each schema step is one atomic procedural statement. Expansion and contraction acquire `NOWAIT` locks on the view, then the underlying table. Lock contention aborts promptly without schema or row changes. Existing compatible replicas continue to serve, and a failed Helm hook leaves the existing Deployment in place. Short metadata locks can affect latency. There is no absolute latency guarantee.

1. If a migration fails, retain the compatible serving replicas.
2. Examine the migration error, actual schema phase, and `schema_migrations.version` and `dirty` values.
3. Resolve lock contention or the reported error before another migration attempt.
4. If metadata is dirty, select its repair version from the inspected schema.
5. Apply deliberate metadata repair through the separate runner, then retry the selected phase.

Never automate `Force` or infer the actual phase from the dirty version alone. `Force` changes metadata only. It cannot resolve rejection or change the schema. Disable automatic rollback to an unprepared image after phase 39.

### Roll back by compatible phase

This release supplies DOWN 038 only. The 039 and 040 rollback steps apply after their following PRs ship.

1. If schema 40 must return to 39, apply DOWN 040 with preparation or newer replicas still serving.
2. Before DOWN 039, resolve every rejected row through successful reauthentication or explicit user termination.
3. Apply DOWN 039 to schema 38 while preparation replicas continue to serve.
4. If an older broker is required, roll it across every replica only after schema 38 is established.
5. Before DOWN 038, drain every preparation or newer replica that uses the stable SQL boundary.
6. Apply DOWN 038 to schema 37 while matching old replicas continue to serve.

DOWN 040 restores a nullable legacy column only and retains rejection state. It fabricates no dates. DOWN 039 atomically restores preparation view/functions and removes the flag only after its rejection guard passes. A refused DOWN preserves schema and rows but can dirty metadata. Inspect both before manual repair. Never clear rejection or automatically delete sessions to bypass the guard.

Rollback locks also fail fast. On failure, keep the compatible replicas serving and use the same inspection procedure. Helm image rollback alone does not establish database compatibility. Do not deploy an unprepared binary at schema 39 or 40.

See [ADR 040](../../adrs/040-third-party-session-validity.md) for the approved contract and lifecycle decision.
