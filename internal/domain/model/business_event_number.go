package model

import (
	"encoding/json"
	"maps"
	"math"
	"math/big"
	"math/bits"
	"reflect"
	"slices"
	"strconv"
)

// BusinessEventNumericValue returns a lossless JSON number that OTLP can represent.
func BusinessEventNumericValue(value any) (any, bool) {
	switch value := value.(type) {
	case int, int8, int16, int32, int64:
		return reflect.ValueOf(value).Int(), true
	case uint, uint8, uint16, uint32, uint64:
		number := reflect.ValueOf(value).Uint()
		if number <= math.MaxInt64 {
			return int64(number), true
		}
		if bits.Len64(number)-bits.TrailingZeros64(number) <= 53 {
			return number, true
		}
	case float32:
		number := float64(value)
		if !math.IsNaN(number) && !math.IsInf(number, 0) {
			// Widen the JSON decimal, not float32's binary approximation.
			number, _ = strconv.ParseFloat(strconv.FormatFloat(number, 'g', -1, 32), 64)
			return number, true
		}
	case float64:
		if !math.IsNaN(value) && !math.IsInf(value, 0) {
			return value, true
		}
	case json.Number:
		if !json.Valid([]byte(value)) {
			return nil, false
		}
		if integer, err := value.Int64(); err == nil {
			return integer, true
		}
		number, err := value.Float64()
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, false
		}
		decimal := strconv.FormatFloat(number, 'g', -1, 64)
		if string(value) == decimal {
			return number, true
		}
		exact, ok := new(big.Rat).SetString(string(value))
		if !ok {
			return nil, false
		}
		if exact.IsInt() && exact.Num().IsInt64() {
			return exact.Num().Int64(), true
		}
		if exact.IsInt() && exact.Num().IsUint64() {
			return BusinessEventNumericValue(exact.Num().Uint64())
		}
		roundTrip, ok := new(big.Rat).SetString(decimal)
		if ok && exact.Cmp(roundTrip) == 0 || exact.Cmp(new(big.Rat).SetFloat64(number)) == 0 {
			return number, true
		}
	}
	return nil, false
}

func businessEventPayloadWire(value any) (any, bool) {
	switch value := value.(type) {
	case json.Number, float32:
		return BusinessEventNumericValue(value)
	case map[string]any:
		var wire map[string]any
		for key, item := range value {
			if converted, changed := businessEventPayloadWire(item); changed {
				if wire == nil {
					wire = maps.Clone(value)
				}
				wire[key] = converted
			}
		}
		if wire != nil {
			return wire, true
		}
	case []any:
		var wire []any
		for i, item := range value {
			if converted, changed := businessEventPayloadWire(item); changed {
				if wire == nil {
					wire = slices.Clone(value)
				}
				wire[i] = converted
			}
		}
		if wire != nil {
			return wire, true
		}
	case []json.Number, []float32:
		items := reflect.ValueOf(value)
		wire := make([]any, items.Len())
		for i := range items.Len() {
			item := items.Index(i).Interface()
			converted, changed := businessEventPayloadWire(item)
			if !changed {
				return value, false
			}
			wire[i] = converted
		}
		return wire, true
	}
	return value, false
}
