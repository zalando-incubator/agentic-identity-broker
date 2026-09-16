//go:build integration
// +build integration

package migrations_test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
)

// MigrationTestFramework provides reusable migration testing infrastructure.
type MigrationTestFramework struct {
	db            *sql.DB
	connStr       string
	dbName        string
	migrationsDir string
	cleanup       func()
}

// NewMigrationTestFramework creates a test database and initializes the framework.
func NewMigrationTestFramework(t *testing.T) *MigrationTestFramework {
	t.Helper()

	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	migrationsDir := filepath.Join("..", "..", "..", "migrations")
	absPath, err := filepath.Abs(migrationsDir)
	if err != nil {
		t.Fatalf("Failed to resolve migrations directory: %v", err)
	}

	postgres := bootstrap.RequireSharedPostgres(t)
	dbName, connStr, cleanup := postgres.SetupDatabaseFromTemplate(t, "migration_framework_blank", func(t *testing.T, dbName string) {})

	db, err := sql.Open("pgx", connStr)
	require.NoError(t, err)
	require.NoError(t, db.PingContext(context.Background()))

	return &MigrationTestFramework{
		db:            db,
		connStr:       connStr,
		dbName:        dbName,
		migrationsDir: absPath,
		cleanup:       cleanup,
	}
}

// Cleanup closes the database handle and drops the test database.
func (f *MigrationTestFramework) Cleanup(t *testing.T) {
	t.Helper()
	if f.db != nil {
		require.NoError(t, f.db.Close())
	}
	if f.cleanup != nil {
		f.cleanup()
	}
}

// Up runs migrations up to the specified version.
func (f *MigrationTestFramework) Up(t *testing.T, targetVersion uint) error {
	t.Helper()

	migrationsURL := "file://" + f.migrationsDir
	t.Logf("Applying migrations up to version %d from: %s", targetVersion, migrationsURL)

	m, err := migrate.New(migrationsURL, f.connStr)
	require.NoErrorf(t, err, "failed to create migrate instance")
	defer func() { _, _ = m.Close() }()

	if err := m.Migrate(targetVersion); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration to version %d failed: %w", targetVersion, err)
	}

	return nil
}

// UpAll runs all available migrations.
func (f *MigrationTestFramework) UpAll(t *testing.T) error {
	t.Helper()

	migrationsURL := "file://" + f.migrationsDir
	t.Logf("Migration source URL: %s", migrationsURL)
	t.Logf("Migrations directory exists: %s", f.migrationsDir)

	files, err := filepath.Glob(filepath.Join(f.migrationsDir, "*.sql"))
	if err == nil {
		t.Logf("Found %d SQL files:", len(files))
		for _, file := range files {
			t.Logf("  - %s", filepath.Base(file))
		}
	}

	m, err := migrate.New(migrationsURL, f.connStr)
	require.NoErrorf(t, err, "failed to create migrate instance")
	defer func() { _, _ = m.Close() }()

	s, d, err := m.Version()
	if err != migrate.ErrNilVersion {
		t.Logf("Current migration version: source=%d, dirty=%v", s, d)
	}

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("migration up failed: %w", err)
	}

	s, d, err = m.Version()
	if err != migrate.ErrNilVersion && err != nil {
		return err
	}
	t.Logf("Final state after Up: version=%d, dirty=%v", s, d)

	return nil
}

// Down rolls back migrations down to the specified version.
func (f *MigrationTestFramework) Down(t *testing.T, targetVersion uint) error {
	t.Helper()

	m, err := migrate.New("file://"+f.migrationsDir, f.connStr)
	require.NoErrorf(t, err, "failed to create migrate instance")
	defer func() { _, _ = m.Close() }()

	currentVersion, _, err := m.Version()
	if err != nil && err != migrate.ErrNilVersion {
		return fmt.Errorf("failed to get current version: %w", err)
	}

	steps := int(currentVersion) - int(targetVersion)
	if steps > 0 {
		if err := m.Steps(-steps); err != nil && err != migrate.ErrNoChange {
			return fmt.Errorf("migration down failed: %w", err)
		}
	}

	return nil
}

