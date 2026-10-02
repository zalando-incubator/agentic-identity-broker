package e2e_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const consentV2Host = "consent-v2.test.invalid"

type consentV2Scenario struct {
	server, localhostServer                                                                  *bootstrap.TestServer
	callbackURL                                                                              string
	principal, otherPrincipal                                                                id.Principal
	services                                                                                 []*model.ThirdpartyOAuth2ProviderEntity
	sets                                                                                     []*storage.PermissionSet
	first, delta, expired, missing, longName, other, verified, localhost                     *storage.Agent
	delegations                                                                              []*storage.Agent
	grants                                                                                   []*storage.UserGrant
	sessions                                                                                 []*storage.UserSession
	pending, unrated, approved, denied, expiredApproval, otherApproval, incoming, concurrent *storage.ToolApproval
	decisions                                                                                []*storage.ToolApproval
	approvals                                                                                []*storage.ToolApproval
}

func readV2[T any](value T, err error) T {
	GinkgoHelper()
	Expect(err).NotTo(HaveOccurred())
	return value
}

func newConsentV2Scenario(ctx context.Context) *consentV2Scenario {
	GinkgoHelper()
	f := &consentV2Scenario{principal: id.Principal(fixtures.DefaultPrincipal().String()), otherPrincipal: id.Principal(fixtures.AnotherPrincipal().String())}
	callback := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	DeferCleanup(callback.Close)
	f.callbackURL = callback.URL + "/callback"
	GetMockUpstream().WithSuccessfulTokenResponse()
	rejecting := helpers.NewMockUpstreamOAuth2Server().WithErrorResponse("invalid_grant")
	DeferCleanup(rejecting.Close)
	for _, name := range []string{"Project files", "Work calendar", "Expired connection", "Refreshable connection", "Rejected refresh", "Unconnected provider", "Private provider"} {
		upstream := GetMockUpstream().URL()
		if name == "Rejected refresh" {
			upstream = rejecting.URL()
		}
		service := fixtures.ConsentV2Service(name, upstream)
		Expect(GetTestStorage().Services().Create(ctx, service)).To(Succeed())
		f.services = append(f.services, service)
	}
	f.sets = fixtures.ConsentV2PermissionSets(f.services[0].ID, f.services[1].ID)
	for _, set := range f.sets {
		Expect(GetTestStorage().PermissionSets().Create(ctx, set)).To(Succeed())
	}
	f.first = fixtures.ConsentV2Agent("Roadmap assistant", f.callbackURL, f.sets)
	f.delta = fixtures.ConsentV2Agent("Returning assistant", f.callbackURL, f.sets)
	f.expired = fixtures.ConsentV2Agent("Expired assistant", f.callbackURL, f.sets)
	f.longName = fixtures.ConsentV2LongNameAgent(f.callbackURL, f.sets)
	f.other = fixtures.ConsentV2Agent("Private assistant", f.callbackURL, f.sets)
	f.verified = fixtures.ConsentV2CIMDAgent("Verified assistant", "https://"+consentV2Host+"/client", f.sets)
	f.localhost = fixtures.ConsentV2CIMDAgent("Local assistant", "https://localhost/client", f.sets)
	missingSets := fixtures.ConsentV2PermissionSets(f.services[5].ID, f.services[1].ID)
	for _, set := range missingSets {
		set.Name = "Unconnected " + set.Name
		Expect(GetTestStorage().PermissionSets().Create(ctx, set)).To(Succeed())
	}
	f.missing = fixtures.ConsentV2Agent("Disconnected assistant", f.callbackURL, missingSets)
	f.delegations = fixtures.ConsentV2GrantedAgents(f.callbackURL, f.sets)
	agents := append([]*storage.Agent{f.first, f.delta, f.expired, f.longName, f.other, f.verified, f.localhost, f.missing}, f.delegations...)
	for _, agent := range agents {
		Expect(GetTestStorage().Agents().Create(ctx, agent)).To(Succeed())
	}
	f.grants = []*storage.UserGrant{
		fixtures.ConsentV2DeltaGrant(f.principal, f.delta, f.sets),
		fixtures.ConsentV2ExpiredGrant(f.principal, f.expired, f.sets),
		fixtures.ConsentV2Grant(f.principal, f.longName, f.sets[0], f.sets[1]),
		fixtures.ConsentV2Grant(f.principal, f.missing, missingSets[0], missingSets[1]),
		fixtures.ConsentV2Grant(f.otherPrincipal, f.other, f.sets[0], f.sets[1]),
		fixtures.ConsentV2Grant(f.otherPrincipal, f.delta, f.sets[0], f.sets[1]),
	}
	for i, agent := range f.delegations {
		grant := fixtures.ConsentV2Grant(f.principal, agent, f.sets[0], f.sets[1])
		if i%2 == 1 {
			expiry := fixtures.ConsentV2Future()
			grant.ValidUntil = &expiry
		}
		f.grants = append(f.grants, grant)
	}
	for _, grant := range f.grants {
		Expect(GetTestStorage().UserGrants().Create(ctx, grant)).To(Succeed())
	}
	for i, state := range []string{"usable", "usable", "expired", "refreshable", "refreshable"} {
		f.sessions = append(f.sessions, fixtures.ConsentV2Session(f.principal, f.services[i].ID, state))
	}
	f.sessions = append(f.sessions, fixtures.ConsentV2Session(f.otherPrincipal, f.services[6].ID, "usable"))
	for _, session := range f.sessions {
		Expect(GetTestStorage().UserSessions().Create(ctx, session)).To(Succeed())
	}
	f.pending = fixtures.ConsentV2Approval(f.principal, f.delta.ID, "read_roadmap", "high")
	f.unrated = fixtures.ConsentV2Approval(f.principal, f.delta.ID, "unrated_roadmap", "")
	f.approved = fixtures.ConsentV2ResolvedApproval(f.principal, f.delta.ID, "standing_read", true)
	f.denied = fixtures.ConsentV2ResolvedApproval(f.principal, f.delta.ID, "standing_delete", false)
	f.expiredApproval = fixtures.ConsentV2ExpiredApproval(f.principal, f.delta.ID)
	f.otherApproval = fixtures.ConsentV2Approval(f.otherPrincipal, f.other.ID, "private_roadmap", "high")
	f.incoming = fixtures.ConsentV2Approval(f.principal, f.delta.ID, "arriving_roadmap", "low")
	f.concurrent = fixtures.ConsentV2Approval(f.principal, f.delta.ID, "concurrent_roadmap", "low")
	f.approvals = []*storage.ToolApproval{f.pending, f.unrated, f.approved, f.denied, f.expiredApproval, f.otherApproval, f.concurrent}
	for _, choice := range []string{"once", "session", "permanent", "deny"} {
		approval := fixtures.ConsentV2Approval(f.principal, f.delta.ID, "decide_"+choice, "medium")
		f.decisions = append(f.decisions, approval)
		f.approvals = append(f.approvals, approval)
	}
	for _, approval := range f.approvals {
		_, err := GetTestStorage().ToolApprovals().Create(ctx, approval)
		Expect(err).NotTo(HaveOccurred())
	}

	// The same TLS document/fake-host transport seam as cimd_flow_test.go. All
	// metadata parsing, redirect validation, sealing and authorization remain real.
	for _, host := range []string{consentV2Host, "localhost"} {
		document := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Cache-Control", "max-age=300")
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "https://" + host + "/client", "client_name": "Consent UI assistant", "redirect_uris": []string{f.callbackURL}, "logo_uri": "https://external-images.example.invalid/agent.png"})
		}))
		DeferCleanup(document.Close)
		fetcher := readV2(bootstrap.NewCIMDTestFetcher(document, host, 5120))
		factory := bootstrap.NewServerFactory(fixtures.OAuth2ConfigWithCIMD(GetMockUpstream().URL()), GetLogger())
		server := readV2(bootstrap.NewCIMDEndUserTestServer(GetTestStorage(), factory, fetcher, GetLogger()))
		DeferCleanup(server.Close)
		if host == consentV2Host {
			f.server = server
		} else {
			f.localhostServer = server
		}
	}
	return f
}

