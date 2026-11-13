# Observability Contract: AuthZEN delegation and permit-bound token reuse

**Scope**: Feature 054 broker evaluation and standalone ExtProc delegation paths.
**Binding sources**: `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`, ADRs 027/033/048, SR-003/SR-004.
**Status**: D-7 was approved by mjungsbluth on 2026-11-06 at https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6688524728. Run 42297225880/1 implements section 3, registers both namespaces in `coding-standards/OPENTELEMETRY.md` and publishes the breaking-telemetry changelog/query migration in `docs/reference/token-exchange.md`. Section 1 preserves the pre-migration inventory, not current names. ADR-2's approved log-only mismatch diagnostic is still unimplemented.

## 1. Historical pre-migration signal inventory

| Signal | Scope / kind or level | Bounded metadata and handling |
|---|---|---|
| `authzen.access_evaluation` span | `tokenexchange`, Internal (current implementation's default) | `token_exchange.operation=access_evaluation`; diagnostic outcome, recovery action/target, stage/detail and typed service/agent/grant IDs from the existing diagnostic reference. `access_evaluation.decision=permit|deny|error`; permit `access_evaluation.max_age` integer 0–60; error HTTP status 400/401/403/500. |
| `authzen.access_evaluation.permitted`, `.denied`, `.failed` log events | Existing evaluation audit/security records | Verified identities come from the shared security context/principal policy, never from raw claims. Outcomes and error metadata are bounded; no raw dependency cause or credential. These identifiers are the existing baseline, not the F2 target. |
| `extproc.delegation.resolve` span | `extproc`, Internal (current implementation's default) | `delegation.cache=hit|miss` and the resolution family below; original permit deadline is never extended by observability. |
| `extproc.delegation.evaluate` span | `extproc`, Client | Resolution family below; no body, headers, full URL or requested resource URI. |
| `extproc.delegation_context.denied` log event | Info, expected delegation denial | `operation=delegation_resolution`; allowlisted `delegation.code`; cache hit/miss and diagnostic stage/detail. |
| `extproc.delegation_context.unavailable` log event | Warn, fail-closed delegation failure | `error_kind=timeout|unavailable|rejected|invalid_response`; resolution emitters also carry operation/cache metadata. Emitted for resolution failure, terminal exchange-result binding conflict, or invalid grant timestamp (see §2.1); no wrapped cause. |
| `extproc.delegation_context.permit_expired` log event | Warn | `operation=delegation_resolution`; bounded unavailable diagnostic; no permit/context contents. |
| Permit cache hit/miss/success diagnostics | Debug | Closed hit/miss values and success diagnostics only. Do not add principals to Debug records. |

ExtProc resolution span attributes use these exact current outcome/detail pairs:

| `token_exchange.outcome` | `token_exchange.failure_detail` |
|---|---|
| `success` | `none` |
| `authorization_denied` | `delegation_denied` |
| `infrastructure_error` | `delegation_unavailable` |
| `canceled` | `caller_canceled` |

Infrastructure errors set a constant status description and `error.type` from `timeout|unavailable|rejected|invalid_response`. Cancellation is an outcome, not an error status; delegation denials do not set an error status. The broker's diagnostic families retain the closed value sets in `docs/reference/token-exchange.md#diagnostic-attributes`; do not invent values or export arbitrary errors.

## 2. Proposed ADR-2 diagnostic

| Aspect | Contract |
|---|---|
| Trigger | The request detects that a cached token's scope-set/grant-instant binding differs from its permit and takes the re-exchange path. |
| Output | One request-context Debug diagnostic with `delegation_binding=mismatch`. |
| Attribute family | Log-only `delegation_binding`; closed value set `{mismatch}`. |
| Plumbing | Existing constructor-injected logger, `loggerFromContext` and bounded diagnostic helpers. Constant lowercase message; no logging at a returning error layer. |
| Event identifier | This internal diagnostic is not an audit, security-denial or lifecycle record; LOGGING §2.2 permits omission of `event`. Existing exchange-error/security events retain their current handling. |
| Excluded data | Subject token, assertion, provider access/refresh token, Authorization, raw scope set, principal, grant contents, requested resource URI, permit, request/response structs and dependency cause. |
| Spans and metrics | No new span, span attribute namespace or metric instrument. Existing exchange instrumentation continues to observe a real broker re-exchange. No binding data is added to it. |

This diagnostic does not rename any existing identifier and needs no new tracing namespace. It does not perform or replace F2's migration.

### 2.1. Delegation-unavailable emitters (approved F5/P5)

The event is `extproc.delegation_context.unavailable` with Warn level on both body-bearing
and header-only paths. Its canonical classification key is `error_kind`, not `delegation.error`.

| Emitter | `error_kind` | Additional bounded metadata / response |
|---|---|---|
| `resolveDelegation`: permit resolution failure | `timeout`, `unavailable`, `rejected`, or `invalid_response` (untyped errors use `unavailable`) | `operation=delegation_resolution`, `delegation.cache=hit|miss`; 503 `delegation context unavailable`. |
| `exchangeErrorResponse`: terminal `ErrExchangeResultBindingConflict` | `unavailable` | 503 `delegation token binding unavailable`; no grant, scopes or exchange-result contents. |
| `exchangeErrorResponse`: local `errInvalidGrantTimestamp` from `exchangeWithDelegation` | `invalid_response` | 503 `delegation context unavailable`; never export the malformed timestamp or parse cause. Arbitrary permit-client errors are not timestamp failures. |

All three emitters use the bounded `delegation_unavailable` diagnostic and mark the request
span as Error. Resolution failures use span `error.type=unavailable`, while the log carries
the finer classification; binding conflicts use `unavailable` and timestamp failures use
`invalid_response` on both signals. Permit expiry retains the separate
`extproc.delegation_context.permit_expired` event. No new signal or identifier is introduced.
Reason: [approved F5/P5](https://github.com/zalando-incubator/agentic-identity-broker/pull/292#issuecomment-6729032045).

## 3. Exact registry and identifier migration (approved D-7; T103)

The pre-migration baseline violated OPENTELEMETRY §§3.2, 3.3 and 4.1. The supplied five-item approval covers this concrete mapping. The old identifiers below are retained for query migration and historical review; production now emits only the targets. No audit record or span is dual-emitted. Query both old and new names during rolling deployment and the desired historical retention window, then retire the old query branches; attribute filters remain unchanged.

| Signal | Old identifier / kind | Required target / explicit kind |
|---|---|---|
| Broker span (scope `tokenexchange`) | `authzen.access_evaluation`, implicit Internal | `tokenexchange.access_evaluation`, `trace.WithSpanKind(trace.SpanKindInternal)` |
| Permit log event | `authzen.access_evaluation.permitted` | `tokenexchange.access_evaluation.permitted` |
| Denial log event | `authzen.access_evaluation.denied` | `tokenexchange.access_evaluation.denied` |
| Failure log event | `authzen.access_evaluation.failed` | `tokenexchange.access_evaluation.failed` |
| Resolver span (scope `extproc`) | `extproc.delegation.resolve`, implicit Internal | Same name, explicit `trace.WithSpanKind(trace.SpanKindInternal)` |
| Broker-call span (scope `extproc`) | `extproc.delegation.evaluate`, Client | Same name, explicit `trace.WithSpanKind(trace.SpanKindClient)` |
| Attributes | `access_evaluation.decision`, `access_evaluation.max_age`, `delegation.cache` | Names unchanged; registration below MUST precede publication/use |
| ExtProc security events (F6) | `extproc.delegation_context.denied`, `.unavailable`, `.permit_expired` | Unchanged; no additional rename |

**Required registration**: add these rows to OPENTELEMETRY §4.1 before publishing the identifiers/code:

| Namespace | Describes | Reference |
|---|---|---|
| `access_evaluation` | Credential-free broker AuthZEN decision and freshness | `specs/054-authzen-delegation-pip/contracts/observability.md` |
| `delegation` | ExtProc permit-cache lookup | `specs/054-authzen-delegation-pip/contracts/observability.md` |

Attribute values remain `access_evaluation.decision=permit|deny|error`, permit-only integer `access_evaluation.max_age` 0–60 and `delegation.cache=hit|miss`. Existing `token_exchange.*` diagnostic families and semantic HTTP/error keys are already registered and unchanged. Do not add principal attributes pending #356, metrics, request values or synonym namespaces.

**Migration behavior**: replace broker span/event identifiers atomically; do not dual-emit audit records or duplicate spans. Preserve bounded outcomes/levels and constructor plumbing. Update queries, pinned tests and diagnostic docs to the target names. Record old→new identifiers, unchanged attribute names and rollout guidance in a dedicated breaking-telemetry changelog section of `docs/reference/token-exchange.md` and its feature-local counterpart here. No metric rename exists, so the one-release metric dual-emission rule does not apply. Main alternative: retain old identifiers under a standard exception; rejected because the existing scope/name rule can be followed without weakening it.

**Publication prerequisite satisfied in run 42297225880/1**: the implementation workflow now allows `coding-standards/**`, and this change adds both exact namespace rows together with the code/test migration. No workflow, permission or allowlist is changed by the implementation worker. The former publication blocker is resolved; telemetry compliance does not establish completion of the independent token-binding work.

## 4. Metrics

No new metric instruments are designed for this feature or ADR-2. Therefore there is no new instrument name/kind/unit/scope/attribute-set registration. Existing exchange instruments retain their contracts. Adding a new instrument would require an explicit contract entry and applicable standards before use.

## 5. Safety and contract tests

- `internal/adapters/http/enduser/authzen_evaluation_test.go`: pin event IDs and span kind/attributes; reject subject/assertion markers and unfiltered claims in bodies, audit output, span attributes/events/status. Section 3's exact target assertions are introduced test-first with T103, not claimed present here.
- `internal/extproc/delegation/client_test.go` and `resolver_test.go`: pin resolution kinds, bounded outcomes/details, cache values and safe statuses; reject subject/assertion/permit/resource markers in exported signals.
- `internal/extproc/server/delegation_test.go`: retain F6 event assertions, no forwarding on denied/unavailable/expired permits, and credential-safe logging through production constructors.
- `internal/extproc/server/exchanger_binding_test.go` plus `delegation_test.go`: first fail on the missing mismatch diagnostic, then assert its closed log-only value, terminal-conflict response and same-token/different-generation replacement preservation and absence of all excluded data on mismatch and re-exchange failure. No principal span attribute is added while the central span-principal policy is unavailable.
- Use real buffer loggers and span recorders through existing test patterns; no test-only production APIs, checks disabled, or retrospective claims of red evidence. Run `just check`, `just extproc-test` with race detection and the matching broker/integration gates before recording completion.
