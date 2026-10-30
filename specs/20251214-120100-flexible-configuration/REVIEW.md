# Go Implementation Review Summary

**Date**: 2025-12-14
**Reviewer**: golang-pro agent
**Overall Grade**: B+ → A- (with recommended changes)

## Executive Summary

The implementation plan is solid with appropriate library choices (Viper, Cobra, godotenv) and good architectural separation. However, several Go-specific issues must be addressed before implementation:

### Critical Issues (Must Fix)
1. **Missing Context in Interface** - ConfigPort methods need `context.Context` parameter
2. **Circular Reference Detection Not Implemented** - Required by FR-013, security risk
3. **ConfigError Design Issues** - Missing `Unwrap()`, redundant `Message` field
4. **Viper Instance Scoping** - Global state affects testability

### Risk Level
**LOW** (after addressing critical issues)

---

## Critical Changes Required

### 1. Add Context to ConfigPort Interface

**File**: `contracts/config-port.go`

**Change**:
```go
// BEFORE
type ConfigPort interface {
    GetConfig() (*Config, error)
    GetSources() []ConfigSource
    Reload() error
}

// AFTER
type ConfigPort interface {
    GetConfig(ctx context.Context) (*Config, error)
    GetSources() []ConfigSource
    Reload(ctx context.Context) error
}
```

**Rationale**: Configuration loading involves I/O operations. Without context, you cannot implement timeouts, cancellation, or proper request tracing. This is non-negotiable for idiomatic Go.

---

### 2. Implement Circular Reference Detection

**File**: `internal/config/loader.go` (expandEnvVars method)

**Change**:
```go
// expandEnvVars expands ${VAR} references with circular detection.
func (l *Loader) expandEnvVars() error {
    visited := make(map[string]bool)

    for key, val := range l.viper.AllSettings() {
        if strVal, ok := val.(string); ok {
            expanded, err := l.expandValue(strVal, visited, 0)
            if err != nil {
                return fmt.Errorf("expanding %q: %w", key, err)
            }
            l.viper.Set(key, expanded)
        }
    }
    return nil
}

// expandValue recursively expands ${VAR} with depth limit and cycle detection.
func (l *Loader) expandValue(value string, visited map[string]bool, depth int) (string, error) {
    const maxDepth = 10

    if depth > maxDepth {
        return "", fmt.Errorf("maximum expansion depth exceeded (possible circular reference)")
    }

    // Find all ${VAR} patterns
    re := regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)
    matches := re.FindAllStringSubmatch(value, -1)

    if len(matches) == 0 {
        return value, nil
    }

    result := value
    for _, match := range matches {
        varName := match[1]

        // Check for circular reference
        if visited[varName] {
            return "", fmt.Errorf("circular reference detected: %s", varName)
        }

        visited[varName] = true
        defer func() { delete(visited, varName) }()

        varValue, exists := os.LookupEnv(varName)
        if !exists {
            return "", fmt.Errorf("environment variable %q not set", varName)
        }

        expandedValue, err := l.expandValue(varValue, visited, depth+1)
        if err != nil {
            return "", fmt.Errorf("expanding %s: %w", varName, err)
        }

        result = strings.ReplaceAll(result, match[0], expandedValue)
    }

    return result, nil
}
```

**Rationale**: FR-013 requires circular reference detection. Without this, the application could panic or hang during startup. This is a security and stability issue.

---

### 3. Fix ConfigError Design

**File**: `contracts/config-port.go`

**Change**:
```go
// BEFORE
type ConfigError struct {
    Field    string
    Value    string
    Source   string
    Expected string
    Message  string  // Redundant
}

func (e *ConfigError) Error() string {
    return e.Message
}

// AFTER
type ConfigError struct {
    Field    string      // e.g., "log.level"
    Value    interface{} // Invalid value (type-flexible)
    Source   string      // e.g., "cli flag --log-level"
    Expected string      // e.g., "one of [debug, info, warn, error]"
    Err      error       // Underlying error (for wrapping)
}

func (e *ConfigError) Error() string {
    var b strings.Builder
    b.WriteString("configuration error: ")

    if e.Field != "" {
        fmt.Fprintf(&b, "invalid value %q for field %q", e.Value, e.Field)
    }

    if e.Expected != "" {
        fmt.Fprintf(&b, " (expected: %s)", e.Expected)
    }

    if e.Source != "" {
        fmt.Fprintf(&b, " [source: %s]", e.Source)
    }

    if e.Err != nil {
        fmt.Fprintf(&b, ": %v", e.Err)
    }

    return b.String()
}

func (e *ConfigError) Unwrap() error {
    return e.Err
}
```

