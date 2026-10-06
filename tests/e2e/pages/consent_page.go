// Package pages provides page object models for frontend E2E testing.
// Page objects abstract UI selectors and interactions into high-level methods
// that tests call, making tests more readable and maintainable.
package pages

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/mxschmitt/playwright-go"
)

// Decision and detail share /agents/:id; presence of session_token selects the decision.
const (
	agentDetailPath   = "/agents/%s"
	overviewPath      = "/agents"
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
// The /agents/:id route renders the grant editor when no session_token is present.
// This method waits for the agent heading before returning.
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

// NavigateToOverview opens the agent collection at /agents.
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
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.page().GetByTestId("agent-name-heading").
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("agent heading did not appear: %w", err)
	}
	return ctx.Err()
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
	hasToken, err := cp.hasSessionToken()
	if err != nil {
		return err
	}
	if hasToken {
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
	var name string
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("agent-name-heading"), `root => {
		const name = root.querySelector('[data-testid="agent-name"]') || root.querySelector('h1');
		if (!name) throw new Error('Missing visible agent name');
		return name.innerText.trim();
	}`, &name)
	return name, err
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
	return cp.page().GetByRole("main").Locator("[role='alert']:not([data-testid='localhost-warning']):not([data-testid='consent-risk'])")
}

func (cp *ConsentPage) grantSavedStatus() playwright.Locator {
	return cp.page().Locator("[data-sonner-toaster] [data-sonner-toast][data-type='success']").Filter(playwright.LocatorFilterOptions{
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
// Reads decision/editor errors without treating risk or localhost warnings as failures.
//
// Example:
//
//	msg, err := consentPage.GetErrorMessage(ctx)
//	// err will be non-nil if no error message is displayed
//	if err == nil {
//	  fmt.Printf("Error: %s\n", msg)
//	}
func (cp *ConsentPage) GetErrorMessage(ctx context.Context) (string, error) {
	visible, err := cp.locatorVisible(ctx, cp.grantErrors().First(), "grant error alert")
	if err != nil {
		return "", err
	}
	if !visible {
		return "", fmt.Errorf("no grant error message is visible")
	}
	text, err := cp.locatorText(ctx, cp.grantErrors().First(), "grant error message")
	if err != nil {
		return "", err
	}
	if text == "" {
		return "", fmt.Errorf("grant error message is empty")
	}
	return text, nil
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
	return cp.locatorVisible(ctx, cp.grantErrors().First(), "grant error alert")
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
		GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: permissionSetName, Exact: playwright.Bool(true)}), "toggle permission group "+permissionSetName)
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

// GetRevokeButton selects the direct, visible grant-revocation action.
func (cp *ConsentPage) GetRevokeButton(ctx context.Context) playwright.Locator {
	return cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Revoke access", Exact: playwright.Bool(true)})
}

func (cp *ConsentPage) ClickRevokeButton(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.GetRevokeButton(ctx), "revoke agent access")
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

// Agents without an active grant have no revocation action on the detail page.
func (cp *ConsentPage) IsRevokeButtonPresent(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.GetRevokeButton(ctx), "revoke access button")
}

func (cp *ConsentPage) RevokeCancelHasFocus(ctx context.Context) (bool, error) {
	var focused bool
	err := cp.evaluateJSON(ctx, cp.GetRevokeDialog(ctx).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}),
		`button => button === document.activeElement`, &focused)
	return focused, err
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

// The overview's entity actions are independent of whole-card links.
func (cp *ConsentPage) getOverviewRevokeButton() playwright.Locator {
	return cp.page().GetByTestId("agents-collection").GetByTestId("agent-entity").
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}).First()
}

func (cp *ConsentPage) IsOverviewRevokeButtonPresent(ctx context.Context) (bool, error) {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return false, err
	}
	if err := cp.getOverviewRevokeButton().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err()
		}
		if isPlaywrightTimeout(err) {
			return false, nil
		}
		return false, fmt.Errorf("locate Agents revoke button: %w", err)
	}
	return true, ctx.Err()
}

