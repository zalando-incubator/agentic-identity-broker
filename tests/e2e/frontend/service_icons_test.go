package e2e_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Original artwork: the embedded raster is generated here, not downloaded or
// copied from a logo. The documents intentionally preserve export formatting.
func serviceIconVisualDocuments(resourceOrigin, raster string) []string {
	return []string{
		`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE svg PUBLIC "-//W3C//DTD SVG 1.1//EN" "` + resourceOrigin + `/svg11.dtd">
<svg xmlns="http://www.w3.org/2000/svg" xmlns:reuse="http://www.w3.org/1999/xlink" viewBox="0 0 96 96" width="96" height="96">
<!-- Original Atlas constellation, with forward references and Unicode. -->
<metadata id="service-icon-artwork">Atlas # Δ &amp; stars</metadata>
<style>.orbit { fill: url(#paint); stroke: #194; stroke-width: 2 } text { font: italic bold 12px sans-serif; fill: #134 }</style>
<use reuse:href="#star" x="45" y="10"/>
<defs>
 <linearGradient id="paint"><stop stop-color="#69d"/><stop offset="1" stop-color="#1a8"/></linearGradient>
 <radialGradient id="glow"><stop stop-color="white"/><stop offset="1" stop-color="#39b"/></radialGradient>
 <pattern id="dots" width="8" height="8" patternUnits="userSpaceOnUse"><circle cx="2" cy="2" r="1" fill="#fff"/></pattern>
 <clipPath id="clip"><circle cx="48" cy="48" r="42"/></clipPath>
 <mask id="mask"><rect width="96" height="96" fill="white"/><circle cx="48" cy="48" r="9" fill="black"/></mask>
 <filter id="soft"><feGaussianBlur stdDeviation="0.3"/><feColorMatrix type="saturate" values="0.8"/></filter>
 <marker id="tip" markerWidth="4" markerHeight="4" refX="2" refY="2"><path d="M0 0L4 2L0 4z" fill="#194"/></marker>
 <path id="star" d="M0-6 2-2 6 0 2 2 0 6-2 2-6 0-2-2z" fill="#ffb"/>
</defs>
<g clip-path="url(#clip)" mask="url(#mask)"><circle class="orbit" cx="48" cy="48" r="40" filter="url(#soft)"/><rect width="96" height="96" fill="url(#dots)" opacity=".4"/></g>
<ellipse cx="48" cy="48" rx="40" ry="13" fill="none" stroke="#173" marker-end="url(#tip)"/>
<circle cx="48" cy="48" r="7" fill="url(#glow)"><animate attributeName="opacity" values=".5;1;.5" dur=".2s" repeatCount="indefinite"/></circle>
<image x="42" y="42" width="12" height="12" href="data:image/png;base64,` + raster + `"/>
<text x="29" y="83">A &amp; Δ</text>
</svg>`,
		`<!DOCTYPE svg SYSTEM "` + resourceOrigin + `/original.dtd">
<s:svg xmlns:s="http://www.w3.org/2000/svg" xmlns:link="http://www.w3.org/1999/xlink" viewBox="0 0 80 80" width="80" height="80">
<s:metadata id="service-icon-artwork">Original Cobalt weaving</s:metadata>
<s:use link:href="#weave" x="40" y="40"/>
<s:g id="weave"><s:rect x="-24" y="-24" width="48" height="48" rx="9" fill="#478"><s:animateTransform attributeName="transform" type="rotate" values="-8;8;-8" dur=".3s" repeatCount="indefinite"/></s:rect><s:path d="M-22 0H22M0-22V22" stroke="#fec" stroke-width="6"/></s:g>
<s:text x="12" y="73" style="font: bold 10px sans-serif; fill: #254">Weave Δ</s:text>
</s:svg>`,
	}
}

type iconBrowserService struct {
	ID      string
	Name    string
	Request map[string]any
}

type iconBrowserJourney struct {
	ctx             context.Context
	admin           *bootstrap.TestServer
	broker          *bootstrap.TestServer
	token           string
	services        []iconBrowserService
	agentID         string
	consent         *pages.ConsentPage
	connections     *pages.ConsentPage
	sessions        *pages.SessionsPage
	consentIcons    *pages.ServiceIconsPage
	connectionIcons *pages.ServiceIconsPage
	sessionIcons    *pages.ServiceIconsPage
	visuals         []string
	resources       *httptest.Server
	discoveryLoads  atomic.Int64
	externalLoads   atomic.Int64
	browserLoads    atomic.Int64
	avatarLoads     atomic.Int64
	executions      atomic.Int64
	pageErrors      atomic.Int64
}

func (j *iconBrowserJourney) adminJSON(method, path string, body map[string]any, status int) map[string]any {
	GinkgoHelper()
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		Expect(err).NotTo(HaveOccurred())
		input = bytes.NewReader(encoded)
	}
	response, err := j.admin.DirectRequest(method, path, fixtures.AdminPrincipal().String(), map[string]string{"Content-Type": "application/json"}, input)
	Expect(err).NotTo(HaveOccurred())
	defer func() { Expect(response.Body.Close()).To(Succeed()) }()
	data, err := io.ReadAll(response.Body)
	Expect(err).NotTo(HaveOccurred())
	Expect(response.StatusCode).To(Equal(status), "%s %s: %s", method, path, data)
	var result map[string]any
	Expect(json.Unmarshal(data, &result)).To(Succeed())
	return result
}

