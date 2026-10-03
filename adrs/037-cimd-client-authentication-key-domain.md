# ADR 037: CIMD Client-Authentication Key Domain

**Status**: Accepted
**Date**: 2026-09-21

---

## Context

The broker signs access tokens with locally managed signing keys. It publishes the public verification keys through `/oauth2/jwks.json`. Those keys serve the broker authorization-server trust boundary.

CIMD confidential third-party services need a different trust boundary. The broker must sign outbound `private_key_jwt` assertions. Third-party authorization servers must obtain only the matching public keys from a Client ID Metadata Document. A token-signing key must never sign a client assertion. A CIMD key must never sign a broker-issued access token.

The existing signing-key lifecycle already provides encrypted private-key storage, activation grace, immediate promotion, explicit removal, and global `kid` uniqueness. A second independent lifecycle would duplicate security-critical behavior. Reusing the unscoped token-signing manager would mix trust surfaces and could expose CIMD keys through `/oauth2/jwks.json`.

## Decision

Add a required `key_domain` discriminator to the existing signing-key model and persistence table. The closed values are:

- `token_signing` for broker-issued access-token keys.
- `cimd_client_authentication` for outbound CIMD client-assertion keys.

Backfill existing rows as `token_signing`. Keep `kid` globally unique across both domains. Enforce at most one active current key per domain.

Share lifecycle mechanics, not key ownership. Each domain has domain-filtered repository operations for listing, lookup, bootstrap, promotion, removal, row locking, and current-key selection. The token issuer and aggregated `/oauth2/jwks.json` publisher use only `token_signing`. The CIMD metadata service, JWK publisher, and assertion signer use only `cimd_client_authentication`.

CIMD keys use ES256 only. Their private material stays inside the CIMD key service. It uses a CIMD branch-key namespace and an encryption context that contains exactly one `kid` subject. The context must not contain a service ID, a key domain, or multiple subjects.

CIMD lifecycle routes are separate from token-signing routes. They are available in proxy, local, and hybrid modes. They must not expose token-signing keys or make token-signing routes available in proxy mode.

## Consequences

### Positive

- The database, adapters, and domain services enforce a distinct trust boundary.
- Public CIMD JWK Sets and `/oauth2/jwks.json` cannot share a key ID.
- Existing lifecycle behavior remains consistent without copying cryptographic code.
- A key failure can fail closed within its domain without changing other authentication modes.

### Negative

- Every signing-key repository operation needs a domain argument or domain-specific contract.
- The migration must replace the global active-current constraint with a per-domain constraint.
- Both storage adapters, migrations, token issuance, public-key publication, and tests require coordinated changes.

## Alternatives Considered

### Reuse the token-signing key domain

Rejected. It allows a token key to sign a client assertion and risks publishing CIMD verification keys through the token JWK Set.

### Add a separate CIMD key table

Rejected. It duplicates lifecycle code and cannot enforce global `kid` uniqueness across both public key sets.

### Create one CIMD key set per service

Rejected. The feature requires one broker-global CIMD key set. Per-service keys add unnecessary lifecycle, storage, and rotation complexity.

### Share encryption context with service credentials

Rejected. CIMD keys are broker-global assets. A service ID misrepresents their scope. ADR 008 requires one stable non-secret subject, so CIMD key ciphertext uses only `kid`.

## References

- Feature plan: `specs/046-cimd-upstream-client/plan.md`
- Data model: `specs/046-cimd-upstream-client/data-model.md`
- ADR 008: `adrs/008-encryption-context-optimization.md`
- ADR 009: `adrs/009-envelope-encryption-design.md`
- ADR 004: `adrs/004-storage-layer-architecture.md`
- Constitution: `.specify/memory/constitution.md` Principles I, II, III, VI, VIII, IX, and XII
