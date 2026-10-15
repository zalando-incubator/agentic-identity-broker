package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/principal"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type fakeSessionAdmin struct {
	listQuery oauth2session.SessionListQuery
	list      oauth2session.SessionList
	listErr   error
	get       *storage.SessionAdminSummary
	getErr    error
	listCalls int
}

func (f *fakeSessionAdmin) ListSessions(_ context.Context, q oauth2session.SessionListQuery) (oauth2session.SessionList, error) {
	f.listQuery = q
	f.listCalls++
	return f.list, f.listErr
}

func (f *fakeSessionAdmin) GetSession(context.Context, id.SessionID) (*storage.SessionAdminSummary, error) {
	return f.get, f.getErr
}

func sessionsRouter(h *SessionsHandler) http.Handler {
	r := chi.NewRouter()
	r.Get("/api/sessions", h.List)
	r.Get("/api/sessions/{session-id}", h.Get)
	return r
}

func sessionsRequest(target, operator string) *http.Request {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	if operator != "" {
		req = req.WithContext(principal.WithPrincipal(req.Context(), operator))
	}
	return req
}

func TestSessionsHandlerList(t *testing.T) {
	summary := storage.NewSessionAdminSummary(&storage.UserSession{
		ID: id.NewSessionID(), Principal: "a@example.com", ServiceID: id.NewServiceID(),
		EncryptedAccessToken: []byte("secret-ciphertext"), TokenType: "Bearer",
	}, time.Now())
	fake := &fakeSessionAdmin{list: oauth2session.SessionList{Items: []*storage.SessionAdminSummary{summary}, Truncated: true, NextCursor: "next-position"}}
	var logs bytes.Buffer
	router := sessionsRouter(NewSessionsHandler(fake, sweepTestLogger(&logs)))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions?principal=a@example.com&limit=5&cursor=prev-position", "operator"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, oauth2session.SessionListQuery{Principal: "a@example.com", Limit: 5, Cursor: "prev-position"}, fake.listQuery)
	assert.NotContains(t, rec.Body.String(), "ciphertext")
	var body struct {
		Items      []map[string]any `json:"items"`
		Truncated  bool             `json:"truncated"`
		NextCursor string           `json:"next_cursor"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Items, 1)
	assert.Equal(t, "a@example.com", body.Items[0]["principal"])
	assert.True(t, body.Truncated)
	assert.Equal(t, "next-position", body.NextCursor)
	assert.Contains(t, logs.String(), `"operator_principal":"operator"`)
	assert.Contains(t, logs.String(), `"target_principal":"a@example.com"`)
}

func TestSessionsHandlerListEmptyIsArray(t *testing.T) {
	fake := &fakeSessionAdmin{list: oauth2session.SessionList{Items: []*storage.SessionAdminSummary{}}}
	router := sessionsRouter(NewSessionsHandler(fake, sweepTestLogger(&bytes.Buffer{})))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions", "operator"))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.JSONEq(t, `{"items":[],"truncated":false}`, rec.Body.String())
	assert.Equal(t, oauth2session.SessionListQuery{}, fake.listQuery)
}

func TestSessionsHandlerListRejectsBadQuery(t *testing.T) {
	limitMessage := oauth2session.ErrInvalidSessionLimit.Error()
	prefix := oauth2session.ErrInvalidSessionQuery.Error() + ": "
	for target, wantMessage := range map[string]string{
		"/api/sessions?principal=":                 prefix + "principal must not be empty",
		"/api/sessions?limit=abc":                  limitMessage,
		"/api/sessions?limit=":                     limitMessage,
		"/api/sessions?limit=-1":                   limitMessage,
		"/api/sessions?limit=0":                    limitMessage,
		"/api/sessions?limit=99999999999999999999": limitMessage,
		"/api/sessions?cursor=":                    prefix + "cursor must not be empty",
	} {
		t.Run(target, func(t *testing.T) {
			fake := &fakeSessionAdmin{}
			var logs bytes.Buffer
			router := sessionsRouter(NewSessionsHandler(fake, sweepTestLogger(&logs)))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, sessionsRequest(target, "operator"))
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Zero(t, fake.listCalls)
			var body ErrorResponse
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, wantMessage, body.Message)
			assert.Contains(t, logs.String(), `"operator_principal":"operator"`)
			assert.Contains(t, logs.String(), `"outcome":"rejected"`)
		})
	}
}

func TestSessionsHandlerMapsServiceErrors(t *testing.T) {
	var rejectedLogs bytes.Buffer
	router := sessionsRouter(NewSessionsHandler(&fakeSessionAdmin{
		listErr: oauth2session.ErrInvalidSessionLimit,
	}, sweepTestLogger(&rejectedLogs)))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions?limit=5000", "operator"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), oauth2session.ErrInvalidSessionLimit.Error())
	assert.Contains(t, rejectedLogs.String(), `"outcome":"rejected"`)

	var logs bytes.Buffer
	router = sessionsRouter(NewSessionsHandler(&fakeSessionAdmin{listErr: errors.New("db unreachable")}, sweepTestLogger(&logs)))
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions", "operator"))
	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.NotContains(t, rec.Body.String(), "db unreachable")
	assert.Contains(t, logs.String(), `"error":"db unreachable"`)
	assert.Contains(t, logs.String(), `"outcome":"failed"`)
}

func TestSessionsHandlerGet(t *testing.T) {
	sessionID := id.NewSessionID()
	summary := storage.NewSessionAdminSummary(&storage.UserSession{
		ID: sessionID, Principal: "a@example.com", ServiceID: id.NewServiceID(), TokenType: "Bearer",
	}, time.Now())
	var logs bytes.Buffer
	router := sessionsRouter(NewSessionsHandler(&fakeSessionAdmin{get: summary}, sweepTestLogger(&logs)))

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions/"+sessionID.String(), "operator"))
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, sessionID.String(), body["id"])
	assert.Contains(t, logs.String(), `"operator_principal":"operator"`)
	assert.Contains(t, logs.String(), `"session_id":"`+sessionID.String()+`"`)

	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, sessionsRequest("/api/sessions/not-a-uuid", "operator"))
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, logs.String(), `"outcome":"rejected"`)
}

func TestSessionsHandlerGetErrors(t *testing.T) {
	target := "/api/sessions/" + id.NewSessionID().String()
	for name, tc := range map[string]struct {
		err    error
		status int
	}{
		"not found": {oauth2session.ErrSessionNotFound, http.StatusNotFound},
		"internal":  {errors.New("db down"), http.StatusInternalServerError},
	} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			router := sessionsRouter(NewSessionsHandler(&fakeSessionAdmin{getErr: tc.err}, sweepTestLogger(&logs)))
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, sessionsRequest(target, "operator"))
			assert.Equal(t, tc.status, rec.Code)
			assert.NotContains(t, rec.Body.String(), "db down")
			if tc.status == http.StatusInternalServerError {
				assert.Contains(t, logs.String(), `"error":"db down"`)
			}
		})
	}
}

func TestSessionsHandlerRequiresOperator(t *testing.T) {
	fake := &fakeSessionAdmin{}
	router := sessionsRouter(NewSessionsHandler(fake, sweepTestLogger(&bytes.Buffer{})))
	for _, target := range []string{"/api/sessions", "/api/sessions/" + id.NewSessionID().String()} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, sessionsRequest(target, ""))
		assert.Equal(t, http.StatusInternalServerError, rec.Code, target)
	}
	assert.Zero(t, fake.listCalls)
}
