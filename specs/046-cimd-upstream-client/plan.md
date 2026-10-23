# Implementation Plan: CIMD Client Authentication for Third-Party OAuth2 Services

**Branch**: `046-cimd-upstream-client` | **Date**: 2026-09-21 | **Spec**: [spec.md](./spec.md)

**Input**: Feature specification from `specs/046-cimd-upstream-client/spec.md`

## Summary

Extend third-party OAuth2 services with an explicit `private_key_jwt` confidential-client mode.
The broker assigns a stable HTTPS client ID URL. It serves public metadata and ES256 JWKs. It signs outbound token requests with a dedicated CIMD key domain.

The design preserves the current public `none` mode and static confidential mode.
It keeps inbound CIMD behavior, token-signing keys, `/oauth2/jwks.json`, and token-key routes separate.

The key design is a persisted `key_domain` discriminator on the existing `signing_keys` model.
It provides independent key lifecycle rules while keeping global `kid` uniqueness.

## Technical Context

**Language/Version**: Go 1.26.8

**Primary Dependencies**: `golang.org/x/oauth2` v0.37.0, `github.com/lestrrat-go/jwx/v4` v4.5.0, Go `crypto/ecdsa`, chi v5, sqlx, Viper, Ginkgo v2, Gomega, testify

**Storage**: PostgreSQL with sqlx in production and in-memory storage for development and E2E tests. Existing key material uses the configured `EncryptionPort` and branch-key manager.

**Testing**: Go unit tests, adapter tests, PostgreSQL integration tests with testcontainers, and Ginkgo/Gomega production-bootstrap E2E tests.

**Target Platform**: Linux broker service in production. Darwin is a supported development platform.

**Project Type**: Backend feature in a Go monorepo. It changes administrative and anonymous HTTP contracts. It has no React UI change.

**Performance Goal**: In the defined normal-load measurement, 100 successful anonymous requests to each public metadata and CIMD JWK route run with ten concurrent clients after one warm-up request per route. The p95 retrieval latency for each route is less than one second. Both responses use `Cache-Control: public, max-age=300`.

**Constraints**:

- `private_key_jwt` is explicit. It is never inferred from missing input or provider metadata.
- The client ID URL is HTTPS and equals `<server.enduser.public_url>/.well-known/oauth-client/<service-id>`.
- The dedicated key domain uses ES256 only. It never signs broker access tokens.
- The token-signing domain never signs CIMD client assertions.
- The public metadata and JWK routes reveal only public information.
- Assertion failures and key failures stop the affected request. They never downgrade authentication.
- The feature supports `proxy`, `local`, and `hybrid` OAuth server modes.
- The existing proxy-mode restriction for inbound CIMD remains unchanged.
- The broker uses the existing encryption port and branch-key lifecycle. It adds no cryptographic implementation.
- Builder bootstraps the first CIMD key only when persisted CIMD services exist. Otherwise, an operator provisions the first key through the dedicated route.

**Scale/Scope**: One service-authentication state, one broker-global key domain, four new HTTP route groups, two OpenAPI surfaces, two reversible database changes, 31 acceptance scenarios, and one SC-008 performance measurement.

**NEEDS CLARIFICATION**: None. [research.md](./research.md) resolves all design questions.

## Constitution Check

*GATE: Pass before Phase 0 research. Re-check after Phase 1 design.*

### Pre-Phase 0 gate

| Check | Status | Plan |
|---|---|---|
| Domain model | Pass | [data-model.md](./data-model.md) defines the service state, key domain, public document, and assertion. |
| Domain concepts | Pass | Add CIMD confidential service, CIMD key, Client Assertion, and Key Domain to `ARCHITECTURE.md`. |
| Entity IDs | Pass | The feature extends the existing signing-key entity and `SigningKeyID`. It introduces no new UUID entity type. |
| Configuration design | Pass | No new configuration parameter is required. The feature requires an existing HTTPS `server.enduser.public_url`. |
| Configuration examples | Pass | Document the HTTPS precondition and add a relevant example to `examples/config/`. |
| Helm chart | Not applicable | No chart value, template input, or configuration parameter changes. |
| API design first | Pass | The stakeholder approved the service, public document, and key routes in the specification clarifications. |
| OpenAPI documentation | Pass | Update `api/admin/openapi.yaml` and `api/enduser/openapi.yaml` from the contracts in `contracts/`. |
| Database design | Pass | Add reversible go-migrate files after migration 031. |
| E2E acceptance tests | Pass | Write 31 production-bootstrap Ginkgo tests before feature implementation. |
| E2E mapping | Pass | The Testing Strategy maps every acceptance scenario to one `It()` block. |
| E2E red phase | Pass | Each test must compile and fail on an observable contract before production behavior changes. |
| Frontend Playwright E2E | Not applicable | The specification excludes a new administration UI and changes no React surface. |
| Frontend screenshots | Not applicable | The feature changes no UI state. |

