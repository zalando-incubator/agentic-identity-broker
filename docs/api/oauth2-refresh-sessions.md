# OAuth2 local refresh sessions

> **Implemented feature 049 contract for broker-issued local refresh sessions.**
> The canonical API contracts are [end-user OpenAPI](../../api/enduser/openapi.yaml)
> and [admin OpenAPI](../../api/admin/openapi.yaml).

This guide covers broker-issued user refresh sessions from authorization-code exchanges.
It applies to `local` mode and the local minting path of `hybrid` mode. The proxy path,
upstream refresh, vaulted third-party tokens, and client-credentials grants retain their
existing behavior. Refresh uses the existing `POST /oauth2/token` operation on the
end-user server. The admin lifecycle actions use the existing admin server.
The examples use placeholder credentials and token strings, not usable secrets.
Each section gives an independent example. Repeated placeholders do not link
sessions across sections.

## Rotate within the original authorization

The request is form-encoded. Use the original session's `client_id` and the current
`refresh_token`. A confidential client supplies its **current** `client_secret`.
A client that was public must authenticate if its agent later receives credentials.
The optional `scope` requests a subset of the original session scope ceiling.
If `scope` is omitted, the fresh result uses that ceiling. The request cannot
change the session's principal, agent, client, grant origin, start time, or ceiling.

In this example, the original ceiling is `read write offline_access`. The caller
requests a narrower access token with `read offline_access`:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_0&scope=read%20offline_access
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"ACCESS_1","refresh_token":"REFRESH_1","token_type":"Bearer","expires_in":3600,"scope":"read offline_access"}
```

The scope reduction applies **only to `ACCESS_1`**. A later fresh rotation can
request `write` within the unchanged ceiling if the current grant and agent still
permit it. A fresh rotation replaces the current refresh token and advances the
inactivity deadline. It does not reset the session's original start or absolute
deadline. The optional response `scope` member follows the existing token contract.
No local refresh response requires RFC 8693 `issued_token_type`.

The omitted reuse interval is `30s` when `token_ttl` is longer than 30 seconds.
For `token_ttl <= 30s`, omission selects `0s` (strict single use).
An explicit positive interval must be shorter than both token lifetimes.
The interval starts at the first successful consumption of `REFRESH_0`.
The default absolute lifetime is `0s` (no total-duration deadline).
The default inactivity lifetime is `720h` (30 days). The existing
`oauth2_authorization_server.local.refresh_token_ttl` setting controls that
inactivity lifetime. Only a fresh successful rotation renews inactivity.
Resource calls, failed requests, and permitted duplicate retries do not.
A session or retry deadline denies a request at equality. A longer policy or a
renewed grant cannot restore an expired or revoked session.

Signing and retry encryption/decryption run outside the agent lock. A guarded
shared-clock sample fixes the consumption and reuse deadline. Before commit,
another guarded sample and authoritative reread recheck authorization and deadlines.
Preparation time can shorten recovery but cannot extend it.

## Recover a lost response within the bounded interval

If the client loses the successful response for `REFRESH_0`, it can send **the same
normalized requested scope** and predecessor token. The successor must still be
current and unused, and the original access token must remain unexpired.
An omitted or empty scope request differs from an explicit scope set. Nonempty
scope sets compare after splitting, deduplication, and sorting.
The broker checks current client authentication, original active consent, session
lifetimes, agent refresh capability, and the exact original response scope again.
A permitted retry returns the same token strings and the same `scope` value or
omission as the first response:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_0&scope=read%20offline_access
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"ACCESS_1","refresh_token":"REFRESH_1","token_type":"Bearer","expires_in":3590,"scope":"read offline_access"}
```

