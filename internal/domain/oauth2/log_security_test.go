package oauth2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/lestrrat-go/jwx/v4/jwk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/cimd"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	querySecretClientURI = "https://agent.example.com/client?marker=query-secret"
	redactedClientURI    = "https://agent.example.com/client"
)

func TestAgentClientResolver_CIMDLogsNoURLQuery(t *testing.T) {
	tests := []struct {
		name    string
		repoErr error
		fetch   *ports.CIMDFetchResult
	}{
		{
			name:    "ambiguous client URI",
			repoErr: storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConflict, errors.New("conflict"), "conflict"),
		},
		{
			name:    "repository failure",
			repoErr: storage.NewStorageError("GetAgentByClientURI", storage.ErrorKindConnection, errors.New("connection lost"), "connection lost"),
		},
		{
			name:  "document validation failure",
			fetch: &ports.CIMDFetchResult{Body: []byte(`{"client_id":"https://agent.example.com/other?marker=query-secret","redirect_uris":["https://agent.example.com/cb"]}`)},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			agent := &storage.Agent{ID: id.NewAgentID(), DisplayName: "Test Agent"}
			repo := NewMockAgentRepository()
			require.NoError(t, repo.Create(context.Background(), agent))
			repo.RegisterURI(querySecretClientURI, agent)
			repo.getByClientURIErr = test.repoErr

			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			cache, err := cimd.NewCIMDCache(60*time.Second, time.Hour, 1000)
			require.NoError(t, err)
			svc := cimd.NewService(&mockCIMDFetcher{result: test.fetch}, cache, nil, logger)
			resolver := NewAgentClientResolverWithCIMD(repo, svc, logger)

			_, err = resolver.ResolveClient(context.Background(), id.ClientID(querySecretClientURI))
			require.Error(t, err)

			out := logs.String()
			assert.NotContains(t, out, "query-secret")
			assert.Contains(t, out, `"client_uri":"`+redactedClientURI+`"`)
		})
	}
}

func TestAuthorizationService_ErrorRedirectFailureLogsNoRedirectURIQuery(t *testing.T) {
	var logs bytes.Buffer
	svc := &AuthorizationService{logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	agentID := id.NewAgentID()
	req := &ports.AuthorizationRequest{
		RedirectURI: "https://user:pass@[::1/cb?code=query-secret#fragment-secret",
		State:       "state-secret",
	}

	redirect := svc.errorRedirectURL(context.Background(), req, agentID, "server_error", "server error")

	assert.Empty(t, redirect)
	out := logs.String()
	assert.NotContains(t, out, "query-secret")
	assert.NotContains(t, out, "fragment-secret")
	assert.NotContains(t, out, "pass")
	assert.NotContains(t, out, "state-secret")
	assert.NotContains(t, out, "::1")
	assert.Contains(t, out, `"agent_id":"`+agentID.String()+`"`)
	assert.Contains(t, out, `"reason":"invalid_redirect_uri"`)
}

func TestAuthorizationService_ErrorRedirectURLBuildsRedirect(t *testing.T) {
	svc := &AuthorizationService{logger: slog.New(slog.DiscardHandler)}
	req := &ports.AuthorizationRequest{RedirectURI: "https://agent.example.com/cb", State: "xyz"}

	redirect := svc.errorRedirectURL(context.Background(), req, id.NewAgentID(), "invalid_scope", "requested scope is not permitted")

	assert.Equal(t, "https://agent.example.com/cb?error=invalid_scope&error_description=requested+scope+is+not+permitted&state=xyz", redirect)
}

func TestPublishJWKS_Proxy_UpstreamErrorLogsNoURLQueryOrCause(t *testing.T) {
	var logs bytes.Buffer
	upstream := &mockPublisherJWKSPort{
		getFn: func(_ context.Context) (jwk.Set, error) {
			return nil, fmt.Errorf("failed to refresh resource %q: %w", "https://idp.example.com/jwks?tenant=query-secret", context.DeadlineExceeded)
		},
	}
	svc := NewProxyJWKSPublisher(upstream, slog.New(slog.NewJSONHandler(&logs, nil)))

	_, err := svc.PublishJWKS(context.Background())
	require.Error(t, err)

	assert.NotContains(t, logs.String(), "query-secret")
	assert.Contains(t, logs.String(), `"error_kind":"timeout"`)
}
