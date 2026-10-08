package ledger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/url"
	"reflect"
	"strings"
	"time"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
	"github.com/google/jsonschema-go/jsonschema"
)

const schemaURNPrefix = "urn:agentic-identity-broker:events:v1:"
const schemaDraft = "https://json-schema.org/draft/2020-12/schema"

var errInvalidSchema = errors.New("invalid business event schema source")

type registeredEvent struct {
	schema      *jsonschema.Resolved
	outcome     model.BusinessEventOutcome
	reasonUser  string
	reasonAdmin string
}

type Registry struct {
	events map[string]registeredEvent
}

// NewRegistry compiles the published schemas and any additional offline sources once.
// The first source supplies the common envelope; later sources may only add new types.
func NewRegistry(sources ...fs.FS) (*Registry, error) {
	if len(sources) == 0 {
		return nil, errInvalidSchema
	}
	documents := make(map[string]*jsonschema.Schema)
	typeCount := 0
	var envelope *jsonschema.Schema
	for index, source := range sources {
		if source == nil || nilSchemaSource(source) {
			return nil, errInvalidSchema
		}
		entries, err := fs.ReadDir(source, ".")
		if err != nil {
			return nil, fmt.Errorf("schema source %d: %w", index, errInvalidSchema)
		}
		count := 0
		for _, entry := range entries {
			filename := entry.Name()
			if entry.IsDir() || !fs.ValidPath(filename) || strings.Contains(filename, "/") {
				return nil, errInvalidSchema
			}
			if filename == "examples.json" {
				continue
			}
			name, ok := strings.CutSuffix(filename, ".schema.json")
			if !ok || !validSchemaName(name) {
				return nil, errInvalidSchema
			}
			contents, err := fs.ReadFile(source, filename)
			if err != nil {
				return nil, fmt.Errorf("schema source %d: %w", index, errInvalidSchema)
			}
			var schema jsonschema.Schema
			if err := json.Unmarshal(contents, &schema); err != nil || schema.Schema != schemaDraft || schema.ID != schemaURNPrefix+name {
				return nil, errInvalidSchema
			}
			if _, exists := documents[schema.ID]; exists {
				return nil, errInvalidSchema
			}
			if index != 0 && (name == "envelope" || name == "catalogue") {
				return nil, errInvalidSchema
			}
			documents[schema.ID] = &schema
			switch name {
			case "envelope":
				envelope = &schema
			case "catalogue":
			default:
				typeCount++
			}
			count++
		}
		if count == 0 {
			return nil, errInvalidSchema
		}
	}
	if envelope == nil || typeCount == 0 || envelope.Type != "object" || !closedSchema(envelope) || !schemaConst(envelope.Properties["source"], model.BusinessEventSource) {
		return nil, errInvalidSchema
	}
	basePaths := make(map[string]bool)
	if err := checkSchemaTree(envelope, true, basePaths); err != nil {
		return nil, err
	}

	loader := func(uri *url.URL) (*jsonschema.Schema, error) {
		if uri.Fragment != "" || !validSchemaURN(uri.String()) {
			return nil, errInvalidSchema
		}
		schema := documents[uri.String()]
		if schema == nil {
			return nil, errInvalidSchema
		}
		return schema, nil
	}
	registry := &Registry{events: make(map[string]registeredEvent, typeCount)}
	for urn, schema := range documents {
		if schema != envelope {
			paths := make(map[string]bool)
			if urn != schemaURNPrefix+"catalogue" {
				paths = maps.Clone(basePaths)
			}
			if err := checkSchemaTree(schema, false, paths); err != nil {
				return nil, fmt.Errorf("schema %s: %w", urn, err)
			}
		}
		resolved, err := schema.Resolve(&jsonschema.ResolveOptions{Loader: loader})
		if err != nil {
			return nil, fmt.Errorf("schema %s: %w", urn, errInvalidSchema)
		}
		if schema == envelope || strings.HasSuffix(urn, ":catalogue") {
			continue
		}
		name := strings.TrimPrefix(urn, schemaURNPrefix)
		registered, err := registerEvent(name, schema, resolved)
		if err != nil {
			return nil, fmt.Errorf("schema %s: %w", urn, err)
		}
		registry.events[model.BusinessEventTypePrefix+name] = registered
	}
	return registry, nil
}

