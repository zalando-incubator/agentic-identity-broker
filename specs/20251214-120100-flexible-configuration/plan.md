# Implementation Plan: Flexible Application Configuration

**Branch**: `20251214-120100-flexible-configuration` | **Date**: 2025-12-14 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/20251214-120100-flexible-configuration/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Enable flexible configuration of the Agentic Identity Broker through multiple sources (.env files, YAML files, command-line flags) with environment variable substitution for sensitive data. This allows deployment across different environments (development, staging, production) without code changes. Implementation uses Viper for unified configuration management, Cobra for CLI flag parsing, and godotenv for .env file support.

**Review Status**: golang-pro agent review completed (Grade: B+ → A- with fixes)
**Critical Fixes Required**: 4 issues must be addressed before implementation (see [REVIEW.md](REVIEW.md))

## Technical Context

**Language/Version**: Go 1.21+
**Primary Dependencies**:
- github.com/spf13/viper (v1.18+) - Unified configuration management
- github.com/spf13/cobra (v1.8+) - CLI command and flag parsing
- github.com/joho/godotenv (v1.5+) - .env file loading
- **Note**: Custom validation recommended over go-playground/validator for initial scope (performance + simplicity)
**Storage**: File system (configuration files: .env, YAML)
**Testing**: Go standard testing package (table-driven tests), testify/require for test setup only
**Target Platform**: Linux/macOS/Windows server environments
**Project Type**: Single (backend service with CLI)
**Performance Goals**: Application startup <250ms for configuration loading (realistic budget), total <5 seconds
**Constraints**: Configuration files memory-bound (no explicit size limit), synchronous validation at startup
**Scale/Scope**: Initial scope limited to logging configuration (level, format); extensible for future config categories

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

- [x] **Security-First**: YES - Sensitive values redacted in logs (SR-001, SR-002), environment variable substitution validated for injection (SR-004), file permissions respected (SR-003), fail-closed on errors (FR-012, Assumption 6)
- [x] **Architecture Docs**: YES - Will update ARCHITECTURE.md with configuration subsystem architecture and Glossary entries for domain concepts (Configuration Schema, Configuration Source, Environment Variable Reference)
- [x] **ADRs**: YES - Requires ADR for library selection (Viper/Cobra/godotenv) and configuration precedence strategy
- [x] **Library-First Security**: YES - Using standard Go libraries and established open-source packages (Viper, Cobra, godotenv); no custom cryptography
- [x] **API Documentation**: YES - Will update docs/ with configuration guide for operators (environment setup, YAML structure, CLI flags)
- [x] **Domain Model**: YES - New domain concepts (Configuration Schema, Configuration Source, Environment Variable Reference) will be added to ARCHITECTURE.md Glossary
- [x] **Hexagonal Architecture**: YES - Configuration loading is an adapter (driven/outbound); domain logic depends on configuration port (interface) for retrieval

*All checks pass. No violations requiring justification.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
internal/
├── config/
│   ├── config.go              # Configuration port (interface)
│   ├── loader.go              # Configuration loading adapter (Viper integration)
│   ├── validator.go           # Configuration validation logic
│   ├── schema.go              # Configuration schema definitions
│   └── redactor.go            # Sensitive value redaction utility
├── domain/
│   └── config/
│       ├── types.go           # Domain types (LogLevel, LogFormat enums)
│       └── errors.go          # Configuration-specific domain errors
└── ports/
    └── config.go              # Configuration port interface definition

cmd/
└── agentic-identity-broker/
    ├── main.go                # Application entry point
    └── root.go                # Cobra root command with flag definitions

tests/
├── integration/
│   └── config/
│       ├── env_loading_test.go
│       ├── yaml_loading_test.go
│       ├── cli_override_test.go
│       └── validation_test.go
└── unit/
    └── config/
        ├── validator_test.go
        ├── redactor_test.go
        └── schema_test.go

docs/
└── configuration.md           # End-user configuration guide

adrs/
└── 20251215-120000-configuration-libraries.md  # ADR for Viper/Cobra/godotenv selection

# Configuration file examples
examples/
└── config/
    ├── config.yaml.example
    ├── .env.example
    └── .env.production.example
```

**Structure Decision**: Single project (backend service) following hexagonal architecture. Configuration loading is a driven adapter in `internal/config/` implementing the port defined in `internal/ports/config.go`. Domain logic accesses configuration through the port interface, not directly from files. CLI entry point in `cmd/` uses Cobra for flag parsing and delegates to the configuration adapter.

## Complexity Tracking

No constitution violations. This section intentionally left empty.

## Critical Implementation Notes

**IMPORTANT**: Before beginning implementation, address these 4 critical issues identified in the golang-pro review:

### 1. Add Context to ConfigPort Interface

**Location**: `contracts/config-port.go`, `internal/ports/config.go`

**Required Change**:
```go
type ConfigPort interface {
    GetConfig(ctx context.Context) (*Config, error)  // Add ctx parameter
    GetSources() []ConfigSource
    Reload(ctx context.Context) error                 // Add ctx parameter
}
```

**Rationale**: Configuration loading involves I/O operations. Context enables timeouts, cancellation, and proper request tracing. This is mandatory for idiomatic Go.

---

### 2. Implement Circular Reference Detection

**Location**: `internal/config/loader.go`

**Required Implementation**: See [REVIEW.md](REVIEW.md#issue-24-circular-reference-detection-not-implemented) for complete implementation with:
- Depth-limited recursion (maxDepth = 10)
- Visited variable tracking
- Clear error messages on circular reference detection

**Rationale**: FR-013 requires this. Without it, circular references can cause infinite loops or panics. This is a security and stability requirement.

---

### 3. Fix ConfigError Design

**Location**: `contracts/config-port.go`, `internal/domain/config/errors.go`

**Required Changes**:
```go
type ConfigError struct {
    Field    string      // Changed from string to interface{}
    Value    interface{} // Support any value type
    Source   string
    Expected string
    Err      error       // Added for error wrapping
    // Removed: Message string (constructed in Error() method)
}

