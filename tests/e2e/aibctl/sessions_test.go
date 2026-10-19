package aibctl_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("US9 Session sweep", func() {
	var h *harness

	BeforeEach(func() { h = newHarness() })
	AfterEach(func() { h.close() })

	// US9-S1 from specs/048-admin-cli/spec.md
	It("previews and runs a sweep, and reports invalid durations", func() {
		preview := h.runJSON("session", "sweep", "--dry-run", "--lookahead", "PT10M", "--page-size", "100")
		Expect(preview["dry_run"]).To(BeTrue())
		Expect(preview["total_evaluated"]).To(BeNumerically("==", 0))
		actual := h.runJSON("session", "sweep")
		Expect(actual["dry_run"]).To(BeFalse())

		r := h.run("", "session", "sweep", "--lookahead", "10m")
		Expect(r.code).To(Equal(6), r.stderr)
		Expect(r.stderr).To(ContainSubstring("ISO 8601"))
		Expect(r.stdout).To(BeEmpty())
	})
})
