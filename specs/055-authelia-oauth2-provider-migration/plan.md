# Plan: Migrate the OAuth2 server library from `ory/fosite` to `authelia.com/provider/oauth2`

**Input**: `research.md` in this folder (read it first; it holds the evidence and the interface
deltas this plan relies on).
**Branch**: `055-authelia-oauth2-provider-migration`
**Scope**: One PR. No API, schema, configuration, or user-visible behaviour change, with one
documented exception (refresh `scope` narrowing, task T13). Fosite types stay contained in
`internal/domain/oauth2server/` under the new library, exactly as ADR 014 requires.

## Summary

Replace the frozen `github.com/ory/fosite v0.49.0` dependency with the maintained hard fork
`authelia.com/provider/oauth2` at its latest tag. The broker uses only the headless handler layer
(`handler/oauth2`, `handler/pkce`, `storage`, root request/response/error types) and implements
its own strategies, storage adapters and client authentication. A compile-and-test spike showed
the port is seven one-line interface additions, one new storage method, a config literal, and one
error-mapping change; the package and focused E2E suites then pass. The migration also removes
go-jose v3 from the module graph, which closes three `osv-scanner.toml` exceptions that expire on
2026-11-30.

## Technical Context

- **Language**: Go 1.27.3 (authelia supports only the latest Go minor; aligned).
- **Library target**: `authelia.com/provider/oauth2` latest tag. v0.3.3 (2026-09-19) when this plan
  was written. Check `git ls-remote --tags https://github.com/authelia/oauth2-provider.git` first.
  If a newer tag exists, re-run T4 and T5 against it and apply the conditional tasks (T10, T12, T14).
- **Pinned transitive**: `github.com/dgraph-io/ristretto` must resolve to the version the chosen
  tag requires (v0.2.0 for v0.3.3; `ristretto/v2` on `master`). `go mod tidy` alone does not fix a
  pre-existing v1.0.0 pin.
- **Packages touched**: `internal/domain/oauth2server/` (all files), `go.mod`, `go.sum`, docs.
- **Unchanged**: `internal/ports`, `internal/adapters`, `internal/app`, HTTP handlers, storage
  repositories, migrations, OpenAPI contracts, Helm chart, web.

## Constitution Check

| Principle | Status |
|---|---|
| I Security-First | Client authentication stays in broker code; `UpdateRefreshTokenSession` fails closed; PKCE S256-only and `EnforcePKCE` retained. |
| II ADRs binding | ADR 014 is merged and Accepted; this change supersedes its library choice, so a new ADR 047 is required and MAY be Accepted in the same PR (T2). ADR 043 only references fosite as comparison baseline; add a one-line note, do not amend its decision. |
| III Library-First Security | Same category of library (fosite lineage), Apache-2.0. |
| IV / X API-First | No API change. |
| V DDD / glossary | Glossary entries that say "fosite request chain" are reworded (T20). |
| VI Hexagonal | Type containment unchanged; `oauth2server` remains the only importer. |
| VII Configuration | No new config keys. |
| VIII TDD | Red–green on the error mapping (T11) and on the new storage method (T9). Existing 11.6k lines of package tests plus 1:1 E2E suites are the regression oracle. |
| IX Persistence | No repository or migration change. |
| XII DI via builder | `NewProvider` signature unchanged; builder untouched. |
| XIII E2E | Existing suites are the acceptance tests; no new journey. |
| XIV Spec by example | Not applicable: no new user-facing journey. The ADR carries the decoded-error example for the refresh mapping. |

## Project Structure

```
specs/055-authelia-oauth2-provider-migration/
├── research.md      # evaluation and evidence (done)
├── plan.md          # this file
└── adr-047-draft.md # ADR text to copy to adrs/047-authelia-oauth2-provider.md (T2)

internal/domain/oauth2server/
├── provider.go            # handler wiring, Config literal, refresh error mapping
├── oauth2_storage.go      # renamed from fosite_storage.go; adds UpdateRefreshTokenSession
├── oauth2_client.go       # renamed from fosite_client.go; new Client methods
├── strategies.go          # IsOpaque* methods
├── errors.go              # translateFositeError → translateLibraryError
├── profile_session.go, token_claims_cel.go, argon2_hasher.go, client_auth.go  # import swap only
└── *_test.go              # import swap, DefaultClient fixtures, renamed helpers
```

