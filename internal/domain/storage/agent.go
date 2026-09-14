package storage

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
)

// ClientType classifies an Agent based on its registered properties.
// Classification is derived at runtime from Agent.ClientID and Agent.ClientURIs.
type ClientType int

const (
	// UnknownClient is the zero value — uninitialized; must never reach dispatch logic.
	UnknownClient ClientType = iota
	// ProxyClient: agent has a ClientID — requests are forwarded to an upstream OAuth2 server.
	ProxyClient
	// CIMDClient: agent has ClientURIs but no ClientID — tokens are issued locally via CIMD.
	CIMDClient
	// LocalClient: agent has neither ClientID nor ClientURIs — tokens are issued locally.
	LocalClient
	// AmbiguousClient: agent has both ClientID and ClientURIs set — invalid, must not proceed.
	AmbiguousClient
)

// AgentPermissionSetEntry is one element of Agent.PermissionSets.
// Pairs a permission set reference with its requirement type.
type AgentPermissionSetEntry struct {
	PermissionSetID id.PermissionSetID `json:"permission_set_id"`
	RequirementType RequirementType    `json:"requirement_type"`
}

// Agent represents an AI agent registered in the identity broker.
// An agent can request delegated permissions from users to access third-party services.
type Agent struct {
	ID                   id.AgentID                `json:"id" db:"id"`
	CanonicalID          *string                   `json:"canonical_id,omitempty" db:"canonical_id"`
	ClearCanonicalID     bool                      `json:"-" db:"-"`
	ClientID             *id.ClientID              `json:"client_id,omitempty" db:"client_id"`
	ExternalID           *id.ExternalID            `json:"external_id,omitempty" db:"external_id"`
	DisplayName          string                    `json:"display_name" db:"display_name"`
	Description          string                    `json:"description" db:"description"`
	GovernanceURL        *string                   `json:"governance_url,omitempty" db:"governance_url"`
	UserDocumentationURL *string                   `json:"user_documentation_url,omitempty" db:"user_documentation_url"`
	AgentInterfaceURL    *string                   `json:"agent_interface_url,omitempty" db:"agent_interface_url"`
	ServiceRequirements  []ServiceRequirement      `json:"service_requirements,omitempty" db:"service_requirements"`
	PermissionSets       []AgentPermissionSetEntry `json:"permission_sets,omitempty" db:"permission_sets"`
	RedirectURIs         []string                  `json:"redirect_uris" db:"redirect_uris"`
	AllowedScopes        []string                  `json:"allowed_scopes" db:"allowed_scopes"`
	// ClientURIs holds pre-registered Client ID Metadata Document URLs.
	// Each entry must be a valid HTTPS URL, globally unique across all agents.
	ClientURIs []string  `json:"client_uris,omitempty" db:"-"`
	CreatedAt  time.Time `json:"created_at" db:"created_at"`
	UpdatedAt  time.Time `json:"updated_at" db:"updated_at"`
}

