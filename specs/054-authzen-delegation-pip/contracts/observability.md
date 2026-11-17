# Observability Contract: AuthZEN delegation and permit-bound token reuse

**Scope**: Feature 054 broker evaluation and standalone ExtProc delegation paths.
**Binding sources**: `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`, ADRs 027/033/048, SR-003/SR-004.
**Status**: Baseline inventory is not compliance. Section 3 is the exact Proposed F2 migration (D-7); T103 remains required implementation work, including namespace registration through a permitted publication path. ADR-2's log-only mismatch diagnostic remains Proposed. Neither registration nor new approval is recorded here.

## 1. Existing signal inventory

| Signal | Scope / kind or level | Bounded metadata and handling |
|---|---|---|
| `authzen.access_evaluation` span | `tokenexchange`, Internal (current implementation's default) | `token_exchange.operation=access_evaluation`; diagnostic outcome, recovery action/target, stage/detail and typed service/agent/grant IDs from the existing diagnostic reference. `access_evaluation.decision=permit|deny|error`; permit `access_evaluation.max_age` integer 0–60; error HTTP status 400/401/403/500. |
| `authzen.access_evaluation.permitted`, `.denied`, `.failed` log events | Existing evaluation audit/security records | Verified identities come from the shared security context/principal policy, never from raw claims. Outcomes and error metadata are bounded; no raw dependency cause or credential. These identifiers are the existing baseline, not the F2 target. |
| `extproc.delegation.resolve` span | `extproc`, Internal (current implementation's default) | `delegation.cache=hit|miss` and the resolution family below; original permit deadline is never extended by observability. |
| `extproc.delegation.evaluate` span | `extproc`, Client | Resolution family below; no body, headers, full URL or requested resource URI. |
| `extproc.delegation_context.denied` log event | Info, expected delegation denial | `operation=delegation_resolution`; allowlisted `delegation.code`; cache hit/miss and diagnostic stage/detail. |
| `extproc.delegation_context.unavailable` log event | Warn, degraded dependency | Same operation/cache metadata; `delegation.error=timeout|unavailable|rejected|invalid_response`; no wrapped cause. |
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

## 3. Exact registry and identifier migration (Proposed D-7; required T103)

The baseline violates OPENTELEMETRY §§3.2, 3.3 and 4.1; historical green runs do not close this obligation. The 2026-11-06 F2 selection provenance in quickstart §13 is retained, but this concrete mapping is Proposed for current sign-off.

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

**Permitted publication prerequisite**: the maintainer must enable the implementation worker's allowed-file configuration for `coding-standards/OPENTELEMETRY.md` (and the existing code/test/reference paths), or publish the exact namespace rows in an earlier maintainer change and supply that updated base/head to the implementation run. T103 may publish only when registration and its code changes have a permitted path. This feature-document worker does not edit the standard, CI/workflows or allowlist, and does not claim registration already happened. T103 remains in-scope required compliance; absence of this permission defers only that task, never waives the violation or allows overall implementation completion.

## 4. Metrics

No new metric instruments are designed for this feature or ADR-2. Therefore there is no new instrument name/kind/unit/scope/attribute-set registration. Existing exchange instruments retain their contracts. Adding a new instrument would require an explicit contract entry and applicable standards before use.

## 5. Safety and contract tests

- `internal/adapters/http/enduser/authzen_evaluation_test.go`: pin event IDs and span kind/attributes; reject subject/assertion markers and unfiltered claims in bodies, audit output, span attributes/events/status. Section 3's exact target assertions are introduced test-first with T103, not claimed present here.
- `internal/extproc/delegation/client_test.go` and `resolver_test.go`: pin resolution kinds, bounded outcomes/details, cache values and safe statuses; reject subject/assertion/permit/resource markers in exported signals.
- `internal/extproc/server/delegation_test.go`: retain F6 event assertions, no forwarding on denied/unavailable/expired permits, and credential-safe logging through production constructors.
- `internal/extproc/server/exchanger_binding_test.go` plus `delegation_test.go`: first fail on the missing mismatch diagnostic, then assert its closed log-only value, terminal-conflict response and same-token/different-generation replacement preservation and absence of all excluded data on mismatch and re-exchange failure. No principal span attribute is added while the central span-principal policy is unavailable.
- Use real buffer loggers and span recorders through existing test patterns; no test-only production APIs, checks disabled, or retrospective claims of red evidence. Run `just check`, `just extproc-test` with race detection and the matching broker/integration gates before recording completion.