// startAuthorizationRequest never constructs a session token: the browser
// obtains it from the production /oauth2/authorize redirect with real PKCE.
func startAuthorizationRequest(ctx context.Context, page playwright.Page, baseURL string, agent *storage.Agent, redirectURI string) string {
	GinkgoHelper()
	clientID := agent.ID.String()
	if len(agent.ClientURIs) != 0 {
		clientID = agent.ClientURIs[0]
	}
	query := url.Values{"client_id": {clientID}, "redirect_uri": {redirectURI}, "response_type": {"code"}, "state": {"consent-v2-state"}, "code_challenge": {helpers.GenerateCodeChallenge(helpers.PKCEVerifier())}, "code_challenge_method": {"S256"}}
	_, err := page.Goto(baseURL + "/oauth2/authorize?" + query.Encode())
	Expect(err).NotTo(HaveOccurred())
	Eventually(func() string { return page.URL() }).Should(ContainSubstring("/agents/" + agent.ID.String() + "?"))
	location := readV2(url.Parse(page.URL()))
	Expect(location.Query().Get("session_token")).NotTo(BeEmpty(), "production authorize must issue the opaque authorization context")
	Expect(pages.NewConsentPage(page, baseURL).WaitForPageLoad(ctx)).To(Succeed())
	return location.RequestURI()
}

