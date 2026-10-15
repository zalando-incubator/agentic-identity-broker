package aibctl_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const agentDocument = `display_name: Research Agent
description: Reads repositories on the user's behalf
canonical_id: research-agent
client_id: research-client
permission_sets:
  - permission_set_id: github-read
    requirement_type: mandatory
service_requirements:
  - service_id: github
    requirement_type: mandatory
    required_scopes: [read]
redirect_uris:
  - https://research.example.com/callback
`

var _ = Describe("US2 Agents", func() {
	var h *harness

	BeforeEach(func() {
		h = newHarness()
		h.seedService("github", "GitHub")
		h.mustRun("permission-set", "create", "--name", "GitHub Read", "--description", "Read GitHub", "--canonical-id", "github-read", "--service-scope", "github=read:mandatory")
	})
	AfterEach(func() { h.close() })

	createAgentFromFile := func() {
		path := filepath.Join(GinkgoT().TempDir(), "agent.yaml")
		Expect(os.WriteFile(path, []byte(agentDocument), 0o600)).To(Succeed())
		r := h.mustRun("agent", "create", "-f", path)
		Expect(r.stderr).To(ContainSubstring(`Agent "research-agent" created.`))
	}

	// US2-S1 from specs/048-admin-cli/spec.md
	It("creates an agent from a document and lists it by canonical ID", func() {
		createAgentFromFile()

		rows := tableRows(h.mustRun("agent", "list").stdout)
		Expect(rows).To(HaveLen(2))
		Expect(rows[0][0]).To(Equal("ID"))
		Expect(rows[1][0]).To(Equal("research-agent"))
		Expect(rows[1][1]).To(Equal("Research"))
		Expect(rows[1]).NotTo(ContainElement(MatchRegexp(`^[0-9a-f-]{36}$`)), "UUIDs are hidden by default")

		rows = tableRows(h.mustRun("agent", "list", "--show-uuid").stdout)
		Expect(rows[0]).To(ContainElement("UUID"))
		Expect(rows[1][len(rows[1])-1]).To(MatchRegexp(`^[0-9a-f-]{36}$`))
	})

	// US2-S2 from specs/048-admin-cli/spec.md
	It("resolves an agent by canonical ID or UUID and renders nested references canonically", func() {
		createAgentFromFile()
		byCanonical := h.runJSON("agent", "get", "research-agent")
		uuid := byCanonical["id"].(string)
		Expect(uuid).To(MatchRegexp(`^[0-9a-f-]{36}$`))
		byUUID := h.runJSON("agent", "get", uuid)
		Expect(byUUID["canonical_id"]).To(Equal("research-agent"))

		permissionSets := byCanonical["permission_sets"].([]any)
		Expect(permissionSets[0].(map[string]any)["permission_set_id"]).To(Equal("github-read"))
		requirements := byCanonical["service_requirements"].([]any)
		Expect(requirements[0].(map[string]any)["service_id"]).To(Equal("github"))

		r := h.mustRun("agent", "get", "research-agent")
		Expect(r.stdout).To(ContainSubstring("ID:                      research-agent"))
		Expect(r.stdout).To(ContainSubstring("UUID:                    " + uuid))
	})

	// US2-S3 from specs/048-admin-cli/spec.md
	It("clears a canonical ID with a flag and falls back to the UUID", func() {
		createAgentFromFile()
		r := h.mustRun("agent", "update", "research-agent", "--clear-canonical-id", "--display-name", "Renamed")
		Expect(r.stderr).To(ContainSubstring("updated"))

		rows := tableRows(h.mustRun("agent", "list").stdout)
		Expect(rows[1][0]).To(MatchRegexp(`^[0-9a-f-]{36}$`))
		Expect(rows[1][1]).To(Equal("Renamed"))
		Expect(h.run("", "agent", "get", "research-agent").code).To(Equal(4))

		agent := h.runJSON("agent", "get", rows[1][0])
		Expect(agent["canonical_id"]).To(BeNil())
		Expect(agent["client_id"]).To(Equal("research-client"), "untouched fields are preserved")
	})

	// US2-S4 from specs/048-admin-cli/spec.md
	It("round-trips an agent through get and update with a document", func() {
		createAgentFromFile()
		doc := h.mustRun("agent", "get", "research-agent", "-o", "yaml").stdout
		Expect(doc).To(ContainSubstring("canonical_id: research-agent"))

		r := h.run(doc, "agent", "update", "research-agent", "-f", "-")
		Expect(r.code).To(Equal(0), r.stderr)

		agent := h.runJSON("agent", "get", "research-agent")
		Expect(agent["display_name"]).To(Equal("Research Agent"))
		Expect(agent["redirect_uris"]).To(Equal([]any{"https://research.example.com/callback"}))
		Expect(agent["service_requirements"].([]any)[0].(map[string]any)["service_name"]).To(Equal("GitHub"))
	})

	// US2-S5 from specs/048-admin-cli/spec.md
	It("deletes an agent only after confirmation", func() {
		createAgentFromFile()
		r := h.run("", "agent", "delete", "research-agent")
		Expect(r.code).To(Equal(2))
		Expect(h.run("", "agent", "get", "research-agent").code).To(Equal(0), "refused delete leaves the agent")

		r = h.mustRun("agent", "delete", "research-agent", "--yes")
		Expect(r.stdout).To(BeEmpty())
		Expect(r.stderr).To(ContainSubstring("deleted"))
		Expect(h.run("", "agent", "get", "research-agent").code).To(Equal(4))
	})

	// US2-S6 from specs/048-admin-cli/spec.md
	It("reports a server-side validation failure with the validation exit code", func() {
		r := h.run("", "agent", "create", "--display-name", "Broken", "--description", "d", "--permission-set", "does-not-exist")
		Expect(r.code).To(Equal(6))
		Expect(r.stderr).To(HavePrefix("Error: "))
		Expect(r.stdout).To(BeEmpty())
	})
})

