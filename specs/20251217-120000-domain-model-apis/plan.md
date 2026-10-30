# Implementation Plan: Domain Model and Consent APIs

**Branch**: `20251217-120000-domain-model-apis` | **Date**: 2025-12-17 | **Spec**: [spec.md](spec.md)
**Feature Branch**: `20251217-120000-domain-model-apis`

## Summary

This feature implements the core domain model for the identity broker with three new entities (Agent, ThirdpartyOAuth2Service, UserGrant) and introduces a consent service in the domain layer. The implementation provides full CRUD APIs for admin management and user consent APIs for granting/revoking permissions to agents.

**Key Components**:
- **Domain Entities**: Agent, ThirdpartyOAuth2Service, UserGrant with validation
- **Consent Service**: Core business logic for granting/retrieving consent, determining available third-party services
- **Admin APIs**: Full CRUD for agents and OAuth2 service configurations
- **User Consent APIs**: View agent info, create/view/modify/revoke grants with user-selectable expiration
- **Repository Interfaces**: Three focused repositories following Interface Segregation Principle
- **Dual Adapters**: In-memory (development) and PostgreSQL (production) implementations

**Technical Approach**: Following hexagonal architecture established in 20251215-120100-persistence-layer with small focused interfaces, sqlx for PostgreSQL, comprehensive error wrapping, and testcontainers integration tests.

## Technical Context

**Language/Version**: Go 1.24.0 (configured in go.mod)
**Primary Dependencies**:
- chi v5.2.3 (HTTP router)
- sqlx v1.3.5 (PostgreSQL queries)
- pgx v5 (PostgreSQL driver)
- viper v1.21.0 (configuration)
- slog (structured logging)
- testcontainers-go (integration tests)

**Storage**: PostgreSQL 12+ (production), In-memory (development/testing)
**Testing**: Go standard library testing + testify for assertions + testcontainers
**Target Platform**: Linux server (backend service)
**Project Type**: Backend microservice with dual HTTP ports (8000 enduser, 14000 admin)
**Performance Goals**:
- GET /api/consent/agent/:id response < 2 seconds (SC-003)
- 100 concurrent grant creation requests without errors (SC-009)
- OAuth2 discovery 95% success rate (SC-006)

**Constraints**:
- Client secrets encrypted at rest (EncryptionPort interface with no-op adapter for now)
- Grant expiration enforcement within 1 minute (SC-005)
- One grant per user-agent pair (upsert semantics)
- Service deletion blocked if grants exist

**Scale/Scope**:
- 3 new domain entities
- 3 repository interfaces (15 methods total)
- 1 consent service interface (4 methods)
- 10 HTTP endpoints (6 admin, 4 user)
- 6 database tables (3 entities + junction/audit tables)
- 28-37 hours estimated implementation time

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-checked after Phase 1 design.*

✅ **Security-First**:
- Client secrets encrypted at rest via EncryptionPort interface (SR-002) ✅
- Principal-based authorization (SR-004, SR-006) ✅
- URL validation prevents injection (SR-011) ✅
- Rate limiting on all APIs (SR-012) ✅
- Audit logging for all operations (SR-007, SR-008) ✅
- No security bypasses or optional controls ✅

✅ **Architecture Docs**:
- Will update ARCHITECTURE.md with new domain entities and consent subsystem ✅
- Glossary will include Agent, ThirdpartyOAuth2Service, UserGrant, OAuth Scope ✅

✅ **ADRs**:
- Follows ADR 20251215-120300 (Storage Layer Architecture) - sqlx, interface segregation, error wrapping ✅
- No new ADR needed - consistent with established patterns ✅

✅ **Library-First Security**:
- Encryption via port interface (future AES-256-GCM adapter) ✅
- Uses pgx v5 for database security (prepared statements, connection security) ✅
- No custom cryptography ✅

✅ **API Documentation**:
- docs/ will be updated with new API endpoints ✅
- OpenAPI 3.0 spec generated in contracts/openapi.yaml ✅

✅ **Domain Model**:
- New domain concepts documented in data-model.md ✅
- Will add to ARCHITECTURE.md Glossary: Agent, ThirdpartyOAuth2Service, UserGrant, OAuth Scope ✅

✅ **Hexagonal Architecture**:
- Domain logic in internal/domain/consent/ (consent service) ✅
- Ports defined in internal/ports/storage.go (repository interfaces) ✅
- Adapters in internal/adapters/storage/{memory,postgres}/ ✅
- HTTP handlers in internal/adapters/http/ (driving adapters) ✅
- Clear port/adapter separation maintained ✅

