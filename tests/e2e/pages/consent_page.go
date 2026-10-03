// Package pages provides page object models for frontend E2E testing.
// Page objects abstract UI selectors and interactions into high-level methods
// that tests call, making tests more readable and maintainable.
package pages

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// Route constants for navigation.
// The React app serves the overview at /delegations and the agent detail page at /agents/:agentId.
const (
	agentDetailPath   = "/agents/%s"
	overviewPath      = "/delegations"
	revokeDialogTitle = "Revoke access"
)

// ConsentPage models the authorization decision and console grant editor.
// Selectors target accessible controls and the owned components' semantic test IDs.
//
// Usage pattern:
//
//	consentPage := NewConsentPage(page, baseURL)
//	consentPage.NavigateToAgent(ctx, "agent-id")
//	consentPage.TogglePermissionSet(ctx, "Optional access")
//	consentPage.SaveChanges(ctx)
type ConsentPage struct {
	*Page // Embed Page for method forwarding
}

// NewConsentPage creates a new ConsentPage instance.
//
// Parameters:
//   - page: Playwright page instance for this test
//   - baseURL: Base URL of the frontend (e.g., "http://localhost:3000" or "http://localhost:8000")
//
// Returns:
//   - *ConsentPage: Initialized page object
//
// Example:
//
//	page := GetTestPage()
//	consentPage := NewConsentPage(page, GetFrontendURL())
func NewConsentPage(page playwright.Page, baseURL string) *ConsentPage {
	return &ConsentPage{
		Page: NewPage(page, baseURL),
	}
}

// NavigateToAgent navigates to the consent page for a specific agent.
//
// Parameters:
//   - ctx: Context for cancellation
//   - agentID: UUID of the agent
//
// Returns:
//   - error: If navigation fails
//
// The page waits implicitly for load events before returning.
// NavigateToAgent navigates to the detail page for a specific agent.
// Waits for the page to finish loading before returning.
//
// The React app serves the agent detail page at /agents/:agentId.
// This corresponds to the `/agents/:agentId` React route.
//
// Returns an error if navigation fails or the page does not load within the timeout.
// If the page fails to load, returns a descriptive error.
//
// Example:
//
//	err := consentPage.NavigateToAgent(ctx, "agent-uuid-123")
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) NavigateToAgent(ctx context.Context, agentID string) error {
	if agentID == "" {
		return fmt.Errorf("agentID cannot be empty")
	}

	// Navigate to agent detail page
	// Route: /agents/:agentId (defined in React Router App.tsx)
	path := fmt.Sprintf(agentDetailPath, agentID)
	if err := cp.Navigate(ctx, path); err != nil {
		return fmt.Errorf("failed to navigate to agent consent page: %w", err)
	}

	// Wait for agent name heading to ensure page is interactive
	if err := cp.waitForAgentNameHeading(ctx); err != nil {
		return fmt.Errorf("agent name heading not found (page may not have loaded): %w", err)
	}

	return nil
}

// NavigateToAgentWithRedirectURI navigates to the consent page for a specific agent
// with a redirect_uri query parameter, simulating the OAuth2 authorization flow redirect.
//
// Parameters:
//   - ctx: Context for cancellation
//   - agentID: UUID of the agent
//   - redirectURI: Redirect URI to include as a query parameter (e.g., "/some-callback")
//
// Returns:
//   - error: If navigation fails or the page does not load
func (cp *ConsentPage) NavigateToAgentWithRedirectURI(ctx context.Context, agentID, redirectURI string) error {
	if agentID == "" {
		return fmt.Errorf("agentID cannot be empty")
	}

	path := fmt.Sprintf("%s?redirect_uri=%s", fmt.Sprintf(agentDetailPath, agentID), url.QueryEscape(redirectURI))
	if err := cp.Navigate(ctx, path); err != nil {
		return fmt.Errorf("failed to navigate to agent consent page with redirect_uri: %w", err)
	}

	if err := cp.waitForAgentNameHeading(ctx); err != nil {
		return fmt.Errorf("agent name heading not found after navigation with redirect_uri: %w", err)
	}

	return nil
}

// NavigateToOverview navigates to the consent overview page (list of delegations).
// The React app serves the overview at /delegations.
//
// Returns an error if navigation fails.
//
// Example:
//
//	err := consentPage.NavigateToOverview(ctx)
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) NavigateToOverview(ctx context.Context) error {
	if err := cp.Navigate(ctx, overviewPath); err != nil {
		return fmt.Errorf("failed to navigate to consent overview page: %w", err)
	}
	return nil
}

// page returns the underlying Playwright page object.
func (cp *ConsentPage) page() playwright.Page {
	return cp.GetPlaywrightPage()
}

