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
	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
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
	Expect(domainapproval.ApplyExactPatterns(approval)).To(Succeed())
	return approval
}

// Approval inbox tests exercise the fixed pending list, shared review panel,
// and remembered decisions at /approvals and /approvals/remembered.
var _ = Describe("Approvals Inbox", func() {
	var (
		ctx       context.Context
		authzPage *pages.ApprovalsInboxPage
		testAgent *storage.Agent
	)

	BeforeEach(func() {
		ctx = context.Background()

		// Create a test agent
		testAgent = fixtures.ValidAgent()
		err := GetTestStorage().Agents().Create(ctx, testAgent)
		Expect(err).NotTo(HaveOccurred(), "Failed to create test agent")

		// Initialize page object
		authzPage = pages.NewApprovalsInboxPage(GetTestPage(), GetFrontendURL())
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md: empty inbox.
	It("should display empty state when no approvals exist", func() {
		err := authzPage.NavigateToApprovals(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			hasEmpty, err := authzPage.HasEmptyState(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasEmpty).To(BeTrue())
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		err = authzPage.TakeScreenshot(ctx, "approvals_empty")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Empty state displayed correctly")
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md: stable pending rows.
	It("should display pending approvals with tool details and action buttons", func() {
		principal := fixtures.DefaultPrincipal()

		// Seed two pending approvals
		approval1 := newTestApproval(id.Principal(principal.Email), testAgent.ID, "delete_files", "Delete files recursively", "critical")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval1)
		Expect(err).NotTo(HaveOccurred())

		approval2 := newTestApproval(id.Principal(principal.Email), testAgent.ID, "send_email", "Send email to recipient", "medium")
		_, err = GetTestStorage().ToolApprovals().Create(ctx, approval2)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToApprovals(ctx)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.WaitForLoaded(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			rows, err := authzPage.PendingRows(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rows).To(HaveLen(2))
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

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

		err = authzPage.TakeScreenshot(ctx, "approvals_pending_list")
		Expect(err).NotTo(HaveOccurred())

		GetLogger().Info("Test passed: Pending approvals list displayed correctly")
	})

	// Scenario US1-S1 from specs/024-approval-api-ui/spec.md
	// Regression: selecting a high-risk pending request must keep the shared panel stable.
	It("should render high-risk pending approvals without hitting the global error boundary", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "delete_repo", "Delete repository permanently", "high")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred(), "Failed to create high-risk pending approval")

		err = authzPage.NavigateToApprovals(ctx)
		Expect(err).NotTo(HaveOccurred())

		Eventually(func(g Gomega) {
			rows, err := authzPage.PendingRows(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rows).To(ContainElement(HaveField("Tool", "delete_repo")))
			hasGlobalError, err := authzPage.HasGlobalErrorBoundary(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasGlobalError).To(BeFalse(), "high-risk approval must not crash the inbox")
		}).WithPolling(500 * time.Millisecond).Should(Succeed())
		Expect(authzPage.SelectPendingRow(ctx, approval.ToolName)).To(Succeed())
		Expect(readV2(authzPage.ToolName(ctx))).To(Equal(approval.ToolName))
		Expect(readV2(authzPage.AgentName(ctx))).To(Equal(testAgent.DisplayName))
		Expect(readV2(authzPage.RiskLabel(ctx))).To(Equal("High"))
		Expect(readV2(authzPage.HasApproveOptionsAndDeny(ctx))).To(BeTrue(), "the selected request still offers decisions")
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md: only the current request may offer a decision.
	It("keeps rapid selection bound to the current request, not an exiting panel", func() {
		principal := fixtures.DefaultPrincipal()
		first := newTestApproval(id.Principal(principal.Email), testAgent.ID, "read_document", "Read the project files you choose.", "low")
		second := newTestApproval(id.Principal(principal.Email), testAgent.ID, "write_document", "Read the project files you choose.", "high")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, first)
		Expect(err).NotTo(HaveOccurred())
		_, err = GetTestStorage().ToolApprovals().Create(ctx, second)
		Expect(err).NotTo(HaveOccurred())
		Expect(authzPage.NavigateToApprovals(ctx)).To(Succeed())
		Expect(authzPage.WaitForLoaded(ctx)).To(Succeed())

		for _, tool := range []string{first.ToolName, second.ToolName, first.ToolName} {
			Expect(authzPage.SelectPendingRow(ctx, tool)).To(Succeed())
			Expect(readV2(authzPage.SelectedTool(ctx))).To(Equal(tool))
			Expect(readV2(authzPage.ToolName(ctx))).To(Equal(tool), "the active panel shows the selected tool, not the shared description")
			Expect(readV2(authzPage.HasStaleReviewActions(ctx))).To(BeFalse(), "exiting panels cannot retain decision controls")
		}
		Expect(authzPage.ClickApproveOnce(ctx)).To(Succeed())
		Eventually(func(g Gomega) {
			approved, err := GetTestStorage().ToolApprovals().Get(ctx, first.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(approved.Status).To(Equal(storage.ApprovalStatusApproved))
			g.Expect(approved.Persistence).NotTo(BeNil())
			g.Expect(*approved.Persistence).To(Equal(storage.ApprovalPersistenceOnce))
			unrelated, err := GetTestStorage().ToolApprovals().Get(ctx, second.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(unrelated.Status).To(Equal(storage.ApprovalStatusPending))
		}).Should(Succeed())
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md; US1-S4 from specs/024-approval-api-ui/spec.md.
	It("approves one of two pending requests with confirmed permanent scope", func() {
		principal := fixtures.DefaultPrincipal()

		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "read_file", "Read configuration file", "low")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		otherApproval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "send_email", "Send email", "medium")
		otherApproval.CreatedAt = approval.CreatedAt.Add(-time.Minute)
		_, err = GetTestStorage().ToolApprovals().Create(ctx, otherApproval)
		Expect(err).NotTo(HaveOccurred())

		err = authzPage.NavigateToApprovals(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(authzPage.WaitForLoaded(ctx)).To(Succeed())
		Eventually(func(g Gomega) {
			count, err := authzPage.GetPendingCount(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(2))
		}).WithPolling(500 * time.Millisecond).Should(Succeed())

		bounds := readV2(authzPage.PendingListBounds(ctx))
		Expect(authzPage.SelectPendingRow(ctx, approval.ToolName)).To(Succeed())
		Expect(readV2(authzPage.SelectedTool(ctx))).To(Equal(approval.ToolName))
		Expect(authzPage.OpenApproveOptions(ctx)).To(Succeed())
		Expect(authzPage.ChooseRememberDuration(ctx, "Always…")).To(Succeed())
		Expect(readV2(authzPage.ApprovalScopeText(ctx))).To(ContainSubstring(approval.ToolName))
		Expect(readV2(authzPage.HasPermanentWarning(ctx))).To(BeTrue())
		Expect(readV2(authzPage.PendingListBounds(ctx))).To(Equal(bounds), "scope editing cannot expand the pending list")
		Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Status).To(Equal(storage.ApprovalStatusPending), "scope editing alone cannot approve")
		Expect(authzPage.TakeScreenshot(ctx, "approvals_scope_editor")).To(Succeed())
		Expect(authzPage.ConfirmRememberedApproval(ctx)).To(Succeed())
		Eventually(func(g Gomega) {
			count, err := authzPage.GetPendingCount(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(1), "the unrelated request must stay pending")
		}).Should(Succeed())
		Expect(readV2(authzPage.PendingListBounds(ctx))).To(Equal(bounds))
		Expect(authzPage.OpenRemembered(ctx)).To(Succeed())
		Expect(readV2(authzPage.StandingDecisions(ctx))).To(ContainElement(HaveField("Tool", approval.ToolName)))
		Eventually(func(g Gomega) {
			selected, err := GetTestStorage().ToolApprovals().Get(ctx, approval.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(selected.Status).To(Equal(storage.ApprovalStatusApproved))
			g.Expect(selected.Persistence).NotTo(BeNil())
			g.Expect(*selected.Persistence).To(Equal(storage.ApprovalPersistencePermanent))
			g.Expect(selected.ToolPattern).To(Equal(approval.ToolPattern))
			g.Expect(selected.ParamsPattern).To(Equal(approval.ParamsPattern))

			other, err := GetTestStorage().ToolApprovals().Get(ctx, otherApproval.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(other.Status).To(Equal(storage.ApprovalStatusPending))
		}).Should(Succeed())
		GetLogger().Info("Test passed: Approve action with persistence completed")
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md: Deny is an explicit panel action.
	It("should deny the selected pending request without an inline row expansion", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "write_file", "Overwrite system file", "critical")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		Expect(authzPage.NavigateToApprovals(ctx)).To(Succeed())
		Expect(authzPage.WaitForLoaded(ctx)).To(Succeed())
		Expect(authzPage.SelectPendingRow(ctx, approval.ToolName)).To(Succeed())
		Expect(readV2(authzPage.ToolName(ctx))).To(Equal(approval.ToolName))
		Expect(readV2(authzPage.RiskLabel(ctx))).To(Equal("Critical"))
		Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Status).To(Equal(storage.ApprovalStatusPending))
		Expect(authzPage.TakeScreenshot(ctx, "approvals_deny_action")).To(Succeed())
		Expect(authzPage.ClickDeny(ctx)).To(Succeed())
		Eventually(func(g Gomega) {
			resolved, err := GetTestStorage().ToolApprovals().Get(ctx, approval.ID)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(resolved.Status).To(Equal(storage.ApprovalStatusDenied))
			g.Expect(resolved.Persistence).To(BeNil())
			count, err := authzPage.GetPendingCount(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(count).To(Equal(0))
		}).Should(Succeed())
		Expect(readV2(authzPage.HasEmptyState(ctx))).To(BeTrue())
	})

	// AS-12 from specs/047-redesign-consent-console/spec.md: remembered allow decisions can be revoked.
	It("shows and revokes a remembered approval after confirmation", func() {
		principal := fixtures.DefaultPrincipal()
		approval := newTestApproval(id.Principal(principal.Email), testAgent.ID, "access_api", "Access external API", "medium")
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
		_, err = GetTestStorage().ToolApprovals().Approve(ctx, approval.ID, storage.ApprovalDecision{Persistence: storage.ApprovalPersistencePermanent, ToolPattern: approval.ToolPattern, ParamsPattern: approval.ParamsPattern}, time.Now())
		Expect(err).NotTo(HaveOccurred())
		Expect(authzPage.NavigateRemembered(ctx)).To(Succeed())
		Expect(authzPage.FilterRemembered(ctx, "Always allowed")).To(Succeed())
		Eventually(func(g Gomega) {
			rows, err := authzPage.StandingDecisions(ctx)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(rows).To(ContainElement(HaveField("Tool", approval.ToolName)))
		}).Should(Succeed())
		Expect(readV2(authzPage.HasAlwaysAllowed(ctx))).To(BeTrue())
		Expect(readV2(authzPage.HasRevokeButton(ctx))).To(BeTrue())
		Expect(readV2(authzPage.StandingScope(ctx, approval.ToolName))).To(ContainSubstring(approval.ToolName))
		Expect(authzPage.TakeScreenshot(ctx, "approvals_remembered")).To(Succeed())
		Expect(authzPage.RevokeStanding(ctx, approval.ToolName)).To(Succeed())
		Expect(*readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Persistence).To(Equal(storage.ApprovalPersistencePermanent))
		Expect(authzPage.CancelRevokeStanding(ctx)).To(Succeed())
		Expect(*readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Persistence).To(Equal(storage.ApprovalPersistencePermanent))
		Expect(authzPage.RevokeStanding(ctx, approval.ToolName)).To(Succeed())
		Expect(authzPage.ConfirmRevokeStanding(ctx)).To(Succeed())
		Eventually(func() *storage.ApprovalPersistence {
			return readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID)).Persistence
		}).Should(BeNil())
		Expect(readV2(authzPage.StandingDecisions(ctx))).NotTo(ContainElement(HaveField("Tool", approval.ToolName)))
	})
})
