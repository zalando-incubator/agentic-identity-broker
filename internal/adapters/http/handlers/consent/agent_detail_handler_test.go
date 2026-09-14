package consent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/testutil"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newTestEncryption returns a real encryption adapter backed by the shared deterministic test key.
func newTestEncryption() ports.EncryptionPort {
	return testutil.NewPanicTestEncryptionAdapter()
}

// newTestProviderService wraps a ThirdpartyOAuth2ProviderRepository in a domain service
// with test encryption. Used in tests across the consent handler package.
func newTestProviderService(repo ports.ThirdpartyOAuth2ProviderRepository) *thirdparty.ThirdpartyOAuth2ProviderService {
	return thirdparty.NewThirdpartyOAuth2ProviderService(repo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, slog.Default())
}

// mockAgentDetailService is a configurable mock implementation of ConsentService for testing.
type mockAgentDetailService struct {
	getAgentConsentDetailFunc func(ctx context.Context, agentID id.AgentID, principal id.Principal) (*consent.AgentConsentDetail, error)
}

func (m *mockAgentDetailService) GetAgentConsentDetail(ctx context.Context, agentID id.AgentID, p id.Principal) (*consent.AgentConsentDetail, error) {
	if m.getAgentConsentDetailFunc != nil {
		return m.getAgentConsentDetailFunc(ctx, agentID, p)
	}
	return &consent.AgentConsentDetail{Agent: &storage.Agent{}}, nil
}

func (m *mockAgentDetailService) GrantConsent(ctx context.Context, req *consent.GrantRequest) (*storage.UserGrant, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAgentDetailService) RevokeConsent(ctx context.Context, p id.Principal, agentID id.AgentID) error {
	return errors.New("not implemented")
}

func (m *mockAgentDetailService) GetAgentDelegations(ctx context.Context, p id.Principal) ([]consent.AgentDelegation, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAgentDetailService) GetUserGrants(ctx context.Context, p id.Principal, agentID id.AgentID) ([]*storage.UserGrant, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAgentDetailService) RevokeConsentForPrincipal(ctx context.Context, p id.Principal, agentID id.AgentID) error {
	return nil
}

func TestGetAgentDetail_Success(t *testing.T) {
	agentID := id.NewAgentID()
	principalID := "user@example.com"
	governanceURL := "https://example.com/governance"
	userDocsURL := "https://example.com/docs"
	agentInterfaceURL := "https://example.com/interface"
	githubServiceID := id.NewServiceID()
	googleServiceID := id.NewServiceID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{
					ID:                   agentID,
					ClientID:             ptr.To(id.NewClientID("test-client-id")),
					DisplayName:          "Test Agent",
					Description:          "A test agent for testing purposes",
					GovernanceURL:        &governanceURL,
					UserDocumentationURL: &userDocsURL,
					AgentInterfaceURL:    &agentInterfaceURL,
				},
				ServiceRequirements: []consent.ServiceRequirementStatus{
					{
						ServiceID:       githubServiceID,
						DisplayName:     "GitHub",
						RequirementType: storage.RequirementTypeMandatory,
						RequiredScopes: []consent.ServiceScopeInfo{
							{Name: "read:user", Description: "Read user profile"},
							{Name: "repo", Description: "Full control of repositories"},
						},
						IsConnected: false,
					},
					{
						ServiceID:       googleServiceID,
						DisplayName:     "Google",
						RequirementType: storage.RequirementTypeOptional,
						RequiredScopes:  []consent.ServiceScopeInfo{{Name: "email", Description: "View email address"}},
						IsConnected:     false,
					},
				},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), principalID))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code, rr.Body.String())

	var response GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))

	assert.Equal(t, agentID.String(), response.Data.Agent.ID)
	assert.Equal(t, "Test Agent", response.Data.Agent.DisplayName)
	require.NotNil(t, response.Data.Agent.GovernanceURL)
	assert.Equal(t, governanceURL, *response.Data.Agent.GovernanceURL)

	require.Len(t, response.Data.Services, 2)
	assert.Equal(t, githubServiceID.String(), response.Data.Services[0].ServiceID)
	assert.Len(t, response.Data.Services[0].RequiredScopes, 2)
}

