package thirdparty

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

func saMetadata(t *testing.T, labels, annotations map[string]string) model.ObjectMetadata {
	t.Helper()
	metadata, err := model.NewObjectMetadata(labels, annotations)
	require.NoError(t, err)
	return metadata
}

func TestServiceAccountService_CreateMetadata(t *testing.T) {
	ctx := context.Background()

	t.Run("create stores metadata and the audit record carries none of it", func(t *testing.T) {
		h := newSAHarness(t)
		request := h.createRequest("platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		request.Metadata = saMetadata(t, map[string]string{"team.example.com/owner": "zz-sentinel"}, map[string]string{"note": "zz-sentinel"})

		view, err := h.service.Create(ctx, saOperator, h.serviceID, request)

		require.NoError(t, err)
		assert.True(t, request.Metadata.Equal(view.Account.ObjectMetadata))
		assert.True(t, request.Metadata.Equal(h.repo.accounts[view.Account.ID].ObjectMetadata))
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.created", records[0]["event"])
		assert.NotContains(t, h.logs.String(), "zz-sentinel")
	})

	t.Run("create without metadata stores empty metadata", func(t *testing.T) {
		h := newSAHarness(t)
		view := h.create(t, "platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		assert.True(t, view.Account.ObjectMetadata.IsEmpty())
	})

	t.Run("create refuses metadata that breaks a rule and stores nothing", func(t *testing.T) {
		h := newSAHarness(t)
		request := h.createRequest("platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		request.Metadata = model.RestoreObjectMetadata(map[string]string{"bad key": "v"}, nil)

		_, err := h.service.Create(ctx, saOperator, h.serviceID, request)

		require.ErrorIs(t, err, model.ErrMetadataLabelsInvalidKey)
		assert.Empty(t, h.repo.accounts)
		assert.Empty(t, h.encryption.plaintexts, "metadata is checked before the credential is encrypted")
	})
}

func TestServiceAccountService_UpdateMetadata(t *testing.T) {
	ctx := context.Background()
	stored := func(t *testing.T) (*saHarness, *ServiceAccountView) {
		t.Helper()
		h := newSAHarness(t)
		request := h.createRequest("platform-triage-bot", h.token(t, "k", h.now.Add(time.Hour)))
		request.Metadata = saMetadata(t, map[string]string{"tier": "gold"}, map[string]string{"note": "kept"})
		view, err := h.service.Create(ctx, saOperator, h.serviceID, request)
		require.NoError(t, err)
		h.resetLogs()
		h.now = h.now.Add(time.Minute)
		return h, view
	}

	t.Run("omitted metadata keeps the stored value", func(t *testing.T) {
		h, created := stored(t)
		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"write"}})
		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		assert.True(t, created.Account.ObjectMetadata.Equal(view.Account.ObjectMetadata))
		assert.True(t, created.Account.ObjectMetadata.Equal(h.repo.accounts[created.Account.ID].ObjectMetadata))
	})

	t.Run("sent metadata replaces both maps and is audited without values", func(t *testing.T) {
		h, created := stored(t)
		replacement := saMetadata(t, map[string]string{"tier": "zz-sentinel"}, nil)

		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Metadata: &replacement})

		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		assert.Equal(t, map[string]string{"tier": "zz-sentinel"}, view.Account.ObjectMetadata.Labels())
		assert.Empty(t, view.Account.ObjectMetadata.Annotations(), "keys are never merged")
		assert.Equal(t, int64(2), view.Account.Version)
		assert.Equal(t, h.now, view.Account.UpdatedAt)
		assert.Len(t, h.encryption.plaintexts, 1, "no encryption without a new credential")
		records := h.eventRecords(t, "service.service_account.")
		require.Len(t, records, 1)
		assert.Equal(t, "service.service_account.metadata_changed", records[0]["event"])
		assert.Equal(t, []any{"metadata"}, records[0]["changes"])
		for _, key := range []string{"previous_observed_canonical_id", "previous_observed_scopes", "previous_observed_credential", "metadata", "labels", "annotations"} {
			assert.NotContains(t, records[0], key)
		}
		assert.NotContains(t, h.logs.String(), "zz-sentinel")
		assert.NotContains(t, h.logs.String(), "gold")
	})

	t.Run("empty metadata clears the stored value", func(t *testing.T) {
		h, created := stored(t)
		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Metadata: &model.ObjectMetadata{}})
		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUpdated, outcome)
		assert.True(t, view.Account.ObjectMetadata.IsEmpty())
	})

	t.Run("equal metadata in any key order is a no-op", func(t *testing.T) {
		h, created := stored(t)
		same := saMetadata(t, map[string]string{"tier": "gold"}, map[string]string{"note": "kept"})
		view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Metadata: &same})
		require.NoError(t, err)
		assert.Equal(t, ServiceAccountUnchanged, outcome)
		assert.Equal(t, int64(1), view.Account.Version)
		assert.Equal(t, 1, h.repo.writes)
		assert.Empty(t, h.eventRecords(t, "service.service_account."))
	})

	t.Run("metadata is the last change in event precedence", func(t *testing.T) {
		for name, tc := range map[string]struct {
			update ServiceAccountUpdate
			event  string
			change []any
		}{
			"rename":  {ServiceAccountUpdate{CanonicalID: new("platform-triage"), Scopes: []string{"read"}}, "service.service_account.renamed", []any{"canonical_id", "metadata"}},
			"ceiling": {ServiceAccountUpdate{Scopes: []string{"admin"}}, "service.service_account.ceiling_changed", []any{"scopes", "metadata"}},
		} {
			t.Run(name, func(t *testing.T) {
				h, created := stored(t)
				replacement := saMetadata(t, map[string]string{"tier": "silver"}, nil)
				tc.update.Metadata = &replacement
				_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, tc.update)
				require.NoError(t, err)
				records := h.eventRecords(t, "service.service_account.")
				require.Len(t, records, 1)
				assert.Equal(t, tc.event, records[0]["event"])
				assert.Equal(t, tc.change, records[0]["changes"])
			})
		}
	})

	t.Run("invalid metadata is refused before any write", func(t *testing.T) {
		h, created := stored(t)
		invalid := model.RestoreObjectMetadata(nil, map[string]string{"k": strings.Repeat("a", 65537)})
		_, _, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Metadata: &invalid})
		require.ErrorIs(t, err, model.ErrMetadataAnnotationsTooLarge)
		assert.Equal(t, 1, h.repo.writes)
		assert.True(t, created.Account.ObjectMetadata.Equal(h.repo.accounts[created.Account.ID].ObjectMetadata))
	})
}

