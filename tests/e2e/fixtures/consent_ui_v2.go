package fixtures

import (
	"fmt"
	"strings"
	"time"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// ConsentV2Clock fixes displayed creation timestamps, independently of server TTLs.
func ConsentV2Clock() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) }

// ConsentV2Future is deliberately far from the wall clock: stored states must not
// change between screenshot runs or while a slow browser worker starts.
func ConsentV2Future() time.Time { return time.Date(2099, 10, 27, 12, 0, 0, 0, time.UTC) }

func ConsentV2Service(name, upstreamURL string) *model.ThirdpartyOAuth2ProviderEntity {
	service := ServiceWithID(id.NewServiceID().String())
	service.DisplayName = name
	service.IssuerURI = upstreamURL
	service.Endpoints.AuthorizeEndpoint = upstreamURL + "/oauth/authorize"
	service.Endpoints.TokenEndpoint = upstreamURL + "/oauth/token"
	service.ProtectedResources = nil
	service.CreatedAt, service.UpdatedAt = ConsentV2Clock(), ConsentV2Clock()
	return service
}

// ConsentV2PermissionSets returns two required purposes followed by two optional
// purposes across two providers. Requirement ordering on the agent is shuffled
// separately, so the acceptance journey detects an unsorted presentation.
func ConsentV2PermissionSets(first, second id.ServiceID) []*storage.PermissionSet {
	names := []string{"Read project files", "Read work calendar", "Edit project files", "Edit work calendar"}
	descriptions := []string{"Read the project files you choose.", "Read your work calendar events.", "Update the project files you choose.", "Update your work calendar events."}
	sets := make([]*storage.PermissionSet, 4)
	for i := range sets {
		service := first
		if i%2 == 1 {
			service = second
		}
		scope := "read"
		if i >= 2 {
			scope = "write"
		}
		sets[i] = &storage.PermissionSet{
			ID: id.NewPermissionSetID(), Name: names[i], Description: descriptions[i],
			ServiceScopes: []storage.ServiceScope{{ServiceID: service, Scopes: []string{scope}, RequirementType: storage.RequirementTypeMandatory}},
		}
	}
	return sets
}

func ConsentV2Agent(name, redirectURI string, sets []*storage.PermissionSet) *storage.Agent {
	agent := AgentWithURLs()
	agent.ClientID = nil // local authorization, including the real PKCE continuation
	agent.DisplayName = name
	agent.Description = "Read and update your project files and work calendar."
	agent.RedirectURIs = []string{redirectURI}
	agent.ServiceRequirements = []storage.ServiceRequirement{
		{ServiceID: sets[0].ServiceScopes[0].ServiceID, RequirementType: storage.RequirementTypeMandatory, RequireAllScopes: true},
		{ServiceID: sets[1].ServiceScopes[0].ServiceID, RequirementType: storage.RequirementTypeMandatory, RequireAllScopes: true},
	}
	agent.PermissionSets = nil
	for _, i := range []int{2, 0, 3, 1} {
		requirement := storage.RequirementTypeOptional
		if i < 2 {
			requirement = storage.RequirementTypeMandatory
		}
		agent.PermissionSets = append(agent.PermissionSets, storage.AgentPermissionSetEntry{PermissionSetID: sets[i].ID, RequirementType: requirement})
	}
	agent.CreatedAt, agent.UpdatedAt = ConsentV2Clock(), ConsentV2Clock()
	return agent
}

func ConsentV2CIMDAgent(name, clientURI string, sets []*storage.PermissionSet) *storage.Agent {
	agent := ConsentV2Agent(name, "", sets)
	agent.RedirectURIs = nil
	agent.ClientURIs = []string{clientURI}
	return agent
}

// ConsentV2LongNameAgent stays within backend validation. AS-15 supplies the
// longer adversarial display name only at the browser response boundary.
func ConsentV2LongNameAgent(redirectURI string, sets []*storage.PermissionSet) *storage.Agent {
	return ConsentV2Agent("Long-name assistant", redirectURI, sets)
}

func ConsentV2AdversarialDisplayName() string {
	markup := "<script>window.consentV2Injected=true</script>"
	return markup + strings.Repeat("x", 300-len(markup))
}

func ConsentV2GrantedAgents(redirectURI string, sets []*storage.PermissionSet) []*storage.Agent {
	agents := make([]*storage.Agent, 14)
	for i := range agents {
		agents[i] = ConsentV2Agent(fmt.Sprintf("Project assistant %02d", i+1), redirectURI, sets)
	}
	return agents
}

