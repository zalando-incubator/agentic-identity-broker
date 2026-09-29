# Implementation Tasks: Persistence Layer

**Feature**: 004-persistence-layer
**Generated**: 2025-12-15
**Branch**: 004-persistence-layer

## Quick Summary

This document breaks down the implementation of a hexagonal architecture persistence layer with in-memory and PostgreSQL backends into 60 actionable tasks across 6 phases.

**Total Tasks**: 119 (reduced by 1: ProductRepository deferred)
**User Stories**: 3 (P1: In-Memory, P2: PostgreSQL, P3: Configuration)
**Phases**: 6 (Setup, Foundation, US1, US2, US3, Polish)

**Scope**: Full hexagonal architecture with proper port/adapter separation, complete error handling, comprehensive testing, and production-ready configuration.

---

## Phase Overview & Dependencies

```
Phase 1 (Setup)
    ↓
Phase 2 (Foundation - blocking all user stories)
    ↓
Phase 3 (US1: In-Memory Backend) [Can run parallel with US2 after Phase 2]
    ↓
Phase 4 (US2: PostgreSQL Backend) [Can run parallel with US3 after Phase 2]
    ↓
Phase 5 (US3: Configuration) [Can run parallel after Phase 2]
    ↓
Phase 6 (Polish & Cross-Cutting)
```

**Independent Test Criteria**:
- **US1**: In-memory storage initializes instantly, CRUD operations work correctly, no persistence across restarts
- **US2**: PostgreSQL storage connects with valid credentials, CRUD operations persisted across restarts
- **US3**: Storage backend selection via config, zero runtime changes needed

**Suggested MVP**: Complete Phase 1-3 (Setup + Foundation + US1) to deliver in-memory storage, then US2 for production readiness.

---

## Phase 1: Setup & Project Initialization

Prepare project structure, dependencies, and configuration foundation.

### Setup Tasks

- [x] T001 Create project directories in `internal/domain/storage/`, `internal/adapters/storage/`, and `test/integration/storage/`
- [x] T002 Add Go module dependencies: `github.com/jmoiron/sqlx v1.3.5+`, `github.com/jackc/pgx/v5`, `github.com/testcontainers/testcontainers-go v0.28.0+`
- [x] T003 Run `go mod tidy` to verify dependencies and download packages
- [x] T004 Create example configuration files in `examples/config/`: `storage-memory.yaml` and `storage-postgres.yaml`
- [x] T005 Update `AGENT.md` to add `sqlx v1.3.5+`, `pgx v5`, and storage layer documentation to active technologies list

---

## Phase 2: Foundation & Hexagonal Architecture

Core infrastructure: ports, domain types, and factory pattern setup.

### Port Definition & Configuration

- [x] T006 Create `internal/ports/storage.go` with StorageLifecycle interface (Initialize, Close, HealthCheck methods)
- [x] T007 Add UserRepository interface to `internal/ports/storage.go` (CreateUser, GetUser, UpdateUser, DeleteUser, ListUsers methods)
- [x] T008 [P] Add StorageConfig struct to `internal/ports/config.go` with Backend, Postgres, Timeouts fields
- [x] T009 [P] Add PostgresConfig struct to `internal/ports/config.go` with ConnectionURL field
- [x] T010 [P] Add StorageTimeouts struct to `internal/ports/config.go` with Read (5s default) and Write (10s default) fields
- [x] T011 Add configuration validation tags to all storage config structs (required, oneof, validate:required_if)
- [x] T012 Extend main `Config` struct in `internal/ports/config.go` to include StorageConfig field with validation

### Domain Types & Errors

- [x] T013 Create `internal/domain/storage/backend.go` with StorageBackend enum (memory, postgres constants)
- [x] T014 Create `internal/domain/storage/errors.go` with ErrorKind enum (connection, timeout, validation, not_found, conflict, unknown)
- [x] T015 Create `internal/domain/storage/errors.go` with StorageError struct (Operation, Cause, Message, Kind fields) and error interface implementation
- [x] T016 Add NewStorageError factory function to `internal/domain/storage/errors.go`
- [x] T017 [P] Create `internal/domain/storage/types.go` with User struct (ID, Email, CreatedAt, UpdatedAt) and Validate method
- [x] T018 [P] Create `internal/domain/storage/connection.go` with ConnectionParameters struct and Redacted() logging method