func (j *iconBrowserJourney) seed(sources []string) {
	GinkgoHelper()
	for index, name := range []string{"Atlas Files", "Cobalt Notes"} {
		body := map[string]any{
			"display_name": name, "client_id": "icon-client-" + name,
			"client_secret": "synthetic-icon-secret-" + name,
			"issuer_uri":    GetMockUpstream().URL(),
			"discovery":     map[string]any{"enable_discovery": false},
			"endpoints":     map[string]any{"authorize_endpoint": GetMockUpstream().URL() + "/oauth/authorize", "token_endpoint": GetMockUpstream().URL() + "/oauth/token"},
			"scopes":        []map[string]any{{"scope_value": "read", "description": "Read original test documents"}},
		}
		if sources[index] != "" {
			body["icon_svg"] = sources[index]
		}
		created := j.adminJSON(http.MethodPost, "/api/services", body, http.StatusCreated)
		serviceID, ok := created["id"].(string)
		Expect(ok).To(BeTrue())
		j.services = append(j.services, iconBrowserService{ID: serviceID, Name: name, Request: body})
	}
	first := id.MustParseServiceID(j.services[0].ID)
	second := id.MustParseServiceID(j.services[1].ID)
	permissionID := id.NewPermissionSetID()
	createPermissionSet(j.ctx, permissionID, "Original document access", "Required and optional live service requirements", []storage.ServiceScope{
		{ServiceID: first, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory},
		{ServiceID: second, Scopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
	})
	agent := fixtures.ValidAgent()
	agent.DisplayName = "Original Artwork Assistant"
	agent.PermissionSets = []storage.AgentPermissionSetEntry{{PermissionSetID: permissionID, RequirementType: storage.RequirementTypeMandatory}}
	agent.ServiceRequirements = []storage.ServiceRequirement{
		{ServiceID: first, RequiredScopes: []string{"read"}, RequirementType: storage.RequirementTypeMandatory},
		{ServiceID: second, RequiredScopes: []string{"read"}, RequirementType: storage.RequirementTypeOptional},
	}
	Expect(GetTestStorage().Agents().Create(j.ctx, agent)).To(Succeed())
	j.agentID = agent.ID.String()
	otherGrant := fixtures.IndefiniteGrant(fixtures.AnotherPrincipal().String(), j.agentID, first.String(), []string{"read"})
	otherGrant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionID, IncludedServiceIDs: []id.ServiceID{first, second}}}
	Expect(GetTestStorage().UserGrants().Create(j.ctx, otherGrant)).To(Succeed())
	grant := fixtures.IndefiniteGrant(fixtures.DefaultPrincipal().String(), j.agentID, first.String(), []string{"read"})
	grant.GrantedPermissionSets = []storage.GrantedPermissionSetEntry{{PermissionSetID: permissionID, IncludedServiceIDs: []id.ServiceID{first, second}}}
	Expect(GetTestStorage().UserGrants().Create(j.ctx, grant)).To(Succeed())
	for _, service := range j.services {
		Expect(GetTestStorage().UserSessions().Create(j.ctx, fixtures.SessionForServiceWithScopes(fixtures.DefaultPrincipal().String(), service.ID, []string{"read"}))).To(Succeed())
	}
}

func (j *iconBrowserJourney) update(index int, changes map[string]any, status int) {
	GinkgoHelper()
	body := make(map[string]any, len(j.services[index].Request)+len(changes))
	for key, value := range j.services[index].Request {
		if key != "icon_svg" {
			body[key] = value
		}
	}
	for key, value := range changes {
		body[key] = value
	}
	j.adminJSON(http.MethodPut, "/api/services/"+j.services[index].ID, body, status)
}

func (j *iconBrowserJourney) expectAdminSource(index int, source string) {
	GinkgoHelper()
	result := j.adminJSON(http.MethodGet, "/api/services/"+j.services[index].ID, nil, http.StatusOK)
	if source == "" {
		Expect(result).NotTo(HaveKey("icon_svg"))
	} else {
		Expect(result).To(HaveKeyWithValue("icon_svg", source))
	}
}

func (j *iconBrowserJourney) fresh(page playwright.Page, path string, action func() error, sources []string) {
	GinkgoHelper()
	response, err := page.ExpectResponse(func(response playwright.Response) bool {
		target, err := url.Parse(response.URL())
		return err == nil && target.Path == path && response.Request().Method() == http.MethodGet
	}, action)
	Expect(err).NotTo(HaveOccurred(), "a real fresh GET must reach the broker, not only the JavaScript cache")
	Expect(response.Status()).To(Equal(http.StatusOK))
	var envelope struct {
		Data struct {
			Services []map[string]any `json:"services"`
			Sessions []map[string]any `json:"sessions"`
		} `json:"data"`
	}
	Expect(response.JSON(&envelope)).To(Succeed())
	entries, idField, iconField := envelope.Data.Services, "serviceId", "iconSvg"
	if path == "/api/third-party/sessions" {
		entries, idField, iconField = envelope.Data.Sessions, "service_id", "service_icon_svg"
	}
	for index, service := range j.services {
		var found map[string]any
		for _, entry := range entries {
			if entry[idField] == service.ID {
				found = entry
				break
			}
		}
		Expect(found).NotTo(BeNil(), "existing delivery projection must contain %s", service.Name)
		if sources[index] == "" {
			Expect(found).NotTo(HaveKey(iconField))
		} else {
			Expect(found).To(HaveKeyWithValue(iconField, sources[index]))
		}
	}
}

func (j *iconBrowserJourney) open(sources []string) {
	GinkgoHelper()
	path := "/api/consent/agents/" + j.agentID
	j.fresh(j.consent.GetPlaywrightPage(), path, func() error { return j.consent.NavigateToAgent(j.ctx, j.agentID) }, sources)
	j.fresh(j.connections.GetPlaywrightPage(), path, func() error { return j.connections.NavigateToAgent(j.ctx, j.agentID) }, sources)
	j.fresh(j.sessions.GetPlaywrightPage(), "/api/third-party/sessions", func() error { return j.sessions.NavigateToSessions(j.ctx) }, sources)
}

