# Research: Migrate `ory/fosite` to `authelia.com/provider/oauth2`

**Date**: 2026-11-05
**Status**: Evaluation complete. Feasible. See `plan.md` for the executable migration.

## R1 — Why move at all

- `ory/fosite` is pinned at v0.49.0 in `go.mod`. Hydra's `main` module is `github.com/ory/hydra/v2`
  and now carries `fosite/` inside that module (verified: `raw.githubusercontent.com/ory/hydra/master/fosite/fosite.go`
  exists and `go.mod` declares `module github.com/ory/hydra/v2`). Fosite development therefore
  continues as `github.com/ory/hydra/v2/fosite`, shaped by Hydra's needs and release cadence. The
  standalone `ory/fosite` module receives no new tags. I could not reach the GitHub API from this
  session to read the repository's archived flag, so treat "frozen" as the user's statement plus the
  Hydra evidence above, not as something I verified on the repository itself.
- Importing fosite from Hydra is not attractive: it drags Hydra's whole module graph into the
  broker and ties the broker to Hydra's versioning.
- `osv-scanner.toml` carries three go-jose v3 advisories (`GHSA-4w3g-jfj2-cw2q`, `GHSA-mgcc-cww5-fp2q`,
  `GHSA-vvm8-gf78-28g4`) ignored **until 2026-11-30** because go-jose v3 arrives only through fosite.
  `CONTRIBUTING.md` says to reassess "when fosite moves to go-jose v4". Fosite will not. The
  migration removes go-jose v3 from the module graph entirely (verified in the spike: no `go-jose`
  and no `ory/*` module remains after `go mod tidy`).

## R2 — What the broker actually uses from fosite

All usage is inside `internal/domain/oauth2server/` (19 non-test and 11 test files). ADR 014
"type containment" held: no other package imports fosite. The surface used:

| Area | Symbols |
|---|---|
| Handlers | `handler/oauth2.AuthorizeExplicitGrantHandler`, `ClientCredentialsGrantHandler`, `RefreshTokenGrantHandler`, `HandleHelper`, `handler/pkce.Handler` |
| Strategy interfaces implemented | `AccessTokenStrategy` (`JWXAccessTokenStrategy`), `AuthorizeCodeStrategy` (`RandomCodeStrategy`), `RefreshTokenStrategy` (`RandomRefreshTokenStrategy`) |
| Storage interfaces implemented | `CoreStorage`, `TokenRevocationStorage`, `pkce.Storage`, `storage.Transactional`, plus `GetClient`, `ClientAssertionJWTValid`, `SetClientAssertionJWT` |
| Client interface implemented | `fosite.Client` by `confidentialClient` and `publicClient` |
| Request/response types | `NewAccessRequest`, `NewAuthorizeRequest`, `NewAccessResponse`, `NewAuthorizeResponse`, `Request`, `AccessRequest`, `Arguments`, `DefaultSession`, `ExtraClaimsSession`, `TokenType` constants |
| Config | `fosite.Config{AuthorizeCodeLifespan, AccessTokenLifespan, RefreshTokenLifespan, RefreshTokenScopes, EnforcePKCE, EnablePKCEPlainChallengeMethod, ScopeStrategy}` |
| Errors | `RFC6749Error{ErrorField, DescriptionField, CodeField}`, `ErrNotFound`, `ErrInvalidClient`, `ErrInvalidGrant`, `ErrInvalidRequest`, `ErrInvalidScope`, `ErrServerError`, `ErrUnsupportedResponseType`, `ErrInvalidatedAuthorizeCode`, `ErrInactiveToken`, `ErrTokenExpired`, `ErrTokenSignatureMismatch`, `ErrJTIKnown`, `WithHint(f)`, `WithDebug(f)`, `WithWrap` |

Not used: fosite's HTTP layer, client authentication, JWT strategies, introspection, revocation
endpoints, OIDC. Client authentication (Argon2id, CIMD `private_key_jwt` under ADR 043) is the
broker's own code and stays untouched.

## R3 — authelia/oauth2-provider: shape and health

- Module `authelia.com/provider/oauth2`, Apache-2.0 (same license as fosite). Hard fork of fosite
  that keeps the handler/strategy/storage decomposition, so the headless pattern from ADR 014 maps
  one to one.
