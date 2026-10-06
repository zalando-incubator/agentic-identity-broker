---
description: "Task list for protected-resource OAuth2 discovery"
---

# Tasks: Protected Resource Discovery for Third-Party OAuth2 Services

**Input**: `specs/050-oauth2-protected-resource-discovery/{spec.md,plan.md,research.md,data-model.md,quickstart.md,contracts/}`.

**Tests**: Constitution Principles VIII and XIII require test-first development and one Ginkgo `It()` per acceptance scenario. Write all 39 functional cases and the separate SC-006 performance case before feature behavior. Each red test must compile and fail on an observable result, not a broken fixture or placeholder assertion. Keep manual and direct-`metadata_url` behavior.

**Order**: Complete Phase 0 as a separate behavior-neutral PR with matching `ARCHITECTURE.md` updates. Complete Phase 2, including API review, the Builder test seam, and the 39-case plus SC-006 semantic-red gates, before Phase 2.5 or any user story. The feature extends the existing service aggregate. Do not add a new entity, table, frontend, or End-user route.

## Phase 0: Pre-implementation Refactoring

**Goal**: Share the existing CIMD outbound safety boundary without changing CIMD behavior. Submit this phase as a separate PR before feature code.

- [ ] T001 Draft and obtain acceptance of `adrs/038-protected-resource-discovery-and-dcr.md` before relocating the ADR 015 adapter; record the new outbound port boundary and supersede ADR 036 decision 2 only for DCR-issued `client_secret_basic` and `client_secret_post`.
- [ ] T002 Relocate `internal/domain/oauth2/cimd/blocklist.go` and its tests to `internal/domain/netpolicy/`; migrate imports without aliases or behavior changes. Retain special-range and operator-CIDR coverage. In the same Phase 0 PR, update the "New Domain Packages" paragraph in the CIMD Subsystem and the `SSRFBlocklist` glossary entry in `ARCHITECTURE.md`. Name `internal/domain/netpolicy/` as the new location.
- [ ] T003 Relocate `internal/adapters/cimd/fetcher.go` and its tests to `internal/adapters/outboundhttp/`; migrate imports and Builder construction without behavior changes. Remove the old adapter package. In the same Phase 0 PR, update the `CIMDFetcher` glossary entry in `ARCHITECTURE.md` to name `internal/adapters/outboundhttp/`.
- [ ] T004 Run `just verify` for the Phase 0 refactor. Confirm that `ARCHITECTURE.md` names both new packages and no longer places the blocklist or fetcher in their old packages. Submit these code and documentation changes together, with no protected-resource behavior.

**Checkpoint**: The existing CIMD scenarios still pass. ADR 038 is accepted before the shared adapter changes.

## Phase 1: Setup

**Goal**: Prepare isolated acceptance fixtures and compile-only discovery-port and token-client injection seams. Do not implement discovery behavior in this phase.

- [ ] T005 [P] Add a per-scenario TLS provider double in `tests/e2e/helpers/mock_protected_resource_provider.go` for challenges, both RFC 9728 locations, root/path issuer metadata, DCR, authorization, and token endpoints; capture request order, DCR count, PKCE, resource parameters, and auth forms without capturing secrets in logs.
- [ ] T006 [P] Define compile-only `ports.OAuthDiscoveryClient` probe/GET-JSON/POST-JSON signatures in `internal/ports/oauth_discovery.go`. Add `Builder.WithOAuthDiscoveryClient(ports.OAuthDiscoveryClient)` and `Builder.WithDiscoveryTokenHTTPClient(*http.Client)` in `internal/app/builder.go` before T024. Add the minimal token-client setter in `internal/domain/oauth2session/service.go` and a compile-only `outboundhttp.NewDiscoveryClientWithClient` constructor. In `tests/e2e/bootstrap/protected_resource.go`, wrap one per-scenario fake-host TLS client in the outbound discovery adapter and pass that port and client to Builder. The fake client maps only synthetic public hosts to TLS test servers and rejects other hosts. It skips dial-time IP blocking to reach loopback, so it must never be selected through runtime configuration. Do not change the default production dialer or manual token calls.
- [ ] T007 [P] Add isolated CIMD-only, CIMD-plus-DCR, confidential/public DCR, multi-issuer, and rejection configurations in `tests/e2e/fixtures/protected_resource.go`; keep provider state and service IDs independent per `It()`.

## Phase 2: Design Preconditions (Blocking)

**Goal**: Complete the binding domain, configuration, API, database, and E2E preconditions before behavior changes.

### Phase 2a: Domain Model and Glossary (Principles II and V)

- [ ] T008 Add Protected Resource Metadata, Authorization Server Metadata, Client bootstrap method, DCR client identity, Effective resource, and Discovery status to the glossary in `ARCHITECTURE.md`; distinguish discovery URL, token audience, and `protected_resources` ownership.
- [ ] T009 Document the trust boundary, probe order, failure isolation, DCR method order, and one-resource invariant in `ARCHITECTURE.md`. Record SC-006: at least 19 of 20 registrations finish within five seconds when each external response takes at most one second. Record the 15-second/256-KiB security limits. Link accepted `adrs/038-protected-resource-discovery-and-dcr.md` without changing manual-service behavior.
- [ ] T010 Compare the aggregate, owned status value, states, and issuer-change-with-sessions rule in `specs/050-oauth2-protected-resource-discovery/data-model.md` with accepted `adrs/038-protected-resource-discovery-and-dcr.md`; resolve any mismatch there before implementation.

