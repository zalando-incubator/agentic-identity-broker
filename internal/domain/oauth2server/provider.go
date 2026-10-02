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
	"go.opentelemetry.io/otel/metric"

	domainencryption "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/encryption"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
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
	refreshDeps     RefreshSessionDependencies
	refreshStrategy *RandomRefreshTokenStrategy

	config          *fosite.Config
	logger          *slog.Logger
	retryAccepted   metric.Int64Counter
	refreshRejected metric.Int64Counter
}

// NewProvider constructs the OAuth2 server provider with fosite handlers.
func NewProvider(
	codeRepo ports.AuthorizationCodeRepository,
	refreshDeps RefreshSessionDependencies,
	pkceRepo ports.PKCESessionRepository,
	credRepo ports.ClientCredentialRepository,
	clientResolver ports.ClientResolver,
	signingKeyService *SigningKeyService,
	issuerURI string,
	tokenTTL time.Duration,
	tokenClaimsExpression string,
	logger *slog.Logger,
	transactions ports.StorageTransactionManager,
) (*Provider, error) {
	if codeRepo == nil || pkceRepo == nil || credRepo == nil || clientResolver == nil || signingKeyService == nil || logger == nil {
		return nil, fmt.Errorf("authorization code, PKCE, client credentials, resolver, signing key service and logger are required")
	}
	if refreshDeps.Agents == nil || refreshDeps.Sessions == nil || refreshDeps.Tokens == nil || refreshDeps.Revocations == nil ||
		refreshDeps.Coordinator == nil || refreshDeps.Clock == nil || refreshDeps.Verifier == nil ||
		refreshDeps.Encryption == nil || refreshDeps.BranchKeys == nil || transactions == nil {
		return nil, fmt.Errorf("native refresh session agent and repositories, coordinator, clock, verifier, encryption, branch keys and transactions are required")
	}
	if refreshDeps.Policy.InactivityLifetime <= 0 || refreshDeps.Policy.AbsoluteLifetime < 0 ||
		refreshDeps.Policy.ReuseInterval < 0 || refreshDeps.Policy.ReuseInterval >= refreshDeps.Policy.InactivityLifetime ||
		refreshDeps.Policy.ReuseInterval >= tokenTTL ||
		(refreshDeps.Policy.AbsoluteLifetime > 0 && refreshDeps.Policy.ReuseInterval >= refreshDeps.Policy.AbsoluteLifetime) {
		return nil, fmt.Errorf("invalid refresh session policy")
	}

	// Compile token claims CEL expression at startup (FR-013b: fail if invalid)
	customClaimsEval, err := NewTokenClaimsEvaluator(tokenClaimsExpression)
	if err != nil {
		return nil, fmt.Errorf("invalid token_claims_expression: %w", err)
	}

	retryAccepted, refreshRejected, err := newRefreshMetrics()
	if err != nil {
		return nil, fmt.Errorf("initialize refresh telemetry: %w", err)
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
	storage := NewFositeStorage(codeRepo, refreshDeps, pkceRepo, credRepo, clientResolver, logger, transactions)

	config := &fosite.Config{
		AuthorizeCodeLifespan:          60 * time.Second,
		AccessTokenLifespan:            tokenTTL,
		RefreshTokenLifespan:           refreshDeps.Policy.InactivityLifetime,
		RefreshTokenScopes:             oidcscope.ReservedRefreshTokenScopes,
		EnforcePKCE:                    true,
		EnablePKCEPlainChallengeMethod: false,
		// Empty AllowedScopes means unrestricted in our domain model.
		// fosite's default WildcardScopeStrategy treats empty as no scopes allowed.
		ScopeStrategy: oauth2.IsScopeAllowed,
		// Fosite otherwise removes PKCE evidence before authorization-code persistence.
		SanitationWhiteList: []string{"code", "redirect_uri", "client_id", "code_challenge", "code_challenge_method"},
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
		refreshDeps:     refreshDeps,
		clientAuth:      clientAuth,
		accessStrategy:  accessStrategy,
		refreshStrategy: refreshStrategy,
		config:          config,
		logger:          logger,
		retryAccepted:   retryAccepted,
		refreshRejected: refreshRejected,
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
	defer func() { err = translateFositeError(err) }()

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

	authClient, err := p.clientAuth.Authenticate(ctx, cc.agent.ID, secret)
	if err != nil {
		return nil, err
	}

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

	fositeResp := fosite.NewAccessResponse()
	if err := p.ccHandler.PopulateTokenEndpointResponse(ctx, req, fositeResp); err != nil {
		return nil, err
	}

	return &ports.TokenResponse{
		AccessToken: fositeResp.GetAccessToken(),
		TokenType:   "Bearer",
		ExpiresIn:   int64(p.config.AccessTokenLifespan.Seconds()),
		Scope:       strings.Join(req.GetGrantedScopes(), " "),
	}, nil
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
	defer func() { err = translateFositeError(err) }()

	preparedClient, err := p.authenticateProviderClient(ctx, clientID, secret)
	if err != nil {
		return nil, err
	}

	codeSignature := p.authCodeHandler.AuthorizeCodeStrategy.AuthorizeCodeSignature(ctx, code)
	authCode, err := p.fositeStorage.codeRepo.FindByCodeHash(ctx, codeSignature)
	if err != nil {
		if isStorageNotFound(err) {
			return nil, fosite.ErrInvalidGrant.WithHint("authorization code not found")
		}
		return nil, fosite.ErrServerError.WithDebugf("failed to look up authorization code: %v", err)
	}
	clientAgentID, err := extractAgentID(preparedClient)
	if err != nil {
		return nil, fosite.ErrServerError.WithWrap(err)
	}
	if authCode.UsedAt == nil && clientAgentID != authCode.AgentID {
		return nil, fosite.ErrInvalidGrant.WithHint("authorization code was issued to a different client")
	}
	rootID, err := id.ParseRefreshSessionID(authCode.ID.String())
	if err != nil {
		return nil, fosite.ErrServerError.WithWrap(err)
	}
	if authCode.UsedAt == nil && preparedClient.GetGrantTypes().Has("refresh_token") &&
		fosite.Arguments(oauth2.SplitScope(authCode.Scope)).HasOneOf(oidcscope.ReservedRefreshTokenScopes...) {
		keyID, err := p.refreshDeps.BranchKeys.Create(ctx, domainencryption.NewRefreshSessionBranchKeySubject(rootID))
		if err != nil || keyID != "refresh_"+rootID.String()+"_branch_key" {
			return nil, fosite.ErrServerError.WithDebugf("failed to provision refresh session branch key: %v", err)
		}
	}

	if authCode.UsedAt == nil {
		signer, err := p.accessStrategy.signingKeyService.signingMaterial(ctx)
		if err != nil {
			return nil, fosite.ErrServerError.WithDebugf("failed to prepare signing material: %v", err)
		}
		ctx = withPreparedSigningMaterial(ctx, signer)
	}
	// Material, client metadata and KMS provisioning are prepared before the agent gate.
	// Only the shared owner callback can consume the code or publish refresh lineage.
	input := &codeExchangeInput{
		clientID: clientID, secret: secret, code: code, redirectURI: redirectURI,
		verifier: codeVerifier, signature: codeSignature, client: preparedClient,
		codeRecord: authCode, clientAgentID: clientAgentID, rootID: rootID,
	}
	var staged *ports.TokenResponse
	var replayError, callbackError error
	err = p.refreshDeps.Coordinator.Run(ctx, authCode.AgentID, func(owner context.Context, at time.Time) error {
		staged, replayError, callbackError = p.exchangeCodeInOwner(owner, at, input)
		return callbackError
	})
	if err != nil {
		if callbackError == nil {
			return nil, fosite.ErrServerError.WithDebugf("authorization session commit failed: %v", err)
		}
		return nil, err
	}
	if replayError != nil {
		if !input.replayRoot.ID.IsZero() {
			logRefreshTransition(p.logger, ctx, input.replayRoot, storage.RefreshReasonCodeReplay)
		}
		return nil, replayError
	}
	return staged, nil
}

type codeExchangeInput struct {
	clientID, secret, code, redirectURI, verifier, signature string
	client                                                   fosite.Client
	codeRecord                                               *storage.AuthorizationCode
	clientAgentID                                            id.AgentID
	rootID                                                   id.RefreshSessionID
	replayRoot                                               storage.RefreshSessionAuditIdentity
}

func (p *Provider) codeClientInOwner(owner context.Context, at time.Time, input *codeExchangeInput) (*storage.AuthorizationCode, fosite.Client, id.GrantID, error) {
	current, err := p.fositeStorage.codeRepo.FindByCodeHash(owner, input.signature)
	if err != nil {
		if isStorageNotFound(err) {
			return nil, nil, id.GrantID{}, fosite.ErrInvalidGrant.WithHint("authorization code not found")
		}
		return nil, nil, id.GrantID{}, fosite.ErrServerError.WithWrap(err)
	}
	if current.ID != input.codeRecord.ID || current.AgentID != input.codeRecord.AgentID {
		return nil, nil, id.GrantID{}, fosite.ErrInvalidGrant.WithHint("authorization code ownership changed")
	}
	if current.UsedAt != nil {
		return current, input.client, id.GrantID{}, nil
	}
	if input.clientAgentID != current.AgentID {
		return nil, nil, id.GrantID{}, fosite.ErrInvalidGrant.WithHint("authorization code was issued to a different client")
	}
	client, err := p.currentProviderClient(owner, input.clientID, input.secret, input.client)
	if err != nil {
		return nil, nil, id.GrantID{}, err
	}
	if err := validateProviderResponseScopes(client, fosite.Arguments(oauth2.SplitScope(current.Scope))); err != nil {
		return nil, nil, id.GrantID{}, err
	}
	decision, err := p.verifyProviderGrant(owner, current.Principal, current.AgentID, at, id.GrantID{})
	if err != nil {
		return nil, nil, id.GrantID{}, err
	}
	return current, client, decision.GrantID, nil
}

func (p *Provider) exchangeCodeInOwner(owner context.Context, at time.Time, input *codeExchangeInput) (*ports.TokenResponse, error, error) {
	owner = withRefreshOperation(owner, at, input.client, nil)
	current, client, grantID, err := p.codeClientInOwner(owner, at, input)
	if err != nil {
		return nil, nil, err
	}
	owner = withRefreshOperation(owner, at, client, nil)
	session := &fosite.DefaultSession{Subject: current.Principal.String(), ExpiresAt: map[fosite.TokenType]time.Time{
		fosite.AccessToken: at.Add(p.config.AccessTokenLifespan),
	}}
	setSessionProfile(session, current.Email, current.DisplayName)
	req := fosite.NewAccessRequest(session)
	req.Client = client
	req.GrantTypes = fosite.Arguments{"authorization_code"}
	req.Form = url.Values{
		"code":          {input.code},
		"redirect_uri":  {input.redirectURI},
		"code_verifier": {input.verifier},
		"grant_type":    {"authorization_code"},
	}

	var activeRoot storage.RefreshSessionAuditIdentity
	if current.UsedAt != nil {
		original, err := p.refreshDeps.Sessions.FindByID(owner, input.rootID)
		if err != nil && !isStorageNotFound(err) {
			return nil, nil, fosite.ErrServerError.WithWrap(err)
		}
		if err == nil && original.TerminalReason == nil {
			activeRoot = original.AuditIdentity()
		}
	}
	// Fosite's used-code path revokes the original request even when the
	// replaying client differs. Commit that side effect, but not other errors.
	if handleErr := p.authCodeHandler.HandleTokenEndpointRequest(owner, req); handleErr != nil {
		if current.UsedAt != nil && errors.Is(handleErr, fosite.ErrInvalidGrant) {
			root, rootErr := p.refreshDeps.Sessions.FindByID(owner, input.rootID)
			if rootErr != nil && !isStorageNotFound(rootErr) {
				return nil, nil, fosite.ErrServerError.WithWrap(rootErr)
			}
			if rootErr == nil && root.TerminalReason == nil {
				return nil, nil, fosite.ErrServerError.WithDebug("authorization-code replay failed to revoke its refresh session")
			}
			if rootErr == nil && root.TerminalReason != nil && *root.TerminalReason == storage.RefreshReasonCodeReplay {
				input.replayRoot = activeRoot
			}
			return nil, handleErr, nil
		}
		if errors.Is(handleErr, fosite.ErrInvalidGrant) {
			return nil, nil, fosite.ErrInvalidGrant.WithWrap(handleErr)
		}
		return nil, nil, handleErr
	}
	if err := p.pkceHandler.HandleTokenEndpointRequest(owner, req); err != nil {
		return nil, nil, err
	}
	response := fosite.NewAccessResponse()
	if err := p.authCodeHandler.PopulateTokenEndpointResponse(owner, req, response); err != nil {
		if errors.Is(err, fosite.ErrInvalidGrant) || errors.Is(err, fosite.ErrInvalidatedAuthorizeCode) {
			return nil, nil, fosite.ErrInvalidGrant.WithWrap(err)
		}
		return nil, nil, err
	}
	if err := p.pkceHandler.PopulateTokenEndpointResponse(owner, req, response); err != nil {
		return nil, nil, err
	}
	secondAt, err := p.refreshDeps.Clock.Now(owner)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	if _, err := p.verifyProviderGrant(owner, current.Principal, current.AgentID, secondAt, grantID); err != nil {
		return nil, nil, err
	}
	secondClient, err := p.fositeStorage.GetClient(owner, input.clientID)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	if response.GetExtra("refresh_token") != nil && !secondClient.GetGrantTypes().Has("refresh_token") {
		return nil, nil, fosite.ErrUnauthorizedClient.WithHint("client no longer permits refresh_token")
	}
	if err := validateProviderResponseScopes(secondClient, req.GetGrantedScopes()); err != nil {
		return nil, nil, err
	}
	staged := &ports.TokenResponse{
		AccessToken: response.GetAccessToken(),
		TokenType:   "Bearer",
		ExpiresIn:   int64(p.config.AccessTokenLifespan.Seconds()),
		Scope:       strings.Join(req.GetGrantedScopes(), " "),
	}
	if refresh, ok := response.GetExtra("refresh_token").(string); ok {
		staged.RefreshToken = refresh
	}
	return staged, nil, nil
}

// HandleRefreshToken processes a refresh_token grant for locally-minted tokens.
func (p *Provider) HandleRefreshToken(
	ctx context.Context,
	clientID string,
	secret string,
	refreshToken string,
	scope string,
) (tokenResp *ports.TokenResponse, err error) {
	var auditInput *refreshExchangeInput
	defer func() {
		event, reason := "RefreshRejected", "client_binding"
		var root *storage.RefreshSession
		if auditInput != nil {
			root = auditInput.auditRoot
			reason = auditInput.auditStage
		}
		if err == nil {
			event, reason = "RefreshRotated", "fresh_rotation"
			if auditInput != nil && auditInput.retry {
				event, reason = "RefreshRetryAccepted", "stored_result"
			}
		} else {
			reason = refreshFailureReason(err, reason)
		}
		p.auditRefresh(ctx, event, reason, clientID, root)
		err = translateFositeError(err)
	}()

	preparedCtx, input, err := p.prepareRefreshExchange(ctx, clientID, secret, refreshToken, scope)
	auditInput = input
	if err != nil {
		return nil, err
	}
	ctx = preparedCtx
	var staged *ports.TokenResponse
	var reuseError, callbackError error
	err = p.refreshDeps.Coordinator.Run(ctx, input.clientAgentID, func(owner context.Context, at time.Time) error {
		staged, reuseError, callbackError = p.refreshInOwner(owner, at, input)
		return callbackError
	})
	if err != nil {
		if callbackError == nil {
			return nil, fosite.ErrServerError.WithDebugf("authorization session commit failed: %v", err)
		}
		return nil, err
	}
	if reuseError != nil {
		if input.auditStage == "prohibited_reuse" && input.auditRoot != nil {
			logRefreshTransition(p.logger, ctx, input.auditRoot.AuditIdentity(), storage.RefreshReasonProhibitedReuse)
		}
		return nil, reuseError
	}
	return staged, nil
}

type refreshExchangeInput struct {
	clientID, secret, token, scope, requestedScope, signature string
	client                                                    fosite.Client
	clientAgentID                                             id.AgentID
	rootID                                                    id.RefreshSessionID
	auditRoot                                                 *storage.RefreshSession
	auditStage                                                string
	retry                                                     bool
}

func (p *Provider) prepareRefreshExchange(ctx context.Context, clientID, secret, refreshToken, scope string) (context.Context, *refreshExchangeInput, error) {
	client, err := p.authenticateProviderClient(ctx, clientID, secret)
	if err != nil {
		return nil, nil, err
	}
	agentID, err := extractAgentID(client)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	signature := p.refreshStrategy.RefreshTokenSignature(ctx, refreshToken)
	token, err := p.refreshDeps.Tokens.FindBySignature(ctx, signature)
	if err != nil {
		if isStorageNotFound(err) {
			return nil, nil, fosite.ErrInvalidGrant.WithHint("refresh token not found")
		}
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	root, err := p.refreshDeps.Sessions.FindByID(ctx, token.SessionID)
	if err != nil {
		if isStorageNotFound(err) {
			return nil, nil, fosite.ErrInvalidGrant.WithHint("refresh session not found")
		}
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	// Keep the root for audit only. A mismatched authenticated client must not
	// enter the owner's transaction or trigger Fosite's consumed-token revocation.
	if root.ClientID.String() != clientID || root.AgentID != agentID {
		return nil, &refreshExchangeInput{auditRoot: root, auditStage: "client_binding"}, fosite.ErrInvalidGrant.WithHint("refresh token belongs to another client")
	}
	if token.UsedAt == nil {
		signer, err := p.accessStrategy.signingKeyService.signingMaterial(ctx)
		if err != nil {
			return nil, nil, fosite.ErrServerError.WithDebugf("failed to prepare signing material: %v", err)
		}
		ctx = withPreparedSigningMaterial(ctx, signer)
	}
	return ctx, &refreshExchangeInput{
		clientID: clientID, secret: secret, token: refreshToken, scope: scope,
		requestedScope: canonicalRequestedScope(scope), signature: signature,
		rootID: root.ID, clientAgentID: agentID, client: client,
		auditRoot: root, auditStage: "client_binding", retry: token.UsedAt != nil,
	}, nil
}

func (p *Provider) authorizedRefreshInOwner(owner context.Context, at time.Time, input *refreshExchangeInput) (context.Context, *storage.RefreshSession, *storage.RefreshToken, fosite.Client, error) {
	token, err := p.refreshDeps.Tokens.FindBySignature(owner, input.signature)
	if err != nil {
		if isStorageNotFound(err) {
			return nil, nil, nil, nil, fosite.ErrInvalidGrant.WithHint("refresh token not found")
		}
		return nil, nil, nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	root, err := p.refreshDeps.Sessions.FindByID(owner, token.SessionID)
	if err != nil {
		return nil, nil, nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	if root.ID != input.rootID || root.ClientID.String() != input.clientID || root.AgentID != input.clientAgentID {
		return nil, nil, nil, nil, fosite.ErrInvalidGrant.WithHint("refresh token belongs to another client")
	}
	owner = withRefreshOperation(owner, at, input.client, root)
	input.auditRoot = root
	input.auditStage = "client_authentication"
	client, err := p.currentProviderClient(owner, input.clientID, input.secret, input.client)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	input.auditStage = "session_revoked"
	if root.TerminalReason != nil {
		return nil, nil, nil, nil, fosite.ErrInvalidGrant.WithHint("refresh session is no longer active")
	}
	input.auditStage = "consent"
	if _, err := p.verifyProviderGrant(owner, root.Principal, root.AgentID, at, root.OriginalGrantID); err != nil {
		return nil, nil, nil, nil, err
	}
	input.auditStage = "session_lifetime"
	if !p.refreshAuthorityActive(root, at) || (token.UsedAt == nil && !at.Before(token.ExpiresAt)) {
		return nil, nil, nil, nil, fosite.ErrInvalidGrant.WithHint("refresh session has expired")
	}
	input.auditStage = "refresh_capability"
	if !client.GetGrantTypes().Has("refresh_token") {
		return nil, nil, nil, nil, fosite.ErrUnauthorizedClient.WithHint("client no longer permits refresh_token")
	}
	input.auditStage = "token_classification"
	if err := p.refreshDeps.Tokens.CheckCurrentLineage(owner, root.ID); err != nil {
		if isStorageNotFound(err) {
			return nil, nil, nil, nil, fosite.ErrInvalidGrant.WithHint("refresh lineage is unsupported")
		}
		return nil, nil, nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	return owner, root, token, client, nil
}

func (p *Provider) refreshInOwner(owner context.Context, at time.Time, input *refreshExchangeInput) (*ports.TokenResponse, error, error) {
	owner, root, token, client, err := p.authorizedRefreshInOwner(owner, at, input)
	if err != nil {
		return nil, nil, err
	}
	if token.UsedAt != nil {
		input.retry = true
		return p.retryRefreshInOwner(owner, at, input, root, client)
	}
	owner = withRefreshOperation(owner, at, client, root)
	op, _ := refreshOperationFromContext(owner)
	op.requestedScope = input.requestedScope
	requestForm := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {input.token},
		"client_id":     {input.clientID},
	}
	if input.scope != "" {
		requestForm.Set("scope", input.scope)
	}
	req := fosite.NewAccessRequest(&fosite.DefaultSession{})
	req.Client = &refreshProtocolClient{Client: client, agent: client.(agentHolder).getAgent(), scopeCeiling: fosite.Arguments(oauth2.SplitScope(root.Scope))}
	req.GrantTypes = fosite.Arguments{"refresh_token"}
	req.Form = requestForm

	if handleErr := p.refreshHandler.HandleTokenEndpointRequest(owner, req); handleErr != nil {
		return nil, nil, handleErr
	}
	if input.scope != "" {
		if input.requestedScope == "" {
			return nil, nil, fosite.ErrInvalidScope.WithHint("requested scope is empty")
		}
		narrowed := fosite.Arguments(oauth2.SplitScope(input.requestedScope))
		for _, requested := range narrowed {
			if !req.GetGrantedScopes().Has(requested) {
				return nil, nil, fosite.ErrInvalidScope.WithHintf("scope %q was not granted", requested)
			}
		}
		req.RequestedScope = narrowed
		req.GrantedScope = narrowed
	}
	input.auditStage = "response_scope"
	if err := validateProviderResponseScopes(client, req.GetGrantedScopes()); err != nil {
		return nil, nil, err
	}
	response := fosite.NewAccessResponse()
	if err := p.refreshHandler.PopulateTokenEndpointResponse(owner, req, response); err != nil {
		return nil, nil, err
	}
	secondAt, err := p.refreshDeps.Clock.Now(owner)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	secondClient, err := p.currentProviderClient(owner, input.clientID, input.secret, client)
	if err != nil {
		return nil, nil, err
	}
	updated, err := p.refreshDeps.Sessions.FindByID(owner, root.ID)
	if err != nil {
		return nil, nil, fosite.ErrServerError.WithWrap(err)
	}
	if updated.TerminalReason != nil {
		return nil, nil, fosite.ErrInvalidGrant.WithHint("refresh session is no longer active")
	}
	if _, err := p.verifyProviderGrant(owner, root.Principal, root.AgentID, secondAt, root.OriginalGrantID); err != nil {
		return nil, nil, err
	}
	if !p.refreshAuthorityActive(root, secondAt) || !secondAt.Before(token.ExpiresAt) || !p.refreshAuthorityActive(updated, secondAt) {
		return nil, nil, fosite.ErrInvalidGrant.WithHint("refresh session expired before commit")
	}
	if !secondClient.GetGrantTypes().Has("refresh_token") {
		return nil, nil, fosite.ErrUnauthorizedClient.WithHint("client no longer permits refresh_token")
	}
	if err := validateProviderResponseScopes(secondClient, req.GetGrantedScopes()); err != nil {
		return nil, nil, err
	}
	staged := &ports.TokenResponse{
		AccessToken: response.GetAccessToken(),
		TokenType:   "Bearer",
		ExpiresIn:   int64(p.config.AccessTokenLifespan.Seconds()),
		Scope:       strings.Join(req.GetGrantedScopes(), " "),
	}
	if next, ok := response.GetExtra("refresh_token").(string); ok {
		staged.RefreshToken = next
	}
	return staged, nil, nil
}

// authenticateProviderClient resolves metadata and verifies a client's current secret.
// Call before taking the agent gate so a CIMD lookup cannot fetch while holding SQL.
func (p *Provider) authenticateProviderClient(ctx context.Context, clientID, secret string) (fosite.Client, error) {
	client, err := p.fositeStorage.GetClient(ctx, clientID)
	if err != nil {
		if errors.Is(err, fosite.ErrNotFound) {
			return nil, fosite.ErrInvalidClient.WithHintf("client %s not found", clientID)
		}
		return nil, fosite.ErrServerError.WithDebugf("client lookup failed: %v", err)
	}
	switch bc := client.(type) {
	case *confidentialClient:
		authenticated, err := p.clientAuth.Authenticate(ctx, bc.agent.ID, secret)
		if err != nil {
			return nil, err
		}
		return &confidentialClient{clientID: clientID, agent: authenticated.Agent, credential: authenticated.Credential}, nil
	case *publicClient:
		return client, nil
	default:
		return nil, fosite.ErrServerError.WithDebugf("unexpected client type %T", client)
	}
}

func (p *Provider) currentProviderClient(ctx context.Context, clientID, secret string, prepared fosite.Client) (fosite.Client, error) {
	current, err := p.authenticateProviderClient(ctx, clientID, secret)
	if err != nil {
		return nil, err
	}
	before, err := extractAgentID(prepared)
	if err != nil {
		return nil, fosite.ErrServerError.WithWrap(err)
	}
	after, err := extractAgentID(current)
	if err != nil {
		return nil, fosite.ErrServerError.WithWrap(err)
	}
	if current.IsPublic() != prepared.IsPublic() || current.GetID() != prepared.GetID() || before != after {
		return nil, fosite.ErrInvalidClient.WithHint("client authentication changed during token exchange")
	}
	return current, nil
}

func validateProviderResponseScopes(client fosite.Client, granted fosite.Arguments) error {
	allowed := []string(client.GetScopes())
	for _, name := range granted {
		if !oauth2.IsScopeAllowed(allowed, name) {
			return fosite.ErrInvalidScope.WithHintf("scope %q is no longer allowed for this client", name)
		}
	}
	return nil
}

func (p *Provider) verifyProviderGrant(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time, original id.GrantID) (ports.UserDelegationDecision, error) {
	decision, err := p.refreshDeps.Verifier.VerifyUserDelegation(ctx, principal, agentID, at)
	if err != nil {
		return ports.UserDelegationDecision{}, fosite.ErrServerError.WithWrap(err)
	}
	if decision.Status != ports.UserDelegationActive || decision.GrantID.IsZero() ||
		(decision.ValidUntil != nil && !decision.ValidUntil.After(at)) ||
		(!original.IsZero() && decision.GrantID != original) {
		return ports.UserDelegationDecision{}, fosite.ErrInvalidGrant.WithHint("original user delegation is no longer active")
	}
	return decision, nil
}

func (p *Provider) refreshAuthorityActive(root *storage.RefreshSession, at time.Time) bool {
	policy := p.refreshDeps.Policy
	return at.Before(root.InactivityExpiresAt) && at.Before(root.LastFreshAt.Add(policy.InactivityLifetime)) &&
		(root.AbsoluteExpiresAt == nil || at.Before(*root.AbsoluteExpiresAt)) &&
		(policy.AbsoluteLifetime == 0 || at.Before(root.StartedAt.Add(policy.AbsoluteLifetime)))
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
