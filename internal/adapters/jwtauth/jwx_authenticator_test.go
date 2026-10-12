package jwtauth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/jwtauth"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(&discardWriter{}, nil))
}

type discardWriter struct{}

func (d *discardWriter) Write(p []byte) (n int, err error) { return len(p), nil }

func generateTestKeyPair(t *testing.T) (*ecdsa.PrivateKey, []byte, jwk.Key) {
	t.Helper()

	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	jwkKey, err := jwk.Import[jwk.Key](privKey)
	require.NoError(t, err)
	err = jwkKey.Set(jwk.KeyIDKey, "test-key-1")
	require.NoError(t, err)
	err = jwkKey.Set(jwk.AlgorithmKey, jwa.ES256())
	require.NoError(t, err)

	pubKey, err := jwkKey.PublicKey()
	require.NoError(t, err)

	set := jwk.NewSet()
	_ = set.AddKey(pubKey)
	jwksBytes, err := json.Marshal(set)
	require.NoError(t, err)

	return privKey, jwksBytes, jwkKey
}

func startMockJWKSServer(t *testing.T, jwksBytes []byte) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(jwksBytes)
	}))
	t.Cleanup(server.Close)
	return server
}

func createSignedJWT(t *testing.T, claims map[string]interface{}, signingKey jwk.Key) string {
	t.Helper()

	builder := jwt.New()
	for k, v := range claims {
		switch k {
		case "sub":
			_ = builder.Set(jwt.SubjectKey, v)
		case "iss":
			_ = builder.Set(jwt.IssuerKey, v)
		case "aud":
			_ = builder.Set(jwt.AudienceKey, v)
		case "exp":
			if ts, ok := v.(time.Time); ok {
				_ = builder.Set(jwt.ExpirationKey, ts)
			}
		case "iat":
			if ts, ok := v.(time.Time); ok {
				_ = builder.Set(jwt.IssuedAtKey, ts)
			}
		default:
			_ = builder.Set(k, v)
		}
	}

	signed, err := jwt.Sign(builder, jwt.WithKey(jwa.ES256(), signingKey))
	require.NoError(t, err)
	return string(signed)
}

func newTestAuthenticator(t *testing.T, jwksURL string, signingKey jwk.Key, jwtConfig *ports.JWTConfig) *JWXAuthenticator {
	t.Helper()

	if jwtConfig == nil {
		jwtConfig = &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "jwks",
			JWKSURI:      jwksURL,
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		}
	}

	celConfig := jwtauth.CELEvaluatorConfig{
		PrincipalExpression:   jwtConfig.ClaimExtraction.PrincipalExpression,
		DisplayNameExpression: jwtConfig.ClaimExtraction.DisplayNameExpression,
		EmailExpression:       jwtConfig.ClaimExtraction.EmailExpression,
		PictureURLExpression:  jwtConfig.ClaimExtraction.PictureURLExpression,
	}
	celEvaluator, err := jwtauth.NewCELEvaluator(celConfig, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig:    jwtConfig,
		CELEvaluator: celEvaluator,
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		Logger:       testLogger(),
	})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, auth.Shutdown(context.Background())) })
	return auth
}

func TestNewJWXAuthenticator_RejectsUnsupportedJWKSKeys(t *testing.T) {
	jwksServer := startMockJWKSServer(t, []byte(`{"keys":[{"kty":"unsupported"}]}`))
	celEvaluator, err := jwtauth.NewCELEvaluator(jwtauth.CELEvaluatorConfig{
		PrincipalExpression: "claims.sub",
	}, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "jwks",
			JWKSURI:      jwksServer.URL,
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		},
		CELEvaluator: celEvaluator,
		HTTPClient:   &http.Client{Timeout: 5 * time.Second},
		Logger:       testLogger(),
	})
	require.Error(t, err)
	assert.Nil(t, auth)
}

func TestJWXAuthenticator_ValidSignedJWT(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, nil)

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
		"iat": time.Now(),
	}, signingKey)

	result, err := auth.Authenticate(context.Background(), "Bearer "+tokenStr)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "alice@example.com", result.Principal)
	assert.NotNil(t, result.Claims)
}

