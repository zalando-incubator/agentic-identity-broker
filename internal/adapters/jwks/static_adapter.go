package jwks

import (
	"context"
	"fmt"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// StaticAdapter serves an inline Local Key Set for the process lifetime. It never refreshes and
// makes no outbound request.
type StaticAdapter struct {
	set trustanchor.LocalKeySet
}

var _ ports.JWKSPort = (*StaticAdapter)(nil)

// NewStaticAdapter creates an adapter that serves set.
func NewStaticAdapter(set trustanchor.LocalKeySet) *StaticAdapter {
	return &StaticAdapter{set: set}
}

// GetKeySet returns the inline key set.
func (a *StaticAdapter) GetKeySet(_ context.Context) (jwk.Set, error) {
	return a.set.Set, nil
}

// GetKey looks up kid in the inline key set.
func (a *StaticAdapter) GetKey(_ context.Context, kid string) (jwk.Key, error) {
	return lookupKey(a.set.Set, kid)
}

// HealthState is always healthy: an inline key set has no external dependency.
func (a *StaticAdapter) HealthState() ports.ComponentHealth {
	return ports.ComponentHealthHealthy
}

// LocalKeySet returns the served key set, for startup diagnostics.
func (a *StaticAdapter) LocalKeySet() trustanchor.LocalKeySet {
	return a.set
}

func lookupKey(set jwk.Set, kid string) (jwk.Key, error) {
	key, found := set.LookupKeyID(kid)
	if !found {
		return nil, fmt.Errorf("key not found in jwks (kid: %s)", kid)
	}
	return key, nil
}
