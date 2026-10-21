package enduser

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cimdAuthTestClientID = "https://client.example/metadata.json"

func cimdAuthTestResolution(clientType storage.ClientType) *mockOAuth2ServiceForToken {
	resolution, err := ports.NewTokenGrantResolution(id.NewAgentID(), nil, clientType)
	if err != nil && clientType == storage.ProxyClient {
		resolution, err = ports.NewTokenGrantResolution(id.NewAgentID(), ptr.To(id.ClientID("upstream-client")), clientType)
	}
	if err != nil {
		panic(err)
	}
	return &mockOAuth2ServiceForToken{resolveFn: func(context.Context, id.ClientID) (*ports.TokenGrantResolution, error) {
		return resolution, nil
	}}
}

func cimdAuthTestForm(grantType string) url.Values {
	form := url.Values{
		"grant_type": {grantType},
		"client_id":  {cimdAuthTestClientID},
	}
	if grantType == "authorization_code" {
		form.Set("code", "test-code")
		form.Set("redirect_uri", "https://client.example/callback")
		form.Set("code_verifier", "test-verifier")
	} else {
		form.Set("refresh_token", "test-refresh")
	}
	return form
}

func runCIMDAuthHandler(form url.Values, authorization string, minting *mockTokenMintingStrategy) *httptest.ResponseRecorder {
	handler := &OAuth2TokenHandler{
		GrantHandler:  NewLocalGrantStrategy(minting, nil),
		OAuth2Service: cimdAuthTestResolution(storage.CIMDClient),
	}
	request := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if authorization != "" {
		request.Header.Set("Authorization", authorization)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func TestCIMDTokenRequestPassesCredentialPresenceToProvider(t *testing.T) {
	for _, test := range []struct {
		name               string
		assertionType      bool
		emptyAssertionType bool
		assertion          bool
		clientSecret       bool
		authorization      string
	}{
		{name: "assertion type without assertion", assertionType: true},
		{name: "assertion without type", assertion: true},
		{name: "assertion combined with secret", assertionType: true, assertion: true, clientSecret: true},
		{name: "assertion combined with authorization header", assertionType: true, assertion: true, authorization: "Basic dGVzdA=="},
		{name: "empty assertion type is still present", emptyAssertionType: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			var received ports.ClientCredentials
			minting := &mockTokenMintingStrategy{
				authorizationCodeExchangeFn: func(_ context.Context, _ id.ClientID, credentials ports.ClientCredentials, _, _, _ string) (*ports.TokenResponse, error) {
					called = true
					received = credentials
					return &ports.TokenResponse{AccessToken: "unexpected"}, nil
				},
				refreshTokenFn: func(context.Context, id.ClientID, ports.ClientCredentials, string, string) (*ports.TokenResponse, error) {
					called = true
					return &ports.TokenResponse{AccessToken: "unexpected"}, nil
				},
			}
			form := cimdAuthTestForm("authorization_code")
			if test.emptyAssertionType {
				form["client_assertion_type"] = []string{""}
			} else if test.assertionType {
				form.Set("client_assertion_type", oauth2server.ClientAssertionTypeJWTBearer)
			}
			if test.assertion {
				form.Set("client_assertion", "signed-assertion")
			}
			if test.clientSecret {
				form.Set("client_secret", "unexpected-secret")
			}
			response := runCIMDAuthHandler(form, test.authorization, minting)
			assert.Equal(t, http.StatusOK, response.Code)
			assert.True(t, called, "the adapter must pass credential facts to the provider")
			assert.Equal(t, test.assertionType || test.emptyAssertionType, received.HasAssertionType)
			assert.Equal(t, test.assertion, received.HasAssertion)
			assert.Equal(t, test.clientSecret, received.HasClientSecret)
			assert.Equal(t, test.authorization != "", received.HasAuthorizationHeader)
		})
	}
}

func TestCIMDTokenRequestUsesStableAuthenticationErrorDescription(t *testing.T) {
	for _, cause := range []string{"signature", "expired", "audience"} {
		form := cimdAuthTestForm("authorization_code")
		form.Set("client_assertion_type", oauth2server.ClientAssertionTypeJWTBearer)
		form.Set("client_assertion", "assertion-"+cause)
		minting := &mockTokenMintingStrategy{
			authorizationCodeExchangeFn: func(context.Context, id.ClientID, ports.ClientCredentials, string, string, string) (*ports.TokenResponse, error) {
				return nil, oauth2server.NewRFC6749Error("invalid_client", cause, http.StatusUnauthorized, oauth2server.ErrInvalidClient)
			},
		}
		response := runCIMDAuthHandler(form, "", minting)
		assert.Equal(t, http.StatusUnauthorized, response.Code)
		var body map[string]any
		require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
		assert.Equal(t, "invalid_client", body["error"])
		assert.NotEqual(t, cause, body["error_description"])
	}
}

func TestNonCIMDClientsKeepExistingAssertionHandling(t *testing.T) {
	t.Run("proxy request is forwarded unchanged", func(t *testing.T) {
		var upstreamForm url.Values
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.NoError(t, r.ParseForm())
			upstreamForm = r.Form
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"upstream-token","token_type":"Bearer"}`))
		}))
		defer upstream.Close()
		resolution, err := ports.NewTokenGrantResolution(id.NewAgentID(), ptr.To(id.ClientID("upstream-client")), storage.ProxyClient)
		require.NoError(t, err)
		oauthService := &mockOAuth2ServiceForToken{resolveFn: func(context.Context, id.ClientID) (*ports.TokenGrantResolution, error) { return resolution, nil }}
		handler := &OAuth2TokenHandler{GrantHandler: newProxyTokenGrantStrategyForTest(upstream.URL, nil), OAuth2Service: oauthService}
		form := cimdAuthTestForm("authorization_code")
		form.Set("client_assertion_type", oauth2server.ClientAssertionTypeJWTBearer)
		form.Set("client_assertion", "proxy-assertion")
		request := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, oauth2server.ClientAssertionTypeJWTBearer, upstreamForm.Get("client_assertion_type"))
		assert.Equal(t, "proxy-assertion", upstreamForm.Get("client_assertion"))
	})

	t.Run("local client secret and assertion are passed through unchanged", func(t *testing.T) {
		resolution, err := ports.NewTokenGrantResolution(id.NewAgentID(), nil, storage.LocalClient)
		require.NoError(t, err)
		var received ports.ClientCredentials
		minting := &mockTokenMintingStrategy{
			authorizationCodeExchangeFn: func(_ context.Context, _ id.ClientID, creds ports.ClientCredentials, _, _, _ string) (*ports.TokenResponse, error) {
				received = creds
				return &ports.TokenResponse{AccessToken: "local-token", TokenType: "Bearer"}, nil
			},
		}
		oauthService := &mockOAuth2ServiceForToken{resolveFn: func(context.Context, id.ClientID) (*ports.TokenGrantResolution, error) { return resolution, nil }}
		handler := &OAuth2TokenHandler{GrantHandler: NewLocalGrantStrategy(minting, nil), OAuth2Service: oauthService}
		form := cimdAuthTestForm("authorization_code")
		form.Set("client_secret", "local-secret")
		form.Set("client_assertion_type", oauth2server.ClientAssertionTypeJWTBearer)
		form.Set("client_assertion", "local-assertion")
		request := httptest.NewRequest("POST", "/oauth2/token", strings.NewReader(form.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		require.Equal(t, http.StatusOK, response.Code)
		assert.Equal(t, "local-secret", received.Secret)
		assert.Equal(t, "local-assertion", received.Assertion)
	})
}
