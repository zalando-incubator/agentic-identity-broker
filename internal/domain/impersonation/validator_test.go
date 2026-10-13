package impersonation

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

func compactJWT(t *testing.T, header, payload map[string]any) string {
	t.Helper()
	enc := func(m map[string]any) string {
		b, err := json.Marshal(m)
		require.NoError(t, err)
		return base64.RawURLEncoding.EncodeToString(b)
	}
	return enc(header) + "." + enc(payload) + ".sig"
}

func TestProtectedHeaderAlgorithm(t *testing.T) {
	token := compactJWT(t, map[string]any{"alg": "RS256", "typ": "JWT"}, map[string]any{"sub": "x"})
	alg, err := protectedHeaderAlgorithm(token)
	require.NoError(t, err)
	assert.Equal(t, jwa.RS256(), alg)

	none := compactJWT(t, map[string]any{"alg": "none"}, map[string]any{"sub": "x"})
	alg, err = protectedHeaderAlgorithm(none)
	require.NoError(t, err)
	assert.Equal(t, jwa.NoSignature(), alg)

	_, err = protectedHeaderAlgorithm("not-a-jwt")
	require.Error(t, err)
}

func TestIsApprovedAlgorithm(t *testing.T) {
	for _, ok := range []string{"RS256", "RS384", "RS512", "PS256", "PS384", "PS512", "ES256", "ES384", "ES512", "EdDSA"} {
		assert.True(t, IsApprovedAlgorithm(ok), ok)
	}
	for _, bad := range []string{"none", "HS256", "HS384", "HS512", "", "rs256"} {
		assert.False(t, IsApprovedAlgorithm(bad), bad)
	}
}

func TestUnverifiedIssuer(t *testing.T) {
	token := compactJWT(t, map[string]any{"alg": "RS256"}, map[string]any{"iss": "https://idp.example.com", "sub": "x"})
	iss, err := unverifiedIssuer(token)
	require.NoError(t, err)
	assert.Equal(t, "https://idp.example.com", iss)
}

type configurableJWKSProvider struct {
	set jwk.Set
	err error
}

func (p configurableJWKSProvider) GetKeySet(context.Context) (jwk.Set, error) { return p.set, p.err }
func (p configurableJWKSProvider) GetKey(context.Context, string) (jwk.Key, error) {
	return nil, errors.New("not implemented")
}

func signedValidationKey(t *testing.T, kid string) (jwk.Key, jwk.Set) {
	t.Helper()
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	signingKey, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, signingKey.Set(jwk.KeyIDKey, kid))
	require.NoError(t, signingKey.Set(jwk.AlgorithmKey, jwa.ES256()))
	publicKey, err := signingKey.PublicKey()
	require.NoError(t, err)
	set := jwk.NewSet()
	require.NoError(t, set.AddKey(publicKey))
	return signingKey, set
}

func signedValidationToken(t *testing.T, algorithm jwa.SignatureAlgorithm, signingKey jwk.Key, issuer, audience string, expiration, notBefore time.Time) string {
	t.Helper()
	return signedValidationTokenWithSubject(t, algorithm, signingKey, issuer, audience, "subject", expiration, notBefore)
}

func signedValidationTokenWithSubject(t *testing.T, algorithm jwa.SignatureAlgorithm, signingKey jwk.Key, issuer, audience, subject string, expiration, notBefore time.Time) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(issuer).Audience([]string{audience}).Subject(subject).Expiration(expiration).NotBefore(notBefore).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(algorithm, signingKey))
	require.NoError(t, err)
	return string(signed)
}

func signedValidationTokenWithoutAudience(t *testing.T, algorithm jwa.SignatureAlgorithm, signingKey jwk.Key, issuer string, expiration, notBefore time.Time) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(issuer).Subject("subject").Expiration(expiration).NotBefore(notBefore).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(algorithm, signingKey))
	require.NoError(t, err)
	return string(signed)
}
func signedValidationTokenWithoutExpiration(t *testing.T, algorithm jwa.SignatureAlgorithm, signingKey jwk.Key, issuer, audience string) string {
	t.Helper()
	token, err := jwt.NewBuilder().Issuer(issuer).Audience([]string{audience}).Subject("subject").NotBefore(time.Now().Add(-time.Minute)).Build()
	require.NoError(t, err)
	signed, err := jwt.Sign(token, jwt.WithKey(algorithm, signingKey))
	require.NoError(t, err)
	return string(signed)
}

