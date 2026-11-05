package telemetry

import (
	"encoding/json"
	"errors"
	"math"
	"reflect"
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
		return businessEventArrayAttribute(reflect.ValueOf(value))
	default:
		if number, ok := model.BusinessEventNumericValue(value); ok {
			switch number := number.(type) {
			case int64:
				return attribute.Int64Value(number), nil
			case uint64:
				return attribute.Float64Value(float64(number)), nil
			case float64:
				return attribute.Float64Value(number), nil
			}
		}
		items := reflect.ValueOf(value)
		if items.IsValid() && items.Kind() == reflect.Slice {
			switch items.Type().Elem().Kind() {
			case reflect.Int8, reflect.Int16, reflect.Int32, reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32:
				return businessEventArrayAttribute(items)
			case reflect.String:
				if items.Type().Elem() == reflect.TypeFor[json.Number]() {
					return businessEventArrayAttribute(items)
				}
			}
		}
	}
	return attribute.Value{}, errBusinessEventEncoding
}

func businessEventArrayAttribute(items reflect.Value) (attribute.Value, error) {
	if items.Len() == 0 {
		return attribute.StringSliceValue(nil), nil
	}
	values := make([]attribute.Value, items.Len())
	hasFloat := false
	for i := range items.Len() {
		primitive, err := businessEventAttribute(items.Index(i).Interface())
		if err != nil || (primitive.Type() != attribute.STRING && primitive.Type() != attribute.BOOL && primitive.Type() != attribute.INT64 && primitive.Type() != attribute.FLOAT64) {
			return attribute.Value{}, errBusinessEventEncoding
		}
		if i != 0 && primitive.Type() != values[0].Type() {
			numeric := primitive.Type() == attribute.INT64 || primitive.Type() == attribute.FLOAT64
			firstNumeric := values[0].Type() == attribute.INT64 || values[0].Type() == attribute.FLOAT64
			if !numeric || !firstNumeric {
				return attribute.Value{}, errBusinessEventEncoding
			}
		}
		hasFloat = hasFloat || primitive.Type() == attribute.FLOAT64
		values[i] = primitive
	}
	if hasFloat {
		for i, value := range values {
			if value.Type() == attribute.INT64 {
				integer := value.AsInt64()
				widened := float64(integer)
				if widened >= 1<<63 || int64(widened) != integer {
					return attribute.Value{}, errBusinessEventEncoding
				}
				values[i] = attribute.Float64Value(widened)
			}
		}
	}
	return attribute.SliceValue(values...), nil
}
