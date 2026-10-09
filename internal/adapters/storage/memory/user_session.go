package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

// InMemoryUserSessionRepository is an in-memory implementation for testing/development.
type InMemoryUserSessionRepository struct {
	mu                sync.RWMutex
	issuerSessionGate *sync.RWMutex
	provider          *InMemoryThirdpartyOAuth2ProviderRepository
	sessions          map[id.SessionID]*storage.UserSession // Key: session ID
	index             map[string]*storage.UserSession       // Key: "{principal}#{serviceID}"
	locks             map[string]*sessionLock
}

type sessionLock struct {
	available chan struct{}
	users     int
}

// NewInMemoryUserSessionRepository creates a new in-memory repository.
func NewInMemoryUserSessionRepository() *InMemoryUserSessionRepository {
	return &InMemoryUserSessionRepository{
		sessions: make(map[id.SessionID]*storage.UserSession),
		index:    make(map[string]*storage.UserSession),
		locks:    make(map[string]*sessionLock),
	}
}

// Create creates a new user session with upsert semantics.
func (r *InMemoryUserSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
	if session == nil {
		return errors.New("session cannot be nil")
	}
	if err := session.Validate(); err != nil {
		return err
	}
	if r.issuerSessionGate != nil {
		r.issuerSessionGate.RLock()
		defer r.issuerSessionGate.RUnlock()
	}
	if !r.matchesCurrentDiscoveryState(session) {
		return storage.NewStorageError("Create", storage.ErrorKindConflict, nil, "provider issuer or audience is no longer current")
	}

	key := principalServiceKey(session.Principal, session.ServiceID)
	gate, err := r.lockSession(ctx, key)
	if err != nil {
		return err
	}
	defer r.unlockSession(key, gate)

	r.mu.Lock()
	defer r.mu.Unlock()
	// If session exists for this principal+service, update it
	if existing, ok := r.index[key]; ok {
		// Reuse ID
		session.ID = existing.ID
		delete(r.sessions, existing.ID)
	}

	stored := *session
	stored.ExpectedIssuerURI = ""
	stored.ExpectedResource = ""
	r.sessions[stored.ID] = &stored
	r.index[key] = &stored
	return nil
}

func (r *InMemoryUserSessionRepository) matchesCurrentDiscoveryState(session *storage.UserSession) bool {
	if r.provider == nil {
		return session.ExpectedIssuerURI == "" && session.ExpectedResource == ""
	}
	r.provider.mu.RLock()
	defer r.provider.mu.RUnlock()

	current := r.provider.providers[session.ServiceID]
	if current == nil {
		return false
	}
	if current.entity.Discovery.ResourceURL != nil {
		return session.ExpectedIssuerURI != "" && session.ExpectedResource != "" &&
			session.ExpectedIssuerURI == current.entity.IssuerURI &&
			session.ExpectedResource == current.entity.AuthorizationParams["resource"]
	}
	return session.ExpectedIssuerURI == "" && session.ExpectedResource == ""
}

// Get retrieves a session by ID.
func (r *InMemoryUserSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storage.UserSession, error) {
	if sessionID.IsZero() {
		return nil, errors.New("session ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "session not found")
	}
	return session, nil
}

// FindByPrincipalAndService retrieves the session for a principal and service.
func (r *InMemoryUserSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	key := principalServiceKey(principal, serviceID)
	session, ok := r.index[key]
	if !ok {
		return nil, nil // Not found is not an error
	}
	return session, nil
}

// WithLockedSession serializes refreshes for one session while leaving other sessions available.
func (r *InMemoryUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	key := principalServiceKey(principal, serviceID)
	gate, err := r.lockSession(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.unlockSession(key, gate)

	r.mu.RLock()
	current := r.index[key]
	if current == nil {
		r.mu.RUnlock()
		return nil, nil
	}
	session := *current
	r.mu.RUnlock()
	updated, err := refresh(ctx, &session)
	if err != nil {
		return nil, err
	}
	if updated {
		if err := session.Validate(); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.mu.Lock()
		r.index[key] = &session
		r.sessions[session.ID] = &session
		r.mu.Unlock()
	}
	return &session, nil
}

// ListByPrincipal retrieves all sessions for a principal, including expired ones.
func (r *InMemoryUserSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*storage.UserSession
	for _, session := range r.sessions {
		if session.Principal == principal {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

// ListActiveByPrincipal retrieves only non-expired sessions for a principal.
func (r *InMemoryUserSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*storage.UserSession
	for _, session := range r.sessions {
		if session.Principal == principal && !session.IsExpired() {
			sessions = append(sessions, session)
		}
	}
	return sessions, nil
}

// Delete deletes a session by ID.
func (r *InMemoryUserSessionRepository) Delete(ctx context.Context, sessionID id.SessionID) error {
	if sessionID.IsZero() {
		return errors.New("session ID cannot be empty")
	}
	for {
		r.mu.RLock()
		session := r.sessions[sessionID]
		r.mu.RUnlock()
		if session == nil {
			return nil
		}
		key := principalServiceKey(session.Principal, session.ServiceID)
		gate, err := r.lockSession(ctx, key)
		if err != nil {
			return err
		}
		r.mu.Lock()
		current := r.sessions[sessionID]
		if current != nil && principalServiceKey(current.Principal, current.ServiceID) != key {
			r.mu.Unlock()
			r.unlockSession(key, gate)
			continue
		}
		if current != nil {
			delete(r.sessions, sessionID)
			delete(r.index, key)
		}
		r.mu.Unlock()
		r.unlockSession(key, gate)
		return nil
	}
}

// DeleteByPrincipalAndService deletes the session for a principal and service.
func (r *InMemoryUserSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	if principal.IsZero() || serviceID.IsZero() {
		return errors.New("principal and serviceID required")
	}

	key := principalServiceKey(principal, serviceID)
	gate, err := r.lockSession(ctx, key)
	if err != nil {
		return err
	}
	defer r.unlockSession(key, gate)

	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.index[key]
	if ok {
		delete(r.sessions, session.ID)
		delete(r.index, key)
	}
	return nil
}

// CountByService counts sessions referencing a service.
func (r *InMemoryUserSessionRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	if serviceID.IsZero() {
		return 0, errors.New("serviceID required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, session := range r.sessions {
		if session.ServiceID == serviceID {
			count++
		}
	}
	return count, nil
}

// Helper function
func principalServiceKey(principal id.Principal, serviceID id.ServiceID) string {
	return principal.String() + "#" + serviceID.String()
}

func (r *InMemoryUserSessionRepository) lockSession(ctx context.Context, key string) (*sessionLock, error) {
	r.mu.Lock()
	gate := r.locks[key]
	if gate == nil {
		gate = &sessionLock{available: make(chan struct{}, 1)}
		gate.available <- struct{}{}
		r.locks[key] = gate
	}
	gate.users++
	r.mu.Unlock()

	select {
	case <-ctx.Done():
		r.mu.Lock()
		gate.users--
		if gate.users == 0 {
			delete(r.locks, key)
		}
		r.mu.Unlock()
		return nil, ctx.Err()
	case <-gate.available:
	}
	if err := ctx.Err(); err != nil {
		r.unlockSession(key, gate)
		return nil, err
	}
	return gate, nil
}

func (r *InMemoryUserSessionRepository) unlockSession(key string, gate *sessionLock) {
	gate.available <- struct{}{}
	r.mu.Lock()
	gate.users--
	if gate.users == 0 {
		delete(r.locks, key)
	}
	r.mu.Unlock()
}
