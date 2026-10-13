//go:build integration

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	e2ebootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/bootstrap"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/fixtures"
	"github.com/agentic-identity-broker/agentic-identity-broker/tests/e2e/helpers"
)

func TestRefreshConsentEuropeBerlin(t *testing.T) {
	pg, dbName, connStr, cleanupDB := setupCIMDKeyDomainDatabase(t, 36)
	defer cleanupDB()
	pg.ExecuteSQL(t, dbName, fmt.Sprintf("ALTER DATABASE %s SET timezone TO 'Europe/Berlin'", dbName))
	require.Equal(t, "Europe/Berlin", pg.QuerySQL(t, dbName, "SHOW timezone"))
	store, cleanupStore := newCIMDKeyDomainStorage(t, connStr)
	defer cleanupStore()
	ctx := context.Background()
	require.NoError(t, fixtures.SeedPlaceholderGrantData(ctx, store))
	agent := fixtures.LocalAgent()
	require.NoError(t, store.Agents().Create(ctx, agent))
	principal := fixtures.DefaultPrincipal().String()
	now := time.Now().UTC()
	grant := fixtures.ActiveGrant(principal, agent.ID.String(), fixtures.PlaceholderServiceID.String(), []string{"read"})
	grant.CreatedAt, grant.UpdatedAt = now.Add(-time.Hour), now.Add(-time.Hour)
	validUntil := now.Add(time.Hour)
	grant.ValidUntil = &validUntil
	require.NoError(t, store.UserGrants().Create(ctx, grant))
	logger := e2ebootstrap.TestLogger(slog.LevelInfo)
	config := fixtures.LocalConfig()
	config.Storage.Backend = "postgres"
	config.Storage.Postgres.ConnectionURL = connStr
	application, err := e2ebootstrap.NewServerFactory(config, logger).BuildApp(store)
	require.NoError(t, err)
	admin, err := e2ebootstrap.NewAdminTestServer(application, logger)
	require.NoError(t, err)
	defer admin.Close()
	enduser, err := e2ebootstrap.NewEndUserTestServer(application, logger)
	require.NoError(t, err)
	defer enduser.Close()
	require.NoError(t, helpers.ProvisionSigningKey(admin.BaseURL()))

	postToken := func(form url.Values) (int, map[string]any) {
		form.Set("client_id", agent.ID.String())
		resp, err := enduser.PublicPOST("/oauth2/token", "application/x-www-form-urlencoded", strings.NewReader(form.Encode()))
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		return resp.StatusCode, body
	}
	issue := func() string {
		verifier := helpers.PKCEVerifier()
		resp, err := enduser.AuthenticatedGET("/oauth2/authorize?"+url.Values{
			"response_type": {"code"}, "client_id": {agent.ID.String()}, "redirect_uri": {agent.RedirectURIs[0]},
			"scope": {"read offline_access"}, "code_challenge": {helpers.GenerateCodeChallenge(verifier)}, "code_challenge_method": {"S256"},
		}.Encode(), principal)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusFound, resp.StatusCode)
		location, err := helpers.ExtractRedirectURL(resp)
		require.NoError(t, err)
		code := location.Query().Get("code")
		require.NotEmpty(t, code)
		status, body := postToken(url.Values{
			"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {agent.RedirectURIs[0]}, "code_verifier": {verifier},
		})
		require.Equal(t, http.StatusOK, status, "OAuth2 error: %v", body["error"])
		require.NotEmpty(t, body["access_token"])
		require.NotEmpty(t, body["refresh_token"])
		return body["refresh_token"].(string)
	}
	refresh := func(token string) (int, map[string]any) {
		return postToken(url.Values{"grant_type": {"refresh_token"}, "refresh_token": {token}})
	}
	makeLegacy := func(token string, createdAt time.Time) {
		signature := (&oauth2server.RandomRefreshTokenStrategy{}).RefreshTokenSignature(ctx, token)
		pg.ExecuteSQL(t, dbName, fmt.Sprintf("UPDATE refresh_token_sessions SET grant_id = NULL, created_at = '%s' WHERE signature = '%s'", createdAt.Format(time.RFC3339Nano), signature))
	}

	legacy := issue()
	makeLegacy(legacy, now.Add(-30*time.Minute))
	status, body := refresh(legacy)
	require.Equal(t, http.StatusOK, status, "OAuth2 error: %v", body["error"])
	require.NotEmpty(t, body["access_token"])
	require.NotEmpty(t, body["refresh_token"])
	rotated := body["refresh_token"].(string)
	require.False(t, legacy == rotated, "refresh must rotate the token")
	signature := (&oauth2server.RandomRefreshTokenStrategy{}).RefreshTokenSignature(ctx, rotated)
	successor, err := store.RefreshTokenSessions().FindBySignature(ctx, signature)
	require.NoError(t, err)
	require.Equal(t, &grant.ID, successor.GrantID)

	tooOld := issue()
	makeLegacy(tooOld, grant.CreatedAt.Add(-time.Minute))
	status, body = refresh(tooOld)
	require.Equal(t, http.StatusBadRequest, status, "OAuth2 error: %v", body["error"])
	require.Equal(t, "invalid_grant", body["error"])
	require.NotContains(t, body, "access_token")
	require.NotContains(t, body, "refresh_token")

	resp, err := enduser.DirectRequest(http.MethodDelete, "/api/consent/agents/"+agent.ID.String()+"/grants", principal, nil, nil)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
	status, body = refresh(rotated)
	require.Equal(t, http.StatusBadRequest, status, "OAuth2 error: %v", body["error"])
	require.Equal(t, "invalid_grant", body["error"])
	require.NotContains(t, body, "access_token")
	require.NotContains(t, body, "refresh_token")
}
