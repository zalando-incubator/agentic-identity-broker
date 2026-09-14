package e2e_test

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func requestSecurityTraceID(resp *http.Response) string {
	if resp == nil {
		return ""
	}

	header := resp.Header.Get("traceresponse")
	if header == "" {
		return ""
	}

	parts := strings.Split(header, "-")
	if len(parts) != 4 {
		return ""
	}

	return parts[1]
}

func requestSecurityAccessLogs(records []map[string]any, path string) []map[string]any {
	filtered := make([]map[string]any, 0, len(records))
	for _, record := range records {
		if record["msg"] != "HTTP request" {
			continue
		}
		if pathValue, ok := record["path"].(string); ok && pathValue == path {
			filtered = append(filtered, record)
		}
	}
	return filtered
}

func requestSecurityAccessLog(records []map[string]any, path string) map[string]any {
	accessLogs := requestSecurityAccessLogs(records, path)
	Expect(accessLogs).NotTo(BeEmpty())
	return accessLogs[len(accessLogs)-1]
}

func requestSecurityOAuth2AuditLog(records []map[string]any, path string) map[string]any {
	for _, record := range records {
		if record["msg"] != "OAuth2 Authorization Request" {
			continue
		}
		if requestSecurityString(record, "path") == path {
			return record
		}
	}

	Fail(fmt.Sprintf("missing OAuth2 authorization audit log for path %q", path))
	return nil
}

func requestSecurityString(record map[string]any, key string) string {
	value, _ := record[key].(string)
	return value
}

func requestSecurityObservationByLayer(observations []bootstrap.SecurityContextObservation, layer string) (bootstrap.SecurityContextObservation, bool) {
	for _, observation := range observations {
		if observation.Layer == layer {
			return observation, true
		}
	}
	return bootstrap.SecurityContextObservation{}, false
}