func (r *Registry) Validate(event *model.BusinessEvent) (map[string]any, error) {
	if r == nil || event == nil {
		return nil, errInvalidEvent
	}
	registered, exists := r.events[event.Type]
	if !exists {
		return nil, errInvalidEvent
	}
	if err := validateEventValues(event); err != nil || !validPayloadValues(event.Data) {
		return nil, errInvalidEvent
	}
	if event.Outcome != registered.outcome || event.ReasonUser != registered.reasonUser || event.ReasonAdmin != registered.reasonAdmin {
		return nil, errInvalidEvent
	}
	wire := event.Wire()
	if err := registered.schema.Validate(wire); err != nil {
		// The library reports rejected instance values; never return its error.
		return nil, errInvalidEvent
	}
	return wire, nil
}

func registerEvent(name string, schema *jsonschema.Schema, resolved *jsonschema.Resolved) (registeredEvent, error) {
	var event registeredEvent
	constraints := append([]*jsonschema.Schema{schema}, schema.AllOf...)
	refersToEnvelope := false
	dataClosed := false
	values := make(map[string]string, 5)
	for _, constraint := range constraints {
		if constraint.Ref == schemaURNPrefix+"envelope" {
			refersToEnvelope = true
		}
		for _, key := range []string{"type", "outcome", "source", "reason_user", "reason_admin"} {
			property, exists := constraint.Properties[key]
			if !exists {
				continue
			}
			value, ok := stringConst(property)
			if !ok || values[key] != "" && values[key] != value {
				return event, errInvalidSchema
			}
			values[key] = value
		}
		if data, exists := constraint.Properties["data"]; exists {
			if data.Type != "object" || !closedSchema(data) {
				return event, errInvalidSchema
			}
			dataClosed = true
		}
	}
	if !refersToEnvelope || !dataClosed || values["type"] != model.BusinessEventTypePrefix+name || values["source"] != "" && values["source"] != model.BusinessEventSource {
		return event, errInvalidSchema
	}
	event.outcome = model.BusinessEventOutcome(values["outcome"])
	switch event.outcome {
	case model.BusinessEventSuccess, model.BusinessEventFailure, model.BusinessEventDenied, model.BusinessEventPending:
	default:
		return event, errInvalidSchema
	}
	event.reasonUser, event.reasonAdmin = schema.Description, schema.Description
	if user := values["reason_user"]; user != "" {
		event.reasonUser = user
	}
	if admin := values["reason_admin"]; admin != "" {
		event.reasonAdmin = admin
	}
	if !fixedSentence(event.reasonUser) || !fixedSentence(event.reasonAdmin) {
		return event, errInvalidSchema
	}
	if recipe, catalogue := catalogueRecipes[name]; catalogue && (event.outcome != recipe.outcome || event.reasonUser != recipe.reason || event.reasonAdmin != recipe.reason) {
		return event, errInvalidSchema
	}
	event.schema = resolved
	return event, nil
}

func validSchemaName(name string) bool {
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		return false
	}
	for _, c := range name[1:] {
		if c >= 'a' && c <= 'z' || c >= '0' && c <= '9' || c == '-' {
			continue
		}
		return false
	}
	return true
}

func validSchemaURN(ref string) bool {
	name, ok := strings.CutPrefix(ref, schemaURNPrefix)
	return ok && validSchemaName(name)
}

func nilSchemaSource(source fs.FS) bool {
	value := reflect.ValueOf(source)
	switch value.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Func, reflect.Interface:
		return value.IsNil()
	}
	return false
}

func stringConst(schema *jsonschema.Schema) (string, bool) {
	if schema == nil || schema.Const == nil {
		return "", false
	}
	value, ok := (*schema.Const).(string)
	return value, ok && value != ""
}

func schemaConst(schema *jsonschema.Schema, expected string) bool {
	value, ok := stringConst(schema)
	return ok && value == expected
}

func closedSchema(schema *jsonschema.Schema) bool {
	return schema != nil && reflect.DeepEqual(schema.AdditionalProperties, &jsonschema.Schema{Not: &jsonschema.Schema{}})
}

func fixedSentence(sentence string) bool {
	return sentence != "" && strings.TrimSpace(sentence) == sentence && strings.HasSuffix(sentence, ".") && !strings.ContainsAny(sentence, "\r\n\t")
}

