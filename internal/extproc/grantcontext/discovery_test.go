package grantcontext

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryURL_InsertsWellKnownBeforePath(t *testing.T) {
	t.Parallel()
	for pdp, want := range map[string]string{
		"https://broker.example.com":           "https://broker.example.com/.well-known/authzen-configuration",
		"https://broker.example.com/":          "https://broker.example.com/.well-known/authzen-configuration",
		"https://example.com:8443/tenant1":     "https://example.com:8443/.well-known/authzen-configuration/tenant1",
		"https://example.com/broker/tenant1/":  "https://example.com/.well-known/authzen-configuration/broker/tenant1",
		"https://[fe80::1%25eth0]:8443/broker": "https://[fe80::1%25eth0]:8443/.well-known/authzen-configuration/broker",
	} {
		t.Run(pdp, func(t *testing.T) {
			t.Parallel()
			got, err := DiscoveryURL(pdp)
			require.NoError(t, err)
			assert.Equal(t, want, got)
		})
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
		{name: "unavailable", status: 503, allowHTTP: true, body: func() string { return `{}` }, errText: "grant context discovery failed"},
		{name: "invalid JSON", status: 200, allowHTTP: true, body: func() string { return `{` }, errText: "grant context discovery failed"},
		{name: "trailing JSON value", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"}{}`
		}, errText: "grant context discovery failed"},
		{name: "trailing invalid bytes", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"} x`
		}, errText: "grant context discovery failed"},
		{name: "oversized document with valid prefix", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"}` +
				strings.Repeat(" ", maxMetadataBytes)
		}, errText: "grant context discovery failed"},
		{name: "missing endpoint", status: 200, allowHTTP: true, body: func() string { return `{"policy_decision_point":"` + srvURL + `"}` }, errText: "grant context discovery failed"},
		{name: "PDP mismatch", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"https://evil.example.com","access_evaluation_endpoint":"https://evil.example.com/access/v1/evaluation"}`
		}, errText: "policy_decision_point mismatch (expected " + "SRV" + ", advertised https://evil.example.com)"},
		{name: "PDP mismatch escapes control characters", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"https://evil.example.com\nlevel=ERROR msg=forged","access_evaluation_endpoint":"https://evil.example.com/access/v1/evaluation"}`
		}, errText: `advertised https://evil.example.com\nlevel=ERROR msg=forged)`},
		{name: "PDP path differs", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `/","access_evaluation_endpoint":"` + srvURL + `/access/v1/evaluation"}`
		}, errText: "policy_decision_point mismatch"},
		{name: "relative endpoint", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"/access/v1/evaluation"}`
		}, errText: "invalid access_evaluation_endpoint"},
		{name: "port-only endpoint authority", status: 200, allowHTTP: true, body: func() string {
			return `{"policy_decision_point":"` + srvURL + `","access_evaluation_endpoint":"https://:8443/access/v1/evaluation"}`
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

func TestDiscover_CallerCancellationIsTyped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cancel()
		<-r.Context().Done()
	}))
	defer srv.Close()
	// The client timeout bounds the test if the caller context stops reaching the request;
	// the result is then a timeout instead of context.Canceled.
	client := &http.Client{Timeout: 10 * time.Second, Transport: srv.Client().Transport}

	_, err := Discover(ctx, client, srv.URL, true)

	require.ErrorIs(t, err, ErrDiscoveryFailed)
	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), srv.URL, "the raw transport error is not exposed")
}
