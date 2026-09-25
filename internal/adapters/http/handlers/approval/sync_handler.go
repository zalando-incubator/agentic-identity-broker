package approval

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	domainapproval "github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/approval"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
)

const (
	defaultLongPollTimeout = 30 * time.Second
	maxLongPollTimeout     = 120 * time.Second
)

// syncResponse matches the ApprovalSyncResponse schema.
type syncResponse struct {
	Data syncResponseData `json:"data"`
}

type syncResponseData struct {
	Pairs []syncResponsePair `json:"pairs"`
}

type syncResponsePair struct {
	Principal             string                `json:"principal"`
	AgentID               string                `json:"agent_id"`
	Approvals             []toolApprovalSummary `json:"approvals"`
	GrantedPermissionSets map[string]any        `json:"granted_permission_sets"`
}

type toolApprovalSummary struct {
	ID             string            `json:"id"`
	ToolName       string            `json:"tool_name"`
	ArgumentsHash  string            `json:"arguments_hash"`
	ToolPattern    string            `json:"tool_pattern"`
	ParamsPattern  map[string]string `json:"params_pattern"`
	Status         string            `json:"status"`
	Persistence    *string           `json:"persistence,omitempty"`
	Consumed       bool              `json:"consumed"`
	AgentSessionID *string           `json:"agent_session_id,omitempty"`
}

func toSummary(a *storage.ToolApproval) toolApprovalSummary {
	s := toolApprovalSummary{
		ID:            a.ID.String(),
		ToolName:      a.ToolName,
		ArgumentsHash: a.ArgumentsHash,
		ToolPattern:   a.ToolPattern,
		ParamsPattern: a.ParamsPattern,
		Status:        string(a.Status),
		Consumed:      a.Consumed,
	}
	if s.ParamsPattern == nil {
		s.ParamsPattern = map[string]string{}
	}
	if a.Persistence != nil {
		p := string(*a.Persistence)
		s.Persistence = &p
	}
	if a.AgentSessionID != nil {
		s.AgentSessionID = a.AgentSessionID
	}
	return s
}

// SyncHandler handles GET /api/approvals with long-poll semantics.
type SyncHandler struct {
	service        *domainapproval.Service
	longPollActive metric.Int64UpDownCounter
}

// NewSyncHandler creates a handler for syncing approval state.
func NewSyncHandler(service *domainapproval.Service) *SyncHandler {
	meter := otel.Meter("approval")
	longPollActive, _ := meter.Int64UpDownCounter("long_poll_connections_active",
		metric.WithDescription("Number of active long-poll connections"),
		metric.WithUnit("{connection}"),
	)
	return &SyncHandler{service: service, longPollActive: longPollActive}
}

func (h *SyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h.service == nil {
		http.Error(w, "not implemented", http.StatusNotImplemented)
		return
	}

	var principalFilter *id.Principal
	if p := r.URL.Query().Get("principal"); p != "" {
		principal := id.Principal(p)
		principalFilter = &principal
	}

	activeAgentSessionIDs := r.URL.Query()["agent_session_id"]

	var clientVersion int64
	var hasClientVersion bool
	if etag := r.Header.Get("If-None-Match"); etag != "" {
		if len(etag) >= 2 && etag[0] == '"' && etag[len(etag)-1] == '"' {
			etag = etag[1 : len(etag)-1]
		}
		if len(etag) > 1 && etag[0] == 'v' {
			etag = etag[1:]
		}
		if v, err := strconv.ParseInt(etag, 10, 64); err == nil {
			clientVersion = v
			hasClientVersion = true
		}
	}

	timeout := defaultLongPollTimeout
	if timeoutStr := r.Header.Get("X-Long-Poll-Timeout"); timeoutStr != "" {
		if secs, err := strconv.Atoi(timeoutStr); err == nil && secs > 0 {
			timeout = time.Duration(secs) * time.Second
			if timeout > maxLongPollTimeout {
				timeout = maxLongPollTimeout
			}
		}
	}

	if hasClientVersion {
		broadcaster := h.service.GetBroadcaster()
		if broadcaster != nil {
			ch := broadcaster.Subscribe()
			defer broadcaster.Unsubscribe(ch)

			currentVersion, err := h.service.GetVersion(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to get sync version")
				return
			}
			if currentVersion == clientVersion {
				h.longPollActive.Add(r.Context(), 1)
				defer h.longPollActive.Add(context.WithoutCancel(r.Context()), -1)

				timer := time.NewTimer(timeout)
				defer timer.Stop()

				select {
				case <-ch:
				case <-timer.C:
					w.WriteHeader(http.StatusNotModified)
					return
				case <-r.Context().Done():
					return
				}
			}
		} else {
			currentVersion, err := h.service.GetVersion(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, "internal_error", "failed to get sync version")
				return
			}
			if currentVersion == clientVersion {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
	}

	state, err := h.service.GetSyncState(r.Context(), principalFilter, activeAgentSessionIDs)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal_error", "failed to get sync state")
		return
	}

	pairs := make([]syncResponsePair, 0, len(state.Pairs))
	for _, p := range state.Pairs {
		summaries := make([]toolApprovalSummary, 0, len(p.Approvals))
		for _, a := range p.Approvals {
			summaries = append(summaries, toSummary(a))
		}
		pairs = append(pairs, syncResponsePair{
			Principal:             string(p.Principal),
			AgentID:               p.AgentID.String(),
			Approvals:             summaries,
			GrantedPermissionSets: map[string]any{},
		})
	}

	resp := syncResponse{Data: syncResponseData{Pairs: pairs}}

	w.Header().Set("ETag", fmt.Sprintf(`"v%d"`, state.Version))
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
