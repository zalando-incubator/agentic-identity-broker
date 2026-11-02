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
	if err != nil {
		return "", err
	}
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

// sessionSweepArgs enables the chart-managed sweep with a literal operator header, the
// minimum identity the chart accepts, in the "identity" namespace with an admin public URL.
func sessionSweepArgs(extra ...string) []string {
	args := []string{
		"--namespace", "identity",
		"--set-string", "broker.server.admin.publicUrl=https://broker-admin.example.test",
		"--set", "sessionSweep.enabled=true",
		"--set-string", "sessionSweep.headers.X-Operator=session-sweep",
	}
	return append(args, extra...)
}

func TestHelmTemplate_SessionSweep(t *testing.T) {
	// US1-S1 from specs/053-session-auto-refresh/spec.md
	t.Run("renders no sweep resources when disabled", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--set-string", "sessionSweep.headers.X-Operator=session-sweep",
			"--set-string", "sessionSweep.schedule=*/5 * * * *",
		)
		require.NotContains(t, output, "kind: CronJob")
		require.NotContains(t, output, "session-sweep")
	})

	// US1-S2 from specs/053-session-auto-refresh/spec.md
	t.Run("renders an hourly Forbid CronJob targeting the admin Service with the public-URL Host and PT2H", func(t *testing.T) {
		output := renderHelmTemplate(t, sessionSweepArgs("--show-only", "templates/cronjob-session-sweep.yaml")...)
		for _, want := range []string{
			"kind: CronJob",
			"name: broker-agentic-identity-broker-session-sweep",
			`schedule: "0 * * * *"`,
			"concurrencyPolicy: Forbid",
			"startingDeadlineSeconds: 3600",
			"backoffLimit: 0",
			"activeDeadlineSeconds: 2100",
			"automountServiceAccountToken: false",
			"readOnlyRootFilesystem: true",
			`url = "http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep"`,
			`header = "Host: broker-admin.example.test"`,
			`header = "X-Operator: session-sweep"`,
			`data = "{\"lookahead_duration\":\"PT2H\",\"dry_run\":false}"`,
			"max-time = 1800",
		} {
			require.Contains(t, output, want)
		}
	})

	// US1-S12 from specs/053-session-auto-refresh/spec.md
	t.Run("allows the configured number of job retries", func(t *testing.T) {
		output := renderHelmTemplate(t, sessionSweepArgs(
			"--show-only", "templates/cronjob-session-sweep.yaml",
			"--set", "sessionSweep.backoffLimit=1",
		)...)
		require.Contains(t, output, "backoffLimit: 1\n")
	})

	t.Run("keeps the session-sweep suffix when the fullname is long", func(t *testing.T) {
		fullname := strings.Repeat("a", 52)
		output := renderHelmTemplate(t, sessionSweepArgs(
			"--show-only", "templates/serviceaccount-session-sweep.yaml",
			"--set", "fullnameOverride="+fullname,
		)...)
		require.Contains(t, output, "name: "+fullname[:38]+"-session-sweep\n")
	})

	// US1-S10 from specs/053-session-auto-refresh/spec.md
	t.Run("keeps the job pod out of the broker Service and PDB selectors", func(t *testing.T) {
		output := renderHelmTemplate(t, sessionSweepArgs(
			"--show-only", "templates/cronjob-session-sweep.yaml",
			"--set-string", `commonLabels.app\.kubernetes\.io/name=agentic-identity-broker`,
			"--set-string", `sessionSweep.podLabels.app\.kubernetes\.io/name=agentic-identity-broker`,
			"--set-string", `sessionSweep.podLabels.app\.kubernetes\.io/component=api`,
		)...)
		_, podTemplate, found := strings.Cut(output, "jobTemplate:")
		require.True(t, found, "rendered CronJob has no jobTemplate")
		require.NotContains(t, podTemplate, "app.kubernetes.io/name:")
		require.Contains(t, podTemplate, "app.kubernetes.io/component: session-sweep")
		require.NotContains(t, podTemplate, "app.kubernetes.io/component: api")
	})

	// US1-S5 from specs/053-session-auto-refresh/spec.md (render-time part)
	t.Run("reads file headers through curl config variables and never renders their value", func(t *testing.T) {
		output := renderHelmTemplate(t, sessionSweepArgs(
			"--show-only", "templates/cronjob-session-sweep.yaml",
			"--set-string", "sessionSweep.headersFromFile.Authorization.path=/var/run/secrets/session-sweep/token",
			"--set-string", "sessionSweep.headersFromFile.Authorization.prefix=Bearer ",
		)...)
		require.Contains(t, output, `variable = "header_0@/var/run/secrets/session-sweep/token"`)
		require.Contains(t, output, `expand-header = "Authorization: Bearer {{header_0:trim:json}}"`)
		require.NotContains(t, output, "cat ")
	})

	// US1-S7 and US1-S11 from specs/053-session-auto-refresh/spec.md
	t.Run("fails to render with an invalid sweep configuration", func(t *testing.T) {
		tests := []struct {
			name    string
			args    []string
			wantErr string
		}{
			{"no identity", []string{"--namespace", "identity", "--set", "sessionSweep.enabled=true"},
				"sessionSweep.enabled requires an identity for the admin API"},
			{"timeout not below the deadline", sessionSweepArgs("--set", "sessionSweep.timeoutSeconds=60", "--set", "sessionSweep.activeDeadlineSeconds=60"),
				"sessionSweep.timeoutSeconds (60) must be shorter than sessionSweep.activeDeadlineSeconds (60)"},
			{"unpaired TLS certificate", sessionSweepArgs("--set-string", "sessionSweep.tls.certFile=/var/run/secrets/session-sweep/tls.crt"),
				"sessionSweep.tls.certFile and sessionSweep.tls.keyFile must be set together"},
			{"non-ISO lookahead", sessionSweepArgs("--set-string", "sessionSweep.lookaheadDuration=2h"),
				"lookaheadDuration"},
			{"zero lookahead", sessionSweepArgs("--set-string", "sessionSweep.lookaheadDuration=PT0S"),
				"lookaheadDuration"},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				output, err := runHelmTemplate(t, tt.args...)
				require.Error(t, err, output)
				require.Contains(t, output, tt.wantErr)
			})
		}
	})
}

