package integration

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const trustMaterialValues = `
broker:
  security:
    trustMaterial:
      directory: /etc/agentic-identity-broker/trust
      sources:
        - secret:
            name: agentgateway-jwks
            items:
              - key: jwks.json
                path: agentgateway-jwks.json
        - configMap:
            name: mesh-jwks
            items:
              - key: jwks.json
                path: mesh-jwks.json
  tokenExchange:
    clientAssertion:
      issuerUri: https://agentgateway.example.com
      jwksFile: /etc/agentic-identity-broker/trust/agentgateway-jwks.json
`

func writeHelmValues(t *testing.T, values string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(path, []byte(values), 0o600))
	return path
}

// renderedManifests decodes every document of a helm template output.
func renderedManifests(t *testing.T, output string) []map[string]any {
	t.Helper()
	var manifests []map[string]any
	decoder := yaml.NewDecoder(bytes.NewBufferString(output))
	for {
		var doc map[string]any
		if err := decoder.Decode(&doc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err, "decode rendered manifest")
		}
		if doc != nil {
			manifests = append(manifests, doc)
		}
	}
	return manifests
}

func manifestOfKind(t *testing.T, manifests []map[string]any, kind, nameSuffix string) map[string]any {
	t.Helper()
	for _, manifest := range manifests {
		metadata, _ := manifest["metadata"].(map[string]any)
		name, _ := metadata["name"].(string)
		if manifest["kind"] == kind && strings.HasSuffix(name, nameSuffix) {
			return manifest
		}
	}
	t.Fatalf("no %s manifest ending in %q", kind, nameSuffix)
	return nil
}

func dig(t *testing.T, value any, path ...any) any {
	t.Helper()
	for _, step := range path {
		switch key := step.(type) {
		case string:
			m, ok := value.(map[string]any)
			require.True(t, ok, "expected a map at %v", key)
			value = m[key]
		case int:
			s, ok := value.([]any)
			require.True(t, ok && key < len(s), "expected a list with index %d", key)
			value = s[key]
		}
	}
	return value
}

func TestRenderChartConcurrentValues(t *testing.T) {
	realHelm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is not installed")
	}
	realHelm, err = filepath.Abs(realHelm)
	require.NoError(t, err)

	root := t.TempDir()
	wrapperDir := filepath.Join(root, "helm")
	require.NoError(t, os.Mkdir(wrapperDir, 0o755))
	const wrapper = `#!/bin/sh
set -eu
printf 'x\n' >> "$HELM_BARRIER"
attempt=0
while [ "$(wc -l < "$HELM_BARRIER")" -lt 2 ]; do
    attempt=$((attempt + 1))
    [ "$attempt" -lt 500 ] || exit 1
    sleep 0.01
done
exec "$REAL_HELM" "$@"
`
	require.NoError(t, os.WriteFile(filepath.Join(wrapperDir, "helm"), []byte(wrapper), 0o755))
	t.Setenv("REAL_HELM", realHelm)
	t.Setenv("HELM_BARRIER", filepath.Join(root, "barrier"))
	t.Setenv("PATH", wrapperDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	type result struct {
		directory string
		chart     *helpers.RenderedChart
		err       error
	}
	results := make(chan result, 2)
	for _, directory := range []string{"/etc/trust-a", "/etc/trust-b"} {
		go func(directory string) {
			chart, err := helpers.RenderChart(fmt.Sprintf("broker:\n  security:\n    trustMaterial:\n      directory: %s\n", directory))
			results <- result{directory: directory, chart: chart, err: err}
		}(directory)
	}
	var rendered [2]result
	for i := range rendered {
		rendered[i] = <-results
	}
	for _, got := range rendered {
		require.NoError(t, got.err, "render %s", got.directory)
		var config map[string]any
		require.NoError(t, yaml.Unmarshal([]byte(got.chart.ConfigYAML), &config))
		assert.Equal(t, got.directory, dig(t, config, "security", "trust_material_dir"))
	}
}

