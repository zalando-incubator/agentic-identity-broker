package memory

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

type memoryScopeKey struct{}
type borrowedScopeKey struct{}

type receiptKey struct {
	session id.RefreshSessionID
	reason  storage.RefreshRevocationReason
}

// A scope holds only rows that the operation has changed. A nil value is a
// deletion; an absent key reads through to the committed repository.
type memoryScope struct {
	store        *RefreshSessionStore
	agentID      id.AgentID
	decisionTime time.Time
	latestTime   time.Time
	versions     map[id.AgentID]uint64
	modified     map[id.AgentID]bool
	failed       error
	closed       bool
	roots        map[id.RefreshSessionID]*storage.RefreshSession
	tokens       map[string]*storage.RefreshToken
	agents       map[id.AgentID]*storage.Agent
	grants       map[id.GrantID]*storage.UserGrant
	credentials  map[id.AgentID]*storage.ClientCredential
	codes        map[id.AuthorizationCodeID]*storage.AuthorizationCode
	pkce         map[string]*storage.PKCESession
	pkceOwners   map[string]id.AgentID
	legacy       map[string]*legacyRefreshToken
	receipts     map[receiptKey]*storage.RefreshRevocationReceipt
}

var (
	_ ports.StorageTransactionManager       = (*RefreshSessionStore)(nil)
	_ ports.AuthorizationSessionCoordinator = (*RefreshSessionStore)(nil)
	_ ports.AuthorizationClock              = (*RefreshSessionStore)(nil)
)

func scopeFor(ctx context.Context, store *RefreshSessionStore) *memoryScope {
	if store == nil || ctx == nil {
		return nil
	}
	scope, _ := ctx.Value(memoryScopeKey{}).(*memoryScope)
	if scope != nil && scope.store == store {
		return scope
	}
	return nil
}

func (s *RefreshSessionStore) lockRead() func() {
	if s == nil {
		return func() {}
	}
	s.mu.RLock()
	return s.mu.RUnlock
}

func (s *RefreshSessionStore) lockWrite() func() {
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	return s.mu.Unlock
}

func (s *RefreshSessionStore) Now(ctx context.Context) (time.Time, error) {
	if err := ctx.Err(); err != nil {
		return time.Time{}, memoryRefreshContextError("AuthorizationClock.Now", err)
	}
	if scope := scopeFor(ctx, s); scope != nil {
		if scope.closed {
			return time.Time{}, memoryRefreshValidation("AuthorizationClock.Now", errors.New("authorization transaction is closed"))
		}
		now := time.Now().UTC()
		scope.latestTime = now
		return now, nil
	}
	return time.Now().UTC(), nil
}

func (s *RefreshSessionStore) agentGate(agentID id.AgentID) chan struct{} {
	s.gatesMu.Lock()
	defer s.gatesMu.Unlock()
	gate := s.gates[agentID]
	if gate == nil {
		gate = make(chan struct{}, 1)
		gate <- struct{}{}
		s.gates[agentID] = gate
	}
	return gate
}

func (s *RefreshSessionStore) lockAgent(ctx context.Context, agentID id.AgentID) (chan struct{}, error) {
	gate := s.agentGate(agentID)
	select {
	case <-ctx.Done():
		return nil, memoryRefreshContextError("AuthorizationSession.Run", ctx.Err())
	case <-gate:
		if err := ctx.Err(); err != nil {
			gate <- struct{}{}
			return nil, memoryRefreshContextError("AuthorizationSession.Run", err)
		}
		return gate, nil
	}
}

func (s *RefreshSessionStore) Run(ctx context.Context, agentID id.AgentID, operation func(context.Context, time.Time) error) error {
	if agentID.IsZero() || operation == nil {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("authorization scope requires agent and operation"))
	}
	if scopeFor(ctx, s) != nil {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("authorization coordinator is non-reentrant"))
	}
	gate, err := s.lockAgent(ctx, agentID)
	if err != nil {
		return err
	}
	defer func() { gate <- struct{}{} }()
	s.mu.RLock()
	_, exists := s.agents.agents[agentID]
	s.mu.RUnlock()
	if !exists {
		return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindNotFound, ports.ErrNotFound, "agent not found")
	}
	at, err := s.Now(ctx)
	if err != nil {
		return err
	}
	scopeCtx, err := s.begin(ctx, agentID)
	if err != nil {
		return err
	}
	scope := scopeFor(scopeCtx, s)
	scope.decisionTime, scope.latestTime = at, at
	if err = operation(scopeCtx, at); err != nil {
		_ = s.Rollback(scopeCtx)
		return err
	}
	return s.Commit(scopeCtx)
}

