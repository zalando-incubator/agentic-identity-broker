// Package pages provides page object models for frontend E2E testing with Playwright.
// ApprovalPage abstracts the tool approval review UI selectors and interactions.
package pages

import (
	"context"
	"fmt"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

// ApprovalPage represents the tool approval review page where users approve or deny
// pending tool calls submitted by AI agents.
//
// Route: /approvals/:id
//
// States:
//   - Loading: Skeleton placeholder with aria-busy="true"
//   - Error: Alert banner (expired, forbidden, not_found, network_error, server_error, already_actioned)
//   - Review: Tool call details, scope editor, and explicit once/remembered/deny actions
//   - Confirmed: ApprovalConfirmation (approved or denied)
type ApprovalPage struct {
	*Page
}

// NewApprovalPage creates a new ApprovalPage wrapping a Playwright page.
func NewApprovalPage(page playwright.Page, baseURL string) *ApprovalPage {
	return &ApprovalPage{
		Page: NewPage(page, baseURL),
	}
}

// page returns the underlying Playwright page for selector access.
func (ap *ApprovalPage) pwPage() playwright.Page {
	return ap.GetPlaywrightPage()
}

// reviewPanel excludes inert exit animations but remains strict if two panels are actionable.
func (ap *ApprovalPage) reviewPanel() playwright.Locator {
	return ap.pwPage().Locator("[data-testid='approval-review-panel']:not([inert] *)")
}

// Exiting panels must not retain enabled decision controls.
func (ap *ApprovalPage) HasStaleReviewActions(ctx context.Context) (bool, error) {
	var stale bool
	err := ap.evaluateJSON(ctx, ap.pwPage().Locator("html"), `root =>
		[...root.querySelectorAll('[data-exiting="true"] [data-testid="approval-review-panel"]')].some(panel =>
			[...panel.querySelectorAll('button')].some(button => !button.disabled &&
				(button.hasAttribute('data-approval-action') || ['Approve options', 'Deny options'].includes(button.getAttribute('aria-label')))))`, &stale)
	return stale, err
}

// NavigateToApproval navigates to the approval review page for the given approval ID.
// Route: /approvals/{approvalID}
func (ap *ApprovalPage) NavigateToApproval(ctx context.Context, approvalID string) error {
	if approvalID == "" {
		return fmt.Errorf("approvalID cannot be empty")
	}
	path := fmt.Sprintf("/approvals/%s", approvalID)
	return ap.Navigate(ctx, path)
}

// BeginApprovalLoad navigates without waiting for the detail request so loading is observable.
func (ap *ApprovalPage) BeginApprovalLoad(ctx context.Context, approvalID string) error {
	if approvalID == "" {
		return fmt.Errorf("approvalID cannot be empty")
	}
	timeout, err := ap.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/approvals/%s", ap.GetBaseURL(), approvalID)
	if _, err := ap.pwPage().Goto(url, playwright.PageGotoOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("begin approval load at %s: %w", url, err)
	}
	return ctx.Err()
}

// --- Loading state ---

// IsLoading returns true while the approval detail skeleton is visible.
func (ap *ApprovalPage) IsLoading(ctx context.Context) (bool, error) {
	return ap.locatorVisible(ctx, ap.pwPage().Locator("[role='status'][aria-busy='true']"), "approval loading skeleton")
}

// --- Error states ---

// HasErrorAlert returns true if an error alert is visible on the page.
func (ap *ApprovalPage) HasErrorAlert(ctx context.Context) (bool, error) {
	alertHeading := ap.pwPage().GetByRole("alert").GetByRole("heading").First()
	if err := alertHeading.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(ap.timeout.Milliseconds())),
	}); err != nil {
		return false, fmt.Errorf("approval error alert did not become visible: %w", err)
	}
	return true, nil
}

// HasRetryButton returns true if the retry button is visible.
func (ap *ApprovalPage) HasRetryButton(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Try again",
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check retry button: %w", err)
	}
	return count > 0, nil
}

// --- Review state ---

