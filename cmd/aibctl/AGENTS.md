# `cmd/aibctl/` — Admin CLI entry point

`main.go` only calls `aibctlcmd.Execute(deps, os.Args[1:])` and exits with its code. All behaviour lives in `internal/aibctl/`; read `internal/aibctl/AGENTS.md` before changing anything.

Build: `just aibctl-build` (version and commit are injected through `AIBCTL_LDFLAGS`). Cross-compile: `just aibctl-build-all`. Tests: `just aibctl-test`, `just test-e2e-aibctl`.

Operator documentation: `docs/guides/admin-cli.md`; configuration: `docs/configuration.md` ("aibctl configuration"); example: `examples/config/aibctl.yaml`; decision: `adrs/038-admin-cli-standalone-binary.md`.
