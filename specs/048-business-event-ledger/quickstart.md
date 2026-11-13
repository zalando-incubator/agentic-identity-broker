# Quickstart: Business Event Ledger

**Feature**: `048-business-event-ledger` | **Spec**: [spec.md](./spec.md)

This guide verifies the feature by hand after Phase 3 of [tasks.md](./tasks.md). Names follow the [contracts](./contracts/). Container-backed commands
need Docker; do not run them in a sandbox without it.

Use a local PostgreSQL database that holds test data only. The SQL statements in sections 3, 5,
and 6 need the PostgreSQL storage adapter; the memory adapter keeps the ledger in the process and
has no query interface. Section 6 changes and deletes rows.

## 1. Enable the ledger

```yaml
# config.yaml, added to a working local configuration
business_events:
  enabled: true
```

```sh
just run
```

On PostgreSQL, apply migration `048_create_business_events` first, with the migration job or
image that the deployment uses (ADR 009). The memory storage adapter needs no migration.

## 2. Produce an event

Create a grant for an agent in the consent UI, then revoke it:

```sh
curl -i -X DELETE \
  -H 'X-Remote-User: alice@example.com' \
  "http://localhost:8000/api/consent/agents/${AGENT_ID}/grants"
# HTTP/1.1 204 No Content
```

## 3. Check the three views of the same fact

Stdout has the audit record:

```json
{"level":"INFO","msg":"grant revoked","event":"consent.grant.revoked","principal":"alice@example.com","agent_id":"...","grant_id":"...","trace_id":"...","actor":"alice@example.com"}
```

The ledger has one row with the same identifier and attributes:

```sql
SELECT id, event, occurred_at, principal, actor, trace_id, attributes
FROM business_events
WHERE principal = 'alice@example.com'
ORDER BY occurred_at DESC, id DESC
LIMIT 5;
```

With telemetry enabled, the OpenTelemetry log record with the same `trace_id` has the same
attributes. These examples use the current identity principal policy. Under any future policy,
the ledger must contain the same rendered principal values as the operational sinks, not raw
identities; SQL filters must use stored policy output.

## 4. Check what must not be stored

```sh
curl -i -X DELETE -H 'X-Remote-User: alice@example.com' \
  "http://localhost:8000/api/consent/agents/${AGENT_ID}/grants"
# HTTP/1.1 404 Not Found — the ledger still has exactly one row (User Story 1, Scenario 2)
```

Set `business_events.enabled: false`, restart, and repeat sections 2 and 3: the stdout record is the
same and the ledger has no new row (Scenario 3).

## 5. Check a failed write (Scenario 4)

Set `business_events.enabled: true` again, restart the broker, and create a new grant for the
agent. Then stop PostgreSQL access to the table only (for example `REVOKE INSERT ON business_events FROM
<broker role>`), then revoke a grant. Expected: `204`, the `grant revoked` record, one
`business event write failed` record with `event=business_event.ledger.write_failed`, and
`business_events.write_failures` increased by one. Grant the privilege again afterwards.

## 6. Retention (User Story 2)

Apply this configuration to the broker's configuration file:

```yaml
business_events:
  enabled: true
  retention:
    period: 24h
    interval: 1m
```

Restart the broker to apply the 24-hour retention period and one-minute interval.
Then age the row:

```sql
-- Use the id of the row from section 3. Do not match by principal.
UPDATE business_events SET recorded_at = now() - interval '2 days' WHERE id = '<id from section 3>';
```

During healthy operation, the next scheduled run deletes the row within one minute plus that run's duration.
No log record reports the deletion. Failed runs retry at the next scheduled interval, with no fixed maximum row age (SC-005).
Set `business_events.enabled: false`, restart, and repeat with another old row: it stays, because
retention runs only with the ledger enabled.

## Automated verification

```sh
just check                    # format, vet, lint
just test                     # unit tests, includes handler and memory adapter tests
just test-integration         # self-contained integration tests
just test-e2e-backend         # existing non-container acceptance suites
just test-integration-infra   # PostgreSQL adapter and migration 048 (containers)
just test-e2e-docker          # all five ledger scenarios; test-local SQL inspection (containers)
just test-e2e-performance     # SC-004 measurement (containers)
```
