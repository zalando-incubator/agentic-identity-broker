package oauth2session_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestServiceIconCurrentSessionProjections(t *testing.T) {
	ctx := context.Background()
	service, _, providers := setupService(t)
	provider := createTestService(id.NewServiceID())
	first := `<svg xmlns="http://www.w3.org/2000/svg"><text>Atlas 星</text></svg>`
	second := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="8"/></svg>`
	provider.IconSVG, provider.IconSVGProvided = &first, true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"issued-access","refresh_token":"issued-refresh","token_type":"Bearer","expires_in":3600,"scope":"repo user"}`))
	}))
	defer upstream.Close()
	provider.Endpoints.TokenEndpoint = upstream.URL
	require.NoError(t, providers.Create(ctx, provider))
	principal := id.Principal("user@example.com")
	flow, err := service.InitiateOAuth2Flow(ctx, principal, provider.ID, "https://example.com/sessions")
	require.NoError(t, err)
	connected, err := service.HandleCallback(ctx, principal, &oauth2session.HandleCallbackRequest{ServiceID: provider.ID, Code: "code", State: flow.StateToken})
	require.NoError(t, err)
	require.NotNil(t, connected.Session)
	sessionID := connected.Session.ID
	for _, icon := range []*string{&first, &second, nil} {
		current, err := providers.Get(ctx, provider.ID)
		require.NoError(t, err)
		current.IconSVG, current.IconSVGProvided = icon, true
		require.NoError(t, providers.Update(ctx, current, nil))
		list, err := service.ListUserSessions(ctx, principal)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, icon, list[0].ServiceIconSVG)
		assert.Equal(t, sessionID, list[0].ID)
		detail, err := service.GetSessionWithAgents(ctx, principal, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, icon, detail.Session.ServiceIconSVG)
		assert.Equal(t, sessionID, detail.Session.ID)
		refreshed, err := service.ForceRefreshSession(ctx, principal, provider.ID)
		require.NoError(t, err)
		assert.Equal(t, icon, refreshed.ServiceIconSVG)
		wire, err := json.Marshal(refreshed)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(wire, &value))
		if icon == nil {
			assert.NotContains(t, value, "service_icon_svg")
		} else {
			assert.Equal(t, *icon, value["service_icon_svg"])
		}
		assert.NotContains(t, value, "access_token")
		assert.NotContains(t, value, "refresh_token")
	}
}

type delayedProviderLookup struct {
	ports.ThirdpartyOAuth2ProviderRepository
}

func (r delayedProviderLookup) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	time.Sleep(2 * time.Second)
	return r.ThirdpartyOAuth2ProviderRepository.Get(ctx, serviceID)
}

func TestSessionDetailReflectsExpiryDuringProviderLookup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		principal := id.Principal("user@example.com")
		provider := createTestService(id.NewServiceID())
		provider.Secret = model.NewAbsentSecret()
		provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
		providers := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
		require.NoError(t, providers.Create(ctx, provider))
		providerService := thirdparty.NewThirdpartyOAuth2ProviderService(
			delayedProviderLookup{providers}, newTestEncryption(t), newNoopBranchKeyManager(), nil, false, slog.Default())
		sessions := memory.NewInMemoryUserSessionRepository()
		expiry := time.Now().Add(time.Second)
		require.NoError(t, sessions.Create(ctx, &storage.UserSession{
			ID: id.NewSessionID(), Principal: principal, ServiceID: provider.ID,
			EncryptedAccessToken: []byte("ciphertext"), AccessTokenExpiresAt: &expiry,
			TokenType: "Bearer",
		}))
		service := oauth2session.NewOAuth2SessionService(providerService, sessions, sessions,
			memory.NewUserGrantRepository(), nil, nil, nil, nil, oauth2session.DefaultConfig(), slog.Default())
		detail, err := service.GetSessionWithAgents(ctx, principal, provider.ID)
		require.NoError(t, err)
		assert.True(t, detail.Session.AccessTokenExpired)
		assert.True(t, detail.Session.RequiresReauthentication)
		assert.True(t, detail.Session.IsExpired)
	})
}