func (s *RefreshSessionStore) begin(ctx context.Context, agentID id.AgentID) (context.Context, error) {
	if err := ctx.Err(); err != nil {
		return nil, memoryRefreshContextError("AuthorizationSession.Run", err)
	}
	scope := &memoryScope{store: s, agentID: agentID, versions: make(map[id.AgentID]uint64), modified: make(map[id.AgentID]bool)}
	if !agentID.IsZero() {
		s.mu.RLock()
		scope.observe(agentID)
		s.mu.RUnlock()
	}
	return context.WithValue(ctx, memoryScopeKey{}, scope), nil
}

func (s *RefreshSessionStore) BeginTX(ctx context.Context) (context.Context, error) {
	if scope := scopeFor(ctx, s); scope != nil {
		if scope.closed {
			return nil, memoryRefreshValidation("AuthorizationSession.Run", errors.New("authorization transaction is closed"))
		}
		// Fosite borrows the same staged records, not a second scope.
		return context.WithValue(ctx, borrowedScopeKey{}, true), nil
	}
	return s.begin(ctx, id.AgentID{})
}

func (s *RefreshSessionStore) Commit(ctx context.Context) error {
	scope := scopeFor(ctx, s)
	if scope == nil {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("missing memory transaction"))
	}
	if ctx.Value(borrowedScopeKey{}) == true {
		return nil
	}
	if scope.closed {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("authorization transaction is closed"))
	}
	scope.closed = true
	if scope.failed != nil {
		return memoryRefreshValidation("AuthorizationSession.Run", scope.failed)
	}
	if err := ctx.Err(); err != nil {
		return memoryRefreshContextError("AuthorizationSession.Run", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for agentID, version := range scope.versions {
		if s.versions[agentID] != version {
			return storage.NewStorageError("AuthorizationSession.Run", storage.ErrorKindConflict,
				fmt.Errorf("authorization for agent %s changed during transaction", agentID), "authorization changed during transaction")
		}
	}
	if err := s.validateScope(scope); err != nil {
		return memoryRefreshValidation("AuthorizationSession.Run", err)
	}
	s.publish(scope)
	for agentID := range scope.modified {
		s.versions[agentID]++
	}
	return nil
}

func (s *RefreshSessionStore) Rollback(ctx context.Context) error {
	scope := scopeFor(ctx, s)
	if scope == nil {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("missing memory transaction"))
	}
	if ctx.Value(borrowedScopeKey{}) == true {
		scope.failed = errors.New("borrowed authorization transaction rolled back")
		return nil
	}
	if scope.closed {
		return memoryRefreshValidation("AuthorizationSession.Run", errors.New("authorization transaction is closed"))
	}
	scope.closed = true
	return nil
}

func (scope *memoryScope) checkAgent(agentID id.AgentID) error {
	if scope.closed {
		return memoryRefreshValidation("AuthorizationSession", errors.New("authorization transaction is closed"))
	}
	if agentID.IsZero() || (!scope.agentID.IsZero() && scope.agentID != agentID) {
		return memoryRefreshValidation("AuthorizationSession", errors.New("record belongs to another agent"))
	}
	return nil
}

// observe is called while reading committed records under the backend lock.
func (scope *memoryScope) observe(agentID id.AgentID) {
	if _, ok := scope.versions[agentID]; !ok {
		scope.versions[agentID] = scope.store.versions[agentID]
	}
}

func (scope *memoryScope) touch(agentID id.AgentID) {
	scope.observe(agentID)
	scope.modified[agentID] = true
}

func (scope *memoryScope) root(sessionID id.RefreshSessionID) *storage.RefreshSession {
	root, touched := scope.roots[sessionID]
	if !touched {
		root = scope.store.roots[sessionID]
	}
	if root != nil {
		scope.observe(root.AgentID)
	}
	return root
}

func (scope *memoryScope) token(signature string) *storage.RefreshToken {
	token, touched := scope.tokens[signature]
	if !touched {
		token = scope.store.tokens[signature]
	}
	if token != nil {
		scope.root(token.SessionID)
	}
	return token
}

func (scope *memoryScope) agent(agentID id.AgentID) *storage.Agent {
	agent, touched := scope.agents[agentID]
	if !touched {
		agent = scope.store.agents.agents[agentID]
	}
	if agent != nil {
		scope.observe(agentID)
	}
	return agent
}