var _ = Describe("Request Security Context", func() {
	var (
		logger          *slog.Logger
		logCapture      *bootstrap.BufferedLogCapture
		storageFactory  *bootstrap.StorageFactory
		serverFactory   *bootstrap.ServerFactory
		testStorage     *storageadapter.Adapter
		enduserServer   *bootstrap.TestServer
		mockUpstream    *helpers.MockUpstreamOAuth2Server
		requestObserver *bootstrap.SecurityContextObserver
		tracerProvider  *sdktrace.TracerProvider
		config          *ports.Config
		principal       string
		agent           *storagedomain.Agent
		githubService   *model.ThirdpartyOAuth2ProviderEntity
		delegatedJWTs   *helpers.TokenExchangeJWTs
	)

	buildEnduserServer := func() {
		app, err := serverFactory.BuildAppWithTracerProvider(testStorage, tracerProvider)
		Expect(err).NotTo(HaveOccurred())

		enduserServer, err = bootstrap.NewTestServerV2(
			app,
			logger,
			bootstrap.WithServerType(bootstrap.ServerTypeEndUser),
			bootstrap.WithRequestSecurityObserver(requestObserver),
		)
		Expect(err).NotTo(HaveOccurred())
		logCapture.Reset()
	}

	authenticatedGET := func(path string, headers map[string]string) *http.Response {
		resp, err := enduserServer.DirectRequest(http.MethodGet, path, principal, headers, nil)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	publicGET := func(path string, headers map[string]string) *http.Response {
		resp, err := enduserServer.DirectRequest(http.MethodGet, path, "", headers, nil)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	rawGET := func(path string, principalValue string, headerValues map[string][]string) *http.Response {
		req, err := http.NewRequest(http.MethodGet, enduserServer.BaseURL()+path, nil)
		Expect(err).NotTo(HaveOccurred())
		if principalValue != "" {
			req.Header.Set("X-Remote-User", principalValue)
		}
		for key, values := range headerValues {
			for _, value := range values {
				req.Header.Add(key, value)
			}
		}

		client := &http.Client{
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		resp, err := client.Do(req)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	delegatedTokenExchange := func(path string, headers map[string]string) *http.Response {
		if path == "" {
			path = "/oauth2/token"
		}

		values := url.Values{
			"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
			"subject_token":         {delegatedJWTs.SubjectToken},
			"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
			"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
			"client_assertion":      {delegatedJWTs.ClientAssertion},
			"resource":              {"https://api.github.com"},
		}

		requestHeaders := map[string]string{
			"Content-Type": "application/x-www-form-urlencoded",
		}
		for key, value := range headers {
			requestHeaders[key] = value
		}

		resp, err := enduserServer.DirectRequest(
			http.MethodPost,
			path,
			"",
			requestHeaders,
			strings.NewReader(values.Encode()),
		)
		Expect(err).NotTo(HaveOccurred())
		return resp
	}

	capturedRecordsForTraceID := func(traceID string) []map[string]any {
		records, err := logCapture.Records()
		Expect(err).NotTo(HaveOccurred())
		Expect(records).NotTo(BeEmpty())
		for _, record := range records {
			Expect(requestSecurityString(record, "trace_id")).To(Equal(traceID), fmt.Sprintf("expected isolated trace_id on record %#v", record))
		}
		return records
	}

	BeforeEach(func() {
		logger, logCapture = bootstrap.NewBufferedJSONLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		tracerProvider = sdktrace.NewTracerProvider()
		requestObserver = bootstrap.NewSecurityContextObserver()

		config = fixtures.EnableTelemetryTracing(fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL()))
		serverFactory = bootstrap.NewServerFactory(config, logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		principal = fixtures.DefaultPrincipal().String()
		agent = fixtures.ValidAgent()
		githubService = fixtures.GitHubService()
		githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
		githubService.Endpoints.AuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"

		ctx := context.Background()
		Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())
		Expect(testStorage.Services().Create(ctx, githubService)).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)).To(Succeed())
		Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), githubService.ID.String(), []string{"repo", "user"}))).To(Succeed())
		Expect(testStorage.UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal))).To(Succeed())

		delegatedJWTs, err = helpers.BuildTokenExchangeJWTs(
			mockUpstream.GetPrivateKeyPEM(),
			mockUpstream.URL(),
			"token-exchange-broker",
			principal,
			agent.ID.String(),
			"privileged-gateway-client",
			time.Now(),
		)
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		buildEnduserServer()
	})

	AfterEach(func() {
		if enduserServer != nil {
			enduserServer.Close()
		}
		if tracerProvider != nil {
			_ = tracerProvider.Shutdown(context.Background())
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	Context("when an authenticated request crosses the HTTP perimeter", func() {
		// Scenario 1.1 from specs/033-request-security-context/spec.md
		It("should establish trace_id, actor, and client_ip at the perimeter before business logic runs", func() {
			resp := authenticatedGET("/api/me", map[string]string{"User-Agent": "request-security-context-e2e/1.0"})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			records := capturedRecordsForTraceID(traceID)
			accessLog := requestSecurityAccessLog(records, "/api/me")
			Expect(accessLog).To(HaveKeyWithValue("trace_id", traceID))
			Expect(accessLog).To(HaveKeyWithValue("actor", principal))
			Expect(accessLog).NotTo(HaveKey("calling_peer"))
			Expect(requestSecurityString(accessLog, "client_ip")).NotTo(BeEmpty())

			logCapture.Reset()
			authorizeResp := authenticatedGET("/oauth2/authorize", map[string]string{"User-Agent": "request-security-authorize-audit/1.0"})
			defer func() { _ = authorizeResp.Body.Close() }()

			Expect(authorizeResp.StatusCode).To(Equal(http.StatusBadRequest))
			authorizeTraceID := requestSecurityTraceID(authorizeResp)
			auditLog := requestSecurityOAuth2AuditLog(capturedRecordsForTraceID(authorizeTraceID), "/oauth2/authorize")
			Expect(auditLog).To(HaveKeyWithValue("trace_id", authorizeTraceID))
			Expect(auditLog).To(HaveKeyWithValue("actor", principal))
		})

		// Scenario 1.2 from specs/033-request-security-context/spec.md
		It("should expose the finalized security context to the downstream service layer", func() {
			resp := delegatedTokenExchange("", map[string]string{"User-Agent": "request-security-service/1.0"})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			capturedRecordsForTraceID(traceID)

			serviceObservation, ok := requestSecurityObservationByLayer(requestObserver.Snapshot(), "service")
			Expect(ok).To(BeTrue())
			Expect(serviceObservation.TraceID).To(Equal(traceID))
			Expect(serviceObservation.Actor).To(Equal(principal))
			Expect(serviceObservation.CallingPeer).To(Equal("privileged-gateway-client"))
			Expect(serviceObservation.ClientIP).NotTo(BeEmpty())
			Expect(serviceObservation.UserAgent).To(Equal("request-security-service/1.0"))
			Expect(serviceObservation.RequestMethod).To(Equal(http.MethodPost))
			Expect(serviceObservation.RequestTarget).To(Equal("/oauth2/token"))
		})

		// Scenario 1.3 from specs/033-request-security-context/spec.md
		It("should expose the same finalized security context to the repository layer", func() {
			resp := delegatedTokenExchange("", map[string]string{"User-Agent": "request-security-repository/1.0"})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			capturedRecordsForTraceID(traceID)

			observations := requestObserver.Snapshot()
			serviceObservation, serviceOK := requestSecurityObservationByLayer(observations, "service")
			repositoryObservation, repositoryOK := requestSecurityObservationByLayer(observations, "repository")
			Expect(serviceOK).To(BeTrue())
			Expect(repositoryOK).To(BeTrue())
			Expect(repositoryObservation.TraceID).To(Equal(traceID))
			Expect(repositoryObservation.Actor).To(Equal(serviceObservation.Actor))
			Expect(repositoryObservation.CallingPeer).To(Equal(serviceObservation.CallingPeer))
			Expect(repositoryObservation.ClientIP).To(Equal(serviceObservation.ClientIP))
			Expect(repositoryObservation.UserAgent).To(Equal(serviceObservation.UserAgent))
			Expect(repositoryObservation.RequestMethod).To(Equal(serviceObservation.RequestMethod))
			Expect(repositoryObservation.RequestTarget).To(Equal(serviceObservation.RequestTarget))
		})

		// Scenario 1.4 from specs/033-request-security-context/spec.md
		It("should attach the security context automatically on public metadata endpoints without endpoint-specific capture code", func() {
			resp := publicGET("/.well-known/oauth-authorization-server", map[string]string{"User-Agent": "request-security-public/1.0"})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			records := capturedRecordsForTraceID(traceID)
			accessLog := requestSecurityAccessLog(records, "/.well-known/oauth-authorization-server")
			Expect(accessLog).To(HaveKeyWithValue("trace_id", traceID))
			Expect(accessLog).To(HaveKeyWithValue("actor", "anonymous"))
			Expect(accessLog).NotTo(HaveKey("calling_peer"))
			Expect(requestSecurityString(accessLog, "client_ip")).NotTo(BeEmpty())
		})

		// Scenario 1.5 from specs/033-request-security-context/spec.md
		It("should keep actor and calling_peer separate when a delegated request has a distinct caller", func() {
			resp := delegatedTokenExchange("", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			records := capturedRecordsForTraceID(traceID)
			Expect(records).To(ContainElement(HaveKeyWithValue("actor", principal)))
			Expect(records).To(ContainElement(HaveKeyWithValue("calling_peer", "privileged-gateway-client")))

			observations := requestObserver.Snapshot()
			serviceObservation, serviceOK := requestSecurityObservationByLayer(observations, "service")
			repositoryObservation, repositoryOK := requestSecurityObservationByLayer(observations, "repository")
			Expect(serviceOK).To(BeTrue())
			Expect(repositoryOK).To(BeTrue())
			Expect(serviceObservation.Actor).To(Equal(principal))
			Expect(serviceObservation.CallingPeer).To(Equal("privileged-gateway-client"))
			Expect(repositoryObservation.Actor).To(Equal(principal))
			Expect(repositoryObservation.CallingPeer).To(Equal("privileged-gateway-client"))
		})
	})

	Context("when an authenticated request crosses the admin HTTP perimeter", func() {
		// Both perimeters share http.NewHandler (internal/adapters/http/server.go),
		// so this asserts the admin server captures the same trace/actor/context and
		// emits the traceresponse header — guarding against an admin-only wiring
		// regression that end-user coverage alone would not catch.
		It("should establish trace_id, actor, and traceresponse on admin routes", func() {
			adminApp, err := serverFactory.BuildAppWithTracerProvider(testStorage, tracerProvider)
			Expect(err).NotTo(HaveOccurred())

			adminServer, err := bootstrap.NewTestServerV2(
				adminApp,
				logger,
				bootstrap.WithServerType(bootstrap.ServerTypeAdmin),
				bootstrap.WithRequestSecurityObserver(requestObserver),
			)
			Expect(err).NotTo(HaveOccurred())
			defer adminServer.Close()
			logCapture.Reset()

			resp, err := adminServer.DirectRequest(
				http.MethodGet,
				"/api/agents",
				principal,
				map[string]string{"User-Agent": "request-security-admin/1.0"},
				nil,
			)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/api/agents")
			Expect(accessLog).To(HaveKeyWithValue("trace_id", traceID))
			Expect(accessLog).To(HaveKeyWithValue("actor", principal))
			Expect(accessLog).NotTo(HaveKey("calling_peer"))
			Expect(requestSecurityString(accessLog, "client_ip")).NotTo(BeEmpty())
		})
	})

	Context("when tracing headers are present or absent", func() {
		const inboundTraceID = "1234567890abcdef1234567890abcdef"
		const inboundSpanID = "1111111111111111"

		// Scenario 2.1 from specs/033-request-security-context/spec.md
		It("should reuse an inbound traceparent as the request trace_id", func() {
			resp := authenticatedGET("/api/me", map[string]string{
				"traceparent": fmt.Sprintf("00-%s-%s-01", inboundTraceID, inboundSpanID),
			})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(requestSecurityTraceID(resp)).To(Equal(inboundTraceID))
			capturedRecordsForTraceID(inboundTraceID)
		})

		// Edge case from specs/033-request-security-context/spec.md (Multiple or duplicate inbound trace identifiers)
		It("should choose the first valid duplicate traceparent value when multiple identifiers are supplied", func() {
			const firstValidTraceID = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			const secondValidTraceID = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

			resp := rawGET("/api/me", principal, map[string][]string{
				"traceparent": {
					"00-not-a-valid-traceparent",
					fmt.Sprintf("00-%s-%s-01", firstValidTraceID, inboundSpanID),
					fmt.Sprintf("00-%s-2222222222222222-01", secondValidTraceID),
				},
			})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(requestSecurityTraceID(resp)).To(Equal(firstValidTraceID))
			capturedRecordsForTraceID(firstValidTraceID)
		})

		// Edge case from specs/033-request-security-context/spec.md (Multiple or duplicate inbound trace identifiers)
		It("should generate a fresh trace_id when duplicate traceparent values are all invalid", func() {
			resp := rawGET("/api/me", principal, map[string][]string{
				"traceparent": {
					"00-not-a-valid-traceparent",
					"still-not-valid",
				},
			})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			Expect(traceID).To(MatchRegexp(`^[0-9a-f]{32}$`))
			capturedRecordsForTraceID(traceID)
		})

		// Scenario 2.2 from specs/033-request-security-context/spec.md
		It("should generate a unique trace_id when none is supplied", func() {
			resp := authenticatedGET("/api/me", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			Expect(traceID).To(MatchRegexp(`^[0-9a-f]{32}$`))
			capturedRecordsForTraceID(traceID)
		})

		// Scenario 2.3 from specs/033-request-security-context/spec.md
		It("should stamp the same trace_id onto every structured log entry emitted during the request", func() {
			resp := delegatedTokenExchange("", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			records := capturedRecordsForTraceID(traceID)
			Expect(len(records)).To(BeNumerically(">", 1))
		})

		// Scenario 2.4 from specs/033-request-security-context/spec.md
		It("should return the request trace_id in the traceresponse header", func() {
			resp := authenticatedGET("/api/me", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))
			traceID := requestSecurityTraceID(resp)
			accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/api/me")
			Expect(accessLog).To(HaveKeyWithValue("trace_id", traceID))
		})

		// Scenario 2.5 from specs/033-request-security-context/spec.md
		It("should keep trace_id bound to its originating actor across concurrent requests", func() {
			type concurrentCase struct {
				principalValue string
				traceID        string
				traceparent    string
			}
			cases := []concurrentCase{
				{
					principalValue: "alice@example.com",
					traceID:        "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					traceparent:    "00-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa-bbbbbbbbbbbbbbbb-01",
				},
				{
					principalValue: "bob@example.com",
					traceID:        "cccccccccccccccccccccccccccccccc",
					traceparent:    "00-cccccccccccccccccccccccccccccccc-dddddddddddddddd-01",
				},
			}

			var wg sync.WaitGroup
			responses := make([]*http.Response, len(cases))
			for i, c := range cases {
				wg.Add(1)
				go func(idx int, cc concurrentCase) {
					defer GinkgoRecover()
					defer wg.Done()
					responses[idx] = rawGET("/api/me", cc.principalValue, map[string][]string{
						"traceparent": {cc.traceparent},
					})
				}(i, c)
			}
			wg.Wait()

			// Each response must carry back its own distinct inbound trace ID.
			expectedActorByTrace := map[string]string{}
			for i, c := range cases {
				Expect(responses[i]).NotTo(BeNil())
				Expect(responses[i].StatusCode).To(Equal(http.StatusOK))
				Expect(requestSecurityTraceID(responses[i])).To(Equal(c.traceID))
				_ = responses[i].Body.Close()
				expectedActorByTrace[c.traceID] = c.principalValue
			}

			records, err := logCapture.Records()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).NotTo(BeEmpty())

			// Isolation: every log line carries exactly one of the two request trace IDs.
			for _, record := range records {
				traceID := requestSecurityString(record, "trace_id")
				Expect(traceID).NotTo(BeEmpty(), fmt.Sprintf("record missing trace_id: %#v", record))
				_, known := expectedActorByTrace[traceID]
				Expect(known).To(BeTrue(), fmt.Sprintf("unexpected trace_id %q on record %#v", traceID, record))
			}

			// Correlation: each request's access log must pair its trace_id with the
			// actor of the request that originated it. A full trace swap between the
			// two concurrent requests would surface here as a mismatched actor.
			accessLogs := requestSecurityAccessLogs(records, "/api/me")
			Expect(accessLogs).To(HaveLen(len(cases)))
			seenTraceIDs := map[string]struct{}{}
			for _, accessLog := range accessLogs {
				traceID := requestSecurityString(accessLog, "trace_id")
				Expect(accessLog).To(HaveKeyWithValue("actor", expectedActorByTrace[traceID]),
					fmt.Sprintf("trace_id %q must stay bound to its originating actor", traceID))
				seenTraceIDs[traceID] = struct{}{}
			}
			Expect(seenTraceIDs).To(HaveLen(len(cases)))
		})
	})

	Context("when requests degrade to anonymous context", func() {
		// Scenario 3.1 from specs/033-request-security-context/spec.md
		It("should record actor as anonymous for unauthenticated public requests", func() {
			resp := publicGET("/.well-known/oauth-authorization-server", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/.well-known/oauth-authorization-server")
			Expect(accessLog).To(HaveKeyWithValue("actor", "anonymous"))
			Expect(accessLog).NotTo(HaveKey("calling_peer"))
		})

		// Scenario 3.2 from specs/033-request-security-context/spec.md
		It("should degrade gracefully without rejecting requests that have missing or garbled metadata", func() {
			resp := publicGET("/.well-known/oauth-authorization-server", map[string]string{
				"User-Agent":      "",
				"X-Forwarded-For": "garbled-forwarded-for",
			})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/.well-known/oauth-authorization-server")
			Expect(accessLog).To(HaveKeyWithValue("actor", "anonymous"))
			Expect(accessLog).NotTo(HaveKey("calling_peer"))
			Expect(requestSecurityString(accessLog, "client_ip")).NotTo(Equal("garbled-forwarded-for"))
		})

		// Scenario 3.3 from specs/033-request-security-context/spec.md
		It("should still reject unauthenticated access to protected routes with the existing 401 behavior", func() {
			resp := publicGET("/api/consent/agents", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))
		})

		// Scenario 3.4 from specs/033-request-security-context/spec.md
		It("should still emit a trace_id for degraded health requests", func() {
			resp := publicGET("/health", nil)
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))
		})
	})

	Context("when client IP and secret-bearing inputs are asserted", func() {
		// Edge case from specs/033-request-security-context/spec.md (Spoofed forwarding headers)
		It("should ignore forged X-Forwarded-For when proxy trust is disabled", func() {
			resp := authenticatedGET("/api/me", map[string]string{"X-Forwarded-For": "1.2.3.4"})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			traceID := requestSecurityTraceID(resp)
			accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/api/me")
			Expect(requestSecurityString(accessLog, "client_ip")).NotTo(BeEmpty())
			Expect(accessLog).NotTo(HaveKeyWithValue("client_ip", "1.2.3.4"))
		})

		Context("and trusted proxy support is enabled", func() {
			BeforeEach(func() {
				config = fixtures.EnableTrustedProxy(config, "X-Forwarded-For")
				serverFactory = bootstrap.NewServerFactory(config, logger)
			})

			// Edge case from specs/033-request-security-context/spec.md (Trusted proxy configuration)
			It("should use the right-most forwarded entry when proxy trust is enabled", func() {
				resp := authenticatedGET("/api/me", map[string]string{"X-Forwarded-For": "9.9.9.9, 10.0.0.5"})
				defer func() { _ = resp.Body.Close() }()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))
				traceID := requestSecurityTraceID(resp)
				accessLog := requestSecurityAccessLog(capturedRecordsForTraceID(traceID), "/api/me")
				Expect(accessLog).To(HaveKeyWithValue("client_ip", "10.0.0.5"))
			})
		})

		// Edge case from specs/033-request-security-context/spec.md (Sensitive material)
		It("should keep query params, authorization headers, and cookies out of logs and propagated context", func() {
			secretCode := "secret-authorization-code"
			bearerSecret := "Bearer super-secret-token"
			cookieSecret := "session_cookie=super-secret-cookie"

			resp := delegatedTokenExchange("/oauth2/token?code="+secretCode, map[string]string{
				"Authorization": bearerSecret,
				"Cookie":        cookieSecret,
				"User-Agent":    "request-security-secret-check/1.0",
			})
			defer func() { _ = resp.Body.Close() }()

			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("traceresponse")).To(MatchRegexp(`^00-[0-9a-f]{32}-[0-9a-f]{16}-[0-9a-f]{2}$`))

			traceID := requestSecurityTraceID(resp)
			records := capturedRecordsForTraceID(traceID)
			serviceObservation, ok := requestSecurityObservationByLayer(requestObserver.Snapshot(), "service")
			Expect(ok).To(BeTrue())
			Expect(serviceObservation.RequestTarget).To(Equal("/oauth2/token"))

			combinedObservedFields := strings.Join([]string{
				serviceObservation.TraceID,
				serviceObservation.Actor,
				serviceObservation.CallingPeer,
				serviceObservation.ClientIP,
				serviceObservation.UserAgent,
				serviceObservation.RequestTarget,
			}, "\n")
			Expect(combinedObservedFields).NotTo(ContainSubstring(secretCode))
			Expect(combinedObservedFields).NotTo(ContainSubstring(bearerSecret))
			Expect(combinedObservedFields).NotTo(ContainSubstring(cookieSecret))

			Expect(logCapture.Raw()).NotTo(ContainSubstring(secretCode))
			Expect(logCapture.Raw()).NotTo(ContainSubstring(bearerSecret))
			Expect(logCapture.Raw()).NotTo(ContainSubstring(cookieSecret))
			Expect(requestSecurityAccessLogs(records, "/oauth2/token")).NotTo(BeEmpty())
		})
	})
})