- Latest tag **v0.3.3 (2026-09-19)**. Tags in 2026: v0.2.31 (May) through v0.3.3; roughly one
  release every two to four weeks. `master` had 388 commits in the 120 days before 2026-11-05.
- Go policy (README): only the **latest Go minor** is supported, currently 1.27; the project
  "prefers security over backwards compatibility". The broker is on Go 1.27.3, so this matches today
  and must be kept in step at every toolchain bump.
- Dependencies at v0.3.3: `dgraph-io/ristretto v0.2.0`, `google/uuid`, `gorilla/mux`,
  `hashicorp/go-retryablehttp`, `pkg/errors`, `x/crypto`, `x/net`, `x/oauth2`, `x/text`. No go-jose.
  `master` has since replaced ristretto with `ristretto/v2`, dropped `gorilla/mux`, and added
  `authelia.com/provider/jose`; the next tag will carry those.
- Feature set far exceeds the broker's needs (CIMD, RFC 8693 token exchange, RFC 7523, DCR
  RFC 7591/7592, DPoP, PAR, JAR, device grant, mTLS, RAR). Only `handler/oauth2`, `handler/pkce`,
  `storage` and the root package are needed. The extra packages are not imported and do not
  compile into the binary. They are a possible future consolidation target (the broker's own CIMD
  resolver and token-exchange domain), not part of this migration.

## R4 — Interface deltas that matter (v0.3.3, with `master` notes)

Confirmed by reading the sources and by a compile spike (R5).

1. **`Client` interface**: `GetHashedSecret() []byte` is gone; `GetClientSecret() ClientSecret` and
   `GetClientSecretPlainText() ([]byte, bool, error)` are added. `master` also adds
   `GetResource() Arguments`. The broker never lets the library authenticate clients, so these can
   return `nil`/empty. `GetHashedSecret` is currently dead code on both broker client types.
2. **Strategy interfaces** add `IsOpaqueAccessToken`, `IsOpaqueAuthorizeCode`,
   `IsOpaqueRefreshToken`. Only the JWT introspector calls them; the broker returns
   `false` for JWT access tokens and `true` for the random code and refresh token.
3. **`RefreshTokenStorage`** adds `UpdateRefreshTokenSession(ctx, signature, requester)`. It is
   called only when rotation is disabled (`DisableRefreshTokenRotation` or a client opting out).
   The broker always rotates. Implement it fail-closed (return `ErrServerError`-wrapped error) so a
   future config change cannot silently skip rotation.
4. **Refresh grant, `HandleTokenEndpointRequest`**: the `scope` form parameter is now honoured
   (RFC 6749 §6): a narrower scope is granted, a scope not originally granted returns
   `invalid_scope`. Fosite v0.49 ignored it and always re-granted the original scopes.
   `Provider.HandleRefreshToken` already forwards `scope`; existing tests pass, but the E2E suite
   must confirm the broker's documented behaviour for `scope` on refresh. `ErrScopeNotGranted` for
   the `offline`/`offline_access` check became `ErrInvalidGrant`.
5. **Refresh grant, `PopulateTokenEndpointResponse`**: inside the transaction the handler re-reads
   `GetRefreshTokenSession(ctx, signature, nil)` before `RotateRefreshToken`. In the broker that
   means a second `VerifyAgentAccess` and a second client resolution per refresh (the first is in
   `HandleTokenEndpointRequest`). Functionally fine; one extra consent lookup and, for CIMD
   clients, one extra resolver call per refresh (the resolver caches).
   On v0.3.3 storage `ErrNotFound`/`ErrInactiveToken` surface as `invalid_request` with the cause
   in the debug field only (no `WithWrap`), so the broker's existing workaround in
   `HandleRefreshToken` that checks `errors.Is(err, ErrNotFound/ErrInactiveToken)` no longer fires.
   This is the **only** behavioural break found by the spike (3 tests). On `master` the handler
   itself returns `invalid_grant` for replay and revocation, so the workaround can be deleted
   once the broker is on a tag that includes that change.