// An expired delegated connection is an existing production re-consent trigger.
// Restore that connection after authorization issues its real token, so the
// decision under test contains the existing grant and fully connected services.
func startReauthorizationRequest(ctx context.Context, page playwright.Page, f *consentV2Scenario, agent *storage.Agent) string {
	GinkgoHelper()
	session := readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[0].ID))
	Expect(session).NotTo(BeNil())
	expired := fixtures.ConsentV2Session(f.principal, f.services[0].ID, "expired")
	Expect(GetTestStorage().UserSessions().Create(ctx, expired)).To(Succeed())
	restored := false
	defer func() {
		if !restored {
			Expect(GetTestStorage().UserSessions().Create(ctx, session)).To(Succeed())
		}
	}()
	path := startAuthorizationRequest(ctx, page, f.server.BaseURL(), agent, f.callbackURL)
	Expect(GetTestStorage().UserSessions().Create(ctx, session)).To(Succeed())
	restored = true
	consentV2Reload(page)
	return path
}

func newConsentV2Browser(principal id.Principal) (playwright.BrowserContext, playwright.Page) {
	GinkgoHelper()
	browserContext := readV2(suiteCtx.Browser.NewContext(playwright.BrowserNewContextOptions{
		Viewport: &playwright.Size{Width: 1280, Height: 1080}, Locale: playwright.String("en-US"),
		TimezoneId: playwright.String("UTC"), ColorScheme: playwright.ColorSchemeLight,
		ExtraHttpHeaders: map[string]string{"X-Remote-User": principal.String()},
	}))
	page := readV2(browserContext.NewPage())
	page.SetDefaultTimeout(10_000)
	return browserContext, page
}

// forEachTheme is exclusively for AS-15's read-only surface traversal. A fresh
// context prevents preference/cookie leakage, and the recorder rejects mutations.
func forEachTheme(ctx context.Context, baseURL string, body func(string, playwright.Page, *pages.Page)) {
	GinkgoHelper()
	for _, theme := range []string{"light", "dark"} {
		func() {
			browserContext, page := newConsentV2Browser(id.Principal(fixtures.DefaultPrincipal().String()))
			defer func() { Expect(browserContext.Close()).To(Succeed()) }()
			base := pages.NewPage(page, baseURL)
			Expect(base.SetThemePreference(ctx, theme)).To(Succeed())
			Expect(base.StartRequestRecorder(ctx)).To(Succeed())
			defer func() {
				for _, request := range readV2(base.RecordedRequests(ctx)) {
					location := readV2(url.Parse(request.URL))
					if strings.HasPrefix(location.Path, "/api/") {
						Expect(request.Method).To(Equal(http.MethodGet), "AS-15 theme traversal must not mutate authorization")
					}
				}
			}()
			body(theme, page, base)
		}()
	}
}

