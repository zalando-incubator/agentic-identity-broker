package jwks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/trustanchor"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	testAnchor    = "token_exchange.client_assertion"
	testFilePath  = "/trust/gateway.json"
	otherKeySet   = `{"keys":[{"kty":"EC","crv":"P-256","x":"f83OJ3D2xF1Bg8vub9tLe1gHMzV76e8Tus9uPHvRVEU","y":"x_FEzRu9m36HLN_tue659LNpXW6pCyStikYjKIWI5a0","kid":"k2"}]}`
	contentMarker = "FILE-CONTENT-MARKER-55d1"
)

var fastBounds = trustanchor.RefreshBounds{Min: 20 * time.Millisecond, Max: 400 * time.Millisecond, Explicit: true}

// fakeReader serves a mutable document or error in place of the trust directory.
type fakeReader struct {
	mu    sync.Mutex
	doc   string
	err   error
	reads int
}

func (r *fakeReader) ReadFile(path string) ([]byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.reads++
	if path != testFilePath {
		return nil, errors.New("unexpected path " + path)
	}
	if r.err != nil {
		return nil, r.err
	}
	return []byte(r.doc), nil
}

func (r *fakeReader) set(doc string, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.doc, r.err = doc, err
}

func (r *fakeReader) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.reads
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *syncBuffer) records(t *testing.T, message string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(b.String()), "\n") {
		if line == "" {
			continue
		}
		var record map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &record))
		if record["msg"] == message {
			out = append(out, record)
		}
	}
	return out
}

func newTestFileAdapter(t *testing.T, reader *fakeReader, bounds trustanchor.RefreshBounds) (*FileAdapter, *syncBuffer) {
	t.Helper()
	logs := &syncBuffer{}
	adapter, err := newFileAdapter(reader, testFilePath, bounds, testAnchor, slog.New(slog.NewJSONHandler(logs, nil)))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, adapter.Shutdown(context.Background())) })
	return adapter, logs
}

func currentKIDs(t *testing.T, adapter *FileAdapter) []string {
	t.Helper()
	set, err := adapter.GetKeySet(context.Background())
	if err != nil {
		return nil
	}
	var kids []string
	for i := range set.Len() {
		key, _ := set.Key(i)
		kid, _ := key.KeyID()
		kids = append(kids, kid)
	}
	return kids
}

func TestFileAdapterStartupFailures(t *testing.T) {
	tests := []struct {
		name    string
		doc     string
		err     error
		message string
	}{
		{name: "missing file names the anchor", err: &trustanchor.Error{Category: trustanchor.CategoryNotFound,
			Detail: map[string]string{trustanchor.DetailPath: testFilePath}},
			message: testAnchor + `: jwks_file "/trust/gateway.json" does not exist`},
		{name: "invalid document names the setting", doc: `{"keys":["` + contentMarker,
			message: testAnchor + ".jwks_file: key set is not a valid JWK Set document"},
		{name: "empty set", doc: `{"keys":[]}`, message: testAnchor + ".jwks_file: key set contains no keys"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reader := &fakeReader{doc: tc.doc, err: tc.err}
			_, err := newFileAdapter(reader, testFilePath, fastBounds, testAnchor, slog.New(slog.DiscardHandler))
			require.Error(t, err)
			assert.Equal(t, tc.message, err.Error())
			assert.NotContains(t, err.Error(), contentMarker)
		})
	}
}

func TestFileAdapterRejectsANonPositiveRereadInterval(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	bounds := trustanchor.RefreshBounds{Min: -time.Second, Max: time.Hour, Explicit: true}

	_, err := newFileAdapter(reader, testFilePath, bounds, testAnchor, slog.New(slog.DiscardHandler))

	require.Error(t, err)
	assert.Equal(t, testAnchor+".jwks_min_refresh: must be a positive duration", err.Error())
	assert.Zero(t, reader.count(), "the interval is checked before any read")
}

func TestFileAdapterReplacesTheSetOnAValidReRead(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, logs := newTestFileAdapter(t, reader, fastBounds)
	assert.Equal(t, []string{"k1"}, currentKIDs(t, adapter))
	assert.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())

	readsBefore := reader.count()
	require.Eventually(t, func() bool { return reader.count() >= readsBefore+2 }, time.Second, 5*time.Millisecond)
	assert.Empty(t, logs.records(t, "trust anchor key set changed"), "unchanged content logs nothing")

	reader.set(otherKeySet, nil)
	require.Eventually(t, func() bool { return slices.Equal(currentKIDs(t, adapter), []string{"k2"}) }, time.Second, 5*time.Millisecond)

	changed := logs.records(t, "trust anchor key set changed")
	require.Len(t, changed, 1)
	assert.Equal(t, testAnchor, changed[0]["trust_anchor"])
	assert.Equal(t, "file", changed[0]["key_source"])
	assert.Equal(t, testFilePath, changed[0]["path"])
	assert.EqualValues(t, 1, changed[0]["key_count"])
	assert.Equal(t, []any{"k2"}, changed[0]["added_key_ids"])
	assert.Equal(t, []any{"k1"}, changed[0]["removed_key_ids"])
}

