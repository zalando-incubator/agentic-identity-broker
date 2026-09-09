# ADR 011: ExtProc Token Exchange as Standalone Binary with Separate Configuration Schema

**Status**: Accepted
**Date**: 2026-02-23
**Feature**: 015-extproc-token-exchange

---

## Context

The ExtProc token exchange service (`cmd/extproc-token-exchange`) is implemented as a new standalone binary in this repository. It runs as an independent process alongside agentgateway, intercepting gRPC streams via the Envoy External Processor (ExtProc) protocol to perform transparent RFC 8693 token exchange.

Constitution Principle VII (Configuration-Driven Design) requires all features to use the shared configuration port defined in `internal/ports/config.go` and declare their configuration structs in `internal/config/schema.go` with the env prefix `IDENTITY_BROKER_`. This ADR documents and formally accepts a deviation from that principle for the ExtProc service.

---

## Decision

The ExtProc service uses its own separate Viper/Cobra configuration stack in `internal/extproc/config/` with the env prefix `EXTPROC_`. It does **not** share configuration structs or configuration loading infrastructure with the identity broker binary.

The configuration schema is defined exclusively by:

- `internal/extproc/config/config.go` — Config struct (`GRPCConfig`, `OAuth2Config`, `CacheConfig`, `LogConfig`)
- `internal/extproc/config/loader.go` — Viper loader with `EXTPROC_` prefix
- `examples/config/extproc-token-exchange.yaml` — Reference example for operators

The broker Helm chart is not an ExtProc configuration surface because it deploys the broker binary only. ExtProc settings MUST NOT be added to that chart's broker ConfigMap or Deployment. A chart that later deploys ExtProc MUST provide a distinct workload and configuration path for `EXTPROC_*` settings.
This Constitution amendment reconciles accepted practice rather than making a feature exemption.

---

## Rationale

### 1. Separate operational concern

The ExtProc service is a completely separate process with a completely different operational concern from the identity broker. The identity broker manages OAuth2 grants, user consent, session storage, and encryption. The ExtProc service is a stateless gRPC proxy that performs token exchange on behalf of agentgateway. Sharing the identity broker's config schema would require operators to set `IDENTITY_BROKER_*` env vars for a service that has no identity broker concerns.

### 2. Env prefix clarity

The separate env prefix (`EXTPROC_`) provides clear operational semantics: every runtime parameter for the ExtProc service is configured exclusively with `EXTPROC_*` vars. There is no ambiguity about which binary a given env var controls.

### 3. 12-factor app config namespace isolation

Running both binaries in the same container or environment would produce a shared environment variable namespace. Sharing the `IDENTITY_BROKER_` prefix between two binaries violates the 12-factor app principle that each process owns its configuration namespace. Separate prefixes enforce this boundary.

### 4. Identity broker schema has irrelevant fields

The identity broker's config schema includes sections for PostgreSQL storage, envelope encryption (AWS KMS), the admin server, and session management. None of these fields have any meaning for a stateless gRPC proxy that holds only an in-memory token cache. Including them in a shared struct would force the ExtProc service to load, validate, and potentially fail on configuration fields it cannot use.

### 5. Pattern reuse without schema coupling

The ExtProc loader reuses the same approach established in `internal/config/loader.go` — Viper with multi-source loading (YAML file, env vars, CLI flags), `${ENV_VAR}` expansion for secrets, structured startup logging, and fail-fast validation at startup. The approach is shared; the schema is not.

---

## Alternatives Considered

### A. Embed ExtProc logic in the identity broker binary as a subcommand

Rejected. The ExtProc service is designed to be deployed independently alongside agentgateway, not co-located with the identity broker. Co-location would couple the operational lifecycle of two services with different uptime and scaling requirements and would contradict the spec's deployment model (separate containers in docker-compose).

### B. Add an `extproc` section to the shared config schema

Rejected. This would grow `internal/config/schema.go` with ExtProc-specific fields that have no relevance to the identity broker's operation. It would also require the identity broker binary to load and validate gRPC bind address and token exchange endpoint parameters on every start, even in deployments where the ExtProc service is not used.

---

## Consequences

### Positive

- **Operational clarity**: `EXTPROC_*` env vars are unambiguously for the ExtProc service; no confusion with identity broker configuration.
- **Independent deployment**: The ExtProc binary can be deployed, scaled, and restarted independently without any coupling to identity broker configuration files.
- **Smaller identity broker footprint**: The identity broker schema stays focused on its own concerns and does not accumulate fields from unrelated services.
- **Pattern consistency**: Both binaries use Viper/Cobra with env prefixes, YAML files, env expansion, and fail-fast validation — a consistent infrastructure approach across all binaries in the repository.
- **Correct deployment ownership**: ExtProc configuration is kept out of the broker chart unless that chart adds an ExtProc workload with its own configuration path.

### Negative

- **Formal deviation from Principle VII**: Any reviewer reading the constitution must be aware that this deviation is intentional and accepted for multi-binary repository patterns.
- **Duplicate loader infrastructure**: Each binary maintains its own loader implementation. Changes to the loader pattern (e.g., adding a new config source) must be applied to both.

### Precedent established

A deviation from Principle VII is formally accepted for standalone multi-binary repository patterns. Each new binary in `cmd/` that represents a separate deployable unit may use its own configuration schema and env prefix, provided it:

1. Reuses the Viper/Cobra loading approach established in ADR 002.
2. Uses a distinct, non-colliding env prefix that identifies the service (e.g., `EXTPROC_`).
3. Applies the same security patterns: env expansion for secrets, sensitive field redaction in logs, and fail-fast validation at startup.
4. Documents the deviation in an ADR.
5. Keeps its settings out of a Helm chart that does not deploy that binary; any chart that does deploy it must provide its own workload configuration path.

---

## References

- [ADR 002: Configuration Library Selection](002-configuration-libraries.md)
- Constitution Principle VII: Configuration-Driven Design (`.specify/memory/constitution.md`)
- Feature Specification: `specs/015-extproc-token-exchange/`
- Configuration Contract: `specs/015-extproc-token-exchange/contracts/configuration.md`
- Implementation: `internal/extproc/config/`, `cmd/extproc-token-exchange/`
- Example configuration: `examples/config/extproc-token-exchange.yaml`
