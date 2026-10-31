package middleware

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

const (
	testApprovalIssuer   = "https://issuer.example.com"
	testApprovalAudience = "agentic-identity-broker"
	testApprovalKeyID    = "approval-auth-test-key"
)

type staticJWKSProvider struct {
	keySet jwk.Set
}

func (p *staticJWKSProvider) GetKeySet(_ context.Context) (jwk.Set, error) {
	return p.keySet, nil
}

func (p *staticJWKSProvider) GetKey(_ context.Context, kid string) (jwk.Key, error) {
	key, ok := p.keySet.LookupKeyID(kid)
	if !ok {
		return nil, tokenexchange.NewInvalidClientError("key not found")
	}
	return key, nil
}

func newApprovalRequestAuthenticatorForTest(t *testing.T, authorizationExpression string) (*ApprovalRequestAuthenticator, *rsa.PrivateKey) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	publicKey, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, publicKey.Set(jwk.KeyIDKey, testApprovalKeyID))
	require.NoError(t, publicKey.Set(jwk.AlgorithmKey, jwa.RS256()))

	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(publicKey))

	jwtValidator, err := tokenexchange.NewJWTValidatorWithPolicies(
		tokenexchange.JWTValidationPolicy{
			JWKSProvider:    &staticJWKSProvider{keySet: keySet},
			ExpectedIssuers: []string{testApprovalIssuer},
		},
		tokenexchange.JWTValidationPolicy{
			JWKSProvider:    &staticJWKSProvider{keySet: keySet},
			ExpectedIssuers: []string{testApprovalIssuer},
		},
		testApprovalAudience,
		tokenexchange.DefaultClockSkewTolerance,
	)
	require.NoError(t, err)

	celEvaluator, err := tokenexchange.NewCELEvaluator(tokenexchange.CELEvaluatorConfig{
		PrincipalExpression:     "subject_token.sub",
		AgentIDExpression:       "subject_token.azp",
		AuthorizationExpression: authorizationExpression,
		EvaluationTimeout:       time.Second,
	})
	require.NoError(t, err)

	return NewApprovalRequestAuthenticator(jwtValidator, celEvaluator), privateKey
}

func signApprovalJWT(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()

	tok := jwt.New()
	for key, value := range claims {
		require.NoError(t, tok.Set(key, value))
	}

	jwkKey, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, jwkKey.Set(jwk.KeyIDKey, testApprovalKeyID))

	signed, err := jwt.Sign(tok, jwt.WithKey(jwa.RS256(), jwkKey))
	require.NoError(t, err)
	return string(signed)
}

func validApprovalJWTClaims(subject string) map[string]any {
	now := time.Now()
	return map[string]any{
		"sub": subject,
		"iss": testApprovalIssuer,
		"aud": testApprovalAudience,
		"iat": now.Unix(),
		"exp": now.Add(time.Hour).Unix(),
	}
}

func decodeApprovalAuthError(t *testing.T, body *httptest.ResponseRecorder) map[string]string {
	t.Helper()

	var resp map[string]string
	require.NoError(t, json.NewDecoder(body.Body).Decode(&resp))
	return resp
}

