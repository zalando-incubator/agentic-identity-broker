package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	encryptionnoop "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/encryption/noop"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/storage/memory"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/thirdparty"
)

func TestAdminServiceIconLifecycle(t *testing.T) {
	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	repo := memory.NewInMemoryThirdpartyOAuth2ProviderRepository()
	svc := thirdparty.NewThirdpartyOAuth2ProviderService(repo, newTestEncryption(), &encryptionnoop.BranchKeyManager{}, nil, false, logger)
	handler := NewServicesHandler(svc, testConfig(), logger)
	router := chi.NewRouter()
	router.Post("/api/services", handler.CreateService)
	router.Get("/api/services", handler.ListServices)
	router.Get("/api/services/{service-id}", handler.GetService)
	router.Put("/api/services/{service-id}", handler.UpdateService)
	request := func(method, path string, body map[string]any, etag string) *httptest.ResponseRecorder {
		t.Helper()
		wire, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, path, bytes.NewReader(wire))
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}
	decode := func(response *httptest.ResponseRecorder) map[string]any {
		t.Helper()
		var value map[string]any
		require.NoError(t, json.Unmarshal(response.Body.Bytes(), &value))
		return value
	}
	original := `<svg xmlns="http://www.w3.org/2000/svg"><text>SVG_SENTINEL 星</text></svg>`
	replacement := `<svg xmlns="http://www.w3.org/2000/svg"><circle r="8"/></svg>`
	body := map[string]any{
		"display_name": "Atlas", "client_id": "atlas-client", "client_secret": "SECRET_SENTINEL",
		"issuer_uri": "https://atlas.example", "discovery": map[string]any{"enable_discovery": false},
		"endpoints": map[string]any{"token_endpoint": "https://atlas.example/token", "authorize_endpoint": "https://atlas.example/authorize"},
		"scopes":    []any{map[string]any{"scope_value": "read", "description": "Read"}}, "icon_svg": original,
	}
	created := request(http.MethodPost, "/api/services", body, "")
	require.Equal(t, http.StatusCreated, created.Code, created.Body.String())
	value := decode(created)
	require.Equal(t, original, value["icon_svg"])
	serviceID := value["id"].(string)
	path := "/api/services/" + serviceID
	read := request(http.MethodGet, path, nil, "")
	require.Equal(t, http.StatusOK, read.Code)
	assert.Equal(t, original, decode(read)["icon_svg"])
	list := request(http.MethodGet, "/api/services", nil, "")
	require.Equal(t, http.StatusOK, list.Code)
	assert.Contains(t, list.Body.String(), "icon_svg")
	delete(body, "icon_svg")
	preserved := request(http.MethodPut, path, body, "")
	require.Equal(t, http.StatusOK, preserved.Code, preserved.Body.String())
	assert.Equal(t, original, decode(preserved)["icon_svg"])
	body["icon_svg"] = replacement
	replaced := request(http.MethodPut, path, body, "")
	require.Equal(t, http.StatusOK, replaced.Code, replaced.Body.String())
	assert.Equal(t, replacement, decode(replaced)["icon_svg"])
	staleETag := read.Header().Get("ETag")
	require.NotEmpty(t, staleETag)
	body["protected_resources"] = []string{}
	stale := request(http.MethodPut, path, body, staleETag)
	assert.Equal(t, http.StatusPreconditionFailed, stale.Code)
	delete(body, "protected_resources")
	parsedID, err := id.ParseServiceID(serviceID)
	require.NoError(t, err)
	before, err := repo.Get(context.Background(), parsedID)
	require.NoError(t, err)
	for _, invalid := range []any{
		"", "  ", "/atlas.svg", "https://images.invalid/atlas.svg", 3, []any{}, map[string]any{},
		`<svg xmlns="http://www.w3.org/2000/svg" onload="SVG_SENTINEL"/>`,
		"<svg xmlns=\"http://www.w3.org/2000/svg\"><!--\x00--></svg>",
	} {
		body["icon_svg"], body["display_name"], body["client_secret"] = invalid, "must-not-change", "must-not-store"
		response := request(http.MethodPut, path, body, "")
		require.Equal(t, http.StatusBadRequest, response.Code, response.Body.String())
		assert.Contains(t, decode(response), "error")
		after, err := repo.Get(context.Background(), before.ID)
		require.NoError(t, err)
		assert.Equal(t, before, after)
	}
	body["display_name"], body["client_secret"], body["icon_svg"] = "Atlas", "SECRET_SENTINEL", nil
	cleared := request(http.MethodPut, path, body, "")
	require.Equal(t, http.StatusOK, cleared.Code, cleared.Body.String())
	assert.NotContains(t, decode(cleared), "icon_svg")
	assert.NotContains(t, decode(request(http.MethodGet, path, nil, "")), "icon_svg")
	assert.NotContains(t, logs.String(), "SVG_SENTINEL")
	assert.NotContains(t, logs.String(), "SECRET_SENTINEL")
}

func TestServiceRequestIconPresence(t *testing.T) {
	source := `<svg xmlns="http://www.w3.org/2000/svg"/>`
	wire, err := json.Marshal(source)
	require.NoError(t, err)
	for _, tc := range []struct {
		name     string
		body     string
		value    *string
		provided bool
	}{
		{name: "omitted", body: `{}`},
		{name: "clear", body: `{"icon_svg":null}`, provided: true},
		{name: "replace", body: `{"icon_svg":` + string(wire) + `}`, value: &source, provided: true},
		{name: "later null clears", body: `{"icon_svg":` + string(wire) + `,"ICON_SVG":null}`, provided: true},
		{name: "later string replaces", body: `{"icon_svg":null,"ICON_SVG":` + string(wire) + `}`, value: &source, provided: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req ServiceRequest
			require.NoError(t, json.Unmarshal([]byte(tc.body), &req))
			assert.Equal(t, tc.provided, req.iconSVGProvided)
			assert.Equal(t, tc.value, req.IconSVG)
		})
	}
}

func BenchmarkServiceRequestIconDecode(b *testing.B) {
	source := `<svg xmlns="http://www.w3.org/2000/svg"><text>` + strings.Repeat("x", 60000) + `</text></svg>`
	wire, err := json.Marshal(ServiceRequest{IconSVG: &source})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var req ServiceRequest
		if err := json.Unmarshal(wire, &req); err != nil {
			b.Fatal(err)
		}
	}
}
