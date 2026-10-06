package memory

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var (
	_ ports.RefreshSessionRepository            = (*RefreshSessionStore)(nil)
	_ ports.RefreshSessionRevocationRepository  = (*RefreshSessionStore)(nil)
	_ ports.RefreshSessionMaintenanceRepository = (*RefreshSessionStore)(nil)
	_ ports.RefreshTokenRepository              = (*RefreshTokenStore)(nil)
)

// RefreshSessionStore owns the memory backend's authorization transaction,
// shared clock, gates and committed refresh records. Participant repositories
// remain the owners of their own records and indexes.
type RefreshSessionStore struct {
	mu                 sync.RWMutex
	gatesMu            sync.Mutex
	gates              map[id.AgentID]chan struct{}
	versions           map[id.AgentID]uint64
	roots              map[id.RefreshSessionID]*storage.RefreshSession
	lineageInvalid     map[id.RefreshSessionID]bool
	activeRootsByAgent map[id.AgentID]map[id.RefreshSessionID]struct{}
	tokens             map[string]*storage.RefreshToken
	tokensByRoot       map[id.RefreshSessionID]map[string]struct{}
	receipts           map[receiptKey]*storage.RefreshRevocationReceipt
	agents             *AgentRepository
	grants             *UserGrantRepository
	credentials        *ClientCredentialStore
	codes              *AuthorizationCodeStore
	pkce               *PKCESessionStore
	legacy             map[string]*legacyRefreshToken
}

type RefreshTokenStore struct{ refresh *RefreshSessionStore }

func NewRefreshSessionStore(agents *AgentRepository, grants *UserGrantRepository, credentials *ClientCredentialStore, codes *AuthorizationCodeStore, pkce *PKCESessionStore) *RefreshSessionStore {
	s := &RefreshSessionStore{
		gates: make(map[id.AgentID]chan struct{}), versions: make(map[id.AgentID]uint64),
		roots: make(map[id.RefreshSessionID]*storage.RefreshSession), tokens: make(map[string]*storage.RefreshToken),
		lineageInvalid:     make(map[id.RefreshSessionID]bool),
		activeRootsByAgent: make(map[id.AgentID]map[id.RefreshSessionID]struct{}),
		tokensByRoot:       make(map[id.RefreshSessionID]map[string]struct{}),
		receipts:           make(map[receiptKey]*storage.RefreshRevocationReceipt),
		legacy:             make(map[string]*legacyRefreshToken),
		agents:             agents, grants: grants, credentials: credentials, codes: codes, pkce: pkce,
	}
	agents.refresh = s
	grants.refresh = s
	credentials.refresh = s
	codes.refresh = s
	pkce.refresh = s
	return s
}

func NewRefreshTokenStore(refresh *RefreshSessionStore) *RefreshTokenStore {
	return &RefreshTokenStore{refresh: refresh}
}

func copyRefreshSession(root *storage.RefreshSession) *storage.RefreshSession {
	if root == nil {
		return nil
	}
	copy := *root
	if root.Email != nil {
		value := *root.Email
		copy.Email = &value
	}
	if root.AbsoluteExpiresAt != nil {
		value := *root.AbsoluteExpiresAt
		copy.AbsoluteExpiresAt = &value
	}
	if root.PreviousSignature != nil {
		value := *root.PreviousSignature
		copy.PreviousSignature = &value
	}
	if root.PreviousConsumedAt != nil {
		value := *root.PreviousConsumedAt
		copy.PreviousConsumedAt = &value
	}
	if root.ReuseUntil != nil {
		value := *root.ReuseUntil
		copy.ReuseUntil = &value
	}
	if root.OriginalRequestedScope != nil {
		value := *root.OriginalRequestedScope
		copy.OriginalRequestedScope = &value
	}
	if root.OriginalRequestContextFingerprint != nil {
		value := *root.OriginalRequestContextFingerprint
		copy.OriginalRequestContextFingerprint = &value
	}
	copy.RetryCiphertext = append([]byte(nil), root.RetryCiphertext...)
	if root.RetryAccessExpiresAt != nil {
		value := *root.RetryAccessExpiresAt
		copy.RetryAccessExpiresAt = &value
	}
	if root.RetryExpiresAt != nil {
		value := *root.RetryExpiresAt
		copy.RetryExpiresAt = &value
	}
	if root.RevokedAt != nil {
		value := *root.RevokedAt
		copy.RevokedAt = &value
	}
	if root.ExpiredAt != nil {
		value := *root.ExpiredAt
		copy.ExpiredAt = &value
	}
	if root.TerminalReason != nil {
		value := *root.TerminalReason
		copy.TerminalReason = &value
	}
	return &copy
}

