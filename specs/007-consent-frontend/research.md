# Phase 0 Research Document: Consent Management Frontend

**Feature**: 007-consent-frontend
**Date**: 2025-12-18
**Status**: Research
**Constitution Version**: 1.2.0

> **Historical visual scope**: The aesthetic and frontend choices below record feature 007's original research, not a current constitutional aesthetic mandate. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

## Executive Summary

This document provides comprehensive Phase 0 research for implementing the consent management frontend feature. The implementation consists of:

1. **Frontend (React SPA)**: User interface for consent management built with React 18+, Tailwind CSS v4.0, Headless UI, and React Router, implementing progressive loading, inline error recovery, and grant expiration controls
2. **Backend (Go APIs)**: New API endpoints for listing consented agents and user info, plus SPA serving logic with History API support

All implementations must comply with constitution principles:
- **Principle II**: Follow accepted ADR patterns (ADR 003: Chi, ADR 004: Storage layer)
- **Principle VI**: Hexagonal architecture with port/adapter separation
- **Principle VII**: Unified configuration system
- **Principle VIII**: Test-driven development with automated tests
- **Principle IX**: Persistence patterns from quickstart.md

**Design Aesthetic**: "Refined Trust Architecture" with trust-deep blues, warm amber actions, serif headings (Crimson Pro), sans-serif body (Manrope), and deliberate micro-interactions.

## Table of Contents

