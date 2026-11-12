package config_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func credentialRuntimeOptions(t *testing.T, providerURL, mode, source string, bindings any) bootstrap.CredentialServerOptions {
	t.Helper()
	document := fixtures.CredentialConfigurationDocument(providerURL, map[string]any{}, 8000, 14000)
	authorizationServer := document["oauth2_authorization_server"].(map[string]any)
	authorizationServer["mode"] = mode
	if mode == "local" {
		delete(authorizationServer, "proxy")
		document["token_exchange"].(map[string]any)["client_assertion"] = map[string]any{"issuer_uri": providerURL}
	}
	options := bootstrap.CredentialServerOptions{Directory: t.TempDir(), Document: document, Principal: fixtures.DefaultPrincipal().String()}
	switch source {
	case "YAML":
		document["third_party_oauth2"].(map[string]any)["credential_files"] = bindings
	case "environment":
		encoded, err := json.Marshal(bindings)
		require.NoError(t, err)
		options.Environment = map[string]string{"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": string(encoded)}
	default:
		t.Fatalf("unsupported runtime source %q", source)
	}
	return options
}

func credentialRuntimeLaunch(t *testing.T, options bootstrap.CredentialServerOptions) (*bootstrap.CredentialServer, error) {
	t.Helper()
	process, err := bootstrap.StartCredentialServer(options)
	if process != nil {
		t.Cleanup(process.Close)
	}
	return process, err
}

func credentialRuntimeObject(t *testing.T, process *bootstrap.CredentialServer, method, path string, request any, status int) map[string]any {
	t.Helper()
	response, err := process.AdminJSON(method, path, request)
	require.NoError(t, err)
	body, err := helpers.CredentialResponseObject(response)
	require.NoError(t, err)
	require.Equal(t, status, response.StatusCode)
	return body
}

func credentialRuntimeServices(t *testing.T, process *bootstrap.CredentialServer) []map[string]any {
	t.Helper()
	response, err := process.AdminJSON(http.MethodGet, "/api/services", nil)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	var services []map[string]any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&services))
	require.Equal(t, http.StatusOK, response.StatusCode)
	return services
}

func credentialRuntimeRegister(t *testing.T, process *bootstrap.CredentialServer, providerURL, canonicalID, source string) string {
	t.Helper()
	request := fixtures.CredentialServiceRequest(canonicalID, providerURL, source)
	request["protected_resources"] = []string{providerURL + "/resource/" + canonicalID}
	body := credentialRuntimeObject(t, process, http.MethodPost, "/api/services", request, http.StatusCreated)
	serviceID, ok := body["id"].(string)
	require.True(t, ok)
	require.NotEmpty(t, serviceID)
	return serviceID
}

func credentialRuntimeConnectAndRefresh(t *testing.T, process *bootstrap.CredentialServer, provider *helpers.MockUpstreamOAuth2Server, serviceID, clientID, secret string) {
	t.Helper()
	before := len(provider.GetTokenRequests())
	response, err := process.EndUserRequest(http.MethodGet, "/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(process.EndUserURL+"/done"), nil)
	require.NoError(t, err)
	location := response.Header.Get("Location")
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	authorize, err := url.Parse(location)
	require.NoError(t, err)
	assert.Equal(t, provider.URL(), authorize.Scheme+"://"+authorize.Host)
	assert.Equal(t, clientID, authorize.Query().Get("client_id"))
	callback, err := helpers.ProviderAuthorizationCallback(location)
	require.NoError(t, err)
	assert.Equal(t, process.EndUserURL, callback.Scheme+"://"+callback.Host)
	response, err = process.EndUserRequest(http.MethodGet, callback.RequestURI(), nil)
	require.NoError(t, err)
	result, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusFound, response.StatusCode)
	assert.Equal(t, "true", result.Query().Get("success"))
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
		assert.Empty(t, requests[before+i].Header.Get("X-Remote-User"))
	}
}

