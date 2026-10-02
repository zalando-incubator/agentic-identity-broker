// Package pages provides page object models for frontend E2E testing with Playwright.
// ToolAuthorizationsPage abstracts the tool authorizations list UI selectors and interactions.
package pages

import (
	"context"
	"fmt"
	"regexp"

	"github.com/mxschmitt/playwright-go"
)

var inlineDecisionConfirmation = regexp.MustCompile("^(Confirm approve|Deny this request)$")
var inlineDecisionToggle = regexp.MustCompile("^(Approve|Deny)$")

// ToolAuthorizationsPage represents the tool authorizations management page where users
// view pending approvals and manage permanent tool permissions.
//
// Route: /approvals
//
// Sections:
//   - Pending requests: tool requests with inline approve/deny
//   - Standing allow and deny decisions: remembered decisions with confirmed revoke
type ToolAuthorizationsPage struct {
	*Page
	standingRevokeRow playwright.Locator
}

// NewToolAuthorizationsPage creates a new ToolAuthorizationsPage wrapping a Playwright page.
func NewToolAuthorizationsPage(page playwright.Page, baseURL string) *ToolAuthorizationsPage {
	return &ToolAuthorizationsPage{
		Page: NewPage(page, baseURL),
	}
}

// pwPage returns the underlying Playwright page for selector access.
func (tp *ToolAuthorizationsPage) pwPage() playwright.Page {
	return tp.GetPlaywrightPage()
}

// NavigateToToolAuthorizations navigates to the tool authorizations page.
// Route: /approvals
func (tp *ToolAuthorizationsPage) NavigateToToolAuthorizations(ctx context.Context) error {
	return tp.Navigate(ctx, "/approvals")
}

// --- Page state ---

// WaitForPageHeading waits for the Approvals heading to appear.
func (tp *ToolAuthorizationsPage) WaitForPageHeading(ctx context.Context) error {
	heading := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Approvals",
	})
	err := heading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: new(float64(tp.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("tool authorizations heading not found: %w", err)
	}
	return nil
}

// WaitForLoaded waits for the loading state to resolve (no more skeletons).
func (tp *ToolAuthorizationsPage) WaitForLoaded(ctx context.Context) error {
	// Wait for the page heading first
	if err := tp.WaitForPageHeading(ctx); err != nil {
		return err
	}
	// Wait for network to settle (loading state finished)
	timeout := float64(tp.timeout.Milliseconds())
	return tp.pwPage().WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: &timeout,
	})
}

// HasGlobalErrorBoundary returns true if the global error screen is visible.
func (tp *ToolAuthorizationsPage) HasGlobalErrorBoundary(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByTestId("global-error-boundary")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check global error boundary: %w", err)
	}
	return count > 0, nil
}

// --- Empty state ---

// HasEmptyState returns true if the empty state message is visible.
func (tp *ToolAuthorizationsPage) HasEmptyState(ctx context.Context) (bool, error) {
	for _, message := range []string{"No pending approvals", "No standing allow decisions", "No standing deny decisions"} {
		visible, err := tp.locatorVisible(ctx, tp.pwPage().GetByText(message, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}), "empty approvals section")
		if err != nil || !visible {
			return false, err
		}
	}
	return true, nil
}

// --- Pending section ---

// HasPendingSection returns true if the pending requests heading is visible.
func (tp *ToolAuthorizationsPage) HasPendingSection(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Pending requests",
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check pending section: %w", err)
	}
	return count > 0, nil
}

// GetPendingCount returns the number of pending rows without an open decision.
func (tp *ToolAuthorizationsPage) GetPendingCount(ctx context.Context) (int, error) {
	count, err := tp.unexpandedPendingRows().Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count pending cards: %w", err)
	}
	return count, nil
}

func (tp *ToolAuthorizationsPage) unexpandedPendingRows() playwright.Locator {
	return tp.pwPage().GetByTestId("pending-approval-row").Filter(playwright.LocatorFilterOptions{
		HasNot: tp.pwPage().Locator("button[aria-expanded='true']").Filter(playwright.LocatorFilterOptions{HasText: inlineDecisionToggle}),
	})
}

// HasToolName returns true if the given tool name text is visible on the page.
func (tp *ToolAuthorizationsPage) HasToolName(ctx context.Context, toolName string) (bool, error) {
	locator := tp.pwPage().GetByText(toolName)
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check tool name %q: %w", toolName, err)
	}
	return count > 0, nil
}

// HasRiskBadge returns true if a risk badge with the given level is visible.
func (tp *ToolAuthorizationsPage) HasRiskBadge(ctx context.Context, level string) (bool, error) {
	locator := tp.pwPage().GetByText(level + " Risk")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check risk badge: %w", err)
	}
	return count > 0, nil
}

