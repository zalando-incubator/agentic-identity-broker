# Phase 0 Research: Protected Resource Discovery

## Scope and sources

This feature extends third-party OAuth2 services. It does not change inbound CIMD, the broker authorization server, protected-resource ownership, or the React UI.

The decisions use [spec.md](./spec.md), [ADR 004](../../adrs/004-storage-layer-architecture.md), [ADR 008](../../adrs/008-encryption-context-optimization.md), [ADR 012](../../adrs/012-encryption-layer-separation.md), [ADR 015](../../adrs/015-cimd-fetcher-architecture.md), [ADR 036](../../adrs/036-public-client-token-endpoint-auth.md), [ADR 037](../../adrs/037-cimd-client-authentication-key-domain.md), [RFC 9728](https://www.rfc-editor.org/rfc/rfc9728), [RFC 8414](https://www.rfc-editor.org/rfc/rfc8414), [RFC 7591](https://www.rfc-editor.org/rfc/rfc7591), and [RFC 8707](https://www.rfc-editor.org/rfc/rfc8707). No technical-context question remains unresolved.

## Decision: Keep two distinct discovery sources

**Decision:** Add `DiscoveryConfig.ResourceURL` for protected-resource discovery. Keep `MetadataURL` for direct authorization-server metadata discovery. Require `enable_discovery: true` with `resource_url`. Reject a request that combines this source with `metadata_url`, manual endpoints, or caller-supplied client credentials. Preserve existing manual and direct-metadata behavior when `resource_url` is absent.

**Rationale:** The resource URL identifies the party that advertises trusted issuers. An authorization-server metadata URL does not establish a protected resource's identity. The current handler permits manual fallback after metadata failure, which is forbidden for the new path.

**Alternatives considered:** Reuse `metadata_url` for protected-resource metadata. Rejected because it changes that field's meaning. Apply the current manual fallback to the new path. Rejected because invalid metadata would become a usable service.

**Evidence:** `internal/domain/model/discovery_config.go`; `internal/adapters/http/handlers/admin/services_handler.go`; spec FR-001–FR-004 and FR-014–FR-015.

## Decision: Follow the exact metadata probe and trust order

**Decision:** Make one bounded attempt for resource challenge, resource metadata, issuer metadata, and DCR. Prefer a validated `WWW-Authenticate` `resource_metadata` URL when present. Otherwise try the path-specific RFC 9728 URL, then the root URL only after a 404. For an issuer with a path, try path-inserted OAuth metadata, path-inserted OpenID metadata, then path-appended OpenID metadata. For a root issuer, try root OAuth metadata, then root OpenID metadata. Continue only after 404. Match the resource and issuer identifiers exactly. Require authorization and token endpoints.
Only one `resource_metadata` value in a `401` Bearer or DPoP challenge is eligible. Duplicates, invalid challenges with that value, and values on other statuses fail without a well-known fallback.

**Rationale:** The declared resource and issuer are separate trust anchors. A non-404 response that has invalid metadata cannot authorize a lower-priority document. Strict identity checks prevent issuer and resource substitution.

**Alternatives considered:** Probe all locations until one parses. Rejected because malformed or mismatched higher-priority metadata must fail closed. Select the first advertised issuer. Rejected because a multi-issuer resource requires an administrator choice.

**Evidence:** Spec FR-003–FR-005 and US1-S4–S9; RFC 9728 sections 3 and 5; RFC 8414 section 3; `internal/domain/storage/discovery.go` implements only a single direct issuer probe today.

## Decision: Put orchestration in the existing third-party domain service

**Decision:** Add narrow outbound metadata and registration ports. Implement remote HTTP work in a driven adapter. Let `ThirdpartyOAuth2ProviderService` choose the issuer, bootstrap method, effective resource, and persistence operation. Keep the Admin handler limited to request parsing, service calls, and safe responses. Builder constructs the adapter and injects its ports.

**Rationale:** The current handler calls `storage.DiscoverOAuth2Endpoints` directly and contains fallback logic. That location cannot enforce the new aggregate invariants consistently on create and update. The existing service already owns validation, secret encryption, and storage.

**Alternatives considered:** Add more HTTP calls to `ServicesHandler`. Rejected because it leaks security policy into an inbound adapter. Put remote I/O in `internal/domain/storage`. Rejected because domain packages must not own HTTP infrastructure.

**Evidence:** `internal/domain/thirdparty/service.go`; `internal/domain/storage/discovery.go`; `internal/app/builder.go`; `internal/adapters/AGENTS.md`.

## Decision: Apply one SSRF-safe transport to discovery, registration, and discovered token calls

**Decision:** Move the blocked-address value object from `internal/domain/oauth2/cimd` to `internal/domain/netpolicy`. Move the CIMD fetcher into a new `internal/adapters/outboundhttp` package. Put the discovery and registration client in the same package so that both clients use one dial-time guard. Keep CIMD-specific parsing and caching separate. Validate public HTTPS before each remote request. Block disallowed resolved IPs in `net.Dialer.Control` before TCP connect. Disable redirects, cap each fetched document or registration response at 256 KiB, and apply one 15-second deadline to the whole discovery and registration attempt. Use a guarded token HTTP client for discovery-backed code exchange and refresh. Retain manual-service wire behavior. Inject a TLS fake-public-host transport only in tests. Do not add an insecure production flag.

Main now constructs one shared `upstreamClient` in Builder for existing session, proxy, and JWKS calls. Its cloned default transport has no dial-time IP guard, and its client follows redirects. Keep it for existing flows. Builder must construct one separate guarded transport for discovery, registration, and discovery-backed token requests. Do not mutate the shared client's transport or redirect policy. Preserve the configured upstream timeout, optional OTel instrumentation, and idle-connection shutdown for both transport lifecycles.

The guarded transport must disable proxy routing, including `HTTPS_PROXY`. Otherwise, `net.Dialer.Control` sees the proxy's IP instead of the resource's resolved IP. A proxy could then reach a private destination. The existing shared upstream transport keeps its current proxy behavior.

The 15-second attempt deadline and 256-KiB response limit apply to discovery and registration, not token exchange or refresh. Token calls keep the configured upstream timeout and their own response limits. CIMD retains its separate fetch timeout and body limit. It shares the blocked-address policy and dial-time guard code, not necessarily the same transport instance.

The E2E bootstrap wraps a fake-host HTTP client in the outbound discovery adapter and injects that port. It also injects the same client for discovery-backed code exchange and refresh. URL validation, no-redirect policy, and document limits remain in the client layer. The test client skips `net.Dialer.Control` so it can reach loopback TLS servers. Production Builder instances never set either option. US3-S3 tests private-address blocking with the production dialer.

**Rationale:** The existing issuer discovery helper has no IP guard or body cap and follows redirects. Main now injects the shared `upstreamClient` into `OAuth2SessionService`: code exchange uses it through `oauth2.HTTPClient`, and refresh calls it directly. That client has no resolved-IP guard and permits redirects. Reusing it for discovery-backed token calls would expose those calls to private destinations. The existing CIMD adapter cannot be imported from a sibling adapter.

**Alternatives considered:** Reuse or alter the shared `upstreamClient`. Rejected because it lacks SSRF protection, and changing it would affect manual sessions, proxy grants, and JWKS. Reuse the CIMD adapter directly. Rejected by the cross-adapter rule. Copy its security code. Rejected because two blocklists can drift. Use `skip_thirdparty_https_validation` for all new requests. Rejected because it does not block private IPs or redirects.

**Evidence:** `internal/app/builder.go` constructs and closes the shared transport. `internal/domain/oauth2session/service.go` uses it for code exchange and refresh. `internal/adapters/cimd/fetcher.go` applies the dial-time guard. `internal/domain/oauth2/cimd/blocklist.go` defines blocked addresses. `internal/domain/storage/discovery.go` uses the direct issuer helper. [ADR 015](../../adrs/015-cimd-fetcher-architecture.md) binds the existing CIMD implementation until superseded.

## Decision: Prefer hosted CIMD, then explicit confidential DCR, then public DCR

**Decision:** Choose hosted CIMD only when AS metadata advertises Client ID Metadata Documents, `private_key_jwt`, and ES256. Require an available broker-hosted identity and published signing key before selecting it. Do not call DCR after this selection fails. Otherwise, require a `registration_endpoint` for DCR. Choose `client_secret_basic` before `client_secret_post` when advertised. Use `none` only when no supported confidential method is offered and metadata advertises PKCE `S256`. If AS metadata omits authentication methods, apply RFC 8414's `client_secret_basic` default. Send the actual callback, `application_type: web`, the service display name, `authorization_code`, `refresh_token`, `code`, and the selected method. Accept a response without a refresh grant, but do not assume a refresh token. Verify the returned client ID, callback, and selected authentication mode. Discard unused registration-management credentials.
Reject a non-zero `client_secret_expires_at` even when it lies in the future. Without rotation, an expiring secret would break both new code exchanges and refreshes. The broker accepts omitted or zero expiry and never re-registers a client automatically.

**Rationale:** Hosted CIMD avoids a second registration. A fixed DCR method prevents OAuth library auto-detection from probing another method after selection. A public client has no client secret, so it needs explicit PKCE support. The MCP client registration section requires an application type for DCR and prefers CIMD before DCR. The specification forbids a public retry after a confidential registration failure.

**Alternatives considered:** Use `AuthStyleAutoDetect` for DCR secrets. Rejected because it can try another method. Retry public DCR on confidential failure. Rejected by FR-008 and SR-004. Add a separate CIMD public mode. Rejected because feature 046 provides only confidential CIMD.

**Evidence:** `internal/domain/model/token_endpoint_auth_method.go`; `internal/domain/cimdclient/assertion_signer.go`; `internal/domain/oauth2session/service.go`; RFC 8414 metadata defaults; RFC 7591 sections 2–3; spec FR-007–FR-010. [ADR 036](../../adrs/036-public-client-token-endpoint-auth.md) explicitly excludes basic/post. An accepted superseding ADR is required before this DCR-only extension is implemented.

## Decision: Make DCR identity issuer-scoped and service-owned

**Decision:** Store `client_method = dcr`, the selected issuer, returned client ID, and explicit token authentication method on the service. Encrypt a confidential DCR secret with the existing service-scoped `EncryptionPort`. A public DCR service stores no secret. Add a partial unique PostgreSQL index on `(issuer_uri, client_id)` for DCR rows only. Enforce the same rule in memory. Keep manual-service uniqueness behavior unchanged. Load credentials by service ID and verify the stored issuer at token time.

**Rationale:** The same client ID can exist at different issuers. The service already owns its credential and the session flow fetches by service ID. No separate registration entity or credential table is required.

**Alternatives considered:** Make `client_id` globally unique. Rejected because it blocks valid different-issuer identities and changes manual behavior. Store a second DCR credential record keyed only by client ID. Rejected because it risks cross-issuer credential selection.

**Evidence:** `internal/domain/thirdparty/service.go`; `internal/domain/oauth2session/service.go`; `migrations/002_create_thirdparty_services.up.sql`; `migrations/035_add_cimd_private_key_jwt_authentication.up.sql`; spec FR-018 and SR-006.

## Decision: Persist one effective resource and its source

**Decision:** Materialize `authorization_params.resource` during discovery-backed create. Store a boolean source marker that identifies an administrator override. Validate an explicit value as a non-empty absolute URI without a fragment. A changed verified resource URL replaces a derived value. An unchanged explicit override survives refresh. An update with a replacement `authorization_params` object that omits `resource` clears the override and restores the verified URL. An omitted object retains the prior source and value. Reject a discovery-backed token endpoint whose URL already has a `resource` query parameter.

**Rationale:** The current parameter map already reaches authorization, code exchange, and refresh with `url.Values.Set`. The map alone cannot distinguish a derived value from an override. A query-level `resource` on the token endpoint can create a second value beside the form field.

**Alternatives considered:** Derive `resource` at request time. Rejected because reads and restarts must show the persisted effective value. Keep only the map. Rejected because refresh cannot distinguish an override from a former derived value.

**Evidence:** `internal/domain/oauth2session/service.go`; `internal/domain/model/thirdparty_oauth2_provider.go`; `internal/adapters/storage/{memory,postgres}/thirdparty_provider.go`; RFC 8707 section 2; spec FR-011 and FR-017.

## Decision: Make a successful refresh atomic and record failures separately

**Decision:** Store active discovery fields and status timestamps on the existing service row. On success, write active fields, encrypted credentials when changed, and `last_attempt_at`/`last_success_at` in one service-row transaction. On failure, update only `last_attempt_at` and a sanitized `failure_reason` through one failure-status repository operation. Leave active fields, success time, sessions, and service version unchanged. Guard both writes against stale versions or newer attempts. A manual service has no discovery timestamps and returns `not_applicable`.

**Rationale:** One row avoids a second commit boundary between active configuration and success status. Remote registration and KMS work happen before the local transaction. A remote registration can remain orphaned if local persistence fails; the specification accepts that limit.

**Alternatives considered:** Persist success status in a separate transaction. Rejected because readers could see a partial configuration. Rewrite the active service on failed refresh. Rejected because it could replace a working client or invalidate an ETag.

**Evidence:** `internal/adapters/storage/postgres/thirdparty_provider.go`; `internal/adapters/storage/memory/thirdparty_provider.go`; spec DB-003–DB-005 and US4-S1–S5.

## Decision: Keep refresh method and issuer stable unless an issuer change is explicit

**Decision:** For a discovery-backed update, prefer the active issuer and client method while they remain valid. Preserve the client identity and DCR secret when both remain unchanged. Permit a different issuer only when the administrator names it in `issuer_uri`, the resource advertises it, and the service has no user sessions. Keep the client method. Keep the hosted CIMD ID, or register a new DCR client at the new issuer before an atomic replacement. A client-method change has no request selector in this feature, so reject it. Create a new service to choose a different method. Never silently switch issuer or downgrade the client method.
The exact `token_endpoint_auth_method` also remains fixed, including Basic versus POST and confidential versus public DCR. For an explicit issuer change, require the new issuer to support that saved method. Reject any other result without changing the active service; do not re-run creation preference.

**Rationale:** An update must not silently invalidate connected users. A stored refresh token belongs to its old issuer and must not reach a new issuer. The session check happens before network access and uses the existing service-session relationship. The API has an explicit issuer field but no explicit client-method request field. A consistent method avoids unrequested DCR and public fallback on refresh.

**Alternatives considered:** Reapply creation preference on every update. Rejected because a newly advertised method could silently change the client. Silently choose another issuer on failure. Rejected by FR-013 and FR-014.

**Evidence:** Spec FR-013–FR-014 and US4-S4–S8; `internal/adapters/http/handlers/admin/services_handler.go` uses a full-replacement PUT.

## Decision: Return durable, credential-free status

**Decision:** Add authenticated `GET /api/services/{service-id}/discovery-status`. It reads stored state only. Return `ready` after a successful latest attempt, `failed` after a failed refresh, and `not_applicable` for a manual service. Return null for absent fields. On a failed refresh, report the active resource URL, issuer, and client method with the latest attempt time, prior success time, and safe reason. A deleted or unknown service returns 404.

**Rationale:** Administrators need both the last attempt and the last working configuration after restart. The current Admin API uses `PreAuthProxy` and `{error,message}`; no new public End-user endpoint is needed.

**Alternatives considered:** Re-run discovery on status GET. Rejected because reads must not contact the provider or alter state. Return raw upstream error bodies. Rejected because they can contain credentials and untrusted text.

**Evidence:** `api/admin/openapi.yaml`; `internal/adapters/http/routing/admin.go`; spec API-003–API-007 and SR-003–SR-005.

## Decision: Prove every scenario through production bootstrap

**Decision:** Add 39 Ginkgo `It()` blocks with one `USx-Sy` identifier each. Use both Admin and End-user test servers from one `app.Builder`. Add a per-test, HTTPS-capable resource/issuer/DCR provider double with request-order and registration counts. Use PostgreSQL testcontainers for real restart and migration cases. Measure SC-006 separately with 20 isolated registrations and a single Ginkgo worker.
Write the SC-006 performance block before production behavior, run it in the semantic-red gate, and re-run the unchanged assertion after implementation.

**Rationale:** The current upstream mock captures authorization and token forms but has no protected-resource metadata, DCR, or status surface. In-memory adapter reuse across two app instances does not prove disk durability.

**Alternatives considered:** Use only unit tests or the standalone plain-HTTP mock. Rejected because neither proves the full Admin → provider → End-user → persistence journey. Use a global insecure SSRF bypass for tests. Rejected because it would not test the production protection.

**Evidence:** `tests/e2e/thirdparty_public_client_test.go`; `tests/e2e/thirdparty_cimd_authentication_test.go`; `tests/e2e/provider_authorization_params_test.go`; `tests/e2e/bootstrap/cimd.go`; `tests/e2e/bootstrap/postgres.go`.

## Decision: Keep RFC 9207 callback validation outside this feature

**Decision:** Do not add new authorization-response `iss` validation in this feature. Keep the existing per-service callback URL, JWE-sealed state, and PKCE verifier binding. Record the validated issuer on the service so that a later feature can add RFC 9207 validation without another discovery change.

**Rationale:** The specification defines protected-resource discovery, client bootstrap, resource binding, and status. It has no acceptance scenario for authorization-response issuer validation. The current callback is service-specific, so one service response cannot complete another service's flow.

**Alternatives considered:** Add RFC 9207 validation now. Rejected for this plan because it changes callback behavior for every discovery-backed provider and needs its own acceptance scenarios. Ignore issuer identity. Rejected because discovery must persist the validated issuer for credential scope and audit records.

**Evidence:** [MCP authorization specification](https://modelcontextprotocol.io/specification/2026-07-28/basic/authorization); `internal/adapters/http/oauth2_sessions/handler.go`; `internal/domain/oauth2session/service.go`.