// waitForAgentNameHeading waits for the main h1 heading (agent name) to appear.
// This waits for the page to load the agent detail content (not the header).
// Uses data-testid to avoid ambiguity when multiple h1 elements exist on page.
func (cp *ConsentPage) waitForAgentNameHeading(ctx context.Context) error {
	agentNameHeading := cp.page().GetByTestId("agent-name-heading").GetByRole("heading", playwright.LocatorGetByRoleOptions{Level: playwright.Int(1)})
	err := agentNameHeading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("agent name heading not found after waiting %v (page may not have loaded): %w", cp.timeout, err)
	}
	return nil
}

// SubmitConsent submits the decision or console draft for the current route context.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - error: If the button is not found or not clickable
//
// The button may be disabled if mandatory requirements are not met.
func (cp *ConsentPage) SubmitConsent(ctx context.Context) error {
	sessionToken, err := cp.GetURLQueryParam("session_token")
	if err != nil {
		return err
	}
	if sessionToken != "" {
		return cp.ClickAllow(ctx)
	}
	return cp.SaveChanges(ctx)
}

func (cp *ConsentPage) endDateInput() playwright.Locator {
	return cp.page().Locator("input[type='date']").And(cp.page().GetByLabel("Custom date", playwright.PageGetByLabelOptions{
		Exact: playwright.Bool(true),
	}))
}

// SetExpiration sets the grant expiration days using the expiration input.
//
// Parameters:
//   - ctx: Context for cancellation
//   - expirationDays: Number of days until expiration
//
// Returns:
//   - error: If expiration input not found or not fillable
//
// Note: This method calculates the expiration date and fills the date input.
// The date format expected is ISO 8601 (YYYY-MM-DD).
//
// Example:
//
//	err := consentPage.SetExpiration(ctx, 30)
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) SetExpiration(ctx context.Context, expirationDays int) error {
	if expirationDays <= 0 {
		return fmt.Errorf("expirationDays must be positive")
	}

	return cp.SetExpirationDate(ctx, time.Now().AddDate(0, 0, expirationDays))
}

// EnableSpecificEndDate selects the specific end date option.
func (cp *ConsentPage) EnableSpecificEndDate(ctx context.Context) error {
	return cp.ChooseDuration(ctx, "Custom date")
}

// SetExpirationDate sets the grant expiration to a specific date.
//
// Parameters:
//   - ctx: Context for cancellation
//   - date: Expiration date as time.Time
//
// Returns:
//   - error: If expiration input not found or not fillable
//
// The date is formatted as ISO 8601 (YYYY-MM-DD) for the date input.
//
// Example:
//
//	tomorrow := time.Now().AddDate(0, 0, 1)
//	err := consentPage.SetExpirationDate(ctx, tomorrow)
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) SetExpirationDate(ctx context.Context, date time.Time) error {
	if err := cp.EnableSpecificEndDate(ctx); err != nil {
		return err
	}
	return cp.SetCustomDate(ctx, date.Format("2006-01-02"))
}

// GetAgentName retrieves the agent display name from the page heading.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - string: Agent display name
//   - error: If heading not found
//
// Example:
//
//	name, err := consentPage.GetAgentName(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(name).To(Equal("GitHub Agent"))
func (cp *ConsentPage) GetAgentName(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("agent-name-heading").
		GetByRole("heading", playwright.LocatorGetByRoleOptions{Level: playwright.Int(1)}), "agent name")
}

// IsSaveButtonEnabled checks if the existing-grant Save action is enabled.
func (cp *ConsentPage) IsSaveButtonEnabled(ctx context.Context) (bool, error) {
	button := cp.page().GetByTestId("grant-save-bar").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save changes", Exact: playwright.Bool(true)})
	visible, err := cp.locatorVisible(ctx, button, "save grant changes")
	if err != nil || !visible {
		return false, err
	}
	return button.IsEnabled()
}

func (cp *ConsentPage) grantErrors() playwright.Locator {
	return cp.page().GetByRole("main").Locator("[role='alert']:not([data-testid='localhost-warning'])")
}

func (cp *ConsentPage) grantSavedStatus() playwright.Locator {
	return cp.page().Locator("form").GetByRole("status").Filter(playwright.LocatorFilterOptions{
		HasText: "Grant updated successfully.",
	})
}

