package delegation

import (
	"context"
	"fmt"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/httpclient"
)

// NewResolverFromConfig discovers the configured PDP without credentials and builds the
// permit resolver. Any discovery failure is returned so startup stops instead of silently
// disabling delegation context. cfg must already be validated.
func NewResolverFromConfig(ctx context.Context, cfg *extprocconfig.Config, assertions AssertionProvider) (*Resolver, error) {
	httpClient, err := httpclient.New(cfg, cfg.DelegationContext.RequestTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize delegation context HTTP client: %w", err)
	}
	metadata, err := Discover(ctx, httpClient, cfg.DelegationContext.PolicyDecisionPoint, cfg.OAuth2.TLS.AllowHTTP)
	if err != nil {
		return nil, err
	}
	client, err := NewClient(httpClient, metadata, assertions, cfg.DelegationContext.MaxStaleness)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize delegation context client: %w", err)
	}
	return NewResolver(client, cfg.DelegationContext.MaxStaleness), nil
}
