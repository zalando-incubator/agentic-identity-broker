// Package server — exchanger.go implements RFC 8693 token exchange with
// in-memory caching, singleflight deduplication, and background client
// assertion refresh. It is safe for concurrent use.
package server

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/clientcredentials"
	"golang.org/x/sync/singleflight"

	"github.com/sony/gobreaker/v2"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

// tokenCacheKey uniquely identifies a cached token by subject + resource.
// Using a struct as a map key avoids separator-injection attacks.
type tokenCacheKey struct {
	subjectToken string
	resourceURI  string
}

// ErrAssertionExpired is returned by Exchange when the stored client assertion
// has expired and the background refresh has not yet succeeded.
// Callers should surface this as a 503 Service Unavailable to signal a transient
// infrastructure failure rather than a client error.
var ErrAssertionExpired = errors.New("client assertion expired")

// reAuthCooldownTTL is the minimum interval between broker calls for the same
// token+resource key when the broker returns a re-auth error. It prevents tight
// retry loops from hammering the broker during an active re-auth flow while
// remaining short enough that a completed re-auth is observable on the next
// attempt (re-auth flows take longer than 5 s in practice).
const reAuthCooldownTTL = 5 * time.Second

// cachedToken holds either an exchanged access token or a re-auth rate-limit
// entry. reAuthErr and accessToken are mutually exclusive: when reAuthErr is
// set the entry acts as a short-lived rate-limit, not a long-lived cache.
//
// Success entries are served from cache until expiresAt with no intermediate
// broker probe. grantedPermissionSets is cached alongside the access token as the
// same token-bound snapshot, so it inherits the same TTL and stale-window bounds.
// staleUntil is an immutable upper bound set at population time
// (min(expiresAt + reAuthCooldownTTL, issueTime + cache.max_ttl)) allowing the
// stale token to be served for a brief outage window. When ttl == cache.max_ttl
// (the common case where expires_in exceeds the cap), staleUntil == expiresAt
// and the token is never served past the configured maximum. Because it is fixed
// at token creation it cannot be extended by repeated transient failures.
type cachedToken struct {
	accessToken           string
	grantedPermissionSets map[string][]string
	reAuthErr             *BrokerExchangeError // non-nil = rate-limit window for re-auth
	expiresAt             time.Time
	staleUntil            time.Time // immutable upper bound for stale-token fallback; set at population
}

// isExpired reports whether the cached entry has expired.
func (c *cachedToken) isExpired() bool {
	return time.Now().After(c.expiresAt)
}

// tokenExchangeResponse is the JSON shape from the RFC 8693 token exchange endpoint.
type tokenExchangeResponse struct {
	AccessToken           string              `json:"access_token"`
	TokenType             string              `json:"token_type"`
	ExpiresIn             *int                `json:"expires_in"` // pointer: nil means absent
	GrantedPermissionSets map[string][]string `json:"granted_permission_sets,omitempty"`
}

func cloneGrantedPermissionSets(src map[string][]string) map[string][]string {
	if src == nil {
		return nil
	}
	dst := make(map[string][]string, len(src))
	for permissionSetID, serviceIDs := range src {
		dst[permissionSetID] = append([]string(nil), serviceIDs...)
	}
	return dst
}

// brokerErrorBody is the JSON shape returned by the broker on non-200 responses.
type brokerErrorBody struct {
	Code        string `json:"error"`
	Description string `json:"error_description"`
	ErrorURI    string `json:"error_uri,omitempty"`
}

// BrokerExchangeError is returned by Exchange when the broker responds with a non-200 status
// and a parseable RFC 8693 error body. ErrorURI, when non-empty, carries the re-authentication
// URL from the broker (RFC 6749 §5.2 error_uri). Callers that detect a non-empty ErrorURI
// should return a MCP URLElicitationRequiredError (JSON-RPC -32042).
type BrokerExchangeError struct {
	StatusCode  int
	Code        string
	Description string
	ErrorURI    string
}

func (e *BrokerExchangeError) Error() string {
	if e.Description != "" {
		return fmt.Sprintf("broker error %d: %s (%s)", e.StatusCode, e.Code, e.Description)
	}
	return fmt.Sprintf("broker error %d: %s", e.StatusCode, e.Code)
}

// assertionState is the atomically-swapped snapshot of the client assertion.
// Storing all fields together ensures readers always observe a consistent snapshot.
type assertionState struct {
	value     string
	issuedAt  time.Time
	expiresAt time.Time
}

