# Feature Specification: Agent Lookup by External ID

**Feature Branch**: `054-agent-external-id-filter`

**Created**: 2026-10-26

**Status**: Draft

**Input**: User description: "Search agents by external ID: extend the admin API so an administrator can look up agents by `external_id`, for example a governance URN that `canonical_id` cannot hold. Keep `external_id` non-unique."

## Clarifications

### Session 2026-10-26

- Q: Can an agent's `canonical_id` hold a URN? → A: No. A canonical ID uses only letters, digits, `.`, `_`, and `-`, so the `:` in a URN is invalid. An identifier from an external governance system belongs in `external_id`, which this feature makes searchable.
- Q: Should `external_id` become unique, or should a lookup return zero or more agents? → A: Keep `external_id` non-unique. A lookup filters the agent collection and returns zero, one, or several agents.
- Q: Is the API contract approved for implementation? → A: Yes. The requesting stakeholder approved the API changes on 2026-10-26, before implementation began (API-005).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Find the agents registered under an external ID (Priority: P1)

An administrator, or automation that acts for one, holds an identifier from an external governance system, such as `urn:example:ai-registry:agent:7f3a9c`. The administrator asks the broker for the agents registered under that identifier and receives exactly those agents in one request. Today the admin API addresses a single agent only by UUID or canonical ID, so the caller must download every agent and filter the list itself.

**Why this priority**: This is the capability the feature exists for. Governance systems reconcile their records with broker agents through their own identifiers, and URN-shaped identifiers cannot be canonical IDs.

**Independent Test**: Register agents with a unique external ID, a shared external ID, and no external ID. Look up each value and verify that each response contains exactly the matching agents.

**Acceptance Scenarios**:

1. **Given** agent *Data Analyst* has external ID `urn:example:ai-registry:agent:1b44e0` and every other agent has a different external ID or none, **When** an administrator lists agents filtered by `urn:example:ai-registry:agent:1b44e0`, **Then** the broker returns `200 OK` with an array that contains only *Data Analyst*.
2. **Given** no agent has external ID `urn:example:ai-registry:agent:9d0e11`, **When** an administrator lists agents filtered by that value, **Then** the broker returns `200 OK` with an empty array.
3. **Given** agents *Research Assistant* and *Research Assistant (EU)* both have external ID `urn:example:ai-registry:agent:7f3a9c` and *Data Analyst* has another external ID, **When** an administrator lists agents filtered by `urn:example:ai-registry:agent:7f3a9c`, **Then** the broker returns `200 OK` with both Research Assistant agents, in any order, and without *Data Analyst*.

---

### User Story 2 - Unusable lookups fail instead of returning every agent (Priority: P1)

A lookup that the broker cannot read as exactly one valid external ID fails with an error. A job with a blank variable, a duplicated parameter, or a faulty encoder never receives the full agent list, so it cannot bind a governance record to the wrong agent.

**Why this priority**: Without these rules, a failed lookup looks like a successful one: if the broker ignored an unusable `external_id`, it would return every agent. For an identity lookup, that fallback fails open (Constitution Principle I). User Story 1 is safe to release only together with this story.

**Independent Test**: Send each unusable form of the filter. Verify that each response is `400 Bad Request` with error `invalid external_id` and no agent data.

**Acceptance Scenarios**:

1. **Given** registered agents exist, **When** an administrator lists agents with `external_id` present but empty, **Then** the broker returns `400 Bad Request` with error `invalid external_id`.
2. **Given** registered agents exist, **When** an administrator supplies `external_id` twice with the same value, **Then** the broker returns `400 Bad Request` with error `invalid external_id`.
3. **Given** registered agents exist, **When** the decoded filter value has 256 characters, is not valid UTF-8 text (`%FF`), or contains a NUL character (`a%00b`), **Then** the broker returns `400 Bad Request` with error `invalid external_id`.
4. **Given** registered agents exist, **When** the filter value cannot be decoded because it contains malformed percent-encoding (`urn%3Aexample%3`) or an unencoded `;` (`urn:example:x;rev=2`), **Then** the broker returns `400 Bad Request` with error `invalid external_id` instead of ignoring the parameter.

