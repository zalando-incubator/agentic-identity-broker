# Research: Consent-Bound Refresh Sessions for Local Token Minting

**Feature**: [spec.md](spec.md)  
**Date**: 2026-09-30  
**Status**: ADR 038 is accepted. Open Decisions and dependent planning remain provisional until T001. API/release approval still precedes runtime implementation.

## 1. Continuing authorization

**Decision**: Inject `ports.UserDelegationVerifier` into the local issuer and extend its native result with grant identity and validity. Pass the shared decision time explicitly. Require active delegation for the original principal, agent, and grant binding inside the authorization transaction.

**Rationale**: The port already isolates impersonation from the consent domain. It reports active, missing, expired, or an infrastructure error. Missing and expired delegation become `invalid_grant`. Unknown state becomes `server_error`. No configuration disables this check.

**Alternatives considered**: Importing `consent.Service` into the issuer duplicates cross-context coupling. Checking only before acquiring the transaction leaves a revocation race. Checking only during initial authorization preserves the reported exposure.

**Evidence**: `internal/ports/oauth2.go:270–277`; `internal/app/impersonation_delegation.go:13–35`; `internal/domain/consent/service.go:642–682`; accepted ADR 032. The user's vulnerability report is the planning baseline, not a request to reproduce it.

## 2. One agent-scoped unit of work

**Decision**: Add `AuthorizationSessionCoordinator.Run(ctx, agentID, operation)` and `AuthorizationClock.Now(ctx)`. The callback receives shared decision time after the agent lock. PostgreSQL uses `clock_timestamp()` through the ambient executor. Memory uses its single coordinator clock. Migrate deadline checks away from replica-local time.

**Rationale**: The existing memory transaction manager is a no-op. PostgreSQL lifecycle methods also bypass its ambient executor. A mutex alone cannot roll back a partial lifecycle change. A transaction alone cannot stop a stale read followed by new issuance.

The coordinator stages memory changes before publishing them. It does not clone whole repositories. PostgreSQL reads and writes use the same transaction context. All callers acquire the agent guard before child records. For fresh anchored consumption, lock and re-read the legacy current row after root/token lookup, before trusting new-table unused state; hold it through rotation. The conservative design serializes new refreshes for one agent, while legacy writers are fenced by their conditional `MarkUsed` on that row. Signing-key preparation stays outside the guard, and refresh work does not call an upstream provider.

**Alternatives considered**: Independent service locks fail across replicas. Grant-row locking cannot protect a missing grant or agent-wide credential revocation. Shared agent locks plus family locks permit more parallelism but add another lock mode without a measured need. Full-store memory snapshots add avoidable copying.

**Evidence**: `internal/adapters/storage/factory.go:19–26,91–115,128–152`; `internal/adapters/storage/postgres/transaction.go:13–55`; lifecycle research inspected grant, agent, and credential adapters. PostgreSQL already cascades agent deletion through migration 023; memory does not.

## 3. Preserve Fosite protocol handling without nested commits

**Decision**: Put refresh policy in a small issuer-domain coordinator. Authenticate and bind the client before side effects. Check revocation, active consent, effective lifetimes, and current agent refresh-grant capability, then classify token reuse. Capability removal retains the existing `unauthorized_client` behavior without mutation; consent or lifetime failure wins over it, and it wins over prohibited reuse. Classify permitted retries before calling Fosite. Delegate fresh rotation to the pinned Fosite handler under an explicitly borrowed outer transaction.

For a fresh or otherwise eligible retry candidate, check the response scope against current agent permissions after classification. Preserve existing `invalid_scope`/`scope_not_granted` conventions without mutation. A retry checks the exact original response scope, not a narrower new request. Older, out-of-window, scope-mismatched, or fourth otherwise-eligible reuse still revokes, even if the old response scope was withdrawn; never decrypt prohibited reuse merely to inspect scope.

`FositeStorage.BeginTX` joins the coordinator context when present. Its inner `Commit` and `Rollback` never end the owner transaction. Refresh authorization denials and pre-commit failures roll back request changes. Lost commit acknowledgement returns token-free `server_error` without a rollback claim. Later requests resolve durable state under the guard and normal retry policy. For refresh-token presentation, only bound-client prohibited reuse commits an authorization rejection. Code replay separately follows FR-040. Startup and maintenance own expiry transitions. Repeat authorization and eligibility checks before a successful commit.

