# Data Model: Explicit OAuth2 Credential Sources

**Propagated**: 2026-10-07 — Updated from spec.md refinement

**Updated**: 2026-10-08 — Aligned with implemented source-aware constraints, atomic persistence, nullable identity, and guarded migration 036 under accepted ADR 038.

## Model boundary

~~This feature changes outbound credential selection, not registered-service persistence. It introduces no aggregate, repository, table, migration, or UUID entity type.~~ Superseded by FR-021–FR-025.

The existing service owns an explicit credential source. Both stores and a migration support source-specific credentials and session identity. No new aggregate or UUID entity is introduced.

This document describes the implemented, unreleased model and existing migration 036. [spec.md](./spec.md), [plan.md](./plan.md), and [ADR 038](../../adrs/038-oauth2-client-secret-file-overrides.md) remain authoritative.

## Existing entity: Registered third-party service

The implementation uses `model.ThirdpartyOAuth2ProviderEntity` in `internal/domain/model/thirdparty_oauth2_provider.go`.

| Existing field | Role in this feature | Invariant |
|---|---|---|
| `ID id.ServiceID` | Identity in events, sessions, storage, and encryption contexts | Remains the service UUID. |
| `CanonicalID *string` | Exact binding selector | Filesystem requires a valid canonical ID. Clearing it is invalid. Comparison is case-sensitive. Stored and excluded services retain existing rules. |
| `ClientID id.ClientID` | Stored provider client identity | Stored confidential mode requires its existing non-empty value. Filesystem uses the zero value, never a placeholder or imported file value. |
| `Secret model.Secret` | Stored credential or explicit absence | Filesystem uses `model.NewAbsentSecret()`. Stored-secret encryption and excluded-mode absent-secret behavior remain unchanged. |
| `CredentialSource model.CredentialSource` | Service-owned source | Exactly `stored` or `filesystem`. Persist as `credential_source`. |
| `CredentialSourceTransitioned bool` | Irreversible internal source history | Persist as `credential_source_transitioned`. False on creation/backfill. True atomically on the first actual source change. Never expose it through the admin API or reset it. |
| `TokenEndpointAuthMethod` | Public or CIMD exclusion | `none` and `private_key_jwt` ignore bindings. |
| `Flavor model.OAuth2Flavor` | Google exclusion | Google ignores bindings. Default-standard and GitHub confidential services remain eligible. |
| `Endpoints` | Existing authorize and token endpoints | The file cannot alter either endpoint. |
| `Version`, `UpdatedAt` | Existing record metadata | Override use and rotation do not update them. |

~~An eligible confidential service still requires a non-empty managed secret at registration. A placeholder does not weaken registration validation.~~ Superseded for filesystem mode. Stored confidential validation remains unchanged.

Retained constraints: “Comparison is case-sensitive”, “At most 65,536 bytes before whitespace removal”, and “No cross-operation cache, retained descriptor, or last-known value”. Refined FR-009 requires “one coherent pair under the immutable-target publication contract”. Refined FR-025 permits legacy absence only for “stored services that have never changed source”.

## Configuration value: Filesystem credential binding

~~`ThirdPartyOAuth2Config.ClientSecretFiles map[string]string` represented secret-only bindings.~~ Removed because filesystem mode supplies both credentials.

**Representation:** `ports.ThirdPartyOAuth2Config.CredentialFiles map[string]ports.CredentialFileBinding`. The binding fields are `ClientIDFile` and `ClientSecretFile`, with `client_id_file` and `client_secret_file` configuration names.

| Component | Type | Validation |
|---|---|---|
| Canonical-ID key | String | Existing `canonical.Validate()`: 1–128 letters, digits, `.`, `_`, or `-`, excluding UUID-shaped strings. |
| `client_id_file` | String | Non-empty, not whitespace-only, and absolute on the broker platform. |
| `client_secret_file` | String | Non-empty, not whitespace-only, and absolute on the broker platform. |

The mapping belongs to one application configuration snapshot. It is not attached to a service record. Startup does not require a matching registered service or usable file.

~~An absent mapping or winning `{}` disables overrides.~~ Superseded because service-source selection is explicit. A higher-precedence source still replaces the entire mapping. Empty configuration leaves filesystem services selected but unable to authenticate without a binding. Case folding, filename inference, UUID matching, and cross-source entry merging remain prohibited.

