package enduser

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

func TestOAuth2TokenProxy_RejectsOversizedBufferedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"token","token_type":"Bearer"}`)
		_, _ = io.WriteString(w, strings.Repeat(" ", 1<<20))
	}))
	defer server.Close()

	proxy := NewOAuth2TokenProxy(server.URL, server.Client())
	response, failure, err := proxy.Exchange(context.Background(), "grant_type=client_credentials", "application/x-www-form-urlencoded")
	require.NoError(t, err)
	require.Equal(t, ports.TokenProxyNoFailure, failure)
	defer func() { require.NoError(t, response.CloseBody()) }()

	body, err := response.ReadBody()
	require.ErrorContains(t, err, "exceeds")
	require.Nil(t, body)
}
