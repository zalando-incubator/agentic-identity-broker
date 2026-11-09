package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/credentialfile"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Observe the real filesystem port, never replace its result. Zero calls is a
// metadata boundary assertion: a handler that reads and ignores errors also fails.
// The ordinary CRUD contract tests retain the unmodified Builder-created reader.
// WithCredentialFileReader is the same setter used by production Builder wiring.
type credentialMetadataReads struct {
	reader        ports.CredentialFileReader
	mu            sync.Mutex
	clientIDReads int
	pairReads     int
	failures      []error
}

func (r *credentialMetadataReads) ReadClientID(path string) (string, error) {
	value, err := r.reader.ReadClientID(path)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clientIDReads++
	if err != nil {
		r.failures = append(r.failures, err)
	}
	return value, err
}

func (r *credentialMetadataReads) ReadPair(clientIDPath, clientSecretPath string) (string, string, error) {
	clientID, clientSecret, err := r.reader.ReadPair(clientIDPath, clientSecretPath)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pairReads++
	if err != nil {
		r.failures = append(r.failures, err)
	}
	return clientID, clientSecret, err
}

func (r *credentialMetadataReads) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clientIDReads = 0
	r.pairReads = 0
	r.failures = nil
}

func (r *credentialMetadataReads) snapshot() (int, int, []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.clientIDReads, r.pairReads, append([]error(nil), r.failures...)
}

func assertCredentialMetadataReads(t *testing.T, reads *credentialMetadataReads, clientIDs, pairs int) {
	t.Helper()
	clientIDReads, pairReads, _ := reads.snapshot()
	assert.Equal(t, clientIDs, clientIDReads, "client-ID acquisitions")
	assert.Equal(t, pairs, pairReads, "coherent-pair acquisitions")
}

func newCredentialMetadataJourney(t *testing.T) (*helpers.OAuth2CredentialJourney, map[string]string, id.ServiceID, *credentialMetadataReads) {
	t.Helper()
	journey, binding := newCredentialAPIJourney(t)
	reads := &credentialMetadataReads{reader: credentialfile.New()}
	journey.Application.OAuth2SessionService.WithCredentialFileReader(reads)
	serviceID, _, err := journey.Register(credentialAPIServiceRequest(journey, "filesystem"))
	require.NoError(t, err)
	assertCredentialMetadataReads(t, reads, 0, 0)
	return journey, binding, serviceID, reads
}

func credentialMetadataBody(t *testing.T, response *http.Response) string {
	t.Helper()
	encoded, err := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	return string(encoded)
}

func assertCredentialMetadataSafe(t *testing.T, journey *helpers.OAuth2CredentialJourney, binding map[string]string, text string) {
	t.Helper()
	for _, forbidden := range []string{
		fixtures.CredentialClientID, fixtures.CredentialClientSecret,
		fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret,
		binding["client_id_file"], binding["client_secret_file"], journey.Directory,
		"provider-controlled-description", "Authorization: Basic", "client_secret=", "code_verifier=",
		"no such file or directory", "permission denied", "is a directory",
	} {
		assert.NotContains(t, text, forbidden)
	}
}

func credentialMetadataPendingCode(t *testing.T, journey *helpers.OAuth2CredentialJourney, serviceID id.ServiceID) *url.URL {
	t.Helper()
	before := len(journey.Upstream.GetTokenRequests())
	response, err := journey.Authorize(serviceID)
	require.NoError(t, err)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, journey.Upstream.URL(), location.Scheme+"://"+location.Host)
	require.Equal(t, fixtures.CredentialClientID, location.Query().Get("client_id"))
	assert.Len(t, journey.Upstream.GetTokenRequests(), before)
	callback, err := helpers.ProviderAuthorizationCallback(location.String())
	require.NoError(t, err)
	return callback
}

func credentialMetadataConnect(t *testing.T, journey *helpers.OAuth2CredentialJourney, serviceID id.ServiceID) {
	t.Helper()
	before := len(journey.Upstream.GetTokenRequests())
	callback := credentialMetadataPendingCode(t, journey, serviceID)
	response, err := journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
	require.NoError(t, err)
	location, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "true", location.Query().Get("success"))
	require.Empty(t, location.Query().Get("error"))
	requests := journey.Upstream.GetTokenRequests()
	require.Greater(t, len(requests), before)
	for _, request := range requests[before:] {
		clientID, clientSecret, err := helpers.CredentialTokenRequest(request)
		require.NoError(t, err)
		assert.Equal(t, fixtures.CredentialClientID, clientID)
		assert.Equal(t, fixtures.CredentialClientSecret, clientSecret)
		form, err := url.ParseQuery(request.Body)
		require.NoError(t, err)
		assert.Equal(t, "authorization_code", form.Get("grant_type"))
	}
}