// TokenExchanger implements the Exchanger interface.
// It manages a token cache, singleflight group, circuit breaker, and HTTP
// client for outbound calls to the identity broker and the upstream OAuth2 server.
type TokenExchanger struct {
	cfg    *extprocconfig.Config
	client *http.Client
	logger *slog.Logger

	// assertion holds the current client assertion (id_token or access_token).
	// Written only by the background refresh goroutine; read atomically by Exchange callers.
	// Reading a slightly stale but still-valid assertion is acceptable and avoids locking.
	assertion atomic.Pointer[assertionState]

	// cache holds exchanged tokens keyed by subject+resource.
	cacheMu sync.RWMutex
	cache   map[tokenCacheKey]*cachedToken

	// sfGroup deduplicates concurrent cache refresh calls for the same key.
	sfGroup singleflight.Group

	// cb protects outbound token exchange calls with a circuit breaker
	// to prevent thundering herd when the identity broker recovers.
	cb *gobreaker.CircuitBreaker[ExchangeResult]

	// stopCh signals the background goroutine to stop.
	stopCh chan struct{}
}

// NewTokenExchanger creates a new TokenExchanger and performs a startup
// client_credentials grant to obtain the initial client assertion.
// Returns an error if the startup assertion grant fails (fail-fast per FR-006).
func NewTokenExchanger(cfg *extprocconfig.Config, logger *slog.Logger) (*TokenExchanger, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config must not be nil")
	}

	httpClient, err := buildHTTPClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("building HTTP client: %w", err)
	}

	te := &TokenExchanger{
		cfg:    cfg,
		client: httpClient,
		logger: logger,
		cache:  make(map[tokenCacheKey]*cachedToken),
		stopCh: make(chan struct{}),
	}

	// Only create the circuit breaker when enabled (default: true).
	if cfg.CircuitBreaker.Enabled {
		te.cb = newGobreakerCB(cfg, logger)
	} else {
		logger.Info("circuit breaker disabled by configuration")
	}

	// Fail-fast: acquire initial client assertion at startup.
	if err := te.refreshClientAssertion(); err != nil {
		return nil, fmt.Errorf("startup client assertion failed: %w", err)
	}

	// Start background eviction goroutine.
	go te.runEviction()

	return te, nil
}

// isAssertionExpired reports whether the current client assertion is absent or past its expiry.
// Used by Exchange fast paths to skip the cache when ErrAssertionExpired would be returned anyway.
func (te *TokenExchanger) isAssertionExpired() bool {
	s := te.assertion.Load()
	if s == nil || s.value == "" {
		return true
	}
	return !s.expiresAt.IsZero() && time.Now().After(s.expiresAt)
}

