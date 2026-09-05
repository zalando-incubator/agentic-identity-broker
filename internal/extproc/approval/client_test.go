package approval

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type assertionStub struct {
	value string
	err   error
}

func (s assertionStub) ClientAssertion() (string, error) {
	return s.value, s.err
}

func TestClientUsesEndpointSpecificCredentialsAndTargetedReadScope(t *testing.T) {
	approvedAt := time.Date(2026, time.September, 4, 10, 11, 12, 0, time.UTC)
	var createBody CreateRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/approvals":
			switch r.Method {
			case http.MethodGet:
				require.Equal(t, "Bearer assertion", r.Header.Get("Authorization"))
				require.Empty(t, r.Header.Get("X-Client-Assertion"))
				require.Equal(t, "alice", r.URL.Query().Get("principal"))
				require.Equal(t, []string{"session-a", "session-b"}, r.URL.Query()["agent_session_id"])
				require.Empty(t, r.Header.Get("If-None-Match"))
				require.Empty(t, r.Header.Get("X-Long-Poll-Timeout"))
				w.Header().Set("ETag", `"v4"`)
				_, _ = w.Write([]byte(`{"data":{"pairs":[{"principal":"alice","agent_id":"agent","approvals":[{"id":"approval","tool_pattern":"create_issue","params_pattern":{},"status":"approved","persistence":"permanent","approved_at":"2026-09-04T10:11:12Z"}]}]}}`))
			case http.MethodPost:
				require.Equal(t, "Bearer subject", r.Header.Get("Authorization"))
				require.Equal(t, "assertion", r.Header.Get("X-Client-Assertion"))
				require.NoError(t, json.NewDecoder(r.Body).Decode(&createBody))
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"data":{"approval_url":"https://broker.example/approvals/id"}}`))
			default:
				t.Fatalf("unexpected method: %s", r.Method)
			}
		case "/api/approvals/id/consume":
			require.Equal(t, http.MethodPost, r.Method)
			require.Equal(t, "Bearer subject", r.Header.Get("Authorization"))
			require.Empty(t, r.Header.Get("X-Client-Assertion"))
			require.Equal(t, int64(0), r.ContentLength)
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/", time.Second, assertionStub{value: "assertion"})
	require.NoError(t, err)
	pairs, etag, err := client.Read(context.Background(), "alice", []string{"session-a", "session-b"})
	require.NoError(t, err)
	require.Len(t, pairs, 1)
	require.Len(t, pairs[0].Approvals, 1)
	assert.Equal(t, "approval", pairs[0].Approvals[0].ID)
	require.NotNil(t, pairs[0].Approvals[0].ApprovedAt)
	assert.True(t, approvedAt.Equal(*pairs[0].Approvals[0].ApprovedAt))
	assert.Equal(t, `"v4"`, etag)

	request := CreateRequest{ToolName: "create_issue", Arguments: map[string]any{}, RiskLevel: "medium"}
	request.Metadata.Description = "Approval required for create_issue"
	approvalURL, err := client.Create(context.Background(), "subject", request)
	require.NoError(t, err)
	assert.Equal(t, "https://broker.example/approvals/id", approvalURL)
	assert.Equal(t, request, createBody)
	require.NoError(t, client.Consume(context.Background(), "subject", "id"))
}

func TestClientPollUsesLongPollHeadersAndPreservesETagOn304(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer assertion", r.Header.Get("Authorization"))
		require.Empty(t, r.URL.Query().Get("principal"))
		require.Equal(t, []string{"session-a"}, r.URL.Query()["agent_session_id"])
		require.Equal(t, `"v1"`, r.Header.Get("If-None-Match"))
		require.Equal(t, "30", r.Header.Get("X-Long-Poll-Timeout"))
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusNotModified)
	}))
	defer server.Close()

	client, err := NewClient(server.URL, time.Nanosecond, assertionStub{value: "assertion"})
	require.NoError(t, err)
	pairs, etag, unchanged, err := client.Poll(context.Background(), []string{"session-a"}, `"v1"`, 30*time.Second)
	require.NoError(t, err)
	assert.Nil(t, pairs)
	assert.Empty(t, etag)
	assert.True(t, unchanged)
}

func TestClientStatusHandling(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		method  string
		status  int
		success bool
	}{
		{name: "read OK", path: "/api/approvals", method: http.MethodGet, status: http.StatusOK, success: true},
		{name: "read rejects unauthorized", path: "/api/approvals", method: http.MethodGet, status: http.StatusUnauthorized},
		{name: "read rejects server error", path: "/api/approvals", method: http.MethodGet, status: http.StatusInternalServerError},
		{name: "create accepts created", path: "/api/approvals", method: http.MethodPost, status: http.StatusCreated, success: true},
		{name: "create accepts deduplicated", path: "/api/approvals", method: http.MethodPost, status: http.StatusOK, success: true},
		{name: "create rejects malformed request", path: "/api/approvals", method: http.MethodPost, status: http.StatusBadRequest},
		{name: "create rejects unauthorized", path: "/api/approvals", method: http.MethodPost, status: http.StatusUnauthorized},
		{name: "create rejects rate limit", path: "/api/approvals", method: http.MethodPost, status: http.StatusTooManyRequests},
		{name: "create rejects server error", path: "/api/approvals", method: http.MethodPost, status: http.StatusInternalServerError},
		{name: "consume accepts OK", path: "/api/approvals/id/consume", method: http.MethodPost, status: http.StatusOK, success: true},
		{name: "consume rejects forbidden", path: "/api/approvals/id/consume", method: http.MethodPost, status: http.StatusForbidden},
		{name: "consume rejects missing approval", path: "/api/approvals/id/consume", method: http.MethodPost, status: http.StatusNotFound},
		{name: "consume rejects non-once approval", path: "/api/approvals/id/consume", method: http.MethodPost, status: http.StatusUnprocessableEntity},
		{name: "consume rejects server error", path: "/api/approvals/id/consume", method: http.MethodPost, status: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				require.Equal(t, test.path, r.URL.Path)
				require.Equal(t, test.method, r.Method)
				if test.success && test.method == http.MethodGet {
					w.Header().Set("ETag", `"v1"`)
				}
				w.WriteHeader(test.status)
				if test.success && test.method == http.MethodGet {
					_, _ = w.Write([]byte(`{"data":{"pairs":[]}}`))
				}
				if test.success && test.path == "/api/approvals" && test.method == http.MethodPost {
					_, _ = w.Write([]byte(`{"data":{"approval_url":"https://broker.example/approval"}}`))
				}
			}))
			defer server.Close()

			client, err := NewClient(server.URL, time.Second, assertionStub{value: "assertion"})
			require.NoError(t, err)
			switch test.path {
			case "/api/approvals":
				if test.method == http.MethodGet {
					_, _, err = client.Read(context.Background(), "alice", nil)
				} else {
					_, err = client.Create(context.Background(), "subject", CreateRequest{ToolName: "tool", Arguments: map[string]any{}})
				}
			default:
				err = client.Consume(context.Background(), "subject", "id")
			}
			if test.success {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "HTTP ")
			}
		})
	}
}

func TestClientRejectsMalformedResponsesAndUnavailableCredentials(t *testing.T) {
	t.Run("constructor rejects unusable base URL or assertion provider", func(t *testing.T) {
		_, err := NewClient("", time.Second, assertionStub{value: "assertion"})
		require.Error(t, err)
		_, err = NewClient("/relative", time.Second, assertionStub{value: "assertion"})
		require.Error(t, err)
		_, err = NewClient("https://broker.example?query=forbidden", time.Second, assertionStub{value: "assertion"})
		require.Error(t, err)
		_, err = NewClient("https://broker.example", time.Second, nil)
		require.Error(t, err)
	})

	t.Run("rejects incomplete sync payload", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("ETag", `"v1"`)
			_, _ = w.Write([]byte(`{"data":{}}`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, time.Second, assertionStub{value: "assertion"})
		require.NoError(t, err)
		_, _, err = client.Read(context.Background(), "alice", nil)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pairs")
	})

	t.Run("rejects successful create without a URL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"data":{}}`))
		}))
		defer server.Close()
		client, err := NewClient(server.URL, time.Second, assertionStub{value: "assertion"})
		require.NoError(t, err)
		_, err = client.Create(context.Background(), "subject", CreateRequest{ToolName: "tool", Arguments: map[string]any{}})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "approval_url")
	})

	t.Run("does not send requests without a usable assertion", func(t *testing.T) {
		client, err := NewClient("https://broker.example", time.Second, assertionStub{err: errors.New("expired")})
		require.NoError(t, err)
		_, _, err = client.Read(context.Background(), "alice", nil)
		require.Error(t, err)
		assert.True(t, strings.Contains(err.Error(), "client assertion"))
	})
}
