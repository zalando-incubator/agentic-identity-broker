package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/domainevent"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/storage"
	"github.com/agentic-identity-broker/agentic-identity-broker/internal/ports"
)

const (
	ledgerDurationInstrument = "domain_events.write.duration"
	ledgerFailuresInstrument = "domain_events.write_failures"
)

type ledgerObject map[string]any

type ledgerHandler struct {
	appender     ports.DomainEventAppender
	fallback     *slog.Logger
	writeTimeout time.Duration
	duration     metric.Float64Histogram
	failures     metric.Int64Counter
	attrs        ledgerObject
	definition   domainevent.Definition
	groups       []string
	openGroups   int
}

// NewLedgerHandler persists typed domain events after principal policy. The fallback
// must begin with a context handler and must not include this ledger handler.
func NewLedgerHandler(appender ports.DomainEventAppender, fallback slog.Handler, writeTimeout time.Duration) slog.Handler {
	h := &ledgerHandler{appender: appender, fallback: slog.New(fallback), writeTimeout: writeTimeout}
	meter := otel.Meter("domain_events")
	var err error
	h.duration, err = meter.Float64Histogram(ledgerDurationInstrument,
		metric.WithDescription("Duration of domain event ledger write attempts"), metric.WithUnit("s"))
	if err != nil {
		h.instrumentWarning(ledgerDurationInstrument)
	}
	h.failures, err = meter.Int64Counter(ledgerFailuresInstrument,
		metric.WithDescription("Failed domain event ledger write attempts"), metric.WithUnit("{event}"))
	if err != nil {
		h.instrumentWarning(ledgerFailuresInstrument)
	}
	return h
}

func (h *ledgerHandler) instrumentWarning(instrument string) {
	h.fallback.WarnContext(context.Background(), "domain event instrument creation failed",
		"event", "domain_event.instrument.initialization_failed", "instrument", instrument)
}

func (h *ledgerHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h *ledgerHandler) Handle(ctx context.Context, record slog.Record) error {
	if h.definition.Name() == "" && !h.hasDefinition(record) {
		return nil
	}
	definition := h.definition
	var attrs ledgerObject
	var marker *domainevent.Definition
	if len(h.groups) == 0 {
		attrs = cloneLedgerObject(h.attrs)
		marker = &definition
	} else {
		attrs = make(ledgerObject, record.NumAttrs())
	}
	record.Attrs(func(attr slog.Attr) bool {
		encodeLedgerAttr(attrs, attr, marker)
		return true
	})
	if definition.Name() == "" {
		return nil
	}
	document := attrs
	var identity groupedIdentity
	if len(h.groups) != 0 {
		document = mergeLedgerAttrs(h.attrs, h.groups, h.openGroups, attrs)
		identity, _ = ctx.Value(groupedIdentityKey{}).(groupedIdentity)
	}
	if eventName, ok := document["event"].(string); !ok || eventName != definition.Name() {
		return nil
	}
	event := ledgerOccurrence(record.Time, definition.Name(), document, attrs, identity)

	measurementCtx := context.WithoutCancel(ctx)
	appendCtx, cancel := context.WithTimeout(measurementCtx, h.writeTimeout)
	started := time.Now()
	err := h.appender.Append(appendCtx, event)
	elapsed := time.Since(started)
	cancel()
	outcome := "stored"
	if err != nil {
		outcome = "failed"
	}
	h.duration.Record(measurementCtx, elapsed.Seconds(), metric.WithAttributes(attribute.String("outcome", outcome)))
	if err != nil {
		kind := ledgerErrorKind(err)
		h.failures.Add(measurementCtx, 1, metric.WithAttributes(attribute.String("error_kind", string(kind))))
		h.fallback.ErrorContext(ctx, "domain event write failed",
			"event", "domain_event.ledger.write_failed", "failed_event", definition.Name(),
			"error_kind", string(kind), "duration_ms", elapsed.Milliseconds())
	}
	return nil
}

func ledgerOccurrence(occurredAt time.Time, name string, document, recordAttrs ledgerObject, identity groupedIdentity) *model.DomainEvent {
	event := &model.DomainEvent{
		ID:         id.DomainEventID(uuid.Must(uuid.NewV7())),
		Event:      name,
		OccurredAt: occurredAt.UTC(),
		Attributes: document,
	}
	actorAttrs, traceAttrs := document, document
	switch identity {
	case groupedActorAndTrace:
		actorAttrs, traceAttrs = recordAttrs, recordAttrs
	case groupedTraceOnly:
		traceAttrs = recordAttrs
	}
	if principal, ok := document["principal"].(string); ok {
		event.Principal = &principal
	}
	if actor, ok := actorAttrs["actor"].(string); ok {
		event.Actor = &actor
	}
	event.TraceID, _ = traceAttrs["trace_id"].(string)
	delete(document, "event")
	delete(document, "principal")
	delete(document, "actor")
	delete(document, "trace_id")
	return event
}

