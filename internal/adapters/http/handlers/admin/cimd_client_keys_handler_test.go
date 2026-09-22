package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const cimdKeySensitiveMaterial = "private-pem-ciphertext-branch-key-id-client-assertion"

type MockCIMDClientKeyLifecycle struct {
	mock.Mock
}

func (m *MockCIMDClientKeyLifecycle) GenerateKey(ctx context.Context, algorithm string) (*storage.SigningKey, error) {
	args := m.Called(ctx, algorithm)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockCIMDClientKeyLifecycle) ListKeys(ctx context.Context) ([]*storage.SigningKey, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*storage.SigningKey), args.Error(1)
}

func (m *MockCIMDClientKeyLifecycle) PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error) {
	args := m.Called(ctx, kid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*storage.SigningKey), args.Error(1)
}

func (m *MockCIMDClientKeyLifecycle) DeleteKey(ctx context.Context, kid id.KeyID) error {
	return m.Called(ctx, kid).Error(0)
}

var _ ports.CIMDClientKeyLifecycle = (*MockCIMDClientKeyLifecycle)(nil)

type cimdClientKeyResponse struct {
	KID         string `json:"kid"`
	Algorithm   string `json:"algorithm"`
	IsCurrent   bool   `json:"is_current"`
	ActivatesAt string `json:"activates_at"`
	CreatedAt   string `json:"created_at"`
}

type cimdClientKeyListResponse struct {
	Items []cimdClientKeyResponse `json:"items"`
}

func newTestCIMDClientKeysHandler(keys *MockCIMDClientKeyLifecycle, logger *slog.Logger) *CIMDClientKeysHandler {
	return NewCIMDClientKeysHandler(keys, logger)
}

func newCIMDClientKeyRequest(method, target, kid, body string) *http.Request {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if kid == "" {
		return req
	}

	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("kid", kid)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

func withCIMDKeyPrincipal(req *http.Request, value string) *http.Request {
	return req.WithContext(principal.WithPrincipal(req.Context(), value))
}

func cimdClientKeyForTest(kid string, isCurrent bool, activatesAt, createdAt time.Time) *storage.SigningKey {
	return &storage.SigningKey{
		ID:                  id.NewSigningKeyID(),
		KID:                 id.NewKeyID(kid),
		KeyDomain:           storage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte(cimdKeySensitiveMaterial),
		IsCurrent:           isCurrent,
		ActivatesAt:         activatesAt,
		CreatedAt:           createdAt,
	}
}

func assertCIMDKeyDisclosureFree(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{
		cimdKeySensitiveMaterial,
		"private_key_encrypted",
		"private_key",
		"ciphertext",
		"branch_key",
		"client_assertion",
	} {
		assert.NotContains(t, value, forbidden)
	}
}

func assertCIMDKeyLifecycleAudit(t *testing.T, audit string, operator, keyID, operation, outcome string) {
	t.Helper()
	assert.Contains(t, audit, `"operator_principal":"`+operator+`"`)
	assert.Contains(t, audit, `"key_id":"`+keyID+`"`)
	assert.Contains(t, audit, `"operation":"`+operation+`"`)
	assert.Contains(t, audit, `"outcome":"`+outcome+`"`)
	assertCIMDKeyDisclosureFree(t, audit)
}

func TestCIMDClientKeysHandler_Create(t *testing.T) {
	t.Run("creates the first ES256 key as immediately current without private material", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		activation := time.Date(2026, time.September, 22, 9, 0, 0, 0, time.UTC)
		firstKey := cimdClientKeyForTest("first-cimd-key", true, activation, activation.Add(time.Second))
		keys.On("GenerateKey", mock.Anything, "ES256").Return(firstKey, nil)

		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Create(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodPost, "/api/cimd-client-keys", "", `{}`), "operator@example.com"))

		require.Equal(t, http.StatusCreated, w.Code)
		require.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assertCIMDKeyDisclosureFree(t, w.Body.String())

		var response cimdClientKeyResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, firstKey.KID.String(), response.KID)
		assert.Equal(t, "ES256", response.Algorithm)
		assert.True(t, response.IsCurrent)
		assert.Equal(t, firstKey.ActivatesAt.Format(time.RFC3339), response.ActivatesAt)
		assert.Equal(t, firstKey.CreatedAt.Format(time.RFC3339), response.CreatedAt)

		returnedActivation, err := time.Parse(time.RFC3339, response.ActivatesAt)
		require.NoError(t, err)
		returnedCreation, err := time.Parse(time.RFC3339, response.CreatedAt)
		require.NoError(t, err)
		assert.False(t, returnedActivation.After(returnedCreation), "the first CIMD key must be usable immediately")
		assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", firstKey.KID.String(), "generate", "success")
		keys.AssertExpectations(t)
	})

	t.Run("rejects non-ES256 generation before invoking the lifecycle port", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Create(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(
			http.MethodPost,
			"/api/cimd-client-keys",
			"",
			`{"algorithm":"RS256","private_key":"`+cimdKeySensitiveMaterial+`"}`,
		), "operator@example.com"))

		require.Equal(t, http.StatusBadRequest, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "unsupported algorithm", response.Error)
		assertCIMDKeyDisclosureFree(t, w.Body.String())
		assert.Contains(t, audit.String(), `"operator_principal":"operator@example.com"`)
		assert.Contains(t, audit.String(), `"operation":"generate"`)
		assert.Contains(t, audit.String(), `"outcome":"rejected"`)
		assertCIMDKeyDisclosureFree(t, audit.String())
		keys.AssertNotCalled(t, "GenerateKey", mock.Anything, mock.Anything)
	})

	t.Run("requires an authenticated operator", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		w := httptest.NewRecorder()
		newTestCIMDClientKeysHandler(keys, slog.Default()).Create(w, newCIMDClientKeyRequest(http.MethodPost, "/api/cimd-client-keys", "", ``))

		require.Equal(t, http.StatusInternalServerError, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "server_misconfiguration", response.Error)
		assert.Equal(t, "operator principal required for mutating CIMD key operations", response.Message)
		keys.AssertNotCalled(t, "GenerateKey", mock.Anything, mock.Anything)
	})
}

