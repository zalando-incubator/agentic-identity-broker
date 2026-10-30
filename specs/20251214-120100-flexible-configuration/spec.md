# Feature Specification: Flexible Application Configuration

**Feature Branch**: `20251214-120100-flexible-configuration`
**Created**: 2025-12-14
**Status**: Draft
**Input**: User description: "Enable flexible configuration of the Agentic Identity Broker application through multiple sources (.env files, YAML file, and command-line flags), with support for environment variable substitution for sensitive data. This allows different deployment scenarios (development, staging, production) without code changes."

## Clarifications

### Session 2025-12-14

- Q: When a command-line flag is provided twice with different values, how should the system behave? → A: Last value wins (override with most recent)
- Q: When a YAML file contains duplicate keys, how should the system behave? → A: Last key wins (YAML spec compliant)
- Q: What format and destination should audit logs use when configuration is loaded? → A: Structured JSON to stdout
- Q: How should the system handle .env files that exist but are empty or contain only comments? → A: Treat as valid, continue loading
- Q: What is the maximum size limit for configuration files? → A: No explicit limit (memory-bound)

## User Scenarios & Testing *(mandatory)*

<!--
  IMPORTANT: User stories should be PRIORITIZED as user journeys ordered by importance.
  Each user story/journey must be INDEPENDENTLY TESTABLE - meaning if you implement just ONE of them,
  you should still have a viable MVP (Minimum Viable Product) that delivers value.

  Assign priorities (P1, P2, P3, etc.) to each story, where P1 is the most critical.
  Think of each story as a standalone slice of functionality that can be:
  - Developed independently
  - Tested independently
  - Deployed independently
  - Demonstrated to users independently
-->

### User Story 1 - Environment-Specific Configuration Loading (Priority: P1)

As a DevOps engineer deploying the Agentic Identity Broker, I need the application to automatically load configuration appropriate for each environment (development, staging, production) so that I can deploy the same codebase across environments without manual configuration changes.

**Why this priority**: This is foundational for any multi-environment deployment. Without it, operators cannot safely deploy the application to different environments, making it critical for production readiness.

**Independent Test**: Can be fully tested by placing environment-specific .env files in the application directory and starting the application with different NODE_ENV values (or equivalent). The application should display configuration summary at startup showing which files were loaded and in what order, with sensitive values redacted.

**Acceptance Scenarios**:

1. **Given** the application directory contains .env, .env.local, .env.production, and .env.production.local files, **When** the application starts with environment set to "production", **Then** all four files are loaded in order (.env → .env.local → .env.production → .env.production.local) with later values overriding earlier ones
2. **Given** a configuration value exists in both .env and .env.production.local, **When** the application starts in production mode, **Then** the value from .env.production.local is used
3. **Given** no environment-specific files exist, **When** the application starts, **Then** it loads the base .env file and continues successfully with defaults
4. **Given** a required configuration value is missing from all .env files, **When** the application starts, **Then** it displays a clear error message identifying the missing field and terminates gracefully

---

### User Story 2 - YAML Configuration with Environment Variable Substitution (Priority: P1)

As a security-conscious operator, I need to define application configuration in a YAML file while referencing sensitive values through environment variables, so that secrets never appear in version-controlled configuration files.

**Why this priority**: Security best practices require separating secrets from configuration. This enables secure configuration management and is essential before production deployment.

**Independent Test**: Can be fully tested by creating a config.yaml file containing environment variable references (e.g., `${IDENTITY_BROKER_API_KEY}`), setting those environment variables, and starting the application. The application should substitute variables at startup, display a configuration summary with sensitive values redacted, and fail clearly if referenced variables are undefined.

**Acceptance Scenarios**:

