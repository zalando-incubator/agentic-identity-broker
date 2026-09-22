package fixtures

import (
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	// CIMDEndUserPublicURL is the stable HTTPS end-user URL used to derive outbound
	// CIMD client identities, callback URLs, and public JWK URLs in E2E scenarios.
	CIMDEndUserPublicURL = "https://broker.e2e.test"

	// CIMDPrivateKeyJWTAuthMethod is deliberately a fixture-level typed literal.
	// The production model gains its named private_key_jwt constant in T043.
	CIMDPrivateKeyJWTAuthMethod model.TokenEndpointAuthMethod = "private_key_jwt"
)

const (
	cimdStaticServiceID       = "04600000-0000-0000-0000-000000000001"
	cimdPublicServiceID       = "04600000-0000-0000-0000-000000000002"
	cimdConfidentialServiceID = "04600000-0000-0000-0000-000000000003"

	cimdActiveKeyID             = "04600000-0000-0000-0000-000000000011"
	cimdGracePreviousKeyID      = "04600000-0000-0000-0000-000000000012"
	cimdGracePendingKeyID       = "04600000-0000-0000-0000-000000000013"
	cimdPromotionCurrentKeyID   = "04600000-0000-0000-0000-000000000014"
	cimdPromotionCandidateKeyID = "04600000-0000-0000-0000-000000000015"
)

var cimdFixtureTimestamp = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

// CIMDClientIDURL returns the broker-hosted client ID URL for serviceID.
func CIMDClientIDURL(serviceID id.ServiceID) string {
	return CIMDEndUserPublicURL + "/.well-known/oauth-client/" + serviceID.String()
}

// CIMDCallbackURL returns the broker callback URL advertised for serviceID.
func CIMDCallbackURL(serviceID id.ServiceID) string {
	return CIMDEndUserPublicURL + "/api/third-party/" + serviceID.String() + "/oauth2/callback"
}

// CIMDJWKSURL returns the public CIMD JWK Set URL for serviceID.
func CIMDJWKSURL(serviceID id.ServiceID) string {
	return CIMDClientIDURL(serviceID) + "/jwks.json"
}

// CIMDStaticConfidentialService returns a static confidential service with an
// encrypted test credential. It is independent from the public and CIMD service
// fixtures and is suitable for compatibility scenarios.
func CIMDStaticConfidentialService() *model.ThirdpartyOAuth2ProviderEntity {
	serviceID := id.MustParseServiceID(cimdStaticServiceID)
	service := newCIMDClientService(
		serviceID,
		"Static Confidential CIMD Fixture Service",
		id.ClientID("static-confidential-cimd-fixture-client"),
		EncryptedSecret(serviceID.String(), "fixture-static-confidential-credential"),
		"",
	)
	service.ProtectedResources = []string{"https://cimd-upstream.e2e.test/static"}
	return service
}

// CIMDPublicService returns a public OAuth2 service. It has an operator-supplied
// client ID and no shared credential or broker-hosted CIMD identity.
func CIMDPublicService() *model.ThirdpartyOAuth2ProviderEntity {
	service := newCIMDClientService(
		id.MustParseServiceID(cimdPublicServiceID),
		"Public CIMD Fixture Service",
		id.ClientID("public-cimd-fixture-client"),
		model.NewAbsentSecret(),
		model.TokenEndpointAuthMethodNone,
	)
	service.ProtectedResources = []string{"https://cimd-upstream.e2e.test/public"}
	return service
}

// CIMDConfidentialService returns a private_key_jwt service with a stable
// broker-hosted HTTPS client identity and no shared credential.
func CIMDConfidentialService() *model.ThirdpartyOAuth2ProviderEntity {
	serviceID := id.MustParseServiceID(cimdConfidentialServiceID)
	service := newCIMDClientService(
		serviceID,
		"CIMD Confidential Fixture Service",
		id.ClientID(CIMDClientIDURL(serviceID)),
		model.NewAbsentSecret(),
		CIMDPrivateKeyJWTAuthMethod,
	)
	service.ProtectedResources = []string{"https://cimd-upstream.e2e.test/cimd"}
	return service
}