The configuration contract is in [contracts/configuration.md](./contracts/configuration.md).

## Operation value: Current credentials

**Representation:** Local normalized strings returned by `ports.CredentialFileReader`:

```go
ReadClientID(path string) (string, error)
ReadPair(clientIDPath, clientSecretPath string) (clientID, clientSecret string, err error)
```

Initiation calls `ReadClientID` without secret-path access. Each filesystem exchange/refresh attempt calls `ReadPair` exactly once. A failed acquisition returns neither credential value.

| Property | Invariant |
|---|---|
| Origin | Only the configured path for the exact canonical-ID key. |
| Target | The opened descriptor identifies a regular file. Symlinks can select its current target. |
| Coherence | Open both targets before either bounded read. After both reads, revalidate both paths with `os.SameFile` against their open descriptors. Detected target changes return `generation_changed`, without acquisition retries. |
| Size | At most 65,536 bytes before whitespace removal. |
| Normalization | Remove surrounding whitespace with `strings.TrimSpace`. Preserve internal characters. |
| Validity | The normalized string is non-empty. |
| Lifetime | One authorization initiation, broker-owned code-exchange attempt, or actual token-refresh operation. |
| Persistence | Neither file value populates the service. Only the established non-secret client-identity association enters authorization/session context. File secrets and paths never persist. |
| Retention | No cross-operation cache, retained descriptor, or last-known value. |
| Disclosure | Never include it in errors, events, logs, diagnostics, admin output, or metadata. |

This conceptual value does not require another public wrapper or persisted credential model. Local strings are sufficient. Reuse the existing `model.Secret` absent/encrypted states for registered credentials.

Both descriptors remain open through pair validation and close before return on every path. Unavailable targets return their applicable safe source error. Providers publish a complete pair through fresh immutable targets. They never modify published targets or republish retired targets during acquisition. Successful revalidation establishes coherence under that publication contract.

Publication after validation can leave an in-flight operation using its validated pair. Later acquisitions use current targets. Established-identity checks and provider validity remain mandatory.

## Error value: Credential-source error

~~The proposed `client_secret_source_error.go`, `ErrClientSecretSourceUnavailable`, and `ClientSecretSourceError` described a secret-only override.~~ Superseded by the shared source category.

**Owner:** `internal/domain/model/credential_source_error.go`. `ErrCredentialSourceUnavailable` supports `errors.Is`. `CredentialSourceError` carries one bounded reason. The reader returns this domain category rather than raw OS errors.

| Reason | Meaning |
|---|---|
| `missing_binding` | Filesystem mode has no exact canonical-ID file pair. |
| `identity_missing` | An eligible exchange/refresh has no established identity while the source is filesystem or the service has transitioned before. |
| `identity_mismatch` | The effective client ID differs from the established authorization/session identity. |
| `generation_changed` | A configured path no longer identifies its opened target after pair acquisition. |
| `not_found` | The source or a symlink target is missing. |
| `permission_denied` | The broker cannot open or read the source. |
| `not_regular` | The opened target is a directory, FIFO, socket, device, or another non-regular source. |
| `too_large` | The source exceeds 65,536 bytes, including growth detected during the bounded read. |
| `empty` | The file is empty or contains only surrounding whitespace. |
| `read_failed` | Another open, descriptor inspection, or read failure prevents a usable value. |

An open failure that prevents target inspection can use `read_failed`. For example, a socket open can fail before a descriptor exists. Every source reason still stops before provider authentication.

The printable error and exposed chain contain no file contents or raw provider/OS payload. Service UUID and operation belong to the structured event, not a secret-bearing error message.

Source errors do not wrap `ErrRefreshFailed` or become `RefreshRejectedError`. Provider rejection keeps its existing category and responses.

## Events

Use the existing request-context-aware `slog.Logger`. Event identifiers and values are fixed in [contracts/outbound-authentication.md](./contracts/outbound-authentication.md).

| Event | Required fields | Meaning |
|---|---|---|
| Filesystem use | `event`, `service_id`, `operation`, `outcome` | The broker selected usable required credentials and validated identity. It does not claim provider acceptance. |
| Source error | `event`, `service_id`, `operation`, `outcome`, `reason` | The broker stopped before provider authentication. |
| Provider rejection | Service UUID, operation, rejection outcome/reason | A provider rejected a request with a usable selected credential. This is not a source error. |