Only `expires_in` decreases to the remaining validity of **the same** `ACCESS_1`.
The retry never mints another pair or advances the fixed reuse, inactivity, or
absolute deadline. Only the immediate predecessor of an unused current successor
qualifies. At most **three committed stored-result returns** are allowed for each
consumed token. A lost response or commit acknowledgement still uses a count.
A changed retry request scope, an older predecessor, a closed window, or a fourth
otherwise eligible request is prohibited reuse by the bound client. It returns
`400 invalid_grant` and revokes the remaining session tokens. Explicit `0s` reuse
disables all predecessor recovery: a second presentation is prohibited reuse.

Expiry of the original access token alone denies its cached retry result without
revoking a still-valid current refresh token. But active consent, both session
lifetimes, and lifecycle revocation **always** take precedence over retry grace.
A retry cannot narrow the previously issued access token after permissions change.

## Denial after consent ends

The user can revoke access with `DELETE /api/consent/agents/{agent-id}/grants`.
It revokes every local refresh session for that principal and agent, including
retry-eligible predecessors. DELETE retains its `404` result when the grant is absent.
A POST with empty `granted_permission_sets` returns `400` without changing the
grant or refresh sessions. Revocation does not terminate connected third-party sessions.

A refresh with either `REFRESH_0` or `REFRESH_1` after consent revocation returns
the same generic denial. An expired grant, expired session, or prohibited reuse
also uses this token-free shape. The response does not reveal the reason or include
an `error_uri`:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_0&scope=read%20offline_access
```

```http
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error":"invalid_grant"}
```

Grant state that cannot be determined gives token-free `500 server_error`, not a
missing-grant response. After client binding, the broker checks session revocation,
consent, lifetimes, agent refresh capability, token classification, and candidate
response scope, in that order. Missing capability keeps `400 unauthorized_client`.
For an otherwise eligible result, a disallowed scope keeps `400 invalid_scope` or
`403 scope_not_granted`. The request does not consume a token. Except for bound-client
prohibited reuse, other refresh authorization denials do not revoke the session.
The `access_denied` response with a consent `error_uri` for **user impersonation**
is a separate flow, not the refresh error contract.

## Recover after a failed commit

An unavailable grant/session store, failed decryption, or failed commit returns a
5xx `server_error` without tokens. The response body does **not** reveal whether
an indeterminate commit took effect. These examples show distinct durable outcomes,
not two distinguishable HTTP error shapes.

### Confirmed pre-commit failure or rollback

Suppose the broker confirms that the transaction failed before commit or rolled
back while `REFRESH_1` was presented. The failed request neither consumes the
token nor advances its deadlines:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_1&scope=read%20offline_access
```

```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error":"server_error"}
```

After the underlying failure clears, the same client can retry `REFRESH_1` while
normal consent and deadline checks still pass. It performs an ordinary **fresh**
rotation, not a return of a result from the rolled-back transaction:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_1&scope=read%20offline_access
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"ACCESS_2","refresh_token":"REFRESH_2","token_type":"Bearer","expires_in":3600,"scope":"read offline_access"}
```

### Indeterminate commit acknowledgement

A lost commit acknowledgement yields the **same** token-free `500 server_error`
without a rollback claim. A later request resolves durable state under the normal
client, consent, lifetime, capability, classification, and scope rules.

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_1&scope=read%20offline_access
```

```http
HTTP/1.1 500 Internal Server Error
Content-Type: application/json

{"error":"server_error"}
```

If the rotation committed and `REFRESH_1` remains an eligible immediate predecessor,
a retry within the unchanged reuse interval returns **only the committed pair**:

```http
POST /oauth2/token HTTP/1.1
Content-Type: application/x-www-form-urlencoded

grant_type=refresh_token&client_id=550e8400-e29b-41d4-a716-446655440000&client_secret=EXAMPLE_SECRET&refresh_token=REFRESH_1&scope=read%20offline_access
```

```http
HTTP/1.1 200 OK
Content-Type: application/json

{"access_token":"ACCESS_2","refresh_token":"REFRESH_2","token_type":"Bearer","expires_in":3580,"scope":"read offline_access"}
```

