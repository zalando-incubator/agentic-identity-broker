//go:build integration

package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	storageadapter "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	integrationbootstrap "github.com/agentic-identity-broker/agentic-identity-broker/tests/integration/bootstrap"
	"github.com/jackc/pgx/v5"
	"github.com/jmoiron/sqlx"
	"github.com/onsi/ginkgo/v2"
)

func LedgerBackends() []LedgerBackend { return []LedgerBackend{LedgerMemory, LedgerPostgres} }

// The required acceptance lane fails, rather than skips, when infrastructure is absent.
type ledgerPostgresHandle struct {
	integrationbootstrap.PostgresTestHandle
}

func (ledgerPostgresHandle) Skip(args ...any) { ginkgo.Fail(fmt.Sprint(args...)) }
func (ledgerPostgresHandle) Skipf(format string, args ...any) {
	ginkgo.Fail(fmt.Sprintf(format, args...))
}

func openLedgerPostgres(h *LedgerHarness) error {
	t := ledgerPostgresHandle{ginkgo.GinkgoT()}
	pg := integrationbootstrap.RequireSharedPostgres(t)
	root, err := integrationbootstrap.FindProjectRoot()
	if err != nil {
		return err
	}
	files, err := filepath.Glob(filepath.Join(root, "migrations", "*.up.sql"))
	if err != nil {
		return err
	}
	migrations := make([]integrationbootstrap.SQLMigration, 0, len(files))
	for _, file := range files {
		name := filepath.Base(file)
		version, err := strconv.ParseInt(strings.SplitN(name, "_", 2)[0], 10, 64)
		if err != nil {
			return errors.New("ledger bootstrap found an invalid migration filename")
		}
		migrations = append(migrations, integrationbootstrap.SQLMigration{File: name, Version: version})
	}
	if len(migrations) == 0 {
		return errors.New("ledger bootstrap requires repository migrations")
	}
	latest := migrations[len(migrations)-1].Version
	dbName, ownerURL, dropDatabase := pg.SetupDatabaseFromTemplate(t, fmt.Sprintf("ledger_%03d", latest), func(name string) {
		pg.ApplyMigrationsUpTo(t, name, migrations, int(latest))
	})
	// Register the clone immediately, including cleanup on connection/setup errors.
	ginkgo.DeferCleanup(dropDatabase)
	owner, err := sqlx.Connect("pgx", ownerURL+"&statement_timeout=30000")
	if err != nil {
		return errors.New("ledger bootstrap cannot connect as migration owner")
	}
	h.OwnerDB = owner

	roles := []string{dbName + "_runtime", dbName + "_reader", dbName + "_eraser"}
	createdRoles := make([]string, 0, len(roles))
	// Roles cannot be dropped before their clone and its owned grants are removed.
	ginkgo.DeferCleanup(func() {
		dropDatabase()
		for _, role := range createdRoles {
			pg.ExecuteSQL(t, "postgres", "DROP ROLE "+pgx.Identifier{role}.Sanitize())
		}
	})
	for _, role := range roles {
		_, err = owner.Exec("CREATE ROLE " + pgx.Identifier{role}.Sanitize() + " LOGIN PASSWORD 'ledger-test-only'")
		if err != nil {
			return errors.New("ledger bootstrap cannot create isolated operational role")
		}
		createdRoles = append(createdRoles, role)
	}
	runtimeRole, readerRole, erasureRole := pgx.Identifier{roles[0]}.Sanitize(), pgx.Identifier{roles[1]}.Sanitize(), pgx.Identifier{roles[2]}.Sanitize()
	_, err = owner.Exec("GRANT USAGE ON SCHEMA public TO " + runtimeRole + ", " + readerRole + ", " + erasureRole + "; GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO " + runtimeRole)
	if err != nil {
		return errors.New("ledger bootstrap cannot grant business-table access")
	}
	var ledgerExists bool
	if err = owner.Get(&ledgerExists, "SELECT to_regclass('public.business_events') IS NOT NULL"); err != nil {
		return errors.New("ledger bootstrap cannot inspect migrated schema")
	}
	if ledgerExists {
		_, err = owner.Exec("REVOKE UPDATE, DELETE ON public.business_events FROM " + runtimeRole + "; GRANT SELECT ON public.business_events, public.business_event_delivery_pending TO " + readerRole)
		if err != nil {
			return errors.New("ledger bootstrap cannot restrict ledger privileges")
		}
		for _, parent := range []string{"public.business_events", "public.business_event_delivery_pending"} {
			var children []string
			if err = owner.Select(&children, "SELECT relid::regclass::text FROM pg_partition_tree($1::regclass) WHERE level > 0", parent); err != nil {
				return errors.New("ledger bootstrap cannot inspect partitions")
			}
			for _, child := range children {
				// regclass text is produced by PostgreSQL from migration-owned names.
				if _, err = owner.Exec("REVOKE ALL ON " + child + " FROM " + runtimeRole); err != nil {
					return errors.New("ledger bootstrap cannot restrict child privileges")
				}
			}
		}
		var erasureExists bool
		if err = owner.Get(&erasureExists, "SELECT to_regprocedure('public.business_event_erase_subject(text)') IS NOT NULL"); err != nil {
			return errors.New("ledger bootstrap cannot inspect erasure function")
		}
		if erasureExists {
			if _, err = owner.Exec("GRANT EXECUTE ON FUNCTION public.business_event_erase_subject(text) TO " + erasureRole); err != nil {
				return errors.New("ledger bootstrap cannot grant operational erasure")
			}
		}
	}
	connectAs := func(role string) (string, error) {
		parsed, err := url.Parse(ownerURL)
		if err != nil {
			return "", errors.New("ledger bootstrap has an invalid database URL")
		}
		parsed.User = url.UserPassword(role, "ledger-test-only")
		return parsed.String(), nil
	}
	runtimeURL, err := connectAs(roles[0])
	if err != nil {
		return err
	}
	h.ConnectionURL = runtimeURL
	config := &ports.StorageConfig{Backend: "postgres", Postgres: ports.PostgresConfig{ConnectionURL: runtimeURL}, Timeouts: ports.StorageTimeouts{Read: 5 * time.Second, Write: 10 * time.Second}}
	h.Storage, err = storageadapter.NewAdapter(config)
	if err != nil {
		return errors.New("ledger bootstrap cannot initialize runtime storage")
	}
	h.closeStorage = func() error { return h.Storage.Close(context.Background()) }
	for i, destination := range []**sqlx.DB{&h.ReaderDB, &h.ErasureDB} {
		connectionURL, err := connectAs(roles[i+1])
		if err != nil {
			return err
		}
		*destination, err = sqlx.Connect("pgx", connectionURL+"&statement_timeout=30000")
		if err != nil {
			return errors.New("ledger bootstrap cannot connect operational role")
		}
	}
	return nil
}