### Phase 2b: Configuration Design (Principle VII)

- [ ] T011 [P] Document that discovery adds no runtime key, has fixed 15-second/256-KiB limits, and requires an HTTPS `server.enduser.public_url` for hosted CIMD in `docs/configuration.md`.
- [ ] T012 [P] Add a protected-resource setup example using existing configuration keys in `examples/config/protected-resource-discovery.yaml`; include the HTTPS hosted-CIMD public URL and no insecure bypass or new flag.
- [ ] T013 Reference `examples/config/protected-resource-discovery.yaml` from `examples/config/README.md` and distinguish broker configuration from each service's `discovery.resource_url` request field.
- [ ] T014 Confirm that `charts/agentic-identity-broker/templates/configmap.yaml` already forwards `broker.server.enduser.publicUrl` to `server.enduser.public_url` and that `charts/agentic-identity-broker/values.yaml` needs no new key; record this no-change conclusion in `specs/050-oauth2-protected-resource-discovery/plan.md`.

### Phase 2c: API Design (Principles IV and X)

- [ ] T015 Apply `specs/050-oauth2-protected-resource-discovery/contracts/admin-api.md` to `api/admin/openapi.yaml`: request exclusivity, derived/explicit resource, read-only `discovery.client_method`, DCR response-only auth modes, status GET with `PreAuthProxy`, nullable fields, examples, ETags, errors, and safe failure codes; leave `api/enduser/openapi.yaml` unchanged.
- [ ] T016 Obtain written stakeholder review of `api/admin/openapi.yaml` response method values, `discovery.client_method`, and failure-code/status mappings; link the review in the PR before implementation, in addition to the core-field confirmation in `specs/050-oauth2-protected-resource-discovery/spec.md`.
- [ ] T017 [P] Document the confirmed administrative create/update/status API and correct the stale `ErrorResponse.message` requirement in `docs/reference/api.md`; describe authentication, examples, and 400/404/409/504 results.
- [ ] T018 [P] Document hosted CIMD, DCR, resource override, status, and failed-refresh operator journeys in `docs/guides/manage-agents-and-services.md` from the confirmed `api/admin/openapi.yaml` contract.

### Phase 2d: Database Design (Principle IX)

- [ ] T019 Specify migration 036's exact field-combination and status checks, DCR-only `(issuer_uri, client_id)` unique predicate, migration-035 authentication-check restoration, and guarded rollback in `specs/050-oauth2-protected-resource-discovery/data-model.md`; retain the existing service table and manual uniqueness policy.

### Phase 2e: Frontend and Design System Review (Principle XI)

No task applies: `spec.md` and `plan.md` exclude React, administrative UI, Playwright, and screenshots.

### Phase 2f: E2E Acceptance Test Design (Principle XIII)

Use `tests/e2e/README.md`, the production bootstrap, fresh fixture state, `Describe` → `Context` → `It`, and a nearby `// USx-Sy from specs/050-oauth2-protected-resource-discovery/spec.md` comment for every block. Each block asserts observable HTTP/provider behavior, not a placeholder. Mark every block `Label("protected-resource-discovery")`.

- [ ] T020 [P] Write 10 semantic-red `It()` blocks for US1-S1–S10 in `tests/e2e/thirdparty_protected_resource_discovery_test.go`: issuer selection, CIMD-over-DCR, resource plus PKCE, a single `401` Bearer/DPoP `resource_metadata` value, path/root fallback order, and the derived service response.
- [ ] T021 [P] Write nine semantic-red `It()` blocks for US2-S1–S9 in `tests/e2e/thirdparty_protected_resource_dcr_test.go`: public/confidential DCR, exact auth/resource requests, override visibility, issuer-scoped identities, and US2-S3 renewal after PostgreSQL restart with a provider-issued refresh token and valid credential. Label US2-S3 `docker`.
- [ ] T022 [P] Write 12 semantic-red `It()` blocks for US3-S1–S12 in `tests/e2e/thirdparty_protected_resource_rejection_test.go`: missing/mismatched metadata, no downgrade or broader retry, invalid/conflicting request, failed confidential DCR, and same-issuer duplicate. For US3-S3, build without either T006 injection option and assert that the production dialer blocks a private IP before connect.
- [ ] T023 [P] Write eight semantic-red `It()` blocks for US4-S1–S8 in `tests/e2e/thirdparty_protected_resource_status_test.go`: ready/failed/not-applicable status, unchanged active integration and ETag, durable PostgreSQL restart for US4-S5 with `Label("docker")`, and derived/explicit override transitions.
- [ ] T024 Add the separate SC-006 case in `tests/e2e/thirdparty_protected_resource_performance_test.go` before production behavior. Create 20 independent providers. Delay four responses by one second for each registration. Use one Ginkgo process. Require 19 of 20 registrations within five seconds. Label the block `protected-resource-discovery` and `performance`. Run `ginkgo -v --label-filter="protected-resource-discovery && !performance" ./tests/e2e/` and record exactly 39 compiling semantic failures. Run `ginkgo -v --procs=1 --label-filter="protected-resource-discovery && performance" ./tests/e2e/` and record the separate semantic-red measurement. Neither red gate may fail because of missing Builder symbols, unreachable test hosts, or a fixture error.