func (cp *ConsentPage) ClickOverviewRevokeButton(ctx context.Context) error {
	return cp.locatorClick(ctx, cp.getOverviewRevokeButton(), "revoke access on first agent card")
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
	// The query parameter's presence routes to the decision even for an invalid empty token.

	path := fmt.Sprintf("%s?session_token=%s", fmt.Sprintf(agentDetailPath, agentID), url.QueryEscape(sessionToken))
	if err := cp.Navigate(ctx, path); err != nil {
		return fmt.Errorf("failed to navigate to agent consent page with session_token: %w", err)
	}

	if err := cp.waitForAgentNameHeading(ctx); err != nil {
		return fmt.Errorf("agent name heading not found after navigation with session_token: %w", err)
	}

	return nil
}

// IsCIMDSummaryVisible checks the explicit access request in the decision heading.
func (cp *ConsentPage) IsCIMDSummaryVisible(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("agent-name-heading").
		Filter(playwright.LocatorFilterOptions{HasText: "wants to act on your behalf"}), "agent access request summary")
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

// OpenTechnicalDetails reveals raw client and redirect metadata in a named dialog.
func (cp *ConsentPage) OpenTechnicalDetails(ctx context.Context) error {
	if err := cp.locatorClick(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Technical details", Exact: playwright.Bool(true)}), "open technical details"); err != nil {
		return err
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Technical details", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("technical details dialog did not appear: %w", err)
	}
	return ctx.Err()
}

func (cp *ConsentPage) IsTechnicalDetailsOpen(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Technical details", Exact: playwright.Bool(true)}), "technical details dialog")
}

// HasCIMDClientName checks the exact agent identity, not the heading suffix.
func (cp *ConsentPage) HasCIMDClientName(ctx context.Context, name string) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("agent-name").And(
		cp.page().GetByText(name, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})), "CIMD agent name")
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
	return cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Technical details", Exact: playwright.Bool(true)}).
		GetByText(label, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}).Locator("xpath=following-sibling::dd")
}

// HasCIMDRedirectURIInDetails checks the redirect in the technical-details dialog.
func (cp *ConsentPage) HasCIMDRedirectURIInDetails(ctx context.Context, uri string) (bool, error) {
	return cp.locatorVisible(ctx, cp.cimdDetail("Redirect URI").Filter(playwright.LocatorFilterOptions{HasText: uri}), "redirect URI in technical details")
}

// HasCIMDScopeInDetails checks an exact requested scope in the technical-details dialog.
func (cp *ConsentPage) HasCIMDScopeInDetails(ctx context.Context, scope string) (bool, error) {
	return cp.locatorVisible(ctx, cp.cimdDetail("Requested scopes").GetByText(scope, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}), "requested scope in technical details")
}

// HasCIMDClientIDInDetails checks the client ID in the technical-details dialog.
func (cp *ConsentPage) HasCIMDClientIDInDetails(ctx context.Context, clientID string) (bool, error) {
	return cp.locatorVisible(ctx, cp.cimdDetail("Client ID").Filter(playwright.LocatorFilterOptions{HasText: clientID}), "client ID in technical details")
}

// WaitForGrantSuccess waits for the server-confirmed outcome for this route context.
func (cp *ConsentPage) WaitForGrantSuccess(ctx context.Context) error {
	hasToken, err := cp.hasSessionToken()
	if err != nil {
		return err
	}
	success := cp.grantSavedStatus()
	if hasToken {
		success = cp.page().GetByTestId("consent-outcome").Filter(playwright.LocatorFilterOptions{HasText: "Access allowed."})
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := success.WaitFor(playwright.LocatorWaitForOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("grant success outcome did not appear: %w", err)
	}
	return ctx.Err()
}

// WaitForConnectError reads the failed consent decision or console connection result.
func (cp *ConsentPage) WaitForConnectError(ctx context.Context) (string, error) {
	alert := cp.page().GetByTestId("consent-error").Or(cp.page().GetByTestId("agent-connections").GetByRole("alert")).First()
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	if err := alert.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return "", fmt.Errorf("connection error did not appear: %w", err)
	}
	return cp.locatorText(ctx, alert, "connection error")
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

func (cp *ConsentPage) hasSessionToken() (bool, error) {
	currentURL := cp.page().URL()
	parsed, err := url.Parse(currentURL)
	if err != nil {
		return false, fmt.Errorf("parse current URL %q: %w", currentURL, err)
	}
	return parsed.Query().Has("session_token"), nil
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

func (cp *ConsentPage) PrimaryOriginLabelText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByTestId("agent-origin-label").Locator(":scope > span:first-child"), "agent origin label")
}

