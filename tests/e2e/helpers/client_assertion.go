package helpers

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

const ClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" // #nosec G101 -- RFC-defined assertion type identifier, not a credential.

// ClientAssertionKey holds a test signing key and its public JWK representation.
type ClientAssertionKey struct {
	KID        string
	Algorithm  jwa.SignatureAlgorithm
	PrivateKey any
	PublicKey  jwk.Key
}

// GenerateClientAssertionKey creates an RSA-2048 or P-256 test signing key.
func GenerateClientAssertionKey(algorithm jwa.SignatureAlgorithm, kid string) (*ClientAssertionKey, error) {
	if kid == "" {
		kid = uuid.NewString()
	}
	var privateKey any
	switch algorithm.String() {
	case "RS256":
		generated, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, fmt.Errorf("generate RSA assertion key: %w", err)
		}
		privateKey = generated
	case "ES256":
		generated, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("generate P-256 assertion key: %w", err)
		}
		privateKey = generated
	default:
		return nil, fmt.Errorf("unsupported test assertion key algorithm %q", algorithm)
	}

	var publicKey any
	switch key := privateKey.(type) {
	case *rsa.PrivateKey:
		publicKey = &key.PublicKey
	case *ecdsa.PrivateKey:
		publicKey = &key.PublicKey
	}
	publicJWK, err := jwk.Import[jwk.Key](publicKey)
	if err != nil {
		return nil, fmt.Errorf("import assertion public key: %w", err)
	}
	if err := publicJWK.Set(jwk.KeyIDKey, kid); err != nil {
		return nil, fmt.Errorf("set assertion key id: %w", err)
	}
	if err := publicJWK.Set(jwk.KeyUsageKey, "sig"); err != nil {
		return nil, fmt.Errorf("set assertion key use: %w", err)
	}

	return &ClientAssertionKey{
		KID:        kid,
		Algorithm:  algorithm,
		PrivateKey: privateKey,
		PublicKey:  publicJWK,
	}, nil
}

// JWKSJSON returns a JSON Web Key Set containing only the public key.
func (key *ClientAssertionKey) JWKSJSON() ([]byte, error) {
	if key == nil || key.PublicKey == nil {
		return nil, fmt.Errorf("assertion public key is required")
	}
	keySet := jwk.NewSet()
	if err := keySet.AddKey(key.PublicKey); err != nil {
		return nil, fmt.Errorf("add assertion public key to set: %w", err)
	}
	encoded, err := json.Marshal(keySet)
	if err != nil {
		return nil, fmt.Errorf("marshal assertion key set: %w", err)
	}
	return encoded, nil
}

// ClientAssertionOptions controls assertion claims and JOSE headers for tests.
// Zero values produce a valid assertion with a five-minute lifetime.
type ClientAssertionOptions struct {
	Now            time.Time
	Algorithm      jwa.SignatureAlgorithm
	Issuer         string
	Subject        string
	Audience       []string
	Lifetime       time.Duration
	Expiration     *time.Time
	IssuedAt       *time.Time
	NotBefore      *time.Time
	JTI            string
	AssertionType  string
	OmitKID        bool
	OmitJTI        bool
	EmptyJTI       bool
	OmitExpiration bool
	OmitIssuedAt   bool
	JKU            string
	IncludeJWK     bool
}

// SignClientAssertion builds and signs a CIMD client assertion for an E2E case.
func SignClientAssertion(clientID, audience string, key *ClientAssertionKey, options ClientAssertionOptions) (string, string, error) {
	if key == nil || key.PrivateKey == nil {
		return "", "", fmt.Errorf("assertion signing key is required")
	}
	now := options.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	issuer := options.Issuer
	if issuer == "" {
		issuer = clientID
	}
	subject := options.Subject
	if subject == "" {
		subject = clientID
	}
	claimAudience := options.Audience
	if claimAudience == nil {
		claimAudience = []string{audience}
	}
	lifetime := options.Lifetime
	if lifetime == 0 {
		lifetime = 5 * time.Minute
	}

	builder := jwt.NewBuilder().Issuer(issuer).Subject(subject).Audience(claimAudience)
	if !options.OmitExpiration {
		expiresAt := now.Add(lifetime)
		if options.Expiration != nil {
			expiresAt = *options.Expiration
		}
		builder = builder.Expiration(expiresAt)
	}
	if !options.OmitIssuedAt {
		issuedAt := now
		if options.IssuedAt != nil {
			issuedAt = *options.IssuedAt
		}
		builder = builder.IssuedAt(issuedAt)
	}
	if options.NotBefore != nil {
		builder = builder.NotBefore(*options.NotBefore)
	}
	if !options.OmitJTI {
		jti := options.JTI
		if jti == "" && !options.EmptyJTI {
			jti = uuid.NewString()
		}
		builder = builder.JwtID(jti)
	}
	token, err := builder.Build()
	if err != nil {
		return "", "", fmt.Errorf("build client assertion: %w", err)
	}

	algorithm := options.Algorithm
	if algorithm.String() == "" {
		algorithm = key.Algorithm
	}
	headers := jws.NewHeaders()
	if !options.OmitKID {
		if err := headers.Set(jws.KeyIDKey, key.KID); err != nil {
			return "", "", fmt.Errorf("set assertion key id header: %w", err)
		}
	}
	if options.JKU != "" {
		if err := headers.Set("jku", options.JKU); err != nil {
			return "", "", fmt.Errorf("set assertion key set URL header: %w", err)
		}
	}
	if options.IncludeJWK {
		if err := headers.Set("jwk", key.PublicKey); err != nil {
			return "", "", fmt.Errorf("set assertion JWK header: %w", err)
		}
	}

	var signed []byte
	if algorithm == jwa.NoSignature() {
		signed, err = jwt.Sign(token, jwt.WithInsecureNoSignature())
	} else {
		signed, err = jwt.Sign(token, jwt.WithKey(algorithm, key.PrivateKey, jws.WithProtectedHeaders(headers)))
	}
	if err != nil {
		return "", "", fmt.Errorf("sign client assertion: %w", err)
	}
	assertionType := options.AssertionType
	if assertionType == "" {
		assertionType = ClientAssertionTypeJWTBearer
	}
	return string(signed), assertionType, nil
}
