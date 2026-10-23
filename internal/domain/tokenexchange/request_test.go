package tokenexchange

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTokenExchangeRequest_Valid tests valid token exchange requests.
func TestTokenExchangeRequest_Valid(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		request  *TokenExchangeRequest
		wantErr  bool
		wantCode string
	}{
		{
			name: "valid request with all parameters",
			request: NewTokenExchangeRequest(
				TokenExchangeGrantType,
				"eyJhbGc...", // subject_token (JWT string)
				AccessTokenType,
				"eyJpc3M...", // client_assertion (JWT string)
				JWTBearerType,
				"https://api.example.com",
				"read write",
			),
			wantErr: false,
		},
		{
			name: "valid request with minimal parameters (only required fields)",
			request: NewTokenExchangeRequest(
				TokenExchangeGrantType,
				"eyJhbGc...",
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				"https://api.example.com",
				"", // scope is optional
			),
			wantErr: false,
		},
		{
			name: "valid request with complex resource URI",
			request: NewTokenExchangeRequest(
				TokenExchangeGrantType,
				"eyJhbGc...",
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				"https://api.service.example.com:8443/oauth2",
				"repo:read write user",
			),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.request.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestTokenExchangeRequest_InvalidGrantType tests invalid grant types.
func TestTokenExchangeRequest_InvalidGrantType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		grantType  string
		wantCode   string
		wantErrMsg string
	}{
		{
			name:       "empty grant type",
			grantType:  "",
			wantCode:   "invalid_request",
			wantErrMsg: "grant_type parameter is required",
		},
		{
			name:       "wrong grant type (authorization_code)",
			grantType:  AuthorizationCodeGrantType,
			wantCode:   "invalid_request",
			wantErrMsg: "grant_type must be",
		},
		{
			name:       "wrong grant type (refresh_token)",
			grantType:  RefreshTokenGrantType,
			wantCode:   "invalid_request",
			wantErrMsg: "grant_type must be",
		},
		{
			name:       "completely wrong grant type",
			grantType:  "invalid_grant_type",
			wantCode:   "invalid_request",
			wantErrMsg: "grant_type must be",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := NewTokenExchangeRequest(
				tt.grantType,
				"eyJhbGc...",
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				"https://api.example.com",
				"",
			)
			err := req.Validate()
			if err == nil {
				t.Errorf("Validate() error = nil, want error")
				return
			}
			if texErr, ok := err.(*TokenExchangeError); ok {
				if texErr.Code() != tt.wantCode {
					t.Errorf("error code = %q, want %q", texErr.Code(), tt.wantCode)
				}
			}
		})
	}
}

// TestTokenExchangeRequest_InvalidSubjectToken tests missing or invalid subject_token.
func TestTokenExchangeRequest_InvalidSubjectToken(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		subjectToken string
		wantCode     string
		wantErrMsg   string
	}{
		{
			name:         "empty subject_token",
			subjectToken: "",
			wantCode:     "invalid_request",
			wantErrMsg:   "subject_token parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := NewTokenExchangeRequest(
				TokenExchangeGrantType,
				tt.subjectToken,
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				"https://api.example.com",
				"",
			)
			err := req.Validate()
			if err == nil {
				t.Errorf("Validate() error = nil, want error")
				return
			}
			if texErr, ok := err.(*TokenExchangeError); ok {
				if texErr.Code() != tt.wantCode {
					t.Errorf("error code = %q, want %q", texErr.Code(), tt.wantCode)
				}
			}
		})
	}
}

