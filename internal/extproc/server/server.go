// Package server implements the Envoy ExtProc gRPC server for metadata-driven token exchange.
// The Server type validates token-exchange metadata, performs RFC 8693 token exchange,
// and replaces the Authorization header after success. In OPA mode, body-bearing requests
// exchange in the RequestHeaders phase and evaluate policy in the RequestBody phase;
// header-only requests evaluate policy first.
package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/url"
	"strings"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	extprocfilterv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/ext_proc/v3"
	extprocv3 "github.com/envoyproxy/go-control-plane/envoy/service/ext_proc/v3"
	httpv3 "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/authorization"
	extprocconfig "github.com/agentic-identity-broker/agentic-identity-broker/internal/extproc/config"
)

const (
	agentgatewayProtocolMetadataKey = "agentgateway"
	agentgatewayProtocolFieldKey    = "protocol"
	agentgatewayMCPServerFieldKey   = "mcp_server"
	tokenExchangeMetadataNamespace  = "aib.tokenexchange"
	subjectTokenFieldKey            = "subject_token"
	resourceURIFieldKey             = "resource_uri"
)

// ExchangeResult holds the result of a successful token exchange.
type ExchangeResult struct {
	Token                 string
	GrantedPermissionSets map[string][]string
}

// Exchanger performs RFC 8693 token exchange with in-memory caching.
// Implementations must be safe for concurrent use.
type Exchanger interface {
	// Exchange exchanges subjectToken for a downstream token scoped to resourceURI.
	// ctx carries trace context and deadlines and must be passed to outbound HTTP requests.
	// Returns an ExchangeResult containing the token and any granted permission sets, or an error.
	Exchange(ctx context.Context, subjectToken, resourceURI string) (ExchangeResult, error)
	// Shutdown releases any resources held by the exchanger (e.g., background goroutines).
	Shutdown()
}

// Server implements the Envoy ExternalProcessorServer gRPC interface.
// It intercepts request headers, performs token exchange, and replaces
// the Authorization header before the request reaches the upstream.
// When an authorizer is configured, body-bearing requests exchange in the
// headers phase and evaluate OPA in the body phase; header-only requests do the reverse.
type Server struct {
	extprocv3.UnimplementedExternalProcessorServer
	cfg             *extprocconfig.Config
	exchanger       Exchanger
	authorizer      authorization.Authorizer // nil when OPA authorization is disabled
	logger          *slog.Logger
	requestCounter  metric.Int64Counter
	requestDuration metric.Float64Histogram
}

// requestState holds per-stream state accumulated from the RequestHeaders phase
// that is needed when processing the RequestBody phase (OPA mode only).
type requestState struct {
	subjectToken          string
	resourceURI           string
	headers               map[string]string
	protocol              string
	targetServerName      string
	grantedPermissionSets map[string][]string
	requestContext        context.Context
	finishObservation     func(outcome, resourceURI, errorType string)
}

type tokenExchangeInput struct {
	subjectToken string
	resourceURI  string
}

type inputRejection struct {
	code   string
	reason string
}

type requestLoggerKey struct{}

type requestTraceIDKey struct{}

// NewServer creates a new ExtProc Server without OPA authorization.
// cfg provides the service configuration; exchanger performs token exchange;
// logger is used for structured logging.
// Metric instruments are obtained from the globally registered MeterProvider so
// that tests can inject a ManualReader-backed provider before calling NewServer.
func NewServer(cfg *extprocconfig.Config, exchanger Exchanger, logger *slog.Logger) *Server {
	meter := otel.GetMeterProvider().Meter("extproc")
	requestCounter, err := meter.Int64Counter("extproc.token_exchange.requests",
		metric.WithDescription("Total number of token exchange requests processed by ExtProc"))
	if err != nil {
		logger.Warn("failed to create request counter instrument", "error", err)
	}
	requestDuration, err := meter.Float64Histogram("extproc.token_exchange.duration",
		metric.WithDescription("Duration of token exchange requests in seconds"),
		metric.WithUnit("s"))
	if err != nil {
		logger.Warn("failed to create request duration instrument", "error", err)
	}
	return &Server{
		cfg:             cfg,
		exchanger:       exchanger,
		logger:          logger,
		requestCounter:  requestCounter,
		requestDuration: requestDuration,
	}
}

// NewServerWithAuthorizer creates a new ExtProc Server with OPA authorization enabled.
// Body-bearing requests exchange in the headers phase, then OPA evaluates in the body phase.
// Header-only requests evaluate OPA first and exchange only after allow.
//
// This constructor is used by Phase 3+ implementation and E2E tests.
func NewServerWithAuthorizer(cfg *extprocconfig.Config, exchanger Exchanger, authorizer authorization.Authorizer, logger *slog.Logger) *Server {
	srv := NewServer(cfg, exchanger, logger)
	srv.authorizer = authorizer
	return srv
}

func (s *Server) extractTraceContext(ctx context.Context, headers *extprocv3.HttpHeaders) context.Context {
	if !s.cfg.Telemetry.Enabled {
		return ctx
	}
	normalizeTraceparentHeaders(headers)
	return otel.GetTextMapPropagator().Extract(ctx, (*headerCarrier)(headers))
}

func normalizeTraceparentHeaders(headers *extprocv3.HttpHeaders) {
	if headers == nil || headers.Headers == nil {
		return
	}

	var selected *corev3.HeaderValue
	for _, header := range headers.Headers.Headers {
		if !strings.EqualFold(header.Key, "traceparent") || selected != nil {
			continue
		}
		value := header.Value
		if len(header.RawValue) > 0 {
			value = string(header.RawValue)
		}
		candidate := propagation.TraceContext{}.Extract(context.Background(), propagation.MapCarrier{"traceparent": value})
		if trace.SpanContextFromContext(candidate).IsValid() {
			selected = header
		}
	}

	normalized := headers.Headers.Headers[:0]
	for _, header := range headers.Headers.Headers {
		if !strings.EqualFold(header.Key, "traceparent") || header == selected {
			normalized = append(normalized, header)
		}
	}
	headers.Headers.Headers = normalized
}

