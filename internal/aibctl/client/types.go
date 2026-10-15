package client

import (
	"bytes"
	"encoding/json"
)

// NullableString distinguishes an omitted JSON field from an explicit null.
// Used for three-state update semantics (keep, clear, set).
type NullableString struct {
	Set   bool
	Null  bool
	Value string
}

func SetString(v string) NullableString { return NullableString{Set: true, Value: v} }
func NullString() NullableString        { return NullableString{Set: true, Null: true} }

func (n NullableString) IsZero() bool { return !n.Set }

func (n NullableString) MarshalJSON() ([]byte, error) {
	if n.Null {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

func (n *NullableString) UnmarshalJSON(data []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		n.Null, n.Value = true, ""
		return nil
	}
	n.Null = false
	return json.Unmarshal(data, &n.Value)
}

// Ptr returns the value as *string, nil when null or unset.
func (n NullableString) Ptr() *string {
	if !n.Set || n.Null {
		return nil
	}
	v := n.Value
	return &v
}

type Health struct {
	Status        string `json:"status"`
	Timestamp     string `json:"timestamp"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

type ServiceRequirement struct {
	ServiceID        string   `json:"service_id"`
	ServiceName      string   `json:"service_name,omitempty"`
	RequirementType  string   `json:"requirement_type"`
	RequiredScopes   []string `json:"required_scopes"`
	RequireAllScopes bool     `json:"require_all_scopes,omitempty"`
}

type PermissionSetRef struct {
	PermissionSetID string `json:"permission_set_id"`
	RequirementType string `json:"requirement_type"`
}

type AgentWrite struct {
	CanonicalID          NullableString       `json:"canonical_id,omitzero"`
	ClientID             NullableString       `json:"client_id,omitzero"`
	ExternalID           *string              `json:"external_id,omitempty"`
	DisplayName          string               `json:"display_name"`
	Description          string               `json:"description"`
	GovernanceURL        *string              `json:"governance_url,omitempty"`
	UserDocumentationURL *string              `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string              `json:"agent_interface_url,omitempty"`
	ServiceRequirements  []ServiceRequirement `json:"service_requirements,omitempty"`
	PermissionSets       []PermissionSetRef   `json:"permission_sets"`
	RedirectURIs         []string             `json:"redirect_uris,omitempty"`
	AllowedScopes        []string             `json:"allowed_scopes,omitempty"`
	ClientURIs           []string             `json:"client_uris,omitempty"`
}

type Agent struct {
	ID                   string               `json:"id"`
	CanonicalID          *string              `json:"canonical_id"`
	ClientID             *string              `json:"client_id,omitempty"`
	ExternalID           *string              `json:"external_id,omitempty"`
	DisplayName          string               `json:"display_name"`
	Description          string               `json:"description"`
	GovernanceURL        *string              `json:"governance_url,omitempty"`
	UserDocumentationURL *string              `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string              `json:"agent_interface_url,omitempty"`
	ServiceRequirements  []ServiceRequirement `json:"service_requirements,omitempty"`
	PermissionSets       []PermissionSetRef   `json:"permission_sets,omitempty"`
	RedirectURIs         []string             `json:"redirect_uris,omitempty"`
	AllowedScopes        []string             `json:"allowed_scopes,omitempty"`
	ClientURIs           []string             `json:"client_uris,omitempty"`
	CreatedAt            string               `json:"created_at"`
	UpdatedAt            string               `json:"updated_at"`
}

// Write converts a read representation into the full-replacement body.
func (a Agent) Write() AgentWrite {
	w := AgentWrite{
		ExternalID:           a.ExternalID,
		DisplayName:          a.DisplayName,
		Description:          a.Description,
		GovernanceURL:        a.GovernanceURL,
		UserDocumentationURL: a.UserDocumentationURL,
		AgentInterfaceURL:    a.AgentInterfaceURL,
		PermissionSets:       a.PermissionSets,
		RedirectURIs:         a.RedirectURIs,
		AllowedScopes:        a.AllowedScopes,
		ClientURIs:           a.ClientURIs,
	}
	for _, sr := range a.ServiceRequirements {
		sr.ServiceName = ""
		w.ServiceRequirements = append(w.ServiceRequirements, sr)
	}
	if a.CanonicalID != nil {
		w.CanonicalID = SetString(*a.CanonicalID)
	}
	if a.ClientID != nil {
		w.ClientID = SetString(*a.ClientID)
	}
	return w
}

type Discovery struct {
	EnableDiscovery bool    `json:"enable_discovery"`
	MetadataURL     *string `json:"metadata_url,omitempty"`
}

type Endpoints struct {
	TokenEndpoint     string `json:"token_endpoint"`
	AuthorizeEndpoint string `json:"authorize_endpoint"`
}

type Scope struct {
	ScopeValue  string `json:"scope_value"`
	Description string `json:"description"`
}

type ServiceWrite struct {
	CanonicalID             NullableString    `json:"canonical_id,omitzero"`
	DisplayName             string            `json:"display_name"`
	ClientID                string            `json:"client_id,omitempty"`
	ClientSecret            string            `json:"client_secret,omitempty"`
	TokenEndpointAuthMethod *string           `json:"token_endpoint_auth_method,omitempty"`
	OAuth2Flavor            string            `json:"oauth2_flavor,omitempty"`
	IssuerURI               string            `json:"issuer_uri,omitempty"`
	Discovery               Discovery         `json:"discovery"`
	Endpoints               *Endpoints        `json:"endpoints,omitempty"`
	Scopes                  []Scope           `json:"scopes"`
	ProtectedResources      []string          `json:"protected_resources,omitempty"`
	AuthorizationParams     map[string]string `json:"authorization_params,omitempty"`
}

type Service struct {
	ID                      string            `json:"id"`
	CanonicalID             *string           `json:"canonical_id"`
	DisplayName             string            `json:"display_name"`
	ClientID                string            `json:"client_id"`
	ClientSecret            *string           `json:"client_secret,omitempty"`
	TokenEndpointAuthMethod *string           `json:"token_endpoint_auth_method"`
	OAuth2Flavor            string            `json:"oauth2_flavor"`
	IssuerURI               string            `json:"issuer_uri"`
	Discovery               Discovery         `json:"discovery"`
	Endpoints               Endpoints         `json:"endpoints"`
	Scopes                  []Scope           `json:"scopes"`
	ProtectedResources      []string          `json:"protected_resources,omitempty"`
	AuthorizationParams     map[string]string `json:"authorization_params,omitempty"`
	CreatedAt               string            `json:"created_at"`
	UpdatedAt               string            `json:"updated_at"`
}

// RedactedSecret is the placeholder the API returns instead of a stored secret.
const RedactedSecret = "REDACTED"

// Write converts a read representation into the full-replacement body. The
// redacted secret placeholder is never sent back.
func (s Service) Write() ServiceWrite {
	w := ServiceWrite{
		DisplayName:             s.DisplayName,
		ClientID:                s.ClientID,
		TokenEndpointAuthMethod: s.TokenEndpointAuthMethod,
		OAuth2Flavor:            s.OAuth2Flavor,
		IssuerURI:               s.IssuerURI,
		Discovery:               s.Discovery,
		Scopes:                  s.Scopes,
		ProtectedResources:      s.ProtectedResources,
		AuthorizationParams:     s.AuthorizationParams,
	}
	if s.Scopes == nil {
		w.Scopes = []Scope{}
	}
	if s.Endpoints.TokenEndpoint != "" || s.Endpoints.AuthorizeEndpoint != "" {
		e := s.Endpoints
		w.Endpoints = &e
	}
	if s.CanonicalID != nil {
		w.CanonicalID = SetString(*s.CanonicalID)
	}
	return w
}

type ProtectedResourceSet struct {
	ProtectedResources []string `json:"protected_resources"`
}

type ProtectedResourceMutation struct {
	Resource           string   `json:"resource"`
	ProtectedResources []string `json:"protected_resources"`
}

type ServiceScope struct {
	ServiceID       string   `json:"service_id"`
	Scopes          []string `json:"scopes"`
	RequirementType string   `json:"requirement_type,omitempty"`
}

type PermissionSetWrite struct {
	CanonicalID   NullableString `json:"canonical_id,omitzero"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ServiceScopes []ServiceScope `json:"service_scopes"`
}

type PermissionSet struct {
	ID            string         `json:"id"`
	CanonicalID   *string        `json:"canonical_id"`
	Name          string         `json:"name"`
	Description   string         `json:"description"`
	ServiceScopes []ServiceScope `json:"service_scopes"`
	CreatedAt     string         `json:"created_at"`
	UpdatedAt     string         `json:"updated_at"`
}

func (p PermissionSet) Write() PermissionSetWrite {
	w := PermissionSetWrite{Name: p.Name, Description: p.Description, ServiceScopes: p.ServiceScopes}
	if p.CanonicalID != nil {
		w.CanonicalID = SetString(*p.CanonicalID)
	}
	return w
}

type ClientCredentials struct {
	ClientID              string  `json:"client_id"`
	ClientSecret          string  `json:"client_secret"`
	CreatedAt             string  `json:"created_at"`
	PreviousInvalidatedAt *string `json:"previous_invalidated_at,omitempty"`
}

type ClientCredentialsInfo struct {
	ClientID  string  `json:"client_id"`
	CreatedAt string  `json:"created_at"`
	RotatedAt *string `json:"rotated_at,omitempty"`
}

type Key struct {
	KID         string  `json:"kid"`
	Algorithm   string  `json:"algorithm"`
	IsCurrent   bool    `json:"is_current"`
	ActivatesAt string  `json:"activates_at"`
	CreatedAt   string  `json:"created_at"`
	RemovedAt   *string `json:"removed_at,omitempty"`
}

type SessionSweepRequest struct {
	LookaheadDuration *string `json:"lookahead_duration,omitempty"`
	DryRun            *bool   `json:"dry_run,omitempty"`
	PageSize          *int    `json:"page_size,omitempty"`
}

type SessionSweepResult struct {
	Refreshed      int  `json:"refreshed"`
	Skipped        int  `json:"skipped"`
	Failed         int  `json:"failed"`
	TotalEvaluated int  `json:"total_evaluated"`
	DryRun         bool `json:"dry_run"`
}
