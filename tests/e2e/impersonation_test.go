package e2e_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/matchers"
)

const (
	imperAudience  = "https://broker.example.com/impersonation"
	imperJWTType   = "urn:ietf:params:oauth:token-type:jwt"
	imperAccessTyp = "urn:ietf:params:oauth:token-type:access_token"
	imperBearerTyp = "urn:ietf:params:oauth:client-assertion-type:jwt-bearer"
	imperGrant     = "urn:ietf:params:oauth:grant-type:token-exchange"
)

// imperSignedRole builds a role entry with per-role expected audience and extraction.
func imperSignedRole(principalExpr, emailExpr string) ports.ImpersonationRoleConfig {
	return ports.ImpersonationRoleConfig{
		ExpectedAudience:    imperAudience,
		PrincipalExpression: principalExpr,
		EmailExpression:     emailExpr,
	}
}

// imperSignedRule builds one signed-subject rule trusting a single issuer for all three roles.
func imperSignedRule(name, issuerURI, jwksURI, authzExpr string) ports.ImpersonationRuleConfig {
	return ports.ImpersonationRuleConfig{
		Name: name,
		Roles: map[string]ports.ImpersonationRoleConfig{
			"client_assertion": imperSignedRole("client_assertion.sub", ""),
			"actor":            imperSignedRole("actor_token.sub", ""),
			"subject":          imperSignedRole("subject_token.sub", "has(subject_token.email) ? subject_token.email : ''"),
		},
		TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
			IssuerURI:         issuerURI,
			JWKSURI:           jwksURI,
			JWKSMinRefresh:    15 * time.Minute,
			JWKSMaxRefresh:    time.Hour,
			AllowedAlgorithms: []string{"RS256", "ES256"},
			SignsRoles:        []string{"client_assertion", "actor", "subject"},
		}},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL: ports.CELAuthorizationConfig{
				Expression:        authzExpr,
				EvaluationTimeout: 2 * time.Second,
			},
		},
	}
}

// imperUnverifiedRule builds a rule accepting an unsigned unverified subject (verification: none,
// FR-003d). The issuer signs only client_assertion and actor; the subject role has no expected
// audience and never appears in signs_roles. The predicate MUST bind subject_token (FR-007a).
func imperUnverifiedRule(name, issuerURI, jwksURI, authzExpr string) ports.ImpersonationRuleConfig {
	return ports.ImpersonationRuleConfig{
		Name: name,
		Roles: map[string]ports.ImpersonationRoleConfig{
			"client_assertion": imperSignedRole("client_assertion.sub", ""),
			"actor":            imperSignedRole("actor_token.sub", ""),
			"subject": {
				Verification:        ports.SubjectVerificationNone,
				PrincipalExpression: "subject_token.sub",
				EmailExpression:     "has(subject_token.email) ? subject_token.email : ''",
			},
		},
		TrustedIssuers: []ports.TrustedTokenIssuerConfig{{
			IssuerURI:         issuerURI,
			JWKSURI:           jwksURI,
			JWKSMinRefresh:    15 * time.Minute,
			JWKSMaxRefresh:    time.Hour,
			AllowedAlgorithms: []string{"RS256", "ES256"},
			SignsRoles:        []string{"client_assertion", "actor"},
		}},
		Authorization: ports.AuthorizationConfig{
			Type: "cel",
			CEL: ports.CELAuthorizationConfig{
				Expression:        authzExpr,
				EvaluationTimeout: 2 * time.Second,
			},
		},
	}
}

// decodeJWTClaims decodes a JWT payload without verification for assertion purposes.
func decodeJWTClaims(token string) map[string]any {
	parts := strings.Split(token, ".")
	Expect(len(parts)).To(BeNumerically(">=", 2))
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	Expect(err).ToNot(HaveOccurred())
	var claims map[string]any
	Expect(json.Unmarshal(payload, &claims)).To(Succeed())
	return claims
}

