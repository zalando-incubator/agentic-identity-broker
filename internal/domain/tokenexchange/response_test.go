package tokenexchange

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTokenExchangeResponse_ValidMinimal tests a minimal valid response.
func TestTokenExchangeResponse_ValidMinimal(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...", // access_token
		BearerTokenType,      // token_type
		AccessTokenType,      // issued_token_type
	)

	if err := resp.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	if resp.AccessToken != "ya29.a0AfH6SMBx..." {
		t.Errorf("AccessToken = %q, want %q", resp.AccessToken, "ya29.a0AfH6SMBx...")
	}
	if resp.TokenType != BearerTokenType {
		t.Errorf("TokenType = %q, want %q", resp.TokenType, BearerTokenType)
	}
	if resp.IssuedTokenType != AccessTokenType {
		t.Errorf("IssuedTokenType = %q, want %q", resp.IssuedTokenType, AccessTokenType)
	}
}

// TestTokenExchangeResponse_ValidWithExpiry tests response with expiration time.
func TestTokenExchangeResponse_ValidWithExpiry(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponseWithExpiry(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		3600, // 1 hour
	)

	if err := resp.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	if resp.ExpiresIn != 3600 {
		t.Errorf("ExpiresIn = %d, want 3600", resp.ExpiresIn)
	}
}

// TestTokenExchangeResponse_ValidFull tests response with all optional fields.
func TestTokenExchangeResponse_ValidFull(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponseFull(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		"1//0gvVy...", // refresh_token
		"repo read write",
		3600,
	)

	if err := resp.Validate(); err != nil {
		t.Errorf("Validate() error = %v, want nil", err)
	}

	if resp.RefreshToken != "1//0gvVy..." {
		t.Errorf("RefreshToken = %q, want %q", resp.RefreshToken, "1//0gvVy...")
	}
	if resp.Scope != "repo read write" {
		t.Errorf("Scope = %q, want %q", resp.Scope, "repo read write")
	}
}

// TestTokenExchangeResponse_InvalidEmptyAccessToken tests validation with empty access_token.
func TestTokenExchangeResponse_InvalidEmptyAccessToken(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"", // empty access_token
		BearerTokenType,
		AccessTokenType,
	)

	err := resp.Validate()
	if err == nil {
		t.Errorf("Validate() error = nil, want error for empty access_token")
	}
	if err.Error() != "access_token is required" {
		t.Errorf("error message = %q, want 'access_token is required'", err.Error())
	}
}

// TestTokenExchangeResponse_InvalidEmptyTokenType tests validation with empty token_type.
func TestTokenExchangeResponse_InvalidEmptyTokenType(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		"", // empty token_type
		AccessTokenType,
	)

	err := resp.Validate()
	if err == nil {
		t.Errorf("Validate() error = nil, want error for empty token_type")
	}
	if err.Error() != "token_type is required" {
		t.Errorf("error message = %q, want 'token_type is required'", err.Error())
	}
}

// TestTokenExchangeResponse_InvalidEmptyIssuedTokenType tests validation with empty issued_token_type.
func TestTokenExchangeResponse_InvalidEmptyIssuedTokenType(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		"", // empty issued_token_type
	)

	err := resp.Validate()
	if err == nil {
		t.Errorf("Validate() error = nil, want error for empty issued_token_type")
	}
	if err.Error() != "issued_token_type is required" {
		t.Errorf("error message = %q, want 'issued_token_type is required'", err.Error())
	}
}

// TestTokenExchangeResponse_InvalidNegativeExpiresIn tests validation with negative expires_in.
func TestTokenExchangeResponse_InvalidNegativeExpiresIn(t *testing.T) {
	t.Parallel()
	resp := &TokenExchangeResponse{
		AccessToken:     "ya29.a0AfH6SMBx...",
		TokenType:       BearerTokenType,
		IssuedTokenType: AccessTokenType,
		ExpiresIn:       -1, // negative expiration
	}

	err := resp.Validate()
	if err == nil {
		t.Errorf("Validate() error = nil, want error for negative expires_in")
	}
	if err.Error() != "expires_in must be non-negative" {
		t.Errorf("error message = %q, want 'expires_in must be non-negative'", err.Error())
	}
}