func ConsentV2Grant(principal id.Principal, agent *storage.Agent, sets ...*storage.PermissionSet) *storage.UserGrant {
	grant := IndefiniteGrant(principal.String(), agent.ID.String(), sets[0].ServiceScopes[0].ServiceID.String(), nil)
	grant.GrantedPermissionSets = nil
	for _, set := range sets {
		services := make([]id.ServiceID, len(set.ServiceScopes))
		for i, scope := range set.ServiceScopes {
			services[i] = scope.ServiceID
		}
		grant.GrantedPermissionSets = append(grant.GrantedPermissionSets, storage.GrantedPermissionSetEntry{PermissionSetID: set.ID, IncludedServiceIDs: services})
	}
	grant.CreatedAt, grant.UpdatedAt = ConsentV2Clock(), ConsentV2Clock()
	return grant
}

func ConsentV2DeltaGrant(principal id.Principal, agent *storage.Agent, sets []*storage.PermissionSet) *storage.UserGrant {
	grant := ConsentV2Grant(principal, agent, sets[0], sets[1], sets[2])
	until := ConsentV2Future()
	grant.ValidUntil = &until
	return grant
}

func ConsentV2ExpiredGrant(principal id.Principal, agent *storage.Agent, sets []*storage.PermissionSet) *storage.UserGrant {
	grant := ExpiredGrant(principal.String(), agent.ID.String(), sets[0].ServiceScopes[0].ServiceID.String(), nil)
	grant.GrantedPermissionSets = ConsentV2Grant(principal, agent, sets[0], sets[1]).GrantedPermissionSets
	past := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	grant.ValidUntil = &past
	grant.CreatedAt, grant.UpdatedAt = ConsentV2Clock(), ConsentV2Clock()
	return grant
}

// ConsentV2Session builds usable, expired-refresh, or refreshable stored sessions.
// Providers determine whether the real refresh exchange succeeds or returns 502.
func ConsentV2Session(principal id.Principal, service id.ServiceID, state string) *storage.UserSession {
	session := SessionForService(principal.String(), service.String())
	future, past := ConsentV2Future(), time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt = &future, &future
	switch state {
	case "expired":
		session.AccessTokenExpiresAt, session.RefreshTokenExpiresAt = &past, &past
	case "refreshable":
		session.AccessTokenExpiresAt = &past
	}
	session.CreatedAt, session.InitiatedAt, session.UpdatedAt = ConsentV2Clock(), ConsentV2Clock(), ConsentV2Clock()
	return session
}

// ConsentV2Approval uses the production exact-pattern builder, not a test scope.
func ConsentV2Approval(principal id.Principal, agent id.AgentID, tool, risk string) *storage.ToolApproval {
	sessionID := "consent-v2-agent-session"
	approval := &storage.ToolApproval{
		ID: id.NewApprovalID(), Principal: principal, AgentID: agent,
		GatewayClientID: "consent-v2-gateway", ToolName: tool,
		Arguments:   map[string]any{"path": "/projects/roadmap", "recursive": true},
		Description: "Read the project roadmap files.", RiskLevel: risk,
		AgentSessionID: &sessionID, Status: storage.ApprovalStatusPending,
		CreatedAt: ConsentV2Clock(), ExpiresAt: ConsentV2Future(),
	}
	approval.ArgumentsHash = storage.ComputeArgumentsHash(approval.Arguments)
	approval.ApprovalURL = "http://localhost/approvals/" + approval.ID.String()
	if err := domainapproval.ApplyExactPatterns(approval); err != nil {
		panic(err)
	}
	return approval
}

func ConsentV2ResolvedApproval(principal id.Principal, agent id.AgentID, tool string, allow bool) *storage.ToolApproval {
	approval := ConsentV2Approval(principal, agent, tool, "medium")
	persistence := storage.ApprovalPersistencePermanent
	var err error
	if allow {
		err = approval.Approve(principal, storage.ApprovalDecision{Persistence: persistence, ToolPattern: approval.ToolPattern, ParamsPattern: approval.ParamsPattern}, ConsentV2Clock())
	} else {
		err = approval.Deny(principal, &persistence, ConsentV2Clock())
	}
	if err != nil {
		panic(err)
	}
	return approval
}

func ConsentV2ExpiredApproval(principal id.Principal, agent id.AgentID) *storage.ToolApproval {
	approval := ConsentV2Approval(principal, agent, "expired_roadmap_call", "")
	approval.ExpiresAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	return approval
}
