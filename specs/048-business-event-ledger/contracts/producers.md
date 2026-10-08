# Catalogue Producer and Transaction Map

All listed paths are existing integration targets unless explicitly marked **new**.
Every row is a required producer.
This map does not permit implementation of only representative event families.
Event names use the namespace in `events.md`.

| Event name | Domain owner / existing seam | Recorded boundary and no-op rule |
|---|---|---|
| grant-created | `internal/domain/consent/service.go`, GrantConsent (291-397) | The new grant and event commit together. A concurrent upsert loser creates no second occurrence. |
| grant-updated | Same, existing-grant branch. grantMatchesRequest (548-573) | Compare effective permissions/validity under lock. Equal state returns without an event. |
| grant-revoked | Same, RevokeConsent and RevokeConsentForPrincipal (578-620) | Capture the actual removed grant or return it from deletion. Preserve the distinct existing HTTP absence semantics. |
| grant-expired | Same, GetActiveGrants / VerifyAgentAccess / delegation reads (625-683), plus `internal/domain/oauth2/service.go` grant verification | The first successful effective-expiry marker update and event commit together. Repeated lazy recognition is silent. |
| session-established | `internal/domain/oauth2session/service.go`, HandleCallback -> createSession (474-666) | A successful authorization callback establishes usable session state, including reauthorization upsert. The event uses the authoritative persisted session ID returned by the upsert. Low-level Create alone does not establish this fact. |
| session-refreshed | Same, refreshLockedSession -> persistRefreshedSession, using UpdateRefreshedSession. GetValidAccessToken and ForceRefreshSession | Accepted encrypted state and the event commit together in a short owner transaction. A later exchange failure does not remove the fact. |
| session-refresh-failed | Same, refreshLockedSession, with persistRefreshedSession / rollbackRefreshWrite outcome classification | Record one known terminal refresh-attempt failure after any unsuccessful mutation rolls back. Indeterminate commit or rollback emits no contradictory failure fact. Internal network retries produce no separate events. |
| session-terminated | Same, TerminateSession | Only an actual removed session produces the event. Before deletion, capture subject/session/service references. Provider deletion preserves the session restriction. |
| authorization-requested | `internal/domain/oauth2/service.go`, AuthorizationService.HandleAuthorization (137-401) | Record each accepted validated authorization request before the consent/proceed response. Later authorization-code writes do not produce another event. |
| token-issued | `internal/domain/oauth2server/provider.go`, HandleClientCredentials, HandleAuthorizationCodeExchange, HandleRefreshToken (141-458). Proxy grant completion via `internal/adapters/http/enduser/token_grant_strategy.go` | The non-exchange response is ready. Local response-population writes and the event share the outer transaction. The proxy outcome commits before response bytes pass to the caller. |
| token-request-failed | Provider failures, client authentication/authorization decisions, and proxy failures reported from `internal/adapters/http/enduser/oauth2_token.go` / token_grant_strategy.go to the domain outcome service | Record the primary terminal token failure before response release, unless it is a specific exchange refusal. HTTP input rejected before credential authentication produces no event, even when the ledger is unavailable. |
| token-exchanged | `internal/domain/tokenexchange/service.go`, Exchange (170-405). Successful `internal/domain/impersonation/service.go`, Impersonate | A successful RFC 8693 result identifies the receiving agent. Commit the event before response release. Do not produce a token-issued duplicate. |
| token-exchange-denied | Same exchange/impersonation orchestration after typed authentication or authorization rejection | Use the specific primary denial classification. Do not also record token-request-failed for the same denial. Preserve independently known identities only. |
| impersonation-granted | `internal/domain/impersonation/service.go`, evaluateRule (250-285) | Authorization and active delegation passed. Record the permission decision before minting. The decision does not claim successful token issuance. |
| impersonation-denied | Same, final rule rejection through Impersonate | Record one final refused permission decision after rule evaluation, not one per failed candidate rule. Pre-rule scope validation is structural and produces no event. Do not mislabel mint/internal failure as refusal. |
| approval-requested | `internal/domain/approval/service.go`, CreatePendingApproval (415-548) | Only `IsNew` winning creation produces the event. An existing pending dedup return produces no event. |
| approval-approved | Same, ApproveApproval (235-286) | The winning pending-to-approved CAS, event, and sync-state mutation commit together. |
| approval-denied | Same, DenyApproval (349-410) | Only the winning user-denial CAS produces the event. Revocation that stores denied status does not produce this event. |
| approval-consumed | Same, ConsumeApproval (645-721) | Only the winning once-persistence consumption produces the event. An already-consumed 200 and a concurrent loser are silent. |
| approval-revoked | Same, RevokePermanentApproval (872-929), and applicable terminal cascades | Only actual permanent-approval revocation produces the event. A repeated transition produces no no-op event. |
| approval-expired | Same, GetApproval (188-230), mutation race handling, expired pending dedup retirement and filtered queries | The existing lifecycle recognizes effective expiry. The marker and event commit atomically. Do not add an expired enum or scheduler. |
| agent-registered | `internal/domain/agents/service.go`, Create (66-91) | Record the validated newly inserted agent. The event contains the agent ID, not display/configuration snapshots. |
| agent-updated | Same, Update (97-135) | Compare effective configuration under lock. The update and event commit together. Unchanged configuration produces no event. |
| agent-deleted | Same, Delete (163-169) | Actual deletion and child terminal facts commit in one transaction. Ledger records never cascade away. |
| credential-generated | `internal/domain/oauth2server/credential_service.go`, CredentialService.Generate | The first credential record creation and event commit before the plaintext secret response. The generation library alone does not establish the fact. |
| credential-rotated | Same, CredentialService.Generate rotation branch | Replacement and the event are atomic. Record the new credential record ID and owning agent. Do not emit separate generated/revoked facts for internal rotation steps. |
| credential-revoked | Same, CredentialService.Revoke. Agent deletion in `internal/domain/agents/service.go` | Only actual credential deletion produces the event. Read the removed record ID under the transaction. An absent result is not a success event. |
| signing-key-promoted | `internal/domain/oauth2server/signing_key_service.go`, PromoteKey (208-210), generateAndStore (79-139), EnsureInitialKey (228-271) | Record actual committed current-key selection, including bootstrap/makeCurrent. Selection of the same current key is a no-op. Retain activates_at separately from selection time. |

