# Labels and Annotations on Agents, Services, and Permission Sets

**Status**: Accepted
**Date**: 2026-11-27

## Context

Operators manage Agents, Services, and Permission Sets from GitOps repositories and internal catalogues. They need to categorize these resources and attach tool-owned notes without inventing naming conventions in display names or descriptions. Kubernetes labels and annotations are the shape their controllers already copy.

The maintainer confirmed the API draft ([object-metadata.md](../specs/20261127-093439-object-labels-annotations/contracts/object-metadata.md) and [admin-openapi-metadata.yaml](../specs/20261127-093439-object-labels-annotations/contracts/admin-openapi-metadata.yaml)) on 2026-11-27 ([confirmation comment](https://github.com/zalando-incubator/agentic-identity-broker/pull/577#issuecomment-7219784410)), including the fixed error messages, the write matrix, and the 512 KiB body caps. The research record is [research.md](../specs/20261127-093439-object-labels-annotations/research.md).

## Decision

### Value object

**Object Metadata** is one immutable value in `internal/domain/model`: a label map and an annotation map. It has no ID, repository, or lifecycle. `storage.Agent`, `storage.PermissionSet`, and `model.ThirdpartyOAuth2ProviderEntity` each embed exactly one. It has three constructors and checks:

- `NewObjectMetadata` is the Admin write constructor. It applies every rule below, including the reserved prefix, and copies its inputs.
- `RestoreObjectMetadata` is the persistence constructor. It copies stored maps and applies no rule.
- `Validate` re-checks the grammar, NUL, count, and size rules, but not the reserved prefix. The entity validators call it, and they also run inside the Agent repositories' `Create` and `Update`. A reserved-prefix check there would block a key the broker writes itself once it defines one. Every Admin write still applies the reserved prefix through `NewObjectMetadata`.

`Labels()` and `Annotations()` return copies. `Equal` compares both maps independent of key order. The zero value is valid and empty.

### Rules

Every rule runs on the decoded string exactly as received. Nothing trims, case-folds, or normalizes it.

- A key is `[prefix/]name` with at most one `/`. The prefix is 1–253 characters of a lowercase DNS subdomain. The name is 1–63 characters matching `([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]`. This mirrors Kubernetes qualified names. The patterns use the standard `regexp` package.
- A label value is empty, or 1–63 characters matching the name pattern. An annotation value is valid UTF-8 without U+0000.
- At most 64 labels. The decoded UTF-8 bytes of all annotation keys and values total at most 65,536.
- A key whose prefix equals `agenticidentitybroker.dev`, or ends with `.agenticidentitybroker.dev`, is reserved for the broker and rejected in both maps on every Admin write path. The broker defines no reserved keys yet.

### Decoding

The Admin handlers keep their existing `encoding/json` decoding of every other field. A separate scan with the standard `encoding/json/jsontext` package reads the first top-level JSON value of the same body bytes, finds the member spelled exactly `metadata`, and stops when that value ends. A repeated `metadata` member, or a case variant such as `Metadata`, is rejected, because v1 would resolve it last-wins and a GitOps typo would otherwise clear Agent metadata. Other members keep their current lenient treatment of duplicates and invalid UTF-8.

The `metadata` value is decoded strictly. `null` at any level, unknown or repeated members, duplicate map keys, non-string values, invalid UTF-8, and unpaired surrogate escapes are rejected. v1 would silently keep the last duplicate and replace invalid text with U+FFFD. Checks run in a fixed order: request member, metadata shape, labels, then annotations. Within a map, entries are checked in document order. The count and size limits apply after the map is read.

Every failure returns `400 validation failed` with one of the fixed messages in contract §4. A message names the map and the rule and never contains a submitted key, a value, or `jsontext` error text, so the existing `WarnContext(..., "error", err)` calls stay free of metadata. Existing body-read and decode errors keep their codes and precedence. Metadata checks run before any domain call, discovery, branch-key, or encryption step.

The three `POST` handlers now read the body with `io.ReadAll` and decode with `json.NewDecoder(bytes.NewReader(body))`, so trailing bytes and error messages keep their current behavior while the scan reads the same value. Agent ID-addressed `PUT` and collection `PUT` raise their undocumented 64 KiB body cap to 512 KiB. The largest valid annotation map, sent entirely as six-byte `\u00XX` escapes, needs 393,216 bytes. Other write endpoints keep their behavior.

### Write semantics

| Request `metadata` | Agent (POST, PUT, collection PUT) | Permission Set (POST, PUT) | Service POST | Service PUT |
|---|---|---|---|---|
| Omitted | Empty | Empty | Empty | Keep stored value |
| `{}` | Empty | Empty | Empty | Empty |
| Sent | Both maps replaced; a missing member is empty | same | same | same |
| `null` or invalid | `400`, nothing changes | `400` | `400` | `400` |

Keys are never merged. For Agents the handler passes the empty value when `metadata` is omitted. An omitted `metadata` and `{}` are equal under every Agent write, so this conversion decides no field policy, and the domain keeps field policy under `047-unique-agent-external-id-reconcile`. `sameDefinition` adds `Equal` on Object Metadata, so a collection `PUT` that only reorders keys is a no-op with no write or audit record, and any map difference is an update. This extends `047-unique-agent-external-id-reconcile` without deviating from it: `metadata` joins the update-branch fields that are emptied when omitted and replaced when sent.

Services follow the `IconSVG` precedent with a transient `ObjectMetadataProvided` flag. `prepareUpdate` copies the persisted value when the flag is false. The memory adapter clears the flag in stored records. Accepted changes advance `updated_at` and emit the resource's existing audit record. Service writes also increment the row version, and with it the `ETag`.

### Persistence

Atomic migration `051_object_metadata` adds `labels` and `annotations` as `JSONB NOT NULL DEFAULT '{}'` to `agents`, `thirdparty_oauth2_services`, and `permission_sets`, each with a named `CHECK (jsonb_typeof(...) = 'object')`. `NOT NULL` alone would admit the JSON literal `null`. There is no index or backfill, and existing rows read as empty. Rollback loses only metadata.

Adapters write `'{}'` for an empty map. Every explicit column list that reads or writes these entities includes both columns, including the OAuth2 hot paths `GetByClientID` and `GetByClientURI`. Reads decode each column into a string map and rebuild the value with `RestoreObjectMetadata`. Reads apply no write rule, so a broker-defined reserved key or a row edited directly in the database can never fail an OAuth2 client lookup or token exchange. A column that is not a JSON object of strings is an internal storage error, and the adapter returns no partial entity. Repository interfaces do not change.

### No runtime effect and no telemetry

Labels and annotations play no part in authorization, consent, token issuance or claims, CEL, ExtProc or OPA input, approval matching, or AuthZEN Grant Context. No End-User API response carries them; the End-User DTOs are explicit and gain no field, and the entity fields carry no JSON tag. Only Admin API callers can write them.

The feature adds no log event, span, attribute, or metric. Existing audit records log named fields only, so they carry no metadata. A later ADR may expose labels to CEL and OPA. It must supersede this decision and decide scope, freshness, and write controls.

## Consequences

- Admin API 3.1.0 adds `metadata` additively. End-User API 1.3.0 is unchanged.
- A worst-case Agent carries about 90 KB of metadata, which Admin list responses return. Label selectors and their index are out of scope.
- `NewObjectMetadata` is the only path that builds metadata from an Admin request, and it fails closed.
- The broker can define reserved keys later without breaking reads of existing rows.

## References

- [spec.md](../specs/20261127-093439-object-labels-annotations/spec.md), [plan.md](../specs/20261127-093439-object-labels-annotations/plan.md), [data-model.md](../specs/20261127-093439-object-labels-annotations/data-model.md)
- `047-unique-agent-external-id-reconcile`
- `20261119-211813-service-icons-image-mode`
