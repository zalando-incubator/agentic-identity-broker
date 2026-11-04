package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcileSuppliedFieldsOnBothBranches(t *testing.T) {
	for _, exists := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			set  func(*storage.Agent)
		}{
			{"identities", func(a *storage.Agent) {
				a.CanonicalID = ptr.To("replacement")
				a.ClientID = ptr.To(id.ClientID("replacement"))
			}},
			{"metadata", func(a *storage.Agent) {
				a.GovernanceURL = ptr.To("https://example.com/gov")
				a.UserDocumentationURL = ptr.To("https://example.com/docs")
				a.AgentInterfaceURL = ptr.To("https://example.com/api")
			}},
			{"collections", func(a *storage.Agent) {
				a.ClientURIs = []string{"https://example.com/z.json", "https://example.com/a.json"}
				a.RedirectURIs = []string{"https://example.com/cb"}
				a.AllowedScopes = []string{"read"}
				a.ServiceRequirements = []storage.ServiceRequirement{{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional}}
			}},
		} {
			name := "create/"
			if exists {
				name = "update/"
			}
			t.Run(name+tc.name, func(t *testing.T) {
				repo := newMockAgentRepo()
				existing := desiredAgent()
				existing.ID = id.NewAgentID()
				existing.ExternalID = ptr.To(id.ExternalID("registry"))
				existing.CanonicalID = ptr.To("original")
				existing.ClientID = ptr.To(id.ClientID("original"))
				if tc.name == "collections" {
					existing.ClientID = nil
				}
				existing.CreatedAt = time.Now().Add(-time.Hour)
				if exists {
					repo.agents[existing.ID] = existing.Copy()
				}
				repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
					if exists {
						return []*storage.Agent{existing.Copy()}, nil
					}
					return nil, nil
				}
				desired := desiredAgent()
				tc.set(desired)
				before := desired.Copy()
				svc := newTestService(repo, false)
				svc.permissionSets = coveredPermissionSets(desired.ServiceRequirements)
				got, outcome, err := svc.ReconcileByExternalID(context.Background(), "registry", desired, ReconcileInput{CanonicalIDPresent: true})
				require.NoError(t, err)
				require.NotNil(t, got)
				assert.Equal(t, before, desired)
				assert.Equal(t, desired.CanonicalID, got.CanonicalID)
				if desired.ClientID != nil || !exists {
					assert.Equal(t, desired.ClientID, got.ClientID)
				} else {
					assert.Equal(t, existing.ClientID, got.ClientID)
				}
				assert.Equal(t, desired.GovernanceURL, got.GovernanceURL)
				assert.Equal(t, desired.UserDocumentationURL, got.UserDocumentationURL)
				assert.Equal(t, desired.AgentInterfaceURL, got.AgentInterfaceURL)
				assert.Equal(t, desired.ServiceRequirements, got.ServiceRequirements)
				assert.Equal(t, desired.PermissionSets, got.PermissionSets)
				assert.Equal(t, desired.RedirectURIs, got.RedirectURIs)
				assert.Equal(t, desired.AllowedScopes, got.AllowedScopes)
				assert.Equal(t, desired.ClientURIs, got.ClientURIs)
				if exists {
					assert.Equal(t, ReconcileUpdated, outcome)
					assert.Equal(t, existing.ID, got.ID)
					assert.Equal(t, existing.CreatedAt, got.CreatedAt)
				} else {
					assert.Equal(t, ReconcileCreated, outcome)
					assert.NotEqual(t, existing.ID, got.ID)
				}
			})
		}
	}
}

func TestReconcileLiteralSelectorBoundaries(t *testing.T) {
	for _, key := range []id.ExternalID{" ", "Registry", "registry", " Registry", id.ExternalID(id.NewAgentID().String()), id.ExternalID(strings.Repeat("界", 255))} {
		t.Run(string(key), func(t *testing.T) {
			repo := newMockAgentRepo()
			reads := 0
			repo.listByExternalIDFn = func(_ context.Context, got id.ExternalID) ([]*storage.Agent, error) {
				assert.Equal(t, key, got)
				reads++
				return nil, nil
			}
			desired := desiredAgent()
			desired.ExternalID = ptr.To(key)
			got, outcome, err := newTestService(repo, false).ReconcileByExternalID(context.Background(), key, desired, ReconcileInput{ExternalIDPresent: true})
			require.NoError(t, err)
			assert.Equal(t, ReconcileCreated, outcome)
			assert.Equal(t, key, *got.ExternalID)
			assert.Equal(t, 1, reads)
		})
	}
}

func TestReconcileInitialAmbiguityAndLookupFailure(t *testing.T) {
	for _, ambiguous := range []bool{false, true} {
		repo := newMockAgentRepo()
		repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
			if ambiguous {
				return []*storage.Agent{desiredAgent(), desiredAgent()}, nil
			}
			return nil, storage.NewStorageError("lookup", storage.ErrorKindConnection, nil, "unavailable")
		}
		repo.createFn = func(context.Context, *storage.Agent) error { t.Fatal("unexpected create"); return nil }
		repo.updateFn = func(context.Context, *storage.Agent) error { t.Fatal("unexpected update"); return nil }
		_, _, err := newTestService(repo, false).ReconcileByExternalID(context.Background(), "registry", desiredAgent(), ReconcileInput{})
		require.Error(t, err)
		var se *storage.StorageError
		require.ErrorAs(t, err, &se)
		if ambiguous {
			assert.Equal(t, storage.ErrorKindUnknown, se.Kind)
		} else {
			assert.Equal(t, storage.ErrorKindConnection, se.Kind)
		}
	}
}
