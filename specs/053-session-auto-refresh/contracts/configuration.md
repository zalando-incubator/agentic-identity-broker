# Contract: Broker configuration

**Feature**: `053-session-auto-refresh` | Spec: Configuration Requirements | Research R15

## Keys

| Key | Type | Default | Validation | Env var | CLI flag |
|---|---|---|---|---|---|
| `token_refresh.health_metrics.enabled` | bool | `true` | none | `IDENTITY_BROKER_TOKEN_REFRESH_HEALTH_METRICS_ENABLED` | `--token_refresh.health_metrics.enabled` |
| `token_refresh.health_metrics.query_timeout` | duration | `5s` | `> 0` | `IDENTITY_BROKER_TOKEN_REFRESH_HEALTH_METRICS_QUERY_TIMEOUT` | `--token_refresh.health_metrics.query_timeout` |

The gauge publishes only when the global MeterProvider exports metrics: `telemetry.enabled` and `telemetry.metrics.enabled` both true. Otherwise the no-op provider never runs the query (research R11).

## Go types (`internal/ports/config.go`)

```go
type TokenRefreshConfig struct {
	LookaheadDuration time.Duration                   `mapstructure:"lookahead_duration"`
	BackgroundWorkers int                             `mapstructure:"background_workers"`
	Sweep             TokenRefreshSweepConfig         `mapstructure:"sweep"`
	HealthMetrics     TokenRefreshHealthMetricsConfig `mapstructure:"health_metrics"`
}

type TokenRefreshHealthMetricsConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	QueryTimeout time.Duration `mapstructure:"query_timeout"`
}
```

## Loader and validator

| File | Change |
|---|---|
| `internal/config/loader.go:210-215` | `SetDefault` and `BindEnv` for both keys |
| `internal/config/loader.go:304` | Add both keys to the defaults source key list |
| `internal/config/loader.go:739-752` | Copy both changed flags in `bindFlags` |
| `cmd/agentic-identity-broker/root.go:453-455` | `PersistentFlags().Bool(...)` and `.Duration(...)` with "(default: …)" help text |
| `internal/config/validator.go:688-699` | `formatValidationError("token_refresh.health_metrics.query_timeout", v.String(), "greater than 0", nil)` when `<= 0` |

## Example YAML

Added to `examples/config/telemetry.yaml` and `examples/config/config.production.yaml`:

```yaml
token_refresh:
  lookahead_duration: 5m
  health_metrics:
    enabled: true        # publishes oauth2_sessions when telemetry metrics are enabled
    query_timeout: 5s
```

## Documentation

- `docs/configuration.md:248-255`: add two rows to the Token Refresh reference table.
- `docs/configuration.md:700-722`: extend the YAML block, replace "The three flags" with the current count, and replace line 722 with a reference to the chart-managed schedule and the runbook.
- `docs/configuration.md:1486-1503`: mention `oauth2_sessions` in the telemetry reference.

## Helm

See [chart-values.md](chart-values.md#broker-values-existing-section).
