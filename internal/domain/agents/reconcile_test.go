package agents

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func desiredAgent() *storage.Agent {
	return &storage.Agent{DisplayName: "Research", Description: "Definition", PermissionSets: testPermissionSets()}
}

func TestReconcileLifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockAgentRepo()
	repo.listByExternalIDFn = func(_ context.Context, key id.ExternalID) ([]*storage.Agent, error) {
		result := []*storage.Agent{}
		for _, a := range repo.agents {
			if a.ExternalID != nil && *a.ExternalID == key {
				result = append(result, a.Copy())
			}
		}
		return result, nil
	}
	var logs bytes.Buffer
	svc := NewService(repo, &mockServiceReqValidator{}, slog.New(slog.NewJSONHandler(&logs, nil)), false)
	desired := desiredAgent()
	desired.ClientURIs = []string{"https://agent.example/z.json", "https://agent.example/a.json"}
	original := desired.Copy()
	created, outcome, err := svc.ReconcileByExternalID(ctx, "registry", desired, ReconcileInput{})
	require.NoError(t, err)
	require.NotNil(t, created)
	assert.Equal(t, Created, outcome)
	assert.False(t, created.ID.IsZero())
	assert.Equal(t, id.ExternalID("registry"), *created.ExternalID)
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)
	assert.Equal(t, created.CreatedAt.UTC().Truncate(time.Microsecond), created.CreatedAt)
	assert.Equal(t, original, desired)
	require.Contains(t, logs.String(), "agent_created")
	logs.Reset()
	writes := 0
	repo.updateFn = func(_ context.Context, a *storage.Agent) error { writes++; repo.agents[a.ID] = a.Copy(); return nil }
	desired.ClientURIs[0], desired.ClientURIs[1] = desired.ClientURIs[1], desired.ClientURIs[0]
	for range 10 {
		replay, result, err := svc.ReconcileByExternalID(ctx, "registry", desired, ReconcileInput{})
		require.NoError(t, err)
		assert.Equal(t, Unchanged, result)
		assert.Equal(t, created, replay)
	}
	assert.Zero(t, writes)
	assert.Empty(t, logs.String())
	desired.DisplayName = "Changed"
	changed, result, err := svc.ReconcileByExternalID(ctx, "registry", desired, ReconcileInput{})
	require.NoError(t, err)
	require.NotNil(t, changed)
	assert.Equal(t, Updated, result)
	assert.Equal(t, created.ID, changed.ID)
	assert.Equal(t, created.CreatedAt, changed.CreatedAt)
	assert.True(t, changed.UpdatedAt.After(created.UpdatedAt))
	assert.Equal(t, 1, writes)
	assert.Contains(t, logs.String(), "agent_updated")
}

func TestReconcileFieldPolicy(t *testing.T) {
	for _, canonicalPresent := range []bool{false, true} {
		t.Run(map[bool]string{false: "omitted identities", true: "explicit null canonical"}[canonicalPresent], func(t *testing.T) {
			existing := desiredAgent()
			existing.ID = id.NewAgentID()
			existing.ExternalID = ptr.To(id.ExternalID("registry"))
			existing.CanonicalID = ptr.To("research")
			existing.ClientID = ptr.To(id.ClientID("upstream"))
			existing.GovernanceURL = ptr.To("https://example.com/governance")
			existing.UserDocumentationURL = ptr.To("https://example.com/docs")
			existing.AgentInterfaceURL = ptr.To("https://example.com/api")
			existing.RedirectURIs = []string{"https://example.com/callback"}
			existing.AllowedScopes = []string{"openid"}
			existing.CreatedAt = time.Now().Add(-time.Hour)
			existing.UpdatedAt = time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
			repo := newMockAgentRepo()
			repo.agents[existing.ID] = existing.Copy()
			repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
				return []*storage.Agent{existing.Copy()}, nil
			}
			desired := desiredAgent()
			desired.PermissionSets = existing.PermissionSets
			got, result, err := newTestService(repo, false).ReconcileByExternalID(context.Background(), "registry", desired, ReconcileInput{CanonicalIDPresent: canonicalPresent})
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, Updated, result)
			if canonicalPresent {
				assert.Nil(t, got.CanonicalID)
			} else {
				assert.Equal(t, existing.CanonicalID, got.CanonicalID)
			}
			assert.Equal(t, existing.ClientID, got.ClientID)
			assert.Nil(t, got.GovernanceURL)
			assert.Nil(t, got.UserDocumentationURL)
			assert.Nil(t, got.AgentInterfaceURL)
			assert.Empty(t, got.RedirectURIs)
			assert.Empty(t, got.AllowedScopes)
			assert.Equal(t, existing.UpdatedAt.Add(time.Microsecond), got.UpdatedAt)
			assert.Equal(t, existing.CreatedAt, got.CreatedAt)
		})
	}
}

