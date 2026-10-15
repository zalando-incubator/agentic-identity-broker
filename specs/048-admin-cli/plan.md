# Implementation Plan: Admin CLI (`aibctl`)

**Branch**: `claude/nifty-fermat-uptxnc` | **Date**: 2026-10-09 | **Spec**: `specs/048-admin-cli/spec.md`

## Summary

A standalone Go CLI over the Admin API with per-user contexts, token-command authentication, canonical-first output, dynamic shell completion, documented exit codes, and an in-process Ginkgo E2E suite. Governed by ADR 038.

## Technical Context

- Go 1.27, Cobra 1.10, Viper 1.21, `go.yaml.in/yaml/v3` (already present), stdlib `text/tabwriter`, `encoding/json`, `os/exec`.
- No new modules. No broker package imports (AST-enforced).
- Targets: linux amd64/arm64, darwin amd64/arm64, windows amd64.

## Constitution Check

| Principle | Status |
|---|---|
| I Security-First | Tokens never logged; no-shell exec with timeout; 0600 config; redacted secret never resent. |
| II ADRs binding | ADR 038 added; ARCHITECTURE.md §3.3 added in the same change. |
| IV/X OpenAPI first | No API change; the CLI follows `api/admin/openapi.yaml`. |
| V Glossary | Context (aibctl), Token Command, Canonical-first display added. |
| VI Hexagonal | Not applicable to a client binary; isolation test keeps the CLI outside the hexagon. |
| VII Configuration | Deviation documented in ADR 038 per ADR 011 precedent; `docs/configuration.md` and `examples/config/aibctl.yaml` updated; Helm chart untouched. |
| VIII TDD | Unit tests per package, table-driven, testify; E2E written per scenario. |
| XII Builder | Not applicable; `NewRootCommand(deps)` is the composition root. |
| XIII E2E | One `It()` per scenario in `tests/e2e/aibctl/`, production bootstrap via `bootstrap.NewAdminTestServer`. |

## Project Structure

```
cmd/aibctl/main.go
internal/aibctl/
  architecture_test.go   version/   config/   auth/   client/   output/   input/   cmd/
tests/e2e/aibctl/        suite, harness, context_test, agents_test, services_test, keys_test
adrs/038-admin-cli-standalone-binary.md
docs/guides/admin-cli.md   docs/configuration.md (section)   examples/config/aibctl.yaml
```

## Design and Implementation Approach

1. **Config** (`internal/aibctl/config`): schema with contexts; `RegisterFlags` on the root; `LoadFromViper` for tests; `Resolve` merges context, defaults, env, flags; `Validate` for the file; `Save` atomic 0600; `Redacted` for display.
2. **Auth** (`internal/aibctl/auth`): `TokenSource` (static, command), `Authenticator.Apply` sets `Authorization` and the principal header, memoised per process.
3. **Client** (`internal/aibctl/client`): one `do()` that sets `Accept`, `Content-Type` on writes, `Prefer` on reads, `If-Match`, `Host`, extra headers; `APIError` maps JSON and plain-text failures; DTOs with `NullableString` for three-state fields and separate read/write structs.
4. **Output/Input**: `Printer` (table, wide, json, yaml via JSON key names), `DisplayID`; `input.Decode` normalises YAML to JSON, drops read-only keys, rejects unknown fields.
5. **Commands** (`internal/aibctl/cmd`): `NewRootCommand(deps)`, lazy `runtime()`, `UsageError`/`ExitCode`, `confirm`, per-resource command files, `completeResource` with a 2s timeout.
6. **E2E**: harness builds storage + app + admin server per `It`, writes a temp config, runs `aibctlcmd.Execute` in-process.

## Implementation Phase Overview

Phase 1 foundation → Phase 2 agents and permission sets → Phase 3 services and protected resources → Phase 4 credentials and keys → Phase 5 documentation, release wiring.

## Testing Strategy

- **E2E**: `tests/e2e/aibctl/` (`just test-e2e-aibctl`), 19 scenarios.
- **Unit**: `just aibctl-test` (`go test -race ./internal/aibctl/... ./cmd/aibctl/...`).
- **Lint**: `just check`.

## Complexity Tracking

| Deviation | Why | Alternative rejected |
|---|---|---|
| Separate config schema (`AIBCTL_`) | Client tool on a workstation; ADR 011 precedent | Sharing `internal/ports/config.go` drags server settings into the client |
| Viper overlay plus yaml.v3 for contexts | Viper cannot overlay a list element | Flattening contexts into dotted keys makes the file unreadable |
