package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type agentMetadataFixture struct {
	*metadataHarness
	permissionSetID string
}

func newAgentMetadataFixture(t *testing.T) agentMetadataFixture {
	t.Helper()
	h := newMetadataHarness(t)
	service := h.createService("")
	set := h.createPermissionSet(service["id"].(string), "")
	return agentMetadataFixture{metadataHarness: h, permissionSetID: set["id"].(string)}
}

// agentWrite sends an Agent definition by one of the three Agent write paths.
func (f agentMetadataFixture) agentWrite(t *testing.T, path, metadata string) (int, map[string]any, string) {
	t.Helper()
	switch path {
	case "POST":
		recorder := f.do(http.MethodPost, "/api/agents", agentBody(f.permissionSetID, "", metadata))
		return recorder.Code, decodeOptional(recorder.Body.Bytes()), recorder.Body.String()
	case "PUT":
		created := f.createAgent(f.permissionSetID, `{"labels":{"stored":"value"},"annotations":{"stored":"value"}}`)
		recorder := f.do(http.MethodPut, "/api/agents/"+created["id"].(string), agentBody(f.permissionSetID, "", metadata))
		return recorder.Code, decodeOptional(recorder.Body.Bytes()), recorder.Body.String()
	default:
		selector := "/api/agents?external_id=registry-" + id.NewAgentID().String()
		seed := f.do(http.MethodPut, selector, agentBody(f.permissionSetID, "", `{"labels":{"stored":"value"},"annotations":{"stored":"value"}}`))
		require.Equal(t, http.StatusCreated, seed.Code, seed.Body.String())
		recorder := f.do(http.MethodPut, selector, agentBody(f.permissionSetID, "", metadata))
		return recorder.Code, decodeOptional(recorder.Body.Bytes()), recorder.Body.String()
	}
}

func decodeOptional(body []byte) map[string]any {
	var value map[string]any
	_ = json.Unmarshal(body, &value)
	return value
}

var agentWritePaths = []string{"POST", "PUT", "collection PUT"}

func TestAgentMetadataWritePaths(t *testing.T) {
	cases := []struct {
		name     string
		metadata string
		want     any
	}{
		{"omitted is empty", "", nil},
		{"empty object is empty", `{}`, nil},
		{"labels only", `{"labels":{"tier":"gold"}}`, map[string]any{"labels": map[string]any{"tier": "gold"}}},
		{"annotations only", `{"annotations":{"note":"x"}}`, map[string]any{"annotations": map[string]any{"note": "x"}}},
		{"both", `{"labels":{"b":"2","a":"1"},"annotations":{"note":""}}`, map[string]any{"labels": map[string]any{"a": "1", "b": "2"}, "annotations": map[string]any{"note": ""}}},
		{"empty maps are omitted", `{"labels":{},"annotations":{"note":"x"}}`, map[string]any{"annotations": map[string]any{"note": "x"}}},
	}
	for _, path := range agentWritePaths {
		for _, tc := range cases {
			t.Run(path+"/"+tc.name, func(t *testing.T) {
				f := newAgentMetadataFixture(t)
				status, body, raw := f.agentWrite(t, path, tc.metadata)
				require.Contains(t, []int{http.StatusOK, http.StatusCreated}, status, raw)
				if tc.want == nil {
					assert.NotContains(t, body, "metadata")
				} else {
					assert.Equal(t, tc.want, body["metadata"])
				}
				read := f.do(http.MethodGet, "/api/agents/"+body["id"].(string), "")
				assert.Equal(t, body["metadata"], f.decode(read)["metadata"])
			})
		}
	}
}

func TestAgentMetadataNullRejectedWithoutChange(t *testing.T) {
	for _, path := range agentWritePaths {
		for metadata, message := range map[string]string{
			`null`:                     "metadata must not be null",
			`{"labels":null}`:          "metadata.labels must not be null",
			`{"annotations":null}`:     "metadata.annotations must not be null",
			`{"labels":{"k":"bad v"}}`: "metadata.labels contains an invalid value",
		} {
			t.Run(path+"/"+metadata, func(t *testing.T) {
				f := newAgentMetadataFixture(t)
				status, body, _ := f.agentWrite(t, path, metadata)
				assert.Equal(t, http.StatusBadRequest, status)
				assert.Equal(t, "validation failed", body["error"])
				assert.Equal(t, message, body["message"])
				after, err := f.agents.List(context.Background())
				require.NoError(t, err)
				seeded := 1
				if path == "POST" {
					seeded = 0
				}
				assert.Len(t, after, seeded, "no agent is created")
				for _, agent := range after {
					assert.Equal(t, map[string]string{"stored": "value"}, agent.ObjectMetadata.Labels(), "stored metadata is unchanged")
				}
			})
		}
	}
}