func credentialMetadataEvents(t *testing.T, journey *helpers.OAuth2CredentialJourney, serviceID id.ServiceID, operation string) []map[string]any {
	t.Helper()
	records, err := journey.Logs.Records()
	require.NoError(t, err)
	matching := []map[string]any{}
	for _, record := range records {
		if record["service_id"] == serviceID.String() && record["operation"] == operation {
			if event, _ := record["event"].(string); strings.HasPrefix(event, "session.oauth2.credential_") {
				matching = append(matching, record)
			}
		}
	}
	return matching
}

func assertCredentialMetadataEvent(t *testing.T, records []map[string]any, event, outcome, reason string) {
	t.Helper()
	found := false
	for _, record := range records {
		if record["event"] == event {
			found = true
			assert.Equal(t, outcome, record["outcome"])
			if reason != "" {
				assert.Equal(t, reason, record["reason"])
			}
		}
	}
	assert.True(t, found, "expected %s for the actual service and operation; got %v", event, records)
}

// T057: independently lose each file after an established connection. Every
// metadata endpoint must still return the affected service, not an empty success.
func TestCredentialSourceMetadataAvailableDuringEachFileOutage(t *testing.T) {
	schemas := credentialAPISchemas(t)
	for _, file := range []string{"client_id_file", "client_secret_file"} {
		t.Run(file, func(t *testing.T) {
			journey, binding, serviceID, reads := newCredentialMetadataJourney(t)
			credentialMetadataConnect(t, journey, serviceID)
			ctx := context.Background()
			require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, journey.Storage, serviceID))
			agent := fixtures.ValidAgent()
			require.NoError(t, journey.Storage.Agents().Create(ctx, agent))
			require.NoError(t, journey.Storage.UserGrants().Create(ctx, fixtures.ActiveGrant(journey.Principal, agent.ID.String(), serviceID.String(), []string{"profile"})))
			path := "/api/services/" + serviceID.String()
			_, before := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			require.NoError(t, os.Remove(binding[file]))
			reads.reset()
			journey.Logs.Reset()

			_, read := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			assert.Equal(t, before, read)
			assertCredentialAPIService(t, schemas, read, "filesystem", binding)
			assertCredentialMetadataReads(t, reads, 0, 0)
			response, listed := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
			require.Equal(t, http.StatusOK, response.StatusCode)
			services, ok := listed.([]any)
			require.True(t, ok)
			found := false
			for _, value := range services {
				service := value.(map[string]any)
				assert.NoError(t, schemas["Service"].Validate(service), "every listed service must report its source")
				if service["id"] == serviceID.String() {
					found = true
					assertCredentialAPIService(t, schemas, service, "filesystem", binding)
				}
			}
			assert.True(t, found)
			encoded, err := json.Marshal(listed)
			require.NoError(t, err)
			assertCredentialMetadataSafe(t, journey, binding, string(encoded))
			assertCredentialMetadataReads(t, reads, 0, 0)

			metadata := credentialAPIServiceRequest(journey, "filesystem")
			delete(metadata, "credential_source")
			delete(metadata, "protected_resources")
			delete(metadata, "canonical_id")
			metadata["display_name"] = "Metadata during source outage"
			_, updated := credentialAPIObject(t, journey, http.MethodPut, path, metadata, nil, http.StatusOK)
			assertCredentialAPIService(t, schemas, updated, "filesystem", binding)
			assert.Equal(t, metadata["display_name"], updated["display_name"])
			assert.Equal(t, before["protected_resources"], updated["protected_resources"])
			assert.Equal(t, before["canonical_id"], updated["canonical_id"])
			assertCredentialMetadataReads(t, reads, 0, 0)

			beforeTokens := len(journey.Upstream.GetTokenRequests())
			beforeAuthorizations := journey.Upstream.GetAuthorizationRequestCount()
			for _, endpoint := range []string{
				"/api/consent/agents/" + agent.ID.String(),
				"/api/consent/agents/" + agent.ID.String() + "/grants",
				"/api/third-party/" + serviceID.String() + "/session",
				"/api/third-party/sessions",
			} {
				response, err := journey.EndUser.AuthenticatedGET(endpoint, journey.Principal)
				require.NoError(t, err)
				body := credentialMetadataBody(t, response)
				require.Equal(t, http.StatusOK, response.StatusCode, "%s: %s", endpoint, body)
				assert.Contains(t, body, serviceID.String())
				assertCredentialMetadataSafe(t, journey, binding, body)
				assert.NotContains(t, body, "upstream_client_id")
				assert.NotContains(t, body, "credential_source_transitioned")
				assertCredentialMetadataReads(t, reads, 0, 0)
			}
			assert.Len(t, journey.Upstream.GetTokenRequests(), beforeTokens)
			assert.Equal(t, beforeAuthorizations, journey.Upstream.GetAuthorizationRequestCount())
			assert.Empty(t, credentialMetadataEvents(t, journey, serviceID, "authorization_initiation"))
			assert.Empty(t, credentialMetadataEvents(t, journey, serviceID, "code_exchange"))
			assert.Empty(t, credentialMetadataEvents(t, journey, serviceID, "refresh"))
			assertCredentialMetadataSafe(t, journey, binding, journey.Logs.Raw())
		})
	}
}

