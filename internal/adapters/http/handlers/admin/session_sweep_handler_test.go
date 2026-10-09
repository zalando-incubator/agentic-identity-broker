package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	brokerhttp "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSessionSweeper struct {
	calls []oauth2session.SweepRequest
	sweep func(context.Context, oauth2session.SweepRequest) (oauth2session.SweepResult, error)
}

func (f *fakeSessionSweeper) Sweep(ctx context.Context, request oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
	f.calls = append(f.calls, request)
	return f.sweep(ctx, request)
}

func sweepRequest(body string, operator string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/sweep", strings.NewReader(body))
	if operator != "" {
		req = req.WithContext(principal.WithPrincipal(req.Context(), operator))
	}
	return req
}

func sweepTestLogger(logs *bytes.Buffer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(logs, &slog.HandlerOptions{Level: slog.LevelInfo}))
}

func sweepAudit(t *testing.T, logs *bytes.Buffer) map[string]any {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(logs.Bytes()))
	var audit map[string]any
	for {
		var record map[string]any
		err := decoder.Decode(&record)
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		if record["msg"] == "session sweep" {
			require.Nil(t, audit, "one audit event per sweep invocation")
			audit = record
		}
	}
	require.NotNil(t, audit, "the session sweep must be audit-logged")
	return audit
}

func assertSweepAudit(t *testing.T, audit map[string]any, operator, outcome string, result oauth2session.SweepResult) {
	t.Helper()
	assert.Equal(t, "INFO", audit["level"])
	assert.Equal(t, operator, audit["operator_principal"])
	assert.Equal(t, outcome, audit["outcome"])
	assert.Equal(t, result.DryRun, audit["dry_run"])
	assert.Equal(t, float64(result.Refreshed), audit["refreshed"])
	assert.Equal(t, float64(result.Skipped), audit["skipped"])
	assert.Equal(t, float64(result.Failed), audit["failed"])
	assert.Equal(t, float64(result.TotalEvaluated), audit["total_evaluated"])
	assert.Contains(t, audit, "lookahead")
	assert.Contains(t, audit, "page_size")
	duration, ok := audit["duration_ms"].(float64)
	assert.True(t, ok && duration >= 0, "audit duration_ms must be nonnegative")
	assert.NotContains(t, audit, "principal", "end-user identity must not appear in the audit record")
	assert.NotContains(t, audit, "session_id")
	assert.NotContains(t, audit, "service_id")
	assert.NotContains(t, audit, "access_token")
	assert.NotContains(t, audit, "refresh_token")
}

func TestSessionSweepHandlerRequestAndSummary(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		request  oauth2session.SweepRequest
		response oauth2session.SweepResult
	}{
		{
			name:     "empty body uses domain-configured defaults",
			body:     "",
			request:  oauth2session.SweepRequest{},
			response: oauth2session.SweepResult{Refreshed: 2, Skipped: 1, Failed: 3, TotalEvaluated: 6},
		},
		{
			name:     "empty object uses domain-configured defaults",
			body:     "{}",
			request:  oauth2session.SweepRequest{},
			response: oauth2session.SweepResult{Refreshed: 2, Skipped: 1, Failed: 3, TotalEvaluated: 6},
		},
		{
			name:     "optional dry run keeps omitted duration and page size",
			body:     `{"dry_run":true}`,
			request:  oauth2session.SweepRequest{DryRun: true},
			response: oauth2session.SweepResult{Refreshed: 4, Failed: 1, TotalEvaluated: 5, DryRun: true},
		},
		{
			name: "ISO lookahead and page size reach the domain unchanged",
			body: `{"lookahead_duration":"P1DT2H3M4.5S","dry_run":true,"page_size":500}`,
			request: oauth2session.SweepRequest{
				Lookahead: 24*time.Hour + 2*time.Hour + 3*time.Minute + 4500*time.Millisecond,
				DryRun:    true, PageSize: 500,
			},
			response: oauth2session.SweepResult{Refreshed: 4, Failed: 1, TotalEvaluated: 5, DryRun: true},
		},
		{
			name:     "every per-session failure is still a completed sweep",
			body:     `{}`,
			request:  oauth2session.SweepRequest{},
			response: oauth2session.SweepResult{Failed: 7, TotalEvaluated: 7},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
				return tt.response, nil
			}}
			handler := NewSessionSweepHandler(fake, sweepTestLogger(&logs))
			w := httptest.NewRecorder()
			handler.Sweep(w, sweepRequest(tt.body, "operator@example.test"))

			require.Equal(t, http.StatusOK, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			require.Equal(t, []oauth2session.SweepRequest{tt.request}, fake.calls)
			var body map[string]any
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, map[string]any{
				"refreshed":       float64(tt.response.Refreshed),
				"skipped":         float64(tt.response.Skipped),
				"failed":          float64(tt.response.Failed),
				"total_evaluated": float64(tt.response.TotalEvaluated),
				"dry_run":         tt.response.DryRun,
			}, body, "the admin response must contain only the five documented summary fields")
			assertSweepAudit(t, sweepAudit(t, &logs), "operator@example.test", "completed", tt.response)
		})
	}
}

