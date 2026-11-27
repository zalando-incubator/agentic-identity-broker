# Research: Labels and Annotations on Agents, Services, and Permission Sets

Source inspection on 2026-11-27 covered the Admin handlers in
`internal/adapters/http/handlers/admin/`, `internal/domain/agents/`, `internal/domain/permissionset/`,
`internal/domain/thirdparty/service.go`, `internal/domain/model/`, `internal/domain/storage/`, both
storage adapters, `migrations/`, `api/admin/openapi.yaml`, and
`adrs/047-unique-agent-external-id-reconcile.md`. Each decision below names the source that drives it.

## 1. JSON decoding of the metadata member

**Decision:** Decode the `metadata` value with the standard library's `encoding/json/jsontext`
token decoder, not with `encoding/json` v1. Track member names in a set per object, so a repeated
key gets a fixed message. Treat any `jsontext` error as a fixed-message validation failure. Never
return, wrap, or log the `jsontext` error text.

**Rationale:** A probe against the repository toolchain (Go 1.27.4) showed that v1
`json.Unmarshal` accepts `{"a":"1","a":"2"}` as `a=2`. It also replaces invalid UTF-8 and unpaired
surrogate escapes (`"\ud800"`) with U+FFFD without returning an error. Either behavior would let
the stored map differ from what the caller sent. FR-004, FR-008, and FR-010 forbid that. `jsontext`
is in the Go 1.27 standard library and needs no `GOEXPERIMENT`. By default it rejects invalid UTF-8,
invalid surrogates, and duplicate names. Its error strings quote the offending member name
(`duplicate object member name "a"`), so returning them would break FR-012.

**Alternatives considered:** v1 `json.Decoder.Token()` does not reject duplicate names, and it
still replaces invalid UTF-8. Making all Admin decoding v2 would change how every existing field
is decoded, which is outside this feature's scope.

## 2. Locating the top-level `metadata` member

**Decision:** The handler reads the body bytes once. It decodes the existing request struct with
v1 as today; that struct has no metadata field. A separate top-level `jsontext` scan then finds
`metadata` and is the only source of its value and presence. The scan allows invalid UTF-8 and
duplicate names in other members, so their current behavior does not change. Two cases are
rejected: a repeated `metadata` member, and a member whose name matches `metadata` only when case
is ignored (for example `Metadata`).

**Rationale:** v1 binds struct fields case-insensitively and keeps the last duplicate. If v1 bound
`metadata`, a body with `"metadata"` and `"METADATA"` would be resolved last-wins. A v1 binding
with a separate presence map could also disagree with that map. Silently ignoring a misspelled
`Metadata` would be worse: Agent writes treat an omitted `metadata` as empty (FR-013), so a GitOps
typo would clear stored metadata. Rejection fails closed (SR-001).

**Precedence:** Existing body-read and v1 decode errors keep their current codes and messages.
Metadata checks run after those errors and before any domain call, discovery, branch-key, or
encryption step.

## 3. Key and value grammar

**Decision:** Implement FR-005 and FR-006 with `regexp` patterns from the standard library. They
mirror Kubernetes `IsQualifiedName`, `IsValidLabelValue`, and `IsDNS1123Subdomain`. The prefix is
at most 253 characters and matches
`[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*`. Like Kubernetes, the grammar
has no separate 63-character limit per DNS label. The name and the label value match
`([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]`, with 1–63 characters. The empty string is also a valid
label value. Annotation keys use the label-key grammar. Every check runs on the string exactly as
received.

**Rationale:** GitOps controllers copy Kubernetes metadata. Kubernetes parity means a key that a
Kubernetes object accepts is also accepted here, except for reserved keys. Because the patterns
are ASCII-only, they reject whitespace, tabs, non-ASCII look-alikes, and uppercase prefixes
without any normalization.

**Alternatives considered:** Importing `k8s.io/apimachinery/pkg/util/validation` would add a large
dependency tree for three expressions. Constitution III covers cryptography, not grammar. An RFC
1123 rule with a 63-character limit per label would reject keys that Kubernetes accepts.

