package app

import (
	"context"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2server"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type localMintingStrategy struct {
	provider *oauth2server.Provider
}

func newLocalMintingStrategy(provider *oauth2server.Provider) *localMintingStrategy {
	return &localMintingStrategy{provider: provider}
}

func (s *localMintingStrategy) HandleClientCredentials(ctx context.Context, clientID id.ClientID, clientSecret, scope string) (*ports.TokenResponse, error) {
	return s.provider.HandleClientCredentials(ctx, string(clientID), clientSecret, scope)
}

func (s *localMintingStrategy) HandleAuthorizationCodeExchange(ctx context.Context, clientID id.ClientID, creds ports.ClientCredentials, code, redirectURI, codeVerifier string) (*ports.TokenResponse, error) {
	return s.provider.HandleAuthorizationCodeExchange(ctx, string(clientID), creds, code, redirectURI, codeVerifier)
}

func (s *localMintingStrategy) HandleRefreshToken(ctx context.Context, clientID id.ClientID, creds ports.ClientCredentials, refreshToken, scope string) (*ports.TokenResponse, error) {
	return s.provider.HandleRefreshToken(ctx, string(clientID), creds, refreshToken, scope)
}

type localCodeIssuer struct {
	provider *oauth2server.Provider
}

func newLocalCodeIssuer(provider *oauth2server.Provider) *localCodeIssuer {
	return &localCodeIssuer{provider: provider}
}

func (s *localCodeIssuer) IssueAuthorizationCode(ctx context.Context, req *ports.AuthorizationRequest, principal id.Principal) (string, error) {
	return s.provider.HandleAuthorize(
		ctx,
		string(req.ClientID),
		req.RedirectURI,
		req.ResponseType,
		req.Scope,
		req.State,
		req.CodeChallenge,
		req.CodeChallengeMethod,
		principal,
	)
}