// GetErrorMessage retrieves the error message if one is displayed.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - string: Error message text
//   - error: If error region not found or empty
//
// Reads decision/editor errors without treating the localhost safety warning as a failure.
//
// Example:
//
//	msg, err := consentPage.GetErrorMessage(ctx)
//	// err will be non-nil if no error message is displayed
//	if err == nil {
//	  fmt.Printf("Error: %s\n", msg)
//	}
func (cp *ConsentPage) GetErrorMessage(ctx context.Context) (string, error) {
	// Find alert region
	alert := cp.grantErrors()

	// Check if it exists
	count, err := alert.Count()
	if err != nil || count == 0 {
		return "", fmt.Errorf("no error message found")
	}

	// Get the text content
	text, err := alert.First().TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get error message text: %w", err)
	}

	if strings.TrimSpace(text) == "" {
		return "", fmt.Errorf("error message is empty")
	}

	return strings.TrimSpace(text), nil
}

// HasError checks if an error message is currently displayed.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - bool: True if error region found, false otherwise
//   - error: Only if there's a critical error accessing the page
//
// Example:
//
//	hasErr, err := consentPage.HasError(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(hasErr).To(BeTrue())
func (cp *ConsentPage) HasError(ctx context.Context) (bool, error) {
	// Try to find alert region
	count, err := cp.grantErrors().Count()
	if err != nil {
		return false, fmt.Errorf("failed to check for error: %w", err)
	}

	return count > 0, nil
}

// GetExpirationDate retrieves the current expiration date value from the input.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - string: Expiration date value (ISO 8601 format YYYY-MM-DD)
//   - error: If expiration input not found
//
// Example:
//
//	date, err := consentPage.GetExpirationDate(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(date).To(Equal("2026-02-15"))
func (cp *ConsentPage) GetExpirationDate(ctx context.Context) (string, error) {
	return cp.CustomDateValue(ctx)
}

// DelegateService starts the provider connection flow without changing the grant.
func (cp *ConsentPage) DelegateService(ctx context.Context, serviceDisplayName string) error {
	if serviceDisplayName == "" {
		return fmt.Errorf("serviceDisplayName cannot be empty")
	}
	return cp.ClickConnect(ctx, serviceDisplayName)
}

// TogglePermissionSet changes an optional permission set's selection by name.
func (cp *ConsentPage) TogglePermissionSet(ctx context.Context, permissionSetName string) error {
	if permissionSetName == "" {
		return fmt.Errorf("permission set name cannot be empty")
	}
	return cp.locatorClick(ctx, cp.permissionGroup(permissionSetName).
		Locator("[role='checkbox'][aria-label]"), "toggle permission group "+permissionSetName)
}

// Radix renders checkbox controls as buttons, so SetChecked (for native inputs)
// cannot set these controls. Click only when the displayed state differs.
func (cp *ConsentPage) setCheckboxChecked(ctx context.Context, control playwright.Locator, checked bool) error {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	state, err := control.GetAttribute("aria-checked", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("read checkbox selection: %w", err)
	}
	if state != "true" && state != "false" {
		return fmt.Errorf("checkbox has unexpected aria-checked value %q", state)
	}
	if (state == "true") == checked {
		return nil
	}
	return cp.locatorClick(ctx, control, "checkbox")
}

// GetRevokeButton returns the destructive menu item after OpenOverflowMenu.
func (cp *ConsentPage) GetRevokeButton(ctx context.Context) playwright.Locator {
	return cp.page().GetByRole("menuitem", playwright.PageGetByRoleOptions{
		Name: "Revoke all access", Exact: playwright.Bool(true),
	})
}

// ClickRevokeButton opens the overflow action and its confirmation dialog.
func (cp *ConsentPage) ClickRevokeButton(ctx context.Context) error {
	if err := cp.OpenOverflowMenu(ctx); err != nil {
		return err
	}
	return cp.ChooseRevokeAllAccess(ctx)
}

// GetRevokeDialog returns the shared, named grant-revocation dialog.
func (cp *ConsentPage) GetRevokeDialog(ctx context.Context) playwright.Locator {
	return cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: revokeDialogTitle, Exact: playwright.Bool(true)})
}

// ConfirmRevoke confirms deletion of the acting user's grant.
func (cp *ConsentPage) ConfirmRevoke(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.GetRevokeDialog(ctx).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "confirm grant revocation")
}

// CancelRevoke dismisses the confirmation without changing access.
func (cp *ConsentPage) CancelRevoke(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.GetRevokeDialog(ctx).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}), "cancel grant revocation")
}

// IsRevokeButtonPresent inspects the overflow menu and restores it closed.
// Agents without an active grant have no overflow trigger.
func (cp *ConsentPage) IsRevokeButtonPresent(ctx context.Context) (bool, error) {
	trigger := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Agent actions", Exact: playwright.Bool(true)})
	visible, err := cp.locatorVisible(ctx, trigger, "agent actions")
	if err != nil || !visible {
		return false, err
	}
	if err := cp.OpenOverflowMenu(ctx); err != nil {
		return false, err
	}
	item := cp.GetRevokeButton(ctx)
	present, err := cp.locatorVisible(ctx, item, "revoke all access")
	if closeErr := cp.page().Keyboard().Press("Escape"); closeErr != nil {
		return false, fmt.Errorf("close agent actions: %w", closeErr)
	}
	return present, err
}