✅ **Configuration-Driven**:
- Uses existing system configuration port ✅
- No custom configuration loading ✅

✅ **TDD & Automated Testing**:
- Table-driven unit tests for validation ✅
- Testcontainers integration tests for PostgreSQL ✅
- No Bash scripts for correctness validation ✅

✅ **Persistence Pattern Consistency**:
- Follows specs/20251215-120100-persistence-layer/quickstart.md patterns ✅
- Small focused interfaces per entity (ISP) ✅
- Separate StorageLifecycle interface ✅
- Repository accessors on Adapter struct ✅
- sqlx for PostgreSQL (not GORM) ✅
- Error wrapping in domain.StorageError ✅
- Testcontainers integration tests ✅

**Status**: All checks pass ✅ - Ready to proceed

## Project Structure

### Documentation (this feature)

```text
specs/20251217-120000-domain-model-apis/
├── spec.md                          # Feature specification (completed)
├── plan.md                          # This file (implementation plan)
├── research.md                      # Phase 0: Technology decisions
├── data-model.md                    # Phase 1: Entity definitions
├── quickstart.md                    # Phase 1: Implementation guide
├── implementation-plan.md           # Phase 1: Detailed Go implementation plan
├── contracts/                       # Phase 1: API contracts
│   ├── openapi.yaml                 # OpenAPI 3.0 specification
│   ├── API_DESIGN.md                # API design principles
│   ├── ARCHITECTURE_DIAGRAM.md      # System diagrams
│   ├── QUICK_REFERENCE.md           # cURL examples and checklists
│   ├── README.md                    # Documentation overview
│   └── VALIDATION_CHECKLIST.md      # Requirements traceability
└── checklists/
    └── requirements.md              # Specification quality checklist
```

### Source Code (repository root)

```text
internal/
├── domain/
│   ├── consent/                     # NEW: Consent service (business logic)
│   │   ├── service.go               # ConsentService interface and implementation
│   │   └── service_test.go          # Unit tests for consent logic
│   └── storage/                     # EXTEND: Add new entities
│       ├── agent.go                 # Agent entity (validation, methods)
│       ├── thirdparty_service.go    # ThirdpartyOAuth2Service entity
│       ├── user_grant.go            # UserGrant entity
│       ├── oauth_scope.go           # OAuth Scope value object
│       └── errors.go                # Domain errors (extend existing)
│
├── ports/
│   ├── storage.go                   # EXTEND: Add 3 repository interfaces
│   │   # - AgentRepository (5 methods)
│   │   # - ThirdpartyOAuth2ServiceRepository (6 methods)
│   │   # - UserGrantRepository (7 methods)
│   └── encryption.go                # NEW: EncryptionPort interface
│
├── adapters/
│   ├── storage/
│   │   ├── memory/                  # EXTEND: In-memory implementations
│   │   │   ├── adapter.go           # Add repository accessors
│   │   │   ├── agents.go            # AgentRepository implementation
│   │   │   ├── agents_test.go       # Unit tests
│   │   │   ├── thirdparty_services.go
│   │   │   ├── thirdparty_services_test.go
│   │   │   ├── user_grants.go
│   │   │   └── user_grants_test.go
│   │   │
│   │   ├── postgres/                # EXTEND: PostgreSQL implementations
│   │   │   ├── adapter.go           # Add repository accessors
│   │   │   ├── agents.go            # AgentRepository implementation
│   │   │   ├── agents_test.go       # Integration tests (testcontainers)
│   │   │   ├── thirdparty_services.go
│   │   │   ├── thirdparty_services_test.go
│   │   │   ├── user_grants.go
│   │   │   └── user_grants_test.go
│   │   │
│   │   └── noop/                    # NEW: No-op encryption adapter
│   │       ├── encryption.go        # NoOpEncryption implementation
│   │       └── encryption_test.go   # Tests
│   │
│   └── http/                        # EXTEND: HTTP handlers
│       ├── admin/                   # NEW: Admin API handlers (port 14000)
│       │   ├── agents_handler.go    # CRUD for agents
│       │   ├── agents_handler_test.go
│       │   ├── services_handler.go  # CRUD for OAuth2 services
│       │   └── services_handler_test.go
│       │
│       └── consent/                 # NEW: User consent API handlers (port 8000)
│           ├── agent_info_handler.go    # GET /api/consent/agent/:id
│           ├── agent_info_handler_test.go
│           ├── grants_handler.go        # GET/POST /api/consent/agent/:id/grants
│           └── grants_handler_test.go
│
migrations/                          # NEW: Database migrations
├── 001_create_agents.up.sql         # Create agents table
├── 001_create_agents.down.sql
├── 002_create_thirdparty_services.up.sql
├── 002_create_thirdparty_services.down.sql
├── 003_create_user_grants.up.sql
└── 003_create_user_grants.down.sql

docs/                                # EXTEND: API documentation
├── api/
│   ├── admin-apis.md                # NEW: Admin API documentation
│   └── consent-apis.md              # NEW: User consent API documentation
└── configuration.md                 # EXTEND: Add domain model config section
```

