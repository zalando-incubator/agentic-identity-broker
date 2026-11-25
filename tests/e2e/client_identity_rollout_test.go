package e2e_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	domainstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type rolloutHTTPResult struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func rolloutHTTP(broker *bootstrap.CredentialServer, principal, method, path string, body io.Reader) rolloutHTTPResult {
	response, err := broker.EndUserRequestAs(principal, method, path, body)
	if err != nil {
		return rolloutHTTPResult{err: err}
	}
	defer func() { _ = response.Body.Close() }()
	content, err := io.ReadAll(response.Body)
	return rolloutHTTPResult{status: response.StatusCode, header: response.Header, body: content, err: err}
}

var _ = Describe("Client identity continuous rolling upgrade", Label("docker", "client-identity-change"), func() {
	var (
		journey                  *helpers.OAuth2CredentialJourney
		postgres                 *bootstrap.PostgresFixture
		db                       *sql.DB
		store                    *storageadapter.Adapter
		document                 map[string]any
		binding                  map[string]string
		prior                    []*helpers.ClientIdentityPriorRelease
		current                  *bootstrap.CredentialServer
		restarted                *bootstrap.CredentialProcess
		serviceID, filesystemID  id.ServiceID
		agent                    *domainstorage.Agent
		alice, bob, storedSecret string
		hostPorts                []int
	)
	ctx := context.Background()

	object := func(result rolloutHTTPResult, status int) map[string]any {
		GinkgoHelper()
		Expect(result.err).NotTo(HaveOccurred())
		Expect(result.status).To(Equal(status), "%s", result.body)
		var body map[string]any
		Expect(json.Unmarshal(result.body, &body)).To(Succeed())
		return body
	}

	callback := func(broker *bootstrap.CredentialServer, principal string, target id.ServiceID) *url.URL {
		GinkgoHelper()
		result := rolloutHTTP(broker, principal, http.MethodGet, "/api/third-party/"+target.String()+"/oauth2/authorize?redirect_uri="+url.QueryEscape(broker.EndUserPublicURL+"/sessions"), nil)
		Expect(result.err).NotTo(HaveOccurred())
		Expect(result.status).To(Equal(http.StatusFound))
		provider, err := url.Parse(result.header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(provider.Query().Get("client_id")).To(Equal(fixtures.ReportsClientV1))
		value, err := helpers.ProviderAuthorizationCallback(provider.String())
		Expect(err).NotTo(HaveOccurred())
		return value
	}

	connect := func(broker *bootstrap.CredentialServer, principal string, target id.ServiceID) {
		GinkgoHelper()
		value := callback(broker, principal, target)
		result := rolloutHTTP(broker, principal, http.MethodGet, value.RequestURI(), nil)
		Expect(result.err).NotTo(HaveOccurred())
		Expect(result.status).To(Equal(http.StatusFound))
		location, err := url.Parse(result.header.Get("Location"))
		Expect(err).NotTo(HaveOccurred())
		Expect(location.Path).To(Equal("/sessions"))
		Expect(location.Query().Get("success")).To(Equal("true"))
		Expect(location.Query().Get("error")).To(BeEmpty())
	}

	register := func(request map[string]any) id.ServiceID {
		GinkgoHelper()
		response, err := prior[0].AdminJSON(http.MethodPost, "/api/services", request)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusCreated), "%v", body)
		value, err := id.ParseServiceID(body["id"].(string))
		Expect(err).NotTo(HaveOccurred())
		return value
	}

	startPrior := func() *helpers.ClientIdentityPriorRelease {
		GinkgoHelper()
		value, err := helpers.StartClientIdentityPriorRelease(ctx, journey.Directory, document, hostPorts)
		Expect(err).NotTo(HaveOccurred())
		Expect(value.Image).To(Equal(helpers.ClientIdentityPriorReleaseImage()))
		inspection, err := value.Container.Inspect(ctx)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(inspection.HostConfig.RestartPolicy.Name)).To(Or(BeEmpty(), Equal("no")))
		prior = append(prior, value)
		return value
	}

	startCurrent := func() {
		GinkgoHelper()
		var err error
		current, err = bootstrap.StartCredentialServer(bootstrap.CredentialServerOptions{Directory: journey.Directory, Document: document, Principal: alice})
		Expect(err).NotTo(HaveOccurred())
		journey.Config, err = bootstrap.LoadCredentialConfiguration(current.ConfigPath, nil, nil)
		Expect(err).NotTo(HaveOccurred())
	}

	BeforeEach(func() {
		journey, postgres, db, store, current, restarted = nil, nil, nil, nil, nil, nil
		prior = nil
		// Docker may not share the OS temp directory with its VM.
		workspace, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		journey, err = helpers.NewOAuth2CredentialJourneyIn(workspace)
		Expect(err).NotTo(HaveOccurred())
		alice, bob, storedSecret = fixtures.DefaultPrincipal().String(), fixtures.AnotherPrincipal().String(), fixtures.ReportsSecretV1
		binding, err = journey.WritePair(fixtures.ReportsClientV1, fixtures.ReportsSecretV1)
		Expect(err).NotTo(HaveOccurred())
		postgres, err = bootstrap.NewPostgresFixtureAtVersion(ctx, 48)
		Expect(err).NotTo(HaveOccurred())
		db, err = sql.Open("pgx", postgres.ConnectionURL)
		Expect(err).NotTo(HaveOccurred())
		document = fixtures.ClientIdentityRolloutDocument(journey.Upstream.URL(), postgres.ConnectionURL, map[string]map[string]string{"reports-files": binding})
		hostPorts = nil
		for _, endpoint := range []string{journey.Upstream.URL(), postgres.ConnectionURL} {
			port, err := helpers.ClientIdentityHostPort(endpoint)
			Expect(err).NotTo(HaveOccurred())
			hostPorts = append(hostPorts, port)
		}
		startPrior()
		serviceID = register(fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored"))
		filesystem := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "filesystem")
		filesystem["canonical_id"] = "reports-files"
		filesystem["protected_resources"] = []string{journey.Upstream.URL() + "/filesystem-resource"}
		filesystemID = register(filesystem)
		for _, principal := range []string{alice, bob} {
			connect(prior[0].CredentialServer, principal, serviceID)
			connect(prior[0].CredentialServer, principal, filesystemID)
		}
		// Historical stored sessions predate ADR 041-oauth2-client-secret-file-overrides's establishing-identity claim.
		result, err := db.ExecContext(ctx, "UPDATE user_sessions SET upstream_client_id=NULL WHERE service_id=$1", serviceID)
		Expect(err).NotTo(HaveOccurred())
		count, err := result.RowsAffected()
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int64(2)))
		result, err = db.ExecContext(ctx, "UPDATE user_sessions SET access_token_expires_at=NULL WHERE principal=$1 AND service_id=$2", bob, serviceID)
		Expect(err).NotTo(HaveOccurred())
		count, err = result.RowsAffected()
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(int64(1)))
		config := ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: postgres.ConnectionURL}, Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}}
		store, err = storageadapter.NewAdapter(&config)
		Expect(err).NotTo(HaveOccurred())
		permissions := fixtures.ClientIdentityPermissionSet(serviceID, "Reports rollout")
		permissions.ServiceScopes = append(permissions.ServiceScopes, domainstorage.ServiceScope{ServiceID: filesystemID, Scopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeOptional})
		Expect(store.PermissionSets().Create(ctx, permissions)).To(Succeed())
		agent = fixtures.ClientIdentityRolloutAgent(serviceID, permissions.ID)
		agent.ServiceRequirements = append(agent.ServiceRequirements, domainstorage.ServiceRequirement{ServiceID: filesystemID, RequiredScopes: []string{"profile"}, RequirementType: domainstorage.RequirementTypeOptional})
		Expect(store.Agents().Create(ctx, agent)).To(Succeed())
		for _, principal := range []string{alice, bob} {
			grant := fixtures.ActiveGrant(principal, agent.ID.String(), serviceID.String(), []string{"profile"})
			grant.GrantedPermissionSets[0].PermissionSetID = permissions.ID
			grant.GrantedPermissionSets[0].IncludedServiceIDs = []id.ServiceID{serviceID, filesystemID}
			Expect(store.UserGrants().Create(ctx, grant)).To(Succeed())
		}
	})

	AfterEach(func() {
		if restarted != nil {
			restarted.Close()
		} else if current != nil {
			current.Close()
		}
		for _, value := range prior {
			Expect(value.Container.Terminate(ctx)).To(Succeed())
		}
		if store != nil {
			Expect(store.Close(ctx)).To(Succeed())
		}
		if db != nil {
			Expect(db.Close()).To(Succeed())
		}
		if postgres != nil {
			Expect(postgres.Close(ctx)).To(Succeed())
		}
		if journey != nil {
			Expect(journey.Close()).To(Succeed())
		}
	})

	exchange := func(broker *bootstrap.CredentialServer, principal string, target id.ServiceID) rolloutHTTPResult {
		form, err := helpers.ClientIdentityExchangeForm(journey, agent.ID, principal)
		if err != nil {
			return rolloutHTTPResult{err: err}
		}
		if target == filesystemID {
			form.Set("resource", journey.Upstream.URL()+"/filesystem-resource")
		}
		return rolloutHTTP(broker, principal, http.MethodPost, "/oauth2/token", strings.NewReader(form.Encode()))
	}

	expectToken := func(broker *bootstrap.CredentialServer, principal string, target id.ServiceID) {
		GinkgoHelper()
		body := object(exchange(broker, principal, target), http.StatusOK)
		Expect(body).To(HaveKeyWithValue("access_token", "mock-access-token"))
	}

	expectFrozen := func() {
		GinkgoHelper()
		var clientID, source, canonical string
		Expect(db.QueryRowContext(ctx, "SELECT client_id, credential_source, canonical_id FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&clientID, &source, &canonical)).To(Succeed())
		Expect([]string{clientID, source, canonical}).To(Equal([]string{fixtures.ReportsClientV1, "stored", "reports-prod"}))
		Expect(db.QueryRowContext(ctx, "SELECT credential_source, canonical_id FROM thirdparty_oauth2_services WHERE id=$1", filesystemID).Scan(&source, &canonical)).To(Succeed())
		Expect([]string{source, canonical}).To(Equal([]string{"filesystem", "reports-files"}))
		fileID, err := os.ReadFile(binding["client_id_file"])
		Expect(err).NotTo(HaveOccurred())
		Expect(string(fileID)).To(Equal(fixtures.ReportsClientV1))
	}

	expectZeroEvidence := func() {
		GinkgoHelper()
		var increments, inferred int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM thirdparty_oauth2_services WHERE client_identity_revision<>0").Scan(&increments)).To(Succeed())
		Expect(increments).To(BeZero())
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM user_sessions WHERE service_id=$1 AND (upstream_client_id IS NOT NULL OR upstream_client_revision<>0)", serviceID).Scan(&inferred)).To(Succeed())
		Expect(inferred).To(BeZero(), "rollout and refresh must not invent legacy history")
	}

	metadata := func(broker *bootstrap.CredentialServer, name string) rolloutHTTPResult {
		request := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
		delete(request, "protected_resources")
		request["display_name"], request["client_secret"] = name, storedSecret
		response, err := broker.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), request)
		if err != nil {
			return rolloutHTTPResult{err: err}
		}
		defer func() { _ = response.Body.Close() }()
		body, err := io.ReadAll(response.Body)
		return rolloutHTTPResult{status: response.StatusCode, body: body, err: err}
	}

	traffic := func(brokers ...*bootstrap.CredentialServer) {
		GinkgoHelper()
		type operation struct {
			name   string
			status int
			run    func() rolloutHTTPResult
		}
		var operations []operation
		for i, broker := range brokers {
			principal := alice
			if i%2 != 0 {
				principal = bob
			}
			for _, target := range []id.ServiceID{serviceID, filesystemID} {
				operations = append(operations, operation{"token", http.StatusOK, func() rolloutHTTPResult { return exchange(broker, principal, target) }})
			}
			operations = append(operations,
				operation{"refresh", http.StatusOK, func() rolloutHTTPResult {
					return rolloutHTTP(broker, alice, http.MethodPost, "/api/third-party/"+serviceID.String()+"/session/refresh", nil)
				}},
				operation{"sessions", http.StatusOK, func() rolloutHTTPResult {
					return rolloutHTTP(broker, principal, http.MethodGet, "/api/third-party/sessions", nil)
				}},
				operation{"browser", http.StatusFound, func() rolloutHTTPResult {
					query := url.Values{"client_id": {agent.ID.String()}, "response_type": {"code"}, "redirect_uri": {agent.RedirectURIs[0]}, "scope": {"openid profile"}, "state": {"rollout-browser"}, "code_challenge": {helpers.GenerateCodeChallenge(helpers.PKCEVerifier())}, "code_challenge_method": {"S256"}}
					return rolloutHTTP(broker, principal, http.MethodGet, "/oauth2/authorize?"+query.Encode(), nil)
				}},
			)
		}
		results, start := make([]rolloutHTTPResult, len(operations)), make(chan struct{})
		var workers sync.WaitGroup
		for i, operation := range operations {
			workers.Add(1)
			go func() { defer workers.Done(); <-start; results[i] = operation.run() }()
		}
		close(start)
		workers.Wait()
		for i, operation := range operations {
			result := results[i]
			Expect(result.err).NotTo(HaveOccurred(), operation.name)
			Expect(result.status).To(Equal(operation.status), "%s: %s", operation.name, result.body)
			switch operation.name {
			case "token":
				Expect(object(result, http.StatusOK)).To(HaveKeyWithValue("access_token", "mock-access-token"))
			case "browser":
				location, err := url.Parse(result.header.Get("Location"))
				Expect(err).NotTo(HaveOccurred())
				Expect(location.Query().Get("code")).NotTo(BeEmpty())
				Expect(location.Query().Get("state")).To(Equal("rollout-browser"))
			case "sessions":
				data := object(result, http.StatusOK)["data"].(map[string]any)
				Expect(data["sessions"].([]any)).To(HaveLen(2))
				for _, raw := range data["sessions"].([]any) {
					summary := raw.(map[string]any)
					Expect(summary["requires_reauthentication"]).To(BeFalse())
					Expect(summary["is_expired"]).To(Equal(summary["requires_reauthentication"]))
				}
			case "refresh":
				Expect(object(result, http.StatusOK)["data"].(map[string]any)["requires_reauthentication"]).To(BeFalse())
			}
		}
		// Serialize accepted metadata writes; optimistic conflicts are not rollout failures.
		for _, broker := range brokers {
			Expect(object(metadata(broker, "Reports rolling update"), http.StatusOK)).To(HaveKeyWithValue("client_id", fixtures.ReportsClientV1))
		}
		expectFrozen()
	}

	expand := func() {
		GinkgoHelper()
		Expect(postgres.MigrateTo(50)).To(Succeed())
		version, dirty, err := postgres.MigrationVersion()
		Expect(err).NotTo(HaveOccurred())
		Expect(version).To(Equal(uint(50)))
		Expect(dirty).To(BeFalse())
		var legacy, sessions int
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM thirdparty_oauth2_services WHERE client_identity IS NULL AND client_identity_revision=0").Scan(&legacy)).To(Succeed())
		Expect(legacy).To(Equal(2), "schema expansion must not backfill a historical identity")
		Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM user_sessions WHERE upstream_client_revision=0").Scan(&sessions)).To(Succeed())
		Expect(sessions).To(Equal(4))
	}

	expectDenied := func(principal string) {
		GinkgoHelper()
		result := exchange(current, principal, serviceID)
		body := object(result, http.StatusBadRequest)
		Expect(body["error"]).To(Equal("invalid_grant"))
		Expect(body).NotTo(HaveKey("access_token"))
		Expect(body).NotTo(HaveKey("refresh_token"))
		Expect(result.header.Get("WWW-Authenticate")).To(BeEmpty())
		Expect(body["error_uri"]).To(Equal(current.EndUserURL + "/api/third-party/" + serviceID.String() + "/oauth2/authorize?redirect_uri=" + url.QueryEscape(current.EndUserURL+"/sessions")))
		before := journey.Upstream.TokenGrantCount("refresh_token")
		refresh := object(rolloutHTTP(current, principal, http.MethodPost, "/api/third-party/"+serviceID.String()+"/session/refresh", nil), http.StatusConflict)
		Expect(refresh["error"]).To(Equal("refresh_unavailable"))
		Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(before))
		data := object(rolloutHTTP(current, principal, http.MethodGet, "/api/third-party/"+serviceID.String()+"/session", nil), http.StatusOK)["data"].(map[string]any)
		summary := data["session"].(map[string]any)
		Expect(summary["requires_reauthentication"]).To(BeTrue())
		Expect(summary["is_expired"]).To(BeTrue())
		Expect(summary["access_token_expired"]).To(BeFalse())
	}

	change := func(clientID string, revision int64) {
		GinkgoHelper()
		request := fixtures.ClientIdentityServiceRequest(journey.Upstream.URL(), "stored")
		delete(request, "protected_resources")
		request["client_id"], request["client_secret"] = clientID, storedSecret
		response, err := current.AdminJSON(http.MethodPut, "/api/services/"+serviceID.String(), request)
		Expect(err).NotTo(HaveOccurred())
		body, err := helpers.CredentialResponseObject(response)
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK), "%v", body)
		var actual int64
		Expect(db.QueryRowContext(ctx, "SELECT client_identity_revision FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&actual)).To(Succeed())
		Expect(actual).To(Equal(revision))
	}

	Context("when the published preceding release shares the online-expanded schema", func() {
		// Supplemental AS-1.6 from specs/056-invalidate-client-sessions/spec.md (User Story 1).
		It("should retain concurrent old/new traffic, drain every old process, and preserve invalidation across restart and rollback", func(_ SpecContext) {
			By("keeping ingress active while additive migrations expand the old release's database")
			traffic(prior[0].CredentialServer)
			expand()
			startCurrent()
			second := startPrior()
			traffic(prior[0].CredentialServer, current, second.CredentialServer)
			expectZeroEvidence()

			By("rejecting genuine old callback state on new handlers before exchange, while new additive state is understood by old handlers before activation")
			legacy := callback(prior[0].CredentialServer, alice, serviceID)
			legacyClaims, err := helpers.ClientIdentityCallbackClaims(journey, legacy)
			Expect(err).NotTo(HaveOccurred())
			Expect(legacyClaims).NotTo(HaveKey("upstream_client_revision"))
			before := journey.Upstream.TokenGrantCount("authorization_code")
			rejected := rolloutHTTP(current, alice, http.MethodGet, legacy.RequestURI(), nil)
			Expect(rejected.err).NotTo(HaveOccurred())
			Expect(rejected.status).To(Equal(http.StatusFound))
			location, err := url.Parse(rejected.header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(location.Path).To(Equal("/sessions"))
			Expect(location.Query().Get("error")).To(Equal("callback_failed"))
			Expect(journey.Upstream.TokenGrantCount("authorization_code")).To(Equal(before))
			freshPrincipal := fixtures.AdminPrincipal().String()
			additive := callback(current, freshPrincipal, serviceID)
			claims, err := helpers.ClientIdentityCallbackClaims(journey, additive)
			Expect(err).NotTo(HaveOccurred())
			Expect(claims).To(HaveKeyWithValue("upstream_client_id", fixtures.ReportsClientV1))
			Expect(claims).To(HaveKeyWithValue("upstream_client_revision", float64(0)))
			accepted := rolloutHTTP(second.CredentialServer, freshPrincipal, http.MethodGet, additive.RequestURI(), nil)
			Expect(accepted.err).NotTo(HaveOccurred())
			Expect(accepted.status).To(Equal(http.StatusFound))
			location, err = url.Parse(accepted.header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(location.Query().Get("success")).To(Equal("true"))
			Expect(journey.Upstream.TokenGrantCount("authorization_code")).To(Equal(before + 1))
			var defaultRevision int64
			Expect(db.QueryRowContext(ctx, "SELECT upstream_client_revision FROM user_sessions WHERE principal=$1 AND service_id=$2", freshPrincipal, serviceID).Scan(&defaultRevision)).To(Succeed())
			Expect(defaultRevision).To(BeZero(), "the actual old INSERT omits the additive revision column")

			By("allowing same-ID file and stored-secret rotation plus metadata writes without losing unknown evidence columns")
			Expect(helpers.PublishCredentialPair(binding, fixtures.ReportsClientV1, fixtures.ReportsSecretV2)).To(Succeed())
			storedSecret = fixtures.ReportsSecretV2
			object(metadata(current, "Reports rotated"), http.StatusOK)
			var identity string
			var revision int64
			Expect(db.QueryRowContext(ctx, "SELECT client_identity, client_identity_revision FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&identity, &revision)).To(Succeed())
			Expect(identity).To(Equal(fixtures.ReportsClientV1))
			Expect(revision).To(BeZero())
			refreshes, requestsBefore := journey.Upstream.TokenGrantCount("refresh_token"), len(journey.Upstream.GetTokenRequests())
			traffic(prior[0].CredentialServer, current, second.CredentialServer)
			Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(refreshes + 3))
			for _, request := range journey.Upstream.GetTokenRequests()[requestsBefore:] {
				form, err := url.ParseQuery(request.Body)
				Expect(err).NotTo(HaveOccurred())
				if form.Get("grant_type") != "refresh_token" {
					continue
				}
				clientID, secret, err := helpers.CredentialTokenRequest(request)
				Expect(err).NotTo(HaveOccurred())
				Expect([]string{clientID, secret}).To(Equal([]string{fixtures.ReportsClientV1, fixtures.ReportsSecretV2}))
			}
			for _, broker := range []*bootstrap.CredentialServer{prior[0].CredentialServer, current} {
				before := journey.Upstream.TokenGrantCount("refresh_token")
				object(rolloutHTTP(broker, alice, http.MethodPost, "/api/third-party/"+filesystemID.String()+"/session/refresh", nil), http.StatusOK)
				Expect(journey.Upstream.TokenGrantCount("refresh_token")).To(Equal(before + 1))
				requests := journey.Upstream.GetTokenRequests()
				clientID, secret, err := helpers.CredentialTokenRequest(requests[len(requests)-1])
				Expect(err).NotTo(HaveOccurred())
				Expect([]string{clientID, secret}).To(Equal([]string{fixtures.ReportsClientV1, fixtures.ReportsSecretV2}))
			}
			var preservedIdentity string
			Expect(db.QueryRowContext(ctx, "SELECT client_identity, client_identity_revision FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&preservedIdentity, &defaultRevision)).To(Succeed())
			Expect(preservedIdentity).To(Equal(identity))
			Expect(defaultRevision).To(Equal(revision))
			var inferred int
			Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM user_sessions WHERE service_id=$1 AND principal=ANY($2::text[]) AND upstream_client_id IS NOT NULL", serviceID, []string{alice, bob}).Scan(&inferred)).To(Succeed())
			Expect(inferred).To(BeZero())

			By("draining admitted old refresh work individually while healthy replicas still serve")
			barrier := journey.Upstream.GateTokenGrant("refresh_token")
			DeferCleanup(barrier.Release)
			refreshDone := make(chan rolloutHTTPResult, 1)
			go func() {
				refreshDone <- rolloutHTTP(prior[0].CredentialServer, alice, http.MethodPost, "/api/third-party/"+serviceID.String()+"/session/refresh", nil)
			}()
			Eventually(barrier.Entered, 5*time.Second).Should(BeClosed())
			stopDone := make(chan error, 1)
			stopTimeout := 10 * time.Second
			go func() { stopDone <- prior[0].Container.Stop(ctx, &stopTimeout) }()
			expectToken(current, bob, serviceID)
			expectToken(second.CredentialServer, bob, filesystemID)
			expectFrozen()
			barrier.Release()
			var completed rolloutHTTPResult
			Eventually(refreshDone, 5*time.Second).Should(Receive(&completed))
			object(completed, http.StatusOK)
			Eventually(stopDone, 5*time.Second).Should(Receive(BeNil()))
			Expect(prior[0].Container.IsRunning()).To(BeFalse())
			Expect(prior[0].Container.Terminate(ctx)).To(Succeed())
			traffic(current, second.CredentialServer)
			Expect(second.Container.Stop(ctx, &stopTimeout)).To(Succeed())
			Expect(second.Container.IsRunning()).To(BeFalse())
			Expect(second.Container.Terminate(ctx)).To(Succeed())
			prior = nil // Removed containers cannot be restarted by a deployment restart policy.
			expectToken(current, alice, serviceID)

			By("resuming identity changes only after the complete old serving/writing inventory is removed")
			change(fixtures.ReportsClientV2, 1)
			beforeLateCallback := journey.Upstream.TokenGrantCount("authorization_code")
			lateCallback := rolloutHTTP(current, alice, http.MethodGet, legacy.RequestURI(), nil)
			Expect(lateCallback.err).NotTo(HaveOccurred())
			Expect(lateCallback.status).To(Equal(http.StatusFound))
			lateLocation, err := url.Parse(lateCallback.header.Get("Location"))
			Expect(err).NotTo(HaveOccurred())
			Expect(lateLocation.Path).To(Equal("/sessions"))
			Expect(lateLocation.Query().Get("error")).To(Equal("callback_failed"))
			Expect(journey.Upstream.TokenGrantCount("authorization_code")).To(Equal(beforeLateCallback))
			for _, principal := range []string{alice, bob} {
				expectDenied(principal)
			}
			current.Close()
			restarted, err = bootstrap.StartCredentialProcess(bootstrap.CredentialProcessOptions{Directory: journey.Directory, Document: document, Principal: alice})
			Expect(err).NotTo(HaveOccurred())
			current = restarted.CredentialServer
			for _, principal := range []string{alice, bob} {
				expectDenied(principal)
			}
			change(fixtures.ReportsClientV1, 2)
			for _, principal := range []string{alice, bob} {
				expectDenied(principal)
			}
			connect(current, alice, serviceID)
			expectToken(current, alice, serviceID)
			expectDenied(bob)
			var established string
			Expect(db.QueryRowContext(ctx, "SELECT upstream_client_id, upstream_client_revision FROM user_sessions WHERE principal=$1 AND service_id=$2", alice, serviceID).Scan(&established, &revision)).To(Succeed())
			Expect(established).To(Equal(fixtures.ReportsClientV1))
			Expect(revision).To(Equal(int64(2)))
		}, SpecTimeout(4*time.Minute))
	})

	Context("when online validation fails before a new replica may serve", func() {
		// Supplemental AS-1.6 from specs/056-invalidate-client-sessions/spec.md (User Story 1).
		It("should keep healthy prior-release traffic and the identity freeze without forcing past a failed migration", func(_ SpecContext) {
			Expect(postgres.MigrateTo(49)).To(Succeed())
			transaction, err := db.BeginTx(ctx, nil)
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(func() { _ = transaction.Rollback() })
			_, err = transaction.ExecContext(ctx, "LOCK TABLE user_sessions IN SHARE UPDATE EXCLUSIVE MODE")
			Expect(err).NotTo(HaveOccurred())
			Expect(postgres.MigrateTo(50)).NotTo(Succeed(), "NOWAIT validation must abort rather than suspend healthy traffic")
			version, dirty, err := postgres.MigrationVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(uint(50)))
			Expect(dirty).To(BeTrue(), "the failed migration must remain visible, not force-cleared")
			traffic(prior[0].CredentialServer)
			expectZeroEvidence()
			Expect(transaction.Rollback()).To(Succeed())
			connect(prior[0].CredentialServer, fixtures.AdminPrincipal().String(), filesystemID)
			var constraints int
			Expect(db.QueryRowContext(ctx, "SELECT count(*) FROM pg_constraint WHERE conname IN ('chk_thirdparty_oauth2_services_client_identity_revision','chk_user_sessions_upstream_client_revision') AND NOT convalidated").Scan(&constraints)).To(Succeed())
			Expect(constraints).To(Equal(2))
			expectFrozen()
		}, SpecTimeout(3*time.Minute))
	})

	Context("when deployment is aborted before operational activation", func() {
		// Supplemental AS-1.6 from specs/056-invalidate-client-sessions/spec.md (User Story 1).
		It("should drain new work and retain the expanded schema and zero-increment evidence while returning traffic to the actual prior release", func(_ SpecContext) {
			expand()
			startCurrent()
			traffic(prior[0].CredentialServer, current)
			expectZeroEvidence()
			var ciphertext string
			Expect(db.QueryRowContext(ctx, "SELECT encode(encrypted_access_token,'hex') FROM user_sessions WHERE principal=$1 AND service_id=$2", bob, serviceID).Scan(&ciphertext)).To(Succeed())
			var identity string
			Expect(db.QueryRowContext(ctx, "SELECT client_identity FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&identity)).To(Succeed())
			Expect(identity).To(Equal(fixtures.ReportsClientV1))
			barrier := journey.Upstream.GateTokenGrant("refresh_token")
			DeferCleanup(barrier.Release)
			refreshDone := make(chan rolloutHTTPResult, 1)
			go func() {
				refreshDone <- rolloutHTTP(current, alice, http.MethodPost, "/api/third-party/"+serviceID.String()+"/session/refresh", nil)
			}()
			Eventually(barrier.Entered, 5*time.Second).Should(BeClosed())
			closed := make(chan struct{})
			go func() { current.Close(); close(closed) }()
			expectToken(prior[0].CredentialServer, bob, serviceID)
			object(metadata(prior[0].CredentialServer, "Reports abort retained"), http.StatusOK)
			expectFrozen()
			barrier.Release()
			var completed rolloutHTTPResult
			Eventually(refreshDone, 5*time.Second).Should(Receive(&completed))
			object(completed, http.StatusOK)
			Eventually(closed, 5*time.Second).Should(BeClosed())
			current = nil
			traffic(prior[0].CredentialServer)
			expectZeroEvidence()
			version, dirty, err := postgres.MigrationVersion()
			Expect(err).NotTo(HaveOccurred())
			Expect(version).To(Equal(uint(50)))
			Expect(dirty).To(BeFalse())
			var retained string
			Expect(db.QueryRowContext(ctx, "SELECT encode(encrypted_access_token,'hex') FROM user_sessions WHERE principal=$1 AND service_id=$2", bob, serviceID).Scan(&retained)).To(Succeed())
			Expect(retained).To(Equal(ciphertext), "abort must retain evidence and existing session ciphertext")
			var retainedIdentity string
			Expect(db.QueryRowContext(ctx, "SELECT client_identity FROM thirdparty_oauth2_services WHERE id=$1", serviceID).Scan(&retainedIdentity)).To(Succeed())
			Expect(retainedIdentity).To(Equal(identity))
			expectFrozen()
		}, SpecTimeout(3*time.Minute))
	})
})
