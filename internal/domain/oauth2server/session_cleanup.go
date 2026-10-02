package oauth2server

import (
	"context"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const cleanupBatchSize = 200

// SessionCleanup reconciles persisted retry deadlines and erases expired results.
type SessionCleanup struct {
	repositories [2]sessionCleanupRepository
	logger       *slog.Logger
	refresh      RefreshSessionDependencies
	maintenance  ports.RefreshSessionMaintenanceRepository
	ready        atomic.Bool
}

type sessionCleanupRepository struct {
	name          string
	deleteExpired func(context.Context) (int, error)
}

func NewSessionCleanup(
	codes ports.AuthorizationCodeRepository,
	pkce ports.PKCESessionRepository,
	refresh RefreshSessionDependencies,
	maintenance ports.RefreshSessionMaintenanceRepository,
	logger *slog.Logger,
) *SessionCleanup {
	if codes == nil || pkce == nil || refresh.Sessions == nil || refresh.Tokens == nil || refresh.Revocations == nil || refresh.Coordinator == nil || refresh.Clock == nil || maintenance == nil || logger == nil {
		panic("oauth2server.NewSessionCleanup: native maintenance dependencies are required")
	}
	return &SessionCleanup{
		repositories: [2]sessionCleanupRepository{
			{name: "authorization_codes", deleteExpired: codes.DeleteExpired},
			{name: "pkce_sessions", deleteExpired: pkce.DeleteExpired},
		},
		logger: logger, refresh: refresh, maintenance: maintenance,
	}
}

func (s *SessionCleanup) HealthCheck(context.Context) error {
	if !s.ready.Load() {
		return errors.New("refresh session maintenance is not ready")
	}
	return nil
}

// Reconcile scans active roots in bounded pages. Each write re-reads the root
// under its owner's gate and must commit before maintenance becomes ready.
func (s *SessionCleanup) Reconcile(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		s.ready.Store(false)
		return err
	}
	policy := s.refresh.Policy
	if policy.ReuseInterval < 0 || policy.InactivityLifetime <= 0 || policy.AbsoluteLifetime < 0 {
		return s.fail(ctx, "refresh_sessions", "invalid policy")
	}
	now, err := s.refresh.Clock.Now(ctx)
	if err != nil {
		return s.fail(ctx, "refresh_sessions", "clock unavailable")
	}
	if _, err := s.maintenance.ListDue(ctx, now, cleanupBatchSize); err != nil {
		return s.fail(ctx, "refresh_sessions", "listing unavailable")
	}
	var after id.RefreshSessionID
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		roots, err := s.maintenance.ListActive(ctx, after, cleanupBatchSize)
		if err != nil {
			return s.fail(ctx, "refresh_sessions", "listing unavailable")
		}
		for _, root := range roots {
			if root == nil || (root.PreviousSignature == nil && len(root.RetryCiphertext) != 0) {
				return s.fail(ctx, "refresh_sessions", "invalid listing")
			}
			if err := s.reconcileRoot(ctx, root.ID, root.AgentID); err != nil {
				return err
			}
			after = root.ID
		}
		if len(roots) < cleanupBatchSize {
			break
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			s.ready.Store(false)
			return err
		}
		before, err := s.refresh.Clock.Now(ctx)
		if err != nil {
			return s.fail(ctx, "refresh_sessions", "clock unavailable")
		}
		deleted, err := s.maintenance.DeleteTerminal(ctx, before, cleanupBatchSize)
		if err != nil {
			return s.fail(ctx, "refresh_sessions", "retention unavailable")
		}
		if deleted < cleanupBatchSize {
			break
		}
	}
	for _, repository := range s.repositories {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := repository.deleteExpired(ctx); err != nil {
			return s.fail(ctx, repository.name, "deletion unavailable")
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	s.ready.Store(true)
	return nil
}

// InvalidateRestored ends all snapshot-local refresh authority before token writers resume.
// Callers must keep every broker offline until this operation succeeds.
func (s *SessionCleanup) InvalidateRestored(ctx context.Context) error {
	s.ready.Store(false)
	ctx = security.WithMaintenanceAuditOrigin(ctx)
	var after id.AgentID
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		agents, err := s.maintenance.ListAgentIDs(ctx, after, cleanupBatchSize)
		if err != nil {
			return s.fail(ctx, "refresh_sessions", "restoration listing unavailable")
		}
		for _, agentID := range agents {
			if agentID.IsZero() || (!after.IsZero() && agentID.String() <= after.String()) {
				return s.fail(ctx, "refresh_sessions", "invalid restoration listing")
			}
			var revoked []storage.RefreshSessionAuditIdentity
			if err := s.refresh.Coordinator.Run(ctx, agentID, func(owner context.Context, at time.Time) error {
				var err error
				revoked, err = s.refresh.Revocations.ListActiveByAgent(owner, agentID, nil)
				if err != nil {
					return err
				}
				return s.refresh.Revocations.RevokeByAgent(owner, agentID, at, storage.RefreshReasonRestoreInvalidation)
			}); err != nil {
				return s.fail(ctx, "refresh_sessions", "restoration commit unavailable")
			}
			for _, root := range revoked {
				logRefreshTransition(s.logger, ctx, root, storage.RefreshReasonRestoreInvalidation)
			}
			after = agentID
		}
		if len(agents) < cleanupBatchSize {
			break
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	remaining, err := s.maintenance.HasRemainingAuthority(ctx)
	if err != nil {
		return s.fail(ctx, "refresh_sessions", "restoration check unavailable")
	}
	if remaining {
		return s.fail(ctx, "refresh_sessions", "restored authority remains")
	}
	return nil
}

func (s *SessionCleanup) reconcileRoot(ctx context.Context, sessionID id.RefreshSessionID, agentID id.AgentID) error {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	ctx = security.WithMaintenanceAuditOrigin(ctx)
	var expiredRoot storage.RefreshSessionAuditIdentity
	var expiryReason storage.RefreshRevocationReason
	defer cancel()
	err := s.refresh.Coordinator.Run(ctx, agentID, func(owner context.Context, _ time.Time) error {
		root, err := s.refresh.Sessions.FindByID(owner, sessionID)
		if ports.IsNotFoundErr(err) {
			return nil // Removed or terminalized after the listing.
		}
		if err != nil {
			return err
		}
		if root == nil {
			return errors.New("refresh session missing")
		}
		if root.TerminalReason != nil {
			return nil
		}
		if root.PreviousSignature != nil && (root.PreviousConsumedAt == nil || root.ReuseUntil == nil || root.RetryExpiresAt == nil || root.RetryAccessExpiresAt == nil) {
			return errors.New("refresh session lacks predecessor deadlines")
		}
		now, err := s.refresh.Clock.Now(owner)
		if err != nil {
			return err
		}
		current, err := s.refresh.Tokens.FindBySignature(owner, root.CurrentSignature)
		if err != nil {
			return err
		}
		if current == nil || current.SessionID != root.ID || current.ExpiresAt.IsZero() {
			return errors.New("refresh session current token is invalid")
		}
		policy := s.refresh.Policy
		inactivity := earliestRefreshDeadline(root.InactivityExpiresAt, root.LastFreshAt.Add(policy.InactivityLifetime), current.ExpiresAt)
		var absolute *time.Time
		if root.AbsoluteExpiresAt != nil && !now.Before(*root.AbsoluteExpiresAt) {
			// A stored deadline that already elapsed cannot be lifted by a policy change.
			absolute = root.AbsoluteExpiresAt
		} else if policy.AbsoluteLifetime > 0 {
			candidate := root.StartedAt.Add(policy.AbsoluteLifetime)
			absolute = &candidate
		}
		absoluteElapsed := absolute != nil && !now.Before(*absolute)
		if absoluteElapsed || !now.Before(inactivity) {
			s.ready.Store(false)
			reason := storage.RefreshReasonInactivityExpiry
			if absoluteElapsed && !absolute.After(inactivity) {
				reason = storage.RefreshReasonAbsoluteExpiry
			}
			if err := s.refresh.Revocations.RevokeByID(owner, root.ID, now, reason); err != nil {
				return err
			}
			expiredRoot, expiryReason = root.AuditIdentity(), reason
			root, err = s.refresh.Sessions.FindByID(owner, root.ID)
			if err != nil {
				return err
			}
		}
		changed := false
		if inactivity.Before(root.InactivityExpiresAt) {
			root.InactivityExpiresAt = inactivity
			changed = true
		}
		if (root.AbsoluteExpiresAt == nil) != (absolute == nil) ||
			(root.AbsoluteExpiresAt != nil && absolute != nil && !root.AbsoluteExpiresAt.Equal(*absolute)) {
			root.AbsoluteExpiresAt = absolute
			changed = true
		}
		if root.PreviousSignature != nil {
			deadline := earliestRefreshDeadline(*root.RetryExpiresAt, root.PreviousConsumedAt.Add(policy.ReuseInterval), *root.ReuseUntil, *root.RetryAccessExpiresAt, inactivity)
			if absolute != nil {
				deadline = earliestRefreshDeadline(deadline, *absolute)
			}
			if deadline.Before(*root.RetryExpiresAt) {
				root.RetryExpiresAt = &deadline
				changed = true
			}
			if len(root.RetryCiphertext) != 0 && !now.Before(deadline) {
				s.ready.Store(false)
				root.RetryCiphertext = nil
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return s.refresh.Sessions.Save(owner, root)
	})
	if err != nil {
		return s.fail(ctx, "refresh_sessions", "reconciliation unavailable")
	}
	if !expiredRoot.ID.IsZero() {
		logRefreshTransition(s.logger, ctx, expiredRoot, expiryReason)
	}
	return nil
}

// Run checks nearby deadlines from the shared clock. The periodic fallback
// discovers results committed by another instance; it is not the erasure timer.
// Cancellation waits for any in-flight scoped write to finish.
func (s *SessionCleanup) Run(ctx context.Context) {
	nextFullScan := time.Now().Add(time.Minute)
	for {
		if ctx.Err() != nil {
			s.ready.Store(false)
			return
		}
		if !s.ready.Load() || !time.Now().Before(nextFullScan) {
			s.sweep(ctx)
			nextFullScan = time.Now().Add(time.Minute)
		}
		if ctx.Err() != nil {
			s.ready.Store(false)
			return
		}
		delay := s.dueDelay(ctx)
		if ctx.Err() != nil {
			s.ready.Store(false)
			return
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			s.ready.Store(false)
			return
		case <-timer.C:
		}
	}
}

func (s *SessionCleanup) dueDelay(ctx context.Context) time.Duration {
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	const horizon = 500 * time.Millisecond
	wasReady := s.ready.Load()
	now, err := s.refresh.Clock.Now(ctx)
	if err != nil {
		_ = s.fail(ctx, "refresh_sessions", "clock unavailable")
		return horizon
	}
	roots, err := s.maintenance.ListDue(ctx, now.Add(horizon), cleanupBatchSize)
	if err != nil {
		_ = s.fail(ctx, "refresh_sessions", "listing unavailable")
		return horizon
	}
	// ListDue is ordered by ID, not by deadline, and has no cursor. When
	// saturated (including by elapsed roots without ciphertext), only the
	// paginated active scan can prove that no live result was omitted.
	if len(roots) == cleanupBatchSize {
		_ = s.Reconcile(ctx)
		return horizon
	}
	delay := horizon
	for _, root := range roots {
		if root == nil {
			_ = s.fail(ctx, "refresh_sessions", "invalid listing")
			return horizon
		}
		deadline := root.InactivityExpiresAt
		if root.AbsoluteExpiresAt != nil {
			deadline = earliestRefreshDeadline(deadline, *root.AbsoluteExpiresAt)
		}
		if len(root.RetryCiphertext) != 0 {
			if root.RetryExpiresAt == nil {
				_ = s.fail(ctx, "refresh_sessions", "invalid deadline")
				return horizon
			}
			deadline = earliestRefreshDeadline(deadline, *root.RetryExpiresAt)
		}
		if !now.Before(deadline) {
			s.ready.Store(false)
			if err := s.reconcileRoot(ctx, root.ID, root.AgentID); err != nil {
				return horizon
			}
		} else if until := deadline.Sub(now); until < delay {
			delay = until
		}
	}
	if wasReady {
		s.ready.Store(true)
	}
	return delay
}

func (s *SessionCleanup) sweep(ctx context.Context) {
	_ = s.Reconcile(ctx)
}

func (s *SessionCleanup) fail(ctx context.Context, repository, reason string) error {
	s.ready.Store(false)
	if ctx.Err() == nil {
		// Storage errors may contain credentials: report only bounded labels.
		s.logger.WarnContext(ctx, "OAuth2 session cleanup failed", "repository", repository, "reason", reason)
		return errors.New("OAuth2 session cleanup " + repository + ": " + reason)
	}
	return ctx.Err()
}
