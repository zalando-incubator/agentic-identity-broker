package main

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestRootCommandRegistersRequestContextFlags(t *testing.T) {
	for _, name := range []string{
		"request_context.trusted_proxy.enabled",
		"request_context.trusted_proxy.forwarded_header",
		"request_context.trace.response_enabled",
	} {
		if rootCmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("missing request-context CLI flag %q", name)
		}
	}
}

func TestRootCommandLoadsBusinessEventFlagsOverEnvironmentAndFile(t *testing.T) {
	for _, name := range []string{"business_events.retention", "business_events.telemetry_copy_enabled"} {
		flag := rootCmd.PersistentFlags().Lookup(name)
		require.NotNil(t, flag, "missing operator CLI flag %q", name)
		oldValue, oldChanged := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			_ = flag.Value.Set(oldValue)
			flag.Changed = oldChanged
		})
	}

	// A valid startup fixture avoids unrelated key and authentication failures.
	key := base64.StdEncoding.EncodeToString([]byte("0123456789abcdef0123456789abcdef"))
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", key)
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", key)
	t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_RETENTION", "36h")
	t.Setenv("IDENTITY_BROKER_BUSINESS_EVENTS_TELEMETRY_COPY_ENABLED", "true")
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("business_events:\n  retention: 48h\n  telemetry_copy_enabled: true\n"), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", configPath)

	require.NoError(t, rootCmd.ParseFlags([]string{
		"--business_events.retention=12h",
		"--business_events.telemetry_copy_enabled=false",
	}))
	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, 12*time.Hour, cfg.BusinessEvents.Retention)
	require.False(t, cfg.BusinessEvents.TelemetryCopyEnabled)

	for _, source := range loader.GetSources() {
		if source.Type == ports.SourceTypeCLI {
			require.ElementsMatch(t, []string{
				"business_events.retention",
				"business_events.telemetry_copy_enabled",
			}, source.Keys)
			return
		}
	}
	t.Fatal("explicit business-event CLI flags missing from startup source metadata")
}
