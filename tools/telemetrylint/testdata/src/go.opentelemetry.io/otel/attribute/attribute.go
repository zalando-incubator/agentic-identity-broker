// Package attribute is a minimal stub of go.opentelemetry.io/otel/attribute for analyzer tests.
package attribute

import "fmt"

type Key string

type Value struct{}

type KeyValue struct {
	Key   Key
	Value Value
}

type Set struct{}

func (k Key) String(v string) KeyValue { return KeyValue{Key: k} }

func (k Key) Int(v int) KeyValue { return KeyValue{Key: k} }

func String(k, v string) KeyValue { return KeyValue{Key: Key(k)} }

func Int(k string, v int) KeyValue { return KeyValue{Key: Key(k)} }

func Bool(k string, v bool) KeyValue { return KeyValue{Key: Key(k)} }

func Stringer(k string, v fmt.Stringer) KeyValue { return KeyValue{Key: Key(k)} }

func NewSet(kvs ...KeyValue) Set { return Set{} }
