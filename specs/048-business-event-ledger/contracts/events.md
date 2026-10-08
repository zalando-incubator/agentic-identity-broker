# Event and Telemetry Contracts

## Stakeholder review

**Approved**: 2026-09-27, in the implementation conversation.
**Reference**: The user selected `Approve both contracts` for this document and `storage.md`.
The approval covers the closed 28-type catalogue and its event/telemetry contract.
It preserves existing HTTP responses and adds no HTTP or CLI read/erase API.
Additional public contract changes require separate confirmation.

[ADR 039](../../../adrs/039-business-event-ledger.md) records maintainer Jan Brennenstuhl's design acceptance on 2026-09-26.
That design decision does not replace the separate stakeholder approval recorded here.

## Publication and registration

The files in `schemas/` contain the reviewed contracts.
Their published copies reside in `api/events/v1/`.
The common embedding entry point resides in `api/events/`.
The broker embeds schemas at build time and never fetches them over the network.

Each `$id` is the URN `urn:agentic-identity-broker:events:v1:<name>`.
The `<name>` value is `envelope`, `catalogue`, or an event name.
Every `$ref` is an absolute URN of this form.
The registry loader serves these URNs only from its offline sources.
The loader does not use relative references because a URN base cannot resolve them.

The registry compiles from offline `fs.FS` schema sources.
Production passes only the embedded `api/events/v1` catalogue.
`app.Builder.WithBusinessEventSchemas(fs.FS)` appends further reviewed offline sources.
It uses the existing builder injection pattern (`WithCIMDFetcher`, `WithJWKSPublisher`).
Acceptance tests use this method to register a fixture type for US2-AS5.
An added source can introduce new types under the fixed functional name only.

These changes cause builder startup to fail:

- Redefinition of an existing type
- A change to the functional name or source
- Remote `$ref` resolution
- A flattened-name collision.

The schema files contain these contracts:

- `envelope.schema.json`: The common envelope and closed actor/client objects
- `<event-name>.schema.json`: One full-envelope contract per catalogue type. It constrains the type, outcome, required references, and closed data.
- `catalogue.schema.json`: The initial 28-type union. Runtime recording selects the registered per-type schema directly instead of trying 28 alternatives per append.
- `examples.json`: One synthetic shape example per type, with no credentials. These are contract examples, not runtime evidence or event reason-template definitions.

Type names use Zalando Rule 213, `<functional-name>.<event-name>`.
The functional name is `agentic-identity-broker` (functional domain `agentic-identity`, component `broker`).
Each event name matches `[a-z][a-z0-9-]*`, for example `agentic-identity-broker.session-refresh-failed`.
The source is `urn:agentic-identity-broker:broker`.
Neither name nor source depends on the hosting organization.
Neither can vary by deployment, and neither is configurable.

The source identifies the broker application.
Deployment identity remains in existing telemetry Resource metadata.

A registry addition cannot reassign an existing name or meaning.
Compatible evolution adds optional fields and preserves old records.
Previously compiled consumers have no guarantee that they can accept fields outside their published closed schema.

Before emitting added fields, publish revised contracts.
Coordinate consumers before emission.

A semantic or required-field breaking change gets a new type.
It does not reinterpret old events.
Registering a type alone requires no database migration.

## Common semantics beyond JSON shape

See `../data-model.md` for every envelope field.
The recorder verifies these additional invariants:

- IDs are nonzero typed IDs. A library generates the event ID as UUIDv7. The event ID is not an HTTP idempotency key.
- Timestamps parse as real UTC instants. Storage assigns `recorded_at`. Storage does not copy it from request time or the event ID.
- A known affected principal is mandatory even when the caller differs. Unknown and non-user subjects are JSON null, not empty strings or an agent UUID.
- Only established delegation permits non-null `actor.on_behalf_of`. The initiating authenticated caller supplies `actor.id`. It does not blindly use `SecurityContext.Actor`.
- `trace_id`/`span_id` must be valid, nonzero IDs. Only a match with the request's authoritative trace permits inclusion of a span. Background work never receives synthesized request correlation.
- Every reference to an existing grant/session/approval remains after deletion. Refresh failure must retain known service references. The schema permits their absence before resolution.
- The domain event recipe selects one fixed, credential-free sentence for each reason. Reasons contain no user-controlled interpolation, raw errors, claims, or URLs. Events never use existing slog output as a reconstruction source.
- `permission_set_ids` is a sorted unique set. Null and absent are different. Nullable identity fields are present with null. Unavailable optional fields are absent.
- User-agent family normalization produces only a fixed schema enum value. It discards all other text and omits unrecognized input. Client IP uses the normalized authoritative IP, not forwarding headers. Opaque session correlations require established non-credential provenance. Printable or truncated text alone does not make a string safe.
- JSON Schema patterns supplement UUID/time/IP parsing. They do not replace that parsing. The selected validator ignores `format`.

