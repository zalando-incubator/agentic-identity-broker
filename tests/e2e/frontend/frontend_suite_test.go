// Package e2e_test provides end-to-end tests for the frontend UI using Playwright.
// This file defines the test suite entry point for frontend E2E tests.
// Individual test files (e.g., consent_flow_test.go) import from this package
// and use the exported test resources and helper functions.
package e2e_test

import (
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// SuiteContext holds resources shared across all tests in the suite.
// These are initialized on each worker after the shared build completes.
// SuiteContext remains immutable after setup.
type SuiteContext struct {
	// Logger is the structured logger for the entire test suite
	Logger *slog.Logger

	// Browser is the Playwright browser instance (Chromium)
	Browser playwright.Browser

	// Playwright owns the driver process backing Browser.
	Playwright *playwright.Playwright

	// FrontendMode is either "built" (production build) or "dev" (Vite dev server)
	FrontendMode string

	// Headless controls whether the browser is shown (false) or hidden (true)
	Headless bool

	// DevFrontendURL is the Vite dev server URL (only set in dev mode)
	// This is "http://localhost:3000" when FrontendMode is "dev"
	DevFrontendURL string
}

// TestContext holds resources fresh for each individual test.
// These are created in BeforeEach and cleaned in AfterEach.
// TestContext is never shared between tests - each test gets a fresh instance.
type TestContext struct {
	// Suite provides read-only access to suite-level resources
	Suite *SuiteContext

	// Storage is the in-memory storage adapter for this test
	Storage *storageadapter.Adapter

	// StorageFactory creates fresh test storage instances
	StorageFactory *bootstrap.StorageFactory

	// MockUpstream is a mock OAuth2 server for this test
	MockUpstream *helpers.MockUpstreamOAuth2Server

	// Server is the HTTP test server for this test
	Server *bootstrap.TestServer

	// ServerFactory builds app instances with dependency injection
	ServerFactory *bootstrap.ServerFactory

	// BrowserContext is the Playwright browser context for this test (isolates page state)
	BrowserContext playwright.BrowserContext

	// Page is the Playwright page for this test
	Page playwright.Page

	// FrontendURL is the frontend URL for this test (varies by mode and test server port)
	// In dev mode: http://localhost:3000
	// In built mode: http://127.0.0.1:RANDOM_PORT
	FrontendURL string
}

// suiteCtx holds suite-level resources initialized in BeforeSuite
// This is shared by all tests but should be treated as immutable after initialization
var suiteCtx *SuiteContext

// testCtx holds test-level resources created in BeforeEach and cleaned in AfterEach
// This is fresh for each test to ensure complete isolation
var testCtx *TestContext

// TestFrontendE2E is the entry point for the frontend E2E test suite.
// Ginkgo calls this to register and run all tests in this package.
func TestFrontendE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Frontend E2E Tests")
}

