package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/go-chi/chi/v5"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type MockSigningKeyRepository struct {
	mock.Mock
}

func (m *MockSigningKeyRepository) Create(ctx context.Context, key *storage.SigningKey) error {
	return m.Called(ctx, key).Error(0)
}

func (m *MockSigningKeyRepository) CreateAndSetCurrent(ctx context.Context, key *storage.SigningKey) error {
	return m.Called(ctx, key).Error(0)
}

func (m *MockSigningKeyRepository) GetByKIDInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) (*storage.SigningKey, error) {
	args := m.Called(ctx, domain, kid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyRepository) GetCurrentInDomain(ctx context.Context, domain storage.KeyDomain) (*storage.SigningKey, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyRepository) ListActiveInDomain(ctx context.Context, domain storage.KeyDomain) ([]*storage.SigningKey, error) {
	args := m.Called(ctx, domain)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyRepository) SetCurrentInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID, activatesAt time.Time) (*storage.SigningKey, error) {
	args := m.Called(ctx, domain, kid, activatesAt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyRepository) DeleteInDomain(ctx context.Context, domain storage.KeyDomain, kid id.KeyID) error {
	return m.Called(ctx, domain, kid).Error(0)
}

func (m *MockSigningKeyRepository) CountActiveInDomain(ctx context.Context, domain storage.KeyDomain) (int, error) {
	args := m.Called(ctx, domain)
	return args.Int(0), args.Error(1)
}

type MockSigningKeyManager struct {
	mock.Mock
}

func (m *MockSigningKeyManager) GenerateAndStoreKey(ctx context.Context, algorithm string, makeCurrent bool) (*storage.SigningKey, error) {
	args := m.Called(ctx, algorithm, makeCurrent)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyManager) BuildJWKS(ctx context.Context) (jwk.Set, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(jwk.Set), args.Error(1)
}

func (m *MockSigningKeyManager) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyManager) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	args := m.Called(ctx, kid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockSigningKeyManager) DeleteKey(ctx context.Context, kid id.KeyID) error {
	return m.Called(ctx, kid).Error(0)
}

func newTestSigningKeysHandler(manager *MockSigningKeyManager) *SigningKeysHandler {
	return newTestSigningKeysHandlerWithLogger(manager, slog.Default())
}

func newTestSigningKeysHandlerWithLogger(manager *MockSigningKeyManager, logger *slog.Logger) *SigningKeysHandler {
	return NewSigningKeysHandler(manager, logger)
}

func newSigningKeyRequest(method, target, kid string) *http.Request {
	req := httptest.NewRequest(method, target, http.NoBody)
	if kid == "" {
		return req
	}
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kid", kid)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func stubSigningKey() *storage.SigningKey {
	return &storage.SigningKey{
		KID:         id.NewKeyID("test-kid"),
		Algorithm:   "ES256",
		IsCurrent:   true,
		ActivatesAt: time.Now(),
		CreatedAt:   time.Now(),
	}
}

func withPrincipal(req *http.Request, principalValue string) *http.Request {
	return req.WithContext(principal.WithPrincipal(req.Context(), principalValue))
}

func TestSigningKeysHandler_Add(t *testing.T) {
	t.Run("empty body defaults to ES256", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("GenerateAndStoreKey", mock.Anything, "ES256", true).Return(stubSigningKey(), nil)

		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", http.NoBody), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		manager.AssertExpectations(t)
	})

	t.Run("valid JSON with ES256 creates key", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("GenerateAndStoreKey", mock.Anything, "ES256", true).Return(stubSigningKey(), nil)

		body := `{"algorithm":"ES256"}`
		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", strings.NewReader(body)), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		assert.NotContains(t, w.Body.String(), "private_key_encrypted")
		manager.AssertExpectations(t)
	})

	t.Run("omitted algorithm field defaults to ES256", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("GenerateAndStoreKey", mock.Anything, "ES256", true).Return(stubSigningKey(), nil)

		body := `{}`
		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", strings.NewReader(body)), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		manager.AssertExpectations(t)
	})

	t.Run("malformed JSON returns 400", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", strings.NewReader(`{bad json`)), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid request body", resp.Error)
		manager.AssertNotCalled(t, "GenerateAndStoreKey")
	})

	t.Run("unsupported algorithm returns 400", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		body := `{"algorithm":"RS256"}`
		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", strings.NewReader(body)), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusBadRequest, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "unsupported algorithm", resp.Error)
		manager.AssertNotCalled(t, "GenerateAndStoreKey")
	})

	t.Run("service error returns 500", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("GenerateAndStoreKey", mock.Anything, "ES256", true).
			Return(nil, storage.NewStorageError("test", storage.ErrorKindUnknown, nil, "db error"))

		body := `{"algorithm":"ES256"}`
		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", strings.NewReader(body)), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Add(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		manager.AssertExpectations(t)
	})

	t.Run("logs operator principal on success", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("GenerateAndStoreKey", mock.Anything, "ES256", true).Return(stubSigningKey(), nil)

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		req := withPrincipal(httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", http.NoBody), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).Add(w, req)

		require.Equal(t, http.StatusCreated, w.Code)
		assert.Contains(t, logBuf.String(), "operator_principal")
		assert.Contains(t, logBuf.String(), "operator@example.com")
		manager.AssertExpectations(t)
	})

	t.Run("returns 500 when operator principal is missing from context", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).Add(w, httptest.NewRequest(http.MethodPost, "/api/oauth2-server/signing-keys", http.NoBody))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "server_misconfiguration", resp.Error)
		assert.Equal(t, "operator principal required for mutating signing key operations", resp.Message)
		assert.Contains(t, logBuf.String(), "operator principal missing from context")
		manager.AssertNotCalled(t, "GenerateAndStoreKey")
	})
}

