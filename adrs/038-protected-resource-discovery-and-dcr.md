# ADR 038: Protected Resource Discovery and Dynamic Client Registration

**Status**: Accepted
**Date**: 2026-10-07
**Acceptance**: The stakeholder accepted ADR 038 in the implementation review on 2026-10-07. This includes the optional broker-wide DCR client name and Helm mapping.

## Context

A third-party service can identify a protected resource without giving the broker an authorization-server URL or client credentials. The broker must obtain the resource's metadata, choose an advertised issuer, and create a client identity. Discovery and registration expose the broker to untrusted network destinations. A failed refresh must not replace a working client or its status history.

ADR 015 locates the CIMD fetcher in `internal/adapters/cimd` and its address policy in `internal/domain/oauth2/cimd`. ADR 036 allows only `none` as an explicit token-endpoint authentication method. Confidential DCR clients need explicit Basic or POST authentication without the existing automatic method probe. These changes need a decision before the adapter moves or new authentication methods become available.

## Decision

### Ownership and trust boundary

`ThirdpartyOAuth2ProviderService` owns the protected-resource discovery flow and the existing service aggregate. It validates the requested source, metadata identities, issuer selection, client method, effective resource, credential encryption, and state changes. It uses a narrow `OAuthDiscoveryClient` port for unauthenticated challenge probes, bounded JSON reads, and one bounded JSON registration. The outbound HTTP adapter performs remote I/O. The Admin handler parses requests and formats safe results; Builder supplies the port. Direct authorization-server `metadata_url` discovery and manual services keep their existing behavior.

Move the shared blocked-address value object to `internal/domain/netpolicy`. Move the CIMD fetcher to `internal/adapters/outboundhttp` and share its dial-time address guard with protected-resource discovery and registration. This location change supersedes ADR 015's package locations, not its CIMD timeout, response limit, cache, or validation behavior. Do not make an adapter import another adapter.

The new production transport validates public HTTPS URLs and blocks private resolved addresses before TCP connect. It disables redirects and proxy routing so its dialer checks the destination rather than a proxy. Discovery and registration share one 15-second attempt deadline. Each remote response has a 256-KiB limit. The transport used for discovery-backed code exchange and refresh has the same destination guard but keeps the configured token timeout and token response limits. Builder keeps its existing shared upstream client unchanged for manual services, proxy grants, and JWKS.

### Client selection and state

First use hosted CIMD only when the issuer advertises the document feature, `private_key_jwt`, and ES256, and the broker has a usable CIMD key. If this selected identity fails, do not try DCR. Otherwise, select confidential DCR with `client_secret_basic`, then `client_secret_post`, before public `none` with PKCE S256. If the issuer omits authentication-method metadata, use the RFC 8414 Basic default. A selected confidential registration never falls back to public registration.

For DCR, send the deployment-wide `third_party_oauth2.client_name` as `client_name`. Do not use the service display name. Reject DCR for an absent or blank broker client name before the registration request. Startup, manual services, and hosted CIMD remain available without that name. This configuration key must use the normal broker configuration port, all supported configuration sources, and the broker Helm chart.

Accept a DCR response only if its client ID, callback, authentication method, credential, and secret expiry meet the selected method's rules. Reject every non-zero `client_secret_expires_at`. Discard registration-management credentials. Store a confidential DCR secret with the existing service-scoped encryption port and exactly one `service_id` encryption-context subject. A public DCR client stores no secret. DCR client identity is `(issuer_uri, client_id)`; keep the existing manual-client uniqueness rule.

Extend the stored `TokenEndpointAuthMethod` with `client_secret_basic` and `client_secret_post` **only for discovery-backed DCR clients**. Pin the selected method for code exchange and refresh. This decision supersedes ADR 036 decision 2 for those clients only. Manual confidential clients retain automatic authentication-style detection. Manual public and hosted CIMD clients retain their existing explicit methods.

Persist one effective RFC 8707 `authorization_params.resource` and whether it is derived from the verified resource URL or set by an administrator. Send that value exactly once on authorization, code exchange, and refresh. Never retry without it. A discovery-backed token endpoint cannot contain a `resource` query parameter.

Store the latest attempt and last success on the existing service row. Commit a successful discovery and active configuration together. On a failed refresh, write only the safe failure code and attempt timestamp, guarded by the service version and attempt order. Do not change the previous issuer, credential, resource, sessions, success timestamp, or ETag. Keep this failure-only write in a separate, focused storage port with memory and PostgreSQL implementations. On update, keep the client method and the exact token authentication method. An issuer change requires explicit selection and zero user sessions.

## Consequences

The broker gains one guarded outbound boundary without changing the manual-service HTTP client. A remote DCR registration can remain orphaned if local persistence fails; no local usable service or credential remains. A provider can also reject a requested `resource`, so metadata success alone does not prove that an opaque token has the right audience. The broker must report safe failures instead of retrying with a broader request.

The new configuration key and Helm mapping are required by the latest feature specification. The earlier plan's no-new-key and no-chart-change statements must be corrected before implementation. Acceptance tests must cover the two additional client-name scenarios in the specification.

## Alternatives Considered

- Use the existing direct `metadata_url` helper for resource discovery. Rejected: it does not establish the resource's identity.
- Reuse the shared upstream HTTP client for discovery-backed token requests. Rejected: it follows redirects and lacks the dial-time address guard.
- Keep CIMD and discovery in separate outbound packages with duplicate address rules. Rejected: their security policies can diverge.
- Let the OAuth2 library detect the DCR client's authentication style. Rejected: it can send a second token request with another method.
- Store status separately from the active service row. Rejected: readers can observe a partial success.

## References

- [Feature specification](../specs/050-oauth2-protected-resource-discovery/spec.md)
- [Feature plan](../specs/050-oauth2-protected-resource-discovery/plan.md)
- [ADR 015](015-cimd-fetcher-architecture.md)
- [ADR 036](036-public-client-token-endpoint-auth.md)
- [ADR 037](037-cimd-client-authentication-key-domain.md)
- [ADR 004](004-storage-layer-architecture.md)
- [ADR 008](008-encryption-context-optimization.md)
