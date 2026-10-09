package migrationguard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	upFile   = "036_user_sessions_access_token_expiry_index.up.sql"
	downFile = "036_user_sessions_access_token_expiry_index.down.sql"
	upSQL    = "-- migrate:no-transaction\nCREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);\n"
	downSQL  = "-- migrate:no-transaction\nDROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;\n"
	dbURL    = "postgres://migrator:unguessable-password@localhost:5432/broker?sslmode=disable"
)

func TestValidate(t *testing.T) {
	tests := []struct {
		name          string
		up            string
		down          string
		omitUp        bool
		omitDown      bool
		otherFile     string
		otherSQL      string
		unreadable    string
		url           string
		wantErrorFile string
		wantCondition string
	}{
		{
			name:      "existing history without 036 is allowed",
			omitUp:    true,
			omitDown:  true,
			otherFile: "030_cimd_client_uri_pattern_index.up.sql",
			otherSQL:  "CREATE INDEX CONCURRENTLY idx_agent_client_uris_pattern ON agent_client_uris (client_uri) WHERE client_uri LIKE '%*%';\n",
		},
		{
			name: "matching concurrent pair and a normal database URL are allowed",
		},
		{
			name: "one statement may have surrounding whitespace",
			up:   "-- migrate:no-transaction\n\n  CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);\n\n",
			down: "-- migrate:no-transaction\n  DROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;\n",
		},
		{
			name:          "missing UP directive is rejected",
			up:            "CREATE INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at ON user_sessions (access_token_expires_at, id);\n",
			wantErrorFile: upFile,
		},
		{
			name:          "directive must be the exact first line",
			up:            "\n" + upSQL,
			wantErrorFile: upFile,
		},
		{
			name:          "near miss directive is rejected",
			down:          "-- migrate: no-transaction\nDROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;\n",
			wantErrorFile: downFile,
		},
		{
			name:          "missing DOWN directive is rejected",
			down:          "DROP INDEX CONCURRENTLY idx_user_sessions_access_token_expires_at;\n",
			wantErrorFile: downFile,
		},
		{
			name:          "UP without DOWN is rejected",
			omitDown:      true,
			wantErrorFile: downFile,
		},
		{
			name:          "DOWN without UP is rejected",
			omitUp:        true,
			wantErrorFile: upFile,
		},
		{
			name:          "UP must create the named index",
			up:            strings.Replace(upSQL, "idx_user_sessions_access_token_expires_at", "idx_unrelated", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must index the named table",
			up:            strings.Replace(upSQL, "ON user_sessions", "ON other_sessions", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must index expiry then ID",
			up:            strings.Replace(upSQL, "(access_token_expires_at, id)", "(refresh_token_expires_at, id)", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must not reverse index keys",
			up:            strings.Replace(upSQL, "(access_token_expires_at, id)", "(id, access_token_expires_at)", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must not omit the ID key",
			up:            strings.Replace(upSQL, "(access_token_expires_at, id)", "(access_token_expires_at)", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must create an ordinary concurrent index",
			up:            strings.Replace(upSQL, "CREATE INDEX CONCURRENTLY", "CREATE UNIQUE INDEX CONCURRENTLY", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must use a plain B-tree index",
			up:            strings.Replace(upSQL, "ON user_sessions (", "ON user_sessions USING hash (", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must not use a partial index",
			up:            strings.Replace(upSQL, ");", ") WHERE access_token_expires_at IS NOT NULL;", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "UP must not hide an invalid index with IF NOT EXISTS",
			up:            strings.Replace(upSQL, "CONCURRENTLY idx_", "CONCURRENTLY IF NOT EXISTS idx_", 1),
			wantErrorFile: upFile,
		},
		{
			name:          "different DDL is not a concurrent index migration",
			up:            "-- migrate:no-transaction\nALTER TABLE user_sessions ADD COLUMN extra TEXT;\n",
			wantErrorFile: upFile,
		},
		{
			name:          "DOWN must drop the named index",
			down:          strings.Replace(downSQL, "idx_user_sessions_access_token_expires_at", "idx_unrelated", 1),
			wantErrorFile: downFile,
		},
		{
			name:          "DOWN must drop concurrently",
			down:          strings.Replace(downSQL, "DROP INDEX CONCURRENTLY", "DROP INDEX", 1),
			wantErrorFile: downFile,
		},
		{
			name:          "additional UP statement is rejected",
			up:            upSQL + "SELECT 1;\n",
			wantErrorFile: upFile,
		},
		{
			name:          "additional DOWN statement is rejected",
			down:          downSQL + "SELECT 1;\n",
			wantErrorFile: downFile,
		},
		{
			name:          "BEGIN before UP is rejected",
			up:            "-- migrate:no-transaction\nBEGIN;\n" + strings.TrimPrefix(upSQL, "-- migrate:no-transaction\n"),
			wantErrorFile: upFile,
		},
		{
			name:          "COMMIT after DOWN is rejected",
			down:          downSQL + "COMMIT;\n",
			wantErrorFile: downFile,
		},
		{
			name:          "directive on unrelated migration is rejected",
			otherFile:     "035_previous.up.sql",
			otherSQL:      "-- migrate:no-transaction\nSELECT 1;\n",
			wantErrorFile: "035_previous.up.sql",
		},
		{
			name:          "directive elsewhere in unrelated migration is rejected",
			otherFile:     "035_previous.up.sql",
			otherSQL:      "SELECT 1;\n-- migrate:no-transaction\n",
			wantErrorFile: "035_previous.up.sql",
		},
		{
			name:          "directive without 036 is rejected",
			omitUp:        true,
			omitDown:      true,
			otherFile:     "035_previous.down.sql",
			otherSQL:      "-- migrate:no-transaction\nSELECT 1;\n",
			wantErrorFile: "035_previous.down.sql",
		},
		{
			name:          "unreadable UP path is rejected",
			unreadable:    upFile,
			wantErrorFile: upFile,
		},
		{
			name:          "unreadable DOWN path is rejected",
			unreadable:    downFile,
			wantErrorFile: downFile,
		},
		{
			name:          "multi-statement database option is rejected without exposing credentials",
			url:           "postgres://migrator:unguessable-password@localhost:5432/broker?x-multi-statement=true",
			wantCondition: "x-multi-statement",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if !tt.omitUp {
				content := upSQL
				if tt.up != "" {
					content = tt.up
				}
				if tt.unreadable == upFile {
					require.NoError(t, os.Mkdir(filepath.Join(dir, upFile), 0o700))
				} else {
					require.NoError(t, os.WriteFile(filepath.Join(dir, upFile), []byte(content), 0o600))
				}
			}
			if !tt.omitDown {
				content := downSQL
				if tt.down != "" {
					content = tt.down
				}
				if tt.unreadable == downFile {
					require.NoError(t, os.Mkdir(filepath.Join(dir, downFile), 0o700))
				} else {
					require.NoError(t, os.WriteFile(filepath.Join(dir, downFile), []byte(content), 0o600))
				}
			}
			if tt.otherFile != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, tt.otherFile), []byte(tt.otherSQL), 0o600))
			}
			url := dbURL
			if tt.url != "" {
				url = tt.url
			}

			err := Validate(dir, url)
			if tt.wantErrorFile == "" && tt.wantCondition == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			if tt.wantErrorFile != "" {
				assert.Contains(t, err.Error(), tt.wantErrorFile)
			}
			if tt.wantCondition != "" {
				assert.Contains(t, err.Error(), tt.wantCondition)
			}
			assert.NotContains(t, err.Error(), "unguessable-password")
		})
	}
}
