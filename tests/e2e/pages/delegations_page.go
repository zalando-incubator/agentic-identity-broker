package pages

import (
	"context"
	"fmt"
	"regexp"

	"github.com/mxschmitt/playwright-go"
)

// DelegationsRow records only agent details actually rendered in the collection.
type DelegationsRow struct {
	Agent   string
	Expiry  string
	Recency string
}

type DelegationsPage struct {
	*Page
}

func NewDelegationsPage(page playwright.Page, baseURL string) *DelegationsPage {
	return &DelegationsPage{Page: NewPage(page, baseURL)}
}

func (d *DelegationsPage) Navigate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.Page.Navigate(ctx, "/agents"); err != nil {
		return err
	}
	timeout, err := d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := d.page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Agents", Exact: playwright.Bool(true)}).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for Agents page: %w", err)
	}
	return ctx.Err()
}

func (d *DelegationsPage) Search(ctx context.Context, text string) error {
	search := d.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)})
	visible, err := d.locatorVisible(ctx, search, "agent search field")
	if err != nil {
		return err
	}
	if !visible {
		if err := d.locatorClick(ctx, d.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: regexp.MustCompile(`^Search agents(?:$|: )`)}), "open agent search"); err != nil {
			return err
		}
	}
	timeout, err := d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := search.Fill(text, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("search agents: %w", err)
	}
	timeout, err = d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	_, err = d.page.WaitForFunction(`term => {
		const query = new URL(location.href).searchParams.get('q') ?? '';
		return query === term;
	}`, text, playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("wait for agent search %q: %w", text, err)
	}
	return ctx.Err()
}

func (d *DelegationsPage) FocusSearchShortcut(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := d.page.Keyboard().Press("/"); err != nil {
		return fmt.Errorf("focus agent search with /: %w", err)
	}
	var focused bool
	err := d.evaluateJSON(ctx, d.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)}),
		`input => input === document.activeElement`, &focused)
	if err != nil {
		return err
	}
	if !focused {
		return fmt.Errorf("/ did not focus agent search")
	}
	return nil
}

func (d *DelegationsPage) ClearSearchWithEscape(ctx context.Context) error {
	visible, err := d.locatorVisible(ctx, d.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)}), "agent search field")
	if err != nil {
		return err
	}
	if !visible {
		if err := d.FocusSearchShortcut(ctx); err != nil {
			return err
		}
	}
	if err := d.locatorClick(ctx, d.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)}), "focus agent search"); err != nil {
		return err
	}
	if err := d.page.Keyboard().Press("Escape"); err != nil {
		return fmt.Errorf("clear agent search with Escape: %w", err)
	}
	timeout, err := d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := d.page.GetByRole("textbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)}).
		WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateHidden, Timeout: timeout}); err != nil {
		return fmt.Errorf("agent search did not collapse: %w", err)
	}
	return ctx.Err()
}

func (d *DelegationsPage) Sort(ctx context.Context, option string) error {
	if err := d.locatorClick(ctx, d.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Sort", Exact: playwright.Bool(true)}), "open agent sorting"); err != nil {
		return err
	}
	if err := d.locatorClick(ctx, d.page.GetByRole("menuitemradio", playwright.PageGetByRoleOptions{Name: option, Exact: playwright.Bool(true)}), "sort agents by "+option); err != nil {
		return err
	}
	timeout, err := d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := d.page.Locator("[role='menu']").WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateDetached, Timeout: timeout}); err != nil {
		return fmt.Errorf("agent sorting did not close: %w", err)
	}
	return ctx.Err()
}

func (d *DelegationsPage) collection() playwright.Locator {
	return d.page.GetByTestId("agents-collection")
}

func (d *DelegationsPage) collectionOrEmpty() playwright.Locator {
	return d.collection().Or(d.page.GetByTestId("delegations-empty-state")).Or(
		d.page.GetByText("No agents match your search.", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)})).First()
}

func (d *DelegationsPage) entity(agent string) playwright.Locator {
	return d.collection().GetByTestId("agent-entity").Filter(playwright.LocatorFilterOptions{
		Has: d.page.GetByRole("link", playwright.PageGetByRoleOptions{Name: agent, Exact: playwright.Bool(true)}),
	})
}

func (d *DelegationsPage) Rows(ctx context.Context) ([]DelegationsRow, error) {
	var rows []DelegationsRow
	err := d.evaluateJSON(ctx, d.collectionOrEmpty(), `root => root.matches('[data-testid="agents-collection"]') ?
		[...root.querySelectorAll('[data-testid="agent-entity"]')]
		.filter(entity => !entity.closest('[data-exiting], [inert]')).map(entity => {
		const name = entity.querySelector('[data-testid="entity-name"]');
		const expiry = entity.querySelector('[data-testid="agent-expiry"]');
		const changed = entity.querySelector('[data-testid="agent-changed-at"]');
		if (!name?.innerText.trim() || !expiry?.innerText.trim() || !changed?.parentElement?.innerText.trim())
			throw new Error('Agent entity is missing a name, expiry or recency');
		return {Agent: name.innerText.trim(), Expiry: expiry.innerText.trim(), Recency: changed.parentElement.innerText.trim()};
	}) : []`, &rows)
	return rows, err
}

