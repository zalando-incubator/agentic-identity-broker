package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/configutil"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

func validDelegationConfig(t *testing.T) *config.Config {
	t.Helper()
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"deny\"}\n"), 0o600))
	cfg := validConfig()
	cfg.Authorization = config.AuthorizationConfig{Enabled: true, Policy: config.PolicyConfig{Path: policyPath, Package: "aib.extproc.authz", Decision: "result"},
		DefaultDecision: "deny", EvaluationTimeout: time.Second, MaxBodySize: 1}
	cfg.DelegationContext = config.DelegationContextConfig{Enabled: true, RequestTimeout: 5 * time.Second, MaxStaleness: time.Minute}
	return cfg
}

func TestValidateDelegationContext(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*config.Config)
		contains string
	}{
		{"requires authorization", func(c *config.Config) { c.Authorization.Enabled = false }, "delegation_context.enabled requires authorization.enabled"},
		{"rejects relative PDP", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "/broker" }, "delegation_context.policy_decision_point must be an absolute URL"},
		{"rejects userinfo", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "https://user:pw@broker.example.com" }, "must be an absolute URL"},
		{"rejects query", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "https://broker.example.com?x=1" }, "must be an absolute URL"},
		{"rejects fragment", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "https://broker.example.com#x" }, "must be an absolute URL"},
		{"rejects other schemes", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "ftp://broker.example.com" }, "must be an absolute URL"},
		{"rejects http without allow_http", func(c *config.Config) { c.DelegationContext.PolicyDecisionPoint = "http://broker.example.com" }, "delegation_context.policy_decision_point must use https"},
		{"rejects nonpositive timeout", func(c *config.Config) { c.DelegationContext.RequestTimeout = 0 }, "delegation_context.request_timeout must be positive"},
		{"rejects nonpositive staleness", func(c *config.Config) { c.DelegationContext.MaxStaleness = -time.Second }, "delegation_context.max_staleness must be positive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validDelegationConfig(t)
			tt.mutate(cfg)
			err := config.Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
			assert.NotContains(t, err.Error(), "supersecret")
		})
	}

	t.Run("defaults PDP to the token endpoint origin including port", func(t *testing.T) {
		cfg := validDelegationConfig(t)
		cfg.OAuth2.TokenEndpoint = "https://broker.example.com:8443/oauth2/token"
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "https://broker.example.com:8443", cfg.DelegationContext.PolicyDecisionPoint)
	})

	t.Run("keeps an explicit path-prefixed PDP", func(t *testing.T) {
		cfg := validDelegationConfig(t)
		cfg.DelegationContext.PolicyDecisionPoint = "https://example.com/broker"
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "https://example.com/broker", cfg.DelegationContext.PolicyDecisionPoint)
	})

	t.Run("permits local http under allow_http", func(t *testing.T) {
		cfg := validDelegationConfig(t)
		cfg.OAuth2.TLS.AllowHTTP = true
		cfg.DelegationContext.PolicyDecisionPoint = "http://localhost:8000"
		require.NoError(t, config.Validate(cfg))
	})

	t.Run("disabled mode skips every rule", func(t *testing.T) {
		cfg := validConfig()
		cfg.DelegationContext = config.DelegationContextConfig{PolicyDecisionPoint: "not a url"}
		require.NoError(t, config.Validate(cfg))
		assert.Equal(t, "not a url", cfg.DelegationContext.PolicyDecisionPoint)
	})
}

func TestLoadFromViper_DelegationContextDefaults(t *testing.T) {
	v := configutil.NewViper()
	v.Set(configutil.Key("oauth2", "token_endpoint"), "https://idp.example.com/oauth2/token")
	v.Set(configutil.Key("oauth2", "issuer"), "https://idp.example.com")
	v.Set(configutil.Key("oauth2", "client_id"), "test-client")
	v.Set(configutil.Key("oauth2", "client_secret"), "test-secret")

	cfg, err := config.LoadFromViper(v)
	require.NoError(t, err)
	assert.Equal(t, config.DelegationContextConfig{RequestTimeout: 5 * time.Second, MaxStaleness: 60 * time.Second}, cfg.DelegationContext)
}

func TestLoadWithCommand_DelegationContextPrecedence(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte("package aib.extproc.authz\nresult := {\"action\": \"deny\"}\n"), 0o600))
	t.Setenv("EXTPROC_OAUTH2_TOKEN_ENDPOINT", "https://env.example.com/oauth2/token")
	t.Setenv("EXTPROC_OAUTH2_ISSUER", "https://env.example.com")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_ID", "env-client")
	t.Setenv("EXTPROC_OAUTH2_CLIENT_SECRET", "env-secret")
	t.Setenv("EXTPROC_AUTHORIZATION_ENABLED", "true")
	t.Setenv("EXTPROC_AUTHORIZATION_POLICY_PATH", policyPath)
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_ENABLED", "true")
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_POLICY_DECISION_POINT", "https://env-pdp.example.com")
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_REQUEST_TIMEOUT", "3s")
	t.Setenv("EXTPROC_DELEGATION_CONTEXT_MAX_STALENESS", "20s")

	cmd := newTestCommand()
	require.NoError(t, cmd.Flags().Set("delegation_context.policy_decision_point", "https://flag-pdp.example.com"))
	require.NoError(t, cmd.Flags().Set("delegation_context.max_staleness", "10s"))

	cfg, err := config.LoadWithCommand(cmd)
	require.NoError(t, err)
	assert.True(t, cfg.DelegationContext.Enabled)
	assert.Equal(t, "https://flag-pdp.example.com", cfg.DelegationContext.PolicyDecisionPoint, "flags override env")
	assert.Equal(t, 3*time.Second, cfg.DelegationContext.RequestTimeout, "env overrides default")
	assert.Equal(t, 10*time.Second, cfg.DelegationContext.MaxStaleness)
}
