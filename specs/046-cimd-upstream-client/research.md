# Phase 0 Research: CIMD Client Authentication

## Scope

This feature lets the broker act as an OAuth2 client that uses `private_key_jwt`.
It does not change inbound CIMD client resolution.

All research questions from the technical context are resolved.

## Decision: Model three explicit service authentication modes

**Decision:** Add `private_key_jwt` to `TokenEndpointAuthMethod`.
Keep the three modes explicit:

- Omitted or `null`: Static confidential client with an encrypted shared secret.
- `none`: Public client with an operator-supplied client ID and no secret.
- `private_key_jwt`: CIMD confidential client with a broker-assigned client ID URL and no secret.

Persist the generated CIMD URL in the existing service `client_id` field.
The service ID makes this URL stable across ordinary service updates and key rotation.

**Rationale:** `ThirdpartyOAuth2ProviderEntity` already owns `ClientID`, `Secret`, and the authentication method. The existing `client_id` database column persists the identity presented to a token endpoint. A second identity column would duplicate one concept.

**Alternatives considered:**

- Infer the mode from a missing secret. Rejected. An omitted method already means static confidential authentication.
- Store the CIMD URL in a separate service column. Rejected. The existing `client_id` is the durable upstream identity.
- Reuse `none` for key-based authentication. Rejected. A public client and a confidential `private_key_jwt` client have different security contracts.

**Evidence:** `internal/domain/model/token_endpoint_auth_method.go`, `internal/domain/model/thirdparty_oauth2_provider.go`, `internal/domain/thirdparty/service.go`, `internal/adapters/http/handlers/admin/services_handler.go`, and `migrations/031_add_token_endpoint_auth_method.up.sql`.

## Decision: Use a domain-scoped CIMD key set

**Decision:** Extend the existing `signing_keys` persistence model with a required key-domain discriminator.
Use `token_signing` for existing local-token keys and `cimd_client_authentication` for CIMD keys.
Retain a globally unique `kid` and replace the current-key index with a per-domain partial unique index.

Create a dedicated CIMD key manager and a narrow assertion-signer port. Do not give outbound session code the token-signing manager or private key bytes.

Give CIMD private keys a distinct encryption and branch-key subject namespace. Keep one AAD subject key per the encryption design. The shared table keeps `kid` globally unique across key domains.

**Rationale:** The current lifecycle already provides ES256 generation, encrypted private storage, activation grace, immediate promotion, explicit removal, and bootstrap locking. A domain discriminator reuses these tested mechanics while enforcing independent current-key invariants. One table also gives a database-level global `kid` uniqueness guarantee.

**Alternatives considered:**

- Reuse the unscoped token-signing manager. Rejected. It permits accidental client assertions with token keys and can leak CIMD keys into `/oauth2/jwks.json`.
- Create an independent key table without a shared `kid` namespace. Rejected. It cannot prove that published key IDs never overlap.
- Create per-service key sets. Rejected. The approved feature requires one broker-global CIMD key set.

**Evidence:** `internal/domain/oauth2server/signing_key_service.go`, `internal/ports/oauth2server.go`, `internal/ports/storage.go`, `migrations/011_create_signing_keys.up.sql`, `migrations/021_enforce_single_current_signing_key.up.sql`, `internal/domain/encryption/subject.go`, and `internal/adapters/encryption/branchkey/id.go`.

## Decision: Keep token and CIMD JWK Sets separate

**Decision:** Keep `/oauth2/jwks.json` unchanged. It publishes only the broker token-signing and upstream verification key sources that apply to the selected OAuth server mode.

Add a dedicated CIMD public-key publisher. It uses only the CIMD key manager and serves only `/.well-known/oauth-client/{service-id}/jwks.json`.

**Rationale:** Proxy mode publishes upstream verification keys. Hybrid mode aggregates upstream and local token keys. Neither surface is valid for outbound CIMD client authentication.

**Alternatives considered:**

- Add CIMD keys to the aggregate JWK Set. Rejected. It violates the key-domain separation requirement.
- Alias the CIMD JWK URL to `/oauth2/jwks.json`. Rejected. The set is mode-dependent and can expose unrelated keys.

**Evidence:** `internal/domain/oauth2/jwks_publisher.go`, `internal/adapters/http/handlers/enduser/jwks_handler.go`, `internal/app/builder.go`, and `tests/e2e/aggregated_jwks_test.go`.

## Decision: Serve outbound metadata from dedicated public handlers

**Decision:** Add anonymous handlers under the existing `/.well-known` router:

- `GET /.well-known/oauth-client/{service-id}`
- `GET /.well-known/oauth-client/{service-id}/jwks.json`

Derive every URL from `server.enduser.public_url`, not from the request host or the admin URL. Require HTTPS before registration or use of a CIMD confidential service.

Each successful response uses `application/json` and `Cache-Control: public, max-age=300`. A public, static, deleted, unknown, or unusable CIMD service returns the standard JSON `404` response.

**Rationale:** The end-user router already owns anonymous protocol documents. The existing callback is deterministic at `/api/third-party/{service-id}/oauth2/callback`. The public URL is the configured source for both callback and client-identity construction.

**Alternatives considered:**

- Reuse the inbound `internal/domain/oauth2/cimd` document parser. Rejected. It deliberately accepts only public-client metadata and rejects `private_key_jwt`.
- Build URLs from request headers. Rejected. A hostile host header can change the client identity.
- Put the public documents under `/api`. Rejected. They are anonymous discovery documents, not administrative APIs.

