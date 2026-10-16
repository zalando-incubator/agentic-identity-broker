package jwks

import (
	"bytes"
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// keySetReader reads a key-set document. *TrustDirectory is the production implementation.
type keySetReader interface {
	ReadFile(path string) ([]byte, error)
}

// FileAdapter serves a Local Key Set read from a file in the trust directory. It re-reads the
// file every bounds.Min. A failed re-read keeps the last valid set and reports degraded health;
// once bounds.Max has passed since the last successful read, GetKeySet fails closed until a
// valid re-read succeeds.
type FileAdapter struct {
	reader     keySetReader
	path       string
	anchorPath string
	bounds     trustanchor.RefreshBounds
	logger     *slog.Logger

	mu            sync.RWMutex
	current       trustanchor.LocalKeySet
	document      []byte
	lastSuccessAt time.Time
	lastErr       error
	expiryLogged  bool

	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

var _ ports.JWKSPort = (*FileAdapter)(nil)

// NewFileAdapter reads and validates path through dir once, then re-reads it every bounds.Min.
// anchorPath is the trust anchor's configuration path, used in errors and logs.
func NewFileAdapter(
	dir *TrustDirectory,
	path string,
	bounds trustanchor.RefreshBounds,
	anchorPath string,
	logger *slog.Logger,
) (*FileAdapter, error) {
	return newFileAdapter(dir, path, bounds, anchorPath, logger)
}

func newFileAdapter(
	reader keySetReader,
	path string,
	bounds trustanchor.RefreshBounds,
	anchorPath string,
	logger *slog.Logger,
) (*FileAdapter, error) {
	a := &FileAdapter{
		reader:     reader,
		path:       path,
		anchorPath: anchorPath,
		bounds:     bounds,
		logger:     logger,
		stop:       make(chan struct{}),
		done:       make(chan struct{}),
	}
	doc, set, err := a.read()
	if err != nil {
		return nil, a.locate(err)
	}
	a.current, a.document, a.lastSuccessAt = set, doc, time.Now()
	go a.loop()
	return a, nil
}

// GetKeySet returns the last valid key set, or a stale error once it has not been read
// successfully within the maximum refresh interval.
func (a *FileAdapter) GetKeySet(_ context.Context) (jwk.Set, error) {
	a.mu.RLock()
	set, lastErr, lastSuccessAt := a.current.Set, a.lastErr, a.lastSuccessAt
	a.mu.RUnlock()
	if lastErr == nil || time.Since(lastSuccessAt) <= a.bounds.Max {
		return set, nil
	}

	a.mu.Lock()
	if !a.expiryLogged && a.lastErr != nil {
		a.expiryLogged = true
		a.logger.Warn("trust anchor key set expired",
			"trust_anchor", a.anchorPath,
			"key_source", string(trustanchor.SourceFile),
			"path", a.path,
			"error_kind", string(trustanchor.CategoryStale),
		)
	}
	a.mu.Unlock()
	return nil, trustanchor.At(a.anchorPath, &trustanchor.Error{Category: trustanchor.CategoryStale})
}

// GetKey looks up kid in the current key set.
func (a *FileAdapter) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
	set, err := a.GetKeySet(ctx)
	if err != nil {
		return nil, err
	}
	return lookupKey(set, kid)
}

// HealthState is degraded while the latest re-read has failed.
func (a *FileAdapter) HealthState() ports.ComponentHealth {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if a.lastErr != nil {
		return ports.ComponentHealthDegraded
	}
	return ports.ComponentHealthHealthy
}

// LocalKeySet returns the current key set, for startup diagnostics.
func (a *FileAdapter) LocalKeySet() trustanchor.LocalKeySet {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.current
}

// Shutdown stops the re-read loop. It is safe to call more than once.
func (a *FileAdapter) Shutdown(ctx context.Context) error {
	a.stopOnce.Do(func() { close(a.stop) })
	select {
	case <-a.done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (a *FileAdapter) loop() {
	defer close(a.done)
	ticker := time.NewTicker(a.bounds.Min)
	defer ticker.Stop()
	for {
		select {
		case <-a.stop:
			return
		case <-ticker.C:
			a.reread()
		}
	}
}

func (a *FileAdapter) read() ([]byte, trustanchor.LocalKeySet, error) {
	doc, err := a.reader.ReadFile(a.path)
	if err != nil {
		return nil, trustanchor.LocalKeySet{}, err
	}
	set, err := trustanchor.ParseLocalKeySet(doc)
	if err != nil {
		return nil, trustanchor.LocalKeySet{}, err
	}
	return doc, set, nil
}

func (a *FileAdapter) reread() {
	doc, set, err := a.read()

	a.mu.Lock()
	defer a.mu.Unlock()
	if err != nil {
		a.lastErr = err
		a.logger.Warn("trust anchor key set re-read failed",
			"trust_anchor", a.anchorPath,
			"key_source", string(trustanchor.SourceFile),
			"path", a.path,
			"error_kind", string(trustanchor.CategoryOf(err)),
			"last_success_at", a.lastSuccessAt.UTC(),
			"fails_closed_at", a.lastSuccessAt.Add(a.bounds.Max).UTC(),
		)
		return
	}

	recovered := a.lastErr != nil
	changed := !bytes.Equal(doc, a.document)
	previous := a.current
	a.current, a.document, a.lastSuccessAt, a.lastErr, a.expiryLogged = set, doc, time.Now(), nil, false
	if changed {
		added, removed := trustanchor.DiffKeyIDs(previous, set)
		a.logger.Info("trust anchor key set changed",
			"trust_anchor", a.anchorPath,
			"key_source", string(trustanchor.SourceFile),
			"path", a.path,
			"key_count", set.Set.Len(),
			"added_key_ids", nonNil(added),
			"removed_key_ids", nonNil(removed),
		)
	}
	if recovered {
		a.logger.Info("trust anchor key set recovered",
			"trust_anchor", a.anchorPath,
			"key_source", string(trustanchor.SourceFile),
			"path", a.path,
			"key_count", set.Set.Len(),
		)
	}
}

// locate reports a startup error against the trust anchor for file rules and against the
// jwks_file setting for key-set document rules.
func (a *FileAdapter) locate(err error) error {
	switch trustanchor.CategoryOf(err) {
	case trustanchor.CategoryOutsideTrustDirectory, trustanchor.CategoryNotFound,
		trustanchor.CategoryUnreadable, trustanchor.CategoryNotRegularFile:
		return trustanchor.At(a.anchorPath, err)
	default:
		return trustanchor.At(a.anchorPath+".jwks_file", err)
	}
}

func nonNil(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}
