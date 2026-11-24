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

var domainEventsFlags = []struct {
	key  string
	env  string
	kind string
}{
	{"domain_events.enabled", "IDENTITY_BROKER_DOMAIN_EVENTS_ENABLED", "bool"},
	{"domain_events.write_timeout", "IDENTITY_BROKER_DOMAIN_EVENTS_WRITE_TIMEOUT", "duration"},
	{"domain_events.retention.period", "IDENTITY_BROKER_DOMAIN_EVENTS_RETENTION_PERIOD", "duration"},
	{"domain_events.retention.interval", "IDENTITY_BROKER_DOMAIN_EVENTS_RETENTION_INTERVAL", "duration"},
	{"domain_events.retention.batch_size", "IDENTITY_BROKER_DOMAIN_EVENTS_RETENTION_BATCH_SIZE", "int"},
}

func loadDomainEventsRootConfig(t *testing.T, document string, environment, args []string) (*ports.Config, error) {
	t.Helper()
	for _, setting := range domainEventsFlags {
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
	for i, setting := range domainEventsFlags {
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

func TestDomainEventsRootConfigurationPrecedence(t *testing.T) {
	const file = "domain_events:\n  enabled: true\n  write_timeout: 2s\n  retention:\n    period: 48h\n    interval: 2h\n    batch_size: 200\n"
	defaults := ports.DomainEventsConfig{
		Enabled:      false,
		WriteTimeout: time.Second,
		Retention:    ports.DomainEventsRetentionConfig{Period: 2160 * time.Hour, Interval: time.Hour, BatchSize: 1000},
	}
	fileValues := ports.DomainEventsConfig{
		Enabled:      true,
		WriteTimeout: 2 * time.Second,
		Retention:    ports.DomainEventsRetentionConfig{Period: 48 * time.Hour, Interval: 2 * time.Hour, BatchSize: 200},
	}
	environment := []string{"false", "3s", "72h", "3h", "300"}
	envValues := ports.DomainEventsConfig{
		Enabled:      false,
		WriteTimeout: 3 * time.Second,
		Retention:    ports.DomainEventsRetentionConfig{Period: 72 * time.Hour, Interval: 3 * time.Hour, BatchSize: 300},
	}
	cliValues := ports.DomainEventsConfig{
		Enabled:      true,
		WriteTimeout: 4 * time.Second,
		Retention:    ports.DomainEventsRetentionConfig{Period: 96 * time.Hour, Interval: 4 * time.Hour, BatchSize: 400},
	}
	cli := []string{
		"--domain_events.enabled=true",
		"--domain_events.write_timeout=4s",
		"--domain_events.retention.period=96h",
		"--domain_events.retention.interval=4h",
		"--domain_events.retention.batch_size=400",
	}
	disabledFile := fileValues
	disabledFile.Enabled = false
	for _, tc := range []struct {
		name        string
		document    string
		environment []string
		args        []string
		want        ports.DomainEventsConfig
	}{
		{name: "omitted flags retain defaults without a file", want: defaults},
		{name: "omitted flags preserve file over defaults", document: file, want: fileValues},
		{name: "omitted flags preserve environment over defaults", environment: environment, want: envValues},
		{name: "omitted flags preserve environment over file", document: file, environment: environment, want: envValues},
		{name: "all dotted CLI flags override environment and file", document: file, environment: environment, args: cli, want: cliValues},
		{name: "all dotted CLI flags override defaults without a file", args: cli, want: cliValues},
		{name: "explicit CLI false overrides enabled file and preserves omitted fields", document: file, args: []string{"--domain_events.enabled=false"}, want: disabledFile},
		{name: "explicit CLI false overrides enabled environment and file", document: file, environment: []string{"true", "3s", "72h", "3h", "300"}, args: []string{"--domain_events.enabled=false"}, want: envValues},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := loadDomainEventsRootConfig(t, tc.document, tc.environment, tc.args)
			require.NoError(t, err)
			require.Equal(t, tc.want, cfg.DomainEvents)
		})
	}
}

func TestDomainEventsFlagOverridesUnresolvedValue(t *testing.T) {
	for _, source := range []string{"yaml", "dotenv"} {
		t.Run(source, func(t *testing.T) {
			t.Setenv("TEST_LEDGER_RETENTION_UNSET", "")
			require.NoError(t, os.Unsetenv("TEST_LEDGER_RETENTION_UNSET"))
			var document string
			if source == "dotenv" {
				t.Chdir(t.TempDir())
				require.NoError(t, os.WriteFile(".env", []byte("IDENTITY_BROKER_DOMAIN_EVENTS_RETENTION_PERIOD='${TEST_LEDGER_RETENTION_UNSET}'\n"), 0o600))
			} else {
				document = "domain_events:\n  retention:\n    period: ${TEST_LEDGER_RETENTION_UNSET}\n"
			}
			_, err := loadDomainEventsRootConfig(t, document, nil, nil)
			require.ErrorContains(t, err, "TEST_LEDGER_RETENTION_UNSET")
			cfg, err := loadDomainEventsRootConfig(t, document, nil, []string{"--domain_events.retention.period=24h"})
			require.NoError(t, err)
			require.Equal(t, 24*time.Hour, cfg.DomainEvents.Retention.Period)
		})
	}
}

func TestDomainEventsRootRejectsInvalidExplicitCLIValuesWhileDisabled(t *testing.T) {
	for _, tc := range []struct {
		key   string
		value string
	}{
		{"domain_events.write_timeout", "0s"},
		{"domain_events.retention.period", "0s"},
		{"domain_events.retention.interval", "0s"},
		{"domain_events.retention.batch_size", "0"},
	} {
		t.Run(tc.key, func(t *testing.T) {
			_, err := loadDomainEventsRootConfig(t, "{}\n", []string{"true", "1s", "2160h", "1h", "1000"}, []string{
				"--domain_events.enabled=false",
				"--" + tc.key + "=" + tc.value,
			})
			require.ErrorContains(t, err, tc.key)
		})
	}
}
