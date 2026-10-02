// Package pages provides page object models for frontend E2E testing with Playwright.
// Page objects abstract away selector details and provide high-level methods that describe
// user interactions from a functional perspective.
//
// All page objects should embed the base Page struct and use its methods for navigation
// and waiting. Subclasses should define their own methods for page-specific interactions.
//
// Example pattern:
//
//	type ConsentPage struct {
//		*Page  // Embed base Page
//	}
//
//	func NewConsentPage(page playwright.Page, baseURL string) *ConsentPage {
//		return &ConsentPage{
//			Page: NewPage(page, baseURL),
//		}
//	}
//
//	func (cp *ConsentPage) ClickApprove(ctx context.Context) error {
//		// Use cp.page to interact with selectors
//		// Wrap errors with fmt.Errorf using %w
//	}
package pages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/mxschmitt/playwright-go"
)

const screenshotWaitTimeoutMs = 10_000

const screenshotDynamicStyle = "[data-screenshot-dynamic] { display: none !important; }"

type screenshotPage interface {
	WaitForLoadState(options ...playwright.PageWaitForLoadStateOptions) error
	Evaluate(expression string, arg ...any) (any, error)
	Screenshot(options ...playwright.PageScreenshotOptions) ([]byte, error)
}

// Page represents a Playwright page with common navigation and interaction utilities.
// This is the base struct that all page objects (ConsentPage, AgentDetailPage, etc.) embed.
// It provides high-level methods for navigation, waiting, and screenshots while keeping
// selector details private to subclasses.
//
// Design principles:
// - No selectors exposed to callers
// - Context propagation for all async operations
// - Implicit waits via Playwright (no manual sleep)
// - Clear error messages with proper error wrapping
type Page struct {
	page                     playwright.Page
	baseURL                  string
	timeout                  time.Duration
	screenshotDir            string
	requests                 requestLog
	requestHandler           func(playwright.Request)
	closeHandler             func(playwright.BrowserContext)
	screenshotContextFactory func(context.Context, string) (playwright.BrowserContext, error)
}

// NewPage creates a new Page wrapper around a Playwright page.
// This initializes the page with a 30-second timeout (Playwright default)
// and prepares the screenshot directory.
//
// Parameters:
//   - page: Playwright page instance (required)
//   - baseURL: Base URL for navigation (e.g., "http://localhost:3000")
//
// Returns:
//   - *Page: Initialized page wrapper
//   - Panics if page is nil (programming error, not recoverable)
//
// Example:
//
//	page, err := context.NewPage()
//	require.NoError(t, err)
//	p := pages.NewPage(page, "http://localhost:3000")
func NewPage(page playwright.Page, baseURL string) *Page {
	if page == nil {
		panic("page is required and cannot be nil")
	}

	// Prepare screenshot directory
	screenshotDir := filepath.Join("coverage", "screenshots")
	_ = os.MkdirAll(screenshotDir, 0o700) // Best effort; don't fail if it fails

	return &Page{
		page:          page,
		baseURL:       baseURL,
		timeout:       30 * time.Second,
		screenshotDir: screenshotDir,
	}
}

// Navigate navigates to the given path relative to the base URL.
// Path should start with "/" or be a path like "agent/123".
// The method will combine baseURL + path to create the full URL.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - path: Relative path (e.g., "/", "agent/123")
//
// Returns:
//   - error: If navigation fails (network error, invalid URL, etc.)
//
// Error messages include the full URL for debugging:
// - "failed to navigate to http://localhost:3000/: context cancelled"
// - "failed to navigate to http://localhost:3000/404: HTTP 404"
//
// Example:
//
//	err := p.Navigate(ctx, "/")
//	require.NoError(t, err)
//
//	// Navigate with path parameters
//	err = p.Navigate(ctx, "/agents/123/detail")
//	require.NoError(t, err)
func (p *Page) Navigate(ctx context.Context, path string) error {
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	url := p.baseURL + path
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if _, err := p.page.Goto(url, playwright.PageGotoOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("failed to navigate to %s: %w", url, err)
	}
	timeout, err = p.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	_, err = p.page.WaitForFunction(`() => {
		const main = document.getElementById('root')?.querySelector('main');
		if (!main) return false;
		return ![...main.querySelectorAll('[aria-busy="true"], [role="status"]')].some(element =>
			element.getAttribute('aria-busy') === 'true' ||
			/^Loading/.test((element.getAttribute('aria-label') || element.textContent || '').trim()));
	}`, nil, playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("application did not finish loading at %s: %w", url, err)
	}
	return nil
}

// GetBaseURL returns the base URL of this page.
// Useful for subclasses that need to build URLs programmatically.
//
// Returns:
//   - string: Base URL (e.g., "http://localhost:3000")
//
// Example:
//
//	baseURL := p.GetBaseURL()
//	// Use in subclass to build full URL
func (p *Page) GetBaseURL() string {
	return p.baseURL
}

