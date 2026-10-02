package telemetry

import (
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	otellog "go.opentelemetry.io/otel/log"
)

var errBusinessEventEncoding = errors.New("invalid business event telemetry encoding")

func encodeBusinessEvent(event *model.BusinessEvent) (otellog.Record, int, error) {
	var record otellog.Record
	if event == nil {
		return record, 0, errBusinessEventEncoding
	}
	var attributes []attribute.KeyValue
	var nulls, emptyObjects []string
	if err := flattenBusinessEvent("", event.Wire(), &attributes, &nulls, &emptyObjects); err != nil {
		return record, 0, err
	}
	if len(nulls) != 0 {
		slices.Sort(nulls)
		attributes = append(attributes, attribute.StringSlice("_ledger.null_fields", nulls))
	}
	if len(emptyObjects) != 0 {
		slices.Sort(emptyObjects)
		attributes = append(attributes, attribute.StringSlice("_ledger.empty_objects", emptyObjects))
	}
	slices.SortFunc(attributes, func(a, b attribute.KeyValue) int { return strings.Compare(string(a.Key), string(b.Key)) })
	record.SetEventName(event.Type)
	record.SetTimestamp(event.OccurredAt)
	record.SetObservedTimestamp(time.Now().UTC())
	record.AddAttributes(attributes...)
	return record, len(attributes), nil
}

func flattenBusinessEvent(path string, value any, attributes *[]attribute.KeyValue, nulls, emptyObjects *[]string) error {
	if value == nil {
		*nulls = append(*nulls, path)
		return nil
	}
	if object, ok := value.(map[string]any); ok {
		if len(object) == 0 {
			*emptyObjects = append(*emptyObjects, path)
		}
		for key, child := range object {
			if key == "" || strings.Contains(key, ".") || strings.HasPrefix(key, "_ledger") {
				return errBusinessEventEncoding
			}
			childPath := key
			if path != "" {
				childPath = path + "." + key
			}
			if err := flattenBusinessEvent(childPath, child, attributes, nulls, emptyObjects); err != nil {
				return err
			}
		}
		return nil
	}
	primitive, err := businessEventAttribute(value)
	if err != nil {
		return err
	}
	*attributes = append(*attributes, attribute.KeyValue{Key: attribute.Key(path), Value: primitive})
	return nil
}

func businessEventAttribute(value any) (attribute.Value, error) {
	switch value := value.(type) {
	case string:
		return attribute.StringValue(value), nil
	case bool:
		return attribute.BoolValue(value), nil
	case int:
		return attribute.IntValue(value), nil
	case int64:
		return attribute.Int64Value(value), nil
	case float64:
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			return attribute.Float64Value(value), nil
		}
	case json.Number:
		if integer, err := value.Int64(); err == nil {
			return attribute.Int64Value(integer), nil
		}
		if number, err := value.Float64(); err == nil {
			return businessEventAttribute(number)
		}
	case []string:
		return attribute.StringSliceValue(value), nil
	case []bool:
		return attribute.BoolSliceValue(value), nil
	case []int:
		return attribute.IntSliceValue(value), nil
	case []int64:
		return attribute.Int64SliceValue(value), nil
	case []float64:
		for _, number := range value {
			if math.IsNaN(number) || math.IsInf(number, 0) {
				return attribute.Value{}, errBusinessEventEncoding
			}
		}
		return attribute.Float64SliceValue(value), nil
	case []any:
		if len(value) == 0 {
			return attribute.StringSliceValue(nil), nil
		}
		values := make([]attribute.Value, len(value))
		for i, item := range value {
			primitive, err := businessEventAttribute(item)
			if err != nil || (primitive.Type() != attribute.STRING && primitive.Type() != attribute.BOOL && primitive.Type() != attribute.INT64 && primitive.Type() != attribute.FLOAT64) {
				return attribute.Value{}, errBusinessEventEncoding
			}
			if i != 0 && primitive.Type() != values[0].Type() {
				return attribute.Value{}, errBusinessEventEncoding
			}
			values[i] = primitive
		}
		return attribute.SliceValue(values...), nil
	}
	return attribute.Value{}, errBusinessEventEncoding
}
