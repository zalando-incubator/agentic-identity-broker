package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/google/uuid"
)

var _ ports.RefreshSessionMaintenanceControl = (*RefreshMaintenanceControl)(nil)

type RefreshMaintenanceControl struct {
	adapter *Adapter
	owner   uuid.UUID
}

func NewRefreshMaintenanceControl(adapter *Adapter) *RefreshMaintenanceControl {
	return &RefreshMaintenanceControl{adapter: adapter, owner: uuid.New()}
}

func (c *RefreshMaintenanceControl) TryAcquireLease(ctx context.Context, duration time.Duration) (bool, error) {
	if duration < time.Millisecond || duration > time.Minute {
		return false, refreshValidation("RefreshMaintenance.Lease", errors.New("lease must be between one millisecond and one minute"))
	}
	var acquired, present bool
	err := c.adapter.storageExecutor(ctx).QueryRowxContext(ctx, `WITH lease AS (
		UPDATE refresh_maintenance
		SET lease_owner = $1, lease_until = clock_timestamp() + $2::bigint * interval '1 millisecond'
		WHERE singleton AND (lease_owner = $1 OR lease_until <= clock_timestamp())
		RETURNING singleton
	) SELECT EXISTS (SELECT 1 FROM lease), EXISTS (SELECT 1 FROM refresh_maintenance WHERE singleton)`,
		c.owner, duration.Milliseconds()).Scan(&acquired, &present)
	if err != nil {
		return false, refreshStoreError("RefreshMaintenance.Lease", err)
	}
	if !present {
		return false, refreshValidation("RefreshMaintenance.Lease", errors.New("maintenance control row is missing"))
	}
	return acquired, nil
}

func (c *RefreshMaintenanceControl) DatabaseID(ctx context.Context) (string, error) {
	var identity string
	err := c.adapter.storageExecutor(ctx).QueryRowxContext(ctx,
		`SELECT database_id::text FROM refresh_maintenance WHERE singleton`).Scan(&identity)
	return identity, refreshStoreError("RefreshMaintenance.DatabaseID", err)
}
