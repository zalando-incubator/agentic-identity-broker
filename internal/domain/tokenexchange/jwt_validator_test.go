package tokenexchange

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockJWKSProvider is a test double for the JWKSProvider interface
type MockJWKSProvider struct {
	keySet jwk.Set
	err    error
}

func (m *MockJWKSProvider) GetKeySet(ctx context.Context) (jwk.Set, error) {
	return m.keySet, m.err
}

func (m *MockJWKSProvider) GetKey(ctx context.Context, kid string) (jwk.Key, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.keySet == nil {
		return nil, NewInvalidClientError("no keyset")
	}
	key, found := m.keySet.LookupKeyID(kid)
	if !found {
		return nil, NewInvalidClientError("key not found")
	}
	return key, nil
}

// TestNewJWTValidator tests validator creation with various parameter combinations
func TestNewJWTValidator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		jwksProvider     JWKSProvider
		expectedIssuer   string
		brokerAudience   string
		clockSkewSeconds int64
		expectError      bool
		errorContains    string
	}{
		{
			name: "valid parameters",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "broker-id",
			clockSkewSeconds: 60,
			expectError:      false,
		},
		{
			name: "zero clock skew",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "broker-id",
			clockSkewSeconds: 0,
			expectError:      false,
		},
		{
			name:             "nil jwks provider",
			jwksProvider:     nil,
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "broker-id",
			clockSkewSeconds: 60,
			expectError:      true,
			errorContains:    "cannot be nil",
		},
		{
			name: "empty issuer",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "",
			brokerAudience:   "broker-id",
			clockSkewSeconds: 60,
			expectError:      true,
			errorContains:    "cannot be empty",
		},
		{
			name: "empty audience",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "",
			clockSkewSeconds: 60,
			expectError:      true,
			errorContains:    "cannot be empty",
		},
		{
			name: "negative clock skew",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "broker-id",
			clockSkewSeconds: -1,
			expectError:      true,
			errorContains:    "cannot be negative",
		},
		{
			name: "clock skew exceeds max",
			jwksProvider: &MockJWKSProvider{
				keySet: jwk.NewSet(),
			},
			expectedIssuer:   "https://auth.example.com",
			brokerAudience:   "broker-id",
			clockSkewSeconds: MaxClockSkewTolerance + 1,
			expectError:      true,
			errorContains:    "cannot exceed max tolerance",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			validator, err := NewJWTValidator(
				tt.jwksProvider,
				tt.expectedIssuer,
				tt.brokerAudience,
				tt.clockSkewSeconds,
			)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, validator)
				if tt.errorContains != "" {
					assert.ErrorContains(t, err, tt.errorContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, validator)
			}
		})
	}
}

func TestNewJWTValidatorWithPolicies(t *testing.T) {
	t.Parallel()

	baseProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validSubjectPolicy := JWTValidationPolicy{
		JWKSProvider:    baseProvider,
		ExpectedIssuers: []string{"https://upstream.example.com", "https://broker.example.com"},
	}
	validClientPolicy := JWTValidationPolicy{
		JWKSProvider:    baseProvider,
		ExpectedIssuers: []string{"https://upstream.example.com"},
	}

	tests := []struct {
		name                  string
		subjectTokenPolicy    JWTValidationPolicy
		clientAssertionPolicy JWTValidationPolicy
		expectError           bool
		errorContains         string
	}{
		{
			name:                  "valid distinct policies",
			subjectTokenPolicy:    validSubjectPolicy,
			clientAssertionPolicy: validClientPolicy,
		},
		{
			name: "subject policy requires jwks provider",
			subjectTokenPolicy: JWTValidationPolicy{
				ExpectedIssuers: []string{"https://upstream.example.com"},
			},
			clientAssertionPolicy: validClientPolicy,
			expectError:           true,
			errorContains:         "subject_token jwks_provider cannot be nil",
		},
		{
			name:               "client assertion policy requires jwks provider",
			subjectTokenPolicy: validSubjectPolicy,
			clientAssertionPolicy: JWTValidationPolicy{
				ExpectedIssuers: []string{"https://upstream.example.com"},
			},
			expectError:   true,
			errorContains: "client_assertion jwks_provider cannot be nil",
		},
		{
			name: "subject policy requires issuer",
			subjectTokenPolicy: JWTValidationPolicy{
				JWKSProvider: baseProvider,
			},
			clientAssertionPolicy: validClientPolicy,
			expectError:           true,
			errorContains:         "subject_token expected_issuer cannot be empty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			validator, err := NewJWTValidatorWithPolicies(
				tt.subjectTokenPolicy,
				tt.clientAssertionPolicy,
				"broker-id",
				60,
			)

			if tt.expectError {
				require.Error(t, err)
				assert.Nil(t, validator)
				assert.ErrorContains(t, err, tt.errorContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, validator)
			assert.Equal(t, []string{"https://upstream.example.com", "https://broker.example.com"}, validator.subjectTokenPolicy.ExpectedIssuers)
			assert.Equal(t, []string{"https://upstream.example.com"}, validator.clientAssertionPolicy.ExpectedIssuers)
		})
	}
}