## Required integration changes

### Preserve domain ownership

The ledger service exposes typed outcome recipes.
It does not infer facts from arbitrary HTTP status codes or storage inserts.
Handlers never bypass the domain service to access the ledger repository.

Before authentication, the token adapter returns the existing 4xx error for method, media type, and empty or malformed form errors.
It also returns that error for missing required parameters.
Malformed impersonation audiences also remain structural errors outside the durable outcome path.
These errors do not enter the durable outcome path.

The domain service classifies authentication and authorization decisions and authenticated terminal proxy failures.
If recording fails, these decisions and failures fail closed.

`CredentialService` owns credential generation, rotation, revocation, and their
transactions in the existing OAuth2-server bounded context.
`internal/adapters/http/handlers/admin/client_credentials_handler.go` is the
inbound adapter and delegates to that service.

Existing credential logs remain in the same response flow.
They retain exactly their current names, levels, messages, fields, and success status codes.

For proxy mode, separate upstream transport from the domain completion decision through a port.
Stage the upstream response until verification and ledger outcome recording complete.
Then forward its existing permitted headers/body/status unchanged.
Do not log or retain the staged credential payload in the ledger.

The existing unverified streaming path cannot release bytes before recording.

Do not invent a subject from upstream JWT claims without verification.
Hybrid dispatch must use the same local/proxy completion seams.
Do not add another recorder.