// WaitForRevokeDialog waits for the confirmation opened from the detail or overview.
func (cp *ConsentPage) WaitForRevokeDialog(ctx context.Context) error {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.GetRevokeDialog(ctx).WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible, Timeout: timeout,
	}); err != nil {
		return fmt.Errorf("revoke grant dialog did not appear: %w", err)
	}
	return ctx.Err()
}

// TakeRevokeDialogScreenshot captures only the revoke dialog panel.
// This avoids false-positive diffs from dynamic page content behind the modal.
func (cp *ConsentPage) TakeRevokeDialogScreenshot(ctx context.Context, name string) error {
	if err := cp.WaitForRevokeDialog(ctx); err != nil {
		return err
	}
	return cp.TakeLocatorScreenshot(ctx, name, cp.GetRevokeDialog(ctx))
}

// IsRevokeDialogVisible reports whether the RevokeAgentDialog is currently visible.
// Returns false (not an error) when the dialog has been dismissed.
func (cp *ConsentPage) IsRevokeDialogVisible(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.GetRevokeDialog(ctx), "grant revocation dialog")
}

// RevokeDialogContainsText reads only the open confirmation, not the page behind it.
func (cp *ConsentPage) RevokeDialogContainsText(ctx context.Context, text string) (bool, error) {
	count, err := cp.GetRevokeDialog(ctx).GetByText(text).Count()
	if err != nil {
		return false, fmt.Errorf("failed to locate text %q on page: %w", text, err)
	}
	return count > 0, nil
}

// getOverviewRevokeButton selects the first row action in the Agents table.
func (cp *ConsentPage) getOverviewRevokeButton() playwright.Locator {
	return cp.page().GetByRole("table", playwright.PageGetByRoleOptions{Name: "Agents", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}).First()
}

// IsOverviewRevokeButtonPresent reports whether at least one "Revoke" action button
// is visible in the Agents table.
// Waits for the button to appear (delegation list loads asynchronously after navigation).
func (cp *ConsentPage) IsOverviewRevokeButtonPresent(ctx context.Context) (bool, error) {
	btn := cp.getOverviewRevokeButton()
	if err := btn.WaitFor(); err != nil {
		// Timeout means button never appeared — that is a valid "not present" result.
		return false, nil
	}
	return true, nil
}

// ClickOverviewRevokeButton opens confirmation for the first Agents table row.
// Waits for the button to appear before clicking (delegation list loads asynchronously).
func (cp *ConsentPage) ClickOverviewRevokeButton(ctx context.Context) error {
	btn := cp.getOverviewRevokeButton()
	if err := btn.WaitFor(); err != nil {
		return fmt.Errorf("no Revoke button found in the Agents table: %w", err)
	}
	if err := btn.Click(); err != nil {
		return fmt.Errorf("failed to click overview Revoke button: %w", err)
	}
	return nil
}

// WaitForRevokeDialogDismissed waits for the RevokeAgentDialog to fully close.
// Should be called after CancelRevoke or ConfirmRevoke to ensure the dialog has
// fully animated out before asserting on page state.
func (cp *ConsentPage) WaitForRevokeDialogDismissed(ctx context.Context) error {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.GetRevokeDialog(ctx).WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden, Timeout: timeout,
	}); err != nil {
		return fmt.Errorf("revoke grant dialog did not close: %w", err)
	}
	return ctx.Err()
}

// WaitForServiceToAppear waits for a specific service to appear on the page.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceDisplayName: Display name of the service to wait for
//
// Returns an error if the service does not appear within the page timeout.
//
// Example:
//
//	err := consentPage.WaitForServiceToAppear(ctx, "GitHub")
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) WaitForServiceToAppear(ctx context.Context, serviceDisplayName string) error {
	if serviceDisplayName == "" {
		return fmt.Errorf("serviceDisplayName cannot be empty")
	}
	groups, err := cp.PermissionGroups(ctx)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if !group.Checked {
			continue
		}
		if err := cp.ExpandPermissionGroup(ctx, group.Name); err != nil {
			return err
		}
		services, err := cp.GroupServices(ctx, group.Name)
		if err != nil {
			return err
		}
		for _, name := range services {
			if name == serviceDisplayName {
				return nil
			}
		}
	}
	return fmt.Errorf("service %q is not displayed in a selected permission group", serviceDisplayName)
}