// TestTokenExchangeRequest_InvalidClientAssertion tests missing or invalid client_assertion.
func TestTokenExchangeRequest_InvalidClientAssertion(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		clientAssertion string
		wantCode        string
		wantErrMsg      string
	}{
		{
			name:            "empty client_assertion",
			clientAssertion: "",
			wantCode:        "invalid_request",
			wantErrMsg:      "client_assertion parameter is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := NewTokenExchangeRequest(
				TokenExchangeGrantType,
				"eyJhbGc...",
				AccessTokenType,
				tt.clientAssertion,
				JWTBearerType,
				"https://api.example.com",
				"",
			)
			err := req.Validate()
			if err == nil {
				t.Errorf("Validate() error = nil, want error")
				return
			}
			if texErr, ok := err.(*TokenExchangeError); ok {
				if texErr.Code() != tt.wantCode {
					t.Errorf("error code = %q, want %q", texErr.Code(), tt.wantCode)
				}
			}
		})
	}
}

// TestTokenExchangeRequest_InvalidResource tests missing or invalid resource parameter.
func TestTokenExchangeRequest_InvalidResource(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		resource   string
		wantCode   string
		wantDetail FailureDetail
	}{
		{
			name:       "empty resource",
			resource:   "",
			wantCode:   "invalid_request",
			wantDetail: DetailResourceMissing,
		},
		{
			name:       "invalid percent-encoding",
			resource:   "https://api.example.com/%GG/",
			wantCode:   "invalid_request",
			wantDetail: DetailRequestMalformed,
		},
		{
			name:       "relative resource URI",
			resource:   "api.example.com",
			wantCode:   "invalid_request",
			wantDetail: DetailRequestMalformed,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := NewTokenExchangeRequest(
				TokenExchangeGrantType,
				"eyJhbGc...",
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				tt.resource,
				"",
			)
			err := req.Validate()
			if err == nil {
				t.Errorf("Validate() error = nil, want error")
				return
			}
			if texErr, ok := err.(*TokenExchangeError); ok {
				if texErr.Code() != tt.wantCode {
					t.Errorf("error code = %q, want %q", texErr.Code(), tt.wantCode)
				}
				if got := texErr.Diagnostic(); got.Detail() != tt.wantDetail || got.Stage() != StageRequestValidation || got.Outcome() != OutcomeInvalidRequest {
					t.Errorf("request classification = %#v, want invalid_request/request_validation/%s", got, tt.wantDetail)
				}
			}
		})
	}
}

