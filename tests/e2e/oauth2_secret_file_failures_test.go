package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// The two subjects have separate files, provider, storage, ports, and log capture.
// Each numbered scenario exercises the identity and secret independently.
type credentialFailureSubject struct {
	journey     *helpers.OAuth2CredentialJourney
	binding     map[string]string
	fileKey     string
	serviceID   id.ServiceID
	redirectURI string
	request     func(string, string, io.Reader) (*http.Response, error)
	logs        *bootstrap.BufferedLogCapture
}

func startCredentialFailureJourney(subject *credentialFailureSubject) {
	GinkgoHelper()
	Expect(subject.journey.Start(map[string]map[string]string{"failure-service": subject.binding})).To(Succeed())
	serviceID, _, err := subject.journey.Register(fixtures.CredentialServiceRequest("failure-service", subject.journey.Upstream.URL(), "filesystem"))
	Expect(err).NotTo(HaveOccurred())
	subject.serviceID = serviceID
	subject.redirectURI = subject.journey.Config.Server.EndUser.PublicURL + "/done"
	subject.logs = subject.journey.Logs
	subject.request = func(method, path string, body io.Reader) (*http.Response, error) {
		return subject.journey.EndUser.DirectRequest(method, path, subject.journey.Principal, map[string]string{"Content-Type": "application/x-www-form-urlencoded"}, body)
	}
}

func startCredentialFailureProcess(subject *credentialFailureSubject) *bootstrap.CredentialProcess {
	GinkgoHelper()
	process, err := bootstrap.StartCredentialProcess(bootstrap.CredentialProcessOptions{
		Directory: subject.journey.Directory,
		Document:  fixtures.CredentialConfigurationDocument(subject.journey.Upstream.URL(), map[string]map[string]string{"failure-service": subject.binding}, 8000, 14000),
		Principal: subject.journey.Principal,
		NonRoot:   true,
	})
	if process != nil {
		DeferCleanup(process.Close)
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(process.UID).NotTo(BeZero())
	registerCredentialFailureServer(subject, process.CredentialServer)
	return process
}

func startCredentialFailureServer(subject *credentialFailureSubject) *bootstrap.CredentialServer {
	GinkgoHelper()
	server, err := bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{
		Directory: subject.journey.Directory,
		Document:  fixtures.CredentialConfigurationDocument(subject.journey.Upstream.URL(), map[string]map[string]string{"failure-service": subject.binding}, 8000, 14000),
		Principal: subject.journey.Principal,
	})
	if server != nil {
		DeferCleanup(server.Close)
	}
	Expect(err).NotTo(HaveOccurred())
	registerCredentialFailureServer(subject, server)
	return server
}

func registerCredentialFailureServer(subject *credentialFailureSubject, server *bootstrap.CredentialServer) {
	GinkgoHelper()
	response, err := server.AdminJSON(http.MethodPost, "/api/services", fixtures.CredentialServiceRequest("failure-service", subject.journey.Upstream.URL(), "filesystem"))
	Expect(err).NotTo(HaveOccurred())
	body, err := helpers.CredentialResponseObject(response)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusCreated))
	value, ok := body["id"].(string)
	Expect(ok).To(BeTrue())
	subject.serviceID, err = id.ParseServiceID(value)
	Expect(err).NotTo(HaveOccurred())
	subject.redirectURI = server.EndUserURL + "/done"
	subject.request = server.EndUserRequest
	subject.logs = server.Logs
}

func credentialFailureAuthorize(subject *credentialFailureSubject) (*http.Response, error) {
	return subject.request(http.MethodGet, "/api/third-party/"+subject.serviceID.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(subject.redirectURI), nil)
}

func credentialFailureRefresh(subject *credentialFailureSubject) (*http.Response, error) {
	return subject.request(http.MethodPost, "/api/third-party/"+subject.serviceID.String()+"/session/refresh", nil)
}

