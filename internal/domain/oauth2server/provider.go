package oauth2server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"strings"
	"time"

	"github.com/ory/fosite"
	fositeOAuth2 "github.com/ory/fosite/handler/oauth2"
	"github.com/ory/fosite/handler/pkce"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/ledger"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2/oidcscope"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/urivalidation"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// Provider is the central wiring for the OAuth2 server mode.
// It constructs fosite handlers with custom strategies and storage adapters,
// and exposes domain-native methods for use by HTTP handlers.
type Provider struct {
	authCodeHandler *fositeOAuth2.AuthorizeExplicitGrantHandler
	ccHandler       *fositeOAuth2.ClientCredentialsGrantHandler
	refreshHandler  *fositeOAuth2.RefreshTokenGrantHandler
	pkceHandler     *pkce.Handler

	fositeStorage   *FositeStorage
	clientAuth      *ClientAuthService
	accessStrategy  *JWXAccessTokenStrategy
	refreshStrategy *RandomRefreshTokenStrategy

	config *fosite.Config
	logger *slog.Logger
	ledger *ledger.Service
}

// NewProvider constructs the OAuth2 server provider with fosite handlers.
func NewProvider(
	codeRepo ports.AuthorizationCodeRepository,
	refreshRepo ports.RefreshTokenSessionRepository,
	pkceRepo ports.PKCESessionRepository,
	credRepo ports.ClientCredentialRepository,
	clientResolver ports.ClientResolver,
	signingKeyService *SigningKeyService,
	issuerURI string,
	tokenTTL time.Duration,
	refreshTokenTTL time.Duration,
	tokenClaimsExpression string,
	logger *slog.Logger,
	recorder *ledger.Service,
	transactions ports.StorageTransactionManager,
) (*Provider, error) {
	if recorder == nil || transactions == nil {
		return nil, fmt.Errorf("token recorder and storage transactions are required")
	}
	// Compile token claims CEL expression at startup (FR-013b: fail if invalid)
	customClaimsEval, err := NewTokenClaimsEvaluator(tokenClaimsExpression)
	if err != nil {
		return nil, fmt.Errorf("invalid token_claims_expression: %w", err)
	}

	// Build services
	clientAuth := NewClientAuthService(credRepo, clientResolver, logger)

	// Our strategies
	accessStrategy, err := NewJWXAccessTokenStrategy(signingKeyService, issuerURI, tokenTTL, customClaimsEval, logger)
	if err != nil {
		return nil, fmt.Errorf("invalid access token strategy configuration: %w", err)
	}
	codeStrategy := &RandomCodeStrategy{}
	refreshStrategy := &RandomRefreshTokenStrategy{}

	// Storage adapters
	storage := NewFositeStorage(codeRepo, refreshRepo, pkceRepo, credRepo, clientResolver, logger, transactions)

	config := &fosite.Config{
		AuthorizeCodeLifespan:          60 * time.Second,
		AccessTokenLifespan:            tokenTTL,
		RefreshTokenLifespan:           refreshTokenTTL,
		RefreshTokenScopes:             oidcscope.ReservedRefreshTokenScopes,
		EnforcePKCE:                    true,
		EnablePKCEPlainChallengeMethod: false,
		// Empty AllowedScopes means unrestricted in our domain model.
		// fosite's default WildcardScopeStrategy treats empty as no scopes allowed.
		ScopeStrategy: oauth2.IsScopeAllowed,
	}

	helper := &fositeOAuth2.HandleHelper{
		AccessTokenStrategy: accessStrategy,
		AccessTokenStorage:  storage,
		Config:              config,
	}

	return &Provider{
		authCodeHandler: &fositeOAuth2.AuthorizeExplicitGrantHandler{
			AccessTokenStrategy:    accessStrategy,
			AuthorizeCodeStrategy:  codeStrategy,
			RefreshTokenStrategy:   refreshStrategy,
			CoreStorage:            storage,
			TokenRevocationStorage: storage,
			Config:                 config,
		},
		ccHandler: &fositeOAuth2.ClientCredentialsGrantHandler{
			HandleHelper: helper,
			Config:       config,
		},
		refreshHandler: &fositeOAuth2.RefreshTokenGrantHandler{
			AccessTokenStrategy:    accessStrategy,
			RefreshTokenStrategy:   refreshStrategy,
			TokenRevocationStorage: storage,
			Config:                 config,
		},
		pkceHandler: &pkce.Handler{
			AuthorizeCodeStrategy: codeStrategy,
			Storage:               storage,
			Config:                config,
		},
		fositeStorage:   storage,
		clientAuth:      clientAuth,
		accessStrategy:  accessStrategy,
		refreshStrategy: refreshStrategy,
		config:          config,
		logger:          logger,
		ledger:          recorder,
	}, nil
}

