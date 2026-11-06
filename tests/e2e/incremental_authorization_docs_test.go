package e2e_test

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.yaml.in/yaml/v3"
)

// fencedBlocks returns the bodies of the guide's fenced code blocks for one language.
func fencedBlocks(document, language string) []string {
	pattern := regexp.MustCompile("(?s)```" + language + "\n(.*?)```")
	var blocks []string
	for _, match := range pattern.FindAllStringSubmatch(document, -1) {
		blocks = append(blocks, match[1])
	}
	return blocks
}

// challengeAttributes parses the auth-params of an RFC 6750 Bearer challenge.
func challengeAttributes(challenge string) map[string]string {
	attributes := map[string]string{}
	for _, match := range regexp.MustCompile(`([a-z_]+)="([^"]*)"`).FindAllStringSubmatch(challenge, -1) {
		attributes[match[1]] = match[2]
	}
	return attributes
}

// Scenarios reference specs/055-incremental-service-authorization/spec.md.
var _ = Describe("Incremental Service Authorization Operator Guide", func() {
	var journey *incrementalJourney

	BeforeEach(func() {
		journey = newIncrementalJourney("local", nil)
	})

	// US4 S4 from specs/055-incremental-service-authorization/spec.md.
	It("documents the initial challenge, metadata variant, recovery paths, bounded retries, and mode limits, and discovery omits virtual scopes", func() {
		response, err := journey.enduser.PublicGET("/.well-known/oauth-authorization-server")
		Expect(err).NotTo(HaveOccurred())
		Expect(response.StatusCode).To(Equal(http.StatusOK))
		discovery := decodeJSON[map[string]any](response)
		for _, scope := range discovery["scopes_supported"].([]any) {
			Expect(scope).NotTo(ContainSubstring("/sessions/"))
		}

		guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "guides", "incremental-authorization.md"))
		Expect(err).NotTo(HaveOccurred(), "the operator guide must exist")
		document := string(guide)

		var routeMetadata, brokerConfig bool
		for _, block := range fencedBlocks(document, "yaml") {
			var parsed map[string]any
			Expect(yaml.Unmarshal([]byte(block), &parsed)).To(Succeed(), "guide YAML must parse:\n%s", block)
			if extProc, ok := parsed["extProc"].(map[string]any); ok {
				Expect(extProc["failureMode"]).To(Equal("failClosed"))
				fields := extProc["metadataContext"].(map[string]any)["aib.tokenexchange"].(map[string]any)
				Expect(fields).To(HaveKey("subject_token"))
				Expect(fields).To(HaveKey("resource_uri"))
				literal := fields["resource_metadata_url"].(string)
				Expect(literal).To(HavePrefix("'https://"))
				Expect(literal).To(HaveSuffix("'"))
				metadataURL, err := url.Parse(strings.Trim(literal, "'"))
				Expect(err).NotTo(HaveOccurred())
				Expect(metadataURL.User).To(BeNil())
				Expect(metadataURL.Fragment).To(BeEmpty())
				routeMetadata = true
			}
			if server, ok := parsed["server"].(map[string]any); ok {
				publicURL := server["enduser"].(map[string]any)["public_url"].(string)
				Expect(parsed["oauth2_authorization_server"]).To(HaveKey("mode"))
				for _, challenge := range fencedBlocks(document, "http") {
					for _, line := range strings.Split(challenge, "\n") {
						if !strings.HasPrefix(line, "WWW-Authenticate:") {
							continue
						}
						attributes := challengeAttributes(line)
						Expect(attributes["error"]).To(Equal("insufficient_scope"))
						Expect(attributes["scope"]).To(MatchRegexp(`^` + regexp.QuoteMeta(publicURL) + `/sessions/[A-Za-z0-9._-]{1,128}$`))
						recovery, err := url.Parse(attributes["error_uri"])
						Expect(err).NotTo(HaveOccurred())
						Expect(recovery.Query().Get("redirect_uri")).To(Equal(attributes["scope"]))
					}
				}
				brokerConfig = true
			}
		}
		Expect(routeMetadata).To(BeTrue(), "the guide must show the route's resource_metadata_url")
		Expect(brokerConfig).To(BeTrue(), "the guide must show the broker public_url and mode")
		for _, anchor := range []string{"## Initial challenge", "## Protected-resource metadata", "## Recovery paths", "## Client retries", "## OAuth2 mode limitations"} {
			Expect(document).To(ContainSubstring(anchor))
		}
	})
})