// TestTokenExchangeResponse_ToJSON tests JSON serialization.
func TestTokenExchangeResponse_ToJSON(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponseFull(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		"1//0gvVy...",
		"repo read",
		3600,
	)

	jsonBytes, err := resp.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error = %v, want nil", err)
		return
	}

	// Parse JSON to verify structure
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		t.Errorf("JSON unmarshal error = %v", err)
		return
	}

	// Verify required fields are present
	if data["access_token"] != "ya29.a0AfH6SMBx..." {
		t.Errorf("access_token in JSON = %v, want 'ya29.a0AfH6SMBx...'", data["access_token"])
	}
	if data["token_type"] != BearerTokenType {
		t.Errorf("token_type in JSON = %v, want %q", data["token_type"], BearerTokenType)
	}
	if data["issued_token_type"] != AccessTokenType {
		t.Errorf("issued_token_type in JSON = %v, want %q", data["issued_token_type"], AccessTokenType)
	}

	// Verify optional fields are present when set
	if data["refresh_token"] != "1//0gvVy..." {
		t.Errorf("refresh_token in JSON = %v, want '1//0gvVy...'", data["refresh_token"])
	}
	if data["expires_in"] != float64(3600) {
		t.Errorf("expires_in in JSON = %v, want 3600", data["expires_in"])
	}
}

// TestTokenExchangeResponse_ToJSON_OmitEmptyOptionalFields tests that empty optional fields are omitted.
func TestTokenExchangeResponse_ToJSON_OmitEmptyOptionalFields(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	)

	jsonBytes, err := resp.ToJSON()
	if err != nil {
		t.Errorf("ToJSON() error = %v", err)
		return
	}

	// Parse and check that empty optional fields are not in JSON
	var data map[string]any
	if err := json.Unmarshal(jsonBytes, &data); err != nil {
		t.Errorf("JSON unmarshal error = %v", err)
		return
	}

	// refresh_token, scope, and expires_in should not be present if empty/zero
	if _, hasRefreshToken := data["refresh_token"]; hasRefreshToken {
		// It's ok if it's null/omitted, but we prefer omitted for RFC 8693 compliance
		if data["refresh_token"] != nil {
			t.Errorf("refresh_token should not be present in JSON for empty value")
		}
	}
}

// TestTokenExchangeResponse_String tests the string representation doesn't expose tokens.
func TestTokenExchangeResponse_String(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponseFull(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		"1//0gvVy...",
		"repo read",
		3600,
	)

	str := resp.String()

	// Verify tokens are NOT in the string representation
	if strings.Contains(str, "ya29.a0AfH6SMBx") {
		t.Errorf("String() contains access_token: %s", str)
	}
	if strings.Contains(str, "1//0gvVy") {
		t.Errorf("String() contains refresh_token: %s", str)
	}

	// Verify metadata IS in the string representation
	if !strings.Contains(str, "TokenExchangeResponse") {
		t.Errorf("String() should contain 'TokenExchangeResponse'")
	}
	if !strings.Contains(str, BearerTokenType) {
		t.Errorf("String() should contain token_type")
	}
}

// TestTokenExchangeResponse_WithRefreshToken tests the builder method.
func TestTokenExchangeResponse_WithRefreshToken(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	)

	newResp := resp.WithRefreshToken("1//0gvVy...")

	// Original should be unchanged
	if resp.RefreshToken != "" {
		t.Errorf("original response RefreshToken changed: %q", resp.RefreshToken)
	}

	// New response should have refresh token
	if newResp.RefreshToken != "1//0gvVy..." {
		t.Errorf("new response RefreshToken = %q, want '1//0gvVy...'", newResp.RefreshToken)
	}
}

// TestTokenExchangeResponse_WithScope tests the builder method.
func TestTokenExchangeResponse_WithScope(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	)

	newResp := resp.WithScope("repo read write")

	// Original should be unchanged
	if resp.Scope != "" {
		t.Errorf("original response Scope changed: %q", resp.Scope)
	}

	// New response should have scope
	if newResp.Scope != "repo read write" {
		t.Errorf("new response Scope = %q, want 'repo read write'", newResp.Scope)
	}
}

// TestTokenExchangeResponse_WithExpiresIn tests the builder method.
func TestTokenExchangeResponse_WithExpiresIn(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	)

	newResp := resp.WithExpiresIn(3600)

	// Original should be unchanged
	if resp.ExpiresIn != 0 {
		t.Errorf("original response ExpiresIn changed: %d", resp.ExpiresIn)
	}

	// New response should have expiration
	if newResp.ExpiresIn != 3600 {
		t.Errorf("new response ExpiresIn = %d, want 3600", newResp.ExpiresIn)
	}
}

// TestTokenExchangeResponse_HasRefreshToken tests the query method.
func TestTokenExchangeResponse_HasRefreshToken(t *testing.T) {
	t.Parallel()
	resp1 := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	)
	if resp1.HasRefreshToken() {
		t.Errorf("HasRefreshToken() = true, want false for empty refresh_token")
	}

	resp2 := NewTokenExchangeResponseFull(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		"1//0gvVy...",
		"",
		0,
	)
	if !resp2.HasRefreshToken() {
		t.Errorf("HasRefreshToken() = false, want true")
	}
}

