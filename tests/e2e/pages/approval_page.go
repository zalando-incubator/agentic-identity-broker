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
//   - Review: ToolCallCard + PersistenceSelector + Approve/Deny buttons
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

// NavigateToApproval navigates to the approval review page for the given approval ID.
// Route: /approvals/{approvalID}
func (ap *ApprovalPage) NavigateToApproval(ctx context.Context, approvalID string) error {
	if approvalID == "" {
		return fmt.Errorf("approvalID cannot be empty")
	}
	path := fmt.Sprintf("/approvals/%s", approvalID)
	return ap.Navigate(ctx, path)
}

// --- Loading state ---

// IsLoading returns true if the loading skeleton is visible (aria-busy="true").
func (ap *ApprovalPage) IsLoading(ctx context.Context) (bool, error) {
	locator := ap.pwPage().Locator("[aria-busy='true']")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check loading state: %w", err)
	}
	return count > 0, nil
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

// GetErrorTitle returns the heading text within the error alert.
func (ap *ApprovalPage) GetErrorTitle(ctx context.Context) (string, error) {
	heading := ap.pwPage().GetByRole("alert").GetByRole("heading").First()
	text, err := heading.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get error heading text: %w", err)
	}
	return text, nil
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

// WaitForReviewPage waits for the "Tool Approval Request" heading to appear.
func (ap *ApprovalPage) WaitForReviewPage(ctx context.Context) error {
	heading := ap.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Tool Approval Request",
	})
	err := heading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(ap.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("review page heading not found: %w", err)
	}
	return nil
}

// HasReviewHeading returns true if the approval review heading is visible.
func (ap *ApprovalPage) HasReviewHeading(ctx context.Context) (bool, error) {
	heading := ap.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Tool Approval Request",
	})
	count, err := heading.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check review page heading: %w", err)
	}
	return count > 0, nil
}

// GetToolName returns the tool name displayed in the ToolCallCard (h3 element).
func (ap *ApprovalPage) GetToolName(ctx context.Context) (string, error) {
	locator := ap.pwPage().GetByTestId("approval-tool-name").Locator(":scope > h3")
	count, err := locator.Count()
	if err != nil {
		return "", fmt.Errorf("failed to count tool name heading: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("tool name heading not found")
	}
	text, err := locator.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get tool name: %w", err)
	}
	return text, nil
}

// GetAgentName returns the requesting agent's display name.
func (ap *ApprovalPage) GetAgentName(ctx context.Context) (string, error) {
	locator := ap.pwPage().GetByTestId("approval-agent-name").Locator(":scope > p")
	count, err := locator.Count()
	if err != nil {
		return "", fmt.Errorf("failed to check agent name: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("agent name not found")
	}
	text, err := locator.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get agent name: %w", err)
	}
	return text, nil
}

// HasSectionLabel returns true if the given section label text is visible.
func (ap *ApprovalPage) HasSectionLabel(ctx context.Context, label string) (bool, error) {
	locator := ap.pwPage().GetByText(label, playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check section label %q: %w", label, err)
	}
	return count > 0, nil
}

// HasVisibleText returns true if the given text is visible anywhere on the page.
func (ap *ApprovalPage) HasVisibleText(ctx context.Context, text string) (bool, error) {
	locator := ap.pwPage().GetByText(text, playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	})
	visible, err := locator.First().IsVisible()
	if err != nil {
		count, countErr := locator.Count()
		if countErr != nil {
			return false, fmt.Errorf("failed to locate text %q: %w", text, countErr)
		}
		if count == 0 {
			return false, nil
		}
		return false, fmt.Errorf("failed to check visibility for text %q: %w", text, err)
	}
	return visible, nil
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
	locator := ap.pwPage().GetByText(level + " Risk")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check risk badge: %w", err)
	}
	return count > 0, nil
}

// --- Persistence selector ---

// SelectPersistence clicks the radio button for the given persistence option.
// Valid values: "Just this once", "For this session", "Always allow"
func (ap *ApprovalPage) SelectPersistence(ctx context.Context, label string) error {
	remember := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve and remember", Exact: playwright.Bool(true)})
	expanded, err := remember.GetAttribute("aria-expanded")
	if err != nil {
		return fmt.Errorf("read remembered approval controls: %w", err)
	}
	if label == "Just this once" {
		if expanded == "true" {
			return ap.locatorClick(ctx, remember, "close remembered approval controls")
		}
		return nil
	}
	if expanded != "true" {
		if err := ap.ClickApproveAndRemember(ctx); err != nil {
			return err
		}
	}
	return ap.ChooseRememberDuration(ctx, label)
}

// HasPermanentWarning returns true if the permanent warning alert is visible.
func (ap *ApprovalPage) HasPermanentWarning(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByText("Warning:")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanent warning: %w", err)
	}
	return count > 0, nil
}

// --- Action buttons ---

