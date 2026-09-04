// Package pages provides page object models for frontend E2E testing with Playwright.
// ApprovalPage abstracts the tool approval review UI selectors and interactions.
package pages

import (
	"context"
	"fmt"

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
	locator := ap.pwPage().GetByRole("alert")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check error alert: %w", err)
	}
	return count > 0, nil
}

// GetErrorTitle returns the heading text within the error alert.
func (ap *ApprovalPage) GetErrorTitle(ctx context.Context) (string, error) {
	alert := ap.pwPage().GetByRole("alert")
	heading := alert.GetByRole("heading")
	count, err := heading.Count()
	if err != nil {
		return "", fmt.Errorf("failed to count error heading: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("no error heading found in alert")
	}
	text, err := heading.First().TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get error heading text: %w", err)
	}
	return text, nil
}

// HasRetryButton returns true if the "Try Again" retry button is visible.
func (ap *ApprovalPage) HasRetryButton(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Try Again",
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
		Timeout: playwright.Float(10000),
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
	locator := ap.pwPage().Locator("h3").First()
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

// GetAgentName returns the agent display name shown as "Requested by {name}".
func (ap *ApprovalPage) GetAgentName(ctx context.Context) (string, error) {
	locator := ap.pwPage().GetByText("Requested by")
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

// HasGlobalErrorBoundary returns true if the global Oops error screen is visible.
func (ap *ApprovalPage) HasGlobalErrorBoundary(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByText("Oops! Something went wrong", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	})
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
	radio := ap.pwPage().GetByRole("radio", playwright.PageGetByRoleOptions{
		Name: label,
	})
	count, err := radio.Count()
	if err != nil {
		return fmt.Errorf("failed to find radio button %q: %w", label, err)
	}
	if count == 0 {
		return fmt.Errorf("radio button %q not found", label)
	}
	if err := radio.Click(); err != nil {
		return fmt.Errorf("failed to click radio %q: %w", label, err)
	}
	return nil
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

// ClickApprove clicks the "Approve" button.
func (ap *ApprovalPage) ClickApprove(ctx context.Context) error {
	button := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Approve",
	})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Approve button: %w", err)
	}
	return nil
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

// SetPatternField fills a labelled tool or parameter pattern input.
func (ap *ApprovalPage) SetPatternField(ctx context.Context, label, value string) error {
	input := ap.pwPage().GetByLabel(label)
	if count, err := input.Count(); err != nil || count == 0 {
		return fmt.Errorf("pattern field %q not found: %w", label, err)
	}
	if err := input.Fill(value); err != nil {
		return fmt.Errorf("fill pattern field %q: %w", label, err)
	}
	return nil
}

// ClickAllowAnyParameters clears all parameter constraints.
func (ap *ApprovalPage) ClickAllowAnyParameters(ctx context.Context) error {
	if err := ap.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Allow any parameters"}).Click(); err != nil {
		return fmt.Errorf("click Allow any parameters: %w", err)
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
		Timeout: playwright.Float(10000),
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
		Timeout: playwright.Float(10000),
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

// HasPermanentDenialNote returns true if the permanent denial note is visible.
func (ap *ApprovalPage) HasPermanentDenialNote(ctx context.Context) (bool, error) {
	locator := ap.pwPage().GetByText("permanent and will apply to future requests")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanent denial note: %w", err)
	}
	return count > 0, nil
}
