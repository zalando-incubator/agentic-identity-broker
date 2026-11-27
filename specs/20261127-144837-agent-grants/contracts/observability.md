# Observability Contract: Agent Grants

**Scope**: Admin agent-grant operations of the broker and the changed service-account delete
(issue #472). No ExtProc signal.

**Binding sources**: `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`,
Constitution Principles I and XV, spec FR-019, SR-004.

**Status**: Proposed. No amendment to either standard is needed.

## 1. Audit records

These are written by `consent.AgentGrantService` with `InfoContext`, after the write commits. A
write that changes nothing writes no record. A refused or failed operation writes no record from
this table.

| `event` | Written when |
|---|---|
| `consent.agent_grant.created` | The agent had no agent grant and one was stored |
| `consent.agent_grant.replaced` | An existing grant (expired or not) was replaced with different content |
| `consent.agent_grant.deleted` | The grant and its bindings were deleted through the admin API |

| Key | Value | Events |
|---|---|---|
| `operator_principal` | `id.Principal` of the acting administrator | all |
| `agent_id` | `id.AgentID` | all |
| `grant_id` | `id.GrantID` | all |
| `permission_sets` | Granted set IDs after the write (before it, for `deleted`) | all |
| `granted_permission_sets` | Set ID → covered service IDs, same timing | all |
| `service_account_bindings` | Service ID → account ID, same timing | all |
| `valid_until` | RFC 3339 or absent, same timing | all |
| `previous_observed_permission_sets`, `previous_observed_granted_permission_sets`, `previous_observed_service_account_bindings`, `previous_observed_valid_until` | The stored content before the write | `replaced` |

The grant's principal is not logged: it is the agent's UUID, already carried as `agent_id`, and it
is not a person (LOGGING.md section 5 governs person principals). Deleting the agent itself
removes the grant through the store cascade. The existing agent-deletion record covers it, and no
`consent.agent_grant.deleted` record is written for it.

`trace_id`, `actor` and `calling_peer` come from the context handler.

## 2. Failure records

| `event` | Level | Written when | Keys |
|---|---|---|---|
| `consent.agent_grant.write_failed` | `Error` | Storage failed during `Put` or `Delete` after validation, other than a version mismatch or a missing reference | `operator_principal`, `agent_id`, `operation` (`put`, `delete`), `error_kind` (a `storage.ErrorKind`) |

Refusals (`400`, `404`, `412`) are ordinary client errors and get no record. The request log
already carries the status. The `409` on service-account deletion keeps #471's behaviour.

## 3. Spans

No new domain span. Each method of the PostgreSQL agent-grant repository starts a `Client` span
through `startStorageSpan` in the existing `storage` scope:

| Span | `db.operation` | Method |
|---|---|---|
| `storage.get.agent_grant` | `GetAgentGrant` | `Get` |
| `storage.save.agent_grant` | `SaveAgentGrant` | `Save` |
| `storage.delete.agent_grant` | `DeleteAgentGrant` | `Delete` |
| `storage.list_binding_agents.agent_grant` | `ListAgentsBindingServiceAccount` | `ListAgentsBindingServiceAccount` |

Attributes are the ones `startStorageSpan` already sets. Not found and version mismatch are
recorded as `storage.outcome`, not errors. No span carries an agent, account or grant ID, a
principal or a principal kind. No new attribute namespace is registered.

Existing spans of the human grant and session repositories keep their names. Their statements
gain the kind predicate.

Each new span gets a description in `docs/reference/telemetry-descriptions.yaml`, and
`docs/reference/telemetry.md` is regenerated with `just telemetry-doc`.

## 4. Metrics

None. This feature creates no instrument.

## 5. Never recorded

The feature reads no credential (SR-004). No signal carries a service-account credential, its
summary or its fingerprint. The agent-grant audit records name accounts by ID only.

## 6. Required tests

| Path | Assertion |
|---|---|
| Create, replace, delete | One record with the asserted `event`, `operator_principal` (principal-policy output through a test policy that changes the value), `agent_id`, `grant_id`, `permission_sets`, `granted_permission_sets`, `service_account_bindings`, `valid_until`; `previous_observed_*` on `replaced`. Message text is not asserted. |
| No-op write, each refusal rule, `412` | No record from section 1 |
| Storage failure on `Save` and `Delete` | One `write_failed` record with `error_kind`, and no record from section 1 |
| Every operation | `TestLoggedPrincipalsAreTyped` and `telemetrylint` pass; no plain-string principal key |
| Storage spans | Name, kind `Client`, and `storage.outcome` for not found and version mismatch |