Paths and values are not necessary event fields. Do not log a secret-bearing form, Basic header, assertion, code, or provider-controlled error description.

## Selection relationships

| Service state | Exact binding | Source choice |
|---|---|---|
| Public client | Present or absent | Existing secretless behavior, no file access. |
| CIMD confidential | Present or absent | Existing signed assertion, no file access. |
| Google flavor | Present or absent | Existing Google-flavor credential behavior, no file access. |
| Eligible confidential, source `stored` | Present or absent | Stored client ID and secret. No file access. |
| Eligible confidential, source `filesystem` | Present | Current required file values, subject to established identity, or source error. |
| Eligible confidential, source `filesystem` | Absent, including `{}` | `missing_binding`. Never infer stored mode. |
| Filesystem registration without canonical ID | Cannot match | Reject before persistence or file access. |

## State transitions

~~The broker stores no file-source state.~~ Superseded because the service persists its source choice. Each operation still obtains current required values from configuration and files. It retains no file secret or descriptor.

| Change between operations | Result of the next affected operation |
|---|---|
| Secret A becomes secret B for the same client ID | Use B on the next exchange/refresh. No registration update, restart, or rotation write. |
| Client ID A becomes B | New initiation uses B. A-associated codes/sessions stop before authentication as B and require a new connection. |
| Valid required file becomes unavailable or unusable | Return a source error. Do not use its previous value or stored credentials. |
| Unavailable file becomes valid | Use the restored current value. |
| Projected-volume symlink selects a new generation | Open and use the new target. |
| Broker instance restarts with an unusable binding | Fail closed before its first successful read. |
| Filesystem service clears its canonical ID | Reject the update without mutation or file access. |
| Filesystem service changes to another valid canonical ID | Preserve filesystem mode and use only the new exact binding. A missing binding fails closed. |
| Another service receives that canonical ID | Its explicit source governs selection. A binding never selects filesystem mode by itself. |
| Service changes to an excluded mode while selecting filesystem | Reject the combination. Existing excluded-mode behavior and unused bindings remain unchanged. |
| Configuration update removes the binding, followed by restart | Preserve filesystem mode and fail with `missing_binding`. Never restore stored credentials. |

Canonical-ID reassignment does not copy a credential or mutate the mapping. Existing canonical-ID uniqueness within services prevents simultaneous owners.

### Source lifecycle and administrative representation

| Request or transition | Domain invariant | Persisted/API result |
|---|---|---|
| Create with source omitted | Select `stored`. Retain existing auth-mode validation. | Existing confidential credentials remain encrypted. Report `stored`. |
| Create as `filesystem` | Eligible shared-secret mode, valid canonical ID, and neither inline field supplied. No file access. | Store zero/absent credentials. Report `filesystem`; omit `client_id` and `client_secret`. |
| Update with source omitted | Preserve the existing source and source-specific credential rules. | An unrelated update does not change source or resolve files. |
| Explicit `stored` to `filesystem` | Neither inline field can accompany the request. Remove both old credentials in one version-checked update. | Source, credential erasure, and `credential_source_transitioned: true` commit atomically or remain unchanged. |
| Explicit `filesystem` to `stored` | Require supplied non-empty client ID and secret. Never import files. | Store explicit encrypted credentials atomically. Keep `credential_source_transitioned: true`. |
| Unknown source or excluded filesystem mode | Reject validation. | No file access or record mutation. |

Creation and backfill initialize the internal marker to false. An actual source change sets it to true in the same persisted update. No-op selections and unrelated metadata updates preserve it. A rejected update or CAS conflict preserves the previous source, credentials, and marker. Source transitions never rewrite established authorization/session identities.

Administrative input distinguishes omitted fields from supplied values. Filesystem requests reject either inline field, including explicit `null`, empty strings, or placeholders. A `null` source is not omission and is invalid.

Stored confidential reads retain the stored client ID and existing `REDACTED` secret representation. Filesystem reads never synthesize `REDACTED`. Public and CIMD absent-secret representations remain unchanged.

Administrative and consent/session metadata never resolve files or expose file paths. `api/admin/openapi.yaml` defines implemented source-aware create, PUT, get, and list behavior under Admin API `2.0.0`. Existing ETag/concurrency and unrelated PUT semantics remain unchanged.

