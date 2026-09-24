package pages

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/mxschmitt/playwright-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeScreenshotPage struct {
	waitErr         error
	evaluateErr     error
	evaluateResult  string
	screenshotErr   error
	screenshotData  []byte
	waitCalls       int
	evaluateCalls   []string
	screenshotCalls int
}

func (f *fakeScreenshotPage) WaitForLoadState(options ...playwright.PageWaitForLoadStateOptions) error {
	f.waitCalls++
	return f.waitErr
}

func (f *fakeScreenshotPage) Evaluate(expression string, arg ...any) (any, error) {
	f.evaluateCalls = append(f.evaluateCalls, expression)
	return f.evaluateResult, f.evaluateErr
}

func (f *fakeScreenshotPage) Screenshot(options ...playwright.PageScreenshotOptions) ([]byte, error) {
	f.screenshotCalls++
	if f.screenshotData == nil {
		f.screenshotData = []byte("fake-image")
	}
	return f.screenshotData, f.screenshotErr
}

func TestCaptureScreenshot_AllowsNetworkIdleTimeoutFallback(t *testing.T) {
	t.Parallel()

	dir := filepath.Join(t.TempDir(), "screenshots")
	page := &fakeScreenshotPage{
		waitErr: errors.New("timeout:Timeout 10000.00ms exceeded."),
	}

	err := captureScreenshot(page, dir, "networkidle-timeout")
	require.NoError(t, err)

	assert.Equal(t, 1, page.waitCalls)
	assert.GreaterOrEqual(t, len(page.evaluateCalls), 1)
	assert.Equal(t, 1, page.screenshotCalls)

	data, readErr := os.ReadFile(filepath.Join(dir, "networkidle-timeout.png"))
	require.NoError(t, readErr)
	assert.Equal(t, []byte("fake-image"), data)

	fileInfo, statErr := os.Stat(filepath.Join(dir, "networkidle-timeout.png"))
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())

	dirInfo, statErr := os.Stat(dir)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
}

func TestCaptureScreenshot_RejectsUnavailableFont(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "unavailable-font.png")
	require.NoError(t, os.WriteFile(path, []byte("existing-image"), 0o600))
	page := &fakeScreenshotPage{evaluateResult: "Manrope"}

	err := captureScreenshot(page, dir, "unavailable-font")
	require.ErrorContains(t, err, "Manrope")
	assert.Equal(t, 0, page.screenshotCalls)
	data, readErr := os.ReadFile(path)
	require.NoError(t, readErr)
	assert.Equal(t, []byte("existing-image"), data)
}

func TestCaptureScreenshot_ReturnsErrorOnNonTimeoutLoadFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	page := &fakeScreenshotPage{
		waitErr: errors.New("navigation failed"),
	}

	err := captureScreenshot(page, dir, "load-failure")
	require.Error(t, err)
	assert.ErrorContains(t, err, "failed waiting for network idle before screenshot load-failure")
	assert.Equal(t, 0, page.screenshotCalls)
}