### Implementation considerations

| Check | Status | Plan |
|---|---|---|
| Security-first | Pass | Validate service state before endpoint discovery. Use fail-closed key selection and assertion signing. Do not use a credential fallback. |
| Architecture documentation | Pass | Update route inventory, key-domain boundary, outbound CIMD behavior, glossary entries, and the SC-008 load profile and p95 target in `ARCHITECTURE.md`. |
| ADRs | Pass with precondition | Add an ADR for the distinct CIMD client-authentication key domain. The ADR must be accepted before implementation begins. |
| Library-first security | Pass | Use Go `crypto/ecdsa` and JWX v4. Keep encryption behind the existing `EncryptionPort`. |
| Zalando API guidance | Pass | Reuse current resource names, HTTP status handling, JSON error envelopes, and strong ETag behavior. |
| End-user documentation | Pass | Build Redocusaurus from the end-user OpenAPI input. Update the API reference and service and mode guides. |
| Migration testing | Pass | Test apply, guarded rollback, replay, and adapter round trips against PostgreSQL. |
| Hexagonal architecture | Pass | Domain services depend on narrow ports. HTTP handlers parse and format only. The builder performs all wiring. |
| Persistence patterns | Pass | Extend ports, both adapters, migrations, and integration coverage together. |
| Configuration and Helm | Verified | `internal/ports/config.go`, the Helm values, and templates already expose the existing public URL. CIMD adds no configuration input or chart change. |

### Post-design re-check

The gate passes after Phase 1.

- The data model uses the existing service and signing-key entities without a new unrelated aggregate.
- The public and administrative contracts are explicit in `contracts/`.
- The design keeps public routes anonymous and keeps administrative mutations behind the existing operator boundary.
- The design has no unresolved security, API approval, configuration, storage, UI, or test-traceability question.
- The feature needs an ADR because the new key domain is an architectural boundary. The plan includes that ADR before code changes.

## Project Structure

### Documentation for this feature

```text
specs/046-cimd-upstream-client/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
└── contracts/
    ├── admin-api.md
    └── enduser-api.md
```

### Source code and documentation affected

```text
api/
├── admin/openapi.yaml                         # Service and CIMD key admin contract
└── enduser/openapi.yaml                       # Anonymous metadata and JWK contracts

internal/
├── app/
│   ├── builder.go                             # CIMD domain DI in all OAuth modes
│   └── handlers.go                            # Dedicated handler fields
├── adapters/
│   ├── http/
│   │   ├── handlers/admin/                    # CIMD key and service handlers
│   │   ├── handlers/enduser/                  # Public metadata and JWK handlers
│   │   └── routing/                           # Flat admin and .well-known routes
│   └── storage/
│       ├── memory/                            # Domain-filtered signing key persistence
│       └── postgres/                          # Domain-filtered signing key persistence
├── domain/
│   ├── cimdclient/                            # Outbound key, assertion, and metadata services
│   ├── encryption/                            # CIMD key subject namespace
│   ├── model/                                 # Three-state service authentication model
│   ├── oauth2session/                         # Code and refresh assertion injection
│   ├── oauth2server/                          # Reused lifecycle mechanics only
│   └── storage/                               # Key-domain data model
└── ports/
    ├── cimd_client.go                         # Narrow lifecycle, signer, and metadata ports
    └── storage.go                             # Domain-scoped key repository operations

migrations/
├── 034_add_cimd_key_domain.{up,down}.sql       # Shared key table domain boundary
└── 035_add_cimd_private_key_jwt_authentication.{up,down}.sql

docs/
├── reference/api.md
└── guides/
    ├── manage-agents-and-services.md
    └── operate-oauth2-server-modes.md

examples/config/
└── *.yaml                                    # HTTPS end-user public URL example

tests/
├── e2e/
│   ├── helpers/mock_cimd_upstream.go          # Conformant private_key_jwt provider double
│   ├── thirdparty_cimd_service_test.go
│   ├── thirdparty_cimd_metadata_test.go
│   ├── thirdparty_cimd_metadata_performance_test.go # SC-008 p95 measurement
│   ├── thirdparty_cimd_authentication_test.go
│   └── thirdparty_cimd_rotation_test.go
└── integration/
    ├── migrations/
    ├── storage/infra/
    └── infra/
```

**Structure Decision**: Create `internal/domain/cimdclient/` as an independent outbound-client bounded context. It owns only CIMD key selection, assertion construction, and hosted-document composition. It does not reuse the inbound `internal/domain/oauth2/cimd/` parser or fetcher. Existing storage and key lifecycle mechanics become domain-scoped rather than copied.