func TestRequireApprovalSubjectTokenAndClientAssertion(t *testing.T) {
	t.Run("rejects missing subject token", func(t *testing.T) {
		auth, _ := newApprovalRequestAuthenticatorForTest(t, "true")
		handler := RequireApprovalSubjectTokenAndClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "unauthorized", resp["error"])
		assert.Equal(t, "missing subject token", resp["message"])
	})

	t.Run("rejects invalid client assertion", func(t *testing.T) {
		auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "true")
		agentID := id.NewAgentID()
		subjectClaims := validApprovalJWTClaims("alice@example.com")
		subjectClaims["azp"] = agentID.String()
		subjectToken := signApprovalJWT(t, privateKey, subjectClaims)

		handler := RequireApprovalSubjectTokenAndClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals", nil)
		req.Header.Set("Authorization", "Bearer "+subjectToken)
		req.Header.Set(ApprovalClientAssertionHeaderName, "not-a-jwt")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "unauthorized", resp["error"])
		assert.Equal(t, "invalid client assertion", resp["message"])
	})

	t.Run("sets principal and approval auth context", func(t *testing.T) {
		auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "request.grant_type == 'approval_create'")
		agentID := id.NewAgentID()
		subjectClaims := validApprovalJWTClaims("alice@example.com")
		subjectClaims["azp"] = agentID.String()
		subjectToken := signApprovalJWT(t, privateKey, subjectClaims)
		clientAssertion := signApprovalJWT(t, privateKey, validApprovalJWTClaims("gateway-client"))

		var capturedPrincipal string
		var capturedAuth ApprovalAuthContext
		var authContextOK bool
		handler := RequireApprovalSubjectTokenAndClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPrincipal, _ = principal.FromContext(r.Context())
			capturedAuth, authContextOK = ApprovalAuthFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals", nil)
		req.Header.Set("Authorization", "Bearer "+subjectToken)
		req.Header.Set(ApprovalClientAssertionHeaderName, clientAssertion)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "alice@example.com", capturedPrincipal)
		assert.True(t, authContextOK)
		assert.Equal(t, id.Principal("alice@example.com"), capturedAuth.Principal)
		assert.Equal(t, agentID, capturedAuth.AgentID)
		assert.Equal(t, "gateway-client", capturedAuth.GatewayClientID)
	})

	t.Run("returns 503 when validation dependencies are unavailable", func(t *testing.T) {
		handler := RequireApprovalSubjectTokenAndClientAssertion(NewApprovalRequestAuthenticator(nil, nil))(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "service_unavailable", resp["error"])
		assert.Equal(t, "approval subject token validation is not configured", resp["message"])
	})
}

func TestRequireApprovalClientAssertion(t *testing.T) {
	t.Run("rejects missing client assertion", func(t *testing.T) {
		auth, _ := newApprovalRequestAuthenticatorForTest(t, "true")
		handler := RequireApprovalClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "unauthorized", resp["error"])
		assert.Equal(t, "missing client assertion", resp["message"])
	})

	t.Run("rejects invalid client assertion", func(t *testing.T) {
		auth, _ := newApprovalRequestAuthenticatorForTest(t, "true")
		handler := RequireApprovalClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
		req.Header.Set("Authorization", "Bearer not-a-jwt")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "unauthorized", resp["error"])
		assert.Equal(t, "invalid client assertion", resp["message"])
	})

	t.Run("sets gateway client id and preserves principal-free context", func(t *testing.T) {
		auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "request.grant_type == 'approval_sync' && request.principal == 'alice@example.com'")
		clientAssertion := signApprovalJWT(t, privateKey, validApprovalJWTClaims("gateway-client"))

		var capturedAuth ApprovalAuthContext
		var authContextOK bool
		var principalOK bool
		handler := RequireApprovalClientAssertion(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, principalOK = principal.FromContext(r.Context())
			capturedAuth, authContextOK = ApprovalAuthFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodGet, "/api/approvals?principal=alice@example.com", nil)
		req.Header.Set("Authorization", "Bearer "+clientAssertion)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
		assert.False(t, principalOK)
		assert.True(t, authContextOK)
		assert.Equal(t, "gateway-client", capturedAuth.GatewayClientID)
	})
}

