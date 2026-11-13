# Observability Contract: Incremental Service Authorization

**Scope**: Feature-owned Go log calls and credential-handling paths (SR-005; Constitution Principles I and XV). Follow `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. This contract neither approves an API/ADR nor changes authentication outcomes.

## Feature-owned security records

Use injected loggers and request-context methods. Messages are constant lowercase phrases, not event identifiers. Each security record has an `event` attribute. The following maps the feature's pre-merge labels to the binding `<bounded_context>.<subject>.<past_tense_verb>` grammar; unrelated existing records are unchanged.

| Pre-merge label | Classification / `event` | Level / when | Permitted attributes |
|---|---|---|---|
| `incremental_authorization_required` | Security audit / `oauth2.authorization.incremental_required` | Info: expected business outcome requiring focused consent | `agent_id`, policy-rendered `principal`, `service_id`, `reason` |
| `requested_service_submission_rejected` | Security audit / `consent.requested_service.submission_rejected` | Warn: reject unsatisfied or identity-mismatched submission before persistence | `agent_id`, policy-rendered `principal`, `service_ids`, `reason` |
| `refresh_service_recheck_failed` | Security audit / `oauth2_server.service.recheck_failed` | Warn for definitive refusal; Error for dependency failure requiring operator action | `agent_id`, policy-rendered `principal`, known `service_id`, `reason`, `error_kind` |
| `requested_service_scope_rejected` | Security audit / `oauth2.requested_service.scope_rejected` | Warn: malformed, unknown, ineligible or binding-mismatched scope refused before issuance/forwarding | `agent_id`, known `service_id`, policy-rendered `principal` when available, `reason`, `error_kind=definitive` |
| `requested_service_resolution_failed` | Security audit / `oauth2.requested_service.resolution_failed` | Error: resolver dependency failure handled as fail-closed `server_error` | `agent_id`, known `service_id`, policy-rendered `principal` when available, `reason=dependency_failed`, `error_kind=transient` |
| `requested_service_evaluation_failed` | Security audit / `oauth2.requested_service.evaluation_failed` | Error: evaluator dependency failure handled as fail-closed `server_error` | `agent_id`, known `service_id`, policy-rendered `principal`, `reason=dependency_failed`, `error_kind=transient` |

Broker principals are `id.Principal` under the reserved `principal` key; never render them manually. `error_kind` replaces the pre-merge `kind` discriminator with the canonical key and has the closed set `definitive`, `transient`. Log errors once at the handling layer and only as typed safe metadata, never a wrapped cause or provider description.

Closed reason sets:

- Focused consent: `not_selected`, `required_scopes_uncovered`, `missing`, `unusable_definitive`.
- Submission rejection: `binding_mismatch`, `unsatisfied`.
- Scope rejection: `malformed_scope`, `unknown_service`, `service_not_eligible`, `binding_mismatch`.
  Map typed/sentinel permission errors; never use `err.Error()` or echo the requested scope.
- Resolution/evaluation failure: `dependency_failed`. This safe coarse classification is sufficient;
  do not parse a wrapped cause to invent a more detailed reason or record its text.
- Local refresh refusal: `binding_unresolved`, `not_selected`, `required_scopes_uncovered`, `missing`, `unusable_definitive`, `service_lookup_failed`, `grant_lookup_failed`, `permission_set_lookup_failed`, `usability_unverifiable`.

Use typed constants for classifications. A transient result does not write a rejection marker. Recorded provider rejection, reconnect and background/sweep telemetry reuse the existing ADR 046 session events without renaming them. No new persisted audit ledger is introduced.

## Continuation redirect-error diagnostic

The feature-owned `redirectError` helper's pre-merge `failed to build error redirect URL` record is
an operational diagnostic, not an authorization-success or mutation audit. Use Error level, request
`ctx`, a constant lowercase phrase and `reason=redirect_build_failed`, `error_kind=internal`.
It may omit `event` under LOGGING §2.2. This diagnostic has its own closed error-kind set `{internal}`;
security records above use `{definitive, transient}`. Include `agent_id` only if already known.
Do not record `redirect_uri`, `state`, `original_url`, the request/JWE or a wrapped URL/error value.
The primary denial/failure is logged once at its handling layer; this diagnostic occurs only if
building that denial's redirect fails. Telemetry never changes the existing HTTP/error fallback.

## ExtProc diagnostics

The invalid optional metadata diagnostic (`challenge_metadata_invalid` in the design) uses a constant message phrase such as `challenge metadata invalid`, Warn level, and only the fixed field name `resource_metadata_url` plus existing bounded token-exchange diagnostics. It never includes the rejected value. This advisory diagnostic can omit `event` under LOGGING §2.2.

The existing challenge record adds only `challenge.scope_present` and `challenge.resource_metadata_present`, each with value set `{true, false}`. These are log attributes, not span or metric attributes. Omit invalid optional values from the challenge without changing its status, body, exchange result or authorization decision.

## Traces and metrics

No new metric instruments, span names, span-attribute families, instrumentation scopes or namespaces are introduced. Reuse production HTTP, storage, session-refresh, token-exchange and ExtProc instrumentation. Pure scope parsing/coverage adds no spans. Existing `token_exchange.*` attributes keep the value sets in `docs/reference/token-exchange.md#diagnostic-attributes`.

Any modification of a feature-owned existing span must satisfy the OpenTelemetry review checklist: constant name, correct kind, bounded status/error metadata and no credential values. Do not add principal span attributes until the policy and `id.Principal.SpanAttribute` required by #356 exist. Principals never belong in metrics. No coding-standard amendment or registry addition is required by this design.

## Required evidence

Before changing the corresponding production call, write tests through production constructors using a buffer JSON logger and an in-memory span recorder where the production path is traced. Assert:

1. Canonical `event`, IDs, closed `reason` and `error_kind` sets, classification and level for every
   security-record row, including each scope-rejection reason and injected resolver/evaluator
   dependency failure. Assert the redirect-build diagnostic's level/closed metadata and optional
   event exemption separately. No assertions on message text. A refusal produces no success audit,
   mutation, code issuance or upstream authorize redirect.
2. Principal attributes equal central-policy output. ExtProc must not import broker ID types; any existing principal log uses a reserved key and the shared policy.
3. Distinct synthetic access/refresh/subject tokens, client assertion/secret, code, PKCE verifier, state and sealed JWE are absent from logs and from span attributes/events/status. Inject a dependency error containing synthetic secrets to prove wrapped causes are not exported.
4. Invalid metadata containing quotes, backslashes, CR/LF or oversized text is absent from diagnostics; challenge presence attributes remain booleans.
5. Recorded refresh rejection prevents new issuance even with valid access; transient failure leaves state unchanged, and reconnect clears the rejection under existing locking.

Evidence belongs in the relevant `_test.go` files and the implementation run's result, not in new test-only production APIs. Container-backed evidence remains separate from locally runnable safety tests.
