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

func TestHelmTemplate_ManagedKeysUseBase64StringData(t *testing.T) {
	key := "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="
	output := renderHelmTemplate(t,
		"--set-string", "broker.thirdPartyOauth2.jweSigningKeyBase64="+key,
		"--set-string", "broker.encryption.memory.rawKey="+key,
	)

	require.Contains(t, output, "stringData:\n  signing-key: \""+key+"\"")
	require.Contains(t, output, "stringData:\n  memory-raw-key: \""+key+"\"")
}

func TestHelmTemplate_CIMDClientAuthentication(t *testing.T) {
	output := renderHelmTemplate(t,
		"--set-string", "broker.oauth2AuthorizationServer.cimd.clientAuthentication.keySetCacheTtl=2h",
		"--set-string", "broker.oauth2AuthorizationServer.cimd.clientAuthentication.keySetMinRefetchInterval=7m",
		"--set", "broker.oauth2AuthorizationServer.cimd.clientAuthentication.keySetMaxBytes=32768",
		"--set-string", "broker.oauth2AuthorizationServer.cimd.clientAuthentication.keySetFetchTimeout=3s",
	)

	require.Contains(t, output, "client_authentication:\n          key_set_cache_ttl: \"2h\"")
	require.Contains(t, output, "key_set_min_refetch_interval: \"7m\"")
	require.Contains(t, output, "key_set_max_bytes: 32768")
	require.Contains(t, output, "key_set_fetch_timeout: \"3s\"")
}
