// Package pages provides page object models for frontend E2E testing.
// Page objects abstract UI selectors and interactions into high-level methods
// that tests call, making tests more readable and maintainable.
package pages

import (
	"context"
	"encoding/base64"
	"encoding/json"
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
	revokeDialogTitle = "Revoke All Access"
)

// ConsentPage represents the OAuth2 consent flow page where users grant
// OAuth2 scopes to agents. This page is actually the AgentGrantDetailPage
// in the React app, which allows users to delegate services and scopes.
//
// ConsentPage wraps the base Page object for common navigation and utilities.
// All selectors use Playwright's semantic methods (GetByRole, GetByLabel, GetByText)
// to interact with accessible UI elements, never relying on HTML structure or CSS classes.
//
// Usage pattern:
//
//	consentPage := NewConsentPage(page, baseURL)
//	defer consentPage.Close()
//	consentPage.NavigateToAgent(ctx, "agent-id")
//	consentPage.SelectScope(ctx, "user:read")
//	consentPage.SubmitConsent(ctx)
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
// The React app serves the overview at the root route ("/").
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

func (cp *ConsentPage) getConsentActionButton(label string) playwright.Locator {
	return cp.page().GetByRole(
		"button",
		playwright.PageGetByRoleOptions{Name: label},
	)
}

func (cp *ConsentPage) isConsentActionEnabled(label string) (bool, error) {
	button := cp.getConsentActionButton(label)
	count, err := button.Count()
	if err != nil {
		return false, fmt.Errorf("failed to locate consent action button: %w", err)
	}
	if count == 0 {
		return false, fmt.Errorf("consent action button not found")
	}

	enabled, err := button.IsEnabled()
	if err != nil {
		return false, fmt.Errorf("failed to check consent action button enabled state: %w", err)
	}

	return enabled, nil
}

// waitForAgentNameHeading waits for the main h1 heading (agent name) to appear.
// This waits for the page to load the agent detail content (not the header).
// Uses data-testid to avoid ambiguity when multiple h1 elements exist on page.
func (cp *ConsentPage) waitForAgentNameHeading(ctx context.Context) error {
	agentNameHeading := cp.page().GetByTestId("agent-name-heading")
	err := agentNameHeading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(cp.timeout.Milliseconds())),
	})
	if err != nil {
		return fmt.Errorf("agent name heading not found after waiting %v (page may not have loaded): %w", cp.timeout, err)
	}
	return nil
}

// SelectScope is not applicable in the current UI (Phase 8 - Simplified UI).
//
// In the current design, scopes are displayed as read-only StatusIndicator badges.
// Service delegation happens at the service level (Login/Delegate buttons), not at the scope level.
// Scopes are automatically included based on service requirements.
//
// This method is kept for API compatibility but always returns an error.
//
// Returns:
//   - error: Always returns an error indicating scopes are not selectable
//
// Deprecated: Scopes are determined by service requirements, not user selection.
func (cp *ConsentPage) SelectScope(ctx context.Context, scopeName string) error {
	if scopeName == "" {
		return fmt.Errorf("scopeName cannot be empty")
	}

	// In the current UI (Phase 8), scopes are not selectable.
	// Scopes are automatically included based on service requirements.
	// Use DelegateService() instead to grant access to a service with its required scopes.
	return fmt.Errorf("scope selection is not available in current UI; scopes are determined by service requirements. Use DelegateService() to grant service access")
}

// ClearScope is not applicable in the current UI (Phase 8 - Simplified UI).
//
// In the current design, scopes are displayed as read-only StatusIndicator badges.
// Scopes cannot be individually cleared; instead, revoke access to the entire service using RevokeService().
//
// This method is kept for API compatibility but always returns an error.
//
// Returns:
//   - error: Always returns an error indicating scopes are not modifiable
//
// Deprecated: Use RevokeService() instead to revoke all scopes for a service.
func (cp *ConsentPage) ClearScope(ctx context.Context, scopeName string) error {
	if scopeName == "" {
		return fmt.Errorf("scopeName cannot be empty")
	}

	// In the current UI (Phase 8), scopes are not individually modifiable.
	// Use RevokeService() instead to revoke all scopes for a service.
	return fmt.Errorf("scope clearing is not available in current UI; scopes are determined by service requirements. Use RevokeService() to revoke all scopes for a service")
}

