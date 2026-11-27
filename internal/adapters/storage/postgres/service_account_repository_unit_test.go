package postgres

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestClassifyServiceAccountError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		kind     storage.ErrorKind
		sentinel error
	}{
		{"no rows", sql.ErrNoRows, storage.ErrorKindNotFound, nil},
		{"deadline", context.DeadlineExceeded, storage.ErrorKindTimeout, nil},
		{"name taken", &pgconn.PgError{Code: "23505", ConstraintName: "uq_service_accounts_service_canonical_id"}, storage.ErrorKindConflict, ports.ErrServiceAccountCanonicalIDConflict},
		{"other unique violation", &pgconn.PgError{Code: "23505", ConstraintName: "service_accounts_pkey"}, storage.ErrorKindConflict, nil},
		{"unknown service", &pgconn.PgError{Code: "23503"}, storage.ErrorKindNotFound, nil},
		{"check violation", &pgconn.PgError{Code: "23514"}, storage.ErrorKindValidation, nil},
		{"connection failure", errors.New("connection reset"), storage.ErrorKindConnection, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := classifyServiceAccountError("ServiceAccountRepository.Test", tc.err)
			var storageErr *storage.StorageError
			require.ErrorAs(t, err, &storageErr)
			assert.Equal(t, tc.kind, storageErr.Kind)
			assert.Equal(t, "ServiceAccountRepository.Test", storageErr.Operation)
			if tc.sentinel != nil {
				assert.ErrorIs(t, err, tc.sentinel)
			}
			assert.NotErrorIs(t, err, ports.ErrServiceAccountVersionMismatch)
		})
	}
}

func TestServiceAccountRecordConversion(t *testing.T) {
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	expiry := now.Add(36 * 24 * time.Hour)
	exp := expiry.Unix()
	account := &model.ServiceAccount{
		ID: id.NewServiceAccountID(), ServiceID: id.NewServiceID(), CanonicalID: "platform-triage-bot", Kind: model.ServiceAccountKindJWTBearer,
		Scopes: []string{"read", "write"}, Credential: model.NewEncryptedSecret([]byte{0x01, 0x02}),
		Summary: model.CredentialSummary{
			Format: model.CredentialFormatSignedJWT, Use: model.CredentialUseAccessToken,
			Iss: new("https://idp.example.com"), Sub: new("triage-bot"), Kid: new("2026-11"), Exp: &exp, Fingerprint: "SHA256:x",
		},
		CredentialExpiresAt: &expiry, Version: 3, CreatedAt: now, UpdatedAt: now.Add(time.Minute),
	}

	record, err := serviceAccountToRecord(account)
	require.NoError(t, err)
	assert.JSONEq(t, `["read","write"]`, string(record.Scopes))
	assert.JSONEq(t, `{"format":"signed_jwt","use":"access_token","iss":"https://idp.example.com","sub":"triage-bot","kid":"2026-11","exp":`+strconv.FormatInt(exp, 10)+`,"fingerprint":"SHA256:x"}`, string(record.Summary))
	assert.Equal(t, []byte{0x01, 0x02}, record.Credential)

	restored, err := record.toEntity()
	require.NoError(t, err)
	assert.Equal(t, account, restored)

	record.ExpiresAt = sql.NullTime{}
	withoutExpiry, err := record.toEntity()
	require.NoError(t, err)
	assert.Nil(t, withoutExpiry.CredentialExpiresAt)

	_, err = serviceAccountToRecord(&model.ServiceAccount{Credential: model.NewPlaintextSecret("eyJ.plaintext.jwt")})
	require.Error(t, err, "plaintext never reaches the table")
	assert.NotContains(t, err.Error(), "plaintext.jwt")

	record.Scopes = []byte(`not json`)
	_, err = record.toEntity()
	var storageErr *storage.StorageError
	require.ErrorAs(t, err, &storageErr)
	assert.Equal(t, storage.ErrorKindUnknown, storageErr.Kind)
}

func TestServiceAccountRepository_SpansWithoutDatabase(t *testing.T) {
	repo := NewServiceAccountRepository(&Adapter{})
	serviceID, accountID := id.NewServiceID(), id.NewServiceAccountID()
	account := &model.ServiceAccount{ID: accountID, ServiceID: serviceID, Credential: model.NewEncryptedSecret([]byte{1})}
	calls := []struct {
		span string
		call func(ctx context.Context) error
	}{
		{"storage.create.service_account", func(ctx context.Context) error { return repo.Create(ctx, account) }},
		{"storage.get.service_account", func(ctx context.Context) error { _, err := repo.Get(ctx, serviceID, accountID); return err }},
		{"storage.get_by_canonical_id.service_account", func(ctx context.Context) error {
			_, err := repo.GetByCanonicalID(ctx, serviceID, "platform-triage-bot")
			return err
		}},
		{"storage.list.service_account", func(ctx context.Context) error { _, err := repo.ListByService(ctx, serviceID); return err }},
		{"storage.update.service_account", func(ctx context.Context) error { return repo.Update(ctx, account, nil) }},
		{"storage.delete.service_account", func(ctx context.Context) error { return repo.Delete(ctx, serviceID, accountID, nil) }},
		{"storage.count.service_account", func(ctx context.Context) error { _, err := repo.CountByService(ctx, serviceID); return err }},
	}
	for _, test := range calls {
		t.Run(test.span, func(t *testing.T) {
			recorder := recordStorageSpans(t)

			require.Error(t, test.call(context.Background()))

			assertStorageSpanResult(t, requireOneStorageSpan(t, recorder, test.span), codes.Error, "connection", "")
		})
	}
}
