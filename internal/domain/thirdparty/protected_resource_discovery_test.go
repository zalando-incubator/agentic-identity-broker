package thirdparty

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	discoveryResource       = "https://mcp.example.test/mcp"
	discoveryIssuer         = "https://auth.example.test/tenant"
	discoveryResourcePath   = "https://mcp.example.test/.well-known/oauth-protected-resource/mcp"
	discoveryResourceRoot   = "https://mcp.example.test/.well-known/oauth-protected-resource"
	discoveryIssuerOAuth    = "https://auth.example.test/.well-known/oauth-authorization-server/tenant"
	discoveryIssuerOpenID   = "https://auth.example.test/.well-known/openid-configuration/tenant"
	discoveryIssuerAppended = "https://auth.example.test/tenant/.well-known/openid-configuration"
)

type discoveryProbeReply struct {
	status     int
	challenges []string
	err        error
}

type discoveryJSONReply struct {
	body []byte
	err  error
}

// The port double records network intent independently of the service's storage calls.
// An unexpected URL is an error, not a default document or an implicit 404.
type recordingOAuthDiscoveryClient struct {
	probes map[string]discoveryProbeReply
	gets   map[string]discoveryJSONReply
	calls  []string
	postFn func(context.Context, string, []byte) ([]byte, error)
}

func (c *recordingOAuthDiscoveryClient) Probe(_ context.Context, rawURL string) (int, []string, error) {
	c.calls = append(c.calls, "probe "+rawURL)
	reply, ok := c.probes[rawURL]
	if !ok {
		return 0, nil, fmt.Errorf("unexpected resource probe: %s", rawURL)
	}
	return reply.status, reply.challenges, reply.err
}

func (c *recordingOAuthDiscoveryClient) GetJSON(_ context.Context, rawURL string) ([]byte, error) {
	c.calls = append(c.calls, "get "+rawURL)
	reply, ok := c.gets[rawURL]
	if !ok {
		return nil, fmt.Errorf("unexpected metadata read: %s", rawURL)
	}
	return reply.body, reply.err
}

func (c *recordingOAuthDiscoveryClient) PostJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error) {
	c.calls = append(c.calls, "post "+rawURL)
	if c.postFn == nil {
		return nil, fmt.Errorf("unexpected DCR registration: %s", rawURL)
	}
	return c.postFn(ctx, rawURL, body)
}

var _ ports.OAuthDiscoveryClient = (*recordingOAuthDiscoveryClient)(nil)

func discoveredProvider(resource, selectedIssuer string) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:          id.NewServiceID(),
		DisplayName: "Protected MCP",
		Secret:      model.NewAbsentSecret(),
		IssuerURI:   selectedIssuer,
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: true,
			ResourceURL:     &resource,
		},
	}
}

func protectedResourceDocument(resource string, issuers ...string) []byte {
	result, err := json.Marshal(struct {
		Resource             string   `json:"resource"`
		AuthorizationServers []string `json:"authorization_servers"`
	}{resource, issuers})
	if err != nil {
		panic(err)
	}
	return result
}

func issuerCIMDDocument(issuer, authorize string) []byte {
	return []byte(fmt.Sprintf(`{"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"client_id_metadata_document_supported":true,"token_endpoint_auth_methods_supported":["private_key_jwt","client_secret_basic"],"token_endpoint_auth_signing_alg_values_supported":["ES256"],"registration_endpoint":%q}`,
		issuer, authorize, issuer+"/token", issuer+"/register"))
}

func runDiscoveredCreate(client *recordingOAuthDiscoveryClient, entity *model.ThirdpartyOAuth2ProviderEntity) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
		stored = provider.Copy()
		return nil
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, &functionFieldEncryption{}, newNoopBranchKeyManager(), nil, false, slog.Default()).
		WithCIMDPublicURL("https://broker.example.test").
		WithCIMDKeyReadiness(readyCIMDKeyReadiness{}).
		WithOAuthDiscoveryClient(client)
	err := service.Create(context.Background(), entity)
	return stored, err
}

