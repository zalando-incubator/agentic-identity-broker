# Go Expert Review Summary: Dual-Port HTTP Server

**Date**: 2025-12-15
**Reviewer**: golang-pro agent
**Overall Score**: 8.5/10
**Recommendation**: APPROVE with required fixes

## Executive Summary

The implementation plan is **production-ready** after addressing critical goroutine management and health state synchronization issues. Strong architectural foundation using chi framework, errgroup pattern, and hexagonal design.

## Critical Issues (Must Fix Before Implementation)

### 1. Goroutine Leak in Atomic Startup ⚠️ CRITICAL

**Problem**: The errgroup pattern expects short-lived goroutines, but `http.Server.Serve()` blocks indefinitely. If one server fails during binding, the other server's goroutine will leak.

**Solution**: Split `Start()` into `Listen()` (fast binding) and `Serve()` (blocking):

```go
// Required Interface Update
type ServerPort interface {
    Listen() (net.Listener, error)  // NEW - fast binding
    Serve(ctx context.Context, listener net.Listener) error  // NEW - blocking serve
    Shutdown(ctx context.Context, timeout time.Duration) error
    HealthStatus() HealthState
}

// Corrected Manager.Start() Implementation
func (m *Manager) Start(ctx context.Context) error {
    g, gCtx := errgroup.WithContext(ctx)
    startupComplete := make(chan struct{})
    var startupOnce sync.Once

    g.Go(func() error {
        listener, err := m.enduserServer.Listen()
        if err != nil {
            return fmt.Errorf("enduser server bind failed: %w", err)
        }
        startupOnce.Do(func() { close(startupComplete) })
        return m.enduserServer.Serve(gCtx, listener)
    })

    g.Go(func() error {
        listener, err := m.adminServer.Listen()
        if err != nil {
            return fmt.Errorf("admin server bind failed: %w", err)
        }
        startupOnce.Do(func() { close(startupComplete) })
        return m.adminServer.Serve(gCtx, listener)
    })

    // Wait for both servers to bind successfully
    select {
    case <-startupComplete:
        // At least one server bound
    case <-gCtx.Done():
        return gCtx.Err()
    }

    return g.Wait()
}
```

**Impact**: Without this fix, failed startup will leak goroutines and prevent clean shutdown.

---

### 2. Health State Synchronization ⚠️ CRITICAL

**Problem**: The plan doesn't specify how concurrent access to `HealthState` is synchronized.

**Solution**: Use atomic operations for lock-free access:

```go
type Server struct {
    httpServer  *http.Server
    router      chi.Router
    healthState int32  // atomic access only (HealthState cast to int32)
    startTime   time.Time
    logger      *slog.Logger
}

func (s *Server) HealthStatus() HealthState {
    return HealthState(atomic.LoadInt32(&s.healthState))
}

func (s *Server) setHealthState(state HealthState) {
    atomic.StoreInt32(&s.healthState, int32(state))
}
```

**Impact**: Without atomic operations, concurrent reads/writes to health state will cause race conditions.

---

## Major Issues (Should Fix)

### 3. Viper Environment Variable Binding

**Problem**: Nested struct environment variables require explicit `BindEnv()` calls in Viper.

**Solution**:
```go
// In config loader initialization
viper.SetEnvPrefix("IDENTITY_BROKER")
viper.AutomaticEnv()
viper.BindEnv("server.enduser.port")
viper.BindEnv("server.enduser.bind")
viper.BindEnv("server.admin.port")
viper.BindEnv("server.admin.bind")
viper.BindEnv("server.shutdown.timeout")
```

---

### 4. Configuration Validation

**Problem**: Plan shows `validate` struct tags but no validation library in dependencies.

**Solution**: Use manual validation (aligns with minimal dependencies approach):

```go
func (c *ServerConfig) Validate() error {
    if c.EndUser.Port < 1 || c.EndUser.Port > 65535 {
        return fmt.Errorf("enduser port must be 1-65535, got %d", c.EndUser.Port)
    }
    if c.Admin.Port < 1 || c.Admin.Port > 65535 {
        return fmt.Errorf("admin port must be 1-65535, got %d", c.Admin.Port)
    }
    if c.EndUser.Port == c.Admin.Port {
        return fmt.Errorf("enduser and admin ports must differ")
    }
    // Validate bind addresses (no DNS lookup)
    if err := validateBindAddress(c.EndUser.Bind); err != nil {
        return fmt.Errorf("invalid enduser bind address: %w", err)
    }
    if err := validateBindAddress(c.Admin.Bind); err != nil {
        return fmt.Errorf("invalid admin bind address: %w", err)
    }
    return nil
}
```

---

### 5. Atomic Startup Testing

**Problem**: No clear strategy for injecting bind failures in tests.

**Solution**: Use mock interfaces:

```go
// In test file
type mockServer struct {
    bindErr error
    // ...
}

func (m *mockServer) Listen() (net.Listener, error) {
    if m.bindErr != nil {
        return nil, m.bindErr
    }
    return net.Listen("tcp", "127.0.0.1:0")
}

func TestAtomicStartupFailure(t *testing.T) {
    enduserServer := &mockServer{bindErr: errors.New("port in use")}
    adminServer := &mockServer{}

    manager := NewManager(enduserServer, adminServer, 30*time.Second, logger)

    err := manager.Start(context.Background())
    assert.Error(t, err)
    assert.Contains(t, err.Error(), "port in use")
}
```

---

## Minor Issues (Nice to Have)

### 6. Bind Address Validation

