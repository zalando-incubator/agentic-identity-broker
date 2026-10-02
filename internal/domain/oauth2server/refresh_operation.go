package oauth2server

import (
	"context"
	"time"

	"github.com/ory/fosite"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// refreshOperation marks an agent-scoped owner transaction. Fosite borrows this
// scope; only the enclosing coordinator is allowed to commit or roll it back.
type refreshOperation struct {
	at                 time.Time
	client             fosite.Client
	root               *storage.RefreshSession
	accessExpiresAt    time.Time
	accessToken        string
	refreshToken       string
	code               *storage.AuthorizationCode
	presentedSignature string
	requestedScope     string
}

type refreshOperationKey struct{}

func withRefreshOperation(ctx context.Context, at time.Time, client fosite.Client, root *storage.RefreshSession) context.Context {
	return context.WithValue(ctx, refreshOperationKey{}, &refreshOperation{at: at, client: client, root: root})
}

func refreshOperationFromContext(ctx context.Context) (*refreshOperation, bool) {
	op, ok := ctx.Value(refreshOperationKey{}).(*refreshOperation)
	return op, ok && op != nil && !op.at.IsZero() && op.client != nil
}

// recordRefreshAccessExpiry captures the access-token strategy's actual signed
// expiry, not Fosite's independently calculated session expiry.
func recordRefreshAccessExpiry(ctx context.Context, expiry time.Time) {
	if op, ok := refreshOperationFromContext(ctx); ok {
		op.accessExpiresAt = expiry.UTC()
	}
}
