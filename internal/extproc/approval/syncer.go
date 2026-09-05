package approval

import (
	"context"
	"log/slog"
	"time"
)

type Poller interface {
	Poll(context.Context, []string, string, time.Duration) ([]Pair, string, bool, error)
}

type Syncer struct {
	cache           *Cache
	client          Poller
	longPollTimeout time.Duration
	initialBackoff  time.Duration
	maximumBackoff  time.Duration
	logger          *slog.Logger
}

func NewSyncer(cache *Cache, client Poller, longPollTimeout time.Duration, logger *slog.Logger) *Syncer {
	return &Syncer{cache: cache, client: client, longPollTimeout: longPollTimeout, initialBackoff: time.Second, maximumBackoff: time.Minute, logger: logger}
}

func (s *Syncer) Bootstrap(ctx context.Context) {
	pairs, etag, _, err := s.client.Poll(ctx, nil, "", 0)
	if err != nil {
		s.logger.WarnContext(ctx, "approval cache bootstrap failed", "error", err)
		return
	}
	s.cache.Replace(pairs, etag)
}

func (s *Syncer) Run(ctx context.Context) {
	backoff := s.initialBackoff
	evictEvery := s.cache.idleTTL / 2
	if evictEvery <= 0 {
		evictEvery = time.Second
	}
	ticker := time.NewTicker(evictEvery)
	defer ticker.Stop()
	for {
		pollCtx, cancel := context.WithTimeout(ctx, s.longPollTimeout+5*time.Second)
		pairs, etag, unchanged, err := s.client.Poll(pollCtx, s.cache.ActiveSessions(), s.cache.ETag(), s.longPollTimeout)
		cancel()
		if err == nil {
			if !unchanged {
				s.cache.Replace(pairs, etag)
			}
			backoff = time.Second
		} else {
			s.logger.WarnContext(ctx, "approval cache sync failed", "error", err, "retry_after", backoff)
			timer := time.NewTimer(backoff)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-ticker.C:
				s.cache.EvictIdle()
			case <-timer.C:
			}
			backoff = min(backoff*2, s.maximumBackoff)
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.cache.EvictIdle()
		default:
		}
	}
}
