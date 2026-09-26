# Quickstart: Request Principal Extraction

## Overview

This guide shows how to implement request principal extraction middleware for the agentic-identity-broker. The middleware extracts authenticated user principals from HTTP headers (set by a trusted reverse proxy) and makes them available throughout request processing via Go context.

**What You'll Build**:
- HTTP middleware that extracts principals from headers
- Configuration for principal header names
- Context utilities for type-safe principal propagation
- Protected and optional route patterns

**Time to Complete**: 30-45 minutes

**Prerequisites**:
- Go 1.24.0+
- chi/v5 v5.2.3 (already in go.mod)
- Existing HTTP server setup in `internal/adapters/http/`
- Familiarity with Go context and HTTP middleware patterns

## Architecture Overview

```
[Reverse Proxy]          [HTTP Middleware]              [Request Handler]
       │                        │                              │
       │  Sets header:          │  Extracts principal          │  Uses principal
       │  X-Remote-User         │  Validates                   │  from context
       │  "alice@example.com"   │  Adds to context            │
       │                        │                              │
       └───────────────────────>│                              │
                                │  ctx = WithPrincipal(...)   │
                                └─────────────────────────────>│
                                                               │
                                                          principal := FromContext(ctx)
```

**Components**:

1. **Configuration** (`internal/config/schema.go`): `authentication.preauth.principal_header_name` nested field
2. **Middleware** (`internal/adapters/http/principal_middleware.go`): Extract and validate
3. **Context Utilities** (`internal/domain/principal/context.go`): Type-safe storage/retrieval
4. **Route Groups** (chi router): Apply middleware to specific routes

**Configuration Design**: The configuration uses a nested structure (`authentication.preauth.principal_header_name`) to support future extensibility. This allows JWT authentication (`authentication.jwt.*`) to be added later without breaking existing configurations.

**Security Model**: The application trusts the reverse proxy to authenticate users. The reverse proxy sets a header (e.g., `X-Remote-User`) after successful authentication. The application extracts this header and uses it for authorization, audit logging, and business logic.

## Step 1: Add Configuration

### 1.1: Update Configuration Schema

Add the principal header configuration to `internal/config/schema.go`:

```go
// File: internal/config/schema.go

package config

// AuthenticationConfig holds authentication configuration for a server
type AuthenticationConfig struct {
    // Preauth holds configuration for pre-authentication (reverse proxy) mode
    Preauth PreauthConfig `mapstructure:"preauth"`

    // Future: JWT configuration can be added here
    // JWT JWTConfig `mapstructure:"jwt"`
}

// PreauthConfig holds configuration for reverse proxy pre-authentication
type PreauthConfig struct {
    // PrincipalHeaderName is the HTTP header from which principals are extracted
    // This header is set by a trusted reverse proxy after authentication.
    // Example values: "X-Remote-User", "X-Authenticated-User", "Remote-User"
    // Default: "X-Remote-User"
    PrincipalHeaderName string `mapstructure:"principal_header_name"`
}

// ServerInstanceConfig holds configuration for a single server instance
type ServerInstanceConfig struct {
    // Existing fields
    Port            int    `mapstructure:"port"`
    Name            string `mapstructure:"name"`
    ReadTimeout     int    `mapstructure:"read_timeout"`
    WriteTimeout    int    `mapstructure:"write_timeout"`
    IdleTimeout     int    `mapstructure:"idle_timeout"`
    ShutdownTimeout int    `mapstructure:"shutdown_timeout"`

    // NEW: Authentication configuration (supports preauth, future: JWT)
    Authentication AuthenticationConfig `mapstructure:"authentication"`
}
```

### 1.2: Set Default Values

Update the configuration loader to set default values:

```go
// File: internal/config/loader.go (or wherever defaults are set)

func setDefaults() {
    // Existing defaults...

    // Principal header defaults (nested structure for future extensibility)
    viper.SetDefault("servers.admin.authentication.preauth.principal_header_name", "X-Remote-User")
    viper.SetDefault("servers.api.authentication.preauth.principal_header_name", "X-Remote-User")
}
```

