package oauth2session_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/lestrrat-go/jwx/v4/jwa"
	"github.com/lestrrat-go/jwx/v4/jwt"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/cimdclient"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestCredentialExclusions_PublicClientConnectsAndRefreshesWithoutSecretOrFileAccess(t *testing.T) {
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "absent-id"), ClientSecretFile: filepath.Join(t.TempDir(), "absent-secret")}
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"public-service": binding})
	provider := createTestService(id.NewServiceID())
	canonicalID := "public-service"
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = model.CredentialSourceStored
	provider.ClientID = "public-client"
	provider.TokenEndpointAuthMethod = model.TokenEndpointAuthMethodNone
	provider.Secret = model.NewAbsentSecret()
	provider.Endpoints.TokenEndpoint = h.endpoint.server.URL
	require.NoError(t, h.providers.Create(context.Background(), provider))
	before, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	principal := id.Principal("public@example.com")
	flow := h.initiate(t, principal, provider.ID)
	authorization, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "public-client", authorization.Query().Get("client_id"))
	assert.NotEmpty(t, authorization.Query().Get("code_challenge"))
	h.callback(t, principal, provider.ID, flow.StateToken)
	_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	h.expire(t, principal, provider.ID)
	_, token, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "credential-access", token)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, "public-client", request.form.Get("client_id"))
		assert.Empty(t, request.secret)
		assert.NotContains(t, request.form, "client_secret")
		assert.NotContains(t, request.form, "client_assertion")
		assert.Empty(t, request.header.Get("Authorization"))
	}
	assert.NotEmpty(t, requests[0].form.Get("code_verifier"))
	assert.Equal(t, "authorization_code", requests[0].form.Get("grant_type"))
	assert.Equal(t, "refresh_token", requests[1].form.Get("grant_type"))
	assert.Equal(t, "refresh_token", requests[2].form.Get("grant_type"))
	ids, pairs := h.reader.counts()
	assert.Zero(t, ids)
	assert.Zero(t, pairs)
	after, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCredentialExclusions_CIMDUsesRealFreshVerifiedAssertionsAndNeverReadsBindings(t *testing.T) {
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "absent-id"), ClientSecretFile: filepath.Join(t.TempDir(), "absent-secret")}
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"cimd-service": binding})
	ctx := context.Background()
	keys := memory.NewSigningKeyStore()
	keyService := cimdclient.NewKeyService(keys, keys, newTestEncryption(t), newNoopBranchKeyManager(), slog.New(slog.NewTextHandler(io.Discard, nil)))
	_, err := keyService.GenerateKey(ctx, "ES256")
	require.NoError(t, err)
	h.providers.WithCIMDPublicURL("https://broker.example.com").WithCIMDKeyReadiness(keyService)
	h.service.WithCIMDAssertionSigner(cimdclient.NewAssertionSigner(keyService))
	provider := createCIMDTestProvider(id.NewServiceID(), h.endpoint.server.URL)
	canonicalID := "cimd-service"
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = model.CredentialSourceStored
	require.NoError(t, h.providers.Create(ctx, provider))
	before, err := h.records.Get(ctx, provider.ID)
	require.NoError(t, err)
	principal := id.Principal("cimd@example.com")
	flow := h.initiate(t, principal, provider.ID)
	authorization, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, provider.ClientID.String(), authorization.Query().Get("client_id"))
	h.callback(t, principal, provider.ID, flow.StateToken)
	_, err = h.service.ForceRefreshSession(ctx, principal, provider.ID)
	require.NoError(t, err)
	h.expire(t, principal, provider.ID)
	_, token, err := h.service.GetValidAccessToken(ctx, principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "credential-access", token)
	publicKeys, err := keyService.PublicJWKSet(ctx)
	require.NoError(t, err)
	require.Equal(t, 1, publicKeys.Len())
	publicKey, ok := publicKeys.Key(0)
	require.True(t, ok)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 3)
	assertionIDs := make(map[string]bool)
	for _, request := range requests {
		assert.Equal(t, provider.ClientID.String(), request.form.Get("client_id"))
		assert.Equal(t, "urn:ietf:params:oauth:client-assertion-type:jwt-bearer", request.form.Get("client_assertion_type"))
		assert.NotContains(t, request.form, "client_secret")
		assert.Empty(t, request.header.Get("Authorization"))
		claims, err := jwt.Parse([]byte(request.form.Get("client_assertion")), jwt.WithKey(jwa.ES256(), publicKey), jwt.WithIssuer(provider.ClientID.String()), jwt.WithSubject(provider.ClientID.String()), jwt.WithAudience(h.endpoint.server.URL))
		require.NoError(t, err, "provider authentication must contain a valid production-generated CIMD assertion")
		assertionID, ok := claims.JwtID()
		require.True(t, ok)
		require.NotEmpty(t, assertionID)
		assert.False(t, assertionIDs[assertionID], "each token request must sign a fresh assertion")
		assertionIDs[assertionID] = true
	}
	assert.NotEmpty(t, requests[0].form.Get("code_verifier"))
	ids, pairs := h.reader.counts()
	assert.Zero(t, ids)
	assert.Zero(t, pairs)
	after, err := h.records.Get(ctx, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCredentialExclusions_GoogleFlavorUsesStoredCredentialDocumentWithoutFileAccess(t *testing.T) {
	binding := ports.CredentialFileBinding{ClientIDFile: filepath.Join(t.TempDir(), "absent-id"), ClientSecretFile: filepath.Join(t.TempDir(), "absent-secret")}
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"google-service": binding})
	credential, err := json.Marshal(map[string]string{"type": "service_account", "project_id": "synthetic-project", "private_key_id": "synthetic-key", "private_key": "synthetic-structural-key", "client_email": "account@synthetic-project.iam.gserviceaccount.com", "token_uri": h.endpoint.server.URL, "client_id": "112233445566778899001"})
	require.NoError(t, err)
	provider := createTestService(id.NewServiceID())
	canonicalID := "google-service"
	provider.CanonicalID = &canonicalID
	provider.CredentialSource = model.CredentialSourceStored
	provider.Flavor = model.OAuth2FlavorGoogle
	provider.ClientID = ""
	provider.Secret = model.NewPlaintextSecret(string(credential))
	provider.IssuerURI = ""
	provider.Endpoints = model.OAuth2Endpoints{}
	require.NoError(t, h.providers.Create(context.Background(), provider))
	before, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Equal(t, model.OAuth2FlavorGoogle, before.Flavor)
	assert.True(t, before.Secret.IsEncrypted())
	assert.Equal(t, "112233445566778899001", before.ClientID.String())
	assert.Equal(t, h.endpoint.server.URL, before.Endpoints.TokenEndpoint)
	principal := id.Principal("google@example.com")
	flow := h.initiate(t, principal, provider.ID)
	authorization, err := url.Parse(flow.AuthorizationURL)
	require.NoError(t, err)
	assert.Equal(t, "accounts.google.com", authorization.Host)
	assert.Equal(t, "112233445566778899001", authorization.Query().Get("client_id"))
	h.callback(t, principal, provider.ID, flow.StateToken)
	_, err = h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	h.expire(t, principal, provider.ID)
	_, token, err := h.service.GetValidAccessToken(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	assert.Equal(t, "credential-access", token)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 3)
	for _, request := range requests {
		assert.Equal(t, "112233445566778899001", request.clientID)
		assert.JSONEq(t, string(credential), request.secret)
		assert.NotContains(t, request.form, "client_assertion")
	}
	ids, pairs := h.reader.counts()
	assert.Zero(t, ids)
	assert.Zero(t, pairs)
	after, err := h.records.Get(context.Background(), provider.ID)
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestCredentialExclusions_StoredSourceIgnoresAvailableDifferentClientPair(t *testing.T) {
	binding := credentialPair(t, "unrelated-file-client", "unrelated-file-secret")
	h := newCredentialSessionHarness(t, map[string]ports.CredentialFileBinding{"bound-stored-service": binding})
	provider := h.register(t, model.CredentialSourceStored, "bound-stored-service", "registered-client", "registered-secret")
	principal := id.Principal("bound-stored@example.com")
	flow := h.initiate(t, principal, provider.ID)
	h.callback(t, principal, provider.ID, flow.StateToken)
	_, err := h.service.ForceRefreshSession(context.Background(), principal, provider.ID)
	require.NoError(t, err)
	requests := h.endpoint.captured(t)
	require.Len(t, requests, 2)
	for _, request := range requests {
		assert.Equal(t, "registered-client", request.clientID)
		assert.Equal(t, "registered-secret", request.secret)
	}
	ids, pairs := h.reader.counts()
	assert.Zero(t, ids)
	assert.Zero(t, pairs)
}