Authorization-code redemption joins the same guard before creating a refresh session. It rechecks delegation at the issuance boundary. Credential authentication prepared before the guard is revalidated against the current credential identity inside it. Code replay revokes the session created by that code even when another client authenticates as itself to replay it. FR-030 and SR-009 protect refresh-token presentations, not code replay. Unrelated sessions remain usable.

**Rationale**: Fosite v0.49.0 treats an inactive refresh token as replay before checking client binding. It also opens and commits its own mutation transaction. Wrapping it with an unrelated outer transaction is unsafe.

Signing material can request another database connection. Prepare it with the existing signing-material service before opening the authorization transaction. Pass the prepared material through request-local issuer context, not mutable provider fields. The existing strategy retains signing and claims logic.

Resolve CIMD metadata before the guard as well. Fosite storage reuses that request-local client snapshot for the matching client ID, while re-reading local agent and credential state inside the transaction. No metadata fetch occurs under the database lock.

**Alternatives considered**: Pretending a consumed token is active allows another successor. Copying the complete Fosite refresh handler creates a second OAuth implementation. Letting an inner handler commit early loses atomic retry-result persistence. Minting with transaction-unaware key reads can deadlock a one-connection pool.

**Evidence**: pinned `github.com/ory/fosite v0.49.0`, `handler/oauth2/flow_refresh.go:45–56,81–112,123–164`; `flow_authorize_code_token.go:155–187`; `storage/transactional.go:20–56`; `internal/domain/oauth2server/fosite_storage.go:79–102`; `strategies.go:82–101`. ADR 014 keeps Fosite types inside the issuer domain.

## 4. Durable session root and hashed token history

**Decision**: Replace the token-only storage model with a `RefreshSession` root and `RefreshToken` records. Generate `id.RefreshSessionID`. The root UUID uses the authorization-code UUID already used as Fosite's request ID. Store immutable ownership and original issuance time once per session.

Trust only anchored root/token records written by new issuers. Require the original active grant ID, first issued token and issuance time, principal, agent, client, complete ancestry, last fresh rotation, and revocation history. Every pre-feature unanchored row and unsupported old-writer descendant requires fresh authorization. An anchored new-issuer family can continue across rollout or restart only with complete evidence and current authorization. No external legacy evidence source is assumed.

Keep consumed-token signatures and revocation state through the last recorded token expiry. Store one current signature and at most one previous-token result, its normalized requested scope, accepted-retry count, and original request-context fingerprint. Narrowed access-token scope never replaces the root's original scope ceiling.

**Rationale**: Existing token rows do not provide a trustworthy original start after rotation, nor the identical response required after a lost response. Deleting consumed signatures solely by their old token expiry loses replay-to-family association while descendants remain active.

**Alternatives considered**: Copying family metadata onto every token permits divergence. Treating each rotated token's creation as session start defeats absolute lifetime. An in-process retry cache fails restart and replica acceptance. A new refresh-token wire format is unnecessary.

**Evidence**: `internal/domain/storage/refresh_token_session.go:11–23`; `internal/domain/oauth2server/fosite_storage.go:165–166,231–241`; `migrations/023_create_refresh_token_sessions.up.sql`; ADR 013.

**Tradeoff**: Consumed signature history grows for a continuously active session without an absolute deadline. Keep active lineage and terminal state through the maximum recorded token expiry. Agent deletion records a transactional audit receipt before cascade removal. Cleanup never discards live-family tombstones.

## 5. Bounded, idempotent retry of the last refresh

**Decision**: A first successful consumption establishes a fixed `reuse_until`. A retry can return the same access and refresh token only for the immediately previous signature, while its successor remains current and unused. Recheck consent and lifetimes before token classification; check current grant capability before classification. Check the original response scope only after the retry otherwise qualifies and its stored result decrypts successfully.

