---
title: "Resource identifiers"
description: Living specification of how an administrator or automation addresses agents, third-party services, and permission sets by UUID or by a readable canonical ID, and chooses how references are shown. Every example on this page is checked against the acceptance suite.
---

# Resource identifiers

The broker gives every **Agent**, third-party service, and permission set a UUID. A UUID is safe
but hard to read and hard to type. An administrator can therefore give a resource a second,
readable name: its **Canonical ID**. This page describes where that name works and what it never
replaces.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

`admin@example.com` manages the broker through the admin API, by hand and with scripts. The broker
already knows the provider *GitHub* under the canonical ID `github-service`. It also knows the
permission set *GitHub Read* under the canonical ID `github-read`. That permission set contains the
scope `read` of GitHub.

The UUID of a resource is its **Stable Identifier**. Relationships between resources store UUIDs
only. A canonical ID is optional. It is an alias that the admin API accepts as input and reports
as the attribute `canonical_id`.

## Give a resource a readable name

The administrator registers a new agent and chooses the canonical ID `research-agent`. The request
also names the provider and the permission set by their canonical IDs, so the administrator copies
no UUID: `example:resource-identifiers.assign.create`

```http example="resource-identifiers.assign.create"
POST /api/agents/ HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "canonical_id": "research-agent",
  "client_id": "canonical-client-research-agent",
  "display_name": "Canonical Agent",
  "description": "Canonical resource acceptance test agent",
  "service_requirements": [
    {
      "service_id": "github-service",
      "requirement_type": "mandatory",
      "required_scopes": ["read"]
    }
  ],
  "permission_sets": [
    {
      "permission_set_id": "github-read",
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 201 Created

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": "research-agent",
  "client_id": "canonical-client-research-agent",
  "display_name": "Canonical Agent",
  "description": "Canonical resource acceptance test agent",
  "service_requirements": [
    {
      "service_id": "660e8400-e29b-41d4-a716-446655440002",
      "service_name": "GitHub",
      "requirement_type": "mandatory",
      "required_scopes": ["read"]
    }
  ],
  "permission_sets": [
    {
      "permission_set_id": "770e8400-e29b-41d4-a716-446655440001",
      "requirement_type": "mandatory"
    }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

The agent now has two identifiers. `id` is the UUID that the broker generated. `canonical_id` is
the name that the administrator chose. The response shows the two references as UUIDs, although
the request named them by canonical ID.

A canonical ID is never required. An agent that is created without one has `canonical_id` `null`,
receives no generated name, and is managed by UUID as before. `example:resource-identifiers.assign.uuid-only`

Two agents cannot hold the same canonical ID. A write that gives an agent the canonical ID of
another agent is refused with `409 Conflict`, and both agents stay as they were. `example:resource-identifiers.assign.unique`

### Keep, change, or remove the name

A full update with `PUT` treats `canonical_id` in three ways. When the body omits the member, the
agent keeps its canonical ID. A string replaces it. An explicit `null` removes it, and the agent
keeps its UUID: `example:resource-identifiers.assign.update`

```http example="resource-identifiers.assign.update"
PUT /api/agents/550e8400-e29b-41d4-a716-446655440000 HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "canonical_id": null,
  "display_name": "Mutable",
  "…": "…"
}

HTTP/1.1 200 OK

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": null,
  "display_name": "Mutable",
  "…": "…"
}
```

## Either identifier addresses the same resource

Wherever an admin path takes the identifier of an agent, a service, or a permission set, it takes
the UUID or the canonical ID. Both lead to the same resource. For the agent `path-agent`, the
administrator reads it by name:

```http example="resource-identifiers.address.either-identifier"
GET /api/agents/path-agent HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": "path-agent",
  "…": "…"
}
```

The same scenario reads the agent by UUID and receives the same `id`. It creates the agent's
client credentials by canonical ID, reads them by UUID, and deletes them by canonical ID. It then
reads, updates, and deletes a service and a permission set by their canonical IDs, and reads each
of them by UUID. `example:resource-identifiers.address.either-identifier`

## References accept either identifier

Three places in a definition point to another resource: the `service_id` of an agent's service
requirement, the `permission_set_id` of an agent's permission set, and the `service_id` of a
permission set's service scope. Each of them accepts the canonical ID of the target. `example:resource-identifiers.reference.canonical`
Each of them accepts the UUID of the target, with the same result. `example:resource-identifiers.reference.uuid`

A reference that matches no resource stops the whole write. Here the administrator mistypes the
provider's name: `example:resource-identifiers.reference.unresolved`

```http example="resource-identifiers.reference.unresolved"
POST /api/agents/ HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "missing-reference-client",
  "display_name": "Missing",
  "description": "Missing reference",
  "service_requirements": [
    {
      "service_id": "missing-service",
      "requirement_type": "mandatory",
      "required_scopes": ["read"]
    }
  ],
  "…": "…"
}

