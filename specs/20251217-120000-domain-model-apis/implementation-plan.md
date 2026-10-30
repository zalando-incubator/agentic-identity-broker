# Implementation Plan: Domain Model and Consent APIs (Feature 20251217-120000)

**Feature Branch**: `20251217-120000-domain-model-apis`
**Status**: Implementation Ready
**Created**: 2025-12-17

## Table of Contents

1. [Overview](#overview)
2. [Domain Entity Definitions](#domain-entity-definitions)
3. [Repository Interface Definitions](#repository-interface-definitions)
4. [Consent Service Design](#consent-service-design)
5. [Database Schema Design](#database-schema-design)
6. [Directory Structure](#directory-structure)
7. [Implementation Phases](#implementation-phases)
8. [Testing Strategy](#testing-strategy)
9. [Security Considerations](#security-considerations)
10. [Key Implementation Patterns](#key-implementation-patterns)

---

## Overview

This plan implements the core domain model for the identity broker, introducing three new entities (Agent, ThirdpartyOAuth2Service, UserGrant) with full CRUD APIs and a consent service in the domain layer. The implementation follows hexagonal architecture principles established in feature 20251215-120100-persistence-layer, using small focused repository interfaces, sqlx for PostgreSQL, and interface segregation.

**Key Principles**:
- Hexagonal architecture with clear port/adapter boundaries
- Interface Segregation Principle (ISP): one repository interface per entity
- Domain logic in internal/domain/, isolated from storage implementation
- Consent service encapsulates business rules and validation logic
- No GORM - use sqlx with explicit SQL for PostgreSQL adapter
- Comprehensive testing: table-driven unit tests + testcontainers integration tests

---

## Domain Entity Definitions

### 1. Agent Entity

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/agent.go`

```go
package ports

import "time"

// Agent represents an AI agent registered in the identity broker.
// Contains OAuth2 client configuration, display metadata, and governance URLs.
type Agent struct {
	// Unique identifier (UUID v4, generated on creation)
	ID string `db:"id" json:"id"`

	// OAuth2 client_id for this agent (required)
	ClientID string `db:"client_id" json:"client_id"`

	// Optional external governance system identifier
	ExternalID *string `db:"external_id" json:"external_id,omitempty"`

	// Human-readable display name (required)
	DisplayName string `db:"display_name" json:"display_name"`

	// Human-readable description explaining agent's purpose (required)
	Description string `db:"description" json:"description"`

	// Optional URL to governance documentation
	GovernanceURL *string `db:"governance_url" json:"governance_url,omitempty"`

	// Optional URL to user documentation
	UserDocumentationURL *string `db:"user_documentation_url" json:"user_documentation_url,omitempty"`

	// Optional URL to agent interface
	AgentInterfaceURL *string `db:"agent_interface_url" json:"agent_interface_url,omitempty"`

	// Timestamps
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// Validate performs domain-level validation on the Agent entity.
func (a *Agent) Validate() error {
	if a.ID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if a.ClientID == "" {
		return fmt.Errorf("agent client_id cannot be empty")
	}
	if a.DisplayName == "" {
		return fmt.Errorf("agent display_name cannot be empty")
	}
	if a.Description == "" {
		return fmt.Errorf("agent description cannot be empty")
	}
	
	// Validate URLs if provided (SR-011: prevent injection attacks)
	if a.GovernanceURL != nil && !isValidURL(*a.GovernanceURL) {
		return fmt.Errorf("governance_url is not a valid URL")
	}
	if a.UserDocumentationURL != nil && !isValidURL(*a.UserDocumentationURL) {
		return fmt.Errorf("user_documentation_url is not a valid URL")
	}
	if a.AgentInterfaceURL != nil && !isValidURL(*a.AgentInterfaceURL) {
		return fmt.Errorf("agent_interface_url is not a valid URL")
	}
	
	return nil
}
```

### 2. ThirdpartyOAuth2Service Entity

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/thirdparty_oauth2_service.go`

```go
package ports

import (
	"encoding/json"
	"time"
)

// ThirdpartyOAuth2Service represents an external OAuth2 provider configuration.
// Examples: Google, GitHub, Databricks, Microsoft Azure AD
type ThirdpartyOAuth2Service struct {
	// Unique identifier (UUID v4, generated on creation)
	ID string `db:"id" json:"id"`

	// Human-readable service name (e.g., "Google Drive", "GitHub")
	DisplayName string `db:"display_name" json:"display_name"`

	// OAuth2 client_id for this service (required)
	ClientID string `db:"client_id" json:"client_id"`

	// OAuth2 client_secret (encrypted at rest, redacted in responses)
	ClientSecret string `db:"client_secret" json:"-"` // Never serialize

	// OAuth2 issuer URI (e.g., "https://accounts.google.com")
	IssuerURI string `db:"issuer_uri" json:"issuer_uri"`

	// Enable automatic endpoint discovery via well-known metadata
	EnableDiscovery bool `db:"enable_discovery" json:"enable_discovery"`

	// Optional: Override metadata URL (if not using standard well-known path)
	// Standard path: {issuer}/.well-known/oauth-authorization-server
	MetadataURL *string `db:"metadata_url" json:"metadata_url,omitempty"`

	// OAuth2 token endpoint (manually configured or auto-discovered)
	TokenEndpoint string `db:"token_endpoint" json:"token_endpoint"`

	// OAuth2 authorization endpoint (manually configured or auto-discovered)
	AuthorizeEndpoint string `db:"authorize_endpoint" json:"authorize_endpoint"`

	// Available OAuth2 scopes (JSON array of ScopeDefinition)
	// Stored as JSONB in PostgreSQL
	Scopes []ScopeDefinition `db:"scopes" json:"scopes"`

	// Timestamps
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// ScopeDefinition represents an OAuth2 scope with human-readable description.
type ScopeDefinition struct {
	// OAuth2 scope value (e.g., "https://www.googleapis.com/auth/drive.readonly")
	ScopeValue string `json:"scope_value"`

	// Human-readable description (e.g., "Read access to Google Drive files")
	Description string `json:"description"`
}

// Validate performs domain-level validation.
func (t *ThirdpartyOAuth2Service) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("service ID cannot be empty")
	}
	if t.DisplayName == "" {
		return fmt.Errorf("display_name cannot be empty")
	}
	if t.ClientID == "" {
		return fmt.Errorf("client_id cannot be empty")
	}
	if t.ClientSecret == "" {
		return fmt.Errorf("client_secret cannot be empty")
	}
	if t.IssuerURI == "" {
		return fmt.Errorf("issuer_uri cannot be empty")
	}
	
	// Validate issuer URI
	if !isValidURL(t.IssuerURI) {
		return fmt.Errorf("issuer_uri is not a valid URL")
	}
	
	// Validate metadata URL if provided
	if t.MetadataURL != nil && !isValidURL(*t.MetadataURL) {
		return fmt.Errorf("metadata_url is not a valid URL")
	}
	
	// Validate endpoints (required if discovery disabled)
	if !t.EnableDiscovery {
		if t.TokenEndpoint == "" {
			return fmt.Errorf("token_endpoint required when discovery disabled")
		}
		if t.AuthorizeEndpoint == "" {
			return fmt.Errorf("authorize_endpoint required when discovery disabled")
		}
	}
	
	// Validate scopes
	for i, scope := range t.Scopes {
		if scope.ScopeValue == "" {
			return fmt.Errorf("scope[%d].scope_value cannot be empty", i)
		}
		if scope.Description == "" {
			return fmt.Errorf("scope[%d].description cannot be empty", i)
		}
	}
	
	return nil
}

// RedactedCopy returns a copy with client_secret redacted for API responses.
func (t *ThirdpartyOAuth2Service) RedactedCopy() *ThirdpartyOAuth2Service {
	copy := *t
	copy.ClientSecret = "[REDACTED]"
	return &copy
}
```

### 3. UserGrant Entity

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/user_grant.go`

```go
package ports

import (
	"encoding/json"
	"time"
)

// UserGrant represents a user's delegation of permissions to an agent.
// One active grant per user-agent pair (upsert semantics).
type UserGrant struct {
	// Unique identifier (UUID v4, generated on creation)
	ID string `db:"id" json:"id"`

	// User principal (derived from authenticated session)
	Principal string `db:"principal" json:"principal"`

	// Agent receiving the grant (foreign key to agents.id)
	AgentID string `db:"agent_id" json:"agent_id"`

	// Optional expiration timestamp (null = indefinite grant)
	ValidUntil *time.Time `db:"valid_until" json:"valid_until,omitempty"`

	// Delegated OAuth2 tokens (JSON array of DelegatedToken)
	// Stored as JSONB in PostgreSQL
	DelegatedTokens []DelegatedToken `db:"delegated_tokens" json:"delegated_tokens"`

	// Timestamps
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
}

// DelegatedToken represents granted scopes for a specific third-party service.
type DelegatedToken struct {
	// Third-party OAuth2 service ID (foreign key to thirdparty_oauth2_services.id)
	ThirdpartyOAuth2ServiceID string `json:"thirdparty_oauth2_service_id"`

	// Granted OAuth2 scopes (array of scope values)
	GrantedScopes []string `json:"granted_scopes"`
}

// Validate performs domain-level validation.
func (u *UserGrant) Validate() error {
	if u.ID == "" {
		return fmt.Errorf("grant ID cannot be empty")
	}
	if u.Principal == "" {
		return fmt.Errorf("principal cannot be empty")
	}
	if u.AgentID == "" {
		return fmt.Errorf("agent_id cannot be empty")
	}
	
	// Validate valid_until is in the future if specified
	if u.ValidUntil != nil && u.ValidUntil.Before(time.Now()) {
		return fmt.Errorf("valid_until must be in the future")
	}
	
	// Validate delegated tokens
	if len(u.DelegatedTokens) == 0 {
		return fmt.Errorf("delegated_tokens cannot be empty")
	}
	
	for i, token := range u.DelegatedTokens {
		if token.ThirdpartyOAuth2ServiceID == "" {
			return fmt.Errorf("delegated_tokens[%d].thirdparty_oauth2_service_id cannot be empty", i)
		}
		if len(token.GrantedScopes) == 0 {
			return fmt.Errorf("delegated_tokens[%d].granted_scopes cannot be empty", i)
		}
	}
	
	return nil
}

// IsExpired checks if the grant has expired.
func (u *UserGrant) IsExpired() bool {
	if u.ValidUntil == nil {
		return false // Indefinite grant never expires
	}
	return time.Now().After(*u.ValidUntil)
}
```

---

## Repository Interface Definitions

Following ISP (Interface Segregation Principle), each entity gets its own focused repository interface.

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/storage.go` (append to existing file)

```go
// AgentRepository defines storage operations for Agent entities.
// Follows Repository pattern with focused CRUD operations.
type AgentRepository interface {
	// CreateAgent creates a new agent entity in storage.
	// Returns StorageError with Kind=Conflict if agent ID already exists.
	CreateAgent(ctx context.Context, agent *Agent) error

	// GetAgent retrieves an agent entity by ID.
	// Returns StorageError with Kind=NotFound if agent not found.
	GetAgent(ctx context.Context, id string) (*Agent, error)

	// UpdateAgent updates an existing agent entity.
	// Returns StorageError with Kind=NotFound if agent not found.
	UpdateAgent(ctx context.Context, agent *Agent) error

	// DeleteAgent deletes an agent entity by ID.
	// CASCADE: Also deletes all associated grants (FR-021).
	// Idempotent: safe to delete non-existent agents.
	DeleteAgent(ctx context.Context, id string) error

	// ListAgents retrieves all agent entities (no filtering in MVP).
	// Returns empty slice if no agents exist (not an error).
	ListAgents(ctx context.Context) ([]*Agent, error)
}

// ThirdpartyOAuth2ServiceRepository defines storage operations for third-party OAuth2 service entities.
type ThirdpartyOAuth2ServiceRepository interface {
	// CreateService creates a new third-party OAuth2 service configuration.
	// Client secret must be encrypted before storage (SR-002).
	// Returns StorageError with Kind=Conflict if service ID already exists.
	CreateService(ctx context.Context, service *ThirdpartyOAuth2Service) error

	// GetService retrieves a service configuration by ID.
	// Client secret is returned encrypted (caller must decrypt if needed).
	// Returns StorageError with Kind=NotFound if service not found.
	GetService(ctx context.Context, id string) (*ThirdpartyOAuth2Service, error)

	// UpdateService updates an existing service configuration.
	// Client secret must be re-encrypted if changed (SR-002).
	// Returns StorageError with Kind=NotFound if service not found.
	UpdateService(ctx context.Context, service *ThirdpartyOAuth2Service) error

	// DeleteService deletes a service configuration by ID.
	// BLOCKED: Returns StorageError with Kind=Conflict if active grants reference this service (FR-022).
	// Idempotent: safe to delete non-existent services.
	DeleteService(ctx context.Context, id string) error

	// ListServices retrieves all service configurations (no filtering in MVP).
	// Returns empty slice if no services exist (not an error).
	ListServices(ctx context.Context) ([]*ThirdpartyOAuth2Service, error)

	// CountGrantsReferencingService counts active grants referencing this service.
	// Used to enforce referential integrity before deletion (FR-022).
	CountGrantsReferencingService(ctx context.Context, serviceID string) (int, error)
}

// UserGrantRepository defines storage operations for UserGrant entities.
type UserGrantRepository interface {
	// CreateGrant creates a new user grant or updates existing grant (upsert semantics).
	// One active grant per user-agent pair (FR-015).
	// Returns StorageError if agent_id or thirdparty_oauth2_service_ids are invalid (foreign key violation).
	CreateGrant(ctx context.Context, grant *UserGrant) error

	// GetGrant retrieves a grant by ID.
	// Returns StorageError with Kind=NotFound if grant not found.
	GetGrant(ctx context.Context, id string) (*UserGrant, error)

	// GetGrantByPrincipalAndAgent retrieves a grant by user principal and agent ID.
	// Used to enforce one grant per user-agent pair.
	// Returns StorageError with Kind=NotFound if grant not found.
	GetGrantByPrincipalAndAgent(ctx context.Context, principal, agentID string) (*UserGrant, error)

	// ListGrantsByPrincipalAndAgent retrieves all grants for a user-agent pair.
	// Filters expired grants (valid_until < current_time) unless includeExpired=true.
	// Returns empty slice if no grants exist (not an error).
	ListGrantsByPrincipalAndAgent(ctx context.Context, principal, agentID string, includeExpired bool) ([]*UserGrant, error)

	// UpdateGrant updates an existing grant entity.
	// Returns StorageError with Kind=NotFound if grant not found.
	UpdateGrant(ctx context.Context, grant *UserGrant) error

	// DeleteGrant deletes a grant entity by ID.
	// Idempotent: safe to delete non-existent grants.
	DeleteGrant(ctx context.Context, id string) error

	// DeleteGrantsByAgent deletes all grants for a specific agent (CASCADE support for FR-021).
	// Returns count of deleted grants.
	DeleteGrantsByAgent(ctx context.Context, agentID string) (int, error)
}
```

---

## Consent Service Design

The consent service encapsulates business logic for consent workflows, sitting in the domain layer above storage adapters.

### Service Interface

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/domain/consent/service.go`

```go
package consent

import (
	"context"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/google/uuid"
)

// Service provides business logic for consent management.
// Encapsulates grant creation, validation, and retrieval.
type Service struct {
	agentRepo      ports.AgentRepository
	serviceRepo    ports.ThirdpartyOAuth2ServiceRepository
	grantRepo      ports.UserGrantRepository
}

// NewService creates a new consent service.
func NewService(
	agentRepo ports.AgentRepository,
	serviceRepo ports.ThirdpartyOAuth2ServiceRepository,
	grantRepo ports.UserGrantRepository,
) *Service {
	return &Service{
		agentRepo:   agentRepo,
		serviceRepo: serviceRepo,
		grantRepo:   grantRepo,
	}
}

// GetAgentConsentInfo retrieves all information needed for consent UI.
// Returns agent metadata and all available third-party services with scopes (FR-025).
func (s *Service) GetAgentConsentInfo(ctx context.Context, agentID string) (*AgentConsentInfo, error) {
	// Get agent
	agent, err := s.agentRepo.GetAgent(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}

	// Get all available third-party services (FR-025: return all configured services)
	services, err := s.serviceRepo.ListServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list services: %w", err)
	}

	// Build response with redacted secrets
	var serviceInfos []ThirdpartyServiceInfo
	for _, svc := range services {
		serviceInfos = append(serviceInfos, ThirdpartyServiceInfo{
			ID:          svc.ID,
			DisplayName: svc.DisplayName,
			Scopes:      svc.Scopes,
		})
	}

	return &AgentConsentInfo{
		AgentID:              agent.ID,
		DisplayName:          agent.DisplayName,
		Description:          agent.Description,
		GovernanceURL:        agent.GovernanceURL,
		UserDocumentationURL: agent.UserDocumentationURL,
		AgentInterfaceURL:    agent.AgentInterfaceURL,
		AvailableServices:    serviceInfos,
	}, nil
}

// GrantConsent creates or updates a user grant with scope validation.
// Enforces one grant per user-agent pair (upsert semantics, FR-015).
func (s *Service) GrantConsent(ctx context.Context, req *GrantConsentRequest) (*ports.UserGrant, error) {
	// Validate agent exists
	agent, err := s.agentRepo.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, fmt.Errorf("invalid agent_id: %w", err)
	}

	// Validate valid_until is in future if specified (FR-016)
	if req.ValidUntil != nil && req.ValidUntil.Before(time.Now()) {
		return nil, storage.NewStorageError(
			"GrantConsent",
			storage.ErrorKindValidation,
			nil,
			"valid_until must be in the future",
		)
	}

	// Validate all requested services and scopes exist (FR-018)
	var delegatedTokens []ports.DelegatedToken
	for _, reqToken := range req.DelegatedTokens {
		// Get service configuration
		svc, err := s.serviceRepo.GetService(ctx, reqToken.ThirdpartyOAuth2ServiceID)
		if err != nil {
			return nil, fmt.Errorf("invalid thirdparty_oauth2_service_id %s: %w", reqToken.ThirdpartyOAuth2ServiceID, err)
		}

		// Validate all requested scopes exist in service configuration
		if err := s.validateScopes(svc, reqToken.GrantedScopes); err != nil {
			return nil, err
		}

		delegatedTokens = append(delegatedTokens, ports.DelegatedToken{
			ThirdpartyOAuth2ServiceID: reqToken.ThirdpartyOAuth2ServiceID,
			GrantedScopes:             reqToken.GrantedScopes,
		})
	}

	// Check if grant already exists (one per user-agent pair)
	existingGrant, err := s.grantRepo.GetGrantByPrincipalAndAgent(ctx, req.Principal, req.AgentID)
	if err == nil {
		// Update existing grant
		existingGrant.DelegatedTokens = delegatedTokens
		existingGrant.ValidUntil = req.ValidUntil
		existingGrant.UpdatedAt = time.Now()

		if err := existingGrant.Validate(); err != nil {
			return nil, storage.NewStorageError(
				"GrantConsent",
				storage.ErrorKindValidation,
				err,
				"invalid grant data",
			)
		}

		if err := s.grantRepo.UpdateGrant(ctx, existingGrant); err != nil {
			return nil, fmt.Errorf("failed to update grant: %w", err)
		}

		return existingGrant, nil
	}

	// Create new grant
	newGrant := &ports.UserGrant{
		ID:              uuid.New().String(),
		Principal:       req.Principal,
		AgentID:         req.AgentID,
		ValidUntil:      req.ValidUntil,
		DelegatedTokens: delegatedTokens,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	if err := newGrant.Validate(); err != nil {
		return nil, storage.NewStorageError(
			"GrantConsent",
			storage.ErrorKindValidation,
			err,
			"invalid grant data",
		)
	}

	if err := s.grantRepo.CreateGrant(ctx, newGrant); err != nil {
		return nil, fmt.Errorf("failed to create grant: %w", err)
	}

	return newGrant, nil
}

// RevokeConsent revokes a user's grant for an agent (FR-014).
func (s *Service) RevokeConsent(ctx context.Context, principal, agentID string) error {
	// Find existing grant
	grant, err := s.grantRepo.GetGrantByPrincipalAndAgent(ctx, principal, agentID)
	if err != nil {
		// Grant doesn't exist - idempotent success
		return nil
	}

	// Delete grant
	if err := s.grantRepo.DeleteGrant(ctx, grant.ID); err != nil {
		return fmt.Errorf("failed to delete grant: %w", err)
	}

	return nil
}

// GetActiveGrants retrieves active (non-expired) grants for a user-agent pair (FR-019).
func (s *Service) GetActiveGrants(ctx context.Context, principal, agentID string) ([]*ports.UserGrant, error) {
	// Get grants (excluding expired)
	grants, err := s.grantRepo.ListGrantsByPrincipalAndAgent(ctx, principal, agentID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	return grants, nil
}

// validateScopes checks that all requested scopes exist in service configuration (FR-018).
func (s *Service) validateScopes(svc *ports.ThirdpartyOAuth2Service, requestedScopes []string) error {
	// Build set of valid scopes
	validScopes := make(map[string]bool)
	for _, scope := range svc.Scopes {
		validScopes[scope.ScopeValue] = true
	}

	// Check each requested scope
	for _, requestedScope := range requestedScopes {
		if !validScopes[requestedScope] {
			return storage.NewStorageError(
				"GrantConsent",
				storage.ErrorKindValidation,
				nil,
				fmt.Sprintf("scope %q not available for service %s", requestedScope, svc.DisplayName),
			)
		}
	}

	return nil
}

// AgentConsentInfo contains all information for consent UI (FR-010).
type AgentConsentInfo struct {
	AgentID              string                  `json:"agent_id"`
	DisplayName          string                  `json:"display_name"`
	Description          string                  `json:"description"`
	GovernanceURL        *string                 `json:"governance_url,omitempty"`
	UserDocumentationURL *string                 `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string                 `json:"agent_interface_url,omitempty"`
	RequestedServices    []ThirdpartyServiceInfo `json:"requested_services"`
}

// ThirdpartyServiceInfo contains service metadata for consent UI.
type ThirdpartyServiceInfo struct {
	ID          string                   `json:"id"`
	DisplayName string                   `json:"display_name"`
	Scopes      []ports.ScopeDefinition `json:"scopes"`
}

// GrantConsentRequest represents a consent grant request.
type GrantConsentRequest struct {
	Principal       string                 `json:"principal"`
	AgentID         string                 `json:"agent_id"`
	ValidUntil      *time.Time             `json:"valid_until,omitempty"`
	DelegatedTokens []ports.DelegatedToken `json:"delegated_tokens"`
}
```

---

## Database Schema Design

### Migration File Structure

Migrations live in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/migrations/` (create if doesn't exist).

### Migration 001: Initial Domain Model Schema

**File**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/migrations/001_domain_model.up.sql`

```sql
-- Migration 001: Domain Model and Consent APIs
-- Feature: 20251217-120000-domain-model-apis
-- Description: Creates agents, thirdparty_oauth2_services, and user_grants tables

-- Enable UUID generation
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Note: Client secrets encrypted via EncryptionPort interface

-- ============================================================================
-- Table: agents
-- Description: AI agents registered in the identity broker
-- ============================================================================
CREATE TABLE agents (
    id                         UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    client_id                  VARCHAR(255) NOT NULL UNIQUE,
    external_id                VARCHAR(255),
    display_name               VARCHAR(255) NOT NULL,
    description                TEXT NOT NULL,
    governance_url             TEXT,
    user_documentation_url     TEXT,
    agent_interface_url        TEXT,
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for agents
CREATE INDEX idx_agents_client_id ON agents(client_id);
CREATE INDEX idx_agents_external_id ON agents(external_id) WHERE external_id IS NOT NULL;
CREATE INDEX idx_agents_created_at ON agents(created_at);

-- ============================================================================
-- Table: thirdparty_oauth2_services
-- Description: External OAuth2 provider configurations
-- ============================================================================
CREATE TABLE thirdparty_oauth2_services (
    id                    UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    display_name          VARCHAR(255) NOT NULL,
    client_id             VARCHAR(255) NOT NULL,
    -- Client secret encrypted at rest via EncryptionPort (SR-002)
    -- Stored as BYTEA (encrypted bytes from EncryptionPort.Encrypt)
    client_secret         BYTEA NOT NULL,
    issuer_uri            TEXT NOT NULL,
    enable_discovery      BOOLEAN NOT NULL DEFAULT true,
    metadata_url          TEXT,
    token_endpoint        TEXT,
    authorize_endpoint    TEXT,
    -- Scopes stored as JSONB array of {scope_value, description}
    scopes                JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes for thirdparty_oauth2_services
CREATE INDEX idx_thirdparty_oauth2_services_client_id ON thirdparty_oauth2_services(client_id);
CREATE INDEX idx_thirdparty_oauth2_services_display_name ON thirdparty_oauth2_services(display_name);
CREATE INDEX idx_thirdparty_oauth2_services_created_at ON thirdparty_oauth2_services(created_at);

-- ============================================================================
-- Table: user_grants
-- Description: User permissions delegated to agents
-- ============================================================================
CREATE TABLE user_grants (
    id                 UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    principal          VARCHAR(255) NOT NULL,
    agent_id           UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    valid_until        TIMESTAMPTZ,
    -- Delegated tokens stored as JSONB array of {thirdparty_oauth2_service_id, granted_scopes[]}
    delegated_tokens   JSONB NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    
    -- Enforce one active grant per user-agent pair (FR-015)
    CONSTRAINT unique_principal_agent UNIQUE (principal, agent_id)
);

-- Indexes for user_grants
CREATE INDEX idx_user_grants_principal ON user_grants(principal);
CREATE INDEX idx_user_grants_agent_id ON user_grants(agent_id);
CREATE INDEX idx_user_grants_valid_until ON user_grants(valid_until) WHERE valid_until IS NOT NULL;
CREATE INDEX idx_user_grants_principal_agent ON user_grants(principal, agent_id);
CREATE INDEX idx_user_grants_created_at ON user_grants(created_at);

-- GIN index for JSON querying on delegated_tokens
CREATE INDEX idx_user_grants_delegated_tokens ON user_grants USING gin(delegated_tokens);

-- ============================================================================
-- Function: update_updated_at_column
-- Description: Automatically updates updated_at timestamp on row modification
-- ============================================================================
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Triggers for automatic updated_at
CREATE TRIGGER update_agents_updated_at
    BEFORE UPDATE ON agents
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_thirdparty_oauth2_services_updated_at
    BEFORE UPDATE ON thirdparty_oauth2_services
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

CREATE TRIGGER update_user_grants_updated_at
    BEFORE UPDATE ON user_grants
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- ============================================================================
-- Schema version tracking
-- ============================================================================
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    INTEGER PRIMARY KEY,
    applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO schema_migrations (version) VALUES (1);
```

**File**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/migrations/001_domain_model.down.sql`

```sql
-- Rollback migration 001: Domain Model and Consent APIs

DROP TRIGGER IF EXISTS update_user_grants_updated_at ON user_grants;
DROP TRIGGER IF EXISTS update_thirdparty_oauth2_services_updated_at ON thirdparty_oauth2_services;
DROP TRIGGER IF EXISTS update_agents_updated_at ON agents;

DROP FUNCTION IF EXISTS update_updated_at_column();

DROP TABLE IF EXISTS user_grants;
DROP TABLE IF EXISTS thirdparty_oauth2_services;
DROP TABLE IF EXISTS agents;

DELETE FROM schema_migrations WHERE version = 1;
```

### Encryption Port Interface

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/encryption.go`

```go
package ports

// EncryptionPort defines the interface for encrypting/decrypting sensitive data.
// Implementations can range from no-op (development) to AES-256-GCM (production)
// to external KMS (cloud deployments).
type EncryptionPort interface {
	// Encrypt encrypts plaintext with optional encryption context.
	// encryptionContext provides additional authenticated data for AEAD ciphers.
	// Returns encrypted bytes or error if encryption fails.
	Encrypt(plaintext string, encryptionContext map[string]string) ([]byte, error)

	// Decrypt decrypts ciphertext with optional encryption context.
	// encryptionContext must match the context used during encryption for AEAD ciphers.
	// Returns decrypted plaintext or error if decryption fails.
	Decrypt(ciphertext []byte, encryptionContext map[string]string) (string, error)
}
```

### No-Op Encryption Adapter

**Location**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/storage/noop/encryption.go`

```go
package noop

// NoOpEncryption implements EncryptionPort by storing plaintext.
// Suitable for development and testing environments only.
// WARNING: Do not use in production - secrets are stored unencrypted.
type NoOpEncryption struct{}

func NewNoOpEncryption() *NoOpEncryption {
	return &NoOpEncryption{}
}

func (n *NoOpEncryption) Encrypt(plaintext string, encryptionContext map[string]string) ([]byte, error) {
	// Store plaintext as bytes (no actual encryption)
	return []byte(plaintext), nil
}

func (n *NoOpEncryption) Decrypt(ciphertext []byte, encryptionContext map[string]string) (string, error) {
	// Return bytes as plaintext string
	return string(ciphertext), nil
}
```

### Usage in PostgreSQL Adapter

```go
// In postgres adapter when storing client secret:
encryptionContext := map[string]string{
	"service_id": service.ID,
	"purpose":    "oauth2_client_secret",
}

encrypted, err := a.encryption.Encrypt(service.ClientSecret, encryptionContext)
if err != nil {
	return storage.NewStorageError("CreateService", storage.ErrorKindUnknown, err, "failed to encrypt secret")
}

// Store encrypted bytes
query := `INSERT INTO thirdparty_oauth2_services (..., client_secret, ...) VALUES (..., $1, ...)`
_, err = a.db.ExecContext(ctx, query, ..., encrypted, ...)
```

---

## Directory Structure

```
/Users/magnus.jungsbluth/Projects/agentic-identity-broker/

├── internal/
│   ├── domain/
│   │   ├── consent/                    # NEW: Consent service domain logic
│   │   │   ├── service.go              # Consent service implementation
│   │   │   ├── service_test.go         # Unit tests for consent service
│   │   │   └── types.go                # Request/response types
│   │   └── storage/
│   │       └── (entities defined here)
│   │
│   ├── ports/
│   │   ├── agent.go                    # NEW: Agent entity + repository interface
│   │   ├── thirdparty_oauth2_service.go # NEW: Service entity + repository interface
│   │   ├── user_grant.go               # NEW: UserGrant entity + repository interface
│   │   ├── encryption.go               # NEW: EncryptionPort interface
│   │   └── storage.go                  # EXTEND: Add new repository interfaces
│   │
│   ├── adapters/
│   │   ├── storage/
│   │   │   ├── memory/
│   │   │   │   ├── agent.go            # NEW: In-memory agent repository
│   │   │   │   ├── agent_test.go       # NEW: Agent repository tests
│   │   │   │   ├── thirdparty_oauth2_service.go  # NEW: In-memory service repository
│   │   │   │   ├── thirdparty_oauth2_service_test.go
│   │   │   │   ├── user_grant.go       # NEW: In-memory grant repository
│   │   │   │   ├── user_grant_test.go
│   │   │   │   └── adapter.go          # EXTEND: Add repository accessors
│   │   │   │
│   │   │   └── postgres/
│   │   │       ├── agent.go            # NEW: PostgreSQL agent repository
│   │   │       ├── agent_test.go       # NEW: Agent repository integration tests
│   │   │       ├── thirdparty_oauth2_service.go  # NEW: PostgreSQL service repository
│   │   │       ├── thirdparty_oauth2_service_test.go
│   │   │       ├── user_grant.go       # NEW: PostgreSQL grant repository
│   │   │       ├── user_grant_test.go
│   │   │       └── adapter.go          # EXTEND: Add repository accessors
│   │   │
│   │   └── http/
│   │       ├── admin_agent_handlers.go        # NEW: Admin agent CRUD endpoints
│   │       ├── admin_agent_handlers_test.go
│   │       ├── admin_service_handlers.go      # NEW: Admin service CRUD endpoints
│   │       ├── admin_service_handlers_test.go
│   │       ├── consent_handlers.go            # NEW: User consent endpoints
│   │       ├── consent_handlers_test.go
│   │       └── server.go                      # EXTEND: Add new route groups
│   │
│   └── config/
│       └── schema.go                   # EXTEND: Add encryption_key config
│
├── migrations/
│   ├── 001_domain_model.up.sql        # NEW: Create domain tables
│   └── 001_domain_model.down.sql      # NEW: Rollback domain tables
│
└── specs/
    └── 20251217-120000-domain-model-apis/
        ├── spec.md                     # Feature specification
        ├── implementation-plan.md      # This document
        └── plan.md                     # EXTEND: Add implementation tasks
```

---

## Implementation Phases

### Phase 1: Domain Entities and Interfaces (Priority: P1)

**Estimated Time**: 2-3 hours

**Tasks**:
1. Define Agent entity in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/agent.go`
2. Define ThirdpartyOAuth2Service entity in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/thirdparty_oauth2_service.go`
3. Define UserGrant entity in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/user_grant.go`
4. Add repository interfaces to `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/ports/storage.go`
5. Write unit tests for entity validation methods

**Success Criteria**:
- All entity structs compile with proper validation
- Repository interfaces follow ISP (one per entity)
- Unit tests achieve 95%+ coverage on validation logic

### Phase 2: Database Schema and Encryption (Priority: P1)

**Estimated Time**: 2-3 hours

**Tasks**:
1. Create migration files (`001_domain_model.up.sql`, `001_domain_model.down.sql`)
2. Implement encryption in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/domain/storage/encryption.go`
3. Add encryption_key configuration field
4. Write encryption unit tests with test keys
5. Test migrations locally with PostgreSQL

**Success Criteria**:
- Migration applies cleanly on fresh database
- Migration rolls back cleanly
- Encryption/decryption tests pass with AES-256-GCM
- Client secrets properly encrypted/decrypted

### Phase 3: In-Memory Adapter Implementation (Priority: P1)

**Estimated Time**: 4-5 hours

**Tasks**:
1. Implement AgentRepository in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/storage/memory/agent.go`
2. Implement ThirdpartyOAuth2ServiceRepository in `memory/thirdparty_oauth2_service.go`
3. Implement UserGrantRepository in `memory/user_grant.go`
4. Update `memory/adapter.go` to expose new repositories
5. Write table-driven unit tests for all repositories
6. Test CASCADE deletion (agent → grants)
7. Test BLOCK deletion (service with active grants)

**Success Criteria**:
- All CRUD operations work in-memory
- Concurrency-safe with sync.RWMutex
- Data copying prevents external mutation
- Unit tests achieve 90%+ coverage

### Phase 4: PostgreSQL Adapter Implementation (Priority: P1)

**Estimated Time**: 6-8 hours

**Tasks**:
1. Implement AgentRepository in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/storage/postgres/agent.go`
2. Implement ThirdpartyOAuth2ServiceRepository in `postgres/thirdparty_oauth2_service.go`
   - Encrypt client_secret on write
   - Decrypt client_secret on read
3. Implement UserGrantRepository in `postgres/user_grant.go`
   - JSONB handling for delegated_tokens
4. Update `postgres/adapter.go` to expose new repositories
5. Write testcontainers integration tests for all repositories
6. Test foreign key constraints
7. Test CASCADE and BLOCK deletion behavior

**Success Criteria**:
- All CRUD operations work with PostgreSQL
- Client secrets encrypted at rest
- JSONB columns properly serialized/deserialized
- Integration tests pass with real PostgreSQL
- Foreign key constraints enforced

### Phase 5: Consent Service Implementation (Priority: P2)

**Estimated Time**: 4-5 hours

**Tasks**:
1. Implement consent service in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/domain/consent/service.go`
2. Implement GetAgentConsentInfo (returns all services per FR-025)
3. Implement GrantConsent with scope validation
4. Implement RevokeConsent
5. Implement GetActiveGrants (filter expired grants)
6. Write comprehensive unit tests with mocked repositories

**Success Criteria**:
- Scope validation enforces service configurations
- One grant per user-agent pair (upsert semantics)
- Expired grants filtered correctly
- Unit tests achieve 95%+ coverage

### Phase 6: Admin API Endpoints (Priority: P2)

**Estimated Time**: 4-5 hours

**Tasks**:
1. Implement agent CRUD handlers in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/http/admin_agent_handlers.go`
   - POST /api/agents
   - GET /api/agents/:agent-id
   - PUT /api/agents/:agent-id
   - DELETE /api/agents/:agent-id
   - GET /api/agents (list)
2. Implement service CRUD handlers in `admin_service_handlers.go`
   - POST /api/third-party/oauth2/clients
   - GET /api/third-party/oauth2/clients/:client-id
   - PUT /api/third-party/oauth2/clients/:client-id
   - DELETE /api/third-party/oauth2/clients/:client-id
   - GET /api/third-party/oauth2/clients (list)
   - Redact client_secret in responses
3. Add route groups to `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/http/server.go`
4. Write HTTP handler tests

**Success Criteria**:
- All endpoints return proper HTTP status codes
- JSON request/response bodies validated
- Client secrets redacted in responses
- DELETE blocked if grants exist (for services)
- Validation errors return 400 with details

### Phase 7: User Consent API Endpoints (Priority: P3)

**Estimated Time**: 3-4 hours

**Tasks**:
1. Implement consent handlers in `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/internal/adapters/http/consent_handlers.go`
   - GET /api/consent/agent/:agent-id (agent info + services)
   - POST /api/consent/agent/:agent-id/grants (grant/update consent)
   - GET /api/consent/agent/:agent-id/grants (list user's grants)
2. Integrate with principal extraction middleware (20251216-120000-session-management)
3. Write HTTP handler tests with principal context

**Success Criteria**:
- Principal derived from session
- Only grant owner can view/modify grants
- Empty scopes array triggers revocation
- Expired grants excluded from listings

### Phase 8: Integration Testing and Documentation (Priority: P3)

**Estimated Time**: 3-4 hours

**Tasks**:
1. Write end-to-end integration tests covering full workflows
2. Test OAuth2 endpoint discovery (mock HTTP server)
3. Update ARCHITECTURE.md with domain model glossary
4. Update configuration examples with encryption_key
5. Document admin API endpoints
6. Document user consent API endpoints
7. Add troubleshooting guide for common errors

**Success Criteria**:
- Full consent workflow tested end-to-end
- OAuth2 discovery tested with mock metadata
- Documentation complete and accurate
- Configuration examples include all new fields

---

## Testing Strategy

### Unit Tests

**Test Coverage Goals**: 90%+ overall, 95%+ for domain logic

**Key Areas**:
1. **Entity Validation**: Table-driven tests for all validation rules
2. **Repository Operations**: CRUD operations for memory adapter
3. **Encryption**: Encrypt/decrypt roundtrip with various inputs
4. **Consent Service**: Scope validation, grant upsert, expiration filtering

**Example Test Pattern** (table-driven):

```go
func TestAgent_Validate(t *testing.T) {
	tests := []struct {
		name    string
		agent   *ports.Agent
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid agent",
			agent: &ports.Agent{
				ID:          "agent-1",
				ClientID:    "client-1",
				DisplayName: "Test Agent",
				Description: "Test description",
			},
			wantErr: false,
		},
		{
			name: "missing client_id",
			agent: &ports.Agent{
				ID:          "agent-1",
				DisplayName: "Test Agent",
				Description: "Test description",
			},
			wantErr: true,
			errMsg:  "client_id cannot be empty",
		},
		{
			name: "invalid governance URL",
			agent: &ports.Agent{
				ID:            "agent-1",
				ClientID:      "client-1",
				DisplayName:   "Test Agent",
				Description:   "Test description",
				GovernanceURL: stringPtr("not-a-url"),
			},
			wantErr: true,
			errMsg:  "governance_url is not a valid URL",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.agent.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
```

### Integration Tests

**Test Coverage**: All PostgreSQL repository operations

**Key Areas**:
1. **Database Operations**: Full CRUD lifecycle with testcontainers
2. **Foreign Key Constraints**: Verify CASCADE and BLOCK behavior
3. **Encryption**: Client secrets encrypted at rest in database
4. **JSONB**: Complex queries on delegated_tokens

**Example Test Pattern** (testcontainers):

```go
func TestPostgres_AgentRepository(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Start PostgreSQL container
	ctx := context.Background()
	postgres, err := setupPostgresContainer(ctx)
	require.NoError(t, err)
	defer postgres.Terminate(ctx)

	// Run migrations
	err = runMigrations(ctx, postgres.ConnectionString())
	require.NoError(t, err)

	// Create adapter
	config := &ports.StorageConfig{
		Backend: "postgres",
		Postgres: ports.PostgresConfig{
			ConnectionURL: postgres.ConnectionString(),
		},
	}
	adapter, err := NewAdapter(config)
	require.NoError(t, err)

	err = adapter.Initialize(ctx)
	require.NoError(t, err)
	defer adapter.Close(ctx)

	// Test CRUD operations
	agent := &ports.Agent{
		ID:          uuid.New().String(),
		ClientID:    "client-1",
		DisplayName: "Test Agent",
		Description: "Test description",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}

	// Create
	err = adapter.CreateAgent(ctx, agent)
	require.NoError(t, err)

	// Read
	retrieved, err := adapter.GetAgent(ctx, agent.ID)
	require.NoError(t, err)
	assert.Equal(t, agent.DisplayName, retrieved.DisplayName)

	// Update
	retrieved.Description = "Updated description"
	err = adapter.UpdateAgent(ctx, retrieved)
	require.NoError(t, err)

	// Delete
	err = adapter.DeleteAgent(ctx, agent.ID)
	require.NoError(t, err)

	// Verify deletion
	_, err = adapter.GetAgent(ctx, agent.ID)
	assert.Error(t, err)
}
```

### End-to-End Tests

**Test Coverage**: Full consent workflow

**Key Scenarios**:
1. Admin creates agent and service
2. User views agent consent info
3. User grants consent with scopes
4. User retrieves active grants
5. User modifies granted scopes
6. User revokes consent
7. Admin attempts to delete service (blocked if grants exist)
8. Admin deletes agent (cascades to grants)

---

## Security Considerations

### SR-002: Client Secret Encryption at Rest

**Implementation**:
- AES-256-GCM encryption using `crypto/cipher`
- Encryption key loaded from environment variable (32 bytes)
- Key stored in secure configuration (not committed to git)
- Automatic encryption on write, decryption on read

**Implementation note (superseded configuration)**:
This snippet predates the backend-explicit encryption contract.
Use `encryption.aws_kms` or `encryption.memory` for live configuration.

**Configuration**:
```yaml
storage:
  postgres:
    encryption_key: "${IDENTITY_BROKER_ENCRYPTION_KEY}"  # 32-byte hex string
```

**Key Rotation Strategy**:
- Re-encrypt all secrets when key changes
- Migration script to rotate keys without downtime
- Audit log all key rotation events

### SR-003: Client Secret Redaction

**Implementation**:
- `RedactedCopy()` method on ThirdpartyOAuth2Service
- All GET/LIST API responses use redacted copy
- Client secret never appears in logs or audit trails

### SR-004: Grant Authorization

**Implementation**:
- Principal extracted from session (20251216-120000-session-management)
- All grant operations filtered by session principal
- No cross-user grant access possible

### SR-007 & SR-008: Audit Logging

**Implementation**:
- Structured JSON logs for all CRUD operations
- Include: operation, principal, entity_id, timestamp, result
- Admin operations logged at INFO level
- Grant operations logged at INFO level

**Example Log**:
```json
{
  "timestamp": "2025-12-17T10:30:00Z",
  "level": "info",
  "operation": "CreateAgent",
  "principal": "admin@example.com",
  "agent_id": "agent-123",
  "result": "success"
}
```

### SR-011: URL Validation

**Implementation**:
- `isValidURL()` helper function validates all URLs
- Reject URLs with:
  - Invalid scheme (only https:// allowed in production)
  - JavaScript protocol (`javascript:`)
  - Data URLs (`data:`)
  - File URLs (`file://`)
- Use `url.Parse()` from standard library

```go
func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	
	// Only allow http/https schemes
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	
	// Reject empty host
	if u.Host == "" {
		return false
	}
	
	return true
}
```

---

## Key Implementation Patterns

### Pattern 1: Interface Segregation Principle (ISP)

**Principle**: Each entity gets its own focused repository interface.

**Benefits**:
- Smaller, testable interfaces
- Clear dependencies
- Easy to mock for testing
- No "God interface" with all methods

**Example**:
```go
// GOOD: Small, focused interfaces
type AgentRepository interface {
	CreateAgent(ctx context.Context, agent *Agent) error
	GetAgent(ctx context.Context, id string) (*Agent, error)
	UpdateAgent(ctx context.Context, agent *Agent) error
	DeleteAgent(ctx context.Context, id string) error
	ListAgents(ctx context.Context) ([]*Agent, error)
}

// BAD: Monolithic interface
type StoragePort interface {
	CreateAgent(...) error
	GetAgent(...) error
	CreateService(...) error
	GetService(...) error
	CreateGrant(...) error
	GetGrant(...) error
	// ... 20+ more methods
}
```

### Pattern 2: Repository Accessor Pattern

**Principle**: Adapter struct exposes repositories via accessor methods.

**Benefits**:
- Lazy initialization possible
- Clear dependency injection
- Easy to swap implementations

**Example**:
```go
type Adapter struct {
	db             *sqlx.DB
	agentRepo      *AgentRepositoryImpl
	serviceRepo    *ServiceRepositoryImpl
	grantRepo      *GrantRepositoryImpl
}

func (a *Adapter) Agents() ports.AgentRepository {
	return a.agentRepo
}

func (a *Adapter) Services() ports.ThirdpartyOAuth2ServiceRepository {
	return a.serviceRepo
}

func (a *Adapter) Grants() ports.UserGrantRepository {
	return a.grantRepo
}
```

### Pattern 3: Error Wrapping with StorageError

**Principle**: All adapter errors wrapped in domain StorageError.

**Benefits**:
- Hides implementation details (SQL errors, connection strings)
- Consistent error handling across adapters
- Error classification via ErrorKind

**Example**:
```go
// BAD: Exposes SQL error
return nil, err

// GOOD: Wraps in StorageError
return nil, storage.NewStorageError(
	"CreateAgent",
	storage.ErrorKindConflict,
	err,
	fmt.Sprintf("agent with ID %q already exists", agent.ID),
)
```

### Pattern 4: Data Copying (In-Memory Adapter)

**Principle**: Always copy data in/out to prevent external mutation.

**Benefits**:
- Prevents accidental data corruption
- Thread-safe even if caller mutates returned data
- Explicit ownership transfer

**Example**:
```go
// BAD: Direct reference
a.agents[agent.ID] = agent
return a.agents[id], nil

// GOOD: Copy in and out
agentCopy := *agent
a.agents[agent.ID] = &agentCopy

userCopy := *a.agents[id]
return &userCopy, nil
```

### Pattern 5: Context Propagation

**Principle**: Always pass context.Context for cancellation/timeout.

**Benefits**:
- Request-scoped timeouts
- Graceful cancellation
- Distributed tracing support

**Example**:
```go
func (s *Service) GrantConsent(ctx context.Context, req *GrantConsentRequest) (*UserGrant, error) {
	// Context flows through all operations
	agent, err := s.agentRepo.GetAgent(ctx, req.AgentID)
	if err != nil {
		return nil, err
	}
	
	// Context respects cancellation
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	
	// ...
}
```

### Pattern 6: JSONB Handling in PostgreSQL

**Principle**: Use sqlx with explicit JSONB marshaling.

**Implementation**:
```go
// Writing JSONB
scopesJSON, err := json.Marshal(service.Scopes)
if err != nil {
	return storage.NewStorageError("CreateService", storage.ErrorKindValidation, err, "failed to marshal scopes")
}

query := `INSERT INTO thirdparty_oauth2_services (id, scopes) VALUES ($1, $2)`
_, err = a.db.ExecContext(ctx, query, service.ID, scopesJSON)

// Reading JSONB
var scopesJSON []byte
query := `SELECT scopes FROM thirdparty_oauth2_services WHERE id = $1`
err := a.db.QueryRowContext(ctx, query, id).Scan(&scopesJSON)

var scopes []ports.ScopeDefinition
err = json.Unmarshal(scopesJSON, &scopes)
```

### Pattern 7: Foreign Key Enforcement

**Principle**: Use database constraints + explicit checks.

**Implementation**:
```go
// Delete with referential integrity check
func (a *Adapter) DeleteService(ctx context.Context, id string) error {
	// Check for active grants
	count, err := a.CountGrantsReferencingService(ctx, id)
	if err != nil {
		return err
	}
	
	if count > 0 {
		return storage.NewStorageError(
			"DeleteService",
			storage.ErrorKindConflict,
			nil,
			fmt.Sprintf("cannot delete service: %d active grants reference it", count),
		)
	}
	
	// Safe to delete
	_, err = a.db.ExecContext(ctx, "DELETE FROM thirdparty_oauth2_services WHERE id = $1", id)
	return err
}
```

---

## Summary

This implementation plan provides a complete, production-ready design for the Domain Model and Consent APIs feature. The plan follows established patterns from feature 20251215-120100-persistence-layer, using hexagonal architecture with clear separation of concerns.

**Key Deliverables**:
1. Three domain entities (Agent, ThirdpartyOAuth2Service, UserGrant)
2. Three repository interfaces (one per entity, following ISP)
3. Full CRUD implementations for memory and PostgreSQL adapters
4. Consent service with business logic and scope validation
5. Complete database schema with encryption and foreign key constraints
6. Admin and user API endpoints with proper authentication
7. Comprehensive testing strategy (unit + integration)
8. Security-first design (encryption, redaction, audit logging)

**Total Estimated Time**: 28-37 hours (4-5 working days)

**Next Steps**:
1. Review and approve this plan
2. Create feature branch `20251217-120000-domain-model-apis`
3. Implement phases sequentially (P1 → P2 → P3)
4. Run `just check` before each commit
5. Submit PR with full test coverage and documentation