// audienceContains reports whether a JWT aud claim (string or array form) contains want.
func audienceContains(aud any, want string) bool {
	switch v := aud.(type) {
	case string:
		return v == want
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

// syncBuffer is a goroutine-safe buffer for capturing audit log output.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

var _ = Describe("OAuth2 User Impersonation", func() {
	var (
		logger            *slog.Logger
		logBuf            *syncBuffer
		storageFactory    *bootstrap.StorageFactory
		testStorage       *storageadapter.Adapter
		adminServer       *bootstrap.TestServer
		enduserServer     *bootstrap.TestServer
		issuer            *helpers.MockJWKSServer
		targetAgent       = fixtures.AgentWithClientID("impersonation-target")
		targetAudience    string
		canonicalAudience string
		now               time.Time
	)

	BeforeEach(func() {
		logBuf = &syncBuffer{}
		logger = slog.New(slog.NewJSONHandler(io.MultiWriter(logBuf, io.Discard), &slog.HandlerOptions{Level: slog.LevelInfo}))
		storageFactory = bootstrap.NewStorageFactory(logger)
		issuer = helpers.NewMockJWKSServer()
		now = time.Now()

		var err error
		testStorage, err = storageFactory.NewTestStorage()
		Expect(err).ToNot(HaveOccurred())
		targetAgent = fixtures.AgentWithClientID("impersonation-target")
		canonicalID := "impersonation-target"
		targetAgent.CanonicalID = &canonicalID
		targetAgent.AllowedScopes = []string{"read", "write"}
		targetAudience = imperAudience + "/" + targetAgent.ID.String()
		canonicalAudience = imperAudience + "/" + canonicalID
		Expect(testStorage.Agents().Create(context.Background(), targetAgent)).To(Succeed())
		Expect(testStorage.UserGrants().Create(context.Background(), fixtures.ActiveGrant(
			"user-1", targetAgent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"},
		))).To(Succeed())
	})

	AfterEach(func() {
		if adminServer != nil {
			adminServer.Close()
			adminServer = nil
		}
		if enduserServer != nil {
			enduserServer.Close()
			enduserServer = nil
		}
		if issuer != nil {
			issuer.Close()
		}
		if testStorage != nil {
			_ = storageFactory.CloseStorage(testStorage)
		}
	})

	// boot builds and starts the servers for the given config, provisioning a broker signing key.
	// Returns the BuildApp error so startup-failure scenarios can assert it.
	boot := func(config *ports.Config) error {
		serverFactory := bootstrap.NewServerFactory(config, logger)
		app, err := serverFactory.BuildApp(testStorage)
		if err != nil {
			return err
		}
		adminServer, err = bootstrap.NewAdminTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		enduserServer, err = bootstrap.NewEndUserTestServer(app, logger)
		Expect(err).ToNot(HaveOccurred())
		Expect(helpers.ProvisionSigningKey(adminServer.BaseURL())).ToNot(HaveOccurred())
		return nil
	}

	// validConfig builds a bootable local-mode config with one signed rule permitting gateway-prod.
	validConfig := func() *ports.Config {
		config := fixtures.LocalConfig()
		config.Security.SkipThirdpartyHTTPSValidation = true
		config.OAuth2AuthServer.Local.TokenClaimsExpression = `principal.email == "" ? {"cel_target_agent": agent.id, "cel_target_client_id": agent.client_id, "cel_target_display_name": agent.display_name, "cel_grant": request.grant_type, "aud": "https://policy.example.com"} : {"cel_target_agent": agent.id, "cel_target_client_id": agent.client_id, "cel_target_display_name": agent.display_name, "cel_grant": request.grant_type, "aud": "https://policy.example.com", "email": principal.email}`
		config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{
			AudiencePrefix: imperAudience,
			Rules: []ports.ImpersonationRuleConfig{
				imperSignedRule("internal-gateway", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "gateway-prod"`),
			},
		}
		return config
	}

	signCred := func(sub, email, aud string, exp time.Time) string {
		claims := map[string]any{
			"iss": issuer.URL(),
			"sub": sub,
			"aud": aud,
			"iat": now.Unix(),
			"exp": exp.Unix(),
		}
		if email != "" {
			claims["email"] = email
		}
		token, err := issuer.SignJWT(claims)
		Expect(err).ToNot(HaveOccurred())
		return token
	}

	postImpersonation := func(values url.Values) *http.Response {
		resp, err := enduserServer.PublicPOST(
			"/oauth2/token",
			"application/x-www-form-urlencoded",
			strings.NewReader(values.Encode()),
		)
		Expect(err).ToNot(HaveOccurred())
		return resp
	}

	// baseRequest builds a fully valid signed impersonation request form.
	baseRequest := func() url.Values {
		return url.Values{
			"grant_type":            {imperGrant},
			"audience":              {targetAudience},
			"client_assertion_type": {imperBearerTyp},
			"client_assertion":      {signCred("gateway-prod", "", imperAudience, now.Add(time.Hour))},
			"actor_token_type":      {imperJWTType},
			"actor_token":           {signCred("actor-1", "", imperAudience, now.Add(time.Hour))},
			"subject_token_type":    {imperJWTType},
			"subject_token":         {signCred("user-1", "user@example.com", imperAudience, now.Add(time.Hour))},
		}
	}

	decodeBody := func(resp *http.Response) map[string]any {
		defer func() { _ = resp.Body.Close() }()
		var body map[string]any
		Expect(json.NewDecoder(resp.Body).Decode(&body)).To(Succeed())
		return body
	}

	seedDelegation := func(subject string) {
		Expect(testStorage.UserGrants().Create(context.Background(), fixtures.ActiveGrant(
			subject, targetAgent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"},
		))).To(Succeed())
	}

	Context("local mode with a matching signed rule", func() {
		BeforeEach(func() {
			Expect(boot(validConfig())).To(Succeed())
		})

		// Scenario US1.1 from specs/037-oauth2-user-impersonation/spec.md
		It("mints a token with sub=subject and act.iss/act.sub identifying the actor", func() {
			resp := postImpersonation(baseRequest())
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			body := decodeBody(resp)

			Expect(body["token_type"]).To(Equal("Bearer"))
			Expect(body["issued_token_type"]).To(Equal(imperAccessTyp))
			Expect(body).ToNot(HaveKey("scope"))

			claims := decodeJWTClaims(body["access_token"].(string))
			Expect(claims["sub"]).To(Equal("user-1"))
			Expect(claims["agent_id"]).To(Equal(targetAgent.ID.String()))
			Expect(claims["cel_target_agent"]).To(Equal(targetAgent.ID.String()))
			Expect(claims["cel_target_client_id"]).To(Equal(targetAgent.ClientID.String()))
			Expect(claims["cel_target_display_name"]).To(Equal(targetAgent.DisplayName))
			Expect(claims["cel_grant"]).To(Equal(imperGrant))
			Expect(audienceContains(claims["aud"], "https://policy.example.com")).To(BeTrue())
			act, ok := claims["act"].(map[string]any)
			Expect(ok).To(BeTrue())
			Expect(act["sub"]).To(Equal("actor-1"))
			Expect(act["iss"]).To(Equal(issuer.URL()))
			Expect(claims["scope"]).To(Equal(""))
		})

		// Scenario US1.1 from specs/037-oauth2-user-impersonation/spec.md
		It("should mint an equivalent token when the audience addresses the target by canonical ID", func() {
			req := baseRequest()
			req.Set("audience", canonicalAudience)
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			claims := decodeJWTClaims(decodeBody(resp)["access_token"].(string))
			Expect(claims["agent_id"]).To(Equal(targetAgent.ID.String()))
			Expect(claims["sub"]).To(Equal("user-1"))
			Expect(claims["act"].(map[string]any)["sub"]).To(Equal("actor-1"))
		})

		// Scenario US1.1 from specs/037-oauth2-user-impersonation/spec.md
		It("mints the requested scope allowed by the target agent", func() {
			req := baseRequest()
			req.Set("scope", "read")
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			body := decodeBody(resp)
			Expect(body["scope"]).To(Equal("read"))
			Expect(decodeJWTClaims(body["access_token"].(string))["scope"]).To(Equal("read"))
		})

		// Scenario US1.2 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects a scope outside the target allow list", func() {
			req := baseRequest()
			req.Set("scope", "admin")
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			body := decodeBody(resp)
			Expect(body["error"]).To(Equal("invalid_scope"))
			Expect(body).ToNot(HaveKey("access_token"))
		})

		// Scenario US1.2 from specs/037-oauth2-user-impersonation/spec.md
		It("allows requested scopes when the target allow list is empty", func() {
			targetAgent.AllowedScopes = nil
			Expect(testStorage.Agents().Update(context.Background(), targetAgent)).To(Succeed())
			req := baseRequest()
			req.Set("scope", "admin")
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			body := decodeBody(resp)
			Expect(body["scope"]).To(Equal("admin"))
			Expect(decodeJWTClaims(body["access_token"].(string))["scope"]).To(Equal("admin"))
		})

		// Scenario US1.2 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an unsupported subject_token_type with invalid_request", func() {
			req := baseRequest()
			req.Set("subject_token_type", imperAccessTyp)
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Scenario US1.2 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an absent actor_token_type with invalid_request", func() {
			req := baseRequest()
			req.Del("actor_token_type")
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Scenario US1.3 from specs/037-oauth2-user-impersonation/spec.md
		It("issues local-policy base claims and the subject email, without credential leakage", func() {
			resp := postImpersonation(baseRequest())
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			claims := decodeJWTClaims(decodeBody(resp)["access_token"].(string))
			Expect(claims).To(HaveKey("iss"))
			Expect(claims).To(HaveKey("iat"))
			Expect(claims).To(HaveKey("exp"))
			Expect(claims).To(HaveKey("jti"))
			Expect(claims["email"]).To(Equal("user@example.com"))
			Expect(claims).ToNot(HaveKey("client_assertion"))
			Expect(claims).ToNot(HaveKey("subject_token"))
		})

		// Scenario US1.4 from specs/037-oauth2-user-impersonation/spec.md
		It("does not activate impersonation for a different audience", func() {
			req := baseRequest()
			req.Set("audience", "https://example.com/other")
			resp := postImpersonation(req)
			// Falls through to normal token exchange, which is not impersonation:
			// no impersonated token is issued (normal path lacks resource → invalid_request).
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			_ = resp.Body.Close()
		})

		// Scenario US1.4 from specs/037-oauth2-user-impersonation/spec.md
		It("does not activate impersonation for an absent audience", func() {
			req := baseRequest()
			req.Del("audience")
			resp := postImpersonation(req)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			_ = resp.Body.Close()
		})

		// Edge case (target resolution) from specs/037-oauth2-user-impersonation/spec.md
		It("rejects malformed and unknown audience targets without token exchange", func() {
			for _, tc := range []struct {
				audience string
				code     string
			}{
				{imperAudience, "invalid_request"},
				{imperAudience + "/550e8400-e29b-41d4-a716-446655440000", "invalid_target"},
				{imperAudience + "/unknown-canonical-target", "invalid_target"},
				{imperAudience + "/not a canonical id", "invalid_request"},
			} {
				req := baseRequest()
				req.Set("audience", tc.audience)
				resp := postImpersonation(req)
				Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
				Expect(decodeBody(resp)["error"]).To(Equal(tc.code))
			}
		})

		// Edge case (resource present) from specs/037-oauth2-user-impersonation/spec.md
		It("rejects a request that also supplies resource with invalid_request", func() {
			req := baseRequest()
			req.Set("resource", "https://api.example.com")
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Edge case (requested_token_type mismatch) from specs/037-oauth2-user-impersonation/spec.md
		It("rejects a requested_token_type that is not the access-token type", func() {
			req := baseRequest()
			req.Set("requested_token_type", imperJWTType)
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusBadRequest))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Edge case (actor == subject) from specs/037-oauth2-user-impersonation/spec.md
		It("permits an actor identity equal to the subject identity", func() {
			req := baseRequest()
			req.Set("actor_token", signCred("user-1", "", imperAudience, now.Add(time.Hour)))
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			claims := decodeJWTClaims(decodeBody(resp)["access_token"].(string))
			Expect(claims["sub"]).To(Equal("user-1"))
			Expect(claims["act"].(map[string]any)["sub"]).To(Equal("user-1"))
		})

		// Scenario US3.1 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an unsigned subject token submitted under the RFC 8693 JWT type", func() {
			req := baseRequest()
			unsigned, err := helpers.CreateUnsignedJWT(map[string]any{
				"iss": issuer.URL(), "sub": "user-1", "aud": imperAudience,
				"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
			})
			Expect(err).ToNot(HaveOccurred())
			req.Set("subject_token", unsigned)
			resp := postImpersonation(req)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Scenario US3.2 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an expired actor token", func() {
			req := baseRequest()
			req.Set("actor_token", signCred("actor-1", "", imperAudience, now.Add(-time.Hour)))
			resp := postImpersonation(req)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_request"))
		})

		// Scenario US3.2 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects a client assertion with a wrong audience with invalid_client", func() {
			req := baseRequest()
			req.Set("client_assertion", signCred("gateway-prod", "", "https://wrong.example.com", now.Add(time.Hour)))
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_client"))
		})

		// Scenario US3.3 from specs/037-oauth2-user-impersonation/spec.md
		It("returns access_denied when no rule predicate permits the client", func() {
			req := baseRequest()
			req.Set("client_assertion", signCred("gateway-untrusted", "", imperAudience, now.Add(time.Hour)))
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			Expect(decodeBody(resp)["error"]).To(Equal("access_denied"))
		})

		// Scenario US3.4 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects when the subject identity cannot be extracted", func() {
			req := baseRequest()
			// subject token missing the sub claim → extraction yields empty identity.
			noSub, err := issuer.SignJWT(map[string]any{
				"iss": issuer.URL(), "aud": imperAudience,
				"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
			})
			Expect(err).ToNot(HaveOccurred())
			req.Set("subject_token", noSub)
			resp := postImpersonation(req)
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			body := decodeBody(resp)
			Expect(body).To(HaveKey("error"))
			Expect(body).ToNot(HaveKey("access_token"))
		})

		// Scenario US4.1 from specs/037-oauth2-user-impersonation/spec.md
		It("emits a credential-free audit event on success", func() {
			req := baseRequest()
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			accessToken := decodeBody(resp)["access_token"].(string)

			logs := logBuf.String()
			Expect(logs).To(ContainSubstring("impersonation_decision"))
			Expect(logs).To(ContainSubstring("internal-gateway"))
			Expect(logs).To(ContainSubstring("gateway-prod"))
			Expect(logs).To(ContainSubstring(targetAgent.ID.String()))
			Expect(logs).ToNot(ContainSubstring(req.Get("client_assertion")))
			Expect(logs).ToNot(ContainSubstring(req.Get("subject_token")))
			Expect(logs).ToNot(ContainSubstring(accessToken))
		})

		// Scenario US4.2 from specs/037-oauth2-user-impersonation/spec.md
		It("emits a credential-free audit event on failure", func() {
			req := baseRequest()
			req.Set("client_assertion", signCred("gateway-untrusted", "", imperAudience, now.Add(time.Hour)))
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			logs := logBuf.String()
			Expect(logs).To(ContainSubstring("impersonation_decision"))
			Expect(logs).To(ContainSubstring("access_denied"))
			Expect(logs).ToNot(ContainSubstring(req.Get("client_assertion")))
		})

		// Scenario US2.3 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects a client assertion from an issuer the rule does not trust", func() {
			untrusted := helpers.NewMockJWKSServer()
			defer untrusted.Close()
			req := baseRequest()
			token, err := untrusted.SignJWT(map[string]any{
				"iss": untrusted.URL(), "sub": "gateway-prod", "aud": imperAudience,
				"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
			})
			Expect(err).ToNot(HaveOccurred())
			req.Set("client_assertion", token)
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusUnauthorized))
			Expect(decodeBody(resp)["error"]).To(Equal("invalid_client"))
		})

		// Scenario US4.3 from specs/037-oauth2-user-impersonation/spec.md
		It("never records any submitted or issued token value in the audit event", func() {
			req := baseRequest()
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			accessToken := decodeBody(resp)["access_token"].(string)
			logs := logBuf.String()
			Expect(logs).To(ContainSubstring("impersonation_decision"))
			for _, credential := range []string{
				req.Get("client_assertion"), req.Get("actor_token"), req.Get("subject_token"), accessToken,
			} {
				Expect(logs).ToNot(ContainSubstring(credential))
			}
		})

		// SC-001 from specs/037-oauth2-user-impersonation/spec.md
		It("serves 100 concurrent signed impersonation requests within the SC-001 budget", Label("performance"), func() {
			form := baseRequest().Encode() // sign once; the same valid request is replayed
			// Warm the JWKS cache and signing key so we measure a warmed deployment (SC-001).
			warm := postImpersonation(baseRequest())
			Expect(warm).To(matchers.HaveStatusCode(http.StatusOK))
			_ = warm.Body.Close()

			const n = 100
			durations := make([]time.Duration, n)
			statuses := make([]int, n)
			var wg sync.WaitGroup
			for i := range n {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()
					start := time.Now()
					resp, err := enduserServer.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form))
					durations[i] = time.Since(start)
					if err == nil {
						statuses[i] = resp.StatusCode
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
					}
				}(i)
			}
			wg.Wait()

			ok := 0
			for _, s := range statuses {
				if s == http.StatusOK {
					ok++
				}
			}
			sorted := append([]time.Duration(nil), durations...)
			sort.Slice(sorted, func(a, b int) bool { return sorted[a] < sorted[b] })
			p95 := sorted[94]
			GinkgoWriter.Printf("SC-001: %d/%d succeeded; p95=%s max=%s min=%s\n", ok, n, p95, sorted[n-1], sorted[0])
			Expect(ok).To(BeNumerically(">=", 95), "at least 95%% of requests must mint a token")
			Expect(p95).To(BeNumerically("<", 500*time.Millisecond), "95th percentile must complete under 500ms")
		})
	})

	Context("first-match rule evaluation", func() {
		// Scenario US2.5 from specs/037-oauth2-user-impersonation/spec.md
		It("falls through the first rule and applies the second, sharing an issuer", func() {
			config := validConfig()
			// Rule 1 only permits gateway-A; rule 2 permits gateway-prod; both trust the same issuer.
			config.OAuth2AuthServer.Impersonation.Rules = []ports.ImpersonationRuleConfig{
				imperSignedRule("rule-a", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "gateway-A"`),
				imperSignedRule("rule-b", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "gateway-prod"`),
			}
			Expect(boot(config)).To(Succeed())

			resp := postImpersonation(baseRequest())
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(decodeJWTClaims(decodeBody(resp)["access_token"].(string))["sub"]).To(Equal("user-1"))
		})

		// Scenario US2.5 from specs/037-oauth2-user-impersonation/spec.md
		It("applies only the first fully matching rule", func() {
			config := validConfig()
			second := imperSignedRule("rule-second", issuer.URL(), issuer.JWKSURL(), "true")
			secondSubject := second.Roles["subject"]
			secondSubject.PrincipalExpression = "subject_token.sub"
			second.Roles["subject"] = secondSubject
			config.OAuth2AuthServer.Impersonation.Rules = []ports.ImpersonationRuleConfig{
				imperSignedRule("rule-first", issuer.URL(), issuer.JWKSURL(), "true"),
				second,
			}
			Expect(boot(config)).To(Succeed())

			resp := postImpersonation(baseRequest())
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(decodeJWTClaims(decodeBody(resp)["access_token"].(string))["sub"]).To(Equal("user-1"))
			logs := logBuf.String()
			Expect(strings.Count(logs, `"event":"impersonation_decision"`)).To(Equal(1))
			Expect(logs).To(ContainSubstring(`"rule":"rule-first"`))
			Expect(logs).ToNot(ContainSubstring(`"rule":"rule-second"`))
		})

		// Edge case (JWKS unavailable → fall through) from specs/037-oauth2-user-impersonation/spec.md
		It("falls through a rule whose issuer JWKS is unavailable to a rule whose issuer is reachable", func() {
			dead := helpers.NewMockJWKSServer()
			deadJWKSURL := dead.JWKSURL()
			dead.Close() // this issuer's JWKS can never be fetched

			config := validConfig()
			// Both rules trust the same issuer_uri and permit gateway-prod, but rule-a resolves the
			// issuer's keys from a dead JWKS endpoint (distinct provider) while rule-b uses the live one.
			config.OAuth2AuthServer.Impersonation.Rules = []ports.ImpersonationRuleConfig{
				imperSignedRule("rule-dead-jwks", issuer.URL(), deadJWKSURL, `client_assertion.sub == "gateway-prod"`),
				imperSignedRule("rule-live-jwks", issuer.URL(), issuer.JWKSURL(), `client_assertion.sub == "gateway-prod"`),
			}
			Expect(boot(config)).To(Succeed())

			resp := postImpersonation(baseRequest())
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			Expect(decodeJWTClaims(decodeBody(resp)["access_token"].(string))["sub"]).To(Equal("user-1"))
		})
	})

	Context("user delegation enforcement", func() {
		// Scenario US5.2 from specs/037-oauth2-user-impersonation/spec.md
		It("should reject a rule-authorized subject without a delegation and provide the consent URL", func() {
			config := validConfig()
			Expect(boot(config)).To(Succeed())
			req := baseRequest()
			req.Set("subject_token", signCred("unconsented-user", "", imperAudience, now.Add(time.Hour)))

			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			body := decodeBody(resp)
			Expect(body["error"]).To(Equal("access_denied"))
			Expect(body["error_uri"]).To(Equal(config.Server.EndUser.PublicURL + "/agents/" + targetAgent.ID.String()))
			Expect(body).ToNot(HaveKey("access_token"))
			Expect(logBuf.String()).To(ContainSubstring("user_grant_missing"))
		})

		// Scenario US5.3 from specs/037-oauth2-user-impersonation/spec.md
		It("should reject an expired subject delegation without revealing its prior existence", func() {
			config := validConfig()
			Expect(testStorage.UserGrants().Create(context.Background(), fixtures.ExpiredGrant(
				"expired-user", targetAgent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"},
			))).To(Succeed())
			Expect(boot(config)).To(Succeed())
			req := baseRequest()
			req.Set("subject_token", signCred("expired-user", "", imperAudience, now.Add(time.Hour)))

			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			body := decodeBody(resp)
			Expect(body["error"]).To(Equal("access_denied"))
			Expect(body["error_uri"]).To(Equal(config.Server.EndUser.PublicURL + "/agents/" + targetAgent.ID.String()))
			Expect(body).ToNot(HaveKey("access_token"))
			Expect(logBuf.String()).To(ContainSubstring("user_grant_expired"))
		})
	})

	Context("when delegation storage cannot be queried", Label("docker"), func() {
		var postgres *bootstrap.PostgresFixture

		BeforeEach(func() {
			if err := bootstrap.CanAccessContainerRuntime(); err != nil {
				Skip("Skipping delegation-storage E2E scenario: " + err.Error())
			}

			ctx := context.Background()
			Expect(storageFactory.CloseStorage(testStorage)).To(Succeed())
			testStorage = nil

			var err error
			postgres, err = bootstrap.NewPostgresFixture(ctx)
			Expect(err).ToNot(HaveOccurred())
			connectionURL := postgres.ConnectionURL

			storageConfig := ports.StorageConfig{
				Backend: "postgres",
				Postgres: ports.PostgresConfig{
					ConnectionURL: connectionURL,
				},
				Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second},
			}
			testStorage, err = storageadapter.NewAdapter(&storageConfig)
			Expect(err).ToNot(HaveOccurred())
			Expect(fixtures.SeedPlaceholderGrantData(ctx, testStorage)).To(Succeed())
			Expect(testStorage.Agents().Create(ctx, targetAgent)).To(Succeed())

			config := validConfig()
			config.Storage = storageConfig
			Expect(boot(config)).To(Succeed())

			db, err := sql.Open("pgx", connectionURL)
			Expect(err).ToNot(HaveOccurred())
			defer func() { Expect(db.Close()).To(Succeed()) }()
			_, err = db.ExecContext(ctx, "ALTER TABLE user_grants RENAME TO user_grants_unavailable")
			Expect(err).ToNot(HaveOccurred())
		})

		AfterEach(func() {
			Expect(postgres.Close(context.Background())).To(Succeed())
			postgres = nil
		})

		// Scenario US5.5 from specs/037-oauth2-user-impersonation/spec.md
		It("should fail closed when delegation storage cannot be queried", func() {
			req := baseRequest()
			resp := postImpersonation(req)
			Expect(resp).To(matchers.HaveStatusCode(http.StatusInternalServerError))
			body := decodeBody(resp)
			Expect(body["error"]).To(Equal("server_error"))
			Expect(body).ToNot(HaveKey("access_token"))
			Expect(body).ToNot(HaveKey("error_uri"))
			Expect(logBuf.String()).To(ContainSubstring(`"failure_category":"user_grant_lookup_failed"`))
			Expect(logBuf.String()).ToNot(ContainSubstring(req.Get("client_assertion")))
			Expect(logBuf.String()).ToNot(ContainSubstring(req.Get("actor_token")))
			Expect(logBuf.String()).ToNot(ContainSubstring(req.Get("subject_token")))
		})
	})

	Context("configuration validation at startup", func() {
		// Scenario US2.4 from specs/037-oauth2-user-impersonation/spec.md
		It("fails startup when a trusted issuer omits allowed_algorithms", func() {
			config := validConfig()
			config.OAuth2AuthServer.Impersonation.Rules[0].TrustedIssuers[0].AllowedAlgorithms = nil
			err := boot(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("allowed_algorithms"))
		})

		// Scenario US2.4 from specs/037-oauth2-user-impersonation/spec.md
		It("fails startup when a rule omits its authorization predicate", func() {
			config := validConfig()
			config.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression = ""
			err := boot(config)
			Expect(err).To(HaveOccurred())
		})

		// Configuration edge (HS256) from specs/037-oauth2-user-impersonation/spec.md
		It("fails startup when a trusted issuer allows a symmetric algorithm", func() {
			config := validConfig()
			config.OAuth2AuthServer.Impersonation.Rules[0].TrustedIssuers[0].AllowedAlgorithms = []string{"HS256"}
			err := boot(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("allowed_algorithms"))
		})
	})

	Context("proxy mode boundary", func() {
		// Scenario US1.5 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects impersonation configuration outside local mode", func() {
			upstream := helpers.NewMockUpstreamOAuth2Server()
			defer upstream.Close()
			config := fixtures.HybridConfig(upstream.URL())
			config.Security.SkipThirdpartyHTTPSValidation = true
			config.OAuth2AuthServer.Impersonation = &ports.ImpersonationConfig{
				AudiencePrefix: imperAudience,
				Rules:          []ports.ImpersonationRuleConfig{imperSignedRule("r", issuer.URL(), issuer.JWKSURL(), "true")},
			}
			err := boot(config)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("issuer signing metadata unavailable", func() {
		// Edge case (JWKS unavailable → fail closed) from specs/037-oauth2-user-impersonation/spec.md
		It("fails closed when a trusted issuer's JWKS cannot be fetched", func() {
			config := validConfig() // captures issuer URLs while still valid
			// Take the issuer's JWKS endpoint offline before startup so the broker can never
			// fetch its keys; signing still works from the in-memory key.
			issuer.Close()
			Expect(boot(config)).To(Succeed())

			resp := postImpersonation(baseRequest())
			issuer = nil // already closed; prevent AfterEach double-close
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			body := decodeBody(resp)
			Expect(body).To(HaveKey("error"))
			Expect(body).ToNot(HaveKey("access_token"))
		})
	})

	Context("unverified subject mode (broker profile extension)", func() {

		// unverifiedConfig configures one rule accepting an unverified subject; the predicate binds
		// subject_token (FR-007a) and subject_token.email (FR-006a), permitting @example.com subjects.
		unverifiedConfig := func() *ports.Config {
			config := validConfig()
			config.OAuth2AuthServer.Impersonation.Rules = []ports.ImpersonationRuleConfig{
				imperUnverifiedRule("chat-bridge", issuer.URL(), issuer.JWKSURL(),
					`client_assertion.sub == "gateway-prod" && subject_token.sub != "" && `+
						`has(subject_token.email) && subject_token.email.endsWith("@example.com")`),
			}
			return config
		}

		unverifiedSubjectToken := func(sub, email string) string {
			claims := map[string]any{"sub": sub}
			if email != "" {
				claims["email"] = email
			}
			token, err := helpers.CreateUnsignedJWT(claims)
			Expect(err).ToNot(HaveOccurred())
			return token
		}

		unverifiedRequest := func(subjectToken string) url.Values {
			req := baseRequest()
			req.Set("subject_token_type", imperJWTType)
			req.Set("subject_token", subjectToken)
			return req
		}

		// Scenario US1.6 from specs/037-oauth2-user-impersonation/spec.md
		It("mints a token from an unsigned unverified subject carrying principal id and email", func() {
			Expect(boot(unverifiedConfig())).To(Succeed())
			seedDelegation("chat-user-1")
			resp := postImpersonation(unverifiedRequest(unverifiedSubjectToken("chat-user-1", "chat-user-1@example.com")))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			body := decodeBody(resp)
			Expect(body).ToNot(HaveKey("scope"))
			claims := decodeJWTClaims(body["access_token"].(string))
			Expect(claims["sub"]).To(Equal("chat-user-1"))
			Expect(claims["email"]).To(Equal("chat-user-1@example.com"))
			Expect(audienceContains(claims["aud"], "https://policy.example.com")).To(BeTrue())
			Expect(claims["act"].(map[string]any)["sub"]).To(Equal("actor-1"))
			Expect(claims["act"].(map[string]any)["iss"]).To(Equal(issuer.URL()))
			Expect(claims["scope"]).To(Equal(""))
		})

		// Scenario US3.5 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an unverified subject when the configured rules only accept signed subjects", func() {
			Expect(boot(validConfig())).To(Succeed()) // only the signed-subject rule is configured
			resp := postImpersonation(unverifiedRequest(unverifiedSubjectToken("chat-user-1", "chat-user-1@example.com")))
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeBody(resp)).To(HaveKey("error"))
		})

		// Scenario US3.5 from specs/037-oauth2-user-impersonation/spec.md
		It("rejects an unverified subject the authorization predicate does not permit", func() {
			Expect(boot(unverifiedConfig())).To(Succeed())
			resp := postImpersonation(unverifiedRequest(unverifiedSubjectToken("chat-user-1", "user@notexample.com")))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			Expect(decodeBody(resp)["error"]).To(Equal("access_denied"))
		})

		// Scenario US3.5 from specs/037-oauth2-user-impersonation/spec.md — a signed JWS under
		// the JWT type must never enter the unverified route (no signature bypass).
		It("rejects a signed JWS submitted under the JWT type", func() {
			Expect(boot(unverifiedConfig())).To(Succeed())
			signedSubject := signCred("chat-user-1", "chat-user-1@example.com", imperAudience, now.Add(time.Hour))
			resp := postImpersonation(unverifiedRequest(signedSubject))
			Expect(resp.StatusCode).ToNot(Equal(http.StatusOK))
			Expect(decodeBody(resp)).To(HaveKey("error"))
		})

		// FR-007a/CR-005a from specs/037-oauth2-user-impersonation/spec.md
		It("fails startup when an unverified rule's predicate does not reference subject_token", func() {
			config := unverifiedConfig()
			config.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression = `client_assertion.sub == "gateway-prod"`
			err := boot(config)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("subject_token"))
		})

		// FR-006a from specs/037-oauth2-user-impersonation/spec.md — a caller-controlled email is not
		// minted unless the predicate binds subject_token.email.
		It("does not mint a caller-supplied email that the predicate does not bind", func() {
			config := unverifiedConfig()
			config.OAuth2AuthServer.Impersonation.Rules[0].Authorization.CEL.Expression =
				`client_assertion.sub == "gateway-prod" && subject_token.sub != ""`
			Expect(boot(config)).To(Succeed())
			seedDelegation("chat-user-1")
			resp := postImpersonation(unverifiedRequest(unverifiedSubjectToken("chat-user-1", "chat-user-1@example.com")))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusOK))
			claims := decodeJWTClaims(decodeBody(resp)["access_token"].(string))
			Expect(claims["sub"]).To(Equal("chat-user-1"))
			Expect(claims).ToNot(HaveKey("email"))
		})

		// Scenario US5.4 from specs/037-oauth2-user-impersonation/spec.md
		It("should reject an unverified subject without a delegation", func() {
			config := unverifiedConfig()
			Expect(boot(config)).To(Succeed())
			resp := postImpersonation(unverifiedRequest(unverifiedSubjectToken("unconsented-chat-user", "unconsented-chat-user@example.com")))
			Expect(resp).To(matchers.HaveStatusCode(http.StatusForbidden))
			body := decodeBody(resp)
			Expect(body["error"]).To(Equal("access_denied"))
			Expect(body["error_uri"]).To(Equal(config.Server.EndUser.PublicURL + "/agents/" + targetAgent.ID.String()))
			Expect(body).ToNot(HaveKey("access_token"))
		})
	})

	// Guard against unused import of context in some build configurations.
	_ = context.Background
})
