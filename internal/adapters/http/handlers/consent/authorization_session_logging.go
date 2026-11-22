package consent

import (
	"context"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type authorizationSessionReason string

const (
	reasonInvalidAuthorizationSession authorizationSessionReason = "invalid_authorization_session"
	reasonExpiredAuthorizationSession authorizationSessionReason = "expired_authorization_session"
	reasonAgentMismatch               authorizationSessionReason = "agent_mismatch"
	reasonPrincipalMismatch           authorizationSessionReason = "principal_mismatch"
)

// logAuthorizationSessionRejected writes the consent.authorization_session.rejected security
// record. It never logs the session token or the validation cause.
func logAuthorizationSessionRejected(ctx context.Context, logger *slog.Logger, agentID id.AgentID, principal id.Principal, reason authorizationSessionReason) {
	logger.WarnContext(ctx, "authorization session rejected", "event", "consent.authorization_session.rejected",
		"agent_id", agentID, "principal", principal, "reason", reason)
}
