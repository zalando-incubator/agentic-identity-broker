package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

type ConnectionsPage struct {
	*Page
}

func NewConnectionsPage(page playwright.Page, baseURL string) *ConnectionsPage {
	return &ConnectionsPage{Page: NewPage(page, baseURL)}
}

func (sp *ConnectionsPage) NavigateToConnections(ctx context.Context) error {
	if err := sp.Navigate(ctx, "/connections"); err != nil {
		return err
	}
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := sp.page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Connections", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for Connections page: %w", err)
	}
	return ctx.Err()
}

func (sp *ConnectionsPage) collection() playwright.Locator {
	return sp.page.GetByTestId("connections-collection")
}

func (sp *ConnectionsPage) connection(provider string) playwright.Locator {
	return sp.collection().Locator(`[data-testid="connection-card"], [data-testid="connection-row"]`).
		Filter(playwright.LocatorFilterOptions{Has: sp.entityNameLocator(provider)})
}

func (sp *ConnectionsPage) refreshButtonLocator() playwright.Locator {
	return sp.collection().GetByTestId("connection-action").Filter(playwright.LocatorFilterOptions{HasText: "Refresh"})
}

func (sp *ConnectionsPage) GetRefreshButtonCount(ctx context.Context) (int, error) {
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return 0, err
	}
	if err := sp.collection().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return 0, fmt.Errorf("connections collection did not become visible: %w", err)
	}
	count, err := sp.refreshButtonLocator().Count()
	if err != nil {
		return 0, fmt.Errorf("count refreshable connections: %w", err)
	}
	return count, ctx.Err()
}

func (sp *ConnectionsPage) IsRefreshButtonVisible(ctx context.Context) (bool, error) {
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return false, err
	}
	if err := sp.refreshButtonLocator().First().WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return false, fmt.Errorf("refresh connection action did not become visible: %w", err)
	}
	return true, ctx.Err()
}

func (sp *ConnectionsPage) ClickRefreshButton(ctx context.Context) error {
	return sp.locatorClick(ctx, sp.refreshButtonLocator().First(), "refresh first connection")
}

func (sp *ConnectionsPage) WaitForSuccessMessage(ctx context.Context, message string) error {
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := sp.page.Locator("[data-sonner-toast]").Filter(playwright.LocatorFilterOptions{HasText: message, Visible: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("connection toast %q did not appear: %w", message, err)
	}
	return ctx.Err()
}

// ConnectionRow reports stored connection metadata that is visible in either view.
type ConnectionRow struct {
	Provider   string
	ScopeCount int
	State      string
	Action     string
	CreatedAt  string
}

func (sp *ConnectionsPage) ConnectionRows(ctx context.Context) ([]ConnectionRow, error) {
	var rows []ConnectionRow
	content := sp.collection().Or(sp.page.GetByTestId("connections-empty-state")).Or(
		sp.page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "No matching connections", Exact: playwright.Bool(true)})).First()
	err := sp.evaluateJSON(ctx, content, `root => root.matches('[data-testid="connections-collection"]') ?
		[...root.querySelectorAll('[data-testid="connection-card"], [data-testid="connection-row"]')]
			.filter(row => !row.closest('[data-exiting], [inert]')).map(row => {
			const text = id => {
				const element = row.querySelector('[data-testid="' + id + '"]');
				if (!element) throw new Error('Missing connection ' + id);
				return element.innerText.trim();
			};
			const name = row.querySelector('[data-testid="entity-name"]');
			const date = row.querySelector('time[data-testid="connection-created-at"]');
			if (!name || !date || !date.getAttribute('datetime')) throw new Error('Connection has no visible name or creation time');
			const scopes = text('connection-scope-count');
			const count = scopes.match(/^(\d+)\s+scopes?$/i);
			if (!count) throw new Error('Invalid visible scope count: ' + scopes);
			const actions = [...row.querySelectorAll('[data-testid="connection-action"],button[id^="disconnect-session-"]')]
				.filter(action => action.getClientRects().length > 0).map(action => action.innerText.trim()).join(' ');
			return {Provider: name.innerText.trim(), ScopeCount: Number(count[1]),
				State: text('connection-state'), Action: actions, CreatedAt: date.getAttribute('datetime')};
		}) : []`, &rows)
	return rows, err
}

