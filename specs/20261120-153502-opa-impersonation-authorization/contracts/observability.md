# Observability contract: OPA impersonation authorization

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Sign-off**: plan.md Approval
Item API-DIAG | **Research**: R9, R12 | **Standards**: `coding-standards/LOGGING.md`,
`coding-standards/OPENTELEMETRY.md`

No signal contains a claim value, raw token, decision member other than the closed `action` set,
bootstrap value, bundle-service URL or credential, or new principal attribute. Existing audited
principals continue through the central principal policy, not through copied claim maps. Rule names, bundle names, and bundle
manifest revisions are operator-defined configuration identifiers.

## Diagnostics (token-exchange diagnostic attributes)

Two new `token_exchange.failure_detail` values, both with `token_exchange.failure_stage =
client_authorization` and exchange kind `impersonation`. They are added to the closed value set in
`docs/reference/token-exchange.md#diagnostic-attributes` and to `internal/domain/tokenexchange/diagnostic.go`.

| Detail | When | External result |
| --- | --- | --- |
| `opa_evaluation_failed` | OPA evaluation error, strict built-in error, timeout, or cancellation | `500 server_error` |
| `opa_decision_invalid` | Undefined, multiple, non-object, or unsupported-action decision | `500 server_error` |

Caller cancellation keeps the existing `caller_canceled` override in `originError`. A disconnected
client is not promised an HTTP response. Scenario 3.2 starts a request with a synthetic W3C trace
context, waits for evaluation to begin through a test-local span processor, then cancels the client.
It asserts the canceled client outcome, an ended OPA evaluation span with that trace ID and
`canceled` action, no issuance audit, and no authorization by the permissive later rule (its test
fixture would produce a distinct rule-selection audit). Wait for the correlated request span to
end before asserting absence. Domain tests separately assert internal `server_error` mapping and
no fallthrough. No new production test API, manually logged trace ID, or credential signal is added.

## Span

| Name | Scope | Kind | When | Attributes | Status |
| --- | --- | --- | --- | --- | --- |
| `tokenexchange.impersonation.opa.evaluate` | `tokenexchange` | Internal | Each OPA decision evaluation | `impersonation.rule` (configured rule-name set), `impersonation.authorization.action` (`allow`, `deny`, `invalid`, `error`, `canceled`), `error.type` (`opa_decision_invalid`, `opa_evaluation_failed`, on failures only) | `Error` with a constant description for `invalid` and `error`; unset for `allow`, `deny`, and caller cancellation (`canceled`) |

CEL evaluation keeps no span, as today. The rule attribute is bounded by the configured rule-name
set. Free-form bundle manifest revisions remain log/audit attributes only, not span or metric
attributes (`OPENTELEMETRY.md` § 4.2 and § 6). Caller cancellation uses the `canceled` outcome without
`error.type` or error status; deadline expiration remains an evaluation failure (§ 3.4).

## Metrics

| Name | Kind | Unit | Attributes | Changes when |
| --- | --- | --- | --- | --- |
| `tokenexchange.impersonation.opa.revisions` | Counter | `{revision}` | `outcome` (`admitted`, `rejected`); `reason` (`none`, `compile_failed`, `prohibited_builtin`, `decision_undefined`, `binding_missing`, `evidence_voided`, `snapshot_failed`, `bundles_incomplete`) | Each candidate revision of the shared engine (each activation in managed mode, once in local mode) |
| `tokenexchange.impersonation.opa.revision.age` | Observable gauge (Float64) | `s` | none | Read per export: seconds since the shared active revision was admitted; no point before the first admission |

There is one engine and one active revision, so neither instrument carries `rule`. The rejecting
rule is a log attribute only.
Both instruments use scope `tokenexchange` and are created once in the adapter constructor; constructor errors are logged once at
`Warn`.

## Log records

| Message | `event` | Level | Attributes |
| --- | --- | --- | --- |
| `impersonation policy revision admitted` | `oauth2_server.impersonation_policy.revision_admitted` | Info | `policy_revision`, `policy_admission_id`, `bundle_count`, `rule_count`, `duration_ms` |
| `impersonation policy revision rejected` | `oauth2_server.impersonation_policy.revision_rejected` | Warn | `rule` (first failing rule, for `decision_undefined`, `binding_missing`, and `evidence_voided` only), `policy_revision`, `reason` (closed set above), `error_kind`; for `binding_missing` also `missing_reference` (`subject_token`, `client_assertion`, `actor_token`); for `prohibited_builtin` also `builtin` (built-in name) |
| `impersonation policy not ready` | `oauth2_server.impersonation_policy.startup_failed` | Error | `reason` (`timeout`, `bootstrap_invalid`, `local_bundle_invalid`), `rule` (last rejecting rule, when known), `duration_ms` |
| `impersonation policy loaded from local bundle` | `oauth2_server.impersonation_policy.local_mode` | Info | `policy_revision`, `file_count`; once at startup in local mode. Message states that policy and data change only on restart (spec clarification 2026-11-24) |
| `impersonation policy bundle token unavailable` | `oauth2_server.impersonation_policy.token_unavailable` | Warn | `reason` (`unreadable`, `file_too_large`, `empty`), at most once per minute while the condition lasts; never the path's contents |

`policy_revision` is the sorted `<bundle>=<manifest revision>` list joined with `,`; a bundle without a
revision contributes `<bundle>=`. Raw OPA errors are not logged when they can contain bootstrap
values; compile errors log only the module path and the error class.

`policy_admission_id` is a broker-generated process-instance UUID joined with the strictly
increasing successful-admission sequence of the shared engine. It is allocated only at admission, stored on
that immutable revision, and copied unchanged to its admission log and deciding audits. It is
unique within the process instance even when bundle manifest labels are
empty or reused; process-instance UUID prevents confusion across restarts. Rejected candidates
have no admission ID. It is log/audit-only, never a metric or span attribute. The main alternative
is requiring fresh nonempty manifest labels and rejecting unchanged labels (would reject valid
updates from existing bundle publishers).

## Audit event `impersonation_decision`

Adds three attributes, set on every record after rule selection:

| Attribute | Values |
| --- | --- |
| `authorization_engine` | `cel`, `opa`, or absent when no rule reached authorization |
| `policy_revision` | OPA only: the evaluated revision's operator manifest-label string |
| `policy_admission_id` | OPA only: the evaluated immutable revision's broker admission identifier |

Existing attributes keep their meaning.

## Tests

- Audit and log tests assert that no claim value, token string, decision `reasons`, or bootstrap
  credential appears in captured records for allow, deny, invalid, error, rejection, and startup
  failure.
- Span tests use the in-memory exporter and assert the constant span name, attributes, and status.
- Metric tests assert the counter outcomes and reasons and the gauge after admission; no revision
  label or admission ID appears in a metric or span.
- Admit two different snapshots with equal or empty manifest labels; their admission IDs differ.
  Concurrent swap tests pair each audit with exactly the snapshot that made its decision.
- Formatting and JSON/text slog tests of the sensitive input type assert constant redaction for
  value/pointer `%s`, `%v`, `%+v`, `%#v`, and `slog.Any`; evaluation still receives full typed claims.
