package e2e_test

import (
	"os"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const e2eUpstreamBaseURLEnv = "E2E_UPSTREAM_BASE_URL"

var suiteUpstream *helpers.MockUpstreamOAuth2Server

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "OAuth2 Authorization Server E2E Suite")
}

func TestCredentialBrokerProcess(t *testing.T) {
	if os.Getenv("AIB_CREDENTIAL_BROKER_HELPER") != "1" {
		return
	}
	if err := bootstrap.RunCredentialBrokerProcess(); err != nil {
		t.Fatal(err)
	}
}

var _ = SynchronizedBeforeSuite(func() []byte {
	suiteUpstream = helpers.NewMockUpstreamOAuth2Server()
	return []byte(suiteUpstream.URL())
}, func(data []byte) {
	if err := os.Setenv(e2eUpstreamBaseURLEnv, string(data)); err != nil {
		panic(err)
	}
})

var _ = SynchronizedAfterSuite(func() {
	_ = os.Unsetenv(e2eUpstreamBaseURLEnv)
}, func() {
	if suiteUpstream != nil {
		suiteUpstream.Close()
	}
})
