# Contract: Observability and Audit

**Standards**: `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md` (Principle XV) | **Spec**: SR-004, Domain Events | **Research**: R15

## Spans and metrics

This feature adds no span, span attribute or metric instrument and renames none. The OPENTELEMETRY.md registry and the observability contracts of features 054 and earlier are unchanged. After each Go change, `just telemetry-doc` runs, and any regenerated `docs/reference/telemetry.md` is committed.

## Audit log events

All events below are Info-level records with a constant lowercase message and an `event` attribute, as the existing `permissionset.permission_set.*` and `consent.grant.*` events use. Capability IDs, group IDs and canonical IDs are configuration identifiers, not credentials. No event carries a token, scope value from a provider response, or credential.

| Event | Emitted by | Fields |
|---|---|---|
| `capability.capability_group.created` | `capability.Service.Create` | `capability_group_id`, `canonical_id` (if set), `capabilities` (list of `{id, canonical_id, read_only, service_scopes}`) |
| `capability.capability_group.updated` | `capability.Service.Update` | As created, plus `previous_observed_capabilities`, `previous_observed_updated_at` |
| `capability.capability_group.deleted` | `capability.Service.Delete` | `capability_group_id`, `canonical_id` (if set) |
| `permissionset.permission_set.created` / `.updated` (changed) | `permissionset.Service` | `capabilities` (entries) replaces `service_scopes`; `.updated` has `previous_observed_capabilities` in place of `previous_observed_service_scopes` |
| `agents.agent.created` / `.updated` (changed) | `agents.Service` | `permission_sets` entries now include `capabilities` when set |
| `consent.grant.created` / `.updated` / `.revoked` (changed) | `consent.Service` | `granted_permission_sets` entries carry `granted_capability_ids` instead of `included_service_ids`; `principal` stays `id.Principal` |

## Diagnostic logs

When exchange or evaluation fails closed because of a stale or undeclared capability (research R7), the existing grant-authorization diagnostic is used. That covers the stage and the detail `grant_permission_set_missing` or `grant_permission_set_undeclared`. One attribute is added to the existing log record: `capability_id`, the UUID of the first offending capability. Principals appear only as `id.Principal`, under the reserved `principal` key.

## Required tests

- **Buffer-logger tests**: Tests inject a buffer logger through the production constructors (`capability.NewService`, `consent.NewService`) and assert the event names and field sets above.
- **Credential absence**: Grant-event tests assert that no `subject_token` or provider token appears.
- **Typed principals**: The existing `TestLoggedPrincipalsAreTyped` stays green.
