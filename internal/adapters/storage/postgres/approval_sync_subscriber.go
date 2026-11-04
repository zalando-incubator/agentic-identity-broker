package postgres

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
)

const (
	approvalSyncChannel    = "approval_sync"
	reconnectBaseDelay     = 1 * time.Second
	reconnectMaxDelay      = 30 * time.Second
	reconnectBackoffFactor = 2
)

// ApprovalSyncSubscriber listens for PostgreSQL NOTIFY events on the approval_sync
// channel and wakes the ApprovalSyncBroadcaster when notifications arrive.
// It runs as a long-lived goroutine per broker instance.
type ApprovalSyncSubscriber struct {
	connString  string
	broadcaster *approval.ApprovalSyncBroadcaster
	logger      *slog.Logger
}

// NewApprovalSyncSubscriber creates a new subscriber that will connect to the given
// PostgreSQL DSN and forward notifications to the broadcaster.
func NewApprovalSyncSubscriber(connString string, broadcaster *approval.ApprovalSyncBroadcaster, logger *slog.Logger) *ApprovalSyncSubscriber {
	return &ApprovalSyncSubscriber{
		connString:  connString,
		broadcaster: broadcaster,
		logger:      logger,
	}
}

// Listen blocks and listens for NOTIFY events until the context is cancelled.
// It automatically reconnects with exponential backoff on connection failures.
func (s *ApprovalSyncSubscriber) Listen(ctx context.Context) error {
	delay := reconnectBaseDelay

	for {
		err := s.listenOnce(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}

		s.logger.WarnContext(ctx, "approval sync listen connection lost, reconnecting",
			"error", err,
			"retry_delay", delay,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}

		delay = delay * time.Duration(reconnectBackoffFactor)
		if delay > reconnectMaxDelay {
			delay = reconnectMaxDelay
		}
	}
}

// listenOnce establishes a single LISTEN connection and processes notifications
// until the connection fails or context is cancelled.
func (s *ApprovalSyncSubscriber) listenOnce(ctx context.Context) error {
	conn, err := pgx.Connect(ctx, s.connString)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close(ctx) }()

	_, err = conn.Exec(ctx, "LISTEN "+approvalSyncChannel)
	if err != nil {
		return fmt.Errorf("LISTEN: %w", err)
	}

	s.logger.InfoContext(ctx, "approval sync listen connection established",
		"channel", approvalSyncChannel,
	)

	for {
		_, err := conn.WaitForNotification(ctx)
		if err != nil {
			return fmt.Errorf("wait for notification: %w", err)
		}
		s.broadcaster.Broadcast()
	}
}
