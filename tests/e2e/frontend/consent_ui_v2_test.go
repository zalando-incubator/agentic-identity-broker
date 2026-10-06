package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/pages"
	"github.com/mxschmitt/playwright-go"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consent UI v2", func() {
	var (
		ctx         context.Context
		f           *consentV2Scenario
		base        *pages.Page
		shell       *pages.ConsoleShell
		consent     *pages.ConsentPage
		delegations *pages.DelegationsPage
		approval    *pages.ApprovalPage
		connections *pages.ConnectionsPage
		queue       *pages.ApprovalsInboxPage
		settings    *pages.SettingsPage
		baseURL     string
	)

	BeforeEach(func() {
		base = nil
		ctx = context.Background()
		f = newConsentV2Scenario(ctx)
		baseURL = f.server.BaseURL()
		// These journeys use the production-served SPA, also when the outer suite uses Vite.
		Expect(GetTestContext().SetExtraHTTPHeaders(map[string]string{"X-Remote-User": f.principal.String()})).To(Succeed())
		base = pages.NewPage(GetTestPage(), baseURL)
		shell = pages.NewConsoleShell(GetTestPage(), baseURL)
		consent = pages.NewConsentPage(GetTestPage(), baseURL)
		delegations = pages.NewDelegationsPage(GetTestPage(), baseURL)
		approval = pages.NewApprovalPage(GetTestPage(), baseURL)
		connections = pages.NewConnectionsPage(GetTestPage(), baseURL)
		queue = pages.NewApprovalsInboxPage(GetTestPage(), baseURL)
		settings = pages.NewSettingsPage(GetTestPage(), baseURL)
		Expect(base.SetThemePreference(ctx, "system")).To(Succeed())
		Expect(base.EmulateColorScheme(ctx, "light")).To(Succeed())
		Expect(base.StartRequestRecorder(ctx)).To(Succeed())
		base.SetScreenshotContextFactory(consentV2ScreenshotContext)
	})

	AfterEach(func() {
		if base == nil {
			return
		}
		for _, request := range readV2(base.RecordedRequests(ctx)) {
			location := readV2(url.Parse(request.URL))
			Expect(request.Method == http.MethodGet && location.Path == "/api/approvals").To(BeFalse(), "no browser journey may use the gateway-wide long-poll")
			if strings.HasPrefix(location.Path, "/api/") {
				Expect(location.Path).NotTo(ContainSubstring(f.other.ID.String()))
				Expect(location.Path).NotTo(ContainSubstring(f.otherApproval.ID.String()))
				Expect(location.Path).NotTo(ContainSubstring(f.sessions[len(f.sessions)-1].ID.String()))
			}
		}
	})

	Context("Decide on an agent request", func() {
		// AS-01 from specs/047-redesign-consent-console/spec.md (User Story 1).
		It("AS-01 should explain access and trustworthy origin before Allow", func() {
			redirectHost := readV2(url.Parse(f.callbackURL)).Host
			for _, request := range []struct {
				agent          *storage.Agent
				origin, server string
				warning        bool
			}{
				{f.verified, "Verified domain: " + consentV2Host, baseURL, true},
				{f.localhost, "Unverified", f.localhostServer.BaseURL(), true},
				{f.compact, "Unverified", f.localhostServer.BaseURL(), true},
				{f.first, "", baseURL, false},
			} {
				requestStart := len(readV2(base.RecordedRequests(ctx)))
				startAuthorizationRequest(ctx, GetTestPage(), request.server, request.agent, f.callbackURL)
				if request.origin != "" {
					Expect(readV2(consent.PrimaryOriginLabelText(ctx))).To(Equal(request.origin))
				}
				returnOrigin := ""
				if request.agent != f.first {
					returnOrigin = "Returns to " + redirectHost
				}
				Expect(readV2(consent.ReturnOriginText(ctx))).To(Equal(returnOrigin))
				Expect(readV2(consent.HasLocalhostBanner(ctx))).To(Equal(request.warning))
				Expect(readV2(shell.HasSidebar(ctx))).To(BeFalse())
				groups := readV2(consent.PermissionGroups(ctx))
				Expect(groups).To(HaveLen(len(request.agent.PermissionSets)))
				for i, group := range groups {
					Expect(group.Required).To(Equal(i < 2))
					Expect(group.Name).To(Equal(f.sets[i].Name))
					Expect(group.Description).To(Equal(f.sets[i].Description))
					Expect(readV2(consent.GroupServices(ctx, group.Name))).To(ConsistOf(f.services[i%2].DisplayName))
					Expect(readV2(consent.GroupHasRiskRating(ctx, group.Name))).To(BeFalse())
					Expect(readV2(consent.GroupShowsScopeStrings(ctx, group.Name))).To(BeFalse())
					Expect(readV2(consent.HasServiceDisclosure(ctx, group.Name))).To(BeFalse(), "single-service group must have no disclosure")
				}
				Expect(readV2(consent.HasSignedInAs(ctx, f.principal.String()))).To(BeTrue())
				Expect(readV2(consent.DurationOptions(ctx))).To(ConsistOf("Until I revoke it", "30 days", "Custom date"))
				Expect(readV2(base.PrimaryAccentActionLabels(ctx))).To(Equal([]string{"Allow"}))
				Expect(readV2(consent.HasOutlinedDeny(ctx))).To(BeTrue())
				Expect(readV2(consent.AgentsManagementHref(ctx))).To(Equal("/agents"))
				Expect(strings.ToLower(readV2(base.VisibleText(ctx)))).NotTo(ContainSubstring("publisher"))
				expectNoThirdPartyOrGatewayRequests(request.server, readV2(base.RecordedRequests(ctx))[requestStart:])
			}
			Expect(base.TakeThemedScreenshots(ctx, "agent_consent")).To(Succeed())
			Expect(GetTestPage().SetViewportSize(1280, 720)).To(Succeed())
			startAuthorizationRequest(ctx, GetTestPage(), f.localhostServer.BaseURL(), f.compact, f.callbackURL)
			Expect(readV2(consent.PermissionGroups(ctx))).To(HaveLen(3))
			Expect(readV2(consent.HasLocalhostBanner(ctx))).To(BeTrue())
			Expect(readV2(consent.DecisionCardWidth(ctx))).To(BeNumerically("~", 480, 2))
			Expect(readV2(consent.AllowViewportBottom(ctx))).To(BeNumerically("<=", 720), "Allow must fit inside 1280 × 720 with three groups and risk")
			Expect(consentV2Grant(ctx, f.principal, f.compact)).To(BeNil())
			Expect(consentV2Grant(ctx, f.principal, f.first)).To(BeNil())
		})

		// AS-02 from specs/047-redesign-consent-console/spec.md (User Story 1).
		It("AS-02 should preserve prior access and validity during re-consent", func() {
			before := consentV2Grant(ctx, f.principal, f.delta)
			priorGroups := append([]storage.GrantedPermissionSetEntry(nil), before.GrantedPermissionSets...)
			priorValidity := *before.ValidUntil
			startReauthorizationRequest(ctx, GetTestPage(), f, f.delta)
			groups := readV2(consent.PermissionGroups(ctx))
			Expect(groups).To(HaveLen(4))
			for _, group := range groups {
				if group.Name == f.sets[3].Name {
					Expect(group.AlreadyGranted).To(BeFalse())
					Expect(group.ReadOnly).To(BeFalse())
				} else {
					Expect(group.AlreadyGranted).To(BeTrue())
					Expect(group.Checked).To(BeTrue())
					Expect(group.ReadOnly).To(BeTrue())
					Expect(group.Expanded).To(BeFalse())
				}
			}
			Expect(readV2(consent.SelectedDuration(ctx))).To(Equal("Custom date"))
			Expect(readV2(consent.CustomDateValue(ctx))).To(Equal(before.ValidUntil.Format("2006-01-02")))
			Expect(consent.ClickAllow(ctx)).To(Succeed())
			Eventually(func() string { return GetTestPage().URL() }).Should(HavePrefix(f.callbackURL + "?"))
			unchanged := consentV2Grant(ctx, f.principal, f.delta)
			Expect(unchanged.GrantedPermissionSets).To(ConsistOf(priorGroups), "continuing without selection must not widen prior access")
			Expect(unchanged.ValidUntil).To(Equal(&priorValidity), "unchanged consent must preserve the grant validity")
			startReauthorizationRequest(ctx, GetTestPage(), f, f.delta)
			Expect(readV2(consent.SelectedDuration(ctx))).To(Equal("Custom date"))
			Expect(readV2(consent.CustomDateValue(ctx))).To(Equal(priorValidity.Format("2006-01-02")))
			Expect(consent.SetPermissionGroupChecked(ctx, f.sets[3].Name, true)).To(Succeed())
			Expect(consent.ClickAllow(ctx)).To(Succeed())
			Eventually(func() string { return GetTestPage().URL() }).Should(HavePrefix(f.callbackURL + "?"))
			after := consentV2Grant(ctx, f.principal, f.delta)
			expected := append(priorGroups, storage.GrantedPermissionSetEntry{PermissionSetID: f.sets[3].ID, IncludedServiceIDs: []id.ServiceID{f.services[1].ID}})
			Expect(after.GrantedPermissionSets).To(ConsistOf(expected))
			Expect(after.ValidUntil).To(Equal(&priorValidity))
		})

		Context("with a required provider callback and an expired-session response", func() {
			BeforeEach(func() {
				Expect(GetTestStorage().UserSessions().DeleteByPrincipalAndService(ctx, f.principal, f.services[1].ID)).To(Succeed())
			})

			// AS-03 from specs/047-redesign-consent-console/spec.md (User Story 1).
			// Expiry is the documented HTTP failure boundary, not simulated backend time.
			It("AS-03 should resume safely, deny locally, and handle session_expired", func() {
				startAuthorizationRequest(ctx, GetTestPage(), baseURL, f.first, f.callbackURL)
				Expect(consent.ChooseDuration(ctx, "Custom date")).To(Succeed())
				Expect(consent.SetCustomDate(ctx, "2099-11-06")).To(Succeed())
				Expect(consent.SetPermissionGroupChecked(ctx, f.sets[2].Name, true)).To(Succeed())
				Expect(consent.SetPermissionGroupChecked(ctx, f.sets[3].Name, false)).To(Succeed())
				Expect(readV2(base.PrimaryAccentActionLabels(ctx))).To(Equal([]string{"Connect " + f.services[1].DisplayName + " to continue"}))
				Expect(consent.ClickDecisionConnect(ctx, f.services[1].DisplayName)).To(Succeed())
				Eventually(func() string { return GetTestPage().URL() }).Should(ContainSubstring("/agents/" + f.first.ID.String()))
				Eventually(func() *storage.UserSession {
					return readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[1].ID))
				}).ShouldNot(BeNil())
				consentV2ExpectProviderCallback(readV2(base.RecordedRequests(ctx)), f.services[1].ID)
				Expect(readV2(consent.SelectedDuration(ctx))).To(Equal("Custom date"))
				Expect(readV2(consent.CustomDateValue(ctx))).To(Equal("2099-11-06"))
				consentV2ExpectPermissionSelection(ctx, consent, f.sets[0].Name, f.sets[1].Name, f.sets[2].Name)
				Expect(consent.ClickAllow(ctx)).To(Succeed())
				Eventually(func() string { return GetTestPage().URL() }).Should(HavePrefix(f.callbackURL + "?"))
				callback := readV2(url.Parse(GetTestPage().URL()))
				Expect(callback.Query().Get("code")).NotTo(BeEmpty())
				Expect(callback.Query().Get("state")).To(Equal("consent-v2-state"))
				grant := consentV2Grant(ctx, f.principal, f.first)
				Expect(grant.ValidUntil.Format("2006-01-02")).To(Equal("2099-11-06"))
				Expect(grant.GrantedPermissionSets).To(ConsistOf(fixtures.ConsentV2Grant(f.principal, f.first, f.sets[0], f.sets[1], f.sets[2]).GrantedPermissionSets))

				before := consentV2GrantSnapshot(ctx, f.principal, f.delta)
				decisionPath := startReauthorizationRequest(ctx, GetTestPage(), f, f.delta)
				requestStart := len(readV2(base.RecordedRequests(ctx)))
				Expect(consent.ClickDeny(ctx)).To(Succeed())
				Expect(strings.ToLower(readV2(consent.DecisionOutcomeText(ctx)))).To(ContainSubstring("denied"))
				Expect(GetTestPage().URL()).To(Equal(baseURL + decisionPath))
				Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
				for _, request := range readV2(base.RecordedRequests(ctx))[requestStart:] {
					Expect(request.Method).To(Equal(http.MethodGet), "Deny must neither create nor revoke a grant")
				}

				// A genuine issued token receives the production session_expired wire
				// contract. Backend cryptographic/TTL validation has separate coverage.
				Expect(GetTestPage().Route("**/api/consent/agents/"+f.delta.ID.String()+"?*", func(route playwright.Route) {
					defer GinkgoRecover()
					Expect(route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(http.StatusBadRequest), ContentType: playwright.String("application/json"), Body: `{"error":"session_expired","message":"authorization session has expired, please restart the authorization flow"}`})).To(Succeed())
				})).To(Succeed())
				Expect(base.Navigate(ctx, decisionPath)).To(Succeed())
				Expect(strings.ToLower(readV2(consent.DecisionErrorText(ctx)))).To(ContainSubstring("expired"))
				Expect(readV2(shell.HasSidebar(ctx))).To(BeFalse())
				Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
				// Presence of session_token, even when empty, keeps the focused decision
				// context rather than falling through to the console grant editor.
				Expect(base.Navigate(ctx, "/agents/"+f.delta.ID.String()+"?session_token=")).To(Succeed())
				Expect(readV2(shell.HasSidebar(ctx))).To(BeFalse())
				Expect(strings.ToLower(readV2(consent.DecisionErrorText(ctx)))).To(ContainSubstring("expired"))
				Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
			})
		})
	})

	Context("Review a tool call", func() {
		// AS-04 from specs/047-redesign-consent-console/spec.md (User Story 2).
		It("AS-04 should show exact context before any tool decision", func() {
			for _, item := range []*storage.ToolApproval{f.pending, f.unrated} {
				Expect(base.Navigate(ctx, "/approvals/"+item.ID.String())).To(Succeed())
				Expect(readV2(approval.ToolName(ctx))).To(Equal(item.ToolName))
				Expect(readV2(approval.AgentName(ctx))).To(Equal(f.delta.DisplayName))
				Expect(readV2(approval.ActingUser(ctx))).To(Equal(f.principal.String()))
				risk := "High"
				if item.RiskLevel == "" {
					risk = "Risk not rated"
				}
				Expect(readV2(approval.RiskLabel(ctx))).To(Equal(risk))
				Expect(readV2(approval.ArgumentRows(ctx))).To(Equal(map[string]string{"path": "/projects/roadmap", "recursive": "true"}), "two-column tool inputs are shown before any disclosure")
				Expect(approval.OpenArgumentsJSON(ctx)).To(Succeed())
				var args map[string]any
				Expect(json.Unmarshal([]byte(readV2(approval.ArgumentsText(ctx))), &args)).To(Succeed())
				Expect(args).To(Equal(item.Arguments))
				Expect(readV2(approval.ArgumentsAreMonospace(ctx))).To(BeTrue())
				Expect(approval.CloseArgumentsJSON(ctx)).To(Succeed())
				Expect(readV2(approval.ApprovalScopeText(ctx))).To(Equal(consentV2Scope(item)))
				Expect(readV2(approval.HasApproveOptionsAndDeny(ctx))).To(BeTrue())
				Expect(readV2(shell.HasSidebar(ctx))).To(BeFalse())
			}
			Expect(base.Navigate(ctx, "/approvals/"+f.pending.ID.String())).To(Succeed())
			Expect(base.TakeThemedScreenshots(ctx, "approval_review")).To(Succeed())
		})

		// AS-05 from specs/047-redesign-consent-console/spec.md (User Story 2).
		It("AS-05 should isolate decisions and stop offering resolved actions", func() {
			for i, item := range f.decisions {
				func() {
					browserContext, page := newConsentV2Browser(f.principal)
					defer func() { Expect(browserContext.Close()).To(Succeed()) }()
					review := pages.NewApprovalPage(page, baseURL)
					before := consentV2ApprovalSnapshot(ctx, f, item.ID)
					Expect(review.Navigate(ctx, "/approvals/"+item.ID.String())).To(Succeed())
					Expect(readV2(review.HasDecisionActions(ctx))).To(BeTrue())
					status, persistence := storage.ApprovalStatusApproved, storage.ApprovalPersistenceOnce
					switch i {
					case 0:
						Expect(review.ClickApproveOnce(ctx)).To(Succeed())
					case 1, 2:
						label := "For this session"
						persistence = storage.ApprovalPersistenceSession
						if i == 2 {
							label, persistence = "Always…", storage.ApprovalPersistencePermanent
						}
						Expect(review.OpenApproveOptions(ctx)).To(Succeed())
						Expect(review.ChooseRememberDuration(ctx, label)).To(Succeed())
						Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, item.ID)).Status).To(Equal(storage.ApprovalStatusPending))
						Expect(review.ConfirmRememberedApproval(ctx)).To(Succeed())
					case 3:
						status = storage.ApprovalStatusDenied
						Expect(review.ClickDeny(ctx)).To(Succeed())
					}
					if status == storage.ApprovalStatusDenied {
						consentV2EventuallyDecision(ctx, item, status, nil)
					} else {
						consentV2EventuallyDecision(ctx, item, status, &persistence)
					}
					Expect(consentV2ApprovalSnapshot(ctx, f, item.ID)).To(Equal(before), "another approval must not change")
					Expect(strings.ToLower(readV2(review.ResolvedOutcomeText(ctx)))).To(ContainSubstring(string(status)))
					Expect(readV2(review.HasDecisionActions(ctx))).To(BeFalse())
				}()
			}
			for _, item := range []*storage.ToolApproval{f.approved, f.denied, f.expiredApproval} {
				Expect(base.Navigate(ctx, "/approvals/"+item.ID.String())).To(Succeed())
				outcome := string(item.Status)
				if item == f.expiredApproval {
					outcome = "expired"
				}
				Expect(strings.ToLower(readV2(approval.ResolvedOutcomeText(ctx)))).To(ContainSubstring(outcome))
				Expect(readV2(approval.HasDecisionActions(ctx))).To(BeFalse())
			}
			Expect(base.Navigate(ctx, "/approvals/"+f.concurrent.ID.String())).To(Succeed())
			Expect(readV2(approval.HasDecisionActions(ctx))).To(BeTrue())
			secondContext, secondPage := newConsentV2Browser(f.principal)
			defer func() { Expect(secondContext.Close()).To(Succeed()) }()
			secondReview := pages.NewApprovalPage(secondPage, baseURL)
			Expect(secondReview.Navigate(ctx, "/approvals/"+f.concurrent.ID.String())).To(Succeed())
			Expect(secondReview.ClickApproveOnce(ctx)).To(Succeed())
			persistence := storage.ApprovalPersistenceOnce
			consentV2EventuallyDecision(ctx, f.concurrent, storage.ApprovalStatusApproved, &persistence)
			consentV2Reload(GetTestPage())
			Expect(strings.ToLower(readV2(approval.ResolvedOutcomeText(ctx)))).To(ContainSubstring("approved"))
			Expect(readV2(approval.HasDecisionActions(ctx))).To(BeFalse())
		})
	})

	Context("Find and revoke an agent", func() {
		// AS-06 from specs/047-redesign-consent-console/spec.md (User Story 3), SC-005.
		It("AS-06 should link searchable agent cards and show compact list above twelve", func() {
			Expect(delegations.Navigate(ctx)).To(Succeed())
			Expect(readV2(delegations.View(ctx))).To(Equal("list"), "more than twelve active grants default to list")
			rows := readV2(delegations.Rows(ctx))
			for i, agent := range f.delegations {
				Expect(consentV2RowsContain(rows, agent.DisplayName)).To(BeTrue())
				for _, row := range rows {
					if row.Agent == agent.DisplayName {
						if i%2 == 0 {
							Expect(row.Expiry).To(Equal("Until revoked"))
						} else {
							Expect(row.Expiry).To(ContainSubstring("2099"))
						}
					}
				}
			}
			Expect(consentV2RowsContain(rows, f.expired.DisplayName)).To(BeFalse())
			Expect(consentV2RowsContain(rows, f.other.DisplayName)).To(BeFalse())
			Expect(delegations.ChooseView(ctx, "grid")).To(Succeed())
			Expect(readV2(delegations.View(ctx))).To(Equal("grid"))
			Expect(readV2(delegations.CardCount(ctx))).To(Equal(len(rows)))
			Expect(readV2(delegations.Rows(ctx))).To(ConsistOf(rows))
			Expect(delegations.ChooseView(ctx, "list")).To(Succeed())
			Expect(readV2(delegations.Rows(ctx))).To(ConsistOf(rows))
			Expect(base.TakeThemedScreenshots(ctx, "agents")).To(Succeed())
			Expect(delegations.Sort(ctx, "Recently changed")).To(Succeed())
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("sort")).To(Equal("recent"))
			Expect(delegations.FocusSearchShortcut(ctx)).To(Succeed())
			Expect(delegations.Search(ctx, "assistant 07")).To(Succeed())
			Expect(readV2(delegations.Rows(ctx))).To(HaveLen(1))
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("q")).To(Equal("assistant 07"))
			consentV2Reload(GetTestPage())
			Expect(readV2(delegations.Rows(ctx))).To(HaveLen(1), "a linked query survives reload")
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("sort")).To(Equal("recent"))
			Expect(delegations.ClearSearchWithEscape(ctx)).To(Succeed())
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("q")).To(BeEmpty())
			Eventually(func() []pages.DelegationsRow { return readV2(delegations.Rows(ctx)) }).Should(HaveLen(len(rows)))
			Expect(delegations.Search(ctx, "assistant 07")).To(Succeed())
			Expect(delegations.ChooseView(ctx, "grid")).To(Succeed())
			Expect(readV2(delegations.CardCount(ctx))).To(Equal(1))
			for _, request := range readV2(base.RecordedRequests(ctx)) {
				path := readV2(url.Parse(request.URL)).Path
				Expect(path).NotTo(MatchRegexp(`/api/consent/agents/[^/]+(?:/grants)?$`), "list must not make per-agent count requests")
			}
			Expect(delegations.ClickCard(ctx, f.delegations[6].DisplayName)).To(Succeed())
			Eventually(func() string { return GetTestPage().URL() }).Should(Equal(baseURL + "/agents/" + f.delegations[6].ID.String()))
			Expect(delegations.Navigate(ctx)).To(Succeed())
			before := consentV2GrantSnapshot(ctx, f.principal, f.delegations[6])
			Expect(delegations.ClickRevoke(ctx, f.delegations[6].DisplayName)).To(Succeed())
			Expect(readV2(delegations.RevokeDialogText(ctx))).To(ContainSubstring(f.delegations[6].DisplayName))
			Expect(consentV2GrantSnapshot(ctx, f.principal, f.delegations[6])).To(Equal(before))
			Expect(delegations.CancelRevoke(ctx)).To(Succeed())
			// A new browser with fewer grants must initially show the card grid.
			for _, agent := range f.delegations[5:] {
				grant := consentV2Grant(ctx, f.principal, agent)
				Expect(GetTestStorage().UserGrants().Delete(ctx, grant.ID)).To(Succeed())
			}
			browserContext, page := newConsentV2Browser(f.principal)
			defer func() { Expect(browserContext.Close()).To(Succeed()) }()
			shortList := pages.NewDelegationsPage(page, baseURL)
			Expect(shortList.Navigate(ctx)).To(Succeed())
			Expect(readV2(shortList.View(ctx))).To(Equal("grid"))
			Expect(readV2(shortList.CardCount(ctx))).To(BeNumerically("<=", 12))
		})

		// AS-07 from specs/047-redesign-consent-console/spec.md (User Story 3).
		It("AS-07 should rollback only the failed revoke and explain emptiness", func() {
			Expect(delegations.Navigate(ctx)).To(Succeed())
			first, second := f.delegations[0], f.delegations[1]
			blocked, release, routeResult := make(chan struct{}), make(chan struct{}), make(chan error, 1)
			var once sync.Once
			defer once.Do(func() { close(release) })
			Expect(GetTestPage().Route("**/api/consent/agents/"+first.ID.String()+"/grants", func(route playwright.Route) {
				defer GinkgoRecover()
				if route.Request().Method() != http.MethodDelete {
					Expect(route.Continue()).To(Succeed())
					return
				}
				close(blocked)
				go func() {
					<-release
					routeResult <- route.Fulfill(playwright.RouteFulfillOptions{Status: playwright.Int(503), ContentType: playwright.String("application/json"), Body: `{"error":"unavailable","message":"Revoke could not be saved"}`})
				}()
			})).To(Succeed())
			Expect(delegations.ClickRevoke(ctx, first.DisplayName)).To(Succeed())
			Expect(delegations.ConfirmRevoke(ctx)).To(Succeed())
			Eventually(blocked).Should(BeClosed())
			Eventually(func() bool { return readV2(delegations.RowIsPending(ctx, first.DisplayName)) }).Should(BeTrue())
			Expect(delegations.ClickRevoke(ctx, second.DisplayName)).To(Succeed())
			Expect(delegations.ConfirmRevoke(ctx)).To(Succeed())
			Eventually(func() bool { return consentV2RowsContain(readV2(delegations.Rows(ctx)), second.DisplayName) }).Should(BeFalse())
			Expect(strings.ToLower(readV2(shell.LiveRegionText(ctx)))).To(ContainSubstring("revok"))
			once.Do(func() { close(release) })
			Eventually(routeResult).Should(Receive(BeNil()))
			Eventually(func(g Gomega) {
				pending, err := delegations.RowIsPending(ctx, first.DisplayName)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(pending).To(BeFalse(), "failed revoke must leave its row actionable again")
				text, err := base.VisibleText(ctx)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(text).To(ContainSubstring("Could not revoke access. Try again."))
			}).Should(Succeed())
			Eventually(func() bool { return consentV2RowsContain(readV2(delegations.Rows(ctx)), first.DisplayName) }).Should(BeTrue())
			Expect(consentV2RowsContain(readV2(delegations.Rows(ctx)), second.DisplayName)).To(BeFalse(), "failed row rollback must not restore a different successful revoke")
			Expect(consentV2Grant(ctx, f.principal, first)).NotTo(BeNil())
			Expect(consentV2Grant(ctx, f.principal, second)).To(BeNil())
			for _, grant := range readV2(GetTestStorage().UserGrants().ListByPrincipal(ctx, f.principal)) {
				Expect(GetTestStorage().UserGrants().Delete(ctx, grant.ID)).To(Succeed())
			}
			consentV2Reload(GetTestPage())
			explanation := readV2(delegations.EmptyStateText(ctx))
			Expect(strings.ToLower(explanation)).To(And(ContainSubstring("agent"), ContainSubstring("access")))
		})
	})

	Context("Change an agent grant", func() {
		BeforeEach(func() { Expect(base.Navigate(ctx, "/agents/"+f.delta.ID.String())).To(Succeed()) })

		// AS-08 from specs/047-redesign-consent-console/spec.md (User Story 4).
		It("AS-08 should save only intentional optional permission edits", func() {
			Expect(readV2(shell.HasSidebar(ctx))).To(BeTrue())
			Expect(readV2(consent.HasOriginLabel(ctx))).To(BeFalse())
			Expect(readV2(base.VisibleText(ctx))).To(ContainSubstring(f.delta.DisplayName))
			links := readV2(consent.IdentityLinks(ctx))
			for _, target := range []*string{f.delta.GovernanceURL, f.delta.UserDocumentationURL, f.delta.AgentInterfaceURL} {
				// A root URL serializes with a trailing slash; that is the same external destination.
				expected := readV2(url.Parse(*target))
				if expected.Path == "" {
					expected.Path = "/"
				}
				Expect(links).To(ContainElement(HaveField("URL", Equal(expected.String()))))
			}
			Expect(readV2(consent.ConnectionsRailRows(ctx))).To(HaveLen(2))
			Expect(readV2(consent.HasDetailTabs(ctx))).To(BeFalse(), "permissions and Connections rail share the detail page")
			for _, group := range readV2(consent.PermissionGroups(ctx)) {
				Expect(group.ReadOnly).To(Equal(group.Required))
			}
			Expect(readV2(consent.IsSaveBarVisible(ctx))).To(BeFalse())
			Expect(base.TakeThemedScreenshots(ctx, "agent_detail")).To(Succeed())
			before := consentV2GrantSnapshot(ctx, f.principal, f.delta)
			Expect(consent.SetPermissionGroupChecked(ctx, f.sets[2].Name, false)).To(Succeed())
			Expect(readV2(consent.IsSaveBarVisible(ctx))).To(BeTrue())
			Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
			Expect(consent.CancelChanges(ctx)).To(Succeed())
			consentV2ExpectPermissionSelection(ctx, consent, f.sets[0].Name, f.sets[1].Name, f.sets[2].Name)
			Expect(readV2(consent.IsSaveBarVisible(ctx))).To(BeFalse())
			Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
			Expect(consent.SetPermissionGroupChecked(ctx, f.sets[2].Name, false)).To(Succeed())
			Expect(consent.SaveChanges(ctx)).To(Succeed())
			Eventually(func() []storage.GrantedPermissionSetEntry {
				return consentV2Grant(ctx, f.principal, f.delta).GrantedPermissionSets
			}).Should(ConsistOf(fixtures.ConsentV2Grant(f.principal, f.delta, f.sets[0], f.sets[1]).GrantedPermissionSets))
			Expect(readV2(consent.IsSaveBarVisible(ctx))).To(BeFalse())
		})

		// AS-09 from specs/047-redesign-consent-console/spec.md (User Story 4).
		It("AS-09 should confirm revocation without touching another principal", func() {
			before := consentV2GrantSnapshot(ctx, f.principal, f.delta)
			otherBefore := consentV2GrantSnapshot(ctx, f.otherPrincipal, f.delta)
			Expect(consent.ClickRevokeButton(ctx)).To(Succeed())
			Expect(readV2(consent.RevokeCancelHasFocus(ctx))).To(BeTrue(), "Cancel receives focus before any destructive confirmation")
			Expect(readV2(consent.RevokeDialogText(ctx))).To(And(ContainSubstring(f.delta.DisplayName), ContainSubstring("access")))
			Expect(consent.CancelRevoke(ctx)).To(Succeed())
			Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(before))
			Expect(consent.ClickRevokeButton(ctx)).To(Succeed())
			Expect(consent.ConfirmRevoke(ctx)).To(Succeed())
			Eventually(func() *storage.UserGrant { return consentV2Grant(ctx, f.principal, f.delta) }).Should(BeNil())
			Expect(consentV2GrantSnapshot(ctx, f.otherPrincipal, f.delta)).To(Equal(otherBefore))
		})
	})

	Context("Manage connections", func() {
		BeforeEach(func() { Expect(base.Navigate(ctx, "/connections")).To(Succeed()) })

		// AS-10 from specs/047-redesign-consent-console/spec.md (User Story 5).
		It("AS-10 should distinguish stored connections from missing requirements", func() {
			rows := readV2(connections.ConnectionRows(ctx))
			Expect(rows).To(HaveLen(5))
			Expect(readV2(connections.View(ctx))).To(Equal("grid"), "five stored connections use cards by default")
			for i, state := range []string{"Connected", "Connected", "Expired", "Needs sign-in", "Needs sign-in"} {
				row := consentV2Connection(rows, f.services[i].DisplayName)
				Expect(row.ScopeCount).To(Equal(2))
				Expect(row.State).To(Equal(state))
				Expect(row.CreatedAt).To(Equal(fixtures.ConsentV2Clock().Format(time.RFC3339)))
				action := "Disconnect"
				if i == 2 {
					action = "Reconnect"
				} else if i > 2 {
					action = "Refresh"
				}
				Expect(row.Action).To(ContainSubstring(action))
			}
			Expect(strings.ToLower(readV2(base.VisibleText(ctx)))).NotTo(ContainSubstring("session"), "connection copy must not revert to session wording")
			Expect(readV2(connections.ConnectionScopes(ctx, f.services[0].DisplayName))).To(ConsistOf("read", "write"))
			Expect(readV2(connections.ConnectionScopes(ctx, f.services[2].DisplayName))).To(ConsistOf("read", "write"), "expired scopes come from its own stored connection")
			Expect(connections.ChooseView(ctx, "list")).To(Succeed())
			Expect(readV2(connections.ConnectionRows(ctx))).To(ConsistOf(rows))
			Expect(connections.ChooseView(ctx, "grid")).To(Succeed())
			Expect(connections.FilterState(ctx, "Expired")).To(Succeed())
			Expect(readV2(connections.ConnectionRows(ctx))).To(HaveLen(1))
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("state")).To(Equal("expired"))
			Expect(readV2(connections.ActiveFilterChips(ctx))).To(ContainElement(ContainSubstring("Expired")))
			consentV2Reload(GetTestPage())
			Expect(readV2(connections.ConnectionRows(ctx))).To(HaveLen(1), "state-filtered connection links survive reload")
			Expect(connections.FilterState(ctx, "All")).To(Succeed())
			Expect(readV2(url.Parse(GetTestPage().URL())).Query().Get("state")).To(BeEmpty())
			Expect(readV2(connections.ConnectionRows(ctx))).To(HaveLen(5))
			Expect(base.TakeThemedScreenshots(ctx, "connections")).To(Succeed())
			Expect(readV2(base.VisibleText(ctx))).NotTo(ContainSubstring("Missing scopes"))
			refreshStatus := make(chan int, 1)
			GetTestPage().OnResponse(func(response playwright.Response) {
				if response.URL() == baseURL+"/api/third-party/"+f.services[4].ID.String()+"/session/refresh" {
					select {
					case refreshStatus <- response.Status():
					default:
					}
				}
			})
			Expect(connections.ClickRefresh(ctx, f.services[4].DisplayName)).To(Succeed())
			Eventually(refreshStatus).Should(Receive(Equal(http.StatusBadGateway)), "the rejecting provider must exercise the real 502 refresh contract")
			Eventually(func(g Gomega) {
				row := consentV2Connection(readV2(connections.ConnectionRows(ctx)), f.services[4].DisplayName)
				g.Expect(row.State).To(Equal("Needs sign-in"))
				g.Expect(row.Action).To(ContainSubstring("Reconnect"))
			}).Should(Succeed())
			Expect(base.Navigate(ctx, "/agents/"+f.missing.ID.String())).To(Succeed())
			Expect(readV2(consent.ConnectionsRailRows(ctx))).To(ContainElement(pages.AgentConnectionRow{Service: f.services[5].DisplayName, State: "No connection", Action: "Connect"}))
			Expect(readV2(base.VisibleText(ctx))).NotTo(ContainSubstring("Missing scopes"))
		})

		// AS-11 from specs/047-redesign-consent-console/spec.md (User Story 5).
		It("AS-11 should reconnect refresh and disconnect with truthful warnings", func() {
			Expect(connections.ClickReconnect(ctx, f.services[2].DisplayName)).To(Succeed())
			Eventually(func() string { return GetTestPage().URL() }).Should(Equal(baseURL + "/connections"))
			Eventually(func() string {
				return consentV2Connection(readV2(connections.ConnectionRows(ctx)), f.services[2].DisplayName).State
			}).Should(Equal("Connected"))
			consentV2ExpectProviderCallback(readV2(base.RecordedRequests(ctx)), f.services[2].ID)
			Expect(connections.WaitForSuccessMessage(ctx, "Service connected.")).To(Succeed())
			reconnected := readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[2].ID))
			Expect(reconnected.AccessTokenExpiresAt.After(time.Now())).To(BeTrue())
			Expect(connections.ClickRefresh(ctx, f.services[3].DisplayName)).To(Succeed())
			Eventually(func() string {
				return consentV2Connection(readV2(connections.ConnectionRows(ctx)), f.services[3].DisplayName).State
			}).Should(Equal("Connected"))
			refreshed := readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[3].ID))
			Expect(refreshed.AccessTokenExpiresAt.After(time.Now())).To(BeTrue())
			Expect(connections.WaitForSuccessMessage(ctx, "Connection refreshed.")).To(Succeed())
			grantBefore := consentV2GrantSnapshot(ctx, f.principal, f.delta)
			otherBefore := string(readV2(json.Marshal(readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.otherPrincipal, f.services[6].ID)))))
			Expect(connections.ClickDisconnect(ctx, f.services[0].DisplayName)).To(Succeed())
			warning := readV2(connections.DisconnectDialogText(ctx))
			Expect(warning).To(ContainSubstring(f.delta.DisplayName))
			Expect(strings.ToLower(warning)).To(And(ContainSubstring("does not revoke"), ContainSubstring("provider"), ContainSubstring("token")))
			Expect(readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[0].ID))).NotTo(BeNil())
			Expect(connections.ConfirmDisconnect(ctx)).To(Succeed())
			Eventually(func() *storage.UserSession {
				return readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.principal, f.services[0].ID))
			}).Should(BeNil())
			Expect(connections.WaitForSuccessMessage(ctx, "Connection disconnected.")).To(Succeed())
			Expect(consentV2GrantSnapshot(ctx, f.principal, f.delta)).To(Equal(grantBefore))
			Expect(string(readV2(json.Marshal(readV2(GetTestStorage().UserSessions().FindByPrincipalAndService(ctx, f.otherPrincipal, f.services[6].ID)))))).To(Equal(otherBefore))
		})
	})

	Context("Triage approvals", func() {
		BeforeEach(func() { Expect(base.Navigate(ctx, "/approvals")).To(Succeed()) })

		// AS-12 from specs/047-redesign-consent-console/spec.md (User Story 6).
		It("AS-12 should keep inbox bounds stable through decisions, shortcuts, and remembered revocation", func() {
			initial := readV2(queue.PendingRows(ctx))
			Expect(consentV2PendingContains(initial, f.pending.ToolName)).To(BeTrue())
			bounds := readV2(queue.PendingListBounds(ctx))
			Expect(bounds).To(HaveLen(4))
			Expect(base.TakeThemedScreenshots(ctx, "approvals")).To(Succeed())

			// Keyboard selection and panel focus do not alter the pending list's box.
			middle := initial[len(initial)/2].Tool
			Expect(queue.SelectPendingRow(ctx, middle)).To(Succeed())
			Expect(queue.PressInboxKey(ctx, "j")).To(Succeed())
			Expect(readV2(queue.SelectedTool(ctx))).NotTo(Equal(middle))
			Expect(queue.PressInboxKey(ctx, "k")).To(Succeed())
			Expect(readV2(queue.SelectedTool(ctx))).To(Equal(middle))
			Expect(queue.PressInboxKey(ctx, "Enter")).To(Succeed())
			Expect(readV2(queue.PanelHasFocus(ctx))).To(BeTrue())
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds))

			before := consentV2ApprovalSnapshot(ctx, f, f.pending.ID)
			rowBounds := readV2(queue.PendingRowBounds(ctx, f.pending.ToolName))
			Expect(queue.SelectPendingRow(ctx, f.pending.ToolName)).To(Succeed())
			Expect(queue.OpenApproveOptions(ctx)).To(Succeed())
			Expect(queue.ChooseRememberDuration(ctx, "For this session")).To(Succeed())
			Expect(readV2(queue.ApprovalScopeText(ctx))).To(Equal(consentV2Scope(f.pending)))
			Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, f.pending.ID)).Status).To(Equal(storage.ApprovalStatusPending))
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds), "scope editor must replace panel content, never expand the list")
			Expect(readV2(queue.PendingRowBounds(ctx, f.pending.ToolName))).To(Equal(rowBounds), "persistence editing cannot resize a pending row")
			Expect(queue.BackFromScopeEditor(ctx)).To(Succeed())
			Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, f.pending.ID)).Status).To(Equal(storage.ApprovalStatusPending))
			Expect(queue.OpenApproveOptions(ctx)).To(Succeed())
			Expect(queue.ChooseRememberDuration(ctx, "For this session")).To(Succeed())
			Expect(queue.ConfirmRememberedApproval(ctx)).To(Succeed())
			persistence := storage.ApprovalPersistenceSession
			consentV2EventuallyDecision(ctx, f.pending, storage.ApprovalStatusApproved, &persistence)
			Expect(consentV2ApprovalSnapshot(ctx, f, f.pending.ID)).To(Equal(before))
			Eventually(func() bool { return consentV2PendingContains(readV2(queue.PendingRows(ctx)), f.pending.ToolName) }).Should(BeFalse())
			Expect(readV2(queue.SelectedTool(ctx))).NotTo(BeEmpty(), "the next request is selected automatically")
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds))

			Expect(queue.SelectPendingRow(ctx, f.decisions[2].ToolName)).To(Succeed())
			Expect(queue.OpenApproveOptions(ctx)).To(Succeed())
			Expect(queue.ChooseRememberDuration(ctx, "Always…")).To(Succeed())
			Expect(readV2(queue.ApprovalScopeText(ctx))).To(Equal(consentV2Scope(f.decisions[2])))
			Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, f.decisions[2].ID)).Status).To(Equal(storage.ApprovalStatusPending))
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds))
			Expect(queue.ConfirmRememberedApproval(ctx)).To(Succeed())
			permanent := storage.ApprovalPersistencePermanent
			consentV2EventuallyDecision(ctx, f.decisions[2], storage.ApprovalStatusApproved, &permanent)

			Expect(queue.SelectPendingRow(ctx, f.decisions[0].ToolName)).To(Succeed())
			Expect(queue.PressInboxKey(ctx, "a")).To(Succeed())
			once := storage.ApprovalPersistenceOnce
			consentV2EventuallyDecision(ctx, f.decisions[0], storage.ApprovalStatusApproved, &once)
			Expect(queue.SelectPendingRow(ctx, f.decisions[3].ToolName)).To(Succeed())
			Expect(queue.PressInboxKey(ctx, "d")).To(Succeed())
			consentV2EventuallyDecision(ctx, f.decisions[3], storage.ApprovalStatusDenied, nil)
			Expect(queue.SelectPendingRow(ctx, f.decisions[1].ToolName)).To(Succeed())
			Expect(queue.OpenDenyOptions(ctx)).To(Succeed())
			Expect(queue.ChooseDenyOption(ctx, "Always deny…")).To(Succeed())
			Expect(readV2(GetTestStorage().ToolApprovals().Get(ctx, f.decisions[1].ID)).Status).To(Equal(storage.ApprovalStatusPending), "opening denial confirmation changes nothing")
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds))
			Expect(queue.ConfirmRememberedDenial(ctx)).To(Succeed())
			consentV2EventuallyDecision(ctx, f.decisions[1], storage.ApprovalStatusDenied, &permanent)
			Expect(readV2(queue.PendingListBounds(ctx))).To(Equal(bounds))

			Expect(queue.OpenRemembered(ctx)).To(Succeed())
			Expect(GetTestPage().URL()).To(Equal(baseURL + "/approvals/remembered"))
			Expect(queue.FilterRemembered(ctx, "All")).To(Succeed())
			Expect(readV2(queue.StandingDecisions(ctx))).To(ContainElements(HaveField("Tool", f.approved.ToolName), HaveField("Tool", f.denied.ToolName), HaveField("Tool", f.decisions[2].ToolName), HaveField("Tool", f.decisions[1].ToolName)))
			Expect(base.TakeThemedScreenshots(ctx, "remembered_approvals")).To(Succeed())
			Expect(queue.FilterRemembered(ctx, "Always allowed")).To(Succeed())
			Expect(GetTestPage().URL()).To(ContainSubstring("filter=allowed"))
			Expect(readV2(queue.StandingDecisions(ctx))).NotTo(ContainElement(HaveField("Tool", f.denied.ToolName)))
			Expect(readV2(queue.StandingScope(ctx, f.approved.ToolName))).To(Equal(consentV2Scope(f.approved)))
			Expect(queue.FilterRemembered(ctx, "Always denied")).To(Succeed())
			Expect(GetTestPage().URL()).To(ContainSubstring("filter=denied"))
			Expect(readV2(queue.StandingDecisions(ctx))).To(ContainElement(HaveField("Tool", f.denied.ToolName)))
			Expect(readV2(queue.StandingDecisions(ctx))).NotTo(ContainElement(HaveField("Tool", f.approved.ToolName)))
			Expect(queue.FilterRemembered(ctx, "All")).To(Succeed())
			for _, item := range []*storage.ToolApproval{f.approved, f.denied} {
				others := consentV2ApprovalSnapshot(ctx, f, item.ID)
				Expect(queue.RevokeStanding(ctx, item.ToolName)).To(Succeed())
				Expect(*readV2(GetTestStorage().ToolApprovals().Get(ctx, item.ID)).Persistence).To(Equal(storage.ApprovalPersistencePermanent))
				Expect(queue.CancelRevokeStanding(ctx)).To(Succeed())
				Expect(*readV2(GetTestStorage().ToolApprovals().Get(ctx, item.ID)).Persistence).To(Equal(storage.ApprovalPersistencePermanent))
				Expect(queue.RevokeStanding(ctx, item.ToolName)).To(Succeed())
				Expect(queue.ConfirmRevokeStanding(ctx)).To(Succeed())
				Eventually(func() *storage.ApprovalPersistence {
					return readV2(GetTestStorage().ToolApprovals().Get(ctx, item.ID)).Persistence
				}).Should(BeNil())
				Expect(readV2(queue.StandingDecisions(ctx))).NotTo(ContainElement(HaveField("Tool", item.ToolName)))
				Expect(consentV2ApprovalSnapshot(ctx, f, item.ID)).To(Equal(others))
			}
		})

		// AS-13 from specs/047-redesign-consent-console/spec.md (User Story 6).
		It("AS-13 should announce principal-scoped arrivals within fifteen seconds", func() {
			initialRows := readV2(queue.PendingRows(ctx))
			initialCount := readV2(shell.PendingApprovalCount(ctx))
			Expect(initialCount).To(Equal(len(initialRows)))
			Expect(GetTestPage().Keyboard().Press("Tab")).To(Succeed())
			focus := readV2(base.ActiveElementDescription(ctx))
			Expect(focus.Visible && focus.FocusVisible && focus.Unobscured).To(BeTrue())
			location := GetTestPage().URL()
			requestStart := len(readV2(base.RecordedRequests(ctx)))
			_, err := GetTestStorage().ToolApprovals().Create(ctx, f.incoming)
			Expect(err).NotTo(HaveOccurred())
			Eventually(func(g Gomega) {
				rows, err := queue.PendingRows(ctx)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(consentV2PendingContains(rows, f.incoming.ToolName)).To(BeTrue())
				g.Expect(consentV2PendingContains(rows, f.otherApproval.ToolName)).To(BeFalse())
				count, err := shell.PendingApprovalCount(ctx)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(count).To(Equal(initialCount + 1))
				announcement, err := shell.LiveRegionText(ctx)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(strings.ToLower(announcement)).To(And(ContainSubstring("new"), ContainSubstring("approval")))
			}).WithTimeout(15 * time.Second).Should(Succeed())
			Expect(readV2(shell.LiveRegionPoliteness(ctx))).To(Equal("polite"))
			Expect(readV2(base.ActiveElementDescription(ctx))).To(Equal(focus))
			Expect(GetTestPage().URL()).To(Equal(location))
			requests := readV2(base.RecordedRequests(ctx))[requestStart:]
			expectNoThirdPartyOrGatewayRequests(baseURL, requests)
			var pendingRead bool
			for _, request := range requests {
				path := readV2(url.Parse(request.URL)).Path
				if path == "/api/approvals/pending" && request.Method == http.MethodGet {
					pendingRead = true
				}
				Expect(path).NotTo(Equal("/approvals"), "queue must update without a document reload")
			}
			Expect(pendingRead).To(BeTrue())
		})
	})

	Context("Use the console in either theme", func() {
		// AS-14 from specs/047-redesign-consent-console/spec.md (User Story 7).
		It("AS-14 should persist appearance before paint and follow system changes", func() {
			Expect(delegations.Navigate(ctx)).To(Succeed())
			Expect(readV2(shell.NavigationItems(ctx))).To(Equal([]string{"Agents", "Connections", "Approvals"}))
			Expect(shell.CollapseSidebar(ctx)).To(Succeed())
			Expect(readV2(shell.IsSidebarCollapsed(ctx))).To(BeTrue())
			for _, choice := range []struct{ label, theme, os string }{{"Light", "light", "dark"}, {"Dark", "dark", "light"}} {
				Expect(base.EmulateColorScheme(ctx, choice.os)).To(Succeed())
				Expect(shell.OpenUserMenu(ctx)).To(Succeed())
				Expect(shell.ChooseTheme(ctx, choice.label)).To(Succeed())
				consentV2Reload(GetTestPage())
				Expect(readV2(base.ResolvedTheme(ctx))).To(Equal(choice.theme))
				Expect(readV2(base.FirstPaintTheme(ctx))).To(Equal(pages.ThemeFrame{Theme: choice.theme, ColorScheme: choice.theme}))
				Expect(readV2(shell.IsSidebarCollapsed(ctx))).To(BeTrue())
			}
			Expect(shell.OpenUserMenu(ctx)).To(Succeed())
			Expect(shell.ChooseTheme(ctx, "System")).To(Succeed())
			consentV2Reload(GetTestPage())
			for _, os := range []string{"dark", "light"} {
				Expect(base.EmulateColorScheme(ctx, os)).To(Succeed())
				Eventually(func() string { return readV2(base.ResolvedTheme(ctx)) }).Should(Equal(os))
				Expect(readV2(GetTestPage().Evaluate(`getComputedStyle(document.documentElement).colorScheme`))).To(Equal(os))
			}
			Expect(shell.OpenSettingsFromUserMenu(ctx)).To(Succeed())
			Expect(GetTestPage().URL()).To(Equal(baseURL + "/settings/appearance"))
		})

		// AS-15 from specs/047-redesign-consent-console/spec.md (User Story 7).
		// Oversized metadata is an adversarial-response boundary; production
		// registration retains its 255-character display-name validation.
		It("AS-15 should remain keyboard usable at narrow and zoomed sizes", func() {
			before := consentV2ApprovalSnapshot(ctx, f, id.ApprovalID{})
			forEachTheme(ctx, baseURL, func(theme string, page playwright.Page, themed *pages.Page) {
				Expect(themed.EmulateReducedMotion(ctx, false)).To(Succeed())
				Expect(themed.Navigate(ctx, "/agents")).To(Succeed())
				motionShell := pages.NewConsoleShell(page, baseURL)
				Expect(readV2(motionShell.NavigationIconFrameChanges(ctx, "Agents"))).To(BeNumerically(">", 1))
				Expect(themed.EmulateReducedMotion(ctx, true)).To(Succeed())
				Expect(readV2(themed.ReducedMotionMatches(ctx))).To(BeTrue())
				Expect(readV2(motionShell.NavigationIconFrameChanges(ctx, "Agents"))).To(Equal(1), "a live reduced-motion change must stop icon drawing")
				Expect(themed.EmulateReducedMotion(ctx, false)).To(Succeed())
				Expect(readV2(motionShell.NavigationIconFrameChanges(ctx, "Agents"))).To(BeNumerically(">", 1), "motion can resume after the preference changes back")
				Expect(themed.EmulateReducedMotion(ctx, true)).To(Succeed())
				consentV2AdversarialAgentResponse(ctx, page, baseURL, f.longName.ID)
				decisionPath := startAuthorizationRequest(ctx, page, baseURL, f.verified, f.callbackURL)
				for _, viewport := range []string{"320px", "200%"} {
					if viewport == "320px" {
						Expect(page.SetViewportSize(320, 800)).To(Succeed())
					} else {
						Expect(themed.EmulateZoom(ctx, 200)).To(Succeed())
					}
					viewportHeight := 800
					if viewport == "200%" {
						viewportHeight = 540
					}
					for _, path := range []string{"/agents", "/agents/" + f.delta.ID.String(), "/agents/" + f.longName.ID.String(), "/connections", "/approvals", "/approvals/remembered", "/settings/appearance", decisionPath, "/approvals/" + f.pending.ID.String()} {
						Expect(themed.Navigate(ctx, path)).To(Succeed())
						if path == decisionPath {
							Expect(readV2(pages.NewConsentPage(page, baseURL).AllowViewportBottom(ctx))).To(BeNumerically("<=", viewportHeight), "consent action remains visible or pinned")
						}
						if path == "/approvals/"+f.pending.ID.String() {
							Expect(readV2(pages.NewApprovalPage(page, baseURL).ApproveViewportBottom(ctx))).To(BeNumerically("<=", viewportHeight), "approval action remains visible or pinned")
						}
						if path == "/agents/"+f.longName.ID.String() {
							Eventually(func(g Gomega) {
								name, err := pages.NewConsentPage(page, baseURL).GetAgentName(ctx)
								g.Expect(err).NotTo(HaveOccurred())
								g.Expect(name).To(Equal(fixtures.ConsentV2AdversarialDisplayName()), "the full 300-character escaped name must reach the rendered heading")
							}).Should(Succeed())
						}
						Expect(readV2(themed.ResolvedTheme(ctx))).To(Equal(theme))
						Expect(readV2(themed.HasHorizontalPageOverflow(ctx))).To(BeFalse(), "%s %s %s", theme, viewport, path)
						actions := readV2(themed.KeyboardActionCoverage(ctx))
						Expect(actions).NotTo(BeEmpty(), "view must retain usable actions")
						for _, action := range actions {
							Expect(action.Name).NotTo(BeEmpty())
							Expect(action.Reached).To(BeTrue(), "Tab cannot reach %q on %s", action.Name, path)
							Expect(action.VisibleFocus).To(BeTrue(), "focus is hidden for %q on %s", action.Name, path)
						}
						Expect(readV2(page.Evaluate(`window.consentV2Injected === true`))).To(BeFalse(), "agent markup must remain text")
					}
				}
				expectNoThirdPartyOrGatewayRequests(baseURL, readV2(themed.RecordedRequests(ctx)))
			})
			Expect(consentV2ApprovalSnapshot(ctx, f, id.ApprovalID{})).To(Equal(before))
			Expect(consentV2Grant(ctx, f.principal, f.first)).To(BeNil())
			Expect(readV2(GetTestStorage().Agents().Get(ctx, f.longName.ID)).DisplayName).To(Equal(f.longName.DisplayName), "adversarial display metadata must not alter stored agent identity")
		})
	})

	Context("Settings and command palette", func() {
		// AS-17 from specs/047-redesign-consent-console/spec.md (User Story 8).
		It("AS-17 should share appearance without changing approval choices", func() {
			before := consentV2ApprovalSnapshot(ctx, f, id.ApprovalID{})
			Expect(settings.Navigate(ctx)).To(Succeed())
			Expect(readV2(settings.HasThemePreviews(ctx))).To(BeTrue())
			Expect(readV2(settings.HasCategoryMenu(ctx))).To(BeFalse(), "a one-category menu must not render")
			Expect(readV2(settings.HasApprovalPersistenceControl(ctx))).To(BeFalse())
			Expect(base.TakeThemedScreenshots(ctx, "settings")).To(Succeed())
			for _, choice := range []struct{ label, theme string }{{"Light", "light"}, {"System", "light"}, {"Dark", "dark"}} {
				Expect(settings.ChooseTheme(ctx, choice.label)).To(Succeed())
				Expect(readV2(settings.SelectedTheme(ctx))).To(Equal(choice.label))
				Expect(readV2(base.ResolvedTheme(ctx))).To(Equal(choice.theme))
			}
			Expect(settings.ChooseDefaultView(ctx, "list")).To(Succeed())
			consentV2Reload(GetTestPage())
			Expect(readV2(settings.SelectedTheme(ctx))).To(Equal("Dark"))
			Expect(readV2(settings.SelectedDefaultView(ctx))).To(Equal("list"))
			Expect(readV2(base.ResolvedTheme(ctx))).To(Equal("dark"))
			Expect(base.Navigate(ctx, "/connections")).To(Succeed())
			Expect(readV2(connections.View(ctx))).To(Equal("list"), "the stored default applies to a five-connection collection")
			Expect(settings.Navigate(ctx)).To(Succeed())
			Expect(settings.ChooseDefaultView(ctx, "grid")).To(Succeed())
			consentV2Reload(GetTestPage())
			Expect(readV2(settings.SelectedDefaultView(ctx))).To(Equal("grid"))
			Expect(base.Navigate(ctx, "/connections")).To(Succeed())
			Expect(readV2(connections.View(ctx))).To(Equal("grid"))
			Expect(base.Navigate(ctx, "/approvals/"+f.pending.ID.String())).To(Succeed())
			Expect(readV2(base.ResolvedTheme(ctx))).To(Equal("dark"))
			Expect(readV2(approval.HasApproveOptionsAndDeny(ctx))).To(BeTrue())
			Expect(approval.OpenApproveOptions(ctx)).To(Succeed())
			Expect(approval.ChooseRememberDuration(ctx, "Always…")).To(Succeed())
			Expect(readV2(approval.ApprovalScopeText(ctx))).To(Equal(consentV2Scope(f.pending)))
			Eventually(func() bool { return readV2(approval.HasDecisionActions(ctx)) }).Should(BeTrue())
			Expect(consentV2ApprovalSnapshot(ctx, f, id.ApprovalID{})).To(Equal(before), "appearance and persistence selection must never submit a decision")
		})

		// AS-18 from specs/047-redesign-consent-console/spec.md (User Story 9).
		It("AS-18 should navigate scoped records and change theme by keyboard", func() {
			Expect(delegations.Navigate(ctx)).To(Succeed())
			for _, record := range []struct{ label, path string }{{f.delta.DisplayName, "/agents/" + f.delta.ID.String()}, {f.services[0].DisplayName, "/connections"}, {f.pending.ToolName, "/approvals/" + f.pending.ID.String()}, {"Appearance", "/settings/appearance"}} {
				Expect(shell.OpenCommandPalette(ctx)).To(Succeed())
				Expect(shell.SearchCommandPalette(ctx, record.label)).To(Succeed())
				Expect(readV2(shell.CommandPaletteResults(ctx))).To(ContainElement(record.label))
				Expect(shell.ChooseCommandResult(ctx, record.label)).To(Succeed())
				Eventually(func() string { return GetTestPage().URL() }).Should(Equal(baseURL + record.path))
				Expect(delegations.Navigate(ctx)).To(Succeed())
			}
			Expect(shell.OpenCommandPalette(ctx)).To(Succeed())
			for _, privateName := range []string{f.other.DisplayName, f.services[6].DisplayName, f.otherApproval.ToolName} {
				Expect(shell.SearchCommandPalette(ctx, privateName)).To(Succeed())
				Expect(readV2(shell.CommandPaletteResults(ctx))).NotTo(ContainElement(ContainSubstring(privateName)))
			}
			Expect(shell.SearchCommandPalette(ctx, "Dark")).To(Succeed())
			Expect(shell.ChooseCommandResult(ctx, "Dark")).To(Succeed())
			Expect(readV2(base.ResolvedTheme(ctx))).To(Equal("dark"))
			expectNoThirdPartyOrGatewayRequests(baseURL, readV2(base.RecordedRequests(ctx)))
		})
	})
})
