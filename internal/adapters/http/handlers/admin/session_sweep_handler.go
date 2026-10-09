package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/oauth2session"
)

type sessionSweeper interface {
	Sweep(context.Context, oauth2session.SweepRequest) (oauth2session.SweepResult, error)
}

type sessionSweepRequestBody struct {
	LookaheadDuration json.RawMessage `json:"lookahead_duration"`
	DryRun            json.RawMessage `json:"dry_run"`
	PageSize          json.RawMessage `json:"page_size"`
}

type sessionSweepResponse struct {
	Refreshed      int  `json:"refreshed"`
	Skipped        int  `json:"skipped"`
	Failed         int  `json:"failed"`
	TotalEvaluated int  `json:"total_evaluated"`
	DryRun         bool `json:"dry_run"`
}

type SessionSweepHandler struct {
	sweeper sessionSweeper
	logger  *slog.Logger
}

func NewSessionSweepHandler(sweeper sessionSweeper, logger *slog.Logger) *SessionSweepHandler {
	return &SessionSweepHandler{sweeper: sweeper, logger: logger}
}

func (h *SessionSweepHandler) Sweep(w http.ResponseWriter, r *http.Request) {
	operator, ok := operatorPrincipalFromContext(r.Context(), h.logger)
	if !ok {
		h.writeError(w, http.StatusInternalServerError, "server_misconfiguration", "operator principal required for session sweep")
		return
	}

	started := time.Now()
	var request oauth2session.SweepRequest
	var result oauth2session.SweepResult
	outcome := "rejected"
	defer func() {
		effective := request
		if result.EffectiveLookahead != 0 {
			effective.Lookahead = result.EffectiveLookahead
		}
		if result.EffectivePageSize != 0 {
			effective.PageSize = result.EffectivePageSize
		}
		h.logger.InfoContext(r.Context(), "session sweep",
			"operator_principal", operator,
			"dry_run", request.DryRun,
			"lookahead", effective.Lookahead.String(),
			"page_size", effective.PageSize,
			"refreshed", result.Refreshed,
			"skipped", result.Skipped,
			"failed", result.Failed,
			"total_evaluated", result.TotalEvaluated,
			"duration_ms", time.Since(started).Milliseconds(),
			"outcome", outcome)
	}()

	var err error
	request, err = parseSessionSweepRequest(w, r)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
		return
	}

	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		if errors.Is(err, http.ErrNotSupported) {
			h.logger.WarnContext(r.Context(), "session sweep write deadline cannot be cleared")
		} else {
			outcome = "aborted"
			h.logger.ErrorContext(r.Context(), "session sweep write deadline could not be cleared")
			h.writeError(w, http.StatusInternalServerError, "internal server error", "")
			return
		}
	}

	result, err = h.sweeper.Sweep(r.Context(), request)
	if r.Context().Err() != nil {
		outcome = "canceled"
		return
	}
	if err != nil {
		if errors.Is(err, oauth2session.ErrInvalidSweepRequest) {
			h.writeError(w, http.StatusBadRequest, "invalid request body", err.Error())
			return
		}
		outcome = "aborted"
		h.writeError(w, http.StatusInternalServerError, "internal server error", "")
		return
	}

	outcome = "completed"
	h.writeJSON(w, http.StatusOK, sessionSweepResponse{
		Refreshed:      result.Refreshed,
		Skipped:        result.Skipped,
		Failed:         result.Failed,
		TotalEvaluated: result.TotalEvaluated,
		DryRun:         result.DryRun,
	})
}

func parseSessionSweepRequest(w http.ResponseWriter, r *http.Request) (oauth2session.SweepRequest, error) {
	var request oauth2session.SweepRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10))
	decoder.DisallowUnknownFields()
	var body *sessionSweepRequestBody
	if err := decoder.Decode(&body); err != nil {
		if !errors.Is(err, io.EOF) {
			return request, err
		}
		return request, nil
	}
	if body == nil {
		return request, errors.New("request body must be a JSON object")
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err != nil {
			return request, err
		}
		return request, errors.New("request body must contain only one JSON object")
	}

	if body.LookaheadDuration != nil {
		var lookahead string
		if err := json.Unmarshal(body.LookaheadDuration, &lookahead); err != nil || string(body.LookaheadDuration) == "null" {
			return request, errors.New("lookahead_duration must be an ISO 8601 duration of days, hours, minutes, or seconds (e.g. PT5M)")
		}
		var err error
		request.Lookahead, err = parseISO8601Duration(lookahead)
		if err != nil {
			return request, errors.New("lookahead_duration must be an ISO 8601 duration of days, hours, minutes, or seconds (e.g. PT5M)")
		}
	}
	if body.DryRun != nil {
		if err := json.Unmarshal(body.DryRun, &request.DryRun); err != nil || string(body.DryRun) == "null" {
			return request, errors.New("dry_run must be a boolean")
		}
	}
	if body.PageSize != nil {
		if err := json.Unmarshal(body.PageSize, &request.PageSize); err != nil || string(body.PageSize) == "null" {
			return request, errors.New("page_size must be an integer between 1 and 1000")
		}
		if request.PageSize == 0 {
			return request, errors.New("page_size must be between 1 and 1000")
		}
	}
	return request, nil
}

func (h *SessionSweepHandler) writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		h.logger.Warn("failed to encode session sweep response")
	}
}

func (h *SessionSweepHandler) writeError(w http.ResponseWriter, status int, code, message string) {
	h.writeJSON(w, status, ErrorResponse{Error: code, Message: message})
}
