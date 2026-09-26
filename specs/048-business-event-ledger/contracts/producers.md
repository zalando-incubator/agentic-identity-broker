# Catalogue Producer and Transaction Map

All paths below are existing integration targets unless explicitly marked **new**. Every row is a required producer; this map is not permission to implement only representative event families. Event names use the namespace in `events.md`.

| Event name | Domain owner / existing seam | Recorded boundary and no-op rule |
|---|---|---|
| grant-created | `internal/domain/consent/service.go`, GrantConsent (291-397) | New grant and event commit together; concurrent upsert loser does not create a second occurrence |
| grant-updated | Same, existing-grant branch; grantMatchesRequest (548-573) | Locked comparison of effective permissions/validity; equal state returns without event |
| grant-revoked | Same, RevokeConsent and RevokeConsentForPrincipal (578-620) | Capture/deletion return of the actual removed grant; preserve distinct existing HTTP absence semantics |
| grant-expired | Same, GetActiveGrants / VerifyAgentAccess / delegation reads (625-683), plus `internal/domain/oauth2/service.go` grant checks | First successful effective-expiry marker update and event commit; repeated lazy recognition is silent |
| session-established | `internal/domain/oauth2session/service.go`, HandleCallback -> createSession (474-666) | A successful authorization callback establishes usable session state, including reauthorization upsert; not inferred from low-level Create alone |
| session-refreshed | Same, refreshSessionTokens -> UpdateSessionTokens (793-851,1180-1247), ForceRefreshSession | Accepted refreshed encrypted state plus event in its own transaction or explicit owner scope; remains a fact if a later exchange fails |
| session-refresh-failed | Same, refreshSessionTokens and direct RefreshAccessToken orchestration callers | One terminal refresh-attempt failure after any unsuccessful mutation is rolled back; no event per internal network retry |
| session-terminated | Same, TerminateSession (946-1023), and owning provider-deletion cascades | Only actual removed session; capture subject/session/service references before deletion |
| authorization-requested | `internal/domain/oauth2/service.go`, AuthorizationService.HandleAuthorization (137-401) | Accepted validated authorization request before consent/proceed response; one per accepted request, not per later authorization-code write |
| token-issued | `internal/domain/oauth2server/provider.go`, HandleClientCredentials, HandleAuthorizationCodeExchange, HandleRefreshToken (141-458); proxy grant completion via `internal/adapters/http/enduser/token_grant_strategy.go` | Non-exchange response ready; local response-population writes and event share outer transaction; proxy outcome commits before forwarding response bytes |
| token-request-failed | Provider failures and typed parse/proxy failures reported from `internal/adapters/http/enduser/oauth2_token.go` / token_grant_strategy.go to domain outcome service | Primary token failure when not a specific authentication/authorization exchange refusal; complete before HTTP failure response; null identities before authentication |
| token-exchanged | `internal/domain/tokenexchange/service.go`, Exchange (170-405); successful `internal/domain/impersonation/service.go`, Impersonate | Successful RFC 8693 result with receiving agent; commit event before response release; no token-issued duplicate |
| token-exchange-denied | Same exchange/impersonation orchestration after typed authentication or authorization rejection | Specific primary denial classification; no second token-request-failed for the same denial; preserve independently known identities only |
| impersonation-granted | `internal/domain/impersonation/service.go`, evaluateRule (250-285) | Authorization and active delegation have passed; record permission decision before minting; decision does not claim token issuance succeeded |
| impersonation-denied | Same, rule rejection/target rejection paths reported through Impersonate / HTTP parsing seam | One final refused permission decision, not one per failed candidate rule; do not mislabel mint/internal failure as refusal |
| approval-requested | `internal/domain/approval/service.go`, CreatePendingApproval (415-548) | `IsNew` winning creation only; existing pending dedup return does not produce event |
| approval-approved | Same, ApproveApproval (235-286) | Winning pending-to-approved CAS plus event and sync-state mutation |
| approval-denied | Same, DenyApproval (349-410) | Winning user-denial CAS; not also produced when revocation stores denied status |
| approval-consumed | Same, ConsumeApproval (645-721) | Winning once-persistence consumption; already-consumed 200 and concurrent loser are silent |
| approval-revoked | Same, RevokePermanentApproval (872-929), and applicable terminal cascades | Actual permanent-approval revocation; no no-op event on repeated transition |
| approval-expired | Same, GetApproval (188-230), mutation race handling, expired pending dedup retirement and filtered queries | Effective expiry recognized by existing lifecycle; marker/event atomic; do not add expired enum or scheduler |
| agent-registered | `internal/domain/agents/service.go`, Create (66-91) | Validated newly inserted agent; event contains agent ID, not display/config snapshots |
| agent-updated | Same, Update (97-135) | Locked effective-config comparison, update plus event; unchanged config produces no event |
| agent-deleted | Same, Delete (163-169) | Actual deletion plus child terminal facts in one transaction; ledger records never cascade away |
| credential-generated | `internal/adapters/http/handlers/admin/client_credentials_handler.go`, Generate (78-154); move policy into **new** `internal/domain/oauth2server/credential_service.go` | First credential record creation and event commit before plaintext secret response; generation library alone is not the fact |
| credential-rotated | Same current handler Generate rotation branch | Atomic replacement and event; record new credential record ID and owning agent; do not emit separate generated/revoked facts for internal rotation steps |
| credential-revoked | Same current handler Revoke (201-229), moved to credential domain service; agent deletion | Actual credential deletion; read removed record ID under transaction; absent result is not a success event |
| signing-key-promoted | `internal/domain/oauth2server/signing_key_service.go`, PromoteKey (208-210), generateAndStore (79-139), EnsureInitialKey (228-271) | Actual committed current-key selection including bootstrap/makeCurrent; same current selection is a no-op; retain activates_at separately from selection time |