// Validate performs validation on the Agent entity.
// Returns an error if any validation rules are violated.
func (a *Agent) validateFields() error {
	if err := ValidateCanonicalID(a.CanonicalID); err != nil {
		return err
	}

	if a.ClientID != nil && strings.TrimSpace(string(*a.ClientID)) == "" {
		return errors.New("client_id cannot be empty when provided")
	}
	if a.ClientID != nil && len(a.ClientURIs) > 0 {
		return errors.New("agent cannot have both client_id and client_uris set: client mode must be unambiguous")
	}
	if a.DisplayName == "" {
		return errors.New("display_name is required")
	}
	if len(a.DisplayName) > 255 {
		return errors.New("display_name exceeds 255 characters")
	}
	if a.Description == "" {
		return errors.New("description is required")
	}
	if len(a.Description) > 1000 {
		return errors.New("description exceeds 1000 characters")
	}

	// URL validation (SR-011: prevent injection attacks)
	if a.GovernanceURL != nil && !isValidURL(*a.GovernanceURL) {
		return errors.New("governance_url is not a valid HTTP/HTTPS URL")
	}
	if a.UserDocumentationURL != nil && !isValidURL(*a.UserDocumentationURL) {
		return errors.New("user_documentation_url is not a valid HTTP/HTTPS URL")
	}
	if a.AgentInterfaceURL != nil && !isValidURL(*a.AgentInterfaceURL) {
		return errors.New("agent_interface_url is not a valid HTTP/HTTPS URL")
	}

	// Redirect URI validation (stricter than isValidURL: requires non-empty host, no fragment)
	for i, uri := range a.RedirectURIs {
		if !urivalidation.IsValidRedirectURI(uri) {
			return fmt.Errorf("redirect_uris[%d] is not a valid absolute HTTP/HTTPS URI without a fragment", i)
		}
	}

	if err := validateClientURIFormats(a.ClientURIs); err != nil {
		return err
	}

	if err := a.ValidateServiceRequirements(); err != nil {
		return fmt.Errorf("service_requirements validation failed: %w", err)
	}

	if err := a.ValidatePermissionSets(); err != nil {
		return fmt.Errorf("permission_sets validation failed: %w", err)
	}

	return nil
}

func (a *Agent) Validate() error {
	if a.ID.IsZero() {
		return errors.New("agent ID cannot be empty")
	}
	return a.validateFields()
}

// ClientType returns the classification of this agent based on its registered properties.
// Returns AmbiguousClient when both ClientID and ClientURIs are set — callers must reject this.
func (a Agent) ClientType() ClientType {
	if a.ClientID != nil && len(a.ClientURIs) > 0 {
		return AmbiguousClient
	}
	if a.ClientID != nil {
		return ProxyClient
	}
	if len(a.ClientURIs) > 0 {
		return CIMDClient
	}
	return LocalClient
}

// ValidateClientURIsForWrite runs the full client URI validation (format and duplicates).
// Used by admin mutation paths (create and update).
func ValidateClientURIsForWrite(uris []string) error {
	return validateClientURIs(uris)
}

// validateClientURIFormats validates the format and uniqueness of each entry in the list.
func validateClientURIFormats(uris []string) error {
	seen := make(map[string]struct{}, len(uris))
	for i, uriStr := range uris {
		if _, dup := seen[uriStr]; dup {
			return fmt.Errorf("client_uris[%d] is a duplicate: %q", i, uriStr)
		}
		seen[uriStr] = struct{}{}
		if err := validateClientURI(uriStr); err != nil {
			return fmt.Errorf("client_uris[%d]: %w", i, err)
		}
	}
	return nil
}

func validateClientURIs(uris []string) error {
	return validateClientURIFormats(uris)
}

func validateClientURI(uriStr string) error {
	return urivalidation.ValidateCIMDClientURIRegistration(uriStr)
}

// isValidURL validates that a string is a valid HTTP or HTTPS URL.
func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

