package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	domconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialFilesKey = "third_party_oauth2.credential_files"
const credentialFilesEnvironment = "IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES"

func credentialFilesTestEnvironment(t *testing.T) {
	t.Helper()
	// Loader reads .env and config.yaml relative to the working directory.
	t.Chdir(t.TempDir())
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "IDENTITY_BROKER_") {
			t.Setenv(name, "")
			require.NoError(t, os.Unsetenv(name))
		}
	}
	setMinimalConfigEnv(t)
}

func credentialFilesJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func credentialFilesLoad(t *testing.T, source, value string) (*ports.Config, error) {
	t.Helper()
	credentialFilesTestEnvironment(t)
	switch source {
	case "YAML":
		path := filepath.Join(t.TempDir(), "selected.yaml")
		require.NoError(t, os.WriteFile(path, []byte("third_party_oauth2:\n  credential_files: "+value+"\n"), 0o600))
		t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	case "environment":
		t.Setenv(credentialFilesEnvironment, value)
	default:
		t.Fatalf("unsupported test source %q", source)
	}
	return NewLoader().GetConfig(context.Background())
}

func requireCredentialFilesError(t *testing.T, err error, forbidden ...string) {
	t.Helper()
	require.Error(t, err)
	assert.Contains(t, err.Error(), credentialFilesKey)
	var diagnostic *domconfig.ConfigError
	require.ErrorAs(t, err, &diagnostic)
	assert.NotEmpty(t, diagnostic.Expected, "diagnostic must identify the violated rule")
	for current := err; current != nil; current = errors.Unwrap(current) {
		for _, value := range forbidden {
			if value != "" {
				assert.NotContains(t, current.Error(), value)
				assert.NotContains(t, fmt.Sprintf("%#v", current), value, "structured diagnostics must not retain raw values")
			}
		}
	}
}

func TestCredentialFilesLoaderPreservesLiteralKeysAndPaths(t *testing.T) {
	for _, source := range []string{"YAML", "environment"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			bindings := map[string]ports.CredentialFileBinding{
				"GitHub-Prod":         {ClientIDFile: filepath.Join(root, "UPPER identity"), ClientSecretFile: filepath.Join(root, "UPPER secret")},
				"github-prod":         {ClientIDFile: filepath.Join(root, "lower identity"), ClientSecretFile: filepath.Join(root, "lower secret")},
				"com.Example.service": {ClientIDFile: filepath.Join(root, "dotted identity"), ClientSecretFile: filepath.Join(root, "dotted secret")},
			}
			value := credentialFilesJSON(t, bindings)
			if source == "YAML" {
				value = fmt.Sprintf("\n    GitHub-Prod:\n      client_id_file: %q\n      client_secret_file: %q\n    github-prod:\n      client_id_file: %q\n      client_secret_file: %q\n    com.Example.service:\n      client_id_file: %q\n      client_secret_file: %q", bindings["GitHub-Prod"].ClientIDFile, bindings["GitHub-Prod"].ClientSecretFile, bindings["github-prod"].ClientIDFile, bindings["github-prod"].ClientSecretFile, bindings["com.Example.service"].ClientIDFile, bindings["com.Example.service"].ClientSecretFile)
			}
			cfg, err := credentialFilesLoad(t, source, value)
			require.NoError(t, err)
			assert.Equal(t, bindings, cfg.ThirdPartyOAuth2.CredentialFiles)
		})
	}
}

func TestCredentialFilesLoaderDefaultsAndExplicitEmptyObjects(t *testing.T) {
	t.Run("omission", func(t *testing.T) {
		credentialFilesTestEnvironment(t)
		cfg, err := NewLoader().GetConfig(context.Background())
		require.NoError(t, err)
		assert.Empty(t, cfg.ThirdPartyOAuth2.CredentialFiles)
	})
	for _, source := range []string{"YAML", "environment"} {
		t.Run(source, func(t *testing.T) {
			cfg, err := credentialFilesLoad(t, source, "{}")
			require.NoError(t, err)
			assert.Empty(t, cfg.ThirdPartyOAuth2.CredentialFiles)
		})
	}
}