**Checkpoint**: ADR 038 is accepted, the Admin OpenAPI delta is reviewed, migration 036 is specified, and 39 functional scenarios plus SC-006 fail semantically. The Phase 1 Builder seam compiles before these gates.

## Phase 2.5: Foundational Infrastructure

**Goal**: Add shared contract, guarded outbound I/O, and baseline storage only after both E2E red gates. Write focused tests before each implementation. Compile-only structure for later focused tests is not a completed feature.

- [ ] T025 Complete the probe/GET-JSON/POST-JSON port begun in T006 with a distinct 404 sentinel and safe error categories in `internal/ports/oauth_discovery.go`. Keep probe ordering and metadata validation in the domain, not the port or adapter.
- [ ] T026 Add only fields and constructors needed by focused foundation and story tests in `internal/domain/model/discovery_config.go` and `internal/domain/model/thirdparty_oauth2_provider.go`. Complete test-facing adapter types around the T006 constructor in `internal/adapters/outboundhttp/discovery_client.go`. The earlier E2E red gate must compile without these later symbols.
- [ ] T027 [P] Write semantic-red outbound adapter tests in `internal/adapters/outboundhttp/discovery_client_test.go` for public HTTPS validation, dial-time blocked-IP rejection, no redirects, challenge extraction, 404 sentinel, 256-KiB response limit, one 15-second attempt deadline, and no response-body leaks.
- [ ] T028 Implement the production guarded dialer and bounded probe/JSON operations in `internal/adapters/outboundhttp/discovery_client.go`. Enforce public HTTPS URL validation, `Accept: application/json`, HTTP 200 plus JSON content type, no redirects, and 256-KiB metadata bounds in the client layer even when T006 injects a fake-host HTTP client. Use `internal/domain/netpolicy` at production dial time; reject user information and fragments. Do not expose an insecure runtime constructor.
- [ ] T029 [P] Write semantic-red migration tests in `tests/integration/migrations/migrations_test.go` for legacy rows, migration 036 apply, DCR check/index constraints, guarded down with discovery rows, clean down, and replay on real PostgreSQL.
- [ ] T030 Add `migrations/036_add_protected_resource_discovery.up.sql` and `.down.sql`: same-row source/status fields, DCR-only issuer/client unique index, expanded auth check, consistency checks, and a down guard that refuses to discard discovery-backed rows before restoring migration-035 rules.
- [ ] T031 Define a focused `ThirdpartyOAuth2ProviderDiscoveryStatusWriter` interface in `internal/ports/storage.go` for guarded failure-only status writes. Keep successful active-state writes on the existing repository operations. Do not add methods to the existing ten-method `ThirdpartyOAuth2ProviderRepository`. Require optimistic version and attempt ordering without adapter-specific errors.
- [ ] T032 [P] Write semantic-red shared persistence tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` for discovery fields, atomic success state, and stale failure-only writes without changing service version or sessions.
- [ ] T033 Implement baseline discovery field copying, active-state writes, and guarded status writes in `internal/adapters/storage/memory/thirdparty_provider.go` and `internal/adapters/storage/memory/thirdparty_provider_record.go`; use the adapter lock and preserve manual behavior.
- [ ] T034 [P] Write semantic-red shared persistence tests in `internal/adapters/storage/postgres/thirdparty_provider_test.go` for migration-036 fields, atomic success transaction, and stale failure-only writes under a competing version/newer success.
- [ ] T035 Implement active-state and status columns in `internal/adapters/storage/postgres/thirdparty_provider.go` and `internal/adapters/storage/postgres/thirdparty_provider_record.go`; commit success with the service row, limit failure writes to status fields, and translate unique conflicts into domain storage errors.

**Checkpoint**: Common outbound security and persistence are ready. No story-specific method selection or response behavior is claimed complete.

## Phase 3: User Story 1 — Connect a CIMD Service (P1, MVP)

**Goal**: Register a valid protected resource, discover an advertised issuer, choose compatible hosted CIMD, and connect with one derived resource and PKCE.

**Independent test**: Register a CIMD-plus-DCR resource, finish an account connection, inspect the service response and provider requests, and observe zero DCR calls. Run US1-S1–S10.

### Tests first (Principles VIII and XIII)

- [ ] T036 [P] [US1] Write semantic-red model tests in `internal/domain/model/thirdparty_oauth2_provider_test.go` and `internal/domain/model/discovery_config_test.go` for source exclusivity, endpoint/resource validation, copies, derived value, and manual compatibility.
- [ ] T037 [P] [US1] Write semantic-red discovery and provider-service tests in `internal/domain/thirdparty/protected_resource_discovery_test.go` and `internal/domain/thirdparty/service_test.go`. Cover `401` Bearer/DPoP challenge priority, duplicate or invalid metadata values with no fallback, 404-only well-known fallback, issuer selection, CIMD preference, and zero DCR calls.
- [ ] T038 [P] [US1] Write semantic-red session tests in `internal/domain/oauth2session/service_test.go` for PKCE S256, a single derived `resource` on authorization and code exchange, guarded token client injection, and unchanged manual requests.
- [ ] T039 [P] [US1] Write semantic-red Admin handler tests in `internal/adapters/http/handlers/admin/services_handler_test.go` for create/read/list/update representations without manual fallback or credential disclosure.

### Model, service, adapter, and integration

- [ ] T040 [US1] Implement discovery-source validation in `internal/domain/model/discovery_config.go`: `Discovery.EnableDiscovery` — "Must be true when `ResourceURL` is present." `Discovery.ResourceURL` — "The configured public HTTPS resource identifier. It must have no fragment. Metadata must report the exact same value." `Discovery.MetadataURL` — "Must be absent with `ResourceURL`. It retains its direct AS-metadata meaning for existing services."
- [ ] T041 [US1] Validate discovered issuer and endpoints in `internal/domain/model/thirdparty_oauth2_provider.go`: `IssuerURI` — "Must exactly match one issuer advertised by the resource. A single issuer needs no administrator selection." `Endpoints.AuthorizeEndpoint` — "Required public HTTPS URL from the selected AS metadata." `Endpoints.TokenEndpoint` — "Required public HTTPS URL from the selected AS metadata. It cannot have a `resource` query parameter." `Endpoints.JWKsURI` — "Optional, but a non-empty discovered URL must be public HTTPS."
- [ ] T042 [US1] Establish the service/client identity in `internal/domain/model/thirdparty_oauth2_provider.go`: `ID` — "Assigned before hosted CIMD identity or DCR callback registration." `ClientID` — "Hosted CIMD URL or returned DCR ID. Non-empty and scoped to `IssuerURI` for DCR." `ClientMethod` — "Null for manual or direct AS-metadata services. Stable on discovery-backed refresh."
- [ ] T043 [US1] Copy and validate effective resource state in `internal/domain/model/thirdparty_oauth2_provider.go`: `AuthorizationParams` — "Contains exactly one effective `resource` entry for discovery-backed services. Other permitted provider parameters remain unchanged." `ResourceExplicit` — "True only when an administrator supplies `authorization_params.resource`. False when the value is derived from verified metadata." `Version` — "Existing optimistic-concurrency version. A failure-only status write does not change active configuration or this version." Validate explicit values as non-empty absolute URIs without fragments.
- [ ] T044 [US1] Implement RFC 9728 challenge selection, path-specific/root well-known URLs, 404-only fallback, and PRM validation in `internal/domain/thirdparty/protected_resource_discovery.go`. Use only a single `resource_metadata` from a 401 Bearer or DPoP challenge. Reject duplicate, malformed, or unsupported `resource_metadata` without falling back. Require exact `resource` identity and at least one non-empty `authorization_servers` issuer.
- [ ] T045 [US1] Implement root/path RFC 8414 and OpenID probe order, selected issuer validation, and safe endpoints in `internal/domain/thirdparty/protected_resource_discovery.go`: required `issuer`, `authorization_endpoint`, `token_endpoint`; optional `registration_endpoint`, `jwks_uri`, `client_id_metadata_document_supported`, `token_endpoint_auth_methods_supported`, `token_endpoint_auth_signing_alg_values_supported` — "Exact issuer match; safe public HTTPS endpoints; choose the first non-404 metadata location; no fallback after an invalid response."
- [ ] T046 [US1] Select hosted CIMD only with advertised `private_key_jwt` and ES256 and a usable signing key in `internal/domain/thirdparty/protected_resource_discovery.go`; hosted identity — "Reuse the existing ES256 key domain and public document. No DCR call after CIMD selection." Construct `<server.enduser.public_url>/.well-known/oauth-client/<service-id>`.
- [ ] T047 [US1] Orchestrate create with a preassigned ID, one 15-second context, hosted CIMD, derived resource, safe audit fields, and atomic storage in `internal/domain/thirdparty/service.go`; keep direct `metadata_url` logic and manual requests on their existing path.
- [ ] T048 [US1] Require one stored RFC 8707 resource, PKCE S256, and a separate HTTP client for discovery-backed authorization/code exchange in `internal/domain/oauth2session/service.go`. Use the T006 token-client setter for fake-host E2E calls. Keep manual token wire behavior and its existing `upstreamClient` unchanged. Production code must use the guarded token client.
- [ ] T049 [US1] Parse `discovery.resource_url` without handler-side discovery or fallback, and expose selected method, issuer, client ID, and effective resource without secrets in `internal/adapters/http/handlers/admin/services_handler.go`.
- [ ] T050 [US1] Persist the derived resource, hosted CIMD identity, and success timestamps together in `internal/adapters/storage/memory/thirdparty_provider.go`; ensure read/list return redacted copies.
- [ ] T051 [US1] Persist the same fields atomically and decode legacy/manual rows unchanged in `internal/adapters/storage/postgres/thirdparty_provider.go` and `internal/adapters/storage/postgres/thirdparty_provider_record.go`.
- [ ] T052 [US1] Construct the guarded production discovery port and token client through `internal/app/builder.go`. Inject the T006 discovery-port and token-client options only for E2E discovery-backed requests; send both code exchange and refresh through that token client. Keep the production dial-time guard, manual token client, and routing construction boundary unchanged.
- [ ] T053 [US1] Run US1-S1–S10 in `tests/e2e/thirdparty_protected_resource_discovery_test.go` to green without weakening the assertions; confirm standalone hosted CIMD and manual-service tests remain green.

**Checkpoint**: A CIMD-backed service can connect without manual endpoints. US1 does not require DCR or the new status GET.

## Phase 4: User Story 2 — Connect Through DCR (P1)

**Goal**: Register one compatible DCR client when CIMD does not apply, preserve its issuer-scoped credential, and connect/renew with the selected auth method and resource.

**Independent test**: Register a DCR-only provider, connect, restart against the same PostgreSQL database, renew with the original client, and inspect exact public/basic/post wire forms. Run US2-S1–S9.

### Tests first (Principles VIII and XIII)

- [ ] T054 [P] [US2] Write semantic-red method/secret validation and redaction tests in `internal/domain/model/token_endpoint_auth_method_test.go` and `internal/domain/model/thirdparty_oauth2_provider_security_test.go`; preserve manual confidential auto-detection.
- [ ] T055 [P] [US2] Write semantic-red DCR selection, response-validation, no-reregistration, and encrypted-secret tests in `internal/domain/thirdparty/protected_resource_discovery_test.go` and `internal/domain/thirdparty/service_test.go`. Accept a response without `refresh_token`. Reject every non-zero `client_secret_expires_at`, including a future time, without saving a service or retrying public registration. Accept omitted or zero expiry.
- [ ] T056 [P] [US2] Write semantic-red wire tests in `internal/domain/oauth2session/service_test.go` for public, Basic, POST, CIMD, one resource on exchange/refresh, and issuer-bound credentials. Cover terminal renewal without a refresh token and provider credential rejection on connection or renewal. Require no new registration, broader retry, or auth-style probing.
- [ ] T057 [P] [US2] Write semantic-red issuer-scoped duplicate and secret-at-rest tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` and `tests/integration/storage/infra/thirdparty_service_test.go`; permit equal client IDs only across different issuers.