Validate an explicit serialization view with the final wire types (UUID/time strings, nullable identities, primitive payload fields).
Do not validate raw UUID arrays or credential-bearing domain structs.
Reuse that view for one JSON serialization.
Compile schemas once.
Do not allocate a generic request/response copy solely to validate.
Do not marshal then unmarshal solely to validate.

## Actor attribution

`actor` identifies the initiator, not the affected principal or receiving agent.

Select its kind from the authenticated workflow, never from an event name alone.
Keep `subject` as the affected principal.
Keep `agent_id` as the receiving or affected agent.

These rules also govern downstream activity views and SSF `initiating_entity`.
Neither is permitted to reinterpret the subject as the caller.

| Workflow | Actor kind and identity | Represented principal |
|---|---|---|
| RFC 8693 exchange and impersonation using a client assertion | `gateway`. The independently verified initiating client ID, or null before verification | `on_behalf_of=subject` only after accepted delegation |
| Approval creation using dual authentication | `gateway`. The verified client-assertion ID | The established user/agent context |
| Approval consumption using subject-token-only authentication (ADR 018) | `gateway`. Null because the bearer authenticates the represented principal, not a distinct consuming gateway | The authorized approval owner |
| Browser approval decisions, consent, and direct user session actions | `user`. The authenticated acting principal | Null for direct actions |
| Administrative mutations | `admin`. The authenticated administrator, or null when the trusted boundary supplies no administrator identity | Null unless delegation is explicitly established |
| Background expiry and lifecycle work | `system`. The broker lifecycle identity | Null |

Never recover the consuming caller from an approval's creation-time `gateway_client_id`.
That reference identifies the creator.
The creator is not necessarily the later consumer.

A verified actor-token identity in impersonation remains `data.delegating_actor_id`, distinct from the initiating client.
A session refresh triggered by exchange inherits the exchange caller and established delegation.

## Token request boundary

Requests rejected for input errors before credential authentication are not catalogued business outcomes.
This boundary includes HTTP methods, media types, form syntax, and missing required parameters.
These requests retain their existing OAuth2 error responses without durable ledger writes.
Ledger errors must not turn these input errors into 500 responses.

Impersonation rejects scopes outside the target's `AllowedScopes` before rule credential verification.
This `scope_not_permitted` rejection is structural.
It produces neither `impersonation-denied` nor `token-exchange-denied`.
Later rule and delegation decisions retain their recorded outcomes.

Authentication and authorization decisions remain catalogued.
Authenticated terminal workflow failures remain catalogued and fail closed on recording errors.
A resolved public client ID is not authentication.

## Per-type data

Most successful events use an intentionally empty, closed `data` object.
The envelope already contains the identity and relationships of the fact.
This is a selected minimal contract, not an unfinished payload.

| Types | Allowed data fields |
|---|---|
| grant-created, grant-updated, grant-revoked, grant-expired | Empty object |
| session-established, session-refreshed, session-terminated | Empty object |
| session-refresh-failed | Required `reason_code`: upstream_rejected, upstream_unavailable, invalid_response, refresh_unavailable, internal_failure |
| authorization-requested, token-issued, token-exchanged | Empty object |
| token-request-failed | Required `reason_code`: invalid_request, authentication_failed, authorization_failed, upstream_failed, internal_failure |
| token-exchange-denied | Required `reason_code`: invalid_request, authentication_failed, authorization_failed |
| impersonation-granted | Optional `delegating_actor_id`: verified actor-token identity, distinct from initiating client |
| impersonation-denied | Required `reason_code`: authentication_failed, authorization_failed, delegation_missing. Optional verified `delegating_actor_id` |
| approval-requested, approval-approved, approval-consumed, approval-revoked, approval-expired | Empty object |
| approval-denied | Required `reason_code`: user_denied |
| agent-registered, agent-updated, agent-deleted | Empty object |
| credential-generated, credential-rotated, credential-revoked | Required `credential_id`: broker credential record UUID, never secret or hash |
| signing-key-promoted | Required `signing_key_id` UUID and `activates_at` UTC instant. The latter preserves existing JWKS eligibility grace |

Typed outcomes determine these codes.
Do not derive codes by parsing or copying error strings.

`token-exchange-denied` is the primary token failure for authentication/authorization refusal.
After a request enters an authenticated workflow, terminal parsing, transport, and internal failures use `token-request-failed`.
The pre-authentication input errors in the token request boundary do not use that event.
`impersonation-denied` can accompany the primary token failure as a separate permission fact.

If permitted impersonation fails during minting, it records `impersonation-granted` and `token-request-failed`.
It does not record a false denied decision.
Unlisted observations do not acquire new event types implicitly.

