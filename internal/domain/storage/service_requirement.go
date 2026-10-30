package storage

import (
	"fmt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// ServiceRequirement represents a third-party OAuth2 service that an agent requires or can optionally use.
// Each requirement specifies:
// - ServiceID: UUID of the third-party service
// - RequirementType: Whether the service is mandatory or optional
// - RequiredScopes: OAuth2 scopes that must be granted for this service
// - RequireAllScopes: Whether all permission-set scopes for the service are required
//
// Domain invariants:
// - ServiceID must be a valid UUID
// - RequirementType must be "mandatory" or "optional"
// - RequiredScopes may be empty only when the service defines no scopes or RequireAllScopes is true
// - RequiredScopes must be empty when RequireAllScopes is true
// - Each scope name must be non-empty
type ServiceRequirement struct {
	ServiceID        id.ServiceID    `json:"service_id" db:"service_id"`
	RequirementType  RequirementType `json:"requirement_type" db:"requirement_type"`
	RequiredScopes   []string        `json:"required_scopes" db:"required_scopes"`
	RequireAllScopes bool            `json:"require_all_scopes,omitempty" db:"require_all_scopes"`
}

// Validate checks if the service requirement is valid.
// Returns error if:
// - ServiceID is empty
// - RequirementType is invalid
// - Any scope is empty
// - RequiredScopes is non-empty when RequireAllScopes is true
func (sr *ServiceRequirement) Validate() error {
	if sr.ServiceID.IsZero() {
		return fmt.Errorf("service_id is required")
	}

	if err := sr.RequirementType.Validate(); err != nil {
		return fmt.Errorf("requirement_type validation failed: %w", err)
	}

	for i, scope := range sr.RequiredScopes {
		if scope == "" {
			return fmt.Errorf("required_scopes[%d] is empty", i)
		}
	}

	if sr.RequireAllScopes && len(sr.RequiredScopes) > 0 {
		return fmt.Errorf("required_scopes must be empty when require_all_scopes is true")
	}

	return nil
}

// HasScope returns true if the service requirement includes the specified scope.
// Scope comparison is case-sensitive.
func (sr *ServiceRequirement) HasScope(scope string) bool {
	for _, s := range sr.RequiredScopes {
		if s == scope {
			return true
		}
	}
	return false
}

// IsMandatory returns true if this is a mandatory service requirement.
func (sr *ServiceRequirement) IsMandatory() bool {
	return sr.RequirementType.IsMandatory()
}

// IsOptional returns true if this is an optional service requirement.
func (sr *ServiceRequirement) IsOptional() bool {
	return sr.RequirementType.IsOptional()
}
