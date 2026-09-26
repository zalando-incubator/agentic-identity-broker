# Event and Telemetry Contracts

## Publication and registration

The files in `schemas/` are the Phase 1 contract artifacts. Before producer implementation, publish these exact reviewed contracts under `api/events/v1/`, with the common embedding entry point in `api/events/`. Embed at build time; never fetch schemas over the network. Each `$id` is the URN `urn:agentic-identity-broker:events:v1:<name>`, where `<name>` is `envelope`, `catalogue`, or an event name, and every `$ref` is such an absolute URN. The registry's loader serves these URNs only from its offline sources. Relative references are not used, because a URN base cannot resolve them.

The registry compiles from offline `fs.FS` schema sources. Production passes only the embedded `api/events/v1` catalogue. `app.Builder.WithBusinessEventSchemas(fs.FS)` appends further reviewed offline sources, following the builder's existing injection pattern (`WithCIMDFetcher`, `WithJWKSPublisher`); acceptance tests use it to register a fixture type for US2-AS5. An added source may introduce new types under the fixed functional name only. Redefining an existing type, changing the functional name or source, remote `$ref` resolution, or a flattened-name collision fails builder startup.

- `envelope.schema.json`: common envelope and closed actor/client objects.
- `<event-name>.schema.json`: one full-envelope contract per catalogue type, constraining type, outcome, required references and closed data.
- `catalogue.schema.json`: the initial 28-type union. Runtime recording selects the registered per-type schema directly instead of trying 28 alternatives per append.
- `examples.json`: one synthetic shape example per type, containing no credentials. These are contract examples, not runtime evidence or event reason-template definitions.

Type names follow Zalando Rule 213, `<functional-name>.<event-name>`: the functional name is `agentic-identity-broker` (functional domain `agentic-identity`, component `broker`), and each event name matches `[a-z][a-z0-9-]*`, for example `agentic-identity-broker.session-refresh-failed`. Source: `urn:agentic-identity-broker:broker`. Neither depends on the hosting organization, neither may vary by deployment, and neither is configurable. Source identifies the broker application; deployment identity remains existing telemetry Resource metadata.

A registry addition cannot reassign an existing name or meaning. Compatible evolution adds optional fields and preserves old records. Previously compiled consumers are not promised acceptance of fields outside their published closed schema: publish revised contracts and coordinate consumers before emitting added fields. A semantic or required-field breaking change gets a new type rather than reinterpreting old events. No database migration is needed merely to register a type.

## Common semantics beyond JSON shape

See `../data-model.md` for every envelope field. The recorder checks these additional invariants:

- IDs are nonzero typed IDs and the event ID is library-generated UUIDv7. The event ID is not an HTTP idempotency key.
- Timestamps parse as real UTC instants. `recorded_at` is assigned by storage, not copied from request time or event ID.
- A known affected principal is mandatory even when the caller differs. Unknown and non-user subjects are JSON null, not empty strings or an agent UUID.
- `actor.on_behalf_of` is non-null only after the delegation is established. `actor.id` is the initiating authenticated caller, not blindly `SecurityContext.Actor`.
- `trace_id`/`span_id` must be valid, nonzero IDs. A span is included only if its trace matches the request's authoritative trace. Never synthesize request correlation for background work.
- Every reference to an existing grant/session/approval is preserved after deletion. Known service references on refresh failure must be retained even though the schema permits their absence before resolution.
- Reasons are one fixed, credential-free sentence each, selected by the domain event recipe. No user-controlled interpolation, raw errors, claims or URLs. Existing slog output is not a source from which events are reconstructed.
- `permission_set_ids` is a sorted unique set. Null and absent are different: nullable identity fields are present with null; unavailable optional fields are omitted.
- User-agent family normalization produces only a fixed schema enum value, discards all other text, and omits unrecognized input. Reuse normalized authoritative IP, not forwarding headers. Opaque session correlations require established non-credential provenance; being printable or truncated does not make a string safe.
- JSON Schema patterns supplement but do not replace UUID/time/IP parsing. The selected validator ignores `format`.

Validate an explicit serialization view containing the final wire types (UUID/time strings, nullable identities, primitive payload fields), not raw UUID arrays or credential-bearing domain structs. Reuse that view for one JSON serialization. Compile schemas once; do not allocate a generic request/response copy or marshal then unmarshal solely to validate.

## Per-type data

Most successful events use an intentionally empty, closed `data` object because the envelope already contains the fact's identity and relationships. This is a selected minimal contract, not an unfinished payload.

