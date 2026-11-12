# Quickstart: Business Event Ledger

**Feature**: `056-business-event-ledger` | **Spec**: [spec.md](./spec.md)

This guide verifies the feature by hand after delivery step 5 (first use case) and step 6
(retention and erasure). Names follow the [contracts](./contracts/). Container-backed commands
need Docker; do not run them in a sandbox without it.

## 1. Enable the ledger (after step 5)

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
attributes.

## 4. Check what must not be stored

```sh
curl -i -X DELETE -H 'X-Remote-User: alice@example.com' \
  "http://localhost:8000/api/consent/agents/${AGENT_ID}/grants"
# HTTP/1.1 404 Not Found — the ledger has no new row (User Story 1, Scenario 2)
```

Set `business_events.enabled: false`, restart, and repeat steps 2 and 3: the stdout record is the
same and the ledger has no new row (Scenario 3).

## 5. Check a failed write (Scenario 4)

Stop PostgreSQL access to the table only (for example `REVOKE INSERT ON business_events FROM
<broker role>`), then revoke a grant. Expected: `204`, the `grant revoked` record, one
`business event write failed` record with `event=business_event.ledger.write_failed`, and
`business_events.write_failures` increased by one. Grant the privilege again afterwards.

## 6. Retention and erasure (after step 6)

```yaml
business_events:
  enabled: true
  retention:
    period: 24h
    interval: 1m
```

```sql
UPDATE business_events SET recorded_at = now() - interval '2 days' WHERE principal = 'alice@example.com';
```

Within one minute the rows are gone and stdout has `business events expired` with `row_count`.

Erasure through the admin server (send it through the admin proxy in a real deployment):

```sh
curl -i -X POST -H 'Content-Type: application/json' -H 'X-Remote-User: operator@example.com' \
  -d '{"principal":"alice@example.com"}' \
  http://localhost:14000/api/business-events/erasure
# HTTP/1.1 200 OK
# {"deleted_count": 3, "redacted_count": 0}
```

## 7. Onboard another event (User Story 2)

Follow [checklists/onboarding.md](./checklists/onboarding.md). The whole change is:

```go
// internal/domain/consent/events.go
var GrantCreated = bizevent.MustDefine("consent.grant.created", bizevent.OwnerVisible())

// internal/domain/consent/service.go
s.logger.InfoContext(ctx, "grant created",
	"event", GrantCreated,
	"principal", grant.Principal,
	"agent_id", grant.AgentID,
	"grant_id", grant.ID)
```

## Automated verification

```sh
just check                    # format, vet, lint
just test                     # unit tests, includes handler and memory adapter tests
just test-integration         # self-contained integration tests
just test-e2e-backend         # includes tests/e2e/business_event_ledger_test.go
just test-integration-infra   # PostgreSQL adapter and migration 048 (containers)
```