## Design and Implementation Approach

### 1. Preserve a three-state service model

Extend `TokenEndpointAuthMethod` with `private_key_jwt`.
Add separate predicates for public, CIMD confidential, and shared-secret requirements.
Do not use `!IsPublicClient()` as a proxy for “has a secret.”

The administrative handler must reject a private-key request that includes a client ID or a non-empty secret.
It must assign the client ID only after it has the immutable service ID.
It must validate the entire replacement before it removes a static secret.

The provider service and both storage adapters must represent a CIMD service as an absent secret.
Read and list responses must omit `client_secret`, not redact it.
`ThirdpartyOAuth2ProviderService.Get` and `List` skip secret decryption when `Secret` is absent for a CIMD service.

### 2. Scope the existing key lifecycle by purpose

Add a closed `KeyDomain` value to existing signing-key records.
Backfill existing rows as `token_signing`.
Make repository queries, bootstrap counts, row locks, promotion, deletion, active lists, and current-key lookup domain-specific.
The in-memory `SigningKeyStore` and PostgreSQL `SigningKeyRepo` apply every query and lifecycle guard with the key domain.

Keep a global unique `kid` constraint.
Use a CIMD-specific branch-key subject and encryption context namespace.
Replace the existing one-current global index with a one-current-per-domain index.

Create a dedicated CIMD key manager with separate interfaces for:

- Administrative lifecycle operations.
- Public CIMD JWK construction.
- Client assertion signing.

Do not pass the token-key manager into CIMD code.
Do not pass the CIMD manager into the token strategy or aggregated JWK publisher.

### 3. Build and publish outbound client documents

Add an outbound metadata service that obtains a metadata-safe service projection.
It derives the client ID URL, callback URI, and JWK URI from the configured end-user public URL and service ID.
The advertised callback is `<server.enduser.public_url>/api/third-party/<service-id>/oauth2/callback`.

Register the two anonymous routes under `/.well-known`.
Return JSON and the exact five-minute public cache policy.
Return one generic JSON `404` response for all unavailable service states.
No inactive service state is introduced. The metadata service serves only existing, non-deleted CIMD confidential services with usable public-key publication. It returns the same JSON `404` response for every other state.

Build the CIMD JWK Set only from active CIMD key-domain rows.
Include public ES256 keys with `kid`, `alg`, `use: sig`, P-256 coordinates, and no private member.

### 4. Authenticate code and refresh requests with assertions

Add a narrow `CIMDClientAssertionSigner` dependency to `OAuth2SessionService`.
It accepts only the broker client ID and configured token endpoint.

For a CIMD confidential service:

- Keep the existing authorization URL and mandatory PKCE S256 challenge.
- Use an empty client secret and `AuthStyleInParams` for code exchange.
- Generate the assertion inside every retry attempt.
- Pass the new pair with `oauth2.SetAuthURLParam` to `Config.Exchange`. Do not use config fields as an assertion channel.
- Add exactly one JWT-bearer assertion type and assertion pair to the form.
- Use the manual refresh form and a new assertion for each refresh call.
- Reject a signing or key-selection error before sending a token request.
- Send no `ClientSecret` value or `Authorization` header for either private token request path.

Keep public and static token behavior byte-for-byte compatible.
Do not change inbound `FositeStorage.ClientAssertionJWTValid`.

### 5. Wire all modes and record safe audit data

Build the CIMD key manager, signer, metadata service, and public handlers before the local or proxy mode split.
Expose `/api/cimd-client-keys` in `proxy`, `local`, and `hybrid`.
Keep `/api/oauth2-server/signing-keys` unavailable in proxy mode.

When persisted CIMD services exist at startup, Builder calls the CIMD equivalent of `EnsureInitialKey`. The first key becomes usable immediately. Builder fails startup if it cannot produce or publish that key.
When no CIMD service exists, startup remains available and Builder creates no key. An operator provisions the first key through the dedicated lifecycle route. Registration must not create a key as a side effect.

Add structured audit events for registration, update, metadata access, key lifecycle, signing, and rejection.
Record identifiers and outcomes only.

## Implementation Phase Overview

### Phase 0: Pre-implementation refactoring

**Include.** Submit this phase separately because it changes a shared key lifecycle without adding CIMD HTTP behavior.

- Draft and obtain acceptance of ADR 037 before T003 starts. The ADR defines the CIMD key-domain boundary for this separate refactoring delivery.
- Add `key_domain` to the existing key model and repositories.
- Backfill token-signing rows and scope existing token behavior to `token_signing`.
- Replace the global current-key constraint with a per-domain constraint.
- Add regression tests proving token issuance and `/oauth2/jwks.json` stay unchanged.
- Run `just verify`. All existing tests must pass before Phase 1 begins.