func (s *Server) withRequestLogger(ctx context.Context, actor, callingPeer string) (context.Context, *slog.Logger) {
	if logger, ok := ctx.Value(requestLoggerKey{}).(*slog.Logger); ok && logger != nil {
		return ctx, logger
	}

	if actor == "" {
		actor = "anonymous"
	}

	traceID := traceIDFromContext(ctx)
	if traceID == "" {
		traceID = generateFallbackTraceID()
	}

	ctx = context.WithValue(ctx, requestTraceIDKey{}, traceID)

	logger := s.logger.With("trace_id", traceID, "actor", actor)
	if callingPeer != "" {
		logger = logger.With("calling_peer", callingPeer)
	}

	ctx = context.WithValue(ctx, requestLoggerKey{}, logger)
	return ctx, logger
}

func loggerFromContext(ctx context.Context, fallback *slog.Logger) *slog.Logger {
	if logger, ok := ctx.Value(requestLoggerKey{}).(*slog.Logger); ok && logger != nil {
		return logger
	}

	return fallback
}

func traceIDFromContext(ctx context.Context) string {
	if traceID, ok := ctx.Value(requestTraceIDKey{}).(string); ok && traceID != "" {
		return traceID
	}

	spanContext := trace.SpanContextFromContext(ctx)
	if spanContext.IsValid() {
		return spanContext.TraceID().String()
	}

	return ""
}

func generateFallbackTraceID() string {
	var traceID [16]byte
	if _, err := rand.Read(traceID[:]); err != nil {
		return strings.ReplaceAll(uuid.NewString(), "-", "")
	}

	return hex.EncodeToString(traceID[:])
}

func (s *Server) beginTokenExchangeObservation(ctx context.Context) (context.Context, func(outcome, resourceURI, errorType string)) {
	start := time.Now()
	telemetryEnabled := s.cfg.Telemetry.Enabled
	tracesEnabled := telemetryEnabled && s.cfg.Telemetry.Traces.Enabled
	metricsEnabled := telemetryEnabled && s.cfg.Telemetry.Metrics.Enabled

	var span trace.Span
	if tracesEnabled {
		ctx, span = otel.Tracer("extproc").Start(ctx, "extproc.token_exchange")
	} else {
		span = trace.SpanFromContext(ctx)
	}

	finished := false
	return ctx, func(outcome, resourceURI, errorType string) {
		if finished {
			return
		}
		finished = true

		if resourceURI != "" {
			span.SetAttributes(attribute.String("resource.uri", sanitizeURIForTelemetry(resourceURI)))
		}
		if errorType != "" {
			span.SetAttributes(attribute.String("error.type", errorType))
		}
		span.SetAttributes(attribute.String("outcome", outcome))
		span.End()

		if metricsEnabled {
			outcomeAttr := metric.WithAttributes(attribute.String("outcome", outcome))
			metricCtx := context.WithoutCancel(ctx)
			if s.requestCounter != nil {
				s.requestCounter.Add(metricCtx, 1, outcomeAttr)
			}
			if s.requestDuration != nil {
				s.requestDuration.Record(metricCtx, time.Since(start).Seconds(), outcomeAttr)
			}
		}
	}
}

// Process implements the streaming ExtProc gRPC RPC.
// When OPA is disabled (authorizer == nil), RequestHeaders validates token-exchange metadata,
// exchanges it, and replaces the Authorization header. All other phases pass through unchanged.
//
// When OPA is enabled, body-bearing requests exchange validated metadata in RequestHeaders and
// evaluate policy in RequestBody. Header-only requests evaluate policy first and exchange on allow.
func (s *Server) Process(stream extprocv3.ExternalProcessor_ProcessServer) error {
	// Per-stream OPA state is populated by a preceding RequestHeaders message on the same
	// ExtProc stream and consumed by the body or header-only processing path.
	var state *requestState
	activeCtx := stream.Context()

	for {
		req, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			if status.Code(err) == codes.Canceled {
				return nil
			}
			loggerFromContext(activeCtx, s.logger).DebugContext(activeCtx, "stream recv error", "error", err)
			return err
		}

		var resp *extprocv3.ProcessingResponse

		switch msg := req.Request.(type) {
		case *extprocv3.ProcessingRequest_RequestHeaders:
			activeCtx = s.extractTraceContext(stream.Context(), msg.RequestHeaders)
			activeCtx, _ = s.withRequestLogger(activeCtx, "", "")
			if s.authorizer != nil {
				resp, state = s.processRequestHeadersOPA(activeCtx, msg.RequestHeaders.EndOfStream, req, msg.RequestHeaders)
				if state != nil && msg.RequestHeaders.EndOfStream {
					// No body phase will follow. For MCP only GET (SSE streams) and POST
					// (JSON-RPC) are valid transports. POST without a body is malformed (400).
					// All other methods are unsupported (405).
					if state.protocol == "mcp" && !isMCPHeaderOnlyMethod(state.headers[":method"]) {
						outcome := "invalid_request"
						errorType := "invalid_method"
						if isBodyBearingMethod(state.headers[":method"]) {
							resp = immediateResponse(httpv3.StatusCode_BadRequest,
								`{"error":"invalid_request","error_description":"MCP request must have a body"}`)
							errorType = "missing_body"
						} else {
							resp = immediateResponseWithHeaders(httpv3.StatusCode_MethodNotAllowed,
								`{"error":"invalid_request","error_description":"method not supported for MCP"}`,
								map[string]string{"allow": "GET, POST"})
						}
						if state.finishObservation != nil {
							state.finishObservation(outcome, state.resourceURI, errorType)
						}
						state = nil
					} else {
						requestCtx := stream.Context()
						if state.requestContext != nil {
							requestCtx = state.requestContext
						}
						resp = s.processHeadersOnlyOPA(requestCtx, state)
						state = nil
					}
				}
			} else {
				resp = s.processRequestHeaders(activeCtx, req, msg.RequestHeaders)
			}

		case *extprocv3.ProcessingRequest_RequestBody:
			if state != nil {
				bodyCtx := activeCtx
				if state.requestContext != nil {
					bodyCtx = state.requestContext
				}
				activeCtx = bodyCtx
				resp = s.processRequestBody(bodyCtx, state, msg.RequestBody)
				state = nil // consumed
			} else {
				// No OPA state from a preceding RequestHeaders phase on this stream: echo the
				// body unchanged. This preserves pass-through behavior for non-OPA traffic and
				// for any body message that arrives without prior per-stream state.
				resp = echoRequestBody(msg.RequestBody)
			}

		case *extprocv3.ProcessingRequest_ResponseHeaders:
			resp = passThroughResponseHeaders()

		case *extprocv3.ProcessingRequest_ResponseBody:
			resp = echoResponseBody(msg.ResponseBody)

		case *extprocv3.ProcessingRequest_RequestTrailers:
			resp = passThroughRequestTrailers()

		case *extprocv3.ProcessingRequest_ResponseTrailers:
			resp = passThroughResponseTrailers()

		default:
			resp = passThrough()
		}

		if err := stream.Send(resp); err != nil {
			loggerFromContext(activeCtx, s.logger).InfoContext(activeCtx, "stream send error", "error", err)
			return err
		}

		// After an ImmediateResponse the stream is complete.
		if _, isImmediate := resp.Response.(*extprocv3.ProcessingResponse_ImmediateResponse); isImmediate {
			return nil
		}
	}
}

