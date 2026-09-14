package e2e_test

import (
	"context"
	"log/slog"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
)

var _ = Describe("Portless Redirect URI — Opaque Agent", func() {
	var (
		logger         *slog.Logger
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		server         *bootstrap.TestServer
		agent          *storage.Agent
	)

	BeforeEach(func() {
		logger = bootstrap.TestLogger(slog.LevelInfo)
		storageFactory = bootstrap.NewStorageFactory(logger)

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		// SC-006: opaque (UUID) agent — no ClientURIs, no ClientID, explicit :3000 in redirect_uris.
		agent = fixtures.LocalAgent()
		agent.DisplayName = "Opaque Portless Agent"
		agent.Description = "Opaque agent with explicit-port localhost redirect URI for 028b SC-006 testing"
		agent.RedirectURIs = []string{"http://localhost:3000/callback"}
		Expect(testStorage.Agents().Create(context.Background(), agent)).To(Succeed())

		config := fixtures.LocalConfig()
		sf := bootstrap.NewServerFactory(config, logger)
		appInstance, err := sf.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())

		server, err = bootstrap.NewEndUserTestServer(appInstance, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// SC-006 from specs/028b-portless-registration/spec.md
	It("accepts a different ephemeral port for an opaque agent with explicit-port loopback redirect URI", func() {
		completeAuthorizationCodeFlow(
			server,
			testStorage,
			fixtures.DefaultPrincipal().String(),
			agent.ID.String(),
			"http://localhost:9999/callback",
			func(data map[string]any) {
				_, hasCIMDMetadata := data["cimd_metadata"]
				Expect(hasCIMDMetadata).To(BeFalse(), "opaque client flows must not surface cimd_metadata")
			},
		)
	})
})
