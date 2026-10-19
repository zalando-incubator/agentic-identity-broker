# ADR 038: Admin CLI (`aibctl`) as a Standalone Binary with Per-User Context Configuration

**Status**: Accepted
**Date**: 2026-10-09
**Feature**: 048-admin-cli

---

## Context

Operators administer the broker through the Admin API on port 14000 with hand-written `curl` calls (`docs/guides/manage-agents-and-services.md`, `scripts/seed-sample-data.sh`). Every call must set `Content-Type: application/json`, the principal header, a matching `Host`, and, for protected-resource replacement, an `If-Match` ETag. Canonical IDs (ADR 030, feature 036) are accepted everywhere but nothing presents them to a human by default.

The repository ships two binaries, both servers. A command-line client has different needs: a per-user configuration file in the OS-specific location, credentials derived at invocation time from a single-sign-on helper, shell completion, and output that scripts can parse.

The Admin API does not validate Bearer tokens itself. It reads a principal header (`X-Remote-User` by default) and relies on a reverse proxy to authenticate the caller. A CLI therefore has to serve two deployments: a proxied one where it presents a Bearer token, and a local one where it sets the principal header directly.

## Decision

`aibctl` is a third binary in `cmd/aibctl/` with its implementation in `internal/aibctl/`. It is a pure HTTP client of `api/admin/openapi.yaml`.

1. **Standalone configuration schema** under the `AIBCTL_` prefix, following the precedent of ADR 011. The file lives at `os.UserConfigDir()/aibctl/config.yaml` (`~/.config/aibctl/` on Linux, `~/Library/Application Support/aibctl/` on macOS, `%AppData%\aibctl\` on Windows) and holds named **contexts**: a server URL, an optional `host_header`, extra headers, and the authentication settings. Precedence is flags, then `AIBCTL_*` environment variables, then the selected context, then `defaults`, then built-in defaults. The file is written with owner-only permissions. Viper carries the flat flag and environment overlay; the context list is decoded from the file with `go.yaml.in/yaml/v3` and the overlay is applied to the selected context, because Viper cannot overlay a list element.
2. **Two credentials, both optional, at least one required.** A context may define a Bearer token, either static (`token`, with `${VAR}` expansion) or produced by a **token command** (`token_command`, an argv list executed without a shell, trimmed stdout, bounded by `token_command_timeout`), and may define a `principal` sent in the configurable principal header. The CLI never logs tokens and resolves the token at most once per process. There is no on-disk token cache.
3. **No broker package imports.** `internal/aibctl/` and `cmd/aibctl/` import neither `internal/domain`, `internal/ports`, `internal/adapters`, `internal/app`, nor `internal/extproc`. The request and response types are re-declared from the OpenAPI document. An AST test (`internal/aibctl/architecture_test.go`) enforces the boundary.
4. **Canonical-first identifiers.** Every read sends `Prefer: reference-id=canonical`. Tables show a resource's canonical ID and fall back to its UUID; `--show-uuid` and `-o wide` add the UUID column. Positional identifiers accept either form unchanged.
5. **Full-replacement updates with conditional service writes.** `update -f` sends the document as the complete representation; field flags fetch the current representation and overlay the given fields. Read-only fields are dropped on input so `get -o yaml | update -f -` round-trips. `service update` always reads the ETag first and sends `If-Match`, and refuses to send the redacted `client_secret` placeholder back.
6. **Testable command tree.** `aibctlcmd.NewRootCommand(deps)` takes stdin, stdout, stderr, and the HTTP client explicitly; there are no package-level globals. The E2E suite in `tests/e2e/aibctl/` runs commands in-process against an admin server started with the production bootstrap.
7. **Documented exit codes** (0 success, 1 generic, 2 usage, 3 authentication, 4 not found, 5 conflict or precondition, 6 server-side validation) and a strict stdout/stderr split: stdout carries data only.

Out of scope for this decision: a terminal UI, an on-disk token cache or keyring integration, a generated OpenAPI client, TLS options, and user-grant or approval commands (those are End-User API concerns).

## Rationale

- **Separate deployable, separate schema.** The CLI runs on an operator's workstation, not in the broker's pod. Sharing `internal/ports/config.go` would drag storage, encryption, and server settings into a client that needs a URL and a credential. ADR 011 already accepts this for standalone binaries.
- **Proxy trust boundary is unchanged.** Supporting Bearer and principal headers in the client keeps the broker's authentication model exactly as documented: the proxy authenticates, the broker trusts the principal header. No broker code changes.
- **Decoupling from broker internals.** Re-declaring DTOs keeps the CLI honest about the published contract, lets it be built and versioned separately, and prevents a handler refactor from silently changing CLI behaviour.
- **Canonical IDs are the human interface.** Feature 036 made canonical IDs valid input everywhere; the CLI is the first consumer that makes them the default output, as the contract recommends for human-facing clients.
- **In-process E2E.** Running the Cobra root in-process gives the suite the same production bootstrap as the backend suite, with no binary build step and full parallelism under Ginkgo.

## Alternatives Considered

### A. `admin` subcommand of `cmd/agentic-identity-broker`

Rejected. It would ship the whole server, its dependencies, and its configuration surface to every operator, and it would couple CLI releases to broker releases.

### B. Generated client with `oapi-codegen`

Rejected for now. The admin API has eight resources and no pagination; the hand-written client is about 300 lines and lets the CLI treat ETags, the `Prefer` header, and three-state fields exactly as the contract describes. A generator can replace it later without changing commands.

### C. Reuse handler DTOs from `internal/adapters/http/handlers/admin`

Rejected. It would make the CLI depend on an adapter package, inverting the hexagonal direction and tying the CLI to broker builds.

### D. Keyring or on-disk token cache

Rejected for this iteration. Token commands such as single-sign-on helpers cache on their own; the CLI memoises the token per process, which covers multi-request commands and completion.

### E. Terminal UI

Explicitly left out by the stakeholder.

## Consequences

### Positive

- One command for every Admin API operation, with correct headers every time.
- Scripts get stable exit codes and `-o json`; humans get canonical IDs and tab completion.
- The constitution's Principle VII boundary is explicit: `AIBCTL_*` belongs to the CLI, the Helm chart stays untouched.

### Negative

- A third configuration loader to maintain.
- DTOs duplicate the OpenAPI schemas and must be updated when the contract changes; the E2E suite against the real server catches drift.
- `--client-secret` on the command line can land in shell history; the guide recommends the `-f` document or an environment variable.

### Precedent established

Any further command-line tool in `cmd/` follows this ADR: a `NewRootCommand(deps)` factory with explicit I/O, an isolation test against broker packages, documented exit codes, and an in-process Ginkgo suite under `tests/e2e/<tool>/`.

---

## References

- [ADR 002: Configuration Library Selection](002-configuration-libraries.md)
- [ADR 011: ExtProc Token Exchange as Standalone Binary](011-extproc-standalone-binary.md)
- [ADR 030: Normalize Protected Resources](030-normalize-protected-resources.md)
- Feature 036 contract: `specs/036-canonical-resource-ids/contracts/admin-canonical-ids.md`
- Feature specification: `specs/048-admin-cli/`
- Admin API contract: `api/admin/openapi.yaml`
- Implementation: `cmd/aibctl/`, `internal/aibctl/`, `tests/e2e/aibctl/`
- Example configuration: `examples/config/aibctl.yaml`
