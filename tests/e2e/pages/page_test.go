package pages

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

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

func TestRequestRecorderRejectsLateEventsAfterRestart(t *testing.T) {
	t.Parallel()
	var log requestLog
	first := log.start()
	log.record(first, RecordedRequest{URL: "/old", Method: "POST"})
	log.stop()
	log.record(first, RecordedRequest{URL: "/late", Method: "DELETE"})
	require.Equal(t, []RecordedRequest{{URL: "/old", Method: "POST"}}, log.snapshot())

	second := log.start()
	log.record(first, RecordedRequest{URL: "/stale", Method: "PUT"})
	log.record(second, RecordedRequest{URL: "/current", Method: "GET"})
	assert.Equal(t, []RecordedRequest{{URL: "/current", Method: "GET"}}, log.snapshot())
}

func TestRequestRecorderSnapshotsRemainIndependentDuringRecording(t *testing.T) {
	t.Parallel()
	var log requestLog
	generation := log.start()
	log.record(generation, RecordedRequest{URL: "/document", Method: "GET"})
	snapshot := log.snapshot()
	snapshot[0].Method = "DELETE"

	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for range 50 {
				log.record(generation, RecordedRequest{URL: "/font.woff2", Method: "GET"})
				_ = log.snapshot()
			}
		})
	}
	workers.Wait()
	log.stop()
	recorded := log.snapshot()
	require.Len(t, recorded, 401, "concurrent callbacks must not lose subresource requests")
	assert.Equal(t, "GET", recorded[0].Method, "callers must not mutate the recorder through a snapshot")
	assert.Len(t, snapshot, 1, "a snapshot must not acquire subsequent events")
}

func TestZoomViewportExercisesLayoutReflow(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		percent int
		width   int
		height  int
	}{
		{100, 1280, 1080},
		{200, 640, 540},
		{400, 320, 270},
	} {
		width, height, err := zoomViewport(test.percent)
		require.NoError(t, err)
		assert.Equal(t, test.width, width)
		assert.Equal(t, test.height, height)
	}
	for _, percent := range []int{0, -100, 108001} {
		_, _, err := zoomViewport(percent)
		require.Error(t, err, "invalid zoom must not disable or empty the viewport")
	}
}

func TestLocatorDeadlineNeverDisablesPlaywrightTimeout(t *testing.T) {
	t.Parallel()
	page := &Page{timeout: 30 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	timeout, err := page.locatorTimeout(ctx)
	require.NoError(t, err)
	require.NotNil(t, timeout)
	assert.GreaterOrEqual(t, *timeout, float64(1))
	assert.LessOrEqual(t, *timeout, float64(1000))

	cancel()
	_, err = page.locatorTimeout(ctx)
	require.ErrorIs(t, err, context.Canceled)
}