func copyRefreshToken(token *storage.RefreshToken) *storage.RefreshToken {
	if token == nil {
		return nil
	}
	copy := *token
	if token.UsedAt != nil {
		used := *token.UsedAt
		copy.UsedAt = &used
	}
	return &copy
}

// legacyRefreshToken is a backend-private row of the old refresh table. It is
// kept for mixed-version invalidation, never as a source of session authority.
type legacyRefreshToken struct {
	Signature            string
	RequestID            string
	AgentID              id.AgentID
	ClientID             id.ClientID
	Principal            id.Principal
	Scope                string
	Email                *string
	DisplayName          string
	ExpiresAt            time.Time
	UsedAt               *time.Time
	CreatedAt            time.Time
	PredecessorSignature *string
}

func copyLegacy(session *legacyRefreshToken) *legacyRefreshToken {
	if session == nil {
		return nil
	}
	copy := *session
	if session.Email != nil {
		value := *session.Email
		copy.Email = &value
	}
	if session.UsedAt != nil {
		value := *session.UsedAt
		copy.UsedAt = &value
	}
	if session.PredecessorSignature != nil {
		value := *session.PredecessorSignature
		copy.PredecessorSignature = &value
	}
	return &copy
}

func refreshNotFound(operation string) error {
	return storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "refresh record not found")
}

func refreshConflict(operation string) error {
	return storage.NewStorageError(operation, storage.ErrorKindConflict, errors.New("refresh record already exists"), "refresh record already exists")
}

func memoryRefreshValidation(operation string, cause error) error {
	return storage.NewStorageError(operation, storage.ErrorKindValidation, cause, "invalid refresh session transition")
}

func memoryRefreshContextError(operation string, err error) error {
	kind := storage.ErrorKindConnection
	if errors.Is(err, context.DeadlineExceeded) {
		kind = storage.ErrorKindTimeout
	}
	return storage.NewStorageError(operation, kind, err, "refresh storage operation failed")
}

func (s *RefreshSessionStore) Create(ctx context.Context, root *storage.RefreshSession) error {
	const op = "RefreshSession.Create"
	if err := root.Validate(); err != nil {
		return memoryRefreshValidation(op, err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if err := scope.checkAgent(root.AgentID); err != nil {
			return err
		}
		if scope.agentID.IsZero() || root.StartedAt.After(scope.latestTime) || s.lineageInvalid[root.ID] {
			return memoryRefreshValidation(op, errors.New("first issuance requires an active agent scope and shared time"))
		}
		if scope.root(root.ID) != nil {
			return refreshConflict(op)
		}
		if scope.roots == nil {
			scope.roots = make(map[id.RefreshSessionID]*storage.RefreshSession)
		}
		scope.touch(root.AgentID)
		scope.roots[root.ID] = copyRefreshSession(root)
		return nil
	}
	return memoryRefreshValidation(op, errors.New("initial refresh issuance requires an authorization transaction with its token"))
}

func (s *RefreshSessionStore) FindByID(ctx context.Context, sessionID id.RefreshSessionID) (*storage.RefreshSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var root *storage.RefreshSession
	if scope := scopeFor(ctx, s); scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		root = scope.root(sessionID)
		if root != nil {
			if err := scope.checkAgent(root.AgentID); err != nil {
				return nil, err
			}
		}
	} else {
		root = s.roots[sessionID]
	}
	if root == nil {
		return nil, refreshNotFound("RefreshSession.FindByID")
	}
	return copyRefreshSession(root), nil
}

