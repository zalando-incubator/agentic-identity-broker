package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	brokerconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const credentialRootFlag = "third_party_oauth2.credential_files"
const credentialRootEnvironment = "IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES"

func credentialRootJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	return string(encoded)
}

func credentialRootConfig(t *testing.T, value string) string {
	t.Helper()
	path := writeBrokerConfig(t, 8000, 14000)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	if value != "" {
		content = []byte(strings.Replace(string(content), "third_party_oauth2:\n", "third_party_oauth2:\n  credential_files: "+value+"\n", 1))
	}
	require.NoError(t, os.WriteFile(path, content, 0o600))
	return path
}

func credentialRootParse(t *testing.T, args []string) error {
	t.Helper()
	// Use the production command and its registered flags, not a cloned option set.
	rootCmd.PersistentFlags().VisitAll(func(flag *pflag.Flag) {
		value, changed := flag.Value.String(), flag.Changed
		t.Cleanup(func() {
			require.NoError(t, flag.Value.Set(value))
			flag.Changed = changed
		})
	})
	return rootCmd.ParseFlags(args)
}

func credentialRootLoad(t *testing.T, yamlValue string, environment *string, args []string) (*ports.Config, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	credentialRootTestEnvironment(t, credentialRootConfig(t, yamlValue))
	if environment != nil {
		t.Setenv(credentialRootEnvironment, *environment)
	}
	require.NoError(t, credentialRootParse(t, args), "the production CLI must accept the credential-pair JSON flag")
	loader := brokerconfig.NewLoader()
	loader.SetCommand(rootCmd)
	return loader.GetConfig(context.Background())
}

func TestCredentialFilesRootCLIAndEnvironmentReplaceWholeMappings(t *testing.T) {
	yamlValue := `{"alpha":{"client_id_file":"/yaml/id","client_secret_file":"/yaml/secret"},"yaml-only":{"client_id_file":"/yaml/other-id","client_secret_file":"/yaml/other-secret"}}`
	envValue := `{"alpha":{"client_id_file":"/env/id","client_secret_file":"/env/secret"},"Env.Pair":{"client_id_file":"/env/other-id","client_secret_file":"/env/other-secret"}}`
	cliValue := `{"alpha":{"client_id_file":"/cli/id","client_secret_file":"/cli/secret"},"CLI.Pair":{"client_id_file":"/cli/other-id","client_secret_file":"/cli/other-secret"},"cli.Pair":{"client_id_file":"/cli/lower-id","client_secret_file":"/cli/lower-secret"}}`
	empty := "{}"
	cases := []struct {
		name        string
		yaml        string
		environment *string
		args        []string
		expected    string
	}{
		{"empty default", "", nil, nil, empty},
		{"unchanged flag leaves YAML", yamlValue, nil, nil, yamlValue},
		{"environment replaces YAML", yamlValue, &envValue, nil, envValue},
		{"CLI replaces environment and YAML", yamlValue, &envValue, []string{"--" + credentialRootFlag + "=" + cliValue}, cliValue},
		{"winning empty environment", yamlValue, &empty, nil, empty},
		{"winning empty CLI", yamlValue, &envValue, []string{"--" + credentialRootFlag + "={}"}, empty},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := credentialRootLoad(t, tc.yaml, tc.environment, tc.args)
			require.NoError(t, err)
			var expected map[string]ports.CredentialFileBinding
			require.NoError(t, json.Unmarshal([]byte(tc.expected), &expected))
			if len(expected) == 0 {
				assert.Empty(t, cfg.ThirdPartyOAuth2.CredentialFiles)
			} else {
				assert.Equal(t, expected, cfg.ThirdPartyOAuth2.CredentialFiles)
			}
		})
	}
}

