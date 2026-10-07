package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"golang.org/x/oauth2"
)

// Operation identifies the standalone ExtProc operation that failed.
type Operation string

const (
	OperationExchange         Operation = "exchange"
	OperationAssertionRefresh Operation = "assertion_refresh"
	OperationConfiguration    Operation = "configuration"
)

// ErrorKind and Dependency are bounded telemetry values, never provider text.
type ErrorKind string
type Dependency string

const (
	KindNone             ErrorKind = "none"
	KindConfiguration    ErrorKind = "configuration"
	KindUnavailable      ErrorKind = "unavailable"
	KindRejected         ErrorKind = "rejected"
	KindInvalidResponse  ErrorKind = "invalid_response"
	KindCallerCanceled   ErrorKind = "caller_canceled"
	KindAssertionExpired ErrorKind = "assertion_expired"
	KindCircuitOpen      ErrorKind = "circuit_open"
	KindInternal         ErrorKind = "internal"

	DependencyLocal         Dependency = "local"
	DependencyBroker        Dependency = "broker"
	DependencyOAuthProvider Dependency = "oauth_provider"
)

// ErrorMetadata is an immutable, credential-free diagnostic snapshot.
// Provider descriptions, URLs, bodies, headers and causes are deliberately absent.
type ErrorMetadata struct {
	operation  Operation
	kind       ErrorKind
	dependency Dependency
	statusCode int
	oauthCode  string
}

func NewErrorMetadata(operation Operation, kind ErrorKind, dependency Dependency, statusCode int, oauthCode string) ErrorMetadata {
	switch operation {
	case OperationExchange, OperationAssertionRefresh, OperationConfiguration:
	default:
		operation = OperationExchange
	}
	switch kind {
	case KindNone, KindConfiguration, KindUnavailable, KindRejected, KindInvalidResponse, KindCallerCanceled, KindAssertionExpired, KindCircuitOpen, KindInternal:
	default:
		kind = KindInternal
	}
	switch dependency {
	case DependencyLocal, DependencyBroker, DependencyOAuthProvider:
	default:
		dependency = DependencyLocal
	}
	if statusCode < 100 || statusCode > 599 {
		statusCode = 0
	}
	return ErrorMetadata{operation: operation, kind: kind, dependency: dependency, statusCode: statusCode, oauthCode: safeOAuthCode(oauthCode)}
}

func (m ErrorMetadata) Operation() Operation   { return m.operation }
func (m ErrorMetadata) Kind() ErrorKind        { return m.kind }
func (m ErrorMetadata) Dependency() Dependency { return m.dependency }
func (m ErrorMetadata) StatusCode() int        { return m.statusCode }
func (m ErrorMetadata) OAuthCode() string      { return m.oauthCode }

// OperationError retains the original cause for errors.Is/As, but its string is
// exclusively bounded metadata. Telemetry must use Metadata(), not the cause.
type OperationError struct {
	metadata   ErrorMetadata
	cause      error
	diagnostic Diagnostic
}

func NewOperationError(metadata ErrorMetadata, cause error) *OperationError {
	metadata = NewErrorMetadata(metadata.operation, metadata.kind, metadata.dependency, metadata.statusCode, metadata.oauthCode)
	return &OperationError{metadata: metadata, cause: cause, diagnostic: diagnosticForMetadata(metadata, cause)}
}

func (e *OperationError) Metadata() ErrorMetadata { return e.metadata }
func (e *OperationError) Unwrap() error           { return e.cause }
func (e *OperationError) Diagnostic() Diagnostic  { return e.diagnostic }
func (e *OperationError) WithDiagnostic(d Diagnostic) *OperationError {
	if d.Outcome() == OutcomeSuccess {
		d = SuccessDiagnostic(d.ExchangeKind())
	} else {
		d = NewDiagnostic(d.Stage(), d.Detail()).WithExchangeKind(d.ExchangeKind())
	}
	return &OperationError{metadata: e.metadata, cause: e.cause, diagnostic: d}
}
func (e *OperationError) Error() string {
	m := e.metadata
	return fmt.Sprintf("extproc %s failed: %s (%s, status=%d, code=%s)", m.operation, m.kind, m.dependency, m.statusCode, m.oauthCode)
}

func safeOAuthCode(code string) string {
	switch code {
	case "invalid_request", "invalid_client", "invalid_grant", "unauthorized_client", "unsupported_grant_type", "invalid_scope", "invalid_target", "invalid_token", "access_denied", "server_error", "temporarily_unavailable", "slow_down", "reauth_required":
		return code
	default:
		return "unknown"
	}
}

func responseErrorKind(statusCode int) ErrorKind {
	if statusCode == http.StatusTooManyRequests || statusCode >= 500 {
		return KindUnavailable
	}
	return KindRejected
}

func metadataFromError(ctx context.Context, err error) ErrorMetadata {
	var operationErr *OperationError
	if errors.As(err, &operationErr) {
		return operationErr.Metadata()
	}
	if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
		return NewErrorMetadata(OperationExchange, KindCallerCanceled, DependencyLocal, 0, "")
	}
	var brokerErr *BrokerExchangeError
	if errors.As(err, &brokerErr) {
		return brokerErr.Metadata()
	}
	switch {
	case errors.Is(err, ErrAssertionExpired):
		return NewErrorMetadata(OperationAssertionRefresh, KindAssertionExpired, DependencyLocal, 0, "")
	case errors.Is(err, ErrCircuitOpen):
		return NewErrorMetadata(OperationExchange, KindCircuitOpen, DependencyBroker, 0, "")
	default:
		return NewErrorMetadata(OperationExchange, KindInternal, DependencyLocal, 0, "")
	}
}

func assertionErrorMetadata(err error) ErrorMetadata {
	statusCode, code, kind := 0, "", KindInvalidResponse
	var networkErr net.Error
	if errors.As(err, &networkErr) || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		kind = KindUnavailable
	}
	var retrieveErr *oauth2.RetrieveError
	if errors.As(err, &retrieveErr) {
		if retrieveErr.Response != nil {
			statusCode = retrieveErr.Response.StatusCode
		}
		code = retrieveErr.ErrorCode
		kind = responseErrorKind(statusCode)
		if statusCode == http.StatusOK {
			kind = KindInvalidResponse
		}
	}
	return NewErrorMetadata(OperationAssertionRefresh, kind, DependencyOAuthProvider, statusCode, code)
}

func logOperationError(ctx context.Context, logger *slog.Logger, level slog.Level, message string, metadata ErrorMetadata, diagnostic Diagnostic) {
	logger.LogAttrs(ctx, level, message,
		slog.String("operation", string(metadata.Operation())),
		slog.String("error_kind", string(metadata.Kind())),
		slog.String("dependency", string(metadata.Dependency())),
		slog.Int("status", metadata.StatusCode()),
		slog.String("oauth_error_code", metadata.OAuthCode()),
		slog.String("token_exchange.outcome", string(diagnostic.Outcome())),
		slog.String("token_exchange.failure_stage", string(diagnostic.Stage())),
		slog.String("token_exchange.failure_detail", string(diagnostic.Detail())),
		slog.String("token_exchange.recovery_action", string(diagnostic.RecoveryAction())),
		slog.String("token_exchange.recovery_target", string(diagnostic.RecoveryTarget())),
		slog.String("token_exchange.exchange_kind", string(diagnostic.ExchangeKind())))
}
