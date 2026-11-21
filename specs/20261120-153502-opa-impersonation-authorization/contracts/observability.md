# Observability contract: OPA impersonation authorization

**Feature**: `specs/20261120-153502-opa-impersonation-authorization` | **Sign-off**: plan.md Approval
Item API-DIAG | **Research**: R9, R12 | **Standards**: `coding-standards/LOGGING.md`,
`coding-standards/OPENTELEMETRY.md`

No signal contains a claim value, raw token, decision member other than the closed `action` set,
bootstrap value, bundle-service URL or credential, or principal. Rule names, bundle names, and bundle
manifest revisions are operator-defined configuration identifiers.

## Diagnostics (token-exchange diagnostic attributes)

Two new `token_exchange.failure_detail` values, both with `token_exchange.failure_stage =
client_authorization` and exchange kind `impersonation`. They are added to the closed value set in
`docs/reference/token-exchange.md#diagnostic-attributes` and to `internal/domain/tokenexchange/diagnostic.go`.

| Detail | When | External result |
| --- | --- | --- |
| `opa_evaluation_failed` | OPA evaluation error, strict built-in error, timeout, or cancellation | `500 server_error` |
| `opa_decision_invalid` | Undefined, multiple, non-object, or unsupported-action decision | `500 server_error` |

Caller cancellation keeps the existing `caller_canceled` override in `originError`.

## Span

| Name | Scope | Kind | When | Attributes | Status |
| --- | --- | --- | --- | --- | --- |
| `tokenexchange.impersonation.opa.evaluate` | `tokenexchange` | Internal | Each OPA decision evaluation | `impersonation.rule` (string), `impersonation.authorization.action` (`allow`, `deny`, `invalid`, `error`), `impersonation.policy_revision` (string) | `Error` with a constant description for `invalid` and `error`; unset otherwise |

CEL evaluation keeps no span, as today.

## Metrics

| Name | Kind | Unit | Attributes | Changes when |
| --- | --- | --- | --- | --- |
| `tokenexchange.impersonation.opa.revisions` | Counter | `{revision}` | `rule`; `outcome` (`admitted`, `rejected`); `reason` (`none`, `compile_failed`, `prohibited_builtin`, `decision_undefined`, `binding_missing`, `evidence_voided`, `snapshot_failed`) | Each candidate revision after an OPA activation |
| `tokenexchange.impersonation.opa.revision.age` | Observable gauge (Float64) | `s` | `rule` | Read per export: seconds since the active revision was admitted; no point before the first admission |

`rule` is bounded by the operator's rule list, like `service_id` in the `oauth2_sessions` precedent.
Instruments are created once in the adapter constructor; constructor errors are logged once at
`Warn`.

## Log records

| Message | `event` | Level | Attributes |
| --- | --- | --- | --- |
| `impersonation policy revision admitted` | `oauth2_server.impersonation_policy.revision_admitted` | Info | `rule`, `policy_revision`, `bundle_count`, `duration_ms` |
| `impersonation policy revision rejected` | `oauth2_server.impersonation_policy.revision_rejected` | Warn | `rule`, `policy_revision`, `reason` (closed set above), `error_kind`; for `binding_missing` also `missing_reference` (`subject_token`, `client_assertion`, `actor_token`); for `prohibited_builtin` also `builtin` (built-in name) |
| `impersonation policy not ready` | `oauth2_server.impersonation_policy.startup_failed` | Error | `rule`, `reason` (`timeout`, `bootstrap_invalid`), `duration_ms` |

`policy_revision` is the sorted `<bundle>=<manifest revision>` list joined with `,`; a bundle without a
revision contributes `<bundle>=`. Raw OPA errors are not logged when they can contain bootstrap
values; compile errors log only the module path and the error class.

## Audit event `impersonation_decision`

Adds two attributes, set on every record after rule selection:

| Attribute | Values |
| --- | --- |
| `authorization_engine` | `cel`, `opa`, or absent when no rule reached authorization |
| `policy_revision` | OPA only: the evaluated revision's `policy_revision` string |

Existing attributes keep their meaning.

## Tests

- Audit and log tests assert that no claim value, token string, decision `reasons`, or bootstrap
  credential appears in captured records for allow, deny, invalid, error, rejection, and startup
  failure.
- Span tests use the in-memory exporter and assert the constant span name, attributes, and status.
- Metric tests assert the counter outcomes and reasons and the gauge after admission.