6. **Auth-code grant**: `redirect_uri` binding, client-id binding, replay revocation via
   `ErrInvalidatedAuthorizeCode` + non-nil requester, and the transaction around
   invalidate/create are the same. `canIssueRefreshToken` is the same. Lifespans use
   `Truncate(time.Second)` instead of `Round(time.Second)`.
7. **Sanitation**: both libraries keep `grant_type`, `response_type`, `scope`, `client_id` plus a
   whitelist; fosite's default whitelist is `code, redirect_uri`, authelia's is `scope, redirect_uri`.
   `CreateAuthorizeCodeSession` reads `client_id`, `redirect_uri` and `code_challenge` from the
   form. `client_id` and `redirect_uri` survive in both; `code_challenge` is stripped in both (the
   PKCE handler stores it separately). No change, but set `SanitationWhiteList` explicitly to make
   the dependency visible.
8. **PKCE**: v0.3.3 behaves like fosite. `master` adds strict S256 challenge syntax at the
   authorize endpoint (43–128 chars, base64url of exactly 32 bytes) and a constant-time compare,
   validates the stored challenge against both the request client and the stored requester's
   client, and deletes the PKCE session in `PopulateTokenEndpointResponse` instead of
   `HandleTokenEndpointRequest`. `GetPKCERequestSession` currently returns a request with a nil
   `Client`; that is safe today but should carry the client to stay robust.
9. **Redirect security check**: identical (`IsRedirectURISecure` rejects only non-loopback `http`),
   so private-use schemes under ADR 042 keep working.
10. **Errors**: `RFC6749Error` keeps `ErrorField`, `DescriptionField`, `CodeField`, `WithHintf`,
    `WithDebugf`, `WithWrap`; all sentinels used by the broker exist under the same names. Errors
    are wrapped with `pkg/errors` `WithStack`, which supports `errors.As`/`errors.Is`, as before.
11. **Transactions**: `storage.Transactional` and `MaybeBeginTx/Commit/Rollback` are identical.
12. **Sessions**: `DefaultSession` has `Extra map[string]any`, `GetExtraClaims`, and `Clone` (deep
    clones `Extra`). The broker's `consent_grant_id`, `refresh_parent_signature`,
    `client_auth_method`, `email`, `display_name` extra claims survive cloning.
13. **Config**: `oauth2.Config` is a struct implementing every provider interface, like
    `fosite.Config`. Field names used by the broker are unchanged. New relevant fields:
    `EnforcePKCEForPublicClients`, `ClientCredentialsFlowImplicitGrantRequested`,
    `DisableRefreshTokenRotation`, `SanitationWhiteList`, `AudienceStrategy`, `ResourceStrategy`
    (nil → default strategies, which accept empty audience/resource).

## R5 — Compile and test spike (evidence)

Done in a throwaway worktree on branch `spike/authelia-oauth2`, never pushed. Patch saved at
`/tmp/claude-0/-home-user-agentic-identity-broker/282b5ee5-afda-5d59-9969-2b85b979dd99/scratchpad/spike-authelia-v0.3.3.patch`.

1. `go get authelia.com/provider/oauth2@v0.3.3`; `go mod edit -droprequire github.com/ory/fosite`;
   `go get github.com/dgraph-io/ristretto@v0.2.0`; `go mod tidy`.
   Without the explicit ristretto downgrade the build fails inside the library: the broker's graph
   had ristretto v1.0.0 (generic API) and v0.3.3 needs v0.2.0.
2. Import rewrite only (`sed`): `github.com/ory/fosite` → `authelia.com/provider/oauth2` aliased as
   `fosite`, `handler/oauth2`, `handler/pkce`, `storage` likewise.
3. Compile errors: exactly the methods in R4 items 1 and 2 (7 one-line methods). Nothing else.
4. `go build ./...` and `go vet ./...` clean for the whole repository.
5. `go test ./internal/domain/oauth2server/`: 3 failing top-level tests, all in
   `refresh_consent_test.go` (`TestProvider_ConcurrentRefreshAssertionsRotateExactlyOnce`,
   `TestProvider_RefreshConsentRevokedBeforeRotation`, `TestProvider_RefreshRevokedAfterConsumption`),
   all `invalid_request` where `invalid_grant` is expected — R4 item 5. Relaxing the workaround to
   map a non-`server_error` `invalid_request` from `PopulateTokenEndpointResponse` to `invalid_grant`
   makes the package green (`ok` in 6.7 s, including the concurrency test).