func TestFileAdapterKeepsTheLastValidSetAndRecovers(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, logs := newTestFileAdapter(t, reader, fastBounds)

	reader.set(`{"keys":["`+contentMarker, nil)
	require.Eventually(t, func() bool { return adapter.HealthState() == ports.ComponentHealthDegraded }, time.Second, 5*time.Millisecond)
	assert.Equal(t, []string{"k1"}, currentKIDs(t, adapter), "the last valid set keeps serving")

	failures := logs.records(t, "trust anchor key set re-read failed")
	require.NotEmpty(t, failures)
	assert.Equal(t, "WARN", failures[0]["level"])
	assert.Equal(t, testAnchor, failures[0]["trust_anchor"])
	assert.Equal(t, "not_a_jwk_set", failures[0]["error_kind"])
	assert.Contains(t, failures[0], "last_success_at")
	assert.Contains(t, failures[0], "fails_closed_at")
	assert.NotContains(t, logs.String(), contentMarker)

	reader.set(testKeySet, nil)
	require.Eventually(t, func() bool { return adapter.HealthState() == ports.ComponentHealthHealthy }, time.Second, 5*time.Millisecond)
	recovered := logs.records(t, "trust anchor key set recovered")
	require.Len(t, recovered, 1)
	assert.EqualValues(t, 1, recovered[0]["key_count"])
}

func TestFileAdapterFailsClosedAfterTheMaximumRefresh(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, logs := newTestFileAdapter(t, reader, fastBounds)

	reader.set("", &trustanchor.Error{Category: trustanchor.CategoryNotFound, Detail: map[string]string{trustanchor.DetailPath: testFilePath}})
	require.Eventually(t, func() bool {
		_, err := adapter.GetKeySet(context.Background())
		return err != nil
	}, 2*time.Second, 10*time.Millisecond)

	_, err := adapter.GetKeySet(context.Background())
	assert.Equal(t, trustanchor.CategoryStale, trustanchor.CategoryOf(err))
	_, err = adapter.GetKey(context.Background(), "k1")
	require.Error(t, err)
	assert.Equal(t, ports.ComponentHealthDegraded, adapter.HealthState())

	expired := logs.records(t, "trust anchor key set expired")
	require.Len(t, expired, 1, "the expiry is logged once")
	assert.Equal(t, "stale", expired[0]["error_kind"])
	assert.Equal(t, "not_found", logs.records(t, "trust anchor key set re-read failed")[0]["error_kind"])

	reader.set(otherKeySet, nil)
	require.Eventually(t, func() bool { return slices.Equal(currentKIDs(t, adapter), []string{"k2"}) }, time.Second, 5*time.Millisecond)
	assert.Equal(t, ports.ComponentHealthHealthy, adapter.HealthState())
}

func TestFileAdapterRecordsAFailedFetchSpanWhenStale(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, _ := newTestFileAdapter(t, reader, fastBounds)
	reader.set("", &trustanchor.Error{Category: trustanchor.CategoryNotFound, Detail: map[string]string{trustanchor.DetailPath: testFilePath}})
	require.Eventually(t, func() bool { _, err := adapter.GetKeySet(context.Background()); return err != nil }, 2*time.Second, 10*time.Millisecond)

	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	previous := otel.GetTracerProvider()
	otel.SetTracerProvider(provider)
	t.Cleanup(func() { otel.SetTracerProvider(previous); _ = provider.Shutdown(context.Background()) })

	_, err := adapter.GetKeySet(context.Background())
	require.Error(t, err)
	require.NoError(t, provider.ForceFlush(context.Background()))
	span := findSpanByName(recorder.Ended(), "jwks.fetch")
	require.NotNil(t, span)
	assert.Equal(t, codes.Error, span.Status().Code)
	assert.Empty(t, span.Events(), "key-set errors must not become exception events")
	assert.Contains(t, span.Attributes(), attribute.String("error.type", "jwks_unavailable"))
}

func TestFileAdapterNeverAppliesAPartiallyValidDocument(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, logs := newTestFileAdapter(t, reader, fastBounds)

	partial := `{"keys":[` + strings.TrimSuffix(strings.TrimPrefix(otherKeySet, `{"keys":[`), `]}`) +
		`,{"kty":"oct","k":"c2VjcmV0","kid":"shared"}]}`
	reader.set(partial, nil)
	require.Eventually(t, func() bool { return len(logs.records(t, "trust anchor key set re-read failed")) > 0 }, time.Second, 5*time.Millisecond)
	assert.Equal(t, "symmetric_key", logs.records(t, "trust anchor key set re-read failed")[0]["error_kind"])
	assert.Equal(t, []string{"k1"}, currentKIDs(t, adapter))
}

func TestFileAdapterShutdownStopsTheLoop(t *testing.T) {
	reader := &fakeReader{doc: testKeySet}
	adapter, err := newFileAdapter(reader, testFilePath, fastBounds, testAnchor, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	require.NoError(t, adapter.Shutdown(context.Background()))
	require.NoError(t, adapter.Shutdown(context.Background()), "Shutdown is idempotent")

	reads := reader.count()
	time.Sleep(5 * fastBounds.Min)
	assert.Equal(t, reads, reader.count())
}

func TestFileAdapterReadsThroughTheTrustDirectory(t *testing.T) {
	layout := newTrustLayout(t)
	trust := openTestTrustDirectory(t, layout.trust)
	path := layout.kubernetes(t, "jwks.json", testKeySet)

	adapter, err := NewFileAdapter(trust, path, fastBounds, testAnchor, slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapter.Shutdown(context.Background()) })
	assert.Equal(t, []string{"k1"}, adapter.LocalKeySet().KeyIDs)
}
