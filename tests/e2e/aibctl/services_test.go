package aibctl_test

import (
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("US3 Services and protected resources", func() {
	var h *harness

	BeforeEach(func() { h = newHarness() })
	AfterEach(func() { h.close() })

	createGitHub := func() {
		h.mustRun("service", "create", "--display-name", "GitHub", "--canonical-id", "github",
			"--client-id", "gh-client", "--client-secret", "gh-secret", "--oauth2-flavor", "github", "--issuer-uri", "https://github.com",
			"--token-endpoint", "https://github.com/login/oauth/access_token", "--authorize-endpoint", "https://github.com/login/oauth/authorize",
			"--scope", "repo=Repository access", "--protected-resource", "https://api.github.com/")
	}

	// US3-S1 from specs/048-admin-cli/spec.md
	It("registers a service from flags, redacts its secret, and lists it by canonical ID", func() {
		createGitHub()
		rows := tableRows(h.mustRun("service", "list").stdout)
		Expect(rows).To(HaveLen(2))
		Expect(rows[1][0]).To(Equal("github"))
		Expect(rows[1][5]).To(Equal("1"))

		service := h.runJSON("service", "get", "github")
		Expect(service["client_secret"]).To(Equal("REDACTED"))
		Expect(service["client_id"]).To(Equal("gh-client"))
		Expect(service["protected_resources"]).To(Equal([]any{"https://api.github.com"}))
		Expect(h.runJSON("service", "get", service["id"].(string))["canonical_id"]).To(Equal("github"))
	})

	// US3-S2 from specs/048-admin-cli/spec.md
	It("adds, renames, and removes protected resources and rejects a URI another service owns", func() {
		createGitHub()
		h.mustRun("service", "create", "--display-name", "Other", "--canonical-id", "other", "--client-id", "o", "--client-secret", "s",
			"--issuer-uri", "https://other.example.com", "--token-endpoint", "https://other.example.com/t", "--authorize-endpoint", "https://other.example.com/a")

		r := h.mustRun("service", "protected-resource", "add", "github", "https://uploads.github.com/", "https://api.github.com/")
		Expect(r.stderr).To(ContainSubstring("Added https://uploads.github.com."))
		Expect(r.stderr).To(ContainSubstring("https://api.github.com was already protected."))
		Expect(tableRows(r.stdout)).To(HaveLen(3))

		r = h.run("", "service", "pr", "add", "other", "https://api.github.com/")
		Expect(r.code).To(Equal(5), r.stderr)

		r = h.mustRun("service", "pr", "rename", "github", "https://uploads.github.com", "https://uploads.github.com/v2")
		Expect(r.stdout).To(ContainSubstring("https://uploads.github.com/v2"))
		Expect(r.stdout).NotTo(ContainSubstring("https://uploads.github.com\n"))

		r = h.mustRun("service", "pr", "remove", "github", "https://uploads.github.com/v2", "--yes")
		Expect(tableRows(r.stdout)).To(Equal([][]string{{"RESOURCE"}, {"https://api.github.com"}}))

		r = h.mustRun("__complete", "service", "pr", "remove", "github", "")
		Expect(r.stdout).To(ContainSubstring("https://api.github.com\n"))
	})

	// US3-S3 from specs/048-admin-cli/spec.md
	It("updates a service conditionally and never sends the redacted secret back", func() {
		createGitHub()
		doc := h.mustRun("service", "get", "github", "-o", "yaml").stdout
		Expect(doc).To(ContainSubstring("client_secret: REDACTED"))

		r := h.run(doc, "service", "update", "github", "-f", "-")
		Expect(r.code).To(Equal(2))
		Expect(r.stderr).To(ContainSubstring("redacted placeholder"))

		r = h.run(strings.Replace(doc, "client_secret: REDACTED", "client_secret: rotated", 1), "service", "update", "github", "-f", "-")
		Expect(r.code).To(Equal(0), r.stderr)

		r = h.run("", "service", "update", "github", "--display-name", "GitHub Enterprise")
		Expect(r.code).To(Equal(2), "a static secret must be re-supplied")
		Expect(r.stderr).To(ContainSubstring("--client-secret"))

		r = h.mustRun("service", "update", "github", "--display-name", "GitHub Enterprise", "--client-secret", "rotated-again")
		Expect(h.runJSON("service", "get", "github")["display_name"]).To(Equal("GitHub Enterprise"))

		r = h.run("", "service", "update", "github", "--display-name", "Stale", "--client-secret", "x", "--if-match", `"1"`)
		Expect(r.code).To(Equal(5), r.stderr)
		Expect(h.runJSON("service", "get", "github")["display_name"]).To(Equal("GitHub Enterprise"))
	})

	// US3-S4 from specs/048-admin-cli/spec.md
	It("deletes a service after confirmation", func() {
		createGitHub()
		Expect(h.run("", "service", "delete", "github").code).To(Equal(2))
		h.mustRun("service", "delete", "github", "-y")
		Expect(h.run("", "service", "get", "github").code).To(Equal(4))
		Expect(tableRows(h.mustRun("service", "list").stdout)).To(HaveLen(1))
	})
})
