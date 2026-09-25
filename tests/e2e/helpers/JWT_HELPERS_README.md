# JWT Helpers for RFC 8693 Token Exchange E2E Testing

## Overview

This package provides test infrastructure for RFC 8693 Token Exchange E2E tests, enabling tests to create real, cryptographically-signed JWT tokens that pass validation. Previously, E2E tests used string placeholders that failed JWT validation. These helpers bridge the gap by providing:

1. RSA key pair generation for test fixtures
2. JWT signing with RS256 algorithm
3. JWKS endpoint serving for JWT validation
4. Mock upstream OAuth2 server integration

## Quick Start

### Generate Test JWTs

```go
import "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"

// Create RSA key pair
privateKeyPEM, publicKeyPEM, err := helpers.GenerateTestRSAKeyPair()
if err != nil {
    t.Fatalf("Failed to generate keys: %v", err)
}

// Sign a JWT with test claims
claims := map[string]interface{}{
    "sub": "user@example.com",
    "iss": "https://auth.example.com",
    "aud": "broker",
    "exp": time.Now().Add(1 * time.Hour).Unix(),
    "azp": "agent-123",
}

token, err := helpers.SignTestJWT(claims, privateKeyPEM)
if err != nil {
    t.Fatalf("Failed to sign JWT: %v", err)
}

fmt.Println("Token:", token)
```

### Serve JWKS from Mock Server

```go
import "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"

// Create mock upstream OAuth2 server
mockServer := helpers.NewMockUpstreamOAuth2Server()
defer mockServer.Close()

// Mock server automatically serves JWKS at /.well-known/jwks.json
resp, err := helpers.HTTPClient().Get(mockServer.URL() + "/.well-known/jwks.json")
if err != nil {
    t.Fatalf("Failed to fetch JWKS: %v", err)
}
defer resp.Body.Close()

// Verify JWKS structure
var jwks map[string]interface{}
json.NewDecoder(resp.Body).Decode(&jwks)

keys := jwks["keys"].([]interface{})
fmt.Printf("JWKS contains %d key(s)\n", len(keys))
```

### Complete E2E Test Example

```go
func TestTokenExchangeWithRealJWT(t *testing.T) {
    // Create mock upstream server
    mockServer := helpers.NewMockUpstreamOAuth2Server()
    defer mockServer.Close()

    // Get test keys
    privateKeyPEM := mockServer.GetPrivateKeyPEM()
    publicKeyPEM := mockServer.GetPublicKeyPEM()

    // Create subject token (from user)
    subjectClaims := map[string]interface{}{
        "sub": "user@example.com",
        "iss": mockServer.URL(),
        "aud": "broker",
        "exp": time.Now().Add(1 * time.Hour).Unix(),
        "azp": "agent-123",
    }
    subjectToken, _ := helpers.SignTestJWT(subjectClaims, privateKeyPEM)

    // Create client assertion (from gateway)
    assertionClaims := map[string]interface{}{
        "sub": "gateway-123",
        "iss": "https://gateway.example.com",
        "aud": mockServer.URL(),
        "exp": time.Now().Add(1 * time.Hour).Unix(),
    }
    clientAssertion, _ := helpers.SignTestJWT(assertionClaims, privateKeyPEM)

    // Send token exchange request
    resp, _ := helpers.HTTPClient().PostForm(
        testServer.URL() + "/oauth2/token",
        url.Values{
            "grant_type":       {"urn:ietf:params:oauth:grant-type:token-exchange"},
            "subject_token":    {subjectToken},
            "client_assertion": {clientAssertion},
            "resource":         {"https://api.github.com"},
        },
    )

    // Verify RFC 8693 response
    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)

    assert.Equal(t, "Bearer", result["token_type"])
    assert.NotEmpty(t, result["access_token"])
    assert.NotEmpty(t, result["issued_token_type"])
}
```

## API Reference

### GenerateTestRSAKeyPair()

Generates a new 2048-bit RSA key pair in PEM format.

```go
privateKeyPEM, publicKeyPEM, err := GenerateTestRSAKeyPair()
// Returns:
// privateKeyPEM: Private key as PEM string (-----BEGIN RSA PRIVATE KEY-----)
// publicKeyPEM: Public key as PEM string (-----BEGIN PUBLIC KEY-----)
// err: Error if generation failed
```

