package oauth2server

import (
	"context"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	dstorage "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFositeStorage_GetClientPreservesCIMDAuthenticationMetadata(t *testing.T) {
	const clientID = "https://client.example/metadata.json"
	agent := &dstorage.Agent{ID: id.NewAgentID(), DisplayName: "CIMD Agent"}
	metadata := &ports.CIMDMetadataDTO{
		ClientID:     clientID,
		AuthMethod:   "private_key_jwt",
		SigningAlg:   "RS256",
		JWKSURI:      "https://client.example/jwks.json",
		RedirectURIs: []string{"https://client.example/callback"},
		GrantTypes:   []string{"authorization_code", "refresh_token"},
	}
	resolver := &mockClientResolver{resolveFunc: func(context.Context, id.ClientID) (*ports.ClientResolution, error) {
		return ports.NewClientResolution(agent, metadata), nil
	}}
	store := NewFositeStorage(
		memory.NewAuthorizationCodeStore(),
		memory.NewRefreshTokenSessionStore(),
		memory.NewPKCESessionStore(),
		memory.NewClientCredentialStore(),
		resolver,
		newTestGrantVerifier(memory.NewAgentRepository()),
		testSlogger(),
	)

	client, err := store.GetClient(context.Background(), clientID)
	require.NoError(t, err)
	public, ok := client.(*publicClient)
	require.True(t, ok)
	require.NotNil(t, public.metadata)
	assert.Equal(t, "private_key_jwt", public.metadata.AuthMethod)
	assert.Equal(t, "RS256", public.metadata.SigningAlg)
	assert.Equal(t, metadata.RedirectURIs, public.metadata.RedirectURIs)
}
