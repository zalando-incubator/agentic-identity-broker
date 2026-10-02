package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

// DelegationsRow mirrors only the list's user-visible agent and expiry. The
// existing API does not provide a permission-set count for this projection.
type DelegationsRow struct {
	Agent  string `json:"agent"`
	Expiry string `json:"expiry"`
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
	if err := d.Page.Navigate(ctx, "/delegations"); err != nil {
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
	timeout, err := d.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := d.page.GetByRole("searchbox", playwright.PageGetByRoleOptions{Name: "Search agents", Exact: playwright.Bool(true)}).Fill(text, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("search agents: %w", err)
	}
	return ctx.Err()
}

func (d *DelegationsPage) table() playwright.Locator {
	return d.page.GetByRole("table", playwright.PageGetByRoleOptions{Name: "Agents", Exact: playwright.Bool(true)})
}

func (d *DelegationsPage) Rows(ctx context.Context) ([]DelegationsRow, error) {
	var rows []DelegationsRow
	err := d.evaluateJSON(ctx, d.page.GetByRole("main"), `main => [...main.querySelectorAll('table[aria-label="Agents"] tbody tr')]
		.filter(row => row.querySelector('[data-column="agent"]'))
		.map(row => ({agent: row.querySelector('[data-column="agent"] [data-slot="truncated-text"] > span').textContent.trim(),
			expiry: row.querySelector('[data-column="expiry"] > div').textContent.trim()}))`, &rows)
	return rows, err
}

func (d *DelegationsPage) ColumnHeaders(ctx context.Context) ([]string, error) {
	var headers []string
	err := d.evaluateJSON(ctx, d.table(), `table => [...table.querySelectorAll('thead th')].map(th => th.textContent.trim())`, &headers)
	return headers, err
}

// VisibleRowCount counts complete rows inside the viewport, rather than rows
// merely mounted below the fold. It measures the dense-list acceptance target.
func (d *DelegationsPage) VisibleRowCount(ctx context.Context) (int, error) {
	var count int
	err := d.evaluateJSON(ctx, d.page.GetByRole("main"), `main => [...main.querySelectorAll('table[aria-label="Agents"] tbody tr')].filter(row => {
		if (!row.querySelector('[data-column="agent"]')) return false;
		const r = row.getBoundingClientRect(), s = getComputedStyle(row);
		return r.width > 0 && r.height > 0 && r.top >= 0 && r.bottom <= innerHeight &&
			s.visibility === 'visible' && s.display !== 'none';
	}).length`, &count)
	return count, err
}

func (d *DelegationsPage) row(agent string) playwright.Locator {
	return d.table().GetByRole("row").Filter(playwright.LocatorFilterOptions{
		Has: d.page.Locator("[data-column='agent']").GetByText(agent, playwright.LocatorGetByTextOptions{Exact: playwright.Bool(true)}),
	})
}

func (d *DelegationsPage) ClickView(ctx context.Context, agent string) error {
	return d.locatorClick(ctx, d.row(agent).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: "View", Exact: playwright.Bool(true)}), "view "+agent)
}

func (d *DelegationsPage) ClickRevoke(ctx context.Context, agent string) error {
	return d.locatorClick(ctx, d.row(agent).GetByRole("button", playwright.LocatorGetByRoleOptions{Name: "Revoke", Exact: playwright.Bool(true)}), "revoke "+agent)
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
	err := d.evaluateJSON(ctx, d.row(agent), `row => row.getAttribute('aria-busy') === 'true'`, &pending)
	return pending, err
}

func (d *DelegationsPage) HasStatusFilter(ctx context.Context) (bool, error) {
	return d.locatorVisible(ctx, d.page.GetByRole("combobox", playwright.PageGetByRoleOptions{Name: "Status", Exact: playwright.Bool(true)}), "delegation status filter")
}
