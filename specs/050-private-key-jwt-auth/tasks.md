# Tasks: `private_key_jwt` Client Authentication for CIMD Clients

**Input**: Design documents from `specs/050-private-key-jwt-auth/`

**Prerequisites**: [plan.md](./plan.md), [spec.md](./spec.md), [research.md](./research.md), [data-model.md](./data-model.md), [contracts/](./contracts/), [quickstart.md](./quickstart.md)

**Tests**: Mandatory (Constitution Principles VIII and XIII). Every implementation task is preceded by a test task that is written first and must fail first. All E2E tests are written and verified red in Phase 2f, before any implementation.

**Organization**: Tasks are grouped by user story. Phases 0 to 2.5 are blocking; Phases 3 to 7 follow spec priority; Phase N is mandatory compliance.

## Conventions

- Format: `- [ ] T### [P?] [US?] Description with file path`. `[P]` means a different file with no dependency on an incomplete task.
- All paths are relative to the repository root. Go module path prefix `github.com/agentic-identity-broker/agentic-identity-broker/` is omitted.
- Run commands with `just`: `just check` (fmt, vet, lint), `just test` (fast unit tests), `just test-e2e` (includes `just test-e2e-docker`, the container-dependent scenarios), `just test-integration`, `just test-integration-infra` (Docker or Podman).
- Phase 2e (frontend) and Phase 2.7 (entity boilerplate) are **skipped**: there is no UI change, and the only new entity is one small repository (see plan.md, Implementation Phase Overview).
- Spec scenario IDs: `US1-S1` means User Story 1, acceptance scenario 1 in [spec.md](./spec.md).
- Audit and error-category names come from [data-model.md](./data-model.md), "Domain events".

---

## Phase 0: Pre-implementation Refactoring

**Purpose**: Behaviour-preserving changes, submitted as a separate PR. All existing tests pass with signature edits only.

- [ ] T001 Add `type ClientCredentials struct { Secret, AssertionType, Assertion string }` to `internal/ports/oauth2.go` and change `TokenMintingStrategy.HandleAuthorizationCodeExchange(ctx, clientID, creds ClientCredentials, code, redirectURI, codeVerifier)` and `HandleRefreshToken(ctx, clientID, creds ClientCredentials, refreshToken, scope)` to take it in place of `clientSecret string`.
- [ ] T002 Update `Provider.HandleAuthorizationCodeExchange` and `Provider.HandleRefreshToken` in `internal/domain/oauth2server/provider.go` to the new signatures, using `creds.Secret` wherever `clientSecret` was used. Update every caller and fake in `internal/domain/oauth2server/provider_test.go`, `refresh_consent_test.go` and `test_helpers_test.go`. Depends on T001.
- [ ] T003 [P] Update `internal/app/local_strategy.go` and `internal/adapters/http/enduser/token_grant_strategy.go` (`localGrantStrategy.HandleTokenGrant`, which reads `formData.Get("client_secret")`) to build `ports.ClientCredentials{Secret: ...}`. Update fakes implementing `TokenMintingStrategy` in `internal/adapters/http/enduser/oauth2_token_test.go` (find others with `grep -rn "HandleRefreshToken" --include=*_test.go`). Depends on T001.
- [ ] T004 [P] In `internal/domain/oauth2server/session_cleanup.go` replace the fixed-size `[3]sessionCleanupRepository` array with a slice built inside `NewSessionCleanup`. Keep the constructor signature and behaviour. `session_cleanup_test.go` must pass unchanged.
- [ ] T005 Run `just check` and `just test`. Expected: green with no test-logic changes, only the signature edits from T002 and T003.

**Checkpoint**: Refactoring merged or isolated; behaviour unchanged.

---

## Phase 1: Setup

**Purpose**: Branch hygiene, ADR draft and configuration skeleton. No behaviour yet.

- [ ] T006 Confirm the earlier draft is not carried over. `ls specs/050-cimd-private-key-jwt adrs/040-inbound-cimd-client-authentication.md` must report both missing. Do not push on your own.
- [ ] T007 [P] Draft `adrs/040-inbound-cimd-private-key-jwt.md` with status **Proposed** and sections Context, Decision, Consequences, Status. Decisions to record, taken from [research.md](./research.md): D1 verifier built on `jwx` v4 outside fosite (with the comparison table); D3 order of checks and when `jti` is spent; D4 replay store on the pool, not the grant transaction; D5 persisted `client_auth_method` and the stricter-of-two rule; D8 and D9 fixed profile (RS256 and ES256, five-minute lifetime, 30-second skew, mandatory `kid` and `jti`); D11 dependency failures return `server_error`. State explicitly that it supersedes spec 028 FR-022 and the public-only edge case for `private_key_jwt` only (SR-009 of spec 028 still holds: secret-based methods stay rejected), and which statements of ADR 015 no longer hold. ADR 008 (token-exchange JWKS adapter pattern, `jwk.Cache`) is Accepted and merged, so ADR 040 MUST address it explicitly: record why client key sets are not fetched through that pattern (reuse of the SSRF-hardened CIMD fetcher, per-client refetch interval on unknown `kid`, fail-closed dependency errors; research D6 and D7), and mark ADR 040 as superseding ADR 008 in part wherever the two differ. Do **not** edit ADR 015 or ADR 008 (merged and binding).
- [ ] T008 Write failing tests in `internal/ports/config_test.go` for new `CIMDConfig.ClientAuthentication`: defaults `KeySetCacheTTL=1h`, `KeySetMinRefetchInterval=5m`, `KeySetMaxBytes=65536`, `KeySetFetchTimeout=5s`; validation rejects zero or negative values for each, and rejects `KeySetCacheTTL` above 24h.
- [ ] T009 Add `CIMDClientAuthenticationConfig` (the four fields above; mapstructure keys `key_set_cache_ttl`, `key_set_min_refetch_interval`, `key_set_max_bytes`, `key_set_fetch_timeout`) in `internal/ports/config.go`, nested under `CIMDConfig` as `ClientAuthentication` (`mapstructure:"client_authentication"`). Set defaults in `DefaultCIMDConfig()` (L369) and validate next to `validateCIMDCache` (L597). Copies of CIMD config in `LocalOAuth2Config` and `HybridOAuth2Config` (L637, L662) must carry the new struct. Validation also rejects `KeySetCacheTTL` above 24h. Depends on T008.
- [ ] T010 [P] Write failing cases then bind defaults and environment variables in `internal/config/loader.go` beside the CIMD block (L258-275), using `configutil.Key(...)`: `IDENTITY_BROKER_CIMD_CLIENT_AUTHENTICATION_KEY_SET_CACHE_TTL`, `…_KEY_SET_MIN_REFETCH_INTERVAL`, `…_KEY_SET_MAX_BYTES`, `…_KEY_SET_FETCH_TIMEOUT`. Add cases to `internal/config/loader_test.go`. Depends on T009.

---

## Phase 2: Design Preconditions (blocking)

**Purpose**: Domain, configuration, API, database and E2E design complete before implementation. No code under `internal/` changes behaviour until this phase ends.

### Phase 2a: Domain model and glossary

The model is complete in [data-model.md](./data-model.md). Remaining work:

- [ ] T011 [P] Add three terms to the `ARCHITECTURE.md` glossary beside **ClientAssertion Trust Anchor** (about L1555), using the definitions in data-model.md "Glossary additions": **CIMD Client Assertion**, **Client Key Set**, **Assertion Replay Record**. State that CIMD Client Assertion is distinct from the privileged-client `ClientAssertion` of the token-exchange trust anchor.

### Phase 2b: Configuration design

- [ ] T012 [P] Add a documented `client_authentication` block with the four keys and defaults to `examples/config/cimd.yaml`, and correct the stale "draft-ietf-oauth-client-id-metadata-document-01" citation to `-02`.
- [ ] T013 [P] Reference the new keys in `examples/config/README.md` where CIMD examples are listed.
- [ ] T014 [P] Helm: add `cimd.clientAuthentication` (`keySetCacheTtl: 1h`, `keySetMinRefetchInterval: 5m`, `keySetMaxBytes: 65536`, `keySetFetchTimeout: 5s`, each with a `# --` doc comment) after `clientNameBlocklist` in `charts/agentic-identity-broker/values.yaml` (about L474-492); extend the `cimd` schema in `values.schema.json` (about L793); render `client_authentication` inside the `with .Values.broker.oauth2AuthorizationServer.cimd` block of `templates/configmap.yaml` (about L175); add the rows to `charts/agentic-identity-broker/README.md`; add a rendering assertion to `tests/integration/helm_chart_test.go`. Document the 24 h maximum for `keySetCacheTtl` in the values comment and schema description.
- [ ] T015 [P] Add a `### CIMD` settings section to `docs/configuration.md` covering every `oauth2_authorization_server.cimd.*` key (existing ones too, since none are documented today) in the existing table format (Parameter, Type, Default, Required, Env var, Description), plus a YAML example. The existing `### CIMD SSRF protection` subsection (about L920) stays. State the 24 h maximum for `key_set_cache_ttl`.

### Phase 2c: API design

- [ ] T016 Apply [contracts/token-endpoint.md](./contracts/token-endpoint.md) to `api/enduser/openapi.yaml`: extend the `POST /oauth2/token` description (L1378) with a "CIMD client authentication (`private_key_jwt`)" subsection; add `refresh_token` to the `grant_type` enum (about L3846-3850); update the `client_assertion_type` and `client_assertion` descriptions (about L3857-3868) so they no longer read "required for token-exchange grant type" only; document `invalid_request` (400), `invalid_client` (401) and `server_error` (5xx) for these cases.
- [ ] T017 Apply [contracts/authorization-server-metadata.md](./contracts/authorization-server-metadata.md) to `api/enduser/openapi.yaml`: description and example for `token_endpoint_auth_methods_supported` and `token_endpoint_auth_signing_alg_values_supported` in `OAuth2MetadataResponse` (about L3318-3330) and the metadata example response (about L2455-2475).
- [ ] T018 Apply [contracts/authorize-endpoint.md](./contracts/authorize-endpoint.md) to `api/enduser/openapi.yaml` for `GET /oauth2/authorize` (L1246): add the CIMD fetch-failure case to the error list in the operation description, document it under the `500` response with the example from the contract, and leave the `400` text for validation failures as is.
- [ ] T019 **BLOCKING**: obtain stakeholder confirmation of all three contract deltas (token endpoint, metadata, authorisation endpoint; API-003, Constitution Principle X). Record who confirmed and when in the `Status` line of each file in `contracts/` and in the PR description. No task after Phase 2 may start until this is done.

### Phase 2d: Database design

- [ ] T020 [P] Create `migrations/038_create_client_assertion_replays.up.sql` and `.down.sql` exactly as in data-model.md: table `client_assertion_replays` with `client_id TEXT NOT NULL`, `jti TEXT NOT NULL`, `expires_at TIMESTAMPTZ NOT NULL`, `created_at TIMESTAMPTZ NOT NULL DEFAULT now()`, `PRIMARY KEY (client_id, jti)`, and index `idx_client_assertion_replays_expires_at` on `expires_at`. Down drops the index and the table.
- [ ] T021 [P] Create `migrations/039_add_refresh_token_client_auth_method.up.sql` and `.down.sql`: `ALTER TABLE refresh_token_sessions ADD COLUMN client_auth_method TEXT NULL` plus `ADD CONSTRAINT chk_refresh_token_sessions_client_auth_method CHECK (client_auth_method IS NULL OR client_auth_method = 'private_key_jwt') NOT VALID`, so adding it does not scan the table. Do not run `VALIDATE CONSTRAINT`: every existing row is `NULL` by construction, and the constraint is enforced for all new and updated rows. Down drops the constraint and the column. Nullable, no default: existing rows stay `NULL`, which is correct for grants issued before this feature.
- [ ] T022 Write failing migration tests in `tests/integration/migrations/client_assertion_replay_test.go` (build tag `integration`, follow `refresh_consent_test.go`): `TestMigration038` applies, rolls back and re-applies; asserts the primary key rejects a duplicate `(client_id, jti)` and allows the same `jti` for another client. `TestMigration039` seeds `refresh_token_sessions` rows at version 38, applies, and asserts every existing row has `client_auth_method IS NULL` and no other column changed; asserts the check constraint rejects an insert or update setting any value other than `private_key_jwt` or `NULL`, and that `pg_constraint.convalidated` is false for it; rolls back and re-applies without data loss.

### Phase 2f: E2E acceptance test design (red phase)

**Constitution Reference**: Principle XIII. One `It()` per spec scenario, Ginkgo and Gomega, `Describe` then `Context` then `It`, comment on each naming the scenario (for example `// Scenario US2.3 from specs/050-private-key-jwt-auth/spec.md`). Realistic assertions only; no `XIt`, `PIt` or `Skip()` anywhere in E2E files, no "red phase" comments. A scenario that needs a container runtime carries `Label("docker")` and is selected by label (T025); it fails if the runtime is missing and is never skipped in code. Serve documents with `httptest.NewTLSServer` and route a fake host through `bootstrap.NewCIMDTestFetcher`, as `tests/e2e/cimd_flow_test.go` does. Read the expected audience from the running test server (`bootstrap/test_server.go` rewrites the public URL to `http://127.0.0.1:<port>`); never hard-code it.

