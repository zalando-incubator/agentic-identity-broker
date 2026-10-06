package oauth2server

import (
	"bytes"
	"context"
	"slices"
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
	prepared           *preparedRefreshResult
}

type preparedRefreshResult struct {
	at              time.Time
	root            *storage.RefreshSession
	token           *storage.RefreshToken
	client          fosite.Client
	request         fosite.AccessRequester
	prohibited      bool
	accessToken     string
	refreshToken    string
	accessExpiresAt time.Time
	responseScope   string
	sealedRetry     []byte
	payload         *refreshRetryPayload
}

func sameSnapshotValue[T comparable](left, right *T) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && *left == *right)
}

func sameSnapshotTime(left, right *time.Time) bool {
	return (left == nil && right == nil) || (left != nil && right != nil && left.Equal(*right))
}

func sameRefreshSnapshot(before, after *storage.RefreshSession) bool {
	return before != nil && after != nil &&
		before.ID == after.ID && before.OriginalGrantID == after.OriginalGrantID &&
		before.OriginalTokenSignature == after.OriginalTokenSignature &&
		before.AgentID == after.AgentID && before.Principal == after.Principal && before.ClientID == after.ClientID &&
		before.Scope == after.Scope && sameSnapshotValue(before.Email, after.Email) && before.DisplayName == after.DisplayName &&
		before.StartedAt.Equal(after.StartedAt) && before.LastFreshAt.Equal(after.LastFreshAt) &&
		sameSnapshotTime(before.AbsoluteExpiresAt, after.AbsoluteExpiresAt) && before.InactivityExpiresAt.Equal(after.InactivityExpiresAt) &&
		before.RetainUntil.Equal(after.RetainUntil) && before.BranchKeyID == after.BranchKeyID &&
		before.CurrentSignature == after.CurrentSignature && sameSnapshotValue(before.PreviousSignature, after.PreviousSignature) &&
		sameSnapshotTime(before.PreviousConsumedAt, after.PreviousConsumedAt) && sameSnapshotTime(before.ReuseUntil, after.ReuseUntil) &&
		sameSnapshotValue(before.OriginalRequestedScope, after.OriginalRequestedScope) &&
		sameSnapshotValue(before.OriginalRequestContextFingerprint, after.OriginalRequestContextFingerprint) &&
		sameSnapshotTime(before.RetryAccessExpiresAt, after.RetryAccessExpiresAt) && sameSnapshotTime(before.RetryExpiresAt, after.RetryExpiresAt) &&
		before.RetryCount <= after.RetryCount && sameSnapshotTime(before.RevokedAt, after.RevokedAt) &&
		sameSnapshotTime(before.ExpiredAt, after.ExpiredAt) && sameSnapshotValue(before.TerminalReason, after.TerminalReason) &&
		bytes.Equal(before.RetryCiphertext, after.RetryCiphertext)
}

func sameRefreshTokenSnapshot(before, after *storage.RefreshToken) bool {
	return before != nil && after != nil && before.Signature == after.Signature && before.SessionID == after.SessionID &&
		before.IssuedAt.Equal(after.IssuedAt) && before.ExpiresAt.Equal(after.ExpiresAt) && sameSnapshotTime(before.UsedAt, after.UsedAt)
}

func samePreparedRefreshClient(before, after fosite.Client) bool {
	initial, current := before.(agentHolder).getAgent(), after.(agentHolder).getAgent()
	return initial != nil && current != nil && initial.ID == current.ID && initial.DisplayName == current.DisplayName &&
		sameSnapshotValue(initial.ClientID, current.ClientID) && before.GetID() == after.GetID() &&
		slices.Equal(before.GetScopes(), after.GetScopes())
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
