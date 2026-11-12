package e2e_test

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/livecheck"
)

// TestMain and the BeforeEach node send the telemetry of the suite to Weaver when
// `just telemetry-live-check` runs it. They do nothing otherwise.
func TestMain(m *testing.M) {
	os.Exit(livecheck.Run(m))
}

var _ = BeforeEach(livecheck.Restore)
