package oauth2server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"time"

	clientkeys "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
)

const (
	ClientAssertionTypeJWTBearer = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer" // #nosec G101 -- RFC-defined assertion type identifier, not a credential.
	MaxAssertionLifetime         = 5 * time.Minute
	ClockSkew                    = 30 * time.Second
)

var (
	ErrClientAuthentication            = errors.New("client authentication failed")
	ErrClientAuthenticationUnavailable = errors.New("client authentication dependency unavailable")
)

// VerifiedAssertion represents a client assertion after signature and claim validation.
type VerifiedAssertion struct {
	ClientID  string
	JTI       string
	ExpiresAt time.Time
	Method    string
}

// ClientKeySource resolves a CIMD document's published verification keys.
type ClientKeySource interface {
	KeysFor(ctx context.Context, document *ports.CIMDMetadataDTO) (jwk.Set, error)
	RefreshForKID(ctx context.Context, document *ports.CIMDMetadataDTO, kid string) (jwk.Set, error)
}

// ClientAssertionVerifier validates inbound private_key_jwt assertions.
type ClientAssertionVerifier struct {
	keys      ClientKeySource
	replay    ports.ClientAssertionReplayRepository
	audiences map[string]struct{}
	clock     func() time.Time
	logger    *slog.Logger
}

// NewClientAssertionVerifier creates a verifier with fixed audience and time dependencies.
func NewClientAssertionVerifier(keys ClientKeySource, replay ports.ClientAssertionReplayRepository, audiences []string, clock func() time.Time, logger *slog.Logger) *ClientAssertionVerifier {
	if clock == nil {
		clock = time.Now
	}
	if logger == nil {
		logger = slog.Default()
	}
	approvedAudiences := make(map[string]struct{}, len(audiences))
	for _, audience := range audiences {
		if audience != "" {
			approvedAudiences[audience] = struct{}{}
		}
	}
	return &ClientAssertionVerifier{keys: keys, replay: replay, audiences: approvedAudiences, clock: clock, logger: logger}
}

// Verify authenticates a private_key_jwt assertion and spends its jti after validation.
func (v *ClientAssertionVerifier) Verify(ctx context.Context, document *ports.CIMDMetadataDTO, assertionType, assertion string) (*VerifiedAssertion, error) {
	if document == nil || document.ClientID == "" || assertionType == "" || assertion == "" {
		return nil, v.reject(ctx, document, "missing", "")
	}
	if document.AuthMethod != "private_key_jwt" || assertionType != ClientAssertionTypeJWTBearer {
		return nil, v.reject(ctx, document, "malformed", "")
	}

	algorithm, ok := assertionSigningAlgorithm(document.SigningAlg)
	if !ok {
		return nil, v.reject(ctx, document, "claims", "")
	}
	message, err := jws.Parse([]byte(assertion))
	if err != nil || len(message.Signatures()) != 1 {
		return nil, v.reject(ctx, document, "malformed", "")
	}
	headers := message.Signatures()[0].ProtectedHeaders()
	headerAlgorithm, ok := headers.Algorithm()
	if !ok || headerAlgorithm != algorithm {
		return nil, v.reject(ctx, document, "signature", "")
	}
	kid, ok := headers.KeyID()
	if !ok || kid == "" {
		return nil, v.reject(ctx, document, "malformed", "")
	}
	if v.keys == nil {
		return nil, v.unavailable(ctx, document)
	}
	keySet, err := v.keys.KeysFor(ctx, document)
	if err != nil {
		return nil, v.unavailable(ctx, document)
	}
	key, found := keySet.LookupKeyID(kid)
	if !found {
		keySet, err = v.keys.RefreshForKID(ctx, document, kid)
		if err != nil {
			return nil, v.unavailable(ctx, document)
		}
		if keySet != nil {
			key, found = keySet.LookupKeyID(kid)
		}
	}
	if !found || !clientkeys.ClientKeyEligible(key, algorithm.String()) {
		return nil, v.reject(ctx, document, "signature", "")
	}
	if keyAlgorithm, found := key.Algorithm(); found && keyAlgorithm.String() != algorithm.String() {
		return nil, v.reject(ctx, document, "signature", "")
	}

	token, err := jwt.ParseString(assertion, jwt.WithKey(algorithm, key), jwt.WithValidate(false))
	if err != nil {
		return nil, v.reject(ctx, document, "signature", "")
	}
	verified, category := v.validateClaims(document, token)
	if category != "" {
		jti := assertionJTI(token)
		if verified != nil {
			jti = verified.JTI
		}
		return nil, v.reject(ctx, document, category, jti)
	}
	if v.replay == nil {
		return nil, v.unavailable(ctx, document)
	}
	if err := v.replay.Consume(ctx, document.ClientID, verified.JTI, verified.ExpiresAt.Add(ClockSkew)); err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) && storageErr.Kind == storage.ErrorKindConflict {
			return nil, v.reject(ctx, document, "replayed", verified.JTI)
		}
		return nil, v.unavailable(ctx, document)
	}

	v.logger.InfoContext(ctx, "client_authenticated",
		"client_id", document.ClientID,
		"algorithm", algorithm.String(),
		"kid", kid,
		"jti_hash", clientAssertionJTIHash(verified.JTI),
	)
	return verified, nil
}