// WaitForURL waits for the page URL to match a pattern (regex or substring)
// and for the matching document to load.
//
// The URL predicate runs in the browser, so it observes both the current URL and
// a redirect that completes immediately after a user action.
func (p *Page) WaitForURL(ctx context.Context, pattern string) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("context cancelled while waiting for URL %q: %w", pattern, ctx.Err())
	default:
	}

	urlPattern, err := regexp.Compile(pattern)
	if err != nil {
		urlPattern = regexp.MustCompile(regexp.QuoteMeta(pattern))
	}

	_, err = p.page.WaitForFunction(
		"(pattern) => new RegExp(pattern).test(window.location.href)",
		urlPattern.String(),
		playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(float64(p.timeout.Milliseconds())),
		},
	)
	if err != nil {
		return fmt.Errorf("failed waiting for URL to match %q (current: %q): %w", pattern, p.page.URL(), err)
	}

	if err := p.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateLoad,
		Timeout: playwright.Float(float64(p.timeout.Milliseconds())),
	}); err != nil {
		return fmt.Errorf("failed waiting for URL %q to load: %w", pattern, err)
	}

	return nil
}

// WaitForNavigation waits for the page to navigate (URL change).
// This is useful after clicking a link that triggers navigation.
// Returns when a navigation event occurs or timeout expires.
// Internally uses WaitForLoadState to detect navigation completion.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//
// Returns:
//   - error: If timeout expires or context is cancelled
//
// Example:
//
//	// Click a link and wait for navigation
//	err := p.page.Click("#continue-link")
//	require.NoError(t, err)
//
//	err = p.WaitForNavigation(ctx)
//	require.NoError(t, err)
//
//	// Now on the new page
//	currentURL := p.page.URL()
func (p *Page) WaitForNavigation(ctx context.Context) error {
	// Set timeout
	_, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Wait for page to reach load state after navigation
	// This ensures the new page is fully loaded
	if err := p.page.WaitForLoadState(); err != nil {
		return fmt.Errorf("timeout waiting for navigation (waited %v): %w", p.timeout, err)
	}

	return nil
}

// TakeScreenshot captures a screenshot of the current page state.
// Screenshots are saved to coverage/screenshots/{name}.png.
// This is useful for debugging test failures.
//
// Parameters:
//   - ctx: Context for cancellation (Playwright has built-in timeout)
//   - name: Screenshot name (e.g., "consent_page_loaded")
//
// Returns:
//   - error: If screenshot capture fails
//
// File location:
//   - Screenshots are saved to: coverage/screenshots/{name}.png
//   - Directory is created automatically if it doesn't exist
//
// Example:
//
//	// Take screenshot on test failure
//	err := p.Navigate(ctx, "/")
//	if err != nil {
//		_ = p.TakeScreenshot(ctx, "navigation_failed")
//		t.Fatal(err)
//	}
//
//	// Take screenshot for comparison
//	_ = p.TakeScreenshot(ctx, "consent_page_state")
func (p *Page) TakeScreenshot(ctx context.Context, name string) error {
	_ = ctx
	if !captureScreenshotsEnabled() {
		return nil
	}
	return captureScreenshot(p.page, p.screenshotDir, name)
}

func captureScreenshot(page screenshotPage, screenshotDir, name string) error {
	if name == "" {
		return fmt.Errorf("screenshot name cannot be empty")
	}

	if err := os.MkdirAll(screenshotDir, 0o700); err != nil {
		return fmt.Errorf("failed to create screenshot directory %s: %w", screenshotDir, err)
	}

	filePath := filepath.Join(screenshotDir, name+".png")

	if err := waitForScreenshotStability(page, name); err != nil {
		return err
	}

	// Scroll to the top of the page before capturing so that the sticky
	// header is always positioned at y=0 in the full-page composite image.
	// Without this, the header's pixel position shifts depending on the
	// current scroll offset at capture time, producing spurious diffs.
	if _, err := page.Evaluate("window.scrollTo(0, 0)"); err != nil {
		return fmt.Errorf("failed to scroll to top before screenshot %s: %w", name, err)
	}

	// Take screenshot with animations disabled so that CSS transitions
	// (e.g. modal dialog entrance animations) are fast-forwarded to their
	// final state.  This avoids flaky captures where a dialog is mid-fade.
	data, err := page.Screenshot(playwright.PageScreenshotOptions{
		Animations: playwright.ScreenshotAnimationsDisabled,
		FullPage:   playwright.Bool(true),
		Style:      playwright.String(screenshotDynamicStyle),
	})
	if err != nil {
		return fmt.Errorf("failed to take screenshot %s: %w", filePath, err)
	}

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write screenshot to %s: %w", filePath, err)
	}

	return nil
}