1. **Given** a config.yaml file contains `apiKey: ${IDENTITY_BROKER_API_KEY}` and the environment variable IDENTITY_BROKER_API_KEY is set, **When** the application starts, **Then** the API key value is substituted from the environment variable
2. **Given** a config.yaml references ${IDENTITY_BROKER_DATABASE_PASSWORD}, **When** that environment variable is not set, **Then** the application displays a clear error message identifying the missing variable and terminates
3. **Given** a config.yaml contains both sensitive values (prefixed with IDENTITY_BROKER_) and non-sensitive values, **When** the application displays its startup configuration summary, **Then** sensitive values are shown as "***REDACTED***" while non-sensitive values are displayed
4. **Given** the config.yaml file path is specified via command-line flag or IDENTITY_BROKER_CONFIG_PATH environment variable, **When** the application starts, **Then** it loads configuration from the specified path

---

### User Story 3 - Command-Line Flag Override (Priority: P2)

As an operator troubleshooting an issue, I need to override specific configuration values using command-line flags without modifying files, so that I can quickly test configuration changes or run the application with temporary settings.

**Why this priority**: Command-line overrides enable rapid iteration during troubleshooting and testing. While important for operational flexibility, it's less critical than core configuration loading.

**Independent Test**: Can be fully tested by starting the application with command-line flags (e.g., `--log-level=debug --config=/path/to/config.yaml`) and verifying that these values override corresponding settings from .env files and YAML configuration. The startup summary should clearly show which values came from command-line flags.

**Acceptance Scenarios**:

1. **Given** a config.yaml file sets logLevel to "info" and a command-line flag specifies --log-level=debug, **When** the application starts, **Then** the log level is set to "debug"
2. **Given** both .env and config.yaml define a logging format, **When** the application starts with --log-format=json flag, **Then** the command-line value takes precedence over both file sources
3. **Given** no configuration files exist, **When** the application starts with all required values provided via command-line flags, **Then** the application starts successfully using only flag values
4. **Given** an invalid value is provided via command-line flag, **When** the application starts, **Then** it displays a validation error identifying the problematic flag and its invalid value

---

### User Story 4 - Configuration Validation and Clear Error Messages (Priority: P1)

As an operator deploying the application for the first time, I need the application to validate all configuration at startup and provide clear, actionable error messages if anything is wrong, so that I can quickly identify and fix configuration issues.

**Why this priority**: Poor error messages lead to operational delays and frustration. This is critical for production readiness and reduces deployment time significantly.

**Independent Test**: Can be fully tested by intentionally providing invalid or missing configuration values and verifying that the application fails fast at startup with helpful error messages that identify the specific problem, the expected format, and where to fix it.

**Acceptance Scenarios**:

1. **Given** a required configuration field is missing, **When** the application starts, **Then** it displays an error message like "Configuration error: Required field 'logLevel' is missing. Expected: one of [debug, info, warn, error]. Check: .env files, config.yaml, or use --log-level flag"
2. **Given** a log level is set to an invalid value "verbose", **When** the application starts, **Then** it displays an error message identifying the invalid value and listing valid options
3. **Given** a config.yaml file contains malformed YAML syntax, **When** the application starts, **Then** it displays a clear error message identifying the file path, line number (if possible), and nature of the syntax error
4. **Given** all required configuration is valid, **When** the application starts, **Then** it displays a summary showing all loaded configuration values (with sensitive values redacted) and their sources (e.g., "logLevel: debug [source: CLI flag]")

---

### Edge Cases

