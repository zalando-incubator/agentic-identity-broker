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
	// OperationExchange means the token exchange with the broker, including the wait for a shared exchange; ExtProc
	// also uses it for an unknown operation.
	OperationExchange Operation = "exchange"
	// OperationAssertionRefresh means the refresh of ExtProc's own client assertion at the OAuth provider; an
	// exchange that finds the assertion expired also reports it.
	OperationAssertionRefresh Operation = "assertion_refresh"
	// OperationConfiguration means building the token exchanger at startup; a failure stops ExtProc, so no span or
	// metric carries this value.
	OperationConfiguration Operation = "configuration"
)

// ErrorKind and Dependency are bounded telemetry values, never provider text.
type ErrorKind string
type Dependency string

const (
	// KindNone means nothing failed; ExtProc never sets `error.type` to it.
	KindNone ErrorKind = "none"
	// KindConfiguration means ExtProc's own configuration is unusable, such as a client assertion type that it does
	// not support or a token endpoint that it cannot build a request for.
	KindConfiguration ErrorKind = "configuration"
	// KindUnavailable means the broker or the OAuth provider was unreachable, timed out or answered 5xx or 429, or
	// ExtProc had no usable grant context.
	KindUnavailable ErrorKind = "unavailable"
	// KindRejected means the broker or the OAuth provider answered with a 4xx error, or ExtProc rejected the
	// request's token-exchange metadata and answered 503.
	KindRejected ErrorKind = "rejected"
	// KindInvalidResponse means the broker or the OAuth provider answered with a body that ExtProc could not use, or
	// the delegation permit carried an invalid grant timestamp.
	KindInvalidResponse ErrorKind = "invalid_response"
	// KindCallerCanceled means the request was canceled, or its deadline passed, while ExtProc waited for the
	// exchange; ExtProc sets no error status for it.
	KindCallerCanceled ErrorKind = "caller_canceled"
	// KindAssertionExpired means ExtProc's own client assertion had expired when an exchange needed it, so ExtProc
	// answered 503 without calling the broker.
	KindAssertionExpired ErrorKind = "assertion_expired"
	// KindCircuitOpen means ExtProc's circuit breaker for the broker was open, or half-open with no free probe, so
	// ExtProc answered 503 without calling the broker; failures inside ExtProc also count toward opening it.
	KindCircuitOpen ErrorKind = "circuit_open"
	// KindInternal means ExtProc could not classify the error; it is also the fallback for an unknown kind.
	KindInternal ErrorKind = "internal"

	// DependencyLocal means ExtProc itself, without an outbound call, such as its configuration or its expired
	// client assertion; ExtProc also uses it for an unknown dependency.
	DependencyLocal Dependency = "local"
	// DependencyBroker means the broker's token endpoint, which ExtProc calls for the token exchange behind its
	// circuit breaker.
	DependencyBroker Dependency = "broker"
	// DependencyOAuthProvider means the OAuth provider that issues ExtProc's client assertion through the client
	// credentials grant, not a provider behind the broker.
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
	logger.LogAttrs(ctx, level, message, //nolint:sloglint // Each caller passes a constant message.
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
