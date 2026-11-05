package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/telemetry"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var newBusinessEventDeliveryProvider = telemetry.NewBusinessEventProvider
var errBusinessEventDelivery = errors.New("business event storage or exporter operation failed")

func runBusinessEventDelivery(ctx context.Context, delivery ports.BusinessEventDeliveryRepository, cfg *ports.Config, ticks <-chan time.Time, logger *slog.Logger) (workerErr error) {
	var provider *telemetry.BusinessEventProvider
	var nextInitialization time.Time
	now := time.Now()
	defer func() {
		if provider != nil {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.Telemetry.Exporter.Timeout)
			defer cancel()
			workerErr = errors.Join(workerErr, provider.Shutdown(shutdownCtx))
		}
	}()
	for {
		if ctx.Err() != nil {
			return nil
		}
		if cfg.Telemetry.Enabled && cfg.Telemetry.Logs.Enabled && cfg.BusinessEvents.TelemetryCopyEnabled {
			if provider == nil && !now.Before(nextInitialization) {
				initCtx, cancel := context.WithTimeout(ctx, cfg.Telemetry.Exporter.Timeout)
				initialized, err := newBusinessEventDeliveryProvider(initCtx, cfg.Telemetry)
				cancel()
				if err != nil {
					nextInitialization = time.Now().Add(30 * time.Second)
					logger.WarnContext(ctx, "Business event telemetry initialization failed", "error", err)
				} else {
					provider = initialized
				}
			}
			if provider != nil {
				readCtx, cancel := context.WithTimeout(ctx, cfg.Storage.Timeouts.Read)
				keys, err := delivery.ListDue(readCtx, 100)
				cancel()
				if err != nil {
					logger.WarnContext(ctx, "Business event delivery scan failed", "error", businessEventDeliveryError(err))
				} else {
					for _, key := range keys {
						if ctx.Err() != nil {
							return nil
						}
						writeCtx, cancel := context.WithTimeout(ctx, cfg.Storage.Timeouts.Write)
						_, err := delivery.DispatchOne(writeCtx, key, func(exportCtx context.Context, event *model.BusinessEvent) error {
							attemptCtx, stopAttempt := context.WithTimeout(exportCtx, cfg.Telemetry.Exporter.Timeout)
							defer stopAttempt()
							return provider.Emit(attemptCtx, event)
						})
						cancel()
						if err != nil && ctx.Err() == nil {
							logger.WarnContext(ctx, "Business event telemetry delivery failed", "error", businessEventDeliveryError(err))
						}
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case tick, open := <-ticks:
			if !open {
				return nil
			}
			now = tick
		}
	}
}

func businessEventDeliveryError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, context.Canceled):
		return context.Canceled
	default:
		return errBusinessEventDelivery
	}
}

func startBusinessEventDelivery(delivery ports.BusinessEventDeliveryRepository, cfg *ports.Config, logger *slog.Logger) func(context.Context) error {
	ctx, cancel := context.WithCancel(context.Background())
	finished := make(chan struct{})
	var workerErr error
	ticker := time.NewTicker(time.Second)
	go func() {
		defer close(finished)
		defer ticker.Stop()
		workerErr = runBusinessEventDelivery(ctx, delivery, cfg, ticker.C, logger)
	}()
	return func(shutdownCtx context.Context) error {
		cancel()
		select {
		case <-finished:
			return workerErr
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}
}