func TestReconcileInvalidInputs(t *testing.T) {
	for _, tc := range []struct {
		name  string
		key   id.ExternalID
		body  *id.ExternalID
		input ReconcileInput
	}{
		{name: "empty", key: ""},
		{name: "long", key: id.ExternalID(strings.Repeat("界", 256))},
		{name: "invalid UTF8", key: "\xff"},
		{name: "NUL", key: "a\x00b"},
		{name: "mismatch", key: "registry", body: ptr.To(id.ExternalID("other")), input: ReconcileInput{ExternalIDPresent: true}},
		{name: "null", key: "registry", input: ReconcileInput{ExternalIDPresent: true, ExternalIDNull: true}},
		{name: "body empty", key: "registry", body: ptr.To(id.ExternalID("")), input: ReconcileInput{ExternalIDPresent: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockAgentRepo()
			repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
				t.Fatal("invalid input reached storage")
				return nil, nil
			}
			desired := desiredAgent()
			desired.ExternalID = tc.body
			_, _, err := newTestService(repo, false).ReconcileByExternalID(context.Background(), tc.key, desired, tc.input)
			require.ErrorIs(t, err, ErrInvalidExternalID)
			assert.Empty(t, repo.agents)
		})
	}
}

func TestReconcileUpdatedAt(t *testing.T) {
	observed := time.Date(2026, 1, 1, 12, 0, 0, 123456789, time.FixedZone("offset", 3600))
	for _, now := range []time.Time{observed.Add(-time.Hour), observed, observed.Add(time.Nanosecond), observed.Add(time.Second)} {
		got := reconcileUpdatedAt(now, observed)
		minimum := observed.UTC().Truncate(time.Microsecond).Add(time.Microsecond)
		assert.False(t, got.Before(minimum))
		assert.Equal(t, got.UTC().Truncate(time.Microsecond), got)
		if now.Before(minimum) {
			assert.Equal(t, minimum, got)
		} else {
			assert.Equal(t, now.UTC().Truncate(time.Microsecond), got)
		}
	}
}

