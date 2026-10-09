# Session sweep operations

## Purpose and access

`POST /api/sessions/sweep` runs synchronously on the admin server (port `14000`). It finds third-party sessions whose access tokens expire within the lookahead window. It processes candidates across all users and services, one at a time in bounded pages. Sessions without a recorded expiry are not candidates. The broker has no internal sweep scheduler or distributed lock.

CAUTION: Send sweep requests only through the authenticated admin proxy. The endpoint can refresh sessions across all users.

The proxy must authenticate and authorize the operator. With plain-header pre-authentication, it must remove caller-supplied principal headers and set the configured admin header (usually `X-Remote-User`). With JWT pre-authentication, it must forward a signed operator JWT that the configured JWKS validates; the broker ignores the plain header. Unsigned JWT pre-authentication fails at startup. Do not expose the admin server directly. The broker audits each invocation with the operator principal. A missing principal returns `500 server_misconfiguration`; no sweep starts.

The Helm chart denies ingress to port `14000` by default. Set `networkPolicy.adminProxy.namespaceLabels` and `networkPolicy.adminProxy.podLabels` to the trusted proxy labels. Keep `service.type: ClusterIP`. The CNI must enforce ingress NetworkPolicy. If you disable the chart policy, install an equivalent restriction before you run a sweep. Do not allow the CronJob to connect to the broker Service directly.

The request must use the Host authority configured by `broker.server.admin.publicUrl` (`server.admin.public_url` in broker configuration). The proxy must preserve that Host. The admin API ignores forwarded Host headers. Set `Content-Type: application/json` even for `{}`. Host and browser-boundary failures return `403`; a missing or invalid JSON content type returns `415`.

## Run a sweep

Before the first scheduled request, run a dry run through the admin proxy. These examples use mTLS operator authentication. If your proxy uses another method, configure its approved curl authentication first. Store the certificate and private key in restricted files from your credential manager. Set `OPERATOR_CERT_FILE` and `OPERATOR_KEY_FILE` to those paths. Do not put credentials or principal headers in the commands.

```sh
ADMIN_PROXY_URL=https://broker-admin.internal.example.com
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error --max-time 900 \
  -H 'Content-Type: application/json' \
  -d '{"lookahead_duration":"PT10M","dry_run":true,"page_size":100}' \
  "$ADMIN_PROXY_URL/api/sessions/sweep"
```

If the dry-run counts match the expected candidate population, run the real sweep:

```sh
curl --cert "$OPERATOR_CERT_FILE" --key "$OPERATOR_KEY_FILE" \
  --fail-with-body --silent --show-error --max-time 900 \
  -H 'Content-Type: application/json' \
  -d '{"lookahead_duration":"PT10M","dry_run":false,"page_size":100}' \
  "$ADMIN_PROXY_URL/api/sessions/sweep"
```

The request accepts an empty JSON object to use configured defaults. `lookahead_duration` accepts a positive ISO 8601 duration of days, hours, minutes, and seconds, such as `PT10M` or `P1D`. It does not accept `10m`, weeks, months, or years. `page_size` is from `1` to `1000`. Invalid input, including unknown fields, returns `400`.

The broker clears its write deadline only for this route. Set caller and admin-proxy upstream timeouts long enough for sequential provider calls. A disconnect or timeout stops the scan after the current session. Committed refreshes remain committed. Do not blindly retry a timed-out or `500` sweep. Inspect the audit event and stored state before you decide whether to run another sweep.

A successful response contains counts only, never tokens or end-user principals:

```json
{"refreshed":42,"skipped":3,"failed":1,"total_evaluated":46,"dry_run":false}
```

`total_evaluated = refreshed + skipped + failed`. `refreshed` means a stored refresh succeeded. In a dry run, it means a refresh would occur without an upstream call or write. `skipped` means another request renewed the candidate or a user deleted it before evaluation. `failed` means the candidate could not refresh, for example because its refresh token expired or its provider failed. Sessions outside the window appear in no count. A completed sweep returns `200` even if **every** candidate failed. Alert on `failed`, not just HTTP status. A repository failure returns `500` and leaves earlier committed refreshes intact.

## Schedule outside the broker chart

Choose a lookahead shorter than the shortest access-token lifetime from your providers. Otherwise, every exchange can trigger another refresh. Allow enough lead time and sweep frequency for inactive sessions. For example, run every five minutes with a ten-minute lookahead only if all relevant providers issue longer-lived access tokens. Include provider latency and skipped runs when you choose the caller timeout. The default broker lookahead is `5m`, with `10` background workers and a sweep page size of `100`. The worker cap is `20` per replica because the database pool has `25` connections.

This reference CronJob is **operator-owned**. The broker Helm chart does not create or manage a sweep CronJob. The example assumes the admin proxy validates short-lived Kubernetes service-account tokens for audience `broker-admin-proxy`. The proxy authorizes the service account as an operator and injects the principal header. If your proxy does not support this trust model, use its approved authentication method instead. Create the dedicated service account and configure the proxy. Do not grant this service account database or broker-pod access.

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

Replace the example namespace, service account, proxy URL, token audience, and schedule with deployment values. Set the proxy URL Host to match `broker.server.admin.publicUrl`. The proxy must validate the token issuer, signature, audience, expiry, and service-account identity before it sets the principal header. Restrict access to the proxy and broker admin port with network policy. Do not put a reusable credential in the manifest, command arguments, ConfigMap, or logs. The example streams a projected token to curl through standard input. It does not print or store the token in the Job specification.

