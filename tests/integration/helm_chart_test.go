package integration

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func executeHelmTemplate(t *testing.T, extraArgs ...string) (string, error) {
	t.Helper()

	if _, err := exec.LookPath("helm"); err != nil {
		t.Skip("helm is not installed")
	}

	chartPath, err := filepath.Abs("../../charts/agentic-identity-broker")
	if err != nil {
		return "", err
	}

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
	output, err := executeHelmTemplate(t, extraArgs...)
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

func TestHelmTemplate_StagedSessionMigrations(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		upgrade bool
		target  string
	}{
		{name: "install_default"},
		{name: "install_zero", args: []string{"--set", "migration.targetVersion=0"}},
		{name: "install_38", args: []string{"--set", "migration.targetVersion=38"}},
		{name: "install_39", args: []string{"--set", "migration.targetVersion=39"}},
		{name: "install_40", args: []string{"--set", "migration.targetVersion=40"}},
		{name: "upgrade_default", upgrade: true, target: "40"},
		{name: "upgrade_38", args: []string{"--set", "migration.targetVersion=38"}, upgrade: true, target: "38"},
		{name: "upgrade_39", args: []string{"--set", "migration.targetVersion=39"}, upgrade: true, target: "39"},
		{name: "upgrade_40", args: []string{"--set", "migration.targetVersion=40"}, upgrade: true, target: "40"},
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
					require.Contains(t, output, "target="+testCase.target)
					require.Contains(t, output, `current="$(/usr/local/bin/migrate "$@" version 2>&1)"`)
					require.Contains(t, output, "Dirty migration metadata: inspect schema before manual repair")
					require.Contains(t, output, `if [ "$current" -ge "$target" ]; then`)
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
			if compatibility == "true" {
				args = append(args, "--set", "broker.thirdPartyOauth2.sessionAPICompatibility=true")
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
		{name: "zero target refused", args: []string{"--set", "migration.targetVersion=0"}, errorText: "positive migration.targetVersion"},
		{name: "negative target rejected", args: []string{"--set", "migration.targetVersion=-1"}, errorText: "targetVersion"},
		{name: "fractional target rejected", args: []string{"--set", "migration.targetVersion=1.5"}, errorText: "targetVersion"},
		{name: "string target rejected", args: []string{"--set-string", "migration.targetVersion=40"}, errorText: "targetVersion"},
		{name: "future target unavailable", args: []string{"--set", "migration.targetVersion=41"}, errorText: "targetVersion"},
		{name: "default cleanup target permitted"},
		{name: "preparation target permitted", args: []string{"--set", "migration.targetVersion=38"}},
		{name: "activation target permitted", args: []string{"--set", "migration.targetVersion=39"}},
		{name: "cleanup target permitted", args: []string{"--set", "migration.targetVersion=40"}},
		{name: "disabled migration permits zero", args: []string{"--set", "migration.enabled=false", "--set", "migration.targetVersion=0"}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			output, err := executeHelmTemplate(t, append(baseArgs, testCase.args...)...)
			if testCase.errorText != "" {
				require.Error(t, err)
				require.Contains(t, output, testCase.errorText)
			} else {
				require.NoError(t, err, output)
			}
		})
	}
}