### Phase 1: Setup

- Add the test-only conformant CIMD provider double and public HTTPS test bootstrap seam.
- Add focused fixtures for each service authentication state and each OAuth mode.

### Phase 2: Design Preconditions

#### Phase 2a: Domain Model and Glossary

- Apply [data-model.md](./data-model.md) to the domain model.
- Update `ARCHITECTURE.md` with the key domain, outbound flow, route inventory, and glossary terms.

#### Phase 2b: Configuration Design

- Document the HTTPS requirement for `server.enduser.public_url`.
- Update a relevant `examples/config/` YAML file with a stable HTTPS public URL.
- Record that Helm changes are not necessary because no configuration input changes.

#### Phase 2c: API Design

- Apply [contracts/admin-api.md](./contracts/admin-api.md) as a narrative delta to the authoritative `api/admin/openapi.yaml`.
- Apply [contracts/enduser-api.md](./contracts/enduser-api.md) as a narrative delta to the authoritative `api/enduser/openapi.yaml`.
- Update API reference and operator guides.
- Build the Redocusaurus documentation site from the OpenAPI files.

#### Phase 2d: Database Design

- Add `034_add_cimd_key_domain`. Its up migration adds `key_domain`, backfills `token_signing`, and replaces the global current-key index with a per-domain index.
- The `034` down migration refuses rollback while `cimd_client_authentication` key rows exist. It then restores the token-only domain check and global current-key index.
- Add `035_add_cimd_private_key_jwt_authentication`. Its up migration extends the service-auth CHECK for secretless `private_key_jwt` services.
- The `035` down migration refuses rollback while a `private_key_jwt` service exists. It then restores the migration 031 CHECK.
- Add migration tests for existing static and public rows, CIMD rows, both guarded rollbacks, and migration replay.

#### Phase 2e: Frontend and Design System Review

- The approved scope has no React or administrative UI change. No design-system component, Playwright scenario, or screenshot task applies.

#### Phase 2f: E2E Acceptance Test Design

- Add the 31 Ginkgo `It()` blocks before production behavior changes. Give every functional block `Label("cimd-upstream-client")` and a nearby `US#-S# from specs/046-cimd-upstream-client/spec.md` comment.
- Add `tests/e2e/thirdparty_cimd_metadata_performance_test.go` with `Label("cimd-upstream-client", "performance")`. It must measure the SC-008 workload and p95 target.
- Run `ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/` and record 31 semantic-red functional scenarios. Run the SC-008 test separately with one Ginkgo worker.

### Phase 2.5: Foundational Infrastructure

- Add compile-only contracts and inert stubs before semantic-red tests use new CIMD symbols. Those declarations must compile and must not implement required behavior.
- Add domain-scoped key ports, repositories, adapters, encryption subjects, and readiness coordination.
- Add the dedicated CIMD manager, assertion signer, metadata service, handlers, route fields, and builder wiring after their tests fail semantically.

### Phase 2.7: Entity Boilerplate

**Skip.** The feature extends the existing service and signing-key entity families. It does not add a new independent CRUD aggregate.

### Phase 3 and later: User stories

- Implement P1 service registration and representation.
- Implement P1 public metadata and JWK publication.
- Implement P1 code exchange and refresh assertions.
- Implement P2 lifecycle rotation and all-mode administration.
- Implement P3 posture review through existing read and list surfaces.

### Phase N: Constitution compliance verification

- Review each principle with the completed code, contracts, migrations, tests, and documentation.
- Confirm the accepted ADR, API documentation, audit behavior, key separation, migration safety, and all 31 passing scenarios.

## Testing Strategy

### End-to-end acceptance tests

**Location**: Four functional scenario files plus `thirdparty_cimd_metadata_performance_test.go` under `tests/e2e/`.

**Framework**: Ginkgo v2 and Gomega. Tests use `app.Builder` through `tests/e2e/bootstrap/` with fresh storage and servers for each scenario.

Every functional feature `It()` uses `Label("cimd-upstream-client")`. The command `ginkgo -v --label-filter="cimd-upstream-client && !performance" ./tests/e2e/` must select exactly the 31 mapped scenarios. The SC-008 test uses both `cimd-upstream-client` and `performance` labels.

**Provider double**: `tests/e2e/helpers/mock_cimd_upstream.go` fetches the broker metadata and JWK Set. It validates ES256 assertions, request cardinality, claims, PKCE, and key rotation. It rejects secrets and invalid assertions. It must not log assertion, secret, verifier, code, or token values.

**Scenario mapping**:

| Spec scenario | E2E location | Test description |
|---|---|---|
| US1-S1 | `thirdparty_cimd_service_test.go` | Creates a CIMD confidential service without an operator client ID or secret. |
| US1-S2 | `thirdparty_cimd_service_test.go` | Rejects a CIMD request with an operator client ID or shared secret. |
| US1-S3 | `thirdparty_cimd_service_test.go` | Preserves static confidential registration when the method is absent. |
| US1-S4 | `thirdparty_cimd_service_test.go` | Preserves public registration when the method is `none`. |
| US1-S5 | `thirdparty_cimd_service_test.go` | Rejects a credential-derived provider flavor for CIMD authentication. |
| US2-S1 | `thirdparty_cimd_metadata_test.go` | Returns metadata whose client ID equals the requested URL. |
| US2-S2 | `thirdparty_cimd_metadata_test.go` | Returns the callback URI, private-key method, and JWK URL. |
| US2-S3 | `thirdparty_cimd_metadata_test.go` | Exposes public metadata and public JWK material only. |
| US2-S4 | `thirdparty_cimd_metadata_test.go` | Does not expose CIMD identity for public or static services. |
| US2-S5 | `thirdparty_cimd_metadata_test.go` | Returns JSON and the five-minute cache header for an existing, non-deleted CIMD service with a usable published key. |
| US2-S6 | `thirdparty_cimd_metadata_test.go` | Returns JSON 404 for each unavailable service state. |
| US2-S7 | `thirdparty_cimd_metadata_test.go` | Keeps CIMD and token JWK key IDs disjoint. |
| US3-S1 | `thirdparty_cimd_authentication_test.go` | Sends the broker URL client ID and preserves PKCE on authorization. |
| US3-S2 | `thirdparty_cimd_authentication_test.go` | Exchanges a code with a valid assertion and no secret. |
| US3-S3 | `thirdparty_cimd_authentication_test.go` | Refreshes with a valid assertion and stores encrypted replacement tokens. |
| US3-S4 | `thirdparty_cimd_authentication_test.go` | Fails closed after signing or provider assertion rejection. |
| US3-S5 | `thirdparty_cimd_authentication_test.go` | Preserves public and static authorization and refresh behavior. |
| US3-S6 | `thirdparty_cimd_authentication_test.go` | Sends one valid assertion pair with the required claims and key. |
| US3-S7 | `thirdparty_cimd_authentication_test.go` | Uses the CIMD key domain in proxy mode. |
| US3-S8 | `thirdparty_cimd_authentication_test.go` | Prevents cross-use between client assertion and access-token keys. |
| US4-S1 | `thirdparty_cimd_rotation_test.go` | Keeps metadata JWK URL stable and publishes a rotated key first. |
| US4-S2 | `thirdparty_cimd_rotation_test.go` | Lets a cached provider obtain and use a newly active key. |
| US4-S3 | `thirdparty_cimd_rotation_test.go` | Refuses a CIMD token request when no usable key exists. |
| US4-S4 | `thirdparty_cimd_rotation_test.go` | Publishes a generated key during activation grace without signing with it. |
| US4-S5 | `thirdparty_cimd_rotation_test.go` | Activates an operator-promoted key immediately. |
| US4-S6 | `thirdparty_cimd_rotation_test.go` | Activates the first CIMD key immediately. |
| US4-S7 | `thirdparty_cimd_rotation_test.go` | Serves CIMD key routes in all modes and isolates token-key routes in proxy mode. |
| US4-S8 | `thirdparty_cimd_rotation_test.go` | Rejects every non-ES256 key-generation request. |
| US5-S1 | `thirdparty_cimd_service_test.go` | Lists all service authentication postures without ambiguity. |
| US5-S2 | `thirdparty_cimd_service_test.go` | Reads a CIMD service with its URL and without a secret. |
| US5-S3 | `thirdparty_cimd_service_test.go` | Converts static confidential authentication to CIMD and removes the secret. |

### Performance measurement

The SC-008 test warms each anonymous route once. It then sends 100 requests to each route with ten concurrent clients. Every request must return `200 OK`. It runs with one Ginkgo worker and records a separate p95 for each route. Each p95 must be less than one second.

### Unit and adapter tests

| Layer | Coverage |
|---|---|
| `internal/domain/model` | Authentication state validation, private-mode contradictions, Google rejection, copy and redaction semantics. |
| `internal/domain/thirdparty` | Validation before discovery, generated identity, secret removal after valid replacement, structured audit data. |
| `internal/domain/cimdclient` | ES256-only key selection, metadata composition, public JWK filtering, assertion claims, fresh JTI, short expiry, no token-key use, and credential-free key-lifecycle audit events. |
| `internal/domain/oauth2session` | In-parameter code exchange, per-retry assertion creation, manual refresh assertion, no secret or header, no downgrade, and safe errors. |
| HTTP handlers | Operator principal checks, error mapping, cache headers, JSON 404 response, and no private material. |
| Memory and PostgreSQL adapters | Domain-scoped key queries, current-key isolation, secret absence, and service identity persistence. |

