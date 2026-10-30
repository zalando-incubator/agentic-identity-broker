# Data Model: Consent Management Frontend

**Feature**: 20251217-120100-consent-frontend
**Date**: 2025-12-18
**Status**: Design
**Constitution Version**: 1.2.0

## Overview

This document defines the data structures used in the consent management frontend feature, including both frontend TypeScript types and backend Go structs. The data model enables users to view and manage their consent delegations to agents with fine-grained service and scope control.

## Design Principles

1. **Type Safety**: All data structures use strong typing (TypeScript on frontend, Go structs on backend)
2. **Nullability Clarity**: Optional fields explicitly marked with `?` (TypeScript) or pointers (Go)
3. **API Consistency**: Frontend types mirror backend API response structures
4. **Domain Alignment**: Types align with domain entities from 20251217-120000-domain-model-apis
5. **Serialization**: Go structs use JSON tags matching frontend expectations

---

## Part I: Frontend Data Model (TypeScript)

### Location
`web/src/types/consent.ts`

### Core Domain Types

#### User Information

```typescript
/**
 * Current authenticated user information.
 * Returned by GET /api/me
 */
export interface UserInfo {
  /** User's principal identifier (e.g., email, UUID) */
  principal: string;

  /** Human-readable display name */
  displayName: string;

  /** URL to user's profile picture/avatar */
  pictureUrl?: string;
}
```

#### Agent Delegation Summary

```typescript
/**
 * Summary of an agent the user has delegated access to.
 * Returned by GET /api/consent/agents
 */
export interface AgentDelegation {
  /** Unique agent identifier */
  agentId: string;

  /** Agent's display name shown to users */
  displayName: string;

  /** URL to agent's logo/avatar */
  logoUrl?: string;

  /** Number of active service grants for this agent */
  activeGrantCount: number;

  /** ISO 8601 timestamp of last grant modification */
  lastModifiedAt: string;

  /** Optional grant expiration (null = indefinite) */
  expiresAt?: string | null;
}
```

#### Agent Detail

```typescript
/**
 * Detailed agent information for grant management page.
 * Returned by GET /api/consent/agent/:agent-id
 */
export interface AgentDetail {
  /** Unique agent identifier */
  agentId: string;

  /** Agent's display name */
  displayName: string;

  /** Agent description/purpose */
  description: string;

  /** URL to agent's logo/avatar */
  logoUrl?: string;

  /** Link to agent governance documentation */
  governanceUrl?: string;

  /** Link to user-facing agent documentation */
  userDocumentationUrl?: string;

  /** Link to agent's public interface (if applicable) */
  agentInterfaceUrl?: string;
}
```

#### Third-Party Service

```typescript
/**
 * External OAuth2 service that can be delegated to agents.
 * Part of GET /api/consent/agent/:agent-id response.
 */
export interface ThirdpartyService {
  /** Unique service identifier */
  serviceId: string;

  /** Service display name (e.g., "Google Drive", "GitHub") */
  displayName: string;

  /** URL to service logo */
  logoUrl?: string;

  /** Available OAuth2 scopes for this service */
  scopes: ServiceScope[];
}
```

#### Service Scope

```typescript
/**
 * OAuth2 scope within a third-party service.
 */
export interface ServiceScope {
  /** OAuth2 scope value (e.g., "read:email") */
  value: string;

  /** Human-readable scope description */
  description: string;
}
```

#### User Grant

```typescript
/**
 * User's existing grant to an agent.
 * Returned by GET /api/consent/agent/:agent-id/grants
 */
export interface UserGrant {
  /** Unique grant identifier */
  grantId: string;

  /** Agent receiving the grant */
  agentId: string;

  /** User principal who created the grant */
  principal: string;

  /** Delegated service access tokens */
  delegatedTokens: DelegatedToken[];

  /** Optional expiration timestamp (null = indefinite) */
  validUntil?: string | null;

  /** ISO 8601 timestamp of grant creation */
  createdAt: string;

  /** ISO 8601 timestamp of last update */
  updatedAt: string;
}
```

#### Delegated Token

```typescript
/**
 * Service-specific delegation within a grant.
 */
export interface DelegatedToken {
  /** Third-party service identifier */
  serviceId: string;

  /** OAuth2 scopes granted for this service */
  scopes: string[];
}
```

### API Request/Response Types

#### GET /api/me Response

```typescript
export interface GetUserInfoResponse {
  data: UserInfo;
}
```

#### GET /api/consent/agents Response

