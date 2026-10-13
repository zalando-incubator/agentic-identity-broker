# ADR 039: Atomic Business Event Ledger and Recoverable Telemetry

**Status**: Accepted
**Date**: 2026-09-26
**Accepted**: 2026-09-26 by maintainer Jan Brennenstuhl
**Feature**: [048-business-event-ledger](../specs/048-business-event-ledger/spec.md)
**Numbering**: The ADR number changed from 037 on 2026-10-06 to distinguish this decision from main's CIMD and consent ADRs. Acceptance remains dated 2026-09-26. Number 038 belonged to a removed ADR about performance criteria. The number is not reused.

## Context

The broker needs records of 28 catalogued security facts alongside broker-owned state transitions. These records require exact subject investigation, credential exclusion, retention, and subject erasure. Existing slog records remain unchanged.

The accepted feature clarification requires automatic recovery after a crash between database commit and emission of the additional telemetry copy. Duplicate copies retain the same event ID.

The current PostgreSQL transaction context is OAuth2-specific. Other repositories contain autonomous transactions. Memory uses a no-op OAuth2 transaction manager. Ordinary logs use a batched OpenTelemetry processor.

Accepted ADR 009 separates migration/schema capabilities from runtime. Accepted ADR 011 assigns provider ownership to app wiring and rejects a custom telemetry port. These ADRs constrain this decision. This ADR does not supersede them.

## Decision

1. Generalize the existing transaction manager/executor. Migrate every catalogued producer, repository, and Fosite caller. Outer domain operations own commit. Nested scopes join the outer operation and cannot commit independently. Preserve intentional security mutations on failed OAuth2 validation paths. Memory uses a transaction-wide visibility gate and an undo journal for touched records, not whole-store snapshots.
2. Domain services construct immutable, schema-validated facts. Shared values belong in domain/model. Ledger invariants belong in domain/ledger. ISP repositories belong in ports/storage.go. Builder owns all wiring. Extract credential lifecycle logic from its current handler port bypass into the existing OAuth2-server context.
3. Use generated named UUIDv7 BusinessEventID values. Use fixed event type names `agentic-identity-broker.<event-name>` that follow Zalando event type naming (Rule 213). These names are independent of the hosting organization and deployment. Do not make these names configurable. Use URNs `urn:agentic-identity-broker:events:v1:<name>` for schema `$id`s.

   Publish embedded, closed Draft 2020-12 schemas under api/events/v1. Use the existing google/jsonschema-go dependency. JSON shape does not replace identity provenance or credential-safe field construction.
4. Store immutable events in PostgreSQL partitions for six-hour UTC recorded_at windows. Store payload-free delivery references in paired partitions within the business transaction. Do not create cascading event foreign keys to mutable business objects. Do not create a DEFAULT partition. Keep effective-expiry occurrence markers on the owning grant/approval records. These markers prevent deletion of event history from replaying a past expiry.
5. A builder-owned worker uses the existing telemetry destination configuration through a separate non-global, unbuffered SDK log provider. Before acknowledging the delivery reference, capture synchronous export success. Retries preserve the event ID. Do not alter existing slog or its batch pipeline. The design introduces no custom telemetry port.
6. Append, emission, erasure, and retention share a lifecycle/subject lock order. Emission reloads a retained event under shared barriers and holds them through its synchronous attempt. Erasure obtains exclusive subject access. Partition maintenance obtains exclusive lifecycle access. No buffered broker payload can outlive the deletion barrier. External copies already dispatched remain external data.
7. PostgreSQL partition creation and deletion run in a five-minute operational CronJob. This CronJob uses the existing PostgreSQL-client image and migration-owned credentials. Runtime remains DML-only. The maintenance function reads the validated common retention policy that startup stores. Six-hour partition granularity satisfies the 24-hour grace.

   Migrations and pre-install/pre-upgrade jobs only provision partitions. Only scheduled maintenance deletes partitions. Thus, an upgrade cannot sweep with a superseded policy. Memory does equivalent logical maintenance in-process.
8. Investigation uses operational SQL or internal scoped repositories. Subject erasure uses one operator-authorized SQL function. The feature adds no HTTP or CLI interface, external ingestion, historical log backfill, or user interface.

