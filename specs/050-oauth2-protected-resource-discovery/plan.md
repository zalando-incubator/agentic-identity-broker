# Implementation Plan: Protected Resource Discovery for Third-Party OAuth2 Services

**Branch**: `050-oauth2-protected-resource-discovery` | **Date**: 2026-10-06 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/050-oauth2-protected-resource-discovery/spec.md`.

## Summary

The broker will register a third-party OAuth2 service from a protected resource URL. It will validate the resource metadata, select an advertised issuer, and discover the issuer's endpoints. It will use a compatible hosted CIMD client before it considers confidential or public dynamic client registration (DCR). The service will persist its chosen client and one effective RFC 8707 resource value. An authenticated Admin API status resource will report the latest attempt without exposing credentials or replacing working configuration after a failed refresh.

The design keeps the existing `ThirdpartyOAuth2ProviderEntity` aggregate. It adds a protected-resource discovery path in `internal/domain/thirdparty`, a shared SSRF-safe outbound HTTP adapter, explicit DCR token authentication, same-row status persistence, and a DCR-only `(issuer_uri, client_id)` uniqueness rule.

## Technical Context

**Language/Version**: Go 1.27.1 (`go.mod`).

**Primary Dependencies**: Go `net/http`, `net`, and `crypto/tls`; `golang.org/x/oauth2` v0.37.0; the existing hosted CIMD signer with JWX v4; chi v5; sqlx + pgx v5; Ginkgo v2/Gomega; testify.

**Storage**: PostgreSQL in production and in-memory storage in development and E2E. Service secrets use the existing `EncryptionPort` with exactly one `service_id` AAD subject. Migration 035 is the current database head. DCR also requires the new optional deployment-wide `third_party_oauth2.client_name` configuration key; an absent value blocks DCR only.

**Testing**: Go unit tests, adapter tests, PostgreSQL migration and storage integration tests with testcontainers, and dual-server Ginkgo E2E tests through production `app.Builder`.

**Target Platform**: Linux broker service in production. Darwin is a supported development platform. No React UI changes.

**Project Type**: Go backend in a monorepo with Admin and End-user HTTP surfaces. This feature changes the administrative service contract and outbound OAuth2 client behavior. It adds no End-user route.

**Performance Goal**: At least 19 of 20 registrations complete within five seconds when each remote response takes at most one second (SC-006).

**Constraints**:

- One 15-second deadline covers each discovery and registration attempt.
- Each remote response has a 256-KiB limit.
- Production remote connections use public HTTPS and dial-time IP blocking. They reject redirects.
- Every discovery-backed connection uses PKCE S256.
- Every discovery-backed authorization, code exchange, and refresh request sends exactly one `resource` value.
- CIMD and confidential DCR never fall back to another client method.
- No response, status record, or audit record contains a secret, assertion, token, code, URL query, or remote body.
- Existing manual and direct-metadata services keep their current validation and wire behavior.

**Scale/Scope**: One existing aggregate, 41 mapped acceptance scenarios (US1: 10, US2: 10, US3: 13, US4: 8), one Admin API route, one reversible migration, one new ADR, and one performance measurement. No scheduled discovery or UI.

**NEEDS CLARIFICATION**: None. [research.md](./research.md) resolves the technical questions. Accepted ADR 038 and written review of the canonical OpenAPI response and failure-code delta are implementation preconditions. The latest specification also requires the broker-wide DCR client name and two additional acceptance scenarios.

## Constitution Check

*GATE: Evaluate before Phase 0 research. Re-check after Phase 1 design. The design passes this planning gate. Implementation does not begin until the listed preconditions are complete.*

### Pre-Phase 0 gate

| Check | Status | Plan |
|---|---|---|
| Domain model and glossary | Pass | [data-model.md](./data-model.md) defines the aggregate fields, metadata values, client registration, status, transitions, and invariants. Add its glossary terms to `ARCHITECTURE.md` before implementation. |
| Entity IDs | Pass | Status and client registration belong to the existing `id.ServiceID` aggregate. No new UUID entity or `gen_ids.go` entry is necessary. |
| Configuration design and examples | Planned | Add the optional `third_party_oauth2.client_name` key through the broker configuration port, loader, environment binding, CLI flag, configuration guide, and protected-resource example. Require a non-blank value only when DCR is selected. Keep the fixed 15-second and 256-KiB limits and the HTTPS hosted-CIMD public URL. |
| Helm deployment contract | Planned | Add `broker.thirdPartyOauth2.clientName` in values, schema, README, and ConfigMap. The existing chart already passes `server.enduser.public_url`. Do not block startup or manual/CIMD use when the name is absent. |
| API design and approval | Pass with implementation precondition | The 2026-10-01 clarification confirms `discovery.resource_url`, `authorization_params.resource`, and the status path and fields. [contracts/admin-api.md](./contracts/admin-api.md) defines the remaining representation details. Record stakeholder review of the canonical `api/admin/openapi.yaml` before implementation. |
| Database design | Pass | Add `036_add_protected_resource_discovery.{up,down}.sql`. Test apply, guarded rollback, clean rollback, replay, and existing rows. |
| E2E acceptance mapping and red phase | Pass | Write 41 scenario-mapped Ginkgo blocks and a separate SC-006 performance block before production behavior. Record semantic-red results for both filters in Phase 2f. |
| Frontend Playwright and screenshots | Not applicable | The specification excludes UI work. The React SPA has no service-management route. |

**2026-10-07 security-scan exception:** The stakeholder chose to finish this feature before remediating advisories tracked in Dependabot PRs. The Phase 0 `just verify` run passed formatting, vet, and lint, then failed on 11 npm advisories in unchanged lockfiles. Three advisories had no fixed version listed. Focused Go tests for the moved packages, Builder, and E2E bootstrap passed. The existing CIMD E2E filter passed 101 scenarios. This exception does not make `just verify` green or disable a runtime security control. Re-run and report the full gate at the final checkpoint.

Phase 0 was published for review in [PR #190](https://github.com/zalando-incubator/agentic-identity-broker/pull/190). It contains only the accepted ADR, relocated code, bootstrap/Builder callers, and architecture paths. The feature implementation remains a separate stacked change.
The approved Admin API review record is linked in the draft [feature PR #191](https://github.com/zalando-incubator/agentic-identity-broker/pull/191), stacked on Phase 0 PR #190 before feature behavior.

### Implementation considerations

| Check | Status | Plan |
|---|---|---|
| Security-first | Pass | Enforce exact identities, safe destinations, no redirects, bounded responses, fixed method selection, and fail-closed errors. Use the guarded transport for discovery-backed token requests too. |
| Binding ADRs | Precondition | Draft and accept ADR 038 before Phase 2.5. It defines the discovery and registration port boundary. It supersedes ADR 036 decision 2 only for DCR-issued `client_secret_basic` and `client_secret_post` clients. ADRs 004, 008, 012, 015, 030, and 037 remain binding. |
| Architecture and glossary | Planned | Update the outbound flow, SSRF boundary, client modes, glossary, audit rules, and ADR index in `ARCHITECTURE.md`. Document SC-006: at least 19 of 20 registrations finish within five seconds when each external response takes at most one second. Record the 15-second/256-KiB security limits there. Phase 0 path changes belong in the separate refactor PR. |
| Library-first security | Pass | Use Go TLS and network controls, `golang.org/x/oauth2`, and the existing JWX CIMD signer. Add no cryptography. Keep DCR secret encryption inside the existing domain service and `EncryptionPort`. |
| Zalando and OpenAPI | Planned | Reuse resource names, `PreAuthProxy`, `ErrorResponse`, status mappings, and ETags. Update the canonical OpenAPI file first. |
| End-user documentation | Planned | Update `docs/reference/api.md` and `docs/guides/manage-agents-and-services.md`. No End-user OpenAPI route changes. |
| Migration testing | Planned | Extend `tests/integration/migrations/migrations_test.go` and the third-party PostgreSQL storage tests. |
| Hexagonal architecture | Pass | Domain logic owns probe order, validation, selection, and state transitions. Adapters perform HTTP and storage work. Handlers parse and format. Builder wires all dependencies. |
| Persistence patterns | Pass | Keep active configuration and success status on the existing repository operations. Define a separate, focused failure-status writer in `internal/ports/storage.go` and implement it in both adapters. |

### Post-design re-check

The gate passes after Phase 1.

- The data model extends the existing service aggregate. It adds no unrelated aggregate, table, or UUID type.
- The contracts specify the administrative and outbound wire behavior in testable detail.
- Failed creates leave no service. Failed refreshes change only status timestamps and reason.
- Hosted CIMD remains the first choice. A selected CIMD or confidential DCR method never falls back.
- The new adapter boundary and DCR authentication methods need ADR 038 before implementation. This is a required precondition, not an exception.
- No security, API, configuration, storage, UI, or test-traceability question remains open.

## Project Structure

### Documentation for this feature

```text
specs/050-oauth2-protected-resource-discovery/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── admin-api.md
    └── upstream-oauth.md