`Forbid` prevents overlap within this CronJob, not requests from other schedules or operators. `backoffLimit: 0` prevents automatic retries after a Job failure. The 900-second curl timeout and 960-second Job deadline are examples. Set both for your workload. A `200` with `failed > 0` still finishes this example Job successfully. Inspect its count-only output or collect the exported metrics. Alert on persistent failures and missed or skipped CronJob runs.

## Observe outcomes

The broker emits five OpenTelemetry counters when metrics are enabled. They are not exposed on `/metrics` or `/health`:

| Counter | `triggered_by` | Meaning |
|---|---|---|
| `proactive_refresh_triggered_total` | `background` | Background slot accepted a refresh. |
| `proactive_refresh_dropped_total` | `background` | All background slots were busy. |
| `proactive_refresh_failed_total` | `background` | Accepted background refresh ended in an error. |
| `session_sweep_refreshed_total` | `sweep` | Real sweep committed a refreshed session. |
| `session_sweep_failed_total` | `sweep` | Real sweep classified a candidate as failed. |

These counters have only the fixed-cardinality `triggered_by` attribute. They do not include session IDs, service IDs, or principals. Dry runs add no sweep counts. The session-service logs identify refresh events without end-user principals or token material. The separate `session sweep` audit event records the operator principal, counts, duration, and outcome. A background refresh creates a root span linked to the request span that triggered it.

## Recover migration `036` (ADR 039)

CAUTION: Migration `036` uses concurrent PostgreSQL index DDL outside a transaction. Invalid-index cleanup is **not** atomic rollback.

If a migration UP or DOWN fails, stop competing migration Jobs before you touch metadata or retry. The Helm migration Job can retry pods, but `golang-migrate` refuses a dirty version. Suspend competing releases. Arrange a database review. Use the same migration role and schema search path as the failed Job. Inspect its error and the `schema_migrations` row. Then inspect **all** relations with the target name:

```sql
SELECT version, dirty FROM schema_migrations;
SELECT current_schema(), current_schemas(true);
SELECT n.nspname AS schema_name, c.relname, c.relkind,
       i.indrelid::regclass AS table_name, i.indisvalid,
       pg_get_indexdef(c.oid) AS definition
FROM pg_class AS c
JOIN pg_namespace AS n ON n.oid = c.relnamespace
LEFT JOIN pg_index AS i ON i.indexrelid = c.oid
WHERE c.relname = 'idx_user_sessions_access_token_expires_at';
```

Confirm that the expected index is a plain B-tree index on `user_sessions(access_token_expires_at)` in the migration schema. `pg_index.indisvalid = false` means the index is invalid; it can still add write overhead. If there is no row, the index is absent. If another relation owns the name, stop and investigate. Do not delete that relation. A clean version `36` with a missing or invalid index also requires database review.

If UP stopped at **dirty version 36** and the inspected index is invalid, drop only that index. Substitute its *verified* schema for `confirmed_schema` in this example. Run the statement with an autocommit SQL client. Do not use `BEGIN`, `COMMIT`, or `psql -1`:

```sql
DROP INDEX CONCURRENTLY confirmed_schema.idx_user_sessions_access_token_expires_at;
```

If UP stopped at dirty version `36` and the index is absent, investigate the original error. When the entire schema matches version `35`, use `force 35` to clear dirty metadata. Then retry UP through the guarded migration image. If UP stopped at dirty version `36` and the index is **valid** with the expected definition, review the entire schema. Then use `force 36`. Do not drop the valid index or rerun UP.

If DOWN stopped at **dirty version 35** and the index is still present, inspect its identity and validity. After schema review, use `force 36` and retry DOWN through the guarded migration image. If DOWN stopped at dirty version `35` and the index is absent, review the failure and confirm that DOWN completed. Then use `force 35` without rerunning DOWN.

Run only the command for the branch that the database review selected. Use a restricted maintenance container built from the chart's separate `migration.image`. It contains `/app/migrations`; connect with the migration role. The chart uses `/usr/local/bin/migration-guard` as the entrypoint. Do not invoke the unguarded `/usr/local/bin/migrate`. Supply the connection URL through approved secret handling, not a literal URL in a Job manifest or shell history. A password in a command-line URL can appear in process arguments. Restrict maintenance-shell access and prefer a short-lived credential.

```sh
# Choose exactly one metadata repair after schema review:
/usr/local/bin/migration-guard -path /app/migrations -database "$MIGRATION_DATABASE_URL" force 35
# OR
/usr/local/bin/migration-guard -path /app/migrations -database "$MIGRATION_DATABASE_URL" force 36

# Run only the matching direction when the reviewed branch calls for a retry:
/usr/local/bin/migration-guard -path /app/migrations -database "$MIGRATION_DATABASE_URL" up 1
# OR
/usr/local/bin/migration-guard -path /app/migrations -database "$MIGRATION_DATABASE_URL" down 1
```

`force` edits migration metadata only. It does not create, remove, or validate an index. After UP, confirm clean version `36` and a valid index with the expected definition. After DOWN, confirm clean version `35` and absence of the index. If the version, index identity, and schema disagree, stop the release and get a database review. Do not auto-clear `dirty`, use `IF NOT EXISTS`, or automatically retry the DDL.
