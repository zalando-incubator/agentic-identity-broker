package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jmoiron/sqlx"
)

// Compile-time interface check
var _ ports.ClientCredentialRepository = (*ClientCredentialRepo)(nil)

// ClientCredentialRepo implements ClientCredentialRepository using PostgreSQL.
type ClientCredentialRepo struct {
	adapter *Adapter
}

// NewClientCredentialRepo creates a new PostgreSQL broker client credential repository.
func NewClientCredentialRepo(adapter *Adapter) *ClientCredentialRepo {
	return &ClientCredentialRepo{adapter: adapter}
}

func (r *ClientCredentialRepo) Create(ctx context.Context, credential *storage.ClientCredential) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("ClientCredentialRepo.Create", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if credential == nil || credential.AgentID.IsZero() {
		return refreshValidation("ClientCredentialRepo.Create", errors.New("credential agent required"))
	}
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != credential.AgentID {
		return refreshValidation("ClientCredentialRepo.Create", errors.New("credential belongs to another authorization scope"))
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	_, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`INSERT INTO client_credentials (id, agent_id, secret_hash, created_at, rotated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		credential.ID, credential.AgentID,
		credential.SecretHash, credential.CreatedAt, credential.RotatedAt,
	)
	if err != nil {
		return storage.NewStorageError("ClientCredentialRepo.Create", storage.ErrorKindUnknown, err, "failed to create credential")
	}
	return nil
}

func (r *ClientCredentialRepo) GetByAgentID(ctx context.Context, agentID id.AgentID) (*storage.ClientCredential, error) {
	if r.adapter.db == nil {
		return nil, storage.NewStorageError("ClientCredentialRepo.GetByAgentID", storage.ErrorKindConnection, nil, "database not initialized")
	}

	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()

	var cred storage.ClientCredential
	err := sqlx.GetContext(queryCtx, r.adapter.storageExecutor(queryCtx), &cred,
		`SELECT id, agent_id, secret_hash, created_at, rotated_at
		 FROM client_credentials WHERE agent_id = $1`, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.NewStorageError("ClientCredentialRepo.GetByAgentID", storage.ErrorKindNotFound, err, "credential not found")
		}
		if errors.Is(err, context.DeadlineExceeded) {
			return nil, storage.NewStorageError("ClientCredentialRepo.GetByAgentID", storage.ErrorKindTimeout, err, "operation exceeded timeout")
		}
		return nil, storage.NewStorageError("ClientCredentialRepo.GetByAgentID", storage.ErrorKindConnection, err, "failed to query credential")
	}
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != cred.AgentID {
		return nil, refreshValidation("ClientCredentialRepo.GetByAgentID", errors.New("credential belongs to another authorization scope"))
	}
	return &cred, nil
}

func (r *ClientCredentialRepo) GetByClientID(ctx context.Context, clientID id.ClientID) (*storage.ClientCredential, error) {
	agentID, err := id.ParseAgentID(clientID.String())
	if err != nil {
		return nil, storage.NewStorageError("ClientCredentialRepo.GetByClientID", storage.ErrorKindNotFound, nil, "invalid client_id")
	}
	return r.GetByAgentID(ctx, agentID)
}

func (r *ClientCredentialRepo) Delete(ctx context.Context, agentID id.AgentID) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("ClientCredentialRepo.Delete", storage.ErrorKindConnection, nil, "database not initialized")
	}

	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agentID {
		return refreshValidation("ClientCredentialRepo.Delete", errors.New("credential belongs to another authorization scope"))
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	result, err := r.adapter.storageExecutor(execCtx).ExecContext(execCtx,
		`DELETE FROM client_credentials WHERE agent_id = $1`, agentID)
	if err != nil {
		return storage.NewStorageError("ClientCredentialRepo.Delete", storage.ErrorKindUnknown, err, "failed to delete credential")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return storage.NewStorageError("ClientCredentialRepo.Delete", storage.ErrorKindNotFound, nil, "credential not found")
	}
	return nil
}

func (r *ClientCredentialRepo) Rotate(ctx context.Context, agentID id.AgentID, newCredential *storage.ClientCredential) error {
	if r.adapter.db == nil {
		return storage.NewStorageError("ClientCredentialRepo.Rotate", storage.ErrorKindConnection, nil, "database not initialized")
	}
	if newCredential == nil || newCredential.AgentID != agentID {
		return refreshValidation("ClientCredentialRepo.Rotate", errors.New("credential must belong to the replaced agent"))
	}
	if scope, ok := scopedAuthorization(ctx); ok && scope.agentID != agentID {
		return refreshValidation("ClientCredentialRepo.Rotate", errors.New("credential belongs to another authorization scope"))
	}

	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()

	tx, owned, err := r.adapter.participantTx(execCtx)
	if err != nil {
		return storage.NewStorageError("ClientCredentialRepo.Rotate", storage.ErrorKindUnknown, err, "failed to begin transaction")
	}
	defer func() {
		if owned {
			_ = tx.Rollback()
		}
	}()

	// DELETE before INSERT: agent_id is UNIQUE, so we must remove the old row first.
	// The transaction guarantees atomicity: if the INSERT fails the DELETE rolls back.
	result, err := tx.ExecContext(execCtx,
		`DELETE FROM client_credentials WHERE agent_id = $1`, agentID)
	if err != nil {
		return storage.NewStorageError("ClientCredentialRepo.Rotate", storage.ErrorKindUnknown, err, "failed to delete old credential")
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return storage.NewStorageError("ClientCredentialRepo.Rotate", storage.ErrorKindNotFound, nil,
			"no existing credential found for agent")
	}

	_, err = tx.ExecContext(execCtx,
		`INSERT INTO client_credentials (id, agent_id, secret_hash, created_at, rotated_at)
		 VALUES ($1, $2, $3, $4, $5)`,
		newCredential.ID, newCredential.AgentID,
		newCredential.SecretHash, newCredential.CreatedAt, newCredential.RotatedAt,
	)
	if err != nil {
		return storage.NewStorageError("ClientCredentialRepo.Rotate", storage.ErrorKindUnknown, err, "failed to insert new credential")
	}

	if owned {
		owned = false
		if err := commitAuthorizationTx(tx); err != nil {
			return err
		}
	}
	return nil
}
