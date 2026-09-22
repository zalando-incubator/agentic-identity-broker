package cimdclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	crand "crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const cimdClientAssertionLifetime = 5 * time.Minute

// AssertionSigner creates fresh outbound client assertions using CIMD keys.
type AssertionSigner struct {
	keyService *KeyService
	logger     *slog.Logger
}

// NewAssertionSigner constructs an assertion signer over the CIMD key service.
func NewAssertionSigner(keyService *KeyService) *AssertionSigner {
	logger := slog.Default()
	if keyService != nil && keyService.logger != nil {
		logger = keyService.logger
	}
	return &AssertionSigner{keyService: keyService, logger: logger}
}

// SignClientAssertion creates a short-lived ES256 assertion for one outbound token request.
func (s *AssertionSigner) SignClientAssertion(ctx context.Context, clientID id.ClientID, tokenEndpoint string) (string, error) {
	if s.keyService == nil || clientID.IsZero() || tokenEndpoint == "" {
		s.audit(id.KeyID(""), "rejected")
		return "", ports.ErrCIMDKeyUnavailable
	}

	key, err := s.keyService.currentUsableCIMDAssertionKey(ctx)
	if err != nil {
		keyID := id.KeyID("")
		if key != nil {
			keyID = key.KID
		}
		s.audit(keyID, "rejected")
		return "", ports.ErrCIMDKeyUnavailable
	}

	assertionID, err := uuid.NewRandomFromReader(crand.Reader)
	if err != nil {
		s.audit(key.KID, "rejected")
		return "", ports.ErrCIMDKeyUnavailable
	}
	now := time.Now().UTC()
	token, err := jwt.NewBuilder().
		Issuer(clientID.String()).
		Subject(clientID.String()).
		Audience([]string{tokenEndpoint}).
		IssuedAt(now).
		Expiration(now.Add(cimdClientAssertionLifetime)).
		JwtID(assertionID.String()).
		Build()
	if err != nil {
		s.audit(key.KID, "rejected")
		return "", ports.ErrCIMDKeyUnavailable
	}

	assertion, err := s.keyService.signCIMDClientAssertion(ctx, key.KID, token)
	if err != nil {
		s.audit(key.KID, "rejected")
		return "", ports.ErrCIMDKeyUnavailable
	}
	s.audit(key.KID, "success")
	return assertion, nil
}

func (s *KeyService) currentUsableCIMDAssertionKey(ctx context.Context) (*storage.SigningKey, error) {
	keys, err := s.repository.ListActiveInDomain(ctx, storage.KeyDomainCIMDClientAuthentication)
	if err != nil {
		return nil, ports.ErrCIMDKeyUnavailable
	}
	key := currentUsableKey(keys, time.Now().UTC())
	if key == nil || key.Algorithm != "ES256" {
		return nil, ports.ErrCIMDKeyUnavailable
	}

	published, err := s.PublicJWKSet(ctx)
	if err != nil {
		return key, ports.ErrCIMDKeyUnavailable
	}
	if _, found := published.LookupKeyID(key.KID.String()); !found {
		return key, ports.ErrCIMDKeyUnavailable
	}
	return key, nil
}

func (s *KeyService) signCIMDClientAssertion(ctx context.Context, expectedKeyID id.KeyID, token jwt.Token) (string, error) {
	key, err := s.currentUsableCIMDAssertionKey(ctx)
	if err != nil || key.KID != expectedKeyID {
		return "", ports.ErrCIMDKeyUnavailable
	}

	privatePEM, err := s.encryption.Decrypt(ctx, key.PrivateKeyEncrypted, cimdKeyEncryptionContext(key.KID))
	if err != nil {
		return "", ports.ErrCIMDKeyUnavailable
	}
	privateKey, err := cimdES256PrivateKeyFromPEM(privatePEM)
	if err != nil {
		return "", ports.ErrCIMDKeyUnavailable
	}

	headers := jws.NewHeaders()
	if err := headers.Set(jws.KeyIDKey, key.KID.String()); err != nil {
		return "", ports.ErrCIMDKeyUnavailable
	}
	assertion, err := jwt.Sign(token, jwt.WithKey(jwa.ES256(), privateKey, jws.WithProtectedHeaders(headers)))
	if err != nil {
		return "", ports.ErrCIMDKeyUnavailable
	}
	return string(assertion), nil
}

func cimdES256PrivateKeyFromPEM(privatePEM []byte) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode(privatePEM)
	if block == nil {
		return nil, ports.ErrCIMDKeyUnavailable
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, ports.ErrCIMDKeyUnavailable
	}
	privateKey, ok := key.(*ecdsa.PrivateKey)
	if !ok || privateKey.Curve != elliptic.P256() {
		return nil, ports.ErrCIMDKeyUnavailable
	}
	return privateKey, nil
}

func (s *AssertionSigner) audit(keyID id.KeyID, outcome string) {
	s.logger.Info("CIMD client assertion signing", "key_id", keyID, "operation", "sign", "outcome", outcome)
}

var _ ports.CIMDClientAssertionSigner = (*AssertionSigner)(nil)
