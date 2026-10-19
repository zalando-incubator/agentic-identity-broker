package e2e_test

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

const sweepCronJobName = "broker-agentic-identity-broker-session-sweep"

func renderSweepCronJob(values map[string]any) map[string]any {
	docs, err := helpers.RenderChart(values)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	cronJob := helpers.FindResource(docs, "CronJob", sweepCronJobName)
	ExpectWithOffset(1, cronJob).NotTo(BeNil(), "rendered chart has no CronJob %s", sweepCronJobName)
	return cronJob
}

func sweepPodTemplate(cronJob map[string]any) map[string]any {
	template, ok := helpers.Path(cronJob, "spec", "jobTemplate", "spec", "template").(map[string]any)
	ExpectWithOffset(1, ok).To(BeTrue(), "CronJob has no pod template")
	return template
}

func sweepScript(cronJob map[string]any) string {
	containers, ok := helpers.Path(sweepPodTemplate(cronJob), "spec", "containers").([]any)
	ExpectWithOffset(1, ok).To(BeTrue())
	ExpectWithOffset(1, containers).NotTo(BeEmpty())
	command, ok := containers[0].(map[string]any)["command"].([]any)
	ExpectWithOffset(1, ok).To(BeTrue())
	ExpectWithOffset(1, command).To(HaveLen(3))
	return command[2].(string)
}

func stringMap(value any) map[string]string {
	out := map[string]string{}
	if m, ok := value.(map[string]any); ok {
		for k, v := range m {
			out[k] = fmt.Sprint(v)
		}
	}
	return out
}

func writeTempFile(dir, name string, content []byte) string {
	path := filepath.Join(dir, name)
	ExpectWithOffset(1, os.WriteFile(path, content, 0o600)).To(Succeed())
	return path
}

