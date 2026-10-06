package storage

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUserGrant_Validate(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	past := time.Now().Add(-24 * time.Hour)
	psID := id.NewPermissionSetID()
	svcID := id.NewServiceID()

	tests := []struct {
		name    string
		grant   *UserGrant
		wantErr string
	}{
		{
			name: "valid grant with expiration",
			grant: &UserGrant{
				ID:         id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal:  id.Principal("user123@example.com"),
				AgentID:    id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ValidUntil: &future,
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "",
		},
		{
			name: "valid grant without expiration (indefinite)",
			grant: &UserGrant{
				ID:         id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal:  id.Principal("user123@example.com"),
				AgentID:    id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ValidUntil: nil,
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "",
		},
		{
			name: "multiple permission sets with multiple services",
			grant: &UserGrant{
				ID:        id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal: id.Principal("user123@example.com"),
				AgentID:   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID(), id.NewServiceID()}},
					{PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()}},
				},
			},
			wantErr: "",
		},
		{
			name: "missing ID",
			grant: &UserGrant{
				Principal: id.Principal("user123@example.com"),
				AgentID:   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "grant ID cannot be empty",
		},
		{
			name: "missing principal",
			grant: &UserGrant{
				ID:      id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				AgentID: id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "principal is required",
		},
		{
			name: "missing agent_id",
			grant: &UserGrant{
				ID:        id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal: id.Principal("user123@example.com"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "agent_id is required",
		},
		{
			name: "valid_until in the past",
			grant: &UserGrant{
				ID:         id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal:  id.Principal("user123@example.com"),
				AgentID:    id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ValidUntil: &past,
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "valid_until must be in the future",
		},
		{
			name: "no permission sets (allowed - agent may have only optional requirements)",
			grant: &UserGrant{
				ID:                    id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal:             id.Principal("user123@example.com"),
				AgentID:               id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{},
			},
			wantErr: "",
		},
		{
			name: "entry with empty included_service_ids",
			grant: &UserGrant{
				ID:        id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
				Principal: id.Principal("user123@example.com"),
				AgentID:   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{}},
				},
			},
			wantErr: "at least one included_service_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.grant.Validate(time.Now())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestUserGrant_IsActive(t *testing.T) {
	expiresAt := time.Date(2001, time.January, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		validUntil   *time.Time
		decisionTime time.Time
		active       bool
	}{
		{name: "finite before expiry", validUntil: &expiresAt, decisionTime: expiresAt.Add(-time.Nanosecond), active: true},
		{name: "finite at expiry", validUntil: &expiresAt, decisionTime: expiresAt, active: false},
		{name: "finite after expiry", validUntil: &expiresAt, decisionTime: expiresAt.Add(time.Nanosecond), active: false},
		{name: "finite historical decision is independent of wall clock", validUntil: &expiresAt, decisionTime: time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC), active: true},
		{name: "indefinite at distant future decision", decisionTime: time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC), active: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			grant := &UserGrant{ValidUntil: tt.validUntil}
			assert.Equal(t, tt.active, grant.IsActive(tt.decisionTime))
		})
	}
}

func TestUserGrant_Copy(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	ps1 := id.NewPermissionSetID()
	ps2 := id.NewPermissionSetID()
	svc1 := id.NewServiceID()
	svc2 := id.NewServiceID()

	original := &UserGrant{
		ID:         id.MustParseGrantID("750e8400-e29b-41d4-a716-446655440002"),
		Principal:  id.Principal("user123@example.com"),
		AgentID:    id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
		ValidUntil: &future,
		GrantedPermissionSets: []GrantedPermissionSetEntry{
			{PermissionSetID: ps1, IncludedServiceIDs: []id.ServiceID{svc1}},
			{PermissionSetID: ps2, IncludedServiceIDs: []id.ServiceID{svc1, svc2}},
		},
	}

	cp := original.Copy()

	// Verify equality
	assert.Equal(t, original.ID, cp.ID)
	assert.Equal(t, original.Principal, cp.Principal)
	assert.Equal(t, original.AgentID, cp.AgentID)
	assert.Equal(t, len(original.GrantedPermissionSets), len(cp.GrantedPermissionSets))

	// Verify deep copy of ValidUntil
	*cp.ValidUntil = time.Now().Add(48 * time.Hour)
	assert.NotEqual(t, original.ValidUntil.Unix(), cp.ValidUntil.Unix())

	// Verify slice independence
	assert.Equal(t, original.GrantedPermissionSets[0].PermissionSetID, cp.GrantedPermissionSets[0].PermissionSetID)
	assert.Equal(t, original.GrantedPermissionSets[1].IncludedServiceIDs, cp.GrantedPermissionSets[1].IncludedServiceIDs)
}

func TestUserGrant_Copy_Nil(t *testing.T) {
	var grant *UserGrant
	cp := grant.Copy()
	assert.Nil(t, cp)
}

func TestUserGrant_ValidateForCreate(t *testing.T) {
	future := time.Now().Add(24 * time.Hour)
	psID := id.NewPermissionSetID()
	svcID := id.NewServiceID()

	tests := []struct {
		name    string
		grant   *UserGrant
		wantErr string
	}{
		{
			name: "valid grant for creation",
			grant: &UserGrant{
				Principal:  id.Principal("user123@example.com"),
				AgentID:    id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ValidUntil: &future,
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "",
		},
		{
			name: "missing principal",
			grant: &UserGrant{
				AgentID: id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				GrantedPermissionSets: []GrantedPermissionSetEntry{
					{PermissionSetID: psID, IncludedServiceIDs: []id.ServiceID{svcID}},
				},
			},
			wantErr: "principal is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.grant.ValidateForCreate(time.Now())
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