---

### User Story 3 - Existing list callers keep their behavior (Priority: P2)

Callers of the agent list that do not use the filter see no change. Callers that use the filter keep the representation options of the list, including canonical nested references.

**Why this priority**: The list operation already has callers. The filter must not change their results, and filtered results must work with the canonical-reference presentation from feature 036.

**Independent Test**: List agents without the filter. Then list agents with the filter and `Prefer: reference-id=canonical`. Compare both responses with the existing list contract.

**Acceptance Scenarios**:

1. **Given** agents with and without external IDs exist, **When** an administrator lists agents without an `external_id` parameter, **Then** the broker returns `200 OK` with every registered agent.
2. **Given** agent *Release Notes Writer* has external ID `urn:example:ai-registry:agent:9d0e11` and requires the permission set with canonical ID `github-read`, **When** an administrator lists agents filtered by that value and sends `Prefer: reference-id=canonical`, **Then** the broker returns that agent with `permission_set_id` `github-read` and the header `Preference-Applied: reference-id=canonical`.

### Edge Cases

- **Matching scope**: The filter compares only the external ID. `?external_id=research-assistant` does not return the agent whose canonical ID is `research-assistant`. A UUID-shaped value is compared as an external ID and does not address an agent by `id`. Agents without an external ID never match.
- **No normalization**: The broker does not trim, case-fold, or Unicode-normalize the value. `URN:EXAMPLE:AI-REGISTRY:AGENT:7F3A9C` does not match `urn:example:ai-registry:agent:7f3a9c`, and a value with a leading space matches only an external ID with the same leading space.
- **Literal characters**: `*`, `%`, `_`, and `?` are ordinary characters. `?external_id=urn%3Aexample%3A*` matches only an agent whose external ID is exactly `urn:example:*`.
- **Encoding**: Clients percent-encode the whole value; `:` and `/` may stay unencoded. External ID `urn:example:ai-registry:r&d:agent+7;rev=2` is sent as `urn%3Aexample%3Aai-registry%3Ar%26d%3Aagent%2B7%3Brev%3D2`. An unencoded `+` becomes a space, so the lookup searches for a different value and usually returns `[]`. An unencoded `&` ends the value, so the broker searches for the part before it. An unencoded `;` in the value is rejected, because some proxies treat `;` as a parameter separator. A `#` starts a URI fragment, which the client does not send.
- **Length counts characters**: A 255-character value is valid even when its non-ASCII characters need more than 255 bytes. A 256-character value is rejected.
- **Other parameters**: The broker ignores unrecognized query parameters, as other admin list operations do, and it keeps today's handling of malformed parts outside `external_id`. A misspelled filter such as `?externalId=...` therefore returns every agent. Callers that bind identities check `external_id` in each returned agent.
- **Empty stored value**: The write path accepts an empty `external_id` string today. The filter cannot find such an agent, because an empty filter value is rejected.
- **Changes are visible on the next lookup**: After an agent is created, its external ID changes, or the agent is deleted, the next lookup reflects that change.
- **Many matches**: The response contains all matching agents. Like the unfiltered list, it has no pagination.

## Concrete Example Journeys *(mandatory)*

### Journey 1 — The AI-registry sync job maps registry entries to broker agents (User Story 1, Scenarios 2–3; User Story 3, Scenario 2)

**Cast**: The admin API is published at `https://broker-admin.example.com` (`server.admin.public_url`) behind an admin proxy. The proxy authenticates the AI-registry sync job and forwards its principal as `X-Remote-User: ai-registry-sync@example.com` (`server.admin.authentication.preauth.principal_header_name: X-Remote-User`). Permission set *GitHub Read Access* has `id` `a1b2c3d4-e5f6-7890-abcd-ef1234567890` and `canonical_id` `github-read`. Three agents require it as `mandatory` and have no `client_id`:

| Agent | `id` | `canonical_id` | `external_id` | Created |
|---|---|---|---|---|
| Research Assistant | `550e8400-e29b-41d4-a716-446655440000` | `research-assistant` | `urn:example:ai-registry:agent:7f3a9c` | 2026-09-01T10:00:00Z |
| Research Assistant (EU) | `6a1f4c2e-93b7-4d1a-8c55-0e7d2b9f4a10` | `research-assistant-eu` | `urn:example:ai-registry:agent:7f3a9c` | 2026-09-14T08:30:00Z |
| Data Analyst | `660e8400-e29b-41d4-a716-446655440001` | `data-analyst` | `urn:example:ai-registry:agent:1b44e0` | 2026-09-20T12:00:00Z |

The registry entry `urn:example:ai-registry:agent:7f3a9c` describes the research assistant, which runs as two regional deployments. The entry `urn:example:ai-registry:agent:9d0e11` describes *Release Notes Writer*, which the broker does not know yet. All times are 2026-10-26 UTC, and all IDs are synthetic. `urn:example:` is the documentation namespace from RFC 6963.

**Step 1 — 08:00:00: the job looks up registry entry `7f3a9c`**

```http
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com

HTTP/1.1 200 OK
Content-Type: application/json
Vary: Prefer

[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "canonical_id": "research-assistant",
    "external_id": "urn:example:ai-registry:agent:7f3a9c",
    "display_name": "Research Assistant",
    "description": "AI assistant that helps with academic research",
    "permission_sets": [
      { "permission_set_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "requirement_type": "mandatory" }
    ],
    "created_at": "2026-09-01T10:00:00Z",
    "updated_at": "2026-09-01T10:00:00Z"
  },
  {
    "id": "6a1f4c2e-93b7-4d1a-8c55-0e7d2b9f4a10",
    "canonical_id": "research-assistant-eu",
    "external_id": "urn:example:ai-registry:agent:7f3a9c",
    "display_name": "Research Assistant (EU)",
    "description": "EU deployment of the research assistant",
    "permission_sets": [
      { "permission_set_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "requirement_type": "mandatory" }
    ],
    "created_at": "2026-09-14T08:30:00Z",
    "updated_at": "2026-09-14T08:30:00Z"
  }
]
```

The job percent-encodes the whole value; it may also send `:` unencoded. *Data Analyst* is not in the response, and the order of the two agents is not guaranteed.
Variant (FR-002): the value `URN%3AEXAMPLE%3AAI-REGISTRY%3AAGENT%3A7F3A9C` returns `200 OK` with `[]`, because matching is case-sensitive.

**Step 2 — 08:00:01: the job looks up registry entry `9d0e11`**

```http
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A9d0e11 HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com

HTTP/1.1 200 OK
Content-Type: application/json
Vary: Prefer

[]
```

The empty array tells the job that no broker agent carries this external ID.

**Step 3 — 08:00:02: the job registers the missing agent (existing operation, unchanged)**

```http
POST /api/agents HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com
Content-Type: application/json

{
  "canonical_id": "release-notes-writer",
  "external_id": "urn:example:ai-registry:agent:9d0e11",
  "display_name": "Release Notes Writer",
  "description": "Drafts release notes from merged pull requests",
  "permission_sets": [
    { "permission_set_id": "github-read", "requirement_type": "mandatory" }
  ]
}

HTTP/1.1 201 Created
Content-Type: application/json

{
  "id": "7b9e2d4c-1f3a-4e8b-9c6d-5a2f1e0d3c4b",
  "canonical_id": "release-notes-writer",
  "external_id": "urn:example:ai-registry:agent:9d0e11",
  "display_name": "Release Notes Writer",
  "description": "Drafts release notes from merged pull requests",
  "permission_sets": [
    { "permission_set_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890", "requirement_type": "mandatory" }
  ],
  "created_at": "2026-10-26T08:00:02Z",
  "updated_at": "2026-10-26T08:00:02Z"
}
```

**Step 4 — 08:00:03: the job reads the new agent back with canonical references**