func TestTokenExchangeRequest_RequiresExactTokenTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                string
		subjectTokenType    string
		clientAssertionType string
		parameter           string
	}{
		{"missing subject type", "", JWTBearerType, SubjectTokenTypeParam},
		{"JWT subject type", JWTTokenType, JWTBearerType, SubjectTokenTypeParam},
		{"ID token subject type", IDTokenType, JWTBearerType, SubjectTokenTypeParam},
		{"refresh token subject type", RefreshTokenType, JWTBearerType, SubjectTokenTypeParam},
		{"unknown subject type", "unsupported-sensitive-type", JWTBearerType, SubjectTokenTypeParam},
		{"uppercase subject type", strings.ToUpper(AccessTokenType), JWTBearerType, SubjectTokenTypeParam},
		{"leading subject whitespace", " " + AccessTokenType, JWTBearerType, SubjectTokenTypeParam},
		{"trailing subject whitespace", AccessTokenType + " ", JWTBearerType, SubjectTokenTypeParam},
		{"blank subject type", " \t", JWTBearerType, SubjectTokenTypeParam},
		{"missing assertion type", AccessTokenType, "", ClientAssertionTypeParam},
		{"JWT assertion type", AccessTokenType, JWTTokenType, ClientAssertionTypeParam},
		{"ID token assertion type", AccessTokenType, IDTokenType, ClientAssertionTypeParam},
		{"access token assertion type", AccessTokenType, AccessTokenType, ClientAssertionTypeParam},
		{"unknown assertion type", AccessTokenType, "unsupported-sensitive-type", ClientAssertionTypeParam},
		{"uppercase assertion type", AccessTokenType, strings.ToUpper(JWTBearerType), ClientAssertionTypeParam},
		{"leading assertion whitespace", AccessTokenType, " " + JWTBearerType, ClientAssertionTypeParam},
		{"trailing assertion whitespace", AccessTokenType, JWTBearerType + " ", ClientAssertionTypeParam},
		{"blank assertion type", AccessTokenType, " \t", ClientAssertionTypeParam},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			requests := map[string]*TokenExchangeRequest{
				"constructor": NewTokenExchangeRequest(
					TokenExchangeGrantType, "subject-secret", tt.subjectTokenType,
					"assertion-secret", tt.clientAssertionType, "https://api.example.com", "",
				),
				"literal": {
					GrantType:           TokenExchangeGrantType,
					SubjectToken:        "subject-secret",
					SubjectTokenType:    tt.subjectTokenType,
					ClientAssertion:     "assertion-secret",
					ClientAssertionType: tt.clientAssertionType,
					Resource:            "https://api.example.com",
				},
			}
			for name, req := range requests {
				t.Run(name, func(t *testing.T) {
					err := req.Validate()
					require.Error(t, err)
					var tokenErr *TokenExchangeError
					require.ErrorAs(t, err, &tokenErr)
					assert.Equal(t, InvalidRequestError, tokenErr.Code())
					assert.Equal(t, 400, tokenErr.HTTPStatus())
					assert.Contains(t, tokenErr.Description(), tt.parameter)
					assert.Equal(t, StageRequestValidation, tokenErr.Diagnostic().Stage())
					assert.Equal(t, DetailRequestMalformed, tokenErr.Diagnostic().Detail())
					assert.Equal(t, OutcomeInvalidRequest, tokenErr.Diagnostic().Outcome())
					assert.NotContains(t, err.Error(), "subject-secret")
					assert.NotContains(t, err.Error(), "assertion-secret")
					assert.NotContains(t, err.Error(), "unsupported-sensitive-type")
				})
			}
		})
	}
}

// TestTokenExchangeRequest_String tests the string representation doesn't expose tokens.
func TestTokenExchangeRequest_String(t *testing.T) {
	t.Parallel()
	req := NewTokenExchangeRequest(
		TokenExchangeGrantType,
		"eyJhbGc...", // This should NOT appear in String() output
		AccessTokenType,
		"eyJpc3M...", // This should NOT appear in String() output
		JWTBearerType,
		"https://api.example.com",
		"read write",
	)

	str := req.String()

	// Verify tokens are NOT in the string representation
	if strings.Contains(str, "eyJhbGc") {
		t.Errorf("String() contains subject_token: %s", str)
	}
	if strings.Contains(str, "eyJpc3M") {
		t.Errorf("String() contains client_assertion: %s", str)
	}

	// Verify expected fields ARE in the string representation
	if !strings.Contains(str, "TokenExchangeRequest") {
		t.Errorf("String() should contain 'TokenExchangeRequest'")
	}
	if !strings.Contains(str, "https://api.example.com") {
		t.Errorf("String() should contain resource")
	}
}

// TestTokenExchangeRequest_IsTokenExchangeRequest tests the detector method.
func TestTokenExchangeRequest_IsTokenExchangeRequest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		grantType string
		want      bool
	}{
		{
			name:      "token exchange grant type",
			grantType: TokenExchangeGrantType,
			want:      true,
		},
		{
			name:      "authorization code grant type",
			grantType: AuthorizationCodeGrantType,
			want:      false,
		},
		{
			name:      "empty grant type",
			grantType: "",
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := NewTokenExchangeRequest(
				tt.grantType,
				"eyJhbGc...",
				AccessTokenType,
				"eyJpc3M...",
				JWTBearerType,
				"https://api.example.com",
				"",
			)
			if req.IsTokenExchangeRequest() != tt.want {
				t.Errorf("IsTokenExchangeRequest() = %v, want %v", req.IsTokenExchangeRequest(), tt.want)
			}
		})
	}
}
