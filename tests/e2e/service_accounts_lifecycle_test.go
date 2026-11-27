package e2e_test

import (
	"log/slog"
	"net/http"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Service accounts", func() {
	const expiryTimeout, expiryPolling = 15 * time.Second, 200 * time.Millisecond

	var (
		server         *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		accountPath    string
		statusPath     string
		token          string
		created        helpers.ServiceAccount
	)

	read := func(path string) helpers.CapturedResponse {
		return serviceAccountCall(server, http.MethodGet, path, "", nil)
	}

	readAccount := func() helpers.ServiceAccount {
		response := read(accountPath)
		Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
		account, err := response.ServiceAccount()
		Expect(err).To(Succeed())
		return account
	}

	readStatus := func() helpers.ServiceAccountStatus {
		response := read(statusPath)
		Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
		status, err := response.Status()
		Expect(err).To(Succeed())
		return status
	}

	update := func(ifMatch string, body map[string]any) helpers.CapturedResponse {
		return serviceAccountCall(server, http.MethodPut, accountPath, ifMatch, body)
	}

	registerAccount := func(jwt string) {
		token = jwt
		created = createServiceAccount(server, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, token)
	}

	waitUntilFailed := func() {
		Eventually(func(g Gomega) {
			response := read(statusPath)
			g.Expect(response.StatusCode).To(Equal(http.StatusOK))
			status, err := response.Status()
			g.Expect(err).To(Succeed())
			g.Expect(status.Status).To(Equal("failed"))
		}, expiryTimeout, expiryPolling).Should(Succeed())
	}

	BeforeEach(func() {
		storageFactory = bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelWarn))
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).To(Succeed())
		seedServiceAccountService(testStorage, fixtures.CompanyPlatformCanonicalID)
		server = serviceAccountAdminServer(testStorage)
		accountPath = helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, "platform-triage-bot")
		statusPath = accountPath + "/status"
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	Context("User Story 3 - rotating the credential and changing the ceiling", func() {
		Context("when a ready account exists", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenT1())
			})

			// Scenario 1 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should rotate the credential in place and change summary and ETag", func() {
				replacement := fixtures.ServiceAccountTokenT2()
				exp, err := helpers.ServiceAccountTokenExp(replacement)
				Expect(err).To(Succeed())

				response := update("", fixtures.ServiceAccountUpdateBody([]string{"read"}, replacement))

				Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
				Expect(response.Header.Get("ETag")).To(Equal(`"2"`))
				rotated, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect([]string{rotated.ID, rotated.CanonicalID}).To(Equal([]string{created.ID, created.CanonicalID}))
				Expect(rotated.CredentialSummary.Kid).To(Equal(new(fixtures.ServiceAccountT2KeyID)))
				Expect(rotated.CredentialSummary.Exp).To(Equal(&exp))
				Expect(rotated.CredentialSummary.Fingerprint).To(Equal(helpers.ServiceAccountFingerprint(replacement)))
				Expect(rotated.CredentialSummary.Iss).To(Equal(new(fixtures.ServiceAccountTokenIssuer)))
				Expect(rotated.CredentialSummary.Sub).To(Equal(new(fixtures.ServiceAccountTokenSubject)))
				Expect(readAccount()).To(Equal(rotated))
				expectNoJWTMaterial(token, response)
				expectNoJWTMaterial(replacement, response)
			})

			// Scenario 2 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should change the ceiling and keep credential and summary", func() {
				response := update("", fixtures.ServiceAccountUpdateBody([]string{"write", "read"}, ""))

				Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
				Expect(response.Header.Get("ETag")).To(Equal(`"2"`))
				changed, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect(changed.Scopes).To(Equal([]string{"read", "write"}))
				Expect(changed.CredentialSummary).To(Equal(created.CredentialSummary))
				Expect(changed.ID).To(Equal(created.ID))
				expectNoJWTMaterial(token, response)
			})

			// Scenario 4 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should leave the account unchanged when an update carries an invalid credential", func() {
				invalid := map[string]string{
					"unsigned":  fixtures.UnsignedServiceAccountTokens()["alg none"],
					"malformed": fixtures.MalformedServiceAccountTokens()["leading space"],
					"expired":   fixtures.ServiceAccountTokenExpiringAt(time.Now().Unix() - 60),
					"too large": fixtures.OversizedServiceAccountToken(),
				}
				for variant, jwt := range invalid {
					By(variant)
					response := update("", fixtures.ServiceAccountUpdateBody([]string{"read", "write"}, jwt))
					Expect(response.StatusCode).To(Equal(http.StatusBadRequest), string(response.Body))
					expectNoJWTMaterial(jwt, response)
				}
				unchanged := read(accountPath)
				Expect(unchanged.Header.Get("ETag")).To(Equal(`"1"`))
				Expect(unchanged.ServiceAccount()).To(Equal(created))
			})

			// Scenario 5 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should answer 200 with the same ETag when nothing changes", func() {
				response := update(`"1"`, fixtures.ServiceAccountUpdateBody([]string{"read"}, token))

				Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
				Expect(response.Header.Get("ETag")).To(Equal(`"1"`))
				Expect(response.ServiceAccount()).To(Equal(created))
				Expect(readAccount()).To(Equal(created))
				expectNoJWTMaterial(token, response)
			})

			// Scenario 6 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should answer 404 for an unknown account and for an account of another service", func() {
				seedServiceAccountService(testStorage, "other-platform")
				foreign := createServiceAccount(server, "other-platform", "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT2())
				for _, ref := range []string{foreign.ID, id.NewServiceID().String(), "unknown-bot"} {
					response := serviceAccountCall(server, http.MethodPut, helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, ref), "", fixtures.ServiceAccountUpdateBody([]string{"write"}, ""))
					Expect(response.StatusCode).To(Equal(http.StatusNotFound), ref)
				}
				Expect(listServiceAccounts(server, "other-platform")).To(Equal([]helpers.ServiceAccount{foreign}))
				Expect(readAccount()).To(Equal(created))
			})

			// Scenario 8 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should rename the account and stop addressing it by the old name", func() {
				response := update("", map[string]any{"canonical_id": "platform-triage", "scopes": []string{"read"}})

				Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
				Expect(response.Header.Get("ETag")).To(Equal(`"2"`))
				renamed, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect([]string{renamed.ID, renamed.CanonicalID}).To(Equal([]string{created.ID, "platform-triage"}))
				Expect(renamed.CredentialSummary).To(Equal(created.CredentialSummary))
				Expect(renamed.Scopes).To(Equal(created.Scopes))
				byNewName := read(helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, "platform-triage"))
				Expect(byNewName.StatusCode).To(Equal(http.StatusOK))
				Expect(byNewName.ServiceAccount()).To(Equal(renamed))
				Expect(read(accountPath).StatusCode).To(Equal(http.StatusNotFound))
			})
		})

		Context("when the account's stated expiry has passed", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenExpiringIn(2 * time.Second))
				waitUntilFailed()
			})

			// Scenario 3 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should return a failed account to ready when a valid JWT replaces the expired one", func() {
				replacement := fixtures.ServiceAccountTokenT2()

				response := update("", fixtures.ServiceAccountUpdateBody([]string{"read"}, replacement))

				Expect(response.StatusCode).To(Equal(http.StatusOK), string(response.Body))
				rotated, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect(rotated.ID).To(Equal(created.ID))
				status := readStatus()
				Expect(status.Status).To(Equal("ready"))
				Expect(status.FailureReason).To(BeNil())
				expectNoJWTMaterial(replacement, response)
			})
		})

		Context("when concurrent writers address one account", func() {
			// Scenario 7 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should answer 412 for a stale If-Match and carry out a current or absent one", Label("docker"), func() {
				for _, backend := range serviceAccountBackends {
					withServiceAccountBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
						seedServiceAccountService(st, fixtures.CompanyPlatformCanonicalID)
						createServiceAccount(s, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT1())
						path := helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, "platform-triage-bot")
						Expect(serviceAccountCall(s, http.MethodPut, path, "", fixtures.ServiceAccountUpdateBody([]string{"read", "write"}, "")).Header.Get("ETag")).To(Equal(`"2"`))
						current := serviceAccountCall(s, http.MethodGet, path, "", nil)

						staleUpdate := serviceAccountCall(s, http.MethodPut, path, `"1"`, fixtures.ServiceAccountUpdateBody([]string{"admin"}, ""))
						staleDelete := serviceAccountCall(s, http.MethodDelete, path, `"1"`, nil)
						Expect([]int{staleUpdate.StatusCode, staleDelete.StatusCode}).To(Equal([]int{http.StatusPreconditionFailed, http.StatusPreconditionFailed}))
						Expect(staleUpdate.Error()).To(Equal(helpers.ServiceAccountError{Error: "precondition failed", Message: "service account was changed; read it again and retry"}))
						unchanged := serviceAccountCall(s, http.MethodGet, path, "", nil)
						Expect(unchanged.Header.Get("ETag")).To(Equal(`"2"`))
						Expect(unchanged.Body).To(Equal(current.Body))

						matching := serviceAccountCall(s, http.MethodPut, path, `"2"`, fixtures.ServiceAccountUpdateBody([]string{"read"}, ""))
						Expect(matching.StatusCode).To(Equal(http.StatusOK), string(matching.Body))
						Expect(matching.Header.Get("ETag")).To(Equal(`"3"`))
						unconditional := serviceAccountCall(s, http.MethodPut, path, "", fixtures.ServiceAccountUpdateBody([]string{"write"}, ""))
						Expect(unconditional.StatusCode).To(Equal(http.StatusOK), string(unconditional.Body))
						Expect(unconditional.Header.Get("ETag")).To(Equal(`"4"`))
					})
				}
			})
		})

		Context("when the service holds two accounts", func() {
			// Scenario 9 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 3)
			It("should refuse a rename to a name taken on the same service", Label("docker"), func() {
				for _, backend := range serviceAccountBackends {
					withServiceAccountBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
						seedServiceAccountService(st, fixtures.CompanyPlatformCanonicalID)
						createServiceAccount(s, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT1())
						createServiceAccount(s, fixtures.CompanyPlatformCanonicalID, "platform-planner-bot", []string{"write"}, fixtures.ServiceAccountTokenT2())
						before := listServiceAccounts(s, fixtures.CompanyPlatformCanonicalID)

						response := serviceAccountCall(s, http.MethodPut, helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, "platform-triage-bot"), "",
							map[string]any{"canonical_id": "platform-planner-bot", "scopes": []string{"read"}})

						Expect(response.StatusCode).To(Equal(http.StatusConflict))
						Expect(response.Error()).To(Equal(helpers.ServiceAccountError{Error: "conflict", Message: "service account canonical_id already exists on this service"}))
						Expect(listServiceAccounts(s, fixtures.CompanyPlatformCanonicalID)).To(Equal(before))
					})
				}
			})
		})
	})

	Context("User Story 4 - seeing whether an account is usable", func() {
		Context("when a ready account states an exp in the future", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenT1())
			})

			// Scenario 1 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 4)
			It("should report ready with the stated expiry and no failure reason", func() {
				response := read(statusPath)

				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Header.Values("ETag")).To(BeEmpty())
				exp, err := helpers.ServiceAccountTokenExp(token)
				Expect(err).To(Succeed())
				Expect(response.Object()).To(Equal(map[string]any{
					"status": "ready", "failure_reason": nil, "credential_expires_at": time.Unix(exp, 0).UTC().Format(time.RFC3339),
				}))
				expectNoJWTMaterial(token, response)
			})
		})

		Context("when the account's stated exp passes", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenExpiringIn(2 * time.Second))
			})

			// Scenario 2 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 4)
			It("should report failed with credential_expired in status once exp has passed", func() {
				Eventually(func(g Gomega) {
					status, err := read(statusPath).Status()
					g.Expect(err).To(Succeed())
					g.Expect(status.Status).To(Equal("failed"))
					g.Expect(status.FailureReason).To(Equal(new("credential_expired")))
				}, expiryTimeout, expiryPolling).Should(Succeed())
				accounts, err := read(helpers.ServiceAccountsPath(fixtures.CompanyPlatformCanonicalID)).ServiceAccounts()
				Expect(err).To(Succeed())
				Expect(accounts).To(HaveLen(1), "an expired account is still listed")
				Expect(accounts[0].CredentialSummary).To(Equal(created.CredentialSummary))
			})
		})

		Context("when the account's JWT states no exp", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenWithoutExp())
			})

			// Scenario 3 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 4)
			It("should keep an account without exp ready", func() {
				response := read(statusPath)

				Expect(response.StatusCode).To(Equal(http.StatusOK))
				Expect(response.Object()).To(Equal(map[string]any{"status": "ready", "failure_reason": nil, "credential_expires_at": nil}))
				expectNoJWTMaterial(token, response)
			})
		})

		Context("when the account identifier matches no account of the service", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenT1())
			})

			// Scenario 4 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 4)
			It("should answer 404 for the status of an unknown account", func() {
				seedServiceAccountService(testStorage, "other-platform")
				foreign := createServiceAccount(server, "other-platform", "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT2())
				for _, ref := range []string{foreign.ID, id.NewServiceID().String(), "unknown-bot"} {
					response := read(helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, ref) + "/status")
					Expect(response.StatusCode).To(Equal(http.StatusNotFound), ref)
				}
			})
		})
	})

	Context("User Story 5 - removing a service account", func() {
		Context("when an account that no agent grant binds exists", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenT1())
			})

			// Scenario 1 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 5)
			It("should delete the account and remove it from the list", func() {
				response := serviceAccountCall(server, http.MethodDelete, accountPath, "", nil)

				Expect(response.StatusCode).To(Equal(http.StatusNoContent))
				Expect(response.Body).To(BeEmpty())
				Expect(listServiceAccounts(server, fixtures.CompanyPlatformCanonicalID)).To(BeEmpty())
				Expect(read(helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, created.ID)).StatusCode).To(Equal(http.StatusNotFound))
				expectNoJWTMaterial(token, response)
			})

			// Scenario 2 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 5)
			It("should answer 404 when deleting an unknown account", func() {
				Expect(serviceAccountCall(server, http.MethodDelete, accountPath, "", nil).StatusCode).To(Equal(http.StatusNoContent))
				for _, ref := range []string{created.ID, "platform-triage-bot", "unknown-bot"} {
					response := serviceAccountCall(server, http.MethodDelete, helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, ref), "", nil)
					Expect(response.StatusCode).To(Equal(http.StatusNotFound), ref)
				}
			})
		})

		Context("when the account's current ETag is \"2\"", func() {
			BeforeEach(func() {
				registerAccount(fixtures.ServiceAccountTokenT1())
				Expect(update("", fixtures.ServiceAccountUpdateBody([]string{"read", "write"}, "")).Header.Get("ETag")).To(Equal(`"2"`))
			})

			// Scenario 3 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 5)
			It("should answer 412 and keep the account when a delete carries a stale If-Match", func() {
				response := serviceAccountCall(server, http.MethodDelete, accountPath, `"1"`, nil)

				Expect(response.StatusCode).To(Equal(http.StatusPreconditionFailed))
				Expect(response.Error()).To(Equal(helpers.ServiceAccountError{Error: "precondition failed", Message: "service account was changed; read it again and retry"}))
				Expect(readAccount().ID).To(Equal(created.ID))
				Expect(read(accountPath).Header.Get("ETag")).To(Equal(`"2"`))
			})
		})

		Context("when a service holds a service account", func() {
			// Scenario 4 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 5)
			It("should refuse deleting a service that holds an account and allow it after the last one is removed", Label("docker"), func() {
				for _, backend := range serviceAccountBackends {
					withServiceAccountBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
						service := seedServiceAccountService(st, fixtures.CompanyPlatformCanonicalID)
						account := createServiceAccount(s, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT1())
						servicePath := "/api/services/" + service.ID.String()

						refused := serviceAccountCall(s, http.MethodDelete, servicePath, "", nil)

						Expect(refused.StatusCode).To(Equal(http.StatusConflict), string(refused.Body))
						Expect(refused.Error()).To(Equal(helpers.ServiceAccountError{Error: "conflict", Message: "cannot delete service: 1 service account(s) are attached to it"}))
						Expect(serviceAccountCall(s, http.MethodGet, servicePath, "", nil).StatusCode).To(Equal(http.StatusOK))
						Expect(listServiceAccounts(s, fixtures.CompanyPlatformCanonicalID)).To(Equal([]helpers.ServiceAccount{account}))

						Expect(serviceAccountCall(s, http.MethodDelete, helpers.ServiceAccountPath(service.ID.String(), account.ID), "", nil).StatusCode).To(Equal(http.StatusNoContent))
						Expect(serviceAccountCall(s, http.MethodDelete, servicePath, "", nil).StatusCode).To(Equal(http.StatusNoContent))
						Expect(serviceAccountCall(s, http.MethodGet, servicePath, "", nil).StatusCode).To(Equal(http.StatusNotFound))
					})
				}
			})
		})
	})
})
