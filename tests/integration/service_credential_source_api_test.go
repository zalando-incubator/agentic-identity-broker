package integration_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/santhosh-tekuri/jsonschema/v6"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
)

// These contracts use the same loader, production Builder, separate administrative
// and end-user routers, and protocol provider as the acceptance journeys.
func newCredentialAPIJourney(t *testing.T) (*helpers.OAuth2CredentialJourney, map[string]string) {
	t.Helper()
	journey, err := helpers.NewOAuth2CredentialJourney()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, journey.Close()) })
	binding, err := journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
	require.NoError(t, err)
	require.NoError(t, journey.Start(map[string]map[string]string{"api-source": binding}))
	return journey, binding
}

func credentialAPIRequest(t *testing.T, journey *helpers.OAuth2CredentialJourney, method, path string, body any, headers map[string]string) (*http.Response, any) {
	t.Helper()
	var encoded []byte
	if body != nil {
		var err error
		encoded, err = json.Marshal(body)
		require.NoError(t, err)
	}
	requestHeaders := map[string]string{"Content-Type": "application/json"}
	for key, value := range headers {
		requestHeaders[key] = value
	}
	response, err := journey.Admin.DirectRequest(method, path, journey.Principal, requestHeaders, bytes.NewReader(encoded))
	require.NoError(t, err)
	defer func() { require.NoError(t, response.Body.Close()) }()
	var result any
	require.NoError(t, json.NewDecoder(response.Body).Decode(&result))
	return response, result
}

func credentialAPIObject(t *testing.T, journey *helpers.OAuth2CredentialJourney, method, path string, body any, headers map[string]string, status int) (*http.Response, map[string]any) {
	t.Helper()
	response, result := credentialAPIRequest(t, journey, method, path, body, headers)
	require.Equal(t, status, response.StatusCode, "response: %v", result)
	object, ok := result.(map[string]any)
	require.True(t, ok, "expected an object, got %T", result)
	return response, object
}

func credentialAPIServiceRequest(journey *helpers.OAuth2CredentialJourney, source string) map[string]any {
	return fixtures.CredentialServiceRequest("api-source", journey.Upstream.URL(), source)
}

func credentialSchemaValue(t *testing.T, value any) any {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var decoded any
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	return decoded
}

// OpenAPI 3.0's nullable is the only dialect adaptation. In particular, retain
// required, oneOf, not, enums and field-presence constraints from the approved API.
func credentialOpenAPINullable(value any) {
	switch node := value.(type) {
	case map[string]any:
		if nullable, _ := node["nullable"].(bool); nullable {
			if kind, ok := node["type"].(string); ok {
				node["type"] = []any{kind, "null"}
			}
		}
		delete(node, "nullable")
		for _, child := range node {
			credentialOpenAPINullable(child)
		}
	case []any:
		for _, child := range node {
			credentialOpenAPINullable(child)
		}
	}
}

func credentialAPISchemas(t *testing.T) map[string]*jsonschema.Schema {
	t.Helper()
	encoded, err := os.ReadFile("../../api/admin/openapi.yaml")
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, yaml.Unmarshal(encoded, &document))
	require.Equal(t, "2.0.0", document["info"].(map[string]any)["version"])
	components := document["components"].(map[string]any)
	root := map[string]any{
		"$schema":    "http://json-schema.org/draft-07/schema#",
		"components": map[string]any{"schemas": components["schemas"]},
	}
	credentialOpenAPINullable(root)
	compiler := jsonschema.NewCompiler()
	const location = "https://broker.test/admin-contract.json"
	require.NoError(t, compiler.AddResource(location, root))
	schemas := make(map[string]*jsonschema.Schema)
	for _, name := range []string{"ServiceCreateRequest", "ServiceUpdateRequest", "Service"} {
		schema, err := compiler.Compile(location + "#/components/schemas/" + name)
		require.NoError(t, err)
		schemas[name] = schema
	}
	return schemas
}