// The server rejects names over 255 characters. Exercise defensive rendering
// of an adversarial 300-character response without bypassing that validation:
// fetch the real authenticated detail and replace only its display_name field.
func consentV2AdversarialAgentResponse(ctx context.Context, page playwright.Page, baseURL string, agentID id.AgentID) {
	GinkgoHelper()
	Expect(ctx.Err()).NotTo(HaveOccurred())
	Expect(page.Route(baseURL+"/api/consent/agents/"+agentID.String(), func(route playwright.Route) {
		defer GinkgoRecover()
		if route.Request().Method() != http.MethodGet {
			Expect(route.Continue()).To(Succeed())
			return
		}
		response := readV2(route.Fetch())
		defer func() { Expect(response.Dispose()).To(Succeed()) }()
		Expect(response.Status()).To(Equal(http.StatusOK), "adversarial display data must decorate a real authorized response")
		var envelope, data, agent map[string]json.RawMessage
		Expect(response.JSON(&envelope)).To(Succeed())
		Expect(json.Unmarshal(envelope["data"], &data)).To(Succeed())
		Expect(json.Unmarshal(data["agent"], &agent)).To(Succeed())
		var returnedID string
		Expect(json.Unmarshal(agent["agentId"], &returnedID)).To(Succeed())
		Expect(returnedID).To(Equal(agentID.String()))
		agent["display_name"] = readV2(json.Marshal(fixtures.ConsentV2AdversarialDisplayName()))
		data["agent"] = readV2(json.Marshal(agent))
		envelope["data"] = readV2(json.Marshal(data))
		headers := response.Headers()
		delete(headers, "content-length")
		Expect(route.Fulfill(playwright.RouteFulfillOptions{
			Response: response, Headers: headers, Body: readV2(json.Marshal(envelope)),
		})).To(Succeed())
	})).To(Succeed())
}

func expectNoThirdPartyOrGatewayRequests(baseURL string, recorded []pages.RecordedRequest) {
	GinkgoHelper()
	origin := readV2(url.Parse(baseURL))
	for _, request := range recorded {
		location := readV2(url.Parse(request.URL))
		Expect(location.Scheme+"://"+location.Host).To(Equal(origin.Scheme+"://"+origin.Host), "automatic resource %s", request.URL)
		Expect(request.Method == http.MethodGet && location.Path == "/api/approvals").To(BeFalse(), "browser must never call gateway long-poll")
	}
}

func consentV2ApprovalSnapshot(ctx context.Context, f *consentV2Scenario, except id.ApprovalID) map[string]string {
	GinkgoHelper()
	result := make(map[string]string)
	for _, approval := range f.approvals {
		if approval.ID == except {
			continue
		}
		stored := readV2(GetTestStorage().ToolApprovals().Get(ctx, approval.ID))
		result[approval.ID.String()] = string(readV2(json.Marshal(stored)))
	}
	return result
}

func consentV2Grant(ctx context.Context, principal id.Principal, agent *storage.Agent) *storage.UserGrant {
	GinkgoHelper()
	grant, err := GetTestStorage().UserGrants().FindByPrincipalAndAgent(ctx, principal, agent.ID)
	if ports.IsNotFoundErr(err) {
		return nil
	}
	Expect(err).NotTo(HaveOccurred())
	return grant
}

