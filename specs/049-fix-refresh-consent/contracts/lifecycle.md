# Refresh and Lifecycle Contract

These are changes to existing operations, not new endpoints. Canonical contracts remain `api/enduser/openapi.yaml` and `api/admin/openapi.yaml`.

This is a feature 049 design document, not a claim of implemented behavior. Current canonical APIs remain unchanged until implementation. Approved contract updates belong to T005 before runtime edits.

## Refresh Grant Design

Refresh continues to use the existing `POST /oauth2/token` operation. No separate endpoint or operation ID is added.

- The form uses `grant_type=refresh_token`, the original `client_id`, and a `refresh_token` from that session.
- Confidential clients supply their current `client_secret`. A previously public client requires authentication once its registration has credentials.
- Optional `scope` is a space-delimited list. Fresh narrowing affects only the new access token, not the session ceiling.
- Success retains `access_token`, `refresh_token`, `token_type=Bearer`, `expires_in`, and the existing optional `scope` member.
- An eligible retry returns the original token strings and response scope or omission. Its `expires_in` reports remaining access-token validity.
- The default reuse interval is 30 seconds from first consumption, with at most three committed stored-result returns per predecessor.
- Only the immediate predecessor qualifies while its successor remains current and unused and its original access token remains unexpired.
- Retry requires the original normalized requested scope, active consent, current client authentication, permitted response scope, and valid session lifetimes.
- Zero reuse disables recovery. Retry advances no session or token deadline.

The failure rules and lifecycle effects that follow are part of the same planned contract.

## Lifecycle Effects

| Existing operation | Authorization effect added | Existing result retained |
|--------------------|----------------------------|--------------------------|
| `DELETE /api/consent/agents/{agent-id}/grants` | Revoke every local refresh session for the authenticated principal and agent in the same transaction as grant deletion | 204 success. 404 if that principal's grant is absent. Existing 400/401/500 behavior remains. |
| `POST /api/consent/agents/{agent-id}/grants` with empty `granted_permission_sets` | Revoke every matching local refresh session, even if the grant is already absent | Idempotent 204. Existing request authentication and validation remain. |
| `POST /api/consent/agents/{agent-id}/grants` with nonempty `granted_permission_sets` | Renewing a grant after expiry revokes its earlier sessions. Updating an active grant, including its scopes or future validity, keeps existing sessions under the current grant. | Existing 201 grant response, including redirect behavior. |
| `DELETE /api/agents/{agent-id}` on the admin server | End all local refresh sessions across all principals with the agent deletion | 204 success. Existing 400/404/500 behavior remains. |
| `DELETE /api/agents/{agent-id}/client-credentials` on the admin server | Revoke all local refresh sessions for the agent with credential deletion | 204 success. 404 when credentials are absent. Existing error schema remains. |
| `POST /api/agents/{agent-id}/client-credentials` replacing existing credentials | Keep existing sessions, but require the replacement credential on every later refresh. The previous secret cannot authenticate. | 200 and existing replacement metadata. No secret response until commit. |
| First credential creation through that POST | Keep sessions created while the agent was public; subsequent refresh requires the new credential. | Existing 201 response. |

## Common guarantees

- Lifecycle success means affected refresh tokens, retry results, and concurrent descendants are unusable across broker instances. Clear live retry ciphertext on revocation. Encrypted backup copies cannot authorize a later retry.
- A lifecycle write and related revocation commit together. Confirmed rollback preserves both. An indeterminate commit reports failure without a rollback claim or replacement secret. It never exposes a partial transaction as successful completion.
- Grant/agent revocation covers root records and relevant legacy current-token rows during a rolling upgrade, so an old writer cannot mark them used after committed revocation.
- Startup or maintenance commits lifetime expiry (`ExpiredAt` and its reason) and live-ciphertext erasure in one scoped transaction. That transaction also invalidates every matching unconsumed legacy current or descendant mirror. A failed mirror update rolls back the transition. Failed startup reconciliation blocks readiness.
- Before a binary-only rollback admits old binaries, quiesce token traffic and old writers and complete outgoing-policy expiry reconciliation. Binary-only rollback cannot restore terminal sessions. Down migration and reapplication also keep terminal legacy mirrors unusable.
- Revocation stays scoped to the authenticated principal and agent, or to all principals of the deleted/revoked agent. Other users and agents remain unaffected.
- Regrant, public-client fallback, and credentials created after explicit revocation cannot restore revoked sessions. Routine credential replacement does not revoke sessions, but the old credential cannot refresh them.
- An expired grant renewed after its deadline revokes all earlier sessions for that principal and agent. An active grant updated before its deadline retains its sessions and original clocks.
- Authorization-code replay revokes the refresh session created by that code. Connected third-party OAuth2 sessions remain active after local grant deletion.
- Before authorization-code replay, a still-eligible predecessor retry returns the original access-token and successor-refresh-token strings with remaining `expires_in`. Replaying that code then revokes both its current refresh token and its retry-eligible predecessor. An unrelated session stays usable. The positive retry must succeed before replay to establish that revocation, not a pre-existing retry denial, blocks recovery.
- Existing JWT access tokens retain their original expiry at signature-only resource servers. A lifecycle response does not claim immediate access-token invalidation.
- Existing middleware supplies principal/authentication and admin-server isolation. Handlers do not call storage ports directly.
- Lifecycle writes, refresh decisions, and terminal-expiry maintenance for an agent share one guard. It supplies a shared decision time after acquisition. All grant, lifetime, and retry deadlines use that time. Transactional reads use the ambient executor.
- Existing impersonation, token-exchange, and consent callers of `UserDelegationVerifier` must pass an explicit decision time rather than consult node-local time.

