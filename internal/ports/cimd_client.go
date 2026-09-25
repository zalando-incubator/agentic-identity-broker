package ports

import (
	"context"
	"errors"

	"github.com/lestrrat-go/jwx/v4/jwk"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

var (
	// ErrCIMDKeyUnavailable reports that no usable CIMD client-authentication key exists.
	ErrCIMDKeyUnavailable = errors.New("CIMD client-authentication key is unavailable")
	// ErrCIMDPublicKeyUnavailable reports that a usable CIMD public key cannot be published.
	ErrCIMDPublicKeyUnavailable = errors.New("CIMD client-authentication public key is unavailable")
)

// CIMDClientKeyLifecycle manages the dedicated CIMD client-authentication key lifecycle.
type CIMDClientKeyLifecycle interface {
	GenerateKey(ctx context.Context, algorithm string) (*storage.SigningKey, error)
	ListKeys(ctx context.Context) ([]*storage.SigningKey, error)
	PromoteKey(ctx context.Context, kid id.KeyID) (*storage.SigningKey, error)
	DeleteKey(ctx context.Context, kid id.KeyID) error
}

// CIMDClientKeyBootstrapper initializes the first CIMD key when persisted services require it.
type CIMDClientKeyBootstrapper interface {
	EnsureInitialKey(ctx context.Context) (*storage.SigningKey, bool, error)
}

// CIMDClientKeyReadiness guards operations that require a usable published CIMD public key.
type CIMDClientKeyReadiness interface {
	RequirePublishedKey(ctx context.Context) error
}

// CIMDClientPublicKeyProvider publishes verification keys for CIMD client assertions.
type CIMDClientPublicKeyProvider interface {
	PublicJWKSet(ctx context.Context) (jwk.Set, error)
}

// CIMDClientAssertionSigner creates an assertion bound to one client identity and token endpoint.
type CIMDClientAssertionSigner interface {
	SignClientAssertion(ctx context.Context, clientID id.ClientID, tokenEndpoint string) (string, error)
}

// CIMDClientKeyService combines the narrow capabilities of the CIMD key domain.
type CIMDClientKeyService interface {
	CIMDClientKeyLifecycle
	CIMDClientKeyBootstrapper
	CIMDClientKeyReadiness
	CIMDClientPublicKeyProvider
}

// CIMDClientServiceProjection exposes only the service fields safe for public metadata composition.
type CIMDClientServiceProjection struct {
	ID                      id.ServiceID
	ClientID                id.ClientID
	TokenEndpointAuthMethod string
}

// CIMDClientServiceReader reads metadata-safe service state without revealing credentials.
type CIMDClientServiceReader interface {
	GetCIMDClientService(ctx context.Context, serviceID id.ServiceID) (*CIMDClientServiceProjection, error)
}

// CIMDClientMetadata contains the public Client ID Metadata Document fields.
type CIMDClientMetadata struct {
	ClientID                     string   `json:"client_id"`
	RedirectURIs                 []string `json:"redirect_uris"`
	GrantTypes                   []string `json:"grant_types"`
	ResponseTypes                []string `json:"response_types"`
	TokenEndpointAuthMethod      string   `json:"token_endpoint_auth_method"`
	TokenEndpointAuthSigningAlgo string   `json:"token_endpoint_auth_signing_alg"`
	JWKSURI                      string   `json:"jwks_uri"`
}

// CIMDClientMetadataProvider builds public metadata and its corresponding public JWK Set.
type CIMDClientMetadataProvider interface {
	Metadata(ctx context.Context, serviceID id.ServiceID) (*CIMDClientMetadata, error)
	JWKSet(ctx context.Context, serviceID id.ServiceID) (jwk.Set, error)
}