// ClientAuth returns the client authentication service.
func (p *Provider) ClientAuth() *ClientAuthService {
	return p.clientAuth
}

// IssueImpersonationToken implements ports.ImpersonationTokenIssuer by minting a locally signed
// impersonated broker token via the access-token strategy.
func (p *Provider) IssueImpersonationToken(ctx context.Context, input ports.ImpersonationMintInput) (string, error) {
	return p.accessStrategy.GenerateImpersonationToken(ctx, input)
}

// HandleClientCredentials processes a client_credentials grant type request.
// Scope validation and token generation are fully delegated to fosite's ccHandler.
func (p *Provider) HandleClientCredentials(ctx context.Context, clientID string, secret string, requestedScope string) (resp *ports.TokenResponse, err error) {
	facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: "agent"}}
	defer p.finishTokenRequest(ctx, &facts, &err)

	fositeClient, err := p.fositeStorage.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, fosite.ErrNotFound) {
			return nil, fosite.ErrInvalidClient.WithHintf("client %s not found", clientID)
		}
		return nil, fosite.ErrServerError.WithDebugf("client lookup failed: %v", err)
	}

	cc, ok := fositeClient.(*confidentialClient)
	if !ok {
		return nil, fosite.ErrInvalidClient.WithHintf("client_credentials grant requires a confidential client")
	}
	facts.AgentID = cc.agent.ID

	authClient, err := p.clientAuth.Authenticate(ctx, cc.agent.ID, secret)
	if err != nil {
		return nil, err
	}
	actorID := authClient.Agent.ID.String()
	facts.Actor.ID = &actorID

	client := &confidentialClient{clientID: clientID, agent: authClient.Agent, credential: authClient.Credential}
	scopes := fosite.Arguments(oauth2.SplitScope(requestedScope))

	session := &fosite.DefaultSession{
		Subject: authClient.Agent.ID.String(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AccessToken: time.Now().Add(p.config.AccessTokenLifespan),
		},
	}

	req := fosite.NewAccessRequest(session)
	req.Client = client
	req.GrantTypes = fosite.Arguments{"client_credentials"}
	req.RequestedScope = scopes

	if err := p.ccHandler.HandleTokenEndpointRequest(ctx, req); err != nil {
		return nil, err
	}

	// fosite v0.49 HandleTokenEndpointRequest validates scopes but does not grant them.
	for _, scope := range scopes {
		req.GrantScope(scope)
	}

	return p.issueToken(ctx, req, facts, func(txCtx context.Context, response fosite.AccessResponder) error {
		return p.ccHandler.PopulateTokenEndpointResponse(txCtx, req, response)
	})
}

