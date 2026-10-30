# ADR 20260428-120000: Stateless Authorization Sessions — Consent Screen Anti-Spoofing via JWE Token

**Status**: Accepted
**Date**: 2026-04-28

---

## Context

When an agent uses a CIMD URL-format `client_id`, the OAuth2 authorization flow must display the agent's identity metadata (client name, logo, redirect URIs, scopes) on the consent screen so the user can make an informed delegation decision.

A naïve implementation passes this metadata as URL query parameters on the consent page redirect (e.g., `?client_name=...&logo_uri=...&redirect_uri=...&scope=...`). This creates a consent screen spoofing vector: an attacker can craft a consent URL with misleading metadata — a different display name, a different logo, or inflated scopes — and trick the user into granting permissions they would not otherwise approve. The frontend, which renders whatever the URL parameters say, becomes complicit in the phishing attack.

This is distinct from the SSRF risk addressed by ADR 20260425-120000 (fetcher architecture). ADR 20260425-120000 secures the *inbound* fetch of the CIMD document. This ADR secures the *outbound* delivery of that metadata to the consent UI.

---

## Decision

### JWE Authorization Session Token

For every CIMD-based authorization request, the `OAuth2AuthorizationService` mints a JWE token that captures the full authorization context at the moment the broker processes the `/authorize` request:

- Agent identity (`agent_id`, `client_id`)
- Authorization parameters (`redirect_uri`, `scope`, `state`, PKCE `code_challenge` + `code_challenge_method`)
- The original authorize URL (`original_url`) — used as the redirect target after consent
- A `CIMDMetadataSnapshot` containing the trusted CIMD document fields (`client_name`, `logo_uri`, `redirect_uris`, `auth_method`, `jwks_uri`) as resolved and validated by the CIMD service
- The authenticated principal (`principal`) — validated at consent-submission time to prevent cross-user replay
- `iat` and `exp` claims encoding issue time and a 10-minute TTL

The token is encrypted with `A256GCMKW` key wrapping and `A256GCM` content encryption using the existing `IDENTITY_BROKER_JWE_SIGNING_KEY`. The consent redirect URL contains only this opaque `session_token` parameter — no authorization context leaks into the URL.

### Token Properties

The `session_token` is a compact JWE with the following mandatory properties:

| Property | Mechanism | Rationale |
|---|---|---|
| **Tamper-proof** | JWE authenticated encryption (A256GCM) — any mutation breaks decryption | Equivalent guarantee to a signed + encrypted DB-backed token, with no storage required |
| **Confidentiality** | Encrypted — payload is opaque to the browser | Authorization context (redirect_uri, scope, state, PKCE) cannot be read or replayed by the browser |
| **TTL** | `exp` claim validated server-side; 10-minute TTL | Consent decisions are interactive and immediate; 10 minutes accommodates page load delays and user deliberation |
| **Principal binding** | `principal` claim must match the authenticated user at consent-submission time | Prevents a token issued for user A from being submitted by user B |
| **Agent-bound** | `agent_id` sealed into the token at issuance | Prevents a token created for agent A from being presented on agent B's consent page |

### Decode Endpoint

The consent page calls `GET /api/consent/session?token=<jwe>` to retrieve the structured authorization context. The endpoint decrypts the token, validates `exp`, and returns the display fields (client name, verified domain, redirect URI, scopes, CIMD metadata). The raw token value is never expanded into individual URL query parameters.

### Consent Submission

The consent submission body includes the `session_token`. The backend decrypts it, re-validates `exp` and the `principal` claim, then uses the token's trusted `redirect_uri`, `state`, and `code_challenge` values to issue the authorization code redirect. The frontend never supplies these values directly.

### Replay Within TTL Window

Without a `consumed_at` column, a JWE token can be resubmitted within its 10-minute validity window. This is accepted as benign:

1. Grant creation uses upsert semantics — a duplicate consent submission produces the same grant.
2. PKCE binds the resulting authorization code to the agent's `code_verifier`. A replayed consent submission issues a fresh code, but only the original agent (holding the verifier) can exchange it.

### Shared JWE Package

Both the existing `OAuth2StateToken` and the `AuthorizationSessionToken` follow the same pattern: JSON-marshal claims → JWE encrypt with A256GCMKW + A256GCM → compact serialization, and reverse. A shared `internal/domain/jwe/` package provides a single `TokenService` for both use cases.

### Non-CIMD Flows

For opaque `client_id` flows (UUID-format agent identifiers), the metadata displayed on the consent screen is static and server-controlled (it comes from the pre-registered agent record, not from an external document). These flows continue using URL parameters for backward compatibility, since there is no external-origin metadata to spoof.

### Redirect Target Integrity

After consent submission, the frontend needs the original authorize URL to re-enter the OAuth2 flow. This URL is sealed in the token's `OriginalURL` field and returned in the grant creation response. The frontend never constructs or modifies this URL — it receives it as an opaque string from the server.

---

## Consequences

**Benefits**:
- Eliminates consent screen spoofing for CIMD flows — the frontend renders only server-attested metadata
- No server-side storage required — the JWE token is self-contained and stateless
- The `session_token` is the sole URL-visible artifact; no authorization parameters leak into browser history, referrer headers, or server logs of intermediate proxies
- No database migration, no cleanup job, no replica synchronization concerns
- Key reuse — the existing `IDENTITY_BROKER_JWE_SIGNING_KEY` already used for `OAuth2StateToken` is sufficient; no new key management surface

**Trade-offs**:
- Replay within the TTL window is accepted (see above); replay does not produce a different grant due to upsert semantics, and PKCE limits code usability to the original agent
- Deep-linking directly to a consent URL without going through `/authorize` is intentionally impossible for CIMD flows — this is a feature, not a limitation
- Token size is bounded by the `AuthorizationSessionClaims` struct; CIMD metadata snapshot adds ~200–400 bytes to the compact JWE

**New invariant**: For CIMD flows, all authorization context displayed on the consent screen MUST originate from the decrypted `session_token`. Any future consent UI path that bypasses token decryption for CIMD agents violates this ADR.

---

## Amendment (2026-05-18): Extension to All Authorization Modes

The original decision scoped JWE session tokens to "every CIMD-based authorization request." This amendment extends the mechanism to all authorization modes (local UUID-based `client_id`, proxy, and CIMD).

**Rationale**: Using a unified state transport for all modes eliminates the `redirect_uri` query parameter as a spoofable vector regardless of agent type, simplifies handler logic (single validation path), and removes the need for mode-conditional branching in consent submission.

**Impact**: The "Non-CIMD Flows" section above is superseded. All flows now use `session_token` as the sole state transport to the consent page. The `redirect_uri` query parameter fallback is removed.