- [ ] T023 [P] Create `tests/e2e/helpers/client_assertion.go`: generate RSA-2048 and P-256 keys with a `kid`; publish them as a JWKS document; sign assertions with `jwx` v4 (pattern: `internal/domain/cimdclient/assertion_signer.go`) using an options struct whose zero value is a fully valid assertion and whose fields inject one defect at a time: algorithm, omit `kid`, `iss`, `sub`, audience (single or array), lifetime, `iat`, `nbf`, omit `exp`, omit or empty `jti`, wrong assertion type, a `jku` header, an embedded `jwk` header. Add `tests/e2e/helpers/client_assertion_test.go` (pattern: `jwt_helpers_test.go`).
- [ ] T024 Add `func (b *Builder) WithCIMDKeySetFetcher(ports.CIMDFetcher) *Builder` in `internal/app/builder.go` beside `WithCIMDFetcher` (L174-181): store the field only; wiring comes in T053. Add `ServerFactory.BuildAppWithCIMDFetchers(storage, documentFetcher, keySetFetcher ports.CIMDFetcher)` to `tests/e2e/bootstrap/server_factory.go` (beside `BuildAppWithCIMDFetcher`, L156-179) and a matching `NewCIMDEndUserTestServer` variant in `tests/e2e/bootstrap/cimd.go` (L66). Passing a nil key-set fetcher must select the production fetcher, so SSRF scenarios can exercise it.
- [ ] T025 Add a container-dependent E2E target and keep the default suite runtime-free. In `justfile`: change `--label-filter="!performance"` to `--label-filter="!performance && !docker"` in `test-e2e-backend` (about L99), `test-e2e-backend-coverage` (about L110), `test-e2e-backend-watch` (about L115), the combined watch recipe (about L173) and `verify-e2e-backend-junit` (about L431); add recipe `test-e2e-docker` running `ginkgo -v --procs=1 --label-filter="docker" ./tests/e2e/`; make `test-e2e` (about L163) depend on it, so `just verify` runs it. This also moves the existing `Label("docker")` scenario in `tests/e2e/impersonation_test.go` (L722) into the new target; leave that test unchanged. If CI reports need the scenario, add a matching JUnit run to `verify-junit`.
- [ ] T026 Extend the E2E config fixture. In `tests/e2e/fixtures/config.go` (`OAuth2ConfigWithCIMD`, about L539-552) set `ClientAuthentication` to `KeySetCacheTTL` 1h, `KeySetMinRefetchInterval` 1s, `KeySetMaxBytes` 65536, `KeySetFetchTimeout` 5s, and add `OAuth2ConfigWithCIMDKeySetInterval(upstreamURL string, minRefetch time.Duration)` returning that config with the interval overridden. US4-S1 and S2 use the 1 s default; US4-S6 uses one hour so a second unknown `kid` cannot refetch. Depends on T009.
- [ ] T027 (US1) Write the baseline E2E that passes today, in `tests/e2e/cimd_private_key_jwt_connect_test.go`: a CIMD client declaring `none` completes authorize, consent and re-authorize (reuse the sequence in `cimd_flow_test.go` FL-001), exchanges the code at `/oauth2/token` with `server.PublicPOST` and no assertion, then refreshes. This is the first CIMD E2E to reach `/oauth2/token`; it pins SC-006 and covers US1-S5.
- [ ] T028 (US1) In `tests/e2e/cimd_private_key_jwt_connect_test.go` add `It()` blocks for US1-S1 (RS256 code exchange with PKCE), US1-S2 (refresh with a fresh assertion; the refresh token rotates), US1-S3 (a document declaring `private_key_jwt` is accepted at authorise time; no `invalid_client`) and US1-S4 (document also carries `token_endpoint_auth_methods_supported: ["none","private_key_jwt"]`; an exchange without assertion fails, one with a valid assertion succeeds). Use a ChatGPT-shaped document (RS256, `jwks_uri`, `authorization_code` and `refresh_token` grants). Depends on T023, T024 and T027 for the shared setup.
- [ ] T029 [P] (US2) Create `tests/e2e/cimd_private_key_jwt_downgrade_test.go` with US2-S1 (no assertion on code exchange and on refresh gives 401 `invalid_client`), US2-S2 (`DescribeTable`, one entry per rule: bad signature, `iss`, `sub`, audience, expired, lifetime over five minutes, missing `kid`, missing `jti`, wrong assertion type, `alg` differing from the document, `alg` `none`), US2-S3 (after each rejection the same code still exchanges with a valid assertion, and the refresh token still refreshes), US2-S4 (plural field lists `none` but singular says `private_key_jwt`; no assertion is rejected), US2-S5 (grant issued under `private_key_jwt`; the document is then served as `none`; refresh without assertion is rejected; with assertion succeeds), US2-S6 (assertion signed by a key that is only in a `jwk` or `jku` header is rejected), US2-S7 (a client declaring `none` sends an assertion with a code exchange and with a refresh: 401 `invalid_client`) and US2-S8 (an assertion together with `client_secret`, and separately with an `Authorization` header: 400 `invalid_request`, code and refresh token untouched) and US2-S9 (two pre-registered `private_key_jwt` clients A and B; B presents a valid assertion of its own with A's authorisation code, and separately with A's refresh token: both rejected with no tokens issued, and A then exchanges its code and refreshes with its own valid assertions). In the US2-S2 entries also capture logs (the `logBuf` pattern in `tests/e2e/impersonation_test.go`) and assert they contain neither the assertion nor the authorisation code (SC-007).
- [ ] T030 [P] (US3) Create `tests/e2e/cimd_private_key_jwt_replay_test.go` with US3-S1 (second use of one assertion gives `invalid_client`), US3-S3 (an assertion that authenticated but whose grant failed on a wrong PKCE verifier stays spent), US3-S4 (the same `jti` from two different registered clients is accepted for both), and US3-S2 labelled `Label("docker")`: build two apps over one `bootstrap.NewPostgresFixture` (pattern: `impersonation_test.go` L722-760, but without its `Skip()`: start the fixture with `Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed())` so a missing runtime fails the test loudly; the label keeps it out of the default target, see T025), fire one assertion concurrently at both and expect exactly one success.
- [ ] T031 [P] (US4) Create `tests/e2e/cimd_private_key_jwt_keys_test.go` with US4-S1 (key rotated at the `jwks_uri`; an assertion with the new `kid` succeeds after one refetch), US4-S2 (a removed key is rejected after the broker refetched), US4-S3 (a `jwks_uri` at `https://10.0.0.1/jwks.json` is refused: pass a nil key-set fetcher so the production SSRF dialer is used, as `cimd_ssrf_test.go` does for documents), US4-S4 (redirect, oversized body, malformed JSON and timeout from the `jwks_uri` each give a 5xx `server_error`, not `invalid_client`; the authorisation code stays usable afterwards), US4-S5 (key set containing RSA-1024, an `enc`-use key and an unsupported key type: none selected, `invalid_client`), US4-S6 (repeated unknown `kid` values within the minimum interval cause one fetch only; count requests at the `httptest` server). Use `fixtures.OAuth2ConfigWithCIMD` (1 s refetch interval, T026) for S1 and S2, waiting at most 2 s for the interval, and `OAuth2ConfigWithCIMDKeySetInterval(..., time.Hour)` for S6.
- [ ] T032 [P] (US5) Create `tests/e2e/cimd_private_key_jwt_metadata_test.go` with US5-S1 (local and hybrid mode with CIMD: `private_key_jwt` in `token_endpoint_auth_methods_supported` and `["RS256","ES256"]` in `token_endpoint_auth_signing_alg_values_supported`), US5-S2 (CIMD disabled, and proxy mode: neither appears) and US5-S3 (every advertised method is one the broker accepts; every algorithm in the list verifies). See `tests/e2e/oauth2_metadata_test.go` for the existing metadata test style.
- [ ] T033 Verify the red phase: `ginkgo -v ./tests/e2e/ --focus "CIMD private_key_jwt"`. Expected: the baseline from T027 passes; every other scenario fails on realistic expectations (status codes, `error` values, token presence). Confirm no `XIt`, `PIt`, `Skip()` or placeholder assertion was used. Also run `just test-e2e-docker` (Docker or Podman available) and confirm US3-S2 runs and fails on its expectations; it must not be reported as skipped.

**Checkpoint**: Contracts confirmed (T019), migrations written with failing tests (T022), E2E suite red.

---

## Phase 2.5: Foundational Infrastructure