func (v *ClientAssertionVerifier) validateClaims(document *ports.CIMDMetadataDTO, token jwt.Token) (*VerifiedAssertion, string) {
	issuer, issuerOK := token.Issuer()
	subject, subjectOK := token.Subject()
	if !issuerOK || !subjectOK || issuer != document.ClientID || subject != document.ClientID {
		return nil, "claims"
	}

	audiences, audienceOK := token.Audience()
	approved := false
	if audienceOK {
		for _, audience := range audiences {
			if _, found := v.audiences[audience]; found {
				approved = true
				break
			}
		}
	}
	if !approved {
		return nil, "audience"
	}

	expiresAt, hasExpiration := token.Expiration()
	if !hasExpiration {
		return nil, "claims"
	}
	now := v.clock()
	if expiresAt.Before(now.Add(-ClockSkew)) {
		return &VerifiedAssertion{JTI: assertionJTI(token), ExpiresAt: expiresAt}, "expired"
	}
	if expiresAt.After(now.Add(MaxAssertionLifetime + ClockSkew)) {
		return &VerifiedAssertion{JTI: assertionJTI(token), ExpiresAt: expiresAt}, "claims"
	}

	if issuedAt, present := token.IssuedAt(); present {
		if issuedAt.After(now.Add(ClockSkew)) || !expiresAt.After(issuedAt) || expiresAt.Sub(issuedAt) > MaxAssertionLifetime {
			return &VerifiedAssertion{JTI: assertionJTI(token), ExpiresAt: expiresAt}, "claims"
		}
	}
	if notBefore, present := token.NotBefore(); present && notBefore.After(now.Add(ClockSkew)) {
		return &VerifiedAssertion{JTI: assertionJTI(token), ExpiresAt: expiresAt}, "claims"
	}
	jti := assertionJTI(token)
	if jti == "" {
		return &VerifiedAssertion{ExpiresAt: expiresAt}, "claims"
	}
	return &VerifiedAssertion{ClientID: document.ClientID, JTI: jti, ExpiresAt: expiresAt, Method: "private_key_jwt"}, ""
}

func assertionJTI(token jwt.Token) string {
	jti, _ := token.JwtID()
	return jti
}

func assertionSigningAlgorithm(name string) (jwa.SignatureAlgorithm, bool) {
	switch name {
	case "", "RS256":
		return jwa.RS256(), true
	case "ES256":
		return jwa.ES256(), true
	default:
		return jwa.SignatureAlgorithm{}, false
	}
}

func clientAssertionJTIHash(jti string) string {
	hash := sha256.Sum256([]byte(jti))
	return hex.EncodeToString(hash[:8])
}

func (v *ClientAssertionVerifier) reject(ctx context.Context, document *ports.CIMDMetadataDTO, category, jti string) error {
	attributes := []any{"failure_category", category}
	if document != nil && document.ClientID != "" {
		attributes = append(attributes, "client_id", document.ClientID)
	}
	if jti != "" {
		attributes = append(attributes, "jti_hash", clientAssertionJTIHash(jti))
	}
	v.logger.WarnContext(ctx, "client_authentication_rejected", attributes...)
	return ErrClientAuthentication
}

func (v *ClientAssertionVerifier) unavailable(ctx context.Context, document *ports.CIMDMetadataDTO) error {
	attributes := []any{"failure_category", "key_unavailable"}
	if document != nil && document.ClientID != "" {
		attributes = append(attributes, "client_id", document.ClientID)
	}
	v.logger.ErrorContext(ctx, "client_authentication_rejected", attributes...)
	return ErrClientAuthenticationUnavailable
}
