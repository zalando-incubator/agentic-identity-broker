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
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return 0, err
	}
	if err := sp.page().GetByTestId("connections-table").WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateVisible, Timeout: timeout,
	}); err != nil {
		return 0, fmt.Errorf("connections table did not become visible: %w", err)
	}
	count, err := sp.refreshButtonLocator().Count()
	if err != nil {
		return 0, fmt.Errorf("failed to count refresh buttons: %w", err)
	}
	return count, ctx.Err()
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
		playwright.PageGetByRoleOptions{Name: "Connections", Exact: playwright.Bool(true)},
	).First()
	if err := heading.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(float64(sp.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("connections heading did not appear: %w", err)
	}
	return nil
}

func (sp *SessionsPage) refreshButtonLocator() playwright.Locator {
	return sp.page().GetByTestId("connections-table").GetByRole("button", playwright.LocatorGetByRoleOptions{
		Name:  "Refresh",
		Exact: playwright.Bool(true),
	})
}

// ConnectionRow describes a stored connection, not an inferred missing service.
type ConnectionRow struct {
	Provider   string
	ScopeCount int
	State      string
	Action     string
	CreatedAt  string
}

func (sp *SessionsPage) ConnectionRows(ctx context.Context) ([]ConnectionRow, error) {
	var rows []ConnectionRow
	err := sp.evaluateJSON(ctx, sp.page().GetByTestId("connections-table"), `root =>
		Array.from(root.querySelectorAll('[data-testid="connection-row"]')).map(row => {
			const text = (id, child = '') => {
				const element = row.querySelector('[data-testid="' + id + '"]' + child);
				if (!element) throw new Error('Missing connection ' + id);
				return element.innerText.trim();
			};
			const scopes = text('connection-scope-count');
			const match = scopes.match(/^(\d+)(?:\s+scopes?)?$/i);
			if (!match) throw new Error('Invalid visible scope count: ' + scopes);
			const actions = Array.from(row.querySelectorAll('td:last-child button, td:last-child a[role="button"]'))
				.filter(action => action.getClientRects().length > 0)
				.map(action => action.innerText.trim()).join(' ');
			const created = row.querySelector('[data-testid="connection-created-at"]');
			if (!created || !created.innerText.trim() || created.getClientRects().length === 0) {
				throw new Error('Missing visible connection creation time');
			}
			const timestamp = created.matches('time') ? created : created.querySelector('time');
			const createdAt = timestamp?.getAttribute('datetime');
			if (!createdAt) throw new Error('Connection creation time has no datetime');
			return {Provider: text('connection-provider', ' [data-slot="truncated-text"] > span'), ScopeCount: Number(match[1]),
				State: text('connection-state'), Action: actions,
				CreatedAt: createdAt};
		})`, &rows)
	return rows, err
}

func (sp *SessionsPage) connectionRow(provider string) playwright.Locator {
	return sp.page().GetByTestId("connections-table").GetByTestId("connection-row").Filter(playwright.LocatorFilterOptions{
		Has: sp.page().GetByTestId("connection-provider").GetByText(provider, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}),
	})
}

func (sp *SessionsPage) ClickReconnect(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connectionRow(provider).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Reconnect", Exact: playwright.Bool(true)}), "reconnect "+provider)
}

func (sp *SessionsPage) ClickRefresh(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connectionRow(provider).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Refresh", Exact: playwright.Bool(true)}), "refresh "+provider)
}

func (sp *SessionsPage) ClickDisconnect(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connectionRow(provider).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Disconnect", Exact: playwright.Bool(true)}), "disconnect "+provider)
}

func (sp *SessionsPage) DisconnectDialogText(ctx context.Context) (string, error) {
	dialog := sp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Disconnect service", Exact: playwright.Bool(true)})
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	if err := dialog.Locator("button:enabled").Filter(playwright.LocatorFilterOptions{HasText: "Disconnect"}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return "", fmt.Errorf("wait for current connection dependencies: %w", err)
	}
	return sp.locatorText(ctx, dialog, "disconnect connection dialog")
}

func (sp *SessionsPage) ConfirmDisconnect(ctx context.Context) error {
	return sp.locatorClick(ctx, sp.page().GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Disconnect service", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Disconnect", Exact: playwright.Bool(true)}), "confirm connection disconnection")
}
