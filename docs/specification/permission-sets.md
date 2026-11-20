---
title: "Permission sets"
description: Living specification of how an administrator bundles provider scopes into named permission sets and declares which ones an agent needs. Every example on this page is checked against the acceptance suite.
---

# Permission sets

A provider scope such as `repo:read` means little to most users. A **PermissionSet** gives a group
of scopes a name and a description that a user can understand. This page describes how an
administrator defines permission sets, attaches them to an **Agent**, and removes them, in the
order an administrator meets the behavior.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

The administrator `admin@example.com` works through the admin API. The broker already knows two
third-party services:

| Service | UUID | Scopes |
|---|---|---|
| *GitHub* | `880e8400-e29b-41d4-a716-446655440010` | `repo:read`, `user:email` |
| *Google* | `880e8400-e29b-41d4-a716-446655440020` | `calendar.read` |

The administrator wants users such as Alice to see "GitHub Read Access" on the consent screen, not
a list of scope strings. The administrator also wants to say which permission sets the
*Research Assistant* and other agents cannot work without.

Each scenario behind this page creates its own data. For that reason, the names of permission sets
and agents differ between the examples.

## A permission set names a group of scopes

A permission set has a name, a description, and a list of **ServiceScope** entries. Each entry
pairs one service with scopes of that service. The administrator creates the set with one call.
The broker returns the new UUID and says where the set lives: `example:permission-sets.define.create`

```http example="permission-sets.define.create"
POST /api/permission-sets HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "name": "GitHub Read Access",
  "description": "Read repository contents and user profile from GitHub",
  "service_scopes": [
    {
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
      "scopes": ["repo:read", "user:email"]
    }
  ]
}

HTTP/1.1 201 Created
Location: /api/permission-sets/770e8400-e29b-41d4-a716-446655440001
Content-Type: application/json

{
  "id": "770e8400-e29b-41d4-a716-446655440001",
  "canonical_id": null,
  "name": "GitHub Read Access",
  "description": "Read repository contents and user profile from GitHub",
  "service_scopes": [
    {
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
      "scopes": ["repo:read", "user:email"],
      "requirement_type": "optional"
    }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

Two details are worth noticing:

- `Location` and `id` carry the same new UUID. Agents refer to the permission set by this UUID.
- The request gave no `requirement_type` for the service scope. The broker stored `optional`.

A read by UUID returns the same representation. `example:permission-sets.define.read`

### A service scope must name a known service

A permission set that names a service the broker does not know is refused, and nothing is stored: `example:permission-sets.references.unresolved`

```http example="permission-sets.references.unresolved"
POST /api/permission-sets/ HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "name": "Unresolved Service Scope",
  "description": "Must not persist",
  "service_scopes": [
    {
      "service_id": "missing-service",
      "scopes": ["read"],
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "service_scope[0]: invalid service_id: …"
}
```

### An update replaces the definition

The administrator changes a permission set with a `PUT` to its address. The request carries the
complete definition: name, description, and service scopes. The response shows the new description
and the new scope list under the same UUID. `example:permission-sets.define.update`

### Finding the permission sets of one service

Before the administrator changes the scopes of a service, the administrator wants to know which
permission sets use that service. Suppose two sets exist: "GitHub PS" covers GitHub and "Google PS"
covers Google. A list with the query `service_id=880e8400-e29b-41d4-a716-446655440010` returns
only "GitHub PS". `example:permission-sets.define.filter-by-service`

## An agent declares the permission sets it needs

An agent carries a `permission_sets` list. Each entry names one permission set and says whether it
is `mandatory` or `optional` for this agent. The same permission set can be mandatory for one agent
and optional for another, because the requirement type belongs to the agent's entry.

Suppose two permission sets exist: one that covers GitHub
(`770e8400-e29b-41d4-a716-446655440001`) and one that covers Google
(`770e8400-e29b-41d4-a716-446655440002`). The administrator registers an agent that needs the
first and can use the second. The broker stores the list and returns it in the order of the
declaration: `example:permission-sets.agent.declaration-order`

```http example="permission-sets.agent.declaration-order"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "agent-ordered-ps",
  "display_name": "Ordered PS Agent",
  "description": "Agent with ordered PSets",
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440001", "requirement_type": "mandatory" },
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440002", "requirement_type": "optional" }
  ]
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": null,
  "client_id": "agent-ordered-ps",
  "display_name": "Ordered PS Agent",
  "description": "Agent with ordered PSets",
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440001", "requirement_type": "mandatory" },
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440002", "requirement_type": "optional" }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-30T09:00:00Z"
}
```

The consent screen shows the permission sets to the user in this order; the "Consent and
delegation" page describes that screen.

### Every agent declares at least one permission set

An agent without a permission set would have nothing to ask the user for. A definition that
declares none is refused: `example:permission-sets.agent.none-declared`

```http example="permission-sets.agent.none-declared"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "test-agent-6",
  "display_name": "Test Agent 6",
  "description": "Test agent",
  "service_requirements": [
    {
      "service_id": "00000000-0000-0000-0000-000000000000",
      "requirement_type": "mandatory",
      "required_scopes": ["some:scope"]
    }
  ]
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "at least one permission set entry is required"
}
```

### A declared permission set must exist

An agent cannot refer to a permission set that the broker does not know. The answer names the
missing identifier: `example:permission-sets.agent.unknown-permission-set`

```http example="permission-sets.agent.unknown-permission-set"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "agent-invalid-ps",
  "display_name": "Agent With Invalid PS",
  "description": "Agent referencing non-existent PS",
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440009", "requirement_type": "mandatory" }
  ]
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "validation failed: missing permission set IDs: [770e8400-e29b-41d4-a716-446655440009]"
}
```

## Permission sets and service requirements

An agent has a second list: its **ServiceRequirement** entries. A service requirement names one
third-party service, says whether the agent needs it (`mandatory`) or can use it (`optional`), and
lists the scopes of that service that this agent may use.

The two lists answer different questions. Permission sets are shared by all agents and say what a
user can grant. Service requirements belong to one agent and say which services that agent works
with. Neither list refers to the other, and the agent representation returns both: `example:permission-sets.agent.independent-fields`

```http example="permission-sets.agent.independent-fields"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "agent-both-fields",
  "display_name": "Agent With Both Fields",
  "description": "Agent with both service_requirements and permission_sets",
  "service_requirements": [
    {
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
      "requirement_type": "optional",
      "required_scopes": ["repo:read"]
    }
  ],
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440001", "requirement_type": "mandatory" }
  ]
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "service_requirements": [
    {
      "service_id": "880e8400-e29b-41d4-a716-446655440010",
      "service_name": "GitHub",
      "requirement_type": "optional",
      "required_scopes": ["repo:read"]
    }
  ],
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440001", "requirement_type": "mandatory" }
  ],
  "…": "…"
}
```

The request named the service by UUID only. The response adds `service_name`, which the broker
takes from the display name of the service. A later read of the agent shows the same name. `example:permission-sets.service-requirements.read`

An update of the agent replaces the whole list of service requirements with the list in the
request. `example:permission-sets.service-requirements.replace`

How the scopes of a service requirement limit the scopes of an issued token is described on the
"Token exchange" page.

### Every required service must be covered by a permission set

A user grants permission sets, not services. If an agent requires a service that none of its
permission sets covers, no user could ever grant access to that service. The broker therefore
refuses such an agent.

In this example the agent's only permission set covers GitHub
(`880e8400-e29b-41d4-a716-446655440010`), but the agent requires Google
(`880e8400-e29b-41d4-a716-446655440020`). The answer lists the uncovered service: `example:permission-sets.coverage.uncovered-service`

```http example="permission-sets.coverage.uncovered-service"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "client_id": "agent-uncovered-sr",
  "display_name": "Agent with Uncovered SR",
  "description": "Agent referencing service not covered by any PS",
  "service_requirements": [
    {
      "service_id": "880e8400-e29b-41d4-a716-446655440020",
      "requirement_type": "mandatory",
      "required_scopes": ["calendar.read"]
    }
  ],
  "permission_sets": [
    { "permission_set_id": "770e8400-e29b-41d4-a716-446655440001", "requirement_type": "mandatory" }
  ]
}

