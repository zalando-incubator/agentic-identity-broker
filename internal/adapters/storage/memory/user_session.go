package memory

import (
	"context"
	"errors"
	"sync"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

// InMemoryUserSessionRepository is an in-memory implementation for testing/development.
type InMemoryUserSessionRepository struct {
	mu           sync.RWMutex
	transactions *TransactionManager
	sessions     map[id.SessionID]*storage.UserSession // Key: session ID
	index        map[string]*storage.UserSession       // Key: "{principal}#{serviceID}"
}

// NewInMemoryUserSessionRepository creates a new in-memory repository.
func NewInMemoryUserSessionRepository(transactions *TransactionManager) ports.UserSessionRepository {
	return &InMemoryUserSessionRepository{
		transactions: transactions,
		sessions:     make(map[id.SessionID]*storage.UserSession),
		index:        make(map[string]*storage.UserSession),
	}
}

// Create creates a new user session with upsert semantics.
func (r *InMemoryUserSessionRepository) Create(ctx context.Context, session *storage.UserSession) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	if session == nil {
		return errors.New("session cannot be nil")
	}
	if err := session.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalServiceKey(session.Principal, session.ServiceID)

	// If session exists for this principal+service, update it
	if existing, ok := r.index[key]; ok {
		// Reuse ID
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
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	if sessionID.IsZero() {
		return errors.New("session ID cannot be empty")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if ok {
		journalEntry(ctx, r.sessions, sessionID)
		delete(r.sessions, sessionID)
		key := principalServiceKey(session.Principal, session.ServiceID)
		journalEntry(ctx, r.index, key)
		delete(r.index, key)
	}
	return nil
}

// DeleteByPrincipalAndService deletes the session for a principal and service.
func (r *InMemoryUserSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	guard, gateErr := r.transactions.lock(ctx, true)
	if gateErr != nil {
		return gateErr
	}
	defer guard.release()
	if principal.IsZero() || serviceID.IsZero() {
		return errors.New("principal and serviceID required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalServiceKey(principal, serviceID)
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
