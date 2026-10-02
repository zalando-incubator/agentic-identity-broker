package oauth2server

import (
	"fmt"

	"github.com/ory/fosite"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var _ fosite.Client = (*confidentialClient)(nil)
var _ fosite.Client = (*publicClient)(nil)

// confidentialClient wraps an agent that authenticates with a client secret.
type confidentialClient struct {
	clientID   string
	agent      *storage.Agent
	credential *storage.ClientCredential
}

func (c *confidentialClient) GetID() string             { return c.clientID }
func (c *confidentialClient) GetHashedSecret() []byte   { return []byte(c.credential.SecretHash) }
func (c *confidentialClient) GetRedirectURIs() []string { return c.agent.RedirectURIs }
func (c *confidentialClient) GetGrantTypes() fosite.Arguments {
	return fosite.Arguments{"authorization_code", "client_credentials", "refresh_token"}
}
func (c *confidentialClient) GetResponseTypes() fosite.Arguments { return fosite.Arguments{"code"} }
func (c *confidentialClient) GetScopes() fosite.Arguments {
	if len(c.agent.AllowedScopes) > 0 {
		return fosite.Arguments(c.agent.AllowedScopes)
	}
	return fosite.Arguments{}
}
func (c *confidentialClient) IsPublic() bool                { return false }
func (c *confidentialClient) GetAudience() fosite.Arguments { return fosite.Arguments{} }

// publicClient wraps a CIMD agent that uses no client secret (public client).
// redirectURIs comes from the CIMD document, not from agent.RedirectURIs (which is empty for CIMD agents).
type publicClient struct {
	clientID     string
	agent        *storage.Agent
	redirectURIs []string
	grantTypes   fosite.Arguments
}

func (c *publicClient) GetID() string             { return c.clientID }
func (c *publicClient) GetHashedSecret() []byte   { return nil }
func (c *publicClient) GetRedirectURIs() []string { return c.redirectURIs }
func (c *publicClient) GetGrantTypes() fosite.Arguments {
	return c.grantTypes
}
func (c *publicClient) GetResponseTypes() fosite.Arguments { return fosite.Arguments{"code"} }
func (c *publicClient) GetScopes() fosite.Arguments {
	if len(c.agent.AllowedScopes) > 0 {
		return fosite.Arguments(c.agent.AllowedScopes)
	}
	return fosite.Arguments{}
}
func (c *publicClient) IsPublic() bool                { return true }
func (c *publicClient) GetAudience() fosite.Arguments { return fosite.Arguments{} }

// agentHolder provides access to the underlying Agent for internal use.
type agentHolder interface {
	getAgent() *storage.Agent
}

func (c *confidentialClient) getAgent() *storage.Agent { return c.agent }
func (c *publicClient) getAgent() *storage.Agent       { return c.agent }

// Fosite validates the original grant before narrowing. Current permissions
// apply to the candidate response in the provider, not to every ceiling scope.
type refreshProtocolClient struct {
	fosite.Client
	agent        *storage.Agent
	scopeCeiling fosite.Arguments
}

func (c *refreshProtocolClient) GetScopes() fosite.Arguments { return c.scopeCeiling }
func (c *refreshProtocolClient) getAgent() *storage.Agent    { return c.agent }

func publicClientGrantTypes(cimd []string) fosite.Arguments {
	if len(cimd) == 0 {
		return fosite.Arguments{"authorization_code"}
	}
	return fosite.Arguments(cimd)
}

func extractAgentID(client fosite.Client) (id.AgentID, error) {
	h, ok := client.(agentHolder)
	if !ok {
		return id.AgentID{}, fmt.Errorf("expected broker client, got %T", client)
	}
	agent := h.getAgent()
	if agent == nil || agent.ID.IsZero() {
		return id.AgentID{}, fmt.Errorf("broker client has no valid agent ID")
	}
	return agent.ID, nil
}