// SynchronizedBeforeSuite builds the shared bundle once, then starts a browser on every worker.
var _ = SynchronizedBeforeSuite(func() []byte {
	mode := os.Getenv("E2E_FRONTEND_MODE")
	if mode == "" {
		mode = "built"
	}
	if mode != "built" && mode != "dev" {
		Fail(fmt.Sprintf("Invalid E2E_FRONTEND_MODE=%q. Must be 'built' or 'dev'", mode))
	}
	if mode == "built" {
		Expect(ensureFrontendBuilt(bootstrap.TestLogger(slog.LevelInfo))).To(Succeed(), "Failed to build frontend")
	}
	return []byte(mode)
}, func(mode []byte) {
	SetDefaultEventuallyTimeout(30 * time.Second)
	// Step 1: Initialize structured logger
	logger := bootstrap.TestLogger(slog.LevelInfo)

	// Verify working directory for relative paths
	// This is important for the SPA handler to find web/dist/index.html
	cwd, err := os.Getwd()
	Expect(err).NotTo(HaveOccurred(), "Failed to get current working directory")
	logger.Info("Test suite starting from directory", "cwd", cwd)

	logger.Info("Starting frontend E2E test suite initialization")

	// Step 2: Use the mode validated on process 1.
	frontendMode := string(mode)

	headlessStr := os.Getenv("HEADLESS")
	headless := headlessStr == "" || headlessStr == "true"

	logger.Info("Frontend E2E mode",
		"mode", frontendMode,
		"headless", headless,
	)

	// Step 3: Initialize Playwright
	pw, err := playwright.Run()
	Expect(err).NotTo(HaveOccurred(), "Failed to start Playwright")

	// Step 4: Launch browser
	browserInstance, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(headless),
	})
	if err != nil {
		_ = pw.Stop()
	}
	Expect(err).NotTo(HaveOccurred(), "Failed to launch Chromium browser")

	logger.Info("Playwright browser launched", "headless", headless)

	// Handle frontend URL based on mode.
	var devFrontendURL string
	if frontendMode == "dev" {
		// Dev mode: Vite dev server on port 3000 with automatic X-Remote-User injection
		devFrontendURL = "http://localhost:3000"
		logger.Info("Frontend URL (dev mode - requires 'just web-dev')", "url", devFrontendURL)

		// Verify the Vite dev server is accessible before running specs.
		// Check the root path where the consent app is served.
		verifyURL := devFrontendURL + "/"
		logger.Info("Verifying Vite dev server accessibility", "url", verifyURL)
		err = verifyFrontendAccessible(logger, verifyURL, 30*time.Second)
		Expect(err).NotTo(HaveOccurred(), "Vite dev server not accessible at "+verifyURL)
	} else {
		// Built mode: Frontend served by per-test Go backend (URL determined in BeforeEach)
		logger.Info("Frontend mode: built (served by per-test Go backend on random port)")
	}

	// Store suite context
	suiteCtx = &SuiteContext{
		Logger:         logger,
		Playwright:     pw,
		Browser:        browserInstance,
		FrontendMode:   frontendMode,
		Headless:       headless,
		DevFrontendURL: devFrontendURL,
	}

	logger.Info("Frontend E2E suite initialization complete")
})

// BeforeEach creates fresh per-test resources for each individual test.
// This runs before each test to ensure complete isolation.
//
// Setup steps:
// 1. Create fresh test storage
// 2. Create fresh mock upstream server
// 3. Create fresh app instance with test config
// 4. Create fresh test server
// 5. Create fresh Playwright context and page
var _ = BeforeEach(func() {
	var err error

	// Step 1: Create fresh test storage
	storageFactory := bootstrap.NewStorageFactory(suiteCtx.Logger)
	storage, err := storageFactory.NewTestStorage()
	Expect(err).NotTo(HaveOccurred(), "Failed to create test storage")

	// Step 2: Create fresh mock upstream server
	mockUpstream := helpers.NewMockUpstreamOAuth2Server()

	// Step 3: Create fresh app instance with test config
	config := fixtures.OAuth2ConfigWithUpstream(mockUpstream.Server.URL)
	serverFactory := bootstrap.NewServerFactory(config, suiteCtx.Logger)
	appInstance, err := serverFactory.BuildApp(storage)
	Expect(err).NotTo(HaveOccurred(), "Failed to build app instance")

	// Step 4: Create fresh test server
	// Frontend tests use end-user routes (OAuth2, consent UI)
	server, err := bootstrap.NewEndUserTestServer(appInstance, suiteCtx.Logger)
	Expect(err).NotTo(HaveOccurred(), "Failed to create test server")

	// Step 5: Determine frontend URL based on mode
	var frontendURL string
	if suiteCtx.FrontendMode == "dev" {
		// Dev mode: Use Vite dev server (Vite injects X-Remote-User header automatically)
		frontendURL = suiteCtx.DevFrontendURL
	} else {
		// Built mode: Use test server's actual URL
		// The SPA is mounted at root (/*) to match production routing
		frontendURL = server.BaseURL()
	}

	// Step 6: Create an isolated context with the same rendering settings in both modes.
	contextOpts := playwright.BrowserNewContextOptions{
		Viewport:          &playwright.Size{Width: 1280, Height: 720},
		Screen:            &playwright.Size{Width: 1280, Height: 720},
		DeviceScaleFactor: playwright.Float(1),
		Locale:            playwright.String("en-US"),
		TimezoneId:        playwright.String("UTC"),
		ColorScheme:       playwright.ColorSchemeLight,
		ReducedMotion:     playwright.ReducedMotionNoPreference,
		ForcedColors:      playwright.ForcedColorsNone,
	}
	if suiteCtx.FrontendMode == "built" {
		contextOpts.ExtraHttpHeaders = map[string]string{
			"X-Remote-User": fixtures.DefaultPrincipal().String(),
		}
	}
	browserContext, err := suiteCtx.Browser.NewContext(contextOpts)
	Expect(err).NotTo(HaveOccurred(), "Failed to create Playwright context")

	page, err := browserContext.NewPage()
	Expect(err).NotTo(HaveOccurred(), "Failed to create Playwright page")

	// Match the shared page-object timeout to reduce CI flakiness on slower workers.
	page.SetDefaultTimeout(30 * 1000)

	// Store in test context
	testCtx = &TestContext{
		Suite:          suiteCtx,
		Storage:        storage,
		StorageFactory: storageFactory,
		MockUpstream:   mockUpstream,
		Server:         server,
		ServerFactory:  serverFactory,
		BrowserContext: browserContext,
		Page:           page,
		FrontendURL:    frontendURL,
	}

	suiteCtx.Logger.Info("Test setup complete",
		"server_url", server.BaseURL(),
		"frontend_url", frontendURL,
		"frontend_mode", suiteCtx.FrontendMode,
	)
})

