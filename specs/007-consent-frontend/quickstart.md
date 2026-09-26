# Quickstart: Consent Management Frontend

**Feature**: 007-consent-frontend
**Date**: 2025-12-18
**Status**: Implementation Guide
**Constitution Version**: 1.2.0

> **Historical visual scope**: The typography, palette, motion, and styling examples in this guide record feature 007's original design, not a current constitutional aesthetic mandate. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

## Overview

This quickstart guide helps developers implement and work with the consent management frontend feature. Follow this guide to set up your development environment, understand the architecture, and implement the feature following established patterns.

---

## Table of Contents

1. [Prerequisites](#prerequisites)
2. [Initial Setup](#initial-setup)
3. [Frontend Development](#frontend-development)
4. [Backend Development](#backend-development)
5. [Testing](#testing)
6. [Common Patterns](#common-patterns)
7. [Troubleshooting](#troubleshooting)

---

## Prerequisites

### Required Tools

- **Go 1.21+**: Backend development
- **Node.js 18+**: Frontend development (Vite, React)
- **PostgreSQL 12+**: Database (or use in-memory adapter for development)
- **just**: Task runner (install via `brew install just` or see [justfile.md](https://github.com/casey/just))

### Optional Tools

- **Air**: Hot reload for Go development (`go install github.com/cosmtrek/air@latest`)
- **golangci-lint**: Code linting (`brew install golangci-lint`)
- **Playwright**: E2E testing (`npx playwright install`)

### Knowledge Prerequisites

- Familiarity with React 18+, TypeScript, Tailwind CSS
- Understanding of hexagonal architecture (ports/adapters pattern)
- Basic Go web development (chi router, sqlx)
- OAuth2 concepts (scopes, delegations)

---

## Initial Setup

### 1. Clone and Verify Prerequisites

```bash
cd /Users/magnus.jungsbluth/Projects/agentic-identity-broker

# Verify prerequisites using speckit script
.specify/scripts/bash/check-prerequisites.sh
```

### 2. Install Dependencies

```bash
# Backend dependencies
just deps

# Frontend dependencies (new for this feature)
cd web
npm install
cd ..
```

### 3. Configure the Application

Create or update your configuration file:

```yaml
# examples/config/consent-frontend.yaml
server:
  admin_port: 9090
  enduser_port: 8080

spa:
  enabled: true
  static_files_path: ./dist/consent

storage:
  adapter: postgres  # or "memory" for development
  postgres:
    host: localhost
    port: 5432
    database: identity_broker
    user: postgres
    password: postgres
    ssl_mode: disable
```

### 4. Database Setup (if using PostgreSQL)

```bash
# Create database
createdb identity_broker

# Run migrations (from 006-domain-model-apis)
# Migrations should already exist for Agent, ThirdpartyOAuth2Service, UserGrant tables
just migrate-up
```

### 5. Verify Setup

```bash
# Build backend
just build

# Build frontend
cd web
npm run build
cd ..

# Run application
just run
```

Visit http://localhost:8080/consent (should show React app or 404 if not built yet)

---

## Frontend Development

### Project Structure

```
web/
├── src/
│   ├── pages/           # Page components (routes)
│   ├── components/      # Reusable components
│   ├── services/        # API client, session management
│   ├── hooks/           # Custom React hooks
│   ├── context/         # React context providers
│   ├── utils/           # Utility functions
│   ├── types/           # TypeScript type definitions
│   ├── assets/          # Fonts, images
│   └── styles/          # Global CSS, Tailwind config
├── vite.config.ts       # Build configuration
├── tailwind.config.ts   # Tailwind CSS v4.0 config
└── package.json
```

### Development Workflow

#### 1. Start Frontend Dev Server

```bash
cd web
npm run dev
# Opens http://localhost:3000
# Proxies /api requests to http://localhost:8080
```

#### 2. Start Backend Server

```bash
# In separate terminal
just dev  # Uses Air for hot reload
# Or: just run
```

#### 3. Implement a Page Component

Example: `web/src/pages/ConsentOverviewPage.tsx`

```tsx
import { useConsent } from '@/hooks/useConsent';
import { DelegationList } from '@/components/consent/DelegationList';
import { DelegationListSkeleton } from '@/components/ui/Skeleton';
import { InlineError } from '@/components/ui/InlineError';
import { EmptyState } from '@/components/ui/EmptyState';

export function ConsentOverviewPage() {
  const { data: delegations, isLoading, error, refetch } = useConsent();

  if (isLoading) {
    return (
      <div className="container mx-auto p-6">
        <h1 className="text-3xl font-serif font-semibold text-trust mb-6">
          My Consent Delegations
        </h1>
        <DelegationListSkeleton count={5} />
      </div>
    );
  }

  if (error) {
    return (
      <div className="container mx-auto p-6">
        <h1 className="text-3xl font-serif font-semibold text-trust mb-6">
          My Consent Delegations
        </h1>
        <InlineError message={error.message} onRetry={refetch} />
      </div>
    );
  }

  if (delegations.length === 0) {
    return (
      <div className="container mx-auto p-6">
        <h1 className="text-3xl font-serif font-semibold text-trust mb-6">
          My Consent Delegations
        </h1>
        <EmptyState
          title="No Active Delegations"
          description="You haven't granted access to any agents yet."
        />
      </div>
    );
  }

  return (
    <div className="container mx-auto p-6">
      <h1 className="text-3xl font-serif font-semibold text-trust mb-6">
        My Consent Delegations
      </h1>
      <DelegationList delegations={delegations} />
    </div>
  );
}
```

#### 4. Create Custom Hook for Data Fetching

Example: `web/src/hooks/useConsent.ts`

```tsx
import { useEffect, useState } from 'react';
import { fetchAgentDelegations } from '@/services/api/consent';
import type { AgentDelegation } from '@/types/consent';
import type { ApiError } from '@/services/api/client';

interface UseConsentReturn {
  data: AgentDelegation[];
  isLoading: boolean;
  error: ApiError | null;
  refetch: () => Promise<void>;
}

export function useConsent(): UseConsentReturn {
  const [data, setData] = useState<AgentDelegation[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);

  const fetchData = async () => {
    try {
      setIsLoading(true);
      setError(null);
      const delegations = await fetchAgentDelegations();
      setData(delegations);
    } catch (err) {
      setError(err as ApiError);
    } finally {
      setIsLoading(false);
    }
  };

  useEffect(() => {
    fetchData();
  }, []);

  return { data, isLoading, error, refetch: fetchData };
}
```

#### 5. Build for Production

```bash
cd web
npm run build
# Output: dist/consent/ (served by Go backend)
```

### Design System Reference

- **Typography**: Crimson Pro (serif headings), Manrope (sans body), JetBrains Mono (monospace)
- **Colors**: Trust blues (#0A2540, #1E4D6B, #E8F1F5), Action amber (#D97706), Success emerald (#059669)
- **Spacing**: Tailwind spacing scale (4px increments)
- **Animations**: Framer Motion for staggered lists, spring physics for toggles

---

## Backend Development

### Project Structure

```
internal/
├── adapters/
│   ├── http/
│   │   ├── handlers/           # HTTP request handlers
│   │   │   ├── consent_agents.go
│   │   │   ├── user_info.go
│   │   │   └── spa.go
│   │   └── middleware/         # Middleware (CORS, auth)
│   └── storage/
│       ├── memory/             # In-memory implementations
│       └── postgres/           # PostgreSQL implementations
├── ports/
│   └── storage.go              # Repository interfaces
├── services/
│   └── consent.go              # Business logic
├── domain/
│   └── consent.go              # Domain types
└── config/
    └── schema.go               # Configuration schema
```

### Development Workflow

#### 1. Define Repository Interface

Add to `internal/ports/storage.go`:

```go
package ports

// UserGrantRepository manages user grant persistence.
type UserGrantRepository interface {
    // ... existing methods from 006-domain-model-apis ...

    // NEW: ListByPrincipal returns all grants for a user.
    ListByPrincipal(ctx context.Context, principal string) ([]domain.UserGrant, error)
}
```

#### 2. Implement Repository (PostgreSQL)

Update `internal/adapters/storage/postgres/user_grant.go`:

```go
package postgres

import (
    "context"
    "database/sql"
    "time"

    "github.com/jmoiron/sqlx"
    "github.com/your-org/agentic-identity-broker/internal/domain"
    "github.com/your-org/agentic-identity-broker/internal/ports"
)

type userGrantRepository struct {
    db *sqlx.DB
}

func NewUserGrantRepository(db *sqlx.DB) ports.UserGrantRepository {
    return &userGrantRepository{db: db}
}

func (r *userGrantRepository) ListByPrincipal(ctx context.Context, principal string) ([]domain.UserGrant, error) {
    query := `
        SELECT
            grant_id,
            agent_id,
            principal,
            delegated_tokens,
            valid_until,
            created_at,
            updated_at
        FROM user_grants
        WHERE principal = $1
          AND (valid_until IS NULL OR valid_until > NOW())
        ORDER BY updated_at DESC
    `

    var grants []domain.UserGrant
    if err := r.db.SelectContext(ctx, &grants, query, principal); err != nil {
        if err == sql.ErrNoRows {
            return []domain.UserGrant{}, nil
        }
        return nil, domain.WrapStorageError("list_grants_by_principal", err)
    }

    return grants, nil
}
```

#### 3. Implement Service Layer

Create `internal/services/consent.go`:

```go
package services

import (
    "context"
    "fmt"

    "github.com/your-org/agentic-identity-broker/internal/domain"
    "github.com/your-org/agentic-identity-broker/internal/ports"
)

type ConsentService struct {
    agentRepo   ports.AgentRepository
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository
    grantRepo   ports.UserGrantRepository
}

func NewConsentService(
    agentRepo ports.AgentRepository,
    serviceRepo ports.ThirdpartyOAuth2ServiceRepository,
    grantRepo ports.UserGrantRepository,
) *ConsentService {
    return &ConsentService{
        agentRepo:   agentRepo,
        serviceRepo: serviceRepo,
        grantRepo:   grantRepo,
    }
}

// GetAgentDelegations returns agents the user has delegated to.
func (s *ConsentService) GetAgentDelegations(ctx context.Context, principal string) ([]domain.AgentDelegation, error) {
    // Fetch user's grants
    grants, err := s.grantRepo.ListByPrincipal(ctx, principal)
    if err != nil {
        return nil, fmt.Errorf("fetch grants: %w", err)
    }

    // Group grants by agent and build delegations
    agentGrants := make(map[string][]domain.UserGrant)
    for _, grant := range grants {
        agentGrants[grant.AgentID] = append(agentGrants[grant.AgentID], grant)
    }

    delegations := make([]domain.AgentDelegation, 0, len(agentGrants))
    for agentID, grants := range agentGrants {
        // Fetch agent details
        agent, err := s.agentRepo.GetByID(ctx, agentID)
        if err != nil {
            continue // Skip if agent no longer exists
        }

        // Calculate active grant count and latest modification
        activeCount := 0
        var lastModified time.Time
        var expiration *time.Time

        for _, grant := range grants {
            activeCount += len(grant.DelegatedTokens)
            if grant.UpdatedAt.After(lastModified) {
                lastModified = grant.UpdatedAt
            }
            if grant.ValidUntil != nil {
                if expiration == nil || grant.ValidUntil.Before(*expiration) {
                    expiration = grant.ValidUntil
                }
            }
        }

        delegations = append(delegations, domain.AgentDelegation{
            AgentID:          agentID,
            DisplayName:      agent.DisplayName,
            LogoURL:          agent.LogoURL,
            ActiveGrantCount: activeCount,
            LastModifiedAt:   lastModified,
            ExpiresAt:        expiration,
        })
    }

    return delegations, nil
}
```

#### 4. Implement HTTP Handler

Create `internal/adapters/http/handlers/consent_agents.go`:

```go
package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/your-org/agentic-identity-broker/internal/adapters/http/dto"
    "github.com/your-org/agentic-identity-broker/internal/services"
    "github.com/your-org/agentic-identity-broker/pkg/session"
)

type ConsentAgentsHandler struct {
    consentService *services.ConsentService
}

func NewConsentAgentsHandler(consentService *services.ConsentService) *ConsentAgentsHandler {
    return &ConsentAgentsHandler{consentService: consentService}
}

func (h *ConsentAgentsHandler) GetAgentDelegations(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Extract principal from session
    principal := session.GetPrincipal(ctx)
    if principal == "" {
        h.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
        return
    }

    // Fetch delegations
    delegations, err := h.consentService.GetAgentDelegations(ctx, principal)
    if err != nil {
        h.writeError(w, http.StatusInternalServerError, "FETCH_FAILED", err.Error())
        return
    }

    // Respond
    response := dto.GetAgentDelegationsResponse{Data: delegations}
    h.writeJSON(w, http.StatusOK, response)
}

func (h *ConsentAgentsHandler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteStatus(status)
    json.NewEncoder(w).Encode(data)
}

func (h *ConsentAgentsHandler) writeError(w http.ResponseWriter, status int, code, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteStatus(status)
    json.NewEncoder(w).Encode(dto.ErrorResponse{
        Status:  status,
        Code:    code,
        Message: message,
    })
}
```

#### 5. Register Routes

Update `internal/adapters/http/server.go`:

```go
func (s *Server) setupEnduserRoutes() {
    // ... existing setup ...

    consentService := services.NewConsentService(s.agentRepo, s.serviceRepo, s.grantRepo)
    consentHandler := handlers.NewConsentAgentsHandler(consentService)

    s.router.Route("/api/consent", func(r chi.Router) {
        r.Use(middleware.RequireAuth)  // Ensure authentication
        r.Get("/agents", consentHandler.GetAgentDelegations)
        // ... other consent routes ...
    })

    // Serve React SPA
    s.serveSPA("/consent", s.config.SPA.StaticFilesPath, s.logger)
}
```

#### 6. Run and Test

```bash
# Terminal 1: Start backend with hot reload
just dev

# Terminal 2: Test endpoint
curl -X GET http://localhost:8080/api/consent/agents \
  -H "Cookie: session_token=test-token" \
  -H "Accept: application/json"
```

---

## Testing

### Frontend Tests

#### Unit Tests (Vitest)

```bash
cd web
npm run test
```

Example: `web/src/hooks/useConsent.test.ts`

```typescript
import { renderHook, waitFor } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { useConsent } from './useConsent';
import * as consentApi from '@/services/api/consent';

vi.mock('@/services/api/consent');

describe('useConsent', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('fetches delegations on mount', async () => {
    const mockDelegations = [
      { agentId: '1', displayName: 'Agent 1', activeGrantCount: 2 }
    ];
    vi.mocked(consentApi.fetchAgentDelegations).mockResolvedValue(mockDelegations);

    const { result } = renderHook(() => useConsent());

    expect(result.current.isLoading).toBe(true);

    await waitFor(() => {
      expect(result.current.isLoading).toBe(false);
      expect(result.current.data).toEqual(mockDelegations);
    });
  });
});
```

#### Component Tests (React Testing Library)

```typescript
import { render, screen, fireEvent } from '@testing-library/react';
import { describe, it, expect, vi } from 'vitest';
import { DelegationCard } from './DelegationCard';

describe('DelegationCard', () => {
  const mockDelegation = {
    agentId: '123',
    displayName: 'Test Agent',
    activeGrantCount: 3,
    lastModifiedAt: '2025-12-18T10:00:00Z'
  };

  it('renders agent information', () => {
    render(<DelegationCard delegation={mockDelegation} />);
    expect(screen.getByText('Test Agent')).toBeInTheDocument();
    expect(screen.getByText(/3 active/i)).toBeInTheDocument();
  });

  it('navigates on click', () => {
    const mockNavigate = vi.fn();
    vi.mock('react-router-dom', () => ({
      useNavigate: () => mockNavigate
    }));

    render(<DelegationCard delegation={mockDelegation} />);
    fireEvent.click(screen.getByRole('button'));

    expect(mockNavigate).toHaveBeenCalledWith('/agent/123');
  });
});
```

### Backend Tests

#### Unit Tests

```bash
just test
```

Example: `internal/services/consent_test.go`

```go
package services_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/your-org/agentic-identity-broker/internal/domain"
    "github.com/your-org/agentic-identity-broker/internal/services"
)

func TestGetAgentDelegations(t *testing.T) {
    // Setup mocks
    mockGrantRepo := new(MockGrantRepository)
    mockAgentRepo := new(MockAgentRepository)
    mockServiceRepo := new(MockServiceRepository)

    service := services.NewConsentService(mockAgentRepo, mockServiceRepo, mockGrantRepo)

    // Test data
    principal := "user@example.com"
    grants := []domain.UserGrant{
        {AgentID: "agent-1", DelegatedTokens: []domain.DelegatedToken{{ServiceID: "svc-1"}}},
    }

    mockGrantRepo.On("ListByPrincipal", mock.Anything, principal).Return(grants, nil)
    mockAgentRepo.On("GetByID", mock.Anything, "agent-1").Return(&domain.Agent{
        AgentID:     "agent-1",
        DisplayName: "Test Agent",
    }, nil)

    // Execute
    delegations, err := service.GetAgentDelegations(context.Background(), principal)

    // Assert
    assert.NoError(t, err)
    assert.Len(t, delegations, 1)
    assert.Equal(t, "agent-1", delegations[0].AgentID)
    assert.Equal(t, 1, delegations[0].ActiveGrantCount)
}
```

#### Integration Tests (testcontainers)

```go
package postgres_test

import (
    "context"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/testcontainers/testcontainers-go"
    "github.com/your-org/agentic-identity-broker/internal/adapters/storage/postgres"
)

func TestUserGrantRepository_ListByPrincipal(t *testing.T) {
    // Start PostgreSQL container
    ctx := context.Background()
    container, db := startPostgresContainer(t, ctx)
    defer container.Terminate(ctx)

    // Setup repository
    repo := postgres.NewUserGrantRepository(db)

    // Seed test data
    principal := "user@example.com"
    // ... insert grants ...

    // Execute
    grants, err := repo.ListByPrincipal(ctx, principal)

    // Assert
    assert.NoError(t, err)
    assert.NotEmpty(t, grants)
}
```

---

## Common Patterns

### Pattern 1: Progressive Loading with Skeleton Screens

**Frontend:**
```tsx
{isLoading ? <SkeletonLoader /> : <ActualContent data={data} />}
```

### Pattern 2: Inline Error Recovery

**Frontend:**
```tsx
{error && <InlineError message={error.message} onRetry={refetch} />}
```

### Pattern 3: Optimistic UI Updates

**Frontend:**
```tsx
const [localState, setLocalState] = useState(initialState);

const handleToggle = async () => {
  setLocalState(!localState);  // Optimistic update
  try {
    await apiCall();
  } catch (err) {
    setLocalState(localState);  // Rollback on error
  }
};
```

### Pattern 4: Hexagonal Architecture (Backend)

**Layer Separation:**
```
Handler (HTTP) → Service (Business Logic) → Repository (Persistence)
```

**Example:**
```go
// Handler: Parse request, validate, call service
func (h *Handler) HandleRequest(w http.ResponseWriter, r *http.Request) {
    data, err := h.service.DoWork(r.Context())
    h.writeJSON(w, data)
}

// Service: Business logic, orchestrate repositories
func (s *Service) DoWork(ctx context.Context) (Result, error) {
    entity, err := s.repo.Fetch(ctx)
    // ... business logic ...
    return result, nil
}

// Repository: Persistence operations
func (r *Repository) Fetch(ctx context.Context) (Entity, error) {
    // ... database query ...
}
```

---

## Troubleshooting

### Issue: SPA Routes Return 404

**Cause**: Go backend not configured to serve index.html for client-side routes.

**Solution**: Verify `serveSPA()` function in `internal/adapters/http/server.go` includes fallback logic.

### Issue: CORS Errors in Browser Console

**Cause**: Missing CORS middleware for `/api/consent/*` routes.

**Solution**: Add CORS middleware in `setupEnduserRoutes()`:

```go
s.router.Route("/api/consent", func(r chi.Router) {
    r.Use(middleware.CORS())
    // ... routes ...
})
```

### Issue: Session Token Not Sent from Frontend

**Cause**: Cookies not included in fetch requests.

**Solution**: Ensure Axios client includes credentials:

```typescript
axios.create({
  baseURL: '/api',
  withCredentials: true  // Include cookies
})
```

### Issue: Frontend Build Not Updating

**Cause**: Cached build artifacts or Go embed.FS not refreshed.

**Solution**:
```bash
cd web && npm run build && cd ..
just clean
just build
just run
```

---

## Next Steps

1. Implement Phase 1 foundation (SPA serving, user info endpoint)
2. Implement Phase 2 consent APIs (list agents, get agent detail, grants)
3. Implement Phase 3 frontend pages (overview, agent detail)
4. Implement Phase 4 frontend components (delegations, service cards, validity controls)
5. Implement Phase 5 testing (unit, integration, E2E)
6. Update ARCHITECTURE.md and API documentation
7. Create ADRs for SPA serving pattern and frontend stack (if needed)

Refer to [plan.md](plan.md) for complete implementation phases and [research.md](research.md) for detailed technical guidance.
