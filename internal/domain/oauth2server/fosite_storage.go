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
	fositestorage "github.com/ory/fosite/storage"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ fositestorage.Transactional = (*FositeStorage)(nil)

// FositeStorage wraps project repositories to implement fosite storage interfaces.
type FositeStorage struct {
	codeRepo       ports.AuthorizationCodeRepository
	refreshRepo    ports.RefreshTokenSessionRepository
	transactions   ports.OAuth2TransactionManager
	pkceRepo       ports.PKCESessionRepository
	credRepo       ports.ClientCredentialRepository
	clientResolver ports.ClientResolver
	logger         *slog.Logger
}

// NewFositeStorage creates a new FositeStorage wrapping our repositories.
func NewFositeStorage(
	codeRepo ports.AuthorizationCodeRepository,
	refreshRepo ports.RefreshTokenSessionRepository,
	pkceRepo ports.PKCESessionRepository,
	credRepo ports.ClientCredentialRepository,
	clientResolver ports.ClientResolver,
	logger *slog.Logger,
	transactions ...ports.OAuth2TransactionManager,
) *FositeStorage {
	storage := &FositeStorage{
		codeRepo:       codeRepo,
		refreshRepo:    refreshRepo,
		pkceRepo:       pkceRepo,
		credRepo:       credRepo,
		clientResolver: clientResolver,
		logger:         logger,
	}
	if len(transactions) > 0 {
		storage.transactions = transactions[0]
	}
	return storage
}

// mapStorageError translates a storage error for fosite consumption.
// Not-found errors become fosite.ErrNotFound so fosite maps them to invalid_grant/invalid_client.
// Infrastructure errors (connection, timeout) are logged and returned as-is so fosite surfaces
// them as server_error rather than silently treating them as "not found".
func (s *FositeStorage) mapStorageError(ctx context.Context, err error) error {
	var se *storage.StorageError
	if errors.As(err, &se) {
		if se.Kind == storage.ErrorKindNotFound {
			return fosite.ErrNotFound
		}
		s.logger.ErrorContext(ctx, "storage failure in OAuth2 flow",
			"storage_op", se.Operation,
			"storage_kind", string(se.Kind),
		)
		return err
	}
	s.logger.ErrorContext(ctx, "unexpected error type in OAuth2 flow", "error", err.Error())
	return err
}

// BeginTX begins the storage transaction Fosite uses for token operations.
func (s *FositeStorage) BeginTX(ctx context.Context) (context.Context, error) {
	if s.transactions == nil {
		return ctx, nil
	}
	return s.transactions.BeginTX(ctx)
}

// Commit commits the storage transaction for a Fosite token operation.
func (s *FositeStorage) Commit(ctx context.Context) error {
	if s.transactions == nil {
		return nil
	}
	return s.transactions.Commit(ctx)
}

// Rollback rolls back the storage transaction for a Fosite token operation.
func (s *FositeStorage) Rollback(ctx context.Context) error {
	if s.transactions == nil {
		return nil
	}
	return s.transactions.Rollback(ctx)
}

// CreateAuthorizeCodeSession stores an authorization code issued by the authorization endpoint.
func (s *FositeStorage) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	agentID, err := extractAgentID(req.GetClient())
	if err != nil {
		return fmt.Errorf("CreateAuthorizeCodeSession: %w", err)
	}
	session := req.GetSession()
	authCode := &storage.AuthorizationCode{
		ID:            id.NewAuthorizationCodeID(),
		CodeHash:      code, // code is already the fosite signature (sha256Hex of raw code)
		AgentID:       agentID,
		ClientID:      id.NewClientID(req.GetRequestForm().Get("client_id")),
		Principal:     id.NewPrincipal(session.GetSubject()),
		RedirectURI:   req.GetRequestForm().Get("redirect_uri"),
		CodeChallenge: req.GetRequestForm().Get("code_challenge"),
		Scope:         strings.Join(req.GetRequestedScopes(), " "),
		ExpiresAt:     session.GetExpiresAt(fosite.AuthorizeCode),
		CreatedAt:     time.Now(),
	}
	if profile, ok := principal.ProfileFromContext(ctx); ok {
		authCode.Email = profile.Email()
		authCode.DisplayName = profile.DisplayName()
	}
	return s.codeRepo.Create(ctx, authCode)
}