// SubmitConsent clicks the "Approve & Delegate" action for a new grant.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - error: If the button is not found or not clickable
//
// The button may be disabled if mandatory requirements are not met.
func (cp *ConsentPage) SubmitConsent(ctx context.Context) error {
	button := cp.getConsentActionButton("Approve & Delegate")
	count, err := button.Count()
	if err != nil {
		return fmt.Errorf("failed to locate consent submit button: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("approve and delegate button not found")
	}

	enabled, err := button.IsEnabled()
	if err != nil {
		return fmt.Errorf("failed to check button enabled state: %w", err)
	}
	if !enabled {
		return fmt.Errorf("approve and delegate button is not enabled (mandatory services may not be connected)")
	}

	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click approve and delegate button: %w", err)
	}

	return nil
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

	// Find expiration input by label
	input := cp.page().GetByLabel("Expiration")

	// Check if element exists
	count, err := input.Count()
	if err != nil {
		return fmt.Errorf("failed to count expiration input: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("expiration input not found")
	}

	// Calculate expiration date
	expirationDate := time.Now().AddDate(0, 0, expirationDays).Format("2006-01-02")

	// Fill the input
	if err := input.Fill(expirationDate); err != nil {
		return fmt.Errorf("failed to set expiration date: %w", err)
	}

	return nil
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
	// Find expiration input by label
	input := cp.page().GetByLabel("Expiration")

	// Check if element exists
	count, err := input.Count()
	if err != nil {
		return fmt.Errorf("failed to count expiration input: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("expiration input not found")
	}

	// Format date as ISO 8601
	dateStr := date.Format("2006-01-02")

	// Fill the input
	if err := input.Fill(dateStr); err != nil {
		return fmt.Errorf("failed to set expiration date: %w", err)
	}

	return nil
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
	// Find h1 heading in the main content area using semantic role
	heading := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Level: playwright.Int(1)},
	).First()

	// Check if element exists
	count, err := heading.Count()
	if err != nil {
		return "", fmt.Errorf("failed to count h1 heading: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("agent name heading not found")
	}

	// Get the text content
	text, err := heading.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get agent name heading: %w", err)
	}

	if text == "" {
		return "", fmt.Errorf("agent name heading is empty")
	}

	return strings.TrimSpace(text), nil
}

// GetAvailableScopes retrieves all available scopes displayed on the page.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - []string: Slice of scope names (e.g., ["repo", "user"])
//   - error: If no scopes found
//
// Scopes are displayed as StatusIndicator badges in the "Permissions" sections
// of each service card. This method finds all visible scope labels on the page.
//
// Example:
//
//	scopes, err := consentPage.GetAvailableScopes(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(scopes).To(ContainElement("repo"))
func (cp *ConsentPage) GetAvailableScopes(ctx context.Context) ([]string, error) {
	// StatusIndicators use data-testid for semantic identification
	// Pattern: data-testid="scope-indicator-*"
	indicators := cp.page().Locator("[data-testid^='scope-indicator-']")

	// Get count of indicators
	count, err := indicators.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count scope indicators: %w", err)
	}
	if count == 0 {
		return nil, fmt.Errorf("no scopes found on page")
	}

	scopes := []string{}
	seen := make(map[string]bool) // Track unique scopes

	// Iterate through each StatusIndicator
	for i := 0; i < count; i++ {
		indicator := indicators.Nth(i)

		// Get all spans within the indicator
		// The last span contains the scope label (first span is the icon)
		spans := indicator.Locator("span")
		spanCount, _ := spans.Count()

		if spanCount > 0 {
			// Get the last span which contains the label
			lastSpan := spans.Last()
			text, err := lastSpan.TextContent()
			if err == nil && text != "" {
				text = strings.TrimSpace(text)
				// Avoid duplicates and skip non-scope text (like "Permissions")
				if !seen[text] && text != "" && !strings.Contains(strings.ToLower(text), "permissions") {
					scopes = append(scopes, text)
					seen[text] = true
				}
			}
		}
	}

	if len(scopes) == 0 {
		return nil, fmt.Errorf("no scope labels found")
	}

	return scopes, nil
}

// GetSelectedScopes retrieves all currently selected/checked scopes.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - []string: Slice of selected scope names
//   - error: Only if there's an error accessing the page
//
// Returns an empty slice if no scopes are selected (not an error).
//
// Example:
//
//	selected, err := consentPage.GetSelectedScopes(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(selected).To(HaveLen(2))
func (cp *ConsentPage) GetSelectedScopes(ctx context.Context) ([]string, error) {
	// Find all checkboxes
	checkboxes := cp.page().GetByRole("checkbox")

	// Get count
	count, err := checkboxes.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count checkboxes: %w", err)
	}

	selected := []string{}

	// Iterate through each checkbox and check if it's checked
	for i := 0; i < count; i++ {
		checkbox := checkboxes.Nth(i)

		// Check if this checkbox is checked
		isChecked, err := checkbox.IsChecked()
		if err != nil {
			continue
		}

		if isChecked {
			// Get the scope label
			label, err := checkbox.GetAttribute("aria-label")
			if err == nil && label != "" {
				selected = append(selected, label)
			} else {
				// Try to find associated label by id
				id, err := checkbox.GetAttribute("id")
				if err == nil && id != "" {
					labelElem := cp.page().Locator(fmt.Sprintf("label[for='%s']", id))
					if count, _ := labelElem.Count(); count > 0 {
						text, err := labelElem.TextContent()
						if err == nil && text != "" {
							selected = append(selected, strings.TrimSpace(text))
						}
					}
				}
			}
		}
	}

	return selected, nil
}

// IsConsentButtonEnabled checks if the new-grant consent action is enabled.
func (cp *ConsentPage) IsConsentButtonEnabled(ctx context.Context) (bool, error) {
	return cp.isConsentActionEnabled("Approve & Delegate")
}

// IsSaveButtonEnabled checks if the existing-grant Save action is enabled.
func (cp *ConsentPage) IsSaveButtonEnabled(ctx context.Context) (bool, error) {
	return cp.isConsentActionEnabled("Save")
}

