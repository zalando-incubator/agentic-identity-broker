package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// Acceptance tests for specs/20261120-153502-opa-impersonation-authorization/spec.md.
var _ = Describe("OPA Impersonation Authorization", func() {
	var j *opaImpersonation

	BeforeEach(func() {
		j = newOPAImpersonation()
	})

	governed := func(categories ...string) ports.ImpersonationRuleConfig {
		return j.rule("governed-inventory", categories...).OPA(opaPackage, "result", 0)
	}

	Context("User Story 1: governance is enforced before a token is issued", func() {
		// Scenario 1.1 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 1)
		It("issues the normal token when governance policy allows a delegated human", func() {
			By("loading Journey 1's policy and governance data from a managed bundle")
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))

			status, body := j.exchange(j.request(j.agent))
			claims := j.expectToken(status, body, opaHuman)
			Expect(audienceContains(claims["aud"], "https://inventory.example.com")).To(BeTrue())
			audit := j.lastAudit()
			Expect(audit).To(HaveKeyWithValue("outcome", "success"))
			Expect(audit).To(HaveKeyWithValue("rule", "governed-inventory"))
			Expect(audit).To(HaveKeyWithValue("authorization_engine", "opa"))
			Expect(audit).To(HaveKeyWithValue("policy_revision", "impersonation=r1"))
			Expect(audit["policy_admission_id"]).NotTo(BeEmpty())

			By("loading the same bundle from a local directory")
			local := filepath.Join(j.dir, "local-bundle")
			j.writeFile("local-bundle/governance.rego", opaFixture("governance.rego"))
			j.writeFile("local-bundle/data.json", opaFixture("data.json"))
			j.writeFile("local-bundle/.manifest", `{"revision": "local-1"}`)
			config := j.config(governed())
			config.OAuth2AuthServer.Impersonation.OPA = ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{Path: local}}
			j.boot(config)
			status, body = j.exchange(j.request(j.agent))
			j.expectToken(status, body, opaHuman)
			Expect(j.lastAudit()).To(HaveKeyWithValue("policy_revision", "local=local-1"))
			Expect(j.events("oauth2_server.impersonation_policy.local_mode")).To(HaveLen(1))
		})

		// Scenario 1.2 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 1)
		It("denies with 403 access_denied when governance marks the agent unregistered", func() {
			j.publish(opaBundleName, "r1", opaFixtureData("data_revoked.json"), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))

			status, body := j.exchange(j.request(j.agent))
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)
			Expect(body).NotTo(HaveKey("error_uri"))
			audit := j.lastAudit()
			Expect(audit).To(HaveKeyWithValue("outcome", "access_denied"))
			Expect(audit).To(HaveKeyWithValue("authorization_engine", "opa"))
		})

		// Scenario 1.3 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 1)
		It("keeps the delegation recovery response when the policy allows", func() {
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))

			form := j.request(j.agent)
			form.Set("subject_token", j.signed("bob@example.com", nil))
			status, body := j.exchange(form)
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", "User authorization is insufficient. Please re-consent.")
			Expect(body["error_uri"]).To(HaveSuffix("/agents/" + j.agent.ID.String()))
		})

		// Scenario 1.4 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 1)
		It("issues a signed self-agent token without human state", func() {
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed("agent")), 10*time.Second))
			before := j.agentHumanState()

			form := j.request(j.agent)
			form.Set("subject_token", j.signed(j.agent.ID.String(), nil))
			status, body := j.exchange(form)
			claims := j.expectToken(status, body, j.agent.ID.String())
			Expect(claims["sub"]).To(Equal(claims["agent_id"]))
			Expect(claims).NotTo(HaveKey("email"))
			Expect(j.lastAudit()).To(HaveKeyWithValue("subject_category", "agent"))
			Expect(j.agentHumanState()).To(Equal(before))
		})
	})

	Context("User Story 2: policy input keeps the verified identity context", func() {
		// Scenario 2.1 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 2)
		It("exposes Envoy-compatible HTTP fields and CEL-equivalent context", func() {
			request := func() url.Values {
				form := j.request(j.agent)
				form.Set("client_assertion", j.signed(opaClient, map[string]any{"tenant_level": 3}))
				form.Set("actor_token", j.signed(opaActor, map[string]any{"groups": []string{"ops", "release"}}))
				form.Set("subject_token", j.signed(opaHuman, map[string]any{"profile": map[string]any{"department": "inventory", "level": 2}}))
				return form
			}
			headers := http.Header{
				"X-Tenant":      {"acme", "blue"},
				"Cookie":        {"session=secret-cookie"},
				"X-Agent-Id":    {"spoofed-agent"},
				"Authorization": {"Bearer spoofed-token"},
			}
			expected := map[string]any{"agent_id": j.agent.ID.String(), "host": "localhost:8000", "scheme": "http"}

			By("evaluating the OPA policy")
			j.publish(opaBundleName, "r1", map[string]any{"expected": expected}, "cel_equivalent.rego")
			j.boot(j.managed(j.config(j.rule("parity").OPA(opaPackage, "result", 0)), 10*time.Second))
			status, body := j.exchangeWithHeaders(request(), headers, "trace=1")
			j.expectToken(status, body, opaHuman)

			By("denying when a claim type differs from the CEL activation")
			form := request()
			form.Set("client_assertion", j.signed(opaClient, map[string]any{"tenant_level": "3"}))
			status, body = j.exchangeWithHeaders(form, headers, "trace=1")
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)

			By("evaluating the equivalent CEL predicate on the same request")
			cel := `client_assertion.sub == "gateway-1" && client_assertion.tenant_level == 3 && ` +
				`actor_token.groups == ["ops", "release"] && subject_token.profile == {"department": "inventory", "level": 2} && ` +
				`!subject_unverified && request.scope == "read" && request.resource == "" && ` +
				`request.agent_id == "` + j.agent.ID.String() + `"`
			j.boot(j.config(j.rule("parity").CEL(cel)))
			status, body = j.exchangeWithHeaders(request(), headers, "trace=1")
			j.expectToken(status, body, opaHuman)
		})

		// Scenario 2.2 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 2)
		It("ignores identity-looking headers when the policy requires another client", func() {
			j.publish(opaBundleName, "r1", map[string]any{"required_client": "gateway-2"}, "header_spoof.rego")
			j.boot(j.managed(j.config(j.rule("spoof").OPA(opaPackage, "result", 0)), 10*time.Second))

			headers := http.Header{"X-Client-Assertion-Sub": {"gateway-2"}, "X-Forwarded-User": {"gateway-2"}}
			status, body := j.exchangeWithHeaders(j.request(j.agent), headers, "")
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)
			Expect(j.lastAudit()).To(HaveKeyWithValue("privileged_client_identity", opaClient))
		})

		// Scenario 2.3 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 2)
		It("issues an unsigned human token with the subject's email when the policy allows", func() {
			rule := j.rule("chat-bridge")
			rule.UnverifiedSubject = true
			j.publish(opaBundleName, "r1", map[string]any{}, "exceptional.rego")
			j.boot(j.managed(j.config(rule.OPA(opaExceptionalPackage, "unsigned_subject", 0)), 10*time.Second))

			form := j.request(j.agent)
			form.Set("subject_token", unsignedJWT(map[string]any{"sub": opaHuman, "email": "alice@example.com"}))
			status, body := j.exchange(form)
			claims := j.expectToken(status, body, opaHuman)
			Expect(claims["email"]).To(Equal("alice@example.com"))

			By("denying a caller-asserted email the policy does not accept")
			form = j.request(j.agent)
			form.Set("subject_token", unsignedJWT(map[string]any{"sub": opaHuman, "email": "alice@attacker.example"}))
			status, body = j.exchange(form)
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)
		})

		// Scenario 2.4 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 2)
		It("issues an unsigned self-agent token for the exact target and rejects cross-agent subjects", func() {
			other := j.registerAgent("other-assistant", nil)
			rule := j.rule("chat-bridge", "human", "agent")
			rule.UnverifiedSubject = true
			data := map[string]any{"governance": map[string]any{"approved_agent_subjects": []string{j.agent.ID.String(), other.ID.String()}}}
			j.publish(opaBundleName, "r1", data, "exceptional.rego")
			j.boot(j.managed(j.config(rule.OPA(opaExceptionalPackage, "unsigned_subject", 0)), 10*time.Second))
			before := j.agentHumanState()

			form := j.request(j.agent)
			form.Set("subject_token", unsignedJWT(map[string]any{"sub": j.agent.ID.String()}))
			status, body := j.exchange(form)
			claims := j.expectToken(status, body, j.agent.ID.String())
			Expect(claims["sub"]).To(Equal(claims["agent_id"]))
			Expect(j.agentHumanState()).To(Equal(before))

			By("rejecting another registered agent's UUID without a consent link")
			form = j.request(j.agent)
			form.Set("subject_token", unsignedJWT(map[string]any{"sub": other.ID.String()}))
			status, body = j.exchange(form)
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", "impersonation not permitted")
			Expect(body).NotTo(HaveKey("error_uri"))
		})

		// Scenario 2.5 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 2)
		It("accepts audience-absent signed roles and rejects aud, bad signatures, and expiry", func() {
			type variant struct {
				roles []string
			}
			for _, v := range []variant{
				{roles: []string{"client_assertion"}},
				{roles: []string{"actor"}},
				{roles: []string{"subject"}},
				{roles: []string{"client_assertion", "actor", "subject"}},
			} {
				By("absent audience for " + joinRoles(v.roles))
				rule := j.rule("audienceless")
				rule.AbsentAudience = v.roles
				j.publish(opaBundleName, "r1", map[string]any{}, "exceptional.rego")
				j.boot(j.managed(j.config(rule.OPA(opaExceptionalPackage, "audienceless", 0)), 10*time.Second))

				absent := map[string]bool{}
				for _, role := range v.roles {
					absent[role] = true
				}
				form := func(mutate func(role string) string) url.Values {
					values := j.request(j.agent)
					for role, sub := range map[string]string{"client_assertion": opaClient, "actor_token": opaActor, "subject_token": opaHuman} {
						name := map[string]string{"client_assertion": "client_assertion", "actor_token": "actor", "subject_token": "subject"}[role]
						if absent[name] {
							values.Set(role, j.sign(sub, nil, nil))
						}
						if token := mutate(name); token != "" {
							values.Set(role, token)
						}
					}
					return values
				}
				status, body := j.exchange(form(func(string) string { return "" }))
				j.expectToken(status, body, opaHuman)

				for _, role := range v.roles {
					sub := map[string]string{"client_assertion": opaClient, "actor": opaActor, "subject": opaHuman}[role]
					audience := fixtures.ImpersonationAudience
					for name, token := range map[string]string{
						"aud present":     j.sign(sub, &audience, nil),
						"bad signature":   tamperSignature(j.sign(sub, nil, nil)),
						"expired":         j.sign(sub, nil, map[string]any{"exp": j.now.Add(-time.Hour).Unix()}),
						"wrong issuer":    j.sign(sub, nil, map[string]any{"iss": "https://other.example.com"}),
						"unsigned (none)": unsignedJWT(map[string]any{"iss": j.issuer.URL(), "sub": sub, "exp": j.now.Add(time.Hour).Unix()}),
					} {
						role, token := role, token
						status, body := j.exchange(form(func(r string) string {
							if r == role {
								return token
							}
							return ""
						}))
						Expect(status).NotTo(Equal(http.StatusOK), "%s on %s: %v", name, role, body)
						Expect(body).NotTo(HaveKey("access_token"))
					}
				}
			}
		})
	})

	Context("User Story 3: managed policy and data changes apply safely", func() {
		// Scenario 3.1 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 3)
		It("denies the next request after a revoking revision activates", func() {
			By("revoking the agent through a bundle update")
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))
			j.expectHumanToken(j.exchange(j.request(j.agent)))
			j.publish(opaBundleName, "r2", opaFixtureData("data_revoked.json"), "governance.rego")
			Eventually(j.status(func() url.Values { return j.request(j.agent) }), 15*time.Second, 200*time.Millisecond).Should(Equal(http.StatusForbidden))
			Expect(j.lastAudit()).To(HaveKeyWithValue("policy_revision", "impersonation=r2"))

			By("rotating the bundle token file between polls")
			token := j.writeFile("token/token", "T1\n")
			config := j.config(governed())
			config.OAuth2AuthServer.Impersonation.OPA = ports.ImpersonationOPAEngineConfig{
				Policy: ports.ImpersonationOPAPolicySource{ConfigFile: j.writeBootstrap(
					"    credentials:\n      plugin: aib_bearer_token_file\nplugins:\n  aib_bearer_token_file:\n    path: " + token + "\n")},
				StartupTimeout: 10 * time.Second,
			}
			j.bundles.RequireToken("T1")
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(config)
			j.expectHumanToken(j.exchange(j.request(j.agent)))

			j.bundles.RequireToken("T2")
			j.publish(opaBundleName, "r2", opaFixtureData("data_revoked.json"), "governance.rego")
			Consistently(j.status(func() url.Values { return j.request(j.agent) }), 2500*time.Millisecond, 250*time.Millisecond).Should(Equal(http.StatusOK))
			j.writeFile("token/token", "T2")
			Eventually(j.status(func() url.Values { return j.request(j.agent) }), 15*time.Second, 200*time.Millisecond).Should(Equal(http.StatusForbidden))
			Expect(j.bundles.AuthorizedRequests(opaBundleName)).To(BeNumerically(">=", 2))

			By("keeping r2 when the token file is emptied")
			j.writeFile("token/token", "")
			j.publish(opaBundleName, "r3", j.governanceData(), "governance.rego")
			Consistently(j.status(func() url.Values { return j.request(j.agent) }), 2500*time.Millisecond, 250*time.Millisecond).Should(Equal(http.StatusForbidden))
			Eventually(func() []map[string]any { return j.events("oauth2_server.impersonation_policy.token_unavailable") }, 5*time.Second).ShouldNot(BeEmpty())
			Expect(j.logs.Raw()).NotTo(ContainSubstring("T2"))
		})

		// Scenario 3.2 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 3)
		It("fails closed without fallthrough on runtime failures and observes caller cancellation through correlated telemetry", func() {
			permissive := j.rule("permissive").CEL("true")
			for _, variant := range []struct {
				name, module string
				timeout      time.Duration
			}{
				{"evaluation error", "strict_error.rego", 0},
				{"timeout", "slow.rego", 20 * time.Millisecond},
				{"undefined decision", "undefined.rego", 0},
				{"malformed decision", "malformed.rego", 0},
			} {
				By(variant.name)
				j.publish(opaBundleName, "r1", j.governanceData(), variant.module)
				Expect(j.build(j.managed(j.config(j.rule("governed-inventory").OPA(opaPackage, "result", variant.timeout), permissive), 10*time.Second))).To(Succeed(),
					"the decision rule exists, so the revision is admitted and the broker becomes ready")
				status, body := j.exchange(j.request(j.agent))
				expectOAuthError(status, body, http.StatusInternalServerError, "server_error", opaInternalError)
				audit := j.lastAudit()
				Expect(audit).To(HaveKeyWithValue("rule", "governed-inventory"))
				Expect(audit).To(HaveKeyWithValue("outcome", "server_error"))
				Expect(j.logs.Raw()).NotTo(ContainSubstring(`"rule":"permissive"`))
			}

			By("cancelling the client while the policy evaluates")
			tp, recorder, started := bootstrap.NewSignalingTracerProvider(opaEvaluateSpan)
			DeferCleanup(func() { _ = tp.Shutdown(context.Background()) })
			j.publish(opaBundleName, "r1", j.governanceData(), "slow.rego")
			Expect(j.buildWith(j.managed(j.config(j.rule("governed-inventory").OPA(opaPackage, "result", 5*time.Second), permissive), 10*time.Second), tp)).To(Succeed())
			j.logs.Reset()

			traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
			ctx, cancel := context.WithCancel(context.Background())
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.enduser.BaseURL()+"/oauth2/token", strings.NewReader(j.request(j.agent).Encode()))
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.Header.Set("traceparent", "00-"+traceID+"-00f067aa0ba902b7-01")
			done := make(chan error, 1)
			go func() {
				resp, err := http.DefaultClient.Do(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				done <- err
			}()
			Eventually(started.Started, 10*time.Second).Should(BeClosed())
			cancel()
			Eventually(done, 10*time.Second).Should(Receive(MatchError(context.Canceled)))

			var evaluation sdktrace.ReadOnlySpan
			Eventually(func() bool {
				for _, span := range recorder.Ended() {
					if span.Name() == opaEvaluateSpan && span.SpanContext().TraceID().String() == traceID {
						evaluation = span
					}
				}
				return evaluation != nil
			}, 10*time.Second).Should(BeTrue())
			Expect(spanAttribute(evaluation, "impersonation.authorization.action")).To(Equal("canceled"))
			Expect(evaluation.Status().Code).NotTo(Equal(codes.Error))
			Eventually(func() bool {
				for _, span := range recorder.Ended() {
					if span.SpanKind() == trace.SpanKindServer && span.SpanContext().TraceID().String() == traceID {
						return true
					}
				}
				return false
			}, 10*time.Second).Should(BeTrue())
			for _, audit := range j.audits() {
				Expect(audit["outcome"]).NotTo(Equal("success"))
				Expect(audit["rule"]).NotTo(Equal("permissive"))
			}
		})

		// Scenario 3.3 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 3)
		It("fails startup for a missing or invalid bootstrap or no ready policy within the bound", func() {
			const secret = "SYNTHETIC-BUNDLE-SECRET"
			engine := func(policy ports.ImpersonationOPAPolicySource, timeout time.Duration) *ports.Config {
				config := j.config(governed())
				config.OAuth2AuthServer.Impersonation.OPA = ports.ImpersonationOPAEngineConfig{Policy: policy, StartupTimeout: timeout}
				return config
			}
			missingBundle := filepath.Join(j.dir, "missing-bundle")
			invalidBundle := filepath.Join(j.dir, "invalid-bundle")
			j.writeFile("invalid-bundle/governance.rego", opaFixture("governance.rego"))
			j.writeFile("invalid-bundle/notes.txt", secret)
			for _, variant := range []struct {
				name   string
				config func() *ports.Config
				want   []string
			}{
				{"missing bootstrap file", func() *ports.Config {
					return engine(ports.ImpersonationOPAPolicySource{ConfigFile: filepath.Join(j.dir, "missing.yaml")}, time.Second)
				}, []string{"oauth2_authorization_server.impersonation.opa", "policy.config_file", "unreadable"}},
				{"bootstrap with an unsupported key", func() *ports.Config {
					path := j.writeFile("unsupported.yaml", "services:\n  governance:\n    url: "+j.bundles.URL()+"\n    credentials:\n      bearer:\n        token: "+secret+"\nbundles:\n  impersonation:\n    service: governance\n    resource: x\ndecision_logs:\n  console: true\n")
					return engine(ports.ImpersonationOPAPolicySource{ConfigFile: path}, time.Second)
				}, []string{"policy.config_file", "unsupported_key", "decision_logs"}},
				{"bootstrap with a file-reading credential", func() *ports.Config {
					path := j.writeFile("token-path.yaml", "services:\n  governance:\n    url: "+j.bundles.URL()+"\n    credentials:\n      bearer:\n        token_path: /var/run/"+secret+"\nbundles:\n  impersonation:\n    service: governance\n    resource: x\n")
					return engine(ports.ImpersonationOPAPolicySource{ConfigFile: path}, time.Second)
				}, []string{"policy.config_file", "services.governance.credentials.bearer.token_path"}},
				{"missing local bundle", func() *ports.Config {
					return engine(ports.ImpersonationOPAPolicySource{Path: missingBundle}, time.Second)
				}, []string{"policy.path", "unreadable"}},
				{"local bundle with an unsupported file", func() *ports.Config {
					return engine(ports.ImpersonationOPAPolicySource{Path: invalidBundle}, time.Second)
				}, []string{"policy.path", "unsupported_file", "notes.txt"}},
				{"no reachable bundle within the bound", func() *ports.Config {
					j.bundles.SetFailing(true)
					return j.managed(j.config(governed()), 2*time.Second)
				}, []string{"oauth2_authorization_server.impersonation.opa.startup_timeout", "2s"}},
				{"decision rule absent", func() *ports.Config {
					j.bundles.SetFailing(false)
					j.publish(opaBundleName, "r1", j.governanceData(), "exceptional.rego")
					return j.managed(j.config(governed()), 2*time.Second)
				}, []string{"startup_timeout", "decision_absent", "rules[0] (governed-inventory)"}},
				{"usable first bundle and a rejected complete candidate", func() *ports.Config {
					Expect(j.bundles.Publish("governance", helpers.OPABundle{
						Modules: map[string]string{"/governance.rego": opaFixture("governance.rego")},
						Data:    j.governanceData(), Revision: "g1", Roots: []string{"company", "governance"},
					})).To(Succeed())
					Expect(j.bundles.Publish("lookup", helpers.OPABundle{
						Modules: map[string]string{"/lookup.rego": "package lookup\n\nimport rego.v1\n\nurl := http.send({\"method\": \"GET\", \"url\": \"http://127.0.0.1:1\"}).body\n"},
						Data:    map[string]any{}, Revision: "l1", Roots: []string{"lookup"},
					})).To(Succeed())
					return j.managed(j.config(governed()), 3*time.Second, "governance", "lookup")
				}, []string{"startup_timeout", "prohibited_builtin"}},
			} {
				By(variant.name)
				j.logs.Reset()
				err := j.build(variant.config())
				Expect(err).To(HaveOccurred())
				for _, want := range variant.want {
					Expect(err.Error()).To(ContainSubstring(want))
				}
				Expect(err.Error()).NotTo(ContainSubstring(secret))
				Expect(j.logs.Raw()).NotTo(ContainSubstring(secret))
			}
		})

		// Scenario 3.4 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 3)
		It("keeps the last complete revision after a failed fetch or invalid update", func() {
			unapproved := j.registerAgent("reporting-assistant", ptrExternalID("reporting-agent"))
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))
			expectR1 := func() {
				j.expectHumanToken(j.exchange(j.request(j.agent)))
				Expect(j.lastAudit()).To(HaveKeyWithValue("policy_revision", "impersonation=r1"))
				status, body := j.exchange(j.request(unapproved))
				expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)
			}
			expectR1()

			By("failing bundle downloads")
			j.bundles.SetFailing(true)
			failed := j.bundles.Requests(opaBundleName)
			Eventually(func() int { return j.bundles.Requests(opaBundleName) }, 10*time.Second).Should(BeNumerically(">", failed+1))
			expectR1()
			Eventually(func() []map[string]any { return j.events("oauth2_server.impersonation_policy.bundle_download_failed") }, 5*time.Second).ShouldNot(BeEmpty())

			By("publishing a revision that does not compile")
			j.bundles.SetFailing(false)
			Expect(j.bundles.Publish(opaBundleName, helpers.OPABundle{
				Modules: map[string]string{"/broken.rego": "package company.impersonation\n\nresult := {\"action\": \"allow\"} if { undefined_function(input) }\n"},
				Data:    j.governanceData(), Revision: "r2",
			})).To(Succeed())
			Eventually(func() []map[string]any { return j.events("oauth2_server.impersonation_policy.revision_rejected") }, 10*time.Second).ShouldNot(BeEmpty())
			Expect(j.events("oauth2_server.impersonation_policy.revision_rejected")[0]).To(HaveKeyWithValue("reason", "compile_failed"))
			expectR1()
		})

		// Scenario 3.5 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 3)
		It("rejects revisions that use http.send without contacting the destination", func() {
			recorder := helpers.NewRequestRecorder()
			DeferCleanup(recorder.Close)
			lookup := map[string]any{"lookup": map[string]any{"url": recorder.URL()}}

			By("offering it as the initial revision")
			j.publish(opaBundleName, "r0", lookup, "http_send.rego")
			err := j.build(j.managed(j.config(governed()), 2*time.Second))
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("prohibited_builtin"))

			By("offering it as a managed update")
			j.publish(opaBundleName, "r1", j.governanceData(), "governance.rego")
			j.boot(j.managed(j.config(governed()), 10*time.Second))
			j.expectHumanToken(j.exchange(j.request(j.agent)))
			j.publish(opaBundleName, "r2", lookup, "http_send.rego")
			Eventually(func() []map[string]any { return j.events("oauth2_server.impersonation_policy.revision_rejected") }, 10*time.Second).ShouldNot(BeEmpty())
			rejected := j.events("oauth2_server.impersonation_policy.revision_rejected")
			Expect(rejected[len(rejected)-1]).To(HaveKeyWithValue("reason", "prohibited_builtin"))
			Expect(rejected[len(rejected)-1]).To(HaveKeyWithValue("builtin", "http.send"))
			j.expectHumanToken(j.exchange(j.request(j.agent)))
			Expect(j.lastAudit()).To(HaveKeyWithValue("policy_revision", "impersonation=r1"))
			Expect(recorder.Count()).To(BeZero())
		})
	})

	Context("User Story 4: OPA is adopted one rule at a time", func() {
		// Scenario 4.1 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 4)
		It("lets a later CEL rule authorize after an explicit OPA deny", func() {
			j.publish(opaBundleName, "r1", opaFixtureData("data_revoked.json"), "governance.rego")
			later := j.rule("cel-fallback").CEL(`client_assertion.sub == "gateway-1" && actor_token.sub == "automation-worker"`)
			j.boot(j.managed(j.config(governed(), later), 10*time.Second))

			j.expectHumanToken(j.exchange(j.request(j.agent)))
			audit := j.lastAudit()
			Expect(audit).To(HaveKeyWithValue("rule", "cel-fallback"))
			Expect(audit).To(HaveKeyWithValue("authorization_engine", "cel"))
			Expect(audit).NotTo(HaveKey("policy_revision"))
		})

		// Scenario 4.2 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 4)
		It("keeps CEL-only deployments unchanged without OPA configuration", func() {
			By("impersonation with CEL rules only")
			j.boot(j.config(j.rule("cel-only").CEL(`client_assertion.sub == "gateway-1"`)))
			j.expectHumanToken(j.exchange(j.request(j.agent)))
			Expect(j.lastAudit()).To(HaveKeyWithValue("authorization_engine", "cel"))
			form := j.request(j.agent)
			form.Set("client_assertion", j.signed("gateway-2", nil))
			status, body := j.exchange(form)
			expectOAuthError(status, body, http.StatusForbidden, "access_denied", opaDeniedDescription)
			Expect(j.events("oauth2_server.impersonation_policy.revision_admitted")).To(BeEmpty())

			By("ordinary third-party exchange")
			exchange := newAuthZENFixture(nil)
			exchange.SeedSession(exchange.Repo, []string{"read:repo"})
			status, _ = exchange.Exchange(authzenRepoResource)
			Expect(status).To(Equal(http.StatusOK))
			exchange.RevokeGrant()
			status, body = exchange.Exchange(authzenRepoResource)
			Expect(status).To(Equal(http.StatusForbidden))
			Expect(body["error"]).To(Equal("access_denied"))
			Expect(j.bundles.TotalRequests()).To(BeZero())
		})

		// Scenario 4.3 from specs/20261120-153502-opa-impersonation-authorization/spec.md (User Story 4)
		It("fails startup for unknown, conflicting, or missing engine configuration", func() {
			const rulePrefix = "oauth2_authorization_server.impersonation.rules[0] (governed-inventory): "
			engineBlock := ports.ImpersonationOPAEngineConfig{Policy: ports.ImpersonationOPAPolicySource{ConfigFile: j.writeBootstrap("")}}
			for _, variant := range []struct {
				name   string
				config func() *ports.Config
				want   string
			}{
				{"unknown engine", func() *ports.Config {
					rule := governed()
					rule.Authorization.Type = "rego"
					config := j.config(rule)
					config.OAuth2AuthServer.Impersonation.OPA = engineBlock
					return config
				}, rulePrefix + "authorization.type"},
				{"both engine blocks populated", func() *ports.Config {
					rule := governed()
					rule.Authorization.CEL = ports.CELAuthorizationConfig{Expression: "true"}
					config := j.config(rule)
					config.OAuth2AuthServer.Impersonation.OPA = engineBlock
					return config
				}, rulePrefix + "authorization.cel"},
				{"CEL rule with an OPA block", func() *ports.Config {
					rule := j.rule("governed-inventory").CEL("true")
					rule.Authorization.OPA = ports.ImpersonationRuleOPADecision{Package: opaPackage}
					return j.config(rule)
				}, rulePrefix + "authorization.opa"},
				{"OPA rule without the shared engine block", func() *ports.Config {
					return j.config(governed())
				}, "oauth2_authorization_server.impersonation.opa"},
				{"populated engine block without an OPA rule", func() *ports.Config {
					config := j.config(j.rule("governed-inventory").CEL("true"))
					config.OAuth2AuthServer.Impersonation.OPA = engineBlock
					return config
				}, "oauth2_authorization_server.impersonation.opa"},
				{"ordinary exchange selecting OPA", func() *ports.Config {
					config := j.config(j.rule("governed-inventory").CEL("true"))
					config.TokenExchange.Authorization = ports.AuthorizationConfig{Type: "opa"}
					return config
				}, "token_exchange.authorization.type: OPA is supported only on impersonation rules"},
			} {
				By(variant.name)
				err := j.build(variant.config())
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(variant.want))
			}

			By("an unsupported key in a rule's authorization.opa block")
			raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "config", "impersonation-opa.yaml"))
			Expect(err).NotTo(HaveOccurred())
			yaml := strings.Replace(string(raw), `            decision: "result"`, "            decision: \"result\"\n            policy_url: \"http://opa:8181\"", 1)
			_, err = bootstrap.LoadCredentialConfiguration(j.writeFile("config.yaml", yaml), nil, nil)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("oauth2_authorization_server.impersonation.rules[0] (governed-inventory)"))
			Expect(err.Error()).To(ContainSubstring("policy_url"))
			_, err = bootstrap.LoadCredentialConfiguration(j.writeFile("valid.yaml", string(raw)), nil, nil)
			Expect(err).NotTo(HaveOccurred())
		})
	})
})