Retries preserve session start, last fresh activity, consumption time, and deadlines. `expires_in` reports the original JWT's remaining validity. Expired original access blocks recovery without revoking a valid current refresh token. Older or out-of-window consumed signatures revoke a live session even after their individual expiry. A withdrawn old response scope cannot bypass that prohibited-reuse classification.

The default retry interval is 30 seconds from first successful consumption. It is a fixed deadline, not a sliding window. The retry returns the original token pair rather than performing another rotation.

At most three committed retry authorizations are allowed per consumed token. Lost responses and commit acknowledgements do not refund the count. A fourth otherwise eligible presentation or different normalized requested scope is prohibited reuse. Failed authentication, client mismatch, and confirmed rollback preserve state. Indeterminate commits follow FR-031, including zero reuse. Audit context changes and count outcomes through existing telemetry.

**Rationale**: This handles a lost response without creating branches or turning repeated requests into fresh authorization activity.

**Alternatives considered**: Rotating again for every duplicate branches or invalidates the successor. Returning a newly minted access token changes the specified token result. Returning expired credentials is not successful recovery.

**Commit outcome decision**: A lost acknowledgement cannot prove rollback. The ordinary durable root/token state resolves a later request without another journal or recovery bypass. A committed predecessor can recover only its eligible original pair. Zero reuse retains strict replay revocation. Unavailable resolution stays a token-free server error. Test actual PostgreSQL acknowledgement loss for rotation and retry-count commits, not a mock commit result.

**Evidence**: specification FR-011–016 and FR-028; current Fosite handler has no grace implementation. The current issuer stores only token hashes, so encrypted result persistence is a required addition.

## 6. Reuse existing credential encryption at rest

**Decision**: Encrypt the retry payload through `ports.EncryptionPort` in the issuer domain. Add a typed refresh-session subject with the single non-secret AAD key `refresh_session_id`. Provision its deterministic `refresh_<UUID>_branch_key` through the existing branch-key manager. Record the branch identifier on the root.

Payload fields bind purpose/version, session, original principal/agent/client, predecessor, successor, normalized requested scope, original token strings, and expiry. Decrypt only after live eligibility checks and compare all bindings. Crypto failure returns `server_error` without consuming a token or advancing a counter.

**Rationale**: Current DB-005 requires the existing at-rest credential mechanism. The former JWE-only proposal did not meet that integration requirement. Both raw-key and AWS adapters retain their vetted authenticated-encryption implementations.

**Alternatives considered**: Plaintext results are forbidden. Repurposing `service_id` or `kid` breaks subject isolation. A process-local result cache fails restart/replica requirements. Custom cryptography is unnecessary.

**Approval boundary**: Accepted ADR 038 extends ADR 008's approved subject list. It preserves the exactly-one-subject AAD rule. API/release approval and implementation remain pending. Keep Fosite types and plaintext out of repository interfaces.

**Restore decision**: After any full database or refresh-state restore, invalidate all restored local refresh authority before admission. Stop token traffic and all writers before restoring. The supported one-shot command is `agentic-identity-broker --config <file> refresh-sessions invalidate-restored`. It uses the existing loader, builder, storage, coordinator, clock, and maintenance. It starts no HTTP server and returns no credentials.

**Rationale**: Snapshot-local rows and receipts cannot prove post-snapshot revocation or authorization. A snapshot-history proof can revive revoked sessions. No automatic restore detector, new public endpoint, or new runtime configuration is part of this design.

**Operation**: Scan agent IDs from roots and legacy rows in bounded batches. Under each agent transaction, revoke active roots with `restore_invalidation`, erase ciphertext, and mark every unused legacy row used. Include unanchored rows without roots. Preserve existing terminal reasons, receipts, grants, agents, credentials, signing keys, and third-party sessions. On failure or an indeterminate commit, exit nonzero and keep all brokers offline. Rerun idempotently until acknowledged success includes a final scan with zero active roots, ciphertext, and unused legacy rows. Fresh authorization creates new sessions after admission.

**Evidence**: `internal/ports/encryption.go`; `internal/domain/encryption/subject.go`; `internal/adapters/encryption/branchkey/id.go`; accepted ADRs 008/009/012. The existing supplier derives branch IDs without a second PostgreSQL lookup. Prepare provisioning before the guard where possible and bound crypto work by the operation context.

