# ADR 038: Refresh-Session Encryption and Transaction Ownership

## Status

Accepted on 2026-10-01. User approval: "I accept ADR 038".

Acceptance covers the encryption-subject extension and transaction ownership. API/release approval and feature implementation remain separate.

## Context

The [feature specification](../specs/049-fix-refresh-consent/spec.md) defines refresh behavior. This ADR records two architectural choices, not another copy of those requirements.

Persisted retry results need an encryption subject that represents their refresh session. ADR 008's 2026-06-01 amendment originally approved only `service_id` and `kid`. It requires an amendment or superseding ADR before another subject is introduced. A refresh session is neither a third-party service nor a signing key.

Refresh and authorization changes must have one consistent outcome across broker instances. Independent repository transactions or a Fosite-owned commit cannot provide that outcome. The [research](../specs/049-fix-refresh-consent/research.md) records the existing transaction limitations.

## Decision

### 1. Add a refresh-session encryption subject

Use the existing `EncryptionPort` credential-encryption adapters for persisted retry results. Their authenticated context contains exactly `{"refresh_session_id":"<session UUID>"}`. The session identifier is stable and contains no credential material.

This ADR supersedes only ADR 008's approved subject list to permit `refresh_session_id`. The exactly-one-subject rule and existing `service_id` and `kid` namespaces remain unchanged. Future subject keys still require an ADR amendment or superseding ADR.

### 2. Give one agent-scoped coordinator transaction ownership

An authorization coordinator serializes operations for each agent and owns their complete unit of work. All participating authorization and session repositories use that scope. PostgreSQL uses one transaction and an agent-row lock. Memory stages touched rows and publishes them only after validation.

Fosite retains OAuth protocol handling but borrows the coordinator's transaction. It cannot commit or roll back that transaction independently. Authorization decisions use the backend's shared clock within the coordinated scope. Signing preparation and external metadata retrieval stay outside the database lock.

## Rationale and Alternatives

### Encryption subject

- A session-specific subject binds retry ciphertext to its authorization lineage without changing existing encryption namespaces.
- Reusing `service_id` or `kid` misrepresents the protected resource and its branch-key namespace.
- JWE-only persistence uses a different encryption mechanism from the feature's required credential backend.
- Reusing `EncryptionPort` keeps encryption policy and key ownership in one existing subsystem.

### Transaction ownership

- Independent transactions can commit authorization changes without the corresponding session changes.
- An inner Fosite commit can publish rotation before the outer operation persists its recovery result.
- Process-local locks cannot coordinate broker instances. Grant-row locks cannot cover missing grants or agent-wide credential actions.
- Agent-scoped ownership gives those operations one lock order and one commit boundary.
- Finer-grained session locks permit more concurrency but add lock-order complexity. The design accepts lower per-agent concurrency instead.

## Consequences

- Existing service and signing-key encryption identities remain stable. Refresh sessions add a distinct subject namespace using the same vetted adapters.
- Participating repositories and Fosite must join the coordinator's scope. Their interfaces and wiring belong in the implementation plan.
- Operations for one agent serialize. A slow operation delays other operations for that agent.
- Memory needs staged writes rather than no-op rollback or full-store copies.
- External calls and transaction-unaware database lookups must stay outside the lock to prevent long lock holds and pool deadlocks.

Retry policy, lifetimes, error precedence, payload fields, migration rules, and acceptance tests remain in the feature documents. This ADR does not redefine them.

## References

- [Feature specification](../specs/049-fix-refresh-consent/spec.md)
- [Implementation plan](../specs/049-fix-refresh-consent/plan.md)
- [Data model and payload fields](../specs/049-fix-refresh-consent/data-model.md)
- [Storage interfaces and transaction contracts](../specs/049-fix-refresh-consent/contracts/storage.md)
- [Research and integration evidence](../specs/049-fix-refresh-consent/research.md)
- [ADR 004: Storage Layer Architecture](004-storage-layer-architecture.md)
- [ADR 008: Encryption Context Optimization](008-encryption-context-optimization.md)
- [ADR 014: OAuth2 Server Mode](014-oauth2-server-mode.md)
