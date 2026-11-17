package enduser

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	httpmiddleware "github.com/agentic-identity-broker/agentic-identity-broker/internal/adapters/http/middleware"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

// MaxAccessEvaluationBodyBytes bounds the evaluation request body.
const MaxAccessEvaluationBodyBytes = 64 << 10

// AccessEvaluator answers AuthZEN access evaluations.
type AccessEvaluator interface {
	Evaluate(ctx context.Context, req tokenexchange.AccessEvaluationRequest, clientAssertion string) tokenexchange.DelegationDecision
}

// AuthZENEvaluationHandler serves POST /access/v1/evaluation. It only decodes, authenticates
// the transport, delegates to the domain service and serializes the decision.
type AuthZENEvaluationHandler struct {
	Evaluator AccessEvaluator
	Logger    *slog.Logger
}

type evaluationErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message,omitempty"`
}

type evaluationPermitBody struct {
	Decision bool                             `json:"decision"`
	Context  *tokenexchange.DelegationContext `json:"context"`
}

type evaluationDenialBody struct {
	Decision bool                   `json:"decision"`
	Context  evaluationDenialReason `json:"context"`
}

type evaluationDenialReason struct {
	ReasonAdmin struct {
		Code string `json:"code"`
	} `json:"reason_admin"`
	ReasonUser struct {
		ConsentURL string `json:"consent_url"`
	} `json:"reason_user"`
}

// Unknown members are ignored at every level (AuthZEN 1.0 HTTPS JSON binding); required
// members must be present with the documented JSON types.
type wireEvaluationRequest struct {
	Subject *struct {
		Type *string `json:"type"`
		ID   *string `json:"id"`
	} `json:"subject"`
	Action *struct {
		Name *string `json:"name"`
	} `json:"action"`
	Resource *struct {
		Type *string `json:"type"`
		ID   *string `json:"id"`
	} `json:"resource"`
	Context json.RawMessage `json:"context"`
}

var errMalformedEvaluation = errors.New("malformed access evaluation request")

func (h *AuthZENEvaluationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	echoRequestID(w, r)
	ctx, span := otel.Tracer("tokenexchange").Start(r.Context(), "authzen.access_evaluation")
	defer span.End()

	assertion, ok := bearerCredential(r)
	if !ok {
		h.writeEarlyFailure(ctx, span, w, http.StatusUnauthorized, tokenexchange.DetailClientInvalid, `Bearer`)
		return
	}
	if mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mediaType != "application/json" {
		h.writeEarlyFailure(ctx, span, w, http.StatusBadRequest, tokenexchange.DetailRequestMalformed, "")
		return
	}
	req, err := decodeEvaluationRequest(http.MaxBytesReader(w, r.Body, MaxAccessEvaluationBodyBytes))
	if err != nil {
		h.writeEarlyFailure(ctx, span, w, http.StatusBadRequest, tokenexchange.DetailRequestMalformed, "")
		return
	}

	decision := h.Evaluator.Evaluate(ctx, req, assertion)
	httpmiddleware.FinalizeRequestSecurityContext(ctx)
	h.observe(ctx, span, decision)

	switch decision.Kind {
	case tokenexchange.DecisionPermit:
		w.Header().Set("Cache-Control", "private, max-age="+strconv.Itoa(decision.MaxAge))
		writeEvaluationJSON(w, http.StatusOK, evaluationPermitBody{Decision: true, Context: decision.Context})
	case tokenexchange.DecisionDeny:
		body := evaluationDenialBody{}
		body.Context.ReasonAdmin.Code = string(decision.Detail)
		body.Context.ReasonUser.ConsentURL = decision.ConsentURL
		w.Header().Set("Cache-Control", "no-store")
		writeEvaluationJSON(w, http.StatusOK, body)
	default:
		challenge := ""
		if decision.Status == http.StatusUnauthorized {
			challenge = `Bearer error="invalid_token"`
		}
		writeEvaluationError(w, decision.Status, challenge)
	}
}

func bearerCredential(r *http.Request) (string, bool) {
	scheme, credential, found := strings.Cut(r.Header.Get("Authorization"), " ")
	if !found || !strings.EqualFold(scheme, "Bearer") {
		return "", false
	}
	credential = strings.TrimSpace(credential)
	return credential, credential != ""
}

