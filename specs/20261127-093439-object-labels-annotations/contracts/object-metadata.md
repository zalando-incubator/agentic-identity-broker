# Contract: Object Metadata on Agents, Services, Permission Sets, and Service Accounts

Status: confirmed by the maintainer on 2026-11-27 ([confirmation](https://github.com/zalando-incubator/agentic-identity-broker/pull/577#issuecomment-7219784410); API-004, Constitution X) and published in
`api/admin/openapi.yaml` 3.1.0. The Service Account rows were added on 2026-11-27 by the
follow-up amendment to `release/v0.2` (User Story 4) and are confirmed in its pull request. The
OpenAPI fragments are in
[admin-openapi-metadata.yaml](admin-openapi-metadata.yaml). This contract applies only to the
Admin API. `api/enduser/openapi.yaml` does not change.

## 1. Shape

```json
"metadata": {
  "labels":      {"deployment": "local", "team.example.com/owner": "platform"},
  "annotations": {"gitops.example.com/revision": "4f2c9e1"}
}
```

- `metadata` is optional on these schemas: `AgentCreateRequest` (used by POST and collection
  PUT), `AgentUpdateRequest`, `ServiceCreateRequest`, `ServiceUpdateRequest`, and
  `CreatePermissionSetRequest` (used by POST and PUT), and `ServiceAccountRequest` (used by POST
  and PUT). The responses `Agent`, `Service`, `PermissionSetResponse`, and `ServiceAccount` carry
  it as well. `ServiceAccountStatus` does not.
- `metadata` has only the members `labels` and `annotations`, spelled exactly that way. Both are
  optional. Each is a JSON object whose member values are strings.
- The top-level request member must be spelled exactly `metadata` and appear at most once.
- `"metadata": null` is rejected. `"labels": null` and `"annotations": null` mean an empty map, as Kubernetes accepts them and Go clients send them for empty maps. A `null` value inside a map is rejected.

## 2. Presentation

- Responses omit an empty `labels` or `annotations` map, and they omit `metadata` when both maps
  are empty.
- Map members appear in ascending byte order of their keys, whatever order the request used.
- Keys and values are returned exactly as stored, byte for byte.
- No End-User API response carries `metadata`, labels, or annotations. This includes
  `AgentDelegation`, `AgentDetail`, consent views, and session views.

## 3. Rules

Each rule is checked on the decoded string exactly as received. No step trims or normalizes it,
and keys and values are stored and compared exactly as sent. The one exception follows
Kubernetes: the grammar and reserved-prefix checks of an annotation key ignore ASCII case, so
`Example.com/owner` is a valid annotation key. Label keys stay case-sensitive.

| Item | Rule |
|---|---|
| Key (labels and annotations) | `[prefix/]name`, with at most one `/` |
| Key prefix | 1–253 characters matching `[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*` |
| Key name | 1–63 characters matching `([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]` |
| Reserved key | A prefix equal to `agenticidentitybroker.dev` or ending with `.agenticidentitybroker.dev`. Rejected in both maps on every Admin write |
| Label value | Empty, or 1–63 characters matching the key-name pattern |
| Label count | At most 64 per object |
| Annotation value | Any valid UTF-8 text without U+0000. The empty string is allowed |
| Annotation size | The sum of the UTF-8 byte lengths of all decoded annotation keys and values is at most 65,536 |
| Duplicates | A key repeated within one `labels` or `annotations` object is rejected. So is a repeated member within `metadata` |
| Encoding | Invalid UTF-8 and unpaired surrogate escapes are rejected |

Keys are matched literally, so `Exposure` and `exposure` are distinct keys in both maps. Only
ASCII letters are folded for the annotation-key check, so a non-ASCII look-alike such as the
Kelvin sign (U+212A) never passes as `k`.

## 4. Errors

Every metadata failure returns `400` with the existing envelope `{"error": "validation failed",
"message": <fixed message>}`. Messages are fixed strings and never contain a submitted key or
value. The first failure wins. Checks run in this order:

1. The request-level member.
2. Decoding of `metadata` with `encoding/json/v2`, in document order: `null` or non-object
   `metadata`, unknown or repeated members, non-object maps, duplicate keys, invalid UTF-8,
   unpaired surrogates, and non-string or `null` values.
3. The `labels` rules, then the `annotations` rules. Within a map, entries are checked in
   ascending key order: key grammar, reserved prefix, then value. The count and size limits are
   checked after the map's entries.

| Condition | `message` |
|---|---|
| `metadata` repeated, or present under a case variant such as `Metadata` | `request body must contain at most one metadata member spelled exactly metadata` |
| `metadata` is `null` | `metadata must not be null` |
| `metadata` is not an object | `metadata must be an object` |
| `metadata` has a member other than `labels` or `annotations`, or repeats one | `metadata contains an unknown or repeated member` |
| `labels` is not an object | `metadata.labels must be an object` |
| A label key is repeated | `metadata.labels contains a duplicate key` |
| A label key is invalid, including invalid UTF-8 | `metadata.labels contains an invalid key` |
| A label key is reserved | `metadata.labels contains a reserved key` |
| A label value is invalid, including a non-string, `null`, or invalid UTF-8 | `metadata.labels contains an invalid value` |
| There are more than 64 labels | `metadata.labels contains more than 64 entries` |
| `annotations` is not an object | `metadata.annotations must be an object` |
| An annotation key is repeated | `metadata.annotations contains a duplicate key` |
| An annotation key is invalid | `metadata.annotations contains an invalid key` |
| An annotation key is reserved | `metadata.annotations contains a reserved key` |
| An annotation value is invalid: a non-string, `null`, invalid UTF-8, or U+0000 | `metadata.annotations contains an invalid value` |
| Annotation keys and values exceed 65,536 bytes | `metadata.annotations exceeds 65536 bytes` |

These conditions keep their existing codes, which are unaffected by metadata:
- A body that cannot be read, or that exceeds the endpoint cap, returns `400 invalid request body`.
- A body that is not JSON returns `400 invalid request body`.
- Bytes after the request object keep their current treatment on each endpoint. `metadata` is
  read only from the request object.
- Other field errors keep their existing codes and messages.

A failed write changes nothing. This holds even when the same request carries other valid
changes (FR-012).

## 5. Write semantics

| Request `metadata` | Agent (POST, PUT, collection PUT) | Permission Set (POST, PUT) | Service POST | Service PUT | Service Account POST | Service Account PUT |
|---|---|---|---|---|---|---|
| Omitted | Empty | Empty | Empty | Keep stored value | Empty | Keep stored value |
| `{}` | Empty | Empty | Empty | Empty | Empty | Empty |
| Sent with one member | That map replaced, the other empty | same | same | same | same | same |
| Sent with both | Both replaced | Both replaced | Both stored | Both replaced | Both stored | Both replaced |

- Keys are never merged.
- On Agent collection PUT, labels and annotations are compared as maps for the no-op rule. A
  reordered replay returns `200` with the unchanged representation and `updated_at`. It writes
  nothing and emits no audit record. A difference in either map is an actual update.
- Accepted changes advance `updated_at` and emit the resource's existing mutation audit record.
  The record contains no metadata. Service writes also increment the row version, and with it
  the `ETag`, as any Service update does.
- Service Account updates follow the same rule with their own audit records. A metadata change
  adds `metadata` to `changes`. When metadata is the only change, the record's `event` is
  `service.service_account.metadata_changed`, last in the existing precedence. A metadata change
  increments the version and the `ETag` and does not re-encrypt the credential. A Service Account
  `PUT` whose metadata equals the stored metadata, in any key order, and that changes nothing
  else is a no-op: `200` with the same `ETag`, no write, and no record.
- On Service Account writes, the metadata checks run after the body is decoded and the
  `canonical_id`, `kind`, `scopes`, and `credential` members are read, and before any domain
  rule, credential validation, or encryption. A `null` member error therefore wins over a
  metadata error, and a metadata error wins over a Service Account rule.
- Only Admin API callers can write metadata (FR-018).

## 6. Request-body caps

Agent ID-addressed `PUT` and collection `PUT` accept bodies up to 512 KiB. Their previous
undocumented cap was 64 KiB. Service Account `POST` and `PUT` also accept bodies up to 512 KiB,
up from their documented 64 KiB, and the `invalid request body` message for a larger body reads
`request body must be one JSON object of at most 512 KiB with known members`. Other write
endpoints keep their current behavior.

## 7. Runtime isolation

Labels and annotations play no part in these: authorization, consent, token issuance and claims,
CEL, ExtProc or OPA input, approval matching, and AuthZEN Grant Context. They never appear in a
log record, audit record, span, span event, or metric attribute. A later, superseding ADR may
expose labels to CEL and OPA, and that ADR decides scope, freshness, and write controls.

## 8. Collection PUT field-policy row (ADR 047 extension)

| Fields | Omitted | `null` | Value |
| --- | --- | --- | --- |
| `metadata` | emptied | `400 validation failed` | replaced |
| `metadata.labels`, `metadata.annotations` | emptied | emptied | replaced |