func TestToServiceRequirementForUser_SerializesDisclosedScopes(t *testing.T) {
	serviceID := id.NewServiceID()

	result := toServiceRequirementForUser([]consent.ServiceRequirementStatus{{
		ServiceID:       serviceID,
		DisplayName:     "GitHub",
		RequirementType: storage.RequirementTypeMandatory,
		RequiredScopes: []consent.ServiceScopeInfo{
			{Name: "admin", Description: "Admin access"},
			{Name: "read", Description: "Read access"},
			{Name: "write", Description: "Write access"},
		},
	}})

	require.Len(t, result, 1)
	assert.Equal(t, serviceID.String(), result[0].ServiceID)
	assert.Equal(t, []ScopeWithDescription{
		{Name: "admin", Description: "Admin access"},
		{Name: "read", Description: "Read access"},
		{Name: "write", Description: "Write access"},
	}, result[0].RequiredScopes)
}
func TestGetAgentDetail_DomainErrors(t *testing.T) {
	agentID := id.NewAgentID()
	tests := []struct {
		name            string
		serviceError    error
		expectedStatus  int
		expectedError   string
		expectedMessage string
	}{
		{
			name:            "agent not found",
			serviceError:    consent.ErrAgentNotFound,
			expectedStatus:  http.StatusNotFound,
			expectedError:   "not found",
			expectedMessage: "agent not found",
		},
		{
			name:            "missing permission sets",
			serviceError:    fmt.Errorf("%w: agent must declare at least one permission set", consent.ErrMissingMandatoryPS),
			expectedStatus:  http.StatusBadRequest,
			expectedError:   "missing mandatory permission set",
			expectedMessage: "missing mandatory permission set: agent must declare at least one permission set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockService := &mockAgentDetailService{
				getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
					return nil, tt.serviceError
				},
			}
			handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

			req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
			rctx := chi.NewRouteContext()
			rctx.URLParams.Add("agent-id", agentID.String())
			req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
			req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

			rr := httptest.NewRecorder()
			handler.GetAgentDetail(rr, req)

			require.Equal(t, tt.expectedStatus, rr.Code)

			var response ErrorResponse
			require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
			assert.Equal(t, tt.expectedError, response.Error)
			assert.Equal(t, tt.expectedMessage, response.Message)
		})
	}
}

func TestGetAgentDetail_MissingAgentID(t *testing.T) {
	handler := NewAgentDetailHandler(&mockAgentDetailService{}, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/", nil)
	rctx := chi.NewRouteContext()
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var response ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "bad request", response.Error)
}

func TestGetAgentDetail_ServiceError(t *testing.T) {
	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return nil, storage.NewStorageError("GetAgent", storage.ErrorKindConnection, errors.New("database connection failed"), "db unavailable")
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	testAgentID := id.NewAgentID()
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+testAgentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", testAgentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	var response ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "internal server error", response.Error)
}

func TestGetAgentDetail_ServiceRequirementError(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return nil, storage.NewStorageError("FindByPrincipalAndService", storage.ErrorKindConnection, nil, "database unavailable")
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusInternalServerError, rr.Code)

	var response ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Equal(t, "internal server error", response.Error)
}

func TestGetAgentDetail_EmptyServicesList(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{
					ID:          agentID,
					ClientID:    ptr.To(id.NewClientID("test-client-id")),
					DisplayName: "Test Agent",
					Description: "A test agent",
				},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var response GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	assert.Len(t, response.Data.Services, 0)
}

func TestResolveCIMDMetadata_SessionAgentMismatch(t *testing.T) {
	agentA := id.NewAgentID()
	agentB := id.NewAgentID()
	principalID := "user@example.com"

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, agentID id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent:               &storage.Agent{ID: agentID, ClientID: ptr.To(id.NewClientID("test")), DisplayName: "Agent"},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	ts := newTestJWETokenService()
	validator := newTestSessionTokenValidator()
	tokenForAgentA := newTestSessionToken(ts, agentA, principalID, "https://example.com/original")

	handler := NewAgentDetailHandler(mockService, nil, validator)

	// Request agent B's detail with a session token that belongs to agent A
	req := httptest.NewRequest(http.MethodGet,
		"/api/consent/agent/"+agentB.String()+"?session_token="+tokenForAgentA, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentB.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), principalID))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "bad request", resp.Error)
	assert.Equal(t, "invalid authorization session", resp.Message)
}