// AfterEach releases the per-test browser context, server, storage, and upstream.
var _ = AfterEach(func() {
	if testCtx == nil {
		return
	}
	if testCtx.BrowserContext != nil {
		if err := testCtx.BrowserContext.Close(); err != nil {
			suiteCtx.Logger.Error("Failed to close Playwright context", "error", err)
		}
	}
	if testCtx.Server != nil {
		testCtx.Server.Close()
	}
	if testCtx.Storage != nil && testCtx.StorageFactory != nil {
		if err := testCtx.StorageFactory.CloseStorage(testCtx.Storage); err != nil {
			suiteCtx.Logger.Error("Failed to close storage", "error", err)
		}
	}
	if testCtx.MockUpstream != nil {
		testCtx.MockUpstream.Close()
	}
	suiteCtx.Logger.Info("Test cleanup complete")
	testCtx = nil
})

// AfterSuite closes each worker's browser and Playwright process.
var _ = AfterSuite(func() {
	if suiteCtx == nil {
		return
	}
	if suiteCtx.Browser != nil {
		if err := suiteCtx.Browser.Close(); err != nil {
			suiteCtx.Logger.Error("Failed to close Playwright browser", "error", err)
		}
	}
	if suiteCtx.Playwright != nil {
		if err := suiteCtx.Playwright.Stop(); err != nil {
			suiteCtx.Logger.Error("Failed to stop Playwright", "error", err)
		}
	}
	suiteCtx.Logger.Info("Frontend E2E suite completed")
})

// Helper Functions

// ensureFrontendBuilt checks for dist/index.html and builds it locally if absent.
// CI must provide the artifact before the suite starts.
//
// Parameters:
//   - logger: Structured logger for informational output
//
// Returns error if:
// - npm install fails
// - npm run build fails
// - Build output is not found after build completes
func ensureFrontendBuilt(logger *slog.Logger) error {
	webDir := filepath.Join(getProjectRoot(), "web")
	indexPath := filepath.Join(webDir, "dist", "index.html")

	if _, err := os.Stat(indexPath); err == nil {
		logger.Info("Frontend already built", "file", indexPath)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to check frontend build at %s: %w", indexPath, err)
	}
	if os.Getenv("CI") != "" {
		return fmt.Errorf("frontend build missing %s; run just web-build before CI tests", indexPath)
	}

	logger.Info("Frontend not built, building now", "dir", webDir)
	for _, args := range [][]string{{"install"}, {"run", "build"}} {
		cmd := exec.Command("npm", args...)
		cmd.Dir = webDir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("npm %v failed: %w", args, err)
		}
	}
	if _, err := os.Stat(indexPath); err != nil {
		return fmt.Errorf("frontend build completed but %s is missing: %w", indexPath, err)
	}
	logger.Info("Frontend built successfully", "file", indexPath)
	return nil
}

