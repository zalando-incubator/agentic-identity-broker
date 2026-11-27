package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

// metadataRejection is one contract §4 row. body builds the request body from
// a valid definition, so each row also carries valid field changes.
type metadataRejection struct {
	name    string
	body    func(definition string) string
	message string
}

func metadataRow(name, metadata, message string) metadataRejection {
	return metadataRejection{name: name, message: message, body: func(definition string) string { return withMetadata(definition, metadata) }}
}

func metadataRejections() []metadataRejection {
	s := metadataSentinel
	return []metadataRejection{
		{name: "repeated member", message: "request body must contain at most one metadata member spelled exactly metadata", body: func(definition string) string {
			return withMetadata(withMetadata(definition, `{}`), `{"labels":{"`+s+`":"x"}}`)
		}},
		{name: "case variant member", message: "request body must contain at most one metadata member spelled exactly metadata", body: func(definition string) string {
			return strings.TrimSuffix(definition, "}") + `,"Metadata":{"labels":{"` + s + `":"x"}}}`
		}},
		metadataRow("metadata null", `null`, "metadata must not be null"),
		metadataRow("metadata not object", `"`+s+`"`, "metadata must be an object"),
		metadataRow("unknown member", `{"`+s+`":{}}`, "metadata contains an unknown or repeated member"),
		metadataRow("repeated inner member", `{"labels":{},"labels":{"`+s+`":"x"}}`, "metadata contains an unknown or repeated member"),
		metadataRow("labels null", `{"labels":null}`, "metadata.labels must not be null"),
		metadataRow("labels not object", `{"labels":["`+s+`"]}`, "metadata.labels must be an object"),
		metadataRow("label duplicate", `{"labels":{"`+s+`":"a","`+s+`":"b"}}`, "metadata.labels contains a duplicate key"),
		metadataRow("label invalid key", `{"labels":{"`+s+` key":"a"}}`, "metadata.labels contains an invalid key"),
		metadataRow("label invalid UTF-8 key", "{\"labels\":{\""+s+"\xff\":\"a\"}}", "metadata.labels contains an invalid key"),
		metadataRow("label reserved key", `{"labels":{"agenticidentitybroker.dev/`+s+`":"a"}}`, "metadata.labels contains a reserved key"),
		metadataRow("label invalid value", `{"labels":{"k":"`+s+` value"}}`, "metadata.labels contains an invalid value"),
		metadataRow("label non-string value", `{"labels":{"`+s+`":1}}`, "metadata.labels contains an invalid value"),
		metadataRow("too many labels", `{"labels":`+labelsJSON(65)+`}`, "metadata.labels contains more than 64 entries"),
		metadataRow("annotations null", `{"annotations":null}`, "metadata.annotations must not be null"),
		metadataRow("annotations not object", `{"annotations":"`+s+`"}`, "metadata.annotations must be an object"),
		metadataRow("annotation duplicate", `{"annotations":{"`+s+`":"a","`+s+`":"a"}}`, "metadata.annotations contains a duplicate key"),
		metadataRow("annotation invalid key", `{"annotations":{"`+s+` key":"a"}}`, "metadata.annotations contains an invalid key"),
		metadataRow("annotation reserved key", `{"annotations":{"policy.agenticidentitybroker.dev/`+s+`":"a"}}`, "metadata.annotations contains a reserved key"),
		metadataRow("annotation NUL value", `{"annotations":{"k":"`+s+`\u0000"}}`, "metadata.annotations contains an invalid value"),
		metadataRow("annotation surrogate value", `{"annotations":{"k":"`+s+`\ud800"}}`, "metadata.annotations contains an invalid value"),
		metadataRow("annotations too large", `{"annotations":{"`+s+`":"`+strings.Repeat("a", 65536)+`"}}`, "metadata.annotations exceeds 65536 bytes"),
	}
}

type metadataWritePath struct {
	name   string
	method string
	path   func(*metadataHarness, map[string]string) string
	body   func(map[string]string) string
}

