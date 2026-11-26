package opa

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	telemetryScope = "tokenexchange"
	evaluateSpan   = "tokenexchange.impersonation.opa_evaluate"

	attrRule        = "impersonation.rule"
	attrAction      = "impersonation.authorization.action"
	attrErrorType   = "error.type"
	attrOutcome     = "outcome"
	attrReason      = "reason"
	attrBundle      = "bundle"
	logRateInterval = time.Minute
)

// evaluationAction is the impersonation.authorization.action attribute of an OPA evaluation.
type evaluationAction string

const (
	// actionAllow is set when the decision allowed the request.
	actionAllow evaluationAction = "allow"
	// actionDeny is set when the decision explicitly denied the request; later rules still run.
	actionDeny evaluationAction = "deny"
	// actionInvalid is set when the decision was undefined, malformed, or an unsupported action;
	// the request failed with server_error.
	actionInvalid evaluationAction = "invalid"
	// actionError is set when evaluation failed or timed out; the request failed with server_error.
	actionError evaluationAction = "error"
	// actionCanceled is set when the caller canceled the request during evaluation.
	actionCanceled evaluationAction = "canceled"
)

// evaluationErrorType is the error.type attribute of a failed OPA evaluation.
type evaluationErrorType string

const (
	// errorTypeDecisionInvalid is set when the decision was not exactly one allow or deny object.
	errorTypeDecisionInvalid evaluationErrorType = "opa_decision_invalid"
	// errorTypeEvaluationFailed is set when evaluation raised an error or timed out.
	errorTypeEvaluationFailed evaluationErrorType = "opa_evaluation_failed"
)

// revisionOutcome is the outcome attribute of a candidate revision.
type revisionOutcome string

const (
	// revisionAdmitted is set when a candidate became the active revision.
	revisionAdmitted revisionOutcome = "admitted"
	// revisionRejected is set when a candidate was rejected and the previous revision stayed
	// active; alert on it.
	revisionRejected revisionOutcome = "rejected"
)

// downloadOutcome is the outcome attribute of a bundle download attempt.
type downloadOutcome string

const (
	// downloadSuccess is set when the bundle was downloaded or was unchanged.
	downloadSuccess downloadOutcome = "success"
	// downloadFailure is set when the download failed; the active revision stays and ages.
	downloadFailure downloadOutcome = "failure"
)

// downloadErrorKind classifies a failed bundle download for the lifecycle log.
type downloadErrorKind string

const (
	// downloadHTTPStatus is set when the bundle service answered with an error status.
	downloadHTTPStatus downloadErrorKind = "http_status"
	// downloadTransport is set when the request failed before a response.
	downloadTransport downloadErrorKind = "transport"
	// downloadInvalidBundle is set when OPA downloaded the bundle but could not activate it.
	downloadInvalidBundle downloadErrorKind = "invalid_bundle"
)

// telemetry owns the adapter's span, instruments, and lifecycle log records. Instrument
// creation failures are logged once and never affect issuance.
type telemetry struct {
	logger    *slog.Logger
	revisions metric.Int64Counter
	downloads metric.Int64Counter

	mu            sync.Mutex
	registrations []metric.Registration
	lastLog       map[string]time.Time
}

func newTelemetry(logger *slog.Logger) *telemetry {
	t := &telemetry{logger: logger, lastLog: map[string]time.Time{}}
	meter := otel.Meter(telemetryScope)
	var err error
	t.revisions, err = meter.Int64Counter("tokenexchange.impersonation_opa.revisions",
		metric.WithDescription("Candidate impersonation policy revisions by outcome and reason"),
		metric.WithUnit("{revision}"))
	t.instrumentFailed(err)
	t.downloads, err = meter.Int64Counter("tokenexchange.impersonation_opa.bundle_downloads",
		metric.WithDescription("Impersonation policy bundle download attempts by bundle and outcome"),
		metric.WithUnit("{download}"))
	t.instrumentFailed(err)
	return t
}

func (t *telemetry) instrumentFailed(err error) {
	if err != nil {
		t.logger.Warn("impersonation policy instrument unavailable", "error_kind", "instrument_creation")
	}
}

// observeRevisionAge registers the revision age gauge, read per export from admittedAt.
func (t *telemetry) observeRevisionAge(admittedAt func() (time.Time, bool)) {
	meter := otel.Meter(telemetryScope)
	gauge, err := meter.Float64ObservableGauge("tokenexchange.impersonation_opa.revision_age",
		metric.WithDescription("Seconds since the active impersonation policy revision was admitted"),
		metric.WithUnit("s"))
	if err != nil {
		t.instrumentFailed(err)
		return
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		if at, ok := admittedAt(); ok {
			o.ObserveFloat64(gauge, time.Since(at).Seconds())
		}
		return nil
	}, gauge)
	t.register(registration, err)
}

