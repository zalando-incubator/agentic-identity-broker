package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const (
	ReportsClientV1 = "reports-client-v1"
	ReportsClientV2 = "reports-client-v2"
	ReportsSecretV1 = "synthetic-reports-secret-v1"
	ReportsSecretV2 = "synthetic-reports-secret-v2"
)

func ClientIdentityServiceRequest(providerURL, source string) map[string]any {
	request := CredentialServiceRequest("reports-prod", providerURL, source)
	request["display_name"] = "Reports Production"
	if source != "filesystem" {
		request["client_id"] = ReportsClientV1
		request["client_secret"] = ReportsSecretV1
	}
	return request
}

func ClientIdentityPermissionSet(serviceID id.ServiceID, name string) *storage.PermissionSet {
	now := time.Now()
	return &storage.PermissionSet{
		ID: id.NewPermissionSetID(), Name: name, Description: "Reports access for client identity recovery",
		ServiceScopes: []storage.ServiceScope{{
			ServiceID: serviceID, Scopes: []string{"profile"}, RequirementType: storage.RequirementTypeMandatory,
		}},
		CreatedAt: now, UpdatedAt: now,
	}
}

func ClientIdentityAgent(serviceID id.ServiceID, required, optional id.PermissionSetID) *storage.Agent {
	agent := LocalAgent()
	agent.DisplayName = "Reports Reader"
	agent.PermissionSets = []storage.AgentPermissionSetEntry{
		{PermissionSetID: required, RequirementType: storage.RequirementTypeMandatory},
		{PermissionSetID: optional, RequirementType: storage.RequirementTypeOptional},
	}
	agent.ServiceRequirements = []storage.ServiceRequirement{{
		ServiceID: serviceID, RequiredScopes: []string{"profile"}, RequirementType: storage.RequirementTypeMandatory,
	}}
	return agent
}

func ClientIdentityCalendarRequest(providerURL string) map[string]any {
	request := CredentialServiceRequest("calendar-prod", providerURL, "stored")
	request["display_name"] = "Calendar Production"
	request["client_id"] = "calendar-client-v1"
	request["client_secret"] = "synthetic-calendar-secret"
	request["protected_resources"] = []string{providerURL + "/calendar-resource"}
	return request
}
