package ledgerfixture

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type GrantExpiryMarkers struct{ recorded map[id.GrantID]time.Time }

func (*GrantExpiryMarkers) ListUnrecordedExpired(context.Context, time.Time, int) ([]*storage.UserGrant, error) {
	panic("unexpected global expiration query in grant-access mock")
}

func (*GrantExpiryMarkers) ListUnrecordedExpiredForPrincipal(context.Context, id.Principal, time.Time, int) ([]*storage.UserGrant, error) {
	panic("unexpected principal expiration query in grant-access mock")
}

func (m *GrantExpiryMarkers) RecordExpiration(_ context.Context, grantID id.GrantID, expiry time.Time) (bool, error) {
	if expiry.After(time.Now()) || m.recorded[grantID].Equal(expiry) {
		return false, nil
	}
	if m.recorded == nil {
		m.recorded = make(map[id.GrantID]time.Time)
	}
	m.recorded[grantID] = expiry
	return true, nil
}
