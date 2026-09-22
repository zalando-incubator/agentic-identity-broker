package enduser

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	cimdMetadataNotFoundError   = "not found"
	cimdMetadataUnavailableText = "CIMD service unavailable"
)

type cimdMetadataProviderFake struct {
	metadata    *ports.CIMDClientMetadata
	metadataErr error
	jwkSet      jwk.Set
	jwkSetErr   error

	metadataServiceID id.ServiceID
	jwkSetServiceID   id.ServiceID
}

func (f *cimdMetadataProviderFake) Metadata(_ context.Context, serviceID id.ServiceID) (*ports.CIMDClientMetadata, error) {
	f.metadataServiceID = serviceID
	return f.metadata, f.metadataErr
}

func (f *cimdMetadataProviderFake) JWKSet(_ context.Context, serviceID id.ServiceID) (jwk.Set, error) {
	f.jwkSetServiceID = serviceID
	return f.jwkSet, f.jwkSetErr
}

func newCIMDMetadataHandlerForTest(provider ports.CIMDClientMetadataProvider) *CIMDMetadataHandler {
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewCIMDMetadataHandler(provider, logger)
}

func newCIMDMetadataRequest(method, target string, serviceID id.ServiceID) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	routeContext := chi.NewRouteContext()
	routeContext.URLParams.Add("service-id", serviceID.String())
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, routeContext))
}

func newPublicCIMDJWKSet(t *testing.T) jwk.Set {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	publicKey, err := jwk.Import[jwk.Key](privateKey.Public())
	require.NoError(t, err)
	require.NoError(t, publicKey.Set(jwk.KeyIDKey, "cimd-client-key"))
	require.NoError(t, publicKey.Set(jwk.KeyUsageKey, "sig"))
	require.NoError(t, publicKey.Set(jwk.AlgorithmKey, jwa.ES256()))

	set := jwk.NewSet()
	require.NoError(t, set.AddKey(publicKey))
	return set
}

func assertNoCIMDMetadataRedirect(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()

	assert.False(t, response.Code >= http.StatusMultipleChoices && response.Code < http.StatusBadRequest)
	assert.Empty(t, response.Header().Get("Location"))
}

func TestCIMDMetadataHandler_Metadata(t *testing.T) {
	t.Run("serves the exact anonymous metadata document as JSON", func(t *testing.T) {
		serviceID := id.NewServiceID()
		clientID := "https://broker.example.com/.well-known/oauth-client/" + serviceID.String()
		provider := &cimdMetadataProviderFake{
			metadata: &ports.CIMDClientMetadata{
				ClientID:                     clientID,
				RedirectURIs:                 []string{"https://broker.example.com/api/third-party/" + serviceID.String() + "/oauth2/callback"},
				GrantTypes:                   []string{"authorization_code", "refresh_token"},
				ResponseTypes:                []string{"code"},
				TokenEndpointAuthMethod:      "private_key_jwt",
				TokenEndpointAuthSigningAlgo: "ES256",
				JWKSURI:                      clientID + "/jwks.json",
			},
		}
		handler := newCIMDMetadataHandlerForTest(provider)
		response := httptest.NewRecorder()

		handler.Metadata(response, newCIMDMetadataRequest(http.MethodGet, "/.well-known/oauth-client/"+serviceID.String(), serviceID))

		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=300", response.Header().Get("Cache-Control"))
		assertNoCIMDMetadataRedirect(t, response)
		assert.Equal(t, serviceID, provider.metadataServiceID)

		var document map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
		assert.Equal(t, map[string]any{
			"client_id":                       clientID,
			"redirect_uris":                   []any{"https://broker.example.com/api/third-party/" + serviceID.String() + "/oauth2/callback"},
			"grant_types":                     []any{"authorization_code", "refresh_token"},
			"response_types":                  []any{"code"},
			"token_endpoint_auth_method":      "private_key_jwt",
			"token_endpoint_auth_signing_alg": "ES256",
			"jwks_uri":                        clientID + "/jwks.json",
		}, document)
	})
}