func (j *iconBrowserJourney) refresh(sources []string) {
	GinkgoHelper()
	// Save and Refresh are the existing production invalidation paths for the
	// five-minute consent and two-minute session/detail caches, respectively.
	Expect(j.consent.ExpectGrantSuccess(func() error {
		j.fresh(j.consent.GetPlaywrightPage(), "/api/consent/agents/"+j.agentID, func() error { return j.consent.SaveConsent(j.ctx) }, sources)
		return nil
	})).To(Succeed())
	j.fresh(j.sessions.GetPlaywrightPage(), "/api/third-party/sessions", func() error { return j.sessions.RefreshSession(j.ctx, j.services[0].Name) }, sources)
	Expect(j.sessions.WaitForSuccessMessage(j.ctx, "Session token refreshed successfully.")).To(Succeed())
}

func (j *iconBrowserJourney) freshConnections(sources []string) {
	GinkgoHelper()
	// The disconnected user's mandatory service has no Save/refetch action.
	// Expire its real five-minute cache and use SPA navigation, not a reload.
	j.fresh(j.connections.GetPlaywrightPage(), "/api/consent/agents/"+j.agentID, func() error { return j.connectionIcons.ReopenConnectionsAfterExpiry("Original Artwork Assistant") }, sources)
}

func (j *iconBrowserJourney) connectAccounts() {
	GinkgoHelper()
	for _, service := range j.services {
		Expect(j.connections.DelegateService(j.ctx, service.Name)).To(Succeed())
		Expect(j.connections.WaitForURL(j.ctx, "service_id="+service.ID)).To(Succeed())
		Expect(j.connections.WaitForPageLoad(j.ctx)).To(Succeed())
	}
	Expect(j.connections.IsSaveButtonEnabled(j.ctx)).To(BeTrue())
	Expect(j.connections.ExpectGrantSuccess(func() error { return j.connections.SaveConsent(j.ctx) })).To(Succeed())
}

func expectServiceIconIdentity(page *pages.ServiceIconsPage, surface pages.ServiceIconSurface, service iconBrowserService, source string) {
	GinkgoHelper()
	if source != "" {
		Expect(page.WaitForImage(surface, service.ID, service.Name, source)).To(Succeed())
	}
	Eventually(func(g Gomega) {
		identity, err := page.ReadIdentity(surface, service.ID, service.Name)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(identity.NameVisible).To(BeTrue(), "%s name must remain readable on %s", service.Name, surface)
		g.Expect(identity.InlineArt).To(BeZero(), "operator SVG must never enter the page DOM")
		g.Expect(identity.Embeddings).To(BeZero())
		if source == "" {
			g.Expect(identity.Images).To(BeEmpty(), "fallback must hide a failed/broken image marker")
			if surface == pages.ServiceIconSession || surface == pages.ServiceIconDetail {
				g.Expect(identity.Initials).To(BeZero(), "session fallback remains name-only")
			} else {
				g.Expect(identity.Initials).To(Equal(1), "consent fallback retains its original initial")
			}
			return
		}
		g.Expect(identity.Images).To(HaveLen(1))
		img := identity.Images[0]
		g.Expect(img.Source).To(HavePrefix("data:image/svg+xml;charset=utf-8,"))
		decoded, err := url.PathUnescape(strings.TrimPrefix(img.Source, "data:image/svg+xml;charset=utf-8,"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(decoded).To(Equal(source))
		g.Expect(img.Complete).To(BeTrue())
		g.Expect(img.NaturalWidth).To(BeNumerically(">", 0))
		g.Expect(img.Alt).To(BeEmpty())
		g.Expect(img.Focusable).To(BeFalse())
	}).Should(Succeed())
}

func (j *iconBrowserJourney) expectConnected(sources []string) {
	GinkgoHelper()
	for index, surface := range []pages.ServiceIconSurface{pages.ServiceIconRequired, pages.ServiceIconOptional} {
		expectServiceIconIdentity(j.consentIcons, surface, j.services[index], sources[index])
		expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconSession, j.services[index], sources[index])
	}
}

func (j *iconBrowserJourney) freshDetail(index int, source string, action func() error) {
	GinkgoHelper()
	response, err := j.sessions.GetPlaywrightPage().ExpectResponse(func(response playwright.Response) bool {
		target, err := url.Parse(response.URL())
		return err == nil && target.Path == "/api/third-party/"+j.services[index].ID+"/session" && response.Request().Method() == http.MethodGet
	}, action)
	Expect(err).NotTo(HaveOccurred(), "session-detail source must come from an observed authenticated response")
	Expect(response.Status()).To(Equal(http.StatusOK))
	var envelope struct {
		Data struct {
			Session map[string]any `json:"session"`
		} `json:"data"`
	}
	Expect(response.JSON(&envelope)).To(Succeed())
	Expect(envelope.Data.Session).To(HaveKeyWithValue("service_id", j.services[index].ID))
	if source == "" {
		Expect(envelope.Data.Session).NotTo(HaveKey("service_icon_svg"))
	} else {
		Expect(envelope.Data.Session).To(HaveKeyWithValue("service_icon_svg", source))
	}
}

func (j *iconBrowserJourney) expectAll(sources []string) {
	GinkgoHelper()
	j.expectConnected(sources)
	for index, service := range j.services {
		expectServiceIconIdentity(j.connectionIcons, pages.ServiceIconConnection, service, sources[index])
		j.freshDetail(index, sources[index], func() error { return j.sessions.OpenSessionDetails(j.ctx, service.Name) })
		expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconDetail, service, sources[index])
		Expect(j.sessions.CancelSessionDetails(j.ctx)).To(Succeed())
	}
}

func (j *iconBrowserJourney) failConnected(sources []string) {
	GinkgoHelper()
	for index, surface := range []pages.ServiceIconSurface{pages.ServiceIconRequired, pages.ServiceIconOptional} {
		service := j.services[index]
		Expect(j.consentIcons.FailImage(surface, service.ID, service.Name, sources[index])).To(Succeed())
		Expect(j.sessionIcons.FailImage(pages.ServiceIconSession, service.ID, service.Name, sources[index])).To(Succeed())
	}
	j.expectConnected([]string{"", ""})
}

