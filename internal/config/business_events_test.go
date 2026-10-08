package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

const (
	businessEventsRetentionEnv = "IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION"
	businessEventsCopyEnv      = "IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED"
)

func businessEventsConfigFixture(t *testing.T) {
	t.Helper()
	setMinimalConfigEnv(t) // Required JWE and memory encryption keys, authentication, and OAuth2 mode.
	t.Setenv(businessEventsRetentionEnv, "")
	t.Setenv(businessEventsCopyEnv, "")
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
}

func writeBusinessEventsConfig(t *testing.T, retention, copyEnabled string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "business-events.yaml")
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	content := fmt.Sprintf("business_events:\n  retention: %s\n  telemetry_copy_enabled: %s\n", retention, copyEnabled)
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func TestBusinessEventsConfigurationPrecedence(t *testing.T) {
	tests := []struct {
		name, fileRetention, fileCopy, envRetention, envCopy, cliRetention, cliCopy string
		wantRetention                                                               time.Duration
		wantCopy                                                                    bool
	}{
		{name: "defaults without operator overrides", wantRetention: 2160 * time.Hour, wantCopy: true},
		{name: "file overrides defaults including explicit false", fileRetention: "48h", fileCopy: "false", wantRetention: 48 * time.Hour},
		{name: "environment overrides file including explicit false", fileRetention: "48h", fileCopy: "true", envRetention: "36h", envCopy: "false", wantRetention: 36 * time.Hour},
		{name: "environment true overrides file false", fileRetention: "48h", fileCopy: "false", envRetention: "36h", envCopy: "true", wantRetention: 36 * time.Hour, wantCopy: true},
		{name: "CLI overrides environment and file including explicit false", fileRetention: "48h", fileCopy: "true", envRetention: "36h", envCopy: "true", cliRetention: "12h", cliCopy: "false", wantRetention: 12 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			businessEventsConfigFixture(t)
			if tt.fileRetention != "" {
				writeBusinessEventsConfig(t, tt.fileRetention, tt.fileCopy)
			}
			if tt.envRetention != "" {
				t.Setenv(businessEventsRetentionEnv, tt.envRetention)
				t.Setenv(businessEventsCopyEnv, tt.envCopy)
			}

			loader := NewLoader()
			if tt.cliRetention != "" {
				cmd := &cobra.Command{Use: "config-test"}
				cmd.Flags().Duration("business_events.retention", 0, "")
				cmd.Flags().Bool("business_events.telemetry_copy_enabled", true, "")
				require.NoError(t, cmd.ParseFlags([]string{
					"--business_events.retention=" + tt.cliRetention,
					"--business_events.telemetry_copy_enabled=" + tt.cliCopy,
				}))
				loader.SetCommand(cmd)
			}

			cfg, err := loader.GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantRetention, cfg.BusinessEvents.Retention)
			require.Equal(t, tt.wantCopy, cfg.BusinessEvents.TelemetryCopyEnabled)
		})
	}
}

func TestBusinessEventsRetentionRejectedAtConfigurationLoad(t *testing.T) {
	for _, tt := range []struct {
		name, retention string
	}{
		{name: "malformed Go duration", retention: "90d"},
		{name: "bare number is not a duration", retention: "1"},
		{name: "zero", retention: "0s"},
		{name: "negative", retention: "-1ns"},
		{name: "duration parser overflow", retention: "2562048h"},
		{name: "microsecond ceiling overflow", retention: "2562047h47m16.854775001s"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			businessEventsConfigFixture(t)
			writeBusinessEventsConfig(t, "48h", "false")
			baseline, err := NewLoader().GetConfig(context.Background())
			require.NoError(t, err, "valid JWE, memory key, authentication and file must pass before testing retention")
			require.Equal(t, 48*time.Hour, baseline.BusinessEvents.Retention)

			writeBusinessEventsConfig(t, tt.retention, "false")
			_, err = NewLoader().GetConfig(context.Background())
			require.ErrorContains(t, err, "business_events.retention")
		})
	}
}

func TestBusinessEventsRetentionNormalization(t *testing.T) {
	for _, tt := range []struct {
		name, retention string
		want            time.Duration
	}{
		{name: "one nanosecond is not truncated to zero", retention: "1ns", want: time.Microsecond},
		{name: "just under one microsecond", retention: "999ns", want: time.Microsecond},
		{name: "fractional microsecond rounds upward", retention: "1001ns", want: 2 * time.Microsecond},
		{name: "fractional millisecond rounds upward", retention: "1.000001ms", want: 1001 * time.Microsecond},
		{name: "largest representable whole microsecond", retention: "2562047h47m16.854775s", want: 9223372036854775 * time.Microsecond},
	} {
		t.Run(tt.name, func(t *testing.T) {
			businessEventsConfigFixture(t)
			writeBusinessEventsConfig(t, tt.retention, "false")
			cfg, err := NewLoader().GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.want, cfg.BusinessEvents.Retention)
		})
	}
}