func (sp *ConnectionsPage) ClickReconnect(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connection(provider).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Reconnect", Exact: playwright.Bool(true)}), "reconnect "+provider)
}

func (sp *ConnectionsPage) ClickRefresh(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connection(provider).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Refresh", Exact: playwright.Bool(true)}), "refresh "+provider)
}

func (sp *ConnectionsPage) ClickDisconnect(ctx context.Context, provider string) error {
	return sp.locatorClick(ctx, sp.connection(provider).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Disconnect", Exact: playwright.Bool(true)}), "disconnect "+provider)
}

func (sp *ConnectionsPage) DisconnectDialogText(ctx context.Context) (string, error) {
	dialog := sp.page.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Disconnect service", Exact: playwright.Bool(true)})
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	if err := dialog.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Disconnect", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return "", fmt.Errorf("wait for connection dependencies: %w", err)
	}
	return sp.locatorText(ctx, dialog, "disconnect connection dialog")
}

func (sp *ConnectionsPage) ConfirmDisconnect(ctx context.Context) error {
	return sp.locatorClick(ctx, sp.page.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Disconnect service", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Disconnect", Exact: playwright.Bool(true)}), "confirm connection disconnection")
}

func (sp *ConnectionsPage) ConnectionScopes(ctx context.Context, provider string) ([]string, error) {
	button := sp.connection(provider).GetByTestId("connection-scope-count")
	var count int
	if err := sp.evaluateJSON(ctx, button, `element => {
		const match = element.innerText.trim().match(/^(\d+)\s+scopes?$/i);
		if (!match) throw new Error('Invalid visible scope count');
		return Number(match[1]);
	}`, &count); err != nil {
		return nil, err
	}
	if count == 0 {
		return []string{}, ctx.Err()
	}
	if err := sp.locatorClick(ctx, button, "view granted scopes for "+provider); err != nil {
		return nil, err
	}
	var scopes []string
	popover := sp.page.Locator("[data-slot='popover-content']").Filter(playwright.LocatorFilterOptions{
		Has: sp.page.GetByText("Granted scopes", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}),
	})
	if err := sp.evaluateJSON(ctx, popover, `root => [...root.querySelectorAll('ul > li')].map(item => item.innerText.trim())`, &scopes); err != nil {
		return nil, err
	}
	if err := sp.page.Keyboard().Press("Escape"); err != nil {
		return nil, fmt.Errorf("close granted scopes: %w", err)
	}
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return nil, err
	}
	if err := popover.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return nil, fmt.Errorf("granted scopes popover did not close: %w", err)
	}
	return scopes, ctx.Err()
}

func (sp *ConnectionsPage) ToastText(ctx context.Context) (string, error) {
	return sp.locatorText(ctx, sp.page.Locator("[data-sonner-toast]").Filter(playwright.LocatorFilterOptions{Visible: playwright.Bool(true)}).Last(), "connection toast")
}

