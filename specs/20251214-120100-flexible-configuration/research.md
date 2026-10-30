# Research: Flexible Application Configuration

**Feature**: 20251214-120100-flexible-configuration
**Date**: 2025-12-14
**Status**: Complete

## Overview

This document captures research decisions for implementing flexible configuration management in the Agentic Identity Broker using Go libraries.

## Library Selection

### Decision: Viper for Configuration Management

**Chosen**: `github.com/spf13/viper` v1.18+

**Rationale**:
- Unified interface for multiple configuration sources (files, environment variables, flags, remote config)
- Native support for YAML, JSON, TOML, HCL configuration formats
- Automatic environment variable binding with prefix support
- Built-in configuration watching and live reloading capabilities (for future use)
- Widely adopted in Go ecosystem (50k+ GitHub stars, used by Kubernetes, Hugo, Docker)
- Active maintenance and community support
- Zero custom configuration parsing code required

**Alternatives Considered**:
1. **Standard library only** (`os`, `encoding/json`, `gopkg.in/yaml.v3`)
   - Rejected: Requires significant boilerplate for precedence handling, environment variable substitution, and validation
   - Would need ~500-1000 LOC of custom code vs ~100 LOC with Viper
2. **github.com/knadh/koanf**
   - Rejected: Less mature ecosystem, fewer integrations, similar feature set to Viper but smaller community
3. **github.com/kelseyhightower/envconfig**
   - Rejected: Only handles environment variables, doesn't support YAML/file-based config or CLI flags natively

### Decision: Cobra for CLI Framework

**Chosen**: `github.com/spf13/cobra` v1.8+

**Rationale**:
- De facto standard for Go CLI applications (used by kubectl, gh, hugo, docker)
- Seamless integration with Viper for flag precedence
- Automatic help generation and flag validation
- Supports persistent flags, sub-commands, and aliases
- POSIX-compliant flag parsing (including "last wins" behavior for duplicates)
- Extensive documentation and community examples

**Alternatives Considered**:
1. **Standard library `flag` package**
   - Rejected: Limited functionality, no sub-command support, manual precedence handling required
2. **github.com/urfave/cli**
   - Rejected: Less tight integration with Viper, different flag parsing semantics
3. **github.com/alecthomas/kong**
   - Rejected: Tag-based approach less suitable for dynamic configuration precedence requirements

### Decision: godotenv for .env File Loading

**Chosen**: `github.com/joho/godotenv` v1.5+

**Rationale**:
- Lightweight, focused library for .env file parsing
- Supports environment-specific file loading (.env.local, .env.production, etc.)
- Compatible with Viper's environment variable binding
- Used by 10k+ projects, well-tested
- Handles edge cases (empty files, comments, multi-line values, quoted strings)
- Follows dotenv standard from Ruby/Node.js ecosystems

**Alternatives Considered**:
1. **Viper's built-in .env support**
   - Rejected: Viper's `AutomaticEnv()` only reads existing environment variables, doesn't load .env files
2. **github.com/caarlos0/env**
   - Rejected: Focuses on struct tags for environment variable binding, not file loading
3. **Custom implementation**
   - Rejected: Unnecessary complexity when godotenv handles all edge cases (quoted values, escaping, comments)

## Configuration Precedence Strategy

### Decision: Four-Layer Precedence Hierarchy

**Chosen**: CLI Flags > YAML File > .env Files > Defaults

**Rationale**:
- Matches operator mental model: "most specific wins"
- CLI flags provide override capability for troubleshooting (highest precedence)
- YAML provides structured, version-controlled configuration (medium precedence)
- .env files provide environment-specific overrides without code changes (low precedence)
- Defaults ensure application always has valid configuration (lowest precedence)
- Aligns with 12-factor app methodology and cloud-native best practices

**Implementation with Viper**:
```go
// Order matters: later Set* calls override earlier ones
viper.SetDefault("log.level", "info")           // 1. Defaults (lowest)
loadEnvFiles()  // godotenv.Load()               // 2. .env files
viper.SetConfigFile("config.yaml")              //  3. YAML file
viper.ReadInConfig()
viper.BindPFlags(cmd.Flags())                   // 4. CLI flags (highest)
```

**Edge Case Handling**:
- Duplicate CLI flags: Cobra's default "last wins" behavior (FR-003)
- Duplicate YAML keys: YAML 1.2 spec "last wins" behavior (FR-015)
- Empty .env files: Treated as valid, godotenv continues silently
- Missing files: Non-fatal for optional sources (.env), fatal for required sources if needed

## Environment Variable Substitution

### Decision: Viper's Built-in ${VAR} Expansion

**Chosen**: Viper's `AutomaticEnv()` + `SetEnvPrefix()` + custom ${} expansion for YAML values

