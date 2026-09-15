package fixtures

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

const (
	rs256JWTKeyID = "e2e-extproc-rs256"
	rsaKeyBits    = 2048
)

// RS256JWTFixture mints JWTs that match its serialized public JWKS.
//
// Create one fixture in a suite's BeforeAll and reuse it for the suite. The
// private key is kept unexported and is never serialized or included in errors.
type RS256JWTFixture struct {
	signingKey jwk.Key
	issuer     string
	audience   string
	jwksJSON   string
}

// NewRS256JWTFixture generates an RSA key pair and serializes its public key
// as a single-key RS256 JWKS document. issuer and audience are included in
// every token minted by the fixture.
func NewRS256JWTFixture(issuer, audience string) (*RS256JWTFixture, error) {
	if strings.TrimSpace(issuer) == "" {
		return nil, fmt.Errorf("JWT issuer is required")
	}
	if strings.TrimSpace(audience) == "" {
		return nil, fmt.Errorf("JWT audience is required")
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key pair: %w", err)
	}
	if err := privateKey.Validate(); err != nil {
		return nil, fmt.Errorf("validate generated RSA key pair: %w", err)
	}
	privateKey.Precompute()

	signingKey, err := jwk.Import[jwk.Key](privateKey)
	if err != nil {
		return nil, fmt.Errorf("import signing JWK: %w", err)
	}
	if err := signingKey.Set(jwk.KeyIDKey, rs256JWTKeyID); err != nil {
		return nil, fmt.Errorf("set signing JWK key ID: %w", err)
	}
	if err := signingKey.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		return nil, fmt.Errorf("set signing JWK algorithm: %w", err)
	}

	publicKey, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("import public JWK: %w", err)
	}
	if err := publicKey.Set(jwk.KeyIDKey, rs256JWTKeyID); err != nil {
		return nil, fmt.Errorf("set public JWK key ID: %w", err)
	}
	if err := publicKey.Set(jwk.AlgorithmKey, jwa.RS256()); err != nil {
		return nil, fmt.Errorf("set public JWK algorithm: %w", err)
	}
	if err := publicKey.Set(jwk.KeyUsageKey, jwk.ForSignature); err != nil {
		return nil, fmt.Errorf("set public JWK use: %w", err)
	}

	jwks := jwk.NewSet()
	if err := jwks.AddKey(publicKey); err != nil {
		return nil, fmt.Errorf("add public JWK to set: %w", err)
	}
	jwksJSON, err := json.Marshal(jwks)
	if err != nil {
		return nil, fmt.Errorf("marshal public JWKS: %w", err)
	}

	return &RS256JWTFixture{
		signingKey: signingKey,
		issuer:     issuer,
		audience:   audience,
		jwksJSON:   string(jwksJSON),
	}, nil
}

// Issuer returns the issuer set on tokens minted by the fixture.
func (f *RS256JWTFixture) Issuer() string {
	return f.issuer
}

// Audience returns the audience set on tokens minted by the fixture.
func (f *RS256JWTFixture) Audience() string {
	return f.audience
}

// JWKSJSON returns the public JWKS document for mounting in Agentgateway.
func (f *RS256JWTFixture) JWKSJSON() string {
	return f.jwksJSON
}

// MintToken returns an RS256 JWT containing sub, iss, aud, and exp claims.
// expiresAt must be non-zero; a past value is allowed for expiry scenarios.
func (f *RS256JWTFixture) MintToken(subject string, expiresAt time.Time) (string, error) {
	if expiresAt.IsZero() {
		return "", fmt.Errorf("JWT expiration is required")
	}

	token, err := jwt.NewBuilder().
		Issuer(f.issuer).
		Audience([]string{f.audience}).
		Subject(subject).
		Expiration(expiresAt).
		Build()
	if err != nil {
		return "", fmt.Errorf("build JWT claims: %w", err)
	}

	signed, err := jwt.Sign(token, jwt.WithKey(jwa.RS256(), f.signingKey))
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	return string(signed), nil
}