// GetAuthorizeCodeSession retrieves an authorization code session by code signature.
func (s *FositeStorage) GetAuthorizeCodeSession(ctx context.Context, code string, requestSession fosite.Session) (fosite.Requester, error) {
	authCode, err := s.codeRepo.FindByCodeHash(ctx, code)
	if err != nil {
		return nil, s.mapStorageError(ctx, err)
	}
	if !authCode.ExpiresAt.After(time.Now()) {
		return nil, fosite.ErrInvalidGrant
	}

	// Fosite validates the caller's session before replacing it with the stored session.
	if requestSession != nil {
		requestSession.SetExpiresAt(fosite.AuthorizeCode, authCode.ExpiresAt)
	}

	// Look up the client using the stored ClientID (the original client_id from the authorize
	// request). For CIMD clients this is the metadata URL; for opaque clients it's the UUID.
	// Using AgentID.String() would fail for CIMD clients whose resolver rejects UUID lookups.
	clientLookupID := authCode.ClientID.String()
	if clientLookupID == "" {
		clientLookupID = authCode.AgentID.String()
	}
	client, err := s.GetClient(ctx, clientLookupID)
	if err != nil {
		return nil, fmt.Errorf("failed to look up client: %w", err)
	}

	// Reconstruct the fosite request
	session := &fosite.DefaultSession{
		Subject: authCode.Principal.String(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.AuthorizeCode: authCode.ExpiresAt,
		},
	}
	setSessionProfile(session, authCode.Email, authCode.DisplayName)

	req := &fosite.Request{
		ID:             authCode.ID.String(),
		Client:         client,
		Session:        session,
		RequestedScope: fosite.Arguments(oauth2.SplitScope(authCode.Scope)),
		GrantedScope:   fosite.Arguments(oauth2.SplitScope(authCode.Scope)),
		Form: map[string][]string{
			"redirect_uri":   {authCode.RedirectURI},
			"code_challenge": {authCode.CodeChallenge},
			// code_challenge_method is hardcoded to S256 because EnablePKCEPlainChallengeMethod
			// is false in provider config, so only S256 can reach this point. The actual PKCE
			// verification uses GetPKCERequestSession which stores the method independently.
			"code_challenge_method": {"S256"},
		},
		RequestedAt: authCode.CreatedAt,
	}

	// fosite requires a non-nil requester alongside ErrInvalidatedAuthorizeCode so it can
	// revoke associated tokens (RFC 6749 §4.1.2 — code replay must revoke previous grants).
	if authCode.UsedAt != nil {
		return req, fosite.ErrInvalidatedAuthorizeCode
	}

	return req, nil
}

// InvalidateAuthorizeCodeSession marks an authorization code as used.
func (s *FositeStorage) InvalidateAuthorizeCodeSession(ctx context.Context, code string) error {
	authCode, err := s.codeRepo.FindByCodeHash(ctx, code)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	if err := s.codeRepo.MarkUsed(ctx, authCode.ID); err != nil {
		mapped := s.mapStorageError(ctx, err)
		if errors.Is(mapped, fosite.ErrNotFound) {
			return fosite.ErrInvalidatedAuthorizeCode
		}
		return mapped
	}
	return nil
}

// CreateAccessTokenSession is a no-op for stateless JWT tokens.
func (s *FositeStorage) CreateAccessTokenSession(_ context.Context, _ string, _ fosite.Requester) error {
	return nil // JWT tokens are stateless — no storage needed
}

// GetAccessTokenSession is not needed for stateless JWT tokens.
func (s *FositeStorage) GetAccessTokenSession(_ context.Context, _ string, _ fosite.Session) (fosite.Requester, error) {
	return nil, fosite.ErrNotFound // JWT tokens are stateless
}

