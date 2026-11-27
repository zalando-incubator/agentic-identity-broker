package storage

import (
	"errors"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

// ServiceScope pairs a ThirdpartyOAuth2Service reference with a list of OAuth2 scopes.
type ServiceScope struct {
	ServiceID       id.ServiceID    `json:"service_id"`
	Scopes          []string        `json:"scopes"`
	RequirementType RequirementType `json:"requirement_type"`
}

// PermissionSet is an admin-defined bundle of OAuth2 scopes spanning one or more third-party services.
type PermissionSet struct {
	ID               id.PermissionSetID   `json:"id" db:"id"`
	CanonicalID      *string              `json:"canonical_id,omitempty" db:"canonical_id"`
	ClearCanonicalID bool                 `json:"-" db:"-"`
	Name             string               `json:"name" db:"name"`
	Description      string               `json:"description" db:"description"`
	ServiceScopes    []ServiceScope       `json:"service_scopes" db:"service_scopes"`
	ObjectMetadata   model.ObjectMetadata `json:"-" db:"-"`
	CreatedAt        time.Time            `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time            `json:"updated_at" db:"updated_at"`
}

// validateCommon validates fields shared between Validate and ValidateForCreate.
func (ps *PermissionSet) validateCommon() error {
	if err := ValidateCanonicalID(ps.CanonicalID); err != nil {
		return err
	}
	if err := ps.ObjectMetadata.Validate(); err != nil {
		return err
	}

	if ps.Name == "" {
		return errors.New("name is required")
	}
	if len(ps.Name) > 255 {
		return fmt.Errorf("name exceeds 255 characters (got %d)", len(ps.Name))
	}
	if ps.Description == "" {
		return errors.New("description is required")
	}

	if len(ps.ServiceScopes) == 0 {
		return errors.New("at least one service scope is required")
	}

	for i, ss := range ps.ServiceScopes {
		if ss.ServiceID.IsZero() {
			return fmt.Errorf("service_scope[%d]: service_id is required", i)
		}
		scopeSet := make(map[string]bool)
		for _, scope := range ss.Scopes {
			if scope == "" {
				return fmt.Errorf("service_scope[%d]: scope cannot be empty", i)
			}
			if scopeSet[scope] {
				return fmt.Errorf("service_scope[%d]: duplicate scope '%s'", i, scope)
			}
			scopeSet[scope] = true
		}

		if ss.RequirementType == "" {
			return fmt.Errorf("service_scope[%d]: requirement_type is required", i)
		} else if !ss.RequirementType.Valid() {
			return fmt.Errorf("service_scope[%d]: invalid requirement_type %q", i, ss.RequirementType)
		}
	}

	seenServices := make(map[id.ServiceID]int)
	for i, ss := range ps.ServiceScopes {
		if prevIdx, exists := seenServices[ss.ServiceID]; exists {
			return fmt.Errorf("duplicate service_id %q found at indices %d and %d", ss.ServiceID, prevIdx, i)
		}
		seenServices[ss.ServiceID] = i
	}

	return nil
}

// Validate performs validation on the PermissionSet entity.
func (ps *PermissionSet) Validate() error {
	if ps.ID.IsZero() {
		return errors.New("permission set ID cannot be empty")
	}
	return ps.validateCommon()
}

// ValidateForCreate validates a permission set before creation.
// ID will be generated, so it may be empty.
func (ps *PermissionSet) ValidateForCreate() error {
	return ps.validateCommon()
}

// Copy creates a deep copy of the PermissionSet.
func (ps *PermissionSet) Copy() *PermissionSet {
	if ps == nil {
		return nil
	}

	copy := &PermissionSet{
		ID:             ps.ID,
		Name:           ps.Name,
		Description:    ps.Description,
		ObjectMetadata: ps.ObjectMetadata,
		CreatedAt:      ps.CreatedAt,
		UpdatedAt:      ps.UpdatedAt,
	}

	if ps.CanonicalID != nil {
		canonicalID := *ps.CanonicalID
		copy.CanonicalID = &canonicalID
	}

	// Deep copy service scopes
	if ps.ServiceScopes != nil {
		copy.ServiceScopes = make([]ServiceScope, len(ps.ServiceScopes))
		for i, ss := range ps.ServiceScopes {
			copy.ServiceScopes[i] = ServiceScope{
				ServiceID:       ss.ServiceID,
				Scopes:          append([]string(nil), ss.Scopes...),
				RequirementType: ss.RequirementType,
			}
		}
	}

	return copy
}