func (scope *memoryScope) grant(grantID id.GrantID) *storage.UserGrant {
	grant, touched := scope.grants[grantID]
	if !touched {
		grant = scope.store.grants.grants[grantID]
	}
	if grant != nil {
		scope.observe(grant.AgentID)
	}
	return grant
}

func (scope *memoryScope) credential(agentID id.AgentID) *storage.ClientCredential {
	cred, touched := scope.credentials[agentID]
	if !touched {
		cred = scope.store.credentials.byAgentID[agentID]
	}
	if cred != nil {
		scope.observe(agentID)
	}
	return cred
}

func (scope *memoryScope) code(codeID id.AuthorizationCodeID) *storage.AuthorizationCode {
	code, touched := scope.codes[codeID]
	if !touched {
		code = scope.store.codes.byID[codeID]
	}
	if code != nil {
		scope.observe(code.AgentID)
	}
	return code
}

func (scope *memoryScope) pkceSession(signature string) *storage.PKCESession {
	session, touched := scope.pkce[signature]
	if !touched {
		session = scope.store.pkce.data[signature]
	}
	if session != nil {
		if owner := scope.pkceOwner(signature); !owner.IsZero() {
			scope.observe(owner)
		}
	}
	return session
}

func (scope *memoryScope) legacySession(signature string) *legacyRefreshToken {
	session, touched := scope.legacy[signature]
	if !touched {
		session = scope.store.legacy[signature]
	}
	if session != nil {
		scope.observe(session.AgentID)
	}
	return session
}

func (scope *memoryScope) eachRoot(visit func(*storage.RefreshSession)) {
	for key, committed := range scope.store.roots {
		if staged, touched := scope.roots[key]; touched {
			if staged != nil {
				visit(staged)
			}
		} else {
			visit(committed)
		}
	}
	for key, root := range scope.roots {
		if _, committed := scope.store.roots[key]; !committed && root != nil {
			visit(root)
		}
	}
}

func (scope *memoryScope) eachToken(visit func(*storage.RefreshToken)) {
	for key, committed := range scope.store.tokens {
		if staged, touched := scope.tokens[key]; touched {
			if staged != nil {
				visit(staged)
			}
		} else {
			visit(committed)
		}
	}
	for key, token := range scope.tokens {
		if _, committed := scope.store.tokens[key]; !committed && token != nil {
			visit(token)
		}
	}
}

func (scope *memoryScope) eachTokenForRoot(sessionID id.RefreshSessionID, visit func(*storage.RefreshToken)) {
	committed := scope.store.tokensByRoot[sessionID]
	for signature := range committed {
		if token, changed := scope.tokens[signature]; changed {
			if token != nil {
				visit(token)
			}
		} else {
			visit(scope.store.tokens[signature])
		}
	}
	for signature, token := range scope.tokens {
		if _, exists := committed[signature]; !exists && token != nil && token.SessionID == sessionID {
			visit(token)
		}
	}
}

func (scope *memoryScope) eachLegacy(visit func(*legacyRefreshToken)) {
	for key, committed := range scope.store.legacy {
		if staged, touched := scope.legacy[key]; touched {
			if staged != nil {
				visit(staged)
			}
		} else {
			visit(committed)
		}
	}
	for key, session := range scope.legacy {
		if _, committed := scope.store.legacy[key]; !committed && session != nil {
			visit(session)
		}
	}
}

func (scope *memoryScope) eachGrant(visit func(*storage.UserGrant)) {
	for key, committed := range scope.store.grants.grants {
		if staged, touched := scope.grants[key]; touched {
			if staged != nil {
				visit(staged)
			}
		} else {
			visit(committed)
		}
	}
	for key, grant := range scope.grants {
		if _, committed := scope.store.grants.grants[key]; !committed && grant != nil {
			visit(grant)
		}
	}
}

func (s *RefreshSessionStore) validateScope(scope *memoryScope) error {
	if err := s.validateAgents(scope); err != nil {
		return err
	}
	if err := s.validateAuthorizationRecords(scope); err != nil {
		return err
	}
	if err := s.validateRefreshRoots(scope); err != nil {
		return err
	}
	return s.validateRefreshTokens(scope)
}

