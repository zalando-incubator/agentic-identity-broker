package impersonation

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/unit/ledgerfixture"
)

func TestImpersonationCategoryRuleOrder(t *testing.T) {
	for _, tc := range []struct {
		name                                        string
		categories                                  [][]string
		human, unsigned, firstDenied, verifierError bool
		subjectUnbound                              bool
		wantCategory                                SubjectCategory
		status                                      ports.UserDelegationStatus
		wantRule                                    string
		wantDelegation                              int
	}{
		{name: "ineligible first then independently eligible", categories: [][]string{{"human"}, {"agent"}}, wantRule: "second", wantCategory: SubjectCategoryAgent},
		{name: "false policy then later agent", categories: [][]string{{"agent"}, {"agent"}}, firstDenied: true, wantRule: "second", wantCategory: SubjectCategoryAgent},
		{name: "agent-only mismatch cannot fall back", categories: [][]string{{"agent"}}, human: true},
		{name: "explicit collision with active human delegation", categories: [][]string{{"human"}}, status: ports.UserDelegationActive},
		{name: "omitted collision with active human delegation", categories: [][]string{nil}, status: ports.UserDelegationActive},
		{name: "dual human active", categories: [][]string{{"human", "agent"}}, human: true, status: ports.UserDelegationActive, wantRule: "first", wantCategory: SubjectCategoryHuman, wantDelegation: 1},
		{name: "missing human is terminal", categories: [][]string{{"human", "agent"}, {"agent"}}, human: true, status: ports.UserDelegationMissing, wantDelegation: 1},
		{name: "expired human is terminal", categories: [][]string{{"human", "agent"}, {"human", "agent"}}, human: true, status: ports.UserDelegationExpired, wantDelegation: 1},
		{name: "verifier failure is terminal", categories: [][]string{{"human", "agent"}, {"human", "agent"}}, human: true, verifierError: true, wantDelegation: 1},
		{name: "unsigned exact target bound by predicate is agent", categories: [][]string{{"agent"}}, unsigned: true, wantRule: "first", wantCategory: SubjectCategoryAgent},
		{name: "unsigned exact target bound by predicate on dual rule is agent", categories: [][]string{{"human", "agent"}}, unsigned: true, status: ports.UserDelegationActive, wantRule: "first", wantCategory: SubjectCategoryAgent},
		{name: "unsigned exact target unbound by predicate is delegated human", categories: [][]string{{"human", "agent"}}, unsigned: true, subjectUnbound: true, status: ports.UserDelegationActive, wantDelegation: 1, wantRule: "first", wantCategory: SubjectCategoryHuman},
		{name: "unsigned exact target unbound by predicate never agent", categories: [][]string{{"agent"}}, unsigned: true, subjectUnbound: true, status: ports.UserDelegationActive},
		{name: "unsigned human bound by predicate stays human", categories: [][]string{{"agent"}}, unsigned: true, human: true, status: ports.UserDelegationActive},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig("first", "second")
			cfg.Rules = cfg.Rules[:len(tc.categories)]
			for i, categories := range tc.categories {
				if tc.unsigned {
					cfg.Rules[i] = unverifiedSubjectConfig().Rules[0]
					cfg.Rules[i].Name = "first"
				}
				cfg.Rules[i].AllowedSubjectCategories = categories
				cfg.Rules[i].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
				cfg.Rules[i].Authorization.CEL.Expression = "subject_token.sub != ''"
				if tc.subjectUnbound {
					cfg.Rules[i].Authorization.CEL.Expression = "client_assertion.sub != '' && !has(subject_token.nonce)"
				}
				if i == 0 && tc.firstDenied {
					cfg.Rules[i].Authorization.CEL.Expression = "false"
				}
			}
			key, keyset := signedValidationKey(t, "category-order")
			issuer := &stubIssuer{}
			verifier := &recordingDelegationVerifier{status: tc.status}
			if tc.verifierError {
				verifier.err = context.DeadlineExceeded
			}
			store := &ledgerfixture.Store{}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return configurableJWKSProvider{set: keyset}, nil
			}, stubAgentRepository{}, issuer, 0, slog.New(slog.DiscardHandler), verifier, "https://broker.example.com", store.Recorder(t))
			require.NoError(t, err)
			target := testTarget()
			subject := target.Agent.ID.String()
			if tc.human {
				subject = "alice@example.com"
			}
			sign := func(sub string) string {
				return signedValidationTokenWithSubject(t, jwa.ES256(), key, "https://idp.example.com", "aud", sub, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
			}
			subjectToken := sign(subject)
			if tc.unsigned {
				subjectToken = unsignedSubjectJWT(t, map[string]any{"sub": subject})
			}
			outcome, err := svc.Impersonate(context.Background(), &Request{ClientAssertion: sign("gw"), ActorToken: sign("actor"), SubjectToken: subjectToken, SubjectTokenType: JWTTokenType}, target)
			require.Equal(t, tc.wantDelegation, verifier.calls)
			if tc.wantRule != "" {
				require.NoError(t, err)
				require.True(t, issuer.called)
				require.Equal(t, tc.wantRule, outcome.Audit.SelectedRule)
				if tc.wantCategory != "" {
					require.Equal(t, tc.wantCategory, outcome.Audit.SubjectCategory)
					require.Len(t, store.Events, 2)
					require.Equal(t, model.BusinessEventTypePrefix+"impersonation-granted", store.Events[0].Type)
					require.Equal(t, model.BusinessEventTypePrefix+"token-exchanged", store.Events[1].Type)
					for _, event := range store.Events {
						require.Equal(t, model.BusinessEventSuccess, event.Outcome)
						require.Equal(t, target.Agent.ID, event.AgentID)
						if tc.wantCategory == SubjectCategoryAgent {
							require.Nil(t, event.Subject)
							require.Nil(t, event.Actor.OnBehalfOf)
						} else {
							require.NotNil(t, event.Subject)
							require.Equal(t, id.Principal(subject), *event.Subject)
							require.Equal(t, event.Subject, event.Actor.OnBehalfOf)
						}
					}
				}
			} else {
				require.Error(t, err)
				require.False(t, issuer.called)
				var exchangeErr *tokenexchange.TokenExchangeError
				require.ErrorAs(t, err, &exchangeErr)
				if tc.verifierError {
					require.Equal(t, "server_error", exchangeErr.Code())
					require.Empty(t, exchangeErr.ErrorURI())
				} else {
					require.Equal(t, "access_denied", exchangeErr.Code())
					if tc.human && tc.wantDelegation > 0 {
						require.Contains(t, exchangeErr.ErrorURI(), "/agents/")
					} else {
						require.Empty(t, exchangeErr.ErrorURI())
					}
				}
				require.NotEmpty(t, store.Events)
				for _, event := range store.Events {
					require.Equal(t, target.Agent.ID, event.AgentID)
					require.Nil(t, event.Actor.OnBehalfOf, "a refused rule establishes no delegation")
					if outcome.Audit.SubjectCategory == SubjectCategoryHuman && !tc.unsigned {
						require.NotNil(t, event.Subject)
						require.Equal(t, id.Principal(subject), *event.Subject)
					} else {
						require.Nil(t, event.Subject, "attempted unsigned or agent identity is not a human ledger subject")
					}
				}
			}
		})
	}
}