```

### Source code and documentation affected

```text
adrs/
└── 038-protected-resource-discovery-and-dcr.md          # Port boundary and DCR auth decision

api/admin/openapi.yaml                                   # Discovery fields, methods, status route, errors

internal/
├── domain/
│   ├── netpolicy/                                       # Shared blocked-address value object (moved from CIMD)
│   ├── model/
│   │   ├── discovery_config.go                          # ResourceURL and client method
│   │   ├── discovery_status.go                          # Status value object and derived state
│   │   ├── thirdparty_oauth2_provider.go                # Invariants, resource rules, copy semantics
│   │   └── token_endpoint_auth_method.go                # DCR-only basic/post methods
│   ├── thirdparty/
│   │   ├── service.go                                   # Create/update/status orchestration and audit
│   │   └── protected_resource_discovery.go             # Probe order, metadata validation, method selection
│   └── oauth2session/service.go                         # Explicit DCR auth, resource guard, safe token client
├── ports/
│   ├── oauth_discovery.go                               # Outbound discovery and registration contract
│   └── storage.go                                       # Focused failure-status writer beside existing storage ports
├── adapters/
│   ├── outboundhttp/                                    # Shared SSRF-safe transport, CIMD fetcher, discovery client
│   ├── http/
│   │   ├── handlers/admin/services_handler.go           # DTOs, errors, and status handler
│   │   └── routing/admin.go                             # Status route
│   └── storage/
│       ├── memory/thirdparty_provider*.go               # Fields, status write, DCR uniqueness
│       └── postgres/thirdparty_provider*.go             # Fields, transaction, status write, conflict mapping
└── app/builder.go                                       # Discovery port, token-client, and production wiring