HTTP/1.1 400 Bad Request

{
  "error": "validation failed",
  "message": "service_requirements reference services not covered by any permission set: [880e8400-e29b-41d4-a716-446655440020]"
}
```

The rule works in one direction only. The first agent on this page declares permission sets and
no service requirement at all, and the broker accepts it.

## Deleting a permission set

A permission set that no agent declares can be deleted. The broker answers `204 No Content`, and a
later read by its UUID answers `404 Not Found`. `example:permission-sets.delete.unused`

A permission set that an agent declares cannot be deleted. Otherwise the agent would point to a
permission set that no longer exists. Here one agent declares the set as mandatory, and the broker
says how many agents stand in the way: `example:permission-sets.delete.referenced-by-agent`

```http example="permission-sets.delete.referenced-by-agent"
DELETE /api/permission-sets/770e8400-e29b-41d4-a716-446655440001 HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 409 Conflict

{
  "error": "conflict",
  "message": "cannot delete permission set: 1 agent(s) reference it"
}
```

To delete the set, the administrator first removes it from the `permission_sets` list of each
agent that declares it.

## Not yet on this page

This page covers the admin API only. These parts of the capability are specified but not shown
here:

- No acceptance scenario backs these rules, so the page does not state them as fact: a permission
  set name is unique; a permission set has at least one service scope; a service requirement must
  name a known service and only scopes of that service, and a service at most once; a third-party
  service that a permission set covers cannot be deleted; a permission set that a user grant
  refers to cannot be deleted; the coverage rule and the other agent rules also hold when an agent
  is updated.
- The `requirement_type` of a service scope and the `require_all_scopes` flag of a service
  requirement. Their effect shows on the consent screen and in token exchange.
- Canonical IDs such as `github-read` as an alternative to UUIDs. See the "Resource identifiers"
  page.
- The consent screen and what the user grants. See the "Consent and delegation" page.
- How a token exchange resolves granted permission sets to scopes. See the "Token exchange" page.

## Sources

The behavior on this page was decided in `specs/019-permission-sets/spec.md` and
`specs/011-agent-permission-requirements/spec.md`. The contract is `api/admin/openapi.yaml`.
The scenarios are in `tests/e2e/permission_sets_test.go`,
`tests/e2e/agent_permission_requirements_test.go`, and
`tests/e2e/canonical_resource_ids_test.go`.