func (s *RefreshSessionStore) Save(ctx context.Context, root *storage.RefreshSession) error {
	const op = "RefreshSession.Save"
	if root == nil {
		return memoryRefreshValidation(op, errors.New("missing refresh root"))
	}
	if err := root.Validate(); err != nil {
		return memoryRefreshValidation(op, err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if err := scope.checkAgent(root.AgentID); err != nil {
			return err
		}
		previous := scope.root(root.ID)
		if previous == nil {
			return refreshNotFound(op)
		}
		if err := root.ValidateTransition(previous, time.Now().UTC()); err != nil {
			return memoryRefreshValidation(op, err)
		}
		if previous.TerminalReason == nil && root.TerminalReason != nil {
			at := root.RevokedAt
			if at == nil {
				at = root.ExpiredAt
			}
			return scope.stageTerminalRoot(ctx, previous, copyRefreshSession(root), *at, op)
		}
		if scope.roots == nil {
			scope.roots = make(map[id.RefreshSessionID]*storage.RefreshSession)
		}
		scope.touch(root.AgentID)
		scope.roots[root.ID] = copyRefreshSession(root)
		return nil
	}
	previous, err := s.FindByID(ctx, root.ID)
	if err != nil {
		return err
	}
	return s.Run(ctx, previous.AgentID, func(scoped context.Context, _ time.Time) error { return s.Save(scoped, root) })
}

func (r *RefreshTokenStore) Create(ctx context.Context, token *storage.RefreshToken) error {
	const op = "RefreshToken.Create"
	if err := token.Validate(); err != nil {
		return memoryRefreshValidation(op, err)
	}
	s := r.refresh
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		root := scope.root(token.SessionID)
		if root == nil {
			return refreshNotFound("RefreshSession.FindByID")
		}
		if err := scope.checkAgent(root.AgentID); err != nil {
			return err
		}
		if scope.token(token.Signature) != nil {
			return refreshConflict(op)
		}
		if scope.legacySession(token.Signature) != nil {
			return refreshConflict(op)
		}
		if root.TerminalReason != nil || token.UsedAt != nil {
			return memoryRefreshValidation(op, errors.New("only an active, unconsumed child can be issued"))
		}
		var predecessor *string
		if token.Signature == root.OriginalTokenSignature {
			if root.CurrentSignature != token.Signature || !token.IssuedAt.Equal(root.StartedAt) ||
				!token.ExpiresAt.Equal(root.RetainUntil) || !token.ExpiresAt.Equal(root.InactivityExpiresAt) {
				return memoryRefreshValidation(op, errors.New("first token must bind the original root and issuance time"))
			}
		} else {
			previous := scope.token(root.CurrentSignature)
			mirror := scope.legacySession(root.CurrentSignature)
			if previous == nil || previous.UsedAt == nil || !previous.UsedAt.Equal(token.IssuedAt) ||
				mirror == nil || mirror.UsedAt == nil || !mirror.UsedAt.Equal(token.IssuedAt) ||
				!token.IssuedAt.After(root.LastFreshAt) {
				return memoryRefreshValidation(op, errors.New("successor requires matching anchored predecessor consumption"))
			}
			value := root.CurrentSignature
			predecessor = &value
		}
		if scope.tokens == nil {
			scope.tokens = make(map[string]*storage.RefreshToken)
		}
		scope.touch(root.AgentID)
		scope.tokens[token.Signature] = copyRefreshToken(token)
		if scope.legacy == nil {
			scope.legacy = make(map[string]*legacyRefreshToken)
		}
		scope.legacy[token.Signature] = &legacyRefreshToken{
			Signature: token.Signature, RequestID: root.ID.String(), AgentID: root.AgentID,
			ClientID: root.ClientID, Principal: root.Principal, Scope: root.Scope,
			ExpiresAt: token.ExpiresAt, UsedAt: token.UsedAt, CreatedAt: token.IssuedAt,
			Email: root.Email, DisplayName: root.DisplayName,
			PredecessorSignature: predecessor,
		}
		return nil
	}
	root, err := s.FindByID(ctx, token.SessionID)
	if err != nil {
		return err
	}
	return s.Run(ctx, root.AgentID, func(scoped context.Context, _ time.Time) error { return r.Create(scoped, token) })
}

func (r *RefreshTokenStore) FindBySignature(ctx context.Context, signature string) (*storage.RefreshToken, error) {
	s := r.refresh
	s.mu.RLock()
	defer s.mu.RUnlock()
	var token *storage.RefreshToken
	if scope := scopeFor(ctx, s); scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		token = scope.token(signature)
		if token != nil {
			root := scope.root(token.SessionID)
			if root == nil {
				return nil, refreshNotFound("RefreshSession.FindByID")
			}
			if err := scope.checkAgent(root.AgentID); err != nil {
				return nil, err
			}
		}
	} else {
		token = s.tokens[signature]
	}
	if token == nil {
		return nil, refreshNotFound("RefreshToken.FindBySignature")
	}
	return copyRefreshToken(token), nil
}

func (r *RefreshTokenStore) CheckCurrentLineage(ctx context.Context, sessionID id.RefreshSessionID) error {
	s := r.refresh
	scope := scopeFor(ctx, s)
	if scope == nil || scope.agentID.IsZero() || scope.closed {
		return memoryRefreshValidation("AuthorizationSession", errors.New("current refresh lineage requires an active agent scope"))
	}
	s.mu.RLock()
	root := scope.root(sessionID)
	if root == nil {
		s.mu.RUnlock()
		return refreshNotFound("RefreshSession.FindByID")
	}
	err := scope.checkCurrentLineage(root)
	s.mu.RUnlock()
	if err != nil && ports.IsNotFoundErr(err) && root.TerminalReason == nil {
		s.invalidateBrokenLineage(sessionID)
	}
	return err
}

