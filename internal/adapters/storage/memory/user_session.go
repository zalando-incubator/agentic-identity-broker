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
	mu       sync.RWMutex
	sessions map[id.SessionID]*storage.UserSession // Key: session ID
	index    map[string]*storage.UserSession       // Key: "{principal}#{serviceID}"
}

// NewInMemoryUserSessionRepository creates a new in-memory repository.
func NewInMemoryUserSessionRepository() *InMemoryUserSessionRepository {
	return &InMemoryUserSessionRepository{
		sessions: make(map[id.SessionID]*storage.UserSession),
		index:    make(map[string]*storage.UserSession),
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

	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalServiceKey(session.Principal, session.ServiceID)

	// If session exists for this principal+service, update it
	if existing, ok := r.index[key]; ok {
		// Reuse ID
		session.ID = existing.ID
		delete(r.sessions, existing.ID)
	}

	r.sessions[session.ID] = session
	r.index[key] = session
	return nil
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

// WithLockedSession holds the repository lock until a refreshed session is committed.
func (r *InMemoryUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) (bool, error)) (*storage.UserSession, error) {
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalServiceKey(principal, serviceID)
	current := r.index[key]
	if current == nil {
		return nil, nil
	}
	session := *current
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
		r.index[key] = &session
		r.sessions[session.ID] = &session
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

	r.mu.Lock()
	defer r.mu.Unlock()

	session, ok := r.sessions[sessionID]
	if ok {
		delete(r.sessions, sessionID)
		key := principalServiceKey(session.Principal, session.ServiceID)
		delete(r.index, key)
	}
	return nil
}

// DeleteByPrincipalAndService deletes the session for a principal and service.
func (r *InMemoryUserSessionRepository) DeleteByPrincipalAndService(ctx context.Context, principal id.Principal, serviceID id.ServiceID) error {
	if principal.IsZero() || serviceID.IsZero() {
		return errors.New("principal and serviceID required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	key := principalServiceKey(principal, serviceID)
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