// processRequestHeaders implements the primary ExtProc processing phase when OPA is disabled.
func (s *Server) processRequestHeaders(ctx context.Context, req *extprocv3.ProcessingRequest, headers *extprocv3.HttpHeaders) *extprocv3.ProcessingResponse {
	start := time.Now()
	outcome := "success"

	telemetryEnabled := s.cfg.Telemetry.Enabled
	tracesEnabled := telemetryEnabled && s.cfg.Telemetry.Traces.Enabled
	metricsEnabled := telemetryEnabled && s.cfg.Telemetry.Metrics.Enabled

	ctx = s.extractTraceContext(ctx, headers)
	var span trace.Span
	if tracesEnabled {
		ctx, span = otel.Tracer("extproc").Start(ctx, "extproc.token_exchange")
	} else {
		span = trace.SpanFromContext(ctx)
	}
	ctx, logger := s.withRequestLogger(ctx, "", "")
	defer func() {
		span.SetAttributes(attribute.String("outcome", outcome))
		span.End()
		if metricsEnabled {
			outcomeAttr := metric.WithAttributes(attribute.String("outcome", outcome))
			metricCtx := context.WithoutCancel(ctx)
			if s.requestCounter != nil {
				s.requestCounter.Add(metricCtx, 1, outcomeAttr)
			}
			if s.requestDuration != nil {
				s.requestDuration.Record(metricCtx, time.Since(start).Seconds(), outcomeAttr)
			}
		}
	}()

	input, rejection := extractTokenExchangeInput(req)
	if rejection != nil {
		outcome = rejection.code
		logger.WarnContext(ctx, "extproc: token-exchange metadata rejected", "reason", rejection.reason)
		span.SetAttributes(attribute.String("error.type", rejection.code))
		return inputRejectionResponse(rejection)
	}

	sanitizedURI := sanitizeURIForTelemetry(input.resourceURI)
	span.SetAttributes(attribute.String("resource.uri", sanitizedURI))

	protocol, _ := extractProtocolFromMetadata(req)
	if protocol == "" {
		protocol = "mcp"
	}

	result, err := s.exchanger.Exchange(ctx, input.subjectToken, input.resourceURI)
	if err != nil {
		resp, mappedOutcome, mappedErrorType := s.exchangeErrorResponse(ctx, "", protocol, input.resourceURI, err)
		outcome = mappedOutcome
		span.SetAttributes(attribute.String("error.type", mappedErrorType))
		return resp
	}
	logger.DebugContext(ctx, "token exchanged successfully", "resource", sanitizedURI)
	return replaceAuthorizationHeader("Bearer " + result.Token)
}

// processRequestHeadersOPA handles the RequestHeaders phase when OPA is enabled.
func (s *Server) processRequestHeadersOPA(ctx context.Context, endOfStream bool, req *extprocv3.ProcessingRequest, headers *extprocv3.HttpHeaders) (*extprocv3.ProcessingResponse, *requestState) {
	ctx = s.extractTraceContext(ctx, headers)
	ctx, finishObservation := s.beginTokenExchangeObservation(ctx)
	ctx, logger := s.withRequestLogger(ctx, "", "")
	finishNow := true
	outcome := "success"
	errorType := ""
	resourceURI := ""
	defer func() {
		if finishNow {
			finishObservation(outcome, resourceURI, errorType)
		}
	}()

	input, rejection := extractTokenExchangeInput(req)
	if rejection != nil {
		outcome = rejection.code
		errorType = rejection.code
		logger.WarnContext(ctx, "extproc OPA: token-exchange metadata rejected", "reason", rejection.reason)
		return inputRejectionResponse(rejection), nil
	}
	resourceURI = input.resourceURI

	protocol, ok := extractProtocolFromMetadata(req)
	if !ok {
		outcome = "authorization_denied"
		errorType = "missing_protocol_metadata"
		logger.WarnContext(ctx, "OPA: protocol metadata absent — rejecting with 403 (misconfiguration)",
			"resource", sanitizeURIForTelemetry(resourceURI))
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"protocol metadata is required when authorization is enabled"}`), nil
	}

	headerMap := extractAllHeaders(headers)
	if protocol == "mcp" && !isMCPHeaderOnlyMethod(headerMap[":method"]) && !isBodyBearingMethod(headerMap[":method"]) {
		outcome = "invalid_request"
		errorType = "invalid_method"
		return immediateResponseWithHeaders(httpv3.StatusCode_MethodNotAllowed,
			`{"error":"invalid_request","error_description":"method not supported for MCP"}`,
			map[string]string{"allow": "GET, POST"}), nil
	}

	// mcp_server metadata is mandatory for MCP requests (FR-004), mirroring the
	// protocol metadata check above. It is not required for non-MCP protocols,
	// since it is never populated for those requests either way (FR-005).
	targetServerName, mcpServerOK := extractMCPServerFromMetadata(req)
	if protocol == "mcp" && !mcpServerOK {
		outcome = "authorization_denied"
		errorType = "missing_target_server_metadata"
		logger.WarnContext(ctx, "OPA: mcp_server metadata absent — rejecting with 403 (misconfiguration)",
			"resource", sanitizeURIForTelemetry(resourceURI))
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"mcp_server metadata is required when authorization is enabled for MCP requests"}`), nil
	}

	state := &requestState{
		subjectToken:      input.subjectToken,
		resourceURI:       resourceURI,
		headers:           headerMap,
		protocol:          protocol,
		targetServerName:  targetServerName,
		requestContext:    ctx,
		finishObservation: finishObservation,
	}

	if endOfStream {
		finishNow = false
		return passThrough(), state
	}

	exchangeResult, exchErr := s.exchanger.Exchange(ctx, input.subjectToken, resourceURI)
	if exchErr != nil {
		resp, mappedOutcome, mappedErrorType := s.exchangeErrorResponse(ctx, "OPA: headers-phase", protocol, resourceURI, exchErr)
		outcome = mappedOutcome
		errorType = mappedErrorType
		return resp, nil
	}
	state.grantedPermissionSets = exchangeResult.GrantedPermissionSets
	logger.DebugContext(ctx, "OPA: token exchanged in headers phase, buffering body for OPA evaluation",
		"resource", sanitizeURIForTelemetry(resourceURI))
	return requestBodyBufferingResponseWithAuth("Bearer " + exchangeResult.Token), state
}

