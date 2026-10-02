package main

import (
	"context"
	"encoding/base64"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
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

func TestRootCommandAdvertisesRefreshPolicyFlags(t *testing.T) {
	var help strings.Builder
	rootCmd.SetOut(&help)
	t.Cleanup(func() { rootCmd.SetOut(nil) })
	require.NoError(t, rootCmd.Help())
	for _, name := range []string{
		"--oauth2_authorization_server.local.refresh_token_reuse_interval",
		"--oauth2_authorization_server.local.absolute_session_lifetime",
		"--oauth2_authorization_server.local.refresh_token_ttl",
	} {
		require.Contains(t, help.String(), name)
	}
}

func TestRootRefreshPolicyFlagsControlRealRefresh(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(`server:
  enduser:
    public_url: http://localhost:8000
    authentication:
      preauth:
        principal_header_name: X-Remote-User
  admin:
    authentication:
      preauth:
        principal_header_name: X-Remote-User
storage:
  backend: memory
oauth2_authorization_server:
  mode: local
  local:
    token_ttl: 5s
    refresh_token_reuse_interval: 2s
    absolute_session_lifetime: 4s
    refresh_token_ttl: 4s
third_party_oauth2:
  jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}
encryption:
  memory:
    raw_key: ${IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY}
`), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
	t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "1s")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_ABSOLUTE_SESSION_LIFETIME", "1s")
	t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "6s")
	flags := []string{
		"oauth2_authorization_server.local.refresh_token_reuse_interval",
		"oauth2_authorization_server.local.absolute_session_lifetime",
		"oauth2_authorization_server.local.refresh_token_ttl",
	}
	t.Cleanup(func() {
		for _, name := range flags {
			flag := rootCmd.Flags().Lookup(name)
			if flag != nil {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			}
		}
	})
	require.NoError(t, rootCmd.ParseFlags([]string{
		"--" + flags[0] + "=0s",
		"--" + flags[1] + "=0s",
		"--" + flags[2] + "=20s",
	}))
	loader := config.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)

	ctx := context.Background()
	broker, err := bootstrap.NewRefreshEnvironment(cfg, nil)
	require.NoError(t, err)
	t.Cleanup(broker.Close)
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, broker.Storage))
	client, err := broker.AddClient(ctx, id.Principal("cli-policy@example.test"), true)
	require.NoError(t, err)
	issued, err := helpers.AuthorizeRefreshSession(ctx, broker.Enduser.BaseURL(), *client, "read offline_access")
	require.NoError(t, err)
	first, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, issued.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, first.Status, first.Error)

	// Environment's 1s absolute limit must not end a session when CLI selects 0s.
	time.Sleep(1200 * time.Millisecond)
	stillActive, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *client, first.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, stillActive.Status, stillActive.Error)

	strictClient, err := broker.AddClient(ctx, id.Principal("cli-reuse@example.test"), true)
	require.NoError(t, err)
	strictSession, err := helpers.AuthorizeRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, "read offline_access")
	require.NoError(t, err)
	strictRotation, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, strictSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, strictRotation.Status, strictRotation.Error)
	retry, err := helpers.RotateRefreshSession(ctx, broker.Enduser.BaseURL(), *strictClient, strictSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "invalid_grant", retry.Error, "explicit CLI 0s must disable retry despite environment and YAML")

	require.NoError(t, rootCmd.ParseFlags([]string{"--" + flags[2] + "=2s"}))
	idleLoader := config.NewLoader()
	idleLoader.SetCommand(rootCmd)
	idleConfig, err := idleLoader.GetConfig(ctx)
	require.NoError(t, err)
	idleBroker, err := bootstrap.NewRefreshEnvironment(idleConfig, nil)
	require.NoError(t, err)
	t.Cleanup(idleBroker.Close)
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, idleBroker.Storage))
	second, err := idleBroker.AddClient(ctx, id.Principal("cli-idle@example.test"), true)
	require.NoError(t, err)
	newSession, err := helpers.AuthorizeRefreshSession(ctx, idleBroker.Enduser.BaseURL(), *second, "read offline_access")
	require.NoError(t, err)
	time.Sleep(2200 * time.Millisecond)
	expired, err := helpers.RotateRefreshSession(ctx, idleBroker.Enduser.BaseURL(), *second, newSession.Tokens.RefreshToken, "")
	require.NoError(t, err)
	require.Equal(t, "invalid_grant", expired.Error, "CLI inactivity must supersede environment and YAML")
}