**Rationale**: Go 1.13+ error wrapping requires `Unwrap()` method. The `Message` field is redundant since `Error()` should construct it. This makes errors compatible with `errors.Is()` and `errors.As()`.

---

### 4. Fix Viper Instance Scoping

**File**: `cmd/agentic-identity-broker/root.go`

**Change**:
```go
// BEFORE (uses global viper)
func init() {
    viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
    viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
}

// AFTER (use loader's viper instance)
func run(cmd *cobra.Command, args []string) error {
    loader := config.NewLoader()

    // Bind flags to loader's viper instance
    loader.BindFlags(cmd.Flags())

    cfg, err := loader.GetConfig(cmd.Context())
    if err != nil {
        return err
    }
    // ...
}

// Add to Loader
func (l *Loader) BindFlags(flags *pflag.FlagSet) {
    l.v.BindPFlag("config", flags.Lookup("config"))
    l.v.BindPFlag("log.level", flags.Lookup("log-level"))
    l.v.BindPFlag("log.format", flags.Lookup("log-format"))
}
```

**Rationale**: Global Viper instance causes test conflicts when tests run in parallel. Each Loader should have its own Viper instance for isolation.

---

## Recommended Changes (Should Fix)

### 5. Replace go-playground/validator with Custom Validation

**Why**: For the initial scope (just logging config), custom validation is:
- 10x faster than reflection-based validation
- Zero allocations for valid configuration
- Simpler code with better error messages
- No external dependency

**Implementation**:
```go
// internal/domain/config/types.go
func (l LogLevel) Validate() error {
    switch l {
    case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
        return nil
    default:
        return &ConfigError{
            Field:    "log.level",
            Value:    string(l),
            Expected: "one of [debug, info, warn, error]",
        }
    }
}

func (c *Config) Validate() error {
    if err := c.Log.Validate(); err != nil {
        return fmt.Errorf("log configuration: %w", err)
    }
    return nil
}

func (l *LogConfig) Validate() error {
    if err := l.Level.Validate(); err != nil {
        return fmt.Errorf("log.level: %w", err)
    }
    if err := l.Format.Validate(); err != nil {
        return fmt.Errorf("log.format: %w", err)
    }
    return nil
}
```

**When to reconsider**: If schema grows to 50+ fields with complex constraints, then go-playground/validator becomes worth the overhead.

---

### 6. Add Functional Options Pattern

**Implementation**:
```go
type LoaderOption func(*Loader) error

func WithConfigPath(path string) LoaderOption {
    return func(l *Loader) error {
        l.configPath = path
        return nil
    }
}

func WithEnvironment(env string) LoaderOption {
    return func(l *Loader) error {
        l.environment = env
        return nil
    }
}

func NewLoader(opts ...LoaderOption) (*Loader, error) {
    l := &Loader{
        v:           viper.New(),
        environment: "development",
        defaults:    DefaultConfig,
        sources:     make([]ConfigSource, 0, 4),
    }

    for _, opt := range opts {
        if err := opt(l); err != nil {
            return nil, fmt.Errorf("applying option: %w", err)
        }
    }

    return l, nil
}
```

**Benefits**: Flexible, extensible, testable construction without telescoping constructors.

---

### 7. Improve Redaction Function

**Enhancement**:
```go
func Redact(key string, value interface{}) interface{} {
    keyLower := strings.ToLower(key)

    sensitivePatterns := []string{
        "password", "secret", "token", "key", "credential",
        "identity_broker_",
    }

    for _, pattern := range sensitivePatterns {
        if strings.Contains(keyLower, pattern) {
            return "***REDACTED***"
        }
    }

    return value
}

func IsSensitive(key string) bool {
    return Redact(key, "") == "***REDACTED***"
}
```

**Rationale**: Catches more sensitive patterns beyond just `IDENTITY_BROKER_` prefix.

---

## Go Idioms & Best Practices

### Interface Naming
- **Current**: `ConfigPort`
- **Better**: `ConfigProvider` (follows Go "-er" convention) or `ConfigurationPort` (full word)

### Method Naming
- Remove "Get" prefix: `Config()` instead of `GetConfig()`
- Exception: When differentiating from setter (`GetX()` vs `SetX()`)

### Testing
- Use table-driven tests extensively
- Prefer standard library testing over testify/assert
- Use `testify/require` only for test setup code

