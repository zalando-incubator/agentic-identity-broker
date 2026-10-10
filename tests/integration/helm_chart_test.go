package integration

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

func runHelmTemplate(t *testing.T, extraArgs ...string) (string, error) {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		if os.Getenv("REQUIRE_HELM") != "" {
			t.Fatalf("Helm is required for the deployment contract: %v", err)
		}
		t.Skipf("Skipping Helm rendering tests: %v", err)
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
	output, err := exec.Command(helm, append(args, extraArgs...)...).CombinedOutput()
	return string(output), err
}

func renderHelmTemplate(t *testing.T, extraArgs ...string) string {
	t.Helper()
	output, err := runHelmTemplate(t, extraArgs...)
	require.NoError(t, err, output)
	return output
}

type credentialHelmMount struct {
	Name        string `yaml:"name"`
	MountPath   string `yaml:"mountPath"`
	ReadOnly    bool   `yaml:"readOnly"`
	SubPath     string `yaml:"subPath"`
	SubPathExpr string `yaml:"subPathExpr"`
}

type credentialHelmVolume struct {
	Name   string `yaml:"name"`
	Secret *struct {
		SecretName  string `yaml:"secretName"`
		DefaultMode int    `yaml:"defaultMode"`
	} `yaml:"secret"`
	ConfigMap *struct {
		Name string `yaml:"name"`
	} `yaml:"configMap"`
	EmptyDir *struct{} `yaml:"emptyDir"`
}

type credentialHelmSecurity struct {
	RunAsNonRoot *bool  `yaml:"runAsNonRoot"`
	RunAsUser    *int64 `yaml:"runAsUser"`
	RunAsGroup   *int64 `yaml:"runAsGroup"`
	FSGroup      *int64 `yaml:"fsGroup"`
}

type credentialHelmPod struct {
	SecurityContext credentialHelmSecurity `yaml:"securityContext"`
	Containers      []struct {
		SecurityContext credentialHelmSecurity `yaml:"securityContext"`
		VolumeMounts    []credentialHelmMount  `yaml:"volumeMounts"`
		Env             []struct {
			Name      string `yaml:"name"`
			Value     string `yaml:"value"`
			ValueFrom *struct {
				SecretKeyRef *struct {
					Name string `yaml:"name"`
					Key  string `yaml:"key"`
				} `yaml:"secretKeyRef"`
			} `yaml:"valueFrom"`
		} `yaml:"env"`
	} `yaml:"containers"`
	Volumes []credentialHelmVolume `yaml:"volumes"`
}

type credentialHelmObject struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Data       map[string]string `yaml:"data"`
	StringData map[string]string `yaml:"stringData"`
	Spec       struct {
		Template struct {
			Spec credentialHelmPod `yaml:"spec"`
		} `yaml:"template"`
	} `yaml:"spec"`
}

func credentialHelmConfiguration(t *testing.T, output string) (map[string]any, credentialHelmPod, map[string]credentialHelmObject) {
	t.Helper()
	decoder := yaml.NewDecoder(strings.NewReader(output))
	configMaps := map[string]credentialHelmObject{}
	secrets := map[string]credentialHelmObject{}
	var pod credentialHelmPod
	deployments := 0
	for {
		var object credentialHelmObject
		err := decoder.Decode(&object)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch object.Kind {
		case "ConfigMap":
			configMaps[object.Metadata.Name] = object
		case "Secret":
			secrets[object.Metadata.Name] = object
		case "Deployment":
			require.Equal(t, "apps/v1", object.APIVersion)
			pod = object.Spec.Template.Spec
			deployments++
		}
	}
	require.Equal(t, 1, deployments)
	require.Len(t, pod.Containers, 1)
	configMapName := ""
	for _, volume := range pod.Volumes {
		if volume.Name == "config" {
			require.NotNil(t, volume.ConfigMap)
			configMapName = volume.ConfigMap.Name
		}
	}
	require.NotEmpty(t, configMapName)
	configMap, ok := configMaps[configMapName]
	require.True(t, ok, "the Deployment must reference the rendered broker ConfigMap")
	require.Equal(t, "v1", configMap.APIVersion)
	require.NotEmpty(t, configMap.Data["config.yaml"])
	var configuration map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(configMap.Data["config.yaml"]), &configuration))
	return configuration, pod, secrets
}

