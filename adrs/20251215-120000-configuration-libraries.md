# ADR 20251215-120000: Configuration Library Selection

**Status**: Accepted
**Date**: 2025-12-15
**Technical Story**: Feature 20251214-120100-flexible-configuration

## Context

The Agentic Identity Broker requires a flexible configuration system that supports multiple sources (.env files, YAML files, command-line flags) with clear precedence rules, environment variable substitution, and security-first design. We needed to select appropriate Go libraries for configuration management, CLI parsing, and .env file support.

## Decision

We will use the following libraries for configuration management:

1. **Viper v1.19.0+** (github.com/spf13/viper) for unified configuration management
2. **Cobra v1.8.1+** (github.com/spf13/cobra) for CLI command and flag parsing
3. **godotenv v1.5.1+** (github.com/joho/godotenv) for .env file loading

## Rationale

### Viper Selection

**Alternatives Considered:**
- envconfig (only supports env vars, no YAML/CLI)
- cleanenv (limited precedence control)
- go-ini (only INI format)
- Custom implementation (high development cost)

**Why Viper:**
- Native support for multiple configuration sources (YAML, JSON, env vars, flags)
- Built-in precedence hierarchy (can be customized)
- Widely adopted in Go ecosystem (30k+ GitHub stars)
- Active maintenance and security updates
- Integration with Cobra for CLI flags
- Automatic unmarshaling to structs
- Environment variable support
- Watch/reload capabilities (for future use)

**Trade-offs:**
- Larger dependency footprint compared to single-purpose libraries
- Global state patterns (mitigated by using instance-scoped Viper)
- Reflection-based unmarshaling (acceptable performance for startup)

### Cobra Selection

**Alternatives Considered:**
- flag (stdlib, but limited features)
- kingpin (less maintained)
- urfave/cli (different API style)
- cobra (chosen)

**Why Cobra:**
- Industry standard for Go CLIs (used by kubectl, hugo, gh)
- Powerful subcommand support (extensible for future features)
- Automatic help generation
- Native Viper integration
- Persistent and local flags
- Active development
- Strong community support

**Trade-offs:**
- More features than needed initially (acceptable for future growth)
- Additional dependency

### godotenv Selection

**Alternatives Considered:**
- joho/godotenv (chosen)
- Viper's built-in env support (lacks .env file loading)
- Custom .env parser (reinventing the wheel)

**Why godotenv:**
- Simple, focused API for .env file parsing
- Support for multiple .env files
- Widely adopted (8k+ GitHub stars)
- Minimal dependencies
- Well-tested and stable
- Compatible with Viper workflow

**Trade-offs:**
- Additional dependency (but lightweight)
- Requires integration with Viper manually

### Custom Validation over go-playground/validator

**Decision:** Use custom validation functions instead of go-playground/validator

**Rationale:**
- **Performance**: 10x faster than reflection-based validation
- **Zero allocations**: Custom validators don't allocate for valid input
- **Clarity**: Explicit validation logic is easier to understand and debug
- **Error messages**: Better control over error formatting
- **Scope**: Initial feature only validates 2 fields (log.level, log.format)
- **Maintainability**: Less external dependencies

**Trade-offs:**
- More code to write/maintain (but minimal for current scope)
- Less feature-rich than go-playground/validator
- Will reconsider if validation complexity grows significantly

## Consequences

### Positive

- **Unified configuration management**: Single source of truth (Viper)
- **CLI-first design**: Professional command-line interface (Cobra)
- **Environment flexibility**: Support for .env files in multiple environments
- **Extensibility**: Easy to add new configuration sources in future
- **Performance**: Fast startup (<250ms) with custom validation
- **Developer experience**: Well-documented libraries with examples
- **Security**: Active maintenance and vulnerability patching

### Negative

- **Dependency count**: Three additional direct dependencies (Viper, Cobra, godotenv)
- **Binary size**: Slightly larger binary (~2MB increase)
- **Learning curve**: Team needs to understand Viper/Cobra patterns
- **Global state**: Viper has global state patterns (mitigated with instance scoping)

### Risks and Mitigation

**Risk**: Viper global state causes test conflicts
**Mitigation**: Use instance-scoped Viper via NewLoader(), not global viper.Get()

**Risk**: Breaking changes in library updates
**Mitigation**: Pin versions in go.mod, test updates before upgrading

**Risk**: Security vulnerabilities in dependencies
**Mitigation**: Regular dependency audits with `go list -m all | nancy sleuth`, Dependabot alerts enabled

## Implementation Notes

- Viper instance is scoped to Loader struct (no global state)
- Cobra command is passed explicitly to Loader.SetCommand()
- godotenv loads .env files, then values are set in Viper
- Custom validators in internal/config/validator.go
- Error wrapping uses Go 1.13+ patterns (Unwrap() method)

## References

- [Viper Documentation](https://github.com/spf13/viper)
- [Cobra Documentation](https://github.com/spf13/cobra)
- [godotenv Documentation](https://github.com/joho/godotenv)
- Feature Specification: specs/20251214-120100-flexible-configuration/spec.md
- Implementation Plan: specs/20251214-120100-flexible-configuration/plan.md
- golang-pro Review: specs/20251214-120100-flexible-configuration/REVIEW.md
