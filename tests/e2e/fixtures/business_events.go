package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// BusinessEventConfig returns PostgreSQL configuration with explicit ledger settings and response trace IDs.
func BusinessEventConfig(connectionURL string, enabled bool) *ports.Config {
	cfg := DefaultOAuth2Config()
	cfg.Storage = ports.StorageConfig{
		Backend:  "postgres",
		Postgres: ports.PostgresConfig{ConnectionURL: connectionURL},
		Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 5 * time.Second},
	}
	cfg.BusinessEvents = ports.BusinessEventsConfig{
		Enabled:      enabled,
		WriteTimeout: time.Second,
		Retention: ports.BusinessEventsRetentionConfig{
			Period:    2160 * time.Hour,
			Interval:  time.Hour,
			BatchSize: 1000,
		},
	}
	SetTraceResponseEnabled(cfg, true)
	return cfg
}