func TestSessionSweepHandlerAuditsServiceReportedEffectiveControls(t *testing.T) {
	var logs bytes.Buffer
	result := oauth2session.SweepResult{
		Refreshed: 2, Skipped: 1, TotalEvaluated: 3,
		EffectiveLookahead: 10 * time.Minute, EffectivePageSize: 37,
	}
	fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
		return result, nil
	}}
	w := httptest.NewRecorder()
	NewSessionSweepHandler(fake, sweepTestLogger(&logs)).Sweep(w, sweepRequest(`{}`, "operator@example.test"))

	require.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []oauth2session.SweepRequest{{}}, fake.calls, "omitted controls must reach the service for default resolution")
	audit := sweepAudit(t, &logs)
	assertSweepAudit(t, audit, "operator@example.test", "completed", result)
	assert.Equal(t, "10m0s", audit["lookahead"])
	assert.Equal(t, float64(37), audit["page_size"])
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Equal(t, map[string]any{
		"refreshed": float64(2), "skipped": float64(1), "failed": float64(0),
		"total_evaluated": float64(3), "dry_run": false,
	}, body, "effective controls are audit-only, not extra HTTP response fields")
}

func TestSessionSweepHandlerRequiresOperator(t *testing.T) {
	fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
		t.Fatal("unauthenticated request reached session sweep")
		return oauth2session.SweepResult{}, nil
	}}
	handler := NewSessionSweepHandler(fake, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	handler.Sweep(w, sweepRequest(`{"lookahead_duration":"PT5M"}`, ""))

	require.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	assert.JSONEq(t, `{"error":"server_misconfiguration","message":"operator principal required for session sweep"}`, w.Body.String())
	assert.Empty(t, fake.calls)
}

func TestSessionSweepHandlerRejectsInvalidPayloads(t *testing.T) {
	tests := []struct {
		name, body, message string
	}{
		{name: "oversized body", body: `{}` + strings.Repeat(" ", 4095)},
		{name: "unknown property", body: `{"lookahead":"PT5M"}`, message: `unknown field "lookahead"`},
		{name: "malformed JSON", body: `{"dry_run":`, message: ""},
		{name: "trailing JSON document", body: `{} {}`, message: ""},
		{name: "Go duration is not ISO 8601", body: `{"lookahead_duration":"5m"}`, message: "lookahead_duration"},
		{name: "ISO month is not a fixed duration", body: `{"lookahead_duration":"P1M"}`, message: "lookahead_duration"},
		{name: "explicit zero duration", body: `{"lookahead_duration":"PT0S"}`, message: "lookahead_duration"},
		{name: "explicit zero page size", body: `{"page_size":0}`, message: "page_size"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
				t.Fatal("invalid HTTP input reached session sweep")
				return oauth2session.SweepResult{}, nil
			}}
			w := httptest.NewRecorder()
			NewSessionSweepHandler(fake, sweepTestLogger(&logs)).Sweep(w, sweepRequest(tt.body, "operator@example.test"))

			require.Equal(t, http.StatusBadRequest, w.Code)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, "invalid request body", body.Error)
			if tt.message != "" {
				assert.Contains(t, body.Message, tt.message)
			}
			assert.Empty(t, fake.calls)
			assertSweepAudit(t, sweepAudit(t, &logs), "operator@example.test", "rejected", oauth2session.SweepResult{})
		})
	}
}

func TestSessionSweepHandlerAcceptsBodyAtLimit(t *testing.T) {
	fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
		return oauth2session.SweepResult{}, nil
	}}
	w := httptest.NewRecorder()
	NewSessionSweepHandler(fake, slog.New(slog.NewTextHandler(io.Discard, nil))).Sweep(w, sweepRequest(`{}`+strings.Repeat(" ", 4094), "operator@example.test"))

	require.Equal(t, http.StatusOK, w.Code, "exactly 4 KiB of valid JSON must not be rejected as oversized")
	assert.Len(t, fake.calls, 1)
}

