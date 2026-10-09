# Contract: Configuration (`token_refresh`)

**Feature**: `029-proactive-token-refresh` | Principle VII | Research R9

## Schema

**File**: `internal/ports/config.go`. Add `TokenRefresh TokenRefreshConfig \`mapstructure:"token_refresh"\``
to `ports.Config`, after `Approvals`. The structs are in [data-model.md](../data-model.md#configuration-entity-tokenrefreshconfig).

| YAML key | Go field | Type | Default | Env var | CLI flag | Validation (`internal/config/validator.go`) |
|---|---|---|---|---|---|---|
| `token_refresh.lookahead_duration` | `LookaheadDuration` | Go duration string | `5m` | `IDENTITY_BROKER_TOKEN_REFRESH_LOOKAHEAD_DURATION` | `--token_refresh.lookahead_duration` | `> 0` |
| `token_refresh.background_workers` | `BackgroundWorkers` | int | `10` | `IDENTITY_BROKER_TOKEN_REFRESH_BACKGROUND_WORKERS` | `--token_refresh.background_workers` | `1 ≤ n ≤ 20` |
| `token_refresh.sweep.default_page_size` | `Sweep.DefaultPageSize` | int | `100` | `IDENTITY_BROKER_TOKEN_REFRESH_SWEEP_DEFAULT_PAGE_SIZE` | `--token_refresh.sweep.default_page_size` | `1 ≤ n ≤ 1000` |

- Defaults: `l.v.SetDefault(...)` in `Loader.setDefaults` (`internal/config/loader.go:103-292`).
  Durations are given as strings (`"5m"`), as for `approvals.pending_ttl`.
- Environment: explicit `l.v.BindEnv(...)` with the `IDENTITY_BROKER_` prefix
  (`loader.go:132-174`). Numeric YAML durations are already rejected by `rejectNumericDurationHook`.
- Validation: add `validateTokenRefreshConfig(cfg.TokenRefresh)` to `config.Validate`. Errors name the
  key and the bound. The `background_workers` message states the reason: "each in-flight refresh
  holds a database connection; at most 20 of the 25 pooled connections may be used by background
  refresh".
- Precedence is unchanged: CLI flags > env > file > defaults.

Register the three persistent flags in `cmd/agentic-identity-broker/root.go`. Bind changed flags
to the matching Viper keys in `internal/config/loader.go` (`Loader.bindFlags`). Use `Duration`
for lookahead and `Int` for both counts. Test CLI > environment > file > defaults and startup
validation through the existing loader tests.

### Validation error examples (startup fails closed)

```text
token_refresh.lookahead_duration must be greater than 0 (got 0s)
token_refresh.background_workers must be between 1 and 20 (got 32): each in-flight refresh holds a database connection; at most 20 of the 25 pooled connections may be used by background refresh
token_refresh.sweep.default_page_size must be between 1 and 1000 (got 5000)
```

## YAML example

Added to `examples/config/third-party-oauth2.yaml` (the spec's named location). `examples/config/README.md`
gets a one-line mention in the existing third-party OAuth2 entry (`README.md:120-146`).

```yaml
# Proactive refresh of third-party access tokens (spec 029).
# Tokens are refreshed in the background when token exchange serves a token that
# expires within lookahead_duration, and by POST /api/sessions/sweep on the admin server.
token_refresh:
  # Refresh tokens this long before they expire. Must be shorter than the access-token
  # lifetime your providers issue; otherwise every exchange triggers a refresh.
  lookahead_duration: 5m
  # Concurrent background refreshes per broker replica (1-20). Requests beyond this
  # are dropped; the caller still receives its valid token.
  background_workers: 10
  sweep:
    # Sessions read per page by the admin sweep when the request omits page_size (1-1000).
    default_page_size: 100
```

## Documentation

`docs/configuration.md`:
- TOC entry (`:10-21`).
- Three rows in the Configuration Reference table (`:234-250` style: key, type, default, env).
- An "Available Settings → Token Refresh" subsection (`:592-598` style) with the YAML example and
  the operational notes: lookahead versus token lifetime; worker cap tied to DB pool size; the sweep
  is triggered externally (link to the runbook).

## Helm deployment contract (Principle VII)

`token_refresh` configures the broker, which `charts/agentic-identity-broker/` deploys. All four
chart artifacts MUST change:

| Artifact | Change |
|---|---|
| `values.yaml` | New `broker.tokenRefresh` block after `broker.tokenExchange` (`:600-636`), with `# --` doc comments. |
| `values.schema.json` | New `broker.tokenRefresh` object with `additionalProperties: false`, `lookaheadDuration` (string, pattern `^[0-9]+(ns|us|µs|ms|s|m|h)([0-9]+(ns|us|µs|ms|s|m|h))*$`), `backgroundWorkers` (integer 1–20), `sweep.defaultPageSize` (integer 1–1000). The `broker` object already disallows unknown properties (`:522-526`). |
| `templates/configmap.yaml` | Render a `token_refresh:` block after `third_party_oauth2` (`:77-79`) from `.Values.broker.tokenRefresh.*`. Always rendered, because the values have defaults. |
| `README.md` | Three rows in the parameter table (`:112-177`). |

```yaml
# values.yaml
broker:
  # Proactive third-party token refresh
  tokenRefresh:
    # -- Refresh third-party access tokens this long before expiry (Go duration).
    lookaheadDuration: "5m"
    # -- Maximum concurrent background refreshes per replica (1-20).
    backgroundWorkers: 10
    sweep:
      # -- Default page size for POST /api/sessions/sweep (1-1000).
      defaultPageSize: 100
```

```yaml
# templates/configmap.yaml (rendered)
    token_refresh:
      lookahead_duration: {{ .Values.broker.tokenRefresh.lookaheadDuration | quote }}
      background_workers: {{ .Values.broker.tokenRefresh.backgroundWorkers }}
      sweep:
        default_page_size: {{ .Values.broker.tokenRefresh.sweep.defaultPageSize }}
```

**Not in the chart**: a sweep CronJob. The spec makes scheduling an operator responsibility. The
runbook (`docs/operations/session-sweep.md`) provides a reference CronJob manifest. The CronJob
calls the admin API through the admin authentication proxy, so the configured principal header
identifies the operator. Shipping a CronJob template would add chart surface (schedule, auth
sidecar, proxy URL) that no requirement asks for.

## Related existing settings that bound behavior

| Setting | Effect on this feature |
|---|---|
| `server.shutdown.timeout` | Upper bound on draining in-flight background refreshes at shutdown (research R7). |
| `storage.timeouts.read` / `.write` | Bound each `ListExpiringSessions` page read and each locked refresh's database steps. |
| Upstream HTTP client timeout (`internal/app/builder.go:603-605`) | Bounds each refresh call; with storage timeouts, it forms `refreshOperationContext`. |