| Types | Allowed data fields |
|---|---|
| grant-created, grant-updated, grant-revoked, grant-expired | Empty object |
| session-established, session-refreshed, session-terminated | Empty object |
| session-refresh-failed | Required `reason_code`: upstream_rejected, upstream_unavailable, invalid_response, refresh_unavailable, internal_failure |
| authorization-requested, token-issued, token-exchanged | Empty object |
| token-request-failed | Required `reason_code`: invalid_request, authentication_failed, authorization_failed, upstream_failed, internal_failure |
| token-exchange-denied | Required `reason_code`: invalid_request, authentication_failed, authorization_failed |
| impersonation-granted | Optional `delegating_actor_id`: verified actor-token identity, distinct from initiating client |
| impersonation-denied | Required `reason_code`: authentication_failed, authorization_failed, delegation_missing; optional verified `delegating_actor_id` |
| approval-requested, approval-approved, approval-consumed, approval-revoked, approval-expired | Empty object |
| approval-denied | Required `reason_code`: user_denied |
| agent-registered, agent-updated, agent-deleted | Empty object |
| credential-generated, credential-rotated, credential-revoked | Required `credential_id`: broker credential record UUID, never secret or hash |
| signing-key-promoted | Required `signing_key_id` UUID and `activates_at` UTC instant; the latter preserves existing JWKS eligibility grace |

These codes are classified from typed outcomes; do not derive them by parsing or copying error strings. `token-exchange-denied` is the primary token failure for authentication/authorization refusal; parsing, transport and internal failures use `token-request-failed`. `impersonation-denied` may accompany the primary token failure as a separate permission fact. A permitted impersonation that fails during minting records `impersonation-granted` and `token-request-failed`, not a false denied decision. Unlisted observations do not acquire new event types implicitly.

## Flat OpenTelemetry encoding

Use a dedicated non-global log scope `agentic-identity-broker.ledger`. EventName equals the full type. Event timestamp is `occurred_at`; observed timestamp is emission time. Native OTel trace/span fields may be populated from the validated envelope, never the background worker's own trace. The complete envelope remains attributes even when native fields are also set. The body is empty so it cannot become a second unsanitized channel.

Attribute keys are literal field paths: `id`, `type`, `source`, `occurred_at`, `recorded_at`, `subject`, `actor.kind`, `actor.id`, `actor.on_behalf_of`, business reference names, `client.ip`, `client.user_agent`, `data.reason_code`, and so on. Schema property names containing `.`, beginning `_ledger`, or colliding after flattening are rejected at registry startup. Sort field traversal for deterministic encoding. No map-valued attribute is emitted.

| JSON value | OTLP encoding |
|---|---|
| String / number / boolean | Corresponding primitive attribute; times and UUIDs are strings |
| Array | One typed array attribute; empty array is a present empty array |
| Absent optional field | No attribute and no null marker |
| Explicit null | Omit the value attribute and include its full path in `_ledger.null_fields` (sorted string array) |
| Empty data object | Include `data` in `_ledger.empty_objects` (sorted string array) |
| Non-empty nested object | Flatten scalar/array leaves; do not emit a map |

This makes explicit null, absent, empty string, empty array, and empty object distinguishable without using a magic value such as the string `null`. Bookkeeping attributes are outside the JSON envelope and cannot collide with schema fields. If no null/empty-object paths exist, omit the corresponding bookkeeping attribute.

Set explicit SDK provider limits to unlimited count and value length, overriding environment defaults only for this ledger provider. The emitting processor verifies zero dropped attributes and the intended attribute count before exporting. No required field may disappear silently. Use existing OTLP resource settings without copying request headers into Resource attributes. The event credential-exclusion guarantee covers event values; operator-managed exporter credentials remain transport configuration, never event attributes.

## Delivery lifecycle and configuration

Recording and delivery are separate. Pending references are created in the business transaction only when `telemetry.enabled && telemetry.logs.enabled && business_events.telemetry_copy_enabled`. A temporary exporter failure does not change that eligibility decision. The exporter uses the existing endpoint, protocol, headers, timeout, TLS/insecure and compression settings. It introduces no independent destination settings.

A builder-owned background worker starts after storage and schema registry initialization. It scans at startup and at a fixed one-second cadence, at most 100 candidate IDs per scan, and dispatches each retained event under the storage delivery barrier. On export failure set next attempt 30 seconds later; attempts and transport retries are bounded by the existing exporter timeout and the storage operation deadline. These are internal work scheduling constants, not additional user configuration.

A private synchronous SDK processor captures the actual export result per call; `Logger.Emit` returning is not sufficient evidence. Successful export plus acknowledgement commit removes the pending reference. Export failure leaves it pending. A post-export crash may cause duplicates with the same ID. Disabling copying pauses pending work and omits new references; re-enabling resumes only retained pending work. Retention remains active regardless of any telemetry switch.

The SDK ledger path has no asynchronous payload buffer. Erasure and retention wait for any already-running synchronous attempt to finish/cancel under the barrier before they commit. Packets already dispatched to an external collector cannot be recalled; external retention remains that system's responsibility. No later broker retry may send a deleted event. Existing slog and its batch pipeline remain unchanged, including names, levels, messages and fields.

Shutdown order: drain HTTP requests; cancel/wait for ledger worker and memory maintenance; close ledger provider; complete existing telemetry shutdown; close storage. A timed-out worker leaves pending work recoverable rather than acknowledging unsent events.