// WaitForPageLoad waits for the consent page to be fully interactive — specifically,
// for the agent name heading to appear. Use this after browser-level redirects where
// navigation happens outside of Navigate() (e.g., after GetTestPage().Goto()).
func (cp *ConsentPage) WaitForPageLoad(ctx context.Context) error {
	return cp.waitForAgentNameHeading(ctx)
}

// NavigateToAgentWithSessionToken navigates to the consent page for a CIMD authorization
// flow using a stateless JWE session token. The token encodes the full authorization
// context and is passed as a session_token query parameter.
func (cp *ConsentPage) NavigateToAgentWithSessionToken(ctx context.Context, agentID, sessionToken string) error {
	if agentID == "" {
		return fmt.Errorf("agentID cannot be empty")
	}
	if sessionToken == "" {
		return fmt.Errorf("sessionToken cannot be empty")
	}

	path := fmt.Sprintf("%s?session_token=%s", fmt.Sprintf(agentDetailPath, agentID), url.QueryEscape(sessionToken))
	if err := cp.Navigate(ctx, path); err != nil {
		return fmt.Errorf("failed to navigate to agent consent page with session_token: %w", err)
	}

	if err := cp.waitForAgentNameHeading(ctx); err != nil {
		return fmt.Errorf("agent name heading not found after navigation with session_token: %w", err)
	}

	return nil
}

// IsCIMDSummaryVisible checks the plain-language access request.
func (cp *ConsentPage) IsCIMDSummaryVisible(ctx context.Context) (bool, error) {
	loc := cp.page().GetByText("requests permission to use the services you select below.", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	})
	if err := loc.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	}); err != nil {
		return false, fmt.Errorf("CIMD summary did not become visible: %w", err)
	}
	return true, nil
}

// HasCIMDDomainBadge reports whether the CIMDDomainBadge component ("Verified domain: …")
// is visible on the page.
func (cp *ConsentPage) HasCIMDDomainBadge(ctx context.Context) (bool, error) {
	loc := cp.page().GetByTestId("agent-origin-label").Filter(playwright.LocatorFilterOptions{HasText: "Verified domain:"})
	if err := loc.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	}); err != nil {
		return false, fmt.Errorf("CIMD domain badge did not become visible: %w", err)
	}
	return true, nil
}

// HasCIMDLocalhostWarning checks the prominent local-machine redirect warning.
func (cp *ConsentPage) HasCIMDLocalhostWarning(ctx context.Context) (bool, error) {
	loc := cp.page().GetByTestId("localhost-warning")
	if err := loc.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	}); err != nil {
		return false, fmt.Errorf("CIMD localhost warning did not become visible: %w", err)
	}
	return true, nil
}

// ClickCIMDAdvancedDetails toggles the technical authorization details.
func (cp *ConsentPage) ClickCIMDAdvancedDetails(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Advanced details", Exact: playwright.Bool(true),
	}), "toggle advanced details")
}

// IsCIMDAdvancedDetailsExpanded reads the accordion's disclosure state.
func (cp *ConsentPage) IsCIMDAdvancedDetailsExpanded(ctx context.Context) (bool, error) {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return false, err
	}
	expanded, err := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{
		Name: "Advanced details", Exact: playwright.Bool(true),
	}).GetAttribute("aria-expanded", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return false, fmt.Errorf("read advanced details state: %w", err)
	}
	return expanded == "true", nil
}

// HasCIMDClientName checks the visible identity heading.
func (cp *ConsentPage) HasCIMDClientName(ctx context.Context, name string) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("agent-name-heading").
		GetByRole("heading", playwright.LocatorGetByRoleOptions{Level: playwright.Int(1)}).
		And(cp.page().GetByText(name, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})), "CIMD agent name")
}

// HasCIMDDomainText reports whether the given domain string appears within the verified domain badge.
func (cp *ConsentPage) HasCIMDDomainText(ctx context.Context, domain string) (bool, error) {
	loc := cp.page().GetByTestId("agent-origin-label").Filter(playwright.LocatorFilterOptions{HasText: domain})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check CIMD domain text %q: %w", domain, err)
	}
	return visible, nil
}

// GetCIMDLocalhostWarningText returns the text content of the localhost warning alert.
func (cp *ConsentPage) GetCIMDLocalhostWarningText(ctx context.Context) (string, error) {
	loc := cp.page().GetByTestId("localhost-warning")
	text, err := loc.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get localhost warning text: %w", err)
	}
	return text, nil
}

func (cp *ConsentPage) cimdDetail(label string) playwright.Locator {
	return cp.page().GetByText(label, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).
		Locator("xpath=following-sibling::dd")
}

