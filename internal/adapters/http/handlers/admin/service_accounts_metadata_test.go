package admin

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func (h *saHandlerHarness) createWithMetadata(t *testing.T, canonicalID, metadata string) map[string]any {
	t.Helper()
	jwt := h.jwt(t, "2026-11", time.Now().Add(time.Hour))
	w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, withMetadata(createBody(canonicalID, jwt), metadata))
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
	return decodeObject(t, w)
}

func TestServiceAccountsHandler_CreateMetadata(t *testing.T) {
	for name, tc := range map[string]struct {
		metadata string
		want     any
	}{
		"omitted is empty":      {"", nil},
		"empty object is empty": {`{}`, nil},
		"null maps are empty":   {`{"labels":null,"annotations":null}`, nil},
		"labels only":           {`{"labels":{"tier":"gold"}}`, map[string]any{"labels": map[string]any{"tier": "gold"}}},
		"both":                  {`{"labels":{"b":"2","a":"1"},"annotations":{"note":""}}`, map[string]any{"labels": map[string]any{"a": "1", "b": "2"}, "annotations": map[string]any{"note": ""}}},
		"empty map is omitted":  {`{"labels":{},"annotations":{"note":"x"}}`, map[string]any{"annotations": map[string]any{"note": "x"}}},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSAHandlerHarness(t)
			created := h.createWithMetadata(t, "platform-triage-bot", tc.metadata)
			if tc.want == nil {
				assert.NotContains(t, created, "metadata")
			} else {
				assert.Equal(t, tc.want, created["metadata"])
			}
			read := h.do(t, http.MethodGet, h.path("company-platform", "/platform-triage-bot"), saTestOperator, nil, "")
			assert.Equal(t, created["metadata"], decodeObject(t, read)["metadata"])
		})
	}
}

func TestServiceAccountsHandler_MetadataPresentation(t *testing.T) {
	h := newSAHandlerHarness(t)
	h.createWithMetadata(t, "platform-triage-bot", `{"labels":{"z":"1","B":"2","a":"3"},"annotations":{"y":"x"}}`)
	want := `"metadata":{"labels":{"B":"2","a":"3","z":"1"},"annotations":{"y":"x"}}`
	for _, path := range []string{h.path("company-platform"), h.path("company-platform", "/platform-triage-bot")} {
		w := h.do(t, http.MethodGet, path, saTestOperator, nil, "")
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), want)
	}
	status := h.do(t, http.MethodGet, h.path("company-platform", "/platform-triage-bot/status"), saTestOperator, nil, "")
	assert.NotContains(t, status.Body.String(), "metadata", "the status resource carries no metadata")
}