// GetSuccessMessage retrieves the success message after form submission.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - string: Success message text
//   - error: If success message not found
//
// Searches for an element with role="status" which contains the success feedback.
// This is typically a toast or status region that appears after successful submission.
//
// Example:
//
//	msg, err := consentPage.GetSuccessMessage(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(msg).To(ContainSubstring("updated successfully"))
func (cp *ConsentPage) GetSuccessMessage(ctx context.Context) (string, error) {
	// Find status region
	status := cp.page().GetByRole("status")

	// Check if it exists
	count, err := status.Count()
	if err != nil || count == 0 {
		return "", fmt.Errorf("success message (status region) not found")
	}

	// Get the text content
	text, err := status.First().TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get success message text: %w", err)
	}

	return strings.TrimSpace(text), nil
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
// Searches for an element with role="alert" which contains the error feedback.
// Returns empty string and nil if no error is found (not an error condition).
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
	alert := cp.page().GetByRole("alert")

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
	count, err := cp.page().GetByRole("alert").Count()
	if err != nil {
		return false, fmt.Errorf("failed to check for error: %w", err)
	}

	return count > 0, nil
}

// WaitForNoValidationError asserts that no validation alert appears on the page within
// the given timeout window. It works by waiting for an alert role element to become
// visible; a timeout (meaning no alert appeared) is treated as success.
//
// This avoids the false-negative that the previous "wait for hidden" approach had: if
// the alert is not yet in the DOM when WaitFor is called, Playwright considers the
// locator already "hidden" and returns immediately — so a late-appearing alert would
// not be caught. By waiting for the alert to become *visible* and treating a timeout as
// the expected "no error" outcome, we cover the full React state-update window.
//
// Parameters:
//   - ctx: Context for cancellation
//   - timeoutMs: Maximum wait in milliseconds (default 2000 if ≤ 0)
//
// Returns:
//   - error: If an alert becomes visible within the timeout window
func (cp *ConsentPage) WaitForNoValidationError(ctx context.Context, timeoutMs int) error {
	if timeoutMs <= 0 {
		timeoutMs = 2000
	}
	alert := cp.page().GetByRole("alert")
	err := alert.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(timeoutMs)),
	})
	if err != nil {
		// Timeout means no alert appeared within the window — that is the expected outcome.
		return nil
	}
	// An alert became visible unexpectedly — report it as a validation error.
	text, _ := alert.TextContent()
	return fmt.Errorf("unexpected validation error appeared: %s", text)
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
	// Find expiration input by label
	input := cp.page().GetByLabel("Expiration")

	// Check if element exists
	count, err := input.Count()
	if err != nil {
		return "", fmt.Errorf("failed to count expiration input: %w", err)
	}
	if count == 0 {
		return "", fmt.Errorf("expiration input not found")
	}

	// Get the value attribute
	value, err := input.InputValue()
	if err != nil {
		return "", fmt.Errorf("failed to get expiration input value: %w", err)
	}

	return value, nil
}

// DelegateService clicks the Delegate or Login button for a service.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceDisplayName: Display name of the service (e.g., "GitHub")
//
// Returns:
//   - error: If service button not found or not clickable
//
// Finds the service by its display name heading (semantic role="heading" level 3)
// then locates and clicks the action button (Login or Delegate).
// Uses data-testid for reliable button identification without brittle XPath selectors.
//
// Example:
//
//	err := consentPage.DelegateService(ctx, "GitHub")
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) DelegateService(ctx context.Context, serviceDisplayName string) error {
	if serviceDisplayName == "" {
		return fmt.Errorf("serviceDisplayName cannot be empty")
	}

	// Step 1: Find the service heading (semantic)
	serviceHeading := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Name: serviceDisplayName, Level: playwright.Int(3)},
	)

	count, err := serviceHeading.Count()
	if err != nil {
		return fmt.Errorf("failed to find service %q: %w", serviceDisplayName, err)
	}
	if count == 0 {
		return fmt.Errorf("service %q not found on page", serviceDisplayName)
	}

	// Step 2: Find the article parent (semantic role wrapper added in React)
	// Use aria-label attribute for semantic, accessible selector
	serviceArticle := cp.page().Locator(
		fmt.Sprintf(`article[aria-label="Service: %s"]`, serviceDisplayName),
	).First()

	articleCount, err := serviceArticle.Count()
	if err != nil || articleCount == 0 {
		return fmt.Errorf("service article wrapper not found for %q", serviceDisplayName)
	}

	// Step 3: Find action button within the service article (not page-wide)
	// Try Login button first (for services not yet connected)
	actionBtn := serviceArticle.Locator("[data-testid='service-login-button']")
	if btnCount, _ := actionBtn.Count(); btnCount > 0 {
		if err := actionBtn.Click(); err != nil {
			return fmt.Errorf("failed to click login button for service %q: %w", serviceDisplayName, err)
		}
		return nil
	}

	// Try Delegate button (for connected services not yet delegated)
	actionBtn = serviceArticle.Locator("[data-testid='service-delegate-button']")
	if btnCount, _ := actionBtn.Count(); btnCount > 0 {
		if err := actionBtn.Click(); err != nil {
			return fmt.Errorf("failed to click delegate button for service %q: %w", serviceDisplayName, err)
		}
		return nil
	}

	return fmt.Errorf("action button (Login or Delegate) not found for service %q", serviceDisplayName)
}

// TogglePermissionSet changes an optional permission set's selection by name.
func (cp *ConsentPage) TogglePermissionSet(_ context.Context, permissionSetName string) error {
	if permissionSetName == "" {
		return fmt.Errorf("permission set name cannot be empty")
	}
	return cp.page().GetByRole("switch", playwright.PageGetByRoleOptions{
		Name: "Toggle " + permissionSetName,
	}).Click()
}

