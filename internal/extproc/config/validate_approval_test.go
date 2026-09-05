package config_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateApprovalConfiguration(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte(`package aib.extproc.authz
result := {"action": "deny"}`), 0o600))

	valid := func() *config.Config {
		cfg := validConfig()
		cfg.Authorization = config.AuthorizationConfig{Enabled: true, Policy: config.PolicyConfig{Path: policyPath, Package: "aib.extproc.authz", Decision: "result"}, DefaultDecision: "deny", EvaluationTimeout: time.Second, MaxBodySize: 1}
		cfg.ToolApprovals = config.ToolApprovalsConfig{Enabled: true, URL: "https://broker.example.com/", LongPollTimeoutSeconds: 30, ApprovalCacheIdleTTL: time.Minute, RequestTimeout: time.Second}
		cfg.Sessions.Extraction.HTTPHeader = "Mcp-Session-Id"
		return cfg
	}

	tests := []struct {
		name     string
		mutate   func(*config.Config)
		contains string
	}{
		{"requires authorization", func(c *config.Config) { c.Authorization.Enabled = false }, "requires authorization.enabled"},
		{"requires URL", func(c *config.Config) { c.ToolApprovals.URL = "" }, "tool_approvals.url"},
		{"rejects HTTP in production", func(c *config.Config) { c.ToolApprovals.URL = "http://broker.example.com" }, "must use https"},
		{"bounds poll timeout", func(c *config.Config) { c.ToolApprovals.LongPollTimeoutSeconds = 121 }, "long_poll_timeout_seconds"},
		{"requires idle TTL", func(c *config.Config) { c.ToolApprovals.ApprovalCacheIdleTTL = 0 }, "approval_cache_idle_ttl"},
		{"requires request timeout", func(c *config.Config) { c.ToolApprovals.RequestTimeout = 0 }, "request_timeout"},
		{"validates session header", func(c *config.Config) { c.Sessions.Extraction.HTTPHeader = "bad header" }, "http_header"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid()
			tt.mutate(cfg)
			err := config.Validate(cfg)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.contains)
		})
	}
	cfg := valid()
	require.NoError(t, config.Validate(cfg))
	assert.Equal(t, "https://broker.example.com", cfg.ToolApprovals.URL)
}