## 7. Lifetimes and policy changes

**Decision**: Compute a finite absolute deadline from `StartedAt`. For the current activity interval, effective inactivity is `min(InactivityExpiresAt, LastFreshAt + configured inactivity, current token ExpiresAt)`. Persist effective deadlines in addition to the original clocks. A session becomes terminal when an effective deadline is reached.

At startup after a policy change, reconcile active roots before readiness. Check stored deadlines for elapsed expiry before applying a policy increase. Persist a shorter `InactivityExpiresAt` for a still-active root, bounded by its current token's immutable expiry. Later increases cannot lengthen that token or current activity interval; only a fresh valid rotation sets the successor's `ExpiresAt`, new `InactivityExpiresAt`, and `LastFreshAt` from the rotation time and current configured lifetime. A nonterminal absolute deadline can be recomputed from the original `StartedAt` under CR-005 after ruling out old elapsed deadlines. Runtime refresh checks these bounds under the guard without terminalizing on a denied request.

The sealed payload's `retry_expires_at` stays the original `min(ReuseUntil, RetryAccessExpiresAt)` and must match the root's original deadlines. Root `RetryExpiresAt` is the persisted effective minimum of that bound, shorter current reuse, effective session deadlines, and its earlier value. It never increases for one predecessor. The payload deadline must be at least the effective deadline. Both must be after decision time before returning a result. A shorter effective deadline does not reencrypt an unchanged result.

At the effective deadline, deny retries even if cleanup has not run. With reachable storage, deadline-driven maintenance erases ciphertext within one second. A missed bound or unavailable cleanup storage blocks readiness until overdue ciphertext is cleared. Startup persists shorter effective deadlines and erases overdue results before readiness. Zero reuse erases all cached results before readiness and never creates new results. Rotation, revocation, and terminal expiry erase ciphertext in the same transaction.

**Rationale**: Recomputing from a longer inactivity configuration without the stored deadline or current token expiry extends an already-issued session. A shorter configuration must persist its bound before a later increase can erase its history. Recomputing session start on restart resets the absolute limit. Checking elapsed stored deadlines before a policy increase prevents resurrection after downtime.

**Alternatives considered**: Apply changes only to new sessions conflicts with CR-005. Resetting clocks during migration or restart grants additional time. Using downstream resource activity requires an unrequested reporting mechanism.

**Evidence**: specification CR-005 and FR-017–025. Fosite resets refresh expiry on rotation in `flow_refresh.go:109–112`, so its lifespan alone is not the absolute-session policy.

## 8. Explicit-zero configuration

**Decision**: Model unresolved local reuse interval as `*time.Duration` in `LocalModeConfig`. Nil means omitted and receives 30 seconds during local/hybrid validation. A non-nil zero disables reuse. Resolved configuration carries a concrete duration. Do not install a global local-policy default that makes proxy mode appear to contain local configuration.

Absolute lifetime is a concrete non-negative duration with zero meaning unlimited. Retain `refresh_token_ttl` and its existing omission/zero-to-720h semantics. Use source strings such as `0s`, not bare numbers.

Add explicit environment and dotted-duration CLI bindings. Add all three values to Helm values, schema, ConfigMap, and README. Current chart, examples, and reference omit even the existing inactivity setting.

**Rationale**: Re-defaulting every zero to 30 seconds makes strict single use impossible to configure. A presence-aware field also preserves correct semantics for configurations constructed by the application and tests.

**Alternatives considered**: A plain duration cannot distinguish omission from explicit zero at normalization. Loader-only defaults leave direct configuration construction ambiguous and can conflict with proxy validation. A generic optional-duration framework is unnecessary.

**Observed feasibility**: A disposable program used the pinned Viper/mapstructure dependencies and the existing numeric-duration guard pattern. Omission decoded as nil. `0s` remained zero, `2m` remained two minutes, malformed input and bare numeric zero failed decoding. Negative duration decoded and was identified for validation rejection. The program was removed. This verifies decoding feasibility, not the unimplemented feature's loader wiring.

