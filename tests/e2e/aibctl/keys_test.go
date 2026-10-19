package aibctl_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("US5 Client credentials", func() {
	var h *harness

	BeforeEach(func() {
		h = newHarness()
		h.seedService("github", "GitHub")
		h.mustRun("ps", "create", "--name", "GitHub Read", "--description", "d", "--canonical-id", "github-read", "--service-scope", "github=read")
		h.mustRun("agent", "create", "--display-name", "Research", "--description", "d", "--canonical-id", "research-agent", "--permission-set", "github-read")
	})
	AfterEach(func() { h.close() })

	// US5-S1 from specs/048-admin-cli/spec.md
	It("creates, inspects, rotates, and revokes credentials by agent canonical ID", func() {
		Expect(h.run("", "agent", "client-credentials", "get", "research-agent").code).To(Equal(4))
		Expect(h.run("", "agent", "client-credentials", "rotate", "research-agent", "--yes").code).To(Equal(4))

		created := h.runJSON("agent", "client-credentials", "create", "research-agent")
		Expect(created["client_secret"]).NotTo(BeEmpty())
		Expect(created["previous_invalidated_at"]).To(BeNil())

		r := h.run("", "agent", "client-credentials", "create", "research-agent")
		Expect(r.code).To(Equal(5), r.stderr)
		Expect(r.stderr).To(ContainSubstring("already has client credentials"))

		info := h.runJSON("agent", "client-credentials", "get", "research-agent")
		Expect(info["client_id"]).To(Equal(created["client_id"]))
		Expect(info).NotTo(HaveKey("client_secret"))

		r = h.run("", "agent", "client-credentials", "rotate", "research-agent")
		Expect(r.code).To(Equal(2), r.stderr)
		Expect(r.stderr).To(ContainSubstring("requires confirmation"))
		rotated := h.runJSON("agent", "client-credentials", "rotate", "research-agent", "--yes")
		Expect(rotated["client_secret"]).NotTo(Equal(created["client_secret"]))
		Expect(rotated["previous_invalidated_at"]).NotTo(BeNil())

		h.mustRun("agent", "client-credentials", "delete", "research-agent", "--yes")
		Expect(h.run("", "agent", "client-credentials", "get", "research-agent").code).To(Equal(4))
	})
})

var _ = Describe("US6 Signing keys", func() {
	var h *harness

	BeforeEach(func() { h = newHarness() })
	AfterEach(func() { h.close() })

	// US6-S1 from specs/048-admin-cli/spec.md
	It("adds a signing key, promotes it, and refuses to delete the current key", func() {
		first := h.runJSON("signing-key", "create")
		Expect(first["kid"]).NotTo(BeEmpty())
		second := h.runJSON("signing-key", "create", "--algorithm", "ES256")
		Expect(second["kid"]).NotTo(Equal(first["kid"]))

		rows := tableRows(h.mustRun("signing-key", "list").stdout)
		Expect(rows).To(HaveLen(4), "the server bootstraps one key; two were added")
		Expect(rows[0]).To(Equal([]string{"KID", "ALGORITHM", "CURRENT", "ACTIVATES_AT", "CREATED_AT"}))

		promoted := h.runJSON("signing-key", "set-current", second["kid"].(string))
		Expect(promoted["is_current"]).To(BeTrue())

		r := h.run("", "signing-key", "delete", second["kid"].(string), "-y")
		Expect(r.code).To(Equal(5), r.stderr)
		Expect(r.stderr).To(ContainSubstring("HTTP 409"))

		h.mustRun("signing-key", "delete", first["kid"].(string), "-y")
		Expect(tableRows(h.mustRun("signing-key", "list").stdout)).To(HaveLen(3))

		r = h.mustRun("__complete", "signing-key", "delete", "")
		Expect(r.stdout).To(ContainSubstring(second["kid"].(string) + "\tES256 (current)\n"))
	})
})

var _ = Describe("US6 CIMD client keys", func() {
	var h *harness

	BeforeEach(func() { h = newHarness() })
	AfterEach(func() { h.close() })

	// US6-S2 from specs/048-admin-cli/spec.md
	It("manages CIMD client keys with the same verbs as signing keys", func() {
		before := len(tableRows(h.mustRun("cimd-client-key", "list").stdout))
		created := h.runJSON("cimd-client-key", "create")
		Expect(created["kid"]).NotTo(BeEmpty())
		Expect(tableRows(h.mustRun("cimd-client-key", "list").stdout)).To(HaveLen(before + 1))

		promoted := h.runJSON("cimd-client-key", "set-current", created["kid"].(string))
		Expect(promoted["is_current"]).To(BeTrue())
		Expect(h.run("", "cimd-client-key", "delete", created["kid"].(string), "-y").code).To(Equal(5))
	})
})