### 1.3: Update Configuration File

Add the new nested authentication configuration to your configuration file:

```yaml
# File: config.yaml

servers:
  api:
    port: 8080
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"  # Header set by reverse proxy
    # ... other settings ...

  admin:
    port: 8081
    authentication:
      preauth:
        principal_header_name: "X-Remote-User"  # Same header for admin
    # ... other settings ...
```

### 1.4: Environment Variable Support

The configuration system supports environment variables:

```bash
# Override via environment variables (nested keys use underscores)
export SERVERS_API_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME="X-Authenticated-User"
export SERVERS_ADMIN_AUTHENTICATION_PREAUTH_PRINCIPAL_HEADER_NAME="X-Admin-User"
```

## Step 2: Implement Context Utilities

### 2.1: Create Principal Package

Create the directory structure:

```bash
mkdir -p internal/domain/principal
```

### 2.2: Implement Context Functions

Create `internal/domain/principal/context.go`:

```go
// File: internal/domain/principal/context.go

package principal

import "context"

// principalContextKey is an unexported type for context keys.
// This prevents collisions with other packages using context values.
type principalContextKey struct{}

// WithPrincipal adds a principal to the context.
// The principal MUST be validated before calling this function.
//
// Example:
//
//	ctx := principal.WithPrincipal(r.Context(), "alice@example.com")
func WithPrincipal(ctx context.Context, principal string) context.Context {
    return context.WithValue(ctx, principalContextKey{}, principal)
}

// FromContext retrieves the principal from context.
// Returns (principal, true) if present, ("", false) otherwise.
//
// Example:
//
//	principal, ok := principal.FromContext(r.Context())
//	if !ok {
//	    // No principal in context
//	    http.Error(w, "unauthorized", http.StatusUnauthorized)
//	    return
//	}
func FromContext(ctx context.Context) (string, bool) {
    principal, ok := ctx.Value(principalContextKey{}).(string)
    return principal, ok
}

// MustFromContext retrieves the principal from context.
// Panics if principal is not present.
// Use only on protected routes where middleware guarantees principal presence.
//
// Example:
//
//	// On protected route with RequirePrincipalMiddleware
//	principal := principal.MustFromContext(r.Context())
//	// Safe to use—middleware guarantees presence
func MustFromContext(ctx context.Context) string {
    principal, ok := FromContext(ctx)
    if !ok {
        panic("principal not found in context")
    }
    return principal
}
```

### 2.3: Add Unit Tests

Create `internal/domain/principal/context_test.go`:

```go
// File: internal/domain/principal/context_test.go

package principal_test

import (
    "context"
    "testing"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
    "github.com/stretchr/testify/assert"
)

func TestWithPrincipal(t *testing.T) {
    ctx := context.Background()
    ctx = principal.WithPrincipal(ctx, "alice@example.com")

    value, ok := principal.FromContext(ctx)
    assert.True(t, ok)
    assert.Equal(t, "alice@example.com", value)
}

func TestFromContext_Missing(t *testing.T) {
    ctx := context.Background()

    value, ok := principal.FromContext(ctx)
    assert.False(t, ok)
    assert.Empty(t, value)
}

func TestFromContext_Present(t *testing.T) {
    ctx := context.Background()
    ctx = principal.WithPrincipal(ctx, "bob@example.com")

    value, ok := principal.FromContext(ctx)
    assert.True(t, ok)
    assert.Equal(t, "bob@example.com", value)
}

func TestMustFromContext_Success(t *testing.T) {
    ctx := context.Background()
    ctx = principal.WithPrincipal(ctx, "alice@example.com")

    value := principal.MustFromContext(ctx)
    assert.Equal(t, "alice@example.com", value)
}

func TestMustFromContext_Panics(t *testing.T) {
    ctx := context.Background()

    assert.Panics(t, func() {
        principal.MustFromContext(ctx)
    })
}

func TestMustFromContext_PanicMessage(t *testing.T) {
    ctx := context.Background()

    defer func() {
        if r := recover(); r != nil {
            assert.Equal(t, "principal not found in context", r)
        }
    }()

    principal.MustFromContext(ctx)
    t.Fatal("expected panic")
}
```