// --- Actions on pending cards ---

// ClickApproveOnFirst opens approval on the first row without an open decision.
func (tp *ToolAuthorizationsPage) ClickApproveOnFirst(ctx context.Context) error {
	button := tp.unexpandedPendingRows().First().GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name:  "Approve",
		Exact: playwright.Bool(true),
	})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Approve button: %w", err)
	}
	return nil
}

// ClickDenyOnFirst opens denial on the first row without an open decision.
func (tp *ToolAuthorizationsPage) ClickDenyOnFirst(ctx context.Context) error {
	button := tp.unexpandedPendingRows().First().GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name:  "Deny",
		Exact: playwright.Bool(true),
	})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Deny button: %w", err)
	}
	return nil
}

// SelectPersistence clicks a persistence option in the first expanded approval.
func (tp *ToolAuthorizationsPage) SelectPersistence(ctx context.Context, label string) error {
	if label == "This session" {
		label = "For this session"
	}
	radio := tp.pwPage().GetByTestId("pending-approval-row").Filter(playwright.LocatorFilterOptions{
		Has: tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Confirm approve", Exact: playwright.Bool(true)}),
	}).First().GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)})
	if err := radio.Click(); err != nil {
		return fmt.Errorf("failed to click persistence radio %q: %w", label, err)
	}
	return nil
}

// ClickConfirmApprove confirms the first expanded approval.
func (tp *ToolAuthorizationsPage) ClickConfirmApprove(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Confirm approve",
	}).First()
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Confirm Approve: %w", err)
	}
	return nil
}

// ClickDenyThisRequest clicks the "Deny this request" button.
func (tp *ToolAuthorizationsPage) ClickDenyThisRequest(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Deny this request",
	})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Deny this request: %w", err)
	}
	return nil
}

// HasPermanentWarning returns true if the permanent warning is visible.
func (tp *ToolAuthorizationsPage) HasPermanentWarning(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByText("grants permanent access")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanent warning: %w", err)
	}
	return count > 0, nil
}

// --- Permanent section ---

// HasPermanentSection returns true when a standing decision row is visible.
func (tp *ToolAuthorizationsPage) HasPermanentSection(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByTestId("standing-decisions").GetByTestId("standing-decision-row")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check standing decisions: %w", err)
	}
	return count > 0, nil
}

// HasPermanentlyAllowed returns true if "Permanently allowed" text is visible.
func (tp *ToolAuthorizationsPage) HasPermanentlyAllowed(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByText("Permanently allowed")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanently allowed: %w", err)
	}
	return count > 0, nil
}

// HasPermanentlyDenied returns true if "Permanently denied" text is visible.
func (tp *ToolAuthorizationsPage) HasPermanentlyDenied(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByText("Permanently denied")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanently denied: %w", err)
	}
	return count > 0, nil
}

// HasRevokeButton returns true if a "Revoke" button is visible.
func (tp *ToolAuthorizationsPage) HasRevokeButton(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByTestId("standing-decisions").GetByTestId("standing-decision-row").GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name:  "Revoke",
		Exact: playwright.Bool(true),
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check revoke button: %w", err)
	}
	return count > 0, nil
}

// ClickRevokeOnFirst clicks the first "Revoke" button.
func (tp *ToolAuthorizationsPage) ClickRevokeOnFirst(ctx context.Context) error {
	tool, err := tp.locatorText(ctx, tp.pwPage().GetByTestId("standing-decision-row").First().
		GetByTestId("approval-tool-name").Locator(":scope > span"), "first standing decision tool")
	if err != nil {
		return err
	}
	return tp.RevokeStanding(ctx, tool)
}

type PendingApprovalRow struct {
	Tool         string
	Agent        string
	Persistence  string
	ScopePreview string
}

type StandingDecisionRow struct {
	Tool         string
	Agent        string
	Decision     string
	ScopePreview string
}

func (tp *ToolAuthorizationsPage) SectionOrder(ctx context.Context) ([]string, error) {
	var sections []string
	err := tp.evaluateJSON(ctx, tp.pwPage().GetByRole("main"), `root =>
		Array.from(root.querySelectorAll('[data-testid="approval-section"]')).map(section => {
			const heading = section.querySelector('h2');
			if (!heading) throw new Error('Approval section has no heading');
			return heading.innerText.trim();
		})`, &sections)
	return sections, err
}

