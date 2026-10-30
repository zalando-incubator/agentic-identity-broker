# Implementation Plan: OAuth2 Provider Flavor Support

**Branch**: `20260312-120000-oauth2-provider-flavors` | **Date**: 2026-03-12 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/20260312-120000-oauth2-provider-flavors/spec.md`

## Summary

Extend the `ThirdpartyOAuth2ProviderEntity` and its admin API with an `oauth2_flavor` field to support non-standard OAuth2 credential formats — specifically Google service account JSON keys. For `standard` flavor, the existing plain-string `client_secret` behavior is preserved unchanged. For `google` flavor, the `client_secret` field carries a Google service account JSON document; credential parsing and validation delegates to `golang.org/x/oauth2/google.JWTConfigFromJSON` (already in `go.mod`); `client_id` is extracted automatically from the JSON's `client_id` field.

## Technical Context

**Language/Version**: Go 1.25.6 (existing constraint)
**Primary Dependencies**: `golang.org/x/oauth2 v0.35.0` (already in `go.mod`) — `google.JWTConfigFromJSON` for service account JSON validation; chi v5 for HTTP; sqlx/pgx v5 for PostgreSQL
**Storage**: PostgreSQL (prod), in-memory (dev/test)
**Testing**: go test + testify (unit), Ginkgo v2/Gomega (E2E), testcontainers (integration)
**Target Platform**: Linux server
**Project Type**: Single Go backend (hexagonal architecture)
**Performance Goals**: No additional I/O; JSON parsing is a one-time cost at request validation time
**Constraints**: No custom crypto; library-first for all parsing; fail closed on validation errors; 32 KB credential size cap
**Scale/Scope**: Admin-only configuration change; zero impact on end-user consent flow

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**Design Preconditions (BLOCKING)**:

- [x] **Domain Model**: `OAuth2Flavor` enum value object, `GoogleServiceAccountKey` value object, `ThirdpartyOAuth2ProviderEntity.Flavor` field — all identified
- [x] **Domain Concepts**: `OAuth2Flavor`, `ClientCredential`, `GoogleServiceAccountKey` will be added to `ARCHITECTURE.md` Glossary (per spec.md §Domain Model)
- [x] **Configuration Design**: No new runtime config — flavor is per-entity data; no config port changes required
- [x] **Config Examples**: N/A (no config changes)
- [x] **API Design First**: Admin OpenAPI updated in `api/admin/openapi.yaml` before implementation; `oauth2_flavor` enum field added to `ThirdPartyOAuth2Service` schema; API-007 confirmed in spec
- [x] **API Documentation**: Changes go to `/api/admin/openapi.yaml`
- [x] **API Changes**: Confirmed in spec.md (API-001 through API-007); user confirmation embedded in spec clarifications
- [x] **Database Design**: Migration `007_add_oauth2_flavor.{up,down}.sql` — ADD COLUMN `oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard'`
- [x] **E2E Acceptance Tests**: Will be written for all 24 spec scenarios in `tests/e2e/oauth2_provider_flavor_test.go` BEFORE implementation
- [x] **E2E Test Mapping**: Each acceptance scenario maps 1:1 to one `It()` block
- [x] **E2E Red Phase**: Tests compile (empty stubs) and fail semantically before implementation

**Implementation Considerations**:

- [x] **Security-First**: Credential validation fails closed; 32 KB limit enforces SR-004; private key never appears in logs/errors (SR-002/SR-003)
- [x] **Architecture Docs**: `ARCHITECTURE.md` glossary updated with 3 new domain terms
- [x] **ADRs**: No ADR needed — this is an additive change following established patterns
- [x] **Library-First Security**: `golang.org/x/oauth2/google.JWTConfigFromJSON` for all JSON parsing and type validation; no custom JWT or crypto code
- [x] **Zalando Guidelines**: `oauth2_flavor` is a string enum with documented values; `detail` field in 400 responses
- [x] **End-User Docs**: N/A (admin-only API)
- [x] **Migration Testing**: Both `up` and `down` verified in PostgreSQL integration tests
- [x] **Hexagonal Architecture**: `OAuth2Flavor` and `GoogleServiceAccountKey` in domain/model; no infrastructure imports
- [x] **Persistence Patterns**: Follows `specs/20251215-120100-persistence-layer/quickstart.md`

