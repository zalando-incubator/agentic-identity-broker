package oauth2server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
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

// FositeStorage adapts code, PKCE and native refresh repositories for Fosite.
type FositeStorage struct {
	codeRepo       ports.AuthorizationCodeRepository
	refresh        RefreshSessionDependencies
	transactions   ports.StorageTransactionManager
	pkceRepo       ports.PKCESessionRepository
	credRepo       ports.ClientCredentialRepository
	clientResolver ports.ClientResolver
	logger         *slog.Logger
}

func NewFositeStorage(
	codeRepo ports.AuthorizationCodeRepository,
	refreshDeps RefreshSessionDependencies,
	pkceRepo ports.PKCESessionRepository,
	credRepo ports.ClientCredentialRepository,
	clientResolver ports.ClientResolver,
	logger *slog.Logger,
	transactions ports.StorageTransactionManager,
) *FositeStorage {
	return &FositeStorage{
		codeRepo: codeRepo, refresh: refreshDeps, transactions: transactions,
		pkceRepo: pkceRepo, credRepo: credRepo, clientResolver: clientResolver, logger: logger,
	}
}

// mapStorageError translates a storage error for fosite consumption.
// Not-found errors become fosite.ErrNotFound so fosite maps them to invalid_grant/invalid_client.
// Infrastructure errors (connection, timeout) are logged and returned as-is so fosite surfaces
// them as server_error rather than silently treating them as "not found".
func (s *FositeStorage) mapStorageError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
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

// Fosite joins an existing authorization operation without owning its commit.
func (s *FositeStorage) BeginTX(ctx context.Context) (context.Context, error) {
	if ctx.Value(refreshOperationKey{}) != nil {
		return ctx, nil
	}
	if s.transactions == nil {
		return nil, errors.New("fosite storage requires a transaction manager")
	}
	return s.transactions.BeginTX(ctx)
}

func (s *FositeStorage) Commit(ctx context.Context) error {
	if ctx.Value(refreshOperationKey{}) != nil {
		return nil
	}
	if s.transactions == nil {
		return errors.New("fosite storage requires a transaction manager")
	}
	return s.transactions.Commit(ctx)
}

func (s *FositeStorage) Rollback(ctx context.Context) error {
	if ctx.Value(refreshOperationKey{}) != nil {
		return nil
	}
	if s.transactions == nil {
		return errors.New("fosite storage requires a transaction manager")
	}
	return s.transactions.Rollback(ctx)
}