func assertCredentialAPIService(t *testing.T, schemas map[string]*jsonschema.Schema, body map[string]any, source string, binding map[string]string) {
	t.Helper()
	assert.NoError(t, schemas["Service"].Validate(body), "actual HTTP representation must satisfy Admin API 2.0.0")
	assert.Equal(t, source, body["credential_source"])
	for _, field := range []string{"client_id_file", "client_secret_file", "credential_files", "credential_source_transitioned"} {
		assert.NotContains(t, body, field)
	}
	encoded, err := json.Marshal(body)
	require.NoError(t, err)
	for _, forbidden := range []string{fixtures.CredentialClientSecret, binding["client_id_file"], binding["client_secret_file"]} {
		assert.NotContains(t, string(encoded), forbidden)
	}
	if source == "filesystem" {
		assert.NotContains(t, body, "client_id")
		assert.NotContains(t, body, "client_secret")
		assert.NotContains(t, string(encoded), fixtures.CredentialClientID)
		assert.NotContains(t, string(encoded), "REDACTED", "filesystem mode must not invent a stored secret")
	} else {
		assert.IsType(t, "", body["client_id"])
		assert.NotEmpty(t, body["client_id"])
		if body["token_endpoint_auth_method"] == nil {
			assert.Equal(t, "REDACTED", body["client_secret"])
		} else {
			assert.NotContains(t, body, "client_secret")
		}
	}
}

func TestServiceCredentialSourceAPIAllCRUDRepresentations(t *testing.T) {
	schemas := credentialAPISchemas(t)
	for _, mode := range []string{"default-stored", "stored", "filesystem", "filesystem-github", "public-stored"} {
		t.Run(mode, func(t *testing.T) {
			journey, binding := newCredentialAPIJourney(t)
			source := "stored"
			request := credentialAPIServiceRequest(journey, source)
			switch mode {
			case "default-stored":
				delete(request, "credential_source")
			case "filesystem", "filesystem-github":
				source = "filesystem"
				request = credentialAPIServiceRequest(journey, source)
				if mode == "filesystem-github" {
					request["oauth2_flavor"] = "github"
				}
			case "public-stored":
				request["token_endpoint_auth_method"] = "none"
				request["client_secret"] = nil
			}
			require.NoError(t, schemas["ServiceCreateRequest"].Validate(credentialSchemaValue(t, request)))
			_, created := credentialAPIObject(t, journey, http.MethodPost, "/api/services", request, nil, http.StatusCreated)
			assertCredentialAPIService(t, schemas, created, source, binding)
			if source == "stored" {
				assert.Equal(t, fixtures.CredentialClientID, created["client_id"])
			}
			serviceID := created["id"].(string)
			path := "/api/services/" + serviceID
			response, read := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			assertCredentialAPIService(t, schemas, read, source, binding)
			assert.Equal(t, created, read)
			require.Regexp(t, `^"[0-9]+"$`, response.Header.Get("ETag"))
			_, byCanonicalID := credentialAPIObject(t, journey, http.MethodGet, "/api/services/api-source", nil, nil, http.StatusOK)
			assert.Equal(t, read, byCanonicalID)

			// Source and canonical-ID omissions are documented exceptions, not PATCH.
			delete(request, "credential_source")
			delete(request, "canonical_id")
			delete(request, "protected_resources")
			request["display_name"] = "Replacement metadata"
			require.NoError(t, schemas["ServiceUpdateRequest"].Validate(credentialSchemaValue(t, request)))
			_, updated := credentialAPIObject(t, journey, http.MethodPut, path, request, nil, http.StatusOK)
			assertCredentialAPIService(t, schemas, updated, source, binding)
			if source == "stored" {
				assert.Equal(t, fixtures.CredentialClientID, updated["client_id"])
			}
			assert.Equal(t, "api-source", updated["canonical_id"])
			assert.Equal(t, "Replacement metadata", updated["display_name"])
			assert.Equal(t, created["created_at"], updated["created_at"])
			assert.Equal(t, created["protected_resources"], updated["protected_resources"])
			_, read = credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			assert.Equal(t, updated, read)
			assertCredentialAPIService(t, schemas, read, source, binding)
			response, listed := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
			require.Equal(t, http.StatusOK, response.StatusCode)
			services, ok := listed.([]any)
			require.True(t, ok)
			require.Len(t, services, 1)
			assert.Equal(t, read, services[0])
			assertCredentialAPIService(t, schemas, services[0].(map[string]any), source, binding)
			assert.Empty(t, journey.Upstream.GetTokenRequests())
			assert.Zero(t, journey.Upstream.GetAuthorizationRequestCount())
		})
	}
}

