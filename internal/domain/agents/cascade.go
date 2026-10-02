package agents

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type agentCascade struct {
	grants     []*storage.UserGrant
	approvals  []*storage.ToolApproval
	credential *storage.ClientCredential
}

func (s *Service) loadDependents(ctx context.Context, agentID id.AgentID) (agentCascade, error) {
	grants, err := s.dependents.ListGrantsByAgent(ctx, agentID)
	if err != nil {
		return agentCascade{}, err
	}
	approvals, err := s.dependents.ListApprovalsByAgent(ctx, agentID)
	if err != nil {
		return agentCascade{}, err
	}
	return agentCascade{grants: grants, approvals: approvals}, nil
}

func (cascade agentCascade) subjects() map[id.Principal]bool {
	subjects := make(map[id.Principal]bool, len(cascade.grants)+len(cascade.approvals))
	for _, grant := range cascade.grants {
		subjects[grant.Principal] = true
	}
	for _, approval := range cascade.approvals {
		subjects[approval.Principal] = true
	}
	return subjects
}

func (cascade agentCascade) coveredBy(subjects map[id.Principal]bool) bool {
	for _, grant := range cascade.grants {
		if !subjects[grant.Principal] {
			return false
		}
	}
	for _, approval := range cascade.approvals {
		if !subjects[approval.Principal] {
			return false
		}
	}
	return true
}

func (s *Service) recordCascade(ctx context.Context, agentID id.AgentID, cascade agentCascade) error {
	if err := s.recordAgent(ctx, "agent-deleted", agentID); err != nil {
		return err
	}
	for _, grant := range cascade.grants {
		permissionIDs := make([]id.PermissionSetID, len(grant.GrantedPermissionSets))
		for i, permission := range grant.GrantedPermissionSets {
			permissionIDs[i] = permission.PermissionSetID
		}
		if err := s.recordDependent(ctx, "grant-revoked", model.BusinessEvent{Subject: &grant.Principal, AgentID: agentID, GrantID: grant.ID, PermissionSetIDs: permissionIDs}); err != nil {
			return err
		}
	}
	for _, approval := range cascade.approvals {
		if approval.Status != storage.ApprovalStatusApproved || approval.Persistence == nil || *approval.Persistence != storage.ApprovalPersistencePermanent || approval.Consumed {
			continue
		}
		if err := s.recordDependent(ctx, "approval-revoked", model.BusinessEvent{Subject: &approval.Principal, AgentID: agentID, ApprovalID: approval.ID, GatewayClientID: id.ClientID(approval.GatewayClientID)}); err != nil {
			return err
		}
	}
	if cascade.credential != nil {
		return s.recordDependent(ctx, "credential-revoked", model.BusinessEvent{AgentID: agentID, Data: map[string]any{"credential_id": cascade.credential.ID.String()}})
	}
	return nil
}