**Purpose**: Everything the stories share: replay repository, document parsing, fetcher option, key-set service and the assertion verifier. Blocks all user stories.

### Replay repository

- [ ] T034 [P] Write failing `internal/domain/storage/client_assertion_replay_test.go`, then add `internal/domain/storage/client_assertion_replay.go`: `ClientAssertionReplay{ClientID string \`db:"client_id"\`; JTI string \`db:"jti"\`; ExpiresAt time.Time \`db:"expires_at"\`; CreatedAt time.Time \`db:"created_at"\`}` with `Validate()` rejecting an empty `ClientID`, an empty `JTI` and a zero `ExpiresAt`. No typed ID: the key is the natural pair.
- [ ] T035 Add `ClientAssertionReplayRepository` to `internal/ports/storage.go` (ISP, two methods): `Consume(ctx, clientID, jti string, expiresAt time.Time) error` (returns `*storage.StorageError{Kind: ErrorKindConflict}` when the pair already exists) and `DeleteExpired(ctx) (int, error)`. Document that `Consume` must be atomic and must not join an ambient transaction.
- [ ] T036 [P] Write failing tests, then implement `internal/adapters/storage/memory/client_assertion_replay_store.go` (mutex and map, pattern: `pkce_session_store.go`). Tests cover: first call succeeds and second conflicts; 50 goroutines on one pair give exactly one success; same `jti` for different clients both succeed; `DeleteExpired` removes only records with `expires_at` before now.
- [ ] T037 [P] Write failing integration tests (build tag `integration`, `setupMigratedAdapter`, pattern: `refresh_token_session_repo_test.go`), then implement `internal/adapters/storage/postgres/client_assertion_replay_repo.go` (pattern: `pkce_session_repo.go`). Use `adapter.db` directly and **not** `adapter.storageExecutor(ctx)`; `INSERT ... ON CONFLICT (client_id, jti) DO NOTHING` and treat `RowsAffected() == 0` as `ErrorKindConflict`; honour `timeouts.Write`; check `adapter.db == nil`; wrap every error as `StorageError`. Tests cover: concurrent consumption gives one winner; per-client scope; **the record survives a rolled-back outer transaction** (begin a transaction through the adapter, consume with that context, roll back, consume again and expect a conflict); `DeleteExpired` boundaries (verifies FR-016 and DB-004).
- [ ] T038 Wire the repository in `internal/adapters/storage/factory.go`: struct field (about L62), memory construction (about L118), postgres construction (about L155) and accessor `ClientAssertionReplays()` (about L306). Extend the factory tests. Depends on T035, T036, T037.

### Document parsing and key sources

- [ ] T039 [P] Write failing cases in `internal/domain/oauth2/cimd/document_test.go`, replacing the current "rejects private_key_jwt auth method" case (L91-95): accepts `private_key_jwt` with an HTTPS `jwks_uri`; accepts it with inline `jwks`; rejects neither source and both sources; rejects `jwks_uri` with `http`, a non-443 port, userinfo, a fragment or dot segments; rejects `token_endpoint_auth_signing_alg` of `none`, `HS256`, `PS256`; omitted alg normalises to `RS256`; rejects inline `jwks` containing any of `d`, `p`, `q`, `dp`, `dq`, `qi`, `k`; still rejects `client_secret_basic`, `client_secret_post`, `client_secret_jwt` and a `client_secret` field; ignores `token_endpoint_auth_methods_supported` (never rejects, never selects); a `none` client with `jwks` or `jwks_uri` is accepted and those fields are ignored. Extend `internal/domain/oauth2/cimd/document_fuzz_test.go` with the new fields.
- [ ] T040 Implement in `internal/domain/oauth2/cimd/document.go`: add `SigningAlg` (`token_endpoint_auth_signing_alg`), `JWKSURI` (`jwks_uri`) and `JWKS json.RawMessage` (`jwks`) to `ClientIDMetadataDocument` (L14-24); after validation expose the parsed public keys as `KeySet jwk.Set` tagged `json:"-"`. Replace the `!= "none"` rejection (L64-67) with: allow `none` and `private_key_jwt`, reject everything else. Reuse `urivalidation.ValidateCIMDClientURL` (`internal/domain/urivalidation/cimd.go`) for `jwks_uri`. Update the header comment of `ParseDocument` (L28), which says the method must be non-secret-bearing. Depends on T039.
- [ ] T041 [P] Carry the new fields through the port DTO. Write failing tests first in `internal/domain/oauth2/cimd/cache_test.go` (mutating a returned document's key set does not affect the cached entry) and `internal/domain/oauth2/client_resolver_test.go`; then add `SigningAlg`, `JWKSURI` and `KeySet jwk.Set` to `CIMDMetadataDTO` in `internal/ports/cimd.go` (L126-136), map them in `toCIMDMetadataDTO` (`internal/domain/oauth2/client_resolver.go:118-133`), and deep-copy the key set in `deepCopyDocument` (`internal/domain/oauth2/cimd/cache.go:97-106`) with `Clone()`. Depends on T040.
- [ ] T042 [P] Add an option to accept additional media types in `internal/adapters/cimd/fetcher.go`. Write failing tests first in `fetcher_test.go`: by default `application/jwk-set+json` is still rejected (behaviour unchanged for documents); with `WithAcceptedContentTypes("application/json", "application/jwk-set+json")` both are accepted; redirects are still refused and the size cap still applies. Implement as a functional option `WithAcceptedContentTypes(...string)` on `NewFetcher` and `NewFetcherInsecure` (and `NewFetcherWithClient` if it shares the check at L136-138).

### Key-set service

- [ ] T043 [P] Write failing `internal/domain/oauth2/cimd/keyset_test.go` for the core of `ClientKeySetService`: keys from an inline set involve no network call; keys from a `jwks_uri` are fetched through a fake `ports.CIMDFetcher`; eligibility filter (RSA below 2048 bits dropped; EC curves other than P-256 dropped; `use` of `enc` dropped; `key_ops` present without `verify` dropped; `use` absent accepted; private parameters dropped); cached for `KeySetCacheTTL` and refetched after it with an injected clock; fetch error, oversize and malformed JSON all return `ErrKeySetUnavailable` and leave any previous cached set untouched.
- [ ] T044 Implement `internal/domain/oauth2/cimd/keyset.go`: `NewClientKeySetService(fetcher ports.CIMDFetcher, cfg ports.CIMDClientAuthenticationConfig, clock func() time.Time, logger *slog.Logger)` and `KeysFor(ctx, doc *ports.CIMDMetadataDTO) (jwk.Set, error)`. Cache keyed by `clientID` plus `jwks_uri`, with `FetchedAt` and `LastRefetchAt` as in data-model.md "ClientKeySet". Emit the `client_key_set_refreshed` audit event (client id, source host, outcome, key count) via `slog`. Export the sentinel `ErrKeySetUnavailable`. Depends on T041, T042, T043.

### Assertion verifier

- [ ] T045 Write failing `internal/domain/oauth2server/client_assertion_test.go`, table-driven, with an injected clock and fakes for the key source and the replay repository. Cases: valid RS256 and valid ES256; header `alg` differs from the document's; `alg` `none`; HS256 signed with the public key bytes as secret (algorithm confusion); missing `kid`; unknown `kid`; wrong `client_assertion_type`; `iss` differs from `client_id`; `sub` differs; audience equal to the issuer, equal to `<issuer>/oauth2/token`, an array containing an approved value (also with unapproved extra entries, and with both approved values), an unapproved value, and an array with no approved value; missing `exp`; expired by more than 30 s but accepted at 29 s; `exp` more than five minutes ahead (boundary at five minutes plus 30 s); `exp - iat` over five minutes; with `iat` absent, lifetime measured from the injected clock; `iat` or `nbf` more than 30 s ahead; missing or empty `jti`; `jku`, `jwk` and `x5u` headers ignored; replay of the same pair rejected; same `jti` for another client accepted; **a bad signature does not call `Consume`** (assert the fake's call count); replay store error and key-set error both return the dependency error; every rejection emits `client_authentication_rejected` with one of the categories `missing`, `malformed`, `signature`, `claims`, `audience`, `expired`, `replayed`, `key_unavailable`; no log record contains the assertion, the signature or the full `jti`.
- [ ] T046 Implement `internal/domain/oauth2server/client_assertion.go`: constants `ClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"`, `MaxAssertionLifetime = 5 * time.Minute`, `ClockSkew = 30 * time.Second`; a small `ClientKeySource` interface (`KeysFor(ctx, doc) (jwk.Set, error)`); `ClientAssertionVerifier` built by `NewClientAssertionVerifier(keys ClientKeySource, replay ports.ClientAssertionReplayRepository, audiences []string, clock func() time.Time, logger *slog.Logger)`; `Verify(ctx, doc *ports.CIMDMetadataDTO, assertionType, assertion string) (*VerifiedAssertion, error)`. Order is fixed (research D3): parse; check structure and header; select the key by `kid` among eligible keys; verify the signature with the algorithm pinned from the document (default `RS256`) via `jwx` v4, never passing `jku`, `jwk` or `x5u` headers to verification; validate claims; then `Consume(clientID, jti, exp+ClockSkew)`. Return `ErrClientAuthentication` (maps to `fosite.ErrInvalidClient`) for authentication failures and `ErrClientAuthenticationUnavailable` (maps to `fosite.ErrServerError`) for key-set or replay-store failures. Emit `client_authenticated` and `client_authentication_rejected` audit events with the identifier of the `jti` as a truncated SHA-256, never the value. Depends on T034, T035, T044, T045.
- [ ] T047 Run `just check` and `just test`; run `just test-integration-infra` for the new migration and repository tests. Expected: all of T022, T034 to T046 green; E2E still red.