// HasCIMDRedirectURIInDetails reports whether the given redirect URI value is visible
// in the expanded CIMDAdvancedDetails panel under the "Redirect URI" label.
func (cp *ConsentPage) HasCIMDRedirectURIInDetails(ctx context.Context, uri string) (bool, error) {
	loc := cp.cimdDetail("Redirect URI").Filter(playwright.LocatorFilterOptions{HasText: uri})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check redirect URI %q in advanced details: %w", uri, err)
	}
	return visible, nil
}

// HasCIMDScopeInDetails checks an exact requested scope in expanded CIMD metadata.
func (cp *ConsentPage) HasCIMDScopeInDetails(ctx context.Context, scope string) (bool, error) {
	loc := cp.cimdDetail("Requested scopes").GetByText(scope, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check scope %q in advanced details: %w", scope, err)
	}
	return visible, nil
}

// HasCIMDClientIDInDetails reports whether the given client ID URL is visible in the
// expanded CIMDAdvancedDetails panel under the "Client ID" label.
func (cp *ConsentPage) HasCIMDClientIDInDetails(ctx context.Context, clientID string) (bool, error) {
	loc := cp.cimdDetail("Client ID").Filter(playwright.LocatorFilterOptions{HasText: clientID})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check client ID %q in advanced details: %w", clientID, err)
	}
	return visible, nil
}

// WaitForGrantSuccess waits for the server-confirmed outcome for this route context.
func (cp *ConsentPage) WaitForGrantSuccess(ctx context.Context) error {
	sessionToken, err := cp.GetURLQueryParam("session_token")
	if err != nil {
		return err
	}
	success := cp.grantSavedStatus()
	if sessionToken != "" {
		success = cp.page().GetByTestId("consent-outcome").Filter(playwright.LocatorFilterOptions{HasText: "Access allowed."})
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := success.WaitFor(playwright.LocatorWaitForOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("grant success outcome did not appear: %w", err)
	}
	return nil
}

// WaitForConnectError reads the inline connection failure alert.
func (cp *ConsentPage) WaitForConnectError(ctx context.Context) (string, error) {
	alert := cp.page().GetByRole("alert")
	err := alert.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	})
	if err != nil {
		return "", fmt.Errorf("no connection error appeared: %w", err)
	}

	text, err := alert.First().TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to read connection error: %w", err)
	}

	return strings.TrimSpace(text), nil
}

// GetURLQueryParam returns the value of a query parameter from the current page URL.
func (cp *ConsentPage) GetURLQueryParam(param string) (string, error) {
	currentURL := cp.page().URL()
	parsed, err := url.Parse(currentURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse current URL %q: %w", currentURL, err)
	}
	return parsed.Query().Get(param), nil
}

// PermissionGroup is the displayed selection state, in the order shown to the user.
type PermissionGroup struct {
	Name           string
	Description    string
	Required       bool
	AlreadyGranted bool
	ReadOnly       bool
	Checked        bool
	Expanded       bool
}

// PermissionService is a service control displayed inside an expanded group.
type PermissionService struct {
	Name     string
	Required bool
	ReadOnly bool
	Checked  bool
}

// AgentConnectionRow describes a service required by the agent.
type AgentConnectionRow struct {
	Service string
	State   string
	Action  string
}

type IdentityLink struct {
	Label string
	URL   string
}

func (cp *ConsentPage) OriginLabelText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("agent-origin-label"), "agent origin label")
}

func (cp *ConsentPage) HasLocalhostBanner(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("localhost-warning"), "localhost warning")
}

func (cp *ConsentPage) PermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	var groups []PermissionGroup
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("permission-groups"), `root => {
		const text = (group, id) => {
			const element = group.querySelector('[data-testid="' + id + '"]');
			if (!element) throw new Error('Missing permission group ' + id);
			return element.innerText.trim();
		};
		return Array.from(root.querySelectorAll('[data-testid="permission-group"]')).map(group => {
			const control = group.querySelector('[role="checkbox"][aria-label]');
			const disclosure = group.querySelector('button[aria-expanded]');
			if (!control || !disclosure) throw new Error('Missing permission selection or disclosure');
			return {
				Name: text(group, 'permission-group-name'),
				Description: text(group, 'permission-group-description'),
				Required: !!group.querySelector('[data-testid="permission-group-required"]'),
				AlreadyGranted: !!group.querySelector('[data-testid="permission-group-granted"]'),
				ReadOnly: control.disabled === true,
				Checked: control.getAttribute('aria-checked') === 'true',
				Expanded: disclosure.getAttribute('aria-expanded') === 'true'
			};
		});
	}`, &groups)
	return groups, err
}

func (cp *ConsentPage) permissionGroup(name string) playwright.Locator {
	return cp.page().GetByTestId("permission-group").Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByTestId("permission-group-name").
			And(cp.page().GetByText(name, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})),
	})
}

