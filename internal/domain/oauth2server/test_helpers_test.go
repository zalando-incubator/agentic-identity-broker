package oauth2server

import (
	"log/slog"
	"os"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// testAgentID returns a test agent ID for testing.
func testAgentID() id.AgentID {
	return id.NewAgentID()
}

func testAgentPermissionSets() []storage.AgentPermissionSetEntry {
	return []storage.AgentPermissionSetEntry{{
		PermissionSetID: id.NewPermissionSetID(),
		RequirementType: storage.RequirementTypeOptional,
	}}
}

// testAgent returns a test agent for testing.
func testAgent() *storage.Agent {
	return &storage.Agent{
		ID:             id.NewAgentID(),
		ClientID:       ptr.To(id.ClientID("test-client")),
		DisplayName:    "Test Agent",
		Description:    "A test agent for unit testing",
		PermissionSets: testAgentPermissionSets(),
	}
}

// testSlogger returns a slog.Logger for test purposes.
func testSlogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}