### 2.4: Run Tests

```bash
just test
# Or directly:
go test -v ./internal/domain/principal/...
```

## Step 3: Implement Middleware

### 3.1: Create Middleware File

Create `internal/adapters/http/principal_middleware.go`:

```go
// File: internal/adapters/http/principal_middleware.go

package http

import (
    "encoding/json"
    "log/slog"
    "net/http"
    "strings"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

const maxPrincipalLength = 200

// ErrorResponse is the JSON structure for error responses.
type ErrorResponse struct {
    Error string `json:"error"`
}

// RequirePrincipalMiddleware extracts and validates principals from HTTP headers.
// Requests without valid principals are rejected with 401 Unauthorized (missing/empty)
// or 400 Bad Request (too long).
//
// Example usage:
//
//	r.Group(func(r chi.Router) {
//	    r.Use(RequirePrincipalMiddleware("X-Remote-User"))
//	    r.Get("/api/users", handleUsers)  // Protected route
//	})
func RequirePrincipalMiddleware(headerName string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract header (case-insensitive per HTTP spec)
            principalValue := r.Header.Get(headerName)

            // Trim whitespace
            principalValue = strings.TrimSpace(principalValue)

            // Validate: non-empty
            if principalValue == "" {
                slog.Warn("principal missing or empty",
                    "header", headerName,
                    "path", r.URL.Path)
                writeErrorJSON(w, http.StatusUnauthorized, "missing or empty principal")
                return
            }

            // Validate: max length
            if len(principalValue) > maxPrincipalLength {
                slog.Warn("principal too long",
                    "header", headerName,
                    "length", len(principalValue),
                    "max", maxPrincipalLength,
                    "path", r.URL.Path)
                writeErrorJSON(w, http.StatusBadRequest,
                    "principal exceeds maximum length of 200 characters")
                return
            }

            // Add principal to context
            ctx := principal.WithPrincipal(r.Context(), principalValue)

            slog.Debug("principal extracted",
                "principal", principalValue,
                "header", headerName,
                "path", r.URL.Path)

            // Continue with modified context
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// OptionalPrincipalMiddleware extracts principals if present but allows
// requests without principals to continue. Malformed principals are logged
// as warnings but do not block the request.
//
// Example usage:
//
//	r.Group(func(r chi.Router) {
//	    r.Use(OptionalPrincipalMiddleware("X-Remote-User"))
//	    r.Get("/health", handleHealth)      // Works with or without principal
//	    r.Get("/metrics", handleMetrics)    // Works with or without principal
//	})
func OptionalPrincipalMiddleware(headerName string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // Extract header (case-insensitive)
            principalValue := r.Header.Get(headerName)

            // Trim whitespace
            principalValue = strings.TrimSpace(principalValue)

            // If missing or empty, continue without principal
            if principalValue == "" {
                slog.Debug("principal not provided (optional)",
                    "header", headerName,
                    "path", r.URL.Path)
                next.ServeHTTP(w, r)
                return
            }

            // Validate: max length (log warning but continue)
            if len(principalValue) > maxPrincipalLength {
                slog.Warn("principal too long, ignoring",
                    "header", headerName,
                    "length", len(principalValue),
                    "max", maxPrincipalLength,
                    "path", r.URL.Path)
                next.ServeHTTP(w, r)
                return
            }

            // Add principal to context
            ctx := principal.WithPrincipal(r.Context(), principalValue)

            slog.Debug("principal extracted (optional)",
                "principal", principalValue,
                "header", headerName,
                "path", r.URL.Path)

            // Continue with modified context
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

// writeErrorJSON writes a JSON error response with the specified status code.
func writeErrorJSON(w http.ResponseWriter, status int, message string) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    json.NewEncoder(w).Encode(ErrorResponse{Error: message})
}
```

### 3.2: Add Middleware Tests

Create `internal/adapters/http/principal_middleware_test.go`:

```go
// File: internal/adapters/http/principal_middleware_test.go

package http_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/go-chi/chi/v5"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

func TestRequirePrincipalMiddleware_Success(t *testing.T) {
    // Create test handler that checks context
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        principalValue, ok := principal.FromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "alice@example.com", principalValue)
        w.WriteHeader(http.StatusOK)
    })

    // Apply middleware
    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    // Create test request with principal header
    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "alice@example.com")
    rec := httptest.NewRecorder()

    // Execute
    wrappedHandler.ServeHTTP(rec, req)

    // Assert
    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePrincipalMiddleware_MissingPrincipal(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach handler")
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    // No principal header set
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusUnauthorized, rec.Code)
    assert.Contains(t, rec.Body.String(), "missing or empty principal")
    assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

func TestRequirePrincipalMiddleware_EmptyPrincipal(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach handler")
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "")
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusUnauthorized, rec.Code)
    assert.Contains(t, rec.Body.String(), "missing or empty principal")
}

func TestRequirePrincipalMiddleware_WhitespaceOnly(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach handler")
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "   ")
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

func TestRequirePrincipalMiddleware_TooLong(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach handler")
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    // Create 201-character principal
    longPrincipal := strings.Repeat("a", 201)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", longPrincipal)
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusBadRequest, rec.Code)
    assert.Contains(t, rec.Body.String(), "exceeds maximum length")
}

func TestRequirePrincipalMiddleware_TrimsWhitespace(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        principalValue, ok := principal.FromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "alice@example.com", principalValue)
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "  alice@example.com  ")
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePrincipalMiddleware_CaseInsensitiveHeader(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        principalValue, ok := principal.FromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "alice@example.com", principalValue)
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("x-remote-user", "alice@example.com")  // lowercase
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOptionalPrincipalMiddleware_WithPrincipal(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        principalValue, ok := principal.FromContext(r.Context())
        assert.True(t, ok)
        assert.Equal(t, "alice@example.com", principalValue)
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.OptionalPrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "alice@example.com")
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOptionalPrincipalMiddleware_WithoutPrincipal(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, ok := principal.FromContext(r.Context())
        assert.False(t, ok, "expected no principal in context")
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.OptionalPrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    // No principal header set
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestOptionalPrincipalMiddleware_TooLong(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        _, ok := principal.FromContext(r.Context())
        assert.False(t, ok, "expected no principal in context for too-long value")
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.OptionalPrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    // Create 201-character principal
    longPrincipal := strings.Repeat("a", 201)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", longPrincipal)
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    // Should continue processing (no error)
    assert.Equal(t, http.StatusOK, rec.Code)
}

func TestRequirePrincipalMiddleware_Integration(t *testing.T) {
    r := chi.NewRouter()

    // Protected route
    r.Group(func(r chi.Router) {
        r.Use(httpAdapter.RequirePrincipalMiddleware("X-Remote-User"))
        r.Get("/protected", func(w http.ResponseWriter, r *http.Request) {
            principalValue := principal.MustFromContext(r.Context())
            w.Write([]byte(principalValue))
        })
    })

    // Test with valid principal
    req := httptest.NewRequest("GET", "/protected", nil)
    req.Header.Set("X-Remote-User", "alice@example.com")
    rec := httptest.NewRecorder()

    r.ServeHTTP(rec, req)

    assert.Equal(t, http.StatusOK, rec.Code)
    assert.Equal(t, "alice@example.com", rec.Body.String())
}

func TestErrorResponseFormat(t *testing.T) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        t.Fatal("should not reach handler")
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    // No principal header
    rec := httptest.NewRecorder()

    wrappedHandler.ServeHTTP(rec, req)

    // Verify JSON format
    var errorResp httpAdapter.ErrorResponse
    err := json.Unmarshal(rec.Body.Bytes(), &errorResp)
    require.NoError(t, err)
    assert.Equal(t, "missing or empty principal", errorResp.Error)
    assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}
```

### 3.3: Run Middleware Tests

```bash
just test
# Or directly:
go test -v ./internal/adapters/http/...
```

## Step 4: Apply Middleware to Routes