func (s *RefreshSessionStore) invalidateBrokenLineage(sessionID id.RefreshSessionID) {
	s.mu.Lock()
	defer s.mu.Unlock()
	root := s.roots[sessionID]
	if root == nil || root.TerminalReason != nil {
		return
	}
	committed := &memoryScope{store: s, agentID: root.AgentID, versions: make(map[id.AgentID]uint64)}
	if root.Validate() != nil || !committed.completeRefreshLineage(root) {
		s.lineageInvalid[sessionID] = true
	}
}

// checkCurrentLineage requires the backend read lock and the agent scope.
func (scope *memoryScope) checkCurrentLineage(root *storage.RefreshSession) error {
	if err := scope.checkAgent(root.AgentID); err != nil {
		return err
	}
	if root.TerminalReason != nil || scope.store.lineageInvalid[root.ID] || root.Validate() != nil || !scope.completeRefreshLineage(root) {
		return refreshNotFound("RefreshToken.LegacyOrigin")
	}
	return nil
}

// completeRefreshLineage walks every native token and matching private mirror.
// The mirror records predecessor links; native rows alone cannot prove ancestry.
func (scope *memoryScope) completeRefreshLineage(root *storage.RefreshSession) bool {
	expected, allPresent := 0, true
	scope.eachTokenForRoot(root.ID, func(token *storage.RefreshToken) {
		if token == nil || token.SessionID != root.ID {
			allPresent = false
			return
		}
		expected++
	})
	if !allPresent || expected == 0 {
		return false
	}
	matchedMirrors := 0
	scope.eachLegacy(func(mirror *legacyRefreshToken) {
		if mirror.AgentID == root.AgentID && mirror.RequestID == root.ID.String() {
			matchedMirrors++
			if scope.token(mirror.Signature) == nil {
				allPresent = false
			}
		}
	})
	if !allPresent || matchedMirrors != expected {
		return false
	}
	signature := root.CurrentSignature
	requestID := root.ID.String()
	var successorIssuedAt time.Time
	for visited := range expected {
		token := scope.token(signature)
		mirror := scope.legacySession(signature)
		if token == nil || token.Signature != signature || token.SessionID != root.ID || token.Validate() != nil ||
			mirror == nil || mirror.Signature != signature || mirror.RequestID != requestID ||
			mirror.AgentID != root.AgentID || mirror.ClientID != root.ClientID || mirror.Principal != root.Principal ||
			mirror.Scope != root.Scope || (mirror.Email == nil) != (root.Email == nil) ||
			(mirror.Email != nil && *mirror.Email != *root.Email) || mirror.DisplayName != root.DisplayName ||
			!mirror.CreatedAt.Equal(token.IssuedAt) || !mirror.ExpiresAt.Equal(token.ExpiresAt) || token.ExpiresAt.After(root.RetainUntil) {
			return false
		}
		if signature == root.CurrentSignature {
			if token.UsedAt != nil || mirror.UsedAt != nil || !token.IssuedAt.Equal(root.LastFreshAt) ||
				(root.PreviousSignature == nil) != (mirror.PredecessorSignature == nil) ||
				(root.PreviousSignature != nil && *root.PreviousSignature != *mirror.PredecessorSignature) {
				return false
			}
		} else if token.UsedAt == nil || mirror.UsedAt == nil || !token.UsedAt.Equal(*mirror.UsedAt) ||
			!token.UsedAt.Equal(successorIssuedAt) {
			return false
		}
		if root.PreviousSignature != nil && signature == *root.PreviousSignature &&
			(token.UsedAt == nil || !token.UsedAt.Equal(*root.PreviousConsumedAt)) {
			return false
		}
		if signature == root.OriginalTokenSignature {
			return mirror.PredecessorSignature == nil && token.IssuedAt.Equal(root.StartedAt) && visited+1 == expected
		}
		if mirror.PredecessorSignature == nil || *mirror.PredecessorSignature == signature {
			return false
		}
		successorIssuedAt = token.IssuedAt
		signature = *mirror.PredecessorSignature
	}
	return false
}