func TestJWXAuthenticator_InvalidSignature(t *testing.T) {
	_, jwksBytes, _ := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	_, _, wrongKey := generateTestKeyPair(t)

	auth := newTestAuthenticator(t, jwksServer.URL, nil, &ports.JWTConfig{
		HeaderName:   "Authorization",
		Verification: "jwks",
		JWKSURI:      jwksServer.URL,
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
	}, wrongKey)

	_, err := auth.Authenticate(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrInvalidSignature)
}

func TestJWXAuthenticator_ExpiredJWT(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, nil)

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(-1 * time.Hour),
		"iat": time.Now().Add(-2 * time.Hour),
	}, signingKey)

	_, err := auth.Authenticate(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrTokenExpired)
}

func TestJWXAuthenticator_WrongAudience(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
		HeaderName:       "Authorization",
		Verification:     "jwks",
		JWKSURI:          jwksServer.URL,
		ExpectedAudience: "my-broker",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
		"aud": []string{"wrong-audience"},
	}, signingKey)

	_, err := auth.Authenticate(context.Background(), "Bearer "+tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrAudienceMismatch)
}

func TestJWXAuthenticator_WrongIssuer(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
		HeaderName:     "Authorization",
		Verification:   "jwks",
		JWKSURI:        jwksServer.URL,
		ExpectedIssuer: "https://trusted-issuer.example.com",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"iss": "https://wrong-issuer.example.com",
		"exp": time.Now().Add(1 * time.Hour),
	}, signingKey)

	_, err := auth.Authenticate(context.Background(), "Bearer "+tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrIssuerMismatch)
}

func TestJWXAuthenticator_MissingExpiry(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, nil)

	// Create a JWT without exp claim
	builder := jwt.New()
	_ = builder.Set(jwt.SubjectKey, "alice@example.com")
	signed, err := jwt.Sign(builder, jwt.WithKey(jwa.ES256(), signingKey))
	require.NoError(t, err)

	_, err = auth.Authenticate(context.Background(), string(signed))
	require.Error(t, err)
	// Should be ErrMissingExpiry
	errStr := err.Error()
	hasMissingExpiry := strings.Contains(errStr, "missing exp") || strings.Contains(errStr, "exp claim")
	assert.True(t, hasMissingExpiry, "expected error about missing exp, got: %v", err)
}

func TestJWXAuthenticator_MalformedToken(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, nil)

	tests := []struct {
		name  string
		token string
	}{
		{"empty token", ""},
		{"random string", "not-a-jwt"},
		{"incomplete parts", "header.payload"},
		{"just dots", "..."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.Authenticate(context.Background(), tt.token)
			require.Error(t, err)
			assert.ErrorIs(t, err, jwtauth.ErrMalformedToken)
		})
	}
}

func TestJWXAuthenticator_BearerPrefixStripping(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	t.Run("strips Bearer prefix for Authorization header", func(t *testing.T) {
		auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "jwks",
			JWKSURI:      jwksServer.URL,
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		})

		tokenStr := createSignedJWT(t, map[string]interface{}{
			"sub": "alice@example.com",
			"exp": time.Now().Add(1 * time.Hour),
		}, signingKey)

		result, err := auth.Authenticate(context.Background(), "Bearer "+tokenStr)
		require.NoError(t, err)
		assert.Equal(t, "alice@example.com", result.Principal)
	})

	t.Run("does not strip Bearer prefix for custom header", func(t *testing.T) {
		auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
			HeaderName:   "X-JWT-Token",
			Verification: "jwks",
			JWKSURI:      jwksServer.URL,
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		})

		tokenStr := createSignedJWT(t, map[string]interface{}{
			"sub": "alice@example.com",
			"exp": time.Now().Add(1 * time.Hour),
		}, signingKey)

		result, err := auth.Authenticate(context.Background(), tokenStr)
		require.NoError(t, err)
		assert.Equal(t, "alice@example.com", result.Principal)
	})
}

