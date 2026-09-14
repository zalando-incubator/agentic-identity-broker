package storage

import (
	"strings"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgent_Validate(t *testing.T) {
	validGovernanceURL := "https://governance.example.com"
	validUserDocURL := "https://docs.example.com"
	validAgentURL := "https://chat.example.com"

	tests := []struct {
		name    string
		agent   *Agent
		wantErr string
	}{
		{
			name: "valid agent with all fields",
			agent: &Agent{
				ID:                   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:             ptr.To(id.ClientID("test-client")),
				DisplayName:          "Test Agent",
				Description:          "A test agent for validation",
				GovernanceURL:        &validGovernanceURL,
				UserDocumentationURL: &validUserDocURL,
				AgentInterfaceURL:    &validAgentURL,
				PermissionSets: []AgentPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(),
					RequirementType: RequirementTypeOptional,
				}},
			},
			wantErr: "",
		},
		{
			name: "valid agent with minimal fields",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: "Test Agent",
				Description: "A test agent",
				PermissionSets: []AgentPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(),
					RequirementType: RequirementTypeOptional,
				}},
			},
			wantErr: "",
		},
		{
			name: "missing ID",
			agent: &Agent{
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: "Test Agent",
				Description: "A test agent",
			},
			wantErr: "agent ID cannot be empty",
		},
		{
			name: "nil client_id is valid",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				DisplayName: "Test Agent",
				Description: "A test agent",
				PermissionSets: []AgentPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(),
					RequirementType: RequirementTypeOptional,
				}},
			},
			wantErr: "",
		},
		{
			name: "missing display_name",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("test-client")),
				Description: "A test agent",
			},
			wantErr: "display_name is required",
		},
		{
			name: "display_name too long",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: strings.Repeat("a", 256),
				Description: "A test agent",
			},
			wantErr: "display_name exceeds 255 characters",
		},
		{
			name: "missing description",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: "Test Agent",
			},
			wantErr: "description is required",
		},
		{
			name: "description too long",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: "Test Agent",
				Description: strings.Repeat("a", 1001),
			},
			wantErr: "description exceeds 1000 characters",
		},
		{
			name: "invalid governance_url",
			agent: &Agent{
				ID:            id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:      ptr.To(id.ClientID("test-client")),
				DisplayName:   "Test Agent",
				Description:   "A test agent",
				GovernanceURL: stringPtr("not-a-url"),
			},
			wantErr: "governance_url is not a valid HTTP/HTTPS URL",
		},
		{
			name: "invalid user_documentation_url",
			agent: &Agent{
				ID:                   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:             ptr.To(id.ClientID("test-client")),
				DisplayName:          "Test Agent",
				Description:          "A test agent",
				UserDocumentationURL: stringPtr("ftp://invalid.com"),
			},
			wantErr: "user_documentation_url is not a valid HTTP/HTTPS URL",
		},
		{
			name: "invalid agent_interface_url",
			agent: &Agent{
				ID:                id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:          ptr.To(id.ClientID("test-client")),
				DisplayName:       "Test Agent",
				Description:       "A test agent",
				AgentInterfaceURL: stringPtr("javascript:alert(1)"),
			},
			wantErr: "agent_interface_url is not a valid HTTP/HTTPS URL",
		},
		{
			name: "client_id and client_uris both set is invalid",
			agent: &Agent{
				ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
				ClientID:    ptr.To(id.ClientID("upstream-id")),
				ClientURIs:  []string{"https://example.com/.well-known/agent"},
				DisplayName: "Ambiguous Agent",
				Description: "A test agent",
			},
			wantErr: "agent cannot have both client_id and client_uris set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgent_ValidatePermissionSets_RejectsEmpty(t *testing.T) {
	err := (&Agent{}).ValidatePermissionSets()
	require.Error(t, err)
	assert.EqualError(t, err, "at least one permission set entry is required")
}