// WaitForReviewPage waits for the current request's tool identity to appear.
func (ap *ApprovalPage) WaitForReviewPage(ctx context.Context) error {
	timeout, err := ap.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := ap.reviewPanel().GetByTestId("approval-tool-name").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible, Timeout: timeout,
	}); err != nil {
		return fmt.Errorf("approval tool identity did not appear: %w", err)
	}
	return ctx.Err()
}

// HasReviewIdentity returns true if the current request's tool identity is visible.
func (ap *ApprovalPage) HasReviewIdentity(ctx context.Context) (bool, error) {
	return ap.locatorVisible(ctx, ap.reviewPanel().GetByTestId("approval-tool-name"), "approval tool identity")
}

// HasVisibleText reports whether the current review shows the given text.
func (ap *ApprovalPage) HasVisibleText(ctx context.Context, text string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	panel := ap.reviewPanel()
	panels, err := panel.Count()
	if err != nil {
		return false, fmt.Errorf("find current approval review: %w", err)
	}
	if panels > 1 {
		return false, fmt.Errorf("more than one actionable approval review")
	}
	if panels == 0 {
		return false, nil
	}
	locator := panel.GetByText(text, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(false)})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("find approval text %q: %w", text, err)
	}
	for index := range count {
		visible, err := locator.Nth(index).IsVisible()
		if err != nil {
			return false, fmt.Errorf("inspect approval text %q: %w", text, err)
		}
		if visible {
			return true, ctx.Err()
		}
	}
	return false, ctx.Err()
}

// HasGlobalErrorBoundary returns true if the global error screen is visible.
func (ap *ApprovalPage) HasGlobalErrorBoundary(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByTestId("global-error-boundary")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check global error boundary: %w", err)
	}
	return count > 0, nil
}

// HasRiskBadge returns true if a risk badge is visible.
func (ap *ApprovalPage) HasRiskBadge(ctx context.Context, level string) (bool, error) {
	return ap.locatorVisible(ctx, ap.reviewPanel().GetByText(level+" risk"), "approval risk badge")
}

// --- Persistence selector ---

func (ap *ApprovalPage) OpenApproveOptions(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approve options", Exact: playwright.Bool(true)}), "open approval options")
}

func (ap *ApprovalPage) OpenDenyOptions(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Deny options", Exact: playwright.Bool(true)}), "open denial options")
}

func (ap *ApprovalPage) ChooseDenyOption(ctx context.Context, label string) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("menuitem", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose denial option "+label)
}

func (ap *ApprovalPage) ClickSelectedApproval(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().Locator(`[data-approval-action="approve"]`), "activate selected approval")
}

func (ap *ApprovalPage) ClickSelectedDenial(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().Locator(`[data-approval-action="deny"]`), "activate selected denial")
}

func (ap *ApprovalPage) ConfirmRememberedDenial(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Confirm permanent denial", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Confirm permanent denial", Exact: playwright.Bool(true)}), "confirm permanent denial")
}

func (ap *ApprovalPage) HasPermanentWarning(ctx context.Context) (bool, error) {
	return ap.locatorVisible(ctx, ap.reviewPanel().GetByRole("alert").Filter(playwright.LocatorFilterOptions{
		HasText: "grants permanent access",
	}), "permanent approval warning")
}

func (ap *ApprovalPage) ClickDeny(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name: "Deny", Exact: playwright.Bool(true),
	}), "deny selected tool request")
}

// ExpandApprovalScope opens the "Approval scope" disclosure when it is collapsed.
func (ap *ApprovalPage) ExpandApprovalScope(ctx context.Context) error {
	toggle := ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approval scope"})
	expanded, err := toggle.GetAttribute("aria-expanded")
	if err != nil {
		return fmt.Errorf("read approval scope state: %w", err)
	}
	if expanded == "true" {
		return nil
	}
	if err := toggle.Click(); err != nil {
		return fmt.Errorf("expand approval scope: %w", err)
	}
	return nil
}

// parameterScope locates the scope block of a single request parameter.
func (ap *ApprovalPage) parameterScope(key string) playwright.Locator {
	return ap.reviewPanel().Locator(fmt.Sprintf("[data-testid='approval-scope-param-%s']", key))
}

