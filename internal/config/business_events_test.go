package config

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validBusinessEventsConfig() ports.BusinessEventsConfig {
	return ports.BusinessEventsConfig{
		Enabled:      false,
		WriteTimeout: time.Second,
		Retention: ports.BusinessEventsRetentionConfig{
			Period:    2160 * time.Hour,
			Interval:  time.Hour,
			BatchSize: 1000,
		},
	}
}

func TestBusinessEventsConfigurationDefaults(t *testing.T) {
	setMinimalConfigEnv(t)
	setConfigYAML(t, "{}\n")
	for _, name := range []string{
		"IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED",
		"IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT",
		"IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD",
		"IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL",
		"IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE",
	} {
		t.Setenv(name, "")
	}

	cfg, err := NewLoader().GetConfig(context.Background())
	require.NoError(t, err)
	assert.False(t, cfg.BusinessEvents.Enabled)
	assert.Equal(t, time.Second, cfg.BusinessEvents.WriteTimeout)
	assert.Equal(t, 2160*time.Hour, cfg.BusinessEvents.Retention.Period)
	assert.Equal(t, time.Hour, cfg.BusinessEvents.Retention.Interval)
	assert.Equal(t, 1000, cfg.BusinessEvents.Retention.BatchSize)
}

func TestBusinessEventsConfigurationBounds(t *testing.T) {
	// Contract: write timeout "100ms to 10s", period "24h to 8760h",
	// interval "1s to 24h", and batch size "1 to 10000", all inclusive.
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%t", enabled), func(t *testing.T) {
			for _, tc := range []struct {
				key   string
				set   func(*ports.BusinessEventsConfig, int64)
				lower int64
				upper int64
			}{
				{"business_events.write_timeout", func(c *ports.BusinessEventsConfig, value int64) { c.WriteTimeout = time.Duration(value) }, int64(100 * time.Millisecond), int64(10 * time.Second)},
				{"business_events.retention.period", func(c *ports.BusinessEventsConfig, value int64) { c.Retention.Period = time.Duration(value) }, int64(24 * time.Hour), int64(8760 * time.Hour)},
				{"business_events.retention.interval", func(c *ports.BusinessEventsConfig, value int64) { c.Retention.Interval = time.Duration(value) }, int64(time.Second), int64(24 * time.Hour)},
				{"business_events.retention.batch_size", func(c *ports.BusinessEventsConfig, value int64) { c.Retention.BatchSize = int(value) }, 1, 10000},
			} {
				t.Run(tc.key, func(t *testing.T) {
					for _, boundary := range []struct {
						name  string
						value int64
						valid bool
					}{
						{"lower inclusive", tc.lower, true},
						{"upper inclusive", tc.upper, true},
						{"below lower", tc.lower - 1, false},
						{"above upper", tc.upper + 1, false},
						{"zero", 0, false},
						{"negative", -1, false},
					} {
						t.Run(boundary.name, func(t *testing.T) {
							cfg := validTestConfig()
							cfg.BusinessEvents = validBusinessEventsConfig()
							cfg.BusinessEvents.Enabled = enabled
							tc.set(&cfg.BusinessEvents, boundary.value)
							err := Validate(cfg)
							if boundary.valid {
								require.NoError(t, err)
							} else {
								require.ErrorContains(t, err, tc.key)
							}
						})
					}
				})
			}
		})
	}
}

func TestBusinessEventsConfigurationRejectsInvalidEnvironmentWhileDisabled(t *testing.T) {
	for _, tc := range []struct {
		key   string
		env   string
		value string
	}{
		{"business_events.write_timeout", "IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT", "99ms"},
		{"business_events.retention.period", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD", "23h"},
		{"business_events.retention.interval", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL", "999ms"},
		{"business_events.retention.batch_size", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE", "10001"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			setMinimalConfigEnv(t)
			setConfigYAML(t, "{}\n")
			t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED", "false")
			t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT", "1s")
			t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD", "2160h")
			t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL", "1h")
			t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE", "1000")
			t.Setenv(tc.env, tc.value)

			_, err := NewLoader().GetConfig(context.Background())
			require.ErrorContains(t, err, tc.key)
		})
	}
}
