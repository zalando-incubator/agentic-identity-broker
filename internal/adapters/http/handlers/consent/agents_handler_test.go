package consent

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// TestGetAgentDelegations_Success tests successful retrieval of agent delegations.
func TestGetAgentDelegations_Success(t *testing.T) {
	// Setup mock data
	principalValue := "user@example.com"
	expiresAt := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	mockDelegations := []consent.AgentDelegation{
		{
			AgentID:          id.MustParseAgentID("00000000-0000-0000-0000-000000000001"),
			DisplayName:      "Data Analysis Assistant",
			LogoURL:          nil,
			ActiveGrantCount: 3,
			LastModifiedAt:   time.Date(2025, 12, 17, 14, 30, 0, 0, time.UTC),
			ExpiresAt:        nil,
		},
		{
			AgentID:          id.MustParseAgentID("00000000-0000-0000-0000-000000000002"),
			DisplayName:      "Document Processor",
			LogoURL:          nil,
			ActiveGrantCount: 2,
			LastModifiedAt:   time.Date(2025, 12, 16, 9, 15, 0, 0, time.UTC),
			ExpiresAt:        &expiresAt,
		},
	}

	// Create mock service
	mockSvc := newMockAgentsService(t)
	mockSvc.On("GetAgentDelegations", mock.Anything, id.Principal(principalValue)).Return(mockDelegations, nil).Once()

	// Create handler
	handler := NewAgentsHandler(mockSvc, nil)

	// Create request with principal in context
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	req = req.WithContext(ctx)

	// Execute request
	rec := httptest.NewRecorder()
	handler.GetAgentDelegations(rec, req)

	// Assert response
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp GetAgentDelegationsResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify delegations (order is non-deterministic due to map iteration)
	assert.Len(t, resp.Data, 2)

	// Build a map for easier lookup
	delegationMap := make(map[string]consent.AgentDelegation)
	for _, d := range resp.Data {
		delegationMap[d.AgentID.String()] = d
	}

	// Verify agent-1
	agent1, ok := delegationMap["00000000-0000-0000-0000-000000000001"]
	require.True(t, ok, "agent-1 should be in response")
	assert.Equal(t, "Data Analysis Assistant", agent1.DisplayName)
	assert.Equal(t, 3, agent1.ActiveGrantCount)
	assert.Nil(t, agent1.ExpiresAt)

	// Verify agent-2
	agent2, ok := delegationMap["00000000-0000-0000-0000-000000000002"]
	require.True(t, ok, "agent-2 should be in response")
	assert.Equal(t, "Document Processor", agent2.DisplayName)
	assert.Equal(t, 2, agent2.ActiveGrantCount)
	assert.NotNil(t, agent2.ExpiresAt)
	assert.Equal(t, expiresAt, *agent2.ExpiresAt)
}

// TestGetAgentDelegations_EmptyList tests successful retrieval with no delegations.
func TestGetAgentDelegations_EmptyList(t *testing.T) {
	principalValue := "user@example.com"

	mockSvc := newMockAgentsService(t)
	mockSvc.On("GetAgentDelegations", mock.Anything, id.Principal(principalValue)).Return([]consent.AgentDelegation{}, nil).Once()

	handler := NewAgentsHandler(mockSvc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetAgentDelegations(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GetAgentDelegationsResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Empty(t, resp.Data)
}

// TestGetAgentDelegations_MissingPrincipal tests error when principal is not in context.
func TestGetAgentDelegations_MissingPrincipal(t *testing.T) {
	mockSvc := newMockAgentsService(t)
	handler := NewAgentsHandler(mockSvc, nil)

	// Request without principal in context
	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents", nil)

	rec := httptest.NewRecorder()
	handler.GetAgentDelegations(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)
}

// TestGetAgentDelegations_ServiceError tests error handling when service fails.
func TestGetAgentDelegations_ServiceError(t *testing.T) {
	principalValue := "user@example.com"

	mockSvc := newMockAgentsService(t)
	mockSvc.On("GetAgentDelegations", mock.Anything, id.Principal(principalValue)).Return([]consent.AgentDelegation(nil), errors.New("database connection failed")).Once()

	handler := NewAgentsHandler(mockSvc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetAgentDelegations(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)

	var resp ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "internal server error", resp.Error)
}

// TestGetAgentDelegations_ContentType tests that response has correct content type.
func TestGetAgentDelegations_ContentType(t *testing.T) {
	principalValue := "user@example.com"

	mockSvc := newMockAgentsService(t)
	mockSvc.On("GetAgentDelegations", mock.Anything, id.Principal(principalValue)).Return([]consent.AgentDelegation{}, nil).Once()

	handler := NewAgentsHandler(mockSvc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/consent/agents", nil)
	ctx := principal.WithPrincipal(req.Context(), principalValue)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetAgentDelegations(rec, req)

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}