func TestCredentialFilesRootCLIRequiresStrictCompletePairObjects(t *testing.T) {
	cases := []struct {
		name  string
		value string
	}{
		{"empty input", ""},
		{"malformed JSON", `{"synthetic-private-input":`},
		{"trailing JSON", `{} synthetic-private-input`},
		{"null map", "null"},
		{"array map", "[]"},
		{"string map", `"synthetic-private-input"`},
		{"numeric map", "713579"},
		{"boolean map", "true"},
		{"null pair", `{"alpha":null}`},
		{"array pair", `{"alpha":[]}`},
		{"string pair", `{"alpha":"synthetic-private-input"}`},
		{"numeric pair", `{"alpha":713579}`},
		{"boolean pair", `{"alpha":true}`},
		{"missing both fields", `{"alpha":{}}`},
	}
	for _, field := range []string{"client_id_file", "client_secret_file"} {
		for _, invalid := range []struct {
			name    string
			value   any
			missing bool
		}{
			{"missing", nil, true}, {"null", nil, false}, {"number", 713579, false},
			{"boolean", true, false}, {"array", []string{"/synthetic-private/path"}, false},
			{"object", map[string]string{"path": "/synthetic-private/path"}, false},
			{"empty", "", false}, {"whitespace", " \t\n ", false}, {"relative", "synthetic-private/relative", false},
		} {
			pair := map[string]any{"client_id_file": "/synthetic-private/id", "client_secret_file": "/synthetic-private/secret"}
			if invalid.missing {
				delete(pair, field)
			} else {
				pair[field] = invalid.value
			}
			cases = append(cases, struct {
				name  string
				value string
			}{field + "/" + invalid.name, credentialRootJSON(t, map[string]any{"alpha": pair})})
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lower := `{"alpha":{"client_id_file":"/synthetic-private/lower-id","client_secret_file":"/synthetic-private/lower-secret"}}`
			_, err := credentialRootLoad(t, lower, &lower, []string{"--" + credentialRootFlag + "=" + tc.value})
			require.Error(t, err, "a winning invalid map must not inherit entries or fields from lower sources")
			assert.Contains(t, err.Error(), credentialRootFlag)
			for _, private := range []string{"/synthetic-private", "synthetic-private-input", "synthetic-private/relative", "713579"} {
				assert.NotContains(t, err.Error(), private)
			}
		})
	}
}

func TestCredentialFilesRootCLIUsesCanonicalGrammar(t *testing.T) {
	cases := []struct {
		key   string
		valid bool
	}{
		{"A", true}, {"7", true}, {"GitHub.Prod_1-2", true}, {".", true}, {"_", true}, {"-", true}, {strings.Repeat("a", 128), true},
		{"", false}, {"alpha beta", false}, {"alpha/beta", false}, {"alpha:beta", false}, {"alphä", false}, {strings.Repeat("a", 129), false},
		{"550e8400-e29b-41d4-a716-446655440000", false}, {"550E8400-E29B-41D4-A716-446655440000", false},
		{"550e8400e29b41d4a716446655440000", false}, {"{550e8400-e29b-41d4-a716-446655440000}", false},
		{"urn:uuid:550e8400-e29b-41d4-a716-446655440000", false}, {"00000000-0000-0000-0000-000000000000", false}, {"ffffffff-ffff-ffff-ffff-ffffffffffff", false},
	}
	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "id"), ClientSecretFile: filepath.Join(t.TempDir(), "secret")}
			bindings := map[string]ports.CredentialFileBinding{tc.key: binding}
			cfg, err := credentialRootLoad(t, "", nil, []string{"--" + credentialRootFlag + "=" + credentialRootJSON(t, bindings)})
			if tc.valid {
				require.NoError(t, err)
				assert.Equal(t, bindings, cfg.ThirdPartyOAuth2.CredentialFiles)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), credentialRootFlag)
				assert.NotContains(t, err.Error(), binding.ClientIDFile)
				assert.NotContains(t, err.Error(), binding.ClientSecretFile)
			}
		})
	}
}

