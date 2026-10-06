// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the tool approval review UI (feature 024-approval-api-ui).
//
// 10 UI states tested:
//  1. Pending review (US1-S1)
//  2. Permanent warning (FR-016)
//  3. Confirmed once (US1-S2)
//  4. Confirmed permanent (US1-S4)
//  5. Denied (US2-S1)
//  6. Expired (US1-S6)
//  7. Forbidden (US1-S5)
//  8. Not found (Edge Case)
//  9. Loading skeleton (Edge Case)
//  10. Network error (Edge Case)
package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/google/uuid"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newPendingApproval creates a pending tool approval record for seeding test storage.
func newPendingApproval(principal id.Principal, agentID id.AgentID, toolName string, expiresAt time.Time) *storage.ToolApproval {
	approval := &storage.ToolApproval{
		ID:              id.NewApprovalID(),
		Principal:       principal,
		AgentID:         agentID,
		GatewayClientID: "test-gw-client",
		ToolName:        toolName,
		Arguments:       map[string]any{"path": "/tmp/test", "recursive": true},
		ArgumentsHash:   "abc123hash",
		Description:     "Delete files from the filesystem recursively",
		RiskLevel:       "critical",
		Status:          storage.ApprovalStatusPending,
		ApprovalURL:     "http://localhost/approvals/test",
		CreatedAt:       time.Now(),
		ExpiresAt:       expiresAt,
	}
	Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
	return approval
}

func stringPtr(value string) *string {
	return &value
}

