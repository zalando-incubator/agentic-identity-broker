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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
	page          playwright.Page
	baseURL       string
	timeout       time.Duration
	screenshotDir string
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
	// Ensure path is absolute
	if path != "" && path[0] != '/' {
		path = "/" + path
	}

	url := p.baseURL + path

	// Set timeout for navigation
	_, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	// Navigate (Playwright automatically waits for page load)
	// Note: Playwright Go client handles timeouts internally, not via context
	_, err := p.page.Goto(url)
	if err != nil {
		return fmt.Errorf("failed to navigate to %s: %w", url, err)
	}

	// Wait for page to be fully loaded
	if err := p.page.WaitForLoadState(); err != nil {
		return fmt.Errorf("failed waiting for page load at %s: %w", url, err)
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
