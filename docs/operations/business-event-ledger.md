# Business Event Ledger Operations

The ledger records credential-free business facts. Recording, retention, erasure, scheduling, and recoverable telemetry are implemented. Performance release acceptance remains open in the [validation gates](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/quickstart.md). This guide adds no HTTP or CLI read, erase, export, or ingestion endpoint.

## Ownership and prerequisites

PostgreSQL requires migration 036 and a five-minute maintenance scheduler. The migration owner owns ledger tables, partitions, and functions. Broker credentials provide runtime DML only. The broker must not own the database, public schema, ledger objects, or a role that inherits those ownership rights.

The broker role cannot update, delete, truncate, or directly write child partitions. It cannot invoke provisioning, maintenance, or erasure. The immutable-event trigger also rejects updates. Future partition creation removes inherited direct child privileges. Existing business-table permissions remain unchanged.

The migration owner must use TLS and a protected credential store. Do not put passwords in commands, manifests, or job logs. The broker Deployment must not receive the migration Secret.

## Install the scheduler

### Helm

For PostgreSQL with `migration.enabled: true`, the chart renders a maintenance CronJob. It uses `migration.grants.image`, migration credentials, and the migration ServiceAccount. Its schedule is `*/5 * * * *`, with `Forbid` concurrency. Each Job has a 120-second deadline and at most two retries. Resources and SSL connectivity follow the migration configuration.

The pre-install/pre-upgrade Job runs schema migrations, then drop-free provisioning. Optional grants run only when `migration.grants.enabled` is true. Disabling optional grants does not disable provisioning or maintenance. Disabling `migration.enabled` removes both Jobs, so an external scheduler and migration procedure then become mandatory.

The CronJob runs:

```sh
PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' \
  psql -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'
```

The template does not render `spec.suspend`. An operator suspension therefore survives `helm upgrade`.

### Bare-process or container deployments

Install PostgreSQL client tools and an equivalent five-minute scheduler. Configure a protected `broker-maintenance` PostgreSQL service for the migration owner. Put connection details in a service file and credentials in a protected password file. Set both file modes to `0600` and configure the required SSL mode.

Before the broker starts, run provisioning as the migration owner:

```sh
PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' \
  psql 'service=broker-maintenance' -v ON_ERROR_STOP=1 \
  -c 'SELECT public.business_event_provision_partitions();'
```

On Linux, a cron entry can use `flock` to prevent overlap:

```cron
*/5 * * * * PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' /usr/bin/flock -n /run/lock/broker-business-events.lock /usr/bin/psql 'service=broker-maintenance' -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'
```

Adjust executable and lock paths for the host. Route command failures and skipped overlapping runs to operational monitoring. The scheduler remains required when telemetry is disabled.

## Configure operational roles

The Helm values `migration.grants.businessEvents.readerRole` and `migration.grants.businessEvents.erasureRole` name existing roles. The chart does not create roles. Empty values grant nothing. Keep both roles separate from the broker and migration owner.

A custom `migration.grants.sql` replaces the complete default script. It must apply runtime restrictions and operational grants itself. The two role values do not supplement a custom script.

For non-Helm deployments, use an authorized `psql` session. Replace the example existing role names:

```sql
\set broker_role 'broker'
\set reader_role 'broker-ledger-reader'
\set erasure_role 'broker-ledger-eraser'

GRANT USAGE ON SCHEMA public TO :"broker_role", :"reader_role", :"erasure_role";
REVOKE CREATE ON SCHEMA public FROM PUBLIC, :"broker_role";
REVOKE ALL ON public.business_events, public.business_event_delivery_pending,
  public.business_event_policy FROM PUBLIC, :"broker_role";
GRANT SELECT, INSERT ON public.business_events TO :"broker_role";
GRANT SELECT, INSERT, UPDATE, DELETE ON public.business_event_delivery_pending TO :"broker_role";
GRANT SELECT, UPDATE ON public.business_event_policy TO :"broker_role";

SELECT format('REVOKE ALL ON TABLE %I.%I FROM PUBLIC, "%s"', n.nspname, c.relname,
              replace(:'broker_role', '"', '""'))
FROM pg_catalog.pg_inherits i
JOIN pg_catalog.pg_class c ON c.oid = i.inhrelid
JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
WHERE i.inhparent IN ('public.business_events'::regclass,
                     'public.business_event_delivery_pending'::regclass) \gexec

REVOKE ALL ON FUNCTION public.business_event_reject_update(),
  public.business_event_create_partition_pair(timestamptz),
  public.business_event_provision_partitions(),
  public.business_event_maintain_partitions(),
  public.business_event_erase_subject(text) FROM PUBLIC, :"broker_role";

ALTER DEFAULT PRIVILEGES REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC, :"broker_role";
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE EXECUTE ON FUNCTIONS FROM PUBLIC, :"broker_role";

GRANT SELECT ON public.business_events, public.business_event_delivery_pending TO :"reader_role";
GRANT EXECUTE ON FUNCTION public.business_event_erase_subject(text) TO :"erasure_role";
```

Apply restrictions after broad business-table grants. Run default-privilege commands as the migration owner. They affect future functions created by that owner, not existing business-table grants. Give non-ledger functions explicit execution grants when necessary. Runtime roles must not inherit forbidden privileges through another role.

