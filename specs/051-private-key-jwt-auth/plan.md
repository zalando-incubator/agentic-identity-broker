# Implementation Plan: `private_key_jwt` Client Authentication for CIMD Clients

**Branch**: `220-private_key_jwt` | **Date**: 2026-10-15 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/051-private-key-jwt-auth/spec.md`

## Summary

Let a CIMD client that declares `token_endpoint_auth_method: private_key_jwt` authenticate at `/oauth2/token` with an RFC 7523 signed assertion, for authorisation-code exchange and refresh-token grants in `local` and `hybrid` modes.

The broker verifies assertions in a new domain verifier built on `jwx` v4. It does **not** use fosite's built-in `private_key_jwt` path. The provider calls fosite grant handlers directly and never reaches fosite's client-authentication code, and that code's behaviour also conflicts with the spec (see [research.md](./research.md) D1).

Design in one paragraph: `ParseDocument` accepts `private_key_jwt` and records the key source and signing algorithm. A `ClientKeySetService` resolves the public keys, using the existing SSRF-hardened fetcher with its own timeout and size budget, a bounded cache, and a per-client refetch limit. `ClientAssertionVerifier` pins the algorithm from the document, selects the key by `kid`, validates claims, and only then spends `(client_id, jti)` in a new replay repository that is atomic, per client, and unaffected by grant rollbacks. `Provider` calls the verifier before the existing public/confidential branch, so a failure never touches the authorisation code or the refresh token. The authentication method is persisted on the refresh-token session so a client cannot weaken it by editing its own document.

## Technical Context

**Language/Version**: Go 1.27.1

**Primary Dependencies**: `github.com/lestrrat-go/jwx/v4` v4.5.0 (JWT and JWK parsing and verification), `github.com/ory/fosite` v0.49.0 (grant handlers only), chi v5, sqlx and pgx v5, Viper, Ginkgo v2, Gomega, testify. No new dependencies.

**Storage**: PostgreSQL via sqlx in production and in-memory for development and E2E. Two schema changes: a new replay table and one nullable column on `refresh_token_sessions`.

**Testing**: Go unit tests (table-driven), adapter tests, PostgreSQL integration tests via testcontainers, Ginkgo/Gomega E2E on the production `app.Builder` bootstrap.

**Target Platform**: Linux broker service; Darwin for development.

**Project Type**: Backend feature in a Go monorepo. It changes the anonymous token endpoint and the metadata endpoint. No React UI change.

**Performance Goals**: Assertion verification adds no network call on the warm path (key set cached in process). A cold key-set fetch is bounded by the fetch timeout (default 5 s). The replay check is one indexed insert.

**Constraints**:

- Fail closed on every error. No fallback to unauthenticated handling for a `private_key_jwt` client.
- Algorithms: RS256 and ES256 only; verification algorithm pinned from the document, never from the unverified header alone.
- Fixed policy, not configurable: 5-minute lifetime cap, 30-second skew, mandatory `kid` and `jti`, RSA ≥ 2048 bits, P-256 only for ES256.
- The replay record must survive grant-transaction rollback, so the repository uses the pool, not the ambient fosite transaction.
- No custom cryptography. All parsing and signature verification via `jwx` v4.

**Scale/Scope**: Tens of CIMD clients per broker, assertions on the order of one per token request. The replay table grows by one row per authenticated token request and is swept every minute.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: Entities and value objects are in [data-model.md](./data-model.md) (Client Key Set, Assertion Replay Record, Verified Assertion, authentication-method marker on the refresh-token session).
- [x] **Domain Concepts**: `CIMD Client Assertion`, `Client Key Set` and `Assertion Replay Record` will be added to the `ARCHITECTURE.md` glossary in Phase 6.
- [x] **Entity IDs**: No new UUID-keyed entity. The replay record's key is the natural pair `(client_id, jti)`, so no typed ID is added to `internal/domain/id/`.
- [x] **Configuration Design**: Four parameters under `oauth2_authorization_server.cimd.client_authentication`. YAML in the Configuration section below.
- [x] **Config Examples**: To be added to `examples/config/cimd.yaml`, plus the stale "draft-01" citation there is corrected.
- [x] **Helm Deployment Contract**: `charts/agentic-identity-broker/values.yaml` and `values.schema.json` already expose `cimd.*`; both and the chart README are updated. The standalone ExtProc binary is unaffected.
- [x] **API Design First**: Contract deltas are in [contracts/](./contracts/) and are confirmed before implementation (Phase 2 gate).
- [x] **API Documentation**: `api/enduser/openapi.yaml` (token endpoint, metadata schema). No admin API change.
- [x] **API Changes**: Stakeholder confirmation of the contract deltas is a blocking task in Phase 2 and is recorded in the PR description.
- [x] **Database Design**: Two migrations in `/migrations/` with go-migrate naming. Versions `040` and `041`, the next available numbers after the rebased `main` migrations (see Risks R1).
- [x] **E2E Acceptance Tests**: One It() per spec scenario; see mapping table.
- [x] **E2E Test Mapping**: Table below; every acceptance scenario has an E2E test. The `DescribeTable` grouping for US2-S2 and US4-S4 is justified under Complexity Tracking.
- [x] **E2E Red Phase**: Tests are written with realistic assertions before implementation.
- [x] **Frontend Playwright E2E**: N/A, no UI change.
- [x] **Frontend Screenshots**: N/A.

**Implementation Considerations**:

- [x] **Security-First**: Enforcement is always on and fails closed (SR-001, SR-003).
- [x] **Architecture Docs**: `ARCHITECTURE.md` updated in the same PR: token-endpoint client authentication, replay protection, glossary, and the metadata advertisement rule.
- [x] **ADRs**: A new ADR is required (verifier outside fosite; persisted authentication method; fixed profile). It also records the partial supersession of spec 028 FR-022, and it addresses ADR 008 (JWKS adapter pattern) explicitly: either why client key sets are not fetched through that pattern, or that ADR 041 supersedes ADR 008 in part. It takes number `041` because ADR 040 is already accepted (see R1). Because it is introduced on this branch it can be Accepted in this PR (Constitution Principle II).
- [x] **Library-First Security**: `jwx` v4 for everything cryptographic.
- [x] **Zalando Guidelines**: The token endpoint follows RFC 6749 and RFC 7523 error semantics; no new REST resources.
- [x] **End-User Docs**: `docs/api/enduser-api.md` gets a client-authentication section with a worked example.
- [x] **Migration Testing**: Apply, rollback and repeat tests for both migrations in the PostgreSQL integration suite.
- [x] **Hexagonal Architecture**: Verifier and key-set service in the domain; storage and fetching behind ports; wiring only in `internal/app/builder.go`.
- [x] **Persistence Patterns**: Small ISP repository in `internal/ports/storage.go`, memory and PostgreSQL adapters, factory accessor, `StorageError` wrapping.

**Post-design re-check (after Phase 1)**: All items still hold. No principle is violated. Replay-record retention and expiry (FR-016, DB-004) depends on elapsed time; it is a requirement verified by repository integration tests, not an acceptance scenario. The table grouping of US2-S2 and US4-S4, the test-only `WithCIMDKeySetFetcher` option and the verifier built outside fosite are recorded under Complexity Tracking. The two-replica scenario (US3-S2) runs as a PostgreSQL-backed E2E test using the harness's existing `Label("docker")` fixture.

## Project Structure

### Documentation (this feature)

```text
specs/051-private-key-jwt-auth/
├── plan.md              # This file
├── research.md          # Phase 0: decisions D1-D14 with rationale and alternatives
├── data-model.md        # Phase 1: entities, validation rules, state, schema
├── quickstart.md        # Phase 1: runnable validation scenarios
├── contracts/
│   ├── token-endpoint.md     # client_assertion parameters and error mapping
│   ├── authorization-server-metadata.md
│   └── authorize-endpoint.md   # CIMD fetch failure returns server_error
├── checklists/requirements.md
└── tasks.md             # Phase 2 output (/speckit-tasks, not created here)
```

### Source Code (repository root)

```text
internal/
├── ports/
│   ├── oauth2.go                    # MetadataResponse: +TokenEndpointAuthSigningAlgValuesSupported;
│   │                                #   TokenMintingStrategy takes ClientCredentials (Phase 0)
│   ├── cimd.go                      # CIMDMetadataDTO: +SigningAlg, +JWKSURI, +JWKS
│   ├── storage.go                   # +ClientAssertionReplayRepository; RefreshTokenSession: +ClientAuthMethod
│   └── config.go                    # CIMDConfig.ClientAuthentication
├── domain/
│   ├── oauth2/cimd/
│   │   ├── document.go              # accept private_key_jwt; key-source and alg validation
│   │   ├── cache.go                 # deep-copy the new fields
│   │   └── keyset.go                # NEW ClientKeySetService (fetch, parse, cache, refetch limit)
│   ├── oauth2/
│   │   ├── client_resolver.go       # dependency failures -> server_error, not invalid_client
│   │   └── service.go               # metadata advertisement
│   ├── oauth2server/
│   │   ├── client_assertion.go      # NEW ClientAssertionVerifier
│   │   ├── fosite_client.go         # publicClient carries the declared method
│   │   ├── provider.go              # authenticate before the client-type switch
│   │   ├── fosite_storage.go        # persist client_auth_method on refresh sessions
│   │   └── session_cleanup.go       # sweep the replay store as a fourth repository
│   └── storage/
│       ├── client_assertion_replay.go   # NEW entity
│       └── refresh_token_session.go     # +ClientAuthMethod
├── adapters/
│   ├── cimd/fetcher.go              # configurable accepted media types
│   ├── http/enduser/
│   │   └── token_grant_strategy.go  # local strategy: read client_assertion(_type), reject ambiguous input for CIMD clients
│   └── storage/
│       ├── memory/client_assertion_replay_store.go    # NEW
│       ├── postgres/client_assertion_replay_repo.go   # NEW
│       └── factory.go               # accessor
├── app/
│   ├── builder.go                   # wire key-set fetcher, service, verifier, replay repo
│   └── local_strategy.go            # adapt to ClientCredentials
migrations/
├── 038_create_client_assertion_replays.{up,down}.sql
└── 039_add_refresh_token_client_auth_method.{up,down}.sql
api/enduser/openapi.yaml             # token endpoint and metadata schema
justfile                             # backend E2E targets exclude label docker; new test-e2e-docker
adrs/041-inbound-cimd-private-key-jwt.md
docs/                                # configuration.md, api/enduser-api.md
examples/config/cimd.yaml            # new keys; correct the draft citation
charts/agentic-identity-broker/      # values.yaml, values.schema.json, README
tests/
├── e2e/
│   ├── cimd_private_key_jwt_test.go        # NEW, one It() per spec scenario
│   ├── helpers/client_assertion.go         # NEW key generation and assertion signing
│   └── bootstrap/                          # +WithCIMDKeySetFetcher injection (test-only wiring)
└── integration/                            # replay repo, migrations, two-instance replay
```

**Structure Decision**: Follow the existing hexagonal layout. Verification logic and key-set policy are domain code so they are unit-testable without HTTP. The only new adapter surface is the replay repository (memory and PostgreSQL). The key-set fetcher is a second instance of the existing CIMD fetcher adapter, configured differently, not a new adapter.

## Configuration Requirements

**Parameters** (all optional, defaults are secure; validated as positive durations or sizes at startup):

| Key | Env var | Default | Purpose |
|---|---|---|---|
| `oauth2_authorization_server.cimd.client_authentication.key_set_cache_ttl` | `IDENTITY_BROKER_CIMD_CLIENT_AUTHENTICATION_KEY_SET_CACHE_TTL` | `1h` | Maximum age of a cached client key set; values above `24h` are rejected at startup |
| `…key_set_min_refetch_interval` | `…_KEY_SET_MIN_REFETCH_INTERVAL` | `5m` | Minimum gap between refetches for one client after an unknown `kid` |
| `…key_set_max_bytes` | `…_KEY_SET_MAX_BYTES` | `65536` | Maximum key-set response size |
| `…key_set_fetch_timeout` | `…_KEY_SET_FETCH_TIMEOUT` | `5s` | Key-set fetch timeout |

```yaml
oauth2_authorization_server:
  mode: local
  cimd:
    enabled: true
    client_authentication:
      key_set_cache_ttl: 1h
      key_set_min_refetch_interval: 5m
      key_set_max_bytes: 65536
      key_set_fetch_timeout: 5s
