package oauth2

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/url"
	"slices"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sessionUsabilityEvaluatorFunc func(context.Context, []*storage.UserSession, ports.UsabilityMode) (map[id.ServiceID]bool, error)

func (f sessionUsabilityEvaluatorFunc) RequiresReauthentication(ctx context.Context, sessions []*storage.UserSession, mode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
	return f(ctx, sessions, mode)
}

func TestAuthorizationService_HandleAuthorization_ClientIdentityUsability(t *testing.T) {
	modes := []struct {
		name     string
		strategy ModeStrategy
		clientID *id.ClientID
	}{
		{name: "local", strategy: NewLocalModeStrategy()},
		{name: "hybrid local client", strategy: NewHybridModeStrategy()},
		{name: "hybrid proxy client", strategy: NewHybridModeStrategy(), clientID: ptr.To(id.ClientID("upstream-agent"))},
	}
	cases := []struct {
		name             string
		includeReports   bool
		identityUnusable bool
		unknownExpiry    bool
		expiredAccess    bool
		refreshable      bool
		rejected         bool
		expiredGrant     bool
		gateError        bool
		wantAction       string
		missingEvaluator bool
		mandatoryReports bool
		emptyGrant       bool
	}{
		{name: "included superseded session with unexpired access requires consent", includeReports: true, identityUnusable: true, wantAction: "redirect_to_consent"},
		{name: "included superseded session without expiry requires consent", includeReports: true, identityUnusable: true, unknownExpiry: true, wantAction: "redirect_to_consent"},
		{name: "refresh token cannot rescue superseded identity", includeReports: true, identityUnusable: true, expiredAccess: true, refreshable: true, wantAction: "redirect_to_consent"},
		{name: "usable identity retains automatic consent", includeReports: true, wantAction: "proceed"},
		{name: "expired access with usable identity remains refreshable", includeReports: true, expiredAccess: true, refreshable: true, wantAction: "proceed"},
		{name: "observed rejection still requires consent despite unexpired access", includeReports: true, refreshable: true, rejected: true, wantAction: "redirect_to_consent"},
		{name: "excluded optional superseded service does not require consent", identityUnusable: true, wantAction: "proceed"},
		{name: "usable reconnect does not revive expired grant", includeReports: true, expiredGrant: true, wantAction: "redirect_to_consent"},
		{name: "identity read failure fails closed instead of falling back", includeReports: true, gateError: true, wantAction: "error"},
		{name: "missing evaluator fails closed", includeReports: true, missingEvaluator: true, wantAction: "error"},
		{name: "mandatory omitted superseded session requires consent", mandatoryReports: true, emptyGrant: true, identityUnusable: true, wantAction: "redirect_to_consent"},
		{name: "mandatory omitted superseded session without expiry requires consent", mandatoryReports: true, emptyGrant: true, identityUnusable: true, unknownExpiry: true, wantAction: "redirect_to_consent"},
		{name: "mandatory omitted refresh token cannot rescue superseded identity", mandatoryReports: true, emptyGrant: true, identityUnusable: true, expiredAccess: true, refreshable: true, wantAction: "redirect_to_consent"},
		{name: "mandatory omitted identity source failure returns server error", mandatoryReports: true, emptyGrant: true, gateError: true, wantAction: "error"},
		{name: "mandatory omitted missing evaluator fails closed", mandatoryReports: true, emptyGrant: true, missingEvaluator: true, wantAction: "error"},
		{name: "mandatory omitted usable identity proceeds", mandatoryReports: true, emptyGrant: true, wantAction: "proceed"},
		{name: "mandatory omitted usable identity without expiry proceeds", mandatoryReports: true, emptyGrant: true, unknownExpiry: true, wantAction: "proceed"},
		{name: "mandatory omitted expired access remains refreshable", mandatoryReports: true, emptyGrant: true, expiredAccess: true, refreshable: true, wantAction: "proceed"},
		{name: "mandatory omitted from nonempty grant still requires consent", mandatoryReports: true, identityUnusable: true, wantAction: "redirect_to_consent"},
		{name: "mandatory omitted source failure after included gate returns server error", mandatoryReports: true, gateError: true, wantAction: "error"},
		{name: "mandatory included service is read and evaluated once", mandatoryReports: true, includeReports: true, wantAction: "proceed"},
	}

	for _, mode := range modes {
		t.Run(mode.name, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					ctx := context.Background()
					principal := id.Principal("alice@example.com")
					agentID, psID := id.NewAgentID(), id.NewPermissionSetID()
					reportsID, calendarID := id.NewServiceID(), id.NewServiceID()
					agentRepo, grantRepo := NewMockAgentRepository(), NewMockGrantRepository()
					agent := &storage.Agent{
						ID: agentID, ClientID: mode.clientID, DisplayName: "Reports Reader",
						RedirectURIs:        []string{"https://agent.example.com/callback"},
						AllowedScopes:       []string{"openid", "profile"},
						PermissionSets:      []storage.AgentPermissionSetEntry{{PermissionSetID: psID, RequirementType: storage.RequirementTypeMandatory}},
						ServiceRequirements: []storage.ServiceRequirement{{ServiceID: reportsID, RequirementType: storage.RequirementTypeOptional}},
					}
					if tc.mandatoryReports {
						agent.ServiceRequirements[0].RequirementType = storage.RequirementTypeMandatory
						agent.ServiceRequirements[0].RequiredScopes = []string{"reports:read"}
					}
					require.NoError(t, agentRepo.Create(ctx, agent))
					included := []id.ServiceID{calendarID}
					if tc.includeReports {
						included = append(included, reportsID)
					}
					if tc.emptyGrant {
						included = nil
					}
					relevant := slices.Clone(included)
					if tc.mandatoryReports && !tc.includeReports {
						relevant = append(relevant, reportsID)
					}
					validUntil := time.Now().Add(time.Hour)
					if tc.expiredGrant {
						validUntil = time.Now().Add(-time.Hour)
					}
					grant := &storage.UserGrant{
						ID: id.NewGrantID(), Principal: principal, AgentID: agentID, ValidUntil: &validUntil,
						GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{PermissionSetID: psID, IncludedServiceIDs: included}},
						CreatedAt:             time.Now().Add(-24 * time.Hour), UpdatedAt: time.Now().Add(-2 * time.Hour),
					}
					require.NoError(t, grantRepo.Create(ctx, grant))
					beforeGrant := grant.Copy()
					expiresAt := time.Now().Add(time.Hour)
					if tc.expiredAccess {
						expiresAt = time.Now().Add(-time.Hour)
					}
					reports := &storage.UserSession{
						ID: id.NewSessionID(), Principal: principal, ServiceID: reportsID,
						UpstreamClientID: ptr.To(id.ClientID("reports-client-v1")), UpstreamClientRevision: 1,
						TokenType: "Bearer", Scope: []string{"reports:read"}, AccessTokenExpiresAt: &expiresAt,
					}
					if tc.unknownExpiry {
						reports.AccessTokenExpiresAt = nil
					}
					if tc.refreshable {
						reports.EncryptedRefreshToken = []byte("encrypted-refresh-token")
					}
					if tc.rejected {
						rejectedAt := time.Now().Add(-time.Minute)
						reports.RefreshRejectedAt = &rejectedAt
					}
					calendar := &storage.UserSession{ID: id.NewSessionID(), Principal: principal, ServiceID: calendarID, Scope: []string{"calendar:read"}}
					beforeReports, beforeCalendar := *reports, *calendar
					beforeReports.Scope = slices.Clone(reports.Scope)
					beforeReports.EncryptedRefreshToken = slices.Clone(reports.EncryptedRefreshToken)
					beforeReports.UpstreamClientID = ptr.To(*reports.UpstreamClientID)
					beforeCalendar.Scope = slices.Clone(calendar.Scope)
					sessionRepo := NewMockSessionRepository()
					reads := make(map[id.ServiceID]int)
					sessionRepo.findFunc = func(_ context.Context, gotPrincipal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
						assert.Equal(t, principal, gotPrincipal)
						require.Contains(t, relevant, serviceID, "excluded optional services must not participate in authorization")
						reads[serviceID]++
						assert.Equal(t, 1, reads[serviceID], "included mandatory services must reuse the session already read by the included-service gate")
						if serviceID == reportsID {
							return reports, nil
						}
						return calendar, nil
					}
					seen := make(map[id.ServiceID]bool)
					gateErr := &ports.SessionUsabilityError{
						ServiceID: reportsID,
						Err: errors.Join(model.NewCredentialSourceError(model.CredentialSourceReasonReadFailed),
							errors.New("/private/credential-source-sentinel/client-id credential-value-sentinel")),
					}
					evaluator := sessionUsabilityEvaluatorFunc(func(_ context.Context, sessions []*storage.UserSession, gotMode ports.UsabilityMode) (map[id.ServiceID]bool, error) {
						assert.Equal(t, ports.UsabilityGate, gotMode)
						result := make(map[id.ServiceID]bool, len(sessions))
						for _, session := range sessions {
							assert.Equal(t, principal, session.Principal)
							require.Contains(t, relevant, session.ServiceID)
							assert.False(t, seen[session.ServiceID], "each service must reach the identity evaluator only once")
							seen[session.ServiceID] = true
							result[session.ServiceID] = session.RequiresReauthentication() || (session.ServiceID == reportsID && tc.identityUnusable)
						}
						if tc.gateError && seen[reportsID] {
							return result, gateErr
						}
						return result, nil
					})
					tokens := newTestSessionTokenService()
					var logs bytes.Buffer
					svc := NewAuthorizationService(grantRepo, sessionRepo, NewAgentClientResolver(agentRepo, slog.New(slog.DiscardHandler)), &OAuth2Config{
						PublicURL: "https://broker.example.com", UpstreamAuthorizeEndpoint: "https://upstream.example.com/authorize", ModeStrategy: mode.strategy,
					}, slog.New(slog.NewJSONHandler(&logs, nil)), tokens)
					if !tc.missingEvaluator {
						svc.WithSessionUsabilityEvaluator(evaluator)
					}
					params := url.Values{
						"client_id": {agentID.String()}, "redirect_uri": {"https://agent.example.com/callback"},
						"response_type": {"code"}, "scope": {"openid profile"}, "state": {"original-agent-state"},
						"code_challenge": {"E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM"}, "code_challenge_method": {"S256"},
					}
					req := &ports.AuthorizationRequest{
						ClientID: id.ClientID(agentID.String()), RedirectURI: params.Get("redirect_uri"), ResponseType: "code",
						Scope: params.Get("scope"), State: params.Get("state"), CodeChallenge: params.Get("code_challenge"), CodeChallengeMethod: "S256",
						OriginalURL: "https://broker.example.com/oauth2/authorize?" + params.Encode(),
					}
					beforeRequest := *req

					decision, err := svc.HandleAuthorization(ctx, req, principal)
					require.NoError(t, err)
					require.NotNil(t, decision)
					assert.Equal(t, tc.wantAction, decision.Action)
					if !tc.expiredGrant && !tc.rejected && !tc.missingEvaluator {
						assert.NotEmpty(t, seen, "authorization must consult the injected evaluator")
					}
					if !tc.expiredGrant && tc.includeReports && !tc.rejected && !tc.gateError && !tc.missingEvaluator {
						assert.True(t, seen[reportsID], "the included session must reach the identity evaluator")
					}
					if !tc.expiredGrant && tc.mandatoryReports && !tc.missingEvaluator {
						assert.True(t, seen[reportsID], "mandatory sessions omitted from the grant must still reach the identity evaluator")
					}
					if tc.wantAction == "proceed" {
						assert.Equal(t, agent.ClientType(), decision.ClientType)
					}
					if tc.wantAction == "redirect_to_consent" {
						redirect, parseErr := url.Parse(decision.RedirectURL)
						require.NoError(t, parseErr)
						assert.Equal(t, "/agents/"+agentID.String(), redirect.Path)
						token := redirect.Query().Get("session_token")
						require.NotEmpty(t, token)
						authorization, validateErr := tokens.ValidateAuthorizationSessionToken(token, agentID, principal)
						require.NoError(t, validateErr)
						assert.Equal(t, req.OriginalURL, authorization.OriginalURL, "reconnect must retain state, scopes, redirect URI, and PKCE")
						_, validateErr = tokens.ValidateAuthorizationSessionToken(token, agentID, id.Principal("bob@example.com"))
						assert.ErrorIs(t, validateErr, ports.ErrSessionPrincipalMismatch)
						_, validateErr = tokens.ValidateAuthorizationSessionToken(token, id.NewAgentID(), principal)
						assert.ErrorIs(t, validateErr, ports.ErrSessionAgentMismatch)
					}
					if tc.gateError || tc.missingEvaluator {
						assert.Equal(t, "server_error", decision.ErrorCode)
						redirect, parseErr := url.Parse(decision.RedirectURL)
						require.NoError(t, parseErr)
						assert.Equal(t, "server_error", redirect.Query().Get("error"))
						assert.Equal(t, req.State, redirect.Query().Get("state"))
						assert.Empty(t, redirect.Query().Get("session_token"), "an infrastructure failure is not a reconnect prompt")
						var records []map[string]any
						for _, line := range bytes.Split(bytes.TrimSpace(logs.Bytes()), []byte("\n")) {
							if len(line) == 0 {
								continue
							}
							var record map[string]any
							require.NoError(t, json.Unmarshal(line, &record))
							records = append(records, record)
						}
						require.Len(t, records, 1, "the authorization boundary must own exactly one source-failure record")
						assert.Equal(t, "session.oauth2.credential_source_failed", records[0]["event"])
						assert.Equal(t, "ERROR", records[0]["level"])
						assert.Equal(t, "read_failed", records[0]["reason"])
						assert.Equal(t, "usability_gate", records[0]["operation"])
						assert.Equal(t, "failed", records[0]["outcome"])
						if tc.gateError || tc.emptyGrant {
							assert.Equal(t, reportsID.String(), records[0]["service_id"])
						}
						assert.Equal(t, map[string]any{
							"operation": "session_lookup", "failure_detail": "credential_source_unavailable",
							"error_kind": "infrastructure", "dependency": "credential_source",
							"http_status_code": float64(0), "oauth_error_code": "",
						}, records[0]["oauth2_session"])
						for _, sentinel := range []string{"/private/credential-source-sentinel/client-id", "credential-source-sentinel", "credential-value-sentinel", "reports-client-v1", "encrypted-refresh-token"} {
							assert.NotContains(t, logs.String(), sentinel)
							assert.NotContains(t, decision.ErrorDesc, sentinel)
							assert.NotContains(t, decision.RedirectURL, sentinel)
						}
					}
					stored, err := grantRepo.Get(ctx, grant.ID)
					require.NoError(t, err)
					assert.Equal(t, beforeGrant, stored, "recovery must not change selections or grant validity")
					if !tc.gateError && !tc.missingEvaluator {
						assert.NotContains(t, logs.String(), "session.oauth2.credential_source_failed", "an ordinary unusable verdict is not a source failure")
					}
					assert.Equal(t, beforeReports, *reports)
					assert.Equal(t, beforeCalendar, *calendar)
					assert.Equal(t, beforeRequest, *req)
				})
			}
		})
	}
}
