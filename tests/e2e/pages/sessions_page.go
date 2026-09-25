package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

const thirdPartySessionsRoute = "/sessions"

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