func TestResolveCIMDMetadata_SessionPrincipalMismatch(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, aID id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent:               &storage.Agent{ID: aID, ClientID: ptr.To(id.NewClientID("test")), DisplayName: "Agent"},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	ts := newTestJWETokenService()
	validator := newTestSessionTokenValidator()
	tokenForUserA := newTestSessionToken(ts, agentID, "userA@example.com", "https://example.com/original")

	handler := NewAgentDetailHandler(mockService, nil, validator)

	// User B tries to use user A's session token
	req := httptest.NewRequest(http.MethodGet,
		"/api/consent/agent/"+agentID.String()+"?session_token="+tokenForUserA, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "userB@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "bad request", resp.Error)
	assert.Equal(t, "invalid authorization session", resp.Message)
}

func TestGetAgentDetail_ExpiredSessionToken(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, aID id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent:               &storage.Agent{ID: aID, ClientID: ptr.To(id.NewClientID("test")), DisplayName: "Agent"},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	ts := newTestJWETokenService()
	validator := newTestSessionTokenValidator()
	expiredToken := newExpiredTestSessionToken(ts, agentID, "user@example.com")

	handler := NewAgentDetailHandler(mockService, nil, validator)

	req := httptest.NewRequest(http.MethodGet,
		"/api/consent/agent/"+agentID.String()+"?session_token="+expiredToken, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "session_expired", resp.Error)
}

// TestGetAgentDetail_MalformedSessionToken verifies that a session_token that is not
// valid JWE compact serialization returns 400 (T017a).
func TestGetAgentDetail_MalformedSessionToken(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, aID id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent:               &storage.Agent{ID: aID, ClientID: ptr.To(id.NewClientID("test")), DisplayName: "Agent"},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet,
		"/api/consent/agent/"+agentID.String()+"?session_token=notvalidjwe", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
	var resp ErrorResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, "invalid_token", resp.Error)
}

func TestGetAgentDetail_SortsMandatoryFirst(t *testing.T) {
	agentID := id.NewAgentID()
	optionalServiceID := id.NewServiceID()
	mandatoryServiceID := id.NewServiceID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{ID: agentID, ClientID: ptr.To(id.NewClientID("c")), DisplayName: "Agent"},
				ServiceRequirements: []consent.ServiceRequirementStatus{
					{ServiceID: optionalServiceID, DisplayName: "Optional", RequirementType: storage.RequirementTypeOptional},
					{ServiceID: mandatoryServiceID, DisplayName: "Mandatory", RequirementType: storage.RequirementTypeMandatory},
				},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)

	var response GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&response))
	require.Len(t, response.Data.Services, 2)
	assert.Equal(t, "mandatory", response.Data.Services[0].RequirementType)
	assert.Equal(t, "optional", response.Data.Services[1].RequirementType)
}

func TestGetAgentDetail_ConnectionStatus(t *testing.T) {
	agentID := id.NewAgentID()
	svcID := id.NewServiceID()

	t.Run("connected", func(t *testing.T) {
		mockService := &mockAgentDetailService{
			getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
				return &consent.AgentConsentDetail{
					Agent:               &storage.Agent{ID: agentID, ClientID: ptr.To(id.NewClientID("c")), DisplayName: "A"},
					ServiceRequirements: []consent.ServiceRequirementStatus{{ServiceID: svcID, IsConnected: true, RequirementType: storage.RequirementTypeMandatory}},
				}, nil
			},
		}
		handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
		req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", agentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		req = req.WithContext(principal.WithPrincipal(req.Context(), "u@example.com"))
		rr := httptest.NewRecorder()
		handler.GetAgentDetail(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		var resp GetAgentDetailResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		require.Len(t, resp.Data.Services, 1)
		assert.Equal(t, "connected", resp.Data.Services[0].ConnectionStatus)
	})

	t.Run("not_connected", func(t *testing.T) {
		mockService := &mockAgentDetailService{
			getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
				return &consent.AgentConsentDetail{
					Agent:               &storage.Agent{ID: agentID, ClientID: ptr.To(id.NewClientID("c")), DisplayName: "A"},
					ServiceRequirements: []consent.ServiceRequirementStatus{{ServiceID: svcID, IsConnected: false, RequirementType: storage.RequirementTypeMandatory}},
				}, nil
			},
		}
		handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
		req := httptest.NewRequest(http.MethodGet, "/api/consent/agent/"+agentID.String(), nil)
		rctx := chi.NewRouteContext()
		rctx.URLParams.Add("agent-id", agentID.String())
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
		req = req.WithContext(principal.WithPrincipal(req.Context(), "u@example.com"))
		rr := httptest.NewRecorder()
		handler.GetAgentDetail(rr, req)
		require.Equal(t, http.StatusOK, rr.Code)
		var resp GetAgentDetailResponse
		require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
		require.Len(t, resp.Data.Services, 1)
		assert.Equal(t, "not_connected", resp.Data.Services[0].ConnectionStatus)
	})
}