// Copy creates a deep copy of the Agent to prevent external mutation.
func (a *Agent) Copy() *Agent {
	if a == nil {
		return nil
	}

	copy := &Agent{
		ID:          a.ID,
		DisplayName: a.DisplayName,
		Description: a.Description,
		CreatedAt:   a.CreatedAt,
		UpdatedAt:   a.UpdatedAt,
	}

	if a.CanonicalID != nil {
		canonicalID := *a.CanonicalID
		copy.CanonicalID = &canonicalID
	}

	if a.ClientID != nil {
		clientID := *a.ClientID
		copy.ClientID = &clientID
	}

	if a.ExternalID != nil {
		externalID := *a.ExternalID
		copy.ExternalID = &externalID
	}
	if a.GovernanceURL != nil {
		governanceURL := *a.GovernanceURL
		copy.GovernanceURL = &governanceURL
	}
	if a.UserDocumentationURL != nil {
		userDocURL := *a.UserDocumentationURL
		copy.UserDocumentationURL = &userDocURL
	}
	if a.AgentInterfaceURL != nil {
		agentURL := *a.AgentInterfaceURL
		copy.AgentInterfaceURL = &agentURL
	}

	// Deep copy permission sets
	if a.PermissionSets != nil {
		copy.PermissionSets = make([]AgentPermissionSetEntry, len(a.PermissionSets))
		for i, ps := range a.PermissionSets {
			copy.PermissionSets[i] = AgentPermissionSetEntry{
				PermissionSetID: ps.PermissionSetID,
				RequirementType: ps.RequirementType,
			}
		}
	}

	// Deep copy service requirements
	if a.ServiceRequirements != nil {
		copy.ServiceRequirements = make([]ServiceRequirement, len(a.ServiceRequirements))
		for i, sr := range a.ServiceRequirements {
			copy.ServiceRequirements[i] = ServiceRequirement{
				ServiceID:        sr.ServiceID,
				RequirementType:  sr.RequirementType,
				RequiredScopes:   append([]string(nil), sr.RequiredScopes...),
				RequireAllScopes: sr.RequireAllScopes,
			}
		}
	}

	// Deep copy redirect URIs and allowed scopes
	if a.RedirectURIs != nil {
		copy.RedirectURIs = append([]string(nil), a.RedirectURIs...)
	}
	if a.AllowedScopes != nil {
		copy.AllowedScopes = append([]string(nil), a.AllowedScopes...)
	}

	if a.ClientURIs != nil {
		copy.ClientURIs = append([]string(nil), a.ClientURIs...)
	}

	return copy
}

// ValidateForCreate validates an agent before creation.
// ID will be generated, so it may be empty.
func (a *Agent) ValidateForCreate() error {
	return a.validateFields()
}

// ValidateServiceRequirements validates the service requirements array.
// Returns error if:
// - Any service requirement is invalid
// - Duplicate service_id exists in the array
func (a *Agent) ValidateServiceRequirements() error {
	if len(a.ServiceRequirements) == 0 {
		return nil // Empty array is valid (no requirements)
	}

	// Validate each service requirement
	for i, sr := range a.ServiceRequirements {
		if err := sr.Validate(); err != nil {
			return fmt.Errorf("service_requirements[%d] invalid: %w", i, err)
		}
	}

	// Check for duplicate service_id (domain invariant)
	seen := make(map[id.ServiceID]int)
	for i, sr := range a.ServiceRequirements {
		if prevIdx, exists := seen[sr.ServiceID]; exists {
			return fmt.Errorf("duplicate service_id %q found at indices %d and %d", sr.ServiceID, prevIdx, i)
		}
		seen[sr.ServiceID] = i
	}

	return nil
}

// ValidatePermissionSets validates the permission sets array.
// Returns error if:
// - The array is empty
// - Any entry has a zero PermissionSetID
// - Any entry has an invalid RequirementType
// - Duplicate PermissionSetID exists in the array
func (a *Agent) ValidatePermissionSets() error {
	if len(a.PermissionSets) == 0 {
		return errors.New("at least one permission set entry is required")
	}

	seen := make(map[id.PermissionSetID]int)
	for i, ps := range a.PermissionSets {
		if ps.PermissionSetID.IsZero() {
			return fmt.Errorf("permission_sets[%d]: permission_set_id is required", i)
		}
		if !ps.RequirementType.Valid() {
			return fmt.Errorf("permission_sets[%d]: invalid requirement_type %q", i, ps.RequirementType)
		}
		if prevIdx, exists := seen[ps.PermissionSetID]; exists {
			return fmt.Errorf("duplicate permission_set_id %q found at indices %d and %d", ps.PermissionSetID, prevIdx, i)
		}
		seen[ps.PermissionSetID] = i
	}

	return nil
}