```

Fixed by the spec and therefore not configurable: assertion lifetime cap (5 min), skew (30 s), algorithm allow-list. The SSRF blocklist stays extend-only and is shared with document fetches.

Locations: `internal/ports/config.go` (`DefaultCIMDConfig`, validation next to `validateCIMDCache`), `internal/config/loader.go` (defaults and env binding), `docs/configuration.md` (a new CIMD section, since none exists today), `examples/config/cimd.yaml`, Helm values, schema and README.

## Implementation Phase Overview

| Phase | Purpose | Included? |
|---|---|---|
| **Phase 0** | Behaviour-preserving refactors | Yes |
| **Phase 1** | Setup: ADR, config skeleton | Yes |
| **Phase 2** | Design preconditions: contract confirmation, E2E red tests, migrations designed | **Mandatory** |
| **Phase 2.7** | Entity boilerplate | Skipped |
| **Phase 2.5** | Foundational: replay repo, key-set service, verifier | Yes |
| **Phase 3+** | User stories in priority order | Yes |
| **Phase N** | Constitution compliance | **Mandatory** |

- [x] Phase 0 (refactoring): **Include.** Two changes with no behaviour change, so they can be reviewed first: (a) replace the `clientSecret string` argument of `TokenMintingStrategy` methods with a `ClientCredentials` value (`Secret`, plus empty assertion fields), threaded through `token_grant_strategy.go`, `local_strategy.go` and `Provider`; (b) turn the `[3]` array in `SessionCleanup` into a slice, so adding a store is one line. Existing tests must pass unchanged apart from signatures.
- [ ] Phase 2.7 (entity boilerplate): **Skip.** One small repository; stubs-first would add a PR without making review easier.

**Phase outline**

1. **Phase 0 refactors** (above).
2. **Phase 1 setup**: draft the ADR; add the config struct, defaults, validation and env binding (no behaviour yet).
3. **Phase 2 preconditions**:
   - Contract deltas in [contracts/](./contracts/) confirmed by the stakeholder (API-003). Blocking.
   - E2E suite written and red; a baseline E2E for the existing `none` code exchange is written first (it passes today and pins SC-006, because no CIMD E2E currently reaches `/oauth2/token`).
   - Migrations and OpenAPI edits drafted.
4. **Phase 2.5 foundation**: replay repository (port, memory, PostgreSQL, factory, integration tests, migration); key-set fetcher options; `ClientKeySetService`; `ClientAssertionVerifier` (pure, table-driven tests over the full invalid-assertion matrix); document parsing changes; `client_resolver` error classification.
5. **Phase 3 – US1 (P1)**: token endpoint reads the parameters; `Provider` authenticates; refresh and code exchange work end to end; `publicClient` carries the method.
6. **Phase 4 – US2 (P1)**: no-downgrade paths: missing assertion, ambiguity, `none` client sending an assertion, another client's code or refresh token, persisted method on refresh sessions (migration + `fosite_storage` + stricter-of-two rule), failure leaves the code and refresh token untouched.
7. **Phase 5 – US3 (P2)**: replay consumption wired into the verifier; cleanup sweep; two-instance integration test.
8. **Phase 6 – US4 (P2)**: unknown-`kid` refetch, rotation, rate limiting, hostile key sources.
9. **Phase 7 – US5 (P3)**: metadata advertisement, OpenAPI and docs.
10. **Phase N**: ARCHITECTURE.md and glossary, ADR accepted, Helm, `just check`, `just test`, `just test-e2e`, `just test-integration-all`.

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: one file per user story so stories can be written in parallel: `tests/e2e/cimd_private_key_jwt_connect_test.go` (US1 and the `none` baseline), `…_downgrade_test.go` (US2), `…_replay_test.go` (US3), `…_keys_test.go` (US4), `…_metadata_test.go` (US5).

**Framework**: Ginkgo/Gomega following [tests/e2e/README.md](../../tests/e2e/README.md) and `tests/e2e/AGENTS.md`.

**Test Organization**: Top-level `Describe("CIMD private_key_jwt client authentication")`, one `Context` per user story, one `It()` per acceptance scenario. Each `It` carries a comment naming its spec scenario. The test host serves the document and key set over `httptest.NewTLSServer`, routed through `bootstrap.NewCIMDTestFetcher` as the existing CIMD E2E tests do.

**Scenario Mapping** (line numbers filled during Phase 2f):

| Spec Scenario | E2E Test Location | Test Description |
|---|---|---|
| US1-S1 | `cimd_private_key_jwt_test.go` | `It("exchanges an authorisation code with a valid RS256 assertion")` |
| US1-S2 | same | `It("refreshes tokens with a fresh assertion")` |
| US1-S3 | same | `It("accepts a document that declares private_key_jwt")` |
| US1-S4 | same | `It("uses the singular method when the plural field is also present")` |
| US1-S5 | same | `It("keeps unauthenticated exchange working for none clients")` |
| US2-S1 | same | `It("rejects exchange and refresh without an assertion")` |
| US2-S2 | same | `DescribeTable` over signature, `iss`, `sub`, audience, `exp`, lifetime, `kid`, `jti` failures (one entry per rule, all inside this scenario's `It` set) |
| US2-S3 | same | `It("leaves the code unconsumed and the refresh token unrotated after a rejection")` |
| US2-S4 | same | `It("rejects when the plural field lists none but the singular selects private_key_jwt")` |
| US2-S5 | same | `It("keeps requiring authentication after the document downgrades to none")` |
| US2-S6 | same | `It("ignores keys supplied in the assertion header")` |
| US2-S7 | same | `It("rejects an assertion sent by a client that declares none")` |
| US2-S8 | same | `It("rejects an assertion combined with a client secret or an Authorization header as invalid_request")` |
| US2-S9 | same | `It("rejects another client's code and refresh token even with a valid assertion of its own")` |
| US3-S1 | same | `It("rejects a second use of the same assertion")` |
| US3-S2 | `…_replay_test.go`, `Label("docker")` | `It("accepts at most one of concurrent presentations across two broker instances sharing PostgreSQL")` |
| US3-S3 | same E2E file | `It("keeps an assertion spent when the grant fails on PKCE")` |
| US3-S4 | same | `It("scopes jti per client")` |
| US4-S1 | E2E file | `It("refetches on an unknown kid and accepts the rotated key")` |
| US4-S2 | same | `It("rejects a removed key after refresh")` |
| US4-S3 | same | `It("refuses a jwks_uri that resolves to a blocked address")` |
| US4-S4 | same | `It("fails closed with a server error on redirect, timeout, oversize or malformed key sets")` |
| US4-S5 | same | `It("never selects weak, non-signature or unsupported keys")` |
| US4-S6 | same | `It("rate-limits refetches for repeated unknown kids")` |
| US5-S1 | same | `It("advertises private_key_jwt and algorithms in local and hybrid mode with CIMD")` |
| US5-S2 | same | `It("does not advertise them with CIMD disabled or in proxy mode")` |
| US5-S3 | same | `It("advertises only methods the broker accepts")` |

**Red Phase Requirements**: as in the template. Tests compile and fail on realistic expectations (status codes, `error` values, token presence). No `XIt`, `PIt` or `Skip()`; no "red phase" comments.

**Test Data Strategy**:
- Fixtures: reuse `fixtures.OAuth2ConfigWithCIMD` (extended with the key-set settings and a 1 s minimum refetch interval, plus `OAuth2ConfigWithCIMDKeySetInterval` to override it for the rate-limit scenario), agents with a pre-registered client URI, and grants/principals fixtures from `tests/e2e/fixtures/`.
- New: `helpers/client_assertion.go` generates RSA 2048 and P-256 keys, publishes them as a JWK set, and signs assertions with configurable defects (wrong `aud`, expired, no `jti`, and so on).

**Bootstrap Strategy**:
- Production `app.Builder`, fresh server and storage per test. US3-S2 builds two apps on one `bootstrap.NewPostgresFixture`, as `impersonation_test.go` does for its PostgreSQL scenario. It carries `Label("docker")`, is excluded from the default backend E2E target and runs in a new `just test-e2e-docker` target that `just test-e2e` and `just verify` include. It never calls `Skip()` (Principle XIII); with no container runtime the fixture setup fails the test.
- The builder creates a second fetcher for key sets (different timeout and size budget). E2E needs a matching injection point, `WithCIMDKeySetFetcher`, wired like `WithCIMDFetcher`. Without it E2E would hit real DNS.
- The expected audience must be read from the running test server (its public URL is rewritten to `http://127.0.0.1:<port>`); never hard-code it.