func (e *ConfigError) Error() string {
    // Construct message dynamically
}

func (e *ConfigError) Unwrap() error {
    return e.Err  // Enable error wrapping
}
```

**Rationale**: Go 1.13+ error wrapping requires `Unwrap()`. The `Message` field is redundant. This makes errors compatible with `errors.Is()` and `errors.As()`.

---

### 4. Fix Viper Instance Scoping

**Location**: `cmd/agentic-identity-broker/root.go`, `internal/config/loader.go`

**Required Changes**:
- Remove global Viper usage in `root.go` init()
- Create instance-scoped Viper in Loader
- Add `BindFlags()` method to Loader
- Pass flags to loader explicitly, not via global state

**Rationale**: Global Viper instance causes test conflicts in parallel execution. Each Loader needs its own Viper instance for proper isolation and testability.

---

## Recommended Improvements

While not blocking, these improvements should be implemented during development:

### 5. Use Custom Validation (Not go-playground/validator)

For initial scope (just logging config), implement custom validation:
- 10x faster than reflection-based validation
- Zero allocations for valid configuration
- Better error messages
- No external dependency

See [REVIEW.md](REVIEW.md#recommended-changes-should-fix) for implementation examples.

### 6. Implement Functional Options Pattern

```go
loader, err := config.NewLoader(
    config.WithConfigPath("/etc/config.yaml"),
    config.WithEnvironment("production"),
)
```

Provides flexibility without telescoping constructors.

### 7. Enhance Redaction Function

Expand beyond `IDENTITY_BROKER_` prefix to catch patterns like:
- "password", "secret", "token", "key", "credential"

### 8. Add Security Validations

- File permission checking (warn if world-readable)
- Command injection prevention in env var expansion
- Validate shell metacharacters in configuration values

---

## Testing Requirements

### Must Implement

1. **Table-Driven Tests** - For all validation logic
2. **Test Fixtures** - Create `tests/fixtures/config/` with valid/invalid/security test files
3. **Benchmark Tests** - Target: <10ms per configuration load
4. **Error Message Tests** - Validate exact error message format
5. **Integration Tests** - Test complete configuration loading flow

### Test Structure

```
tests/
├── fixtures/
│   └── config/
│       ├── valid/
│       ├── invalid/
│       └── security/
├── integration/
│   └── config/
└── unit/
    └── config/
```

---

## Performance Validation

**Realistic Startup Budget** (from golang-pro review):
- .env file loading: <10ms
- YAML parsing: 20-50ms
- Environment variable expansion: 5-20ms
- Viper Unmarshal: 50-100ms
- Custom validation: 10-50ms
- Audit logging: <10ms
- **Total**: 100-250ms (well under 5s budget)

Add timing logs to verify actual performance matches estimates.

---

## Implementation Checklist

Before merging, verify all items complete:

### Critical (Must Fix)
- [ ] Add `context.Context` to ConfigPort interface methods
- [ ] Implement circular reference detection with depth limit
- [ ] Fix ConfigError design (add Unwrap, remove Message)
- [ ] Ensure Viper instance is scoped, not global
- [ ] Add security validation (file permissions, injection prevention)

### Recommended (Should Fix)
- [ ] Use custom validation instead of go-playground/validator
- [ ] Implement functional options pattern for Loader
- [ ] Add comprehensive table-driven tests
- [ ] Create test fixtures directory structure
- [ ] Add benchmark tests for configuration loading
- [ ] Improve redaction to check for additional sensitive patterns

### Documentation
- [ ] Update all code examples with context parameters
- [ ] Document security considerations in comments
- [ ] Add package-level documentation
- [ ] Create ADR for library selection (20251215-120000-configuration-libraries.md)
- [ ] Update ARCHITECTURE.md with configuration subsystem
- [ ] Update docs/configuration.md for end users

---

## References

- **golang-pro Review**: [REVIEW.md](REVIEW.md) - Complete review with all recommendations
- **Feature Spec**: [spec.md](spec.md) - Original requirements
- **Research**: [research.md](research.md) - Library selection rationale
- **Data Model**: [data-model.md](data-model.md) - Domain entities
- **Quickstart**: [quickstart.md](quickstart.md) - Implementation guide (needs updates for critical fixes)
