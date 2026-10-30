# Implementation Plan: Provider Authorization Parameters

**Branch**: `20260720-120000-provider-auth-params` | **Date**: 2026-07-20 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/20260720-120000-provider-auth-params/spec.md`

## Summary

Add an optional, static `authorization_params` string map to each third-party OAuth2 service. Administrators manage and read it through the existing service API; the broker persists it in the existing service aggregate and storage backends, validates blank and broker-owned names, and supplies only stored values to both upstream authorization and authorization-code exchange requests. Browser-supplied provider parameters are ignored.

## Technical Context

**Language/Version**: Go 1.25.6  
**Primary Dependencies**: chi v5; `golang.org/x/oauth2`; sqlx with pgx v5; Ginkgo/Gomega; testify  
**Storage**: In-memory maps and PostgreSQL; new JSONB column on `thirdparty_oauth2_services`  
**Testing**: Go `testing` + testify for unit/handler/storage; Ginkgo/Gomega for end-to-end; testcontainers PostgreSQL integration  
**Target Platform**: Linux containerized broker with separate end-user and admin HTTP servers  
**Project Type**: Backend web service; no frontend change  
**Performance Goals**: Preserve normal authorization-start performance; the small in-memory configuration map adds no network round trip  
**Constraints**: Values come only from stored admin configuration; no values in logs; broker-owned OAuth2 authorization and token fields remain authoritative; update omission preserves existing map; authorization-code exchange and refresh-token requests both receive these provider parameters.
**Scale/Scope**: One optional map per existing third-party service; no new service type, port, UI, runtime setting, or dependency

## Constitution Check

### Design Preconditions

- [x] **Domain Model**: Extend the existing `ThirdpartyOAuth2ProviderEntity`; [data-model.md](data-model.md) defines ownership, invariants, and update semantics.
- [x] **Domain Concepts**: Add Provider Authorization Parameters to the `ARCHITECTURE.md` glossary in implementation.
- [x] **Entity IDs**: No new entity or UUID primary key; ADR 20260313-120000 changes are not applicable.
- [x] **Configuration Design**: This is persisted administrator-managed service configuration, not runtime configuration; no unified config, YAML example, or Helm value is required.
- [x] **API Design First**: The requested field and behavior are designed in [contracts/admin-service-authorization-params.md](contracts/admin-service-authorization-params.md).
- [x] **API Documentation**: Update `api/admin/openapi.yaml` for service create, update, and response schemas.
- [x] **API Changes**: Explicitly requested and approved by the user in this feature specification.
- [x] **Database Design**: Add reversible go-migrate up/down migration for a JSONB service column.
- [x] **E2E Acceptance Tests**: Add backend E2E tests for all eight spec scenarios before feature implementation.
- [x] **E2E Test Mapping**: The mapping below assigns one `It` block to each acceptance scenario.
- [x] **E2E Red Phase**: Tests will compile and fail semantically before implementation.
- [x] **Frontend Playwright E2E**: Not applicable; no React UI changes.
- [x] **Frontend Screenshots**: Not applicable; no React UI changes.

### Implementation Considerations

- [x] **Security-First**: Reserved names and blank entries fail closed; end-user query parameters are not forwarded; configuration values are excluded from logs.
- [x] **Architecture Docs**: Update the glossary only; component boundaries and DI remain unchanged.
- [x] **ADRs**: No new major architectural decision. Existing ADR 20251215-120300 governs storage and ADR 20260313-120000 is unchanged.
- [x] **Library-First Security**: No cryptography or new security library is introduced.
- [x] **Zalando Guidelines**: Existing resource shapes and validation-error conventions are retained.
- [x] **End-User Docs**: Not applicable; the end-user API contract is unchanged.
- [x] **Migration Testing**: Verify migration up/down and PostgreSQL persistence with the existing shared-container test pattern.
- [x] **Hexagonal Architecture**: Entity validation remains in domain; adapters map transport and storage; session-service behavior uses persisted entity data.
- [x] **Persistence Patterns**: Existing provider repository and both adapters are extended consistently.

**Post-design re-check**: Passed. No unresolved clarifications or constitution-gate exceptions remain.

## Project Structure

### Documentation

```text
specs/20260720-120000-provider-auth-params/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   └── admin-service-authorization-params.md
└── tasks.md                         # generated later
```

### Source and Test Changes

```text
api/admin/openapi.yaml
ARCHITECTURE.md
docs/guides/manage-agents-and-services.md
migrations/022_add_service_authorization_params.{up,down}.sql
internal/
├── domain/model/thirdparty_oauth2_provider.go
├── domain/model/thirdparty_oauth2_provider_test.go
├── domain/oauth2session/service.go
├── domain/oauth2session/service_test.go
├── adapters/http/handlers/admin/services_handler.go
├── adapters/http/handlers/admin/services_handler_test.go
├── adapters/storage/memory/thirdparty_provider_test.go
├── adapters/storage/postgres/thirdparty_provider.go
├── adapters/storage/postgres/thirdparty_provider_record.go
└── adapters/storage/postgres/thirdparty_provider_record_test.go
tests/
├── e2e/provider_authorization_params_test.go
└── integration/storage/infra/thirdparty_service_test.go
```

**Structure Decision**: Extend the existing provider aggregate and its admin, session, and storage paths. No new package, port, repository, handler, route, dependency, or frontend surface is needed.

## Implementation Phase Overview

- [x] **Phase 0 (refactoring)**: Skip. The existing entity, handler, storage record, and session service are direct extension points.
- [x] **Phase 2.7 (entity boilerplate)**: Skip. This is a field on an existing aggregate, not a new entity or CRUD surface.
- **Phase 2**: Update OpenAPI, glossary, migration, E2E red tests, and unit/storage red tests.
- **Phase 3**: Add domain validation and copying, admin mapping/update preservation, storage mapping, and server-side upstream URL merging.
- **Final phase**: Run focused suites, then `just check` and the smallest relevant complete test/integration gate.

## Implementation Design

1. **Contract and model**: Add `authorization_params` to the existing admin service request and response shapes, OpenAPI schemas, entity, entity copy/redaction behavior, PostgreSQL record mapping, and migration.
2. **Validation and update semantics**: Add one pure entity validator called by both create and update validation. Reject blank/whitespace values and case-insensitive reserved names. Use a nil map to represent omission; on update, retrieve/preserve existing configuration only when omitted, while an empty object clears it.
3. **Storage**: Deep-copy the map for memory storage and serialize it as JSONB in PostgreSQL queries and adapter-local records. Preserve old-row behavior with an empty JSONB default.
4. **Upstream requests**: Build the existing broker OAuth2 authorization URL first, then add the validated persisted map server-side via the URL query. Pass the same map as authorization-code exchange options to `oauth2.Config.Exchange`, and add it to the refresh-token form. Do not read unknown end-user query parameters or log values. The target Zalando Platform IdP requires `business_partner_id` on its refresh-token grant.
5. **Documentation**: Document the field in the admin contract/OpenAPI and service-management guide, including the Zalando `business_partner_id` example and the no-browser-forwarding boundary.

## Testing Strategy

### End-to-End Acceptance Tests

**Test Location**: `tests/e2e/provider_authorization_params_test.go`

**Framework**: Ginkgo/Gomega with `bootstrap.NewAdminTestServer()` and `bootstrap.NewEndUserTestServer()` over production `app.Builder` wiring.

**Test data and helpers**:

- Reuse the service request and admin CRUD approach in `tests/e2e/oauth2_provider_flavor_test.go`.
- Use a mock upstream authorization and token endpoint that captures the redirect query and form-encoded code-exchange body.
- Create fresh in-memory storage and both production servers in each `BeforeEach`.
- No new frontend, page object, screenshot, or generic helper is needed unless the existing mock upstream cannot capture the token request cleanly.

| Spec Scenario | E2E Test | Expected externally visible behavior |
|---|---|---|
| User Story 1, Scenario 1 | `It` create, get, and list service configuration | Create/get/list return `business_partner_id: "12345"`. |
| User Story 1, Scenario 2 | `It` update without map | Omitted field preserves the stored map. |
| User Story 1, Scenario 3 | `It` omit or empty map | No additional configuration is returned or retained as appropriate. |
| User Story 2, Scenario 1 | `It` complete configured authorization | Upstream redirect and code-exchange form each include the stored value and normal broker fields. |
| User Story 2, Scenario 2 | `It` complete unconfigured authorization | Neither upstream request contains a provider-specific value. |
| User Story 2, Scenario 3 | `It` start with conflicting browser query | Upstream redirect and code exchange retain stored value and exclude `untrusted`. |
| User Story 3, Scenario 1 | `It` reject blank key or value | Admin request receives validation failure and persists no change. |
| User Story 3, Scenario 2 | `It` reject reserved name | Admin request receives validation failure and persists no change. |

Each `It` must carry its exact `// Scenario X.Y from specs/20260720-120000-provider-auth-params/spec.md` reference, compile, and fail semantically before behavior is implemented.