func TestCIMDMetadataHandler_JWKS(t *testing.T) {
	t.Run("serves anonymous public ES256 JWKs without private material", func(t *testing.T) {
		serviceID := id.NewServiceID()
		provider := &cimdMetadataProviderFake{jwkSet: newPublicCIMDJWKSet(t)}
		handler := newCIMDMetadataHandlerForTest(provider)
		response := httptest.NewRecorder()

		handler.JWKS(response, newCIMDMetadataRequest(http.MethodGet, "/.well-known/oauth-client/"+serviceID.String()+"/jwks.json", serviceID))

		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
		assert.Equal(t, "public, max-age=300", response.Header().Get("Cache-Control"))
		assertNoCIMDMetadataRedirect(t, response)
		assert.Equal(t, serviceID, provider.jwkSetServiceID)

		var document map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&document))
		require.Len(t, document, 1)
		keys, ok := document["keys"].([]any)
		require.True(t, ok)
		require.Len(t, keys, 1)
		key, ok := keys[0].(map[string]any)
		require.True(t, ok)
		assert.Len(t, key, 7)
		assert.Equal(t, "EC", key["kty"])
		assert.Equal(t, "P-256", key["crv"])
		assert.Equal(t, "cimd-client-key", key["kid"])
		assert.Equal(t, "sig", key["use"])
		assert.Equal(t, "ES256", key["alg"])
		assert.NotEmpty(t, key["x"])
		assert.NotEmpty(t, key["y"])
		for _, privateMember := range []string{"d", "p", "q", "dp", "dq", "qi", "k"} {
			assert.NotContains(t, key, privateMember)
		}
	})
}

func TestCIMDMetadataHandler_UnavailableServices(t *testing.T) {
	unavailableStates := []struct {
		name string
		err  error
	}{
		{name: "unknown", err: ports.ErrNotFound},
		{name: "deleted", err: fmt.Errorf("deleted service: %w", ports.ErrNotFound)},
		{name: "public", err: fmt.Errorf("public service: %w", ports.ErrNotFound)},
		{name: "static confidential", err: fmt.Errorf("static confidential service: %w", ports.ErrNotFound)},
		{name: "no usable published key", err: ports.ErrCIMDPublicKeyUnavailable},
	}
	endpoints := []struct {
		name   string
		invoke func(*CIMDMetadataHandler, http.ResponseWriter, *http.Request)
	}{
		{name: "metadata", invoke: func(handler *CIMDMetadataHandler, w http.ResponseWriter, r *http.Request) { handler.Metadata(w, r) }},
		{name: "jwks", invoke: func(handler *CIMDMetadataHandler, w http.ResponseWriter, r *http.Request) { handler.JWKS(w, r) }},
	}

	for _, state := range unavailableStates {
		for _, endpoint := range endpoints {
			t.Run(state.name+" "+endpoint.name+" response is indistinguishable", func(t *testing.T) {
				serviceID := id.NewServiceID()
				provider := &cimdMetadataProviderFake{metadataErr: state.err, jwkSetErr: state.err}
				handler := newCIMDMetadataHandlerForTest(provider)
				response := httptest.NewRecorder()
				target := "/.well-known/oauth-client/" + serviceID.String()
				if endpoint.name == "jwks" {
					target += "/jwks.json"
				}

				endpoint.invoke(handler, response, newCIMDMetadataRequest(http.MethodGet, target, serviceID))

				require.Equal(t, http.StatusNotFound, response.Code)
				assert.Equal(t, "application/json", response.Header().Get("Content-Type"))
				assertNoCIMDMetadataRedirect(t, response)

				var body map[string]any
				require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
				assert.Equal(t, map[string]any{
					"error":   cimdMetadataNotFoundError,
					"message": cimdMetadataUnavailableText,
				}, body)
				if endpoint.name == "metadata" {
					assert.Equal(t, serviceID, provider.metadataServiceID)
				} else {
					assert.Equal(t, serviceID, provider.jwkSetServiceID)
				}
			})
		}
	}
}

var _ ports.CIMDClientMetadataProvider = (*cimdMetadataProviderFake)(nil)