func TestCIMDClientKeysHandler_List(t *testing.T) {
	keys := &MockCIMDClientKeyLifecycle{}
	createdAt := time.Date(2026, time.September, 21, 9, 0, 0, 0, time.UTC)
	key := cimdClientKeyForTest("listed-cimd-key", true, createdAt, createdAt)
	keys.On("ListKeys", mock.Anything).Return([]*storage.SigningKey{key}, nil)

	w := httptest.NewRecorder()
	newTestCIMDClientKeysHandler(keys, slog.Default()).List(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodGet, "/api/cimd-client-keys", "", ``), "operator@example.com"))

	require.Equal(t, http.StatusOK, w.Code)
	require.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assertCIMDKeyDisclosureFree(t, w.Body.String())

	var response cimdClientKeyListResponse
	require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
	require.Equal(t, []cimdClientKeyResponse{{
		KID:         key.KID.String(),
		Algorithm:   "ES256",
		IsCurrent:   true,
		ActivatesAt: key.ActivatesAt.Format(time.RFC3339),
		CreatedAt:   key.CreatedAt.Format(time.RFC3339),
	}}, response.Items)
	keys.AssertExpectations(t)
}

func TestCIMDClientKeysHandler_Promote(t *testing.T) {
	t.Run("promotes a CIMD key immediately without exposing private material", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		activation := time.Date(2026, time.September, 22, 10, 0, 0, 0, time.UTC)
		key := cimdClientKeyForTest("promoted-cimd-key", true, activation, activation.Add(-time.Hour))
		keys.On("PromoteKey", mock.Anything, key.KID).Return(key, nil)

		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Promote(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodPut, "/api/cimd-client-keys/"+key.KID.String()+"/current", key.KID.String(), ``), "operator@example.com"))

		require.Equal(t, http.StatusOK, w.Code)
		assertCIMDKeyDisclosureFree(t, w.Body.String())
		var response cimdClientKeyResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, key.KID.String(), response.KID)
		assert.True(t, response.IsCurrent)
		assert.Equal(t, key.ActivatesAt.Format(time.RFC3339), response.ActivatesAt)
		assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", key.KID.String(), "promote", "success")
		keys.AssertExpectations(t)
	})

	t.Run("maps an unknown CIMD key to the canonical not-found response and a safe audit", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		missingKey := id.NewKeyID("missing-cimd-key")
		keys.On("PromoteKey", mock.Anything, missingKey).
			Return(nil, storage.NewStorageError("PromoteKey", storage.ErrorKindNotFound, nil, cimdKeySensitiveMaterial))

		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Promote(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodPut, "/api/cimd-client-keys/missing-cimd-key/current", missingKey.String(), ``), "operator@example.com"))

		require.Equal(t, http.StatusNotFound, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "CIMD client key not found", response.Error)
		assertCIMDKeyDisclosureFree(t, w.Body.String())
		assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", missingKey.String(), "promote", "rejected")
		keys.AssertExpectations(t)
	})
}

