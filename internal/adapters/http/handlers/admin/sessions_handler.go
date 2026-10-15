package admin

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

type sessionAdminReader interface {
	ListSessions(context.Context, oauth2session.SessionListQuery) (oauth2session.SessionList, error)
	GetSession(context.Context, id.SessionID) (*storage.SessionAdminSummary, error)
}

type sessionListResponse struct {
	Items      []*storage.SessionAdminSummary `json:"items"`
	Truncated  bool                           `json:"truncated"`
	NextCursor string                         `json:"next_cursor,omitempty"`
}

type SessionsHandler struct {
	sessions sessionAdminReader
	logger   *slog.Logger
}

func NewSessionsHandler(sessions sessionAdminReader, logger *slog.Logger) *SessionsHandler {
	return &SessionsHandler{sessions: sessions, logger: logger}
}

// List handles GET /api/sessions with optional exact-match principal, limit, and cursor query parameters.
func (h *SessionsHandler) List(w http.ResponseWriter, r *http.Request) {
	operator, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "server_misconfiguration", "operator principal required for session listing")
		return
	}

	var query oauth2session.SessionListQuery
	reject := func(message string) {
		h.auditList(r.Context(), slog.LevelInfo, operator, query.Principal, "rejected")
		h.writeError(w, http.StatusBadRequest, "invalid_request", message)
	}

	values := r.URL.Query()
	if values.Has("principal") {
		query.Principal = id.Principal(values.Get("principal"))
		if query.Principal.IsZero() {
			reject(oauth2session.ErrInvalidSessionQuery.Error() + ": principal must not be empty")
			return
		}
	}
	if values.Has("limit") {
		limit, err := strconv.Atoi(values.Get("limit"))
		if err != nil || limit < 1 {
			reject(oauth2session.ErrInvalidSessionLimit.Error())
			return
		}
		query.Limit = limit
	}
	if values.Has("cursor") {
		query.Cursor = values.Get("cursor")
		if query.Cursor == "" {
			reject(oauth2session.ErrInvalidSessionQuery.Error() + ": cursor must not be empty")
			return
		}
	}

	list, err := h.sessions.ListSessions(r.Context(), query)
	if err != nil {
		if errors.Is(err, oauth2session.ErrInvalidSessionQuery) {
			reject(err.Error())
			return
		}
		h.auditList(r.Context(), slog.LevelError, operator, query.Principal, "failed", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	h.auditList(r.Context(), slog.LevelInfo, operator, query.Principal, "completed",
		"count", len(list.Items), "truncated", list.Truncated)
	h.writeJSON(w, http.StatusOK, sessionListResponse{Items: list.Items, Truncated: list.Truncated, NextCursor: list.NextCursor})
}

// Get handles GET /api/sessions/{session-id}.
func (h *SessionsHandler) Get(w http.ResponseWriter, r *http.Request) {
	operator, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "server_misconfiguration", "operator principal required for session lookup")
		return
	}

	sessionID, err := id.ParseSessionID(chi.URLParam(r, "session-id"))
	if err != nil {
		h.auditGet(r.Context(), slog.LevelInfo, operator, "rejected")
		h.writeError(w, http.StatusBadRequest, "invalid_request", "invalid session id")
		return
	}

	summary, err := h.sessions.GetSession(r.Context(), sessionID)
	if err != nil {
		if errors.Is(err, oauth2session.ErrSessionNotFound) {
			h.auditGet(r.Context(), slog.LevelInfo, operator, "not_found", "session_id", sessionID.String())
			h.writeError(w, http.StatusNotFound, "not_found", "session not found")
			return
		}
		h.auditGet(r.Context(), slog.LevelError, operator, "failed", "session_id", sessionID.String(), "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}
	h.auditGet(r.Context(), slog.LevelInfo, operator, "completed",
		"session_id", sessionID.String(), "target_principal", summary.Principal.String())
	h.writeJSON(w, http.StatusOK, summary)
}

func (h *SessionsHandler) auditList(ctx context.Context, level slog.Level, operator string, target id.Principal, outcome string, attrs ...any) {
	h.logger.Log(ctx, level, "session admin list",
		append([]any{"operator_principal", operator, "target_principal", target.String(), "outcome", outcome}, attrs...)...)
}

func (h *SessionsHandler) auditGet(ctx context.Context, level slog.Level, operator, outcome string, attrs ...any) {
	h.logger.Log(ctx, level, "session admin get",
		append([]any{"operator_principal", operator, "outcome", outcome}, attrs...)...)
}

func (h *SessionsHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Warn("failed to encode sessions response")
	}
}

func (h *SessionsHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}
