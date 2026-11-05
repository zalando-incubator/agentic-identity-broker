# ADR 047: OAuth2 Server Library — `authelia.com/provider/oauth2`

**Status**: Proposed (set to Accepted in the migration PR)
**Date**: 2026-11-05
**Supersedes**: ADR 014's choice of `ory/fosite`. ADR 014's headless-integration pattern, strategy
implementations, and type-containment rules remain in force.

## Context

ADR 014 chose `ory/fosite` as a headless OAuth2 protocol library. Ory consolidated development into
the Hydra module in December 2025; fosite now lives at `github.com/ory/hydra/v2/fosite` and the
standalone `ory/fosite` module is no longer released. The broker is pinned to fosite v0.49.0, whose
only route to go-jose is v3, for which three advisories are excluded in `osv-scanner.toml` until
2026-11-30 with the stated plan to reassess "when fosite moves to go-jose v4". That will not happen
on the standalone module. Importing fosite from Hydra would bind the broker to Hydra's module graph
and release cadence.

`authelia.com/provider/oauth2` is a hard fork of fosite maintained by the Authelia project
(Apache-2.0). It keeps the handler/strategy/storage decomposition the broker depends on, is released
every few weeks, supports only the latest Go minor, and carries no go-jose v3. A compile-and-test
spike against v0.3.3 required seven one-line interface additions and one error-mapping change; the
`oauth2server` package tests and the focused backend E2E suites then passed.

## Decision

- Replace `github.com/ory/fosite` with `authelia.com/provider/oauth2`, pinned to a released tag,
  never a pseudo-version of `master`.
- Keep the headless pattern: only `handler/oauth2` (`AuthorizeExplicitGrantHandler`,
  `ClientCredentialsGrantHandler`, `RefreshTokenGrantHandler`), `handler/pkce`, `storage` and the
  root request, response, session and error types are used. The library's HTTP layer, client
  authentication, JWT strategies, CIMD resolver, token exchange and OIDC packages are not used.
- Client authentication remains broker code (`ClientAuthService`, ADR 043). Broker client types
  return no secret to the library (`GetClientSecret` is nil).
- Refresh-token rotation is mandatory. `UpdateRefreshTokenSession`, which the library calls only
  when rotation is disabled, returns `server_error`.
- A rolled-back refresh rotation (token already used, revoked, or a serialization failure) is
  reported to clients as `invalid_grant` (RFC 6749 §5.2), whatever the library's own code for it.
- The `scope` parameter on `refresh_token` follows RFC 6749 §6 as implemented by the library: a
  subset of the originally granted scopes is honoured; any other scope returns `invalid_scope`.
- Library upgrades and Go toolchain upgrades are made together, because the library supports only
  the latest Go minor.
- Type containment is unchanged: `internal/domain/oauth2server/` is the only importer of
  `authelia.com/provider/oauth2`.

## Consequences

- go-jose v3 and all `ory/*` modules leave the module graph; the three go-jose exceptions in
  `osv-scanner.toml` are removed.
- Each refresh verifies continuing consent twice: once while handling the request and once inside
  the rotation transaction, because the library re-reads the refresh session before rotating.
- Tags of the library can lag `master` by several weeks and interfaces move between tags (for
  example `Client.GetResource`, strict PKCE challenge syntax). Every upgrade re-runs the full
  `oauth2server` package tests and the backend E2E suites.
- Token formats, signatures, stored sessions and configuration are unchanged; a rollback is a
  revert of one PR.

## Alternatives Considered

- **Import fosite from `github.com/ory/hydra/v2/fosite`**: rejected; it couples the broker to
  Hydra's module graph and versioning and does not resolve go-jose v3.
- **Stay on `ory/fosite` v0.49.0**: rejected; no security fixes, go-jose v3 exceptions cannot be
  closed.
- **Custom implementation (ADR 014 fallback, ~510 LOC)**: rejected for now; the fork preserves the
  battle-tested PKCE, replay and rotation logic at near-zero porting cost. It remains the fallback
  if the fork's compatibility policy becomes a burden.
