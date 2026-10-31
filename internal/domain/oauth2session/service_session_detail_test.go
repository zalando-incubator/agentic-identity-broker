package oauth2session_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// Service session detail from specs/055-incremental-service-authorization (FR-015–FR-016; SR-005).
func TestGetServiceSessionDetail(t *testing.T) {
	ctx := context.Background()
	service, _, sessions, grants, agents, services := setupServiceWithConfig(t, nil)
	var providerCalls atomic.Int32
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		providerCalls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer provider.Close()

	alice, mallory := id.Principal("alice@example.com"), id.Principal("mallory@example.com")
	serviceID := id.NewServiceID()
	github := createTestService(serviceID)
	canonical := "github"
	github.CanonicalID = &canonical
	github.Endpoints.TokenEndpoint = provider.URL
	require.NoError(t, services.Create(ctx, github))

	declared := []storage.AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional}}
	ownAgent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Data Analysis Assistant", Description: "Analyzes repositories", RedirectURIs: []string{"https://client.example.com/cb"}, PermissionSets: declared}
	foreignAgent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Foreign Agent", Description: "Belongs to another user", RedirectURIs: []string{"https://client.example.com/cb"}, PermissionSets: declared}
	require.NoError(t, agents.Create(ctx, ownAgent))
	require.NoError(t, agents.Create(ctx, foreignAgent))
	entry := []storage.GrantedPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{serviceID}}}
	require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: alice, AgentID: ownAgent.ID, GrantedPermissionSets: entry, CreatedAt: time.Now(), UpdatedAt: time.Now()}))
	require.NoError(t, grants.Create(ctx, &storage.UserGrant{ID: id.NewGrantID(), Principal: mallory, AgentID: foreignAgent.ID, GrantedPermissionSets: entry, CreatedAt: time.Now(), UpdatedAt: time.Now()}))

	for _, identifier := range []string{"github", serviceID.String()} {
		detail, err := service.GetServiceSessionDetail(ctx, alice, identifier)
		require.NoError(t, err, identifier)
		assert.Equal(t, serviceID, detail.Service.ID)
		assert.Equal(t, "https://broker.example.com/sessions/github", detail.ScopeURL)
		assert.Nil(t, detail.Session, "a known service without a session has a nil session")
		assert.Equal(t, []oauth2session.AgentInfo{{ID: ownAgent.ID, DisplayName: "Data Analysis Assistant"}}, detail.DependentAgents)
	}

	expired := time.Now().Add(-time.Hour)
	rejected := time.Now().Add(-time.Minute)
	require.NoError(t, sessions.Create(ctx, &storage.UserSession{
		ID: id.NewSessionID(), Principal: alice, ServiceID: serviceID, EncryptedAccessToken: []byte("sealed"),
		EncryptedRefreshToken: []byte("sealed"), TokenType: "Bearer", AccessTokenExpiresAt: &expired, RefreshTokenExpiresAt: &rejected,
		Scope: []string{"repo"}, EncryptionContext: storage.EncryptionContext{ServiceID: serviceID}, InitiatedAt: time.Now(), CreatedAt: time.Now(),
	}))
	detail, err := service.GetServiceSessionDetail(ctx, alice, "github")
	require.NoError(t, err)
	require.NotNil(t, detail.Session)
	assert.True(t, detail.Session.IsExpired(), "a persisted rejection is shown as unusable")
	assert.Zero(t, providerCalls.Load(), "the read never refreshes")

	for _, unknown := range []string{"unknown-service", id.NewServiceID().String(), "github/", "", "880E8400-E29B-41D4-A716-446655440010"} {
		_, err := service.GetServiceSessionDetail(ctx, alice, unknown)
		assert.ErrorIs(t, err, oauth2session.ErrServiceNotFound, unknown)
	}
}