```http
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A9d0e11 HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com
Prefer: reference-id=canonical

HTTP/1.1 200 OK
Content-Type: application/json
Preference-Applied: reference-id=canonical
Vary: Prefer

[
  {
    "id": "7b9e2d4c-1f3a-4e8b-9c6d-5a2f1e0d3c4b",
    "canonical_id": "release-notes-writer",
    "external_id": "urn:example:ai-registry:agent:9d0e11",
    "display_name": "Release Notes Writer",
    "description": "Drafts release notes from merged pull requests",
    "permission_sets": [
      { "permission_set_id": "github-read", "requirement_type": "mandatory" }
    ],
    "created_at": "2026-10-26T08:00:02Z",
    "updated_at": "2026-10-26T08:00:02Z"
  }
]
```

The agent's own `id` stays a UUID. Only the nested permission-set reference uses its canonical ID.

**Observable result**: The job maps `urn:example:ai-registry:agent:7f3a9c` to `research-assistant` and `research-assistant-eu`, and it maps `urn:example:ai-registry:agent:9d0e11` to `release-notes-writer`. No response contained *Data Analyst*.

### Journey 2 — A faulty job run fails visibly instead of receiving every agent (User Story 2, Scenarios 1, 2, and 4)

**Cast**: The broker, admin proxy, sync job, and four agents from Journey 1, including *Release Notes Writer* from its step 3. At 14:00 on the same day, a faulty change to the job leaves one registry identifier blank, sends another identifier twice, and cuts a third identifier in the middle of a percent-escape.

**Step 1 — 14:00:00: blank identifier**

```http
GET /api/agents?external_id= HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid external_id","message":"external_id must be 1 to 255 characters"}
```

Before this feature, the broker ignored the parameter and returned all four agents.

**Step 2 — 14:00:01: identifier sent twice**

```http
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c&external_id=urn%3Aexample%3Aai-registry%3Aagent%3A7f3a9c HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid external_id","message":"external_id must appear at most once"}
```

The broker rejects the request although both values are the same.

**Step 3 — 14:00:02: identifier cut inside a percent-escape**

```http
GET /api/agents?external_id=urn%3Aexample%3Aai-registry%3Aagent%3 HTTP/1.1
Host: broker-admin.example.com
X-Remote-User: ai-registry-sync@example.com

HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid external_id","message":"external_id contains malformed percent-encoding"}
```

`%3` is not a complete escape. The broker rejects the request instead of ignoring the parameter and returning every agent.

**Observable result**: The run stops with three `400 Bad Request` responses. No response contains agent data, so the job binds no registry entry to an agent. The `message` texts are examples; the status code and the `error` value are the contract.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The admin operation that lists agents (`GET /api/agents`) MUST accept an optional `external_id` query parameter.
- **FR-002**: When the request carries one valid `external_id` value, the response MUST contain every agent whose external ID equals that value and no other agent. Equality is exact: case-sensitive, with no trimming, Unicode normalization, prefix matching, or wildcard interpretation. The filter MUST NOT compare `id`, `canonical_id`, `client_id`, or any other attribute.
- **FR-003**: When no agent matches, the broker MUST return `200 OK` with an empty array.
- **FR-004**: A filtered response MUST use the same agent representation, ordering rule (no guaranteed order), and headers as the unfiltered list. This includes `Vary: Prefer` and the `Prefer: reference-id=canonical` handling from feature 036.
- **FR-005**: When a request has no `external_id` parameter, the operation MUST behave as it does before this feature and return every registered agent.
- **FR-006**: A valid `external_id` value MUST be 1 to 255 Unicode code points long (not bytes), MUST be valid UTF-8 text, and MUST NOT contain a NUL character. For any other value, the broker MUST return `400 Bad Request`.
- **FR-007**: When `external_id` appears more than once in the query, the broker MUST return `400 Bad Request`, even when the values are the same.
- **FR-008**: When the value of `external_id` cannot be decoded, because it contains malformed percent-encoding or an unencoded `;`, the broker MUST return `400 Bad Request` instead of ignoring the parameter. Malformed parts of the query outside `external_id` keep today's handling.
- **FR-009**: Each `400 Bad Request` response under FR-006 to FR-008 MUST use the admin error object with the `error` value `invalid external_id` and a `message` that states which rule failed. It MUST NOT contain agent data.
- **FR-010**: Every supported storage backend MUST return the same agents for the same filter value.
- **FR-011**: This feature MUST NOT change how agents are created, updated, or stored. `external_id` stays optional and non-unique, and its write validation stays unchanged.