// RevokeService clicks the Revoke button for a service.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceDisplayName: Display name of the service (e.g., "GitHub")
//
// Returns:
//   - error: If service or Revoke button not found
//
// Example:
//
//	err := consentPage.RevokeService(ctx, "GitHub")
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) RevokeService(ctx context.Context, serviceDisplayName string) error {
	if serviceDisplayName == "" {
		return fmt.Errorf("serviceDisplayName cannot be empty")
	}

	// Find service heading (semantic)
	serviceHeading := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Name: serviceDisplayName, Level: playwright.Int(3)},
	)

	count, err := serviceHeading.Count()
	if err != nil {
		return fmt.Errorf("failed to find service %q: %w", serviceDisplayName, err)
	}
	if count == 0 {
		return fmt.Errorf("service %q not found on page", serviceDisplayName)
	}

	// Find the article parent (semantic role wrapper)
	// Use aria-label attribute for semantic, accessible selector
	serviceArticle := cp.page().Locator(
		fmt.Sprintf(`article[aria-label="Service: %s"]`, serviceDisplayName),
	).First()

	if articleCount, _ := serviceArticle.Count(); articleCount == 0 {
		return fmt.Errorf("service article wrapper not found for %q", serviceDisplayName)
	}

	// Find Revoke button within service article
	revokeBtn := serviceArticle.GetByRole(
		"button",
		playwright.LocatorGetByRoleOptions{Name: "Revoke"},
	).First()

	btnCount, err := revokeBtn.Count()
	if err != nil || btnCount == 0 {
		return fmt.Errorf("revoke button not found for service %q", serviceDisplayName)
	}

	if err := revokeBtn.Click(); err != nil {
		return fmt.Errorf("failed to click revoke button for service %q: %w", serviceDisplayName, err)
	}

	return nil
}

// GetServiceCount returns the number of services displayed on the page.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - int: Number of services
//   - error: If failed to count services
//
// Example:
//
//	count, err := consentPage.GetServiceCount(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(count).To(Equal(3))
func (cp *ConsentPage) GetServiceCount(ctx context.Context) (int, error) {
	// Count service headings (level 3)
	headings := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Level: playwright.Int(3)},
	)

	count, err := headings.Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count service headings: %w", err)
	}

	// Exclude the main title (Services) heading
	if count > 0 {
		count-- // Subtract 1 for the "Services" section heading
	}

	return count, nil
}

// GetServiceNames returns the display names of all services on the page.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - []string: Slice of service display names
//   - error: If failed to retrieve service names
//
// Example:
//
//	names, err := consentPage.GetServiceNames(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(names).To(Equal([]string{"GitHub", "Google Cloud"}))
func (cp *ConsentPage) GetServiceNames(ctx context.Context) ([]string, error) {
	// Count service headings (level 3)
	headings := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Level: playwright.Int(3)},
	)

	count, err := headings.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count service headings: %w", err)
	}

	names := []string{}

	// Iterate through headings starting from index 1 (skip the main "Services" heading)
	for i := 1; i < count; i++ {
		heading := headings.Nth(i)
		text, err := heading.TextContent()
		if err != nil {
			continue
		}
		names = append(names, strings.TrimSpace(text))
	}

	return names, nil
}

// GetValidationErrorCount returns the number of validation errors displayed.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - int: Number of validation errors
//   - error: If failed to count errors
//
// Example:
//
//	count, err := consentPage.GetValidationErrorCount(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(count).To(Equal(1))
func (cp *ConsentPage) GetValidationErrorCount(ctx context.Context) (int, error) {
	// Find error container using semantic role="alert"
	errorAlert := cp.page().GetByRole("alert").First()

	// Check if error container exists
	count, err := cp.page().GetByRole("alert").Count()
	if err != nil {
		return 0, fmt.Errorf("failed to check for error container: %w", err)
	}

	if count == 0 {
		return 0, nil
	}

	// Count list items in error container
	errorItems := errorAlert.GetByRole("listitem")
	itemCount, err := errorItems.Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count error items: %w", err)
	}

	return itemCount, nil
}

// GetValidationErrors returns all validation error messages displayed.
//
// Parameters:
//   - ctx: Context for cancellation
//
// Returns:
//   - []string: Slice of error message texts
//   - error: If failed to retrieve error messages
//
// Example:
//
//	errors, err := consentPage.GetValidationErrors(ctx)
//	Expect(err).NotTo(HaveOccurred())
//	Expect(errors).To(ContainElement(ContainSubstring("required")))
func (cp *ConsentPage) GetValidationErrors(ctx context.Context) ([]string, error) {
	// Find error container using semantic role="alert"
	errorAlert := cp.page().GetByRole("alert").First()

	// Check if error container exists
	count, err := cp.page().GetByRole("alert").Count()
	if err != nil {
		return nil, fmt.Errorf("failed to check for error container: %w", err)
	}

	if count == 0 {
		return []string{}, nil
	}

	// Get all error items
	errorItems := errorAlert.GetByRole("listitem")
	itemCount, err := errorItems.Count()
	if err != nil {
		return nil, fmt.Errorf("failed to count error items: %w", err)
	}

	errors := []string{}
	for i := 0; i < itemCount; i++ {
		item := errorItems.Nth(i)
		text, err := item.TextContent()
		if err != nil {
			continue
		}
		errors = append(errors, strings.TrimSpace(text))
	}

	return errors, nil
}