## 4. Reserved prefix

**Decision:** A key is reserved when its prefix equals `agenticidentitybroker.dev` or ends with
`.agenticidentitybroker.dev`. The check covers both maps on every Admin write path: Agent `POST`,
Agent `PUT`, Agent collection `PUT`, Service `POST` and `PUT` in both discovery and explicit
branches, and Permission Set `POST` and `PUT` (SR-003). The broker defines no reserved keys yet.

**Rationale:** Prefixes are lowercase by grammar, so `Agenticidentitybroker.dev/x` already fails
as an invalid key. The reserved check therefore needs no case-folding. A suffix check on the prefix
string accepts `example.agenticidentitybroker.dev.evil.com`, as required by US2.2.

## 5. Size limits and request-body caps

**Decision:** Allow at most 64 labels per object. The UTF-8 byte lengths of all decoded annotation
keys and values together must not exceed 65,536 bytes. Raise the existing 64 KiB
`http.MaxBytesReader` cap on Agent ID-addressed `PUT` and collection `PUT` to 512 KiB. Do not add
a cap to Agent `POST`, Service writes, or Permission Set writes, which have none today. A body
over the cap keeps the existing `400 invalid request body` response.

**Rationale:** `agents_handler.go` limits `UpdateAgent` and `ReconcileAgent` to 64 KiB. That makes
US2.3 (exactly 65,536 annotation bytes accepted) impossible on two of the three Agent write paths.
In the worst case, every annotation byte is a control character escaped as `\u00XX`
(6 × 65,536 = 393,216 bytes). Add 64 maximum labels (about 25 KB) and the previous 64 KiB for the
other fields, and the body stays under 512 KiB. The 64 KiB cap is not documented in
`api/admin/openapi.yaml`, so raising it is additive. API-004 confirmation covers the change.

**Cost:** Admin list responses grow with metadata. A worst-case Agent carries about 90 KB of
metadata. Pagination and list shapes do not change, and label selectors are out of scope.

## 6. Domain placement

**Decision:** Add a `model.ObjectMetadata` value object in `internal/domain/model/`. Its label and
annotation maps are unexported. `model.NewObjectMetadata(labels, annotations)` validates FR-004 to
FR-009 and copies its inputs. It also has `Labels()` and `Annotations()`, which return copies, plus
`Equal` and `IsEmpty`. The zero value is valid and empty. The HTTP adapter owns JSON shape:
object, members, null, duplicates, UTF-8, and string types. The domain owns the semantic rules.
`storage.Agent.Validate`/`ValidateForCreate`, `storage.PermissionSet` validation, and the provider
`ValidateForCreate`/`ValidateForUpdate`/`ValidateDiscoveryRequest` re-check the value, so domain
services enforce the invariant (Constitution VI).

**Rationale:** `internal/domain/storage` already imports `internal/domain/model`, and `model` does
not import `storage`. One value object can therefore serve all three entities without a new
bounded context. `ObjectMetadata` is a value object, not an entity, so it needs no typed ID under
ADR 013.

**Alternatives considered:** Plain `map[string]string` fields on each entity would spread
validation and equality across three packages. A `domain/metadata` package would be a bounded
context without behavior of its own.

## 7. Write semantics