### Configuration Validation & Parsing

- [x] T019 Add validator custom rules to `internal/config/validator.go` for postgresql:// URL format validation
- [ ] T020 Create configuration parsing logic in `internal/config/storage.go` for backend selection (memory vs postgres)
- [ ] T021 Test configuration validation in `internal/config/validator_test.go` with table-driven tests for valid/invalid backends
- [ ] T022 Add configuration loading integration tests in `internal/config/config_test.go` for storage configuration sections

### Storage Factory

- [x] T023 Create `internal/adapters/storage/factory.go` with NewAdapter factory function that creates appropriate adapter based on config
- [x] T024 Add error handling to factory for unsupported backends
- [x] T025 Write unit tests in `internal/adapters/storage/factory_test.go` for factory function

---

## Phase 3: User Story 1 - In-Memory Storage Implementation

Implement ephemeral storage for development and testing (zero-configuration option).

**Story Goal**: Developers can use in-memory storage with instant initialization and zero external dependencies

**Independent Test**: Start app with in-memory config → perform CRUD operations → restart → verify data lost

### Memory Adapter Core

- [x] T026 [P] [US1] Create `internal/adapters/storage/memory/adapter.go` with Adapter struct (mu sync.RWMutex, users map[string]*storage.User)
- [x] T027 [P] [US1] Implement Initialize() method in memory adapter (returns nil immediately, no I/O)
- [x] T028 [P] [US1] Implement Close() method in memory adapter (clears internal map, no-op cleanup)
- [x] T029 [P] [US1] Implement HealthCheck() method in memory adapter (returns nil, always healthy)
- [x] T030 [US1] Implement CreateUser() in memory adapter with RWMutex Lock, duplicate key checking, data copying
- [x] T031 [US1] Implement GetUser() in memory adapter with RLock (concurrent reads), data copying to prevent external mutation
- [x] T032 [US1] Implement UpdateUser() in memory adapter with Lock, existence checking, immutable update pattern
- [x] T033 [US1] Implement DeleteUser() in memory adapter with Lock, existence checking
- [x] T034 [US1] Implement ListUsers() in memory adapter with RLock, optional filtering support, data copying for all results

### Memory Adapter Tests

- [x] T035 [P] [US1] Create `internal/adapters/storage/memory/adapter_test.go` with table-driven tests for all operations
- [x] T036 [US1] Write concurrent access tests in memory adapter test file (100+ concurrent readers to verify RLock works)
- [x] T037 [US1] Write isolation tests to verify separate in-memory instances don't share data
- [x] T038 [US1] Write timeout tests to verify context cancellation doesn't crash adapter
- [x] T039 [US1] Add race detector tests: `go test -race ./internal/adapters/storage/memory/...`

### Memory Adapter Documentation & Configuration

- [x] T040 [US1] Create in-memory configuration example in `examples/config/storage-memory.yaml` with backend=memory, timeouts
- [ ] T041 [US1] Update quickstart guide section on in-memory implementation with usage examples
- [ ] T042 [US1] Create `internal/adapters/storage/memory/README.md` documenting in-memory limitations and use cases

### Integration & Verification

- [ ] T043 [US1] Integrate memory adapter into main application startup in `cmd/agentic-identity-broker/main.go`
- [ ] T044 [US1] Test full lifecycle: initialize → CRUD → health check → close in integration tests
- [x] T045 [US1] Verify startup time < 1 second (SC-001) with benchmarks

---

## Phase 4: User Story 2 - PostgreSQL Storage Implementation

Implement durable production-grade storage with connection pooling and schema versioning.

**Story Goal**: Operations teams can deploy with PostgreSQL for production workloads with automatic connection pooling and schema validation

**Independent Test**: Configure PostgreSQL → start app → store data → restart → verify data persists

### PostgreSQL Adapter Core