```typescript
export interface GetAgentDelegationsResponse {
  data: AgentDelegation[];
}
```

#### GET /api/consent/agent/:agent-id Response

```typescript
export interface GetAgentDetailResponse {
  data: {
    agent: AgentDetail;
    services: ThirdpartyService[];
  };
}
```

#### GET /api/consent/agent/:agent-id/grants Response

```typescript
export interface GetAgentGrantsResponse {
  data: UserGrant[];
}
```

#### POST /api/consent/agent/:agent-id/grants Request

```typescript
export interface CreateOrUpdateGrantRequest {
  /** Service delegations (empty array = revoke grant) */
  delegatedTokens: {
    serviceId: string;
    scopes: string[];
  }[];

  /** Optional expiration timestamp (omit for indefinite) */
  validUntil?: string | null;
}
```

#### POST /api/consent/agent/:agent-id/grants Response

```typescript
export interface CreateOrUpdateGrantResponse {
  data: UserGrant;
}
```

### Error Response Type

```typescript
/**
 * Standard error response from backend APIs.
 */
export interface ApiError {
  /** HTTP status code */
  status: number;

  /** Error code (e.g., "INVALID_AGENT_ID") */
  code: string;

  /** Human-readable error message */
  message: string;

  /** Optional field-level validation errors */
  details?: Record<string, string[]>;
}
```

### UI State Types

```typescript
/**
 * UI state for service grant toggle.
 */
export interface ServiceGrantState {
  serviceId: string;
  isEnabled: boolean;
  isExpanded: boolean;
  selectedScopes: Set<string>;
}

/**
 * Grant validity form state.
 */
export interface GrantValidityState {
  noExpiration: boolean;
  expiresAt?: Date;
}

/**
 * Loading state for async operations.
 */
export interface LoadingState {
  isLoading: boolean;
  error?: ApiError;
}
```

---

## Part II: Backend Data Model (Go)

### Location
- `internal/domain/consent.go` (domain types)
- `internal/adapters/http/dto/consent.go` (API DTOs)

### Domain Types

#### UserInfo

```go
package domain

// UserInfo represents the current authenticated user.
type UserInfo struct {
    Principal   string  `json:"principal"`
    DisplayName string  `json:"displayName"`
    PictureURL  *string `json:"pictureUrl,omitempty"`
}
```

#### AgentDelegation

```go
// AgentDelegation is a summary of an agent with active grants.
type AgentDelegation struct {
    AgentID          string     `json:"agentId"`
    DisplayName      string     `json:"displayName"`
    LogoURL          *string    `json:"logoUrl,omitempty"`
    ActiveGrantCount int        `json:"activeGrantCount"`
    LastModifiedAt   time.Time  `json:"lastModifiedAt"`
    ExpiresAt        *time.Time `json:"expiresAt,omitempty"`
}
```

#### AgentDetail

```go
// AgentDetail contains detailed agent information for grant management.
type AgentDetail struct {
    AgentID              string  `json:"agentId"`
    DisplayName          string  `json:"displayName"`
    Description          string  `json:"description"`
    LogoURL              *string `json:"logoUrl,omitempty"`
    GovernanceURL        *string `json:"governanceUrl,omitempty"`
    UserDocumentationURL *string `json:"userDocumentationUrl,omitempty"`
    AgentInterfaceURL    *string `json:"agentInterfaceUrl,omitempty"`
}
```

#### ThirdpartyService

```go
// ThirdpartyService represents an external OAuth2 provider.
type ThirdpartyService struct {
    ServiceID   string         `json:"serviceId"`
    DisplayName string         `json:"displayName"`
    LogoURL     *string        `json:"logoUrl,omitempty"`
    Scopes      []ServiceScope `json:"scopes"`
}
```

#### ServiceScope

```go
// ServiceScope is an OAuth2 scope within a service.
type ServiceScope struct {
    Value       string `json:"value"`
    Description string `json:"description"`
}
```

#### UserGrant

```go
// UserGrant represents a user's delegation to an agent.
type UserGrant struct {
    GrantID         string           `json:"grantId"`
    AgentID         string           `json:"agentId"`
    Principal       string           `json:"principal"`
    DelegatedTokens []DelegatedToken `json:"delegatedTokens"`
    ValidUntil      *time.Time       `json:"validUntil,omitempty"`
    CreatedAt       time.Time        `json:"createdAt"`
    UpdatedAt       time.Time        `json:"updatedAt"`
}
```

#### DelegatedToken

