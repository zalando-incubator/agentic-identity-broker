# Storage and Operational Contracts

These are planned internal/operational interfaces, not new HTTP endpoints. The domain service validates registry, authorization context, and lifecycle semantics; handlers never append directly to repositories.

## Ports and transaction ownership

All repository interfaces live in `internal/ports/storage.go`; shared event/query/reference value types live in `internal/domain/model/`. Use the existing storage factory accessors and builder injection.

| Contract | Operations and behavior |
|---|---|
| StorageTransactionManager | Generalized existing BeginTX/Commit/Rollback protocol; transaction scopes distinguish owning and joined handles; only owner commits physically; joined rollback marks owner rollback-only |
| BusinessEventRepository | Append validated event in ambient transaction; Query with a required subject selector (exact principal or explicit no-subject), optional filters and cursor; Get by `(recorded_at,id)`; no Update or generic Delete |
| BusinessEventLifecycleRepository | EraseSubject, ApplyRetention (logical memory operation), SetRetentionPolicy; PostgreSQL ApplyRetention is available only to the maintenance owner, never runtime wiring |
| BusinessEventDeliveryRepository | ListDue reference identifiers; DispatchOne under barriers using a synchronous callback; no payload returned outside the barrier for later sending |
| BusinessEventRecorder | Domain service contract for recording registered facts and independent outcomes; recipes determine identity, outcome and allowed data, not the HTTP handler |
| UserGrantExpirationRepository, ToolApprovalExpirationRepository | Separate ISP interfaces implemented by the existing grant and approval adapters, because `UserGrantRepository` already exceeds the method budget. List expired objects whose `expiration_recorded_for` differs from their effective expiry, and conditionally set that marker in the ambient transaction, reporting whether this call recognized the expiry |

Use existing `StorageError` categories for validation/conflict/not-found/connection/timeout. Do not wrap raw event values in error messages. Keep each repository under seven methods. Repository implementations cannot import each other or the telemetry adapter; the app worker passes a synchronous callback to delivery storage, and the callback invokes the already-wired telemetry adapter. This is a transaction-scoped operation callback, not a custom telemetry port.

Generalize the existing PostgreSQL context-carried `sqlx.Tx` and executor, migrating direct pool statements and autonomous transactions on all affected paths. Never nest independent database transactions inside an owning transaction. Code that changes several subjects determines and locks the subjects in sorted order, rechecks the affected set under the business-object lock, and restarts its transaction if discovery changed. Do not acquire a new out-of-order subject lock while holding business rows. Existing permission-set serializable delete and signing bootstrap coordination must not silently lose their protections.

For local token issuance, keep validation/replay detection that intentionally commits security revocations outside the issuance-success scope. The outer issuance scope covers response-population mutations, token response construction, event append and commit; Fosite's inner BeginTX/Commit join this scope. Verify each pinned Fosite handler's failure-side effects before migration and explicitly preserve any committed revocation. No HTTP success bytes or plaintext credential response may escape before the relevant commit.

Record independent denial/failure facts in a fresh short transaction after an unsuccessful mutation scope has ended. An unavailable ledger cannot promise a durable outage event in itself; return the existing failure contract, never permit access. Do not automatically retry complete user operations: external token issuance/refresh may already have happened.

## Query contract

Required filters: a subject selector that is either an exact nonempty principal or the explicit no-subject selector, optional exact registered type and outcome, and UTC occurrence interval `[start,end)`, with start < end. Order by `occurred_at ASC, id ASC`. A cursor contains the last `(occurred_at,id)`; subsequent pages use strict tuple comparison with the same filters. Repository limit defaults to 200 and is bounded at 1000; operational SQL may deliberately request a different bounded limit. Do not paginate by OFFSET for long investigations. Ledger querying does not need the referenced business rows to exist. The no-subject selector (`subject IS NULL`) retrieves system, administrative and pre-authentication events with the same type, outcome and time filters; no query spans all subjects implicitly.

