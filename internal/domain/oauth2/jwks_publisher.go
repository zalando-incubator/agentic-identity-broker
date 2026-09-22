package oauth2

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// JWKSPublisherService implements JWKSPublisherPort using dependency-driven behavior.
// The builder selects the appropriate constructor based on mode; the service itself
// never inspects a mode string. It is safe for concurrent use.
const upstreamFailureRelogEvery = 10

type JWKSPublisherService struct {
	tokenSigningKeys           ports.SigningKeyManager
	upstreamKeys               ports.JWKSPort
	requiresUpstream           bool
	logger                     *slog.Logger
	consecutiveUpstreamFailure atomic.Int64
}

var _ ports.JWKSPublisherPort = (*JWKSPublisherService)(nil)
var _ ports.JWKSPublisherHealthPort = (*JWKSPublisherService)(nil)

// NewLocalJWKSPublisher creates a publisher that serves only broker token-signing keys.
// Used in local mode where no upstream key source exists.
func NewLocalJWKSPublisher(tokenSigningKeys ports.SigningKeyManager, logger *slog.Logger) *JWKSPublisherService {
	if tokenSigningKeys == nil {
		panic("BUG: NewLocalJWKSPublisher requires non-nil tokenSigningKeys")
	}
	if logger == nil {
		panic("BUG: NewLocalJWKSPublisher requires non-nil logger")
	}
	return &JWKSPublisherService{
		tokenSigningKeys: tokenSigningKeys,
		requiresUpstream: false,
		logger:           logger,
	}
}

// NewProxyJWKSPublisher creates a publisher that republishes upstream keys only.
func NewProxyJWKSPublisher(upstreamKeys ports.JWKSPort, logger *slog.Logger) *JWKSPublisherService {
	if upstreamKeys == nil {
		panic("BUG: NewProxyJWKSPublisher requires non-nil upstreamKeys")
	}
	if logger == nil {
		panic("BUG: NewProxyJWKSPublisher requires non-nil logger")
	}
	return &JWKSPublisherService{
		upstreamKeys:     upstreamKeys,
		requiresUpstream: true,
		logger:           logger,
	}
}

// NewHybridJWKSPublisher creates a publisher that aggregates token-signing and upstream keys.
func NewHybridJWKSPublisher(tokenSigningKeys ports.SigningKeyManager, upstreamKeys ports.JWKSPort, logger *slog.Logger) *JWKSPublisherService {
	if tokenSigningKeys == nil {
		panic("BUG: NewHybridJWKSPublisher requires non-nil tokenSigningKeys")
	}
	if upstreamKeys == nil {
		panic("BUG: NewHybridJWKSPublisher requires non-nil upstreamKeys")
	}
	if logger == nil {
		panic("BUG: NewHybridJWKSPublisher requires non-nil logger")
	}
	return &JWKSPublisherService{
		tokenSigningKeys: tokenSigningKeys,
		upstreamKeys:     upstreamKeys,
		requiresUpstream: true,
		logger:           logger,
	}
}

// PublishJWKS returns the aggregated JWKS based on the configured key sources.
func (s *JWKSPublisherService) PublishJWKS(ctx context.Context) (jwk.Set, error) {
	var upstream jwk.Set
	if s.requiresUpstream {
		var err error
		upstream, err = s.fetchUpstream(ctx)
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ports.ErrUpstreamUnavailable, err)
		}
	}

	var local jwk.Set
	if s.tokenSigningKeys != nil {
		var err error
		local, err = s.tokenSigningKeys.BuildJWKS(ctx)
		if err != nil {
			s.logger.Error("failed to build local JWKS", "error", err)
			return nil, fmt.Errorf("failed to build local JWKS: %w", err)
		}
	}

	if local != nil && upstream != nil {
		return s.mergeWithKidCheck(local, upstream)
	}
	if local != nil {
		return local, nil
	}
	return upstream, nil
}

// HealthState returns ComponentHealthHealthy while upstream key material remains
// servable and ComponentHealthDegraded once it becomes unavailable or stale.
// Always returns ComponentHealthHealthy when upstream is not required.
func (s *JWKSPublisherService) HealthState() ports.ComponentHealth {
	if !s.requiresUpstream {
		return ports.ComponentHealthHealthy
	}
	return s.upstreamKeys.HealthState()
}

func (s *JWKSPublisherService) mergeWithKidCheck(local, upstream jwk.Set) (jwk.Set, error) {
	localKids := make(map[string]struct{}, local.Len())
	for i := 0; i < local.Len(); i++ {
		key, ok := local.Key(i)
		if !ok {
			continue
		}
		if kid, ok := key.KeyID(); ok && kid != "" {
			localKids[kid] = struct{}{}
		}
	}

	for i := 0; i < upstream.Len(); i++ {
		key, ok := upstream.Key(i)
		if !ok {
			continue
		}
		if kid, ok := key.KeyID(); ok && kid != "" {
			if _, conflict := localKids[kid]; conflict {
				s.logger.Error("kid conflict detected between local and upstream key sets",
					"kid", kid)
				return nil, ports.ErrKidConflict
			}
		}
	}

	merged := jwk.NewSet()
	for i := 0; i < local.Len(); i++ {
		key, ok := local.Key(i)
		if !ok {
			continue
		}
		if err := merged.AddKey(key); err != nil {
			return nil, fmt.Errorf("failed to add local key to merged set: %w", err)
		}
	}
	for i := 0; i < upstream.Len(); i++ {
		key, ok := upstream.Key(i)
		if !ok {
			continue
		}
		if err := merged.AddKey(key); err != nil {
			return nil, fmt.Errorf("failed to add upstream key to merged set: %w", err)
		}
	}

	return merged, nil
}

func (s *JWKSPublisherService) fetchUpstream(ctx context.Context) (jwk.Set, error) {
	set, err := s.upstreamKeys.GetKeySet(ctx)
	if err != nil {
		return nil, s.recordUpstreamFailure(err)
	}

	failures := s.consecutiveUpstreamFailure.Swap(0)
	if failures > 0 {
		s.logger.Info("upstream JWKS fetch recovered", "consecutive_failures", failures)
	}

	return set, nil
}

func (s *JWKSPublisherService) recordUpstreamFailure(err error) error {
	failures := s.consecutiveUpstreamFailure.Add(1)
	if failures == 1 || failures%upstreamFailureRelogEvery == 0 {
		s.logger.Error("upstream JWKS fetch failed",
			"error", err,
			"consecutive_failures", failures,
		)
	}
	return err
}