### Model, service, adapter, and integration

- [ ] T058 [US2] Extend `internal/domain/model/token_endpoint_auth_method.go` and `internal/domain/model/thirdparty_oauth2_provider.go`: `TokenEndpointAuthMethod` — "`private_key_jwt` for hosted CIMD; `client_secret_basic` or `client_secret_post` for confidential DCR; `none` for public DCR. Existing null/manual confidential behavior remains unchanged." `Secret` — "Absent for CIMD/public DCR. Confidential DCR has encrypted ciphertext at rest under the existing `service_id` encryption context." Reject DCR-only methods in manual input.
- [ ] T059 [US2] Send one bounded DCR POST through `internal/adapters/outboundhttp/discovery_client.go`; request fields `redirect_uris`, `grant_types`, `response_types`, `token_endpoint_auth_method` — "Exact existing callback, authorization code, refresh token, code response, and one selected method." Send `application_type: web`, the service name, and no initial access token.
- [ ] T060 [US2] Validate a 201 DCR result in `internal/domain/thirdparty/protected_resource_discovery.go`. Reject missing ID, incompatible callback, mismatched method, missing required secret, and any non-zero `client_secret_expires_at` with `client_registration_invalid`. Do not retry public DCR after a selected confidential registration fails. Permit a response without `refresh_token` while making no promise of renewal. Discard management tokens and URLs.
- [ ] T061 [US2] For creation only, select compatible CIMD first, then `client_secret_basic`, `client_secret_post`, or public `none` with PKCE S256 in `internal/domain/thirdparty/protected_resource_discovery.go`. Apply the RFC 8414 basic default. Never retry public after a chosen confidential registration fails. On updates, do not re-run this preference order; T086 pins the stored methods.
- [ ] T062 [US2] Register once, encrypt a confidential DCR secret with the existing `EncryptionPort` and exactly one `service_id` context, persist the issuer/client pair, and discard management credentials in `internal/domain/thirdparty/service.go`.
- [ ] T063 [US2] Pin DCR code exchange to `AuthStyleInHeader` for Basic or `AuthStyleInParams` for POST/public, and preserve the CIMD assertion path in `internal/domain/oauth2session/service.go`; never probe another style.
- [ ] T064 [US2] Send only the selected Basic header or POST body secret on refresh, one stored `resource`, and no secret for public DCR in `internal/domain/oauth2session/service.go`. Load credentials by service ID and verify issuer. If no refresh token exists or the provider rejects a credential, fail without re-registering or probing another method.
- [ ] T065 [US2] Enforce issuer-scoped DCR uniqueness under the adapter lock and copy absent/encrypted secrets correctly in `internal/adapters/storage/memory/thirdparty_provider.go`; preserve manual uniqueness.
- [ ] T066 [US2] Persist confidential/public DCR fields using migration 036's partial unique index and conflict mapping in `internal/adapters/storage/postgres/thirdparty_provider.go`; verify encrypted secret survives a fresh adapter and deletes with the service.
- [ ] T067 [US2] Return `discovery.client_method`, explicit DCR auth method, issuer and client ID, and effective override without a DCR secret in `internal/adapters/http/handlers/admin/services_handler.go`.
- [ ] T068 [US2] Run US2-S1–S9 in `tests/e2e/thirdparty_protected_resource_dcr_test.go` to green; use a new PostgreSQL adapter after restart for US2-S3 and assert registration count stays one.