// processRequestBody handles the RequestBody phase when OPA is enabled.
// Token exchange has already occurred in the headers phase; this phase only
// evaluates OPA policy using the body and returns:
//   - allow: echo the request body unchanged (Authorization was already set)
//   - deny: 403 ImmediateResponse with JSON access_denied body
func (s *Server) processRequestBody(ctx context.Context, state *requestState, body *extprocv3.HttpBody) *extprocv3.ProcessingResponse {
	ctx, logger := s.withRequestLogger(ctx, "", "")
	var bodyBytes []byte
	if body != nil {
		bodyBytes = body.Body
	}
	sanitizedURI := sanitizeURIForTelemetry(state.resourceURI)

	if state.protocol == "mcp" && isMCPHeaderOnlyMethod(state.headers[":method"]) {
		return immediateResponseWithHeaders(httpv3.StatusCode_MethodNotAllowed,
			`{"error":"invalid_request","error_description":"method not supported for MCP"}`,
			map[string]string{"allow": "GET, POST"})
	}

	maxSize := s.cfg.Authorization.MaxBodySize
	if maxSize > 0 && len(bodyBytes) > maxSize {
		logger.WarnContext(ctx, "OPA: request body exceeds max_body_size — rejecting",
			"resource", sanitizedURI, "body_size", len(bodyBytes), "max_body_size", maxSize)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"request_too_large","error_description":"request body exceeds maximum allowed size"}`)
	}

	if state.protocol == "mcp" {
		if trimmed := bytes.TrimLeft(bodyBytes, " \t\r\n"); len(trimmed) > 0 && trimmed[0] == '[' {
			return s.processRequestBodyBatch(ctx, state, bodyBytes, body)
		}
	}

	opaInput, buildErr := authorization.BuildOPAInput(state.protocol, bodyBytes, state.headers, state.targetServerName, state.grantedPermissionSets)
	if buildErr != nil {
		logger.WarnContext(ctx, "OPA: failed to parse request body — denying", "resource", sanitizedURI, "error", buildErr)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"failed to parse request protocol"}`)
	}

	decision, err := s.authorizer.Evaluate(ctx, opaInput)
	if err != nil {
		logger.ErrorContext(ctx, "OPA evaluation error — denying", "resource", sanitizedURI, "error", err)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"authorization evaluation failed"}`)
	}

	if decision.Action != "allow" {
		logger.InfoContext(ctx, "OPA denied request", "reasons", decision.Reasons, "resource", sanitizedURI)
		return accessDeniedResponse(decision.Reasons)
	}

	logger.DebugContext(ctx, "OPA allowed request, echoing body", "resource", sanitizedURI)
	return echoRequestBody(body)
}

// processHeadersOnlyOPA handles header-only requests in OPA mode (end_of_stream=true in
// the headers phase, meaning no body phase will follow). It evaluates OPA with an empty
// body and performs token exchange, returning a headers-phase response on success.
// Deny and exchange errors return ImmediateResponse, identical to the body path.
func (s *Server) processHeadersOnlyOPA(ctx context.Context, state *requestState) *extprocv3.ProcessingResponse {
	ctx, logger := s.withRequestLogger(ctx, "", "")
	outcome := "success"
	errorType := ""
	if state.finishObservation != nil {
		defer func() {
			state.finishObservation(outcome, state.resourceURI, errorType)
		}()
	}
	sanitizedURI := sanitizeURIForTelemetry(state.resourceURI)

	opaInput, buildErr := authorization.BuildOPAInputHeadersOnly(state.protocol, state.headers, state.targetServerName)
	if buildErr != nil {
		outcome = "authorization_denied"
		errorType = "invalid_request"
		logger.WarnContext(ctx, "OPA: failed to build input for header-only request — denying", "resource", sanitizedURI, "error", buildErr)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"failed to parse request protocol"}`)
	}

	decision, err := s.authorizer.Evaluate(ctx, opaInput)
	if err != nil {
		outcome = "authorization_denied"
		errorType = "evaluation_error"
		logger.ErrorContext(ctx, "OPA evaluation error — denying", "resource", sanitizedURI, "error", err)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"authorization evaluation failed"}`)
	}

	if decision.Action != "allow" {
		outcome = "authorization_denied"
		errorType = "access_denied"
		logger.InfoContext(ctx, "OPA denied header-only request", "reasons", decision.Reasons, "resource", sanitizedURI)
		return accessDeniedResponse(decision.Reasons)
	}

	exchangeResult, exchErr := s.exchanger.Exchange(ctx, state.subjectToken, state.resourceURI)
	if exchErr != nil {
		resp, mappedOutcome, mappedErrorType := s.exchangeErrorResponse(ctx, "OPA: header-only", state.protocol, state.resourceURI, exchErr)
		outcome = mappedOutcome
		errorType = mappedErrorType
		return resp
	}
	logger.DebugContext(ctx, "OPA allowed header-only request, token exchanged successfully", "resource", sanitizedURI)
	return replaceAuthorizationHeader("Bearer " + exchangeResult.Token)
}

// processRequestBodyBatch evaluates a JSON-RPC batch body (FR-023).
// Each element is evaluated independently; if any is denied the entire batch is denied
// with a 403 response that aggregates reasons from all denying messages.
// Original raw JSON bytes are passed to BuildOPAInput to preserve any extra top-level fields.
func (s *Server) processRequestBodyBatch(ctx context.Context, state *requestState, bodyBytes []byte, body *extprocv3.HttpBody) *extprocv3.ProcessingResponse {
	ctx, logger := s.withRequestLogger(ctx, "", "")
	var rawMessages []json.RawMessage
	sanitizedURI := sanitizeURIForTelemetry(state.resourceURI)
	if err := json.Unmarshal(bodyBytes, &rawMessages); err != nil {
		logger.WarnContext(ctx, "OPA: failed to parse batch body — denying", "resource", sanitizedURI, "error", err)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"failed to parse batch request"}`)
	}
	if len(rawMessages) == 0 {
		logger.WarnContext(ctx, "OPA: empty batch body — rejecting as malformed", "resource", sanitizedURI)
		return immediateResponse(httpv3.StatusCode_Forbidden,
			`{"error":"access_denied","error_description":"empty batch is not valid JSON-RPC 2.0"}`)
	}

	var (
		denied      bool
		denyReasons []string
	)
	for i, raw := range rawMessages {
		if _, err := authorization.ParseMCPMessage(raw); err != nil {
			logger.WarnContext(ctx, "OPA: invalid batch element — denying", "resource", sanitizedURI, "index", i, "error", err)
			denied = true
			denyReasons = append(denyReasons, "batch element could not be evaluated")
			continue
		}
		opaInput, buildErr := authorization.BuildOPAInput(state.protocol, raw, state.headers, state.targetServerName, state.grantedPermissionSets)
		if buildErr != nil {
			logger.WarnContext(ctx, "OPA: failed to build input for batch element — denying", "resource", sanitizedURI, "index", i, "error", buildErr)
			denied = true
			denyReasons = append(denyReasons, "failed to parse batch element")
			continue
		}
		decision, evalErr := s.authorizer.Evaluate(ctx, opaInput)
		if evalErr != nil {
			logger.ErrorContext(ctx, "OPA evaluation error for batch element — denying", "resource", sanitizedURI, "index", i, "error", evalErr)
			denied = true
			denyReasons = append(denyReasons, "authorization evaluation failed")
			continue
		}
		if decision.Action != "allow" {
			denied = true
			if len(decision.Reasons) > 0 {
				denyReasons = append(denyReasons, decision.Reasons...)
			} else {
				denyReasons = append(denyReasons, "access denied")
			}
		}
	}

	if denied {
		logger.InfoContext(ctx, "OPA denied batch request", "reasons", denyReasons, "resource", sanitizedURI)
		return accessDeniedResponse(denyReasons)
	}

	logger.DebugContext(ctx, "OPA allowed batch, echoing body", "resource", sanitizedURI)
	return echoRequestBody(body)
}

