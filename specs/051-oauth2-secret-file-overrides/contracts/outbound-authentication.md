# Contract: Outbound Credential Selection

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Refreshed T009 for coherent pair acquisition, identity checks in both source modes, and irreversible transition evidence.

This contract describes implemented, unreleased behavior under ADR 038. The source fields, reader port/adapter, events, identity associations, both stores, and migration 036 exist. It does not claim a published broker release or a completed full final gate.

## Operation boundaries

Credential selection applies at three boundaries:

- Authorization initiation through `OAuth2SessionService`, which needs the effective client ID for its redirect
- Authorization-code exchange through `HandleCallback()` and `exchangeCodeWithRetry()`, which need the client ID and secret
- Token refresh through `RefreshAccessToken()`, including forced session refresh and automatic refresh during RFC 8693 token exchange.

~~Authorization initiation never reads override files.~~ Superseded because filesystem initiation requires the current client-ID file. It never reads the secret file. Administration, consent, and session metadata remain file-independent.

The service owns `credential_source: stored | filesystem`. Operator configuration owns the two paths. Filesystem mode needs neither a stored credential nor a placeholder. Existing `Secret` encryption semantics remain intact. Filesystem services use the explicit absent state, `model.NewAbsentSecret()`, not the uninitialized zero value.

## Selection

~~A matching binding selects a file secret, and an absent binding selects the stored secret.~~ Superseded because only the registered source selects credentials.

| Service/source | Matching canonical-ID binding | Credential behavior |
|---|---|---|
| Public (`none`) | Ignored | Existing secretless request. Filesystem selection is invalid. |
| CIMD confidential (`private_key_jwt`) | Ignored | Existing fresh signed assertion. Filesystem selection is invalid. |
| Google flavor | Ignored | Existing Google-flavor behavior. Filesystem selection is invalid. |
| Eligible confidential / `stored` | Ignored, including a present binding | Existing stored client ID and managed secret. No file read. |
| Eligible confidential / `filesystem` | Absent, including an empty mapping | `missing_binding` before a provider request. No fallback. |
| Eligible confidential / `filesystem` | Present and required values usable | Current client ID for initiation. Current pair for exchange/refresh, subject to identity validation. |
| Eligible confidential / `filesystem` | Present but a required file unusable | Source error before a provider request. No fallback. |

Eligibility uses existing entity predicates plus the Google exclusion. It does not infer authentication modes from file or stored-credential presence.

The binding uses the service's current canonical ID, not its UUID. Matching is exact, including case and dots. UUIDs identify events and persisted service relationships.

`ports.ThirdPartyOAuth2Config.CredentialFiles` maps canonical IDs to `ports.CredentialFileBinding`. Each binding contains `ClientIDFile` and `ClientSecretFile`, with YAML/JSON fields `client_id_file` and `client_secret_file`. [configuration.md](./configuration.md) defines source precedence and pair validation. Binding removal or a winning empty mapping never changes filesystem mode to stored.

## Administrative source contract

The approved selector and source-specific schemas appear in `api/admin/openapi.yaml`, with `info.version: 2.0.0`. [data-model.md](../data-model.md) defines entity and persistence invariants. Administrative requests cannot supply file paths or construct paths from request identifiers.

| Administrative operation | Required source behavior |
|---|---|
| Create with omitted `credential_source` | Select `stored` and apply existing authentication-mode validation. |
| Create or update with an explicit selector | Accept only `stored` or `filesystem`. Reject unknown values and explicit null. Null is not omission. |
| Update with omitted `credential_source` | Preserve the current source. An unrelated update never changes it. |
| Create/update an eligible stored confidential service | Preserve existing non-empty inline `client_id` and `client_secret` validation. |
| Create/update a filesystem service | Require a valid canonical ID. Reject either inline field when present, including empty or null. Do not read files or silently ignore mixed sources. |
| Select filesystem for public, CIMD confidential, or Google flavor | Reject the unsupported combination without file access or changing the existing record. Their other validation stays unchanged. |
| Switch stored to filesystem | Require an explicit selector and no inline fields. Atomically remove both stored credentials, persist filesystem mode, and set `credential_source_transitioned: true`. Do not require available files. |
| Switch filesystem to stored | Require an explicit selector and explicitly supplied non-empty client ID and secret. Atomically persist encrypted stored credentials and preserve `credential_source_transitioned: true`. Never import file contents. |
| Clear a filesystem service's canonical ID | Reject the update. A different valid canonical ID preserves filesystem mode and requires its binding at use. |
| Read a stored confidential service | Report `credential_source: stored`, its stored client ID, and the existing `REDACTED` secret representation. |
| Read a filesystem service | Report `credential_source: filesystem`. Omit both `client_id` and `client_secret`. Return no synthetic `REDACTED` placeholder or path, and read no files. |