func credentialFailurePendingCode(subject *credentialFailureSubject, clientID string) *url.URL {
	GinkgoHelper()
	beforeTokens := len(subject.journey.Upstream.GetTokenRequests())
	beforeAuthorizations := subject.journey.Upstream.GetAuthorizationRequestCount()
	response, err := credentialFailureAuthorize(subject)
	Expect(err).NotTo(HaveOccurred())
	location, err := url.Parse(response.Header.Get("Location"))
	Expect(response.Body.Close()).To(Succeed())
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	Expect(location.Scheme + "://" + location.Host).To(Equal(subject.journey.Upstream.URL()))
	Expect(location.Path).To(Equal("/oauth/authorize"))
	Expect(location.Query().Get("client_id")).To(Equal(clientID))
	Expect(len(subject.journey.Upstream.GetTokenRequests())).To(Equal(beforeTokens))
	Expect(subject.journey.Upstream.GetAuthorizationRequestCount()).To(Equal(beforeAuthorizations))
	callback, err := helpers.ProviderAuthorizationCallback(location.String())
	Expect(err).NotTo(HaveOccurred())
	Expect(subject.journey.Upstream.GetAuthorizationRequestCount()).To(Equal(beforeAuthorizations + 1))
	return callback
}

func expectCredentialFailurePair(subject *credentialFailureSubject, before int, clientID, clientSecret string) {
	GinkgoHelper()
	requests := subject.journey.Upstream.GetTokenRequests()[before:]
	Expect(requests).NotTo(BeEmpty())
	for _, request := range requests {
		selectedID, selectedSecret, err := helpers.CredentialTokenRequest(request)
		Expect(err).NotTo(HaveOccurred())
		Expect(selectedID).To(Equal(clientID))
		Expect(selectedSecret).To(Equal(clientSecret))
	}
}

func expectCredentialFailureConnected(subject *credentialFailureSubject, clientID, clientSecret string) {
	GinkgoHelper()
	callback := credentialFailurePendingCode(subject, clientID)
	before := len(subject.journey.Upstream.GetTokenRequests())
	response, err := subject.request(http.MethodGet, callback.RequestURI(), nil)
	Expect(err).NotTo(HaveOccurred())
	location, err := url.Parse(response.Header.Get("Location"))
	Expect(response.Body.Close()).To(Succeed())
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(http.StatusFound))
	Expect(location.Path).To(Equal("/done"))
	Expect(location.Query().Get("error")).To(BeEmpty())
	Expect(location.Query().Get("success")).To(Equal("true"))
	Expect(location.Query().Get("service_id")).To(Equal(subject.serviceID.String()))
	expectCredentialFailurePair(subject, before, clientID, clientSecret)
	if subject.journey.Storage != nil {
		session, err := subject.journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(subject.journey.Principal), subject.serviceID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session).NotTo(BeNil())
		Expect(session.UpstreamClientID).NotTo(BeNil())
		Expect(session.UpstreamClientID.String()).To(Equal(clientID))
	}
}

func expectCredentialFailureSafe(subject *credentialFailureSubject, text string) {
	GinkgoHelper()
	for _, forbidden := range []string{
		fixtures.CredentialClientID, fixtures.CredentialClientSecret,
		fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret,
		subject.binding["client_id_file"], subject.binding["client_secret_file"], subject.journey.Directory,
		"no such file or directory", "permission denied", "is a directory",
		"provider-controlled-description", "Authorization: Basic", "client_secret=", "code_verifier=",
	} {
		Expect(text).NotTo(ContainSubstring(forbidden))
	}
}