func TestRootRefreshPolicyFlagsRejectBadValues(t *testing.T) {
	for _, tt := range []struct {
		flag, value, key string
	}{
		{"refresh_token_reuse_interval", "0", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "30", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "not-a-duration", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "-1s", "refresh_token_reuse_interval"},
		{"refresh_token_reuse_interval", "2562048h", "refresh_token_reuse_interval"},
		{"absolute_session_lifetime", "0", "absolute_session_lifetime"},
		{"absolute_session_lifetime", "-1s", "absolute_session_lifetime"},
		{"absolute_session_lifetime", "2562048h", "absolute_session_lifetime"},
		{"refresh_token_ttl", "0", "refresh_token_ttl"},
		{"refresh_token_ttl", "-1s", "refresh_token_ttl"},
		{"refresh_token_ttl", "2562048h", "refresh_token_ttl"},
	} {
		t.Run(tt.flag+"="+tt.value, func(t *testing.T) {
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
			cmd := rootCmd
			name := "oauth2_authorization_server.local." + tt.flag
			flag := cmd.PersistentFlags().Lookup(name)
			require.NotNil(t, flag)
			t.Cleanup(func() {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			})
			require.NoError(t, cmd.ParseFlags([]string{"--" + name + "=" + tt.value}))
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
			loader := config.NewLoader()
			loader.SetCommand(cmd)
			_, err := loader.GetConfig(context.Background())
			require.ErrorContains(t, err, tt.key)
		})
	}
}

func TestRootRefreshPolicyFlagsRejectInvalidRelations(t *testing.T) {
	for _, tt := range []struct {
		name, flag, value, env, envValue, field string
	}{
		{"reuse equals access lifetime", "refresh_token_reuse_interval", "30s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_TOKEN_TTL", "30s", "refresh_token_reuse_interval"},
		{"reuse equals inactivity lifetime", "refresh_token_reuse_interval", "2s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_TTL", "2s", "refresh_token_reuse_interval"},
		{"absolute equals reuse", "absolute_session_lifetime", "30s", "IDENTITY_BROKER_OAUTH2_AUTH_SERVER_LOCAL_REFRESH_TOKEN_REUSE_INTERVAL", "30s", "absolute_session_lifetime"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			name := "oauth2_authorization_server.local." + tt.flag
			flag := rootCmd.PersistentFlags().Lookup(name)
			require.NotNil(t, flag)
			t.Cleanup(func() {
				_ = flag.Value.Set(flag.DefValue)
				flag.Changed = false
			})
			require.NoError(t, rootCmd.ParseFlags([]string{"--" + name + "=" + tt.value}))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", filepath.Join(t.TempDir(), "absent.yaml"))
			t.Setenv("IDENTITY_BROKER_OAUTH2_AUTH_SERVER_MODE", "local")
			t.Setenv("IDENTITY_BROKER_SERVER_ENDUSER_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_SERVER_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME", "X-Remote-User")
			t.Setenv("IDENTITY_BROKER_JWE_SIGNING_KEY", base64.StdEncoding.EncodeToString([]byte("test-32-byte-key-must-be-exact-x")))
			t.Setenv("IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY", fixtures.TestKEKMaterialDeterministic())
			t.Setenv(tt.env, tt.envValue)
			loader := config.NewLoader()
			loader.SetCommand(rootCmd)
			_, err := loader.GetConfig(context.Background())
			require.ErrorContains(t, err, tt.field)
		})
	}
}
