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
	transactions      *TransactionManager
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
	if _, ambient := memoryTransaction(ctx); !ambient {
		gate, err := r.lockSession(ctx, key)
		if err != nil {
			return nil, err
		}
		defer r.unlockSession(key, gate)
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return nil, err
	}
	defer guard.release()

	r.mu.Lock()
	defer r.mu.Unlock()
	// If session exists for this principal+service, update it
	if existing, ok := r.index[key]; ok {
		if !replace {
			return copyUserSession(existing), nil
		}
		// Reuse the retained identity and report it back to the caller.
		session.ID = existing.ID
		session.InitiatedAt = existing.InitiatedAt
		session.CreatedAt = existing.CreatedAt
		session.UpdatedAt = time.Now().UTC()
		journalEntry(ctx, r.sessions, existing.ID)
		delete(r.sessions, existing.ID)
	}

	stored := copyUserSession(session)
	stored.ExpectedIssuerURI = ""
	stored.ExpectedResource = ""
	journalEntry(ctx, r.sessions, stored.ID)
	r.sessions[stored.ID] = stored
	journalEntry(ctx, r.index, key)
	r.index[key] = stored
	return copyUserSession(stored), nil
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
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()

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
	if principal.IsZero() || serviceID.IsZero() {
		return nil, errors.New("principal and serviceID required")
	}
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()

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
// The callback owns persistence through UpdateRefreshedSession inside its own transaction.
func (r *InMemoryUserSessionRepository) WithLockedSession(ctx context.Context, principal id.Principal, serviceID id.ServiceID, refresh func(context.Context, *storage.UserSession) error) (*storage.UserSession, error) {
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
	if err := refresh(ctx, session); err != nil {
		return nil, err
	}
	return session, nil
}

// UpdateRefreshedSession replaces the session only if the stored row still matches the
// snapshot obtained before the provider exchange. The surrounding owner also journals the
// event, so both changes commit or roll back together. Only a new callback generation may
// replace the established upstream client identity.
func (r *InMemoryUserSessionRepository) UpdateRefreshedSession(ctx context.Context, previous, current *storage.UserSession) error {
	const operation = "UpdateRefreshedSession"
	if previous == nil || current == nil || previous.ID != current.ID ||
		previous.Principal != current.Principal || previous.ServiceID != current.ServiceID {
		return storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "invalid refreshed session")
	}
	if err := current.Validate(); err != nil {
		return err
	}
	if _, ambient := memoryTransaction(ctx); !ambient {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "refresh requires an owning transaction")
	}
	guard, err := r.transactions.lock(ctx, true)
	if err != nil {
		return err
	}
	defer guard.release()
	r.mu.Lock()
	defer r.mu.Unlock()
	key := principalServiceKey(previous.Principal, previous.ServiceID)
	stored := r.index[key]
	if stored == nil || stored.ID != previous.ID || !stored.UpdatedAt.Equal(previous.UpdatedAt) ||
		stored.CallbackID != previous.CallbackID ||
		!bytes.Equal(stored.EncryptedAccessToken, previous.EncryptedAccessToken) ||
		!bytes.Equal(stored.EncryptedRefreshToken, previous.EncryptedRefreshToken) {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, nil, "session changed during refresh")
	}
	if current.CallbackID == stored.CallbackID {
		current.UpstreamClientID = copyPointer(stored.UpstreamClientID)
	}
	copy := copyUserSession(current)
	copy.ExpectedIssuerURI = ""
	copy.ExpectedResource = ""
	journalEntry(ctx, r.index, key)
	r.index[key] = copy
	journalEntry(ctx, r.sessions, copy.ID)
	r.sessions[copy.ID] = copy
	return nil
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

	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Keep only the earliest limit (expiry, ID) pairs in a max-heap.
	var selected []*storage.UserSession
	for _, session := range r.sessions {
		if err := ctx.Err(); err != nil {
			return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
		}
		if session.RefreshRejected() || session.AccessTokenExpiresAt == nil || session.AccessTokenExpiresAt.After(threshold) {
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
		selected[i] = copyUserSession(session)
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

// CountSessionsByExpiry groups stored sessions by service, access-expiry class, and refresh usability.
func (r *InMemoryUserSessionRepository) CountSessionsByExpiry(ctx context.Context, threshold, now time.Time) ([]storage.SessionExpiryGroup, error) {
	const operation = "CountSessionsByExpiry"
	if err := ctx.Err(); err != nil {
		return nil, storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation cancelled or timed out")
	}

	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	type groupKey struct {
		serviceID     id.ServiceID
		accessExpiry  storage.AccessExpiryClass
		refreshUsable bool
	}
	counts := make(map[groupKey]int)
	for _, session := range r.sessions {
		key := groupKey{serviceID: session.ServiceID, accessExpiry: storage.AccessExpiryExpired}
		switch {
		case session.AccessTokenExpiresAt == nil:
			key.accessExpiry = storage.AccessExpiryUnknown
		case session.AccessTokenExpiresAt.After(threshold):
			key.accessExpiry = storage.AccessExpiryAfterThreshold
		case session.AccessTokenExpiresAt.After(now):
			key.accessExpiry = storage.AccessExpiryDue
		}
		key.refreshUsable = session.CanRefresh()
		counts[key]++
	}

	groups := make([]storage.SessionExpiryGroup, 0, len(counts))
	for key, count := range counts {
		groups = append(groups, storage.SessionExpiryGroup{
			ServiceID: key.serviceID, AccessExpiry: key.accessExpiry, RefreshUsable: key.refreshUsable, Count: count,
		})
	}
	return groups, nil
}

var _ ports.UserSessionHealthRepository = (*InMemoryUserSessionRepository)(nil)

// ListByPrincipal retrieves all sessions for a principal, including expired ones.
func (r *InMemoryUserSessionRepository) ListByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSession, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}

	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
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

func (r *InMemoryUserSessionRepository) ListSummariesByPrincipal(ctx context.Context, principal id.Principal) ([]*storage.UserSessionSummary, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	summaries := make([]*storage.UserSessionSummary, 0)
	for _, session := range r.sessions {
		if session.Principal == principal {
			summary := storage.NewUserSessionSummary(session, "", 0)
			summary.Scope = slices.Clone(summary.Scope)
			summaries = append(summaries, summary)
		}
	}
	return summaries, nil
}

func (r *InMemoryUserSessionRepository) ListActiveServiceIDsByPrincipal(ctx context.Context, principal id.Principal) ([]id.ServiceID, error) {
	if principal.IsZero() {
		return nil, errors.New("principal required")
	}
	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]id.ServiceID, 0)
	for _, session := range r.sessions {
		if session.Principal == principal && !session.RequiresReauthentication() {
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

	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return nil, gateErr
	}
	defer guard.release()
	r.mu.RLock()
	defer r.mu.RUnlock()

	var sessions []*storage.UserSession
	for _, session := range r.sessions {
		if session.Principal == principal && !session.RequiresReauthentication() {
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
		readGuard, err := r.transactions.lock(ctx, false)
		if err != nil {
			return err
		}
		r.mu.RLock()
		session := r.sessions[sessionID]
		r.mu.RUnlock()
		readGuard.release()
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
	if serviceID.IsZero() {
		return 0, errors.New("serviceID required")
	}

	guard, gateErr := r.transactions.lock(ctx, false)
	if gateErr != nil {
		return 0, gateErr
	}
	defer guard.release()
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