// tokenExchangeErrorResponse maps a token exchange error to the appropriate ImmediateResponse.
// Handles ErrAssertionExpired (503), ErrCircuitOpen (503), and generic failures (500).
// BrokerExchangeError with error_uri is handled by callers before reaching this function.
func (s *Server) tokenExchangeErrorResponse(ctx context.Context, err error, resourceURI string) *extprocv3.ProcessingResponse {
	logger := loggerFromContext(ctx, s.logger)
	sanitizedURI := sanitizeURIForTelemetry(resourceURI)
	if errors.Is(err, ErrAssertionExpired) {
		logger.ErrorContext(ctx, "token exchange failed: client assertion expired — background refresh may have failed",
			"resource", sanitizedURI)
		return immediateResponse(httpv3.StatusCode_ServiceUnavailable,
			`{"error":"service_unavailable","error_description":"client assertion expired"}`)
	}
	if errors.Is(err, ErrCircuitOpen) {
		logger.WarnContext(ctx, "token exchange rejected: circuit breaker is open",
			"resource", sanitizedURI)
		return immediateResponse(httpv3.StatusCode_ServiceUnavailable,
			`{"error":"service_unavailable","error_description":"circuit breaker is open"}`)
	}
	logger.ErrorContext(ctx, "token exchange failed", "resource", sanitizedURI, "error", err)
	return immediateResponse(httpv3.StatusCode_InternalServerError,
		`{"error":"token_exchange_failed","error_description":"token exchange request failed"}`)
}

func accessDeniedResponse(reasons []string) *extprocv3.ProcessingResponse {
	body403, _ := json.Marshal(map[string]string{
		"error":             "access_denied",
		"error_description": strings.Join(reasons, "; "),
	})
	return immediateResponse(httpv3.StatusCode_Forbidden, string(body403))
}