func (cp *ConsentPage) ExpandPermissionGroup(ctx context.Context, name string) error {
	toggle := cp.permissionGroup(name).Locator("button[aria-expanded]")
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	expanded, err := toggle.GetAttribute("aria-expanded", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("read permission group %q disclosure: %w", name, err)
	}
	if expanded == "true" {
		return nil
	}
	return cp.locatorClick(ctx, toggle, "expand permission group "+name)
}

func (cp *ConsentPage) GroupServices(ctx context.Context, name string) ([]string, error) {
	var services []string
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name), `group =>
		Array.from(group.querySelectorAll('[data-testid="permission-service-name"]'))
			.filter(element => element.getClientRects().length > 0)
			.map(element => element.innerText.trim())`, &services)
	return services, err
}

// PermissionServices expands a group and reads its actual service controls.
func (cp *ConsentPage) PermissionServices(ctx context.Context, name string) ([]PermissionService, error) {
	if err := cp.ExpandPermissionGroup(ctx, name); err != nil {
		return nil, err
	}
	var services []PermissionService
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name).GetByRole("list", playwright.LocatorGetByRoleOptions{
		Name: "Services", Exact: playwright.Bool(true),
	}), `root => Array.from(root.querySelectorAll('[data-testid="permission-service"]')).map(row => {
		const name = row.querySelector('[data-testid="permission-service-name"]');
		const control = row.querySelector('[role="checkbox"]');
		if (!name || !control) throw new Error('Missing permission service name or control');
		return {
			Name: name.innerText.trim(),
			Required: Array.from(row.querySelectorAll('[data-slot="badge"]')).some(badge => badge.innerText.trim() === 'Required'),
			ReadOnly: control.disabled === true,
			Checked: control.getAttribute('aria-checked') === 'true'
		};
	})`, &services)
	return services, err
}

func (cp *ConsentPage) SetPermissionServiceChecked(ctx context.Context, group, service string, checked bool) error {
	if err := cp.ExpandPermissionGroup(ctx, group); err != nil {
		return err
	}
	control := cp.permissionGroup(group).GetByTestId("permission-service").
		Filter(playwright.LocatorFilterOptions{Has: cp.page().GetByTestId("permission-service-name").
			And(cp.page().GetByText(service, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}))}).
		GetByRole("checkbox")
	if err := cp.setCheckboxChecked(ctx, control, checked); err != nil {
		return fmt.Errorf("set service %q in permission group %q checked=%t: %w", service, group, checked, err)
	}
	return nil
}

func (cp *ConsentPage) HasServiceConnectPrompt(ctx context.Context, service string) (bool, error) {
	row := cp.page().GetByTestId("consent-service").Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByText(service, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}),
	})
	return cp.locatorVisible(ctx, row.GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name: "Connect", Exact: playwright.Bool(true),
	}), "connect prompt for "+service)
}

func (cp *ConsentPage) GroupHasRiskIndicator(ctx context.Context, name string) (bool, error) {
	return cp.locatorVisible(ctx, cp.permissionGroup(name).GetByTestId("risk-indicator"), "permission group risk indicator")
}

func (cp *ConsentPage) GroupShowsScopeStrings(ctx context.Context, name string) (bool, error) {
	var visible bool
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name), `group =>
		Array.from(group.querySelectorAll('code, [data-testid="permission-scope"]'))
			.some(element => element.getClientRects().length > 0 && element.innerText.trim() !== '')`, &visible)
	return visible, err
}

func (cp *ConsentPage) ChooseDuration(ctx context.Context, label string) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Access duration", Exact: playwright.Bool(true)}).
		GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose access duration "+label)
}

