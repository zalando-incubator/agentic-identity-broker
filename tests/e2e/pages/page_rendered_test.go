package pages

import (
	"context"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These are instrumentation regressions, not application acceptance scenarios:
// the deliberately inaccessible HTML ensures observations cannot report a pass
// merely because an element matches :focus-visible or has a decorative shadow.
func TestRenderedPageObservations(t *testing.T) {
	pw, err := playwright.Run()
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, pw.Stop()) })
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, browser.Close()) })
	ctx := context.Background()

	newPage := func(t *testing.T) (playwright.Page, *Page) {
		t.Helper()
		browserContext, err := browser.NewContext()
		require.NoError(t, err)
		t.Cleanup(func() { assert.NoError(t, browserContext.Close()) })
		page, err := browserContext.NewPage()
		require.NoError(t, err)
		return page, NewPage(page, "")
	}

	t.Run("cleared theme stays system after reload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><script>
				const stored = localStorage.getItem('aib.theme');
				const theme = stored === 'dark' || stored === 'light' ? stored :
					matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
				document.documentElement.dataset.theme = theme;
				document.documentElement.style.colorScheme = theme;
			</script><body>Theme fixture</body>`))
		}))
		t.Cleanup(server.Close)
		page, observation := newPage(t)
		require.NoError(t, observation.EmulateColorScheme(ctx, "light"))
		require.NoError(t, observation.SetThemePreference(ctx, "dark"))
		_, err := page.Goto(server.URL)
		require.NoError(t, err)
		first, err := observation.FirstPaintTheme(ctx)
		require.NoError(t, err)
		assert.Equal(t, ThemeFrame{Theme: "dark", ColorScheme: "dark"}, first)
		_, err = page.Evaluate(`() => localStorage.removeItem('aib.theme')`)
		require.NoError(t, err)
		_, err = page.Reload()
		require.NoError(t, err)
		reloaded, err := observation.FirstPaintTheme(ctx)
		require.NoError(t, err)
		assert.Equal(t, ThemeFrame{Theme: "light", ColorScheme: "light"}, reloaded)
		stored, err := page.Evaluate(`() => localStorage.getItem('aib.theme')`)
		require.NoError(t, err)
		assert.Nil(t, stored)
	})

	for _, test := range []struct {
		name    string
		markup  string
		visible bool
		ring    bool
	}{
		{"transparent ancestor hides focus", `<style>button:focus-visible{outline:3px solid blue}</style><div style="opacity:0"><button>Hidden action</button></div>`, false, false},
		{"static shadow is not focus treatment", `<style>button{outline:none;box-shadow:0 2px 4px black}</style><button>Shadow action</button>`, true, false},
		{"changed outline is visible focus", `<style>button{outline:none}button:focus-visible{outline:3px solid blue}</style><button>Outlined action</button>`, true, true},
		{"layered ring ignores transparent placeholders", `<style>button{outline:none}button:focus-visible{box-shadow:0 0 0 0 transparent,0 0 0 3px blue,0 0 0 0 transparent}</style><button>Ring action</button>`, true, true},
		{"transparent shadow is not focus treatment", `<style>button{outline:none}button:focus-visible{box-shadow:0 0 0 3px transparent}</style><button>Transparent action</button>`, true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, observation := newPage(t)
			require.NoError(t, page.SetContent(test.markup))
			require.NoError(t, page.Keyboard().Press("Tab"))
			focus, err := observation.ActiveElementDescription(ctx)
			require.NoError(t, err)
			assert.Equal(t, test.visible, focus.Visible)
			assert.Equal(t, test.ring, focus.FocusVisible)
			assert.Equal(t, "button", focus.Tag)
		})
	}

	for _, test := range []struct {
		name string
		rule string
		ring bool
	}{
		{"native date subcontrols keep a visible focus-within ring", "input:focus-within{outline:3px solid blue}", true},
		{"native date without an indicator remains a failure", "", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			page, observation := newPage(t)
			require.NoError(t, page.SetContent(`<style>input,button{outline:none}button:focus-visible{outline:3px solid blue}`+test.rule+`</style>
				<label for="due">Due date</label><input id="due" type="date" value="2099-11-06"><button>After date</button>`))
			coverage, err := observation.KeyboardActionCoverage(ctx)
			require.NoError(t, err)
			assert.Contains(t, coverage, KeyboardAction{Name: "Due date", Reached: true, VisibleFocus: test.ring})
			assert.Contains(t, coverage, KeyboardAction{Name: "After date", Reached: true, VisibleFocus: true})
			value, err := page.GetByLabel("Due date").InputValue()
			require.NoError(t, err)
			assert.Equal(t, "2099-11-06", value)
		})
	}

	t.Run("programmatic landmarks are not actions but untabbable buttons remain failures", func(t *testing.T) {
		page, observation := newPage(t)
		require.NoError(t, page.SetContent(`<style>button{outline:none}button:focus-visible{outline:3px solid blue}</style>
			<main tabindex="-1"><button>Reachable action</button><button tabindex="-1">Unreachable action</button></main>`))
		coverage, err := observation.KeyboardActionCoverage(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []KeyboardAction{
			{Name: "Reachable action", Reached: true, VisibleFocus: true},
			{Name: "Unreachable action", Reached: false, VisibleFocus: false},
		}, coverage)
	})

	t.Run("all roving items are reached and selection restored", func(t *testing.T) {
		page, observation := newPage(t)
		require.NoError(t, page.SetContent(rovingControlFixture))
		coverage, err := observation.KeyboardActionCoverage(ctx)
		require.NoError(t, err)
		assert.ElementsMatch(t, []KeyboardAction{
			{Name: "Permissions", Reached: true, VisibleFocus: true},
			{Name: "Connections", Reached: true, VisibleFocus: true},
			{Name: "Light", Reached: true, VisibleFocus: true},
			{Name: "Dark", Reached: true, VisibleFocus: true},
			{Name: "Panel action", Reached: true, VisibleFocus: true},
		}, coverage)
		selected, err := page.Evaluate(`() => [...document.querySelectorAll('[aria-selected="true"],[aria-checked="true"]')].map(el => el.textContent)`)
		require.NoError(t, err)
		assert.Equal(t, []any{"Permissions", "Light"}, selected)
	})

	t.Run("remounted panels retain action identities and nested roving coverage", func(t *testing.T) {
		page, observation := newPage(t)
		require.NoError(t, page.SetContent(`<style>[tabindex],button{outline:none}[tabindex]:focus-visible,button:focus-visible{outline:3px solid blue}</style>
			<div role="tablist"><button role="tab" aria-selected="true" tabindex="0">Permissions</button><button role="tab" aria-selected="false" tabindex="-1">Connections</button></div>
			<div id="slot"></div><script>
			function renderPanel() {
				const slot = document.getElementById('slot');
				slot.firstElementChild?.replaceChildren();
				slot.innerHTML = '<div role="tabpanel" tabindex="0"><div role="radiogroup"><button role="radio" aria-checked="true" tabindex="0">Light</button><button role="radio" aria-checked="false" tabindex="-1">Dark</button></div><button>Panel action</button></div>';
			}
			renderPanel();
			document.addEventListener('keydown', event => {
				const group = event.target.closest('[role="tablist"],[role="radiogroup"]');
				if (!group || !['ArrowRight','ArrowLeft'].includes(event.key)) return;
				event.preventDefault();
				const items = [...group.children], current = items.indexOf(document.activeElement);
				const next = (current + (event.key === 'ArrowRight' ? 1 : -1) + items.length) % items.length;
				requestAnimationFrame(() => {
					const attribute = group.getAttribute('role') === 'tablist' ? 'aria-selected' : 'aria-checked';
					items.forEach((item,i) => {item.tabIndex = i === next ? 0 : -1; item.setAttribute(attribute, String(i === next));});
					items[next].focus();
					if (attribute === 'aria-selected') renderPanel();
				});
			});
			</script>`))
		coverage, err := observation.KeyboardActionCoverage(ctx)
		require.NoError(t, err)
		assert.Contains(t, coverage, KeyboardAction{Name: "Dark", Reached: true, VisibleFocus: true})
		assert.Contains(t, coverage, KeyboardAction{Name: "Panel action", Reached: true, VisibleFocus: true})
		for _, action := range coverage {
			assert.True(t, action.Reached && action.VisibleFocus, action.Name)
		}
		selected, err := page.Evaluate(`() => [...document.querySelectorAll('[aria-selected="true"],[aria-checked="true"]')].map(el => el.textContent)`)
		require.NoError(t, err)
		assert.Equal(t, []any{"Permissions", "Light"}, selected)
	})

	t.Run("broken arrows do not hide unreachable roving items", func(t *testing.T) {
		page, observation := newPage(t)
		require.NoError(t, page.SetContent(`<style>button{outline:none}button:focus-visible{outline:3px solid blue}</style>
			<div role="tablist"><button role="tab" aria-selected="true" tabindex="0">Permissions</button>
			<button role="tab" aria-selected="false" tabindex="-1">Connections</button></div>`))
		coverage, err := observation.KeyboardActionCoverage(ctx)
		require.NoError(t, err)
		assert.Contains(t, coverage, KeyboardAction{Name: "Connections", Reached: false, VisibleFocus: false})
	})

	t.Run("theme captures isolate context and retain caller state", func(t *testing.T) {
		t.Setenv("E2E_CAPTURE_SCREENSHOTS", "true")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><style>html{background:white}html[data-theme="dark"]{background:black}</style>
				<script>document.documentElement.dataset.theme=localStorage.getItem('aib.theme')||'light'</script>
				<div id="root"><main style="min-height:1800px"><details><summary>Permissions</summary><p>Expanded content</p></details></main></div>`))
		}))
		t.Cleanup(server.Close)
		page, observation := newPage(t)
		observation.screenshotDir = t.TempDir()
		require.NoError(t, page.SetViewportSize(720, 600))
		require.NoError(t, observation.SetThemePreference(ctx, "system"))
		_, err := page.Goto(server.URL + "/observation?view=permissions")
		require.NoError(t, err)
		require.NoError(t, page.GetByText("Permissions", playwright.PageGetByTextOptions{Exact: playwright.Bool(true)}).Click())
		_, err = page.Evaluate(`() => window.callerSentinel = 'unchanged'`)
		require.NoError(t, err)
		var created, closed atomic.Int32
		observation.SetScreenshotContextFactory(func(context.Context, string) (playwright.BrowserContext, error) {
			capture, err := browser.NewContext()
			if err == nil {
				created.Add(1)
				capture.OnClose(func(playwright.BrowserContext) { closed.Add(1) })
			}
			return capture, err
		})
		require.NoError(t, observation.TakeThemedScreenshots(ctx, "isolated"))
		assert.Equal(t, int32(2), created.Load())
		assert.Equal(t, int32(2), closed.Load())
		assert.Equal(t, &playwright.Size{Width: 720, Height: 600}, page.ViewportSize())
		state, err := page.Evaluate(`() => ({open:document.querySelector('details').open,
			sentinel:window.callerSentinel, preference:localStorage.getItem('aib.theme'), path:location.pathname+location.search})`)
		require.NoError(t, err)
		assert.Equal(t, map[string]any{"open": true, "sentinel": "unchanged", "preference": "system",
			"path": "/observation?view=permissions"}, state)
		for _, theme := range []string{"light", "dark"} {
			file, err := os.Open(filepath.Join(observation.screenshotDir, "isolated_"+theme+".png"))
			require.NoError(t, err)
			image, err := png.Decode(file)
			require.NoError(t, file.Close())
			require.NoError(t, err)
			assert.Equal(t, 1280, image.Bounds().Dx())
			assert.Equal(t, 720, image.Bounds().Dy())
			r, g, b, _ := image.At(2, 2).RGBA()
			if theme == "light" {
				assert.Equal(t, uint32(65535), r)
				assert.Equal(t, uint32(65535), g)
				assert.Equal(t, uint32(65535), b)
			} else {
				assert.Zero(t, r)
				assert.Zero(t, g)
				assert.Zero(t, b)
			}
		}
		observation.SetScreenshotContextFactory(func(context.Context, string) (playwright.BrowserContext, error) {
			capture, err := browser.NewContext(playwright.BrowserNewContextOptions{Offline: playwright.Bool(true)})
			if err == nil {
				capture.OnClose(func(playwright.BrowserContext) { closed.Add(1) })
			}
			return capture, err
		})
		require.Error(t, observation.TakeThemedScreenshots(ctx, "offline"))
		assert.Equal(t, int32(3), closed.Load(), "failed capture must also close its isolated context")
		sentinel, err := page.Evaluate(`() => window.callerSentinel`)
		require.NoError(t, err)
		assert.Equal(t, "unchanged", sentinel)
	})
	t.Run("themed route captures wait for mounted content instead of a loading shell", func(t *testing.T) {
		t.Setenv("E2E_CAPTURE_SCREENSHOTS", "true")
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			_, _ = w.Write([]byte(`<!doctype html><style>body{margin:0;background:rgb(220,0,0)}main{height:100vh}</style><script>document.documentElement.dataset.theme=localStorage.getItem('aib.theme')||'light';setTimeout(()=>{document.body.innerHTML='<div id="root"><main>Loaded collection</main></div>';document.body.style.background='rgb(0,160,0)'},1500)</script><div role="status" aria-label="Loading collection">Loading collection</div>`))
		}))
		t.Cleanup(server.Close)
		for _, route := range []string{"/connections", "/approvals", "/approvals/remembered"} {
			t.Run(route, func(t *testing.T) {
				page, observation := newPage(t)
				observation.screenshotDir = t.TempDir()
				_, err := page.Goto(server.URL + route)
				require.NoError(t, err)
				observation.SetScreenshotContextFactory(func(context.Context, string) (playwright.BrowserContext, error) { return browser.NewContext() })
				require.NoError(t, observation.TakeThemedScreenshots(ctx, "loaded"))
				for _, theme := range []string{"light", "dark"} {
					file, err := os.Open(filepath.Join(observation.screenshotDir, "loaded_"+theme+".png"))
					require.NoError(t, err)
					image, err := png.Decode(file)
					require.NoError(t, file.Close())
					require.NoError(t, err)
					red, green, _, _ := image.At(100, 100).RGBA()
					assert.Zero(t, red, "loading shell must not be captured")
					assert.Equal(t, uint32(160*257), green, "capture must contain the loaded route")
				}
			})
		}
	})

	t.Run("agent collection exposes cards, list choice, and independent revoke action", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetContent(`<style>button{border:1px solid #933} [data-testid="agent-entity"]{height:88px}</style>
			<div role="group" aria-label="Grid view / List view">
				<button aria-label="Grid view" aria-pressed="true">Grid view</button>
				<button aria-label="List view" aria-pressed="false" onclick="this.setAttribute('aria-pressed','true');this.previousElementSibling.setAttribute('aria-pressed','false');document.querySelector('[data-testid=agent-entity]').dataset.slot='entity-row'">List view</button>
			</div>
			<div data-testid="agents-collection"><div data-testid="agent-entity" data-slot="entity-card">
				<a href="#detail" aria-label="Example agent"><span data-testid="entity-name">Example agent</span></a>
				<span data-testid="agent-expiry">Until revoked</span>
				<span>Changed <time data-testid="agent-changed-at">yesterday</time></span>
				<button>Revoke</button>
			</div></div>`))
		agents := NewDelegationsPage(page, "")
		view, err := agents.View(ctx)
		require.NoError(t, err)
		assert.Equal(t, "grid", view)
		rows, err := agents.Rows(ctx)
		require.NoError(t, err)
		assert.Equal(t, []DelegationsRow{{Agent: "Example agent", Expiry: "Until revoked", Recency: "Changed yesterday"}}, rows)
		count, err := agents.CardCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, count)
		require.NoError(t, agents.ChooseView(ctx, "list"))
		view, err = agents.View(ctx)
		require.NoError(t, err)
		assert.Equal(t, "list", view)
		visible, err := agents.VisibleRowCount(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, visible)
		require.NoError(t, agents.ClickCard(ctx, "Example agent"))
		assert.Contains(t, page.URL(), "#detail")
	})

	t.Run("inbox reads stable rows and panel arguments without inline decisions", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetContent(`<style>[data-testid="pending-approvals"]{height:180px;width:320px}
			</style>
			<div data-testid="pending-approvals" role="list">
				<div data-testid="pending-approval-row" data-selected="true"><button aria-label="Tool A" onclick="document.querySelectorAll('[data-testid=pending-approval-row]').forEach(row=>row.dataset.selected='false');this.parentElement.dataset.selected='true'"><span data-testid="entity-name">Tool A</span></button><span data-testid="approval-agent-name">Agent A</span><span data-testid="approval-risk">High Risk</span></div>
				<div data-testid="pending-approval-row" data-selected="false"><button aria-label="Tool B" onclick="document.querySelectorAll('[data-testid=pending-approval-row]').forEach(row=>row.dataset.selected='false');this.parentElement.dataset.selected='true'"><span data-testid="entity-name">Tool B</span></button><span data-testid="approval-agent-name">Agent B</span><span data-testid="approval-risk">Low Risk</span></div>
			</div>
			<section data-testid="approval-review-panel" tabindex="-1">
				<section aria-label="Arguments"><header><p>Arguments</p></header><dl><div><dt>path</dt><dd>/projects/roadmap</dd></div><div><dt>recursive</dt><dd>true</dd></div></dl></section>
				<button onclick="document.querySelector('[role=dialog]').hidden=false">View JSON</button><button>Approve once</button>
			</section>
			<div role="dialog" aria-label="Raw tool arguments" hidden><pre data-testid="approval-arguments">{"path":"/projects/roadmap","recursive":true}</pre><button onclick="this.parentElement.hidden=true">Close</button></div>`))
		inbox := NewApprovalsInboxPage(page, "")
		rows, err := inbox.PendingRows(ctx)
		require.NoError(t, err)
		assert.Equal(t, []PendingApprovalRow{{Tool: "Tool A", Agent: "Agent A", Risk: "High Risk"}, {Tool: "Tool B", Agent: "Agent B", Risk: "Low Risk"}}, rows)
		before, err := inbox.PendingListBounds(ctx)
		require.NoError(t, err)
		require.Len(t, before, 4)
		require.NoError(t, inbox.SelectPendingRow(ctx, "Tool B"))
		selected, err := inbox.SelectedTool(ctx)
		require.NoError(t, err)
		assert.Equal(t, "Tool B", selected)
		after, err := inbox.PendingListBounds(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, after)
		arguments, err := inbox.ArgumentRows(ctx)
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"path": "/projects/roadmap", "recursive": "true"}, arguments)
		require.NoError(t, inbox.OpenArgumentsJSON(ctx))
		raw, err := inbox.ArgumentsText(ctx)
		require.NoError(t, err)
		assert.JSONEq(t, `{"path":"/projects/roadmap","recursive":true}`, raw)
		require.NoError(t, inbox.CloseArgumentsJSON(ctx))
	})

	t.Run("inbox geometry ignores viewport scrolling but detects document reflow", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetViewportSize(320, 200))
		require.NoError(t, page.SetContent(`<style>body{margin:0;min-height:1800px}[data-testid="pending-approvals"]{margin-top:600px;width:280px;height:180px}</style><div data-testid="pending-approvals"><div data-testid="pending-approval-row"><button aria-label="Tool A"><span data-testid="entity-name">Tool A</span></button></div></div>`))
		inbox := NewApprovalsInboxPage(page, "")
		before, err := inbox.PendingListBounds(ctx)
		require.NoError(t, err)
		_, err = page.Evaluate(`window.scrollTo(0, 400)`)
		require.NoError(t, err)
		_, err = page.WaitForFunction(`() => window.scrollY === 400`, nil)
		require.NoError(t, err)
		scrolled, err := inbox.PendingListBounds(ctx)
		require.NoError(t, err)
		assert.Equal(t, before, scrolled)
		_, err = page.Evaluate(`document.querySelector('[data-testid="pending-approvals"]').style.marginTop = '640px'`)
		require.NoError(t, err)
		moved, err := inbox.PendingListBounds(ctx)
		require.NoError(t, err)
		before[1] += 40
		assert.Equal(t, before, moved)
	})

	t.Run("appearance preference uses pressed buttons, not a second theme menu", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetContent(`<div role="radiogroup" aria-label="Appearance">
				<button role="radio" aria-label="Light" aria-checked="true">Light</button>
				<button role="radio" aria-label="Dark" aria-checked="false">Dark</button>
				<button role="radio" aria-label="System" aria-checked="false">System</button></div>
			<div role="group" aria-label="Default collection view">
				<button aria-label="Grid view" aria-pressed="true" onclick="this.setAttribute('aria-pressed','true');this.nextElementSibling.setAttribute('aria-pressed','false')">Grid view</button>
				<button aria-label="List view" aria-pressed="false" onclick="this.setAttribute('aria-pressed','true');this.previousElementSibling.setAttribute('aria-pressed','false')">List view</button></div>`))
		settings := NewSettingsPage(page, "")
		previews, err := settings.HasThemePreviews(ctx)
		require.NoError(t, err)
		assert.True(t, previews)
		require.NoError(t, settings.ChooseDefaultView(ctx, "list"))
		selected, err := settings.SelectedDefaultView(ctx)
		require.NoError(t, err)
		assert.Equal(t, "list", selected)
		categories, err := settings.HasCategoryMenu(ctx)
		require.NoError(t, err)
		assert.False(t, categories)
	})

	t.Run("consent observes one-service locks, granted hints and viewport fit", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetViewportSize(1280, 720))
		require.NoError(t, page.SetContent(`<main>
			<form data-testid="consent-card" style="width:480px"><header data-testid="agent-name-heading"><h1><span data-testid="agent-name">Example agent</span> wants to act on your behalf</h1></header>
			<section data-testid="permission-groups"><ul><li data-testid="permission-group">
				<div><button role="checkbox" aria-label="Code Access" aria-checked="true" disabled></button>
					<div><div><span><span data-testid="permission-group-name">Code Access</span><span>Required</span></span>
						<span data-testid="permission-group-description"><span>Read a repository</span><span data-testid="permission-group-granted">Granted</span></span>
					</div></div><div><span data-slot="avatar" role="img" aria-label="GitHub">G</span></div></div>
			</li></ul></section><footer data-testid="consent-footer"><button>Allow</button></footer></form></main>`))
		consent := NewConsentPage(page, "")
		name, err := consent.GetAgentName(ctx)
		require.NoError(t, err)
		assert.Equal(t, "Example agent", name)
		groups, err := consent.PermissionGroups(ctx)
		require.NoError(t, err)
		assert.Equal(t, []PermissionGroup{{Name: "Code Access", Description: "Read a repository", Required: true, AlreadyGranted: true, ReadOnly: true, Checked: true}}, groups)
		services, err := consent.GroupServices(ctx, "Code Access")
		require.NoError(t, err)
		assert.Equal(t, []string{"GitHub"}, services)
		disclosure, err := consent.HasServiceDisclosure(ctx, "Code Access")
		require.NoError(t, err)
		assert.False(t, disclosure)
		width, err := consent.DecisionCardWidth(ctx)
		require.NoError(t, err)
		assert.InDelta(t, 480, width, 1)
		bottom, err := consent.AllowViewportBottom(ctx)
		require.NoError(t, err)
		assert.LessOrEqual(t, bottom, float64(720))
	})

	t.Run("connection metadata and popover reveal only rendered scopes", func(t *testing.T) {
		page, _ := newPage(t)
		require.NoError(t, page.SetContent(`<main><div data-testid="connections-collection">
			<div data-testid="connection-card" data-slot="entity-card"><span data-testid="entity-name">Example provider</span>
				<span data-testid="connection-state">Expired</span>
				<button data-testid="connection-scope-count" onclick="document.querySelector('[data-slot=popover-content]').hidden=false">2 scopes</button>
				<time data-testid="connection-created-at" datetime="2026-10-03T10:00:00Z">3 Oct</time>
				<button data-testid="connection-action">Reconnect</button><button id="disconnect-session-1">Disconnect</button>
			</div></div><div hidden data-slot="popover-content"><p>Granted scopes</p><ul><li>read</li><li>write</li></ul></div></main>
			<script>document.addEventListener('keydown',event=>{if(event.key==='Escape')document.querySelector('[data-slot=popover-content]').hidden=true})</script>`))
		connections := NewConnectionsPage(page, "")
		rows, err := connections.ConnectionRows(ctx)
		require.NoError(t, err)
		assert.Equal(t, []ConnectionRow{{Provider: "Example provider", State: "Expired", ScopeCount: 2, Action: "Reconnect Disconnect", CreatedAt: "2026-10-03T10:00:00Z"}}, rows)
		scopes, err := connections.ConnectionScopes(ctx, "Example provider")
		require.NoError(t, err)
		assert.Equal(t, []string{"read", "write"}, scopes)
	})

	t.Run("reduced motion is observable through browser media", func(t *testing.T) {
		page, observation := newPage(t)
		require.NoError(t, page.SetContent(`<main>Reduced motion</main>`))
		require.NoError(t, observation.EmulateReducedMotion(ctx, true))
		reduced, err := observation.ReducedMotionMatches(ctx)
		require.NoError(t, err)
		assert.True(t, reduced)
		require.NoError(t, observation.EmulateReducedMotion(ctx, false))
		reduced, err = observation.ReducedMotionMatches(ctx)
		require.NoError(t, err)
		assert.False(t, reduced)
	})
}

