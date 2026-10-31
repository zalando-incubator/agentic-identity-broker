package e2e_test

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func maxAgeOf(resp *helpers.AuthZENResponse) int {
	value := resp.Header.Get("Cache-Control")
	Expect(value).To(HavePrefix("private, max-age="))
	seconds, err := strconv.Atoi(strings.TrimPrefix(value, "private, max-age="))
	Expect(err).NotTo(HaveOccurred())
	return seconds
}

var _ = Describe("AuthZEN delegation evaluation", Label("authzen-delegation"), func() {
	var f *authzenFixture

	Context("with token exchange configured", func() {
		BeforeEach(func() {
			f = newAuthZENFixture(nil)
		})

		// Scenario US1.1 from specs/054-authzen-delegation-pip/spec.md
		It("publishes the PDP identifier and evaluation endpoint from the public URL", func() {
			resp, err := helpers.GetAuthZENConfiguration(&http.Client{}, f.Enduser.BaseURL())
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.StatusCode).To(Equal(http.StatusOK))
			Expect(resp.Header.Get("Content-Type")).To(Equal("application/json"))
			Expect(resp.Body).To(Equal(map[string]any{
				"policy_decision_point":      "https://broker.example.com",
				"access_evaluation_endpoint": "https://broker.example.com/access/v1/evaluation",
			}))
		})

		// Scenario US1.2 from specs/054-authzen-delegation-pip/spec.md
		It("permits with the same context with a matching anchor, without context.agent, and without context", func() {
			anchored := f.Evaluate(authzenRepoResource, map[string]any{"agent": f.Agent.ID.String()})
			withoutAgent := f.Evaluate(authzenRepoResource, map[string]any{})
			withoutContext := f.Evaluate(authzenRepoResource, nil)

			expected := permitContext(withoutContext)
			Expect(permitContext(anchored)).To(Equal(expected))
			Expect(permitContext(withoutAgent)).To(Equal(expected))
			Expect(expected["agent"]).To(Equal(map[string]any{"id": f.Agent.ID.String(), "canonical_id": "data-assistant"}))
			Expect(expected["subject"]).To(Equal(map[string]any{"id": "alice@example.com"}))
			for _, resp := range []*helpers.AuthZENResponse{anchored, withoutAgent, withoutContext} {
				Expect(maxAgeOf(resp)).To(BeNumerically("<=", 60))
			}
			f.SeedSession(f.Repo, []string{"read:repo"})
			status, _ := f.Exchange(authzenRepoResource)
			Expect(status).To(Equal(http.StatusOK), "the same caller policy authorizes exchange")
		})

		// Scenario US1.3 from specs/054-authzen-delegation-pip/spec.md
		It("permits without a provider session while exchange returns its session error", func() {
			permitContext(f.Evaluate(authzenRepoResource, nil))
			status, body := f.Exchange(authzenRepoResource)
			Expect(status).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_grant"))
			Expect(f.Upstream.GetTokenRequests()).To(BeEmpty(), "evaluation never refreshes a provider credential")
		})

		// Scenario US1.4 from specs/054-authzen-delegation-pip/spec.md
		It("reports only ceiling-capped effective scopes in resource.scopes", func() {
			resource := permitContext(f.Evaluate(authzenRepoResource, nil))["resource"].(map[string]any)
			Expect(resource["scopes"]).To(Equal([]any{"read:repo"}))
			f.SeedSession(f.Repo, []string{"read:repo"})
			status, body := f.Exchange(authzenRepoResource)
			Expect(status).To(Equal(http.StatusOK))
			Expect(body["scope"]).To(Equal("read:repo"), "exchange applies the same ceiling")
		})

		// Scenario US1.5 from specs/054-authzen-delegation-pip/spec.md
		It("resolves an equivalent resource URI to the same service and decision as exchange", func() {
			canonical := permitContext(f.Evaluate(authzenRepoResource, nil))
			equivalent := permitContext(f.Evaluate(authzenRepoResource+"/", nil))
			Expect(equivalent).To(Equal(canonical))
			Expect(equivalent["resource"].(map[string]any)["resource_uri"]).To(Equal(authzenRepoResource))
			f.SeedSession(f.Repo, []string{"read:repo"})
			status, _ := f.Exchange(authzenRepoResource + "/")
			Expect(status).To(Equal(http.StatusOK))
		})

		// Scenario US1.6 from specs/054-authzen-delegation-pip/spec.md
		It("bounds max-age by grant expiry and denies with grant_expired at expiry", func() {
			validUntil := time.Now().Add(15 * time.Second).Truncate(time.Second)
			f.Grant.ValidUntil = &validUntil
			f.SaveGrant()
			resp := f.Evaluate(authzenRepoResource, nil)
			Expect(maxAgeOf(resp)).To(BeNumerically("<=", 15))
			Expect(permitContext(resp)["grant"]).To(HaveKeyWithValue("valid_until", validUntil.UTC().Format(time.RFC3339)))

			expired := time.Now().Add(-time.Second)
			f.Grant.ValidUntil = &expired
			Expect(f.Storage.UserGrants().Create(context.Background(), f.Grant)).To(Succeed())
			f.ExpectDenial(f.Evaluate(authzenRepoResource, nil), "grant_expired")
		})

		// Scenario US1.8 from specs/054-authzen-delegation-pip/spec.md
		It("omits unset canonical_id fields everywhere and keeps configured ones", func() {
			ctx := permitContext(f.Evaluate(authzenRepoResource, nil))
			Expect(ctx["resource"]).To(HaveKeyWithValue("canonical_id", "repo-api"))
			Expect(ctx["grant"]).NotTo(HaveKey("valid_until"))
			sets := ctx["permission_sets"].([]any)
			Expect(sets).To(HaveLen(2))
			reader, writer := sets[0].(map[string]any), sets[1].(map[string]any)
			Expect(reader).To(HaveKeyWithValue("canonical_id", "repo-reader"))
			Expect(writer).NotTo(HaveKey("canonical_id"))
			services := writer["services"].([]any)
			Expect(services[0]).To(Equal(map[string]any{"id": f.Repo.ID.String(), "canonical_id": "repo-api", "scopes": []any{"read:repo", "write:repo"}}))
			Expect(services[1]).To(Equal(map[string]any{"id": f.Wiki.ID.String(), "scopes": []any{}}), "unset canonical IDs are omitted, never null or replaced")

			f.Agent.CanonicalID = nil
			f.SaveAgent()
			agent := permitContext(f.Evaluate(authzenRepoResource, nil))["agent"]
			Expect(agent).To(Equal(map[string]any{"id": f.Agent.ID.String()}))
		})
	})

	// Scenario US1.7 from specs/054-authzen-delegation-pip/spec.md
	It("serves neither route without token exchange and both routes once it is configured", func() {
		data := newAuthZENData(nil)
		data.Boot(func(cfg *ports.Config) { cfg.TokenExchange = ports.TokenExchangeConfig{} })
		discovery, err := helpers.GetAuthZENConfiguration(&http.Client{}, data.Enduser.BaseURL())
		Expect(err).NotTo(HaveOccurred())
		Expect(discovery.StatusCode).To(Equal(http.StatusNotFound))
		evaluation, err := helpers.DoAuthZENEvaluation(&http.Client{}, data.Enduser.BaseURL(), data.ClientAssertion, "",
			helpers.AuthZENEvaluationBody(data.SubjectToken, authzenRepoResource, nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(evaluation.StatusCode).To(Equal(http.StatusNotFound))
		Expect(evaluation.Raw).NotTo(ContainSubstring("<html"))

		data.Boot(nil)
		discovery, err = helpers.GetAuthZENConfiguration(&http.Client{}, data.Enduser.BaseURL())
		Expect(err).NotTo(HaveOccurred())
		Expect(discovery.StatusCode).To(Equal(http.StatusOK))
		permitContext(data.Evaluate(authzenRepoResource, nil))
	})
})
