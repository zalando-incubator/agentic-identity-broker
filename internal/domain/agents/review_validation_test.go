package agents

import (
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/require"
)

func TestWriteExternalIDFormat(t *testing.T) {
	for _, value := range []string{"a\x00b", "\xff", strings.Repeat("界", 256)} {
		for _, operation := range []string{"create", "update"} {
			t.Run(operation+value, func(t *testing.T) {
				repo := newMockAgentRepo()
				existing := desiredAgent()
				existing.ID = id.NewAgentID()
				repo.agents[existing.ID] = existing.Copy()
				candidate := existing.Copy()
				candidate.ExternalID = ptr.To(id.ExternalID(value))
				svc := newTestService(repo, true)
				var err error
				if operation == "create" {
					err = svc.Create(context.Background(), candidate)
				} else {
					err = svc.Update(context.Background(), existing.ID, candidate, UpdateInput{})
				}
				require.Error(t, err)
				require.Equal(t, existing, repo.agents[existing.ID])
			})
		}
	}
}

func TestAgentExternalIDUnicodeBoundary(t *testing.T) {
	candidate := desiredAgent()
	candidate.ExternalID = ptr.To(id.ExternalID(strings.Repeat("界", 255)))
	require.NoError(t, candidate.ValidateForCreate())
	for _, value := range []string{"a\x00b", "\xff", strings.Repeat("界", 256)} {
		candidate.ExternalID = ptr.To(id.ExternalID(value))
		require.Error(t, candidate.ValidateForCreate())
	}
}

func TestWritesEnforceServiceCoverage(t *testing.T) {
	for _, operation := range []string{"create", "update", "reconcile create", "reconcile update", "reconcile replay"} {
		t.Run(operation, func(t *testing.T) {
			repo := newMockAgentRepo()
			existing := desiredAgent()
			existing.ID = id.NewAgentID()
			existing.ExternalID = ptr.To(id.ExternalID("registry"))
			existing.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional}}
			repo.agents[existing.ID] = existing.Copy()
			repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
				if operation == "reconcile create" {
					return []*storage.Agent{}, nil
				}
				return []*storage.Agent{existing.Copy()}, nil
			}
			candidate := existing.Copy()
			if operation != "reconcile replay" {
				candidate.DisplayName = "Changed"
			}
			svc := newTestService(repo, true)
			svc.permissionSets = reviewPermissionSets{}
			var err error
			switch operation {
			case "create":
				err = svc.Create(context.Background(), candidate)
			case "update":
				err = svc.Update(context.Background(), existing.ID, candidate, UpdateInput{})
			default:
				_, _, err = svc.ReconcileByExternalID(context.Background(), "registry", candidate, ReconcileInput{})
			}
			require.ErrorContains(t, err, "not covered by any permission set")
			require.Equal(t, existing, repo.agents[existing.ID])
		})
	}
}

func TestReconcileDoesNotRereadUnrelatedConflict(t *testing.T) {
	repo := newMockAgentRepo()
	reads := 0
	repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) { reads++; return nil, nil }
	conflict := storage.NewStorageError("Create", storage.ErrorKindConflict, nil, "canonical conflict")
	repo.createFn = func(context.Context, *storage.Agent) error { return conflict }
	_, _, err := newTestService(repo, true).ReconcileByExternalID(context.Background(), "registry", desiredAgent(), ReconcileInput{})
	require.ErrorIs(t, err, conflict)
	require.Equal(t, 1, reads)
}

type coveredPermissionSets []storage.ServiceRequirement

func (reqs coveredPermissionSets) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	ps := &storage.PermissionSet{}
	for _, req := range reqs {
		ps.ServiceScopes = append(ps.ServiceScopes, storage.ServiceScope{ServiceID: req.ServiceID})
	}
	return []*storage.PermissionSet{ps}, nil
}

type countingRequirementValidator struct{ calls int }

func (v *countingRequirementValidator) ValidateServiceRequirements(context.Context, []storage.ServiceRequirement) error {
	v.calls++
	return nil
}

func TestUpdateCandidateValidationRunsOnce(t *testing.T) {
	for _, operation := range []string{"update", "reconcile update", "reconcile replay"} {
		t.Run(operation, func(t *testing.T) {
			repo := newMockAgentRepo()
			existing := desiredAgent()
			existing.ID = id.NewAgentID()
			existing.ExternalID = ptr.To(id.ExternalID("registry"))
			existing.CanonicalID = ptr.To("research")
			existing.ClientID = ptr.To(id.ClientID("upstream"))
			repo.agents[existing.ID] = existing.Copy()
			repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
				return []*storage.Agent{existing.Copy()}, nil
			}
			uniquenessChecks := 0
			repo.existsOtherFn = func(context.Context, id.ClientID, *id.AgentID) (bool, error) { uniquenessChecks++; return false, nil }
			validation := &countingRequirementValidator{}
			svc := NewService(repo, validation, slog.Default(), false, reviewPermissionSets{})
			candidate := existing.Copy()
			candidate.ClientID = nil
			candidate.CanonicalID = nil
			if operation != "reconcile replay" {
				candidate.DisplayName = "Changed"
			}
			if operation == "update" {
				require.NoError(t, svc.Update(context.Background(), existing.ID, candidate, UpdateInput{}))
				require.Equal(t, existing.CanonicalID, candidate.CanonicalID)
				require.Equal(t, existing.ClientID, candidate.ClientID)
			} else {
				got, _, err := svc.ReconcileByExternalID(context.Background(), "registry", candidate, ReconcileInput{})
				require.NoError(t, err)
				require.Equal(t, existing.CanonicalID, got.CanonicalID)
				require.Equal(t, existing.ClientID, got.ClientID)
			}
			require.Equal(t, 1, validation.calls)
			require.Equal(t, 1, uniquenessChecks)
		})
	}
}

type reviewPermissionSets struct{}

func (reviewPermissionSets) GetByIDs(context.Context, []id.PermissionSetID) ([]*storage.PermissionSet, error) {
	return []*storage.PermissionSet{}, nil
}
