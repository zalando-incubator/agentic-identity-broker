//go:build integration

package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/jmoiron/sqlx"
	"github.com/jwx-go/jwkfetch/v4"
	"github.com/lestrrat-go/jwx/v4/jwt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Consent-Bound Refresh Sessions / US5 inactivity", func() {
	var (
		ctx           context.Context
		cfg           *ports.Config
		environment   *bootstrap.RefreshEnvironment
		database      *sqlx.DB
		cleanup       func()
		client        *fixtures.RefreshClient
		authorization *helpers.RefreshAuthorization
	)

	BeforeEach(func() {
		ctx = context.Background()
		cfg = fixtures.LocalConfig()
		environment, database, cleanup = nil, nil, nil
	})

	JustBeforeEach(func() {
		store, db, closeDatabase, err := bootstrap.NewRefreshPostgresStorage(refreshSuiteTestingT, cfg)
		Expect(err).NotTo(HaveOccurred())
		database, cleanup = db, closeDatabase
		environment, err = bootstrap.NewRefreshEnvironment(cfg, store)
		Expect(err).NotTo(HaveOccurred())
		Expect(fixtures.SeedPlaceholderGrantData(ctx, store)).To(Succeed())
		client, err = environment.AddClient(ctx, id.Principal(fixtures.DefaultPrincipal().String()), true)
		Expect(err).NotTo(HaveOccurred())
		authorization, err = helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		if environment != nil {
			environment.Stop()
		}
		if cleanup != nil {
			cleanup()
		}
	})

	Context("with the default inactivity lifetime", func() {
		// US5-S1 from specs/049-fix-refresh-consent/spec.md.
		It("permits a still-active interval and denies renewal after thirty inactive days", func() {
			near, err := helpers.AuthorizeRefreshSession(ctx, environment.Enduser.BaseURL(), *client, "read offline_access")
			Expect(err).NotTo(HaveOccurred())
			Expect(helpers.AgeInitialRefreshSession(ctx, database, near.Tokens.RefreshToken, 719*time.Hour)).To(Succeed())
			valid, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, near.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(valid.Status).To(Equal(http.StatusOK))
			Expect(helpers.AgeInitialRefreshSession(ctx, database, authorization.Tokens.RefreshToken, 721*time.Hour)).To(Succeed())
			expired, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			expectInactivityDenial(expired)
		})
	})

	Context("when a fresh rotation occurs before expiry", func() {
		BeforeEach(func() {
			var err error
			cfg, err = fixtures.RefreshConfig("1s", "0s", "4s")
			Expect(err).NotTo(HaveOccurred())
		})

		// US5-S2 from specs/049-fix-refresh-consent/spec.md.
		It("keeps the successor usable after the original inactivity interval closes", func() {
			Expect(helpers.AgeInitialRefreshSession(ctx, database, authorization.Tokens.RefreshToken, 2*time.Second)).To(Succeed())
			originalExpiry := inactivityTokenExpiry(ctx, database, authorization.Tokens.RefreshToken)
			rotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Status).To(Equal(http.StatusOK))
			successorExpiry := inactivityTokenExpiry(ctx, database, rotated.Tokens.RefreshToken)
			Expect(successorExpiry).To(BeTemporally(">", originalExpiry))
			Eventually(func() (time.Time, error) { return helpers.RefreshSharedTime(ctx, database) }).
				WithTimeout(5 * time.Second).WithPolling(20 * time.Millisecond).
				Should(BeTemporally(">=", originalExpiry))
			continued, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, rotated.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(continued.Status).To(Equal(http.StatusOK))
			var hasRoots bool
			Expect(database.GetContext(ctx, &hasRoots, "SELECT to_regclass('refresh_sessions') IS NOT NULL")).To(Succeed())
			if hasRoots {
				root, rootErr := helpers.ReadRefreshRoot(ctx, database, continued.Tokens.RefreshToken)
				Expect(rootErr).NotTo(HaveOccurred())
				fresh, err := time.Parse(time.RFC3339Nano, root["last_fresh_at"].(string))
				Expect(err).NotTo(HaveOccurred())
				deadline, err := time.Parse(time.RFC3339Nano, root["inactivity_expires_at"].(string))
				Expect(err).NotTo(HaveOccurred())
				Expect(deadline.Sub(fresh)).To(Equal(4 * time.Second))
			}
		})
	})

	Context("when only the original access token is used", func() {
		var resource *httptest.Server
		var request *http.Request

		BeforeEach(func() {
			var err error
			cfg, err = fixtures.RefreshConfig("500ms", "0s", "2s")
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			keys, err := jwkfetch.NewClient().Fetch(ctx, environment.Enduser.BaseURL()+"/oauth2/jwks.json")
			Expect(err).NotTo(HaveOccurred())
			resource = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
				token, err := jwt.Parse([]byte(raw), jwt.WithKeySet(keys), jwt.WithIssuer(cfg.Server.EndUser.PublicURL))
				if err != nil {
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				subject, _ := token.Subject()
				agent, _ := token.Field("agent_id")
				_ = json.NewEncoder(w).Encode(struct {
					Subject string `json:"sub"`
					Agent   any    `json:"agent_id"`
				}{subject, agent})
			}))
			request, err = http.NewRequestWithContext(ctx, http.MethodGet, resource.URL, nil)
			Expect(err).NotTo(HaveOccurred())
			request.Header.Set("Authorization", "Bearer "+authorization.Tokens.AccessToken)
		})

		AfterEach(func() {
			if resource != nil {
				resource.Close()
				resource = nil
			}
		})

		// US5-S3 from specs/049-fix-refresh-consent/spec.md.
		It("expires refresh authority despite successful signed-token resource requests", func() {
			expiry := inactivityTokenExpiry(ctx, database, authorization.Tokens.RefreshToken)
			Eventually(func() (time.Time, error) {
				response, err := helpers.HTTPClient().Do(request)
				if err != nil {
					return time.Time{}, err
				}
				defer func() { _ = response.Body.Close() }()
				Expect(response.StatusCode).To(Equal(http.StatusOK))
				var claims struct {
					Subject string `json:"sub"`
					Agent   string `json:"agent_id"`
				}
				Expect(json.NewDecoder(response.Body).Decode(&claims)).To(Succeed())
				Expect(claims.Subject).To(Equal(client.Principal.String()))
				Expect(claims.Agent).To(Equal(client.Agent.ID.String()))
				return helpers.RefreshSharedTime(ctx, database)
			}).WithTimeout(4 * time.Second).WithPolling(50 * time.Millisecond).Should(BeTemporally(">=", expiry))
			result, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			expectInactivityDenial(result)
		})
	})

	Context("when a consumed predecessor returns its stored result", func() {
		BeforeEach(func() {
			var err error
			cfg, err = fixtures.RefreshConfig("3s", "0s", "10s")
			Expect(err).NotTo(HaveOccurred())
		})

		// US5-S4 from specs/049-fix-refresh-consent/spec.md.
		It("changes neither the inactivity deadline nor the fixed reuse interval", func() {
			rotated, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(rotated.Status).To(Equal(http.StatusOK))
			before, observationErr := helpers.ReadRefreshRoot(ctx, database, rotated.Tokens.RefreshToken)
			retry, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			Expect(retry.Status).To(Equal(http.StatusOK))
			Expect(retry.Tokens.AccessToken == rotated.Tokens.AccessToken).To(BeTrue(), "original access token must match")
			Expect(retry.Tokens.RefreshToken == rotated.Tokens.RefreshToken).To(BeTrue(), "original refresh token must match")
			Expect(observationErr).NotTo(HaveOccurred())
			after, err := helpers.ReadRefreshRoot(ctx, database, retry.Tokens.RefreshToken)
			Expect(err).NotTo(HaveOccurred())
			for _, field := range []string{"started_at", "last_fresh_at", "inactivity_expires_at", "reuse_until", "previous_consumed_at", "retry_expires_at"} {
				Expect(after[field]).To(Equal(before[field]), field)
			}
			Expect(after["retry_count"]).To(Equal(before["retry_count"].(float64) + 1))
		})
	})

	Context("with a custom inactivity lifetime", func() {
		BeforeEach(func() {
			var err error
			cfg, err = fixtures.RefreshConfig("500ms", "0s", "1800ms")
			Expect(err).NotTo(HaveOccurred())
		})

		// US5-S5 from specs/049-fix-refresh-consent/spec.md.
		It("denies renewal at the custom interval instead of the thirty-day default", func() {
			expiry := inactivityTokenExpiry(ctx, database, authorization.Tokens.RefreshToken)
			Eventually(func() (time.Time, error) { return helpers.RefreshSharedTime(ctx, database) }).
				WithTimeout(3 * time.Second).WithPolling(20 * time.Millisecond).
				Should(BeTemporally(">=", expiry))
			result, err := helpers.RotateRefreshSession(ctx, environment.Enduser.BaseURL(), *client, authorization.Tokens.RefreshToken, "")
			Expect(err).NotTo(HaveOccurred())
			expectInactivityDenial(result)
		})
	})
})

func inactivityTokenExpiry(ctx context.Context, database *sqlx.DB, rawToken string) time.Time {
	var expires time.Time
	Expect(database.GetContext(ctx, &expires, "SELECT expires_at FROM refresh_token_sessions WHERE signature = $1", helpers.RefreshSignature(rawToken))).To(Succeed())
	return expires
}

func expectInactivityDenial(result *helpers.RefreshTokenResult) {
	Expect(result.Status).To(Equal(http.StatusBadRequest))
	Expect(result.Error).To(Equal("invalid_grant"))
	_, hasAccess := result.Body["access_token"]
	_, hasRefresh := result.Body["refresh_token"]
	Expect(hasAccess).To(BeFalse(), "denial must not return an access token")
	Expect(hasRefresh).To(BeFalse(), "denial must not return a refresh token")
}