func (s *Server) exchangeErrorResponse(ctx context.Context, phase, protocol, resourceURI string, err error) (*extprocv3.ProcessingResponse, string, string) {
	logger := loggerFromContext(ctx, s.logger)
	sanitizedURI := sanitizeURIForTelemetry(resourceURI)
	var brokerErr *BrokerExchangeError
	if errors.As(err, &brokerErr) && brokerErr.ErrorURI != "" && !isTransientBrokerError(err) {
		if protocol == "mcp" {
			msg := "token exchange requires re-authentication — returning URLElicitationRequiredError"
			if phase != "" {
				msg = phase + " token exchange requires re-auth — returning URLElicitationRequiredError"
			}
			logger.InfoContext(ctx, msg,
				"resource", sanitizedURI,
				"code", brokerErr.Code,
				"error_uri", brokerErr.ErrorURI)
			return urlElicitationResponse(brokerErr, nil), "exchange_failure", brokerErr.Code
		}

		msg := "token exchange requires re-authentication but protocol is non-MCP — returning 503"
		if phase != "" {
			msg = phase + " token exchange requires re-auth but protocol is non-MCP — returning 503"
		}
		logger.WarnContext(ctx, msg, "resource", sanitizedURI, "protocol", protocol)
		return immediateResponse(httpv3.StatusCode_ServiceUnavailable,
			`{"error":"service_unavailable","error_description":"token exchange requires re-authentication"}`), "exchange_failure", brokerErr.Code
	}

	switch {
	case errors.Is(err, ErrAssertionExpired):
		return s.tokenExchangeErrorResponse(ctx, err, resourceURI), "assertion_expired", "assertion_expired"
	case errors.Is(err, ErrCircuitOpen):
		return s.tokenExchangeErrorResponse(ctx, err, resourceURI), "circuit_open", "circuit_open"
	default:
		return s.tokenExchangeErrorResponse(ctx, err, resourceURI), "exchange_failure", "exchange_failure"
	}
}

func extractTokenExchangeInput(req *extprocv3.ProcessingRequest) (tokenExchangeInput, *inputRejection) {
	if req == nil || req.MetadataContext == nil {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_subject_token", reason: "metadata context is missing"}
	}

	metadata, ok := req.MetadataContext.FilterMetadata[tokenExchangeMetadataNamespace]
	if !ok || metadata == nil || metadata.Fields == nil {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_subject_token", reason: "token-exchange namespace is missing"}
	}

	subjectToken, reason := metadataStringField(metadata.Fields, subjectTokenFieldKey)
	if reason != "" {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_subject_token", reason: reason}
	}
	if strings.TrimSpace(subjectToken) == "" {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_subject_token", reason: "subject token is blank"}
	}
	if len(subjectToken) >= len("bearer ") && strings.EqualFold(subjectToken[:len("bearer ")], "bearer ") {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_subject_token", reason: "subject token has a bearer scheme"}
	}

	resourceURI, reason := metadataStringField(metadata.Fields, resourceURIFieldKey)
	if reason != "" {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_resource", reason: reason}
	}
	if strings.TrimSpace(resourceURI) == "" {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_resource", reason: "resource URI is blank"}
	}
	if err := validateResourceURI(resourceURI); err != nil {
		return tokenExchangeInput{}, &inputRejection{code: "invalid_resource", reason: status.Convert(err).Message()}
	}

	return tokenExchangeInput{subjectToken: subjectToken, resourceURI: resourceURI}, nil
}

func metadataStringField(fields map[string]*structpb.Value, key string) (string, string) {
	value, ok := fields[key]
	if !ok || value == nil {
		return "", "field is missing"
	}

	stringValue, ok := value.GetKind().(*structpb.Value_StringValue)
	if !ok {
		return "", "field is not a string"
	}

	return stringValue.StringValue, ""
}

// extractProtocolFromMetadata extracts the agentgateway protocol value from the
// MetadataContext FilterMetadata.
//
// Returns (protocol, true) when the agentgateway metadata key is present and contains
// a non-empty protocol string. The returned protocol may be any value (e.g. "mcp",
// "a2a", or an unrecognised string).
//
// Returns ("", false) when the metadata is entirely absent — i.e. MetadataContext is
// nil, the "agentgateway" key is missing, or the protocol field is absent or empty.
// Callers in OPA mode MUST reject the request with 403 on a false return (FR-003).
//
// The metadata structure is: FilterMetadata["agentgateway"]["protocol"] = "<type>".
func extractProtocolFromMetadata(req *extprocv3.ProcessingRequest) (string, bool) {
	if req.MetadataContext == nil {
		return "", false
	}
	agwMeta, ok := req.MetadataContext.FilterMetadata[agentgatewayProtocolMetadataKey]
	if !ok || agwMeta == nil {
		return "", false
	}
	fields := agwMeta.GetFields()
	if fields == nil {
		return "", false
	}
	protoVal, ok := fields[agentgatewayProtocolFieldKey]
	if !ok || protoVal == nil {
		return "", false
	}
	// GetStringValue returns "" for non-string protobuf Values.
	v := protoVal.GetStringValue()
	if v == "" {
		return "", false
	}
	return v, true
}

// extractMCPServerFromMetadata extracts the agentgateway mcp_server value from the
// MetadataContext FilterMetadata. This is agentgateway routing metadata identifying
// which downstream MCP server the request targets — not an MCP protocol field.
//
// Like extractProtocolFromMetadata, callers in OPA mode MUST reject the request with
// 403 on a false return when protocol == "mcp" (FR-004) — mcp_server is mandatory for
// MCP requests. It is not required for non-MCP protocols, since it is never populated
// for those requests either way (FR-005).
//
// Returns (server, true) when the agentgateway metadata key is present and contains a
// non-empty mcp_server string. Returns ("", false) when MetadataContext is entirely
// nil, the "agentgateway" key is missing, or the mcp_server field is absent or empty.
//
// The metadata structure is: FilterMetadata["agentgateway"]["mcp_server"] = "<name>".
func extractMCPServerFromMetadata(req *extprocv3.ProcessingRequest) (string, bool) {
	if req.MetadataContext == nil {
		return "", false
	}
	agwMeta, ok := req.MetadataContext.FilterMetadata[agentgatewayProtocolMetadataKey]
	if !ok || agwMeta == nil {
		return "", false
	}
	fields := agwMeta.GetFields()
	if fields == nil {
		return "", false
	}
	serverVal, ok := fields[agentgatewayMCPServerFieldKey]
	if !ok || serverVal == nil {
		return "", false
	}
	v := serverVal.GetStringValue()
	if v == "" {
		return "", false
	}
	return v, true
}

// extractAllHeaders returns all request headers as a lowercase-key map.
func extractAllHeaders(headers *extprocv3.HttpHeaders) map[string]string {
	result := make(map[string]string)
	if headers == nil || headers.Headers == nil {
		return result
	}
	for _, h := range headers.Headers.Headers {
		key := strings.ToLower(h.Key)
		if len(h.RawValue) > 0 {
			result[key] = string(h.RawValue)
		} else {
			result[key] = h.Value
		}
	}
	return result
}