// verifyFrontendAccessible polls the dev server URL until it responds with 200 OK.
// This ensures the frontend is ready before starting tests.
//
// Parameters:
//   - logger: Structured logger for diagnostic output
//   - url: Frontend URL to verify (e.g., "http://localhost:8000")
//   - timeout: Maximum time to wait for frontend to become accessible
//
// Returns error if frontend is not accessible within the timeout.
func verifyFrontendAccessible(logger *slog.Logger, url string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)

	for {
		// Try HTTP GET request
		resp, err := helpers.HTTPClient().Get(url)
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			logger.Info("Frontend verified accessible", "url", url)
			return nil
		}

		if resp != nil {
			_ = resp.Body.Close()
		}

		// Check if deadline exceeded
		if time.Now().After(deadline) {
			return fmt.Errorf("frontend not accessible after %v (url=%s)", timeout, url)
		}

		// Wait before retrying
		time.Sleep(500 * time.Millisecond)
	}
}

// getProjectRoot returns the root directory of the project.
// Used to locate the web/ directory for frontend build commands.
func getProjectRoot() string {
	// Get the directory of this file
	_, file, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(file)

	// Navigate up to project root
	// Path: frontend_suite_test.go -> frontend/ -> e2e/ -> tests/ -> .
	// That's 4 directory levels up
	return filepath.Join(testDir, "..", "..", "..")
}

// Test Resource Getters
// These functions provide convenient access to test resources.
// Tests should prefer accessing resources through testCtx and suiteCtx directly.

// GetTestContext returns the Playwright context for the current test.
// This context isolates page state between tests (separate cookies, storage, etc.).
func GetTestContext() playwright.BrowserContext {
	if testCtx == nil {
		return nil
	}
	return testCtx.BrowserContext
}

// GetTestPage returns the Playwright page for the current test.
// This page is fresh for each test and should be used for UI interactions.
func GetTestPage() playwright.Page {
	if testCtx == nil {
		return nil
	}
	return testCtx.Page
}

// GetTestServer returns the test server for the current test.
// This server can be used to make authenticated API requests via
// AuthenticatedGET, AuthenticatedPOST, PublicGET, etc.
func GetTestServer() *bootstrap.TestServer {
	if testCtx == nil {
		return nil
	}
	return testCtx.Server
}

// GetTestStorage returns the test storage adapter for the current test.
// This storage can be used to set up test data (agents, principals, grants, etc.).
func GetTestStorage() *storageadapter.Adapter {
	if testCtx == nil {
		return nil
	}
	return testCtx.Storage
}

// GetFrontendURL returns the frontend base URL for the current test.
// This URL depends on E2E_FRONTEND_MODE:
// - "built" mode: URL of per-test Go backend server (random port)
// - "dev" mode: http://localhost:3000 (Vite dev server)
//
// In built mode, each test gets a fresh test server on a random port.
// In dev mode, all tests share the same Vite dev server.
func GetFrontendURL() string {
	if testCtx == nil {
		return ""
	}
	return testCtx.FrontendURL
}

// GetMockUpstream returns the mock upstream OAuth2 server for the current test.
// This server can be used to verify OAuth2 requests and simulate OAuth2 flows.
func GetMockUpstream() *helpers.MockUpstreamOAuth2Server {
	if testCtx == nil {
		return nil
	}
	return testCtx.MockUpstream
}

// GetLogger returns the logger for the test suite.
// Use this for debugging and diagnostic logging during test execution.
func GetLogger() *slog.Logger {
	if suiteCtx == nil {
		return nil
	}
	return suiteCtx.Logger
}
