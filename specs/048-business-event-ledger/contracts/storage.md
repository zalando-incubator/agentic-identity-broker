# Storage and Operational Contracts

The broker implements these internal/operational interfaces without new HTTP endpoints.
The domain service validates the registry, authorization context, and lifecycle semantics.
Handlers never append directly to repositories.
The [validation evidence](../quickstart.md) records results and retirement of the performance gate, deployment profile, and feature-specific diagnostic tools. All 20 functional scenarios remain required in shared lanes. The accepted storage/security design is unchanged.

## Stakeholder review

**Approved**: 2026-09-27, in the implementation conversation.
**Reference**: The user selected `Approve both contracts` for this document and `events.md`.
The approval covers atomic recording, exact-subject access, partition retention, erasure, and deletion-safe recovery.
Operational access remains internal or database-based.
This contract adds no HTTP or CLI read/erase API.
Existing HTTP responses remain unchanged.
Additional public contract changes require separate confirmation.

[ADR 039](../../../adrs/039-business-event-ledger.md) records maintainer Jan Brennenstuhl's design acceptance on 2026-09-26.
It is binding, but its acceptance alone does not approve public API changes.

**Database design review**: T013, 2026-09-27, confirms the paired-partition, expiry-marker, lock-order, and privilege design against ADRs 004, 009, and 039.
Migration 036 contains this feature.
Rebase moved it from 035, which main uses for CIMD authentication.

Provisioning never drops partitions.
Production rollback requires separate operator acknowledgement of ledger-history loss and the matching old binary.
This design review applied no migration.

## Ports and transaction ownership

All repository interfaces live in `internal/ports/storage.go`.
Shared event/query/reference value types live in `internal/domain/model/`.
Use the existing storage factory accessors and builder injection.

| Contract | Operations and behavior |
|---|---|
| StorageTransactionManager | Generalizes the existing BeginTX/Commit/Rollback protocol. Transaction scopes distinguish owning and joined handles. Only the owner commits physically. Joined rollback marks the owner rollback-only. |
| BusinessEventRepository | Appends a validated event in the ambient transaction. Query requires a subject selector (exact principal or explicit no-subject), with optional filters and a cursor. Get uses `(recorded_at,id)`. There is no Update or generic Delete. |
| BusinessEventLifecycleRepository | EraseSubject, ApplyRetention (logical memory operation), SetRetentionPolicy. PostgreSQL ApplyRetention is available only to the maintenance owner, never runtime wiring. |
| BusinessEventDeliveryRepository | ListDue returns reference identifiers. DispatchOne uses barriers and a synchronous callback. It returns no payload outside the barrier for later delivery. |
| BusinessEventRecorder | Domain service contract for recording registered facts and independent outcomes. Recipes determine identity, outcome, and allowed data, not the HTTP handler. |
| UserGrantExpirationRepository, ToolApprovalExpirationRepository | Separate ISP interfaces in the existing grant and approval adapters. `UserGrantRepository` already exceeds the method budget. These interfaces list expired objects whose `expiration_recorded_for` differs from their effective expiry. They conditionally set that marker in the ambient transaction. They report whether this call recognized the expiry. |

Grant delegation reads use `ListUnrecordedExpiredForPrincipal` on the grant expiration facet.
The facet exposes scoped discovery and conditional recognition only; services obtain it from their existing CRUD repository rather than a duplicate factory accessor.
Public grant filtering remains unchanged, and there is no new expiration scheduler.

Apply the exact-principal predicate before the bounded limit.
Another principal's expired rows must not hide candidates or cause unrelated recognition.

Approval pending-list reads use `ListUnrecordedExpiredForPrincipal(ctx, principal, at, limit)` with one bounded batch of 200 candidates.
Sync polling does no expiration discovery.
Creation recognizes only a returned expired dedup candidate by ID.
Cleanup and object reads retain marker/event atomicity.
No caller does an unbounded global approval-expiry sweep.

Before the limit, apply exact principal and pending/unrecorded predicates.
Order candidates by `(expires_at,id)`.
Use a matching partial index.

Use existing `StorageError` categories for validation/conflict/not-found/connection/timeout.
Do not wrap raw event values in error messages.
Keep each repository under seven methods.

Repository implementations cannot import each other or the telemetry adapter.
The app worker passes a synchronous callback to delivery storage.
The callback calls the already-wired telemetry adapter.
This is a transaction-scoped operation callback, not a custom telemetry port.

Generalize the existing PostgreSQL context-carried `sqlx.Tx` and executor.
Migrate direct pool statements and autonomous transactions on all affected paths.
Never nest independent database transactions inside an owning transaction.

