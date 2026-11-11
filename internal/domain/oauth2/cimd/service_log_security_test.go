package cimd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	queryClientURL = "https://agent.example.com/client?marker=query-secret"
)

func TestService_Resolve_LogsNoURLQueryOrWrappedCause(t *testing.T) {
	leakyCause := &url.Error{Op: "Get", URL: queryClientURL, Err: errors.New("dial tcp: connection refused")}

	tests := []struct {
		name      string
		fetcher   *mockFetcher
		wantKind  string
		wantLevel string
	}{
		{
			name:     "fetch blocked by SSRF protection",
			fetcher:  &mockFetcher{err: fmt.Errorf("fetching CIMD document: %w", &url.Error{Op: "Get", URL: queryClientURL, Err: &ports.SSRFBlockedError{IP: "10.0.0.1"}})},
			wantKind: ErrorKindSSRFBlocked,
		},
		{
			name:     "fetch unavailable",
			fetcher:  &mockFetcher{err: &ports.CIMDUnavailableError{Cause: leakyCause}},
			wantKind: ErrorKindUnavailable,
		},
		{
			name:     "fetch failed",
			fetcher:  &mockFetcher{err: fmt.Errorf("building CIMD request: %w", leakyCause)},
			wantKind: ErrorKindFetchFailed,
		},
		{
			name: "document invalid",
			fetcher: &mockFetcher{result: &ports.CIMDFetchResult{
				Body: []byte(`{"client_id":"https://agent.example.com/other?marker=query-secret","redirect_uris":["https://agent.example.com/cb?marker=query-secret"]}`),
			}},
			wantKind: ErrorKindInvalidDocument,
		},
		{
			name: "private-use redirect not allowlisted",
			fetcher: &mockFetcher{result: &ports.CIMDFetchResult{
				Body: []byte(`{"client_id":"` + queryClientURL + `","redirect_uris":["github-app://oauth/callback?marker=query-secret"]}`),
			}},
			wantKind: ErrorKindRedirectURINotAllowlisted,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var logs bytes.Buffer
			svc := NewService(test.fetcher, mustNewCIMDCache(t, 60*time.Second, time.Hour), nil,
				slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
			agent := testAgent(id.NewAgentID())

			_, err := svc.Resolve(context.Background(), queryClientURL, agent)
			require.Error(t, err)

			out := logs.String()
			assert.NotContains(t, out, "query-secret")
			assert.NotContains(t, out, "connection refused")
			assert.Contains(t, out, `"error_kind":"`+test.wantKind+`"`)
			assert.NotContains(t, out, `"url"`)
			assert.Contains(t, out, `"agent_id":"`+agent.ID.String()+`"`)
		})
	}
}

func TestService_Resolve_SuccessLogsNoURLQuery(t *testing.T) {
	var logs bytes.Buffer
	cache := mustNewCIMDCache(t, 60*time.Second, time.Hour)
	svc := NewService(&mockFetcher{result: cimdFetchResult(t, queryClientURL, "none")}, cache, nil,
		slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	agent := testAgent(id.NewAgentID())

	for range 2 {
		_, err := svc.Resolve(context.Background(), queryClientURL, agent)
		require.NoError(t, err)
	}

	out := logs.String()
	assert.NotContains(t, out, "query-secret")
	assert.Contains(t, out, "cimd_document_fetched")
	assert.Contains(t, out, "cimd_document_cache_hit")
	assert.Contains(t, out, "client_id URL contains query string")
	assert.NotContains(t, out, `"url"`)
	assert.Contains(t, out, `"agent_id":"`+agent.ID.String()+`"`)
}
