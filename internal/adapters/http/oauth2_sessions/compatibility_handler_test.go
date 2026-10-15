package oauth2_sessions

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
)

func TestSessionAPICompatibilityKeepsLegacyClientsAwareOfReauthentication(t *testing.T) {
	ctx := context.Background()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	enc := testutil.NewTestEncryptionAdapter(t)
	providers := thirdparty.NewThirdpartyOAuth2ProviderService(memory.NewInMemoryThirdpartyOAuth2ProviderRepository(), enc, &noop.BranchKeyManager{}, nil, false, logger)
	serviceID := id.NewServiceID()
	require.NoError(t, providers.Create(ctx, &model.ThirdpartyOAuth2ProviderEntity{
		ID: serviceID, DisplayName: "Provider", ClientID: id.ClientID("client"), Secret: model.NewPlaintextSecret("secret"),
		IssuerURI: "https://provider.example", Endpoints: model.OAuth2Endpoints{AuthorizeEndpoint: "https://provider.example/authorize", TokenEndpoint: "https://provider.example/token"},
		Scopes: []model.OAuthScope{{ScopeValue: "read", Description: "Read"}},
	}))
	sessions := memory.NewInMemoryUserSessionRepository()
	principal := id.Principal("user@example.com")
	expiry := time.Now().Add(time.Hour)
	row := &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: serviceID, EncryptedAccessToken: []byte("opaque"), EncryptedRefreshToken: []byte("opaque-refresh"), TokenType: "Bearer", AccessTokenExpiresAt: &expiry, Scope: []string{"read"}, InitiatedAt: time.Now(), CreatedAt: time.Now()}
	require.NoError(t, sessions.Create(ctx, row))
	service := oauth2session.NewOAuth2SessionService(providers, sessions, sessions, memory.NewUserGrantRepository(), memory.NewAgentRepository(), enc, &http.Client{}, nil, oauth2session.DefaultConfig(), logger)
	t.Cleanup(func() { require.NoError(t, service.Close(ctx)) })
	for _, rejected := range []bool{false, true} {
		_, err := sessions.WithLockedSession(ctx, principal, serviceID, func(_ context.Context, current *storage.UserSession) (bool, error) {
			current.RefreshTokenRejected = rejected
			return true, nil
		})
		require.NoError(t, err)
		for _, compatibility := range []bool{true, false} {
			handler := NewHandler(service, compatibility)
			router := chi.NewRouter()
			router.Use(middleware.OptionalPrincipalMiddleware(ports.AuthenticationConfig{Preauth: ports.PreauthConfig{PrincipalHeaderName: "X-Remote-User"}}, nil, logger))
			router.Route("/api", handler.RegisterRoutes)
			for _, path := range []string{"/api/third-party/sessions", "/api/third-party/" + serviceID.String() + "/session"} {
				request := httptest.NewRequest(http.MethodGet, path, nil)
				request.Header.Set("X-Remote-User", principal.String())
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request)
				require.Equal(t, http.StatusOK, response.Code, response.Body.String())
				var body struct {
					Data struct {
						Sessions []map[string]any `json:"sessions"`
						Session  map[string]any   `json:"session"`
					} `json:"data"`
				}
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
				status := body.Data.Session
				if body.Data.Sessions != nil {
					require.Len(t, body.Data.Sessions, 1)
					status = body.Data.Sessions[0]
				}
				require.Equal(t, rejected, status["requires_reauthentication"])
				require.Equal(t, false, status["access_token_expired"])
				require.Equal(t, true, status["has_refresh_token"])
				if compatibility {
					require.Equal(t, rejected, status["is_expired"], "legacy clients must see reauthentication even with unexpired access")
				} else {
					require.NotContains(t, status, "is_expired")
				}
				require.NotContains(t, status, "refresh_token_expires_at")
			}
		}
	}
}
