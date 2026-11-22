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
| `authorization_continuation_rejected` | Security audit / `oauth2.authorization.continuation_rejected` | Warn: empty, repeated, forged, malformed, expired or request/principal/agent-mismatched authorization session refused before issuance/forwarding | `agent_id`, policy-rendered `principal`, `reason=invalid_authorization_session`, `error_kind=definitive` |
| `consent_authorization_session_rejected` | Security audit / `consent.authorization_session.rejected` | Warn: empty, repeated, invalid, expired, agent-mismatched or principal-mismatched session token rejected by consent agent-detail or grant submission | `agent_id`, policy-rendered `principal`, `reason` |
| `requested_service_unverifiable` | Security audit / `oauth2.requested_service.unverifiable` | Warn: transient inability to verify usability of a selected requested service; redirect with `temporarily_unavailable`. An unselected service follows `incremental_required` with `not_selected` instead | `agent_id`, policy-rendered `principal`, `reason=unverifiable_transient`, `error_kind=transient` |
| `requested_service_evaluation_failed` | Security audit / `oauth2.requested_service.evaluation_failed` | Error: evaluator dependency failure handled as fail-closed `server_error` | `agent_id`, known `service_id`, policy-rendered `principal`, `reason=dependency_failed`, `error_kind=transient` |

Broker principals are `id.Principal` under the reserved `principal` key; never render them manually. `error_kind` replaces the pre-merge `kind` discriminator with the canonical key and has the closed set `definitive`, `transient`. Log errors once at the handling layer and only as typed safe metadata, never a wrapped cause or provider description.

Closed reason sets:

- Focused consent: `not_selected`, `required_scopes_uncovered`, `missing`, `unusable_definitive`.
- Submission rejection: `binding_mismatch`, `unsatisfied`.
- Scope rejection: `malformed_scope`, `unknown_service`, `service_not_eligible`, `binding_mismatch`.
  Map typed/sentinel permission errors; never use `err.Error()` or echo the requested scope.
- Resolution/evaluation failure: `dependency_failed`. This safe coarse classification is sufficient;
  do not parse a wrapped cause to invent a more detailed reason or record its text.
- Authorization continuation rejection: `invalid_authorization_session` (the AuthorizationSessionClaims JWE, not a UserSession). Authorize collapses every continuation failure, including expiry and mismatch, into this reason.
- Consent authorization-session rejection: `invalid_authorization_session` (empty, repeated, forged or malformed token, with the same meaning as authorize), `expired_authorization_session`, `agent_mismatch` (sealed agent differs from the requested agent) and `principal_mismatch` (sealed principal differs from the authenticated principal). Consent keeps the distinct reasons because it already returns distinct HTTP errors for them. Consent retains existing HTTP status codes and error responses and logs no token or validation cause.
- Requested-service unverifiability: `unverifiable_transient`.
- Local refresh refusal: `binding_unresolved`, `not_selected`, `required_scopes_uncovered`, `missing`, `unusable_definitive`, `service_lookup_failed`, `grant_lookup_failed`, `permission_set_lookup_failed`, `unverifiable_transient`.

`servicescope.Outcome.Reason()` returns `OutcomeReason`, the shared log projection of an Outcome. Its constants cover `not_selected`, `required_scopes_uncovered`, `usable`, `missing`, `unusable_definitive` and `unverifiable_transient`. It is distinct from structural `CoverageFailure` (which it coarsens for logs) and verified `Usability` (which it projects after selection/coverage precedence). Authorization and local refresh both use `unverifiable_transient` for unverifiable usability of a selected service. Not selected takes precedence over transient usability: `servicescope.AnyTransient` ignores unselected outcomes, so an unselected service whose usability is unverifiable is logged as `not_selected`. These pre-merge corrections replace the former refresh-only `usability_unverifiable` and ambiguous continuation `invalid_session` labels, as approved in [the remediation proposal](https://github.com/zalando-incubator/agentic-identity-broker/pull/323#issuecomment-7047148239).

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

The existing challenge record adds only `token_exchange.challenge_scope_present` and `token_exchange.challenge_resource_metadata_present`, each with value set `{true, false}`. These are log attributes, not span or metric attributes. Omit invalid optional values from the challenge without changing its status, body, exchange result or authorization decision.

## Traces and metrics

No new metric instruments, span-attribute families, instrumentation scopes or namespaces are introduced. Reuse production HTTP, storage, session-refresh, token-exchange and ExtProc instrumentation. Pure scope parsing/coverage adds no spans. Existing `token_exchange.*` attributes keep the value sets in `docs/reference/token-exchange.md#diagnostic-attributes`.

The existing `oauth2.token_proxy` span (`upstream` scope, `Internal` kind) records proxy-refresh scope refusals before forwarding:

| Outcome | `error.type` | `http.response.status_code` | Span status |
|---|---|---|---|
| Malformed, unknown or ineligible broker scope | `definitive` | `400` (`invalid_scope`) | `codes.Error` |
| Scope-validation dependency unavailable | `transient` | `500` (`server_error`) | `codes.Error` |

These are the closed scope-refusal `error.type` values on this span. The client-caused 400 still marks this `Internal` operation as failed; it does not mark the inbound HTTP `Server` span as an error. Status descriptions are constant; no scope, request field, credential or wrapped cause is recorded. `TokenOutcomeService.Proxy` emits the corresponding bounded audit record using the shared domain reason classifier; the HTTP adapter retains response mapping and span status, without a second audit.

`RefreshTokenSessionRepo.CreateSuccessor` adds `storage.create_successor.refresh_token_session` in the existing `storage` scope, with `Client` kind, `db.system=postgresql` and `db.operation=RefreshTokenSessionRepo.CreateSuccessor`. It wraps predecessor lookup, inheritance validation and the existing child Create call. `startStorageSpan` records normal not-found/validation outcomes as `storage.outcome`, dependency failures as bounded `error.type` with `codes.Error`, and caller cancellation as an outcome, following the storage standard. Neither refresh signatures nor binding/scope values are recorded.

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