The [investigation guide](../api/business-event-ledger.md) provides exact-subject SQL and stable pagination.

## Erase an exact subject

Use the erasure role with autocommit and the normal `READ COMMITTED` transaction isolation. Set bounded command timeouts before the call:

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30s';
\set subject 'principal-example'
SELECT public.business_event_erase_subject(:'subject');
```

The result counts deleted event rows. The function rejects null or blank selectors and uses exact subject equality, not actor matching or a wildcard. It removes associated delivery references across retained partitions. A repeated call returns zero. It creates no replacement event or identity tombstone.

The function acquires lifecycle shared and subject exclusive locks before row locks. It waits for earlier recorders and dispatch attempts. Statements after the wait see committed predecessors. With an explicit transaction, commit before reporting erasure complete:

```sql
BEGIN ISOLATION LEVEL READ COMMITTED;
SELECT public.business_event_erase_subject(:'subject');
COMMIT;
```

A recorder ordered after that commit can create a legitimate new occurrence. Business expiry markers remain, so lazy discovery cannot recreate an already-recognized old expiry. The ledger erases events by subject only. Other subjects remain unchanged, including records whose actor names the erased principal.

## Retention and partition health

Retention uses recorded time, not occurrence time. The default is `2160h`, or 90 days. Configure a positive Go duration such as `720h`, not `90d`. Invalid, zero, negative, or overflowing values reject startup. Positive fractions round upward: `1ns` becomes `1us`, and `1001ns` becomes `2us`.

Partitions cover six-hour UTC windows. Maintenance removes a complete event/reference pair only when its upper bound is no later than database time minus retention. It never deletes young rows early, including policies shorter than six hours. Normal grace is less than six hours plus five minutes, plus bounded execution. Removal must not remain overdue for 24 hours.

Provisioning creates current and future windows without dropping history. Maintenance repairs missing current/future pairs and catches up after downtime. Corrupt policy or historical partition metadata fails closed. There is no DEFAULT partition or runtime DDL fallback.

Monitor CronJob failures, missing successful sweeps, startup policy errors, and storage readiness. Alert well before the 24-hour removal limit. Investigate a stopped memory worker warning as an operational fault. Missing current event or delivery partitions fail PostgreSQL readiness and recording. Successful provisioning or maintenance restores readiness.

Memory applies the same logical boundary at startup and every five minutes. It has no physical partitions or process-restart durability. Neither telemetry switch affects recording or retention.

## Change retention safely

All replicas sharing a ledger must use the same startup-scoped retention value. Each policy write serializes with maintenance. The last successful startup write is authoritative.

CAUTION: Suspend maintenance before a policy change. An old shorter policy can delete history that an increased duration must retain.

1. Suspend the CronJob or external scheduler.
2. Upgrade every broker replica with the same new retention value.
3. Query the stored policy with migration credentials.
4. Verify the normalized duration for every replica's completed startup.
5. Resume the scheduler.

```sh
kubectl patch cronjob BROKER-business-events --type=merge -p '{"spec":{"suspend":true}}'
```

```sql
SELECT retention_microseconds FROM public.business_event_policy WHERE singleton;
```

For example, `720h` is `2592000000000` microseconds, and `2160h` is `7776000000000` microseconds.

```sh
kubectl patch cronjob BROKER-business-events --type=merge -p '{"spec":{"suspend":false}}'
```

Use the rendered CronJob name, which can be shortened for long release names. The pre-upgrade Job only provisions partitions and never sweeps under the superseded policy.

## Verify outage recovery

The copy worker uses the existing telemetry destination. It scans retained references at startup and every second, with at most 100 candidates. Collector work remains outside business commits. Failed initialization retries automatically. Failed exports retry after 30 seconds with the original ID.

For a disposable PostgreSQL dataset, stop the collector and complete a real broker workflow. Query the event and its payload-free pending reference with operational credentials. Restart the broker and collector. After the reference becomes due, verify the same event ID at the receiver and absence of its acknowledged reference.

```sql
\set event_id '00000000-0000-0000-0000-000000000000'
SELECT id, type, subject FROM public.business_events WHERE id = :'event_id'::uuid;
SELECT recorded_at, event_id, next_attempt_at
FROM public.business_event_delivery_pending WHERE event_id = :'event_id'::uuid;
```

An export followed by a crash before acknowledgement can produce duplicate copies with the same ID. Deduplicate receiver data by event ID. Do not treat a disabled copy switch as an outage. Disabled-period events have no references and no backfill. Re-enabling resumes only retained pre-disable references.

During shutdown, HTTP drains before delivery and memory maintenance stop. The ledger provider closes before existing telemetry and storage. Unfinished work remains pending. Memory recovery lasts only while the process lives.

## External copies, backups, and rollback

Ledger erasure cannot retract telemetry already handed to an external receiver. External telemetry retention and independent backups require their own deletion procedures. Do not claim system-wide identity deletion from a ledger operation.

CAUTION: Do not roll migration 036 down without a backup and explicit operator acknowledgement. The down migration removes ledger history, delivery references, policy, and expiry markers. It does not restore erased or retained-away events.

For rollback, stop new broker writes and maintenance first. Back up retained ledger data and business records. Record the data-loss acknowledgement. Apply the down migration only with migration ownership, then deploy the matching old binary. A ledger-aware binary must not run against a schema without its required tables and functions.