### Authorization and session client-identity association

| Owner | Field | Representation and invariant |
|---|---|---|
| `oauth2session.OAuth2StateTokenClaims` | `UpstreamClientID *id.ClientID` | Optional JWE claim `upstream_client_id,omitempty`. Capture the effective client ID on new eligible initiation. Never put the secret or a path in state. |
| `storage.UserSession` | `UpstreamClientID *id.ClientID` | Nullable `upstream_client_id` persistence field, excluded from JSON with `json:"-"`. Copy the verified initiating identity after successful exchange. |
| Public session/consent DTOs | No added identity field | Preserve existing representations. Do not resolve or disclose file values. |

The existing JWE still binds principal, service UUID, PKCE verifier, redirect URI, and expiry. Client identity is an additional binding, not a replacement. A supplied empty identity claim is invalid.

Before each eligible exchange attempt or actual refresh, compare the selected client ID with the established association in either source mode. A different value produces `identity_mismatch`. Missing identity produces `identity_missing` when filesystem mode or the transition marker applies. Both stop before provider authentication and require a new connection.

Record the effective identity for new eligible stored flows as well, so source transitions cannot relabel established codes or sessions. A present association remains authoritative after a source transition. It must match the effective identity before provider authentication.

Legacy eligible contexts without an association retain behavior only for stored services with `credential_source_transitioned: false`. Filesystem mode or any prior transition requires a new connection, including after return to stored. Excluded modes remain unchanged. Never infer a legacy identity from current files, current service credentials, or provider token contents.

Session upsert, retrieval, locked refresh copies, and token updates must preserve the association in both stores. A new successful connection can replace it alongside that session's new tokens. Refresh never rewrites it to conceal a mismatch.

Keep encryption context exactly `service_id` for service/session tokens. The identity column is non-secret context, not AAD or a credential cache.

### PostgreSQL and in-memory persistence design

**Existing migration:** `036_add_oauth2_credential_source_and_client_identity.{up,down}.sql`.

The 2026-10-08 design review found 035 as the latest sequence and reserved slot 036. Implementation created both migration 036 files. PostgreSQL and in-memory adapters persist the source, monotonic history, source-specific credential absence, and nullable session identity.

#### Binding baseline

| Existing source | Relevant schema or behavior |
|---|---|
| `migrations/002_create_thirdparty_services.up.sql` | Service `client_id VARCHAR(255) NOT NULL` and `client_secret_encrypted BYTEA`. Keep their types. |
| `migrations/007_add_oauth2_flavor.up.sql` | `oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard'`. Keep the default and all existing values. |
| `migrations/029_add_canonical_ids.up.sql` | Nullable `canonical_id VARCHAR(255)` and the existing partial unique index. Preserve exact, case-sensitive ownership. |
| `migrations/031_add_token_endpoint_auth_method.up.sql` | Nullable ciphertext, nullable `token_endpoint_auth_method VARCHAR(32)`, and `chk_thirdparty_oauth2_services_client_auth`. SQL NULL represents the absent shared-secret method. |
| `migrations/035_add_cimd_private_key_jwt_authentication.up.sql` | The authentication CHECK allows shared-secret, public `none`, and CIMD `private_key_jwt` alternatives. Migration 036 replaces this CHECK under the same name. |
| `migrations/004_create_user_sessions.up.sql` | Session foreign key, `(principal, service_id)` uniqueness, indexes, encrypted tokens, and timestamp trigger remain unchanged. |
| Both provider records and repositories | PostgreSQL uses an adapter-local record and transactional version updates. Memory uses entity copies and mutex-protected canonical/resource indexes. |
| Both session repositories | Session creation has upsert semantics. PostgreSQL refresh uses `FOR UPDATE`; memory refresh uses a per-session lock and a session copy. |

Migration 035's final `=` comparison can produce UNKNOWN for invalid NULL combinations. PostgreSQL CHECK permits UNKNOWN. Migration 036 preserves valid authentication alternatives, not that loophole. Non-empty stored identities and ciphertext already belong to domain and record-validation contracts.

#### Columns and representations

