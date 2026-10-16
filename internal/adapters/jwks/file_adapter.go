package jwks

import (
	"context"
	"log/slog"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// FileAdapter serves a Local Key Set read from a file in the trust directory and re-reads it
// within the trust anchor's refresh bounds.
type FileAdapter struct{}

var _ ports.JWKSPort = (*FileAdapter)(nil)

// NewFileAdapter reads and validates path once, then re-reads it every bounds.Min.
func NewFileAdapter(
	dir *TrustDirectory,
	path string,
	bounds trustanchor.RefreshBounds,
	anchorPath string,
	logger *slog.Logger,
) (*FileAdapter, error) {
	_, _, _, _, _ = dir, path, bounds, anchorPath, logger
	return nil, errNotImplemented
}

// GetKeySet returns the last valid key set, or an error once it is older than the maximum refresh.
func (a *FileAdapter) GetKeySet(_ context.Context) (jwk.Set, error) {
	return nil, errNotImplemented
}

// GetKey looks up kid in the current key set.
func (a *FileAdapter) GetKey(_ context.Context, _ string) (jwk.Key, error) {
	return nil, errNotImplemented
}

// HealthState is degraded while the latest re-read has failed.
func (a *FileAdapter) HealthState() ports.ComponentHealth {
	return ports.ComponentHealthDegraded
}

// Shutdown stops the re-read loop.
func (a *FileAdapter) Shutdown(_ context.Context) error {
	return nil
}