// IsMandatoryServiceConnected checks if a mandatory service shows as connected/delegated.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceDisplayName: Display name of the service
//
// Returns:
//   - bool: True if service is connected/delegated
//   - error: If service not found
//
// Example:
//
//	connected, err := consentPage.IsMandatoryServiceConnected(ctx, "GitHub")
//	Expect(err).NotTo(HaveOccurred())
//	Expect(connected).To(BeTrue())
func (cp *ConsentPage) IsMandatoryServiceConnected(ctx context.Context, serviceDisplayName string) (bool, error) {
	if serviceDisplayName == "" {
		return false, fmt.Errorf("serviceDisplayName cannot be empty")
	}

	// Find service heading (semantic)
	serviceHeading := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Name: serviceDisplayName, Level: playwright.Int(3)},
	)

	count, err := serviceHeading.Count()
	if err != nil {
		return false, fmt.Errorf("failed to find service %q: %w", serviceDisplayName, err)
	}
	if count == 0 {
		return false, fmt.Errorf("service %q not found", serviceDisplayName)
	}

	// Find the article parent (semantic role wrapper)
	// Use aria-label attribute for semantic, accessible selector
	serviceArticle := cp.page().Locator(
		fmt.Sprintf(`article[aria-label="Service: %s"]`, serviceDisplayName),
	).First()

	if articleCount, _ := serviceArticle.Count(); articleCount == 0 {
		return false, fmt.Errorf("service article wrapper not found for %q", serviceDisplayName)
	}

	// Check for Revoke button (indicates service is delegated)
	revokeBtn := serviceArticle.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke"})
	btnCount, _ := revokeBtn.Count()

	return btnCount > 0, nil
}

// ===== Revoke Grant Page Object Methods =====
// These methods support the RevokeGrantButton and RevokeGrantDialog UI components
// introduced by feature 022-revoke-agent-consent.

// GetRevokeButton returns the "Revoke All Access" button locator on the agent detail page.
// The button is only rendered when the user has an active grant for the agent (FR-009).
func (cp *ConsentPage) GetRevokeButton(ctx context.Context) playwright.Locator {
	return cp.page().GetByRole(
		"button",
		playwright.PageGetByRoleOptions{Name: revokeDialogTitle},
	)
}

// ClickRevokeButton clicks the "Revoke All Access" button on the detail page.
// Returns an error if the button is not found or not clickable.
func (cp *ConsentPage) ClickRevokeButton(ctx context.Context) error {
	btn := cp.GetRevokeButton(ctx)

	count, err := btn.Count()
	if err != nil {
		return fmt.Errorf("failed to count revoke button: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("revoke all access button not found")
	}

	if err := btn.Click(); err != nil {
		return fmt.Errorf("failed to click revoke all access button: %w", err)
	}
	return nil
}

// GetRevokeDialog returns the RevokeGrantDialog locator (role="dialog").
// The dialog is shown after clicking the Revoke All Access button.
// NOTE: In Headless UI v2 with portal rendering, prefer WaitForRevokeDialog
// (heading-based) over calling WaitFor() on this locator directly.
func (cp *ConsentPage) GetRevokeDialog(ctx context.Context) playwright.Locator {
	return cp.revokeDialogHeading().Locator("xpath=ancestor::*[@role='dialog'][1]")
}

// ConfirmRevoke clicks the primary confirmation button inside the RevokeGrantDialog.
// This is the destructive action that permanently deletes the grant.
func (cp *ConsentPage) ConfirmRevoke(ctx context.Context) error {
	dialog := cp.GetRevokeDialog(ctx)

	count, err := dialog.Count()
	if err != nil {
		return fmt.Errorf("failed to find revoke dialog: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("revoke dialog not found; call ClickRevokeButton first")
	}

	// Confirmation button inside dialog — labeled "Revoke All Access" or "Confirm"
	confirmBtn := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: revokeDialogTitle})
	btnCount, err := confirmBtn.Count()
	if err != nil || btnCount == 0 {
		// Try alternate label "Confirm"
		confirmBtn = dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Confirm"})
		btnCount, _ = confirmBtn.Count()
		if btnCount == 0 {
			return fmt.Errorf("confirm button not found inside revoke dialog")
		}
	}

	if err := confirmBtn.Click(); err != nil {
		return fmt.Errorf("failed to click confirm button in revoke dialog: %w", err)
	}
	return nil
}

// CancelRevoke clicks the cancel button inside the RevokeGrantDialog.
// The dialog is dismissed without making any changes.
func (cp *ConsentPage) CancelRevoke(ctx context.Context) error {
	dialog := cp.GetRevokeDialog(ctx)

	count, err := dialog.Count()
	if err != nil {
		return fmt.Errorf("failed to find revoke dialog: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("revoke dialog not found; call ClickRevokeButton first")
	}

	cancelBtn := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel"})
	btnCount, err := cancelBtn.Count()
	if err != nil || btnCount == 0 {
		return fmt.Errorf("cancel button not found inside revoke dialog")
	}

	if err := cancelBtn.Click(); err != nil {
		return fmt.Errorf("failed to click cancel button in revoke dialog: %w", err)
	}
	return nil
}

// IsRevokeButtonPresent checks whether the "Revoke All Access" button is visible on the page.
// Returns false (not an error) when the button is absent, which is the expected state
// when the user has no active grant for the agent (FR-009).
func (cp *ConsentPage) IsRevokeButtonPresent(ctx context.Context) (bool, error) {
	btn := cp.GetRevokeButton(ctx)

	count, err := btn.Count()
	if err != nil {
		return false, fmt.Errorf("failed to count revoke button: %w", err)
	}
	if count == 0 {
		return false, nil
	}

	visible, err := btn.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check revoke button visibility: %w", err)
	}
	return visible, nil
}

