# Observability Contract: Service Accounts

**Scope**: Admin service-account operations of the broker (issue #471). No ExtProc signal.

**Binding sources**: `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`,
Constitution Principles I and XV, spec FR-022, FR-023, SR-006.

**Status**: Proposed. The maintainer reviews the amendment in section 4 in the PR (STD-LOGGING
in [plan.md](../plan.md#review-items)).

## 1. Audit records

Written by `thirdparty.ServiceAccountService` with `InfoContext`, after the write commits. An
update that changes nothing writes no record. A refused or failed operation writes no record from
this table.

| `event` | Written when |
|---|---|
| `service.service_account.created` | An account was created |
| `service.service_account.credential_rotated` | An update replaced the stored credential |
| `service.service_account.ceiling_changed` | An update changed the ceiling and not the credential |
| `service.service_account.renamed` | An update changed the `canonical_id` and neither the credential nor the ceiling |
| `service.service_account.metadata_changed` | An update changed only labels or annotations (added by the object-metadata amendment, `specs/20261127-093439-object-labels-annotations/spec.md` FR-024) |
| `service.service_account.deleted` | An account and its credential were erased |

One record per committed write. When one update changes several aspects, `event` is the first
match in the order above and `changes` lists all of them.

| Key | Value | Events |
|---|---|---|
| `operator_principal` | `id.Principal` of the acting administrator | all |
| `service_id` | `id.ServiceID` | all |
| `service_account_id` | `id.ServiceAccountID` | all |
| `canonical_id` | The account's name after the write (before it, for `deleted`) | all |
| `kind` | `jwt_bearer` | all |
| `scopes` | The ceiling after the write (before it, for `deleted`) | all |
| `credential` | Credential summary group, section 3 | all |
| `changes` | List from the closed set `credential`, `scopes`, `canonical_id`, `metadata` | the four update events |
| `previous_observed_canonical_id` | Name before the write | update events, when `changes` has `canonical_id` |
| `previous_observed_scopes` | Ceiling before the write | update events, when `changes` has `scopes` |
| `previous_observed_credential` | Credential summary group before the write | `credential_rotated` |

No record carries a label or annotation key or value, before or after the write.

`trace_id`, `actor` and `calling_peer` come from the context handler.

## 2. Security and failure records

| `event` | Level | Written when | Keys |
|---|---|---|---|
| `service.service_account.credential_rejected` | `Warn` | A create or update was refused by a credential rule | `operator_principal`, `service_id`, `service_account_id` (update only), `operation` (`create`, `update`), `reason` |
| `service.service_account.write_failed` | `Error` | Encryption or storage failed during create, update or delete | `operator_principal`, `service_id`, `service_account_id` (when known), `operation` (`create`, `update`, `delete`), `error_kind` |

`reason` is one of: `unsupported_format`, `unsupported_use`, `missing_jwt`, `too_large`,
`malformed`, `unsigned`, `expired`.

`error_kind` is `encryption` or a `storage.ErrorKind`. The wrapped cause is not logged: this is a
credential boundary (LOGGING.md section 4).

Other refusals (`400` for non-credential members, `404`, `409`, `412`) are ordinary client
errors. The handler does not add a record for them; the request log already carries the status.

## 3. Credential summary group

`model.CredentialSummary` implements `slog.LogValuer` and renders:

| Key | Value |
|---|---|
| `format` | `signed_jwt` |
| `use` | `access_token` |
| `iss`, `kid` | As the token states them, each capped at 256 bytes; omitted when not stated |
| `sub` | `id.Principal`, capped at 256 bytes, rendered through the principal policy (LOGGING.md section 5); omitted when not stated |
| `exp` | Whole seconds since the epoch; omitted when not stated |
| `fingerprint` | `SHA256:<base64>` |

The values come from an unverified token and are untrusted input. They appear only as attribute
values, never in a message or an error string.

## 4. Amendment to `coding-standards/LOGGING.md` (same PR)

Section 6.2, new row:

| Data | Permitted form |
|---|---|
| Credential fingerprint | `SHA256:` and the base64 SHA-256 digest of a stored service-account credential, only in `service.service_account.*` audit records. Never the credential, a fragment of it, or its ciphertext. |

Rationale for the PR description: FR-022 requires audit records to carry the credential summary so
that a rotation can be recognised. Section 6.2 already permits `iss`, `kid` and `exp`, and permits
`sub` as an `id.Principal` through the principal policy, but has no row for a credential
fingerprint. The digest cannot be reversed to the credential. The row is scoped to these audit
events.

## 5. Spans

No new domain span. Each method of the PostgreSQL repository starts a `Client` span through
`startStorageSpan` in the existing `storage` scope:

| Span | `db.operation` | Method |
|---|---|---|
| `storage.create.service_account` | `Create` | `Create` |
| `storage.get.service_account` | `Get` | `Get` |
| `storage.get_by_canonical_id.service_account` | `GetByCanonicalID` | `GetByCanonicalID` |
| `storage.list.service_account` | `ListByService` | `ListByService` |
| `storage.update.service_account` | `Update` | `Update` |
| `storage.delete.service_account` | `Delete` | `Delete` |
| `storage.count.service_account` | `CountByService` | `CountByService` |

Attributes are those `startStorageSpan` already sets (`db.system`, `db.operation`,
`storage.outcome`, `error.type`). Not found, name conflict and version mismatch are
`storage.outcome`, not errors. No span carries a service-account ID, a claim value, the
fingerprint or a principal. No new attribute namespace is registered.

Each span gets a description in `docs/reference/telemetry-descriptions.yaml`, and
`docs/reference/telemetry.md` is regenerated with `just telemetry-doc`.

The inbound `otelchi` server span and the existing `encryption` spans cover the rest of a request.

## 6. Metrics

None. This feature creates no instrument.

## 7. Never recorded

In no log record, span name, span attribute, span event, status description, metric or error
string: the stored JWT, any segment or substring of it, the credential document, its ciphertext,
or the request body.

## 8. Required tests

| Path | Assertion |
|---|---|
| Create, rotation, refused write (each rule of section 2), list, single read, status, delete | A JSON buffer logger injected through the production constructors does not contain the JWT under test or any of its three segments |
| The same paths | An in-memory span recorder shows no attribute, event or status description containing the JWT or a segment |
| The same paths | Response bodies and headers do not contain the JWT or a segment |
| Credential summary in audit events | `sub` is the principal policy output, injected through a test policy that changes the value, so the test fails if `sub` bypasses the policy |
| Each audit event | `event`, `operator_principal` (policy output), `service_id`, `service_account_id`, `changes` and the `previous_observed_*` keys are asserted; message text is not |
| No-op update, refused write, failed write | No record from section 1 is written |
| Storage spans | Name, kind `Client`, and `storage.outcome` for not found, name conflict and version mismatch |