- [x] T046 [P] [US2] Create `internal/adapters/storage/postgres/adapter.go` with Adapter struct (db *sqlx.DB, config, timeouts)
- [x] T047 [P] [US2] Implement NewAdapter() factory function for PostgreSQL adapter
- [x] T048 [US2] Implement Initialize() in postgres adapter: connect via sqlx, configure pool (25 max, 5 min), verify schema
- [x] T049 [US2] Implement schema verification method in postgres adapter with expected version checking (FR-013)
- [x] T050 [US2] Implement Close() in postgres adapter with graceful pool shutdown
- [x] T051 [US2] Implement HealthCheck() in postgres adapter with Ping validation
- [x] T052 [US2] Implement CreateUser() in postgres adapter with 10s write timeout, parameterized queries, duplicate key error mapping
- [x] T053 [US2] Implement GetUser() in postgres adapter with 5s read timeout, sqlx struct scanning, not-found error mapping
- [x] T054 [US2] Implement UpdateUser() in postgres adapter with 10s write timeout, existence checking
- [x] T055 [US2] Implement DeleteUser() in postgres adapter with 10s write timeout
- [x] T056 [US2] Implement ListUsers() in postgres adapter with 5s read timeout, optional filtering, pagination support

### PostgreSQL Error Mapping

- [x] T057 [US2] Create error mapping functions in postgres adapter for PostgreSQL-specific errors (duplicate key 23505, connection errors, timeouts)
- [x] T058 [US2] Implement StorageError wrapping for all database operations

### PostgreSQL Connection Pooling

- [x] T059 [US2] Configure connection pool in postgres adapter: MaxOpenConns=25, MaxIdleConns=5, MaxConnLifetime=1h, MaxConnIdleTime=15m
- [ ] T060 [US2] Test connection pool behavior with concurrent access (1,000+ operations per SC-005)

### PostgreSQL Migration & Schema Setup

- [ ] T061 [US2] Create `migrations/` directory structure for schema migrations
- [ ] T062 [US2] Create `migrations/001_initial_schema.sql` with users table and schema_migrations tracking table
- [ ] T063 [US2] Document migration procedures in `migrations/README.md` with clear runbook for operators
- [ ] T064 [US2] Create migration verification test that checks expected schema version on startup

### PostgreSQL Configuration

- [ ] T065 [US2] Create PostgreSQL configuration example in `examples/config/storage-postgres.yaml` with production settings
- [ ] T066 [US2] Document connection string format and SSL/TLS options (sslmode=verify-full default)
- [ ] T067 [US2] Add environment variable examples for connection credentials with proper masking

### PostgreSQL Tests

- [x] T068 [P] [US2] Create `internal/adapters/storage/postgres/adapter_test.go` with unit tests (mocked queries)
- [x] T069 [P] [US2] Create `test/integration/storage/postgres_test.go` with testcontainers-based integration tests
- [x] T070 [US2] Write integration test for schema version checking (both valid and mismatch scenarios)
- [x] T071 [US2] Write integration test for timeout enforcement (read 5s, write 10s)
- [x] T072 [US2] Write integration test for connection failure scenarios
- [x] T073 [US2] Write integration test for duplicate key detection
- [ ] T074 [US2] Run PostgreSQL integration tests in CI with Docker container

### PostgreSQL Documentation

- [ ] T075 [US2] Create `docs/storage.md` with comprehensive storage layer documentation
- [ ] T076 [US2] Update `ARCHITECTURE.md` with persistence layer subsystem section (3.1.2)
- [ ] T077 [US2] Update glossary in ARCHITECTURE.md with storage terminology (StoragePort, StorageAdapter, StorageBackend, etc.)

---

## Phase 5: User Story 3 - Configuration-Driven Backend Selection

Enable runtime storage backend selection without code changes.

**Story Goal**: System administrators can configure storage backend at deployment time for development, testing, and production environments

**Independent Test**: Deploy with memory config → verify memory storage used → redeploy with postgres config → verify postgres storage used

### Configuration Integration

- [x] T078 [P] [US3] Integrate StorageConfig into main application configuration loading (`cmd/agentic-identity-broker/main.go`)
- [x] T079 [P] [US3] Implement backend factory selection logic in main startup sequence
- [x] T080 [US3] Add configuration validation before adapter initialization
- [x] T081 [US3] Implement clear error messages for configuration validation failures

