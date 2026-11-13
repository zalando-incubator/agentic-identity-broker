package e2e_test

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

// Every scenario here is a request or dependency failure, never a delegation decision.
var _ = Describe("AuthZEN evaluation request failures", Label("authzen-delegation"), func() {
	Context("with the in-memory broker", func() {
		var f *authzenFixture

		BeforeEach(func() {
			f = newAuthZENFixture(nil)
		})

		// Scenario US2.11 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for a subject token with an invalid signature", func() {
			parts := strings.Split(f.SubjectToken, ".")
			tampered := parts[0] + "." + parts[1] + "." + strings.Repeat("A", len(parts[2]))
			resp := f.EvaluateRaw(f.ClientAssertion, helpers.AuthZENEvaluationBody(tampered, authzenRepoResource, nil))
			expectAuthZENError(resp, http.StatusBadRequest, "invalid_request")
			Expect(resp.Raw).NotTo(ContainSubstring(tampered))
		})

		// Scenario US2.12 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for an expired subject token", func() {
			expired := f.SignSubject(time.Now().Add(-time.Hour))
			resp := f.EvaluateRaw(f.ClientAssertion, helpers.AuthZENEvaluationBody(expired, authzenRepoResource, nil))
			expectAuthZENError(resp, http.StatusBadRequest, "invalid_request")
		})

		// Scenario US2.13 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for an asserted identity subject", func() {
			body := []byte(`{"subject":{"type":"identity","id":"alice@example.com"},
				"action":{"name":"urn:ietf:params:oauth:grant-type:token-exchange"},
				"resource":{"type":"protected_resource","id":"` + authzenRepoResource + `"}}`)
			expectAuthZENError(f.EvaluateRaw(f.ClientAssertion, body), http.StatusBadRequest, "invalid_request")
		})

		// Scenario US2.14 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for malformed JSON without resolving delegation", func() {
			expectAuthZENError(f.EvaluateRaw(f.ClientAssertion, []byte(`{"subject":`)), http.StatusBadRequest, "invalid_request")
			Expect(f.Logs.Raw()).To(ContainSubstring(`"token_exchange.failure_stage":"request_validation"`))
			Expect(f.Logs.Raw()).NotTo(ContainSubstring(`"token_exchange.failure_stage":"grant_authorization"`))
			Expect(f.Logs.Raw()).NotTo(ContainSubstring(`"token_exchange.agent.id"`), "no delegation was resolved")
		})

		// Scenario US2.15 from specs/054-authzen-delegation-pip/spec.md
		It("returns 401 without a bearer client assertion", func() {
			resp := f.EvaluateRaw("", helpers.AuthZENEvaluationBody(f.SubjectToken, authzenRepoResource, nil))
			expectAuthZENError(resp, http.StatusUnauthorized, "unauthorized")
			Expect(resp.Header.Get("WWW-Authenticate")).To(Equal("Bearer"))
			Expect(resp.Raw).NotTo(ContainSubstring(f.Grant.ID.String()))
		})

		// Scenario US2.16 from specs/054-authzen-delegation-pip/spec.md
		It("returns 401 for an expired or untrusted client assertion", func() {
			for _, assertion := range []string{f.SignAssertion(time.Now().Add(-time.Hour)), "not-a-signed-assertion"} {
				resp := f.EvaluateRaw(assertion, helpers.AuthZENEvaluationBody(f.SubjectToken, authzenRepoResource, nil))
				expectAuthZENError(resp, http.StatusUnauthorized, "unauthorized")
				Expect(resp.Header.Get("WWW-Authenticate")).To(Equal(`Bearer error="invalid_token"`))
				Expect(resp.Raw).NotTo(ContainSubstring(f.Grant.ID.String()))
			}
		})

		// Scenario US2.18 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for a mismatched context.agent anchor", func() {
			resp := f.Evaluate(authzenRepoResource, map[string]any{"agent": f.OtherAgent.ID.String()})
			expectAuthZENError(resp, http.StatusBadRequest, "invalid_request")
			Expect(f.Logs.Raw()).NotTo(ContainSubstring(`"token_exchange.grant.id"`), "neither agent's grant is read")
		})

		// Scenario US2.20 from specs/054-authzen-delegation-pip/spec.md
		It("returns 400 for an unregistered protected resource", func() {
			resp := f.Evaluate("https://unregistered.example.com/api", nil)
			expectAuthZENError(resp, http.StatusBadRequest, "invalid_request")
			Expect(f.Logs.Raw()).To(ContainSubstring(`"token_exchange.failure_detail":"resource_unregistered"`))
		})
	})

	// Scenario US2.17 from specs/054-authzen-delegation-pip/spec.md
	It("returns 403 when the unchanged token-exchange caller policy rejects the gateway", func() {
		f := newAuthZENFixture(func(cfg *ports.Config) {
			cfg.TokenExchange.Authorization.CEL.Expression = `request.grant_type == "urn:ietf:params:oauth:grant-type:token-exchange" && client_assertion.sub == "another-gateway"`
		})
		resp := f.Evaluate(authzenRepoResource, nil)
		expectAuthZENError(resp, http.StatusForbidden, "forbidden")
		f.SeedSession(f.Repo, []string{"read:repo"})
		status, body := f.Exchange(authzenRepoResource)
		Expect(status).To(Equal(http.StatusForbidden), "exchange is rejected by the same policy")
		Expect(body["error"]).To(Equal("access_denied"))
	})

	Context("when the grant repository is unavailable", Label("docker"), func() {
		var (
			f        *authzenFixture
			postgres *bootstrap.PostgresFixture
		)

		BeforeEach(func() {
			Expect(bootstrap.CanAccessContainerRuntime()).To(Succeed(), "US2.19 requires a Docker or Podman runtime")
			ctx := context.Background()
			var err error
			postgres, err = bootstrap.NewPostgresFixture(ctx)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(postgres.Close(context.Background())).To(Succeed()) })
			storageConfig := ports.StorageConfig{
				Backend:  "postgres",
				Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL},
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
			}
			storage, err := storageadapter.NewAdapter(&storageConfig)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { Expect(storage.Close(context.Background())).To(Succeed()) })
			Expect(fixtures.SeedPlaceholderGrantData(ctx, storage)).To(Succeed())
			f = newAuthZENData(storage)
			f.Boot(func(cfg *ports.Config) { cfg.Storage = storageConfig })

			db, err := sql.Open("pgx", postgres.ConnectionURL)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()
			_, err = db.ExecContext(ctx, "ALTER TABLE user_grants RENAME TO user_grants_unavailable")
			Expect(err).NotTo(HaveOccurred())
		})

		// Scenario US2.19 from specs/054-authzen-delegation-pip/spec.md
		It("returns 500 during a grant-repository outage", func() {
			resp := f.Evaluate(authzenRepoResource, nil)
			expectAuthZENError(resp, http.StatusInternalServerError, "server_error")
			Expect(resp.Raw).NotTo(ContainSubstring("consent_url"))
			Expect(f.Logs.Raw()).To(ContainSubstring(`"token_exchange.failure_detail":"grant_repository_unavailable"`))
		})
	})
})
