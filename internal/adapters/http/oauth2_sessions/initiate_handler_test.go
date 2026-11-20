package oauth2_sessions

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domjwe "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwe"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitiateFlow_ReturnURI(t *testing.T) {
	key, err := jwk.Import[jwk.Key]([]byte("test-secret-key-must-be-32-bytes"))
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, "test-key"))
	require.NoError(t, key.Set(jwk.AlgorithmKey, "A256GCM"))
	logger := slog.New(slog.DiscardHandler)
	encryption := testutil.NewTestEncryptionAdapter(t)
	providers := thirdparty.NewThirdpartyOAuth2ProviderService(
		memory.NewInMemoryThirdpartyOAuth2ProviderRepository(), encryption,
		&encryptionnoop.BranchKeyManager{}, nil, false, logger,
	)
	serviceID := id.NewServiceID()
	require.NoError(t, providers.Create(context.Background(), &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "Example", ClientID: id.ClientID("example-client"),
		Secret: model.NewPlaintextSecret("synthetic-secret"), IssuerURI: "https://provider.example.com",
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://provider.example.com/authorize",
			TokenEndpoint:     "https://provider.example.com/token",
		},
	}))
	sessions := memory.NewInMemoryUserSessionRepository()
	config := oauth2session.DefaultConfig()
	config.CallbackBaseURL = "https://broker.example.com"
	service := oauth2session.NewOAuth2SessionService(
		providers, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(),
		encryption, &http.Client{}, domjwe.New(key), config, logger,
	)
	handler := NewHandler(service, logger)
	router := chi.NewRouter()
	router.HandleFunc("/api/third-party/{serviceId}/oauth2/authorize", handler.InitiateFlow)
	owner := id.Principal("alice@example.com")
	const consentID = "550e8400-e29b-41d4-a716-446655440000"

	for _, tc := range []struct {
		name, method, query, form, wantURI, wantConsentID, wantError string
		status                                                       int
	}{
		{name: "omitted GET", method: http.MethodGet, wantURI: config.CallbackBaseURL + "/sessions", status: http.StatusFound},
		{name: "empty GET", method: http.MethodGet, query: "?redirect_uri=", wantURI: config.CallbackBaseURL + "/sessions", status: http.StatusFound},
		{name: "explicit GET", method: http.MethodGet, query: "?redirect_uri=" + url.QueryEscape(config.CallbackBaseURL+"/agents?tab=services"), wantURI: config.CallbackBaseURL + "/agents?tab=services", status: http.StatusFound},
		{name: "reserved consent query stripped", method: http.MethodGet, query: "?redirect_uri=" + url.QueryEscape(config.CallbackBaseURL+"/agents?tab=services&consent_state=legacy&consent_state_id=spoofed"), wantURI: config.CallbackBaseURL + "/agents?tab=services", status: http.StatusFound},
		{name: "foreign origin", method: http.MethodGet, query: "?redirect_uri=" + url.QueryEscape("https://other.example.com/sessions"), wantError: "invalid_redirect_uri", status: http.StatusBadRequest},
		{name: "relative URI", method: http.MethodGet, query: "?redirect_uri=%2Fsessions", wantError: "invalid_redirect_uri", status: http.StatusBadRequest},
		{name: "malformed URI", method: http.MethodGet, query: "?redirect_uri=%25", wantError: "invalid_redirect_uri", status: http.StatusBadRequest},
		{name: "POST missing URI", method: http.MethodPost, form: "consent_state_id=" + consentID, wantError: "invalid_request", status: http.StatusBadRequest},
		{name: "POST missing consent ID", method: http.MethodPost, form: "redirect_uri=" + url.QueryEscape(config.CallbackBaseURL+"/agents"), wantError: "invalid_request", status: http.StatusBadRequest},
		{name: "POST consent state", method: http.MethodPost, form: "redirect_uri=" + url.QueryEscape(config.CallbackBaseURL+"/agents") + "&consent_state_id=" + consentID, wantURI: config.CallbackBaseURL + "/agents", wantConsentID: consentID, status: http.StatusFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			request := httptest.NewRequest(tc.method, "https://untrusted.example.com/api/third-party/"+serviceID.String()+"/oauth2/authorize"+tc.query, strings.NewReader(tc.form))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.Header.Set("X-Forwarded-Host", "forwarded.example.com")
			request.Header.Set("X-Forwarded-Proto", "http")
			request = request.WithContext(principal.WithPrincipal(request.Context(), owner))
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			require.Equal(t, tc.status, response.Code, response.Body.String())
			if tc.wantError != "" {
				var body map[string]string
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				assert.Equal(t, tc.wantError, body["error"])
				assert.Empty(t, response.Header().Get("Location"))
				return
			}
			authorizeURL, err := url.Parse(response.Header().Get("Location"))
			require.NoError(t, err)
			assert.Equal(t, "https://provider.example.com/authorize", authorizeURL.Scheme+"://"+authorizeURL.Host+authorizeURL.Path)
			assert.Equal(t, config.CallbackBaseURL+"/api/third-party/"+serviceID.String()+"/oauth2/callback", authorizeURL.Query().Get("redirect_uri"))
			claims, err := service.ValidateStateToken(authorizeURL.Query().Get("state"), owner, serviceID)
			require.NoError(t, err)
			assert.Equal(t, tc.wantURI, claims.RedirectURI)
			assert.Equal(t, tc.wantConsentID, claims.ConsentStateID)
		})
	}
}