### Integration tests

- Extend `tests/integration/migrations/migrations_test.go` for apply, guarded rollback, replay, and legacy row compatibility.
- Extend `tests/integration/storage/infra/thirdparty_service_test.go` for all service modes and update transitions.
- Add a PostgreSQL test for conditional CIMD key bootstrap, based on the token-key bootstrap pattern.
- Test both storage adapters against the same key-domain and service-secret invariants.

### Manual smoke scenario

Use [quickstart.md](./quickstart.md) after the automated tests pass.
It exercises service registration, anonymous document access, complete connect and refresh, rotation, and all OAuth modes.

## Complexity Tracking

No constitution violation needs a complexity exception.

The feature adds a bounded key domain because the specification requires separate trust surfaces. The shared signing-key table avoids a second lifecycle implementation while retaining a database-enforced global key-ID boundary.

## Increment 2 (issue #264)

**Date**: 2026-10-22 | **Scope**: Augment feature 046 in place. This section does not change the original 046 plan.

### Summary and technical context delta

Add `client_registration` (`static` or `cimd`) as a service setting independent of `token_endpoint_auth_method`. The four manual modes are static confidential, static public, CIMD confidential, and CIMD public. CIMD public uses the broker-hosted client identifier URL with `none` authentication. ServiceNow needs this explicit mode. CIMD draft-02 §4.1 excludes shared-secret CIMD methods, while §8.2 recommends authentication when available. `private_key_jwt` remains the recommended CIMD mode.

On create or full-replacement `PUT`, an omitted registration resolves to `cimd` for `private_key_jwt` and `static` otherwise. The broker never infers CIMD public. Read and list responses always include the resolved setting. Legacy rows with `private_key_jwt` become `cimd`. All other rows become `static`. Discovery reports the selected registration but never selects CIMD public.

The Go version for this increment is 1.27.3 (`go.mod`). Existing dependencies, Go crypto, PostgreSQL/sqlx, memory storage, and Ginkgo remain in use. No new dependency, configuration key, Helm value, frontend change, ExtProc change, or CIMD key-lifecycle change is required. The existing HTTPS public URL remains necessary for every CIMD service. The hosted document keeps `Cache-Control: public, max-age=300`. Providers can cache a prior variant for 300 seconds after a mode switch.

### Stakeholder gates

Issue #264 approves the field, the four-mode matrix, omission, read/list reporting, and legacy classification (API-013). Plan approval alone did not confirm D-A–D-D or accept ADR 043. On 2026-10-22, the stakeholder explicitly confirmed all four API-014 decisions and accepted ADR 043 (then numbered 042), including D-E–D-J and the D-I rolling-upgrade limitation. Both gates passed before Stage 2. The acceptance note and ADR index appear in `AGENTS.md`.

### Constitution Check — pre-design

| Principle | Status | Planned control |
|---|---|---|
| I. Security-first | Design passes, implementation gated | CIMD public is explicit and secretless. PKCE S256 and the hosted callback remain mandatory. No method fallback occurs after provider rejection. |
| II. Architecture and ADRs | Accepted 2026-10-22 | ADR 043 records D-A–D-J. Amend `ARCHITECTURE.md` and its glossary in the same PR. |
| IV. OpenAPI transparency | Confirmed 2026-10-22 | Specify the Admin 2.1.0 field and end-user 1.1.0 document variants before code. The stakeholder confirmed D-A–D-D. |
| V. Domain concepts | Planned | Add client registration, CIMD public service, and static public service to the model and architecture glossary. |
| VI. Hexagonal boundaries | Planned | Put registration resolution and validation in the domain. Keep HTTP handlers as parsers and storage behind existing ports. |
| VIII. TDD | Planned | Write unit, migration, adapter, and E2E tests first. Observe semantic red failures before implementing behavior. |
| IX. Persistence | Planned | Add reversible migrations 042 and 043, a separate constraint, both adapter changes, and PostgreSQL migration tests. |
| X. API-first | Confirmed 2026-10-22 | The stakeholder confirmed D-A–D-D before the OpenAPI cutover or implementation. |
| XII. Builder wiring | Planned | Builder checks every CIMD identity and bootstraps keys only for CIMD confidential services. Existing route ownership stays unchanged. |
| XIII. E2E traceability | Planned | Map US6 (6), US7 (6), and US8 (8) to exactly 20 new production-bootstrap `It()` blocks. |
| XIV. Reviewable journeys | Planned | Add six chronological journeys to `spec.md`, including RFC-style exchanges, configuration, and decoded assertion claims. No UI wireframes apply. |