## Project Structure

### Documentation (this feature)

```text
specs/20260312-120000-oauth2-provider-flavors/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
└── tasks.md             # Phase 2 output (/speckit.tasks command)
```

### Source Code Changes

```text
internal/domain/model/
  oauth2_flavor.go                 # NEW: OAuth2Flavor type + constants + Validate()
  google_service_account.go        # NEW: GoogleServiceAccountKey value object
  google_service_account_test.go   # NEW: TDD unit tests (red first)
  thirdparty_oauth2_provider.go    # EXTEND: Add Flavor field; update ValidateForCreate/Update
  thirdparty_oauth2_provider_test.go  # EXTEND: New test cases for flavor logic

internal/adapters/storage/postgres/
  thirdparty_provider_record.go    # EXTEND: Add oauth2_flavor field
  thirdparty_provider.go           # EXTEND: Include oauth2_flavor in all SQL queries

internal/adapters/storage/memory/
  thirdparty_provider.go           # EXTEND: Persist Flavor field through Create/Get/List/Update

internal/adapters/http/handlers/admin/
  services_handler.go              # EXTEND: ServiceRequest.OAuth2Flavor, flavor-aware creation

migrations/
  007_add_oauth2_flavor.up.sql     # NEW: ADD COLUMN oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard'
  007_add_oauth2_flavor.down.sql   # NEW: DROP COLUMN oauth2_flavor

api/admin/openapi.yaml             # EXTEND: oauth2_flavor enum in ThirdPartyOAuth2Service schema

tests/e2e/
  oauth2_provider_flavor_test.go   # NEW: 24 E2E acceptance tests (red phase before implementation)

tests/integration/migrations/
  migrations_test.go               # EXTEND: Verify migration 007 up/down
```

## Testing Strategy

### End-to-End (E2E) Acceptance Tests

**Test Location**: `tests/e2e/oauth2_provider_flavor_test.go`

**Framework**: Ginkgo/Gomega BDD following patterns in `tests/e2e/README.md`

**Scenario Mapping**:

| Spec Scenario | Test Description |
|---|---|
| US1.S1 | should assign default `standard` flavor when `oauth2_flavor` is omitted |
| US1.S2 | should accept explicit `standard` flavor and validate credential as plain string |
| US1.S3 | should accept `google` flavor and validate credential as service account JSON |
| US1.S4 | should reject unrecognized flavor values with HTTP 400 listing valid values |
| US1.S5 | should re-validate credential against new flavor when flavor is updated |
| US2.S1 | should store service account JSON encrypted and extract `client_id` from JSON |
| US2.S2 | should return service config with `client_id` populated and credential redacted |
| US2.S3 | should reject google credential with missing `private_key` field with HTTP 400 |
| US2.S4 | should reject google credential with `type` != `service_account` with HTTP 400 |
| US2.S5 | should return `oauth2_flavor: google` and redacted credential in GET response |
| US2.S6 | should reject `issuer_uri` whose host differs from `token_uri` in JSON with HTTP 400 |
| US2.S7 | should use `token_uri` from JSON as token endpoint when `issuer_uri` is omitted |
| US3.S1 | should accept non-empty string credential for `standard` flavor |
| US3.S2 | should reject empty/whitespace-only credential for `standard` flavor with HTTP 400 |
| US3.S3 | should accept valid google service account JSON for `google` flavor |
| US3.S4 | should reject non-JSON credential for `google` flavor with HTTP 400 |
| US3.S5 | should reject google JSON missing required fields with HTTP 400 identifying each missing field |
| US3.S6 | should reject plain string (not JSON) for `google` flavor with HTTP 400 |
| US4.S1 | should include `oauth2_flavor` in list response for all services |
| US4.S2 | should include `oauth2_flavor` in single service GET response |
| EC1 | should reject update from `google` to `standard` without providing a plain-text credential |
| EC2 | should reject google service account JSON exceeding 32 KB with HTTP 400 |
| EC3 | should reject google JSON where `client_id` field is missing with HTTP 400 |
| EC4 | should reject google JSON where `private_key` is present but empty with HTTP 400 |

