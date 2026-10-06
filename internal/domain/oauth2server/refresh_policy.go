package oauth2server

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"slices"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// initializeRefreshSession fixes the original issuance clock and finite lifetime.
// The issuer creates the first token in the same authorization transaction.
func initializeRefreshSession(root *storage.RefreshSession, policy storage.RefreshSessionPolicy, at time.Time) error {
	if root == nil || root.ID.IsZero() || at.IsZero() || at.Location() != time.UTC || policy.InactivityLifetime <= 0 || policy.AbsoluteLifetime < 0 || policy.ReuseInterval < 0 {
		return errors.New("refresh issuance requires a root, UTC decision time, and valid policy")
	}
	root.StartedAt = at
	root.LastFreshAt = at
	root.InactivityExpiresAt = at.Add(policy.InactivityLifetime)
	root.RetainUntil = root.InactivityExpiresAt
	root.AbsoluteExpiresAt = nil
	if policy.AbsoluteLifetime > 0 {
		absolute := at.Add(policy.AbsoluteLifetime)
		root.AbsoluteExpiresAt = &absolute
	}
	root.BranchKeyID = "refresh_" + root.ID.String() + "_branch_key"
	if !root.RetainUntil.After(at) || root.RetainUntil.Year() > 9999 {
		return errors.New("refresh issuance has no finite token lifetime")
	}
	return nil
}

func canonicalRequestedScope(scope string) string {
	parts := strings.Fields(scope)
	slices.Sort(parts)
	return strings.Join(slices.Compact(parts), " ")
}

func earliestRefreshDeadline(first time.Time, deadlines ...time.Time) time.Time {
	for _, deadline := range deadlines {
		if !deadline.IsZero() && deadline.Before(first) {
			first = deadline
		}
	}
	return first
}

// refreshRequestFingerprint omits request IDs, raw tokens and forwarded headers.
// The security context's client address has already passed the trusted-proxy
// boundary; an absent context produces the stable empty-pair fingerprint.
func refreshRequestFingerprint(ctx context.Context) string {
	var host, userAgent string
	if sc, ok := security.FromContext(ctx); ok {
		host = strings.TrimSpace(sc.ClientIP)
		if withoutPort, _, err := net.SplitHostPort(host); err == nil {
			host = withoutPort
		}
		host = strings.ToLower(strings.Trim(host, "[]"))
		userAgent = security.TruncateUserAgent(sc.UserAgent)
	}
	data := make([]byte, 0, 16+len(host)+len(userAgent))
	data = binary.BigEndian.AppendUint64(data, uint64(len(host)))
	data = append(data, host...)
	data = binary.BigEndian.AppendUint64(data, uint64(len(userAgent)))
	data = append(data, userAgent...)
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}