func (cp *ConsentPage) ReturnOriginText(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	returnOrigin := cp.page().GetByTestId("agent-origin-label").Locator(":scope > span:nth-child(2)")
	count, err := returnOrigin.Count()
	if err != nil {
		return "", fmt.Errorf("inspect return origin: %w", err)
	}
	if count == 0 {
		return "", ctx.Err()
	}
	return cp.locatorText(ctx, returnOrigin, "return origin")
}

func (cp *ConsentPage) HasLocalhostBanner(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("localhost-warning"), "localhost warning")
}

func (cp *ConsentPage) PermissionGroups(ctx context.Context) ([]PermissionGroup, error) {
	var groups []PermissionGroup
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("permission-groups"), `root =>
		[...root.querySelectorAll('[data-testid="permission-group"]')].map(group => {
			const control = group.querySelector('[role="checkbox"]');
			const name = group.querySelector('[data-testid="permission-group-name"]');
			const description = group.querySelector('[data-testid="permission-group-description"]');
			if (!name || !description || (!control && group.dataset.readOnly !== 'true')) throw new Error('Permission group is missing its name, description or selection');
			const required = [...name.parentElement.children].some(element =>
				element !== name && element.textContent.trim() === 'Required');
			const descriptionText = description.cloneNode(true);
			descriptionText.querySelectorAll('[aria-hidden="true"],button,[data-testid="permission-group-granted"]').forEach(node => node.remove());
			const disclosure = group.querySelector('button[aria-expanded]');
			return {Name: name.innerText.trim(), Description: descriptionText.textContent.trim(),
				Required: required, AlreadyGranted: !!group.querySelector('[data-testid="permission-group-granted"]'),
				ReadOnly: control ? control.disabled === true : group.dataset.readOnly === 'true',
				Checked: control ? control.getAttribute('aria-checked') === 'true' : group.dataset.selected === 'true',
				Expanded: disclosure?.getAttribute('aria-expanded') === 'true'};
		})`, &groups)
	return groups, err
}

func (cp *ConsentPage) permissionGroup(name string) playwright.Locator {
	return cp.page().GetByTestId("permission-group").Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByTestId("permission-group-name").
			And(cp.page().GetByText(name, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})),
	})
}

func (cp *ConsentPage) HasServiceDisclosure(ctx context.Context, name string) (bool, error) {
	return cp.locatorVisible(ctx, cp.permissionGroup(name).GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name: "Choose services for " + name, Exact: playwright.Bool(true),
	}), "service disclosure for "+name)
}

func (cp *ConsentPage) ExpandPermissionGroup(ctx context.Context, name string) error {
	toggle := cp.permissionGroup(name).GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name: "Choose services for " + name, Exact: playwright.Bool(true),
	})
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	expanded, err := toggle.GetAttribute("aria-expanded", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("group %q has no service disclosure: %w", name, err)
	}
	if expanded == "true" {
		return nil
	}
	return cp.locatorClick(ctx, toggle, "expand services for "+name)
}

func (cp *ConsentPage) GroupServices(ctx context.Context, name string) ([]string, error) {
	var services []string
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name), `group => {
		const names = [...group.querySelectorAll('[data-testid="permission-service-name"]')].map(node => node.textContent.trim()).filter(Boolean);
		return [...new Set(names.length ? names : [...group.querySelectorAll('[data-slot="avatar"][role="img"]')].map(avatar => avatar.getAttribute('aria-label')).filter(Boolean))];
	}`, &services)
	return services, err
}

