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
				<details><summary>Permissions</summary><p>Expanded content</p></details>`))
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
			assert.Equal(t, 720, image.Bounds().Dx())
			assert.Equal(t, 600, image.Bounds().Dy())
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