Resolver `server_error` records `internal_failure`, not an authentication refusal.
Proxy completion classifies recognized OAuth `error` codes in 4xx responses.
`invalid_client` records `authentication_failed`.
`invalid_grant`, `invalid_scope`, `unauthorized_client`, and `access_denied` record `authorization_failed`.
`invalid_request` and `unsupported_grant_type` retain the `invalid_request` reason.
Unknown codes, malformed bodies, transport errors, and upstream 5xx remain `upstream_failed`.

Only fixed reason codes enter the ledger.
Upstream descriptions and response bodies never enter the ledger.

## Flat OpenTelemetry encoding

Use a dedicated non-global log scope `agentic-identity-broker.ledger`.

EventName equals the full type.
The event timestamp is `occurred_at`.
The observed timestamp is emission time.
Native OTel trace/span fields can use the validated envelope, never the background worker's own trace.
The full envelope remains in attributes even when native fields also contain these values.
The body is empty, so it cannot become a second unsanitized channel.

Attribute keys are literal field paths.
They include `id`, `type`, `source`, `occurred_at`, `recorded_at`, `subject`, `actor.kind`, `actor.id`, `actor.on_behalf_of`, and business reference names.
They also include `client.ip`, `client.user_agent`, `data.reason_code`, and other field paths.
The registry rejects schema property names that contain `.`, begin with `_ledger`, or collide after flattening.
These names cause registry startup to fail.
The encoder emits no map-valued attribute.

Sort field traversal for deterministic encoding.

| JSON value | OTLP encoding |
|---|---|
| String / number / boolean | The corresponding primitive attribute. Times and UUIDs are strings |
| Array | One typed array attribute. An empty array remains a present empty array |
| Absent optional field | No attribute and no null marker |
| Explicit null | Omit the value attribute and include its full path in `_ledger.null_fields` (sorted string array) |
| Empty data object | Include `data` in `_ledger.empty_objects` (sorted string array) |
| Non-empty nested object | Flatten scalar/array leaves. Do not emit a map |

This encoding distinguishes explicit null, absent, empty string, empty array, and empty object without a magic value such as the string `null`.
Bookkeeping attributes remain outside the JSON envelope and cannot collide with schema fields.

If no null/empty-object paths exist, omit the corresponding bookkeeping attribute.
Set explicit SDK provider limits to unlimited count and value length.
Override environment defaults only for this ledger provider.

Before export, the emitting processor verifies zero dropped attributes and the intended attribute count.
No required field is permitted to disappear silently.

Use existing OTLP resource configuration.
Do not copy request headers into Resource attributes.

The event credential-exclusion guarantee covers event values.
Operator-managed exporter credentials remain transport configuration, never event attributes.

## Delivery lifecycle and configuration

Recording and delivery are separate.
The business transaction creates delivery references only when `telemetry.enabled && telemetry.logs.enabled && business_events.telemetry_copy_enabled`.
A temporary exporter failure does not change that eligibility decision.

The exporter uses the existing endpoint, protocol, headers, timeout, TLS/insecure, and compression configuration.
It introduces no independent destination configuration.

A builder-owned background worker starts after storage and schema registry initialization.
It scans at startup and at a fixed one-second cadence.
Each scan selects at most 100 candidate IDs.
The worker dispatches each retained event under the storage delivery barrier.

Export failure or cancellation, including an exporter deadline, sets the next attempt 30 seconds later.
The existing exporter timeout and storage write budget bound export.
Storage reserves at most one additional second to persist the retry schedule before it releases deletion barriers.
These constants control internal work schedules, not additional user configuration.

A private synchronous SDK processor captures the actual export result per call.
Return from `Logger.Emit` is not sufficient evidence.
Successful export plus acknowledgement commit removes the delivery reference.
Export failure leaves it pending.
A post-export crash can cause duplicates with the same ID.

Disabling copying pauses pending work and omits new references.
Re-enabling copying resumes only retained pending work.
Retention remains active regardless of telemetry configuration.

The SDK ledger path has no asynchronous payload buffer.
Before commit, erasure and retention wait for already-running synchronous attempts to finish or cancel under the barrier.
The broker cannot recall packets that it already dispatched to an external collector.
The external system remains responsible for its retention.
A later broker retry must not send a deleted event.

Existing slog and its batch pipeline remain unchanged, including names, levels, messages, and fields.

The graceful shutdown sequence is:

1. HTTP servers drain requests.
2. With copying enabled, delivery shutdown cancels and waits for the worker,
   which closes its ledger provider before exit.
3. On memory storage, retention shutdown then cancels and waits for the
   maintenance worker.
4. Existing telemetry shutdown completes.
5. Storage closes.

Disabled copying omits delivery/provider shutdown.
PostgreSQL retention uses the external maintenance scheduler, not an in-process
worker.

A worker timeout returns an error, and later shutdown steps still run.
Unsent events remain unacknowledged.
PostgreSQL pending work remains recoverable after restart.
Memory storage does not persist pending work across restarts.
