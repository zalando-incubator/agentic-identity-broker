package pages

import (
	"context"
	"fmt"

	"github.com/mxschmitt/playwright-go"
)

type SettingsPage struct {
	*Page
}

func NewSettingsPage(page playwright.Page, baseURL string) *SettingsPage {
	return &SettingsPage{Page: NewPage(page, baseURL)}
}

func (s *SettingsPage) Navigate(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.Page.Navigate(ctx, "/settings/appearance"); err != nil {
		return err
	}
	timeout, err := s.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := s.page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Settings", Exact: playwright.Bool(true)}).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for Settings page: %w", err)
	}
	return ctx.Err()
}

func (s *SettingsPage) ChooseTheme(ctx context.Context, label string) error {
	return s.locatorClick(ctx, s.page.GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Appearance", Exact: playwright.Bool(true)}).GetByRole("radio", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), label+" appearance")
}

func (s *SettingsPage) SelectedTheme(ctx context.Context) (string, error) {
	var label string
	err := s.evaluateJSON(ctx, s.page.GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Appearance", Exact: playwright.Bool(true)}).GetByRole("radio", playwright.LocatorGetByRoleOptions{Checked: playwright.Bool(true)}), `radio => {
		const labelled = (radio.getAttribute('aria-labelledby') || '').split(/\s+/)
			.map(id => document.getElementById(id)?.textContent || '').join(' ').trim();
		return (radio.getAttribute('aria-label') || labelled ||
			[...(radio.labels || [])].map(label => label.textContent).join(' ') ||
			radio.textContent).trim();
	}`, &label)
	return label, err
}

// ChooseDefaultView changes the preference immediately; the pressed button is the saved value.
func (s *SettingsPage) ChooseDefaultView(ctx context.Context, view string) error {
	if view != "grid" && view != "list" {
		return fmt.Errorf("invalid default collection view %q", view)
	}
	label := "Grid view"
	if view == "list" {
		label = "List view"
	}
	return s.locatorClick(ctx, s.page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Default collection view", Exact: playwright.Bool(true)}).
		GetByRole("button", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), "choose default "+view+" view")
}

func (s *SettingsPage) SelectedDefaultView(ctx context.Context) (string, error) {
	var view string
	err := s.evaluateJSON(ctx, s.page.GetByRole("group", playwright.PageGetByRoleOptions{Name: "Default collection view", Exact: playwright.Bool(true)}), `group => {
		const selected = [...group.querySelectorAll('button[aria-pressed="true"]')];
		if (selected.length !== 1) throw new Error('Expected one selected default view');
		const label = selected[0].getAttribute('aria-label') || selected[0].innerText.trim();
		if (label !== 'Grid view' && label !== 'List view') throw new Error('Unknown default view: ' + label);
		return label === 'Grid view' ? 'grid' : 'list';
	}`, &view)
	return view, err
}

func (s *SettingsPage) HasCategoryMenu(ctx context.Context) (bool, error) {
	return s.locatorVisible(ctx, s.page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "Settings categories", Exact: playwright.Bool(true)}), "settings categories")
}

func (s *SettingsPage) HasThemePreviews(ctx context.Context) (bool, error) {
	var count int
	err := s.evaluateJSON(ctx, s.page.GetByRole("radiogroup", playwright.PageGetByRoleOptions{Name: "Appearance", Exact: playwright.Bool(true)}), `group =>
		[...group.querySelectorAll('[role="radio"]')].filter(tile => tile.getClientRects().length > 0).length`, &count)
	return count == 3, err
}

func (s *SettingsPage) HasApprovalPersistenceControl(ctx context.Context) (bool, error) {
	var present bool
	err := s.evaluateJSON(ctx, s.page.GetByRole("main"), `main => [...main.querySelectorAll('input,select,button,[role="combobox"],[role="radio"],[role="switch"],[role="checkbox"]')].some(el => {
		const labelled = (el.getAttribute('aria-labelledby') || '').split(/\s+/)
			.map(id => document.getElementById(id)?.textContent || '').join(' ');
		const labels = [...(el.labels || [])].map(label => label.textContent).join(' ');
		const name = [el.getAttribute('aria-label'), labelled, labels, el.textContent,
			el.closest('fieldset')?.querySelector('legend')?.textContent,
			el.closest('[role="radiogroup"]')?.getAttribute('aria-label')].filter(Boolean).join(' ');
		return /approval|persistence|remember.*decision/i.test(name);
	})`, &present)
	return present, err
}