- Empty or comment-only .env files are treated as valid and processing continues normally
- How does the system handle circular references in environment variable substitution (e.g., VAR1=${VAR2}, VAR2=${VAR1})?
- When a YAML file contains duplicate keys, the last key wins (YAML 1.2 spec compliant behavior)
- How does the system handle environment variable names that don't follow the IDENTITY_BROKER_ prefix convention?
- When a command-line flag is provided twice with different values, the last value wins (most recent takes precedence)
- Configuration files have no explicit size limit and are constrained only by available system memory
- What happens when file permissions prevent reading a configuration file?
- How does the system handle configuration values containing special characters or multi-line strings?

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: System MUST support loading configuration from .env files with environment-specific precedence: .env → .env.local → .env.{environment} → .env.{environment}.local; empty files or files containing only comments are treated as valid
- **FR-002**: System MUST support loading configuration from a YAML file with the path configurable via command-line flag (--config) or environment variable (IDENTITY_BROKER_CONFIG_PATH)
- **FR-003**: System MUST support overriding any configuration value via command-line flags; when a flag is provided multiple times, the last value takes precedence
- **FR-004**: Command-line flags MUST take precedence over YAML configuration, which MUST take precedence over .env files
- **FR-005**: System MUST support environment variable substitution in YAML values using the syntax ${VARIABLE_NAME}
- **FR-006**: Environment variables referenced in YAML MUST use the prefix IDENTITY_BROKER_ for all sensitive values
- **FR-007**: System MUST validate all configuration values at startup before initializing any application components
- **FR-008**: System MUST display clear, actionable error messages identifying missing or invalid configuration fields, expected formats, and all possible configuration sources
- **FR-009**: System MUST provide default values for all non-required configuration options
- **FR-010**: System MUST display a configuration summary at startup showing all loaded values, their sources, and redacting sensitive values
- **FR-011**: System MUST support logging configuration including log level (debug, info, warn, error) and log format (text, json); application logs are written to stdout
- **FR-012**: System MUST terminate gracefully if any required configuration is missing or invalid
- **FR-013**: System MUST detect and reject circular references in environment variable substitution
- **FR-014**: System MUST handle malformed YAML files with clear error messages including file path and error location
- **FR-015**: System MUST follow YAML 1.2 specification behavior where duplicate keys result in the last value taking precedence

### Security Requirements

- **SR-001**: Configuration summary logging MUST redact all values from environment variables prefixed with IDENTITY_BROKER_ (replacing with "***REDACTED***")
- **SR-002**: System MUST NOT log raw environment variable values during substitution or validation
- **SR-003**: Configuration file loading MUST respect file system permissions and fail securely if access is denied
- **SR-004**: System MUST emit structured JSON audit log entries to stdout when configuration is loaded, including sources used and timestamp


### Key Entities

- **Configuration Schema**: Defines all valid configuration options including their types, default values, validation rules, and sensitivity level (sensitive vs. non-sensitive). Schema includes logging configuration (level, format) as initial scope.
- **Configuration Source**: Represents a source of configuration data (.env file, YAML file, command-line flag) with associated precedence level and loading mechanism.
- **Environment Variable Reference**: Placeholder in configuration that references an environment variable for runtime substitution.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Operators can deploy the application to new environments (development, staging, production) by changing only the environment variable without modifying code or configuration files
- **SC-002**: Configuration validation errors are resolved within 5 minutes of first occurrence due to clear, actionable error messages
- **SC-003**: Zero instances of sensitive configuration values (API keys, passwords, tokens) appearing in logs or version control systems
- **SC-004**: Application startup completes successfully in under 5 seconds when all configuration is valid
- **SC-005**: 100% of required configuration fields are validated at startup before any application logic executes
- **SC-006**: Configuration changes can be tested without file modifications by using command-line flag overrides in 100% of scenarios

## Assumptions

1. **File Size Limits**: Configuration files have no explicit size limit; they are constrained only by available system memory at runtime
2. **YAML Format**: Standard YAML 1.2 specification is used for configuration files
3. **Configuration Scope**: Initial configuration scope is limited to logging settings (level and format); additional configuration categories will be added in future phases
4. **File Locations**: Configuration files (.env, config.yaml) are expected in the application root directory by default, with override capability via environment variables or flags
5. **Validation Timing**: Configuration validation occurs synchronously at application startup, blocking further initialization until complete
6. **Error Handling Strategy**: Configuration errors are terminal (application does not start) rather than falling back to defaults, following fail-safe principles
7. **Concurrent Access**: Configuration files are read-only after initial load; no hot-reloading or concurrent modification support is required
8. **Character Encoding**: All configuration files use UTF-8 encoding