// TestValidateSubjectToken_EmptyToken tests handling of empty token
func TestValidateSubjectToken_EmptyToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateSubjectToken(ctx, "")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "empty or missing")
}

// TestValidateSubjectToken_MalformedToken tests handling of malformed token
func TestValidateSubjectToken_MalformedToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateSubjectToken(ctx, "not.a.jwt")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "malformed")
}

// TestValidateSubjectToken_JWKSFetchError tests handling of JWKS fetch error
func TestValidateSubjectToken_JWKSFetchError(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{err: NewServerError("connection failed")}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	// Use a valid JWT structure (but invalid for other reasons)
	// The test will fail at JWKS fetch step
	_, err = validator.ValidateSubjectToken(ctx, "eyJhbGciOiJIUzI1NiIsImtpZCI6InRlc3Qta2lkIn0.eyJzdWIiOiJ0ZXN0In0.test")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "failed to fetch JWKS")
}

// TestValidateClientAssertion_EmptyToken tests handling of empty client_assertion
func TestValidateClientAssertion_EmptyToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateClientAssertion(ctx, "")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "empty or missing")
}

// TestValidateClientAssertion_MalformedToken tests handling of malformed client_assertion
func TestValidateClientAssertion_MalformedToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateClientAssertion(ctx, "not.a.jwt")

	assert.Error(t, err)
	assert.ErrorContains(t, err, "malformed")
}

// TestMapParseError_WrapsUnderlyingCause verifies that JWT validation errors include the
// underlying library error as a cause so operators can log full details internally.
func TestMapParseError_WrapsUnderlyingCause(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	tests := []struct {
		name            string
		rawErr          error
		wantErrContains string
	}{
		{name: "parse/signature error mapped via ParseError sentinel", rawErr: jwt.ParseError{}, wantErrContains: "malformed"},
		{name: "issuer error includes expected iss value", rawErr: jwt.InvalidIssuerError{}, wantErrContains: `expected iss="https://auth.example.com"`},
		{name: "audience error includes expected aud value", rawErr: jwt.InvalidAudienceError{}, wantErrContains: `expected aud="broker-id"`},
		{name: "expired token mapped", rawErr: jwt.TokenExpiredError{}, wantErrContains: "expired"},
		{name: "not-yet-valid token mapped", rawErr: jwt.TokenNotYetValidError{}, wantErrContains: "not yet valid"},
		{name: "unknown error falls through to default case", rawErr: fmt.Errorf("some totally unknown jwt failure"), wantErrContains: "validation failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			domErr := validator.mapParseError(tt.rawErr, "subject_token", "")
			require.Error(t, domErr)

			// The domain error should contain the expected user-facing description
			assert.ErrorContains(t, domErr, tt.wantErrContains)

			// The underlying cause must be wrapped for internal logging
			tokenErr, ok := domErr.(*TokenExchangeError)
			require.True(t, ok, "expected *TokenExchangeError")
			require.NotNil(t, tokenErr.Unwrap(), "expected underlying cause to be set for logging")
			assert.Equal(t, tt.rawErr, tokenErr.Unwrap())
		})
	}
}