func TestCredentialSourceMetadataInitiationRequiresOnlyClientID(t *testing.T) {
	for _, file := range []string{"client_id_file", "client_secret_file"} {
		t.Run(file, func(t *testing.T) {
			journey, binding, serviceID, reads := newCredentialMetadataJourney(t)
			require.NoError(t, os.Remove(binding[file]))
			reads.reset()
			journey.Logs.Reset()
			response, err := journey.Authorize(serviceID)
			require.NoError(t, err)
			body := credentialMetadataBody(t, response)
			assertCredentialMetadataReads(t, reads, 1, 0)
			assert.Empty(t, journey.Upstream.GetTokenRequests())
			assert.Zero(t, journey.Upstream.GetAuthorizationRequestCount())
			events := credentialMetadataEvents(t, journey, serviceID, "authorization_initiation")
			if file == "client_secret_file" {
				require.Equal(t, http.StatusFound, response.StatusCode)
				location, err := url.Parse(response.Header.Get("Location"))
				require.NoError(t, err)
				assert.Equal(t, fixtures.CredentialClientID, location.Query().Get("client_id"))
				assert.Equal(t, journey.Upstream.URL(), location.Scheme+"://"+location.Host)
				assertCredentialMetadataEvent(t, events, "session.oauth2.credential_files_used", "used", "")
				for _, event := range events {
					assert.NotEqual(t, "session.oauth2.credential_source_failed", event["event"])
				}
			} else {
				require.Equal(t, http.StatusInternalServerError, response.StatusCode)
				assert.Empty(t, response.Header.Get("Location"))
				var result map[string]any
				require.NoError(t, json.Unmarshal([]byte(body), &result))
				assert.Equal(t, "internal_error", result["error"])
				assertCredentialMetadataEvent(t, events, "session.oauth2.credential_source_failed", "failed", "not_found")
				for _, event := range events {
					assert.NotEqual(t, "session.oauth2.credential_files_used", event["event"])
				}
			}
			// The authorization Location legitimately carries client_id to the
			// provider; neither response error text nor operator logs may echo it.
			if file == "client_id_file" {
				assertCredentialMetadataSafe(t, journey, binding, body)
			}
			assertCredentialMetadataSafe(t, journey, binding, journey.Logs.Raw())
		})
	}
}

