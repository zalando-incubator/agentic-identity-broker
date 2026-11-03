package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Agent-Subject Impersonation", func() {
	var (
		store              *storageadapter.Adapter
		factory            *bootstrap.StorageFactory
		issuer             *helpers.MockJWKSServer
		upstream           *helpers.MockUpstreamOAuth2Server
		target, other      *storage.Agent
		admin, enduser     *bootstrap.TestServer
		tp                 *sdktrace.TracerProvider
		recorder           *tracetest.SpanRecorder
		previousProvider   trace.TracerProvider
		previousPropagator propagation.TextMapPropagator
	)
	ctx := context.Background()
	closeServers := func() {
		if enduser != nil {
			enduser.Close()
			enduser = nil
		}
		if admin != nil {
			admin.Close()
			admin = nil
		}
		if tp != nil {
			shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			Expect(tp.Shutdown(shutdownCtx)).To(Succeed())
			tp = nil
		}
	}
	BeforeEach(func() {
		previousProvider = otel.GetTracerProvider()
		previousPropagator = otel.GetTextMapPropagator()
		factory = bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelInfo))
		var err error
		store, err = factory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())
		issuer = helpers.NewMockJWKSServer()
		upstream = helpers.NewMockUpstreamOAuth2Server()
		target = fixtures.ImpersonationTarget("research-agent")
		other = fixtures.ImpersonationTarget("other-agent")
		Expect(store.Agents().Create(ctx, target)).To(Succeed())
		Expect(store.Agents().Create(ctx, other)).To(Succeed())
	})
	AfterEach(func() {
		closeServers()
		otel.SetTracerProvider(previousProvider)
		otel.SetTextMapPropagator(previousPropagator)
		issuer.Close()
		upstream.Close()
		Expect(factory.CloseStorage(store)).To(Succeed())
	})
	configFor := func(categories []string) *ports.Config {
		cfg := fixtures.LocalConfig()
		cfg.Security.SkipThirdpartyHTTPSValidation = true
		cfg.OAuth2AuthServer.Local.TokenClaimsExpression = `{"aud": "https://broker.example.com/impersonation"}`
		cfg.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{
			AudiencePrefix: fixtures.ImpersonationAudience,
			Rules:          []ports.ImpersonationRuleConfig{fixtures.SignedImpersonationRule(issuer.URL(), issuer.JWKSURL(), categories)},
		}
		return cfg
	}
	boot := func(cfg *ports.Config, traced bool) {
		closeServers()
		serverFactory := bootstrap.NewServerFactory(cfg, bootstrap.TestLogger(slog.LevelInfo))
		if traced {
			fixtures.EnableTelemetryTracing(cfg)
			tp, recorder = bootstrap.NewInMemoryTracerProvider()
			app, err := serverFactory.BuildAppWithTracerProvider(store, tp)
			Expect(err).NotTo(HaveOccurred())
			admin, err = bootstrap.NewAdminTestServer(app, bootstrap.TestLogger(slog.LevelInfo))
			Expect(err).NotTo(HaveOccurred())
			enduser, err = bootstrap.NewEndUserTestServer(app, bootstrap.TestLogger(slog.LevelInfo))
			Expect(err).NotTo(HaveOccurred())
		} else {
			app, err := serverFactory.BuildApp(store)
			Expect(err).NotTo(HaveOccurred())
			admin, err = bootstrap.NewAdminTestServer(app, bootstrap.TestLogger(slog.LevelInfo))
			Expect(err).NotTo(HaveOccurred())
			enduser, err = bootstrap.NewEndUserTestServer(app, bootstrap.TestLogger(slog.LevelInfo))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(helpers.ProvisionSigningKey(admin.BaseURL())).To(Succeed())
	}
	sign := func(subject, audience string) string {
		now := time.Now()
		token, err := issuer.SignJWT(map[string]any{"iss": issuer.URL(), "sub": subject, "aud": audience, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
		Expect(err).NotTo(HaveOccurred())
		return token
	}
	requestFor := func(subject string) url.Values {
		return url.Values{
			"grant_type": {imperGrant}, "audience": {fixtures.ImpersonationAudience + "/" + target.ID.String()},
			"client_assertion_type": {imperBearerTyp}, "client_assertion": {sign("gateway-prod", fixtures.ImpersonationAudience)},
			"actor_token_type": {imperJWTType}, "actor_token": {sign("research-runner", fixtures.ImpersonationAudience)},
			"subject_token_type": {imperJWTType}, "subject_token": {sign(subject, fixtures.ImpersonationAudience)},
		}
	}
	post := func(form url.Values) (int, map[string]any) {
		resp, err := enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		Expect(err).NotTo(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return resp.StatusCode, body
	}
	seedGrant := func(principal string, expired bool) *storage.UserGrant {
		grant := fixtures.ActiveGrant(principal, target.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
		if expired {
			grant = fixtures.ExpiredGrant(principal, target.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
		}
		Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
		return grant
	}
	mint := func(subject string) string {
		status, body := post(requestFor(subject))
		Expect(status).To(Equal(http.StatusOK), "%v", body)
		Expect(body).NotTo(HaveKey("error_uri"))
		Expect(body["issued_token_type"]).To(Equal(imperAccessTyp))
		Expect(body["token_type"]).To(Equal("Bearer"))
		token, ok := body["access_token"].(string)
		Expect(ok).To(BeTrue())
		Expect(validateJWTWithJWKS(token, fetchBrokerJWKS(enduser.BaseURL()))).To(Succeed())
		return token
	}
	assertNoState := func(principal string) {
		grants, err := store.UserGrants().ListByPrincipalAndAgent(ctx, id.Principal(principal), target.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(grants).To(BeEmpty())
		sessions, err := store.UserSessions().ListByPrincipal(ctx, id.Principal(principal))
		Expect(err).NotTo(HaveOccurred())
		Expect(sessions).To(BeEmpty())
	}
	assertDenied := func(form url.Values, consent bool) map[string]any {
		status, body := post(form)
		Expect(status).To(Equal(http.StatusForbidden), "%v", body)
		Expect(body["error"]).To(Equal("access_denied"))
		Expect(body).NotTo(HaveKey("access_token"))
		if consent {
			Expect(body["error_uri"]).To(ContainSubstring("/agents/" + target.ID.String()))
		} else {
			Expect(body).NotTo(HaveKey("error_uri"))
		}
		return body
	}

	Context("signed self-agent credentials", func() {
		BeforeEach(func() { boot(configFor([]string{"agent"}), false) })
		// US1.1 from specs/054-agent-subject-impersonation/spec.md
		It("issues an attributable signed target token without creating human state", func() {
			assertNoState(target.ID.String())
			claims := decodeJWTClaims(mint(target.ID.String()))
			Expect(claims["sub"]).To(Equal(target.ID.String()))
			Expect(claims["agent_id"]).To(Equal(target.ID.String()))
			Expect(claims).NotTo(HaveKey("subject_category"))
			Expect(claims).NotTo(HaveKey("email"))
			assertNoState(target.ID.String())
		})
		// US1.2 from specs/054-agent-subject-impersonation/spec.md
		It("retains the verified actor separately from client and represented target", func() {
			claims := decodeJWTClaims(mint(target.ID.String()))
			Expect(claims["act"]).To(Equal(map[string]any{"iss": issuer.URL(), "sub": "research-runner"}))
			form := requestFor(target.ID.String())
			form.Set("actor_token", sign(target.ID.String(), fixtures.ImpersonationAudience))
			cfg := configFor([]string{"agent"})
			cfg.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression = "client_assertion.sub == 'gateway-prod' && actor_token.sub == subject_token.sub"
			boot(cfg, false)
			status, body := post(form)
			Expect(status).To(Equal(http.StatusOK), "%v", body)
			Expect(decodeJWTClaims(body["access_token"].(string))["act"]).To(Equal(map[string]any{"iss": issuer.URL(), "sub": target.ID.String()}))
		})
		// US1.3 from specs/054-agent-subject-impersonation/spec.md
		It("rejects independently invalid client and actor signatures", func() {
			untrusted := helpers.NewMockJWKSServer()
			defer untrusted.Close()
			for _, field := range []string{"client_assertion", "actor_token"} {
				form := requestFor(target.ID.String())
				claims := decodeJWTClaims(form.Get(field))
				invalid, err := untrusted.SignJWT(claims)
				Expect(err).NotTo(HaveOccurred())
				form.Set(field, invalid)
				status, body := post(form)
				if field == "client_assertion" {
					Expect(status).To(Equal(http.StatusUnauthorized))
				} else {
					Expect(status).To(Equal(http.StatusBadRequest))
				}
				Expect(body).NotTo(HaveKey("access_token"))
				Expect(body).NotTo(HaveKey("error_uri"))
			}
			assertNoState(target.ID.String())
		})
	})

	Context("category eligibility and human consent", func() {
		// US2.1 from specs/054-agent-subject-impersonation/spec.md
		It("enforces all six category-rule combinations, including same-text delegation", func() {
			seedGrant("alice@example.com", false)
			seedGrant(target.ID.String(), false)
			for _, categories := range [][]string{{"human"}, {"agent"}, {"human", "agent"}} {
				boot(configFor(categories), false)
				for _, subject := range []string{"alice@example.com", target.ID.String()} {
					allowed := len(categories) == 2 || (categories[0] == "agent") == (subject == target.ID.String())
					if allowed {
						Expect(decodeJWTClaims(mint(subject))["sub"]).To(Equal(subject))
					} else {
						body := assertDenied(requestFor(subject), false)
						Expect(body["error_description"]).To(Equal("impersonation not permitted"))
					}
				}
			}
		})
		// US2.2 from specs/054-agent-subject-impersonation/spec.md
		It("uses consent-free agent issuance and delegated human issuance on a dual rule", func() {
			boot(configFor([]string{"human", "agent"}), false)
			seedGrant("alice@example.com", false)
			Expect(decodeJWTClaims(mint(target.ID.String()))["sub"]).To(Equal(target.ID.String()))
			Expect(decodeJWTClaims(mint("alice@example.com"))["sub"]).To(Equal("alice@example.com"))
			assertNoState(target.ID.String())
		})
		// US2.3 from specs/054-agent-subject-impersonation/spec.md
		It("never recovers a missing or expired human delegation by reclassifying", func() {
			boot(configFor([]string{"human", "agent"}), false)
			for _, subject := range []string{"alice@example.com", other.ID.String(), strings.ToUpper(target.ID.String()), *target.CanonicalID} {
				assertDenied(requestFor(subject), true)
			}
			seedGrant("alice@example.com", true)
			assertDenied(requestFor("alice@example.com"), true)
		})
		// US2.4 from specs/054-agent-subject-impersonation/spec.md
		It("keeps omission human-only but rejects signed exact-target collisions", func() {
			boot(configFor(nil), false)
			assertDenied(requestFor("alice@example.com"), true)
			seedGrant("expired@example.com", true)
			assertDenied(requestFor("expired@example.com"), true)
			seedGrant("alice@example.com", false)
			Expect(decodeJWTClaims(mint("alice@example.com"))["sub"]).To(Equal("alice@example.com"))
			assertDenied(requestFor(target.ID.String()), false)
			seedGrant(target.ID.String(), false)
			assertDenied(requestFor(target.ID.String()), false)
		})
		// US2.5 from specs/054-agent-subject-impersonation/spec.md
		It("rejects invalid category values and YAML shapes before accepting traffic", func() {
			for _, categories := range [][]string{{}, {"machine"}, {"Human"}, {"human", "human"}, {"agent", "human", "agent"}} {
				_, err := bootstrap.NewServerFactory(configFor(categories), bootstrap.TestLogger(slog.LevelInfo)).BuildApp(store)
				Expect(err).To(HaveOccurred(), "categories=%v", categories)
				Expect(err.Error()).To(ContainSubstring("allowed_subject_categories"))
			}
			example, err := os.ReadFile("../../examples/config/impersonation-agent-subject.yaml")
			Expect(err).NotTo(HaveOccurred())
			dir := GinkgoT().TempDir()
			for _, value := range []string{"[]", "[machine]", "[Human]", "[human, human]", "null", "agent", "{agent: true}", "[agent, 1]", "[null]"} {
				path := filepath.Join(dir, "config.yaml")
				document := strings.Replace(string(example), "allowed_subject_categories: [agent]", "allowed_subject_categories: "+value, 1)
				Expect(os.WriteFile(path, []byte(document), 0600)).To(Succeed())
				_, err := bootstrap.LoadCredentialConfiguration(path, nil, nil)
				Expect(err).To(HaveOccurred(), "value=%s", value)
				Expect(err.Error()).To(ContainSubstring("oauth2_authorization_server.impersonation.rules[0].allowed_subject_categories"))
			}
			for _, categories := range [][]string{nil, {"human"}, {"agent"}, {"human", "agent"}, {"agent", "human"}} {
				boot(configFor(categories), false)
				line := "allowed_subject_categories: [" + strings.Join(categories, ", ") + "]"
				if categories == nil {
					line = "# omitted categories"
				}
				document := strings.Replace(string(example), "allowed_subject_categories: [agent]", line, 1)
				path := filepath.Join(dir, "valid.yaml")
				Expect(os.WriteFile(path, []byte(document), 0600)).To(Succeed())
				loaded, err := bootstrap.LoadCredentialConfiguration(path, nil, nil)
				Expect(err).NotTo(HaveOccurred())
				Expect(loaded.OAuth2AuthServer.Impersonation.Rules[0].AllowedSubjectCategories).To(Equal(ports.SubjectCategoryList(categories)))
			}
			cfg := configFor([]string{"agent"})
			rule := imperUnverifiedRule("unsigned-human", issuer.URL(), issuer.JWKSURL(), "subject_token.sub != ''")
			rule.AllowedSubjectCategories = []string{"agent"}
			cfg.OAuth2AuthServer.Impersonation.Rules = []ports.ImpersonationRuleConfig{rule}
			boot(cfg, false)
		})
	})

	Context("broker authorization and third-party isolation", func() {
		// US3.1 from specs/054-agent-subject-impersonation/spec.md
		It("denies an otherwise valid agent outside the CEL policy", func() {
			cfg := configFor([]string{"agent"})
			cfg.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression = "subject_token.sub == 'not-authorized' && request.agent_id != ''"
			boot(cfg, false)
			assertDenied(requestFor(target.ID.String()), false)
			assertNoState(target.ID.String())
		})
		// US3.2 from specs/054-agent-subject-impersonation/spec.md
		It("rejects nonexact subject identities and unresolved audience targets", func() {
			boot(configFor([]string{"agent"}), false)
			for _, subject := range []string{"", "invalid", *target.CanonicalID, strings.ToUpper(target.ID.String()), "{" + target.ID.String() + "}", other.ID.String()} {
				status, body := post(requestFor(subject))
				Expect(status).To(BeNumerically(">=", 400), "subject=%s body=%v", subject, body)
				Expect(body).NotTo(HaveKey("access_token"))
				Expect(body).NotTo(HaveKey("error_uri"))
			}
			form := requestFor(target.ID.String())
			form.Set("audience", fixtures.ImpersonationAudience+"/"+id.NewAgentID().String())
			status, body := post(form)
			Expect(status).To(BeNumerically(">=", 400))
			Expect(body).NotTo(HaveKey("access_token"))
		})
		// US3.3 from specs/054-agent-subject-impersonation/spec.md
		It("fails closed on runtime authorization errors without consent recovery", func() {
			cfg := configFor([]string{"agent"})
			cfg.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression = "subject_token.missing == request.agent_id"
			boot(cfg, false)
			status, body := post(requestFor(target.ID.String()))
			Expect(status).To(Equal(http.StatusInternalServerError))
			Expect(body["error"]).To(Equal("server_error"))
			Expect(body).NotTo(HaveKey("access_token"))
			Expect(body).NotTo(HaveKey("error_uri"))
			assertNoState(target.ID.String())
		})
	})

	// US3.4 from specs/054-agent-subject-impersonation/spec.md
	Context("third-party detection of agent-subject tokens", func() {
		const (
			compliantDetector   = "(has(subject_token.sub) && has(subject_token.agent_id) && subject_token.sub == subject_token.agent_id) || (has(subject_token.kind) && subject_token.kind == 'machine')"
			nonMatchingDetector = "has(subject_token.sub) && subject_token.sub == 'nobody'"
			failingDetector     = "subject_token.sub"
			agentSubjectDenial  = "agent-subject tokens cannot be exchanged for third-party service tokens"
		)
		type detectorOutcome int
		const (
			rejected detectorOutcome = iota
			continued
			failed
		)
		var service *model.ThirdpartyOAuth2ProviderEntity

		BeforeEach(func() {
			service = fixtures.GitHubService()
			service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
			service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
			Expect(store.Services().Create(ctx, service)).To(Succeed())
			Expect(fixtures.SeedPlaceholderGrantData(ctx, store, service.ID)).To(Succeed())
		})

		configure := func(expression string) *ports.Config {
			cfg := configFor([]string{"human", "agent"})
			cfg.TokenExchange = ports.TokenExchangeConfig{
				ExpectedAudience: fixtures.ImpersonationAudience,
				ClientAssertion:  ports.ClientAssertionTrustConfig{IssuerURI: issuer.URL(), JWKSURI: issuer.JWKSURL()},
				ClaimExtraction:  ports.ClaimExtractionConfig{PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.agent_id", AgentSubjectExpression: expression},
				Authorization:    ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true", EvaluationTimeout: time.Second}},
			}
			return cfg
		}
		observe := func(token string) (int, map[string]any, map[string]int, []string) {
			traceID := strings.ReplaceAll(uuid.NewString(), "-", "")
			form := url.Values{"grant_type": {imperGrant}, "subject_token_type": {imperAccessTyp}, "subject_token": {token}, "client_assertion_type": {imperBearerTyp}, "client_assertion": {sign("gateway-prod", fixtures.ImpersonationAudience)}, "resource": {"https://api.github.com"}}
			req, err := http.NewRequest(http.MethodPost, enduser.BaseURL()+"/oauth2/token", strings.NewReader(form.Encode()))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("traceparent", fmt.Sprintf("00-%s-0123456789abcdef-01", traceID))
			resp, err := helpers.HTTPClient().Do(req)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()
			var body map[string]any
			Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
			Eventually(func() bool {
				for _, span := range recorder.Ended() {
					if span.SpanContext().TraceID().String() == traceID && span.SpanKind() == trace.SpanKindServer {
						return true
					}
				}
				return false
			}, 5*time.Second, 10*time.Millisecond).To(BeTrue(), "completed sampled HTTP trace required")
			counts := map[string]int{}
			var details []string
			for _, span := range recorder.Ended() {
				if span.SpanContext().TraceID().String() != traceID {
					continue
				}
				counts[span.Name()]++
				for _, attr := range span.Attributes() {
					if string(attr.Key) == "token_exchange.failure_detail" {
						details = append(details, attr.Value.AsString())
					}
				}
			}
			return resp.StatusCode, body, counts, details
		}
		// assertHumanControls proves the observed spans reach human state for a human token,
		// so their absence for an agent-subject token is meaningful.
		assertHumanControls := func() {
			grant := seedGrant("observation-control", false)
			token := mint("observation-control")
			Expect(store.UserGrants().Delete(ctx, grant.ID)).To(Succeed())
			status, body, spans, _ := observe(token)
			Expect(status).To(Equal(http.StatusForbidden))
			Expect(body["error_uri"]).To(ContainSubstring("/agents/"))
			Expect(spans["tokenexchange.verify_agent_access"]).To(Equal(1))
			Expect(spans["tokenexchange.get_valid_access_token"]).To(BeZero())
			grant = fixtures.ActiveGrant("observation-control", target.ID.String(), service.ID.String(), []string{"repo", "user"})
			Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
			session := fixtures.GitHubSessionForPrincipal("observation-control")
			Expect(store.UserSessions().Create(ctx, session)).To(Succeed())
			status, body, spans, _ = observe(token)
			Expect(status).To(Equal(http.StatusOK), "%v", body)
			Expect(spans["tokenexchange.verify_agent_access"]).To(Equal(1))
			Expect(spans["tokenexchange.get_valid_access_token"]).To(Equal(1))
			Expect(store.UserGrants().Delete(ctx, grant.ID)).To(Succeed())
			Expect(store.UserSessions().Delete(ctx, session.ID)).To(Succeed())
		}

		DescribeTable("observes detector rejection, replacement and failure before human-state boundaries",
			func(expression string, outcome detectorOutcome, sameTextHuman bool) {
				if outcome == failed {
					boot(configure(""), true)
					assertHumanControls()
				}
				boot(configure(expression), true)
				if outcome != failed {
					assertHumanControls()
				}
				token := mint(target.ID.String())
				var grant *storage.UserGrant
				var session *storage.UserSession
				if sameTextHuman {
					grant = fixtures.ActiveGrant(target.ID.String(), target.ID.String(), service.ID.String(), []string{"repo", "user"})
					session = fixtures.GitHubSessionForPrincipal(target.ID.String())
					Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
					Expect(store.UserSessions().Create(ctx, session)).To(Succeed())
				}
				upstream.Reset()

				status, body, spans, details := observe(token)
				switch outcome {
				case rejected:
					Expect(status).To(Equal(http.StatusForbidden))
					Expect(body["error"]).To(Equal("access_denied"))
					Expect(body["error_description"]).To(Equal(agentSubjectDenial))
					Expect(details).To(ContainElement("subject_category_unsupported"))
				case failed:
					Expect(status).To(Equal(http.StatusInternalServerError))
					Expect(body["error"]).To(Equal("server_error"))
				case continued:
					Expect(body["error_description"]).NotTo(Equal(agentSubjectDenial))
					Expect(details).NotTo(ContainElement("subject_category_unsupported"))
					Expect(spans["tokenexchange.verify_agent_access"]).To(Equal(1))
					if sameTextHuman {
						Expect(spans["tokenexchange.get_valid_access_token"]).To(Equal(1))
					} else {
						Expect(status).To(Equal(http.StatusForbidden))
						Expect(body["error"]).To(Equal("access_denied"))
						Expect(body["error_uri"]).To(ContainSubstring("/agents/"))
						Expect(details).To(ContainElement("grant_missing"))
					}
				}
				if outcome != continued {
					Expect(body).NotTo(HaveKey("error_uri"))
					Expect(spans["tokenexchange.verify_agent_access"]).To(BeZero())
					Expect(spans["tokenexchange.get_valid_access_token"]).To(BeZero())
				}
				Expect(upstream.GetTokenRequests()).To(BeEmpty())

				if !sameTextHuman {
					assertNoState(target.ID.String())
					return
				}
				storedGrant, err := store.UserGrants().Get(ctx, grant.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(storedGrant).To(Equal(grant))
				storedSession, err := store.UserSessions().Get(ctx, session.ID)
				Expect(err).NotTo(HaveOccurred())
				Expect(storedSession).To(Equal(session))
			},
			Entry("default detector rejects", "", rejected, false),
			Entry("default detector rejects despite same-text human state", "", rejected, true),
			Entry("compliant replacement rejects", compliantDetector, rejected, false),
			Entry("compliant replacement rejects despite same-text human state", compliantDetector, rejected, true),
			Entry("non-matching replacement continues to grant checks", nonMatchingDetector, continued, false),
			Entry("non-matching replacement continues to same-text human state", nonMatchingDetector, continued, true),
			Entry("detector failure fails closed", failingDetector, failed, false),
			Entry("detector failure fails closed despite same-text human state", failingDetector, failed, true),
		)

		It("rejects a replacement detector that does not reference subject_token.sub at startup", func() {
			for _, expression := range []string{"false", "true", "subject_token.azp == subject_token.agent_id"} {
				_, err := bootstrap.NewServerFactory(configure(expression), bootstrap.TestLogger(slog.LevelInfo)).BuildApp(store)
				Expect(err).To(HaveOccurred(), "expression=%s", expression)
				Expect(err.Error()).To(ContainSubstring("subject_token.sub"))
			}
		})
	})
})
