package enduser

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type mockSuccessPublisher struct {
	set jwk.Set
}

func (m *mockSuccessPublisher) PublishJWKS(_ context.Context) (jwk.Set, error) {
	return m.set, nil
}

func newTestJWKSHandler(t *testing.T) *JWKSHandler {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	publicKey, err := jwk.Import[jwk.Key](privateKey.Public())
	require.NoError(t, err)
	require.NoError(t, publicKey.Set(jwk.KeyIDKey, "test-kid"))
	require.NoError(t, publicKey.Set(jwk.KeyUsageKey, "sig"))
	require.NoError(t, publicKey.Set(jwk.AlgorithmKey, jwa.ES256()))

	set := jwk.NewSet()
	require.NoError(t, set.AddKey(publicKey))

	logger := slog.New(slog.DiscardHandler)
	return NewJWKSHandler(&mockSuccessPublisher{set: set}, logger)
}

func TestJWKSHandler_ServeJWKS(t *testing.T) {
	t.Run("returns JWK Set with active public keys", func(t *testing.T) {
		handler := newTestJWKSHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var body map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)

		keys, ok := body["keys"].([]interface{})
		require.True(t, ok, "response should contain 'keys' array")
		require.Len(t, keys, 1, "should have exactly one key")

		key := keys[0].(map[string]interface{})
		assert.NotEmpty(t, key["kid"], "key should have a kid")
		assert.Equal(t, "sig", key["use"], "key usage should be 'sig'")
	})

	t.Run("sets Cache-Control header", func(t *testing.T) {
		handler := newTestJWKSHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		assert.Equal(t, "public, max-age=300", rec.Header().Get("Cache-Control"))
	})

	t.Run("no private key material in response", func(t *testing.T) {
		handler := newTestJWKSHandler(t)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		// Parse the response body
		var body map[string]interface{}
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)

		keys, ok := body["keys"].([]interface{})
		require.True(t, ok)
		require.NotEmpty(t, keys)

		// Verify no private key exponent "d" field in any key
		for _, rawKey := range keys {
			key := rawKey.(map[string]interface{})
			_, hasD := key["d"]
			assert.False(t, hasD, "JWKS response must not contain private key exponent 'd'")
		}

		// Also verify via raw body that no "d" JSON key (private exponent) is present
		rawBody := rec.Body.String()
		assert.False(t, strings.Contains(rawBody, `"d":`), "raw response must not contain '\"d\":' private key field")
	})

	t.Run("returns 503 when upstream unavailable", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		handler := NewJWKSHandler(&mockErrorPublisher{err: ports.ErrUpstreamUnavailable}, logger)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var body map[string]string
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "upstream key material temporarily unavailable", body["error"])
	})

	t.Run("returns 503 when kid conflict detected", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		handler := NewJWKSHandler(&mockErrorPublisher{err: ports.ErrKidConflict}, logger)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var body map[string]string
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "JWKS configuration conflict requires operator action", body["error"])
	})

	t.Run("returns 500 when publisher returns generic error", func(t *testing.T) {
		logger := slog.New(slog.DiscardHandler)
		handler := NewJWKSHandler(&mockErrorPublisher{err: errors.New("storage error")}, logger)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := httptest.NewRecorder()

		handler.ServeJWKS(rec, req)

		assert.Equal(t, http.StatusInternalServerError, rec.Code)
		assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

		var body map[string]string
		err := json.NewDecoder(rec.Body).Decode(&body)
		require.NoError(t, err)
		assert.Equal(t, "internal server error", body["error"])
	})

	t.Run("logs warn when error response encoding fails", func(t *testing.T) {
		var logs bytes.Buffer
		logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelWarn}))
		handler := NewJWKSHandler(&mockErrorPublisher{err: ports.ErrUpstreamUnavailable}, logger)

		req := httptest.NewRequest(http.MethodGet, "/oauth2/jwks.json", nil)
		rec := &failingResponseWriter{}

		handler.ServeJWKS(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.status)
		assert.Contains(t, logs.String(), "level=WARN")
		assert.Contains(t, logs.String(), "failed to encode error response")
	})
}

type mockErrorPublisher struct {
	err error
}

func (m *mockErrorPublisher) PublishJWKS(_ context.Context) (jwk.Set, error) {
	return nil, m.err
}

type failingResponseWriter struct {
	header http.Header
	status int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) WriteHeader(status int) {
	w.status = status
}

func (w *failingResponseWriter) Write(_ []byte) (int, error) {
	return 0, assert.AnError
}

var _ ports.JWKSPublisherPort = (*mockSuccessPublisher)(nil)
var _ ports.JWKSPublisherPort = (*mockErrorPublisher)(nil)