// checkSchemaTree checks property names even inside alternatives and $defs, not
// only in the branches exercised by a particular event instance.
func checkSchemaTree(root *jsonschema.Schema, envelope bool, paths map[string]bool) error {
	var walk func(*jsonschema.Schema, string, bool) error
	walk = func(schema *jsonschema.Schema, path string, top bool) error {
		if schema == nil || !top && (schema.ID != "" || schema.Schema != "") || len(schema.Extra) != 0 || len(schema.PatternProperties) != 0 || schema.DynamicRef != "" || schema.Ref != "" && !validSchemaURN(schema.Ref) {
			return errInvalidSchema
		}
		if schema.Type == "object" && !top && (!envelope || path != "data") && !closedSchema(schema) {
			return errInvalidSchema
		}
		if schema.Type == "array" && (schema.Items == nil || !scalarArrayItem(schema.Items)) {
			return errInvalidSchema
		}
		for name, child := range schema.Properties {
			if strings.Contains(name, ".") || strings.HasPrefix(name, "_ledger") || name == "" || child == nil {
				return errInvalidSchema
			}
			field := name
			if path != "" {
				field = path + "." + name
			}
			object := child.Type == "object" || len(child.Properties) > 0
			if previous, exists := paths[field]; exists && previous != object {
				return errInvalidSchema
			}
			paths[field] = object
			if err := walk(child, field, false); err != nil {
				return err
			}
		}
		for _, children := range [][]*jsonschema.Schema{schema.AllOf, schema.AnyOf, schema.OneOf, schema.PrefixItems, schema.ItemsArray} {
			for _, child := range children {
				if err := walk(child, path, false); err != nil {
					return err
				}
			}
		}
		for _, definitions := range []map[string]*jsonschema.Schema{schema.Defs, schema.Definitions, schema.DependencySchemas, schema.DependentSchemas} {
			for _, child := range definitions {
				if err := walk(child, path, false); err != nil {
					return err
				}
			}
		}
		for _, child := range []*jsonschema.Schema{schema.Items, schema.AdditionalProperties, schema.AdditionalItems, schema.Contains, schema.UnevaluatedItems, schema.UnevaluatedProperties, schema.PropertyNames, schema.Not, schema.If, schema.Then, schema.Else, schema.ContentSchema} {
			if child != nil {
				if err := walk(child, path, false); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(root, "", true)
}

func scalarArrayItem(schema *jsonschema.Schema) bool {
	switch schema.Type {
	case "string", "integer", "number", "boolean":
		return true
	}
	return false
}

func validPayloadValues(data map[string]any) bool {
	for key, value := range data {
		switch key {
		case "credential_id", "signing_key_id":
			if !validPayloadUUID(value) {
				return false
			}
		case "activates_at":
			text, ok := value.(string)
			if !ok || !strings.HasSuffix(text, "Z") {
				return false
			}
			instant, err := time.Parse(time.RFC3339Nano, text)
			if err != nil || !isUTC(instant) {
				return false
			}
		}
		switch item := value.(type) {
		case map[string]any:
			if item == nil || !validPayloadValues(item) {
				return false
			}
		default:
			if !primitiveValue(item) && !primitiveArray(item) {
				return false
			}
		}
	}
	return true
}

func primitiveValue(value any) bool {
	switch value.(type) {
	case nil, string, bool:
		return true
	}
	_, ok := model.BusinessEventNumericValue(value)
	return ok
}

func primitiveArray(value any) bool {
	items := reflect.ValueOf(value)
	if !items.IsValid() || items.Kind() != reflect.Slice || items.IsNil() {
		return false
	}
	switch items.Type().Elem().Kind() {
	case reflect.Bool, reflect.String, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Float32, reflect.Float64, reflect.Interface:
	default:
		return false
	}
	hasFloat, lossyInteger := false, false
	for i := range items.Len() {
		value := items.Index(i).Interface()
		switch value.(type) {
		case nil, string, bool:
			continue
		}
		number, ok := model.BusinessEventNumericValue(value)
		if !ok {
			return false
		}
		switch number := number.(type) {
		case float64, uint64:
			hasFloat = true
		case int64:
			widened := float64(number)
			lossyInteger = lossyInteger || widened >= 1<<63 || int64(widened) != number
		}
	}
	return !hasFloat || !lossyInteger
}
