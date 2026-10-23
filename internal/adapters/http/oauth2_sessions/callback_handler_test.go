package oauth2_sessions

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandleCallback_RejectsStateFromOtherPrincipalOrService(t *testing.T) {
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	encryption := testutil.NewTestEncryptionAdapter(t)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		memory.NewInMemoryThirdpartyOAuth2ProviderRepository(), encryption,
		&encryptionnoop.BranchKeyManager{}, nil, false, logger,
	)
	sessionRepo := memory.NewInMemoryUserSessionRepository()
	service := oauth2session.NewOAuth2SessionService(
		providerService, sessionRepo, sessionRepo, memory.NewUserGrantRepository(), memory.NewAgentRepository(),
		encryption, &http.Client{}, domjwe.New(key), oauth2session.DefaultConfig(), logger,
	)
	handler := NewHandler(service)
	handler.logger = logger
	router := chi.NewRouter()
	router.Use(middleware.OptionalPrincipalMiddleware(ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
	}, nil, logger))
	router.Route("/api", handler.RegisterRoutes)

	owner := id.Principal("owner@example.com")
	serviceID := id.NewServiceID()
	state, err := service.CreateStateToken(&oauth2session.OAuth2StateTokenClaims{
		Principal: owner, ServiceID: serviceID,
		PKCEVerifier: strings.Repeat("a", 43),
		RedirectURI:  "https://broker.example.com/sessions",
		IssuedAt:     time.Now(), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)

	for _, test := range []struct {
		name, principal, errorCode string
		pathServiceID              id.ServiceID
		status                     int
	}{
		{"another principal", "other@example.com", "forbidden", serviceID, http.StatusForbidden},
		{"another service", owner.String(), "bad_request", id.NewServiceID(), http.StatusBadRequest},
	} {
		t.Run(test.name, func(t *testing.T) {
			requestURL := "/api/third-party/" + test.pathServiceID.String() + "/oauth2/callback?code=auth-code&state=" + url.QueryEscape(state)
			request := httptest.NewRequest(http.MethodGet, requestURL, nil)
			request.Header.Set("X-Remote-User", test.principal)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			require.Equal(t, test.status, response.Code, response.Body.String())
			var body struct {
				Error string `json:"error"`
			}
			require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
			assert.Equal(t, test.errorCode, body.Error)
			stored, err := sessionRepo.FindByPrincipalAndService(context.Background(), owner, serviceID)
			require.NoError(t, err)
			assert.Nil(t, stored)
		})
	}
}

func TestHandleCallback_ProviderErrorsRequireValidStateAndNeverReflectRemoteText(t *testing.T) {
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))

	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))
	encryption := testutil.NewTestEncryptionAdapter(t)
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
		memory.NewInMemoryThirdpartyOAuth2ProviderRepository(), encryption,
		&encryptionnoop.BranchKeyManager{}, nil, false, logger,
	)
	sessions := memory.NewInMemoryUserSessionRepository()
	service := oauth2session.NewOAuth2SessionService(
		providerService, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(),
		encryption, &http.Client{}, domjwe.New(key), oauth2session.DefaultConfig(), logger,
	)
	handler := NewHandler(service)
	handler.logger = logger
	router := chi.NewRouter()
	router.Use(middleware.OptionalPrincipalMiddleware(ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
	}, nil, logger))
	router.Route("/api", handler.RegisterRoutes)

	serviceID := id.NewServiceID()
	owner := id.Principal("owner@example.com")
	state, err := service.CreateStateToken(&oauth2session.OAuth2StateTokenClaims{
		Principal: owner, ServiceID: serviceID, PKCEVerifier: strings.Repeat("a", 43),
		RedirectURI: "https://broker.example.com/sessions", IssuedAt: time.Now(), ExpiresAt: time.Now().Add(time.Minute),
	})
	require.NoError(t, err)
	const hostile = "private-secret-<script>alert(1)</script>"
	for _, tc := range []struct {
		name, principal, state, providerCode, expectedCode string
		pathServiceID                                      id.ServiceID
		status                                             int
	}{
		{name: "invalid state", principal: owner.String(), state: "forged-state", providerCode: "access_denied", pathServiceID: serviceID, status: http.StatusFound, expectedCode: "invalid_state"},
		{name: "wrong principal", principal: "other@example.com", state: state, providerCode: "access_denied", pathServiceID: serviceID, status: http.StatusForbidden, expectedCode: "forbidden"},
		{name: "wrong service", principal: owner.String(), state: state, providerCode: "access_denied", pathServiceID: id.NewServiceID(), status: http.StatusBadRequest, expectedCode: "bad_request"},
		{name: "unknown provider code", principal: owner.String(), state: state, providerCode: "<script>steal()</script>", pathServiceID: serviceID, status: http.StatusFound, expectedCode: "callback_failed"},
		{name: "known provider code", principal: owner.String(), state: state, providerCode: "access_denied", pathServiceID: serviceID, status: http.StatusFound, expectedCode: "access_denied"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logs.Reset()
			query := url.Values{"state": {tc.state}, "error": {tc.providerCode}, "error_description": {hostile}}
			request := httptest.NewRequest(http.MethodGet, "/api/third-party/"+tc.pathServiceID.String()+"/oauth2/callback?"+query.Encode(), nil)
			request.Header.Set("X-Remote-User", tc.principal)
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.status == http.StatusFound {
				location := response.Header().Get("Location")
				require.Equal(t, "/sessions", strings.SplitN(location, "?", 2)[0])
				parsed, err := url.Parse(location)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedCode, parsed.Query().Get("error"))
				expectedDescription := "Authorization failed - please try again"
				if tc.expectedCode == "invalid_state" {
					expectedDescription = "OAuth2 state token invalid - please try again"
				}
				assert.Equal(t, expectedDescription, parsed.Query().Get("error_description"), "callback errors must have a fixed description")
			} else {
				var body struct {
					Error string `json:"error"`
				}
				require.NoError(t, json.NewDecoder(response.Body).Decode(&body))
				assert.Equal(t, tc.expectedCode, body.Error)
			}
			assert.NotContains(t, logs.String(), hostile)
			if tc.providerCode != "access_denied" {
				assert.NotContains(t, logs.String(), tc.providerCode)
				assert.NotContains(t, response.Header().Get("Location"), tc.providerCode)
			}
			assert.NotContains(t, logs.String(), tc.state, "sealed state must not appear in logs")
			assert.NotContains(t, response.Header().Get("Location"), hostile)
			assert.NotContains(t, response.Body.String(), hostile)
			stored, err := sessions.FindByPrincipalAndService(context.Background(), owner, serviceID)
			require.NoError(t, err)
			assert.Nil(t, stored, "provider error must never establish a session")
		})
	}
}