### Affected files and design approach

| Area | Increment 2 changes |
|---|---|
| Planning and decisions | `specs/046-cimd-upstream-client/{spec,plan,research,data-model,quickstart}.md`, `contracts/{admin-api,enduser-api}.md`, `checklists/requirements.md`, `adrs/043-client-registration-independent-of-authentication.md`, and `AGENTS.md`. Append Increment 2 tasks to `tasks.md` only after the gates. |
| Architecture and guides | `ARCHITECTURE.md`, `examples/config/third-party-oauth2.yaml`, `docs/reference/api.md`, `docs/guides/{manage-agents-and-services,operate-oauth2-server-modes}.md`, and `docs/concepts/oauth2-server-modes.md`. No new configuration key or Helm change. |
| API contracts | `api/admin/openapi.yaml` adds a required response setting and an optional non-null create/PUT setting, plus the matrix and errors. `api/enduser/openapi.yaml` adds closed confidential/public document variants and the JWK Set `404`. Versions become 2.1.0 and 1.1.0 after confirmation. |
| Domain | Add `internal/domain/model/client_registration.go`. Update `thirdparty_oauth2_provider.go`, `internal/domain/thirdparty/{service,protected_resource_discovery}.go`, and `internal/domain/cimdclient/metadata_service.go`. Identity follows registration. Key readiness follows `private_key_jwt`. Secretless wire behavior follows `none`. |
| Ports and application | Update `internal/ports/cimd_client.go` for optional key members and `internal/app/builder.go` for conditional key bootstrap. Keep `internal/domain/oauth2session/**` behavior unchanged. |
| HTTP and storage | Update `internal/adapters/http/handlers/admin/services_handler.go` and both `internal/adapters/storage/{memory,postgres}/thirdparty_provider{,_record}.go`. The end-user CIMD metadata handler needs tests but no logic change. |
| Database | Add `migrations/042_add_thirdparty_client_registration.{up,down}.sql` and `043_validate_thirdparty_client_registration.{up,down}.sql`. Add `tests/integration/migrations/client_registration_migration_test.go`. |
| Tests | Add `internal/domain/model/client_registration_test.go` and the three `tests/e2e/thirdparty_{cimd_public_service,client_registration,cimd_public_authentication}_test.go` files. Extend domain, handler, adapter, Builder, session, and existing E2E exact-shape tests. Extend `tests/e2e/helpers/mock_cimd_upstream.go`. |

The domain resolves an omitted registration before validating the four-mode matrix. Explicit `null`, empty, or unknown values fail with `400`. Invalid combinations, Google CIMD requests, filesystem CIMD requests, and discovery requests that supply registration fail before provider traffic. Full replacement preserves the CIMD URL between confidential and public methods. A static/CIMD switch changes identity. A switch to CIMD confidential requires a usable key. A switch to static requires an operator `client_id` and, for static confidential, a secret. No runtime method fallback occurs. Existing sessions follow 046/051 behavior. The provider can reject an old refresh token after an identity switch, so the user reconnects.

Every CIMD row must have the exact broker client identifier URL at startup. Only CIMD confidential rows require CIMD key bootstrap or published-key readiness. The CIMD public document has exactly five members: `client_id`, `redirect_uris`, `grant_types`, `response_types`, and `token_endpoint_auth_method: none`. The broker serves it without keys. Its `jwks.json` returns the existing uniform JSON `404`. The public token exchange and refresh reuse `IsPublicClient()` and never send a secret, assertion, or Authorization header. Preserve existing public-client retries and session audit records. Lifecycle records use `CIMD public service`. The existing mode-neutral metadata-access record needs no duplicate.

The database column is `TEXT NOT NULL DEFAULT 'static'`. Migration 042 adds a separate `NOT VALID` registration constraint under a one-second `ACCESS EXCLUSIVE NOWAIT` lock. The constraint checks the matrix, absent CIMD secret, stored credential source, discovery method, and broker URL suffix. Migration 043 backfills `private_key_jwt` rows to `cimd` and validates under `SHARE UPDATE EXCLUSIVE NOWAIT`. The domain and startup check enforce the exact URL. The 042 down migration refuses CIMD public rows. Operators convert or delete those rows, use `migrate force 42` after the guarded failure, and retry. During rolling upgrade, old pods can fail closed on CIMD confidential creates and CIMD rewrites. Retry those writes after rollout. ADR 043 records this limitation. Do not add a database trigger.

### Phase 2 design preconditions (after both gates)

#### Phase 2a: Domain model and architecture

Add the glossary terms to `ARCHITECTURE.md`. Update its hosted-document and public-JWK trust-surface descriptions. Apply the independent registration model in [data-model.md](./data-model.md).

