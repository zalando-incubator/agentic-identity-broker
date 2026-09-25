package cimdclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const cimdMetadataTestPublicURL = "https://broker.example.test"

func TestMetadataService_MetadataContainsExactCIMDPublicFields(t *testing.T) {
	serviceID := id.MustParseServiceID("00000000-0000-0000-0000-000000000101")
	reader := &cimdMetadataServiceReader{
		getCIMDClientServiceFn: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
			return cimdMetadataServiceProjection(serviceID), nil
		},
	}
	publicKeys := &cimdMetadataPublicKeyProvider{
		publicJWKSetFn: func(context.Context) (jwk.Set, error) {
			return cimdMetadataJWKSet(t, cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-current", jwa.ES256())), nil
		},
	}
	service := NewMetadataService(reader, publicKeys, cimdMetadataTestPublicURL)

	metadata, err := service.Metadata(context.Background(), serviceID)

	require.NoError(t, err)
	require.NotNil(t, metadata)
	assert.Equal(t, &ports.CIMDClientMetadata{
		ClientID:                     cimdMetadataClientID(serviceID),
		RedirectURIs:                 []string{cimdMetadataCallbackURL(serviceID)},
		GrantTypes:                   []string{"authorization_code", "refresh_token"},
		ResponseTypes:                []string{"code"},
		TokenEndpointAuthMethod:      "private_key_jwt",
		TokenEndpointAuthSigningAlgo: "ES256",
		JWKSURI:                      cimdMetadataClientID(serviceID) + "/jwks.json",
	}, metadata)
}

func TestMetadataService_JWKSetPublishesOnlyPublicES256P256SigningKeys(t *testing.T) {
	serviceID := id.MustParseServiceID("00000000-0000-0000-0000-000000000102")
	currentKey := cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-current", jwa.ES256())
	graceKey := cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-published-during-grace", jwa.ES256())
	privateInputKey := cimdMetadataPrivateJWK(t, elliptic.P256(), "cimd-private-input", jwa.ES256())
	wrongAlgorithm := cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-es384", jwa.ES384())
	wrongCurve := cimdMetadataPublicJWK(t, elliptic.P384(), "cimd-p384", jwa.ES256())

	reader := &cimdMetadataServiceReader{
		getCIMDClientServiceFn: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
			return cimdMetadataServiceProjection(serviceID), nil
		},
	}
	publicKeys := &cimdMetadataPublicKeyProvider{
		publicJWKSetFn: func(context.Context) (jwk.Set, error) {
			return cimdMetadataJWKSet(t, currentKey, graceKey, privateInputKey, wrongAlgorithm, wrongCurve), nil
		},
	}
	service := NewMetadataService(reader, publicKeys, cimdMetadataTestPublicURL)

	set, err := service.JWKSet(context.Background(), serviceID)

	require.NoError(t, err)
	require.NotNil(t, set)
	require.Equal(t, 3, set.Len(), "the current, published-grace, and sanitized public keys must remain available")

	keysByID := cimdMetadataJWKsByID(t, set)
	assertCIMDMetadataPublicJWK(t, keysByID["cimd-current"], "cimd-current")
	assertCIMDMetadataPublicJWK(t, keysByID["cimd-published-during-grace"], "cimd-published-during-grace")
	assertCIMDMetadataPublicJWK(t, keysByID["cimd-private-input"], "cimd-private-input")
	assert.NotContains(t, keysByID, "cimd-es384")
	assert.NotContains(t, keysByID, "cimd-p384")
}

