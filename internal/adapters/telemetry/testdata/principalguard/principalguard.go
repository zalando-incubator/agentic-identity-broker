// Package principalguard is a fixture for TestPrincipalGuardDetectsUntypedPrincipals.
package principalguard

import (
	"context"
	"log/slog"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func logPrincipals(ctx context.Context, logger *slog.Logger, p id.Principal, raw string) {
	logger.InfoContext(ctx, "typed", "principal", p)
	logger.InfoContext(ctx, "typed attr", slog.Any("operator_principal", p))
	logger.With("actor", p).InfoContext(ctx, "typed with")

	logger.InfoContext(ctx, "slog string", slog.String("principal", p.String()))
	logger.InfoContext(ctx, "stringer", "principal", p.String())
	logger.InfoContext(ctx, "conversion", "principal", string(p))
	logger.InfoContext(ctx, "plain string", "principal", raw)
	operator := raw
	logger.InfoContext(ctx, "operator", "operator_principal", operator)
	actor := raw
	logger.With("actor", actor).InfoContext(ctx, "actor")
	logger.InfoContext(ctx, "any string", slog.Any("principal", raw))
	logger.InfoContext(ctx, "other key", "agent_id", raw)
}