func (j *iconBrowserJourney) screenshots(state string) {
	GinkgoHelper()
	Expect(j.consent.TakeScreenshot(j.ctx, "service_icons_"+state+"_consent_requirements")).To(Succeed())
	Expect(j.connections.TakeScreenshot(j.ctx, "service_icons_"+state+"_connection_cards")).To(Succeed())
	Expect(j.sessions.TakeScreenshot(j.ctx, "service_icons_"+state+"_session_cards")).To(Succeed())
}

func (j *iconBrowserJourney) expectIsolation() {
	GinkgoHelper()
	Consistently(func() int64 { return j.externalLoads.Load() + j.browserLoads.Load() + j.executions.Load() }, 500*time.Millisecond, 25*time.Millisecond).Should(BeZero())
	for _, page := range []*pages.ServiceIconsPage{j.consentIcons, j.connectionIcons, j.sessionIcons} {
		executed, err := page.GetPlaywrightPage().Evaluate(`() => window.__serviceIconExecuted === true`)
		Expect(err).NotTo(HaveOccurred())
		Expect(executed).To(BeFalse())
	}
}

var _ = Describe("Configurable service icons", Label("built-frontend"), func() {
	var j *iconBrowserJourney

	BeforeEach(func() {
		j = &iconBrowserJourney{ctx: context.Background()}
		var raster bytes.Buffer
		art := image.NewRGBA(image.Rect(0, 0, 2, 2))
		art.Set(0, 0, color.RGBA{R: 30, G: 160, B: 100, A: 255})
		Expect(png.Encode(&raster, art)).To(Succeed())
		j.resources = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/avatar.png" {
				j.avatarLoads.Add(1)
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(raster.Bytes())
				return
			}
			if r.URL.Path == "/.well-known/oauth-authorization-server" {
				j.discoveryLoads.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]string{
					"authorization_endpoint": GetMockUpstream().URL() + "/oauth/authorize",
					"token_endpoint":         GetMockUpstream().URL() + "/oauth/token",
				})
				return
			}
			j.externalLoads.Add(1)
			w.Header().Set("Content-Type", "application/javascript")
			_, _ = io.WriteString(w, "window.__serviceIconExecuted = true; alert('service icon execution');")
		}))
		DeferCleanup(j.resources.Close)
		j.visuals = serviceIconVisualDocuments(j.resources.URL, base64.StdEncoding.EncodeToString(raster.Bytes()))
		jwks := helpers.NewMockJWKSServer()
		DeferCleanup(jwks.Close)
		config := fixtures.OAuth2ConfigWithUpstream(GetMockUpstream().URL())
		signedIdentity := fixtures.SignedJWTConfig(jwks.JWKSURL())
		config.Server.EndUser.Authentication = signedIdentity.Server.EndUser.Authentication
		config.Security = signedIdentity.Security
		factory := bootstrap.NewServerFactory(config, GetLogger())
		builder, err := bootstrap.NewTestServerBuilder(config, GetTestStorage(), factory, GetLogger())
		Expect(err).NotTo(HaveOccurred())
		j.broker, err = builder.Build()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(j.broker.Close)
		j.admin, err = bootstrap.NewAdminTestServer(j.broker.App(), GetLogger())
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(j.admin.CloseListener)
		claims := func(principal string) map[string]any {
			return map[string]any{"sub": principal, "name": "Original Artwork Reviewer", "email": principal, "picture": j.resources.URL + "/avatar.png", "iss": "https://auth.example.com", "aud": "agentic-identity-broker", "exp": time.Now().Add(time.Hour).Unix()}
		}
		j.token, err = jwks.SignJWT(claims(fixtures.DefaultPrincipal().String()))
		Expect(err).NotTo(HaveOccurred())
		Expect(GetTestContext().SetExtraHTTPHeaders(map[string]string{"Authorization": "Bearer " + j.token})).To(Succeed())
		connectionPage, err := GetTestContext().NewPage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(connectionPage.Close)
		otherToken, err := jwks.SignJWT(claims(fixtures.AnotherPrincipal().String()))
		Expect(err).NotTo(HaveOccurred())
		Expect(connectionPage.SetExtraHTTPHeaders(map[string]string{"Authorization": "Bearer " + otherToken})).To(Succeed())
		sessionPage, err := GetTestContext().NewPage()
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(sessionPage.Close)
		j.consent = pages.NewConsentPage(GetTestPage(), j.broker.BaseURL())
		j.connections = pages.NewConsentPage(connectionPage, j.broker.BaseURL())
		j.sessions = pages.NewSessionsPage(sessionPage, j.broker.BaseURL())
		j.consentIcons = pages.NewServiceIconsPage(GetTestPage(), j.broker.BaseURL())
		j.connectionIcons = pages.NewServiceIconsPage(connectionPage, j.broker.BaseURL())
		j.sessionIcons = pages.NewServiceIconsPage(sessionPage, j.broker.BaseURL())
		for _, page := range []playwright.Page{GetTestPage(), connectionPage, sessionPage} {
			Expect(page.AddInitScript(playwright.Script{Content: playwright.String("window.__serviceIconExecuted = false;")})).To(Succeed())
			page.OnDialog(func(dialog playwright.Dialog) { j.executions.Add(1); _ = dialog.Dismiss() })
			page.OnPageError(func(error) { j.pageErrors.Add(1) })
			page.OnRequest(func(request playwright.Request) {
				if (strings.HasPrefix(request.URL(), j.resources.URL) && request.URL() != j.resources.URL+"/avatar.png") || (request.ResourceType() == "image" && strings.HasPrefix(request.URL(), "http") && request.URL() != j.resources.URL+"/avatar.png") {
					j.browserLoads.Add(1)
				}
			})
		}
		GetMockUpstream().WithSuccessfulTokenResponse().WithAccessToken("icon-refreshed-access").WithRefreshToken("icon-refreshed-refresh").WithExpiresIn(3600)
	})

	Context("when an operator edits configured services", func() {
		BeforeEach(func() { j.seed([]string{j.visuals[0], j.visuals[0]}) })

		// US1.3 from specs/20261118-224349-configure-service-icons/spec.md (User Story 1).
		It("should preserve an omitted icon and restore original fallbacks after explicit null [US1.3]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			j.expectAll(sources)
			j.update(0, map[string]any{}, http.StatusOK)
			j.expectAdminSource(0, sources[0])
			j.refresh(sources)
			j.freshConnections(sources)
			j.expectAll(sources)
			j.screenshots("preserved_configured")
			j.update(0, map[string]any{"icon_svg": nil}, http.StatusOK)
			sources[0] = ""
			j.expectAdminSource(0, "")
			j.refresh(sources)
			j.freshConnections(sources)
			j.expectAll(sources)
			j.screenshots("cleared_original_fallback")
			j.expectIsolation()
		})

		// US1.4 from specs/20261118-224349-configure-service-icons/spec.md (User Story 1).
		It("should reject URL path and empty icon writes atomically without fetching them [US1.4]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			before := j.adminJSON(http.MethodGet, "/api/services/"+j.services[0].ID, nil, http.StatusOK)
			for _, invalid := range []any{j.resources.URL + "/remote.svg", "/images/service.svg", "", " \n\t ", 42, map[string]any{"svg": "not a string"}, []any{"svg"}} {
				j.update(0, map[string]any{"icon_svg": invalid, "display_name": "Must not persist", "client_id": "must-not-persist", "client_secret": "must-not-persist"}, http.StatusBadRequest)
				Expect(j.adminJSON(http.MethodGet, "/api/services/"+j.services[0].ID, nil, http.StatusOK)).To(Equal(before))
			}
			j.refresh(sources)
			j.expectAll(sources)
			j.expectIsolation()
		})

		// US2.2 from specs/20261118-224349-configure-service-icons/spec.md (User Story 2).
		It("should replace every fresh service identity without changing an identical-source peer [US2.2]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			j.expectAll(sources)
			j.update(0, map[string]any{"icon_svg": j.visuals[1]}, http.StatusOK)
			sources[0] = j.visuals[1]
			j.refresh(sources)
			j.freshConnections(sources)
			j.expectAll(sources)
			j.expectAdminSource(0, sources[0])
			j.expectAdminSource(1, sources[1])
			j.screenshots("replacement_peer_unchanged")
		})

		// US2.3 from specs/20261118-224349-configure-service-icons/spec.md (User Story 2).
		It("should reject named active constructs atomically and isolate accepted external references [US2.3]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			before := j.adminJSON(http.MethodGet, "/api/services/"+j.services[0].ID, nil, http.StatusOK)
			stored, err := GetTestStorage().Services().Get(j.ctx, id.MustParseServiceID(j.services[0].ID))
			Expect(err).NotTo(HaveOccurred())
			storedBefore, err := json.Marshal(stored)
			Expect(err).NotTo(HaveOccurred())
			credentialBefore, err := stored.Secret.GetCiphertext()
			Expect(err).NotTo(HaveOccurred())
			wrap := func(body string) string { return `<svg xmlns="http://www.w3.org/2000/svg">` + body + `</svg>` }
			invalid := []struct{ name, source string }{
				{"malformed XML", wrap(`<g>`)}, {"oversized decoded source", wrap(strings.Repeat("é", 32769))},
				{"script", wrap(`<script>window.__serviceIconExecuted=true</script>`)},
				{"aliased mixed-case script", wrap(`<x:ScRiPt xmlns:x="urn:other"/>`)},
				{"hyperlink", wrap(`<a href="` + j.resources.URL + `/link">link</a>`)},
				{"foreignObject", wrap(`<foreignObject/>`)},
				{"root handler", `<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"/>`},
				{"namespaced handler", wrap(`<g xmlns:x="urn:other" x:OnLoad="alert(1)"/>`)},
				{"decoded script scheme", wrap(`<image href="java&#x09;script:alert(1)"/>`)},
				{"vbscript scheme", wrap(`<image href=" VbScRiPt:alert(1)"/>`)},
				{"internal subset", `<!DOCTYPE svg [<!ELEMENT svg ANY>]>` + wrap("")},
				{"external entity declaration", `<!DOCTYPE svg [<!ENTITY x SYSTEM "` + j.resources.URL + `/entity">]>` + wrap(`&x;`)},
				{"other directive", `<!ENTITY x "active">` + wrap("")},
				{"processing instruction", `<?xml-stylesheet href="` + j.resources.URL + `/style.css"?>` + wrap("")},
				{"unknown entity", wrap(`&unknown;`)}, {"duplicate attribute", wrap(`<g id="a" id="b"/>`)},
				{"multiple roots", wrap("") + wrap("")}, {"non SVG namespace", `<svg xmlns="urn:other"/>`},
			}
			discovery := map[string]any{"enable_discovery": true, "metadata_url": j.resources.URL + "/.well-known/oauth-authorization-server"}
			create := map[string]any{
				"display_name": "Discovery probe", "client_id": "icon-discovery-probe", "client_secret": "synthetic-probe-secret",
				"issuer_uri": j.resources.URL, "discovery": discovery, "icon_svg": invalid[0].source,
			}
			j.adminJSON(http.MethodPost, "/api/services", create, http.StatusBadRequest)
			listed, err := GetTestStorage().Services().List(j.ctx)
			Expect(err).NotTo(HaveOccurred())
			Expect(listed).To(HaveLen(len(j.services)), "rejected create must not add a service")
			for _, example := range invalid {
				By("rejecting " + example.name + " without partial metadata or credential changes")
				j.update(0, map[string]any{"icon_svg": example.source, "display_name": "Must not persist", "client_secret": "must-not-persist", "discovery": discovery}, http.StatusBadRequest)
				Expect(j.adminJSON(http.MethodGet, "/api/services/"+j.services[0].ID, nil, http.StatusOK)).To(Equal(before))
				stored, err = GetTestStorage().Services().Get(j.ctx, id.MustParseServiceID(j.services[0].ID))
				Expect(err).NotTo(HaveOccurred())
				storedAfter, err := json.Marshal(stored)
				Expect(err).NotTo(HaveOccurred())
				Expect(storedAfter).To(Equal(storedBefore))
				credentialAfter, err := stored.Secret.GetCiphertext()
				Expect(err).NotTo(HaveOccurred())
				Expect(bytes.Equal(credentialAfter, credentialBefore)).To(BeTrue(), "stored credential must not change")
			}
			Expect(j.discoveryLoads.Load()).To(BeZero(), "rejected writes must not fetch issuer metadata")
			create["icon_svg"] = j.visuals[0]
			j.adminJSON(http.MethodPost, "/api/services", create, http.StatusCreated)
			Expect(j.discoveryLoads.Load()).To(Equal(int64(1)), "valid input must reach the counted metadata endpoint")
			accepted := `<svg xmlns="http://www.w3.org/2000/svg" xmlns:x="http://www.w3.org/1999/xlink" width="64" height="64" viewBox="0 0 64 64"><metadata id="service-icon-artwork">Inert references</metadata><style>@import url('` + j.resources.URL + `/import.css'); @font-face {font-family:probe;src:url('` + j.resources.URL + `/font.woff')} .probe {fill:url('` + j.resources.URL + `/paint.svg#paint');background-image:url('` + j.resources.URL + `/background.png')}</style><rect width="64" height="64" fill="#279"/><image href="` + j.resources.URL + `/attribute.png" width="10" height="10"/><use x:href="` + j.resources.URL + `/reuse.svg#shape"/><text class="probe" style="font-family:probe" x="2" y="30">Inert</text><image id="animated" width="10" height="10"><animate attributeName="href" values="#local;` + j.resources.URL + `/animated.png;javascript:window.__serviceIconExecuted=true" dur=".1s" repeatCount="2"/></image></svg>`
			j.update(0, map[string]any{"icon_svg": accepted}, http.StatusOK)
			sources[0] = accepted
			j.expectAdminSource(0, accepted)
			j.refresh(sources)
			j.freshConnections(sources)
			j.expectAll(sources)
			j.expectIsolation()
			Expect(j.pageErrors.Load()).To(BeZero())
			j.screenshots("accepted_external_references_inert")
		})

		// US3.2 from specs/20261118-224349-configure-service-icons/spec.md (User Story 3).
		It("should retain original fallbacks and working controls after deterministic image errors [US3.2]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			j.failConnected(sources)
			for index, service := range j.services {
				Expect(j.connectionIcons.FailImage(pages.ServiceIconConnection, service.ID, service.Name, sources[index])).To(Succeed())
				expectServiceIconIdentity(j.connectionIcons, pages.ServiceIconConnection, service, "")
				if index == 1 {
					continue // Check this dialog when confirming termination below.
				}
				j.freshDetail(index, sources[index], func() error { return j.sessions.OpenSessionDetails(j.ctx, service.Name) })
				Expect(j.sessionIcons.FailImage(pages.ServiceIconDetail, service.ID, service.Name, sources[index])).To(Succeed())
				expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconDetail, service, "")
				Expect(j.sessions.TakeDetailScreenshot(j.ctx, "service_icons_failed_detail_"+service.Name)).To(Succeed())
				Expect(j.sessions.CancelSessionDetails(j.ctx)).To(Succeed())
			}
			j.screenshots("failed_original_fallback")
			Expect(j.consentIcons.HasPageError()).To(BeFalse())
			Expect(j.connectionIcons.HasPageError()).To(BeFalse())
			Expect(j.sessionIcons.HasPageError()).To(BeFalse())
			Expect(j.pageErrors.Load()).To(BeZero())
			j.connectAccounts()
			j.refresh(sources)
			service := j.services[1]
			j.freshDetail(1, sources[1], func() error { return j.sessions.OpenSessionDetails(j.ctx, service.Name) })
			Expect(j.sessionIcons.FailImage(pages.ServiceIconDetail, service.ID, service.Name, sources[1])).To(Succeed())
			expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconDetail, service, "")
			Expect(j.sessions.TakeDetailScreenshot(j.ctx, "service_icons_failed_detail_"+service.Name)).To(Succeed())
			page := j.sessions.GetPlaywrightPage()
			response, err := page.ExpectResponse(func(response playwright.Response) bool {
				target, err := url.Parse(response.URL())
				return err == nil && target.Path == "/api/third-party/sessions" && response.Request().Method() == http.MethodGet
			}, func() error { return j.sessions.ConfirmTermination(j.ctx, service.Name) })
			Expect(err).NotTo(HaveOccurred())
			Expect(response.Status()).To(Equal(http.StatusOK))
			var list struct {
				Data struct {
					Sessions []struct {
						ServiceID string `json:"service_id"`
					} `json:"sessions"`
				} `json:"data"`
			}
			Expect(response.JSON(&list)).To(Succeed())
			Expect(list.Data.Sessions).To(ConsistOf(HaveField("ServiceID", j.services[0].ID)))
			Expect(j.sessions.WaitForSuccessMessage(j.ctx, "Session terminated successfully.")).To(Succeed())
			Expect(j.sessions.GetTerminateButtonCount(j.ctx)).To(Equal(1))
			j.expectIsolation()
		})

		// US3.3 from specs/20261118-224349-configure-service-icons/spec.md (User Story 3).
		It("should recover changed source in open consent and session views without a full page reload [US3.3]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			j.failConnected(sources)
			Expect(j.consentIcons.RememberDocument()).To(Succeed())
			Expect(j.sessionIcons.RememberDocument()).To(Succeed())
			j.update(0, map[string]any{"icon_svg": j.visuals[1]}, http.StatusOK)
			sources[0] = j.visuals[1]
			j.refresh(sources)
			Expect(j.consent.IsSaveButtonEnabled(j.ctx)).To(BeTrue())
			j.expectConnected(sources)
			Expect(j.consentIcons.DocumentPreserved()).To(BeTrue(), "consent recovery must not reload the page")
			Expect(j.sessionIcons.DocumentPreserved()).To(BeTrue(), "session recovery must not reload the page")
			j.expectAdminSource(0, sources[0])
			j.freshDetail(0, sources[0], func() error { return j.sessions.OpenSessionDetails(j.ctx, j.services[0].Name) })
			expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconDetail, j.services[0], sources[0])
			Expect(j.sessions.TakeDetailScreenshot(j.ctx, "service_icons_recovered_current_detail")).To(Succeed())
			Expect(j.sessions.CancelSessionDetails(j.ctx)).To(Succeed())
			Expect(j.consent.TakeScreenshot(j.ctx, "service_icons_recovered_open_consent")).To(Succeed())
			Expect(j.sessions.TakeScreenshot(j.ctx, "service_icons_recovered_open_sessions")).To(Succeed())
			j.update(0, map[string]any{"icon_svg": nil}, http.StatusOK)
			sources[0] = ""
			j.refresh(sources)
			j.expectConnected(sources)
			Expect(j.consentIcons.DocumentPreserved()).To(BeTrue())
			Expect(j.sessionIcons.DocumentPreserved()).To(BeTrue())
		})

		// US3.4 from specs/20261118-224349-configure-service-icons/spec.md (User Story 3).
		It("should preserve decorative semantics readable names and keyboard actions at narrow and wide sizes [US3.4]", func() {
			sources := []string{j.visuals[0], j.visuals[0]}
			j.open(sources)
			for _, width := range []int{390, 1280} {
				for _, page := range []*pages.ServiceIconsPage{j.consentIcons, j.connectionIcons, j.sessionIcons} {
					Expect(page.GetPlaywrightPage().SetViewportSize(width, 900)).To(Succeed())
				}
				for _, state := range []string{"configured", "failed", "recovered", "unset"} {
					size := "wide"
					if width == 390 {
						size = "narrow"
					}
					switch state {
					case "failed":
						j.failConnected(sources)
						for index, service := range j.services {
							Expect(j.connectionIcons.FailImage(pages.ServiceIconConnection, service.ID, service.Name, sources[index])).To(Succeed())
						}
					case "recovered":
						replacement := j.visuals[1]
						if sources[0] == replacement {
							replacement = j.visuals[0]
						}
						for index := range j.services {
							j.update(index, map[string]any{"icon_svg": replacement}, http.StatusOK)
							sources[index] = replacement
						}
						j.refresh(sources)
						j.freshConnections(sources)
					case "unset":
						for index := range j.services {
							j.update(index, map[string]any{"icon_svg": nil}, http.StatusOK)
						}
						j.refresh([]string{"", ""})
						j.freshConnections([]string{"", ""})
					}
					expected := sources
					if state == "failed" || state == "unset" {
						expected = []string{"", ""}
					}
					j.expectConnected(expected)
					if state == "configured" {
						for index, surface := range []pages.ServiceIconSurface{pages.ServiceIconRequired, pages.ServiceIconOptional} {
							service := j.services[index]
							Expect(j.consentIcons.ChipIconFits(surface, service.ID, service.Name)).To(BeTrue(),
								"%s icon needs a 4px inset and unclipped corners at %dpx viewport width", service.Name, width)
						}
					}
					for index, service := range j.services {
						expectServiceIconIdentity(j.connectionIcons, pages.ServiceIconConnection, service, expected[index])
						for _, surface := range []pages.ServiceIconSurface{pages.ServiceIconConnection, pages.ServiceIconSession} {
							iconPage := j.connectionIcons
							if surface == pages.ServiceIconSession {
								iconPage = j.sessionIcons
							}
							aria, err := iconPage.IdentityAccessibility(surface, service.ID, service.Name)
							Expect(err).NotTo(HaveOccurred())
							Expect(aria).To(ContainSubstring(service.Name))
							iconAria, err := iconPage.IconAccessibility(surface, service.ID, service.Name)
							Expect(err).NotTo(HaveOccurred())
							Expect(iconAria).To(BeEmpty(), "decorative source must not add an accessibility node")
						}
						requirement := pages.ServiceIconRequired
						if index == 1 {
							requirement = pages.ServiceIconOptional
						}
						aria, err := j.consentIcons.IdentityAccessibility(requirement, service.ID, service.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(aria).To(ContainSubstring(service.Name))
						iconAria, err := j.consentIcons.IconAccessibility(requirement, service.ID, service.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(iconAria).To(BeEmpty())
						detailSource := sources[index]
						if state == "unset" {
							detailSource = ""
						}
						j.freshDetail(index, detailSource, func() error {
							return j.sessionIcons.KeyboardActivate(pages.ServiceIconSession, service.ID, service.Name, "Terminate")
						})
						if state == "failed" {
							Expect(j.sessionIcons.FailImage(pages.ServiceIconDetail, service.ID, service.Name, sources[index])).To(Succeed())
						}
						expectServiceIconIdentity(j.sessionIcons, pages.ServiceIconDetail, service, expected[index])
						aria, err = j.sessionIcons.IdentityAccessibility(pages.ServiceIconDetail, service.ID, service.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(aria).To(ContainSubstring(service.Name))
						iconAria, err = j.sessionIcons.IconAccessibility(pages.ServiceIconDetail, service.ID, service.Name)
						Expect(err).NotTo(HaveOccurred())
						Expect(iconAria).To(BeEmpty())
						Expect(j.sessions.TakeDetailScreenshot(j.ctx, "service_icons_"+state+"_"+size+"_detail_"+strings.ReplaceAll(strings.ToLower(service.Name), " ", "_"))).To(Succeed())
						Expect(j.sessionIcons.KeyboardActivate(pages.ServiceIconDetail, service.ID, service.Name, "Cancel session termination")).To(Succeed())
					}
					Expect(j.consentIcons.KeyboardActivate(pages.ServiceIconOptional, j.services[1].ID, j.services[1].Name, "Exclude Cobalt Notes")).To(Succeed())
					Expect(j.consent.IsPermissionSetServiceExcluded(j.ctx, "Original document access", j.services[1].Name)).To(BeTrue())
					Expect(j.consentIcons.KeyboardActivate(pages.ServiceIconOptional, j.services[1].ID, j.services[1].Name, "Include Cobalt Notes")).To(Succeed())
					Expect(j.consent.IsPermissionSetServiceExcluded(j.ctx, "Original document access", j.services[1].Name)).To(BeFalse())
					apiSources := sources
					if state == "unset" {
						apiSources = []string{"", ""}
					}
					Expect(j.consent.ExpectGrantSuccess(func() error {
						j.fresh(j.consent.GetPlaywrightPage(), "/api/consent/agents/"+j.agentID, j.consentIcons.KeyboardSaveConsent, apiSources)
						return nil
					})).To(Succeed())
					j.fresh(j.sessions.GetPlaywrightPage(), "/api/third-party/sessions", func() error {
						return j.sessionIcons.KeyboardActivate(pages.ServiceIconSession, j.services[0].ID, j.services[0].Name, "Refresh")
					}, apiSources)
					Expect(j.sessions.WaitForSuccessMessage(j.ctx, "Session token refreshed successfully.")).To(Succeed())
					if state == "failed" {
						j.failConnected(sources)
					} else {
						j.expectConnected(expected)
					}
					j.screenshots(state + "_" + size)
					if state == "unset" {
						service := j.services[1]
						j.freshDetail(1, "", func() error {
							return j.sessionIcons.KeyboardActivate(pages.ServiceIconSession, service.ID, service.Name, "Terminate")
						})
						Expect(j.sessionIcons.KeyboardActivate(pages.ServiceIconDetail, service.ID, service.Name, "Terminate session with "+service.Name)).To(Succeed())
						Expect(j.sessions.WaitForSuccessMessage(j.ctx, "Session terminated successfully.")).To(Succeed())
						Expect(GetTestStorage().UserSessions().Create(j.ctx, fixtures.SessionForServiceWithScopes(fixtures.DefaultPrincipal().String(), service.ID, []string{"read"}))).To(Succeed())
					}
				}
				for index := range j.services {
					j.update(index, map[string]any{"icon_svg": sources[index]}, http.StatusOK)
				}
				j.refresh(sources)
				j.freshConnections(sources)
			}
			for index, width := range []int{390, 1280} {
				service := j.services[index]
				Expect(j.connectionIcons.GetPlaywrightPage().SetViewportSize(width, 900)).To(Succeed())
				Expect(j.connectionIcons.KeyboardActivate(pages.ServiceIconConnection, service.ID, service.Name, "Login")).To(Succeed())
				Expect(j.connections.WaitForURL(j.ctx, "service_id="+service.ID)).To(Succeed())
				Expect(j.connections.WaitForPageLoad(j.ctx)).To(Succeed())
			}
		})
	})

	Context("when an operator creates original visual documents", func() {
		BeforeEach(func() { j.seed(j.visuals) })

		// US2.1 from specs/20261118-224349-configure-service-icons/spec.md (User Story 2).
		It("should round-trip and decode original visual SVGs as isolated images while remote avatars render [US2.1]", func() {
			for index, source := range j.visuals {
				j.expectAdminSource(index, source)
			}
			j.open(j.visuals)
			j.expectAll(j.visuals)
			Eventually(func() error { return j.consentIcons.WaitForRemoteAvatar(j.resources.URL + "/avatar.png") }).Should(Succeed())
			Expect(j.avatarLoads.Load()).To(BeNumerically(">", 0), "a different-origin avatar must actually load under the built SPA CSP")
			response, err := j.broker.PublicGET("/agents/" + j.agentID)
			Expect(err).NotTo(HaveOccurred())
			defer func() { Expect(response.Body.Close()).To(Succeed()) }()
			Expect(response.StatusCode).To(Equal(http.StatusOK))
			policy := response.Header.Get("Content-Security-Policy")
			Expect(policy).To(ContainSubstring("frame-ancestors 'none'"))
			Expect(policy).To(ContainSubstring("object-src 'none'"))
			Expect(policy).NotTo(ContainSubstring("img-src"))
			Expect(policy).NotTo(ContainSubstring("default-src"))
			j.expectIsolation()
			Expect(j.pageErrors.Load()).To(BeZero())
			j.screenshots("configured_original_visual_features")
		})
	})

	Context("when services have no icon metadata", func() {
		BeforeEach(func() { j.seed([]string{"", ""}) })

		// US3.1 from specs/20261118-224349-configure-service-icons/spec.md (User Story 3).
		It("should preserve initials name-only session identities and existing actions without an icon [US3.1]", func() {
			sources := []string{"", ""}
			j.open(sources)
			j.expectAll(sources)
			j.screenshots("unset_no_icon")
			j.connectAccounts()
			j.refresh(sources)
			Expect(j.consent.IsSaveButtonEnabled(j.ctx)).To(BeTrue())
			j.freshDetail(1, "", func() error { return j.sessions.OpenSessionDetails(j.ctx, j.services[1].Name) })
			Expect(j.sessions.TakeDetailScreenshot(j.ctx, "service_icons_unset_name_only_detail")).To(Succeed())
			Expect(j.sessions.ConfirmTermination(j.ctx, j.services[1].Name)).To(Succeed())
			Expect(j.sessions.WaitForSuccessMessage(j.ctx, "Session terminated successfully.")).To(Succeed())
			Expect(j.consentIcons.HasPageError()).To(BeFalse())
			Expect(j.sessionIcons.HasPageError()).To(BeFalse())
			j.expectIsolation()
		})
	})
})
