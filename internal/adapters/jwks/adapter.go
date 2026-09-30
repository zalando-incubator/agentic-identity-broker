// Package jwks provides the JWKS adapter for token exchange.
// This adapter abstracts HTTP fetching and caching of JSON Web Key Sets from upstream OAuth2 servers.
package jwks

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/httprc/v3/errsink"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const staleServeRelogEvery = 10
const maxJWKSResponseBytes = 1 << 20

// Adapter implements the JWKSPort interface for fetching and caching JWKS from a known upstream JWKS URI.
// Each adapter instance maintains its own cached key set, refresh schedule, and refresh health.
//
// The adapter uses httprc resources directly so it can observe both synchronous fetches and
// background refreshes. Successful refreshes update the cached set and clear any degraded
// health state; failed refreshes are tracked so stale upstream material is not served forever.
type Adapter struct {
	ctrl         httprc.Controller
	resource     *httprc.ResourceBase[jwk.Set]
	jwksURI      string
	fetchTimeout time.Duration
	logger       *slog.Logger

	refreshMu sync.Mutex
	refresh   *refreshState
	mu        sync.Mutex

	healthMu           sync.RWMutex
	lastSuccessAt      time.Time
	nextRefreshAt      time.Time
	lastErr            error
	staleServeLogCount int
}

type refreshState struct {
	done chan struct{}
	err  error
}

type controllerShutdowner interface {
	ShutdownContext(context.Context) error
}

type trackingTransformer struct {
	onSuccess func()
}

