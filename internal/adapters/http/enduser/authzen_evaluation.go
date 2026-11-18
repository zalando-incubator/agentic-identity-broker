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
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/security"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/tokenexchange"
)

// MaxAccessEvaluationBodyBytes bounds the evaluation request body.
const MaxAccessEvaluationBodyBytes = 64 << 10

// AccessEvaluator answers AuthZEN access evaluations.
type AccessEvaluator interface {
	Evaluate(ctx context.Context, req tokenexchange.AccessEvaluationRequest, clientAssertion string) tokenexchange.GrantDecision
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
	Decision bool                        `json:"decision"`
	Context  *tokenexchange.GrantContext `json:"context"`
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

var errMalformedEvaluation = errors.New("malformed access evaluation request")

func (h *AuthZENEvaluationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	echoRequestID(w, r)
	ctx, span := otel.Tracer("tokenexchange").Start(r.Context(), "tokenexchange.access_evaluation", trace.WithSpanKind(trace.SpanKindInternal))
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

// decodeEvaluationRequest reads members by exact name. Unknown members, including case
// variants of supported names, are ignored at every level (AuthZEN 1.0 HTTPS JSON binding)
// and cannot replace a supported member; required members must be present with the
// documented JSON types.
func decodeEvaluationRequest(body io.Reader) (tokenexchange.AccessEvaluationRequest, error) {
	var raw json.RawMessage
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(&raw); err != nil {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	top, ok := jsonObject(raw)
	if !ok {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	subject, subjectOK := jsonObject(top["subject"])
	action, actionOK := jsonObject(top["action"])
	resource, resourceOK := jsonObject(top["resource"])
	if !subjectOK || !actionOK || !resourceOK {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	var req tokenexchange.AccessEvaluationRequest
	for _, member := range []struct {
		object map[string]json.RawMessage
		name   string
		target *string
	}{
		{subject, "type", &req.SubjectType},
		{subject, "id", &req.SubjectID},
		{action, "name", &req.ActionName},
		{resource, "type", &req.ResourceType},
		{resource, "id", &req.ResourceID},
	} {
		value, ok := jsonString(member.object[member.name])
		if !ok {
			return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
		}
		*member.target = value
	}
	rawContext, present := top["context"]
	if !present {
		return req, nil
	}
	members, ok := jsonObject(rawContext)
	if !ok {
		return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
	}
	if rawAnchor, present := members["agent"]; present {
		anchor, ok := jsonString(rawAnchor)
		if !ok {
			return tokenexchange.AccessEvaluationRequest{}, errMalformedEvaluation
		}
		req.AgentAnchor = &anchor
	}
	return req, nil
}

// jsonObject decodes a JSON object into its members; null and other types are rejected.
func jsonObject(raw json.RawMessage) (map[string]json.RawMessage, bool) {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte("{")) {
		return nil, false
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return nil, false
	}
	return members, true
}

// jsonString decodes a JSON string; null and other types are rejected.
func jsonString(raw json.RawMessage) (string, bool) {
	if !bytes.HasPrefix(bytes.TrimSpace(raw), []byte(`"`)) {
		return "", false
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false
	}
	return value, true
}

func (h *AuthZENEvaluationHandler) writeEarlyFailure(ctx context.Context, span trace.Span, w http.ResponseWriter, status int, detail tokenexchange.FailureDetail, challenge string) {
	stage := tokenexchange.StageRequestValidation
	if status == http.StatusUnauthorized {
		stage = tokenexchange.StageClientValidation
	}
	httpmiddleware.FinalizeRequestSecurityContext(ctx)
	h.observe(ctx, span, tokenexchange.GrantDecision{
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
func (h *AuthZENEvaluationHandler) observe(ctx context.Context, span trace.Span, decision tokenexchange.GrantDecision) {
	diagnostic := decision.Diagnostic
	attrs := []attribute.KeyValue{
		attribute.String("token_exchange.operation", "access_evaluation"),
		attribute.String("token_exchange.outcome", string(diagnostic.Outcome())),
		attribute.String("token_exchange.recovery_action", string(diagnostic.RecoveryAction())),
		attribute.String("token_exchange.recovery_target", string(diagnostic.RecoveryTarget())),
	}
	level, event := slog.LevelInfo, "tokenexchange.access_evaluation.permitted"
	switch decision.Kind {
	case tokenexchange.DecisionPermit:
		attrs = append(attrs, attribute.String("access_evaluation.decision", "permit"), attribute.Int("access_evaluation.max_age", decision.MaxAge))
	case tokenexchange.DecisionDeny:
		event = "tokenexchange.access_evaluation.denied"
		attrs = append(attrs, attribute.String("access_evaluation.decision", "deny"))
	default:
		level, event = slog.LevelError, "tokenexchange.access_evaluation.failed"
		if decision.Status >= http.StatusBadRequest && decision.Status < http.StatusInternalServerError {
			level = slog.LevelWarn
		}
		attrs = append(attrs, attribute.String("access_evaluation.decision", "error"), attribute.Int("http.response.status_code", decision.Status))
		if diagnostic.Outcome() != tokenexchange.OutcomeCanceled {
			span.SetStatus(codes.Error, "access evaluation failed")
		}
	}
	if decision.Kind != tokenexchange.DecisionPermit {
		attrs = append(attrs,
			attribute.String("token_exchange.failure_stage", string(diagnostic.Stage())),
			attribute.String("token_exchange.failure_detail", string(diagnostic.Detail())),
		)
	}
	// Entity IDs use the OpenTelemetry <namespace>.<entity>.id keys on the span and the
	// canonical typed <entity>_id keys in the audit record (LOGGING.md §2.3).
	var spanIDs []attribute.KeyValue
	var logIDs []slog.Attr
	if !decision.Service.ID.IsZero() {
		spanIDs = append(spanIDs, attribute.String("token_exchange.service.id", decision.Service.ID.String()))
		logIDs = append(logIDs, slog.Any("service_id", decision.Service.ID))
	}
	authorization := decision.Authorization
	if !authorization.AgentID.IsZero() {
		spanIDs = append(spanIDs, attribute.String("token_exchange.agent.id", authorization.AgentID.String()))
		logIDs = append(logIDs, slog.Any("agent_id", authorization.AgentID))
	}
	if !authorization.GrantID.IsZero() {
		spanIDs = append(spanIDs, attribute.String("token_exchange.grant.id", authorization.GrantID.String()))
		logIDs = append(logIDs, slog.Any("grant_id", authorization.GrantID))
		attrs = append(attrs, attribute.String("token_exchange.grant.updated_at", authorization.GrantUpdatedAt.UTC().Format(time.RFC3339Nano)))
	}
	span.SetAttributes(append(attrs, spanIDs...)...)
	if h.Logger != nil {
		principal := authorization.Principal
		if principal == "" {
			principal = id.NewPrincipal(security.AnonymousActor)
			if sc, ok := security.FromContext(ctx); ok {
				principal = sc.Actor
			}
		}
		logAttrs := make([]slog.Attr, 0, len(attrs)+len(logIDs)+2)
		logAttrs = append(logAttrs, slog.String("event", event), slog.Any("principal", principal))
		for _, attr := range attrs {
			logAttrs = append(logAttrs, slog.Any(string(attr.Key), attr.Value.AsInterface()))
		}
		logAttrs = append(logAttrs, logIDs...)
		h.Logger.LogAttrs(ctx, level, "access evaluation completed", logAttrs...)
	}
}
