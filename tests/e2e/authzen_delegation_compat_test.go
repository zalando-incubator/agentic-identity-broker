package e2e_test

import (
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

func enduserOpenAPI() map[string]any {
	_, thisFile, _, ok := runtime.Caller(0)
	Expect(ok).To(BeTrue())
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "..", "..", "api", "enduser", "openapi.yaml"))
	Expect(err).NotTo(HaveOccurred())
	var document map[string]any
	Expect(yaml.Unmarshal(raw, &document)).To(Succeed())
	return document
}

var _ = Describe("Token exchange compatibility during the AuthZEN deprecation window", Label("authzen-delegation"), func() {
	// Scenario US4.6 from specs/054-authzen-delegation-pip/spec.md
	It("keeps principal, agent_id, and deprecated granted_permission_sets on exchange", func() {
		f := newAuthZENFixture(nil)
		f.SeedSession(f.Repo, []string{"read:repo"})

		status, body := f.Exchange(authzenRepoResource)

		Expect(status).To(Equal(http.StatusOK))
		Expect(body["principal"]).To(Equal("alice@example.com"))
		Expect(body["agent_id"]).To(Equal(f.Agent.ID.String()))
		Expect(body["granted_permission_sets"]).To(Equal(map[string]any{
			f.Reader.ID.String(): []any{f.Repo.ID.String()},
			f.Writer.ID.String(): []any{f.Repo.ID.String(), f.Wiki.ID.String()},
		}))

		document := enduserOpenAPI()
		Expect(document["info"].(map[string]any)["version"]).To(Equal("1.2.0"))
		properties := document["components"].(map[string]any)["schemas"].(map[string]any)["TokenExchangeResponse"].(map[string]any)["properties"].(map[string]any)
		Expect(properties["granted_permission_sets"]).To(HaveKeyWithValue("deprecated", true))
		Expect(properties["principal"]).NotTo(HaveKey("deprecated"))
		Expect(properties["agent_id"]).NotTo(HaveKey("deprecated"))
	})
})