func consentV2GrantSnapshot(ctx context.Context, principal id.Principal, agent *storage.Agent) string {
	return string(readV2(json.Marshal(consentV2Grant(ctx, principal, agent))))
}

func consentV2RowsContain(rows []pages.DelegationsRow, name string) bool {
	for _, row := range rows {
		if row.Agent == name {
			return true
		}
	}
	return false
}

func consentV2PendingContains(rows []pages.PendingApprovalRow, tool string) bool {
	for _, row := range rows {
		if row.Tool == tool {
			return true
		}
	}
	return false
}

func consentV2Connection(rows []pages.ConnectionRow, provider string) pages.ConnectionRow {
	GinkgoHelper()
	for _, row := range rows {
		if row.Provider == provider {
			return row
		}
	}
	Fail("connection row not found: " + provider)
	return pages.ConnectionRow{}
}

func consentV2Reload(page playwright.Page) {
	GinkgoHelper()
	_, err := page.Reload()
	Expect(err).NotTo(HaveOccurred())
}

func consentV2Scope(approval *storage.ToolApproval) string {
	return fmt.Sprintf("%s(path=/projects/roadmap,recursive=true)", approval.ToolName)
}

func consentV2ScreenshotContext(ctx context.Context, _ string) (playwright.BrowserContext, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	browserContext, err := suiteCtx.Browser.NewContext(playwright.BrowserNewContextOptions{
		Locale: playwright.String("en-US"), TimezoneId: playwright.String("UTC"),
		ColorScheme:      playwright.ColorSchemeLight,
		ExtraHttpHeaders: map[string]string{"X-Remote-User": fixtures.DefaultPrincipal().String()},
	})
	if err != nil {
		return nil, err
	}
	// Fix only screenshot display time. The caller's clock, draft, focus, theme,
	// and server-side TTL validation are untouched by visual capture.
	stamp := fixtures.ConsentV2Clock().UnixMilli()
	script := fmt.Sprintf(`{ const NativeDate = Date; const fixed = %d; window.Date = class extends NativeDate { constructor(...args) { super(...(args.length ? args : [fixed])); } static now() { return fixed; } }; }`, stamp)
	if err := browserContext.AddInitScript(playwright.Script{Content: &script}); err != nil {
		_ = browserContext.Close()
		return nil, err
	}
	return browserContext, nil
}

func consentV2ExpectPermissionSelection(ctx context.Context, page *pages.ConsentPage, names ...string) {
	GinkgoHelper()
	groups := readV2(page.PermissionGroups(ctx))
	var selected []string
	for _, group := range groups {
		if group.Checked {
			selected = append(selected, group.Name)
		}
	}
	Expect(selected).To(ConsistOf(names))
}

func consentV2EventuallyDecision(ctx context.Context, approval *storage.ToolApproval, status storage.ApprovalStatus, persistence *storage.ApprovalPersistence) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		stored, err := GetTestStorage().ToolApprovals().Get(ctx, approval.ID)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(stored.Status).To(Equal(status))
		g.Expect(stored.Persistence).To(Equal(persistence))
		g.Expect(stored.ToolPattern).To(Equal(approval.ToolPattern))
		g.Expect(stored.ParamsPattern).To(Equal(approval.ParamsPattern))
		g.Expect(stored.Consumed).To(BeFalse())
	}).WithTimeout(5 * time.Second).Should(Succeed())
}

func consentV2ExpectProviderCallback(recorded []pages.RecordedRequest, service id.ServiceID) {
	GinkgoHelper()
	var callback bool
	for _, request := range recorded {
		location := readV2(url.Parse(request.URL))
		if location.Path == "/api/third-party/"+service.String()+"/oauth2/callback" {
			Expect(request.Method).To(Equal(http.MethodGet))
			Expect(location.Query().Get("code")).NotTo(BeEmpty())
			Expect(location.Query().Get("state")).NotTo(BeEmpty())
			callback = true
		}
	}
	Expect(callback).To(BeTrue(), "the browser must complete the real provider callback")
}
