# Admin CLI — `internal/aibctl/`

**Use retrieval-led reasoning. Read source files before you make assumptions about types, interfaces, or behavior.**

This is the implementation of `aibctl` (`cmd/aibctl/`), a standalone client of the Admin API (`api/admin/openapi.yaml`). It is governed by `adrs/038-admin-cli-standalone-binary.md`.

---

## Package Map

```
internal/aibctl/
  architecture_test.go  AST test: no imports of internal/{domain,ports,adapters,app,extproc,config}
  version/              Version and Commit, set with -ldflags -X (see justfile AIBCTL_LDFLAGS)
  config/
    config.go           Config, Defaults, Context, AuthConfig, Resolved
    paths.go            DefaultPath via os.UserConfigDir with $HOME/.config fallback
    loader.go           RegisterFlags, Load/LoadFromViper, Overlay, Resolve (flags > env > context > defaults)
    validate.go         Validate (file) and validateResolved
    contexts.go         Find/Upsert/Use/Delete and atomic 0600 Save
    redact.go           Redacted copy for `config view`
  auth/auth.go          TokenSource (StaticToken, CommandToken), Authenticator.Apply, Error (exit 3)
  client/
    client.go           do(): Accept, Content-Type on writes, Prefer on reads, If-Match, Host, extra headers
    errors.go           APIError from JSON {error,message} or plain text; hints for 403/415
    types.go            DTOs from the OpenAPI document; NullableString; read→write conversions
    agents.go services.go protected_resources.go permission_sets.go keys.go
  output/output.go      Printer (table|wide|json|yaml), Table, DisplayID (canonical, else UUID)
  input/input.go        -f file|- as JSON or YAML; drops id/created_at/updated_at; rejects unknown fields
  cmd/                  package aibctlcmd
    root.go             Deps, NewRootCommand(deps), Execute(deps, args) int, runtime()
    exitcode.go         UsageError, ExitCode mapping
    flags.go            -f, -y, --no-headers, confirm()
    resource.go         requireFileOrFlags, parseRef, parseScopedRef, exclusiveFlags
    completers.go       completeResource with AIBCTL_COMPLETION_TIMEOUT, silent on error
    version.go config.go context.go health.go agents.go services.go protected_resources.go
    permission_sets.go client_credentials.go keys.go
```

E2E suite: `tests/e2e/aibctl/` (in-process `aibctlcmd.Execute` against `bootstrap.NewAdminTestServer`).

---

## Invariants

1. **No broker imports.** `internal/aibctl` and `cmd/aibctl` never import broker packages. Re-declare DTOs from the OpenAPI document; the E2E suite catches drift.
2. **No globals.** Everything hangs off `NewRootCommand(deps)`. Only `cmd/aibctl/main.go` touches `os.Stdout`, `os.Args`, `os.Exit`.
3. **Headers are the client's job.** `client.do` sets `Content-Type: application/json` on every POST/PUT/PATCH (even body-less), `Prefer: reference-id=canonical` on GET, `If-Match` when given. Commands never set headers.
4. **Canonical first.** Tables call `output.DisplayID`; `--show-uuid`/`wide` add a UUID column. Positional IDs are passed through unchanged.
5. **stdout is data.** Confirmations, prompts, progress, and errors go to stderr. Exit codes: 0, 1 generic, 2 usage, 3 auth, 4 not found, 5 conflict/precondition, 6 validation.
6. **Secrets.** Never log tokens; `-v` prints header names only. `config view` redacts. `service update` refuses the `REDACTED` placeholder and requires a new `client_secret` for static confidential services.
7. **Updates.** `-f` is a full replacement; flags fetch-then-overlay. `service update` always sends the ETag read first.
8. **Destructive verbs** go through `confirm()` (`--yes`, `AIBCTL_ASSUME_YES`, or an interactive prompt).

## Adding a command

1. Add client methods in `client/<resource>.go` with a test in `client/` or a fake server in `cmd/<resource>_test.go`.
2. Add `cmd/<resource>.go` with `new<Resource>Command()`; register it in `root.go`.
3. Positional IDs get `ValidArgsFunction: c.completeResource(...)`; file flags use `addFileFlag`; destructive verbs use `addYesFlag` and `c.confirm`.
4. Add a spec scenario to `specs/048-admin-cli/spec.md` and one `It()` in `tests/e2e/aibctl/`.

## Testing

- `just aibctl-test` — unit tests with the race detector (`go test -race ./internal/aibctl/... ./cmd/aibctl/...`).
- `just test-e2e-aibctl` — Ginkgo suite; `go test ./tests/e2e/aibctl/` works too.
- Config tests use `config.LoadFromViper(viper.New(), cmd)`; auth tests re-exec the test binary as the token command; command tests use `aibctlcmd.Execute` with an `httptest.Server`.