#### Phase 2b: Configuration design

Change the comment in `examples/config/third-party-oauth2.yaml` so the HTTPS public-URL precondition covers every `client_registration: cimd` service. Add no broker configuration key or Helm value.

#### Phase 2c: API design

Apply [contracts/admin-api.md](./contracts/admin-api.md) and [contracts/enduser-api.md](./contracts/enduser-api.md) to their OpenAPI files before implementation. Update `docs/reference/api.md`. Redocusaurus renders API pages from these files, so no hand-written `docs/api/` page is necessary. Document the full-replacement omission hazard for CIMD public services.

#### Phase 2d: Database design

Add migrations 042 and 043 with their guarded down paths. Write migration tests before storage implementation. Cover legacy classification, forbidden combinations, rollback refusal, and replay.

#### Phase 2e: Frontend and design system review

Record that this increment changes no frontend or UI state. No design-system component, Playwright test, or screenshot applies.

#### Phase 2f: E2E acceptance test design

Write all 20 `It()` blocks before production behavior changes. Use `Label("cimd-upstream-client", "cimd-public-client")` and a nearby `US#-S# from specs/046-cimd-upstream-client/spec.md` comment on each block. Use the production bootstrap. Add `WithPublicClientMode()` to the provider double. It checks `none`, the five-member document, PKCE, credential absence, and the exact client identifier URL. It never fetches a JWK Set or logs codes, verifiers, or tokens. Obtain compiled, semantic-red results, not placeholder failures.

### Implementation and testing strategy (Stage 2 only)

After design preconditions, add domain validation and predicates, then update service identity, metadata, conditional key readiness, ports, handlers, storage, and migrations. Update exact-shape assertions to include `client_registration`. Keep code exchange, refresh retry policy, key lifecycle, and metadata handler logic unchanged. Update operator and OAuth-mode guides, then do the Phase N constitution review.

| Layer | Acceptance coverage |
|---|---|
| Model and domain | Four valid manual pairs, two invalid pairs, omission, explicit invalid values, Google/filesystem, discovery selection, stored consistency, identity transitions, startup URL check, conditional key readiness, and audits. |
| Metadata and session | Closed five-member keyless document, uniform JWK `404`, unchanged confidential document, PKCE exchange and refresh without credentials, encrypted session, and fail-closed provider rejection. |
| Handlers and adapters | Request presence, all four response modes, memory/PostgreSQL round trips, invalid stored values, constraint violations, and exact response shapes. |
| Migrations and startup | Backfill legacy modes, rollback and replay, mismatch failure, and an empty key set for CIMD-public-only brokers in proxy, local, and hybrid modes. |
| E2E | One block per US6-S1–S6, US7-S1–S6, and US8-S1–S8. Include no-key startup, all server modes, and session storage on exchange and refresh. |

The `cimd-public-client` label selects exactly 20 new E2E scenarios. `cimd-upstream-client && !performance` selects 51 functional scenarios (31 existing and 20 new). After implementation, run `just check`, `just test`, `just test-integration`, `just test-integration-infra`, both Ginkgo label filters, `just docs-build`, and `just verify`. Use [quickstart.md](./quickstart.md) for a smoke journey with a keyless broker, a provider that requires `none`, document and JWK responses, connect, refresh, and stored session.

### Constitution Check — post-design

| Principle | Design result and implementation gate |
|---|---|
| I | The planned `none` path is explicit, uses PKCE S256, sends no credential, and fails closed without method fallback. |
| II | ADR 043 documents D-A–D-J and the rolling-upgrade limitation. The stakeholder accepted it on 2026-10-22. |
| IV | Both contract variants, examples, errors, and versions are designed. The stakeholder confirmed D-A–D-D on 2026-10-22. |
| V | The data model defines independent registration and both public-service terms. `ARCHITECTURE.md` glossary changes remain a Stage 2 precondition. |
| VI | Domain validation, narrow metadata ports, HTTP parsing, and both storage adapters preserve the existing boundary. |
| VIII | Layered tests and semantic-red acceptance tests precede their production changes in Stage 2. |
| IX | Migrations 042/043 separate the fast schema lock from validation. Guarded rollback and real PostgreSQL tests cover persistence. |
| X | Issue #264 confirms API-013. The stakeholder confirmed D-A–D-D (API-014) in writing before Stage 2. |
| XII | Builder owns startup identity checks and conditional key creation. Existing routes and handlers remain wired through the Builder. |
| XIII | Exactly 20 new `It()` blocks cover the 20 new acceptance scenarios, with labels and spec references. |
| XIV | Six spec journeys cover distinct flows with request/response examples, configuration, and synthetic decoded JWT claims. |

**Complexity tracking**: None. No constitution exception or new architectural component is required.
