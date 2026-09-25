package e2e_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/lestrrat-go/jwx/v4/jws"
	"github.com/lestrrat-go/jwx/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

var _ = Describe("US5: Signing Key Management (local mode)", func() {
	var (
		adminServer    *bootstrap.TestServer
		enduserServer  *bootstrap.TestServer
		storageFactory *bootstrap.StorageFactory
		testStorage    *storageadapter.Adapter
		logger         *slog.Logger
	)

	createClientCredentials := func(agentID string) string {
		resp, err := helpers.HTTPClient().Post(
			adminServer.BaseURL()+"/api/agents/"+agentID+"/client-credentials",
			"application/json",
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		clientSecret, ok := body["client_secret"].(string)
		Expect(ok).To(BeTrue())
		Expect(clientSecret).ToNot(BeEmpty())
		return clientSecret
	}

	issueAccessToken := func(agentID, clientSecret string) string {
		form := url.Values{
			"grant_type":    {"client_credentials"},
			"client_id":     {agentID},
			"client_secret": {clientSecret},
		}
		resp, err := enduserServer.PublicPOST(
			"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(form.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		accessToken, ok := body["access_token"].(string)
		Expect(ok).To(BeTrue())
		Expect(accessToken).ToNot(BeEmpty())
		return accessToken
	}

	addSigningKey := func() string {
		resp, err := adminServer.AuthenticatedPOST(
			"/api/oauth2-server/signing-keys",
			fixtures.AdminPrincipal().String(),
			"application/json",
			strings.NewReader(`{"algorithm":"ES256"}`),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		kid, ok := body["kid"].(string)
		Expect(ok).To(BeTrue())
		Expect(kid).ToNot(BeEmpty())
		return kid
	}

	promoteSigningKey := func(kid string) {
		resp, err := adminServer.DirectRequest(
			http.MethodPut,
			"/api/oauth2-server/signing-keys/"+kid+"/current",
			fixtures.AdminPrincipal().String(),
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
	}

	fetchJWKS := func() jwk.Set {
		resp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		set, err := jwk.ParseReader(resp.Body)
		Expect(err).ToNot(HaveOccurred())
		return set
	}

	tokenKID := func(accessToken string) string {
		msg, err := jws.Parse([]byte(accessToken))
		Expect(err).ToNot(HaveOccurred())
		Expect(msg.Signatures()).To(HaveLen(1))
		kid, ok := msg.Signatures()[0].ProtectedHeaders().KeyID()
		Expect(ok).To(BeTrue())
		Expect(kid).ToNot(BeEmpty())
		return kid
	}

	validateTokenWithJWKS := func(accessToken string, keySet jwk.Set) {
		_, err := jwt.Parse(
			[]byte(accessToken),
			jwt.WithVerify(true),
			jwt.WithKeySet(keySet),
			jwt.WithValidate(false),
		)
		Expect(err).ToNot(HaveOccurred())
	}

	BeforeEach(func() {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
		config := fixtures.LocalConfig()
		storageFactory = bootstrap.NewStorageFactory(logger)
		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())

		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		Expect(err).ToNot(HaveOccurred())
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
		}
		if enduserServer != nil {
			enduserServer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// Scenario 5.1 from specs/025-oauth2-server/spec.md
	It("adds a signing key", func() {
		resp, err := adminServer.AuthenticatedPOST(
			"/api/oauth2-server/signing-keys",
			fixtures.AdminPrincipal().String(),
			"application/json",
			strings.NewReader(`{"algorithm":"ES256"}`),
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusCreated))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("kid"))
		Expect(body["algorithm"]).To(Equal("ES256"))
		Expect(body["is_current"]).To(BeTrue())
	})

	// Scenario 5.2 from specs/025-oauth2-server/spec.md
	It("lists signing keys", func() {
		resp, err := helpers.HTTPClient().Get(adminServer.BaseURL() + "/api/oauth2-server/signing-keys")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body).To(HaveKey("items"))
	})

	// Scenario 5.3 from specs/025-oauth2-server/spec.md
	It("promotes a key to current", func() {
		// Add a second key (auto-current)
		resp, err := adminServer.AuthenticatedPOST(
			"/api/oauth2-server/signing-keys",
			fixtures.AdminPrincipal().String(),
			"application/json",
			strings.NewReader(`{"algorithm":"ES256"}`),
		)
		Expect(err).ToNot(HaveOccurred())
		var key2 map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&key2)).ToNot(HaveOccurred())
		_ = resp.Body.Close()

		// Get the auto-generated key (not current anymore)
		listResp, _ := helpers.HTTPClient().Get(adminServer.BaseURL() + "/api/oauth2-server/signing-keys")
		var listBody map[string]interface{}
		Expect(json.NewDecoder(listResp.Body).Decode(&listBody)).ToNot(HaveOccurred())
		_ = listResp.Body.Close()

		items := listBody["items"].([]interface{})
		var nonCurrentKid string
		for _, item := range items {
			m := item.(map[string]interface{})
			if !m["is_current"].(bool) {
				nonCurrentKid = m["kid"].(string)
				break
			}
		}
		Expect(nonCurrentKid).ToNot(BeEmpty())

		// Promote non-current key
		promoteResp, err := adminServer.DirectRequest(
			http.MethodPut,
			"/api/oauth2-server/signing-keys/"+nonCurrentKid+"/current",
			fixtures.AdminPrincipal().String(),
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = promoteResp.Body.Close() }()
		Expect(promoteResp.StatusCode).To(Equal(http.StatusOK))
	})

	// Scenario 5.4 from specs/025-oauth2-server/spec.md
	It("removes a non-current key and invalidates tokens signed with it", func() {
		agent := fixtures.LocalAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())
		clientSecret := createClientCredentials(agent.ID.String())

		oldToken := issueAccessToken(agent.ID.String(), clientSecret)
		oldKID := tokenKID(oldToken)

		// Add and then explicitly promote the replacement key so it is immediately usable for signing.
		newKID := addSigningKey()
		Expect(newKID).ToNot(Equal(oldKID))
		promoteSigningKey(newKID)
		Expect(tokenKID(issueAccessToken(agent.ID.String(), clientSecret))).To(Equal(newKID))

		// Find non-current key — this should be the key that signed oldToken.
		listResp, _ := helpers.HTTPClient().Get(adminServer.BaseURL() + "/api/oauth2-server/signing-keys")
		var listBody map[string]interface{}
		Expect(json.NewDecoder(listResp.Body).Decode(&listBody)).ToNot(HaveOccurred())
		_ = listResp.Body.Close()

		items := listBody["items"].([]interface{})
		var nonCurrentKid string
		for _, item := range items {
			m := item.(map[string]interface{})
			if !m["is_current"].(bool) {
				nonCurrentKid = m["kid"].(string)
				break
			}
		}
		Expect(nonCurrentKid).To(Equal(oldKID))

		// Delete non-current key.
		delResp, err := adminServer.DirectRequest(
			http.MethodDelete,
			"/api/oauth2-server/signing-keys/"+nonCurrentKid,
			fixtures.AdminPrincipal().String(),
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = delResp.Body.Close() }()
		Expect(delResp.StatusCode).To(Equal(http.StatusNoContent))

		keySet := fetchJWKS()
		_, ok := keySet.LookupKeyID(nonCurrentKid)
		Expect(ok).To(BeFalse(), "deleted signing key must disappear from JWKS")

		_, err = jwt.Parse(
			[]byte(oldToken),
			jwt.WithVerify(true),
			jwt.WithKeySet(keySet),
			jwt.WithValidate(false),
		)
		Expect(err).To(HaveOccurred(), "tokens signed with a deleted key must fail verification")
	})

	// Scenario 5.4 from specs/025-oauth2-server/spec.md
	It("returns 409 when removing the grace-period fallback key that is still signing tokens", func() {
		agent := fixtures.LocalAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())
		clientSecret := createClientCredentials(agent.ID.String())

		oldToken := issueAccessToken(agent.ID.String(), clientSecret)
		oldKID := tokenKID(oldToken)

		newKID := addSigningKey()
		Expect(newKID).ToNot(Equal(oldKID))
		Expect(tokenKID(issueAccessToken(agent.ID.String(), clientSecret))).To(Equal(oldKID))

		resp, err := adminServer.DirectRequest(
			http.MethodDelete,
			"/api/oauth2-server/signing-keys/"+oldKID,
			fixtures.AdminPrincipal().String(),
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusConflict))

		var body map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&body)).ToNot(HaveOccurred())
		Expect(body["error"]).To(Equal("current_key"))
		Expect(body["message"]).To(Equal("wait for the promoted signing key to activate or promote a different key before removing the key still signing tokens"))
	})

	// Scenario 5.5 from specs/025-oauth2-server/spec.md
	It("cannot remove last key", func() {
		// Only auto-generated key exists; try to delete it
		listResp, _ := helpers.HTTPClient().Get(adminServer.BaseURL() + "/api/oauth2-server/signing-keys")
		var listBody map[string]interface{}
		Expect(json.NewDecoder(listResp.Body).Decode(&listBody)).ToNot(HaveOccurred())
		_ = listResp.Body.Close()

		items := listBody["items"].([]interface{})
		Expect(len(items)).To(BeNumerically(">=", 1))
		kid := items[0].(map[string]interface{})["kid"].(string)

		delResp, err := adminServer.DirectRequest(
			http.MethodDelete,
			"/api/oauth2-server/signing-keys/"+kid,
			fixtures.AdminPrincipal().String(),
			nil,
			nil,
		)
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = delResp.Body.Close() }()
		Expect(delResp.StatusCode).To(Equal(http.StatusConflict))
	})

	// User Story 5 independent rotation journey from specs/025-oauth2-server/spec.md
	// Scenario 5.3 from specs/025-oauth2-server/spec.md
	It("keeps existing tokens valid while a promoted key signs new tokens", func() {
		agent := fixtures.LocalAgent()
		Expect(testStorage.Agents().Create(context.Background(), agent)).ToNot(HaveOccurred())
		clientSecret := createClientCredentials(agent.ID.String())

		firstToken := issueAccessToken(agent.ID.String(), clientSecret)
		firstKid := tokenKID(firstToken)

		secondKid := addSigningKey()
		Expect(firstKid).ToNot(Equal(secondKid))

		overlapKeySet := fetchJWKS()
		Expect(overlapKeySet.Len()).To(Equal(2))
		_, ok := overlapKeySet.LookupKeyID(firstKid)
		Expect(ok).To(BeTrue())
		_, ok = overlapKeySet.LookupKeyID(secondKid)
		Expect(ok).To(BeTrue())

		overlapToken := issueAccessToken(agent.ID.String(), clientSecret)
		Expect(tokenKID(overlapToken)).To(Equal(firstKid))
		validateTokenWithJWKS(firstToken, overlapKeySet)
		validateTokenWithJWKS(overlapToken, overlapKeySet)

		promoteSigningKey(secondKid)

		secondToken := issueAccessToken(agent.ID.String(), clientSecret)
		Expect(tokenKID(secondToken)).To(Equal(secondKid))

		keySet := fetchJWKS()
		Expect(keySet.Len()).To(Equal(2))
		_, ok = keySet.LookupKeyID(firstKid)
		Expect(ok).To(BeTrue())
		_, ok = keySet.LookupKeyID(secondKid)
		Expect(ok).To(BeTrue())

		validateTokenWithJWKS(firstToken, keySet)
		validateTokenWithJWKS(secondToken, keySet)
	})

	// Scenario 5.6 from specs/025-oauth2-server/spec.md
	It("key appears in JWKS via discovery", func() {
		// Get JWKS
		resp, err := helpers.HTTPClient().Get(enduserServer.BaseURL() + "/oauth2/jwks.json")
		Expect(err).ToNot(HaveOccurred())
		defer func() { _ = resp.Body.Close() }()
		Expect(resp.StatusCode).To(Equal(http.StatusOK))

		var jwks map[string]interface{}
		Expect(json.NewDecoder(resp.Body).Decode(&jwks)).ToNot(HaveOccurred())
		keys := jwks["keys"].([]interface{})
		Expect(len(keys)).To(BeNumerically(">=", 1))

		// Verify key has kid field
		key := keys[0].(map[string]interface{})
		Expect(key).To(HaveKey("kid"))
		Expect(key).To(HaveKey("kty"))
	})
})
