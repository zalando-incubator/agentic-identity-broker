package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/golang-migrate/migrate/v4"
	"github.com/jmoiron/sqlx"
)

// NewHistoricalRefreshFixture copies verified HTTP-issued history into an isolated
// pre-proof fixture. Call only after stopping its source broker. Aging precedes
// migration 037's complete backfill; no trigger or proof is disabled or repaired.
func NewHistoricalRefreshFixture(t *testing.T, cfg *ports.Config, source *sqlx.DB, age func(*sqlx.DB) error) (_ *storageadapter.Adapter, _ *sqlx.DB, _ func(), err error) {
	t.Helper()
	ctx := context.Background()
	snapshot, err := source.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = snapshot.Rollback() }()
	var invalid bool
	if err := snapshot.GetContext(ctx, &invalid, `SELECT EXISTS (SELECT 1 FROM refresh_sessions WHERE terminal_reason IS NULL AND NOT lineage_valid)`); err != nil {
		return nil, nil, nil, err
	}
	if invalid {
		return nil, nil, nil, fmt.Errorf("historical fixture requires intact issued ancestry")
	}
	store, database, cleanup, err := newRefreshPostgresStorage(t, cfg, 36)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() {
		if err != nil {
			cleanup()
		}
	}()
	writeScope, err := database.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _ = writeScope.Rollback() }()
	for _, table := range []string{
		"thirdparty_oauth2_services", "permission_sets", "permission_set_service_scopes",
		"agents", "user_grants", "client_credentials", "signing_keys",
		"refresh_sessions", "refresh_tokens", "refresh_token_sessions", "refresh_revocation_receipts",
	} {
		var records []byte
		if err = snapshot.GetContext(ctx, &records, `SELECT COALESCE(jsonb_agg(row), '[]'::jsonb) FROM `+table+` row`); err != nil {
			return nil, nil, nil, err
		}
		if _, err = writeScope.ExecContext(ctx, `INSERT INTO `+table+` SELECT * FROM jsonb_populate_recordset(NULL::`+table+`, $1::jsonb)`, records); err != nil {
			return nil, nil, nil, err
		}
	}
	if err = writeScope.Commit(); err != nil {
		return nil, nil, nil, err
	}
	if err = age(database); err != nil {
		return nil, nil, nil, err
	}
	root, err := integrationbootstrap.FindProjectRoot()
	if err != nil {
		return nil, nil, nil, err
	}
	runner, err := migrate.New("file://"+filepath.Join(root, "migrations"), cfg.Storage.Postgres.ConnectionURL)
	if err != nil {
		return nil, nil, nil, err
	}
	defer func() { _, _ = runner.Close() }()
	if err = runner.Migrate(37); err != nil {
		return nil, nil, nil, err
	}
	// Install the remaining production migrations only after 037 backfills the
	// deliberately aged lineage; the broker must start with the current schema.
	if err = runner.Up(); err != nil && err != migrate.ErrNoChange {
		return nil, nil, nil, err
	}
	return store, database, cleanup, nil
}