**Structure Decision**: Backend microservice following hexagonal architecture with clear separation between domain logic (consent service), ports (repository interfaces), and adapters (storage implementations, HTTP handlers). The consent service lives in the domain layer per user requirement, containing core business logic for granting/retrieving consent and determining available third-party services.

## Complexity Tracking

> **No violations requiring justification**

All complexity is aligned with constitution principles:
- Hexagonal architecture (Principle VI) ✅
- Persistence patterns from quickstart.md (Principle IX) ✅
- Security-first with encryption and audit logging (Principle I) ✅
- No custom cryptography (Principle III) ✅
- Comprehensive testing strategy (Principle VIII) ✅

## Phase 0: Research & Decisions

**Document**: [research.md](research.md)

**Key Research Areas**:
1. ✅ Client secret encryption strategy (AES-256-GCM)
2. ✅ OAuth2 endpoint discovery implementation (well-known + metadata_url override)
3. ✅ Grant expiration enforcement patterns (query-time filtering)
4. ✅ Service deletion blocking strategy (foreign key + explicit count check)
5. ✅ JSONB handling in sqlx (explicit marshal/unmarshal)

**Decisions Made**:
- **Encryption**: AES-256-GCM with key from configuration, stored as BYTEA in PostgreSQL
- **Discovery**: Construct `{issuer}/.well-known/oauth-authorization-server` or use metadata_url override
- **Grant Expiration**: Filter at query time with `WHERE valid_until IS NULL OR valid_until > NOW()`
- **Service Deletion**: `CountGrantsReferencingService()` before deletion, return 409 if count > 0
- **JSONB**: Use `json.Marshal` for scopes array, explicit `Scan` for JSONB columns

All research complete - no blockers identified.

## Phase 1: Design & Contracts

**Documents**:
- [data-model.md](data-model.md) - Entity definitions with validation rules
- [quickstart.md](quickstart.md) - Step-by-step implementation guide
- [implementation-plan.md](implementation-plan.md) - Detailed Go implementation plan
- [contracts/openapi.yaml](contracts/openapi.yaml) - OpenAPI 3.0 API specification

### Data Model Summary

**Three core entities**:
1. **Agent**: AI agent registry with OAuth2 client_id, display metadata, governance URLs
2. **ThirdpartyOAuth2Service**: External OAuth2 providers with client credentials, endpoints, scopes
3. **UserGrant**: User delegations to agents with optional expiration

**Value Objects**:
- **OAuth Scope**: scope_value + description (embedded in ThirdpartyOAuth2Service)
- **DelegatedOAuth2Token**: thirdparty_oauth2_service_id + scopes (embedded in UserGrant)

See [data-model.md](data-model.md) for complete entity definitions, validation rules, and relationships.

### API Contracts Summary

**10 REST endpoints** (OpenAPI 3.0 spec in contracts/openapi.yaml):

**Admin APIs (Port 14000 - existing admin server)**:
- `POST /api/agents` - Create agent
- `GET /api/agents/:agent-id` - Get agent
- `PUT /api/agents/:agent-id` - Update agent
- `DELETE /api/agents/:agent-id` - Delete agent (cascade grants)
- `POST /api/third-party/oauth2/clients` - Create OAuth2 service
- `GET /api/third-party/oauth2/clients/:client-id` - Get service (secret redacted)
- `PUT /api/third-party/oauth2/clients/:client-id` - Update service
- `DELETE /api/third-party/oauth2/clients/:client-id` - Delete service (blocked if grants exist)

