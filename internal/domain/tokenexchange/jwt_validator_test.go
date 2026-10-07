package tokenexchange

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
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
				_ = assertOriginDiagnostic(t, err, "server_error", OutcomeConfigurationError, StageSubjectValidation, DetailJWTConfiguration)
				assert.Nil(t, validator)
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
		stage                 FailureStage
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
			stage:                 StageSubjectValidation,
		},
		{
			name:               "client assertion policy requires jwks provider",
			subjectTokenPolicy: validSubjectPolicy,
			clientAssertionPolicy: JWTValidationPolicy{
				ExpectedIssuers: []string{"https://upstream.example.com"},
			},
			expectError: true,
			stage:       StageClientValidation,
		},
		{
			name: "subject policy requires issuer",
			subjectTokenPolicy: JWTValidationPolicy{
				JWKSProvider: baseProvider,
			},
			clientAssertionPolicy: validClientPolicy,
			expectError:           true,
			stage:                 StageSubjectValidation,
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
				_ = assertOriginDiagnostic(t, err, "server_error", OutcomeConfigurationError, tt.stage, DetailJWTConfiguration)
				assert.Nil(t, validator)
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

	_ = assertOriginDiagnostic(t, err, "invalid_request", OutcomeInvalidRequest, StageRequestValidation, DetailRequestMalformed)
}

// TestValidateSubjectToken_MalformedToken tests handling of malformed token
func TestValidateSubjectToken_MalformedToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateSubjectToken(ctx, "not.a.jwt")

	_ = assertOriginDiagnostic(t, err, "invalid_grant", OutcomeAuthenticationFailed, StageSubjectValidation, DetailSubjectInvalid)
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

	_ = assertOriginDiagnostic(t, err, "server_error", OutcomeInfrastructureError, StageSubjectValidation, DetailJWKSUnavailable)
}

// TestValidateClientAssertion_EmptyToken tests handling of empty client_assertion
func TestValidateClientAssertion_EmptyToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateClientAssertion(ctx, "")

	_ = assertOriginDiagnostic(t, err, "invalid_request", OutcomeInvalidRequest, StageRequestValidation, DetailRequestMalformed)
}

// TestValidateClientAssertion_MalformedToken tests handling of malformed client_assertion
func TestValidateClientAssertion_MalformedToken(t *testing.T) {
	t.Parallel()
	mockProvider := &MockJWKSProvider{keySet: jwk.NewSet()}
	validator, err := NewJWTValidator(mockProvider, "https://auth.example.com", "broker-id", 60)
	require.NoError(t, err)

	ctx := context.Background()
	_, err = validator.ValidateClientAssertion(ctx, "not.a.jwt")

	_ = assertOriginDiagnostic(t, err, "invalid_client", OutcomeAuthenticationFailed, StageClientValidation, DetailClientInvalid)
}

func assertOriginDiagnostic(t *testing.T, err error, code string, outcome Outcome, stage FailureStage, detail FailureDetail) *TokenExchangeError {
	t.Helper()
	require.Error(t, err)
	var tokenErr *TokenExchangeError
	require.ErrorAs(t, err, &tokenErr)
	assert.Equal(t, code, tokenErr.Code())
	assert.Equal(t, outcome, tokenErr.Diagnostic().Outcome())
	assert.Equal(t, stage, tokenErr.Diagnostic().Stage())
	assert.Equal(t, detail, tokenErr.Diagnostic().Detail())
	assert.Empty(t, tokenErr.Details())
	return tokenErr
}

func TestJWTValidationPreservesTypedCauses(t *testing.T) {
	t.Parallel()
	validator, err := NewJWTValidator(&MockJWKSProvider{keySet: jwk.NewSet()}, "https://issuer.example", "broker", 60)
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		cause   error
		expired bool
	}{
		{"parse", jwt.ParseError{}, false},
		{"issuer", jwt.InvalidIssuerError{}, false},
		{"audience", jwt.InvalidAudienceError{}, false},
		{"expired", jwt.TokenExpiredError{}, true},
		{"not yet valid", jwt.TokenNotYetValidError{}, false},
		{"unknown", errors.New("secret-cause-credential"), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			subjectDetail, clientDetail := DetailSubjectInvalid, DetailClientInvalid
			if tc.expired {
				subjectDetail, clientDetail = DetailSubjectExpired, DetailClientExpired
			}
			subjectErr := validator.mapParseError(tc.cause)
			clientErr := validator.mapClientAssertionParseError(tc.cause)
			_ = assertOriginDiagnostic(t, subjectErr, "invalid_grant", OutcomeAuthenticationFailed, StageSubjectValidation, subjectDetail)
			_ = assertOriginDiagnostic(t, clientErr, "invalid_client", OutcomeAuthenticationFailed, StageClientValidation, clientDetail)
			assert.ErrorIs(t, subjectErr, tc.cause)
			assert.ErrorIs(t, clientErr, tc.cause)
			assert.NotContains(t, subjectErr.Error(), "secret-cause-credential")
			assert.NotContains(t, clientErr.Error(), "secret-cause-credential")
		})
	}
}