func (r *RefreshTokenStore) MarkUsed(ctx context.Context, signature string, usedAt time.Time) error {
	const op = "RefreshToken.MarkUsed"
	s := r.refresh
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		token := scope.token(signature)
		if token == nil {
			return refreshNotFound("RefreshToken.MarkUsed")
		}
		root := scope.root(token.SessionID)
		if root == nil {
			return refreshNotFound("RefreshSession.FindByID")
		}
		if err := scope.checkAgent(root.AgentID); err != nil {
			return err
		}
		if scope.agentID.IsZero() || usedAt.After(scope.latestTime) || !usedAt.After(root.LastFreshAt) || usedAt.Before(token.IssuedAt) {
			return memoryRefreshValidation(op, errors.New("consumption must follow issuance and precede the final shared authorization time"))
		}
		if !usedAt.Before(token.ExpiresAt) {
			return refreshNotFound(op)
		}
		if token.UsedAt != nil {
			return refreshNotFound("RefreshToken.LegacyCurrent")
		}
		if root.TerminalReason != nil || root.CurrentSignature != signature {
			return memoryRefreshValidation("RefreshToken.MarkUsed", errors.New("only the live current token can be consumed"))
		}
		changed := copyRefreshToken(token)
		changed.UsedAt = &usedAt
		if err := changed.ValidateTransition(token); err != nil {
			return memoryRefreshValidation("RefreshToken.MarkUsed", err)
		}
		if err := scope.checkCurrentLineage(root); err != nil {
			return err
		}
		if scope.tokens == nil {
			scope.tokens = make(map[string]*storage.RefreshToken)
		}
		scope.touch(root.AgentID)
		scope.tokens[signature] = changed
		if scope.legacy == nil {
			scope.legacy = make(map[string]*legacyRefreshToken)
		}
		legacy := copyLegacy(scope.legacySession(signature))
		legacy.UsedAt = &usedAt
		scope.legacy[signature] = legacy
		return nil
	}
	current, err := r.FindBySignature(ctx, signature)
	if err != nil {
		return err
	}
	root, err := s.FindByID(ctx, current.SessionID)
	if err != nil {
		return err
	}
	return s.Run(ctx, root.AgentID, func(scoped context.Context, _ time.Time) error { return r.MarkUsed(scoped, signature, usedAt) })
}

func validMemoryRevocation(at time.Time, reason storage.RefreshRevocationReason) error {
	_, offset := at.Zone()
	if at.IsZero() || offset != 0 || at.Year() < 1 || at.Year() > 9999 {
		return errors.New("revocation requires a finite UTC timestamp")
	}
	switch reason {
	case storage.RefreshReasonGrantDeleted, storage.RefreshReasonExpiredGrantRenewal, storage.RefreshReasonAgentDeleted, storage.RefreshReasonCredentialRevoked, storage.RefreshReasonCodeReplay, storage.RefreshReasonProhibitedReuse, storage.RefreshReasonAbsoluteExpiry, storage.RefreshReasonInactivityExpiry, storage.RefreshReasonRestoreInvalidation:
		return nil
	default:
		return errors.New("invalid refresh revocation reason")
	}
}