### Configuration Examples & Documentation

- [x] T082 [US3] Create development configuration (`configs/config.dev.yaml`) with in-memory backend
- [x] T083 [US3] Create production configuration (`configs/config.prod.yaml`) with PostgreSQL backend
- [x] T084 [US3] Create testing configuration (`configs/config.test.yaml`) with in-memory backend for automated tests
- [x] T085 [US3] Document configuration switching procedures for operators
- [x] T086 [US3] Add configuration precedence documentation (env > YAML > defaults)

### Runtime Verification

- [x] T087 [US3] Write tests that verify backend selection based on configuration
- [ ] T088 [US3] Add environment variable override tests (IDENTITY_BROKER_STORAGE_BACKEND)
- [x] T089 [US3] Test that wrong backend type fails startup with clear error message

### Backend Switching Tests

- [x] T090 [US3] Create integration test that verifies memory → postgres backend switch
- [x] T091 [US3] Create integration test for invalid backend configuration handling
- [ ] T092 [US3] Create integration test for missing required PostgreSQL parameters

---

## Phase 6: Polish, Testing & Documentation

Cross-cutting concerns, comprehensive testing, and final documentation.

### Comprehensive Testing

- [x] T093 Create `test/integration/storage/` test suite with shared fixtures
- [x] T094 Write end-to-end tests for full lifecycle (init → CRUD → health → close)
- [x] T095 Write concurrent stress tests for both adapters (1000+ operations)
- [x] T096 Write timeout tests for all operations (verify 5s reads, 10s writes)
- [x] T097 Write error recovery tests (connection failures, graceful degradation)
- [x] T098 Run full test suite with race detector: `go test -race ./...`
- [x] T099 Achieve minimum 80% code coverage for storage layer (measure with `go tool cover`)

### Performance & Benchmarking

- [ ] T100 Create benchmarks in `internal/adapters/storage/memory/adapter_bench_test.go` for in-memory operations
- [ ] T101 Create benchmarks in `internal/adapters/storage/postgres/adapter_bench_test.go` for PostgreSQL operations
- [ ] T102 Verify in-memory startup < 1 second (SC-001)
- [ ] T103 Verify PostgreSQL supports 1000+ concurrent operations (SC-005)
- [ ] T104 Document performance characteristics in ARCHITECTURE.md

### Security & Compliance

- [ ] T105 Verify connection strings never logged in plain text (SR-001)
- [ ] T106 Verify TLS/SSL support in PostgreSQL configuration (SR-002)
- [ ] T107 Verify SSL verification failure causes startup failure (SR-003)
- [ ] T108 Add security checklist to documentation (credentials handling, SSL defaults)
- [ ] T109 Create security review checklist for contributors

### Documentation & ADRs

- [x] T110 Create Architecture Decision Record: `adrs/004-storage-layer-architecture.md`
- [ ] T111 Document hexagonal architecture pattern in ARCHITECTURE.md
- [ ] T112 Create contributor guide: `docs/storage-extension-guide.md` for adding new entities
- [ ] T113 Add storage layer troubleshooting guide to docs/
- [ ] T114 Create migration guide for adding new entity repositories

### Code Quality & CI/CD

- [ ] T115 Ensure `just check` (fmt, vet, lint) and `just verify` both pass
- [ ] T116 Configure CI pipeline to run tests with race detector
- [ ] T117 Configure CI pipeline to run integration tests with PostgreSQL container
- [ ] T118 Add coverage reporting to CI pipeline (fail if < 80%)
- [ ] T119 Add pre-commit hook to run storage tests locally

---

## Test Coverage Map

| Component | Type | Location | Count |
|-----------|------|----------|-------|
| Memory Adapter | Unit | adapter_test.go | 8 |
| Memory Adapter | Concurrent | adapter_test.go | 3 |
| Memory Adapter | Isolation | adapter_test.go | 2 |
| PostgreSQL Adapter | Unit | adapter_test.go | 8 |
| PostgreSQL Adapter | Integration | integration_test.go | 8 |
| Configuration | Unit | config_test.go | 4 |
| Factory | Unit | factory_test.go | 3 |
| Error Mapping | Unit | errors_test.go | 4 |
| **Total** | | | **40+** |