Code that changes several subjects determines and locks them in sorted order.
It verifies the affected set again under the business-object lock.
If discovery changes, it restarts its transaction.
Do not acquire a new out-of-order subject lock while holding business rows.
Only a local owning deletion transaction can restart, and only after successful rollback.
Cancellation, rollback failure, or an ambient joined scope stops the restart.
Existing permission-set serializable delete and signing bootstrap coordination must not silently lose their protections.

For local token issuance, keep validation/replay detection that intentionally commits security revocations outside the issuance-success scope.
Before migration, verify each pinned Fosite handler's failure-side effects.
Explicitly preserve committed revocations.

The outer issuance scope covers response-population mutations, token response construction, event append, and commit.
Signing preparation and external cryptographic I/O precede this scope.
The short transaction revalidates the prepared signing selection before it mutates state or releases a response.
Fosite's inner BeginTX/Commit join this scope.
HTTP success bytes and plaintext credential responses must not escape before the relevant commit.

After an unsuccessful mutation scope ends, record independent denial/failure facts in a fresh short transaction.
An unavailable ledger cannot promise a durable outage event in itself.
Return the existing failure contract.
Never permit access.

Do not automatically retry whole user operations.
An unsuccessful local operation does not prove that external token issuance/refresh failed.

## Query contract

A query requires a subject selector and a UTC occurrence interval.
The subject selector is either an exact nonempty principal or the explicit no-subject selector.
The interval is `[start,end)`, with start < end.
Exact registered type and outcome filters are optional.

Order results by `occurred_at ASC, id ASC`.
A cursor contains the last `(occurred_at,id)`.
Subsequent pages use strict tuple comparison with the same filters.
The repository limit defaults to 200 and cannot exceed 1000.
Operational SQL can deliberately request a different bounded limit.
Do not paginate by OFFSET for long investigations.

Ledger queries do not require the referenced business rows to exist.
The no-subject selector (`subject IS NULL`) retrieves system, administrative, and pre-authentication events.
It uses the same type, outcome, and time filters.
No query spans all subjects implicitly.

Example operational query with authorized PostgreSQL access:

```sql
SELECT id, occurred_at, envelope->>'agent_id' AS agent_id
FROM public.business_events
WHERE subject = :'principal'
  AND type = 'agentic-identity-broker.token-exchanged'
  AND outcome = 'success'
  AND occurred_at >= :'start'::timestamptz
  AND occurred_at < :'end'::timestamptz
ORDER BY occurred_at, id
LIMIT 200;
```

For a set of agents, the investigator deduplicates receiving agent IDs.
Multiple successful occurrences are not duplicate records.
The query requires neither tokens nor email/string searches.

## Shared lock protocol

The PostgreSQL lock hierarchy is mandatory:

1. The lifecycle gate is shared for append, dispatch, and erasure. It is exclusive for policy changes and partition maintenance.
2. Subject gates are shared for append/dispatch and exclusive for erasure. Multiple subjects require deterministic sorted acquisition. Null-subject events omit this gate.
3. Business-object locks/CAS preconditions precede delivery-reference locks. Delivery-reference locks use `(recorded_at,id)` order.

Memory uses the equivalent, deliberately coarser lifecycle/visibility protocol described in Memory parity.

Use transaction-scoped PostgreSQL advisory locks for lifecycle and subject gates.
Lifecycle and subject use separate two-int key classes, distinct from existing signing bootstrap locks.
Subject key hashes can collide.
Collisions only reduce concurrency because predicates still use exact subject equality.

A delivery candidate's subject can be read without a barrier only for lock selection.
After acquiring the gates, reload the event and delivery-reference row.
If either row disappeared, skip the candidate.
`FOR UPDATE SKIP LOCKED` on the delivery-reference row lets multiple workers compete without duplicate concurrent dispatch.
No event payload survives release of its dispatch transaction.

Dispatch separates export cancellation from the lifetime of its owning transaction.
Export observes the caller's cancellation and the storage write budget.
The transaction reserves one additional second for acknowledgement/rescheduling.
An export deadline must not roll back the 30-second retry schedule.
Retry writes use a one-second cleanup context under the same deletion barriers.
The transaction never acknowledges a cancelled export as successful.

Signing bootstrap lock acquisition precedes the domain operation that selects the initial key.
No other ledger operation attempts to acquire that bootstrap lock while holding lifecycle/subject gates.
Generate key material, provision branch keys, and encrypt before bootstrap coordination.
The callback only rechecks selection and persists the prepared candidate with its event and commit effects.
A losing candidate stores no key or promotion event.
Token issuance also loads its signer before ledger ownership.
PostgreSQL revalidation holds a shared lock on the signing revision row until completion.
Key mutations update that row. Changed selection fails closed without external cryptographic I/O under the gate.

Preserve the existing bootstrap timeout recovery.
Do not treat a key row without its event as a successful ledger-aware bootstrap.

