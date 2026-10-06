//go:build integration

package integration

import (
	"testing"
	"time"
)

func TestPostgresRefreshBackendParityContracts(t *testing.T) {
	adapter, db := newRolloutStorage(t)
	root := createRolloutRoot(t, adapter, db, time.Now().UTC().Truncate(time.Second))
	runRefreshBackendParityContracts(t, adapter, root)
}