## Refresh failure precedence

Preserve existing malformed-request and client-authentication failures. Under the agent guard, use this precedence. The first failed check determines the response and audit reason:

1. Authenticate the client and bind the session to that client.
2. Check session revocation.
3. Require active consent.
4. Check stored and current-policy session lifetimes.
5. Require current agent refresh-grant capability.
6. Classify the token and check retry eligibility.
7. Check response-scope eligibility only for a fresh or otherwise eligible stored-result candidate.

Consent or lifetime denial takes precedence over missing capability. Missing capability takes precedence over prohibited reuse. A deadline expires at equality. Repeat all success checks at a fresh shared time before commit.

After valid client binding, missing or expired consent and revoked or expired sessions return token-free HTTP 400 `invalid_grant`. Unsupported legacy lineage and prohibited reuse return the same error. Missing consent has no `error_uri`. Missing current refresh-grant capability retains HTTP 400 `unauthorized_client` without tokens or mutation.

On a candidate success, a disallowed response scope retains HTTP 400 `invalid_scope` or HTTP 403 `scope_not_granted`, as appropriate. These errors return no tokens and do not mutate request state. A fresh narrower request limits only its new access token and does not reduce the session ceiling. Retry requires the original normalized requested scope and permission to return the exact original response scope, including any omission. It cannot narrow the original result.

Unavailable grant or session state, failed decryption, or failed commit returns HTTP 500 `server_error` without tokens.

Only prohibited reuse by the bound client revokes the session. Older predecessors remain replay evidence after individual expiry. Prohibited reuse includes a closed reuse window, changed retry request scope, or fourth otherwise eligible retry authorization. These cases revoke even if the old response scope is no longer permitted. Do not decrypt a prohibited retry just to inspect response scope or add another stored scope field. All other authorization denials leave request state unchanged. These include unknown tokens, failed authentication, client mismatch, consent or lifetime denial, missing capability, disallowed response scope, and expired cached access. Confirmed pre-commit failure or rollback preserves the presented token, rotation state, count, and deadlines. Startup and maintenance handle terminal expiry and retry-result clearing separately.

For an anchored current token, lock and re-read its matching legacy current row before trusting the new table's unused state. Hold that lock through fresh rotation. If an old writer consumed the mirror without matching new-token or history evidence, its original token and unsupported descendant return token-free `invalid_grant`. This denial creates no successor and changes no request state. An old-first writer is detected on re-read. A new-first writer commits its mirror consumption before the old writer can use that row. Revocation rechecks old-writer descendants.

An indeterminate commit returns token-free HTTP 500 `server_error` without a rollback claim. A later request resolves durable state under the agent guard and applies normal authorization and classification. Confirmed rollback permits ordinary rotation. Committed consumption permits only its existing eligible retry result. At zero reuse, committed consumption is prohibited reuse and requires fresh authorization. Unresolved state returns `server_error`. Recovery never creates another successor, extends a retry window, or refunds a committed retry count.

Unknown or deleted client resolution retains its existing authentication failure. It never permits fallback to revive a revoked root. Explicit credential deletion rejects old sessions even if the remaining agent resolves as public. A previously public session requires client authentication after its agent receives credentials.

## Contract review and release gate

The user's remediation request confirms the intended consent, lifecycle, recovery, and lifetime changes. Final OpenAPI details require review before implementation.

Record this security behavior change and the new default recovery policy in the changelog. Existing strict-reuse consumers must use the explicit zero interval if required. Handle any released-contract break through the constitution's major-release or approved versioned-contract process. Do not introduce a silent compatibility bypass.

Accepted ADR 038 records the encryption-subject and transaction-ownership decisions. Its acceptance does not imply API/release approval or runtime implementation.
