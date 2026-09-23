# ADR 037: Consent Selection OAuth Redirect State

**Status**: Proposed
**Date**: 2026-09-22
**Feature**: 008-thirdparty-oauth2-sessions — User Story 5 Amendment

---

## Supersedes

This decision supersedes only the URL-encoded consent-selection transport in Feature 008 User Story 5. The User Story 5 amendment in Feature 008 is the current specification for this transport.

Feature 008 remains authoritative for third-party OAuth session initiation, JWE and PKCE validation, token exchange, token storage, callback errors, and all unrelated session behavior.

## Context

The consent page puts selected permission sets in `consent_state`. It base64url-encodes the selection map in the return URL before a third-party OAuth login starts.

The broker places that return URL in the authenticated third-party state JWE. More selections make the JWE larger. Some providers limit the `state` parameter to about 6,000 bytes.

The selection map is presentation state. It does not authorize a grant. The browser tab that begins the login already owns this state.

## Decision

The consent page stores `ConsentSelections` in browser `sessionStorage` under an opaque UUID reference. The page creates the reference with `crypto.randomUUID()` and uses the key prefix `agentic-identity-broker:consent-state:`. The reference is a bare UUID. It needs collision resistance only among live records in the originating tab during the record lifetime. The key prefix is not part of `consent_state_id`. The page also keeps its current pending reference under the internal key `agentic-identity-broker:pending-consent-state`.

Each stored record contains `expiresAt` and `selections`. Records expire after 15 minutes, which matches the maximum third-party state lifetime. Saving prunes expired records under this prefix.

When selections exist, the page submits `redirect_uri` and `consent_state_id=<uuid>` in a same-origin form POST. The broker seals the bare UUID into the private `consent_state_id` claim of `OAuth2StateTokenClaims`. The nested return URL contains no `consent_state_id`, `consent_state`, or selection JSON. The existing `redirect_uri` JWE claim carries the protected return URL. The third-party state JWE otherwise remains unchanged and still protects the PKCE verifier, authenticated principal, service binding, expiration, and validated return URL.

The page blocks a login with non-empty selections when it cannot store the record. It shows `Unable to preserve selections. Please try again.` and does not navigate.

A missing, malformed, expired, or unreadable pending reference does not interrupt callback processing. The consent page uses the existing grant or default selection state. A valid record remains until it expires so that a page reload retains the same-tab selection state. The callback returns the original `redirect_uri` with its existing status parameters and no selection-state URL parameter.

This design supports the originating browser tab only. It does not synchronize selections across tabs.

## Consequences

The provider-facing state has a bounded selection contribution. The JWE adds only a fixed-size UUID claim, so selection count does not grow the state JWE.

A callback opened in another tab, an expired record, or unavailable browser storage cannot restore presentation selections. The third-party OAuth callback still completes with existing grant or default selections.

This decision does not make arbitrary future return-URL data bounded. A future cross-tab or server-owned variable payload needs a separate opaque server-side correlation design.

## References

- Supersedes the selection transport in [Feature 008 User Story 5](../specs/008-thirdparty-oauth2-sessions/spec.md#user-story-5---preserve-consent-selections-across-third-party-oauth2-redirects-priority-p2)
- Feature 008 amendment: [Third-Party OAuth2 Session Management](../specs/008-thirdparty-oauth2-sessions/spec.md)
- Constitution: `.specify/memory/constitution.md` — Principles II, VIII, and XIII
