package postgres

import (
	"fmt"
	"testing"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentExternalIDConstraintMapping(t *testing.T) {
	for _, operation := range []string{"CreateAgent", "UpdateAgent"} {
		for _, tc := range []struct {
			code, constraint string
			kind             storage.ErrorKind
		}{
			{"23505", "uq_agents_external_id", storage.ErrorKindConflict},
			{"23514", "chk_agents_external_id_not_empty", storage.ErrorKindValidation},
		} {
			t.Run(operation+tc.code, func(t *testing.T) {
				err := mapAgentExternalIDError(operation, fmt.Errorf("driver: %w", &pgconn.PgError{Code: tc.code, ConstraintName: tc.constraint}))
				require.Error(t, err)
				var result *storage.StorageError
				require.ErrorAs(t, err, &result)
				assert.Equal(t, tc.kind, result.Kind)
				if tc.kind == storage.ErrorKindConflict {
					assert.ErrorIs(t, err, ports.ErrExternalIDConflict)
				}
			})
		}
	}
	assert.Nil(t, mapAgentExternalIDError("CreateAgent", &pgconn.PgError{Code: "23505", ConstraintName: "other"}))
}