migrations/
└── 036_add_protected_resource_discovery.{up,down}.sql

docs/
├── configuration.md
├── reference/api.md
└── guides/manage-agents-and-services.md

examples/config/
└── protected-resource-discovery.yaml

tests/
├── e2e/
│   ├── bootstrap/protected_resource.go                  # Port-level fake-host client and token-client seam
│   ├── helpers/mock_protected_resource_provider.go      # Resource, issuer, DCR, and token double
│   ├── thirdparty_protected_resource_discovery_test.go  # US1
│   ├── thirdparty_protected_resource_dcr_test.go        # US2
│   ├── thirdparty_protected_resource_rejection_test.go  # US3
│   ├── thirdparty_protected_resource_status_test.go     # US4
│   └── thirdparty_protected_resource_performance_test.go # SC-006
└── integration/
    ├── migrations/migrations_test.go
    └── storage/infra/thirdparty_service_test.go
```

**Structure Decision**: Keep protected-resource discovery inside the existing `internal/domain/thirdparty` bounded context because it creates and updates the existing service aggregate. Move the reusable blocked-address policy to `internal/domain/netpolicy`. Move the CIMD fetcher into a new `internal/adapters/outboundhttp` package beside the discovery client. This lets both clients share one dial-time guard without a cross-adapter import. The direct `metadata_url` helper in `internal/domain/storage/discovery.go` remains unchanged for existing services.

## Design and Implementation Approach

### 1. Share one outbound safety boundary

Phase 0 moves `SSRFBlocklist` from `internal/domain/oauth2/cimd` to `internal/domain/netpolicy`. It moves `internal/adapters/cimd` to `internal/adapters/outboundhttp` without behavior changes. Existing CIMD fetcher tests must prove the same blocked ranges, no-redirect rule, size limit, timeout, and TLS behavior.

The Phase 0 PR must update the "New Domain Packages" paragraph in the CIMD Subsystem section and the `SSRFBlocklist` and `CIMDFetcher` glossary entries in `ARCHITECTURE.md`. Name `internal/domain/netpolicy/` and `internal/adapters/outboundhttp/` as the new locations. The later feature PR cannot repair stale paths in the separate refactor PR.

The production outbound client uses the dial-time `Control` callback to block resolved private IPs. The client layer validates public HTTPS URLs, rejects redirects, bounds metadata responses at 256 KiB, and applies one attempt deadline. E2E tests inject a fake-host HTTP client through the discovery port and a separate discovery-backed token-client option. The injected client replaces the guarded dialer only for those tests; no runtime configuration selects it.

Main already builds one shared `upstreamClient` for session tokens, proxy grants, and JWKS. It has no dial-time IP guard and follows redirects. Builder must keep that client unchanged for existing flows. It creates one separate guarded transport for discovery, DCR, and discovery-backed token calls. The new transport follows the same TLS and optional OTel wiring pattern. Builder closes both transports' idle connections at shutdown. Do not mutate the shared client's transport or redirect policy.

Set `Proxy` to nil on the guarded transport so its dial-time guard sees the actual target IP. Do not inherit `http.ProxyFromEnvironment` from the shared transport. Leave proxy behavior on the existing shared client unchanged.

Discovery and DCR use one 15-second attempt deadline and 256-KiB response bounds. Code exchange and refresh use the configured upstream timeout and their existing token-response limits. The guarded transport can serve clients with different per-operation deadlines. CIMD keeps its separate fetch timeout and body cap while reusing the dial-time guard code.

### 2. Add a narrow discovery port

Add one port in `internal/ports/oauth_discovery.go`. It provides only these operations:

- Probe a resource and return `resource_metadata` challenge values.
- GET bounded JSON and identify a `404` with a sentinel error.
- POST one bounded JSON registration request.

Phase 1 adds compile-only port signatures so Builder can accept `ports.OAuthDiscoveryClient` before the E2E red gate. T025 completes the 404 sentinel and safe error categories. No discovery behavior is required in Phase 1.

The domain owns URL construction, probe order, JSON validation, exact identity checks, method selection, and failure codes. This keeps the security policy testable with hand-written domain mocks. ADR 038 records this boundary.

### 3. Orchestrate create and update in the provider service

`ThirdpartyOAuth2ProviderService.Create` and `Update` branch when `Discovery.ResourceURL` is present. The handler makes no discovery call for this mode.

The service follows these steps:

1. Validate request shape before network access.
2. Allocate the service ID for create.
3. Check the explicit issuer-change session rule with `UserSessionRepository.CountByService` for update.
4. Run resource and issuer discovery under one 15-second context.
5. On create, select hosted CIMD, confidential DCR, or public DCR according to [upstream-oauth.md](./contracts/upstream-oauth.md). On every update, keep both `ClientMethod` and the exact `TokenEndpointAuthMethod`. If an explicitly selected new issuer cannot support the stored method, reject the update without registering another method or downgrading a confidential client.
6. Validate the selected identity and effective resource.
7. Encrypt a confidential DCR secret with the service subject.
8. Persist active state and success status in one repository operation.

A failed create returns a safe domain error and leaves no row. A failed update writes only `last_attempt_at` and `failure_reason` through the narrow status operation. Audit records use `InfoContext` with service ID, operation, outcome, issuer, client method, and failure code only.

The existing handler-level metadata fallback remains only for the existing direct `metadata_url` path. It never runs for `resource_url`.

### 4. Model the effective resource and client methods

Extend `DiscoveryConfig` with `ResourceURL` and `ClientMethod`. Add `ResourceExplicit` and `DiscoveryStatus` to the aggregate. Validate an explicit resource as an absolute URI without a fragment. Reject a discovery-backed token endpoint that has a `resource` query parameter.

Add `client_secret_basic` and `client_secret_post` to `TokenEndpointAuthMethod`. Permit them only when `ClientMethod` is `dcr` and a secret exists. Manual service request parsing still accepts only omitted, `null`, `none`, and `private_key_jwt`.

Update `Copy` and `RedactedCopy` so that a discovery-backed response never contains a DCR secret.

### 5. Send exact OAuth requests

`OAuth2SessionService` already receives the shared `upstreamClient` from Builder. Code exchange injects it through `oauth2.HTTPClient`, and refresh calls it directly. For a discovery-backed service, both paths select the separate guarded token client. Manual services keep the shared client and current wire behavior.

Implement each method exactly:

- `client_secret_basic`: `AuthStyleInHeader` for code exchange. Use an RFC 6749 encoded Basic header for refresh and no body secret.
- `client_secret_post`: `AuthStyleInParams` for code exchange and a body secret for refresh. Send no `Authorization` header.
- `none`: Use the existing public client path.
- `private_key_jwt`: Use the existing hosted CIMD assertion path.

Before each request, require the discovery-backed `resource` value. Continue to use the stored map so that each request has one value. Treat `invalid_target` as a permanent code-exchange error. Store no token after a rejected request.

A DCR registration can omit the refresh grant, but it must not have a non-zero secret expiry. Reject an expiring secret with `client_registration_invalid` before saving a service; do not try public DCR after a selected confidential registration fails. Renewal needs a provider-issued refresh token and a credential the provider still accepts. Failed connection or renewal never triggers re-registration or another authentication method.

### 6. Persist active state and status safely

Migration 036 adds the fields in [data-model.md](./data-model.md). It extends the client-authentication check and adds a DCR-only unique index on `(issuer_uri, client_id)`.

PostgreSQL create and update write active state and success timestamps in the existing repository transaction. A unique-index violation maps to a storage conflict. The failure-status operation updates only status columns when the service version and attempt order still match. The memory adapter enforces the same uniqueness and stale-attempt rules under its lock.

Define a focused failure-only writer in `internal/ports/storage.go`. Keep successful create/update on the existing repository operations. Do not add another method to the existing ten-method `ThirdpartyOAuth2ProviderRepository`.

Deletion keeps its existing guards. Because status and the DCR secret are service-row columns, a successful delete removes them.

### 7. Expose the status resource

Add `GetDiscoveryStatus` to `ServicesHandler`. Mount `GET /api/services/{service-id}/discovery-status` in `SetupAdminRoutes`. The handler resolves the existing UUID or canonical ID and calls the domain service. The domain service derives `ready`, `failed`, or `not_applicable` from stored state. It never contacts a provider.

## Implementation Phase Overview

### Phase 0: Pre-implementation refactoring

**Include.** Submit this phase as a separate PR because it moves existing SSRF infrastructure.

- Move the blocked-address policy to `internal/domain/netpolicy`.
- Move the CIMD fetcher to `internal/adapters/outboundhttp`.
- Update Builder and tests without changing CIMD behavior.
- Update the "New Domain Packages" paragraph and the `SSRFBlocklist` and `CIMDFetcher` glossary entries in `ARCHITECTURE.md`. Document the new `internal/domain/netpolicy/` and `internal/adapters/outboundhttp/` locations in this refactor PR.
- Run `just verify`. All existing tests must pass before Phase 1.

### Phase 1: Setup

- Add `tests/e2e/helpers/mock_protected_resource_provider.go`. It serves challenges, both PRM locations, all AS metadata locations, DCR, authorization, and token endpoints. It records request order, registration count, authorization queries, and token forms without logging secrets.
- Add a TLS fake-public-host transport seam in `tests/e2e/bootstrap/protected_resource.go`.
- Before the red gate, add `Builder.WithOAuthDiscoveryClient(ports.OAuthDiscoveryClient)` and `Builder.WithDiscoveryTokenHTTPClient(*http.Client)`. The first option follows the existing CIMD fetcher injection pattern. The second supplies only discovery-backed code-exchange and refresh calls, never manual requests. The E2E bootstrap wraps one fake-host HTTP client for both options. The production Builder sets neither option and uses the guarded dialer.
- Add fixtures for CIMD-only, CIMD-plus-DCR, confidential DCR, public DCR, multi-issuer, and rejection metadata.

### Phase 2: Design Preconditions

#### Phase 2a: Domain Model and Glossary

- Apply [data-model.md](./data-model.md).
- Add glossary terms and the outbound discovery flow to `ARCHITECTURE.md`.
- Draft and accept ADR 038.

#### Phase 2b: Configuration Design

- Document the new optional `third_party_oauth2.client_name` key. DCR rejects an absent or blank value before registration; startup, manual services, and CIMD remain available.
- Keep the 15-second and 256-KiB discovery limits fixed. Hosted CIMD needs an HTTPS `server.enduser.public_url`.
- Add `examples/config/protected-resource-discovery.yaml` with the broker client name and reference it from `examples/config/README.md`.
- Add the broker client name to the Helm values, schema, ConfigMap, and README. Keep the existing public-URL mapping.
- The chart already forwards `broker.server.enduser.publicUrl` to `server.enduser.public_url`. The new `broker.thirdPartyOauth2.clientName` key forwards nonempty names to `third_party_oauth2.client_name`.

#### Phase 2c: API Design

- Apply [contracts/admin-api.md](./contracts/admin-api.md) to `api/admin/openapi.yaml`.
- Record stakeholder review of the response method values, `discovery.client_method`, and failure codes in the PR.
- Update `docs/reference/api.md` and `docs/guides/manage-agents-and-services.md`.

#### Phase 2d: Database Design

- Add migration 036.
- The down migration refuses rollback while discovery-backed rows exist. Then it restores the migration 035 check and removes the new index and columns.
- Add migration tests for existing rows, new rows, guarded rollback, clean rollback, and replay.

#### Phase 2e: Frontend and Design System Review

- No React or administrative UI work is in scope. No design-system, Playwright, or screenshot task applies.

#### Phase 2f: E2E Acceptance Test Design

- Add all 41 functional `It()` blocks and the separate SC-006 performance block before production behavior.
- Give each functional block `Label("protected-resource-discovery")`.
- Add `Label("docker")` to US2-S3 and US4-S5.
- Run the functional filter and record exactly 41 compiling semantic-red results. Run the single-process performance filter and record one semantic-red SC-006 result. Missing Builder symbols or broken fake-host transport do not count as semantic failures.

### Phase 2.5: Foundational Infrastructure

- Complete safe port errors, model fields, adapter constructors, repository methods, and handler/route structure for focused foundation and story tests. The earlier HTTP-level E2E gate compiles with the minimal Phase 1 port and Builder seams.
- Implement the outbound adapter, domain discovery logic, storage changes, and Builder wiring after their focused tests fail semantically.

### Phase 2.7: Entity Boilerplate

**Skip.** The feature extends the existing service aggregate. It adds no independent CRUD entity.

### Phase 3 and later: User stories

1. US1: Resource discovery, issuer selection, metadata order, hosted CIMD, and derived resource.
2. US2: DCR selection, deployment-wide client name, encrypted credentials, explicit token methods, and resource renewal.
3. US3: Fail-closed rejection, absent DCR client name, SSRF controls, identity checks, and duplicate DCR identities.
4. US4: Durable status, failed-refresh isolation, refresh preservation, and override transitions.

### Phase N: Constitution Compliance Verification

- Verify ADR 038, OpenAPI, glossary, SC-006 and security NFRs in `ARCHITECTURE.md`, migration tests, audit safety, Builder wiring, the focused status writer in `internal/ports/storage.go`, and both storage adapters.
- Run `just check`, the focused E2E command, `just test`, `just test-integration`, and `just test-integration-infra`.
- Run `just verify` as the final gate.

## Testing Strategy

### End-to-end acceptance tests

**Location**: Four functional files and one performance file under `tests/e2e/`.

**Framework**: Ginkgo v2 and Gomega. Every test uses one production `app.Builder` with both Admin and End-user test servers.

**Labels**: Every functional block uses `Label("protected-resource-discovery")`. The PostgreSQL restart scenarios also use `docker`. The SC-006 block uses both `protected-resource-discovery` and `performance`. This command selects exactly 41 functional blocks:

```sh
ginkgo -v --label-filter="protected-resource-discovery && !performance" ./tests/e2e/
```

**Provider double**: Each scenario starts its own `MockProtectedResourceProvider`. The double validates PKCE and selected client authentication. It records exact request order, DCR count, authorization `resource` values, and token-form `resource` values. It never logs secrets, assertions, codes, verifiers, or tokens.

**Transport rule**: T006 adds compile-only discovery-port and token-client Builder options before the red gate. The E2E bootstrap wraps a per-scenario fake-host HTTP client in the outbound adapter and injects that port. The adapter still validates URLs, rejects redirects, and bounds metadata bodies. The injected client skips the production dial-time IP block so it can reach TLS `httptest` servers on loopback. T052 wires both options to discovery and discovery-backed code exchange and refresh. US3-S3 sets neither option and tests the production guard before connect.

**Restart rule**: US2-S3 and US4-S5 use PostgreSQL. They close the first server and adapter, open a new adapter for the same database, and build a new app.

**Scenario mapping**:

| Spec scenario | E2E location | Test description |
|---|---|---|
| US1-S1 | `thirdparty_protected_resource_discovery_test.go` | Saves the single advertised issuer and its endpoints without `issuer_uri`. |
| US1-S2 | `thirdparty_protected_resource_discovery_test.go` | Selects hosted CIMD and makes zero DCR requests when both methods exist. |
| US1-S3 | `thirdparty_protected_resource_discovery_test.go` | Sends the configured resource once on authorization and code exchange with PKCE S256. |
| US1-S4 | `thirdparty_protected_resource_discovery_test.go` | Uses only endpoints from the administrator-selected issuer. |
| US1-S5 | `thirdparty_protected_resource_discovery_test.go` | Accepts matching metadata from one `resource_metadata` value in a `401` Bearer or DPoP challenge. |
| US1-S6 | `thirdparty_protected_resource_discovery_test.go` | Uses path-inserted OpenID metadata before path-appended OpenID metadata. |
| US1-S7 | `thirdparty_protected_resource_discovery_test.go` | Uses path-appended OpenID metadata after two `404` responses. |
| US1-S8 | `thirdparty_protected_resource_discovery_test.go` | Uses root OpenID metadata after root OAuth metadata returns `404`. |
| US1-S9 | `thirdparty_protected_resource_discovery_test.go` | Uses path-specific PRM before root PRM without a challenge. |
| US1-S10 | `thirdparty_protected_resource_discovery_test.go` | Returns the verified resource URL as derived `authorization_params.resource`. |
| US2-S1 | `thirdparty_protected_resource_dcr_test.go` | Stores one working DCR identity from a DCR-only server. |
| US2-S2 | `thirdparty_protected_resource_dcr_test.go` | Connects a public DCR client with PKCE and no secret. |
| US2-S3 | `thirdparty_protected_resource_dcr_test.go` | Renews a confidential client with an issued refresh token and valid credential after PostgreSQL restart, without disclosing its secret. |
| US2-S4 | `thirdparty_protected_resource_dcr_test.go` | Sends the configured resource once on DCR session renewal. |
| US2-S5 | `thirdparty_protected_resource_dcr_test.go` | Sends an explicit resource override exactly once on connection and renewal. |
| US2-S6 | `thirdparty_protected_resource_dcr_test.go` | Shows an explicit resource override separately from `discovery.resource_url`. |
| US2-S7 | `thirdparty_protected_resource_dcr_test.go` | Registers a confidential client when confidential and public DCR exist. |
| US2-S8 | `thirdparty_protected_resource_dcr_test.go` | Registers a public PKCE client when only public DCR exists. |
| US2-S9 | `thirdparty_protected_resource_dcr_test.go` | Keeps identical DCR client IDs usable under different issuers. |
| US2-S10 | `thirdparty_protected_resource_dcr_test.go` | Sends the configured broker client name instead of the service display name. |
| US3-S1 | `thirdparty_protected_resource_rejection_test.go` | Rejects missing PRM even with manual endpoint values and creates no service. |
| US3-S2 | `thirdparty_protected_resource_rejection_test.go` | Rejects resource or issuer mismatch before registration. |
| US3-S3 | `thirdparty_protected_resource_rejection_test.go` | Blocks an internal destination before connect and creates no service. |
| US3-S4 | `thirdparty_protected_resource_rejection_test.go` | Rejects no compatible CIMD or DCR method without static fallback. |
| US3-S5 | `thirdparty_protected_resource_rejection_test.go` | Rejects unavailable selected CIMD without trying DCR. |
| US3-S6 | `thirdparty_protected_resource_rejection_test.go` | Rejects multiple issuers without an administrator selection. |
| US3-S7 | `thirdparty_protected_resource_rejection_test.go` | Rejects the first issuer mismatch without trying lower-priority metadata. |
| US3-S8 | `thirdparty_protected_resource_rejection_test.go` | Fails a rejected resource without a broader token request. |
| US3-S9 | `thirdparty_protected_resource_rejection_test.go` | Rejects an invalid explicit resource and stores no service. |
| US3-S10 | `thirdparty_protected_resource_rejection_test.go` | Rejects simultaneous `resource_url` and `metadata_url`. |
| US3-S11 | `thirdparty_protected_resource_rejection_test.go` | Rejects failed confidential DCR without a public retry. |
| US3-S12 | `thirdparty_protected_resource_rejection_test.go` | Rejects a same-issuer duplicate DCR client ID without changing credentials. |
| US3-S13 | `thirdparty_protected_resource_rejection_test.go` | Rejects DCR without a non-blank broker client name before registration; leaves manual and CIMD available. |
| US4-S1 | `thirdparty_protected_resource_status_test.go` | Reports ready status, resource, issuer, method, and success time without secrets. |
| US4-S2 | `thirdparty_protected_resource_status_test.go` | Keeps active settings after failed refresh and reports separate failure and success. |
| US4-S3 | `thirdparty_protected_resource_status_test.go` | Reports `not_applicable` for a manual service and keeps its connection behavior. |
| US4-S4 | `thirdparty_protected_resource_status_test.go` | Updates endpoints and keeps the client identity for unchanged issuer and method. |
| US4-S5 | `thirdparty_protected_resource_status_test.go` | Keeps failed and successful outcomes after PostgreSQL restart and uses the prior client. |
| US4-S6 | `thirdparty_protected_resource_status_test.go` | Replaces a derived resource after a same-issuer discovery URL change. |
| US4-S7 | `thirdparty_protected_resource_status_test.go` | Keeps an explicit resource override after refresh. |
| US4-S8 | `thirdparty_protected_resource_status_test.go` | Restores the verified resource after a replacement map omits `resource`. |

**Red phase**: Each functional block must compile and assert concrete HTTP status, JSON fields, provider request order, request counts, or persisted outcomes. The SC-006 block must assert the 19-of-20/five-second result before production behavior. Neither filter may fail due to missing Builder symbols or broken fixtures. Do not skip cases or mark them as red in test comments.

### Performance measurement

Write `thirdparty_protected_resource_performance_test.go` in Phase 2f and record its semantic-red result. It creates 20 independent providers and services. Each provider delays the challenge, PRM, AS metadata, and DCR responses by one second. Run registrations concurrently in one Ginkgo process. At least 19 must complete within five seconds. Re-run the unchanged test in Phase N.

### Unit and adapter tests

| Layer | Coverage |
|---|---|
| `internal/domain/netpolicy` | Existing blocked ranges and operator CIDR behavior after the move. |
| `internal/domain/model` | Field combinations, explicit resource validation, token endpoint query rule, DCR methods, copying, and redaction. |
| `internal/domain/thirdparty` | Probe order, `404` fallback, `401` challenge cardinality, identity checks, issuer selection, method preference, no fallback, status transitions, audit safety, and issuer-change session rule. DCR response tests include missing refresh grants and rejection of non-zero secret expiry. |
| `internal/domain/oauth2session` | Exact basic/post/public/CIMD wire forms, one resource value, per-service guarded-client selection for code exchange and refresh, unchanged shared-client use for manual services, `invalid_target`, no stored token after rejection, and terminal renewal when the provider withholds a refresh token or rejects the stored credential. |
| `internal/adapters/outboundhttp` | Dial-time blocking, redirect rejection, HTTPS validation, size limit, deadline, challenge extraction, 404 sentinel, and no body leakage. Builder tests confirm separate shared and guarded transports, OTel wrapping, and idle-connection cleanup. |
| HTTP handlers | Request validation, error mapping, status responses, canonical ID resolution, and absent DCR secrets. |
| Memory adapter | Field persistence, success transaction semantics, stale failure writes, and DCR uniqueness. |

### Integration tests

- Extend `tests/integration/migrations/migrations_test.go` for migration 036 apply, guarded rollback, clean rollback, replay, and legacy rows.
- Extend `tests/integration/storage/infra/thirdparty_service_test.go` for all discovery-backed modes, DCR uniqueness, failure-only writes, and delete behavior.
- Run the same repository behavior tests against memory and PostgreSQL adapters.

### Manual smoke scenario

Use [quickstart.md](./quickstart.md) after automated tests pass. It covers hosted CIMD, DCR, resource binding, failure isolation, restart, rejection paths, and manual service compatibility.

## Complexity Tracking

No constitution violation needs a complexity exception.

ADR 038 is a required superseding decision, not an exception. It permits explicit `client_secret_basic` and `client_secret_post` only for DCR-issued clients. It keeps the ADR 036 behavior for manual services. The new outbound adapter package replaces duplicated SSRF code with one shared boundary.