// TakeLocatorScreenshot captures a screenshot of a specific locator.
// Use this for overlays/dialogs where full-page captures include unstable background content.
func (p *Page) TakeLocatorScreenshot(ctx context.Context, name string, locator playwright.Locator) error {
	_ = ctx
	if !captureScreenshotsEnabled() {
		return nil
	}

	if name == "" {
		return fmt.Errorf("screenshot name cannot be empty")
	}

	if err := os.MkdirAll(p.screenshotDir, 0o700); err != nil {
		return fmt.Errorf("failed to create screenshot directory %s: %w", p.screenshotDir, err)
	}

	filePath := filepath.Join(p.screenshotDir, name+".png")

	if err := waitForScreenshotStability(p.page, name); err != nil {
		return err
	}

	data, err := locator.Screenshot(playwright.LocatorScreenshotOptions{
		Animations: playwright.ScreenshotAnimationsDisabled,
		Style:      playwright.String(screenshotDynamicStyle),
	})
	if err != nil {
		return fmt.Errorf("failed to take locator screenshot %s: %w", filePath, err)
	}

	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("failed to write screenshot to %s: %w", filePath, err)
	}

	return nil
}

func waitForScreenshotStability(page screenshotPage, name string) error {
	// Wait for network to be idle before capturing to avoid intermediate loading states
	// (spinners, skeleton screens) that cause screenshot flicker between runs.
	screenshotTimeout := float64(screenshotWaitTimeoutMs)
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State:   playwright.LoadStateNetworkidle,
		Timeout: &screenshotTimeout,
	}); err != nil {
		if !isPlaywrightTimeout(err) {
			return fmt.Errorf("failed waiting for network idle before screenshot %s: %w", name, err)
		}
	}

	if err := waitForScreenshotRenderStability(page, name); err != nil {
		return err
	}

	return nil
}

func waitForScreenshotRenderStability(page screenshotPage, name string) error {
	result, err := page.Evaluate(`async () => {
		await document.fonts.ready
		for (const [family, specification, sample] of [
			['Crimson Pro', '700 24px "Crimson Pro"', 'Consent Management'],
			['Manrope', '400 16px "Manrope"', 'Approve & Delegate'],
			['JetBrains Mono', '400 14px "JetBrains Mono"', 'tool_name'],
		]) {
			try {
				const faces = await document.fonts.load(specification, sample)
				if (faces.length === 0 || !document.fonts.check(specification, sample)) return family
			} catch (_) {
				return family
			}
		}
		await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))
		return ''
	}`)
	if err != nil {
		return fmt.Errorf("failed waiting for font/render stability before screenshot %s: %w", name, err)
	}
	if family, ok := result.(string); !ok {
		return fmt.Errorf("invalid font stability result before screenshot %s: %T", name, result)
	} else if family != "" {
		return fmt.Errorf("font %s unavailable before screenshot %s", family, name)
	}

	return nil
}

func isPlaywrightTimeout(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "timeout") || strings.Contains(message, "timed out")
}

// GetCurrentURL returns the current page URL.
// Useful for verifying navigation without waiting.
//
// Returns:
//   - string: Current page URL (e.g., "http://localhost:3000/")
//   - error: Never returns error; always succeeds
//
// Example:
//
//	currentURL, err := p.GetCurrentURL(ctx)
//	require.NoError(t, err)
//	fmt.Printf("Currently on: %s\n", currentURL)
//
//	// Verify that a URL was returned.
//	require.NotEmpty(t, currentURL)
func (p *Page) GetCurrentURL(ctx context.Context) (string, error) {
	return p.page.URL(), nil
}

// GetPlaywrightPage returns the underlying Playwright page object.
// This is used by subclasses (like ConsentPage) to access Playwright APIs.
//
// Returns:
//   - playwright.Page: The underlying Playwright page
//
// Example:
//
//	page := p.GetPlaywrightPage()
//	page.GetByRole("button").Click()
func (p *Page) GetPlaywrightPage() playwright.Page {
	return p.page
}

func captureScreenshotsEnabled() bool {
	value := strings.TrimSpace(os.Getenv("E2E_CAPTURE_SCREENSHOTS"))
	return strings.EqualFold(value, "true") || value == "1"
}

// ThemeFrame records the theme observed before the browser's first painted frame.
type ThemeFrame struct {
	Theme       string `json:"theme"`
	ColorScheme string `json:"colorScheme"`
}

type RecordedRequest struct {
	URL    string
	Method string
}

// requestLog protects event callbacks against simultaneous reads and late events
// from an earlier recording. Returned snapshots never share the mutable buffer.
type requestLog struct {
	mu         sync.Mutex
	generation uint64
	active     bool
	entries    []RecordedRequest
}