func TestCredentialFilesRootWinningMapIgnoresLowerTypesAndExpansion(t *testing.T) {
	valid := `{"Winning.Pair":{"client_id_file":"/winning/id","client_secret_file":"/winning/secret"}}`
	empty := "{}"
	malformed := `{"synthetic-private-input":`
	for _, lowerYAML := range []string{"null", "[]", `{"alpha":{"client_id_file":"${CREDENTIAL_ROOT_UNDEFINED}/id","client_secret_file":"${CREDENTIAL_ROOT_UNDEFINED}/secret"}}`} {
		for _, source := range []string{"environment", "CLI", "empty CLI"} {
			t.Run(source+"/"+lowerYAML, func(t *testing.T) {
				t.Setenv("CREDENTIAL_ROOT_UNDEFINED", "")
				require.NoError(t, os.Unsetenv("CREDENTIAL_ROOT_UNDEFINED"))
				winner := valid
				environment := &valid
				var args []string
				if source != "environment" {
					environment = &malformed
					if source == "empty CLI" {
						winner = empty
					}
					args = []string{"--" + credentialRootFlag + "=" + winner}
				}
				cfg, err := credentialRootLoad(t, lowerYAML, environment, args)
				require.NoError(t, err, "only the winning map may be decoded, expanded, or validated")
				var expected map[string]ports.CredentialFileBinding
				require.NoError(t, json.Unmarshal([]byte(winner), &expected))
				if len(expected) == 0 {
					assert.Empty(t, cfg.ThirdPartyOAuth2.CredentialFiles)
				} else {
					assert.Equal(t, expected, cfg.ThirdPartyOAuth2.CredentialFiles)
				}
			})
		}
	}
}

func TestCredentialFilesRootConfigFlagSelectsFileBeforeLoadingBindings(t *testing.T) {
	t.Chdir(t.TempDir())
	environmentFile := credentialRootConfig(t, `{"environment-file":{"client_id_file":"/env-file/id","client_secret_file":"/env-file/secret"}}`)
	selectedFile := credentialRootConfig(t, `{"Selected.Case":{"client_id_file":"/selected/id","client_secret_file":"/selected/secret"}}`)
	credentialRootTestEnvironment(t, environmentFile)
	require.NoError(t, os.WriteFile("config.yaml", []byte("third_party_oauth2:\n  credential_files: null\n"), 0o600))
	require.NoError(t, credentialRootParse(t, []string{"--config", selectedFile}))
	loader := brokerconfig.NewLoader()
	loader.SetCommand(rootCmd)
	cfg, err := loader.GetConfig(context.Background())
	require.NoError(t, err)
	assert.Equal(t, map[string]ports.CredentialFileBinding{"Selected.Case": {ClientIDFile: "/selected/id", ClientSecretFile: "/selected/secret"}}, cfg.ThirdPartyOAuth2.CredentialFiles)
	var selected []string
	for _, source := range loader.GetSources() {
		if source.Type == ports.SourceTypeYAML {
			selected = append(selected, source.Path)
		}
	}
	assert.Equal(t, []string{selectedFile}, selected)
}

func TestCredentialFilesRootDoesNotRegisterOldSecretOnlyFlagAlias(t *testing.T) {
	t.Chdir(t.TempDir())
	err := credentialRootParse(t, []string{`--third_party_oauth2.client_secret_files={"alpha":"/synthetic-private/old-secret"}`})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown flag")
	assert.Contains(t, err.Error(), "third_party_oauth2.client_secret_files")
}

func TestCredentialFilesRootDiagnosticsRedactRenamedBinding(t *testing.T) {
	value := `{"Diagnostic.Pair":{"client_id_file":"/synthetic-private/id","client_secret_file":"/synthetic-private/secret"}}`
	cfg, err := credentialRootLoad(t, value, nil, nil)
	require.NoError(t, err)
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	defer func() { _ = reader.Close() }()
	output := make(chan string, 1)
	go func() {
		data, _ := io.ReadAll(reader)
		output <- string(data)
	}()
	displayConfigValue(credentialRootFlag, credentialRootJSON(t, cfg.ThirdPartyOAuth2.CredentialFiles), "yaml")
	displayConfigValue(credentialRootEnvironment, value, "environment")
	require.NoError(t, writer.Close())
	diagnostic := <-output
	assert.Contains(t, diagnostic, credentialRootFlag)
	assert.Contains(t, diagnostic, brokerconfig.RedactedValue)
	assert.NotContains(t, diagnostic, "/synthetic-private")
	assert.NotContains(t, diagnostic, value)
}