// ClickApprove submits the selected remembered duration or approves once.
func (ap *ApprovalPage) ClickApprove(ctx context.Context) error {
	selected := ap.pwPage().GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Approval persistence", Exact: playwright.Bool(true)}).
		Locator("[role='radio'][aria-checked='true']")
	count, err := selected.Count()
	if err != nil {
		return fmt.Errorf("read selected approval persistence: %w", err)
	}
	if count > 0 {
		return ap.ConfirmRememberedApproval(ctx)
	}
	return ap.ClickApproveOnce(ctx)
}

// ClickDeny clicks the "Deny" button.
func (ap *ApprovalPage) ClickDeny(ctx context.Context) error {
	button := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Deny",
		Exact: playwright.Bool(true),
	})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Deny button: %w", err)
	}
	return nil
}

// ExpandApprovalScope opens the "Approval scope" disclosure when it is collapsed.
func (ap *ApprovalPage) ExpandApprovalScope(ctx context.Context) error {
	toggle := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approval scope"})
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
	return ap.pwPage().Locator(fmt.Sprintf("[data-testid='approval-scope-param-%s']", key))
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
	preview := ap.pwPage().GetByLabel("Approval pattern preview").Locator("code")
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

func (ap *ApprovalPage) ExpandArguments(ctx context.Context) error {
	toggle := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Arguments", Exact: playwright.Bool(true)})
	timeout, err := ap.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	expanded, err := toggle.GetAttribute("aria-expanded", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("read tool arguments disclosure: %w", err)
	}
	if expanded == "true" {
		return nil
	}
	return ap.locatorClick(ctx, toggle, "expand tool arguments")
}

func (ap *ApprovalPage) ArgumentsText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-arguments"), "tool arguments")
}

func (ap *ApprovalPage) ActingUser(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-acting-user"), "acting user")
}

func (ap *ApprovalPage) RiskLabel(ctx context.Context) (string, error) {
	label, err := ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-risk"), "tool risk label")
	return strings.TrimSuffix(label, " Risk"), err
}

func (ap *ApprovalPage) ApprovalScopeText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByLabel("Approval pattern preview").Locator("code"), "approval scope preview")
}

func (ap *ApprovalPage) ClickApproveOnce(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve once", Exact: playwright.Bool(true)}), "approve tool call once")
}

// ClickApproveAndRemember opens the remembered-decision controls; it does not submit a decision.
func (ap *ApprovalPage) ClickApproveAndRemember(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve and remember", Exact: playwright.Bool(true)}), "review remembered approval")
}

func (ap *ApprovalPage) ChooseRememberDuration(ctx context.Context, label string) error {
	if label == "This session" {
		label = "For this session"
	}
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Approval persistence", Exact: playwright.Bool(true)}).
		GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose remembered approval duration "+label)
}

func (ap *ApprovalPage) HasDecisionActions(ctx context.Context) (bool, error) {
	var visible bool
	err := ap.evaluateJSON(ctx, ap.pwPage().GetByRole("main"), `root =>
		Array.from(root.querySelectorAll('button')).some(button =>
			button.getClientRects().length > 0 &&
			['Approve once', 'Approve and remember', 'Deny', 'Confirm approval',
				'Deny permanently — block this tool for this agent', 'Confirm permanent denial'].includes(
				(button.getAttribute('aria-label') || button.innerText).trim()))`, &visible)
	return visible, err
}

func (ap *ApprovalPage) ResolvedOutcomeText(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-outcome"), "resolved approval outcome")
}

func (ap *ApprovalPage) ToolName(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-tool-name").Locator(":scope > h3"), "approval tool name")
}

func (ap *ApprovalPage) AgentName(ctx context.Context) (string, error) {
	return ap.locatorText(ctx, ap.pwPage().GetByTestId("approval-agent-name").Locator(":scope > p"), "approval agent name")
}

func (ap *ApprovalPage) ArgumentsAreMonospace(ctx context.Context) (bool, error) {
	var monospace bool
	err := ap.evaluateJSON(ctx, ap.pwPage().GetByTestId("approval-arguments"), `element =>
		element.getClientRects().length > 0 &&
		getComputedStyle(element).fontFamily.split(',').some(family =>
			['monospace', 'ui-monospace'].includes(family.trim().replace(/["']/g, '')))`, &monospace)
	return monospace, err
}

func (ap *ApprovalPage) HasRememberAndDenyActions(ctx context.Context) (bool, error) {
	remember, err := ap.locatorVisible(ctx, ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Approve and remember", Exact: playwright.Bool(true)}), "remember approval action")
	if err != nil || !remember {
		return false, err
	}
	return ap.locatorVisible(ctx, ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), "deny approval action")
}

func (ap *ApprovalPage) ConfirmRememberedApproval(ctx context.Context) error {
	return ap.locatorClick(ctx, ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Confirm approval", Exact: playwright.Bool(true)}), "confirm remembered approval")
}