func (r *requestLog) start() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.generation++
	r.active = true
	r.entries = nil
	return r.generation
}

func (r *requestLog) record(generation uint64, request RecordedRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.active && generation == r.generation {
		r.entries = append(r.entries, request)
	}
}

func (r *requestLog) stop() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.active = false
}

func (r *requestLog) snapshot() []RecordedRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]RecordedRequest{}, r.entries...)
}

func (p *Page) locatorTimeout(ctx context.Context) (*float64, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	timeout := p.timeout
	if deadline, ok := ctx.Deadline(); ok {
		timeout = min(timeout, time.Until(deadline))
	}
	// Playwright interprets zero as unlimited.
	return playwright.Float(max(1, float64(timeout.Milliseconds()))), nil
}

func (p *Page) locatorClick(ctx context.Context, locator playwright.Locator, description string) error {
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	if err := locator.Click(playwright.LocatorClickOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("click %s: %w", description, err)
	}
	return ctx.Err()
}

func (p *Page) locatorText(ctx context.Context, locator playwright.Locator, description string) (string, error) {
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return "", err
	}
	text, err := locator.InnerText(playwright.LocatorInnerTextOptions{Timeout: timeout})
	if err != nil {
		return "", fmt.Errorf("read %s: %w", description, err)
	}
	return strings.TrimSpace(text), ctx.Err()
}

func (p *Page) locatorVisible(ctx context.Context, locator playwright.Locator, description string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	visible, err := locator.IsVisible()
	if err != nil {
		return false, fmt.Errorf("inspect %s: %w", description, err)
	}
	return visible, ctx.Err()
}

func (p *Page) evaluateJSON(ctx context.Context, locator playwright.Locator, script string, result any) error {
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	value, err := locator.Evaluate(script, nil, playwright.LocatorEvaluateOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("evaluate page observation: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode page observation: %w", err)
	}
	if err := json.Unmarshal(data, result); err != nil {
		return fmt.Errorf("decode page observation: %w", err)
	}
	return nil
}

// SetThemePreference seeds a fresh browser before navigation. Existing stored
// preferences are preserved on reload so tests can observe real user changes.
// Install it before the first navigation; the same script records the first frame.
func (p *Page) SetThemePreference(ctx context.Context, value string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if value != "light" && value != "dark" && value != "system" {
		return fmt.Errorf("unsupported theme preference %q", value)
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	script := `(() => {
		if (window !== window.top) return;
		if (localStorage.getItem('__aibE2EThemeSeeded') === null) {
			if (localStorage.getItem('aib.theme') === null) localStorage.setItem('aib.theme', ` + string(encoded) + `);
			localStorage.setItem('__aibE2EThemeSeeded', 'true');
		}
		requestAnimationFrame(() => {
			window.__aibFirstPaintTheme = {
				theme: document.documentElement.dataset.theme || '',
				colorScheme: getComputedStyle(document.documentElement).colorScheme
			};
		});
	})()`
	if err := p.page.Context().AddInitScript(playwright.Script{Content: &script}); err != nil {
		return fmt.Errorf("install theme preference and first-frame recorder: %w", err)
	}
	return ctx.Err()
}

func (p *Page) EmulateColorScheme(ctx context.Context, scheme string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if scheme != "light" && scheme != "dark" && scheme != "no-override" {
		return fmt.Errorf("unsupported color scheme %q", scheme)
	}
	colorScheme := playwright.ColorScheme(scheme)
	if err := p.page.EmulateMedia(playwright.PageEmulateMediaOptions{ColorScheme: &colorScheme}); err != nil {
		return fmt.Errorf("emulate color scheme: %w", err)
	}
	return ctx.Err()
}

func (p *Page) ResolvedTheme(ctx context.Context) (string, error) {
	var theme string
	err := p.evaluateJSON(ctx, p.page.Locator("html"), `root => root.dataset.theme || ''`, &theme)
	return theme, err
}

func (p *Page) FirstPaintTheme(ctx context.Context) (ThemeFrame, error) {
	var frame ThemeFrame
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return frame, err
	}
	handle, err := p.page.WaitForFunction(`() => window.__aibFirstPaintTheme !== undefined`, nil,
		playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return frame, fmt.Errorf("wait for first frame (install SetThemePreference before navigation): %w", err)
	}
	if err := handle.Dispose(); err != nil {
		return frame, err
	}
	err = p.evaluateJSON(ctx, p.page.Locator("html"), `() => window.__aibFirstPaintTheme`, &frame)
	return frame, err
}