**User Consent APIs (Port 8000 - existing enduser server)**:
- `GET /api/consent/agent/:agent-id` - Get agent info + all requested services
- `GET /api/consent/agent/:agent-id/grants` - Get user's grants (principal from session)
- `POST /api/consent/agent/:agent-id/grants` - Create/update grant (upsert semantics)

See contracts/API_DESIGN.md for detailed design principles, error handling, and security patterns.

### Repository Interfaces Summary

**Three focused repositories following ISP**:
1. **AgentRepository**: Create, Get, Update, Delete, List (5 methods)
2. **ThirdpartyOAuth2ServiceRepository**: Create, Get, Update, Delete, List, CountGrantsReferencingService (6 methods)
3. **UserGrantRepository**: Create, Get, Update, Delete, ListByPrincipalAndAgent, FindByPrincipalAndAgent, DeleteByAgent (7 methods)

All methods:
- Accept `context.Context` for timeout/cancellation
- Return domain errors wrapped in `StorageError`
- Follow naming conventions from quickstart.md

See [implementation-plan.md](implementation-plan.md) for complete interface definitions.

### Consent Service Summary

**Location**: `internal/domain/consent/service.go`

**Interface**:
```go
type ConsentService interface {
    GetAgentConsentInfo(ctx context.Context, agentID string) (*AgentConsentInfo, error)
    GrantConsent(ctx context.Context, req *GrantConsentRequest) (*UserGrant, error)
    RevokeConsent(ctx context.Context, principal, agentID string) error
    GetActiveGrants(ctx context.Context, principal, agentID string) ([]*UserGrant, error)
}
```

**Core Business Logic**:
- Validates agent existence before returning consent info
- Returns ALL configured third-party services as requested_services (per clarification)
- Validates scopes against service configurations
- Enforces one grant per user-agent pair (upsert)
- Filters expired grants (valid_until < now)
- Handles optional valid_until (null = indefinite)

See [implementation-plan.md](implementation-plan.md) Section 4 for detailed consent service design.

## Phase 2: Implementation Roadmap

**Total Estimated Time**: 28-37 hours

### Phase 2.1: Domain Layer (2-3 hours)
- [ ] Create entity structs (Agent, ThirdpartyOAuth2Service, UserGrant)
- [ ] Implement `Validate()` methods with domain rules
- [ ] Define EncryptionPort interface in ports/encryption.go
- [ ] Write table-driven validation tests

### Phase 2.2: Database Schema (2-3 hours)
- [ ] Write migration 001: agents table
- [ ] Write migration 002: thirdparty_oauth2_services table
- [ ] Write migration 003: user_grants table
- [ ] Add foreign key constraints and indexes
- [ ] Test migrations up/down

### Phase 2.3: Repository Interfaces (1-2 hours)
- [ ] Define AgentRepository interface in ports/storage.go
- [ ] Define ThirdpartyOAuth2ServiceRepository interface
- [ ] Define UserGrantRepository interface
- [ ] Add repository accessors to Adapter struct

### Phase 2.4: In-Memory Adapter (4-5 hours)
- [ ] Implement AgentRepository (memory/agents.go)
- [ ] Implement ThirdpartyOAuth2ServiceRepository (memory/thirdparty_services.go)
- [ ] Implement UserGrantRepository (memory/user_grants.go)
- [ ] Implement NoOpEncryption adapter (noop/encryption.go)
- [ ] Write unit tests for each repository (table-driven)
- [ ] Test cascade delete and referential integrity

### Phase 2.5: PostgreSQL Adapter (6-8 hours)
- [ ] Implement AgentRepository (postgres/agents.go)
- [ ] Implement ThirdpartyOAuth2ServiceRepository (postgres/thirdparty_services.go)
- [ ] Implement UserGrantRepository (postgres/user_grants.go)
- [ ] Handle JSONB columns (scopes, delegated_tokens)
- [ ] Integrate EncryptionPort for client secret storage
- [ ] Write integration tests with testcontainers

### Phase 2.6: Consent Service (4-5 hours)
- [ ] Implement ConsentService in domain/consent/service.go
- [ ] Implement GetAgentConsentInfo (fetch agent + all services)
- [ ] Implement GrantConsent (validate scopes, upsert)
- [ ] Implement RevokeConsent (delete grant)
- [ ] Implement GetActiveGrants (filter expired)
- [ ] Write unit tests with mock repositories

