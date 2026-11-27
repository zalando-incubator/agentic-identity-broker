package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/agents"
	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/permissionset"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const metadataSentinel = "zz-metadata-sentinel"

type countingEncryption struct {
	ports.EncryptionPort
	calls *atomic.Int64
}

func (c countingEncryption) Encrypt(ctx context.Context, plaintext []byte, encryptionContext map[string]string) ([]byte, error) {
	c.calls.Add(1)
	return c.EncryptionPort.Encrypt(ctx, plaintext, encryptionContext)
}

type countingBranchKeys struct {
	encryptionnoop.BranchKeyManager
	calls *atomic.Int64
}

func (c countingBranchKeys) Create(ctx context.Context, subject domainencryption.BranchKeySubject) (string, error) {
	c.calls.Add(1)
	return c.BranchKeyManager.Create(ctx, subject)
}

// metadataHarness serves the Admin Agent, Service, and Permission Set routes
// over in-memory storage, with production domain services.
type metadataHarness struct {
	t           *testing.T
	router      chi.Router
	logs        *bytes.Buffer
	agents      *memory.AgentRepository
	sets        *memory.PermissionSetRepository
	services    *memory.InMemoryThirdpartyOAuth2ProviderRepository
	providers   *thirdparty.ThirdpartyOAuth2ProviderService
	sideEffects *atomic.Int64
}

func newMetadataHarness(t *testing.T) *metadataHarness {
	t.Helper()
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	sideEffects := &atomic.Int64{}
	agentRepo := memory.NewAgentRepository()
	setRepo := memory.NewPermissionSetRepository().WithAgentRepository(agentRepo)
	serviceRepo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	sets := permissionset.NewPermissionSetService(setRepo, memory.NewUserGrantRepository(), logger)
	t.Cleanup(sets.Close)
	providers := thirdparty.NewThirdpartyOAuth2ProviderService(serviceRepo,
		countingEncryption{EncryptionPort: newTestEncryption(), calls: sideEffects},
		countingBranchKeys{calls: sideEffects}, nil, false, logger).
		WithCIMDPublicURL(testConfig().Server.EndUser.PublicURL).
		WithCIMDKeyReadiness(readyCIMDKeyReadiness{})
	agentHandler := NewAgentsHandler(agents.NewService(agentRepo, providers, logger, true, sets), providers, sets, logger)
	serviceHandler := NewServicesHandler(providers, testConfig(), logger)
	setHandler := NewPermissionSetsHandler(sets, providers, logger)
	router := chi.NewRouter()
	router.Put("/api/agents", agentHandler.ReconcileAgent)
	router.Post("/api/agents", agentHandler.CreateAgent)
	router.Get("/api/agents", agentHandler.ListAgents)
	router.Get("/api/agents/{agent-id}", agentHandler.GetAgent)
	router.Put("/api/agents/{agent-id}", agentHandler.UpdateAgent)
	router.Post("/api/services", serviceHandler.CreateService)
	router.Get("/api/services", serviceHandler.ListServices)
	router.Get("/api/services/{service-id}", serviceHandler.GetService)
	router.Put("/api/services/{service-id}", serviceHandler.UpdateService)
	router.Post("/api/permission-sets", setHandler.Create)
	router.Get("/api/permission-sets", setHandler.List)
	router.Get("/api/permission-sets/{id}", setHandler.Get)
	router.Put("/api/permission-sets/{id}", setHandler.Update)
	return &metadataHarness{t: t, router: router, logs: &logs, agents: agentRepo, sets: setRepo, services: serviceRepo, providers: providers, sideEffects: sideEffects}
}

func (h *metadataHarness) do(method, path, body string) *httptest.ResponseRecorder {
	h.t.Helper()
	recorder := httptest.NewRecorder()
	h.router.ServeHTTP(recorder, httptest.NewRequest(method, path, strings.NewReader(body)))
	return recorder
}

func (h *metadataHarness) decode(recorder *httptest.ResponseRecorder) map[string]any {
	h.t.Helper()
	var value map[string]any
	require.NoError(h.t, json.Unmarshal(recorder.Body.Bytes(), &value), recorder.Body.String())
	return value
}

func (h *metadataHarness) createService(metadata string) map[string]any {
	h.t.Helper()
	created := h.do(http.MethodPost, "/api/services", serviceBody(metadata))
	require.Equal(h.t, http.StatusCreated, created.Code, created.Body.String())
	return h.decode(created)
}

func (h *metadataHarness) createPermissionSet(serviceID, metadata string) map[string]any {
	h.t.Helper()
	created := h.do(http.MethodPost, "/api/permission-sets", permissionSetBody("Read files", serviceID, metadata))
	require.Equal(h.t, http.StatusCreated, created.Code, created.Body.String())
	return h.decode(created)
}

func (h *metadataHarness) createAgent(permissionSetID, metadata string) map[string]any {
	h.t.Helper()
	created := h.do(http.MethodPost, "/api/agents", agentBody(permissionSetID, "", metadata))
	require.Equal(h.t, http.StatusCreated, created.Code, created.Body.String())
	return h.decode(created)
}

// withMetadata appends a raw metadata member to a JSON object body; an empty
// member leaves the body without metadata.
func withMetadata(body, metadata string) string {
	if metadata == "" {
		return body
	}
	return strings.TrimSuffix(body, "}") + `,"metadata":` + metadata + `}`
}

func serviceBody(metadata string) string {
	return withMetadata(`{"display_name":"Atlas","client_id":"atlas-client","client_secret":"secret",`+
		`"issuer_uri":"https://atlas.example","discovery":{"enable_discovery":false},`+
		`"endpoints":{"token_endpoint":"https://atlas.example/token","authorize_endpoint":"https://atlas.example/authorize"},`+
		`"scopes":[{"scope_value":"read","description":"Read"}]}`, metadata)
}

func discoveryServiceBody(metadata string) string {
	return withMetadata(`{"display_name":"Discovered","issuer_uri":"https://must-not-discover.invalid",`+
		`"discovery":{"enable_discovery":true,"resource_url":"https://must-not-discover.invalid/resource"},`+
		`"scopes":[{"scope_value":"read","description":"Read"}]}`, metadata)
}

func permissionSetBody(name, serviceID, metadata string) string {
	return withMetadata(`{"name":"`+name+`","description":"Read","service_scopes":[{"service_id":"`+serviceID+
		`","scopes":["read"],"requirement_type":"mandatory"}]}`, metadata)
}

func agentBody(permissionSetID, displayName, metadata string) string {
	if displayName == "" {
		displayName = "Research"
	}
	return withMetadata(`{"display_name":"`+displayName+`","description":"Definition","permission_sets":[{"permission_set_id":"`+
		permissionSetID+`","requirement_type":"mandatory"}]}`, metadata)
}