func TestHelmTemplate_StagedSessionMigrations(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		upgrade bool
	}{
		{name: "install_default"},
		{name: "install_zero", args: []string{"--set", "migration.targetVersion=0"}},
		{name: "install_46", args: []string{"--set", "migration.targetVersion=46"}},
		{name: "upgrade_default", upgrade: true},
		{name: "upgrade_46", args: []string{"--set", "migration.targetVersion=46"}, upgrade: true},
	}
	for _, testCase := range cases {
		for _, grants := range []string{"true", "false"} {
			t.Run(testCase.name+"_grants_"+grants, func(t *testing.T) {
				args := []string{
					"--show-only", "templates/job-migrate.yaml",
					"--set", "storage.type=postgres",
					"--set", "postgresql.external.enabled=true",
					"--set", "postgresql.external.host=postgres.example.com",
					"--set", "migration.grants.enabled=" + grants,
					"--set", "migration.image.repository=custom-migrations",
					"--set", "migration.image.tag=phase-test",
					"--set", "migration.grants.image=custom-postgres:17",
				}
				args = append(args, testCase.args...)
				if testCase.upgrade {
					args = append(args, "--is-upgrade")
				}
				output := renderHelmTemplate(t, args...)

				require.Contains(t, output, `"helm.sh/hook": pre-install,pre-upgrade`)
				require.Contains(t, output, `image: "custom-migrations:phase-test"`)
				require.Contains(t, output, "- \"-path\"\n          - \"/app/migrations\"\n          - \"-database\"")
				require.Contains(t, output, "- name: migrate")
				if testCase.upgrade {
					require.Contains(t, output, "command: [\"/bin/sh\", \"-ec\"]")
					require.Contains(t, output, "target=46")
					require.Contains(t, output, `current="$(/usr/local/bin/migrate "$@" version 2>&1)"`)
					require.Contains(t, output, `exec /usr/local/bin/migrate "$@" goto "$target"`)
					require.NotContains(t, output, "force")
					require.NotContains(t, output, `- "up"`)
				} else {
					require.Contains(t, output, `- "up"`)
					require.NotContains(t, output, "command: [\"/bin/sh\", \"-ec\"]")
				}
				if grants == "true" {
					require.Contains(t, output, "initContainers:")
					require.Contains(t, output, "- name: apply-grants")
					require.Contains(t, output, `image: "custom-postgres:17"`)
					require.Less(t, strings.Index(output, "- name: migrate"), strings.Index(output, "- name: apply-grants"))
				} else {
					require.Contains(t, output, "initContainers: []")
					require.NotContains(t, output, "- name: apply-grants")
				}
			})
		}
	}
}

