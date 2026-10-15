# Phase 0 Research: `private_key_jwt` for CIMD Clients

All technical-context unknowns are resolved. Evidence paths are relative to the repository root; the code was read at `main` plus the spec commit.

## D1 – Verify assertions in a domain verifier, not in fosite

**Decision:** Implement `ClientAssertionVerifier` in `internal/domain/oauth2server` on `jwx` v4. `Provider` calls it before the existing public/confidential `switch`.

**Rationale:**
- `Provider` builds `AuthorizeExplicitGrantHandler`, `RefreshTokenGrantHandler` and `pkce.Handler` directly and never calls `Fosite.AuthenticateClient` (`internal/domain/oauth2server/provider.go:43-119`). Fosite's client-authentication code is therefore not on the path. The existing `ClientAssertionJWTValid` and `SetClientAssertionJWT` stubs in `fosite_storage.go:469-480` are dead and fail closed.
- If fosite's `DefaultClientAuthenticationStrategy` were wired in, it would conflict with the spec:

| Spec requirement | Fosite v0.49.0 behaviour |
|---|---|
| FR-017 `jti` scoped per client | `ClientAssertionJWTValid(ctx, jti)` and `SetClientAssertionJWT(ctx, jti, exp)` receive no `client_id` |
| FR-014 atomic single use | check, then write (two calls) |
| FR-022 replay → `invalid_client` | `ErrJTIKnown`, code `jti_known`, HTTP 400 |
| FR-007 `kid` mandatory | `kid` optional; first matching key used when absent |
| FR-009 five-minute cap, 30 s skew | no lifetime cap, no leeway |
| FR-019 bounded refetch | one unconditional force-refetch per miss |
| FR-018 no redirects, bounded size, SSRF | default JWKS fetcher follows redirects, no size cap, no SSRF dialer |
| FR-020 key suitability | no RSA minimum, no curve check; keys without `use=sig` skipped |
| FR-008 two audiences | `Config.GetTokenURLs` can return several only via a custom `Configurator` |

- Fosite's JWKS types are go-jose v3 (indirect dependency today). The repo uses `jwx` throughout.

**Alternatives considered:**
- Wire `Fosite.AuthenticateClient` with custom `ClientAuthenticationStrategy`, `JWKSFetcherStrategy`, `Configurator` and `ClientManager` replay adapters. Rejected: overrides every behaviour above and adds a second key model.
- Use fosite's default and accept the gaps. Rejected: breaks FR-007, 009, 014, 017, 019, 020, 022.

**Evidence:** `provider.go:43-119, 295-447`; `fosite_storage.go:469-480`; fosite `client_authentication.go:68-303`, `client_authentication_jwks_strategy.go`, `errors.go:204-208`; `internal/domain/tokenexchange/jwt_validator.go:229-260` (existing `jwx` validation pattern).

## D2 – Where authentication happens

**Decision:** Authenticate inside `Provider.HandleAuthorizationCodeExchange` and `HandleRefreshToken`, after the client is resolved and before the `switch` on client type, using a new `ClientCredentials` input instead of the bare `clientSecret`. `publicClient` gains the declared method.

**Rationale:** Failure before any fosite handler runs satisfies FR-011 for free: the code is not consumed and the refresh token is not rotated. The `exchangeClientContextKey` pattern (`fosite_storage.go:137`) already carries per-request client state to storage.

**Alternatives considered:**
- Authenticate in the HTTP handler. Rejected: domain logic leaking into a handler (Principle VI), and the refresh path needs the stored session.
- A new fosite client type. Rejected: fosite does not use it for authentication here, so it would hold state with no consumer.

**Evidence:** `oauth2_token.go:46-132`; `token_grant_strategy.go:155-261`; `internal/ports/oauth2.go:174-186`.

## D3 – Order of checks and when `jti` is spent