// DeleteAccessTokenSession is a no-op for stateless JWT tokens.
func (s *FositeStorage) DeleteAccessTokenSession(_ context.Context, _ string) error {
	return nil // JWT tokens are stateless
}

// CreateRefreshTokenSession stores a refresh token for single-use rotation.
func (s *FositeStorage) CreateRefreshTokenSession(ctx context.Context, signature string, _ string, req fosite.Requester) error {
	agentID, err := extractAgentID(req.GetClient())
	if err != nil {
		return fmt.Errorf("CreateRefreshTokenSession: %w", err)
	}

	email, displayName := sessionProfile(req.GetSession())

	session := &storage.RefreshTokenSession{
		Signature:   signature,
		RequestID:   req.GetID(),
		AgentID:     agentID,
		ClientID:    id.NewClientID(req.GetClient().GetID()),
		Principal:   id.NewPrincipal(req.GetSession().GetSubject()),
		Email:       email,
		DisplayName: displayName,
		Scope:       strings.Join(req.GetGrantedScopes(), " "),
		ExpiresAt:   req.GetSession().GetExpiresAt(fosite.RefreshToken),
		CreatedAt:   time.Now(),
	}
	if err := session.Validate(); err != nil {
		return err
	}
	return s.refreshRepo.Create(ctx, session)
}

// GetRefreshTokenSession retrieves and hydrates a refresh token session.
func (s *FositeStorage) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	refreshSession, err := s.refreshRepo.FindBySignature(ctx, signature)
	if err != nil {
		return nil, s.mapStorageError(ctx, err)
	}

	client, err := s.GetClient(ctx, refreshSession.ClientID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to look up client: %w", err)
	}

	sess := &fosite.DefaultSession{
		Subject: refreshSession.Principal.String(),
		ExpiresAt: map[fosite.TokenType]time.Time{
			fosite.RefreshToken: refreshSession.ExpiresAt,
		},
	}
	setSessionProfile(sess, refreshSession.Email, refreshSession.DisplayName)

	req := &fosite.Request{
		ID:             refreshSession.RequestID,
		Client:         client,
		Session:        sess,
		RequestedScope: fosite.Arguments(oauth2.SplitScope(refreshSession.Scope)),
		GrantedScope:   fosite.Arguments(oauth2.SplitScope(refreshSession.Scope)),
		RequestedAt:    refreshSession.CreatedAt,
	}

	if refreshSession.UsedAt != nil {
		return req, fosite.ErrInactiveToken
	}

	return req, nil
}

// DeleteRefreshTokenSession marks a refresh token inactive. It is idempotent: a token that is
// already used (or absent) is treated as successfully deleted, which fosite relies on during
// refresh-token reuse detection.
func (s *FositeStorage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	if err := s.refreshRepo.MarkUsed(ctx, signature); err != nil {
		var se *storage.StorageError
		if errors.As(err, &se) && se.Kind == storage.ErrorKindNotFound {
			return nil
		}
		return s.mapStorageError(ctx, err)
	}
	return nil
}

// RotateRefreshToken marks the presented refresh token inactive before minting a new one.
func (s *FositeStorage) RotateRefreshToken(ctx context.Context, _ string, signature string) error {
	if err := s.refreshRepo.MarkUsed(ctx, signature); err != nil {
		return s.mapStorageError(ctx, err)
	}
	return nil
}

// RevokeAccessToken is a no-op for stateless JWT tokens.
func (s *FositeStorage) RevokeAccessToken(_ context.Context, _ string) error {
	return nil
}

// RevokeRefreshToken revokes all refresh tokens belonging to a request chain.
func (s *FositeStorage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	if err := s.refreshRepo.RevokeByRequestID(ctx, requestID); err != nil {
		return s.mapStorageError(ctx, err)
	}
	return nil
}

