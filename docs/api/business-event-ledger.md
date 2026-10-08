# Business Event Ledger

The broker records credential-free business facts in an immutable ledger. Business mutations and their facts share one transaction. If recording fails, the broker rejects the operation before successful responses or credentials leave the broker.

This feature adds no HTTP or CLI endpoint for read, erase, export, or ingestion operations. The existing [end-user](../../api/enduser/openapi.yaml) and [admin](../../api/admin/openapi.yaml) contracts remain unchanged.

If the broker rejects a malformed token request before authentication, that request is an input error, not a durable business event. Ledger unavailability does not change the existing 4xx responses to these requests.

## Operational access

Investigations use an authorized PostgreSQL connection. The optional Helm value `migration.grants.businessEvents.readerRole` names an existing reader role. Its empty default grants nothing. The migration owner manages access. Broker runtime credentials are not operator credentials.

The reader needs `SELECT` on `public.business_events`. The [storage contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/contracts/storage.md#privileges-and-immutability) defines operational role grants and erasure ownership.

## Find the receiving agents

In `psql`, set the exact subject and UTC interval. The interval includes its start and excludes its end. Then run the query:

```sql
\set principal 'principal-example'
\set start '2026-09-26T00:00:00Z'
\set end '2026-09-27T00:00:00Z'

SELECT DISTINCT envelope->>'agent_id' AS receiving_agent
FROM public.business_events
WHERE subject = :'principal'
  AND type = 'agentic-identity-broker.token-exchanged'
  AND outcome = 'success'
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY receiving_agent;
```

The result is the exact set of receiving agents with successful exchanges in that interval. Multiple successful exchanges for one agent produce one set entry. The result excludes failed exchanges and unrelated subjects. This query does not require deleted business objects.

## Read occurrences with stable pagination

With the subject and UTC interval set, run this query:

```sql
SELECT occurred_at, id, type, outcome, envelope
FROM public.business_events
WHERE subject = :'principal'
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY occurred_at, id
LIMIT 200;
```

For the next page, retain the last row's `occurred_at` and `id`. Keep the same filters. Add this predicate:

```sql
AND (occurred_at, id) > (:'cursor_time'::timestamptz, :'cursor_id'::uuid)
```

Do not use `OFFSET` for long investigations. Repository queries default to 200 records and permit at most 1000 records per page.

## Select events without a subject

Administrative resource actions, background actions without an affected principal, and pre-authentication failures can have a null subject.

To select these events, use an explicit selector:

```sql
SELECT occurred_at, id, type, envelope
FROM public.business_events
WHERE subject IS NULL
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY occurred_at, id
LIMIT 200;
```

The internal repository equivalent is `BusinessEventSubject{NoSubject: true}`. An empty principal is invalid. A null subject differs from an unavailable actor ID. A system expiry event can still identify its affected subject.

For `signing-key-promoted`, `data.activates_at` matches the persisted key's eligibility time, including PostgreSQL's microsecond precision. It is separate from `occurred_at`, which records the current-key selection time.

## Interpret the caller

The subject is the affected principal, not the initiator. Client-assertion token exchange and impersonation identify the verified initiating client as a `gateway` actor. The `agent_id` identifies the receiving agent.

Approval consumption uses subject-token-only authentication. Its `gateway` actor ID is null, not the user or the approval creator. The `on_behalf_of` field retains the established represented principal.

Activity views and SSF initiating-entity mappings must use the actor. They must not fill unknown caller IDs from subject or resource references.

## Interpret identities and references

The ledger uses these identities and references:

- **Subject**: The affected principal established by the domain workflow
- **Actor ID**: The authenticated initiating caller, if available
- **On behalf of**: The represented principal after the domain establishes delegation
- **Gateway reference**: The verified gateway client associated with the workflow
- **Resource references**: Safe agent, service, grant, session, approval, and permission-set identifiers.

Unavailable nullable identities remain null. The slog sentinel `anonymous` is not an authenticated identity. Request parameters and unverified JWT claims do not establish ledger identities.

Automatic session refresh during token exchange keeps the session principal as its subject. Its actor identifies the verified initiating client. The event populates `on_behalf_of` only after the exchange establishes delegation.

The request trace comes from authoritative transport capture. A stored span identifies the actual HTTP server span, not an inbound parent or a later domain child span. Without a matching valid request span, the envelope omits the span field.

The ledger retains known references after business-object deletion. It contains no access tokens, refresh tokens, client secrets, assertions, authorization codes, PKCE verifiers, private keys, or credential hashes.

Reasons come from fixed recipes. User-agent values contain only an allowed family, not raw versions or comments.

## Refusals and failures

If the broker refuses an exchange because the third-party session lacks required scopes, it records one `token-exchange-denied` event. This event has outcome `denied` and `data.reason_code` equal to `authorization_failed`. It preserves the known principal, initiating caller, grant, agent, service, and session references. It does not also record `token-request-failed` for the same refusal.

The token endpoint still returns `invalid_grant` and the existing reauthentication URI. A generic token failure remains distinct from an authentication or authorization refusal.

## Event contracts and evolution

The published [schemas](https://github.com/zalando-incubator/agentic-identity-broker/tree/main/api/events/v1) define closed envelopes and type-specific data. The [synthetic examples](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/api/events/v1/examples.json) contain no credentials. The [catalogue contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/contracts/events.md) defines all 28 initial types.

Type names always use `agentic-identity-broker.<event-name>`. The source is always `urn:agentic-identity-broker:broker`. Neither value depends on a deployment or hosting organization.

Offline schema registration adds a reviewed type without a database migration. It cannot redefine an existing type or change its meaning. Unknown types, invalid payloads, remote schema references, and flattened-name collisions fail closed.

Compatible changes preserve existing fields and meanings. Breaking semantic changes require a new type and coordinated consumer updates.

Retained JSON numbers preserve signed 64-bit integers in scalar and array fields. This preservation includes values beyond float64's exact-integer range. Numeric validation and telemetry use the same representability rules. Supported numeric fields include Go integer widths, finite float32/float64 values, and lossless `json.Number` values. For export as doubles, float32 values retain their shortest JSON decimal.

OTLP arrays have one scalar type. Mixed integer/fraction arrays require lossless conversion of every integer to a double. If a value requires rounding for OTLP, validation rejects it before commit. Byte slices remain JSON strings, not numeric arrays.

## Telemetry copies and recovery

Additional copies require `telemetry.enabled`, `telemetry.logs.enabled`, and `business_events.telemetry_copy_enabled` together. A disabled switch does not disable mandatory recording, retention, or existing slog behavior.

With copies enabled, occurrences commit payload-free pending references with their events. The background worker scans at startup and every second, with at most 100 candidates. Collector I/O does not run in business commits. Initialization and export failures leave recoverable pending work. Failed attempts retry after 30 seconds with the original event ID.

The dedicated non-global scope is `agentic-identity-broker.ledger`. EventName is the full event type. The full envelope appears as literal flat attributes. Sorted `_ledger.null_fields` and `_ledger.empty_objects` markers preserve null and empty-object distinctions. Body values remain empty. Telemetry cannot silently truncate required attributes.

Successful synchronous export and committed acknowledgement remove a reference. A crash or failed acknowledgement after export can produce duplicate copies with the same event ID. Consumers must deduplicate by event ID, not occurrence type or subject.

Occurrences from a disabled period have no references and receive no historical backfill. After enablement resumes, the worker processes only retained references created before disablement. Erasure and retention remove the replay source and wait for in-flight synchronous attempts. The broker cannot retract packets already dispatched to an external receiver.

The broker drains HTTP requests before it cancels delivery and memory maintenance. It closes the ledger provider before existing telemetry and storage. Unfinished work stays pending. PostgreSQL survives process restart. Memory provides only process-local recovery.

## Evidence and limits

The [Validation quickstart](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/quickstart.md) records actual commands and results. A passed catalogue journey does not establish retention or recovery acceptance. The functional release criteria cover all 20 active scenarios, including the PostgreSQL lane. US4-AS4 and the FR-008/SC-007 performance gate are retired.

The [performance report](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/performance-results.md) retains historical measurements. The user removed the feature-specific diagnostic tools, without a replacement suite. The 5 ms p99 goal remains unverified and non-blocking. The feature requires no deployment profile or numeric performance pass.