func credentialHelmValuesFile(t *testing.T, values map[string]any) string {
	t.Helper()
	encoded, err := json.Marshal(values)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(path, encoded, 0o600))
	return path
}

func requireCredentialHelmMounts(t *testing.T, pod credentialHelmPod, credentialMount credentialHelmMount) credentialHelmVolume {
	t.Helper()
	require.NotNil(t, pod.SecurityContext.RunAsNonRoot)
	require.True(t, *pod.SecurityContext.RunAsNonRoot)
	require.NotNil(t, pod.SecurityContext.RunAsUser)
	require.Greater(t, *pod.SecurityContext.RunAsUser, int64(0))
	require.NotNil(t, pod.SecurityContext.RunAsGroup)
	require.Greater(t, *pod.SecurityContext.RunAsGroup, int64(0))
	require.NotNil(t, pod.SecurityContext.FSGroup)
	require.Greater(t, *pod.SecurityContext.FSGroup, int64(0))
	container := pod.Containers[0]
	require.NotNil(t, container.SecurityContext.RunAsNonRoot)
	require.True(t, *container.SecurityContext.RunAsNonRoot)
	require.NotNil(t, container.SecurityContext.RunAsUser)
	require.Greater(t, *container.SecurityContext.RunAsUser, int64(0))
	mounts := map[string]credentialHelmMount{}
	for _, mount := range container.VolumeMounts {
		require.NotContains(t, mounts, mount.Name, "each mount name must be unique")
		mounts[mount.Name] = mount
	}
	require.Len(t, mounts, 3)
	require.Equal(t, credentialMount, mounts[credentialMount.Name])
	require.Equal(t, credentialHelmMount{Name: "config", MountPath: "/app/config.yaml", ReadOnly: true, SubPath: "config.yaml"}, mounts["config"])
	require.Equal(t, credentialHelmMount{Name: "tmp", MountPath: "/tmp"}, mounts["tmp"])
	volumes := map[string]credentialHelmVolume{}
	for _, volume := range pod.Volumes {
		require.NotContains(t, volumes, volume.Name, "each volume name must be unique")
		volumes[volume.Name] = volume
	}
	require.Len(t, volumes, 3)
	require.NotNil(t, volumes[credentialMount.Name].Secret)
	require.Equal(t, "agentic-platform-credentials", volumes[credentialMount.Name].Secret.SecretName)
	require.NotNil(t, volumes["config"].ConfigMap)
	require.NotNil(t, volumes["tmp"].EmptyDir)
	return volumes[credentialMount.Name]
}

// US7-S1 rendering contract; the corresponding E2E journey applies paired configuration over HTTP.
func TestHelmCredentialFilesDefaultsOmitBinding(t *testing.T) {
	for _, values := range []map[string]any{nil, {"broker": map[string]any{"thirdPartyOauth2": map[string]any{"credentialFiles": map[string]any{}}}}} {
		var args []string
		if values != nil {
			args = []string{"-f", credentialHelmValuesFile(t, values)}
		}
		configuration, pod, _ := credentialHelmConfiguration(t, renderHelmTemplate(t, args...))
		thirdParty, ok := configuration["third_party_oauth2"].(map[string]any)
		require.True(t, ok)
		require.NotContains(t, thirdParty, "credential_files")
		for _, volume := range pod.Volumes {
			require.NotEqual(t, "credentials", volume.Name)
		}
	}
}

// US7-S1 preserves existing mounts for single and multiple bindings.
func TestHelmCredentialFilesPairsAndExistingMounts(t *testing.T) {
	bindings := map[string]any{
		"zalando-platform":    map[string]any{"client_id_file": "/meta/credentials/employee-client-id", "client_secret_file": "/meta/credentials/employee-client-secret"},
		"GitHub-Prod":         map[string]any{"client_id_file": "/run/secrets/github-id", "client_secret_file": "/run/secrets/github-secret"},
		"com.example.service": map[string]any{"client_id_file": "/run/secrets/example-id", "client_secret_file": "/run/secrets/example-secret"},
	}
	for _, pairs := range []map[string]any{{"zalando-platform": bindings["zalando-platform"]}, bindings} {
		values := map[string]any{"broker": map[string]any{
			"thirdPartyOauth2":  map[string]any{"credentialFiles": pairs},
			"extraVolumes":      []map[string]any{{"name": "credentials", "secret": map[string]any{"secretName": "agentic-platform-credentials"}}},
			"extraVolumeMounts": []map[string]any{{"name": "credentials", "mountPath": "/meta/credentials", "readOnly": true}},
		}}
		configuration, pod, _ := credentialHelmConfiguration(t, renderHelmTemplate(t, "-f", credentialHelmValuesFile(t, values)))
		thirdParty, ok := configuration["third_party_oauth2"].(map[string]any)
		require.True(t, ok)
		require.Equal(t, pairs, thirdParty["credential_files"])
		requireCredentialHelmMounts(t, pod, credentialHelmMount{Name: "credentials", MountPath: "/meta/credentials", ReadOnly: true})
	}
}