func (s *RefreshSessionStore) validateAgents(scope *memoryScope) error {
	if len(scope.agents) == 0 {
		return nil
	}
	seenCanonical := make(map[string]id.AgentID, len(scope.agents))
	seenURI := make(map[string]id.AgentID)
	for agentID, agent := range scope.agents {
		if agent == nil {
			continue
		}
		if agent.CanonicalID != nil {
			canonical := *agent.CanonicalID
			if owner, found := seenCanonical[canonical]; found && owner != agentID {
				return errors.New("agent canonical ID conflicts within transaction")
			}
			seenCanonical[canonical] = agentID
			if owner, found := s.agents.byCanonicalID[canonical]; found && owner != agentID {
				other := scope.agent(owner)
				if other != nil && other.CanonicalID != nil && *other.CanonicalID == canonical {
					return errors.New("agent canonical ID conflicts with another agent")
				}
			}
		}
		for _, uri := range agent.ClientURIs {
			if owner, found := seenURI[uri]; found && owner != agentID {
				return errors.New("agent client URI conflicts within transaction")
			}
			seenURI[uri] = agentID
			if owner, found := s.agents.byClientURI[uri]; found && owner != agentID {
				other := scope.agent(owner)
				if other != nil {
					for _, registered := range other.ClientURIs {
						if registered == uri {
							return errors.New("agent client URI conflicts with another agent")
						}
					}
				}
			}
		}
	}
	return nil
}

func (s *RefreshSessionStore) validateAuthorizationRecords(scope *memoryScope) error {
	seenCredentials := make(map[id.CredentialID]id.AgentID, len(scope.credentials))
	for grantID, grant := range scope.grants {
		if grant == nil {
			continue
		}
		if grant.ID != grantID || scope.agent(grant.AgentID) == nil {
			return errors.New("grant requires an existing owner and unchanged ID")
		}
		if prior := s.grants.grants[grantID]; prior != nil && (prior.AgentID != grant.AgentID || prior.Principal != grant.Principal) {
			return errors.New("grant ID belongs to another delegation")
		}
	}
	for agentID, credential := range scope.credentials {
		if credential == nil {
			continue
		}
		if err := credential.Validate(); err != nil {
			return err
		}
		if credential.AgentID != agentID || scope.agent(agentID) == nil {
			return errors.New("credential requires its registered agent")
		}
		if owner, found := seenCredentials[credential.ID]; found && owner != agentID {
			return errors.New("credential ID conflicts within transaction")
		}
		seenCredentials[credential.ID] = agentID
		if prior := s.credentials.byID[credential.ID]; prior != nil && prior.AgentID != agentID {
			other := scope.credential(prior.AgentID)
			if other != nil && other.ID == credential.ID {
				return errors.New("credential ID belongs to another agent")
			}
		}
	}
	seenCodes := make(map[string]id.AuthorizationCodeID, len(scope.codes))
	for codeID, code := range scope.codes {
		if code == nil {
			continue
		}
		if err := code.Validate(); err != nil {
			return err
		}
		if scope.agent(code.AgentID) == nil {
			return errors.New("authorization code owner agent does not exist")
		}
		if previous := s.codes.byID[codeID]; previous != nil && (previous.AgentID != code.AgentID || previous.CodeHash != code.CodeHash) {
			return errors.New("authorization code ID belongs to another issuance")
		}
		if previous, found := seenCodes[code.CodeHash]; found && previous != codeID {
			return errors.New("authorization code hash conflicts within transaction")
		}
		seenCodes[code.CodeHash] = codeID
		if prior := s.codes.byCodeHash[code.CodeHash]; prior != nil && prior.ID != codeID {
			other := scope.code(prior.ID)
			if other != nil && other.CodeHash == code.CodeHash {
				return errors.New("authorization code hash conflicts with another record")
			}
		}
	}
	for signature, legacy := range scope.legacy {
		if previous := s.legacy[signature]; previous != nil && legacy != nil && previous.AgentID != legacy.AgentID {
			return errors.New("legacy refresh signature belongs to another agent")
		}
	}
	for signature, session := range scope.pkce {
		if session != nil && s.pkce.data[signature] != nil && scope.pkceOwners[signature] != s.pkce.owners[signature] {
			return errors.New("PKCE signature belongs to another agent")
		}
	}
	return nil
}

func (s *RefreshSessionStore) validateRefreshRoots(scope *memoryScope) error {
	if len(scope.roots) == 0 && len(scope.tokens) == 0 {
		return nil
	}
	affected := make(map[id.RefreshSessionID]*storage.RefreshSession, len(scope.roots))
	for sessionID, root := range scope.roots {
		if root != nil {
			affected[sessionID] = root
		}
	}
	for _, token := range scope.tokens {
		if token != nil && affected[token.SessionID] == nil {
			affected[token.SessionID] = scope.root(token.SessionID)
		}
	}
	for _, root := range affected {
		if root == nil {
			continue
		}
		if err := s.validateRefreshRoot(scope, root); err != nil {
			return err
		}
	}
	return nil
}