// observeDownloadAge registers the per-bundle download age gauge.
func (t *telemetry) observeDownloadAge(lastSuccess func() map[string]time.Time) {
	meter := otel.Meter(telemetryScope)
	gauge, err := meter.Float64ObservableGauge("tokenexchange.impersonation_opa.bundle_download_age",
		metric.WithDescription("Seconds since each impersonation policy bundle was last downloaded successfully"),
		metric.WithUnit("s"))
	if err != nil {
		t.instrumentFailed(err)
		return
	}
	registration, err := meter.RegisterCallback(func(_ context.Context, o metric.Observer) error {
		for bundle, at := range lastSuccess() {
			o.ObserveFloat64(gauge, time.Since(at).Seconds(), metric.WithAttributes(attribute.String(attrBundle, bundle)))
		}
		return nil
	}, gauge)
	t.register(registration, err)
}

func (t *telemetry) register(registration metric.Registration, err error) {
	if err != nil {
		t.instrumentFailed(err)
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.registrations = append(t.registrations, registration)
}

// close unregisters the gauge callbacks.
func (t *telemetry) close() {
	t.mu.Lock()
	registrations := t.registrations
	t.registrations = nil
	t.mu.Unlock()
	for _, registration := range registrations {
		_ = registration.Unregister()
	}
}

// endEvaluation records the evaluation outcome. Caller cancellation is an outcome, not an error.
func endEvaluation(span trace.Span, action evaluationAction, errorType evaluationErrorType) {
	span.SetAttributes(attribute.String(attrAction, string(action)))
	if errorType != "" {
		span.SetAttributes(attribute.String(attrErrorType, string(errorType)))
		span.SetStatus(codes.Error, "impersonation policy evaluation failed")
	}
	span.End()
}

func (t *telemetry) revisionAdmitted(rev *revision, bundleCount, ruleCount int, duration time.Duration) {
	t.revisions.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String(attrOutcome, string(revisionAdmitted)), attribute.String(attrReason, string(reasonNone))))
	t.logger.Info("impersonation policy revision admitted",
		"event", "oauth2_server.impersonation_policy.revision_admitted",
		"policy_revision", rev.label,
		"policy_admission_id", rev.admissionID,
		"bundle_count", bundleCount,
		"rule_count", ruleCount,
		"duration_ms", duration.Milliseconds())
}

func (t *telemetry) revisionRejected(rej *rejection, label string) {
	t.revisions.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String(attrOutcome, string(revisionRejected)), attribute.String(attrReason, string(rej.reason))))
	attrs := []any{
		"event", "oauth2_server.impersonation_policy.revision_rejected",
		"policy_revision", label,
		"reason", string(rej.reason),
		"error_kind", string(rej.reason),
	}
	if rej.rule != "" {
		attrs = append(attrs, "rule", rej.rule)
	}
	if rej.builtin != "" {
		attrs = append(attrs, "builtin", rej.builtin)
	}
	if rej.module != "" {
		attrs = append(attrs, "module", rej.module)
	}
	t.logger.Warn("impersonation policy revision rejected", attrs...)
}

func (t *telemetry) startupFailed(reason, rule string, duration time.Duration) {
	attrs := []any{
		"event", "oauth2_server.impersonation_policy.startup_failed",
		"reason", reason,
		"duration_ms", duration.Milliseconds(),
	}
	if rule != "" {
		attrs = append(attrs, "rule", rule)
	}
	t.logger.Error("impersonation policy not ready", attrs...)
}

func (t *telemetry) localMode(label string, fileCount int) {
	t.logger.Info("impersonation policy loaded from local bundle",
		"event", "oauth2_server.impersonation_policy.local_mode",
		"policy_revision", label,
		"file_count", fileCount,
		"updates", "restart_only")
}

func (t *telemetry) bundleDownload(bundle string, outcome downloadOutcome, kind downloadErrorKind) {
	t.downloads.Add(context.Background(), 1, metric.WithAttributes(
		attribute.String(attrBundle, bundle), attribute.String(attrOutcome, string(outcome))))
	if outcome == downloadFailure && t.allowLog("download:"+bundle) {
		t.logger.Warn("impersonation policy bundle download failed",
			"event", "oauth2_server.impersonation_policy.bundle_download_failed",
			"bundle", bundle,
			"error_kind", string(kind))
	}
}

func (t *telemetry) tokenUnavailable(reason string) {
	if t.allowLog("token") {
		t.logger.Warn("impersonation policy bundle token unavailable",
			"event", "oauth2_server.impersonation_policy.token_unavailable",
			"reason", reason)
	}
}

// allowLog rate-limits a lifecycle record to once per minute per key.
func (t *telemetry) allowLog(key string) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := time.Now()
	if last, ok := t.lastLog[key]; ok && now.Sub(last) < logRateInterval {
		return false
	}
	t.lastLog[key] = now
	return true
}
