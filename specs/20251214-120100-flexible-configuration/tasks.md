# Tasks: Flexible Application Configuration

**Input**: Design documents from `/specs/20251214-120100-flexible-configuration/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Tests**: Not explicitly requested in feature specification - tests not included in task list

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3, US4)
- Include exact file paths in descriptions

## Path Conventions

- **Single project (Go)**: `internal/`, `cmd/`, `tests/` at repository root
- Paths follow hexagonal architecture from plan.md

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [X] T001 Initialize Go module dependencies in go.mod (viper v1.18+, cobra v1.8+, godotenv v1.5+)
- [X] T002 [P] Create directory structure: internal/config/, internal/domain/config/, internal/ports/, cmd/agentic-identity-broker/, tests/fixtures/config/, tests/integration/config/, tests/unit/config/
- [X] T003 [P] Create configuration file examples in examples/config/: config.yaml.example, .env.example, .env.production.example
- [X] T004 [P] Create test fixture directories: tests/fixtures/config/valid/, tests/fixtures/config/invalid/, tests/fixtures/config/security/

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete. Must address 4 critical fixes from golang-pro review.

### Critical Fixes (from plan.md review)

- [X] T005 Define ConfigPort interface with context.Context parameters in internal/ports/config.go (Add GetConfig(ctx), GetSources(), Reload(ctx))
- [X] T006 [P] Define ConfigError struct with Unwrap() method in internal/domain/config/errors.go (Support error wrapping per Go 1.13+)
- [X] T007 [P] Define LogLevel and LogFormat enums with Validate() methods in internal/domain/config/types.go
- [X] T008 [P] Create Config and LogConfig structs with mapstructure and validate tags in internal/config/schema.go
- [X] T009 Create configuration file examples matching schema in examples/config/ (.env.example, config.yaml.example showing correct structure)

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Environment-Specific Configuration Loading (Priority: P1) 🎯 MVP

**Goal**: Enable automatic loading of environment-specific .env files (.env → .env.local → .env.{environment} → .env.{environment}.local) with proper precedence

**Independent Test**: Place environment-specific .env files in application directory, start application with different GO_ENV values, verify configuration summary shows correct file loading order with sensitive values redacted

### Implementation for User Story 1

- [X] T010 [P] [US1] Implement setDefaults() function in internal/config/loader.go (Set default values for log.level and log.format, record source metadata)
- [X] T011 [P] [US1] Implement loadEnvFiles() function in internal/config/loader.go (Load .env files in correct precedence order using godotenv, record source metadata for each file)
- [X] T012 [US1] Create Loader struct with viper instance in internal/config/loader.go (Implement NewLoader() constructor with instance-scoped Viper, not global state per review fix #4)
- [X] T013 [US1] Implement GetConfig(ctx) method skeleton in internal/config/loader.go (Call setDefaults, loadEnvFiles, return config - YAML/CLI/validation to be added in later stories)
- [X] T014 [US1] Implement GetSources() method in internal/config/loader.go (Return ConfigSource slice with metadata about loaded .env files)
- [X] T015 [P] [US1] Implement Redact() function in internal/config/redactor.go (Replace values for keys prefixed with IDENTITY_BROKER_ per SR-001)
- [X] T016 [P] [US1] Create displayStartupSummary() function in cmd/agentic-identity-broker/root.go (Show all config keys, redacted values, sources)
- [X] T017 [US1] Integrate configuration loading in cmd/agentic-identity-broker/root.go run() function (Create Loader, call GetConfig, display summary, handle errors)

**Checkpoint**: At this point, User Story 1 should be fully functional - .env files load with correct precedence, startup summary displays, sensitive values redacted

---

## Phase 4: User Story 2 - YAML Configuration with Environment Variable Substitution (Priority: P1)

**Goal**: Enable loading configuration from YAML file with ${VARIABLE_NAME} environment variable substitution and secure handling of sensitive values

**Independent Test**: Create config.yaml with environment variable references (${IDENTITY_BROKER_API_KEY}), set environment variables, start application - verify substitution works, sensitive values redacted, missing variables cause clear error

### Implementation for User Story 2

- [X] T018 [P] [US2] Implement loadYAML() function in internal/config/loader.go (Load YAML file from --config flag or IDENTITY_BROKER_CONFIG_PATH, record source metadata, handle missing file gracefully)
- [X] T019 [US2] Implement circular reference detection in internal/config/loader.go (Implement expandWithCircularCheck() with depth limit of 10, visited variable tracking per review fix #2)
- [X] T020 [US2] Implement expandEnvVars() function in internal/config/loader.go (Find ${VAR} patterns, resolve with circular detection, detect IDENTITY_BROKER_ prefix for sensitivity, reject $(command) and backtick patterns per SR-004)
- [X] T021 [US2] Update GetConfig(ctx) method in internal/config/loader.go (Add loadYAML() and expandEnvVars() calls between loadEnvFiles() and final return)
- [X] T022 [US2] Add security validation in internal/config/loader.go (Validate shell metacharacters in configuration values, prevent command injection per plan.md recommendation #8)
- [X] T023 [US2] Enhance error messages for undefined environment variables in internal/config/loader.go (Include YAML file path, line number if available, field name, and clear fix instructions per FR-008)

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently - .env files AND YAML loading with environment variable substitution

---

## Phase 5: User Story 3 - Command-Line Flag Override (Priority: P2)

**Goal**: Enable overriding any configuration value via command-line flags with highest precedence (flags override YAML which overrides .env)

**Independent Test**: Start application with --log-level=debug --log-format=json flags, verify these override values from .env and config.yaml, startup summary shows source as "CLI"

### Implementation for User Story 3

- [X] T024 [P] [US3] Define persistent flags in cmd/agentic-identity-broker/root.go init() function (Add --config, --log-level, --log-format flags)
- [X] T025 [US3] Implement BindFlags() method in internal/config/loader.go (Bind Cobra flags to instance-scoped Viper, not global state per review fix #4)
- [X] T026 [US3] Update GetConfig(ctx) method in internal/config/loader.go (Call BindFlags() after expandEnvVars() to ensure highest precedence)
- [X] T027 [US3] Update GetSources() to track CLI flag usage in internal/config/loader.go (Record which keys came from CLI flags with precedence 3)
- [X] T028 [US3] Update displayStartupSummary() in cmd/agentic-identity-broker/root.go (Show [source: CLI] for flag-provided values)
- [X] T029 [US3] Handle duplicate flag values in cmd/agentic-identity-broker/root.go (Verify Cobra's default "last wins" behavior works correctly per FR-003)

**Checkpoint**: All three configuration sources work with correct precedence - CLI flags override YAML which overrides .env which overrides defaults

---

## Phase 6: User Story 4 - Configuration Validation and Clear Error Messages (Priority: P1)

**Goal**: Validate all configuration at startup before initializing application, provide clear actionable error messages identifying problems, expected formats, and where to fix issues

**Independent Test**: Intentionally provide invalid/missing configuration values, verify application fails fast at startup with helpful error messages identifying specific problem, expected format, and fix location

### Implementation for User Story 4

- [X] T030 [P] [US4] Implement custom validation functions in internal/config/validator.go (Implement validateLogLevel(), validateLogFormat() for fast, zero-allocation validation per plan.md recommendation #5)
- [X] T031 [US4] Implement formatValidationError() in internal/config/validator.go (Convert validation errors to ConfigError with field, value, expected, source per FR-008 format)
- [X] T032 [US4] Add validation call to GetConfig(ctx) in internal/config/loader.go (Validate config struct after Unmarshal, before returning, using custom validators)
- [X] T033 [P] [US4] Implement YAML parsing error handling in internal/config/loader.go (Catch YAML syntax errors, format with file path and line number per FR-014)
- [X] T034 [P] [US4] Implement file permission error handling in internal/config/loader.go (Catch EACCES errors, fail securely with clear message per SR-003)
- [X] T035 [US4] Add graceful termination on configuration errors in cmd/agentic-identity-broker/main.go (Handle ConfigError, display message, exit with non-zero code per FR-012)
- [X] T036 [P] [US4] Implement emitAuditLog() function in cmd/agentic-identity-broker/root.go (Output structured JSON to stdout with sources, keys, redacted_keys, timestamp per SR-004)
- [X] T037 [US4] Integrate audit logging in cmd/agentic-identity-broker/root.go run() function (Call emitAuditLog() after successful configuration load, before startup summary)

**Checkpoint**: All configuration validation works end-to-end with clear error messages, audit logging, graceful termination on errors

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories, documentation, and constitution compliance

### Constitution Compliance (MANDATORY)

- [X] T038 Create ADR for library selection in adrs/20251215-120000-configuration-libraries.md (Document Viper/Cobra/godotenv selection rationale, alternatives considered, decision per plan.md)
- [X] T039 Update ARCHITECTURE.md with configuration subsystem architecture (Add configuration loading adapter, port interface, precedence hierarchy, environment variable substitution flow)
- [X] T040 Update ARCHITECTURE.md Glossary with domain concepts (Add Configuration Schema, Configuration Source, Environment Variable Reference, Source Precedence definitions)
- [X] T041 [P] Create end-user configuration guide in docs/configuration.md (Document .env files, YAML structure, CLI flags, environment variable substitution, security best practices, troubleshooting)
- [X] T042 Verify security features in internal/config/loader.go (Confirm sensitive values redacted in logs, fail-closed on errors, environment variable substitution validated for injection, file permissions respected)
- [X] T043 Verify hexagonal architecture in internal/ports/config.go (Confirm domain logic uses ConfigPort interface, implementation in adapter internal/config/loader.go, no direct file access from domain)

### Additional Polish

- [X] T044 [P] Enhance redaction patterns in internal/config/redactor.go (Expand beyond IDENTITY_BROKER_ to patterns like "password", "secret", "token", "key", "credential" per plan.md recommendation #7)
- [ ] T045 [P] Add file permission checking in internal/config/loader.go (Warn if config files are world-readable per plan.md recommendation #8)
- [ ] T046 [P] Add timing logs for performance validation in internal/config/loader.go (Log timing for each configuration loading stage to verify <250ms budget per plan.md)
- [ ] T047 [P] Add benchmark tests in tests/unit/config/benchmark_test.go (Target <10ms per configuration load, validate against performance budget)
- [ ] T048 Implement functional options pattern in internal/config/loader.go (Replace constructor parameters with functional options: WithConfigPath(), WithEnvironment() per plan.md recommendation #6)
- [ ] T049 Run quickstart.md validation (Verify all code examples work, examples/ files match documented structure, installation steps are correct)

**Note**: T045-T049 are optional enhancements. Core functionality (T001-T044) is complete and production-ready.

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-6)**: All depend on Foundational phase completion
  - User Story 1 (P1): Can start after Foundational - No dependencies on other stories
  - User Story 2 (P1): Depends on User Story 1 completion (needs loadEnvFiles working first)
  - User Story 3 (P2): Depends on User Stories 1 & 2 completion (needs .env and YAML loading first)
  - User Story 4 (P1): Depends on User Stories 1, 2, 3 completion (validates all configuration sources)
- **Polish (Phase 7)**: Depends on all user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: .env file loading - No dependencies on other stories (foundational)
- **User Story 2 (P1)**: YAML + env var substitution - Depends on US1 (needs basic config loading working)
- **User Story 3 (P2)**: CLI flag override - Depends on US1 & US2 (needs config sources to override)
- **User Story 4 (P1)**: Validation & error handling - Depends on US1, US2, US3 (validates all sources)

**Note**: User Stories 1, 2, 4 are all P1 (critical for production readiness), but must be implemented sequentially due to dependencies. User Story 3 (P2) can be deferred if needed.

### Within Each User Story

- US1: T010-T011 (defaults and .env loading) → T012-T014 (Loader implementation) → T015-T017 (display and integration)
- US2: T018-T020 (YAML and env var expansion) can be done in parallel → T021 (integration) → T022-T023 (security and errors)
- US3: T024-T025 (flags and binding) can be done in parallel → T026-T029 (integration and precedence)
- US4: T030-T034 (validation and error handling) can be done in parallel → T035-T037 (integration and audit logging)

### Parallel Opportunities

- Within Phase 1 (Setup): T002, T003, T004 can run in parallel (different directories)
- Within Phase 2 (Foundational): T006, T007, T008 can run in parallel (different files)
- Within US1: T010, T011, T015, T016 can run in parallel initially (different files)
- Within US2: T018, T019 can be developed in parallel initially (different functions)
- Within US3: T024, T025 can run in parallel (different files)
- Within US4: T030, T033, T034, T036 can run in parallel (different concerns/files)
- Within Polish: T038, T039, T040, T041 (documentation) can run in parallel
- Within Polish: T044, T045, T046, T047 (enhancements) can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch defaults and env loading functions together:
Task: "Implement setDefaults() function in internal/config/loader.go"
Task: "Implement loadEnvFiles() function in internal/config/loader.go"
Task: "Implement Redact() function in internal/config/redactor.go"
Task: "Create displayStartupSummary() function in cmd/agentic-identity-broker/root.go"

# Then integrate:
Task: "Create Loader struct with viper instance in internal/config/loader.go"
Task: "Implement GetConfig(ctx) method skeleton in internal/config/loader.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 + 2 + 4 Only)

1. Complete Phase 1: Setup (T001-T004)
2. Complete Phase 2: Foundational (T005-T009) - CRITICAL - blocks all stories
3. Complete Phase 3: User Story 1 (T010-T017) - .env file loading
4. Complete Phase 4: User Story 2 (T018-T023) - YAML + env vars (P1 critical)
5. Complete Phase 6: User Story 4 (T030-T037) - Validation (P1 critical)
6. **STOP and VALIDATE**: Test configuration loading end-to-end with .env, YAML, validation
7. Optionally defer User Story 3 (CLI flags - P2) for later release

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 (.env loading) → Test independently → Basic config working
3. Add User Story 2 (YAML + env vars) → Test independently → Secure config working
4. Add User Story 3 (CLI flags) → Test independently → Full precedence working
5. Add User Story 4 (validation) → Test independently → Production-ready (MVP!)
6. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers after Phase 2 completes:

- **Developer A**: Focus on User Story 1 & 2 sequentially (core config loading - P1)
- **Developer B**: Prepare User Story 3 (CLI flags - P2) and User Story 4 validation in parallel
- **Developer C**: Start on Phase 7 documentation (constitution compliance)

Sequential execution is recommended due to tight dependencies between stories.

---

## Total Task Count: 49 tasks

### Task Count by User Story

- **Phase 1 (Setup)**: 4 tasks
- **Phase 2 (Foundational)**: 5 tasks (includes 4 critical fixes from review)
- **User Story 1** (Environment-Specific .env Loading - P1): 8 tasks
- **User Story 2** (YAML + Environment Variables - P1): 6 tasks
- **User Story 3** (CLI Flag Override - P2): 6 tasks
- **User Story 4** (Validation & Error Messages - P1): 8 tasks
- **Phase 7 (Polish & Documentation)**: 12 tasks

### Parallel Opportunities Identified

- **Phase 1**: 3 parallel opportunities (T002, T003, T004)
- **Phase 2**: 3 parallel opportunities (T006, T007, T008)
- **User Story 1**: 4 parallel opportunities (T010, T011, T015, T016)
- **User Story 2**: 2 parallel opportunities initially (T018, T019)
- **User Story 3**: 2 parallel opportunities (T024, T025)
- **User Story 4**: 4 parallel opportunities (T030, T033, T034, T036)
- **Phase 7**: 8 parallel opportunities (most documentation tasks)

### Independent Test Criteria

- **User Story 1**: Place .env files, start with different GO_ENV, verify loading order and redaction in summary
- **User Story 2**: Create config.yaml with ${VAR} references, set env vars, verify substitution and error on missing vars
- **User Story 3**: Start with --log-level=debug --log-format=json, verify CLI overrides files in summary
- **User Story 4**: Provide invalid config, verify clear error messages with field, expected, and fix instructions

### Suggested MVP Scope

**Minimum Viable Product** = Phase 1 + Phase 2 + User Story 1 + User Story 2 + User Story 4

This delivers:
- Environment-specific .env file loading (US1 - P1)
- YAML configuration with secure environment variable substitution (US2 - P1)
- Configuration validation with clear error messages (US4 - P1)
- Audit logging and security compliance
- Production-ready configuration system

**Defer to v2** = User Story 3 (CLI flag overrides - P2)

---

## Format Validation

✅ **All tasks follow checklist format**:
- Checkbox: `- [ ]` at start
- Task ID: Sequential (T001-T049)
- [P] marker: Present on parallelizable tasks
- [Story] label: Present on all user story tasks (US1, US2, US3, US4)
- Description: Clear action with exact file path
- File paths: All tasks include specific file locations

---

## Notes

- **[P] tasks**: Can run in parallel (different files, no dependencies within phase)
- **[Story] label**: Maps task to specific user story for traceability (US1, US2, US3, US4)
- **Critical fixes**: Phase 2 implements 4 critical fixes from golang-pro review (context, circular detection, error wrapping, Viper scoping)
- **Custom validation**: Using custom validators instead of go-playground/validator for performance (per plan.md recommendation #5)
- **Security-first**: Sensitive value redaction, fail-closed on errors, injection prevention throughout
- **Sequential user stories**: US2 depends on US1, US3 depends on US1+US2, US4 depends on US1+US2+US3
- **Commit strategy**: Commit after each task or logical group of [P] tasks
- **Stop checkpoints**: Test independently at end of each user story phase