## 9. Lifecycle integration and rollback

**Decision**: Both consent deletion paths revoke by principal and agent, including idempotent deletion of an already absent grant. Agent deletion and explicit credential deletion revoke by agent. Credential replacement keeps sessions but immediately requires the new credential. Expired-grant renewal revokes pre-renewal sessions; active-grant updates preserve their clocks.

Memory mirrors the agent-related authorization cascades needed for consistency. Every participating read/write uses the coordinator context. A rollback discards staged memory changes or rolls back SQL. Audit and replacement-secret return occur only after commit.

**Rationale**: Deleting a grant alone leaves old refresh sessions available after regrant. Credential revoke can change client classification to public. Revocation must remain effective despite that fallback.

**Alternatives considered**: Relying only on PostgreSQL FK cascades does not handle grant or credential actions and differs from memory. Returning lifecycle success before revocation completes violates the acceptance contract.

**Evidence**: `consent/service.go:575–619`; `agents/service.go:163–169`; `oauth2server/credential_service.go:30–61`; lifecycle research verified all production deletion callers and transaction participation gaps.

## 10. Migration, validation, and delivery boundary

**Decision**: Use additive migration 036, subject to numbering checks. Retain the legacy table layout for rolling old writers and add root/ancestry metadata. New writers issue only anchored tokens. Before trusting an anchored current token as unused or consuming it, lock and re-read its matching legacy current row through rotation. Old-first consumption without matching new-token/history evidence makes both that original and its unsupported descendant `invalid_grant` without request mutation or a new successor. New-first locking blocks the older writer's conditional `MarkUsed` until rotation commits. FR-025 requires original first-issued-token evidence; never infer issuance time, a consumption timestamp, or a retry result from an old row. Recheck old-writer descendants on revocation.

Terminal absolute or shortened-inactivity expiry commits `ExpiredAt`, reason, ciphertext erasure, and matching unconsumed legacy mirror/current/descendant invalidation atomically under agent scope. A failed mirror update rolls back the whole maintenance transition and blocks startup readiness. Before admitting old binaries during rollback, quiesce token traffic and old writers and reconcile expiry under the outgoing policy. Binary-only rollback without a down migration cannot restore terminal sessions; down/reapply must also keep old mirrors unusable.

Map all 47 acceptance scenarios individually to production-bootstrap E2E journeys. Every primary It needs an acceptance-linked semantic failure before runtime implementation. For US2-S10, first prove a bounded identical-result retry succeeds. Then have another client authenticate as itself and replay the authorization code. Both current and still-retry-eligible predecessor tokens must fail, while an unrelated session remains usable. Complete US2-S10 only after US3/T050/T072. Keep focused integration regressions and unchanged baseline regressions separate from the 47 primary scenarios. Use PostgreSQL for restart, replica, shared-clock, acknowledgement-loss, and rolling-version proofs. Test lifetime and retry boundaries at explicit shared times.

Canonical OpenAPI, rendered docs/api/oauth2-refresh-sessions.md examples, the Helm contract, and release review precede runtime edits. ADR 038 is accepted. T025 owns the complete core YAML/environment policy. US6 verifies it and adds source parity rather than implementing core defaults again. This feature changes renewal authority, not immediate downstream access-token validity.

**Provisional choice rule**: T001 confirms the Open Decisions before dependent work. If a choice changes, reconcile the spec, plan, contracts, examples, scenario mapping, and tasks. Validate traceability again. Keep the existing API stakeholder-review order unchanged.

**Rationale**: Existing history cannot meet the new lineage guarantees. Memory is intentionally ephemeral. A design document cannot claim that new behavior or tests already exist.

**Alternatives considered**: Guessing legacy session start silently extends authority. Testing only mocks does not prove cross-repository atomicity. Long real-time waits are unnecessary. Adding introspection or a new revocation endpoint exceeds the requested remediation.

**Evidence**: `tests/e2e/AGENTS.md`; `tests/AGENTS.md`; existing offline-access flow in `tests/e2e/oauth2_authorize_e2e_test.go:154–294`; migration numbering currently ends at 035.