**Helper Utilities**: HTTP via `PublicPOST` (the token endpoint is unauthenticated); no new matchers.

### Unit & Integration Tests

**Unit Tests** (written first):
- `internal/domain/oauth2server/client_assertion_test.go`: full table over valid and invalid assertions, algorithm confusion (`none`, HS256 using the public key as secret), `kid` handling, lifetime and skew boundaries, audience set, ordering (signature verified before the `jti` is spent).
- `internal/domain/oauth2/cimd/keyset_test.go`: TTL, refetch limit, singleflight under concurrency, key suitability filtering, fail-closed paths.
- `document_test.go`: replace the current "rejects private_key_jwt" case; add key-source and algorithm validation and the private-material rejection. `document_fuzz_test.go` gets the new fields.
- `service_metadata_test.go` and `service_test.go`: update pinned values (local `{"client_secret_post"}`, local with CIMD `{"none","client_secret_post"}`) and add the new algorithm field.
- `session_cleanup_test.go`: fourth store; failure of one store does not stop the others.
- `client_resolver_test.go`: fetch failure maps to `server_error`; validation failure stays `invalid_client`.

**Integration Tests** (infra-backed, `just test-integration-infra`):
- Replay repository on PostgreSQL: atomic first use under concurrency, per-client scope, survives a rolled-back outer transaction, `DeleteExpired` removes only elapsed records (verifies FR-016 and DB-004, which are requirements rather than acceptance scenarios because they depend on elapsed time).
- Migrations: apply, rollback, repeat for both migrations.