// Exchange exchanges subjectToken for a downstream token scoped to resourceURI.
// ctx is used for trace propagation and deadline enforcement.
// Results are cached; concurrent requests for the same key are deduplicated via singleflight.
func (te *TokenExchanger) Exchange(ctx context.Context, subjectToken, resourceURI string) (ExchangeResult, error) {
	key := tokenCacheKey{subjectToken: subjectToken, resourceURI: resourceURI}
	logger := loggerFromContext(ctx, te.logger)

	// Fast path: cache hit before expiry.
	// Skip entirely on assertion expiry so ErrAssertionExpired reaches callers immediately,
	// even when a cached re-auth or success entry is present.
	te.cacheMu.RLock()
	if entry, ok := te.cache[key]; ok && !entry.isExpired() && !te.isAssertionExpired() {
		te.cacheMu.RUnlock()
		if entry.reAuthErr != nil {
			return ExchangeResult{}, entry.reAuthErr
		}
		return ExchangeResult{Token: entry.accessToken, GrantedPermissionSets: cloneGrantedPermissionSets(entry.grantedPermissionSets)}, nil
	}
	te.cacheMu.RUnlock()

	// Slow path: singleflight-deduplicated exchange.
	// Use DoChan to allow each caller to respect their own cancellation,
	// not just block on the first caller's context.
	sfKey := subjectToken + "\x00" + resourceURI
	resChan := te.sfGroup.DoChan(sfKey, func() (any, error) {
		// Re-check cache inside singleflight to handle races.
		te.cacheMu.RLock()
		if entry, ok := te.cache[key]; ok && !entry.isExpired() && !te.isAssertionExpired() {
			te.cacheMu.RUnlock()
			if entry.reAuthErr != nil {
				return ExchangeResult{}, entry.reAuthErr
			}
			return ExchangeResult{Token: entry.accessToken, GrantedPermissionSets: cloneGrantedPermissionSets(entry.grantedPermissionSets)}, nil
		}
		te.cacheMu.RUnlock()

		// Detach from the leader's cancellation so a cancelled leader does not abort
		// the shared exchange for concurrent followers. Per-caller cancellation is
		// handled by awaitResult's select. http.Client.Timeout provides a hard upper bound.
		exchangeCtx := context.WithoutCancel(ctx)

		// doExchangeAndCache performs the exchange and caches re-auth or success results.
		// All errors (including transient 5xx) are propagated so gobreaker tracks them
		// correctly. The stale-token fallback for transient failures is applied by the
		// caller AFTER Execute() returns, keeping failure accounting accurate.
		doExchangeAndCache := func() (ExchangeResult, error) {
			tok, permSets, ttl, err := te.doExchange(exchangeCtx, subjectToken, resourceURI)
			if err != nil {
				var brokerErr *BrokerExchangeError
				if errors.As(err, &brokerErr) && brokerErr.ErrorURI != "" && !isTransientBrokerError(err) {
					te.cacheMu.Lock()
					te.cache[key] = &cachedToken{
						reAuthErr: brokerErr,
						expiresAt: time.Now().Add(reAuthCooldownTTL),
					}
					te.cacheMu.Unlock()
				}
				return ExchangeResult{}, err
			}

			cachedPermSets := cloneGrantedPermissionSets(permSets)

			te.cacheMu.Lock()
			now := time.Now()
			staleUntil := now.Add(ttl + reAuthCooldownTTL)
			if maxAbs := now.Add(te.cfg.Cache.MaxTTL); maxAbs.Before(staleUntil) {
				staleUntil = maxAbs
			}
			te.cache[key] = &cachedToken{
				accessToken:           tok,
				grantedPermissionSets: cachedPermSets,
				expiresAt:             now.Add(ttl),
				staleUntil:            staleUntil,
			}
			te.cacheMu.Unlock()

			return ExchangeResult{Token: tok, GrantedPermissionSets: cloneGrantedPermissionSets(cachedPermSets)}, nil
		}

		// serveStaleOrErr returns the cached token (even if expired) for transient errors
		// (5xx, network errors, circuit open — anything isTransientBrokerError returns
		// true for). Authoritative 4xx rejections are propagated unchanged.
		// Stale tokens are never served after assertion expiry regardless of error type.
		// staleUntil is set at token population time (expiresAt + reAuthCooldownTTL)
		// and is never modified here, so the stale window is immutably bounded.
		// Must be called with execErr != nil.
		serveStaleOrErr := func(execErr error) (any, error) {
			if isTransientBrokerError(execErr) && !te.isAssertionExpired() {
				te.cacheMu.RLock()
				entry, ok := te.cache[key]
				if ok && entry.accessToken != "" && time.Now().Before(entry.staleUntil) {
					staleResult := ExchangeResult{Token: entry.accessToken, GrantedPermissionSets: cloneGrantedPermissionSets(entry.grantedPermissionSets)}
					te.cacheMu.RUnlock()
					return staleResult, nil
				}
				te.cacheMu.RUnlock()
			}
			return ExchangeResult{}, execErr
		}

		// When the circuit breaker is disabled (cb == nil), call doExchangeAndCache
		// directly, then apply the stale-token fallback for transient errors.
		if te.cb == nil {
			result, execErr := doExchangeAndCache()
			if execErr != nil {
				return serveStaleOrErr(execErr)
			}
			return result, nil
		}

		// Circuit breaker: wrap doExchangeAndCache so gobreaker tracks 5xx failures.
		// After Execute returns, apply the stale-token fallback outside the breaker so
		// transient probe failures are correctly counted as circuit-breaker failures.
		result, execErr := te.cb.Execute(doExchangeAndCache)
		if execErr != nil {
			if errors.Is(execErr, gobreaker.ErrOpenState) || errors.Is(execErr, gobreaker.ErrTooManyRequests) {
				execErr = ErrCircuitOpen
			}
			return serveStaleOrErr(execErr)
		}
		return result, nil
	})

	// Wait for result while respecting this caller's context cancellation.
	// This allows each caller to abandon long-running singleflight groups
	// if their own deadline is exceeded, even if other callers are still waiting.
	res, err := awaitResult(ctx, resChan)
	if err != nil {
		return ExchangeResult{}, err
	}
	if res.Shared {
		logger.DebugContext(ctx, "singleflight: exchange result shared across concurrent callers",
			"resource", sanitizeURIForTelemetry(resourceURI))
	}
	if res.Err != nil {
		return ExchangeResult{}, res.Err
	}
	return res.Val.(ExchangeResult), nil
}

