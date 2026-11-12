# ADR 042: Agent-Allowlisted Private-Use Redirect URIs

**Status**: Accepted
**Date**: 2026-10-22
**Scope**: Agent redirect_uris (spec 025 FR-006b/c/e/f), inbound CIMD redirect validation (spec 028 FR-004a/FR-004b), local and proxy authorization redirect matching

## Context

GitHub Copilot Desktop publishes `https://github.com/copilot/desktop/client-metadata.json`. Its `redirect_uris` are `github-app://oauth/callback`, `http://127.0.0.1/`, and `http://127.0.0.1/callback`. The broker currently rejects the whole document with `cimd_document_invalid` because its redirect validator accepts only HTTPS and loopback HTTP. The same-origin policy in spec 028 FR-004a also excludes the private-use callback.

RFC 8252 §7.1 permits private-use schemes for native apps. It recommends a reverse-domain scheme and the `scheme:/path` form. GitHub uses `github-app://oauth/callback`, which follows neither recommendation. CIMD draft-02 §8.1 permits an authorization server to restrict redirect URIs, so same-origin validation is local broker policy. Any app on a device can claim a private-use scheme (RFC 8252 §8.6). The broker cannot trust a remote document alone to authorize that destination.

## Decision

1. Accept a private-use callback only if the Agent's `redirect_uris` lists it byte for byte. A CIMD document must also list it, as FR-004 requires. The Agent entry is an exception to FR-004a, not a replacement for document registration. For a CIMD Agent, web entries in the Agent list remain valid on write but do not authorize web callbacks; the document governs them.
2. Reject a whole CIMD document if it lists a private-use URI absent from the Agent allowlist. Check on every client resolution, including cache hits and token-endpoint resolution. Do not cache a failing document.
3. Match private-use URIs by exact bytes in both lists. Do not fold case, match prefixes, schemes alone, hosts alone, or wildcards, or relax ports.
4. Apply the structural validation rules R1–R6 in spec 025 FR-006e: restrict bytes to ASCII letters, digits, and `-._~:/?@!$&'()+,;=`; require canonical parsing, no opaque URI, userinfo, or empty query; exclude one-letter, HTTP(S), script-capable, local-content, and other WHATWG special schemes; require either an empty-host single-slash path or a lowercase, valid authority; and reject `.` and `..` path segments.
5. Accept both `scheme:/path` and `scheme://host/path` forms. Do not require a reverse-domain scheme. This deliberately departs from the RFC 8252 SHOULD to support GitHub Copilot Desktop.
6. After validation, redirect authorization responses and errors permitted by RFC 6749 §4.1.2.1 to the private-use URI. Token requests must repeat the identical `redirect_uri`. S256 PKCE remains mandatory.
7. Keep cross-origin HTTPS entries in CIMD documents rejected, even if an Agent lists them.
8. Implement this decision with `urivalidation.IsValidPrivateUseRedirectURI`, `urivalidation.IsRegisteredRedirectURI`, and `ClientIDMetadataDocument.ValidatePrivateUseRedirectURIs`.

## Consequences

- Operators must update the Agent before its CIMD client can use a private-use callback. Removal takes effect on the next request, even with a cached document.
- Clients with mixed-case schemes, including MSAL bundle-ID schemes, must register and send the lowercase spelling. Percent-encoded callbacks are unsupported.
- A private-use scheme does not prove client identity. The operator allowlist, S256 PKCE, and the full redirect URI in the consent screen's Advanced Details limit risk.
- The broker still auto-approves an active grant. It does not implement the RFC 8252 §8.6 SHOULD to re-prompt. This matches loopback behavior.
- The consent UI does not change. It adds no private-use warning.
- The denylist blocks known unsafe schemes only. Operators review every other scheme.