**Usage in tests**: Call once during test setup to generate a static key pair for multiple token signings.

### SignTestJWT()

Signs a JWT with RS256 algorithm using a private key.

```go
token, err := SignTestJWT(claims map[string]interface{}, privateKeyPEM string)
// Parameters:
// claims: Arbitrary JWT claims map (e.g., {"sub": "user-123", "exp": 9999999999})
// privateKeyPEM: Private key in PEM format from GenerateTestRSAKeyPair()
// Returns:
// token: Signed JWT string in "header.payload.signature" format
// err: Error if signing failed
```

**Claim recommendations**:
- `sub`: Subject (user ID)
- `iss`: Issuer (typically mock server URL)
- `aud`: Audience (broker identifier)
- `exp`: Expiration timestamp (unix seconds)
- `azp`: Authorized party (agent client ID for subject tokens)

### GenerateJWKSFromPublicKey()

Converts a public key to JWKS Set format for endpoint serving.

```go
jwksSet, err := GenerateJWKSFromPublicKey(publicKeyPEM string)
// Parameters:
// publicKeyPEM: Public key in PEM format from GenerateTestRSAKeyPair()
// Returns:
// jwksSet: Map with structure {"keys": [{...key metadata...}]}
// err: Error if conversion failed
```

**JWKS Set structure**:
```json
{
  "keys": [
    {
      "kty": "RSA",
      "use": "sig",
      "alg": "RS256",
      "n": "...",
      "e": "AQAB"
    }
  ]
}
```

### MockUpstreamOAuth2Server

Enhanced mock server with JWKS endpoint support.

#### NewMockUpstreamOAuth2Server()

Creates a new mock upstream OAuth2 server with automatic key generation.

```go
mockServer := NewMockUpstreamOAuth2Server()
defer mockServer.Close()

// Automatically available:
// - /.well-known/openid-configuration (metadata)
// - /.well-known/jwks.json (JWKS endpoint)
// - /oauth/authorize (authorization endpoint)
// - /oauth/token (token endpoint)
```

#### Methods

**Key Management**:
- `GetPrivateKeyPEM() string`: Returns private key for signing test JWTs
- `GetPublicKeyPEM() string`: Returns public key for verification
- `URL() string`: Returns base URL of mock server

**Endpoint Tracking**:
- `GetJWKSCalled() bool`: Returns whether JWKS endpoint was accessed
- `GetTokenCalled() bool`: Returns whether token endpoint was accessed
- `GetMetadataCalled() bool`: Returns whether metadata endpoint was accessed
- `GetAuthorizeCalled() bool`: Returns whether authorize endpoint was accessed

**Token Configuration**:
- `WithSuccessfulTokenResponse()`: Configure server to return successful token
- `WithErrorResponse(errorCode string)`: Configure server to return error
- `WithAccessToken(token string)`: Set custom access token in response
- `WithRefreshToken(token string)`: Set custom refresh token in response
- `WithExpiresIn(seconds int)`: Set token expiration time

## Security Considerations

### Test Keys vs Production Keys

These helpers generate test keys suitable for development and testing ONLY. Characteristics:

- **2048-bit RSA**: Faster test execution, adequate for testing JWT validation logic
- **No key persistence**: Keys generated fresh for each test run
- **No rotation**: Keys do not rotate (acceptable for test fixtures)
- **No audit trail**: No logging of key operations (test infrastructure)

**Production Requirements** (not applicable to testing):
- Minimum 4096-bit RSA or equivalent
- Key rotation schedules
- Secure key storage with HSM or KMS
- Complete audit trails
- Key revocation mechanisms

### JWT Validation in Tests

Tests using these helpers should verify:

1. **JWT Signature**: Verify tokens are signed with RS256
   ```go
   publicKey, err := jwk.ParseKey([]byte(publicKeyPEM), jwk.WithX509(true))
   require.NoError(t, err)
   _, err = jwt.ParseString(tokenString, jwt.WithKey(jwa.RS256(), publicKey))
   require.NoError(t, err)
   ```

2. **Claim Structure**: Verify required claims are present
   ```go
   subject, _ := token.Subject()
   issuer, _ := token.Issuer()
   ```

