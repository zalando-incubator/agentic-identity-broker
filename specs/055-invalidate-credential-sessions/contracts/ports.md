# Contract: Internal ports, domain operations, and audit events

**Status**: Internal design. These are not externally visible contracts. The Proposed decisions they implement are D-SCHEMA, D-FINGERPRINT, D-CURRENCY, and D-INVALIDATE in [plan.md](../plan.md#approval-items).

## New ports (`internal/ports/`)

### `ThirdpartyCredentialStateRepository` (`internal/ports/thirdparty_provider.go`)

```go
// ThirdpartyCredentialStateRepository stores the internal credential generation and
// filesystem fingerprint of each third-party service. A missing record reads as
// generation 1 without a fingerprint. Implementations never expose fingerprints in
// errors, and wrap failures in storage.StorageError.
type ThirdpartyCredentialStateRepository interface {
	// Get returns the current state, including the implicit default for a missing record.
	Get(ctx context.Context, serviceID id.ServiceID) (storage.CredentialState, error)
	// RecordBaseline stores a first fingerprint without changing the generation or sessions.
	// It returns Conflict when the generation or a non-empty fingerprint changed meanwhile.
	RecordBaseline(ctx context.Context, serviceID id.ServiceID, expectedGeneration int64, fingerprint string) (storage.CredentialState, error)
	// RecognizeChange applies change atomically: it locks the service record without modifying it,
	// compare-and-sets the generation, stores the fingerprint, and invalidates every user session
	// of the service. It returns Conflict on a stale ExpectedGeneration and NotFound for a deleted service.
	RecognizeChange(ctx context.Context, serviceID id.ServiceID, change storage.CredentialChange) (storage.CredentialChangeResult, error)
}
```

`ThirdpartyOAuth2ProviderRepository.Update` gains this contract, with no signature change: when `entity.CredentialChange` is non-nil, it applies the change in the same transaction (PostgreSQL) or lock scope (memory) as the service update. It returns no partial result. On a conflict or validation failure, it leaves the service, the credential state, and the sessions unchanged.

### `CredentialFingerprinter` (`internal/ports/credential_file.go`)

```go
// CredentialFingerprinter derives and checks one-way fingerprints of a client credential pair.
// Implementations bound concurrent work and never log inputs.
type CredentialFingerprinter interface {
	Fingerprint(ctx context.Context, clientID, clientSecret string) (string, error)
	// Matches reports false, without an error, for a well-formed fingerprint of another pair.
	Matches(ctx context.Context, fingerprint, clientID, clientSecret string) (bool, error)
}
```

The adapter is `internal/adapters/credentialfile/fingerprint.go`. It wraps `oauth2server.Argon2Hasher`, which is constructed in the builder with `maxConcurrent = 2`. It encodes the pair as `uint32be(len(clientID)) ‖ clientID ‖ clientSecret` and maps the hasher's mismatch result to `false`.

### `CredentialCurrencyChecker` (`internal/ports/oauth2.go`)

```go
// CredentialCurrencyChecker establishes that a filesystem-sourced service's credentials are
// current before an authorization is admitted. It returns nil for other sources.
type CredentialCurrencyChecker interface {
	EnsureCredentialCurrency(ctx context.Context, serviceID id.ServiceID) error
}
```

`*oauth2session.OAuth2SessionService` implements this port. The builder injects it into `oauth2.AuthorizationService` through a `With...` method. Principle XII applies.

## Domain operations

| Component | Change |
|---|---|
| `oauth2session.OAuth2SessionService` | Adds `establishCurrency(ctx, entity) (currentCredentials, error)`, with a process-local digest cache keyed by `(service_id, generation)`. `selectCredentials` receives the validated pair instead of reading again for exchange and refresh. Initiation calls `establishCurrency`, seals `CredentialGeneration`, and keeps exposing only the client ID. `GetValidAccessToken` and `refreshLockedSession` load the credential state and call `establishCurrency` for filesystem services before their fast path or session lock. `HandleCallback` rejects a sealed generation that differs from the current generation before any provider request, and returns `ErrCredentialGenerationChanged` when a reconnection is fenced and the current session is unusable. |
| `thirdparty.ThirdpartyOAuth2ProviderService` | `Update` computes `CredentialChange` for non-discovery updates (research R3), including transitions and the decryption-failure rule. It emits the change audit event after a successful commit. |
| `oauth2.AuthorizationService` | `HandleAuthorization` calls `EnsureCredentialCurrency` for each included or mandatory filesystem-sourced service before steps 4 and 5. An error yields the existing `server_error` redirect decision ("Failed to check session status"). |
| `internal/app/builder.go` | Wires the two new repositories and the fingerprinter, and injects the currency checker. |

## Error mapping (no new public codes)

| Situation | Domain result | Token exchange (RFC 8693) | Third-party browser routes |
|---|---|---|---|
| Recognition committed by this request | Rereads the expired session. Returns `DetailAccessTokenExpired`, or `DetailSessionMissing` if no session exists | `400 invalid_grant` + ADR 044 `error_uri` | Initiation continues at the new generation |
| Source unavailable or incoherent | Existing `DetailCredentialSourceUnavailable` | `500 server_error` (no `error_uri`) | Existing `500`, or `callback_failed` |
| State read or recognition commit failed | Existing `DetailRepositoryUnavailable` or `DetailPersistenceFailed` | `500 server_error` | `500` or `callback_failed` |
| Sealed generation is stale | `ErrCredentialGenerationChanged`, a new internal sentinel | — | `/sessions?error=callback_failed` |

## Audit events (`coding-standards/LOGGING.md`)

| Message | Level | `event` | Attributes |
|---|---|---|---|
| `"third-party credentials changed"` | Info | `service.thirdparty.credentials_changed` | `service_id`, `trigger` (`stored_update`, `filesystem_recognized`, or `source_transition`), `credential_generation`, `session_count`, `outcome` (`invalidated`) |
| `"third-party credential baseline recorded"` | Info | `service.thirdparty.credential_baseline_recorded` | `service_id` |
| `"third-party credential change failed"` | Error | `service.thirdparty.credential_change_failed` | `service_id`, `trigger`, `reason` (`state_conflict`, `repository_unavailable`, or `persistence_failed`), `error` (StorageError only) |
| `"oauth2 callback fenced"` | Warn | `session.oauth2.callback_fenced` | `service_id`, `principal` (`id.Principal`), `reason` (`credential_generation_changed`) |

The following never appear in any event, error, span, or metric: client IDs read from files, secrets, digests, fingerprints, file paths, or tokens. No new span name or metric instrument is added. Recognition runs within the calling operation's existing span (`coding-standards/OPENTELEMETRY.md`).