func TestAgent_ValidateForCreate(t *testing.T) {
	validGovernanceURL := "https://governance.example.com"

	tests := []struct {
		name    string
		agent   *Agent
		wantErr string
	}{
		{
			name: "valid agent for creation",
			agent: &Agent{
				ClientID:      ptr.To(id.ClientID("test-client")),
				DisplayName:   "Test Agent",
				Description:   "A test agent",
				GovernanceURL: &validGovernanceURL,
				PermissionSets: []AgentPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(),
					RequirementType: RequirementTypeOptional,
				}},
			},
			wantErr: "",
		},
		{
			name: "nil client_id is valid for create",
			agent: &Agent{
				DisplayName: "Test Agent",
				Description: "A test agent",
				PermissionSets: []AgentPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(),
					RequirementType: RequirementTypeOptional,
				}},
			},
			wantErr: "",
		},
		{
			name: "display_name exceeds limit with character count",
			agent: &Agent{
				ClientID:    ptr.To(id.ClientID("test-client")),
				DisplayName: strings.Repeat("a", 256),
				Description: "A test agent",
			},
			wantErr: "display_name exceeds 255 characters",
		},
		{
			name: "client_id and client_uris both set is invalid",
			agent: &Agent{
				ClientID:    ptr.To(id.ClientID("upstream-id")),
				ClientURIs:  []string{"https://example.com/.well-known/agent"},
				DisplayName: "Ambiguous Agent",
				Description: "A test agent",
			},
			wantErr: "agent cannot have both client_id and client_uris set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.ValidateForCreate()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgent_Copy(t *testing.T) {
	externalID := id.ExternalID("ext-123")
	governanceURL := "https://governance.example.com"
	userDocURL := "https://docs.example.com"
	agentURL := "https://chat.example.com"

	original := &Agent{
		ID:                   id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
		ClientID:             ptr.To(id.ClientID("test-client")),
		ExternalID:           &externalID,
		DisplayName:          "Test Agent",
		Description:          "A test agent",
		GovernanceURL:        &governanceURL,
		UserDocumentationURL: &userDocURL,
		AgentInterfaceURL:    &agentURL,
	}

	// Create copy
	copy := original.Copy()

	// Verify copy is equal
	assert.Equal(t, original.ID, copy.ID)
	assert.Equal(t, original.ClientID, copy.ClientID)
	assert.Equal(t, original.DisplayName, copy.DisplayName)
	assert.Equal(t, original.Description, copy.Description)
	require.NotNil(t, copy.ExternalID)
	assert.Equal(t, *original.ExternalID, *copy.ExternalID)
	require.NotNil(t, copy.GovernanceURL)
	assert.Equal(t, *original.GovernanceURL, *copy.GovernanceURL)

	// Verify deep copy (modifying copy doesn't affect original)
	*copy.ExternalID = id.ExternalID("modified")
	assert.Equal(t, id.ExternalID("ext-123"), *original.ExternalID)
	assert.Equal(t, id.ExternalID("modified"), *copy.ExternalID)
}

func TestAgent_Copy_Nil(t *testing.T) {
	var agent *Agent
	copy := agent.Copy()
	assert.Nil(t, copy)
}

func stringPtr(s string) *string {
	return &s
}

func TestAgent_ValidateServiceRequirements(t *testing.T) {
	tests := []struct {
		name    string
		agent   *Agent
		wantErr string
	}{
		{
			name: "valid agent with no service requirements",
			agent: &Agent{
				ServiceRequirements: nil,
			},
			wantErr: "",
		},
		{
			name: "valid agent with empty service requirements array",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{},
			},
			wantErr: "",
		},
		{
			name: "valid agent with single mandatory requirement",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"repo", "user:email"},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid agent with single optional requirement",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeOptional,
						RequiredScopes:  []string{"read:user"},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "valid agent with multiple requirements for different services",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"repo"},
					},
					{
						ServiceID:       id.MustParseServiceID("660e8400-e29b-41d4-a716-446655440001"),
						RequirementType: RequirementTypeOptional,
						RequiredScopes:  []string{"profile"},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "invalid requirement - missing service_id",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"repo"},
					},
				},
			},
			wantErr: "service_requirements[0] invalid: service_id is required",
		},
		{
			name: "invalid requirement - invalid requirement_type",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementType("invalid"),
						RequiredScopes:  []string{"repo"},
					},
				},
			},
			wantErr: "service_requirements[0] invalid: requirement_type validation failed",
		},
		{
			name: "valid requirement with empty required_scopes",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{},
					},
				},
			},
			wantErr: "",
		},
		{
			name: "duplicate service_id in requirements",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"repo"},
					},
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"), // Duplicate
						RequirementType: RequirementTypeOptional,
						RequiredScopes:  []string{"user:email"},
					},
				},
			},
			wantErr: "duplicate service_id \"550e8400-e29b-41d4-a716-446655440000\" found at indices 0 and 1",
		},
		{
			name: "multiple invalid requirements - reports first error",
			agent: &Agent{
				ServiceRequirements: []ServiceRequirement{
					{
						ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"repo"},
					},
					{
						RequirementType: RequirementTypeMandatory,
						RequiredScopes:  []string{"user:email"},
					},
				},
			},
			wantErr: "service_requirements[1] invalid: service_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.ValidateServiceRequirements()
			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestAgent_ValidateServiceRequirements_IntegrationWithValidate(t *testing.T) {
	t.Run("Validate() allows empty required_scopes", func(t *testing.T) {
		agent := &Agent{
			ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			ServiceRequirements: []ServiceRequirement{{
				ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
				RequirementType: RequirementTypeMandatory,
				RequiredScopes:  []string{},
			}},
			PermissionSets: []AgentPermissionSetEntry{{
				PermissionSetID: id.NewPermissionSetID(),
				RequirementType: RequirementTypeOptional,
			}},
		}

		require.NoError(t, agent.Validate())
	})

	t.Run("ValidateForCreate() calls ValidateServiceRequirements()", func(t *testing.T) {
		agent := &Agent{
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			ServiceRequirements: []ServiceRequirement{
				{
					ServiceID:       id.MustParseServiceID("550e8400-e29b-41d4-a716-446655440000"),
					RequirementType: RequirementType("invalid"), // Invalid type
					RequiredScopes:  []string{"repo"},
				},
			},
		}

		err := agent.ValidateForCreate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "service_requirements validation failed")
		assert.Contains(t, err.Error(), "requirement_type validation failed")
	})
}

