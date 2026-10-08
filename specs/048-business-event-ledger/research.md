# Business Event Ledger — Research

**Date**: 2026-09-26 | **Branch**: `048-business-event-ledger`

This document records the original design research, not a performance result. The [specification](spec.md) and [plan](plan.md) record the retirement of the performance release gate. They also record the retirement of the deployment-profile requirement.

The 5 ms p99 goal remains unverified and non-blocking. The accepted ledger design is unchanged.

## 1. Atomic recording and transaction ownership

**Evidence**: `internal/ports/storage.go` defines `OAuth2TransactionManager`. `internal/adapters/storage/postgres/oauth2_transaction.go:13-56` carries `sqlx.Tx` in context. Only OAuth2 repositories consistently use that executor. `internal/domain/oauth2server/fosite_storage.go:79-101` bridges Fosite transactions. `internal/adapters/storage/factory.go` supplies a no-op transaction manager for memory.

Grants, agents, approvals, credentials, and signing keys also contain autonomous transactions or direct pool statements.

**Decision**: Generalize the existing protocol to `StorageTransactionManager`, its context key, and a shared executor. Migrate every caller. Remove the OAuth2-specific name and memory no-op. Provide semantics for transaction ownership and joining.

The outer domain operation owns commit. Nested repository/Fosite scopes join the transaction but cannot commit it. A nested failure poisons the owning transaction.

Do not introduce a second transaction stack. A callback helper can wrap Begin/Commit/Rollback, but it must use the same manager.

Domain services own the business transition. They append its validated event before commit. Repository operations return the affected row or changed result. They do not assume success from an earlier read.

Lock mutable state. Read it again before comparison.

No-op returns, lost CAS attempts, existing pending approvals, and already-consumed approvals produce no success event.

Extend all affected autonomous repository transactions to join the ambient executor. Preserve existing stronger isolation/locking requirements. If a nested operation requires stronger isolation than its owner provides, fail the operation. Do not silently weaken isolation.

Local token providers must own an outer transaction across Fosite response population, event recording, and final commit. Inner Fosite commit does not release the owner.

Validation/replay detection that intentionally commits security revocations stays before the issuance-success scope. A failed token request does not permit rollback of security revocation.

Explicitly separate committed security outcomes from unsuccessful issuance.

Where possible, keep third-party network calls and encryption preparation outside long-held database locks. Before you apply prepared state, verify mutable preconditions again.

Even if a later exchange fails, a committed refresh remains a separate fact.

**Alternatives considered**: Post-handler logging cannot be atomic. SQL triggers lack authoritative actor and outcome semantics. An outbox without participation in business transactions does not fix atomicity.

An indiscriminate transaction around every HTTP request changes independent failure-side effects. It also holds transactions across network calls.

## 2. Memory isolation

**Decision**: Replace the no-op with one adapter-wide transaction visibility gate and a touched-record undo journal.

An owning transaction holds the exclusive gate from begin through commit/rollback. Every standalone read/write participates in the gate. Ambient operations use internal unlocked entry points. They do not recursively acquire the gate.

Undo records contain only changed entries and secondary indexes. They contain no whole-store snapshots.

Return independent domain values so callers cannot mutate committed memory through retained pointers. Defer broadcasts and cache invalidation until the owning transaction commits.

Memory also uses a lifecycle barrier before the visibility gate. Append/dispatch take the shared lifecycle barrier. Erasure, retention, and policy changes take the exclusive lifecycle barrier. This design deliberately serializes memory erasure more broadly than PostgreSQL. It preserves the observable contract.

Dispatch claims an immutable event/reference under a short visibility-gate section. It releases that gate during synchronous export but retains the shared lifecycle barrier. It then acquires the visibility gate briefly to acknowledge the export. Collector waits do not monopolize the memory business-transaction gate. Erasure still waits for the in-flight attempt.

**Rationale**: An undo journal alone exposes uncommitted changes. It can overwrite a concurrent writer during rollback. Existing per-repository mutexes are insufficient for cross-repository atomicity.

