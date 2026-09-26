# Implementation Tasks: Consent Management Frontend

**Feature**: 007-consent-frontend
**Branch**: `007-consent-frontend`
**Created**: 2025-12-18
**Plan**: [plan.md](plan.md)
**Spec**: [spec.md](spec.md)

> **Historical visual scope**: The visual choices and design tasks below record feature 007's original plan; their wording and completion states are preserved, not renewed aesthetic requirements. Current visual work follows [Principle XI](../../.specify/memory/constitution.md#xi-design-system-compliance--consistency) and [DESIGN_PRINCIPLES.md](../../web/src/design-system/docs/DESIGN_PRINCIPLES.md); changing that direction requires an accepted ADR.

## Task Summary

**Total Tasks**: 74
**Parallelizable**: 45
**User Stories**: 3 (P1, P2, P3)

### Task Breakdown by Phase
- Phase 1 (Setup): 8 tasks
- Phase 2 (Foundational): 10 tasks
- Phase 3 (US1 - View Delegations): 12 tasks
- Phase 4 (US2 - Review Grants): 8 tasks
- Phase 5 (US3 - Manage Delegation): 16 tasks
- Phase 6 (Polish): 20 tasks

### MVP Scope
**Recommended MVP**: Phase 1-3 (US1 only)
- Complete setup and foundational work
- Deliver User Story 1: View Active Delegations
- Provides immediate transparency value
- ~30 tasks, fully testable, independently deployable

---

## Phase 1: Setup & Project Initialization

**Goal**: Initialize project structure, install dependencies, configure build tooling.

**Duration**: ~2-3 hours

### Tasks

- [ ] T001 Create web/ directory structure per plan.md (src/, public/, config files)
- [ ] T002 [P] Initialize Node.js project with package.json in web/
- [ ] T003 [P] Initialize Go module updates in internal/ for new packages
- [ ] T004 Install frontend dependencies: React, TypeScript, Vite, Tailwind, Headless UI, Framer Motion, date-fns in web/package.json
- [ ] T005 [P] Configure Vite build system in web/vite.config.ts with /consent base path and dist output
- [ ] T006 [P] Configure Tailwind CSS v4.0 in web/tailwind.config.ts with design system colors and fonts
- [ ] T007 [P] Configure TypeScript in web/tsconfig.json with strict mode and path aliases
- [ ] T008 [P] Add frontend build targets to justfile (web-install, web-dev, web-build, build-all)

**Verification**: Run `cd web && npm install && npm run build` successfully, output appears in dist/consent/.

---

## Phase 2: Foundational Infrastructure

**Goal**: Implement blocking prerequisites needed by all user stories.

**Duration**: ~4-6 hours

**Blockers**: None (can start after Phase 1)

### Backend Infrastructure

