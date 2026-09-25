package httpclient

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewRejectsRedirects(t *testing.T) {
	var redirectRequests atomic.Int32
	redirect := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		redirectRequests.Add(1)
	}))
	defer redirect.Close()

	broker := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, redirect.URL, http.StatusFound)
	}))
	defer broker.Close()

	client, err := New(&extprocconfig.Config{}, time.Second)
	require.NoError(t, err)

	response, err := client.Get(broker.URL)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	assert.Equal(t, http.StatusFound, response.StatusCode)
	assert.Zero(t, redirectRequests.Load())
}