// isTransientBrokerError reports whether err represents a transient infrastructure
// failure — a 5xx response, 429 throttling, or network error — rather than an
// authoritative rejection from the broker.
// Only transient errors should fall back to serving a stale cached token;
// authoritative responses (invalid_token, access_denied, 429 with error_uri, etc.)
// must be propagated so that revoked access is not silently extended.
// A 429 with a non-empty ErrorURI carries an explicit re-auth signal and must be
// treated as authoritative so the caller can surface the elicitation URL.
// ErrAssertionExpired is NOT transient: the service cannot re-authenticate on its
// own and the client must receive the 503 signal.
func isTransientBrokerError(err error) bool {
	if errors.Is(err, ErrAssertionExpired) {
		return false
	}
	var brokerErr *BrokerExchangeError
	if errors.As(err, &brokerErr) {
		if brokerErr.StatusCode == 429 {
			return brokerErr.ErrorURI == ""
		}
		return brokerErr.StatusCode >= 500
	}
	return true // network errors — transient
}

// awaitResult waits for the singleflight result while respecting ctx cancellation.
// When both ctx.Done() and resChan fire simultaneously, the post-receive ctx.Err()
// check ensures cancellation wins deterministically.
func awaitResult(ctx context.Context, resChan <-chan singleflight.Result) (singleflight.Result, error) {
	select {
	case <-ctx.Done():
		return singleflight.Result{}, ctx.Err()
	case res := <-resChan:
		if err := ctx.Err(); err != nil {
			return singleflight.Result{}, err
		}
		return res, nil
	}
}

// Shutdown signals the background goroutine to stop and releases resources.
func (te *TokenExchanger) Shutdown() {
	close(te.stopCh)
}

