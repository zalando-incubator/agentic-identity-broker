# ExtProc Approval Observability Contract

These instruments implement the approval telemetry clarification in
[Feature 026](../spec.md#session-2026-09-09) and follow the
[OpenTelemetry standard](../../../coding-standards/OPENTELEMETRY.md).
They use the global meter with instrumentation scope `extproc`.

## Instruments

| Name | Kind | UCUM unit | Description | Attributes | Observation |
|---|---|---|---|---|---|
| `extproc.approval.gate.decisions` | Int64 Counter | `{decision}` | Number of approval gate decisions by outcome | `outcome`: `proceed`, `elicit`, `deny` | Increments once per gate evaluation, including validation failures and unavailable dependencies. |
| `extproc.approval.sync_age` | Int64 Observable Gauge | `s` | Seconds since the last successful approval sync | None | Observed once per collection cycle, in whole seconds. Before the first successful sync, measures time since cache creation. |

The decision counter reports `proceed` when the gate permits forwarding,
`elicit` when it returns an approval URL, and `deny` otherwise. It records
with `context.WithoutCancel` so caller cancellation does not discard the decision.
It counts gate evaluations, not every OPA evaluation or token exchange.

A successful background snapshot replacement or `304 Not Modified` resets the
sync age. A targeted principal read does not reset the global synchronization age.
The gauge is an operational freshness signal; authorization uses the separate
per-pair freshness checks and configured `tool_approvals.max_staleness`.

Neither instrument carries principals, agent IDs, session IDs, approval IDs,
tool names, arguments, URLs, or credentials. The counter has at most three
attribute sets per resource and the gauge has one. No operator-bounded identity
attribute is needed for these instruments.

## Compatibility and Verification

Instrument names, kinds, outcome values, and recording behavior remain unchanged.
The decision counter now declares its previously missing description and unit.
Broker approval instruments and token-exchange instruments are outside this contract.

`TestGateEvaluateEmitsDecisionTelemetry` and
`TestSyncerPublishesApprovalSyncAgeGauge` in `internal/extproc/approval/` use an
`sdkmetric.ManualReader` to enforce names, scope, kinds, units, descriptions,
values, and the complete attribute sets.