HTTP/1.1 400 Bad Request

{
  "error": "invalid service requirements",
  "message": "service_requirements[0]: invalid service_id: …"
}
```

Nothing is stored. The second reference in the request was valid, but the agent list is still
empty afterwards:

```http example="resource-identifiers.reference.unresolved"
GET /api/agents/ HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

[]
```

The same rule holds for a permission set whose service scope names an unknown service.

## Choose how references are shown

By default, every identifier in a response is a UUID. A resource's `id` is its UUID, and so is
each reference to another resource. The canonical ID appears only in the resource's own
`canonical_id`. `example:resource-identifiers.read.default` A resource without a canonical ID
shows `canonical_id` as `null`. `example:resource-identifiers.read.uuid-only` Scripts that store
and compare identifiers therefore always see the same form.

```http example="resource-identifiers.read.default"
GET /api/permission-sets/770e8400-e29b-41d4-a716-446655440001 HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

{
  "id": "770e8400-e29b-41d4-a716-446655440001",
  "canonical_id": "github-read",
  "name": "GitHub Read",
  "description": "Read GitHub",
  "service_scopes": [
    {
      "service_id": "660e8400-e29b-41d4-a716-446655440002",
      "scopes": ["read"],
      "requirement_type": "mandatory"
    }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

A person who reads the response prefers names. A client asks for them with the request header
`Prefer: reference-id=canonical`. The broker then shows each reference as the canonical ID of its
target and confirms this with `Preference-Applied`: `example:resource-identifiers.read.prefer-canonical`

```http example="resource-identifiers.read.prefer-canonical"
GET /api/permission-sets/770e8400-e29b-41d4-a716-446655440001 HTTP/1.1
X-Remote-User: admin@example.com
Prefer: reference-id=canonical

HTTP/1.1 200 OK
Preference-Applied: reference-id=canonical
Vary: Prefer

{
  "id": "770e8400-e29b-41d4-a716-446655440001",
  "canonical_id": "github-read",
  "name": "GitHub Read",
  "description": "Read GitHub",
  "service_scopes": [
    {
      "service_id": "github-service",
      "scopes": ["read"],
      "requirement_type": "mandatory"
    }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

Only the reference changed. The permission set's own `id` is still its UUID. The same request
for an agent shows `github-service` and `github-read` in its service requirement and its
permission set. The preference also applies to a list of agents. `example:resource-identifiers.read.prefer-canonical-list`
A name that the broker shows this way is valid input for the next write.

## A canonical ID is not an External ID

An agent can carry two optional identifiers besides its UUID, and they have different owners. The
administrator chooses the canonical ID as a short name for paths and references. The **External
ID** belongs to an outside governance system and has its own lookup and reconciliation call.
[Agent registration](agent-registration.md) describes the External ID.

## Not yet on this page

No acceptance scenario covers the following parts, so this page does not state them as fact:

- The format of a canonical ID: 1 to 128 characters from letters, digits, `.`, `_`, and `-`,
  case-sensitive, and never in the form of a UUID.
- The refusal of a duplicate canonical ID for a service or a permission set, and through the
  ordinary create and update calls of an agent. The scenario cited above proves it for
  reconciliation only.
- That an agent, a service, and a permission set can share one canonical ID, because uniqueness
  holds within each resource type.
- The answer to a path identifier that matches no resource.
- A response to an unknown `Prefer` value, and a reference whose target has no canonical ID while
  the preference is applied.
- The change and removal of a canonical ID for a service or a permission set.

Canonical IDs also appear outside the admin API: in the audience of an impersonation request, in
the `service_id` attribute of session metrics, and in the binding of credential files to a
service. Those belong to other pages.

## Sources

The behavior on this page was decided in `specs/036-canonical-resource-ids/spec.md`. The
difference from the External ID comes from `specs/054-agent-external-id-filter/spec.md` and
`specs/055-agent-external-id-upsert/spec.md`. The vocabulary is in the `ARCHITECTURE.md` glossary
(Canonical ID, Stable Identifier, External ID). The contract is `api/admin/openapi.yaml`. The
scenarios are in `tests/e2e/canonical_resource_ids_test.go`,
`tests/e2e/agent_external_id_filter_test.go`, and `tests/e2e/agent_external_id_upsert_test.go`.
