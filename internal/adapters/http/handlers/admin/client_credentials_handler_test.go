package admin

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/cascadefixture"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type clientCredentialRepositoryFake struct {
	credential *storage.ClientCredential
	deletedID  id.AgentID
}

func (r *clientCredentialRepositoryFake) Create(_ context.Context, credential *storage.ClientCredential) error {
	r.credential = credential
	return nil
}

func (r *clientCredentialRepositoryFake) GetByAgentID(_ context.Context, _ id.AgentID) (*storage.ClientCredential, error) {
	if r.credential == nil {
		return nil, ports.ErrNotFound
	}
	return r.credential, nil
}

func (r *clientCredentialRepositoryFake) GetByClientID(_ context.Context, _ id.ClientID) (*storage.ClientCredential, error) {
	return r.GetByAgentID(context.Background(), id.AgentID{})
}

func (r *clientCredentialRepositoryFake) Delete(_ context.Context, agentID id.AgentID) error {
	if r.credential == nil {
		return ports.ErrNotFound
	}
	r.deletedID = agentID
	r.credential = nil
	return nil
}

func (r *clientCredentialRepositoryFake) Rotate(_ context.Context, _ id.AgentID, credential *storage.ClientCredential) error {
	r.credential = credential
	return nil
}

type credentialGeneratorFake struct {
	credential *storage.ClientCredential
}

func (g credentialGeneratorFake) GenerateCredentials(agentID id.AgentID) (*storage.ClientCredential, string, error) {
	credential := *g.credential
	credential.AgentID = agentID
	return &credential, "generated-secret", nil
}

func newClientCredentialsHandlerForTest(agentRepo *MockAgentRepository, serviceRepo *MockProviderRepository, credentialRepo ports.ClientCredentialRepository, generator ports.CredentialGenerator) *ClientCredentialsHandler {
	providerService := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())
	agentService := agents.NewService(agentRepo, providerService, slog.Default(), true, ledgerfixture.NewRecorder(), cascadefixture.NewAgentDependents(), cascadefixture.NewCredentialRepository())
	credentialService := oauth2server.NewCredentialService(agentRepo, credentialRepo, generator, slog.Default(), ledgerfixture.NewRecorder())
	return NewClientCredentialsHandler(credentialService, agentService, slog.Default())
}

func canonicalCredentialRequest(method, canonicalID string) *http.Request {
	req := httptest.NewRequest(method, "/api/agents/"+canonicalID+"/client-credentials", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", canonicalID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func TestClientCredentialsHandler_CanonicalAgentPaths(t *testing.T) {
	canonicalID := "canonical-agent"
	agentID := id.NewAgentID()
	agent := &storage.Agent{ID: agentID, DisplayName: "Canonical Agent", Description: "Agent"}
	credential := &storage.ClientCredential{ID: id.NewCredentialID(), AgentID: agentID, SecretHash: "hash", CreatedAt: time.Now().UTC()}

	t.Run("generates credentials through a canonical agent path", func(t *testing.T) {
		agentRepo := new(MockAgentRepository)
		serviceRepo := new(MockProviderRepository)
		credentialRepo := &clientCredentialRepositoryFake{}
		handler := newClientCredentialsHandlerForTest(agentRepo, serviceRepo, credentialRepo, credentialGeneratorFake{credential: credential})
		agentRepo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(agent, nil)
		agentRepo.On("Get", mock.Anything, agentID).Return(agent, nil)
		w := httptest.NewRecorder()
		handler.Generate(w, canonicalCredentialRequest(http.MethodPost, canonicalID))
		require.Equal(t, http.StatusCreated, w.Code)
		assert.NotNil(t, credentialRepo.credential)
		assert.Equal(t, agentID, credentialRepo.credential.AgentID)
		agentRepo.AssertExpectations(t)
	})

	t.Run("reads metadata through a canonical agent path", func(t *testing.T) {
		agentRepo := new(MockAgentRepository)
		serviceRepo := new(MockProviderRepository)
		credentialRepo := &clientCredentialRepositoryFake{credential: credential}
		handler := newClientCredentialsHandlerForTest(agentRepo, serviceRepo, credentialRepo, credentialGeneratorFake{credential: credential})
		agentRepo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(agent, nil)
		w := httptest.NewRecorder()
		handler.Get(w, canonicalCredentialRequest(http.MethodGet, canonicalID))
		require.Equal(t, http.StatusOK, w.Code)
		agentRepo.AssertExpectations(t)
	})

	t.Run("revokes credentials through a canonical agent path", func(t *testing.T) {
		agentRepo := new(MockAgentRepository)
		serviceRepo := new(MockProviderRepository)
		credentialRepo := &clientCredentialRepositoryFake{credential: credential}
		handler := newClientCredentialsHandlerForTest(agentRepo, serviceRepo, credentialRepo, credentialGeneratorFake{credential: credential})
		agentRepo.On("GetByCanonicalID", mock.Anything, canonicalID).Return(agent, nil)
		agentRepo.On("Get", mock.Anything, agentID).Return(agent, nil)
		w := httptest.NewRecorder()
		handler.Revoke(w, canonicalCredentialRequest(http.MethodDelete, canonicalID))
		require.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, agentID, credentialRepo.deletedID)
		agentRepo.AssertExpectations(t)
	})
}
