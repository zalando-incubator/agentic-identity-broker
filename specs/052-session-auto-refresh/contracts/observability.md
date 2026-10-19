# Contract: Observability

**Feature**: `052-session-auto-refresh` | FR-017 to FR-024, SR-007 | Research R10 to R13

These names become a compatibility surface once released. All instruments use the global MeterProvider (ADR 011) on meter `oauth2session`. Nothing is exposed on `/metrics` or `/health`.

## New instruments

| Instrument | Kind | Unit | Attributes | Recorded when |
|---|---|---|---|---|
| `oauth2_sessions` | `Int64ObservableGauge` | `{session}` | `service_id`, `state` | Each export cycle with a successful health query: one point per service with sessions and per state, zero-filled |
| `session_sweep_duration_seconds` | `Float64Histogram`, buckets `1, 5, 10, 30, 60, 120, 300, 600, 1200, 1800, 3600` | `s` | `outcome` (`completed`, `rejected`, `aborted`, `canceled`), `dry_run` (`true`, `false`) | Each sweep request with an operator principal, at the same time as the `session sweep` audit record |
| `session_sweep_skipped_total` | `Int64Counter` | `{refresh}` | `triggered_by="sweep"` | Each skipped candidate in a real sweep |

The existing instruments `session_sweep_refreshed_total`, `session_sweep_failed_total`, and `proactive_refresh_*_total` are unchanged.

Cardinality: `oauth2_sessions` has 4 × services series per replica. The histogram has 8 attribute combinations. No instrument carries a principal, session ID, or token (FR-022).

## New log record

| Message | Level | Attributes | Emitted when |
|---|---|---|---|
| `session health query failed` | WARN | `error_kind` (storage error kind, e.g. `timeout`, `unknown`) | The health query fails, times out, or returns an invalid group. At most once per export cycle per replica. |

The message contains no SQL text, principal, or session ID.

## Aggregation guidance (operations guide)

| Signal | Aggregate across replicas with | Why |
|---|---|---|
| `oauth2_sessions` | `max` | Every replica reports the same totals |
| `session_sweep_*` | `sum` | Only the replica that served the request records it |

Backends that translate OTLP names (for example to Prometheus naming) may add or change suffixes. The guide tells operators to look up the translated name in their backend.

## Example queries (operations guide, PromQL as an example language)

```promql
max by (service_id, state) (oauth2_sessions)
max by (service_id) (oauth2_sessions{state="expired_unrefreshable"})
  - max by (service_id) (oauth2_sessions{state="expired_unrefreshable"} offset 1d) > 0
sum(increase(session_sweep_duration_seconds_count{outcome="completed",dry_run="false"}[3h])) == 0
absent_over_time(oauth2_sessions[15m])
sum(rate(session_sweep_failed_total[1d])) > 0
```

For clusters that run kube-state-metrics (optional):

```promql
time() - kube_cronjob_status_last_successful_time{cronjob=~".*-session-sweep"} > 3 * 3600
kube_job_status_failed{job_name=~".*-session-sweep-.*"} > 0
```