- [ ] T009 Add SPAConfig struct to internal/config/schema.go with static_files_path and serve_enabled fields
- [ ] T010 [P] Add ListByPrincipal(ctx, principal) method to UserGrantRepository interface in internal/ports/storage.go
- [ ] T011 [P] Implement ListByPrincipal in memory adapter at internal/adapters/storage/memory/user_grant.go
- [ ] T012 [P] Implement ListByPrincipal in postgres adapter at internal/adapters/storage/postgres/user_grant.go with expiration filtering
- [ ] T013 Create ConsentService in internal/services/consent.go with GetAgentDelegations method
- [ ] T014 [P] Implement SPA serving handler in internal/adapters/http/handlers/spa.go with History API fallback
- [ ] T015 [P] Add CORS middleware configuration in internal/adapters/http/middleware/cors.go for /api/consent/* routes
- [ ] T016 Register SPA routes and consent routes in internal/adapters/http/server.go setupEnduserRoutes()
- [ ] T017 Add example SPA configuration to examples/config/consent-frontend.yaml
- [ ] T018 Write unit tests for ConsentService.GetAgentDelegations with table-driven test cases

**Verification**: Backend serves index.html at /consent, handles client-side routes, CORS headers present.

### Frontend Infrastructure

- [ ] T019 [P] Create global styles in web/src/styles/index.css with Tailwind imports
- [ ] T020 [P] Create font declarations in web/src/styles/fonts.css for Crimson Pro, Manrope, JetBrains Mono
- [ ] T021 [P] Download and add font files to web/src/assets/fonts/
- [ ] T022 Create API client in web/src/services/api/client.ts with Axios, interceptors, error handling
- [ ] T023 [P] Create TypeScript types in web/src/types/consent.ts matching data-model.md
- [ ] T024 [P] Create session storage utilities in web/src/services/storage/session.ts
- [ ] T025 Create App.tsx with React Router setup (BrowserRouter with /consent basename)
- [ ] T026 [P] Create ErrorBoundary component in web/src/components/ui/ErrorBoundary.tsx
- [ ] T027 [P] Create AppLayout component in web/src/components/layout/AppLayout.tsx
- [ ] T028 Update web/src/main.tsx to mount React app with providers

**Verification**: Frontend dev server starts, routing works, API client configured, types compile.

---

## Phase 3: User Story 1 - View Active Delegations (P1)

**Goal**: Enable users to see which agents they have granted access to at a glance.

**Story Priority**: P1 (Highest)

**Blockers**: Phase 2 must be complete

**Independent Test**: Navigate to /consent while authenticated, verify user sees their principal and list of delegated agents. Works without US2/US3.

**Why Independent**: This story provides immediate transparency value. Backend lists agents from grants, frontend displays them. No grant modification needed.

### Backend API - GET /api/consent/agents

- [ ] T029 [US1] Create AgentDelegation domain type in internal/domain/consent.go
- [ ] T030 [US1] Implement ConsentService.GetAgentDelegations method joining grants with agents
- [ ] T031 [US1] Create consent agents handler in internal/adapters/http/handlers/consent_agents.go
- [ ] T032 [US1] Create DTO types in internal/adapters/http/dto/consent.go for GetAgentDelegationsResponse
- [ ] T033 [US1] Register GET /api/consent/agents route in setupEnduserRoutes() with auth middleware
- [ ] T034 [P] [US1] Write unit tests for consent agents handler with mocked service
- [ ] T035 [P] [US1] Write integration tests for GET /api/consent/agents with testcontainers

### Backend API - GET /api/me

- [ ] T036 [P] [US1] Create UserInfo domain type in internal/domain/consent.go
- [ ] T037 [P] [US1] Create user info handler in internal/adapters/http/handlers/user_info.go extracting principal from session
- [ ] T038 [P] [US1] Register GET /api/me route in setupEnduserRoutes()
- [ ] T039 [P] [US1] Write unit tests for user info handler

### Frontend UI Components

- [ ] T040 [P] [US1] Create Skeleton component in web/src/components/ui/Skeleton.tsx with variants
- [ ] T041 [P] [US1] Create DelegationListSkeleton in web/src/components/ui/Skeleton.tsx
- [ ] T042 [P] [US1] Create InlineError component in web/src/components/ui/InlineError.tsx with retry button
- [ ] T043 [P] [US1] Create EmptyState component in web/src/components/ui/EmptyState.tsx
- [ ] T044 [US1] Create DelegationCard component in web/src/components/consent/DelegationCard.tsx
- [ ] T045 [US1] Create DelegationList component in web/src/components/consent/DelegationList.tsx with Framer Motion

### Frontend Data Fetching

- [ ] T046 [US1] Create consent API functions in web/src/services/api/consent.ts (fetchAgentDelegations, fetchUserInfo)
- [ ] T047 [US1] Create useConsent hook in web/src/hooks/useConsent.ts with loading/error states
- [ ] T048 [P] [US1] Create useRetry hook in web/src/hooks/useRetry.ts with exponential backoff

### Frontend Pages

- [ ] T049 [US1] Create ConsentOverviewPage in web/src/pages/ConsentOverviewPage.tsx using hooks and components
- [ ] T050 [US1] Add /consent route to App.tsx routing configuration

### Testing

- [ ] T051 [P] [US1] Write unit tests for useConsent hook with Vitest
- [ ] T052 [P] [US1] Write component tests for DelegationCard with React Testing Library
- [ ] T053 [P] [US1] Write component tests for ConsentOverviewPage covering loading/error/empty/data states

**Phase 3 Verification**:
1. Start backend: `just dev`
2. Start frontend: `cd web && npm run dev`
3. Navigate to http://localhost:3000/consent
4. Verify: User principal displays, agent list shows (or empty state if no grants)
5. Verify: Skeleton screens show while loading
6. Verify: Error message with retry button if API fails

**Deliverable**: Fully functional consent overview page with transparency into user's delegations.

---

## Phase 4: User Story 2 - Review Agent-Specific Grants (P2)

**Goal**: Enable users to review detailed grant information for a specific agent.

**Story Priority**: P2

**Blockers**: Phase 3 (US1) must be complete (navigation from agent list)

**Independent Test**: Navigate to /consent/agent/{agent-id}, verify agent info and service list display. Works without US3 (view-only, no modification).

**Why Independent**: Builds on US1 navigation but focuses on detailed information display. No grant modification logic needed.

### Backend API - GET /api/consent/agent/:agent-id

- [ ] T054 [US2] Create AgentDetail domain type in internal/domain/consent.go
- [ ] T055 [US2] Create ThirdpartyService domain type in internal/domain/consent.go
- [ ] T056 [US2] Implement ConsentService.GetAgentDetail method fetching agent + services
- [ ] T057 [US2] Create agent detail handler in internal/adapters/http/handlers/agent_detail.go
- [ ] T058 [US2] Register GET /api/consent/agent/:agent-id route
- [ ] T059 [P] [US2] Write unit tests for agent detail handler
- [ ] T060 [P] [US2] Write integration tests for GET /api/consent/agent/:agent-id

### Backend API - GET /api/consent/agent/:agent-id/grants

- [ ] T061 [P] [US2] Implement ConsentService.GetUserGrants method filtering by principal and agent
- [ ] T062 [P] [US2] Create grants handler in internal/adapters/http/handlers/grants.go for GET
- [ ] T063 [P] [US2] Register GET /api/consent/agent/:agent-id/grants route
- [ ] T064 [P] [US2] Write unit tests for grants GET handler

### Frontend UI Components

- [ ] T065 [P] [US2] Create ServiceCard component in web/src/components/consent/ServiceCard.tsx (display-only)
- [ ] T066 [P] [US2] Create ScopeList component in web/src/components/consent/ScopeList.tsx with expand/collapse
- [ ] T067 [P] [US2] Create GrantStatusBadge component in web/src/components/consent/GrantStatusBadge.tsx

### Frontend Data Fetching & Pages

- [ ] T068 [US2] Add fetchAgentDetail and fetchAgentGrants to web/src/services/api/consent.ts
- [ ] T069 [US2] Create useAgentGrants hook in web/src/hooks/useAgentGrants.ts
- [ ] T070 [US2] Create AgentGrantDetailPage in web/src/pages/AgentGrantDetailPage.tsx (view-only mode)
- [ ] T071 [US2] Add /consent/agent/:agentId route to App.tsx

### Testing

- [ ] T072 [P] [US2] Write component tests for ServiceCard (display mode)
- [ ] T073 [P] [US2] Write component tests for AgentGrantDetailPage covering agent not found, services loading, grants display

**Phase 4 Verification**:
1. Navigate to /consent, click on an agent
2. Verify: Agent detail page loads with name, description, governance links
3. Verify: All configured services display with their scopes
4. Verify: Existing grant state shows correctly (if user has grants)
5. Verify: Skeleton screens during load, error handling works

**Deliverable**: View-only agent detail page showing all available services and existing grants.

---

## Phase 5: User Story 3 - Manage Service Delegation (P3)

**Goal**: Enable users to create/update/revoke grants with service and scope selection.

**Story Priority**: P3

**Blockers**: Phase 4 (US2) must be complete (need grant review UI as foundation)

**Independent Test**: Navigate to /consent/agent/{agent-id}, toggle services on/off, select scopes, click "Approve & Delegate", verify grant created/updated via API.

**Why Independent**: Builds on US2's display but adds modification logic. US1 and US2 provide navigation and context, this story adds action capability.

### Backend API - POST /api/consent/agent/:agent-id/grants

- [ ] T074 [US3] Create CreateOrUpdateGrantRequest DTO in internal/adapters/http/dto/consent.go with validation
- [ ] T075 [US3] Implement ConsentService.CreateOrUpdateGrant with scope validation and upsert logic
- [ ] T076 [US3] Add POST handler to grants.go for create/update grant
- [ ] T077 [US3] Implement grant validation logic (scopes exist, future dates)
- [ ] T078 [US3] Add CSRF protection to POST /api/consent/agent/:agent-id/grants route
- [ ] T079 [P] [US3] Write unit tests for CreateOrUpdateGrant service method with table-driven tests
- [ ] T080 [P] [US3] Write integration tests for POST /api/consent/agent/:agent-id/grants covering create, update, revoke

### Frontend UI Components - Interactive

- [ ] T081 [P] [US3] Create Switch component in web/src/components/ui/Switch.tsx wrapping Headless UI
- [ ] T082 [P] [US3] Create DatePicker component in web/src/components/ui/DatePicker.tsx
- [ ] T083 [P] [US3] Create Button component in web/src/components/ui/Button.tsx with variants (primary, secondary, loading)
- [ ] T084 [US3] Create GrantValidityControl in web/src/components/consent/GrantValidityControl.tsx with checkbox and date picker
- [ ] T085 [US3] Update ServiceCard to interactive mode with toggle and scope checkboxes in web/src/components/consent/ServiceCard.tsx
- [ ] T086 [US3] Create ServiceGrantList in web/src/components/consent/ServiceGrantList.tsx managing multiple service states

### Frontend State Management

- [ ] T087 [US3] Create useToggleGrant hook in web/src/hooks/useToggleGrant.ts with optimistic updates
- [ ] T088 [US3] Create useUpdateValidity hook in web/src/hooks/useUpdateValidity.ts
- [ ] T089 [US3] Create grant validation utilities in web/src/utils/validation.ts

### Frontend Grant Submission

- [ ] T090 [US3] Add createOrUpdateGrant API function to web/src/services/api/consent.ts
- [ ] T091 [US3] Update AgentGrantDetailPage to interactive mode with grant submission logic
- [ ] T092 [US3] Implement "Approve & Delegate" button handler with payload construction
- [ ] T093 [US3] Implement grant state refresh after successful submission
- [ ] T094 [US3] Implement inline error display with retry for failed submissions

### Testing

- [ ] T095 [P] [US3] Write unit tests for useToggleGrant hook covering optimistic updates and rollback
- [ ] T096 [P] [US3] Write component tests for GrantValidityControl covering checkbox/date picker interactions
- [ ] T097 [P] [US3] Write component tests for ServiceCard interactive mode
- [ ] T098 [P] [US3] Write integration tests for AgentGrantDetailPage grant submission flow

**Phase 5 Verification**:
1. Navigate to /consent/agent/{agent-id}
2. Toggle services on/off, verify immediate UI feedback
3. Expand service, select specific scopes
4. Set grant expiration or leave indefinite
5. Click "Approve & Delegate", verify success message
6. Refresh page, verify grant persisted correctly
7. Toggle all services off, submit, verify grant revoked

**Deliverable**: Fully functional grant management with create, update, and revoke capabilities.

---

## Phase 6: Polish & Cross-Cutting Concerns

**Goal**: Documentation, additional UI polish, performance optimization, deployment readiness.

**Duration**: ~3-4 hours

**Blockers**: All user stories (US1-US3) complete

### Documentation

- [ ] T099 Update ARCHITECTURE.md with web/ folder structure and SPA serving pattern
- [ ] T100 [P] Create API documentation in docs/api/consent-endpoints.md with examples
- [ ] T101 [P] Update README.md with consent frontend development instructions
- [ ] T102 [P] Document environment variables for SPA configuration

### ADRs (if needed)

- [ ] T103 Create ADR for SPA serving pattern (embed.FS + History API fallback) in adrs/00X-spa-serving.md if pattern is novel
- [ ] T104 [P] Create ADR for frontend technology stack (React + Tailwind v4.0) in adrs/00Y-frontend-stack.md if justified

### UI Polish

- [ ] T105 [P] Add loading indicators to Button component during async operations
- [ ] T106 [P] Add success toast/notification after grant submission
- [ ] T107 [P] Implement smooth scroll to error messages after validation failures
- [ ] T108 [P] Add animation transitions between pages using Framer Motion
- [ ] T109 [P] Optimize font loading with font-display: swap
- [ ] T110 [P] Add favicon and meta tags to web/index.html

### Performance & Optimization

- [ ] T111 [P] Implement React.memo for expensive components (DelegationList, ServiceGrantList)
- [ ] T112 [P] Add query result caching for agent and service data
- [ ] T113 [P] Optimize bundle size with code splitting for routes
- [ ] T114 [P] Add service worker for offline manifest (optional)

### Error Handling & Resilience

- [ ] T115 [P] Implement global error boundary with fallback UI
- [ ] T116 [P] Add retry logic with exponential backoff for API failures
- [ ] T117 [P] Implement authentication failure redirect with return path preservation
- [ ] T118 [P] Add validation for edge cases (agent not found, service unavailable)

**Phase 6 Verification**:
1. Documentation complete and accurate
2. UI animations smooth, loading states clear
3. Bundle size optimized (<500KB gzipped)
4. Error handling comprehensive
5. Performance metrics meet goals (page load <2s, API <2s)

**Deliverable**: Production-ready consent management frontend with complete documentation.

---

## Implementation Strategy

### MVP Approach (Recommended)

**Phase 1-3 Only** (US1: View Delegations)
- Provides immediate transparency value
- ~30 tasks, fully testable
- Backend: List agents API, SPA serving
- Frontend: Overview page with delegation list
- Deploy and gather feedback before US2/US3

### Incremental Delivery

1. **Week 1**: Phase 1-2 (Setup + Foundation) → Deployable skeleton
2. **Week 2**: Phase 3 (US1) → MVP with transparency
3. **Week 3**: Phase 4 (US2) → Add detailed grant review
4. **Week 4**: Phase 5 (US3) → Full grant management
5. **Week 5**: Phase 6 (Polish) → Production ready

### Parallel Execution Opportunities

**Phase 1** (All parallel after T001):
- T002-T008 can run concurrently

**Phase 2** (Backend and Frontend in parallel):
- Backend: T009-T018 (sequential dependency chain)
- Frontend: T019-T028 (all parallel after T019)

**Phase 3 (US1)** (Maximum parallelization):
- Backend APIs: T029-T035 → T036-T039 (two parallel tracks)
- Frontend components: T040-T045 (all parallel)
- Frontend hooks: T046-T048 (all parallel)
- Pages: T049-T050 (after components/hooks)
- Tests: T051-T053 (all parallel, after implementation)

**Phase 4 (US2)**:
- Backend: T054-T060 → T061-T064 (two parallel tracks)
- Frontend: T065-T067 (all parallel) → T068-T071 → T072-T073

**Phase 5 (US3)**:
- Backend: T074-T080 (mostly parallel)
- Frontend components: T081-T086 (all parallel)
- Frontend state: T087-T089 (all parallel)
- Integration: T090-T094 (sequential)
- Tests: T095-T098 (all parallel)

**Phase 6**:
- Documentation: T099-T104 (all parallel)
- UI Polish: T105-T110 (all parallel)
- Performance: T111-T114 (all parallel)
- Error handling: T115-T118 (all parallel)

---

## Dependencies & Execution Order

### Story Dependencies

```
Phase 1 (Setup)
    ↓
Phase 2 (Foundation)
    ↓
Phase 3 (US1) ←──────┐
    ↓                 │ (Independent)
Phase 4 (US2) ←───────┤
    ↓                 │
Phase 5 (US3) ←───────┘
    ↓
Phase 6 (Polish)
```

**Critical Path**: Setup → Foundation → US1 → US2 → US3 → Polish

**Parallelizable Branches**:
- US1 backend and frontend can develop in parallel after Foundation
- US2 backend can start while US1 frontend completes
- US3 backend can start while US2 frontend completes

### Task-Level Dependencies

**Blocking Tasks** (must complete before others):
- T001: Creates directory structure (blocks all Phase 1)
- T009-T018: Foundation backend (blocks US1-US3 backend)
- T019-T028: Foundation frontend (blocks US1-US3 frontend)
- T049-T050: US1 pages (blocks US2 navigation)
- T070-T071: US2 pages (blocks US3 grant management)

**Parallelizable Tasks** (marked with [P]):
- 45 out of 118 tasks can run in parallel
- Most component, test, and documentation tasks
- Backend and frontend streams can run simultaneously

---

## Testing Strategy

### Unit Tests (Vitest + Go testing)
- Backend: All service methods, handlers, repositories
- Frontend: All custom hooks, utility functions
- Target: 85%+ coverage

### Component Tests (React Testing Library)
- All UI components with user interactions
- Page components with loading/error/data states
- Target: 80%+ coverage

### Integration Tests (testcontainers + Playwright)
- Backend: Repository queries with real PostgreSQL
- Frontend: E2E user flows (view, review, manage)
- Target: Critical paths covered

### Test Organization
- Backend tests: `internal/*/testdata/` and `*_test.go` files
- Frontend tests: `web/src/**/*.test.ts` and `web/src/**/*.test.tsx` files
- E2E tests: `web/tests/e2e/` directory

---

## Success Criteria

### Phase 3 (US1 MVP) Success:
- [ ] User can view all agents they've delegated to
- [ ] User identity displays correctly
- [ ] Empty state shows when no delegations
- [ ] Loading states with skeleton screens
- [ ] Error recovery with retry buttons
- [ ] Navigation to agent detail works

### Phase 4 (US2) Success:
- [ ] Agent detail page shows metadata and governance links
- [ ] All services display with their scopes
- [ ] Existing grant state reflects correctly
- [ ] View-only mode works without modification capability

### Phase 5 (US3) Success:
- [ ] Service toggles work with optimistic updates
- [ ] Scope selection updates grant payload
- [ ] Grant expiration controls function correctly
- [ ] "Approve & Delegate" creates/updates grants
- [ ] Grant revocation works (empty scopes)
- [ ] Success/error feedback clear and actionable

### Phase 6 (Polish) Success:
- [ ] Documentation complete and accurate
- [ ] Performance metrics met (<2s load, <2s API)
- [ ] Bundle optimized (<500KB gzipped)
- [ ] Error handling comprehensive
- [ ] ADRs created for novel patterns

---

## Notes

1. **Test-Driven Development**: Tests are INCLUDED as they're explicitly requested in the feature specification (Principle VIII) and quickstart.md emphasizes automated testing.

2. **Constitution Compliance**: All tasks follow constitution principles:
   - Security-first (authentication, authorization, validation)
   - ADRs binding (follow existing, create new for SPA serving)
   - Hexagonal architecture (ports/adapters separation)
   - Unified configuration (SPAConfig)
   - Persistence patterns (sqlx, quickstart.md)
   - Automated testing (unit, integration, E2E)

3. **File Paths**: All tasks include explicit file paths for immediate execution.

4. **Independent Stories**: Each user story is independently testable:
   - US1: Works standalone (transparency only)
   - US2: Works standalone (view-only, no modification)
   - US3: Builds on US2 (adds modification logic)

5. **Parallelization**: 45 tasks marked [P] for parallel execution (38% of total).

6. **MVP Focus**: Phase 1-3 (US1) recommended as MVP for early feedback.

---

**Ready for Implementation**: All tasks are executable, dependencies clear, success criteria defined. Start with Phase 1 and progress sequentially through phases, parallelizing within phases where marked.
