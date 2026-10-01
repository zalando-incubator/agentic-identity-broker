# Storage and Issuer Contracts

**Status**: Design contract for implementation. These symbols are proposed, not present production APIs.  
**Model**: [data-model.md](../data-model.md)  
**Decision record**: [ADR 038](../../../adrs/038-consent-bound-refresh-sessions.md)

## Existing boundaries and shared time

- `ports.StorageTransactionManager` remains the context-based transaction primitive. Fosite types do not enter ports.
- `ports.TokenMintingStrategy.HandleRefreshToken` and `ports.TokenResponse` retain their public signatures and fields.
- Agent, grant, credential, code, and PKCE repositories remain the owners of their records.
- Migrate the **existing** `ports.UserDelegationVerifier` and all callers. It reports active, missing, expired, or infrastructure error at the caller's explicit decision time; it must not read node-local time.

Proposed ports in `internal/ports/storage.go` and `internal/ports/oauth2.go`:

```go
type AuthorizationClock interface {
    Now(ctx context.Context) (time.Time, error)
}

type AuthorizationSessionCoordinator interface {
    Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error
}

type UserDelegationDecision struct {
    Status     UserDelegationStatus
    GrantID    id.GrantID
    ValidUntil *time.Time
}

type UserDelegationVerifier interface {
    VerifyUserDelegation(ctx context.Context, principal id.Principal, agentID id.AgentID, decisionTime time.Time) (UserDelegationDecision, error)
}
```

`AuthorizationClock` obtains PostgreSQL `clock_timestamp()` through the context's ambient executor, including in a transaction. Memory uses the same injected clock as its coordinator. `Run` acquires the agent gate before calling `Now(ctx)` and passes that one decision time to its callback. After a mint or eligible-result decryption, a successful path calls `Now(ctx)` again inside the same scope and rechecks consent, effective lifetimes, current agent refresh-grant capability, and candidate response-scope eligibility before commit. A clock error returns `server_error` without token or request mutation.

Change the existing verifier implementation and consent-domain grant checks to accept `decisionTime` explicitly and return a `UserDelegationDecision`. Active status requires a nonzero `GrantID`; `ValidUntil` is nil or strictly after `decisionTime`. Missing status has no grant ID; expired status returns no usable grant. The issuer obtains `OriginalGrantID` from this decision at first issuance and requires the same ID on later refreshes. An active-grant edit keeps its ID. Deletion or expired-grant renewal revokes old roots even if a later grant is active. Migrate every existing caller, including impersonation, local token exchange, authorization-code issuance, and other consent consumers. Callers outside an agent-scoped operation obtain decision time from `AuthorizationClock`; none can bypass the verifier for a direct grant lookup or retain a node-local deadline fallback.

`Run` consumers are initial local issuance, local refresh, both consent-deletion paths, expired-grant renewal, active-grant edits, agent deletion, credential revocation, and credential replacement. Replacement retains sessions but must recheck current credentials. Public-origin sessions require current credential authentication once the agent becomes confidential.

### Coordinator semantics

- Acquire the agent gate before reading or mutating authorization and session rows. All participating repositories use the callback context, including reads.
- A nil callback error commits its staged changes. A callback error rolls them back before commit. Storage errors never produce successful lifecycle responses. A lost commit acknowledgement can leave the outcome indeterminate rather than rolled back.
- Bound-client prohibited reuse is the only authorization rejection that commits mutation. Commit revocation and ciphertext erasure before returning `invalid_grant`. An unacknowledged commit returns token-free `server_error` and follows FR-031.
- Other authorization denials and confirmed pre-commit failures preserve root, token, retry count, and deadlines. These include expired grant/session/current-token, missing current grant capability, withdrawn eligible response scope, unavailable consent, failed authentication, wrong client, and expired retry access. A consumed token's individual expiry does not bypass prohibited-reuse revocation. Startup and maintenance commit terminal expiry separately.
- Missing agents preserve caller-specific not-found behavior. Token requests do not recreate a deleted agent or root.
- The coordinator is non-reentrant. Fosite transactions join the owner scope without committing or rolling it back.
- Lock order is agent scope, authorization reads/writes, root/token lookup, anchored legacy current-row lock/re-read, then fresh consumption and matching descendant updates. Never acquire the agent gate after a child lock. Hold the anchored legacy lock through the rotation transaction; do not trust a new-table unused token before this re-read.