func decodeEvaluationRequest(body io.Reader) (tokenexchange.AccessEvaluationRequest, error) {
	var wire wireEvaluationRequest
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&wire); err != nil {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	if wire.Subject == nil || wire.Subject.Type == nil || wire.Subject.ID == nil ||
		wire.Action == nil || wire.Action.Name == nil ||
		wire.Resource == nil || wire.Resource.Type == nil || wire.Resource.ID == nil {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	req := tokenexchange.AccessEvaluationRequest{
		SubjectType:  *wire.Subject.Type,
		SubjectID:    *wire.Subject.ID,
		ActionName:   *wire.Action.Name,
		ResourceType: *wire.Resource.Type,
		ResourceID:   *wire.Resource.ID,
	}
	if len(wire.Context) == 0 {
		return req, nil
	}
	if !bytes.HasPrefix(bytes.TrimSpace(wire.Context), []byte("{")) {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(wire.Context, &members); err != nil {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	if raw, ok := members["agent"]; ok {
		var anchor string
		if err := json.Unmarshal(raw, &anchor); err != nil || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
		}
		req.AgentAnchor = &anchor
	}
	return req, nil
}

func (h *AuthZENEvaluationHandler) writeEarlyFailure(ctx context.Context, span trace.Span, w http.ResponseWriter, status int, detail tokenexchange.FailureDetail, challenge string) {
	stage := tokenexchange.StageRequestValidation
	if status == http.StatusUnauthorized {
		stage = tokenexchange.StageClientValidation
	}
	httpmiddleware.FinalizeRequestSecurityContext(ctx)
	h.observe(ctx, span, tokenexchange.DelegationDecision{
		Kind:       tokenexchange.DecisionRequestFailure,
		Status:     status,
		Diagnostic: tokenexchange.NewDiagnostic(stage, detail),
	})
	writeEvaluationError(w, status, challenge)
}

func writeEvaluationError(w http.ResponseWriter, status int, challenge string) {
	body := evaluationErrorBody{Error: "server_error", Message: "access evaluation failed"}
	switch status {
	case http.StatusBadRequest:
		body = evaluationErrorBody{Error: "invalid_request", Message: "invalid access evaluation request"}
	case http.StatusUnauthorized:
		body = evaluationErrorBody{Error: "unauthorized", Message: "client authentication required"}
	case http.StatusForbidden:
		body = evaluationErrorBody{Error: "forbidden", Message: "caller is not authorized for access evaluation"}
	default:
		status = http.StatusInternalServerError
	}
	if challenge != "" {
		w.Header().Set("WWW-Authenticate", challenge)
	}
	w.Header().Set("Cache-Control", "no-store")
	writeEvaluationJSON(w, status, body)
}

func writeEvaluationJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// observe records a credential-free audit event and span attributes using the
// token-exchange diagnostic vocabulary.
func (h *AuthZENEvaluationHandler) observe(ctx context.Context, span trace.Span, decision tokenexchange.DelegationDecision) {
	diagnostic := decision.Diagnostic
	attrs := []attribute.KeyValue{
		attribute.String("token_exchange.operation", "access_evaluation"),
		attribute.String("token_exchange.outcome", string(diagnostic.Outcome())),
		attribute.String("token_exchange.recovery_action", string(diagnostic.RecoveryAction())),
		attribute.String("token_exchange.recovery_target", string(diagnostic.RecoveryTarget())),
	}
	level, message := slog.LevelInfo, "access_evaluation_permitted"
	switch decision.Kind {
	case tokenexchange.DecisionPermit:
		attrs = append(attrs, attribute.String("access_evaluation.decision", "permit"), attribute.Int("access_evaluation.max_age", decision.MaxAge))
	case tokenexchange.DecisionDeny:
		message = "access_evaluation_denied"
		attrs = append(attrs, attribute.String("access_evaluation.decision", "deny"))
	default:
		level, message = slog.LevelError, "access_evaluation_failed"
		attrs = append(attrs, attribute.String("access_evaluation.decision", "error"), attribute.Int("http.response.status_code", decision.Status))
		span.SetStatus(codes.Error, "access evaluation failed")
	}
	if decision.Kind != tokenexchange.DecisionPermit {
		attrs = append(attrs,
			attribute.String("token_exchange.failure_stage", string(diagnostic.Stage())),
			attribute.String("token_exchange.failure_detail", string(diagnostic.Detail())),
		)
	}
	if !decision.Service.ID.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.service.id", decision.Service.ID.String()))
	}
	authorization := decision.Authorization
	if !authorization.AgentID.IsZero() {
		attrs = append(attrs, attribute.String("token_exchange.agent.id", authorization.AgentID.String()))
	}
	if !authorization.GrantID.IsZero() {
		attrs = append(attrs,
			attribute.String("token_exchange.grant.id", authorization.GrantID.String()),
			attribute.String("token_exchange.grant.updated_at", authorization.GrantUpdatedAt.UTC().Format(time.RFC3339Nano)))
	}
	span.SetAttributes(attrs...)
	if h.Logger != nil {
		logAttrs := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			logAttrs = append(logAttrs, slog.Any(string(attr.Key), attr.Value.AsInterface()))
		}
		h.Logger.LogAttrs(ctx, level, message, logAttrs...)
	}
}