**Bootstrap Strategy**:
- Uses `bootstrap.NewAdminTestServer()` for admin API operations (creates/updates/gets services)
- Fresh storage per test via `BeforeEach`/`AfterEach`
- No mock upstream needed — this is pure admin CRUD, no OAuth2 flows

**Test Data Strategy**:
- Google service account JSON fixture in `tests/e2e/fixtures/` — a realistic (but non-functional) service account JSON
- Standard service fixture from existing `fixtures.ServiceWithID()`
- Fixtures updated: `fixtures/services.go` extended with `GoogleServiceAccountFixture()` and `ValidGoogleServiceRequest()`

### Unit Tests

**Location**: `internal/domain/model/google_service_account_test.go`

Key test cases (TDD, written first):
- Parse valid Google service account JSON → returns populated struct
- Parse JSON with wrong `type` → returns error mentioning `service_account`
- Parse invalid JSON → returns error
- Parse JSON missing `private_key` → validation error naming the field
- Parse JSON missing `client_email` → validation error naming the field
- Parse JSON missing `token_uri` → validation error naming the field
- Parse JSON missing `client_id` → validation error naming the field
- Parse JSON with empty `private_key` → validation error (non-empty required)
- JSON exceeding 32 KB → size limit error
- `issuer_uri` host matches `token_uri` host → valid
- `issuer_uri` host differs from `token_uri` host → validation error

**Location**: `internal/domain/model/thirdparty_oauth2_provider_test.go` (extended)

Additional test cases for flavor-aware validation:
- `ValidateForCreate` with `Flavor: google` + valid service account JSON credential
- `ValidateForCreate` with `Flavor: google` + invalid JSON → error
- `ValidateForCreate` with `Flavor: standard` + empty string → error
- `ValidateForCreate` with unknown `Flavor` → error

### Integration Tests

**Location**: `tests/integration/migrations/migrations_test.go` (extended)

- Verify migration 007 applies cleanly on schema with migrations 001-006 applied
- Verify existing rows get `oauth2_flavor = 'standard'` after migration
- Verify migration 007 rolls back cleanly (DROP COLUMN)
- Verify re-apply after rollback

## Complexity Tracking

No constitution violations. All implementation follows established patterns.

---

## Phase 0 — Research

> See [research.md](./research.md) for full findings. Summary inline below.

### Key Decisions

**Decision 1**: Use `golang.org/x/oauth2/google.JWTConfigFromJSON` for Google service account validation.

`golang.org/x/oauth2` is already in `go.mod` at v0.35.0. The `google.JWTConfigFromJSON(jsonKey []byte)` function:
- Parses JSON, returns error for invalid JSON → satisfies FR-008
- Validates `type == "service_account"`, returns error otherwise → satisfies FR-007
- Extracts `client_email` → `jwt.Config.Email`, `private_key` → `jwt.Config.PrivateKey`, `token_uri` → `jwt.Config.TokenURL`
- Does **not** validate private key cryptographic format (stores raw bytes) → satisfies FR-014 (must NOT validate crypto format)
- Does **not** expose `client_id` (internal `credentialsFile.ClientID` is unexported for service accounts)

For `client_id` extraction: unmarshal a minimal auxiliary struct `{ ClientID string \`json:"client_id"\` }` after the primary parse. Simple, no custom crypto.

**Decision 2**: `OAuth2Flavor` as a string type with constants in `internal/domain/model/`.

Domain-layer type, zero infrastructure imports. Follows `RequirementType` pattern already in `model/`.

**Decision 3**: Validation location.

`GoogleServiceAccountKey` parsing lives in `internal/domain/model/google_service_account.go`. Using `golang.org/x/oauth2/google` in domain is acceptable per domain AGENTS.md ("declared external libraries, e.g., `jwx`, `oauth2`, `cel-go`").

**Decision 4**: Google flavor endpoint derivation.