**Checkpoint**: Foundation ready. Verifier, key-set service, replay store and document parsing work in isolation; nothing is wired into the token endpoint yet.

---

## Phase 3: User Story 1 - ChatGPT-style client connects with `private_key_jwt` (Priority: P1) 🎯 MVP

**Goal**: A pre-registered CIMD client declaring `private_key_jwt` completes consent, exchanges its code with a signed assertion and refreshes with a fresh one.

**Independent Test**: The US1 scenarios in `tests/e2e/cimd_private_key_jwt_connect_test.go` pass.

### Tests for User Story 1 ⚠️ (write first, must fail)

- [ ] T048 [P] [US1] In `internal/domain/oauth2server/provider_test.go` add cases with a fake verifier: a `private_key_jwt` CIMD client's code exchange with a valid assertion yields tokens; its refresh yields rotated tokens; a `none` CIMD client is unchanged and the verifier is never called; the verifier runs before any fosite handler (assert the authorisation-code fake saw no read when verification fails); a `private_key_jwt` client that authenticates validly but sends a wrong `code_verifier` or a mismatched `redirect_uri` still fails the exchange exactly as any other client does (FR-012).
- [ ] T049 [P] [US1] In `internal/adapters/http/enduser/oauth2_token_test.go` add: for `authorization_code` and `refresh_token` the handler passes `client_assertion_type` and `client_assertion` form fields into `ports.ClientCredentials`; the RFC 8693 token-exchange path still reads its own `client_assertion` and is unaffected (regression).

### Implementation for User Story 1

- [ ] T050 [US1] Give `publicClient` the document: add `metadata *ports.CIMDMetadataDTO` and a method reporting whether the declared method is `private_key_jwt`, in `internal/domain/oauth2server/fosite_client.go` (L38-46); set it in `FositeStorage.GetClient` from `resolution.CIMDMetadata` (`internal/domain/oauth2server/fosite_storage.go:425-467`). Existing fosite storage tests keep passing.
- [ ] T051 [US1] In `internal/domain/oauth2server/provider.go` add an authenticator dependency (the `ClientAssertionVerifier` behind a small interface) to `NewProvider` (L43). In `HandleAuthorizationCodeExchange` (L295-376) and `HandleRefreshToken` (L379-447) call it for a `*publicClient` whose method is `private_key_jwt`, **before** the client-type `switch` and before any fosite handler. For a `*publicClient` whose method is `none` (or omitted), reject any non-empty `ClientCredentials.Assertion` or `AssertionType` with `fosite.ErrInvalidClient` in the same place, so an assertion is never silently ignored (US2-S7); this applies to both grants. Map `ErrClientAuthentication` to `fosite.ErrInvalidClient` with a fixed description (no category) and `ErrClientAuthenticationUnavailable` to `fosite.ErrServerError`. Update constructor calls in `test_helpers_test.go`. Depends on T046, T048, T050.
- [ ] T052 [US1] Populate `ClientCredentials.AssertionType` and `Assertion` from the form for the `authorization_code` and `refresh_token` grants in `localGrantStrategy.HandleTokenGrant` (`internal/adapters/http/enduser/token_grant_strategy.go`, L155-261). Do not touch the shared `oauth2_token.go` handler, `proxyTokenGrantStrategy` (which forwards the encoded form upstream unchanged) or `handleTokenExchange`. Depends on T049.
- [ ] T053 [US1] Wire it in `internal/app/builder.go` inside the existing CIMD block (about L529-563), only when `cimdEnabled` and the mode is local or hybrid: build the key-set fetcher with `adaptercimd.NewFetcher(cfg.CIMD.ClientAuthentication.KeySetFetchTimeout, KeySetMaxBytes, extraBlockedCIDRs, `WithAcceptedContentTypes("application/json", "application/jwk-set+json")` (T042))`, using the insecure variant under the same `security.skip_cimd_ssrf_validation` rule and the same telemetry `WrapTransport` as the document fetcher; use the fetcher from `WithCIMDKeySetFetcher` (T024) instead when set. Create `ClientKeySetService`, then `ClientAssertionVerifier` with audiences `[issuerURI, issuerURI + "/oauth2/token"]` derived from the configured local issuer the same way `GenerateMetadata` does (`internal/domain/oauth2/service.go:513-515`), and `b.storage.ClientAssertionReplays()`. Pass the verifier to `NewProvider` (about L874-889). Depends on T024, T038, T051.
- [ ] T054 [US1] Run the US1 scenarios: `ginkgo -v ./tests/e2e/ --focus "CIMD private_key_jwt"` limited to the connect file, plus `just test`. Expected: US1-S1 to S5 green. Do not edit the E2E logic; only fixture adjustments are allowed.

**Checkpoint**: A ChatGPT-shaped client connects. **Do not release on this checkpoint alone**: no-downgrade rules (US2) and metadata (US5) are still missing.

---

## Phase 4: User Story 2 - No downgrade, no bypass (Priority: P1)

