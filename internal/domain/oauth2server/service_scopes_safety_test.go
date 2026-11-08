package oauth2server

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/servicescope"
)

type unsafeRefreshScopeServices struct{ cause error }

func (s unsafeRefreshScopeServices) Get(context.Context, id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}
func (s unsafeRefreshScopeServices) GetByCanonicalID(context.Context, string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	return nil, s.cause
}

func TestRefreshScopeDependencyCauseNotLogged(t *testing.T) {
	const secret = "synthetic-provider-refresh-secret-body-JWE"
	var logs bytes.Buffer
	f := newServiceScopeFixture(t, slog.New(slog.NewJSONHandler(&logs, nil)))
	code, err := f.authorize(t, "read offline_access "+githubScope, map[string]id.ServiceID{githubScope: scopeGitHubID})
	require.NoError(t, err)
	issued, err := f.exchange(code)
	require.NoError(t, err)
	logs.Reset()
	f.provider.WithServiceScopes(servicescope.NewResolver(unsafeRefreshScopeServices{cause: errors.New(secret)}, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, f.snapshots))
	_, err = f.refresh(issued.RefreshToken, "")
	requireRFC6749(t, err, "server_error", "requested service usability cannot be verified")
	assert.NotContains(t, logs.String(), secret)
	assert.NotContains(t, logs.String(), issued.AccessToken)
	assert.NotContains(t, logs.String(), issued.RefreshToken)
	assert.Contains(t, logs.String(), `"event":"oauth2_server.service.recheck_failed"`)
	assert.Contains(t, logs.String(), `"level":"ERROR"`)
	assert.Contains(t, logs.String(), `"reason":"service_lookup_failed"`)
	assert.Contains(t, logs.String(), `"error_kind":"transient"`)
	f.provider.WithServiceScopes(servicescope.NewResolver(f.services, scopePermissionSets{}, scopePublicURL), servicescope.NewEvaluator(scopePermissionSets{}, f.snapshots))
	_, err = f.refresh(issued.RefreshToken, "")
	require.NoError(t, err, "dependency refusal must not consume the token")
}
