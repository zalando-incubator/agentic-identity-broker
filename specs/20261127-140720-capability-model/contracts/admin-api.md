# Contract: Admin API 4.0.0 — Capability Groups and Capability-Based Permission Sets

**Target file**: `api/admin/openapi.yaml` (`info.version: 3.0.0` → `4.0.0`) | **Spec**: API-001, US1.1–US1.5 | **Research**: R3, R11

This contract needs stakeholder approval before implementation (Constitution IV, X). The OpenAPI change lands in Phase 2c, before any handler code.

## Versioning

- This is a breaking change on the existing routes, so the version becomes `4.0.0`. There is no versioned route, compatibility field or alias. This follows ARCHITECTURE.md § 5 and the precedent of ADR `047-unique-agent-external-id-reconcile`.
- `info.description` gains a 4.0.0 change summary. `docs/guides/manage-agents-and-services.md` gains "Migrate administrative consumers to Admin API 4.0.0".

## New resources

| Method | Path | Success | Errors |
|---|---|---|---|
| `GET` | `/api/capability-groups` | `200` `CapabilityGroupListResponse` | 401, 403 |
| `POST` | `/api/capability-groups` | `201` `CapabilityGroupResponse`, `Location` | 400, 401, 403 |
| `GET` | `/api/capability-groups/{capability-group-id}` | `200` `CapabilityGroupResponse` | 401, 403, 404 |
| `PUT` | `/api/capability-groups/{capability-group-id}` | `200` `CapabilityGroupResponse` | 400, 401, 403, 404, 409 |
| `DELETE` | `/api/capability-groups/{capability-group-id}` | `204` | 401, 403, 404, 409 |

- **Path ID**: `{capability-group-id}` accepts a UUID or the group's canonical ID, like `PermissionSetId`.
- **Prefer header**: `PreferCanonicalReferences` applies to every response that holds nested `service_id` or `capability_id` references.
- **Pagination and authentication**: Both follow the existing permission-set list and authentication schemes.
- **Error bodies**: These use the existing error envelope.

### Schemas

```yaml
CapabilityGroupRequest:          # POST and PUT (full replacement)
  type: object
  required: [name, description, capabilities]
  properties:
    canonical_id: { $ref: '#/components/schemas/CanonicalId' }   # optional; omitted on PUT clears it, matching permission sets
    name:        { type: string, minLength: 1, maxLength: 255 }
    description: { type: string, minLength: 1 }
    capabilities:
      type: array
      minItems: 1
      items: { $ref: '#/components/schemas/CapabilityRequest' }

CapabilityRequest:
  type: object
  required: [canonical_id, label, description, read_only, service_scopes]
  properties:
    id:           { type: string, format: uuid, description: "PUT only: keeps this capability's identity. Unknown or foreign IDs are 400." }
    canonical_id: { $ref: '#/components/schemas/CanonicalId' }   # required, globally unique across capabilities
    label:        { type: string, minLength: 1, maxLength: 255 }
    description:  { type: string, minLength: 1 }
    read_only:    { type: boolean }
    service_scopes:
      type: array
      minItems: 1
      items: { $ref: '#/components/schemas/CapabilityServiceScope' }

CapabilityServiceScope:
  type: object
  required: [service_id, scopes]
  properties:
    service_id: { type: string, description: UUID or service canonical ID }
    scopes:     { type: array, minItems: 1, uniqueItems: true, items: { type: string, minLength: 1 } }

CapabilityGroupResponse:
  allOf:
    - $ref: '#/components/schemas/CapabilityGroupRequest'
    - type: object
      required: [id, created_at, updated_at]
      properties:
        id: { type: string, format: uuid }
        capabilities: { type: array, items: { $ref: '#/components/schemas/CapabilityResponse' } }
        created_at: { type: string, format: date-time }
        updated_at: { type: string, format: date-time }

CapabilityResponse:              # CapabilityRequest with id required
```

### Rules