func TestCredentialFilesLoaderRejectsNonObjectMapsAndPairs(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"map null", "null"},
		{"map array", "[]"},
		{"map string", `"synthetic-private-input"`},
		{"map number", "713579"},
		{"map boolean", "true"},
		{"pair null", `{"alpha":null}`},
		{"pair array", `{"alpha":[]}`},
		{"pair string", `{"alpha":"synthetic-private-input"}`},
		{"pair number", `{"alpha":713579}`},
		{"pair boolean", `{"alpha":true}`},
		{"missing both fields", `{"alpha":{}}`},
	}
	for _, source := range []string{"YAML", "environment"} {
		for _, tc := range cases {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				_, err := credentialFilesLoad(t, source, tc.value)
				requireCredentialFilesError(t, err, "synthetic-private-input", "713579")
			})
		}
	}
	for _, value := range []string{"", `{"alpha":`, `{"alpha":{}} trailing-synthetic-private-input`} {
		t.Run("invalid JSON/"+value, func(t *testing.T) {
			_, err := credentialFilesLoad(t, "environment", value)
			requireCredentialFilesError(t, err, "trailing-synthetic-private-input")
		})
	}
	for _, key := range []string{"42", "true", "null"} {
		t.Run("non-string YAML key/"+key, func(t *testing.T) {
			value := "\n    " + key + ":\n      client_id_file: /synthetic-private/id\n      client_secret_file: /synthetic-private/secret"
			_, err := credentialFilesLoad(t, "YAML", value)
			requireCredentialFilesError(t, err, "/synthetic-private/id", "/synthetic-private/secret")
		})
	}
}

func TestCredentialFilesLoaderRequiresBothStrictStringPaths(t *testing.T) {
	cases := []struct {
		name    string
		value   any
		missing bool
	}{
		{"missing", nil, true},
		{"null", nil, false},
		{"integer", 713579, false},
		{"boolean", true, false},
		{"array", []string{"/synthetic-private/path"}, false},
		{"object", map[string]string{"path": "/synthetic-private/path"}, false},
		{"empty", "", false},
		{"whitespace", " \t\n", false},
		{"relative", "synthetic-private/relative", false},
		{"dot relative", "./synthetic-private/relative", false},
	}
	for _, source := range []string{"YAML", "environment"} {
		for _, field := range []string{"client_id_file", "client_secret_file"} {
			for _, tc := range cases {
				t.Run(source+"/"+field+"/"+tc.name, func(t *testing.T) {
					pair := map[string]any{"client_id_file": "/synthetic-private/id", "client_secret_file": "/synthetic-private/secret"}
					if tc.missing {
						delete(pair, field)
					} else {
						pair[field] = tc.value
					}
					_, err := credentialFilesLoad(t, source, credentialFilesJSON(t, map[string]any{"alpha": pair}))
					requireCredentialFilesError(t, err, "/synthetic-private", "synthetic-private/relative", "713579")
				})
			}
		}
	}
}

func TestCredentialFilesLoaderAndValidatorUseCanonicalGrammar(t *testing.T) {
	cases := []struct {
		name  string
		key   string
		valid bool
	}{
		{"single letter", "A", true},
		{"single digit", "7", true},
		{"all allowed characters", "Ab9._-", true},
		{"period", ".", true},
		{"underscore", "_", true},
		{"hyphen", "-", true},
		{"maximum length", strings.Repeat("A", 128), true},
		{"empty", "", false},
		{"too long", strings.Repeat("A", 129), false},
		{"space", "alpha beta", false},
		{"slash", "alpha/beta", false},
		{"colon", "alpha:beta", false},
		{"non ASCII", "alphä", false},
		{"newline", "alpha\n", false},
		{"UUID dashed", "550e8400-e29b-41d4-a716-446655440000", false},
		{"UUID uppercase", "550E8400-E29B-41D4-A716-446655440000", false},
		{"UUID undashed", "550e8400e29b41d4a716446655440000", false},
		{"UUID braces", "{550e8400-e29b-41d4-a716-446655440000}", false},
		{"UUID URN", "urn:uuid:550e8400-e29b-41d4-a716-446655440000", false},
		{"UUID zero", "00000000-0000-0000-0000-000000000000", false},
		{"UUID arbitrary version and variant", "ffffffff-ffff-ffff-ffff-ffffffffffff", false},
	}
	for _, tc := range cases {
		for _, source := range []string{"YAML", "environment", "validator"} {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "id"), ClientSecretFile: filepath.Join(t.TempDir(), "secret")}
				bindings := map[string]ports.CredentialFileBinding{tc.key: binding}
				var cfg *ports.Config
				var err error
				if source == "validator" {
					cfg = validTestConfig()
					cfg.ThirdPartyOAuth2.CredentialFiles = bindings
					err = Validate(cfg)
				} else {
					cfg, err = credentialFilesLoad(t, source, credentialFilesJSON(t, bindings))
				}
				if tc.valid {
					require.NoError(t, err)
					assert.Equal(t, bindings, cfg.ThirdPartyOAuth2.CredentialFiles)
				} else {
					requireCredentialFilesError(t, err, binding.ClientIDFile, binding.ClientSecretFile)
				}
			})
		}
	}
}