**Goal**: A `private_key_jwt` client never obtains tokens without proving key possession; failures change no grant state; the declared strength cannot be weakened by editing the document.

**Independent Test**: The US2 scenarios in `tests/e2e/cimd_private_key_jwt_downgrade_test.go` pass.

### Tests for User Story 2 ⚠️

- [ ] T055 [P] [US2] In `internal/domain/oauth2server/provider_test.go` add: missing or invalid assertion on code exchange and on refresh returns `invalid_client`; the authorisation code stays unconsumed and the refresh session stays unused after each failure; a `none` client sending an assertion returns `invalid_client` on both grants; client B authenticating with a valid assertion of its own while presenting client A's authorisation code, or A's refresh token, is rejected, issues no tokens, and leaves A's code unconsumed and A's refresh token unrotated (US2-S9, FR-012); the required-method table from data-model.md ("Required-method decision (refresh)") with all four rows (stored `private_key_jwt` and document `none` requires an assertion; stored `NULL` and document `private_key_jwt` requires one; stored `NULL` and document `none` does not; both `private_key_jwt` requires one); an unknown refresh signature with no assertion on a `private_key_jwt` client returns the same `invalid_client` as a known one (no probing oracle); after a successful authentication, a deleted Agent or a revoked grant gives the same error as for any other client and issues no tokens; two concurrent refreshes with distinct valid assertions for one refresh token produce exactly one success, the other following the existing reuse detection.
- [ ] T056 [P] [US2] In `internal/adapters/http/enduser/oauth2_token_test.go` add, for a request that resolves as a CIMD client: only one of `client_assertion` and `client_assertion_type` gives 400 `invalid_request`; an assertion together with `client_secret` gives 400 `invalid_request`; an assertion together with an `Authorization` header gives 400 `invalid_request`; every authentication failure returns the same `error_description` regardless of cause (FR-023). Add regression cases showing the rules do **not** apply elsewhere: a proxy-mode request carrying `client_assertion` is forwarded upstream with the form unchanged, and a locally registered confidential agent sending `client_assertion` with its `client_secret` behaves as before.
- [ ] T057 [P] [US2] Persistence tests: round-trip `ClientAuthMethod` (nil and `private_key_jwt`) in `internal/adapters/storage/memory/refresh_token_session_store_test.go` and, with build tag `integration`, `internal/adapters/storage/postgres/refresh_token_session_repo_test.go` including `CreateSuccessor`; `Validate()` in `internal/domain/storage` rejects any non-nil value other than `private_key_jwt`.
- [ ] T058 [P] [US2] In `internal/domain/oauth2server/fosite_storage_test.go` add: `CreateRefreshTokenSession` persists `private_key_jwt` when the request was authenticated that way and `NULL` otherwise; `GetRefreshTokenSession` exposes the stored method; rotation carries it to the successor through the session extra claim `client_auth_method` (the route already used by `consent_grant_id` and `refresh_parent_signature`, L345-346 and L250-253).

### Implementation for User Story 2