func TestSigningKeysHandler_List(t *testing.T) {
	t.Run("returns active signing keys", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		key := stubSigningKey()
		manager.On("ListKeys", mock.Anything).Return([]*storage.SigningKey{key}, nil)

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).List(w, httptest.NewRequest(http.MethodGet, "/api/oauth2-server/signing-keys", http.NoBody))

		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "private_key_encrypted")
		var resp struct {
			Items []signingKeyResponse `json:"items"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		require.Len(t, resp.Items, 1)
		assert.Equal(t, key.KID.String(), resp.Items[0].KID)
		assert.Equal(t, key.Algorithm, resp.Items[0].Algorithm)
		assert.Equal(t, key.IsCurrent, resp.Items[0].IsCurrent)
		manager.AssertExpectations(t)
	})

	t.Run("service error returns 500", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("ListKeys", mock.Anything).Return(nil, errors.New("list failed"))

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).List(w, httptest.NewRequest(http.MethodGet, "/api/oauth2-server/signing-keys", http.NoBody))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		manager.AssertExpectations(t)
	})

	t.Run("service error logs operator principal", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("ListKeys", mock.Anything).Return(nil, errors.New("list failed"))

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError}))
		req := withPrincipal(httptest.NewRequest(http.MethodGet, "/api/oauth2-server/signing-keys", http.NoBody), "operator@example.com")
		w := httptest.NewRecorder()

		newTestSigningKeysHandlerWithLogger(manager, logger).List(w, req)

		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, logBuf.String(), "operator_principal")
		assert.Contains(t, logBuf.String(), "operator@example.com")
		manager.AssertExpectations(t)
	})
}

func TestSigningKeysHandler_SetCurrent(t *testing.T) {
	t.Run("empty kid returns 400", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).SetCurrent(w, withPrincipal(newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys//current", ""), "operator@example.com"))

		require.Equal(t, http.StatusBadRequest, w.Code)
		manager.AssertNotCalled(t, "PromoteKey")
	})

	t.Run("unknown kid returns 404", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("PromoteKey", mock.Anything, id.NewKeyID("missing-kid")).
			Return(nil, storage.NewStorageError("test", storage.ErrorKindNotFound, nil, "signing key not found"))

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).SetCurrent(w, withPrincipal(newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys/missing-kid/current", "missing-kid"), "operator@example.com"))

		require.Equal(t, http.StatusNotFound, w.Code)
		manager.AssertExpectations(t)
	})

	t.Run("returns promoted key", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		key := stubSigningKey()
		key.ActivatesAt = time.Date(2026, time.January, 2, 3, 4, 5, 0, time.UTC)
		key.CreatedAt = time.Date(2026, time.January, 1, 2, 3, 4, 0, time.UTC)
		manager.On("PromoteKey", mock.Anything, id.NewKeyID("test-kid")).Return(key, nil)

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).SetCurrent(w, withPrincipal(newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys/test-kid/current", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusOK, w.Code)
		assert.NotContains(t, w.Body.String(), "private_key_encrypted")
		var resp signingKeyResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, key.KID.String(), resp.KID)
		assert.Equal(t, key.Algorithm, resp.Algorithm)
		assert.True(t, resp.IsCurrent)
		assert.Equal(t, key.ActivatesAt.Format(time.RFC3339), resp.ActivatesAt)
		assert.Equal(t, key.CreatedAt.Format(time.RFC3339), resp.CreatedAt)
		manager.AssertExpectations(t)
	})

	t.Run("promotion failure returns 500 and logs error", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("PromoteKey", mock.Anything, id.NewKeyID("test-kid")).Return(nil, errors.New("promote failed"))

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelError}))
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).SetCurrent(w, withPrincipal(newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys/test-kid/current", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		assert.Contains(t, logBuf.String(), "failed to set current key")
		assert.Contains(t, logBuf.String(), "test-kid")
		assert.Contains(t, logBuf.String(), "promote failed")
		manager.AssertExpectations(t)
	})

	t.Run("logs operator principal on success", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("PromoteKey", mock.Anything, id.NewKeyID("test-kid")).Return(stubSigningKey(), nil)

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelInfo}))
		req := withPrincipal(newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys/test-kid/current", "test-kid"), "operator@example.com")
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).SetCurrent(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, logBuf.String(), "operator_principal")
		assert.Contains(t, logBuf.String(), "operator@example.com")
		manager.AssertExpectations(t)
	})

	t.Run("returns 500 when operator principal is missing from context", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).SetCurrent(w, newSigningKeyRequest(http.MethodPut, "/api/oauth2-server/signing-keys/test-kid/current", "test-kid"))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "server_misconfiguration", resp.Error)
		assert.Equal(t, "operator principal required for mutating signing key operations", resp.Message)
		assert.Contains(t, logBuf.String(), "operator principal missing from context")
		manager.AssertNotCalled(t, "PromoteKey")
	})
}

func TestSigningKeysHandler_Remove(t *testing.T) {
	t.Run("empty kid returns 400", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/", ""), "operator@example.com"))

		require.Equal(t, http.StatusBadRequest, w.Code)
		manager.AssertNotCalled(t, "DeleteKey")
	})

	t.Run("unknown kid returns 404", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("DeleteKey", mock.Anything, id.NewKeyID("missing-kid")).
			Return(storage.NewStorageError("test", storage.ErrorKindNotFound, nil, "signing key not found"))

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/missing-kid", "missing-kid"), "operator@example.com"))

		require.Equal(t, http.StatusNotFound, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "signing key not found", resp.Error)
		manager.AssertExpectations(t)
	})

	t.Run("last active key conflict returns 409", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("DeleteKey", mock.Anything, id.NewKeyID("test-kid")).
			Return(ports.ErrLastActiveKey)

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/test-kid", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusConflict, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "last_key", resp.Error)
		assert.Equal(t, "cannot delete the last remaining signing key", resp.Message)
		manager.AssertExpectations(t)
	})

	t.Run("current key conflict returns 409", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("DeleteKey", mock.Anything, id.NewKeyID("test-kid")).
			Return(ports.ErrCurrentKey)

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/test-kid", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusConflict, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "current_key", resp.Error)
		assert.Equal(t, "promote another signing key before removing the current key", resp.Message)
		manager.AssertExpectations(t)
	})

	t.Run("currently signing fallback key conflict returns 409 with activation guidance", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("DeleteKey", mock.Anything, id.NewKeyID("test-kid")).
			Return(ports.ErrEffectiveCurrentKey)

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/test-kid", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusConflict, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "current_key", resp.Error)
		assert.Equal(t, "wait for the promoted signing key to activate or promote a different key before removing the key still signing tokens", resp.Message)
		manager.AssertExpectations(t)
	})

	t.Run("unexpected error returns 500", func(t *testing.T) {

		manager := &MockSigningKeyManager{}
		manager.On("DeleteKey", mock.Anything, id.NewKeyID("test-kid")).Return(errors.New("delete failed"))

		w := httptest.NewRecorder()
		newTestSigningKeysHandler(manager).Remove(w, withPrincipal(newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/test-kid", "test-kid"), "operator@example.com"))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "internal server error", resp.Error)
		manager.AssertExpectations(t)
	})

	t.Run("returns 500 when operator principal is missing from context", func(t *testing.T) {

		manager := &MockSigningKeyManager{}

		var logBuf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelWarn}))
		w := httptest.NewRecorder()
		newTestSigningKeysHandlerWithLogger(manager, logger).Remove(w, newSigningKeyRequest(http.MethodDelete, "/api/oauth2-server/signing-keys/test-kid", "test-kid"))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var resp ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "server_misconfiguration", resp.Error)
		assert.Equal(t, "operator principal required for mutating signing key operations", resp.Message)
		assert.Contains(t, logBuf.String(), "operator principal missing from context")
		manager.AssertNotCalled(t, "DeleteKey")
	})
}
