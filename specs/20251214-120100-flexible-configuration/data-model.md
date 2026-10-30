# Data Model: Flexible Application Configuration

**Feature**: 20251214-120100-flexible-configuration
**Date**: 2025-12-14
**Status**: Complete

## Overview

This document defines the data model for the flexible configuration system, including domain entities, their relationships, validation rules, and state transitions.

## Domain Entities

### 1. Configuration Schema

**Description**: Defines all valid configuration options including their types, default values, validation rules, and sensitivity level.

**Fields**:
```go
type Config struct {
    Log    LogConfig    `mapstructure:"log" validate:"required"`
    // Future: Server, Database, Auth, etc.
}

type LogConfig struct {
    Level  LogLevel     `mapstructure:"level" validate:"required"`
    Format LogFormat    `mapstructure:"format" validate:"required"`
}
```

**Validation Rules**:
- `Log`: Required field, must be present in configuration
- `Log.Level`: Required, must be one of: debug, info, warn, error
- `Log.Format`: Required, must be one of: text, json

**Default Values**:
```go
var DefaultConfig = Config{
    Log: LogConfig{
        Level:  LogLevelInfo,
        Format: LogFormatText,
    },
}
```

**Sensitivity Classification**:
- `Log.*`: Non-sensitive (displayed in startup summary)
- Future sensitive fields (e.g., API keys, passwords): Prefixed with `IDENTITY_BROKER_*` pattern

**Relationships**:
- Has many `ConfigurationSource` (where values come from)
- Consumed by domain services via `ConfigPort` interface

### 2. Configuration Source

**Description**: Represents a source of configuration data with associated precedence level and loading mechanism.

**Fields**:
```go
type ConfigSource struct {
    Type       SourceType
    Path       string          // File path or "env" or "cli"
    Precedence int             // Lower number = lower precedence
    LoadedAt   time.Time
    Keys       []string        // Which config keys came from this source
}

type SourceType string

const (
    SourceTypeDefault   SourceType = "default"     // Precedence: 0
    SourceTypeEnvFile   SourceType = "env_file"    // Precedence: 1
    SourceTypeYAML      SourceType = "yaml"        // Precedence: 2
    SourceTypeCLI       SourceType = "cli"         // Precedence: 3
)
```

**Validation Rules**:
- `Type`: Must be one of the defined SourceType constants
- `Path`: Required for file-based sources (env_file, yaml); N/A for default/cli
- `Precedence`: Enforced by source type, immutable
- `LoadedAt`: Automatically set on load
- `Keys`: Automatically populated during loading

**Precedence Hierarchy** (FR-004):
1. Defaults (lowest)
2. .env files (.env → .env.local → .env.{env} → .env.{env}.local)
3. YAML file
4. CLI flags (highest)

**State Transitions**:
- Created → Loaded → Applied (to final configuration)
- Sources are immutable after loading (no hot-reload in initial scope)

**Relationships**:
- Contributes values to `ConfigurationSchema`
- Multiple sources can exist simultaneously, precedence determines final value

### 3. Environment Variable Reference

**Description**: Placeholder in configuration that references an environment variable for runtime substitution.

**Fields**:
```go
type EnvVarRef struct {
    OriginalValue string      // e.g., "${IDENTITY_BROKER_API_KEY}"
    VarName       string      // e.g., "IDENTITY_BROKER_API_KEY"
    ResolvedValue string      // Actual value from environment
    IsSensitive   bool        // true if var starts with IDENTITY_BROKER_
    ResolvedAt    time.Time
}
```

**Validation Rules** (FR-005, FR-006):
- `OriginalValue`: Must match pattern `${VARIABLE_NAME}` exactly
- `VarName`: Extracted from `${...}`, must be valid environment variable name (alphanumeric + underscore)
- `ResolvedValue`: Must exist in environment, error if undefined (FR-008)
- `IsSensitive`: Automatically determined by `IDENTITY_BROKER_` prefix
- Circular references: Detected and rejected (FR-013)

**Expansion Algorithm**:
```
1. Parse YAML value for ${...} pattern
2. Extract variable name
3. Check if already seen (circular detection)
4. Look up variable in environment
5. If not found → error with clear message (FR-008)
6. If found → replace ${...} with actual value
7. If value contains ${...} → recursively expand (with depth limit)
8. Mark as sensitive if IDENTITY_BROKER_ prefix
```

**Security Constraints** (SR-004):
- Only `${VAR}` pattern allowed, reject `$(command)` or backticks
- No shell interpretation, pure string substitution
- Maximum expansion depth: 10 levels (prevents infinite recursion)

**Relationships**:
- Found within `ConfigurationSource` (YAML file values)
- Resolved before validation of `ConfigurationSchema`

## Domain Value Objects

### LogLevel (Enum)

```go
type LogLevel string

const (
    LogLevelDebug LogLevel = "debug"
    LogLevelInfo  LogLevel = "info"
    LogLevelWarn  LogLevel = "warn"
    LogLevelError LogLevel = "error"
)

func (l LogLevel) Validate() error {
    switch l {
    case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
        return nil
    default:
        return fmt.Errorf("invalid log level: %s, must be one of [debug, info, warn, error]", l)
    }
}
```

### LogFormat (Enum)

```go
type LogFormat string

const (
    LogFormatText LogFormat = "text"
    LogFormatJSON LogFormat = "json"
)

func (f LogFormat) Validate() error {
    switch f {
    case LogFormatText, LogFormatJSON:
        return nil
    default:
        return fmt.Errorf("invalid log format: %s, must be one of [text, json]", f)
    }
}
```

## Configuration Loading Flow

