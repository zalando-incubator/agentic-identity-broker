package storage

import (
	"errors"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// GrantedPermissionSetEntry pairs a granted permission set with the explicit list of
// service IDs the user included at consent time (positive-inclusion model).
type GrantedPermissionSetEntry struct {
	PermissionSetID    id.PermissionSetID `json:"permission_set_id"`
	IncludedServiceIDs []id.ServiceID     `json:"included_service_ids"`
}

// UserGrant represents a user delegating specific permissions to an agent
// for one or more third-party OAuth2 services.
type UserGrant struct {
	ID                    id.GrantID                  `json:"id" db:"id"`
	Principal             id.Principal                `json:"principal" db:"principal"`
	AgentID               id.AgentID                  `json:"agent_id" db:"agent_id"`
	ValidUntil            *time.Time                  `json:"valid_until,omitempty" db:"valid_until"`
	GrantedPermissionSets []GrantedPermissionSetEntry `json:"granted_permission_sets" db:"granted_permission_sets"`
	CreatedAt             time.Time                   `json:"created_at" db:"created_at"`
	UpdatedAt             time.Time                   `json:"updated_at" db:"updated_at"`
}

// Validate performs validation on the UserGrant entity.
func (g *UserGrant) Validate(decisionTime time.Time) error {
	// Required fields
	if g.ID.IsZero() {
		return errors.New("grant ID cannot be empty")
	}
	if g.Principal.IsZero() {
		return errors.New("principal is required")
	}
	if g.AgentID.IsZero() {
		return errors.New("agent_id is required")
	}

	// valid_until must be in future if provided
	if g.ValidUntil != nil && !g.ValidUntil.After(decisionTime) {
		return errors.New("valid_until must be in the future")
	}

	// Validate each granted permission set entry
	for i, entry := range g.GrantedPermissionSets {
		if entry.PermissionSetID.IsZero() {
			return fmt.Errorf("granted_permission_sets[%d]: permission_set_id is required", i)
		}
		if len(entry.IncludedServiceIDs) == 0 {
			return fmt.Errorf("granted_permission_sets[%d]: at least one included_service_id is required", i)
		}
		for j, svcID := range entry.IncludedServiceIDs {
			if svcID.IsZero() {
				return fmt.Errorf("granted_permission_sets[%d].included_service_ids[%d]: service ID is required", i, j)
			}
		}
	}

	return nil
}

// ValidateForCreate validates a grant before creation.
func (g *UserGrant) ValidateForCreate(decisionTime time.Time) error {
	if g.Principal.IsZero() {
		return errors.New("principal is required")
	}
	if g.AgentID.IsZero() {
		return errors.New("agent_id is required")
	}

	if g.ValidUntil != nil && !g.ValidUntil.After(decisionTime) {
		return errors.New("valid_until must be in the future")
	}

	for i, entry := range g.GrantedPermissionSets {
		if entry.PermissionSetID.IsZero() {
			return fmt.Errorf("granted_permission_sets[%d]: permission_set_id is required", i)
		}
		if len(entry.IncludedServiceIDs) == 0 {
			return fmt.Errorf("granted_permission_sets[%d]: at least one included_service_id is required", i)
		}
		for j, svcID := range entry.IncludedServiceIDs {
			if svcID.IsZero() {
				return fmt.Errorf("granted_permission_sets[%d].included_service_ids[%d]: service ID is required", i, j)
			}
		}
	}

	return nil
}

// IsActive reports whether the grant remains valid at decisionTime.
func (g *UserGrant) IsActive(decisionTime time.Time) bool {
	return g.ValidUntil == nil || g.ValidUntil.After(decisionTime)
}

// Copy creates a deep copy of the UserGrant.
func (g *UserGrant) Copy() *UserGrant {
	if g == nil {
		return nil
	}

	copy := &UserGrant{
		ID:        g.ID,
		Principal: g.Principal,
		AgentID:   g.AgentID,
		CreatedAt: g.CreatedAt,
		UpdatedAt: g.UpdatedAt,
	}

	if g.ValidUntil != nil {
		validUntil := *g.ValidUntil
		copy.ValidUntil = &validUntil
	}

	// Deep copy granted permission sets
	if g.GrantedPermissionSets != nil {
		copy.GrantedPermissionSets = make([]GrantedPermissionSetEntry, len(g.GrantedPermissionSets))
		for i, entry := range g.GrantedPermissionSets {
			copy.GrantedPermissionSets[i] = GrantedPermissionSetEntry{
				PermissionSetID:    entry.PermissionSetID,
				IncludedServiceIDs: append([]id.ServiceID(nil), entry.IncludedServiceIDs...),
			}
		}
	}

	return copy
}
