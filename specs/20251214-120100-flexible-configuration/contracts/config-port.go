// Package contracts defines the configuration port interface
// This is a design contract, not executable code - for planning purposes only
package contracts

import (
	"context"
	"fmt"
	"time"
)

// ConfigPort defines the interface for accessing application configuration.
// This is a hexagonal architecture port - domain logic depends on this interface,
// not concrete implementations.
//
// Implementation: internal/config/loader.go (adapter using Viper/Cobra/godotenv)
type ConfigPort interface {
	// GetConfig returns the fully loaded and validated configuration.
	// ctx allows timeout and cancellation control during loading.
	// Returns error if configuration is invalid or cannot be loaded.
	// Called once at application startup (FR-007).
	GetConfig(ctx context.Context) (*Config, error)

	// GetSources returns metadata about all configuration sources used.
	// Used for audit logging (SR-004) and startup summary (FR-010).
	// This is a pure getter, no I/O, so context not needed.
	GetSources() []ConfigSource

	// Reload reloads configuration from all sources.
	// ctx allows timeout and cancellation control during reload.
	// NOT IMPLEMENTED in initial scope (Assumption 7: no hot-reloading).
	// Included for future extensibility.
	Reload(ctx context.Context) error
}

// Config represents the complete application configuration schema.
// See data-model.md for detailed field descriptions and validation rules.
type Config struct {
	Log LogConfig `mapstructure:"log" validate:"required"`
	// Future: Server, Database, Auth, etc. (per Assumption 3)
}

// LogConfig contains logging-related configuration.
type LogConfig struct {
	Level  LogLevel  `mapstructure:"level" validate:"required"`
	Format LogFormat `mapstructure:"format" validate:"required"`
}

// LogLevel is an enumeration of valid log levels (FR-011).
type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

// LogFormat is an enumeration of valid log output formats (FR-011).
type LogFormat string

const (
	LogFormatText LogFormat = "text"
	LogFormatJSON LogFormat = "json"
)

// ConfigSource represents metadata about a configuration source (FR-010).
type ConfigSource struct {
	Type       SourceType // Type of configuration source
	Path       string     // File path (for file-based sources) or "env" / "cli"
	Precedence int        // Precedence level (higher = higher priority, per FR-004)
	LoadedAt   time.Time  // Timestamp when source was loaded
	Keys       []string   // Configuration keys provided by this source
}

// SourceType enumerates the types of configuration sources.
type SourceType string

const (
	SourceTypeDefault SourceType = "default"  // Built-in defaults (Precedence: 0)
	SourceTypeEnvFile SourceType = "env_file" // .env files (Precedence: 1)
	SourceTypeYAML    SourceType = "yaml"     // YAML file (Precedence: 2)
	SourceTypeCLI     SourceType = "cli"      // Command-line flags (Precedence: 3)
)

// ConfigError represents a configuration validation or loading error (FR-008).
// Compatible with Go 1.13+ error wrapping via Unwrap() method.
type ConfigError struct {
	Field    string      // Configuration field with error (e.g., "log.level")
	Value    interface{} // Invalid value provided (type-flexible, redacted if sensitive per SR-003)
	Source   string      // Where the value came from (e.g., "cli flag --log-level")
	Expected string      // Expected format or valid values
	Err      error       // Underlying error (for wrapping)
}

// Error constructs the error message dynamically.
func (e *ConfigError) Error() string {
	var msg string
	if e.Field != "" {
		msg = fmt.Sprintf("configuration error: invalid value %q for field %q", e.Value, e.Field)
	} else {
		msg = "configuration error"
	}

	if e.Expected != "" {
		msg += fmt.Sprintf(" (expected: %s)", e.Expected)
	}

	if e.Source != "" {
		msg += fmt.Sprintf(" [source: %s]", e.Source)
	}

	if e.Err != nil {
		msg += fmt.Sprintf(": %v", e.Err)
	}

	return msg
}

// Unwrap returns the underlying error for error wrapping compatibility.
func (e *ConfigError) Unwrap() error {
	return e.Err
}

// DefaultConfig returns the default configuration values (FR-009).
var DefaultConfig = Config{
	Log: LogConfig{
		Level:  LogLevelInfo,
		Format: LogFormatText,
	},
}