func (cp *ConsentPage) SelectedDuration(ctx context.Context) (string, error) {
	var selected string
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Access duration", Exact: playwright.Bool(true)}), `root => {
		const radio = root.querySelector('[role="radio"][aria-checked="true"]');
		if (!radio) throw new Error('No access duration is selected');
		const label = (radio.getAttribute('aria-labelledby') || '').split(/\s+/).filter(Boolean)
			.map(id => document.getElementById(id)?.innerText || '').join(' ');
		if (!label.trim()) throw new Error('Selected access duration has no accessible label');
		return label.trim();
	}`, &selected)
	return selected, err
}

func (cp *ConsentPage) CustomDateValue(ctx context.Context) (string, error) {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	value, err := cp.endDateInput().InputValue(playwright.LocatorInputValueOptions{Timeout: timeout})
	if err != nil {
		return "", fmt.Errorf("read custom expiry date: %w", err)
	}
	return value, nil
}

func (cp *ConsentPage) SetCustomDate(ctx context.Context, value string) error {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.endDateInput().Fill(value, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("set custom expiry date: %w", err)
	}
	return nil
}

func (cp *ConsentPage) ClickAllow(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Allow", Exact: playwright.Bool(true)}), "allow access")
}

func (cp *ConsentPage) ClickDeny(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), "deny access")
}

func (cp *ConsentPage) DecisionOutcomeText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("consent-outcome"), "consent outcome")
}

func (cp *ConsentPage) DecisionErrorText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("consent-error"), "consent error")
}

func (cp *ConsentPage) OpenTab(ctx context.Context, label string) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("tab", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "open agent tab "+label)
}

func (cp *ConsentPage) ConnectionsTabRows(ctx context.Context) ([]AgentConnectionRow, error) {
	var rows []AgentConnectionRow
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("tabpanel", playwright.PageGetByRoleOptions{Name: "Connections", Exact: playwright.Bool(true)}).
		GetByRole("list"), `root =>
		Array.from(root.querySelectorAll('[data-testid="agent-connection-row"]')).map(row => {
			const text = id => {
				const element = row.querySelector('[data-testid="' + id + '"]');
				if (!element) throw new Error('Missing connection ' + id);
				return element.innerText.trim();
			};
			const action = row.querySelector('[data-testid="connection-action"]');
			return {Service: text('connection-provider'), State: text('connection-state'), Action: action?.innerText.trim() || ''};
		})`, &rows)
	return rows, err
}

func (cp *ConsentPage) IsSaveBarVisible(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("grant-save-bar"), "grant save bar")
}

func (cp *ConsentPage) CancelChanges(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.page().GetByTestId("grant-save-bar").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}), "cancel grant changes")
}

// SaveChanges waits for the console's visible result, including validation and
// server errors. Callers inspect that result; submitting does not imply success.
func (cp *ConsentPage) SaveChanges(ctx context.Context) error {
	if err := cp.locatorClick(ctx, cp.page().GetByTestId("grant-save-bar").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Save changes", Exact: playwright.Bool(true)}), "save grant changes"); err != nil {
		return err
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	saveErrors := cp.page().GetByRole("main").Locator("form > [role='alert'], form fieldset [role='alert']")
	if err := cp.grantSavedStatus().Or(saveErrors).First().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible, Timeout: timeout,
	}); err != nil {
		return fmt.Errorf("grant save outcome did not appear: %w", err)
	}
	return ctx.Err()
}

func (cp *ConsentPage) OpenOverflowMenu(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Agent actions", Exact: playwright.Bool(true)}), "open agent actions")
}

func (cp *ConsentPage) ChooseRevokeAllAccess(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.GetRevokeButton(ctx), "choose revoke all access")
}

func (cp *ConsentPage) DurationOptions(ctx context.Context) ([]string, error) {
	var labels []string
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Access duration", Exact: playwright.Bool(true)}), `root =>
		Array.from(root.querySelectorAll('[role="radio"]')).map(radio =>
			(radio.getAttribute('aria-labelledby') || '').split(/\s+/).filter(Boolean)
				.map(id => document.getElementById(id)?.innerText || '').join(' ').trim()
		)`, &labels)
	return labels, err
}

func (cp *ConsentPage) NextStepsText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("consent-next-steps"), "consent next steps")
}

func (cp *ConsentPage) HasSecondaryDeny(ctx context.Context) (bool, error) {
	var secondary bool
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), `button =>
		button.getClientRects().length > 0 &&
		button.getAttribute('data-variant') === 'secondary'`, &secondary)
	return secondary, err
}

func (cp *ConsentPage) SetPermissionGroupChecked(ctx context.Context, name string, checked bool) error {
	control := cp.permissionGroup(name).Locator("[role='checkbox'][aria-label]")
	if err := cp.setCheckboxChecked(ctx, control, checked); err != nil {
		return fmt.Errorf("set permission group %q checked=%t: %w", name, checked, err)
	}
	return nil
}

func (cp *ConsentPage) IdentityLinks(ctx context.Context) ([]IdentityLink, error) {
	var links []IdentityLink
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("agent-identity"), `root =>
		Array.from(root.querySelectorAll('a[href]')).map(link => ({Label: link.innerText.trim(), URL: link.getAttribute('href')}))`, &links)
	return links, err
}

func (cp *ConsentPage) RevokeDialogText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.GetRevokeDialog(ctx), "revoke access dialog")
}

func (cp *ConsentPage) ClickConnect(ctx context.Context, service string) error {
	row := cp.page().GetByTestId("consent-service").Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByText(service, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}),
	})
	return cp.locatorClick(ctx, row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Connect", Exact: playwright.Bool(true)}), "connect service "+service)
}

func (cp *ConsentPage) HasOriginLabel(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("agent-origin-label"), "agent origin label")
}
