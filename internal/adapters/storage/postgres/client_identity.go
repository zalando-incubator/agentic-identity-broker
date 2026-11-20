package postgres

import (
	"context"
	"database/sql"
	"errors"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

var _ ports.ThirdpartyOAuth2ClientIdentityRepository = (*PostgresThirdpartyOAuth2ProviderRepository)(nil)

func (r *PostgresThirdpartyOAuth2ProviderRepository) RecordObservedClientIdentity(ctx context.Context, serviceID id.ServiceID, observed id.ClientID, expected ports.ClientIdentityObservation) (ports.ClientIdentityState, bool, bool, error) {
	const operation = "RecordObservedClientIdentity"
	if err := r.requireDB(operation); err != nil {
		return ports.ClientIdentityState{}, false, false, err
	}
	if observed.IsZero() {
		return ports.ClientIdentityState{}, false, false, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "observed identity cannot be empty")
	}
	var record struct {
		Identity *string `db:"client_identity"`
		Revision int64   `db:"client_identity_revision"`
	}
	writeCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	err := r.adapter.db.GetContext(writeCtx, &record, `UPDATE thirdparty_oauth2_services
 SET client_identity_revision=client_identity_revision+CASE WHEN client_identity IS NOT NULL AND client_identity<>$2 THEN 1 ELSE 0 END,
     client_identity=$2
 WHERE id=$1 AND credential_source='filesystem' AND version=$3 AND canonical_id=$4 AND client_identity_revision=$5
 RETURNING client_identity,client_identity_revision`, serviceID, observed.String(), expected.ServiceVersion, expected.CanonicalID, expected.Revision)
	cancel()
	applied := err == nil
	if errors.Is(err, sql.ErrNoRows) {
		readCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
		err = r.adapter.db.GetContext(readCtx, &record, `SELECT client_identity,client_identity_revision FROM thirdparty_oauth2_services WHERE id=$1`, serviceID)
		cancel()
	}
	if errors.Is(err, sql.ErrNoRows) {
		return ports.ClientIdentityState{}, false, false, storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if err != nil {
		return ports.ClientIdentityState{}, false, false, providerStorageError(operation, err, "failed to record observed identity")
	}
	state := ports.ClientIdentityState{Revision: record.Revision}
	if record.Identity != nil {
		state.Identity = new(id.ClientID(*record.Identity))
	}
	return state, applied, applied && state.Revision > expected.Revision, nil
}