```go
// DelegatedToken is service-specific access within a grant.
type DelegatedToken struct {
    ServiceID string   `json:"serviceId"`
    Scopes    []string `json:"scopes"`
}
```

### API DTOs (Data Transfer Objects)

#### GetUserInfoResponse

```go
package dto

import "github.com/your-org/agentic-identity-broker/internal/domain"

type GetUserInfoResponse struct {
    Data domain.UserInfo `json:"data"`
}
```

#### GetAgentDelegationsResponse

```go
type GetAgentDelegationsResponse struct {
    Data []domain.AgentDelegation `json:"data"`
}
```

#### GetAgentDetailResponse

```go
type GetAgentDetailResponse struct {
    Data struct {
        Agent    domain.AgentDetail          `json:"agent"`
        Services []domain.ThirdpartyService  `json:"services"`
    } `json:"data"`
}
```

#### GetAgentGrantsResponse

```go
type GetAgentGrantsResponse struct {
    Data []domain.UserGrant `json:"data"`
}
```

#### CreateOrUpdateGrantRequest

```go
type CreateOrUpdateGrantRequest struct {
    DelegatedTokens []struct {
        ServiceID string   `json:"serviceId" validate:"required,uuid"`
        Scopes    []string `json:"scopes" validate:"required,min=1"`
    } `json:"delegatedTokens" validate:"required"`
    ValidUntil *time.Time `json:"validUntil,omitempty" validate:"omitempty,gtefield=Now"`
}
```

#### CreateOrUpdateGrantResponse

```go
type CreateOrUpdateGrantResponse struct {
    Data domain.UserGrant `json:"data"`
}
```

### Error Response

```go
// ErrorResponse is the standard error format for all APIs.
type ErrorResponse struct {
    Status  int                `json:"status"`
    Code    string             `json:"code"`
    Message string             `json:"message"`
    Details map[string][]string `json:"details,omitempty"`
}
```

---

## Data Flow Examples

### Example 1: Fetching Agent Delegations

**Frontend Request:**
```typescript
GET /api/consent/agents
Authorization: Bearer <session-token>
```

**Backend Response:**
```json
{
  "data": [
    {
      "agentId": "550e8400-e29b-41d4-a716-446655440000",
      "displayName": "Data Analysis Assistant",
      "logoUrl": "https://cdn.example.com/agents/data-assistant.png",
      "activeGrantCount": 3,
      "lastModifiedAt": "2025-12-17T14:30:00Z",
      "expiresAt": null
    },
    {
      "agentId": "660e8400-e29b-41d4-a716-446655440001",
      "displayName": "Document Processor",
      "logoUrl": "https://cdn.example.com/agents/doc-processor.png",
      "activeGrantCount": 2,
      "lastModifiedAt": "2025-12-16T09:15:00Z",
      "expiresAt": "2026-01-15T00:00:00Z"
    }
  ]
}
```

**Frontend Parsing:**
```typescript
const response = await apiClient.get<GetAgentDelegationsResponse>('/api/consent/agents');
const delegations: AgentDelegation[] = response.data;
```

### Example 2: Creating a Grant

**Frontend Request:**
```typescript
POST /api/consent/agent/550e8400-e29b-41d4-a716-446655440000/grants
Authorization: Bearer <session-token>
Content-Type: application/json

{
  "delegatedTokens": [
    {
      "serviceId": "google-drive-service-id",
      "scopes": ["read:files", "write:files"]
    },
    {
      "serviceId": "github-service-id",
      "scopes": ["read:repos", "write:repos", "read:org"]
    }
  ],
  "validUntil": "2026-06-01T00:00:00Z"
}
```

**Backend Response:**
```json
{
  "data": {
    "grantId": "770e8400-e29b-41d4-a716-446655440002",
    "agentId": "550e8400-e29b-41d4-a716-446655440000",
    "principal": "user@example.com",
    "delegatedTokens": [
      {
        "serviceId": "google-drive-service-id",
        "scopes": ["read:files", "write:files"]
      },
      {
        "serviceId": "github-service-id",
        "scopes": ["read:repos", "write:repos", "read:org"]
      }
    ],
    "validUntil": "2026-06-01T00:00:00Z",
    "createdAt": "2025-12-18T10:00:00Z",
    "updatedAt": "2025-12-18T10:00:00Z"
  }
}
```

### Example 3: Error Response