**Checkpoint**: DCR-only providers work without manual credentials. The selected identity, token audience, and client authentication survive a restart.

## Phase 5: User Story 3 — Reject Unsafe Discovery (P1)

**Goal**: Reject invalid resource/issuer claims, unsafe destinations, incompatible registration, and failed resource-scoped tokens without creating a partial service or downgrading.

**Independent test**: Send invalid metadata/private-IP/conflicting-source requests, then reject `resource` at token exchange. Observe no stored service or broader retry. Run US3-S1–S12.

### Tests first (Principles VIII and XIII)

- [ ] T069 [P] [US3] Write semantic-red request and endpoint validation tests in `internal/domain/model/discovery_config_test.go` and `internal/domain/model/thirdparty_oauth2_provider_security_test.go` for conflicting sources, malformed explicit resource, URL query duplication, and manual compatibility.
- [ ] T070 [P] [US3] Write semantic-red unsafe-target tests in `internal/adapters/outboundhttp/discovery_client_test.go` for loopback/private/cloud-metadata IPs after DNS, redirects, TLS failure, oversized/malformed JSON, and one shared deadline.
- [ ] T071 [P] [US3] Write semantic-red fail-closed service tests in `internal/domain/thirdparty/protected_resource_discovery_test.go` and `internal/domain/thirdparty/service_test.go`. Reject resource/issuer mismatch, duplicate or malformed challenge metadata, a `resource_metadata` value on non-`401`, unavailable CIMD, failed DCR, and duplicate identities without partial storage.
- [ ] T072 [P] [US3] Write semantic-red token and error tests in `internal/domain/oauth2session/service_security_test.go` and `internal/adapters/http/handlers/admin/services_handler_test.go` for `invalid_target`, no broader token retry, safe error/status fields, and no leaked credential.

