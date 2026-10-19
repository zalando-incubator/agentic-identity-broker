package aibctl_test

import (
	"os"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("US1 Contexts and configuration", func() {
	var h *harness

	BeforeEach(func() { h = newHarness() })
	AfterEach(func() { h.close() })

	// US1-S1 from specs/048-admin-cli/spec.md
	It("stores a context with owner-only permissions and makes it current", func() {
		if runtime.GOOS != "windows" {
			info, err := os.Stat(h.configPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o600)))
		}
		Expect(h.mustRun("context", "current").stdout).To(Equal(e2eContextName + "\n"))

		r := h.mustRun("context", "list")
		Expect(r.stdout).To(ContainSubstring(h.server.BaseURL()))
		Expect(r.stdout).To(ContainSubstring("principal"))

		view := h.mustRun("config", "view").stdout
		Expect(view).To(ContainSubstring("current_context: " + e2eContextName))
		Expect(view).To(ContainSubstring("principal: admin@example.com"))
	})

	// US1-S2 from specs/048-admin-cli/spec.md
	It("reaches the admin server health endpoint through the current context", func() {
		r := h.mustRun("health")
		Expect(r.stdout).To(ContainSubstring("Status:"))
		Expect(r.stdout).To(ContainSubstring("healthy"))

		health := h.runJSON("health")
		Expect(health["status"]).To(Equal("healthy"))
	})

	// US1-S3 from specs/048-admin-cli/spec.md
	It("fails with the usage exit code when no context is configured", func() {
		Expect(h.mustRun("context", "delete", e2eContextName, "--yes").code).To(Equal(0))
		r := h.run("", "agent", "list")
		Expect(r.code).To(Equal(2))
		Expect(r.stderr).To(ContainSubstring("no context"))
		Expect(r.stdout).To(BeEmpty())
	})
})