func TestAgentMetadataResponsePresentation(t *testing.T) {
	f := newAgentMetadataFixture(t)
	created := f.do(http.MethodPost, "/api/agents", agentBody(f.permissionSetID, "", `{"labels":{"z":"1","B":"2","a":"3"},"annotations":{"y":"x"}}`))
	require.Equal(t, http.StatusCreated, created.Code)
	assert.Contains(t, created.Body.String(), `"metadata":{"labels":{"B":"2","a":"3","z":"1"},"annotations":{"y":"x"}}`)
	list := f.do(http.MethodGet, "/api/agents", "")
	assert.Contains(t, list.Body.String(), `"metadata":{"labels":{"B":"2","a":"3","z":"1"},"annotations":{"y":"x"}}`)
}

func TestAgentMetadataErrorPrecedence(t *testing.T) {
	f := newAgentMetadataFixture(t)
	t.Run("an undecodable body keeps its existing error", func(t *testing.T) {
		recorder := f.do(http.MethodPost, "/api/agents", `{"display_name":1,"metadata":null}`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "invalid request body", f.decode(recorder)["error"])
	})
	t.Run("client_id keeps its existing error on collection PUT", func(t *testing.T) {
		recorder := f.do(http.MethodPut, "/api/agents?external_id=a", withMetadata(strings.TrimSuffix(agentBody(f.permissionSetID, "", ""), "}")+`,"client_id":""}`, `null`))
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "client_id must be a nonempty string when supplied", f.decode(recorder)["message"])
	})
	t.Run("metadata is checked before permission sets are resolved", func(t *testing.T) {
		body := withMetadata(agentBody(id.NewPermissionSetID().String(), "", ""), `null`)
		recorder := f.do(http.MethodPost, "/api/agents", body)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		assert.Equal(t, "metadata must not be null", f.decode(recorder)["message"])
	})
}

func TestAgentPostKeepsDecodeBehavior(t *testing.T) {
	f := newAgentMetadataFixture(t)
	t.Run("trailing bytes after the request object are ignored", func(t *testing.T) {
		recorder := f.do(http.MethodPost, "/api/agents", agentBody(f.permissionSetID, "", `{"labels":{"a":"1"}}`)+` {"metadata":null} trailing`)
		require.Equal(t, http.StatusCreated, recorder.Code, recorder.Body.String())
		assert.Equal(t, map[string]any{"labels": map[string]any{"a": "1"}}, f.decode(recorder)["metadata"])
	})
	t.Run("decode errors keep their codes and messages", func(t *testing.T) {
		recorder := f.do(http.MethodPost, "/api/agents", `{"display_name":`)
		assert.Equal(t, http.StatusBadRequest, recorder.Code)
		body := f.decode(recorder)
		assert.Equal(t, "invalid request body", body["error"])
		assert.Equal(t, "unexpected EOF", body["message"])
	})
}

func TestAgentPutBodyCap(t *testing.T) {
	for _, path := range []string{"PUT", "collection PUT"} {
		t.Run(path, func(t *testing.T) {
			f := newAgentMetadataFixture(t)
			created := f.createAgent(f.permissionSetID, "")
			target := "/api/agents/" + created["id"].(string)
			if path == "collection PUT" {
				target = "/api/agents?external_id=cap-" + id.NewAgentID().String()
			}
			definition := agentBody(f.permissionSetID, "", `{"annotations":{"k":"v"}}`)
			padding := maxAgentWriteBody - len(definition) - len(`,"padding":""`)
			accepted := strings.TrimSuffix(definition, "}") + `,"padding":"` + strings.Repeat("a", padding) + `"}`
			require.Len(t, accepted, maxAgentWriteBody)
			recorder := f.do(http.MethodPut, target, accepted)
			assert.Contains(t, []int{http.StatusOK, http.StatusCreated}, recorder.Code, recorder.Body.String())
			rejected := strings.TrimSuffix(definition, "}") + `,"padding":"` + strings.Repeat("a", padding+1) + `"}`
			recorder = f.do(http.MethodPut, target, rejected)
			assert.Equal(t, http.StatusBadRequest, recorder.Code)
			assert.Equal(t, "invalid request body", f.decode(recorder)["error"])
		})
	}
}
