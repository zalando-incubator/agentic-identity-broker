package oauth2_sessions

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
)

func TestInitiateFlow_InvalidRedirectURILogsNoQueryOrParseError(t *testing.T) {
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
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
	handler := NewHandler(service, logger)
	router := chi.NewRouter()
	router.Use(middleware.OptionalPrincipalMiddleware(ports.AuthenticationConfig{
		Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"},
	}, nil, logger))
	router.Route("/api", handler.RegisterRoutes)

	redirectURI := "https://user:pass@[::1/sessions?marker=query-secret#fragment-secret"
	request := httptest.NewRequest(http.MethodGet,
		"/api/third-party/"+id.NewServiceID().String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(redirectURI), nil)
	request.Header.Set("X-Remote-User", "owner@example.com")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusBadRequest, response.Code)
	out := logs.String()
	assert.NotContains(t, out, "query-secret")
	assert.NotContains(t, out, "fragment-secret")
	assert.NotContains(t, out, "pass@")
	assert.Contains(t, out, `"reason":"invalid_redirect_uri"`)
}
