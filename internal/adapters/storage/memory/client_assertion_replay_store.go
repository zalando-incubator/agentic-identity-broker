package memory

import (
	"context"
	"sync"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ClientAssertionReplayRepository = (*ClientAssertionReplayStore)(nil)

type clientAssertionReplayKey struct {
	clientID string
	jti      string
}

// ClientAssertionReplayStore keeps consumed assertion identifiers in memory.
type ClientAssertionReplayStore struct {
	mu   sync.Mutex
	data map[clientAssertionReplayKey]time.Time
}

// NewClientAssertionReplayStore creates an empty replay store.
func NewClientAssertionReplayStore() *ClientAssertionReplayStore {
	return &ClientAssertionReplayStore{data: make(map[clientAssertionReplayKey]time.Time)}
}

func (s *ClientAssertionReplayStore) Consume(_ context.Context, clientID, jti string, expiresAt time.Time) error {
	record := &storage.ClientAssertionReplay{ClientID: clientID, JTI: jti, ExpiresAt: expiresAt}
	if err := record.Validate(); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	key := clientAssertionReplayKey{clientID: clientID, jti: jti}
	if _, exists := s.data[key]; exists {
		return storage.NewStorageError("ClientAssertionReplayStore.Consume", storage.ErrorKindConflict, nil, "client assertion jti was already consumed")
	}
	s.data[key] = expiresAt
	return nil
}

func (s *ClientAssertionReplayStore) DeleteExpired(_ context.Context) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	deleted := 0
	for key, expiresAt := range s.data {
		if expiresAt.Before(now) {
			delete(s.data, key)
			deleted++
		}
	}
	return deleted, nil
}