func TestServiceCredentialSourceAPIRejectsInvalidRequestsWithoutMutation(t *testing.T) {
	schemas := credentialAPISchemas(t)
	type invalidCase struct {
		name   string
		source string
		field  string
		value  any
		omit   bool
	}
	cases := []invalidCase{}
	for _, value := range []any{nil, "", "Filesystem", " filesystem", "unknown", 1, true, []any{"filesystem"}, map[string]any{"source": "filesystem"}} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		cases = append(cases, invalidCase{name: "selector=" + string(encoded), source: "stored", field: "credential_source", value: value})
	}
	for _, field := range []string{"client_id", "client_secret"} {
		for _, value := range []any{nil, "", "inline-synthetic-value", 1, true, []any{}, map[string]any{}} {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			cases = append(cases, invalidCase{name: "filesystem/" + field + "=" + string(encoded), source: "filesystem", field: field, value: value})
		}
		for _, value := range []any{nil, "", 1, true, []any{}, map[string]any{}} {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			cases = append(cases, invalidCase{name: "stored/" + field + "=" + string(encoded), source: "stored", field: field, value: value})
		}
		cases = append(cases, invalidCase{name: "stored/omit-" + field, source: "stored", field: field, omit: true})
	}
	for _, value := range []any{nil, "", " ", "invalid/slash", strings.Repeat("x", 129), "00000000-0000-0000-0000-000000000001", 1, true, []any{}, map[string]any{}} {
		encoded, err := json.Marshal(value)
		require.NoError(t, err)
		cases = append(cases, invalidCase{name: "filesystem/canonical=" + string(encoded), source: "filesystem", field: "canonical_id", value: value})
	}
	for _, value := range []any{"none", "private_key_jwt"} {
		cases = append(cases, invalidCase{name: "excluded/" + value.(string), source: "filesystem", field: "token_endpoint_auth_method", value: value})
	}
	cases = append(cases, invalidCase{name: "excluded/google", source: "filesystem", field: "oauth2_flavor", value: "google"})
	for _, field := range []string{"client_id_file", "client_secret_file", "credential_files", "credential_source_transitioned"} {
		for _, value := range []any{nil, "", "/administrator-controlled/path", map[string]any{"api-source": "/path"}, true} {
			encoded, err := json.Marshal(value)
			require.NoError(t, err)
			cases = append(cases, invalidCase{name: "configuration-boundary/" + field + "=" + string(encoded), source: "stored", field: field, value: value})
		}
	}

	for _, method := range []string{http.MethodPost, http.MethodPut} {
		t.Run(method, func(t *testing.T) {
			journey, _ := newCredentialAPIJourney(t)
			_, created := credentialAPIObject(t, journey, http.MethodPost, "/api/services", credentialAPIServiceRequest(journey, "stored"), nil, http.StatusCreated)
			path := "/api/services/" + created["id"].(string)
			beforeResponse, before := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			beforeETag := beforeResponse.Header.Get("ETag")
			for _, test := range cases {
				t.Run(test.name, func(t *testing.T) {
					request := credentialAPIServiceRequest(journey, test.source)
					delete(request, "protected_resources")
					request["display_name"] = "Must never be committed"
					if method == http.MethodPost {
						request["canonical_id"] = "rejected-source"
					}
					if test.omit {
						delete(request, test.field)
					} else {
						request[test.field] = test.value
					}
					schema := "ServiceCreateRequest"
					target := "/api/services"
					if method == http.MethodPut {
						schema = "ServiceUpdateRequest"
						target = path
					}
					require.Error(t, schemas[schema].Validate(credentialSchemaValue(t, request)), "the approved schema must reject this exact request")
					credentialAPIObject(t, journey, method, target, request, nil, http.StatusBadRequest)
					afterResponse, after := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
					assert.Equal(t, before, after, "invalid source input cannot mutate any service metadata or credential")
					assert.Equal(t, beforeETag, afterResponse.Header.Get("ETag"))
					response, listed := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
					require.Equal(t, http.StatusOK, response.StatusCode)
					assert.Equal(t, []any{before}, listed, "rejected registration must not leave a service")
					assert.Empty(t, journey.Upstream.GetTokenRequests())
					assert.Zero(t, journey.Upstream.GetAuthorizationRequestCount())
				})
			}
		})
	}
}

func TestServiceCredentialSourceAPIMissingFilesystemCanonicalIDOnCreate(t *testing.T) {
	journey, _ := newCredentialAPIJourney(t)
	schemas := credentialAPISchemas(t)
	request := credentialAPIServiceRequest(journey, "filesystem")
	delete(request, "canonical_id")
	require.Error(t, schemas["ServiceCreateRequest"].Validate(credentialSchemaValue(t, request)))
	credentialAPIObject(t, journey, http.MethodPost, "/api/services", request, nil, http.StatusBadRequest)
	response, result := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Empty(t, result)
}

