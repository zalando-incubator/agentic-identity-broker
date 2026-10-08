# Contract: Observability (logs, metrics, traces)

**Feature**: `029-proactive-token-refresh` | NFR-001..NFR-004 | Research R7, R11

Operators build alerts on these names. Once released, they are a compatibility surface. A rename
needs a changelog entry.

## Metrics (OpenTelemetry, NFR-004)

The counters are emitted through the existing global `MeterProvider` (ADR 011; `internal/adapters/telemetry/provider.go:124-135`).
There is no Prometheus registry, no `/metrics` endpoint, and no `/health` counter surface. When
`telemetry.metrics.enabled=false`, the global no-op provider makes every `Add` free.

| Instrument | Kind | Meter | Attributes | Incremented when |
|---|---|---|---|---|
| `proactive_refresh_triggered_total` | `Int64Counter` | `oauth2session` | `triggered_by="background"` | A background refresh is accepted into the pool (slot acquired). |
| `proactive_refresh_dropped_total` | `Int64Counter` | `oauth2session` | `triggered_by="background"` | A background refresh is rejected because every slot is busy. |
| `proactive_refresh_failed_total` | `Int64Counter` | `oauth2session` | `triggered_by="background"` | An accepted background refresh ends in an error, excluding "session deleted" (a no-op) and "not due" (already refreshed). |
| `session_sweep_refreshed_total` | `Int64Counter` | `oauth2session` | `triggered_by="sweep"` | A non-dry-run sweep commits a refreshed session. |
| `session_sweep_failed_total` | `Int64Counter` | `oauth2session` | `triggered_by="sweep"` | A non-dry-run sweep classifies a candidate as `failed`. |

- Unit `"{refresh}"`. Each instrument has a description string.
- Exactly one attribute per instrument, so cardinality is constant. `service_id`, `session_id`, and
  principal are **never** metric attributes.
- Not counted: deduplicated submissions (key already in flight), submissions after `Close`, and any
  dry-run outcome.
- Instruments are created once per constructor. They are not created per request.

**Useful derived signals**:
- `dropped / (triggered + dropped)`: pool saturation. Raise `background_workers` within its cap, or
  investigate upstream latency.
- `failed / triggered`: provider health for proactive refresh.
- `rate(session_sweep_failed_total)` that persists across sweeps: sessions that need
  re-authentication (dead refresh tokens).

## Structured log events

The principal is never logged by the session service. Token material is never logged anywhere.

| `event` / message | Level | Emitted by | Attributes | Requirement |
|---|---|---|---|---|
| `session.oauth2.token_refreshed` | INFO | `OAuth2SessionService` after the locked `UPDATE` commits | `session_id`, `service_id`, `triggered_by`, `public_client`, `reason=token_refresh_succeeded`, `timestamp` | NFR-001, `SessionTokensRefreshed` |
| `session.oauth2.proactive_refresh_dropped` | WARN | `backgroundRefresher.submit` | `session_id`, `service_id`, `triggered_by=background`, `workers` | NFR-002 |
| `session.oauth2.refresh_failed` | WARN or ERROR per research R11 | trigger-aware caller of `refreshDueSession` | `session_id`, `service_id`, `triggered_by`, `public_client`, `oauth2_session` (bounded `ErrorMetadata`: operation, detail, kind, dependency, provider status) | NFR-003 |
| `session.oauth2.sweep_aborted` | ERROR | `SessionSweepService` | `triggered_by=sweep`, `oauth2_session` metadata, partial `refreshed`/`skipped`/`failed`/`total_evaluated` | NFR-003 (infrastructure) |
| `session sweep` (audit) | INFO | `SessionSweepHandler` | `operator_principal`, `dry_run`, `lookahead`, `page_size`, `refreshed`, `skipped`, `failed`, `total_evaluated`, `duration_ms`, `outcome` ∈ {`completed`, `rejected`, `aborted`, `canceled`} | Principle I (auditable security operations) |

`operator_principal` is the **administrator** identity from the admin proxy. It follows the CIMD key
lifecycle audit precedent (`cimd_client_keys_handler.go:161-163`), and it is not an end-user
principal.

The on-demand path keeps its current failure log level (ERROR) and gains `session_id` and
`triggered_by=on-demand`.

## Traces

| Span | Kind | Parent / link | Attributes |
|---|---|---|---|
| `oauth2session.background_refresh` | internal | **new root**, with a link to the triggering request span | `oauth2.refresh.triggered_by=background`, `oauth2.refresh.outcome` ∈ {`refreshed`, `not_due`, `failed`} |
| (existing) upstream token POST | client | child of the refresh span through the instrumented upstream transport (`builder.go:607-610`) | unchanged; no URL, header, or body recording |

The sweep needs no new span. The request's server span (otelchi) covers it, and per-session upstream
calls appear as its children.