**Decision:** (1) Parse and bind to the client; (2) select key and verify signature with the pinned algorithm; (3) validate claims; (4) only then consume `(client_id, jti)`; (5) proceed to the grant.

**Rationale:** FR-014 requires an assertion that authenticates but whose grant later fails to stay spent, so consumption cannot wait for the grant. Consuming after signature verification means nobody without the client's key can burn that client's `jti` values or fill the replay table, which keeps growth bounded by legitimate use.

**Alternatives considered:** Consume first, then verify (fosite's order). Rejected: lets an unauthenticated caller exhaust or poison replay state.

## D4 – Replay store design

**Decision:** A new ISP repository `ClientAssertionReplayRepository` with `Consume(ctx, clientID, jti, expiresAt) error` returning `StorageError{ErrorKindConflict}` on reuse, and `DeleteExpired(ctx) (int, error)`. PostgreSQL: `INSERT … ON CONFLICT (client_id, jti) DO NOTHING` and check `RowsAffected()`. Uses `adapter.db`, **not** `storageExecutor(ctx)`.

**Rationale:** Atomic first use is a storage guarantee (DB-003). Existing repositories surface unique violations as `ErrorKindUnknown` except in `agent_repository.go`, so `ON CONFLICT … DO NOTHING` is more reliable than error sniffing. The refresh repository joins the ambient fosite transaction through `storageExecutor`; if the replay repository did the same, a failed grant would roll back the `jti` and allow reuse, violating US3-S3 (`postgres/transaction.go:57-62`).

**Retention:** `expires_at = assertion exp + 30 s`. Swept by the existing one-minute `SessionCleanup` (local and hybrid only), which matches DB-004.

**Alternatives considered:** Redis or in-process cache. Rejected: not durable across replicas and restarts (FR-015). A fosite-managed table: rejected with D1.

**Evidence:** `ports/storage.go:470-485` (PKCE as the model); `postgres/pkce_session_repo.go`; `session_cleanup.go:22-36`; `builder.go:1196-1215`.

## D5 – Persist the authentication method for FR-013

**Decision:** Add a nullable `client_auth_method` column to `refresh_token_sessions`, set to `private_key_jwt` when the exchange was authenticated that way and carried to the rotated successor. On refresh, the required method is the **stricter** of the stored method and the method in the current document.

**Rationale:** The CIMD cache is per replica with TTLs up to an hour, so re-reading the document cannot enforce "never weaken". Refresh creation reads the originating client, and successor rotation already carries state through `sess.GetExtraClaims()` (`consent_grant_id`, `refresh_parent_signature`); the method rides the same route.

**Rules:**
- Stored `private_key_jwt` and document `none` → authentication required.
- Stored `NULL` and document `private_key_jwt` → authentication required (the client now declares a stronger method; before this feature no client could have declared it, so a `NULL` row means the declaration changed).
- Stored `NULL` and document `none` → unchanged behaviour.
- The stored session is read first, but an unknown signature must not change the response until authentication was attempted; otherwise an unauthenticated caller could probe refresh-token signatures.

**Alternatives considered:**
- Store the method on the grant row. Rejected: a grant spans many refresh chains and clients; the refresh session is the per-token unit.
- Trust the current document only. Rejected: lets a document edit (or a poisoned cache entry on another replica) weaken a live grant.

**Evidence:** `fosite_storage.go:242-349`; `internal/domain/storage/refresh_token_session.go`; `provider.go:379-447`.

## D6 – Key retrieval reuses the CIMD fetcher, as a second instance

**Decision:** Build a second `adapters/cimd.Fetcher` with the key-set timeout and size budget. Add an option for accepted media types (`application/json` and `application/jwk-set+json`). Validate `jwks_uri` with `urivalidation.ValidateCIMDClientURL` (https, port 443 or absent, no userinfo, no fragment, no dot segments).

**Rationale:** SSRF is enforced in the dialer `Control` hook after DNS resolution, which covers every connection, and redirects are never followed (`fetcher.go:96-98, 158-173`). That meets FR-018 without new network code. Timeout and size are baked into each instance (defaults 1 s and 5 KiB), so key sets, which the spec budgets at 5 s and 64 KiB, need their own instance. The current fetcher rejects any `Content-Type` that does not start with `application/json` (`fetcher.go:136-138`), which would reject RFC 7517 `application/jwk-set+json`.

**Alternatives considered:** `jwx-go/jwkfetch` cache, already used by `jwtauth`, and the `jwk.Cache` adapter pattern of ADR 008 (Accepted, merged). Rejected for client key sets: the CIMD fetcher does not expose its `*http.Client`, and neither offers the per-client "refetch at most once per interval on unknown `kid`, fail closed" behaviour or the audited SSRF dialer. ADR 041 records this and supersedes ADR 008 in part where they differ (task T007). A fully custom HTTP client: rejected, it would duplicate and weaken the audited SSRF path.

## D7 – Key-set cache and refetch policy

**Decision:** In-process cache per client, TTL `key_set_cache_ttl`, entries keyed by `client_id` plus the `jwks_uri`. On an unknown `kid`: if the last fetch is older than `key_set_min_refetch_interval`, refetch once (singleflight per client) and retry the lookup; otherwise reject without fetching. A fetch failure fails closed and reports a dependency error.

**Rationale:** Meets FR-019 and US4-S6. An in-process cache is acceptable here because keys are not authority: a stale key set can only reject a rotated key until the next refetch (never accept a removed key beyond the TTL), and the replay store, not the cache, provides cross-replica safety. SC-004 is expressed in terms of one minimum refetch interval for exactly this reason.

**Inline `jwks`:** parsed and validated at document parse time; stored in the cached document; no network.

**Alternatives considered:** Share key sets across replicas through the database. Rejected: adds persistence for non-authoritative data.

## D8 – Key and algorithm rules

**Decision:** The algorithm comes from the document (`token_endpoint_auth_signing_alg`, default RS256) and must also be the assertion header `alg`. Allowed: RS256 and ES256. A key is eligible only if: RSA ≥ 2048 bits (RS256) or EC P-256 (ES256); `use` absent or `sig`; `key_ops`, if present, contains `verify`; matches `kid`. An assertion must have a `kid`. Public keys only; private parameters in an inline `jwks` reject the document.

**Rationale:** Pinning the algorithm removes algorithm confusion (SR-006). Treating an absent `use` as acceptable differs from fosite (which skips keys lacking `use=sig`); ChatGPT's key set may omit `use`, and RFC 7517 makes it optional.

**Alternatives considered:** Accept PS256/ES384. Rejected: out of the spec's allow-list and untested against any real client.

## D9 – Claims policy

**Decision:**
- `iss` = `sub` = `client_id` (the CIMD URL).
- `aud` is one of the issuer URI or `<issuer>/oauth2/token`, derived from broker configuration (an array is accepted if at least one entry is approved; other entries are ignored).
- `exp` required; `exp − now ≤ 5 min + 30 s`; if `iat` is present, `exp − iat ≤ 5 min`.
- `iat` and `nbf`, if present, may be at most 30 s in the future.
- `jti` non-empty; `kid` in header.

**Rationale:** The broker's own metadata derives the token endpoint the same way (`service.go:513-515`). Never reading the audience from `Host` or forwarded headers is FR-008. Five minutes matches the outbound signer's lifetime (`cimdclient/assertion_signer.go:23`).

**Evidence:** `tokenexchange/jwt_validator.go` shows the `jwt.Validate(WithIssuer, WithAudience, WithAcceptableSkew)` idiom to follow.

## D10 – Parameter handling at the token endpoint

**Decision:** Read `client_assertion_type` and `client_assertion` for authorisation-code and refresh grants of CIMD clients, in the local grant strategy. Proxy-mode requests are forwarded upstream with the form unchanged, and locally registered confidential agents are unaffected. Rules for CIMD clients:
- Exactly one of the two → `invalid_request`.
- Assertion together with `client_secret` or an `Authorization` header → `invalid_request`.
- Assertion sent by a client that declares `none` or omits the method → `invalid_client`.
- `client_id` stays mandatory in the request (`oauth2_token.go:88-92`) and must equal the assertion `iss`.
- The token-exchange grant keeps its existing, separate `client_assertion` handling (`handleTokenExchange`) and is untouched.

**Rationale:** RFC 7523 allows omitting `client_id` when an assertion is present, but the existing handler and the broker's resolution step key on it; requiring it costs ChatGPT-style clients nothing and keeps resolution unchanged.

## D11 – Dependency failures must not look like bad credentials

**Decision:** Classify errors from `cimd.Service.Resolve` as fetch/dependency failures (timeout, network, upstream 5xx, cold-cache fetch failure) versus validation failures (SSRF-blocked, malformed, policy). Dependency failures map to `server_error`; validation failures keep `invalid_client`. Key-set retrieval failure also maps to `server_error`.

**Rationale:** `resolveCIMD` flattens every error into `ClientIDError{invalid_client, "CIMD document validation failed"}` (`client_resolver.go:76-80`), which hides outages as credential errors and contradicts FR-021 and the spec's "document unavailable" edge case. The change also applies on the authorisation path, so it is documented in [contracts/authorize-endpoint.md](./contracts/authorize-endpoint.md) and confirmed with the other contract deltas; the tests that pin the old behaviour are updated and the PR calls it out (Risk R3).

## D12 – Metadata advertisement

**Decision:** In `local` and `hybrid` with CIMD enabled, `token_endpoint_auth_methods_supported = ["none","client_secret_post","private_key_jwt"]` and `token_endpoint_auth_signing_alg_values_supported = ["RS256","ES256"]`. Otherwise neither `private_key_jwt` nor the algorithm list appears. `MetadataResponse` gains the second field; the OpenAPI schema already defines it with only a generic description.

**Evidence:** `service.go:485-548`; `ports/oauth2.go:136-174`; `openapi.yaml:3318-3325`; pinned by `service_metadata_test.go:28,50,70,87`.

## D13 – Configuration

**Decision:** Four settings under `oauth2_authorization_server.cimd.client_authentication` (see plan). Fixed policy values are constants, not settings.

**Rationale:** Operators can tune network budgets but cannot weaken the security profile (SR-001). Keys follow the existing `configutil.Key` and `IDENTITY_BROKER_CIMD_*` conventions (`loader.go:258-275`).

**Note:** `docs/configuration.md` has no CIMD settings section today (only the SSRF subsection at L920-934), so this feature adds one covering all CIMD keys.

## D14 – Test infrastructure

**Decision:** Add `helpers/client_assertion.go` (key generation and assertion signing with injectable defects) and a `WithCIMDKeySetFetcher` test injection on `Builder`. Write a baseline E2E for the unauthenticated `none` code exchange first.

**Rationale:** No existing CIMD E2E test calls `/oauth2/token`; code exchange is only covered by unit tests (`provider_test.go:469-476`), so SC-006 has no E2E baseline. E2E documents are served by `httptest.NewTLSServer` and routed by a fake host through `bootstrap.NewCIMDTestFetcher`; a second fetcher for key sets needs the same routing or it would hit real DNS.

## Findings that affect scheduling, not design

- Migration `035_add_cimd_private_key_jwt_authentication` belongs to spec 046 (outbound) and is unrelated.
- After rebase, migrations 038 and 039 are occupied, so this feature uses migrations 040 and 041. The `refresh_token_sessions` change must still be reviewed against feature 049.
- ADR 040 is occupied by the protected-resource discovery decision, so this feature uses ADR 041. The earlier ADR 040 draft on the reused branch is discarded.
- `docs/api/enduser-api.md` exists and gets the client-authentication section.