---

## Success Criteria Verification Map

| SC ID | Criteria | Verification Task | Phase |
|-------|----------|-------------------|-------|
| SC-001 | In-memory startup < 1s | T045 (benchmark) | US1 |
| SC-002 | PostgreSQL persists across restarts | T069-073 (integration) | US2 |
| SC-003 | Backend switchable via config | T087-091 (config tests) | US3 |
| SC-004 | 100% of failures have clear errors | T058 (error mapping) | US2 |
| SC-005 | PostgreSQL startup failure prevents inconsistency | T048, T064 | US2 |
| SC-006 | Timeout enforcement (5s read, 10s write) | T071, T096 | US2 |

---

## Functional Requirement Coverage Map

| FR ID | Requirement | Implementation Task | Phase |
|-------|-------------|-------------------|-------|
| FR-001 | In-memory storage support | T026-045 | US1 |
| FR-002 | PostgreSQL storage support | T046-077 | US2 |
| FR-003 | Exactly one backend at runtime | T078-081 | US3 |
| FR-004 | In-memory ephemeral behavior | T027-029, T039 | US1 |
| FR-005 | PostgreSQL durability | T062-064 | US2 |
| FR-006 | Storage initialization at startup | T048, T078-079 | US2 |
| FR-007 | Fail startup on init failure | T081, T092 | US3 |
| FR-008 | Configuration specifies backend | T008, T078-086 | US3 |
| FR-009 | PostgreSQL URL format support | T019, T065-066 | US2 |
| FR-010 | Configuration validation | T011-012, T020-022 | Phase 2 |
| FR-011 | Graceful error messages | T058, T105-106 | Phase 6 |
| FR-012 | Clear PostgreSQL errors at startup | T049, T064 | US2 |
| FR-013 | Schema version verification | T049, T062-064, T070 | US2 |

---

## Security Requirement Coverage Map

| SR ID | Requirement | Implementation Task | Phase |
|-------|-------------|-------------------|-------|
| SR-001 | Credentials not logged | T105 | Phase 6 |
| SR-002 | TLS/SSL support | T066, T106 | US2 |
| SR-003 | Fail closed on SSL failure | T107 | Phase 6 |
| SR-004 | Config parameter protection | T008-009, T066 | US3 |
| SR-005 | Safe error messages | T058, T105-106 | Phase 6 |

---

## Quick Reference: Task Grouping by Dependency

### Can Run in Parallel (After Phase 2 Complete)

**US1 Tasks**: T026-045 (In-memory adapter + tests)
**US2 Tasks**: T046-077 (PostgreSQL adapter + tests + migrations)
**US3 Tasks**: T078-092 (Configuration integration + tests)

These three user story groups are **independent** and can proceed in parallel after foundational tasks complete.

---

## Execution Sequence Recommendation

1. **Complete Phase 1-2 first** (Setup + Foundation) - 25 tasks, ~2-3 days
2. **Run US1 in parallel with US2** - 31 tasks, ~4-5 days (can overlap)
3. **Implement US3 after US1/US2** - 15 tasks, ~2 days
4. **Execute Phase 6 in parallel with testing** - 27 tasks, ~3 days (overlaps with US1/US2)

**Total Estimated Tasks**: 119 discrete implementation items (reduced by 1: ProductRepository deferred)
**Realistic Timeline**: 7-10 working days for solo developer, 4-5 days with 2 developers

---

## Task Execution Checklist Instructions

1. Before starting any phase, verify all dependencies are complete
2. For each task, update the checkbox when complete: `- [x]`
3. Mark story tasks in progress with `[IN PROGRESS]` comment
4. Run `just check` and `just verify` after each phase completion
5. Commit after each phase: `git commit -m "feat: Complete Phase X tasks"`

Example progress tracking:
```
- [x] T001 Create project directories
- [x] T002 Add dependencies
- [ ] [IN PROGRESS] T003 Run go mod tidy
- [ ] T004 Create example configurations
```

---

**Status**: Ready for implementation
**Next Steps**:
1. Review and approve task list
2. Begin Phase 1 setup tasks
3. Run `/speckit.implement` to start automated execution (when available)