### Unit and Integration Coverage

| Layer | Location | Coverage |
|---|---|---|
| Domain model | `internal/domain/model/thirdparty_oauth2_provider_test.go` | Table-driven blank and reserved-name validation plus map copy isolation. |
| Admin handler | `internal/adapters/http/handlers/admin/services_handler_test.go` | Create/response map mapping, get/list mapping, update omission preservation, and explicit empty-map clearing. |
| OAuth2 session service | `internal/domain/oauth2session/service_test.go` | Stored `business_partner_id` appears in the upstream URL, authorization-code exchange form, and refresh-token form; empty maps add nothing. |
| Memory storage | `internal/adapters/storage/memory/thirdparty_provider_test.go` | Create/get/update/list retain values and do not alias map mutations. |
| PostgreSQL record | `internal/adapters/storage/postgres/thirdparty_provider_record_test.go` | JSONB record/entity conversions preserve map values. |
| PostgreSQL integration | `tests/integration/storage/infra/thirdparty_service_test.go` | Real migration-backed create/get/update/list persistence using the shared PostgreSQL bootstrap. |
| Migration | existing migration integration framework | New column applies, defaults existing rows to empty map, and rolls back cleanly. |

### Validation Commands

```bash
go test ./internal/domain/model ./internal/domain/oauth2session ./internal/adapters/http/handlers/admin ./internal/adapters/storage/memory ./internal/adapters/storage/postgres
just test-e2e-backend
just test-integration
just test-integration-infra
just check
```

Run focused test packages during TDD. Before completion, run `just check`, `just test-e2e-backend`, and the relevant PostgreSQL integration suite; use `just verify` if time and infrastructure are available.

## Complexity Tracking

No constitution violations or additional complexity justification required.
