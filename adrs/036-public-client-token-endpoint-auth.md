# ADR 036: Public Client Token-Endpoint Authentication

**Status**: Accepted
**Date**: 2026-09-11

---

## Context

Third-party OAuth2 services previously required a client secret. Some upstream providers issue only public clients. These providers reject token requests that carry client credentials.

The broker already sends PKCE with `S256` for every third-party authorization flow. Public clients require the same PKCE protection, but they must not store or send a client credential.

`golang.org/x/oauth2` uses `AuthStyleAutoDetect` when no style is selected. With an empty secret, this mode can first send an `Authorization` header with an empty password. It can then retry with a different authentication style. That behavior is unsafe for a public client.

ADR 012 describes `Secret` as a two-state value object. Public clients require a deliberate absent credential state. This change must supersede that part of ADR 012 without changing its domain-encryption boundary.

The administrative API confirmation record for this feature was accepted on 2026-09-10. It approves `null` and absence as confidential on write. It also approves omission of `client_secret` from public-service responses.

## Decision

1. `TokenEndpointAuthMethod` has no default. An absent method means a confidential client, and the broker never infers or persists a method for an operator. An explicit `null` is also absent.
2. `none` is the only accepted token endpoint authentication method. `client_secret_basic` and `client_secret_post` are deliberately excluded because confidential clients retain their existing negotiated behavior.
3. Public authorization-code exchanges use an empty `ClientSecret` and pin `Endpoint.AuthStyle` to `AuthStyleInParams`. Confidential exchanges retain `AuthStyleAutoDetect` and their existing wire behavior.
4. `Secret` gains an explicitly constructed absent state through `NewAbsentSecret()`. This decision supersedes the two-state `Secret` decision in ADR 012.

For public exchanges, `AuthStyleInParams` sends `client_id` in the request body. It omits `client_secret`, sends no `Authorization` header, and prevents authentication-style probing. Every third-party authorization request uses PKCE with `code_challenge_method=S256`. Every code exchange sends its flow-bound code verifier.

A public service stores an absent `Secret`. A confidential service stores an encrypted `Secret`. `GetCiphertext()` continues to error for plaintext, so adapters cannot persist plaintext. The `Secret` zero value remains plaintext-uninitialized. It never represents absence.

### Relation to ADR 017

ADR 017 establishes the representation precedent for optional attributes. It uses a pointer where `Agent.ClientID` has no useful zero value. This feature uses an empty string for the domain string enum and `*string` for the nullable PostgreSQL record. The record follows the existing nullable-column pattern. Like ADR 017, this feature has no auto-generation fallback.

This feature deliberately differs from ADR 017 update behavior. An Agent update can omit `client_id` and preserve its stored value. A third-party service update is full replacement. Omitting `token_endpoint_auth_method`, or sending `null`, requests a confidential service. The update fails unless it also supplies a client secret.

## Consequences

Public services contain no stored client credential and send none to upstream token endpoints. Confidential services preserve their existing client authentication, including negotiated authentication style.

The database migration stores no default method and does not backfill existing services. Existing services therefore remain confidential after the migration.

The down migration refuses to run while public services exist. The guard prevents deletion of public services and prevents fabrication of a credential. A refused migration rolls back its schema changes but leaves the migration state dirty.

### Guarded rollback runbook

If the down migration stops, do these steps:

1. Convert or remove each named public service.
2. Run `migrate force 31` to clear the dirty state.
3. Retry the migration.

---

## References

- [ADR 012: Encryption Layer Separation](012-encryption-layer-separation.md)
- [ADR 017: Optional Agent client_id](017-optional-agent-client-id.md)
- [Feature 042 research, D1, D2, D3, D6, and D11](../specs/042-thirdparty-public-pkce/research.md)
- [Feature 042 administrative API confirmation record](../specs/042-thirdparty-public-pkce/contracts/admin-api.md#8-confirmation-record-api-010)
- `.specify/memory/constitution.md` — Principles II, IV, V, and X
