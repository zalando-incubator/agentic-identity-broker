package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

var businessEventsFlags = []struct {
	key  string
	env  string
	kind string
}{
	{"business_events.enabled", "IDENTITY_BROKER_BUSINESS_EVENTS_ENABLED", "bool"},
	{"business_events.write_timeout", "IDENTITY_BROKER_BUSINESS_EVENTS_WRITE_TIMEOUT", "duration"},
	{"business_events.retention.period", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD", "duration"},
	{"business_events.retention.interval", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_INTERVAL", "duration"},
	{"business_events.retention.batch_size", "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_BATCH_SIZE", "int"},
}

func loadBusinessEventsRootConfig(t *testing.T, document string, environment, args []string) (*ports.Config, error) {
	t.Helper()
	for _, setting := range businessEventsFlags {
		flag := rootCmd.PersistentFlags().Lookup(setting.key)
		require.NotNil(t, flag, "production root command must register %s", setting.key)
		require.Equal(t, setting.kind, flag.Value.Type())
		previous, changed := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			require.NoError(t, flag.Value.Set(previous))
			flag.Changed = changed
		})
		require.NoError(t, flag.Value.Set(flag.DefValue))
		flag.Changed = false
	}

	path := filepath.Join(t.TempDir(), "broker.yaml")
	setBrokerConfigPath(t, path)
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", brokerTestKey)
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", brokerTestKey)
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
	for i, setting := range businessEventsFlags {
		t.Setenv(setting.env, "")
		if environment != nil {
			t.Setenv(setting.env, environment[i])
		}
	}
	if document != "" {
		require.NoError(t, os.WriteFile(path, []byte(document), 0o600))
	}
	require.NoError(t, rootCmd.ParseFlags(args))
	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	return loader.GetConfig(context.Background())
}

func TestBusinessEventsRootConfigurationPrecedence(t *testing.T) {
	const file = "business_events:\n  enabled: true\n  write_timeout: 2s\n  retention:\n    period: 48h\n    interval: 2h\n    batch_size: 200\n"
	defaults := ports.BusinessEventsConfig{
		Enabled:      false,
		WriteTimeout: time.Second,
		Retention:    ports.BusinessEventsRetentionConfig{Period: 2160 * time.Hour, Interval: time.Hour, BatchSize: 1000},
	}
	fileValues := ports.BusinessEventsConfig{
		Enabled:      true,
		WriteTimeout: 2 * time.Second,
		Retention:    ports.BusinessEventsRetentionConfig{Period: 48 * time.Hour, Interval: 2 * time.Hour, BatchSize: 200},
	}
	environment := []string{"false", "3s", "72h", "3h", "300"}
	envValues := ports.BusinessEventsConfig{
		Enabled:      false,
		WriteTimeout: 3 * time.Second,
		Retention:    ports.BusinessEventsRetentionConfig{Period: 72 * time.Hour, Interval: 3 * time.Hour, BatchSize: 300},
	}
	cliValues := ports.BusinessEventsConfig{
		Enabled:      true,
		WriteTimeout: 4 * time.Second,
		Retention:    ports.BusinessEventsRetentionConfig{Period: 96 * time.Hour, Interval: 4 * time.Hour, BatchSize: 400},
	}
	cli := []string{
		"--business_events.enabled=true",
		"--business_events.write_timeout=4s",
		"--business_events.retention.period=96h",
		"--business_events.retention.interval=4h",
		"--business_events.retention.batch_size=400",
	}
	disabledFile := fileValues
	disabledFile.Enabled = false
	for _, tc := range []struct {
		name        string
		document    string
		environment []string
		args        []string
		want        ports.BusinessEventsConfig
	}{
		{name: "omitted flags retain defaults without a file", want: defaults},
		{name: "omitted flags preserve file over defaults", document: file, want: fileValues},
		{name: "omitted flags preserve environment over defaults", environment: environment, want: envValues},
		{name: "omitted flags preserve environment over file", document: file, environment: environment, want: envValues},
		{name: "all dotted CLI flags override environment and file", document: file, environment: environment, args: cli, want: cliValues},
		{name: "all dotted CLI flags override defaults without a file", args: cli, want: cliValues},
		{name: "explicit CLI false overrides enabled file and preserves omitted fields", document: file, args: []string{"--business_events.enabled=false"}, want: disabledFile},
		{name: "explicit CLI false overrides enabled environment and file", document: file, environment: []string{"true", "3s", "72h", "3h", "300"}, args: []string{"--business_events.enabled=false"}, want: envValues},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadBusinessEventsRootConfig(t, tc.document, tc.environment, tc.args)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.BusinessEvents)
		})
	}
}

func TestBusinessEventsFlagOverridesUnresolvedValue(t *testing.T) {
	for _, source := range []string{"yaml", "dotenv"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("TEST_LEDGER_RETENTION_UNSET", "")
			require.NoError(t, os.Unsetenv("TEST_LEDGER_RETENTION_UNSET"))
			var document string
			if source == "dotenv" {
				t.Chdir(t.TempDir())
				require.NoError(t, os.WriteFile(".env", []byte("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION_PERIOD='${TEST_LEDGER_RETENTION_UNSET}'\n"), 0o600))
			} else {
				document = "business_events:\n  retention:\n    period: ${TEST_LEDGER_RETENTION_UNSET}\n"
			}
			_, err := loadBusinessEventsRootConfig(t, document, nil, nil)
			require.ErrorContains(t, err, "TEST_LEDGER_RETENTION_UNSET")
			cfg, err := loadBusinessEventsRootConfig(t, document, nil, []string{"--business_events.retention.period=24h"})
			require.NoError(t, err)
			require.Equal(t, 24*time.Hour, cfg.BusinessEvents.Retention.Period)
		})
	}
}

func TestBusinessEventsRootRejectsInvalidExplicitCLIValuesWhileDisabled(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value string
	}{
		{"business_events.write_timeout", "0s"},
		{"business_events.retention.period", "0s"},
		{"business_events.retention.interval", "0s"},
		{"business_events.retention.batch_size", "0"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, err := loadBusinessEventsRootConfig(t, "{}\n", []string{"true", "1s", "2160h", "1h", "1000"}, []string{
				"--business_events.enabled=false",
				"--" + tc.key + "=" + tc.value,
			})
			require.ErrorContains(t, err, tc.key)
		})
	}
}