func TestSignedValidator(t *testing.T) {
	const issuer = "https://issuer.example.com"
	const audience = "https://broker.example.com/impersonation"
	signingKey, keySet := signedValidationKey(t, "es256")
	validator := &signedValidator{jwksProvider: configurableJWKSProvider{set: keySet}, issuerURI: issuer, allowedAlgorithms: map[jwa.SignatureAlgorithm]struct{}{jwa.ES256(): {}}, clockSkew: defaultClockSkew}
	valid := signedValidationToken(t, jwa.ES256(), signingKey, issuer, audience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))

	t.Run("accepts correctly signed token", func(t *testing.T) {
		claims, err := validator.validate(context.Background(), valid, audience)
		require.NoError(t, err)
		assert.Equal(t, "subject", claims["sub"])
	})
	t.Run("requires absent audience", func(t *testing.T) {
		withoutAudience := signedValidationTokenWithoutAudience(t, jwa.ES256(), signingKey, issuer, time.Now().Add(time.Hour), time.Now().Add(-time.Minute))
		claims, err := validator.validateWithoutAudience(context.Background(), withoutAudience)
		require.NoError(t, err)
		assert.Equal(t, "subject", claims["sub"])

		_, err = validator.validateWithoutAudience(context.Background(), valid)
		require.Error(t, err)
	})
	t.Run("rejects signature key mismatch", func(t *testing.T) {
		otherKey, _ := signedValidationKey(t, "other")
		_, err := validator.validate(context.Background(), signedValidationToken(t, jwa.ES256(), otherKey, issuer, audience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute)), audience)
		require.Error(t, err)
	})
	t.Run("rejects permitted-algorithm mismatch", func(t *testing.T) {
		privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
		require.NoError(t, err)
		rsaKey, err := jwk.Import[jwk.Key](privateKey)
		require.NoError(t, err)
		require.NoError(t, rsaKey.Set(jwk.KeyIDKey, "rs256"))
		require.NoError(t, rsaKey.Set(jwk.AlgorithmKey, jwa.RS256()))
		_, err = validator.validate(context.Background(), signedValidationToken(t, jwa.RS256(), rsaKey, issuer, audience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute)), audience)
		require.ErrorContains(t, err, "not permitted for issuer")
	})
	t.Run("rejects non-approved algorithms before verification", func(t *testing.T) {
		for _, algorithm := range []string{"none", "HS256"} {
			t.Run(algorithm, func(t *testing.T) {
				_, err := validator.validate(context.Background(), compactJWT(t, map[string]any{"alg": algorithm}, map[string]any{"iss": issuer}), audience)
				require.ErrorContains(t, err, "not an approved asymmetric algorithm")
			})
		}
	})
	t.Run("rejects issuer mismatch", func(t *testing.T) {
		_, err := validator.validate(context.Background(), signedValidationToken(t, jwa.ES256(), signingKey, "https://other.example.com", audience, time.Now().Add(time.Hour), time.Now().Add(-time.Minute)), audience)
		require.Error(t, err)
	})
	t.Run("rejects audience mismatch", func(t *testing.T) {
		_, err := validator.validate(context.Background(), signedValidationToken(t, jwa.ES256(), signingKey, issuer, "https://other.example.com", time.Now().Add(time.Hour), time.Now().Add(-time.Minute)), audience)
		require.Error(t, err)
	})
	t.Run("rejects expired token", func(t *testing.T) {
		_, err := validator.validate(context.Background(), signedValidationToken(t, jwa.ES256(), signingKey, issuer, audience, time.Now().Add(-2*time.Minute), time.Now().Add(-time.Hour)), audience)
		require.Error(t, err)
	})
	t.Run("rejects token without expiration", func(t *testing.T) {
		_, err := validator.validate(context.Background(), signedValidationTokenWithoutExpiration(t, jwa.ES256(), signingKey, issuer, audience), audience)
		require.Error(t, err)
	})
	t.Run("rejects not-before token", func(t *testing.T) {
		_, err := validator.validate(context.Background(), signedValidationToken(t, jwa.ES256(), signingKey, issuer, audience, time.Now().Add(time.Hour), time.Now().Add(2*time.Minute)), audience)
		require.Error(t, err)
	})
	t.Run("rejects unavailable JWKS", func(t *testing.T) {
		unavailable := &signedValidator{jwksProvider: configurableJWKSProvider{err: errors.New("unavailable")}, issuerURI: issuer, allowedAlgorithms: map[jwa.SignatureAlgorithm]struct{}{jwa.ES256(): {}}, clockSkew: defaultClockSkew}
		_, err := unavailable.validate(context.Background(), valid, audience)
		var exchangeErr *tokenexchange.TokenExchangeError
		require.ErrorAs(t, err, &exchangeErr)
		assert.Equal(t, tokenexchange.DetailJWKSUnavailable, exchangeErr.Diagnostic().Detail())
	})
}

func TestSignedValidator_JWKSFailurePreservesTypedOrigin(t *testing.T) {
	cause := errors.New("JWKS_SECRET_SENTINEL")
	validator := &signedValidator{jwksProvider: configurableJWKSProvider{err: cause}}
	_, err := validator.verify(context.Background(), "TOKEN_SECRET_SENTINEL")
	var exchangeErr *tokenexchange.TokenExchangeError
	require.ErrorAs(t, err, &exchangeErr)
	assert.ErrorIs(t, err, cause)
	assert.Equal(t, tokenexchange.DetailJWKSUnavailable, exchangeErr.Diagnostic().Detail())
	assert.Equal(t, tokenexchange.OutcomeInfrastructureError, exchangeErr.Diagnostic().Outcome())
	assert.Equal(t, tokenexchange.ExchangeImpersonation, exchangeErr.Diagnostic().ExchangeKind())
	assert.NotContains(t, exchangeErr.Error(), "JWKS_SECRET_SENTINEL")
	assert.NotContains(t, exchangeErr.Error(), "TOKEN_SECRET_SENTINEL")
}

var _ tokenexchange.JWKSProvider = configurableJWKSProvider{}