func TestBusinessEventsRetentionEnvironmentCannotBypassStartupValidation(t *testing.T) {
	businessEventsConfigFixture(t)
	writeBusinessEventsConfig(t, "48h", "false")
	baseline, err := NewLoader().GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 48*time.Hour, baseline.BusinessEvents.Retention)

	t.Setenv(businessEventsRetentionEnv, "0s")
	_, err = NewLoader().GetConfig(context.Background())
	require.ErrorContains(t, err, "business_events.retention")
}

func TestBusinessEventsRetentionCLIOverrideCannotBypassStartupValidation(t *testing.T) {
	for _, retention := range []string{"0s", "-1h"} {
		t.Run(retention, func(t *testing.T) {
			businessEventsConfigFixture(t)
			writeBusinessEventsConfig(t, "48h", "false")
			t.Setenv(businessEventsRetentionEnv, "36h")
			baseline, err := NewLoader().GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, 36*time.Hour, baseline.BusinessEvents.Retention)

			cmd := &cobra.Command{Use: "config-test"}
			cmd.Flags().Duration("business_events.retention", 0, "")
			require.NoError(t, cmd.ParseFlags([]string{"--business_events.retention=" + retention}))
			loader := NewLoader()
			loader.SetCommand(cmd)
			_, err = loader.GetConfig(context.Background())
			require.ErrorContains(t, err, "business_events.retention")
			require.ErrorContains(t, err, "positive Go duration")
		})
	}
}

func TestBusinessEventsCLISelectsConfigurationFile(t *testing.T) {
	for _, tt := range []struct {
		name, environmentPath, envRetention, envCopy, cliRetention, cliCopy string
		wantRetention                                                       time.Duration
		wantCopy                                                            bool
	}{
		{name: "CLI file overrides working-directory default", wantRetention: 12 * time.Hour},
		{name: "CLI file overrides environment-selected file", environmentPath: "environment.yaml", wantRetention: 12 * time.Hour},
		{name: "environment values override selected file", environmentPath: "environment.yaml", envRetention: "6h", envCopy: "true", wantRetention: 6 * time.Hour, wantCopy: true},
		{name: "CLI values override environment and selected file", environmentPath: "environment.yaml", envRetention: "6h", envCopy: "true", cliRetention: "3h", cliCopy: "false", wantRetention: 3 * time.Hour},
	} {
		t.Run(tt.name, func(t *testing.T) {
			businessEventsConfigFixture(t)
			dir := t.TempDir()
			t.Chdir(dir)
			require.NoError(t, os.WriteFile("config.yaml", []byte("business_events:\n  retention: 48h\n  telemetry_copy_enabled: true\n"), 0o600))
			require.NoError(t, os.WriteFile("environment.yaml", []byte("business_events:\n  retention: 24h\n  telemetry_copy_enabled: true\n"), 0o600))
			require.NoError(t, os.WriteFile("selected.yaml", []byte("business_events:\n  retention: 12h\n  telemetry_copy_enabled: false\n"), 0o600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", tt.environmentPath)
			t.Setenv(businessEventsRetentionEnv, tt.envRetention)
			t.Setenv(businessEventsCopyEnv, tt.envCopy)
			cmd := &cobra.Command{Use: "config-test"}
			cmd.Flags().String("config", "", "")
			cmd.Flags().Duration("business_events.retention", 0, "")
			cmd.Flags().Bool("business_events.telemetry_copy_enabled", true, "")
			args := []string{"--config=selected.yaml"}
			if tt.cliRetention != "" {
				args = append(args, "--business_events.retention="+tt.cliRetention, "--business_events.telemetry_copy_enabled="+tt.cliCopy)
			}
			require.NoError(t, cmd.ParseFlags(args))
			loader := NewLoader()
			loader.SetCommand(cmd)
			cfg, err := loader.GetConfig(context.Background())
			require.NoError(t, err)
			require.Equal(t, tt.wantRetention, cfg.BusinessEvents.Retention)
			require.Equal(t, tt.wantCopy, cfg.BusinessEvents.TelemetryCopyEnabled)
			var paths []string
			for _, source := range loader.GetSources() {
				if source.Type == ports.SourceTypeYAML {
					paths = append(paths, source.Path)
				}
			}
			require.Equal(t, []string{filepath.Join(dir, "selected.yaml")}, paths)
			require.Equal(t, tt.environmentPath, os.Getenv("IDENTITY_BROKER_CONFIG_PATH"))
		})
	}
}
