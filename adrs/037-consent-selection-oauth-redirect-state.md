# ADR 037: Consent Selection OAuth Redirect State

**Status**: Proposed
**Date**: 2026-09-22
**Feature**: 008-thirdparty-oauth2-sessions — User Story 5 Amendment

---

## Supersedes

This decision supersedes only the URL-encoded consent-selection transport in Feature 008 User Story 5. The User Story 5 amendment in Feature 008 is the current specification for this transport.

Feature 008 remains authoritative for third-party OAuth session initiation, JWE and PKCE validation, token exchange, token storage, callback errors, and all unrelated session behavior.

## Context

The consent page formerly put selected permission sets in `consent_state`. It base64url-encoded the selection map in the return URL before a third-party OAuth login started.

The broker seals the return URL in the third-party state JWE. Both selection data and a `session_token` JWE in that URL can make the provider-facing state exceed about 6,000 bytes. The browser tab that begins the login already owns the return URL and presentation selections.

## Decision

The consent page stores its selection map and original same-origin return URL in current-tab `sessionStorage` under an opaque UUID reference (`agentic-identity-broker:consent-state:<uuid>`). Each record contains `expiresAt`, `selections`, `serviceID`, and `returnURL`. The bare UUID is generated with `crypto.randomUUID()`; the storage-key prefix is not part of `consent_state_id`. Records expire after 15 minutes, the maximum configurable third-party state lifetime (the default is 10 minutes). Saving prunes expired records.

When the page has selections or return-URL query/fragment data, it submits a same-origin form POST with `consent_state_id=<uuid>` and a clean `redirect_uri` consisting only of the page's origin and path. The original `session_token` and any other return-page parameters stay in current-tab storage rather than inside the provider-facing JWE. With neither selections nor return data, the page uses the existing GET flow. The broker seals the bare UUID into the private `consent_state_id` claim of `OAuth2StateTokenClaims`. The JWE still protects the PKCE verifier, authenticated principal, service binding, expiration, and validated return path.

After a successful token exchange and session write, the callback adds the verified `consent_state_id` to the return URL with `success=true` and `service_id`. The page restores only a non-expired record matching that exact ID, service, origin, and path. It restores the saved page URL, retaining `session_token` when present, and replaces the browser-history entry without `consent_state_id`. A later reload cannot replay the stored selections. A forged `success=true` without a matching ID cannot restore any record. Missing or invalid records use the existing grant or default selections without interrupting the callback.

If the page cannot store selections or return data, it does not start the login. This design supports the originating tab; it does not synchronize return data across tabs. The record remains until it expires, but the callback ID is consumed from the browser URL after restoration.

## Consequences

The provider-facing state has a bounded return-page contribution: only the clean return path and fixed-size UUID are sealed, regardless of selection count or query/fragment length. The broker also refuses to initiate any provider flow whose resulting `state` reaches 6,000 bytes, including GET flows with arbitrary `redirect_uri` values; it returns a client error instead of sending an oversized state to a provider.

Another tab, an expired record, or unavailable browser storage cannot restore presentation selections or tab-local return data. Supporting arbitrary long return URLs from non-browser clients or restoring cross-tab data requires a separately designed server-side correlation store.

## References

- Supersedes the selection transport in [Feature 008 User Story 5](../specs/008-thirdparty-oauth2-sessions/spec.md#user-story-5---preserve-consent-selections-across-third-party-oauth2-redirects-priority-p2)
- Feature 008 amendment: [Third-Party OAuth2 Session Management](../specs/008-thirdparty-oauth2-sessions/spec.md)
- Constitution: `.specify/memory/constitution.md` — Principles II, VIII, and XIII