// StartRequestRecorder observes the entire context, including popup requests,
// navigations, and subresources. Start/stop are called by the scenario goroutine;
// callbacks and snapshots may run concurrently.
func (p *Page) StartRequestRecorder(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.StopRequestRecorder(ctx); err != nil {
		return err
	}
	generation := p.requests.start()
	p.requestHandler = func(request playwright.Request) {
		p.requests.record(generation, RecordedRequest{URL: request.URL(), Method: request.Method()})
	}
	p.closeHandler = func(playwright.BrowserContext) { p.requests.stop() }
	p.page.Context().OnRequest(p.requestHandler)
	p.page.Context().OnClose(p.closeHandler)
	return nil
}

// StopRequestRecorder always detaches its own listeners, even after cancellation.
func (p *Page) StopRequestRecorder(ctx context.Context) error {
	p.requests.stop()
	if p.requestHandler != nil {
		p.page.Context().RemoveListener("request", p.requestHandler)
		p.requestHandler = nil
	}
	if p.closeHandler != nil {
		p.page.Context().RemoveListener("close", p.closeHandler)
		p.closeHandler = nil
	}
	return ctx.Err()
}

func (p *Page) RecordedRequests(ctx context.Context) ([]RecordedRequest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return p.requests.snapshot(), nil
}

func (p *Page) HasHorizontalPageOverflow(ctx context.Context) (bool, error) {
	var overflow bool
	err := p.evaluateJSON(ctx, p.page.Locator("html"), `root => root.scrollWidth > root.clientWidth`, &overflow)
	return overflow, err
}

func zoomViewport(percent int) (int, int, error) {
	if percent <= 0 {
		return 0, 0, fmt.Errorf("zoom must be positive, got %d", percent)
	}
	width, height := 128000/percent, 108000/percent
	if width < 1 || height < 1 {
		return 0, 0, fmt.Errorf("zoom %d produces an empty viewport", percent)
	}
	return width, height, nil
}

// EmulateZoom changes CSS layout dimensions, not deviceScaleFactor, which only
// changes rasterization and does not exercise the reflow required at 200%.
func (p *Page) EmulateZoom(ctx context.Context, percent int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	width, height, err := zoomViewport(percent)
	if err != nil {
		return err
	}
	if err := p.page.SetViewportSize(width, height); err != nil {
		return fmt.Errorf("emulate %d%% layout: %w", percent, err)
	}
	return ctx.Err()
}

func (p *Page) PrimaryAccentActionCount(ctx context.Context) (int, error) {
	labels, err := p.PrimaryAccentActionLabels(ctx)
	return len(labels), err
}

func (p *Page) PrimaryAccentActionLabels(ctx context.Context) ([]string, error) {
	var labels []string
	err := p.evaluateJSON(ctx, p.page.Locator("html"), `root =>
		[...root.querySelectorAll('[data-variant="primary"]')].filter(el => {
			if (el.closest('[aria-hidden="true"], [inert]')) return false;
			const style = getComputedStyle(el);
			const rect = el.getBoundingClientRect();
			return style.visibility === 'visible' && style.display !== 'none' &&
				Number(style.opacity) > 0 && rect.width > 0 && rect.height > 0;
		}).map(el => {
			const labelled = (el.getAttribute('aria-labelledby') || '').split(/\s+/)
				.map(id => document.getElementById(id)?.textContent || '').join(' ').trim();
			return (el.getAttribute('aria-label') || labelled || el.innerText || el.value || '').trim();
		})`, &labels)
	return labels, err
}

type FocusDescription struct {
	Tag          string `json:"tag"`
	Role         string `json:"role"`
	Name         string `json:"name"`
	Visible      bool   `json:"visible"`
	FocusVisible bool   `json:"focusVisible"`
	Unobscured   bool   `json:"unobscured"`
}

