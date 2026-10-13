package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func runMemoryBusinessEventRetention(ctx context.Context, lifecycle ports.BusinessEventLifecycleRepository, ticks <-chan time.Time, started chan<- error) error {
	err := lifecycle.ApplyRetention(ctx)
	started <- err
	if ctx.Err() != nil {
		return nil
	}
	if err != nil {
		return err
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case _, open := <-ticks:
			if !open {
				return nil
			}
			if err := lifecycle.ApplyRetention(ctx); err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return err
			}
		}
	}
}

func startMemoryBusinessEventRetention(lifecycle ports.BusinessEventLifecycleRepository, timeout time.Duration, logger *slog.Logger) (func(context.Context) error, error) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan error, 1)
	finished := make(chan struct{})
	var workerErr error
	ticker := time.NewTicker(5 * time.Minute)
	go func() {
		defer close(finished)
		defer ticker.Stop()
		workerErr = runMemoryBusinessEventRetention(ctx, lifecycle, ticker.C, started)
		if workerErr != nil {
			logger.Warn("Business event retention worker stopped")
		}
	}()
	startupCtx, cancelStartup := context.WithTimeout(context.Background(), timeout)
	var err error
	select {
	case err = <-started:
	case <-startupCtx.Done():
		err = startupCtx.Err()
	}
	cancelStartup()
	if err != nil {
		cancel()
		<-finished
		return nil, err
	}
	return func(shutdownCtx context.Context) error {
		cancel()
		select {
		case <-finished:
			return workerErr
		case <-shutdownCtx.Done():
			return shutdownCtx.Err()
		}
	}, nil
}
