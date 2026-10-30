// Package extproc_test contains the E2E acceptance test suite for the ExtProc Token Exchange Service.
// Tests validate all 12 acceptance scenarios from specs/20260223-120000-extproc-token-exchange/spec.md.
//
// This suite is SEPARATE from the main E2E suite and does NOT reuse the existing E2E harness
// as required by FR-017.
//
// Test Execution:
//
//	cd tests/e2e/extproc && ginkgo -v ./...
//
// Red Phase: All tests compile but FAIL because ExtProc service implementation does not exist yet.
// Green Phase: Tests turn GREEN as implementation satisfies each acceptance scenario.
package extproc_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestExtProcTokenExchange(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ExtProc Token Exchange E2E Suite")
}