// SetParameterMode selects a per-parameter mode from its match dropdown.
// Valid modes: "This value", "Any value", "Custom match".
func (ap *ApprovalPage) SetParameterMode(ctx context.Context, key, mode string) error {
	scope := ap.parameterScope(key)
	trigger := scope.Locator("button[id$='-mode']")
	if err := trigger.Click(); err != nil {
		return fmt.Errorf("open mode dropdown for parameter %q: %w", key, err)
	}
	option := ap.pwPage().GetByRole("option", playwright.PageGetByRoleOptions{Name: mode})
	if err := option.Click(); err != nil {
		return fmt.Errorf("select mode %q for parameter %q: %w", mode, key, err)
	}
	return nil
}

// SetParameterCustomPattern fills the custom match input of a single parameter.
func (ap *ApprovalPage) SetParameterCustomPattern(ctx context.Context, key, value string) error {
	input := ap.parameterScope(key).GetByRole("textbox")
	if err := input.Fill(value); err != nil {
		return fmt.Errorf("fill custom pattern for parameter %q: %w", key, err)
	}
	return nil
}

// GetPatternPreview returns the displayed combined approval pattern.
func (ap *ApprovalPage) GetPatternPreview(ctx context.Context) (string, error) {
	preview := ap.reviewPanel().GetByLabel("Approval pattern preview").Locator("code")
	text, err := preview.TextContent()
	if err != nil {
		return "", fmt.Errorf("get pattern preview: %w", err)
	}
	return text, nil
}

// --- Confirmation state ---

// WaitForApprovedConfirmation waits for the "Approved" heading.
func (ap *ApprovalPage) WaitForApprovedConfirmation(ctx context.Context) error {
	heading := ap.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Approved",
	})
	err := heading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(ap.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("approved confirmation not found: %w", err)
	}
	return nil
}

// WaitForDeniedConfirmation waits for the "Denied" heading.
func (ap *ApprovalPage) WaitForDeniedConfirmation(ctx context.Context) error {
	heading := ap.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Denied",
	})
	err := heading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(ap.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("denied confirmation not found: %w", err)
	}
	return nil
}

// HasCloseMessage returns true if "You can safely close this page" text is visible.
func (ap *ApprovalPage) HasCloseMessage(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByText("You can safely close this page")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check close message: %w", err)
	}
	return count > 0, nil
}

// HasPersistenceText returns true if the given persistence text is in the confirmation.
func (ap *ApprovalPage) HasPersistenceText(ctx context.Context, text string) (bool, error) {
	locator := ap.pwPage().GetByText(text)
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check persistence text: %w", err)
	}
	return count > 0, nil
}

// ArgumentRows returns key/value rows visible without opening the raw JSON dialog.
func (ap *ApprovalPage) ArgumentRows(ctx context.Context) (map[string]string, error) {
	var rows map[string]string
	err := ap.evaluateJSON(ctx, ap.reviewPanel().GetByRole("region", playwright.LocatorGetByRoleOptions{Name: "Arguments", Exact: playwright.Bool(true)}), `section => {
		const list = section.querySelector(':scope > dl');
		if (!list) throw new Error('No visible key/value argument list');
		return Object.fromEntries([...list.querySelectorAll(':scope > div')].map(row => {
			const key = row.querySelector('dt'), value = row.querySelector('dd');
			if (!key || !value) throw new Error('Incomplete argument row');
			return [key.innerText.trim(), value.innerText.trim()];
		}));
	}`, &rows)
	return rows, err
}

func (ap *ApprovalPage) OpenArgumentsJSON(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "View JSON", Exact: playwright.Bool(true)}), "view raw tool arguments")
}

func (ap *ApprovalPage) ArgumentsText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Raw tool arguments", Exact: playwright.Bool(true)}).
		GetByTestId("approval-arguments"), "raw tool arguments")
}

func (ap *ApprovalPage) CloseArgumentsJSON(ctx context.Context) error {
	dialog := ap.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Raw tool arguments", Exact: playwright.Bool(true)})
	if err := ap.locatorClick(ctx, dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Close", Exact: playwright.Bool(true)}), "close raw tool arguments"); err != nil {
		return err
	}
	timeout, err := ap.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := dialog.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return fmt.Errorf("raw tool arguments did not close: %w", err)
	}
	return ctx.Err()
}