// CreateAuthorizeCodeSession stores an authorization code issued by the authorization endpoint.
func (s *FositeStorage) CreateAuthorizeCodeSession(ctx context.Context, code string, req fosite.Requester) error {
	agentID, err := extractAgentID(req.GetClient())
	if err != nil {
		return fmt.Errorf("CreateAuthorizeCodeSession: %w", err)
	}
	at, err := s.refreshTime(ctx)
	if err != nil {
		return err
	}
	session := req.GetSession()
	authCode := &storage.AuthorizationCode{
		ID:            id.NewAuthorizationCodeID(),
		CodeHash:      code, // code is already the fosite signature (sha256Hex of raw code)
		AgentID:       agentID,
		ClientID:      id.NewClientID(req.GetClient().GetID()),
		Principal:     id.NewPrincipal(session.GetSubject()),
		RedirectURI:   req.GetRequestForm().Get("redirect_uri"),
		CodeChallenge: req.GetRequestForm().Get("code_challenge"),
		Scope:         canonicalRequestedScope(strings.Join(req.GetGrantedScopes(), " ")),
		ExpiresAt:     session.GetExpiresAt(fosite.AuthorizeCode),
		CreatedAt:     at,
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
	at, err := s.refreshTime(ctx)
	if err != nil {
		return nil, err
	}
	if authCode == nil || !authCode.ExpiresAt.After(at) {
		return nil, fosite.ErrInvalidGrant
	}
	if op, ok := refreshOperationFromContext(ctx); ok {
		op.code = authCode
	}

	// Fosite validates the caller's session before replacing it with the stored session.
	if requestSession != nil {
		requestSession.SetExpiresAt(fosite.AuthorizeCode, authCode.ExpiresAt)
	}

	client, err := s.codeClient(ctx, authCode)
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

// CreateRefreshTokenSession persists first issuance or one fresh successor.
// Native repositories also write the anchored legacy mirror in this scope.
func (s *FositeStorage) CreateRefreshTokenSession(ctx context.Context, signature string, _ string, req fosite.Requester) error {
	op, err := s.refreshOwner(ctx)
	if err != nil {
		return err
	}
	if req == nil || req.GetSession() == nil || req.GetClient() == nil || req.GetClient().GetID() != op.client.GetID() {
		return errors.New("refresh issuance requires the authenticated request client")
	}
	currentClient, err := s.currentPreparedClient(ctx, op.client)
	if err != nil {
		return err
	}
	if !currentClient.GetGrantTypes().Has("refresh_token") {
		return fosite.ErrUnauthorizedClient
	}
	for _, scope := range req.GetGrantedScopes() {
		if !oauth2.IsScopeAllowed(currentClient.GetScopes(), scope) {
			return fosite.ErrInvalidScope
		}
	}
	if op.root == nil {
		return s.createOriginalRefresh(ctx, op, signature, req)
	}
	return s.createSuccessorRefresh(ctx, op, signature, req)
}

// GetRefreshTokenSession authenticates the original owner before exposing an
// inactive token to Fosite's family-revocation path.
func (s *FositeStorage) GetRefreshTokenSession(ctx context.Context, signature string, _ fosite.Session) (fosite.Requester, error) {
	op, err := s.refreshOwner(ctx)
	if err != nil {
		return nil, err
	}
	root, token, client, err := s.boundNativeRefresh(ctx, op, signature)
	if err != nil {
		return nil, err
	}
	sess := &fosite.DefaultSession{Subject: root.Principal.String(), ExpiresAt: map[fosite.TokenType]time.Time{fosite.RefreshToken: token.ExpiresAt}}
	setSessionProfile(sess, root.Email, root.DisplayName)
	req := &fosite.Request{
		ID: root.ID.String(), Client: client, Session: sess,
		RequestedScope: fosite.Arguments(oauth2.SplitScope(root.Scope)),
		GrantedScope:   fosite.Arguments(oauth2.SplitScope(root.Scope)),
		RequestedAt:    root.StartedAt,
	}
	if root.TerminalReason != nil || token.UsedAt != nil {
		return req, fosite.ErrInactiveToken
	}
	if root.CurrentSignature != token.Signature {
		return nil, errors.New("unconsumed refresh token is not the current successor")
	}
	if !op.at.Before(token.ExpiresAt) || !op.at.Before(root.InactivityExpiresAt) || (root.AbsoluteExpiresAt != nil && !op.at.Before(*root.AbsoluteExpiresAt)) {
		return nil, fosite.ErrNotFound
	}
	return req, nil
}

// boundNativeRefresh establishes original client ownership and first-token
// evidence before the caller can observe a consumed signature's state.
func (s *FositeStorage) boundNativeRefresh(ctx context.Context, op *refreshOperation, signature string) (*storage.RefreshSession, *storage.RefreshToken, fosite.Client, error) {
	token, err := s.refresh.Tokens.FindBySignature(ctx, signature)
	if err != nil {
		return nil, nil, nil, s.mapStorageError(ctx, err)
	}
	if token == nil || token.Signature != signature || token.Validate() != nil {
		return nil, nil, nil, errors.New("refresh token has no valid native evidence")
	}
	root, err := s.refresh.Sessions.FindByID(ctx, token.SessionID)
	if err != nil {
		return nil, nil, nil, s.mapStorageError(ctx, err)
	}
	if root == nil || root.ID != token.SessionID || op.root == nil || op.root.ID != root.ID || op.root.CurrentSignature != root.CurrentSignature {
		return nil, nil, nil, errors.New("refresh operation does not own the current native root")
	}
	if id.ClientID(op.client.GetID()) != root.ClientID {
		return nil, nil, nil, fosite.ErrNotFound
	}
	ownerID, err := extractAgentID(op.client)
	if err != nil || ownerID != root.AgentID {
		return nil, nil, nil, fosite.ErrNotFound
	}
	if err := root.Validate(); err != nil {
		return nil, nil, nil, fmt.Errorf("refresh root has invalid authorization evidence: %w", err)
	}
	client, err := s.currentPreparedClient(ctx, op.client)
	if err != nil {
		return nil, nil, nil, err
	}
	original := token
	if root.OriginalTokenSignature != signature {
		original, err = s.refresh.Tokens.FindBySignature(ctx, root.OriginalTokenSignature)
		if err != nil {
			return nil, nil, nil, s.mapStorageError(ctx, err)
		}
	}
	if original == nil || original.Validate() != nil || original.SessionID != root.ID || original.Signature != root.OriginalTokenSignature || !original.IssuedAt.Equal(root.StartedAt) || original.ExpiresAt.After(root.RetainUntil) || (root.CurrentSignature != root.OriginalTokenSignature && original.UsedAt == nil) {
		return nil, nil, nil, errors.New("refresh origin token does not match the immutable root")
	}
	if root.CurrentSignature == token.Signature && !token.IssuedAt.Equal(root.LastFreshAt) {
		return nil, nil, nil, errors.New("current refresh token was not issued at the last fresh rotation")
	}
	return root, token, client, nil
}

func (s *FositeStorage) refreshOwner(ctx context.Context) (*refreshOperation, error) {
	op, ok := refreshOperationFromContext(ctx)
	if !ok || op.at.Location() != time.UTC {
		return nil, errors.New("native refresh requires a borrowed authorization operation at UTC decision time")
	}
	if s.refresh.Agents == nil || s.refresh.Sessions == nil || s.refresh.Tokens == nil || s.refresh.Revocations == nil || s.refresh.Clock == nil || s.refresh.Coordinator == nil || s.refresh.Verifier == nil || s.refresh.Encryption == nil || s.refresh.BranchKeys == nil || s.transactions == nil || s.clientResolver == nil || s.credRepo == nil {
		return nil, errors.New("native refresh requires complete storage and authorization dependencies")
	}
	if s.refresh.Policy.InactivityLifetime <= 0 || s.refresh.Policy.AbsoluteLifetime < 0 || s.refresh.Policy.ReuseInterval < 0 {
		return nil, errors.New("native refresh requires a valid resolved policy")
	}
	return op, nil
}

func (s *FositeStorage) activeGrant(ctx context.Context, op *refreshOperation, owner id.Principal, agent id.AgentID) (id.GrantID, error) {
	decision, err := s.refresh.Verifier.VerifyUserDelegation(ctx, owner, agent, op.at)
	if err != nil {
		return id.GrantID{}, fmt.Errorf("refresh grant verification: %w", err)
	}
	if decision.Status != ports.UserDelegationActive || decision.GrantID.IsZero() || (decision.ValidUntil != nil && !op.at.Before(*decision.ValidUntil)) {
		return id.GrantID{}, fosite.ErrInvalidGrant
	}
	return decision.GrantID, nil
}

func (s *FositeStorage) createOriginalRefresh(ctx context.Context, op *refreshOperation, signature string, req fosite.Requester) error {
	code := op.code
	if code == nil || code.UsedAt != nil || code.CodeHash == "" || code.ID.IsZero() || s.codeRepo == nil {
		return errors.New("first refresh issuance requires an exchanged authorization code")
	}
	current, err := s.codeRepo.FindByCodeHash(ctx, code.CodeHash)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	agentID, err := extractAgentID(op.client)
	if err != nil {
		return err
	}
	email, displayName := sessionProfile(req.GetSession())
	if current == nil || current.UsedAt == nil || current.ID != code.ID || current.CodeHash != code.CodeHash || !current.ExpiresAt.After(op.at) || current.ID.String() != req.GetID() {
		return errors.New("first refresh token lacks a consumed original authorization code")
	}
	if current.AgentID != agentID || current.ClientID != id.ClientID(op.client.GetID()) || current.Principal != id.Principal(req.GetSession().GetSubject()) || current.Scope != code.Scope || current.Scope != canonicalRequestedScope(strings.Join(req.GetGrantedScopes(), " ")) {
		return errors.New("first refresh token has mismatched owner or scope evidence")
	}
	if (current.Email == nil) != (email == nil) || (current.Email != nil && *current.Email != *email) || current.DisplayName != displayName {
		return errors.New("first refresh token has mismatched original profile evidence")
	}
	grantID, err := s.activeGrant(ctx, op, current.Principal, agentID)
	if err != nil {
		return err
	}
	rootID, err := id.ParseRefreshSessionID(current.ID.String())
	if err != nil || rootID.IsZero() {
		return errors.New("authorization code has no native refresh UUID")
	}
	root := &storage.RefreshSession{
		ID: rootID, OriginalGrantID: grantID, OriginalTokenSignature: signature,
		AgentID: agentID, Principal: current.Principal, ClientID: current.ClientID,
		Scope: current.Scope, Email: current.Email, DisplayName: current.DisplayName,
		CurrentSignature: signature,
	}
	if err := initializeRefreshSession(root, s.refresh.Policy, op.at); err != nil {
		return err
	}
	token := &storage.RefreshToken{Signature: signature, SessionID: root.ID, IssuedAt: op.at, ExpiresAt: root.InactivityExpiresAt}
	if err := root.Validate(); err != nil {
		return err
	}
	if err := token.Validate(); err != nil {
		return err
	}
	if err := s.refresh.Sessions.Create(ctx, root); err != nil {
		return s.mapStorageError(ctx, err)
	}
	return s.mapStorageError(ctx, s.refresh.Tokens.Create(ctx, token))
}

func (s *FositeStorage) createSuccessorRefresh(ctx context.Context, op *refreshOperation, signature string, req fosite.Requester) error {
	if op.presentedSignature == "" || op.accessExpiresAt.IsZero() || !op.accessExpiresAt.After(op.at) {
		return errors.New("fresh rotation requires a consumed predecessor and signed access expiry")
	}
	root, err := s.refresh.Sessions.FindByID(ctx, op.root.ID)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	if root == nil || root.Validate() != nil || root.TerminalReason != nil || root.ID.String() != req.GetID() || root.CurrentSignature != op.presentedSignature || root.CurrentSignature != op.root.CurrentSignature || signature == root.CurrentSignature || !op.at.After(root.LastFreshAt) {
		return errors.New("fresh rotation is not bound to the current root")
	}
	if root.ClientID != id.ClientID(req.GetClient().GetID()) || root.Principal != id.Principal(req.GetSession().GetSubject()) {
		return errors.New("fresh rotation changed the original owner")
	}
	agentID, err := extractAgentID(op.client)
	if err != nil || root.AgentID != agentID {
		return fosite.ErrNotFound
	}
	grantID, err := s.activeGrant(ctx, op, root.Principal, root.AgentID)
	if err != nil {
		return err
	}
	if grantID != root.OriginalGrantID {
		return fosite.ErrInvalidGrant
	}
	predecessor, err := s.refresh.Tokens.FindBySignature(ctx, op.presentedSignature)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	if predecessor == nil || predecessor.SessionID != root.ID || predecessor.UsedAt == nil || !predecessor.UsedAt.Equal(op.at) || !op.at.Before(predecessor.ExpiresAt) || !op.at.Before(root.InactivityExpiresAt) || !op.at.Before(root.LastFreshAt.Add(s.refresh.Policy.InactivityLifetime)) || (root.AbsoluteExpiresAt != nil && !op.at.Before(*root.AbsoluteExpiresAt)) {
		return fosite.ErrInvalidGrant
	}
	if s.refresh.Policy.AbsoluteLifetime > 0 {
		candidate := root.StartedAt.Add(s.refresh.Policy.AbsoluteLifetime)
		if !op.at.Before(candidate) {
			return fosite.ErrInvalidGrant
		}
		if root.AbsoluteExpiresAt == nil || candidate.Before(*root.AbsoluteExpiresAt) {
			root.AbsoluteExpiresAt = &candidate
		}
	}
	allowed := oauth2.SplitScope(root.Scope)
	for _, scope := range req.GetGrantedScopes() {
		if !slices.Contains(allowed, scope) {
			return fosite.ErrInvalidScope
		}
	}
	requested := op.requestedScope
	for _, scope := range oauth2.SplitScope(requested) {
		if !slices.Contains(allowed, scope) {
			return fosite.ErrInvalidScope
		}
	}
	consumedAt := op.at
	reuseUntil := consumedAt.Add(s.refresh.Policy.ReuseInterval)
	inactivity := consumedAt.Add(s.refresh.Policy.InactivityLifetime)
	deadline := earliestRefreshDeadline(reuseUntil, op.accessExpiresAt, inactivity)
	if root.AbsoluteExpiresAt != nil {
		deadline = earliestRefreshDeadline(deadline, *root.AbsoluteExpiresAt)
	}
	fingerprint := refreshRequestFingerprint(ctx)
	root.PreviousSignature = &op.presentedSignature
	root.PreviousConsumedAt = &consumedAt
	root.ReuseUntil = &reuseUntil
	root.OriginalRequestedScope = &requested
	root.OriginalRequestContextFingerprint = &fingerprint
	root.RetryCiphertext = nil
	root.RetryAccessExpiresAt = &op.accessExpiresAt
	root.RetryExpiresAt = &deadline
	root.RetryCount = 0
	root.LastFreshAt = consumedAt
	root.InactivityExpiresAt = inactivity
	root.CurrentSignature = signature
	if inactivity.After(root.RetainUntil) {
		root.RetainUntil = inactivity
	}
	if s.refresh.Policy.ReuseInterval > 0 {
		response := ports.TokenResponse{AccessToken: op.accessToken, RefreshToken: op.refreshToken, TokenType: "Bearer", Scope: canonicalRequestedScope(strings.Join(req.GetGrantedScopes(), " "))}
		root.RetryCiphertext, err = sealRefreshRetry(ctx, s.refresh.Encryption, root, response)
		if err != nil {
			return err
		}
	}
	token := &storage.RefreshToken{Signature: signature, SessionID: root.ID, IssuedAt: consumedAt, ExpiresAt: inactivity}
	if err := root.Validate(); err != nil {
		return err
	}
	if err := token.Validate(); err != nil {
		return err
	}
	if err := s.refresh.Tokens.Create(ctx, token); err != nil {
		return s.mapStorageError(ctx, err)
	}
	return s.mapStorageError(ctx, s.refresh.Sessions.Save(ctx, root))
}

// DeleteRefreshTokenSession is idempotent for a bound consumed or missing token.
func (s *FositeStorage) DeleteRefreshTokenSession(ctx context.Context, signature string) error {
	op, err := s.refreshOwner(ctx)
	if err != nil {
		return err
	}
	if op.root == nil {
		return errors.New("refresh deletion requires original client binding")
	}
	token, err := s.refresh.Tokens.FindBySignature(ctx, signature)
	if ports.IsNotFoundErr(err) {
		return nil
	}
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	root, err := s.refresh.Sessions.FindByID(ctx, token.SessionID)
	if ports.IsNotFoundErr(err) {
		return nil
	}
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	if root == nil || root.ID != op.root.ID || root.ClientID != id.ClientID(op.client.GetID()) {
		return fosite.ErrNotFound
	}
	agentID, err := extractAgentID(op.client)
	if err != nil || agentID != root.AgentID {
		return fosite.ErrNotFound
	}
	if token.UsedAt != nil || root.TerminalReason != nil {
		return nil
	}
	if root.CurrentSignature != signature {
		return errors.New("unconsumed refresh token is not current")
	}
	return s.mapStorageError(ctx, s.refresh.Tokens.MarkUsed(ctx, signature, op.at))
}

// RotateRefreshToken consumes exactly the bound current token in the owner scope.
func (s *FositeStorage) RotateRefreshToken(ctx context.Context, requestID, signature string) error {
	op, err := s.refreshOwner(ctx)
	if err != nil {
		return err
	}
	if op.root == nil || op.root.ID.String() != requestID || op.presentedSignature != "" {
		return errors.New("refresh rotation requires one bound current root")
	}
	if _, err := s.currentPreparedClient(ctx, op.client); err != nil {
		return err
	}
	token, err := s.refresh.Tokens.FindBySignature(ctx, signature)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	root, err := s.refresh.Sessions.FindByID(ctx, op.root.ID)
	if err != nil {
		return s.mapStorageError(ctx, err)
	}
	agentID, err := extractAgentID(op.client)
	if err != nil || token == nil || root == nil || token.SessionID != root.ID || root.AgentID != agentID || root.ClientID != id.ClientID(op.client.GetID()) || root.TerminalReason != nil || root.CurrentSignature != signature || op.root.CurrentSignature != signature || token.UsedAt != nil {
		return fosite.ErrInvalidGrant
	}
	consumedAt, err := s.refresh.Clock.Now(ctx)
	if err != nil {
		return err
	}
	op.at = consumedAt.UTC()
	if !op.at.Before(token.ExpiresAt) || !op.at.Before(root.InactivityExpiresAt) || (root.AbsoluteExpiresAt != nil && !op.at.Before(*root.AbsoluteExpiresAt)) {
		return fosite.ErrInvalidGrant
	}
	if err := s.refresh.Tokens.MarkUsed(ctx, signature, op.at); err != nil {
		return s.mapStorageError(ctx, err)
	}
	op.presentedSignature = signature
	return nil
}

// RevokeAccessToken is a no-op for stateless JWT tokens.
func (s *FositeStorage) RevokeAccessToken(_ context.Context, _ string) error {
	return nil
}

// RevokeRefreshToken revokes the original code request's native root. A used
// authorization code can be replayed by another authenticated client; unlike
// refresh-token reuse, code replay does not bind that client to the root.
func (s *FositeStorage) RevokeRefreshToken(ctx context.Context, requestID string) error {
	op, err := s.refreshOwner(ctx)
	if err != nil {
		return err
	}
	rootID, err := id.ParseRefreshSessionID(requestID)
	if err != nil || rootID.IsZero() {
		return fosite.ErrNotFound
	}
	reason := storage.RefreshReasonProhibitedReuse
	if op.root == nil {
		if op.code == nil || op.code.ID.String() != requestID || op.code.UsedAt == nil {
			return errors.New("code replay requires the original used authorization code")
		}
		reason = storage.RefreshReasonCodeReplay
	} else {
		agentID, err := extractAgentID(op.client)
		if err != nil || op.root.ID != rootID || op.root.ClientID != id.ClientID(op.client.GetID()) || op.root.AgentID != agentID {
			return fosite.ErrNotFound
		}
	}
	return s.mapStorageError(ctx, s.refresh.Revocations.RevokeByID(ctx, rootID, op.at, reason))
}

// CreatePKCERequestSession stores the PKCE code challenge in a dedicated store keyed by code signature.
func (s *FositeStorage) CreatePKCERequestSession(ctx context.Context, signature string, req fosite.Requester) error {
	at, err := s.refreshTime(ctx)
	if err != nil {
		return err
	}
	session := &storage.PKCESession{
		Signature:           signature,
		CodeChallenge:       req.GetRequestForm().Get("code_challenge"),
		CodeChallengeMethod: req.GetRequestForm().Get("code_challenge_method"),
		ExpiresAt:           req.GetSession().GetExpiresAt(fosite.AuthorizeCode),
		CreatedAt:           at,
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

// GetClient resolves public metadata before the guard, then re-reads registered
// agent and credential state inside the borrowed authorization operation.
func (s *FositeStorage) GetClient(ctx context.Context, clientID string) (fosite.Client, error) {
	if ctx.Value(refreshOperationKey{}) != nil {
		op, ok := refreshOperationFromContext(ctx)
		if !ok {
			return nil, errors.New("invalid borrowed authorization operation")
		}
		if op.client.GetID() != clientID {
			return nil, fosite.ErrNotFound
		}
		return s.currentPreparedClient(ctx, op.client)
	}
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

func (s *FositeStorage) refreshTime(ctx context.Context) (time.Time, error) {
	if ctx.Value(refreshOperationKey{}) != nil {
		op, ok := refreshOperationFromContext(ctx)
		if !ok {
			return time.Time{}, errors.New("invalid borrowed authorization operation")
		}
		return op.at, nil
	}
	if s.refresh.Clock == nil {
		return time.Time{}, errors.New("authorization clock is required")
	}
	at, err := s.refresh.Clock.Now(ctx)
	if err != nil {
		return time.Time{}, fmt.Errorf("authorization clock: %w", err)
	}
	if at.IsZero() {
		return time.Time{}, errors.New("authorization clock returned zero time")
	}
	return at.UTC(), nil
}

func (s *FositeStorage) currentPreparedClient(ctx context.Context, prepared fosite.Client) (fosite.Client, error) {
	if s.refresh.Agents == nil || s.credRepo == nil {
		return nil, errors.New("registered agent and credentials are required in the authorization scope")
	}
	agentID, err := extractAgentID(prepared)
	if err != nil {
		return nil, err
	}
	agent, err := s.refresh.Agents.Get(ctx, agentID)
	if err != nil {
		return nil, s.mapStorageError(ctx, err)
	}
	if agent == nil || agent.ID != agentID {
		return nil, errors.New("registered client agent no longer matches authenticated client")
	}
	switch client := prepared.(type) {
	case *confidentialClient:
		if client.credential == nil || agent.ClientType() != storage.LocalClient {
			return nil, fosite.ErrNotFound
		}
		current, err := s.credRepo.GetByAgentID(ctx, agentID)
		if err != nil {
			return nil, s.mapStorageError(ctx, err)
		}
		if current == nil || current.ID != client.credential.ID || current.SecretHash != client.credential.SecretHash {
			return nil, fosite.ErrNotFound
		}
		return &confidentialClient{clientID: client.clientID, agent: agent, credential: current}, nil
	case *publicClient:
		if agent.ClientType() == storage.CIMDClient {
			if !slices.Contains(agent.ClientURIs, client.clientID) {
				return nil, fosite.ErrNotFound
			}
			return &publicClient{clientID: client.clientID, agent: agent, redirectURIs: client.redirectURIs, grantTypes: client.grantTypes}, nil
		}
		if agent.ClientType() != storage.LocalClient || client.clientID != agent.ID.String() {
			return nil, fosite.ErrNotFound
		}
		if _, err := s.credRepo.GetByAgentID(ctx, agentID); err == nil {
			return nil, fosite.ErrNotFound
		} else if !ports.IsNotFoundErr(err) {
			return nil, s.mapStorageError(ctx, err)
		}
		return &publicClient{clientID: client.clientID, agent: agent, redirectURIs: agent.RedirectURIs, grantTypes: client.grantTypes}, nil
	default:
		return nil, errors.New("prepared client is not a registered broker client")
	}
}

func (s *FositeStorage) codeClient(ctx context.Context, code *storage.AuthorizationCode) (fosite.Client, error) {
	if code.ClientID.IsZero() || code.AgentID.IsZero() {
		return nil, errors.New("authorization code lacks original client identity")
	}
	if op, ok := refreshOperationFromContext(ctx); ok && op.client.GetID() != code.ClientID.String() {
		if code.UsedAt == nil {
			return nil, fosite.ErrNotFound
		}
		// A different authenticated client may replay a consumed code. Fosite
		// needs the original request ID to revoke its root, not a remote CIMD
		// fetch or authentication of the victim's original credential.
		if s.refresh.Agents == nil {
			return nil, errors.New("registered agent is required for code replay")
		}
		agent, err := s.refresh.Agents.Get(ctx, code.AgentID)
		if err != nil {
			return nil, s.mapStorageError(ctx, err)
		}
		if agent == nil || agent.ID != code.AgentID {
			return nil, errors.New("authorization code agent is unavailable")
		}
		return &publicClient{clientID: code.ClientID.String(), agent: agent, grantTypes: fosite.Arguments{"authorization_code"}}, nil
	}
	return s.GetClient(ctx, code.ClientID.String())
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
