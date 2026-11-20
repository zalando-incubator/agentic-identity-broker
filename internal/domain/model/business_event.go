package model

import (
	"errors"
	"reflect"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type BusinessEvent struct {
	ID         id.BusinessEventID
	Event      string
	OccurredAt time.Time
	RecordedAt time.Time
	Principal  *string
	Actor      *string
	TraceID    string
	Attributes map[string]any
}

func (e *BusinessEvent) Validate() error {
	if e.ID.IsZero() {
		return errors.New("business event ID is required")
	}
	if e.Event == "" {
		return errors.New("business event identifier is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("business event occurrence time is required")
	}
	return nil
}

func (e *BusinessEvent) Copy() *BusinessEvent {
	copy := *e
	if e.Principal != nil {
		principal := *e.Principal
		copy.Principal = &principal
	}
	if e.Actor != nil {
		actor := *e.Actor
		copy.Actor = &actor
	}
	if e.Attributes != nil {
		copy.Attributes = copyBusinessEventValue(reflect.ValueOf(e.Attributes)).Interface().(map[string]any)
	}
	return &copy
}

func copyBusinessEventValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		return copyBusinessEventValue(value.Elem())
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			copy.SetMapIndex(iter.Key(), copyBusinessEventValue(iter.Value()))
		}
		return copy
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			copy.Index(i).Set(copyBusinessEventValue(value.Index(i)))
		}
		return copy
	case reflect.Array:
		copy := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			copy.Index(i).Set(copyBusinessEventValue(value.Index(i)))
		}
		return copy
	default:
		return value
	}
}