// TestMapClientAssertionParseError_WrapsUnderlyingCause verifies that client assertion validation
// errors include the underlying library error as a cause for internal logging.
func TestMapClientAssertionParseError_WrapsUnderlyingCause(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	tests := []struct {
		name            string
		rawErr          error
		wantErrContains string
	}{
		{name: "parse/signature error mapped via ParseError sentinel", rawErr: jwt.ParseError{}, wantErrContains: "malformed"},
		{name: "issuer error includes expected iss value", rawErr: jwt.InvalidIssuerError{}, wantErrContains: `expected iss="https://auth.example.com"`},
		{name: "audience error includes expected aud value", rawErr: jwt.InvalidAudienceError{}, wantErrContains: `expected aud="broker-id"`},
		{name: "expired token mapped", rawErr: jwt.TokenExpiredError{}, wantErrContains: "expired"},
		{name: "not-yet-valid token mapped", rawErr: jwt.TokenNotYetValidError{}, wantErrContains: "not yet valid"},
		{name: "unknown error falls through to default case", rawErr: fmt.Errorf("some totally unknown jwt failure"), wantErrContains: "validation failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			domErr := validator.mapClientAssertionParseError(tt.rawErr, "")
			require.Error(t, domErr)

			assert.ErrorContains(t, domErr, tt.wantErrContains)

			tokenErr, ok := domErr.(*TokenExchangeError)
			require.True(t, ok, "expected *TokenExchangeError")
			require.NotNil(t, tokenErr.Unwrap(), "expected underlying cause to be set for logging")
			assert.Equal(t, tt.rawErr, tokenErr.Unwrap())
		})
	}
}

// TestExtractDiagnostics verifies that extractDiagnostics returns expected vs actual
// claim values when given a parseable JWT, and empty string for unparseable input.
func TestExtractDiagnostics(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	t.Run("empty token returns empty string", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", validator.extractDiagnostics("", validator.subjectTokenPolicy.ExpectedIssuers))
	})

	t.Run("unparseable token returns empty string", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", validator.extractDiagnostics("not-a-jwt", validator.subjectTokenPolicy.ExpectedIssuers))
	})

	t.Run("parseable token returns expected vs actual claims", func(t *testing.T) {
		t.Parallel()
		tok, buildErr := jwt.NewBuilder().
			Subject("user@example.com").
			Issuer("https://wrong-issuer.com").
			Audience([]string{"wrong-audience"}).
			Build()
		require.NoError(t, buildErr)
		serialized, signErr := jwt.Sign(tok, jwt.WithInsecureNoSignature())
		require.NoError(t, signErr)

		diag := validator.extractDiagnostics(string(serialized), validator.subjectTokenPolicy.ExpectedIssuers)
		assert.Contains(t, diag, `sub="user@example.com"`)
		assert.Contains(t, diag, `expected_iss="https://auth.example.com"`)
		assert.Contains(t, diag, `actual_iss="https://wrong-issuer.com"`)
		assert.Contains(t, diag, `expected_aud="broker-id"`)
		assert.Contains(t, diag, `actual_aud=["wrong-audience"]`)
	})

	t.Run("diagnostics included in mapParseError details", func(t *testing.T) {
		t.Parallel()
		tok, buildErr := jwt.NewBuilder().
			Subject("debug-user").
			Issuer("https://wrong-issuer.com").
			Audience([]string{"wrong-aud"}).
			Build()
		require.NoError(t, buildErr)
		serialized, signErr := jwt.Sign(tok, jwt.WithInsecureNoSignature())
		require.NoError(t, signErr)

		domErr := validator.mapParseError(jwt.InvalidAudienceError{}, "subject_token", string(serialized))
		tokenErr, ok := domErr.(*TokenExchangeError)
		require.True(t, ok)
		assert.Contains(t, tokenErr.Details(), `sub="debug-user"`)
		assert.Contains(t, tokenErr.Details(), `expected_aud="broker-id"`)
		assert.Contains(t, tokenErr.Details(), `actual_aud=["wrong-aud"]`)
	})
}

