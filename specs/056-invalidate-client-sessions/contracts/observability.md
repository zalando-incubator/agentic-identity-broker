# Contract: Observability

**Status**: Internal, following `coding-standards/LOGGING.md` and `coding-standards/OPENTELEMETRY.md`. Event identifiers are a dashboard contract (LOGGING §2.2).

## Log events

| Event | Level | When | Required attributes |
|---|---|---|---|
| `session.oauth2.client_identity_changed` (new) | `Info` | The winning writer commits a revision increment | `service_id`, `client_identity_revision` (committed integer), `channel`. The `admin_update` channel also requires `operator_principal` as `id.Principal`. |
| `session.oauth2.credential_source_failed` (existing) | `Warn` for `identity_missing`, `identity_mismatch`, or `identity_superseded`. `Error` for every other source reason. | Identity enforcement denies use, or source resolution fails. Display fallback also records its source failure. | `service_id`, `operation`, `outcome=failed`, `reason`, `oauth2_session` (bounded `ErrorMetadata`) |
| `session.oauth2.refresh_failed` (existing) | No record for a credential-source or identity denial | Other refresh failures retain the [existing refresh contract](../../029-proactive-token-refresh/contracts/observability.md#structured-log-events). Source failures use only `credential_source_failed`. | Unchanged for other failures: `principal`, `session_id`, `service_id`, `triggered_by`, `oauth2_session`. Include `public_client` only when known. |

The source-failure level depends on `reason`, not the legacy metadata classification `error_kind=infrastructure`.
An ordinary disconnected display result does not emit a failure event. A display source failure does, even though persisted evidence supplies the response.

### Committed-writer attribution

The mutation result determines the writer and revision:

- `admin_update` requires a successful `ThirdpartyOAuth2ProviderRepository.Update` with `identityChanged == true`. That result comes from the mutation lock or transaction. The event uses the committed revision returned on that update's entity.
- `filesystem_observation` requires a successful observation with `applied && changed`. The event uses that observation's returned revision. It does not attribute the change to the user whose request first reads the file.

Never derive attribution from an unlocked before/after comparison or a follow-up read. Another writer can commit between those reads.

A filesystem metadata PUT can pause after its initial read while an observation increments the revision without changing the administrative version.
The successful PUT preserves that evidence and returns `identityChanged == false`. Only the observation emits a change event, with `channel=filesystem_observation`.

Errors, rolled-back mutations, lost observation preconditions, initialization, and unchanged identities emit no identity-change event.
Same-ID secret rotation, metadata updates, and same-identity source transitions also emit none. A dry-run sweep never emits a change event.
Each winning increment emits one post-commit event. These operational logs are not a durable exactly-once ledger (LOGGING §8).

### Closed attribute sets

Use typed string constants for values that dashboards group by. Do not derive values from input or error text.

| Attribute | Closed set | Applies to |
|---|---|---|
| `channel` | `admin_update`, `filesystem_observation` | Identity-change event only |
| `operation` | `authorization_initiation`, `code_exchange`, `token_acquisition`, `refresh`, `usability_gate`, `usability_display` | Source-failure event |
| `outcome` | `failed` | Source-failure event |
| `reason` (identity denial) | `identity_missing`, `identity_mismatch`, `identity_superseded` | Source-failure event at `Warn` |
| `reason` (source unavailable) | `missing_binding`, `generation_changed`, `not_found`, `permission_denied`, `not_regular`, `too_large`, `empty`, `read_failed` | Source-failure event at `Error` |
| `triggered_by` | `on-demand`, `background`, `sweep` | Refresh records, when the trigger is known. Preserve the released `on-demand` spelling. |

`identity_missing` means required establishment evidence is absent or empty, including legacy callback claims.
`identity_mismatch` means the recorded and effective client IDs differ. `identity_superseded` means their revisions differ, including after rollback.
`generation_changed` means observation contention exhausted the bounded retry limit. It is source unavailability, not confirmed invalidation.
The remaining source reasons retain `model.CredentialSourceReason` semantics. Unclassified source failures use `read_failed`, never an arbitrary reason.

The top-level `operation` identifies the failed phase:

| Operation | Boundary | `oauth2_session.operation` |
|---|---|---|
| `authorization_initiation` | Resolve identity before provider authorization | `code_exchange` (existing initiation convention) |
| `code_exchange` | Validate callback identity before code exchange or session replacement | `code_exchange` |
| `token_acquisition` | Check identity before decrypting or releasing a stored access token | `session_lookup` |
| `refresh` | On-demand, manual, background, or sweep refresh evaluation, including dry-run classification | `refresh` |
| `usability_gate` | Agent authorization or grant validation through `UsabilityGate` | `session_lookup` |
| `usability_display` | Consent detail or session list/detail/refresh-summary fallback through `UsabilityDisplay` | `session_lookup` |

The existing `oauth2_session` group contains `operation`, `failure_detail`, `error_kind`, `dependency`, `http_status_code`, and `oauth_error_code`.
For an uncanceled identity/source denial, retain `failure_detail=credential_source_unavailable`, `error_kind=infrastructure`, and `dependency=credential_source`.
No provider call occurred, so `http_status_code=0` and `oauth_error_code=""`. Do not invent provider rejection metadata.
Caller cancellation retains the existing `caller_canceled` classification rather than masquerading as an identity change.
Cancellation alone does not emit an identity/source-failure event. It retains the existing canceled outcome rather than an infrastructure-failure record.
No new `ErrorMetadata` operation, detail, kind, or dependency is required. The top-level `reason` distinguishes identity denial from source unavailability.

### Failure ownership and record safety

Log each failed logical use once, at the layer that handles its denial or fallback. A helper that returns an error does not log it.
The synchronous response boundary, background worker, or sweep candidate handler owns a denied use. The display evaluator owns its persisted-evidence fallback.
Wrappers and callers must not add a second generic failure record for the same handled failure.
Independent consumers of a shared refresh can each record their own denied use, but not another record for each intermediate layer.

Authorization initiation, callback, and manual refresh retain domain ownership of credential-source failures. Their HTTP handlers preserve the response or redirect without adding a generic failure record for the same denial. Constructor-injected handler tests cover this boundary, including Warn-level identity denials and Error-level unavailable sources.

An unknown service during authorization initiation retains its `404 service_not_found` outcome. The HTTP handler owns its single Warn record; the domain does not also classify that caller-input failure as a credential-source infrastructure failure. Unavailable identity sources for existing services retain the domain-owned Error record.

Suppress `session.oauth2.refresh_failed` for every credential-source failure, including all identity reasons, across on-demand, manual, background, and sweep paths.
Keep the existing failed-candidate counts and failure counters. A dry run still writes no identity/session state and changes no refresh counter.
Aggregate sweep audit totals remain distinct from per-use failure records. A committed change event and a subsequent denied-use event describe different facts.

Use the constructor-injected `*slog.Logger` and a context-aware method. Messages are constant lowercase phrases, not event identifiers or formatted values.
Use canonical flat keys such as `service_id`, `session_id`, `principal`, and `operator_principal`. Do not introduce `action`, `err`, or `cause` aliases.
Known session-owner attributes use `principal` as `id.Principal`. Admin attribution uses `operator_principal` as `id.Principal`, never `.String()` or `slog.String`.
The production principal policy renders these values. Do not add an owner or operator to `Debug` records.
The context handler alone adds `trace_id`, `actor`, and `calling_peer`. A filesystem observation never invents an administrative operator.

Never log client-ID values, credential-file paths or contents, tokens, secrets, ciphertext, sealed state, authorization codes, PKCE verifiers, or raw payloads.
This restriction includes previous and current identities, path basenames, wrapped errors, messages, and nested attributes.
Do not log full entities, request structs, provider errors, `*os.PathError`, or `*url.Error`. Use bounded metadata instead of their causes.
The public `client_id` is non-secret, but this feature does not need its value in telemetry.

## Spans and metrics

No new span, instrument, instrumentation scope, or attribute namespace is required. Do not add principal span attributes, including while #356 remains unavailable.
Do not add identity values, file paths, `channel`, or `reason` to spans or metric labels.
No principal appears in a metric, and this feature adds no principal or client-ID span attribute.

Existing token-exchange diagnostics distinguish `credential_identity_changed` from `credential_source_unavailable` within the registered `token_exchange` namespace.
Existing refresh failure counters retain `failure_detail=credential_source_unavailable`. Existing refresh spans retain `oauth2.refresh.outcome=failed` for a failed refresh.
Preserve the existing bounded metric attribute sets. No per-session, per-principal, per-revision, or client-ID metric dimension is introduced.
Recoverable HTTP 4xx outcomes do not set server-span error status. Cancellation remains an outcome, not a span error.
Span status descriptions stay constant. Never call `RecordError` or copy a raw error into an attribute, event, or status description.
Telemetry failure remains fail-open and cannot change an authentication or authorization result.

## Required tests and task mapping

Inject `slog.NewJSONHandler(&buf, ...)` through production constructors. Do not add test-only production APIs.
Assert event identifiers, levels, attributes, and event counts, not message text. Reset or isolate setup logs before each operation.
Exercise the production principal-policy chain and assert its output. Keep `TestLoggedPrincipalsAreTyped` as the static type guard.

| Task | Required evidence |
|---|---|
| T035 | In `internal/domain/thirdparty/service_test.go` and `internal/adapters/http/handlers/admin/services_handler_test.go`, assert one `Info` event after an accepted stored-ID increment. Assert committed service/revision, `channel=admin_update`, and policy-rendered typed operator attribution. Rejected/conflicting/rolled-back updates, initialization, and same-ID changes emit none. The deterministic paused-PUT/intervening-observation race emits only the observation's event. Metadata operations never read files. |
| T040 | Extend adjacent `oauth2session` tests with constructor-injected JSON buffers. Assert exactly one event per winning observation, no event for lost preconditions/no-ops, and the winning committed revision/channel/service. Cover all three identity reasons, source reasons, operations, display fallback, and refresh triggers. Assert `Warn` versus `Error`, bounded metadata, and suppression of duplicate `refresh_failed` or generic failure records. Real and dry-run sweep results preserve their count/write contracts. |
| T064 | Exercise initiation, callback, valid/unknown-expiry token acquisition, every refresh trigger, sweep/dry run, usability gate, and display fallback with distinct sensitive sentinels. Scan complete log records for credential, old/new client-ID, file-content, full-path, and basename absence. Scan recorded spans, events, status, resources, and collected metrics for the same values. Assert no new principal span attribute, namespace, instrument, or metric dimension. Verify principal policy output and unchanged outcomes with telemetry disabled. |

For telemetry safety tests, install an in-memory span recorder and `sdkmetric.ManualReader` before constructing the services.
Restore global providers and shut down test providers in cleanup. Use existing test bootstrap or `Builder.WithTracerProvider`, not new runtime provider plumbing.
Retain background-span root/link and metric-attribute assertions in `background_refresh_test.go` and `sweep_test.go`.
The safety assertion pattern in `internal/extproc/server/exchange_safety_test.go` covers full spans and metric resources, not only span attributes.

## T070 review disposition

The design review resolves unspecified levels/value sets, duplicate source/refresh records, gate-operation coverage, and unclassified metadata on new evaluation paths.
It preserves the committed-writer rules and maps the required buffer, principal-policy, and telemetry-safety tests to T035/T040/T064.
Existing source-error tests cover safe attributes and synchronous duplicate suppression. Existing observation tests cover changed/no-op events and some sensitive-value absence.
Those tests do not establish complete coverage of this contract. T035/T040/T064 must supply and verify the remaining assertions before feature completion.
T070 completes the observability design gate only. It does not claim runtime compliance or passing tests.