### Phase 2.7: Admin API Handlers (4-5 hours)
- [ ] Implement agents CRUD handlers (http/admin/agents_handler.go)
- [ ] Implement OAuth2 services CRUD handlers (http/admin/services_handler.go)
- [ ] Handle client secret redaction in responses
- [ ] Handle service deletion blocking (409 if grants exist)
- [ ] Wire handlers to existing admin server router (port 14000)
- [ ] Write handler unit tests

### Phase 2.8: User Consent API Handlers (3-4 hours)
- [ ] Implement GET /api/consent/agent/:id handler (returns requested_services)
- [ ] Implement GET /api/consent/agent/:id/grants handler
- [ ] Implement POST /api/consent/agent/:id/grants handler
- [ ] Extract principal from session middleware
- [ ] Wire handlers to existing enduser server router (port 8000)
- [ ] Write handler unit tests

### Phase 2.9: Integration Testing & Docs (3-4 hours)
- [ ] Write end-to-end consent workflow tests
- [ ] Write OpenAPI spec validation tests
- [ ] Update ARCHITECTURE.md with new entities
- [ ] Update docs/api/ with admin and consent APIs
- [ ] Update docs/configuration.md with domain model config
- [ ] Generate API documentation from OpenAPI spec

## Testing Strategy

### Unit Tests (Target: 90%+ coverage)
- **Domain validation**: Table-driven tests for entity `Validate()` methods
- **Repository logic**: Memory adapter tests (concurrent access, data copying)
- **Consent service**: Mock repositories, test business logic isolation
- **HTTP handlers**: Mock service layer, test request/response handling

### Integration Tests
- **PostgreSQL adapter**: Testcontainers with real PostgreSQL 15
- **End-to-end workflows**: Full consent flow from HTTP request to database

### Test Organization
```go
// Table-driven validation test pattern
func TestAgent_Validate(t *testing.T) {
    tests := []struct {
        name    string
        agent   *ports.Agent
        wantErr bool
        errMsg  string
    }{
        {name: "valid agent", agent: validAgent(), wantErr: false},
        {name: "missing client_id", agent: agentWithoutClientID(), wantErr: true, errMsg: "client_id"},
        // ... more test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.agent.Validate()
            if tt.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tt.errMsg)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

## Security Implementation

### SR-002: Client Secret Encryption
- **Interface**: `EncryptionPort` in `internal/ports/encryption.go`
- **Methods**: `Encrypt(plaintext string, encryptionContext map[string]string) ([]byte, error)` and `Decrypt(ciphertext []byte, encryptionContext map[string]string) (string, error)`
- **Current Adapter**: NoOpEncryption (stores plaintext, suitable for development)
- **Future Adapter**: AES256GCMEncryption (production-ready encryption)
- **Storage**: Encrypted bytes stored as BYTEA in PostgreSQL
- **Implementation**: `internal/adapters/storage/noop/encryption.go`

### SR-003: Client Secret Redaction
- **Pattern**: `RedactedCopy()` method on ThirdpartyOAuth2Service
- **Behavior**: Returns copy with `client_secret = "REDACTED"`
- **Usage**: All HTTP GET responses call `RedactedCopy()` before JSON encoding

### SR-004/SR-006: Principal-Based Authorization
- **Middleware**: Session middleware extracts principal from authenticated session
- **Enforcement**: All grant operations filtered by `principal` from context
- **Failure Mode**: Fail closed - reject request if principal extraction fails

### SR-007/SR-008: Structured Audit Logging
```go
slog.Info("grant_created",
    "principal", principal,
    "agent_id", grant.AgentID,
    "service_ids", serviceIDs,
    "valid_until", grant.ValidUntil,
)
```

### SR-011: URL Validation
- **Function**: `isValidURL(url string) bool`
- **Validation**: Parse URL, check scheme (http/https), reject invalid formats
- **Protection**: Prevents injection attacks via governance_url, user_documentation_url, agent_interface_url

### SR-012: Rate Limiting
- **Implementation**: chi middleware (configured per-endpoint)
- **Limits**: 100 req/min for admin APIs, 500 req/min for user APIs
- **Response**: 429 Too Many Requests with Retry-After header

## Key Implementation Patterns

### 1. Interface Segregation Principle (ISP)
```go
// Small focused interface per entity (5-7 methods max)
type AgentRepository interface {
    Create(ctx context.Context, agent *Agent) error
    Get(ctx context.Context, id string) (*Agent, error)
    Update(ctx context.Context, agent *Agent) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter *AgentFilter) ([]*Agent, error)
}
```

### 2. Repository Accessor Pattern
```go
// Adapter exposes repositories via accessors
type Adapter struct {
    // ... fields
}