PostgreSQL uses one transaction and agent-row `FOR UPDATE`. Every repository, verifier, legacy mirror, and subject-bound storage lookup selects the ambient executor. Nested credential replacement and Fosite handlers join without committing the owner's transaction.

Memory receives one shared coordinator and clock from the storage factory. It stages only affected rows and publishes after complete validation. A failed callback publishes nothing; a no-op rollback or full-store snapshot is not acceptable.

For an indeterminate PostgreSQL commit, the caller returns token-free `server_error` without a rollback claim. A later request acquires the agent guard and resolves authoritative durable state, including the locked anchored legacy current row, before normal token classification. An unused current token can rotate only if its mirror remains unused. If a consumed mirror has matching new-token/history evidence, classify the committed predecessor normally; unmatched old-writer consumption makes the original and unsupported descendant `invalid_grant`, with no mutation or successor. A committed predecessor can return only its existing eligible result. Zero reuse retains prohibited-reuse revocation and requires fresh authorization after committed consumption. Unavailable or inconsistent state remains `server_error`. Do not retry a commit blindly, mint another successor, refund a committed retry count, or bypass current authorization.

## Focused refresh repository facets

Each facet remains below the constitution's seven-method limit. One concrete backend can implement multiple facets without exposing them all to every consumer.

```go
type RefreshSessionRepository interface {
    Create(ctx context.Context, session *storage.RefreshSession) error
    FindByID(ctx context.Context, sessionID id.RefreshSessionID) (*storage.RefreshSession, error)
    Save(ctx context.Context, session *storage.RefreshSession) error
}

type RefreshTokenRepository interface {
    Create(ctx context.Context, token *storage.RefreshToken) error
    FindBySignature(ctx context.Context, signature string) (*storage.RefreshToken, error)
    MarkUsed(ctx context.Context, signature string, usedAt time.Time) error
}

type RefreshSessionRevocationRepository interface {
    RevokeByID(ctx context.Context, sessionID id.RefreshSessionID, at time.Time, reason storage.RefreshRevocationReason) error
    RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error
    RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error
}

type RefreshSessionMaintenanceRepository interface {
    ListActive(ctx context.Context, afterID id.RefreshSessionID, limit int) ([]*storage.RefreshSession, error)
    ListDue(ctx context.Context, at time.Time, limit int) ([]*storage.RefreshSession, error)
    DeleteTerminal(ctx context.Context, before time.Time, limit int) (int, error)
}
```

Domain enums and models use project-native ID and timestamp types. None imports Fosite or SQL. List results have stable UUID ordering and bounded positive limits. `ListDue` selects stored deadline and retry-cleanup candidates; the domain evaluates policy inside `Run` at shared time. Maintenance also scans active roots after a policy change so shorter inactivity deadlines persist before readiness.

`Create` rejects conflicts. `Save` enforces immutable root ID, `OriginalGrantID`, `OriginalTokenSignature`, owner, scope ceiling, `StartedAt`, and `BranchKeyID`. It rejects `RetryCount` outside `0..3`, a decreasing `RetainUntil`, or reversal of terminal state. While the root remains in its current activity interval, `Save` cannot increase its persisted `InactivityExpiresAt`. Only a fresh successful rotation resets `LastFreshAt` and sets both the successor's immutable `ExpiresAt` and the new `InactivityExpiresAt` from the rotation time and current configured inactivity lifetime. `MarkUsed` requires an unused token and the coordinator time. Revocation methods are idempotent; they erase ciphertext and update root and matching live legacy current rows within the same transaction. Terminal expiry uses the same scoped root/mirror atomicity without adding another repository port.

`RetainUntil` starts at the first token's finite `ExpiresAt` and becomes the maximum of every recorded root-token expiry. Include matching legacy descendants' recorded expiries conservatively during revocation without treating those rows as trusted authorization. Consumed signature history and terminal revocation state remain through this bound. `DeleteTerminal` deletes only terminal roots with `RetainUntil <= before` (a shared-clock value); it never deletes active lineage. Agent deletion can remove roots after the revocation audit record exists.