func TestServiceAccountService_RetriedUpdateReadsTheClockAgain(t *testing.T) {
	ctx := context.Background()
	h := newSAHarness(t)
	created := h.create(t, "a", h.token(t, "k", h.now.Add(time.Hour)))
	h.now = h.now.Add(time.Minute)
	concurrent := h.now.Add(time.Second)
	h.repo.beforeWrite = func() {
		changed := cloneAccount(h.repo.accounts[created.Account.ID])
		changed.CanonicalID = "renamed"
		changed.UpdatedAt = concurrent
		changed.Version++
		h.repo.accounts[created.Account.ID] = changed
		h.now = concurrent.Add(time.Second)
	}
	replacement := saMetadata(t, map[string]string{"tier": "gold"}, nil)

	view, outcome, err := h.service.Update(ctx, saOperator, h.serviceID, created.Account.ID, ServiceAccountUpdate{Scopes: []string{"read"}, Metadata: &replacement})

	require.NoError(t, err)
	assert.Equal(t, ServiceAccountUpdated, outcome)
	assert.Equal(t, "renamed", view.Account.CanonicalID)
	assert.True(t, view.Account.UpdatedAt.After(concurrent), "a retried write must not store an updated_at before the commit it retried after")
	assert.Equal(t, view.Account.UpdatedAt, h.repo.accounts[created.Account.ID].UpdatedAt)
}
