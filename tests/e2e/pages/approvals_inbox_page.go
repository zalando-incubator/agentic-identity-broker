package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

// ApprovalsInboxPage models the pending list, shared review panel, and remembered route.
// ApprovalPage supplies the exact same panel actions for both inbox and standalone review.
type ApprovalsInboxPage struct {
	*ApprovalPage
	standingRevokeRow playwright.Locator
}

func NewApprovalsInboxPage(page playwright.Page, baseURL string) *ApprovalsInboxPage {
	return &ApprovalsInboxPage{ApprovalPage: NewApprovalPage(page, baseURL)}
}

func (tp *ApprovalsInboxPage) NavigateToApprovals(ctx context.Context) error {
	if err := tp.Navigate(ctx, "/approvals"); err != nil {
		return err
	}
	return tp.WaitForPageHeading(ctx)
}

func (tp *ApprovalsInboxPage) NavigateRemembered(ctx context.Context) error {
	if err := tp.Navigate(ctx, "/approvals/remembered"); err != nil {
		return err
	}
	return tp.WaitForPageHeading(ctx)
}

func (tp *ApprovalsInboxPage) WaitForPageHeading(ctx context.Context) error {
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Approvals", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for Approvals page: %w", err)
	}
	return ctx.Err()
}

func (tp *ApprovalsInboxPage) WaitForLoaded(ctx context.Context) error {
	if err := tp.WaitForPageHeading(ctx); err != nil {
		return err
	}
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	content := tp.pwPage().GetByTestId("pending-approvals").Or(
		tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "You're all caught up", Exact: playwright.Bool(true)}))
	if err := content.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for pending approvals: %w", err)
	}
	return ctx.Err()
}

func (tp *ApprovalsInboxPage) HasEmptyState(ctx context.Context) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "You're all caught up", Exact: playwright.Bool(true)}), "pending approvals empty state")
}

func (tp *ApprovalsInboxPage) HasPendingSection(ctx context.Context) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByTestId("approval-section"), "pending approvals section")
}

func (tp *ApprovalsInboxPage) GetPendingCount(ctx context.Context) (int, error) {
	var count int
	content := tp.pwPage().GetByTestId("pending-approvals").Or(
		tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "You're all caught up", Exact: playwright.Bool(true)})).First()
	err := tp.evaluateJSON(ctx, content, `root =>
		[...root.querySelectorAll('[data-testid="pending-approval-row"]')]
			.filter(row => !row.closest('[data-exiting], [inert]')).length`, &count)
	return count, err
}

func (tp *ApprovalsInboxPage) HasToolName(ctx context.Context, tool string) (bool, error) {
	return tp.locatorVisible(ctx, tp.pendingRow(tool), "pending request "+tool)
}

func (tp *ApprovalsInboxPage) HasRiskBadge(ctx context.Context, level string) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByTestId("pending-approvals").GetByTestId("approval-risk").
		Filter(playwright.LocatorFilterOptions{HasText: level + " risk"}).First(), "pending risk "+level)
}

type PendingApprovalRow struct {
	Tool  string
	Agent string
	Risk  string
}

type StandingDecisionRow struct {
	Tool         string
	Agent        string
	Decision     string
	ScopePreview string
}

func (tp *ApprovalsInboxPage) PendingRows(ctx context.Context) ([]PendingApprovalRow, error) {
	var rows []PendingApprovalRow
	content := tp.pwPage().GetByTestId("pending-approvals").Or(
		tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "You're all caught up", Exact: playwright.Bool(true)})).First()
	err := tp.evaluateJSON(ctx, content, `root =>
		[...root.querySelectorAll('[data-testid="pending-approval-row"]')]
			.filter(row => !row.closest('[data-exiting], [inert]')).map(row => {
			const name = row.querySelector('[data-testid="entity-name"]');
			const agent = row.querySelector('[data-testid="approval-agent-name"]');
			const risk = row.querySelector('[data-testid="approval-risk"]');
			if (!name || !agent || !risk) throw new Error('Pending request missing tool, agent or risk');
			return {Tool: name.innerText.trim(), Agent: agent.innerText.trim(), Risk: risk.innerText.trim()};
		})`, &rows)
	return rows, err
}

func (tp *ApprovalsInboxPage) pendingRow(tool string) playwright.Locator {
	return tp.pwPage().GetByTestId("pending-approvals").Locator("[data-testid='pending-approval-row']:not([inert] *)").Filter(playwright.LocatorFilterOptions{
		Has: tp.entityNameLocator(tool),
	})
}