| Table/record | Implemented change | Persistence invariant |
|---|---|---|
| Service `credential_source` | `TEXT NOT NULL DEFAULT 'stored'` | Only `stored` or `filesystem`. Every existing row receives `stored`, including public, CIMD, and Google services. |
| Service `credential_source_transitioned` | `BOOLEAN NOT NULL DEFAULT FALSE` | Every existing row and every new service starts false, including direct filesystem creation. An actual source change sets it true atomically. |
| Service `client_id` | Remove column-level NOT NULL. Retain `VARCHAR(255)`. | SQL NULL only for filesystem. Stored rows retain a non-empty identity, including the existing broker-derived CIMD identifier. |
| Service `client_secret_encrypted` | Retain nullable `BYTEA`. | SQL NULL for filesystem, public, and CIMD. Non-empty opaque ciphertext for stored shared-secret and Google credentials. |
| Session `upstream_client_id` | `TEXT NULL`, without a default | Legacy sessions remain NULL. Present identities cannot be empty. TEXT avoids imposing the stored-client column's length limit on file identities. |
| PostgreSQL provider record | Add source and boolean fields. Change adapter-local `ClientID` to `*string`. | Preserve SQL NULL rather than converting it to an empty stored identity. No database types enter the domain. |
| PostgreSQL session record | Add adapter-local `UpstreamClientID *string`. | Convert to/from `*id.ClientID`, preserving nil distinctly from a present value. |
| Memory provider/session records | Retain the same source, marker, absent credentials, and optional identity. | Preserve entity-copy isolation, indexes, version checks, upsert semantics, and session locks. |

Filesystem conversion uses zero `id.ClientID`, `model.NewAbsentSecret()`, and an absent domain auth method. PostgreSQL writes SQL NULL for all three credential/auth-method columns. It must not write an empty byte slice, empty auth-method string, or encrypted placeholder.

Stored conversion keeps existing public/CIMD absent-secret states and confidential encrypted-secret states. Unknown sources, mixed credentials, and corrupt source-specific records return existing storage validation errors. Reads never infer a source from NULL credentials or a configuration binding.

#### Exact UP schema and constraints

The existing UP file consists of one PL/pgSQL DO statement. The block acquires table locks before schema changes. All changes share the statement's transaction, including constraint validation. The existing migration runner must pass the complete block without semicolon splitting. No concurrent-index or no-transaction mode applies.

```sql
DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE;

    ALTER TABLE thirdparty_oauth2_services
        ADD COLUMN credential_source TEXT NOT NULL DEFAULT 'stored',
        ADD COLUMN credential_source_transitioned BOOLEAN NOT NULL DEFAULT FALSE,
        ALTER COLUMN client_id DROP NOT NULL,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_credential_source
            CHECK (credential_source IN ('stored', 'filesystem')),
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
            CHECK ((
                (
                    credential_source = 'stored'
                    AND client_id IS NOT NULL
                    AND client_id <> ''
                    AND (
                        (
                            token_endpoint_auth_method IS NULL
                            AND client_secret_encrypted IS NOT NULL
                            AND octet_length(client_secret_encrypted) > 0
                        )
                        OR (
                            token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                            AND client_secret_encrypted IS NULL
                        )
                        OR (
                            token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                            AND client_secret_encrypted IS NULL
                        )
                    )
                )
                OR (
                    credential_source = 'filesystem'
                    AND canonical_id IS NOT NULL
                    AND canonical_id COLLATE "C" ~ '^[A-Za-z0-9._-]{1,128}$'
                    AND (
                        CASE WHEN char_length(canonical_id) = 38
                            THEN substring(canonical_id FROM 2 FOR 36)
                            ELSE canonical_id
                        END
                    ) COLLATE "C" !~ '^([0-9A-Fa-f]{32}|[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12})$'
                    AND client_id IS NULL
                    AND client_secret_encrypted IS NULL
                    AND token_endpoint_auth_method IS NULL
                    AND oauth2_flavor IN ('', 'standard', 'github')
                )
            ) IS TRUE);

    ALTER TABLE user_sessions
        ADD COLUMN upstream_client_id TEXT,
        ADD CONSTRAINT chk_user_sessions_upstream_client_id
            CHECK (upstream_client_id IS NULL OR upstream_client_id <> '');
END
$migration$;
```

The source column's NOT NULL rejects NULL before the enum CHECK can return UNKNOWN. The authentication CHECK accepts only TRUE. Each required nullable value has an explicit presence or absence condition. Null-safe auth-method comparisons prevent UNKNOWN from admitting unsupported combinations.

