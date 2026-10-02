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
	"net/url"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/httpctx"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/impersonation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type handlerJWKSProvider struct{ set jwk.Set }

func (p handlerJWKSProvider) GetKeySet(context.Context) (jwk.Set, error) { return p.set, nil }
func (handlerJWKSProvider) GetKey(context.Context, string) (jwk.Key, error) {
	return nil, errors.New("key not found")
}

type handlerAgentRepository struct {
	ports.AgentRepository
	agent *storage.Agent
}

func (r handlerAgentRepository) Get(context.Context, id.AgentID) (*storage.Agent, error) {
	if r.agent == nil {
		return nil, ports.ErrNotFound
	}
	return r.agent, nil
}

func (r handlerAgentRepository) GetByCanonicalID(_ context.Context, canonicalID string) (*storage.Agent, error) {
	if r.agent == nil || r.agent.CanonicalID == nil || *r.agent.CanonicalID != canonicalID {
		return nil, ports.ErrNotFound
	}
	return r.agent, nil
}

type handlerIssuer struct {
	called bool
	input  ports.ImpersonationMintInput
}

func (i *handlerIssuer) IssueImpersonationToken(_ context.Context, input ports.ImpersonationMintInput) (string, error) {
	i.called = true
	i.input = input
	return "minted", nil
}

type handlerDelegationVerifier struct{}

func (handlerDelegationVerifier) VerifyUserDelegation(context.Context, id.Principal, id.AgentID, time.Time) (ports.UserDelegationDecision, error) {
	return ports.UserDelegationDecision{Status: ports.UserDelegationActive, GrantID: id.NewGrantID()}, nil
}

func newImpersonationHeaderHandler(t *testing.T) (*OAuth2TokenHandler, *handlerIssuer, *storage.Agent) {
	t.Helper()
	clientID := id.NewClientID("target-client")
	target := &storage.Agent{ID: id.NewAgentID(), ClientID: &clientID, DisplayName: "Target", Description: "Target agent"}
	issuer := &handlerIssuer{}
	service, err := impersonation.NewService(&ports.ImpersonationConfig{
		AudiencePrefix: "https://broker.example.com/impersonation",
		Rules: []ports.ImpersonationRuleConfig{{
			Name: "rule",
			Roles: map[string]ports.ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "aud", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "aud", PrincipalExpression: "actor_token.sub"},
				"subject":          {ExpectedAudience: "aud", PrincipalExpression: "subject_token.sub"},
			},
			TrustedIssuers: []ports.TrustedTokenIssuerConfig{{IssuerURI: "https://issuer.example.com", AllowedAlgorithms: []string{"ES256"}, SignsRoles: []string{"client_assertion", "actor", "subject"}}},
			Authorization:  ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}},
		}},
	}, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return handlerJWKSProvider{}, nil
	}, handlerAgentRepository{agent: target}, issuer, 0, nil, handlerDelegationVerifier{}, "https://broker.example.com", testAuthorizationClock{now: time.Now()})
	require.NoError(t, err)
	return &OAuth2TokenHandler{Impersonation: service}, issuer, target
}

func newScopedImpersonationHandler(t *testing.T) (*OAuth2TokenHandler, *handlerIssuer, *storage.Agent, jwk.Key) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signingKey, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, signingKey.Set(jwk.KeyIDKey, "es256"))
	require.NoError(t, signingKey.Set(jwk.AlgorithmKey, jwa.ES256()))
	publicKey, err := signingKey.PublicKey()
	require.NoError(t, err)
	keySet := jwk.NewSet()
	require.NoError(t, keySet.AddKey(publicKey))

	clientID := id.NewClientID("target-client")
	target := &storage.Agent{ID: id.NewAgentID(), ClientID: &clientID, DisplayName: "Target", Description: "Target agent", AllowedScopes: []string{"read", "write"}}
	issuer := &handlerIssuer{}
	service, err := impersonation.NewService(&ports.ImpersonationConfig{
		AudiencePrefix: "https://broker.example.com/impersonation",
		Rules: []ports.ImpersonationRuleConfig{{
			Name: "rule",
			Roles: map[string]ports.ImpersonationRoleConfig{
				"client_assertion": {ExpectedAudience: "aud", PrincipalExpression: "client_assertion.sub"},
				"actor":            {ExpectedAudience: "aud", PrincipalExpression: "actor_token.sub"},
				"subject":          {ExpectedAudience: "aud", PrincipalExpression: "subject_token.sub"},
			},
			TrustedIssuers: []ports.TrustedTokenIssuerConfig{{IssuerURI: "https://issuer.example.com", AllowedAlgorithms: []string{"ES256"}, SignsRoles: []string{"client_assertion", "actor", "subject"}}},
			Authorization:  ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}},
		}},
	}, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
		return handlerJWKSProvider{set: keySet}, nil
	}, handlerAgentRepository{agent: target}, issuer, 0, nil, handlerDelegationVerifier{}, "https://broker.example.com", testAuthorizationClock{now: time.Now()})
	require.NoError(t, err)
	return &OAuth2TokenHandler{Impersonation: service}, issuer, target, signingKey
}