func (cp *ConsentPage) serviceControls(ctx context.Context, group string) (playwright.Locator, error) {
	inline := cp.permissionGroup(group).GetByTestId("permission-service")
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	count, err := inline.Count()
	if err != nil {
		return nil, fmt.Errorf("locate inline services for %q: %w", group, err)
	}
	if count > 0 {
		return inline, nil
	}
	visible, err := cp.HasServiceDisclosure(ctx, group)
	if err != nil {
		return nil, err
	}
	if !visible {
		return nil, fmt.Errorf("permission group %q has no service controls or disclosure", group)
	}
	toggle := cp.permissionGroup(group).GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name: "Choose services for " + group, Exact: playwright.Bool(true),
	})
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return nil, err
	}
	popup, err := toggle.GetAttribute("aria-haspopup", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("inspect service disclosure for %q: %w", group, err)
	}
	if err := cp.ExpandPermissionGroup(ctx, group); err != nil {
		return nil, err
	}
	if popup == "dialog" {
		popover := cp.page().GetByTestId("permission-services-popover").And(cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{
			Name: "Services for " + group, Exact: playwright.Bool(true),
		}))
		if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
			return nil, fmt.Errorf("service choices for %q did not appear: %w", group, err)
		}
		return popover.GetByTestId("permission-service"), ctx.Err()
	}
	if err := inline.First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return nil, fmt.Errorf("expanded inline services for %q did not appear: %w", group, err)
	}
	return inline, ctx.Err()
}

// PermissionServices observes the real service controls: inline in console mode,
// or in the opened popover for a multi-service decision group.
func (cp *ConsentPage) PermissionServices(ctx context.Context, name string) ([]PermissionService, error) {
	controls, err := cp.serviceControls(ctx, name)
	if err != nil {
		return nil, err
	}
	var services []PermissionService
	err = cp.evaluateJSON(ctx, controls.First().Locator("xpath=.."), `root => [...root.querySelectorAll('[data-testid="permission-service"]')].map(row => {
		const name = row.querySelector('[data-testid="permission-service-name"]');
		const control = row.querySelector('[role="checkbox"],button[aria-pressed]');
		if (!name) throw new Error('Missing permission service name');
		return {Name: name.innerText.trim(), Required: row.dataset.required === 'true',
			ReadOnly: control ? control.disabled === true : row.dataset.readOnly === 'true',
			Checked: control ? (control.getAttribute('aria-checked') ?? control.getAttribute('aria-pressed')) === 'true' : row.dataset.selected === 'true'};
	})`, &services)
	return services, err
}

func (cp *ConsentPage) SetPermissionServiceChecked(ctx context.Context, group, service string, checked bool) error {
	controls, err := cp.serviceControls(ctx, group)
	if err != nil {
		return err
	}
	row := controls.Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByTestId("permission-service-name").GetByText(service, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}),
	})
	toggle := row.Locator("button[aria-pressed]")
	count, err := toggle.Count()
	if err != nil {
		return fmt.Errorf("locate service choice: %w", err)
	}
	if count > 0 {
		timeout, err := cp.locatorTimeout(ctx)
		if err != nil {
			return err
		}
		state, err := toggle.GetAttribute("aria-pressed", playwright.LocatorGetAttributeOptions{Timeout: timeout})
		if err != nil {
			return fmt.Errorf("read service selection: %w", err)
		}
		if (state == "true") != checked {
			return cp.locatorClick(ctx, toggle, "select service "+service)
		}
		return ctx.Err()
	}
	if err := cp.setCheckboxChecked(ctx, row.GetByRole("checkbox"), checked); err != nil {
		return fmt.Errorf("set service %q in permission group %q checked=%t: %w", service, group, checked, err)
	}
	return nil
}

