package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/grpc"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	storagedomain "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	extprochelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const approvalJourneySessionID = "approval-journey-session"

var _ = Describe("ExtProc Approval Journey", func() {
	var (
		broker             *bootstrap.TestServer
		storageFactory     *bootstrap.StorageFactory
		serverFactory      *bootstrap.ServerFactory
		testStorage        *storageadapter.Adapter
		logger             *slog.Logger
		mockUpstream       *helpers.MockUpstreamOAuth2Server
		agent              *storagedomain.Agent
		principal          string
		machineAuth        *helpers.ApprovalRequestAuthFixture
		subjectToken       string
		extprocEnvironment *bootstrap.ExtProcEnvironment
		grpcClient         extprocv3.ExternalProcessorClient
		grpcConnection     *grpc.ClientConn
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelDebug)
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		config := fixtures.OAuth2ConfigWithTokenExchange(mockUpstream.URL())
		config.Approvals.SyncCoalesceWindow = 0

		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).NotTo(HaveOccurred())

		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).NotTo(HaveOccurred())
		broker, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).NotTo(HaveOccurred())

		principal = fixtures.DefaultPrincipal().String()
		agent = fixtures.ValidAgent()
		ctx := context.Background()
		Expect(testStorage.Agents().Create(ctx, agent)).To(Succeed())

		githubService := fixtures.GitHubService()
		githubService.Endpoints.TokenEndpoint = mockUpstream.URL() + "/oauth/token"
		githubService.Endpoints.AuthorizeEndpoint = mockUpstream.URL() + "/oauth/authorize"
		Expect(testStorage.Services().Create(ctx, githubService)).To(Succeed())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage, githubService.ID)).To(Succeed())
		Expect(testStorage.UserGrants().Create(ctx, fixtures.ActiveGrant(principal, agent.ID.String(), githubService.ID.String(), []string{"repo", "user"}))).To(Succeed())
		Expect(testStorage.UserSessions().Create(ctx, fixtures.GitHubSessionForPrincipal(principal))).To(Succeed())

		machineAuth, err = helpers.NewApprovalRequestAuthFixture(mockUpstream, agent.ID)
		Expect(err).NotTo(HaveOccurred())
		subjectToken, err = machineAuth.SubjectToken(principal)
		Expect(err).NotTo(HaveOccurred())

		policyPath, err := filepath.Abs("fixtures/policies/approval_journey.rego")
		Expect(err).NotTo(HaveOccurred())
		extprocEnvironment, err = bootstrap.NewExtProcEnvironment(broker.BaseURL(), machineAuth.ClientAssertion, policyPath, logger)
		Expect(err).NotTo(HaveOccurred())
		grpcClient, grpcConnection = extprochelpers.ConnectToExtProc(extprocEnvironment.Address)
	})

	AfterEach(func() {
		if grpcConnection != nil {
			_ = grpcConnection.Close()
		}
		if extprocEnvironment != nil {
			extprocEnvironment.Close()
		}
		if broker != nil {
			broker.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if storageFactory != nil && testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// US1-S2, US1-S5, US4-S2 and SC-002/SC-006 from specs/026-extproc-approval-sync/spec.md
	It("creates, synchronizes, consumes, and re-elicits a one-time approval", func() {
		_, first := approvalJourneyToolCall(grpcClient, subjectToken)
		approvalURL := approvalJourneyElicitationURL(first)
		Expect(first).NotTo(extprochelpers.BeForwardedRequestBody())

		sync := approvalJourneySync(broker, machineAuth.ClientAssertion)
		Expect(sync.Data.Pairs).To(HaveLen(1))
		Expect(sync.Data.Pairs[0].Principal).To(Equal(principal))
		Expect(sync.Data.Pairs[0].AgentID).To(Equal(agent.ID.String()))
		Expect(sync.Data.Pairs[0].Approvals).To(HaveLen(1))
		pending := sync.Data.Pairs[0].Approvals[0]
		Expect(pending.Status).To(Equal("pending"))

		detail := approvalJourneyDetail(broker, pending.ID, principal)
		Expect(approvalURL).To(Equal(detail.Data.ApprovalURL))

		response, err := postJSON(broker, fmt.Sprintf("/api/approvals/%s/approve", pending.ID), principal, helpers.ApproveRequest{Persistence: "once"})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		approveResponse := decodeJSON[helpers.ApproveResponse](response)
		Expect(approveResponse.Data.Persistence).To(Equal("once"))

		Eventually(func() *extprocv3.ProcessingResponse {
			_, bodyResponse := approvalJourneyToolCall(grpcClient, subjectToken)
			return bodyResponse
		}, 2*time.Second, 50*time.Millisecond).Should(extprochelpers.BeForwardedRequestBody())

		detail = approvalJourneyDetail(broker, pending.ID, principal)
		Expect(detail.Data.Consumed).To(BeTrue())

		_, third := approvalJourneyToolCall(grpcClient, subjectToken)
		Expect(approvalJourneyElicitationURL(third)).NotTo(BeEmpty())
	})

	// US4-S4 and FR-019 from specs/026-extproc-approval-sync/spec.md
	It("stops forwarding after a permanent approval is revoked", func() {
		_, first := approvalJourneyToolCall(grpcClient, subjectToken)
		_ = approvalJourneyElicitationURL(first)
		sync := approvalJourneySync(broker, machineAuth.ClientAssertion)
		Expect(sync.Data.Pairs).To(HaveLen(1))
		Expect(sync.Data.Pairs[0].Approvals).To(HaveLen(1))
		approvalID := sync.Data.Pairs[0].Approvals[0].ID

		response, err := postJSON(broker, fmt.Sprintf("/api/approvals/%s/approve", approvalID), principal, helpers.ApproveRequest{Persistence: "permanent"})
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		_ = decodeJSON[helpers.ApproveResponse](response)

		Eventually(func() *extprocv3.ProcessingResponse {
			_, bodyResponse := approvalJourneyToolCall(grpcClient, subjectToken)
			return bodyResponse
		}, 2*time.Second, 50*time.Millisecond).Should(extprochelpers.BeForwardedRequestBody())

		response, err = postJSON(broker, fmt.Sprintf("/api/approvals/%s/revoke", approvalID), principal, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		_ = decodeJSON[helpers.DenyResponse](response)

		Eventually(func() bool {
			_, bodyResponse := approvalJourneyToolCall(grpcClient, subjectToken)
			return approvalJourneyDeniedOrElicited(bodyResponse)
		}, 5*time.Second, 50*time.Millisecond).Should(BeTrue())
	})
})

func approvalJourneyToolCall(client extprocv3.ExternalProcessorClient, subjectToken string) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
	headers := extprochelpers.NewRequestHeaders().
		WithHeader(":method", http.MethodPost).
		WithPath("https://api.github.com").
		WithBearerToken(subjectToken).
		WithHeader("Mcp-Session-Id", approvalJourneySessionID).
		WithAgentgatewayProtocol("mcp").
		BuildWithMetadata()
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_issue","arguments":{"repo":"acme/app","title":"Approval journey"}}}`)
	return extprochelpers.SendHeadersAndBody(context.Background(), client, headers, body)
}

func approvalJourneyElicitationURL(response *extprocv3.ProcessingResponse) string {
	Expect(response).NotTo(BeNil())
	Expect(extprochelpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusOK)))

	var envelope struct {
		JSONRPC string `json:"jsonrpc"`
		Error   struct {
			Code int `json:"code"`
			Data struct {
				Elicitations []struct {
					URL string `json:"url"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	Expect(json.Unmarshal([]byte(extprochelpers.ExtractImmediateResponseBody(response)), &envelope)).To(Succeed())
	Expect(envelope.JSONRPC).To(Equal("2.0"))
	Expect(envelope.Error.Code).To(Equal(-32042))
	Expect(envelope.Error.Data.Elicitations).To(HaveLen(1))
	return envelope.Error.Data.Elicitations[0].URL
}

func approvalJourneySync(broker *bootstrap.TestServer, clientAssertion string) helpers.ApprovalSyncResponse {
	response, err := broker.DirectRequest(http.MethodGet, "/api/approvals", "", helpers.ApprovalSyncHeaders(clientAssertion), nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	return decodeJSON[helpers.ApprovalSyncResponse](response)
}

func approvalJourneyDetail(broker *bootstrap.TestServer, approvalID, principal string) helpers.ApprovalDetailResponse {
	response, err := broker.AuthenticatedGET(fmt.Sprintf("/api/approvals/%s", approvalID), principal)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusOK))
	return decodeJSON[helpers.ApprovalDetailResponse](response)
}

func approvalJourneyDeniedOrElicited(response *extprocv3.ProcessingResponse) bool {
	if response == nil {
		return false
	}
	if extprochelpers.ExtractImmediateResponseStatus(response) == uint32(http.StatusForbidden) {
		return true
	}
	if extprochelpers.ExtractImmediateResponseStatus(response) != uint32(http.StatusOK) {
		return false
	}

	var envelope struct {
		Error struct {
			Code int `json:"code"`
			Data struct {
				Elicitations []struct {
					URL string `json:"url"`
				} `json:"elicitations"`
			} `json:"data"`
		} `json:"error"`
	}
	return json.Unmarshal([]byte(extprochelpers.ExtractImmediateResponseBody(response)), &envelope) == nil &&
		envelope.Error.Code == -32042 &&
		len(envelope.Error.Data.Elicitations) == 1 &&
		envelope.Error.Data.Elicitations[0].URL != ""
}