func (s *RefreshSessionStore) validateRefreshRoot(scope *memoryScope, root *storage.RefreshSession) error {
	if err := root.Validate(); err != nil {
		return err
	}
	if root.TerminalReason == nil && s.lineageInvalid[root.ID] {
		return errors.New("broken refresh lineage cannot regain authority")
	}
	if original := s.roots[root.ID]; original != nil {
		if err := root.ValidateTransition(original, time.Now().UTC()); err != nil {
			return err
		}
	}
	if scope.agent(root.AgentID) == nil {
		return errors.New("refresh session agent does not exist")
	}
	if original := s.roots[root.ID]; original == nil {
		grant := scope.grant(root.OriginalGrantID)
		if grant == nil || grant.AgentID != root.AgentID || grant.Principal != root.Principal ||
			root.StartedAt.After(scope.latestTime) || (grant.ValidUntil != nil && !grant.ValidUntil.After(scope.latestTime)) {
			return errors.New("refresh origin requires an active matching grant")
		}
	}
	first := scope.token(root.OriginalTokenSignature)
	current := scope.token(root.CurrentSignature)
	if first == nil || first.SessionID != root.ID || !first.IssuedAt.Equal(root.StartedAt) || current == nil || current.SessionID != root.ID || (root.TerminalReason == nil && current.UsedAt != nil) {
		return errors.New("refresh root requires matching original and unused current token")
	}
	mirror := scope.legacySession(root.CurrentSignature)
	if mirror == nil || mirror.Signature != current.Signature || mirror.RequestID != root.ID.String() ||
		mirror.AgentID != root.AgentID || mirror.ClientID != root.ClientID || mirror.Principal != root.Principal ||
		mirror.Scope != root.Scope || (mirror.Email == nil) != (root.Email == nil) ||
		(mirror.Email != nil && *mirror.Email != *root.Email) || mirror.DisplayName != root.DisplayName ||
		!mirror.CreatedAt.Equal(current.IssuedAt) || !mirror.ExpiresAt.Equal(current.ExpiresAt) ||
		(root.TerminalReason == nil && mirror.UsedAt != nil) {
		return errors.New("refresh current token requires an unused matching legacy mirror")
	}
	if root.PreviousSignature != nil {
		previous := scope.token(*root.PreviousSignature)
		if previous == nil || previous.SessionID != root.ID || previous.UsedAt == nil || !previous.UsedAt.Equal(*root.PreviousConsumedAt) {
			return errors.New("refresh predecessor consumption does not match history")
		}
	}
	if root.TerminalReason == nil && !scope.completeRefreshLineage(root) {
		return errors.New("refresh root lacks complete anchored token and mirror ancestry")
	}
	var tokenError error
	var unused int
	scope.eachTokenForRoot(root.ID, func(token *storage.RefreshToken) {
		if token == nil {
			tokenError = errors.New("refresh root has missing native token history")
			return
		}
		if token.ExpiresAt.After(root.RetainUntil) {
			tokenError = errors.New("refresh retention must cover every issued token")
		}
		if token.UsedAt == nil {
			unused++
		}
	})
	if tokenError != nil {
		return tokenError
	}
	if root.TerminalReason == nil && unused != 1 {
		return errors.New("refresh root requires exactly one unused token")
	}
	return nil
}

func (s *RefreshSessionStore) validateRefreshTokens(scope *memoryScope) error {
	for _, token := range scope.tokens {
		if token == nil {
			continue
		}
		if err := token.Validate(); err != nil {
			return err
		}
		if original := s.tokens[token.Signature]; original != nil {
			if err := token.ValidateTransition(original); err != nil {
				return err
			}
		} else {
			parent := scope.root(token.SessionID)
			if parent == nil || parent.CurrentSignature != token.Signature || token.UsedAt != nil {
				return errors.New("new refresh token must be the unused current successor")
			}
		}
		if root := scope.root(token.SessionID); root == nil || token.ExpiresAt.After(root.RetainUntil) {
			return errors.New("refresh token requires retained parent root")
		}
	}
	return nil
}