- `token_endpoint` on the entity is populated from `token_uri` in the JSON
- `authorize_endpoint` defaults to `google.Endpoint.AuthURL` (`https://accounts.google.com/o/oauth2/auth`) since the service account flow does not use an authorization endpoint but the field must be non-nil for storage consistency
- `issuer_uri` on the entity is set to the value provided in the request (optional); if omitted, left as `token_uri` host base
- The standard HTTPS validation for endpoints is bypassed for google flavor since values are derived from a validated Google credential

**Decision 5**: `ValidateForCreate`/`ValidateForUpdate` changes for Google flavor.

For `Flavor == google`:
- Skip `client_id` required check (derived from JSON)
- `issuer_uri` is optional (if empty, use `token_uri` base URL from JSON)
- If `issuer_uri` is provided, validate `scheme + host` matches `scheme + host` of `token_uri` from JSON
- Skip manual endpoint required checks (`token_endpoint` and `authorize_endpoint` are derived)
- Apply 32 KB size cap on credential bytes

**Decision 6**: Handler changes for `client_id` derivation.

`ServicesHandler.CreateService` and `UpdateService`: when `Flavor == google`, parse the credential as `GoogleServiceAccountKey`, set `entity.ClientID` from `gsk.ClientID` (overriding any request-provided `client_id`).

**Decision 7**: `IssuerURI` handling for google flavor.

Per clarifications: `issuer_uri` optional for google. If omitted, `token_uri` from JSON is authoritative for token acquisition. For storage consistency, `entity.IssuerURI` is set to the base URL of `token_uri` (scheme + host) when `issuer_uri` is not provided by the caller.

---

## Phase 1 — Design Artifacts

> See [data-model.md](./data-model.md), [contracts/](./contracts/), and [quickstart.md](./quickstart.md) for complete design.

### Data Model Summary

**New value objects**:

```go
// internal/domain/model/oauth2_flavor.go
type OAuth2Flavor string

const (
    OAuth2FlavorStandard OAuth2Flavor = "standard"
    OAuth2FlavorGoogle   OAuth2Flavor = "google"
)

func (f OAuth2Flavor) Validate() error // returns error for unrecognized values

// internal/domain/model/google_service_account.go
type GoogleServiceAccountKey struct {
    Type        string  // always "service_account" (validated)
    ClientEmail string  // from "client_email", non-empty
    PrivateKey  []byte  // from "private_key", non-empty (raw bytes, no crypto validation)
    TokenURI    string  // from "token_uri", non-empty
    ClientID    string  // from "client_id", non-empty
}

func ParseGoogleServiceAccountKey(credential string) (*GoogleServiceAccountKey, error)
// Validates size (≤32KB), parses via google.JWTConfigFromJSON + auxiliary client_id unmarshal,
// validates non-emptiness of all required fields.
```

**Extended entity**:

```go
// internal/domain/model/thirdparty_oauth2_provider.go (extension)
type ThirdpartyOAuth2ProviderEntity struct {
    // ... existing fields ...
    Flavor OAuth2Flavor  // NEW: defaults to OAuth2FlavorStandard
}
```

**Updated validation**:
- `ValidateForCreate(skipHTTPS bool)`: flavor-dispatch for credential validation
- `ValidateForUpdate(skipHTTPS bool)`: same dispatch
- New helper `validateFlavorCredential(flavor, credential, issuerURI)` encapsulates flavor-specific rules

### API Changes

`POST /api/services` and `PUT /api/services/{service-id}` request body gains `oauth2_flavor` (string enum, optional, default `standard`).

All response bodies (list, create, get, update) include `oauth2_flavor`.

`client_id` in the request is **optional** when `oauth2_flavor == "google"` (extracted from credential JSON).

For `google` flavor, `endpoints` block and `issuer_uri` in the request body are **optional** (derived from credential).

### Database Schema

Migration `007` adds:

```sql
ALTER TABLE thirdparty_oauth2_services
    ADD COLUMN oauth2_flavor VARCHAR(50) NOT NULL DEFAULT 'standard';

COMMENT ON COLUMN thirdparty_oauth2_services.oauth2_flavor
    IS 'OAuth2 authentication variant: standard (plain client_secret) or google (service account JSON)';
```

Rollback removes the column.
