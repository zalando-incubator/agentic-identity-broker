package e2e_test

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

type credentialRotationServiceSnapshot struct {
	Record     model.ThirdpartyOAuth2ProviderEntity
	Ciphertext []byte
}

var _ = Describe("Filesystem OAuth2 credential rotation", Label("oauth2-secret-file-overrides"), func() {
	var journey *helpers.OAuth2CredentialJourney

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
	})

	snapshotService := func(serviceID id.ServiceID) credentialRotationServiceSnapshot {
		record, err := journey.Storage.Services().Get(context.Background(), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(record).NotTo(BeNil())
		Expect(record.CredentialSource).To(Equal(model.CredentialSourceFilesystem))
		Expect(record.CredentialSourceTransitioned).To(BeFalse())
		Expect(record.ClientID).To(BeZero())
		Expect(record.Secret.IsAbsent()).To(BeTrue())
		ciphertext, err := record.Secret.GetCiphertext()
		Expect(err).To(HaveOccurred())
		Expect(ciphertext).To(BeNil())
		return credentialRotationServiceSnapshot{Record: *record, Ciphertext: ciphertext}
	}

	expectServiceUnchanged := func(serviceID id.ServiceID, before credentialRotationServiceSnapshot) {
		after := snapshotService(serviceID)
		Expect(after.Record.Version).To(Equal(before.Record.Version))
		Expect(after.Record.CreatedAt).To(Equal(before.Record.CreatedAt))
		Expect(after.Record.UpdatedAt).To(Equal(before.Record.UpdatedAt))
		Expect(after.Ciphertext).To(Equal(before.Ciphertext))
		Expect(after.Record).To(Equal(before.Record))
	}

	readSession := func(serviceID id.ServiceID) domainstorage.UserSession {
		session, err := journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.NewPrincipal(journey.Principal), serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session).NotTo(BeNil())
		snapshot := *session
		snapshot.EncryptedAccessToken = slices.Clone(session.EncryptedAccessToken)
		snapshot.EncryptedRefreshToken = slices.Clone(session.EncryptedRefreshToken)
		snapshot.Scope = slices.Clone(session.Scope)
		if session.UpstreamClientID != nil {
			clientID := *session.UpstreamClientID
			snapshot.UpstreamClientID = &clientID
		}
		return snapshot
	}

	expectTokenRequest := func(index int, grantType, clientID, clientSecret string) url.Values {
		requests := journey.Upstream.GetTokenRequests()
		Expect(requests).To(HaveLen(index + 1))
		actualClientID, actualSecret, err := helpers.CredentialTokenRequest(requests[index])
		Expect(err).NotTo(HaveOccurred())
		Expect(actualClientID).To(Equal(clientID))
		Expect(actualSecret).To(Equal(clientSecret))
		form, err := url.ParseQuery(requests[index].Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal(grantType))
		return form
	}

	pauseAuthorization := func(serviceID id.ServiceID, clientID string) *url.URL {
		tokenRequestsBefore := len(journey.Upstream.GetTokenRequests())
		authorizationsBefore := journey.Upstream.GetAuthorizationRequestCount()
		response, err := journey.Authorize(serviceID)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(response.Body.Close)
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		authorizationURL, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationURL.Query().Get("client_id")).To(Equal(clientID))
		Expect(authorizationURL.Query().Get("client_secret")).To(BeEmpty())
		callback, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(callback.Query().Get("code")).NotTo(BeEmpty())
		Expect(callback.Query().Get("error")).To(BeEmpty())
		providerURL, err := url.Parse(journey.Upstream.GetLastAuthorizeURL())
		Expect(err).NotTo(HaveOccurred())
		Expect(providerURL.Query().Get("client_id")).To(Equal(clientID))
		Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationsBefore + 1))
		Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokenRequestsBefore))
		return callback
	}

	completeCallback := func(serviceID id.ServiceID, callback *url.URL, clientID, clientSecret string) domainstorage.UserSession {
		requestIndex := len(journey.Upstream.GetTokenRequests())
		response, err := journey.EndUser.AuthenticatedGET(callback.RequestURI(), journey.Principal)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(response.Body.Close)
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/done"))
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("service_id")).To(Equal(serviceID.String()))
		expectTokenRequest(requestIndex, "authorization_code", clientID, clientSecret)
		session := readSession(serviceID)
		Expect(session.EncryptedAccessToken).NotTo(BeEmpty())
		Expect(session.EncryptedRefreshToken).NotTo(BeEmpty())
		Expect(session.UpstreamClientID).NotTo(BeNil())
		Expect(*session.UpstreamClientID).To(Equal(id.ClientID(clientID)))
		return session
	}

	connect := func(serviceID id.ServiceID, clientID, clientSecret string) domainstorage.UserSession {
		return completeCallback(serviceID, pauseAuthorization(serviceID, clientID), clientID, clientSecret)
	}

	refresh := func(serviceID id.ServiceID, clientID, clientSecret string) domainstorage.UserSession {
		requestIndex := len(journey.Upstream.GetTokenRequests())
		response, err := journey.Refresh(serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(body).To(HaveKey("data"))
		Expect(body).NotTo(HaveKey("error"))
		expectTokenRequest(requestIndex, "refresh_token", clientID, clientSecret)
		session := readSession(serviceID)
		Expect(session.UpstreamClientID).NotTo(BeNil())
		Expect(*session.UpstreamClientID).To(Equal(id.ClientID(clientID)))
		return session
	}

	Context("with a projected-volume binding initially publishing source A", func() {
		var (
			binding       map[string]string
			serviceID     id.ServiceID
			serviceBefore credentialRotationServiceSnapshot
		)

		BeforeEach(func() {
			var err error
			binding, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(err).NotTo(HaveOccurred())
			Expect(journey.Start(map[string]map[string]string{"rotating-service": binding})).To(Succeed())
			serviceID, _, err = journey.Register(fixtures.CredentialServiceRequest("rotating-service", journey.Upstream.URL(), "filesystem"))
			Expect(err).NotTo(HaveOccurred())
			serviceBefore = snapshotService(serviceID)
		})

		Context("when source A has already established a refreshable session", func() {
			var (
				sourceASession domainstorage.UserSession
				principal      string
			)

			BeforeEach(func() {
				principal = journey.Principal
				sourceASession = connect(serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			})

			// US2-S1 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US2-S1 uses a same-identity replacement secret for a new exchange and the existing session without service writes or restart", func() {
				By("publishing source B's secret with source A's unchanged client identity")
				Expect(helpers.PublishCredentialPair(binding, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())

				By("connecting another user through the same running broker")
				journey.Principal = fixtures.AnotherPrincipal().String()
				journey.Upstream.WithAccessToken("rotated-new-connection-access").WithRefreshToken("rotated-new-connection-refresh")
				connect(serviceID, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)

				By("renewing the original source A session rather than the newly connected user's session")
				journey.Principal = principal
				Expect(readSession(serviceID)).To(Equal(sourceASession))
				journey.Upstream.WithAccessToken("rotated-existing-session-access").WithRefreshToken("rotated-existing-session-refresh")
				afterRefresh := refresh(serviceID, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)
				Expect(afterRefresh.ID).To(Equal(sourceASession.ID))
				Expect(afterRefresh.InitiatedAt).To(Equal(sourceASession.InitiatedAt))
				Expect(afterRefresh.EncryptedAccessToken).NotTo(Equal(sourceASession.EncryptedAccessToken))
				Expect(afterRefresh.EncryptedRefreshToken).NotTo(Equal(sourceASession.EncryptedRefreshToken))
				form, err := url.ParseQuery(journey.Upstream.GetTokenRequests()[2].Body)
				Expect(err).NotTo(HaveOccurred())
				Expect(form.Get("refresh_token")).To(Equal("mock-refresh-token"))
				expectServiceUnchanged(serviceID, serviceBefore)
			})
		})

		Context("when a complete projected-volume generation replaces source A", func() {
			var sourceAGeneration string

			BeforeEach(func() {
				var err error
				sourceAGeneration, err = os.Readlink(filepath.Join(filepath.Dir(binding["client_id_file"]), "..data"))
				Expect(err).NotTo(HaveOccurred())
				for path, target := range map[string]string{binding["client_id_file"]: "..data/identity", binding["client_secret_file"]: "..data/credential"} {
					actualTarget, err := os.Readlink(path)
					Expect(err).NotTo(HaveOccurred())
					Expect(actualTarget).To(Equal(target))
				}
				connect(serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			})

			// US2-S3 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US2-S3 follows a complete symlink generation swap and sends only coherent provider credential pairs", func() {
				By("atomically publishing source B through fresh immutable generation targets")
				Expect(helpers.PublishCredentialPair(binding, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				sourceBGeneration, err := os.Readlink(filepath.Join(filepath.Dir(binding["client_id_file"]), "..data"))
				Expect(err).NotTo(HaveOccurred())
				Expect(sourceBGeneration).NotTo(Equal(sourceAGeneration))

				By("connecting and refreshing with source B's complete current pair")
				connect(serviceID, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				refresh(serviceID, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				var providerPairs [][2]string
				for _, request := range journey.Upstream.GetTokenRequests() {
					clientID, secret, err := helpers.CredentialTokenRequest(request)
					Expect(err).NotTo(HaveOccurred())
					providerPairs = append(providerPairs, [2]string{clientID, secret})
				}
				Expect(providerPairs).To(Equal([][2]string{
					{fixtures.CredentialClientID, fixtures.CredentialClientSecret},
					{fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret},
					{fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret},
				}))
				expectServiceUnchanged(serviceID, serviceBefore)
			})
		})

		Context("when source A has both an established session and an unexchanged authorization code", func() {
			var (
				sourceASession  domainstorage.UserSession
				pausedACallback *url.URL
			)

			BeforeEach(func() {
				sourceASession = connect(serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				pausedACallback = pauseAuthorization(serviceID, fixtures.CredentialClientID)
			})

			// US2-S5 from specs/051-oauth2-secret-file-overrides/spec.md
			It("US2-S5 refuses source A's code and session before provider authentication after identity B is published, then completes a new B connection", func() {
				By("publishing source B and starting a fresh authorization with B")
				Expect(helpers.PublishCredentialPair(binding, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				pausedBCallback := pauseAuthorization(serviceID, fixtures.AlternateCredentialClientID)
				tokenRequestsBefore := len(journey.Upstream.GetTokenRequests())
				authorizationsBefore := journey.Upstream.GetAuthorizationRequestCount()

				By("refusing the paused source A code without sending it under B's identity")
				response, err := journey.EndUser.AuthenticatedGET(pausedACallback.RequestURI(), journey.Principal)
				Expect(err).NotTo(HaveOccurred())
				DeferCleanup(response.Body.Close)
				Expect(response.StatusCode).To(Equal(http.StatusFound))
				location, err := url.Parse(response.Header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
				Expect(location.Path).To(Equal("/sessions"))
				Expect(location.Query().Get("error")).To(Equal("callback_failed"))
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokenRequestsBefore))
				Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationsBefore))
				Expect(readSession(serviceID)).To(Equal(sourceASession))

				By("refusing to renew source A's established session as B")
				response, err = journey.Refresh(serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
				body, err := helpers.CredentialResponseObject(response)
				Expect(err).NotTo(HaveOccurred())
				Expect(body["error"]).To(Equal("internal_error"))
				Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokenRequestsBefore))
				Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizationsBefore))
				Expect(readSession(serviceID)).To(Equal(sourceASession))

				By("establishing a new B session only through B's new connection")
				journey.Upstream.WithAccessToken("identity-B-access").WithRefreshToken("identity-B-refresh")
				sourceBSession := completeCallback(serviceID, pausedBCallback, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				Expect(sourceBSession.EncryptedAccessToken).NotTo(Equal(sourceASession.EncryptedAccessToken))
				Expect(sourceBSession.EncryptedRefreshToken).NotTo(Equal(sourceASession.EncryptedRefreshToken))
				refresh(serviceID, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
				expectServiceUnchanged(serviceID, serviceBefore)
			})
		})
	})

	Context("with two independently configured filesystem services", func() {
		var (
			bindingA map[string]string
			serviceA id.ServiceID
			serviceB id.ServiceID
			recordA  credentialRotationServiceSnapshot
			recordB  credentialRotationServiceSnapshot
		)

		BeforeEach(func() {
			var err error
			bindingA, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(err).NotTo(HaveOccurred())
			bindingB, err := journey.WritePair(fixtures.AlternateCredentialClientID, "independent-service-secret")
			Expect(err).NotTo(HaveOccurred())
			Expect(journey.Start(map[string]map[string]string{"independent-first": bindingA, "independent-second": bindingB})).To(Succeed())
			serviceA, _, err = journey.Register(fixtures.CredentialServiceRequest("independent-first", journey.Upstream.URL(), "filesystem"))
			Expect(err).NotTo(HaveOccurred())
			requestB := fixtures.CredentialServiceRequest("independent-second", journey.Upstream.URL(), "filesystem")
			requestB["protected_resources"] = []string{journey.Upstream.URL() + "/independent-resource"}
			serviceB, _, err = journey.Register(requestB)
			Expect(err).NotTo(HaveOccurred())
			recordA = snapshotService(serviceA)
			recordB = snapshotService(serviceB)
			connect(serviceA, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			connect(serviceB, fixtures.AlternateCredentialClientID, "independent-service-secret")
		})

		// US2-S2 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US2-S2 changes only the selected service's provider credentials when one independent binding rotates", func() {
			By("replacing only the first service's source A with its complete source B pair")
			Expect(helpers.PublishCredentialPair(bindingA, "rotated-first-client", "rotated-first-secret")).To(Succeed())
			By("starting new connections for both services without registration changes")
			connect(serviceA, "rotated-first-client", "rotated-first-secret")
			connect(serviceB, fixtures.AlternateCredentialClientID, "independent-service-secret")
			Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(4))
			expectServiceUnchanged(serviceA, recordA)
			expectServiceUnchanged(serviceB, recordB)
		})
	})

	Context("with surrounding whitespace in both projected credential files", func() {
		const (
			clientID     = "synthetic client:\tA+%/identity"
			clientSecret = "synthetic secret:\tA+%/credential"
		)
		var (
			serviceID     id.ServiceID
			serviceBefore credentialRotationServiceSnapshot
		)

		BeforeEach(func() {
			binding, err := journey.WritePair(" \t"+clientID+"\t \r\n", "\n \t"+clientSecret+"\r\n\t ")
			Expect(err).NotTo(HaveOccurred())
			Expect(journey.Start(map[string]map[string]string{"normalized-service": binding})).To(Succeed())
			serviceID, _, err = journey.Register(fixtures.CredentialServiceRequest("normalized-service", journey.Upstream.URL(), "filesystem"))
			Expect(err).NotTo(HaveOccurred())
			serviceBefore = snapshotService(serviceID)
		})

		// US2-S4 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US2-S4 trims both files' surrounding spaces, tabs and newlines while preserving internal credential characters through connect and refresh", func() {
			By("authorizing and exchanging with the normalized identity and secret")
			connected := connect(serviceID, clientID, clientSecret)
			By("preserving the same normalized values in the existing session's refresh authentication")
			renewed := refresh(serviceID, clientID, clientSecret)
			Expect(renewed.ID).To(Equal(connected.ID))
			Expect(renewed.InitiatedAt).To(Equal(connected.InitiatedAt))
			expectServiceUnchanged(serviceID, serviceBefore)
		})
	})
})
