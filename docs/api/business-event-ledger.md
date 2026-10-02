# Business Event Ledger

The broker records credential-free business facts in an immutable ledger. Business mutations and their facts share one transaction. Recording failure rejects the operation before successful responses or credentials leave the broker.

This feature adds no HTTP or CLI read, erase, export, or ingestion endpoint. The existing [end-user](../../api/enduser/openapi.yaml) and [admin](../../api/admin/openapi.yaml) contracts remain unchanged.

## Operational access

Investigations use an authorized PostgreSQL connection. The optional Helm value `migration.grants.businessEvents.readerRole` names an existing reader role. Its empty default grants nothing. The migration owner manages access. Broker runtime credentials are not operator credentials.

The reader needs `SELECT` on `public.business_events`. Operational role grants and erasure ownership follow the [storage contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/contracts/storage.md#privileges-and-immutability).

## Find the receiving agents

In `psql`, set the exact subject and UTC interval. The interval includes its start and excludes its end.

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

The result is the exact set of receiving agents with successful exchanges in that interval. Multiple successful exchanges for one agent produce one set entry. Failed exchanges and unrelated subjects do not belong to that result. Deleted business objects are not necessary for this query.

## Read occurrences with stable pagination

```sql
SELECT occurred_at, id, type, outcome, envelope
FROM public.business_events
WHERE subject = :'principal'
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY occurred_at, id
LIMIT 200;
```

For the next page, retain the last row's `occurred_at` and `id`. Keep the same filters and add this predicate:

```sql
AND (occurred_at, id) > (:'cursor_time'::timestamptz, :'cursor_id'::uuid)
```

Do not use `OFFSET` for long investigations. Repository queries default to 200 records and permit at most 1000 records per page.

## Select events without a subject

Administrative resource actions, background actions without an affected principal, and pre-authentication failures can have a null subject. Use an explicit selector:

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

## Interpret identities and references

- **Subject**: The affected principal established by the domain workflow.
- **Actor ID**: The authenticated initiating caller, when available.
- **On behalf of**: The represented principal after the domain establishes delegation.
- **Gateway reference**: The verified gateway client associated with the workflow.
- **Resource references**: Safe agent, service, grant, session, approval, and permission-set identifiers.

Unavailable nullable identities remain null. The slog sentinel `anonymous` is not an authenticated identity. Request parameters and unverified JWT claims do not establish ledger identities.

Automatic session refresh during token exchange keeps the session principal as its subject. Its actor identifies the verified initiating client, with `on_behalf_of` populated only after the exchange establishes delegation.

The request trace comes from authoritative transport capture. A stored span identifies the actual HTTP server span, not an inbound parent or a later domain child span. Without a matching valid request span, the envelope omits the span field.

The ledger retains known references after business-object deletion. It contains no access tokens, refresh tokens, client secrets, assertions, authorization codes, PKCE verifiers, private keys, or credential hashes. Reasons come from fixed recipes. User-agent values contain only an allowed family, not raw versions or comments.

## Event contracts and evolution

The published [schemas](https://github.com/zalando-incubator/agentic-identity-broker/tree/main/api/events/v1) define closed envelopes and type-specific data. The [synthetic examples](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/api/events/v1/examples.json) contain no credentials. The [catalogue contract](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/contracts/events.md) defines all 28 initial types.

Type names always use `agentic-identity-broker.<event-name>`. The source is always `urn:agentic-identity-broker:broker`. Neither value depends on a deployment or hosting organization.

Offline schema registration adds a reviewed type without a database migration. It cannot redefine an existing type or change its meaning. Unknown types, invalid payloads, remote schema references, and flattened-name collisions fail closed. Compatible changes preserve existing fields and meanings. Breaking semantic changes require a new type and coordinated consumer updates.

## Telemetry copies and recovery

Additional copies require `telemetry.enabled`, `telemetry.logs.enabled`, and `business_events.telemetry_copy_enabled` together. Disabling one switch does not disable mandatory recording, retention, or existing slog behavior.

Enabled occurrences commit payload-free pending references with their events. The background worker scans at startup and every second, with at most 100 candidates. Collector I/O does not run in business commits. Initialization and export failures leave recoverable pending work. Failed attempts retry after 30 seconds with the original event ID.

The dedicated non-global scope is `agentic-identity-broker.ledger`. EventName is the full event type. The complete envelope appears as literal flat attributes. Sorted `_ledger.null_fields` and `_ledger.empty_objects` markers preserve null and empty-object distinctions. Body values remain empty. Required attributes cannot be silently truncated.

Successful synchronous export and committed acknowledgement remove a reference. A crash or failed acknowledgement after export can produce duplicate copies with the same event ID. Consumers must deduplicate by event ID, not occurrence type or subject.

Disabled-period occurrences have no references and receive no historical backfill. Re-enabling resumes only retained references created before disablement. Erasure and retention remove their replay source and wait for in-flight synchronous attempts. The broker cannot retract packets already handed to an external receiver.

The broker drains HTTP requests before cancelling delivery and memory maintenance. It closes the ledger provider before existing telemetry and storage. Unfinished work stays pending. PostgreSQL survives process restart. Memory provides only process-local recovery.

## Evidence and limits

[Validation quickstart](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/quickstart.md) records actual commands and acceptance results. A passed catalogue journey does not establish retention, recovery, or performance acceptance. The release gate requires all 21 scenarios and the approved deployment-profile measurement.

The [performance report](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/performance-results.md) records reference measurements and the unmet deployment-profile gate.