func (t trackingTransformer) Transform(_ context.Context, res *http.Response) (jwk.Set, error) {
	buf, err := io.ReadAll(io.LimitReader(res.Body, maxJWKSResponseBytes+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if len(buf) > maxJWKSResponseBytes {
		return nil, fmt.Errorf("JWKS response exceeds %d byte limit", maxJWKSResponseBytes)
	}

	set, err := jwk.Parse(buf, jwk.WithStrictKeySetParsing(true))
	if err != nil {
		return nil, fmt.Errorf("failed to parse JWK set at %q: %w", res.Request.URL.String(), err)
	}

	if t.onSuccess != nil {
		t.onSuccess()
	}

	return set, nil
}

var _ ports.JWKSPort = (*Adapter)(nil)

// NewJWKSAdapter creates a JWKS adapter for the given upstream URI.
// Fetching is lazy: the adapter does not contact the upstream until the first read.
// minRefreshInterval and maxRefreshInterval bound the refresh cadence after
// upstream cache headers are applied.
func NewJWKSAdapter(
	jwksURI string,
	httpClient *http.Client,
	minRefreshInterval time.Duration,
	maxRefreshInterval time.Duration,
	logger *slog.Logger,
) (*Adapter, error) {
	if jwksURI == "" {
		return nil, fmt.Errorf("jwks_uri cannot be empty")
	}
	if httpClient == nil {
		return nil, fmt.Errorf("http_client cannot be nil")
	}
	if minRefreshInterval < 0 {
		return nil, fmt.Errorf("min_refresh_interval cannot be negative")
	}
	if maxRefreshInterval < 0 {
		return nil, fmt.Errorf("max_refresh_interval cannot be negative")
	}
	if minRefreshInterval > maxRefreshInterval {
		return nil, fmt.Errorf("min_refresh_interval (%v) must be <= max_refresh_interval (%v)", minRefreshInterval, maxRefreshInterval)
	}
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	adapter := &Adapter{
		jwksURI:      jwksURI,
		fetchTimeout: httpClient.Timeout,
		logger:       logger,
	}

	client := httprc.NewClient(
		httprc.WithErrorSink(errsink.NewFunc(func(_ context.Context, err error) {
			// httprc only routes worker-scheduled background sync failures through the
			// errsink. Caller-driven ctrl.Refresh failures are recorded in startRefresh.
			adapter.recordFailure(err)
			adapter.logger.Warn("JWKS refresh failed",
				"jwks_uri", adapter.jwksURI,
				"error", err,
			)
		})),
	)

	ctrl, err := client.Start(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to start httprc client: %w", err)
	}

	resource, err := httprc.NewResource[jwk.Set](
		jwksURI,
		trackingTransformer{onSuccess: adapter.recordSuccess},
		httprc.WithMinInterval(minRefreshInterval),
		httprc.WithMaxInterval(maxRefreshInterval),
		httprc.WithHTTPClient(httpClient),
	)
	if err != nil {
		shutdownControllerWithWarning(ctrl, logger, "resource creation failure")
		return nil, fmt.Errorf("failed to create jwks resource: %w", err)
	}

	adapter.ctrl = ctrl
	adapter.resource = resource

	if err := ctrl.Add(context.Background(), resource, httprc.WithWaitReady(false)); err != nil {
		shutdownControllerWithWarning(ctrl, logger, "resource registration failure")
		return nil, fmt.Errorf("failed to register jwks resource: %w", err)
	}

	return adapter, nil
}

// GetKeySet returns the cached JWKS key set from the upstream OAuth2 server.
//
// On first call, this will fetch the JWKS from the upstream server (may block briefly).
// Subsequent calls return cached results. The adapter handles automatic background refresh
// based on configured intervals and fails closed once the last successful refresh is stale.
//
// Returns the entire jwk.Set which can be searched for specific keys (by kid or other criteria).
//
// Error handling:
// - If first fetch fails: HTTP error (connection, timeout), JWKS format invalid, etc.
// - If the last successful refresh has gone stale and refresh still fails: returns an error
// - If context is cancelled: returns context cancellation error
// - If upstream returns non-200 status: returns HTTP error
//
// Thread-safe: Safe for concurrent calls.
func (a *Adapter) GetKeySet(ctx context.Context) (_ jwk.Set, err error) {
	if _, hasDeadline := ctx.Deadline(); !hasDeadline && a.fetchTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, a.fetchTimeout)
		defer cancel()
	}

	ctx, span := otel.Tracer("jwks").Start(ctx, "jwks.fetch")
	defer func() {
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		span.End()
	}()
	span.SetAttributes(attribute.String("url.full", a.jwksURI))

	if !a.resourceReady() || a.staleError() != nil {
		if refreshErr := awaitRefresh(ctx, a.startRefresh(ctx)); refreshErr != nil {
			return nil, fmt.Errorf("failed to fetch jwks from %s: %w", a.jwksURI, refreshErr)
		}
	}

	// Re-check before serving cached material because a background refresh can fail
	// after the preflight gate above. Within the grace window we may still serve
	// cached keys and log the refresh failure; once nextRefreshAt passes we fail closed.
	if staleErr := a.staleError(); staleErr != nil {
		return nil, fmt.Errorf("failed to fetch jwks from %s: %w", a.jwksURI, staleErr)
	}
	if staleWarn := a.markStaleServeLogged(); staleWarn != nil {
		a.logger.Warn("serving cached JWKS after refresh failure",
			"jwks_uri", a.jwksURI,
			"error", staleWarn,
		)
	}

	keyset := a.resource.Resource()
	if keyset == nil {
		return nil, fmt.Errorf("failed to fetch jwks from %s: resource not ready", a.jwksURI)
	}

	return keyset, nil
}

// HealthState reports whether the adapter still has servable upstream key material.
func (a *Adapter) HealthState() ports.ComponentHealth {
	a.healthMu.RLock()
	defer a.healthMu.RUnlock()

	if a.lastSuccessAt.IsZero() {
		return ports.ComponentHealthDegraded
	}
	if a.staleErrorLocked() != nil {
		return ports.ComponentHealthDegraded
	}

	return ports.ComponentHealthHealthy
}

func shutdownControllerWithWarning(ctrl controllerShutdowner, logger *slog.Logger, reason string) {
	if err := ctrl.ShutdownContext(context.Background()); err != nil {
		logger.Warn("failed to shut down JWKS controller during constructor cleanup",
			"reason", reason,
			"error", err,
		)
	}
}

func (a *Adapter) resourceReady() bool {
	return a.resource != nil && a.resource.Resource() != nil
}