type originTestError struct{ value string }

func (e *originTestError) Error() string { return e.value }

func TestJWTKeyFetchDiagnostic(t *testing.T) {
	t.Parallel()
	secretCause := &originTestError{value: "https://secret-expected:secret-actual@example.test/private?credential=secret-token"}
	for _, role := range []struct {
		name     string
		stage    FailureStage
		validate func(*JWTValidator, context.Context, string) (jwt.Token, error)
	}{
		{"subject", StageSubjectValidation, (*JWTValidator).ValidateSubjectToken},
		{"client", StageClientValidation, (*JWTValidator).ValidateClientAssertion},
	} {
		for _, tc := range []struct {
			name        string
			cause       error
			callerError error
			outcome     Outcome
			detail      FailureDetail
		}{
			{"dependency failure", secretCause, nil, OutcomeInfrastructureError, DetailJWKSUnavailable},
			{"shared deadline", context.DeadlineExceeded, nil, OutcomeInfrastructureError, DetailJWKSUnavailable},
			{"detached cancellation", context.Canceled, nil, OutcomeInfrastructureError, DetailJWKSUnavailable},
			{"caller canceled", context.Canceled, context.Canceled, OutcomeCanceled, DetailCallerCanceled},
			{"caller deadline", context.DeadlineExceeded, context.DeadlineExceeded, OutcomeCanceled, DetailCallerCanceled},
			{"caller canceled with dependency cause", secretCause, context.Canceled, OutcomeCanceled, DetailCallerCanceled},
		} {
			t.Run(role.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				var ctx context.Context
				var cancel context.CancelFunc
				if tc.callerError == context.DeadlineExceeded {
					ctx, cancel = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				} else {
					ctx, cancel = context.WithCancel(context.Background())
					if tc.callerError != nil {
						cancel()
					}
				}
				defer cancel()
				validator, err := NewJWTValidator(&MockJWKSProvider{err: tc.cause}, "https://issuer.example", "broker", 60)
				require.NoError(t, err)
				_, err = role.validate(validator, ctx, "secret-raw-credential")
				tokenErr := assertOriginDiagnostic(t, err, "server_error", tc.outcome, role.stage, tc.detail)
				assert.ErrorIs(t, err, tc.cause)
				if tc.callerError != nil {
					assert.ErrorIs(t, err, tc.callerError)
				}
				if tc.cause == secretCause {
					var dependencyErr *originTestError
					require.ErrorAs(t, err, &dependencyErr)
					assert.Same(t, secretCause, dependencyErr)
				}
				for _, secret := range []string{"secret-expected", "secret-actual", "secret-raw-credential"} {
					assert.NotContains(t, fmt.Sprint(tokenErr, tokenErr.Description(), tokenErr.Details(), tokenErr.Diagnostic()), secret)
				}
			})
		}
	}
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

		_ = assertOriginDiagnostic(t, err, "invalid_client", OutcomeAuthenticationFailed, StageClientValidation, DetailClientInvalid)
	})
}

