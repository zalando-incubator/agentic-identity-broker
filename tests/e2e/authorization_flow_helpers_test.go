package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"

	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func completeAuthorizationCodeFlow(
	server *bootstrap.TestServer,
	testStorage *storageadapter.Adapter,
	principal string,
	clientID string,
	redirectURI string,
	assertConsentContext func(data map[string]any),
) {
	Expect(fixtures.SeedDefaultConsentData(context.Background(), testStorage, id.Principal(principal))).To(Succeed())
	verifier := helpers.PKCEVerifier()
	state := "portless-flow-state"
	challenge := helpers.GenerateCodeChallenge(verifier)

	authorizePath := "/oauth2/authorize?" + url.Values{
		"client_id":             {clientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"state":                 {state},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
	}.Encode()

	authResp, err := server.AuthenticatedGET(authorizePath, principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = authResp.Body.Close() }()
	Expect(authResp.StatusCode).To(Equal(http.StatusFound))

	consentLoc, err := helpers.ExtractRedirectURL(authResp)
	Expect(err).ToNot(HaveOccurred())
	Expect(consentLoc.Path).To(ContainSubstring("/agents/"))
	Expect(consentLoc.Query().Get("redirect_uri")).To(BeEmpty())

	resolvedAgentID, err := id.ParseAgentID(path.Base(consentLoc.Path))
	Expect(err).ToNot(HaveOccurred(), "consent redirect must include a valid agent ID")

	sessionToken := consentLoc.Query().Get("session_token")
	Expect(sessionToken).ToNot(BeEmpty())

	consentViewResp, err := server.AuthenticatedGET(consentLoc.RequestURI(), principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = consentViewResp.Body.Close() }()
	Expect(consentViewResp.StatusCode).To(Equal(http.StatusOK))
	Expect(consentViewResp.Header.Get("Content-Type")).To(HavePrefix("text/html"))

	consentResp, err := server.AuthenticatedGET(
		fmt.Sprintf("/api/consent/agents/%s?session_token=%s", resolvedAgentID, url.QueryEscape(sessionToken)),
		principal,
	)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = consentResp.Body.Close() }()
	Expect(consentResp.StatusCode).To(Equal(http.StatusOK))

	var consentBody map[string]any
	Expect(json.NewDecoder(consentResp.Body).Decode(&consentBody)).To(Succeed())
	data, ok := consentBody["data"].(map[string]any)
	Expect(ok).To(BeTrue(), "consent detail response must include data")
	if assertConsentContext != nil {
		assertConsentContext(data)
	}

	grantBody, _ := json.Marshal(map[string]any{
		"granted_permission_sets": map[string][]string{
			fixtures.PlaceholderPermissionSetID.String(): {fixtures.PlaceholderServiceID.String()},
		},
	})
	grantResp, err := server.AuthenticatedPOST(
		fmt.Sprintf("/api/consent/agents/%s/grants?session_token=%s", resolvedAgentID, url.QueryEscape(sessionToken)),
		principal,
		"application/json",
		bytes.NewReader(grantBody),
	)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = grantResp.Body.Close() }()
	Expect(grantResp.StatusCode).To(Equal(http.StatusCreated))

	var grantBodyResp map[string]any
	Expect(json.NewDecoder(grantResp.Body).Decode(&grantBodyResp)).To(Succeed())
	resumeURL, ok := grantBodyResp["redirect_url"].(string)
	Expect(ok).To(BeTrue(), "grant response must include redirect_url")
	Expect(resumeURL).To(ContainSubstring("/oauth2/authorize"))

	parsedResumeURL, err := url.Parse(resumeURL)
	Expect(err).ToNot(HaveOccurred())
	resumeResp, err := server.AuthenticatedGET(parsedResumeURL.RequestURI(), principal)
	Expect(err).ToNot(HaveOccurred())
	defer func() { _ = resumeResp.Body.Close() }()
	Expect(resumeResp.StatusCode).To(Equal(http.StatusFound))

	finalRedirect, err := helpers.ExtractRedirectURL(resumeResp)
	Expect(err).ToNot(HaveOccurred())
	Expect(finalRedirect.String()).To(HavePrefix(redirectURI))
	Expect(finalRedirect.Query().Get("code")).ToNot(BeEmpty(), "authorization code must be issued after consent")
	Expect(finalRedirect.Query().Get("state")).To(Equal(state))
}