var _ = Describe("US4 Permission sets", func() {
	var h *harness

	BeforeEach(func() {
		h = newHarness()
		h.seedService("github", "GitHub")
		h.seedService("jira", "Jira")
	})
	AfterEach(func() { h.close() })

	// US4-S1 from specs/048-admin-cli/spec.md
	It("creates permission sets with service canonical IDs and filters the list by service", func() {
		h.mustRun("ps", "create", "--name", "GitHub Read", "--description", "d", "--canonical-id", "github-read", "--service-scope", "github=read:mandatory")
		h.mustRun("ps", "create", "--name", "Jira Read", "--description", "d", "--service-scope", "jira=read")

		all := h.runJSONList("permission-set", "list")
		Expect(all).To(HaveLen(2))
		github := h.runJSONList("permission-set", "list", "--service", "github")
		Expect(github).To(HaveLen(1))
		Expect(github[0]["canonical_id"]).To(Equal("github-read"))
		Expect(github[0]["service_scopes"].([]any)[0].(map[string]any)["service_id"]).To(Equal("github"))

		rows := tableRows(h.mustRun("permission-set", "list").stdout)
		Expect(rows).To(HaveLen(3))
		var ids []string
		for _, row := range rows[1:] {
			ids = append(ids, row[0])
		}
		Expect(ids).To(ContainElement("github-read"))
		Expect(ids).To(ContainElement(MatchRegexp(`^[0-9a-f-]{36}$`)), "a set without canonical ID shows its UUID")
	})

	// US4-S2 from specs/048-admin-cli/spec.md
	It("refuses to delete a permission set that an agent references", func() {
		h.mustRun("ps", "create", "--name", "GitHub Read", "--description", "d", "--canonical-id", "github-read", "--service-scope", "github=read")
		h.mustRun("agent", "create", "--display-name", "A", "--description", "d", "--permission-set", "github-read:mandatory")

		r := h.run("", "permission-set", "delete", "github-read", "-y")
		Expect(r.code).To(Equal(5))
		Expect(r.stderr).To(ContainSubstring("HTTP 409"))

		h.mustRun("ps", "update", "github-read", "--name", "GitHub Read Only", "--service-scope", "github=read,write:mandatory")
		updated := h.runJSON("ps", "get", "github-read")
		Expect(updated["name"]).To(Equal("GitHub Read Only"))
		Expect(updated["service_scopes"].([]any)[0].(map[string]any)["scopes"]).To(Equal([]any{"read", "write"}))
	})
})

var _ = Describe("US7 Shell completion", func() {
	var h *harness

	BeforeEach(func() {
		h = newHarness()
		h.seedService("github", "GitHub")
		h.mustRun("ps", "create", "--name", "GitHub Read", "--description", "d", "--canonical-id", "github-read", "--service-scope", "github=read")
		h.mustRun("agent", "create", "--display-name", "Research", "--description", "d", "--canonical-id", "research-agent", "--permission-set", "github-read")
		h.mustRun("agent", "create", "--display-name", "Plain", "--description", "d", "--permission-set", "github-read")
	})
	AfterEach(func() { h.close() })

	// US7-S1 from specs/048-admin-cli/spec.md
	It("completes identifiers from the API, preferring canonical IDs", func() {
		r := h.mustRun("__complete", "agent", "get", "")
		Expect(r.stdout).To(ContainSubstring("research-agent\tResearch\n"))
		Expect(r.stdout).To(MatchRegexp(`(?m)^[0-9a-f-]{36}\tPlain$`))
		Expect(r.stdout).To(HaveSuffix(":4\n"))

		r = h.mustRun("__complete", "agent", "get", "res")
		Expect(r.stdout).NotTo(ContainSubstring("Plain"))

		r = h.mustRun("__complete", "permission-set", "list", "--service", "")
		Expect(r.stdout).To(ContainSubstring("github\tGitHub\n"))

		r = h.mustRun("__complete", "agent", "update", "--permission-set", "")
		Expect(r.stdout).To(ContainSubstring("github-read\tGitHub Read\n"))

		Expect(h.mustRun("completion", "zsh").stdout).To(ContainSubstring("compdef"))
	})
})
