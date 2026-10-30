# ADR 20251218-120000: SPA Serving Pattern

**Status**: Accepted
> **Superseded in part by [ADR 20260904-120000](./20260904-120000-root-mounted-spa.md)**: the root-mounted SPA replaces the serving path and fallback prefix, Vite base and output directory, asset path, and SPA configuration references.

**Date**: 2025-12-18
**Feature**: 20251217-120100-consent-frontend

## Context

The consent frontend is a React-based Single Page Application (SPA) that provides a user interface for managing OAuth2 delegations to AI agents. We need to decide how to deploy and serve this SPA in relation to the Go backend API.

The SPA needs to:
1. Be accessible to end users on the enduser server (port 8080)
2. Make API requests to `/api/consent/*` endpoints
3. Handle client-side routing for multiple views
4. Be deployed and scaled alongside the Go backend
5. Maintain session state and CSRF protection

## Decision

We will **serve the React SPA directly from the Go backend** using Go's `embed` package or filesystem serving, with the following architecture:

1. **Build Process**: React SPA is built with Vite to `web/dist/consent/` directory
2. **Serving**: Go backend serves static files from `/consent` path
3. **Embedding**: Static files are embedded in the Go binary using `//go:embed` (or served from filesystem in development)
4. **History API Fallback**: Go serves `index.html` for all `/consent/*` routes (SPA fallback)
5. **API Routes**: API requests to `/api/*` are handled by Go HTTP handlers
6. **Single Port**: Both SPA and API served on the same port (8080)

**Request Flow:**
```
User Request: https://example.com/consent/agents
  ↓
Go HTTP Server (Port 8080)
  ↓
Static File Handler (/consent/*)
  ↓ (404 fallback for client-side routes)
Serve index.html
  ↓
Browser loads React app
  ↓
React Router handles /agents route
  ↓
Component makes API call: GET /api/consent/agents
  ↓
Go API Handler returns JSON
```

## Rationale

### Advantages of Backend-Served SPA

1. **Single Deployment**: One binary/container contains both frontend and backend
   - Simplified deployment pipeline
   - Single Docker image
   - Single Kubernetes deployment
   - Easier version synchronization

2. **No CORS Complexity**: Same-origin requests from SPA to API
   - No CORS preflight requests
   - Simplified security model
   - Cookies work seamlessly
   - CSRF tokens easier to manage

3. **Simplified Infrastructure**: One service to run and monitor
   - Single load balancer/ingress
   - Single SSL certificate
   - Single domain
   - Fewer moving parts

4. **Consistent Authentication**: Reverse proxy sets headers for both SPA and API
   - Principal header (`X-Principal`) available for both
   - Session state shared
   - No token passing between domains

5. **Atomic Updates**: Frontend and backend version always match
   - No version skew issues
   - API and UI changes deployed together
   - Easier rollback

6. **Development Simplicity**: Developers work with single service
   - One repository
   - One deployment to test
   - Easier integration testing

7. **Cost Efficiency**: No separate CDN or static hosting required
   - Reduced infrastructure costs
   - Simpler billing
   - Fewer external dependencies

### Alternatives Considered

#### Alternative 1: Separate Frontend Deployment (CDN)

**Description**: Deploy SPA to CDN (S3 + CloudFront, Netlify, Vercel), API on separate domain.

**Advantages**:
- Better caching at edge locations
- Lower backend load for static assets
- Can use CDN-specific features (geo-routing, etc.)
- Independent scaling of frontend and backend

**Disadvantages**:
- CORS required (complexity, security, performance)
- Cross-domain authentication challenging (cookies, CSRF)
- Version synchronization issues (API v2.0 with UI v1.9)
- Two separate deployment pipelines
- Higher infrastructure complexity
- Additional costs (CDN, DNS, certificates)
- Harder to test locally

**Decision**: Rejected due to complexity and CORS challenges.

#### Alternative 2: Separate Frontend Server (Node.js)

**Description**: Run separate Node.js server to serve SPA (Express, Next.js SSR).

**Advantages**:
- Better suited for server-side rendering (SSR)
- Can leverage Node.js ecosystem for frontend
- Independent scaling

**Disadvantages**:
- Two services to deploy and monitor
- CORS required if different domains
- Version synchronization complexity
- Additional infrastructure (Node.js runtime, monitoring)
- Two language ecosystems (Go + Node.js)
- Overkill for static SPA (no SSR needed)

**Decision**: Rejected as unnecessary complexity for static SPA.

#### Alternative 3: Reverse Proxy Routing (nginx)

**Description**: nginx routes `/consent` to separate frontend server, `/api` to backend.

**Advantages**:
- Clean separation of concerns
- Can cache static assets at nginx level
- Independent deployment

**Disadvantages**:
- Requires nginx or similar proxy
- More complex architecture
- Version synchronization issues
- Two services to manage
- Doesn't solve CORS for API calls from browser

**Decision**: Rejected due to increased complexity.

#### Alternative 4: Embed in Go Binary

**Description**: Use `//go:embed` to embed SPA assets directly in Go binary.

**Advantages**:
- Single binary contains everything
- No external file dependencies
- Immutable deployments
- Faster startup (no file I/O)

**Disadvantages**:
- Larger binary size (~2-5 MB for SPA)
- Must rebuild Go binary for frontend changes
- Less flexible for frontend-only updates

