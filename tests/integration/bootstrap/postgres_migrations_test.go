//go:build integration

package bootstrap

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/golang-migrate/migrate/v4/source"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	code := m.Run()
	if err := TerminateSharedPostgres(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to terminate shared PostgreSQL container: %v\n", err)
	}
	os.Exit(code)
}

func TestPostgresMigrationTemplates(t *testing.T) {
	pg := RequireSharedPostgres(t)
	migrationSource, err := source.Open("file://" + projectMigrationsDir(t))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, migrationSource.Close()) })

	t.Run("current schema and isolated clones", func(t *testing.T) {
		firstDB, _, cleanupFirst := pg.SetupMigratedDatabase(t)
		defer cleanupFirst()
		secondDB, _, cleanupSecond := pg.SetupMigratedDatabase(t)
		defer cleanupSecond()

		for _, dbName := range []string{firstDB, secondDB} {
			runner := pg.newMigrationRunner(t, dbName)
			defer func() { _, _ = runner.Close() }()
			version, dirty, err := runner.Version()
			require.NoError(t, err)
			require.False(t, dirty)
			_, err = migrationSource.Next(version)
			require.ErrorIs(t, err, os.ErrNotExist, "cloned database must include every repository migration")
		}

		pg.ExecuteSQL(t, firstDB, `INSERT INTO agents (id, display_name, description)
			VALUES ('10000000-0000-0000-0000-000000000001', 'isolated agent', 'migration fixture');`)
		require.Equal(t, "0", pg.QuerySQL(t, secondDB, `SELECT COUNT(*) FROM agents
			WHERE id = '10000000-0000-0000-0000-000000000001';`))
	})

	t.Run("historical schema can upgrade through remaining migrations", func(t *testing.T) {
		const historicalVersion = 26
		dbName, _, cleanup := pg.SetupDatabaseFromTemplate(t, "historical_026", func(t *testing.T, dbName string) {
			pg.ApplyMigrationsUpTo(t, dbName, historicalVersion)
		})
		defer cleanup()

		require.Equal(t, fmt.Sprint(historicalVersion)+"|f", pg.QuerySQL(t, dbName,
			`SELECT version, dirty FROM schema_migrations;`))
		runner := pg.newMigrationRunner(t, dbName)
		defer func() { _, _ = runner.Close() }()
		require.NoError(t, runner.Up())
		version, dirty, err := runner.Version()
		require.NoError(t, err)
		require.False(t, dirty)
		_, err = migrationSource.Next(version)
		require.ErrorIs(t, err, os.ErrNotExist)
	})
}
