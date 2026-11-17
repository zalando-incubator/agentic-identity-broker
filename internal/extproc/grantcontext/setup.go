package grantcontext

import (
	"context"
	"fmt"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
)

// NewPermitResolverFromConfig discovers the configured PDP without credentials and builds the
// permit resolver. Any discovery failure is returned so startup stops instead of silently
// disabling grant context. cfg must already be validated.
func NewPermitResolverFromConfig(ctx context.Context, cfg *extprocconfig.Config, assertions AssertionProvider) (*PermitResolver, error) {
	httpClient, err := httpclient.New(cfg, cfg.GrantContext.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize grant context HTTP client: %w", err)
	}
	metadata, err := Discover(ctx, httpClient, cfg.GrantContext.PolicyDecisionPoint, cfg.OAuth2.TLS.AllowHTTP)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(httpClient, metadata, assertions, cfg.GrantContext.MaxStaleness)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize grant context client: %w", err)
	}
	return NewPermitResolver(client, cfg.GrantContext.MaxStaleness), nil
}
