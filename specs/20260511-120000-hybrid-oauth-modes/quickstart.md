# Quickstart: Hybrid OAuth Server Modes

**Feature**: 20260511-120000-hybrid-oauth-modes | **Date**: 2026-05-12

## What This Feature Does

Replaces the binary `proxy`/`issue_token` mode selection with three symmetric modes (`proxy`, `local`, `hybrid`) and introduces universal agent resolution with property-based classification. In hybrid mode, proxy agents and local agents coexist in the same broker instance.

## Key Concepts

1. **Mode naming**: `issue_token` → `local`. Three modes: `proxy`, `local`, `hybrid`.
2. **Agent classification**: Derived from agent properties, not config. `Agent.ClientID` set → proxy (`ProxyClient`). `client_uris` set → CIMD (`CIMDClient`). Neither → local (`LocalClient`).
3. **Mode as strategy**: Each mode accepts/rejects client modes. Hybrid accepts all.
4. **No new entity fields**: Classification uses existing `ClientID` and `ClientURIs`.

## Implementation Order

### Phase 0: Rename `issue_token` → `local`
Pure refactor. Rename all references: config values, strategy types, builder branches, tests, docs, examples. Existing behavior unchanged.

### Phase 2: Design Preconditions
- Config restructuring (nested `proxy`/`local` sections)
- `ModeStrategy` interface + `ClientMode` value object
- E2E test design (21 scenarios)

### Phase 3-6: User Stories
- US1 (P1): Config validation for three modes
- US2 (P1): Universal resolution + classification + mode enforcement
- US3 (P2): CIMD feature gating by mode
- US4 (P2): Builder wiring as strategy pattern

## File Map

| What | Where |
|------|-------|
| Mode enum + config | `internal/ports/config.go` |
| Agent classification | `internal/domain/storage/agent.go` (method on Agent entity) |
| Mode strategy (domain) | `internal/domain/oauth2/mode_strategy.go` |
| Dispatching strategies (hybrid adapter) | `internal/adapters/http/enduser/` |
| Client resolver (UUID rejection for CIMD) | `internal/domain/oauth2/client_resolver.go` |
| Proceed strategies (renamed) | `internal/adapters/http/enduser/proceed_strategy.go` |
| Token grant strategies (renamed) | `internal/adapters/http/enduser/token_grant_strategy.go` |
| Builder wiring | `internal/app/builder.go` |
| E2E tests | `tests/e2e/hybrid_oauth_modes_e2e_test.go` |
| Config examples | `examples/config/oauth2-hybrid-mode.yaml` |

## Patterns to Follow

- **Strategy pattern**: Follow existing `AuthorizationProceedStrategy` and `TokenGrantStrategy` patterns from ADR 20260328-120100
- **Config validation**: Follow existing `validateProxyMode()` / `validateIssueTokenMode()` pattern in `internal/ports/config.go`
- **Builder wiring**: Follow existing mode branching in `internal/app/builder.go:609-653`
- **Client resolver**: Follow existing `OpaqueClientResolver` / `CIMDClientResolver` pattern
- **E2E tests**: Follow patterns in `tests/e2e/mode_configuration_e2e_test.go`