func credentialRootServer(t *testing.T, providerURL, value string) (*bootstrap.CredentialServer, error) {
	t.Helper()
	t.Chdir(t.TempDir())
	require.NoError(t, credentialRootParse(t, []string{"--" + credentialRootFlag + "=" + value}))
	server, err := bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{
		Directory: t.TempDir(),
		Document:  fixtures.CredentialConfigurationDocument(providerURL, map[string]any{}, 8000, 14000),
		Command:   rootCmd,
		Principal: fixtures.DefaultPrincipal().String(),
	})
	if server != nil {
		t.Cleanup(server.Close)
	}
	return server, err
}

func TestCredentialFilesRootCLIRejectsInvalidPairsBeforeListeners(t *testing.T) {
	for _, value := range []string{"null", "[]", `{"alpha":{"client_id_file":"/synthetic-private/id"}}`, `{"alpha":{"client_id_file":"synthetic-private/relative","client_secret_file":"/synthetic-private/secret"}}`} {
		t.Run(value, func(t *testing.T) {
			t.Chdir(t.TempDir())
			provider := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
			t.Cleanup(provider.Close)
			endUserPort, adminPort := freeBrokerPort(t), freeBrokerPort(t)
			document := fixtures.CredentialConfigurationDocument(provider.URL(), map[string]any{}, endUserPort, adminPort)
			encoded, err := json.Marshal(document)
			require.NoError(t, err)
			path := filepath.Join(t.TempDir(), "startup.yaml")
			require.NoError(t, os.WriteFile(path, encoded, 0o600))
			credentialRootTestEnvironment(t, path)
			require.NoError(t, credentialRootParse(t, []string{"--" + credentialRootFlag + "=" + value}))
			err = run(rootCmd, nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), credentialRootFlag)
			assert.NotContains(t, err.Error(), "unknown flag", "the registered CLI flag must reach loader validation")
			for _, port := range []int{endUserPort, adminPort} {
				connection, dialErr := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 250*time.Millisecond)
				if connection != nil {
					require.NoError(t, connection.Close())
				}
				assert.Error(t, dialErr, "invalid configuration must not expose either listener")
			}
			assert.Zero(t, provider.GetAuthorizationRequestCount())
			assert.Empty(t, provider.GetTokenRequests())
			assert.NotContains(t, err.Error(), "/synthetic-private")
			assert.NotContains(t, err.Error(), "synthetic-private/relative")
		})
	}
}

