// Package helpers provides test utilities for E2E testing.
package helpers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateTestRSAKeyPair verifies RSA key pair generation works.
func TestGenerateTestRSAKeyPair(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GenerateTestRSAKeyPair()

	assert.NoError(t, err)
	assert.NotEmpty(t, privateKeyPEM)
	assert.NotEmpty(t, publicKeyPEM)
	assert.Contains(t, privateKeyPEM, "RSA PRIVATE KEY")
	assert.Contains(t, publicKeyPEM, "PUBLIC KEY")
}

// TestSignTestJWT verifies JWT signing produces valid JWTs.
func TestSignTestJWT(t *testing.T) {
	privateKeyPEM, publicKeyPEM, err := GenerateTestRSAKeyPair()
	require.NoError(t, err)

	claims := map[string]interface{}{
		"sub": "user-123",
		"iss": "https://auth.example.com",
		"aud": "broker",
		"exp": 9999999999,
	}

	tokenString, err := SignTestJWT(claims, privateKeyPEM)
	assert.NoError(t, err)
	assert.NotEmpty(t, tokenString)

	// Verify token has 3 parts (header.payload.signature)
	parts := 0
	for i := 0; i < len(tokenString); i++ {
		if tokenString[i] == '.' {
			parts++
		}
	}
	assert.Equal(t, 2, parts, "JWT should have 3 parts separated by 2 dots")

	// Verify the token signature and basic structure.
	publicKey, err := jwk.ParseKey([]byte(publicKeyPEM), jwk.WithX509(true))
	require.NoError(t, err)
	token, err := jwt.ParseString(tokenString, jwt.WithKey(jwa.RS256(), publicKey))
	require.NoError(t, err)

	// Verify claims are present
	sub, _ := token.Subject()
	assert.Equal(t, "user-123", sub)
}

// TestGenerateJWKSFromPublicKey verifies JWKS generation produces valid JWKS Set.
func TestGenerateJWKSFromPublicKey(t *testing.T) {
	_, publicKeyPEM, err := GenerateTestRSAKeyPair()
	require.NoError(t, err)

	jwksSet, err := GenerateJWKSFromPublicKey(publicKeyPEM)
	assert.NoError(t, err)
	assert.NotNil(t, jwksSet)

	// Verify JWKS Set structure
	keys, ok := jwksSet["keys"].([]map[string]interface{})
	assert.True(t, ok, "JWKS Set should have 'keys' array")
	assert.Len(t, keys, 1, "Should have exactly one key in test JWKS Set")

	key := keys[0]
	// Verify key contains required JWKS fields
	assert.NotEmpty(t, key["kty"], "Key type should be set")
	assert.NotEmpty(t, key["use"], "Key use should be set")
	assert.NotEmpty(t, key["alg"], "Algorithm should be set")

	// Verify it's an RSA key
	assert.Equal(t, "RSA", key["kty"])
	assert.Equal(t, "sig", key["use"])
	assert.Equal(t, "RS256", key["alg"])
}

// TestMockUpstreamJWKSEndpoint verifies mock server serves JWKS correctly.
func TestMockUpstreamJWKSEndpoint(t *testing.T) {
	mockServer := NewMockUpstreamOAuth2Server()
	defer mockServer.Close()

	// Verify server has generated keys
	assert.NotEmpty(t, mockServer.GetPrivateKeyPEM())
	assert.NotEmpty(t, mockServer.GetPublicKeyPEM())

	// Fetch JWKS from mock server
	resp, err := HTTPClient().Get(mockServer.URL() + "/.well-known/jwks.json")
	assert.NoError(t, err)
	assert.NotNil(t, resp)
	defer func() { _ = resp.Body.Close() }()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Parse JWKS response
	var jwksSet map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&jwksSet)
	assert.NoError(t, err)

	// Verify JWKS structure
	keys, ok := jwksSet["keys"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, keys, 1)

	// Verify JWKS endpoint was called
	assert.True(t, mockServer.GetJWKSCalled())
}

// TestJWTSigningAndVerification verifies end-to-end JWT signing and verification.
func TestJWTSigningAndVerification(t *testing.T) {
	mockServer := NewMockUpstreamOAuth2Server()
	defer mockServer.Close()

	// Create a JWT with test server's private key
	privateKeyPEM := mockServer.GetPrivateKeyPEM()
	publicKeyPEM := mockServer.GetPublicKeyPEM()
	claims := map[string]interface{}{
		"sub": "test-user",
		"iss": mockServer.URL(),
		"aud": "broker",
		"exp": 9999999999,
	}

	tokenString, err := SignTestJWT(claims, privateKeyPEM)
	require.NoError(t, err)

	// Verify the token signature and claims.
	publicKey, err := jwk.ParseKey([]byte(publicKeyPEM), jwk.WithX509(true))
	require.NoError(t, err)
	token, err := jwt.ParseString(tokenString, jwt.WithKey(jwa.RS256(), publicKey))
	require.NoError(t, err)

	sub, _ := token.Subject()
	assert.Equal(t, "test-user", sub)

	iss, _ := token.Issuer()
	assert.Equal(t, mockServer.URL(), iss)
}