func (ap *ApprovalPage) OpenSessionContext(ctx context.Context) error {
	if err := ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Session context", Exact: playwright.Bool(true)}), "open session context"); err != nil {
		return err
	}
	timeout, err := ap.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := ap.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Session context", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for session context: %w", err)
	}
	return ctx.Err()
}

func (ap *ApprovalPage) SessionContextText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Session context", Exact: playwright.Bool(true)}), "session context")
}

func (ap *ApprovalPage) ActingUser(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.reviewPanel().GetByTestId("approval-acting-user"), "acting user")
}

func (ap *ApprovalPage) RiskLabel(ctx context.Context) (string, error) {
	label, err := ap.locatorText(ctx, ap.reviewPanel().GetByTestId("approval-risk"), "tool risk label")
	return strings.TrimSuffix(label, " risk"), err
}

func (ap *ApprovalPage) ApprovalScopeText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.reviewPanel().GetByLabel("Approval pattern preview").Locator("code"), "approval scope preview")
}

func (ap *ApprovalPage) ClickApproveOnce(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approve once", Exact: playwright.Bool(true)}), "approve tool call once")
}

func (ap *ApprovalPage) ChooseApprovalOption(ctx context.Context, label string) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("menuitem", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "select approval option "+label)
}

func (ap *ApprovalPage) HasDecisionActions(ctx context.Context) (bool, error) {
	var actions bool
	err := ap.evaluateJSON(ctx, ap.pwPage().Locator("html"), `root => {
		const panels = [...root.querySelectorAll('[data-testid="approval-review-panel"]')].filter(panel => !panel.closest('[inert]'));
		if (panels.length > 1) throw new Error('More than one actionable approval panel');
		const buttons = [...(panels[0]?.querySelectorAll('button') ?? []), ...root.querySelectorAll('[role="dialog"] button')];
		return buttons.some(button => {
			if (button.disabled || button.closest('[inert]') || button.getClientRects().length === 0) return false;
			const label = (button.getAttribute('aria-label') || button.innerText).trim();
			return button.hasAttribute('data-approval-action') || label === 'Confirm permanent denial';
		});
	}`, &actions)
	return actions, err
}

func (ap *ApprovalPage) ResolvedOutcomeText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-outcome"), "resolved approval outcome")
}

func (ap *ApprovalPage) ToolName(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.reviewPanel().GetByTestId("approval-tool-name"), "approval tool name")
}

func (ap *ApprovalPage) AgentName(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.reviewPanel().GetByTestId("approval-agent-name"), "approval agent name")
}

func (ap *ApprovalPage) ArgumentsAreMonospace(ctx context.Context) (bool, error) {
	var monospace bool
	err := ap.evaluateJSON(ctx, ap.pwPage().GetByTestId("approval-arguments"), `element =>
		element.getClientRects().length > 0 &&
		getComputedStyle(element).fontFamily.split(',').some(family =>
			['monospace', 'ui-monospace'].includes(family.trim().replace(/["']/g, '')))`, &monospace)
	return monospace, err
}

func (ap *ApprovalPage) HasApproveOptionsAndDeny(ctx context.Context) (bool, error) {
	options, err := ap.locatorVisible(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approve options", Exact: playwright.Bool(true)}), "approval options")
	if err != nil || !options {
		return false, err
	}
	return ap.locatorVisible(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), "deny approval")
}

func (ap *ApprovalPage) PanelHasFocus(ctx context.Context) (bool, error) {
	var focused bool
	err := ap.evaluateJSON(ctx, ap.reviewPanel(), `panel => panel === document.activeElement`, &focused)
	return focused, err
}

func (ap *ApprovalPage) ApproveViewportBottom(ctx context.Context) (float64, error) {
	var bottom float64
	err := ap.evaluateJSON(ctx, ap.reviewPanel().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Approve once", Exact: playwright.Bool(true)}),
		`button => button.getBoundingClientRect().bottom`, &bottom)
	return bottom, err
}