For DB-008, RevokeByID/RevokeByAgent/scoped revocation writes an immutable receipt to refresh_revocation_receipts before any root or agent cascade removal. It contains non-credential root ownership, reason, shared timestamp, and redacted context. No agent/root delete cascade removes the receipt. A receipt-write failure rolls back the lifecycle action. Success logs follow commit; they do not replace this transactional record.

`FositeStorage.RevokeRefreshToken` maps the original authorization-code request UUID to `id.RefreshSessionID` and calls `RevokeByID`, including on code replay. Scoped revocation covers all roots for the principal/agent, even when a grant is already absent. Explicit credential revocation revokes agent roots; credential replacement does **not**.

Migrate all factory, provider, cleanup, fixtures, and test callers from source-level `RefreshTokenSessionRepository` and `RefreshTokenSession` to the root/token facets and models. Remove those source symbols, aliases, and runtime fallback paths. Retain the old SQL table for rolling old binaries; the new repository implements any legacy mirror internally, not as a second domain model or port.

## Provider ownership, encryption, and protocol delegation

Provider construction requires the verifier, coordinator, clock, refresh facets, resolved policy, existing `EncryptionPort`, and branch-key provisioning for KMS-backed deployments. Missing dependencies fail startup. Neither consent nor durable retry has an optional bypass.

The typed `BranchKeySubjectKindRefreshSession` contains only a nonzero `id.RefreshSessionID`. Its AAD is exactly `{"refresh_session_id":"<session UUID>"}`. Its deterministic key ID is `refresh_<UUID>_branch_key` and must equal root `BranchKeyID`. Extend typed parsing and KMS routing, never reinterpret `service_id` or `kid`. The issuer provisions needed branch keys through existing `BranchKeyManager`; it prepares that work and signing material outside the SQL lock when the initial session ID is known. The raw-key and KMS adapters remain the vetted `EncryptionPort` implementations. Subject-bound reads inside a transaction use its ambient executor.

For an authorization-code exchange:

1. Prepare client authentication, branch-key provisioning, and signing material outside the database transaction for the candidate original request UUID.
2. Acquire the agent scope and its shared decision time. Re-read the code and current credential/client identity. Get the active grant ID and deadline through `UserDelegationVerifier` with that time; persist its `GrantID` as immutable `OriginalGrantID` with first-token evidence.
3. Let Fosite validate code, redirect, and PKCE and populate the result in a borrowed transaction.
4. Create the root, the original token with `IssuedAt`/finite `ExpiresAt`, and a legacy-compatible row with `session_id` in one transaction. Initialize `RetainUntil` from that expiry. Do not create a root or anchor from a later refresh.
5. Recheck authorization/deadlines using the shared clock and return the result only after the owner commits. A replayed authorization code revokes the root it created.

For a refresh:

1. Preflight authentication, legacy/root signature lookup, branch-key provisioning for the known root, and signing preparation without trusting them as final authorization. Resolve CIMD metadata outside `Run`.
2. Acquire the agent scope and decision time. Re-read token, root, current credential/client identity, and agent registration through the ambient executor.
3. Check client binding and root revocation before calling `UserDelegationVerifier`. Require active status and `GrantID == OriginalGrantID`, then check stored/current session deadlines, including the current token's immutable expiry. Effective inactivity is `min(InactivityExpiresAt, LastFreshAt + configured inactivity, current token ExpiresAt)`; a policy increase cannot extend the issued token or current activity interval. Missing or expired consent denies without mutation. Unavailable authorization returns `server_error` before commit.
4. Check current agent refresh-grant capability before token classification. If removed, preserve the existing `unauthorized_client` outcome without mutation. Consent and lifetime failures win over capability removal; capability removal wins over prohibited reuse.
5. Before trusting an anchored current token's unused state, lock and re-read its matching legacy current row, holding that lock through rotation. An old writer's unmatched consumption makes the original and unsupported descendant `invalid_grant` without request mutation or a new successor. Classify the signature as current, immediate predecessor, or bound-client prohibited reuse. An unused current token must remain individually unexpired. A consumed signature remains replay evidence after its individual expiry. Enforce predecessor requested-scope match, fixed window, unused successor, count, and access expiry before decrypting. Older, out-of-window, scope-mismatched, and fourth otherwise-eligible presentations revoke even if the old response scope was withdrawn.
6. Delegate only a fresh-current rotation to Fosite in the borrowed transaction. It prepares the candidate response and successor. The requested narrower scope limits only the returned access token; the root `Scope` ceiling stays unchanged. For an otherwise eligible retry, decrypt and compare the stored original response and all bindings. Never decrypt a prohibited retry just to inspect scope.
7. Check candidate response-scope eligibility against current agent permissions only on a fresh or otherwise eligible retry. On retry use the exact original response scope, not a narrower new request. Preserve existing `invalid_scope`/`scope_not_granted` conventions; a withdrawn candidate response scope denies without state mutation.
8. On an allowed fresh path, save one successor, consumed predecessor, clocks, `RetainUntil`, `RetryCount = 0`, and the original result encrypted with `EncryptionPort` using the root subject. Save matching anchored legacy rows atomically. A zero reuse interval saves no ciphertext. On an allowed retry, return the **original** token pair with remaining `expires_in`, increment only `RetryCount` (up to 3), and audit the comparison with `OriginalRequestContextFingerprint`.
9. For prohibited reuse, commit revocation before returning `invalid_grant`. Roll back other authorization denials and pre-commit errors. Recheck all applicable FR-029 authorization and eligibility conditions at a second shared time before success. Include client binding, revocation, consent, effective lifetimes, capability, token eligibility, and candidate response scope. Validate the staged outcome without treating this request's own consumption as replay or counting its retry increment twice. An indeterminate commit returns token-free `server_error` and follows FR-031, not a rollback claim.

The payload contains exact `refresh_retry` purpose, version 1, immutable root and original-grant/first-token evidence, owner/client binding, predecessor/successor signatures, original normalized requested and response scopes, original request-context fingerprint, token strings, and original access/retry expiries. Validate every binding after authenticated decryption. Missing keys, tampering, or mismatches cause `server_error`, never a fresh mint.

Existing Fosite client capability, audience, current agent permission, and scope validation also apply to retries, without calling its consumed-token replay path. A removed refresh grant returns the existing `unauthorized_client` outcome after consent/lifetime checks, before classification. A retry cannot return its original access scope once the agent withdraws it; that eligible-result denial preserves the established `invalid_scope`/`scope_not_granted` status convention and leaves state unchanged. Different normalized requested scope remains prohibited reuse, not a scope-eligibility denial. The first consumed-token use fixes the reuse deadline. Exactly three stored results can be returned for that predecessor while both the 30-second default window and original access expiry remain open. A fourth otherwise eligible presentation is prohibited reuse. An expired original access token denies recovery but does not revoke an otherwise valid current refresh token.

Signing preparation is request-local and preserves signing-cache invalidation. Record actual JWT expiry from the access-token strategy's mint timestamp, rather than decoding JWT claims again. Retry encryption and decryption must not fetch signing keys via a transaction-unaware connection while holding the only SQL connection.

Erase **live** retry ciphertext at `min(ReuseUntil, RetryAccessExpiresAt)`, at successor rotation, or on terminal state. A deadline-driven maintenance job clears due ciphertext and terminalizes due roots through the shared clock; startup clears overdue ciphertext before readiness. Before any policy increase, maintenance checks stored deadlines for elapsed expiry; for active roots it persists shorter effective `InactivityExpiresAt` values bounded by current token `ExpiresAt`. A fresh rotation alone starts a new activity interval under the then-current policy. Absolute lifetime can be recomputed from `StartedAt` for nonterminal roots after prior elapsed deadlines are ruled out. Terminal expiry commits `ExpiredAt`, the absolute/inactivity reason, ciphertext erasure, and matching unconsumed legacy mirror/current/descendant invalidation in one scoped transaction. A failed mirror write rolls back the transition and blocks readiness during startup reconciliation. Do not rely only on a one-minute sweep, and do not mutate request state for a deadline denial. Preserve predecessor metadata to classify an expired-access-token retry without revoking the current token. Logs and backups never contain plaintext retry results. Encrypted backup copies are allowed; if restoration cannot prove current revocation history, clear cached results **and require fresh authorization for affected roots** before returning tokens. Never promise physical erasure of encrypted backups.