const describeFocusScript = `() => {
	const el = document.activeElement;
	if (!el) return {tag:'', role:'', name:'', visible:false, focusVisible:false, unobscured:false};
	const rect = el.getBoundingClientRect(), style = getComputedStyle(el);
	const labelled = (el.getAttribute('aria-labelledby') || '').split(/\s+/)
		.map(id => document.getElementById(id)?.textContent || '').join(' ').trim();
	const name = el.getAttribute('aria-label') || labelled ||
		[...(el.labels || [])].map(label => label.textContent).join(' ') ||
		el.innerText || el.getAttribute('title') || el.getAttribute('placeholder') || '';
	const visible = rect.width > 0 && rect.height > 0 &&
		el.checkVisibility({checkOpacity: true, checkVisibilityCSS: true}) &&
		!el.closest('[inert], [aria-hidden="true"]');
	const x = Math.max(0, rect.left) + (Math.min(innerWidth, rect.right) - Math.max(0, rect.left))/2;
	const y = Math.max(0, rect.top) + (Math.min(innerHeight, rect.bottom) - Math.max(0, rect.top))/2;
	const hit = document.elementFromPoint(x,y);
	const unobscured = visible && x >= 0 && y >= 0 && x < innerWidth && y < innerHeight &&
		!!hit && (hit === el || el.contains(hit));
	const decoration = node => {
		const css = getComputedStyle(node);
		return {outline: [css.outlineWidth, css.outlineStyle, css.outlineColor].join(' '),
			shadow: css.boxShadow, border: [css.borderColor, css.borderWidth, css.borderStyle].join(' ')};
	};
	const focused = decoration(el);
	const hasFocus = el.matches(':focus-within');
	let baseline = window.__aibKeyboardBaselines?.get(el);
	if (!baseline && typeof el.blur === 'function' && typeof el.focus === 'function') {
		el.blur();
		baseline = decoration(el);
		el.focus({preventScroll:true});
	}
	const hasOutline = parseFloat(style.outlineWidth) > 0 && style.outlineStyle !== 'none' &&
		style.outlineColor !== 'transparent' && style.outlineColor !== 'rgba(0, 0, 0, 0)';
	const hasShadow = focused.shadow.split(/,(?![^(]*\))/).some(layer => {
		const transparent = /\btransparent\b|rgba\([^)]*,\s*0(?:\.0+)?\)|\/\s*0(?:\.0+)?%?\s*\)/.test(layer);
		const hasExtent = [...layer.matchAll(/(-?[\d.]+)px/g)].some(match => Number(match[1]) !== 0);
		return !transparent && hasExtent;
	});
	const hasRing = !!baseline && (
		(hasOutline && focused.outline !== baseline.outline) ||
		(hasShadow && focused.shadow !== baseline.shadow) ||
		focused.border !== baseline.border);
	return {tag:el.tagName.toLowerCase(), role:el.getAttribute('role') || '',
		name:name.trim(), visible, focusVisible:visible && hasFocus && hasRing, unobscured};
}`

func (p *Page) ActiveElementDescription(ctx context.Context) (FocusDescription, error) {
	var description FocusDescription
	err := p.evaluateJSON(ctx, p.page.Locator("html"), describeFocusScript, &description)
	return description, err
}

func (p *Page) VisibleText(ctx context.Context) (string, error) {
	return p.locatorText(ctx, p.page.Locator("body"), "visible page text")
}

type KeyboardAction struct {
	Name         string
	Reached      bool
	VisibleFocus bool
}

// KeyboardActionCoverage uses Tab for sequential targets and arrow keys for
// every enabled item in a tablist/radiogroup. Each roving group is returned to its
// starting selection before continuing; no action button is activated.
func (p *Page) KeyboardActionCoverage(ctx context.Context) ([]KeyboardAction, error) {
	var actions []struct {
		Name  string `json:"name"`
		Group int    `json:"group"`
	}
	err := p.evaluateJSON(ctx, p.page.Locator("html"), `root => {
		const original = document.activeElement;
		window.__aibKeyboardOriginal = original;
		original?.blur();
		const actionSelector = 'a[href],button,input:not([type="hidden"]),select,textarea,[role="button"],[role="tab"],[role="radio"],[role="checkbox"],[role="switch"],[role="option"],[role^="menuitem"]';
		window.__aibKeyboardFindTargets = () => [...root.querySelectorAll(actionSelector + ',[tabindex]')].filter(el => {
			const r = el.getBoundingClientRect(), s = getComputedStyle(el);
			return (el.tabIndex >= 0 || el.matches(actionSelector)) && !el.disabled && el.getAttribute('aria-disabled') !== 'true' &&
				!el.closest('[inert],[aria-hidden="true"]') && r.width > 0 && r.height > 0 &&
				s.visibility === 'visible' && s.display !== 'none' &&
				!el.matches('[role="tablist"],[role="radiogroup"]');
		});
		window.__aibKeyboardTargets = window.__aibKeyboardFindTargets();
		window.__aibKeyboardIdentity = el => [el.tagName, el.getAttribute('role'), el.getAttribute('aria-label'), el.textContent].join('|');
		window.__aibKeyboardIdentities = window.__aibKeyboardTargets.map(window.__aibKeyboardIdentity);
		const groups = [...root.querySelectorAll('[role="tablist"],[role="radiogroup"]')];
		window.__aibKeyboardGroups = groups;
		window.__aibKeyboardBaselines = new WeakMap();
		for (const el of window.__aibKeyboardTargets) {
			const css = getComputedStyle(el);
			window.__aibKeyboardBaselines.set(el, {
				outline:[css.outlineWidth,css.outlineStyle,css.outlineColor].join(' '),
				shadow:css.boxShadow, border:[css.borderColor,css.borderWidth,css.borderStyle].join(' ')
			});
		}
		original?.focus({preventScroll:true});
		return window.__aibKeyboardTargets.map(el => ({
			name:el.getAttribute('aria-label') ||
				[...(el.labels || [])].map(label => label.textContent).join(' ') ||
				el.innerText || el.getAttribute('placeholder') || el.tagName.toLowerCase(),
			group:groups.indexOf(el.closest('[role="tablist"],[role="radiogroup"]'))
		}));
	}`, &actions)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = p.page.Evaluate(`() => {
			window.__aibKeyboardOriginal?.focus({preventScroll:true});
			delete window.__aibKeyboardTargets;
			delete window.__aibKeyboardOriginal;
			delete window.__aibKeyboardGroups;
			delete window.__aibKeyboardBaselines;
			delete window.__aibKeyboardFindTargets;
			delete window.__aibKeyboardIdentity;
			delete window.__aibKeyboardIdentities;
		}`)
	}()
	results := make([]KeyboardAction, len(actions))
	for i, action := range actions {
		results[i].Name = strings.TrimSpace(action.Name)
	}
	observe := func() (int, error) {
		var index int
		if err := p.evaluateJSON(ctx, p.page.Locator("html"),
			`() => window.__aibKeyboardTargets.indexOf(document.activeElement)`, &index); err != nil {
			return -1, err
		}
		if index >= 0 {
			focus, err := p.ActiveElementDescription(ctx)
			if err != nil {
				return -1, err
			}
			results[index].Reached = true
			results[index].VisibleFocus = focus.Visible && focus.FocusVisible && focus.Unobscured
		}
		return index, nil
	}
	visitedGroups := make(map[int]bool)
	for range len(actions)*2 + 2 {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := p.page.Keyboard().Press("Tab"); err != nil {
			return nil, fmt.Errorf("tab through page actions: %w", err)
		}
		index, err := observe()
		if err != nil {
			return nil, err
		}
		if index < 0 || actions[index].Group < 0 || visitedGroups[actions[index].Group] {
			continue
		}
		group := actions[index].Group
		visitedGroups[group] = true
		if err := p.coverRovingGroup(ctx, group, observe); err != nil {
			return nil, err
		}
	}
	return results, nil
}