### Validation, safety, and integration

- [ ] T073 [US3] Validate request exclusivity before network access in `internal/domain/model/discovery_config.go` and `internal/adapters/http/handlers/admin/services_handler.go`; reject manual endpoints/credentials plus `resource_url`, a second `metadata_url`, invalid explicit resource, and operator-supplied DCR auth modes.
- [ ] T074 [US3] Fail on missing PRM, mismatched resource/issuer, unsupported method, invalid first metadata result, and unselected multiple issuers in `internal/domain/thirdparty/protected_resource_discovery.go`. Reject malformed or multiple `resource_metadata` values and a value on non-`401`; do not fall back. Advance to another well-known URL only on `404`.
- [ ] T075 [US3] Apply the production guarded transport to discovery, registration, code exchange, and refresh in `internal/adapters/outboundhttp/discovery_client.go` and `internal/domain/oauth2session/service.go`; reject blocked resolved addresses before connect and never follow a redirect.
- [ ] T076 [US3] Treat `invalid_target` as terminal, send no token request without the stored `resource`, and store no replacement token on failure in `internal/domain/oauth2session/service.go`.
- [ ] T077 [US3] Map only the approved safe failure codes to 400/409/504 or storage/encryption 500 in `internal/domain/thirdparty/service.go` and `internal/adapters/http/handlers/admin/services_handler.go`; audit service ID, operation, outcome, issuer, method, and code without URL queries, raw bodies, assertions, or secrets.
- [ ] T078 [US3] Reject a same-issuer DCR duplicate without overwriting either credential in `internal/adapters/storage/memory/thirdparty_provider.go` and `internal/adapters/storage/postgres/thirdparty_provider.go`; ensure a rejected create leaves no service row or local credential.
- [ ] T079 [US3] Run US3-S1–S12 in `tests/e2e/thirdparty_protected_resource_rejection_test.go` to green. For US3-S3, use Builder without the fake-host discovery port or token client and prove the production transport rejects the private destination before connect. Confirm no fallback registration.

**Checkpoint**: No unsafe discovery request or rejected resource token request produces a broader authorization path.

## Phase 6: User Story 4 — Inspect and Refresh Discovery (P2)

**Goal**: Expose credential-free stored status, retain the active integration on failed refresh, and preserve/update resource source and client identity correctly.

**Independent test**: Read ready and failed statuses, refresh without changing the issuer/method, restart on PostgreSQL, and renew with the prior client. Read `not_applicable` for a manual service. Run US4-S1–S8.

### Tests first (Principles VIII and XIII)

- [ ] T080 [P] [US4] Write semantic-red value/transition tests in `internal/domain/model/discovery_status_test.go` and `internal/domain/model/thirdparty_oauth2_provider_test.go` for nullable timestamps, derived status, and override changes.
- [ ] T081 [P] [US4] Write semantic-red update/status tests in `internal/domain/thirdparty/service_test.go`. Verify that explicit issuer changes retain both `ClientMethod` and exact `TokenEndpointAuthMethod`. Reject Basic-to-POST, confidential-to-public, and CIMD-to-DCR changes without DCR fallback. Cover unchanged identity, zero-session issuer changes, stale failures, preserved ETag/sessions, and safe audit values.
- [ ] T082 [P] [US4] Write semantic-red memory concurrency tests in `internal/adapters/storage/memory/thirdparty_provider_test.go` for success atomicity, last-success retention, guarded failure-only writes, and deletion.
- [ ] T083 [P] [US4] Write semantic-red PostgreSQL restart and failure-write tests in `tests/integration/storage/infra/thirdparty_service_test.go` for durable active/failed status, version/attempt order, sessions, and credential reuse.
- [ ] T084 [P] [US4] Write semantic-red status contract tests in `internal/adapters/http/handlers/admin/services_handler_test.go` for UUID/canonical lookup, authentication, null fields, 400/404 errors, and zero provider calls.