## Required transaction participants and rolling upgrade

These operations use the coordinator context instead of a bare database pool:

- Grant lookup/deletion and updates used by the verifier, consent service, and expired-grant renewal.
- Agent lookup/deletion, including current refresh-grant capability checks.
- Credential lookup/delete/replacement. Replacement never opens an independent transaction and never revokes retained sessions.
- Authorization-code lookup/consumption and PKCE lookup/deletion.
- Root/token lookup, mutation, scoped revocation, expiry reconciliation, and SQL legacy mirror writes.

Keep the existing `refresh_token_sessions` table and add nullable `session_id UUID REFERENCES refresh_sessions(id) ON DELETE CASCADE` and `predecessor_signature TEXT` columns. An initial anchored row has no predecessor; a later anchored row records its previous signature. New issuers write anchored mirror rows and root/token records atomically; old binaries can still insert their original row shape. The current old format alone has no trustworthy first-token, original-grant, or explicit ancestry evidence. Unsupported old rows and unanchored old-writer descendants require fresh authorization under FR-025. Continue an independently evidenced family only with proof of its active grant at first issuance, first token, original issuance time, owner/client, every rotation link, last fresh activity, and revocation history. Never synthesize `StartedAt` from a later row, `created_at`, `request_id`, deployment, or token expiry.

Revocation and terminal expiry mark the root and every matching unconsumed legacy current/descendant row used in one scoped transaction, while extending `RetainUntil` to cover matched legacy expiries. Terminal expiry also writes `ExpiredAt`, its reason, and ciphertext erasure in that transaction. An old writer's `MarkUsed` predicate (`used_at IS NULL`) cannot succeed after either commit. Lock and recheck legacy current and descendant rows after concurrent old-writer transactions; a successor cannot escape revocation or expiry. A failed legacy update rolls back the entire terminal maintenance transition and blocks startup readiness. Before fresh consumption, lock and re-read the anchored mirror while holding the agent scope and hold that row lock through rotation: old-first exposes unmatched used state and unsupported lineage; new-first blocks `MarkUsed` until rotation commits. Never invent first issuance, a consumption timestamp, or a retry result for an old-writer descendant. Keep the legacy table in the upgrade. A down migration must not reactivate revoked or expired sessions. Binary-only rollback without a down migration cannot restore terminal sessions either. Quiesce token traffic and old writers and reconcile outgoing-policy expiry before admitting old binaries on rollback; keep down/reapply protection and unrelated records intact.

## Failure and concurrency proofs

- Two current-token refreshes serialize and converge on one successor under nonzero reuse. With zero reuse, the later bound-client use revokes the root. In a mixed rollout, old-first mirror consumption makes the anchored original and unsupported descendant `invalid_grant` without mutation; new-first mirror locking prevents the old writer's conditional `MarkUsed` from succeeding after rotation.
- Lifecycle revocation or terminal expiry leaves no affected usable refresh root or legacy current/descendant row, including a concurrent old-writer replacement. Neither transition touches other agents or principals. An expiry mirror-write failure rolls back the whole transition and blocks readiness at startup.
- Confirmed lifecycle rollback preserves both authorization and session state. An indeterminate commit returns failure without a partial-transaction or rollback claim. Replacement secrets require an acknowledged successful commit.
- A mint, branch-key, encryption, verifier, or clock failure leaves the predecessor usable and creates no partial successor.
- Confirmed pre-commit errors and non-replay authorization denials preserve session/token state. A lost commit acknowledgement returns token-free `server_error`. Later requests resolve durable state and follow normal retry rules, including zero reuse and committed retry counts.
- An original access token expiring inside the reuse interval denies retry without revoking the current token. A retry cannot increase deadlines or return a fourth stored result. Missing current agent capability takes precedence over prohibited reuse; prohibited reuse takes precedence over withdrawn old response scope.
- One-connection PostgreSQL tests complete without a second SQL connection during signing preparation. Memory stages only touched records, and PostgreSQL tests prove restart/replica outcomes. For US2-S10, first prove an eligible bounded retry returns the identical result; then code replay must reject its current and still-retry-eligible predecessor tokens while an unrelated session stays usable. Do not mark US2-S10 complete before US3/T050/T072.
