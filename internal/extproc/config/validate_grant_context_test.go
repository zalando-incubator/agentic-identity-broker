package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/configutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

func validGrantContextConfig(t *testing.T) *config.Config {
	t.Helper()
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"deny\"}\n"), 0o600))
	cfg := validConfig()
	cfg.Authorization = config.AuthorizationConfig{Enabled: true, Policy: config.PolicyConfig{Path: policyPath, Package: "aib.extproc.authz", Decision: "result"},
		DefaultDecision: "deny", EvaluationTimeout: time.Second, MaxBodySize: 1}
	cfg.GrantContext = config.GrantContextConfig{Enabled: true, RequestTimeout: 5 * time.Second, MaxStaleness: time.Minute}
	return cfg
}

func TestValidateGrantContext(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*config.Config)
		contains string
	}{
		{"requires authorization", func(c *config.Config) { c.Authorization.Enabled = false }, "grant_context.enabled requires authorization.enabled"},
		{"rejects relative PDP", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "/broker" }, "grant_context.policy_decision_point must be an absolute URL"},
		{"rejects port without hostname", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "https://:443" }, "grant_context.policy_decision_point must be an absolute URL"},
		{"rejects userinfo", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "https://user:pw@broker.example.com" }, "must be an absolute URL"},
		{"rejects query", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "https://broker.example.com?x=1" }, "must be an absolute URL"},
		{"rejects fragment", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "https://broker.example.com#x" }, "must be an absolute URL"},
		{"rejects other schemes", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "ftp://broker.example.com" }, "must be an absolute URL"},
		{"rejects http without allow_http", func(c *config.Config) { c.GrantContext.PolicyDecisionPoint = "http://broker.example.com" }, "grant_context.policy_decision_point must use https"},
		{"rejects nonpositive timeout", func(c *config.Config) { c.GrantContext.RequestTimeout = 0 }, "grant_context.request_timeout must be positive"},
		{"rejects nonpositive staleness", func(c *config.Config) { c.GrantContext.MaxStaleness = -time.Second }, "grant_context.max_staleness must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validGrantContextConfig(t)
			tt.mutate(cfg)
			err := config.Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
			assert.NotContains(t, err.Error(), "supersecret")
		})
	}

	t.Run("defaults PDP to the token endpoint origin including port", func(t *testing.T) {
		cfg := validGrantContextConfig(t)
		cfg.OAuth2.TokenEndpoint = "https://broker.example.com:8443/oauth2/token"
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "https://broker.example.com:8443", cfg.GrantContext.PolicyDecisionPoint)
	})

	t.Run("defaults PDP to the token endpoint origin preserving an escaped IPv6 zone", func(t *testing.T) {
		cfg := validGrantContextConfig(t)
		cfg.OAuth2.TokenEndpoint = "https://[fe80::1%25eth0]:8443/oauth2/token"
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "https://[fe80::1%25eth0]:8443", cfg.GrantContext.PolicyDecisionPoint)
	})

	t.Run("keeps an explicit path-prefixed PDP", func(t *testing.T) {
		cfg := validGrantContextConfig(t)
		cfg.GrantContext.PolicyDecisionPoint = "https://example.com/broker"
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "https://example.com/broker", cfg.GrantContext.PolicyDecisionPoint)
	})

	t.Run("keeps an IPv6 PDP with an explicit port", func(t *testing.T) {
		cfg := validGrantContextConfig(t)
		cfg.GrantContext.PolicyDecisionPoint = "https://[2001:db8::1]:8443"
		require.NoError(t, config.Validate(cfg))
	})

	t.Run("permits local http under allow_http", func(t *testing.T) {
		cfg := validGrantContextConfig(t)
		cfg.OAuth2.TLS.AllowHTTP = true
		cfg.GrantContext.PolicyDecisionPoint = "http://localhost:8000"
		require.NoError(t, config.Validate(cfg))
	})

	t.Run("disabled mode skips every rule", func(t *testing.T) {
		cfg := validConfig()
		cfg.GrantContext = config.GrantContextConfig{PolicyDecisionPoint: "not a url"}
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "not a url", cfg.GrantContext.PolicyDecisionPoint)
	})
}

// clearExtProcEnv blanks inherited EXTPROC_* variables for the test; the loader treats empty values as unset.
func clearExtProcEnv(t *testing.T) {
	t.Helper()
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "EXTPROC_") {
			t.Setenv(name, "")
		}
	}
}

func TestLoadFromViper_GrantContextDefaults(t *testing.T) {
	clearExtProcEnv(t)
	v := configutil.NewViper()
	v.Set(configutil.Key("oauth2", "token_endpoint"), "https://idp.example.com/oauth2/token")
	v.Set(configutil.Key("oauth2", "issuer"), "https://idp.example.com")
	v.Set(configutil.Key("oauth2", "client_id"), "test-client")
	v.Set(configutil.Key("oauth2", "client_secret"), "test-secret")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)
	assert.Equal(t, config.GrantContextConfig{RequestTimeout: 5 * time.Second, MaxStaleness: 60 * time.Second}, cfg.GrantContext)
}

func TestLoadWithCommand_GrantContextPrecedence(t *testing.T) {
	clearExtProcEnv(t)
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"deny\"}\n"), 0o600))
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyPath)
	t.Setenv("EXTPROC_GRANT_CONTEXT_ENABLED", "true")
	t.Setenv("EXTPROC_GRANT_CONTEXT_POLICY_DECISION_POINT", "https://env-pdp.example.com")
	t.Setenv("EXTPROC_GRANT_CONTEXT_REQUEST_TIMEOUT", "3s")
	t.Setenv("EXTPROC_GRANT_CONTEXT_MAX_STALENESS", "20s")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("grant_context.policy_decision_point", "https://flag-pdp.example.com"))
	require.NoError(t, cmd.Flags().Set("grant_context.max_staleness", "10s"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)
	assert.True(t, cfg.GrantContext.Enabled)
	assert.Equal(t, "https://flag-pdp.example.com", cfg.GrantContext.PolicyDecisionPoint, "flags override env")
	assert.Equal(t, 3*time.Second, cfg.GrantContext.RequestTimeout, "env overrides default")
	assert.Equal(t, 10*time.Second, cfg.GrantContext.MaxStaleness)
}