// HandleAuthorize processes an authorization endpoint request using the agent's UUID
// as the client identifier. Resolves the agent's broker credentials internally.
//
// PKCE enforcement (EnforcePKCE:true, S256-only) is handled by pkceHandler.
// Scope validation is handled by the configured ScopeStrategy.
// Returns ErrInvalidRedirectURI when the redirect_uri has not been validated so HTTP
// handlers can send a direct JSON response per RFC 6749 §4.1.2.1.
func (p *Provider) HandleAuthorize(
	ctx context.Context,
	clientID string,
	redirectURI string,
	responseType string,
	scope string,
	state string,
	codeChallenge string,
	codeChallengeMethod string,
	principal id.Principal,
) (code string, err error) {
	defer func() { err = translateFositeError(err) }()

	fositeClient, err := p.fositeStorage.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, fosite.ErrNotFound) {
			return "", fosite.ErrInvalidClient.WithHintf("client %s not found", clientID)
		}
		return "", fosite.ErrServerError.WithDebugf("client lookup failed: %v", err)
	}

	h, ok := fositeClient.(agentHolder)
	if !ok {
		return "", fosite.ErrServerError.WithDebugf("internal: expected broker client, got %T", fositeClient)
	}
	agent := h.getAgent()

	// Validate redirect_uri: must be registered and use HTTPS (or loopback HTTP).
	// GetRedirectURIs() returns the agent's registered URIs for confidential clients,
	// and the CIMD document's redirect_uris for public (CIMD) clients.
	if !containsRedirectURI(fositeClient.GetRedirectURIs(), redirectURI) {
		return "", fmt.Errorf("%w: %s", ErrInvalidRedirectURI,
			fosite.ErrInvalidRequest.WithHintf("redirect_uri %q is not registered for this client", redirectURI))
	}
	if !urivalidation.IsValidRedirectURI(redirectURI) {
		return "", fmt.Errorf("%w: %s", ErrInvalidRedirectURI,
			fosite.ErrInvalidRequest.WithHintf("redirect_uri must use HTTPS for non-loopback hosts"))
	}

	if responseType != "code" {
		return "", fosite.ErrUnsupportedResponseType.WithHintf("only 'code' response_type is supported")
	}

	scopes := fosite.Arguments(oauth2.SplitScope(scope))
	for _, requestedScope := range scopes {
		if !oauth2.IsScopeAllowed(agent.AllowedScopes, requestedScope) {
			return "", fosite.ErrInvalidScope.WithHintf("scope %q is not allowed for this client", requestedScope)
		}
	}

	session := &fosite.DefaultSession{
		Subject: principal.String(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AuthorizeCode: time.Now().Add(60 * time.Second),
		},
	}

	parsedRedirectURI, _ := url.Parse(redirectURI)

	authReq := fosite.NewAuthorizeRequest()
	authReq.Client = fositeClient
	authReq.RedirectURI = parsedRedirectURI
	authReq.ResponseTypes = fosite.Arguments{responseType}
	authReq.RequestedScope = scopes
	authReq.GrantedScope = scopes
	authReq.State = state
	authReq.Session = session
	authReq.Form = url.Values{
		"redirect_uri":          {redirectURI},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {codeChallengeMethod},
		"response_type":         {responseType},
		"client_id":             {clientID},
		"scope":                 {scope},
		"state":                 {state},
	}

	resp := fosite.NewAuthorizeResponse()
	if err := p.authCodeHandler.HandleAuthorizeEndpointRequest(ctx, authReq, resp); err != nil {
		return "", err
	}
	if err := p.pkceHandler.HandleAuthorizeEndpointRequest(ctx, authReq, resp); err != nil {
		return "", err
	}

	return resp.GetCode(), nil
}