**Decision**: **Accepted as primary pattern** with filesystem serving as fallback for development.

## Decision Details

### Implementation Approach

**Production (Embedded):**
```go
//go:embed web/dist/consent/*
var spaAssets embed.FS

func serveSPA(w http.ResponseWriter, r *http.Request) {
    // Serve embedded files with History API fallback
}
```

**Development (Filesystem):**
```go
func serveSPA(w http.ResponseWriter, r *http.Request) {
    // Serve from web/dist/consent/ directory
    // Allows rebuilding frontend without restarting backend
}
```

**Configuration:**
```yaml
spa:
  static_files_path: web/dist/consent  # For filesystem serving
  serve_enabled: true                  # Toggle SPA serving
  base_path: /consent                  # URL base path
```

### History API Fallback

The Go handler implements History API fallback for client-side routing:

```
Request: /consent/agents
  ↓
Check if file exists: /consent/agents
  ↓ (no)
Serve index.html
  ↓
React Router: <Route path="/agents" />
```

This allows React Router to handle all `/consent/*` routes without 404 errors.

### Build Process

```bash
# 1. Build frontend
cd web && npm run build
# Output: web/dist/consent/index.html, assets/, etc.

# 2. Build backend (embeds frontend)
go build -o bin/agentic-identity-broker ./cmd/agentic-identity-broker
# Binary contains embedded SPA

# 3. Deploy single binary
./bin/agentic-identity-broker
```

## Consequences

### Positive

1. **Simplified Architecture**: Single service, single deployment
2. **No CORS**: Same-origin requests work seamlessly
3. **Version Synchronization**: Frontend and backend always in sync
4. **Easier Development**: One service to run locally
5. **Atomic Deployments**: One binary, one rollback
6. **Cost Efficiency**: No CDN or separate hosting needed
7. **Security**: Simpler authentication and CSRF model

### Negative

1. **Binary Size**: Go binary includes SPA assets (~2-5 MB larger)
2. **Frontend Updates**: Require backend rebuild in production
3. **Caching**: No edge caching (CDN), must cache at origin
4. **Scaling**: Frontend traffic affects backend (but Go can handle this easily)

### Neutral

1. **Flexibility**: Can switch to CDN later if needed (with CORS)
2. **Development**: Can use Vite dev server with proxy for HMR

### Migration Path

If CDN deployment becomes necessary:
1. Set `SPA_SERVE_ENABLED=false` on backend
2. Build SPA with API URL: `VITE_API_BASE_URL=https://api.example.com`
3. Upload `web/dist/consent/` to CDN
4. Configure CORS on backend
5. Handle cross-domain authentication

## Implementation Notes

### File Structure

```
web/
├── dist/consent/        # Build output
│   ├── index.html       # SPA entry point
│   ├── assets/          # JS, CSS, images
│   └── favicon.ico
├── src/                 # React source
└── vite.config.ts       # Build config

internal/adapters/http/
└── handlers/
    └── spa.go           # SPA serving handler
```

### Vite Configuration

```typescript
// vite.config.ts
export default defineConfig({
  base: '/consent',              // Must match Go route
  build: {
    outDir: 'dist/consent',      // Output directory
    sourcemap: false,            // No source maps in prod
    minify: 'terser',            // Minification
  },
});
```

### Go Handler

```go
// Serve SPA with History API fallback
r.Get("/consent/*", spaHandler.ServeSPA)

// API routes take precedence
r.Route("/api/consent", func(r chi.Router) {
    r.Get("/agents", agentsHandler.GetAgentDelegations)
    // ...
})
```

## Performance Considerations

1. **Static Asset Caching**: Set cache headers for versioned assets
   ```go
   w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
   ```

2. **Compression**: Enable gzip/brotli compression in Go server
   ```go
   r.Use(middleware.Compress(5))
   ```

3. **Content Hashing**: Vite generates content-hashed filenames
   ```
   assets/index-a1b2c3d4.js  # Immutable caching
   ```

4. **Bundle Splitting**: Vite automatically splits code
   - Vendor chunk (React, libraries)
   - App chunk (application code)
   - Route chunks (lazy loaded)

## Security Considerations

1. **Content Security Policy**: Set CSP headers on SPA responses
2. **CSRF Protection**: Automatic with same-origin requests
3. **XSS Protection**: React escapes user input by default
4. **Clickjacking**: Set `X-Frame-Options` header
5. **MIME Sniffing**: Set `X-Content-Type-Options: nosniff`

## Testing Strategy

1. **Unit Tests**: React components (Vitest)
2. **Integration Tests**: SPA + API interaction (Go test with embedded frontend)
3. **E2E Tests**: (Future) Playwright tests against deployed service

## Monitoring

1. **Backend Metrics**: Track static file requests
2. **Performance**: Monitor SPA load times
3. **Errors**: Log SPA serving errors
4. **Usage**: Track API calls from SPA

## References

- [Go embed package](https://pkg.go.dev/embed)
- [Vite build configuration](https://vitejs.dev/config/)
- [React Router History API](https://reactrouter.com/en/main/start/tutorial)
- Research document: `specs/20251217-120100-consent-frontend/research.md`
- Related ADR: [20251218-120100-frontend-stack.md](./20251218-120100-frontend-stack.md)
