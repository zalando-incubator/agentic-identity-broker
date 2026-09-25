package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestLoadWithCommand_ApprovalFlagsOverrideEnvironmentAndFile(t *testing.T) {
	policyPath := filepath.Join(t.TempDir(), "policy.rego")
	require.NoError(t, os.WriteFile(policyPath, []byte(`package aib.extproc.authz
result := {"action": "deny"}`), 0o600))
	configPath := filepath.Join(t.TempDir(), "extproc.yaml")
	require.NoError(t, os.WriteFile(configPath, []byte("grpc:\n  bind: 127.0.0.1\n  port: 50051\noauth2:\n  token_endpoint: https://broker.example.com/oauth2/token\n  issuer: https://issuer.example.com\n  client_id: test\n  client_secret: test\n  tls:\n    allow_http: false\nauthorization:\n  enabled: true\n  default_decision: deny\n  evaluation_timeout: 1s\n  max_body_size: 1\n  policy:\n    path: "+policyPath+"\n    package: aib.extproc.authz\n    decision: result\ntool_approvals:\n  enabled: true\n  url: https://file.example.com\n  long_poll_timeout_seconds: 30\n  approval_cache_idle_ttl: 5m\n  request_timeout: 5s\nsessions:\n  extraction:\n    http_header: Mcp-Session-Id\n"), 0o600))
	t.Setenv("EXTPROC_CONFIG_PATH", configPath)
	t.Setenv("EXTPROC_TOOL_APPROVALS_URL", "https://environment.example.com")
	command := &cobra.Command{Use: "test"}
	config.RegisterFlags(command)
	require.NoError(t, command.Flags().Set("tool_approvals.url", "https://flag.example.com/"))
	loaded, err := config.LoadWithCommand(command)
	require.NoError(t, err)
	require.Equal(t, "https://flag.example.com", loaded.ToolApprovals.URL)
}
