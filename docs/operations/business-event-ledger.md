# Business Event Ledger Operations

The ledger records credential-free business facts. The implementation includes recording, retention, erasure, scheduling, and recoverable telemetry. The [validation guide](https://github.com/zalando-incubator/agentic-identity-broker/blob/main/specs/048-business-event-ledger/quickstart.md) records functional results for 20 active scenarios.

The user removed the performance gate, deployment-profile requirement, and feature-specific diagnostic tools. All 20 functional scenarios remain required in shared lanes. This guide adds no HTTP or CLI endpoint for read, erase, export, or ingestion operations.

## Ownership and prerequisites

PostgreSQL requires migration 036 and a five-minute maintenance scheduler. The migration owner owns ledger tables, partitions, and functions. Broker credentials provide runtime DML only. The broker must not own the database, public schema, or ledger objects. It must not own a role that inherits those ownership rights.

The broker role cannot update, delete, truncate, or directly write child partitions. It cannot run provisioning, maintenance, or erasure. The immutable-event trigger also rejects updates. Creation of future partitions removes inherited direct child privileges. Existing business-table permissions remain unchanged.

Grant `valid_until` values are timezone-less UTC wall timestamps; expiration markers are UTC instants. Expiration discovery and recognition explicitly convert between these types and do not require the PostgreSQL session timezone to be UTC.

The migration owner must use TLS and a protected credential store. Do not put passwords in commands, manifests, or job logs. The broker Deployment must not receive the migration Secret.

### PostgreSQL connection pooling for refresh

Connect the broker directly to PostgreSQL or use a pooler in session mode. Do not use transaction-mode or statement-mode pooling.

Third-party refresh holds a session-level advisory lock on one backend across the provider call and the short persistence transaction. If a pooler switches backends, it can leave this lock behind. This lock can block later refreshes for the same principal and service.

Refresh does not hold ledger lifecycle or subject gates during provider I/O. It still occupies one application pool connection. Lock acquisition has no adapter-set `lock_timeout`. The operation's context deadline or cancellation bounds the wait. Automatic refresh supplies an operation deadline. Read and write timeout configuration does not independently bound this lock wait.

Unlock uses the storage write timeout on a separate cleanup context. If acquisition is uncertain or unlock fails, the broker discards the connection. This cleanup cannot make transaction-mode pooling safe. The pooler can retain a different locked backend.


## Install the scheduler

### Helm

For PostgreSQL with `migration.enabled: true`, the chart renders a maintenance CronJob. It uses `migration.grants.image`, migration credentials, and the migration ServiceAccount. Its schedule is `*/5 * * * *`, with `Forbid` concurrency. Each Job has a 120-second deadline and at most two retries. Resources and SSL connectivity use the migration configuration.

The pre-install/pre-upgrade Job runs schema migrations, then provisioning without partition deletion. Optional grants run only with `migration.grants.enabled` set to true. Disabled optional grants do not disable provisioning or maintenance. If `migration.enabled` is disabled, the chart removes both Jobs. An external scheduler and migration procedure then become mandatory.

The CronJob runs this command with migration credentials:

```sh
PGOPTIONS='-c lock_timeout=5s -c statement_timeout=30s' \
  psql -v ON_ERROR_STOP=1 -c 'SELECT public.business_event_maintain_partitions();'
```

Erasure and maintenance require a caller-configured `statement_timeout` from `1ms` through `30s`.
Set it on the connection or in a separate command before the function call.
A function-local setting cannot start the deadline for its caller statement.
`lock_timeout` bounds lock waits, not total execution.

The template does not render `spec.suspend`. Thus, an operator suspension survives `helm upgrade`.

### Bare-process or container deployments

Install PostgreSQL client tools. Install an equivalent five-minute scheduler.

Configure a protected `broker-maintenance` PostgreSQL service for the migration owner. Put connection details in a service file. Put credentials in a protected password file. Set both file modes to `0600`. Configure the required SSL mode.

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

Adjust the executable and lock paths for the host. Route command failures and skipped overlapping runs to operational monitoring.

The scheduler remains required with telemetry disabled.

## Configure operational roles

The Helm values `migration.grants.businessEvents.readerRole` and `migration.grants.businessEvents.erasureRole` name existing roles. The chart does not create roles. Empty values grant nothing.

Keep both roles separate from the broker and migration owner.

A custom `migration.grants.sql` replaces the full default script. It must apply runtime restrictions and operational grants itself. The two role values do not supplement a custom script.

For non-Helm deployments, use an authorized `psql` session. Before you run this SQL, replace the example role names with existing roles.

After broad business-table grants, apply the restrictions. Run default-privilege commands as the migration owner. For non-ledger functions that require access, give explicit execution grants.

Default-privilege commands affect future functions that the migration owner creates, not existing business-table grants. Runtime roles must not inherit forbidden privileges through another role.

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

The [investigation guide](../api/business-event-ledger.md) provides exact-subject SQL and stable pagination.

## Erase an exact subject

The ledger erases events by subject only. Other subjects remain unchanged, including records whose actor names the erased principal.

The result counts deleted event rows. The function rejects null or blank selectors. It uses exact subject equality, not actor matching or a wildcard. It removes associated delivery references across retained partitions. A repeated call returns zero. It creates no replacement event or identity tombstone.

The function acquires shared lifecycle locks, then exclusive subject locks, before row locks. It waits for earlier recorders and dispatch attempts. After the wait, statements see committed predecessors.

Use the erasure role with autocommit and `READ COMMITTED` isolation.
The function rejects `REPEATABLE READ`, `SERIALIZABLE`, and other isolation settings before it acquires barriers.
Before the call, set bounded command timeouts:

```sql
SET default_transaction_isolation = 'read committed';
SET lock_timeout = '5s';
SET statement_timeout = '30s';
\set subject 'principal-example'
SELECT public.business_event_erase_subject(:'subject');
```

With an explicit transaction, commit before you report completed erasure:

```sql
BEGIN ISOLATION LEVEL READ COMMITTED;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '30s';
SELECT public.business_event_erase_subject(:'subject');
COMMIT;
```

A recorder ordered after that commit can create a legitimate new occurrence. Business expiry markers remain. Thus, lazy discovery cannot recreate an already-recognized old expiry.

## Retention and partition health

Retention uses recorded time, not occurrence time. The default is `2160h`, or 90 days. Invalid, zero, negative, or overflowing values reject startup. Positive fractions round upward: `1ns` becomes `1us`, and `1001ns` becomes `2us`.

Configure a positive Go duration such as `720h`, not `90d`.

Partitions cover six-hour UTC windows. If a pair's upper bound is no later than database time minus retention, maintenance removes the complete event/reference pair. Maintenance never deletes young rows early, including policies shorter than six hours. Normal grace is less than six hours plus five minutes, plus bounded execution. Removal must not remain overdue for 24 hours.

Provisioning creates current and future windows without deleting history. Maintenance repairs missing current/future pairs and catches up after downtime. Corrupt policy or historical partition metadata fails closed. There is no DEFAULT partition or runtime DDL fallback.

Monitor CronJob failures, missing successful sweeps, startup policy errors, and storage readiness. Alert well before the 24-hour removal limit. Treat a warning about a stopped memory worker as an operational fault. Investigate that fault.

Missing current event or delivery partitions fail PostgreSQL readiness and recording. Successful provisioning or maintenance restores readiness.

Memory applies the same logical boundary at startup and every five minutes. It has no physical partitions or durability across process restarts. Neither telemetry switch affects recording or retention.

## Change retention safely

All replicas sharing a ledger must use the same startup-scoped retention value. Each policy write serializes with maintenance. The last successful startup write is authoritative.

CAUTION: Suspend maintenance before a policy change. An old shorter policy can delete history that an increased duration must retain.

To change retention safely, use this sequence:

1. Suspend the CronJob or external scheduler.
2. Upgrade every broker replica with the same new retention value.
3. Query the stored policy with migration credentials.
4. After each replica completes startup, verify its normalized duration.
5. Resume the scheduler.

For Helm, use the rendered CronJob name. Long release names can cause the chart to shorten this name.

To suspend the CronJob, run this command:

```sh
kubectl patch cronjob BROKER-business-events --type=merge -p '{"spec":{"suspend":true}}'
```

With migration credentials, query the stored policy:

```sql
SELECT retention_microseconds FROM public.business_event_policy WHERE singleton;
```

For example, `720h` is `2592000000000` microseconds, and `2160h` is `7776000000000` microseconds.

After every replica completes startup with the verified new policy, resume the CronJob:

```sh
kubectl patch cronjob BROKER-business-events --type=merge -p '{"spec":{"suspend":false}}'
```

The pre-upgrade Job only provisions partitions. It never sweeps under the superseded policy.

## Verify outage recovery

The copy worker uses the existing telemetry destination. It scans retained references at startup and every second, with at most 100 candidates. Collector work remains outside business commits. Failed initialization retries automatically. Failed exports retry after 30 seconds with the original ID.

An exporter deadline also counts as a failed attempt. PostgreSQL keeps the deletion barriers through a bounded one-second cleanup for the retry write. Then it releases the barriers for erasure and maintenance. A hanging collector must not leave its reference immediately due on every one-second worker scan.

For a disposable PostgreSQL dataset, stop the collector. Complete a real broker workflow. With operational credentials, query the event and its payload-free pending reference:

```sql
\set event_id '00000000-0000-0000-0000-000000000000'
SELECT id, type, subject FROM public.business_events WHERE id = :'event_id'::uuid;
SELECT recorded_at, event_id, next_attempt_at
FROM public.business_event_delivery_pending WHERE event_id = :'event_id'::uuid;
```

Restart the broker and collector. After the reference becomes due, verify the same event ID at the receiver. Verify that its acknowledged reference is absent.

An export followed by a crash before acknowledgement can produce duplicate copies with the same ID.

Deduplicate receiver data by event ID. Do not treat a disabled copy switch as an outage.

Events from a disabled period have no references and no backfill. After enablement resumes, the worker processes only retained references from before disablement.

During shutdown, HTTP drains before delivery and memory maintenance stop. The ledger provider closes before existing telemetry and storage. Unfinished work remains pending. Memory recovery lasts only while the process lives.

## External copies, backups, and rollback

Ledger erasure cannot retract telemetry already dispatched to an external receiver. External telemetry retention and independent backups require their own deletion procedures.

Do not claim system-wide identity deletion from a ledger operation.

CAUTION: Do not roll migration 036 down without a backup and explicit operator acknowledgement. The down migration removes ledger history, delivery references, policy, and expiry markers. It does not restore erased events or events removed by retention.

For rollback, stop new broker writes and maintenance first. Back up retained ledger data and business records. Record the data-loss acknowledgement. With migration ownership, apply the down migration. Then deploy the matching old binary.

A ledger-aware binary must not run against a schema without its required tables and functions.