### 4.1: Update HTTP Server

Update `internal/adapters/http/server.go` to apply middleware to route groups:

```go
// File: internal/adapters/http/server.go

package http

import (
    "context"
    "fmt"
    "log/slog"
    "net/http"
    "time"

    "github.com/go-chi/chi/v5"
    "github.com/go-chi/chi/v5/middleware"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
)

type Server struct {
    config *config.ServerInstanceConfig
    router chi.Router
    server *http.Server
}

func NewServer(cfg *config.ServerInstanceConfig) *Server {
    return &Server{
        config: cfg,
    }
}

func (s *Server) Start() error {
    s.setupRoutes()

    s.server = &http.Server{
        Addr:         fmt.Sprintf(":%d", s.config.Port),
        Handler:      s.router,
        ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
        WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
        IdleTimeout:  time.Duration(s.config.IdleTimeout) * time.Second,
    }

    slog.Info("starting server",
        "name", s.config.Name,
        "port", s.config.Port,
        "principal_header", s.config.Authentication.Preauth.PrincipalHeaderName)

    return s.server.ListenAndServe()
}

func (s *Server) setupRoutes() {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)
    r.Use(middleware.RequestID)

    // Public routes - no principal required
    r.Get("/health", s.handleHealth)
    r.Get("/readiness", s.handleReadiness)

    // API routes - principal REQUIRED
    r.Group(func(r chi.Router) {
        r.Use(RequirePrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))
        r.Get("/api/users", s.handleGetUsers)
        r.Post("/api/users", s.handleCreateUser)
        r.Get("/api/users/{id}", s.handleGetUser)
        r.Delete("/api/users/{id}", s.handleDeleteUser)
    })

    // Metrics routes - principal OPTIONAL (for audit logging)
    r.Group(func(r chi.Router) {
        r.Use(OptionalPrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))
        r.Get("/metrics", s.handleMetrics)
    })

    s.router = r
}

func (s *Server) Shutdown(ctx context.Context) error {
    slog.Info("shutting down server", "name", s.config.Name)
    return s.server.Shutdown(ctx)
}
```

### 4.2: Update Handler to Use Principal

Example handler using the extracted principal:

```go
// File: internal/adapters/http/handlers.go (or in server.go)

package http

import (
    "encoding/json"
    "log/slog"
    "net/http"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
)

func (s *Server) handleGetUsers(w http.ResponseWriter, r *http.Request) {
    // Retrieve principal from context
    // Use MustFromContext on protected routes (safer, less boilerplate)
    principalValue := principal.MustFromContext(r.Context())

    slog.Info("fetching users", "principal", principalValue)

    // Use principal for business logic
    // Example: Filter users by principal, log audit trail, etc.
    users := []map[string]string{
        {"id": "1", "name": "Alice", "principal": principalValue},
        {"id": "2", "name": "Bob", "principal": principalValue},
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(users)
}

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
    // Optional route: use FromContext to check if principal is present
    principalValue, ok := principal.FromContext(r.Context())
    if ok {
        slog.Info("metrics request", "principal", principalValue)
    } else {
        slog.Info("metrics request", "principal", "anonymous")
    }

    // Collect metrics
    metrics := map[string]interface{}{
        "requests_total": 1234,
        "errors_total":   5,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(metrics)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("OK"))
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("READY"))
}
```

## Step 5: Configure Deployment

### 5.1: Reverse Proxy Configuration

The reverse proxy MUST set the principal header after authentication. Examples:

**nginx**:
```nginx
location / {
    proxy_pass http://backend:8080;

    # Set principal header after authentication
    proxy_set_header X-Remote-User $remote_user;

    # Remove principal header from client requests (security)
    proxy_set_header X-Remote-User "";

    # Other headers
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
}
```

**Apache**:
```apache
<Location />
    ProxyPass http://backend:8080/

    # Set principal header after authentication
    RequestHeader set X-Remote-User %{REMOTE_USER}e

    # Remove principal header from client requests
    RequestHeader unset X-Remote-User
</Location>
```