func TestMetadataService_ClientAndJWKURLsStayStableAcrossKeyPublicationChanges(t *testing.T) {
	serviceID := id.MustParseServiceID("00000000-0000-0000-0000-000000000103")
	reader := &cimdMetadataServiceReader{
		getCIMDClientServiceFn: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
			return cimdMetadataServiceProjection(serviceID), nil
		},
	}

	published := cimdMetadataJWKSet(t, cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-current", jwa.ES256()))
	publicKeys := &cimdMetadataPublicKeyProvider{
		publicJWKSetFn: func(context.Context) (jwk.Set, error) {
			return published, nil
		},
	}
	service := NewMetadataService(reader, publicKeys, cimdMetadataTestPublicURL)

	beforeRotation, err := service.Metadata(context.Background(), serviceID)
	require.NoError(t, err)

	published = cimdMetadataJWKSet(t,
		cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-current", jwa.ES256()),
		cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-published-during-grace", jwa.ES256()),
	)
	afterPublication, err := service.Metadata(context.Background(), serviceID)
	require.NoError(t, err)

	assert.Equal(t, cimdMetadataClientID(serviceID), beforeRotation.ClientID)
	assert.Equal(t, beforeRotation.ClientID, afterPublication.ClientID)
	assert.Equal(t, cimdMetadataClientID(serviceID)+"/jwks.json", beforeRotation.JWKSURI)
	assert.Equal(t, beforeRotation.JWKSURI, afterPublication.JWKSURI)
	assert.Equal(t, beforeRotation.RedirectURIs, afterPublication.RedirectURIs)
}

func TestMetadataService_UnavailableServicesExposeNeitherMetadataNorKeys(t *testing.T) {
	serviceID := id.MustParseServiceID("00000000-0000-0000-0000-000000000104")
	usableKeys := func(context.Context) (jwk.Set, error) {
		return cimdMetadataJWKSet(t, cimdMetadataPublicJWK(t, elliptic.P256(), "cimd-current", jwa.ES256())), nil
	}

	tests := []struct {
		name        string
		readService func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error)
		publicKeys  func(context.Context) (jwk.Set, error)
	}{
		{
			name: "unknown service",
			readService: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
				return nil, ports.ErrNotFound
			},
			publicKeys: usableKeys,
		},
		{
			name: "deleted service",
			readService: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
				return nil, ports.ErrNotFound
			},
			publicKeys: usableKeys,
		},
		{
			name: "public service",
			readService: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
				projection := cimdMetadataServiceProjection(serviceID)
				projection.TokenEndpointAuthMethod = "none"
				return projection, nil
			},
			publicKeys: usableKeys,
		},
		{
			name: "static confidential service",
			readService: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
				projection := cimdMetadataServiceProjection(serviceID)
				projection.TokenEndpointAuthMethod = ""
				return projection, nil
			},
			publicKeys: usableKeys,
		},
		{
			name: "CIMD service without a usable published key",
			readService: func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
				return cimdMetadataServiceProjection(serviceID), nil
			},
			publicKeys: func(context.Context) (jwk.Set, error) {
				return nil, ports.ErrCIMDPublicKeyUnavailable
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			service := NewMetadataService(
				&cimdMetadataServiceReader{getCIMDClientServiceFn: tt.readService},
				&cimdMetadataPublicKeyProvider{publicJWKSetFn: tt.publicKeys},
				cimdMetadataTestPublicURL,
			)

			metadata, metadataErr := service.Metadata(context.Background(), serviceID)
			require.ErrorIs(t, metadataErr, ports.ErrCIMDPublicKeyUnavailable)
			assert.NotErrorIs(t, metadataErr, ports.ErrNotFound)
			assert.Nil(t, metadata)

			set, keyErr := service.JWKSet(context.Background(), serviceID)
			require.ErrorIs(t, keyErr, ports.ErrCIMDPublicKeyUnavailable)
			assert.NotErrorIs(t, keyErr, ports.ErrNotFound)
			assert.Nil(t, set)
		})
	}
}

func cimdMetadataServiceProjection(serviceID id.ServiceID) *ports.CIMDClientServiceProjection {
	return &ports.CIMDClientServiceProjection{
		ID:                      serviceID,
		ClientID:                id.NewClientID(cimdMetadataClientID(serviceID)),
		TokenEndpointAuthMethod: "private_key_jwt",
	}
}

