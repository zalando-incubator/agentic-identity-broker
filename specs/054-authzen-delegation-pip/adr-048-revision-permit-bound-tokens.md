# ADR 048 revision draft: Permit-bound token reuse (F1)

**Status**: Proposed (approval item ADR-2 in [plan.md](./plan.md#approval-items))
**Revises**: [ADR 048](../../adrs/048-authzen-delegation-pip.md), same PR before merge
(Principle II). ADR 048 is not on the default branch.
**Extends**: ADR 012 (enabled-mode reuse of cached exchanged tokens)
**Date**: 2026-11-06

## Context

The maintainer's F1 direction asks that fresh permit scopes govern upstream token reuse.
A permit is not exchange provenance: the broker returns its stored provider-session token,
without an exchange grant version. This revision therefore guarantees binding-compatible
reuse, not provider-token narrowing or proof of the grant read by exchange. The narrowed
interpretation and terminal concurrency response are Proposed (ADR-2, D-8 and API-7).
No previous approval is extended to this draft.

## Changes to apply to ADR 048 after supplied approval

1. Change `ADR 012 (separate cache)` in **Extends** to
   `ADR 012 (separate cache; permit-bound token reuse in §10)`.
2. Add §10 below, including the concurrency and limitation text.
3. Add the Alternatives and Consequences entries below.
4. Replace "The exchange cache is unchanged by this ADR" in the re-consent consequence
   with "This ADR does not change that cooldown".
5. Record the supplied maintainer approval in Status and Revision history, then remove this
   incorporated draft. Preserve previous approval provenance.

## Decision text

### New §10. The permit governs enabled-mode token reuse

- A **Token Binding** is the sorted, de-duplicated set of permit `resource.scopes` and the
  instant parsed from `grant.updated_at`. Equality compares both sets and both instants.
  It records the permit that first claimed an exchange result, not the grant under which
  the broker actually minted or obtained the credential.
- Every successful exchange result carries an opaque **entry identity** and a shared,
  synchronized binding cell, also for non-cacheable successes. Cache hits and singleflight
  followers carry that same handle and the unchanged exchange identities/extensions.
  Every new cache installation has a fresh identity even when provider token bytes repeat.
  Credential bytes are never an identity, comparison key or exported diagnostic.
- Under synchronization, the first permit may claim an unbound result exactly once; an
  already bound result can only be compared, never rebound. Request ownership, leader
  status or "new exchange" status confers no right to overwrite a binding.
- Before forwarding, validate the delivered result handle against this request's permit.
  A matching binding is usable, including a detached result whose cache entry was replaced.
  An unbound result is usable only after an atomic successful claim. A mismatched result
  is never forwarded, regardless of which request performed the exchange.
- On mismatch, compare-and-delete only the observed entry identity under the cache lock.
  If a concurrent replacement exists, preserve it even if it holds the identical token
  string. Re-read that replacement once: compare/claim its handle as above. If compatible,
  use it without another broker call; otherwise perform at most one additional exchange
  through existing per-key singleflight and circuit-breaker handling. A failed delete is
  not an exchange error and does not authorize deletion of the replacement.
- After that one additional exchange, compare/claim the delivered handle again. If it is
  already bound to another live permit, return the terminal API-7 503
  `service_unavailable` / `delegation token binding unavailable`. Do not rebind, loop,
  delete the competing result, forward either incompatible result or use stale fallback.
  An exchange failure retains existing `exchangeErrorResponse` handling, with no removed
  or mismatched token fallback. Detailed outcomes are in contract §3.1.
- Body path validation follows completion of both concurrent calls and unchanged precedence
  (delegation denial, resolution error, exchange error), before buffering or Authorization
  mutation. Header-only binding uses the permit evaluated by policy after allow. Existing
  deadline checks apply immediately before every policy decision. An expired permit before
  forwarding is rejected with the existing delegation-unavailable 503, not re-resolved.
- Disabled mode, cache key, per-key singleflight key, TTL calculations, `cache.max_ttl`,
  and five-second re-consent cooldown are unchanged. Cooldown entries carry no token binding.
  This revision adds no broker response member, configuration key, schema or dependency.

**Provider limitation**: Narrowing a grant alone does not narrow the provider-session token.
A re-exchange independently checks the broker's then-current grant and session but may return
identical broader token bytes; a new handle can legitimately bind those bytes to the new
permit. Narrowed provider scopes reach upstream only when the session carries them, such as
Journey 5's re-authorization. OPA effective scopes govern local policy, not provider capability.

**Provenance limitation**: On double cache miss, evaluation and exchange can read different
grant versions concurrently; unbound first use can associate an exchange result with a
permit without evidence that exchange read that version. First-claim serialization prevents
conflicting bindings to one handle, not conflicting broker reads. The permit deadline bounds
policy snapshot age and ADR 012 TTL bounds token retention; neither proves exchange provenance
or provides a stronger race bound. This residual limitation is explicitly Proposed in D-8.

## Alternatives

- **Keep forwarding an older bound result until ADR 012 expiry**: contrary to F1; leaves
  policy context and recorded token-reuse binding divergent.
- **Return verified exchange grant version and effective scopes from `/oauth2/token`**:
  stronger provenance, but a new externally visible exchange contract. Not selected for this
  increment; would require its own API/ADR approval and broker implementation.
- **Broker/provider token downscoping**: stronger upstream capability guarantee, but changes
  provider-session behavior and possibly provider requirements. Not selected; separately
  proposed scope is required before implementation.
- **Compare credential bytes, rebind a shared result, or retry until compatible**: rejected;
  loses same-token concurrent replacements, violates ownership, or permits unbounded work.

## Consequences

- A request with a new permit cannot forward an old, differently bound result handle.
  Compatible concurrent replacements avoid unnecessary exchange; other mismatches cost at
  most one extra exchange per request, not necessarily one per grant change/cache key.
- Concurrent live permits may deterministically receive API-7 503 after a conflicting
  singleflight result; availability is traded for binding-consistent fail-closed forwarding.
- New-generation identical provider tokens and unbound first use remain permitted under the
  explicit provider/provenance limitations. These do not claim upstream downscoping.
- D-5's five-second cooldown remains unchanged; only its broader cache-unchanged statement
  is revised. All new corrections require genuine test-first evidence outside historical D-6.