// CreatePKCERequestSession stores the PKCE code challenge in a dedicated store keyed by code signature.
func (s *FositeStorage) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	session := &storage.PKCESession{
		Signature:           signature,
		CodeChallenge:       req.GetRequestForm().Get("code_challenge"),
		CodeChallengeMethod: req.GetRequestForm().Get("code_challenge_method"),
		ExpiresAt:           req.GetSession().GetExpiresAt(fosite.AuthorizeCode),
		CreatedAt:           time.Now(),
	}
	if err := session.Validate(); err != nil {
		return err
	}
	return s.pkceRepo.Create(ctx, session)
}

// GetPKCERequestSession retrieves the PKCE challenge for verifying the code verifier at the token endpoint.
func (s *FositeStorage) GetPKCERequestSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	session, err := s.pkceRepo.FindBySignature(ctx, signature)
	if err != nil {
		return nil, s.mapStorageError(ctx, err)
	}
	return &fosite.Request{
		Form: url.Values{
			"code_challenge":        {session.CodeChallenge},
			"code_challenge_method": {session.CodeChallengeMethod},
		},
	}, nil
}

// DeletePKCERequestSession removes the PKCE session after successful token exchange (one-shot).
func (s *FositeStorage) DeletePKCERequestSession(ctx context.Context, signature string) error {
	if err := s.pkceRepo.Delete(ctx, signature); err != nil {
		return s.mapStorageError(ctx, err)
	}
	return nil
}

// GetClient satisfies the fosite.Storage interface.
// Delegates to ClientResolver which handles both UUID-format and URL-format client_id values.
func (s *FositeStorage) GetClient(ctx context.Context, clientID string) (fosite.Client, error) {
	resolution, err := s.clientResolver.ResolveClient(ctx, id.ClientID(clientID))
	if err != nil {
		var clientErr *ports.ClientIDError
		if errors.As(err, &clientErr) {
			if clientErr.Code == "server_error" {
				s.logger.ErrorContext(ctx, "infrastructure error during client resolution",
					"client_id", clientID, "error", clientErr.Desc)
				return nil, err
			}
			return nil, fosite.ErrNotFound
		}
		return nil, err
	}

	if resolution.CIMDMetadata != nil {
		return &publicClient{
			clientID:     clientID,
			agent:        resolution.Agent,
			redirectURIs: resolution.CIMDMetadata.RedirectURIs,
			grantTypes:   publicClientGrantTypes(resolution.CIMDMetadata.GrantTypes),
		}, nil
	}

	// LocalClient agents may or may not have credentials registered.
	// Try the credential store first; if none exist, treat as a public client
	// (PKCE-only authorization_code flow). This allows the same agent to be used
	// as a confidential client (client_credentials) when credentials are provisioned
	// and as a public client (authorization_code + PKCE) when they are not.
	cred, err := s.credRepo.GetByAgentID(ctx, resolution.Agent.ID)
	if err != nil {
		if isStorageNotFound(err) && resolution.Agent.ClientType() == storage.LocalClient {
			return &publicClient{
				clientID:     clientID,
				agent:        resolution.Agent,
				redirectURIs: resolution.Agent.RedirectURIs,
				grantTypes:   fosite.Arguments{"authorization_code", "refresh_token"},
			}, nil
		}
		return nil, s.mapStorageError(ctx, err)
	}
	return &confidentialClient{clientID: clientID, agent: resolution.Agent, credential: cred}, nil
}

// ClientAssertionJWTValid rejects all JWT assertions. This broker uses client_secret_basic only;
// private_key_jwt and client_secret_jwt are not supported. Returning ErrJTIKnown fails closed:
// any accidental invocation rejects the assertion rather than silently accepting a replay.
func (s *FositeStorage) ClientAssertionJWTValid(_ context.Context, _ string) error {
	return fosite.ErrJTIKnown
}

// SetClientAssertionJWT is a no-op. JWT client assertions are not supported;
// ClientAssertionJWTValid always rejects, so JTIs never need to be recorded.
func (s *FositeStorage) SetClientAssertionJWT(_ context.Context, _ string, _ time.Time) error {
	return nil
}
