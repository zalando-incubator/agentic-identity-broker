package cimdclient

import (
	"context"
	"fmt"
	"strings"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// MetadataService builds public CIMD documents from metadata-safe service state.
type MetadataService struct {
	serviceReader ports.CIMDClientServiceReader
	publicKeys    ports.CIMDClientPublicKeyProvider
	publicURL     string
}

// NewMetadataService constructs a public CIMD metadata provider.
func NewMetadataService(
	serviceReader ports.CIMDClientServiceReader,
	publicKeys ports.CIMDClientPublicKeyProvider,
	publicURL string,
) *MetadataService {
	return &MetadataService{
		serviceReader: serviceReader,
		publicKeys:    publicKeys,
		publicURL:     strings.TrimRight(publicURL, "/"),
	}
}

// Metadata returns the public Client ID Metadata Document for an eligible CIMD service.
func (s *MetadataService) Metadata(ctx context.Context, serviceID id.ServiceID) (*ports.CIMDClientMetadata, error) {
	service, err := s.eligibleService(ctx, serviceID)
	if err != nil {
		return nil, err
	}
	if _, err := s.publishedJWKSet(ctx); err != nil {
		return nil, err
	}

	clientID := service.ClientID.String()
	return &ports.CIMDClientMetadata{ // #nosec G101 -- RFC-defined public Client ID Metadata fields, not credentials.
		ClientID:                     clientID,
		RedirectURIs:                 []string{s.publicURL + "/api/third-party/" + serviceID.String() + "/oauth2/callback"},
		GrantTypes:                   []string{"authorization_code", "refresh_token"},
		ResponseTypes:                []string{"code"},
		TokenEndpointAuthMethod:      "private_key_jwt",
		TokenEndpointAuthSigningAlgo: "ES256",
		JWKSURI:                      clientID + "/jwks.json",
	}, nil
}

// JWKSet returns the public CIMD verification keys for an eligible CIMD service.
func (s *MetadataService) JWKSet(ctx context.Context, serviceID id.ServiceID) (jwk.Set, error) {
	if _, err := s.eligibleService(ctx, serviceID); err != nil {
		return nil, err
	}
	return s.publishedJWKSet(ctx)
}

func (s *MetadataService) eligibleService(ctx context.Context, serviceID id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
	if s.serviceReader == nil || s.publicURL == "" || serviceID.IsZero() {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}
	service, err := s.serviceReader.GetCIMDClientService(ctx, serviceID)
	if err != nil || service == nil || service.ID != serviceID || service.TokenEndpointAuthMethod != "private_key_jwt" {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}

	expectedClientID, err := model.CIMDClientID(s.publicURL, serviceID)
	if err != nil || service.ClientID.IsZero() || service.ClientID != expectedClientID {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}
	return service, nil
}

func (s *MetadataService) publishedJWKSet(ctx context.Context) (jwk.Set, error) {
	if s.publicKeys == nil {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}
	input, err := s.publicKeys.PublicJWKSet(ctx)
	if err != nil || input == nil {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}

	output := jwk.NewSet()
	for index := range input.Len() {
		key, ok := input.Key(index)
		if !ok {
			continue
		}
		algorithm, ok := key.Algorithm()
		if !ok || algorithm != jwa.ES256() {
			continue
		}
		publicKey, err := jwk.PublicKeyOf(key)
		if err != nil {
			continue
		}
		ecdsaKey, ok := publicKey.(jwk.ECDSAPublicKey)
		if !ok {
			continue
		}
		curve, ok := ecdsaKey.Crv()
		if !ok || curve != jwa.P256() {
			continue
		}
		kid, ok := key.KeyID()
		if !ok || kid == "" {
			continue
		}
		if err := publicKey.Set(jwk.KeyIDKey, kid); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key id", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := publicKey.Set(jwk.AlgorithmKey, jwa.ES256()); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key algorithm", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := publicKey.Set(jwk.KeyUsageKey, "sig"); err != nil {
			return nil, fmt.Errorf("%w: set CIMD key use", ports.ErrCIMDPublicKeyUnavailable)
		}
		if err := output.AddKey(publicKey); err != nil {
			return nil, fmt.Errorf("%w: add CIMD public key", ports.ErrCIMDPublicKeyUnavailable)
		}
	}
	if output.Len() == 0 {
		return nil, ports.ErrCIMDPublicKeyUnavailable
	}
	return output, nil
}

var _ ports.CIMDClientMetadataProvider = (*MetadataService)(nil)