```
┌─────────────────┐
│ Application     │
│ Startup         │
└────────┬────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 1. Load Defaults                                            │
│    Config{Log: {Level: "info", Format: "text"}}            │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 2. Load .env Files (godotenv)                               │
│    .env → .env.local → .env.production → .env.production.local │
│    Sources: Multiple ConfigSource entries with Precedence 1 │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 3. Load YAML File (Viper)                                   │
│    config.yaml (path from --config flag or                  │
│    IDENTITY_BROKER_CONFIG_PATH env var)                     │
│    Source: ConfigSource{Type: yaml, Precedence: 2}         │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 4. Expand Environment Variables                             │
│    Find ${VAR} patterns → create EnvVarRef → resolve        │
│    Detect circular references → error if found              │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 5. Apply CLI Flags (Cobra + Viper)                         │
│    --log-level, --log-format, etc.                          │
│    Source: ConfigSource{Type: cli, Precedence: 3}          │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 6. Unmarshal to Config Struct (Viper)                      │
│    viper.Unmarshal(&config)                                 │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 7. Validate Configuration (go-playground/validator)         │
│    validate.Struct(config)                                  │
│    If error → format message (FR-008) → terminate (FR-012)  │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 8. Emit Audit Log (SR-004)                                  │
│    JSON to stdout: sources, keys, redacted_keys            │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────────────────────────────────────────────────┐
│ 9. Display Startup Summary (FR-010)                         │
│    Show all keys, values (redacted if sensitive), sources  │
└────────┬────────────────────────────────────────────────────┘
         │
         ▼
┌─────────────────┐
│ Return Config   │
│ to Application  │
└─────────────────┘
```

## Error Scenarios

### 1. Missing Required Configuration

**Trigger**: No value provided for required field (e.g., log.level missing from all sources)

**Validation Rule**: `validate:"required"` tag fails

**Error Message** (FR-008):
```
Configuration error: Required field 'log.level' is missing.
Expected: one of [debug, info, warn, error]
Check: .env files, config.yaml, or use --log-level flag
```

**Action**: Terminate application (FR-012)

### 2. Invalid Configuration Value

**Trigger**: Value doesn't match enum constraints (e.g., log.level = "verbose")

**Validation Rule**: `validate:"oneof=debug info warn error"` tag fails

**Error Message** (FR-008):
```
Configuration error: Invalid value 'verbose' for field 'log.level'.
Expected: one of [debug, info, warn error]
Provided by: cli flag --log-level
```

**Action**: Terminate application (FR-012)

### 3. Undefined Environment Variable

**Trigger**: YAML contains `${IDENTITY_BROKER_API_KEY}` but variable not set in environment

**Validation Rule**: `os.LookupEnv()` returns false

**Error Message** (FR-008):
```
Configuration error: Environment variable 'IDENTITY_BROKER_API_KEY' referenced in config.yaml but not set.
Location: config.yaml, line 5, field 'apiKey'
Set the variable or remove the reference.
```

**Action**: Terminate application (FR-012)

### 4. Circular Environment Variable Reference

**Trigger**: `VAR1=${VAR2}` and `VAR2=${VAR1}` in environment

**Validation Rule**: Expansion depth exceeds 10 or variable seen twice in dependency chain (FR-013)

**Error Message** (FR-008):
```
Configuration error: Circular reference detected in environment variable expansion.
Chain: VAR1 → VAR2 → VAR1
Remove circular dependency.
```

**Action**: Terminate application (FR-012)

### 5. Malformed YAML File

**Trigger**: YAML syntax error (e.g., invalid indentation, unclosed quote)

**Validation Rule**: Viper's `ReadInConfig()` returns error

**Error Message** (FR-014):
```
Configuration error: Failed to parse YAML file '/path/to/config.yaml'.
Error: yaml: line 12: mapping values are not allowed in this context
Check file syntax and fix errors.
```

**Action**: Terminate application (FR-012)

### 6. File Permission Denied

**Trigger**: config.yaml exists but user lacks read permissions

**Validation Rule**: File open fails with EACCES (SR-003)

**Error Message** (FR-008):
```
Configuration error: Permission denied reading '/path/to/config.yaml'.
Check file permissions and ensure application has read access.
```

**Action**: Terminate application (FR-012) - fail securely (SR-003)

## Redaction Rules (SR-001, SR-002)

**Trigger**: Configuration key or environment variable starts with `IDENTITY_BROKER_` prefix

**Redaction Locations**:
1. Startup summary display (FR-010)
2. Audit log entries (SR-004)
3. Error messages (SR-003) - value not exposed, only key name

**Redaction Function**:
```go
func Redact(key string, value interface{}) interface{} {
    if strings.HasPrefix(strings.ToUpper(key), "IDENTITY_BROKER_") {
        return "***REDACTED***"
    }
    return value
}
```

**Example Redacted Output**:
```
Configuration Summary:
  log.level: debug [source: CLI]
  log.format: json [source: YAML]
  IDENTITY_BROKER_API_KEY: ***REDACTED*** [source: Environment]
```

## Future Extensions

**Planned Additions** (out of initial scope, see Assumption 3):
- Server configuration (host, port, TLS settings)
- Database configuration (connection string, pool settings)
- Authentication configuration (OAuth providers, JWT settings)

**Extensibility Points**:
- Add new fields to `Config` struct
- Add new `SourceType` constants if needed (e.g., remote config server)
- Add new validation rules via struct tags
- Add new sensitive prefixes beyond `IDENTITY_BROKER_` if needed

## References

- Feature Spec: [spec.md](spec.md)
- Research Decisions: [research.md](research.md)
- Implementation Plan: [plan.md](plan.md)
