# OAuth2 Sessions Management - Frontend Implementation

This directory contains the React frontend components for OAuth2 session management.

## Overview

Users can view all their active OAuth2 sessions with third-party services, see detailed status information, and manage session lifecycles.

## Design authority and example status

[DESIGN_PRINCIPLES.md](../../design-system/docs/DESIGN_PRINCIPLES.md) defines the current Refined Trust Architecture direction.
ADR 037 remains Proposed. The redesign target is not current runtime guidance.
Existing examples describe session components, not constitutional aesthetic requirements or verified accessibility results.
Principle XI requires semantic tokens, light/dark story accessibility checks, self-hosted assets, and WCAG 2.1 AA.
Do not copy raw palette utilities into components. Use existing semantic roles and component variants.
Do not load service images from third-party origins. Use repository-hosted assets or a local fallback.
After ADR acceptance, align this file and `DESIGN_SYSTEM_USAGE.md` with the implementation in the single cutover.
Do not use redesign phases, feature flags, or compatibility paths.

## Components

### SessionCard

A card component that displays a single OAuth2 session with comprehensive metadata and actions.

**Features:**

- Status badge with 4 states:
  - **Active**: Session is valid and operational
  - **Expiring Soon**: Refresh token expires within 7 days
  - **Access Token Expired**: Access token expired but refresh token valid
  - **Expired**: Entire session expired
- Encryption indicator (lock icon)
- Dependent agent count (shows how many agents use this session)
- Session initiation timestamp
- OAuth2 scopes display (as pills/badges)
- Action buttons:
  - View Details (optional, triggers modal/navigation)
  - Terminate (with confirmation dialog)

**Props:**

```typescript
interface SessionCardProps {
  session: SessionSummary; // Session data to display
  onTerminate: (serviceId: string) => void; // Termination callback
  onViewDetails?: (serviceId: string) => void; // Optional details callback
  loading?: boolean; // Loading state for actions
}
```

**Example Usage:**

```tsx
<SessionCard
  session={session}
  onTerminate={handleTerminate}
  onViewDetails={handleViewDetails}
  loading={isTerminating}
/>
```

## Pages

### ThirdPartySessionsPage

Main page component that displays all OAuth2 sessions in a responsive grid layout.

**Features:**

- Responsive grid layout (1 column mobile, 2 tablet, 3 desktop)
- Loading state with skeleton cards
- Error state with retry button
- Empty state when no sessions exist
- Confirmation dialog before session termination
- Real-time error handling with user feedback

**Route:**

- Path: `/sessions`
- Base: `/` (configured in App.tsx)
- Full URL: `http://localhost:3000/sessions`

**States:**

1. **Loading**: Shows 3 skeleton cards in grid
2. **Error**: Shows error alert with retry button
3. **Empty**: Shows empty state with illustration and refresh button
4. **Content**: Shows session cards in responsive grid

## API Integration

### Sessions API Service

Located in `/web/src/services/api/sessions.ts`

**Endpoints:**

- `GET /api/third-party/sessions` - List all sessions
- `GET /api/third-party/:service-id/session` - Get session details
- `DELETE /api/third-party/:service-id/session` - Terminate session
- `POST /api/third-party/:service-id/session/refresh` - Refresh session

**Caching:**

- Cache TTL: 2 minutes
- Cache invalidation on mutations (terminate, refresh)

### useSessions Hook

Custom React hook for fetching and managing sessions state.

**API:**

```typescript
const {
  sessions, // SessionSummary[]
  loading, // boolean
  error, // string | null
  refetch, // () => Promise<void>
} = useSessions();
```

**Features:**

- Automatic data fetching on mount
- Loading state management
- Error handling with user-friendly messages
- Manual refetch capability
- Cleanup on unmount

## Design System Compliance

These components use the current **Refined Trust Architecture** design system. The following values describe existing usage, not new palette requirements:

### Colors

- Success (Active): `success-primary` (green)
- Warning (Expiring Soon, Access Token Expired): `warning-primary` (amber)
- Error (Expired): `error-primary` (red)
- Neutral values in existing source: `neutral-300` to `neutral-900`. New component styling must use semantic roles instead.

### Components Used

- `Card` - Container with header, body, footer
- `Button` - Primary, outline, danger variants
- `Badge` - Status indicators with dot and variants
- `StatusIndicator` - Icon + label for metadata
- `Stack` - Flexbox layout (row/column)
- `Grid` - CSS Grid layout (responsive)
- `Skeleton` - Loading placeholders
- `Alert` - Error/warning messages
- `EmptyState` - No data placeholder

