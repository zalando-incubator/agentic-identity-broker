package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/require"
)

func refreshModelSession() *RefreshSession {
	started := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	sessionID := id.RefreshSessionID(id.NewAuthorizationCodeID())
	agentID := id.NewAgentID()
	return &RefreshSession{
		ID: sessionID, OriginalGrantID: id.NewGrantID(), OriginalTokenSignature: strings.Repeat("a", 64),
		AgentID: agentID, Principal: id.Principal("refresh-model@example.test"), ClientID: id.ClientID(agentID.String()),
		Scope: "offline_access read write", StartedAt: started, LastFreshAt: started,
		InactivityExpiresAt: started.Add(720 * time.Hour), RetainUntil: started.Add(720 * time.Hour),
		BranchKeyID: "refresh_" + sessionID.String() + "_branch_key", CurrentSignature: strings.Repeat("a", 64),
	}
}

func refreshModelPrevious() *RefreshSession {
	session := refreshModelSession()
	used := session.StartedAt.Add(time.Hour)
	session.LastFreshAt = used
	session.InactivityExpiresAt = used.Add(720 * time.Hour)
	session.RetainUntil = session.InactivityExpiresAt
	session.CurrentSignature = strings.Repeat("b", 64)
	session.PreviousSignature = ptr.To(session.OriginalTokenSignature)
	session.PreviousConsumedAt = &used
	session.ReuseUntil = ptr.To(used.Add(30 * time.Second))
	session.OriginalRequestedScope = ptr.To("")
	session.OriginalRequestContextFingerprint = ptr.To(strings.Repeat("c", 64))
	session.RetryAccessExpiresAt = ptr.To(used.Add(time.Hour))
	session.RetryExpiresAt = ptr.To(*session.ReuseUntil)
	session.RetryCiphertext = []byte{1, 2, 3}
	return session
}

func TestRefreshSessionRejectsInvalidAuthority(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RefreshSession)
	}{
		{"zero root", func(s *RefreshSession) { s.ID = id.RefreshSessionID{} }},
		{"zero origin grant", func(s *RefreshSession) { s.OriginalGrantID = id.GrantID{} }},
		{"zero agent", func(s *RefreshSession) { s.AgentID = id.AgentID{} }},
		{"empty principal", func(s *RefreshSession) { s.Principal = "" }},
		{"empty client", func(s *RefreshSession) { s.ClientID = "" }},
		{"invalid first signature", func(s *RefreshSession) { s.OriginalTokenSignature = strings.Repeat("G", 64) }},
		{"short current signature", func(s *RefreshSession) { s.CurrentSignature = "abc" }},
		{"wrong branch namespace", func(s *RefreshSession) { s.BranchKeyID = "service_branch_key" }},
		{"unsorted ceiling", func(s *RefreshSession) { s.Scope = "write read" }},
		{"duplicate ceiling", func(s *RefreshSession) { s.Scope = "read read" }},
		{"noncanonical ceiling", func(s *RefreshSession) { s.Scope = "read  write" }},
		{"missing origin time", func(s *RefreshSession) { s.StartedAt = time.Time{} }},
		{"backwards activity", func(s *RefreshSession) { s.LastFreshAt = s.StartedAt.Add(-time.Second) }},
		{"inactivity at issuance", func(s *RefreshSession) { s.InactivityExpiresAt = s.LastFreshAt }},
		{"retention before origin", func(s *RefreshSession) { s.RetainUntil = s.StartedAt }},
		{"negative retries", func(s *RefreshSession) { s.RetryCount = -1 }},
		{"four retries", func(s *RefreshSession) { s.RetryCount = 4 }},
		{"ciphertext without predecessor", func(s *RefreshSession) { s.RetryCiphertext = []byte{1} }},
		{"partial predecessor", func(s *RefreshSession) { s.PreviousSignature = ptr.To(strings.Repeat("b", 64)) }},
		{"reason without terminal state", func(s *RefreshSession) { s.TerminalReason = ptr.To(RefreshReasonGrantDeleted) }},
		{"terminal without reason", func(s *RefreshSession) { s.RevokedAt = ptr.To(s.StartedAt.Add(time.Hour)) }},
		{"two terminal states", func(s *RefreshSession) {
			s.RevokedAt = ptr.To(s.StartedAt.Add(time.Hour))
			s.ExpiredAt = ptr.To(*s.RevokedAt)
			s.TerminalReason = ptr.To(RefreshReasonGrantDeleted)
		}},
		{"credential replacement reason", func(s *RefreshSession) {
			s.RevokedAt = ptr.To(s.StartedAt.Add(time.Hour))
			s.TerminalReason = ptr.To(RefreshRevocationReason("credential_replaced"))
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			session := refreshModelSession()
			test.change(session)
			require.Error(t, session.Validate())
		})
	}
}