// newTestKeyPair creates an ECDSA P-256 key pair for testing.
// Returns the JWK private key and a key set containing the public key.
func newTestKeyPair(t *testing.T) (jwk.Key, jwk.Set) {
	return newTestKeyPairWithID(t, "test-kid")
}

func newTestKeyPairWithID(t *testing.T, kid string) (jwk.Key, jwk.Set) {
	t.Helper()
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	jwkKey, err := jwk.Import[jwk.Key](privKey)
	require.NoError(t, err)
	require.NoError(t, jwkKey.Set(jwk.KeyIDKey, kid))
	require.NoError(t, jwkKey.Set(jwk.AlgorithmKey, jwa.ES256()))

	pubKey, err := jwkKey.PublicKey()
	require.NoError(t, err)
	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(pubKey))
	return jwkKey, keySet
}

func mergeKeySets(t *testing.T, sets ...jwk.Set) jwk.Set {
	t.Helper()

	merged := jwk.NewSet()
	for _, set := range sets {
		for i := 0; i < set.Len(); i++ {
			key, ok := set.Key(i)
			require.True(t, ok)
			require.NoError(t, merged.AddKey(key))
		}
	}
	return merged
}

func newSignedToken(
	t *testing.T,
	signingKey jwk.Key,
	issuer string,
	audience []string,
	subject string,
	expiration time.Time,
) string {
	t.Helper()

	tok, err := jwt.NewBuilder().
		Issuer(issuer).
		Audience(audience).
		Subject(subject).
		Expiration(expiration).
		Build()
	require.NoError(t, err)

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), signingKey))
	require.NoError(t, err)

	return string(signed)
}

func TestJWTValidatorWithSeparatePolicies(t *testing.T) {
	t.Parallel()

	const (
		upstreamIssuer = "https://upstream.example.com"
		localIssuer    = "https://broker.example.com"
		audience       = "broker-id"
	)

	upstreamKey, upstreamSet := newTestKeyPairWithID(t, "upstream-kid")
	localKey, localSet := newTestKeyPairWithID(t, "local-kid")
	subjectTokenProvider := &MockJWKSProvider{keySet: mergeKeySets(t, upstreamSet, localSet)}
	clientAssertionProvider := &MockJWKSProvider{keySet: upstreamSet}

	validator, err := NewJWTValidatorWithPolicies(
		JWTValidationPolicy{
			JWKSProvider:    subjectTokenProvider,
			ExpectedIssuers: []string{upstreamIssuer, localIssuer},
		},
		JWTValidationPolicy{
			JWKSProvider:    clientAssertionProvider,
			ExpectedIssuers: []string{upstreamIssuer},
		},
		audience,
		60,
	)
	require.NoError(t, err)

	t.Run("subject token signed by upstream is accepted", func(t *testing.T) {
		t.Parallel()

		tokenString := newSignedToken(t, upstreamKey, upstreamIssuer, []string{audience}, "upstream-user", time.Now().Add(time.Hour))

		token, err := validator.ValidateSubjectToken(context.Background(), tokenString)

		require.NoError(t, err)
		require.NotNil(t, token)
		issuer, ok := token.Issuer()
		require.True(t, ok)
		assert.Equal(t, upstreamIssuer, issuer)
	})

	t.Run("subject token signed by local broker key is accepted", func(t *testing.T) {
		t.Parallel()

		tokenString := newSignedToken(t, localKey, localIssuer, []string{audience}, "local-user", time.Now().Add(time.Hour))

		token, err := validator.ValidateSubjectToken(context.Background(), tokenString)

		require.NoError(t, err)
		require.NotNil(t, token)
		issuer, ok := token.Issuer()
		require.True(t, ok)
		assert.Equal(t, localIssuer, issuer)
	})

	t.Run("client assertion signed by local broker key is rejected", func(t *testing.T) {
		t.Parallel()

		tokenString := newSignedToken(t, localKey, localIssuer, []string{audience}, "local-client", time.Now().Add(time.Hour))

		_, err := validator.ValidateClientAssertion(context.Background(), tokenString)

		require.Error(t, err)
		assert.ErrorContains(t, err, "malformed or signature verification failed")

		tokenErr, ok := err.(*TokenExchangeError)
		require.True(t, ok)
		assert.Equal(t, "invalid_client", tokenErr.Code())
	})
}