func expectCredentialFailureEvent(subject *credentialFailureSubject, event, operation, outcome, reason string) {
	GinkgoHelper()
	var matching []map[string]any
	Eventually(func() ([]map[string]any, error) {
		records, err := subject.logs.Records()
		if err != nil {
			return nil, err
		}
		matching = nil
		for _, record := range records {
			if record["event"] == event && record["service_id"] == subject.serviceID.String() && record["operation"] == operation {
				matching = append(matching, record)
			}
		}
		return matching, nil
	}, time.Second, time.Millisecond).ShouldNot(BeEmpty(), "expected credential event for the actual service and operation")
	for _, record := range matching {
		Expect(record["outcome"]).To(Equal(outcome))
		if reason != "" {
			Expect(record["reason"]).To(Equal(reason))
		}
		encoded, err := json.Marshal(record)
		Expect(err).NotTo(HaveOccurred())
		expectCredentialFailureSafe(subject, string(encoded))
	}
}

func expectCredentialSourceFailure(subject *credentialFailureSubject, operation, reason string, run func() (*http.Response, error)) {
	GinkgoHelper()
	beforeTokens := len(subject.journey.Upstream.GetTokenRequests())
	beforeAuthorizations := subject.journey.Upstream.GetAuthorizationRequestCount()
	subject.logs.Reset()
	started := time.Now()
	response, err := run()
	elapsed := time.Since(started)
	Expect(err).NotTo(HaveOccurred())
	Expect(elapsed).To(BeNumerically("<", time.Second), "non-regular sources must fail promptly rather than wait for data")
	Expect(len(subject.journey.Upstream.GetTokenRequests())).To(Equal(beforeTokens), "source failures must never authenticate with stored or previous credentials")
	Expect(subject.journey.Upstream.GetAuthorizationRequestCount()).To(Equal(beforeAuthorizations))
	if operation == "code_exchange" {
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		Expect(location.Path).To(Equal("/sessions"))
		Expect(location.Query().Get("error")).To(Equal("callback_failed"))
		Expect(location.Query().Get("error_description")).To(Equal("Authorization failed - please try again"))
		Expect(location.Query().Get("success")).To(BeEmpty())
		body, err := io.ReadAll(response.Body)
		Expect(response.Body.Close()).To(Succeed())
		Expect(err).NotTo(HaveOccurred())
		expectCredentialFailureSafe(subject, string(body)+location.String())
	} else {
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(response.Header.Get("Location")).To(BeEmpty())
		expectedError := "internal_error"
		if operation == "automatic_refresh" {
			expectedError = "server_error"
			Expect(body).NotTo(HaveKey("error_uri"))
		}
		Expect(body["error"]).To(Equal(expectedError))
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		expectCredentialFailureSafe(subject, string(encoded))
	}
	if operation == "automatic_refresh" {
		operation = "refresh"
		records, err := subject.logs.Records()
		Expect(err).NotTo(HaveOccurred())
		Expect(records).To(ContainElement(SatisfyAll(
			HaveKeyWithValue("token_exchange.failure_detail", "credential_source_unavailable"),
			HaveKeyWithValue("token_exchange.failure_stage", "refresh"),
			HaveKeyWithValue("token_exchange.outcome", "infrastructure_error"),
		)))
	}
	expectCredentialFailureEvent(subject, "session.oauth2.credential_source_failed", operation, "failed", reason)
	records, err := subject.logs.Records()
	Expect(err).NotTo(HaveOccurred())
	for _, record := range records {
		if record["service_id"] == subject.serviceID.String() && record["operation"] == operation {
			Expect(record).NotTo(SatisfyAny(
				HaveKeyWithValue("event", "session.oauth2.credential_files_used"),
				HaveKeyWithValue("event", "session.oauth2.credential_provider_rejected"),
			))
		}
	}
	expectCredentialFailureSafe(subject, subject.logs.Raw())
}

