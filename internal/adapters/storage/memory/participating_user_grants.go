package memory

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func (r *UserGrantRepository) scopedCreate(ctx context.Context, grant *storage.UserGrant) error {
	if grant.ID.IsZero() {
		grant.ID = id.NewGrantID()
	}
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(grant.AgentID); err != nil {
		return err
	}
	existing := scope.grantByPrincipalAgent(grant.Principal, grant.AgentID)
	if existing == nil {
		if sameID := scope.grant(grant.ID); sameID != nil && (sameID.AgentID != grant.AgentID || sameID.Principal != grant.Principal) {
			return storage.NewStorageError("CreateUserGrant", storage.ErrorKindConflict, nil, "grant ID belongs to another delegation")
		}
	}
	changed := grant.Copy()
	if existing != nil {
		changed = existing.Copy()
		replacement := grant.Copy()
		changed.ValidUntil = replacement.ValidUntil
		changed.GrantedPermissionSets = replacement.GrantedPermissionSets
		changed.UpdatedAt = grant.UpdatedAt
		grant.ID = existing.ID
	}
	if scope.grants == nil {
		scope.grants = make(map[id.GrantID]*storage.UserGrant)
	}
	scope.touch(grant.AgentID)
	scope.grants[grant.ID] = changed
	return nil
}

func (r *UserGrantRepository) scopedGet(ctx context.Context, grantID id.GrantID) (*storage.UserGrant, error) {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	grant := scope.grant(grantID)
	if grant == nil {
		return nil, storage.NewStorageError("GetUserGrant", storage.ErrorKindNotFound, ports.ErrNotFound, "user grant not found")
	}
	if err := scope.checkAgent(grant.AgentID); err != nil {
		return nil, err
	}
	return grant.Copy(), nil
}

func (r *UserGrantRepository) scopedUpdate(ctx context.Context, grant *storage.UserGrant) error {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(grant.AgentID); err != nil {
		return err
	}
	previous := scope.grant(grant.ID)
	if previous == nil {
		return storage.NewStorageError("UpdateUserGrant", storage.ErrorKindNotFound, ports.ErrNotFound, "user grant not found")
	}
	if previous.AgentID != grant.AgentID || previous.Principal != grant.Principal {
		return storage.NewStorageError("UpdateUserGrant", storage.ErrorKindValidation, nil, "grant ownership cannot change")
	}
	if scope.grants == nil {
		scope.grants = make(map[id.GrantID]*storage.UserGrant)
	}
	scope.touch(grant.AgentID)
	scope.grants[grant.ID] = grant.Copy()
	return nil
}

func (r *UserGrantRepository) scopedDelete(ctx context.Context, grantID id.GrantID) error {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	grant := scope.grant(grantID)
	if grant == nil {
		return nil
	}
	if err := scope.checkAgent(grant.AgentID); err != nil {
		return err
	}
	if scope.grants == nil {
		scope.grants = make(map[id.GrantID]*storage.UserGrant)
	}
	scope.touch(grant.AgentID)
	scope.grants[grantID] = nil
	return nil
}

func (r *UserGrantRepository) scopedList(ctx context.Context, filter func(*storage.UserGrant) bool) ([]*storage.UserGrant, error) {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if scope.closed {
		return nil, storage.NewStorageError("ListUserGrants", storage.ErrorKindValidation, nil, "authorization transaction is closed")
	}
	var grants []*storage.UserGrant
	scope.eachGrant(func(grant *storage.UserGrant) {
		if (scope.agentID.IsZero() || scope.agentID == grant.AgentID) && filter(grant) {
			scope.observe(grant.AgentID)
			grants = append(grants, grant.Copy())
		}
	})
	return grants, nil
}

func (r *UserGrantRepository) scopedByAgent(ctx context.Context, agentID id.AgentID) error {
	scope := scopeFor(ctx, r.refresh)
	if err := scope.checkAgent(agentID); err != nil {
		return err
	}
	grants, err := r.scopedList(ctx, func(grant *storage.UserGrant) bool { return grant.AgentID == agentID })
	if err != nil {
		return err
	}
	for _, grant := range grants {
		if err := r.scopedDelete(ctx, grant.ID); err != nil {
			return err
		}
	}
	return nil
}

func (scope *memoryScope) grantByPrincipalAgent(principal id.Principal, agentID id.AgentID) *storage.UserGrant {
	for _, grant := range scope.grants {
		if grant != nil && grant.Principal == principal && grant.AgentID == agentID {
			return grant
		}
	}
	if committedID, found := scope.store.grants.byPrincipalAndAgent[principalAgentKey(principal, agentID)]; found {
		return scope.grant(committedID)
	}
	return nil
}

func (r *UserGrantRepository) scopedByPrincipalAgent(ctx context.Context, principal id.Principal, agentID id.AgentID) (*storage.UserGrant, error) {
	scope := scopeFor(ctx, r.refresh)
	r.refresh.mu.RLock()
	defer r.refresh.mu.RUnlock()
	if err := scope.checkAgent(agentID); err != nil {
		return nil, err
	}
	if grant := scope.grantByPrincipalAgent(principal, agentID); grant != nil {
		return grant.Copy(), nil
	}
	return nil, storage.NewStorageError("FindUserGrant", storage.ErrorKindNotFound, ports.ErrNotFound, "user grant not found")
}

func (r *UserGrantRepository) scopedCountPermissionSet(ctx context.Context, psID id.PermissionSetID, decisionTime time.Time) (int, error) {
	found, err := r.scopedList(ctx, func(grant *storage.UserGrant) bool {
		if !grant.IsActive(decisionTime) {
			return false
		}
		for _, entry := range grant.GrantedPermissionSets {
			if entry.PermissionSetID == psID {
				return true
			}
		}
		return false
	})
	return len(found), err
}
