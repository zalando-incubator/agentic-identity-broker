package postgres

import (
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/stretchr/testify/require"
)

func TestRefreshSessionRecordCanonicalizesUTCWithoutExtendingAuthority(t *testing.T) {
	at := time.Date(2026, 8, 1, 12, 0, 0, 0, time.FixedZone("database decoding", 2*60*60))
	deadline := at.Add(30 * time.Minute)
	rootID := id.NewRefreshSessionID()
	first := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	record := refreshSessionRecord{
		ID: rootID, OriginalGrantID: id.NewGrantID(), AgentID: id.NewAgentID(),
		Principal: id.NewPrincipal("utc-test@example.test"), ClientID: id.NewClientID("utc-test"),
		OriginalTokenSignature: first, CurrentSignature: first, Scope: "offline_access read",
		StartedAt: at, LastFreshAt: at, AbsoluteExpiresAt: &deadline,
		InactivityExpiresAt: at.Add(time.Hour), RetainUntil: at.Add(time.Hour),
		BranchKeyID: "refresh_" + rootID.String() + "_branch_key",
	}
	root := record.model()
	require.NoError(t, root.Validate(), "database zone representation must not invalidate persisted authority")
	require.True(t, root.StartedAt.Equal(at))
	require.True(t, root.AbsoluteExpiresAt.Equal(deadline))
	require.Equal(t, 30*time.Minute, root.AbsoluteExpiresAt.Sub(root.StartedAt))
	require.Equal(t, time.Hour, root.InactivityExpiresAt.Sub(root.StartedAt))
}
