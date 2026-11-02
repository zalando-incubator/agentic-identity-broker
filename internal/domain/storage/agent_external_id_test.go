package storage

import (
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/assert"
)

func TestAgentExternalIDValidation(t *testing.T) {
	for _, value := range []*id.ExternalID{nil, new(id.ExternalID("")), new(id.ExternalID(" ")), new(id.ExternalID("Registry"))} {
		for _, create := range []bool{false, true} {
			a := &Agent{ID: id.NewAgentID(), DisplayName: "Agent", Description: "Definition", ExternalID: value, PermissionSets: []AgentPermissionSetEntry{{PermissionSetID: id.NewPermissionSetID(), RequirementType: RequirementTypeOptional}}}
			before := a.Copy()
			var err error
			if create {
				err = a.ValidateForCreate()
			} else {
				err = a.Validate()
			}
			if value != nil && *value == "" {
				assert.EqualError(t, err, "external_id cannot be empty when provided")
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, before, a)
		}
	}
}