func TestReconcileDefinitionEquality(t *testing.T) {
	base := desiredAgent()
	base.CanonicalID = ptr.To("research")
	base.ClientID = ptr.To(id.ClientID("upstream"))
	base.ExternalID = ptr.To(id.ExternalID("registry"))
	base.ServiceRequirements = []storage.ServiceRequirement{
		{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeOptional, RequiredScopes: []string{"read", "write"}},
		{ServiceID: id.NewServiceID(), RequirementType: storage.RequirementTypeMandatory},
	}
	base.RedirectURIs = []string{"https://example.com/a", "https://example.com/b"}
	base.AllowedScopes = []string{"read", "write"}
	base.PermissionSets = append(base.PermissionSets, storage.AgentPermissionSetEntry{PermissionSetID: id.NewPermissionSetID(), RequirementType: storage.RequirementTypeOptional})
	for _, tc := range []struct {
		name   string
		change func(*storage.Agent)
		equal  bool
	}{
		{"times and UUID", func(a *storage.Agent) {
			a.ID = id.NewAgentID()
			a.CreatedAt = time.Now()
			a.UpdatedAt = time.Now()
			a.ClearCanonicalID = true
		}, true},
		{"nil empty", func(a *storage.Agent) {
			a.ClientURIs = []string{}
			a.ServiceRequirements[1].RequiredScopes = []string{}
		}, true},
		{"service order", func(a *storage.Agent) {
			a.ServiceRequirements[0], a.ServiceRequirements[1] = a.ServiceRequirements[1], a.ServiceRequirements[0]
		}, false},
		{"scope order", func(a *storage.Agent) {
			a.ServiceRequirements[0].RequiredScopes[0], a.ServiceRequirements[0].RequiredScopes[1] = a.ServiceRequirements[0].RequiredScopes[1], a.ServiceRequirements[0].RequiredScopes[0]
		}, false},
		{"redirect order", func(a *storage.Agent) { a.RedirectURIs[0], a.RedirectURIs[1] = a.RedirectURIs[1], a.RedirectURIs[0] }, false},
		{"allowed scope order", func(a *storage.Agent) {
			a.AllowedScopes[0], a.AllowedScopes[1] = a.AllowedScopes[1], a.AllowedScopes[0]
		}, false},
		{"display name", func(a *storage.Agent) { a.DisplayName = "Changed" }, false},
		{"description", func(a *storage.Agent) { a.Description = "Changed" }, false},
		{"canonical", func(a *storage.Agent) { a.CanonicalID = nil }, false},
		{"client", func(a *storage.Agent) { a.ClientID = nil }, false},
		{"external", func(a *storage.Agent) { a.ExternalID = nil }, false},
		{"governance", func(a *storage.Agent) { a.GovernanceURL = ptr.To("https://example.com/gov") }, false},
		{"documentation", func(a *storage.Agent) { a.UserDocumentationURL = ptr.To("https://example.com/docs") }, false},
		{"interface", func(a *storage.Agent) { a.AgentInterfaceURL = ptr.To("https://example.com/api") }, false},
		{"permission order", func(a *storage.Agent) {
			a.PermissionSets[0], a.PermissionSets[1] = a.PermissionSets[1], a.PermissionSets[0]
		}, false},
		{"scopes", func(a *storage.Agent) { a.ServiceRequirements[0].RequiredScopes = []string{"read"} }, false},
		{"require all scopes", func(a *storage.Agent) { a.ServiceRequirements[0].RequireAllScopes = true }, false},
		{"redirects", func(a *storage.Agent) { a.RedirectURIs = []string{"https://example.com/cb"} }, false},
		{"allowed scopes", func(a *storage.Agent) { a.AllowedScopes = []string{"read"} }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := base.Copy()
			tc.change(changed)
			assert.Equal(t, tc.equal, sameDefinition(base, changed))
		})
	}
	base.ClientURIs = []string{"https://example.com/z", "https://example.com/a"}
	changed := base.Copy()
	changed.ClientURIs[0], changed.ClientURIs[1] = changed.ClientURIs[1], changed.ClientURIs[0]
	assert.True(t, sameDefinition(base, changed))
	changed.ClientURIs[0] = "https://example.com/other"
	assert.False(t, sameDefinition(base, changed))
}

func TestReconcileBoundedConflictReread(t *testing.T) {
	for _, source := range []string{"external", "canonical", "client URI"} {
		cause := error(nil)
		if source == "external" {
			cause = ports.ErrExternalIDConflict
		}
		conflict := storage.NewStorageError("Create", storage.ErrorKindConflict, cause, source+" conflict")
		for _, mode := range []string{"holder", "absent", "ambiguous", "lookup error", "update conflict"} {
			t.Run(mode, func(t *testing.T) {
				repo := newMockAgentRepo()
				desired := desiredAgent()
				holder := desired.Copy()
				holder.ID = id.NewAgentID()
				holder.ExternalID = ptr.To(id.ExternalID("registry"))
				if mode == "update conflict" {
					holder.DisplayName = "Old"
				}
				repo.agents[holder.ID] = holder.Copy()
				reads, creates, updates := 0, 0, 0
				lookupErr := errors.New("lookup failed")
				repo.listByExternalIDFn = func(context.Context, id.ExternalID) ([]*storage.Agent, error) {
					reads++
					if reads == 1 {
						return nil, nil
					}
					switch mode {
					case "absent":
						return nil, nil
					case "ambiguous":
						return []*storage.Agent{holder, holder}, nil
					case "lookup error":
						return nil, lookupErr
					}
					return []*storage.Agent{holder}, nil
				}
				repo.createFn = func(context.Context, *storage.Agent) error { creates++; return conflict }
				repo.updateFn = func(context.Context, *storage.Agent) error { updates++; return conflict }
				got, result, err := newTestService(repo, false).ReconcileByExternalID(context.Background(), "registry", desired, ReconcileInput{})
				if mode == "holder" {
					require.NoError(t, err)
					require.NotNil(t, got)
					assert.Equal(t, holder.ID, got.ID)
					assert.Equal(t, Unchanged, result)
				} else {
					require.Error(t, err)
				}
				if mode == "absent" || mode == "update conflict" {
					assert.ErrorIs(t, err, conflict)
				}
				if mode == "lookup error" {
					assert.ErrorIs(t, err, lookupErr)
				}
				assert.Equal(t, 2, reads)
				assert.Equal(t, 1, creates)
				assert.LessOrEqual(t, updates, 1)
			})
		}
	}
}
