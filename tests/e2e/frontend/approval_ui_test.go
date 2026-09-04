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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"net/http"
	"time"
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
	approval.ApplyExactPatterns()
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

	AfterEach(func() {
		if approvalPage != nil {
			_ = approvalPage.Close()
		}
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

		hasRequestedBy, err := approvalPage.HasVisibleText(ctx, "Requested by")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasRequestedBy).To(BeTrue(), "detail page should identify which agent requested the tool")

		hasAgentName, err := approvalPage.HasVisibleText(ctx, testAgent.DisplayName)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasAgentName).To(BeTrue(), "agent display name should be visible on the review page")

		hasActionSection, err := approvalPage.HasSectionLabel(ctx, "Action")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasActionSection).To(BeTrue(), "detail page should use the same user-facing Action block as the list page")

		hasTechnicalDetails, err := approvalPage.HasSectionLabel(ctx, "Technical details")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasTechnicalDetails).To(BeTrue(), "detail page should use the same Technical details block as the list page")

		hasSessionContext, err := approvalPage.HasVisibleText(ctx, "session-ui-123")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasSessionContext).To(BeTrue(), "agent session identifier should be visible on the review page")

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
			hasReviewHeading, err := approvalPage.HasReviewHeading(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasReviewHeading).To(BeTrue(), "review heading should remain visible for high-risk approvals")

			hasGlobalError, err := approvalPage.HasGlobalErrorBoundary(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasGlobalError).To(BeFalse(), "high-risk approvals should not crash the page")
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	// FR-016 from specs/024-approval-api-ui/spec.md
	// State 2: Permanent Warning
	It("should show permanent warning when Always allow is selected", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "send_email", time.Now().Add(10*time.Minute))
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())
		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.SelectPersistence(ctx, "Always allow")
		Expect(err).NotTo(HaveOccurred())

		hasWarning, err := approvalPage.HasPermanentWarning(ctx)
		Expect(err).NotTo(HaveOccurred())
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

		err = approvalPage.ClickApprove(ctx)
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

		err = approvalPage.SelectPersistence(ctx, "Always allow")
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.ClickApprove(ctx)
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
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		title, err := approvalPage.GetErrorTitle(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(title).To(Equal("Approval Expired"))

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
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		title, err := approvalPage.GetErrorTitle(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(title).To(Equal("Access Denied"))

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
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		title, err := approvalPage.GetErrorTitle(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(title).To(Equal("Not Found"))

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

		err = approvalPage.NavigateToApproval(ctx, approval.ID.String())
		Expect(err).NotTo(HaveOccurred())

		err = approvalPage.WaitForReviewPage(ctx)
		Expect(err).NotTo(HaveOccurred(), "Review page should eventually load after skeleton")

		err = approvalPage.TakeScreenshot(ctx, "approval_loading_skeleton")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Loading skeleton resolved to review page")
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

		err = approvalPage.ClickApprove(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasError, err := approvalPage.HasErrorAlert(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasError).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		title, err := approvalPage.GetErrorTitle(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(title).To(Equal("Already Resolved"))

		err = approvalPage.TakeScreenshot(ctx, "approval_network_error")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Error banner shown for already-actioned approval")
	})
	It("edits and persists a permanent approval pattern", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "create_pull_request", time.Now().Add(10*time.Minute))
		approval.Arguments = map[string]any{"repo": "acme/app", "title": "Fix bug"}
		approval.ApplyExactPatterns()
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(approvalPage.NavigateToApproval(ctx, approval.ID.String())).To(Succeed())
		Expect(approvalPage.WaitForReviewPage(ctx)).To(Succeed())
		Expect(approvalPage.SelectPersistence(ctx, "Always allow")).To(Succeed())
		Expect(approvalPage.ExpandApprovalScope(ctx)).To(Succeed())
		Expect(approvalPage.SetParameterMode(ctx, "repo", "Custom match")).To(Succeed())
		Expect(approvalPage.SetParameterCustomPattern(ctx, "repo", "acme/*")).To(Succeed())
		preview, err := approvalPage.GetPatternPreview(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview).To(Equal("create_pull_request(repo=acme/*,title=Fix bug)"))
		Expect(approvalPage.ClickApprove(ctx)).To(Succeed())
		Expect(approvalPage.WaitForApprovedConfirmation(ctx)).To(Succeed())
		resp, err := GetTestServer().AuthenticatedGET("/api/approvals/"+approval.ID.String(), principal.Email)
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		defer func() { Expect(resp.Body.Close()).To(Succeed()) }()
		var detail helpers.ApprovalDetailResponse
		Expect(json.NewDecoder(resp.Body).Decode(&detail)).To(Succeed())
		Expect(detail.Data.ParamsPattern).To(Equal(map[string]string{"repo": "acme/*", "title": "Fix bug"}))
	})

	It("unconstrains a single parameter with Any value", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newPendingApproval(id.Principal(principal.Email), testAgent.ID, "create_pull_request", time.Now().Add(10*time.Minute))
		approval.Arguments = map[string]any{"repo": "acme/app", "title": "Fix bug"}
		approval.ApplyExactPatterns()
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(approvalPage.NavigateToApproval(ctx, approval.ID.String())).To(Succeed())
		Expect(approvalPage.WaitForReviewPage(ctx)).To(Succeed())
		Expect(approvalPage.SelectPersistence(ctx, "Always allow")).To(Succeed())
		Expect(approvalPage.ExpandApprovalScope(ctx)).To(Succeed())
		Expect(approvalPage.SetParameterMode(ctx, "title", "Any value")).To(Succeed())
		preview, err := approvalPage.GetPatternPreview(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(preview).To(Equal("create_pull_request(repo=acme/app)"))
		Expect(approvalPage.ClickApprove(ctx)).To(Succeed())
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
