# ADR 048 revision draft: Permit-bound exchanged tokens (F1)

**Status**: Proposed (approval item ADR-2 in [plan.md](./plan.md#approval-items))
**Revises**: [ADR 048](../../adrs/048-authzen-delegation-pip.md), same PR before merge
(Principle II). ADR 048 is not on the default branch.
**Extends**: ADR 012 (enabled-mode reuse of cached exchanged tokens)
**Date**: 2026-11-06

`mjungsbluth` directed in the planning command for F1 that the permit is authoritative for
upstream scopes. This draft records that direction. It records no approval. After the
maintainer approves ADR-2, implementation applies the changes below to
`adrs/048-authzen-delegation-pip.md`, records the approval in its Status and Revision history,
and removes this draft.

## Changes to apply to ADR 048

1. Header: change `ADR 012 (separate cache)` in **Extends** to
   `ADR 012 (separate cache; permit-bound token reuse in §10)`.
2. Decision: add §10 after §9, with the text in the next section.
3. Alternatives considered: add the two entries in the Alternatives section below.
4. Consequences: in the re-consent bullet, replace "The exchange cache is unchanged by this
   ADR" with "This ADR does not change that cooldown", and add the Consequences entry below.
5. Revision history: add an entry for this revision with the supplied approval.

## Decision text

### New §10. The permit is authoritative for upstream scopes

In enabled mode, an exchanged token from the ADR 012 cache is reused only under the grant
version for which it was exchanged. The fresh permit decides that version.

- Each cached token entry records a **token binding**: the set of the permit's
  `resource.scopes` and the instant of its `grant.updated_at`. Scope order and duplicates do
  not matter. A token exchanged by a request is bound to that request's permit. A token that
  no permit has bound yet is bound by the first permit that uses it.
- Before ExtProc forwards a cached token, it compares the binding with the permit that the
  current request uses. If the scopes or `grant.updated_at` differ, ExtProc must not forward
  the cached token. It removes that cache entry, but only if the entry still holds the same
  token, and exchanges again. Then it binds the new token to the current permit and forwards
  it. Each request exchanges again at most once.
- On the body path, the comparison runs after both concurrent calls complete and after the
  existing precedence (delegation denial, resolution error, exchange error). On the header-only
  path, the exchange that follows a policy allow uses the binding of the permit that policy
  evaluated.
- A failed re-exchange uses the existing exchange-error handling. The removed entry is never
  used as a stale-on-error fallback, so no token minted under another grant version is
  forwarded.
- Disabled mode, the cache key, the cache TTL, `cache.max_ttl`, and the 5 s re-consent cooldown
  are unchanged. A cooldown entry holds no token, so it has no binding.
- The broker contract does not change. The binding uses only permit members.

**Limitation**: The broker exchanges the stored provider session, whose scopes the provider
granted at session authorization. A narrowed grant alone does not narrow that provider token.
Exchanging again guarantees that the broker re-validates the forwarded token against the
current grant and session. Upstream receives narrowed provider scopes only when the provider
session carries them, for example after the user re-authorizes the service. Broker-side
narrowing of provider tokens is out of scope.

**Residual race**: On a body-path miss of both caches, exchange and evaluation read the grant
concurrently. A grant change between those two reads can bind a token to the newer version.
The existing ADR 012 TTL bounds that window, which is no wider than today's behavior.

## Alternatives

- **Forward cached tokens until ADR 012 expiry after a grant change (§10)**: rejected by the
  maintainer's F1 direction; a token exchanged under a broader grant could reach upstream for up
  to `cache.max_ttl` after narrowing.
- **A grant version in the exchange response to bind tokens (§10)**: rejected for this
  increment; it closes the residual race but changes the `/oauth2/token` contract.

## Consequences

- After a grant changes, the first enabled-mode request with a fresh permit exchanges again
  instead of forwarding a token bound to the earlier grant version. This adds at most one broker
  exchange per grant change and cache key.
- The D-5 re-consent cooldown is unchanged. ADR-2 revises only D-5's broader statement that the
  exchange cache is unchanged.