**Alternatives considered**: Full-map copying adds history-proportional cost. A commit-only mutex fails isolation. No-op transactions violate FR-007.

## 3. Identity, schemas, and immutable type names

**Evidence**: `go.mod` pins google/uuid v1.6.0 and google/jsonschema-go v0.4.2 (currently indirect). ADR 013 requires generated named IDs. The Go module path is `github.com/agentic-identity-broker/agentic-identity-broker`.

The hosting organization appears only in repository metadata (README badges, Dockerfile image labels, the git remote). Accepted ADR 030 keeps vendor-specific values out of public source.

Constitution Principle IV binds APIs to the Zalando RESTful API and Event Guidelines. Rule 213 defines event type names as `<functional-name>.<event-name>`.

**Decision**: Add generated `id.BusinessEventID` with library UUIDv7 generation. Do not change constructors for existing UUIDv4 entity IDs.

Full event names are `agentic-identity-broker.<event-name>` per Zalando Rule 213. They use hyphenated event names such as `grant-created` and `signing-key-promoted`. The source is `urn:agentic-identity-broker:broker`. Neither the event names nor the source is configurable.

The source identifies the producer application. Existing OTLP Resource attributes identify deployment instances.

Use Draft 2020-12 schemas. Promote the existing google/jsonschema-go dependency to direct. Compile schemas once at startup through an embedded-only registry. Resolve schemas once at startup through the same registry. Reject network reference loading, unknown types, and invalid payloads. Before producer implementation, publish the envelope, catalog, and 28 closed per-type schemas under `api/events/v1/`.

The reviewed designs are in `contracts/schemas/`.

Optional field additions must preserve all previously valid records. Incompatible meanings require a new event type.

Schema `$id`s are URNs (`urn:agentic-identity-broker:events:v1:<name>`). Their absolute URN `$ref`s use the offline loader of the registry. They identify resources. The registry never fetches them.

Validate typed UUIDs, UTC instants, IPs, trace identifiers, and semantic field provenance in addition to JSON shape.

The pinned validator ignores `format`. The contracts use patterns and domain parsers. They do not assume that the validator enforces `format`. Error reporting must never echo rejected values.

**Alternatives considered**: UUIDv4 is not time ordered. A hand-written identifier/JSON-schema engine is unnecessary. A free-form data map or open additional properties makes accidental disclosure likely. General event ingestion is out of scope.

A configurable namespace can give one meaning several names across deployments. This breaks published schemas, investigation queries, and consumer rules. Deployment identity already lives in OTLP Resource attributes.

A hosting-derived namespace such as `io.github.<organization>` or `com.<vendor>` ties permanent names to an organization. The organization can change, but published names cannot. Relative `$ref`s require a hierarchical HTTPS base. That base again implies an owned host.