func (cp *ConsentPage) HasServiceConnectPrompt(ctx context.Context, service string) (bool, error) {
	rail := cp.page().GetByTestId("agent-connections")
	console, err := cp.locatorVisible(ctx, rail, "agent connections")
	if err != nil {
		return false, err
	}
	if console {
		row := rail.GetByTestId("agent-connection-row").Filter(playwright.LocatorFilterOptions{
			Has: cp.page().GetByTestId("connection-provider").And(cp.page().GetByText(service, playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})),
		})
		connect := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Connect " + service, Exact: playwright.Bool(true)})
		reconnect := row.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Reconnect " + service, Exact: playwright.Bool(true)})
		return cp.locatorVisible(ctx, connect.Or(reconnect), "connection action for "+service)
	}
	group := cp.page().GetByTestId("permission-group").Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByRole("img", playwright.PageGetByRoleOptions{Name: service, Exact: playwright.Bool(true)}),
	})
	action := group.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Connect " + service, Exact: playwright.Bool(true)}).First()
	visible, err := cp.locatorVisible(ctx, action, "connection action for "+service)
	if err != nil || visible {
		return visible, err
	}
	name, err := cp.locatorText(ctx, group.GetByTestId("permission-group-name"), "permission group for "+service)
	if err != nil {
		return false, err
	}
	disclosure, err := cp.HasServiceDisclosure(ctx, name)
	if err != nil {
		return false, err
	}
	if !disclosure {
		count, err := group.GetByTestId("permission-service").Count()
		if err != nil {
			return false, fmt.Errorf("locate service controls for %q: %w", name, err)
		}
		if count == 0 {
			return false, nil
		}
	}
	popover := cp.page().GetByTestId("permission-services-popover").And(cp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{
		Name: "Services for " + name, Exact: playwright.Bool(true),
	}))
	wasOpen, err := cp.locatorVisible(ctx, popover, "service choices for "+name)
	if err != nil {
		return false, err
	}
	controls, err := cp.serviceControls(ctx, name)
	if err != nil {
		return false, err
	}
	prompt := controls.Filter(playwright.LocatorFilterOptions{
		Has: cp.page().GetByTestId("permission-service-name").GetByText(service, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}),
	}).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Connect " + service, Exact: playwright.Bool(true)})
	visible, err = cp.locatorVisible(ctx, prompt, "missing-service action for "+service)
	if err != nil {
		return false, err
	}
	open, err := cp.locatorVisible(ctx, popover, "service choices for "+name)
	if err != nil {
		return false, err
	}
	if open && !wasOpen {
		if err := cp.page().Keyboard().Press("Escape"); err != nil {
			return false, fmt.Errorf("close service choices for %q: %w", name, err)
		}
		timeout, err := cp.locatorTimeout(ctx)
		if err != nil {
			return false, err
		}
		if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
			return false, fmt.Errorf("service choices for %q did not close: %w", name, err)
		}
	}
	return visible, ctx.Err()
}

func (cp *ConsentPage) GroupHasRiskRating(ctx context.Context, name string) (bool, error) {
	var rated bool
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name), `group =>
		[...group.querySelectorAll('[data-slot="badge"], [role="status"]')].some(el =>
			/^(low|medium|high|critical)\s+risk\b/i.test(el.innerText.trim()))`, &rated)
	return rated, err
}

func (cp *ConsentPage) GroupShowsScopeStrings(ctx context.Context, name string) (bool, error) {
	var visible bool
	err := cp.evaluateJSON(ctx, cp.permissionGroup(name), `group =>
		Array.from(group.querySelectorAll('code, [data-testid="permission-scope"]'))
			.some(element => element.getClientRects().length > 0 && element.innerText.trim() !== '')`, &visible)
	return visible, err
}

