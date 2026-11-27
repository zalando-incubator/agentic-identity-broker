package e2e_test

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/uuid"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("Service accounts", func() {
	var (
		server         *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		service        *model.ThirdpartyOAuth2ProviderEntity
		servicePath    string
	)

	BeforeEach(func() {
		storageFactory = bootstrap.NewStorageFactory(bootstrap.TestLogger(slog.LevelWarn))
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).To(Succeed())
		service = seedServiceAccountService(testStorage, fixtures.CompanyPlatformCanonicalID)
		server = serviceAccountAdminServer(testStorage)
		servicePath = helpers.ServiceAccountsPath(fixtures.CompanyPlatformCanonicalID)
	})

	AfterEach(func() {
		if server != nil {
			server.Close()
		}
		if testStorage != nil {
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
		}
	})

	Context("User Story 1 - registering a ready-signed JWT", func() {
		Context("when the service has no service accounts", func() {
			// Scenario 1 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should create a ready account with a summary and an ETag and without the JWT", func() {
				token := fixtures.ServiceAccountTokenT1()
				exp, err := helpers.ServiceAccountTokenExp(token)
				Expect(err).To(Succeed())

				response := serviceAccountCall(server, http.MethodPost, servicePath, "", fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"read"}, token))

				Expect(response.StatusCode).To(Equal(http.StatusCreated), string(response.Body))
				Expect(response.Header.Get("ETag")).To(Equal(`"1"`))
				account, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect(uuid.Parse(account.ID)).Error().To(Succeed())
				Expect(response.Header.Get("Location")).To(Equal(helpers.ServiceAccountPath(service.ID.String(), account.ID)))
				Expect([]string{account.ServiceID, account.CanonicalID, account.Kind, account.Narrowing, account.Status}).
					To(Equal([]string{service.ID.String(), "platform-triage-bot", "jwt_bearer", "none", "ready"}))
				Expect(account.Scopes).To(Equal([]string{"read"}))
				Expect(account.FailureReason).To(BeNil())
				Expect(response.Object()).To(HaveKeyWithValue("failure_reason", BeNil()), "failure_reason is always present, null while ready")
				Expect(account.CredentialSummary).To(Equal(helpers.ServiceAccountCredentialSummary{
					Format: "signed_jwt", Use: "access_token", Iss: new(fixtures.ServiceAccountTokenIssuer), Sub: new(fixtures.ServiceAccountTokenSubject),
					Kid: new(fixtures.ServiceAccountT1KeyID), Exp: &exp, Fingerprint: helpers.ServiceAccountFingerprint(token),
				}))
				Expect(time.Parse(time.RFC3339, account.CreatedAt)).Error().To(Succeed())
				Expect(account.UpdatedAt).To(Equal(account.CreatedAt))
				expectNoJWTMaterial(token, response)
			})

			// Scenario 3 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should default use to access_token when the credential omits it", func() {
				token := fixtures.ServiceAccountTokenT1()
				body := fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"read"}, token)
				body["credential"] = map[string]any{"format": "signed_jwt", "jwt": token}

				response := serviceAccountCall(server, http.MethodPost, servicePath, "", body)

				Expect(response.StatusCode).To(Equal(http.StatusCreated), string(response.Body))
				account, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect(account.CredentialSummary.Use).To(Equal("access_token"))
				expectNoJWTMaterial(token, response)
			})

			// Scenario 4 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should accept a JWT without exp and report no expiry", func() {
				token := fixtures.ServiceAccountTokenWithoutExp()

				response := serviceAccountCall(server, http.MethodPost, servicePath, "", fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"read"}, token))

				Expect(response.StatusCode).To(Equal(http.StatusCreated), string(response.Body))
				account, err := response.ServiceAccount()
				Expect(err).To(Succeed())
				Expect(account.Status).To(Equal("ready"))
				Expect(account.CredentialSummary.Exp).To(BeNil())
				object, err := response.Object()
				Expect(err).To(Succeed())
				Expect(object["credential_summary"]).NotTo(HaveKey("exp"))
				expectNoJWTMaterial(token, response)
			})
		})

		Context("when the service has two service accounts", func() {
			var plannerToken, triageToken string

			BeforeEach(func() {
				plannerToken, triageToken = fixtures.ServiceAccountTokenT2(), fixtures.ServiceAccountTokenT1()
				createServiceAccount(server, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, triageToken)
				createServiceAccount(server, fixtures.CompanyPlatformCanonicalID, "platform-planner-bot", []string{"read", "write"}, plannerToken)
			})

			// Scenario 2 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should list both accounts with summaries and no credential material", func() {
				response := serviceAccountCall(server, http.MethodGet, servicePath, "", nil)

				Expect(response.StatusCode).To(Equal(http.StatusOK))
				accounts, err := response.ServiceAccounts()
				Expect(err).To(Succeed())
				Expect(accounts).To(HaveLen(2))
				Expect([]string{accounts[0].CanonicalID, accounts[1].CanonicalID}).To(Equal([]string{"platform-planner-bot", "platform-triage-bot"}))
				Expect(accounts[0].CredentialSummary.Fingerprint).To(Equal(helpers.ServiceAccountFingerprint(plannerToken)))
				Expect(accounts[1].CredentialSummary.Fingerprint).To(Equal(helpers.ServiceAccountFingerprint(triageToken)))
				for _, account := range accounts {
					Expect(account.Status).To(Equal("ready"))
					Expect(account.Narrowing).To(Equal("none"))
					Expect(account.CredentialSummary.Kid).NotTo(BeNil())
				}
				expectNoJWTMaterial(plannerToken, response)
				expectNoJWTMaterial(triageToken, response)
			})
		})

		Context("when a service already holds the canonical_id", func() {
			// Scenario 5 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should refuse a taken canonical_id on the same service and accept it on another", Label("docker"), func() {
				for _, backend := range serviceAccountBackends {
					withServiceAccountBackend(backend, func(s *bootstrap.TestServer, st *storageadapter.Adapter) {
						seedServiceAccountService(st, fixtures.CompanyPlatformCanonicalID)
						other := seedServiceAccountService(st, "other-platform")
						existing := createServiceAccount(s, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT1())
						token := fixtures.ServiceAccountTokenT2()

						conflict := serviceAccountCall(s, http.MethodPost, helpers.ServiceAccountsPath(fixtures.CompanyPlatformCanonicalID), "", fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"write"}, token))
						Expect(conflict.StatusCode).To(Equal(http.StatusConflict))
						Expect(conflict.Error()).To(Equal(helpers.ServiceAccountError{Error: "conflict", Message: "service account canonical_id already exists on this service"}))
						Expect(listServiceAccounts(s, fixtures.CompanyPlatformCanonicalID)).To(Equal([]helpers.ServiceAccount{existing}))

						accepted := serviceAccountCall(s, http.MethodPost, helpers.ServiceAccountsPath(other.ID.String()), "", fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"write"}, token))
						Expect(accepted.StatusCode).To(Equal(http.StatusCreated), string(accepted.Body))
						Expect(listServiceAccounts(s, "other-platform")).To(HaveLen(1))
						expectNoJWTMaterial(token, conflict, accepted)
					})
				}
			})
		})

		Context("when the service identifier matches no service", func() {
			// Scenario 6 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should answer 404 for create and list under an unknown service", func() {
				token := fixtures.ServiceAccountTokenT1()
				for _, unknown := range []string{id.NewServiceID().String(), "unknown-platform"} {
					created := serviceAccountCall(server, http.MethodPost, helpers.ServiceAccountsPath(unknown), "", fixtures.ServiceAccountCreateBody("platform-triage-bot", []string{"read"}, token))
					listed := serviceAccountCall(server, http.MethodGet, helpers.ServiceAccountsPath(unknown), "", nil)
					Expect(created.StatusCode).To(Equal(http.StatusNotFound), unknown)
					Expect(listed.StatusCode).To(Equal(http.StatusNotFound), unknown)
					expectNoJWTMaterial(token, created, listed)
				}
				Expect(listServiceAccounts(server, fixtures.CompanyPlatformCanonicalID)).To(BeEmpty())
			})
		})

		Context("when an account exists", func() {
			var (
				token   string
				created helpers.ServiceAccount
			)

			BeforeEach(func() {
				token = fixtures.ServiceAccountTokenT1()
				created = createServiceAccount(server, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, token)
			})

			// Scenario 7 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 1)
			It("should read one account by ID and by canonical_id with its ETag", func() {
				listed := listServiceAccounts(server, fixtures.CompanyPlatformCanonicalID)
				for _, ref := range []string{created.ID, "platform-triage-bot"} {
					response := serviceAccountCall(server, http.MethodGet, helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, ref), "", nil)
					Expect(response.StatusCode).To(Equal(http.StatusOK), ref)
					Expect(response.Header.Get("ETag")).To(Equal(`"1"`))
					Expect(response.ServiceAccount()).To(Equal(listed[0]))
					expectNoJWTMaterial(token, response)
				}
			})
		})
	})

	Context("User Story 2 - refusing credentials that cannot work", func() {
		expectRefusedCreate := func(token, message string, variant string) {
			response := serviceAccountCall(server, http.MethodPost, servicePath, "", fixtures.ServiceAccountCreateBody("platform-planner-bot", []string{"read", "write"}, token))
			By(variant)
			expectValidationFailure(response, message)
			expectNoJWTMaterial(token, response)
		}

		Context("when the service has no service accounts", func() {
			AfterEach(func() {
				Expect(listServiceAccounts(server, fixtures.CompanyPlatformCanonicalID)).To(BeEmpty())
			})

			// Scenario 1 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse a JWT that is not a compact JWS", func() {
				for variant, token := range fixtures.MalformedServiceAccountTokens() {
					expectRefusedCreate(token, "credential.jwt must be a compact JWS with valid JWT claims", variant)
				}
			})

			// Scenario 2 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse a JWT with alg none in any letter case or without alg", func() {
				messages := map[string]string{
					"alg none":    "credential.jwt must be a signed compact JWS",
					"missing alg": "credential.jwt must be a signed compact JWS",
					"alg NONE":    "credential.jwt must be a compact JWS with valid JWT claims",
					"alg None":    "credential.jwt must be a compact JWS with valid JWT claims",
				}
				for variant, token := range fixtures.UnsignedServiceAccountTokens() {
					expectRefusedCreate(token, messages[variant], variant)
				}
			})

			// Scenario 3 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse a JWT whose exp is not in the future", func() {
				now := time.Now().Unix()
				for variant, exp := range map[string]int64{"exp at the request time": now, "exp one second ago": now - 1, "exp one hour ago": now - 3600} {
					expectRefusedCreate(fixtures.ServiceAccountTokenExpiringAt(exp), "credential.jwt is expired", variant)
				}
			})

			// Scenario 4 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse a JWT longer than 16384 bytes", func() {
				token := fixtures.OversizedServiceAccountToken()
				Expect(token).To(HaveLen(fixtures.ServiceAccountJWTMaxBytes + 1))
				expectRefusedCreate(token, "credential.jwt must be at most 16384 bytes", "16385 bytes")
			})

			// Scenario 5 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse use assertion and unknown use, format and kind", func() {
				token := fixtures.ServiceAccountTokenT1()
				variants := map[string]struct {
					mutate  func(map[string]any)
					message string
				}{
					"use assertion":  {func(b map[string]any) { b["credential"].(map[string]any)["use"] = "assertion" }, "credential.use must be access_token"},
					"unknown use":    {func(b map[string]any) { b["credential"].(map[string]any)["use"] = "refresh_token" }, "credential.use must be access_token"},
					"unknown format": {func(b map[string]any) { b["credential"].(map[string]any)["format"] = "private_key" }, "credential.format must be signed_jwt"},
					"unknown kind":   {func(b map[string]any) { b["kind"] = "client_credentials" }, "kind must be jwt_bearer"},
				}
				for variant, tc := range variants {
					body := fixtures.ServiceAccountCreateBody("platform-planner-bot", []string{"read"}, token)
					tc.mutate(body)
					response := serviceAccountCall(server, http.MethodPost, servicePath, "", body)
					By(variant)
					expectValidationFailure(response, tc.message)
					expectNoJWTMaterial(token, response)
				}
			})

			// Scenario 6 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should refuse empty or missing scopes, a missing credential and an invalid canonical_id", func() {
				token := fixtures.ServiceAccountTokenT1()
				const scopesMessage = "scopes must be a non-empty list of unique, non-empty values"
				variants := map[string]struct {
					mutate  func(map[string]any)
					message string
				}{
					"empty scopes":         {func(b map[string]any) { b["scopes"] = []string{} }, scopesMessage},
					"missing scopes":       {func(b map[string]any) { delete(b, "scopes") }, scopesMessage},
					"missing credential":   {func(b map[string]any) { delete(b, "credential") }, "credential is required"},
					"invalid canonical_id": {func(b map[string]any) { b["canonical_id"] = "platform triage bot" }, "canonical_id must be 1-128 characters using only letters, digits, '.', '_', or '-'"},
				}
				for variant, tc := range variants {
					body := fixtures.ServiceAccountCreateBody("platform-planner-bot", []string{"read"}, token)
					tc.mutate(body)
					response := serviceAccountCall(server, http.MethodPost, servicePath, "", body)
					By(variant)
					expectValidationFailure(response, tc.message)
					expectNoJWTMaterial(token, response)
				}
			})
		})

		Context("when an account exists", func() {
			var (
				accountPath string
				before      helpers.ServiceAccount
			)

			BeforeEach(func() {
				before = createServiceAccount(server, fixtures.CompanyPlatformCanonicalID, "platform-triage-bot", []string{"read"}, fixtures.ServiceAccountTokenT1())
				accountPath = helpers.ServiceAccountPath(fixtures.CompanyPlatformCanonicalID, "platform-triage-bot")
			})

			// Scenario 7 from specs/20261125-152113-service-accounts-stored-jwt/spec.md (User Story 2)
			It("should name the failed rule without echoing any part of the JWT", func() {
				refused := map[string]string{
					fixtures.UnsignedServiceAccountTokens()["alg none"]:            "credential.jwt must be a signed compact JWS",
					fixtures.MalformedServiceAccountTokens()["five segments"]:      "credential.jwt must be a compact JWS with valid JWT claims",
					fixtures.ServiceAccountTokenExpiringAt(time.Now().Unix() - 60): "credential.jwt is expired",
					fixtures.OversizedServiceAccountToken():                        "credential.jwt must be at most 16384 bytes",
				}
				for token, message := range refused {
					created := serviceAccountCall(server, http.MethodPost, servicePath, "", fixtures.ServiceAccountCreateBody("platform-planner-bot", []string{"read"}, token))
					updated := serviceAccountCall(server, http.MethodPut, accountPath, "", fixtures.ServiceAccountUpdateBody([]string{"read"}, token))
					for _, response := range []helpers.CapturedResponse{created, updated} {
						expectValidationFailure(response, message)
						Expect(strings.HasPrefix(message, "credential.jwt ")).To(BeTrue())
						expectNoJWTMaterial(token, response)
					}
				}
				Expect(listServiceAccounts(server, fixtures.CompanyPlatformCanonicalID)).To(Equal([]helpers.ServiceAccount{before}))
			})
		})
	})
})