func (sp *ConnectionsPage) View(ctx context.Context) (string, error) {
	var view string
	err := sp.evaluateJSON(ctx, sp.page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Grid view / List view", Exact: playwright.Bool(true)}), `group => {
		const pressed = [...group.querySelectorAll('button[aria-pressed="true"]')];
		if (pressed.length !== 1) throw new Error('Exactly one collection view must be selected');
		const name = pressed[0].getAttribute('aria-label');
		if (name !== 'Grid view' && name !== 'List view') throw new Error('Unknown view: ' + name);
		return name === 'Grid view' ? 'grid' : 'list';
	}`, &view)
	return view, err
}

func (sp *ConnectionsPage) ChooseView(ctx context.Context, view string) error {
	label := "Grid view"
	if view == "list" {
		label = "List view"
	} else if view != "grid" {
		return fmt.Errorf("invalid connection view %q", view)
	}
	return sp.locatorClick(ctx, sp.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "show connections as "+view)
}

func (sp *ConnectionsPage) FilterState(ctx context.Context, state string) error {
	if err := sp.locatorClick(ctx, sp.page.Locator(`[data-slot="collection-toolbar"] button[aria-label^="Filter"]`), "open connection state filter"); err != nil {
		return err
	}
	filter := sp.page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Filter", Exact: playwright.Bool(true)})
	value := ""
	if state == "All" {
		selected := filter.Locator(`button[aria-pressed="true"]`)
		count, err := selected.Count()
		if err != nil {
			return fmt.Errorf("inspect selected connection state: %w", err)
		}
		if count != 1 {
			return fmt.Errorf("cannot clear connection state: expected one selected filter, got %d", count)
		}
		if err := sp.locatorClick(ctx, selected, "clear connection state filter"); err != nil {
			return err
		}
	} else {
		switch state {
		case "Connected":
			value = "connected"
		case "Needs sign-in":
			value = "needs-reauthentication"
		case "Expired":
			value = "expired"
		case "Unavailable":
			value = "error"
		default:
			return fmt.Errorf("unknown connection state %q", state)
		}
		if err := sp.locatorClick(ctx, filter.GetByRole("button", playwright.LocatorGetByRoleOptions{Name: state, Exact: playwright.Bool(true)}), "filter connections by "+state); err != nil {
			return err
		}
	}
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	_, err = sp.page.WaitForFunction(`([value, label]) => {
		const query = new URL(location.href).searchParams.get('state') ?? '';
		const chips = [...document.querySelectorAll('[aria-label="Active connection filters"] button')];
		const chip = chips.some(button => button.textContent.trim() === 'Filter: ' + label);
		return query === value && (value ? chip : !chips.some(button => button.textContent.trim().startsWith('Filter: ')));
	}`, []string{value, state}, playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("wait for connection state filter %q: %w", state, err)
	}
	if err := sp.page.Keyboard().Press("Escape"); err != nil {
		return fmt.Errorf("close connection state filter: %w", err)
	}
	if err := filter.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return fmt.Errorf("connection state filter did not close: %w", err)
	}
	return ctx.Err()
}

func (sp *ConnectionsPage) ActiveFilterChips(ctx context.Context) ([]string, error) {
	var chips []string
	err := sp.evaluateJSON(ctx, sp.page.GetByRole("main"), `main => [...main.querySelectorAll('[aria-label="Active connection filters"] button')]
		.filter(button => button.getClientRects().length > 0).map(button => button.innerText.trim())`, &chips)
	return chips, err
}

func (sp *ConnectionsPage) Search(ctx context.Context, text string) error {
	input := sp.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search", Exact: playwright.Bool(true)})
	visible, err := sp.locatorVisible(ctx, input, "connection search")
	if err != nil {
		return err
	}
	if !visible {
		if err := sp.locatorClick(ctx, sp.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Search", Exact: playwright.Bool(true)}), "open connection search"); err != nil {
			return err
		}
	}
	timeout, err := sp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := input.Fill(text, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("search connections: %w", err)
	}
	timeout, err = sp.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	_, err = sp.page.WaitForFunction(`term => {
		const query = new URL(location.href).searchParams.get('q') ?? '';
		const chip = [...document.querySelectorAll('[aria-label="Active connection filters"] button')]
			.some(button => button.textContent.trim() === 'Search: ' + term);
		return query === term && chip === Boolean(term);
	}`, text, playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("wait for connection search %q: %w", text, err)
	}
	return ctx.Err()
}

func (sp *ConnectionsPage) Sort(ctx context.Context, option string) error {
	if err := sp.locatorClick(ctx, sp.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sort", Exact: playwright.Bool(true)}), "open connection sorting"); err != nil {
		return err
	}
	return sp.locatorClick(ctx, sp.page.GetByRole("menuitemradio", playwright.PageGetByRoleOptions{Name: option, Exact: playwright.Bool(true)}), "sort connections by "+option)
}
