# ADR 038: OAuth2 Client-Secret File Overrides

**Status**: Accepted
**Date**: 2026-10-08
**Feature**: 051-oauth2-secret-file-overrides

## Context

Mounted credentials can rotate independently of registered services. A hidden override behind a stored placeholder misrepresents the credential source and cannot safely change client identity.

The specification's 2026-10-07 clarification confirms the requested administrative API scope: explicit source selection, filesystem credential omission, and no stored placeholders. Filesystem configuration supplies both paths. The administrative API supplies neither path.

Released `v0.1.81` contains Admin API `1.0.0`, whose `Service` response requires `client_id`. Filesystem omission breaks that contract. On 2026-10-08, the user explicitly accepted this ADR and approved Admin API `2.0.0` as a clean cutover on the existing `/api/services` routes. T010 defines the concrete source-aware schemas and breaking-change documentation. No compatibility endpoint or shim is introduced.

**Approval record**: The implementation session's explicit choices were “Accept ADR 038” and “Major contract bump to 2.0.0”. Design prerequisites and semantic-red acceptance runs preceded production implementation. The source-aware Admin API `2.0.0` cutover is implemented but unreleased. No registry publication or live deployment occurred.

## Decision

### Explicit service sources

The existing service owns `CredentialSource`, persisted as `credential_source`, with exactly `stored` and `filesystem` values. Creation omission selects stored. Update omission preserves the source. Unknown values and explicit null are invalid.

Stored mode retains existing credential validation and encryption. It ignores filesystem bindings. Eligible shared-secret services can explicitly select filesystem mode. Public clients, CIMD confidential clients, and Google-flavor services reject that selection and retain existing behavior.

Filesystem mode requires a valid canonical ID and rejects either inline credential field, including empty or null input. It stores no client ID or secret placeholder. It uses `NewAbsentSecret()`. Administrative responses report the source and omit both inline credential fields. Metadata operations never resolve files or expose paths, values, or internal transition evidence.

Operator configuration owns `third_party_oauth2.credential_files`. Each exact, case-sensitive canonical-ID key binds `client_id_file` and `client_secret_file` absolute paths. Bindings never select a source or create services. A missing binding, including an empty mapping, fails closed for filesystem mode. No discovery or stored-credential fallback applies.

### Narrow reader port and coherent acquisition

The existing session domain depends on `ports.CredentialFileReader`:

```go
ReadClientID(path string) (string, error)
ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)
```

Builder injects the immutable configuration snapshot and filesystem adapter. No handler accesses files. No new credential framework or bounded context is necessary.

The adapter opens current symlink targets anew with non-blocking, read-only flags. It validates regular-file descriptors and a 65,536-byte limit before whitespace removal. A 65,537-byte bounded read detects growth. `strings.TrimSpace` removes surrounding whitespace, preserves internal characters, and must produce a non-empty value.

`ReadClientID` never accesses a secret path. `ReadPair` opens both descriptors before either read. After both bounded reads, it revalidates both configured paths with `os.SameFile` against their open descriptors. Detected target changes produce `generation_changed`. Unavailable targets produce the applicable safe source error. Both descriptors close before return on every path. Errors return neither value, with no acquisition retry.

Coherence depends on complete pair publication through fresh immutable targets. Providers never modify published targets or reuse retired targets during acquisition. No common-parent restriction or provider-specific generation identifier applies. Publication after validation can leave an in-flight operation using its validated pair. Later acquisitions use current targets. Identity checks and provider validity still apply.

### Three operation boundaries and established identity

| Boundary | Filesystem acquisition | Identity association |
|---|---|---|
| Authorization initiation | One `ReadClientID` call. No secret read. | New eligible stored and filesystem flows seal their selected identity in the existing JWE. |
| Code exchange | One `ReadPair` call per broker-owned exchange attempt. | Compare the selected identity with the sealed initiating identity before provider authentication. |
| Token refresh | One `ReadPair` call per actual refresh attempt. | Compare the selected identity with the established session identity before provider authentication. |

Both eligible source modes enforce recorded identities, including after either source transition. A mismatch stops authentication and requires a new connection. A matching identity can continue, subject to provider validity.

The implemented fields are `OAuth2StateTokenClaims.UpstreamClientID *id.ClientID` and `storage.UserSession.UpstreamClientID *id.ClientID`. The JWE claim is `upstream_client_id,omitempty`. Successful exchange copies the verified initiating identity into the session. Session JSON excludes it. Neither source transitions nor refresh rewrite it to conceal a mismatch.

The internal boolean `credential_source_transitioned` starts false on creation and legacy backfill. An actual source change atomically sets it true. It never resets, including after return to stored. No-op selections and metadata updates preserve it. The administrative API never exposes it.