func TestCredentialFilesRuntimeSelectsOnlyRegisteredSourcesInSupportedModes(t *testing.T) {
	for _, mode := range []string{"local", "proxy", "hybrid"} {
		for _, source := range []string{"YAML", "environment"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				journey, err := helpers.NewOAuth2CredentialJourney()
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, journey.Close()) })
				pair, err := journey.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				require.NoError(t, err)
				bindings := map[string]map[string]string{"Registered.Stored": pair, "Registered.Filesystem": pair}
				process, err := credentialRuntimeLaunch(t, credentialRuntimeOptions(t, journey.Upstream.URL(), mode, source, bindings))
				require.NoError(t, err)
				require.NotNil(t, process)
				assert.Empty(t, credentialRuntimeServices(t, process), "configuration must not create registrations")
				assert.Zero(t, journey.Upstream.GetAuthorizationRequestCount())
				assert.Empty(t, journey.Upstream.GetTokenRequests())

				storedID := credentialRuntimeRegister(t, process, journey.Upstream.URL(), "Registered.Stored", "")
				storedBefore := credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK)
				credentialRuntimeConnectAndRefresh(t, process, journey.Upstream, storedID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				assert.Equal(t, "stored", storedBefore["credential_source"])
				assert.Equal(t, fixtures.CredentialClientID, storedBefore["client_id"])
				assert.Equal(t, "REDACTED", storedBefore["client_secret"])
				assert.Equal(t, storedBefore, credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK))

				filesystemID := credentialRuntimeRegister(t, process, journey.Upstream.URL(), "Registered.Filesystem", "filesystem")
				filesystemBefore := credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+filesystemID, nil, http.StatusOK)
				credentialRuntimeConnectAndRefresh(t, process, journey.Upstream, filesystemID, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				assert.Equal(t, "filesystem", filesystemBefore["credential_source"])
				assert.NotContains(t, filesystemBefore, "client_id")
				assert.NotContains(t, filesystemBefore, "client_secret")
				assert.Equal(t, filesystemBefore, credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+filesystemID, nil, http.StatusOK))
				for _, private := range []string{pair["client_id_file"], pair["client_secret_file"], fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret} {
					assert.NotContains(t, process.Logs.Raw(), private)
				}
			})
		}
	}
}

func TestCredentialFilesRuntimeUnavailableAndFIFOPathsDoNotBlockStartupOrMetadata(t *testing.T) {
	if os.Getenv("AIB_CREDENTIAL_RUNTIME_FIFO_CHILD") != "1" {
		binary, err := os.Executable()
		require.NoError(t, err)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, binary, "-test.run=^TestCredentialFilesRuntimeUnavailableAndFIFOPathsDoNotBlockStartupOrMetadata$", "-test.count=1", "-test.timeout=20s")
		command.Env = append(os.Environ(), "AIB_CREDENTIAL_RUNTIME_FIFO_CHILD=1")
		output, err := command.CombinedOutput()
		require.NoError(t, ctx.Err(), "unused credential files blocked startup: %s", output)
		require.NoError(t, err, "isolated production bootstrap failed: %s", output)
		return
	}
	for _, source := range []string{"YAML", "environment"} {
		t.Run(source, func(t *testing.T) {
			provider := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
			t.Cleanup(provider.Close)
			root := t.TempDir()
			fifo := filepath.Join(root, "unopened-fifo")
			require.NoError(t, syscall.Mkfifo(fifo, 0o600))
			unreadable := filepath.Join(root, "unreadable")
			require.NoError(t, os.WriteFile(unreadable, []byte("synthetic-never-read-file-contents"), 0o000))
			broken := filepath.Join(root, "broken-link")
			require.NoError(t, os.Symlink(filepath.Join(root, "missing-target"), broken))
			bindings := map[string]map[string]string{
				"Unknown.FIFO":       {"client_id_file": fifo, "client_secret_file": fifo},
				"Unknown.Missing":    {"client_id_file": filepath.Join(root, "missing-id"), "client_secret_file": filepath.Join(root, "missing-secret")},
				"Unknown.Unreadable": {"client_id_file": unreadable, "client_secret_file": unreadable},
				"Unknown.Nonregular": {"client_id_file": root, "client_secret_file": broken},
			}
			process, err := credentialRuntimeLaunch(t, credentialRuntimeOptions(t, provider.URL(), "proxy", source, bindings))
			require.NoError(t, err, "startup must complete without resolving unused files or waiting for a FIFO writer")
			require.NotNil(t, process)
			assert.Empty(t, credentialRuntimeServices(t, process))
			assert.Zero(t, provider.GetAuthorizationRequestCount())
			assert.Empty(t, provider.GetTokenRequests())
			storedID := credentialRuntimeRegister(t, process, provider.URL(), "Unknown.FIFO", "")
			credentialRuntimeConnectAndRefresh(t, process, provider, storedID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			stored := credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK)
			assert.Equal(t, "stored", stored["credential_source"])
			filesystemID := credentialRuntimeRegister(t, process, provider.URL(), "Unknown.Missing", "filesystem")
			metadata := credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+filesystemID, nil, http.StatusOK)
			assert.Equal(t, "filesystem", metadata["credential_source"])
			assert.NotContains(t, metadata, "client_id")
			assert.NotContains(t, metadata, "client_secret")
			assert.Len(t, credentialRuntimeServices(t, process), 2)
			before := len(provider.GetTokenRequests())
			response, err := process.EndUserRequest(http.MethodGet, "/api/third-party/"+filesystemID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(process.EndUserURL+"/done"), nil)
			require.NoError(t, err)
			body, err := helpers.CredentialResponseObject(response)
			require.NoError(t, err)
			assert.Equal(t, http.StatusInternalServerError, response.StatusCode)
			assert.Equal(t, "internal_error", body["error"])
			assert.Empty(t, response.Header.Get("Location"))
			assert.Len(t, provider.GetTokenRequests(), before)
			assert.Equal(t, metadata, credentialRuntimeObject(t, process, http.MethodGet, "/api/services/"+filesystemID, nil, http.StatusOK))
			for _, private := range []string{fifo, unreadable, broken, "synthetic-never-read-file-contents"} {
				assert.NotContains(t, process.Logs.Raw(), private)
			}
		})
	}
}