// TestMapParseError_RealLibraryErrors verifies that validation errors produced by the actual
// jwt.ParseString library function are correctly classified as specific error types (audience,
// issuer, expired) rather than falling into the generic "malformed or signature verification
// failed" bucket. The jwx library wraps all validation failures inside a ParseError, so
// without correct ordering of error checks, specific failures would be misclassified.
func TestMapParseError_RealLibraryErrors(t *testing.T) {
	t.Parallel()
	privKey, keySet := newTestKeyPair(t)

	const expectedIssuer = "https://auth.example.com"
	const expectedAudience = "broker-id"

	tests := []struct {
		name             string
		tokenIssuer      string
		tokenAudience    []string
		tokenExpiration  time.Time
		tokenNBF         time.Time
		wantErrContains  string
		wantCode         string
		wantHasDetails   bool
		wantDetailsField string
	}{
		{
			name:             "audience mismatch produces specific audience error with diagnostics",
			tokenIssuer:      expectedIssuer,
			tokenAudience:    []string{"wrong-audience"},
			tokenExpiration:  time.Now().Add(time.Hour),
			wantErrContains:  "audience validation failed",
			wantCode:         "invalid_grant",
			wantHasDetails:   true,
			wantDetailsField: `actual_aud=["wrong-audience"]`,
		},
		{
			name:             "issuer mismatch produces specific issuer error with diagnostics",
			tokenIssuer:      "https://wrong-issuer.com",
			tokenAudience:    []string{expectedAudience},
			tokenExpiration:  time.Now().Add(time.Hour),
			wantErrContains:  "issuer validation failed",
			wantCode:         "invalid_grant",
			wantHasDetails:   true,
			wantDetailsField: `actual_iss="https://wrong-issuer.com"`,
		},
		{
			name:             "expired token produces specific expiry error with diagnostics",
			tokenIssuer:      expectedIssuer,
			tokenAudience:    []string{expectedAudience},
			tokenExpiration:  time.Now().Add(-time.Hour),
			wantErrContains:  "has expired",
			wantCode:         "invalid_grant",
			wantHasDetails:   true,
			wantDetailsField: "exp=",
		},
		{
			name:             "not-yet-valid token produces specific nbf error with diagnostics",
			tokenIssuer:      expectedIssuer,
			tokenAudience:    []string{expectedAudience},
			tokenExpiration:  time.Now().Add(time.Hour),
			tokenNBF:         time.Now().Add(time.Hour),
			wantErrContains:  "not yet valid",
			wantCode:         "invalid_grant",
			wantHasDetails:   true,
			wantDetailsField: "nbf=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			builder := jwt.NewBuilder().
				Issuer(tt.tokenIssuer).
				Audience(tt.tokenAudience).
				Subject("test-user@example.com").
				Expiration(tt.tokenExpiration)
			if !tt.tokenNBF.IsZero() {
				builder = builder.NotBefore(tt.tokenNBF)
			}
			tok, err := builder.Build()
			require.NoError(t, err)

			signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), privKey))
			require.NoError(t, err)

			// Parse with the same options as the production validator so the test
			// exercises the identical error-wrapping codepath.
			_, parseErr := jwt.ParseString(string(signed),
				jwt.WithVerify(true),
				jwt.WithKeySet(keySet),
				jwt.WithValidate(true),
				jwt.WithIssuer(expectedIssuer),
				jwt.WithAudience(expectedAudience),
				jwt.WithAcceptableSkew(60*time.Second),
			)
			require.Error(t, parseErr, "expected library to reject the token")

			// Now map the error through our domain mapper
			mockProvider := &MockJWKSProvider{keySet: keySet}
			validator, valErr := NewJWTValidator(mockProvider, expectedIssuer, expectedAudience, 60)
			require.NoError(t, valErr)

			domErr := validator.mapParseError(parseErr, "subject_token", string(signed))
			require.Error(t, domErr)

			// Verify the error is classified correctly (not as generic "malformed")
			assert.Contains(t, domErr.Error(), tt.wantErrContains,
				"error should be classified as specific validation failure, not generic parse error")
			assert.NotContains(t, domErr.Error(), "malformed",
				"specific validation failures must NOT be classified as malformed/signature errors")

			tokenErr, ok := domErr.(*TokenExchangeError)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, tokenErr.Code())

			// Verify diagnostics are present
			if tt.wantHasDetails {
				assert.NotEmpty(t, tokenErr.Details(),
					"specific validation errors must include diagnostics for troubleshooting")
				assert.Contains(t, tokenErr.Details(), tt.wantDetailsField)
			}

			// Verify the underlying cause is preserved for logging
			assert.NotNil(t, tokenErr.Unwrap(), "cause must be preserved for internal logging")
		})
	}
}