6. `golangci-lint` could not run here (installed binary built with Go 1.25, target is 1.27.3).
   Must be run by the executing agent via `just check`.
7. Focused backend E2E (`oauth2_*`, `refresh_consent`, `cimd_flow`, `cimd_private_key_jwt_connect`,
   `cimd_private_key_jwt_replay`, `cimd_private_use_redirect`, `local_token_exchange`,
   `impersonation_local`, `proactive_token_refresh`): see the result recorded at the end of this file.

## R6 — Risks and how the plan handles them

| Risk | Handling |
|---|---|
| authelia tags lag `master` by weeks and surfaces move between tags (R4 items 1, 5, 8) | Pin the latest tag at execution time, re-run the compile step, and apply the `master`-conditional tasks only if the tag includes them. Never depend on `master` pseudo-versions. |
| "Latest Go minor only" policy | Already satisfied (1.27). Record in the ADR that toolchain bumps and library bumps move together. |
| Hidden behavioural drift in handlers | Package tests (11.6k lines) plus the 1:1 E2E suites are the oracle; the spike shows only one deliberate error-mapping change. |
| Refresh re-read inside the transaction doubles consent/resolver reads | Accept; document in ARCHITECTURE.md "Authorization-code exchange reads" paragraph's sibling for refresh. Optional follow-up: short-circuit `GetRefreshTokenSession` when the exchange client is already in context, as the code path does for authorization codes. |
| `UpdateRefreshTokenSession` could be reached if rotation is ever disabled | Fail closed. |
| ADR 014 and ADR 043 name fosite | New superseding ADR (Principle II); ADR 043's comparison table keeps its historical meaning with a note. |
| go-jose v3 exceptions expire 2026-11-30 | Migration removes go-jose v3; delete the three `osv-scanner.toml` entries and the CONTRIBUTING paragraph in the same PR. |

## R7 — Effort

Code: ~7 new one-line methods, 1 new storage method, config struct literal, one error-mapping
change, import rewrite across 30 files, rename of `fosite_*` files and `FositeStorage` type
(optional but recommended since the name is now wrong), test fixture updates (`fosite.DefaultClient`
in two tests). Docs: ADR 047, ARCHITECTURE.md (6 paragraphs), ADR 014/043 cross-references,
`osv-scanner.toml`, `CONTRIBUTING.md`, `docs/resources/assurance-case.md`, `AGENTS.md` index and
`internal/domain/AGENTS.md`. One PR, one to two days for an agent including full verification.

## Decision

Migrate. Target `authelia.com/provider/oauth2` at the latest tag (v0.3.3 at the time of writing),
keep the headless handler pattern, keep all client authentication in the broker.

## E2E result (filled in after the spike run)

Command (spike worktree, no `web/dist` built):

```
go test ./tests/e2e/ -timeout 25m -ginkgo.label-filter='!performance && !docker' \
  -ginkgo.focus-file='oauth2_|refresh_consent|cimd_flow|cimd_private_key_jwt_connect|cimd_private_key_jwt_replay|cimd_private_use_redirect|local_token_exchange|impersonation_local|proactive_token_refresh'
```

Result: `Ran 252 of 957 Specs — 249 Passed | 3 Failed`. The three failures are the
`CIMD Private-Use Redirect URIs` specs, each failing at `authorization_flow_helpers_test.go:63`
with `Expected 404 to equal 200` on the consent page HTML view. The broker serves the SPA from
`web/dist` on disk (`internal/app/builder.go:183`); the spike worktree never ran `just web-build`,
so the consent HTML returns 404 before any OAuth2 code runs. Every authorization-code, PKCE,
refresh-rotation, consent-revocation, client-credentials, discovery, signing-key, CIMD
`private_key_jwt`, local token-exchange and impersonation spec in the selection passed against
authelia v0.3.3. The executing agent must run the full `just test-e2e-backend` (which builds the
SPA) rather than this shortcut.
