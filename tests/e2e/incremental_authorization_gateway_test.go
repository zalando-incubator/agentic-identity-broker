package e2e_test

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"time"

	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	extprochelpers "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/extproc/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const incrementalResourceMetadataURL = "https://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization Gateway Challenge", func() {
	var (
		journey      *incrementalJourney
		grpcClient   extprocv3.ExternalProcessorClient
		subjectToken string
	)

	BeforeEach(func() {
		journey = newIncrementalJourney("local", nil)
		journey.usableGitHub()
		subjectToken = journey.issue("openid offline_access")["access_token"].(string)
		journey.putSession(fixtures.ExpiredAccessWithRefresh(journey.principal, journey.cast.GitHub.ID.String(), []string{"repo", "user:email"}))
		journey.provider.WithRefreshGrantError("invalid_grant")

		policyPath, err := filepath.Abs("fixtures/policies/approval_journey.rego")
		Expect(err).NotTo(HaveOccurred())
		environment, err := bootstrap.NewExtProcEnvironment(journey.enduser.BaseURL(), journey.tokens.ClientAssertion, policyPath, bootstrap.TestLogger(0))
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(environment.Close)
		client, connection := extprochelpers.ConnectToExtProc(environment.Address)
		DeferCleanup(func() { _ = connection.Close() })
		grpcClient = client
	})

	toolCall := func(metadata *structpb.Value) (*extprocv3.ProcessingResponse, *extprocv3.ProcessingResponse) {
		builder := extprochelpers.NewRequestHeaders().
			WithHeader(":method", http.MethodPost).
			WithPath("/mcp").
			WithBearerToken(subjectToken).
			WithTokenExchangeMetadata(subjectToken, "https://api.github.com").
			WithHeader("Mcp-Session-Id", "incremental-session").
			WithAgentgatewayProtocol("mcp")
		if metadata != nil {
			builder = builder.WithTokenExchangeMetadataField("resource_metadata_url", metadata)
		}
		body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"list_repositories","arguments":{}}}`)
		return extprochelpers.SendHeadersAndBody(context.Background(), grpcClient, builder.BuildWithMetadata(), body)
	}

	// brokerRecovery returns the broker's recoverable exchange error for comparison.
	brokerRecovery := func() map[string]any {
		status, body := journey.exchange(subjectToken)
		Expect(status).To(Equal(http.StatusBadRequest))
		Expect(body["error"]).To(Equal("invalid_grant"))
		Expect(body["scope"]).To(Equal(journey.githubScope()))
		recovery, err := url.Parse(body["error_uri"].(string))
		Expect(err).NotTo(HaveOccurred())
		Expect(recovery.Path).To(Equal("/api/third-party/" + journey.cast.GitHub.ID.String() + "/oauth2/authorize"))
		Expect(recovery.Query().Get("redirect_uri")).To(Equal(journey.githubScope()))
		return body
	}

	expectedChallenge := func(recovery map[string]any, metadata string) string {
		challenge := fmt.Sprintf(`Bearer error="insufficient_scope", error_description="%s", error_uri="%s", scope="%s"`,
			recovery["error_description"], recovery["error_uri"], recovery["scope"])
		if metadata != "" {
			challenge += fmt.Sprintf(`, resource_metadata="%s"`, metadata)
		}
		return challenge
	}

	expectChallenge := func(response *extprocv3.ProcessingResponse, recovery map[string]any, metadata string) {
		Expect(extprochelpers.ExtractImmediateResponseStatus(response)).To(Equal(uint32(http.StatusForbidden)))
		Expect(extprochelpers.ExtractImmediateResponseHeader(response, "WWW-Authenticate")).To(Equal(expectedChallenge(recovery, metadata)))
		extprochelpers.ExpectReauthElicitationBody([]byte(extprochelpers.ExtractImmediateResponseBody(response)), recovery["error_uri"].(string), recovery["error_description"].(string))
	}

	// US1 S1 from specs/055-incremental-service-authorization/spec.md.
	It("returns the service scope and recovery URL and the gateway challenges with insufficient_scope while keeping URL elicitation", func() {
		first, _ := toolCall(structpb.NewStringValue(incrementalResourceMetadataURL))
		Expect(journey.provider.GetTokenRequestsForGrant("refresh_token")).To(HaveLen(1))
		session, err := journey.store.UserSessions().FindByPrincipalAndService(context.Background(), id.Principal(journey.principal), journey.cast.GitHub.ID)
		Expect(err).NotTo(HaveOccurred())
		Expect(session.IsExpired()).To(BeTrue(), "the definitive provider rejection must be persisted")
		Expect(session.RefreshTokenExpiresAt).NotTo(BeNil())
		Expect(session.RefreshTokenExpiresAt.Before(time.Now().Add(time.Second))).To(BeTrue())

		recovery := brokerRecovery()
		Expect(journey.provider.GetTokenRequestsForGrant("refresh_token")).To(HaveLen(1), "persisted unusability needs no second provider refresh")
		expectChallenge(first, recovery, incrementalResourceMetadataURL)

		journey.provider.WithRefreshGrantError("")
		recoveryURL, err := url.Parse(recovery["error_uri"].(string))
		Expect(err).NotTo(HaveOccurred())
		returned := journey.follow(recoveryURL.RequestURI())
		returned = journey.followCallback(returned)
		Expect(returned.Path).To(Equal("/sessions/github"))
		retried, bodyResponse := toolCall(structpb.NewStringValue(incrementalResourceMetadataURL))
		Expect(extprochelpers.ExtractMutatedAuthorizationHeader(retried)).To(Equal("Bearer synthetic-github-access"))
		Expect(bodyResponse).To(extprochelpers.BeForwardedRequestBody())
	})

	// US4 S1 from specs/055-incremental-service-authorization/spec.md.
	It("includes resource_metadata only when the route configures it, keeping scope and recovery link otherwise", func() {
		withMetadata, _ := toolCall(structpb.NewStringValue(incrementalResourceMetadataURL))
		recovery := brokerRecovery()
		expectChallenge(withMetadata, recovery, incrementalResourceMetadataURL)

		withoutMetadata, _ := toolCall(nil)
		expectChallenge(withoutMetadata, recovery, "")

		for _, invalid := range []*structpb.Value{
			structpb.NewNumberValue(42),
			structpb.NewStringValue("http://agentgateway.example.com/.well-known/oauth-protected-resource/mcp"),
			structpb.NewStringValue("https://user@agentgateway.example.com/metadata"),
			structpb.NewStringValue("https://agentgateway.example.com/metadata#fragment"),
			structpb.NewStringValue(`https://agentgateway.example.com/meta"data`),
			structpb.NewStringValue("https://agentgateway.example.com/" + string(make([]byte, 2048))),
		} {
			response, _ := toolCall(invalid)
			expectChallenge(response, recovery, "")
		}
	})
})

// followCallback completes the mock provider sign-in that a recovery link started.
func (j *incrementalJourney) followCallback(providerAuthorization *url.URL) *url.URL {
	callback, err := helpers.ProviderAuthorizationCallback(providerAuthorization.String())
	Expect(err).NotTo(HaveOccurred())
	return j.follow(callback.RequestURI())
}
