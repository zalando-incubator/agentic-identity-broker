// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file contains tests for the Tool Authorizations page — the approval list
// and actions view (feature 024-approval-api-ui).
//
// 5 UI states tested:
//  1. Empty state (no approvals)
//  2. Pending approvals list displayed
//  3. Approve action from list (with persistence)
//  4. Deny action from list
//  5. Permanent authorizations with revoke action
package e2e_test

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// newTestApproval creates a pending tool approval for testing the list page.
func newTestApproval(principal id.Principal, agentID id.AgentID, toolName, description string, riskLevel string) *storage.ToolApproval {
	approval := &storage.ToolApproval{
		ID:              id.NewApprovalID(),
		Principal:       principal,
		AgentID:         agentID,
		GatewayClientID: "test-gw-client",
		ToolName:        toolName,
		Arguments:       map[string]any{"path": "/tmp/test", "recursive": true},
		ArgumentsHash:   toolName + "-hash",
		Description:     description,
		RiskLevel:       riskLevel,
		Status:          storage.ApprovalStatusPending,
		ApprovalURL:     "http://localhost/approvals/test",
		CreatedAt:       time.Now(),
		ExpiresAt:       time.Now().Add(10 * time.Minute),
	}
	approval.ApplyExactPatterns()
	return approval
}

// Tool Authorizations page tests verify the approval list and management UI
// for the Tool Authorizations route (/approvals).
var _ = Describe("Tool Authorizations Page", func() {
	var (
		ctx       context.Context
		authzPage *pages.ToolAuthorizationsPage
		testAgent *storage.Agent
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a test agent
		testAgent = fixtures.ValidAgent()
		err := GetTestStorage().Agents().Create(ctx, testAgent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		// Initialize page object
		authzPage = pages.NewToolAuthorizationsPage(GetTestPage(), GetFrontendURL())
	})

	AfterEach(func() {
		if authzPage != nil {
			_ = authzPage.Close()
		}
	})

	// State 1: Empty state — no pending or permanent approvals
	It("should display empty state when no approvals exist", func() {
		err := authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasEmpty, err := authzPage.HasEmptyState(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasEmpty).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = authzPage.TakeScreenshot(ctx, "tool_authorizations_empty")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Empty state displayed correctly")
	})

	// State 2: Pending approvals list
	It("should display pending approvals with tool details and action buttons", func() {
		principal := fixtures.DefaultPrincipal()

		// Seed two pending approvals
		approval1 := newTestApproval(id.Principal(principal.Email), testAgent.ID, "delete_files", "Delete files recursively", "critical")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval1)
		Expect(err).NotTo(HaveOccurred())

		approval2 := newTestApproval(id.Principal(principal.Email), testAgent.ID, "send_email", "Send email to recipient", "medium")
		_, err = GetTestStorage().ToolApprovals().Create(ctx, approval2)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasPending, err := authzPage.HasPendingSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPending).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		// Verify tool names are displayed
		hasDelete, err := authzPage.HasToolName(ctx, "delete_files")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasDelete).To(BeTrue())

		hasSend, err := authzPage.HasToolName(ctx, "send_email")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasSend).To(BeTrue())

		// Verify risk badge
		hasRisk, err := authzPage.HasRiskBadge(ctx, "Critical")
		Expect(err).NotTo(HaveOccurred())
		Expect(hasRisk).To(BeTrue())

		err = authzPage.TakeScreenshot(ctx, "tool_authorizations_pending_list")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Pending approvals list displayed correctly")
	})

	// Scenario US1-S1 from specs/024-approval-api-ui/spec.md
	// Regression: high-risk pending approvals must not crash the Tool Authorizations page.
	It("should render high-risk pending approvals without hitting the global error boundary", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "delete_repo", "Delete repository permanently", "high")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred(), "Failed to create high-risk pending approval")

		err = authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasPending, err := authzPage.HasPendingSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPending).To(BeTrue(), "pending approvals section should remain visible")

			hasTool, err := authzPage.HasToolName(ctx, "delete_repo")
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasTool).To(BeTrue(), "high-risk approval should still render its tool name")

			hasGlobalError, err := authzPage.HasGlobalErrorBoundary(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasGlobalError).To(BeFalse(), "high-risk approval should not crash the Tool Authorizations page")
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())
	})

	// State 3: Approve action with persistence selection
	It("should allow approving a pending request with persistence choice", func() {
		principal := fixtures.DefaultPrincipal()

		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "read_file", "Read configuration file", "low")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasPending, err := authzPage.HasPendingSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPending).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		// Click Approve to expand persistence picker
		err = authzPage.ClickApproveOnFirst(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Select "Always allow" persistence
		err = authzPage.SelectPersistence(ctx, "Always allow")
		Expect(err).NotTo(HaveOccurred())

		// Verify permanent warning appears
		Eventually(func(g Gomega) {
			hasWarning, err := authzPage.HasPermanentWarning(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasWarning).To(BeTrue())
		}).WithTimeout(5 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = authzPage.TakeScreenshot(ctx, "tool_authorizations_approve_action")
		Expect(err).NotTo(HaveOccurred())

		// Confirm the approval
		err = authzPage.ClickConfirmApprove(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Wait for the card to be removed from pending and appear in permanent
		Eventually(func(g Gomega) {
			hasPermanent, err := authzPage.HasPermanentSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPermanent).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		GetLogger().Info("Test passed: Approve action with persistence completed")
	})

	// State 4: Deny action from list
	It("should allow denying a pending request from the list", func() {
		principal := fixtures.DefaultPrincipal()

		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "write_file", "Overwrite system file", "critical")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasPending, err := authzPage.HasPendingSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPending).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		// Click Deny to expand denial options
		err = authzPage.ClickDenyOnFirst(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.TakeScreenshot(ctx, "tool_authorizations_deny_action")
		Expect(err).NotTo(HaveOccurred())

		// Click "Deny this request"
		err = authzPage.ClickDenyThisRequest(ctx)
		Expect(err).NotTo(HaveOccurred())

		// Wait for the card to be removed from pending
		Eventually(func(g Gomega) {
			count, err := authzPage.GetPendingCount(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(0))
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		GetLogger().Info("Test passed: Deny action from list completed")
	})

	// State 5: Permanent authorizations with revoke
	It("should display permanent authorizations and allow revoking", func() {
		principal := fixtures.DefaultPrincipal()

		// Create and approve an approval permanently
		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "access_api", "Access external API", "medium")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())

		_, err = GetTestStorage().ToolApprovals().Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistencePermanent, ToolPattern: approval.ToolPattern, ParamsPattern: approval.ParamsPattern}, time.Now())
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToToolAuthorizations(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasPermanent, err := authzPage.HasPermanentSection(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasPermanent).To(BeTrue())
		}).WithTimeout(10 * time.Second).WithPolling(500 * time.Millisecond).Should(Succeed())

		// Verify permanent status
		hasAllowed, err := authzPage.HasPermanentlyAllowed(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasAllowed).To(BeTrue())

		// Verify revoke button
		hasRevoke, err := authzPage.HasRevokeButton(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(hasRevoke).To(BeTrue())

		err = authzPage.TakeScreenshot(ctx, "tool_authorizations_permanent_list")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Permanent authorizations displayed with revoke action")
	})
})
