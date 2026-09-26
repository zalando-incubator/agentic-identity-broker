# Quickstart: Flexible Application Configuration

**Feature**: 002-flexible-configuration
**Audience**: Developers implementing this feature
**Date**: 2025-12-14

## Overview

This guide helps you implement flexible configuration for the Agentic Identity Broker using Viper, Cobra, and godotenv libraries.

## Prerequisites

- Go 1.21 or higher installed
- Familiarity with Go modules
- Basic understanding of environment variables and YAML

## Installation

Add required dependencies to `go.mod`:

```bash
go get github.com/spf13/viper@v1.18.2
go get github.com/spf13/cobra@v1.8.0
go get github.com/joho/godotenv@v1.5.1
go get github.com/go-playground/validator/v10@v10.19.0
```

## Implementation Steps

### Step 1: Define Configuration Port (Interface)

Create `internal/ports/config.go`:

```go
package ports

import "time"

// ConfigPort defines how domain logic accesses configuration
type ConfigPort interface {
    GetConfig() (*Config, error)
    GetSources() []ConfigSource
}

// See contracts/config-port.go for complete type definitions
```

### Step 2: Create Domain Types

Create `internal/domain/config/types.go`:

```go
package config

type LogLevel string

const (
    LogLevelDebug LogLevel = "debug"
    LogLevelInfo  LogLevel = "info"
    LogLevelWarn  LogLevel = "warn"
    LogLevelError LogLevel = "error"
)

type LogFormat string

const (
    LogFormatText LogFormat = "text"
    LogFormatJSON LogFormat = "json"
)
```

### Step 3: Implement Configuration Adapter

Create `internal/config/loader.go`:

```go
package config

import (
    "fmt"
    "os"
    "strings"
    "time"

    "github.com/joho/godotenv"
    "github.com/spf13/viper"
    "github.com/spf13/cobra"
    "github.com/go-playground/validator/v10"
)

type Loader struct {
    viper     *viper.Viper
    validator *validator.Validate
    sources   []ConfigSource
}

func NewLoader(cmd *cobra.Command) *Loader {
    v := viper.New()
    return &Loader{
        viper:     v,
        validator: validator.New(),
        sources:   []ConfigSource{},
    }
}

func (l *Loader) GetConfig() (*Config, error) {
    // 1. Set defaults
    l.setDefaults()

    // 2. Load .env files
    if err := l.loadEnvFiles(); err != nil {
        return nil, err
    }

    // 3. Load YAML file
    if err := l.loadYAML(); err != nil {
        return nil, err
    }

    // 4. Expand environment variables
    if err := l.expandEnvVars(); err != nil {
        return nil, err
    }

    // 5. Bind CLI flags (already done in main)

    // 6. Unmarshal to struct
    var cfg Config
    if err := l.viper.Unmarshal(&cfg); err != nil {
        return nil, fmt.Errorf("failed to unmarshal config: %w", err)
    }

    // 7. Validate
    if err := l.validator.Struct(&cfg); err != nil {
        return nil, l.formatValidationError(err)
    }

    return &cfg, nil
}

func (l *Loader) setDefaults() {
    l.viper.SetDefault("log.level", "info")
    l.viper.SetDefault("log.format", "text")
    l.sources = append(l.sources, ConfigSource{
        Type:       SourceTypeDefault,
        Path:       "built-in",
        Precedence: 0,
        LoadedAt:   time.Now(),
        Keys:       []string{"log.level", "log.format"},
    })
}

func (l *Loader) loadEnvFiles() error {
    env := os.Getenv("GO_ENV")
    if env == "" {
        env = "development"
    }

    files := []string{
        ".env",
        ".env.local",
        fmt.Sprintf(".env.%s", env),
        fmt.Sprintf(".env.%s.local", env),
    }

    for _, file := range files {
        if err := godotenv.Load(file); err == nil {
            l.sources = append(l.sources, ConfigSource{
                Type:       SourceTypeEnvFile,
                Path:       file,
                Precedence: 1,
                LoadedAt:   time.Now(),
            })
        }
        // Ignore errors for missing files (they're optional)
    }

    return nil
}

func (l *Loader) loadYAML() error {
    configPath := l.viper.GetString("config")
    if configPath == "" {
        configPath = os.Getenv("IDENTITY_BROKER_CONFIG_PATH")
    }
    if configPath == "" {
        configPath = "config.yaml"
    }

    l.viper.SetConfigFile(configPath)
    if err := l.viper.ReadInConfig(); err != nil {
        if _, ok := err.(*os.PathError); !ok {
            return fmt.Errorf("failed to read config file: %w", err)
        }
        // Config file is optional
    } else {
        l.sources = append(l.sources, ConfigSource{
            Type:       SourceTypeYAML,
            Path:       configPath,
            Precedence: 2,
            LoadedAt:   time.Now(),
        })
    }

    return nil
}

func (l *Loader) expandEnvVars() error {
    // Simple expansion - production code needs circular detection
    for key, val := range l.viper.AllSettings() {
        if strVal, ok := val.(string); ok {
            if strings.Contains(strVal, "${") {
                expanded := os.ExpandEnv(strVal)
                l.viper.Set(key, expanded)
            }
        }
    }
    return nil
}

func (l *Loader) formatValidationError(err error) error {
    // Convert validator errors to ConfigError with helpful messages
    // See data-model.md for error message format
    return fmt.Errorf("configuration validation failed: %w", err)
}

func (l *Loader) GetSources() []ConfigSource {
    return l.sources
}
```