**Primary documentation**: [google/jsonschema-go v0.4.2](https://github.com/google/jsonschema-go/blob/v0.4.2/jsonschema/doc.go), [UUID library](https://pkg.go.dev/github.com/google/uuid#NewV7).

## 4. Trusted attribution and credential exclusion

**Evidence**: `internal/domain/security/context.go:17-26` has trace, actor, calling peer, client IP, and user agent, but no span ID. It truncates the user agent but does not remove credentials.

`internal/domain/tokenexchange/service.go:176-201` finalizes SecurityContext with the affected principal as Actor. It uses the validated client assertion subject as CallingPeer. A copy of `SecurityContext.Actor` in the ledger caller field can misattribute delegation. The request middleware defers finalization for all POST `/oauth2/token` requests.

**Decision**: Add a domain event-context value from authenticated results and the existing capture.

`subject` is the affected principal. The ledger actor is the verified initiating caller.

For delegated exchange, use the verified calling peer as actor. Use the validated affected principal as `subject` and `actor.on_behalf_of`. Do not modify existing slog attribution.

For gateway approvals, retain the authenticated gateway identity separately from the agent and principal.

Null identities are intentional. The anonymous sentinel is not a principal. Machine client-credentials issuance has a null subject. It does not use an agent UUID as a user.

For impersonation, only a subject from the accepted authorization/delegation path can become authoritative. This includes ADR 031's narrowly authorized unverified-subject mode.

Do not copy partially evaluated audit claims from rejected rules. Keep the independently verified client assertion identity as the initiating caller.

When the actor-token identity differs from the client, optional `data.delegating_actor_id` on impersonation events preserves that verified identity. It does not replace the caller.

The trace comparison uses the request trace ID and the authoritative SecurityContext trace ID.

If the trace IDs match, capture the valid request span ID at the adapter boundary. Otherwise, omit the span ID. Pass that scalar context into the domain. Do not import tracing infrastructure into the event model.

Background work has a fixed system actor and no fabricated request identifiers.

Reasons come from fixed templates selected by safe outcome codes. They never contain raw upstream errors. Type data is deliberately small. Most success events need only envelope references. Credential and key events retain safe resource IDs.

The ledger does not retain raw user-agent strings. It normalizes a recognized product to one of the fixed family names in the schema. It omits unrecognized content. IPs come from the existing trusted-proxy extraction and typed parsing.

Copy optional opaque session correlations only from established non-credential context. Do not copy arbitrary request strings, tool names/arguments, payloads, URLs, claims, or credential hashes.

Schema validity alone does not prove safe provenance.

**Alternatives considered**: Regex token scrubbing or length truncation cannot recognize arbitrary opaque credentials. Serialization of existing structs or AuditRecord wholesale can leak partially trusted identities and arbitrary strings. Omission of all known affected principals defeats subject erasure.

## 5. Crash-safe telemetry without changing slog

**Evidence**: The current log provider uses `sdklog.NewBatchProcessor` (`internal/adapters/telemetry/provider.go:374-380`). The pinned SDK supports EventName, synchronous processor export, explicit provider attribute limits, and dropped-attribute counts. `Logger.Emit` does not return export success. ADR 011 requires app-owned providers and rejects a custom telemetry port.

**Decision**: When copying is effectively enabled, insert a payload-free delivery reference in the same transaction as each event.

A builder-owned worker scans references. It reloads the retained event under the lifecycle/subject barrier. It emits through a separate non-global SDK LoggerProvider in the existing telemetry adapter.

Use the same exporter configuration and Resource construction. Use a separate exporter instance and synchronous result-capturing processor.

The processor delegates to the SDK synchronous processor. It records the error in a private per-call result holder. A missing callback, dropped attribute, cancellation, or error is not an acknowledgment.

Delete the delivery reference only after successful export and commit.

A crash after export but before acknowledgment can duplicate the copy with the same event ID.

This ledger pipeline has no BatchProcessor or payload queue. Explicit provider attribute count/value-length limits are unlimited (`-1`). This prevents SDK environment variables from silently truncating the envelope. Closed schemas bound field names. Arrays remain single attributes. Producer provenance controls values.

Existing request limits remain in force.

Do not silently shorten a required field.

Exporter failure leaves the reference for a later bounded attempt. It cannot undo a business commit. Provider initialization failure remains non-fatal to the durable ledger. Delivery references remain, with a credential-free warning. The worker retries initialization. The worker stops before its provider/storage shutdown.

The effective copy switch is `telemetry.enabled && telemetry.logs.enabled && business_events.telemetry_copy_enabled`.

Explicit disablement of a relevant switch pauses existing delivery references. The ledger creates no references for occurrences during disablement. Re-enablement resumes retained pre-disable pending work, not historical backfill. Telemetry disablement never disables retention or recording.

**Alternatives considered**: After-commit best effort contradicts the accepted clarification. Reuse of the slog bridge cannot reliably set the full event contract. It risks changes to old signals. A batch queue can retain an erased payload after deletion.

An erasure epoch verified before an asynchronous send still has a race. Updates to `emitted_at` on immutable ledger rows mix delivery state with authority.

**Primary documentation**: [SDK synchronous processor](https://github.com/open-telemetry/opentelemetry-go/blob/sdk/log/v0.22.0/sdk/log/simple.go), [log record API](https://pkg.go.dev/go.opentelemetry.io/otel/log#Record), [SDK provider limits](https://pkg.go.dev/go.opentelemetry.io/otel/sdk/log#WithAttributeCountLimit).

## 6. Retention, partitions, and erasure barriers

**Evidence**: Accepted ADR 009 reserves schema-modification capabilities for migration workloads. The chart already uses a PostgreSQL-client maintenance image, migration service account, and separate migration Secret in `templates/job-migrate.yaml`.

At the initial design review, the latest local migration was 032. Main later added migrations 033–035. PostgreSQL partitioned unique constraints must include the partition key.

**Decision**: After rebase onto main, use migration 036 for ledger tables/functions and occurrence markers.

Main's CIMD migrations occupy 033–035.

Use six-hour UTC range partitions on `recorded_at`. Pair them with partitioned delivery-reference tables.

The composite primary key is `(recorded_at, id)`. Trusted UUIDv7 generation supplies event identity.

Mutations, CAS results, and durable object markers establish occurrence uniqueness. A partition-local event-type index does not establish it. No event FK cascades from mutable business objects. There is no DEFAULT partition.

A separate chart CronJob runs every five minutes. It uses the existing PostgreSQL client image and migration-owned credentials. It runs a fixed SQL maintenance function supplied by migrations. The function creates seven days of future partitions.

The function drops a partition only if its upper bound is no later than database-now minus retention. Pair drops occur transactionally. Maximum normal-operation grace is under six hours plus five minutes. This is less than the 24-hour requirement.

The initial migration and pre-upgrade job run drop-free provisioning only. They never run maintenance. Missing partitions fail recording closed. Automatic maintenance restores them. Runtime has no DDL privileges or EXECUTE permission on maintenance.

Broker startup validates the single retention duration. It writes the microsecond-precision value to a singleton policy row via DML. Maintenance reads the policy under the lifecycle lock. It does not parse a second configuration format.

Policy updates acquire the same exclusive lifecycle lock. A duration increase cannot race a drop based on the old value.

During a rollout of retention configuration, all replicas must use the same value. Do not mix configuration values in a rolling rollout.

Changed configuration values apply on restart.

External deployments install an equivalent five-minute scheduler as part of deployment. They do not use a manual retention procedure.

Every recording transaction, emitter, and eraser acquires a shared lifecycle advisory transaction lock first. Recorders and emitters then take shared per-subject locks. Erasure takes the exclusive lock for the exact subject. Maintenance takes the exclusive lifecycle lock and no subject locks.

Use distinct two-int advisory key classes for lifecycle and subject.

Hash collisions only serialize unrelated subjects. SQL predicates always compare the exact principal.

For operations on several principals, acquire subject locks in deterministic order before business row locks. Only then acquire business rows and delivery references.

An emitter can read candidate identifiers optimistically. After it acquires barriers, it must verify row existence again. It must then hold the barriers through synchronous export and acknowledgment.

Erasure uses one operator-authorized SQL function/transaction. It removes delivery references and events across all partitions. It returns only a deletion count. Its completion boundary is commit under the exclusive subject lock.

Erasure deletes events from writers that commit before that boundary. Blocked writers commit after it as new occurrences. Erasure inserts no event for the erased subject.

Network/collector copies that already left the broker are external observability data. Deletion cannot retract them. No buffered SDK copy remains available for a later send.

**Alternatives considered**: Daily partitions plus a periodic sweep can exceed 24 hours. Row-by-row retention violates FR-005. pg_partman adds an unnecessary extension. A DEFAULT partition requires relocation or row deletion. It hides missing maintenance. Snapshot-only erasure fails against concurrent inserts.

Runtime partition DDL conflicts with accepted ADR 009.

**Primary documentation**: [PostgreSQL 17 partitioning](https://www.postgresql.org/docs/17/ddl-partitioning.html), [advisory locks](https://www.postgresql.org/docs/17/explicit-locking.html#ADVISORY-LOCKS).

## 7. Producer ownership and less obvious transitions

The full map is in `contracts/producers.md`. The verified high-risk boundaries are:

- `consent.Service.GrantConsent` has an explicit unchanged-state return. Both revocation paths must converge on one recorded transition. Their distinct 404/idempotent HTTP behavior must remain unchanged.
- `approval.Service.CreatePendingApproval` distinguishes new approvals from returned-existing approvals. ConsumeApproval has an already-consumed success return. Sync increments are transactional. Local broadcasts must wait for outer commit.
- `oauth2session.createSession` upserts both first establishment and reauthorization. A callback that establishes usable credentials is `session-established`. Refresh-token operations are `session-refreshed`. A shared upsert alone cannot classify the event.
- `oauth2server.Provider` exposes client-credentials, authorization-code, and refresh issuance. Fosite's transaction boundaries must join the outer operation. Without multi-agent verification, the proxy strategy currently streams responses. The required sequence is response staging, broker-outcome recording, then forwarding. Parsing upstream tokens merely to invent a subject is prohibited.
- Credential generation/rotation/revocation currently bypasses a domain service in `client_credentials_handler.go:97-123,214`. That policy must move into the existing OAuth2-server bounded context. Handler logs and response contracts remain unchanged.
- `impersonation.evaluateRule` currently combines the permission decision and minting. The function must record a granted decision after authorization and delegation succeed, before minting. A mint failure then produces token request failure, not a false denial. Successful RFC 8693 impersonation produces `token-exchanged`, never `token-issued`.
- `SigningKeyService.PromoteKey`, `generateAndStore(makeCurrent=true)`, and bootstrap all select a current signing key. The committed current-key selection requires one recorded event. The event must retain `data.activates_at` to distinguish selection from eligibility after the existing JWKS grace interval. An activation scheduler and changes to key grace rules are prohibited.
- Agent/provider deletion cascades can remove cataloged grants, credentials, approvals, or sessions. The same transaction must capture affected rows and record their corresponding terminal facts. A database cascade must never erase ledger history. Approval revocation is not also a new user denial because its stored status becomes denied.
- Expiration recognition needs durable `expiration_recorded_for` instants on the owning grant/approval rows, not ledger-only deduplication. Recognition compares the marker with effective expiry. A validity change can create a new lifecycle. Repeated discovery after ledger erasure must not recreate an old occurrence. Existing lazy reads, filtered queries, duplicate retirement, and cleanup must all use this recognition boundary. The design introduces no wall-clock expiration scheduler.

## 8. Shared validation lanes and performance governance

**Decision (2026-10-06)**: Use the existing backend E2E and integration lanes for all 20 active functional scenarios. The user removed feature-specific diagnostic suites, runner commands, and setup. No replacement performance suite is required.

Run `just test-e2e-backend 'business-event-ledger && !performance'` for memory and PostgreSQL with the default `integration` tag.
For memory only, run `just test-e2e-backend 'business-event-ledger && !performance' ''`.
Use `just test-integration-infra`, `just check`, `just security broker`, and normal `just verify` for the matching shared checks.

The [performance report](performance-results.md) preserves historical distributions, conditions, correctness checks, and rollback counts. A mean or a noisy difference of independent p99 values does not establish attributable p99 overhead.

The 5 ms p99 figure remains an unverified, non-blocking goal. No deployment profile, operator approval, or numeric performance result is required. The design adds no production recording bypass.

**Alternatives considered**: A replacement diagnostic suite repeats the rejected feature-specific setup. A guessed current-load number is not evidence. A required deployment profile restores the retired release gate. A production bypass undermines mandatory recording.

The initial and post-design gates select no departure from accepted storage, configuration, typed-ID, migration, or provider ADRs.

ADR 039 records the cross-cutting decision. The maintainer accepted it on 2026-09-26. It binds this feature but approves no public API change. ADR 033 remains Proposed. This feature does not silently promote it.

Explicit implementation prerequisites remain:

- Domain/schema publication
- Real migration tests
- Red E2E evidence
- Deployment configuration
- Operator documentation.