- [ ] T059 [US2] Add `ClientAuthMethod *string \`db:"client_auth_method"\`` to `internal/domain/storage/refresh_token_session.go`; `Validate()` must enforce "when non-nil, MUST equal `private_key_jwt`". Depends on T057.
- [ ] T060 [P] [US2] Copy the field (deep-copying the pointer) in `internal/adapters/storage/memory/refresh_token_session_store.go` for `Create`, `CreateSuccessor` and `FindBySignature`. Depends on T059.
- [ ] T061 [P] [US2] Include the `client_auth_method` column in the `INSERT` and `SELECT` statements for `Create`, `CreateSuccessor` and `FindBySignature` in `internal/adapters/storage/postgres/refresh_token_session_repo.go`. Depends on T059 and migration 039 (T021).
- [ ] T062 [US2] In `internal/domain/oauth2server/provider.go` carry a context marker (pattern: `exchangeClientContextKey`, `fosite_storage.go:137`) recording that this request authenticated with `private_key_jwt`; in `fosite_storage.go` set `ClientAuthMethod` in `CreateRefreshTokenSession` (L242-289) from the marker, or from the `client_auth_method` extra claim on rotation, and surface the stored value in `GetRefreshTokenSession` (L292-349). Depends on T058, T059, T051.
- [ ] T063 [US2] In `Provider.HandleRefreshToken` read the stored session first (find it by the refresh-token signature through the refresh repository, as fosite's handler does), compute the required method as the stricter of stored and current (table in data-model.md), and require an assertion when it is `private_key_jwt`. A missing or unknown session must not change the response before authentication has been attempted: fall back to the document alone. Depends on T055, T062.
- [ ] T064 [US2] In `localGrantStrategy.HandleTokenGrant` (`internal/adapters/http/enduser/token_grant_strategy.go`) enforce the parameter rules for the `authorization_code` and `refresh_token` grants, only when the `TokenGrantResolution` it already receives (currently ignored as `_`) has `ClientType == storage.CIMDClient`: exactly-one-of the two assertion parameters, assertion with `client_secret`, assertion with an `Authorization` header all return 400 `invalid_request`. Local and proxy clients are untouched, so proxy mode, which can legitimately forward `client_assertion` upstream, does not change. In `handleMintingError` (`token_grant_strategy.go:263-294`) make sure no failure category reaches `error_description`. Rejection of an assertion from a `none` client happens in the provider (T051 for both grants; T063 adds the stored-method rule for refresh). Depends on T056.
- [ ] T065 [US2] Run the US2 scenarios (downgrade file) and `just test`; run `just test-integration-infra` for the postgres repository changes. Expected: US2-S1 to S9 green, including the log assertions (SC-007).

**Checkpoint**: US1 and US2 together form the minimum safe behaviour of the feature.

---

## Phase 5: User Story 3 - Assertions are single-use across replicas (Priority: P2)

**Goal**: An assertion is accepted at most once per client across replicas and restarts, even when the grant fails, and replay records are swept.

**Independent Test**: The US3 scenarios in `tests/e2e/cimd_private_key_jwt_replay_test.go` pass, including the Docker-labelled two-instance one.

### Tests for User Story 3 ⚠️

- [ ] T066 [P] [US3] In `internal/domain/oauth2server/session_cleanup_test.go` add: a fourth store `client_assertion_replays` is swept at startup and each minute; an error from it does not stop the other three sweeps, and the reverse (mirror the existing failure-isolation cases).
- [ ] T067 [P] [US3] In `internal/domain/oauth2server/provider_test.go` add: a failing replay store makes code exchange and refresh return `server_error` with the code unconsumed and the refresh token unrotated.

### Implementation for User Story 3

- [ ] T068 [US3] Add the replay repository as a fourth entry to `NewSessionCleanup` in `internal/domain/oauth2server/session_cleanup.go` (name `client_assertion_replays`, `deleteExpired: replays.DeleteExpired`); pass `b.storage.ClientAssertionReplays()` where the builder constructs it (`internal/app/builder.go:1196-1215`, local and hybrid only, which satisfies DB-004); update all callers in tests. Depends on T004, T066.
- [ ] T069 [US3] Run the US3 scenarios: `ginkgo -v ./tests/e2e/ --focus "CIMD private_key_jwt"` for the replay file, plus `just test-e2e-docker` (Docker or Podman required) for the two-instance scenario, and `just test-integration-infra`. Expected: US3-S1 to S4 green; replay-record retention and removal (FR-016, DB-004) is green through the repository tests from T036 and T037. If S3 fails, the verifier consumes after claims validation but the grant ordering is wrong: fix in `provider.go` (consumption must happen inside `Verify`, before fosite is called), not in the test.

**Checkpoint**: Replay protection holds across replicas, failures and restarts.

---

## Phase 6: User Story 4 - Client key rotation and hostile key sources (Priority: P2)

**Goal**: Rotation works without operator action; hostile or broken key sources fail closed and are distinguishable from bad credentials.

**Independent Test**: The US4 scenarios in `tests/e2e/cimd_private_key_jwt_keys_test.go` pass.

### Tests for User Story 4 ⚠️

- [ ] T070 [P] [US4] In `internal/domain/oauth2/cimd/keyset_test.go` add: an unknown `kid` refetches once when `now - LastRefetchAt >= KeySetMinRefetchInterval` and not otherwise; 20 concurrent unknown-`kid` calls cause exactly one fetch; a failed refetch keeps the previous set and returns `ErrKeySetUnavailable`; a key present in the previous set but removed upstream is gone after a successful refetch.
- [ ] T071 [P] [US4] In `internal/domain/oauth2server/client_assertion_test.go` add: on an unknown `kid` the verifier asks the key source to refresh once and retries; refresh failure returns the dependency error; refresh within the minimum interval returns `invalid_client`-class rejection without fetching.
- [ ] T072 [P] [US4] In `internal/domain/oauth2/client_resolver_test.go` add: a document fetch that times out, fails to connect or returns 5xx maps to `ClientIDError{Code: "server_error"}`; an SSRF-blocked address, malformed document, policy violation or 4xx keeps `invalid_client` "CIMD document validation failed"; a cold cache plus unavailable document gives `server_error`. In `internal/adapters/cimd/fetcher_test.go` add the matching typed error for unavailability. Add HTTP-level cases (these are not acceptance scenarios, so they belong in integration tests under Principle XIII) with a stub `ports.CIMDFetcher`: in `tests/integration/oauth2_authorize_test.go`, a CIMD `client_id` whose fetch returns `CIMDUnavailableError` gets a direct 500 `server_error` (no redirect), and one returning an SSRF-blocked, malformed or 4xx result still gets a direct 400 `invalid_client`; in `tests/integration/oauth2_token_test.go`, the same two outcomes at the token endpoint (500 `server_error` versus 401 `invalid_client`). Match `contracts/authorize-endpoint.md` and `contracts/token-endpoint.md`.

### Implementation for User Story 4

- [ ] T073 [US4] In `internal/domain/oauth2/cimd/keyset.go` add `RefreshForKID(ctx, doc, kid) (jwk.Set, error)`: refetch only when the minimum interval has elapsed, deduplicate concurrent callers with `golang.org/x/sync/singleflight` (already in `go.mod`), and never lower the interval below the configured value. Depends on T044, T070.
- [ ] T074 [US4] In `internal/domain/oauth2server/client_assertion.go` call `RefreshForKID` once when the `kid` is not in the current eligible set, then retry the lookup; extend the `ClientKeySource` interface. Depends on T046, T071, T073.
- [ ] T075 [US4] Add `ports.CIMDUnavailableError` (in `internal/ports/cimd.go`, beside `SSRFBlockedError` at L72-81), returned by `internal/adapters/cimd/fetcher.go` for connection failures, timeouts and 5xx responses only. In `resolveCIMD` (`internal/domain/oauth2/client_resolver.go:76-80`) return `&ports.ClientIDError{Code: "server_error", ...}` for it, keeping the `invalid_client` mapping for everything else. No existing test asserts the old `invalid_client` mapping for unavailable documents (searched `CIMD document validation failed` in `_test.go` files), so no test needs rewriting; T072 supplies the new coverage. This also changes the authorisation path; call it out in the PR (plan Risk R3). Depends on T072.
- [ ] T076 [US4] Run the US4 scenarios, `just test` and `just test-integration`. Expected: US4-S1 to S6 green and the T072 integration cases green.

**Checkpoint**: Rotation and hostile key sources behave as specified.

---

## Phase 7: User Story 5 - Operators and clients can discover the capability (Priority: P3)

**Goal**: Metadata advertises `private_key_jwt` and its algorithms in exactly the configurations that accept them.

**Independent Test**: The US5 scenarios in `tests/e2e/cimd_private_key_jwt_metadata_test.go` pass.

**Depends on**: US1 and US2 being implemented, because the broker must never advertise a method it does not enforce (FR-025).

### Tests for User Story 5 ⚠️

- [ ] T077 [P] [US5] Update the pinned expectations in `internal/domain/oauth2/service_metadata_test.go` (L28 proxy unchanged; L50 local without CIMD stays `{"client_secret_post"}`; L70 hybrid; L87 local with CIMD becomes `{"none","client_secret_post","private_key_jwt"}`) and the table at `internal/domain/oauth2/service_test.go` L933 and L950; assert the new algorithm field is `["RS256","ES256"]` with CIMD in local and hybrid and absent otherwise.
- [ ] T078 [P] [US5] Update `tests/integration/oauth2_metadata_test.go` for the same matrix.

### Implementation for User Story 5

- [ ] T079 [US5] Add `TokenEndpointAuthSigningAlgValuesSupported []string \`json:"token_endpoint_auth_signing_alg_values_supported,omitempty"\`` to `MetadataResponse` in `internal/ports/oauth2.go` (struct at L136-174, beside L153). In `GenerateMetadata` (`internal/domain/oauth2/service.go:528-536`) append `private_key_jwt` to the CIMD-enabled method list and set the algorithm list. Confirm `internal/adapters/http/enduser/oauth2_metadata.go` serialises the struct unchanged. Depends on T077.
- [ ] T080 [US5] Run the US5 scenarios, `just test` and `just test-integration`. Expected: US5-S1 to S3 green.

**Checkpoint**: All five stories pass.

---

## Phase N: Constitution Compliance & Polish

**Purpose**: Documentation, ADR acceptance, verification of every principle and the full gates.

### Design phase verification

- [ ] T081 Verify the glossary entries exist in `ARCHITECTURE.md` (Principle V), configuration examples exist in `examples/config/cimd.yaml` and are referenced in `examples/config/README.md` (Principle VII), the contract confirmation from T019 is recorded (Principle X) and `api/admin/openapi.yaml` is unchanged.
- [ ] T082 Verify the E2E suite was written and red before implementation (T033), contains one `It()` per scenario with a scenario comment, has no `XIt`, `PIt` or `Skip()` call anywhere in the new files (`grep -n "Skip(\|XIt\|PIt" tests/e2e/cimd_private_key_jwt_*`) and no "red phase" comments, and that E2E files changed since T033 only in fixtures (`tests/e2e/cimd_private_key_jwt_*`).

### Documentation and architecture

- [ ] T083 Update `ARCHITECTURE.md`: describe token-endpoint client authentication for CIMD clients (verifier, key-set cache, replay store, persisted `client_auth_method`) in the OAuth2 server modes area (§3.1.4.1 about L551-575); state the metadata advertisement rule; extend the migration and storage descriptions for tables and columns 038 and 039; note the sweep alongside `SessionCleanup` (about L571).
- [ ] T084 Set ADR `adrs/040-inbound-cimd-private-key-jwt.md` to **Accepted** (introduced on this branch, so Principle II allows same-PR acceptance) and add its row to the ADR Decision Index in `AGENTS.md`. Confirm the text and the final code agree.
- [ ] T085 [P] Add a short "superseded in part by feature 050" note under spec 028 FR-022 and the matching public-only edge case in `specs/028-cimd-support/spec.md`. Leave SR-009 and the rest of 028 as the historical record (secret-based methods stay rejected).
- [ ] T086 [P] Add a client-authentication section with the worked token request from [contracts/token-endpoint.md](./contracts/token-endpoint.md) to `docs/api/enduser-api.md`.
- [ ] T087 [P] Update `internal/ports/AGENTS.md` (port catalogue: `ClientAssertionReplayRepository`, new `CIMDUnavailableError`, `ClientCredentials`) and `internal/adapters/AGENTS.md` (replay stores, key-set fetcher instance). Keep `internal/ports/agents_doc_test.go` passing.
- [ ] T088 Verify `api/enduser/openapi.yaml` matches the implemented behaviour exactly, including every status and error code in [contracts/token-endpoint.md](./contracts/token-endpoint.md) and [contracts/authorize-endpoint.md](./contracts/authorize-endpoint.md) (Principle X).

### Configuration, database and security

- [ ] T089 Verify the Helm chart renders the new keys and `just` chart tests pass (Principle VII); confirm the standalone ExtProc binary is unaffected.
- [ ] T090 Compare migrations with other open work before merge. If `main` has taken 038 or 039, or 049 landed first, renumber `038` and `039` following the precedent recorded in `AGENTS.md` (feature 029, 036 to 037), update every reference (`data-model.md`, plan, tests, ADR) and re-run T022. Review the `refresh_token_sessions` column against 049's changes to the same table.
- [ ] T091 Verify migrations apply, roll back and repeat (T022), every PostgreSQL repository method has an integration test (T037, T061), and `just test-integration-infra` is green.
- [ ] T092 Security review against the spec: enforcement cannot be disabled (SR-001); only `jwx` v4 and Go `crypto/*` are used (SR-002); all failure paths fail closed (SR-003); grep the new code and a captured E2E log for assertion strings, tokens, codes and key material (SR-004, SC-007); audit events exist for success, rejection and key-set refresh (SR-005); verification algorithm comes only from the document (SR-006); a document edit cannot weaken a live grant (SR-007).
- [ ] T093 Verify hexagonal boundaries and wiring: domain imports no adapter or app package; all construction is in `internal/app/builder.go`; routing receives pre-wired dependencies (Principles VI and XII).

### Final gates

- [ ] T094 Run `just check`, `just test`, `just test-e2e` (which includes `just test-e2e-docker`), `just test-integration`, `just test-integration-infra` (Docker or Podman required) and `just verify`. All green.
- [ ] T095 Run the validation in [quickstart.md](./quickstart.md), sections 1 to 6. Section 4, step 6 is manual: confirm the content type that ChatGPT's real `jwks.json` is served with (`application/json` or `application/jwk-set+json`) and record the result in the PR (plan Risk R4).
- [ ] T096 Prepare the PR: conventional commit titles in British English, description from the repository PR template (ask for one if absent), links to issue #220, the API confirmation record, the behaviour change to dependency-failure errors (T075) and the force-push decision from T006.

---

## Dependencies & Execution Order

### Phase dependencies

- **Phase 0**: no dependencies; separate PR; must finish before Phase 2f edits the same signatures.
- **Phase 1**: independent of Phase 0 (no shared files).
- **Phase 2**: needs Phase 1. Sub-phases 2a to 2d can run in parallel; 2f needs T023 and T024 first (T025 before the Docker-labelled scenario can run, T026 before the US4 tests). **T019 (API confirmation) blocks everything below.** E2E red verification (T033) needs the Phase 0 refactor merged and T024.
- **Phase 2.5**: needs Phase 2 complete. Inside it: T034 → T035 → T036, T037 → T038; T039 → T040 → T041; T042 independent; T043 → T044 (needs T041, T042); T045 → T046 (needs T034, T035, T044).
- **Phases 3 to 7**: need Phase 2.5.
  - US1 (Phase 3) has no story dependencies.
  - US2 (Phase 4) builds on US1's `Provider` wiring (T051) and `ClientCredentials` plumbing (T052).
  - US3 (Phase 5) needs US1 wiring and migration 038.
  - US4 (Phase 6) needs US1 wiring.
  - US5 (Phase 7) needs US1 and US2 (FR-025).
- **Phase N**: after the stories you intend to ship.

### Release gate

User Story 1 is the MVP for validation, but it must not ship alone: it lacks the no-downgrade rules. The spec also requires US4 to be right before production use, and US4 delivers MUST behaviours that no earlier phase provides: refetch on an unknown `kid` (FR-019) and `server_error` instead of `invalid_client` when a client's document cannot be retrieved (FR-021, T075). The release gate is therefore **all five stories (US1 to US5)**. Priorities order the delivery and the merge sequence, not what may be released.

### Within each story

Tests first and failing; storage and domain before adapters; adapters before wiring; wiring before the E2E run.

---

## Parallel Examples

**Phase 2 (after T016 starts, before T019)**: T011, T012, T013, T014, T015, T020, T021 touch different files and run together. T016, T017 and T018 all edit `api/enduser/openapi.yaml`, so run them one after another.

**Phase 2f**: after T023 and T024 (T025 for the Docker-labelled scenario, T026 for the US4 file), run together: T028 (US1), T029 (US2), T030 (US3), T031 (US4), T032 (US5).

**Phase 2.5**: T034, T036 and T037 (after T035); T039 and T042 and T043 together.

**Phase 4 tests**: T055, T056, T057, T058 are separate files; T060 and T061 run together.

**Phase 6 tests**: T070, T071, T072 together.

---

## Implementation Strategy

### MVP first (User Story 1)

1. Phase 0 as its own PR.
2. Phases 1 and 2, including the blocking API confirmation (T019) and the red E2E suite.
3. Phase 2.5 foundation.
4. Phase 3 (US1); stop and validate with the quickstart's manual check. Do not release.

### Incremental delivery

1. Add US2, then US3: now the safe core exists (still not releasable on its own).
2. Add US4 for rotation, hardened key-source handling and dependency-failure classification (FR-019, FR-021).
3. Add US5 so ChatGPT's method negotiation sees the capability.
4. Phase N, then merge; release only when all five stories pass.

### Parallel team strategy

After T019 and Phase 2.5, three tracks can proceed: (A) US1 then US2 on the provider and token handler; (B) US3 and US4 on cleanup, key-set refetch and resolver errors; (C) US5 and documentation. Tracks A and B both edit `provider.go` and `client_assertion.go`, so merge A's T051 first.

---

## Notes

- Existing artefacts reused: `FositeStorage` transaction pattern, `SessionCleanup`, the CIMD fetcher and SSRF blocklist, `urivalidation.ValidateCIMDClientURL`, `jwx` v4 as used in `internal/domain/tokenexchange/jwt_validator.go`.
- The earlier branch draft (`specs/050-cimd-private-key-jwt/`, old ADR 040) is discarded, not merged.
- Migration and ADR numbers (038, 039, 040) are the numbers immediately after `main`; T090 re-checks them before merge.