If the transaction rolled back, the token is still current and can rotate
normally. If the state remains unresolved, the broker returns token-free
`500 server_error` again. It does not create a second successor, extend a deadline,
or refund a committed retry count. With explicit `0s` reuse, a committed
consumption cannot be recovered: predecessor use is prohibited reuse and fresh
authorization is required. A 500 response alone gives no evidence of rollback.

## Lifecycle and reauthorization

| Existing action | Effect on broker-issued local user refresh sessions |
|---|---|
| `DELETE /api/consent/agents/{agent-id}/grants` (end-user) | End all sessions for the authenticated principal and agent. Keep its existing 204 or absent-grant 404. |
| `POST /api/consent/agents/{agent-id}/grants` with empty `granted_permission_sets` | Reject with 400; preserve grant and sessions. Use DELETE to revoke. |
| Nonempty POST for an expired grant | Revoke sessions from before the renewal. An active-grant edit before expiry retains its sessions and original clocks. |
| `DELETE /api/agents/{agent-id}` (admin) | End sessions for that agent across all principals with agent deletion. Keep existing 204 or missing-agent 404. |
| `DELETE /api/agents/{agent-id}/client-credentials` (admin) | Explicitly revoke all its local refresh sessions. Keep existing 204 or missing-credential 404. Public-client fallback cannot revive them. |
| `POST /api/agents/{agent-id}/client-credentials` (admin) | First creation or replacement retains sessions. Later refresh requires the new secret, including an eligible retry. The replaced secret is invalid. |

A successful lifecycle action ends affected current tokens, retry results, and
racing replacements across broker instances as one committed outcome. A confirmed
rollback preserves the prior lifecycle and session state together. An indeterminate
commit reports failure without claiming rollback. A successful action does not
immediately invalidate a broker access JWT at a resource server that only checks
its signature and claims. That JWT remains valid until its issued expiry.

Authorization-code replay is separate from refresh-token presentation. Replaying
the code revokes the session created by that code, even when another client
successfully authenticates as **itself** to replay the code. It also ends an
otherwise eligible predecessor retry, without affecting unrelated sessions.
Failed refresh authentication or a client mismatch cannot revoke a session.

A new grant, recreated agent, replacement credential, or longer lifetime never
restores a revoked or expired session. Clients must repeat the authorization-code
flow to obtain a **new** refresh session. Every pre-feature unanchored local session
and an unsupported old-writer descendant also requires fresh authorization.
Evidenced anchored families issued by new brokers can continue under current
authorization. Before a full database or refresh-state restore, stop token traffic
and all writers. Admit traffic only after acknowledged success from
`agentic-identity-broker --config <file> refresh-sessions invalidate-restored --database-id <backup-database-uuid>`.
Record `SELECT database_id FROM refresh_maintenance WHERE singleton` with the backup.
The command requires PostgreSQL and compares that independently recorded identity
before making changes. A mismatched target or memory backend exits nonzero.
Encrypted backup copies cannot reauthorize an old session.

For rolling upgrades and binary-only rollback, follow the [deployment procedure](../deployment/kubernetes.md#upgrade-and-binary-only-rollback). Retain the additive refresh schema and legacy table. Stop token traffic and old writers before reconciling expiry under the outgoing policy. If reconciliation fails, keep the old binary offline. Rollback cannot restore revoked or expired sessions.

The authorization-server metadata remains unchanged: there is no advertised
revocation or introspection endpoint.

## Audit records

Committed revocation and lifetime expiry produce `RefreshSessionRevoked` and `RefreshSessionExpired` events through the existing structured-log pipeline.
Each event includes the principal, agent, bound client, non-credential session ID, reason, and request or maintenance origin.
Request context contains only the trusted client IP and truncated user agent. Receipts retain the same redacted context before cascade deletion.
Confirmed rollback produces no successful transition event. A known-token client mismatch preserves the owner identity for rejection auditing without changing its session.
Unknown tokens do not produce invented owner identity. Neither audit events nor receipts contain tokens, credentials, or request targets.