func TestCredentialSourceMetadataSourceErrorsAreSafeAtExchangeAndRefresh(t *testing.T) {
	for _, file := range []string{"client_id_file", "client_secret_file"} {
		for _, operation := range []string{"code_exchange", "refresh"} {
			t.Run(file+"/"+operation, func(t *testing.T) {
				journey, binding, serviceID, reads := newCredentialMetadataJourney(t)
				var callback *url.URL
				if operation == "refresh" {
					credentialMetadataConnect(t, journey, serviceID)
				} else {
					callback = credentialMetadataPendingCode(t, journey, serviceID)
				}
				beforeTokens := len(journey.Upstream.GetTokenRequests())
				beforeAuthorizations := journey.Upstream.GetAuthorizationRequestCount()
				require.NoError(t, os.Remove(binding[file]))
				reads.reset()
				journey.Logs.Reset()
				var response *http.Response
				var err error
				if operation == "refresh" {
					response, err = journey.Refresh(serviceID)
				} else {
					response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
				}
				require.NoError(t, err)
				body := credentialMetadataBody(t, response)
				if operation == "refresh" {
					require.Equal(t, http.StatusInternalServerError, response.StatusCode)
					var result map[string]any
					require.NoError(t, json.Unmarshal([]byte(body), &result))
					assert.Equal(t, "internal_error", result["error"])
					assert.Empty(t, response.Header.Get("Location"))
				} else {
					require.Equal(t, http.StatusFound, response.StatusCode)
					location, err := url.Parse(response.Header.Get("Location"))
					require.NoError(t, err)
					assert.Equal(t, "/sessions", location.Path)
					assert.Equal(t, "callback_failed", location.Query().Get("error"))
					assert.Equal(t, "Authorization failed - please try again", location.Query().Get("error_description"))
					assert.Empty(t, location.Query().Get("success"))
					assertCredentialMetadataSafe(t, journey, binding, location.String())
				}
				assertCredentialMetadataReads(t, reads, 0, 1)
				assert.Len(t, journey.Upstream.GetTokenRequests(), beforeTokens)
				assert.Equal(t, beforeAuthorizations, journey.Upstream.GetAuthorizationRequestCount())
				_, _, failures := reads.snapshot()
				require.Len(t, failures, 1)
				assert.True(t, errors.Is(failures[0], model.ErrCredentialSourceUnavailable))
				assert.False(t, errors.Is(failures[0], oauth2session.ErrRefreshFailed), "source unavailability is not provider rejection")
				var pathError *os.PathError
				assert.False(t, errors.As(failures[0], &pathError), "safe source errors must not expose an OS error chain")
				for cause := failures[0]; cause != nil; cause = errors.Unwrap(cause) {
					assertCredentialMetadataSafe(t, journey, binding, fmt.Sprintf("%v %+v", cause, cause))
				}
				events := credentialMetadataEvents(t, journey, serviceID, operation)
				assertCredentialMetadataEvent(t, events, "session.oauth2.credential_source_failed", "failed", "not_found")
				for _, event := range events {
					assert.NotEqual(t, "session.oauth2.credential_files_used", event["event"])
					assert.NotEqual(t, "session.oauth2.credential_provider_rejected", event["event"])
				}
				assertCredentialMetadataSafe(t, journey, binding, body+journey.Logs.Raw())
			})
		}
	}
}

func TestCredentialSourceMetadataUsablePairProviderRejectionIsDistinctAndSafe(t *testing.T) {
	for _, operation := range []string{"code_exchange", "refresh"} {
		t.Run(operation, func(t *testing.T) {
			journey, binding, serviceID, reads := newCredentialMetadataJourney(t)
			var callback *url.URL
			if operation == "refresh" {
				credentialMetadataConnect(t, journey, serviceID)
			} else {
				callback = credentialMetadataPendingCode(t, journey, serviceID)
			}
			beforeTokens := len(journey.Upstream.GetTokenRequests())
			journey.Upstream.WithErrorResponseAndDescription("invalid_client", strings.Join([]string{
				"provider-controlled-description", fixtures.CredentialClientID, fixtures.CredentialClientSecret,
				binding["client_id_file"], binding["client_secret_file"],
			}, " "))
			reads.reset()
			journey.Logs.Reset()
			var response *http.Response
			var err error
			if operation == "refresh" {
				response, err = journey.Refresh(serviceID)
			} else {
				response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
			}
			require.NoError(t, err)
			body := credentialMetadataBody(t, response)
			if operation == "refresh" {
				require.Equal(t, http.StatusBadGateway, response.StatusCode)
				var result map[string]any
				require.NoError(t, json.Unmarshal([]byte(body), &result))
				assert.Equal(t, "refresh_failed", result["error"])
			} else {
				require.Equal(t, http.StatusFound, response.StatusCode)
				location, err := url.Parse(response.Header.Get("Location"))
				require.NoError(t, err)
				assert.Equal(t, "/sessions", location.Path)
				assert.Equal(t, "callback_failed", location.Query().Get("error"))
				assertCredentialMetadataSafe(t, journey, binding, location.String())
			}
			assertCredentialMetadataReads(t, reads, 0, 1)
			_, _, failures := reads.snapshot()
			assert.Empty(t, failures, "usable file acquisition succeeded before the provider rejected authentication")
			requests := journey.Upstream.GetTokenRequests()
			require.Greater(t, len(requests), beforeTokens)
			for _, request := range requests[beforeTokens:] {
				clientID, clientSecret, err := helpers.CredentialTokenRequest(request)
				require.NoError(t, err)
				assert.Equal(t, fixtures.CredentialClientID, clientID)
				assert.Equal(t, fixtures.CredentialClientSecret, clientSecret)
			}
			events := credentialMetadataEvents(t, journey, serviceID, operation)
			assertCredentialMetadataEvent(t, events, "session.oauth2.credential_files_used", "used", "")
			assertCredentialMetadataEvent(t, events, "session.oauth2.credential_provider_rejected", "rejected", "provider_rejected")
			for _, event := range events {
				assert.NotEqual(t, "session.oauth2.credential_source_failed", event["event"])
			}
			assertCredentialMetadataSafe(t, journey, binding, body+journey.Logs.Raw())
		})
	}
}