Example operational query, using authorized PostgreSQL access:

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

The investigator deduplicates receiving agent IDs if asking for the set of agents, rather than confusing multiple successful occurrences with duplicate records. Neither tokens nor email/string searches are needed.

## Shared lock protocol

The PostgreSQL lock hierarchy below is mandatory. Memory uses the equivalent, deliberately coarser lifecycle/visibility protocol described in Memory parity:

1. Lifecycle gate: shared for append, dispatch and erasure; exclusive for policy changes and partition maintenance.
2. Subject gate(s): shared for append/dispatch, exclusive for erasure; deterministic sorted acquisition for multiple subjects. Null-subject events omit this gate.
3. Business-object locks/CAS preconditions, then delivery-reference locks in `(recorded_at,id)` order.

Use transaction-scoped PostgreSQL advisory locks. Lifecycle and subject use separate two-int key classes, distinct from existing signing bootstrap locks. Subject key hashes may collide; this only reduces concurrency because predicates still use exact subject equality. A delivery candidate's subject may be looked up without a barrier only for lock selection: reload the event and delivery-reference row after acquiring the gates and skip if either disappeared. `FOR UPDATE SKIP LOCKED` on the delivery-reference row lets multiple workers compete without duplicate concurrent dispatch. No event payload survives release of its dispatch transaction.

Signing bootstrap lock acquisition precedes the domain operation that selects the initial key; no other ledger operation attempts to acquire that bootstrap lock while holding lifecycle/subject gates. Preserve the existing bootstrap timeout recovery without treating a key row without its event as a successful ledger-aware bootstrap.

### Principal erasure

Operator interface:

```sql
SELECT public.business_event_erase_subject(:'principal');
```

Returns a bigint count of deleted event rows. Reject null/empty selection. One invocation/transaction acquires lifecycle shared then subject exclusive, deletes associated delivery-reference rows and all matching event rows across partitions, and returns the count. Repeat returns zero successfully. The function is VOLATILE so statements after waiting for locks see committed predecessors; do not capture the deletion snapshot before acquiring the barrier. It cannot return success before commit (clients using explicit BEGIN must COMMIT before reporting completion).

A recorder/emitter holding the subject shared gate completes before erasure; erasure then removes its retained records. A blocked new recorder starts its ordered occurrence after erasure commits. No actor-ID matching, wildcard matching, permanent identity tombstone, or replacement event for the erased subject. This does not delete principal references where another subject is the affected identity; that is the explicit subject-based contract.

The erasure function is narrowly scoped SECURITY DEFINER, owned by the migration role, with fixed search_path, qualified object names, parameterized subject predicates, and no caller-supplied SQL. Revoke PUBLIC execution. Grant execution only to the role named by `migration.grants.businessEvents.erasureRole`, or the equivalent grant in non-Helm deployments, never to the broker role; callers need not receive unrestricted event DELETE. Memory exposes the same operation through the domain service to authorized in-process operational/test harnesses; it does not pretend to offer cross-process persistence.

### Partition maintenance

Operator/scheduler interfaces:

```sql
SELECT public.business_event_provision_partitions();
SELECT public.business_event_maintain_partitions();
```

Run both using migration-owned credentials, not runtime credentials. `business_event_provision_partitions` is a SECURITY INVOKER function that obtains lifecycle exclusive and creates the current and next seven days of six-hour UTC partitions. It never drops a partition and does not read the retention policy. `business_event_maintain_partitions` is a SECURITY INVOKER function that obtains lifecycle exclusive, provisions the same windows, reads one validated policy value, and drops eligible event/delivery-reference partition pairs transactionally. It is the only function that drops partitions. Pairs are created through `public.business_event_create_partition_pair(lower_bound timestamptz)`, a migration-owned SECURITY INVOKER function that rejects non-six-hour UTC boundaries, derives quoted names, and creates the pair idempotently. PUBLIC and runtime execution of all three functions are revoked. Provisioning uses the pair function for current and future windows; PostgreSQL retention tests call it as the migration owner to seed historical windows. Eligibility is `partition_upper_bound <= database_now - retention`. Capture database-now once per sweep. Dynamic identifiers are generated from trusted timestamps and quoted. Missing metadata, policy corruption, or privilege errors abort without partially dropping a pair.

