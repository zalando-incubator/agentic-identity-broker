package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const serviceSentinelMetadata = `{"labels":{"` + metadataSentinel + `":"` + metadataSentinel + `"},"annotations":{"note":"` + metadataSentinel + `"}}`

func TestServiceMetadataWriteSemantics(t *testing.T) {
	h := newMetadataHarness(t)
	created := h.createService(`{"labels":{"tier":"gold","team":"platform"},"annotations":{"note":"original"}}`)
	original := map[string]any{"labels": map[string]any{"team": "platform", "tier": "gold"}, "annotations": map[string]any{"note": "original"}}
	assert.Equal(t, original, created["metadata"])
	path := "/api/services/" + created["id"].(string)

	read := h.do(http.MethodGet, path, "")
	assert.Equal(t, original, h.decode(read)["metadata"])
	list := h.do(http.MethodGet, "/api/services", "")
	assert.Contains(t, list.Body.String(), `"metadata":{"labels":{"team":"platform","tier":"gold"},"annotations":{"note":"original"}}`)

	update := func(t *testing.T, metadata string) (*httptest.ResponseRecorder, string) {
		t.Helper()
		before := h.do(http.MethodGet, path, "").Header().Get("ETag")
		h.logs.Reset()
		recorder := h.do(http.MethodPut, path, strings.Replace(serviceBody(metadata), `"Atlas"`, `"Atlas renamed"`, 1))
		return recorder, before
	}
	for _, tc := range []struct {
		name, metadata string
		want           any
	}{
		{"omitted keeps the stored value", "", original},
		{"one member replaces both maps", `{"labels":{"tier":"silver"}}`, map[string]any{"labels": map[string]any{"tier": "silver"}}},
		{"sentinel values replace whole", serviceSentinelMetadata, map[string]any{"labels": map[string]any{metadataSentinel: metadataSentinel}, "annotations": map[string]any{"note": metadataSentinel}}},
		{"empty object clears both", `{}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder, beforeETag := update(t, tc.metadata)
			require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
			body := h.decode(recorder)
			if tc.want == nil {
				assert.NotContains(t, body, "metadata")
			} else {
				assert.Equal(t, tc.want, body["metadata"])
			}
			stored := h.do(http.MethodGet, path, "")
			assert.Equal(t, body["metadata"], h.decode(stored)["metadata"])
			assert.NotEqual(t, beforeETag, stored.Header().Get("ETag"), "an accepted update changes the ETag")
			assert.Equal(t, 1, strings.Count(h.logs.String(), `"event":"service.thirdparty.provider_updated"`))
			assert.NotContains(t, h.logs.String(), metadataSentinel)
		})
	}
	t.Run("null is rejected without change", func(t *testing.T) {
		before := h.do(http.MethodGet, path, "")
		recorder, _ := update(t, `null`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		after := h.do(http.MethodGet, path, "")
		assert.Equal(t, before.Body.String(), after.Body.String())
		assert.Equal(t, before.Header().Get("ETag"), after.Header().Get("ETag"))
	})
	t.Run("create audit carries no metadata", func(t *testing.T) {
		h.logs.Reset()
		h.createService(serviceSentinelMetadata)
		assert.Contains(t, h.logs.String(), `"event":"service.thirdparty.provider_created"`)
		assert.NotContains(t, h.logs.String(), metadataSentinel)
	})
}

func TestServiceMetadataBranchAuditRecords(t *testing.T) {
	for _, tc := range []struct {
		name, method, created, updated string
	}{
		{"CIMD public", "none", "service.thirdparty.cimd_public_service_created", "service.thirdparty.cimd_public_service_updated"},
		{"CIMD confidential", "private_key_jwt", "service.thirdparty.cimd_confidential_service_created", "service.thirdparty.cimd_confidential_service_updated"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMetadataHarness(t)
			body := clientRegistrationHandlerBody(model.ClientRegistrationCIMD, tc.method)
			body["metadata"] = json.RawMessage(serviceSentinelMetadata)
			encoded, err := json.Marshal(body)
			require.NoError(t, err)
			created := h.do(http.MethodPost, "/api/services", string(encoded))
			require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
			serviceID := h.decode(created)["id"].(string)
			assert.Contains(t, created.Body.String(), metadataSentinel, "the Admin response returns the metadata")
			body["display_name"] = "Renamed"
			delete(body, "metadata")
			encoded, err = json.Marshal(body)
			require.NoError(t, err)
			updated := h.do(http.MethodPut, "/api/services/"+serviceID, string(encoded))
			require.Equal(t, http.StatusOK, updated.Code, updated.Body.String())
			assert.Contains(t, updated.Body.String(), metadataSentinel, "an update that omits metadata keeps it")
			assert.Contains(t, h.logs.String(), `"event":"`+tc.created+`"`)
			assert.Contains(t, h.logs.String(), `"event":"`+tc.updated+`"`)
			assert.NotContains(t, h.logs.String(), metadataSentinel)
		})
	}
	t.Run("discovery", func(t *testing.T) {
		h := newMetadataHarness(t)
		h.providers.WithOAuthDiscoveryClient(adminCIMDDiscoveryClient(t))
		recorder := httptest.NewRecorder()
		request := adminDiscoveryRequest(t, http.MethodPost, "", map[string]any{
			"display_name": "MCP service",
			"discovery":    map[string]any{"enable_discovery": true, "resource_url": adminTestResource},
			"metadata":     json.RawMessage(serviceSentinelMetadata),
		})
		h.router.ServeHTTP(recorder, request)
		require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
		assert.Contains(t, recorder.Body.String(), metadataSentinel)
		assert.Contains(t, h.logs.String(), `"event":"service.thirdparty.discovered_service_created"`)
		assert.NotContains(t, h.logs.String(), metadataSentinel)
	})
}