func (cp *ConsentPage) ChooseDuration(ctx context.Context, label string) error {
	if label == "Custom date" {
		selected, err := cp.SelectedDuration(ctx)
		if err != nil {
			return err
		}
		if selected == label {
			return cp.openCustomDate(ctx)
		}
	}
	if err := cp.locatorClick(ctx, cp.page().GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Access lasts", Exact: playwright.Bool(true)}), "open access duration"); err != nil {
		return err
	}
	if err := cp.locatorClick(ctx, cp.page().GetByRole("option", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose duration "+label); err != nil {
		return err
	}
	if label == "Custom date" {
		// Selecting Custom date opens its picker after the duration menu closes.
		return cp.waitForCustomDate(ctx)
	}
	return nil
}

func (cp *ConsentPage) SelectedDuration(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.page().GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Access lasts", Exact: playwright.Bool(true)}), "selected duration")
}

func (cp *ConsentPage) waitForCustomDate(ctx context.Context) error {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.endDateInput().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("custom date picker did not open: %w", err)
	}
	return ctx.Err()
}

func (cp *ConsentPage) openCustomDate(ctx context.Context) error {
	trigger := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Choose custom date", Exact: playwright.Bool(true)})
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	expanded, err := trigger.GetAttribute("aria-expanded", playwright.LocatorGetAttributeOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("read custom date picker state: %w", err)
	}
	if expanded == "false" {
		if err := cp.locatorClick(ctx, trigger, "open custom date picker"); err != nil {
			return err
		}
	} else if expanded != "true" {
		return fmt.Errorf("custom date picker has unexpected aria-expanded value %q", expanded)
	}
	// The picker loads lazily: an expanded trigger can precede its date input.
	return cp.waitForCustomDate(ctx)
}

func (cp *ConsentPage) closeCustomDate(ctx context.Context) error {
	if err := cp.page().Keyboard().Press("Escape"); err != nil {
		return fmt.Errorf("close custom date picker: %w", err)
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.endDateInput().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return fmt.Errorf("custom date picker did not close: %w", err)
	}
	return ctx.Err()
}

