package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// DefaultPermissionSets returns a valid mandatory permission set declaration for test agents.
func DefaultPermissionSets() []storage.AgentPermissionSetEntry {
	return []storage.AgentPermissionSetEntry{{
		PermissionSetID: PlaceholderPermissionSetID,
		RequirementType: storage.RequirementTypeMandatory,
	}}
}

func newTestAgent(clientID *id.ClientID, displayName, description string) *storage.Agent {
	now := time.Now()
	return &storage.Agent{
		ID:             id.NewAgentID(),
		ClientID:       clientID,
		DisplayName:    displayName,
		Description:    description,
		PermissionSets: DefaultPermissionSets(),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
}

// ValidAgent returns a valid test agent with all required fields.
// ClientID: test-client-valid
// DisplayName: Test Agent Valid
// RedirectURIs: ["https://client.example.com/cb"]
// ID and timestamps are generated fresh for each call.
func ValidAgent() *storage.Agent {
	agent := newTestAgent(
		ptr.To(id.ClientID("test-client-valid")),
		"Test Agent Valid",
		"A valid test agent for E2E testing with all required fields",
	)
	agent.RedirectURIs = []string{"https://client.example.com/cb"}
	return agent
}

// LocalAgent returns a valid test agent for local/hybrid mode testing.
// It has NO upstream ClientID, so ClientType() returns LocalClient.
// Use this for tests that exercise local token minting (not proxy forwarding).
func LocalAgent() *storage.Agent {
	agent := newTestAgent(nil, "Test Agent Local", "A local-mode test agent with no upstream client_id")
	agent.RedirectURIs = []string{"https://client.example.com/cb"}
	return agent
}

// AnotherAgent returns an alternative valid test agent.
// ClientID: test-client-another
// DisplayName: Test Agent Another
// RedirectURIs: ["https://client.example.com/cb"]
// ID and timestamps are generated fresh for each call.
func AnotherAgent() *storage.Agent {
	agent := newTestAgent(
		ptr.To(id.ClientID("test-client-another")),
		"Test Agent Another",
		"Another valid test agent for E2E testing with different client ID",
	)
	agent.RedirectURIs = []string{"https://client.example.com/cb"}
	return agent
}

// AgentWithClientID returns a valid test agent with a specific client ID.
// Useful for testing scenarios that require a known client ID.
func AgentWithClientID(clientID string) *storage.Agent {
	return newTestAgent(
		ptr.To(id.ClientID(clientID)),
		"Test Agent "+clientID,
		"Test agent with custom client ID: "+clientID,
	)
}

// AgentWithURLs returns a valid test agent with all optional URL fields.
// Includes governance URL, user documentation URL, and agent interface URL.
func AgentWithURLs() *storage.Agent {
	governanceURL := "https://governance.example.com/agent"
	userDocURL := "https://docs.example.com/user-guide"
	agentInterfaceURL := "https://agent.example.com"

	agent := newTestAgent(
		ptr.To(id.ClientID("test-client-with-urls")),
		"Agent With URLs",
		"Test agent with all URL fields populated for documentation and governance",
	)
	agent.GovernanceURL = &governanceURL
	agent.UserDocumentationURL = &userDocURL
	agent.AgentInterfaceURL = &agentInterfaceURL
	return agent
}
