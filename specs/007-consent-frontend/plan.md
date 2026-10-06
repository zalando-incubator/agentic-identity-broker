# Implementation Plan: Consent Management Frontend

**Branch**: `007-consent-frontend` | **Date**: 2025-12-18 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/007-consent-frontend/spec.md`

> **Historical visual scope**: The visual choices in this plan record feature 007's original design, not a current constitutional aesthetic mandate. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Build a React-based single page application for user consent management, allowing users to view agents they've delegated access to, review and modify service-level permissions with fine-grained scope control, and configure grant expiration. The frontend will be served by the Go backend under `/consent` path prefix, using React 18+ with Tailwind CSS v4.0 and Headless UI components. Backend will expose new APIs for listing consented agents and user info, while frontend implements progressive loading with skeleton screens, inline error recovery, and optimistic UI updates.

## Technical Context

**Language/Version**:
- Frontend: React 18.2+, TypeScript 5.3+
- Backend: Go 1.21+

**Primary Dependencies**:
- Frontend: Vite 5.0+ (build tool), React Router 6.20+ (routing), Tailwind CSS v4.0 (styling), Headless UI 1.7+ (accessible components), Framer Motion 10.16+ (animations), date-fns 2.30+ (date handling)
- Backend: chi v5.2.3 (HTTP router), sqlx v1.3.5+ (database queries), pgx v5 (PostgreSQL driver)

**Storage**: PostgreSQL 12+ (for User Grant, Agent, Thirdparty OAuth2 Service entities)

**Testing**:
- Frontend: Vitest 1.0+ (unit tests), React Testing Library 14.1+ (component tests), Playwright (E2E tests)
- Backend: Go testing (unit/integration), testcontainers (PostgreSQL integration tests)

**Target Platform**: Modern web browsers (last 2 major versions of Chrome, Firefox, Safari, Edge)

**Project Type**: Web application (React SPA + Go backend API)

**Performance Goals**:
- Page load under 2 seconds on broadband
- API responses under 2 seconds for data fetching
- Support 100 concurrent grant requests without degradation
- Handle 20+ third-party services without pagination

**Constraints**:
- Must work with existing session management (principal extraction)
- Must maintain hexagonal architecture with ports/adapters
- Must follow persistence patterns from specs/004-persistence-layer/quickstart.md
- Must use unified configuration system from 002-flexible-configuration
- No React Server Components (Go backend is BFF)
- Client-side routing only (History API, no hash routing)

**Scale/Scope**:
- ~2 main pages (overview + agent detail)
- ~15 React components
- ~5 custom hooks
- ~3 new backend API endpoints
- ~8 implementation phases

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

Before proceeding, verify compliance with [.specify/memory/constitution.md](../../.specify/memory/constitution.md):

- [x] **Security-First**: Are security features enabled by default? No bypasses or optional security?
  - ✅ All consent APIs require authentication (principal from session)
  - ✅ Authorization enforced: users can only view/modify their own grants
  - ✅ Backend validates all inputs (agent IDs, dates, scopes)
  - ✅ Frontend is untrusted client; backend enforces all security policies
  - ✅ CSRF protection for POST /api/consent/agent/:agent-id/grants
  - ✅ Structured audit logging for all grant operations

- [x] **Architecture Docs**: Will ARCHITECTURE.md be updated if this touches architecture?
  - ✅ Yes, will add `web/` folder structure for React SPA
  - ✅ Will document SPA serving pattern under `/consent` path
  - ✅ Will update API endpoint documentation for new /api/consent/* routes
  - ✅ Will document frontend-to-backend integration pattern

- [x] **ADRs**: Does this require an ADR in adrs/ for major decisions?
  - ⚠️ **Potential ADR needed**: SPA serving pattern from Go backend (embed.FS + History API fallback)
  - ⚠️ **Potential ADR needed**: Frontend technology stack (React + Tailwind v4.0 + Headless UI)
  - ✅ Follows existing ADR 003 (chi router for HTTP)
  - ✅ Follows existing ADR 004 (storage layer architecture, sqlx patterns)

- [x] **Library-First Security**: Are we using vetted libraries for crypto/security (no custom implementations)?
  - ✅ No custom cryptography or security primitives
  - ✅ Session management uses existing middleware
  - ✅ All authentication/authorization via established patterns
  - ✅ Frontend uses React (maintained by Meta) and Headless UI (Tailwind Labs)

- [x] **API Documentation**: Will docs/ be updated for any new/changed APIs?
  - ✅ Yes, will document GET /api/consent/agents
  - ✅ Will document GET /api/me
  - ✅ Will document SPA access via /consent path
  - ✅ Will provide API examples and response structures

- [x] **Domain Model**: Are new domain concepts documented in ARCHITECTURE.md Glossary?
  - ✅ No new domain concepts (uses existing entities from 006-domain-model-apis)
  - ✅ Entities already documented: Agent, Thirdparty OAuth2 Service, OAuth Scope, User Grant
  - ✅ Will reference existing glossary entries in docs

- [x] **Hexagonal Architecture**: Does domain logic use ports (interfaces) with clear adapter separation?
  - ✅ New repository method in internal/ports/storage.go: ListByPrincipal()
  - ✅ HTTP handlers in internal/adapters/http/handlers/
  - ✅ Service layer orchestration in internal/services/
  - ✅ No direct database access from handlers
  - ✅ Follows hexagonal architecture pattern per Principle VI

- [x] **Configuration-Driven Design** (Principle VII): Using unified configuration system?
  - ✅ New SPAConfig in internal/config/schema.go
  - ✅ Uses existing ConfigPort from internal/ports/config.go
  - ✅ Environment variable and CLI flag support
  - ✅ Example configuration in examples/config/

- [x] **Test-Driven Development** (Principle VIII): Automated tests included?
  - ✅ Frontend: Vitest unit tests, React Testing Library component tests
  - ✅ Backend: Go unit tests (mocked services), integration tests (testcontainers)
  - ✅ Table-driven tests for validation logic
  - ✅ No Bash scripts for correctness validation

- [x] **Persistence Pattern Consistency** (Principle IX): Following quickstart.md patterns?
  - ✅ Extends existing UserGrantRepository interface
  - ✅ Implements in both memory and postgres adapters
  - ✅ Uses sqlx for PostgreSQL queries (no ORM)
  - ✅ Wraps errors in domain.StorageError
  - ✅ Respects configured timeouts
  - ✅ Follows patterns from specs/004-persistence-layer/quickstart.md

*All checks pass. Feature is compliant with constitution principles. ADRs for SPA serving and frontend stack may be needed during implementation.*

## Project Structure

### Documentation (this feature)

```text
specs/[###-feature]/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
web/                                      # NEW: React SPA for consent management
├── src/
│   ├── pages/
│   │   ├── ConsentOverviewPage.tsx      # Main /consent route
│   │   ├── AgentGrantDetailPage.tsx     # /consent/agent/:agentId route
│   │   └── ErrorPage.tsx                # 404/error states
│   ├── components/
│   │   ├── consent/                     # Consent-specific components
│   │   │   ├── DelegationList.tsx
│   │   │   ├── DelegationCard.tsx
│   │   │   ├── ServiceGrantList.tsx
│   │   │   ├── ServiceGrantCard.tsx
│   │   │   ├── ScopeList.tsx
│   │   │   ├── GrantValidityControl.tsx
│   │   │   └── GrantStatusBadge.tsx
│   │   ├── ui/                          # Reusable UI primitives
│   │   │   ├── Skeleton.tsx
│   │   │   ├── Button.tsx
│   │   │   ├── ErrorBoundary.tsx
│   │   │   ├── InlineError.tsx
│   │   │   ├── EmptyState.tsx
│   │   │   ├── Switch.tsx               # Headless UI wrapper
│   │   │   ├── DatePicker.tsx
│   │   │   ├── Dialog.tsx
│   │   │   └── Card.tsx
│   │   └── layout/
│   │       ├── AppLayout.tsx
│   │       ├── Header.tsx
│   │       └── Footer.tsx
│   ├── services/
│   │   ├── api/
│   │   │   ├── client.ts                # Axios/fetch wrapper
│   │   │   ├── consent.ts               # Consent API calls
│   │   │   └── types.ts                 # API response types
│   │   └── storage/
│   │       └── session.ts               # Session token handling
│   ├── hooks/
│   │   ├── useConsent.ts                # Fetch delegations
│   │   ├── useAgentGrants.ts            # Fetch agent grants
│   │   ├── useToggleGrant.ts            # Toggle grant on/off
│   │   ├── useUpdateValidity.ts         # Update expiration
│   │   └── useRetry.ts                  # Retry failed requests
│   ├── context/
│   │   └── ConsentContext.tsx           # Global consent state
│   ├── utils/
│   │   ├── date.ts
│   │   ├── errors.ts
│   │   └── animations.ts
│   ├── types/
│   │   ├── consent.ts
│   │   └── index.ts
│   ├── assets/
│   │   ├── fonts/
│   │   │   ├── CrimsonPro-Variable.woff2
│   │   │   ├── Manrope-Variable.woff2
│   │   │   └── JetBrainsMono-Variable.woff2
│   │   └── images/
│   ├── styles/
│   │   ├── index.css                    # Global + Tailwind
│   │   └── fonts.css                    # @font-face
│   ├── App.tsx                          # Root + routing
│   ├── main.tsx                         # React 18 entry
│   └── vite-env.d.ts
├── public/
│   └── favicon.ico
├── index.html
├── vite.config.ts                       # Build config (/consent base)
├── tailwind.config.ts                   # Tailwind v4 config
├── postcss.config.js
├── tsconfig.json
├── tsconfig.node.json
├── package.json
└── .gitignore

dist/consent/                             # NEW: Vite build output (gitignored)
└── [compiled assets: index.html, CSS, JS, fonts]

internal/
├── adapters/
│   ├── http/
│   │   ├── handlers/
│   │   │   ├── consent_agents.go        # NEW: GET /api/consent/agents
│   │   │   ├── user_info.go             # NEW: GET /api/me
│   │   │   └── spa.go                   # NEW: Serve React SPA
│   │   └── middleware/
│   │       └── cors.go                  # UPDATED: CORS for SPA
│   └── storage/
│       ├── memory/
│       │   └── user_grant.go            # UPDATED: Add ListByPrincipal()
│       └── postgres/
│           └── user_grant.go            # UPDATED: Add ListByPrincipal()
├── ports/
│   └── storage.go                       # UPDATED: UserGrantRepository interface
├── services/
│   └── consent.go                       # NEW: Consent service layer
└── config/
    └── schema.go                        # UPDATED: Add SPAConfig

docs/
└── api/
    └── consent-endpoints.md             # NEW: Document consent APIs

examples/config/
└── consent-frontend.yaml                # NEW: Example SPA config
```

**Structure Decision**: Web application with React frontend (web/) and Go backend (internal/). Frontend is served as static assets by Go backend under `/consent` path prefix. Go backend provides BFF (Backend for Frontend) APIs under `/api/consent/*` and `/api/me`. No React Server Components - pure client-side React with backend API integration.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

| Violation | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| [e.g., 4th project] | [current need] | [why 3 projects insufficient] |
| [e.g., Repository pattern] | [specific problem] | [why direct DB access insufficient] |