func (s *RefreshSessionStore) RevokeByID(ctx context.Context, sessionID id.RefreshSessionID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := validMemoryRevocation(at, reason); err != nil {
		return memoryRefreshValidation("RefreshSession.Revoke", err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		root := scope.root(sessionID)
		if root == nil {
			return nil
		}
		if err := scope.checkAgent(root.AgentID); err != nil {
			return err
		}
		return scope.revokeRoot(ctx, root, at, reason)
	}
	root, err := s.FindByID(ctx, sessionID)
	if ports.IsNotFoundErr(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.Run(ctx, root.AgentID, func(scoped context.Context, _ time.Time) error { return s.RevokeByID(scoped, sessionID, at, reason) })
}

func (s *RefreshSessionStore) RevokeByPrincipalAndAgent(ctx context.Context, principal id.Principal, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := validMemoryRevocation(at, reason); err != nil {
		return memoryRefreshValidation("RefreshSession.Revoke", err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if err := scope.checkAgent(agentID); err != nil {
			return err
		}
		var matching []*storage.RefreshSession
		scope.eachRoot(func(root *storage.RefreshSession) {
			if root.AgentID == agentID && root.Principal == principal {
				matching = append(matching, root)
			}
		})
		for _, root := range matching {
			if err := scope.revokeRoot(ctx, root, at, reason); err != nil {
				return err
			}
		}
		scope.invalidateLegacy(agentID, &principal, at)
		return nil
	}
	return s.Run(ctx, agentID, func(scoped context.Context, _ time.Time) error {
		return s.RevokeByPrincipalAndAgent(scoped, principal, agentID, at, reason)
	})
}

func (s *RefreshSessionStore) RevokeByAgent(ctx context.Context, agentID id.AgentID, at time.Time, reason storage.RefreshRevocationReason) error {
	if err := validMemoryRevocation(at, reason); err != nil {
		return memoryRefreshValidation("RefreshSession.Revoke", err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		s.mu.RLock()
		defer s.mu.RUnlock()
		if err := scope.checkAgent(agentID); err != nil {
			return err
		}
		var matching []*storage.RefreshSession
		scope.eachRoot(func(root *storage.RefreshSession) {
			if root.AgentID == agentID {
				matching = append(matching, root)
			}
		})
		for _, root := range matching {
			if err := scope.revokeRoot(ctx, root, at, reason); err != nil {
				return err
			}
		}
		scope.invalidateLegacy(agentID, nil, at)
		return nil
	}
	return s.Run(ctx, agentID, func(scoped context.Context, _ time.Time) error { return s.RevokeByAgent(scoped, agentID, at, reason) })
}

func (scope *memoryScope) invalidateLegacy(agentID id.AgentID, principal *id.Principal, at time.Time) {
	scope.eachLegacy(func(legacy *legacyRefreshToken) {
		if legacy.AgentID != agentID || (principal != nil && legacy.Principal != *principal) || legacy.UsedAt != nil {
			return
		}
		changed := copyLegacy(legacy)
		changed.UsedAt = &at
		if scope.legacy == nil {
			scope.legacy = make(map[string]*legacyRefreshToken)
		}
		scope.touch(agentID)
		scope.legacy[legacy.Signature] = changed
	})
}

func (scope *memoryScope) revokeRoot(ctx context.Context, root *storage.RefreshSession, at time.Time, reason storage.RefreshRevocationReason) error {
	changed := copyRefreshSession(root)
	if changed.TerminalReason == nil {
		changed.TerminalReason = &reason
		if reason == storage.RefreshReasonAbsoluteExpiry || reason == storage.RefreshReasonInactivityExpiry {
			changed.ExpiredAt = &at
		} else {
			changed.RevokedAt = &at
		}
	}
	changed.RetryCiphertext = nil
	return scope.stageTerminalRoot(ctx, root, changed, at, "RefreshSession.Revoke")
}

func (scope *memoryScope) stageTerminalRoot(ctx context.Context, previous, changed *storage.RefreshSession, at time.Time, operation string) error {
	known := make(map[string]bool)
	scope.eachTokenForRoot(previous.ID, func(token *storage.RefreshToken) {
		if token != nil {
			known[token.Signature] = true
		}
	})
	legacyChanges := make(map[string]*legacyRefreshToken)
	scope.eachLegacy(func(legacy *legacyRefreshToken) {
		if legacy.AgentID != previous.AgentID || (legacy.RequestID != previous.ID.String() && !known[legacy.Signature]) {
			return
		}
		if legacy.ExpiresAt.After(changed.RetainUntil) {
			changed.RetainUntil = legacy.ExpiresAt
		}
		if legacy.UsedAt == nil {
			copy := copyLegacy(legacy)
			copy.UsedAt = &at
			legacyChanges[legacy.Signature] = copy
		}
	})
	if err := changed.ValidateTransition(previous, at); err != nil {
		return memoryRefreshValidation(operation, err)
	}
	if previous.TerminalReason == nil {
		if scope.receipts == nil {
			scope.receipts = make(map[receiptKey]*storage.RefreshRevocationReceipt)
		}
		reason := *changed.TerminalReason
		key := receiptKey{session: previous.ID, reason: reason}
		if _, exists := scope.store.receipts[key]; !exists {
			scope.receipts[key] = &storage.RefreshRevocationReceipt{
				SessionID: previous.ID, AgentID: previous.AgentID, Principal: previous.Principal,
				ClientID: previous.ClientID, Reason: reason, At: at,
				RedactedContext: security.RedactedAuditContext(ctx),
			}
		}
	}
	if len(legacyChanges) != 0 && scope.legacy == nil {
		scope.legacy = make(map[string]*legacyRefreshToken)
	}
	for signature, session := range legacyChanges {
		scope.legacy[signature] = session
	}
	if scope.roots == nil {
		scope.roots = make(map[id.RefreshSessionID]*storage.RefreshSession)
	}
	scope.touch(previous.AgentID)
	scope.roots[previous.ID] = changed
	return nil
}

func boundedRefreshLimit(limit int) (int, error) {
	if limit < 1 || limit > 1000 {
		return 0, memoryRefreshValidation("RefreshSession.List", errors.New("limit must be between 1 and 1000"))
	}
	return limit, nil
}

func (s *RefreshSessionStore) ListAgentIDs(ctx context.Context, afterID id.AgentID, limit int) ([]id.AgentID, error) {
	limit, err := boundedRefreshLimit(limit)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make(map[id.AgentID]bool)
	if scope := scopeFor(ctx, s); scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		scope.eachRoot(func(root *storage.RefreshSession) {
			if scope.agentID.IsZero() || scope.agentID == root.AgentID {
				ids[root.AgentID] = true
			}
		})
		scope.eachLegacy(func(legacy *legacyRefreshToken) {
			if scope.agentID.IsZero() || scope.agentID == legacy.AgentID {
				ids[legacy.AgentID] = true
			}
		})
	} else {
		for _, root := range s.roots {
			ids[root.AgentID] = true
		}
		for _, legacy := range s.legacy {
			ids[legacy.AgentID] = true
		}
	}
	var ordered []id.AgentID
	for agentID := range ids {
		if afterID.IsZero() || bytes.Compare(agentID[:], afterID[:]) > 0 {
			ordered = append(ordered, agentID)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return bytes.Compare(ordered[i][:], ordered[j][:]) < 0 })
	if len(ordered) > limit {
		ordered = ordered[:limit]
	}
	return ordered, nil
}

func (s *RefreshSessionStore) ListActive(ctx context.Context, afterID id.RefreshSessionID, limit int) ([]*storage.RefreshSession, error) {
	limit, err := boundedRefreshLimit(limit)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*storage.RefreshSession
	scope := scopeFor(ctx, s)
	collect := func(root *storage.RefreshSession) {
		if root.TerminalReason == nil && (afterID.IsZero() || root.ID.String() > afterID.String()) {
			result = append(result, root)
		}
	}
	if scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		scope.eachRoot(func(root *storage.RefreshSession) {
			if scope.agentID.IsZero() || scope.agentID == root.AgentID {
				collect(root)
			}
		})
	} else {
		for _, root := range s.roots {
			collect(root)
		}
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare(result[i].ID[:], result[j].ID[:]) < 0 })
	if len(result) > limit {
		result = result[:limit]
	}
	for i, root := range result {
		if scope != nil {
			scope.observe(root.AgentID)
		}
		result[i] = copyRefreshSession(root)
	}
	return result, nil
}

func (s *RefreshSessionStore) ListActiveByAgent(ctx context.Context, agentID id.AgentID, principal *id.Principal) ([]storage.RefreshSessionAuditIdentity, error) {
	const op = "RefreshSession.ListActiveByAgent"
	scope := scopeFor(ctx, s)
	if agentID.IsZero() || scope == nil || scope.agentID != agentID || scope.closed {
		return nil, memoryRefreshValidation("AuthorizationSession", errors.New("agent-scoped authorization transaction required"))
	}
	if err := ctx.Err(); err != nil {
		return nil, memoryRefreshContextError(op, err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]storage.RefreshSessionAuditIdentity, 0, len(s.activeRootsByAgent[agentID]))
	appendActive := func(root *storage.RefreshSession) {
		if root != nil && root.AgentID == agentID && root.TerminalReason == nil && (principal == nil || root.Principal == *principal) {
			result = append(result, root.AuditIdentity())
		}
	}
	for sessionID := range s.activeRootsByAgent[agentID] {
		appendActive(scope.root(sessionID))
	}
	for sessionID, root := range scope.roots {
		if _, exists := s.activeRootsByAgent[agentID][sessionID]; !exists {
			appendActive(root)
		}
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare(result[i].ID[:], result[j].ID[:]) < 0 })
	return result, nil
}