Filesystem requires the existing canonical grammar and rejects UUID-shaped values. `canonical.Validate()` delegates to `google/uuid.Parse`, which accepts raw 32-character hexadecimal, standard 36-character UUIDs, and 38-character wrappers. The CASE covers its wrapper behavior even when both outer characters satisfy the canonical grammar. Braced and URN forms already fail that grammar. `COLLATE "C"` keeps the regex character ranges ASCII-specific.

Filesystem permits only standard and GitHub shared-secret modes. The empty flavor is the existing zero/default-standard representation, not another authentication mode. SQL NULL, Google, and unknown flavors are invalid for filesystem. Domain validation retains existing flavor rules and the shared-secret eligibility predicate. Stored rows receive no new flavor or canonical restriction.

The default columns perform backfill without row UPDATE statements. Service IDs, canonical IDs, credential bytes, versions, timestamps, and protected resources remain unchanged. Session rows retain their tokens, timestamps, encryption context, and NULL identity. No ciphertext is decrypted, rewritten, or imported from files.

Valid pre-feature rows remain valid in every existing authentication mode. Invalid manually written rows can fail the stronger CHECK. Such an UP failure must leave the full version-035 schema and data intact. The migration cannot repair rows, invent credentials, or weaken validation to continue.

#### Atomic source transitions and both-store updates

The domain resolves omitted creation source to stored and omitted update source to the persisted source. Explicit null and unknown source values remain invalid. Request-field presence remains available through validation. Filesystem rejects either supplied inline credential field, including null, empty strings, and placeholders.

Every service creation stores a false marker. An update computes transition evidence from the persisted prior source, not request defaults or current-source inference. Source, both credential columns, marker, timestamps, and version commit together. Stored-to-filesystem writes both credentials NULL. Filesystem-to-stored requires explicit non-empty inline credentials and domain-owned encryption before persistence.

The PostgreSQL UPDATE adds these assignments to its existing transaction:

```sql
credential_source_transitioned = credential_source_transitioned
    OR (credential_source IS DISTINCT FROM $new_source),
credential_source = $new_source,
client_id = $new_client_id,
client_secret_encrypted = $new_ciphertext
```

These SET expressions read the prior row. No update accepts a caller-supplied false marker as a replacement for persisted true evidence. The existing version predicate guards source-dependent changes against stale validation. A source change never needs a second marker UPDATE.

All provider projections, inserts, scans, conversions, and RETURNING values include the source and marker. Successful updates return the committed marker and existing record metadata. Failed validation, stale CAS, constraint failure, resource conflict, or transaction failure changes none of the persisted state. The existing protected-resource transaction and nil-versus-non-nil version semantics remain unchanged.

Memory computes the same monotonic marker under its existing provider mutex: `oldMarker || oldSource != newSource`. It validates the complete candidate before replacing the record or indexes. Creation, record copies, get/list, resource mutations, and metadata updates retain both fields.

No-op source selection and metadata updates preserve the marker exactly. The sequence stored/false → filesystem/true → stored/true never returns to false. Filesystem/false creation remains false until its first actual transition. File use, rotation, binding removal, and broker restart never change a service record.

A row CHECK cannot compare previous and next versions. The domain and atomic repository update enforce marker irreversibility. No transition-history table or database trigger is necessary. Both-store regression contracts must prove that no supported write path resets the marker.

#### Session identity round trips

`storage.UserSession.UpstreamClientID *id.ClientID` remains separate from service credentials and uses `json:"-"`. Nil represents legacy or excluded-flow absence. A present empty value is invalid. New eligible sessions use the verified initiating identity from the JWE, in both source modes.

PostgreSQL session INSERT and `ON CONFLICT` copy the association with the new connection's tokens. Record scans and every retrieval/list projection preserve it. Locked refresh reads it through the existing `FOR UPDATE` record. Refresh token UPDATE leaves the identity column unchanged. Returned refresh sessions retain the same association.

Memory upsert replaces the association only with a successful connection's new tokens. Its locked refresh copy retains the established identity. Copies of the optional identity must not share a mutable pointer with callers or a failed refresh candidate. Refresh never assigns the currently selected credential ID to an existing association.

