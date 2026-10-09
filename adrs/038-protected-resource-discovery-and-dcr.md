# ADR 038: Protected Resource Discovery and Dynamic Client Registration

**Status**: Proposed
**Date**: 2026-10-07
**Stakeholder approval**: The stakeholder approved the API and implementation approach on 2026-10-07, including the optional broker-wide DCR client name and Helm mapping. Reviewer acceptance of this ADR remains outstanding.

## Context

A third-party service can identify a protected resource without giving the broker an authorization-server URL or client credentials. The broker must obtain the resource's metadata, choose an advertised issuer, and create a client identity. Discovery and registration expose the broker to untrusted network destinations. A failed refresh must not replace a working client or its status history.

ADR 015 locates the CIMD fetcher in `internal/adapters/cimd` and its address policy in `internal/domain/oauth2/cimd`. ADR 036 allows only `none` as an explicit token-endpoint authentication method. Confidential DCR clients need explicit Basic or POST authentication without the existing automatic method probe. These changes need a decision before the adapter moves or new authentication methods become available.

## Decision

### Ownership and trust boundary

`ThirdpartyOAuth2ProviderService` owns discovery for the existing service aggregate. It validates the source, metadata identities, issuer selection, client method, effective resource, credential encryption, and state changes. The narrow `OAuthDiscoveryClient` port handles the resource probe, bounded metadata reads, and registration. The outbound HTTP adapter performs remote I/O. The Admin handler parses requests and formats safe results. Builder supplies the port. Direct `metadata_url` names an administrator-selected authorization-server metadata endpoint. It retains its older transport and metadata fallback behavior. Resource discovery instead follows remote-advertised metadata and issuer URLs through the guarded outbound client. Manual services keep their existing behavior.

Move the shared blocked-address value object to `internal/domain/netpolicy`. Move the CIMD fetcher to `internal/adapters/outboundhttp` and share its dial-time address guard with protected-resource discovery and registration. If reviewers accept this proposal, the location change supersedes only ADR 015's package locations. ADR 015's CIMD timeout, response limit, cache, and validation rules remain binding. Do not make an adapter import another adapter.

The guarded transport validates public HTTPS URLs and blocks private resolved addresses before TCP connect. It disables redirects and proxy routing so its dialer checks the destination rather than a proxy. Discovery and registration share one 15-second attempt deadline. Each remote response has a 256-KiB limit. Discovery-backed code exchange and refresh use the guarded transport with the configured token timeout and token response limits. Direct `metadata_url` and manual traffic retain the older shared client, its transport policy, and its fallback behavior. This older client is not the guarded discovery client.

### Client selection and state

First use hosted CIMD only when the issuer advertises the document feature, `private_key_jwt`, and ES256, and the broker has a usable CIMD key. If this selected identity fails, do not try DCR. Otherwise, select confidential DCR with `client_secret_basic`, then `client_secret_post`, before public `none` with PKCE S256. If the issuer omits authentication-method metadata, use the RFC 8414 Basic default. A selected confidential registration never falls back to public registration.

For DCR, send the deployment-wide `third_party_oauth2.client_name` as `client_name`. Do not use the service display name. Reject DCR for an absent or blank broker client name before the registration request. Startup, manual services, and hosted CIMD remain available without that name. This configuration key must use the normal broker configuration port, all supported configuration sources, and the broker Helm chart.

Accept a DCR response only if its client ID, callback, authentication method, credential, and secret expiry meet the selected method's rules. Reject every non-zero `client_secret_expires_at`. Discard registration-management credentials. Store a confidential DCR secret with the existing service-scoped encryption port and exactly one `service_id` encryption-context subject. A public DCR client stores no secret. DCR client identity is `(issuer_uri, client_id)`; keep the existing manual-client uniqueness rule.

Extend the stored `TokenEndpointAuthMethod` with `client_secret_basic` and `client_secret_post` **only for discovery-backed DCR clients**. Pin the selected method for code exchange and refresh. If reviewers accept this proposal, this extension supersedes ADR 036 decision 2 for those clients only. Manual confidential clients retain automatic authentication-style detection. Manual public and hosted CIMD clients retain their existing explicit methods. All other ADR 036 decisions remain binding.

Persist one effective RFC 8707 `authorization_params.resource` and whether the administrator pinned it or discovery derived it from the verified resource URL. Return `discovery.authorization_param_resource_strategy` as `pinned`, `derived`, or `null` for services without resource discovery. Send the effective audience exactly once on authorization, code exchange, and refresh. Never retry without it. A discovery-backed token endpoint cannot contain a `resource` query parameter.

Store the latest attempt and last success on the existing service row. Commit successful discovery and active configuration together. The service ETag versions active configuration and its owned routing resources. A failed discovery attempt updates only the separate logical status resource with a safe failure code and completion time. This failure-only write has no ETag and does not change the service version, active fields, or sessions. It requires the same service version and a completion time strictly later than the last attempt and last success at stored microsecond precision. A successful completion must also be strictly later than the latest committed attempt at that precision. If two outcomes complete in the same stored microsecond, the first committed outcome wins. On update, keep the client method and exact token authentication method.

Before an issuer change, require explicit selection and zero user sessions. Before a change to the effective audience, require zero user sessions, including expired sessions. Administrators must terminate existing sessions before an audience change. An explicit audience that stays unchanged permits a new discovery URL. An audience-change rejection returns `409` with `{"error":"conflict","message":"resource_change_requires_no_sessions"}`. A discovery-backed rejection records `resource_change_requires_no_sessions` as failure-only status. A discovery-to-manual rejection makes no status write. An issuer change retains its existing conflict precedence.

PostgreSQL locks the service row for final issuer and audience checks and for session insertion. Memory uses a shared gate. Both adapters count sessions before an issuer or audience switch and compare the session's expected issuer and audience under the matching lock or gate before insert or upsert. OAuth2 callback state seals the original issuer and effective audience. If either differs from the current service, the callback fails with invalid state before token exchange. A concurrent change after exchange cannot store a session for the old issuer or audience.

## Consequences

The broker gains one guarded outbound boundary for remotely advertised discovery destinations without changing the older direct-`metadata_url` or manual-service HTTP paths. A remote DCR registration can remain orphaned if local persistence fails. No local usable service or credential remains. A provider can reject a requested `resource`, so metadata success alone does not prove an opaque token has the right audience. The broker reports safe failures instead of retrying with a broader request.

Strict stored-microsecond ordering prevents an older successful refresh from clearing a later committed failure, even when the service version stays unchanged. A tie retains the first committed outcome. The authenticated Admin status GET reads persisted state without decryption or provider traffic. Status has no ETag. It reports the latest completed outcome and retains the prior success and active fields after failure. Manual services return `not_applicable` with null discovery fields.

Helm `broker.thirdPartyOauth2.clientName` maps to the optional `third_party_oauth2.client_name` key. DCR setup requires a non-blank value. Startup, manual services, and hosted CIMD do not require this key.

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
