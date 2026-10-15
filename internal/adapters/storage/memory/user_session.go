package memory

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
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
	_, err := r.create(ctx, session, true)
	return err
}

func (r *InMemoryUserSessionRepository) CreateIfAbsent(ctx context.Context, session *storage.UserSession) (*storage.UserSession, error) {
	return r.create(ctx, session, false)
}

func (r *InMemoryUserSessionRepository) create(ctx context.Context, session *storage.UserSession, replace bool) (*storage.UserSession, error) {
	if session == nil {
		return nil, errors.New("session cannot be nil")
	}
	if err := session.Validate(); err != nil {
		return nil, err
	}
	if r.issuerSessionGate != nil {
		r.issuerSessionGate.RLock()
		defer r.issuerSessionGate.RUnlock()
	}
	operation := "CreateIfAbsent"
	if replace {
		operation = "Create"
	}
	if err := r.currentDiscoveryStateError(session, operation); err != nil {
		return nil, err
	}

	key := principalServiceKey(session.Principal, session.ServiceID)
	gate, err := r.lockSession(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.unlockSession(key, gate)

	r.mu.Lock()
	defer r.mu.Unlock()
	// If session exists for this principal+service, update it
	if existing, ok := r.index[key]; ok {
		if !replace {
			return cloneUserSession(existing), nil
		}
		// Reuse ID
		session.ID = existing.ID
		delete(r.sessions, existing.ID)
	}

	stored := cloneUserSession(session)
	stored.ExpectedIssuerURI = ""
	stored.ExpectedResource = ""
	r.sessions[stored.ID] = stored
	r.index[key] = stored
	return cloneUserSession(stored), nil
}

// currentDiscoveryStateError mirrors the PostgreSQL insert guard: a missing service is
// NotFound and stale issuer or audience expectations conflict. Without a bound provider
// repository (standalone tests) it cannot see services and checks only the expectations.
func (r *InMemoryUserSessionRepository) currentDiscoveryStateError(session *storage.UserSession, operation string) error {
	matches := session.ExpectedIssuerURI == "" && session.ExpectedResource == ""
	if r.provider != nil {
		r.provider.mu.RLock()
		current := r.provider.providers[session.ServiceID]
		if current != nil && current.entity.Discovery.ResourceURL != nil {
			matches = session.ExpectedIssuerURI != "" && session.ExpectedResource != "" &&
				session.ExpectedIssuerURI == current.entity.IssuerURI &&
				session.ExpectedResource == current.entity.AuthorizationParams["resource"]
		}
		r.provider.mu.RUnlock()
		if current == nil {
			return storage.NewStorageError(operation, storage.ErrorKindNotFound, nil, "service not found")
		}
	}
	if !matches {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "provider issuer or audience is no longer current")
	}
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
	return cloneUserSession(session), nil
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
	return cloneUserSession(session), nil
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
	session := cloneUserSession(current)
	upstreamClientID := session.UpstreamClientID
	r.mu.RUnlock()
	updated, err := refresh(ctx, session)
	if err != nil {
		return nil, err
	}
	if session.CallbackID == current.CallbackID {
		session.UpstreamClientID = upstreamClientID
		if upstreamClientID != nil {
			*upstreamClientID = *current.UpstreamClientID
		}
	}
	if updated {
		if err := session.Validate(); err != nil {
			return nil, err
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		r.mu.Lock()
		stored := cloneUserSession(session)
		r.index[key] = stored
		r.sessions[stored.ID] = stored
		r.mu.Unlock()
	}
	return session, nil
}

func (r *InMemoryUserSessionRepository) ListExpiringSessions(ctx context.Context, threshold time.Time, cursor storage.SessionExpiryCursor, limit int) ([]*storage.UserSession, error) {
	const operation = "ListExpiringSessions"
	if limit < 1 || limit > 1000 {
		return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "limit must be between 1 and 1000")
	}
	if cursor.ID.IsZero() && !cursor.AccessTokenExpiresAt.IsZero() {
		return nil, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "cursor expiry requires a session ID")
	}
	hasCursor := !cursor.ID.IsZero()
	if err := ctx.Err(); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Keep only the earliest limit (expiry, ID) pairs in a max-heap.
	var selected []*storage.UserSession
	for _, session := range r.sessions {
		if err := ctx.Err(); err != nil {
			return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
		}
		if session.AccessTokenExpiresAt == nil || session.AccessTokenExpiresAt.After(threshold) {
			continue
		}
		if hasCursor && compareSessionExpiryKey(session, cursor.AccessTokenExpiresAt, cursor.ID) <= 0 {
			continue
		}
		if selected == nil {
			selected = make([]*storage.UserSession, 0, limit)
		}
		if len(selected) < limit {
			selected = append(selected, session)
			for child := len(selected) - 1; child > 0; {
				parent := (child - 1) / 2
				if compareSessionExpiryKey(selected[parent], *selected[child].AccessTokenExpiresAt, selected[child].ID) >= 0 {
					break
				}
				selected[parent], selected[child] = selected[child], selected[parent]
				child = parent
			}
		} else if compareSessionExpiryKey(session, *selected[0].AccessTokenExpiresAt, selected[0].ID) < 0 {
			selected[0] = session
			for parent := 0; ; {
				child := parent*2 + 1
				if child >= len(selected) {
					break
				}
				if right := child + 1; right < len(selected) && compareSessionExpiryKey(selected[right], *selected[child].AccessTokenExpiresAt, selected[child].ID) > 0 {
					child = right
				}
				if compareSessionExpiryKey(selected[parent], *selected[child].AccessTokenExpiresAt, selected[child].ID) >= 0 {
					break
				}
				selected[parent], selected[child] = selected[child], selected[parent]
				parent = child
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}

	sort.Slice(selected, func(i, j int) bool {
		return compareSessionExpiryKey(selected[i], *selected[j].AccessTokenExpiresAt, selected[j].ID) < 0
	})
	for i, session := range selected {
		selected[i] = cloneUserSession(session)
	}
	if err := ctx.Err(); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}
	return selected, nil
}