func TestCIMDClientKeysHandler_Remove(t *testing.T) {
	t.Run("removes a retired CIMD key without disclosing lifecycle credentials", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		kid := id.NewKeyID("retired-cimd-key")
		keys.On("DeleteKey", mock.Anything, kid).Return(nil)

		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Remove(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodDelete, "/api/cimd-client-keys/"+kid.String(), kid.String(), ``), "operator@example.com"))

		require.Equal(t, http.StatusNoContent, w.Code)
		assert.Empty(t, w.Body.String())
		assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", kid.String(), "remove", "success")
		keys.AssertExpectations(t)
	})

	for _, tc := range []struct {
		name      string
		lifecycle error
		wantError string
	}{
		{
			name:      "last active key",
			lifecycle: fmt.Errorf("%s: %w", cimdKeySensitiveMaterial, ports.ErrLastActiveKey),
			wantError: "last_key",
		},
		{
			name:      "current key",
			lifecycle: fmt.Errorf("%s: %w", cimdKeySensitiveMaterial, ports.ErrCurrentKey),
			wantError: "current_key",
		},
		{
			name:      "effective current key",
			lifecycle: fmt.Errorf("%s: %w", cimdKeySensitiveMaterial, ports.ErrEffectiveCurrentKey),
			wantError: "current_key",
		},
	} {
		t.Run("rejects removal of the "+tc.name, func(t *testing.T) {
			keys := &MockCIMDClientKeyLifecycle{}
			kid := id.NewKeyID("guarded-cimd-key")
			keys.On("DeleteKey", mock.Anything, kid).Return(tc.lifecycle)

			var audit bytes.Buffer
			handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
			w := httptest.NewRecorder()
			handler.Remove(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodDelete, "/api/cimd-client-keys/"+kid.String(), kid.String(), ``), "operator@example.com"))

			require.Equal(t, http.StatusConflict, w.Code)
			var response ErrorResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
			assert.Equal(t, tc.wantError, response.Error)
			assertCIMDKeyDisclosureFree(t, w.Body.String())
			assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", kid.String(), "remove", "rejected")
			keys.AssertExpectations(t)
		})
	}

	t.Run("maps an unknown CIMD key to not found", func(t *testing.T) {
		keys := &MockCIMDClientKeyLifecycle{}
		missingKey := id.NewKeyID("missing-cimd-key")
		keys.On("DeleteKey", mock.Anything, missingKey).
			Return(storage.NewStorageError("DeleteKey", storage.ErrorKindNotFound, nil, cimdKeySensitiveMaterial))

		var audit bytes.Buffer
		handler := newTestCIMDClientKeysHandler(keys, slog.New(slog.NewJSONHandler(&audit, &slog.HandlerOptions{Level: slog.LevelInfo})))
		w := httptest.NewRecorder()
		handler.Remove(w, withCIMDKeyPrincipal(newCIMDClientKeyRequest(http.MethodDelete, "/api/cimd-client-keys/missing-cimd-key", missingKey.String(), ``), "operator@example.com"))

		require.Equal(t, http.StatusNotFound, w.Code)
		var response ErrorResponse
		require.NoError(t, json.NewDecoder(w.Body).Decode(&response))
		assert.Equal(t, "CIMD client key not found", response.Error)
		assertCIMDKeyDisclosureFree(t, w.Body.String())
		assertCIMDKeyLifecycleAudit(t, audit.String(), "operator@example.com", missingKey.String(), "remove", "rejected")
		keys.AssertExpectations(t)
	})
}
