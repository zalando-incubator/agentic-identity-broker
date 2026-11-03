package consent

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGetUserInfo_Success tests successful retrieval of user info.
func TestGetUserInfo_Success(t *testing.T) {
	t.Parallel()
	principalValue := "user@example.com"

	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), id.NewPrincipal(principalValue))
	// Set profile to test the enriched path (handler reads profile first, then falls back to principal)
	profile := principal.NewProfile(id.NewPrincipal(principalValue))
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, id.Principal(principalValue), resp.Data.Principal)
	assert.Equal(t, principalValue, resp.Data.DisplayName) // Display name defaults to principal
	assert.Nil(t, resp.Data.Email)
	assert.Nil(t, resp.Data.PictureURL)
}

// TestGetUserInfo_DifferentPrincipals tests with different principal values.
func TestGetUserInfo_DifferentPrincipals(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		name      string
		principal string
	}{
		{
			name:      "email principal",
			principal: "user@example.com",
		},
		{
			name:      "UUID principal",
			principal: "550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:      "username principal",
			principal: "john.doe",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			handler := NewUserInfoHandler(nil)

			req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
			ctx := principal.WithPrincipal(req.Context(), id.NewPrincipal(tc.principal))
			req = req.WithContext(ctx)

			rec := httptest.NewRecorder()
			handler.GetUserInfo(rec, req)

			assert.Equal(t, http.StatusOK, rec.Code)

			var resp GetUserInfoResponse
			err := json.NewDecoder(rec.Body).Decode(&resp)
			require.NoError(t, err)

			assert.Equal(t, id.Principal(tc.principal), resp.Data.Principal)
			assert.Equal(t, tc.principal, resp.Data.DisplayName)
		})
	}
}

// TestGetUserInfo_MissingPrincipal tests error when principal is not in context.
func TestGetUserInfo_MissingPrincipal(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	// Request without principal in context
	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	var resp ErrorResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "unauthorized", resp.Error)
	assert.Equal(t, "authentication required", resp.Message)
}

// TestGetUserInfo_EmptyPrincipal tests error when principal is empty string.
func TestGetUserInfo_EmptyPrincipal(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	// Add empty principal to context (simulating a bug or misconfiguration)
	ctx := principal.WithPrincipal(req.Context(), "")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	// Should return unauthorized since empty principal is treated as missing
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
}

// TestGetUserInfo_ContentType tests that response has correct content type.
func TestGetUserInfo_ContentType(t *testing.T) {
	t.Parallel()
	principalValue := "user@example.com"

	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), id.NewPrincipal(principalValue))
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
}

// TestGetUserInfo_ResponseStructure tests the response envelope structure with enriched profile.
func TestGetUserInfo_ResponseStructure(t *testing.T) {
	t.Parallel()
	principalValue := "test@example.com"

	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), id.NewPrincipal(principalValue))
	profile := principal.NewProfile(id.NewPrincipal(principalValue))
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	// Verify response envelope structure
	assert.NotNil(t, resp.Data)

	// Verify required fields
	assert.NotEmpty(t, resp.Data.Principal)
	assert.NotEmpty(t, resp.Data.DisplayName)

	// Optional fields should be nil when not enriched
	assert.Nil(t, resp.Data.Email)
	assert.Nil(t, resp.Data.PictureURL)
}

// --- Enriched Profile Tests (Phase 6: US3) ---

// TestGetUserInfo_EnrichedProfile tests that enriched profile from JWT is returned.
func TestGetUserInfo_EnrichedProfile(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	email := "alice@corp.com"
	pictureURL := "https://cdn.example.com/alice.jpg"

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
	profile := principal.NewProfile("alice@example.com").
		WithDisplayName("Alice Smith").
		WithEmail(&email).
		WithPictureURL(&pictureURL)
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, id.Principal("alice@example.com"), resp.Data.Principal)
	assert.Equal(t, "Alice Smith", resp.Data.DisplayName)
	assert.NotNil(t, resp.Data.Email)
	assert.Equal(t, "alice@corp.com", *resp.Data.Email)
	assert.NotNil(t, resp.Data.PictureURL)
	assert.Equal(t, "https://cdn.example.com/alice.jpg", *resp.Data.PictureURL)
}

// TestGetUserInfo_PartialProfile tests enriched profile with only some fields.
func TestGetUserInfo_PartialProfile(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	email := "alice@corp.com"

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
	profile := principal.NewProfile("alice@example.com").
		WithDisplayName("Alice Smith").
		WithEmail(&email)
	// No PictureURL
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, id.Principal("alice@example.com"), resp.Data.Principal)
	assert.Equal(t, "Alice Smith", resp.Data.DisplayName)
	assert.NotNil(t, resp.Data.Email)
	assert.Equal(t, "alice@corp.com", *resp.Data.Email)
	assert.Nil(t, resp.Data.PictureURL, "pictureUrl should be nil when not extracted")
}

// TestGetUserInfo_PlainHeaderProfile tests that plain header mode returns principal-only profile.
func TestGetUserInfo_PlainHeaderProfile(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	// Plain header: profile has principal as display name, no email/picture
	profile := principal.NewProfile("user@example.com")
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, id.Principal("user@example.com"), resp.Data.Principal)
	assert.Equal(t, "user@example.com", resp.Data.DisplayName, "display name should equal principal for plain header")
	assert.Nil(t, resp.Data.Email, "email should be nil for plain header")
	assert.Nil(t, resp.Data.PictureURL, "pictureUrl should be nil for plain header")
}

// TestGetUserInfo_EmailOmittedInJSON tests that email field is omitted from JSON when nil.
func TestGetUserInfo_EmailOmittedInJSON(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	profile := principal.NewProfile("user@example.com")
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	// Verify email is not present in JSON when nil (omitempty)
	body := rec.Body.String()
	assert.NotContains(t, body, `"email"`, "email field should be omitted when nil")
}

// TestGetUserInfo_EmailPresentInJSON tests that email field is present in JSON when set.
func TestGetUserInfo_EmailPresentInJSON(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	email := "alice@corp.com"

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	ctx := principal.WithPrincipal(req.Context(), "alice@example.com")
	profile := principal.NewProfile("alice@example.com").WithEmail(&email)
	ctx = principal.WithProfile(ctx, profile)
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, `"email"`, "email field should be present when set")
	assert.Contains(t, body, "alice@corp.com")
}

// TestGetUserInfo_BackwardCompatWithPrincipalOnly tests backward compat when only
// the principal string is in context (no profile).
func TestGetUserInfo_BackwardCompatWithPrincipalOnly(t *testing.T) {
	t.Parallel()
	handler := NewUserInfoHandler(nil)

	req := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	// Only set principal, no profile (simulates old middleware)
	ctx := principal.WithPrincipal(req.Context(), "user@example.com")
	req = req.WithContext(ctx)

	rec := httptest.NewRecorder()
	handler.GetUserInfo(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var resp GetUserInfoResponse
	err := json.NewDecoder(rec.Body).Decode(&resp)
	require.NoError(t, err)

	assert.Equal(t, id.Principal("user@example.com"), resp.Data.Principal)
	assert.Equal(t, "user@example.com", resp.Data.DisplayName)
	assert.Nil(t, resp.Data.Email)
	assert.Nil(t, resp.Data.PictureURL)
}