**Decision:**
- **Agent:** The handler always passes a value; an omitted `metadata` becomes the empty value. The
  domain keeps the field-policy decision under ADR 047 ("handlers parse presence but do not decide
  field policy"). `sameDefinition` in `internal/domain/agents/reconcile.go` adds
  `a.Metadata.Equal(b.Metadata)`, which is map equality on both maps.
- **Permission Set:** Create and `PUT` set the value from the request. Every `PUT` writes and
  advances `updated_at`, as today.
- **Service:** Add `Metadata` and `MetadataProvided` to `ThirdpartyOAuth2ProviderEntity`. They
  follow the `IconSVG`/`IconSVGProvided` precedent. `prepareUpdate` copies the persisted value when
  the field is not provided. The memory record clears `MetadataProvided` before storing, as it does
  for `IconSVGProvided`. Validation runs before discovery, branch-key provisioning, encryption,
  and persistence.

**Rationale:** ADR 047 already replaces the update-branch fields (`governance_url`, collections)
in full. A replaced `metadata` field follows that same policy, so it extends ADR 047 without
deviating from it, and no superseding ADR is needed. The new ADR records the extension. The
service icon feature showed that the merge must happen in `prepareUpdate`, which uses persisted
state, and not only in `ValidateUpdate`, which validates a copy.

## 8. Persistence

**Decision:** Add migration `050_object_metadata` (the inspected sequence ends at `049`). It is
atomic and adds `labels JSONB NOT NULL DEFAULT '{}'` and `annotations JSONB NOT NULL DEFAULT '{}'`
to `agents`, `thirdparty_oauth2_services`, and `permission_sets`. Each column gets a named
`CHECK (jsonb_typeof(<column>) = 'object')`. The down migration drops the constraints and columns.
Adapters always write `{}` for an empty map, never JSON `null`. Every explicit
`SELECT`/`INSERT`/`UPDATE`/`RETURNING` list that builds or writes one of these entities gains
both columns. This includes the hot paths `GetByClientID` and `GetByClientURI`, so that no adapter
returns a partly loaded entity. Read paths rebuild the value through `model.NewObjectMetadata`. A
stored value that fails validation is an internal storage error (fail closed).

**Rationale:** `NOT NULL` alone does not stop the JSON literal `null`, because v1 marshals a nil
map as `null`. The type check does. JSONB reorders keys and keeps only the last of any duplicate
keys. This is harmless here: the application validates before writing and compares maps, not
text. JSONB rejects `\u0000` in strings, which matches the NUL ban.

**Alternatives considered:** A key/value child table would suit label selectors, which are a
follow-up that also needs an index. Plain text columns would lose the database type check.

## 9. Response presentation

**Decision:** Add a response type for `metadata` whose maps use `omitempty`. The response omits
`metadata` when both maps are empty. v1 `encoding/json` encodes `map[string]string` keys in
ascending byte order, which satisfies FR-003.

## 10. Telemetry and audit

**Decision:** Add no log record, span, span attribute, or metric. The existing records stay as they
are: `agents.agent.created`/`updated`, `permissionset.permission_set.created`/`updated`, and
`service.thirdparty.provider_created`/`updated`. Each one logs named fields only, so it carries no
metadata. Validation errors are fixed strings. The existing handler pattern
`WarnContext(..., "error", err)` is therefore safe for metadata failures. Tests inject a buffered
JSON logger, the span recorder, and a metric reader, and they assert that sentinel keys and values
never appear (FR-022, SC-006). No `contracts/observability.md` is needed because there are no new
instruments.

**Sources:** `coding-standards/LOGGING.md`, `coding-standards/OPENTELEMETRY.md`,
`tests/e2e/bootstrap/logger.go` (`NewBufferedJSONLogger`), `tests/e2e/bootstrap/telemetry.go`, and
`tests/e2e/session_metrics_test.go` (`sdkmetric.ManualReader`).

## 11. API versioning and governance

**Decision:** The change is additive, so the Admin API moves from 3.0.0 to 3.1.0. The End-User API
stays at 1.3.0 (API-002). A new timestamp-named ADR records the decisions in sections 1–10 and
states that metadata has no runtime effect until a superseding ADR wires labels into CEL and OPA
(SR-005). API-004 requires maintainer confirmation of `contracts/` before implementation.

## 12. Unknown members (spec wording corrected)

The spec's edge case first said an unknown `metadata` member is rejected "as for other unknown
request fields". Agent, Permission Set, and Service requests do not reject unknown top-level fields
today. Only named Service credential-file fields are refused. The edge case now states that the
rule applies only inside `metadata` and that top-level behavior does not change, which matches
FR-010 and this plan.
