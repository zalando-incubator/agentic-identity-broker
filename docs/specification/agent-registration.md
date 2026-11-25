---
title: "Agent registration"
description: Living specification of how an administrator or governance system registers an agent and keeps its definition current. Every example on this page is checked against the acceptance suite.
---

# Agent registration

An **Agent** has to be known to the broker before a user can delegate anything to it. This page
describes how an agent becomes known and how its definition is kept current, in the order a
governance team meets the behavior.

:::note How to read this page
This is a living specification. A badge such as "tested" after a sentence links to the acceptance
scenario that proves it. Each HTTP exchange shown here is compared with what that scenario really
sent and received. `just living-docs` runs the comparison and fails when the page and the broker
disagree. UUIDs and timestamps are synthetic. The check compares timestamps by shape and UUIDs by
which of them are equal.
:::

## The cast

A company keeps its approved AI agents in an **AI registry**. The registry is the source of truth.
It knows each agent under its own identifier, such as `urn:example:ai-registry:agent:7f3a9c` for
the *Research Assistant*. A sync job copies registry entries to the broker through the admin API
as `admin@example.com`.

The broker gives every agent a UUID. Grants, sessions, and tokens refer to that UUID, so it must
never change. The registry identifier is stored next to it as the agent's **External ID**.

The broker already has a permission set that administrators call `github-read`.

## One call that creates or updates

The sync job does not ask first whether the broker knows an agent. It sends the definition it
wants under the registry identifier. This is **Agent Reconciliation**. When no agent holds the
External ID, the broker creates one and says where it lives: `example:agent-registration.reconcile.create`

```http example="agent-registration.reconcile.create"
PUT /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
  "permission_sets": [
    {
      "permission_set_id": "github-read",
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 201 Created
Location: /api/agents/550e8400-e29b-41d4-a716-446655440000
Content-Type: application/json

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": null,
  "external_id": "urn:example:ai-registry:agent:7f3a9c",
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
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

Three details are worth noticing:

- The body does not repeat the External ID. The query selector supplies it.
- `Location` and `id` carry the same new UUID. That UUID is the agent's identity from now on.
- The request names the permission set as `github-read`. The response shows its UUID.

A lookup by the same identifier now returns exactly this agent:

```http example="agent-registration.reconcile.create"
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "canonical_id": null,
    "external_id": "urn:example:ai-registry:agent:7f3a9c",
    "display_name": "Research Assistant",
    "description": "Helps with academic research",
    "permission_sets": [
      {
        "permission_set_id": "770e8400-e29b-41d4-a716-446655440001",
        "requirement_type": "mandatory"
      }
    ],
    "created_at": "2026-10-30T09:00:00Z",
    "updated_at": "2026-10-30T09:00:00Z"
  }
]
```

### Sending the same definition again changes nothing

Sync jobs retry. If the response to the first call is lost, the job sends the same request again.
The broker answers `200 OK` with the identical representation, including `updated_at`, and writes
nothing. `example:agent-registration.reconcile.replay` The job can therefore run on a schedule
without making every agent look recently changed.

When the registry entry does change, the same call updates the same agent. Its `id` and
`created_at` stay as they were. `example:agent-registration.reconcile.update`

If two sync workers send the first call for a new identifier at the same moment, exactly one
agent is created. One worker receives `201 Created` and the other `200 OK`, both with the same
UUID. `example:agent-registration.reconcile.concurrent-first-call`

### The definition replaces what the registry manages, not who the agent is

Suppose the Research Assistant has existed for a while. An administrator gave it the canonical ID
`research-assistant` and the client ID `research-upstream`. It also has a governance URL,
a documentation URL, an interface URL, an allowed scope, and a service requirement. Alice has
granted it access.

The registry entry holds none of those extras, so the sync job sends only the short definition: `example:agent-registration.reconcile.full-replacement`

```http example="agent-registration.reconcile.full-replacement"
PUT /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
  "permission_sets": [
    {
      "permission_set_id": "github-read",
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 200 OK

{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "canonical_id": "research-assistant",
  "client_id": "research-upstream",
  "external_id": "urn:example:ai-registry:agent:7f3a9c",
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
  "permission_sets": [
    {
      "permission_set_id": "770e8400-e29b-41d4-a716-446655440001",
      "requirement_type": "mandatory"
    }
  ],
  "created_at": "2026-10-30T09:00:00Z",
  "updated_at": "2026-10-31T14:00:00Z"
}
```

The identities survive: UUID, canonical ID, client ID, and External ID. The metadata URLs, the
scope, and the service requirement are gone, because the definition no longer lists them. Alice's
grant is untouched.

## An External ID belongs to one agent

Two agents cannot hold the same External ID. A second agent that claims the Research Assistant's
identifier through the ordinary create call is refused, and nothing is stored: `example:agent-registration.external-id.unique`

```http example="agent-registration.external-id.unique"
POST /api/agents HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
  "external_id": "urn:example:ai-registry:agent:7f3a9c",
  "permission_sets": [
    {
      "permission_set_id": "github-read",
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 409 Conflict

{
  "error": "conflict",
  "message": "external_id is already held by another agent"
}
```

The selector and the body cannot disagree either. A reconciliation call that addresses one
identifier and carries another in its body is rejected before anything changes: `example:agent-registration.reconcile.selector-mismatch`

```http example="agent-registration.reconcile.selector-mismatch"
PUT /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
X-Remote-User: admin@example.com
Content-Type: application/json

{
  "display_name": "Research Assistant",
  "description": "Helps with academic research",
  "external_id": "urn:example:ai-registry:agent:1b44e0",
  "permission_sets": [
    {
      "permission_set_id": "github-read",
      "requirement_type": "mandatory"
    }
  ]
}

HTTP/1.1 400 Bad Request

{
  "error": "invalid external_id",
  "message": "body external_id must equal the selector"
}
```

## External IDs are matched literally

The broker does not interpret an External ID. It compares the exact characters. The same
identifier in upper case finds nothing, and the answer is an empty list, not an error: `example:agent-registration.external-id.lookup`

```http example="agent-registration.external-id.lookup"
GET /api/agents?external_id=URN%3AEXAMPLE%3AAI-REGISTRY%3AAGENT%3A7F3A9C HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

[]
```

An agent's canonical ID is not an External ID. Looking it up as one also finds nothing:

```http example="agent-registration.external-id.lookup"
GET /api/agents?external_id=research-assistant HTTP/1.1
X-Remote-User: admin@example.com

HTTP/1.1 200 OK

[]
```

## Not yet on this page

This page is a pilot and covers reconciliation and External IDs only. Registration by UUID, the
upgrade path for legacy duplicate External IDs, and client registration are specified but not
yet described here.

## Sources

The behavior on this page was decided in `specs/054-agent-external-id-filter/spec.md`,
`specs/055-agent-external-id-upsert/spec.md`, and
`adrs/047-unique-agent-external-id-reconcile.md`. The contract is `api/admin/openapi.yaml`.
The scenarios are in `tests/e2e/agent_external_id_upsert_test.go`.