// requestBodyBufferingResponseWithAuth returns a HeadersResponse that sets the exchanged
// Authorization header and instructs Envoy/proxy to buffer the full request body for the
// body phase. By including the auth mutation in the headers-phase response, proxies that
// only apply header mutations from the first ExtProc response (e.g. agentgateway) will
// correctly forward the exchanged token to the upstream.
func requestBodyBufferingResponseWithAuth(authValue string) *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: []*corev3.HeaderValueOption{
							{
								Header: &corev3.HeaderValue{
									Key:      "authorization",
									RawValue: []byte(authValue),
								},
							},
						},
					},
				},
			},
		},
		ModeOverride: &extprocfilterv3.ProcessingMode{
			RequestBodyMode: extprocfilterv3.ProcessingMode_BUFFERED,
		},
	}
}

// extractHeader returns the first matching header value (case-insensitive key match),
// preferring RawValue over Value. It is used for trace propagation and transport metadata.
func extractHeader(headers *extprocv3.HttpHeaders, name string) string {
	if headers == nil || headers.Headers == nil {
		return ""
	}
	nameLower := strings.ToLower(name)
	for _, h := range headers.Headers.Headers {
		if strings.ToLower(h.Key) == nameLower {
			if len(h.RawValue) > 0 {
				return string(h.RawValue)
			}
			return h.Value
		}
	}
	return ""
}

// validateResourceURI checks that resourceURI is a non-empty absolute URI with
// an HTTP or HTTPS scheme and a non-empty host.
func validateResourceURI(resourceURI string) error {
	if strings.TrimSpace(resourceURI) == "" {
		return status.Error(codes.InvalidArgument, "empty resource URI")
	}
	u, err := url.ParseRequestURI(resourceURI)
	if err != nil {
		return status.Error(codes.InvalidArgument, "invalid resource URI: parse error")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return status.Error(codes.InvalidArgument, "resource URI must have http or https scheme")
	}
	if u.Host == "" {
		return status.Error(codes.InvalidArgument, "resource URI must have a non-empty host")
	}
	return nil
}

// headerCarrier adapts ExtProc headers to the OTel TextMapCarrier interface.
// Get is key-aware: list-valued propagation headers (baggage, tracestate) are
// comma-joined per RFC 9110 §5.2; single-valued headers (traceparent, b3, etc.)
// return first-match only, consistent with http.Header.Get().
type headerCarrier extprocv3.HttpHeaders

// listValuedPropagationHeaders are propagation headers whose spec allows
// multiple header fields to be combined with commas (RFC 9110 §5.2).
var listValuedPropagationHeaders = map[string]bool{
	"baggage":    true,
	"tracestate": true,
}

func (c *headerCarrier) Get(key string) string {
	if !listValuedPropagationHeaders[strings.ToLower(key)] {
		return extractHeader((*extprocv3.HttpHeaders)(c), key)
	}
	if c == nil || c.Headers == nil {
		return ""
	}
	keyLower := strings.ToLower(key)
	var vals []string
	for _, h := range c.Headers.Headers {
		if strings.ToLower(h.Key) == keyLower {
			if len(h.RawValue) > 0 {
				vals = append(vals, string(h.RawValue))
			} else if h.Value != "" {
				vals = append(vals, h.Value)
			}
		}
	}
	return strings.Join(vals, ",")
}

func (c *headerCarrier) Set(key string, value string) {
	// Extraction-only carrier: Set is intentionally a no-op because ExtProc
	// responses don't inject trace headers. If Inject() is called on this
	// carrier (via otel.GetTextMapPropagator().Inject()), injected headers
	// will be silently dropped. Implement Set with header mutation if
	// response header injection becomes needed.
}

func (c *headerCarrier) Keys() []string {
	if c == nil || c.Headers == nil {
		return []string{}
	}
	seen := make(map[string]struct{}, len(c.Headers.Headers))
	keys := make([]string, 0, len(c.Headers.Headers))
	for _, h := range c.Headers.Headers {
		lower := strings.ToLower(h.Key)
		if _, dup := seen[lower]; !dup {
			seen[lower] = struct{}{}
			keys = append(keys, h.Key)
		}
	}
	return keys
}

// sanitizeURIForTelemetry removes caller-controlled credentials, query strings, and
// fragments before recording a URI in telemetry or logs.
func sanitizeURIForTelemetry(resourceURI string) string {
	u, err := url.ParseRequestURI(resourceURI)
	if err != nil {
		return "[invalid resource URI]"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// replaceAuthorizationHeader builds a ProcessingResponse that replaces the
// Authorization header with the provided value.
func replaceAuthorizationHeader(value string) *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{
				Response: &extprocv3.CommonResponse{
					HeaderMutation: &extprocv3.HeaderMutation{
						SetHeaders: []*corev3.HeaderValueOption{
							{
								Header: &corev3.HeaderValue{
									Key:      "authorization",
									RawValue: []byte(value),
								},
							},
						},
					},
				},
			},
		},
	}
}

// immediateResponse builds a ProcessingResponse_ImmediateResponse with the given
// HTTP status code and JSON body. Used for error responses (403, 500, 503).
// isMCPHeaderOnlyMethod reports whether an HTTP method is a valid MCP transport
// that legitimately carries no request body. Only GET is explicitly allowed:
// it is used for SSE stream connections and protocol upgrade handshakes.
func isMCPHeaderOnlyMethod(method string) bool {
	return strings.ToUpper(method) == "GET"
}

// isBodyBearingMethod reports whether an HTTP method is the MCP JSON-RPC transport method.
// MCP uses POST exclusively for JSON-RPC messages.
func isBodyBearingMethod(method string) bool {
	return strings.ToUpper(method) == "POST"
}

func invalidSubjectTokenResponse() *extprocv3.ProcessingResponse {
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable,
		`{"error":"invalid_subject_token","error_description":"subject token metadata is missing or invalid"}`)
}

func invalidResourceResponse() *extprocv3.ProcessingResponse {
	return immediateResponse(httpv3.StatusCode_ServiceUnavailable,
		`{"error":"invalid_resource","error_description":"resource metadata is missing or invalid"}`)
}

