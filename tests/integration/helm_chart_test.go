package integration

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
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

func TestHelmTemplate_ManagedSecretChecksumsChangeWithKeys(t *testing.T) {
	render := func(key string) string {
		return renderHelmTemplate(t,
			"--set-string", "broker.thirdPartyOauth2.jweSigningKey="+key,
			"--set-string", "broker.encryption.memory.rawKey="+key,
		)
	}
	checksum := func(output, name string) string {
		matches := regexp.MustCompile(name + `: ([0-9a-f]{64})`).FindStringSubmatch(output)
		require.Len(t, matches, 2)
		return matches[1]
	}

	first := render("0123456789abcdef0123456789abcdef")
	second := render("abcdef0123456789abcdef0123456789")
	require.NotEqual(t, checksum(first, "checksum/jwe-secret"), checksum(second, "checksum/jwe-secret"))
	require.NotEqual(t, checksum(first, "checksum/encryption-secret"), checksum(second, "checksum/encryption-secret"))
}
