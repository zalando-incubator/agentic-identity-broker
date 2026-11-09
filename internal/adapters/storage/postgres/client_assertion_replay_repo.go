package postgres

import (
	"context"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ClientAssertionReplayRepository = (*ClientAssertionReplayRepo)(nil)

// ClientAssertionReplayRepo stores consumed assertion identifiers in PostgreSQL.
type ClientAssertionReplayRepo struct {
	adapter *Adapter
}

// NewClientAssertionReplayRepo creates a replay repository over the PostgreSQL adapter.
func NewClientAssertionReplayRepo(adapter *Adapter) *ClientAssertionReplayRepo {
	return &ClientAssertionReplayRepo{adapter: adapter}
}

func (r *ClientAssertionReplayRepo) Consume(ctx context.Context, clientID, jti string, expiresAt time.Time) error {
	if r == nil || r.adapter == nil || r.adapter.db == nil {
		return storage.NewStorageError("ClientAssertionReplayRepo.Consume", storage.ErrorKindConnection, nil, "database not initialized")
	}
	record := &storage.ClientAssertionReplay{ClientID: clientID, JTI: jti, ExpiresAt: expiresAt}
	if err := record.Validate(); err != nil {
		return err
	}

	writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	result, err := r.adapter.db.ExecContext(writeCtx,
		`INSERT INTO client_assertion_replays (client_id, jti, expires_at) VALUES ($1, $2, $3)
		 ON CONFLICT (client_id, jti) DO NOTHING`, clientID, jti, expiresAt)
	if err != nil {
		return storage.NewStorageError("ClientAssertionReplayRepo.Consume", storage.ErrorKindUnknown, err, "failed to consume client assertion jti")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return storage.NewStorageError("ClientAssertionReplayRepo.Consume", storage.ErrorKindUnknown, err, "failed to determine replay record insertion")
	}
	if rowsAffected == 0 {
		return storage.NewStorageError("ClientAssertionReplayRepo.Consume", storage.ErrorKindConflict, nil, "client assertion jti was already consumed")
	}
	return nil
}

func (r *ClientAssertionReplayRepo) DeleteExpired(ctx context.Context) (int, error) {
	if r == nil || r.adapter == nil || r.adapter.db == nil {
		return 0, storage.NewStorageError("ClientAssertionReplayRepo.DeleteExpired", storage.ErrorKindConnection, nil, "database not initialized")
	}
	writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	result, err := r.adapter.db.ExecContext(writeCtx,
		`DELETE FROM client_assertion_replays WHERE expires_at < $1`, time.Now())
	if err != nil {
		return 0, storage.NewStorageError("ClientAssertionReplayRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to delete expired client assertion replay records")
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, storage.NewStorageError("ClientAssertionReplayRepo.DeleteExpired", storage.ErrorKindUnknown, err, "failed to determine deleted replay records")
	}
	return int(rowsAffected), nil
}