func TestCredentialFilesRootCLIUnknownUnavailableBindingsHaveNoStartupSideEffects(t *testing.T) {
	if os.Getenv("AIB_CREDENTIAL_CLI_FIFO_CHILD") != "1" {
		binary, err := os.Executable()
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "-test.run=^TestCredentialFilesRootCLIUnknownUnavailableBindingsHaveNoStartupSideEffects$", "-test.count=1", "-test.timeout=20s")
		command.Env = append(os.Environ(), "AIB_CREDENTIAL_CLI_FIFO_CHILD=1")
		output, err := command.CombinedOutput()
		require.NoError(t, ctx.Err(), "unused credential files blocked CLI startup: %s", output)
		require.NoError(t, err, "isolated production CLI bootstrap failed: %s", output)
		return
	}
	provider := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
	t.Cleanup(provider.Close)
	root := t.TempDir()
	fifo := filepath.Join(root, "unopened-fifo")
	require.NoError(t, syscall.Mkfifo(fifo, 0o600))
	bindings := map[string]map[string]string{
		"CLI.Unknown.FIFO":    {"client_id_file": fifo, "client_secret_file": fifo},
		"CLI.Unknown.Missing": {"client_id_file": filepath.Join(root, "missing-id"), "client_secret_file": filepath.Join(root, "missing-secret")},
	}
	process, err := credentialRootServer(t, provider.URL(), credentialRootJSON(t, bindings))
	require.NoError(t, err, "the actual CLI must start without accessing unavailable credentials or waiting for a FIFO writer")
	require.NotNil(t, process)
	response, err := process.AdminJSON(http.MethodGet, "/api/services", nil)
	require.NoError(t, err)
	var services []map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&services))
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Empty(t, services)
	assert.Zero(t, provider.GetAuthorizationRequestCount())
	assert.Empty(t, provider.GetTokenRequests())

	request := fixtures.CredentialServiceRequest("CLI.Unknown.FIFO", provider.URL(), "")
	response, err = process.AdminJSON(http.MethodPost, "/api/services", request)
	require.NoError(t, err)
	body, err := helpers.CredentialResponseObject(response)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	serviceID, ok := body["id"].(string)
	require.True(t, ok)
	assert.Equal(t, "stored", body["credential_source"])
	credentialRootConnectAndRefresh(t, process, provider, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
	assert.NotContains(t, process.Logs.Raw(), fifo)
	assert.NotContains(t, process.Logs.Raw(), filepath.Join(root, "missing-id"))
	assert.NotContains(t, process.Logs.Raw(), filepath.Join(root, "missing-secret"))

	filesystem := fixtures.CredentialServiceRequest("CLI.Unknown.Missing", provider.URL(), "filesystem")
	filesystem["protected_resources"] = []string{provider.URL() + "/filesystem-resource"}
	response, err = process.AdminJSON(http.MethodPost, "/api/services", filesystem)
	require.NoError(t, err)
	metadata, err := helpers.CredentialResponseObject(response)
	require.NoError(t, err)
	require.Equal(t, http.StatusCreated, response.StatusCode)
	assert.Equal(t, "filesystem", metadata["credential_source"])
	assert.NotContains(t, metadata, "client_id")
	assert.NotContains(t, metadata, "client_secret")
	filesystemID, ok := metadata["id"].(string)
	require.True(t, ok)
	response, err = process.AdminJSON(http.MethodGet, "/api/services/"+filesystemID, nil)
	require.NoError(t, err)
	read, err := helpers.CredentialResponseObject(response)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, response.StatusCode)
	assert.Equal(t, metadata, read)
	response, err = process.EndUserRequest(http.MethodGet, fmt.Sprintf("/api/third-party/%s/oauth2/authorize?redirect_uri=%s", filesystemID, url.QueryEscape(process.EndUserURL+"/done")), nil)
	require.NoError(t, err)
	failure, err := helpers.CredentialResponseObject(response)
	require.NoError(t, err)
	assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
	assert.Equal(t, "internal_error", failure["error"])
	assert.Len(t, provider.GetTokenRequests(), 2)
}

func credentialRootTestEnvironment(t *testing.T, path string) {
	t.Helper()
	setBrokerConfigPath(t, path)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(name, "IDENTITY_BROKER_") && name != "IDENTITY_BROKER_CONFIG_PATH" {
			require.NoError(t, os.Unsetenv(name))
		}
	}
}

func credentialRootConnectAndRefresh(t *testing.T, process *bootstrap.CredentialServer, provider *helpers.MockUpstreamOAuth2Server, serviceID, clientID, secret string) {
	t.Helper()
	before := len(provider.GetTokenRequests())
	response, err := process.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(process.EndUserURL+"/done"), nil)
	require.NoError(t, err)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, response.StatusCode)
	assert.Equal(t, provider.URL(), location.Scheme+"://"+location.Host)
	assert.Equal(t, clientID, location.Query().Get("client_id"))
	callback, err := helpers.ProviderAuthorizationCallback(location.String())
	require.NoError(t, err)
	response, err = process.EndUserRequest(http.MethodGet, callback.RequestURI(), nil)
	require.NoError(t, err)
	result, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "true", result.Query().Get("success"))
	assert.Empty(t, result.Query().Get("error"))
	response, err = process.EndUserRequest(http.MethodPost, "/api/third-party/"+serviceID+"/session/refresh", nil)
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusOK, response.StatusCode)
	requests := provider.GetTokenRequests()
	require.Len(t, requests, before+2)
	for i, grantType := range []string{"authorization_code", "refresh_token"} {
		selectedID, selectedSecret, err := helpers.CredentialTokenRequest(requests[before+i])
		require.NoError(t, err)
		assert.Equal(t, clientID, selectedID)
		assert.Equal(t, secret, selectedSecret)
		form, err := url.ParseQuery(requests[before+i].Body)
		require.NoError(t, err)
		assert.Equal(t, grantType, form.Get("grant_type"))
	}
}