func metadataWritePaths() []metadataWritePath {
	return []metadataWritePath{
		{"Agent POST", http.MethodPost, func(*metadataHarness, map[string]string) string { return "/api/agents" },
			func(ids map[string]string) string { return agentBody(ids["set"], "Changed", "") }},
		{"Agent PUT", http.MethodPut, func(_ *metadataHarness, ids map[string]string) string { return "/api/agents/" + ids["agent"] },
			func(ids map[string]string) string { return agentBody(ids["set"], "Changed", "") }},
		{"Agent collection PUT", http.MethodPut, func(*metadataHarness, map[string]string) string { return "/api/agents?external_id=registry" },
			func(ids map[string]string) string { return agentBody(ids["set"], "Changed", "") }},
		{"Service POST explicit", http.MethodPost, func(*metadataHarness, map[string]string) string { return "/api/services" },
			func(map[string]string) string { return serviceBody("") }},
		{"Service POST discovery", http.MethodPost, func(*metadataHarness, map[string]string) string { return "/api/services" },
			func(map[string]string) string { return discoveryServiceBody("") }},
		{"Service PUT explicit", http.MethodPut, func(_ *metadataHarness, ids map[string]string) string { return "/api/services/" + ids["service"] },
			func(map[string]string) string { return strings.Replace(serviceBody(""), `"Atlas"`, `"Changed"`, 1) }},
		{"Service PUT discovery", http.MethodPut, func(_ *metadataHarness, ids map[string]string) string { return "/api/services/" + ids["service"] },
			func(map[string]string) string { return discoveryServiceBody("") }},
		{"Permission Set POST", http.MethodPost, func(*metadataHarness, map[string]string) string { return "/api/permission-sets" },
			func(ids map[string]string) string { return permissionSetBody("New set", ids["service"], "") }},
		{"Permission Set PUT", http.MethodPut, func(_ *metadataHarness, ids map[string]string) string { return "/api/permission-sets/" + ids["set"] },
			func(ids map[string]string) string { return permissionSetBody("Changed", ids["service"], "") }},
	}
}

type metadataSnapshot struct {
	agents, services, sets []string
}

func (h *metadataHarness) snapshot() metadataSnapshot {
	h.t.Helper()
	read := func(path string, wrapped bool) []string {
		recorder := h.do(http.MethodGet, path, "")
		require.Equal(h.t, http.StatusOK, recorder.Code)
		var items []json.RawMessage
		if wrapped {
			var list struct {
				Items []json.RawMessage `json:"items"`
			}
			require.NoError(h.t, json.Unmarshal(recorder.Body.Bytes(), &list))
			items = list.Items
		} else {
			require.NoError(h.t, json.Unmarshal(recorder.Body.Bytes(), &items))
		}
		result := make([]string, len(items))
		for i, item := range items {
			result[i] = string(item)
		}
		slices.Sort(result)
		return result
	}
	return metadataSnapshot{agents: read("/api/agents", false), services: read("/api/services", false), sets: read("/api/permission-sets", true)}
}

func TestMetadataRejectedOnEveryWritePath(t *testing.T) {
	for _, path := range metadataWritePaths() {
		for _, row := range metadataRejections() {
			t.Run(path.name+"/"+row.name, func(t *testing.T) {
				h := newMetadataHarness(t)
				stored := `{"labels":{"stored":"value"}}`
				service := h.createService(stored)
				set := h.createPermissionSet(service["id"].(string), stored)
				agent := h.createAgent(set["id"].(string), stored)
				seeded := h.do(http.MethodPut, "/api/agents?external_id=registry", agentBody(set["id"].(string), "", stored))
				require.Equal(t, http.StatusCreated, seeded.Code, seeded.Body.String())
				ids := map[string]string{"service": service["id"].(string), "set": set["id"].(string), "agent": agent["id"].(string)}
				before := h.snapshot()
				h.logs.Reset()
				sideEffects := h.sideEffects.Load()

				recorder := h.do(path.method, path.path(h, ids), row.body(path.body(ids)))

				require.Equal(t, http.StatusBadRequest, recorder.Code, recorder.Body.String())
				response := h.decode(recorder)
				assert.Equal(t, "validation failed", response["error"])
				assert.Equal(t, row.message, response["message"])
				assert.NotContains(t, recorder.Body.String(), metadataSentinel)
				assert.NotContains(t, h.logs.String(), metadataSentinel)
				assert.NotContains(t, h.logs.String(), `"event"`, "no mutation audit record")
				assert.Equal(t, sideEffects, h.sideEffects.Load(), "no branch-key or encryption call")
				assert.Equal(t, before, h.snapshot(), "nothing changes")
			})
		}
	}
}

func TestMetadataPostKeepsDecodeBehavior(t *testing.T) {
	h := newMetadataHarness(t)
	service := h.createService("")
	serviceID := service["id"].(string)
	for _, tc := range []struct {
		name, path, definition string
	}{
		{"Service POST", "/api/services", serviceBody(`{"labels":{"a":"1"}}`)},
		{"Permission Set POST", "/api/permission-sets", permissionSetBody("Trailing", serviceID, `{"labels":{"a":"1"}}`)},
	} {
		t.Run(tc.name+" ignores trailing bytes", func(t *testing.T) {
			recorder := h.do(http.MethodPost, tc.path, tc.definition+` {"metadata":null} trailing`)
			require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
			assert.Equal(t, map[string]any{"labels": map[string]any{"a": "1"}}, h.decode(recorder)["metadata"])
		})
		t.Run(tc.name+" keeps decode errors", func(t *testing.T) {
			recorder := h.do(http.MethodPost, tc.path, `{"display_name":`)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			response := h.decode(recorder)
			assert.Equal(t, "invalid request body", response["error"])
			assert.Equal(t, "unexpected EOF", response["message"])
		})
	}
	sets, err := h.sets.List(context.Background(), id.ServiceID{})
	require.NoError(t, err)
	assert.Len(t, sets, 1)
}
