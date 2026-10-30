# Configuration Contracts

This directory contains design contracts (interfaces) for the flexible configuration feature. These files define the API between domain logic and configuration infrastructure.

## Purpose

Contracts define the **port** in hexagonal architecture. Domain logic depends on these interfaces, not concrete implementations. This enables:

- **Testability**: Mock implementations for unit tests
- **Flexibility**: Swap implementations without changing domain code
- **Clear boundaries**: Explicit interface between domain and infrastructure

## Files

### config-port.go

Defines the `ConfigPort` interface that domain services use to access configuration.

**Key Types**:
- `ConfigPort`: Main interface for configuration access
- `Config`: Complete configuration schema
- `LogConfig`: Logging configuration subset
- `LogLevel`, `LogFormat`: Configuration enums
- `ConfigSource`: Metadata about configuration sources
- `ConfigError`: Structured configuration errors

**Usage in Domain Code**:
```go
// Domain service constructor takes ConfigPort, not concrete loader
func NewMyService(cfg ConfigPort) *MyService {
    config, err := cfg.GetConfig()
    if err != nil {
        log.Fatal(err)
    }
    // Use config...
}
```

**Implementation Location**: `internal/config/loader.go`
- Concrete adapter using Viper, Cobra, and godotenv
- Implements all ConfigPort methods
- Hidden from domain logic (internal package)

## Design Principles

### Hexagonal Architecture (Constitution Principle VI)

```
┌─────────────────────────────────────┐
│         Domain Logic                │
│  (business rules, services)         │
│                                     │
│  depends on ──► ConfigPort (port)  │
└──────────────┬──────────────────────┘
               │ interface
               │
┌──────────────▼──────────────────────┐
│   Configuration Adapter             │
│   (Viper + Cobra + godotenv)       │
│   implements ConfigPort             │
│                                     │
│   internal/config/loader.go         │
└─────────────────────────────────────┘
```

### Dependency Inversion

- **High-level policy** (domain logic) does NOT depend on **low-level details** (file parsing)
- Both depend on **abstractions** (ConfigPort interface)
- Adapter implements port, domain consumes port

### Interface Segregation

- `ConfigPort` is minimal - only methods domain actually needs
- Future extensions (e.g., configuration watching) added to interface only when domain requires them

## Validation Rules

All validation rules are documented in [../data-model.md](../data-model.md).

## Error Handling

Configuration errors follow fail-closed principle (Constitution Principle I, Assumption 6):
- Invalid configuration → application terminates (FR-012)
- Clear, actionable error messages (FR-008)
- No silent fallbacks or optional security

## Future Extensions

When adding new configuration categories (per Assumption 3):

1. **Add fields to `Config` struct**:
   ```go
   type Config struct {
       Log      LogConfig    `mapstructure:"log" validate:"required"`
       Server   ServerConfig `mapstructure:"server" validate:"required"` // NEW
       Database DBConfig     `mapstructure:"database" validate:"required"` // NEW
   }
   ```

2. **Update validation rules** in data-model.md

3. **Update adapter implementation** (internal/config/loader.go)

4. **No changes required** to domain code consuming ConfigPort (unless domain needs new config fields)

## References

- Implementation Plan: [../plan.md](../plan.md)
- Data Model: [../data-model.md](../data-model.md)
- Feature Spec: [../spec.md](../spec.md)
- Constitution: [../../../.specify/memory/constitution.md](../../../.specify/memory/constitution.md)