// View observes the rendered entity anatomy, rather than a saved preference alone.
func (d *DelegationsPage) View(ctx context.Context) (string, error) {
	var view string
	err := d.evaluateJSON(ctx, d.page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Grid view / List view", Exact: playwright.Bool(true)}), `group => {
		const pressed = [...group.querySelectorAll('button[aria-pressed="true"]')];
		if (pressed.length !== 1) throw new Error('Exactly one collection view must be selected');
		const name = pressed[0].getAttribute('aria-label');
		if (name !== 'Grid view' && name !== 'List view') throw new Error('Unknown view: ' + name);
		return name === 'Grid view' ? 'grid' : 'list';
	}`, &view)
	return view, err
}

func (d *DelegationsPage) ChooseView(ctx context.Context, view string) error {
	label := "Grid view"
	if view == "list" {
		label = "List view"
	} else if view != "grid" {
		return fmt.Errorf("invalid agent collection view %q", view)
	}
	return d.locatorClick(ctx, d.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "show agents as "+view)
}

func (d *DelegationsPage) CardCount(ctx context.Context) (int, error) {
	var count int
	err := d.evaluateJSON(ctx, d.collectionOrEmpty(), `root => root.matches('[data-testid="agents-collection"]') ?
		[...root.querySelectorAll('[data-testid="agent-entity"]')]
			.filter(entity => !entity.closest('[data-exiting], [inert]')).length : 0`, &count)
	return count, err
}

// VisibleRowCount counts fully visible list rows, not partly visible cards.
func (d *DelegationsPage) VisibleRowCount(ctx context.Context) (int, error) {
	var count int
	err := d.evaluateJSON(ctx, d.collectionOrEmpty(), `root => root.matches('[data-testid="agents-collection"]') ?
		[...root.querySelectorAll('[data-slot="entity-row"]')].filter(row => {
			if (row.closest('[data-exiting], [inert]')) return false;
			const bounds = row.getBoundingClientRect();
			return bounds.width > 0 && bounds.height > 0 && bounds.top >= 0 && bounds.bottom <= innerHeight;
		}).length : 0`, &count)
	return count, err
}

func (d *DelegationsPage) ClickCard(ctx context.Context, agent string) error {
	return d.locatorClick(ctx, d.entity(agent).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: agent, Exact: playwright.Bool(true)}), "open agent "+agent)
}

func (d *DelegationsPage) CardMeta(ctx context.Context, agent string) (string, error) {
	return d.locatorText(ctx, d.entity(agent).GetByTestId("agent-changed-at").Locator("xpath=.."), "last changed for "+agent)
}

func (d *DelegationsPage) ClickRevoke(ctx context.Context, agent string) error {
	return d.locatorClick(ctx, d.entity(agent).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "revoke "+agent)
}

func (d *DelegationsPage) revokeDialog() playwright.Locator {
	return d.page.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Revoke access", Exact: playwright.Bool(true)})
}

func (d *DelegationsPage) ConfirmRevoke(ctx context.Context) error {
	return d.locatorClick(ctx, d.revokeDialog().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "confirm revocation")
}

func (d *DelegationsPage) CancelRevoke(ctx context.Context) error {
	return d.locatorClick(ctx, d.revokeDialog().GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Cancel", Exact: playwright.Bool(true)}), "cancel revocation")
}

func (d *DelegationsPage) RevokeDialogText(ctx context.Context) (string, error) {
	return d.locatorText(ctx, d.revokeDialog(), "revocation dialog")
}

func (d *DelegationsPage) EmptyStateText(ctx context.Context) (string, error) {
	return d.locatorText(ctx, d.page.GetByTestId("delegations-empty-state"), "delegations empty state")
}

func (d *DelegationsPage) RowIsPending(ctx context.Context, agent string) (bool, error) {
	var pending bool
	entity := d.collection().GetByTestId("agent-entity").Filter(playwright.LocatorFilterOptions{
		Has: d.page.GetByTestId("entity-name").Filter(playwright.LocatorFilterOptions{HasText: regexp.MustCompile("^" + regexp.QuoteMeta(agent) + "$")}),
	})
	err := d.evaluateJSON(ctx, entity, `entity => entity.getAttribute('aria-busy') === 'true'`, &pending)
	return pending, err
}

func (d *DelegationsPage) HasStatusFilter(ctx context.Context) (bool, error) {
	return d.locatorVisible(ctx, d.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Filter", Exact: playwright.Bool(true)}), "agent status filter")
}