**Rationale**:
- Viper natively reads environment variables with prefix support
- Custom expansion required for YAML ${VAR} syntax (Viper doesn't auto-expand in file values)
- Uses `os.ExpandEnv()` or custom expander to detect circular references (FR-013)
- Injection-safe: expansion only for ${VARIABLE_NAME} pattern, not arbitrary shell execution

**Implementation Approach**:
```go
// After loading YAML
for key, val := range viper.AllSettings() {
    if strVal, ok := val.(string); ok {
        expanded := expandWithCircularCheck(strVal)
        viper.Set(key, expanded)
    }
}
```

**Security Considerations**:
- Only expand ${VAR} pattern, reject $(command) or backtick expressions (SR-004)
- Circular reference detection via dependency graph or max expansion depth
- Log warning (not value) if expansion fails for non-sensitive variable

## Configuration Validation

### Decision: Struct Tags + Custom Validator

**Chosen**: Go struct with validation tags + `github.com/go-playground/validator/v10`

**Rationale**:
- Declarative validation rules via struct tags (e.g., `validate:"oneof=debug info warn error"`)
- Single source of truth for schema and validation logic
- Clear error messages with field paths and constraint violations
- Extensible for custom validators (e.g., circular reference check)

**Schema Definition Example**:
```go
type Config struct {
    Log LogConfig `mapstructure:"log" validate:"required"`
}

type LogConfig struct {
    Level  string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
    Format string `mapstructure:"format" validate:"required,oneof=text json"`
}
```

**Validation Flow**:
1. Load configuration from all sources (Viper)
2. Unmarshal into typed struct (`viper.Unmarshal(&config)`)
3. Run validator (`validate.Struct(config)`)
4. If errors, format with helpful messages (FR-008) and terminate (FR-012)
5. If valid, emit audit log (SR-004) and return config

## Sensitive Value Redaction

### Decision: Prefix-Based Detection + Custom Redactor

**Chosen**: Custom `Redact()` function that replaces values for keys matching `IDENTITY_BROKER_*` pattern

**Rationale**:
- Simple pattern matching for sensitive detection (SR-001)
- Works across all configuration sources (no source-specific logic)
- Applied at display time, not storage (original values preserved in memory)

**Implementation**:
```go
func Redact(key string, value interface{}) interface{} {
    if strings.HasPrefix(strings.ToUpper(key), "IDENTITY_BROKER_") {
        return "***REDACTED***"
    }
    return value
}

// Usage in startup summary
for key, val := range viper.AllSettings() {
    fmt.Printf("%s: %v [source: %s]\n", key, Redact(key, val), getSource(key))
}
```

## Audit Logging Format

### Decision: Structured JSON to Stdout

**Chosen**: `encoding/json` + `log/slog` for structured logging to stdout

**Rationale**:
- JSON format enables easy parsing by log aggregation tools (Clarification Q3)
- Stdout follows 12-factor app principles (infrastructure handles routing)
- Go 1.21+ includes `log/slog` for structured logging out of the box
- No external logging library dependency required

**Log Entry Structure**:
```json
{
  "timestamp": "2025-12-14T10:30:00Z",
  "level": "info",
  "message": "configuration_loaded",
  "sources": [".env", ".env.production", "config.yaml", "cli_flags"],
  "config_keys": ["log.level", "log.format"],
  "redacted_keys": ["IDENTITY_BROKER_API_KEY"]
}
```

## File Size Handling

### Decision: No Explicit Limit (Memory-Bound)

**Chosen**: No file size validation, rely on OS memory limits (Clarification Q5)

**Rationale**:
- Configuration files typically <10 KB, explicit limits add unnecessary complexity
- OS will fail naturally if file exceeds available memory
- Error message from OS (ENOMEM) is clear enough for debugging
- Future optimization: stream parsing if size becomes issue (not required for initial scope)

## Best Practices Integration

### Viper Best Practices
1. **Single Viper instance**: Share one `*viper.Viper` across application (avoid global state)
2. **Explicit key paths**: Use dot notation (`viper.GetString("log.level")`) for nested config
3. **Type-safe access**: Unmarshal into structs, avoid `GetString()` everywhere
4. **Configuration watching**: Disabled for initial release (FR-007 validates once at startup)

### Cobra Best Practices
1. **Persistent flags**: Use for global options (e.g., `--config` file path)
2. **Flag binding**: Bind to Viper early (`viper.BindPFlag()`) for automatic precedence
3. **Help generation**: Leverage Cobra's auto-generated help for discoverability
4. **Command structure**: Single root command for identity broker, sub-commands for future expansion

### Error Handling
1. **Fail fast**: Terminate on configuration errors (FR-012, Assumption 6)
2. **Actionable messages**: Include field name, expected format, and all config sources (FR-008)
3. **No silent fallbacks**: Never default on security-related config (Principle I)

## Performance Considerations

### Startup Time Budget

**Target**: <5 seconds (SC-004)

**Breakdown**:
- .env file loading: <100ms (godotenv is fast, files typically <10 KB)
- YAML parsing: <200ms (Viper uses optimized YAML parser)
- Environment variable expansion: <100ms (simple string replacement)
- Validation: <100ms (struct tag validation is fast)
- Audit logging: <50ms (single JSON write to stdout)
- **Total**: ~550ms configuration overhead (well under 5s budget)

### Memory Footprint

**Estimate**: <5 MB for configuration subsystem
- Viper instance: ~1 MB (holds all config in memory maps)
- godotenv: <100 KB (minimal overhead)
- Cobra: ~500 KB (command tree and flags)
- Configuration struct: <1 KB (only logging config initially)

## Testing Strategy

### Unit Tests
- Configuration validation (invalid values, missing required fields)
- Redaction logic (prefix matching, case sensitivity)
- Environment variable expansion (circular detection, injection patterns)

### Integration Tests
- .env file precedence (.env → .env.local → .env.production → .env.production.local)
- YAML + CLI flag precedence
- End-to-end configuration loading with all sources
- Error message format validation

### Test Data
- Example .env files in `tests/fixtures/config/`
- Example YAML files with valid/invalid schemas
- Mock environment variable sets for substitution tests

## Open Questions (Resolved)

None. All technical decisions finalized.

## References

- [Viper Documentation](https://github.com/spf13/viper)
- [Cobra User Guide](https://github.com/spf13/cobra/blob/main/user_guide.md)
- [godotenv README](https://github.com/joho/godotenv)
- [12-Factor App Config](https://12factor.net/config)
- [Go slog Package](https://pkg.go.dev/log/slog)