Neither migration, service transition, refresh, nor adapter conversion infers a legacy identity. Current files, stored service credentials, and provider tokens cannot supply that missing history. Recorded identities remain binding in either eligible source mode. Missing identity is usable only for stored/false legacy contexts.

Service-secret and session-token encryption remain domain-owned under ADR 012. The AAD remains exactly `service_id` under ADR 008. Migration 004's historical broader AAD comments do not change this binding decision. The new source, marker, and identity are not AAD. The migration never rewrites existing encryption-context JSON or ciphertext.

#### Exact guarded DOWN

DOWN targets the exact version-035 schema, not version 031. CIMD remains a valid stored alternative. The existing DOWN file contains one DO statement with the guard and all DDL. The same table locks prevent a concurrent source change between the guard and column removal.

```sql
DO $migration$
BEGIN
    LOCK TABLE thirdparty_oauth2_services, user_sessions
        IN ACCESS EXCLUSIVE MODE;

    IF EXISTS (
        SELECT 1
        FROM thirdparty_oauth2_services
        WHERE credential_source IS DISTINCT FROM 'stored'
            OR credential_source_transitioned IS DISTINCT FROM FALSE
            OR client_id IS NULL
            OR client_id = ''
            OR (
                (token_endpoint_auth_method IS NULL
                    AND client_secret_encrypted IS NOT NULL
                    AND octet_length(client_secret_encrypted) > 0)
                OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none'
                    AND client_secret_encrypted IS NULL)
                OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'private_key_jwt'
                    AND client_secret_encrypted IS NULL)
            ) IS NOT TRUE
    ) THEN
        RAISE EXCEPTION 'cannot revert OAuth2 credential sources while incompatible services or source-transition evidence exist';
    END IF;

    ALTER TABLE thirdparty_oauth2_services
        DROP CONSTRAINT chk_thirdparty_oauth2_services_client_auth,
        DROP CONSTRAINT chk_thirdparty_oauth2_services_credential_source,
        ALTER COLUMN client_id SET NOT NULL,
        ADD CONSTRAINT chk_thirdparty_oauth2_services_client_auth
            CHECK (
                (token_endpoint_auth_method IS NULL AND client_secret_encrypted IS NOT NULL)
                OR (token_endpoint_auth_method IS NOT DISTINCT FROM 'none' AND client_secret_encrypted IS NULL)
                OR (token_endpoint_auth_method = 'private_key_jwt' AND client_secret_encrypted IS NULL)
            ),
        DROP COLUMN credential_source,
        DROP COLUMN credential_source_transitioned;

    ALTER TABLE user_sessions
        DROP CONSTRAINT chk_user_sessions_upstream_client_id,
        DROP COLUMN upstream_client_id;
END
$migration$;
```

The guard refuses every filesystem row, including a newly created row with a false marker. It also refuses every true marker, including services that returned to stored. This refusal does not depend on currently visible sessions. Legacy JWE authorizations can exist outside the database. Removing evidence must not revive their missing-identity exception.

DOWN cannot delete services or sessions, fabricate credentials, coerce sources, or reset markers. Reconnects and a source round trip do not remove the guard. Compatible DOWN requires only valid, never-transitioned stored services. It restores the original migration-035 CHECK verbatim and the original client-ID NOT NULL.

Compatible DOWN deliberately discards the non-secret session identity column. Existing credentials, tokens, keys, relationships, timestamps, and versions remain unchanged. Reapply creates stored/false services and NULL session associations again. It does not restore discarded associations or infer replacements. Operators must understand this identity-information loss before a compatible downgrade.

#### Integrity, migration metadata, and required regression evidence

The production harness is `tests/integration/migrations/framework.go`, with the PostgreSQL go-migrate driver. The driver sends a complete migration body by default. Existing DO migrations already require that behavior. Semicolon-splitting `x-multi-statement` is incompatible with these blocks and must remain disabled.

SQL block failure rolls back all schema/data changes. Migration metadata has a separate lifecycle: go-migrate records the target version as dirty before executing SQL. A refused 036→035 DOWN can therefore report version 35/dirty while the actual schema remains 036. That metadata does not prove that DOWN succeeded.

If DOWN refuses, recovery requires these steps:

