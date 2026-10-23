package ports

import (
	"context"
	"errors"
)

var (
	ErrOAuthDiscoveryNotFound          = errors.New("OAuth discovery document not found")
	ErrOAuthDiscoveryUnsafeDestination = errors.New("unsafe OAuth discovery destination")
	ErrOAuthDiscoveryResponseTooLarge  = errors.New("OAuth discovery response too large")
	ErrOAuthDiscoveryTimeout           = errors.New("OAuth discovery timed out")
	ErrOAuthDiscoveryUnavailable       = errors.New("OAuth discovery endpoint unavailable")
	ErrOAuthDiscoveryInvalidResponse   = errors.New("invalid OAuth discovery response")
	ErrOAuthDiscoveryRejected          = errors.New("OAuth client registration rejected")
)

// OAuthDiscoveryClient performs outbound resource probes, metadata reads, and
// registration requests. The caller selects URLs and validates their contents.
type OAuthDiscoveryClient interface {
	Probe(ctx context.Context, rawURL string) (status int, challenges []string, err error)
	GetJSON(ctx context.Context, rawURL string) ([]byte, error)
	PostJSON(ctx context.Context, rawURL string, body []byte) ([]byte, error)
}
