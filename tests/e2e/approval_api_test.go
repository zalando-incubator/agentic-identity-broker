package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gstruct"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func approvalTestConfig(upstreamURL string) *ports.Config {
	config := fixtures.OAuth2ConfigWithTokenExchange(upstreamURL)
	config.Approvals.SyncCoalesceWindow = 0
	config.Approvals.RateLimit.MaxRequestsPerMinute = 100
	return config
}

func postJSON(server *bootstrap.TestServer, path, principal string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	return server.AuthenticatedPOST(path, principal, "application/json", bytes.NewReader(data))
}

func postJSONWithHeaders(server *bootstrap.TestServer, path string, headers map[string]string, body any) (*http.Response, error) {
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}

	requestHeaders := map[string]string{"Content-Type": "application/json"}
	for key, value := range headers {
		requestHeaders[key] = value
	}

	return server.DirectRequest(http.MethodPost, path, "", requestHeaders, bytes.NewReader(data))
}

func subjectTokenFor(auth *helpers.ApprovalRequestAuthFixture, principal string) string {
	token, err := auth.SubjectToken(principal)
	Expect(err).NotTo(HaveOccurred())
	return token
}

func consumeApproval(server *bootstrap.TestServer, auth *helpers.ApprovalRequestAuthFixture, approvalID string, principal string) (*http.Response, error) {
	return postJSONWithHeaders(
		server,
		fmt.Sprintf("/api/approvals/%s/consume", approvalID),
		helpers.ApprovalSubjectTokenHeaders(subjectTokenFor(auth, principal)),
		nil,
	)
}

func decodeJSON[T any](resp *http.Response) T {
	var result T
	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred())
	_ = resp.Body.Close()
	err = json.Unmarshal(body, &result)
	Expect(err).NotTo(HaveOccurred(), "body was: %s", string(body))
	return result
}

// createPendingApprovalWithTraceparent creates a pending approval with W3C trace context.
func createPendingApprovalWithTraceparent(server *bootstrap.TestServer, auth *helpers.ApprovalRequestAuthFixture, principal, traceparent string, req helpers.CreateApprovalRequest) helpers.CreateApprovalResponse {
	headers := helpers.ApprovalCreateHeaders(subjectTokenFor(auth, principal), auth.ClientAssertion, auth.AgentID)
	headers["Traceparent"] = traceparent
	resp, err := postJSONWithHeaders(server, "/api/approvals", headers, req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusCreated), Equal(http.StatusOK)))
	return decodeJSON[helpers.CreateApprovalResponse](resp)
}

// createPendingApprovalWithRequest creates a pending approval via the machine-facing API.
func createPendingApprovalWithRequest(server *bootstrap.TestServer, auth *helpers.ApprovalRequestAuthFixture, principal string, req helpers.CreateApprovalRequest) helpers.CreateApprovalResponse {
	headers := helpers.ApprovalCreateHeaders(subjectTokenFor(auth, principal), auth.ClientAssertion, auth.AgentID)
	resp, err := postJSONWithHeaders(server, "/api/approvals", headers, req)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(SatisfyAny(Equal(http.StatusCreated), Equal(http.StatusOK)))
	return decodeJSON[helpers.CreateApprovalResponse](resp)
}

// createPendingApproval is a helper that creates a pending approval via the machine-facing API.
func createPendingApproval(server *bootstrap.TestServer, auth *helpers.ApprovalRequestAuthFixture, principal string, toolName string, args map[string]any) helpers.CreateApprovalResponse {
	return createPendingApprovalWithRequest(server, auth, principal, helpers.CreateApprovalRequest{
		Metadata: helpers.CreateApprovalMetadata{
			Description: fmt.Sprintf("Execute %s", toolName),
		},
		ToolName:  toolName,
		Arguments: args,
	})
}

