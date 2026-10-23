package e2e_test

import (
	"encoding/json"
	"fmt"
	"net/http"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const cimdPrivateKeyJWTMetadataEndpoint = "/.well-known/oauth-authorization-server"

func fetchCIMDPrivateKeyJWTMetadata(config *ports.Config) (map[string]any, error) {
	logger := bootstrap.TestLogger(0)
	storageFactory := bootstrap.NewStorageFactory(logger)
	store, err := storageFactory.NewTestStorage()
	if err != nil {
		return nil, err
	}
	defer func() { _ = storageFactory.CloseStorage(store) }()
	factory := bootstrap.NewServerFactory(config, logger)
	builder, err := bootstrap.NewTestServerBuilder(config, store, factory, logger)
	if err != nil {
		return nil, err
	}
	server, err := builder.Build()
	if err != nil {
		return nil, err
	}
	defer server.Close()
	response, err := server.PublicGET(cimdPrivateKeyJWTMetadataEndpoint)
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata endpoint returned HTTP %d", response.StatusCode)
	}
	var metadata map[string]any
	if err := json.NewDecoder(response.Body).Decode(&metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

var _ = Describe("CIMD private_key_jwt metadata discovery", func() {
	// Scenario US5-S1 from specs/051-private-key-jwt-auth/spec.md
	It("should advertise private_key_jwt and algorithms in local and hybrid modes", func() {
		upstream := helpers.NewMockUpstreamOAuth2Server()
		defer upstream.Close()
		for _, config := range []*ports.Config{
			fixtures.OAuth2ConfigWithCIMD(""),
			fixtures.HybridConfigWithCIMD(upstream.URL()),
		} {
			metadata, err := fetchCIMDPrivateKeyJWTMetadata(config)
			Expect(err).To(Succeed())
			methods, ok := metadata["token_endpoint_auth_methods_supported"].([]any)
			Expect(ok).To(BeTrue())
			Expect(methods).To(ContainElement("private_key_jwt"))
			algorithms, ok := metadata["token_endpoint_auth_signing_alg_values_supported"].([]any)
			Expect(ok).To(BeTrue())
			Expect(algorithms).To(ConsistOf("RS256", "ES256"))
		}
	})

	// Scenario US5-S2 from specs/051-private-key-jwt-auth/spec.md
	It("should not advertise private_key_jwt when CIMD is disabled or proxied", func() {
		upstream := helpers.NewMockUpstreamOAuth2Server()
		defer upstream.Close()
		for _, config := range []*ports.Config{
			fixtures.LocalConfig(),
			fixtures.OAuth2ConfigWithUpstream(upstream.URL()),
		} {
			metadata, err := fetchCIMDPrivateKeyJWTMetadata(config)
			Expect(err).To(Succeed())
			methods, _ := metadata["token_endpoint_auth_methods_supported"].([]any)
			Expect(methods).ToNot(ContainElement("private_key_jwt"))
			Expect(metadata).ToNot(HaveKey("token_endpoint_auth_signing_alg_values_supported"))
		}
	})

	// Scenario US5-S3 from specs/051-private-key-jwt-auth/spec.md
	It("should advertise only authentication methods and algorithms it accepts", func() {
		metadata, err := fetchCIMDPrivateKeyJWTMetadata(fixtures.OAuth2ConfigWithCIMD(""))
		Expect(err).To(Succeed())
		methods, ok := metadata["token_endpoint_auth_methods_supported"].([]any)
		Expect(ok).To(BeTrue())
		Expect(methods).To(ConsistOf("none", "client_secret_post", "private_key_jwt"))
		algorithms, ok := metadata["token_endpoint_auth_signing_alg_values_supported"].([]any)
		Expect(ok).To(BeTrue())
		Expect(algorithms).To(ConsistOf("RS256", "ES256"))
	})
})