// doExchange performs the RFC 8693 token exchange HTTP call.
// ctx is used for trace propagation and deadline enforcement.
// Returns the exchanged access token, any granted permission sets, and the TTL to cache it for.
// Fails fast with ErrAssertionExpired if the stored assertion has expired.
func (te *TokenExchanger) doExchange(ctx context.Context, subjectToken, resourceURI string) (string, map[string][]string, time.Duration, error) {
	logger := loggerFromContext(ctx, te.logger)
	s := te.assertion.Load()
	if s == nil || s.value == "" {
		logger.ErrorContext(ctx, "client assertion unavailable: no assertion stored")
		return "", nil, 0, ErrAssertionExpired
	}
	if !s.expiresAt.IsZero() && time.Now().After(s.expiresAt) {
		logger.ErrorContext(ctx, "client assertion expired: background refresh did not complete in time",
			"expired_at", s.expiresAt.Format(time.RFC3339))
		return "", nil, 0, ErrAssertionExpired
	}
	assertion := s.value

	// Log the full exchange request parameters for observability.
	logger.DebugContext(ctx, "extproc: token exchange request",
		"token_endpoint", te.cfg.OAuth2.TokenEndpoint,
		"grant_type", "urn:ietf:params:oauth:grant-type:token-exchange",
		"subject_token_type", "urn:ietf:params:oauth:token-type:access_token",
		"resource", sanitizeURIForTelemetry(resourceURI),
		"client_assertion_type", "urn:ietf:params:oauth:client-assertion-type:jwt-bearer",
	)

	exchangeCtx, cancel := context.WithTimeout(ctx, te.cfg.OAuth2.ExchangeTimeout)
	defer cancel()

	form := url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {subjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"resource":              {resourceURI},
		"client_assertion":      {assertion},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
	}

	req, err := http.NewRequestWithContext(exchangeCtx, http.MethodPost,
		te.cfg.OAuth2.TokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", nil, 0, fmt.Errorf("building token exchange request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	otel.GetTextMapPropagator().Inject(exchangeCtx, propagation.HeaderCarrier(req.Header))

	resp, err := te.client.Do(req)
	if err != nil {
		return "", nil, 0, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, 0, fmt.Errorf("reading token exchange response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errBody brokerErrorBody
		_ = json.Unmarshal(body, &errBody)
		if errBody.Code != "" {
			logger.WarnContext(ctx, "token exchange returned broker error",
				"status", resp.StatusCode,
				"code", errBody.Code,
				"resource", sanitizeURIForTelemetry(resourceURI),
				"has_error_uri", errBody.ErrorURI != "")
			return "", nil, 0, &BrokerExchangeError{
				StatusCode:  resp.StatusCode,
				Code:        errBody.Code,
				Description: errBody.Description,
				ErrorURI:    errBody.ErrorURI,
			}
		}
		// Body is absent or not an RFC 8693 error — still return a typed error
		// carrying the HTTP status code so isServerError can correctly classify
		// 4xx responses without a parseable error body as client errors.
		logger.DebugContext(ctx, "token exchange non-200 response body is not RFC 8693 JSON",
			"status", resp.StatusCode,
			"resource", sanitizeURIForTelemetry(resourceURI))
		logger.WarnContext(ctx, "token exchange returned non-200",
			"status", resp.StatusCode,
			"resource", sanitizeURIForTelemetry(resourceURI))
		return "", nil, 0, &BrokerExchangeError{
			StatusCode: resp.StatusCode,
			Code:       fmt.Sprintf("http_%d", resp.StatusCode),
		}
	}

	var exResp tokenExchangeResponse
	if err := json.Unmarshal(body, &exResp); err != nil {
		return "", nil, 0, fmt.Errorf("parsing token exchange response: %w", err)
	}
	if exResp.AccessToken == "" {
		return "", nil, 0, fmt.Errorf("token exchange response missing access_token")
	}

	ttl := te.computeTTL(exResp.ExpiresIn)
	return exResp.AccessToken, cloneGrantedPermissionSets(exResp.GrantedPermissionSets), ttl, nil
}

// computeTTL returns the cache TTL for an exchanged token.
// Uses expires_in if present, falls back to default_ttl, capped at max_ttl.
func (te *TokenExchanger) computeTTL(expiresIn *int) time.Duration {
	var ttl time.Duration
	if expiresIn != nil && *expiresIn > 0 {
		ttl = time.Duration(*expiresIn) * time.Second
	} else {
		ttl = te.cfg.Cache.DefaultTTL
	}
	if ttl > te.cfg.Cache.MaxTTL {
		ttl = te.cfg.Cache.MaxTTL
	}
	return ttl
}

// refreshClientAssertion obtains a new client assertion (id_token or access_token based on config)
// via the client_credentials grant using golang.org/x/oauth2/clientcredentials.
// Thread-safe: may be called from the background refresh goroutine.
func (te *TokenExchanger) refreshClientAssertion() error {
	endpoint := te.cfg.OAuth2.ClientCredentialsEndpoint
	if endpoint == "" {
		endpoint = te.cfg.OAuth2.Issuer + "/oauth/token"
	}

	ctx, cancel := context.WithTimeout(context.Background(), te.cfg.OAuth2.ExchangeTimeout)
	defer cancel()
	ctx = context.WithValue(ctx, oauth2.HTTPClient, te.client)

	conf := &clientcredentials.Config{
		ClientID:     te.cfg.OAuth2.ClientID,
		ClientSecret: te.cfg.OAuth2.ClientSecret,
		TokenURL:     endpoint,
		Scopes:       clientCredentialsScopes(te.cfg.OAuth2.ClientCredentialsScopes),
		AuthStyle:    oauth2.AuthStyleInParams,
	}

	token, err := conf.Token(ctx)
	if err != nil {
		te.logger.Error("client credentials grant failed", "error", err)
		return fmt.Errorf("client credentials grant failed: %w", err)
	}

	var assertion string
	switch te.cfg.OAuth2.ClientAssertionType {
	case "id_token":
		idToken, ok := token.Extra("id_token").(string)
		if !ok || idToken == "" {
			return fmt.Errorf("client credentials response missing id_token (required for client assertion with client_assertion_type=id_token)")
		}
		assertion = idToken
	case "access_token":
		if token.AccessToken == "" {
			return fmt.Errorf("client credentials response missing access_token (required for client assertion with client_assertion_type=access_token)")
		}
		assertion = token.AccessToken
	default:
		return fmt.Errorf("invalid client_assertion_type: %q", te.cfg.OAuth2.ClientAssertionType)
	}

	te.assertion.Store(&assertionState{value: assertion, issuedAt: time.Now(), expiresAt: token.Expiry})

	te.logger.Debug("client assertion refreshed",
		"assertion_type", te.cfg.OAuth2.ClientAssertionType,
		"expires_at", token.Expiry.Format(time.RFC3339))
	return nil
}

// runEviction runs two independent background tasks until Shutdown() is called:
//   - Cache eviction: fires every DefaultTTL/2 (per spec, floor 1s) to sweep expired entries.
//   - Assertion refresh: fires every min(DefaultTTL/2, 30s) (floor 1s) to proactively renew
//     the client assertion before expiry, independent of the eviction cadence.
func (te *TokenExchanger) runEviction() {
	evictInterval := max(te.cfg.Cache.DefaultTTL/2, time.Second)
	assertionInterval := min(max(te.cfg.Cache.DefaultTTL/2, time.Second), 30*time.Second)

	evictTicker := time.NewTicker(evictInterval)
	assertionTicker := time.NewTicker(assertionInterval)
	defer evictTicker.Stop()
	defer assertionTicker.Stop()

	for {
		select {
		case <-evictTicker.C:
			te.evictExpired()
		case <-assertionTicker.C:
			te.maybeRefreshAssertion()
		case <-te.stopCh:
			return
		}
	}
}

// evictExpired removes expired entries from the cache.
// Entries that are expired but still within their stale window are kept until the
// stale window passes so serveStaleOrErr can continue serving them.
func (te *TokenExchanger) evictExpired() {
	te.cacheMu.Lock()
	defer te.cacheMu.Unlock()
	now := time.Now()
	for key, entry := range te.cache {
		if entry.isExpired() && (entry.staleUntil.IsZero() || entry.staleUntil.Before(now)) {
			delete(te.cache, key)
		}
	}
}

// maybeRefreshAssertion refreshes the client assertion proactively.
// It triggers when the remaining lifetime is less than the larger of:
//   - 20% of the total token lifetime (exp − iat), or
//   - 30 seconds (hard floor to ensure refresh before imminent expiry).
func (te *TokenExchanger) maybeRefreshAssertion() {
	s := te.assertion.Load()
	if s == nil || s.expiresAt.IsZero() {
		return
	}

	lifetime := s.expiresAt.Sub(s.issuedAt)
	if s.issuedAt.IsZero() || lifetime <= 0 {
		// Fall back to hard floor when lifetime is unknown.
		lifetime = 0
	}
	threshold := max(lifetime/5, 30*time.Second) // 20% of lifetime, floor 30s

	if remaining := time.Until(s.expiresAt); remaining < threshold {
		if err := te.refreshClientAssertion(); err != nil {
			te.logger.Error("background client assertion refresh failed",
				"error", err)
		}
	}
}

// clientCredentialsScopes returns the OAuth2 scopes for the client_credentials grant.
// When no scopes are configured, or when all configured entries are blank after trimming,
// it falls back to ["openid"] to obtain an id_token.
func clientCredentialsScopes(configured []string) []string {
	filtered := make([]string, 0, len(configured))
	for _, s := range configured {
		if trimmed := strings.TrimSpace(s); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	if len(filtered) == 0 {
		return []string{"openid"}
	}
	return filtered
}

// buildHTTPClient constructs an http.Client respecting the TLS configuration.
// Supports InsecureSkipVerify and CaBundlePath from the TLS config block.
// Returns an error if CaBundlePath is set but the file cannot be read or parsed.
func buildHTTPClient(cfg *extprocconfig.Config) (*http.Client, error) {
	tlsCfg := &tls.Config{
		InsecureSkipVerify: cfg.OAuth2.TLS.InsecureSkipVerify, // #nosec G402 -- TLS verification is disabled only by explicit operator configuration; the default is false.
	}

	if cfg.OAuth2.TLS.CaBundlePath != "" {
		pemData, err := os.ReadFile(cfg.OAuth2.TLS.CaBundlePath)
		if err != nil {
			return nil, fmt.Errorf("reading ca_bundle_path %q: %w", cfg.OAuth2.TLS.CaBundlePath, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("ca_bundle_path %q contains no valid PEM certificates", cfg.OAuth2.TLS.CaBundlePath)
		}
		tlsCfg.RootCAs = pool
	}

	transport := &http.Transport{
		TLSClientConfig: tlsCfg,
	}

	// Wrap with otelhttp for automatic span creation on outbound requests.
	// otelhttp resolves the TracerProvider lazily (from otel.GetTracerProvider() at
	// request time, not construction time), so this transport correctly picks up
	// provider changes made after construction — including E2E test global swaps.
	tracedTransport := otelhttp.NewTransport(transport)

	// http.Client.Timeout is the hard deadline for the entire request lifecycle.
	// doExchange also applies context.WithTimeout per call; both use ExchangeTimeout.
	return &http.Client{
		Timeout:   cfg.OAuth2.ExchangeTimeout,
		Transport: tracedTransport,
	}, nil
}