func TestServiceCredentialSourceAPIOmittedFilesystemSelectorStillRejectsInlinePresence(t *testing.T) {
	journey, binding := newCredentialAPIJourney(t)
	schemas := credentialAPISchemas(t)
	_, created := credentialAPIObject(t, journey, http.MethodPost, "/api/services", credentialAPIServiceRequest(journey, "filesystem"), nil, http.StatusCreated)
	path := "/api/services/" + created["id"].(string)
	beforeResponse, before := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	for _, field := range []string{"client_id", "client_secret"} {
		for _, value := range []any{nil, "", "inline-synthetic-value"} {
			request := credentialAPIServiceRequest(journey, "filesystem")
			delete(request, "credential_source")
			delete(request, "protected_resources")
			request[field] = value
			credentialAPIObject(t, journey, http.MethodPut, path, request, nil, http.StatusBadRequest)
			afterResponse, after := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
			assert.Equal(t, before, after)
			assert.Equal(t, beforeResponse.Header.Get("ETag"), afterResponse.Header.Get("ETag"))
			assertCredentialAPIService(t, schemas, after, "filesystem", binding)
		}
	}
}

func TestServiceCredentialSourceAPITransitionsPreservePUTAndETagContracts(t *testing.T) {
	journey, binding := newCredentialAPIJourney(t)
	schemas := credentialAPISchemas(t)
	require.NoError(t, os.Remove(binding["client_id_file"]))
	require.NoError(t, os.Remove(binding["client_secret_file"]))
	_, created := credentialAPIObject(t, journey, http.MethodPost, "/api/services", credentialAPIServiceRequest(journey, "stored"), nil, http.StatusCreated)
	path := "/api/services/" + created["id"].(string)
	beforeResponse, before := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	originalETag := beforeResponse.Header.Get("ETag")

	filesystem := credentialAPIServiceRequest(journey, "filesystem")
	filesystem["protected_resources"] = []string{journey.Upstream.URL() + "/replacement-resource"}
	require.NoError(t, schemas["ServiceUpdateRequest"].Validate(credentialSchemaValue(t, filesystem)))
	credentialAPIObject(t, journey, http.MethodPut, path, filesystem, nil, http.StatusPreconditionRequired)
	credentialAPIObject(t, journey, http.MethodPut, path, filesystem, map[string]string{"If-Match": `"999999"`}, http.StatusPreconditionFailed)
	afterResponse, unchanged := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	assert.Equal(t, before, unchanged)
	assert.Equal(t, originalETag, afterResponse.Header.Get("ETag"))
	_, transitioned := credentialAPIObject(t, journey, http.MethodPut, path, filesystem, map[string]string{"If-Match": originalETag}, http.StatusOK)
	assertCredentialAPIService(t, schemas, transitioned, "filesystem", binding)
	assert.Equal(t, before["created_at"], transitioned["created_at"])
	assert.Equal(t, []any{journey.Upstream.URL() + "/replacement-resource"}, transitioned["protected_resources"])
	response, transitionedRead := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	assert.Equal(t, transitioned, transitionedRead)
	assert.NotEqual(t, originalETag, response.Header.Get("ETag"))

	metadata := credentialAPIServiceRequest(journey, "filesystem")
	delete(metadata, "credential_source")
	delete(metadata, "canonical_id")
	delete(metadata, "protected_resources")
	metadata["display_name"] = "Filesystem metadata replacement"
	_, preserved := credentialAPIObject(t, journey, http.MethodPut, path, metadata, nil, http.StatusOK)
	assertCredentialAPIService(t, schemas, preserved, "filesystem", binding)
	assert.Equal(t, transitioned["canonical_id"], preserved["canonical_id"])
	assert.Equal(t, transitioned["protected_resources"], preserved["protected_resources"])

	stored := credentialAPIServiceRequest(journey, "stored")
	delete(stored, "protected_resources")
	const reverseClientID = "explicit-reverse-client"
	const reverseSecret = "explicit-reverse-secret"
	stored["client_id"] = reverseClientID
	stored["client_secret"] = reverseSecret
	for _, field := range []string{"client_id", "client_secret"} {
		invalid := credentialAPIServiceRequest(journey, "stored")
		delete(invalid, "protected_resources")
		delete(invalid, field)
		credentialAPIObject(t, journey, http.MethodPut, path, invalid, nil, http.StatusBadRequest)
		_, current := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
		assert.Equal(t, preserved, current)
	}
	_, reversed := credentialAPIObject(t, journey, http.MethodPut, path, stored, nil, http.StatusOK)
	assertCredentialAPIService(t, schemas, reversed, "stored", binding)
	assert.Equal(t, reverseClientID, reversed["client_id"], "reverse transitions use supplied credentials, never current file contents")
	assert.Equal(t, transitioned["protected_resources"], reversed["protected_resources"])
	assert.Equal(t, before["created_at"], reversed["created_at"])
	filesystem["display_name"] = "Stale source and resources must not commit"
	credentialAPIObject(t, journey, http.MethodPut, path, filesystem, map[string]string{"If-Match": originalETag}, http.StatusPreconditionFailed)
	_, afterConflict := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	assert.Equal(t, reversed, afterConflict)
	assert.Empty(t, journey.Upstream.GetTokenRequests())
	// Neither transition required available files. After reversal the real
	// provider protocol must receive the explicitly supplied pair, not a file value.
	response, err := journey.EndUser.AuthenticatedGET("/api/third-party/"+created["id"].(string)+"/oauth2/authorize?redirect_uri="+url.QueryEscape(journey.Config.Server.EndUser.PublicURL+"/done"), journey.Principal)
	require.NoError(t, err)
	authorization, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	assert.Equal(t, reverseClientID, authorization.Query().Get("client_id"))
	callback, err := helpers.ProviderAuthorizationCallback(authorization.String())
	require.NoError(t, err)
	response, err = journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
	require.NoError(t, err)
	completed, err := url.Parse(response.Header.Get("Location"))
	require.NoError(t, err)
	require.NoError(t, response.Body.Close())
	require.Equal(t, http.StatusFound, response.StatusCode)
	require.Equal(t, "true", completed.Query().Get("success"))
	requests := journey.Upstream.GetTokenRequests()
	require.NotEmpty(t, requests)
	for _, request := range requests {
		clientID, clientSecret, err := helpers.CredentialTokenRequest(request)
		require.NoError(t, err)
		assert.Equal(t, reverseClientID, clientID)
		assert.Equal(t, reverseSecret, clientSecret)
	}
	_, afterAuthentication := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	assert.Equal(t, reversed, afterAuthentication)
}