func TestCredentialFilesRootCLIWholeMapPrecedenceAndWinningEmpty(t *testing.T) {
	for _, winningEmpty := range []bool{false, true} {
		t.Run(fmt.Sprintf("winning empty=%t", winningEmpty), func(t *testing.T) {
			t.Chdir(t.TempDir())
			journey, err := helpers.NewOAuth2CredentialJourney()
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, journey.Close()) })
			yamlPair, err := journey.WritePair("synthetic-yaml-client", "synthetic-yaml-secret")
			require.NoError(t, err)
			envPair, err := journey.WritePair("synthetic-env-client", "synthetic-env-secret")
			require.NoError(t, err)
			cliPair, err := journey.WritePair("synthetic-cli-client", "synthetic-cli-secret")
			require.NoError(t, err)
			winning := map[string]map[string]string{"CLI.Pair": cliPair}
			if winningEmpty {
				winning = map[string]map[string]string{}
			}
			require.NoError(t, credentialRootParse(t, []string{"--" + credentialRootFlag + "=" + credentialRootJSON(t, winning)}))
			process, err := bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{
				Directory:   t.TempDir(),
				Document:    fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]map[string]string{"CLI.Pair": yamlPair, "YAML.Only": yamlPair}, 8000, 14000),
				Environment: map[string]string{credentialRootEnvironment: credentialRootJSON(t, map[string]map[string]string{"CLI.Pair": envPair, "Env.Only": envPair})},
				Command:     rootCmd,
				Principal:   fixtures.DefaultPrincipal().String(),
			})
			if process != nil {
				t.Cleanup(process.Close)
			}
			require.NoError(t, err)
			require.NotNil(t, process)
			response, err := process.AdminJSON(http.MethodGet, "/api/services", nil)
			require.NoError(t, err)
			var services []map[string]any
			require.NoError(t, json.NewDecoder(response.Body).Decode(&services))
			require.NoError(t, response.Body.Close())
			assert.Empty(t, services)
			for _, canonicalID := range []string{"CLI.Pair", "YAML.Only", "Env.Only"} {
				request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), "filesystem")
				request["protected_resources"] = []string{journey.Upstream.URL() + "/resource/" + canonicalID}
				response, err := process.AdminJSON(http.MethodPost, "/api/services", request)
				require.NoError(t, err)
				metadata, err := helpers.CredentialResponseObject(response)
				require.NoError(t, err)
				require.Equal(t, http.StatusCreated, response.StatusCode)
				assert.Equal(t, "filesystem", metadata["credential_source"])
				assert.NotContains(t, metadata, "client_id")
				assert.NotContains(t, metadata, "client_secret")
				serviceID, ok := metadata["id"].(string)
				require.True(t, ok)
				if canonicalID == "CLI.Pair" && !winningEmpty {
					credentialRootConnectAndRefresh(t, process, journey.Upstream, serviceID, "synthetic-cli-client", "synthetic-cli-secret")
				} else {
					authorizations := journey.Upstream.GetAuthorizationRequestCount()
					tokens := len(journey.Upstream.GetTokenRequests())
					response, err = process.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(process.EndUserURL+"/done"), nil)
					require.NoError(t, err)
					failure, err := helpers.CredentialResponseObject(response)
					require.NoError(t, err)
					assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
					assert.Equal(t, "internal_error", failure["error"])
					assert.Equal(t, authorizations, journey.Upstream.GetAuthorizationRequestCount())
					assert.Len(t, journey.Upstream.GetTokenRequests(), tokens)
				}
				response, err = process.AdminJSON(http.MethodGet, "/api/services/"+serviceID, nil)
				require.NoError(t, err)
				read, err := helpers.CredentialResponseObject(response)
				require.NoError(t, err)
				assert.Equal(t, http.StatusOK, response.StatusCode)
				assert.Equal(t, metadata, read, "winning empty and missing entries must never change the registered source")
			}
			for _, private := range []string{yamlPair["client_id_file"], yamlPair["client_secret_file"], envPair["client_id_file"], envPair["client_secret_file"], cliPair["client_id_file"], cliPair["client_secret_file"], "synthetic-cli-client", "synthetic-cli-secret"} {
				assert.NotContains(t, process.Logs.Raw(), private)
			}
		})
	}
}