**Evidence:** `internal/adapters/http/routing/enduser.go`, `internal/adapters/http/enduser/oauth2_metadata.go`, `internal/domain/oauth2session/service.go`, `internal/ports/config.go`, `internal/config/validator.go`, and `internal/domain/oauth2/cimd/document.go`.

## Decision: Sign a fresh assertion for every private token request

**Decision:** Add a narrow capability equivalent to `SignClientAssertion(ctx, clientID, tokenEndpoint)`.
It selects the current usable CIMD key and returns an ES256 JWT.

The JWT has:

- `iss` and `sub` equal to the exact broker-hosted client ID URL.
- One `aud` value equal to the configured token endpoint.
- An expiry no more than five minutes after issue time.
- A new `jti` for each token request attempt.
- The `kid` of an advertised CIMD key with `use: sig`.

For authorization-code exchange, use `oauth2.AuthStyleInParams`, an empty client secret, and one assertion pair added inside the existing retry loop.
For refresh, retain the manual form-post path and add one fresh assertion pair.

**Rationale:** `golang.org/x/oauth2` has no `private_key_jwt` auth style. Its code-exchange options can add form fields. Its refresh token source cannot create a fresh assertion. The current manual refresh form is the correct controlled path.

**Alternatives considered:**

- Use `AuthStyleAutoDetect`. Rejected. It tries Basic authentication and then body credentials.
- Put the assertion in `ClientSecret`. Rejected. It is the wrong protocol field and can create a secret fallback.
- Use `oauth2.Config.TokenSource` for refresh. Rejected. It does not provide an assertion hook.
- Replace all code exchange logic with manual HTTP. Rejected. It would duplicate established PKCE and token-response behavior.

**Evidence:** `internal/domain/oauth2session/service.go`, `internal/domain/oauth2session/service_security_test.go`, `internal/domain/oauth2server/strategies.go`, `go.mod`, and the JWX v4 project guidance.

## Decision: Make key lifecycle mode-independent and fail closed

**Decision:** Wire the CIMD key manager, assertion signer, public-document service, and CIMD key admin handler in `proxy`, `local`, and `hybrid` modes.
Keep the inbound `oauth2_authorization_server.cimd.enabled` restriction in proxy mode unchanged.

At startup, inspect persisted CIMD confidential services. If one exists, bootstrap or require a usable CIMD key. A bootstrap key becomes usable immediately. If this operation fails, startup fails closed.

If no CIMD confidential service exists, startup continues. A later registration or use fails until a usable CIMD key and its public JWK Set are available.

**Rationale:** Third-party session acquisition is independent of the broker authorization-server mode. The builder creates `OAuth2SessionService` before its mode-specific token strategy wiring.

**Alternatives considered:**

- Tie outbound CIMD to the inbound CIMD feature switch. Rejected. Inbound CIMD is intentionally unavailable in proxy mode.
- Use the proxy upstream key set. Rejected. It is a separate trust relationship.
- Fall back to public or static authentication after a key or provider error. Rejected. The feature must fail closed.

**Evidence:** `internal/app/builder.go`, `internal/ports/config.go`, `internal/domain/oauth2server/signing_key_service.go`, `internal/adapters/http/routing/admin.go`, and `internal/adapters/http/routing/enduser.go`.

## Decision: Use a conformant private-key JWT test double

**Decision:** Add a per-test third-party provider double for CIMD confidential authentication. It fetches broker metadata and the service JWK Set, caches keys for rotation tests, validates ES256 assertions, enforces PKCE, and rejects secret authentication.

Keep the shared `MockUpstreamOAuth2Server` for public and static compatibility tests.

**Rationale:** The shared test server is read-only across E2E workers. It captures public-client behavior but does not fetch metadata or validate assertions.

**Alternatives considered:**

- Add private-key state to the shared upstream mock. Rejected. It leaks state between scenarios and obscures the protocol oracle.
- Use the standalone third-party mock. Rejected. It assumes static credentials and logs credential fields.
- Use only unit tests. Rejected. The feature needs a complete connect and refresh journey against an independent provider validator.

**Evidence:** `tests/e2e/e2e_suite_test.go`, `tests/e2e/helpers/mock_upstream.go`, `tests/e2e/thirdparty_public_client_test.go`, `tests/e2e/bootstrap/cimd.go`, and `tests/e2e/oauth2_signing_keys_e2e_test.go`.

## Decision: Update approved API and operator documentation first

**Decision:** Update both OpenAPI specifications before implementation. Add the service authentication semantics and key routes to `api/admin/openapi.yaml`. Add the anonymous metadata and JWK routes to `api/enduser/openapi.yaml`.

Update `docs/reference/api.md`, `docs/guides/manage-agents-and-services.md`, `docs/guides/operate-oauth2-server-modes.md`, and the affected `ARCHITECTURE.md` route and glossary sections.

**Rationale:** The specification records stakeholder approval for all new external paths and representations. Redocusaurus reads the OpenAPI specifications at documentation build time.

**Alternatives considered:**

- Document only the administrative routes. Rejected. The third-party authorization server consumes the public metadata and JWK contracts.
- Create static generated API pages. Rejected. The site reads the current OpenAPI input at build time.

**Evidence:** `api/admin/openapi.yaml`, `api/enduser/openapi.yaml`, `assets/docusaurus/docusaurus.config.js`, `docs/reference/api.md`, and `docs/guides/operate-oauth2-server-modes.md`.