func TestServiceCredentialSourceAPISelectorOmissionDoesNotTurnPUTIntoPATCH(t *testing.T) {
	journey, binding := newCredentialAPIJourney(t)
	schemas := credentialAPISchemas(t)
	public := credentialAPIServiceRequest(journey, "stored")
	public["token_endpoint_auth_method"] = "none"
	delete(public, "client_secret")
	public["authorization_params"] = map[string]string{"audience": "synthetic-audience"}
	_, created := credentialAPIObject(t, journey, http.MethodPost, "/api/services", public, nil, http.StatusCreated)
	path := "/api/services/" + created["id"].(string)
	invalid := map[string]any{"display_name": "Not a PATCH"}
	credentialAPIObject(t, journey, http.MethodPut, path, invalid, nil, http.StatusBadRequest)
	_, unchanged := credentialAPIObject(t, journey, http.MethodGet, path, nil, nil, http.StatusOK)
	assert.Equal(t, created, unchanged)
	confidential := credentialAPIServiceRequest(journey, "stored")
	delete(confidential, "credential_source")
	delete(confidential, "protected_resources")
	confidential["scopes"] = []any{}
	_, updated := credentialAPIObject(t, journey, http.MethodPut, path, confidential, nil, http.StatusOK)
	assertCredentialAPIService(t, schemas, updated, "stored", binding)
	assert.Equal(t, fixtures.CredentialClientID, updated["client_id"])
	assert.Nil(t, updated["token_endpoint_auth_method"], "omitted authentication method means shared-secret, not preserve-public")
	assert.Empty(t, updated["scopes"], "ordinary fields retain full-replacement semantics")
	assert.Equal(t, created["authorization_params"], updated["authorization_params"], "existing documented omission exceptions remain intact")
}

func TestServiceCredentialSourceAPIRejectsNonObjectJSON(t *testing.T) {
	journey, _ := newCredentialAPIJourney(t)
	for _, body := range []string{`null`, `true`, `42`, `"filesystem"`, `[]`} {
		response, err := journey.Admin.DirectRequest(http.MethodPost, "/api/services", journey.Principal, map[string]string{"Content-Type": "application/json"}, strings.NewReader(body))
		require.NoError(t, err)
		encoded, err := io.ReadAll(response.Body)
		require.NoError(t, response.Body.Close())
		require.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, response.StatusCode, "body %s; response %s", body, encoded)
	}
	response, listed := credentialAPIRequest(t, journey, http.MethodGet, "/api/services", nil, nil)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Empty(t, listed)
}