### Accessibility

- WCAG 2.1 AA is required. Verify composed surfaces and component stories in both themes.
- Semantic HTML (h1, h2, h3 hierarchy)
- Proper ARIA attributes
- Keyboard navigation support
- Focus management in dialogs
- Color + icon redundancy (never color alone)
- Screen reader friendly

## Testing

### SessionCard Tests

Location: `SessionCard.test.tsx`

**Coverage:**

- Component rendering with different states
- Status badge variants
- Action button behavior
- Loading state handling
- Accessibility compliance
- Date formatting

### useSessions Tests

Location: `/web/src/hooks/useSessions.test.ts`

**Coverage:**

- Data fetching on mount
- Loading state management
- Error handling
- Refetch functionality
- Cleanup on unmount

**Run Tests:**

```bash
# Run all tests
cd web && npm test

# Run with coverage
npm run test:coverage

# Watch mode
npm test -- --watch
```

## Development Workflow

### Local Development

**Option 1: Hot Reload (Recommended)**

```bash
# Terminal 1: Start Go backend
just run

# Terminal 2: Start Vite dev server with HMR
just web-dev
```

Access frontend at: http://localhost:3000/sessions
API proxied to: http://localhost:8000

**Option 2: Production Build**

```bash
# Build frontend and serve from Go
just web-build && just run
```

Access at: http://localhost:8000/sessions

### Vite Proxy Configuration

The Vite dev server proxies API requests and injects authentication headers:

```javascript
// vite.config.ts
proxy: {
  '/api': {
    target: 'http://localhost:8000',
    changeOrigin: true,
    headers: {
      'X-Remote-User': 'dev@example.com',  // Simulates authenticated user
    },
  },
}
```

## File Structure

```
web/src/
├── components/
│   └── sessions/
│       ├── SessionCard.tsx           # Session card component
│       ├── SessionCard.test.tsx      # Component tests
│       ├── index.ts                  # Public exports
│       └── README.md                 # This file
├── pages/
│   └── ThirdPartySessionsPage.tsx    # Main sessions page
├── hooks/
│   ├── useSessions.ts                # Sessions data hook
│   └── useSessions.test.ts           # Hook tests
├── services/
│   └── api/
│       └── sessions.ts               # Sessions API client
└── App.tsx                           # Route configuration
```

## Backend API Contract

The frontend expects the following API response format:

```json
{
  "data": {
    "sessions": [
      {
        "id": "uuid-here",
        "service_id": "google",
        "service_display_name": "Google Drive",
        "token_type": "Bearer",
        "scope": ["read:email", "write:files"],
        "initiated_at": "2024-01-01T12:00:00Z",
        "is_expired": false,
        "access_token_expired": false,
        "refresh_token_expires_at": "2024-12-31T23:59:59Z",
        "dependent_agent_count": 2,
        "is_encrypted": true
      }
    ]
  }
}
```

## Historical session backlog — not the redesign delivery plan

These original session-story notes do not define feature 046 scope or delivery order.
The redesign plan owns the single-cutover requirements.

1. **User Story 2**: Terminate a Third-Party Session
   - Implement confirmation dialog refinements
   - Add dependent agents list in dialog
   - Implement revocation logic

2. **User Story 3**: View Session Details
   - Create SessionDetailPage component
   - Show dependent agents list with names/logos
   - Display token expiration timeline
   - Show audit log of session events

3. **OAuth2 Authorization Flow**:
   - Implement OAuth2 redirect handler
   - Create service selection UI
   - Add re-authentication flow for expired sessions

## Performance Optimizations

- Code splitting with React.lazy()
- API response caching (2 minute TTL)
- Memoized date formatting
- Skeleton loading states
- Responsive images (future: lazy loading)

## Security Considerations

- No tokens stored in frontend state
- All API calls use authentication headers
- HTTPS required in production
- CSP headers enforced
- XSS protection via React's built-in escaping

## Browser Support

- Chrome 90+
- Firefox 88+
- Safari 14+
- Edge 90+

## Contributing

When adding new features to this module:

1. Follow existing component patterns
2. Use TypeScript strict mode
3. Write comprehensive tests (>85% coverage)
4. Follow design system guidelines
5. Document props and behavior
6. Add Storybook stories (optional)
7. Run `just check` before committing
