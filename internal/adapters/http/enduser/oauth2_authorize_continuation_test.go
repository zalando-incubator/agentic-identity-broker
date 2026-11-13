package enduser

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/sessiontoken"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ptr"
)

// FR-011: these regressions preserve existing continuation behavior and redirect precedence.
func TestAuthorizeContinuationFailureContract(t *testing.T) {
	for _, withState := range []bool{false, true} {
		for _, variant := range []string{"forged", "malformed", "expired", "request mismatch", "principal mismatch", "agent mismatch", "binding mismatch", "unregistered callback"} {
			t.Run(variant+map[bool]string{true: "/state", false: "/no-state"}[withState], func(t *testing.T) {
				const callback = "https://client.example.com/callback"
				service := &model.ThirdpartyOAuth2ProviderEntity{ID: id.NewServiceID(), CanonicalID: ptr.To("github")}
				agent := &storage.Agent{ID: id.NewAgentID(), ClientID: ptr.To(id.ClientID("upstream-client")), RedirectURIs: []string{callback}, ServiceRequirements: []storage.ServiceRequirement{{ServiceID: service.ID}}}
				agents := newMockAgentRepo()
				require.NoError(t, agents.Create(context.Background(), agent))
				query := url.Values{"client_id": {agent.ID.String()}, "redirect_uri": {callback}, "response_type": {"code"}, "scope": {"https://broker.example.com/sessions/github"}}
				if withState {
					query.Set("state", "synthetic-original-state")
				}
				original := "/oauth2/authorize?" + query.Encode()
				claims, err := sessiontoken.NewAuthorizationSessionClaims(agent.ID, id.Principal("alice@example.com"), original, nil)
				require.NoError(t, err)
				claims.ServiceScopeBindings = map[string]id.ServiceID{query.Get("scope"): service.ID}
				switch variant {
				case "expired":
					claims.ExpiresAt = time.Now().Add(-time.Minute)
				case "principal mismatch":
					claims.Principal = "mallory@example.com"
				case "agent mismatch":
					claims.AgentID = id.NewAgentID()
				case "binding mismatch":
					claims.ServiceScopeBindings[query.Get("scope")] = id.NewServiceID()
				case "request mismatch":
					query.Set("scope", query.Get("scope")+" profile")
				case "unregistered callback":
					query.Set("redirect_uri", "https://unregistered.invalid/callback")
				}
				tokens := newTestSessionTokenSvc()
				token, err := tokens.Create(claims)
				require.NoError(t, err)
				if variant == "forged" {
					token = token[:len(token)-2] + "xx"
				}
				if variant == "malformed" || variant == "unregistered callback" {
					token = "synthetic-malformed-JWE"
				}
				query.Set("session_token", token)
				svc := oauth2.NewAuthorizationService(newMockGrantRepo(), &noopSessionRepository{}, oauth2.NewAgentClientResolver(agents, slog.New(slog.DiscardHandler)), &oauth2.OAuth2Config{ModeStrategy: oauth2.NewProxyModeStrategy(), PublicURL: "https://broker.example.com", UpstreamAuthorizeEndpoint: "https://upstream.invalid/authorize"}, nil, tokens).
					WithRequestedServices(servicescope.NewResolver(&proxyScopeService{service: service}, proxyScopePermissions{}, "https://broker.example.com"), nil)
				proceed := &captureProceedStrategy{}
				handler := &OAuth2AuthorizeHandler{Service: svc, ProceedHandler: proceed}
				request := httptest.NewRequest(http.MethodGet, "/oauth2/authorize?"+query.Encode(), nil)
				request = request.WithContext(principal.WithPrincipal(request.Context(), "alice@example.com"))
				recorder := httptest.NewRecorder()
				handler.ServeHTTP(recorder, request)
				assert.False(t, proceed.called, "no code or upstream authorization on refusal")
				assert.NotContains(t, recorder.Header().Get("Location"), token)
				if variant == "unregistered callback" {
					assert.Equal(t, http.StatusBadRequest, recorder.Code)
					assert.Empty(t, recorder.Header().Get("Location"))
					assert.Contains(t, recorder.Body.String(), "invalid_redirect_uri")
					return
				}
				require.Equal(t, http.StatusFound, recorder.Code)
				location, err := url.Parse(recorder.Header().Get("Location"))
				require.NoError(t, err)
				assert.Equal(t, callback, location.Scheme+"://"+location.Host+location.Path)
				code, description := "access_denied", "authorization session is invalid or expired"
				if variant == "binding mismatch" {
					code, description = "invalid_scope", "requested service scope is not permitted"
				}
				assert.Equal(t, code, location.Query().Get("error"))
				assert.Equal(t, description, location.Query().Get("error_description"))
				assert.Equal(t, withState, location.Query().Has("state"))
				assert.Equal(t, query.Get("state"), location.Query().Get("state"))
				for _, forbidden := range []string{"session_token", "code", "code_verifier", "code_challenge"} {
					assert.False(t, location.Query().Has(forbidden))
				}
			})
		}
	}
}