func (p *Provider) HandleAuthorizationCodeExchange(
	ctx context.Context,
	clientID string,
	secret string,
	code string,
	redirectURI string,
	codeVerifier string,
) (tokenResp *ports.TokenResponse, err error) {
	facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: "agent"}}
	defer p.finishTokenRequest(ctx, &facts, &err)

	fositeClient, err := p.fositeStorage.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, fosite.ErrNotFound) {
			return nil, fosite.ErrInvalidClient.WithHintf("client %s not found", clientID)
		}
		return nil, fosite.ErrServerError.WithDebugf("client lookup failed: %v", err)
	}

	// Domain-level credential pre-check: peek at stored code to enforce binding.
	authCode, err := p.fositeStorage.codeRepo.FindByCodeHash(ctx, p.authCodeHandler.AuthorizeCodeStrategy.AuthorizeCodeSignature(ctx, code))
	if err != nil {
		if isStorageNotFound(err) {
			return nil, fosite.ErrInvalidGrant.WithHintf("authorization code not found")
		}
		return nil, fosite.ErrServerError.WithDebugf("failed to look up authorization code: %v", err)
	}

	switch bc := fositeClient.(type) {
	case *confidentialClient:
		facts.AgentID = bc.agent.ID
		authedClient, err := p.clientAuth.Authenticate(ctx, bc.agent.ID, secret)
		if err != nil {
			return nil, err
		}
		actorID := authedClient.Agent.ID.String()
		facts.Actor.ID = &actorID
		if authedClient.Credential.AgentID != authCode.AgentID {
			return nil, fosite.ErrInvalidGrant.WithHintf("authorization code was issued to a different client credential")
		}
		// Rebuild client with authenticated credential
		fositeClient = &confidentialClient{clientID: clientID, agent: authedClient.Agent, credential: authedClient.Credential}
	case *publicClient:
		facts.AgentID = bc.agent.ID
		if bc.agent.ID != authCode.AgentID {
			return nil, fosite.ErrInvalidGrant.WithHintf("authorization code was issued to a different client")
		}
	default:
		return nil, fosite.ErrServerError.WithDebugf("unexpected client type %T", fositeClient)
	}

	session := &fosite.DefaultSession{
		Subject: authCode.Principal.String(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AccessToken: time.Now().Add(p.config.AccessTokenLifespan),
		},
	}
	setSessionProfile(session, authCode.Email, authCode.DisplayName)
	req := fosite.NewAccessRequest(session)
	req.Client = fositeClient
	req.GrantTypes = fosite.Arguments{"authorization_code"}
	req.Form = url.Values{
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"code_verifier": {codeVerifier},
		"grant_type":    {"authorization_code"},
	}

	// redirect_uri binding: fosite's AuthorizeExplicitGrantHandler.HandleTokenEndpointRequest
	// compares this redirect_uri against the one stored in the authorization code session,
	// returning invalid_grant on mismatch (RFC 6749 §4.1.3).
	if err := p.authCodeHandler.HandleTokenEndpointRequest(ctx, req); err != nil {
		// Fosite wraps storage's invalid_grant in server_error.
		if errors.Is(err, fosite.ErrInvalidGrant) {
			return nil, fosite.ErrInvalidGrant.WithWrap(err)
		}
		return nil, err
	}
	hints := ports.StorageTransactionHintsFromContext(ctx)
	hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: authCode.Principal})
	var validationErr error
	var response *ports.TokenResponse
	err = p.ledger.WithTransaction(ctx, hints, func(txCtx context.Context) error {
		validationErr = p.pkceHandler.HandleTokenEndpointRequest(txCtx, req)
		if validationErr != nil {
			// Fosite consumes the one-shot challenge even when validation fails.
			return nil
		}
		facts.Subject = &authCode.Principal
		actorID := authCode.AgentID.String()
		facts.Actor.ID = &actorID
		var issueErr error
		response, issueErr = p.issueToken(txCtx, req, facts, func(populateCtx context.Context, fositeResp fosite.AccessResponder) error {
			if err := p.authCodeHandler.PopulateTokenEndpointResponse(populateCtx, req, fositeResp); err != nil {
				if errors.Is(err, fosite.ErrInvalidGrant) {
					return fosite.ErrInvalidGrant.WithWrap(err)
				}
				return err
			}
			return p.pkceHandler.PopulateTokenEndpointResponse(populateCtx, req, fositeResp)
		})
		return issueErr
	})
	if err != nil {
		return nil, err
	}
	if validationErr != nil {
		return nil, validationErr
	}
	return response, nil
}