**Traefik**:
```yaml
# traefik.yml
http:
  middlewares:
    auth-headers:
      headers:
        customRequestHeaders:
          X-Remote-User: "{{ .User }}"

  routers:
    api:
      rule: "Host(`api.example.com`)"
      middlewares:
        - auth-headers
      service: backend
```

**Caddy**:
```caddy
api.example.com {
    reverse_proxy backend:8080 {
        header_up X-Remote-User {http.auth.user.id}
    }
}
```

### 5.2: Environment-Specific Configuration

**Development** (no authentication):
```yaml
# configs/config.dev.yaml
servers:
  api:
    port: 8080
    principal_header_name: "X-Remote-User"
    # For dev: Use test header or bypass middleware on public routes
```

**Production** (with authentication):
```yaml
# configs/config.prod.yaml
servers:
  api:
    port: 8080
    principal_header_name: "X-Remote-User"
    # Reverse proxy sets this header after authentication
```

## Step 6: Testing

### 6.1: Unit Tests

Run unit tests for context utilities and middleware:

```bash
just test
# Or:
go test -v ./internal/domain/principal/...
go test -v ./internal/adapters/http/...
```

### 6.2: Integration Tests

Create end-to-end integration test:

```go
// File: tests/integration/principal_middleware_test.go

package integration_test

import (
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"

    "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
    "github.com/agentic-identity-broker/agentic-identity-broker/internal/config"
)

func TestPrincipalExtraction_EndToEnd(t *testing.T) {
    // Setup server
    cfg := &config.ServerInstanceConfig{
        Port: 8080,
        Name: "test-server",
        Authentication: config.AuthenticationConfig{
            Preauth: config.PreauthConfig{
                PrincipalHeaderName: "X-Remote-User",
            },
        },
        ReadTimeout:  30,
        WriteTimeout: 30,
        IdleTimeout:  60,
    }

    server := http.NewServer(cfg)
    // Note: setupRoutes is called internally, expose router for testing or use test server

    // Test protected route with valid principal
    t.Run("protected route with valid principal", func(t *testing.T) {
        req := httptest.NewRequest("GET", "/api/users", nil)
        req.Header.Set("X-Remote-User", "alice@example.com")
        rec := httptest.NewRecorder()

        // Assume server.router is exposed for testing
        // server.router.ServeHTTP(rec, req)

        // assert.Equal(t, http.StatusOK, rec.Code)
    })

    // Test protected route without principal
    t.Run("protected route without principal", func(t *testing.T) {
        req := httptest.NewRequest("GET", "/api/users", nil)
        // No X-Remote-User header
        rec := httptest.NewRecorder()

        // server.router.ServeHTTP(rec, req)

        // assert.Equal(t, http.StatusUnauthorized, rec.Code)
    })

    // Test public route without principal
    t.Run("public route without principal", func(t *testing.T) {
        req := httptest.NewRequest("GET", "/health", nil)
        rec := httptest.NewRecorder()

        // server.router.ServeHTTP(rec, req)

        // assert.Equal(t, http.StatusOK, rec.Code)
    })
}
```

### 6.3: Manual Testing with curl

Start the server and test with curl:

```bash
# Start server
just run

# Test protected route WITHOUT principal (should fail with 401)
curl -v http://localhost:8080/api/users

# Test protected route WITH principal (should succeed)
curl -v -H "X-Remote-User: alice@example.com" http://localhost:8080/api/users

# Test public route WITHOUT principal (should succeed)
curl -v http://localhost:8080/health

# Test with too-long principal (should fail with 400)
curl -v -H "X-Remote-User: $(python3 -c 'print("a"*201)')" http://localhost:8080/api/users
```

## Common Patterns

### Pattern 1: Mixed Protected and Public Routes

Some routes require principals, others don't:

```go
func (s *Server) setupRoutes() {
    r := chi.NewRouter()

    // Global middleware
    r.Use(middleware.Logger)
    r.Use(middleware.Recoverer)

    // Public routes - NO principal
    r.Get("/health", s.handleHealth)
    r.Get("/docs", s.handleDocs)

    // Protected API routes - principal REQUIRED
    r.Group(func(r chi.Router) {
        r.Use(RequirePrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))
        r.Get("/api/users", s.handleGetUsers)
        r.Post("/api/users", s.handleCreateUser)
    })

    // Admin routes - principal REQUIRED (different header)
    r.Group(func(r chi.Router) {
        r.Use(RequirePrincipalMiddleware("X-Admin-User"))
        r.Get("/admin/stats", s.handleAdminStats)
    })

    s.router = r
}
```

### Pattern 2: Optional Principal for Logging

Extract principal for audit logging but don't require it:

```go
func (s *Server) setupRoutes() {
    r := chi.NewRouter()

    // Optional principal for audit logging
    r.Group(func(r chi.Router) {
        r.Use(OptionalPrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))
        r.Use(AuditLoggingMiddleware)  // Logs principal if present

        r.Get("/public/data", s.handlePublicData)
        r.Get("/metrics", s.handleMetrics)
    })

    s.router = r
}

func AuditLoggingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        principalValue, ok := principal.FromContext(r.Context())
        if ok {
            slog.Info("request",
                "principal", principalValue,
                "method", r.Method,
                "path", r.URL.Path)
        } else {
            slog.Info("request",
                "principal", "anonymous",
                "method", r.Method,
                "path", r.URL.Path)
        }

        next.ServeHTTP(w, r)
    })
}
```

### Pattern 3: Per-Endpoint Control

Apply middleware to specific endpoints:

```go
func (s *Server) setupRoutes() {
    r := chi.NewRouter()

    // Most routes are public
    r.Get("/docs", s.handleDocs)
    r.Get("/health", s.handleHealth)

    // Single protected endpoint
    r.With(RequirePrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName)).
        Get("/api/private", s.handlePrivate)

    s.router = r
}
```

### Pattern 4: Principal-Based Authorization

Use principal for role-based access control:

```go
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
    principalValue := principal.MustFromContext(r.Context())

    // Check if principal has admin role
    if !s.authService.IsAdmin(principalValue) {
        http.Error(w, "forbidden", http.StatusForbidden)
        return
    }

    // Proceed with admin logic
    stats := s.statsService.GetAdminStats()
    json.NewEncoder(w).Encode(stats)
}
```

## Error Handling

### Error Response Format

All middleware errors use consistent JSON format:

**401 Unauthorized** (missing/empty principal):
```json
{
  "error": "missing or empty principal"
}
```

**400 Bad Request** (principal too long):
```json
{
  "error": "principal exceeds maximum length of 200 characters"
}
```

### Client Error Handling

Example JavaScript client handling errors:

```javascript
async function fetchUsers() {
    const response = await fetch('/api/users', {
        headers: {
            'X-Remote-User': 'alice@example.com'
        }
    });

    if (!response.ok) {
        const error = await response.json();
        console.error('API error:', error.error);

        if (response.status === 401) {
            // Redirect to login
            window.location.href = '/login';
        }

        return;
    }

    const users = await response.json();
    console.log('Users:', users);
}
```

## Performance Considerations

### Benchmarking

Add benchmarks to measure middleware overhead:

```go
// File: internal/adapters/http/principal_middleware_bench_test.go

package http_test

import (
    "net/http"
    "net/http/httptest"
    "testing"

    httpAdapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
)

func BenchmarkRequirePrincipalMiddleware_Success(b *testing.B) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    req.Header.Set("X-Remote-User", "alice@example.com")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        rec := httptest.NewRecorder()
        wrappedHandler.ServeHTTP(rec, req)
    }
}

func BenchmarkRequirePrincipalMiddleware_Failure(b *testing.B) {
    handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusOK)
    })

    middleware := httpAdapter.RequirePrincipalMiddleware("X-Remote-User")
    wrappedHandler := middleware(handler)

    req := httptest.NewRequest("GET", "/test", nil)
    // No principal header

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        rec := httptest.NewRecorder()
        wrappedHandler.ServeHTTP(rec, req)
    }
}
```

Run benchmarks:

```bash
go test -bench=. -benchmem ./internal/adapters/http/
```

### Expected Performance

- **Context lookup**: ~50-100ns (negligible overhead)
- **Middleware execution**: <1μs for success path
- **Total added latency**: <1ms at p95 (well within budget)

## Security Considerations

### 1. Trusted Reverse Proxy

This pattern assumes the reverse proxy is trusted and performs authentication. The application does NOT verify the principal—it trusts the header value.

**Deployment Requirement**:
- Reverse proxy MUST authenticate users before setting principal header
- Reverse proxy MUST strip principal headers from client requests
- TLS MUST be used between reverse proxy and application

### 2. Header Injection Prevention

The 200-character limit prevents header injection attacks:

```go
const maxPrincipalLength = 200  // Prevents oversized headers
```

### 3. Fail Closed

Invalid requests are rejected by default on protected routes:

```go
// RequirePrincipalMiddleware rejects invalid requests
if principalValue == "" {
    // Return 401—do NOT continue
    writeErrorJSON(w, http.StatusUnauthorized, "missing or empty principal")
    return
}
```

### 4. No Bypass

Middleware must be explicitly applied to routes. No automatic principal extraction:

```go
// Protected routes MUST explicitly use middleware
r.Group(func(r chi.Router) {
    r.Use(RequirePrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))
    r.Get("/api/users", s.handleGetUsers)
})
```

## Troubleshooting

### Problem: Handler receives 401 even with principal header set

**Symptom**: Client sends `X-Remote-User` header but receives 401.

**Causes**:
1. Reverse proxy not setting header correctly
2. Header name mismatch (config vs. actual header)
3. Header value is empty or whitespace

**Solution**:
```bash
# Check logs for middleware warnings
grep "principal missing" /var/log/app.log

# Verify header in reverse proxy logs
# nginx: log_format combined '$http_x_remote_user ...';

# Test directly with curl
curl -v -H "X-Remote-User: alice@example.com" http://localhost:8080/api/users
```

### Problem: Context doesn't contain principal in handler

**Symptom**: `FromContext` returns `(false)` even on protected route.

**Causes**:
1. Middleware not applied to route group
2. Wrong order of middleware (applied after route handler)
3. Context not propagated correctly

**Solution**:
```go
// Verify middleware is applied BEFORE route handlers
r.Group(func(r chi.Router) {
    r.Use(RequirePrincipalMiddleware(s.config.Authentication.Preauth.PrincipalHeaderName))  // BEFORE routes
    r.Get("/api/users", s.handleGetUsers)
})

// Verify context is passed to handler
next.ServeHTTP(w, r.WithContext(ctx))  // Use modified context
```

### Problem: Performance degradation

**Symptom**: Requests take longer after adding middleware.

**Causes**:
1. Context lookups are slow (shouldn't be—~50ns)
2. Logging overhead (debug logs on every request)
3. JSON encoding overhead (error responses)

**Solution**:
```bash
# Run benchmarks to measure overhead
go test -bench=. -benchmem ./internal/adapters/http/

# Expected results:
# BenchmarkRequirePrincipalMiddleware_Success   <1000 ns/op

# If slower, profile with pprof
go test -cpuprofile=cpu.prof -bench=. ./internal/adapters/http/
go tool pprof cpu.prof
```

## Next Steps

After implementing principal extraction, consider:

1. **Role-Based Access Control**: Extract roles from additional headers
2. **Audit Logging**: Persist principal-based audit logs to database
3. **Multi-Tenant Support**: Extract tenant ID from headers
4. **Rate Limiting**: Apply per-principal rate limits
5. **Metrics**: Track principal extraction success/failure rates

## References

- **Feature Specification**: [spec.md](spec.md)
- **Implementation Plan**: [plan.md](plan.md)
- **Research Document**: [research.md](research.md)
- **Data Model**: [data-model.md](data-model.md)
- **API Contracts**: [contracts/middleware.md](contracts/middleware.md)
- **Chi Router Documentation**: https://github.com/go-chi/chi
- **Go Context Documentation**: https://pkg.go.dev/context