Session refresh coordination precedes the ledger write transaction.
Provider I/O and token encryption hold no ledger lifecycle, subject, or memory visibility gates.
The short write transaction verifies the pre-exchange session snapshot, updates tokens, and appends the fact atomically.
Concurrent logout or reauthorization makes that conditional write fail closed.
PostgreSQL reuses its coordinated connection for the write.
If coordination unlock fails, PostgreSQL discards the connection.

Other business paths do not acquire that coordination lock while holding ledger gates.

Refresh coordination is the session-scoped exception.
`pg_advisory_lock(1095320150, hashtext(principal || '/' || service_id))` holds one PostgreSQL backend across provider I/O and the short write transaction.
Hash collisions can serialize unrelated sessions but do not change exact session predicates.

A pinned application connection requires a direct PostgreSQL connection or session-mode pooling.
Transaction-mode and statement-mode poolers are unsupported.
They can change the backend between lock, write, and unlock.
This change can leave a lock on a pooled backend.

The adapter sets no `lock_timeout` for refresh coordination.
Acquisition uses the caller's context.
Only its deadline or cancellation bounds the wait.
The automatic refresh workflow supplies an operation deadline.
Unlock uses a separate context bounded by the storage write timeout.

Failed or unconfirmed acquisition, failed unlock, and a false unlock result cause connection discard.
The adapter does not return that connection to the application pool.

### Principal erasure

Operator interface:

```sql
SET default_transaction_isolation = 'read committed';
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_erase_subject(:'principal');
```

The erasure function returns a bigint count of deleted event rows.
It rejects null/empty selection.
It requires `READ COMMITTED` isolation and rejects stronger ambient transactions, including direct operational SQL calls.
One invocation/transaction acquires lifecycle shared, then subject exclusive.
It deletes associated delivery-reference rows and all matching event rows across partitions.
Then it returns the count.
An invocation that repeats the operation returns zero successfully.

The function is VOLATILE and requires fresh statement snapshots.
After the lock wait, `READ COMMITTED` statements see committed predecessors.
A transaction-wide snapshot cannot provide this guarantee.
The function cannot return success before commit.
If clients use explicit BEGIN, they must COMMIT before reporting completion.
Before the invocation, set `statement_timeout` to a positive value no greater than `30s`.
Use a connection setting or a separate preceding command.
An explicit transaction can use `SET LOCAL statement_timeout = '30s'` before the `SELECT`.
The function rejects an absent or excessive deadline.

A recorder/emitter that holds the subject shared gate completes before erasure.
Erasure then removes its retained records.
A blocked new recorder starts its ordered occurrence after erasure commits.

Erasure uses no actor-ID matching, wildcard matching, permanent identity tombstone, or replacement event for the erased subject.
It does not delete principal references where another subject is the affected identity.
This is the explicit subject-based contract.

The narrowly scoped erasure function uses SECURITY DEFINER.
The migration role owns it.
It uses a fixed search_path, qualified object names, and parameterized subject predicates.
It accepts no caller-supplied SQL.

Revoke PUBLIC execution.
Grant execution only to the role named by `migration.grants.businessEvents.erasureRole`, or the equivalent grant in non-Helm deployments.
Never grant execution to the broker role.
Callers do not require unrestricted event DELETE.

Memory exposes the same operation through the domain service to authorized in-process operational/test harnesses.
Memory does not offer cross-process persistence.

### Partition maintenance

Operator/scheduler interfaces:

```sql
SET lock_timeout = '5s';
SET statement_timeout = '30s';
SELECT public.business_event_provision_partitions();
SELECT public.business_event_maintain_partitions();
```

Run both functions with migration-owned credentials, not runtime credentials.

`business_event_provision_partitions` is a SECURITY INVOKER function.
It obtains lifecycle exclusive and creates the current and next seven days of six-hour UTC partitions.
It never drops a partition and does not read the retention policy.

`business_event_maintain_partitions` is a SECURITY INVOKER function.
It obtains lifecycle exclusive, provisions the same windows, and reads one validated policy value.
Then it drops eligible event/delivery-reference partition pairs transactionally.
It is the only function that drops partitions.

`public.business_event_create_partition_pair(lower_bound timestamptz)` creates pairs.
This migration-owned SECURITY INVOKER function rejects non-six-hour UTC boundaries.
It derives quoted names and creates the pair idempotently.
Revoke PUBLIC and runtime execution of all three functions.

Provisioning uses the pair function for current and future windows.
PostgreSQL retention tests call it as the migration owner to seed historical windows.
Eligibility is `partition_upper_bound <= database_now - retention`.

Capture database-now once per sweep.
Generate dynamic identifiers from trusted timestamps.
Quote these identifiers.

Missing metadata, policy corruption, or privilege errors abort the operation without partial deletion of a pair.

The initial migration and pre-install/pre-upgrade migration job call `business_event_provision_partitions` before broker readiness.
They never drop partitions.
Only new broker startup stores a changed retention policy.
A drop before startup uses the superseded policy.
For a duration increase, that drop deletes history that the new policy retains.