// TestTokenExchangeResponse_GetExpirationSeconds tests the getter method.
func TestTokenExchangeResponse_GetExpirationSeconds(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		expiresIn int64
		want      int64
	}{
		{
			name:      "zero expiration",
			expiresIn: 0,
			want:      0,
		},
		{
			name:      "positive expiration",
			expiresIn: 3600,
			want:      3600,
		},
		{
			name:      "negative expiration",
			expiresIn: -1,
			want:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &TokenExchangeResponse{
				AccessToken:     "token",
				TokenType:       BearerTokenType,
				IssuedTokenType: AccessTokenType,
				ExpiresIn:       tt.expiresIn,
			}

			if resp.GetExpirationSeconds() != tt.want {
				t.Errorf("GetExpirationSeconds() = %d, want %d", resp.GetExpirationSeconds(), tt.want)
			}
		})
	}
}

// TestTokenExchangeResponse_ChainedBuilders tests that builders can be chained.
func TestTokenExchangeResponse_ChainedBuilders(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponse(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
	).WithRefreshToken("1//0gvVy...").
		WithScope("repo read").
		WithExpiresIn(3600)

	if resp.RefreshToken != "1//0gvVy..." {
		t.Errorf("chained WithRefreshToken failed")
	}
	if resp.Scope != "repo read" {
		t.Errorf("chained WithScope failed")
	}
	if resp.ExpiresIn != 3600 {
		t.Errorf("chained WithExpiresIn failed")
	}
}

// TestTokenExchangeResponse_RFC8693Format verifies JSON format matches RFC 8693.
func TestTokenExchangeResponse_RFC8693Format(t *testing.T) {
	t.Parallel()
	resp := NewTokenExchangeResponseFull(
		"ya29.a0AfH6SMBx...",
		BearerTokenType,
		AccessTokenType,
		"1//0gvVy...",
		"repo read",
		3600,
	)

	jsonBytes, _ := resp.ToJSON()

	// Expected RFC 8693 Section 2.2 format
	expected := map[string]any{
		"access_token":      "ya29.a0AfH6SMBx...",
		"token_type":        BearerTokenType,
		"issued_token_type": AccessTokenType,
		"expires_in":        3600.0, // JSON unmarshals as float64
		"refresh_token":     "1//0gvVy...",
		"scope":             "repo read",
	}

	var actual map[string]any
	require.NoError(t, json.Unmarshal(jsonBytes, &actual))

	for key, expectedVal := range expected {
		if actualVal, ok := actual[key]; !ok || actualVal != expectedVal {
			t.Errorf("JSON field %q: got %v, want %v", key, actualVal, expectedVal)
		}
	}
}

// TestTokenExchangeResponse_IsExpired tests expiration check.
func TestTokenExchangeResponse_IsExpired(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		expiresIn int64
		want      bool
	}{
		{
			name:      "not expired (zero)",
			expiresIn: 0,
			want:      false,
		},
		{
			name:      "not expired (positive)",
			expiresIn: 3600,
			want:      false,
		},
		{
			name:      "expired (negative)",
			expiresIn: -1,
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp := &TokenExchangeResponse{
				AccessToken:     "token",
				TokenType:       BearerTokenType,
				IssuedTokenType: AccessTokenType,
				ExpiresIn:       tt.expiresIn,
			}

			if resp.IsExpired() != tt.want {
				t.Errorf("IsExpired() = %v, want %v", resp.IsExpired(), tt.want)
			}
		})
	}
}

func TestTokenExchangeResponse_OptionalApprovalIdentity(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		principal string
		agentID   string
	}{
		{
			name:      "serializes resolved principal and canonical agent ID",
			principal: "alice@example.com",
			agentID:   "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		},
		{
			name:    "omits unresolved principal without suppressing agent ID",
			agentID: "7c9e6679-7425-40de-944b-e07fc1f90ae7",
		},
		{
			name:      "omits unresolved agent ID without suppressing principal",
			principal: "alice@example.com",
		},
		{name: "omits unresolved identity"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			response := NewTokenExchangeResponse("token", BearerTokenType, AccessTokenType)
			response.Principal = tt.principal
			response.AgentID = tt.agentID

			body, err := response.ToJSON()
			require.NoError(t, err)

			var decoded map[string]any
			require.NoError(t, json.Unmarshal(body, &decoded))

			for field, expected := range map[string]string{
				"principal": tt.principal,
				"agent_id":  tt.agentID,
			} {
				actual, present := decoded[field]
				if expected == "" {
					require.False(t, present, "%s must be omitted when unresolved", field)
					continue
				}
				require.True(t, present, "%s must be serialized when resolved", field)
				require.Equal(t, expected, actual)
			}
		})
	}
}