func (tp *ApprovalsInboxPage) SelectPendingRow(ctx context.Context, tool string) error {
	row := tp.pendingRow(tool)
	trigger := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: tool, Exact: playwright.Bool(true)}).
		Or(row.GetByRole("link", playwright.LocatorGetByRoleOptions{Name: tool, Exact: playwright.Bool(true)}))
	return tp.locatorClick(ctx, trigger, "select pending tool request "+tool)
}

func (tp *ApprovalsInboxPage) SelectedTool(ctx context.Context) (string, error) {
	return tp.locatorText(ctx, tp.pwPage().GetByTestId("pending-approvals").Locator(`[data-testid="pending-approval-row"][data-selected="true"]:not([inert] *)`).
		GetByTestId("entity-name"), "selected pending tool")
}

// Document coordinates distinguish collection reflow from scrolling to an off-screen request.
func (tp *ApprovalsInboxPage) PendingListBounds(ctx context.Context) ([]float64, error) {
	var bounds []float64
	err := tp.evaluateJSON(ctx, tp.pwPage().GetByTestId("pending-approvals"), `list => {
		const rect = list.getBoundingClientRect();
		return [rect.x + window.scrollX, rect.y + window.scrollY, rect.width, rect.height];
	}`, &bounds)
	return bounds, err
}

func (tp *ApprovalsInboxPage) PendingRowBounds(ctx context.Context, tool string) ([]float64, error) {
	var bounds []float64
	err := tp.evaluateJSON(ctx, tp.pendingRow(tool), `row => {
		const rect = row.getBoundingClientRect();
		return [rect.x + window.scrollX, rect.y + window.scrollY, rect.width, rect.height];
	}`, &bounds)
	return bounds, err
}

func (tp *ApprovalsInboxPage) PressInboxKey(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if key != "j" && key != "k" && key != "a" && key != "d" && key != "Enter" {
		return fmt.Errorf("unsupported inbox shortcut %q", key)
	}
	if err := tp.pwPage().Keyboard().Press(key); err != nil {
		return fmt.Errorf("press inbox shortcut %q: %w", key, err)
	}
	return ctx.Err()
}

func (tp *ApprovalsInboxPage) OpenRemembered(ctx context.Context) error {
	return tp.locatorClick(ctx, tp.pwPage().GetByRole("tab", playwright.PageGetByRoleOptions{Name: "Remembered", Exact: playwright.Bool(false)}), "open remembered decisions")
}

func (tp *ApprovalsInboxPage) ShiftSelectRememberedText(ctx context.Context, first, second string) (string, error) {
	if err := tp.pwPage().GetByTestId("standing-decision-row").Filter(playwright.LocatorFilterOptions{Has: tp.entityNameLocator(first)}).
		GetByTestId("entity-name").Dblclick(); err != nil {
		return "", fmt.Errorf("double-click remembered tool: %w", err)
	}
	if err := tp.pwPage().GetByTestId("standing-decision-row").Filter(playwright.LocatorFilterOptions{Has: tp.entityNameLocator(second)}).
		GetByTestId("entity-name").Click(playwright.LocatorClickOptions{Modifiers: []playwright.KeyboardModifier{*playwright.KeyboardModifierShift}}); err != nil {
		return "", fmt.Errorf("shift-click remembered tool: %w", err)
	}
	var selected string
	err := tp.evaluateJSON(ctx, tp.pwPage().Locator("html"), `() => window.getSelection().toString()`, &selected)
	return selected, err
}

func (tp *ApprovalsInboxPage) FilterRemembered(ctx context.Context, label string) error {
	if label != "All" && label != "Always allowed" && label != "Always denied" {
		return fmt.Errorf("unknown remembered decisions filter %q", label)
	}
	if err := tp.locatorClick(ctx, tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Filter decisions", Exact: playwright.Bool(false)}), "open remembered decisions filter"); err != nil {
		return err
	}
	option := tp.pwPage().GetByRole("group", playwright.PageGetByRoleOptions{Name: "Filter decisions", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)})
	pressed, err := option.GetAttribute("aria-pressed")
	if err != nil {
		return fmt.Errorf("read remembered filter state: %w", err)
	}
	if pressed != "true" {
		if err := tp.locatorClick(ctx, option, "filter remembered decisions by "+label); err != nil {
			return err
		}
	}
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := option.And(tp.pwPage().Locator(`[aria-pressed="true"]`)).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for remembered filter %q: %w", label, err)
	}
	return tp.pwPage().Keyboard().Press("Escape")
}

func (tp *ApprovalsInboxPage) HasAlwaysAllowed(ctx context.Context) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByTestId("standing-decisions").GetByTestId("approval-decision").
		Filter(playwright.LocatorFilterOptions{HasText: "Always allowed"}).First(), "always allowed decision")
}

