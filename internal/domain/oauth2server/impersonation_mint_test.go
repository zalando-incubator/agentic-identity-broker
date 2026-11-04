package oauth2server

import (
	"context"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/ory/fosite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func audClaimContains(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []string:
		for _, item := range v {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func testImpersonationTarget() *storage.Agent {
	clientID := id.NewClientID("target-client")
	return &storage.Agent{ID: id.NewAgentID(), ClientID: &clientID, DisplayName: "Target", Description: "Target agent"}
}

func TestGenerateImpersonationToken(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	ctx := context.Background()
	key, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
	require.NoError(t, err)

	eval, err := NewTokenClaimsEvaluator(`{"email": principal.email, "aud": "https://policy.example.com", "cel_claim": "present", "act": "policy-actor"}`)
	require.NoError(t, err)
	strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, eval, testSlogger())
	require.NoError(t, err)

	email := "user@example.com"
	target := testImpersonationTarget()
	normalSession := &fosite.DefaultSession{Subject: "user-1"}
	setSessionProfile(normalSession, &email, "")
	normalRequest := fosite.NewAccessRequest(normalSession)
	normalRequest.Client = &publicClient{
		clientID:   target.ID.String(),
		agent:      target,
		grantTypes: fosite.Arguments{tokenexchange.TokenExchangeGrantType},
	}
	normalRequest.GrantTypes = fosite.Arguments{tokenexchange.TokenExchangeGrantType}
	normalRequest.RequestedScope = fosite.Arguments{"read"}
	normalRequest.GrantedScope = fosite.Arguments{"read"}
	normalToken, _, err := strategy.GenerateAccessToken(ctx, normalRequest)
	require.NoError(t, err)

	input := ports.ImpersonationMintInput{
		Subject:     "user-1",
		Email:       &email,
		Actor:       "actor-1",
		ActorIssuer: "https://actor-idp.example.com",
		TargetAgent: target,
		Scopes:      []string{"read"},
	}
	impersonationToken, err := strategy.GenerateImpersonationToken(ctx, input)
	require.NoError(t, err)

	jwks, err := svc.BuildJWKS(ctx)
	require.NoError(t, err)
	for _, tokenString := range []string{normalToken, impersonationToken} {
		token, err := jwt.Parse([]byte(tokenString), jwt.WithKeySet(jwks))
		require.NoError(t, err)
		message, err := jws.Parse([]byte(tokenString))
		require.NoError(t, err)
		require.Len(t, message.Signatures(), 1)
		kid, ok := message.Signatures()[0].ProtectedHeaders().KeyID()
		require.True(t, ok)
		assert.Equal(t, key.KID.String(), kid)

		issuer, ok := token.Issuer()
		require.True(t, ok)
		assert.Equal(t, "https://issuer.example.com", issuer)
		subject, ok := token.Subject()
		require.True(t, ok)
		assert.Equal(t, input.Subject, subject)
		agentID, err := jwt.Get[string](token, "agent_id")
		require.NoError(t, err)
		scope, err := jwt.Get[string](token, "scope")
		require.NoError(t, err)
		gotEmail, err := jwt.Get[string](token, "email")
		require.NoError(t, err)
		celClaim, err := jwt.Get[string](token, "cel_claim")
		require.NoError(t, err)
		assert.Equal(t, target.ID.String(), agentID)
		assert.Equal(t, "read", scope)
		assert.Equal(t, email, gotEmail)
		assert.Equal(t, "present", celClaim)
		audience, err := jwt.Get[any](token, "aud")
		require.NoError(t, err)
		assert.True(t, audClaimContains(audience, "https://policy.example.com"))
		issuedAt, ok := token.IssuedAt()
		require.True(t, ok)
		assert.False(t, issuedAt.IsZero())
		expiresAt, ok := token.Expiration()
		require.True(t, ok)
		assert.True(t, expiresAt.After(time.Now()))
		jti, ok := token.JwtID()
		require.True(t, ok)
		assert.NotEmpty(t, jti)
	}

	normalClaims, err := jwt.Parse([]byte(normalToken), jwt.WithKeySet(jwks))
	require.NoError(t, err)
	impersonationClaims, err := jwt.Parse([]byte(impersonationToken), jwt.WithKeySet(jwks))
	require.NoError(t, err)
	normalAct, err := jwt.Get[string](normalClaims, "act")
	require.NoError(t, err)
	impersonationAct, err := jwt.Get[map[string]any](impersonationClaims, "act")
	require.NoError(t, err)
	assert.Equal(t, "policy-actor", normalAct)
	assert.Equal(t, input.Actor, impersonationAct["sub"])
	assert.Equal(t, input.ActorIssuer, impersonationAct["iss"])
}

func TestGenerateImpersonationToken_NoScopeRetainsEmptyJWTClaim(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	ctx := context.Background()
	_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
	require.NoError(t, err)
	strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, nil, testSlogger())
	require.NoError(t, err)

	tokenString, err := strategy.GenerateImpersonationToken(ctx, ports.ImpersonationMintInput{
		Subject:     "user-2",
		Actor:       "actor-2",
		TargetAgent: testImpersonationTarget(),
	})
	require.NoError(t, err)
	claims, err := jwt.ParseInsecure([]byte(tokenString))
	require.NoError(t, err)
	scope, err := jwt.Get[string](claims, "scope")
	require.NoError(t, err)
	assert.Empty(t, scope)
}

func TestGenerateAgentSubjectTokenClaims(t *testing.T) {
	svc, _ := newStrategyTestSigningKeyService()
	ctx := context.Background()
	_, err := svc.generateAndStore(ctx, "ES256", true, time.Now())
	require.NoError(t, err)
	target := testImpersonationTarget()
	for _, tc := range []struct {
		name, policy string
		email        *string
	}{
		{name: "no policy"},
		{name: "no email policy", policy: `{"aud": "https://policy.example.com"}`},
		{name: "empty principal email", policy: `{"email": principal.email}`, email: func() *string { s := ""; return &s }()},
		{name: "literal policy email", policy: `{"email": "policy@example.com"}`, email: func() *string { s := "policy@example.com"; return &s }()},
		{name: "reserved claims", policy: `{"sub": "wrong", "agent_id": "wrong", "act": "wrong"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var evaluator *TokenClaimsEvaluator
			if tc.policy != "" {
				evaluator, err = NewTokenClaimsEvaluator(tc.policy)
				require.NoError(t, err)
			}
			strategy, err := NewJWXAccessTokenStrategy(svc, "https://issuer.example.com", time.Hour, evaluator, testSlogger())
			require.NoError(t, err)
			tokenString, err := strategy.GenerateImpersonationToken(ctx, ports.ImpersonationMintInput{
				Subject: target.ID.String(), TargetAgent: target, Actor: "actor", ActorIssuer: "https://actor.example.com", Scopes: []string{"read"},
			})
			require.NoError(t, err)
			jwks, err := svc.BuildJWKS(ctx)
			require.NoError(t, err)
			claims, err := jwt.Parse([]byte(tokenString), jwt.WithKeySet(jwks))
			require.NoError(t, err)
			subject, _ := claims.Subject()
			require.Equal(t, target.ID.String(), subject)
			agent, err := jwt.Get[string](claims, "agent_id")
			require.NoError(t, err)
			require.Equal(t, subject, agent)
			require.False(t, claims.Has("subject_category"))
			if tc.email == nil {
				require.False(t, claims.Has("email"))
			} else {
				email, err := jwt.Get[string](claims, "email")
				require.NoError(t, err)
				require.Equal(t, *tc.email, email)
			}
			actor, err := jwt.Get[map[string]any](claims, "act")
			require.NoError(t, err)
			require.Equal(t, "actor", actor["sub"])
			issued, _ := claims.IssuedAt()
			expires, _ := claims.Expiration()
			require.Equal(t, time.Hour, expires.Sub(issued))
			scope, err := jwt.Get[string](claims, "scope")
			require.NoError(t, err)
			require.Equal(t, "read", scope)
		})
	}
}

func TestGenerateImpersonationToken_RequiresTargetAgent(t *testing.T) {
	strategy, err := NewJWXAccessTokenStrategy(nil, "https://issuer.example.com", time.Hour, nil, testSlogger())
	require.NoError(t, err)
	_, err = strategy.GenerateImpersonationToken(context.Background(), ports.ImpersonationMintInput{})
	require.EqualError(t, err, "impersonation target agent is required")
}