// compareSessionExpiryKey compares a due session with an expiry/UUID key.
func compareSessionExpiryKey(session *storage.UserSession, expiry time.Time, sessionID id.SessionID) int {
	if cmp := session.AccessTokenExpiresAt.Compare(expiry); cmp != 0 {
		return cmp
	}
	return bytes.Compare(session.ID[:], sessionID[:])
}

var _ ports.UserSessionExpiryRepository = (*InMemoryUserSessionRepository)(nil)

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
			sessions = append(sessions, cloneUserSession(session))
		}
	}
	return sessions, nil
}

func (r *InMemoryUserSessionRepository) ListSummariesByPrincipal(_ context.Context, principal id.Principal) ([]*storage.UserSessionSummary, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	summaries := make([]*storage.UserSessionSummary, 0)
	for _, session := range r.sessions {
		if session.Principal == principal {
			summary := storage.NewUserSessionSummary(session, "", 0)
			summary.Scope = slices.Clone(summary.Scope)
			if summary.RefreshTokenExpiresAt != nil {
				expiresAt := *summary.RefreshTokenExpiresAt
				summary.RefreshTokenExpiresAt = &expiresAt
			}
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (r *InMemoryUserSessionRepository) ListActiveServiceIDsByPrincipal(_ context.Context, principal id.Principal) ([]id.ServiceID, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]id.ServiceID, 0)
	for _, session := range r.sessions {
		if session.Principal == principal && !session.IsExpired() {
			ids = append(ids, session.ServiceID)
		}
	}
	return ids, nil
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
			sessions = append(sessions, cloneUserSession(session))
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

func cloneUserSession(session *storage.UserSession) *storage.UserSession {
	clone := *session
	if session.UpstreamClientID != nil {
		clientID := *session.UpstreamClientID
		clone.UpstreamClientID = &clientID
	}
	clone.EncryptedAccessToken = slices.Clone(session.EncryptedAccessToken)
	clone.EncryptedRefreshToken = slices.Clone(session.EncryptedRefreshToken)
	clone.Scope = slices.Clone(session.Scope)
	if session.AccessTokenExpiresAt != nil {
		expiresAt := *session.AccessTokenExpiresAt
		clone.AccessTokenExpiresAt = &expiresAt
	}
	if session.RefreshTokenExpiresAt != nil {
		expiresAt := *session.RefreshTokenExpiresAt
		clone.RefreshTokenExpiresAt = &expiresAt
	}
	return &clone
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