**Example**:
```go
func TestLogLevel_Validate(t *testing.T) {
    tests := []struct {
        name    string
        level   LogLevel
        wantErr bool
    }{
        {"valid debug", LogLevelDebug, false},
        {"valid info", LogLevelInfo, false},
        {"invalid verbose", LogLevel("verbose"), true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := tt.level.Validate()
            if (err != nil) != tt.wantErr {
                t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

---

## Security Enhancements

### Validate File Permissions
```go
func (l *Loader) loadYAML(ctx context.Context) error {
    info, err := os.Stat(configPath)
    if err != nil {
        return fmt.Errorf("stat config file: %w", err)
    }

    // Warn if world-readable
    if info.Mode().Perm()&0044 != 0 {
        slog.WarnContext(ctx, "config file has broad permissions",
            "path", configPath,
            "recommendation", "chmod 600")
    }

    // ... continue loading
}
```

### Prevent Command Injection
```go
func rejectInjection(value string) error {
    dangerous := []string{"$(", "`", ";", "|", "&", ">", "<"}

    for _, pattern := range dangerous {
        if strings.Contains(value, pattern) {
            return fmt.Errorf("potential command injection: %q", pattern)
        }
    }

    return nil
}
```

---

## Testing Strategy

### Test Fixtures Structure
```
tests/
└── fixtures/
    └── config/
        ├── valid/
        │   ├── config.yaml
        │   ├── .env
        │   └── .env.production
        ├── invalid/
        │   ├── malformed.yaml
        │   ├── missing-required.yaml
        │   └── circular-ref.yaml
        └── security/
            ├── world-readable.yaml
            └── injection-attempt.yaml
```

### Benchmark Configuration Loading
```go
func BenchmarkLoader_GetConfig(b *testing.B) {
    loader := setupTestLoader(b)
    ctx := context.Background()

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, err := loader.GetConfig(ctx)
        if err != nil {
            b.Fatal(err)
        }
    }
}
```

**Target**: <10ms per load for simple schema

### Test Error Messages
```go
func TestConfigError_Messages(t *testing.T) {
    tests := []struct {
        name    string
        err     *ConfigError
        wantMsg string
    }{
        {
            name: "missing required field",
            err: &ConfigError{
                Field:    "log.level",
                Expected: "one of [debug, info, warn, error]",
            },
            wantMsg: `configuration error: invalid value "" for field "log.level" (expected: one of [debug, info, warn, error])`,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            if got := tt.err.Error(); got != tt.wantMsg {
                t.Errorf("Error() = %q, want %q", got, tt.wantMsg)
            }
        })
    }
}
```

---

## Performance Validation

### Realistic Startup Budget
- `.env` file loading: <10ms
- YAML parsing: 20-50ms
- Environment variable expansion: 5-20ms
- Viper Unmarshal: 50-100ms (reflection)
- Custom validation: 10-50ms
- Audit logging: <10ms
- **Total**: 100-250ms (well under 5s budget)

### Add Timing Logs
```go
func (l *Loader) GetConfig(ctx context.Context) (*Config, error) {
    start := time.Now()
    defer func() {
        slog.InfoContext(ctx, "configuration loading complete",
            "duration_ms", time.Since(start).Milliseconds())
    }()

    // Log each step duration at debug level
}
```

---

## Implementation Checklist

### Must Do (Before Implementation)
- [ ] Add `context.Context` to ConfigPort methods
- [ ] Implement circular reference detection
- [ ] Fix ConfigError design (add Unwrap, remove Message)
- [ ] Ensure Viper instance is scoped, not global
- [ ] Add security validation (permissions, injection)

### Should Do (During Implementation)
- [ ] Use custom validation (not go-playground/validator)
- [ ] Implement functional options pattern
- [ ] Add comprehensive table-driven tests
- [ ] Create test fixtures directory
- [ ] Add benchmark tests
- [ ] Improve redaction patterns

### Nice to Have
- [ ] Add startup time tracking
- [ ] Create configuration diff capability
- [ ] Add golden file tests
- [ ] Add package examples for godoc
- [ ] Use build tags for test separation

---

## Files Requiring Updates

1. **contracts/config-port.go**
   - Add context.Context parameters
   - Fix ConfigError struct
   - Rename to ConfigProvider (optional)

2. **quickstart.md**
   - Update all code examples with context
   - Fix Viper instance scoping examples
   - Add circular reference detection implementation

3. **research.md**
   - Update validation approach section
   - Correct Viper precedence comments
   - Add security considerations

4. **data-model.md**
   - Remove unused EnvVarRef.ResolvedAt field
   - Document maxDepth constant
   - Add performance characteristics

5. **plan.md**
   - Note the critical changes required
   - Link to this review document

---

## Final Verdict

**Time to Implement**: 3-5 days (including tests and documentation)
**Risk Level**: LOW (after addressing critical issues)
**Code Quality**: HIGH (with recommended changes)

The implementation plan is solid. Fix the 4 critical issues before starting development, and implement the recommended improvements during development for a production-ready, idiomatic Go configuration system.