func exerciseCredentialFileFailure(subject *credentialFailureSubject, reason string, makeUnavailable func(string)) {
	GinkgoHelper()
	expectCredentialFailureConnected(subject, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
	pending := credentialFailurePendingCode(subject, fixtures.CredentialClientID)
	makeUnavailable(subject.binding[subject.fileKey])
	if subject.fileKey == "client_id_file" {
		expectCredentialSourceFailure(subject, "authorization_initiation", reason, func() (*http.Response, error) {
			return credentialFailureAuthorize(subject)
		})
	} else {
		// Initiation must not read even an unusable secret. A real provider visit
		// supplies a new code whose exchange must still fail at pair acquisition.
		pending = credentialFailurePendingCode(subject, fixtures.CredentialClientID)
	}
	expectCredentialSourceFailure(subject, "code_exchange", reason, func() (*http.Response, error) {
		return subject.request(http.MethodGet, pending.RequestURI(), nil)
	})
	expectCredentialSourceFailure(subject, "refresh", reason, func() (*http.Response, error) {
		return credentialFailureRefresh(subject)
	})
}

func exerciseCredentialFileFirstUseFailure(subject *credentialFailureSubject, reason string, makeUnavailable func(string)) {
	GinkgoHelper()
	makeUnavailable(subject.binding[subject.fileKey])
	if subject.fileKey == "client_id_file" {
		expectCredentialSourceFailure(subject, "authorization_initiation", reason, func() (*http.Response, error) {
			return credentialFailureAuthorize(subject)
		})
	} else {
		pending := credentialFailurePendingCode(subject, fixtures.CredentialClientID)
		expectCredentialSourceFailure(subject, "code_exchange", reason, func() (*http.Response, error) {
			return subject.request(http.MethodGet, pending.RequestURI(), nil)
		})
	}
	restoreCredentialFailurePath(subject)
	Expect(helpers.PublishCredentialPair(subject.binding, fixtures.CredentialClientID, fixtures.CredentialClientSecret)).To(Succeed())
	// A real connection supplies the session needed to exercise explicit refresh;
	// the same source failure must remain closed after that successful use.
	exerciseCredentialFileFailure(subject, reason, makeUnavailable)
}

func removeCredentialFailurePath(path string) {
	GinkgoHelper()
	err := os.Remove(path)
	if !os.IsNotExist(err) {
		Expect(err).NotTo(HaveOccurred())
	}
}

func restoreCredentialFailurePath(subject *credentialFailureSubject) {
	GinkgoHelper()
	path := subject.binding[subject.fileKey]
	removeCredentialFailurePath(path)
	target := "identity"
	if subject.fileKey == "client_secret_file" {
		target = "credential"
	}
	Expect(os.Symlink("..data/"+target, path)).To(Succeed())
}

func credentialFailureTokenExchange(subject *credentialFailureSubject) url.Values {
	GinkgoHelper()
	ctx := context.Background()
	agent := fixtures.ValidAgent()
	Expect(subject.journey.Storage.Agents().Create(ctx, agent)).To(Succeed())
	Expect(fixtures.SeedPlaceholderGrantData(ctx, subject.journey.Storage, subject.serviceID)).To(Succeed())
	Expect(subject.journey.Storage.UserGrants().Create(ctx, fixtures.ActiveGrant(subject.journey.Principal, agent.ID.String(), subject.serviceID.String(), []string{"profile"}))).To(Succeed())
	tokens, err := helpers.BuildTokenExchangeJWTs(subject.journey.Upstream.GetPrivateKeyPEM(), subject.journey.Upstream.URL(), "token-exchange-broker", subject.journey.Principal, agent.ID.String(), "failure-gateway", time.Now())
	Expect(err).NotTo(HaveOccurred())
	return url.Values{
		"grant_type":            {"urn:ietf:params:oauth:grant-type:token-exchange"},
		"subject_token":         {tokens.SubjectToken},
		"subject_token_type":    {"urn:ietf:params:oauth:token-type:access_token"},
		"client_assertion_type": {"urn:ietf:params:oauth:client-assertion-type:jwt-bearer"},
		"client_assertion":      {tokens.ClientAssertion},
		"resource":              {subject.journey.Upstream.URL() + "/resource"},
	}
}

func expireCredentialFailureAccessToken(subject *credentialFailureSubject) {
	GinkgoHelper()
	ctx := context.Background()
	session, err := subject.journey.Storage.UserSessions().FindByPrincipalAndService(ctx, id.Principal(subject.journey.Principal), subject.serviceID)
	Expect(err).NotTo(HaveOccurred())
	Expect(session).NotTo(BeNil())
	copy := *session
	past := time.Now().Add(-time.Hour)
	copy.AccessTokenExpiresAt = &past
	Expect(subject.journey.Storage.UserSessions().Create(ctx, &copy)).To(Succeed())
}

func expectCredentialProviderRejection(subject *credentialFailureSubject, operation string, run func() (*http.Response, error)) {
	GinkgoHelper()
	before := len(subject.journey.Upstream.GetTokenRequests())
	subject.logs.Reset()
	response, err := run()
	Expect(err).NotTo(HaveOccurred())
	expectCredentialFailurePair(subject, before, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)
	if operation == "code_exchange" {
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		body, err := io.ReadAll(response.Body)
		Expect(response.Body.Close()).To(Succeed())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		Expect(location.Path).To(Equal("/sessions"))
		Expect(location.Query().Get("error")).To(Equal("callback_failed"))
		Expect(location.Query().Get("success")).To(BeEmpty())
		expectCredentialFailureSafe(subject, string(body)+location.String())
	} else {
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		if operation == "automatic_refresh" {
			Expect(response.StatusCode).To(Equal(http.StatusBadRequest))
			Expect(body["error"]).To(Equal("invalid_grant"))
			operation = "refresh"
			records, err := subject.logs.Records()
			Expect(err).NotTo(HaveOccurred())
			Expect(records).To(ContainElement(SatisfyAll(
				HaveKeyWithValue("token_exchange.failure_detail", "refresh_rejected"),
				HaveKeyWithValue("token_exchange.failure_stage", "refresh"),
				HaveKeyWithValue("token_exchange.outcome", "reauth_required"),
			)))
		} else {
			Expect(response.StatusCode).To(Equal(http.StatusBadGateway))
			Expect(body["error"]).To(Equal("refresh_failed"))
		}
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		expectCredentialFailureSafe(subject, string(encoded))
	}
	expectCredentialFailureEvent(subject, "session.oauth2.credential_files_used", operation, "used", "")
	expectCredentialFailureEvent(subject, "session.oauth2.credential_provider_rejected", operation, "rejected", "provider_rejected")
	records, err := subject.logs.Records()
	Expect(err).NotTo(HaveOccurred())
	Expect(records).NotTo(ContainElement(HaveKeyWithValue("event", "session.oauth2.credential_source_failed")))
	expectCredentialFailureSafe(subject, subject.logs.Raw())
}

var _ = Describe("OAuth2 mandatory credential file failures", Label("oauth2-secret-file-overrides"), func() {
	var subjects []*credentialFailureSubject

	BeforeEach(func() {
		subjects = nil
		for _, fileKey := range []string{"client_id_file", "client_secret_file"} {
			journey, err := helpers.NewOAuth2CredentialJourney()
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(journey.Close)
			binding, err := journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(err).NotTo(HaveOccurred())
			subjects = append(subjects, &credentialFailureSubject{journey: journey, binding: binding, fileKey: fileKey})
		}
	})

	Context("when a running filesystem service needs an unusable source", func() {
		BeforeEach(func() {
			for _, subject := range subjects {
				startCredentialFailureJourney(subject)
			}
		})

		// US3-S1 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S1 rejects each nonexistent required file before provider authentication", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "not_found", removeCredentialFailurePath)
			}
		})

		// US3-S3 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S3 rejects each empty required file without stored or previous-value fallback", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "empty", func(path string) {
					removeCredentialFailurePath(path)
					Expect(os.WriteFile(path, nil, 0o644)).To(Succeed())
				})
			}
		})

		// US3-S4 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S4 rejects each whitespace-only required file without fallback", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "empty", func(path string) {
					removeCredentialFailurePath(path)
					Expect(os.WriteFile(path, []byte(" \t\r\n\u2003 "), 0o644)).To(Succeed())
				})
			}
		})

		// US3-S5 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S5 rejects each broken symlink while keeping the other projected-volume file usable", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "not_found", func(path string) {
					removeCredentialFailurePath(path)
					Expect(os.Symlink(filepath.Join(subject.journey.Directory, "unpublished-target"), path)).To(Succeed())
				})
			}
		})

		// US3-S6 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S6 loses neither source requirement after a successful connection", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFailure(subject, "not_found", removeCredentialFailurePath)
				// The successful session still exists, but cannot reuse its last read.
				session, err := subject.journey.Storage.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(subject.journey.Principal), subject.serviceID)
				Expect(err).NotTo(HaveOccurred())
				Expect(session).NotTo(BeNil())
				Expect(session.UpstreamClientID).NotTo(BeNil())
				Expect(session.UpstreamClientID.String()).To(Equal(fixtures.CredentialClientID))
			}
		})

		// US3-S8 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S8 uses restored current values on the next connection and refresh without updating registration", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFailure(subject, "not_found", removeCredentialFailurePath)
				restoreCredentialFailurePath(subject)
				clientID := fixtures.CredentialClientID
				if subject.fileKey == "client_id_file" {
					clientID = fixtures.AlternateCredentialClientID
				}
				Expect(helpers.PublishCredentialPair(subject.binding, clientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				expectCredentialFailureConnected(subject, clientID, fixtures.RotatedCredentialSecret)
				before := len(subject.journey.Upstream.GetTokenRequests())
				response, err := credentialFailureRefresh(subject)
				Expect(err).NotTo(HaveOccurred())
				Expect(response.Body.Close()).To(Succeed())
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				expectCredentialFailurePair(subject, before, clientID, fixtures.RotatedCredentialSecret)
			}
		})

		// US3-S10 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S10 rejects directory targets for either required file before authentication", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "not_regular", func(path string) {
					removeCredentialFailurePath(path)
					Expect(os.Mkdir(path, 0o755)).To(Succeed())
				})
			}
		})

		// US3-S12 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S12 rejects either regular file above 65536 bytes before trimming or authentication", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFirstUseFailure(subject, "too_large", func(path string) {
					removeCredentialFailurePath(path)
					// Padding cannot make an oversized file acceptable after TrimSpace.
					Expect(os.WriteFile(path, []byte("x"+strings.Repeat(" ", 65536)), 0o644)).To(Succeed())
				})
			}
		})
	})

	Context("when a fresh broker rejects unavailable mandatory files", func() {
		// US3-S2 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S2 fails closed on actual permission denial for each file and restores permissions during cleanup", func() {
			for _, subject := range subjects {
				startCredentialFailureProcess(subject)
				exerciseCredentialFileFirstUseFailure(subject, "permission_denied", func(path string) {
					target, err := filepath.EvalSymlinks(path)
					Expect(err).NotTo(HaveOccurred())
					DeferCleanup(func() { Expect(os.Chmod(target, 0o644)).To(Succeed()) })
					Expect(os.Chmod(target, 0)).To(Succeed())
				})
			}
		})

		// US3-S7 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S7 restarts a fresh broker and rejects each unavailable source before its first successful read", func() {
			for _, subject := range subjects {
				first := startCredentialFailureServer(subject)
				first.Close()
				removeCredentialFailurePath(subject.binding[subject.fileKey])
				startCredentialFailureServer(subject)
				// Memory-backed restart requires registration, not a fabricated
				// persisted service. Neither application has authenticated this service.
				Expect(subject.journey.Upstream.GetTokenRequests()).To(BeEmpty())
				if subject.fileKey == "client_id_file" {
					expectCredentialSourceFailure(subject, "authorization_initiation", "not_found", func() (*http.Response, error) {
						return credentialFailureAuthorize(subject)
					})
				} else {
					pending := credentialFailurePendingCode(subject, fixtures.CredentialClientID)
					expectCredentialSourceFailure(subject, "code_exchange", "not_found", func() (*http.Response, error) {
						return subject.request(http.MethodGet, pending.RequestURI(), nil)
					})
				}
			}
		})

		// US3-S11 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S11 promptly rejects FIFO, Unix socket, and device targets for each file with bounded process cleanup", func() {
			for _, subject := range subjects {
				startCredentialFailureProcess(subject)
				for _, kind := range []string{"fifo", "unix_socket", "device"} {
					By("rejecting " + subject.fileKey + " backed by " + kind)
					exerciseCredentialFileFailure(subject, "not_regular", func(path string) {
						removeCredentialFailurePath(path)
						switch kind {
						case "fifo":
							Expect(syscall.Mkfifo(path, 0o644)).To(Succeed())
						case "unix_socket":
							// Unix socket names have a small OS limit; do not put the
							// listener in the potentially long platform temp directory.
							directory, err := os.MkdirTemp("/tmp", "aib-socket-")
							Expect(err).NotTo(HaveOccurred())
							Expect(os.Chmod(directory, 0o755)).To(Succeed())
							DeferCleanup(func() { Expect(os.RemoveAll(directory)).To(Succeed()) })
							socketPath := filepath.Join(directory, "source")
							listener, err := net.Listen("unix", socketPath)
							Expect(err).NotTo(HaveOccurred())
							DeferCleanup(listener.Close)
							Expect(os.Symlink(socketPath, path)).To(Succeed())
						case "device":
							info, err := os.Stat("/dev/null")
							Expect(err).NotTo(HaveOccurred())
							Expect(info.Mode() & os.ModeDevice).NotTo(BeZero())
							Expect(os.Symlink("/dev/null", path)).To(Succeed())
						}
					})
					restoreCredentialFailurePath(subject)
				}
			}
		})
	})

	Context("when operators distinguish unavailable sources from real provider rejection", func() {
		var exchangeForms map[*credentialFailureSubject]url.Values
		BeforeEach(func() {
			exchangeForms = map[*credentialFailureSubject]url.Values{}
			for _, subject := range subjects {
				startCredentialFailureJourney(subject)
				exchangeForms[subject] = credentialFailureTokenExchange(subject)
			}
		})

		// US3-S9 from specs/051-oauth2-secret-file-overrides/spec.md
		It("US3-S9 preserves safe generic responses and distinct source/provider events through real RFC8693 automatic refresh", func() {
			for _, subject := range subjects {
				exerciseCredentialFileFailure(subject, "not_found", removeCredentialFailurePath)
				expireCredentialFailureAccessToken(subject)
				postExchange := func() (*http.Response, error) {
					return subject.journey.EndUser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(exchangeForms[subject].Encode()))
				}
				expectCredentialSourceFailure(subject, "automatic_refresh", "not_found", postExchange)
				restoreCredentialFailurePath(subject)
				Expect(helpers.PublishCredentialPair(subject.binding, fixtures.CredentialClientID, fixtures.RotatedCredentialSecret)).To(Succeed())
				subject.journey.Upstream.WithErrorResponseAndDescription("invalid_grant", "provider-controlled-description "+fixtures.RotatedCredentialSecret)
				pending := credentialFailurePendingCode(subject, fixtures.CredentialClientID)
				expectCredentialProviderRejection(subject, "code_exchange", func() (*http.Response, error) {
					return subject.request(http.MethodGet, pending.RequestURI(), nil)
				})
				expectCredentialProviderRejection(subject, "refresh", func() (*http.Response, error) {
					return credentialFailureRefresh(subject)
				})
				expectCredentialProviderRejection(subject, "automatic_refresh", postExchange)
			}
		})
	})
})
