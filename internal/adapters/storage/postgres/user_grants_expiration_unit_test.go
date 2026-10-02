package postgres

import (
	"context"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

func TestGrantExpirationRepositoryStorageErrorMapping(t *testing.T) {
	for _, tc := range []struct {
		name    string
		adapter *Adapter
		kind    storage.ErrorKind
	}{
		{"nil adapter", nil, storage.ErrorKindConnection},
		{"nil database", &Adapter{}, storage.ErrorKindConnection},
		{"deadline", newUnitTestSigningKeyRepo(t, signingKeyRepoTestConfig{beginErr: context.DeadlineExceeded, execErr: context.DeadlineExceeded, queryErr: context.DeadlineExceeded}).adapter, storage.ErrorKindTimeout},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := NewUserGrantRepository(tc.adapter)
			expiry := time.Now().UTC().Add(-time.Hour)
			_, err := repo.ListUnrecordedExpired(context.Background(), time.Now().UTC(), 10)
			assertSafeBusinessEventStorageError(t, err, tc.kind)
			_, err = repo.RecordExpiration(context.Background(), id.NewGrantID(), expiry)
			assertSafeBusinessEventStorageError(t, err, tc.kind)
		})
	}
}