### Transaction and outcome details

**Local issuance**

The owner transaction covers response-population writes and success append.
Fosite's nested transaction joins the owner instead of committing independently.

Preserve Fosite's failure-side revocation semantics.
Return the token only after outer commit.
Prepare signing material before this transaction.
Revalidate the selected key inside the transaction without external cryptographic I/O.

**Session refresh**

Upstream work stays outside the local write transaction.

Persist the accepted refresh and its own event before using the result in an exchange.

`refreshLockedSession` coordinates the attempt and calls `refreshSessionTokens`
for upstream exchange and encryption.
`persistRefreshedSession` conditionally writes accepted state through
`UpdateRefreshedSession` and appends `session-refreshed` in one short transaction.
It owns the commit. `UpdateRefreshedSession` alone does not commit the event.
The `WithLockedSession` callback returns only an error. Coordination adapters never select or commit persistence on its behalf.

A known terminal failure produces one `session-refresh-failed` fact after
successful rollback of any local mutation.
A commit error or failed rollback leaves the outcome indeterminate.
No contradictory failure fact is recorded.

If subsequent grant/scope verification rejects exchange, the refresh event remains.
Record the separate exchange failure.

**Impersonation**

If minting fails, a grant decision can remain.
Successful minting is an exchange occurrence.
An internal evaluation error produces token request failure, not an invented permission decision.
Diagnostic outcomes, not OAuth error codes, distinguish infrastructure failures from authorization denials.
Recording failures preserve original cancellation causes for the existing diagnostic classification.

Separate the permission decision from minting.
Record only one final denied decision after rule evaluation, not one per evaluated rule.

**Request retries**

Genuinely new issuance/exchange is a new occurrence.
Reading unchanged business state again is not a new occurrence.
There is no new client request-idempotency key.

**Cascades**

Before agent-deletion cascades, enumerate affected grants, approvals, and
credentials under the owning deletion transaction.
Use existing terminal event types for actual catalogued terminal state changes,
not new cascade event types.
If the locked reload discovers another subject, roll back and rediscover the complete ordered subject set.
Restart only the local owning deletion transaction after successful rollback.
Cancellation, rollback failure, and ambient joined ownership stop the restart.

Removing an already denied approval solely as dependent data is not a new user denial.
No destructive path can delete an event FK because none exists.

**Provider session boundary**

Migration 004 keeps `fk_user_sessions_service` as `ON DELETE RESTRICT`.
No later migration changes it.
Retained sessions cause provider deletion rejection, not a session cascade.
This rejection emits no `session-terminated` fact.
The explicit session lifecycle service owns session termination.

Preserve the existing restriction rather than introducing new destructive HTTP behavior.

**Expiry**

Domain reads that currently filter expired state must recognize candidates internally.
They must atomically mark candidates before returning the existing filtered result.
Erasure/retention must not erase these business-row markers.
Approval decision predicates use the current wall clock, not the outer transaction start time.
A validity change reopens only a genuinely different effective expiry.

**Expired pending deduplication**

Repository creation returns an unrecognized expired duplicate without consuming it.
The domain appends its marker/expiry fact.
Then it calls creation again in the same owner to retire the duplicate and create the replacement.
This handles TTL crossings after preflight discovery without a second rate-limit attempt or an added scheduler.

**Signing selection**

The catalogue records selection of the current key.
It does not prove a signature or completion of the JWKS waiting period.
The `activates_at` field explicitly retains the eligibility time.
Bootstrap losers and repeated current-key selection do not generate new promotion facts.
Generation, branch-key provisioning, and encryption precede bootstrap coordination and ledger ownership.
The persistence callback rechecks selection and commits the prepared key with its promotion fact and cache effects.

### Pinned Fosite failure-side revocation boundaries

T002 inspected Fosite v0.49.0 and the broker's `FositeStorage` implementation.
The shared transaction rename does not move these boundaries:

| Path | Fosite v0.49.0 behavior | Required preservation |
|---|---|---|
| Authorization-code replay | `handler/oauth2/flow_authorize_code_token.go:32-52` receives the original requester with `ErrInvalidatedAuthorizeCode`. It calls `RevokeAccessToken` and `RevokeRefreshToken`, then returns `invalid_grant`. This request-validation path does not begin a transaction. | Keep request-chain revocation outside the later issuance-success transaction. Failure-event recording must not undo the revocation. |
| Refresh-token reuse | `handler/oauth2/flow_refresh.go:47-56` calls `handleRefreshTokenReuse`. Lines 178-203 begin a transaction, invalidate the presented token, revoke its request chain, and commit before the caller returns `invalid_grant`. | Let this scope commit independently of the denied refresh request. Do not join it to an outer scope that rolls back the request. |
| Authorization-code success | `flow_authorize_code_token.go:155-187` begins a transaction for code invalidation and token-session writes, populates the response, and commits. Its deferred handler rolls back errors. | The later ledger owner must enclose response population, not the earlier replay-detection path. Inner scopes must join that owner. |
| Refresh success | `flow_refresh.go:135-163` begins a transaction for rotation and replacement-token writes. Its error helper rolls back storage errors. | The later ledger owner must enclose these success writes without absorbing the independent reuse-revocation transaction. |
| PKCE challenge consumption | `handler/pkce/handler.go:135-151` reads and deletes the challenge before verifier validation. `PopulateTokenEndpointResponse` is a no-op. | After replay detection, enclose PKCE validation in the authorization-code owner. On validation rejection, commit without issuance to preserve the one-shot deletion. On later response-population or recording failure, roll back deletion together with the code/token changes. Record the terminal failure independently. |

The broker maps refresh revocation to `RefreshTokenSessionRepository.RevokeByRequestID`.
`DeleteRefreshTokenSession` marks the presented token used and treats an absent or already-used token as success.
`RevokeAccessToken` remains a no-op because access tokens are stateless JWTs.
This refactor does not add access-token revocation guarantees.

Before issuance-success ownership, the provider must complete authorization-code/refresh replay-detecting `HandleTokenEndpointRequest` validation.
PKCE validation consumes state and belongs inside the authorization-code owner described in the pinned behavior table.
`fosite.NewAccessRequest` only constructs the request.
Independent failure recording starts only after an unsuccessful success scope ends.

The PostgreSQL context and executor retain their existing behavior in Phase 0.
Memory also retains its existing no-op manager until the foundation supplies real rollback and isolation.

### Trust mapping

Ordinary user mutations take the subject from the owned business object.
They take the actor from the established principal context.
Administrative object actions generally have null subject.

RFC 8693 exchange and impersonation take the affected subject from the verified domain result.
They set `actor.kind=gateway`.
They take `actor.id` from the independently verified initiating client assertion (null before verification).
They keep the receiving agent in `agent_id`.

Only after accepted delegation, set `actor.on_behalf_of` to the represented principal.
An automatic session refresh inherits that exchange actor.

Gateway approval creation supplies the verified gateway identity and agent association.
Approval consumption authenticates only the represented principal.
Its gateway actor ID remains null instead of borrowing the creator's `gateway_client_id`.

On delegated exchange, `SecurityContext.Actor` currently means the affected principal.
Do not copy it blindly to the ledger's initiating caller.
Request context must not overwrite a caller that the domain already established.
Ledger recording preserves existing slog semantics on the current main baseline.
Token exchange, impersonation, and session refresh retain the credential-safe diagnostics and audit fields from main's telemetry classification contract.

Impersonation AuditRecord contains partially evaluated rule fields on failures.
Construct ledger identity from validation results, not wholesale audit serialization.

ADR 031's authorized unverified-subject path requires policy/delegation establishment before subject attribution.
This feature does not add or remove that exception.