func (tp *ApprovalsInboxPage) HasAlwaysDenied(ctx context.Context) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByTestId("standing-decisions").GetByTestId("approval-decision").
		Filter(playwright.LocatorFilterOptions{HasText: "Always denied"}).First(), "always denied decision")
}

func (tp *ApprovalsInboxPage) HasRevokeButton(ctx context.Context) (bool, error) {
	return tp.locatorVisible(ctx, tp.pwPage().GetByTestId("standing-decisions").GetByTestId("standing-decision-row").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}).First(), "revoke remembered decision")
}

func (tp *ApprovalsInboxPage) StandingDecisions(ctx context.Context) ([]StandingDecisionRow, error) {
	var rows []StandingDecisionRow
	content := tp.pwPage().GetByTestId("standing-decisions").Or(
		tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "No remembered decisions", Exact: playwright.Bool(true)})).First()
	err := tp.evaluateJSON(ctx, content, `root =>
		[...root.querySelectorAll('[data-testid="standing-decision-row"]')]
			.filter(row => !row.closest('[data-exiting], [inert]')).map(row => {
			const text = id => {
				const element = row.querySelector('[data-testid="' + id + '"]');
				if (!element) throw new Error('Remembered decision missing ' + id);
				return element.innerText.trim();
			};
			return {Tool: text('entity-name'), Agent: text('approval-agent-name'),
				Decision: text('approval-decision'), ScopePreview: text('approval-scope-preview')};
		})`, &rows)
	return rows, err
}

func (tp *ApprovalsInboxPage) standingRow(tool string) playwright.Locator {
	return tp.pwPage().GetByTestId("standing-decisions").GetByTestId("standing-decision-row").Filter(playwright.LocatorFilterOptions{
		Has: tp.entityNameLocator(tool),
	})
}

func (tp *ApprovalsInboxPage) StandingScope(ctx context.Context, tool string) (string, error) {
	trigger := tp.standingRow(tool).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "View full scope pattern", Exact: playwright.Bool(true)})
	if err := tp.locatorClick(ctx, trigger, "show full pattern for "+tool); err != nil {
		return "", err
	}
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	popover := tp.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "View full scope pattern", Exact: playwright.Bool(true)})
	if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return "", fmt.Errorf("wait for full scope pattern for %s: %w", tool, err)
	}
	pattern, err := tp.locatorText(ctx, popover.Locator("code"), "full scope for "+tool)
	if err != nil {
		return "", err
	}
	if err := tp.pwPage().Keyboard().Press("Escape"); err != nil {
		return "", fmt.Errorf("close full scope pattern: %w", err)
	}
	if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return "", fmt.Errorf("full scope pattern did not close: %w", err)
	}
	return pattern, ctx.Err()
}

func (tp *ApprovalsInboxPage) RevokeStanding(ctx context.Context, tool string) error {
	row := tp.standingRow(tool)
	if err := tp.locatorClick(ctx, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "revoke remembered decision for "+tool); err != nil {
		return err
	}
	tp.standingRevokeRow = row
	return nil
}

func (tp *ApprovalsInboxPage) ConfirmRevokeStanding(ctx context.Context) error {
	if tp.standingRevokeRow == nil {
		return fmt.Errorf("no remembered decision selected for revocation")
	}
	if err := tp.locatorClick(ctx, tp.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Revoke remembered decision?", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke decision", Exact: playwright.Bool(true)}), "confirm remembered decision revocation"); err != nil {
		return err
	}
	timeout, err := tp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := tp.standingRevokeRow.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateDetached, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for revoked remembered decision to disappear: %w", err)
	}
	tp.standingRevokeRow = nil
	return ctx.Err()
}

func (tp *ApprovalsInboxPage) CancelRevokeStanding(ctx context.Context) error {
	if err := tp.locatorClick(ctx, tp.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Revoke remembered decision?", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}), "cancel remembered decision revocation"); err != nil {
		return err
	}
	tp.standingRevokeRow = nil
	return ctx.Err()
}

func (tp *ApprovalsInboxPage) PendingTabLabel(ctx context.Context) (string, error) {
	return tp.locatorText(ctx, tp.pwPage().GetByRole("tab", playwright.PageGetByRoleOptions{Name: "Pending", Exact: playwright.Bool(false)}), "pending tab count")
}

func (tp *ApprovalsInboxPage) RememberedFilterValue(ctx context.Context) (string, error) {
	return tp.locatorText(ctx, tp.pwPage().GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Filter decisions", Exact: playwright.Bool(true)}), "remembered filter")
}
