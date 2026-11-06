package fixtures

import (
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func AgentWithExternalID(externalID id.ExternalID, permissionID id.PermissionSetID) *storage.Agent {
	agent := LocalAgent()
	agent.DisplayName = "Research Assistant"
	agent.Description = "Helps with academic research"
	agent.ExternalID = &externalID
	agent.PermissionSets = []storage.AgentPermissionSetEntry{{
		PermissionSetID: permissionID,
		RequirementType: storage.RequirementTypeMandatory,
	}}
	return agent
}