### Model, service, endpoint, and integration

- [ ] T085 [US4] Implement the owned status value in `internal/domain/model/discovery_status.go`: `DiscoveryStatus` — "Contains attempt and success timestamps and a safe failure reason. It is stored with the service." `LastAttemptAt` — "Set when a create or refresh completes. Latest attempt wins. Null for manual services." `LastSuccessAt` — "Set after active discovery configuration commits. Failed refresh retains the prior value. Null before success or for manual services." `FailureReason` — "Null after success. Failure has no raw provider body, credential, token, assertion, URL query, or secret." Derive `not_applicable`, `ready`, and `failed` from stored state.
- [ ] T086 [US4] On discovery-backed update, validate the advertised active issuer in `internal/domain/thirdparty/service.go`. Keep both `ClientMethod` and exact `TokenEndpointAuthMethod`. Reuse the client ID/secret when issuer is unchanged. Require an explicit alternate issuer and zero sessions before issuer change. Register a new DCR client only with the stored authentication method. If the new issuer or DCR response cannot support that method, reject the update without public fallback or active-state changes. A successful full-replacement manual update clears discovery source, DCR credential, and status.
- [ ] T087 [US4] Record only attempt time and safe reason after a remote refresh starts and fails. Call the focused writer in `internal/ports/storage.go` from `internal/domain/thirdparty/service.go`. Leave active state, last success, sessions, and version unchanged. Do not write status for pre-network request-shape errors.
- [ ] T088 [US4] Implement derived/explicit resource transitions in `internal/domain/model/thirdparty_oauth2_provider.go` and `internal/domain/thirdparty/service.go`: omitted `authorization_params` retains source; replacement with `resource` makes it explicit; replacement without `resource` restores the verified URL; changed discovery URL updates only a derived value.
- [ ] T089 [US4] Atomically commit successful refresh and reject stale failure-only writes under the lock in `internal/adapters/storage/memory/thirdparty_provider.go`; preserve active state and ETag on failure.
- [ ] T090 [US4] Atomically update success and guard failure-only writes by version/newer attempt in `internal/adapters/storage/postgres/thirdparty_provider.go`; keep status and DCR secret on the service row across restart and remove them on delete.
- [ ] T091 [US4] Add credential-free `GetDiscoveryStatus` to `internal/adapters/http/handlers/admin/services_handler.go`; resolve UUID/canonical service IDs, map null fields and safe reasons, and make the GET read storage without provider I/O.
- [ ] T092 [US4] Register authenticated `GET /api/services/{service-id}/discovery-status` in `internal/adapters/http/routing/admin.go` and inject the pre-wired handler from `internal/app/builder.go`; make the route read-only and return 404 after deletion.
- [ ] T093 [US4] Run US4-S1–S8 in `tests/e2e/thirdparty_protected_resource_status_test.go` to green; rebuild the app with a new PostgreSQL adapter for US4-S5 and prove the old client still renews.

**Checkpoint**: The latest attempt and last success remain distinct after a restart. Manual services retain their connection behavior.

## Phase N: Constitution Compliance and Polish

### Design Phase Verification (Principles II, IV, V, VII, IX, X, XIII)

- [ ] T094 Confirm the glossary, outbound trust flow, accepted ADR 038, and the precise ADR 036/015 relationship in `ARCHITECTURE.md` and `adrs/038-protected-resource-discovery-and-dcr.md`. Confirm that the full SC-006 target and the 15-second/256-KiB limits appear in `ARCHITECTURE.md` (Principles II and V).
- [ ] T095 Confirm the HTTPS public-URL precondition and no-new-key Helm boundary in `docs/configuration.md`, `examples/config/protected-resource-discovery.yaml`, `examples/config/README.md`, and `charts/agentic-identity-broker/templates/configmap.yaml` (Principle VII).
- [ ] T096 Confirm the stakeholder review record and exact request/response/security/error examples in `api/admin/openapi.yaml` against `specs/050-oauth2-protected-resource-discovery/contracts/admin-api.md` (Principles IV and X).
- [ ] T097 Confirm migration design, guarded down rule, and preservation of manual rows in `specs/050-oauth2-protected-resource-discovery/data-model.md` and `migrations/036_add_protected_resource_discovery.up.sql` (Principle IX).
- [ ] T098 Confirm exactly 39 scenario IDs map to `Describe` → `Context` → `It()` blocks with labels and spec references in `tests/e2e/thirdparty_protected_resource_discovery_test.go`, `tests/e2e/thirdparty_protected_resource_dcr_test.go`, `tests/e2e/thirdparty_protected_resource_rejection_test.go`, and `tests/e2e/thirdparty_protected_resource_status_test.go` (Principle XIII); record semantic red/green and reject skipped, placeholder, or weakened assertions.

### Implementation Phase Verification (Principles I–XIII, as applicable)