## Tasks

Format: `[ID] [P] description — files — done when`. `[P]` = can run in parallel with its
neighbours. Commit after each phase. Keep the import alias `fosite` during the swap so diffs stay
reviewable; T18 renames it at the end in one mechanical commit.

### Phase 1: Setup

- **T1** Create branch `055-authelia-oauth2-provider-migration` from `main`.
  Record the chosen library tag in this file under Technical Context.
- **T2** Write `adrs/047-authelia-oauth2-provider.md` from `adr-047-draft.md`, status `Accepted`,
  `Supersedes: ADR 014 (library choice only)`. Add the row to the ADR index in `AGENTS.md`.
  Done when: file exists, index row present, ADR 014 gets a `Superseded by ADR 047 for the library
  choice` line under Status (do not rewrite ADR 014's body).

### Phase 2: Design preconditions

- **T3** [P] Domain model & glossary: no new entity. List in the PR description the glossary
  entries touched by T20. Configuration, API, database, frontend: no change (state this in the PR).
- **T4** Dependency swap, compile only:
  ```
  go get authelia.com/provider/oauth2@<tag>
  go mod edit -droprequire github.com/ory/fosite
  go get github.com/dgraph-io/ristretto@<version required by the tag's go.mod>
  go mod tidy
  ```
  Then rewrite imports in `internal/domain/oauth2server/*.go`:
  `github.com/ory/fosite` → `fosite "authelia.com/provider/oauth2"`,
  `github.com/ory/fosite/handler/oauth2` → `authelia.com/provider/oauth2/handler/oauth2`,
  `github.com/ory/fosite/handler/pkce` → `authelia.com/provider/oauth2/handler/pkce`,
  `github.com/ory/fosite/storage` → `authelia.com/provider/oauth2/storage`.
  Done when: `go build ./internal/domain/oauth2server/` reports only missing-method errors on
  `confidentialClient`, `publicClient`, `JWXAccessTokenStrategy`, `RandomCodeStrategy`,
  `RandomRefreshTokenStrategy` (research R5 step 3), and `go list -m all | grep -E 'ory/|go-jose'`
  is empty.
- **T5** Diff the chosen tag against the surfaces in research R4 before coding:
  ```
  git clone --depth 1 --branch <tag> https://github.com/authelia/oauth2-provider.git /tmp/authelia
  grep -A12 'type Client interface' /tmp/authelia/client.go
  grep -A6 'type AccessTokenStrategy interface' /tmp/authelia/handler/oauth2/strategy.go
  grep -A14 'type RefreshTokenStorage interface' /tmp/authelia/handler/oauth2/storage.go
  grep -c validateChallengeSyntax /tmp/authelia/handler/pkce/handler.go
  grep -c 'replayed != nil' /tmp/authelia/handler/oauth2/flow_refresh.go
  ```
  Record in the PR description which conditional tasks (T10, T12, T14) apply.

### Phase 3: Make it compile

- **T6** [P] `oauth2_client.go` (rename from `fosite_client.go` with `git mv`): remove
  `GetHashedSecret`; add to both client types
  `GetClientSecret() fosite.ClientSecret { return nil }` and
  `GetClientSecretPlainText() ([]byte, bool, error) { return nil, false, nil }`.
  Add a comment above both stating that the library never authenticates broker clients
  (`ClientAuthService` and ADR 043 do), so no secret is exposed to it.
  Add `GetResource() fosite.Arguments { return fosite.Arguments{} }` if T5 shows it in the
  interface (it is on `master`).
- **T7** [P] `strategies.go`: add `IsOpaqueAccessToken` (false), `IsOpaqueAuthorizeCode` (true),
  `IsOpaqueRefreshToken` (true). Keep the compile-time interface assertions.
- **T8** [P] `oauth2_storage.go` (rename from `fosite_storage.go`): rename type `FositeStorage` →
  `OAuth2Storage`, constructor `NewFositeStorage` → `NewOAuth2Storage`, field
  `Provider.fositeStorage` → `Provider.storage`. Update all tests (`fosite_storage*_test.go` →
  `oauth2_storage*_test.go`).
- **T9** `oauth2_storage.go`: implement `UpdateRefreshTokenSession(ctx, signature string, req
  fosite.Requester) error` returning `fosite.ErrServerError.WithDebug("refresh token rotation is
  mandatory")`. Test first: a unit test asserting the error and that no repository method is
  called. Rationale comment: rotation is never disabled in the broker; a non-rotating path must
  not succeed silently.
- Done when: `go build ./... && go vet ./...` is clean.

### Phase 4: Behaviour

- **T10** `provider.go` `NewProvider`: build `&fosite.Config{...}` with the existing fields plus
  `SanitationWhiteList: []string{"scope", "redirect_uri", "code_challenge", "code_challenge_method"}`
  (makes the form fields `CreateAuthorizeCodeSession` reads explicit) and
  `EnforcePKCEForPublicClients: true`. Leave `ClientCredentialsFlowImplicitGrantRequested` false
  and keep the explicit `req.GrantScope` loop in `HandleClientCredentials` (it is the broker's
  deliberate grant step; add a comment). Leave `DisableRefreshTokenRotation` false.
  If the tag's `AuthorizeExplicitGrantHandler.Config` interface embeds providers that
  `*fosite.Config` does not satisfy (it does satisfy all of them on v0.3.3 and `master`), stop and
  report rather than wrapping the config.
- **T11** `provider.go` `HandleRefreshToken`, red–green:
  1. Run `go test ./internal/domain/oauth2server/ -run 'TestProvider_ConcurrentRefreshAssertionsRotateExactlyOnce|TestProvider_RefreshConsentRevokedBeforeRotation|TestProvider_RefreshRevokedAfterConsumption'`
     and confirm the three fail with `invalid_request` where `invalid_grant` is expected (red).
  2. Replace the workaround after `PopulateTokenEndpointResponse` with:
     ```go
     if errors.Is(err, fosite.ErrInvalidRequest) && !errors.Is(err, fosite.ErrServerError) {
         // The library reports a rolled-back rotation (token already used, revoked, or
         // serialization failure) as invalid_request. RFC 6749 §5.2 requires invalid_grant.
         return nil, fosite.ErrInvalidGrant
     }
     ```
  3. Green. Add one unit test that a genuine `invalid_request` from `HandleTokenEndpointRequest`
     (for example an unknown requested scope → `invalid_scope`, and a missing `refresh_token`
     form value) still surfaces unchanged, so the mapping is proven to apply only to the
     post-rotation path.
  If T5 shows the tag already returns `invalid_grant` for replay and revocation (`master` does),
  keep the mapping anyway; it is then a no-op guard and the three tests stay green.
- **T12** Conditional (strict PKCE in the tag, T5): update any test that sends a literal
  `code_challenge` shorter than 43 characters through `Provider.HandleAuthorize`. The current
  tests use real S256 challenges; only storage-level tests use `"challenge"` literals and those
  do not pass through the handler. Verify with
  `grep -rn '"code_challenge": {"' internal tests`.
- **T13** Refresh `scope` parameter. The library now honours RFC 6749 §6 (narrowing allowed,
  widening → `invalid_scope`). Add two package tests in `provider_test.go`:
  (a) refresh with a subset of originally granted scopes returns that subset in `Scope` and the
  stored successor session carries the subset; (b) refresh with a scope not originally granted
  returns `invalid_scope`. Confirm with the E2E `refresh_consent_e2e_test.go` and
  `oauth2_server_mode_e2e_test.go` that no spec asserts the old "ignore scope" behaviour. If one
  does, the spec wins: file the deviation in the ADR and keep the test red until decided.
  Record the outcome in the ADR 047 Consequences section.
- **T14** Conditional (tag deletes the PKCE session in `PopulateTokenEndpointResponse` and
  validates against `requesterPKCE.GetClient()`, T5): set `Client` on the request returned by
  `GetPKCERequestSession` to the client from `exchangeClientContextKey` when present, else resolve
  it with `GetClient`. Add a test that a PKCE session is deleted after a successful exchange and
  retained after a failed verifier check (it is deleted only in Populate, which never runs after a
  failed `HandleTokenEndpointRequest`).
- **T15** `errors.go`: rename `translateFositeError` → `translateLibraryError`, update the comment
  and callers; keep `RFC6749Error` and the sentinels unchanged (they are the public contract of
  the package). Rename `errors_test.go` cases accordingly.
- **T16** `token_claims_cel.go`, `profile_session.go`, `argon2_hasher.go`, `client_auth.go`,
  `impersonation` paths: no logic change; confirm `requester.(*fosite.AccessRequest)` still type
  switches correctly (it does, `AccessRequest` is a struct in the root package).
- **T17** Package doc `doc.go`: replace the fosite sentence with "It uses
  `authelia.com/provider/oauth2` (the maintained fork of ory/fosite) as a headless domain library…".
  Keep the containment sentence.
- **T18** Rename the import alias `fosite` → `oauth2lib` across the package in one mechanical
  commit (`gofmt -r` is not enough; use `sed` on the alias then `gofmt`). The identifier `oauth2`
  is already taken by `internal/domain/oauth2`. Update ARCHITECTURE.md text that says
  "fosite types" to "oauth2-provider types".

### Phase 5: Dependencies and security hygiene

- **T19** `osv-scanner.toml`: delete the three go-jose v3 entries (`GHSA-4w3g-jfj2-cw2q`,
  `GHSA-mgcc-cww5-fp2q`, `GHSA-vvm8-gf78-28g4`). Run `osv-scanner --config osv-scanner.toml -r .`
  (or the CI equivalent) and `govulncheck ./...`; both must be clean. `CONTRIBUTING.md` lines
  70–76: rewrite the paragraph to mention only the deepmerge exception.
  Confirm `go list -m all | grep -E 'go-jose|ory/'` is empty and that `ristretto`,
  `go-retryablehttp`, `pkg/errors`, `x/text` appear as indirect requirements of the new library.

### Phase 6: Documentation

- **T20** [P] `ARCHITECTURE.md`: lines around 569–597 (OAuth2 server mode, type containment,
  "Authorization-code exchange reads", "Continuing consent for local refresh") and glossary rows
  1825 (`RefreshTokenSession`) and 1851 (`OAuth2ServerProvider`): replace fosite with the new
  library name, keep the link to ADR 047, and add one sentence under the refresh paragraph that
  the library re-reads the refresh session inside the rotation transaction, so consent is
  verified twice per refresh.
- **T21** [P] `adrs/043-inbound-cimd-private-key-jwt.md`: add a note under the comparison table
  that "Fosite default client authentication" describes the baseline at decision time; the
  successor library's defaults were not re-evaluated because the broker never uses them.
- **T22** [P] `docs/resources/assurance-case.md` line 107, `internal/domain/AGENTS.md` and
  `internal/AGENTS.md` wherever fosite is named, `AGENTS.md` ADR index, and `specs/025-oauth2-server/`
  are historical and stay as written (specs are not rewritten after delivery).
- **T23** [P] `internal/ports/storage.go` (485, 499, 502), `internal/ports/oauth2.go` (272),
  `internal/domain/storage/pkce_session.go` (6): reword "fosite request ID / code signature" to
  "OAuth2 request ID / code signature". Comment-only change.

### Phase N: Constitution compliance and verification

- **T24** `just check` (fmt, vet, lint). Note: the lint binary in the cloud session that produced
  this plan was too old for Go 1.27; the executing environment must have a golangci-lint built
  with Go ≥ 1.27.
- **T25** `just test` (fast suites) green.
- **T26** `just test-e2e-backend` green (it builds the SPA; the shortcut in research R5 step 7 is
  not sufficient). Then `just test-e2e-extproc` and `just test-integration`.
  `just verify` as the final gate.
- **T27** Grep gate: `grep -rn "ory/fosite" --include=*.go . ; grep -rn "ory/fosite" go.mod` must
  be empty; `grep -rln "authelia.com/provider/oauth2" --include=*.go . | grep -v internal/domain/oauth2server`
  must be empty (type containment).
- **T28** PR description: list the behavioural deltas (refresh `scope`, refresh error mapping,
  double consent read), the conditional tasks applied, the removed advisories, and the ADR status.

## Rollback

Revert the single PR. No data written by the new library differs in shape: codes, PKCE sessions
and refresh sessions are keyed by the same SHA-256 signatures and the storage adapters are
unchanged, so tokens issued before and after the switch are interchangeable within their TTLs.

## Out of scope (possible follow-ups, each needs its own spec)

- Replace the broker's own CIMD resolver with `handler/cimd`.
- Use `handler/rfc8693` for the local token-exchange path.
- Short-circuit the second `GetRefreshTokenSession` read in the rotation transaction.
