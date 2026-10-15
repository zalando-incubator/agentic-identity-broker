package pages

import (
	"context"
	"fmt"
	"regexp"

	"github.com/mxschmitt/playwright-go"
)

const thirdPartySessionsRoute = "/sessions"

var sessionLifetimeClaimPattern = regexp.MustCompile(`(?i)(session (might expire|does not expire|never expires)|refresh token expir|expiring soon)`)

type SessionsPage struct {
	*Page
}

func NewSessionsPage(page playwright.Page, baseURL string) *SessionsPage {
	return &SessionsPage{Page: NewPage(page, baseURL)}
}

func (sp *SessionsPage) NavigateToSessions(ctx context.Context) error {
	if err := sp.Navigate(ctx, thirdPartySessionsRoute); err != nil {
		return err
	}
	return sp.waitForPageLoad(ctx)
}

func (sp *SessionsPage) GetRefreshButtonCount(ctx context.Context) (int, error) {
	count, err := sp.refreshButtonLocator().Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count refresh buttons: %w", err)
	}
	return count, nil
}

func (sp *SessionsPage) GetLoginButtonCount(ctx context.Context) (int, error) {
	count, err := sp.loginButtonLocator().Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count login buttons: %w", err)
	}
	return count, nil
}

func (sp *SessionsPage) GetTerminateButtonCount(ctx context.Context) (int, error) {
	count, err := sp.page().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Terminate",
		Exact: playwright.Bool(true),
	}).Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count terminate buttons: %w", err)
	}
	return count, nil
}

func (sp *SessionsPage) GetActiveSessionCount(ctx context.Context) (int, error) {
	count, err := sp.page().GetByText("Active", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	}).Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count active sessions: %w", err)
	}
	return count, nil
}

func (sp *SessionsPage) GetSessionLifetimeClaimCount(ctx context.Context) (int, error) {
	count, err := sp.page().GetByText(sessionLifetimeClaimPattern).Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count session lifetime claims: %w", err)
	}
	return count, nil
}

func (sp *SessionsPage) IsRefreshButtonVisible(ctx context.Context) (bool, error) {
	button := sp.refreshButtonLocator().First()
	if err := button.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return false, fmt.Errorf("refreshable session button did not become visible: %w", err)
	}
	return true, nil
}

func (sp *SessionsPage) ClickRefreshButton(ctx context.Context) error {
	button := sp.refreshButtonLocator().First()
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to click refresh button: %w", err)
	}
	return nil
}

func (sp *SessionsPage) ClickLoginButton(ctx context.Context) error {
	if err := sp.loginButtonLocator().First().Click(); err != nil {
		return fmt.Errorf("failed to click login button: %w", err)
	}
	return nil
}

func (sp *SessionsPage) ClickRejectedTerminateButton(ctx context.Context) error {
	button := sp.loginButtonLocator().First().Locator("xpath=..").GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Terminate", Exact: playwright.Bool(true)})
	if err := button.Click(); err != nil {
		return fmt.Errorf("failed to terminate rejected session: %w", err)
	}
	return sp.page().GetByRole("dialog").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible})
}

func (sp *SessionsPage) CancelSessionTermination(ctx context.Context) error {
	dialog := sp.page().GetByRole("dialog")
	if err := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel session termination", Exact: playwright.Bool(true)}).Click(); err != nil {
		return fmt.Errorf("failed to cancel session termination: %w", err)
	}
	return dialog.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden})
}

func (sp *SessionsPage) ConfirmSessionTermination(ctx context.Context, serviceName string) error {
	dialog := sp.page().GetByRole("dialog")
	if err := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Terminate session with " + serviceName, Exact: playwright.Bool(true)}).Click(); err != nil {
		return fmt.Errorf("failed to confirm session termination: %w", err)
	}
	return dialog.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden})
}

func (sp *SessionsPage) WaitForReauthenticationRequired(ctx context.Context) error {
	status := sp.page().GetByText("Reauthentication required", playwright.PageGetByTextOptions{
		Exact: playwright.Bool(true),
	}).First()
	if err := status.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("reauthentication-required status did not appear: %w", err)
	}
	return nil
}

func (sp *SessionsPage) WaitForErrorMessage(ctx context.Context, message string) error {
	alert := sp.page().GetByRole("alert").Filter(playwright.LocatorFilterOptions{HasText: message})
	if err := alert.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("error alert containing %q did not appear: %w", message, err)
	}
	return nil
}

func (sp *SessionsPage) WaitForSuccessMessage(ctx context.Context, message string) error {
	status := sp.page().GetByRole("status").Filter(playwright.LocatorFilterOptions{HasText: message})
	if err := status.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("success status containing %q did not appear: %w", message, err)
	}
	return nil
}

func (sp *SessionsPage) page() playwright.Page {
	return sp.GetPlaywrightPage()
}

func (sp *SessionsPage) waitForPageLoad(ctx context.Context) error {
	heading := sp.page().GetByRole(
		"heading",
		playwright.PageGetByRoleOptions{Name: "Third-Party Sessions", Level: playwright.Int(2)},
	).First()
	if err := heading.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("third-party sessions heading did not appear: %w", err)
	}
	return nil
}

func (sp *SessionsPage) refreshButtonLocator() playwright.Locator {
	return sp.page().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Refresh",
		Exact: playwright.Bool(true),
	})
}

func (sp *SessionsPage) loginButtonLocator() playwright.Locator {
	return sp.page().GetByRole("button", playwright.PageGetByRoleOptions{
		Name:  "Login",
		Exact: playwright.Bool(true),
	})
}