Handlers must retain inline-field presence through validation. An empty or null field still counts as supplied input for filesystem mode. This rule prevents pointer decoding from silently converting mixed sources into omission.

The source-omission rule applies only to `credential_source`. It does not change ADR 036's full-replacement semantics for `token_endpoint_auth_method` or other unrelated fields.

Existing records migrate to stored mode without changing confidential, public, or CIMD behavior. Creation and legacy backfill set internal `credential_source_transitioned` to false. An actual source change sets it to true in the same version-checked update as credential removal or replacement. Once true, it never resets. No-op selections and unrelated updates preserve it. Rejected updates and concurrency conflicts preserve the previous source, credentials, and marker.

Never expose or accept the transition marker through the administrative API. File configuration never registers services or selects their source. Filesystem-only credential omission does not change stored redaction or excluded-mode representations.

Published `v0.1.81` contains Admin API `1.0.0`, whose response requires `client_id`. Filesystem omission breaks that contract. On 2026-10-08, the user accepted ADR 038 and approved the major contract bump to `2.0.0`. The implemented source-aware schema uses existing `/api/services` routes without a compatibility endpoint or shim. Repository administrative consumers use the new representation. [`docs/changelog.md`](../../../docs/changelog.md) records the unreleased breaking change and external consumer migration.

