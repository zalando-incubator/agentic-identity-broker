package jwks

import (
	"context"
	"errors"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var errNotImplemented = errors.New("not implemented")

// StaticAdapter serves an inline Local Key Set for the process lifetime. It never refreshes.
type StaticAdapter struct{}

var _ ports.JWKSPort = (*StaticAdapter)(nil)

// NewStaticAdapter creates an adapter that serves set.
func NewStaticAdapter(set trustanchor.LocalKeySet) (*StaticAdapter, error) {
	_ = set
	return nil, errNotImplemented
}

// GetKeySet returns the inline key set.
func (a *StaticAdapter) GetKeySet(_ context.Context) (jwk.Set, error) {
	return nil, errNotImplemented
}

// GetKey looks up kid in the inline key set.
func (a *StaticAdapter) GetKey(_ context.Context, _ string) (jwk.Key, error) {
	return nil, errNotImplemented
}

// HealthState is always healthy: an inline key set has no external dependency.
func (a *StaticAdapter) HealthState() ports.ComponentHealth {
	return ports.ComponentHealthDegraded
}