### Part I: Frontend Research (React SPA)
1. [Frontend Project Structure](#1-frontend-project-structure)
2. [Frontend Build System](#2-frontend-build-system)
3. [Routing Strategy](#3-routing-strategy)
4. [Component Architecture](#4-component-architecture)
5. [Frontend API Integration](#5-frontend-api-integration)
6. [Frontend Implementation Patterns](#6-frontend-implementation-patterns)
7. [Frontend Testing Strategy](#7-frontend-testing-strategy)

### Part II: Backend Research (Go APIs)
8. [SPA Serving Strategy](#8-spa-serving-strategy)
9. [API Endpoint Design](#9-api-endpoint-design)
3. [Hexagonal Architecture Integration](#3-hexagonal-architecture-integration)
4. [Persistence Pattern Compliance](#4-persistence-pattern-compliance)
5. [Configuration Requirements](#5-configuration-requirements)
6. [Security Considerations](#6-security-considerations)
7. [Testing Strategy](#7-testing-strategy)
8. [Implementation Checklist](#8-implementation-checklist)

---

## 1. SPA Serving Strategy

### 1.1 Overview

The Go backend will serve a React SPA under the `/consent` path prefix using chi router's built-in file server capabilities. The implementation follows Go best practices for serving SPAs with client-side routing (History API support).

### 1.2 Directory Structure

```
/Users/magnus.jungsbluth/Projects/agentic-identity-broker/
├── web/
│   └── consent/              # React SPA source code
│       ├── src/
│       ├── public/
│       ├── package.json
│       └── vite.config.ts    # Build outputs to dist/
├── dist/
│   └── consent/              # Built SPA artifacts (served by Go)
│       ├── index.html        # Entry point (no cache)
│       ├── assets/
│       │   ├── index-[hash].js   # Immutable (with hash)
│       │   ├── index-[hash].css  # Immutable (with hash)
│       │   └── vendor-[hash].js  # Immutable (with hash)
│       └── fonts/
│           └── [font-files]  # Static assets
```

### 1.3 Go Implementation Pattern

#### 1.3.1 File Server Setup

Location: `internal/adapters/http/server.go` (extend existing `setupEnduserRoutes()`)

```go
// setupEnduserRoutes registers enduser API routes (consent management).
func (s *Server) setupEnduserRoutes() {
	// Existing consent API routes...
	if s.agentRepo == nil || s.serviceRepo == nil || s.grantRepo == nil {
		s.logger.Warn("Consent routes not registered - missing required repositories")
		return
	}

	consentService := consentservice.NewService(s.agentRepo, s.serviceRepo, s.grantRepo)

	s.router.Route("/api/consent", func(r chi.Router) {
		s.registerConsentRoutes(r, consentService)
	})

	// NEW: Serve React SPA under /consent prefix
	s.serveSPA("/consent", s.config.SPA.StaticFilesPath, s.logger)

	// NEW: User info endpoint
	s.router.Get("/api/me", s.handleGetUserInfo())
}

// serveSPA configures a file server for the React SPA with History API support.
func (s *Server) serveSPA(pathPrefix string, staticDir string, logger *slog.Logger) {
	// Verify static directory exists
	indexPath := filepath.Join(staticDir, "index.html")
	if _, err := os.Stat(indexPath); os.IsNotExist(err) {
		logger.Warn("SPA static files not found, routes not registered",
			"path", staticDir,
			"index", indexPath)
		return
	}

	// Create file server for static assets
	fileServer := http.FileServer(http.Dir(staticDir))

	// Mount under /consent with History API fallback
	s.router.Get(pathPrefix+"/*", func(w http.ResponseWriter, r *http.Request) {
		// Strip the path prefix to get the requested file path
		filePath := strings.TrimPrefix(r.URL.Path, pathPrefix)
		fullPath := filepath.Join(staticDir, filePath)

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			// File doesn't exist - serve index.html for client-side routing
			http.ServeFile(w, r, indexPath)
			return
		}

		// File exists - serve it with appropriate cache headers
		s.serveStaticFile(w, r, pathPrefix, fileServer)
	})

	logger.Info("SPA routes registered",
		"prefix", pathPrefix,
		"static_dir", staticDir)
}

// serveStaticFile serves a static file with appropriate cache headers.
func (s *Server) serveStaticFile(w http.ResponseWriter, r *http.Request, prefix string, fileServer http.Handler) {
	// Determine if this is a hashed asset (immutable)
	path := r.URL.Path
	isAsset := strings.Contains(path, "/assets/") ||
	           strings.Contains(path, "/fonts/") ||
	           strings.Contains(path, "/images/")

	// Assets with content hashes can be cached forever
	if isAsset {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else if strings.HasSuffix(path, ".html") {
		// HTML files should not be cached (for client-side routing updates)
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")
	} else {
		// Other files (fonts, manifests) - moderate caching
		w.Header().Set("Cache-Control", "public, max-age=3600")
	}

	// Set MIME types explicitly to prevent browser guessing
	setMIMEType(w, path)

	// Strip prefix and serve
	http.StripPrefix(prefix, fileServer).ServeHTTP(w, r)
}

// setMIMEType sets the Content-Type header based on file extension.
func setMIMEType(w http.ResponseWriter, path string) {
	ext := filepath.Ext(path)
	switch ext {
	case ".html":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".woff":
		w.Header().Set("Content-Type", "font/woff")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
	case ".ttf":
		w.Header().Set("Content-Type", "font/ttf")
	case ".eot":
		w.Header().Set("Content-Type", "application/vnd.ms-fontobject")
	}
}
```

### 1.4 Cache Strategy Summary

| File Type | Cache-Control | Rationale |
|-----------|---------------|-----------|
| `/assets/*` with hash | `public, max-age=31536000, immutable` | Content-hashed assets never change |
| `index.html` | `no-cache, no-store, must-revalidate` | Entry point must always be fresh for routing |
| Fonts, images | `public, max-age=3600` (1 hour) | Static but not hashed |

### 1.5 History API Support

The implementation provides full support for client-side routing:

1. **Direct URL access**: `/consent/agent/123` serves `index.html`, React Router handles routing
2. **Refresh support**: User can refresh at any SPA route without 404 errors
3. **Static assets**: CSS, JS, fonts served directly without fallback
4. **API routes**: `/api/*` unaffected by SPA fallback (processed by existing handlers)

### 1.6 Security Considerations for Static Files

- **Path traversal prevention**: `filepath.Join()` and `os.Stat()` prevent directory traversal attacks
- **MIME type enforcement**: Explicit `Content-Type` headers prevent MIME sniffing attacks
- **No authentication on static files**: `/consent/*` is public, actual protection is on API routes
- **Principal extraction**: OptionalPrincipalMiddleware still runs, allowing SPA to check authentication

---

## 2. API Endpoint Design

### 2.1 GET /api/consent/agents - List Agents with Active Grants

#### 2.1.1 Purpose
Return a list of agents that the authenticated user has granted permissions to, including grant summary information.

#### 2.1.2 Request Specification

**HTTP Method**: `GET`
**Path**: `/api/consent/agents`
**Authentication**: Required (principal from session)
**Query Parameters**: None

**Request Headers**:
```
X-Remote-User: alice@example.com  (principal header)
```

#### 2.1.3 Response Specification

**Success (200 OK)**:
```json
{
  "agents": [
    {
      "agent_id": "agent-123",
      "display_name": "Research Assistant",
      "description": "AI agent for academic research",
      "governance_url": "https://example.com/governance",
      "user_documentation_url": "https://example.com/docs",
      "agent_interface_url": "https://agent.example.com",
      "grant_summary": {
        "grant_id": "grant-456",
        "granted_at": "2025-12-01T10:00:00Z",
        "expires_at": "2026-01-01T00:00:00Z",  // null if indefinite
        "service_count": 2,
        "services": [
          {
            "service_id": "svc-789",
            "display_name": "GitHub",
            "scope_count": 3
          },
          {
            "service_id": "svc-101",
            "display_name": "Google Drive",
            "scope_count": 2
          }
        ]
      }
    }
  ]
}
```

**Error Responses**:
- `401 Unauthorized`: Principal not found in session
- `500 Internal Server Error`: Database or service error

#### 2.1.4 Handler Implementation

Location: `internal/adapters/http/handlers/consent/agents_list_handler.go` (NEW FILE)

```go
package consent

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/consent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// AgentsListHandler handles GET /api/consent/agents
type AgentsListHandler struct {
	consentService *consent.Service
	logger         *slog.Logger
}

// NewAgentsListHandler creates a new agents list handler.
func NewAgentsListHandler(consentService *consent.Service, logger *slog.Logger) *AgentsListHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &AgentsListHandler{
		consentService: consentService,
		logger:         logger,
	}
}

// ServiceSummary represents a service in the grant summary.
type ServiceSummary struct {
	ServiceID   string `json:"service_id"`
	DisplayName string `json:"display_name"`
	ScopeCount  int    `json:"scope_count"`
}

// GrantSummary represents grant information for an agent.
type GrantSummary struct {
	GrantID      string           `json:"grant_id"`
	GrantedAt    string           `json:"granted_at"`
	ExpiresAt    *string          `json:"expires_at,omitempty"`
	ServiceCount int              `json:"service_count"`
	Services     []ServiceSummary `json:"services"`
}

// AgentWithGrant represents an agent with grant summary.
type AgentWithGrant struct {
	AgentID              string        `json:"agent_id"`
	DisplayName          string        `json:"display_name"`
	Description          string        `json:"description"`
	GovernanceURL        *string       `json:"governance_url,omitempty"`
	UserDocumentationURL *string       `json:"user_documentation_url,omitempty"`
	AgentInterfaceURL    *string       `json:"agent_interface_url,omitempty"`
	GrantSummary         *GrantSummary `json:"grant_summary"`
}

// AgentsListResponse is the response body.
type AgentsListResponse struct {
	Agents []AgentWithGrant `json:"agents"`
}

// GetAgentsWithGrants handles GET /api/consent/agents
func (h *AgentsListHandler) GetAgentsWithGrants(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from session
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	// Call service to get agents with grants
	agents, err := h.consentService.ListAgentsWithGrants(ctx, principalValue)
	if err != nil {
		h.logger.Error("failed to list agents with grants",
			"principal", principalValue,
			"error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	// Convert to response format
	response := AgentsListResponse{
		Agents: make([]AgentWithGrant, len(agents)),
	}

	for i, agentGrant := range agents {
		response.Agents[i] = h.toAgentWithGrant(agentGrant)
	}

	h.logger.Info("agents with grants retrieved",
		"principal", principalValue,
		"count", len(response.Agents))

	h.writeJSON(w, http.StatusOK, response)
}

// toAgentWithGrant converts domain model to response format.
func (h *AgentsListHandler) toAgentWithGrant(ag *consent.AgentWithGrant) AgentWithGrant {
	result := AgentWithGrant{
		AgentID:              ag.Agent.ID,
		DisplayName:          ag.Agent.DisplayName,
		Description:          ag.Agent.Description,
		GovernanceURL:        ag.Agent.GovernanceURL,
		UserDocumentationURL: ag.Agent.UserDocumentationURL,
		AgentInterfaceURL:    ag.Agent.AgentInterfaceURL,
	}

	if ag.Grant != nil {
		grantedAt := ag.Grant.CreatedAt.Format("2006-01-02T15:04:05Z07:00")

		var expiresAt *string
		if ag.Grant.ValidUntil != nil {
			expires := ag.Grant.ValidUntil.Format("2006-01-02T15:04:05Z07:00")
			expiresAt = &expires
		}

		// Build service summaries
		services := make([]ServiceSummary, len(ag.ServiceDetails))
		for i, svc := range ag.ServiceDetails {
			services[i] = ServiceSummary{
				ServiceID:   svc.ID,
				DisplayName: svc.DisplayName,
				ScopeCount:  svc.ScopeCount,
			}
		}

		result.GrantSummary = &GrantSummary{
			GrantID:      ag.Grant.ID,
			GrantedAt:    grantedAt,
			ExpiresAt:    expiresAt,
			ServiceCount: len(services),
			Services:     services,
		}
	}

	return result
}

// writeJSON writes a JSON response.
func (h *AgentsListHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *AgentsListHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
```

#### 2.1.5 Service Layer Method

Location: `internal/domain/consent/service.go` (EXTEND EXISTING)

```go
// AgentWithGrant combines agent metadata with grant information.
type AgentWithGrant struct {
	Agent          *storage.Agent
	Grant          *storage.UserGrant       // nil if no active grant
	ServiceDetails []ServiceDetail
}

// ServiceDetail provides summary info for a delegated service.
type ServiceDetail struct {
	ID          string
	DisplayName string
	ScopeCount  int
}

// ListAgentsWithGrants returns all agents that the user has granted permissions to.
// Only includes agents with active (non-expired) grants.
// Per FR-019: Expired grants are filtered out.
func (s *Service) ListAgentsWithGrants(ctx context.Context, principal string) ([]*AgentWithGrant, error) {
	// Strategy:
	// 1. Query all grants for this principal (filtering expired)
	// 2. For each grant, fetch agent details
	// 3. For each grant, fetch service details for delegated services
	// 4. Build AgentWithGrant objects

	// Get all active grants for this principal
	// This requires a new repository method: ListByPrincipal
	grants, err := s.grantRepo.ListByPrincipal(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	// Build result list
	result := make([]*AgentWithGrant, 0, len(grants))

	for _, grant := range grants {
		// Skip expired grants (FR-019)
		if !grant.IsActive() {
			continue
		}

		// Fetch agent
		agent, err := s.agentRepo.Get(ctx, grant.AgentID)
		if err != nil {
			// If agent deleted but grant exists, skip (shouldn't happen with cascade delete)
			s.logger.Warn("grant references non-existent agent",
				"grant_id", grant.ID,
				"agent_id", grant.AgentID)
			continue
		}

		// Build service details
		serviceDetails := make([]ServiceDetail, 0, len(grant.DelegatedOAuth2Tokens))
		for _, token := range grant.DelegatedOAuth2Tokens {
			service, err := s.serviceRepo.Get(ctx, token.ThirdpartyOAuth2ServiceID)
			if err != nil {
				// Service may have been deleted, log and skip
				s.logger.Warn("grant references non-existent service",
					"grant_id", grant.ID,
					"service_id", token.ThirdpartyOAuth2ServiceID)
				continue
			}

			serviceDetails = append(serviceDetails, ServiceDetail{
				ID:          service.ID,
				DisplayName: service.DisplayName,
				ScopeCount:  len(token.Scopes),
			})
		}

		result = append(result, &AgentWithGrant{
			Agent:          agent,
			Grant:          grant,
			ServiceDetails: serviceDetails,
		})
	}

	return result, nil
}
```

### 2.2 GET /api/me - Get Current User Info

#### 2.2.1 Purpose
Return information about the currently authenticated user. This endpoint provides the frontend with the user's identity for display purposes.

#### 2.2.2 Request Specification

**HTTP Method**: `GET`
**Path**: `/api/me`
**Authentication**: Required (principal from session)
**Query Parameters**: None

**Request Headers**:
```
X-Remote-User: alice@example.com
```

#### 2.2.3 Response Specification

**Success (200 OK)**:
```json
{
  "principal": "alice@example.com",
  "display_name": "Alice Smith",
  "picture_url": "https://ui-avatars.com/api/?name=Alice+Smith&background=random"
}
```

**Error Responses**:
- `401 Unauthorized`: Principal not found in session
- `500 Internal Server Error`: Service error

#### 2.2.4 Handler Implementation

Location: `internal/adapters/http/handlers/user/me_handler.go` (NEW FILE)

```go
package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

// MeHandler handles GET /api/me
type MeHandler struct {
	logger *slog.Logger
}

// NewMeHandler creates a new user info handler.
func NewMeHandler(logger *slog.Logger) *MeHandler {
	if logger == nil {
		logger = slog.Default()
	}
	return &MeHandler{
		logger: logger,
	}
}

// UserInfo represents user information response.
type UserInfo struct {
	Principal   string `json:"principal"`
	DisplayName string `json:"display_name"`
	PictureURL  string `json:"picture_url"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

// GetUserInfo handles GET /api/me
func (h *MeHandler) GetUserInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Extract principal from session
	principalValue, ok := principal.FromContext(ctx)
	if !ok || principalValue == "" {
		h.logger.Warn("principal not found in context")
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	// Generate display name from principal
	// For now, use static generation. Future: query user profile service
	displayName := h.generateDisplayName(principalValue)
	pictureURL := h.generatePictureURL(displayName)

	response := UserInfo{
		Principal:   principalValue,
		DisplayName: displayName,
		PictureURL:  pictureURL,
	}

	h.logger.Debug("user info retrieved",
		"principal", principalValue)

	h.writeJSON(w, http.StatusOK, response)
}

// generateDisplayName generates a display name from principal.
// For email principals: "alice@example.com" → "Alice"
// For non-email: use as-is
func (h *MeHandler) generateDisplayName(principal string) string {
	// Check if principal is email format
	if strings.Contains(principal, "@") {
		parts := strings.Split(principal, "@")
		if len(parts) > 0 && parts[0] != "" {
			// Capitalize first letter
			name := parts[0]
			if len(name) > 0 {
				return strings.ToUpper(name[:1]) + name[1:]
			}
		}
	}

	// Fallback: use principal as-is
	return principal
}

// generatePictureURL generates a picture URL using ui-avatars.com.
// This is a free service that generates avatar images from names.
func (h *MeHandler) generatePictureURL(displayName string) string {
	// Use ui-avatars.com API (no authentication required)
	// Format: https://ui-avatars.com/api/?name=Alice+Smith&background=random
	encoded := url.QueryEscape(displayName)
	return "https://ui-avatars.com/api/?name=" + encoded + "&background=random"
}

// writeJSON writes a JSON response.
func (h *MeHandler) writeJSON(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

// writeError writes an error response.
func (h *MeHandler) writeError(w http.ResponseWriter, statusCode int, error string, message string) {
	resp := ErrorResponse{
		Error:   error,
		Message: message,
	}
	h.writeJSON(w, statusCode, resp)
}
```

#### 2.2.5 Router Integration

Location: `internal/adapters/http/server.go` (EXTEND)

```go
func (s *Server) setupEnduserRoutes() {
	// ... existing consent routes ...

	// NEW: User info endpoint
	meHandler := user.NewMeHandler(s.logger)
	s.router.Get("/api/me", meHandler.GetUserInfo)
}
```

### 2.3 Verification of Existing Endpoints

The following endpoints from 006-domain-model-apis already exist and should work correctly:

| Endpoint | Method | Status | Notes |
|----------|--------|--------|-------|
| `/api/consent/agent/:agent-id` | GET | Implemented | Returns agent metadata + available services |
| `/api/consent/agent/:agent-id/grants` | GET | Implemented | Returns user's grants for agent |
| `/api/consent/agent/:agent-id/grants` | POST | Implemented | Creates/updates grant (upsert) |

**Verification Steps**:
1. Test with `curl` or Postman to confirm response formats match frontend expectations
2. Verify principal extraction from `X-Remote-User` header works correctly
3. Confirm expired grants are filtered per FR-019
4. Validate scope validation per FR-018

---

## 3. Hexagonal Architecture Integration

### 3.1 Port Definitions

All necessary ports already exist in `internal/ports/storage.go`:

- `AgentRepository` - Agent CRUD operations
- `UserGrantRepository` - Grant CRUD operations
- `ThirdpartyOAuth2ServiceRepository` - Service CRUD operations

**NEW PORT NEEDED**: `UserGrantRepository.ListByPrincipal()`

Location: `internal/ports/storage.go` (EXTEND)

```go
type UserGrantRepository interface {
	// ... existing methods ...

	// ListByPrincipal retrieves all active grants for a principal across all agents.
	// Filters expired grants (valid_until < NOW()).
	// Returns empty slice if no active grants exist (not an error).
	// Returns StorageError for connection/timeout issues.
	ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserGrant, error)
}
```

### 3.2 Handler Structure

**Current Structure** (from existing handlers):
```
internal/adapters/http/
├── handlers/
│   ├── admin/              # Admin API handlers
│   │   ├── agents_handler.go
│   │   └── services_handler.go
│   ├── consent/            # Consent API handlers
│   │   ├── agent_info_handler.go
│   │   └── grants_handler.go
│   └── user/               # NEW: User-related handlers
│       └── me_handler.go   # NEW
├── middleware.go           # Logging, recovery middleware
├── principal_middleware.go # Principal extraction
└── server.go               # Server setup and routing
```

**NEW FILES**:
1. `internal/adapters/http/handlers/user/me_handler.go` - GET /api/me
2. `internal/adapters/http/handlers/consent/agents_list_handler.go` - GET /api/consent/agents

### 3.3 Service Layer Responsibilities

Location: `internal/domain/consent/service.go`

**Existing Responsibilities**:
- Validate grant requests against agent and service configurations
- Enforce business rules (scope validation, expiration checks)
- Orchestrate repository calls for consent operations

**NEW RESPONSIBILITIES**:
- `ListAgentsWithGrants(principal)` - Query grants, join with agent/service data, filter expired
- Aggregate service summaries from delegated tokens

**Service Layer Pattern** (per constitution):
- Services depend on repository interfaces (ports), not concrete implementations
- Services perform business logic validation
- Services orchestrate multiple repository calls
- Services return domain types, handlers convert to HTTP responses

### 3.4 Dependency Flow

```
HTTP Request
    ↓
Handler (adapters/http/handlers/)
    ↓ depends on
Service (domain/consent/service.go)
    ↓ depends on
Repository Interface (ports/storage.go)
    ↑ implemented by
Adapter (adapters/storage/{memory,postgres}/)
```

**Example for GET /api/consent/agents**:
```
HTTP GET /api/consent/agents
    ↓
AgentsListHandler.GetAgentsWithGrants()
    ↓
ConsentService.ListAgentsWithGrants(principal)
    ↓
UserGrantRepository.ListByPrincipal(principal)  ← NEW METHOD
AgentRepository.Get(agent_id)                   ← EXISTING
ThirdpartyOAuth2ServiceRepository.Get(svc_id)   ← EXISTING
    ↓
Returns []*AgentWithGrant
    ↓
Handler converts to JSON response
```

---

## 4. Persistence Pattern Compliance

### 4.1 Repository Method Implementation

Per constitution Principle IX, all persistence must follow patterns from `specs/004-persistence-layer/quickstart.md`.

#### 4.1.1 In-Memory Implementation

Location: `internal/adapters/storage/memory/user_grants.go` (EXTEND)

```go
// ListByPrincipal retrieves all grants for a principal.
func (a *Adapter) ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserGrant, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	result := make([]*storage.UserGrant, 0)

	for _, grant := range a.userGrants {
		if grant.Principal == principal {
			// Copy to prevent external mutation
			grantCopy := grant.Copy()
			result = append(result, grantCopy)
		}
	}

	return result, nil
}
```

#### 4.1.2 PostgreSQL Implementation

Location: `internal/adapters/storage/postgres/user_grants.go` (EXTEND)

```go
// ListByPrincipal retrieves all grants for a principal.
func (a *Adapter) ListByPrincipal(ctx context.Context, principal string) ([]*storage.UserGrant, error) {
	// Apply read timeout
	ctx, cancel := context.WithTimeout(ctx, a.timeouts.Read)
	defer cancel()

	query := `
		SELECT
			id,
			principal,
			agent_id,
			valid_until,
			delegated_oauth2_tokens,
			created_at,
			updated_at
		FROM user_grants
		WHERE principal = $1
		ORDER BY created_at DESC
	`

	var grants []*storage.UserGrant
	err := a.db.SelectContext(ctx, &grants, query, principal)
	if err != nil {
		return nil, storage.NewStorageError(
			"ListByPrincipal",
			storage.ErrorKindUnknown,
			err,
			"failed to list grants by principal",
		)
	}

	return grants, nil
}
```

### 4.2 Query Pattern for Joining Data

The service layer orchestrates multiple repository calls to build `AgentWithGrant` objects:

```go
// Pseudocode for ListAgentsWithGrants
grants := grantRepo.ListByPrincipal(ctx, principal)
result := []AgentWithGrant{}

for each grant in grants {
	if grant.IsActive() {  // Filter expired (FR-019)
		agent := agentRepo.Get(ctx, grant.AgentID)

		serviceDetails := []
		for each token in grant.DelegatedOAuth2Tokens {
			service := serviceRepo.Get(ctx, token.ServiceID)
			serviceDetails.append({
				ID: service.ID,
				DisplayName: service.DisplayName,
				ScopeCount: len(token.Scopes)
			})
		}

		result.append(AgentWithGrant{
			Agent: agent,
			Grant: grant,
			ServiceDetails: serviceDetails
		})
	}
}

return result
```

### 4.3 SQL Query for Expired Grant Filtering

**Option 1: Application-Level Filtering** (RECOMMENDED)
- Repository returns all grants, service filters via `grant.IsActive()`
- Simpler, follows existing patterns
- Slightly less efficient but more maintainable

**Option 2: Database-Level Filtering**
```sql
SELECT *
FROM user_grants
WHERE principal = $1
  AND (valid_until IS NULL OR valid_until > NOW())
ORDER BY created_at DESC
```

**Recommendation**: Use Option 1 (application-level) for consistency with existing code. The `UserGrant.IsActive()` method already implements this logic correctly.

### 4.4 Error Handling Pattern

All storage errors must be wrapped in `storage.StorageError`:

```go
if err != nil {
	return nil, storage.NewStorageError(
		"ListByPrincipal",        // Operation name
		storage.ErrorKindUnknown, // Error kind
		err,                      // Underlying error
		"failed to list grants by principal", // Message
	)
}
```

### 4.5 Testing Requirements

Per constitution Principle VIII and quickstart.md:

**Unit Tests** (table-driven):
```go
func TestListByPrincipal(t *testing.T) {
	tests := []struct {
		name      string
		principal string
		setup     func(*memory.Adapter)
		want      int // expected grant count
		wantErr   bool
	}{
		{
			name:      "returns grants for principal",
			principal: "alice@example.com",
			setup: func(a *memory.Adapter) {
				// Create grants for alice
				a.Create(ctx, &storage.UserGrant{
					Principal: "alice@example.com",
					AgentID:   "agent-1",
					// ...
				})
			},
			want: 1,
		},
		{
			name:      "filters by principal",
			principal: "alice@example.com",
			setup: func(a *memory.Adapter) {
				// Create grants for alice and bob
				a.Create(ctx, aliceGrant)
				a.Create(ctx, bobGrant)
			},
			want: 1, // only alice's grant
		},
		{
			name:      "returns empty for no grants",
			principal: "charlie@example.com",
			setup:     func(a *memory.Adapter) {},
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := memory.NewAdapter()
			adapter.Initialize(ctx)
			defer adapter.Close(ctx)

			tt.setup(adapter)

			grants, err := adapter.ListByPrincipal(ctx, tt.principal)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Len(t, grants, tt.want)
			}
		})
	}
}
```

**Integration Tests** (with testcontainers for PostgreSQL):
- Test actual PostgreSQL queries
- Verify JSONB deserialization of delegated_oauth2_tokens
- Test transaction rollback on errors
- Verify concurrent access safety

---

## 5. Configuration Requirements

### 5.1 Configuration Schema Extension

Location: `internal/ports/config.go` (EXTEND)

```go
// Config represents the complete application configuration schema.
type Config struct {
	Log     LogConfig     `mapstructure:"log" validate:"required"`
	Server  ServerConfig  `mapstructure:"server" validate:"required"`
	Storage StorageConfig `mapstructure:"storage" validate:"required"`
	SPA     SPAConfig     `mapstructure:"spa" validate:"required"` // NEW
}

// SPAConfig contains configuration for serving React SPAs.
type SPAConfig struct {
	// StaticFilesPath is the directory containing built SPA assets.
	// Relative paths are resolved from the application binary location.
	// Default: "./dist/consent"
	StaticFilesPath string `mapstructure:"static_files_path" validate:"required"`

	// ServeEnabled controls whether SPA serving is enabled.
	// If false, SPA routes are not registered (useful for API-only deployments).
	// Default: true
	ServeEnabled bool `mapstructure:"serve_enabled"`
}

// DefaultSPAConfig returns default SPA configuration.
func DefaultSPAConfig() SPAConfig {
	return SPAConfig{
		StaticFilesPath: "./dist/consent",
		ServeEnabled:    true,
	}
}
```

### 5.2 Configuration Examples

#### 5.2.1 Development Configuration

Location: `examples/config/config.development.yaml` (EXTEND)

```yaml
# ... existing configuration ...

spa:
  static_files_path: "./dist/consent"
  serve_enabled: true
```

#### 5.2.2 Production Configuration

Location: `examples/config/config.production.yaml` (EXTEND)

```yaml
# ... existing configuration ...

spa:
  # Production builds typically use absolute paths
  static_files_path: "/app/dist/consent"
  serve_enabled: true
```

#### 5.2.3 API-Only Configuration

Location: `examples/config/config.api-only.yaml` (NEW)

```yaml
# ... existing configuration ...

spa:
  static_files_path: "./dist/consent"
  serve_enabled: false  # Disable SPA serving for API-only deployments
```

### 5.3 Environment Variable Support

Per unified configuration system, all settings support environment variable overrides:

```bash
# Override SPA static files path
export IDENTITY_BROKER_SPA_STATIC_FILES_PATH="/custom/path/dist/consent"

# Disable SPA serving
export IDENTITY_BROKER_SPA_SERVE_ENABLED=false
```

### 5.4 CLI Flag Support

Add CLI flags to `cmd/agentic-identity-broker/root.go`:

```go
func init() {
	// ... existing flags ...

	// SPA configuration flags
	rootCmd.PersistentFlags().String("spa-static-files-path", "./dist/consent",
		"Path to SPA static files directory")
	rootCmd.PersistentFlags().Bool("spa-serve-enabled", true,
		"Enable SPA serving")

	viper.BindPFlag("spa.static_files_path", rootCmd.PersistentFlags().Lookup("spa-static-files-path"))
	viper.BindPFlag("spa.serve_enabled", rootCmd.PersistentFlags().Lookup("spa-serve-enabled"))
}
```

### 5.5 Validation Rules

Per constitution Principle VII, configuration must be validated at startup:

```go
// Validation in internal/config/loader.go
func validateSPAConfig(cfg *ports.SPAConfig) error {
	if cfg.ServeEnabled {
		// Validate static files path exists
		if cfg.StaticFilesPath == "" {
			return errors.New("spa.static_files_path is required when spa.serve_enabled is true")
		}

		// Check if directory exists (warning, not error - allows for build-time deployment)
		indexPath := filepath.Join(cfg.StaticFilesPath, "index.html")
		if _, err := os.Stat(indexPath); os.IsNotExist(err) {
			// Log warning but don't fail startup
			// SPA routes simply won't be registered
			slog.Warn("SPA static files not found at startup",
				"path", cfg.StaticFilesPath,
				"index", indexPath,
				"note", "SPA routes will not be registered")
		}
	}

	return nil
}
```

---

## 6. Security Considerations

### 6.1 Authentication and Session Management

#### 6.1.1 Existing Session Pattern

The application uses pre-authentication mode where a trusted reverse proxy (e.g., nginx, Traefik) handles user authentication and sets the `X-Remote-User` header.

**Principal Extraction Flow**:
```
User → Reverse Proxy (OAuth2/OIDC) → Go Backend
                ↓
        Sets X-Remote-User: alice@example.com
                ↓
        OptionalPrincipalMiddleware extracts principal
                ↓
        principal.WithPrincipal(ctx, "alice@example.com")
                ↓
        Handler: principal.FromContext(ctx)
```

#### 6.1.2 Endpoint Protection

| Endpoint | Middleware | Principal Required | Authorization |
|----------|-----------|-------------------|---------------|
| `/consent/*` (SPA) | OptionalPrincipalMiddleware | No | Public access |
| `/api/me` | OptionalPrincipalMiddleware | Yes | Self only |
| `/api/consent/agents` | OptionalPrincipalMiddleware | Yes | Self only |
| `/api/consent/agent/:agent-id` | OptionalPrincipalMiddleware | No | Public metadata |
| `/api/consent/agent/:agent-id/grants` | OptionalPrincipalMiddleware | Yes | Self only |

**Authorization Pattern** (in handlers):
```go
func (h *Handler) GetAgentsWithGrants(w http.ResponseWriter, r *http.Request) {
	// Extract principal
	principal, ok := principal.FromContext(r.Context())
	if !ok || principal == "" {
		// Fail closed (SR-006)
		h.writeError(w, http.StatusUnauthorized, "unauthorized", "")
		return
	}

	// Authorization is implicit: user can only see their own grants
	// Service filters by principal parameter
	agents, err := h.service.ListAgentsWithGrants(ctx, principal)
	// ...
}
```

### 6.2 CSRF Protection

#### 6.2.1 Risk Assessment

**CSRF-Vulnerable Endpoints**:
- `POST /api/consent/agent/:agent-id/grants` - Creates/modifies user grants

**CSRF-Safe Endpoints**:
- `GET /api/me` - Read-only
- `GET /api/consent/agents` - Read-only
- `GET /api/consent/agent/:agent-id` - Read-only
- `GET /api/consent/agent/:agent-id/grants` - Read-only

#### 6.2.2 CSRF Mitigation Strategy

**Option 1: SameSite Cookie + CORS** (RECOMMENDED for reverse proxy setup)
- Reverse proxy sets `SameSite=Strict` or `SameSite=Lax` on session cookies
- Go backend enforces CORS headers allowing only frontend origin
- No CSRF token needed if SameSite cookies work

**Option 2: CSRF Token Middleware**
- Generate CSRF token on first visit
- Store in HTTP-only cookie
- Require `X-CSRF-Token` header on state-changing requests
- Validate token matches cookie value

**Implementation** (Option 1 - CORS):

Location: `internal/adapters/http/server.go` (EXTEND)

```go
func (s *Server) setupRoutes() {
	// ... existing middleware ...

	// Add CORS middleware for SPA
	if s.name == "enduser" {
		s.router.Use(s.corsMiddleware())
	}
}

// corsMiddleware adds CORS headers for React SPA.
func (s *Server) corsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Allow requests from SPA origin
			// In production, this should be configurable
			origin := r.Header.Get("Origin")

			// For same-origin requests (SPA served by same server), no CORS needed
			// For reverse proxy setups, allow configured origins
			if origin != "" {
				// TODO: Make allowed origins configurable
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Remote-User")
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
```

**Note**: CSRF protection is primarily handled by the reverse proxy's session cookie configuration. The Go backend's role is to validate the principal and enforce same-origin policy via CORS.

### 6.3 Authorization Checks

#### 6.3.1 User Can Only See Their Own Grants (SR-004, SR-010)

**Implementation Pattern**:
```go
// Service layer enforces principal-based filtering
func (s *Service) ListAgentsWithGrants(ctx context.Context, principal string) ([]*AgentWithGrant, error) {
	// Query grants filtered by principal
	// User CANNOT specify a different principal - it comes from session
	grants, err := s.grantRepo.ListByPrincipal(ctx, principal)
	// ...
}
```

**Security Properties**:
- Principal comes from authenticated session (reverse proxy)
- User cannot manipulate principal parameter
- Repository queries filter by principal automatically
- No privilege escalation possible

#### 6.3.2 Fail Closed on Missing Principal (SR-006)

```go
principal, ok := principal.FromContext(ctx)
if !ok || principal == "" {
	// Fail closed - reject request
	return http.StatusUnauthorized
}
```

### 6.4 Rate Limiting (SR-012)

**Recommendation**: Implement rate limiting at reverse proxy level (nginx, Traefik) for better performance and DDoS protection.

**Go-level rate limiting** (future enhancement):
```go
// Use golang.org/x/time/rate for per-principal rate limiting
func (s *Server) rateLimitMiddleware() func(http.Handler) http.Handler {
	limiters := sync.Map{} // map[principal]*rate.Limiter

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			principal, ok := principal.FromContext(r.Context())
			if ok {
				limiter := s.getLimiter(principal, &limiters)
				if !limiter.Allow() {
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

### 6.5 Input Validation

#### 6.5.1 Agent ID Validation (SR-011)

Already implemented in existing handlers:
```go
agentID := chi.URLParam(r, "agent-id")
if agentID == "" {
	return http.StatusBadRequest
}
// UUID validation happens in service/repository layer
```

#### 6.5.2 URL Validation

Already implemented in `internal/domain/storage/agent.go`:
```go
func isValidURL(urlStr string) bool {
	u, err := url.Parse(urlStr)
	if err != nil {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}
```

### 6.6 Audit Logging (SR-007, SR-008)

Per constitution Principle I (Security-First), all security-critical operations must be auditable via structured logging.

**Logging Requirements**:
```go
// Success case
h.logger.Info("agents with grants retrieved",
	"principal", principal,
	"count", len(agents),
	"operation", "list_agents_with_grants")

// Authorization failure
h.logger.Warn("unauthorized access attempt",
	"principal", principal,
	"endpoint", r.URL.Path,
	"remote_addr", r.RemoteAddr,
	"operation", "list_agents_with_grants")

// Service error
h.logger.Error("failed to list agents with grants",
	"principal", principal,
	"error", err,
	"operation", "list_agents_with_grants")
```

**Audit Log Fields** (consistent across all handlers):
- `operation`: Semantic operation name (e.g., "list_agents_with_grants")
- `principal`: Authenticated user
- `remote_addr`: Client IP (from reverse proxy if available)
- `error`: Error details (if applicable)
- `entity_id`: Agent/grant/service ID (if applicable)

---

## 7. Testing Strategy

### 7.1 Unit Tests for Handlers

**Pattern**: Mock service layer, test HTTP request/response handling.

Location: `internal/adapters/http/handlers/consent/agents_list_handler_test.go`

```go
func TestAgentsListHandler_GetAgentsWithGrants(t *testing.T) {
	tests := []struct {
		name           string
		principal      string
		setupMock      func(*MockConsentService)
		expectedStatus int
		expectedBody   string
	}{
		{
			name:      "success with grants",
			principal: "alice@example.com",
			setupMock: func(m *MockConsentService) {
				m.On("ListAgentsWithGrants", mock.Anything, "alice@example.com").
					Return([]*consent.AgentWithGrant{
						// Mock agent with grant
					}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:      "empty grants",
			principal: "bob@example.com",
			setupMock: func(m *MockConsentService) {
				m.On("ListAgentsWithGrants", mock.Anything, "bob@example.com").
					Return([]*consent.AgentWithGrant{}, nil)
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:           "missing principal",
			principal:      "", // No principal in context
			setupMock:      func(m *MockConsentService) {},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:      "service error",
			principal: "charlie@example.com",
			setupMock: func(m *MockConsentService) {
				m.On("ListAgentsWithGrants", mock.Anything, "charlie@example.com").
					Return(nil, errors.New("database error"))
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock service
			mockService := new(MockConsentService)
			tt.setupMock(mockService)

			// Create handler
			handler := consent.NewAgentsListHandler(mockService, slog.Default())

			// Create request with principal in context
			req := httptest.NewRequest("GET", "/api/consent/agents", nil)
			if tt.principal != "" {
				ctx := principal.WithPrincipal(req.Context(), tt.principal)
				req = req.WithContext(ctx)
			}

			// Execute request
			w := httptest.NewRecorder()
			handler.GetAgentsWithGrants(w, req)

			// Assert status code
			assert.Equal(t, tt.expectedStatus, w.Code)

			// Assert response body (if applicable)
			if tt.expectedBody != "" {
				assert.JSONEq(t, tt.expectedBody, w.Body.String())
			}

			mockService.AssertExpectations(t)
		})
	}
}
```

### 7.2 Integration Tests for Repository Methods

**Pattern**: Use testcontainers for PostgreSQL, test actual database queries.

Location: `internal/adapters/storage/postgres/user_grants_test.go` (EXTEND)

```go
func TestPostgres_ListByPrincipal(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// Start PostgreSQL container
	ctx := context.Background()
	container, connURL := setupTestContainer(t, ctx)
	defer container.Terminate(ctx)

	// Create adapter
	adapter := setupTestAdapter(t, ctx, connURL)
	defer adapter.Close(ctx)

	// Create test grants
	aliceGrant := &storage.UserGrant{
		ID:        "grant-alice",
		Principal: "alice@example.com",
		AgentID:   "agent-1",
		DelegatedOAuth2Tokens: []storage.DelegatedToken{
			{ThirdpartyOAuth2ServiceID: "svc-1", Scopes: []string{"read"}},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	bobGrant := &storage.UserGrant{
		ID:        "grant-bob",
		Principal: "bob@example.com",
		AgentID:   "agent-1",
		DelegatedOAuth2Tokens: []storage.DelegatedToken{
			{ThirdpartyOAuth2ServiceID: "svc-1", Scopes: []string{"write"}},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	err := adapter.Create(ctx, aliceGrant)
	require.NoError(t, err)
	err = adapter.Create(ctx, bobGrant)
	require.NoError(t, err)

	// Test: List alice's grants
	grants, err := adapter.ListByPrincipal(ctx, "alice@example.com")
	require.NoError(t, err)
	assert.Len(t, grants, 1)
	assert.Equal(t, "grant-alice", grants[0].ID)

	// Test: List bob's grants
	grants, err = adapter.ListByPrincipal(ctx, "bob@example.com")
	require.NoError(t, err)
	assert.Len(t, grants, 1)
	assert.Equal(t, "grant-bob", grants[0].ID)

	// Test: List non-existent principal
	grants, err = adapter.ListByPrincipal(ctx, "charlie@example.com")
	require.NoError(t, err)
	assert.Empty(t, grants)
}
```

### 7.3 Service Layer Tests

**Pattern**: Mock repositories, test business logic.

Location: `internal/domain/consent/service_test.go` (EXTEND)

```go
func TestService_ListAgentsWithGrants(t *testing.T) {
	tests := []struct {
		name         string
		principal    string
		setupMocks   func(*MockGrantRepo, *MockAgentRepo, *MockServiceRepo)
		expectedLen  int
		expectedErr  bool
	}{
		{
			name:      "returns agents with active grants",
			principal: "alice@example.com",
			setupMocks: func(gr *MockGrantRepo, ar *MockAgentRepo, sr *MockServiceRepo) {
				// Mock grant repository
				gr.On("ListByPrincipal", mock.Anything, "alice@example.com").
					Return([]*storage.UserGrant{
						{
							ID:        "grant-1",
							Principal: "alice@example.com",
							AgentID:   "agent-1",
							ValidUntil: nil, // indefinite
							DelegatedOAuth2Tokens: []storage.DelegatedToken{
								{ThirdpartyOAuth2ServiceID: "svc-1", Scopes: []string{"read", "write"}},
							},
						},
					}, nil)

				// Mock agent repository
				ar.On("Get", mock.Anything, "agent-1").
					Return(&storage.Agent{
						ID:          "agent-1",
						DisplayName: "Test Agent",
						Description: "Test",
					}, nil)

				// Mock service repository
				sr.On("Get", mock.Anything, "svc-1").
					Return(&storage.ThirdpartyOAuth2Service{
						ID:          "svc-1",
						DisplayName: "GitHub",
					}, nil)
			},
			expectedLen: 1,
			expectedErr: false,
		},
		{
			name:      "filters expired grants",
			principal: "bob@example.com",
			setupMocks: func(gr *MockGrantRepo, ar *MockAgentRepo, sr *MockServiceRepo) {
				pastTime := time.Now().Add(-24 * time.Hour)
				gr.On("ListByPrincipal", mock.Anything, "bob@example.com").
					Return([]*storage.UserGrant{
						{
							ID:         "grant-2",
							Principal:  "bob@example.com",
							AgentID:    "agent-2",
							ValidUntil: &pastTime, // expired
							DelegatedOAuth2Tokens: []storage.DelegatedToken{
								{ThirdpartyOAuth2ServiceID: "svc-1", Scopes: []string{"read"}},
							},
						},
					}, nil)
			},
			expectedLen: 0, // expired grant filtered out
			expectedErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mocks
			grantRepo := new(MockGrantRepo)
			agentRepo := new(MockAgentRepo)
			serviceRepo := new(MockServiceRepo)

			tt.setupMocks(grantRepo, agentRepo, serviceRepo)

			// Create service
			service := consent.NewService(agentRepo, serviceRepo, grantRepo)

			// Execute
			agents, err := service.ListAgentsWithGrants(context.Background(), tt.principal)

			// Assert
			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Len(t, agents, tt.expectedLen)
			}

			grantRepo.AssertExpectations(t)
			agentRepo.AssertExpectations(t)
			serviceRepo.AssertExpectations(t)
		})
	}
}
```

### 7.4 SPA Serving Tests

Location: `internal/adapters/http/spa_test.go` (NEW FILE)

```go
func TestServer_ServeSPA(t *testing.T) {
	// Create temporary SPA directory
	tmpDir := t.TempDir()

	// Create mock index.html
	indexHTML := `<!DOCTYPE html><html><body>Test SPA</body></html>`
	err := os.WriteFile(filepath.Join(tmpDir, "index.html"), []byte(indexHTML), 0644)
	require.NoError(t, err)

	// Create assets directory with mock JS file
	assetsDir := filepath.Join(tmpDir, "assets")
	err = os.MkdirAll(assetsDir, 0755)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(assetsDir, "app-abc123.js"), []byte("console.log('test')"), 0644)
	require.NoError(t, err)

	// Create server
	config := ports.ServerInstanceConfig{
		Port: 8080,
		Bind: "127.0.0.1",
	}
	server := http.NewServer("enduser", config, slog.Default())

	// Set SPA config
	spaConfig := ports.SPAConfig{
		StaticFilesPath: tmpDir,
		ServeEnabled:    true,
	}
	server.SetSPAConfig(spaConfig)

	// Setup routes
	server.setupRoutes()

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string
		expectedHeader string
	}{
		{
			name:           "serve index.html at root",
			path:           "/consent",
			expectedStatus: http.StatusOK,
			expectedBody:   "Test SPA",
			expectedHeader: "text/html",
		},
		{
			name:           "fallback to index.html for unknown route",
			path:           "/consent/agent/123",
			expectedStatus: http.StatusOK,
			expectedBody:   "Test SPA",
			expectedHeader: "text/html",
		},
		{
			name:           "serve static asset",
			path:           "/consent/assets/app-abc123.js",
			expectedStatus: http.StatusOK,
			expectedBody:   "console.log('test')",
			expectedHeader: "application/javascript",
		},
		{
			name:           "API routes not affected",
			path:           "/api/me",
			expectedStatus: http.StatusUnauthorized, // No principal
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			w := httptest.NewRecorder()

			server.Router().ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedBody != "" {
				assert.Contains(t, w.Body.String(), tt.expectedBody)
			}

			if tt.expectedHeader != "" {
				assert.Contains(t, w.Header().Get("Content-Type"), tt.expectedHeader)
			}
		})
	}
}
```

### 7.5 Test Coverage Requirements

Per constitution Principle VIII:
- **Unit test coverage**: > 80% for handlers and service methods
- **Integration test coverage**: All new repository methods
- **Table-driven tests**: For validation logic and business rules
- **Race detection**: Run all tests with `go test -race`

**Run tests**:
```bash
# Fast Go/package tests
just test

# Full verification gate
just verify

# Integration suites (Docker/Podman required for infra-backed layer)
just test-integration

# With coverage
just test-coverage

# With race detection
go test -race ./...
```

---

## 8. Implementation Checklist

### 8.1 Phase 1: Configuration and SPA Serving

- [ ] Extend `internal/ports/config.go` with `SPAConfig`
- [ ] Update `internal/config/loader.go` to load SPA configuration
- [ ] Add SPA configuration examples to `examples/config/`
- [ ] Implement `serveSPA()` in `internal/adapters/http/server.go`
- [ ] Implement `serveStaticFile()` with cache headers
- [ ] Implement `setMIMEType()` for content type handling
- [ ] Write unit tests for SPA serving logic
- [ ] Update `ARCHITECTURE.md` with SPA serving details

### 8.2 Phase 2: Repository Extension

- [ ] Add `ListByPrincipal()` to `internal/ports/storage.go`
- [ ] Implement `ListByPrincipal()` in `internal/adapters/storage/memory/user_grants.go`
- [ ] Implement `ListByPrincipal()` in `internal/adapters/storage/postgres/user_grants.go`
- [ ] Write unit tests for in-memory implementation
- [ ] Write integration tests for PostgreSQL implementation
- [ ] Verify expired grant filtering works correctly

### 8.3 Phase 3: Service Layer Extension

- [ ] Add `AgentWithGrant` type to `internal/domain/consent/service.go`
- [ ] Add `ServiceDetail` type
- [ ] Implement `ListAgentsWithGrants()` service method
- [ ] Write unit tests for service method (mock repositories)
- [ ] Verify expired grant filtering in service layer
- [ ] Test join logic (agent + service lookups)

### 8.4 Phase 4: GET /api/consent/agents Handler

- [ ] Create `internal/adapters/http/handlers/consent/agents_list_handler.go`
- [ ] Implement `AgentsListHandler` struct
- [ ] Implement `GetAgentsWithGrants()` handler method
- [ ] Implement response conversion (`toAgentWithGrant()`)
- [ ] Write unit tests for handler
- [ ] Register route in `internal/adapters/http/server.go`
- [ ] Test with `curl` or Postman

### 8.5 Phase 5: GET /api/me Handler

- [ ] Create `internal/adapters/http/handlers/user/me_handler.go`
- [ ] Implement `MeHandler` struct
- [ ] Implement `GetUserInfo()` handler method
- [ ] Implement `generateDisplayName()` helper
- [ ] Implement `generatePictureURL()` helper
- [ ] Write unit tests for handler
- [ ] Register route in `internal/adapters/http/server.go`
- [ ] Test with `curl` or Postman

### 8.6 Phase 6: Security and CORS

- [ ] Implement CORS middleware for SPA origin
- [ ] Test CSRF protection with same-origin requests
- [ ] Verify principal extraction for all endpoints
- [ ] Add structured audit logging for new endpoints
- [ ] Test rate limiting behavior (if implemented)
- [ ] Security review of all new code

### 8.7 Phase 7: Documentation and Verification

- [ ] Update `ARCHITECTURE.md` with new endpoints and SPA serving
- [ ] Update glossary with new domain concepts
- [ ] Add examples to `examples/config/README.md`
- [ ] Write end-user documentation for consent UI
- [ ] Verify existing endpoints still work correctly
- [ ] Run full test suite with coverage
- [ ] Perform manual testing with React SPA

### 8.8 Constitution Compliance Verification

- [ ] **Principle II**: Followed ADR 003 (chi framework) and ADR 004 (storage layer)
- [ ] **Principle VI**: Maintained hexagonal architecture (ports/adapters)
- [ ] **Principle VII**: Used unified configuration system
- [ ] **Principle VIII**: Automated tests with > 80% coverage
- [ ] **Principle IX**: Followed persistence patterns from quickstart.md

---

## 9. Appendix: Code Examples

### 9.1 Complete Service Method Implementation

```go
// ListAgentsWithGrants returns all agents that the user has granted permissions to.
// Only includes agents with active (non-expired) grants.
// Implements FR-019: Expired grants are filtered out.
func (s *Service) ListAgentsWithGrants(ctx context.Context, principal string) ([]*AgentWithGrant, error) {
	// Get all grants for this principal
	grants, err := s.grantRepo.ListByPrincipal(ctx, principal)
	if err != nil {
		return nil, fmt.Errorf("failed to list grants: %w", err)
	}

	// Build result list
	result := make([]*AgentWithGrant, 0, len(grants))

	for _, grant := range grants {
		// Skip expired grants (FR-019)
		if !grant.IsActive() {
			s.logger.Debug("skipping expired grant",
				"grant_id", grant.ID,
				"principal", principal,
				"valid_until", grant.ValidUntil)
			continue
		}

		// Fetch agent (with error handling for deleted agents)
		agent, err := s.agentRepo.Get(ctx, grant.AgentID)
		if err != nil {
			if errors.Is(err, ports.ErrNotFound) {
				// Agent was deleted but grant exists (shouldn't happen with cascade delete)
				s.logger.Warn("grant references non-existent agent",
					"grant_id", grant.ID,
					"agent_id", grant.AgentID,
					"principal", principal)
				continue
			}
			// Other errors (connection, timeout) should fail the whole operation
			return nil, fmt.Errorf("failed to fetch agent %s: %w", grant.AgentID, err)
		}

		// Build service details by looking up each delegated service
		serviceDetails := make([]ServiceDetail, 0, len(grant.DelegatedOAuth2Tokens))
		for _, token := range grant.DelegatedOAuth2Tokens {
			service, err := s.serviceRepo.Get(ctx, token.ThirdpartyOAuth2ServiceID)
			if err != nil {
				if errors.Is(err, ports.ErrNotFound) {
					// Service was deleted, log warning but continue
					s.logger.Warn("grant references non-existent service",
						"grant_id", grant.ID,
						"service_id", token.ThirdpartyOAuth2ServiceID,
						"principal", principal)
					continue
				}
				// Other errors should fail the operation
				return nil, fmt.Errorf("failed to fetch service %s: %w", token.ThirdpartyOAuth2ServiceID, err)
			}

			serviceDetails = append(serviceDetails, ServiceDetail{
				ID:          service.ID,
				DisplayName: service.DisplayName,
				ScopeCount:  len(token.Scopes),
			})
		}

		// Skip if no valid services remain after filtering
		if len(serviceDetails) == 0 {
			s.logger.Warn("grant has no valid services after filtering",
				"grant_id", grant.ID,
				"principal", principal)
			continue
		}

		result = append(result, &AgentWithGrant{
			Agent:          agent,
			Grant:          grant,
			ServiceDetails: serviceDetails,
		})
	}

	s.logger.Info("listed agents with grants",
		"principal", principal,
		"total_grants", len(grants),
		"active_grants", len(result))

	return result, nil
}
```

### 9.2 Complete Handler Test Example

```go
func TestAgentsListHandler_GetAgentsWithGrants_Success(t *testing.T) {
	// Setup mock service
	mockService := new(MockConsentService)

	// Expected result
	expectedAgents := []*consent.AgentWithGrant{
		{
			Agent: &storage.Agent{
				ID:          "agent-1",
				DisplayName: "Research Agent",
				Description: "Helps with research",
			},
			Grant: &storage.UserGrant{
				ID:         "grant-1",
				Principal:  "alice@example.com",
				AgentID:    "agent-1",
				ValidUntil: nil,
				CreatedAt:  time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC),
				DelegatedOAuth2Tokens: []storage.DelegatedToken{
					{
						ThirdpartyOAuth2ServiceID: "svc-1",
						Scopes:                    []string{"read", "write"},
					},
				},
			},
			ServiceDetails: []consent.ServiceDetail{
				{
					ID:          "svc-1",
					DisplayName: "GitHub",
					ScopeCount:  2,
				},
			},
		},
	}

	mockService.On("ListAgentsWithGrants", mock.Anything, "alice@example.com").
		Return(expectedAgents, nil)

	// Create handler
	handler := consent.NewAgentsListHandler(mockService, slog.Default())

	// Create request with principal
	req := httptest.NewRequest("GET", "/api/consent/agents", nil)
	ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
	req = req.WithContext(ctx)

	// Execute
	w := httptest.NewRecorder()
	handler.GetAgentsWithGrants(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))

	// Parse response
	var response consent.AgentsListResponse
	err := json.NewDecoder(w.Body).Decode(&response)
	require.NoError(t, err)

	// Verify response content
	require.Len(t, response.Agents, 1)

	agent := response.Agents[0]
	assert.Equal(t, "agent-1", agent.AgentID)
	assert.Equal(t, "Research Agent", agent.DisplayName)
	assert.NotNil(t, agent.GrantSummary)
	assert.Equal(t, "grant-1", agent.GrantSummary.GrantID)
	assert.Equal(t, 1, agent.GrantSummary.ServiceCount)
	assert.Len(t, agent.GrantSummary.Services, 1)
	assert.Equal(t, "GitHub", agent.GrantSummary.Services[0].DisplayName)

	mockService.AssertExpectations(t)
}
```

---

## 10. References

- **Constitution**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/.specify/memory/constitution.md`
- **Persistence Quickstart**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/specs/004-persistence-layer/quickstart.md`
- **Domain Model Spec**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/specs/006-domain-model-apis/spec.md`
- **Architecture Doc**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/ARCHITECTURE.md`
- **ADR 003 (Chi)**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/adrs/003-chi-framework.md`
- **ADR 004 (Storage)**: `/Users/magnus.jungsbluth/Projects/agentic-identity-broker/adrs/004-storage-layer-architecture.md`

---

**Document Status**: Complete
**Next Steps**: Review with stakeholders, then proceed with implementation following the checklist in Section 8.
