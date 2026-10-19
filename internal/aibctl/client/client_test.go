package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/aibctl/client"
)

type headerAuth struct{}

func (headerAuth) Apply(_ context.Context, req *http.Request) error {
	req.Header.Set("Authorization", "Bearer tok")
	req.Header.Set("X-Remote-User", "admin@example.com")
	return nil
}

type recorded struct {
	method, path, host, contentType, prefer, auth, principal, ifMatch, tenant string
	body                                                                      string
}

func newServer(t *testing.T, status int, body string, rec *recorded) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := readAll(r)
		*rec = recorded{
			method: r.Method, path: r.URL.RequestURI(), host: r.Host, contentType: r.Header.Get("Content-Type"),
			prefer: r.Header.Get("Prefer"), auth: r.Header.Get("Authorization"), principal: r.Header.Get("X-Remote-User"),
			ifMatch: r.Header.Get("If-Match"), tenant: r.Header.Get("X-Tenant"), body: data,
		}
		if strings.HasPrefix(body, "{") || strings.HasPrefix(body, "[") {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/plain")
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
}

func readAll(r *http.Request) (string, error) {
	var b strings.Builder
	buf := make([]byte, 1024)
	for {
		n, err := r.Body.Read(buf)
		b.Write(buf[:n])
		if err != nil {
			return b.String(), nil
		}
	}
}

func TestHealthSendsPreferAndAuth(t *testing.T) {
	var rec recorded
	srv := newServer(t, 200, `{"status":"healthy","timestamp":"t","uptime_seconds":12}`, &rec)
	defer srv.Close()

	c := &client.Client{BaseURL: srv.URL + "/", Auth: headerAuth{}, HostHeader: "admin.internal", Headers: map[string]string{"X-Tenant": "p"}}
	h, err := c.Health(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "healthy", h.Status)
	assert.Equal(t, int64(12), h.UptimeSeconds)
	assert.Equal(t, "/health", rec.path)
	assert.Equal(t, "reference-id=canonical", rec.prefer)
	assert.Equal(t, "Bearer tok", rec.auth)
	assert.Equal(t, "admin@example.com", rec.principal)
	assert.Equal(t, "admin.internal", rec.host)
	assert.Equal(t, "p", rec.tenant)
	assert.Empty(t, rec.contentType, "GET carries no Content-Type")
}

func TestJSONErrorMapping(t *testing.T) {
	var rec recorded
	srv := newServer(t, 404, `{"error":"not_found","message":"agent not found"}`, &rec)
	defer srv.Close()

	c := &client.Client{BaseURL: srv.URL}
	_, err := c.Health(context.Background())
	require.Error(t, err)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, 404, apiErr.Status)
	assert.Equal(t, "not_found", apiErr.Code)
	assert.Equal(t, "agent not found (not_found, HTTP 404)", err.Error())
	assert.True(t, client.IsStatus(err, 404))
}

func TestPlainTextErrorMapping(t *testing.T) {
	var rec recorded
	srv := newServer(t, 403, "forbidden host\n", &rec)
	defer srv.Close()

	c := &client.Client{BaseURL: srv.URL}
	_, err := c.Health(context.Background())
	require.Error(t, err)
	var apiErr *client.APIError
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "forbidden host", apiErr.Message)
	assert.Contains(t, err.Error(), "host_header")
}

func TestNullableStringJSON(t *testing.T) {
	type doc struct {
		A client.NullableString `json:"a,omitzero"`
		B client.NullableString `json:"b,omitzero"`
		C client.NullableString `json:"c,omitzero"`
	}
	var d doc
	require.NoError(t, json.Unmarshal([]byte(`{"a":"x","b":null}`), &d))
	assert.True(t, d.A.Set)
	assert.Equal(t, "x", d.A.Value)
	assert.True(t, d.B.Set)
	assert.True(t, d.B.Null)
	assert.False(t, d.C.Set)

	out, err := json.Marshal(d)
	require.NoError(t, err)
	assert.JSONEq(t, `{"a":"x","b":null}`, string(out))
}

func TestAgentWriteRoundTrip(t *testing.T) {
	cid := "research-agent"
	a := client.Agent{ID: "u", CanonicalID: &cid, DisplayName: "R", ServiceRequirements: []client.ServiceRequirement{{ServiceID: "svc", ServiceName: "GitHub", RequirementType: "mandatory", RequiredScopes: []string{"read"}}}, CreatedAt: "c"}
	out, err := json.Marshal(a.Write())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.Equal(t, "research-agent", m["canonical_id"])
	assert.NotContains(t, m, "id")
	assert.NotContains(t, m, "created_at")
	assert.NotContains(t, m, "client_id")
	sr := m["service_requirements"].([]any)[0].(map[string]any)
	assert.NotContains(t, sr, "service_name")
}

func TestServiceWriteDropsRedactedSecret(t *testing.T) {
	secret := client.RedactedSecret
	s := client.Service{DisplayName: "S", ClientSecret: &secret, Endpoints: client.Endpoints{TokenEndpoint: "t", AuthorizeEndpoint: "a"}}
	out, err := json.Marshal(s.Write())
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(out, &m))
	assert.NotContains(t, m, "client_secret")
	assert.Equal(t, "t", m["endpoints"].(map[string]any)["token_endpoint"])
	assert.Equal(t, []any{}, m["scopes"])
}