// DownAll rolls back all migrations.
func (f *MigrationTestFramework) DownAll(t *testing.T) error {
	t.Helper()

	migrationsURL := "file://" + f.migrationsDir
	t.Logf("Rolling back all migrations from: %s", migrationsURL)

	m, err := migrate.New(migrationsURL, f.connStr)
	require.NoErrorf(t, err, "failed to create migrate instance")
	defer func() { _, _ = m.Close() }()

	s, d, err := m.Version()
	if err != migrate.ErrNilVersion {
		t.Logf("Before Down: source=%d, dirty=%v", s, d)
	}

	for i := 0; ; i++ {
		currentVersion, _, err := m.Version()
		if err == migrate.ErrNilVersion {
			t.Logf("All migrations rolled back, at version 0")
			break
		}
		if err != nil {
			return fmt.Errorf("failed to get version: %w", err)
		}

		t.Logf("Rolling back from version %d...", currentVersion)
		if err := m.Steps(-1); err != nil {
			if err == migrate.ErrNoChange {
				t.Logf("No more migrations to rollback")
				break
			}
			return fmt.Errorf("rollback step %d failed: %w", i+1, err)
		}

		if i > 100 {
			return fmt.Errorf("rollback exceeded 100 steps")
		}

		nextVersion, _, err := m.Version()
		if err == migrate.ErrNilVersion {
			t.Logf("After rollback step %d: version=0 (no migrations)", i+1)
		} else if err != nil {
			return fmt.Errorf("failed to get version after rollback: %w", err)
		} else {
			t.Logf("After rollback step %d: version=%d", i+1, nextVersion)
		}
	}

	s, d, err = m.Version()
	if err == migrate.ErrNilVersion {
		t.Logf("After Down: version=0 (no migrations), dirty=%v", d)
		return nil
	}
	if err != nil {
		return err
	}
	t.Logf("After Down: source=%d, dirty=%v", s, d)

	return nil
}

// Force clears migration dirty state for a target version.
func (f *MigrationTestFramework) Force(t *testing.T, version uint) error {
	t.Helper()

	m, err := migrate.New("file://"+f.migrationsDir, f.connStr)
	require.NoErrorf(t, err, "failed to create migrate instance")
	defer func() { _, _ = m.Close() }()

	if err := m.Force(int(version)); err != nil {
		return fmt.Errorf("failed to force migration version %d: %w", version, err)
	}

	return nil
}

// Version returns the current migration version.
func (f *MigrationTestFramework) Version(t *testing.T) (uint, bool, error) {
	t.Helper()

	m, err := migrate.New("file://"+f.migrationsDir, f.connStr)
	if err != nil {
		return 0, false, err
	}
	defer func() { _, _ = m.Close() }()

	version, dirty, err := m.Version()
	if err == migrate.ErrNilVersion {
		return 0, false, nil
	}
	return version, dirty, err
}

// QuerySQL executes a SQL query and returns the first column of the first row as text.
// Additional rows are intentionally ignored; callers that need multi-row results
// should use f.db.QueryContext directly.
func (f *MigrationTestFramework) QuerySQL(t *testing.T, query string) (string, error) {
	t.Helper()

	row := f.db.QueryRowContext(context.Background(), query)
	var value any
	if err := row.Scan(&value); err != nil {
		return "", err
	}

	switch v := value.(type) {
	case nil:
		return "", nil
	case []byte:
		return string(v), nil
	default:
		return fmt.Sprint(v), nil
	}
}

// ExecuteSQL executes a SQL statement.
func (f *MigrationTestFramework) ExecuteSQL(t *testing.T, statement string) error {
	t.Helper()
	_, err := f.db.ExecContext(context.Background(), statement)
	return err
}

// ColumnExists checks if a column exists in a table.
func (f *MigrationTestFramework) ColumnExists(t *testing.T, table, column string) (bool, error) {
	t.Helper()
	result, err := f.QuerySQL(t, fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_name = '%s' AND column_name = '%s';
	`, table, column))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(result) != "0", nil
}

// IndexExists checks if an index exists.
func (f *MigrationTestFramework) IndexExists(t *testing.T, indexName string) (bool, error) {
	t.Helper()
	result, err := f.QuerySQL(t, fmt.Sprintf(`
		SELECT COUNT(*) FROM pg_indexes WHERE indexname = '%s';
	`, indexName))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(result) != "0", nil
}

// GetColumnType returns the data type of a column.
func (f *MigrationTestFramework) GetColumnType(t *testing.T, table, column string) (string, error) {
	t.Helper()
	return f.QuerySQL(t, fmt.Sprintf(`
		SELECT data_type FROM information_schema.columns
		WHERE table_name = '%s' AND column_name = '%s';
	`, table, column))
}

// CountRows returns the number of rows in a table.
func (f *MigrationTestFramework) CountRows(t *testing.T, table string, where ...string) (int64, error) {
	t.Helper()
	whereClause := ""
	if len(where) > 0 {
		whereClause = fmt.Sprintf("WHERE %s", where[0])
	}
	result, err := f.QuerySQL(t, fmt.Sprintf(`
		SELECT COUNT(*) FROM %s %s;
	`, table, whereClause))
	if err != nil {
		return 0, err
	}

	var count int64
	_, err = fmt.Sscanf(result, "%d", &count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// TableExists checks if a table exists.
func (f *MigrationTestFramework) TableExists(t *testing.T, tableName string) (bool, error) {
	t.Helper()
	result, err := f.QuerySQL(t, fmt.Sprintf(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_name = '%s';
	`, tableName))
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(result) != "0", nil
}
