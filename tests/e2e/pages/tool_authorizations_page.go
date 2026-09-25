// Package pages provides page object models for frontend E2E testing with Playwright.
// ToolAuthorizationsPage abstracts the tool authorizations list UI selectors and interactions.
package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

// ToolAuthorizationsPage represents the tool authorizations management page where users
// view pending approvals and manage permanent tool permissions.
//
// Route: /approvals
//
// Sections:
//   - Pending Approvals: list of pending tool requests with inline approve/deny
//   - Permanent Authorizations: list of permanent decisions with revoke action
type ToolAuthorizationsPage struct {
	*Page
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

// WaitForPageHeading waits for the "Tool Authorizations" heading to appear.
func (tp *ToolAuthorizationsPage) WaitForPageHeading(ctx context.Context) error {
	heading := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Tool Authorizations",
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

// HasGlobalErrorBoundary returns true if the global Oops error screen is visible.
func (tp *ToolAuthorizationsPage) HasGlobalErrorBoundary(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByText("Oops! Something went wrong", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check global error boundary: %w", err)
	}
	return count > 0, nil
}

// --- Empty state ---

// HasEmptyState returns true if the empty state message is visible.
func (tp *ToolAuthorizationsPage) HasEmptyState(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByText("No tool authorizations yet")
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check empty state: %w", err)
	}
	return count > 0, nil
}

// --- Pending section ---

// HasPendingSection returns true if the "Pending Approvals" section heading is visible.
func (tp *ToolAuthorizationsPage) HasPendingSection(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Pending Approvals",
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check pending section: %w", err)
	}
	return count > 0, nil
}

// GetPendingCount returns the number of unexpanded pending approval cards.
func (tp *ToolAuthorizationsPage) GetPendingCount(ctx context.Context) (int, error) {
	// Count Approve buttons - one per pending card
	locator := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Approve",
		Exact: playwright.Bool(true),
	})
	count, err := locator.Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count pending cards: %w", err)
	}
	return count, nil
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

// ClickApproveOnFirst clicks the "Approve" button on the first pending card.
func (tp *ToolAuthorizationsPage) ClickApproveOnFirst(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Approve",
		Exact: playwright.Bool(true),
	}).First()
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Approve button: %w", err)
	}
	return nil
}

// ClickDenyOnFirst clicks the "Deny" button on the first pending card.
func (tp *ToolAuthorizationsPage) ClickDenyOnFirst(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Deny",
		Exact: playwright.Bool(true),
	}).First()
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Deny button: %w", err)
	}
	return nil
}

// SelectPersistence clicks a persistence option in the first expanded approval.
func (tp *ToolAuthorizationsPage) SelectPersistence(ctx context.Context, label string) error {
	radio := tp.pwPage().GetByRole("radio", playwright.PageGetByRoleOptions{
		Name: label,
	}).First()
	if err := radio.Click(); err != nil {
		return fmt.Errorf("failed to click persistence radio %q: %w", label, err)
	}
	return nil
}

// ClickConfirmApprove confirms the first expanded approval.
func (tp *ToolAuthorizationsPage) ClickConfirmApprove(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Confirm Approve",
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

// HasPermanentSection returns true if the "Permanent Authorizations" heading is visible.
func (tp *ToolAuthorizationsPage) HasPermanentSection(ctx context.Context) (bool, error) {
	locator := tp.pwPage().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: "Permanent Authorizations",
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check permanent section: %w", err)
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
	locator := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Revoke",
	})
	count, err := locator.Count()
	if err != nil {
		return false, fmt.Errorf("failed to check revoke button: %w", err)
	}
	return count > 0, nil
}

// ClickRevokeOnFirst clicks the first "Revoke" button.
func (tp *ToolAuthorizationsPage) ClickRevokeOnFirst(ctx context.Context) error {
	button := tp.pwPage().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Revoke",
	}).First()
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click Revoke button: %w", err)
	}
	return nil
}
