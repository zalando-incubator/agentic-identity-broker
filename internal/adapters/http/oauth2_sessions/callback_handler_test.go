package oauth2_sessions

import (
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