func (p *Page) coverRovingGroup(ctx context.Context, group int, observe func() (int, error)) (err error) {
	var state struct {
		Count    int    `json:"count"`
		Forward  string `json:"forward"`
		Backward string `json:"backward"`
		Selected string `json:"selected"`
	}
	script := fmt.Sprintf(`() => {
		const group = window.__aibKeyboardGroups[%d];
		const items = window.__aibKeyboardTargets.filter(el => group.contains(el));
		const selected = items.map(el => el.getAttribute('aria-selected') || el.getAttribute('aria-checked') || String(el.checked || false)).join(',');
		const vertical = group.getAttribute('aria-orientation') === 'vertical';
		const rtl = getComputedStyle(group).direction === 'rtl';
		return {count:items.length, selected,
			forward:vertical ? 'ArrowDown' : rtl ? 'ArrowLeft' : 'ArrowRight',
			backward:vertical ? 'ArrowUp' : rtl ? 'ArrowRight' : 'ArrowLeft'};
	}`, group)
	if err := p.evaluateJSON(ctx, p.page.Locator("html"), script, &state); err != nil {
		return err
	}
	pressRovingKey := func(key string) error {
		if err := p.page.Keyboard().Down(key); err != nil {
			return err
		}
		// Radix moves roving focus asynchronously; keep the key held through that render.
		_, renderErr := p.page.Evaluate(`() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)))`)
		return errors.Join(renderErr, p.page.Keyboard().Up(key))
	}
	steps := 0
	defer func() {
		for range steps {
			if restoreErr := pressRovingKey(state.Backward); restoreErr != nil {
				err = errors.Join(err, fmt.Errorf("restore roving selection: %w", restoreErr))
				break
			}
		}
		var restored struct {
			Selected string `json:"selected"`
		}
		if restoreErr := p.evaluateJSON(ctx, p.page.Locator("html"), script, &restored); restoreErr != nil {
			err = errors.Join(err, restoreErr)
		} else if restored.Selected != state.Selected {
			err = errors.Join(err, fmt.Errorf("roving group %d did not restore its original selection", group))
		}
		// Automatic tabs can remount their original panel when restored. Match
		// that unchanged action order rather than retaining detached DOM nodes.
		_, refreshErr := p.page.Evaluate(`async () => {
			await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
			const old = window.__aibKeyboardTargets;
			if (old.every(el => el.isConnected)) return;
			const fresh = window.__aibKeyboardFindTargets();
			const identity = window.__aibKeyboardIdentity;
			const identities = window.__aibKeyboardIdentities;
			if (fresh.length !== identities.length || fresh.some((el,i) => identity(el) !== identities[i])) {
				const changes = Array.from({length:Math.max(old.length, fresh.length)}, (_, index) => ({
					index, previous:identities[index] ?? null, current:fresh[index] ? identity(fresh[index]) : null
				})).filter(change => change.previous !== change.current).slice(0, 3);
				throw new Error('restoring a roving selection changed the action set on ' + location.pathname +
					' (' + old.length + ' to ' + fresh.length + '): ' + JSON.stringify(changes));
			}
			fresh.forEach((el,i) => window.__aibKeyboardBaselines.set(el, window.__aibKeyboardBaselines.get(old[i])));
			window.__aibKeyboardTargets = fresh;
			window.__aibKeyboardGroups = [...document.querySelectorAll('[role="tablist"],[role="radiogroup"]')];
		}`)
		if refreshErr != nil {
			err = errors.Join(err, fmt.Errorf("restore roving action references: %w", refreshErr))
		}
	}()
	for range max(0, state.Count-1) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := pressRovingKey(state.Forward); err != nil {
			return fmt.Errorf("traverse roving control: %w", err)
		}
		steps++
		if _, err := observe(); err != nil {
			return err
		}
	}
	return nil
}

