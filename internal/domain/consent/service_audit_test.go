package consent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/require"
)

func consentRevocationEvents(t *testing.T, logs *bytes.Buffer) []map[string]any {
	t.Helper()
	var events []map[string]any
	decoder := json.NewDecoder(logs)
	for decoder.More() {
		var record map[string]any
		require.NoError(t, decoder.Decode(&record))
		if record["event"] == "RefreshSessionRevoked" {
			events = append(events, record)
		}
	}
	return events
}

func TestConsentRevocationAudit_OnlyCommittedSessionsCarryTrustedContext(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(*consentRenewalFixture, context.Context) error
	}{
		{"idempotent revoke", func(f *consentRenewalFixture, ctx context.Context) error {
			return f.service.RevokeConsent(ctx, f.principal, f.agentID)
		}},
		{"principal DELETE", func(f *consentRenewalFixture, ctx context.Context) error {
			return f.service.RevokeConsentForPrincipal(ctx, f.principal, f.agentID)
		}},
		{"empty submission", func(f *consentRenewalFixture, ctx context.Context) error {
			_, err := f.service.GrantConsent(ctx, &GrantRequest{Principal: f.principal, AgentID: f.agentID})
			return err
		}},
		{"expired grant renewal", func(f *consentRenewalFixture, ctx context.Context) error {
			f.grant.ValidUntil = &f.decisionTime
			f.grants.grants[f.grant.ID].ValidUntil = &f.decisionTime
			_, err := f.service.GrantConsent(ctx, f.request)
			return err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			at := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
			f := newConsentRenewalFixture(at, at.Add(time.Hour))
			first := f.addRoot(t, f.principal, f.agentID, f.grant.ID, true)
			second := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
			unrelated := f.addRoot(t, "other@example.test", f.agentID, f.grant.ID, false)
			var logs bytes.Buffer
			f.service.logger = slog.New(slog.NewJSONHandler(&logs, nil))
			ua := strings.Repeat("a", security.MaxUserAgentByte+7)
			ctx := security.WithSecurityContext(context.Background(), security.SecurityContext{
				ClientIP: "203.0.113.44", UserAgent: ua,
				RequestTarget: "/api/consent?client_secret=do-not-log", TraceID: "trace-test",
			})
			require.NoError(t, tc.run(f, ctx))
			events := consentRevocationEvents(t, &logs)
			require.Len(t, events, 2, "one post-commit audit event per newly revoked session")
			want := map[string]bool{first.ID.String(): true, second.ID.String(): true}
			reason := string(storage.RefreshReasonGrantDeleted)
			if tc.name == "expired grant renewal" {
				reason = string(storage.RefreshReasonExpiredGrantRenewal)
			}
			for _, event := range events {
				sessionID, ok := event["session_id"].(string)
				require.True(t, ok)
				require.True(t, want[sessionID], "no unrelated or duplicate session audit: %s", sessionID)
				delete(want, sessionID)
				require.Equal(t, f.principal.String(), event["principal"])
				require.Equal(t, f.agentID.String(), event["agent_id"])
				require.Equal(t, first.ClientID.String(), event["client_id"])
				require.Equal(t, reason, event["reason"])
				require.Equal(t, "request", event["origin"])
				require.Equal(t, "203.0.113.44", event["client_ip"])
				require.Equal(t, security.TruncateUserAgent(ua), event["user_agent"])
			}
			require.Empty(t, want)
			require.NotContains(t, logs.String(), "do-not-log")
			require.NotContains(t, logs.String(), unrelated.ID.String())
		})
	}
}

func TestConsentRevocationAudit_RollbackNeverLogsSuccessfulTransition(t *testing.T) {
	for _, fault := range []struct {
		name   string
		inject func(*consentRenewalFixture, error)
	}{
		{"revocation", func(f *consentRenewalFixture, err error) { f.roots.fault = err }},
		{"grant delete", func(f *consentRenewalFixture, err error) { f.grants.err = err }},
		{"commit", func(f *consentRenewalFixture, err error) { f.coordinator.beforeCommitErr = err }},
	} {
		t.Run(fault.name, func(t *testing.T) {
			at := time.Date(2034, time.June, 10, 12, 0, 0, 0, time.UTC)
			f := newConsentRenewalFixture(at, at.Add(time.Hour))
			root := f.addRoot(t, f.principal, f.agentID, f.grant.ID, false)
			var logs bytes.Buffer
			f.service.logger = slog.New(slog.NewJSONHandler(&logs, nil))
			errInjected := errors.New("confirmed lifecycle rollback")
			fault.inject(f, errInjected)
			require.ErrorIs(t, f.service.RevokeConsent(context.Background(), f.principal, f.agentID), errInjected)
			require.Empty(t, consentRevocationEvents(t, &logs))
			require.Nil(t, f.roots.roots[root.ID].TerminalReason)
		})
	}
}
