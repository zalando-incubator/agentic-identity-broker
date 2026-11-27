package admin

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

func TestPermissionSetMetadataWriteSemantics(t *testing.T) {
	h := newMetadataHarness(t)
	serviceID := h.createService("")["id"].(string)
	h.logs.Reset()
	created := h.createPermissionSet(serviceID, `{"labels":{"z":"1","exposure":"internal","Exposure":"public"},"annotations":{"note":"`+metadataSentinel+`"}}`)
	want := map[string]any{"labels": map[string]any{"Exposure": "public", "exposure": "internal", "z": "1"}, "annotations": map[string]any{"note": metadataSentinel}}
	assert.Equal(t, want, created["metadata"])
	assert.Contains(t, h.logs.String(), `"event":"permissionset.permission_set.created"`)
	assert.NotContains(t, h.logs.String(), metadataSentinel)
	path := "/api/permission-sets/" + created["id"].(string)
	setID, err := id.ParsePermissionSetID(created["id"].(string))
	require.NoError(t, err)

	read := h.do(http.MethodGet, path, "")
	assert.Equal(t, want, h.decode(read)["metadata"])
	list := h.do(http.MethodGet, "/api/permission-sets", "")
	assert.Contains(t, list.Body.String(), `"metadata":{"labels":{"Exposure":"public","exposure":"internal","z":"1"},"annotations":{"note":"`+metadataSentinel+`"}}`)

	for _, tc := range []struct {
		name, metadata string
		want           any
	}{
		{"one member replaces both maps", `{"annotations":{"owner":"x"}}`, map[string]any{"annotations": map[string]any{"owner": "x"}}},
		{"omitted clears stored metadata", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before, err := h.sets.Get(context.Background(), setID)
			require.NoError(t, err)
			h.logs.Reset()
			recorder := h.do(http.MethodPut, path, permissionSetBody("Read files", serviceID, tc.metadata))
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			body := h.decode(recorder)
			if tc.want == nil {
				assert.NotContains(t, body, "metadata")
			} else {
				assert.Equal(t, tc.want, body["metadata"])
			}
			assert.Equal(t, body["metadata"], h.decode(h.do(http.MethodGet, path, ""))["metadata"])
			after, err := h.sets.Get(context.Background(), setID)
			require.NoError(t, err)
			assert.True(t, after.UpdatedAt.After(before.UpdatedAt), "updated_at advances")
			assert.Equal(t, 1, strings.Count(h.logs.String(), `"event":"permissionset.permission_set.updated"`))
			assert.NotContains(t, h.logs.String(), metadataSentinel)
		})
	}
}
