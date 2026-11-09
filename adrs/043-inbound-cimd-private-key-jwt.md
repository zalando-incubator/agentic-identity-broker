# ADR 043: Inbound CIMD `private_key_jwt` Authentication

**Date**: 2026-10-16
**Status**: Accepted
**Feature**: 051-private-key-jwt-auth

## Context

CIMD -02 permits a client metadata document to declare `private_key_jwt`. The broker currently rejects this method, while accepting it without enforcement would allow unauthenticated token requests and downgrade the client's declared security posture. Authentication must cover authorization-code exchanges and refresh-token grants, enforce one-time assertions across replicas, and preserve the method strength of an existing refresh-token grant if the document later changes.

The broker has two related but different JWKS use cases. Outbound token exchange validates assertions from upstream services under ADR 008. This decision covers inbound assertions from CIMD clients.

## Decision

The broker will verify inbound CIMD client assertions in a domain verifier built on `jwx` v4, before invoking fosite grant handlers. The verifier pins RS256 or ES256 from the client document, requires `kid`, `jti`, and bounded claims, then atomically consumes `(client_id, jti)` in a replay repository that is independent of the grant transaction. Refresh sessions record `client_auth_method`; refresh authentication uses the stricter of the stored method and the current document method.

Key sets use a separate `ClientKeySetService` and cache. Remote sets are fetched through a second instance of the SSRF-hardened CIMD fetcher, with a key-set-specific timeout and response-size limit. A missing `kid` can cause at most one refetch per configured client interval. Fetch and replay-store failures fail closed as server-side dependency errors.

| Requirement | Fosite default client authentication | ADR 043 profile |
|---|---|---|
| Replay scope | `jti` without `client_id` | `(client_id, jti)` |
| Replay consumption | Separate check and write | Atomic storage operation outside grant transaction |
| `kid` | Optional | Required; key comes from the client's document |
| Algorithms and key strength | No complete profile pinning | Document-pinned RS256/ES256; RSA >= 2048 or EC P-256 |
| Assertion lifetime | No five-minute profile cap | Five-minute cap, 30-second skew |
| Unknown key refresh | Unconditional force refresh | Per-client minimum interval and singleflight |
| Fetch protections | Generic JWKS fetch behavior | CIMD SSRF dialer, TLS verification, no redirects, bounded body and timeout |

The fixed profile consists of RS256 and ES256 only, a maximum five-minute assertion lifetime, 30-second clock skew, and mandatory `kid` and non-empty `jti`. `iss` and `sub` equal the `client_id`; the audience is the configured issuer or canonical token endpoint. `jti` is consumed only after signature and claim validation, but before grant processing.

This decision partially supersedes ADR 008 only for inbound CIMD client key sets. The existing outbound token-exchange JWKS adapter and its `jwk.Cache` pattern remain unchanged. Inbound key sets do not use that pattern because it does not provide the CIMD SSRF fetcher's connection-time blocklist checks, per-client unknown-`kid` refetch interval, or required fail-closed dependency classification.

This decision supersedes spec 028 FR-022 and its public-only edge case only for `private_key_jwt`. Spec 028 SR-009 remains binding: shared-secret methods stay rejected. ADR 015's SSRF blocklist, verified TLS, no-redirect behavior, and bounded HTTP reads continue to apply. Its document-fetch timeout, response-size budget, and Cache-Control-derived document cache TTL do not define key-set budgets or key-set cache policy; those use the separate client-authentication settings.

## Consequences

- CIMD clients can authenticate with public keys from their own metadata, without a broker-managed shared secret.
- Authentication failures do not consume codes or rotate refresh tokens; authenticated assertions are still spent if later grant checks fail.
- Replay protection is durable and atomic in PostgreSQL, and in-memory only in development/test storage.
- Existing grants cannot be weakened by editing the client's metadata document.
- Key rotation depends on bounded cache expiry or a rate-limited unknown-`kid` refresh.
- Authorization-server metadata advertises this method only when CIMD is enabled in local or hybrid mode.
- API contract changes require stakeholder confirmation before implementation.

## Status

Accepted in feature 051 after implementation and the three API contract deltas were reviewed and confirmed on 2026-10-16.