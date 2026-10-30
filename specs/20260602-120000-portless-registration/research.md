# Research: Portless Redirect URI Registration

**Branch**: `feat/ephemeral-runtime-port` | **Date**: 2026-06-02

## Decision 1: Change Scope — Two Sites, Not One

**Decision**: Both redirect URI list-membership check sites must be updated.

**Rationale**: The codebase has two independent code paths through authorize:
1. `internal/domain/oauth2/service.go` lines ~177-183 — the primary `HandleAuthorize` path, used for CIMD and opaque clients alike
2. `internal/domain/oauth2server/provider.go` `containsRedirectURI()` helper — the fosite-backed redirect-matching path for the OAuth2 server mode

Both perform `==` string equality. If only site 1 is patched, agents using the fosite path get inconsistent behavior.

**Alternatives considered**: Patching only site 1 (insufficient — dual-path architecture confirmed by Explore agent). Patching at the fosite storage layer (not viable — fosite's `GetRedirectURIs()` returns the raw string slice and fosite performs its own comparison internally that the broker cannot intercept).

---

## Decision 2: `MatchesRedirectURI` and `IsValidRedirectURI` live in `internal/domain/urivalidation/`

**Decision**: Both `MatchesRedirectURI(registered, incoming string) bool` and `IsValidRedirectURI(uriStr string) bool` live in `internal/domain/urivalidation/redirect.go`. The `internal/domain/storage` package was the original placement but was revised after implementation — URI validation predicates belong in the dedicated `urivalidation` package, not alongside the `Agent` entity.

**Rationale**:
- `internal/domain/urivalidation/` already exists as the home for URI validation logic (`ValidateCIMDClientURL`)
- `storage/agent.go` is an entity model file — pure predicate functions about URI semantics do not belong there
- All call sites (`oauth2/service.go`, `oauth2server/provider.go`, `oauth2/cimd/document.go`) now import `urivalidation` directly
- `storage/agent.go` imports `urivalidation` for its `validateFields` redirect URI check

**Alternatives considered**:
- `internal/domain/storage/agent.go` (original): rejected post-implementation — mixing entity model with URI validation predicates weakens package cohesion
- New `internal/domain/oauth2/redirect/` package: rejected — no reason to add a package when `urivalidation` already exists
- Inline the logic at each site: rejected because dual-site maintenance is error-prone
- Add a method to `storage.Agent`: rejected because the comparison is not tied to any entity

---

## Decision 3: `IsValidRedirectURI` Is Unchanged (logic)

**Decision**: `IsValidRedirectURI` logic requires no changes — only its package home moved to `internal/domain/urivalidation/`.

**Rationale**: It validates structural validity (scheme, host non-empty, no fragment). It already accepts `http://localhost/callback` (no port) as valid — Go's `url.Parse` returns an empty `.Port()` for a URI without a port, and the function only checks scheme + host, not port. The write-time validator does not need to know about port-matching semantics.

---

## Decision 4: No Audit Logging on Loopback Match

**Decision**: Successful loopback redirect URI matches are not logged beyond existing error-path logging.

**Rationale**: A successful validation is an expected, unremarkable outcome — the same reason non-loopback matches are not logged. The security-relevant facts (user authorized agent, grant created, authorization code issued) are captured in grant and session audit events. The ephemeral port is incidental transport. PKCE (mandatory for native apps per RFC 8252 §8.1) ensures an intercepted code at any port is unusable without the `code_verifier`, so port visibility provides no meaningful security value in logs.

---

## Decision 5: No CIMD Document Re-validation Needed

**Decision**: `internal/domain/oauth2/cimd/document.go` `validateRedirectOrigin` does not need to change.

**Rationale**: That function validates the CIMD document's declared `redirect_uris` at fetch time — it checks same-origin against the `client_id` URL. It already skips same-origin enforcement entirely for loopback hosts (`localhost`, `127.0.0.1`, `::1`). Port is never checked for loopback in that path. No change needed.

The only change needed is at authorization-request time: when the incoming `redirect_uri` is matched against the document's stored list.

---

## Decision 6: IPv6 `::1` Uses the Same Loopback Match Rule

**Decision**: `::1` is included in the loopback port-ignore set in `MatchesRedirectURI`.

**Rationale**: `IsValidRedirectURI` already permits `http://[::1]/callback` structurally, `validateRedirectOrigin` already skips same-origin checks for `::1`, and RFC 8252 §7.3 treats IPv6 loopback the same as other loopback interfaces. Normalizing `url.URL.Host` to `Hostname()` keeps the existing bracketed-literal parsing while allowing ephemeral-port matches for `http://[::1]:<port>/callback`.

---

## Decision 7: fosite Token Endpoint Binding — No Change

**Decision**: The fosite token endpoint `redirect_uri` binding (RFC 6749 §4.1.3) does not need to change.

**Rationale**: The fosite `AuthorizeExplicitGrantHandler` binds the token-request `redirect_uri` against the one stored in the authorization code session — not against the registered list. The session was created after the authorize endpoint already validated the incoming `redirect_uri` using the (now-updated) loopback-aware comparator. The session stores the exact incoming URI that was validated. The token endpoint just checks `session.redirect_uri == token_request.redirect_uri` — which will be an exact match because both sides use the same ephemeral-port URI. No fosite internals need to change.

---

## Decision 8: No ADR Required

**Decision**: No new ADR will be created for this change.

**Rationale**: The spec (`specs/20260602-120000-portless-registration/spec.md`) and its clarifications session fully document the RFC 8252 §7.3 / OAuth 2.1 §2.3.1 mandate, the loopback host set (`localhost`, `127.0.0.1`, `::1`), and the non-loopback exact-match constraint. The decision is a correction to make the broker RFC-compliant — it is not an architectural pattern choice. An ADR would duplicate the spec without adding information.
