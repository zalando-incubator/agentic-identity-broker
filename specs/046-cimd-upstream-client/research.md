# Phase 0 Research: CIMD Client Authentication

## Scope

This feature lets the broker act as an OAuth2 client that uses `private_key_jwt`.
It does not change inbound CIMD client resolution.

All research questions from the technical context are resolved.

Increment 2 (issue #264) adds CIMD public services. Client registration now determines who supplies the client identifier, independently of token-endpoint authentication. The decisions that follow augment the original research. The stakeholder confirmed D-A through D-D under API-014 and accepted ADR 044, including D-E–D-J, on 2026-10-22.

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

Each successful response uses `application/json` and `Cache-Control: public, max-age=300`. A static public, static confidential, deleted, unknown, or unusable CIMD service returns the standard JSON `404` response. A CIMD public service returns metadata but its JWK Set returns `404`.

**Rationale:** The end-user router already owns anonymous protocol documents. The existing callback is deterministic at `/api/third-party/{service-id}/oauth2/callback`. The public URL is the configured source for both callback and client-identity construction.

**Alternatives considered:**

- Reuse the inbound `internal/domain/oauth2/cimd` document parser. Rejected. Inbound validation checks a remote client's document and keys; outbound publishing constructs a broker-owned document from persisted service state. Inbound validation also accepts `private_key_jwt` under ADR 043.
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

## Increment 2 (issue #264): CIMD public clients

Issue #264 approves `client_registration` as an independent service setting. It approves four combinations: `static` with an absent method or `none`, and `cimd` with `private_key_jwt` or `none`. Omission resolves to `cimd` only for `private_key_jwt`. Read and list responses report the resolved setting. Stored `private_key_jwt` services become `cimd`, and all other stored services become `static`. The broker rejects the other two combinations with `400` before outbound traffic. CIMD draft-02 §4.1 forbids shared-secret methods for CIMD. Its §8.2 recommends authentication when available, so `private_key_jwt` remains the recommended CIMD method.

### Decision: D-A — Reject explicit invalid registration (confirmed 2026-10-22)

**Decision:** The optional `client_registration` field is not nullable. Explicit `null`, an empty string, and an unknown value return `400` with `{"error":"validation failed","message":...}`.

**Rationale:** The `credential_source` precedent in ADR 041 distinguishes omission from an invalid explicit value. The Zalando rule equates null and absence only for nullable fields.

**Alternatives considered:** Treat null as omission. Rejected because it hides an explicit, invalid choice.

### Decision: D-B — Report discovery registration without selecting CIMD public (confirmed 2026-10-22)

**Decision:** A request with `discovery.resource_url` must omit `client_registration`. A supplied field returns `400` before discovery traffic. Hosted-CIMD selection records `cimd` and always uses `private_key_jwt`. DCR selection records `static` for `client_secret_basic`, `client_secret_post`, or `none`. `discovery.client_method` remains the authoritative discovery axis. ADR 040 selection does not change and never selects CIMD public.

**Rationale:** Discovery chooses the client method from issuer capabilities. A caller-supplied registration would conflict with that selection. The issue classifies every non-`private_key_jwt` stored service as `static`.

**Alternatives considered:** Let an explicit registration override discovery. Rejected because it could select a method that ADR 040 excludes.

### Decision: D-C — Publish keyless CIMD public metadata (confirmed 2026-10-22)

**Decision:** A CIMD public document uses the existing client identifier URL and returns `200`, `application/json`, and `Cache-Control: public, max-age=300`. It contains exactly `client_id`, `redirect_uris`, `grant_types`, `response_types`, and `token_endpoint_auth_method: none`. The one redirect URI is `<public_url>/api/third-party/<service-id>/oauth2/callback`. The grant types are `authorization_code` and `refresh_token`, and the response type is `code`. It omits `jwks_uri` and `token_endpoint_auth_signing_alg`. The document needs no CIMD key. Its `jwks.json` returns the existing uniform JSON `404`.

**Rationale:** A public client has no assertion verification key. The existing URL and cache contract still identify the broker-hosted client.

**Alternatives considered:** Publish an empty JWK Set or require a CIMD key. Rejected because neither belongs to public-client authentication.

### Decision: D-D — Version the additive API contracts (confirmed 2026-10-22)

**Decision:** Increase Admin OpenAPI `info.version` from `2.0.0` to `2.1.0`. Increase end-user OpenAPI `info.version` from `1.0.0` to `1.1.0`.

**Rationale:** Both contracts add representations without changing the existing paths. Principle X requires written stakeholder confirmation before implementation.

**Alternatives considered:** Keep both versions unchanged. Rejected because consumers need a visible contract revision.

### Decision: D-E — Exclude Google and filesystem from CIMD

**Decision:** Reject `client_registration: cimd` for the Google flavor and `credential_source: filesystem` with `400`. Explicit `static` remains valid for those services.

**Rationale:** Google and filesystem retain their existing identity and credential-source rules. A broker-assigned identifier cannot replace their configured identities.

**Alternatives considered:** Permit CIMD public with filesystem credentials. Rejected because public clients have no client credential.

### Decision: D-F — Verify every hosted identity at startup

**Decision:** Startup requires every persisted `cimd` service to have `client_id == <server.enduser.public_url>/.well-known/oauth-client/<service-id>`. A mismatch stops startup. Bootstrap and usable-key checks run only if at least one CIMD confidential service exists. CIMD public neither creates nor requires a CIMD client-authentication key.

**Rationale:** The identity check prevents a stored URL from naming a different document. Only confidential assertions require a key.

**Alternatives considered:** Bootstrap keys for every CIMD service. Rejected because public services never use assertions.

### Decision: D-G — Apply full-replacement transitions without authentication fallback

**Decision:** CIMD confidential and CIMD public transitions keep the client identifier URL. Switching to CIMD confidential requires a usable CIMD key. `static` to `cimd` replaces the operator client ID with the broker URL. The broker removes a stored secret only after full-replacement validation. `cimd` to `static` requires an operator `client_id` and, for static confidential, a non-empty `client_secret`. The former hosted document then returns `404`. Method switches are explicit, with no runtime fallback. Provider caches can retain the prior document for 300 seconds.

**Rationale:** The existing full-replacement semantics protect the prior state when validation fails. The established-identity check still applies only to shared-secret services. An old refresh token succeeds only if the provider accepts it with the new client ID. After a rejection, the user reconnects.

**Alternatives considered:** Fall back to another authentication method when the provider rejects a request. Rejected because this changes the requested trust boundary.

### Decision: D-H — Audit the public mode without duplicating session events

**Decision:** Lifecycle create, update, and rejection records for CIMD public use `CIMD public service`. Keep `CIMD confidential service` for confidential records. Keep the existing mode-neutral `CIMD metadata access` record for document serving. Records contain `service_id`, `operation`, and `outcome`, without credentials, tokens, assertions, or codes. Token acquisition uses existing session records with `public_client: true`.

**Rationale:** The metadata handler already records access for both modes. Another metadata event would duplicate one request.

**Alternatives considered:** Emit a second metadata record with a mode-specific message. Rejected because it duplicates the existing access event.

### Decision: D-I — Accept the fail-closed rolling-upgrade limitation

**Decision:** After migration 044, old replicas cannot create `private_key_jwt` services or rewrite `cimd` services to operator client IDs. PostgreSQL rejects these writes. Retry after all replicas upgrade.

**Rationale:** The column defaults to `static`, so old writers cannot supply `cimd`. The database constraint stops inconsistent state rather than silently changing identity.

**Alternatives considered:** Derive registration through a database trigger. Rejected because hidden write behavior complicates the service contract.

### Decision: D-J — Record the independent setting in Accepted ADR 044

**Decision:** Record `adrs/044-client-registration-independent-of-authentication.md` as Accepted by the stakeholder on 2026-10-22. It supersedes no ADR.

**Rationale:** ADR 036 public wire behavior, ADR 037 key separation, ADR 040 discovery selection, and ADR 041 filesystem limits remain intact.

**Alternatives considered:** Amend an Accepted ADR. Rejected because those decisions remain binding and do not conflict.

### Decision: Persist registration with two guarded migrations

**Decision:** Add `thirdparty_oauth2_services.client_registration TEXT NOT NULL DEFAULT 'static'`. Keep `chk_thirdparty_oauth2_services_client_registration` separate from the existing client-authentication constraint. Accept only `static` and `cimd`. Static rows cannot use `private_key_jwt` or discovery method `cimd`. CIMD rows require `private_key_jwt` or `none`, no encrypted secret, stored credentials, and no DCR method. Their `client_id` must end with `/.well-known/oauth-client/` plus the row ID.

Migration 044 uses a one-second statement timeout and `ACCESS EXCLUSIVE NOWAIT`. It adds the column and a `NOT VALID` constraint without scanning old rows. Its down migration refuses CIMD public rows before dropping the column and constraint. Convert or delete each such row, run `migrate force 44` to clear the dirty state, and retry. Migration 045 uses `SHARE UPDATE EXCLUSIVE NOWAIT`, backfills `cimd` for `private_key_jwt`, and validates the constraint. Its down migration restores `NOT VALID` without changing values.

**Rationale:** The suffix check is only a database guard. SQL cannot see `server.enduser.public_url`, so the domain and startup enforce the exact URL. Separate migrations limit exclusive-lock duration and preserve legacy classification. A malformed legacy CIMD URL stops validation and startup.

**Alternatives considered:** Use one migration with an exclusive-lock table scan. Rejected because it increases disruption. Derive registration with a trigger. Rejected because it hides writes and the rolling-upgrade failure.