func TestCredentialFilesValidatorRequiresAbsoluteNonblankPaths(t *testing.T) {
	for _, field := range []string{"client_id_file", "client_secret_file"} {
		for _, value := range []string{"", " \t\n", "synthetic-private/relative", "./synthetic-private/relative"} {
			t.Run(field+"/"+value, func(t *testing.T) {
				cfg := validTestConfig()
				binding := ports.CredentialFileBinding{ClientIDFile: "/synthetic-private/id", ClientSecretFile: "/synthetic-private/secret"}
				if field == "client_id_file" {
					binding.ClientIDFile = value
				} else {
					binding.ClientSecretFile = value
				}
				cfg.ThirdPartyOAuth2.CredentialFiles = map[string]ports.CredentialFileBinding{"alpha": binding}
				requireCredentialFilesError(t, Validate(cfg), "/synthetic-private", "synthetic-private/relative")
			})
		}
	}
}

func TestCredentialFilesEnvironmentReplacesWholeYAMLMapping(t *testing.T) {
	for _, value := range []string{
		`{"beta":{"client_id_file":"/env/id","client_secret_file":"/env/secret"}}`,
		`{"alpha":{"client_id_file":"/env/id","client_secret_file":"/env/secret"}}`,
		"{}",
	} {
		t.Run(value, func(t *testing.T) {
			credentialFilesTestEnvironment(t)
			path := filepath.Join(t.TempDir(), "selected.yaml")
			require.NoError(t, os.WriteFile(path, []byte("third_party_oauth2:\n  credential_files:\n    alpha:\n      client_id_file: /yaml/id\n      client_secret_file: /yaml/secret\n    yaml-only:\n      client_id_file: /yaml/other-id\n      client_secret_file: /yaml/other-secret\n"), 0o600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
			t.Setenv(credentialFilesEnvironment, value)
			cfg, err := NewLoader().GetConfig(context.Background())
			require.NoError(t, err)
			var expected map[string]ports.CredentialFileBinding
			require.NoError(t, json.Unmarshal([]byte(value), &expected))
			assert.Equal(t, expected, cfg.ThirdPartyOAuth2.CredentialFiles)
		})
	}
}

func TestCredentialFilesWinningMapCannotInheritMissingPairFields(t *testing.T) {
	credentialFilesTestEnvironment(t)
	path := filepath.Join(t.TempDir(), "selected.yaml")
	require.NoError(t, os.WriteFile(path, []byte("third_party_oauth2:\n  credential_files:\n    alpha:\n      client_id_file: /synthetic-private/yaml-id\n      client_secret_file: /synthetic-private/yaml-secret\n"), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	t.Setenv(credentialFilesEnvironment, `{"alpha":{"client_id_file":"/synthetic-private/env-id"}}`)
	_, err := NewLoader().GetConfig(context.Background())
	requireCredentialFilesError(t, err, "/synthetic-private")
}

func TestCredentialFilesMalformedWinningInputDoesNotFallBack(t *testing.T) {
	for _, value := range []string{"", "null", "[]", `{"synthetic-private-input":`} {
		t.Run(value, func(t *testing.T) {
			credentialFilesTestEnvironment(t)
			path := filepath.Join(t.TempDir(), "selected.yaml")
			require.NoError(t, os.WriteFile(path, []byte("third_party_oauth2:\n  credential_files:\n    alpha:\n      client_id_file: /synthetic-private/yaml-id\n      client_secret_file: /synthetic-private/yaml-secret\n"), 0o600))
			t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
			t.Setenv(credentialFilesEnvironment, value)
			_, err := NewLoader().GetConfig(context.Background())
			requireCredentialFilesError(t, err, "synthetic-private-input", "/synthetic-private")
		})
	}
}

func TestCredentialFilesYAMLRetainsExpansionAndSelectedFile(t *testing.T) {
	credentialFilesTestEnvironment(t)
	root := t.TempDir()
	t.Setenv("CREDENTIAL_TEST_DIRECTORY", root)
	t.Setenv("CREDENTIAL_TEST_ID", "${CREDENTIAL_TEST_DIRECTORY}/identity")
	t.Setenv("CREDENTIAL_TEST_UNUSED_DEFAULT", "")
	require.NoError(t, os.Unsetenv("CREDENTIAL_TEST_UNUSED_DEFAULT"))
	// A tempting default file must not replace the explicitly selected document.
	require.NoError(t, os.WriteFile("config.yaml", []byte("third_party_oauth2:\n  credential_files: null\n"), 0o600))
	path := filepath.Join(t.TempDir(), "operator-selected.yaml")
	content := fmt.Sprintf("third_party_oauth2:\n  credential_files:\n    GitHub.Prod:\n      client_id_file: '${CREDENTIAL_TEST_ID}'\n      client_secret_file: '${CREDENTIAL_TEST_UNUSED_DEFAULT:%s}'\n", filepath.Join(root, "secret"))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	loader := NewLoader()
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]ports.CredentialFileBinding{"GitHub.Prod": {ClientIDFile: filepath.Join(root, "identity"), ClientSecretFile: filepath.Join(root, "secret")}}, cfg.ThirdPartyOAuth2.CredentialFiles)
	var selectedFiles []string
	for _, source := range loader.GetSources() {
		if source.Type == ports.SourceTypeYAML {
			selectedFiles = append(selectedFiles, source.Path)
		}
	}
	assert.Equal(t, []string{path}, selectedFiles)
}

func TestCredentialFilesLoaderDoesNotAliasOldSecretOnlyName(t *testing.T) {
	credentialFilesTestEnvironment(t)
	path := filepath.Join(t.TempDir(), "selected.yaml")
	require.NoError(t, os.WriteFile(path, []byte("third_party_oauth2:\n  client_secret_files:\n    old-yaml: /synthetic-private/old-secret\n"), 0o600))
	t.Setenv("IDENTITY_BROKER_CONFIG_PATH", path)
	t.Setenv("IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CLIENT_SECRET_FILES", `{"old-env":"/synthetic-private/old-secret"}`)
	cfg, err := NewLoader().GetConfig(context.Background())
	require.NoError(t, err)
	assert.Empty(t, cfg.ThirdPartyOAuth2.CredentialFiles)
}

func TestCredentialFilesValidationDoesNotResolveUnknownOrUnavailableBindings(t *testing.T) {
	for _, source := range []string{"YAML", "environment", "validator"} {
		t.Run(source, func(t *testing.T) {
			root := t.TempDir()
			brokenLink := filepath.Join(root, "broken-link")
			require.NoError(t, os.Symlink(filepath.Join(root, "missing-target"), brokenLink))
			bindings := map[string]ports.CredentialFileBinding{
				"unknown-missing":   {ClientIDFile: filepath.Join(root, "missing-id"), ClientSecretFile: filepath.Join(root, "missing-secret")},
				"unknown-directory": {ClientIDFile: root, ClientSecretFile: brokenLink},
			}
			if source == "validator" {
				cfg := validTestConfig()
				cfg.ThirdPartyOAuth2.CredentialFiles = bindings
				require.NoError(t, Validate(cfg))
			} else {
				cfg, err := credentialFilesLoad(t, source, credentialFilesJSON(t, bindings))
				require.NoError(t, err)
				assert.Equal(t, bindings, cfg.ThirdPartyOAuth2.CredentialFiles)
			}
		})
	}
}

func TestCredentialFilesRenamedDiagnosticKeysRedactWholeValues(t *testing.T) {
	bindings := map[string]ports.CredentialFileBinding{"alpha": {ClientIDFile: "/synthetic-private/id", ClientSecretFile: "/synthetic-private/secret"}}
	for _, key := range []string{credentialFilesKey, credentialFilesEnvironment, credentialFilesKey + ".alpha.client_id_file", credentialFilesKey + ".alpha.client_secret_file"} {
		t.Run(key, func(t *testing.T) {
			assert.Equal(t, RedactedValue, Redact(key, bindings))
			assert.Equal(t, RedactedValue, Redact(key, credentialFilesJSON(t, bindings)))
		})
	}
}

func TestCredentialFilesYAMLLowercasePairControl(t *testing.T) {
	root := t.TempDir()
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(root, "identity"), ClientSecretFile: filepath.Join(root, "secret with trailing space ")}
	value := fmt.Sprintf("\n    alpha:\n      client_id_file: %q\n      client_secret_file: %q", binding.ClientIDFile, binding.ClientSecretFile)
	cfg, err := credentialFilesLoad(t, "YAML", value)
	require.NoError(t, err)
	assert.Equal(t, map[string]ports.CredentialFileBinding{"alpha": binding}, cfg.ThirdPartyOAuth2.CredentialFiles)
}

func TestCredentialFilesDotEnvRetainsExistingLoadingFlow(t *testing.T) {
	credentialFilesTestEnvironment(t)
	value := `{"DotEnv.Pair":{"client_id_file":"/synthetic-private/id","client_secret_file":"/synthetic-private/secret"}}`
	// Register restoration before godotenv mutates the process environment.
	t.Setenv(credentialFilesEnvironment, "")
	require.NoError(t, os.Unsetenv(credentialFilesEnvironment))
	require.NoError(t, os.WriteFile(".env", []byte(credentialFilesEnvironment+"='"+value+"'\n"), 0o600))
	cfg, err := NewLoader().GetConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]ports.CredentialFileBinding{"DotEnv.Pair": {ClientIDFile: "/synthetic-private/id", ClientSecretFile: "/synthetic-private/secret"}}, cfg.ThirdPartyOAuth2.CredentialFiles)
}
