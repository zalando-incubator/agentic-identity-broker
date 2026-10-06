package oauth2server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/require"
)

func TestSessionCleanupAudit_LifetimeExpiryEmitsOnlyCommittedMaintenanceTransitions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason storage.RefreshRevocationReason
	}{
		{"absolute", storage.RefreshReasonAbsoluteExpiry},
		{"inactivity", storage.RefreshReasonInactivityExpiry},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newCleanupUnrotatedFixture(t)
			now := f.root.StartedAt.Add(2 * time.Second)
			var logs bytes.Buffer
			cleanup := newCleanupWorker(f, &now, slog.New(slog.NewJSONHandler(&logs, nil)))
			if tc.reason == storage.RefreshReasonAbsoluteExpiry {
				cleanup.refresh.Policy.AbsoluteLifetime = time.Second
			} else {
				cleanup.refresh.Policy.InactivityLifetime = time.Second
			}
			ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
				ClientIP: "192.0.2.20", UserAgent: "should-not-be-attributed-to-maintenance",
				RequestTarget: "/oauth2/token?refresh_token=do-not-log",
			})
			require.NoError(t, cleanup.Reconcile(ctx))
			events := refreshTransitionEvents(t, &logs, "RefreshSessionExpired")
			require.Len(t, events, 1)
			event := events[0]
			require.Equal(t, f.root.ID.String(), event["session_id"])
			require.Equal(t, f.root.Principal.String(), event["principal"])
			require.Equal(t, f.root.AgentID.String(), event["agent_id"])
			require.Equal(t, f.root.ClientID.String(), event["client_id"])
			require.Equal(t, string(tc.reason), event["reason"])
			require.Equal(t, "maintenance", event["origin"])
			require.NotContains(t, event, "client_ip")
			require.NotContains(t, event, "user_agent")
			require.NotContains(t, logs.String(), "do-not-log")
			after := cleanupRoot(t, f, f.root.ID)
			require.Equal(t, tc.reason, *after.TerminalReason)
			require.Equal(t, &now, after.ExpiredAt)
			logs.Reset()
			require.NoError(t, cleanup.Reconcile(context.Background()))
			require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionExpired"), "idempotent maintenance does not reannounce a transition")
		})
	}
}

type maintenanceAuditFaultRevocations struct {
	ports.RefreshSessionRevocationRepository
	failure error
}

func (r maintenanceAuditFaultRevocations) RevokeByID(ctx context.Context, sessionID id.RefreshSessionID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := r.RefreshSessionRevocationRepository.RevokeByID(ctx, sessionID, at, reason); err != nil {
		return err
	}
	return r.failure
}

func TestSessionCleanupAudit_FailedExpiryDoesNotClaimTransition(t *testing.T) {
	f := newCleanupUnrotatedFixture(t)
	now := f.root.StartedAt.Add(2 * time.Second)
	var logs bytes.Buffer
	cleanup := newCleanupWorker(f, &now, slog.New(slog.NewJSONHandler(&logs, nil)))
	cleanup.refresh.Policy.AbsoluteLifetime = time.Second
	cleanup.refresh.Revocations = maintenanceAuditFaultRevocations{RefreshSessionRevocationRepository: cleanup.refresh.Revocations,
		failure: errors.New("confirmed receipt/lineage rollback")}
	require.Error(t, cleanup.Reconcile(context.Background()))
	require.Empty(t, refreshTransitionEvents(t, &logs, "RefreshSessionExpired"))
	root := cleanupRoot(t, f, f.root.ID)
	require.Nil(t, root.TerminalReason)
	require.Nil(t, root.ExpiredAt)
	require.Error(t, cleanup.HealthCheck(context.Background()))
}

func TestSessionCleanupAudit_RestoreInvalidationNamesMaintenanceOrigin(t *testing.T) {
	f := newCleanupUnrotatedFixture(t)
	now := f.root.StartedAt.Add(time.Second)
	var logs bytes.Buffer
	cleanup := newCleanupWorker(f, &now, slog.New(slog.NewJSONHandler(&logs, nil)))
	require.NoError(t, cleanup.InvalidateRestored(context.Background()))
	events := refreshTransitionEvents(t, &logs, "RefreshSessionRevoked")
	require.Len(t, events, 1)
	require.Equal(t, f.root.ID.String(), events[0]["session_id"])
	require.Equal(t, "maintenance", events[0]["origin"])
	require.Equal(t, string(storage.RefreshReasonRestoreInvalidation), events[0]["reason"])
}
