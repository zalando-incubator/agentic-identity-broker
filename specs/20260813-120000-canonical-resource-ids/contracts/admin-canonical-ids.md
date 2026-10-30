# Admin API Contract: Canonical Resource IDs

This is the pre-implementation contract delta for `api/admin/openapi.yaml`. The stakeholder confirmation is recorded in `spec.md` under Assumptions.

## Identifier inputs

All listed values are strings. A value is either a UUID or a canonical ID for the target resource type; canonical values are case-sensitive URL-safe tokens of 1–128 characters using only `A–Z`, `a–z`, `0–9`, `.`, `_`, and `-`, and must not be syntactically UUID-shaped.

| Endpoint or request field | Target type | Contract |
|---|---|---|
| `/api/agents/{agent-id}` GET, PUT, DELETE | Agent | Path accepts UUID or agent canonical ID. |
| `/api/agents/{agent-id}/client-credentials` POST, GET, DELETE | Agent | Path accepts UUID or agent canonical ID. Credential IDs remain out of scope. |
| `/api/services/{service-id}` GET, PUT, DELETE | Service | Path accepts UUID or service canonical ID. |
| `/api/services/{service-id}/protected-resources` GET, POST and `/api/services/{service-id}/protected-resources/{resource}` PUT, PATCH, DELETE | Service | Path `{service-id}` accepts UUID or service canonical ID; `{resource}` remains the percent-encoded URI segment. |
| `/api/permission-sets/{permission-set-id}` GET, PUT, DELETE | Permission set | Path accepts UUID or permission-set canonical ID. |
| `GET /api/permission-sets?service_id=` | Service | Query accepts UUID or service canonical ID. |
| `service_requirements[].service_id` | Service | Write reference accepts UUID or service canonical ID. |
| `permission_sets[].permission_set_id` | Permission set | Write reference accepts UUID or permission-set canonical ID. |
| `service_scopes[].service_id` | Service | Write reference accepts UUID or service canonical ID. |

OpenAPI path/query/reference schemas above must be `type: string` without `format: uuid`, with UUID and canonical examples.

## Resource write shapes

`AgentCreateRequest`, `AgentUpdateRequest`, `ServiceCreateRequest`, `ServiceUpdateRequest`, and `CreatePermissionSetRequest` add optional `canonical_id`.

- Create: omitted means no canonical ID; supplied values must be valid.
- Update: omission preserves the existing canonical ID; `null` removes it; a non-null valid value assigns or replaces it.
- An empty, UUID-shaped, or otherwise invalid canonical ID returns `400`.
- A same-resource-type duplicate returns `409`; a matching value in a different resource type is permitted.

## Resource read shapes

Every `Agent`, `Service`, and `PermissionSetResponse` keeps its existing `id` and adds `canonical_id`:

| Field | Meaning | Constraints |
|---|---|---|
| `id` | Stable entity UUID | read-only string, `format: uuid` — unchanged from today |
| `canonical_id` | Assigned canonical identifier | nullable, read-only string; not UUID-formatted |

No `uuid` field is added; `id` already carries the UUID. The change is additive, so existing UUID-only clients are unaffected.

Example with a canonical ID:

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","canonical_id":"research-agent"}
```

Example without one:

```json
{"id":"550e8400-e29b-41d4-a716-446655440000","canonical_id":null}
```

## Nested reference shapes

Nested reference identifiers stay UUIDs on read; only the write side relaxes to accept a canonical ID. Read and write share one schema per reference so a GET response round-trips through PUT verbatim.

| Schema | Sharing | Identifier field | Read value | Write value |
|---|---|---|---|---|
| `ServiceScope` | shared (request + response) | `service_id` | target UUID | UUID or service canonical ID |
| `AgentPermissionSetEntry` | shared (request + response) | `permission_set_id` | target UUID | UUID or permission-set canonical ID |
| `ServiceRequirement` / `ServiceRequirementRequest` | already split | `service_id` | target UUID (`format: uuid` retained on response) | UUID or service canonical ID |

For the shared `ServiceScope` and `AgentPermissionSetEntry` schemas, relax `service_id` / `permission_set_id` from `format: uuid` to `type: string`; responses continue to emit UUIDs. No `service_uuid` / `permission_set_uuid` companion fields are added, and no additional schema split is introduced. On `ServiceRequirementRequest` (request only) relax `service_id`; the `ServiceRequirement` response keeps `format: uuid` and its existing read-only `service_name`.

## Canonical presentation preference (read)

A read (`GET` single or list) MAY send an RFC 7240 preference to render nested reference identifiers in canonical form. The top-level `id` is unaffected and stays UUID.

| Direction | Header | Meaning |
|---|---|---|
| Request | `Prefer: reference-id=canonical` | Render `service_id` / `permission_set_id` as the target's canonical ID where one exists, otherwise its UUID. |
| Request | absent, `reference-id=uuid`, or unrecognized | Default: nested references are UUIDs (pre-feature behavior). |
| Response | `Preference-Applied: reference-id=canonical` | Sent only when the canonical preference was honored. |
| Response | `Vary: Prefer` | Sent on every in-scope read so caches do not cross-serve representations. |

No body schema changes: the nested identifier field is already `type: string`, so a canonical value is schema-valid, and every rendered value remains valid input to the corresponding write reference (a preference read round-trips through PUT). Canonical rendering resolves each referenced target's `canonical_id` as a batch lookup performed only when the preference is present.

Add `Prefer` as an optional `in: header` parameter and document the `Preference-Applied` and `Vary` response headers on the in-scope `GET` operations for agents, services, and permission sets (single-resource and list). Clients intended for human operators SHOULD send `reference-id=canonical` by default.

## Error and atomicity contract

| Operation | Condition | Status | Result |
|---|---|---:|---|
| Path/query lookup | UUID and canonical lookup both fail | 404 | No resource is acted on. |
| Create/update | Empty or UUID-shaped `canonical_id` | 400 | No mutation. |
| Create/update | Canonical ID duplicated within target table | 409 | Existing and attempted resources remain unchanged. |
| Create/update | Any target reference is unresolved or wrong resource type | 400 | Entire write is rejected; no partial relationship persists. |

All create/update examples in `api/admin/openapi.yaml` must be revised to use the above shapes consistently.