const rovingControlFixture = `<style>button{outline:none}button:focus-visible{outline:3px solid blue}</style>
<div role="tablist"><button role="tab" aria-selected="true" tabindex="0">Permissions</button><button role="tab" aria-selected="false" tabindex="-1">Connections</button></div>
<div role="radiogroup"><button role="radio" aria-checked="true" tabindex="0">Light</button><button role="radio" aria-checked="false" tabindex="-1">Dark</button></div>
<div id="panel"><button>Panel action</button></div>
<script>
for (const group of document.querySelectorAll('[role="tablist"],[role="radiogroup"]')) {
	group.addEventListener('keydown', event => {
		if (!['ArrowRight','ArrowLeft'].includes(event.key)) return;
		event.preventDefault();
		const items = [...group.children], current = items.indexOf(document.activeElement);
		const next = (current + (event.key === 'ArrowRight' ? 1 : -1) + items.length) % items.length;
		const attribute = group.getAttribute('role') === 'tablist' ? 'aria-selected' : 'aria-checked';
		items.forEach((item,i) => {item.tabIndex = i === next ? 0 : -1; item.setAttribute(attribute, String(i === next));});
		items[next].focus();
		if (attribute === 'aria-selected') document.getElementById('panel').innerHTML = '<button>Panel action</button>';
	});
}
</script>`