func signedImpersonationForm(t *testing.T, target *storage.Agent, signingKey jwk.Key, scope string) url.Values {
	t.Helper()
	credential, err := jwt.NewBuilder().Issuer("https://issuer.example.com").Audience([]string{"aud"}).Subject("subject").Expiration(time.Now().Add(time.Hour)).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(credential, jwt.WithKey(jwa.ES256(), signingKey))
	require.NoError(t, err)
	form := url.Values{
		"grant_type":            {tokenexchange.TokenExchangeGrantType},
		"audience":              {"https://broker.example.com/impersonation/" + target.ID.String()},
		"client_assertion_type": {tokenexchange.JWTBearerType},
		"client_assertion":      {string(signed)},
		"actor_token_type":      {impersonation.JWTTokenType},
		"actor_token":           {string(signed)},
		"subject_token_type":    {impersonation.JWTTokenType},
		"subject_token":         {string(signed)},
	}
	if scope != "" {
		form.Set("scope", scope)
	}
	return form
}

func serveImpersonationForm(handler *OAuth2TokenHandler, form url.Values) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/oauth2/token", bytes.NewBufferString(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
func TestHandleImpersonation_TargetScopes(t *testing.T) {
	t.Run("allowed scope is serialized", func(t *testing.T) {
		handler, issuer, target, signingKey := newScopedImpersonationHandler(t)
		response := serveImpersonationForm(handler, signedImpersonationForm(t, target, signingKey, "read"))

		require.Equal(t, http.StatusOK, response.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, "read", body["scope"])
		assert.NotEmpty(t, body["access_token"])
		assert.Equal(t, []string{"read"}, issuer.input.Scopes)
	})

	t.Run("rejected scope emits no token or audit scope", func(t *testing.T) {
		handler, issuer, target, signingKey := newScopedImpersonationHandler(t)
		var logBuffer bytes.Buffer
		handler.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))
		response := serveImpersonationForm(handler, signedImpersonationForm(t, target, signingKey, "admin"))

		require.Equal(t, http.StatusBadRequest, response.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.Equal(t, tokenexchange.InvalidScopeError, body["error"])
		assert.NotContains(t, body, "access_token")
		assert.NotContains(t, body, "scope")
		assert.False(t, issuer.called)
		var event map[string]any
		require.NoError(t, json.Unmarshal(logBuffer.Bytes(), &event))
		assert.NotContains(t, event, "scope")
	})

	t.Run("absent scope omits response field", func(t *testing.T) {
		handler, _, target, signingKey := newScopedImpersonationHandler(t)
		response := serveImpersonationForm(handler, signedImpersonationForm(t, target, signingKey, ""))

		require.Equal(t, http.StatusOK, response.Code)
		var body map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &body))
		assert.NotContains(t, body, "scope")
	})
}

// The impersonation audit event MUST carry only whitelisted, non-secret fields (FR-012, SC-005)
// and MUST correlate with the request ID set by the audit middleware.
func TestLogImpersonationDecision_WhitelistAndCorrelation(t *testing.T) {
	var buf bytes.Buffer
	handler := &OAuth2TokenHandler{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	ctx := httpctx.WithRequestID(context.Background(), "req-123")
	handler.logImpersonationDecision(ctx, impersonation.AuditRecord{
		Outcome:                  "success",
		Audience:                 "https://broker/impersonation",
		TargetAgentID:            "550e8400-e29b-41d4-a716-446655440000",
		SelectedRule:             "internal-gateway",
		IssuerIdentifiers:        []string{"https://idp.example.com"},
		IssuerRoles:              []string{"client_assertion", "actor", "subject"},
		PrivilegedClientIdentity: "gateway-prod",
		ActorIdentity:            "actor-1",
		SubjectIdentity:          "user-1",
	})

	var event map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &event))
	assert.Equal(t, "impersonation_decision", event["msg"])
	assert.Equal(t, "impersonation_decision", event["event"])
	assert.Equal(t, "success", event["outcome"])
	assert.Equal(t, "internal-gateway", event["rule"])
	assert.Equal(t, "gateway-prod", event["privileged_client_identity"])
	assert.Equal(t, "550e8400-e29b-41d4-a716-446655440000", event["target_agent_id"])
	assert.Equal(t, "req-123", event["request_id"])

	for _, forbidden := range []string{"client_assertion", "actor_token", "subject_token", "access_token", "signing_key", "token"} {
		_, present := event[forbidden]
		assert.Falsef(t, present, "audit event must not contain %q", forbidden)
	}
}