var _ = Describe("Tool Approval API", func() {
	var (
		server         *bootstrap.TestServer
		testStorage    *storageadapter.Adapter
		storageFactory *bootstrap.StorageFactory
		serverFactory  *bootstrap.ServerFactory
		logger         *slog.Logger
		mockUpstream   *helpers.MockUpstreamOAuth2Server
		machineAuth    *helpers.ApprovalRequestAuthFixture
		agentID        id.AgentID
	)

	const (
		alicePrincipal = "alice@example.com"
		bobPrincipal   = "bob@example.com"
	)

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}))
		mockUpstream = helpers.NewMockUpstreamOAuth2Server()
		config := approvalTestConfig(mockUpstream.URL())
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// Seed an agent for testing
		agentID = id.NewAgentID()
		agentRepo := testStorage.Agents()
		err = agentRepo.Create(context.Background(), &storage.Agent{
			ID:             agentID,
			ClientID:       ptr.To(id.ClientID("code-assistant")),
			DisplayName:    "Code Assistant",
			Description:    "An AI coding agent",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			PermissionSets: fixtures.DefaultPermissionSets(),
		})
		Expect(err).ToNot(HaveOccurred())

		machineAuth, err = helpers.NewApprovalRequestAuthFixture(mockUpstream, agentID)
		Expect(err).ToNot(HaveOccurred())

		serverFactory = bootstrap.NewServerFactory(config, logger)
		appInstance, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if mockUpstream != nil {
			mockUpstream.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// =========================================================================
	// User Story 1 — User Approves a Pending Tool Call
	// =========================================================================
	Describe("US1: User Approves a Pending Tool Call", func() {
		// Scenario US1-S1: View pending approval with full context
		It("returns enriched approval details with agent context and canonical risk level (US1-S1)", func() {
			createResp := createPendingApprovalWithRequest(server, machineAuth, alicePrincipal, helpers.CreateApprovalRequest{
				Metadata: helpers.CreateApprovalMetadata{
					MCPSessionID:     "mcp-123",
					AgentSessionID:   "sess-xyz",
					ToolInvocationID: "inv-abc",
					Description:      "Create a pull request in acme/app",
				},
				ToolName:  "create_pull_request",
				Arguments: map[string]any{"repo": "acme/app", "title": "Fix bug"},
				RiskLevel: "dangerous",
			})

			resp, err := server.AuthenticatedGET(fmt.Sprintf("/api/approvals/%s", createResp.Data.ID), alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			detail := decodeJSON[helpers.ApprovalDetailResponse](resp)
			Expect(detail.Data).To(MatchFields(IgnoreExtras, Fields{
				"ID":               Equal(createResp.Data.ID),
				"Principal":        Equal(alicePrincipal),
				"AgentID":          Equal(agentID.String()),
				"AgentDisplayName": Equal("Code Assistant"),
				"Description":      Equal("Create a pull request in acme/app"),
				"RiskLevel":        Equal("critical"),
				"Status":           Equal("pending"),
				"MCPSessionID":     PointTo(Equal("mcp-123")),
				"AgentSessionID":   PointTo(Equal("sess-xyz")),
				"ToolInvocationID": PointTo(Equal("inv-abc")),
				"ToolName":         Equal("create_pull_request"),
			}))
			Expect(detail.Data.Arguments).To(HaveKeyWithValue("repo", "acme/app"))
			Expect(detail.Data.Arguments).To(HaveKeyWithValue("title", "Fix bug"))
		})

		// Coverage extension: GET /api/approvals/pending returns the same enriched detail DTO shape as GET /api/approvals/{id}.
		It("lists pending approvals with the enriched detail payload (FR-013)", func() {
			pendingResp := createPendingApprovalWithRequest(server, machineAuth, alicePrincipal, helpers.CreateApprovalRequest{
				Metadata: helpers.CreateApprovalMetadata{
					MCPSessionID:     "mcp-pending",
					AgentSessionID:   "sess-pending",
					ToolInvocationID: "inv-pending",
					Description:      "Delete stale branches",
				},
				ToolName:  "delete_branch",
				Arguments: map[string]any{"branch": "stale-feature"},
				RiskLevel: "low",
			})

			approvedResp := createPendingApproval(server, machineAuth, alicePrincipal, "run_tests", map[string]any{"suite": "unit"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", approvedResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = server.AuthenticatedGET("/api/approvals/pending", alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			listResp := decodeJSON[helpers.ApprovalListResponse](resp)
			Expect(listResp.Data).To(HaveLen(1))
			Expect(listResp.Data[0]).To(MatchFields(IgnoreExtras, Fields{
				"ID":               Equal(pendingResp.Data.ID),
				"Principal":        Equal(alicePrincipal),
				"AgentID":          Equal(agentID.String()),
				"AgentDisplayName": Equal("Code Assistant"),
				"Description":      Equal("Delete stale branches"),
				"RiskLevel":        Equal("low"),
				"Status":           Equal("pending"),
				"MCPSessionID":     PointTo(Equal("mcp-pending")),
				"AgentSessionID":   PointTo(Equal("sess-pending")),
				"ToolInvocationID": PointTo(Equal("inv-pending")),
				"ToolName":         Equal("delete_branch"),
			}))
			Expect(listResp.Data[0].Arguments).To(HaveKeyWithValue("branch", "stale-feature"))
		})

		// Scenario US1-S2: Approve once
		It("transitions to approved with persistence=once (US1-S2)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", map[string]any{"repo": "acme/app", "title": "Fix bug"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			approveResp := decodeJSON[helpers.ApproveResponse](resp)
			Expect(approveResp.Data.Status).To(Equal("approved"))
			Expect(approveResp.Data.Persistence).To(Equal("once"))
			Expect(approveResp.Data.ApprovedAt).NotTo(BeEmpty())
		})

		// Scenario US1-S3: Approve for session
		It("transitions to approved with persistence=session (US1-S3)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "run_tests", map[string]any{"suite": "unit"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "session"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			approveResp := decodeJSON[helpers.ApproveResponse](resp)
			Expect(approveResp.Data.Persistence).To(Equal("session"))
		})

		// Scenario US1-S4: Approve permanently
		It("transitions to approved with persistence=permanent (US1-S4)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "list_files", map[string]any{"dir": "/src"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			approveResp := decodeJSON[helpers.ApproveResponse](resp)
			Expect(approveResp.Data.Persistence).To(Equal("permanent"))
		})

		// Scenario US1-S5: 403 on principal mismatch
		It("returns 403 when principal does not match approval owner (US1-S5)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "delete_branch", map[string]any{"branch": "feature"})
			// Bob tries to approve Alice's approval
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				bobPrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		})

		// Scenario US1-S6: Expired approval returns 410
		It("returns 410 for expired approval (US1-S6)", func() {
			// Create approval with expired TTL — we need to go through storage directly
			// since the API enforces the configured TTL
			approvalIDVal := id.NewApprovalID()
			repo := testStorage.ToolApprovals()
			pastExpiry := time.Now().UTC().Add(-1 * time.Minute)
			approval := &storage.ToolApproval{
				ID:            approvalIDVal,
				Principal:     id.Principal(alicePrincipal),
				AgentID:       agentID,
				ToolName:      "expired_tool",
				Arguments:     map[string]any{},
				ArgumentsHash: "expired_hash",
				Status:        storage.ApprovalStatusPending,
				ApprovalURL:   "https://localhost/approvals/" + approvalIDVal.String(),
				CreatedAt:     pastExpiry.Add(-10 * time.Minute),
				ExpiresAt:     pastExpiry,
			}
			Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
			_, err := repo.Create(context.Background(), approval)
			Expect(err).NotTo(HaveOccurred())

			resp, httpErr := server.AuthenticatedGET(fmt.Sprintf("/api/approvals/%s", approvalIDVal.String()), alicePrincipal)
			Expect(httpErr).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusGone))
		})

		// Scenario US1-S7: OTel span linking (verified at service level in unit tests)
		It("completes approve flow with traceparent header (US1-S7)", func() {
			createResp := createPendingApprovalWithTraceparent(server, machineAuth, alicePrincipal,
				"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				helpers.CreateApprovalRequest{
					Metadata:  helpers.CreateApprovalMetadata{Description: "Create a pull request"},
					ToolName:  "create_pull_request",
					Arguments: map[string]any{"repo": "acme/app"},
				},
			)
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})
	})

	// =========================================================================
	// User Story 2 — User Denies a Pending Tool Call
	// =========================================================================
	Describe("US2: User Denies a Pending Tool Call", func() {
		// Scenario US2-S1: Deny
		It("transitions to denied state (US2-S1)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "delete_repo", map[string]any{"repo": "acme/app"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/deny", createResp.Data.ID),
				alicePrincipal, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			denyResp := decodeJSON[helpers.DenyResponse](resp)
			Expect(denyResp.Data.Status).To(Equal("denied"))
			Expect(denyResp.Data.DeniedAt).NotTo(BeEmpty())
		})

		// Scenario US2-S2: OTel trace context (verified at service level)
		It("completes deny flow with traceparent header (US2-S2)", func() {
			createResp := createPendingApprovalWithTraceparent(server, machineAuth, alicePrincipal,
				"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				helpers.CreateApprovalRequest{
					Metadata:  helpers.CreateApprovalMetadata{Description: "Trace a denied tool call"},
					ToolName:  "traced_tool",
					Arguments: map[string]any{},
				},
			)
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/deny", createResp.Data.ID),
				alicePrincipal, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		// Scenario US2-S3: Denied records are not deduplicated
		It("creates new approval after previous denial (US2-S3)", func() {
			// Create and deny
			create1 := createPendingApproval(server, machineAuth, alicePrincipal, "dangerous_tool", map[string]any{"action": "destroy"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/deny", create1.Data.ID),
				alicePrincipal, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Create again — should get a new record
			create2 := createPendingApproval(server, machineAuth, alicePrincipal, "dangerous_tool", map[string]any{"action": "destroy"})
			Expect(create2.Data.ID).NotTo(Equal(create1.Data.ID))
			Expect(create2.Data.Status).To(Equal("pending"))
		})

		// Scenario US2-S4: Deny permanently
		It("transitions to denied with persistence=permanent (US2-S4)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "blocked_tool", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/deny", createResp.Data.ID),
				alicePrincipal, helpers.DenyRequest{Persistence: "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			denyResp := decodeJSON[helpers.DenyResponse](resp)
			Expect(denyResp.Data.Status).To(Equal("denied"))
			Expect(denyResp.Data.Persistence).NotTo(BeNil())
			Expect(*denyResp.Data.Persistence).To(Equal("permanent"))
		})
	})

	// =========================================================================
	// User Story 3 — ExtProc Creates a Pending Approval
	// =========================================================================
	Describe("US3: ExtProc Creates a Pending Approval", func() {
		// Scenario US3-S1: Create pending approval
		It("creates pending approval with approval_url (US3-S1)", func() {
			createResp := createPendingApprovalWithTraceparent(server, machineAuth, alicePrincipal,
				"00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				helpers.CreateApprovalRequest{
					Metadata: helpers.CreateApprovalMetadata{
						MCPSessionID:     "mcp-123",
						AgentSessionID:   "sess-xyz",
						ToolInvocationID: "inv-abc",
						Description:      "Create a pull request in acme/app",
					},
					ToolName:  "create_pull_request",
					Arguments: map[string]any{"repo": "acme/app", "title": "Fix bug"},
				},
			)
			Expect(createResp.Data.ID).NotTo(BeEmpty())
			Expect(createResp.Data.Status).To(Equal("pending"))
			Expect(createResp.Data.ApprovalURL).NotTo(BeEmpty())
			Expect(createResp.Data.CreatedAt).NotTo(BeEmpty())
			approvalID, err := id.ParseApprovalID(createResp.Data.ID)
			Expect(err).NotTo(HaveOccurred())
			approval, err := testStorage.ToolApprovals().Get(context.Background(), approvalID)
			Expect(err).NotTo(HaveOccurred())
			Expect(approval.OpenTelemetryTraceparent).NotTo(BeNil())
			Expect(*approval.OpenTelemetryTraceparent).To(Equal("00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"))
		})

		// Scenario US3-S2: Idempotent create
		It("returns existing pending approval for duplicate request (US3-S2)", func() {
			args := map[string]any{"repo": "acme/app", "title": "Fix bug"}
			create1 := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", args)
			create2 := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", args)
			Expect(create2.Data.ID).To(Equal(create1.Data.ID))
		})

		// Scenario US3-S3: New approval after consumed one-time
		It("creates new approval after previous was consumed (US3-S3)", func() {
			args := map[string]any{"file": "main.go"}
			create1 := createPendingApproval(server, machineAuth, alicePrincipal, "read_file", args)
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", create1.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, create1.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			create2 := createPendingApproval(server, machineAuth, alicePrincipal, "read_file", args)
			Expect(create2.Data.ID).NotTo(Equal(create1.Data.ID))
		})

		// Scenario US3-S4: New approval after denial
		It("creates new approval after previous was denied (US3-S4)", func() {
			args := map[string]any{"cmd": "rm -rf"}
			create1 := createPendingApproval(server, machineAuth, alicePrincipal, "run_command", args)
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/deny", create1.Data.ID),
				alicePrincipal, map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			create2 := createPendingApproval(server, machineAuth, alicePrincipal, "run_command", args)
			Expect(create2.Data.ID).NotTo(Equal(create1.Data.ID))
		})

		// Scenario US3-S5: 401 without subject token
		It("returns 401 without Authorization header (US3-S5)", func() {
			req := helpers.CreateApprovalRequest{
				Metadata:  helpers.CreateApprovalMetadata{Description: "test"},
				ToolName:  "tool",
				Arguments: map[string]any{},
			}
			resp, err := postJSONWithHeaders(server, "/api/approvals", map[string]string{
				helpers.ApprovalClientAssertionHeader: machineAuth.ClientAssertion,
			}, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			errorResp := decodeJSON[helpers.ApprovalErrorResponse](resp)
			Expect(errorResp.Error).To(Equal("unauthorized"))
		})

		// Scenario US3-S6: 401 invalid client assertion
		It("returns 401 for invalid client assertion on create (US3-S6)", func() {
			req := helpers.CreateApprovalRequest{
				Metadata:  helpers.CreateApprovalMetadata{Description: "test"},
				ToolName:  "tool",
				Arguments: map[string]any{},
			}
			resp, err := postJSONWithHeaders(server, "/api/approvals", map[string]string{
				"Authorization":                       "Bearer " + subjectTokenFor(machineAuth, alicePrincipal),
				helpers.ApprovalClientAssertionHeader: machineAuth.InvalidClientAssertion,
			}, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			errorResp := decodeJSON[helpers.ApprovalErrorResponse](resp)
			Expect(errorResp.Error).To(Equal("unauthorized"))
		})

		// Scenario US3-S7: 401 principal extraction failure
		It("returns 401 when principal cannot be extracted from subject token (US3-S7)", func() {
			req := helpers.CreateApprovalRequest{
				Metadata:  helpers.CreateApprovalMetadata{Description: "test"},
				ToolName:  "tool",
				Arguments: map[string]any{},
			}
			badSubjectToken, err := machineAuth.SubjectTokenWithoutPrincipal()
			Expect(err).NotTo(HaveOccurred())
			resp, err := postJSONWithHeaders(server, "/api/approvals", map[string]string{
				"Authorization":                       "Bearer " + badSubjectToken,
				helpers.ApprovalClientAssertionHeader: machineAuth.ClientAssertion,
			}, req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			errorResp := decodeJSON[helpers.ApprovalErrorResponse](resp)
			Expect(errorResp.Error).To(Equal("unauthorized"))
		})
	})

	// =========================================================================
	// User Story 4 — ExtProc Syncs Approval State via Long-Poll
	// =========================================================================
	Describe("US4: ExtProc Syncs Approval State via Long-Poll", func() {
		// Scenario US4-S1: Initial sync without If-None-Match
		It("returns 200 with pairs and ETag on initial sync (US4-S1)", func() {
			createPendingApproval(server, machineAuth, alicePrincipal, "sync_test_tool", map[string]any{})
			resp, err := server.DirectRequest(http.MethodGet, "/api/approvals", "", helpers.ApprovalSyncHeaders(machineAuth.ClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("ETag")).NotTo(BeEmpty())
			syncResp := decodeJSON[helpers.ApprovalSyncResponse](resp)
			Expect(syncResp.Data.Pairs).NotTo(BeEmpty())
		})

		// Scenario US4-S2: 304 when no changes within timeout
		It("returns 304 when no changes within timeout (US4-S2)", func() {
			createPendingApproval(server, machineAuth, alicePrincipal, "timeout_test_tool", map[string]any{})

			resp, err := server.DirectRequest(http.MethodGet, "/api/approvals", "", helpers.ApprovalSyncHeaders(machineAuth.ClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			etag := resp.Header.Get("ETag")

			req, err := http.NewRequest(http.MethodGet, server.BaseURL()+"/api/approvals", nil)
			Expect(err).NotTo(HaveOccurred())
			req.Header.Set("Authorization", "Bearer "+machineAuth.ClientAssertion)
			req.Header.Set("If-None-Match", etag)
			req.Header.Set("X-Long-Poll-Timeout", "1")
			client := &http.Client{Timeout: 5 * time.Second}
			resp, err = client.Do(req)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusNotModified))
		})

		// Scenario US4-S3: Wake on approval state change
		It("returns 200 with updated data when approval changes (US4-S3)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "wake_test_tool", map[string]any{})

			resp, err := server.DirectRequest(http.MethodGet, "/api/approvals", "", helpers.ApprovalSyncHeaders(machineAuth.ClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			etag := resp.Header.Get("ETag")
			Expect(etag).NotTo(BeEmpty())

			done := make(chan *http.Response, 1)
			go func() {
				req, _ := http.NewRequest(http.MethodGet, server.BaseURL()+"/api/approvals", nil)
				req.Header.Set("Authorization", "Bearer "+machineAuth.ClientAssertion)
				req.Header.Set("If-None-Match", etag)
				req.Header.Set("X-Long-Poll-Timeout", "30")
				client := &http.Client{Timeout: 35 * time.Second}
				resp, _ := client.Do(req)
				done <- resp
			}()

			time.Sleep(200 * time.Millisecond)

			resp, err = postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			var pollResp *http.Response
			Eventually(done, 5*time.Second).Should(Receive(&pollResp))
			Expect(pollResp).NotTo(BeNil())
			Expect(pollResp.StatusCode).To(Equal(http.StatusOK))
			Expect(pollResp.Header.Get("ETag")).NotTo(Equal(etag))
			syncResp := decodeJSON[helpers.ApprovalSyncResponse](pollResp)
			found := false
			for _, pair := range syncResp.Data.Pairs {
				for _, approval := range pair.Approvals {
					if approval.ID == createResp.Data.ID {
						Expect(approval.Status).To(Equal("approved"))
						Expect(approval.Persistence).To(PointTo(Equal("once")))
						found = true
					}
				}
			}
			Expect(found).To(BeTrue())
		})

		// Scenario US4-S4: Invalid client assertion
		It("returns 401 for invalid client assertion on sync endpoint (US4-S4)", func() {
			resp, err := server.DirectRequest(http.MethodGet, "/api/approvals", "", helpers.ApprovalSyncHeaders(machineAuth.InvalidClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			errorResp := decodeJSON[helpers.ApprovalErrorResponse](resp)
			Expect(errorResp.Error).To(Equal("unauthorized"))
		})

		// Scenario US4-S5: Principal filter
		It("filters results by principal query parameter (US4-S5)", func() {
			createPendingApproval(server, machineAuth, alicePrincipal, "alice_tool", map[string]any{})
			createPendingApproval(server, machineAuth, bobPrincipal, "bob_tool", map[string]any{})

			resp, err := server.DirectRequest(http.MethodGet, "/api/approvals?principal="+alicePrincipal, "", helpers.ApprovalSyncHeaders(machineAuth.ClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			syncResp := decodeJSON[helpers.ApprovalSyncResponse](resp)
			for _, pair := range syncResp.Data.Pairs {
				Expect(pair.Principal).To(Equal(alicePrincipal))
			}
		})
	})

	// =========================================================================
	// User Story 5 — One-Time Approval Is Consumed After Use
	// =========================================================================
	Describe("US5: One-Time Approval Consumed After Use", func() {
		// Scenario US5-S1: Consume once-persistence approval
		It("marks once-persistence approval as consumed (US5-S1)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "consume_tool", map[string]any{})

			// Approve with once
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			consumeResp := decodeJSON[helpers.ConsumeResponse](resp)
			Expect(consumeResp.Data.Consumed).To(BeTrue())
			Expect(consumeResp.Data.ConsumedAt).NotTo(BeEmpty())
		})

		// Security scenario from ADR 018: Subject token principal must own the approval.
		It("returns 403 when another principal consumes an approval", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "consume_owner_tool", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, bobPrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusForbidden))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		// Scenario US5-S2: Idempotent consume
		It("returns 200 on idempotent consume (US5-S2)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "idempotent_consume_tool", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
		})

		// Security scenario from ADR 018: Missing subject token is rejected
		It("returns 401 without subject token on consume", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "consume_auth_tool", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "once"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = postJSONWithHeaders(server, fmt.Sprintf("/api/approvals/%s/consume", createResp.Data.ID), map[string]string{}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnauthorized))
			errorResp := decodeJSON[helpers.ApprovalErrorResponse](resp)
			Expect(errorResp.Error).To(Equal("unauthorized"))
		})

		// Scenario US5-S3: Cannot consume session/permanent
		It("returns 422 for non-once persistence (US5-S3)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "permanent_no_consume", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			resp, err = consumeApproval(server, machineAuth, createResp.Data.ID, alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnprocessableEntity))
		})
	})

	// =========================================================================
	// User Story 6 — Permanent Approvals Are Visible and Revocable
	// =========================================================================
	Describe("US6: Permanent Approvals Visible and Revocable", func() {
		// Scenario US6-S1: List permanent approvals
		It("lists permanent approvals on consent management page (US6-S1)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "create_issue", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// List permanent approvals
			resp, err = server.AuthenticatedGET("/api/approvals/permanent", alicePrincipal)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			listResp := decodeJSON[helpers.ApprovalListResponse](resp)
			Expect(listResp.Data).NotTo(BeEmpty())
		})

		// Scenario US6-S2: Revoke permanent approval
		It("revokes permanent approval (US6-S2)", func() {
			createResp := createPendingApproval(server, machineAuth, alicePrincipal, "revocable_tool", map[string]any{})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", createResp.Data.ID),
				alicePrincipal, helpers.ApproveRequest{Persistence: "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))

			// Revoke
			resp, err = postJSON(server, fmt.Sprintf("/api/approvals/%s/revoke", createResp.Data.ID),
				alicePrincipal, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			revokeResp := decodeJSON[helpers.DenyResponse](resp)
			Expect(revokeResp.Data.Status).To(Equal("denied"))
		})
	})

	Describe("when a user scopes an approval decision", func() {
		// US7-S2 from specs/024-approval-api-ui/spec.md
		It("should persist an edited parameter glob and expose it through sync", func() {
			create := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", map[string]any{"repo": "acme/app", "title": "Fix bug"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", create.Data.ID), alicePrincipal, map[string]any{
				"persistence": "permanent", "tool_pattern": "create_pull_request", "params_pattern": map[string]string{"repo": "acme/*"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			_ = decodeJSON[helpers.ApproveResponse](resp)
			resp, err = server.DirectRequest(http.MethodGet, "/api/approvals?principal="+alicePrincipal, "", helpers.ApprovalSyncHeaders(machineAuth.ClientAssertion), nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			sync := decodeJSON[helpers.ApprovalSyncResponse](resp)
			var found *helpers.ApprovalSummary
			for i := range sync.Data.Pairs {
				for j := range sync.Data.Pairs[i].Approvals {
					if sync.Data.Pairs[i].Approvals[j].ID == create.Data.ID {
						found = &sync.Data.Pairs[i].Approvals[j]
					}
				}
			}
			Expect(found).NotTo(BeNil())
			Expect(found.ToolPattern).To(Equal("create_pull_request"))
			Expect(found.ParamsPattern).To(Equal(map[string]string{"repo": "acme/*"}))
		})

		// US7-S1 from specs/024-approval-api-ui/spec.md
		It("should store exact coverage when pattern fields are omitted", func() {
			create := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", map[string]any{"repo": "acme/app"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", create.Data.ID), alicePrincipal, map[string]any{"persistence": "permanent"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp, err = server.DirectRequest(http.MethodGet, "/api/approvals/"+create.Data.ID, "", map[string]string{"X-Remote-User": alicePrincipal}, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			detail := decodeJSON[helpers.ApprovalDetailResponse](resp)
			Expect(detail.Data.ToolPattern).To(Equal("create_pull_request"))
			Expect(detail.Data.ParamsPattern).To(Equal(map[string]string{"repo": "acme/app"}))
		})

		// US7-S3 from specs/024-approval-api-ui/spec.md
		It("should store unconstrained coverage for an explicit empty params pattern", func() {
			create := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", map[string]any{"repo": "acme/app"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", create.Data.ID), alicePrincipal, map[string]any{"persistence": "permanent", "params_pattern": map[string]string{}})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			resp, err = server.DirectRequest(http.MethodGet, "/api/approvals/"+create.Data.ID, "", map[string]string{"X-Remote-User": alicePrincipal}, nil)
			Expect(err).NotTo(HaveOccurred())
			detail := decodeJSON[helpers.ApprovalDetailResponse](resp)
			Expect(detail.Data.ParamsPattern).To(BeEmpty())
		})

		// US7-S4 from specs/024-approval-api-ui/spec.md
		It("should reject a non-covering pattern and leave the approval pending", func() {
			create := createPendingApproval(server, machineAuth, alicePrincipal, "create_pull_request", map[string]any{"repo": "acme/app"})
			resp, err := postJSON(server, fmt.Sprintf("/api/approvals/%s/approve", create.Data.ID), alicePrincipal, map[string]any{"persistence": "permanent", "tool_pattern": "delete_repository"})
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusUnprocessableEntity))
			resp, err = server.DirectRequest(http.MethodGet, "/api/approvals/"+create.Data.ID, "", map[string]string{"X-Remote-User": alicePrincipal}, nil)
			Expect(err).NotTo(HaveOccurred())
			detail := decodeJSON[helpers.ApprovalDetailResponse](resp)
			Expect(detail.Data.Status).To(Equal("pending"))
		})
	})
})