func TestJWTValidatorRejectedCredentials(t *testing.T) {
	t.Parallel()
	privateKey, keySet := newTestKeyPair(t)
	untrustedKey, _ := newTestKeyPair(t)
	const expectedIssuer = "https://issuer.example/secret-config-issuer"
	const expectedAudience = "secret-config-audience"
	for _, role := range []struct {
		name     string
		code     string
		stage    FailureStage
		invalid  FailureDetail
		expired  FailureDetail
		target   RecoveryTarget
		validate func(*JWTValidator, context.Context, string) (jwt.Token, error)
	}{
		{"subject", "invalid_grant", StageSubjectValidation, DetailSubjectInvalid, DetailSubjectExpired, TargetSubjectIdentity, (*JWTValidator).ValidateSubjectToken},
		{"client", "invalid_client", StageClientValidation, DetailClientInvalid, DetailClientExpired, TargetCallingClient, (*JWTValidator).ValidateClientAssertion},
	} {
		for _, tc := range []struct {
			name       string
			issuer     string
			audience   []string
			expiration time.Time
			nbf        time.Time
			key        jwk.Key
			cause      error
			isExpired  bool
		}{
			{"issuer", "https://untrusted.example/secret-token-issuer", []string{expectedAudience}, time.Now().Add(time.Hour), time.Time{}, privateKey, jwt.InvalidIssuerError{}, false},
			{"audience", expectedIssuer, []string{"secret-token-audience"}, time.Now().Add(time.Hour), time.Time{}, privateKey, jwt.InvalidAudienceError{}, false},
			{"expiration", expectedIssuer, []string{expectedAudience}, time.Now().Add(-time.Hour), time.Time{}, privateKey, jwt.TokenExpiredError{}, true},
			{"not before", expectedIssuer, []string{expectedAudience}, time.Now().Add(time.Hour), time.Now().Add(time.Hour), privateKey, jwt.TokenNotYetValidError{}, false},
			{"signature", expectedIssuer, []string{expectedAudience}, time.Now().Add(time.Hour), time.Time{}, untrustedKey, jwt.ParseError{}, false},
		} {
			t.Run(role.name+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				builder := jwt.NewBuilder().Issuer(tc.issuer).Audience(tc.audience).Subject("secret-token-subject").Expiration(tc.expiration)
				if !tc.nbf.IsZero() {
					builder = builder.NotBefore(tc.nbf)
				}
				token, err := builder.Build()
				require.NoError(t, err)
				signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), tc.key))
				require.NoError(t, err)
				validator, err := NewJWTValidator(&MockJWKSProvider{keySet: keySet}, expectedIssuer, expectedAudience, 60)
				require.NoError(t, err)
				_, err = role.validate(validator, context.Background(), string(signed))
				detail := role.invalid
				if tc.isExpired {
					detail = role.expired
				}
				tokenErr := assertOriginDiagnostic(t, err, role.code, OutcomeAuthenticationFailed, role.stage, detail)
				assert.ErrorIs(t, err, tc.cause)
				assert.Equal(t, RecoveryReauthenticate, tokenErr.Diagnostic().RecoveryAction())
				assert.Equal(t, role.target, tokenErr.Diagnostic().RecoveryTarget())
				for _, secret := range []string{string(signed), expectedIssuer, expectedAudience, "secret-token-subject", "secret-token-issuer", "secret-token-audience"} {
					assert.NotContains(t, fmt.Sprint(tokenErr, tokenErr.Description(), tokenErr.Details(), tokenErr.ErrorURI(), tokenErr.Diagnostic()), secret)
				}
			})
		}
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
		name       string
		issuer     string
		audience   []string
		expires    time.Time
		wantDetail FailureDetail
	}{
		{"second issuer accepted", secondIssuer, []string{audience}, time.Now().Add(time.Hour), ""},
		{"untrusted issuer rejected", "https://other.example.com", []string{audience}, time.Now().Add(time.Hour), DetailSubjectInvalid},
		{"expired second issuer rejected", secondIssuer, []string{audience}, time.Now().Add(-time.Hour), DetailSubjectExpired},
		{"wrong audience rejected", secondIssuer, []string{"other-audience"}, time.Now().Add(time.Hour), DetailSubjectInvalid},
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
			if tc.wantDetail == "" {
				require.NoError(t, err)
			} else {
				_ = assertOriginDiagnostic(t, err, "invalid_grant", OutcomeAuthenticationFailed, StageSubjectValidation, tc.wantDetail)
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
		_ = assertOriginDiagnostic(t, err, "invalid_grant", OutcomeAuthenticationFailed, StageSubjectValidation, DetailSubjectInvalid)
		assert.Equal(t, 1, countingSet.lookups)
	})
}

func TestJWTValidatorNeverSerializesJOSEHeaders(t *testing.T) {
	t.Parallel()
	key, _ := newTestKeyPairWithID(t, "secret-jose-kid")
	_, trustedSet := newTestKeyPair(t)
	headers := jws.NewHeaders()
	require.NoError(t, headers.Set(jws.TypeKey, "secret-jose-typ"))
	require.NoError(t, headers.Set(jws.ContentTypeKey, "secret-jose-cty"))
	token, err := jwt.NewBuilder().Subject("secret-claim-sub").Issuer("https://secret-claim-issuer.example/private").Audience([]string{"secret-claim-aud"}).Expiration(time.Now().Add(time.Hour)).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), key, jws.WithProtectedHeaders(headers)))
	require.NoError(t, err)
	validator, err := NewJWTValidator(&MockJWKSProvider{keySet: trustedSet}, "https://secret-config-issuer.example/private", "secret-config-aud", 60)
	require.NoError(t, err)
	for _, role := range []struct {
		name     string
		code     string
		stage    FailureStage
		detail   FailureDetail
		validate func(*JWTValidator, context.Context, string) (jwt.Token, error)
	}{
		{"subject", "invalid_grant", StageSubjectValidation, DetailSubjectInvalid, (*JWTValidator).ValidateSubjectToken},
		{"client", "invalid_client", StageClientValidation, DetailClientInvalid, (*JWTValidator).ValidateClientAssertion},
	} {
		t.Run(role.name, func(t *testing.T) {
			t.Parallel()
			_, err := role.validate(validator, context.Background(), string(signed))
			tokenErr := assertOriginDiagnostic(t, err, role.code, OutcomeAuthenticationFailed, role.stage, role.detail)
			for _, secret := range []string{string(signed), "secret-jose-kid", "secret-jose-typ", "secret-jose-cty", "secret-claim", "secret-config"} {
				assert.NotContains(t, fmt.Sprint(tokenErr, tokenErr.Description(), tokenErr.Details(), tokenErr.ErrorURI(), tokenErr.Diagnostic()), secret)
			}
		})
	}
}