// TestMapClientAssertionParseError_RealLibraryErrors verifies the same fix for client assertions.
func TestMapClientAssertionParseError_RealLibraryErrors(t *testing.T) {
	t.Parallel()
	privKey, keySet := newTestKeyPair(t)

	const expectedIssuer = "https://auth.example.com"
	const expectedAudience = "broker-id"

	tests := []struct {
		name             string
		tokenIssuer      string
		tokenAudience    []string
		tokenExpiration  time.Time
		wantErrContains  string
		wantCode         string
		wantHasDetails   bool
		wantDetailsField string
	}{
		{
			name:             "audience mismatch produces specific audience error with diagnostics",
			tokenIssuer:      expectedIssuer,
			tokenAudience:    []string{"wrong-audience"},
			tokenExpiration:  time.Now().Add(time.Hour),
			wantErrContains:  "audience validation failed",
			wantCode:         "invalid_client",
			wantHasDetails:   true,
			wantDetailsField: `actual_aud=["wrong-audience"]`,
		},
		{
			name:             "issuer mismatch produces specific issuer error with diagnostics",
			tokenIssuer:      "https://wrong-issuer.com",
			tokenAudience:    []string{expectedAudience},
			tokenExpiration:  time.Now().Add(time.Hour),
			wantErrContains:  "issuer validation failed",
			wantCode:         "invalid_client",
			wantHasDetails:   true,
			wantDetailsField: `actual_iss="https://wrong-issuer.com"`,
		},
		{
			name:             "expired token produces specific expiry error with diagnostics",
			tokenIssuer:      expectedIssuer,
			tokenAudience:    []string{expectedAudience},
			tokenExpiration:  time.Now().Add(-time.Hour),
			wantErrContains:  "has expired",
			wantCode:         "invalid_client",
			wantHasDetails:   true,
			wantDetailsField: "exp=",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			tok, err := jwt.NewBuilder().
				Issuer(tt.tokenIssuer).
				Audience(tt.tokenAudience).
				Subject("test-client").
				Expiration(tt.tokenExpiration).
				Build()
			require.NoError(t, err)

			signed, err := jwt.Sign(tok, jwt.WithKey(jwa.ES256(), privKey))
			require.NoError(t, err)

			_, parseErr := jwt.ParseString(string(signed),
				jwt.WithVerify(true),
				jwt.WithKeySet(keySet),
				jwt.WithValidate(true),
				jwt.WithIssuer(expectedIssuer),
				jwt.WithAudience(expectedAudience),
				jwt.WithAcceptableSkew(60*time.Second),
			)
			require.Error(t, parseErr)

			mockProvider := &MockJWKSProvider{keySet: keySet}
			validator, valErr := NewJWTValidator(mockProvider, expectedIssuer, expectedAudience, 60)
			require.NoError(t, valErr)

			domErr := validator.mapClientAssertionParseError(parseErr, string(signed))
			require.Error(t, domErr)

			assert.Contains(t, domErr.Error(), tt.wantErrContains)
			assert.NotContains(t, domErr.Error(), "malformed")

			tokenErr, ok := domErr.(*TokenExchangeError)
			require.True(t, ok)
			assert.Equal(t, tt.wantCode, tokenErr.Code())

			if tt.wantHasDetails {
				assert.NotEmpty(t, tokenErr.Details())
				assert.Contains(t, tokenErr.Details(), tt.wantDetailsField)
			}

			assert.NotNil(t, tokenErr.Unwrap())
		})
	}
}