The PostgreSQL-only CronJob runs `*/5 * * * *` with `concurrencyPolicy: Forbid`.
It calls `business_event_maintain_partitions` with `psql -v ON_ERROR_STOP=1`.

Reuse the existing `migration.grants.image`, migration service account, migration Secret, security context, and database connection pattern.
No runtime DDL, migration binary in the broker image, or new helper binary is necessary.

An external deployment must install the same scheduled SQL operation under its operations scheduler.
An advisory lock also protects against another deployment or manual maintenance invocation.

There is no DEFAULT partition.
Inserts outside provisioned windows fail closed.
After extended downtime, the next scheduled invocation recreates current/future windows and automatically removes overdue history.
Broker readiness verifies parent/current partition availability and recovers on successful maintenance.

Failed job status and credential-free operation diagnostics show retention failure.
Retention failure never silently disables recording.

Six-hour partition width plus a five-minute sweep gives less than 6h05m normal grace (plus bounded execution).
This grace is less than 24h.

Set bounded lock and statement deadlines before the invocation, through connection configuration or a separate command.
Maintenance requires a positive `statement_timeout` no greater than `30s`.
A function-local setting does not start the timeout for its already-running caller statement.
Persistent blockers or failed scheduler runs are operational faults that require an alert before 24h.
These faults do not permit silent extension of the guarantee.
Retention never removes an event early, even when retention is shorter than the partition width.

## Privileges and immutability

| Capability | Broker runtime | Operational reader | Erasure operator | Migration/maintenance owner |
|---|---|---|---|---|
| Read events | Yes, for dispatch/scoped repository queries | Yes | As authorized | Yes |
| Append events | Yes | No | No | Migration administration only |
| Update committed events | No, also blocked by trigger | No | No | No ordinary update path |
| Delete events | No direct permission | No | Erasure function only | Retention/rollback ownership |
| Delivery-reference DML | Yes | No | Through erasure function | Yes |
| Policy update | Validated startup DML under lifecycle lock | No | No | Yes |
| CREATE/DROP partitions | No | No | No | Yes |
| Maintenance, provisioning and partition-pair functions | No | No | No | Yes |

The existing chart grants all-table DML.

Adjust its grants for feature tables:

- After broad grants, revoke event UPDATE/DELETE.
- Revoke function execution from PUBLIC/runtime.
- Install matching default privileges for future feature objects.

Queries/inserts through partition parents use parent privileges.
Do not grant broad direct writes to child partitions.
The immutable-event UPDATE guard provides defense in depth.
Existing business-table privileges remain unchanged.

The operational reader and erasure operator are the roles named by `migration.grants.businessEvents.readerRole` and `migration.grants.businessEvents.erasureRole`.
Both values default to empty, which grants nothing.
With empty values, only the migration owner can query outside the broker or erase subjects.
The chart grants privileges to existing roles and never creates database roles.
Non-Helm deployments apply the equivalent `GRANT SELECT` and `GRANT EXECUTE` statements in the operations guide.

## Memory parity

Use one shared visibility gate across the involved stores.

Begin holds the visibility gate exclusively.
Standalone operations acquire it once and call private unlocked helpers for cross-store work.
Transaction completion closes admission and waits for every admitted repository operation to release its guard.
Only then can rollback apply undo entries or commit release visibility and effects.

Keep existing store-local locks in a documented subordinate order, or remove redundant locks during the prerequisite refactor.
Never use goroutine identity to detect reentrancy.

Rollback journals restore affected objects and secondary indexes in reverse order.
Readers cannot observe partial transactions.
Caller-owned pointers cannot mutate storage behind the gate.

Acquire a separate lifecycle barrier before the visibility gate.
The lifecycle barrier is shared for business transactions and dispatch.
It is exclusive for erasure, retention, and policy updates.
Thus, memory serializes subject erasure globally instead of introducing per-subject locks.

During dispatch, claim the delivery reference and an immutable event under a short visibility-gate section.
Release the visibility gate while holding lifecycle shared through synchronous export.
Then briefly reacquire visibility to acknowledge or reschedule.
Never hold the exclusive business-transaction gate across collector I/O.

Other dispatchers must not claim that reference concurrently.
Failure/cancellation releases the in-process claim.
Erasure waits for all active dispatch attempts, then removes records/references atomically.
This protocol prevents post-deletion replay.
It also prevents collector-induced serialization of all memory business commits.

The in-memory maintenance worker uses the same six-hour eligibility boundary.
It runs at startup and at a five-minute cadence.
It logically removes eligible records and delivery references under the lifecycle gate.
It uses the shared configured retention, not PostgreSQL's physical policy table.

All functional semantics match.
Process-crash durability and physical partition drop do not apply to memory.