func TestGetAgentDetail_NilClientID(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{
					ID:          agentID,
					ClientID:    nil,
					DisplayName: "Local Agent",
					Description: "Agent without client_id",
				},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	assert.Equal(t, agentID.String(), resp.Data.Agent.ID)
	assert.Empty(t, resp.Data.Agent.ClientID, "client_id should be omitted for local agents")
	assert.Nil(t, resp.Data.Agent.ClientURIs)
}

func TestGetAgentDetail_CIMDAgent(t *testing.T) {
	agentID := id.NewAgentID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{
					ID:          agentID,
					ClientID:    nil,
					ClientURIs:  []string{"https://example.com/.well-known/oauth-client"},
					DisplayName: "CIMD Agent",
					Description: "Agent resolved via CIMD URL",
				},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	assert.Empty(t, resp.Data.Agent.ClientID)
	assert.Equal(t, []string{"https://example.com/.well-known/oauth-client"}, resp.Data.Agent.ClientURIs)
}

func TestGetAgentDetail_PermissionSetsIncluded(t *testing.T) {
	agentID := id.NewAgentID()
	psID := id.NewPermissionSetID()
	svcID := id.NewServiceID()

	mockService := &mockAgentDetailService{
		getAgentConsentDetailFunc: func(_ context.Context, _ id.AgentID, _ id.Principal) (*consent.AgentConsentDetail, error) {
			return &consent.AgentConsentDetail{
				Agent: &storage.Agent{
					ID:          agentID,
					ClientID:    ptr.To(id.NewClientID("proxy-client")),
					DisplayName: "Proxy Agent",
					Description: "Agent with permission sets",
					ServiceRequirements: []storage.ServiceRequirement{
						{ServiceID: svcID, RequirementType: storage.RequirementTypeMandatory},
					},
				},
				ServiceRequirements: []consent.ServiceRequirementStatus{},
				ResolvedPermissionSets: []consent.ResolvedPermissionSetEntry{
					{
						PermissionSet: &storage.PermissionSet{
							ID:          psID,
							Name:        "Core Access",
							Description: "Core permissions",
							ServiceScopes: []storage.ServiceScope{
								{ServiceID: svcID, RequirementType: storage.RequirementTypeMandatory},
							},
						},
						RequirementType: storage.RequirementTypeMandatory,
					},
				},
				ActiveSessionServiceIDs: []id.ServiceID{svcID},
			}, nil
		},
	}

	handler := NewAgentDetailHandler(mockService, nil, newTestSessionTokenValidator())
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents/"+agentID.String(), nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("agent-id", agentID.String())
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	req = req.WithContext(principal.WithPrincipal(req.Context(), "user@example.com"))

	rr := httptest.NewRecorder()
	handler.GetAgentDetail(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp GetAgentDetailResponse
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))

	require.Len(t, resp.Data.PermissionSets, 1)
	assert.Equal(t, psID.String(), resp.Data.PermissionSets[0].PermissionSet.ID)
	assert.Equal(t, "mandatory", resp.Data.PermissionSets[0].RequirementType)

	require.Len(t, resp.Data.ActiveSessionIDs, 1)
	assert.Equal(t, svcID.String(), resp.Data.ActiveSessionIDs[0])

	require.Len(t, resp.Data.ServiceRequirements, 1)
	assert.Equal(t, svcID.String(), resp.Data.ServiceRequirements[0].ServiceID)
	assert.Equal(t, "mandatory", resp.Data.ServiceRequirements[0].RequirementType)
}