### Step 4: Setup CLI with Cobra

Create `cmd/agentic-identity-broker/root.go`:

```go
package main

import (
    "github.com/spf13/cobra"
    "github.com/spf13/viper"
)

var rootCmd = &cobra.Command{
    Use:   "agentic-identity-broker",
    Short: "Agentic Identity Broker",
    RunE:  run,
}

func init() {
    // Persistent flags (available to all commands)
    rootCmd.PersistentFlags().String("config", "", "config file path (default: config.yaml)")
    rootCmd.PersistentFlags().String("log-level", "", "log level (debug, info, warn, error)")
    rootCmd.PersistentFlags().String("log-format", "", "log format (text, json)")

    // Bind flags to Viper
    viper.BindPFlag("config", rootCmd.PersistentFlags().Lookup("config"))
    viper.BindPFlag("log.level", rootCmd.PersistentFlags().Lookup("log-level"))
    viper.BindPFlag("log.format", rootCmd.PersistentFlags().Lookup("log-format"))
}

func run(cmd *cobra.Command, args []string) error {
    // Load configuration
    loader := config.NewLoader(cmd)
    cfg, err := loader.GetConfig()
    if err != nil {
        return err
    }

    // Emit audit log (SR-004)
    emitAuditLog(loader.GetSources())

    // Display startup summary (FR-010)
    displayStartupSummary(cfg, loader.GetSources())

    // Start application with config...
    return nil
}
```

Create `cmd/agentic-identity-broker/main.go`:

```go
package main

import (
    "fmt"
    "os"
)

func main() {
    if err := rootCmd.Execute(); err != nil {
        fmt.Fprintf(os.Stderr, "Error: %v\n", err)
        os.Exit(1)
    }
}
```

### Step 5: Add Validation Logic

Create `internal/config/validator.go`:

```go
package config

import (
    "fmt"
    "github.com/go-playground/validator/v10"
)

func (l *Loader) formatValidationError(err error) error {
    validationErrs, ok := err.(validator.ValidationErrors)
    if !ok {
        return err
    }

    for _, fieldErr := range validationErrs {
        return &ConfigError{
            Field:    fieldErr.Field(),
            Value:    fmt.Sprintf("%v", fieldErr.Value()),
            Expected: getExpectedValue(fieldErr),
            Message:  formatErrorMessage(fieldErr),
        }
    }
    return err
}

func getExpectedValue(err validator.FieldError) string {
    switch err.Tag() {
    case "oneof":
        return err.Param()
    case "required":
        return "non-empty value"
    default:
        return err.Tag()
    }
}

func formatErrorMessage(err validator.FieldError) string {
    return fmt.Sprintf(
        "Configuration error: Invalid value '%v' for field '%s'.\nExpected: %s\nCheck: .env files, config.yaml, or use --%s flag",
        err.Value(),
        err.Field(),
        getExpectedValue(err),
        toFlagName(err.Field()),
    )
}
```

### Step 6: Add Redaction Utility

Create `internal/config/redactor.go`:

```go
package config

import "strings"

func Redact(key string, value interface{}) interface{} {
    if strings.HasPrefix(strings.ToUpper(key), "IDENTITY_BROKER_") {
        return "***REDACTED***"
    }
    return value
}
```

### Step 7: Write Tests

Create `tests/integration/config/loading_test.go`:

```go
package config_test

import (
    "os"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestConfigLoading_Precedence(t *testing.T) {
    // Test that CLI flags override YAML which overrides .env
    // See spec.md User Story 3 for full test scenarios
}

func TestConfigLoading_EnvVarSubstitution(t *testing.T) {
    // Test ${VAR} expansion
    // See spec.md User Story 2 for scenarios
}

func TestConfigValidation_InvalidValue(t *testing.T) {
    // Test that invalid log level triggers clear error
    // See data-model.md Error Scenarios
}
```

## Configuration Examples

### Example .env File

Create `.env.example`:

```bash
# Logging configuration
LOG_LEVEL=info
LOG_FORMAT=text

# Sensitive values (will be redacted in logs)
# IDENTITY_BROKER_API_KEY=your-api-key-here
```

### Example config.yaml

Create `examples/config/config.yaml.example`:

```yaml
log:
  level: info
  format: text

# Example of environment variable substitution
# apiKey: ${IDENTITY_BROKER_API_KEY}
```

## Running the Application

```bash
# With defaults only
./agentic-identity-broker

# With YAML config
./agentic-identity-broker --config /path/to/config.yaml

# Override log level via CLI flag
./agentic-identity-broker --log-level debug

# With environment-specific .env file
GO_ENV=production ./agentic-identity-broker
```

## Expected Output

Startup summary (FR-010):

```
Configuration Summary:
  log.level: debug [source: CLI]
  log.format: json [source: YAML]

Audit Log (JSON):
{"timestamp":"2025-12-14T10:30:00Z","level":"info","message":"configuration_loaded","sources":[".env","config.yaml","cli_flags"]}
```

## Troubleshooting

### Error: "Required field 'log.level' is missing"

**Solution**: Provide log level via .env file, YAML, or CLI flag:
```bash
./agentic-identity-broker --log-level info
```

### Error: "Environment variable 'IDENTITY_BROKER_API_KEY' not set"

**Solution**: Set the environment variable before running:
```bash
export IDENTITY_BROKER_API_KEY=your-key
./agentic-identity-broker
```

### Error: "Permission denied reading 'config.yaml'"

**Solution**: Fix file permissions:
```bash
chmod +r config.yaml
```

## Next Steps

1. Review [data-model.md](data-model.md) for complete type definitions
2. Review [contracts/config-port.go](contracts/config-port.go) for port interface
3. Implement remaining validation edge cases (circular references, injection detection)
4. Add comprehensive integration tests
5. Update [ARCHITECTURE.md](../../docs/ARCHITECTURE.md) with configuration subsystem
6. Create ADR for library selection (see research.md)
7. Update end-user documentation in [docs/configuration.md](../../../docs/configuration.md)

## References

- Feature Spec: [spec.md](spec.md)
- Research: [research.md](research.md)
- Data Model: [data-model.md](data-model.md)
- Implementation Plan: [plan.md](plan.md)