func TestRequireApprovalSubjectToken(t *testing.T) {
	t.Run("rejects missing subject token", func(t *testing.T) {
		auth, _ := newApprovalRequestAuthenticatorForTest(t, "true")
		handler := RequireApprovalSubjectToken(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals/id/consume", nil)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.Equal(t, http.StatusUnauthorized, rec.Code)
		resp := decodeApprovalAuthError(t, rec)
		assert.Equal(t, "unauthorized", resp["error"])
		assert.Equal(t, "missing subject token", resp["message"])
	})

	t.Run("sets principal context from subject token", func(t *testing.T) {
		auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "true")
		agentID := id.NewAgentID()
		subjectClaims := validApprovalJWTClaims("alice@example.com")
		subjectClaims["azp"] = agentID.String()
		subjectToken := signApprovalJWT(t, privateKey, subjectClaims)

		var capturedPrincipal string
		var capturedAuth ApprovalAuthContext
		var authContextOK bool
		handler := RequireApprovalSubjectToken(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			capturedPrincipal, _ = principal.FromContext(r.Context())
			capturedAuth, authContextOK = ApprovalAuthFromContext(r.Context())
			w.WriteHeader(http.StatusNoContent)
		}))

		req := httptest.NewRequest(http.MethodPost, "/api/approvals/id/consume", nil)
		req.Header.Set("Authorization", "Bearer "+subjectToken)
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		require.Equal(t, http.StatusNoContent, rec.Code)
		assert.Equal(t, "alice@example.com", capturedPrincipal)
		assert.True(t, authContextOK)
		assert.Equal(t, id.Principal("alice@example.com"), capturedAuth.Principal)
		assert.True(t, capturedAuth.AgentID.IsZero())
		assert.Empty(t, capturedAuth.GatewayClientID)
	})
}

// ADR 045 decision 6a: agent-subject tokens never authenticate as a human approval principal.
func TestApprovalAuthRejectsAgentSubjectTokens(t *testing.T) {
	agentID := id.NewAgentID()
	agentClaims := validApprovalJWTClaims(agentID.String())
	agentClaims["agent_id"] = agentID.String()
	agentClaims["azp"] = agentID.String()

	for _, tc := range []struct {
		name       string
		middleware func(*ApprovalRequestAuthenticator) func(http.Handler) http.Handler
		path       string
		withAssert bool
	}{
		{"create", RequireApprovalSubjectTokenAndClientAssertion, "/api/approvals", true},
		{"consume", RequireApprovalSubjectToken, "/api/approvals/id/consume", false},
	} {
		t.Run(tc.name+" rejects detected agent-subject token", func(t *testing.T) {
			auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "true")
			called := false
			handler := tc.middleware(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodPost, tc.path, nil)
			req.Header.Set("Authorization", "Bearer "+signApprovalJWT(t, privateKey, agentClaims))
			if tc.withAssert {
				req.Header.Set(ApprovalClientAssertionHeaderName, signApprovalJWT(t, privateKey, validApprovalJWTClaims("gateway")))
			}
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			assert.False(t, called)
			assert.Equal(t, http.StatusForbidden, rec.Code)
			resp := decodeApprovalAuthError(t, rec)
			assert.Equal(t, "forbidden", resp["error"])
			assert.Equal(t, "agent-subject tokens cannot authenticate approval requests", resp["message"])
		})
	}

	t.Run("detector failure fails closed", func(t *testing.T) {
		auth, privateKey := newApprovalRequestAuthenticatorForTest(t, "true")
		evaluator, err := tokenexchange.NewCELEvaluator(tokenexchange.CELEvaluatorConfig{
			PrincipalExpression: "subject_token.sub", AgentIDExpression: "subject_token.azp",
			AgentSubjectExpression: "subject_token.sub == subject_token.missing", AuthorizationExpression: "true",
		})
		require.NoError(t, err)
		auth.celEvaluator = evaluator
		called := false
		handler := RequireApprovalSubjectToken(auth)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
		}))
		claims := validApprovalJWTClaims("alice@example.com")
		claims["azp"] = agentID.String()
		req := httptest.NewRequest(http.MethodPost, "/api/approvals/id/consume", nil)
		req.Header.Set("Authorization", "Bearer "+signApprovalJWT(t, privateKey, claims))
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		assert.False(t, called)
		assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
		assert.Equal(t, "service_unavailable", decodeApprovalAuthError(t, rec)["error"])
	})
}
