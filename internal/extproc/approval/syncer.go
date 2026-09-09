package approval

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"
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
	meter := otel.GetMeterProvider().Meter("extproc")
	syncAgeGauge, err := meter.Int64ObservableGauge("extproc.approval.sync_age",
		metric.WithUnit("s"),
		metric.WithDescription("Seconds since the last successful approval sync"))
	if err != nil {
		logger.Warn("failed to create approval sync-age gauge instrument", "error", err)
	} else if _, err := meter.RegisterCallback(func(_ context.Context, observer metric.Observer) error {
		observer.ObserveInt64(syncAgeGauge, cache.SyncAgeSeconds())
		return nil
	}, syncAgeGauge); err != nil {
		logger.Warn("failed to register approval sync-age gauge callback", "error", err)
	}

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
			s.cache.MarkSynced()
			backoff = time.Second
		} else {
			s.logger.WarnContext(ctx, "approval cache sync failed", "error", err, "retry_after", backoff)
			timer := time.NewTimer(backoff)
		waitForBackoff:
			for {
				select {
				case <-ctx.Done():
					if !timer.Stop() {
						select {
						case <-timer.C:
						default:
						}
					}
					return
				case <-ticker.C:
					s.cache.EvictIdle()
				case <-timer.C:
					break waitForBackoff
				}
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