**Backend Error Response:**
```json
{
  "status": 400,
  "code": "INVALID_SCOPES",
  "message": "One or more requested scopes do not exist for the specified service",
  "details": {
    "delegatedTokens[1].scopes": [
      "Scope 'admin:everything' does not exist for service 'github-service-id'"
    ]
  }
}
```

---

## Validation Rules

### Frontend Validation

```typescript
/**
 * Validates grant creation request before sending to backend.
 */
export function validateGrantRequest(request: CreateOrUpdateGrantRequest): string[] {
  const errors: string[] = [];

  // Must have at least one service (or empty array to revoke)
  if (!request.delegatedTokens) {
    errors.push('delegatedTokens field is required');
  }

  // Each service must have at least one scope
  request.delegatedTokens?.forEach((token, index) => {
    if (!token.serviceId) {
      errors.push(`delegatedTokens[${index}].serviceId is required`);
    }
    if (!token.scopes || token.scopes.length === 0) {
      errors.push(`delegatedTokens[${index}].scopes must contain at least one scope`);
    }
  });

  // If expiration is set, must be in the future
  if (request.validUntil) {
    const expiryDate = new Date(request.validUntil);
    if (expiryDate <= new Date()) {
      errors.push('validUntil must be a future date');
    }
  }

  return errors;
}
```

### Backend Validation

```go
// ValidateGrantRequest validates the create/update grant request.
func ValidateGrantRequest(req *dto.CreateOrUpdateGrantRequest) error {
    if req.DelegatedTokens == nil {
        return errors.New("delegatedTokens field is required")
    }

    for i, token := range req.DelegatedTokens {
        if token.ServiceID == "" {
            return fmt.Errorf("delegatedTokens[%d].serviceId is required", i)
        }
        if len(token.Scopes) == 0 {
            return fmt.Errorf("delegatedTokens[%d].scopes must contain at least one scope", i)
        }
    }

    if req.ValidUntil != nil && req.ValidUntil.Before(time.Now()) {
        return errors.New("validUntil must be a future date")
    }

    return nil
}
```

---

## Type Safety Guarantees

### Compile-Time Guarantees (TypeScript)

```typescript
// Example: Type-safe API client
class ConsentApiClient {
  async getAgentDelegations(): Promise<AgentDelegation[]> {
    const response = await this.httpClient.get<GetAgentDelegationsResponse>(
      '/api/consent/agents'
    );
    return response.data; // Type: AgentDelegation[]
  }

  async createGrant(
    agentId: string,
    request: CreateOrUpdateGrantRequest
  ): Promise<UserGrant> {
    const response = await this.httpClient.post<CreateOrUpdateGrantResponse>(
      `/api/consent/agent/${agentId}/grants`,
      request
    );
    return response.data; // Type: UserGrant
  }
}
```

### Runtime Validation (Go)

```go
// Example: Validated handler with type-safe DTOs
func (h *ConsentHandler) CreateGrant(w http.ResponseWriter, r *http.Request) {
    var req dto.CreateOrUpdateGrantRequest

    // Parse and validate JSON
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, "INVALID_JSON", err.Error())
        return
    }

    // Validate request
    if err := dto.ValidateGrantRequest(&req); err != nil {
        h.writeError(w, http.StatusBadRequest, "VALIDATION_ERROR", err.Error())
        return
    }

    // Extract principal from session
    principal := session.GetPrincipal(r.Context())

    // Business logic
    grant, err := h.consentService.CreateGrant(r.Context(), principal, agentID, &req)
    if err != nil {
        h.writeError(w, http.StatusInternalServerError, "CREATE_FAILED", err.Error())
        return
    }

    // Respond with typed DTO
    h.writeJSON(w, http.StatusOK, dto.CreateOrUpdateGrantResponse{Data: grant})
}
```

---

## Summary

This data model provides:

1. **Type Safety**: Strong typing in both TypeScript and Go
2. **API Consistency**: Frontend types mirror backend responses
3. **Validation**: Client and server-side validation with clear error messages
4. **Nullability**: Explicit handling of optional fields (`?` in TypeScript, `*` in Go)
5. **Time Handling**: ISO 8601 strings in JSON, `time.Time` in Go
6. **Domain Alignment**: Types match entities from 20251217-120000-domain-model-apis

**Next Steps:**
- Define OpenAPI/JSON Schema contracts in `contracts/` directory
- Implement TypeScript types in `web/src/types/consent.ts`
- Implement Go domain types in `internal/domain/consent.go`
- Implement Go DTOs in `internal/adapters/http/dto/consent.go`
- Add validation logic in both frontend and backend