func TestServiceAccountsHandler_UpdateMetadata(t *testing.T) {
	stored := `{"labels":{"tier":"gold"},"annotations":{"note":"kept"}}`
	storedWant := map[string]any{"labels": map[string]any{"tier": "gold"}, "annotations": map[string]any{"note": "kept"}}
	for name, tc := range map[string]struct {
		body    string
		want    any
		version string
	}{
		"omitted keeps the stored value":       {`{"scopes":["read"]}`, storedWant, `"1"`},
		"omitted with another change keeps it": {`{"scopes":["read","write"]}`, storedWant, `"2"`},
		"equal in another key order is a no-op": {
			`{"scopes":["read"],"metadata":{"annotations":{"note":"kept"},"labels":{"tier":"gold"}}}`, storedWant, `"1"`,
		},
		"empty object clears it":        {`{"scopes":["read"],"metadata":{}}`, nil, `"2"`},
		"sent maps replace both":        {`{"scopes":["read"],"metadata":{"labels":{"tier":"silver"}}}`, map[string]any{"labels": map[string]any{"tier": "silver"}}, `"2"`},
		"null map clears only that one": {`{"scopes":["read"],"metadata":{"labels":null,"annotations":{"note":"kept"}}}`, map[string]any{"annotations": map[string]any{"note": "kept"}}, `"2"`},
	} {
		t.Run(name, func(t *testing.T) {
			h := newSAHandlerHarness(t)
			h.createWithMetadata(t, "platform-triage-bot", stored)
			w := h.do(t, http.MethodPut, h.path("company-platform", "/platform-triage-bot"), saTestOperator, map[string]string{"If-Match": `"1"`}, tc.body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
			assert.Equal(t, tc.version, w.Header().Get("ETag"))
			body := decodeObject(t, w)
			if tc.want == nil {
				assert.NotContains(t, body, "metadata")
			} else {
				assert.Equal(t, tc.want, body["metadata"])
			}
			read := h.do(t, http.MethodGet, h.path("company-platform", "/platform-triage-bot"), saTestOperator, nil, "")
			assert.Equal(t, body["metadata"], decodeObject(t, read)["metadata"])
		})
	}
}

func TestServiceAccountsHandler_MetadataRejectedOnEveryWritePath(t *testing.T) {
	paths := map[string]struct {
		method     string
		path       string
		definition func(h *saHandlerHarness, t *testing.T) string
	}{
		"POST": {http.MethodPost, "", func(h *saHandlerHarness, t *testing.T) string {
			return createBody("platform-planner-bot", h.jwt(t, "k", time.Now().Add(time.Hour)))
		}},
		"PUT": {http.MethodPut, "/platform-triage-bot", func(h *saHandlerHarness, t *testing.T) string {
			return `{"canonical_id":"platform-triage","scopes":["admin"],"credential":{"format":"signed_jwt","jwt":"` + h.jwt(t, "k2", time.Now().Add(time.Hour)) + `"}}`
		}},
	}
	for pathName, path := range paths {
		for _, row := range metadataRejections() {
			t.Run(pathName+"/"+row.name, func(t *testing.T) {
				h := newSAHandlerHarness(t)
				before := h.createWithMetadata(t, "platform-triage-bot", `{"labels":{"stored":"value"}}`)
				h.logs.Reset()
				calls := h.repo.calls

				w := h.do(t, path.method, h.path("company-platform", path.path), saTestOperator, nil, row.body(path.definition(h, t)))

				assertErrorBody(t, w, http.StatusBadRequest, "validation failed", row.message)
				assert.NotContains(t, w.Body.String(), metadataSentinel)
				assert.NotContains(t, h.logs.String(), metadataSentinel)
				assert.NotContains(t, h.logs.String(), `"event"`, "no audit or credential record")
				resolveCalls := 0
				if path.method == http.MethodPut {
					resolveCalls = 1
				}
				assert.Equal(t, calls+resolveCalls, h.repo.calls, "no write and no domain read")
				list := h.do(t, http.MethodGet, h.path("company-platform"), saTestOperator, nil, "")
				var after []map[string]any
				require.NoError(t, json.Unmarshal(list.Body.Bytes(), &after))
				assert.Equal(t, []map[string]any{before}, after, "nothing changes")
				h.assertNoCredential(t, w)
			})
		}
	}
}

func TestServiceAccountsHandler_MetadataErrorPrecedence(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	t.Run("an undecodable body keeps its existing error", func(t *testing.T) {
		w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, `{"canonical_id":1,"metadata":null}`)
		assert.Equal(t, "invalid request body", decodeObject(t, w)["error"])
	})
	t.Run("an unknown member keeps its existing error", func(t *testing.T) {
		w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, withMetadata(strings.TrimSuffix(createBody("a", jwt), "}")+`,"labels":{}}`, `null`))
		assert.Equal(t, "invalid request body", decodeObject(t, w)["error"])
	})
	t.Run("a null member keeps its existing error", func(t *testing.T) {
		w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, `{"canonical_id":null,"kind":"jwt_bearer","scopes":["read"],"metadata":null}`)
		assertErrorBody(t, w, http.StatusBadRequest, "validation failed", "canonical_id must not be null")
	})
	t.Run("metadata is checked before the service account rules", func(t *testing.T) {
		w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, `{"canonical_id":"a","kind":"client_credentials","scopes":[],"metadata":null}`)
		assertErrorBody(t, w, http.StatusBadRequest, "validation failed", "metadata must not be null")
	})
	assert.Zero(t, h.repo.calls)
}

func TestServiceAccountsHandler_BodyCapFitsLargestMetadata(t *testing.T) {
	h := newSAHandlerHarness(t)
	jwt := h.jwt(t, "k", time.Now().Add(time.Hour))
	annotations := `{"annotations":{"k":"` + strings.Repeat(`a`, 65535) + `"}}`
	accepted := withMetadata(createBody("platform-triage-bot", jwt), annotations)
	require.Greater(t, len(accepted), 64<<10)
	w := h.do(t, http.MethodPost, h.path("company-platform"), saTestOperator, nil, accepted)
	require.Equal(t, http.StatusCreated, w.Code, w.Body.String())

	padding := serviceAccountMaxBodyBytes - len(`{"scopes":["read"],"padding":""}`)
	rejected := `{"scopes":["read"],"padding":"` + strings.Repeat("a", padding+1) + `"}`
	w = h.do(t, http.MethodPut, h.path("company-platform", "/platform-triage-bot"), saTestOperator, nil, rejected)
	assertErrorBody(t, w, http.StatusBadRequest, "invalid request body", "request body must be one JSON object of at most 512 KiB with known members")
}
