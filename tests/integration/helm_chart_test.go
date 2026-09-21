package integration

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func renderHelmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}

	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)

	args := []string{
		"template",
		"broker",
		chartPath,
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token",
	}
	args = append(args, extraArgs...)

	cmd := exec.Command("helm", args...)

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	require.NoError(t, err, stderr.String())

	return stdout.String()
}

func TestHelmTemplate_ProxyUpstreamTimeout(t *testing.T) {
	t.Run("quotes valid Go duration strings", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--set-string", "broker.oauth2AuthorizationServer.proxy.upstreamTimeout=30s",
		)

		require.Contains(t, output, `upstream_timeout: "30s"`)
	})

	t.Run("omits upstream_timeout when not set", func(t *testing.T) {
		output := renderHelmTemplate(t)

		require.NotContains(t, output, "upstream_timeout:")
	})
}

func TestHelmTemplate_LocalModeLeavesTokenExchangeUnconfigured(t *testing.T) {
	output := renderHelmTemplate(t,
		"--set", "broker.oauth2AuthorizationServer.mode=local",
	)

	require.Contains(t, output, `principal_expression: ""`)
	require.Contains(t, output, `agent_id_expression: ""`)
	require.Contains(t, output, `expression: ""`)
	require.NotContains(t, output, `principal_expression: "subject_token.sub"`)
	require.NotContains(t, output, `agent_id_expression: "resolveAgentIdByClientId(subject_token.azp)"`)
}