func TestRefreshSessionPredecessorBindings(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RefreshSession)
	}{
		{"current equals previous", func(s *RefreshSession) { s.PreviousSignature = ptr.To(s.CurrentSignature) }},
		{"missing original request", func(s *RefreshSession) { s.OriginalRequestedScope = nil }},
		{"missing fingerprint", func(s *RefreshSession) { s.OriginalRequestContextFingerprint = nil }},
		{"invalid fingerprint", func(s *RefreshSession) { s.OriginalRequestContextFingerprint = ptr.To("raw-user-agent") }},
		{"missing retry deadline", func(s *RefreshSession) { s.RetryExpiresAt = nil }},
		{"retry exceeds original bound", func(s *RefreshSession) { s.RetryExpiresAt = ptr.To(s.ReuseUntil.Add(time.Second)) }},
		{"consumption after fresh activity", func(s *RefreshSession) { s.PreviousConsumedAt = ptr.To(s.LastFreshAt.Add(time.Second)) }},
		{"reuse precedes consumption", func(s *RefreshSession) { s.ReuseUntil = ptr.To(s.PreviousConsumedAt.Add(-time.Second)) }},
		{"terminal retains ciphertext", func(s *RefreshSession) {
			s.RevokedAt = ptr.To(s.LastFreshAt.Add(time.Second))
			s.TerminalReason = ptr.To(RefreshReasonRestoreInvalidation)
		}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			session := refreshModelPrevious()
			test.change(session)
			require.Error(t, session.Validate())
		})
	}
}

func TestRefreshSessionRejectsAuthorityRewrites(t *testing.T) {
	cases := []struct {
		name   string
		change func(*RefreshSession)
	}{
		{"root identity", func(s *RefreshSession) { s.ID = id.NewRefreshSessionID() }},
		{"origin grant", func(s *RefreshSession) { s.OriginalGrantID = id.NewGrantID() }},
		{"first token", func(s *RefreshSession) { s.OriginalTokenSignature = strings.Repeat("d", 64) }},
		{"agent", func(s *RefreshSession) { s.AgentID = id.NewAgentID() }},
		{"principal", func(s *RefreshSession) { s.Principal = "other@example.test" }},
		{"client", func(s *RefreshSession) { s.ClientID = "another-client" }},
		{"ceiling", func(s *RefreshSession) { s.Scope = "offline_access read" }},
		{"origin time", func(s *RefreshSession) { s.StartedAt = s.StartedAt.Add(time.Second) }},
		{"branch identity", func(s *RefreshSession) { s.BranchKeyID = "another_branch_key" }},
		{"original profile", func(s *RefreshSession) { s.DisplayName = "replacement profile" }},
		{"retention decrease", func(s *RefreshSession) { s.RetainUntil = s.RetainUntil.Add(-time.Second) }},
		{"sliding retry deadline", func(s *RefreshSession) { s.RetryExpiresAt = ptr.To(s.RetryExpiresAt.Add(time.Second)) }},
		{"cleared retry deadline", func(s *RefreshSession) { s.RetryExpiresAt = nil }},
		{"sliding inactivity without fresh use", func(s *RefreshSession) { s.InactivityExpiresAt = s.InactivityExpiresAt.Add(time.Hour) }},
		{"rewritten consumption time", func(s *RefreshSession) { s.PreviousConsumedAt = ptr.To(s.PreviousConsumedAt.Add(time.Second)) }},
		{"rewritten original request", func(s *RefreshSession) { s.OriginalRequestedScope = ptr.To("read") }},
		{"counter reset on same predecessor", func(s *RefreshSession) { s.RetryCount = 0 }},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			previous := refreshModelPrevious()
			previous.RetryCount = 1
			next := *previous
			test.change(&next)
			require.Error(t, next.ValidateTransition(previous, previous.LastFreshAt.Add(time.Second)))
		})
	}
}

func TestRefreshSessionTerminalAuthorityCannotResume(t *testing.T) {
	for _, reason := range []RefreshRevocationReason{RefreshReasonGrantDeleted, RefreshReasonRestoreInvalidation, RefreshReasonAbsoluteExpiry, RefreshReasonInactivityExpiry} {
		t.Run(string(reason), func(t *testing.T) {
			previous := refreshModelPrevious()
			previous.RetryCiphertext = nil
			previous.TerminalReason = &reason
			at := previous.LastFreshAt.Add(time.Second)
			if reason == RefreshReasonAbsoluteExpiry || reason == RefreshReasonInactivityExpiry {
				previous.ExpiredAt = &at
			} else {
				previous.RevokedAt = &at
			}
			next := *previous
			next.TerminalReason, next.RevokedAt, next.ExpiredAt = nil, nil, nil
			require.Error(t, next.ValidateTransition(previous, at.Add(time.Second)))
		})
	}
}

func TestRefreshSessionElapsedDeadlineCannotBeExtended(t *testing.T) {
	previous := refreshModelSession()
	at := previous.StartedAt.Add(time.Hour)
	previous.AbsoluteExpiresAt = &at
	next := *previous
	next.AbsoluteExpiresAt = ptr.To(at.Add(time.Hour))
	require.Error(t, next.ValidateTransition(previous, at))
}