func TestHelmTemplate_SessionAPICompatibility(t *testing.T) {
	for _, compatibility := range []string{"true", "false"} {
		t.Run(compatibility, func(t *testing.T) {
			args := []string{"--show-only", "templates/configmap.yaml"}
			if compatibility == "false" {
				args = append(args, "--set", "broker.thirdPartyOauth2.sessionAPICompatibility=false")
			}
			output := renderHelmTemplate(t, args...)
			require.Contains(t, output, "third_party_oauth2:\n      jwe_signing_key: ${IDENTITY_BROKER_JWE_SIGNING_KEY}\n      session_api_compatibility: "+compatibility)
		})
	}
}

func TestHelmTemplate_RollingAvailability(t *testing.T) {
	t.Run("online defaults retain ready capacity", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--set", "replicaCount=2",
			"--set", "podDisruptionBudget.enabled=true",
			"--set", "podDisruptionBudget.minAvailable=2",
		)
		require.Contains(t, output, "replicas: 2")
		require.Contains(t, output, "strategy:\n    type: RollingUpdate\n    rollingUpdate:\n      maxSurge: 1\n      maxUnavailable: 0")
		require.Contains(t, output, "terminationGracePeriodSeconds: 60")
		require.Contains(t, output, "kind: PodDisruptionBudget")
		require.Contains(t, output, "minAvailable: 2")
		require.Contains(t, output, "readinessProbe:\n          httpGet:\n            path: /health\n            port: enduser")
	})
	t.Run("custom strategy and grace remain operator controlled", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--show-only", "templates/deployment.yaml",
			"--set", "strategy.rollingUpdate.maxUnavailable=1",
			"--set", "strategy.rollingUpdate.maxSurge=2",
			"--set", "terminationGracePeriodSeconds=90",
		)
		require.Contains(t, output, "maxSurge: 2\n      maxUnavailable: 1")
		require.Contains(t, output, "terminationGracePeriodSeconds: 90")
	})
	t.Run("Recreate omits rolling configuration", func(t *testing.T) {
		output := renderHelmTemplate(t,
			"--show-only", "templates/deployment.yaml",
			"--set", "strategy.type=Recreate",
		)
		require.Contains(t, output, "strategy:\n    type: Recreate")
		require.NotContains(t, output, "rollingUpdate:")
	})
}

func TestHelmTemplate_StagedMigrationUpgradeGuards(t *testing.T) {
	baseArgs := []string{
		"--is-upgrade",
		"--set", "storage.type=postgres",
		"--set", "postgresql.external.enabled=true",
		"--set", "postgresql.external.host=postgres.example.com",
	}
	cases := []struct {
		name      string
		args      []string
		errorText string
	}{
		{name: "zero target refused", args: []string{"--set", "migration.targetVersion=0"}, errorText: "migration.targetVersion >= 46"},
		{name: "obsolete preparation target refused", args: []string{"--set", "migration.targetVersion=38"}, errorText: "migration.targetVersion >= 46"},
		{name: "preparation prerequisite refused", args: []string{"--set", "migration.targetVersion=45"}, errorText: "migration.targetVersion >= 46"},
		{name: "negative target rejected", args: []string{"--set", "migration.targetVersion=-1"}, errorText: "targetVersion"},
		{name: "future activation target rejected", args: []string{"--set", "migration.targetVersion=47"}, errorText: "targetVersion"},
		{name: "fractional target rejected", args: []string{"--set", "migration.targetVersion=1.5"}, errorText: "targetVersion"},
		{name: "string target rejected", args: []string{"--set-string", "migration.targetVersion=46"}, errorText: "targetVersion"},
		{name: "default preparation target permitted"},
		{name: "positive target permitted", args: []string{"--set", "migration.targetVersion=46"}},
		{name: "disabled migration permits zero", args: []string{"--set", "migration.enabled=false", "--set", "migration.targetVersion=0"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := runHelmTemplate(t, append(baseArgs, testCase.args...)...)
			if testCase.errorText != "" {
				require.Error(t, err)
				require.Contains(t, output, testCase.errorText)
			} else {
				require.NoError(t, err, output)
			}
		})
	}
}
