package jwtauth

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jwx-go/jwkfetch/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJWXAuthenticator_JWKSFetchFailureLogsNoURLQueryOrCause(t *testing.T) {
	_, jwksBytes, signingKey := generateTestKeyPair(t)
	server := startMockJWKSServer(t, jwksBytes)
	auth := newTestAuthenticator(t, server.URL, signingKey, nil)

	unreachable := httptest.NewServer(http.NotFoundHandler())
	unreachableURL := unreachable.URL + "/jwks.json?tenant=query-secret"
	unreachable.Close()
	require.NoError(t, auth.jwksCache.Register(context.Background(), unreachableURL,
		jwkfetch.WithMinInterval(time.Hour), jwkfetch.WithWaitReady(false)))
	auth.jwksURL = unreachableURL

	var logs bytes.Buffer
	auth.logger = slog.New(slog.NewJSONHandler(&logs, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := auth.Authenticate(ctx, createSignedJWT(t, map[string]interface{}{"sub": "user"}, signingKey))
	require.Error(t, err)

	out := logs.String()
	assert.NotContains(t, out, "query-secret")
	assert.NotContains(t, out, "connection refused")
	assert.Contains(t, out, `"url":"`+unreachable.URL+`/jwks.json"`)
	assert.Contains(t, out, `"error_kind":`)
}