The initial migration and the pre-install/pre-upgrade migration job invoke `business_event_provision_partitions` before broker readiness. They never drop partitions: a changed retention policy is stored only when the new broker starts, so a drop at that point would still use the superseded policy and, for a duration increase, delete history the new policy retains. The PostgreSQL-only CronJob runs `*/5 * * * *`, `concurrencyPolicy: Forbid`, and calls `business_event_maintain_partitions` with `psql -v ON_ERROR_STOP=1`. Reuse the existing `migration.grants.image`, migration service account, migration Secret, security context and database connection pattern. No runtime DDL, migration binary in the broker image, or new helper binary is needed. An external deployment must install the same scheduled SQL operation under its operations scheduler. An advisory lock also protects against another deployment or manual maintenance invocation.

No DEFAULT partition: inserts outside provisioned windows fail closed. After extended downtime, the next scheduled invocation recreates current/future windows and removes overdue history automatically. Broker readiness checks parent/current partition availability and recovers on successful maintenance. Retention failure is visible via failed job status and credential-free operation diagnostics; it never disables recording silently.

Six-hour partition width plus a five-minute sweep gives less than 6h05m normal grace (plus bounded execution), comfortably below 24h. Use bounded lock/statement deadlines; persistent blockers or failed scheduler executions are operational faults requiring an alert before 24h, not permission to silently extend the guarantee. No event is removed early even for retention shorter than the partition width.

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

The existing chart grants all-table DML. Adjust its grants for feature tables: revoke event UPDATE/DELETE after broad grants, revoke function execution from PUBLIC/runtime, and install matching default privileges for future feature objects. Queries/inserts through partition parents use parent privileges; do not grant broad direct writes to child partitions. The immutable-event UPDATE guard is defense in depth. Existing business-table privileges are not removed.

The operational reader and erasure operator are the roles named by `migration.grants.businessEvents.readerRole` and `migration.grants.businessEvents.erasureRole`. Both default to empty, which grants nothing; only the migration owner can then query outside the broker or erase. The chart grants privileges to existing roles and never creates database roles. Non-Helm deployments apply the equivalent `GRANT SELECT` and `GRANT EXECUTE` statements documented in the operations guide.

## Memory parity

Use one shared visibility gate across involved stores. Begin holds it exclusively; standalone operations acquire it once and call private unlocked helpers for cross-store work. Keep existing store-local locks in a documented subordinate order or remove redundant ones during the prerequisite refactor. Never use goroutine identity to detect reentrancy. Rollback journals restore affected objects and secondary indexes in reverse order. Readers cannot observe partial transactions, and caller-owned pointers cannot mutate storage behind the gate.

Acquire a separate lifecycle barrier before the visibility gate: shared for business transactions and dispatch, exclusive for erasure, retention and policy updates. Memory therefore serializes subject erasure globally rather than introducing per-subject locks. During dispatch, claim the delivery reference and an immutable event under a short visibility-gate section, release the visibility gate while holding lifecycle shared through synchronous export, then reacquire visibility briefly to acknowledge or reschedule. Never hold the exclusive business-transaction gate across collector I/O. No other dispatcher may claim that reference concurrently, and failure/cancellation releases its in-process claim. Erasure waits for all active dispatch attempts, then removes records/references atomically. This avoids both post-deletion replay and collector-induced serialization of all memory business commits.

The in-memory maintenance worker uses the same six-hour eligibility boundary and runs at startup/five-minute cadence; it logically removes eligible records and delivery references under the lifecycle gate. It uses the shared configured retention, not PostgreSQL's physical policy table. All functional semantics match; process-crash durability and physical partition drop do not apply to memory.
