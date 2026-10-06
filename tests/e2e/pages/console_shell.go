package pages

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/mxschmitt/playwright-go"
)

// ConsoleShell exposes navigation shared by all console routes. Decision pages
// intentionally have none of these controls.
type ConsoleShell struct {
	*Page
}

func NewConsoleShell(page playwright.Page, baseURL string) *ConsoleShell {
	return &ConsoleShell{Page: NewPage(page, baseURL)}
}

func (s *ConsoleShell) navigation() playwright.Locator {
	return s.page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "Main navigation", Exact: playwright.Bool(true)})
}

func (s *ConsoleShell) navigateItem(ctx context.Context, label string) error {
	visible, err := s.locatorVisible(ctx, s.navigation(), "main navigation")
	if err != nil {
		return err
	}
	if !visible {
		if err := s.locatorClick(ctx, s.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Open navigation", Exact: playwright.Bool(true)}), "open navigation"); err != nil {
			return err
		}
	}
	return s.locatorClick(ctx, s.navigation().GetByRole("link", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), label+" navigation")
}

func (s *ConsoleShell) NavigationItems(ctx context.Context) ([]string, error) {
	var items []string
	err := s.evaluateJSON(ctx, s.navigation(), `nav => [...nav.querySelectorAll('a[href]')]
		.filter(link => link.getClientRects().length > 0)
		.map(link => (link.getAttribute('aria-label') || link.innerText).trim())`, &items)
	return items, err
}

func (s *ConsoleShell) NavigateAgents(ctx context.Context) error {
	return s.navigateItem(ctx, "Agents")
}

func (s *ConsoleShell) NavigateConnections(ctx context.Context) error {
	return s.navigateItem(ctx, "Connections")
}

func (s *ConsoleShell) NavigateApprovals(ctx context.Context) error {
	return s.navigateItem(ctx, "Approvals")
}

func (s *ConsoleShell) OpenSettingsFromUserMenu(ctx context.Context) error {
	if err := s.OpenUserMenu(ctx); err != nil {
		return err
	}
	return s.locatorClick(ctx, s.page.GetByRole("menuitem", playwright.PageGetByRoleOptions{Name: "Settings", Exact: playwright.Bool(true)}), "open Settings from user menu")
}

func (s *ConsoleShell) CollapseSidebar(ctx context.Context) error {
	return s.locatorClick(ctx, s.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Collapse sidebar", Exact: playwright.Bool(true)}), "collapse sidebar")
}

func (s *ConsoleShell) IsSidebarCollapsed(ctx context.Context) (bool, error) {
	var collapsed bool
	err := s.evaluateJSON(ctx, s.page.GetByTestId("console-sidebar"), `sidebar => sidebar.dataset.state === 'collapsed'`, &collapsed)
	return collapsed, err
}

func (s *ConsoleShell) HasSidebar(ctx context.Context) (bool, error) {
	return s.locatorVisible(ctx, s.page.GetByTestId("console-sidebar"), "console sidebar")
}

func (s *ConsoleShell) PendingApprovalCount(ctx context.Context) (int, error) {
	badge := s.page.GetByTestId("pending-approval-count")
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	count, err := badge.Count()
	if err != nil {
		return 0, fmt.Errorf("locate pending approval count: %w", err)
	}
	// Zero pending approvals need not display a badge.
	if count == 0 {
		return 0, nil
	}
	text, err := s.locatorText(ctx, badge, "pending approval count")
	if err != nil {
		return 0, err
	}
	value, err := strconv.Atoi(text)
	if err != nil {
		return 0, fmt.Errorf("parse pending approval count %q: %w", text, err)
	}
	return value, nil
}

func (s *ConsoleShell) OpenUserMenu(ctx context.Context) error {
	return s.locatorClick(ctx, s.page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "User menu", Exact: playwright.Bool(true)}), "user menu")
}

func (s *ConsoleShell) ChooseTheme(ctx context.Context, label string) error {
	return s.locatorClick(ctx, s.page.GetByRole("menuitemradio", playwright.PageGetByRoleOptions{Name: label, Exact: playwright.Bool(true)}), label+" theme")
}

func (s *ConsoleShell) OpenCommandPalette(ctx context.Context) error {
	timeout, err := s.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := s.page.Keyboard().Press("ControlOrMeta+k"); err != nil {
		return fmt.Errorf("open command palette shortcut: %w", err)
	}
	if err := s.page.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Command palette", Exact: playwright.Bool(true)}).WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for command palette: %w", err)
	}
	return ctx.Err()
}