## Required integration changes

### Preserve domain ownership

The ledger service exposes typed outcome recipes; it does not infer facts by observing arbitrary HTTP status codes or storage inserts. Parsing adapters report an enum such as malformed token request to the domain outcome service before writing the existing response. The domain service selects the event type, fixed reasons, trust level, and transaction behavior. No handler-to-ledger-repository bypass is introduced.

Extract the current credential generation/rotation/revocation orchestration into the existing OAuth2-server bounded context. This is necessary to remove the port bypass on the changed path, not an unrelated cleanup. Existing credential logs remain in the same response flow with exactly their current names, levels, messages, fields, and success status codes.

For proxy mode, separate upstream transport from the domain completion decision through a port. Stage the upstream response until verification and ledger outcome recording complete, then forward its existing permitted headers/body/status unchanged. Do not log or retain the staged credential payload in the ledger. The existing unverified streaming path cannot release bytes before recording. Do not invent a subject by decoding upstream JWT claims without verification. Hybrid dispatch must use the same local/proxy completion seams, not add another recorder.

### Transaction and outcome details

- Local issuance: preserve Fosite's failure-side revocation semantics; the owner transaction covers response-population writes and success append. Fosite's nested transaction joins rather than committing independently. Return the token only after outer commit.
- Session refresh: upstream work is outside the local write transaction. Persist the accepted refresh and its own event before using that result in an exchange. If subsequent grant/scope checks reject exchange, the refresh event remains and the separate exchange failure is recorded.
- Impersonation: separate the permission decision from minting. Grant decision may remain if minting fails; successful minting is an exchange occurrence. Only one final denied decision after rule evaluation, not one per tested rule. An internal evaluation error produces token request failure, not an invented permission decision.
- Request retries: genuinely new issuance/exchange is a new occurrence; re-reading unchanged business state is not. There is no new client request-idempotency key.
- Cascades: enumerate affected grants, sessions, approvals and credentials under the owning deletion transaction before SQL cascades. Translate actual catalogued terminal state changes into their existing terminal event types, not new cascade event types. An already denied approval removed solely as dependent data is not a new user denial. No destructive path may delete an event FK because none exists.
- Expiry: domain reads that currently filter away expired state must recognize candidates internally and atomically mark them before retaining the existing filtered result. Erasure/retention must not erase these business-row markers. A validity change reopens only a genuinely different effective expiry.
- Signing selection: the catalogue records selecting the current key, not proof of a signature or completion of the JWKS waiting period. `activates_at` explicitly retains the eligibility time. Bootstrap losers and repeated current-key selection do not generate new promotion facts.

### Trust mapping

Ordinary user mutations take subject from the owned business object and actor from the established principal context. Administrative object actions generally have null subject. Exchange takes affected subject from the verified domain result and initiating caller from the verified client assertion/calling peer; `actor.on_behalf_of` carries the established represented principal. Gateway approval authentication supplies the gateway identity and agent association; caller-supplied agent/principal strings are not independently authoritative.

`SecurityContext.Actor` currently means affected principal on delegated exchange; it must not be copied blindly to the ledger's initiating caller. Existing slog semantics are preserved. Impersonation AuditRecord contains partially evaluated rule fields on failures; construct ledger identity from validation results, not wholesale audit serialization. ADR 031's authorized unverified-subject path requires policy/delegation establishment before subject attribution; this feature does not add or remove that exception.
