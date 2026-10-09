# 049: Continuing consent for local refresh

## Scope

Local refresh authority ends with user consent. Hybrid mode applies this rule only to locally issued tokens. Proxy and hybrid upstream flows remain unchanged.

## Requirements

- Add nullable `grant_id UUID` to `refresh_token_sessions` in migration 036. Use `lock_timeout`, no foreign key, no backfill, and reversible rollback.
- At authorization-code exchange, bind each new refresh token to the active grant for its principal and agent. Missing or expired consent returns `invalid_grant`.
- Before rotation, call `consent.Service.VerifyAgentAccess`. Reject missing or expired grants and different grant IDs.
- For a null grant ID, accept only when the active grant's `created_at` is no later than the refresh row's `created_at`. Compare UTC instants in Go. Bind the successor to this grant ID.
- Consent denial returns the normal `invalid_grant` body, creates no tokens, and revokes the chain with `RevokeByRequestID`.
- Verifier or storage errors return `server_error`, without rotating or revoking the token.
- Grant deletion revokes unused refresh rows for that principal and agent. Renewal of expired consent does the same. Credential deletion revokes unused rows for the entire agent.
- Agent deletion retains its existing database cascade. Rotation remains strict single use.
- PostgreSQL lock order is agent first, refresh rows second. The first rotation-transaction statement locks the agent `FOR KEY SHARE`. Revocation locks it `FOR UPDATE` before one matching-row update.
- Grant and credential deletes on main do not honor transaction context. Delete first, then revoke, and propagate revocation errors. Deletion cannot roll back after a revocation error.
- Log each consent denial with `grant_missing`, `grant_expired`, or `grant_mismatch`. Log each lifecycle revocation with its trigger and affected-row count. Never log tokens or signatures.

## Acceptance scenarios

Each scenario maps to one production-builder E2E test.

- **S1 — Grant deletion:** Authorize and refresh successfully. Delete the grant. The next refresh returns `invalid_grant` without tokens.
- **S2 — Delete and re-grant:** Authorize and refresh successfully. Delete consent and create a new grant. The old chain cannot refresh.
- **S3 — Expiration:** Authorize and refresh successfully. Expire consent. The next refresh returns `invalid_grant` without tokens.
- **S4 — Credential deletion:** Authorize and refresh successfully. Delete credentials. A public-client refresh attempt cannot continue the old chain.
- **S5 — Agent deletion:** Authorize and refresh successfully. Delete the agent. The old refresh token cannot yield tokens.
- **S6 — Public client:** Authorize a public client with PKCE and refresh successfully. Delete consent. The next refresh returns `invalid_grant` without tokens.

Storage and provider tests cover legacy timestamps, grant identity, infrastructure errors, targeted revocation, both race orders, agent-deletion races, Berlin database time, and populated migration rollback.

## Accepted limits

1. A row without `grant_id` gets one rotation on the strength of an older active grant. Its successor stores the grant ID.
2. Old pods do not check consent during a rolling deployment. Old binaries also do not lock the agent first. Revocation can still deadlock while those pods serve traffic.
3. Existing access tokens remain valid until `token_ttl` expires. This change does not cap their expiry at the grant deadline.

## Exclusions

No retry window, session lifetime policy, encryption changes, new session tables, maintenance workers, configuration keys, readiness changes, coordinator, API-contract changes, or new ADR.