func TestAgent_Copy_WithServiceRequirements(t *testing.T) {
	t.Run("copies service requirements with deep copy", func(t *testing.T) {
		original := &Agent{
			ID:          id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
			ClientID:    ptr.To(id.ClientID("test-client")),
			DisplayName: "Test Agent",
			Description: "A test agent",
			ServiceRequirements: []ServiceRequirement{
				{
					ServiceID:       id.MustParseServiceID("660e8400-e29b-41d4-a716-446655440001"),
					RequirementType: RequirementTypeMandatory,
					RequiredScopes:  []string{"repo", "user:email"},
				},
				{
					ServiceID:       id.MustParseServiceID("770e8400-e29b-41d4-a716-446655440002"),
					RequirementType: RequirementTypeOptional,
					RequiredScopes:  []string{"read:user"},
				},
			},
		}

		copy := original.Copy()

		// Verify copy is equal
		require.Len(t, copy.ServiceRequirements, 2)
		assert.Equal(t, original.ServiceRequirements[0].ServiceID, copy.ServiceRequirements[0].ServiceID)
		assert.Equal(t, original.ServiceRequirements[0].RequirementType, copy.ServiceRequirements[0].RequirementType)
		assert.Equal(t, original.ServiceRequirements[0].RequiredScopes, copy.ServiceRequirements[0].RequiredScopes)

		// Verify deep copy (modifying copy doesn't affect original)
		copy.ServiceRequirements[0].RequiredScopes[0] = "modified"
		assert.Equal(t, "repo", original.ServiceRequirements[0].RequiredScopes[0])
		assert.Equal(t, "modified", copy.ServiceRequirements[0].RequiredScopes[0])

		// Verify modifying copy array doesn't affect original
		copy.ServiceRequirements = append(copy.ServiceRequirements, ServiceRequirement{
			ServiceID:       id.MustParseServiceID("880e8400-e29b-41d4-a716-446655440003"),
			RequirementType: RequirementTypeMandatory,
			RequiredScopes:  []string{"new"},
		})
		assert.Len(t, original.ServiceRequirements, 2)
		assert.Len(t, copy.ServiceRequirements, 3)
	})

	t.Run("handles nil service requirements", func(t *testing.T) {
		original := &Agent{
			ID:                  id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
			ClientID:            ptr.To(id.ClientID("test-client")),
			DisplayName:         "Test Agent",
			Description:         "A test agent",
			ServiceRequirements: nil,
		}

		copy := original.Copy()
		assert.Nil(t, copy.ServiceRequirements)
	})

	t.Run("handles empty service requirements", func(t *testing.T) {
		original := &Agent{
			ID:                  id.MustParseAgentID("550e8400-e29b-41d4-a716-446655440000"),
			ClientID:            ptr.To(id.ClientID("test-client")),
			DisplayName:         "Test Agent",
			Description:         "A test agent",
			ServiceRequirements: []ServiceRequirement{},
		}

		copy := original.Copy()
		assert.NotNil(t, copy.ServiceRequirements)
		assert.Len(t, copy.ServiceRequirements, 0)
	})
}

func TestValidateClientURIsForWrite(t *testing.T) {
	t.Run("accepts single valid CIMD URI", func(t *testing.T) {
		err := ValidateClientURIsForWrite([]string{"https://example.com/client"})
		require.NoError(t, err)
	})

	t.Run("accepts a valid CIMD URI pattern", func(t *testing.T) {
		err := ValidateClientURIsForWrite([]string{"https://chatgpt.com/oauth/codex/*/client.json"})
		require.NoError(t, err)
	})

	t.Run("rejects trailing colon with empty port", func(t *testing.T) {
		err := ValidateClientURIsForWrite([]string{"https://example.com:/client"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "malformed authority")
	})

	t.Run("rejects empty hostname with port", func(t *testing.T) {
		err := ValidateClientURIsForWrite([]string{"https://:443/client"})
		require.Error(t, err)
	})

}

func TestAgentCanonicalIDValidationAndCopy(t *testing.T) {
	canonicalID := "research-agent"
	agent := &Agent{
		ID:          id.NewAgentID(),
		CanonicalID: &canonicalID,
		DisplayName: "Research",
		Description: "Research agent",
		PermissionSets: []AgentPermissionSetEntry{{
			PermissionSetID: id.NewPermissionSetID(),
			RequirementType: RequirementTypeOptional,
		}},
	}
	require.NoError(t, agent.Validate())

	copy := agent.Copy()
	require.NotNil(t, copy.CanonicalID)
	assert.Equal(t, canonicalID, *copy.CanonicalID)
	*copy.CanonicalID = "other-agent"
	assert.Equal(t, canonicalID, *agent.CanonicalID)
}
