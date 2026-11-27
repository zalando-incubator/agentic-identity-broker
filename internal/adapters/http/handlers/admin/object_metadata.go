package admin

import (
	"bytes"
	"encoding/json/jsontext"
	"errors"
	"strings"

	"github.com/agentic-identity-broker/agentic-identity-broker/internal/domain/model"
)

const metadataMember = "metadata"

// Messages are fixed so that no error can carry submitted text or decoder detail.
var (
	errMetadataMember               = errors.New("request body must contain at most one metadata member spelled exactly metadata")
	errMetadataNull                 = errors.New("metadata must not be null")
	errMetadataNotObject            = errors.New("metadata must be an object")
	errMetadataUnknownMember        = errors.New("metadata contains an unknown or repeated member")
	errMetadataLabelsNull           = errors.New("metadata.labels must not be null")
	errMetadataLabelsNotObject      = errors.New("metadata.labels must be an object")
	errMetadataLabelsDuplicate      = errors.New("metadata.labels contains a duplicate key")
	errMetadataAnnotationsNull      = errors.New("metadata.annotations must not be null")
	errMetadataAnnotationsNotObject = errors.New("metadata.annotations must be an object")
	errMetadataAnnotationsDuplicate = errors.New("metadata.annotations contains a duplicate key")
)

// ObjectMetadataResponse presents labels and annotations. Empty maps are
// omitted, and encoding/json writes map keys in ascending byte order.
type ObjectMetadataResponse struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

func objectMetadataResponse(metadata model.ObjectMetadata) *ObjectMetadataResponse {
	if metadata.IsEmpty() {
		return nil
	}
	return &ObjectMetadataResponse{Labels: metadata.Labels(), Annotations: metadata.Annotations()}
}

// decodeRequestMetadata reads the top-level metadata member of the first JSON
// value in body. Other members keep their existing decoding, so the scan
// tolerates invalid UTF-8 and duplicate names outside metadata. The returned
// error is always one of the fixed metadata errors.
func decodeRequestMetadata(body []byte) (model.ObjectMetadata, bool, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(body), jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true))
	token, err := decoder.ReadToken()
	if err != nil {
		return model.ObjectMetadata{}, false, errMetadataMember
	}
	if token.Kind() != '{' {
		return model.ObjectMetadata{}, false, nil
	}
	var raw jsontext.Value
	present := false
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return model.ObjectMetadata{}, false, errMetadataMember
		}
		switch {
		case name.String() == metadataMember:
			if present {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
			value, err := decoder.ReadValue()
			if err != nil {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
			raw, present = value.Clone(), true
		case strings.EqualFold(name.String(), metadataMember):
			return model.ObjectMetadata{}, false, errMetadataMember
		default:
			if err := decoder.SkipValue(); err != nil {
				return model.ObjectMetadata{}, false, errMetadataMember
			}
		}
	}
	if !present {
		return model.ObjectMetadata{}, false, nil
	}
	metadata, err := parseMetadataValue(raw)
	if err != nil {
		return model.ObjectMetadata{}, true, err
	}
	return metadata, true, nil
}

type metadataMapRules struct {
	null, notObject, duplicate, invalidKey, invalidValue error
	check                                                func(key, value string) error
}

var (
	labelRules = metadataMapRules{
		null: errMetadataLabelsNull, notObject: errMetadataLabelsNotObject, duplicate: errMetadataLabelsDuplicate,
		invalidKey: model.ErrMetadataLabelsInvalidKey, invalidValue: model.ErrMetadataLabelsInvalidValue, check: model.CheckLabelEntry,
	}
	annotationRules = metadataMapRules{
		null: errMetadataAnnotationsNull, notObject: errMetadataAnnotationsNotObject, duplicate: errMetadataAnnotationsDuplicate,
		invalidKey: model.ErrMetadataAnnotationsInvalidKey, invalidValue: model.ErrMetadataAnnotationsInvalidValue, check: model.CheckAnnotationEntry,
	}
)

// parseMetadataValue checks the metadata shape first, then labels, then
// annotations, whatever their order in the document.
func parseMetadataValue(raw jsontext.Value) (model.ObjectMetadata, error) {
	decoder := jsontext.NewDecoder(bytes.NewReader(raw), jsontext.AllowInvalidUTF8(true), jsontext.AllowDuplicateNames(true))
	token, err := decoder.ReadToken()
	if err != nil {
		return model.ObjectMetadata{}, errMetadataNotObject
	}
	switch token.Kind() {
	case 'n':
		return model.ObjectMetadata{}, errMetadataNull
	case '{':
	default:
		return model.ObjectMetadata{}, errMetadataNotObject
	}
	var rawLabels, rawAnnotations jsontext.Value
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return model.ObjectMetadata{}, errMetadataUnknownMember
		}
		var target *jsontext.Value
		switch name.String() {
		case "labels":
			target = &rawLabels
		case "annotations":
			target = &rawAnnotations
		default:
			return model.ObjectMetadata{}, errMetadataUnknownMember
		}
		if *target != nil {
			return model.ObjectMetadata{}, errMetadataUnknownMember
		}
		value, err := decoder.ReadValue()
		if err != nil {
			return model.ObjectMetadata{}, errMetadataUnknownMember
		}
		*target = value.Clone()
	}
	labels, err := parseMetadataMap(rawLabels, labelRules)
	if err != nil {
		return model.ObjectMetadata{}, err
	}
	if _, err := model.NewObjectMetadata(labels, nil); err != nil {
		return model.ObjectMetadata{}, err
	}
	annotations, err := parseMetadataMap(rawAnnotations, annotationRules)
	if err != nil {
		return model.ObjectMetadata{}, err
	}
	return model.NewObjectMetadata(labels, annotations)
}

// parseMetadataMap decodes one map strictly: invalid UTF-8 and unpaired
// surrogates fail, and entries are checked in document order.
func parseMetadataMap(raw jsontext.Value, rules metadataMapRules) (map[string]string, error) {
	if raw == nil {
		return nil, nil
	}
	decoder := jsontext.NewDecoder(bytes.NewReader(raw), jsontext.AllowDuplicateNames(true))
	token, err := decoder.ReadToken()
	if err != nil {
		return nil, rules.notObject
	}
	switch token.Kind() {
	case 'n':
		return nil, rules.null
	case '{':
	default:
		return nil, rules.notObject
	}
	entries := make(map[string]string)
	for decoder.PeekKind() != '}' {
		name, err := decoder.ReadToken()
		if err != nil {
			return nil, rules.invalidKey
		}
		key := name.String()
		if _, seen := entries[key]; seen {
			return nil, rules.duplicate
		}
		if err := rules.check(key, ""); err != nil {
			return nil, err
		}
		value, err := decoder.ReadToken()
		if err != nil || value.Kind() != '"' {
			return nil, rules.invalidValue
		}
		if err := rules.check(key, value.String()); err != nil {
			return nil, err
		}
		entries[key] = value.String()
	}
	return entries, nil
}