func TestCredentialFilesRuntimeRejectsInvalidConfigurationBeforeListeners(t *testing.T) {
	cases := []struct {
		name     string
		bindings any
	}{
		{"null map", nil},
		{"array map", []any{}},
		{"undashed UUID", map[string]any{"550e8400e29b41d4a716446655440000": map[string]any{"client_id_file": "/synthetic-private/id", "client_secret_file": "/synthetic-private/secret"}}},
		{"missing field", map[string]any{"alpha": map[string]any{"client_id_file": "/synthetic-private/id"}}},
		{"null path", map[string]any{"alpha": map[string]any{"client_id_file": "/synthetic-private/id", "client_secret_file": nil}}},
		{"numeric path", map[string]any{"alpha": map[string]any{"client_id_file": 713579, "client_secret_file": "/synthetic-private/secret"}}},
		{"relative path", map[string]any{"alpha": map[string]any{"client_id_file": "/synthetic-private/id", "client_secret_file": "synthetic-private/relative"}}},
	}
	for _, source := range []string{"YAML", "environment"} {
		for _, tc := range cases {
			t.Run(source+"/"+tc.name, func(t *testing.T) {
				provider := helpers.NewMockUpstreamOAuth2Server().WithSuccessfulTokenResponse()
				t.Cleanup(provider.Close)
				process, err := credentialRuntimeLaunch(t, credentialRuntimeOptions(t, provider.URL(), "proxy", source, tc.bindings))
				require.Error(t, err)
				require.NotNil(t, process)
				assert.Contains(t, process.Logs.Raw(), "third_party_oauth2.credential_files")
				for _, baseURL := range []string{process.AdminURL, process.EndUserURL} {
					address, err := url.Parse(baseURL)
					require.NoError(t, err)
					connection, dialErr := net.DialTimeout("tcp", address.Host, 250*time.Millisecond)
					if connection != nil {
						require.NoError(t, connection.Close())
					}
					assert.Error(t, dialErr, "invalid configuration must not expose either listener")
				}
				assert.Zero(t, provider.GetAuthorizationRequestCount())
				assert.Empty(t, provider.GetTokenRequests())
				for _, private := range []string{"/synthetic-private", "synthetic-private/relative", "713579"} {
					assert.NotContains(t, process.Logs.Raw(), private)
				}
			})
		}
	}
}
