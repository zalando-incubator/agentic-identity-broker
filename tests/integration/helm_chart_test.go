package integration

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func runHelmTemplate(t *testing.T, extraArgs ...string) (string, error) {
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

	output, err := exec.Command("helm", args...).CombinedOutput()
	return string(output), err
}

func renderHelmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()
	output, err := runHelmTemplate(t, extraArgs...)
	require.NoError(t, err, output)
	return output
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

func TestHelmTemplate_ManagedKeysUseBase64StringData(t *testing.T) {
	key := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	output := renderHelmTemplate(t,
		"--set-string", "broker.thirdPartyOauth2.jweSigningKeyBase64="+key,
		"--set-string", "broker.encryption.memory.rawKey="+key,
	)

	require.Contains(t, output, "stringData:\n  signing-key: \""+key+"\"")
	require.Contains(t, output, "stringData:\n  memory-raw-key: \""+key+"\"")
}

func TestHelmTemplate_DCRClientName(t *testing.T) {
	t.Run("default omits optional client_name", func(t *testing.T) {
		output := renderHelmTemplate(t)
		require.NotContains(t, output, "client_name:")
	})

	t.Run("configured name is quoted in the rendered broker configuration", func(t *testing.T) {
		output := renderHelmTemplate(t, "--set-string", "broker.thirdPartyOauth2.clientName=Example Platform: 01")
		require.Contains(t, output, `client_name: "Example Platform: 01"`)
	})

	t.Run("schema rejects a non-string name", func(t *testing.T) {
		output, err := runHelmTemplate(t, "--set", "broker.thirdPartyOauth2.clientName=true")
		require.Error(t, err, output)
		require.Contains(t, output, "clientName")
		require.Contains(t, output, "string")
	})
}