**Test Coverage Goals**: critical paths at 100% of branches in the verifier and key-set service; every PostgreSQL repository method covered; E2E covers every scenario in spec.md.

## Risks and Open Points

- **R1 – Numbering collisions.** After rebasing, migrations 038 and 039 and ADR 040 are already present, and feature number 050 is used by protected-resource discovery. This feature therefore uses feature 051, ADR 041, and migrations 040 and 041. The `refresh_token_sessions` column must still be reviewed against feature 049 before implementation.
- **R2 – Refresh path ordering (FR-013).** `HandleRefreshToken` authenticates before the stored session is read. The plan reads the stored session first and applies the stricter of (stored method, current document method). A missing or unknown session must not change the error returned before authentication has been attempted, to avoid a probing oracle.
- **R3 – Dependency errors in resolution.** `resolveCIMD` currently turns every document error into `invalid_client`. Changing fetch and timeout failures to `server_error` affects the authorisation path too. Existing E2E and unit tests that pin this are updated in Phase 2.5, and the change is called out in the PR.
- **R4 – JWKS content type.** The current fetcher requires `application/json…`; RFC 7517 allows `application/jwk-set+json`. The key-set fetcher accepts both. Whether ChatGPT's `jwks.json` sends either has to be confirmed against the real endpoint in the quickstart's manual check.
- **R5 – Scope of the fetcher change.** Making accepted media types an option must not change document fetching behaviour; covered by the existing fetcher tests.
- **R6 – Spec 028 and ADR 015 text.** Both contain statements this feature supersedes in part (028 FR-022, 015's "public-only" framing). 028 stays as the historical record; the new ADR carries the supersession.
- **R7 – Earlier draft on the reused branch.** `origin/220-private_key_jwt` still carries an earlier draft (`specs/050-cimd-private-key-jwt/` and `adrs/040-inbound-cimd-client-authentication.md`). The draft is known to be wrong and is discarded, not reconciled: delete those files on the branch and do not carry the ADR 040 number or content over.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
|---|---|---|
| US2-S2 (about 11 invalid-assertion rules) and US4-S4 (four key-set failure modes) are implemented as Ginkgo `DescribeTable` entries, so one scenario yields several `It()` blocks (Principle XIII asks for 1:1 scenario mapping) | Each rule is an independent failure that must be reported on its own; a table lists every defective assertion with its expected result and keeps one scenario comment per table. Every generated `It()` still maps to exactly one acceptance scenario | A single `It()` looping over the rules stops at the first broken rule and hides the others, which is the wrong trade for a security matrix |
| `Builder.WithCIMDKeySetFetcher` is a production option that only tests call (AGENTS.md Code Style: "No test-only production APIs") | The key-set fetcher is a second instance with its own timeout and size budget, so E2E cannot reuse the single injected document fetcher without losing the production SSRF dialer, which US4-S3 (blocked address) needs. It follows the precedent of `WithCIMDFetcher` (`internal/app/builder.go:178`), already documented as an injection point for test TLS certificates | Reusing `WithCIMDFetcher` for key sets would make US4-S3 untestable end to end and move it to a unit test, a second deviation from the 1:1 E2E mapping. Nil means "use the production fetcher", so no test-specific code path exists in production behaviour |
| Verifier built outside fosite (a custom component beside the library) | Fosite's path is not reachable from `Provider` and, if wired in, fails six requirements (global `jti` scope, non-atomic replay, wrong replay error, no `kid`/lifetime/skew policy, unbounded refetch, redirects followed) | Wiring `Fosite.AuthenticateClient` with custom strategies would need a parallel go-jose key model and several overrides, so it would be larger and harder to audit than a focused `jwx` verifier |