func cimdMetadataClientID(serviceID id.ServiceID) string {
	return cimdMetadataTestPublicURL + "/.well-known/oauth-client/" + serviceID.String()
}

func cimdMetadataCallbackURL(serviceID id.ServiceID) string {
	return cimdMetadataTestPublicURL + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
}

func cimdMetadataJWKSet(t *testing.T, keys ...jwk.Key) jwk.Set {
	t.Helper()

	set := jwk.NewSet()
	for _, key := range keys {
		require.NoError(t, set.AddKey(key))
	}
	return set
}

func cimdMetadataPublicJWK(t *testing.T, curve elliptic.Curve, kid string, algorithm jwa.SignatureAlgorithm) jwk.Key {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	require.NoError(t, err)

	key, err := jwk.Import[jwk.Key](&privateKey.PublicKey)
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, kid))
	require.NoError(t, key.Set(jwk.AlgorithmKey, algorithm))
	return key
}

func cimdMetadataPrivateJWK(t *testing.T, curve elliptic.Curve, kid string, algorithm jwa.SignatureAlgorithm) jwk.Key {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(curve, rand.Reader)
	require.NoError(t, err)

	key, err := jwk.Import[jwk.Key](privateKey)
	require.NoError(t, err)
	require.NoError(t, key.Set(jwk.KeyIDKey, kid))
	require.NoError(t, key.Set(jwk.AlgorithmKey, algorithm))
	return key
}

func cimdMetadataJWKsByID(t *testing.T, set jwk.Set) map[string]jwk.Key {
	t.Helper()

	keysByID := make(map[string]jwk.Key, set.Len())
	for index := 0; index < set.Len(); index++ {
		key, ok := set.Key(index)
		require.True(t, ok)
		kid, ok := key.KeyID()
		require.True(t, ok)
		keysByID[kid] = key
	}
	return keysByID
}

func assertCIMDMetadataPublicJWK(t *testing.T, key jwk.Key, kid string) {
	t.Helper()
	require.NotNil(t, key)

	actualKID, ok := key.KeyID()
	require.True(t, ok)
	assert.Equal(t, kid, actualKID)

	algorithm, ok := key.Algorithm()
	require.True(t, ok)
	assert.Equal(t, jwa.ES256(), algorithm)

	usage, ok := key.KeyUsage()
	require.True(t, ok)
	assert.Equal(t, "sig", usage)

	encoded, err := json.Marshal(key)
	require.NoError(t, err)

	var document map[string]any
	require.NoError(t, json.Unmarshal(encoded, &document))
	assert.Equal(t, "EC", document["kty"])
	assert.Equal(t, "P-256", document["crv"])
	assert.NotEmpty(t, document["x"])
	assert.NotEmpty(t, document["y"])
	for _, privateMember := range []string{"d", "k", "secret", "ciphertext", "branch_key", "client_assertion"} {
		assert.NotContains(t, document, privateMember)
	}
}

type cimdMetadataServiceReader struct {
	getCIMDClientServiceFn func(context.Context, id.ServiceID) (*ports.CIMDClientServiceProjection, error)
}

func (r *cimdMetadataServiceReader) GetCIMDClientService(ctx context.Context, serviceID id.ServiceID) (*ports.CIMDClientServiceProjection, error) {
	return r.getCIMDClientServiceFn(ctx, serviceID)
}

type cimdMetadataPublicKeyProvider struct {
	publicJWKSetFn func(context.Context) (jwk.Set, error)
}

func (p *cimdMetadataPublicKeyProvider) PublicJWKSet(ctx context.Context) (jwk.Set, error) {
	return p.publicJWKSetFn(ctx)
}

var _ ports.CIMDClientServiceReader = (*cimdMetadataServiceReader)(nil)
var _ ports.CIMDClientPublicKeyProvider = (*cimdMetadataPublicKeyProvider)(nil)