// revokeDialogHeading returns a locator for the dialog title heading.
// This heading ("Revoke All Access") is only visible when the RevokeGrantDialog is open.
// Headless UI v2 renders the Dialog in a portal and may hide it with display:none when
// closed, making GetByRole("dialog").WaitFor() unreliable for detecting when the dialog opens.
// Waiting for the heading provides a reliable, animation-safe check.
func (cp *ConsentPage) revokeDialogHeading() playwright.Locator {
	return cp.page().GetByRole("heading", playwright.PageGetByRoleOptions{
		Name: revokeDialogTitle,
	})
}

// WaitForRevokeDialog waits for the RevokeGrantDialog to appear on the page.
// Should be called after ClickRevokeButton or ClickOverviewRevokeButton.
// Uses the dialog title heading as a reliable indicator rather than GetByRole("dialog"),
// which can have timing issues with Headless UI v2's portal-based rendering.
func (cp *ConsentPage) WaitForRevokeDialog(ctx context.Context) error {
	if err := cp.revokeDialogHeading().WaitFor(); err != nil {
		return fmt.Errorf("revoke grant dialog did not appear: %w", err)
	}
	return nil
}

func (cp *ConsentPage) revokeDialogPanel() playwright.Locator {
	return cp.page().Locator(`[role="dialog"] .bg-white.rounded-2xl`).First()
}

// TakeRevokeDialogScreenshot captures only the revoke dialog panel.
// This avoids false-positive diffs from dynamic page content behind the modal.
func (cp *ConsentPage) TakeRevokeDialogScreenshot(ctx context.Context, name string) error {
	if err := cp.WaitForRevokeDialog(ctx); err != nil {
		return err
	}
	panel := cp.revokeDialogPanel()
	if err := panel.WaitFor(); err != nil {
		return fmt.Errorf("revoke dialog panel not ready for screenshot: %w", err)
	}
	return cp.TakeLocatorScreenshot(ctx, name, panel)
}

// IsRevokeDialogVisible reports whether the RevokeGrantDialog is currently visible.
// Returns false (not an error) when the dialog has been dismissed.
// Uses the dialog title heading as the visibility indicator.
func (cp *ConsentPage) IsRevokeDialogVisible(ctx context.Context) (bool, error) {
	count, err := cp.revokeDialogHeading().Count()
	if err != nil {
		return false, fmt.Errorf("failed to check revoke dialog visibility: %w", err)
	}
	return count > 0, nil
}

// RevokeDialogContainsText checks whether text is visible on the page that matches
// what is expected to appear inside the RevokeGrantDialog.
// Uses page-wide text search since the dialog renders in a Headless UI portal.
// Must be called after WaitForRevokeDialog to ensure the dialog is open.
func (cp *ConsentPage) RevokeDialogContainsText(ctx context.Context, text string) (bool, error) {
	count, err := cp.page().GetByText(text).Count()
	if err != nil {
		return false, fmt.Errorf("failed to locate text %q on page: %w", text, err)
	}
	return count > 0, nil
}

// getOverviewRevokeButton returns the locator for the first "Revoke" button on an agent card
// in the consent overview page. Shared by IsOverviewRevokeButtonPresent and ClickOverviewRevokeButton.
//
// Uses GetByLabel to match the DelegationCard button's aria-label="Revoke access for {displayName}".
// This is more precise than GetByRole("button", {Name: "Revoke"}) which would also match the
// Card wrapper div (which has role="button" and an accessible name that includes the inner button's
// aria-label text via the ARIA accessible name computation algorithm).
func (cp *ConsentPage) getOverviewRevokeButton() playwright.Locator {
	return cp.page().GetByLabel("Revoke access for", playwright.PageGetByLabelOptions{Exact: playwright.Bool(false)}).First()
}

// IsOverviewRevokeButtonPresent reports whether at least one "Revoke" action button
// is visible on an agent card in the consent overview page.
// Waits for the button to appear (delegation list loads asynchronously after navigation).
func (cp *ConsentPage) IsOverviewRevokeButtonPresent(ctx context.Context) (bool, error) {
	btn := cp.getOverviewRevokeButton()
	if err := btn.WaitFor(); err != nil {
		// Timeout means button never appeared — that is a valid "not present" result.
		return false, nil
	}
	return true, nil
}

// ClickOverviewRevokeButton clicks the first "Revoke" action button on an agent card
// in the consent overview page, opening the RevokeGrantDialog.
// Waits for the button to appear before clicking (delegation list loads asynchronously).
func (cp *ConsentPage) ClickOverviewRevokeButton(ctx context.Context) error {
	btn := cp.getOverviewRevokeButton()
	if err := btn.WaitFor(); err != nil {
		return fmt.Errorf("no Revoke button found on overview page agent cards: %w", err)
	}
	if err := btn.Click(); err != nil {
		return fmt.Errorf("failed to click overview Revoke button: %w", err)
	}
	return nil
}

