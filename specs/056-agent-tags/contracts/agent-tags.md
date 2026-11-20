# Proposed Agent Tags API Contract

**Status**: Proposed; requires written maintainer approval before implementation.
**Scope authority**: [#427 clarification](https://github.com/zalando-incubator/agentic-identity-broker/issues/427#issuecomment-6878251111).

🤖 Prepared by Repo Assist, an automated AI assistant.

This document is a reviewable patch plan for the authoritative OpenAPI contracts, not an additional deployed API. No new endpoint, authentication method, filter parameter, response envelope, or identity namespace is proposed.

## Shared property schema

Add this `tags` property to the schemas listed below. Keep it optional in requests and responses. Do not mark the property `nullable` or add it to `required`.

```yaml
tags:
  type: array
  maxItems: 32
  uniqueItems: true
  description: >-
    Literal, case-sensitive Agent categories. Members are valid UTF-8 strings
    of 1–255 Unicode code points without NUL. No trimming, Unicode normalization,
    or wildcard interpretation. Input order is not significant. Writes replace
    the complete set; omission or [] means the empty set and null is invalid.
    Responses omit the empty set and present nonempty sets in ascending Go
    string byte order without mutating stored order. Tags are descriptive
    metadata, not permissions or verified trust labels.
  items:
    type: string
    minLength: 1
    maxLength: 255
    pattern: '^[^\u0000]*$'
  example: [external, local]
```

The `pattern` forbids NUL while permitting newlines and other literal text. Domain validation remains authoritative for UTF-8, code-point counts, cardinality, and uniqueness; an HTTP decoder alone does not enforce these schema constraints. Wrong JSON types must not be coerced. A null element is not a string. Explicit array null must be distinguished from omission before it can collapse into a nil Go slice.

Response descriptions should retain the membership/ordering rules but describe writes by reference to request schemas rather than implying end-user write access. Request descriptions must retain the omission warning.

## Authoritative schema targets

| Contract | Schema | Effect |
|---|---|---|
| `api/admin/openapi.yaml` | `AgentCreateRequest` | POST and External-ID collection PUT input |
| `api/admin/openapi.yaml` | `AgentUpdateRequest` | ID/canonical-ID-addressed PUT input |
| `api/admin/openapi.yaml` | `Agent` | Single, collection, External-ID-filtered, create/update/reconcile response |
| `api/enduser/openapi.yaml` | `AgentDelegation` | `GET /api/consent/agents` item gains optional `tags` |
| `api/enduser/openapi.yaml` | `AgentDetail` referenced by `GetAgentDetailResponse` | `GET /api/consent/agents/{agentId}` gains optional `data.agent.tags` |

Align the actual unified AgentMetadata projection, TypeScript raw response type, API transformation, and view types. The actual detail metadata uses `display_name`, whereas delegation items use `displayName`; adding tags must not rename those unrelated existing fields. Do not fix other contract discrepancies in this feature.

## Write policy

All rows assume an otherwise valid Agent definition, valid references, and the existing authentication/security boundary. This does not turn PUT into PATCH or relax required `permission_sets`, `display_name`, or `description`.

| `tags` input | POST `/api/agents` | PUT `/api/agents/{agent-id}` | PUT `/api/agents?external_id=…` |
|---|---|---|---|
| Omitted | Empty set | Replace with empty set | Empty set on create; replace with empty set on update |
| `[]` | Empty set | Replace with empty set | Same as omission |
| Valid nonempty array | Set supplied members | Replace complete set | Set/replace complete set |
| Same members, different order | Normal create | Existing ordinary update behavior | No-op if every other effective field matches |
| `null` | 400 | 400 | 400 on either branch |
| Duplicate, empty, NUL-containing, too long, too many | 400 | 400 | 400 on either branch |
| Non-array or non-string member | 400 | 400 | 400 on either branch |

Omission-as-empty follows ADR 047's metadata/collection replacement policy, rather than introducing a new preserve-on-omission field. It means older full-definition clients can clear tags; document this explicitly. Do not silently add tag merging or PATCH semantics. Existing client/canonical-ID preservation rules and External-ID selector validation remain unchanged.

For reconcile, validate tags before effective-definition comparison. A tag-only change is a real update; a reordered equivalent set is not. Do not compare sorted slices by mutating the Agent or ignore tags in `sameDefinition`.

## Responses and errors

- Creation: existing 201. Only collection-PUT creation retains its existing UUID `Location` behavior; this proposal adds no POST header promise.
- ID-addressed PUT and collection-PUT update/no-op: existing 200.
- Nonempty responses: `"tags": ["external", "local"]`, sorted identically on every projection. Empty responses omit the property, not `null` or `[]`.
- Wrong JSON types: existing 400 `invalid request body` envelope. Semantic validation (including explicit array null): 400 `validation failed` envelope. No new error codes.
- Stable semantic duplicate error example: `{"error":"validation failed","message":"tags must contain unique strings"}`.
- Errors must not echo the tag values. Existing 401/403/404/409/415/500 paths and existing Host, principal, cross-origin, content-type and session-token checks remain unchanged.
- No new response headers, cache invalidation protocol, ETag, revision guarantees, or grant timestamp changes.

## Compatibility and approval items

Propose Admin API `3.1.0` and End-User API `1.3.0` as additive optional-property updates from their current `3.0.0`/`1.2.0`. Do not bump the broker `VERSION` or issue a release in this design PR. Existing payloads omitting `tags` retain their prior visible behavior; new metadata requires clients maintaining full Agent definitions to include it to retain it. Existing end-user clients may ignore the new property. Review strict response consumers before release.

The maintainer's clarification confirms literal string sets, end-user display, and deferred filtering. Review still needs to confirm this exact property/limit/presence/error/versioning proposal and its full-definition omission behavior. Record approval with a link in `tasks.md`, then amend the real OpenAPI contracts before writing runtime implementation. If review requires deviating from an Accepted ADR, introduce an Accepted superseding ADR instead of editing the merged ADR in place.