func TestLogImpersonationDecision_FailureOmitsUnavailableIdentities(t *testing.T) {
	var buf bytes.Buffer
	handler := &OAuth2TokenHandler{Logger: slog.New(slog.NewJSONHandler(&buf, nil))}

	handler.logImpersonationDecision(context.Background(), impersonation.AuditRecord{
		Outcome:         "invalid_client",
		Audience:        "https://broker/impersonation",
		OAuthErrorCode:  "invalid_client",
		FailureCategory: "client_assertion_invalid",
	})

	var event map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &event))
	assert.Equal(t, "invalid_client", event["outcome"])
	assert.Equal(t, "invalid_client", event["oauth_error_code"])
	_, hasSubject := event["subject_identity"]
	assert.False(t, hasSubject, "no subject identity is available on early failure")
}

func TestHandleImpersonation_SetsNoStoreHeaders(t *testing.T) {
	handler, issuer, target := newImpersonationHeaderHandler(t)
	var logBuffer bytes.Buffer
	handler.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))
	form := url.Values{
		"grant_type": {tokenexchange.TokenExchangeGrantType},
		"audience":   {"https://broker.example.com/impersonation/" + target.ID.String()},
		"resource":   {"https://api.example.com"},
	}
	req := httptest.NewRequest(http.MethodPost, "/oauth2/token", bytes.NewBufferString(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.ServeHTTP(response, req)

	assert.Equal(t, http.StatusBadRequest, response.Code)
	assert.Equal(t, "no-store", response.Header().Get("Cache-Control"))
	assert.Equal(t, "no-cache", response.Header().Get("Pragma"))
	assert.Contains(t, response.Body.String(), `"error":"invalid_request"`)
	assert.False(t, issuer.called)
	var event map[string]any
	require.NoError(t, json.Unmarshal(logBuffer.Bytes(), &event))
	assert.Equal(t, target.ID.String(), event["target_agent_id"])
}

func TestHandleTokenExchange_RejectsInvalidTargets(t *testing.T) {
	handler, _, target := newImpersonationHeaderHandler(t)
	for _, tc := range []struct {
		name     string
		audience string
		code     string
	}{
		{"bare prefix", "https://broker.example.com/impersonation", tokenexchange.InvalidRequestError},
		{"unknown target", "https://broker.example.com/impersonation/" + target.ID.String(), tokenexchange.InvalidTargetError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "unknown target" {
				handler, _, _ = newImpersonationHeaderHandler(t)
				handler.Impersonation, _ = impersonation.NewService(&ports.ImpersonationConfig{
					AudiencePrefix: "https://broker.example.com/impersonation",
					Rules:          []ports.ImpersonationRuleConfig{{Name: "rule", Roles: map[string]ports.ImpersonationRoleConfig{"client_assertion": {ExpectedAudience: "aud", PrincipalExpression: "client_assertion.sub"}, "actor": {ExpectedAudience: "aud", PrincipalExpression: "actor_token.sub"}, "subject": {ExpectedAudience: "aud", PrincipalExpression: "subject_token.sub"}}, TrustedIssuers: []ports.TrustedTokenIssuerConfig{{IssuerURI: "https://issuer.example.com", AllowedAlgorithms: []string{"ES256"}, SignsRoles: []string{"client_assertion", "actor", "subject"}}}, Authorization: ports.AuthorizationConfig{Type: "cel", CEL: ports.CELAuthorizationConfig{Expression: "true"}}}},
				}, func(ports.TrustedTokenIssuerConfig) (tokenexchange.JWKSProvider, error) {
					return handlerJWKSProvider{}, nil
				}, handlerAgentRepository{}, &handlerIssuer{}, 0, nil, handlerDelegationVerifier{}, "https://broker.example.com", testAuthorizationClock{now: time.Now()})
				require.NotNil(t, handler.Impersonation)
			}
			form := url.Values{"grant_type": {tokenexchange.TokenExchangeGrantType}, "audience": {tc.audience}}
			req := httptest.NewRequest(http.MethodPost, "/oauth2/token", bytes.NewBufferString(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, req)
			assert.Equal(t, http.StatusBadRequest, response.Code)
			assert.Contains(t, response.Body.String(), `"error":"`+tc.code+`"`)
		})
	}
}
