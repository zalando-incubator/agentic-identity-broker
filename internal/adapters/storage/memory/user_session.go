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
	mu           sync.RWMutex
	transactions *TransactionManager
	sessions     map[id.SessionID]*storage.UserSession // Key: session ID
	index        map[string]*storage.UserSession       // Key: "{principal}#{serviceID}"
	locks        map[string]*sessionLock
}

type sessionLock struct {
	available chan struct{}
	users     int
}

// NewInMemoryUserSessionRepository creates a new in-memory repository.
func NewInMemoryUserSessionRepository(transactions *TransactionManager) *InMemoryUserSessionRepository {
	return &InMemoryUserSessionRepository{
		transactions: transactions,
		sessions:     make(map[id.SessionID]*storage.UserSession),
		index:        make(map[string]*storage.UserSession),
		locks:        make(map[string]*sessionLock),
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
	key := principalServiceKey(session.Principal, session.ServiceID)
	if _, ambient := memoryTransaction(ctx); !ambient {
		gate, err := r.lockSession(ctx, key)
		if err != nil {
			return err
		}
		defer r.unlockSession(key, gate)
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return err
	}
	defer guard.release()

	r.mu.Lock()
	defer r.mu.Unlock()
	if existing, ok := r.index[key]; ok {
		session.ID = existing.ID
	}
	stored := copyUserSession(session)
	journalEntry(ctx, r.sessions, session.ID)
	r.sessions[session.ID] = stored
	journalEntry(ctx, r.index, key)
	r.index[key] = stored
	return nil
}

// Get retrieves a session by ID.
func (r *InMemoryUserSessionRepository) Get(ctx context.Context, sessionID id.SessionID) (*storage.UserSession, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	if sessionID.IsZero() {
		return nil, errors.New("session ID cannot be empty")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	session, ok := r.sessions[sessionID]
	if !ok {
		return nil, storage.NewStorageError("Get", storage.ErrorKindNotFound, nil, "session not found")
	}
	return copyUserSession(session), nil
}

// FindByPrincipalAndService retrieves the session for a principal and service.
func (r *InMemoryUserSessionRepository) FindByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) (*storage.UserSession, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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
	return copyUserSession(session), nil
}

// WithLockedSession serializes refreshes for one session while leaving other sessions available.
func (r *InMemoryUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	key := principalServiceKey(principal, serviceID)
	if _, ambient := memoryTransaction(ctx); !ambient {
		gate, err := r.lockSession(ctx, key)
		if err != nil {
			return nil, err
		}
		defer r.unlockSession(key, gate)
	}

	readGuard, err := r.transactions.lock(ctx, false)
	if err != nil {
		return nil, err
	}
	r.mu.RLock()
	current := r.index[key]
	if current == nil {
		r.mu.RUnlock()
		readGuard.release()
		return nil, nil
	}
	session := copyUserSession(current)
	r.mu.RUnlock()
	readGuard.release()
	updated, err := refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if updated {
		if err := session.Validate(); err != nil {
			return nil, err
		}
		writeGuard, err := r.transactions.lock(ctx, true)
		if err != nil {
			return nil, err
		}
		defer writeGuard.release()
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.index[key] != current {
			return nil, storage.NewStorageError("WithLockedSession", storage.ErrorKindConflict, nil, "session changed during refresh")
		}
		stored := copyUserSession(session)
		journalEntry(ctx, r.index, key)
		r.index[key] = stored
		journalEntry(ctx, r.sessions, session.ID)
		r.sessions[session.ID] = stored
	}
	return session, nil
}

// ListByPrincipal retrieves all sessions for a principal, including expired ones.
func (r *InMemoryUserSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*storage.UserSession
	for _, session := range r.sessions {
		if session.Principal == principal {
			sessions = append(sessions, copyUserSession(session))
		}
	}
	return sessions, nil
}

// ListActiveByPrincipal retrieves only non-expired sessions for a principal.
func (r *InMemoryUserSessionRepository) ListActiveByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*storage.UserSession
	for _, session := range r.sessions {
		if session.Principal == principal && !session.IsExpired() {
			sessions = append(sessions, copyUserSession(session))
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
		var gate *sessionLock
		if _, ambient := memoryTransaction(ctx); !ambient {
			var err error
			gate, err = r.lockSession(ctx, key)
			if err != nil {
				return err
			}
		}
		guard, err := r.transactions.lock(ctx, true)
		if err != nil {
			if gate != nil {
				r.unlockSession(key, gate)
			}
			return err
		}
		r.mu.Lock()
		current := r.sessions[sessionID]
		changedKey := current != nil && principalServiceKey(current.Principal, current.ServiceID) != key
		if !changedKey && current != nil {
			journalEntry(ctx, r.sessions, sessionID)
			delete(r.sessions, sessionID)
			journalEntry(ctx, r.index, key)
			delete(r.index, key)
		}
		r.mu.Unlock()
		guard.release()
		if gate != nil {
			r.unlockSession(key, gate)
		}
		if !changedKey {
			return nil
		}
	}
}

// DeleteByPrincipalAndService deletes the session for a principal and service.
func (r *InMemoryUserSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	if principal.IsZero() || serviceID.IsZero() {
		return errors.New("principal and serviceID required")
	}
	key := principalServiceKey(principal, serviceID)
	if _, ambient := memoryTransaction(ctx); !ambient {
		gate, err := r.lockSession(ctx, key)
		if err != nil {
			return err
		}
		defer r.unlockSession(key, gate)
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return err
	}
	defer guard.release()

	r.mu.Lock()
	defer r.mu.Unlock()
	session, ok := r.index[key]
	if ok {
		journalEntry(ctx, r.sessions, session.ID)
		delete(r.sessions, session.ID)
		journalEntry(ctx, r.index, key)
		delete(r.index, key)
	}
	return nil
}

// CountByService counts sessions referencing a service.
func (r *InMemoryUserSessionRepository) CountByService(ctx context.Context, serviceID id.ServiceID) (int, error) {
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
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
