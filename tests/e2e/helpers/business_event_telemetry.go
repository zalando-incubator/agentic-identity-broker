package helpers

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
)

// LedgerTelemetryEnvelope checks the wire contract, including all field paths.
func LedgerTelemetryEnvelope(event *model.BusinessEvent, envelope map[string]any, record OTLPLogRecord) error {
	if record.Scope.Name != LedgerOTLPScope || record.Record.EventName != event.Type {
		return fmt.Errorf("ledger scope or event name differs")
	}
	if record.Record.TimeUnixNano != uint64(event.OccurredAt.UnixNano()) {
		return fmt.Errorf("ledger occurrence timestamp differs")
	}
	if record.Record.ObservedTimeUnixNano > math.MaxInt64 {
		return fmt.Errorf("ledger observed timestamp is outside the supported range")
	}
	observed := time.Unix(0, int64(record.Record.ObservedTimeUnixNano))
	if record.ExportStartedAt.IsZero() || record.CapturedAt.IsZero() || observed.Before(record.ExportStartedAt.Add(-5*time.Second)) || observed.After(record.CapturedAt) {
		return fmt.Errorf("ledger observed timestamp is outside the export interval")
	}
	if len(record.Record.TraceId) != 0 && (event.TraceID == "" || hex.EncodeToString(record.Record.TraceId) != event.TraceID) {
		return fmt.Errorf("native ledger trace differs from authoritative event trace")
	}
	if len(record.Record.SpanId) != 0 && (event.SpanID == "" || hex.EncodeToString(record.Record.SpanId) != event.SpanID || len(record.Record.TraceId) == 0) {
		return fmt.Errorf("native ledger span differs from authoritative event correlation")
	}
	if record.Record.Body != nil && record.Record.Body.Value != nil {
		return fmt.Errorf("ledger body is not empty")
	}
	if err := validateLedgerRawEnvelope(envelope); err != nil {
		return err
	}
	expected := map[string]any{}
	nulls, empty := []string{}, []string{}
	var flatten func(string, any)
	flatten = func(path string, value any) {
		switch typed := value.(type) {
		case nil:
			nulls = append(nulls, path)
		case map[string]any:
			if len(typed) == 0 {
				empty = append(empty, path)
			}
			for key, child := range typed {
				next := key
				if path != "" {
					next = path + "." + key
				}
				flatten(next, child)
			}
		default:
			expected[path] = value
		}
	}
	flatten("", envelope)
	sort.Strings(nulls)
	sort.Strings(empty)
	if len(nulls) != 0 {
		values := make([]any, len(nulls))
		for index, value := range nulls {
			values[index] = value
		}
		expected["_ledger.null_fields"] = values
	}
	if len(empty) != 0 {
		values := make([]any, len(empty))
		for index, value := range empty {
			values[index] = value
		}
		expected["_ledger.empty_objects"] = values
	}
	actual := map[string]any{}
	for _, attribute := range record.Record.Attributes {
		if _, duplicate := actual[attribute.Key]; duplicate {
			return fmt.Errorf("duplicate ledger attribute")
		}
		actual[attribute.Key] = ledgerAttributeValue(attribute.Value)
	}
	actualJSON, err := json.Marshal(actual)
	if err != nil {
		return err
	}
	expectedJSON, err := json.Marshal(expected)
	if err != nil {
		return err
	}
	if string(actualJSON) != string(expectedJSON) {
		return fmt.Errorf("ledger flat attributes differ from the complete envelope")
	}
	if record.Record.DroppedAttributesCount != 0 {
		return fmt.Errorf("ledger attributes were dropped")
	}
	return nil
}

func ledgerAttributeValue(value *commonv1.AnyValue) any {
	if value == nil {
		return nil
	}
	switch typed := value.Value.(type) {
	case *commonv1.AnyValue_StringValue:
		return typed.StringValue
	case *commonv1.AnyValue_BoolValue:
		return typed.BoolValue
	case *commonv1.AnyValue_IntValue:
		return typed.IntValue
	case *commonv1.AnyValue_DoubleValue:
		return typed.DoubleValue
	case *commonv1.AnyValue_ArrayValue:
		values := make([]any, len(typed.ArrayValue.Values))
		for index, child := range typed.ArrayValue.Values {
			values[index] = ledgerAttributeValue(child)
		}
		return values
	default:
		return nil
	}
}