// HandleRefreshToken processes a refresh_token grant for locally-minted tokens.
func (p *Provider) HandleRefreshToken(
	ctx context.Context,
	clientID string,
	secret string,
	refreshToken string,
	scope string,
) (tokenResp *ports.TokenResponse, err error) {
	facts := model.BusinessEvent{Actor: model.BusinessEventActor{Kind: "agent"}}
	defer p.finishTokenRequest(ctx, &facts, &err)

	fositeClient, err := p.fositeStorage.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, fosite.ErrNotFound) {
			return nil, fosite.ErrInvalidClient.WithHintf("client %s not found", clientID)
		}
		return nil, fosite.ErrServerError.WithDebugf("client lookup failed: %v", err)
	}

	switch bc := fositeClient.(type) {
	case *confidentialClient:
		facts.AgentID = bc.agent.ID
		authedClient, err := p.clientAuth.Authenticate(ctx, bc.agent.ID, secret)
		if err != nil {
			return nil, err
		}
		actorID := authedClient.Agent.ID.String()
		facts.Actor.ID = &actorID
		fositeClient = &confidentialClient{clientID: clientID, agent: authedClient.Agent, credential: authedClient.Credential}
	case *publicClient:
		facts.AgentID = bc.agent.ID
		// Public clients authenticate by client_id only.
	default:
		return nil, fosite.ErrServerError.WithDebugf("unexpected client type %T", fositeClient)
	}

	requestScope := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	}
	if scope != "" {
		requestScope.Set("scope", scope)
	}

	req := fosite.NewAccessRequest(&fosite.DefaultSession{})
	req.Client = fositeClient
	req.GrantTypes = fosite.Arguments{"refresh_token"}
	req.Form = requestScope

	if err := p.refreshHandler.HandleTokenEndpointRequest(ctx, req); err != nil {
		return nil, err
	}

	subject := id.Principal(req.GetSession().GetSubject())
	if !subject.IsZero() {
		facts.Subject = &subject
	}
	actorID := facts.AgentID.String()
	facts.Actor.ID = &actorID
	return p.issueToken(ctx, req, facts, func(txCtx context.Context, response fosite.AccessResponder) error {
		return p.refreshHandler.PopulateTokenEndpointResponse(txCtx, req, response)
	})
}

func (p *Provider) issueToken(ctx context.Context, req fosite.AccessRequester, facts model.BusinessEvent, populate func(context.Context, fosite.AccessResponder) error) (*ports.TokenResponse, error) {
	response := fosite.NewAccessResponse()
	hints := ports.StorageTransactionHintsFromContext(ctx)
	if facts.Subject != nil {
		hints.Subjects = append(hints.Subjects, ports.StorageSubjectGate{Principal: *facts.Subject})
	}
	err := p.ledger.WithTransaction(ctx, hints, func(txCtx context.Context) error {
		if err := populate(txCtx, response); err != nil {
			return err
		}
		return p.recordTokenOutcome(txCtx, "token-issued", facts)
	})
	if err != nil {
		return nil, err
	}
	result := &ports.TokenResponse{
		AccessToken: response.GetAccessToken(), TokenType: "Bearer",
		ExpiresIn: int64(p.config.AccessTokenLifespan.Seconds()),
		Scope:     strings.Join(req.GetGrantedScopes(), " "),
	}
	if refreshToken, ok := response.GetExtra("refresh_token").(string); ok {
		result.RefreshToken = refreshToken
	}
	return result, nil
}

func (p *Provider) recordTokenOutcome(ctx context.Context, eventType string, facts model.BusinessEvent) error {
	facts.OccurredAt = time.Now().UTC()
	if facts.Data == nil {
		facts.Data = map[string]any{}
	}
	event, err := p.ledger.NewEvent(ctx, model.BusinessEventTypePrefix+eventType, facts)
	if err != nil {
		return err
	}
	return p.ledger.Record(ctx, event)
}

func (p *Provider) finishTokenRequest(ctx context.Context, facts *model.BusinessEvent, requestErr *error) {
	*requestErr = translateFositeError(*requestErr)
	if *requestErr == nil {
		return
	}
	reason := "internal_failure"
	switch {
	case errors.Is(*requestErr, ErrInvalidClient):
		reason = "authentication_failed"
	case errors.Is(*requestErr, ErrInvalidGrant), errors.Is(*requestErr, ErrInvalidScope):
		reason = "authorization_failed"
	case errors.Is(*requestErr, ErrInvalidRequest):
		reason = "invalid_request"
	}
	facts.Data = map[string]any{"reason_code": reason}
	if err := p.recordTokenOutcome(ctx, "token-request-failed", *facts); err != nil {
		*requestErr = translateFositeError(fosite.ErrServerError.WithDebug("business event recording failed"))
	}
}

func containsRedirectURI(list []string, item string) bool {
	for _, v := range list {
		if urivalidation.MatchesRedirectURI(v, item) {
			return true
		}
	}
	return false
}

// isStorageNotFound returns true when err is a domain storage "not found" error.
func isStorageNotFound(err error) bool {
	var se *storage.StorageError
	return errors.As(err, &se) && se.Kind == storage.ErrorKindNotFound
}