Missing legacy identity remains usable only for a stored service whose marker is false. Filesystem mode or prior transition produces `identity_missing`, including after a source round trip. Never infer identity from current files, stored credentials, or provider tokens. Excluded flows remain unchanged.

Operation credentials remain local. No file secret, file path, descriptor, or last-known credential persists across operations. Only the non-secret established identity enters authorization/session context.

### Source errors

`model.CredentialSourceError` supports `ErrCredentialSourceUnavailable` and closed, non-secret reasons. Reasons include `missing_binding`, `identity_missing`, `identity_mismatch`, `generation_changed`, `not_found`, `permission_denied`, `not_regular`, `too_large`, `empty`, and `read_failed`.

Source errors stop before provider authentication. Printable errors expose no contents, paths, raw OS error chains, or provider payloads. Credential-free events distinguish source failure, successful selection, and provider rejection. Selection does not prove provider acceptance. Existing generic initiation, callback, refresh, and token-exchange responses remain unchanged.

Source errors retain their sentinel and bounded reason through session `OperationError`. Its metadata uses detail `credential_source_unavailable`, kind `infrastructure`, and dependency `credential_source`. Token exchange derives the matching typed diagnostic without a recovery URI. Provider rejection remains distinct from source failure and locally recorded token expiry.

### Both-store persistence and guarded migration

Existing migration `036_add_oauth2_credential_source_and_client_identity.{up,down}.sql` adds stored-default source, false-default transition evidence, nullable filesystem inline credentials, and nullable session `upstream_client_id`. Backfill preserves existing authentication and ciphertext. Existing session identities remain NULL. No identity inference or token rewrite occurs.

Source-aware constraints require filesystem canonical IDs, absent inline credentials, eligible authentication mode, and non-Google flavor. Stored constraints preserve existing shared-secret, public, and CIMD alternatives. Invalid NULL combinations cannot pass through SQL CHECK unknown results.

Both PostgreSQL and in-memory repositories persist the complete validated source, credentials, marker, and session association. Existing indexes, version checks, protected-resource transactions, session locks, and refresh copies remain intact. Stored-to-filesystem removes both credentials atomically with the source and marker. The reverse requires explicit non-empty credentials and domain encryption. Rejection or CAS conflict preserves the prior state.

DOWN refuses filesystem rows, true transition markers, or other incompatible records. It cannot delete services, invent credentials, or discard evidence that protects legacy contexts. Compatible DOWN removes the new columns and loses the non-secret session association. UP/DOWN/reapply and refusal require regression coverage. Dirty migration metadata requires validation of intact schema and data before recovery. A source round trip alone never makes downgrade safe.

### Narrow supersession and preserved boundaries

This ADR supersedes only ADR 036's ordinary-confidential encrypted-stored-secret invariant for selected eligible filesystem services. Stored services retain that invariant. ADR 036's public secretless wire behavior, unconditional S256 PKCE, and confidential auth-style negotiation remain binding. Omission preserves only `credential_source`. Unrelated PUT semantics remain unchanged.

ADR 037's CIMD key domain and trust surfaces remain unchanged. ADR 012's domain-owned encryption remains unchanged. Stored secrets and session tokens retain their existing encryption. ADR 008's service-scoped AAD remains exactly `service_id`. The identity association and transition marker are not AAD. Existing provider retry policy, authentication-style negotiation, refresh locking, and singleflight remain unchanged.

## Consequences

- Operators can select filesystem credentials without stored placeholders or metadata dependence on file availability.
- Fresh acquisition supports same-identity secret rotation without service writes or restarts. Source loss never selects fallback credentials.
- Client-ID changes require new connections for mismatched codes and sessions. Provider overlap remains an operator prerequisite, not a continuity guarantee.
- Both stores, migration 036, repository administrative consumers, and all 50 acceptance journeys changed together. The journeys passed in individual story runs. The full final gate remains pending. External administrative consumers must migrate before release.

## References

- [Feature specification and 2026-10-07 clarification](../specs/051-oauth2-secret-file-overrides/spec.md#session-2026-10-07)
- [Feature plan](../specs/051-oauth2-secret-file-overrides/plan.md)
- [Data model](../specs/051-oauth2-secret-file-overrides/data-model.md)
- [Research and release evidence](../specs/051-oauth2-secret-file-overrides/research.md)
- [ADR 036: Public Client Token-Endpoint Authentication](036-public-client-token-endpoint-auth.md)
- [ADR 037: CIMD Client-Authentication Key Domain](037-cimd-client-authentication-key-domain.md)
- [ADR 012: Encryption Layer Separation](012-encryption-layer-separation.md)
- [ADR 008: Encryption Context Optimization](008-encryption-context-optimization.md)
- [ADR 004: Storage Layer Architecture](004-storage-layer-architecture.md)