func (a *Adapter) Agents() AgentRepository { return a.agentRepo }
func (a *Adapter) ThirdpartyServices() ThirdpartyOAuth2ServiceRepository { return a.serviceRepo }
func (a *Adapter) UserGrants() UserGrantRepository { return a.grantRepo }
```

### 3. Error Wrapping
```go
// All adapter errors wrapped in domain error
if err := a.db.GetContext(ctx, &agent, query, id); err == sql.ErrNoRows {
    return nil, storage.NewStorageError(
        "GetAgent",
        storage.ErrorKindNotFound,
        err,
        fmt.Sprintf("agent %s not found", id),
    )
}
```

### 4. Data Copying (In-Memory)
```go
// Prevent external mutation
func (a *Adapter) CreateAgent(ctx context.Context, agent *ports.Agent) error {
    agentCopy := *agent
    a.agents[agent.ID] = &agentCopy
    return nil
}
```

### 5. Context Propagation
```go
// All methods accept context for timeout/cancellation
ctx, cancel := context.WithTimeout(ctx, a.timeouts.Write)
defer cancel()

_, err := a.db.ExecContext(ctx, query, ...)
```

### 6. JSONB Handling (PostgreSQL)
```go
// Explicit marshal for JSONB columns
scopesJSON, err := json.Marshal(service.Scopes)
if err != nil {
    return storage.NewStorageError("CreateService", storage.ErrorKindUnknown, err, "failed to marshal scopes")
}

// Store as JSONB
_, err = a.db.ExecContext(ctx, query, ..., scopesJSON, ...)
```

### 7. Foreign Key Enforcement
```go
// Database constraint + explicit check for better error messages
count, err := a.UserGrants().CountByService(ctx, serviceID)
if err != nil {
    return storage.NewStorageError("DeleteService", storage.ErrorKindUnknown, err, "failed to check grants")
}
if count > 0 {
    return storage.NewStorageError(
        "DeleteService",
        storage.ErrorKindConflict,
        nil,
        fmt.Sprintf("cannot delete service: %d grants reference it", count),
    )
}
```

## Dependencies & Prerequisites

### Existing Infrastructure (from previous features)
- ✅ 20251214-120100-flexible-configuration: Configuration system (viper)
- ✅ 20251215-120000-dual-port-server: Dual HTTP ports (8000 enduser, 14000 admin) - Use existing servers
- ✅ 20251215-120100-persistence-layer: Storage ports and adapters (memory, postgres)
- ✅ 20251216-120000-session-management: Session middleware (principal extraction)

### New Dependencies (to be added)
- None - all required libraries already present

### Server Configuration
Use existing server configuration from 20251215-120000-dual-port-server:
- **Enduser server**: Port 8000 (default) - Add consent API routes
- **Admin server**: Port 14000 (default) - Add admin API routes

Register new routes in server setup:
```go
// In internal/adapters/http/server.go setupRoutes() method
// For admin server (name == "admin"):
s.router.Mount("/api/agents", agentsHandler)
s.router.Mount("/api/third-party/oauth2/clients", servicesHandler)

// For enduser server (name == "enduser"):
s.router.Mount("/api/consent", consentHandler)
```

## Next Steps

After completing this implementation plan:

1. **Run `/speckit.tasks`**: Generate detailed task breakdown from this plan
2. **Execute Phase 2.1-2.9**: Implement each phase sequentially
3. **Update ARCHITECTURE.md**: Add domain model glossary entries
4. **Update docs/**: Add API documentation
5. **Create PR**: Review against constitution checklist

## References

- **Specification**: [spec.md](spec.md)
- **API Contracts**: [contracts/openapi.yaml](contracts/openapi.yaml)
- **Data Model**: [data-model.md](data-model.md)
- **Implementation Guide**: [quickstart.md](quickstart.md)
- **Detailed Plan**: [implementation-plan.md](implementation-plan.md)
- **Persistence Patterns**: [../20251215-120100-persistence-layer/quickstart.md](../20251215-120100-persistence-layer/quickstart.md)
- **Constitution**: [../../.specify/memory/constitution.md](../../.specify/memory/constitution.md)
- **ADR 20251215-120300**: [../../adrs/20251215-120300-storage-layer-architecture.md](../../adrs/20251215-120300-storage-layer-architecture.md)