type verificationKeySet interface{ jwk.Set }

type countingVerificationKeySet struct {
	verificationKeySet
	lookups int
}

func (s *countingVerificationKeySet) LookupKeyID(kid string) (jwk.Key, bool) {
	s.lookups++
	return s.verificationKeySet.LookupKeyID(kid)
}

func TestJWTValidator_MultipleIssuersVerifyOnlyOnce(t *testing.T) {
	const firstIssuer = "https://first.example.com"
	const secondIssuer = "https://second.example.com"
	const audience = "broker-id"
	privateKey, keySet := newTestKeyPair(t)
	for _, tc := range []struct {
		name      string
		issuer    string
		audience  []string
		expires   time.Time
		wantError string
	}{
		{"second issuer accepted", secondIssuer, []string{audience}, time.Now().Add(time.Hour), ""},
		{"untrusted issuer rejected", "https://other.example.com", []string{audience}, time.Now().Add(time.Hour), "issuer validation failed"},
		{"expired second issuer rejected", secondIssuer, []string{audience}, time.Now().Add(-time.Hour), "has expired"},
		{"wrong audience rejected", secondIssuer, []string{"other-audience"}, time.Now().Add(time.Hour), "audience validation failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			countingSet := &countingVerificationKeySet{verificationKeySet: keySet}
			validator, err := NewJWTValidatorWithPolicies(
				JWTValidationPolicy{JWKSProvider: &MockJWKSProvider{keySet: countingSet}, ExpectedIssuers: []string{firstIssuer, secondIssuer}},
				JWTValidationPolicy{JWKSProvider: &MockJWKSProvider{keySet: countingSet}, ExpectedIssuers: []string{firstIssuer}},
				audience, 60,
			)
			require.NoError(t, err)
			signed := newSignedToken(t, privateKey, tc.issuer, tc.audience, "subject", tc.expires)
			_, err = validator.ValidateSubjectToken(context.Background(), signed)
			if tc.wantError == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.wantError)
			}
			assert.Equal(t, 1, countingSet.lookups, "each credential needs only one cryptographic verification")
		})
	}
	t.Run("invalid signature rejected without issuer retries", func(t *testing.T) {
		untrustedKey, _ := newTestKeyPair(t)
		countingSet := &countingVerificationKeySet{verificationKeySet: keySet}
		validator, err := NewJWTValidatorWithPolicies(
			JWTValidationPolicy{JWKSProvider: &MockJWKSProvider{keySet: countingSet}, ExpectedIssuers: []string{firstIssuer, secondIssuer}},
			JWTValidationPolicy{JWKSProvider: &MockJWKSProvider{keySet: countingSet}, ExpectedIssuers: []string{firstIssuer}},
			audience, 60,
		)
		require.NoError(t, err)
		credential := newSignedToken(t, untrustedKey, secondIssuer, []string{audience}, "subject", time.Now().Add(time.Hour))
		_, err = validator.ValidateSubjectToken(context.Background(), credential)
		require.ErrorContains(t, err, "malformed or signature verification failed")
		assert.Equal(t, 1, countingSet.lookups)
	})
}