func TestSessionSweepHandlerMapsServiceErrors(t *testing.T) {
	tests := []struct {
		name, outcome string
		err           error
		status        int
		code, message string
	}{
		{
			name: "invalid domain request", outcome: "rejected",
			err:    fmt.Errorf("%w: page_size must be between 1 and 1000", oauth2session.ErrInvalidSweepRequest),
			status: http.StatusBadRequest, code: "invalid request body", message: "page_size must be between 1 and 1000",
		},
		{
			name: "storage infrastructure error", outcome: "aborted",
			err:    errors.New("database credentials leaked: private-storage-secret"),
			status: http.StatusInternalServerError, code: "internal server error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logs bytes.Buffer
			fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
				return oauth2session.SweepResult{}, tt.err
			}}
			w := httptest.NewRecorder()
			NewSessionSweepHandler(fake, sweepTestLogger(&logs)).Sweep(w, sweepRequest(`{}`, "operator@example.test"))

			require.Equal(t, tt.status, w.Code)
			require.Len(t, fake.calls, 1)
			assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
			assert.Equal(t, tt.code, body.Error)
			if tt.message != "" {
				assert.Contains(t, body.Message, tt.message)
			} else {
				assert.NotContains(t, w.Body.String(), "private-storage-secret", "infrastructure details must not reach HTTP clients")
			}
			assertSweepAudit(t, sweepAudit(t, &logs), "operator@example.test", tt.outcome, oauth2session.SweepResult{})
		})
	}
}

func TestSessionSweepHandlerCanceledRequestWritesNoResponse(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var logs bytes.Buffer
	fake := &fakeSessionSweeper{sweep: func(ctx context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
		return oauth2session.SweepResult{}, ctx.Err()
	}}
	w := httptest.NewRecorder()
	req := sweepRequest(`{}`, "operator@example.test").WithContext(principal.WithPrincipal(ctx, "operator@example.test"))
	NewSessionSweepHandler(fake, sweepTestLogger(&logs)).Sweep(w, req)

	assert.Empty(t, w.Body.String())
	assertSweepAudit(t, sweepAudit(t, &logs), "operator@example.test", "canceled", oauth2session.SweepResult{})
}

func TestSessionSweepHandlerLiftsRealServerWriteDeadline(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	fake := &fakeSessionSweeper{sweep: func(_ context.Context, _ oauth2session.SweepRequest) (oauth2session.SweepResult, error) {
		close(started)
		<-release
		return oauth2session.SweepResult{Refreshed: 1, TotalEvaluated: 1}, nil
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewSessionSweepHandler(fake, logger)
	router := chi.NewRouter()
	router.Use(brokerhttp.LoggingMiddleware(logger))
	router.Post("/api/sessions/sweep", func(w http.ResponseWriter, r *http.Request) {
		handler.Sweep(w, r.WithContext(principal.WithPrincipal(r.Context(), "operator@example.test")))
	})
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 25 * time.Millisecond
	server.Start()
	defer server.Close()

	client := &http.Client{Timeout: 2 * time.Second}
	response := make(chan *http.Response, 1)
	requestError := make(chan error, 1)
	go func() {
		resp, err := client.Post(server.URL+"/api/sessions/sweep", "application/json", strings.NewReader(`{}`))
		if err != nil {
			requestError <- err
			return
		}
		response <- resp
	}()

	select {
	case <-started:
	case resp := <-response:
		close(release)
		_ = resp.Body.Close()
		t.Fatalf("sweep returned before starting: HTTP %d", resp.StatusCode)
	case err := <-requestError:
		close(release)
		t.Fatalf("request failed before sweep started: %v", err)
	case <-time.After(time.Second):
		close(release)
		t.Fatal("sweep handler did not start")
	}
	<-time.After(100 * time.Millisecond)
	close(release)
	select {
	case resp := <-response:
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var body map[string]any
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
		assert.Equal(t, float64(1), body["refreshed"])
	case err := <-requestError:
		t.Fatalf("the response was lost to the server write timeout: %v", err)
	case <-time.After(time.Second):
		t.Fatal("sweep response did not complete after the write timeout was lifted")
	}
}