func (a *Adapter) staleError() error {
	a.healthMu.RLock()
	defer a.healthMu.RUnlock()
	return a.staleErrorLocked()
}

func (a *Adapter) staleErrorLocked() error {
	if a.lastSuccessAt.IsZero() {
		return a.lastErr
	}
	if a.lastErr != nil && !a.nextRefreshAt.IsZero() && time.Now().After(a.nextRefreshAt) {
		return a.lastErr
	}
	return nil
}

func (a *Adapter) markStaleServeLogged() error {
	a.healthMu.Lock()
	defer a.healthMu.Unlock()

	if a.lastSuccessAt.IsZero() || a.lastErr == nil {
		return nil
	}
	if !a.nextRefreshAt.IsZero() && time.Now().After(a.nextRefreshAt) {
		return nil
	}

	a.staleServeLogCount++
	if a.staleServeLogCount == 1 || a.staleServeLogCount%staleServeRelogEvery == 0 {
		return a.lastErr
	}
	return nil
}

func (a *Adapter) recordSuccess() {
	a.healthMu.Lock()
	defer a.healthMu.Unlock()

	a.lastSuccessAt = time.Now()
	if a.resource != nil {
		a.nextRefreshAt = a.resource.Next()
	}
	a.lastErr = nil
	a.staleServeLogCount = 0
}

func (a *Adapter) recordFailure(err error) {
	if err == nil {
		return
	}

	a.healthMu.Lock()
	defer a.healthMu.Unlock()
	a.lastErr = err
}

func (a *Adapter) newRefreshContext(ctx context.Context) (context.Context, context.CancelFunc) {
	refreshCtx := context.WithoutCancel(ctx)
	if a.fetchTimeout > 0 {
		return context.WithTimeout(refreshCtx, a.fetchTimeout)
	}
	if deadline, ok := ctx.Deadline(); ok {
		return context.WithDeadline(refreshCtx, deadline)
	}
	return refreshCtx, func() {}
}

func (a *Adapter) startRefresh(ctx context.Context) *refreshState {
	a.refreshMu.Lock()
	if a.refresh != nil {
		state := a.refresh
		a.refreshMu.Unlock()
		return state
	}

	state := &refreshState{done: make(chan struct{})}
	a.refresh = state
	a.refreshMu.Unlock()

	go func() {
		refreshCtx, refreshCancel := a.newRefreshContext(ctx)
		defer refreshCancel()

		a.mu.Lock()
		ctrl := a.ctrl
		a.mu.Unlock()

		err := fmt.Errorf("upstream JWKS resource not available")
		if ctrl != nil {
			err = ctrl.Refresh(refreshCtx, a.jwksURI)
		}
		if err != nil {
			// ctrl.Refresh is the caller-driven path and does not flow through the
			// errsink callback above, so record degraded health here as well.
			a.recordFailure(err)
		}

		a.refreshMu.Lock()
		state.err = err
		close(state.done)
		if a.refresh == state {
			a.refresh = nil
		}
		a.refreshMu.Unlock()
	}()

	return state
}

func awaitRefresh(ctx context.Context, state *refreshState) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-state.done:
		if err := ctx.Err(); err != nil {
			return err
		}
		return state.err
	}
}

// GetKey looks up kid in the current JWKS cache.
func (a *Adapter) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
	keyset, err := a.GetKeySet(ctx)
	if err != nil {
		return nil, err
	}

	key, found := keyset.LookupKeyID(kid)
	if !found {
		return nil, fmt.Errorf("key not found in jwks (kid: %s)", kid)
	}

	return key, nil
}

// Shutdown gracefully shuts down the adapter, stopping background refresh goroutines
// and closing any open connections. Should be called during application shutdown.
//
// Parameters:
//   - ctx: Context for cancellation support (typically a shutdown context with timeout)
//
// Returns error if shutdown fails.
func (a *Adapter) Shutdown(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.ctrl == nil {
		return nil
	}

	ctrl := a.ctrl
	a.ctrl = nil
	return ctrl.ShutdownContext(ctx)
}