// A signed (or predicate-bound unverified) subject whose sub is a different registered agent's
// UUID is a cross-agent request: never eligible, never routed through human delegation (A15).
func TestImpersonationRejectsCrossAgentSubject(t *testing.T) {
	other := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Other", Description: "Other agent"}
	for _, tc := range []struct {
		name         string
		categories   []string
		subject      func(target *Target) string
		unsigned     bool
		unbound      bool
		lookupErr    error
		mappedOther  bool
		status       ports.UserDelegationStatus
		wantCode     string
		wantCalls    int
		wantCategory SubjectCategory
	}{
		{name: "dual rule", categories: []string{"human", "agent"}, subject: func(*Target) string { return other.ID.String() }, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "human-only rule", categories: []string{"human"}, subject: func(*Target) string { return other.ID.String() }, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "omitted categories", subject: func(*Target) string { return other.ID.String() }, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "unsigned bound subject", categories: []string{"human", "agent"}, unsigned: true, subject: func(*Target) string { return other.ID.String() }, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "unsigned unbound subject stays human", categories: []string{"human", "agent"}, unsigned: true, unbound: true, subject: func(*Target) string { return other.ID.String() }, wantCode: "access_denied", wantCalls: 1},
		{name: "unregistered uuid stays human", categories: []string{"human", "agent"}, subject: func(*Target) string { return id.NewAgentID().String() }, wantCode: "access_denied", wantCalls: 1},
		{name: "non-canonical uuid text stays human", categories: []string{"human", "agent"}, subject: func(*Target) string { return strings.ToUpper(other.ID.String()) }, wantCode: "access_denied", wantCalls: 1},
		{name: "extracted principal names another agent", categories: []string{"human", "agent"}, subject: func(*Target) string { return "alice@example.com" }, mappedOther: true, status: ports.UserDelegationActive, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "extracted principal names another agent on human-only rule", categories: []string{"human"}, subject: func(*Target) string { return "alice@example.com" }, mappedOther: true, status: ports.UserDelegationActive, wantCode: "access_denied", wantCategory: SubjectCategoryAgent},
		{name: "agent lookup failure fails closed", categories: []string{"human", "agent"}, subject: func(*Target) string { return other.ID.String() }, lookupErr: context.DeadlineExceeded, wantCode: "server_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testImpersonationConfig("only")
			if tc.unsigned {
				cfg.Rules[0] = unverifiedSubjectConfig().Rules[0]
			}
			cfg.Rules[0].AllowedSubjectCategories = tc.categories
			cfg.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"ES256"}
			cfg.Rules[0].Authorization.CEL.Expression = "subject_token.sub != ''"
			if tc.mappedOther {
				subjectRole := cfg.Rules[0].Roles["subject"]
				subjectRole.PrincipalExpression = "'" + other.ID.String() + "'"
				cfg.Rules[0].Roles["subject"] = subjectRole
			}
			if tc.unbound {
				cfg.Rules[0].Authorization.CEL.Expression = "client_assertion.sub != '' && !has(subject_token.nonce)"
			}
			agents := stubAgentRepository{get: func(_ context.Context, agentID id.AgentID) (*storage.Agent, error) {
				if tc.lookupErr != nil {
					return nil, tc.lookupErr
				}
				if agentID == other.ID {
					return other, nil
				}
				return nil, ports.ErrNotFound
			}}
			key, keyset := signedValidationKey(t, "cross-agent")
			issuer := &stubIssuer{}
			status := tc.status
			if status == "" {
				status = ports.UserDelegationMissing
			}
			verifier := &recordingDelegationVerifier{status: status}
			store := &ledgerfixture.Store{}
			svc, err := NewService(cfg, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
				return configurableJWKSProvider{set: keyset}, nil
			}, agents, issuer, 0, slog.New(slog.DiscardHandler), verifier, "https://broker.example.com", store.Recorder(t))
			require.NoError(t, err)
			target := testTarget()
			sign := func(sub string) string {
				return signedValidationTokenWithSubject(t, jwa.ES256(), key, "https://idp.example.com", "aud", sub, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
			}
			subjectToken := sign(tc.subject(target))
			if tc.unsigned {
				subjectToken = unsignedSubjectJWT(t, map[string]any{"sub": tc.subject(target)})
			}
			outcome, err := svc.Impersonate(context.Background(), &Request{ClientAssertion: sign("gw"), ActorToken: sign("actor"), SubjectToken: subjectToken, SubjectTokenType: JWTTokenType}, target)
			require.False(t, issuer.called)
			require.Equal(t, tc.wantCalls, verifier.calls)
			var exchangeErr *tokenexchange.TokenExchangeError
			require.ErrorAs(t, err, &exchangeErr)
			require.Equal(t, tc.wantCode, exchangeErr.Code())
			if tc.wantCalls == 0 {
				require.Empty(t, exchangeErr.ErrorURI())
			}
			if tc.wantCategory != "" {
				require.Equal(t, tc.wantCategory, outcome.Audit.SubjectCategory)
				require.Len(t, store.Events, 2)
				require.Equal(t, "agentic-identity-broker.impersonation-denied", store.Events[0].Type)
				require.Equal(t, "agentic-identity-broker.token-exchange-denied", store.Events[1].Type)
				for _, event := range store.Events {
					require.Equal(t, model.BusinessEventDenied, event.Outcome)
					require.Equal(t, target.Agent.ID, event.AgentID)
					require.Nil(t, event.Subject)
					require.Nil(t, event.Actor.OnBehalfOf)
				}
			}
		})
	}
}
