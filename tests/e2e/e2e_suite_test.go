package e2e_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const e2eUpstreamBaseURLEnv = "E2E_UPSTREAM_BASE_URL"

var suiteUpstream *helpers.MockUpstreamOAuth2Server
var refreshSuiteTestingT *testing.T

func TestE2E(t *testing.T) {
	refreshSuiteTestingT = t
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := integrationbootstrap.TerminateSharedPostgres(ctx); err != nil {
			t.Errorf("close shared refresh PostgreSQL fixture: %v", err)
		}
	})
	RegisterFailHandler(Fail)
	RunSpecs(t, "OAuth2 Authorization Server E2E Suite")
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