// WaitForRevokeDialogDismissed waits for the RevokeGrantDialog to fully close.
// Should be called after CancelRevoke or ConfirmRevoke to ensure the dialog has
// fully animated out before asserting on page state.
func (cp *ConsentPage) WaitForRevokeDialogDismissed(ctx context.Context) error {
	if err := cp.revokeDialogHeading().WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		return fmt.Errorf("revoke grant dialog did not close: %w", err)
	}
	return nil
}

// ServiceScopeRequiredIndicatorCount returns the number of service-scope chips
// marked as required within the given permission-set card (identified by data-testid).
// Locked service chips carry aria-label="<ServiceName> (required)" — this method counts them.
// Returns an error if more than one element matches psTestID to prevent silent false positives.
func (cp *ConsentPage) ServiceScopeRequiredIndicatorCount(ctx context.Context, psTestID string) (int, error) {
	locator := cp.page().GetByTestId(psTestID)
	count, err := locator.Count()
	if err != nil {
		return 0, fmt.Errorf("failed to locate PS card %q: %w", psTestID, err)
	}
	if count == 0 {
		return 0, nil
	}
	if count > 1 {
		return 0, fmt.Errorf("ambiguous locator: %d elements found with testid %q; use a unique test ID per card", count, psTestID)
	}
	chips := locator.GetByLabel("(required)", playwright.LocatorGetByLabelOptions{Exact: playwright.Bool(false)})
	return chips.Count()
}

// WaitForServiceToAppear waits for a specific service to appear on the page.
//
// Parameters:
//   - ctx: Context for cancellation
//   - serviceDisplayName: Display name of the service to wait for
//   - timeout: Maximum time to wait (in milliseconds)
//
// Returns:
//   - error: If service doesn't appear within timeout
//
// Example:
//
//	err := consentPage.WaitForServiceToAppear(ctx, "GitHub", 5000)
//	Expect(err).NotTo(HaveOccurred())
func (cp *ConsentPage) WaitForServiceToAppear(ctx context.Context, serviceDisplayName string, timeout int) error {
	if serviceDisplayName == "" {
		return fmt.Errorf("serviceDisplayName cannot be empty")
	}

	if timeout <= 0 {
		timeout = 5000 // Default to 5 seconds
	}

	// Wait for service heading using Playwright's built-in wait mechanism
	heading := cp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{
			Name:  serviceDisplayName,
			Level: playwright.Int(3),
		},
	)
	err := heading.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(float64(timeout)),
	})
	if err != nil {
		return fmt.Errorf("service %q did not appear within %dms: %w", serviceDisplayName, timeout, err)
	}
	return nil
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

// IsCIMDSummaryVisible reports whether the CIMDConsentSummary component is visible
// on the page (the "The application … wants to access …" paragraph).
func (cp *ConsentPage) IsCIMDSummaryVisible(ctx context.Context) (bool, error) {
	loc := cp.page().GetByText("wants to access", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check CIMD summary visibility: %w", err)
	}
	return visible, nil
}

// HasCIMDDomainBadge reports whether the CIMDDomainBadge component ("Verified domain: …")
// is visible on the page.
func (cp *ConsentPage) HasCIMDDomainBadge(ctx context.Context) (bool, error) {
	loc := cp.page().GetByText("Verified domain:", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check CIMD domain badge visibility: %w", err)
	}
	return visible, nil
}

// HasCIMDLocalhostWarning reports whether the CIMDLocalhostWarning alert is visible.
// The warning uses role="alert" and appears only when the redirect_uri points to localhost.
func (cp *ConsentPage) HasCIMDLocalhostWarning(ctx context.Context) (bool, error) {
	loc := cp.page().GetByRole("alert")
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check localhost warning visibility: %w", err)
	}
	return visible, nil
}

// ClickCIMDAdvancedDetails clicks the "Advanced Details" disclosure button in the
// CIMDAdvancedDetails component, expanding the detail panel.
func (cp *ConsentPage) ClickCIMDAdvancedDetails(ctx context.Context) error {
	btn := cp.page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Advanced Details"})
	count, err := btn.Count()
	if err != nil {
		return fmt.Errorf("failed to locate Advanced Details button: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("advanced details button not found")
	}
	if err := btn.Click(); err != nil {
		return fmt.Errorf("failed to click Advanced Details button: %w", err)
	}
	return nil
}

// IsCIMDAdvancedDetailsExpanded reports whether the CIMDAdvancedDetails panel is
// expanded, indicated by the presence of the "Client ID" detail label.
func (cp *ConsentPage) IsCIMDAdvancedDetailsExpanded(ctx context.Context) (bool, error) {
	loc := cp.page().GetByText("Client ID", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check advanced details panel state: %w", err)
	}
	return visible, nil
}

// HasCIMDClientName reports whether the given name is visible on the CIMD consent page.
// Uses Count to avoid strict-mode violations when the name appears in multiple elements
// (e.g., the agent heading and the consent summary paragraph).
func (cp *ConsentPage) HasCIMDClientName(ctx context.Context, name string) (bool, error) {
	count, err := cp.page().GetByText(name, playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	}).Count()
	if err != nil {
		return false, fmt.Errorf("failed to check CIMD client name %q: %w", name, err)
	}
	return count > 0, nil
}

// HasCIMDDomainText reports whether the given domain string appears within the verified domain badge.
func (cp *ConsentPage) HasCIMDDomainText(ctx context.Context, domain string) (bool, error) {
	loc := cp.page().GetByText(domain, playwright.PageGetByTextOptions{
		Exact: playwright.Bool(false),
	})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check CIMD domain text %q: %w", domain, err)
	}
	return visible, nil
}