// Approval UI tests verify the tool approval review page renders correctly
// for all 10 distinct UI states specified in the feature spec.
var _ = Describe("Approval UI", func() {
	var (
		ctx          context.Context
		approvalPage *pages.ApprovalPage
		testAgent    *storage.Agent
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a test agent - approval UI shows agent display name
		testAgent = fixtures.ValidAgent()
		err := GetTestStorage().Agents().Create(ctx, testAgent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		// Initialize page object
		approvalPage = pages.NewApprovalPage(GetTestPage(), GetFrontendURL())
	})

	// Scenario US1-S1 from specs/024-approval-api-ui/spec.md
	// State 1: Pending Review
	It("should display pending approval with user-facing context, session context, and persistence choices", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "delete_files", time.Now().Add(10*time.Minute))
		approval.AgentSessionID = stringPtr("session-ui-123")
		approval.MCPSessionID = stringPtr("mcp-ui-123")
		approval.ToolInvocationID = stringPtr("invoke-ui-123")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred(), "Failed to create pending approval")

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to approval page")

		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred(), "Review page should be visible")

		Expect(readV2(approvalPage.ToolName(ctx))).To(Equal(approval.ToolName), "the exact tool must be identified before a decision")
		Expect(readV2(approvalPage.AgentName(ctx))).To(Equal(testAgent.DisplayName))
		Expect(readV2(approvalPage.ActingUser(ctx))).To(Equal(principal.Email))
		Expect(readV2(approvalPage.ArgumentRows(ctx))).To(Equal(map[string]string{"path": "/tmp/test", "recursive": "true"}))
		Expect(readV2(approvalPage.ApprovalScopeText(ctx))).To(ContainSubstring(approval.ToolName))
		Expect(readV2(approvalPage.HasVisibleText(ctx, approval.Description))).To(BeTrue(), "the requesting action is explained")

		Expect(approvalPage.OpenSessionContext(ctx)).To(Succeed(), "session context opens on demand")
		sessionContext := readV2(approvalPage.SessionContextText(ctx))
		Expect(sessionContext).To(ContainSubstring("session-ui-123"), "agent session identifier must remain available on review")
		Expect(sessionContext).To(ContainSubstring("mcp-ui-123"), "MCP session identifier must remain available on review")
		Expect(sessionContext).To(ContainSubstring("invoke-ui-123"), "tool invocation identifier must remain available on review")

		hasRisk, err := approvalPage.HasRiskBadge(ctx, "Critical")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasRisk).To(BeTrue(), "high risk badge should be visible")

		err = approvalPage.TakeScreenshot(ctx, "approval_pending_review")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Pending approval displayed with full review context")
	})

	// Scenario US1-S1 from specs/024-approval-api-ui/spec.md
	// Regression: high-risk approvals must not crash the review page.
	It("should render a high-risk approval without hitting the global error boundary", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "dangerous_tool", time.Now().Add(10*time.Minute))
		approval.RiskLevel = "high"
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred(), "Failed to create high-risk approval")

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred(), "Failed to navigate to approval page")

		Eventually(func(g Gomega) {
			identityVisible, err := approvalPage.HasReviewIdentity(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(identityVisible).To(BeTrue(), "the tool identity remains visible for high-risk approvals")

			hasGlobalError, err := approvalPage.HasGlobalErrorBoundary(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasGlobalError).To(BeFalse(), "high-risk approvals should not crash the page")
		}).WithPolling(500 * time.Millisecond).Should(Succeed())
		Expect(readV2(approvalPage.ToolName(ctx))).To(Equal(approval.ToolName))
		Expect(readV2(approvalPage.RiskLabel(ctx))).To(Equal("High"))
	})

	// FR-016 from specs/024-approval-api-ui/spec.md
	// State 2: Permanent Warning
	It("should show the permanent warning in the Always… scope editor", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "send_email", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(approvalPage.OpenApproveOptions(ctx)).To(Succeed())
		Expect(approvalPage.ChooseRememberDuration(ctx, "Always…")).To(Succeed())

		hasWarning, err := approvalPage.HasPermanentWarning(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Status).To(Equal(storage.ApprovalStatusPending), "opening the editor must not record approval")
		Expect(hasWarning).To(BeTrue(), "Permanent warning should be visible")

		err = approvalPage.TakeScreenshot(ctx, "approval_permanent_warning")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Permanent warning displayed correctly")
	})

	// Scenario US1-S2 from specs/024-approval-api-ui/spec.md
	// State 3: Confirmed Once
	It("should show approval confirmation after approving once", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "read_file", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.ClickApproveOnce(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.WaitForApprovedConfirmation(ctx)
		Expect(err).NotTo(HaveOccurred(), "Approved confirmation should be visible")

		hasOnce, err := approvalPage.HasPersistenceText(ctx, "once")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasOnce).To(BeTrue(), "Confirmation should mention once persistence")

		hasClose, err := approvalPage.HasCloseMessage(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasClose).To(BeTrue(), "Close message should be visible")

		err = approvalPage.TakeScreenshot(ctx, "approval_confirmed_once")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Approval confirmed with once persistence")
	})

	// Scenario US1-S4 from specs/024-approval-api-ui/spec.md
	// State 4: Confirmed Permanent
	It("should show approval confirmation after approving permanently", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "write_file", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		Expect(approvalPage.OpenApproveOptions(ctx)).To(Succeed())
		Expect(approvalPage.ChooseRememberDuration(ctx, "Always…")).To(Succeed())

		err = approvalPage.ConfirmRememberedApproval(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.WaitForApprovedConfirmation(ctx)
		Expect(err).NotTo(HaveOccurred(), "Approved confirmation should be visible")

		hasPermanent, err := approvalPage.HasPersistenceText(ctx, "permanent")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasPermanent).To(BeTrue(), "Confirmation should mention permanent persistence")

		err = approvalPage.TakeScreenshot(ctx, "approval_confirmed_permanent")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Approval confirmed with permanent persistence")
	})

	// Scenario US2-S1 from specs/024-approval-api-ui/spec.md
	// State 5: Denied
	It("should show denial confirmation after denying", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "execute_command", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.ClickDeny(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.WaitForDeniedConfirmation(ctx)
		Expect(err).NotTo(HaveOccurred(), "Denied confirmation should be visible")

		hasClose, err := approvalPage.HasCloseMessage(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasClose).To(BeTrue(), "Close message should be visible")

		err = approvalPage.TakeScreenshot(ctx, "approval_denied")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Denial confirmed")
	})

	// Scenario US1-S6 from specs/024-approval-api-ui/spec.md
	// State 6: Expired
	It("should show expired error when approval has passed its TTL", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "stale_tool", time.Now().Add(-1*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasError, err := approvalPage.HasErrorAlert(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasError).To(BeTrue())
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		hasRetry, err := approvalPage.HasRetryButton(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasRetry).To(BeFalse(), "Expired error should not have retry button")

		err = approvalPage.TakeScreenshot(ctx, "approval_expired_error")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Expired approval shows error message")
	})

	// Scenario US1-S5 from specs/024-approval-api-ui/spec.md
	// State 7: Forbidden
	It("should show forbidden error when approval belongs to another user", func() {
		otherPrincipal := id.Principal("other-user@example.com")
		approval := newPendingApproval(otherPrincipal, testAgent.ID, "secret_tool", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasError, err := approvalPage.HasErrorAlert(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasError).To(BeTrue())
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = approvalPage.TakeScreenshot(ctx, "approval_forbidden_error")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Forbidden error shown")
	})

	// Edge Case from specs/024-approval-api-ui/spec.md
	// State 8: Not Found
	It("should show not found error for unknown approval ID", func() {
		fakeID := uuid.New().String()
		err := approvalPage.NavigateToApproval(ctx, fakeID)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasError, err := approvalPage.HasErrorAlert(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasError).To(BeTrue())
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = approvalPage.TakeScreenshot(ctx, "approval_not_found_error")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Not found error shown")
	})

	// Edge Case from specs/024-approval-api-ui/spec.md
	// State 9: Loading Skeleton
	It("should show loading skeleton while approval is being fetched", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "slow_tool", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		requested, release, result := make(chan struct{}), make(chan struct{}), make(chan error, 1)
		var released sync.Once
		defer released.Do(func() { close(release) })
		Expect(GetTestPage().Route("**/api/approvals/"+approval.ID.String(), func(route playwright.Route) {
			if route.Request().Method() != http.MethodGet {
				result <- route.Continue()
				return
			}
			close(requested)
			go func() {
				<-release
				result <- route.Continue()
			}()
		})).To(Succeed())
		Expect(approvalPage.BeginApprovalLoad(ctx, approval.ID.String())).To(Succeed())
		Eventually(requested).WithTimeout(10 * time.Second).Should(BeClosed())
		Eventually(func() bool { return readV2(approvalPage.IsLoading(ctx)) }).WithTimeout(5*time.Second).Should(BeTrue(), "the pending detail request displays the approval skeleton")
		released.Do(func() { close(release) })
		Eventually(result).WithTimeout(10 * time.Second).Should(Receive(BeNil()))
		Expect(approvalPage.WaitForReviewPage(ctx)).To(Succeed(), "the review appears after the detail request resolves")
		Expect(readV2(approvalPage.IsLoading(ctx))).To(BeFalse())
	})

	// Edge Case from specs/024-approval-api-ui/spec.md
	// State 10: Network Error (Already Actioned)
	It("should show error banner when trying to approve an already-actioned approval", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "double_approve", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetTestStorage().ToolApprovals().Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistenceOnce, ToolPattern: approval.ToolPattern, ParamsPattern: approval.ParamsPattern}, time.Now())
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.ClickApproveOnce(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasError, err := approvalPage.HasErrorAlert(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasError).To(BeTrue())
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = approvalPage.TakeScreenshot(ctx, "approval_network_error")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Error banner shown for already-actioned approval")
	})
	// US7-S6 from specs/024-approval-api-ui/spec.md
	It("should edit and persist a permanent parameter scope", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "create_pull_request", time.Now().Add(10*time.Minute))
		approval.Arguments = map[string]any{"repo": "acme/app", "title": "Fix bug"}
		Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(approvalPage.NavigateToApproval(ctx, approval.ID.String())).To(Succeed())
		Expect(approvalPage.WaitForReviewPage(ctx)).To(Succeed())
		Expect(approvalPage.OpenApproveOptions(ctx)).To(Succeed())
		Expect(approvalPage.ChooseRememberDuration(ctx, "Always…")).To(Succeed())
		Expect(approvalPage.ExpandApprovalScope(ctx)).To(Succeed())
		hasToolRule, err := approvalPage.HasVisibleText(ctx, "Tool matching rule")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasToolRule).To(BeFalse())
		hasExactTool, err := approvalPage.HasVisibleText(ctx, "Only the tool create_pull_request")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasExactTool).To(BeTrue())
		Expect(approvalPage.SetParameterMode(ctx, "repo", "Custom match")).To(Succeed())
		Expect(approvalPage.SetParameterCustomPattern(ctx, "repo", "acme/*")).To(Succeed())
		Eventually(func(g Gomega) {
			preview, err := approvalPage.GetPatternPreview(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(preview).To(Equal("create_pull_request(repo=acme/*,title=Fix bug)"))
		}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())
		Expect(approvalPage.ConfirmRememberedApproval(ctx)).To(Succeed())
		Expect(approvalPage.WaitForApprovedConfirmation(ctx)).To(Succeed())
		resp, err := GetTestServer().AuthenticatedGET("/api/approvals/"+approval.ID.String(), principal.Email)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		var detail helpers.ApprovalDetailResponse
		Expect(json.NewDecoder(resp.Body).Decode(&detail)).To(Succeed())
		Expect(detail.Data.ParamsPattern).To(Equal(map[string]string{"repo": "acme/*", "title": "Fix bug"}))
	})

	// US7-S3 from specs/024-approval-api-ui/spec.md
	It("should unconstrain a single parameter with any value", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "create_pull_request", time.Now().Add(10*time.Minute))
		approval.Arguments = map[string]any{"repo": "acme/app", "title": "Fix bug"}
		Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(approvalPage.NavigateToApproval(ctx, approval.ID.String())).To(Succeed())
		Expect(approvalPage.WaitForReviewPage(ctx)).To(Succeed())
		Expect(approvalPage.OpenApproveOptions(ctx)).To(Succeed())
		Expect(approvalPage.ChooseRememberDuration(ctx, "Always…")).To(Succeed())
		Expect(approvalPage.ExpandApprovalScope(ctx)).To(Succeed())
		Expect(approvalPage.SetParameterMode(ctx, "title", "Any value")).To(Succeed())
		Eventually(func(g Gomega) {
			preview, err := approvalPage.GetPatternPreview(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(preview).To(Equal("create_pull_request(repo=acme/app)"))
		}).WithTimeout(5 * time.Second).WithPolling(100 * time.Millisecond).Should(Succeed())
		Expect(approvalPage.ConfirmRememberedApproval(ctx)).To(Succeed())
		Expect(approvalPage.WaitForApprovedConfirmation(ctx)).To(Succeed())
		resp, err := GetTestServer().AuthenticatedGET("/api/approvals/"+approval.ID.String(), principal.Email)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		var detail helpers.ApprovalDetailResponse
		Expect(json.NewDecoder(resp.Body).Decode(&detail)).To(Succeed())
		Expect(detail.Data.ParamsPattern).To(Equal(map[string]string{"repo": "acme/app"}))
	})
})
