package consent

import (
	"bytes"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsentRevocationBusinessEvent(t *testing.T) {
	failure := errors.New("storage unavailable")
	for _, tc := range []struct {
		name        string
		grantExists bool
		deleteErr   error
		cleanupErr  error
		wantErr     error
		wantEvent   bool
	}{
		{name: "success", grantExists: true, wantEvent: true},
		{name: "cleanup failure", grantExists: true, cleanupErr: failure, wantErr: failure, wantEvent: true},
		{name: "delete failure", grantExists: true, deleteErr: failure, wantErr: failure},
		{name: "not found", wantErr: ErrGrantNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			grants := &grantAuditRepository{mockGrantRepo: &mockGrantRepo{grants: map[id.GrantID]*storage.UserGrant{}}}
			validUntil := time.Now().Add(time.Hour).UTC()
			grant := &storage.UserGrant{
				ID: id.NewGrantID(), Principal: id.NewPrincipal("resource-owner"), AgentID: id.NewAgentID(),
				ValidUntil: &validUntil, UpdatedAt: validUntil.Add(-time.Hour),
				GrantedPermissionSets: []storage.GrantedPermissionSetEntry{{
					PermissionSetID: id.NewPermissionSetID(), IncludedServiceIDs: []id.ServiceID{id.NewServiceID()},
				}},
			}
			if tc.grantExists {
				grants.grants[grant.ID] = grant
			}
			grants.err = tc.deleteErr
			refreshRepo := &mockConsentRefreshRepo{err: tc.cleanupErr}
			capture := &grantAuditLogHandler{Handler: slog.NewJSONHandler(&bytes.Buffer{}, nil)}
			service := NewService(nil, nil, grants, nil, refreshRepo, nil, slog.New(capture))
			err := service.RevokeConsentForPrincipal(ctx, grant.Principal, grant.AgentID)
			require.ErrorIs(t, err, tc.wantErr)
			if !tc.wantEvent {
				assert.Empty(t, capture.records)
				return
			}
			require.NotNil(t, grants.deleted)
			assert.NotContains(t, grants.grants, grant.ID)
			grants.deleted.ID = id.NewGrantID()
			grants.deleted.Principal = id.NewPrincipal("different-owner")
			grants.deleted.AgentID = id.NewAgentID()
			*grants.deleted.ValidUntil = validUntil.Add(time.Hour)
			grants.deleted.UpdatedAt = time.Now()
			grants.deleted.GrantedPermissionSets[0].PermissionSetID = id.NewPermissionSetID()
			grants.deleted.GrantedPermissionSets[0].IncludedServiceIDs[0] = id.NewServiceID()
			var events []map[string]any
			for i, record := range capture.JSONRecords(t) {
				if record["event"] != "consent.grant.revoked" {
					continue
				}
				capture.records[i].Attrs(func(attr slog.Attr) bool {
					if attr.Key == "event" {
						assert.Equal(t, GrantRevoked, attr.Value.Any(), "the event must retain its typed definition")
					}
					return true
				})
				assert.Same(t, ctx, capture.contexts[i])
				events = append(events, record)
			}
			require.Len(t, events, 1)
			attributes := events[0]
			assert.Equal(t, grant.Principal.String(), attributes["principal"])
			assert.Equal(t, grant.ID.String(), attributes["grant_id"])
			assert.Equal(t, grant.AgentID.String(), attributes["agent_id"])
			assert.Equal(t, validUntil.Format(time.RFC3339Nano), attributes["valid_until"])
			assert.Equal(t, grant.UpdatedAt.Format(time.RFC3339Nano), attributes["updated_at"])
			assert.Equal(t, []any{map[string]any{
				"permission_set_id":    grant.GrantedPermissionSets[0].PermissionSetID.String(),
				"included_service_ids": []any{grant.GrantedPermissionSets[0].IncludedServiceIDs[0].String()},
			}}, attributes["granted_permission_sets"])
			require.IsType(t, "", attributes["revoked_at"])
			revokedAt, err := time.Parse(time.RFC3339Nano, attributes["revoked_at"].(string))
			require.NoError(t, err)
			assert.Equal(t, revokedAt.UTC().Format(time.RFC3339Nano), attributes["revoked_at"])
			if tc.cleanupErr != nil {
				assert.Equal(t, "failed", attributes["refresh_cleanup"])
			} else {
				assert.NotContains(t, attributes, "refresh_cleanup")
			}
			recordCount := len(capture.records)
			require.ErrorIs(t, service.RevokeConsentForPrincipal(ctx, grant.Principal, grant.AgentID), ErrGrantNotFound)
			assert.Len(t, capture.records, recordCount, "an unchanged retry must emit no event")
		})
	}
}