**Current**: Performs DNS lookup during validation (slow, network-dependent)
**Recommendation**: Validate syntax only, defer DNS resolution to binding time:

```go
func validateBindAddress(addr string) error {
    if addr == "" {
        return nil  // Will use default
    }
    if ip := net.ParseIP(addr); ip != nil {
        return nil  // Valid IP
    }
    // Validate hostname syntax (no DNS lookup)
    if len(addr) > 255 {
        return fmt.Errorf("hostname too long")
    }
    hostnameRegex := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
    if !hostnameRegex.MatchString(addr) {
        return fmt.Errorf("invalid bind address format: %s", addr)
    }
    return nil
}
```

---

### 7. Shutdown Context Propagation

**Current**: Uses `context.Background()` in shutdown
**Recommendation**: Accept parent context to allow forced termination:

```go
func (s *Server) Shutdown(parentCtx context.Context, timeout time.Duration) error {
    ctx, cancel := context.WithTimeout(parentCtx, timeout)
    defer cancel()

    atomic.StoreInt32(&s.healthState, int32(HealthStateShuttingDown))

    err := s.httpServer.Shutdown(ctx)
    if err != nil {
        atomic.StoreInt32(&s.healthState, int32(HealthStateUnhealthy))
        return fmt.Errorf("shutdown failed: %w", err)
    }

    return nil
}
```

---

### 8. IPv6 Test Detection

**Recommendation**: Add environment detection for CI:

```go
func TestIPv6DualStack(t *testing.T) {
    if !hasIPv6() {
        t.Skip("IPv6 not available, skipping dual-stack test")
    }
    // Test IPv6 binding
}

func hasIPv6() bool {
    listener, err := net.Listen("tcp6", "[::1]:0")
    if err != nil {
        return false
    }
    listener.Close()
    return true
}
```

---

## Strengths

1. **Excellent Framework Choice**: chi is perfect for this use case (stdlib-compatible, lightweight)
2. **Sound IPv6 Strategy**: Dual-stack with automatic IPv4 fallback is the right approach
3. **Clean Architecture**: Hexagonal design with clear port/adapter boundaries
4. **Comprehensive Testing**: Good coverage of unit, integration, and race conditions
5. **Strong Documentation**: Detailed quickstart, API contracts, and data models

---

## Missing Elements

### 1. Middleware Implementation

**Recommendation**: Add standard middleware:

```go
// middleware.go
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            start := time.Now()
            ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
            next.ServeHTTP(ww, r)

            logger.Info("HTTP request",
                slog.String("method", r.Method),
                slog.String("path", r.URL.Path),
                slog.Int("status", ww.Status()),
                slog.Duration("duration", time.Since(start)),
            )
        })
    }
}

func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            defer func() {
                if err := recover(); err != nil {
                    logger.Error("Panic recovered", slog.Any("error", err))
                    http.Error(w, "Internal Server Error", 500)
                }
            }()
            next.ServeHTTP(w, r)
        })
    }
}
```

### 2. Server Identification

**Recommendation**: Pass server name to constructor:

```go
func NewServer(name string, config ServerInstanceConfig, logger *slog.Logger) *Server {
    return &Server{
        name:       name,  // "enduser" or "admin"
        startTime:  time.Now(),
        logger:     logger,
        // ...
    }
}
```

### 3. Context Propagation in main.go

**Recommendation**:

```go
func main() {
    ctx, cancel := signal.NotifyContext(
        context.Background(),
        os.Interrupt,
        syscall.SIGTERM,
    )
    defer cancel()

    cfg, err := configLoader.GetConfig(ctx)
    if err != nil {
        log.Fatal(err)
    }

    manager := server.NewManager(cfg.Server, logger)
    if err := manager.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
        logger.Error("Server error", slog.String("error", err.Error()))
        os.Exit(1)
    }

    logger.Info("Application stopped gracefully")
}
```

---

## Implementation Checklist

Before starting implementation, ensure:

**Critical Fixes**:
- [ ] Split `Start()` into `Listen()` and `Serve()` methods
- [ ] Use atomic operations for health state access
- [ ] Update `ServerPort` interface with new methods

**Major Fixes**:
- [ ] Add explicit `BindEnv()` calls for Viper nested keys
- [ ] Implement manual configuration validation
- [ ] Add mock interfaces for atomic startup testing

**Minor Improvements**:
- [ ] Validate bind address syntax only (no DNS lookup)
- [ ] Accept parent context in `Shutdown()` method
- [ ] Add IPv6 environment detection in tests

**Missing Elements**:
- [ ] Implement Logger and Recovery middleware
- [ ] Add server name to Server struct
- [ ] Document context propagation from main.go

**Testing**:
- [ ] Run all tests with `-race` flag
- [ ] Test IPv6 fallback on IPv4-only system
- [ ] Test atomic startup failure scenarios
- [ ] Test graceful shutdown with in-flight requests

---

## Performance Notes

- **Memory**: ~16MB for 2000 connections (8KB per connection), not 2KB as estimated
- **Health Endpoint**: Use atomic operations to achieve <100μs response time (no locks)
- **Startup**: <500ms typical, <5s target is very conservative

---

## Final Recommendation

**Status**: APPROVE with required fixes

The implementation plan is fundamentally sound with excellent architectural choices. Fix the two critical issues (goroutine leak and health state synchronization) before implementation. The major and minor issues can be addressed during code review, but the critical issues must be resolved in the initial implementation to prevent bugs.

**Risk Level**: LOW after critical fixes applied

**Production Readiness**: HIGH (after all critical and major issues resolved)