## Consequences

### Positive

The decision provides these benefits:

- Committed business state and event history cannot diverge through an application crash or ordinary rollback.
- One registry and 28 explicit producers support review of coverage and credential provenance.
- Independent delivery references preserve event immutability and recover missing telemetry. Collector availability does not affect commits.
- Deletion barriers cover in-flight work, not only references that wait in a queue.
- The design retains existing storage, provider, and configuration conventions and migration credential isolation.

### Costs and limits

The decision has these costs and limits:

- Changes to transaction ownership and memory no-op behavior require a separately reviewable structural refactor. They also require comprehensive regression checks with existing suites.
- Synchronous background export holds a bounded deletion barrier during a collector attempt. It must not run inline with business transactions or create an unbounded queue of payload copies.
- PostgreSQL requires a deployed component for partition maintenance. If maintenance stops and current partitions run out, recording fails closed.
- Retention changes require a coordinated rollout with one policy value across replicas. The deployment procedure prevents destructive old-policy sweeps during a duration increase.
- The broker cannot retract packets already dispatched or erase external logs, telemetry, or independent backups. The contract covers ledger-subject erasure, not system-wide identity deletion.
- Actual 5 ms p99 overhead remains unmeasured under documented deployment load. Planning does not claim that the design achieves this figure.

### Refresh coordination clarification (2026-10-06)

Main #124 serialized refresh with `SELECT ... FOR UPDATE` across the provider call. Ledger transactions also acquire the shared lifecycle gate. An open transaction during provider I/O blocks exclusive maintenance and policy changes.

PostgreSQL refresh instead pins one connection and takes a session-level advisory lock before provider I/O, outside a transaction. The short ledger-owned transaction uses that same connection and conditionally updates the pre-exchange session snapshot. This design preserves cross-process refresh serialization without lifecycle, subject, or business-row locks during the provider call.

This exception requires direct PostgreSQL connections or session-mode pooling. Transaction-mode and statement-mode poolers can leave locks on pooled backends. The design does not support these pooling modes.

The adapter sets no acquisition `lock_timeout`. The caller's context bounds the wait, and automatic refresh supplies a deadline. Cleanup uses a separate storage-write deadline. After uncertain acquisition or failed unlock, cleanup discards the connection.


## Alternatives Considered

The decision excludes these alternatives:

- **Best-effort after-commit logging**: This approach rejects the accepted recovery requirement.
- **Database triggers as the event producer**: These triggers lack the domain outcome and authoritative caller context.
- **A parallel transaction abstraction or full-memory snapshots**: These approaches duplicate existing patterns or add work proportional to history.
- **Reusing the normal batched slog bridge**: This approach changes the old signal path and leaves erasable payloads queued after deletion.
- **Erasure epoch check without a send barrier**: This approach leaves a check-to-send race.
- **Runtime partition DDL**: This approach violates the accepted separation of migration capabilities.
- **Daily partitions, row retention, or DEFAULT partition**: Daily partitions risk exceeding the grace. Row retention violates partition-drop retention. A DEFAULT partition hides missing maintenance and requires relocation.
- **Ledger-only expiry deduplication**: Erasure or retention removes the deduplication evidence and permits resurrection.
- **Configurable or hosting-organization type namespaces**: A configuration value gives one meaning several names across deployments. An organization-derived prefix fixes a hosting location in names that can never be reassigned.

## Governance

Maintainer Jan Brennenstuhl accepted this ADR on 2026-09-26. The decisions in this ADR bind this feature (Principle II). A deviation requires a superseding ADR. Acceptance does not authorize public API changes. This ADR supersedes no accepted ADR.

Before producer implementation, verify the event and operational contracts. Publish the schemas. Update the architecture/glossary documentation. Update the configuration/deployment documentation. Establish the required red E2E acceptance evidence.

The detailed plan and interfaces are available in these documents:

- [Plan](../specs/048-business-event-ledger/plan.md)
- [Storage contract](../specs/048-business-event-ledger/contracts/storage.md)
- [Event contract](../specs/048-business-event-ledger/contracts/events.md).
