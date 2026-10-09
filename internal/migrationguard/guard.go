package migrationguard

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const (
	noTransactionDirective = "-- migrate:no-transaction"
	indexUpFile            = "036_user_sessions_access_token_expiry_index.up.sql"
	indexDownFile          = "036_user_sessions_access_token_expiry_index.down.sql"
	upStatement            = "CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at);"
	downStatement          = "DROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;"
)

func Validate(migrationsDir, databaseURL string) error {
	parsedURL, err := url.Parse(databaseURL)
	if err != nil {
		return fmt.Errorf("invalid database URL")
	}
	query, err := url.ParseQuery(parsedURL.RawQuery)
	if err != nil {
		return fmt.Errorf("invalid database URL query")
	}
	for _, value := range query["x-multi-statement"] {
		if value != "false" {
			return fmt.Errorf("x-multi-statement must be false")
		}
	}

	entries, err := os.ReadDir(migrationsDir)
	if err != nil {
		return fmt.Errorf("read migrations directory: %w", err)
	}

	var hasUp, hasDown bool
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".sql") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(migrationsDir, name))
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		var statement string
		switch name {
		case indexUpFile:
			hasUp = true
			statement = upStatement
		case indexDownFile:
			hasDown = true
			statement = downStatement
		default:
			if strings.Contains(string(contents), noTransactionDirective) {
				return fmt.Errorf("migration %s uses an unapproved no-transaction directive", name)
			}
			continue
		}

		firstLine, sql, found := strings.Cut(string(contents), "\n")
		if !found || firstLine != noTransactionDirective || strings.TrimSpace(sql) != statement {
			return fmt.Errorf("migration %s must have the exact first-line directive and approved concurrent index statement", name)
		}
	}
	if hasUp && !hasDown {
		return fmt.Errorf("missing migration %s", indexDownFile)
	}
	if hasDown && !hasUp {
		return fmt.Errorf("missing migration %s", indexUpFile)
	}
	return nil
}