1. Stop the migration attempt and prevent another schema migration.
2. Inspect the actual columns, nullability, defaults, and constraint definitions for version 036.
3. Compare service credentials, markers, sessions, ciphertext, metadata, indexes, and relationships with the pre-attempt snapshot.
4. If the version-036 schema and data are intact, clear dirty metadata with `migrate force 36`.
5. Retain version 036 while filesystem rows or transition evidence remain.

If inspection finds a partial or different schema, do not force a version. Restore or repair the schema from verified migration evidence first. A failed metadata update after successful SQL requires recovery to the actual schema version, not the attempted version. A refused UP requires intact version-035 evidence before recovery to 35. No blind `force 35`, marker reset, or source conversion can make refused DOWN safe.

T032/T036/T056 must supply real migration and both-store regression evidence:

| Required case | Required observation |
|---|---|
| Legacy UP: shared-secret, public, CIMD, Google, and existing sessions | Stored/false backfill, NULL session identity, and byte-for-byte preserved credentials/tokens. IDs, canonical IDs, timestamps, versions, indexes, and relationships remain intact. |
| Source-aware constraints | Valid stored and filesystem rows pass. Unknown/NULL source, NULL marker, invalid canonical IDs, UUID forms, mixed or empty inline credentials, invalid auth methods, and excluded filesystem flavors fail. |
| Both-store creation and transitions | False creation, true actual transition, encrypted reverse credentials, marker persistence through source round trips, no-op selections, metadata/resource writes, and reloads. No file import occurs. |
| Rejected updates and stale CAS | Source, credential bytes, marker, indexes, metadata, and established session identity remain unchanged together. |
| Session storage and refresh | Nullable legacy identity and present eligible identities survive upsert, retrieval/list, locked refresh, token updates, and isolated copies. Neither source transition rewrites them. |
| Compatible UP/DOWN/reapply | Never-transitioned stored authentication remains valid. Only the documented identity association is lost. Reapply does not infer identities. |
| Incompatible DOWN | Filesystem/false and stored/true cases both refuse. Full version-036 schema/data remains intact, including all credentials, evidence, and session identities. |
| Migration failure recovery | Failed SQL preserves the prior complete schema. Dirty target metadata is distinguished from the actual schema before a verified force operation. |

T011 defined these regression obligations before persistence implementation. Migration 036 and both-store coverage now exist. Completed selected package and story runs appear in the [quickstart evidence](./quickstart.md#status-and-prerequisites). The full final gate remains pending.

### Design approval and implementation status

On 2026-10-08, the user explicitly selected “Accept ADR 038” and “Major contract bump to 2.0.0”. ADR 038 is Accepted. Admin API `2.0.0` uses a clean cutover on the existing `/api/services` routes. No compatibility endpoint, shim, or unrelated endpoint change applies.

Published `v0.1.81` contains Admin API `1.0.0`, with required response `client_id`. Filesystem omission is a breaking change. T010 defined source-aware schemas and breaking-change documentation before behavior changes. Admin API `2.0.0` and affected repository consumers are implemented but unreleased.

The implemented fields and constraints follow the refined requirements and accepted ADR. No unresolved specification question remains. Historical semantic-red acceptance evidence and the test-first Helm contract preceded production behavior. All 50 acceptance cases passed in individual story runs. This does not claim a completed full final gate.

Provider publication timing and old/new validity overlap remain unverified deployment prerequisites. They are not broker fallback or continuity guarantees.


## Architecture glossary additions

`ARCHITECTURE.md` defines these implemented terms:

- ~~**Client-secret file binding**: An operator-configured association from an exact service canonical ID to one absolute path.~~ Superseded by paired filesystem credentials.
- ~~**Operation secret**: One current secret for outbound authentication.~~ Superseded by operation credentials at three boundaries.
- ~~**Override-source error**: A failure for a binding-selected override.~~ Superseded by mandatory explicit source selection.

- **Credential source**: The service-owned choice of stored or filesystem credentials, with irreversible internal source-transition evidence.
- **Filesystem credential binding**: An exact canonical-ID association to two absolute file paths.
- **Operation credentials**: A coherent normalized pair from one validated acquisition. Initiation reads only the client ID. Neither file secret nor descriptor persists.
- **Client-identity association**: The established non-secret identity for an eligible authorization/session. Both modes enforce it after transitions. Legacy absence is usable only for never-transitioned stored services.
- **Credential-source error**: A safe local failure that stops mandatory-source use before provider authentication.