// SetScreenshotContextFactory supplies the suite's authenticated context setup,
// including its clock and locale. Each call must return a new owned context.
func (p *Page) SetScreenshotContextFactory(factory func(context.Context, string) (playwright.BrowserContext, error)) {
	p.screenshotContextFactory = factory
}

// TakeThemedScreenshots captures each theme in a fresh context, never reloading
// or modifying the caller's page, its preferences, or its transient selections.
func (p *Page) TakeThemedScreenshots(ctx context.Context, stem string) error {
	if !captureScreenshotsEnabled() {
		return nil
	}
	if stem == "" || filepath.Base(stem) != stem {
		return fmt.Errorf("screenshot stem must be a nonempty filename")
	}
	if p.screenshotContextFactory == nil {
		return fmt.Errorf("themed screenshots require an authenticated screenshot context factory")
	}
	for _, theme := range []string{"light", "dark"} {
		if err := p.takeThemedScreenshot(ctx, stem, theme); err != nil {
			return err
		}
	}
	return ctx.Err()
}

func (p *Page) takeThemedScreenshot(ctx context.Context, stem, theme string) (err error) {
	timeout, err := p.locatorTimeout(ctx)
	if err != nil {
		return err
	}
	browserContext, err := p.screenshotContextFactory(ctx, theme)
	if err != nil {
		return fmt.Errorf("create %s screenshot context: %w", theme, err)
	}
	defer func() {
		if closeErr := browserContext.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close %s screenshot context: %w", theme, closeErr))
		}
	}()
	page, err := browserContext.NewPage()
	if err != nil {
		return fmt.Errorf("create screenshot page: %w", err)
	}
	if viewport := p.page.ViewportSize(); viewport != nil {
		if err := page.SetViewportSize(viewport.Width, viewport.Height); err != nil {
			return fmt.Errorf("copy screenshot viewport: %w", err)
		}
	}
	capture := NewPage(page, p.baseURL)
	if err := capture.SetThemePreference(ctx, theme); err != nil {
		return err
	}
	if _, err := page.Goto(p.page.URL(), playwright.PageGotoOptions{Timeout: timeout}); err != nil {
		return fmt.Errorf("open %s screenshot route: %w", theme, err)
	}
	handle, err := page.WaitForFunction(`theme =>
		document.documentElement.dataset.theme === theme &&
		!document.querySelector('[aria-busy="true"], [data-query-pending="true"]')`,
		theme, playwright.PageWaitForFunctionOptions{Timeout: timeout})
	if err != nil {
		return fmt.Errorf("wait for settled %s theme: %w", theme, err)
	}
	if err := handle.Dispose(); err != nil {
		return err
	}
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle, Timeout: timeout,
	}); err != nil {
		return fmt.Errorf("settle screenshot queries: %w", err)
	}
	if _, err := page.Evaluate(`async () => {
		await document.fonts.ready;
		await new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve)));
		window.scrollTo(0,0);
	}`); err != nil {
		return fmt.Errorf("settle screenshot fonts: %w", err)
	}
	path := filepath.Join(p.screenshotDir, stem+"_"+theme+".png")
	if err := os.MkdirAll(p.screenshotDir, 0o700); err != nil {
		return err
	}
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path: &path, FullPage: playwright.Bool(true), Timeout: timeout,
		Animations: playwright.ScreenshotAnimationsDisabled,
		Style:      playwright.String(screenshotDynamicStyle),
	}); err != nil {
		return fmt.Errorf("capture %s: %w", path, err)
	}
	return ctx.Err()
}
