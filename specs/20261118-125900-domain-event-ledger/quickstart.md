# Quickstart: Domain Event Ledger

**Feature**: `20261118-125900-domain-event-ledger` | **Spec**: [spec.md](./spec.md)

This guide verifies the feature by hand after Phase 3 of [tasks.md](./tasks.md). Names follow the [contracts](./contracts/). Container-backed commands
need Docker; do not run them in a sandbox without it.

Use a local PostgreSQL database that holds test data only. The SQL statements in sections 3, 5,
and 6 need the PostgreSQL storage adapter; the memory adapter keeps the ledger in the process and
has no query interface. Section 6 changes and deletes rows.

To export metrics or logs, enable `telemetry.enabled` and configure a reachable OTLP collector.
For the failed-write metric check in section 5, enable `telemetry.metrics.enabled` and configure the collector's metrics receiver.
Wait for the metrics export interval before comparing counters.
For the OpenTelemetry log comparison in section 3, enable `telemetry.logs.enabled` and configure the collector's logs receiver.
To compare an incoming `traceparent` ID, enable `telemetry.traces.enabled` and include `tracecontext` in `telemetry.traces.propagators`.
See the [telemetry configuration reference](../../docs/configuration.md#observability--opentelemetry) for exporter settings.

## 1. Enable the ledger

```yaml
# config.yaml, added to a working local configuration
log:
  level: info
  format: json
domain_events:
  enabled: true
```

```sh
just run
```

On PostgreSQL, apply migration `049_create_domain_events` first, with the migration job or
image that the deployment uses (`009-separate-migration-docker-image`). The memory storage adapter needs no migration.

## 2. Produce an event

As Alice, create a grant for an agent in the consent UI. List her active delegations:

```sh
curl -sS -H 'X-Remote-User: alice@example.com' \
  'http://localhost:8000/api/consent/agents'
```

Find the selected agent in the response. In the same shell, replace the example UUID with that agent's `agentId`:

```sh
export AGENT_ID='550e8400-e29b-41d4-a716-446655440000'
```

Revoke the grant:

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
FROM domain_events
WHERE principal = 'alice@example.com'
ORDER BY occurred_at DESC, id DESC
LIMIT 5;
```

With OTLP log export enabled, compare the event, agent ID, grant ID, trace ID, and rendered identities.
Compare structured attributes with stdout JSON, not OTLP. The OTLP bridge formats the permission-set entries as strings.
These examples use the current identity principal policy. If that policy changes, use its rendered principal in SQL filters.

Compare `occurred_at` with the log timestamp under [the timestamp precision contract](./data-model.md#timestamp-precision):
PostgreSQL truncates occurrence nanoseconds to microseconds; the handler and memory adapter do not.

## 4. Check what must not be stored

```sh
curl -i -X DELETE -H 'X-Remote-User: alice@example.com' \
  "http://localhost:8000/api/consent/agents/${AGENT_ID}/grants"
# HTTP/1.1 404 Not Found — the ledger still has exactly one row (User Story 1, Scenario 2)
```

Set `domain_events.enabled: false`, restart, and repeat sections 2 and 3: the stdout record is the
same and the ledger has no new row (Scenario 3).

## 5. Check a failed write (Scenario 4)

Set `domain_events.enabled: true` again.
Restart the broker and create a new grant for the agent.
Install this failure trigger in the local test database with a role that can create functions and triggers:

```sql
CREATE FUNCTION reject_domain_event_insert() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION 'domain event insert rejected for manual verification';
END;
$$;

CREATE TRIGGER reject_domain_event_insert
BEFORE INSERT ON domain_events
FOR EACH ROW EXECUTE FUNCTION reject_domain_event_insert();
```

The trigger rejects inserts even for a PostgreSQL superuser, unlike `REVOKE INSERT` (T019).
Revoke the grant as in section 2.
Expected: `204`, the `grant revoked` record, and one failure record with
`event=domain_event.ledger.write_failed` and `error_kind=unknown`.
The `domain_events.write_failures{error_kind="unknown"}` series increases by one, and
`domain_events.write.duration{outcome="failed"}` receives one measurement.
The grant is gone, and the ledger has no row for this revocation.

After the check, remove the trigger and its function, even if the expected signals are absent:

```sql
DROP TRIGGER reject_domain_event_insert ON domain_events;
DROP FUNCTION reject_domain_event_insert();
```

## 6. Retention (User Story 2)

Apply this configuration to the broker's configuration file:

```yaml
domain_events:
  enabled: true
  retention:
    period: 24h
    interval: 1m
```

Restart the broker to apply the 24-hour retention period and one-minute interval.
Then age the row:

```sql
-- Use the id of the row from section 3. Do not match by principal.
UPDATE domain_events SET recorded_at = now() - interval '2 days' WHERE id = '<id from section 3>';
```

During healthy operation, the next scheduled run deletes the row within one minute plus that run's duration.
No log record reports the deletion. Failed runs retry at the next scheduled interval, with no fixed maximum row age (SC-005).
While the ledger is still enabled, create and revoke a new test grant. Record the ID of its ledger row.
Set `domain_events.enabled: false` and restart the broker. Age the new row with the SQL above using its ID
and the test database role that can update `recorded_at`. Wait longer than the configured interval.
Verify that the row remains. The disabled ledger does not run retention.

## Automated verification

```sh
just check                    # format, vet, lint
just test                     # unit tests, includes handler and memory adapter tests
just test-integration         # self-contained integration tests
just test-e2e-backend         # existing non-container acceptance suites
just test-integration-infra   # PostgreSQL adapter and migration 049 (containers)
just test-e2e-docker          # all six ledger scenarios; test-local SQL inspection (containers)
just test-e2e-performance     # SC-004 measurement (containers)
```

For SC-004 acceptance, follow the [measurement protocol and environment record](./plan.md#sc-004-measurement-protocol).
Record the environment and each acceptance run's results in the PR description, including failed runs.
A latency result without the environment record does not establish acceptance.

The commands above are focused feature checks. Before opening the pull request, also pass
`just check` and `just verify` as required by [CONTRIBUTING.md](../../CONTRIBUTING.md#before-committing).
Keep the separate performance run: `just verify` excludes performance-labelled tests.