- [ ] T099 Audit SSRF/TLS/redirect/timeout/body bounds, no crypto replacement, and credential-free errors/audit in `internal/adapters/outboundhttp/discovery_client.go`, `internal/domain/thirdparty/service.go`, and `internal/domain/oauth2session/service.go` (Principles I and III).
- [ ] T100 Confirm the domain uses `internal/ports/oauth_discovery.go` and the focused status writer in `internal/ports/storage.go`. Confirm Builder owns guarded production wiring, both T006 test options are absent in US3-S3, and manual token behavior remains unchanged. Confirm `internal/adapters/http/routing/admin.go` only registers routes (Principles VI, IX, XII).
- [ ] T101 Confirm both adapters preserve same-row success/failure isolation, issuer-scoped DCR uniqueness, encrypted secrets, guarded version ordering, and migration apply/rollback/replay in `tests/integration/storage/infra/thirdparty_service_test.go` and `tests/integration/migrations/migrations_test.go` (Principle IX).
- [ ] T102 Confirm focused tests in `internal/domain/model/thirdparty_oauth2_provider_test.go`, `internal/domain/thirdparty/protected_resource_discovery_test.go`, `internal/domain/oauth2session/service_test.go`, and `internal/adapters/outboundhttp/discovery_client_test.go` failed semantically before implementation and now pass without weaker assertions (Principle VIII).
- [ ] T103 Re-run the unchanged SC-006 case from T024 in one Ginkgo process. Require at least 19 of 20 concurrent registrations within five seconds. Compare its green result with the recorded semantic-red result; do not weaken the threshold (Principles VIII and XIII).
- [ ] T104 Run the 39-case functional filter and the one-process SC-006 filter from `specs/050-oauth2-protected-resource-discovery/quickstart.md`; confirm all functional cases turn green, including both `docker` restart cases, with no test-order dependence (Principle XIII).
- [ ] T105 Run `just check`, `just test`, `just test-integration`, `just test-integration-infra`, and `just docs-build` as listed in `specs/050-oauth2-protected-resource-discovery/quickstart.md`; run `just verify` as the final gate (Principles II, VIII, IX, XIII).
- [ ] T106 Exercise the CIMD, DCR, resource rejection, failed refresh, PostgreSQL restart, and manual-service smoke scenarios in `specs/050-oauth2-protected-resource-discovery/quickstart.md`; compare Admin HTTP behavior with the approved `api/admin/openapi.yaml` and reconcile `docs/reference/api.md` and `docs/guides/manage-agents-and-services.md` (Principles IV and X).

**Frontend**: Principle XI and frontend portions of XIII do not apply. This feature changes no React UI, design-system component, Playwright test, or screenshot.

## Dependencies and Execution Order

### Phase graph

```text
Phase 0 (accepted ADR 038; behavior-neutral refactor, separate PR)
    → Phase 1 (provider, TLS seam, fixtures)
    → Phase 2a–2d (model/glossary, configuration, approved Admin API, DB design)
    → Phase 2f (39 functional plus SC-006 semantic-red cases through the Phase 1 Builder seam)
    → Phase 2.5 (focused tests → safe HTTP/ports/migration/storage)
    → US1 → US2 → US3
                ↘ US4 (after US1 and US2)
    → Phase N (compliance, SC-006 green measurement, full validation)
```

Phase 2e is not applicable. Phase 2.7 entity boilerplate is not applicable: `model.ThirdpartyOAuth2ProviderEntity` already owns the status and registration. Phase 2a–2d can proceed in parallel only where files and approvals permit. Finish the OpenAPI review, ADR acceptance, and E2E red gate before implementation.

### Story dependencies

- **US1 (P1)**: Starts after Phase 2.5. It needs no later story. This is the MVP.
- **US2 (P1)**: Starts after US1 because DCR shares validated resource/issuer metadata and the effective resource path. It remains independently testable with a DCR-only provider.
- **US3 (P1)**: Starts after US2 because its rejection scenarios exercise both CIMD and DCR, including duplicate issuer/client identities. It remains independently testable with invalid requests and token rejection.
- **US4 (P2)**: Starts after US1 and US2 because refresh and PostgreSQL restart must preserve an active DCR client. It can run in parallel with US3 only with explicit ownership of shared `internal/domain/thirdparty/service.go` and storage files; otherwise serialize those edits.
- **Phase N**: Starts after all four stories. Re-run the performance case written in T024 with its own label; it does not increase the 39-case functional count.

### Parallel examples

- **US1**: T036 (model tests), T037 (discovery tests), T038 (session tests), and T039 (handler tests) touch different files; all establish semantic red before T040–T052.
- **US2**: T054 (method tests), T055 (domain tests), T056 (session tests), and T057 (storage tests) touch different files; finish their red assertions before T058–T067.
- **US3**: T069 (model tests), T070 (adapter tests), T071 (service tests), and T072 (token/handler tests) touch different files; run them before T073–T078.
- **US4**: T080 (model tests), T081 (domain tests), T082 (memory tests), T083 (PostgreSQL tests), and T084 (handler tests) touch different files; finish their red phase before T085–T092.

## Implementation Strategy

1. Complete the separate Phase 0 refactor PR with accepted ADR 038 and unchanged CIMD behavior.
2. Complete setup, design preconditions, written Admin API review, the Builder test seam, and all 39 functional plus SC-006 semantic-red cases.
3. Complete the shared foundation with test-first adapter and migration work.
4. Deliver US1 alone as the MVP. Run its 10 scenarios and the existing CIMD/manual flows.
5. Add US2, then US3. Run each story's scenarios without changing earlier expectations.
6. Add US4 after US2. Preserve the active client and ETag when refresh fails.
7. Complete compliance, performance, quickstart smoke, and full validation. Do not claim the feature is ready while an approval or required PostgreSQL scenario remains incomplete.
