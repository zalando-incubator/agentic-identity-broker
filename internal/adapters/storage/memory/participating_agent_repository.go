package memory

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (r *AgentRepository) scopedCreate(ctx context.Context, agent *storage.Agent) error {
	if agent.ID.IsZero() {
		agent.ID = id.NewAgentID()
	}
	if err := agent.ValidateForCreate(); err != nil {
		return err
	}
	return r.scopedPut(ctx, agent, true)
}

func (r *AgentRepository) scopedPut(ctx context.Context, agent *storage.Agent, creating bool) error {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(agent.ID); err != nil {
		return err
	}
	original := scope.agent(agent.ID)
	if creating && original != nil {
		return storage.NewStorageError("CreateAgent", storage.ErrorKindConflict, nil, "agent already exists")
	}
	if !creating && original == nil {
		return storage.NewStorageError("UpdateAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	if !creating {
		if err := agent.Validate(); err != nil {
			return err
		}
	}
	var collision bool
	scope.eachAgent(func(other *storage.Agent) {
		if other.ID == agent.ID {
			return
		}
		if agent.CanonicalID != nil && other.CanonicalID != nil && *agent.CanonicalID == *other.CanonicalID {
			collision = true
		}
		for _, uri := range agent.ClientURIs {
			for _, existing := range other.ClientURIs {
				if uri == existing {
					collision = true
				}
			}
		}
	})
	if collision {
		return storage.NewStorageError("SaveAgent", storage.ErrorKindConflict, nil, "agent canonical ID or client URI already exists")
	}
	if scope.agents == nil {
		scope.agents = make(map[id.AgentID]*storage.Agent)
	}
	scope.touch(agent.ID)
	scope.agents[agent.ID] = agent.Copy()
	return nil
}

func (scope *memoryScope) eachAgent(visit func(*storage.Agent)) {
	for key, original := range scope.store.agents.agents {
		if agent, staged := scope.agents[key]; staged {
			if agent != nil {
				visit(agent)
			}
		} else {
			visit(original)
		}
	}
	for key, agent := range scope.agents {
		if _, existing := scope.store.agents.agents[key]; !existing && agent != nil {
			visit(agent)
		}
	}
}

func (r *AgentRepository) scopedGet(ctx context.Context, agentID id.AgentID) (*storage.Agent, error) {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(agentID); err != nil {
		return nil, err
	}
	agent := scope.agent(agentID)
	if agent == nil {
		return nil, storage.NewStorageError("GetAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	return agent.Copy(), nil
}

func (r *AgentRepository) scopedDelete(ctx context.Context, agentID id.AgentID) error {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(agentID); err != nil {
		return err
	}
	if scope.agent(agentID) == nil {
		return nil
	}
	var roots []*storage.RefreshSession
	var missingReceipt bool
	scope.eachRoot(func(root *storage.RefreshSession) {
		if root.AgentID != agentID {
			return
		}
		roots = append(roots, root)
		if root.TerminalReason == nil {
			missingReceipt = true
			return
		}
		key := receiptKey{session: root.ID, reason: *root.TerminalReason}
		receipt := scope.receipts[key]
		if receipt == nil {
			receipt = r.refresh.receipts[key]
		}
		if receipt == nil || receipt.AgentID != root.AgentID || receipt.Principal != root.Principal || receipt.ClientID != root.ClientID {
			missingReceipt = true
		}
	})
	if missingReceipt {
		return storage.NewStorageError("DeleteAgent", storage.ErrorKindValidation, nil, "refresh sessions need durable revocation receipts before agent deletion")
	}
	if scope.agents == nil {
		scope.agents = make(map[id.AgentID]*storage.Agent)
	}
	if scope.roots == nil {
		scope.roots = make(map[id.RefreshSessionID]*storage.RefreshSession)
	}
	if scope.tokens == nil {
		scope.tokens = make(map[string]*storage.RefreshToken)
	}
	if scope.legacy == nil {
		scope.legacy = make(map[string]*legacyRefreshToken)
	}
	if scope.grants == nil {
		scope.grants = make(map[id.GrantID]*storage.UserGrant)
	}
	if scope.credentials == nil {
		scope.credentials = make(map[id.AgentID]*storage.ClientCredential)
	}
	if scope.codes == nil {
		scope.codes = make(map[id.AuthorizationCodeID]*storage.AuthorizationCode)
	}
	if scope.pkce == nil {
		scope.pkce = make(map[string]*storage.PKCESession)
	}
	scope.touch(agentID)
	scope.agents[agentID] = nil
	rootIDs := make(map[id.RefreshSessionID]bool, len(roots))
	for _, root := range roots {
		rootIDs[root.ID] = true
		scope.roots[root.ID] = nil
	}
	scope.eachToken(func(token *storage.RefreshToken) {
		if rootIDs[token.SessionID] {
			scope.tokens[token.Signature] = nil
		}
	})
	scope.eachLegacy(func(session *legacyRefreshToken) {
		if session.AgentID == agentID {
			scope.legacy[session.Signature] = nil
		}
	})
	scope.eachGrant(func(grant *storage.UserGrant) {
		if grant.AgentID == agentID {
			scope.grants[grant.ID] = nil
		}
	})
	scope.credentials[agentID] = nil
	scope.eachCode(func(code *storage.AuthorizationCode) {
		if code.AgentID == agentID {
			scope.codes[code.ID] = nil
			scope.pkce[code.CodeHash] = nil
		}
	})
	for signature, owner := range r.refresh.pkce.owners {
		if owner == agentID {
			scope.pkce[signature] = nil
		}
	}
	for signature, owner := range scope.pkceOwners {
		if owner == agentID {
			scope.pkce[signature] = nil
		}
	}
	return nil
}

func (r *AgentRepository) scopedList(ctx context.Context) ([]*storage.Agent, error) {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if scope.closed {
		return nil, storage.NewStorageError("ListAgents", storage.ErrorKindValidation, nil, "authorization transaction is closed")
	}
	var list []*storage.Agent
	scope.eachAgent(func(agent *storage.Agent) {
		if scope.agentID.IsZero() || scope.agentID == agent.ID {
			scope.observe(agent.ID)
			list = append(list, agent.Copy())
		}
	})
	return list, nil
}

func (r *AgentRepository) scopedFind(ctx context.Context, matches func(*storage.Agent) bool) (*storage.Agent, error) {
	list, err := r.scopedList(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range list {
		if matches(agent) {
			return agent, nil
		}
	}
	return nil, storage.NewStorageError("FindAgent", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
}

func (r *AgentRepository) scopedByClientURI(ctx context.Context, uri string) (*storage.Agent, error) {
	list, err := r.scopedList(ctx)
	if err != nil {
		return nil, err
	}
	for _, agent := range list {
		for _, registered := range agent.ClientURIs {
			if registered == uri {
				return agent, nil
			}
		}
	}
	var matched *storage.Agent
	for _, agent := range list {
		for _, registered := range agent.ClientURIs {
			if urivalidation.MatchesCIMDClientURI(registered, uri) {
				if matched != nil && matched.ID != agent.ID {
					return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConflict, nil, "multiple client URI matches")
				}
				matched = agent
			}
		}
	}
	if matched == nil {
		return nil, storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	return matched, nil
}
