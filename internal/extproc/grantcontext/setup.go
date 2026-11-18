package grantcontext

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
)

const (
	discoveryRetryInitial = time.Second
	discoveryRetryMax     = 10 * time.Second
)

// NewPermitResolverFromConfig discovers the configured PDP without credentials and builds the
// permit resolver. A failed discovery (ErrDiscoveryFailed) is retried with a capped backoff until
// ctx ends, so a broker outage keeps ExtProc from listening instead of exiting. A
// policy_decision_point mismatch or an invalid advertised endpoint stops startup at once. ExtProc
// never guesses an endpoint or silently disables grant context. cfg must already be validated.
func NewPermitResolverFromConfig(ctx context.Context, cfg *extprocconfig.Config, assertions AssertionProvider, logger *slog.Logger) (*PermitResolver, error) {
	httpClient, err := httpclient.New(cfg, cfg.GrantContext.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize grant context HTTP client: %w", err)
	}
	metadata, err := discoverWithRetry(ctx, httpClient, cfg, logger)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(httpClient, metadata, assertions, cfg.GrantContext.MaxStaleness)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize grant context client: %w", err)
	}
	return NewPermitResolver(client, cfg.GrantContext.MaxStaleness), nil
}

func discoverWithRetry(ctx context.Context, httpClient *http.Client, cfg *extprocconfig.Config, logger *slog.Logger) (*Metadata, error) {
	for delay := discoveryRetryInitial; ; delay = min(2*delay, discoveryRetryMax) {
		metadata, err := Discover(ctx, httpClient, cfg.GrantContext.PolicyDecisionPoint, cfg.OAuth2.TLS.AllowHTTP)
		if err == nil || !errors.Is(err, ErrDiscoveryFailed) || errors.Is(err, context.Canceled) {
			return metadata, err
		}
		logger.WarnContext(ctx, "grant context discovery failed, retrying",
			"event", "extproc.grant_context.discovery_failed", "error", err, "retry_after", delay)
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, fmt.Errorf("%w: %w", ErrDiscoveryFailed, ctx.Err())
		case <-timer.C:
		}
	}
}