func (cp *ConsentPage) CustomDateValue(ctx context.Context) (string, error) {
	if err := cp.openCustomDate(ctx); err != nil {
		return "", err
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	value, err := cp.endDateInput().InputValue(playwright.LocatorInputValueOptions{Timeout: timeout})
	if err != nil {
		return "", fmt.Errorf("read custom expiry date: %w", err)
	}
	return value, cp.closeCustomDate(ctx)
}

func (cp *ConsentPage) SetCustomDate(ctx context.Context, value string) error {
	if err := cp.openCustomDate(ctx); err != nil {
		return err
	}
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := cp.endDateInput().Fill(value, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("set custom expiry date: %w", err)
	}
	return cp.closeCustomDate(ctx)
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

func (cp *ConsentPage) ConnectionsRailRows(ctx context.Context) ([]AgentConnectionRow, error) {
	var rows []AgentConnectionRow
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("agent-connections"), `root =>
		[...root.querySelectorAll('[data-testid="agent-connection-row"]')].map(row => {
			const text = id => {
				const element = row.querySelector('[data-testid="' + id + '"]');
				if (!element) throw new Error('Missing agent connection ' + id);
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

func (cp *ConsentPage) DurationOptions(ctx context.Context) ([]string, error) {
	if err := cp.locatorClick(ctx, cp.page().GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Access lasts", Exact: playwright.Bool(true)}), "open access duration options"); err != nil {
		return nil, err
	}
	var options []string
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("listbox"), `list => [...list.querySelectorAll('[role="option"]')]
		.map(option => option.querySelector('[data-radix-select-item-text]')?.innerText.trim() || option.innerText.trim())`, &options)
	if err != nil {
		return nil, err
	}
	if err := cp.page().Keyboard().Press("Escape"); err != nil {
		return nil, fmt.Errorf("close access duration options: %w", err)
	}
	return options, ctx.Err()
}

func (cp *ConsentPage) AgentsManagementHref(ctx context.Context) (string, error) {
	var href string
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("consent-next-steps"), `paragraph => paragraph.querySelector('a').getAttribute('href')`, &href)
	return href, err
}

func (cp *ConsentPage) HasOutlinedDeny(ctx context.Context) (bool, error) {
	var outlined bool
	err := cp.evaluateJSON(ctx, cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Deny", Exact: playwright.Bool(true)}), `button =>
		button.getClientRects().length > 0 && button.getAttribute('data-variant') === 'outline'`, &outlined)
	return outlined, err
}

func (cp *ConsentPage) SetPermissionGroupChecked(ctx context.Context, name string, checked bool) error {
	control := cp.permissionGroup(name).GetByRole("checkbox", playwright.LocatorGetByRoleOptions{Name: name, Exact: playwright.Bool(true)})
	if err := cp.setCheckboxChecked(ctx, control, checked); err != nil {
		return fmt.Errorf("set permission group %q checked=%t: %w", name, checked, err)
	}
	return nil
}

func (cp *ConsentPage) IdentityLinks(ctx context.Context) ([]IdentityLink, error) {
	var links []IdentityLink
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("agent-about"), `root =>
		[...root.querySelectorAll('a[href]')].map(link => ({Label: link.innerText.trim(), URL: link.getAttribute('href')}))`, &links)
	return links, err
}

func (cp *ConsentPage) RevokeDialogText(ctx context.Context) (string, error) {
	return cp.locatorText(ctx, cp.GetRevokeDialog(ctx), "revoke access dialog")
}

func (cp *ConsentPage) ClickConnect(ctx context.Context, service string) error {
	popover := cp.page().GetByTestId("permission-services-popover")
	open, err := cp.locatorVisible(ctx, popover, "service choices")
	if err != nil {
		return err
	}
	if open {
		choice := popover.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Connect " + service, Exact: playwright.Bool(true)})
		visible, err := cp.locatorVisible(ctx, choice, "service choice for "+service)
		if err != nil {
			return err
		}
		if visible {
			return cp.locatorClick(ctx, choice, "connect service "+service)
		}
		if err := cp.page().Keyboard().Press("Escape"); err != nil {
			return fmt.Errorf("close service choices before connecting %q: %w", service, err)
		}
		timeout, err := cp.locatorTimeout(ctx)
		if err != nil {
			return err
		}
		if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
			return fmt.Errorf("service choices did not close before connecting %q: %w", service, err)
		}
	}
	detail := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Connect " + service, Exact: playwright.Bool(true)})
	reconnect := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Reconnect " + service, Exact: playwright.Bool(true)})
	return cp.locatorClick(ctx, detail.Or(reconnect).First(), "connect service "+service)
}

func (cp *ConsentPage) IsAllowEnabled(ctx context.Context) (bool, error) {
	timeout, err := cp.locatorTimeout(ctx)
	if err != nil {
		return false, err
	}
	return cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Allow", Exact: playwright.Bool(true)}).
		IsEnabled(playwright.LocatorIsEnabledOptions{Timeout: timeout})
}

func (cp *ConsentPage) HasOriginLabel(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("agent-origin-label"), "agent origin label")
}

func (cp *ConsentPage) HasSignedInAs(ctx context.Context, principal string) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByTestId("consent-account").Filter(playwright.LocatorFilterOptions{
		HasText: principal,
	}), "signed-in identity "+principal)
}

func (cp *ConsentPage) DecisionCardWidth(ctx context.Context) (float64, error) {
	var width float64
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("consent-card"), `card => card.getBoundingClientRect().width`, &width)
	return width, err
}

func (cp *ConsentPage) AllowViewportBottom(ctx context.Context) (float64, error) {
	var bottom float64
	err := cp.evaluateJSON(ctx, cp.page().GetByTestId("consent-footer").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Allow", Exact: playwright.Bool(true)}),
		`button => button.getBoundingClientRect().bottom`, &bottom)
	return bottom, err
}

func (cp *ConsentPage) HasDetailTabs(ctx context.Context) (bool, error) {
	return cp.locatorVisible(ctx, cp.page().GetByRole("main").GetByRole("tablist"), "agent detail tab navigation")
}