3. **Expiration**: Verify tokens expire correctly
   ```go
   expiration, _ := token.Expiration()
   assert.After(t, expiration, time.Now())
   ```

### No Custom Cryptography

This package uses **only** battle-tested libraries per Constitution Principle III:
- **stdlib**: crypto/rsa, crypto/x509, encoding/pem for key operations
- **lestrrat-go/jwx/v4**: For JWT and JWK operations

No custom cryptographic implementations are present.

## Testing Patterns

### Pattern 1: Separate Subject and Client Assertion

For RFC 8693 token exchange, create separate JWTs for different purposes:

```go
// Subject token: represents user identity
subjectToken, _ := helpers.SignTestJWT(
    map[string]interface{}{
        "sub": principal,  // User ID
        "iss": mockServer.URL(),
        "aud": "broker",
        "exp": time.Now().Add(1*time.Hour).Unix(),
        "azp": agentClientID,  // Agent ID
    },
    privateKeyPEM,
)

// Client assertion: represents gateway identity
clientAssertion, _ := helpers.SignTestJWT(
    map[string]interface{}{
        "sub": gatewayID,
        "iss": "https://gateway.example.com",
        "aud": mockServer.URL(),
        "exp": time.Now().Add(1*time.Hour).Unix(),
    },
    privateKeyPEM,
)
```

### Pattern 2: Expired Token Testing

Create expired tokens to test error handling:

```go
expiredClaims := map[string]interface{}{
    "sub": "user-123",
    "iss": mockServer.URL(),
    "aud": "broker",
    "exp": time.Now().Add(-1 * time.Hour).Unix(),  // Already expired
}
expiredToken, _ := helpers.SignTestJWT(expiredClaims, privateKeyPEM)

// Send token exchange request with expired token
// Expect 400 invalid_request or 401 invalid_client response
```

### Pattern 3: Invalid Issuer Testing

Create tokens with mismatched issuer to test validation:

```go
wrongIssuerClaims := map[string]interface{}{
    "sub": "user-123",
    "iss": "https://untrusted.example.com",  // Wrong issuer
    "aud": "broker",
    "exp": time.Now().Add(1*time.Hour).Unix(),
}
wrongIssuerToken, _ := helpers.SignTestJWT(wrongIssuerClaims, privateKeyPEM)

// Send token exchange request with wrong issuer token
// Expect 401 invalid_client (issuer mismatch)
```

## Troubleshooting

### "failed to parse PEM block"

Ensure the PEM string includes the full header and footer:
```
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA...
-----END RSA PRIVATE KEY-----
```

### "Key type mismatch"

This package generates RSA keys only. If tests require other key types (ECDSA, symmetric), use different helpers or modify GenerateTestRSAKeyPair().

### "JWKS endpoint not called"

Verify mock server URL is passed to JWT validator configuration:
```go
config := fixtures.OAuth2ConfigWithUpstream(mockServer.URL())
// JWT validator should fetch JWKS from mockServer.URL() + "/.well-known/jwks.json"
```

### Token validation fails despite valid signature

Check token expiration:
```go
expiration, _ := token.Expiration()
if time.Now().After(expiration) {
    t.Error("Token is expired")
}
```

## Performance Notes

- RSA key generation: ~70ms for 2048-bit keys
- JWT signing: ~4ms per token with RS256
- JWKS endpoint response: <1ms
- Mock server startup: <1ms

Suitable for test suites with 100+ tests without performance impact.

## Related Files

- **JWT Helpers**: `tests/e2e/helpers/jwt_helpers.go`
- **Helper Tests**: `tests/e2e/helpers/jwt_helpers_test.go`
- **Mock Upstream**: `tests/e2e/helpers/mock_upstream.go`
- **E2E Tests**: `tests/e2e/token_exchange_test.go`

## References

- [RFC 8693: OAuth 2.0 Token Exchange](https://tools.ietf.org/html/rfc8693)
- [RFC 7517: JSON Web Key](https://tools.ietf.org/html/rfc7517)
- [RFC 7518: JSON Web Algorithms](https://tools.ietf.org/html/rfc7518)
- [lestrrat-go/jwx Documentation](https://github.com/lestrrat-go/jwx)