func newCIMDClientService(
	serviceID id.ServiceID,
	displayName string,
	clientID id.ClientID,
	secret model.Secret,
	authMethod model.TokenEndpointAuthMethod,
) *model.ThirdpartyOAuth2ProviderEntity {
	return &model.ThirdpartyOAuth2ProviderEntity{
		ID:                      serviceID,
		DisplayName:             displayName,
		ClientID:                clientID,
		Secret:                  secret,
		TokenEndpointAuthMethod: authMethod,
		Flavor:                  model.OAuth2FlavorStandard,
		IssuerURI:               "https://cimd-upstream.e2e.test",
		Discovery: model.DiscoveryConfig{
			EnableDiscovery: false,
		},
		Endpoints: model.OAuth2Endpoints{
			AuthorizeEndpoint: "https://cimd-upstream.e2e.test/oauth/authorize",
			TokenEndpoint:     "https://cimd-upstream.e2e.test/oauth/token",
		},
		Scopes: []model.OAuthScope{{
			ScopeValue:  "profile",
			Description: "Read the signed-in profile",
		}},
		ProtectedResources: []string{"https://cimd-upstream.e2e.test/api"},
		CreatedAt:          cimdFixtureTimestamp,
		UpdatedAt:          cimdFixtureTimestamp,
	}
}

// CIMDProxyConfig returns an isolated proxy-mode configuration whose end-user
// public URL is the stable HTTPS URL required by the outbound CIMD client.
// It leaves the unrelated inbound CIMD feature disabled in proxy mode.
func CIMDProxyConfig(upstreamURL string) *ports.Config {
	config := OAuth2ConfigWithUpstream(upstreamURL)
	config.Server.EndUser.PublicURL = CIMDEndUserPublicURL
	return config
}

// CIMDLocalConfig returns an isolated local-mode configuration whose end-user
// public URL is the stable HTTPS URL required by the outbound CIMD client.
func CIMDLocalConfig() *ports.Config {
	config := LocalConfig()
	config.Server.EndUser.PublicURL = CIMDEndUserPublicURL
	return config
}

// CIMDHybridConfig returns an isolated hybrid-mode configuration whose end-user
// public URL is the stable HTTPS URL required by the outbound CIMD client.
func CIMDHybridConfig(upstreamURL string) *ports.Config {
	config := HybridConfig(upstreamURL)
	config.Server.EndUser.PublicURL = CIMDEndUserPublicURL
	return config
}

// CIMDActiveSigningKey returns an immediately usable CIMD client-authentication
// key. now controls its relative lifecycle times so callers can keep tests stable.
func CIMDActiveSigningKey(now time.Time) *domainstorage.SigningKey {
	now = now.UTC()
	return newCIMDSigningKey(
		cimdActiveKeyID,
		"cimd-fixture-active-key",
		true,
		now.Add(-time.Minute),
		now.Add(-time.Hour),
	)
}

// CIMDGracePeriodSigningKeys returns the previous usable key and a newly current
// key that is already publishable but cannot sign until its activation time.
func CIMDGracePeriodSigningKeys(now time.Time) []*domainstorage.SigningKey {
	now = now.UTC()
	return []*domainstorage.SigningKey{
		newCIMDSigningKey(
			cimdGracePreviousKeyID,
			"cimd-fixture-grace-previous-key",
			false,
			now.Add(-time.Hour),
			now.Add(-2*time.Hour),
		),
		newCIMDSigningKey(
			cimdGracePendingKeyID,
			"cimd-fixture-grace-pending-key",
			true,
			now.Add(5*time.Minute),
			now,
		),
	}
}

// CIMDPromotionSigningKeys returns an active current key and an already-published
// candidate key. Promoting the candidate is an immediate transition: it does not
// need to wait for an activation grace period.
func CIMDPromotionSigningKeys(now time.Time) []*domainstorage.SigningKey {
	now = now.UTC()
	return []*domainstorage.SigningKey{
		newCIMDSigningKey(
			cimdPromotionCurrentKeyID,
			"cimd-fixture-promotion-current-key",
			true,
			now.Add(-time.Hour),
			now.Add(-2*time.Hour),
		),
		newCIMDSigningKey(
			cimdPromotionCandidateKeyID,
			"cimd-fixture-promotion-candidate-key",
			false,
			now.Add(-time.Hour),
			now.Add(-3*time.Hour),
		),
	}
}

func newCIMDSigningKey(
	keyID string,
	kid string,
	isCurrent bool,
	activatesAt time.Time,
	createdAt time.Time,
) *domainstorage.SigningKey {
	return &domainstorage.SigningKey{
		ID:                  id.MustParseSigningKeyID(keyID),
		KID:                 id.NewKeyID(kid),
		KeyDomain:           domainstorage.KeyDomainCIMDClientAuthentication,
		Algorithm:           "ES256",
		PrivateKeyEncrypted: []byte("fixture-encrypted-" + kid),
		IsCurrent:           isCurrent,
		ActivatesAt:         activatesAt,
		CreatedAt:           createdAt,
	}
}