// GetCIMDLocalhostWarningText returns the text content of the localhost warning alert.
func (cp *ConsentPage) GetCIMDLocalhostWarningText(ctx context.Context) (string, error) {
	loc := cp.page().GetByRole("alert")
	text, err := loc.TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to get localhost warning text: %w", err)
	}
	return text, nil
}

// GetCIMDClientNameFromDetails returns whether the client name is visible in the expanded
// CIMDAdvancedDetails panel. Scopes to <dd> (role="definition") to avoid strict-mode
// violations when the same name also appears in the consent summary header.
func (cp *ConsentPage) GetCIMDClientNameFromDetails(ctx context.Context, name string) (bool, error) {
	loc := cp.page().GetByRole("definition").Filter(playwright.LocatorFilterOptions{HasText: name})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check client name %q in advanced details: %w", name, err)
	}
	return visible, nil
}

// HasCIMDRedirectURIInDetails reports whether the given redirect URI value is visible
// in the expanded CIMDAdvancedDetails panel under the "Redirect URI" label.
func (cp *ConsentPage) HasCIMDRedirectURIInDetails(ctx context.Context, uri string) (bool, error) {
	loc := cp.page().GetByRole("definition").Filter(playwright.LocatorFilterOptions{HasText: uri})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check redirect URI %q in advanced details: %w", uri, err)
	}
	return visible, nil
}

// HasCIMDScopeInDetails reports whether the given scope badge is visible in the
// expanded CIMDAdvancedDetails panel under "Requested Scopes". Scopes to <dd>
// (role="definition") since scope badges are children of the scopes <dd> element.
func (cp *ConsentPage) HasCIMDScopeInDetails(ctx context.Context, scope string) (bool, error) {
	loc := cp.page().GetByRole("definition").Filter(playwright.LocatorFilterOptions{HasText: scope})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check scope %q in advanced details: %w", scope, err)
	}
	return visible, nil
}

// HasCIMDClientIDInDetails reports whether the given client ID URL is visible in the
// expanded CIMDAdvancedDetails panel under the "Client ID" label.
func (cp *ConsentPage) HasCIMDClientIDInDetails(ctx context.Context, clientID string) (bool, error) {
	loc := cp.page().GetByRole("definition").Filter(playwright.LocatorFilterOptions{HasText: clientID})
	visible, err := loc.IsVisible()
	if err != nil {
		return false, fmt.Errorf("failed to check client ID %q in advanced details: %w", clientID, err)
	}
	return visible, nil
}

// WaitForGrantSuccess waits for the "Grant updated successfully!" toast to appear.
// Returns an error if the toast does not appear within the timeout or if an error toast appears instead.
func (cp *ConsentPage) WaitForGrantSuccess(ctx context.Context, timeoutMs int) error {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	successToast := cp.page().GetByRole("status").Filter(playwright.LocatorFilterOptions{
		HasText: "Grant updated successfully",
	})

	err := successToast.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(timeoutMs)),
	})
	if err != nil {
		// Check if an error toast appeared instead
		errorToast := cp.page().GetByRole("alert")
		if count, _ := errorToast.Count(); count > 0 {
			text, _ := errorToast.First().TextContent()
			return fmt.Errorf("expected success toast but got: %s", text)
		}
		return fmt.Errorf("grant success toast did not appear within %dms", timeoutMs)
	}

	return nil
}

// WaitForGrantError waits for an error toast (role="alert") to appear after grant submission.
// Returns the error message text, or an error if no error toast appears within the timeout.
func (cp *ConsentPage) WaitForGrantError(ctx context.Context, timeoutMs int) (string, error) {
	if timeoutMs <= 0 {
		timeoutMs = 5000
	}

	alert := cp.page().GetByRole("alert")
	err := alert.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(timeoutMs)),
	})
	if err != nil {
		return "", fmt.Errorf("no error toast appeared within %dms", timeoutMs)
	}

	text, err := alert.First().TextContent()
	if err != nil {
		return "", fmt.Errorf("failed to read error toast text: %w", err)
	}

	return strings.TrimSpace(text), nil
}

// NavigateToAgentWithSelections navigates to the consent page with pre-selected
// permission set selections encoded in the consent_state query parameter.
// This simulates the state preserved across third-party OAuth2 login redirects.
func (cp *ConsentPage) NavigateToAgentWithSelections(ctx context.Context, agentID string, selections map[string][]string) error {
	if agentID == "" {
		return fmt.Errorf("agentID cannot be empty")
	}

	path := fmt.Sprintf(agentDetailPath, agentID)
	if len(selections) > 0 {
		encoded, err := encodeSelections(selections)
		if err != nil {
			return fmt.Errorf("failed to encode selections: %w", err)
		}
		path += "?consent_state=" + url.QueryEscape(encoded)
	}

	if err := cp.Navigate(ctx, path); err != nil {
		return fmt.Errorf("failed to navigate to agent consent page with selections: %w", err)
	}

	if err := cp.waitForAgentNameHeading(ctx); err != nil {
		return fmt.Errorf("agent name heading not found after navigation with selections: %w", err)
	}

	return nil
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

// encodeSelections encodes permission set selections to base64url JSON.
func encodeSelections(selections map[string][]string) (string, error) {
	jsonBytes, err := json.Marshal(selections)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(jsonBytes)
	return encoded, nil
}