var _ = Describe("Session Sweep Schedule", func() {
	Context("when the chart is rendered", func() {
		// US1-S1 from specs/052-session-auto-refresh/spec.md
		It("renders no sweep resources and ignores other sessionSweep values when disabled", func() {
			defaults, err := helpers.RenderChart(map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			Expect(helpers.FindResourcesByKind(defaults, "CronJob")).To(BeEmpty())
			Expect(helpers.FindResource(defaults, "ServiceAccount", sweepCronJobName)).To(BeNil())

			defaultRaw, err := helpers.RenderChartRaw(map[string]any{})
			Expect(err).NotTo(HaveOccurred())
			disabledRaw, err := helpers.RenderChartRaw(fixtures.SweepChartValues(map[string]any{
				"broker": nil,
				"sessionSweep": map[string]any{
					"enabled":   false,
					"schedule":  "*/5 * * * *",
					"url":       "https://broker-admin-proxy.example.test",
					"podLabels": map[string]any{"tier": "sweep"},
				},
			}))
			Expect(err).NotTo(HaveOccurred())
			Expect(disabledRaw).To(Equal(defaultRaw))

			enabled, err := helpers.RenderChart(fixtures.SweepChartValues(nil))
			Expect(err).NotTo(HaveOccurred())
			Expect(helpers.FindResource(enabled, "CronJob", sweepCronJobName)).NotTo(BeNil())
			Expect(helpers.FindResource(enabled, "ServiceAccount", sweepCronJobName)).NotTo(BeNil())
		})

		// US1-S2 from specs/052-session-auto-refresh/spec.md
		It("renders an hourly Forbid CronJob targeting the admin Service with the public-URL Host and PT2H", func() {
			cronJob := renderSweepCronJob(fixtures.SweepChartValues(nil))
			Expect(helpers.Path(cronJob, "spec", "schedule")).To(Equal("0 * * * *"))
			Expect(helpers.Path(cronJob, "spec", "concurrencyPolicy")).To(Equal("Forbid"))
			Expect(helpers.Path(cronJob, "spec", "startingDeadlineSeconds")).To(Equal(3600))
			Expect(helpers.Path(cronJob, "spec", "jobTemplate", "spec", "backoffLimit")).To(Equal(0))
			Expect(helpers.Path(cronJob, "spec", "jobTemplate", "spec", "activeDeadlineSeconds")).To(Equal(2100))
			Expect(helpers.Path(sweepPodTemplate(cronJob), "spec", "restartPolicy")).To(Equal("Never"))
			script := sweepScript(cronJob)
			Expect(script).To(ContainSubstring("http://broker-agentic-identity-broker.identity.svc:14000/api/sessions/sweep"))
			Expect(script).To(ContainSubstring(`Host: ` + strings.TrimPrefix(fixtures.SweepAdminPublicURL(), "https://")))
			Expect(script).To(ContainSubstring(`"lookahead_duration":"PT2H"`))
			Expect(script).To(ContainSubstring(`"dry_run":false`))
		})

		// US1-S7 from specs/052-session-auto-refresh/spec.md
		It("fails to render when no identity is configured", func() {
			_, err := helpers.RenderChart(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"headers": nil},
			}))
			Expect(err).To(MatchError(ContainSubstring(
				"sessionSweep.enabled requires an identity for the admin API: set sessionSweep.headers, sessionSweep.headersFromFile, or sessionSweep.tls.certFile.")))
		})

		// US1-S10 from specs/052-session-auto-refresh/spec.md
		It("keeps the job pod out of the broker Service and PDB selectors and labels it session-sweep", func() {
			values := fixtures.SweepChartValues(map[string]any{
				"podDisruptionBudget": map[string]any{"enabled": true},
				"commonLabels":        map[string]any{"team": "identity", "tier": "backend"},
				"sessionSweep":        map[string]any{"podLabels": map[string]any{"tier": "sweep"}},
			})
			docs, err := helpers.RenderChart(values)
			Expect(err).NotTo(HaveOccurred())
			cronJob := helpers.FindResource(docs, "CronJob", sweepCronJobName)
			Expect(cronJob).NotTo(BeNil())
			pod := sweepPodTemplate(cronJob)
			labels := stringMap(helpers.Path(pod, "metadata", "labels"))
			Expect(labels).NotTo(HaveKey("app.kubernetes.io/name"))
			Expect(labels).To(HaveKeyWithValue("app.kubernetes.io/component", "session-sweep"))
			Expect(labels).To(HaveKeyWithValue("team", "identity"))
			Expect(labels).To(HaveKeyWithValue("tier", "sweep"))

			service := helpers.FindResource(docs, "Service", "broker-agentic-identity-broker")
			pdb := helpers.FindResource(docs, "PodDisruptionBudget", "broker-agentic-identity-broker")
			Expect(service).NotTo(BeNil())
			Expect(pdb).NotTo(BeNil())
			for _, selector := range []map[string]string{
				stringMap(helpers.Path(service, "spec", "selector")),
				stringMap(helpers.Path(pdb, "spec", "selector", "matchLabels")),
			} {
				Expect(selector).NotTo(BeEmpty())
				matches := true
				for key, value := range selector {
					if labels[key] != value {
						matches = false
					}
				}
				Expect(matches).To(BeFalse(), "job pod labels %v match selector %v", labels, selector)
			}

			Expect(helpers.Path(pod, "spec", "automountServiceAccountToken")).To(Equal(false))
			Expect(helpers.Path(pod, "spec", "serviceAccountName")).To(Equal(sweepCronJobName))
			serviceAccount := helpers.FindResource(docs, "ServiceAccount", sweepCronJobName)
			Expect(serviceAccount).NotTo(BeNil())
			Expect(serviceAccount["automountServiceAccountToken"]).To(Equal(false))
			containers := helpers.Path(pod, "spec", "containers").([]any)
			securityContext := containers[0].(map[string]any)["securityContext"].(map[string]any)
			Expect(securityContext).To(HaveKeyWithValue("readOnlyRootFilesystem", true))
			Expect(securityContext).To(HaveKeyWithValue("allowPrivilegeEscalation", false))
		})

		// US1-S11 from specs/052-session-auto-refresh/spec.md
		It("rejects a timeout not below the deadline, an unpaired TLS file, or a non-ISO lookahead at render time", func() {
			_, err := helpers.RenderChart(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"timeoutSeconds": 60, "activeDeadlineSeconds": 60},
			}))
			Expect(err).To(MatchError(ContainSubstring("sessionSweep.timeoutSeconds (60) must be shorter than sessionSweep.activeDeadlineSeconds (60).")))

			_, err = helpers.RenderChart(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"tls": map[string]any{"certFile": "/var/run/secrets/session-sweep/tls.crt"}},
			}))
			Expect(err).To(MatchError(ContainSubstring("sessionSweep.tls.certFile and sessionSweep.tls.keyFile must be set together.")))

			_, err = helpers.RenderChart(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"lookaheadDuration": "2h"},
			}))
			Expect(err).To(MatchError(ContainSubstring("lookaheadDuration")))
		})
	})

	Context("when the rendered job runs against the admin API", func() {
		var (
			ctx          context.Context
			logs         *bootstrap.BufferedLogCapture
			storeFactory *bootstrap.StorageFactory
			store        *storageadapter.Adapter
			admin        *bootstrap.TestServer
			upstream     *helpers.MockUpstreamOAuth2Server
			proxy        *helpers.RecordingProxy
			serviceID    id.ServiceID
			tempDir      string
		)

		runAgainst := func(target string, overrides map[string]any) helpers.SweepScriptRun {
			base := map[string]any{
				"sessionSweep": map[string]any{"url": target, "timeoutSeconds": 20, "activeDeadlineSeconds": 30},
			}
			cronJob := renderSweepCronJob(fixtures.SweepChartValues(fixtures.MergeValues(base, overrides)))
			run, err := helpers.RunSweepScript(cronJob, time.Minute)
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			return run
		}

		BeforeEach(func() {
			ctx = context.Background()
			tempDir = GinkgoT().TempDir()
			var logger *slog.Logger
			logger, logs = bootstrap.NewBufferedJSONLogger(slog.LevelDebug)
			upstream = helpers.NewMockUpstreamOAuth2Server()
			upstream.WithSuccessfulTokenResponse().WithAccessToken("renewed-github-token").WithRefreshToken("renewed-github-refresh")
			config := fixtures.OAuth2ConfigWithTokenExchange(upstream.URL())
			config.TokenRefresh = ports.TokenRefreshConfig{LookaheadDuration: 5 * time.Minute, BackgroundWorkers: 10, Sweep: ports.TokenRefreshSweepConfig{DefaultPageSize: 100}}
			config.Server.Admin.PublicURL = fixtures.SweepAdminPublicURL()
			config.Server.Admin.Authentication.Preauth.PrincipalHeaderName = fixtures.SweepOperatorHeader()
			storeFactory = bootstrap.NewStorageFactory(logger)
			var err error
			store, err = storeFactory.NewTestStorage()
			Expect(err).NotTo(HaveOccurred())
			service := fixtures.GitHubService()
			service.Endpoints.TokenEndpoint = upstream.URL() + "/oauth/token"
			service.Endpoints.AuthorizeEndpoint = upstream.URL() + "/oauth/authorize"
			serviceID = service.ID
			Expect(store.Services().Create(ctx, service)).To(Succeed())
			app, err := bootstrap.NewServerFactory(config, logger).BuildApp(store)
			Expect(err).NotTo(HaveOccurred())
			admin, err = bootstrap.NewAdminTestServer(app, logger)
			Expect(err).NotTo(HaveOccurred())
			proxy, err = helpers.NewRecordingProxy(admin.BaseURL(), nil)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			if proxy != nil {
				proxy.Close()
			}
			if admin != nil {
				admin.Close()
			}
			if upstream != nil {
				upstream.Close()
			}
			if store != nil {
				Expect(storeFactory.CloseStorage(store)).To(Succeed())
			}
		})

		// US1-S3 from specs/052-session-auto-refresh/spec.md
		It("sends the request to sessionSweep.url and overrides Host only when sessionSweep.host is set", func() {
			proxyHost := strings.TrimPrefix(proxy.URL(), "http://")
			run := runAgainst(proxy.URL(), nil)
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			run = runAgainst(proxy.URL(), map[string]any{"sessionSweep": map[string]any{"host": "broker-admin.example.test"}})
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			requests := proxy.Requests()
			Expect(requests).To(HaveLen(2))
			Expect(requests[0].Method).To(Equal(http.MethodPost))
			Expect(requests[0].Path).To(Equal("/api/sessions/sweep"))
			Expect(requests[0].Host).To(Equal(proxyHost))
			Expect(requests[1].Host).To(Equal("broker-admin.example.test"))
		})

		// US1-S4 from specs/052-session-auto-refresh/spec.md
		It("sends configured literal headers", func() {
			run := runAgainst(proxy.URL(), map[string]any{
				"sessionSweep": map[string]any{"headers": map[string]any{"X-Request-Source": `scheduled "sweep"`}},
			})
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			requests := proxy.Requests()
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Header.Get(fixtures.SweepOperatorHeader())).To(Equal(fixtures.SweepOperatorPrincipal()))
			Expect(requests[0].Header.Get("X-Request-Source")).To(Equal(`scheduled "sweep"`))
			Expect(requests[0].Header.Get("Content-Type")).To(Equal("application/json"))
		})

		// US1-S5 from specs/052-session-auto-refresh/spec.md
		It("reads file headers at run time and keeps their contents out of the manifest, argv, env, and log", func() {
			secret := make([]byte, 24)
			_, err := rand.Read(secret)
			Expect(err).NotTo(HaveOccurred())
			token := hex.EncodeToString(secret)
			tokenFile := writeTempFile(tempDir, "token", []byte(token+"\n"))
			overrides := map[string]any{"sessionSweep": map[string]any{
				"headersFromFile": map[string]any{"Authorization": map[string]any{"path": tokenFile, "prefix": "Bearer "}},
			}}
			rendered, err := helpers.RenderChartRaw(fixtures.SweepChartValues(fixtures.MergeValues(map[string]any{
				"sessionSweep": map[string]any{"url": proxy.URL()},
			}, overrides)))
			Expect(err).NotTo(HaveOccurred())
			Expect(rendered).NotTo(ContainSubstring(token))

			run := runAgainst(proxy.URL(), overrides)
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			requests := proxy.Requests()
			Expect(requests).To(HaveLen(1))
			Expect(requests[0].Header.Get("Authorization")).To(Equal("Bearer " + token))
			Expect(run.CurlArgv).NotTo(BeEmpty())
			Expect(run.CurlArgv).NotTo(ContainSubstring(token))
			Expect(run.CurlEnv).NotTo(ContainSubstring(token))
			Expect(run.Stdout + run.Stderr).NotTo(ContainSubstring(token))
		})

		// US1-S6 from specs/052-session-auto-refresh/spec.md
		It("presents the configured client certificate and trusts the configured CA", func() {
			ca, err := helpers.NewTestCA()
			Expect(err).NotTo(HaveOccurred())
			certPEM, keyPEM, err := ca.IssueClientCertificate(fixtures.SweepOperatorPrincipal())
			Expect(err).NotTo(HaveOccurred())
			tlsProxy, err := helpers.NewRecordingProxy(admin.BaseURL(), ca.RequireClientCertificates())
			Expect(err).NotTo(HaveOccurred())
			DeferCleanup(tlsProxy.Close)
			caFile := writeTempFile(tempDir, "ca.crt", tlsProxy.CertificatePEM())
			certFile := writeTempFile(tempDir, "tls.crt", certPEM)
			keyFile := writeTempFile(tempDir, "tls.key", keyPEM)

			run := runAgainst(tlsProxy.URL(), map[string]any{"sessionSweep": map[string]any{
				"tls": map[string]any{"certFile": certFile, "keyFile": keyFile, "caFile": caFile},
			}})
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			Expect(tlsProxy.Requests()).To(HaveLen(1))

			run = runAgainst(tlsProxy.URL(), map[string]any{"sessionSweep": map[string]any{
				"tls": map[string]any{"caFile": caFile},
			}})
			Expect(run.ExitCode).NotTo(Equal(0))
			Expect(tlsProxy.Requests()).To(HaveLen(1))
		})

		// US1-S8 from specs/052-session-auto-refresh/spec.md
		It("succeeds and logs the summary on 200 even when failed is non-zero", func() {
			due := fixtures.GitHubSessionForPrincipal(fixtures.DefaultPrincipal().String())
			soon := time.Now().Add(30 * time.Minute)
			due.AccessTokenExpiresAt = &soon
			Expect(store.UserSessions().Create(ctx, due)).To(Succeed())
			Expect(store.UserSessions().Create(ctx, fixtures.FullyExpiredSessionForPrincipal(fixtures.AnotherPrincipal().String(), serviceID.String()))).To(Succeed())

			run := runAgainst(proxy.URL(), nil)
			Expect(run.ExitCode).To(Equal(0), run.Stderr)
			Expect(run.Stdout).To(ContainSubstring(`"refreshed":1`))
			Expect(run.Stdout).To(ContainSubstring(`"failed":1`))
			Expect(run.Stdout).To(ContainSubstring(`"dry_run":false`))

			records, err := logs.Records()
			Expect(err).NotTo(HaveOccurred())
			var audits []map[string]any
			for _, record := range records {
				if record["msg"] == "session sweep" {
					audits = append(audits, record)
				}
			}
			Expect(audits).To(HaveLen(1))
			Expect(audits[0]).To(HaveKeyWithValue("operator_principal", fixtures.SweepOperatorPrincipal()))
			Expect(audits[0]).To(HaveKeyWithValue("outcome", "completed"))
			Expect(audits[0]).To(HaveKeyWithValue("lookahead", "2h0m0s"))
		})
	})

	Context("when the sweep target fails", func() {
		// US1-S9 from specs/052-session-auto-refresh/spec.md
		It("fails without retry on non-2xx, timeout, or unreachable target and never logs request headers", func() {
			for _, status := range []int{http.StatusFound, http.StatusForbidden, http.StatusInternalServerError} {
				var hits atomic.Int32
				target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					hits.Add(1)
					if status == http.StatusFound {
						w.Header().Set("Location", "https://login.example.test/")
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					_, _ = fmt.Fprintf(w, `{"error":"status-%d"}`, status)
				}))
				cronJob := renderSweepCronJob(fixtures.SweepChartValues(map[string]any{
					"sessionSweep": map[string]any{"url": target.URL, "timeoutSeconds": 2, "activeDeadlineSeconds": 5},
				}))
				run, err := helpers.RunSweepScript(cronJob, 30*time.Second)
				target.Close()
				Expect(err).NotTo(HaveOccurred())
				Expect(run.ExitCode).NotTo(Equal(0), "status %d", status)
				Expect(run.Stdout).To(ContainSubstring(fmt.Sprintf(`{"error":"status-%d"}`, status)))
				Expect(run.Stderr).To(ContainSubstring(fmt.Sprintf("session sweep failed: HTTP %d", status)))
				Expect(run.Stdout + run.Stderr).NotTo(ContainSubstring(fixtures.SweepOperatorHeader() + ": " + fixtures.SweepOperatorPrincipal()))
				Expect(hits.Load()).To(Equal(int32(1)), "status %d", status)
			}

			var hangs atomic.Int32
			release := make(chan struct{})
			slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hangs.Add(1)
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			cronJob := renderSweepCronJob(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"url": slow.URL, "timeoutSeconds": 2, "activeDeadlineSeconds": 5},
			}))
			started := time.Now()
			run, err := helpers.RunSweepScript(cronJob, 30*time.Second)
			close(release)
			slow.Close()
			Expect(err).NotTo(HaveOccurred())
			Expect(run.ExitCode).NotTo(Equal(0))
			Expect(time.Since(started)).To(BeNumerically("<", 10*time.Second))
			Expect(hangs.Load()).To(Equal(int32(1)))
			Expect(run.Stdout + run.Stderr).NotTo(ContainSubstring(fixtures.SweepOperatorHeader() + ": " + fixtures.SweepOperatorPrincipal()))

			closed := httptest.NewServer(http.NotFoundHandler())
			unreachable := closed.URL
			closed.Close()
			parsed, err := url.Parse(unreachable)
			Expect(err).NotTo(HaveOccurred())
			cronJob = renderSweepCronJob(fixtures.SweepChartValues(map[string]any{
				"sessionSweep": map[string]any{"url": parsed.String(), "timeoutSeconds": 2, "activeDeadlineSeconds": 5},
			}))
			run, err = helpers.RunSweepScript(cronJob, 30*time.Second)
			Expect(err).NotTo(HaveOccurred())
			Expect(run.ExitCode).NotTo(Equal(0))
			Expect(run.Stdout + run.Stderr).NotTo(ContainSubstring(fixtures.SweepOperatorHeader() + ": " + fixtures.SweepOperatorPrincipal()))
		})
	})
})