func (s *ConsoleShell) commandPalette() playwright.Locator {
	return s.page.GetByRole("dialog", playwright.PageGetByRoleOptions{Name: "Command palette", Exact: playwright.Bool(true)})
}

func (s *ConsoleShell) SearchCommandPalette(ctx context.Context, text string) error {
	timeout, err := s.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := s.commandPalette().GetByRole("combobox").Fill(text, playwright.LocatorFillOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("search command palette: %w", err)
	}
	return ctx.Err()
}

func (s *ConsoleShell) CommandPaletteResults(ctx context.Context) ([]string, error) {
	var results []string
	err := s.evaluateJSON(ctx, s.commandPalette(), `dialog => [...dialog.querySelectorAll('[role="option"]')]
		.filter(el => el.getAttribute('aria-disabled') !== 'true' && el.getBoundingClientRect().height > 0)
		.map(el => (el.getAttribute('aria-label') || el.textContent).trim())`, &results)
	return results, err
}

func (s *ConsoleShell) ChooseCommandResult(ctx context.Context, label string) error {
	timeout, err := s.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	target := s.commandPalette().GetByRole("option", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)})
	if err := target.WaitFor(playwright.LocatorWaitForOptions{State: playwright.WaitForSelectorStateVisible, Timeout: timeout}); err != nil {
		return fmt.Errorf("wait for command result %q: %w", label, err)
	}
	results, err := s.CommandPaletteResults(ctx)
	if err != nil {
		return err
	}
	for range len(results) + 1 {
		selected, err := target.GetAttribute("aria-selected", playwright.LocatorGetAttributeOptions{Timeout: timeout})
		if err != nil {
			return err
		}
		if selected == "true" {
			return s.page.Keyboard().Press("Enter")
		}
		if err := s.page.Keyboard().Press("ArrowDown"); err != nil {
			return err
		}
	}
	return fmt.Errorf("command result %q cannot be selected by keyboard", label)
}

func (s *ConsoleShell) LiveRegionText(ctx context.Context) (string, error) {
	var text string
	err := s.evaluateJSON(ctx, s.page.Locator("html"), `root => [...root.querySelectorAll('[aria-live], [role="status"], [role="alert"]')]
		.filter(el => !el.closest('[aria-hidden="true"], [inert]'))
		.map(el => el.textContent.trim()).filter(Boolean).join('\n')`, &text)
	return strings.TrimSpace(text), err
}

func (s *ConsoleShell) LiveRegionPoliteness(ctx context.Context) (string, error) {
	var politeness string
	err := s.evaluateJSON(ctx, s.page.GetByTestId("approval-announcement"), `region => region.getAttribute('aria-live') || (region.getAttribute('role') === 'status' ? 'polite' : '')`, &politeness)
	return politeness, err
}

func (s *ConsoleShell) NavigationIconFrameChanges(ctx context.Context, label string) (int, error) {
	timeout, err := s.locatorTimeout(ctx)
	if err != nil {
		return 0, err
	}
	if err := s.page.GetByRole("main").Hover(playwright.LocatorHoverOptions{Timeout: timeout}); err != nil {
		return 0, fmt.Errorf("leave navigation icon: %w", err)
	}
	link := s.page.GetByRole("navigation", playwright.PageGetByRoleOptions{Name: "Main navigation"}).GetByRole("link", playwright.LocatorGetByRoleOptions{Name: label, Exact: playwright.Bool(true)})
	_, err = link.Evaluate(`async host => {
		await new Promise(requestAnimationFrame);
		await new Promise(requestAnimationFrame);
		window.__aibIconFrameChanges = -1;
		host.addEventListener('pointerenter', async () => {
			const frames = new Set();
			for (let frame = 0; frame < 16; frame++) {
				await new Promise(requestAnimationFrame);
				frames.add(host.querySelector('svg').outerHTML);
			}
			window.__aibIconFrameChanges = frames.size;
		}, {once: true});
	}`, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _, _ = s.page.Evaluate(`delete window.__aibIconFrameChanges`) }()
	if err := link.Hover(playwright.LocatorHoverOptions{Timeout: timeout}); err != nil {
		return 0, fmt.Errorf("hover navigation icon: %w", err)
	}
	if _, err := s.page.WaitForFunction(`() => window.__aibIconFrameChanges > 0`, nil, playwright.PageWaitForFunctionOptions{Timeout: timeout}); err != nil {
		return 0, fmt.Errorf("navigation icon frame observation: %w", err)
	}
	var changes int
	err = s.evaluateJSON(ctx, link, `() => window.__aibIconFrameChanges`, &changes)
	return changes, err
}
