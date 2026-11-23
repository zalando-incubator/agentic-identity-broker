package model

import (
	"errors"
	"reflect"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/id"
)

type DomainEvent struct {
	ID         id.DomainEventID
	Event      string
	OccurredAt time.Time
	RecordedAt time.Time
	Principal  *string
	Actor      *string
	TraceID    string
	Attributes map[string]any
}

func (e *DomainEvent) Validate() error {
	if e.ID.IsZero() {
		return errors.New("domain event ID is required")
	}
	if e.Event == "" {
		return errors.New("domain event identifier is required")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("domain event occurrence time is required")
	}
	return nil
}

func (e *DomainEvent) Copy() *DomainEvent {
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
		copy.Attributes = copyDomainEventValue(reflect.ValueOf(e.Attributes)).Interface().(map[string]any)
	}
	return &copy
}

func copyDomainEventValue(value reflect.Value) reflect.Value {
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return value
		}
		return copyDomainEventValue(value.Elem())
	case reflect.Map:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			copy.SetMapIndex(iter.Key(), copyDomainEventValue(iter.Value()))
		}
		return copy
	case reflect.Slice:
		if value.IsNil() {
			return value
		}
		copy := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := range value.Len() {
			copy.Index(i).Set(copyDomainEventValue(value.Index(i)))
		}
		return copy
	case reflect.Array:
		copy := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			copy.Index(i).Set(copyDomainEventValue(value.Index(i)))
		}
		return copy
	case reflect.Pointer:
		if value.IsNil() {
			return value
		}
		copy := reflect.New(value.Type().Elem())
		copy.Elem().Set(copyDomainEventValue(value.Elem()))
		return copy.Convert(value.Type())
	case reflect.Struct:
		copy := reflect.New(value.Type()).Elem()
		copy.Set(value)
		for i := range value.NumField() {
			if field := copy.Field(i); field.CanSet() {
				field.Set(copyDomainEventValue(value.Field(i)))
			}
		}
		return copy
	default:
		return value
	}
}