- **Capability canonical ID**: Each capability's `canonical_id` is required. It is unique within the request and globally across capabilities, and follows the canonical-ID format: 1–128 characters of `[A-Za-z0-9._-]`, not UUID-shaped. A violation is `400` (US1.2).
- **Group canonical ID**: Optional; when present it is unique among groups (`400`).
- **Service scopes**: `service_scopes` needs at least one entry with no duplicate service. Every service must exist (`400`) and every scope must be defined on its service (`400`), as for today's permission-set scopes.
- **`PUT` matching by `id`**: A capability with an `id` that belongs to this group is updated. One without an `id` is created. A stored capability that is omitted is deleted, which is `409` when a permission set includes it or an active grant references it (US1.5, FR-008).
- **Group `DELETE`**: `409` under the same reference rule for any of the group's capabilities. Nothing changes on `409`.

## Changed: Permission sets (`/api/permission-sets`, `/api/permission-sets/{permission-set-id}`)

```yaml
CreatePermissionSetRequest:
  type: object
  required: [name, description, capabilities]
  properties:
    canonical_id: { $ref: '#/components/schemas/CanonicalId' }
    name:         { type: string }
    description:  { type: string }
    capabilities:
      type: array
      minItems: 1          # no maxItems (FR-003)
      items: { $ref: '#/components/schemas/CapabilityEntry' }
  not:
    required: [service_scopes]

CapabilityEntry:
  type: object
  required: [capability_id, requirement_type]
  properties:
    capability_id:    { type: string, description: UUID or capability canonical ID }
    requirement_type: { type: string, enum: [mandatory, optional] }

PermissionSetResponse:
  # id, canonical_id, name, description, created_at, updated_at as today
  capabilities:
    type: array
    items:
      type: object
      required: [capability_id, canonical_id, requirement_type]
      properties:
        capability_id:    { type: string }   # UUID, or canonical ID with Prefer reference-id=canonical
        canonical_id:     { type: string }   # always present: the policy key
        requirement_type: { type: string, enum: [mandatory, optional] }
```

- `service_scopes` is removed from requests and responses, and `ServiceScope` is deleted from the Admin schemas.
- **Request rules (`400`)**:
  - The request still sends `service_scopes`. The handler checks the raw member map, as `Update` already does for `canonical_id`.
  - `capabilities` is empty.
  - An entry is duplicated.
  - An entry references an unknown capability.
- **Update rule (`409`)**: An update returns `409` when it drops a capability that an agent entry lists, or makes mandatory a capability that an agent entry marks optional (research R3).
- **Filter**: `GET /api/permission-sets?service_id=` keeps working. It matches sets that include any capability that needs the service.

## Changed: Agent permission-set entries

```yaml
AgentPermissionSetEntry:
  type: object
  required: [permission_set_id, requirement_type]
  properties:
    permission_set_id: { type: string }
    requirement_type:  { type: string, enum: [mandatory, optional] }
    capabilities:
      type: array
      minItems: 1
      description: Omit to use every capability of the set with the set's requirement types.
      items: { $ref: '#/components/schemas/CapabilityEntry' }
```

- **Rules (`400`, US1.4)**:
  - Every listed capability must be in the set.
  - There are no duplicates.
  - An entry may not mark `optional` a capability the set marks `mandatory`.
  - An empty array is invalid.
- **Accepted**: Restating mandatory, tightening optional to mandatory, and listing a subset.
- **Operations**: The rules apply to `POST /api/agents`, `PUT /api/agents/{agent-id}` and the collection reconcile `PUT /api/agents?external_id=`.
- **Reconcile no-op**: A capability list counts as part of the definition for the reconcile no-op comparison.
- **Responses**: `PermissionSetDeclarationResponse` echoes `capabilities` only when stored.
- **Service coverage**: The coverage rule (agents `FR-019`: service requirements must be covered by the declared sets) now gathers services through the sets' capabilities.

## Unchanged

- Error envelopes, ETags, authentication, other resources and session sweep are unchanged.
- The Admin API does not expose grants, so no grant schema changes here.

## Contract tests

- `tests/e2e/permission_sets_test.go` (rewritten for capability entries) and a new `tests/e2e/capability_groups_test.go` cover US1.1–US1.5.
- The existing OpenAPI lint and validation run in `just check`.
