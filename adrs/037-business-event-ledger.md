# ADR 037: Atomic Business Event Ledger and Recoverable Telemetry

**Status**: Approved
**Date**: 2026-09-26
**Feature**: [048-business-event-ledger](../specs/048-business-event-ledger/spec.md)

## Context

The broker needs 28 catalogued security facts recorded with broker-owned state transitions, with exact subject investigation, credential exclusion, retention and subject erasure. Existing slog records remain unchanged. The accepted feature clarification requires automatic recovery when a process crashes after database commit but before its additional telemetry copy is emitted; duplicate copies retain the same event ID.

The current PostgreSQL transaction context is OAuth2-specific, other repositories contain autonomous transactions, and memory uses a no-op OAuth2 transaction manager. Ordinary logs use a batched OpenTelemetry processor. Accepted ADR 009 separates migration/schema capabilities from runtime; accepted ADR 011 owns providers in app wiring and rejects a custom telemetry port. These are constraints, not decisions superseded by this proposal.

## Decision

1. Generalize the existing transaction manager/executor and migrate every catalogued producer, repository, and Fosite caller. Outer domain operations own commit; nested scopes join and cannot commit independently. Preserve intentional security mutations on failed OAuth2 validation paths. Memory uses a transaction-wide visibility gate and touched-record undo journal, not whole-store snapshots.
2. Domain services construct immutable, schema-validated facts. Shared values belong in domain/model, ledger invariants in domain/ledger, and ISP repositories in ports/storage.go. Builder owns all wiring. Extract credential lifecycle logic from its current handler port bypass into the existing OAuth2-server context.
3. Use generated named UUIDv7 BusinessEventID and fixed event type names `agentic-identity-broker.<event-name>` following Zalando event type naming (Rule 213), independent of hosting organization and deployment and not configurable; schema `$id`s are URNs `urn:agentic-identity-broker:events:v1:<name>`. Publish embedded, closed Draft 2020-12 schemas under api/events/v1 using the existing google/jsonschema-go dependency. JSON shape does not replace identity provenance or credential-safe field construction.
4. Store immutable events in PostgreSQL six-hour UTC recorded_at partitions. Store payload-free pending-delivery references in paired partitions within the business transaction. No cascading event foreign keys to mutable business objects and no DEFAULT partition. Keep effective-expiry occurrence markers on owning grant/approval records so deletion of event history cannot replay a past expiry.
5. A builder-owned worker uses the existing telemetry destination configuration through a separate non-global, unbuffered SDK log provider. Capture synchronous export success before acknowledging the pending reference; retries preserve the event ID. Do not alter existing slog or its batch pipeline. No custom telemetry port is introduced.
6. Append, emission, erasure and retention share a lifecycle/subject lock order. Emission reloads a retained event under shared barriers and holds them through its synchronous attempt. Erasure obtains exclusive subject access; partition maintenance obtains exclusive lifecycle access. No buffered broker payload can outlive the deletion barrier. External already-dispatched copies remain external data.
7. PostgreSQL partition creation/drop runs in a five-minute operational CronJob using the existing PostgreSQL-client image and migration-owned credentials. Runtime remains DML-only. The maintenance function reads the validated common retention policy stored by startup, with six-hour partition granularity satisfying the 24-hour grace. Memory performs equivalent logical maintenance in-process.
8. Investigation uses operational SQL or internal scoped repositories; subject erasure is one operator-authorized SQL function. No new HTTP or CLI interface, external ingestion, historical log backfill or user interface is added.

## Consequences

### Positive

- Committed business state and event history cannot diverge through an application crash or ordinary rollback.
- One registry and 28 explicit producers make coverage and credential provenance reviewable.
- Independent delivery references preserve event immutability and recover missing telemetry without collector availability affecting commits.
- Deletion barriers cover in-flight work, not merely references waiting in a queue.
- The design retains existing storage/provider/configuration conventions and migration credential isolation.

### Costs and limits

- Generalizing transaction ownership and replacing memory no-op behavior requires a separately reviewable structural refactor and comprehensive existing-suite regression checks.
- Synchronous background export holds a bounded deletion barrier during a collector attempt. It must not run inline with business transactions or create an unbounded queue of payload copies.
- Partition maintenance is a required operational deployment component for PostgreSQL. Missing maintenance fails recording closed when current partitions run out.
- Retention changes require a coordinated rollout with one policy value across replicas; the deployment procedure prevents destructive old-policy sweeps during a duration increase.
- The broker cannot retract packets already handed off or erase external logs, telemetry, or independent backups. The contract is ledger-subject erasure, not system-wide identity deletion.
- Actual 5 ms p99 overhead remains to be measured under documented deployment load; planning does not claim it is achieved.

## Alternatives Considered

- **Best-effort after-commit logging**: rejects the accepted recovery requirement.
- **Database triggers as the event producer**: lack domain outcome and authoritative caller context.
- **A parallel transaction abstraction or full-memory snapshots**: duplicates existing patterns or adds history-proportional work.
- **Reusing the normal batched slog bridge**: changes the old signal path and leaves erasable payloads queued after deletion.
- **Erasure epoch check without a send barrier**: leaves a check-to-send race.
- **Runtime partition DDL**: violates accepted migration capability separation.
- **Daily partitions, row retention, or DEFAULT partition**: respectively risk exceeding the grace, violate partition-drop retention, or hide missing maintenance and require relocation.
- **Ledger-only expiry deduplication**: erasure or retention removes the dedup evidence and permits resurrection.
- **Configurable or hosting-organization type namespaces**: a setting gives one meaning several names across deployments; an organization-derived prefix freezes a hosting location into names that can never be reassigned.

## Governance

This ADR is a proposal for review, not an accepted exception or authorization to change public APIs. It supersedes no accepted ADR. Before producer implementation, approve the event/operational contracts, publish the schemas, update architecture/glossary and configuration/deployment documentation, and establish the required red E2E acceptance evidence. Detailed plan and interfaces: [plan](../specs/048-business-event-ledger/plan.md), [storage contract](../specs/048-business-event-ledger/contracts/storage.md), [event contract](../specs/048-business-event-ledger/contracts/events.md).