func (h *ledgerHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clone := *h
	var encoded ledgerObject
	var marker *domainevent.Definition
	if len(h.groups) == 0 {
		encoded = cloneLedgerObject(h.attrs)
		marker = &clone.definition
	} else {
		encoded = make(ledgerObject, len(attrs))
	}
	for _, attr := range attrs {
		encodeLedgerAttr(encoded, attr, marker)
	}
	if len(encoded) == 0 {
		return h
	}
	clone.attrs = encoded
	if len(h.groups) != 0 {
		clone.attrs = mergeLedgerAttrs(h.attrs, h.groups, h.openGroups, encoded)
	}
	clone.openGroups = len(h.groups)
	return &clone
}

func (h *ledgerHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	clone := *h
	clone.groups = make([]string, len(h.groups)+1)
	copy(clone.groups, h.groups)
	clone.groups[len(h.groups)] = name
	return &clone
}

func (h *ledgerHandler) hasDefinition(record slog.Record) bool {
	if len(h.groups) != 0 {
		return false
	}
	found := false
	record.Attrs(func(attr slog.Attr) bool {
		found = hasLedgerDefinition(attr)
		return !found
	})
	return found
}

func hasLedgerDefinition(attr slog.Attr) bool {
	if attr.Key == "event" && attr.Value.Kind() == slog.KindLogValuer {
		definition, ok := attr.Value.Any().(domainevent.Definition)
		return ok && definition.Name() != ""
	}
	if attr.Key == "" && attr.Value.Kind() == slog.KindGroup {
		for _, nested := range attr.Value.Group() {
			if hasLedgerDefinition(nested) {
				return true
			}
		}
	}
	return false
}

func encodeLedgerAttr(dst ledgerObject, attr slog.Attr, marker *domainevent.Definition) {
	var definition domainevent.Definition
	var typed bool
	if marker != nil && attr.Key == "event" && attr.Value.Kind() == slog.KindLogValuer {
		definition, typed = attr.Value.Any().(domainevent.Definition)
	}
	value := attr.Value.Resolve()
	if (slog.Attr{Key: attr.Key, Value: value}).Equal(slog.Attr{}) {
		return
	}
	var encoded any
	if value.Kind() == slog.KindGroup {
		group := make(ledgerObject, len(value.Group()))
		var nestedMarker *domainevent.Definition
		if attr.Key == "" {
			nestedMarker = marker
		}
		for _, nested := range value.Group() {
			encodeLedgerAttr(group, nested, nestedMarker)
		}
		if len(group) == 0 {
			return
		}
		if attr.Key == "" {
			for key, item := range group {
				dst[key] = item
			}
			return
		}
		encoded = group
	} else {
		encoded = encodeLedgerValue(value)
	}
	dst[attr.Key] = encoded
	if marker != nil && attr.Key == "event" {
		if typed {
			*marker = definition
		} else {
			*marker = domainevent.Definition{}
		}
	}
}

func encodeLedgerValue(value slog.Value) any {
	switch value.Kind() {
	case slog.KindString:
		return value.String()
	case slog.KindBool:
		return value.Bool()
	case slog.KindInt64:
		return value.Int64()
	case slog.KindUint64:
		return value.Uint64()
	case slog.KindDuration:
		return int64(value.Duration())
	case slog.KindFloat64:
		if number := value.Float64(); !math.IsInf(number, 0) && !math.IsNaN(number) {
			return number
		}
	case slog.KindTime:
		data, err := value.Time().MarshalJSON()
		return ledgerEncodedJSON(data, err)
	}
	item := value.Any()
	if item == nil {
		return nil
	}
	if _, marshaler := item.(json.Marshaler); !marshaler {
		if err, ok := item.(error); ok {
			return err.Error()
		}
	}
	data, err := json.Marshal(item)
	return ledgerEncodedJSON(data, err)
}

func ledgerEncodedJSON(data []byte, err error) any {
	if err != nil {
		return fmt.Sprintf("!ERROR:%v", err)
	}
	return json.RawMessage(data)
}

// Only the open group path is copied; other objects are immutable snapshots.
// Opening a new group replaces an earlier object with that key, as decoded JSON does.
func mergeLedgerAttrs(inherited ledgerObject, groups []string, openGroups int, attrs ledgerObject) ledgerObject {
	root := cloneLedgerObject(inherited)
	if len(attrs) == 0 {
		return root
	}
	dst := root
	for i, name := range groups {
		var group ledgerObject
		if i < openGroups {
			previous, _ := dst[name].(ledgerObject)
			group = cloneLedgerObject(previous)
		} else {
			group = make(ledgerObject)
		}
		dst[name] = group
		dst = group
	}
	for key, value := range attrs {
		dst[key] = value
	}
	return root
}

func cloneLedgerObject(source ledgerObject) ledgerObject {
	clone := maps.Clone(source)
	if clone == nil {
		clone = make(ledgerObject)
	}
	return clone
}

func ledgerErrorKind(err error) storage.ErrorKind {
	if errors.Is(err, context.DeadlineExceeded) {
		return storage.ErrorKindTimeout
	}
	var storageErr *storage.StorageError
	if errors.As(err, &storageErr) {
		switch storageErr.Kind {
		case storage.ErrorKindConnection, storage.ErrorKindTimeout, storage.ErrorKindValidation,
			storage.ErrorKindNotFound, storage.ErrorKindConflict:
			return storageErr.Kind
		}
	}
	return storage.ErrorKindUnknown
}
