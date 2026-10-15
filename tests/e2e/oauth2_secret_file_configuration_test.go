package e2e_test

import (
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("OAuth2 credential-file configuration", Label("oauth2-secret-file-overrides"), func() {
	var (
		journey *helpers.OAuth2CredentialJourney
		pairA   map[string]string
		pairB   map[string]string
		pairC   map[string]string
	)

	BeforeEach(func() {
		var err error
		journey, err = helpers.NewOAuth2CredentialJourney()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(journey.Close)
		pairA, err = journey.WritePair(fixtures.CredentialClientID, fixtures.CredentialClientSecret)
		Expect(err).NotTo(HaveOccurred())
		pairB, err = journey.WritePair(fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)
		Expect(err).NotTo(HaveOccurred())
		pairC, err = journey.WritePair("synthetic-client-C", "synthetic-secret-C")
		Expect(err).NotTo(HaveOccurred())
	})

	encode := func(value any) string {
		encoded, err := json.Marshal(value)
		Expect(err).NotTo(HaveOccurred())
		return string(encoded)
	}

	cliCommand := func(value string) *cobra.Command {
		command := &cobra.Command{Use: "credential-config"}
		command.Flags().String("third_party_oauth2.credential_files", "{}", "")
		Expect(command.ParseFlags([]string{"--third_party_oauth2.credential_files=" + value})).To(Succeed())
		return command
	}

	configuration := func(source string, bindings any) bootstrap.CredentialServerOptions {
		options := bootstrap.CredentialServerOptions{
			Document: fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), map[string]any{}, 8000, 14000),
		}
		switch source {
		case "YAML":
			options.Document = fixtures.CredentialConfigurationDocument(journey.Upstream.URL(), bindings, 8000, 14000)
		case "environment":
			options.Environment = map[string]string{"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": encode(bindings)}
		case "CLI":
			options.Command = cliCommand(encode(bindings))
		}
		return options
	}

	launch := func(options bootstrap.CredentialServerOptions) (*bootstrap.CredentialServer, error) {
		directory, err := os.MkdirTemp(journey.Directory, "configured-server-")
		Expect(err).NotTo(HaveOccurred())
		options.Directory = directory
		options.Principal = journey.Principal
		process, err := bootstrap.StartCredentialServer(options)
		if process != nil {
			DeferCleanup(process.Close)
		}
		return process, err
	}

	start := func(options bootstrap.CredentialServerOptions) *bootstrap.CredentialServer {
		process, err := launch(options)
		Expect(err).NotTo(HaveOccurred())
		Expect(process).NotTo(BeNil())
		return process
	}

	rejectStartup := func(options bootstrap.CredentialServerOptions) {
		authorizations := journey.Upstream.GetAuthorizationRequestCount()
		tokens := len(journey.Upstream.GetTokenRequests())
		process, err := launch(options)
		Expect(err).To(HaveOccurred(), "invalid credential-file configuration must reject startup")
		Expect(process).NotTo(BeNil(), "rejected bootstrap must remain available for cleanup and listener observations")
		for _, baseURL := range []string{process.AdminURL, process.EndUserURL} {
			listener, parseErr := url.Parse(baseURL)
			Expect(parseErr).NotTo(HaveOccurred())
			connection, dialErr := net.DialTimeout("tcp", listener.Host, 500*time.Millisecond)
			if connection != nil {
				Expect(connection.Close()).To(Succeed())
			}
			Expect(dialErr).To(HaveOccurred(), "invalid configuration must not leave a reachable listener at %s", baseURL)
		}
		Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizations))
		Expect(process.Logs.Raw()).To(ContainSubstring("third_party_oauth2.credential_files"))
		for _, pair := range []map[string]string{pairA, pairB, pairC} {
			Expect(process.Logs.Raw()).NotTo(ContainSubstring(pair["client_id_file"]))
			Expect(process.Logs.Raw()).NotTo(ContainSubstring(pair["client_secret_file"]))
		}
		Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokens))
	}

	adminObject := func(process *bootstrap.CredentialServer, method, path string, request any, status int) map[string]any {
		response, err := process.AdminJSON(method, path, request)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(status))
		return body
	}

	endUserObject := func(process *bootstrap.CredentialServer, path string, status int) map[string]any {
		response, err := process.EndUserRequest(http.MethodGet, path, nil)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(status))
		return body
	}

	listServices := func(process *bootstrap.CredentialServer) []map[string]any {
		response, err := process.AdminJSON(http.MethodGet, "/api/services", nil)
		Expect(err).NotTo(HaveOccurred())
		var services []map[string]any
		err = json.NewDecoder(response.Body).Decode(&services)
		Expect(response.Body.Close()).To(Succeed())
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		return services
	}

	serviceRequest := func(canonicalID, source string) map[string]any {
		request := fixtures.CredentialServiceRequest(canonicalID, journey.Upstream.URL(), source)
		request["protected_resources"] = []string{journey.Upstream.URL() + "/resource/" + canonicalID}
		return request
	}

	register := func(process *bootstrap.CredentialServer, canonicalID, source string) string {
		body := adminObject(process, http.MethodPost, "/api/services", serviceRequest(canonicalID, source), http.StatusCreated)
		serviceID, ok := body["id"].(string)
		Expect(ok).To(BeTrue())
		_, err := id.ParseServiceID(serviceID)
		Expect(err).NotTo(HaveOccurred())
		return serviceID
	}

	expectFilesystem := func(body map[string]any, canonicalID string) {
		Expect(body).To(HaveKeyWithValue("canonical_id", canonicalID))
		Expect(body).To(HaveKeyWithValue("credential_source", "filesystem"))
		Expect(body).NotTo(HaveKey("client_id"))
		Expect(body).NotTo(HaveKey("client_secret"))
	}

	authorize := func(process *bootstrap.CredentialServer, serviceID string) *http.Response {
		response, err := process.EndUserRequest(http.MethodGet,
			"/api/third-party/"+serviceID+"/oauth2/authorize?redirect_uri="+url.QueryEscape(process.EndUserURL+"/done"), nil)
		Expect(err).NotTo(HaveOccurred())
		return response
	}

	connect := func(process *bootstrap.CredentialServer, serviceID, clientID, secret string) {
		tokenCount := len(journey.Upstream.GetTokenRequests())
		response := authorize(process, serviceID)
		authorizationURL, err := url.Parse(response.Header.Get("Location"))
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		Expect(err).NotTo(HaveOccurred())
		providerURL, err := url.Parse(journey.Upstream.URL())
		Expect(err).NotTo(HaveOccurred())
		Expect(authorizationURL.Scheme).To(Equal(providerURL.Scheme))
		Expect(authorizationURL.Host).To(Equal(providerURL.Host))
		Expect(authorizationURL.Query().Get("client_id")).To(Equal(clientID))

		callback, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
		Expect(err).NotTo(HaveOccurred())
		Expect(journey.Upstream.GetLastRequest().Header.Get("X-Remote-User")).To(BeEmpty())
		Expect(callback.Scheme + "://" + callback.Host).To(Equal(process.EndUserURL))
		response, err = process.EndUserRequest(http.MethodGet, callback.RequestURI(), nil)
		Expect(err).NotTo(HaveOccurred())
		location, err := url.Parse(response.Header.Get("Location"))
		Expect(response.Body.Close()).To(Succeed())
		Expect(response.StatusCode).To(Equal(http.StatusFound))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Scheme + "://" + location.Host).To(Equal(process.EndUserURL))
		Expect(location.Path).To(Equal("/done"))
		Expect(location.Query().Get("error")).To(BeEmpty())
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("service_id")).To(Equal(serviceID))

		requests := journey.Upstream.GetTokenRequests()
		Expect(requests).To(HaveLen(tokenCount + 1))
		selectedClientID, selectedSecret, err := helpers.CredentialTokenRequest(requests[tokenCount])
		Expect(err).NotTo(HaveOccurred())
		Expect(selectedClientID).To(Equal(clientID))
		Expect(selectedSecret).To(Equal(secret))
		Expect(requests[tokenCount].Header.Get("X-Remote-User")).To(BeEmpty())
		form, err := url.ParseQuery(requests[tokenCount].Body)
		Expect(err).NotTo(HaveOccurred())
		Expect(form.Get("grant_type")).To(Equal("authorization_code"))

		body := endUserObject(process, "/api/third-party/"+serviceID+"/session", http.StatusOK)
		data, ok := body["data"].(map[string]any)
		Expect(ok).To(BeTrue())
		session, ok := data["session"].(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(session["id"]).NotTo(BeEmpty())
		Expect(session["service_id"]).To(Equal(serviceID))
		Expect(session["principal"]).To(Equal(journey.Principal))
	}

	expectUnavailable := func(process *bootstrap.CredentialServer, serviceID string) {
		authorizations := journey.Upstream.GetAuthorizationRequestCount()
		tokens := len(journey.Upstream.GetTokenRequests())
		response := authorize(process, serviceID)
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusInternalServerError))
		Expect(body["error"]).To(Equal("internal_error"))
		Expect(response.Header.Get("Location")).To(BeEmpty())
		Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizations))
		Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokens))
	}

	Context("when operators configure valid bindings through established sources", func() {
		// US6-S1 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S1] connects using the exact pair from a YAML configuration file", func() {
			process := start(configuration("YAML", map[string]map[string]string{"Config.One_1": pairA}))
			serviceID := register(process, "Config.One_1", "filesystem")
			connect(process, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), "Config.One_1")
		})

		// US6-S2 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S2] connects independent case-sensitive dotted pairs from the real environment", func() {
			process := start(configuration("environment", map[string]map[string]string{
				"Env.Pair.One": pairA,
				"env.Pair.One": pairB,
			}))
			for _, service := range []struct{ canonicalID, clientID, secret string }{
				{"Env.Pair.One", fixtures.CredentialClientID, fixtures.CredentialClientSecret},
				{"env.Pair.One", fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret},
			} {
				serviceID := register(process, service.canonicalID, "filesystem")
				connect(process, serviceID, service.clientID, service.secret)
				expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), service.canonicalID)
			}
		})

		// US6-S3 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S3] connects independent dotted pairs from CLI JSON configuration", func() {
			process := start(configuration("CLI", map[string]map[string]string{
				"CLI.Pair-One": pairA,
				"CLI_Pair.Two": pairB,
			}))
			for _, service := range []struct{ canonicalID, clientID, secret string }{
				{"CLI.Pair-One", fixtures.CredentialClientID, fixtures.CredentialClientSecret},
				{"CLI_Pair.Two", fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret},
			} {
				serviceID := register(process, service.canonicalID, "filesystem")
				connect(process, serviceID, service.clientID, service.secret)
				expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), service.canonicalID)
			}
		})

		// US6-S4 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S4] replaces the whole map in CLI then environment then YAML order and preserves sources under winning empty maps", func() {
			yamlBindings := map[string]map[string]string{"Shared.Pair": pairA, "YAML.Only": pairA, "Stored.Control": pairB}
			envBindings := map[string]map[string]string{"Shared.Pair": pairB, "Environment.Only": pairB, "Stored.Control": pairB}
			cliBindings := map[string]map[string]string{"Shared.Pair": pairC, "CLI.Only": pairC, "Stored.Control": pairC}
			for _, variant := range []struct {
				name, clientID, secret string
				yaml, environment, cli map[string]map[string]string
				winning                map[string]map[string]string
			}{
				{"YAML only", fixtures.CredentialClientID, fixtures.CredentialClientSecret, yamlBindings, nil, nil, yamlBindings},
				{"environment replaces YAML", fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret, yamlBindings, envBindings, nil, envBindings},
				{"CLI replaces environment and YAML", "synthetic-client-C", "synthetic-secret-C", yamlBindings, envBindings, cliBindings, cliBindings},
				{"empty YAML", "", "", map[string]map[string]string{}, nil, nil, map[string]map[string]string{}},
				{"empty environment replaces YAML", "", "", yamlBindings, map[string]map[string]string{}, nil, map[string]map[string]string{}},
				{"empty CLI replaces environment and YAML", "", "", yamlBindings, envBindings, map[string]map[string]string{}, map[string]map[string]string{}},
			} {
				By(variant.name)
				options := configuration("YAML", variant.yaml)
				if variant.environment != nil {
					options.Environment = map[string]string{"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": encode(variant.environment)}
				}
				if variant.cli != nil {
					options.Command = cliCommand(encode(variant.cli))
				}
				process := start(options)
				for _, canonicalID := range []string{"Shared.Pair", "YAML.Only", "Environment.Only", "CLI.Only"} {
					serviceID := register(process, canonicalID, "filesystem")
					expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), canonicalID)
					if _, bound := variant.winning[canonicalID]; bound {
						connect(process, serviceID, variant.clientID, variant.secret)
					} else {
						expectUnavailable(process, serviceID)
					}
					expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), canonicalID)
				}
				storedID := register(process, "Stored.Control", "stored")
				connect(process, storedID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
				stored := adminObject(process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK)
				Expect(stored["credential_source"]).To(Equal("stored"))
				Expect(stored["client_id"]).To(Equal(fixtures.CredentialClientID))
				Expect(stored["client_secret"]).To(Equal("REDACTED"))
				process.Close()
			}
		})
	})

	Context("when configuration is invalid before listeners start", func() {
		// US6-S5 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S5] rejects invalid canonical binding keys before either listener serves requests", func() {
			for _, invalid := range []struct{ source, key string }{
				{"YAML", "with/slash"},
				{"environment", "550e8400e29b41d4a716446655440000"},
				{"CLI", "non-ascii-ä"},
			} {
				By(invalid.source + " rejects canonical key " + invalid.key)
				rejectStartup(configuration(invalid.source, map[string]map[string]string{invalid.key: pairA}))
			}
		})

		// US6-S6 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S6] rejects representative invalid file-pair paths from each source before listeners", func() {
			for _, invalid := range []struct {
				source, field, name string
				value               any
				missing             bool
			}{
				{"YAML", "client_id_file", "empty", "", false},
				{"environment", "client_secret_file", "whitespace", " \t\n ", false},
				{"CLI", "client_id_file", "relative", "relative/credential", false},
				{"YAML", "client_secret_file", "missing", nil, true},
				{"environment", "client_id_file", "non-string", false, false},
			} {
				By(invalid.source + " rejects " + invalid.name + " " + invalid.field)
				pair := map[string]any{"client_id_file": pairA["client_id_file"], "client_secret_file": pairA["client_secret_file"]}
				if invalid.missing {
					delete(pair, invalid.field)
				} else {
					pair[invalid.field] = invalid.value
				}
				rejectStartup(configuration(invalid.source, map[string]any{"Invalid.Pair": pair}))
			}
			for _, source := range []string{"environment", "CLI"} {
				By(source + " rejects malformed JSON")
				options := configuration("YAML", map[string]any{})
				if source == "environment" {
					options.Environment = map[string]string{"IDENTITY_BROKER_THIRD_PARTY_OAUTH2_CREDENTIAL_FILES": "{"}
				} else {
					options.Command = cliCommand("{")
				}
				rejectStartup(options)
			}
		})
	})

	Context("when bindings do not make credentials usable", func() {
		// US6-S7 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S7] starts and serves metadata with unavailable files but fails at the operation requiring them", func() {
			for _, unavailable := range []string{"client_id_file", "client_secret_file", "both"} {
				By("unavailable " + unavailable)
				binding := map[string]string{"client_id_file": pairA["client_id_file"], "client_secret_file": pairA["client_secret_file"]}
				for _, field := range []string{"client_id_file", "client_secret_file"} {
					if unavailable == field || unavailable == "both" {
						binding[field] = filepath.Join(journey.Directory, "unavailable", field)
					}
				}
				process := start(configuration("YAML", map[string]map[string]string{"Unavailable.Files": binding}))
				serviceID := register(process, "Unavailable.Files", "filesystem")
				expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), "Unavailable.Files")
				services := listServices(process)
				Expect(services).To(HaveLen(1))
				expectFilesystem(services[0], "Unavailable.Files")
				endUserObject(process, "/api/third-party/sessions", http.StatusOK)
				endUserObject(process, "/api/third-party/"+serviceID+"/session", http.StatusNotFound)

				if unavailable == "client_secret_file" {
					response := authorize(process, serviceID)
					authorizationURL, err := url.Parse(response.Header.Get("Location"))
					Expect(response.Body.Close()).To(Succeed())
					Expect(response.StatusCode).To(Equal(http.StatusFound))
					Expect(err).NotTo(HaveOccurred())
					Expect(authorizationURL.Query().Get("client_id")).To(Equal(fixtures.CredentialClientID))
					Expect(authorizationURL.Scheme + "://" + authorizationURL.Host).To(Equal(journey.Upstream.URL()))
					callback, err := helpers.ProviderAuthorizationCallback(authorizationURL.String())
					Expect(err).NotTo(HaveOccurred())
					Expect(callback.Scheme + "://" + callback.Host).To(Equal(process.EndUserURL))
					Expect(journey.Upstream.GetLastRequest().Header.Get("X-Remote-User")).To(BeEmpty())
					authorizations := journey.Upstream.GetAuthorizationRequestCount()
					tokens := len(journey.Upstream.GetTokenRequests())
					response, err = process.EndUserRequest(http.MethodGet, callback.RequestURI(), nil)
					Expect(err).NotTo(HaveOccurred())
					location, err := url.Parse(response.Header.Get("Location"))
					Expect(response.Body.Close()).To(Succeed())
					Expect(response.StatusCode).To(Equal(http.StatusFound))
					Expect(err).NotTo(HaveOccurred())
					Expect(location.Query().Get("error")).To(Equal("callback_failed"))
					Expect(location.Query().Get("success")).To(BeEmpty())
					Expect(journey.Upstream.GetAuthorizationRequestCount()).To(Equal(authorizations))
					Expect(journey.Upstream.GetTokenRequests()).To(HaveLen(tokens))
				} else {
					expectUnavailable(process, serviceID)
				}
				endUserObject(process, "/api/third-party/"+serviceID+"/session", http.StatusNotFound)
				expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), "Unavailable.Files")
				process.Close()
			}
		})

		// US6-S8 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S8] does not register unknown IDs or select filesystem mode from configuration", func() {
			process := start(configuration("YAML", map[string]map[string]string{"Unknown.Service.1": pairB}))
			Expect(listServices(process)).To(BeEmpty())
			Expect(journey.Upstream.GetAuthorizationRequestCount()).To(BeZero())
			Expect(journey.Upstream.GetTokenRequests()).To(BeEmpty())

			storedID := register(process, "Unknown.Service.1", "")
			stored := adminObject(process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK)
			Expect(stored["credential_source"]).To(Equal("stored"))
			Expect(stored["client_id"]).To(Equal(fixtures.CredentialClientID))
			connect(process, storedID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			Expect(adminObject(process, http.MethodGet, "/api/services/"+storedID, nil, http.StatusOK)).To(Equal(stored))

			for _, canonicalID := range []string{"unknown.Service.1", "Unknown-Service-1"} {
				serviceID := register(process, canonicalID, "filesystem")
				expectUnavailable(process, serviceID)
				expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), canonicalID)
			}
			Expect(listServices(process)).To(HaveLen(3))
		})

		// US6-S9 from specs/051-oauth2-secret-file-overrides/spec.md
		It("[US6-S9] rejects canonical-ID clearing and uses only a reassigned exact binding without stored or name fallback", func() {
			process := start(configuration("YAML", map[string]map[string]string{
				"Canonical.Old":     pairA,
				"Canonical.New":     pairB,
				"canonical.new":     pairC,
				"Canonical-New":     pairC,
				"canonical.missing": pairC,
				"Canonical-Missing": pairC,
			}))
			serviceID := register(process, "Canonical.Old", "filesystem")
			connect(process, serviceID, fixtures.CredentialClientID, fixtures.CredentialClientSecret)
			original := adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK)
			expectFilesystem(original, "Canonical.Old")
			for _, cleared := range []any{nil, ""} {
				request := serviceRequest("Canonical.Old", "filesystem")
				request["canonical_id"] = cleared
				delete(request, "credential_source")
				delete(request, "protected_resources")
				adminObject(process, http.MethodPut, "/api/services/"+serviceID, request, http.StatusBadRequest)
				Expect(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK)).To(Equal(original))
			}

			request := serviceRequest("Canonical.New", "filesystem")
			delete(request, "credential_source")
			delete(request, "protected_resources")
			expectFilesystem(adminObject(process, http.MethodPut, "/api/services/"+serviceID, request, http.StatusOK), "Canonical.New")
			connect(process, serviceID, fixtures.AlternateCredentialClientID, fixtures.RotatedCredentialSecret)

			request = serviceRequest("Canonical.Missing", "filesystem")
			delete(request, "credential_source")
			delete(request, "protected_resources")
			expectFilesystem(adminObject(process, http.MethodPut, "/api/services/"+serviceID, request, http.StatusOK), "Canonical.Missing")
			expectUnavailable(process, serviceID)
			expectFilesystem(adminObject(process, http.MethodGet, "/api/services/"+serviceID, nil, http.StatusOK), "Canonical.Missing")
		})
	})
})
