package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

const providerColumns = `
	s.id, s.canonical_id, s.display_name, s.client_id, s.client_secret_encrypted, s.token_endpoint_auth_method, s.oauth2_flavor, s.issuer_uri,
	s.enable_discovery, s.metadata_url, s.token_endpoint, s.authorize_endpoint, s.scopes,
	COALESCE((SELECT array_agg(pr.resource_uri ORDER BY pr.resource_uri)
	          FROM service_protected_resources pr WHERE pr.service_id = s.id), ARRAY[]::text[]),
	s.authorization_params, s.created_at, s.updated_at, s.version`

type PostgresThirdpartyOAuth2ProviderRepository struct{ adapter *Adapter }

func NewPostgresThirdpartyOAuth2ProviderRepository(adapter *Adapter) *PostgresThirdpartyOAuth2ProviderRepository {
	return &PostgresThirdpartyOAuth2ProviderRepository{adapter: adapter}
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) Create(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity) error {
	if err := r.requireDB("CreateThirdpartyOAuth2Provider"); err != nil {
		return err
	}
	if entity == nil || entity.ID.IsZero() {
		return storage.NewStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "provider and provider ID are required")
	}
	record, err := entityToRecord(entity)
	if err != nil {
		return storage.NewStorageError("CreateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "failed to convert entity to record")
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(execCtx, `INSERT INTO thirdparty_oauth2_services (
		id, canonical_id, display_name, client_id, client_secret_encrypted, token_endpoint_auth_method, oauth2_flavor, issuer_uri,
		enable_discovery, metadata_url, token_endpoint, authorize_endpoint, scopes,
		authorization_params, created_at, updated_at, version
	) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,1)`,
		record.ID, record.CanonicalID, record.DisplayName, record.ClientID, record.SecretCiphertext, record.TokenEndpointAuthMethod, record.Flavor, record.IssuerURI,
		record.EnableDiscovery, record.MetadataURL, record.TokenEndpoint, record.AuthorizeEndpoint, record.Scopes,
		record.AuthorizationParams, record.CreatedAt, record.UpdatedAt)
	if err != nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", err, "failed to create provider")
	}
	if err = insertProtectedResources(execCtx, tx, entity.ID, record.ProtectedResources); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return providerStorageError("CreateThirdpartyOAuth2Provider", err, "failed to commit transaction")
	}
	entity.Version = 1
	return nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) Get(ctx context.Context, serviceID id.ServiceID) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if err := r.requireDB("GetThirdpartyOAuth2Provider"); err != nil {
		return nil, err
	}
	if serviceID.IsZero() {
		return nil, storage.NewStorageError("GetThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "provider ID cannot be empty")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	record, err := scanProvider(r.adapter.db.QueryRowContext(queryCtx, `SELECT `+providerColumns+` FROM thirdparty_oauth2_services s WHERE s.id = $1`, serviceID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, storage.NewStorageError("GetThirdpartyOAuth2Provider", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if err != nil {
		return nil, providerStorageError("GetThirdpartyOAuth2Provider", err, "failed to get provider")
	}
	return recordToEntity(record)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) GetByCanonicalID(ctx context.Context, canonicalID string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if err := r.requireDB("GetThirdpartyOAuth2ProviderByCanonicalID"); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var serviceID id.ServiceID
	if err := r.adapter.db.QueryRowContext(queryCtx, `SELECT id FROM thirdparty_oauth2_services WHERE canonical_id = $1`, canonicalID).Scan(&serviceID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, storage.NewStorageError("GetThirdpartyOAuth2ProviderByCanonicalID", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
		}
		return nil, providerStorageError("GetThirdpartyOAuth2ProviderByCanonicalID", err, "failed to get provider")
	}
	return r.Get(ctx, serviceID)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) GetCanonicalIDs(ctx context.Context, ids []id.ServiceID) (map[id.ServiceID]string, error) {
	if err := r.requireDB("GetCanonicalIDsThirdpartyOAuth2Provider"); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return map[id.ServiceID]string{}, nil
	}

	ctxTimeout, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	rows, err := r.adapter.db.QueryContext(ctxTimeout, `SELECT id, canonical_id FROM thirdparty_oauth2_services WHERE id = ANY($1::uuid[]) AND canonical_id IS NOT NULL`, pq.Array(ids))
	if err != nil {
		return nil, providerStorageError("GetCanonicalIDsThirdpartyOAuth2Provider", err, "failed to get canonical IDs")
	}
	defer func() { _ = rows.Close() }()

	canonicalIDs := make(map[id.ServiceID]string, len(ids))
	for rows.Next() {
		var serviceID id.ServiceID
		var canonicalID string
		if err := rows.Scan(&serviceID, &canonicalID); err != nil {
			return nil, providerStorageError("GetCanonicalIDsThirdpartyOAuth2Provider", err, "failed to scan canonical ID")
		}
		canonicalIDs[serviceID] = canonicalID
	}
	if err := rows.Err(); err != nil {
		return nil, providerStorageError("GetCanonicalIDsThirdpartyOAuth2Provider", err, "failed to read canonical IDs")
	}
	return canonicalIDs, nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) Update(ctx context.Context, entity *model.ThirdpartyOAuth2ProviderEntity, expectedVersion *int64) error {
	if err := r.requireDB("UpdateThirdpartyOAuth2Provider"); err != nil {
		return err
	}
	if entity == nil || entity.ID.IsZero() {
		return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "provider and provider ID are required")
	}
	record, err := entityToRecord(entity)
	if err != nil {
		return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindValidation, err, "failed to convert entity to record")
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()
	paramsOmitted := entity.AuthorizationParams == nil
	canonicalIDOmitted := !entity.ClearCanonicalID && record.CanonicalID == nil
	query := `UPDATE thirdparty_oauth2_services SET display_name=$2, client_id=$3, client_secret_encrypted=$4,
		token_endpoint_auth_method=$5, oauth2_flavor=$6, issuer_uri=$7, enable_discovery=$8, metadata_url=$9,
		token_endpoint=$10, authorize_endpoint=$11, scopes=$12, authorization_params=CASE WHEN $13 THEN authorization_params ELSE $14 END,
		canonical_id=CASE WHEN $15 THEN NULL WHEN $16 THEN canonical_id ELSE $17 END,
		updated_at=$18, version=version+1 WHERE id=$1`
	args := []any{record.ID, record.DisplayName, record.ClientID, record.SecretCiphertext, record.TokenEndpointAuthMethod, record.Flavor, record.IssuerURI,
		record.EnableDiscovery, record.MetadataURL, record.TokenEndpoint, record.AuthorizeEndpoint, record.Scopes,
		paramsOmitted, record.AuthorizationParams, entity.ClearCanonicalID, canonicalIDOmitted, record.CanonicalID, record.UpdatedAt}
	if expectedVersion != nil {
		query += ` AND version=$19`
		args = append(args, *expectedVersion)
	}
	query += ` RETURNING created_at, authorization_params, version, canonical_id`
	var createdAt time.Time
	var authorizationParams providerAuthorizationParams
	var version int64
	var canonicalID *string
	err = tx.QueryRowContext(execCtx, query, args...).Scan(&createdAt, &authorizationParams, &version, &canonicalID)
	if errors.Is(err, sql.ErrNoRows) {
		if expectedVersion != nil {
			var exists bool
			if existsErr := tx.QueryRowContext(execCtx, `SELECT EXISTS(SELECT 1 FROM thirdparty_oauth2_services WHERE id=$1)`, record.ID).Scan(&exists); existsErr != nil {
				return providerStorageError("UpdateThirdpartyOAuth2Provider", existsErr, "failed to check provider")
			}
			if exists {
				return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "provider version is stale")
			}
		}
		return storage.NewStorageError("UpdateThirdpartyOAuth2Provider", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if err != nil {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", err, "failed to update provider")
	}
	if expectedVersion != nil {
		if _, err = tx.ExecContext(execCtx, `DELETE FROM service_protected_resources WHERE service_id=$1`, record.ID); err != nil {
			return providerStorageError("UpdateThirdpartyOAuth2Provider", err, "failed to replace protected resources")
		}
		if err = insertProtectedResources(execCtx, tx, entity.ID, record.ProtectedResources); err != nil {
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return providerStorageError("UpdateThirdpartyOAuth2Provider", err, "failed to commit transaction")
	}
	entity.CreatedAt, entity.UpdatedAt, entity.Version = createdAt, record.UpdatedAt, version
	entity.CanonicalID = canonicalID
	entity.AuthorizationParams = maps.Clone(map[string]string(authorizationParams))
	return nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) Delete(ctx context.Context, serviceID id.ServiceID) error {
	if err := r.requireDB("DeleteThirdpartyOAuth2Provider"); err != nil {
		return err
	}
	if serviceID.IsZero() {
		return storage.NewStorageError("DeleteThirdpartyOAuth2Provider", storage.ErrorKindValidation, nil, "provider ID cannot be empty")
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()

	var lockedServiceID id.ServiceID
	if err := tx.QueryRowContext(execCtx, `SELECT id FROM thirdparty_oauth2_services WHERE id = $1 FOR UPDATE`, serviceID).Scan(&lockedServiceID); errors.Is(err, sql.ErrNoRows) {
		return nil
	} else if err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "failed to lock provider")
	}

	referenceFilter := fmt.Sprintf(`[{"service_id":%q}]`, serviceID.String())
	var referenced bool
	if err := tx.QueryRowContext(execCtx, `SELECT EXISTS(SELECT 1 FROM agents WHERE service_requirements @> $1::jsonb)`, referenceFilter).Scan(&referenced); err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "failed to check agent references")
	}
	if referenced {
		return storage.NewStorageError("DeleteThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "cannot delete provider: agent service requirements reference it")
	}
	var grantReferenced bool
	if err := tx.QueryRowContext(execCtx, `
		SELECT EXISTS(
			SELECT 1
			FROM user_grants ug,
			     jsonb_array_elements(ug.granted_permission_sets) AS entry
			WHERE entry->'included_service_ids' @> to_jsonb($1::text)
			  AND (ug.valid_until IS NULL OR ug.valid_until > NOW())
		)
	`, serviceID.String()).Scan(&grantReferenced); err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "failed to check active user grant references")
	}
	if grantReferenced {
		return storage.NewStorageError("DeleteThirdpartyOAuth2Provider", storage.ErrorKindConflict, nil, "cannot delete provider: active user grants reference it")
	}
	if _, err := tx.ExecContext(execCtx, `DELETE FROM thirdparty_oauth2_services WHERE id = $1`, serviceID); err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "cannot delete provider: it is still referenced")
	}
	if err := tx.Commit(); err != nil {
		return providerStorageError("DeleteThirdpartyOAuth2Provider", err, "failed to commit transaction")
	}
	return nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) List(ctx context.Context) ([]*model.ThirdpartyOAuth2ProviderEntity, error) {
	if err := r.requireDB("ListThirdpartyOAuth2Providers"); err != nil {
		return nil, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	rows, err := r.adapter.db.QueryContext(queryCtx, `SELECT `+providerColumns+` FROM thirdparty_oauth2_services s ORDER BY s.created_at DESC`)
	if err != nil {
		return nil, providerStorageError("ListThirdpartyOAuth2Providers", err, "failed to list providers")
	}
	defer func() { _ = rows.Close() }()
	entities := make([]*model.ThirdpartyOAuth2ProviderEntity, 0)
	for rows.Next() {
		record, scanErr := scanProvider(rows)
		if scanErr != nil {
			return nil, providerStorageError("ListThirdpartyOAuth2Providers", scanErr, "failed to scan provider")
		}
		entity, conversionErr := recordToEntity(record)
		if conversionErr != nil {
			return nil, conversionErr
		}
		entities = append(entities, entity)
	}
	if err = rows.Err(); err != nil {
		return nil, providerStorageError("ListThirdpartyOAuth2Providers", err, "failed to iterate providers")
	}
	return entities, nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) FindByProtectedResource(ctx context.Context, resourceURI string) (*model.ThirdpartyOAuth2ProviderEntity, error) {
	if err := r.requireDB("FindByProtectedResource"); err != nil {
		return nil, err
	}
	if resourceURI == "" {
		return nil, storage.NewStorageError("FindByProtectedResource", storage.ErrorKindValidation, nil, "resource URI cannot be empty")
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	record, err := scanProvider(r.adapter.db.QueryRowContext(queryCtx, `SELECT `+providerColumns+` FROM thirdparty_oauth2_services s JOIN service_protected_resources pr ON pr.service_id=s.id WHERE pr.resource_uri=$1`, resourceURI))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, tokenexchange.NewInvalidTargetError("no service configured for the requested resource")
	}
	if err != nil {
		return nil, providerStorageError("FindByProtectedResource", err, "failed to find provider")
	}
	return recordToEntity(record)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) ListProtectedResources(ctx context.Context, serviceID id.ServiceID) ([]string, int64, error) {
	if err := r.requireDB("ListProtectedResources"); err != nil {
		return nil, 0, err
	}
	queryCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Read)
	defer cancel()
	var (
		resources []string
		version   int64
	)
	err := r.adapter.db.QueryRowContext(queryCtx, `SELECT
	COALESCE((SELECT array_agg(pr.resource_uri ORDER BY pr.resource_uri)
	          FROM service_protected_resources pr WHERE pr.service_id = s.id), ARRAY[]::text[]),
	s.version
FROM thirdparty_oauth2_services s WHERE s.id=$1`, serviceID).Scan(pq.Array(&resources), &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, 0, storage.NewStorageError("ListProtectedResources", storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	}
	if err != nil {
		return nil, 0, providerStorageError("ListProtectedResources", err, "failed to list protected resources")
	}
	return resources, version, nil
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) AddProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	return r.mutateProtectedResources(ctx, "AddProtectedResource", serviceID, func(ctx context.Context, tx *sql.Tx) (bool, error) {
		var inserted string
		err := tx.QueryRowContext(ctx, `INSERT INTO service_protected_resources (resource_uri, service_id) VALUES ($1,$2) ON CONFLICT (resource_uri) DO NOTHING RETURNING resource_uri`, resourceURI, serviceID).Scan(&inserted)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return false, err
		}
		var owner id.ServiceID
		if err = tx.QueryRowContext(ctx, `SELECT service_id FROM service_protected_resources WHERE resource_uri=$1`, resourceURI).Scan(&owner); err != nil {
			return false, err
		}
		if owner == serviceID {
			return false, nil
		}
		return false, storage.NewStorageError("AddProtectedResource", storage.ErrorKindConflict, nil, "protected resource is owned by another provider")
	}, resourceURI)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) RemoveProtectedResource(ctx context.Context, serviceID id.ServiceID, resourceURI string) (ports.ProtectedResourceMutationResult, error) {
	return r.mutateProtectedResources(ctx, "RemoveProtectedResource", serviceID, func(ctx context.Context, tx *sql.Tx) (bool, error) {
		result, err := tx.ExecContext(ctx, `DELETE FROM service_protected_resources WHERE service_id=$1 AND resource_uri=$2`, serviceID, resourceURI)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count == 0 {
			return false, storage.NewStorageError("RemoveProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "protected resource not found")
		}
		return true, nil
	}, resourceURI)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) RenameProtectedResource(ctx context.Context, serviceID id.ServiceID, fromURI, toURI string) (ports.ProtectedResourceMutationResult, error) {
	return r.mutateProtectedResources(ctx, "RenameProtectedResource", serviceID, func(ctx context.Context, tx *sql.Tx) (bool, error) {
		if fromURI == toURI {
			var exists bool
			err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM service_protected_resources WHERE service_id=$1 AND resource_uri=$2)`, serviceID, fromURI).Scan(&exists)
			if err != nil {
				return false, err
			}
			if !exists {
				return false, storage.NewStorageError("RenameProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "protected resource not found")
			}
			return false, nil
		}
		result, err := tx.ExecContext(ctx, `UPDATE service_protected_resources SET resource_uri=$1 WHERE service_id=$2 AND resource_uri=$3`, toURI, serviceID, fromURI)
		if err != nil {
			return false, err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return false, err
		}
		if count == 0 {
			return false, storage.NewStorageError("RenameProtectedResource", storage.ErrorKindNotFound, ports.ErrNotFound, "protected resource not found")
		}
		return true, nil
	}, toURI)
}

func (r *PostgresThirdpartyOAuth2ProviderRepository) mutateProtectedResources(ctx context.Context, operation string, serviceID id.ServiceID, mutate func(context.Context, *sql.Tx) (bool, error), resource string) (ports.ProtectedResourceMutationResult, error) {
	if err := r.requireDB(operation); err != nil {
		return ports.ProtectedResourceMutationResult{}, err
	}
	if serviceID.IsZero() || resource == "" {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError(operation, storage.ErrorKindValidation, nil, "provider ID and resource URI are required")
	}
	execCtx, cancel := context.WithTimeout(ctx, r.adapter.timeouts.Write)
	defer cancel()
	tx, err := r.adapter.db.BeginTx(execCtx, nil)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to begin transaction")
	}
	defer func() { _ = tx.Rollback() }()
	var version int64
	if err = tx.QueryRowContext(execCtx, `SELECT version FROM thirdparty_oauth2_services WHERE id=$1 FOR UPDATE`, serviceID).Scan(&version); errors.Is(err, sql.ErrNoRows) {
		return ports.ProtectedResourceMutationResult{}, storage.NewStorageError(operation, storage.ErrorKindNotFound, ports.ErrNotFound, "provider not found")
	} else if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to lock provider")
	}
	changed, err := mutate(execCtx, tx)
	if err != nil {
		var storageErr *storage.StorageError
		if errors.As(err, &storageErr) {
			return ports.ProtectedResourceMutationResult{}, err
		}
		return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to mutate protected resource")
	}
	if changed {
		if err = tx.QueryRowContext(execCtx, `UPDATE thirdparty_oauth2_services SET version=version+1 WHERE id=$1 RETURNING version`, serviceID).Scan(&version); err != nil {
			return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to update provider version")
		}
	}
	resources, err := protectedResources(execCtx, tx, serviceID)
	if err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to list protected resources")
	}
	if err = tx.Commit(); err != nil {
		return ports.ProtectedResourceMutationResult{}, providerStorageError(operation, err, "failed to commit transaction")
	}
	return ports.ProtectedResourceMutationResult{Resource: resource, ProtectedResources: resources, Version: version, Changed: changed}, nil
}

type providerRowScanner interface{ Scan(...any) error }
type providerResourceQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func scanProvider(scanner providerRowScanner) (*ThirdpartyOAuth2ProviderRecord, error) {
	var record ThirdpartyOAuth2ProviderRecord
	err := scanner.Scan(&record.ID, &record.CanonicalID, &record.DisplayName, &record.ClientID, &record.SecretCiphertext, &record.TokenEndpointAuthMethod, &record.Flavor, &record.IssuerURI, &record.EnableDiscovery, &record.MetadataURL, &record.TokenEndpoint, &record.AuthorizeEndpoint, &record.Scopes, pq.Array(&record.ProtectedResources), &record.AuthorizationParams, &record.CreatedAt, &record.UpdatedAt, &record.Version)
	return &record, err
}
func protectedResources(ctx context.Context, db providerResourceQuerier, serviceID id.ServiceID) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT resource_uri FROM service_protected_resources WHERE service_id=$1 ORDER BY resource_uri`, serviceID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	resources := make([]string, 0)
	for rows.Next() {
		var resource string
		if err = rows.Scan(&resource); err != nil {
			return nil, err
		}
		resources = append(resources, resource)
	}
	return resources, rows.Err()
}
func insertProtectedResources(ctx context.Context, tx *sql.Tx, serviceID id.ServiceID, resources []string) error {
	for _, resource := range resources {
		if _, err := tx.ExecContext(ctx, `INSERT INTO service_protected_resources (resource_uri, service_id) VALUES ($1,$2)`, resource, serviceID); err != nil {
			return providerStorageError("insertProtectedResources", err, "protected resource is already owned")
		}
	}
	return nil
}
func (r *PostgresThirdpartyOAuth2ProviderRepository) requireDB(operation string) error {
	if r.adapter == nil || r.adapter.db == nil {
		return storage.NewStorageError(operation, storage.ErrorKindConnection, nil, "database not initialized")
	}
	return nil
}
func providerStorageError(operation string, err error, message string) error {
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		return storage.NewStorageError(operation, storage.ErrorKindTimeout, err, "operation exceeded timeout")
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && (pgErr.Code == "23505" || pgErr.Code == "23503") {
		return storage.NewStorageError(operation, storage.ErrorKindConflict, err, message)
	}
	return storage.NewStorageError(operation, storage.ErrorKindConnection, err, message)
}