func (s *RefreshSessionStore) HasRemainingAuthority(ctx context.Context) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, memoryRefreshContextError("RefreshSession.HasRemainingAuthority", err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	scope := scopeFor(ctx, s)
	if scope != nil {
		if scope.closed {
			return false, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		for _, root := range scope.roots {
			if root != nil && (root.TerminalReason == nil || len(root.RetryCiphertext) != 0) {
				return true, nil
			}
		}
		for _, legacy := range scope.legacy {
			if legacy != nil && legacy.UsedAt == nil {
				return true, nil
			}
		}
	}
	for key, root := range s.roots {
		if scope != nil {
			if _, modified := scope.roots[key]; modified {
				continue
			}
		}
		if root.TerminalReason == nil || len(root.RetryCiphertext) != 0 {
			return true, nil
		}
	}
	for key, legacy := range s.legacy {
		if scope != nil {
			if _, modified := scope.legacy[key]; modified {
				continue
			}
		}
		if legacy.UsedAt == nil {
			return true, nil
		}
	}
	return false, nil
}

func (s *RefreshSessionStore) ListDue(ctx context.Context, at time.Time, limit int) ([]*storage.RefreshSession, error) {
	limit, err := boundedRefreshLimit(limit)
	if err != nil {
		return nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []*storage.RefreshSession
	scope := scopeFor(ctx, s)
	collect := func(root *storage.RefreshSession) {
		if root.TerminalReason != nil {
			return
		}
		if !root.InactivityExpiresAt.After(at) || root.AbsoluteExpiresAt != nil && !root.AbsoluteExpiresAt.After(at) || len(root.RetryCiphertext) != 0 && root.RetryExpiresAt != nil && !root.RetryExpiresAt.After(at) {
			result = append(result, root)
		}
	}
	if scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
		}
		scope.eachRoot(func(root *storage.RefreshSession) {
			if scope.agentID.IsZero() || scope.agentID == root.AgentID {
				collect(root)
			}
		})
	} else {
		for _, root := range s.roots {
			collect(root)
		}
	}
	sort.Slice(result, func(i, j int) bool { return bytes.Compare(result[i].ID[:], result[j].ID[:]) < 0 })
	if len(result) > limit {
		result = result[:limit]
	}
	for i, root := range result {
		if scope != nil {
			scope.observe(root.AgentID)
		}
		result[i] = copyRefreshSession(root)
	}
	return result, nil
}