func TestJWXAuthenticator_ProfileExtraction(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
		HeaderName:   "Authorization",
		Verification: "jwks",
		JWKSURI:      jwksServer.URL,
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression:   "claims.sub",
			DisplayNameExpression: "claims.name",
			EmailExpression:       "claims.email",
			PictureURLExpression:  "claims.picture",
		},
	})

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub":     "alice@example.com",
		"name":    "Alice Smith",
		"email":   "alice@corp.com",
		"picture": "https://cdn.example.com/alice.jpg",
		"exp":     time.Now().Add(1 * time.Hour),
	}, signingKey)

	result, err := auth.Authenticate(context.Background(), "Bearer "+tokenStr)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "alice@example.com", result.Principal)
	assert.NotNil(t, result.DisplayName)
	assert.Equal(t, "Alice Smith", *result.DisplayName)
	assert.NotNil(t, result.Email)
	assert.Equal(t, "alice@corp.com", *result.Email)
	assert.NotNil(t, result.PictureURL)
	assert.Equal(t, "https://cdn.example.com/alice.jpg", *result.PictureURL)
}

func TestJWXAuthenticator_ValidAudience(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
		HeaderName:       "Authorization",
		Verification:     "jwks",
		JWKSURI:          jwksServer.URL,
		ExpectedAudience: "my-broker",
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"aud": []string{"my-broker", "other-service"},
		"exp": time.Now().Add(1 * time.Hour),
	}, signingKey)

	result, err := auth.Authenticate(context.Background(), fmt.Sprintf("Bearer %s", tokenStr))
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result.Principal)
}

// --- Unsigned JWT Tests (Phase 5: US2) ---

// createUnsignedJWT creates an unsigned JWT (alg: "none") with the given claims.
// Asserts that the serialized output is a valid unsecured JWT (compact form with empty signature segment).
func createUnsignedJWT(t *testing.T, claims map[string]interface{}) string {
	t.Helper()

	builder := jwt.New()
	for k, v := range claims {
		switch k {
		case "sub":
			err := builder.Set(jwt.SubjectKey, v)
			require.NoError(t, err, "failed to set 'sub' claim")
		case "iss":
			err := builder.Set(jwt.IssuerKey, v)
			require.NoError(t, err, "failed to set 'iss' claim")
		case "aud":
			err := builder.Set(jwt.AudienceKey, v)
			require.NoError(t, err, "failed to set 'aud' claim")
		case "exp":
			ts, ok := v.(time.Time)
			require.True(t, ok, "exp claim must be time.Time, got %T", v)
			err := builder.Set(jwt.ExpirationKey, ts)
			require.NoError(t, err, "failed to set 'exp' claim")
		case "iat":
			ts, ok := v.(time.Time)
			require.True(t, ok, "iat claim must be time.Time, got %T", v)
			err := builder.Set(jwt.IssuedAtKey, ts)
			require.NoError(t, err, "failed to set 'iat' claim")
		default:
			err := builder.Set(k, v)
			require.NoError(t, err, "failed to set '%s' claim", k)
		}
	}

	serialized, err := jwt.Sign(builder, jwt.WithInsecureNoSignature())
	require.NoError(t, err)

	// Verify the output is a valid unsecured JWT: compact form "header.payload." with empty signature
	tokenStr := string(serialized)
	parts := strings.Split(tokenStr, ".")
	require.Len(t, parts, 3, "unsigned JWT must have 3 dot-separated parts")
	assert.Empty(t, parts[2], "unsigned JWT must have an empty signature segment")

	return tokenStr
}

func TestJWXAuthenticator_UnsignedJWTAcceptedWhenVerificationNone(t *testing.T) {
	celConfig := jwtauth.CELEvaluatorConfig{
		PrincipalExpression: "claims.sub",
	}
	celEvaluator, err := jwtauth.NewCELEvaluator(celConfig, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "none",
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		},
		CELEvaluator: celEvaluator,
		Logger:       testLogger(),
	})
	require.NoError(t, err)

	tokenStr := createUnsignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
	})

	result, err := auth.Authenticate(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result.Principal)
}