func inputRejectionResponse(rejection *inputRejection) *extprocv3.ProcessingResponse {
	if rejection.code == "invalid_resource" {
		return invalidResourceResponse()
	}
	return invalidSubjectTokenResponse()
}

func immediateResponse(code httpv3.StatusCode, body string) *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status: &httpv3.HttpStatus{Code: code},
				Headers: &extprocv3.HeaderMutation{
					SetHeaders: []*corev3.HeaderValueOption{
						{
							Header: &corev3.HeaderValue{
								Key:      "content-type",
								RawValue: []byte("application/json"),
							},
						},
					},
				},
				Body: []byte(body),
			},
		},
	}
}

// immediateResponseWithHeaders builds an ImmediateResponse with extra response headers
// in addition to the standard content-type. Used for 405 responses that require Allow.
func immediateResponseWithHeaders(code httpv3.StatusCode, body string, extra map[string]string) *extprocv3.ProcessingResponse {
	headers := []*corev3.HeaderValueOption{
		{Header: &corev3.HeaderValue{Key: "content-type", RawValue: []byte("application/json")}},
	}
	for k, v := range extra {
		headers = append(headers, &corev3.HeaderValueOption{
			Header: &corev3.HeaderValue{Key: k, RawValue: []byte(v)},
		})
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ImmediateResponse{
			ImmediateResponse: &extprocv3.ImmediateResponse{
				Status:  &httpv3.HttpStatus{Code: code},
				Headers: &extprocv3.HeaderMutation{SetHeaders: headers},
				Body:    []byte(body),
			},
		},
	}
}

// urlElicitationResponse builds a ProcessingResponse_ImmediateResponse with HTTP 200 and
// a JSON-RPC 2.0 URLElicitationRequiredError body (error code -32042).
// Per MCP spec 2025-11-05: returned when a request cannot proceed until the user visits
// a URL for OAuth re-authentication.
// HTTP 200 is used because JSON-RPC errors always travel over HTTP 200.
// rawID is the raw JSON bytes of the request ID (e.g. `42`, `"req-1"`, `null`).
// Pass nil to emit null — correct when the request ID is unknown.
func urlElicitationResponse(brokerErr *BrokerExchangeError, rawID json.RawMessage) *extprocv3.ProcessingResponse {
	elicitErr := mcp.URLElicitationRequiredError{
		Elicitations: []mcp.ElicitationParams{
			{
				Mode:          mcp.ElicitationModeURL,
				ElicitationID: uuid.New().String(),
				URL:           brokerErr.ErrorURI,
				Message:       brokerErr.Description,
			},
		},
	}
	jsonRPCErr := elicitErr.JSONRPCError()
	// Set the request ID using the raw JSON token so any valid JSON-RPC ID type
	// (string, integer, null) is preserved exactly without numeric precision loss.
	// json.RawMessage.MarshalJSON returns its bytes verbatim, so mcp.NewRequestId
	// will serialise the ID token unchanged.
	if rawID != nil {
		jsonRPCErr.ID = mcp.NewRequestId(rawID)
	}
	// Override the generated message with the broker-supplied description so that
	// the client receives context about why re-authentication is required.
	jsonRPCErr.Error.Message = brokerErr.Description

	// Marshal the response. json.Marshal cannot fail for this struct: all fields are strings,
	// ints, or slices thereof — no encoding/json.Marshaler implementations that could error.
	body, _ := json.Marshal(jsonRPCErr)
	return immediateResponse(httpv3.StatusCode_OK, string(body))
}

// passThrough builds a ProcessingResponse_RequestHeaders with no mutations,
// instructing Envoy to pass the request through unchanged.
func passThrough() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestHeaders{
			RequestHeaders: &extprocv3.HeadersResponse{},
		},
	}
}

// echoRequestBody echoes the request body bytes back unchanged using StreamedBodyResponse.
// StreamedBodyResponse is required by agentgateway; BodyMutation_Body causes body loss.
func echoRequestBody(body *extprocv3.HttpBody) *extprocv3.ProcessingResponse {
	streamed := &extprocv3.StreamedBodyResponse{}
	if body != nil {
		streamed.Body = body.Body
		streamed.EndOfStream = body.EndOfStream
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestBody{
			RequestBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					BodyMutation: &extprocv3.BodyMutation{
						Mutation: &extprocv3.BodyMutation_StreamedResponse{
							StreamedResponse: streamed,
						},
					},
				},
			},
		},
	}
}

// passThroughResponseHeaders builds a phase-specific pass-through response for response headers.
func passThroughResponseHeaders() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseHeaders{
			ResponseHeaders: &extprocv3.HeadersResponse{},
		},
	}
}

// echoResponseBody echoes the response body bytes back unchanged using StreamedBodyResponse.
// StreamedBodyResponse is required by agentgateway; BodyMutation_Body causes body loss.
func echoResponseBody(body *extprocv3.HttpBody) *extprocv3.ProcessingResponse {
	streamed := &extprocv3.StreamedBodyResponse{}
	if body != nil {
		streamed.Body = body.Body
		streamed.EndOfStream = body.EndOfStream
	}
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseBody{
			ResponseBody: &extprocv3.BodyResponse{
				Response: &extprocv3.CommonResponse{
					BodyMutation: &extprocv3.BodyMutation{
						Mutation: &extprocv3.BodyMutation_StreamedResponse{
							StreamedResponse: streamed,
						},
					},
				},
			},
		},
	}
}

// passThroughRequestTrailers builds a phase-specific pass-through response for request trailers.
func passThroughRequestTrailers() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_RequestTrailers{
			RequestTrailers: &extprocv3.TrailersResponse{},
		},
	}
}

// passThroughResponseTrailers builds a phase-specific pass-through response for response trailers.
func passThroughResponseTrailers() *extprocv3.ProcessingResponse {
	return &extprocv3.ProcessingResponse{
		Response: &extprocv3.ProcessingResponse_ResponseTrailers{
			ResponseTrailers: &extprocv3.TrailersResponse{},
		},
	}
}
