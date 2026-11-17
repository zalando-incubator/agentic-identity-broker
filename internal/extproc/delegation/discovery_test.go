package delegation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryURL_InsertsWellKnownBeforePath(t *testing.T) {
	t.Parallel()
	for pdp, want := range map[string]string{
		"https://broker.example.com":          "https://broker.example.com/.well-known/authzen-configuration",
		"https://broker.example.com/":         "https://broker.example.com/.well-known/authzen-configuration",
		"https://example.com:8443/tenant1":    "https://example.com:8443/.well-known/authzen-configuration/tenant1",
		"https://example.com/broker/tenant1/": "https://example.com/.well-known/authzen-configuration/broker/tenant1",
	} {
		got, err := DiscoveryURL(pdp)
		require.NoError(t, err)
		assert.Equal(t, want, got, pdp)
	}
}

func TestDiscover(t *testing.T) {
	t.Parallel()
	var srvURL string
	tests := []struct {
		name      string
		status    int
		body      func() string
		allowHTTP bool
		errText   string
	}{
		{name: "valid", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation","unknown":1}`
		}},
		{name: "unavailable", status: 503, allowHTTP: true, body: func() string { return `{}` }, errText: "delegation context discovery failed"},
		{name: "invalid JSON", status: 200, allowHTTP: true, body: func() string { return `{` }, errText: "delegation context discovery failed"},
		{name: "missing endpoint", status: 200, allowHTTP: true, body: func() string { return `{"policy_decision_point":"` + srvURL + `"}` }, errText: "delegation context discovery failed"},
		{name: "PDP mismatch", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"https://evil.example.com","access_evaluation_endpoint":"https://evil.example.com/access/v1/evaluation"}`
		}, errText: "policy_decision_point mismatch (expected " + "SRV" + ", advertised https://evil.example.com)"},
		{name: "PDP path differs", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `/","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"}`
		}, errText: "policy_decision_point mismatch"},
		{name: "relative endpoint", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"/access/v1/evaluation"}`
		}, errText: "invalid access_evaluation_endpoint"},
		{name: "userinfo endpoint", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"http://u:p@127.0.0.1/x"}`
		}, errText: "invalid access_evaluation_endpoint"},
		{name: "http endpoint without allow_http", status: 200, allowHTTP: false, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"}`
		}, errText: "invalid access_evaluation_endpoint"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sawAuthorization atomic.Bool
			var path atomic.Value
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				path.Store(r.URL.Path)
				if r.Header.Get("Authorization") != "" {
					sawAuthorization.Store(true)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body()))
			}))
			defer srv.Close()
			srvURL = srv.URL

			metadata, err := Discover(context.Background(), srv.Client(), srv.URL, tt.allowHTTP)

			assert.False(t, sawAuthorization.Load(), "discovery never sends credentials")
			assert.Equal(t, "/.well-known/authzen-configuration", path.Load())
			if tt.errText == "" {
				require.NoError(t, err)
				assert.Equal(t, srv.URL+"/access/v1/evaluation", metadata.AccessEvaluationEndpoint.String())
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), strings.ReplaceAll(tt.errText, "SRV", srv.URL))
		})
	}
}