func TestCredentialSourceMetadataConfigurationDiagnosticsDoNotDiscloseInputs(t *testing.T) {
	journey, binding := newCredentialAPIJourney(t)
	assertCredentialMetadataSafe(t, journey, binding, journey.Logs.Raw())
	for _, field := range []string{"client_id_file", "client_secret_file"} {
		for _, invalid := range []any{
			fixtures.CredentialClientSecret,
			map[string]any{"path": binding[field], "contents": fixtures.CredentialClientID + fixtures.CredentialClientSecret},
			nil,
			true,
		} {
			pair := map[string]any{"client_id_file": binding["client_id_file"], "client_secret_file": binding["client_secret_file"]}
			pair[field] = invalid
			encoded, err := json.Marshal(map[string]any{"api-source": pair})
			require.NoError(t, err)
			_, diagnostic := bootstrap.LoadCredentialConfiguration(journey.ConfigPath, map[string]string{
				"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": string(encoded),
			}, nil)
			require.Error(t, diagnostic)
			assert.Contains(t, diagnostic.Error(), "credential_files", "retain useful field-level diagnostics without raw input")
			assertCredentialMetadataSafe(t, journey, binding, diagnostic.Error())
		}
	}
}

func TestCredentialSourceMetadataMixedRequestsNeverAcquireFiles(t *testing.T) {
	journey, binding, serviceID, reads := newCredentialMetadataJourney(t)
	require.NoError(t, os.Remove(binding["client_id_file"]))
	require.NoError(t, os.Remove(binding["client_secret_file"]))
	path := "/api/services/" + serviceID.String()
	beforeResponse, before := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	reads.reset()
	for _, method := range []string{http.MethodPost, http.MethodPut} {
		for _, field := range []string{"client_id", "client_secret"} {
			for _, value := range []any{nil, "", "inline-synthetic-value"} {
				for _, omitSource := range []bool{false, true} {
					if method == http.MethodPost && omitSource {
						continue // Creation omission means stored, not preservation of filesystem.
					}
					request := credentialAPIServiceRequest(journey, "filesystem")
					delete(request, "protected_resources")
					request[field] = value
					request["display_name"] = "Rejected mixed-source metadata"
					if omitSource {
						delete(request, "credential_source")
					}
					target := path
					if method == http.MethodPost {
						target = "/api/services"
						request["canonical_id"] = "rejected-filesystem-source"
					}
					credentialAPIObject(t, journey, method, target, request, nil, http.StatusBadRequest)
					response, after := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
					assert.Equal(t, before, after)
					assert.Equal(t, beforeResponse.Header.Get("ETag"), response.Header.Get("ETag"))
					assertCredentialMetadataReads(t, reads, 0, 0)
					response, services := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
					require.Equal(t, http.StatusOK, response.StatusCode)
					assert.Equal(t, []any{before}, services)
				}
			}
		}
	}
	assert.Empty(t, journey.Upstream.GetTokenRequests())
	assert.Zero(t, journey.Upstream.GetAuthorizationRequestCount())
}
