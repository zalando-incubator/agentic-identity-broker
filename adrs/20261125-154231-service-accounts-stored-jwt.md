# Service Accounts on Third-Party Services with a Stored JWT

**Status**: Accepted
**Date**: 2026-11-25

## Context

Third-party services hold only a user-facing OAuth client. Issue #478 designs agents acting on their own behalf in seven steps. Its first step, issue #471, adds a service account: a provider-issued machine credential attached to one service, with a scope ceiling, that agents can later be bound to (#472) and that token exchange later hands out (#473).

The first kind is `jwt_bearer` with the credential format `signed_jwt`: a JWT that company infrastructure signed outside the broker, stored and later handed out unchanged as an access token. The specification is [specs/20261125-152113-service-accounts-stored-jwt/spec.md](../specs/20261125-152113-service-accounts-stored-jwt/spec.md); its research records the alternatives.

This decision introduces a new entity, a new repository port and a new table, and stores a JWT the broker does not verify. Constitution Principle I requires signature verification for client assertions, actor tokens and signed subjects; reviewers need to see why a stored JWT is not such a token.

## Decision

1. **Entity and bounded context.** `model.ServiceAccount` belongs to the third-party service bounded context. It references its `ThirdpartyOAuth2Service` by `id.ServiceID` for its whole life and is identified by a new UUID-backed `id.ServiceAccountID` (`adrs/013-strongly-typed-entity-ids.md`). Its `canonical_id` follows `canonical.Validate`, is unique within the service and can be renamed; ID, service and `kind` never change. `thirdparty.ServiceAccountService` holds the workflow; no new `domain/X/` package is created.

2. **Repository port.** `ports.ServiceAccountRepository` in `internal/ports/storage.go` has seven service-scoped methods: `Create`, `Get`, `GetByCanonicalID`, `ListByService`, `Update`, `Delete`, `CountByService`. An account addressed under another service is not found by construction. Two sentinels, `ErrServiceAccountCanonicalIDConflict` and `ErrServiceAccountVersionMismatch`, classify `conflict` errors without message parsing. Memory and PostgreSQL adapters pass one shared contract test (`adrs/004-storage-layer-architecture.md`).

3. **Stored-JWT posture.** The broker stores the JWT exactly as submitted, never verifies its signature, never fetches keys for it and never rebuilds it. On every write it checks form only: at most 16384 bytes before any parsing; a compact JWS whose payload is a JWT claims set, parsed without verification by `lestrrat-go/jwx/v4` (no hand-written parser); a header `alg` that is present and not `none`; and, when an `exp` is stated, an `exp` after the request time. Library errors are never returned or logged; each failure maps to a constant message.

   This is **not** an exception to Principle I's signature rule. The broker never accepts the stored JWT as proof of anything: it authenticates nobody with it, authorizes nothing from it, and uses the values it reads only for display and for the account's status. Refusing `alg: none` guarantees the broker never stores, for later hand-out, a token that no party could verify. The exception of `adrs/031-unverified-subject-unsigned-jwt.md` is neither used nor widened.

4. **Encryption.** The plaintext is a credential document discriminated by `format` (`{"format":"signed_jwt","use":"access_token","jwt":"…"}`), encrypted through `ports.EncryptionPort` under the owning service's `service_id` context (`encryption.NewServiceBranchKeySubject`), as `adrs/008-encryption-context-optimization.md` defines. The domain service encrypts; the repository stores opaque bytes (`adrs/012-encryption-layer-separation.md`). There is no plaintext fallback: an encryption failure stores nothing. Later formats (#475, #476) reuse the column with a new `format`.

5. **Credential summary.** Each write that carries a credential computes and stores a summary: `format`, `use`, the `iss`, `sub`, `kid` and `exp` read from the token (each omitted when absent), and `SHA256:` followed by the unpadded standard base64 SHA-256 digest of the exact stored token (`crypto/sha256`). Lists and status reads need no decryption. `sub` is logged only as an `id.Principal` through the principal policy; `coding-standards/LOGGING.md` §6.2 gains one scoped row permitting the fingerprint in `service.service_account.*` audit records.

6. **Status derived on read.** `ready`, or `failed` with `credential_expired` from the instant a stated `exp` has passed, computed from the stored expiry and the broker's clock on every read without skew allowance. Nothing is swept, stored or audited at expiry. Later stored status facts (#474–#476) are added beside this rule without changing `ready` and `failed`.

7. **Concurrency.** A `version` column starts at 1, increments with every committed change and is the strong ETag of the stored account. The ETag does not version the derived status, which changes at credential expiry without a write. `If-Match` is optional on update and delete: a stale value answers `412` and changes nothing; without it the last writer wins. The repository enforces the expected version in the write statement. The service always writes conditionally on the version it read; without `If-Match` it re-reads and applies the request again, up to three attempts, so an update never restores members another writer changed after the read, and a delete audits the credential it erased. An update that changes nothing writes nothing and keeps the ETag.

8. **Service deletion.** A service that holds at least one service account cannot be deleted (`409`). The domain service counts accounts for a precise message; PostgreSQL enforces `ON DELETE RESTRICT`, and the memory provider repository checks the count under its write lock. Machine credentials are never erased as a side effect of another deletion. Deleting a bound account is refused once agent grants exist (#472) through a binding-checker port whose null implementation answers false today.

9. **Migration.** Migration `050_create_service_accounts` is one atomic pair that creates the `service_accounts` table with its checks, `UNIQUE (service_id, canonical_id)` and the `RESTRICT` foreign key. The set of kinds is enforced in the domain, not by a `CHECK`. Rolling it back drops the table and with it the stored credentials.

10. **Admin API.** Create, list, read, update, delete and status under `/api/services/{service-id}/service-accounts` on the admin server only; Admin API `3.1.0` (additive). No response contains the credential.

## Consequences

- An administrator has an auditable inventory of machine credentials per service; no agent can use one until #472 and #473.
- Renewal stays outside the broker. An expired token marks its account `failed` until someone replaces it.
- A rotation is recognisable from the fingerprint without access to the token; identical tokens show identical fingerprints.
- #472 replaces the null binding checker; #473 adds decryption for hand-out and the ADR that supersedes decision 6 of `adrs/045-agent-subject-impersonation.md`.
- No accepted ADR is superseded. `adrs/004-storage-layer-architecture.md`, `adrs/008-encryption-context-optimization.md`, `adrs/012-encryption-layer-separation.md` and `adrs/013-strongly-typed-entity-ids.md` are followed.