### API Requirements

- **API-001**: Before implementation, `api/admin/openapi.yaml` MUST document the `external_id` query parameter of `GET /api/agents`: type string, `minLength: 1`, `maxLength: 255`, exact case-sensitive match, zero or more results because the value is not unique, the need to percent-encode reserved characters, a URN example, and a `400` response with `invalid external_id` examples. The summary and description MUST describe a filterable list instead of "List all agents".
- **API-002**: The parameter name MUST be the attribute name `external_id` (snake_case), and the filter MUST apply to the existing collection resource instead of a new lookup resource (Zalando RESTful API Guidelines).
- **API-003**: The change MUST be additive: requests without `external_id` keep today's behavior, including the handling of other query parameters.
- **API-004**: `docs/guides/manage-agents-and-services.md` MUST show a lookup by external ID with a percent-encoded URN and MUST explain that a lookup can return zero, one, or several agents.
- **API-005**: The stakeholder MUST confirm this contract before implementation starts (Constitution Principle X).

### Security Requirements *(mandatory for security-critical features)*

- **SR-001**: The filter MUST be available only on the admin server, behind the authentication and host checks that already protect `GET /api/agents`. It adds no access path.
- **SR-002**: The broker MUST fail closed. A request that FR-006 to FR-008 reject MUST NOT fall back to the unfiltered list or return partial results.
- **SR-003**: The broker MUST use the filter value only as data in an equality comparison, never as query syntax or a pattern. No character in the value can widen the match, so a caller cannot enumerate agents with patterns.

### Key Entities

- **Agent** (existing): A registered AI agent. It may carry one optional external ID. This feature does not change what is stored.
- **External ID** (existing attribute, new lookup key): An optional identifier that an external governance system assigns to an agent, such as a URN. It has at most 255 characters and is not unique: several agents, such as regional deployments of one governed agent, can share it. Unlike `canonical_id`, it is not limited to URL-safe characters, so it can hold a URN.
- **External ID Filter**: An optional criterion on the agent list. It selects the agents whose external ID equals the given value exactly.

*Add **External ID** to the `ARCHITECTURE.md` glossary (section 12, Constitution Principle V).*

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An administrator who knows an external ID gets every agent registered under it with one request, and the response contains no agent registered under another external ID.
- **SC-002**: 100% of lookups return exactly the matching agents, with none missing and none extra, for zero, one, and several matches.
- **SC-003**: 100% of empty, repeated, invalid, and undecodable `external_id` values are rejected, and 0 of them return agent data.
- **SC-004**: 100% of list requests without the filter return the same agents and representation as before this feature.

## Assumptions

- The callers are administrators and automation behind the admin proxy. End users and agents do not call this operation.
- External IDs are opaque, case-sensitive strings that the governance system assigns, so exact matching is sufficient.
- Rejecting empty and repeated values intentionally differs from the `service_id` filter of `GET /api/permission-sets`. That filter treats an empty value as absent and uses the first of repeated values. An identity lookup must not silently turn into the full list.
- Unrecognized query parameters, and malformed parts of the query outside `external_id`, keep today's handling.
- Stored agents need no migration. Existing external IDs become searchable as they are.
- The requesting user is the API stakeholder for this admin API change.
- The feature depends on the agent `external_id` attribute from `specs/006-domain-model-apis/` and on canonical reference presentation from `specs/036-canonical-resource-ids/`.

## Out of Scope

- Making `external_id` unique or scoping it by source system.
- Prefix, case-insensitive, wildcard, or multi-value search, and filters on other agent attributes.
- Pagination or sorting of the agent list.
- A single-agent lookup resource, such as `GET /api/agents/by-external-id/{external_id}`.
- Changes to the write validation of `external_id`.
- Stricter handling of malformed query parameters other than `external_id`.
- Lookup by external ID in end-user, OAuth2, or ExtProc flows.