func TestProtectedResourceDiscovery_401ChallengeTakesPriority(t *testing.T) {
	for _, scheme := range []string{"Bearer", "DPoP"} {
		t.Run(scheme, func(t *testing.T) {
			challengeURL := "https://metadata.example.test/resource/metadata"
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {
					status: 401, challenges: []string{scheme + ` realm="mcp", resource_metadata="` + challengeURL + `"`},
				}},
				gets: map[string]discoveryJSONReply{
					challengeURL:         {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
					discoveryIssuerOAuth: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
				},
			}
			entity := discoveredProvider(discoveryResource, "")
			_, err := runDiscoveredCreate(client, entity)
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + challengeURL, "get " + discoveryIssuerOAuth}, client.calls)
			require.NoError(t, err)
			assert.Equal(t, discoveryIssuer, entity.IssuerURI)
		})
	}
}

func TestProtectedResourceDiscovery_InvalidChallengeNeverFallsBack(t *testing.T) {
	challengeURL := "https://metadata.example.test/resource/metadata"
	for _, tc := range []struct {
		name       string
		status     int
		challenges []string
	}{
		{"duplicate in one challenge", 401, []string{`Bearer resource_metadata="` + challengeURL + `", resource_metadata="` + challengeURL + `"`}},
		{"duplicate in separate challenges", 401, []string{`Bearer resource_metadata="` + challengeURL + `"`, `DPoP resource_metadata="` + challengeURL + `"`}},
		{"malformed value", 401, []string{`Bearer resource_metadata="not a URL"`}},
		{"unsupported scheme", 401, []string{`Basic resource_metadata="` + challengeURL + `"`}},
		{"challenge on non-401 response", 403, []string{`Bearer resource_metadata="` + challengeURL + `"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {status: tc.status, challenges: tc.challenges}},
				gets: map[string]discoveryJSONReply{
					challengeURL:          {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
					discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
				},
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
			assert.Equal(t, []string{"probe " + discoveryResource}, client.calls, "invalid challenges must not trigger metadata fetch or well-known fallback")
			require.ErrorContains(t, err, "resource_metadata_invalid")
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_WellKnownLocationsOnlyFallBackOn404(t *testing.T) {
	for _, tc := range []struct {
		name       string
		resource   string
		pathResult discoveryJSONReply
		rootResult discoveryJSONReply
		calls      []string
		wantError  string
	}{
		{
			name: "path-specific wins over root", resource: discoveryResource,
			pathResult: discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			calls:      []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth},
		},
		{
			name: "path 404 falls back to root", resource: discoveryResource,
			pathResult: discoveryJSONReply{err: ports.ErrOAuthDiscoveryNotFound},
			rootResult: discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			calls:      []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryResourceRoot, "get " + discoveryIssuerOAuth},
		},
		{
			name: "path failure does not fall back", resource: discoveryResource,
			pathResult: discoveryJSONReply{err: ports.ErrOAuthDiscoveryUnavailable},
			rootResult: discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			calls:      []string{"probe " + discoveryResource, "get " + discoveryResourcePath},
			wantError:  "resource_metadata_unavailable",
		},
		{
			name: "invalid path metadata does not fall back", resource: discoveryResource,
			pathResult: discoveryJSONReply{body: []byte(`{"resource":`)},
			rootResult: discoveryJSONReply{body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			calls:      []string{"probe " + discoveryResource, "get " + discoveryResourcePath},
			wantError:  "resource_metadata_invalid",
		},
		{
			name: "all resource locations missing", resource: discoveryResource,
			pathResult: discoveryJSONReply{err: ports.ErrOAuthDiscoveryNotFound},
			rootResult: discoveryJSONReply{err: ports.ErrOAuthDiscoveryNotFound},
			calls:      []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryResourceRoot},
			wantError:  "resource_metadata_not_found",
		},
		{
			name: "root resource probes root once", resource: "https://mcp.example.test",
			rootResult: discoveryJSONReply{body: protectedResourceDocument("https://mcp.example.test", discoveryIssuer)},
			calls:      []string{"probe https://mcp.example.test", "get " + discoveryResourceRoot, "get " + discoveryIssuerOAuth},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{tc.resource: {status: 401, challenges: []string{`Bearer realm="mcp"`}}},
				gets: map[string]discoveryJSONReply{
					discoveryResourcePath: tc.pathResult,
					discoveryResourceRoot: tc.rootResult,
					discoveryIssuerOAuth:  {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
				},
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(tc.resource, ""))
			assert.Equal(t, tc.calls, client.calls)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, stored)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, tc.resource, stored.AuthorizationParams["resource"])
		})
	}
}

func TestProtectedResourceDiscovery_ChallengeMetadata404DoesNotTryWellKnown(t *testing.T) {
	challengeURL := "https://metadata.example.test/missing"
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 401, challenges: []string{`Bearer resource_metadata="` + challengeURL + `"`}}},
		gets: map[string]discoveryJSONReply{
			challengeURL:          {err: ports.ErrOAuthDiscoveryNotFound},
			discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
		},
	}
	stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + challengeURL}, client.calls)
	require.ErrorContains(t, err, "resource_metadata_not_found")
	assert.Nil(t, stored)
}

func TestProtectedResourceDiscovery_ResourceIdentityMustMatchExactly(t *testing.T) {
	for _, mismatch := range []string{"https://mcp.example.test", "https://mcp.example.test/mcp/"} {
		t.Run(mismatch, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
				gets: map[string]discoveryJSONReply{
					discoveryResourcePath: {body: protectedResourceDocument(mismatch, discoveryIssuer)},
					discoveryIssuerOAuth:  {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
				},
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath}, client.calls)
			require.ErrorContains(t, err, "resource_mismatch")
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_MultipleIssuersRequireValidatedSelection(t *testing.T) {
	const otherIssuer = "https://other-auth.example.test"
	for _, tc := range []struct {
		name       string
		issuer     string
		advertised []string
		wantError  string
		wantCalls  []string
	}{
		{"no selection", "", []string{discoveryIssuer, otherIssuer}, "issuer_selection_required", []string{"probe " + discoveryResource, "get " + discoveryResourcePath}},
		{"unadvertised selection", "https://attacker.example.test", []string{discoveryIssuer, otherIssuer}, "issuer_not_advertised", []string{"probe " + discoveryResource, "get " + discoveryResourcePath}},
		{"duplicate issuers", "", []string{discoveryIssuer, discoveryIssuer}, "resource_metadata_invalid", []string{"probe " + discoveryResource, "get " + discoveryResourcePath}},
		{"unsafe issuer", "", []string{discoveryIssuer, "http://insecure.example.test"}, "unsafe_destination", []string{"probe " + discoveryResource, "get " + discoveryResourcePath}},
		{"selected issuer", otherIssuer, []string{discoveryIssuer, otherIssuer}, "", []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get https://other-auth.example.test/.well-known/oauth-authorization-server"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
				gets: map[string]discoveryJSONReply{
					discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, tc.advertised...)},
					"https://other-auth.example.test/.well-known/oauth-authorization-server": {body: issuerCIMDDocument(otherIssuer, otherIssuer+"/authorize")},
				},
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, tc.issuer))
			assert.Equal(t, tc.wantCalls, client.calls, "only a verified selected issuer may receive a metadata request")
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, stored)
				if tc.wantError == "issuer_selection_required" {
					var discoveryError *DiscoveryError
					require.ErrorAs(t, err, &discoveryError)
					assert.Equal(t, "issuer_selection_required", discoveryError.Code)
					assert.Equal(t, tc.advertised, discoveryError.AuthorizationServers)
					assert.Equal(t, "issuer_selection_required", discoveryError.Error(), "provider URLs must not leak into the safe error message")
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, otherIssuer, stored.IssuerURI)
			assert.Equal(t, otherIssuer+"/authorize", stored.Endpoints.AuthorizeEndpoint)
		})
	}
}

func TestProtectedResourceDiscovery_IssuerMetadata404OnlyProbeOrder(t *testing.T) {
	const rootIssuer = "https://auth.example.test"
	for _, tc := range []struct {
		name      string
		issuer    string
		results   map[string]discoveryJSONReply
		wantCalls []string
		wantAuth  string
		wantError string
	}{
		{
			name: "path OAuth wins", issuer: discoveryIssuer,
			results:   map[string]discoveryJSONReply{discoveryIssuerOAuth: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/oauth-authorize")}},
			wantCalls: []string{"get " + discoveryIssuerOAuth}, wantAuth: discoveryIssuer + "/oauth-authorize",
		},
		{
			name: "path OpenID inserted wins", issuer: discoveryIssuer,
			results: map[string]discoveryJSONReply{
				discoveryIssuerOAuth:  {err: ports.ErrOAuthDiscoveryNotFound},
				discoveryIssuerOpenID: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/inserted-authorize")},
			},
			wantCalls: []string{"get " + discoveryIssuerOAuth, "get " + discoveryIssuerOpenID}, wantAuth: discoveryIssuer + "/inserted-authorize",
		},
		{
			name: "path OpenID appended last", issuer: discoveryIssuer,
			results: map[string]discoveryJSONReply{
				discoveryIssuerOAuth:    {err: ports.ErrOAuthDiscoveryNotFound},
				discoveryIssuerOpenID:   {err: ports.ErrOAuthDiscoveryNotFound},
				discoveryIssuerAppended: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/appended-authorize")},
			},
			wantCalls: []string{"get " + discoveryIssuerOAuth, "get " + discoveryIssuerOpenID, "get " + discoveryIssuerAppended}, wantAuth: discoveryIssuer + "/appended-authorize",
		},
		{
			name: "root OAuth then OpenID", issuer: rootIssuer,
			results: map[string]discoveryJSONReply{
				"https://auth.example.test/.well-known/oauth-authorization-server": {err: ports.ErrOAuthDiscoveryNotFound},
				"https://auth.example.test/.well-known/openid-configuration":       {body: issuerCIMDDocument(rootIssuer, rootIssuer+"/openid-authorize")},
			},
			wantCalls: []string{"get https://auth.example.test/.well-known/oauth-authorization-server", "get https://auth.example.test/.well-known/openid-configuration"}, wantAuth: rootIssuer + "/openid-authorize",
		},
		{
			name: "all issuer locations missing", issuer: discoveryIssuer,
			results: map[string]discoveryJSONReply{
				discoveryIssuerOAuth:    {err: ports.ErrOAuthDiscoveryNotFound},
				discoveryIssuerOpenID:   {err: ports.ErrOAuthDiscoveryNotFound},
				discoveryIssuerAppended: {err: ports.ErrOAuthDiscoveryNotFound},
			},
			wantCalls: []string{"get " + discoveryIssuerOAuth, "get " + discoveryIssuerOpenID, "get " + discoveryIssuerAppended},
			wantError: "authorization_server_metadata_not_found",
		},
		{
			name: "issuer read error stops", issuer: discoveryIssuer,
			results: map[string]discoveryJSONReply{
				discoveryIssuerOAuth:  {err: ports.ErrOAuthDiscoveryUnavailable},
				discoveryIssuerOpenID: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
			},
			wantCalls: []string{"get " + discoveryIssuerOAuth}, wantError: "authorization_server_metadata_unavailable",
		},
		{
			name: "wrong issuer document stops", issuer: discoveryIssuer,
			results: map[string]discoveryJSONReply{
				discoveryIssuerOAuth:  {body: issuerCIMDDocument(rootIssuer, rootIssuer+"/authorize")},
				discoveryIssuerOpenID: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
			},
			wantCalls: []string{"get " + discoveryIssuerOAuth}, wantError: "issuer_mismatch",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
				gets: map[string]discoveryJSONReply{
					discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, tc.issuer)},
				},
			}
			for url, result := range tc.results {
				client.gets[url] = result
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
			wantCalls := append([]string{"probe " + discoveryResource, "get " + discoveryResourcePath}, tc.wantCalls...)
			assert.Equal(t, wantCalls, client.calls)
			if tc.wantError != "" {
				require.ErrorContains(t, err, tc.wantError)
				assert.Nil(t, stored)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, tc.wantAuth, stored.Endpoints.AuthorizeEndpoint)
		})
	}
}

func TestProtectedResourceDiscovery_ConflictingIdentityClaimsStopAtFirstMetadataDocument(t *testing.T) {
	for _, tc := range []struct {
		name         string
		resourceBody []byte
		issuerBody   []byte
		wantCalls    []string
		wantCode     string
	}{
		{
			name:         "conflicting protected resource claims",
			resourceBody: []byte(fmt.Sprintf(`{"resource":%q,"resource":%q,"authorization_servers":[%q]}`, "https://mcp.example.test/other", discoveryResource, discoveryIssuer)),
			issuerBody:   issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize"),
			wantCalls:    []string{"probe " + discoveryResource, "get " + discoveryResourcePath},
			wantCode:     "resource_metadata_invalid",
		},
		{
			name:         "conflicting authorization server claims",
			resourceBody: protectedResourceDocument(discoveryResource, discoveryIssuer),
			issuerBody: []byte(fmt.Sprintf(`{"issuer":%q,"issuer":%q,"authorization_endpoint":%q,"token_endpoint":%q,"client_id_metadata_document_supported":true,"token_endpoint_auth_methods_supported":["private_key_jwt"],"token_endpoint_auth_signing_alg_values_supported":["ES256"]}`,
				"https://auth.example.test/other", discoveryIssuer, discoveryIssuer+"/authorize", discoveryIssuer+"/token")),
			wantCalls: []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth},
			wantCode:  "authorization_server_metadata_invalid",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &recordingOAuthDiscoveryClient{
				probes: map[string]discoveryProbeReply{discoveryResource: {status: 401, challenges: []string{`Bearer realm="mcp"`}}},
				gets: map[string]discoveryJSONReply{
					discoveryResourcePath: {body: tc.resourceBody},
					discoveryResourceRoot: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
					discoveryIssuerOAuth:  {body: tc.issuerBody},
					discoveryIssuerOpenID: {body: issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize")},
				},
			}
			stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
			assert.Equal(t, tc.wantCalls, client.calls, "a non-404 identity claim must not fall back to another metadata location or registration")
			var discoveryErr *DiscoveryError
			require.ErrorAs(t, err, &discoveryErr)
			assert.Equal(t, tc.wantCode, discoveryErr.Code)
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_RejectsMissingIssuerBeforeIssuerProbe(t *testing.T) {
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
		gets:   map[string]discoveryJSONReply{discoveryResourcePath: {body: protectedResourceDocument(discoveryResource)}},
	}
	stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath}, client.calls)
	require.ErrorContains(t, err, "authorization_server_missing")
	assert.Nil(t, stored)
}

func TestProtectedResourceDiscovery_ProbeFailureDoesNotReadMetadata(t *testing.T) {
	client := &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {err: ports.ErrOAuthDiscoveryUnavailable}},
	}
	stored, err := runDiscoveredCreate(client, discoveredProvider(discoveryResource, ""))
	assert.Equal(t, []string{"probe " + discoveryResource}, client.calls)
	require.ErrorContains(t, err, "resource_metadata_unavailable")
	assert.Nil(t, stored)
}

const dcrBrokerOrigin = "https://broker.example.test"

func dcrCallback(serviceID id.ServiceID) string {
	return dcrBrokerOrigin + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
}

func issuerDCRDocument(methods, pkce []string) []byte {
	metadata := map[string]any{
		"issuer":                 discoveryIssuer,
		"authorization_endpoint": discoveryIssuer + "/authorize",
		"token_endpoint":         discoveryIssuer + "/token",
		"registration_endpoint":  discoveryIssuer + "/register",
	}
	if methods != nil {
		metadata["token_endpoint_auth_methods_supported"] = methods
	}
	if pkce != nil {
		metadata["code_challenge_methods_supported"] = pkce
	}
	body, err := json.Marshal(metadata)
	if err != nil {
		panic(err)
	}
	return body
}

func dcrDiscoveryClient(metadata []byte) *recordingOAuthDiscoveryClient {
	return &recordingOAuthDiscoveryClient{
		probes: map[string]discoveryProbeReply{discoveryResource: {status: 200}},
		gets: map[string]discoveryJSONReply{
			discoveryResourcePath: {body: protectedResourceDocument(discoveryResource, discoveryIssuer)},
			discoveryIssuerOAuth:  {body: metadata},
		},
	}
}

func dcrResponse(callback string, method model.TokenEndpointAuthMethod, secret string) map[string]any {
	response := map[string]any{
		"client_id":                  "registered-client-42",
		"token_endpoint_auth_method": method,
		"redirect_uris":              []string{callback},
		"grant_types":                []string{"authorization_code", "refresh_token"},
	}
	if secret != "" {
		response["client_secret"] = secret
	}
	return response
}

func dcrJSON(value map[string]any) []byte {
	body, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return body
}

func runDCRCreate(client *recordingOAuthDiscoveryClient, entity *model.ThirdpartyOAuth2ProviderEntity, name string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	var stored *model.ThirdpartyOAuth2ProviderEntity
	repo := &functionFieldProviderRepository{createFn: func(_ context.Context, provider *model.ThirdpartyOAuth2ProviderEntity) error {
		stored = provider.Copy()
		return nil
	}}
	encryption := &functionFieldEncryption{encryptFn: func(_ context.Context, plaintext []byte, _ map[string]string) ([]byte, error) {
		return append([]byte("sealed:"), plaintext...), nil
	}}
	service := NewThirdpartyOAuth2ProviderService(repo, encryption, newNoopBranchKeyManager(), nil, false, slog.Default()).
		WithCIMDPublicURL(dcrBrokerOrigin).
		WithCIMDKeyReadiness(readyCIMDKeyReadiness{}).
		WithOAuthDiscoveryClient(client).
		WithDCRClientName(name)
	err := service.Create(context.Background(), entity)
	return stored, err
}

func TestProtectedResourceDiscovery_DCRSelectsPreferredMethodAndRegistersExactBrokerRequest(t *testing.T) {
	for _, tc := range []struct {
		name    string
		methods []string
		method  model.TokenEndpointAuthMethod
	}{
		{"Basic before POST and public", []string{"none", "client_secret_post", "client_secret_basic"}, model.TokenEndpointAuthMethodClientSecretBasic},
		{"POST before public", []string{"none", "client_secret_post"}, model.TokenEndpointAuthMethodClientSecretPost},
		{"RFC 8414 omitted methods default to Basic", nil, model.TokenEndpointAuthMethodClientSecretBasic},
		{"public with S256", []string{"none"}, model.TokenEndpointAuthMethodNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entity := discoveredProvider(discoveryResource, "")
			entity.DisplayName = "Files MCP"
			callback := dcrCallback(entity.ID)
			client := dcrDiscoveryClient(issuerDCRDocument(tc.methods, []string{"S256"}))
			client.postFn = func(_ context.Context, rawURL string, body []byte) ([]byte, error) {
				require.Equal(t, discoveryIssuer+"/register", rawURL)
				want := map[string]any{
					"redirect_uris":              []string{callback},
					"client_name":                "Example Platform",
					"application_type":           "web",
					"grant_types":                []string{"authorization_code", "refresh_token"},
					"response_types":             []string{"code"},
					"token_endpoint_auth_method": tc.method,
				}
				require.JSONEq(t, string(dcrJSON(want)), string(body), "only the selected method and broker identity may be registered")
				secret := "provider-issued-secret"
				if tc.method == model.TokenEndpointAuthMethodNone {
					secret = ""
				}
				response := dcrResponse(callback, tc.method, secret)
				if tc.name == "Basic before POST and public" {
					response["client_secret_expires_at"] = 0
				}
				return dcrJSON(response), nil
			}

			stored, err := runDCRCreate(client, entity, "Example Platform")
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register"}, client.calls)
			require.NoError(t, err)
			require.NotNil(t, stored)
			assert.Equal(t, "Files MCP", stored.DisplayName)
			assert.Equal(t, model.ClientBootstrapDCR, stored.Discovery.ClientMethod)
			assert.Equal(t, tc.method, stored.TokenEndpointAuthMethod)
			assert.Equal(t, discoveryIssuer, stored.IssuerURI)
			assert.Equal(t, id.ClientID("registered-client-42"), stored.ClientID)
			assert.Equal(t, discoveryResource, stored.AuthorizationParams["resource"])
			if tc.method == model.TokenEndpointAuthMethodNone {
				assert.True(t, stored.Secret.IsAbsent())
			} else {
				assert.True(t, stored.Secret.IsEncrypted())
			}
		})
	}
}

func TestProtectedResourceDiscovery_PublicDCRRequiresAdvertisedS256(t *testing.T) {
	for _, tc := range []struct {
		name string
		pkce []string
	}{
		{"no PKCE metadata", nil},
		{"only plain PKCE", []string{"plain"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := dcrDiscoveryClient(issuerDCRDocument([]string{"none"}, tc.pkce))
			stored, err := runDCRCreate(client, discoveredProvider(discoveryResource, ""), "Example Platform")
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls)
			require.ErrorContains(t, err, "no_compatible_client_method")
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_DCRResponseAllowsOmittedRefreshGrantAndSecretExpiry(t *testing.T) {
	entity := discoveredProvider(discoveryResource, "")
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
	client.postFn = func(_ context.Context, _ string, _ []byte) ([]byte, error) {
		response := dcrResponse(dcrCallback(entity.ID), model.TokenEndpointAuthMethodClientSecretBasic, "long-lived-secret")
		response["grant_types"] = []string{"authorization_code"}
		delete(response, "client_secret_expires_at")
		return dcrJSON(response), nil
	}
	stored, err := runDCRCreate(client, entity, "Example Platform")
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register"}, client.calls)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.ClientBootstrapDCR, stored.Discovery.ClientMethod)
	assert.Equal(t, model.TokenEndpointAuthMethodClientSecretBasic, stored.TokenEndpointAuthMethod)
}

func TestProtectedResourceDiscovery_DCRRejectsIncompatibleResponseWithoutRetryOrStorage(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(map[string]any)
	}{
		{"empty client ID", func(response map[string]any) { response["client_id"] = "" }},
		{"wrong callback", func(response map[string]any) {
			response["redirect_uris"] = []string{"https://attacker.example.test/callback"}
		}},
		{"wrong authentication method", func(response map[string]any) { response["token_endpoint_auth_method"] = "none" }},
		{"missing confidential secret", func(response map[string]any) { delete(response, "client_secret") }},
		{"expired secret", func(response map[string]any) { response["client_secret_expires_at"] = int64(1) }},
		{"future-expiring secret", func(response map[string]any) { response["client_secret_expires_at"] = int64(4102444800) }},
		{"missing authorization-code grant", func(response map[string]any) { response["grant_types"] = []string{"refresh_token"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entity := discoveredProvider(discoveryResource, "")
			client := dcrDiscoveryClient(issuerDCRDocument([]string{"none", "client_secret_basic"}, []string{"S256"}))
			client.postFn = func(_ context.Context, _ string, _ []byte) ([]byte, error) {
				response := dcrResponse(dcrCallback(entity.ID), model.TokenEndpointAuthMethodClientSecretBasic, "provider-issued-secret")
				tc.change(response)
				return dcrJSON(response), nil
			}
			stored, err := runDCRCreate(client, entity, "Example Platform")
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register"}, client.calls, "rejected confidential registration must not retry public DCR")
			require.ErrorContains(t, err, "client_registration_invalid")
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_RejectedConfidentialDCRDoesNotTryPublic(t *testing.T) {
	client := dcrDiscoveryClient(issuerDCRDocument([]string{"none", "client_secret_basic"}, []string{"S256"}))
	client.postFn = func(_ context.Context, _ string, _ []byte) ([]byte, error) {
		return nil, ports.ErrOAuthDiscoveryRejected
	}
	stored, err := runDCRCreate(client, discoveredProvider(discoveryResource, ""), "Example Platform")
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth, "post " + discoveryIssuer + "/register"}, client.calls)
	require.ErrorContains(t, err, "client_registration_rejected")
	assert.Nil(t, stored)
}

func TestProtectedResourceDiscovery_DCRRequiresGlobalNameBeforeRegistration(t *testing.T) {
	for _, name := range []string{"", "  \t  "} {
		t.Run(fmt.Sprintf("name %q", name), func(t *testing.T) {
			client := dcrDiscoveryClient(issuerDCRDocument([]string{"client_secret_basic"}, nil))
			stored, err := runDCRCreate(client, discoveredProvider(discoveryResource, ""), name)
			assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls)
			require.ErrorContains(t, err, "client_name_unconfigured")
			assert.Nil(t, stored)
		})
	}
}

func TestProtectedResourceDiscovery_CIMDWithoutGlobalDCRNameNeverRegisters(t *testing.T) {
	client := dcrDiscoveryClient(issuerCIMDDocument(discoveryIssuer, discoveryIssuer+"/authorize"))
	stored, err := runDCRCreate(client, discoveredProvider(discoveryResource, ""), "")
	assert.Equal(t, []string{"probe " + discoveryResource, "get " + discoveryResourcePath, "get " + discoveryIssuerOAuth}, client.calls)
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.Equal(t, model.ClientBootstrapCIMD, stored.Discovery.ClientMethod)
}
