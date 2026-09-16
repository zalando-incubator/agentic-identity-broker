package extproc_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
)

var _ = Describe("ExtProc Approval Cache Sync", func() {
	// US1-S1 from specs/026-extproc-approval-sync/spec.md.
	It("creates a pending approval with dual authentication and returns its URL elicitation", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(response).NotTo(BeNil())
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))
		Expect(elicitation(response)).To(Equal("https://broker.example/approvals/approval-1"))

		request, found := brokerRequest(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.Method == http.MethodPost && request.Path == "/api/approvals"
		})
		Expect(found).To(BeTrue())
		Expect(request.Authorization).To(Equal("Bearer " + fixtures.ValidBearerToken))
		Expect(request.ClientAssertion).NotTo(BeEmpty())
		var body struct {
			Metadata struct {
				AgentSessionID string `json:"agent_session_id"`
			} `json:"metadata"`
			ToolName string `json:"tool_name"`
		}
		Expect(json.Unmarshal(request.Body, &body)).To(Succeed())
		Expect(body.ToolName).To(Equal("create_issue"))
		Expect(body.Metadata.AgentSessionID).To(Equal("agent-session"))
	})

	// FR-013 from specs/026-extproc-approval-sync/spec.md.
	It("extracts approval session identity from a configured header independently of the MCP session", func() {
		broker := bootstrap.NewMockApprovalBroker()
		env, _, stop := startApprovalEnvironmentWithSessionHeader(broker, time.Minute, "X-Agent-Session")
		defer stop()
		waitForBootstrap(broker)

		client, conn := env.NewExtProcClient()
		defer conn.Close() //nolint:errcheck
		headers := approvalRequest().
			WithHeader("Mcp-Session-Id", "mcp-session").
			WithHeader("X-Agent-Session", "agent-session").
			BuildWithMetadata()
		_, response := helpers.SendHeadersAndBody(context.Background(), client, headers, toolCallBody("create_issue"))
		Expect(elicitation(response)).To(Equal("https://broker.example/approvals/approval-1"))

		targetedRead, found := brokerRequest(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.Method == http.MethodGet && request.Query.Get("principal") == "alice@example.com"
		})
		Expect(found).To(BeTrue())
		Expect(targetedRead.Query["agent_session_id"]).To(ContainElement("agent-session"))
		create, found := brokerRequest(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.Method == http.MethodPost && request.Path == "/api/approvals"
		})
		Expect(found).To(BeTrue())
		var requestBody struct {
			Metadata struct {
				AgentSessionID string `json:"agent_session_id"`
				MCPSessionID   string `json:"mcp_session_id"`
			} `json:"metadata"`
		}
		Expect(json.Unmarshal(create.Body, &requestBody)).To(Succeed())
		Expect(requestBody.Metadata.AgentSessionID).To(Equal("agent-session"))
		Expect(requestBody.Metadata.MCPSessionID).To(Equal("mcp-session"))
	})

	// US1-S2 from specs/026-extproc-approval-sync/spec.md.
	It("refreshes a newly approved record with a targeted principal read before creating again", func() {
		broker := bootstrap.NewMockApprovalBroker()
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)
		broker.SetSnapshot(`"v2"`, approvalPair("permanent"))

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(broker.TargetedReadCount()).To(Equal(1))
		Expect(broker.CreateCount()).To(Equal(0))
		request, found := brokerRequest(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.Method == http.MethodGet && request.Query.Get("principal") == "alice@example.com"
		})
		Expect(found).To(BeTrue())
		Expect(request.Query["agent_session_id"]).To(ContainElement("agent-session"))
		Expect(request.Authorization).NotTo(BeEmpty())
	})

	// US1-S3 from specs/026-extproc-approval-sync/spec.md.
	It("forwards a cached session approval in its active agent session", func() {
		pair := approvalPair("session")
		sessionID := "agent-session"
		pair.Approvals[0].AgentSessionID = &sessionID
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, pair)
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", sessionID)
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(broker.TargetedReadCount()).To(Equal(0))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// US1-S4 and SC-004 from specs/026-extproc-approval-sync/spec.md.
	It("forwards a cached permanent approval in a different agent session without broker calls", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, approvalPair("permanent"))
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)
		baselineRequests := len(broker.Requests())

		_, response := sendApprovalToolCall(env, "create_issue", "different-agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(broker.Requests()).To(HaveLen(baselineRequests))
	})

	// US1-S5 and SC-006 from specs/026-extproc-approval-sync/spec.md.
	It("consumes a cached once approval and elicits again on the next matching call", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, approvalPair("once"))
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, first := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(first).To(helpers.BeForwardedRequestBody())
		Expect(broker.ConsumeCount()).To(Equal(1))
		_, second := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(second)).To(Equal(uint32(http.StatusOK)))
		Expect(elicitation(second)).To(Equal("https://broker.example/approvals/approval-1"))
		Expect(broker.CreateCount()).To(Equal(1))
	})

	// US1-S6 and FR-009 from specs/026-extproc-approval-sync/spec.md.
	It("evicts an idle session approval before another call can reuse it", func() {
		pair := approvalPair("session")
		oldSessionID := "old-session"
		pair.Approvals[0].AgentSessionID = &oldSessionID
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, pair)
		env, _, stop := startApprovalEnvironment(broker, 40*time.Millisecond)
		defer stop()
		waitForBootstrap(broker)

		_, initial := sendApprovalToolCall(env, "create_issue", oldSessionID)
		Expect(initial).To(helpers.BeForwardedRequestBody())
		idleUntil := time.Now().Add(100 * time.Millisecond)
		Eventually(func() bool { return time.Now().After(idleUntil) }, time.Second, 10*time.Millisecond).Should(BeTrue())
		broker.SetSnapshot(`"v3"`)

		_, oldSessionResponse := sendApprovalToolCall(env, "create_issue", oldSessionID)
		Expect(elicitation(oldSessionResponse)).To(Equal("https://broker.example/approvals/approval-1"))
		_, newSessionResponse := sendApprovalToolCall(env, "create_issue", "new-session")
		Expect(elicitation(newSessionResponse)).To(Equal("https://broker.example/approvals/approval-2"))
		Expect(broker.CreateCount()).To(Equal(2))
	})

	// US2-S1 and SC-003 from specs/026-extproc-approval-sync/spec.md.
	It("denies a policy-rejected tool without creating an approval", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "delete_repository", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// US2-S2 from specs/026-extproc-approval-sync/spec.md.
	It("forwards a policy-allowed tool without approval broker calls", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()
		waitForBootstrap(broker)
		baselineRequests := len(broker.Requests())

		_, response := sendApprovalToolCall(env, "list_repositories", "agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(broker.Requests()).To(HaveLen(baselineRequests))
	})

	// US2-S3 from specs/026-extproc-approval-sync/spec.md.
	It("enters the approval gate only after the permission-set boundary permits the tool", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))
		Expect(broker.CreateCount()).To(Equal(1))
	})

	// US3-S1 and SC-005 from specs/026-extproc-approval-sync/spec.md.
	It("serves a pre-seeded permanent approval from bootstrap", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, approvalPair("permanent"))
		startedAt := time.Now()
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(time.Since(startedAt)).To(BeNumerically("<", 5*time.Second))
		Expect(broker.TargetedReadCount()).To(Equal(0))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// US3-S2 from specs/026-extproc-approval-sync/spec.md.
	It("starts when bootstrap fails and denies if the later authoritative read also fails", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetBootstrapStatus(http.StatusServiceUnavailable)
		broker.SetTargetedReadStatus(http.StatusServiceUnavailable)
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(broker.TargetedReadCount()).To(Equal(1))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// US3-S3 from specs/026-extproc-approval-sync/spec.md.
	It("reuses the bootstrap ETag in the first long-poll request", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v42"`)
		_, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForLongPoll(broker, 1)

		request, found := brokerRequest(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.IfNoneMatch != ""
		})
		Expect(found).To(BeTrue())
		Expect(request.IfNoneMatch).To(Equal(`"v42"`))
		Expect(request.LongPollTimeout).To(Equal("30"))
	})

	// US3-S4 from specs/026-extproc-approval-sync/spec.md.
	It("performs the first unknown-pair lookup before creating an approval", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))
		requests := broker.Requests()
		firstRead, firstCreate := -1, -1
		for index, request := range requests {
			if request.Method == http.MethodGet && request.Query.Get("principal") == "alice@example.com" {
				firstRead = index
			}
			if request.Method == http.MethodPost && request.Path == "/api/approvals" {
				firstCreate = index
			}
		}
		Expect(firstRead).To(BeNumerically(">=", 0))
		Expect(firstCreate).To(BeNumerically(">", firstRead))
	})

	// US4-S1 from specs/026-extproc-approval-sync/spec.md.
	It("immediately re-polls with the same ETag after a 304 response", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v3"`)
		_, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForLongPoll(broker, 1)
		broker.ReplyToNextLongPoll(bootstrap.ApprovalBrokerSyncResponse{Status: http.StatusNotModified})
		waitForLongPoll(broker, 2)

		longPolls := matchingBrokerRequests(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.IfNoneMatch != ""
		})
		Expect(len(longPolls)).To(BeNumerically(">=", 2))
		Expect(longPolls[0].IfNoneMatch).To(Equal(`"v3"`))
		Expect(longPolls[1].IfNoneMatch).To(Equal(`"v3"`))
	})

	// US4-S2 and SC-002 from specs/026-extproc-approval-sync/spec.md.
	It("propagates an approved record from the long poll within two seconds", func() {
		broker := bootstrap.NewMockApprovalBroker()
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForLongPoll(broker, 1)
		startedAt := time.Now()
		broker.PublishSnapshot(`"v2"`, approvalPair("permanent"))
		waitForLongPoll(broker, 2)
		baselineReads := broker.TargetedReadCount()

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(time.Since(startedAt)).To(BeNumerically("<", 2*time.Second))
		Expect(broker.TargetedReadCount()).To(Equal(baselineReads))
	})

	// US4-S3 and SC-007 from specs/026-extproc-approval-sync/spec.md.
	It("reconnects after a dropped long-poll connection with the last ETag", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v7"`)
		_, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForLongPoll(broker, 1)
		broker.ReplyToNextLongPoll(bootstrap.ApprovalBrokerSyncResponse{Abort: true})
		waitForLongPoll(broker, 2)

		longPolls := matchingBrokerRequests(broker, func(request bootstrap.ApprovalBrokerRequest) bool {
			return request.IfNoneMatch != ""
		})
		Expect(longPolls[len(longPolls)-1].IfNoneMatch).To(Equal(`"v7"`))
	})

	// US4-S4 from specs/026-extproc-approval-sync/spec.md.
	It("replaces a stale full snapshot so a revoked approval cannot survive", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v1"`, approvalPair("permanent"))
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForLongPoll(broker, 1)
		broker.PublishSnapshot(`"v2"`)
		waitForLongPoll(broker, 2)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))
		Expect(broker.TargetedReadCount()).To(Equal(1))
		Expect(broker.CreateCount()).To(Equal(1))
	})

	// US5-S5 and SC-010 from specs/026-extproc-approval-sync/spec.md.
	It("selects the later-approved equally specific record regardless of approval ID ordering", func() {
		broker := bootstrap.NewMockApprovalBroker()
		older, newer := approvalPair("permanent"), approvalPair("permanent")
		older.Approvals[0].ID = "a-older"
		newer.Approvals[0].ID = "z-newer"
		older.Approvals[0].ApprovedAt = timePointer(time.Now().Add(-time.Hour))
		newer.Approvals[0].ApprovedAt = timePointer(time.Now())
		newer.Approvals[0].ParamsPattern = map[string]string{"repo": "acme/app"}
		broker.SetSnapshot(`"v2"`, older, newer)
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(response).To(helpers.BeForwardedRequestBody())
		Expect(broker.ConsumeCount()).To(Equal(0))
		Expect(broker.TargetedReadCount()).To(Equal(0))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("never fabricates an elicitation when approval creation fails", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetCreateResponse(http.StatusServiceUnavailable, "")
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(helpers.ExtractImmediateResponseBody(response)).To(ContainSubstring("approval could not be initiated"))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("uses the broker URL returned by idempotent duplicate creation", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetCreateResponse(http.StatusOK, "https://broker.example/approvals/existing")
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(elicitation(response)).To(Equal("https://broker.example/approvals/existing"))
		Expect(broker.CreateCount()).To(Equal(1))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("denies when the broker rejects the request subject token", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetCreateResponse(http.StatusUnauthorized, "")
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(broker.CreateCount()).To(Equal(1))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("denies batched approval-required calls and instructs a standalone retry", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()
		body := []byte(`[{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"repo":"acme/app"}}}]`)
		client, conn := env.NewExtProcClient()
		defer conn.Close() //nolint:errcheck
		_, response := helpers.SendHeadersAndBody(context.Background(), client, approvalRequest().BuildWithMetadata(), body)

		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(helpers.ExtractImmediateResponseBody(response)).To(ContainSubstring("standalone"))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("denies deferred CIBA decisions without creating an approval", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()

		_, response := sendApprovalToolCall(env, "elevated_action", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("denies without broker calls when exchange omits approval identity", func() {
		env, broker, stop := startApprovalEnvironment(nil, time.Minute)
		defer stop()
		env.MockTokenExchange.WithApprovalIdentity("", "")

		_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
		Expect(helpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(broker.TargetedReadCount()).To(Equal(0))
		Expect(broker.CreateCount()).To(Equal(0))
	})

	// Edge case from specs/026-extproc-approval-sync/spec.md.
	It("allows only one same-instance request to consume a once approval", func() {
		broker := bootstrap.NewMockApprovalBroker()
		broker.SetSnapshot(`"v2"`, approvalPair("once"))
		env, _, stop := startApprovalEnvironment(broker, time.Minute)
		defer stop()
		waitForBootstrap(broker)

		responses := make(chan *extprocv3.ProcessingResponse, 2)
		var waitGroup sync.WaitGroup
		for range 2 {
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				_, response := sendApprovalToolCall(env, "create_issue", "agent-session")
				responses <- response
			}()
		}
		waitGroup.Wait()
		close(responses)

		forwarded, elicited := 0, 0
		for response := range responses {
			if helpers.ExtractImmediateResponseStatus(response) == uint32(http.StatusOK) {
				elicited++
			} else {
				forwarded++
			}
		}
		Expect(forwarded).To(Equal(1))
		Expect(elicited).To(Equal(1))
		Expect(broker.ConsumeCount()).To(Equal(1))
	})
})

func startApprovalEnvironment(broker *bootstrap.MockApprovalBroker, idleTTL time.Duration) (*bootstrap.TestEnvironment, *bootstrap.MockApprovalBroker, func()) {
	return startApprovalEnvironmentWithSessionHeader(broker, idleTTL, "Mcp-Session-Id")
}

func startApprovalEnvironmentWithSessionHeader(broker *bootstrap.MockApprovalBroker, idleTTL time.Duration, sessionHeader string) (*bootstrap.TestEnvironment, *bootstrap.MockApprovalBroker, func()) {
	if broker == nil {
		broker = bootstrap.NewMockApprovalBroker()
	}
	cfg := opaEnabledConfig(policyPath("approval_required.rego"))
	cfg.ToolApprovals = extprocconfig.ToolApprovalsConfig{
		Enabled:                true,
		LongPollTimeoutSeconds: 30,
		ApprovalCacheIdleTTL:   idleTTL,
		MaxStaleness:           time.Minute,
		RequestTimeout:         time.Second,
	}
	cfg.Sessions.Extraction.HTTPHeader = sessionHeader
	env := bootstrap.NewTestEnvironmentWithApprovalBroker(cfg, opaLogger, broker)
	env.Start()
	env.MockTokenExchange.WithGrantedPermissionSets(map[string][]string{"github-issues": {"22222222-2222-2222-2222-222222222222"}})
	return env, broker, func() {
		env.Stop()
		broker.Stop()
	}
}

func waitForBootstrap(broker *bootstrap.MockApprovalBroker) {
	Eventually(broker.BootstrapRequestCount, time.Second).Should(BeNumerically(">=", 1))
}

func waitForLongPoll(broker *bootstrap.MockApprovalBroker, count int) {
	Eventually(broker.LongPollRequestCount, 2*time.Second).Should(BeNumerically(">=", count))
}

func sendApprovalToolCall(env *bootstrap.TestEnvironment, toolName, sessionID string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	client, conn := env.NewExtProcClient()
	defer conn.Close() //nolint:errcheck
	headers := approvalRequest().WithHeader("Mcp-Session-Id", sessionID).BuildWithMetadata()
	return helpers.SendHeadersAndBody(context.Background(), client, headers, toolCallBody(toolName))
}

func elicitation(response *extprocv3.ProcessingResponse) string {
	var body struct {
		ID    float64 `json:"id"`
		Error struct {
			Code int `json:"code"`
			Data struct {
				Elicitations []struct {
					URL string `json:"url"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	Expect(json.Unmarshal(response.GetImmediateResponse().Body, &body)).To(Succeed())
	Expect(body.ID).To(Equal(float64(1)))
	Expect(body.Error.Code).To(Equal(-32042))
	Expect(body.Error.Data.Elicitations).To(HaveLen(1))
	return body.Error.Data.Elicitations[0].URL
}

func approvalPair(persistence string) bootstrap.ApprovalBrokerPair {
	return bootstrap.ApprovalBrokerPair{
		Principal: "alice@example.com",
		AgentID:   "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		Approvals: []bootstrap.ApprovalBrokerRecord{{
			ID:            "approval-1",
			ToolPattern:   "create_issue",
			ParamsPattern: map[string]string{"repo": "acme/app"},
			Status:        "approved",
			Persistence:   timeStringPointer(persistence),
			ApprovedAt:    timePointer(time.Now().UTC()),
		}},
	}
}

func approvalRequest() *helpers.ProcessingRequestBuilder {
	return helpers.NewRequestHeaders().
		WithPath(fixtures.ValidResourceURI).
		WithBearerToken(fixtures.ValidBearerToken).
		WithTokenExchangeMetadata(fixtures.ValidBearerToken, fixtures.ValidResourceURI).
		WithHeader(":method", "POST").
		WithAgentgatewayProtocol("mcp")
}

func brokerRequest(broker *bootstrap.MockApprovalBroker, match func(bootstrap.ApprovalBrokerRequest) bool) (bootstrap.ApprovalBrokerRequest, bool) {
	for _, request := range broker.Requests() {
		if match(request) {
			return request, true
		}
	}
	return bootstrap.ApprovalBrokerRequest{}, false
}

func matchingBrokerRequests(broker *bootstrap.MockApprovalBroker, match func(bootstrap.ApprovalBrokerRequest) bool) []bootstrap.ApprovalBrokerRequest {
	var matching []bootstrap.ApprovalBrokerRequest
	for _, request := range broker.Requests() {
		if match(request) {
			matching = append(matching, request)
		}
	}
	return matching
}

func timePointer(value time.Time) *time.Time {
	return &value
}

func timeStringPointer(value string) *string {
	return &value
}