func TestJWXAuthenticator_UnsignedJWTRejectedWhenVerificationJWKS(t *testing.T) {
	_, jwksBytes, _ := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, nil, &ports.JWTConfig{
		HeaderName:   "Authorization",
		Verification: "jwks",
		JWKSURI:      jwksServer.URL,
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	tokenStr := createUnsignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
	})

	_, err := auth.Authenticate(context.Background(), tokenStr)
	require.Error(t, err, "unsigned JWT should be rejected when verification is jwks")
}

func TestJWXAuthenticator_SignedJWTAcceptedWhenVerificationNone(t *testing.T) {
	_, _, signingKey := generateTestKeyPair(t)

	celConfig := jwtauth.CELEvaluatorConfig{
		PrincipalExpression: "claims.sub",
	}
	celEvaluator, err := jwtauth.NewCELEvaluator(celConfig, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "none",
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		},
		CELEvaluator: celEvaluator,
		Logger:       testLogger(),
	})
	require.NoError(t, err)

	// Create a signed JWT — should still be accepted in "none" mode (no signature check)
	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(1 * time.Hour),
	}, signingKey)

	result, err := auth.Authenticate(context.Background(), tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "alice@example.com", result.Principal)
}

func TestJWXAuthenticator_ExpiredJWTRejectedEvenWhenVerificationNone(t *testing.T) {
	celConfig := jwtauth.CELEvaluatorConfig{
		PrincipalExpression: "claims.sub",
	}
	celEvaluator, err := jwtauth.NewCELEvaluator(celConfig, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "none",
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		},
		CELEvaluator: celEvaluator,
		Logger:       testLogger(),
	})
	require.NoError(t, err)

	tokenStr := createUnsignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
		"exp": time.Now().Add(-1 * time.Hour),
	})

	_, err = auth.Authenticate(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrTokenExpired, "expired JWT should be rejected even in 'none' mode")
}

func TestJWXAuthenticator_NoExpAcceptedWhenVerificationNone(t *testing.T) {
	celConfig := jwtauth.CELEvaluatorConfig{
		PrincipalExpression: "claims.sub",
	}
	celEvaluator, err := jwtauth.NewCELEvaluator(celConfig, testLogger())
	require.NoError(t, err)

	auth, err := NewJWXAuthenticator(JWXAuthenticatorConfig{
		JWTConfig: &ports.JWTConfig{
			HeaderName:   "Authorization",
			Verification: "none",
			ClaimExtraction: ports.JWTClaimExtractionConfig{
				PrincipalExpression: "claims.sub",
			},
		},
		CELEvaluator: celEvaluator,
		Logger:       testLogger(),
	})
	require.NoError(t, err)

	// JWT without exp claim — accepted in "none" mode (exp is optional)
	tokenStr := createUnsignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
	})

	result, err := auth.Authenticate(context.Background(), tokenStr)
	require.NoError(t, err, "JWT without exp should be accepted when verification is none")
	assert.Equal(t, "alice@example.com", result.Principal)
}

func TestJWXAuthenticator_NoExpRejectedWhenVerificationJWKS(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	jwksServer := startMockJWKSServer(t, jwksBytes)

	auth := newTestAuthenticator(t, jwksServer.URL, signingKey, &ports.JWTConfig{
		HeaderName:   "Authorization",
		Verification: "jwks",
		JWKSURI:      jwksServer.URL,
		ClaimExtraction: ports.JWTClaimExtractionConfig{
			PrincipalExpression: "claims.sub",
		},
	})

	// JWT without exp claim — rejected in "jwks" mode (exp is mandatory)
	tokenStr := createSignedJWT(t, map[string]interface{}{
		"sub": "alice@example.com",
	}, signingKey)

	_, err := auth.Authenticate(context.Background(), tokenStr)
	require.Error(t, err)
	assert.ErrorIs(t, err, jwtauth.ErrMissingExpiry, "JWT without exp should be rejected when verification is jwks")
}