func (s *RefreshSessionStore) publish(scope *memoryScope) {
	for key, receipt := range scope.receipts {
		s.receipts[key] = receipt
	}
	for key := range scope.agents {
		if old := s.agents.agents[key]; old != nil {
			if old.CanonicalID != nil {
				delete(s.agents.byCanonicalID, *old.CanonicalID)
			}
			if old.ClientID != nil {
				s.agents.removeFromClientIDIndex(*old.ClientID, key)
			}
			for _, uri := range old.ClientURIs {
				delete(s.agents.byClientURI, uri)
			}
		}
	}
	for key, agent := range scope.agents {
		if agent == nil {
			delete(s.agents.agents, key)
			continue
		}
		s.agents.agents[key] = agent
		if agent.CanonicalID != nil {
			s.agents.byCanonicalID[*agent.CanonicalID] = key
		}
		if agent.ClientID != nil {
			s.agents.byClientID[*agent.ClientID] = append(s.agents.byClientID[*agent.ClientID], key)
		}
		for _, uri := range agent.ClientURIs {
			s.agents.byClientURI[uri] = key
		}
	}
	for key, grant := range scope.grants {
		if old := s.grants.grants[key]; old != nil {
			delete(s.grants.byPrincipalAndAgent, principalAgentKey(old.Principal, old.AgentID))
			s.grants.removeGrantFromAgentIndex(old.AgentID, key)
		}
		if grant == nil {
			delete(s.grants.grants, key)
		} else {
			s.grants.grants[key] = grant
			s.grants.byPrincipalAndAgent[principalAgentKey(grant.Principal, grant.AgentID)] = key
			s.grants.grantIDsByAgent[grant.AgentID] = append(s.grants.grantIDsByAgent[grant.AgentID], key)
		}
	}
	for agentID, cred := range scope.credentials {
		if old := s.credentials.byAgentID[agentID]; old != nil {
			delete(s.credentials.byID, old.ID)
		}
		if cred == nil {
			delete(s.credentials.byAgentID, agentID)
		} else {
			s.credentials.byAgentID[agentID] = cred
			s.credentials.byID[cred.ID] = cred
		}
	}
	for key := range scope.codes {
		if old := s.codes.byID[key]; old != nil {
			delete(s.codes.byCodeHash, old.CodeHash)
		}
	}
	for key, code := range scope.codes {
		if code == nil {
			delete(s.codes.byID, key)
			continue
		}
		s.codes.byID[key] = code
		s.codes.byCodeHash[code.CodeHash] = code
	}
	for signature, session := range scope.pkce {
		if session == nil {
			delete(s.pkce.data, signature)
			delete(s.pkce.owners, signature)
		} else {
			s.pkce.data[signature] = session
			if owner := scope.pkceOwners[signature]; !owner.IsZero() {
				s.pkce.owners[signature] = owner
			}
		}
	}
	for signature, session := range scope.legacy {
		if session == nil {
			delete(s.legacy, signature)
		} else {
			s.legacy[signature] = session
		}
	}
	for key, root := range scope.roots {
		original := s.roots[key]
		if root == nil || root.TerminalReason != nil {
			if original != nil && original.TerminalReason == nil {
				delete(s.activeRootsByAgent[original.AgentID], key)
				if len(s.activeRootsByAgent[original.AgentID]) == 0 {
					delete(s.activeRootsByAgent, original.AgentID)
				}
			}
			if root == nil {
				delete(s.roots, key)
			} else {
				s.roots[key] = root
			}
			continue
		}
		s.roots[key] = root
		if original != nil && original.TerminalReason == nil {
			continue
		}
		if s.activeRootsByAgent[root.AgentID] == nil {
			s.activeRootsByAgent[root.AgentID] = make(map[id.RefreshSessionID]struct{})
		}
		s.activeRootsByAgent[root.AgentID][key] = struct{}{}
	}
	for signature, token := range scope.tokens {
		if previous := s.tokens[signature]; previous != nil && token == nil {
			delete(s.tokensByRoot[previous.SessionID], signature)
			if len(s.tokensByRoot[previous.SessionID]) == 0 {
				delete(s.tokensByRoot, previous.SessionID)
			}
		}
		if token == nil {
			delete(s.tokens, signature)
			continue
		}
		if s.tokensByRoot[token.SessionID] == nil {
			s.tokensByRoot[token.SessionID] = make(map[string]struct{})
		}
		s.tokensByRoot[token.SessionID][signature] = struct{}{}
		s.tokens[signature] = token
	}
}