The [release evidence](../research.md#decision-9-preserve-repository-release-note-conventions) remains the published baseline, not a feature-release claim.

## Reader port

**Reader port:** `internal/ports/credential_file.go`.

**Implemented interface:** `ports.CredentialFileReader`, with two operations. `internal/adapters/credentialfile/reader.go` implements the port. Builder owns injection into the session domain.

```go
ReadClientID(path string) (string, error)
ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)
```

This is an internal port, not a public HTTP endpoint. Builder injects its `internal/adapters/credentialfile/` adapter into the session service. Only validated operator configuration supplies paths.

`ReadClientID` opens the configured client-ID path anew in read-only, non-blocking mode. It follows the current symlink target without retaining its resolved pathname. It never accesses the secret path. Before reading, it inspects the opened descriptor and rejects non-regular targets or a size greater than 65,536 bytes. It bounds the read to 65,537 bytes and rejects an oversized result. It removes surrounding whitespace with `strings.TrimSpace`, preserves internal characters, and rejects an empty normalized value.

`ReadPair` must:

1. Open both configured paths anew in read-only, non-blocking mode before reading either value.
2. Follow each current symlink target without retaining its resolved pathname.
3. Inspect both opened descriptors before reading. Reject non-regular targets and sizes greater than 65,536 bytes per file.
4. Bound each read to 65,537 bytes. Reject an oversized result, including growth after descriptor inspection.
5. Remove surrounding whitespace with `strings.TrimSpace`. Preserve internal characters and reject either empty normalized value.
6. After both bounded reads finish, inspect both configured paths. Use `os.SameFile` to compare each current target with its respective opened descriptor.
7. If either comparison identifies a different target, return `generation_changed`. If a target is unavailable, return its applicable safe source error.
8. Keep both descriptors open through pair validation. Close both before returning on success or failure.

Both operations close every opened descriptor on every exit, including failure while opening the second file. A failed `ReadPair` returns neither credential value. No partial pair, acquisition retry, cached value, or retained descriptor is permitted.

A pathname check before a blocking open is insufficient. A target can change between those operations. Descriptor inspection and read limits protect the actual opened objects. All reader failures use the safe domain source-error category from [data-model.md](../data-model.md), never raw OS error chains.

## Freshness and retries

Each filesystem broker-owned code-exchange attempt calls `ReadPair` exactly once, immediately before `Config.Exchange()`. Existing authentication-style probing inside that call uses the same validated pair. The broker preserves negotiation and provider retry policy. A later broker-owned attempt acquires a fresh pair rather than reusing the previous attempt's values.

Filesystem authorization initiation calls `ReadClientID` anew and never accesses the secret path. Each actual filesystem refresh calls `ReadPair` exactly once before provider authentication. Existing refresh locking and singleflight remain unchanged. A request with a valid stored access token does not read files merely because it passes through token exchange.

A source error or identity mismatch stops the attempt without a provider request or acquisition retry. A later operation can succeed after source recovery. No operation uses cached credentials or a retained descriptor.

Pair coherence depends on the immutable-target publication contract. Providers publish a complete, consistent pair through fresh immutable file targets. They never modify a published target or republish a retired target during acquisition. Partial writes or partial pair publication are outside the delivery contract. No common-parent restriction or provider-specific generation identifier is required.

After both reads, successful `os.SameFile` revalidation establishes pair coherence under that publication contract. A detected target change produces `generation_changed` before provider authentication. An unavailable target produces its applicable safe source error. Neither case returns a partial pair or triggers an acquisition retry.

Publication after validated acquisition can leave an in-flight operation using its validated pair. Established client-identity checks and provider validity still apply. Subsequent acquisitions reopen the current targets. Operators remain responsible for complete publication and sufficient old/new provider validity overlap. This contract does not guarantee uninterrupted rotation or prove deployment timing.

## Client-identity association

`OAuth2StateTokenClaims.UpstreamClientID *id.ClientID` uses sealed JWE JSON `upstream_client_id,omitempty`. New eligible authorizations capture the effective client ID in stored and filesystem modes. Existing state integrity, principal/service binding, TTL, and PKCE checks remain unchanged.

Before each eligible broker-owned exchange attempt, the session domain compares the selected client ID with the verified initiating identity in either source mode. Filesystem mode uses the validated pair. Stored mode uses its stored credentials without file access. Successful exchange copies the verified identity to `storage.UserSession.UpstreamClientID *id.ClientID`. Both storage adapters preserve the nullable association. PostgreSQL uses `upstream_client_id TEXT`.

Session JSON excludes it with `json:"-"`. Public session DTOs remain unchanged.

Before each actual eligible refresh, compare the selected client ID with the session's established identity in either source mode. A different ID produces `identity_mismatch` before provider authentication and requires a new connection. A supplied empty identity claim is invalid. Existing JWE integrity, principal/service binding, TTL, and PKCE checks remain mandatory.

A recorded eligible association remains authoritative across both source transition directions. Matching identities can continue subject to provider validity. Source updates never relabel old codes or sessions. Refresh and token updates preserve the association. A new successful connection can replace it with that connection's established identity alongside its new tokens.

Legacy eligible contexts without an association retain behavior only in stored mode with `credential_source_transitioned: false`. Filesystem mode or a prior transition produces `identity_missing` before provider authentication and requires a new connection. A stored-to-filesystem-to-stored round trip never restores the legacy exception. Public, CIMD confidential, and Google-flavor contexts remain unchanged. Never infer legacy identity from current files, service credentials, or provider token contents.

After file client ID A changes to B, new authorization uses B. A's pending codes and established sessions stop before authentication as B. Secret rotation for the same client ID uses the next secret without a registration update. No automatic identity migration or continuity guarantee applies.

## Errors and existing HTTP responses

`internal/domain/model/credential_source_error.go` defines `model.ErrCredentialSourceUnavailable` and `CredentialSourceError`. They remain distinct from provider-rejection categories such as `ErrRefreshFailed` and `RefreshRejectedError`.

The closed source reasons are `missing_binding`, `identity_missing`, `identity_mismatch`, `generation_changed`, `not_found`, `permission_denied`, `not_regular`, `too_large`, `empty`, and `read_failed`. Printable errors include no file path, contents, provider payload, or raw OS error chain.

| Entry point | Source-error response | Existing behavior retained |
|---|---|---|
| Authorization initiation | Existing generic `internal_error` response | No file detail or provider redirect on a source failure. |
| Third-party OAuth2 callback | HTTP 302 to `/sessions?error=callback_failed` with the existing generic description | No source-specific client code or file detail. |
| `POST /api/third-party/{serviceId}/session/refresh` | HTTP 500, `error: internal_error` | Existing generic session error envelope. |
| RFC 8693 `POST /oauth2/token` requiring refresh | HTTP 500, `error: server_error`, without `error_uri` | Existing generic token-exchange error envelope. |

A provider rejection after a usable read remains a provider error. Explicit session refresh retains `502 refresh_failed`. Token exchange retains its existing `invalid_grant` or `server_error` classification. Do not propagate provider-controlled descriptions or secret-bearing bodies.

Source failures bypass misleading provider/PKCE classification. Operational end-user HTTP envelopes and errors remain unchanged. ~~The authoritative OpenAPI files do not change.~~ Superseded only for administrative source selection and representation. Administrative OpenAPI and affected repository management consumers implement Admin API `2.0.0`.

## Structured event schema

Emit through the existing context-aware `slog.Logger`:

~~Use secret-only `client_secret_override_used`, `client_secret_source_failed`, and `client_secret_provider_rejected` event names.~~ Superseded because filesystem selection covers both credentials and three boundaries.

| Event identifier | `operation` | `outcome` | `reason` |
|---|---|---|---|
| `session.oauth2.credential_files_used` | `authorization_initiation`, `code_exchange`, or `refresh` | `used` | Not required. |
| `session.oauth2.credential_source_failed` | `authorization_initiation`, `code_exchange`, or `refresh` | `failed` | Closed source reason from the data model. |
| `session.oauth2.credential_provider_rejected` | `code_exchange` or `refresh` | `rejected` | `provider_rejected`. |

Every event includes `event`, UUID `service_id`, `operation`, and `outcome`. Failure events include the non-secret reason.

Emit the use event only after required acquisition and identity validation succeed. Initiation requires only `ReadClientID`. Exchange and refresh require one coherent `ReadPair`. Emit before the provider operation. A use event proves selection, not provider acceptance.

Emit source failure for missing bindings, unusable required files, `generation_changed`, and missing or mismatched required identity. Identity failures also apply in stored mode, including legacy absence after a source transition. A source failure produces no use or provider-rejected event for that attempt.

Emit provider rejection only after a usable filesystem pair reaches the provider during exchange or refresh. Initiation cannot emit this event. Transport and response-decoding failures retain existing categories, not `provider_rejected`.

RFC 8693 source failures use `token_exchange.failure_detail=credential_source_unavailable`, `token_exchange.failure_stage=refresh`, and `token_exchange.outcome=infrastructure_error`. Provider `invalid_grant` uses `refresh_rejected` with `reauth_required`, distinct from locally recorded `refresh_token_expired`. Provider client rejection and infrastructure failures retain their existing typed diagnostics. This telemetry change adds no API response field.

Never include credential values, raw file contents, file paths, token forms, Authorization headers, assertions, or provider-controlled descriptions in these events. Printable source errors contain only bounded non-secret reasons.

## Persistence and metadata invariants

~~No API/schema change is necessary, and admin updates only manage a stored secret without disabling a binding.~~ Superseded because the service must persist an explicit source and support atomic transitions.

Persist `credential_source`, internal `credential_source_transitioned`, and source-specific credential absence on the existing provider entity. Creation and legacy backfill set the marker to false. Actual source changes set it to true atomically and never reset it. Both PostgreSQL and memory adapters must preserve these invariants from [data-model.md](../data-model.md), including existing encrypted stored credentials and excluded authentication behavior.

A stored-to-filesystem transition intentionally removes stored credentials. Normal file reads and rotation never write a provider record or assign file values to its `ClientID` or `Secret`. Filesystem records retain neither file values nor paths. Removing a binding never restores credentials or stored mode.

The only persisted file-derived value is the non-secret established identity in sealed authorization context and the session. Do not persist file secrets. Keep existing token/session encryption, service-ID AAD, and normal token persistence after successful provider authentication.

Administrative reads/updates, consent, and session metadata read no files. Filesystem administrative responses alone omit both inline credential fields under this feature. Stored confidential redaction and excluded-mode representations remain unchanged. No administrative or metadata response exposes configured paths, file values, the internal transition marker, or a new established-identity field.

Existing migration `036_add_oauth2_credential_source_and_client_identity.{up,down}.sql` enforces source-aware constraints and backfills services to stored/false. It permits nullable filesystem credentials and adds nullable session identity without inferring legacy associations or rewriting tokens.

Guarded downgrade must reject filesystem rows, every true source-transition marker, and other data incompatible with the prior credential/authentication constraint. Removing transition evidence can revive legacy missing-identity contexts, even after return to stored. Never delete records, invent credentials, reset evidence, or coerce sources to permit rollback.

Compatible downgrade requires never-transitioned stored records and an explicit notice that it discards non-secret session identity associations. Refusal must leave schema and data unchanged. A refused DOWN can leave dirty go-migrate metadata after SQL rollback.

Before `migrate force 36`, validate intact version-036 schema and data. Retain version 036 while transition evidence remains. Returning a service to stored does not make rollback safe. Never force an unverified schema version.

Accepted ADRs 012, 036 public-client authentication, and 037 CIMD key-domain separation remain binding. The user accepted ADR 038 on 2026-10-08. It defines implemented source choice and narrowly supersedes ADR 036's confidential stored-secret invariant only for selected filesystem services. Admin API `2.0.0` approval and the unreleased cutover appear in the administrative contract section. Historical design and test-first gates preceded implementation. The full final gate remains pending.

## Verification obligations

Use non-secret synthetic values. Observe both HTTP Basic and form credentials at the mock provider. Source failures must produce zero additional provider token requests.

Map all 50 numbered acceptance journeys one-to-one to Ginkgo `It()` cases, as listed in plan.md. Cover both source modes, file-pair rotation/recovery, first use, source loss/restart, exact matching, excluded modes, and disclosure prevention. Exercise each required file independently. A missing secret must not prevent initiation, but a missing client ID must prevent its provider redirect.

Cover create/read/update defaults and field presence, explicit transitions, canonical-ID changes, legacy upgrade, both adapters, and migration guards. Require false markers on creation/backfill, atomic true markers on actual transitions, and preservation on no-op, rejected, or conflicted updates. Prove source errors retain generic responses at all three boundaries and provider errors retain existing classifications. Assert fixed event names and internal categories separately from client-facing errors.

Prove that new authorization uses B after a file-ID change, while A's pending code and session produce zero provider authentication requests. Exercise established-identity comparison in both eligible source modes across both transition directions. Require `identity_missing` for legacy absence after stored → filesystem → stored, with irreversible transition evidence. Preserve never-transitioned legacy stored and excluded compatibility without inferring identity.

Use deterministic real-I/O interleavings between pair opens, reads, and revalidation. Require `generation_changed`, no returned partial values, closed descriptors, zero provider requests, and next-acquisition recovery. Cover immutable-target publication and permitted use of a validated pair after later publication. Distinguish acquisition failure from existing provider retries and authentication-style probing.

Compare service source, inline-credential presence, ciphertext, version, and timestamps before/after authentication. File use and rotation must not mutate the service. Explicit source transitions must mutate it atomically. Session-token updates and non-secret identity persistence remain permitted.

Use production bootstrap, the real loader/CLI boundary, non-root file permissions, and real chart rendering where the scenario requires them. Retain semantic-red evidence and existing-behavior compatibility controls. All 50 cases passed in individual story runs. Selected unit/integration packages and 30 legacy public/CIMD journeys passed. These results do not claim a completed full final gate, registry publication, or live deployment.