// US7-S3 rendering contract; explicit filesystem registration remains in the HTTP E2E journey.
func TestHelmCredentialFilesTargetDeployment(t *testing.T) {
	configuration, pod, secrets := credentialHelmConfiguration(t, renderHelmTemplate(t,
		"-f", "../e2e/fixtures/oauth2-secret-file-values.yaml",
		"--set", "storage.type=memory",
		"--set", "migration.enabled=false",
		"--set-string", "broker.thirdPartyOauth2.jweSigningKeyBase64="+testJWESigningKey,
		"--set-string", "broker.encryption.memory.rawKey="+testEncryptionKey,
	))
	thirdParty, ok := configuration["third_party_oauth2"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{
		"zalando-platform": map[string]any{
			"client_id_file":     "/meta/credentials/employee-client-id",
			"client_secret_file": "/meta/credentials/employee-client-secret",
		},
	}, thirdParty["credential_files"])
	volume := requireCredentialHelmMounts(t, pod, credentialHelmMount{
		Name: "agentic-platform-credentials", MountPath: "/meta/credentials", ReadOnly: true,
	})
	require.Equal(t, 0o444, volume.Secret.DefaultMode, "mounted files must be readable by the non-root broker")
	environment := map[string]string{}
	for _, variable := range pod.Containers[0].Env {
		require.NotContains(t, environment, variable.Name)
		if variable.ValueFrom == nil {
			environment[variable.Name] = variable.Value
			continue
		}
		require.NotNil(t, variable.ValueFrom.SecretKeyRef)
		reference := variable.ValueFrom.SecretKeyRef
		secret, found := secrets[reference.Name]
		require.True(t, found, "the synthetic chart must supply every broker environment Secret")
		value, found := secret.StringData[reference.Key]
		require.True(t, found, "the rendered environment key must exist in its Secret")
		environment[variable.Name] = value
	}
	require.Equal(t, map[string]string{
		"IDENTITY_BROKER_JWE_SIGNING_KEY":           testJWESigningKey,
		"IDENTITY_BROKER_ENCRYPTION_MEMORY_RAW_KEY": testEncryptionKey,
	}, environment)
}

func TestHelmCredentialFilesRejectInvalidPairShapes(t *testing.T) {
	validPair := map[string]any{"client_id_file": "/run/identity", "client_secret_file": "/run/credential"}
	cases := []struct {
		name     string
		bindings any
	}{
		{"mapping array", []any{validPair}},
		{"mapping scalar", "not-a-mapping"},
		{"pair scalar", map[string]any{"service": "/run/credential"}},
		{"pair array", map[string]any{"service": []string{"/run/identity", "/run/credential"}}},
		{"missing client ID", map[string]any{"service": map[string]any{"client_secret_file": "/run/credential"}}},
		{"missing secret", map[string]any{"service": map[string]any{"client_id_file": "/run/identity"}}},
		{"numeric client ID path", map[string]any{"service": map[string]any{"client_id_file": 1, "client_secret_file": "/run/credential"}}},
		{"boolean secret path", map[string]any{"service": map[string]any{"client_id_file": "/run/identity", "client_secret_file": false}}},
		{"null client ID path", map[string]any{"service": map[string]any{"client_id_file": nil, "client_secret_file": "/run/credential"}}},
		{"empty secret path", map[string]any{"service": map[string]any{"client_id_file": "/run/identity", "client_secret_file": ""}}},
		{"invalid canonical spelling", map[string]any{"invalid canonical ID": validPair}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			values := map[string]any{"broker": map[string]any{"thirdPartyOauth2": map[string]any{"credentialFiles": testCase.bindings}}}
			_, err := runHelmTemplate(t, "-f", credentialHelmValuesFile(t, values))
			require.Error(t, err, "invalid credential pair must stop rendering")
		})
	}
}
