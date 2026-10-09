package integration

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func renderHelmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()
	output, stderr, err := runHelmTemplate(t, extraArgs...)
	require.NoError(t, err, stderr)
	return output
}

func runHelmTemplate(t *testing.T, extraArgs ...string) (string, string, error) {
	t.Helper()
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)
	args := []string{
		"template", "broker", chartPath,
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token",
	}
	args = append(args, extraArgs...)
	cmd := exec.Command("helm", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	return stdout.String(), stderr.String(), err
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

func TestHelmTemplate_AdminPortRequiresTrustedProxy(t *testing.T) {
	type policy struct {
		Kind string `json:"kind"`
		Spec struct {
			PodSelector struct {
				MatchLabels map[string]string `json:"matchLabels"`
			} `json:"podSelector"`
			PolicyTypes []string `json:"policyTypes"`
			Ingress     []struct {
				From []struct {
					NamespaceSelector struct {
						MatchLabels map[string]string `json:"matchLabels"`
					} `json:"namespaceSelector"`
					PodSelector struct {
						MatchLabels map[string]string `json:"matchLabels"`
					} `json:"podSelector"`
				} `json:"from"`
				Ports []struct {
					Port int `json:"port"`
				} `json:"ports"`
			} `json:"ingress"`
		} `json:"spec"`
	}
	renderPolicy := func(t *testing.T, extraArgs ...string) policy {
		t.Helper()
		args := append([]string{"--show-only", "templates/networkpolicy.yaml"}, extraArgs...)
		output := renderHelmTemplate(t, args...)
		var result policy
		require.NoError(t, yaml.Unmarshal([]byte(output), &result))
		return result
	}

	defaultPolicy := renderPolicy(t)
	require.Equal(t, "NetworkPolicy", defaultPolicy.Kind)
	require.Equal(t, []string{"Ingress"}, defaultPolicy.Spec.PolicyTypes)
	require.NotEmpty(t, defaultPolicy.Spec.PodSelector.MatchLabels)
	require.Len(t, defaultPolicy.Spec.Ingress, 1, "admin ingress must be denied until a proxy is selected")
	require.Empty(t, defaultPolicy.Spec.Ingress[0].From, "end-user access can originate from any namespace")
	require.Len(t, defaultPolicy.Spec.Ingress[0].Ports, 1)
	require.Equal(t, 8000, defaultPolicy.Spec.Ingress[0].Ports[0].Port)

	trustedPolicy := renderPolicy(t,
		"--set", "networkPolicy.adminProxy.namespaceLabels.team=platform",
		"--set", "networkPolicy.adminProxy.podLabels.app=admin-proxy",
	)
	require.Len(t, trustedPolicy.Spec.Ingress, 2)
	require.Len(t, trustedPolicy.Spec.Ingress[1].Ports, 1)
	require.Equal(t, 14000, trustedPolicy.Spec.Ingress[1].Ports[0].Port)
	require.Len(t, trustedPolicy.Spec.Ingress[1].From, 1)
	require.Equal(t, map[string]string{"team": "platform"}, trustedPolicy.Spec.Ingress[1].From[0].NamespaceSelector.MatchLabels)
	require.Equal(t, map[string]string{"app": "admin-proxy"}, trustedPolicy.Spec.Ingress[1].From[0].PodSelector.MatchLabels)
}

func TestHelmTemplate_AdminPolicyRejectsListenerPortMismatch(t *testing.T) {
	for _, tc := range []struct {
		name, expectedError string
		settings            []string
	}{
		{name: "end-user Service port exposes admin listener", settings: []string{"service.ports.enduser=14000"}, expectedError: "enduser port must match"},
		{name: "admin listener differs from Service port", settings: []string{"broker.server.admin.port=14001"}, expectedError: "admin port must match"},
		{name: "disabled policy does not permit mismatched ports", settings: []string{"networkPolicy.enabled=false", "service.ports.enduser=14000"}, expectedError: "enduser port must match"},
		{name: "identical listener ports are rejected", settings: []string{"service.ports.enduser=14000", "broker.server.enduser.port=14000"}, expectedError: "enduser and admin ports must differ"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := make([]string, 0, 2*len(tc.settings))
			for _, setting := range tc.settings {
				args = append(args, "--set", setting)
			}
			_, stderr, err := runHelmTemplate(t, args...)
			require.Error(t, err, "a mismatched listener must not produce an ingress policy")
			require.Contains(t, stderr, tc.expectedError)
		})
	}
}