func TestHelmTemplate_TrustMaterial(t *testing.T) {
	output := renderHelmTemplate(t, "-f", writeHelmValues(t, trustMaterialValues))
	manifests := renderedManifests(t, output)

	deployment := manifestOfKind(t, manifests, "Deployment", "")
	podSpec := dig(t, deployment, "spec", "template", "spec").(map[string]any)

	var trustVolume map[string]any
	for _, volume := range podSpec["volumes"].([]any) {
		if v := volume.(map[string]any); v["name"] == "trust-material" {
			trustVolume = v
		}
	}
	require.NotNil(t, trustVolume, "the trust-material volume is rendered")
	sources := dig(t, trustVolume, "projected", "sources").([]any)
	require.Len(t, sources, 2)
	assert.Equal(t, "agentgateway-jwks", dig(t, sources[0], "secret", "name"))
	assert.Equal(t, "agentgateway-jwks.json", dig(t, sources[0], "secret", "items", 0, "path"))
	assert.Equal(t, "mesh-jwks", dig(t, sources[1], "configMap", "name"))

	var trustMount map[string]any
	for _, container := range podSpec["containers"].([]any) {
		for _, mount := range dig(t, container, "volumeMounts").([]any) {
			if m := mount.(map[string]any); m["name"] == "trust-material" {
				trustMount = m
			}
		}
	}
	require.NotNil(t, trustMount, "the trust-material volume is mounted")
	assert.Equal(t, "/etc/agentic-identity-broker/trust", trustMount["mountPath"])
	assert.Equal(t, true, trustMount["readOnly"])
	assert.NotContains(t, trustMount, "subPath", "a subPath mount would never receive Secret updates")

	configMap := manifestOfKind(t, manifests, "ConfigMap", "-config")
	var config map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(dig(t, configMap, "data", "config.yaml").(string)), &config))
	assert.Equal(t, "/etc/agentic-identity-broker/trust", dig(t, config, "security", "trust_material_dir"))
	assert.Equal(t, "/etc/agentic-identity-broker/trust/agentgateway-jwks.json",
		dig(t, config, "token_exchange", "client_assertion", "jwks_file"))
}

func TestHelmTemplate_TrustMaterialInlineClientAssertion(t *testing.T) {
	const keySet = `{"keys":[{"kty":"EC","crv":"P-256","kid":"gw-1","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0"}]}`
	output := renderHelmTemplate(t, "-f", writeHelmValues(t, `
broker:
  tokenExchange:
    clientAssertion:
      issuerUri: https://agentgateway.example.com
      jwks: '`+keySet+`'
`))
	configMap := manifestOfKind(t, renderedManifests(t, output), "ConfigMap", "-config")
	var config map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(dig(t, configMap, "data", "config.yaml").(string)), &config))

	assert.JSONEq(t, keySet, dig(t, config, "token_exchange", "client_assertion", "jwks").(string))
	assert.Nil(t, dig(t, config, "security", "trust_material_dir"), "no trust directory without key-set files")
}

func TestHelmTemplate_TrustMaterialRequiresDirectory(t *testing.T) {
	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}
	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	require.NoError(t, err)

	cmd := exec.Command("helm", "template", "broker", chartPath,
		"--set", "broker.oauth2AuthorizationServer.mode=proxy",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamIssuerUri=https://idp.example.com",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamAuthorizeEndpoint=https://idp.example.com/oauth2/authorize",
		"--set", "broker.oauth2AuthorizationServer.proxy.upstreamTokenEndpoint=https://idp.example.com/oauth2/token",
		"-f", writeHelmValues(t, `
broker:
  security:
    trustMaterial:
      sources:
        - secret:
            name: agentgateway-jwks
            items:
              - key: jwks.json
                path: agentgateway-jwks.json
`))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	require.Error(t, cmd.Run())
	assert.Contains(t, stderr.String(), "broker.security.trustMaterial.directory is required when sources are set")
}