func (tp *ToolAuthorizationsPage) PendingRows(ctx context.Context) ([]PendingApprovalRow, error) {
	var rows []PendingApprovalRow
	err := tp.evaluateJSON(ctx, tp.pwPage().GetByTestId("pending-approvals"), `root =>
		Array.from(root.querySelectorAll('[data-testid="pending-approval-row"]')).map(row => {
			const text = (id, child = '') => {
				const element = row.querySelector('[data-testid="' + id + '"]' + child);
				if (!element) throw new Error('Missing pending approval ' + id);
				return element.innerText.trim();
			};
			const radio = row.querySelector('[role="radio"][aria-checked="true"], input[type="radio"]:checked');
			if (!radio) throw new Error('Pending approval has no selected persistence');
			const label = radio.getAttribute('aria-label') ||
				Array.from((radio.getAttribute('aria-labelledby') || '').split(/\s+/)).filter(Boolean)
					.map(id => document.getElementById(id)?.innerText || '').join(' ') ||
				Array.from(radio.labels || []).map(element => element.innerText).join(' ') || radio.innerText;
			return {Tool: text('approval-tool-name', ' > span'), Agent: text('approval-agent-name', ' > span'),
				Persistence: label.trim(), ScopePreview: text('approval-scope-preview')};
		})`, &rows)
	return rows, err
}

func (tp *ToolAuthorizationsPage) pendingRow(tool string) playwright.Locator {
	return tp.pwPage().GetByTestId("pending-approval-row").Filter(playwright.LocatorFilterOptions{
		Has: tp.pwPage().GetByText(tool, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}),
	})
}

// ApproveRow opens a row's confirmation without recording a decision.
func (tp *ToolAuthorizationsPage) ApproveRow(ctx context.Context, tool string) error {
	return tp.locatorClick(ctx, tp.pendingRow(tool).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approve", Exact: playwright.Bool(true)}), "review approval for "+tool)
}

// DenyRow opens a row's confirmation without recording a decision.
func (tp *ToolAuthorizationsPage) DenyRow(ctx context.Context, tool string) error {
	return tp.locatorClick(ctx, tp.pendingRow(tool).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), "review denial for "+tool)
}

func (tp *ToolAuthorizationsPage) ChoosePersistenceForRow(ctx context.Context, tool, label string) error {
	if label == "This session" {
		label = "For this session"
	}
	return tp.locatorClick(ctx, tp.pendingRow(tool).
		GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose approval persistence for "+tool)
}

func (tp *ToolAuthorizationsPage) ScopePreviewForRow(ctx context.Context, tool string) (string, error) {
	return tp.locatorText(ctx, tp.pendingRow(tool).GetByTestId("approval-scope-preview"), "approval scope preview for "+tool)
}

func (tp *ToolAuthorizationsPage) StandingDecisions(ctx context.Context) ([]StandingDecisionRow, error) {
	var rows []StandingDecisionRow
	err := tp.evaluateJSON(ctx, tp.pwPage().GetByTestId("standing-decisions"), `root =>
		Array.from(root.querySelectorAll('[data-testid="standing-decision-row"]')).map(row => {
			const text = (id, child = '') => {
				const element = row.querySelector('[data-testid="' + id + '"]' + child);
				if (!element) throw new Error('Missing standing decision ' + id);
				return element.innerText.trim();
			};
			return {Tool: text('approval-tool-name', ' > span'), Agent: text('approval-agent-name', ' > span'),
				Decision: text('approval-decision'), ScopePreview: text('approval-scope-preview')};
		})`, &rows)
	return rows, err
}

func (tp *ToolAuthorizationsPage) RevokeStanding(ctx context.Context, tool string) error {
	row := tp.pwPage().GetByTestId("standing-decision-row").Filter(playwright.LocatorFilterOptions{
		Has: tp.pwPage().GetByText(tool, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}),
	})
	if err := tp.locatorClick(ctx, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "revoke standing decision for "+tool); err != nil {
		return err
	}
	tp.standingRevokeRow = row
	return nil
}

func (tp *ToolAuthorizationsPage) ConfirmInlineDecision(ctx context.Context, tool string) error {
	return tp.locatorClick(ctx, tp.pendingRow(tool).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: inlineDecisionConfirmation}), "confirm tool decision for "+tool)
}

func (tp *ToolAuthorizationsPage) ConfirmRevokeStanding(ctx context.Context) error {
	if tp.standingRevokeRow == nil {
		return fmt.Errorf("no standing decision selected for revocation")
	}
	if err := tp.locatorClick(ctx, tp.pwPage().GetByRole("dialog").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke decision", Exact: playwright.Bool(true)}), "confirm standing decision revocation"); err != nil {
		return err
	}
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := tp.standingRevokeRow.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateDetached, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for revoked standing decision removal: %w", err)
	}
	tp.standingRevokeRow = nil
	return nil
}

func (tp *ToolAuthorizationsPage) CancelRevokeStanding(ctx context.Context) error {
	if err := tp.locatorClick(ctx, tp.pwPage().GetByRole("dialog").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}), "cancel standing decision revocation"); err != nil {
		return err
	}
	tp.standingRevokeRow = nil
	return nil
}