func (s *RefreshSessionStore) DeleteTerminal(ctx context.Context, before time.Time, limit int) (int, error) {
	limit, err := boundedRefreshLimit(limit)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return 0, memoryRefreshContextError("RefreshSession.DeleteTerminal", err)
	}
	scope := scopeFor(ctx, s)
	if scope == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		scope = &memoryScope{store: s, versions: make(map[id.AgentID]uint64), modified: make(map[id.AgentID]bool)}
		count, err := s.deleteTerminal(scope, before, limit)
		if err != nil {
			return 0, err
		}
		s.publish(scope)
		return count, nil
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if scope.closed {
		return 0, memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
	}
	return s.deleteTerminal(scope, before, limit)
}

func (s *RefreshSessionStore) deleteTerminal(scope *memoryScope, before time.Time, limit int) (int, error) {
	var expired []*storage.RefreshSession
	scope.eachRoot(func(root *storage.RefreshSession) {
		if root.TerminalReason != nil && !root.RetainUntil.After(before) && (scope.agentID.IsZero() || scope.agentID == root.AgentID) {
			expired = append(expired, root)
		}
	})
	sort.Slice(expired, func(i, j int) bool { return bytes.Compare(expired[i].ID[:], expired[j].ID[:]) < 0 })
	if len(expired) > limit {
		expired = expired[:limit]
	}
	if scope.roots == nil {
		scope.roots = make(map[id.RefreshSessionID]*storage.RefreshSession)
	}
	if scope.tokens == nil {
		scope.tokens = make(map[string]*storage.RefreshToken)
	}
	if scope.legacy == nil {
		scope.legacy = make(map[string]*legacyRefreshToken)
	}
	for _, root := range expired {
		scope.touch(root.AgentID)
		scope.roots[root.ID] = nil
		scope.eachTokenForRoot(root.ID, func(token *storage.RefreshToken) { scope.tokens[token.Signature] = nil })
		scope.eachLegacy(func(legacy *legacyRefreshToken) {
			if legacy.AgentID == root.AgentID && legacy.RequestID == root.ID.String() {
				scope.legacy[legacy.Signature] = nil
			}
		})
	}
	var rootless []*legacyRefreshToken
	scope.eachLegacy(func(legacy *legacyRefreshToken) {
		if legacy.ExpiresAt.After(before) || (scope.agentID != (id.AgentID{}) && scope.agentID != legacy.AgentID) {
			return
		}
		rootID, err := id.ParseRefreshSessionID(legacy.RequestID)
		if err == nil {
			if root := scope.root(rootID); root != nil && root.AgentID == legacy.AgentID {
				return
			}
		}
		rootless = append(rootless, legacy)
	})
	sort.Slice(rootless, func(i, j int) bool {
		if rootless[i].ExpiresAt.Equal(rootless[j].ExpiresAt) {
			return rootless[i].Signature < rootless[j].Signature
		}
		return rootless[i].ExpiresAt.Before(rootless[j].ExpiresAt)
	})
	if len(rootless) > limit {
		rootless = rootless[:limit]
	}
	for _, legacy := range rootless {
		scope.touch(legacy.AgentID)
		scope.legacy[legacy.Signature] = nil
	}
	return len(expired) + len(rootless), nil
}
